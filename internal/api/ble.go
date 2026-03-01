package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/events"
)

type ConnectRequest struct {
	Body struct {
		Address string `json:"address" doc:"BLE MAC address" example:"AA:BB:CC:DD:EE:FF"`
	}
}

type BLEStateResponse struct {
	Body struct {
		State ble.State `json:"state" example:"idle"`
	}
}

type BLEActionResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

type FrameStateResponse struct {
	Body struct {
		Locked bool `json:"locked"`
	}
}

func (s *Server) registerBLERoutes() {
	sse.Register(s.api, huma.Operation{
		OperationID: "ble-scan",
		Method:      http.MethodGet,
		Path:        "/api/ble/scan",
		Summary:     "BLE scan SSE stream",
		Description: "Opening this connection starts scanning; closing it stops scanning",
		Tags:        []string{"ble"},
	}, map[string]any{
		"device": events.BLEScanResultEvent{},
	}, func(ctx context.Context, _ *struct{}, send sse.Sender) {
		if err := s.scanner.Scan(ctx); err != nil {
			send.Data(struct {
				Error string `json:"error"`
			}{Error: err.Error()})
			return
		}

		ch := make(chan any, 64)
		unsub := events.SubscribeToChannel[events.BLEScanResultEvent](s.eventBus, ch)
		defer unsub()

		for {
			select {
			case <-ctx.Done():
				return
			case event := <-ch:
				if err := send.Data(event); err != nil {
					return
				}
			}
		}
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "ble-connect",
		Method:      http.MethodPost,
		Path:        "/api/ble/connect",
		Summary:     "Connect to BLE device",
		Tags:        []string{"ble"},
	}, func(_ context.Context, input *ConnectRequest) (*BLEActionResponse, error) {
		if err := s.scanner.Connect(input.Body.Address); err != nil {
			return nil, huma.Error409Conflict(fmt.Sprintf("cannot connect: %v", err))
		}
		return &BLEActionResponse{Body: struct {
			OK bool `json:"ok"`
		}{OK: true}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "ble-disconnect",
		Method:      http.MethodPost,
		Path:        "/api/ble/disconnect",
		Summary:     "Disconnect from BLE device",
		Tags:        []string{"ble"},
	}, func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		if err := s.scanner.Disconnect(); err != nil {
			return nil, huma.Error409Conflict(fmt.Sprintf("cannot disconnect: %v", err))
		}
		return &BLEActionResponse{Body: struct {
			OK bool `json:"ok"`
		}{OK: true}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "ble-state",
		Method:      http.MethodGet,
		Path:        "/api/ble/state",
		Summary:     "Get BLE state",
		Tags:        []string{"ble"},
	}, func(_ context.Context, _ *struct{}) (*BLEStateResponse, error) {
		resp := &BLEStateResponse{}
		resp.Body.State = s.scanner.GetState()
		return resp, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "ble-frame-lock",
		Method:      http.MethodPost,
		Path:        "/api/ble/frame/lock",
		Summary:     "Lock reference frame",
		Tags:        []string{"ble"},
	}, func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		s.scanner.LockFrame()
		return &BLEActionResponse{Body: struct {
			OK bool `json:"ok"`
		}{OK: true}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "ble-frame-unlock",
		Method:      http.MethodPost,
		Path:        "/api/ble/frame/unlock",
		Summary:     "Unlock reference frame",
		Tags:        []string{"ble"},
	}, func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		s.scanner.UnlockFrame()
		return &BLEActionResponse{Body: struct {
			OK bool `json:"ok"`
		}{OK: true}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "ble-frame-state",
		Method:      http.MethodGet,
		Path:        "/api/ble/frame",
		Summary:     "Get reference frame lock state",
		Tags:        []string{"ble"},
	}, func(_ context.Context, _ *struct{}) (*FrameStateResponse, error) {
		return &FrameStateResponse{Body: struct {
			Locked bool `json:"locked"`
		}{Locked: s.scanner.IsFrameLocked()}}, nil
	})
}
