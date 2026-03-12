package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
)

type ConnectRequest struct {
	Body struct {
		Address string `json:"address" doc:"BLE MAC address" example:"AA:BB:CC:DD:EE:FF"`
		Name    string `json:"name" doc:"BLE advertised name" example:"WT901BLE50"`
	}
}

type OKBody struct {
	OK bool `json:"ok"`
}

type BLEActionResponse struct {
	Body OKBody
}

type FrameStateBody struct {
	Locked bool `json:"locked"`
}

type FrameStateResponse struct {
	Body FrameStateBody
}

var bleOK = &BLEActionResponse{Body: OKBody{OK: true}}

func (s *Server) registerBLERoutes() {
	bleGrp := huma.NewGroup(s.api, "/api/ble")
	bleGrp.UseSimpleModifier(huma.OperationTags("ble"))

	sse.Register(bleGrp, huma.Operation{
		OperationID: "ble-scan",
		Method:      http.MethodGet,
		Path:        "/scan",
		Summary:     "BLE scan SSE stream",
		Description: "Opening this connection starts scanning; closing it stops scanning",
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

	huma.Post(bleGrp, "/connect", func(_ context.Context, input *ConnectRequest) (*BLEActionResponse, error) {
		if err := s.scanner.Connect(input.Body.Address, input.Body.Name); err != nil {
			return nil, huma.Error409Conflict(fmt.Sprintf("cannot connect: %v", err))
		}
		s.updateBLEDevice(input.Body.Address, input.Body.Name)
		return bleOK, nil
	})

	huma.Post(bleGrp, "/disconnect", func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		if err := s.scanner.Disconnect(); err != nil {
			return nil, huma.Error409Conflict(fmt.Sprintf("cannot disconnect: %v", err))
		}
		s.updateBLEDevice("", "")
		return bleOK, nil
	})

	huma.Post(bleGrp, "/frame/lock", func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		s.scanner.LockFrame()
		return bleOK, nil
	})

	huma.Post(bleGrp, "/frame/unlock", func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		s.scanner.UnlockFrame()
		return bleOK, nil
	})

	huma.Post(bleGrp, "/frame/force-lock", func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		s.scanner.ForceLockFrame()
		return bleOK, nil
	})

	huma.Get(bleGrp, "/frame", func(_ context.Context, _ *struct{}) (*FrameStateResponse, error) {
		return &FrameStateResponse{Body: FrameStateBody{Locked: s.scanner.IsFrameLocked()}}, nil
	})
}

func (s *Server) updateBLEDevice(addr, name string) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	cfg, _ := s.loadAppConfig()
	cfg.BLE.DeviceAddress = addr
	cfg.BLE.DeviceName = name
	if addr == "" {
		cfg.BLE.SensorName = ""
	}
	if err := data.SaveAll(s.configPath, cfg); err != nil {
		slog.Error("Failed to save BLE device config", "error", err)
	}
}
