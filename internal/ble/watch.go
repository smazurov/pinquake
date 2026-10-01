package ble

import (
	"context"
	"time"

	"github.com/smazurov/pinquake/internal/events"
)

func (s *Scanner) connContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connCtx
}

func (s *Scanner) pollBattery() {
	ctx := s.connContext()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.readAndPublishBattery()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.readAndPublishBattery()
		}
	}
}

func (s *Scanner) readAndPublishBattery() {
	sen := s.Sensor()
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
