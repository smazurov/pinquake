package api

import (
	"testing"
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
