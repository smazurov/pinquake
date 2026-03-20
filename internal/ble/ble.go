package ble

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

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

type accelSample struct {
	mag float32
	t   time.Time
}

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
	connCtx       context.Context
	connCancel    context.CancelFunc
	refFrame    *orientation.Orientation
	refRotation orientation.Mat3
	gravity     [3]float32
	gMag        float32
	pendingLock   bool
	swapXY        bool
	disconnecting bool
	onConnect     func(sensorName string)

	autoLockEnabled         bool
	autoLockSpreadWindow    time.Duration
	autoLockSpreadThreshold float32
	autoLockSamples         []accelSample
	lastRaw                 *orientation.Orientation

	ready chan struct{} // closed when adapter.Enable() succeeds
}


func NewScanner(eventBus *events.Bus, logger *slog.Logger) *Scanner {
	return &Scanner{
		adapter:         bluetooth.DefaultAdapter,
		eventBus:        eventBus,
		logger:          logger,
		state:           StateIdle,
		autoLockSpreadWindow:    5 * time.Second,
		autoLockSpreadThreshold: 0.005,
		ready:           make(chan struct{}),
	}
}

func (s *Scanner) Init() error {
	err := s.adapter.Enable()
	if err == nil {
		close(s.ready)
	}
	return err
}

// InitWithRetry enables the BLE adapter, retrying with exponential backoff
// until it succeeds or stop is closed.
func (s *Scanner) InitWithRetry(stop chan struct{}) {
	delay := 2 * time.Second
	maxDelay := 30 * time.Second

	for {
		if err := s.adapter.Enable(); err != nil {
			s.logger.Warn("BLE adapter enable failed, retrying", "error", err, "retry_in", delay)
			select {
			case <-stop:
				s.logger.Info("BLE init retry cancelled")
				return
			case <-time.After(delay):
			}
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}
		s.logger.Info("BLE adapter enabled")
		close(s.ready)
		return
	}
}

// Ready returns a channel that is closed when the adapter is enabled.
func (s *Scanner) Ready() <-chan struct{} {
	return s.ready
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

func (s *Scanner) Sensor() sensors.Sensor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sensor
}


// FrameAction performs a lock action and returns the resulting lock state.
// Valid actions: "enable", "disable", "trigger".
func (s *Scanner) FrameAction(action string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch action {
	case "enable":
		s.autoLockEnabled = true
		s.pendingLock = true
		s.autoLockSamples = s.autoLockSamples[:0]
	case "disable":
		s.resetAutoLock()
	case "trigger":
		s.pendingLock = true
	}
	return s.autoLockEnabled
}

// resetAutoLock zeroes all auto-lock and reference frame fields. Caller must hold s.mu.
func (s *Scanner) resetAutoLock() {
	s.autoLockEnabled = false
	s.refFrame = nil
	s.gMag = 0
	s.gravity = [3]float32{}
	s.pendingLock = false
	s.autoLockSamples = s.autoLockSamples[:0]
}

func (s *Scanner) IsFrameLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.autoLockEnabled
}

func (s *Scanner) SetAutoLockParams(window time.Duration, threshold float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoLockSpreadWindow = window
	s.autoLockSpreadThreshold = threshold
}

func (s *Scanner) SetSwapXY(swap bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.swapXY = swap
}

func (s *Scanner) GetSensorName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sensor != nil {
		return s.sensor.Name()
	}
	return ""
}

func (s *Scanner) publishStatus(status, device, reason, sensorName string) {
	s.mu.Lock()
	name := s.deviceName
	s.mu.Unlock()
	s.eventBus.Publish(events.BLEStatusEvent{
		Status:     status,
		Reason:     reason,
		Device:     device,
		DeviceName: name,
		SensorName: sensorName,
		Timestamp:  time.Now().Format(time.RFC3339Nano),
	})
}

func (s *Scanner) ApplySensorConfig(entry sensors.SensorEntry, cfg any) error {
	s.mu.Lock()
	sensor := s.sensor
	s.mu.Unlock()
	if sensor == nil {
		return nil
	}
	if entry.ApplyConfig == nil {
		return nil
	}
	if sensor.Name() != entry.Name {
		return nil
	}
	return entry.ApplyConfig(sensor, cfg)
}

func errState(action string, state State) error {
	return fmt.Errorf("cannot %s: state is %s", action, state)
}
