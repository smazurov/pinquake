package ble

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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

// bluezRadio is the Radio over BlueZ D-Bus: it scans on its own D-Bus
// connections and connects through tinygo bluetooth. The adapter must be
// enabled before use.
type bluezRadio struct {
	adapter *bluetooth.Adapter
	logger  *slog.Logger
}

const adapterPath = dbus.ObjectPath("/org/bluez/hci0")

var errAdapterOff = errors.New("bluetooth adapter powered off")

// Scan runs one discovery session on its own D-Bus connection.
//
// BlueZ keeps a discovery session per client connection, and its Discovering
// property is per adapter: it drops whenever another client that set a
// discovery filter disconnects, even while we are still registered.
// tinygo's Adapter.Scan treats that as the end of its scan and returns
// without StopDiscovery, so BlueZ answers every later StartDiscovery on the
// shared connection with "Operation already in progress". Here Discovering
// is ignored, and closing the connection ends the session even if
// StopDiscovery fails.
func (r *bluezRadio) Scan(ctx context.Context, found func(Advertisement)) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("open D-Bus for scan: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Subscribe before starting so no device found early is missed. The
	// connection is ours, so closing it drops the matches.
	for _, match := range [][]dbus.MatchOption{
		{dbus.WithMatchSender("org.bluez"), dbus.WithMatchInterface("org.freedesktop.DBus.ObjectManager"), dbus.WithMatchMember("InterfacesAdded")},
		{dbus.WithMatchSender("org.bluez"), dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchPathNamespace(adapterPath)},
	} {
		if err := conn.AddMatchSignal(match...); err != nil {
			return fmt.Errorf("watch scan results: %w", err)
		}
	}
	sigs := make(chan *dbus.Signal, 256)
	conn.Signal(sigs)

	adapter := conn.Object("org.bluez", adapterPath)
	if err := callBounded(adapter, "org.bluez.Adapter1.SetDiscoveryFilter", map[string]any{"Transport": "le"}); err != nil {
		return fmt.Errorf("set discovery filter: %w", err)
	}
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	listCtx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	err = conn.Object("org.bluez", "/").CallWithContext(listCtx, "org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objects)
	cancel()
	if err != nil {
		return fmt.Errorf("list known devices: %w", err)
	}
	devices := newDiscovered(objects)
	if err := callBounded(adapter, "org.bluez.Adapter1.StartDiscovery"); err != nil {
		return fmt.Errorf("start discovery: %w", err)
	}

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case sig, ok := <-sigs:
			if !ok {
				return errors.New("D-Bus connection closed during scan")
			}
			adv, seen, err := devices.handle(sig)
			if err != nil {
				return err
			}
			if seen {
				found(adv)
			}
		}
	}
	if err := callBounded(adapter, "org.bluez.Adapter1.StopDiscovery"); err != nil {
		r.logger.Warn("BLE stop discovery failed; closing the connection ends it", "error", err)
	}
	return nil
}

// discovered is BlueZ's view of the devices under our adapter, built from
// the signals of one scan.
type discovered map[dbus.ObjectPath]map[string]dbus.Variant

// newDiscovered starts from the devices BlueZ already knows, as listed by
// GetManagedObjects: they get no InterfacesAdded when seen again.
func newDiscovered(objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant) discovered {
	d := discovered{}
	for path, ifaces := range objects {
		if props, ok := ifaces["org.bluez.Device1"]; ok && underAdapter(path) {
			d[path] = props
		}
	}
	return d
}

func underAdapter(path dbus.ObjectPath) bool {
	return strings.HasPrefix(string(path), string(adapterPath)+"/")
}

// handle applies sig and reports whether it carried an advertisement.
func (d discovered) handle(sig *dbus.Signal) (Advertisement, bool, error) {
	switch sig.Name {
	case "org.freedesktop.DBus.ObjectManager.InterfacesAdded":
		if len(sig.Body) < 2 {
			return Advertisement{}, false, nil
		}
		path, _ := sig.Body[0].(dbus.ObjectPath)
		ifaces, _ := sig.Body[1].(map[string]map[string]dbus.Variant)
		props, ok := ifaces["org.bluez.Device1"]
		if !ok || !underAdapter(path) {
			return Advertisement{}, false, nil
		}
		d[path] = props
		return advertisement(props), true, nil
	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		if len(sig.Body) < 2 {
			return Advertisement{}, false, nil
		}
		iface, _ := sig.Body[0].(string)
		changed, _ := sig.Body[1].(map[string]dbus.Variant)
		if iface == "org.bluez.Adapter1" && sig.Path == adapterPath {
			if on, ok := changed["Powered"].Value().(bool); ok && !on {
				return Advertisement{}, false, errAdapterOff
			}
			return Advertisement{}, false, nil
		}
		props, ok := d[sig.Path]
		if iface != "org.bluez.Device1" || !ok {
			return Advertisement{}, false, nil
		}
		maps.Copy(props, changed)
		// BlueZ updates these only from received advertisements.
		_, rssi := changed["RSSI"]
		_, mdata := changed["ManufacturerData"]
		_, sdata := changed["ServiceData"]
		return advertisement(props), rssi || mdata || sdata, nil
	}
	return Advertisement{}, false, nil
}

func advertisement(props map[string]dbus.Variant) Advertisement {
	addr, _ := props["Address"].Value().(string)
	name, _ := props["Name"].Value().(string)
	rssi, _ := props["RSSI"].Value().(int16)
	adv := Advertisement{Address: addr, Name: name, RSSI: int(rssi)}
	uuidStrs, _ := props["UUIDs"].Value().([]string)
	uuids := make([]bluetooth.UUID, 0, len(uuidStrs))
	for _, s := range uuidStrs {
		if u, err := bluetooth.ParseUUID(s); err == nil {
			uuids = append(uuids, u)
		}
	}
	if entry := sensors.Match(uuids); entry != nil {
		adv.SensorName = entry.Name
	}
	return adv
}

// callBounded calls method with dbusTimeout.
func callBounded(obj dbus.BusObject, method string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer cancel()
	return obj.CallWithContext(ctx, method, 0, args...).Err
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
