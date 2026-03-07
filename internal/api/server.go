package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/sensors"
	"github.com/smazurov/pinquake/ui"
)

const maxEventLogEntries = 200

type Server struct {
	api        huma.API
	mux        *http.ServeMux
	httpServer *http.Server
	eventBus   *events.Bus
	scanner    *ble.Scanner
	configPath string
	configMu   sync.Mutex
	eventLogMu sync.Mutex
	eventLog   []events.LogEntry
}

type Options struct {
	EventBus   *events.Bus
	Scanner    *ble.Scanner
	ConfigPath string
}

func NewServer(opts *Options) *Server {
	mux := http.NewServeMux()

	config := huma.DefaultConfig("PinQuake API", "1.0.0")
	config.Info.Description = "BLE orientation data visualization"
	config.Servers = []*huma.Server{}

	api := humago.New(mux, config)

	corsConfig := DefaultCORSConfig()
	AddCORSHandler(mux, corsConfig)
	api.UseMiddleware(NewCORSMiddleware(corsConfig))

	server := &Server{
		api:        api,
		mux:        mux,
		eventBus:   opts.EventBus,
		scanner:    opts.Scanner,
		configPath: opts.ConfigPath,
	}

	opts.Scanner.OnConnect(func(sensorName string) {
		server.configMu.Lock()
		defer server.configMu.Unlock()
		cfg, _ := server.loadAppConfig()
		cfg.BLE.SensorName = sensorName
		_ = server.saveAppConfig(cfg)
	})

	opts.EventBus.Subscribe(func(e events.BLEStatusEvent) {
		switch e.Status {
		case "connecting":
			server.log("info", fmt.Sprintf("Connecting to %s", e.DisplayName()))
		case "connected":
			server.log("info", fmt.Sprintf("Connected to %s", e.DisplayName()))
		case "idle":
			if e.Device != "" {
				server.log("error", fmt.Sprintf("Connection to %s failed", e.DisplayName()))
			}
		case "disconnected":
			msg := "Disconnected"
			if e.Reason != "" {
				msg = fmt.Sprintf("Disconnected (%s)", e.Reason)
			}
			server.log("warn", msg)
		}
	})

	server.registerRoutes()

	if cfg, err := server.loadAppConfig(); err == nil {
		server.syncConfig(cfg)
	}

	if frontendHandler, err := ui.Handler(); err == nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api") {
				http.NotFound(w, r)
				return
			}
			frontendHandler.ServeHTTP(w, r)
		})
	}

	return server
}

func (s *Server) Start(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) AutoConnect() {
	cfg, err := s.loadAppConfig()
	if err != nil {
		return
	}
	if cfg.BLE.DeviceAddress != "" {
		// Wait for BLE adapter to be ready before attempting connection.
		select {
		case <-s.scanner.Ready():
		case <-time.After(2 * time.Minute):
			return
		}
		if cfg.BLE.SensorName != "" {
			if entry := sensors.FactoryByName(cfg.BLE.SensorName); entry != nil {
				s.scanner.SetSensorFactory(entry.Factory)
			}
		}
		_ = s.scanner.Connect(cfg.BLE.DeviceAddress, cfg.BLE.DeviceName)
	}
}

func (s *Server) log(level, message string) {
	entry := events.LogEntry{
		Message:   message,
		Level:     level,
		Timestamp: time.Now().Format(time.RFC3339Nano),
	}
	s.eventLogMu.Lock()
	s.eventLog = append(s.eventLog, entry)
	if len(s.eventLog) > maxEventLogEntries {
		s.eventLog = s.eventLog[len(s.eventLog)-maxEventLogEntries:]
	}
	s.eventLogMu.Unlock()
	s.eventBus.Publish(entry)
}

func (s *Server) syncConfig(cfg PinQuakeConfig) {
	s.scanner.SetSwapXY(cfg.Waveform.SwapXY)
	s.scanner.SetAutoLockParams(
		float32(cfg.AutoLock.Epsilon),
		time.Duration(cfg.AutoLock.Timeout*float64(time.Second)),
	)
}

func (s *Server) HumaAPI() huma.API {
	return s.api
}

func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(context.Background())
	}
	return nil
}

func (s *Server) registerRoutes() {
	s.registerConfigRoutes()
	s.registerSSERoutes()
	s.registerBLERoutes()
}
