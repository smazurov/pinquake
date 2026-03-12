package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/smazurov/pinquake/internal/data"
)

func TestDefaultConfigHasValidCrosshairDefaults(t *testing.T) {
	cfg := data.DefaultConfig()

	if cfg.Crosshair.ForceYellowG <= 0 {
		t.Error("CrosshairConfig.ForceYellowG should be positive")
	}
	if cfg.Crosshair.ForceRedG <= 0 {
		t.Error("CrosshairConfig.ForceRedG should be positive")
	}
	if cfg.Crosshair.ForceYellowG >= cfg.Crosshair.ForceRedG {
		t.Error("ForceYellowG should be less than ForceRedG")
	}
	if cfg.Crosshair.Decay < 0 || cfg.Crosshair.Decay > 2 {
		t.Errorf("Decay should be in [0,2], got %f", cfg.Crosshair.Decay)
	}
	if cfg.Crosshair.SegmentSize < 2 {
		t.Errorf("SegmentSize should be >= 2, got %d", cfg.Crosshair.SegmentSize)
	}
	if cfg.Crosshair.BarThickness < 4 {
		t.Errorf("BarThickness should be >= 4, got %d", cfg.Crosshair.BarThickness)
	}
}

func TestDefaultConfigHasValidWaveformDefaults(t *testing.T) {
	cfg := data.DefaultConfig()

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
	cfg := data.DefaultConfig()

	cfg.Waveform.ForceYellowG = 0.05
	cfg.Waveform.ForceRedG = 0.2

	if cfg.Crosshair.ForceYellowG == cfg.Waveform.ForceYellowG {
		t.Error("Crosshair should have independent ForceYellowG after waveform modification")
	}
}

func TestDefaultConfigHasPerVizDimensions(t *testing.T) {
	cfg := data.DefaultConfig()

	if cfg.Waveform.Width != 608 || cfg.Waveform.Height != 1080 {
		t.Errorf("Waveform dimensions: want 608x1080, got %dx%d", cfg.Waveform.Width, cfg.Waveform.Height)
	}
	if cfg.Crosshair.Width != 200 || cfg.Crosshair.Height != 200 {
		t.Errorf("Crosshair dimensions: want 200x200, got %dx%d", cfg.Crosshair.Width, cfg.Crosshair.Height)
	}
}

func TestDefaultConfigHasValidDisplayDefaults(t *testing.T) {
	cfg := data.DefaultConfig()

	if cfg.Display.DelayMs < 0 {
		t.Error("DelayMs should be >= 0")
	}
	if cfg.Display.TriggerG <= 0 {
		t.Error("TriggerG should be positive")
	}
	if cfg.Display.FadeS <= 0 {
		t.Error("FadeS should be positive")
	}
}

func TestPutWaveformConfigAcceptsValidFloats(t *testing.T) {
	_, api := humatest.New(t)
	huma.Put(api, "/api/config/waveform", func(_ context.Context, input *sectionRequest[data.WaveformConfig]) (*sectionResponse[data.WaveformConfig], error) {
		return &sectionResponse[data.WaveformConfig]{Body: input.Body}, nil
	})

	body := `{
		"enabled": true,
		"width": 608,
		"height": 1080,
		"buffer_size": 256,
		"log_knee": 0.02,
		"force_yellow_g": 0.03,
		"force_red_g": 0.36,
		"amp_scale": 1.0,
		"swap_xy": false
	}`

	resp := api.Put("/api/config/waveform", strings.NewReader(body))

	if resp.Code != http.StatusOK {
		t.Errorf("PUT /api/config/waveform with force_red_g=0.36: want 200, got %d\nBody: %s",
			resp.Code, resp.Body.String())
	}
}

func TestPutDisplayConfigRejectsInvalidFadeS(t *testing.T) {
	_, api := humatest.New(t)
	huma.Put(api, "/api/config/display", func(_ context.Context, input *sectionRequest[data.DisplayConfig]) (*sectionResponse[data.DisplayConfig], error) {
		return &sectionResponse[data.DisplayConfig]{Body: input.Body}, nil
	})

	body := `{"delay_ms": 0, "trigger_g": 0.02, "fade_s": 0.5}`
	resp := api.Put("/api/config/display", strings.NewReader(body))

	if resp.Code != http.StatusUnprocessableEntity {
		t.Errorf("PUT with fade_s=0.5: want 422, got %d\nBody: %s", resp.Code, resp.Body.String())
	}
}

func TestConfigRoundTrip(t *testing.T) {
	cfg := data.DefaultConfig()
	cfg.Crosshair.Decay = 0.85
	cfg.Crosshair.SegmentSize = 15
	cfg.Crosshair.BarThickness = 20

	if cfg.Crosshair.Decay != 0.85 {
		t.Errorf("expected Decay 0.85, got %f", cfg.Crosshair.Decay)
	}
	if cfg.Crosshair.SegmentSize != 15 {
		t.Errorf("expected SegmentSize 15, got %d", cfg.Crosshair.SegmentSize)
	}
	if cfg.Crosshair.BarThickness != 20 {
		t.Errorf("expected BarThickness 20, got %d", cfg.Crosshair.BarThickness)
	}
}
