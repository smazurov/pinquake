package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/smazurov/pinquake/internal/ble"
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

// BLEScanResultEvent carries a single BLE scan advertisement.
type BLEScanResultEvent struct {
	Address    string `json:"address"`
	Name       string `json:"name"`
	RSSI       int    `json:"rssi"`
	SensorName string `json:"sensor_name,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// BLEScanStateEvent reports the scan behind a scan stream.
type BLEScanStateEvent struct {
	State     string  `json:"state" enum:"scanning,waiting,ended" doc:"waiting: the scan failed and is retried after a backoff; ended: the stream closes"`
	Reason    string  `json:"reason,omitempty" enum:"device-chosen,timeout" doc:"Why the scan ended"`
	Error     string  `json:"error,omitempty" doc:"Why the scan failed (waiting)"`
	RetryInS  float64 `json:"retry_in_s,omitempty" doc:"Seconds until the scan is retried (waiting)"`
	Timestamp string  `json:"timestamp"`
}

func scanStateEvent(st ble.ScanStatus) BLEScanStateEvent {
	ev := BLEScanStateEvent{State: string(st.State), Timestamp: time.Now().Format(time.RFC3339Nano)}
	switch st.State {
	case ble.ScanWaiting:
		ev.Error = st.Err.Error()
		ev.RetryInS = st.RetryIn.Seconds()
	case ble.ScanEnded:
		ev.Reason = "device-chosen"
	}
	return ev
}

// offerLatest puts st in ch (cap 1), replacing a status not yet relayed,
// so the supervisor never blocks on a stalled stream. Only the supervisor
// loop sends.
func offerLatest(ch chan ble.ScanStatus, st ble.ScanStatus) {
	select {
	case <-ch:
	default:
	}
	ch <- st
}

// scanTimeout ends a scan stream, so a list left open doesn't keep the
// radio scanning.
const scanTimeout = time.Minute

// streamScan relays one browse to a scan stream.
func (s *Server) streamScan(ctx context.Context, _ *struct{}, send sse.Sender) {
	ctx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()
	advs := make(chan ble.Advertisement, 64)
	statuses := make(chan ble.ScanStatus, 1)
	s.scanner.Browse(ctx, ble.Browser{
		Found: func(a ble.Advertisement) {
			select { // never block the supervisor; the list catches up
			case advs <- a:
			default:
			}
		},
		Status: func(st ble.ScanStatus) { offerLatest(statuses, st) },
	})
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := send.Data(events.HeartbeatEvent{Timestamp: time.Now().Format(time.RFC3339Nano)}); err != nil {
				return
			}
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				_ = send.Data(BLEScanStateEvent{State: string(ble.ScanEnded), Reason: "timeout", Timestamp: time.Now().Format(time.RFC3339Nano)})
			}
			return
		case a := <-advs:
			if err := send.Data(BLEScanResultEvent{
				Address:    a.Address,
				Name:       a.Name,
				RSSI:       a.RSSI,
				SensorName: a.SensorName,
				Timestamp:  time.Now().Format(time.RFC3339Nano),
			}); err != nil {
				return
			}
		case st := <-statuses:
			if err := send.Data(scanStateEvent(st)); err != nil || st.State == ble.ScanEnded {
				return
			}
		}
	}
}

func (s *Server) registerBLERoutes() {
	bleGrp := huma.NewGroup(s.api, "/api/ble")
	bleGrp.UseSimpleModifier(huma.OperationTags("ble"))

	sse.Register(bleGrp, huma.Operation{
		OperationID: "ble-scan",
		Method:      http.MethodGet,
		Path:        "/scan",
		Summary:     "BLE scan SSE stream",
		Description: "Scans for nearby devices while no device is chosen. Opening this connection starts scanning; closing it stops. Ends with a scan-state 'ended' event once a device is chosen.",
	}, map[string]any{
		"device":     BLEScanResultEvent{},
		"scan-state": BLEScanStateEvent{},
		"heartbeat":  events.HeartbeatEvent{},
	}, s.streamScan)

	huma.Post(bleGrp, "/connect", func(_ context.Context, input *ConnectRequest) (*BLEActionResponse, error) {
		// Detect the sensor type on connect; a factory left over from the
		// previously saved device may not match this one.
		s.scanner.SetSensorFactory(nil)
		if err := s.scanner.Connect(input.Body.Address, input.Body.Name); err != nil {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("cannot connect: %v", err))
		}
		s.updateBLEDevice(input.Body.Address, input.Body.Name)
		return bleOK, nil
	})

	huma.Post(bleGrp, "/disconnect", func(_ context.Context, _ *struct{}) (*BLEActionResponse, error) {
		s.scanner.Disconnect()
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
