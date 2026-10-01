package ble

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

// SetSensorFactory stores a matched sensor factory for the next connection.
func (s *Scanner) SetSensorFactory(factory func() sensors.Sensor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sensorFactory = factory
}

// OnConnect sets a callback invoked (from a goroutine) after each successful
// connection, including automatic reconnects.
func (s *Scanner) OnConnect(fn func(sensorName string)) {
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

// Disconnect forgets the wanted device: closes the link or stops searching.
func (s *Scanner) Disconnect() {
	s.sup.Forget()
}

// onLinkStatus maps supervisor transitions to BLE status events. Searching
// and backing off are not published: the UI keeps showing idle (or
// "connection lost") with its scan button available.
func (s *Scanner) onLinkStatus(st LinkStatus) {
	s.logger.Info("BLE link", "state", st.State, "addr", st.Addr, "err", st.Err, "retry_in", st.RetryIn)

	s.mu.Lock()
	prev := s.state
	switch st.State {
	case LinkConnecting:
		s.state = StateConnecting
	case LinkConnected:
		s.state = StateConnected
	default:
		s.state = StateIdle
	}
	var sensorName string
	if s.sensor != nil {
		sensorName = s.sensor.Name()
	}
	cb := s.onConnect
	s.mu.Unlock()

	switch {
	case st.State == LinkConnecting:
		s.publishStatus("connecting", st.Addr, "", "")
	case st.State == LinkConnected:
		s.publishStatus("connected", st.Addr, "", sensorName)
		if cb != nil {
			go cb(sensorName)
		}
	case errors.Is(st.Err, ErrLinkLost):
		s.publishStatus("disconnected", "", "lost", "")
	case st.State == LinkWaiting && prev == StateConnecting:
		s.publishStatus("idle", st.Addr, st.Err.Error(), "")
	case st.State == LinkWaiting && !errors.Is(st.Err, ErrNotFound):
		s.publishStatus("idle", "", "scan failed: "+st.Err.Error(), "")
	case st.State == LinkIdle && prev == StateConnected:
		s.publishStatus("disconnected", "", "user", "")
	case st.State == LinkIdle && prev == StateConnecting:
		s.publishStatus("idle", "", "", "")
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
