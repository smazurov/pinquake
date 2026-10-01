package ble

import (
	"context"
	"time"

	"github.com/smazurov/pinquake/internal/events"
)

// Scan publishes every advertisement as a BLEScanResultEvent until ctx is
// done. It shares the supervisor's scan, so it works while searching for
// the saved device.
func (s *Scanner) Scan(ctx context.Context) {
	s.sup.Browse(ctx, func(adv Advertisement) {
		s.eventBus.Publish(events.BLEScanResultEvent{
			Address:    adv.Address,
			Name:       adv.Name,
			RSSI:       adv.RSSI,
			SensorName: adv.SensorName,
			Timestamp:  time.Now().Format(time.RFC3339Nano),
		})
	})
}
