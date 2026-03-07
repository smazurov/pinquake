package api

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/pinquake/internal/events"
)

type PinQuakeConfig struct {
	BLE       BLEConfig       `json:"ble" toml:"ble"`
	Waveform  WaveformConfig  `json:"waveform" toml:"waveform"`
	Crosshair CrosshairConfig `json:"crosshair" toml:"crosshair"`
	Viz       VizConfig       `json:"viz" toml:"viz"`
	AutoLock  AutoLockConfig  `json:"auto_lock" toml:"auto_lock"`
}

type BLEConfig struct {
	DeviceAddress string `json:"device_address" toml:"device_address"`
	DeviceName    string `json:"device_name" toml:"device_name"`
	SensorName    string `json:"sensor_name" toml:"sensor_name"`
}

type WaveformConfig struct {
	BufferSize   int     `json:"buffer_size" toml:"buffer_size" doc:"Ring buffer sample count" minimum:"32" maximum:"512" default:"256"`
	LogKnee      float64 `json:"log_knee" toml:"log_knee" doc:"Log compression knee" minimum:"0.001" maximum:"0.1" multipleOf:"0.001" default:"0.02"`
	ForceYellowG float64 `json:"force_yellow_g" toml:"force_yellow_g" doc:"Yellow threshold (g)" minimum:"0.001" maximum:"0.5" multipleOf:"0.001" default:"0.03"`
	ForceRedG    float64 `json:"force_red_g" toml:"force_red_g" doc:"Red threshold (g)" minimum:"0.01" maximum:"1.0" multipleOf:"0.01" default:"0.1"`
	AmpScale     float64 `json:"amp_scale" toml:"amp_scale" doc:"Amplitude multiplier" minimum:"0.1" maximum:"5.0" multipleOf:"0.1" default:"1.0"`
	SwapXY       bool    `json:"swap_xy" toml:"swap_xy" doc:"Swap X and Y axes" default:"false"`
}

type CrosshairConfig struct {
	ForceYellowG float64 `json:"force_yellow_g" toml:"force_yellow_g" doc:"Yellow threshold (g)" minimum:"0.001" maximum:"0.5" multipleOf:"0.001" default:"0.03"`
	ForceRedG    float64 `json:"force_red_g" toml:"force_red_g" doc:"Red threshold (g)" minimum:"0.01" maximum:"1.0" multipleOf:"0.01" default:"0.1"`
	Smoothing    float64 `json:"smoothing" toml:"smoothing" doc:"Exponential smoothing factor" minimum:"0" maximum:"1" multipleOf:"0.01" default:"0.7"`
	SegmentSize  int     `json:"segment_size" toml:"segment_size" doc:"Bar segment size (px)" minimum:"2" maximum:"30" default:"10"`
	BarThickness int     `json:"bar_thickness" toml:"bar_thickness" doc:"Bar thickness (px)" minimum:"4" maximum:"30" default:"12"`
	SwapXY       bool    `json:"swap_xy" toml:"swap_xy" doc:"Swap X and Y axes" default:"false"`
}

type AutoLockConfig struct {
	Timeout float64 `json:"timeout" toml:"timeout" doc:"Seconds of stability before auto-lock" minimum:"1" maximum:"60" default:"10"`
	Epsilon float64 `json:"epsilon" toml:"epsilon" doc:"Max change (g) to count as stable" minimum:"0.001" maximum:"1.0" multipleOf:"0.001" default:"0.01"`
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
		BLE: BLEConfig{},
		Waveform: WaveformConfig{
			BufferSize:   256,
			LogKnee:      0.02,
			ForceYellowG: 0.03,
			ForceRedG:    0.10,
			AmpScale:     1.0,
		},
		Crosshair: CrosshairConfig{
			ForceYellowG: 0.03,
			ForceRedG:    0.10,
			Smoothing:    0.7,
			SegmentSize:  10,
			BarThickness: 12,
		},
		Viz: VizConfig{
			Width:  608,
			Height: 1080,
		},
		AutoLock: AutoLockConfig{
			Timeout: 10,
			Epsilon: 0.01,
		},
	}
}

func (s *Server) registerConfigRoutes() {
	huma.Get(s.api, "/api/config", func(_ context.Context, _ *struct{}) (*ConfigResponse, error) {
		cfg, err := s.loadAppConfig()
		if err != nil {
			return &ConfigResponse{Body: DefaultConfig()}, nil
		}
		return &ConfigResponse{Body: cfg}, nil
	}, huma.OperationTags("config"))

	huma.Put(s.api, "/api/config", func(_ context.Context, input *ConfigRequest) (*ConfigResponse, error) {
		s.configMu.Lock()
		err := s.saveAppConfig(input.Body)
		s.configMu.Unlock()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to save config: %v", err))
		}
		s.syncConfig(input.Body)
		s.eventBus.Publish(events.ConfigChangedEvent{
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
		return &ConfigResponse{Body: input.Body}, nil
	}, huma.OperationTags("config"))
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
	if wrapper.App.BLE.DeviceAddress != "" {
		merged.BLE = wrapper.App.BLE
	}
	if wrapper.App.Waveform.BufferSize > 0 {
		merged.Waveform = wrapper.App.Waveform
	}
	if wrapper.App.Crosshair.ForceRedG > 0 {
		merged.Crosshair = wrapper.App.Crosshair
	}
	if wrapper.App.Viz.Width > 0 {
		merged.Viz = wrapper.App.Viz
	}
	if wrapper.App.AutoLock.Timeout > 0 {
		merged.AutoLock = wrapper.App.AutoLock
	}
	return merged, nil
}

func (s *Server) saveAppConfig(cfg PinQuakeConfig) error {
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
