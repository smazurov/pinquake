package sensors

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type OutputRate int

func intEnum(vals ...int) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = float64(v)
	}
	return out
}

func (o OutputRate) Schema(_ huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "integer",
		Enum: intEnum(10, 20, 50, 100, 200),
	}
}

func (o OutputRate) MarshalJSON() ([]byte, error)  { return json.Marshal(int(o)) }
func (o *OutputRate) UnmarshalJSON(b []byte) error  { var v int; err := json.Unmarshal(b, &v); *o = OutputRate(v); return err }
func (o *OutputRate) UnmarshalText(b []byte) error  { v, err := strconv.Atoi(string(b)); *o = OutputRate(v); return err }

type AccelRange int

func (a AccelRange) Schema(_ huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "integer",
		Enum: intEnum(2, 4, 8, 16),
	}
}

func (a AccelRange) MarshalJSON() ([]byte, error)  { return json.Marshal(int(a)) }
func (a *AccelRange) UnmarshalJSON(b []byte) error  { var v int; err := json.Unmarshal(b, &v); *a = AccelRange(v); return err }
func (a *AccelRange) UnmarshalText(b []byte) error  { v, err := strconv.Atoi(string(b)); *a = AccelRange(v); return err }

type Bandwidth int

func (bw Bandwidth) Schema(_ huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "integer",
		Enum: intEnum(5, 10, 21, 44, 94, 184, 256),
	}
}

func (bw Bandwidth) MarshalJSON() ([]byte, error)  { return json.Marshal(int(bw)) }
func (bw *Bandwidth) UnmarshalJSON(b []byte) error  { var v int; err := json.Unmarshal(b, &v); *bw = Bandwidth(v); return err }
func (bw *Bandwidth) UnmarshalText(b []byte) error  { v, err := strconv.Atoi(string(b)); *bw = Bandwidth(v); return err }

type WT901Config struct {
	OutputRateHz OutputRate `json:"output_rate_hz" toml:"output_rate_hz" doc:"Output rate (Hz)" default:"50"`
	AccelRangeG  AccelRange `json:"accel_range_g" toml:"accel_range_g" doc:"Accelerometer range (±g)" default:"4"`
	BandwidthHz  Bandwidth  `json:"bandwidth_hz" toml:"bandwidth_hz" doc:"Low-pass filter cutoff (Hz)" default:"256"`
	SixAxis      bool       `json:"six_axis" toml:"six_axis" doc:"6-axis mode (disable magnetometer)" default:"true"`
	AccelFilter  int        `json:"accel_filter" toml:"accel_filter" doc:"Accel smoothing (0=raw, 1-15)" minimum:"0" maximum:"15" default:"0"`
}

func DefaultWT901Config() WT901Config {
	return WT901Config{
		OutputRateHz: 50,
		AccelRangeG:  4,
		BandwidthHz:  256,
		SixAxis:      true,
		AccelFilter:  0,
	}
}

func newWT901Config() any {
	cfg := DefaultWT901Config()
	return &cfg
}

var outputRateRegMap = map[OutputRate]byte{
	10: 0x06, 20: 0x07, 50: 0x08, 100: 0x09, 200: 0x0B,
}

var accelRangeRegMap = map[AccelRange]byte{
	2: 0x00, 4: 0x01, 8: 0x02, 16: 0x03,
}

var bandwidthRegMap = map[Bandwidth]byte{
	256: 0x00, 184: 0x01, 94: 0x02, 44: 0x03, 21: 0x04, 10: 0x05, 5: 0x06,
}

func applyWT901Config(s Sensor, cfgAny any) error {
	w, ok := s.(*WT901)
	if !ok {
		return fmt.Errorf("expected *WT901, got %T", s)
	}
	cfg, ok := cfgAny.(*WT901Config)
	if !ok {
		return fmt.Errorf("expected *WT901Config, got %T", cfgAny)
	}

	if err := w.unlock(); err != nil {
		return fmt.Errorf("unlock: %w", err)
	}

	type regWrite struct {
		name string
		addr byte
		lo   byte
	}

	var writes []regWrite

	if v, ok := outputRateRegMap[cfg.OutputRateHz]; ok {
		writes = append(writes, regWrite{"RRATE", 0x03, v})
	}
	if v, ok := accelRangeRegMap[cfg.AccelRangeG]; ok {
		writes = append(writes, regWrite{"ACCRANGE", 0x21, v})
	}
	if v, ok := bandwidthRegMap[cfg.BandwidthHz]; ok {
		writes = append(writes, regWrite{"BANDWIDTH", 0x1F, v})
	}

	axis6Lo := byte(0x00)
	if cfg.SixAxis {
		axis6Lo = 0x01
	}
	writes = append(writes, regWrite{"AXIS6", 0x24, axis6Lo})

	if cfg.AccelFilter >= 0 && cfg.AccelFilter <= 15 {
		writes = append(writes, regWrite{"ACCFILT", 0x2A, byte(cfg.AccelFilter)})
	}

	for _, wr := range writes {
		if err := w.writeRegister(wr.addr, wr.lo, 0x00); err != nil {
			return fmt.Errorf("write %s: %w", wr.name, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := w.save(); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	slog.Info("WT901 config applied",
		"output_rate_hz", cfg.OutputRateHz,
		"accel_range_g", cfg.AccelRangeG,
		"bandwidth_hz", cfg.BandwidthHz,
		"six_axis", cfg.SixAxis,
		"accel_filter", cfg.AccelFilter,
	)
	return nil
}
