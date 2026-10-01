package ble

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/framelock"
	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

// fakeSensor's Connect blocks until release (like a hung GATT discovery)
// and records the notification handler it was given.
type fakeSensor struct {
	release chan struct{}
	once    sync.Once

	mu      sync.Mutex
	handler func([]byte)
	closed  bool
}

func newFakeSensor() *fakeSensor { return &fakeSensor{release: make(chan struct{})} }

func (f *fakeSensor) unblock() { f.once.Do(func() { close(f.release) }) }

func (f *fakeSensor) Name() string                   { return "Fake" }
func (f *fakeSensor) ServiceUUIDs() []bluetooth.UUID { return nil }
func (f *fakeSensor) Connect(_ *bluetooth.Device, h func([]byte)) error {
	f.mu.Lock()
	f.handler = h
	f.mu.Unlock()
	<-f.release
	return nil
}
func (f *fakeSensor) ReadBattery() (*sensors.BatteryState, error) { return nil, sensors.ErrUnsupported }
func (f *fakeSensor) ReadTemperature() (float32, error)           { return 0, sensors.ErrUnsupported }
func (f *fakeSensor) Calibrate() error                            { return sensors.ErrUnsupported }
func (f *fakeSensor) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

func (f *fakeSensor) state() (func([]byte), bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handler, f.closed
}

// deviceLink is a Link that exposes a BLE device, like bluezLink.
type deviceLink struct{ dev bluetooth.Device }

func (l *deviceLink) Lost() <-chan struct{}     { return nil }
func (l *deviceLink) Close() error              { return nil }
func (l *deviceLink) Device() *bluetooth.Device { return &l.dev }

func TestSetupAttachesSensor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestScanner()
		fs := newFakeSensor()
		fs.unblock()
		s.SetSensorFactory(func() sensors.Sensor { return fs })

		if err := s.onLink(context.Background(), &deviceLink{}); err != nil {
			t.Fatalf("onLink: %v", err)
		}
		if s.Sensor() != fs {
			t.Fatal("sensor not attached")
		}
	})
}

func TestSetupHonoursCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestScanner()
		fs := newFakeSensor()
		defer fs.unblock()
		s.SetSensorFactory(func() sensors.Sensor { return fs })

		ctx, cancel := context.WithCancel(context.Background())
		errc := make(chan error, 1)
		go func() { errc <- s.onLink(ctx, &deviceLink{}) }()
		synctest.Wait() // stuck in sensor Connect

		cancel()
		synctest.Wait()
		select {
		case err := <-errc:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("onLink err = %v, want context.Canceled", err)
			}
		default:
			t.Fatal("onLink did not return after cancel")
		}

		fs.unblock() // GATT finally completes
		synctest.Wait()
		handler, closed := fs.state()
		if !closed {
			t.Error("late sensor not closed")
		}
		if s.Sensor() != nil {
			t.Error("late sensor attached after cancel")
		}
		stream(handler, lockWindow+time.Second)
		if st := s.FrameStatus(); st.State == framelock.StateLocked {
			t.Error("late sensor's notifications drove the frame lock")
		}
	})
}

func TestFrameStateEventsCannotArriveStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestScanner()
		hold := make(chan struct{})
		var mu sync.Mutex
		var got []events.FrameStateEvent
		first := true
		s.publishFrame = func(e events.FrameStateEvent) {
			mu.Lock()
			block := first
			first = false
			mu.Unlock()
			if block {
				<-hold // first publisher stalls after reading its state
			}
			mu.Lock()
			got = append(got, e)
			mu.Unlock()
		}

		go s.FrameAction("disable") // reads "disabled", then stalls in publish
		synctest.Wait()
		go s.FrameAction("enable") // a newer state is published meanwhile
		synctest.Wait()
		close(hold)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()
		last := got[len(got)-1]
		if st := s.FrameStatus(); last.Enabled != st.Enabled || last.State != string(st.State) {
			t.Fatalf("last event enabled=%v state=%s, but scanner is enabled=%v state=%s",
				last.Enabled, last.State, st.Enabled, st.State)
		}
	})
}
