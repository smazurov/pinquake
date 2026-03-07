package ble

import (
	"context"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/sensors"
	"tinygo.org/x/bluetooth"
)

func (s *Scanner) Scan(ctx context.Context) error {
	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return errState("scan", s.state)
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
				if entry := sensors.Match(result); entry != nil {
					sensorName = entry.Name
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
