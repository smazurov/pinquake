package data

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pelletier/go-toml/v2"
)

// FadeSeconds represents a fade duration in seconds.
// Valid values: exactly -1 (always visible) or [1, 30] in 0.1 increments.
type FadeSeconds float64

func (f FadeSeconds) Schema(_ huma.Registry) *huma.Schema {
	return &huma.Schema{
		OneOf: []*huma.Schema{
			{
				Type:        "number",
				Description: "Always visible",
				Enum:        []any{float64(-1)},
			},
			{
				Type:       "number",
				Minimum:    ptr(float64(1)),
				Maximum:    ptr(float64(30)),
				MultipleOf: ptr(0.1),
			},
		},
		Default: float64(5),
	}
}

func (f FadeSeconds) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(f))
}

func (f *FadeSeconds) UnmarshalJSON(b []byte) error {
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = FadeSeconds(v)
	return nil
}

func (f *FadeSeconds) UnmarshalText(b []byte) error {
	v, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	*f = FadeSeconds(v)
	return nil
}

func ptr[T any](v T) *T { return &v }

type VizBase struct {
	Enabled bool `json:"enabled" toml:"enabled" doc:"Enable visualization" default:"true"`
	Width   int  `json:"width" toml:"width" doc:"Canvas width (px)" minimum:"50" maximum:"7680" default:"608"`
	Height  int  `json:"height" toml:"height" doc:"Canvas height (px)" minimum:"50" maximum:"4320" default:"1080"`
}

type ForceThresholds struct {
	ForceYellowG float64 `json:"force_yellow_g" toml:"force_yellow_g" doc:"Yellow threshold (g)" minimum:"0.001" maximum:"0.5" multipleOf:"0.001" default:"0.03"`
	ForceRedG    float64 `json:"force_red_g" toml:"force_red_g" doc:"Red threshold (g)" minimum:"0.01" maximum:"1.0" multipleOf:"0.01" default:"0.1"`
}

type DisplayConfig struct {
	SwapXY   bool        `json:"swap_xy" toml:"swap_xy" doc:"Swap X and Y axes" default:"false"`
	DelayMs  int         `json:"delay_ms" toml:"delay_ms" doc:"Delay sensor readings (ms)" minimum:"0" maximum:"2000" multipleOf:"5" default:"0"`
	TriggerG float64     `json:"trigger_g" toml:"trigger_g" doc:"Movement threshold on X/Y axes (g)" minimum:"0.001" maximum:"1.0" multipleOf:"0.001" default:"0.02"`
	FadeS    FadeSeconds `json:"fade_s" toml:"fade_s" doc:"Visible duration after trigger (s)" default:"5"`
}

type WaveformConfig struct {
	VizBase
	ForceThresholds
	BufferSize int     `json:"buffer_size" toml:"buffer_size" doc:"Ring buffer sample count" minimum:"32" maximum:"512" default:"256"`
	LogKnee    float64 `json:"log_knee" toml:"log_knee" doc:"Log compression knee" minimum:"0.001" maximum:"0.1" multipleOf:"0.001" default:"0.02"`
	AmpScale   float64 `json:"amp_scale" toml:"amp_scale" doc:"Amplitude multiplier" minimum:"0.1" maximum:"5.0" multipleOf:"0.1" default:"1.0"`
}

type CrosshairConfig struct {
	VizBase
	ForceThresholds
	Decay        float64 `json:"decay_s" toml:"decay_s" doc:"Decay time (s)" minimum:"0" maximum:"2" multipleOf:"0.01" default:"0.3"`
	SegmentSize  int     `json:"segment_size" toml:"segment_size" doc:"Bar segment size (px)" minimum:"2" maximum:"30" default:"10"`
	BarThickness int     `json:"bar_thickness" toml:"bar_thickness" doc:"Bar thickness (px)" minimum:"4" maximum:"30" default:"12"`
	HideNegY     bool    `json:"hide_neg_y" toml:"hide_neg_y" doc:"Hide negative Y arm" default:"false"`
}

type AutoLockConfig struct {
	SpreadWindow    float64 `json:"spread_window" toml:"spread_window" doc:"Sliding window duration (s)" minimum:"1" maximum:"60" default:"5"`
	SpreadThreshold float64 `json:"spread_threshold" toml:"spread_threshold" doc:"Stdev threshold (g) to trigger lock" minimum:"0.001" maximum:"1.0" multipleOf:"0.001" default:"0.005"`
}

type BLEConfig struct {
	DeviceAddress string `json:"device_address" toml:"device_address"`
	DeviceName    string `json:"device_name" toml:"device_name"`
	SensorName    string `json:"sensor_name" toml:"sensor_name"`
}

type ExperimentConfig struct {
	VizBase
	ForceThresholds
	Decay  float64 `json:"decay_s" toml:"decay_s" doc:"Decay time (s)" minimum:"0.05" maximum:"2" multipleOf:"0.01" default:"0.3"`
}

type PinQuakeConfig struct {
	BLE        BLEConfig        `json:"ble" toml:"ble"`
	Waveform   WaveformConfig   `json:"waveform" toml:"waveform"`
	Crosshair  CrosshairConfig  `json:"crosshair" toml:"crosshair"`
	Experiment ExperimentConfig `json:"experiment" toml:"experiment"`
	AutoLock   AutoLockConfig   `json:"auto_lock" toml:"auto_lock"`
	Display    DisplayConfig    `json:"display" toml:"display"`
	Sensor     map[string]any   `json:"sensor,omitempty" toml:"sensor,omitempty"`
}

type ConfigResponse struct {
	Body PinQuakeConfig
}

func DefaultConfig() PinQuakeConfig {
	return PinQuakeConfig{
		BLE: BLEConfig{},
		Waveform: WaveformConfig{
			VizBase:         VizBase{Enabled: true, Width: 608, Height: 1080},
			ForceThresholds: ForceThresholds{ForceYellowG: 0.03, ForceRedG: 0.10},
			BufferSize:      256,
			LogKnee:         0.02,
			AmpScale:        1.0,
		},
		Crosshair: CrosshairConfig{
			VizBase:         VizBase{Enabled: true, Width: 200, Height: 200},
			ForceThresholds: ForceThresholds{ForceYellowG: 0.03, ForceRedG: 0.10},
			Decay:           0.3,
			SegmentSize:     10,
			BarThickness:    12,
		},
		Experiment: ExperimentConfig{
			VizBase:         VizBase{Enabled: true, Width: 400, Height: 400},
			ForceThresholds: ForceThresholds{ForceYellowG: 0.03, ForceRedG: 0.10},
			Decay:           0.3,
		},
		AutoLock: AutoLockConfig{
			SpreadWindow:    5,
			SpreadThreshold: 0.005,
		},
		Display: DisplayConfig{
			DelayMs:  0,
			TriggerG: 0.02,
			FadeS:    5.0,
		},
	}
}

// LoadFromPath loads config from a TOML file, using DefaultConfig as the base.
// Fields present in the file overwrite defaults; absent fields keep defaults.
func LoadFromPath(path string) (PinQuakeConfig, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	var wrapper struct {
		App PinQuakeConfig `toml:"app"`
	}
	wrapper.App = cfg
	if err := toml.Unmarshal(data, &wrapper); err != nil {
		return cfg, err
	}
	return wrapper.App, nil
}

// SaveSection saves a single named section under [app] in the TOML file.
func SaveSection(path, name string, section any) error {
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &existing); err != nil {
			return err
		}
	}
	app, _ := existing["app"].(map[string]any)
	if app == nil {
		app = make(map[string]any)
	}
	app[name] = section
	existing["app"] = app
	data, err := toml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveSensorSection saves a sensor config under [app.sensor.<name>].
func SaveSensorSection(path, name string, cfg any) error {
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &existing); err != nil {
			return err
		}
	}
	app, _ := existing["app"].(map[string]any)
	if app == nil {
		app = make(map[string]any)
	}
	sensorMap, _ := app["sensor"].(map[string]any)
	if sensorMap == nil {
		sensorMap = make(map[string]any)
	}
	sensorMap[name] = cfg
	app["sensor"] = sensorMap
	existing["app"] = app
	data, err := toml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveAll saves the entire app config to the TOML file.
func SaveAll(path string, cfg PinQuakeConfig) error {
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &existing); err != nil {
			return err
		}
	}
	existing["app"] = cfg
	data, err := toml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
