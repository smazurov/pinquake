package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/ui"
)

type Server struct {
	api        huma.API
	mux        *http.ServeMux
	httpServer *http.Server
	eventBus   *events.Bus
	scanner    *ble.Scanner
	configPath string
}

type Options struct {
	EventBus   *events.Bus
	Scanner    *ble.Scanner
	ConfigPath string
}

func NewServer(opts *Options) *Server {
	mux := http.NewServeMux()

	config := huma.DefaultConfig("PinQuake API", "1.0.0")
	config.Info.Description = "BLE orientation data visualization for OBS"
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

	server.registerRoutes()
	server.syncSwapXY()

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
