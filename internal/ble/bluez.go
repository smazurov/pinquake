package ble

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

// dbusTimeout bounds every BlueZ call the radio makes, so the Radio and
// Link contracts (return promptly after cancel / Close) hold even when
// bluetoothd is slow or wedged.
const dbusTimeout = 5 * time.Second

// bluezRadio is the Radio backed by tinygo bluetooth over BlueZ D-Bus.
// The adapter must be enabled before use.
type bluezRadio struct {
	adapter *bluetooth.Adapter
	logger  *slog.Logger
}

func (r *bluezRadio) Scan(ctx context.Context, found func(Advertisement)) error {
	done := make(chan error, 1)
	go func() {
		done <- r.adapter.Scan(func(_ *bluetooth.Adapter, res bluetooth.ScanResult) {
			adv := Advertisement{
				Address: res.Address.String(),
				Name:    res.LocalName(),
				RSSI:    int(res.RSSI),
			}
			if entry := sensors.Match(res); entry != nil {
				adv.SensorName = entry.Name
			}
			found(adv)
		})
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	// tinygo's StopScan takes no context.
	if err := withTimeout("stop scan", r.adapter.StopScan); err != nil {
		r.logger.Warn("BLE stop scan failed", "error", err)
	}
	select {
	case <-done:
		return nil
	case <-time.After(dbusTimeout):
		return fmt.Errorf("scan did not stop within %s", dbusTimeout)
	}
}

// Connect wraps tinygo's blocking Connect, which ignores ConnectionTimeout
// on Linux. On cancellation the pending BlueZ connect is aborted with
// Device1.Disconnect ("can be also used to cancel a preceding Connect
// call", org.bluez.Device docs); a connection that completes anyway is
// disconnected.
func (r *bluezRadio) Connect(ctx context.Context, addr string) (Link, error) {
	mac, err := bluetooth.ParseMAC(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid MAC address: %w", err)
	}
	obj, path, err := deviceObject(addr)
	if err != nil {
		return nil, err
	}
	ch := make(chan tinygoConnect, 1)
	go func() {
		dev, err := r.adapter.Connect(bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, bluetooth.ConnectionParams{})
		ch <- tinygoConnect{dev, err}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("connect failed: %w", res.err)
		}
		return newBluezLink(res.dev, obj, path, r.logger)
	case <-ctx.Done():
		_ = disconnectDevice(obj) // aborts the pending Connect
		go reapAbandonedConnect(r.logger, addr, ch, func() error { return disconnectDevice(obj) })
		return nil, ctx.Err()
	}
}

// tinygoConnect is the result of tinygo's blocking Adapter.Connect.
type tinygoConnect struct {
	dev bluetooth.Device
	err error
}

// reapAbandonedConnect waits for a tinygo Connect we gave up on and drops
// the connection if it completed anyway. tinygo can block forever: if BlueZ
// answers Device1.Connect but our abort lands before the Connected signal,
// it waits on that signal indefinitely. That leaks its goroutine (and this
// one), so it is logged once after dbusTimeout, and again if it is released.
func reapAbandonedConnect(logger *slog.Logger, addr string, ch <-chan tinygoConnect, disconnect func() error) {
	var res tinygoConnect
	select {
	case res = <-ch:
	case <-time.After(dbusTimeout):
		logger.Warn("Abandoned BLE connect still blocked in tinygo; goroutine leaked until BlueZ answers",
			"addr", addr, "after", dbusTimeout)
		res = <-ch
		logger.Info("Abandoned BLE connect returned; leaked goroutine released", "addr", addr, "error", res.err)
	}
	if res.err == nil {
		_ = disconnect()
	}
}

func deviceObject(addr string) (dbus.BusObject, dbus.ObjectPath, error) {
	bus, err := dbus.SystemBus()
	if err != nil {
		return nil, "", fmt.Errorf("open D-Bus: %w", err)
	}
	path := dbus.ObjectPath("/org/bluez/hci0/dev_" + strings.ReplaceAll(strings.ToUpper(addr), ":", "_"))
	return bus.Object("org.bluez", path), path, nil
}

func disconnectDevice(obj dbus.BusObject) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer cancel()
	return obj.CallWithContext(ctx, "org.bluez.Device1.Disconnect", 0).Err
}

// withTimeout runs a context-less blocking call (tinygo's D-Bus wrappers)
// for at most dbusTimeout. On timeout the call is abandoned, not cancelled.
func withTimeout(what string, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(dbusTimeout):
		return fmt.Errorf("%s: timed out after %s", what, dbusTimeout)
	}
}

// bluezLink watches BlueZ's Device1.Connected property to detect drops;
// tinygo only reports disconnects in peripheral mode.
type bluezLink struct {
	dev       bluetooth.Device
	obj       dbus.BusObject
	lost      chan struct{}
	stopWatch context.CancelFunc
	closeOnce sync.Once
	closeErr  error
}

func newBluezLink(dev bluetooth.Device, obj dbus.BusObject, path dbus.ObjectPath, logger *slog.Logger) (*bluezLink, error) {
	bus, err := dbus.SystemBus()
	if err != nil {
		_ = disconnectDevice(obj)
		return nil, fmt.Errorf("open D-Bus for link watch: %w", err)
	}
	match := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath(path),
	}
	if err := bus.AddMatchSignal(match...); err != nil {
		_ = disconnectDevice(obj)
		return nil, fmt.Errorf("watch link: %w", err)
	}
	sigCh := make(chan *dbus.Signal, 16)
	bus.Signal(sigCh)

	ctx, cancel := context.WithCancel(context.Background())
	l := &bluezLink{dev: dev, obj: obj, lost: make(chan struct{}), stopWatch: cancel}

	go func() {
		defer bus.RemoveSignal(sigCh)
		defer func() { _ = bus.RemoveMatchSignal(match...) }()
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-sigCh:
				if sig.Path != path || !reportsDisconnected(sig) {
					continue
				}
				logger.Warn("BlueZ reports device disconnected", "path", path)
				close(l.lost)
				return
			}
		}
	}()

	// Subscribed first, so a drop between connect and here is not missed.
	callCtx, callCancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer callCancel()
	var connected dbus.Variant
	err = obj.CallWithContext(callCtx, "org.freedesktop.DBus.Properties.Get", 0, "org.bluez.Device1", "Connected").Store(&connected)
	if err == nil {
		if c, _ := connected.Value().(bool); !c {
			_ = l.Close()
			return nil, fmt.Errorf("device disconnected during connect")
		}
	}
	return l, nil
}

func reportsDisconnected(sig *dbus.Signal) bool {
	if sig.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" || len(sig.Body) < 2 {
		return false
	}
	if iface, _ := sig.Body[0].(string); iface != "org.bluez.Device1" {
		return false
	}
	changes, _ := sig.Body[1].(map[string]dbus.Variant)
	v, ok := changes["Connected"]
	if !ok {
		return false
	}
	connected, _ := v.Value().(bool)
	return !connected
}

func (l *bluezLink) Lost() <-chan struct{} { return l.lost }

// Device exposes the tinygo device for sensor setup.
func (l *bluezLink) Device() *bluetooth.Device { return &l.dev }

// Close disconnects with a bounded D-Bus call (tinygo's Disconnect has no
// timeout). Idempotent and safe after the link was lost.
func (l *bluezLink) Close() error {
	l.closeOnce.Do(func() {
		l.stopWatch()
		l.closeErr = disconnectDevice(l.obj)
	})
	return l.closeErr
}
