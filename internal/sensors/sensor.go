package sensors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"tinygo.org/x/bluetooth"
)

var ErrUnsupported = errors.New("unsupported by this sensor")

type BatteryState struct {
	Percent  uint8
	Volts    float32
	Charging bool
}

type Sensor interface {
	Name() string
	ServiceUUIDs() []bluetooth.UUID
	Connect(device *bluetooth.Device, onOrientation func([]byte)) error
	ReadBattery() (*BatteryState, error)
	ReadTemperature() (float32, error)
	Calibrate() error
	Close()
}

type SensorRouteParams struct {
	API     huma.API
	Path    string
	Load    func() any
	Save    func(any) error
	Apply   func(any) error
	Publish func()
}

type sensorResp[T any] struct{ Body T }
type sensorReq[T any] struct{ Body T }

func RegisterSensorRoutes[T any](p SensorRouteParams, defaults func() T) {
	huma.Get(p.API, p.Path, func(_ context.Context, _ *struct{}) (*sensorResp[T], error) {
		cfg, ok := p.Load().(*T)
		if !ok {
			d := defaults()
			return &sensorResp[T]{Body: d}, nil
		}
		return &sensorResp[T]{Body: *cfg}, nil
	}, huma.OperationTags("config"))

	huma.Put(p.API, p.Path, func(_ context.Context, input *sensorReq[T]) (*sensorResp[T], error) {
		if err := p.Save(input.Body); err != nil {
			return nil, huma.Error500InternalServerError("save: " + err.Error())
		}
		p.Publish()
		if err := p.Apply(&input.Body); err != nil {
			slog.Warn("failed to apply sensor config", "error", err)
		}
		return &sensorResp[T]{Body: input.Body}, nil
	}, huma.OperationTags("config"))
}

type SensorEntry struct {
	Name           string
	ServiceUUIDs   []bluetooth.UUID
	Factory        func() Sensor
	NewConfig      func() any              // nil = not configurable
	ApplyConfig    func(Sensor, any) error // nil = not configurable
	RegisterRoutes func(SensorRouteParams)
}

var Registry = []SensorEntry{
	{Name: "PinLevel", ServiceUUIDs: []bluetooth.UUID{pinlevelServiceParsedUUID}, Factory: NewPinLevel},
	{
		Name:         "WT901",
		ServiceUUIDs: []bluetooth.UUID{wt901ServiceParsedUUID},
		Factory:      NewWT901,
		NewConfig:    newWT901Config,
		ApplyConfig:  applyWT901Config,
		RegisterRoutes: func(p SensorRouteParams) {
			RegisterSensorRoutes[WT901Config](p, DefaultWT901Config)
		},
	},
}

func FactoryByName(name string) *SensorEntry {
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	return nil
}

func Match(result bluetooth.ScanResult) *SensorEntry {
	for i := range Registry {
		for _, uuid := range Registry[i].ServiceUUIDs {
			if result.HasServiceUUID(uuid) {
				return &Registry[i]
			}
		}
	}
	return nil
}

func mustParseUUID(s string) bluetooth.UUID {
	uuid, err := bluetooth.ParseUUID(s)
	if err != nil {
		panic(fmt.Sprintf("invalid UUID constant %q: %v", s, err))
	}
	return uuid
}
