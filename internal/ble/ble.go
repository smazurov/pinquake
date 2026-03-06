package ble

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/orientation"
	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

type State string

const (
	StateIdle       State = "idle"
	StateScanning   State = "scanning"
	StateConnecting State = "connecting"
	StateConnected  State = "connected"
)

type Scanner struct {
	adapter  *bluetooth.Adapter
	eventBus *events.Bus
	logger   *slog.Logger

	mu            sync.Mutex
	state         State
	device        *bluetooth.Device
	deviceName    string
	sensor        sensors.Sensor
	sensorFactory func() sensors.Sensor
	scanCancel    context.CancelFunc
	scanDone      chan struct{}
	batteryStop   chan struct{}
	connWatch     chan struct{}
	refFrame      *orientation.Orientation
	pendingLock   bool
	swapXY        bool
	disconnecting bool
	onConnect     func(sensorName string)

	autoLockEnabled bool
	autoLockTimeout time.Duration
	autoLockEpsilon float32
	autoLockRef     [3]float32
	autoLockSince   time.Time
}

func NewScanner(eventBus *events.Bus, logger *slog.Logger) *Scanner {
	return &Scanner{
		adapter:         bluetooth.DefaultAdapter,
		eventBus:        eventBus,
		logger:          logger,
		state:           StateIdle,
		autoLockTimeout: 10 * time.Second,
		autoLockEpsilon: 0.01,
	}
}

func (s *Scanner) Init() error {
	return s.adapter.Enable()
}

func (s *Scanner) watchConnection(deviceAddr string) {
	bus, err := dbus.SystemBus()
	if err != nil {
		s.logger.Warn("Failed to open D-Bus for connection watch", "error", err)
		return
	}

	devPath := "/org/bluez/hci0/dev_" + strings.ReplaceAll(deviceAddr, ":", "_")
	matchOpts := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath(dbus.ObjectPath(devPath)),
	}
	if err := bus.AddMatchSignal(matchOpts...); err != nil {
		s.logger.Warn("Failed to add D-Bus match signal", "error", err)
		return
	}

	sigCh := make(chan *dbus.Signal, 4)
	bus.Signal(sigCh)
	defer bus.RemoveSignal(sigCh)
	defer bus.RemoveMatchSignal(matchOpts...)

	s.logger.Info("Watching D-Bus for disconnect", "path", devPath)

	s.mu.Lock()
	stop := s.connWatch
	s.mu.Unlock()

	for {
		select {
		case <-stop:
			return
		case sig := <-sigCh:
			if sig.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" {
				continue
			}
			iface, _ := sig.Body[0].(string)
			if iface != "org.bluez.Device1" {
				continue
			}
			changes, _ := sig.Body[1].(map[string]dbus.Variant)
			connVar, ok := changes["Connected"]
			if !ok {
				continue
			}
			connected, _ := connVar.Value().(bool)
			if connected {
				continue
			}

			s.logger.Warn("D-Bus reports device disconnected")

			s.mu.Lock()
			if s.disconnecting || s.state != StateConnected {
				s.mu.Unlock()
				return
			}
			batteryStop := s.batteryStop
			if s.sensor != nil {
				s.sensor.Close()
			}
			s.device = nil
			s.deviceName = ""
			s.sensor = nil
			s.state = StateIdle
			s.refFrame = nil
			s.pendingLock = false
			s.autoLockEnabled = false
			s.autoLockSince = time.Time{}
			s.batteryStop = nil
			s.connWatch = nil
			s.mu.Unlock()

			if batteryStop != nil {
				close(batteryStop)
			}

			s.publishStatus("disconnected", "", "lost")
			return
		}
	}
}

func (s *Scanner) GetState() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Scanner) GetDeviceName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deviceName
}

// Sensor returns the currently connected sensor, or nil.
func (s *Scanner) Sensor() sensors.Sensor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sensor
}

// ReadBatteryBlock reads raw registers from a connected WT901 for debugging.
func (s *Scanner) ReadBatteryBlock() (map[string]uint16, error) {
	s.mu.Lock()
	sen := s.sensor
	s.mu.Unlock()
	if sen == nil {
		return nil, fmt.Errorf("no sensor connected")
	}
	wt, ok := sen.(*sensors.WT901)
	if !ok {
		return nil, fmt.Errorf("connected sensor is not WT901")
	}
	return wt.ReadBatteryBlock()
}

func (s *Scanner) LockFrame() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoLockEnabled = true
	s.pendingLock = true
	s.autoLockSince = time.Time{}
}

func (s *Scanner) UnlockFrame() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoLockEnabled = false
	s.refFrame = nil
	s.pendingLock = false
	s.autoLockSince = time.Time{}
}

func (s *Scanner) IsFrameLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.autoLockEnabled
}

func (s *Scanner) ForceLockFrame() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingLock = true
}

func (s *Scanner) SetAutoLockParams(epsilon float32, timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoLockEpsilon = epsilon
	s.autoLockTimeout = timeout
}

func (s *Scanner) SetSwapXY(swap bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.swapXY = swap
}

func (s *Scanner) Scan(ctx context.Context) error {
	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return fmt.Errorf("cannot scan: state is %s", s.state)
	}
	s.state = StateScanning

	scanCtx, cancel := context.WithCancel(ctx)
	s.scanCancel = cancel
	s.scanDone = make(chan struct{})
	done := s.scanDone
	s.mu.Unlock()

	s.publishStatus("scanning", "", "")

	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			if s.state == StateScanning {
				s.state = StateIdle
			}
			s.scanCancel = nil
			s.scanDone = nil
			s.mu.Unlock()
			s.publishStatus("idle", "", "")
			close(done)
		}()

		adapterDone := make(chan error, 1)

		go func() {
			adapterDone <- s.adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
				name := result.LocalName()
				var sensorName string
				factory := sensors.Match(result)
				if factory != nil {
					sensorName = factory().Name()
				}
				s.eventBus.Publish(events.BLEScanResultEvent{
					Address:    result.Address.String(),
					Name:       name,
					RSSI:       int(result.RSSI),
					SensorName: sensorName,
					Timestamp:  time.Now().Format(time.RFC3339Nano),
				})
			})
		}()

		select {
		case <-scanCtx.Done():
			_ = s.adapter.StopScan()
			<-adapterDone
		case err := <-adapterDone:
			if err != nil {
				s.logger.Error("BLE scan failed", "error", err)
			}
		}
	}()

	return nil
}

func (s *Scanner) stopScan() {
	s.mu.Lock()
	cancel := s.scanCancel
	done := s.scanDone
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *Scanner) Connect(addr, name string) error {
	s.stopScan()

	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return fmt.Errorf("cannot connect: state is %s", s.state)
	}
	s.state = StateConnecting
	s.deviceName = name
	s.mu.Unlock()

	s.publishStatus("connecting", addr, "")

	go func() {
		if err := s.connectDevice(addr); err != nil {
			s.logger.Error("Failed to connect", "address", addr, "error", err)
			s.mu.Lock()
			s.state = StateIdle
			s.deviceName = ""
			s.mu.Unlock()
			s.publishStatus("idle", err.Error(), "")
		}
	}()

	return nil
}

// SetSensorFactory stores a matched sensor factory for the next Connect call.
func (s *Scanner) SetSensorFactory(factory func() sensors.Sensor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sensorFactory = factory
}

// OnConnect sets a callback invoked (from a goroutine) after successful connection.
func (s *Scanner) OnConnect(fn func(sensorName string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onConnect = fn
}

func (s *Scanner) connectDevice(addr string) error {
	mac, err := bluetooth.ParseMAC(addr)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}

	address := bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}

	device, err := s.adapter.Connect(address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	handler := s.makeNotificationHandler()

	s.mu.Lock()
	factory := s.sensorFactory
	s.mu.Unlock()

	var sensor sensors.Sensor

	if factory != nil {
		sensor = factory()
		if err := sensor.Connect(&device, handler); err != nil {
			device.Disconnect()
			return fmt.Errorf("sensor connect failed: %w", err)
		}
	} else {
		for _, f := range sensors.Registry {
			candidate := f()
			if err := candidate.Connect(&device, handler); err != nil {
				s.logger.Info("Sensor connect failed, trying next", "sensor", candidate.Name(), "error", err)
				continue
			}
			sensor = candidate
			break
		}
		if sensor == nil {
			device.Disconnect()
			return fmt.Errorf("no compatible sensor found")
		}
	}

	// Seed lastCentavolts outside the mutex (BLE I/O can block 2+ seconds).
	_, batErr := sensor.ReadBattery()

	s.mu.Lock()
	s.device = &device
	s.state = StateConnected
	s.sensor = sensor

	if batErr == nil {
		s.batteryStop = make(chan struct{})
		go s.pollBattery()
	}

	s.connWatch = make(chan struct{})
	s.mu.Unlock()

	go s.watchConnection(addr)

	s.logger.Info("Connected to BLE device", "address", addr, "sensor", sensor.Name())
	s.publishStatus("connected", addr, "")

	s.mu.Lock()
	cb := s.onConnect
	s.mu.Unlock()
	if cb != nil {
		cb(sensor.Name())
	}

	return nil
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func (s *Scanner) makeNotificationHandler() func([]byte) {
	return func(buf []byte) {
		ori, ok := orientation.DecodeV1(buf)
		if !ok {
			return
		}

		var autoLockFired bool

		s.mu.Lock()
		if s.pendingLock {
			ref := ori
			s.refFrame = &ref
			s.pendingLock = false
		}

		// Auto-lock: check stability of raw sensor values
		if s.autoLockEnabled {
			raw := [3]float32{ori.Ax, ori.Ay, ori.Az}
			eps := s.autoLockEpsilon
			timeout := s.autoLockTimeout

			if abs32(raw[0]-s.autoLockRef[0]) > eps ||
				abs32(raw[1]-s.autoLockRef[1]) > eps ||
				abs32(raw[2]-s.autoLockRef[2]) > eps {
				s.autoLockRef = raw
				s.autoLockSince = time.Now()
			} else if s.autoLockSince.IsZero() {
				s.autoLockSince = time.Now()
			}

			if !s.autoLockSince.IsZero() && time.Since(s.autoLockSince) >= timeout {
				// Skip re-lock if already at origin (values unchanged from current ref)
				if s.refFrame != nil {
					t := orientation.CurrentInReferenceFrame(&ori, s.refFrame)
					if abs32(t.Ax) <= eps && abs32(t.Ay) <= eps && abs32(t.Az) <= eps {
						s.autoLockSince = time.Time{}
					} else {
						ref := ori
						s.refFrame = &ref
						s.autoLockSince = time.Time{}
						autoLockFired = true
					}
				} else {
					ref := ori
					s.refFrame = &ref
					s.autoLockSince = time.Time{}
					autoLockFired = true
				}
			}
		}

		ref := s.refFrame
		swap := s.swapXY
		s.mu.Unlock()

		if autoLockFired {
			s.eventBus.Publish(events.LogEntry{
				Message:   fmt.Sprintf("Auto-locked: stable for %ds", int(s.autoLockTimeout.Seconds())),
				Level:     "info",
				Timestamp: time.Now().Format(time.RFC3339Nano),
			})
		}

		if ref != nil {
			ori = orientation.CurrentInReferenceFrame(&ori, ref)
		}

		gx := ori.Ax
		gy := ori.Ay
		gz := ori.Az
		if swap {
			gx, gy = gy, gx
		}

		s.eventBus.Publish(events.OrientationEvent{
			Gx:        gx,
			Gy:        gy,
			Gz:        gz,
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
	}
}

func (s *Scanner) Disconnect() error {
	s.mu.Lock()
	if s.state == StateIdle {
		s.mu.Unlock()
		return nil
	}
	if s.state != StateConnected || s.device == nil {
		s.mu.Unlock()
		return fmt.Errorf("cannot disconnect: state is %s", s.state)
	}
	s.disconnecting = true
	device := s.device
	batteryStop := s.batteryStop
	connWatch := s.connWatch
	if s.sensor != nil {
		s.sensor.Close()
	}
	s.device = nil
	s.deviceName = ""
	s.sensor = nil
	s.state = StateIdle
	s.refFrame = nil
	s.pendingLock = false
	s.autoLockEnabled = false
	s.autoLockSince = time.Time{}
	s.batteryStop = nil
	s.connWatch = nil
	s.mu.Unlock()

	if batteryStop != nil {
		close(batteryStop)
	}
	if connWatch != nil {
		close(connWatch)
	}

	err := device.Disconnect()

	s.mu.Lock()
	s.disconnecting = false
	s.mu.Unlock()

	s.publishStatus("disconnected", "", "user")
	s.logger.Info("Disconnected from BLE device")
	return err
}

func (s *Scanner) pollBattery() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.readAndPublishBattery()

	for {
		select {
		case <-s.batteryStop:
			return
		case <-ticker.C:
			s.readAndPublishBattery()
		}
	}
}

func (s *Scanner) readAndPublishBattery() {
	s.mu.Lock()
	sen := s.sensor
	s.mu.Unlock()
	if sen == nil {
		return
	}
	bat, err := sen.ReadBattery()
	if err != nil {
		s.logger.Warn("Battery poll failed", "error", err)
		return
	}

	s.eventBus.Publish(events.BatteryEvent{
		BatteryPercent: bat.Percent,
		BatteryVolts:   bat.Volts,
		Charging:       bat.Charging,
		Timestamp:      time.Now().Format(time.RFC3339Nano),
	})
}

func (s *Scanner) publishStatus(status, device, reason string) {
	s.mu.Lock()
	name := s.deviceName
	s.mu.Unlock()
	s.eventBus.Publish(events.BLEStatusEvent{
		Status:     status,
		Reason:     reason,
		Device:     device,
		DeviceName: name,
		Timestamp:  time.Now().Format(time.RFC3339Nano),
	})
}
