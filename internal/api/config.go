package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/pinquake/internal/events"
)

type PinQuakeConfig struct {
	BLE       BLEConfig       `json:"ble" toml:"ble"`
	Waveform  WaveformConfig  `json:"waveform" toml:"waveform"`
	Viz       VizConfig       `json:"viz" toml:"viz"`
}

type BLEConfig struct {
	DeviceName string `json:"device_name" toml:"device_name" example:"PinLevel"`
}

type WaveformConfig struct {
	BufferSize   int     `json:"buffer_size" toml:"buffer_size" example:"256"`
	LogKnee      float64 `json:"log_knee" toml:"log_knee" example:"0.02"`
	ForceYellowG float64 `json:"force_yellow_g" toml:"force_yellow_g" example:"0.03"`
	ForceRedG    float64 `json:"force_red_g" toml:"force_red_g" example:"0.10"`
	AmpScale     float64 `json:"amp_scale" toml:"amp_scale" example:"1.0"`
	SwapXY       bool    `json:"swap_xy" toml:"swap_xy" example:"false"`
}

type VizConfig struct {
	Width  int `json:"width" toml:"width" example:"1920"`
	Height int `json:"height" toml:"height" example:"1080"`
}

type ConfigResponse struct {
	Body PinQuakeConfig
}

type ConfigRequest struct {
	Body PinQuakeConfig
}

func DefaultConfig() PinQuakeConfig {
	return PinQuakeConfig{
		BLE: BLEConfig{
			DeviceName: "PinLevel",
		},
		Waveform: WaveformConfig{
			BufferSize:   256,
			LogKnee:      0.02,
			ForceYellowG: 0.03,
			ForceRedG:    0.10,
			AmpScale:     1.0,
		},
		Viz: VizConfig{
			Width:  608,
			Height: 1080,
		},
	}
}

func (s *Server) registerConfigRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "get-config",
		Method:      http.MethodGet,
		Path:        "/api/config",
		Summary:     "Get configuration",
		Tags:        []string{"config"},
	}, func(_ context.Context, _ *struct{}) (*ConfigResponse, error) {
		cfg, err := s.loadAppConfig()
		if err != nil {
			return &ConfigResponse{Body: DefaultConfig()}, nil
		}
		return &ConfigResponse{Body: cfg}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "update-config",
		Method:      http.MethodPut,
		Path:        "/api/config",
		Summary:     "Update configuration",
		Tags:        []string{"config"},
	}, func(_ context.Context, input *ConfigRequest) (*ConfigResponse, error) {
		if err := s.saveAppConfig(input.Body); err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to save config: %v", err))
		}
		s.scanner.SetSwapXY(input.Body.Waveform.SwapXY)
		s.eventBus.Publish(events.ConfigChangedEvent{
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
		return &ConfigResponse{Body: input.Body}, nil
	})
}

func (s *Server) loadAppConfig() (PinQuakeConfig, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return cfg, err
	}
	var wrapper struct {
		App PinQuakeConfig `toml:"app"`
	}
	if err := toml.Unmarshal(data, &wrapper); err != nil {
		return cfg, err
	}
	merged := cfg
	if wrapper.App.BLE.DeviceName != "" {
		merged.BLE = wrapper.App.BLE
	}
	if wrapper.App.Waveform.BufferSize > 0 {
		merged.Waveform = wrapper.App.Waveform
	}
	if wrapper.App.Viz.Width > 0 {
		merged.Viz = wrapper.App.Viz
	}
	return merged, nil
}

func (s *Server) syncSwapXY() {
	cfg, err := s.loadAppConfig()
	if err != nil {
		return
	}
	s.scanner.SetSwapXY(cfg.Waveform.SwapXY)
}

func (s *Server) saveAppConfig(cfg PinQuakeConfig) error {
	// Read existing file to preserve non-app sections
	existing := make(map[string]any)
	if data, err := os.ReadFile(s.configPath); err == nil {
		_ = toml.Unmarshal(data, &existing)
	}
	existing["app"] = cfg
	data, err := toml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath, data, 0644)
}
