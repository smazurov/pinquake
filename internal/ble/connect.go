package ble

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

// SetSensorFactory stores a matched sensor factory for the next connection.
func (s *Scanner) SetSensorFactory(factory func() sensors.Sensor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sensorFactory = factory
}

// ConnectedDevice is the device a link came up to.
type ConnectedDevice struct {
	Addr       string
	Name       string
	SensorName string
}

// OnConnect sets a callback invoked (from a goroutine) after each successful
// connection, including automatic reconnects.
func (s *Scanner) OnConnect(fn func(ConnectedDevice)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onConnect = fn
}

// Connect makes addr the wanted device. The supervisor scans until it
// advertises, connects, and reconnects whenever the link drops.
func (s *Scanner) Connect(addr, name string) error {
	if _, err := bluetooth.ParseMAC(addr); err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	addr = strings.ToUpper(addr)
	s.mu.Lock()
	s.deviceAddr = addr
	s.deviceName = name
	s.mu.Unlock()
	s.sup.Want(addr)
	return nil
}

// Wants reports whether addr is the device to stay connected to.
func (s *Scanner) Wants(addr string) bool { return s.sup.Wanted() == addr }

// Retry searches for the wanted device now if waiting to; see
// Supervisor.Retry.
func (s *Scanner) Retry() { s.sup.Retry() }

// Disconnect forgets the wanted device: closes the link or stops searching.
func (s *Scanner) Disconnect() {
	s.sup.Forget()
}

// onLinkStatus publishes every supervisor transition as a BLE status event.
// Called from the supervisor loop, so events leave in transition order.
func (s *Scanner) onLinkStatus(st LinkStatus) {
	s.logger.Info("BLE link", "state", st.State, "addr", st.Addr, "err", st.Err, "retry_in", st.RetryIn)

	now := time.Now()
	s.mu.Lock()
	prev := s.status
	next := linkStatus{LinkStatus: st}
	switch {
	case errors.Is(st.Err, ErrLinkLost):
		next.reason = ReasonLost
	case st.State == LinkSearching || st.State == LinkWaiting:
		if prev.Addr == st.Addr {
			next.reason = prev.reason // still searching for a lost link
		}
	case st.State == LinkIdle && prev.State != LinkIdle:
		next.reason = ReasonUser
	}
	if st.State == LinkWaiting {
		next.retryAt = now.Add(st.RetryIn)
	}
	s.status = next
	ev := s.statusEventLocked(now)
	cb := s.onConnect
	s.mu.Unlock()

	s.eventBus.Publish(ev)
	if st.State == LinkConnected && cb != nil {
		go cb(ConnectedDevice{Addr: ev.Device, Name: ev.DeviceName, SensorName: ev.SensorName})
	}
}

// onLink attaches a sensor to a new link. The GATT calls can't be
// cancelled, so they run in a goroutine; on ctx cancellation onLink returns
// at once, orphans the notification handler and closes a sensor that
// attaches late. An error makes the supervisor close the link and retry.
func (s *Scanner) onLink(ctx context.Context, link Link) error {
	dl, ok := link.(interface{ Device() *bluetooth.Device })
	if !ok {
		return fmt.Errorf("link %T has no BLE device", link)
	}
	device := dl.Device()

	s.mu.Lock()
	s.session++
	factory := s.sensorFactory
	s.mu.Unlock()
	handler := s.makeNotificationHandler()

	type attached struct {
		sensor sensors.Sensor
		batErr error
		err    error
	}
	done := make(chan attached, 1)
	go func() {
		sensor, err := s.attachSensor(device, factory, handler)
		res := attached{sensor: sensor, err: err}
		if err == nil {
			// Seeds lastCentavolts; BLE I/O can block 2+ seconds.
			_, res.batErr = sensor.ReadBattery()
		}
		done <- res
	}()

	select {
	case res := <-done:
		if res.err != nil {
			return res.err
		}
		sessCtx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.sensor = res.sensor
		s.connCtx, s.connCancel = sessCtx, cancel
		s.mu.Unlock()
		if res.batErr == nil {
			go s.pollBattery()
		}
		s.logger.Info("Sensor attached", "sensor", res.sensor.Name())
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		s.session++ // orphan the handler given to the late sensor
		s.mu.Unlock()
		go func() {
			if res := <-done; res.sensor != nil {
				res.sensor.Close()
			}
		}()
		return ctx.Err()
	}
}

func (s *Scanner) attachSensor(device *bluetooth.Device, factory func() sensors.Sensor, handler func([]byte)) (sensors.Sensor, error) {
	if factory != nil {
		sensor := factory()
		if err := sensor.Connect(device, handler); err != nil {
			return nil, fmt.Errorf("sensor connect failed: %w", err)
		}
		return sensor, nil
	}
	for _, entry := range sensors.Registry {
		candidate := entry.Factory()
		if err := candidate.Connect(device, handler); err != nil {
			s.logger.Info("Sensor connect failed, trying next", "sensor", entry.Name, "error", err)
			continue
		}
		return candidate, nil
	}
	return nil, fmt.Errorf("no compatible sensor found")
}

// onUnlink tears down the sensor session after the link closed or dropped.
// Auto-lock stays enabled; the next connection re-locks on its own.
func (s *Scanner) onUnlink() {
	s.mu.Lock()
	s.session++ // ignore notifications still queued from this link
	if s.connCancel != nil {
		s.connCancel()
		s.connCancel = nil
	}
	if s.sensor != nil {
		s.sensor.Close()
		s.sensor = nil
	}
	s.locker.ResetFrame()
	s.mu.Unlock()
	s.publishFrameState(nil)
}
