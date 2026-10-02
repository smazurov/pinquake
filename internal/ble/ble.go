package ble

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/framelock"
	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

// Reason says why a link ended, as reported in BLE status events.
type Reason string

const (
	ReasonLost Reason = "lost" // the device dropped the link
	ReasonUser Reason = "user" // disconnected on request
)

// linkStatus is the last supervisor status plus what the UI is told about it.
type linkStatus struct {
	LinkStatus
	reason  Reason
	retryAt time.Time // when waiting
}

type Scanner struct {
	adapter  *bluetooth.Adapter
	eventBus *events.Bus
	logger   *slog.Logger
	sup      *Supervisor

	mu            sync.Mutex
	status        linkStatus
	deviceAddr    string
	deviceName    string
	sensor        sensors.Sensor
	sensorFactory func() sensors.Sensor
	connCtx       context.Context // session lifetime (battery polling)
	connCancel    context.CancelFunc
	session       int // bumped per link; stale notification handlers no-op
	swapXY        bool
	onConnect     func(ConnectedDevice)

	locker         *framelock.Locker
	lastFrameState framelock.Status // last published, for dedup
	publishFrame   func(events.FrameStateEvent)
	// frameSem (one slot) makes "read status + publish" atomic, so events
	// leave in the order the state changed and the last one is current.
	frameSem chan struct{}

	ready chan struct{} // closed when adapter.Enable() succeeds
}

// Option configures a Scanner.
type Option func(*Scanner, *Radio)

// WithRadio drives r instead of the BlueZ adapter, e.g. a fake in tests.
// There is no adapter to enable then: Run starts at once, and
// InitWithRetry must not be called.
func WithRadio(r Radio) Option {
	return func(s *Scanner, radio *Radio) {
		*radio = r
		close(s.ready)
	}
}

func NewScanner(eventBus *events.Bus, logger *slog.Logger, opts ...Option) *Scanner {
	s := &Scanner{
		adapter:  bluetooth.DefaultAdapter,
		eventBus: eventBus,
		logger:   logger,
		status:   linkStatus{LinkStatus: LinkStatus{State: LinkIdle}},
		locker: framelock.New(framelock.Config{
			Window:          5 * time.Second,
			Threshold:       0.005,
			RelockThreshold: 0.01,
		}),
		ready: make(chan struct{}),
	}
	s.publishFrame = func(e events.FrameStateEvent) { s.eventBus.Publish(e) }
	s.frameSem = make(chan struct{}, 1)
	var radio Radio = &bluezRadio{adapter: s.adapter, logger: logger}
	for _, opt := range opts {
		opt(s, &radio)
	}
	s.sup = NewSupervisor(radio, DefaultSupervisorConfig(), SupervisorHooks{
		OnStatus: s.onLinkStatus,
		OnLink:   s.onLink,
		OnUnlink: s.onUnlink,
	})
	return s
}

// Run drives the connection supervisor once the adapter is enabled, until
// ctx is done. On return the link (if any) is closed.
func (s *Scanner) Run(ctx context.Context) {
	select {
	case <-s.ready:
	case <-ctx.Done():
		return
	}
	s.sup.Run(ctx)
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

// StatusEvent describes the current link, for clients that just subscribed.
func (s *Scanner) StatusEvent() events.BLEStatusEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusEventLocked(time.Now())
}

func (s *Scanner) statusEventLocked(now time.Time) events.BLEStatusEvent {
	st := s.status
	ev := events.BLEStatusEvent{
		Status:    string(st.State),
		Reason:    string(st.reason),
		Device:    st.Addr,
		Timestamp: now.Format(time.RFC3339Nano),
	}
	if st.Addr != "" && st.Addr == s.deviceAddr {
		ev.DeviceName = s.deviceName
	}
	if st.State == LinkConnected && s.sensor != nil {
		ev.SensorName = s.sensor.Name()
	}
	if st.State == LinkWaiting && st.Err != nil {
		ev.Error = st.Err.Error()
		ev.RetryInS = max(0, st.retryAt.Sub(now).Seconds())
	}
	return ev
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
	s.frameSem <- struct{}{}
	defer func() { <-s.frameSem }()

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
	s.publishFrame(ev)
}

// ConfigureLink sets the connection supervisor's timings.
func (s *Scanner) ConfigureLink(cfg SupervisorConfig) { s.sup.Configure(cfg) }

func (s *Scanner) SetSwapXY(swap bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.swapXY = swap
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
