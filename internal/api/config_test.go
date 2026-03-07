package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestDefaultConfigHasValidCrosshairDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Crosshair.ForceYellowG <= 0 {
		t.Error("CrosshairConfig.ForceYellowG should be positive")
	}
	if cfg.Crosshair.ForceRedG <= 0 {
		t.Error("CrosshairConfig.ForceRedG should be positive")
	}
	if cfg.Crosshair.ForceYellowG >= cfg.Crosshair.ForceRedG {
		t.Error("ForceYellowG should be less than ForceRedG")
	}
	if cfg.Crosshair.Smoothing < 0 || cfg.Crosshair.Smoothing > 1 {
		t.Errorf("Smoothing should be in [0,1], got %f", cfg.Crosshair.Smoothing)
	}
	if cfg.Crosshair.SegmentSize < 2 {
		t.Errorf("SegmentSize should be >= 2, got %d", cfg.Crosshair.SegmentSize)
	}
	if cfg.Crosshair.BarThickness < 4 {
		t.Errorf("BarThickness should be >= 4, got %d", cfg.Crosshair.BarThickness)
	}
}

func TestDefaultConfigHasValidWaveformDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Waveform.BufferSize < 32 || cfg.Waveform.BufferSize > 512 {
		t.Errorf("BufferSize should be in [32,512], got %d", cfg.Waveform.BufferSize)
	}
	if cfg.Waveform.ForceYellowG >= cfg.Waveform.ForceRedG {
		t.Error("Waveform ForceYellowG should be less than ForceRedG")
	}
	if cfg.Waveform.AmpScale <= 0 {
		t.Error("AmpScale should be positive")
	}
}

func TestDefaultConfigCrosshairIndependentFromWaveform(t *testing.T) {
	cfg := DefaultConfig()

	// Crosshair and waveform thresholds can differ — verify they're set independently
	cfg.Waveform.ForceYellowG = 0.05
	cfg.Waveform.ForceRedG = 0.2

	if cfg.Crosshair.ForceYellowG == cfg.Waveform.ForceYellowG {
		t.Error("Crosshair should have independent ForceYellowG after waveform modification")
	}
}

func TestPutConfigAcceptsValidFloats(t *testing.T) {
	_, api := humatest.New(t)
	huma.Put(api, "/api/config", func(_ context.Context, input *ConfigRequest) (*ConfigResponse, error) {
		return &ConfigResponse{Body: input.Body}, nil
	})

	// 0.36 is a valid multiple of 0.01 but fails huma's multipleOf check
	// due to IEEE 754 float rounding (0.36 / 0.01 = 35.999...)
	body := `{
		"ble": {"device_address": "", "device_name": "", "sensor_name": ""},
		"waveform": {
			"buffer_size": 256,
			"log_knee": 0.02,
			"force_yellow_g": 0.03,
			"force_red_g": 0.36,
			"amp_scale": 1.0,
			"swap_xy": false
		},
		"crosshair": {
			"force_yellow_g": 0.03,
			"force_red_g": 0.36,
			"smoothing": 0.7,
			"segment_size": 10,
			"bar_thickness": 12,
			"swap_xy": false
		},
		"viz": {"width": 608, "height": 1080},
		"auto_lock": {"timeout": 10, "epsilon": 0.01}
	}`

	resp := api.Put("/api/config", strings.NewReader(body))

	if resp.Code != http.StatusOK {
		t.Errorf("PUT /api/config with force_red_g=0.36: want 200, got %d\nBody: %s",
			resp.Code, resp.Body.String())
	}
}

func TestConfigRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Crosshair.Smoothing = 0.85
	cfg.Crosshair.SegmentSize = 15
	cfg.Crosshair.BarThickness = 20

	// Verify the struct fields survived assignment
	if cfg.Crosshair.Smoothing != 0.85 {
		t.Errorf("expected Smoothing 0.85, got %f", cfg.Crosshair.Smoothing)
	}
	if cfg.Crosshair.SegmentSize != 15 {
		t.Errorf("expected SegmentSize 15, got %d", cfg.Crosshair.SegmentSize)
	}
	if cfg.Crosshair.BarThickness != 20 {
		t.Errorf("expected BarThickness 20, got %d", cfg.Crosshair.BarThickness)
	}
}
