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

type Server struct {
	api        huma.API
	mux        *http.ServeMux
	httpServer *http.Server
	eventBus   *events.Bus
	scanner    *ble.Scanner
	configPath string
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
		cfg, _ := server.loadAppConfig()
		cfg.BLE.SensorName = sensorName
		_ = server.saveAppConfig(cfg)
	})

	opts.EventBus.Subscribe(func(e events.BLEStatusEvent) {
		switch e.Status {
		case "connecting":
			name := e.DeviceName
			if name == "" {
				name = e.Device
			}
			server.log("info", fmt.Sprintf("Connecting to %s", name))
		case "connected":
			name := e.DeviceName
			if name == "" {
				name = e.Device
			}
			server.log("info", fmt.Sprintf("Connected to %s", name))
		case "idle":
			if e.Device != "" {
				server.log("error", fmt.Sprintf("Connection failed: %s", e.Device))
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
	server.syncSwapXY()
	server.syncAutoLock()

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
	s.eventLogMu.Unlock()
	s.eventBus.Publish(entry)
}

func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(context.Background())
	}
	return nil
}

func (s *Server) registerRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/api/health",
		Summary:     "Health check",
		Tags:        []string{"health"},
	}, func(_ context.Context, _ *struct{}) (*HealthResponse, error) {
		return &HealthResponse{
			Body: HealthData{
				Status:  "ok",
				Message: "PinQuake is running",
			},
		}, nil
	})

	s.registerConfigRoutes()
	s.registerSSERoutes()
	s.registerBLERoutes()
}

type HealthData struct {
	Status  string `json:"status" example:"ok"`
	Message string `json:"message" example:"PinQuake is running"`
}

type HealthResponse struct {
	Body HealthData
}
