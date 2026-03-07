package ble

import (
	"context"
	"fmt"

	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

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

func (s *Scanner) Connect(addr, name string) error {
	s.stopScan()

	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return errState("connect", s.state)
	}
	s.state = StateConnecting
	s.deviceName = name
	s.mu.Unlock()

	s.publishStatus("connecting", addr, "")

	go func() {
		if err := s.connectDevice(addr); err != nil {
			s.logger.Error("Failed to connect", "address", addr, "error", err)
			s.publishStatus("idle", err.Error(), "")
			s.mu.Lock()
			s.state = StateIdle
			s.deviceName = ""
			s.mu.Unlock()
		}
	}()

	return nil
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
		for _, entry := range sensors.Registry {
			candidate := entry.Factory()
			if err := candidate.Connect(&device, handler); err != nil {
				s.logger.Info("Sensor connect failed, trying next", "sensor", entry.Name, "error", err)
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
	s.connCtx, s.connCancel = context.WithCancel(context.Background())

	if batErr == nil {
		go s.pollBattery()
	}
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

func (s *Scanner) Disconnect() error {
	s.mu.Lock()
	if s.state == StateIdle {
		s.mu.Unlock()
		return nil
	}
	if s.state != StateConnected || s.device == nil {
		s.mu.Unlock()
		return errState("disconnect", s.state)
	}
	s.disconnecting = true
	device := s.device
	s.resetConnectionState()
	s.mu.Unlock()

	err := device.Disconnect()

	s.mu.Lock()
	s.disconnecting = false
	s.mu.Unlock()

	s.publishStatus("disconnected", "", "user")
	s.logger.Info("Disconnected from BLE device")
	return err
}

// resetConnectionState cancels the connection context, closes the sensor,
// and zeroes all connection-related fields. Caller must hold s.mu.
func (s *Scanner) resetConnectionState() {
	if s.connCancel != nil {
		s.connCancel()
		s.connCancel = nil
	}
	if s.sensor != nil {
		s.sensor.Close()
	}
	s.device = nil
	s.deviceName = ""
	s.sensor = nil
	s.state = StateIdle
	s.resetAutoLock()
}
