package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/sensors"
)

func (s *Server) registerConfigRoutes() {
	huma.Get(s.api, "/api/config", func(_ context.Context, _ *struct{}) (*data.ConfigResponse, error) {
		cfg, _ := s.loadAppConfig()
		return &data.ConfigResponse{Body: cfg}, nil
	}, huma.OperationTags("config"))

	registerSection(s, "ble", func(c *data.PinQuakeConfig) *data.BLEConfig { return &c.BLE })
	registerSection(s, "waveform", func(c *data.PinQuakeConfig) *data.WaveformConfig { return &c.Waveform })
	registerSection(s, "crosshair", func(c *data.PinQuakeConfig) *data.CrosshairConfig { return &c.Crosshair })
	registerSection(s, "experiment", func(c *data.PinQuakeConfig) *data.ExperimentConfig { return &c.Experiment })
	registerSection(s, "auto_lock", func(c *data.PinQuakeConfig) *data.AutoLockConfig { return &c.AutoLock })
	registerSection(s, "display", func(c *data.PinQuakeConfig) *data.DisplayConfig { return &c.Display })

	for i := range sensors.Registry {
		entry := sensors.Registry[i]
		if entry.RegisterRoutes == nil {
			continue
		}
		entry.RegisterRoutes(sensors.SensorRouteParams{
			API:  s.api,
			Path: "/api/config/sensor/" + entry.Name,
			Load: func() any { return s.loadSensorConfig(entry) },
			Save: func(cfg any) error {
				s.configMu.Lock()
				defer s.configMu.Unlock()
				return data.SaveSensorSection(s.configPath, entry.Name, cfg)
			},
			Apply: func(cfg any) error { return s.scanner.ApplySensorConfig(entry, cfg) },
			Publish: func() {
				s.eventBus.Publish(events.ConfigChangedEvent{
					Section:   "sensor." + entry.Name,
					Timestamp: time.Now().Format(time.RFC3339Nano),
				})
			},
		})
	}
}

type sectionRequest[T any] struct {
	Body T
}

type sectionResponse[T any] struct {
	Body T
}

func registerSection[T any](s *Server, name string, extract func(*data.PinQuakeConfig) *T) {
	path := "/api/config/" + name

	huma.Get(s.api, path, func(_ context.Context, _ *struct{}) (*sectionResponse[T], error) {
		cfg, _ := s.loadAppConfig()
		return &sectionResponse[T]{Body: *extract(&cfg)}, nil
	}, huma.OperationTags("config"))

	huma.Put(s.api, path, func(_ context.Context, input *sectionRequest[T]) (*sectionResponse[T], error) {
		s.configMu.Lock()
		err := data.SaveSection(s.configPath, name, input.Body)
		s.configMu.Unlock()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to save config: %v", err))
		}
		cfg, _ := s.loadAppConfig()
		s.syncConfig(cfg)
		s.eventBus.Publish(events.ConfigChangedEvent{
			Section:   name,
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
		return &sectionResponse[T]{Body: input.Body}, nil
	}, huma.OperationTags("config"))
}

func (s *Server) loadAppConfig() (data.PinQuakeConfig, error) {
	return data.LoadFromPath(s.configPath)
}

func (s *Server) loadSensorConfig(entry sensors.SensorEntry) any {
	cfg := entry.NewConfig()
	appCfg, err := s.loadAppConfig()
	if err != nil || appCfg.Sensor == nil {
		return cfg
	}
	sensorData, ok := appCfg.Sensor[entry.Name]
	if !ok {
		return cfg
	}
	raw, err := toml.Marshal(sensorData)
	if err != nil {
		slog.Warn("Failed to marshal sensor config", "sensor", entry.Name, "error", err)
		return cfg
	}
	if err := toml.Unmarshal(raw, cfg); err != nil {
		slog.Warn("Failed to unmarshal sensor config, using defaults", "sensor", entry.Name, "error", err)
		return entry.NewConfig()
	}
	return cfg
}
