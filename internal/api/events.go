package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/pinquake/internal/events"
)

func (s *Server) registerSSERoutes() {
	sse.Register(s.api, huma.Operation{
		OperationID: "events-stream",
		Method:      http.MethodGet,
		Path:        "/api/events",
		Summary:     "SSE event stream",
		Description: "Real-time orientation data and BLE status events",
		Tags:        []string{"events"},
	}, map[string]any{
		"orientation":    events.OrientationEvent{},
		"ble-status":     events.BLEStatusEvent{},
		"config-changed": events.ConfigChangedEvent{},
	}, func(ctx context.Context, _ *struct{}, send sse.Sender) {
		eventCh := make(chan any, 64)

		unsubscribers := []func(){
			events.SubscribeToChannel[events.OrientationEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.BLEStatusEvent](s.eventBus, eventCh),
			events.SubscribeToChannel[events.ConfigChangedEvent](s.eventBus, eventCh),
		}
		defer func() {
			for _, unsub := range unsubscribers {
				unsub()
			}
		}()

		if err := send.Data(events.BLEStatusEvent{
			Status:    string(s.scanner.GetState()),
			Timestamp: time.Now().Format(time.RFC3339Nano),
		}); err != nil {
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventCh:
				if err := send.Data(event); err != nil {
					return
				}
			}
		}
	})
}
