package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromPathMissingFile(t *testing.T) {
	cfg, err := LoadFromPath("/nonexistent/config.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	defaults := DefaultConfig()
	if cfg.Display.FadeS != defaults.Display.FadeS {
		t.Errorf("expected default FadeS=%v, got %v", defaults.Display.FadeS, cfg.Display.FadeS)
	}
	if cfg.Waveform.BufferSize != defaults.Waveform.BufferSize {
		t.Errorf("expected default BufferSize=%d, got %d", defaults.Waveform.BufferSize, cfg.Waveform.BufferSize)
	}
}

func TestLoadFromPathPartialTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`[app.display]
trigger_g = 0.05
fade_s = 10.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Display.TriggerG != 0.05 {
		t.Errorf("TriggerG: want 0.05, got %v", cfg.Display.TriggerG)
	}
	if cfg.Display.FadeS != 10.0 {
		t.Errorf("FadeS: want 10.0, got %v", cfg.Display.FadeS)
	}
	// Unset display field should keep default
	if cfg.Display.DelayMs != 0 {
		t.Errorf("DelayMs: want 0 (default), got %d", cfg.Display.DelayMs)
	}
	// Other sections should keep defaults
	defaults := DefaultConfig()
	if cfg.Waveform.BufferSize != defaults.Waveform.BufferSize {
		t.Errorf("Waveform.BufferSize: want %d (default), got %d", defaults.Waveform.BufferSize, cfg.Waveform.BufferSize)
	}
	if cfg.Crosshair.Decay != defaults.Crosshair.Decay {
		t.Errorf("Crosshair.Decay: want %v (default), got %v", defaults.Crosshair.Decay, cfg.Crosshair.Decay)
	}
}

func TestLoadFromPathWithSensor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`[app.display]
trigger_g = 0.02
fade_s = 5.0

[app.sensor.WT901]
accel_range_g = 4.0
six_axis = true
output_rate_hz = 200
`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sensor == nil {
		t.Fatal("Sensor map should not be nil")
	}
	wt, ok := cfg.Sensor["WT901"]
	if !ok {
		t.Fatal("WT901 sensor config should exist")
	}
	m, ok := wt.(map[string]any)
	if !ok {
		t.Fatalf("WT901 sensor config should be map[string]any, got %T", wt)
	}
	if v, _ := m["accel_range_g"].(float64); v != 4.0 {
		t.Errorf("accel_range_g: want 4.0, got %v", v)
	}
	if v, _ := m["six_axis"].(bool); !v {
		t.Error("six_axis: want true")
	}
}

func TestLoadFromPathAlwaysVisible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`[app.display]
trigger_g = 0.02
fade_s = -1.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Display.FadeS != -1 {
		t.Errorf("FadeS: want -1, got %v", cfg.Display.FadeS)
	}
}

func TestLoadFromPathPreservesEmbeddedDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`[app.waveform]
buffer_size = 512
`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Waveform.BufferSize != 512 {
		t.Errorf("BufferSize: want 512, got %d", cfg.Waveform.BufferSize)
	}
	defaults := DefaultConfig()
	if cfg.Waveform.Enabled != defaults.Waveform.Enabled {
		t.Errorf("Enabled: want %v (default), got %v", defaults.Waveform.Enabled, cfg.Waveform.Enabled)
	}
	if cfg.Waveform.Width != defaults.Waveform.Width {
		t.Errorf("Width: want %d (default), got %d", defaults.Waveform.Width, cfg.Waveform.Width)
	}
	if cfg.Waveform.LogKnee != defaults.Waveform.LogKnee {
		t.Errorf("LogKnee: want %v (default), got %v", defaults.Waveform.LogKnee, cfg.Waveform.LogKnee)
	}
}

func TestSaveSectionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	display := DisplayConfig{
		DelayMs:  100,
		TriggerG: 0.05,
		FadeS:    -1,
	}
	if err := SaveSection(path, "display", display); err != nil {
		t.Fatalf("SaveSection: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if cfg.Display.DelayMs != 100 {
		t.Errorf("DelayMs: want 100, got %d", cfg.Display.DelayMs)
	}
	if cfg.Display.TriggerG != 0.05 {
		t.Errorf("TriggerG: want 0.05, got %v", cfg.Display.TriggerG)
	}
	if cfg.Display.FadeS != -1 {
		t.Errorf("FadeS: want -1, got %v", cfg.Display.FadeS)
	}
}

func TestSaveSensorSectionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	sensor := map[string]any{
		"accel_range_g":  4.0,
		"output_rate_hz": 200.0,
		"six_axis":       true,
	}
	if err := SaveSensorSection(path, "WT901", sensor); err != nil {
		t.Fatalf("SaveSensorSection: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if cfg.Sensor == nil {
		t.Fatal("Sensor map should not be nil")
	}
	wt, ok := cfg.Sensor["WT901"].(map[string]any)
	if !ok {
		t.Fatalf("WT901 should be map[string]any, got %T", cfg.Sensor["WT901"])
	}
	if v, _ := wt["accel_range_g"].(float64); v != 4.0 {
		t.Errorf("accel_range_g: want 4.0, got %v", v)
	}
}

func TestSaveAllRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.Display.FadeS = -1
	cfg.Display.TriggerG = 0.044

	if err := SaveAll(path, cfg); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	loaded, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if loaded.Display.FadeS != -1 {
		t.Errorf("FadeS: want -1, got %v", loaded.Display.FadeS)
	}
	if loaded.Display.TriggerG != 0.044 {
		t.Errorf("TriggerG: want 0.044, got %v", loaded.Display.TriggerG)
	}
}
