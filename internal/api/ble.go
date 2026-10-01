package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/framelock"
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

type FrameActionRequest struct {
	Body struct {
		Action string `json:"action" doc:"Action to perform" enum:"enable,disable,trigger" example:"enable"`
	}
}

type FrameStateBody struct {
	Enabled  bool    `json:"enabled" doc:"Auto-lock enabled"`
	State    string  `json:"state" enum:"unlocked,settling,locked"`
	Stdev    float32 `json:"stdev" doc:"Max per-axis stdev over buffered samples (g)"`
	LockedAt string  `json:"locked_at,omitempty" doc:"When the current frame was locked"`
}

func frameStateBody(st framelock.Status) FrameStateBody {
	body := FrameStateBody{Enabled: st.Enabled, State: string(st.State), Stdev: st.Stdev}
	if !st.LockedAt.IsZero() {
		body.LockedAt = st.LockedAt.Format(time.RFC3339Nano)
	}
	return body
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

	huma.Post(bleGrp, "/frame", func(_ context.Context, input *FrameActionRequest) (*FrameStateResponse, error) {
		logMessages := map[string]string{
			"enable":  "Auto-lock enabled",
			"disable": "Auto-lock disabled",
			"trigger": "Force-lock requested",
		}
		action := input.Body.Action
		msg, ok := logMessages[action]
		if !ok {
			return nil, huma.Error422UnprocessableEntity("invalid action: must be enable, disable, or trigger")
		}
		// Hold configMu so a concurrent config PUT can't sync a stale
		// frame.auto_lock between applying and persisting the action.
		s.configMu.Lock()
		st := s.scanner.FrameAction(action)
		if action == "enable" || action == "disable" {
			if err := s.saveFrameAutoLock(action == "enable"); err != nil {
				slog.Error("Failed to save frame auto-lock", "error", err)
			}
		}
		s.configMu.Unlock()
		s.log("info", msg)
		return &FrameStateResponse{Body: frameStateBody(st)}, nil
	})

	huma.Get(bleGrp, "/frame", func(_ context.Context, _ *struct{}) (*FrameStateResponse, error) {
		return &FrameStateResponse{Body: frameStateBody(s.scanner.FrameStatus())}, nil
	})
}

// saveFrameAutoLock persists frame.auto_lock. Caller must hold configMu.
func (s *Server) saveFrameAutoLock(enabled bool) error {
	cfg, _ := s.loadAppConfig()
	cfg.Frame.AutoLock = enabled
	return data.SaveAll(s.configPath, cfg)
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
