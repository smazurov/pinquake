package ble

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/framelock"
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
	connCtx       context.Context
	connCancel    context.CancelFunc
	swapXY        bool
	disconnecting bool
	onConnect     func(sensorName string)

	locker         *framelock.Locker
	lastFrameState framelock.Status // last published, for dedup

	ready chan struct{} // closed when adapter.Enable() succeeds
}

func NewScanner(eventBus *events.Bus, logger *slog.Logger) *Scanner {
	return &Scanner{
		adapter:  bluetooth.DefaultAdapter,
		eventBus: eventBus,
		logger:   logger,
		state:    StateIdle,
		locker: framelock.New(framelock.Config{
			Window:          5 * time.Second,
			Threshold:       0.005,
			RelockThreshold: 0.01,
		}),
		ready: make(chan struct{}),
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

// FrameAction performs a lock action and returns the resulting status.
// Valid actions: "enable", "disable", "trigger".
func (s *Scanner) FrameAction(action string) framelock.Status {
	s.mu.Lock()
	switch action {
	case "enable":
		s.locker.Enable()
	case "disable":
		s.locker.Disable()
	case "trigger":
		s.locker.Trigger()
	}
	st := s.locker.Status()
	s.mu.Unlock()
	s.publishFrameState(nil)
	return st
}

func (s *Scanner) FrameStatus() framelock.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.locker.Status()
}

// ConfigureFrameLock applies auto-lock settings. Re-applying unchanged
// settings does not disturb an existing lock.
func (s *Scanner) ConfigureFrameLock(cfg framelock.Config) {
	s.mu.Lock()
	s.locker.Configure(cfg)
	s.mu.Unlock()
	s.publishFrameState(nil)
}

// publishFrameState publishes a FrameStateEvent when the lock state changed
// or a new lock was applied. Must not be called with s.mu held.
func (s *Scanner) publishFrameState(lock *framelock.Lock) {
	s.mu.Lock()
	st := s.locker.Status()
	changed := st.Enabled != s.lastFrameState.Enabled || st.State != s.lastFrameState.State
	s.lastFrameState = st
	s.mu.Unlock()

	if !changed && lock == nil {
		return
	}
	ev := events.FrameStateEvent{
		Enabled:   st.Enabled,
		State:     string(st.State),
		Stdev:     st.Stdev,
		Timestamp: time.Now().Format(time.RFC3339Nano),
	}
	if lock != nil {
		ev.Reason = string(lock.Reason)
		ev.Stdev = lock.Stdev
		ev.Drift = lock.Drift
	}
	s.eventBus.Publish(ev)
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
	err := entry.ApplyConfig(sensor, cfg)
	// Samples from before the reconfiguration (rate/filter changes) are not
	// representative; restart stability evaluation either way.
	s.mu.Lock()
	s.locker.DiscardSamples()
	s.mu.Unlock()
	return err
}

func errState(action string, state State) error {
	return fmt.Errorf("cannot %s: state is %s", action, state)
}
