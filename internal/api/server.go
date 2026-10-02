package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/framelock"
	"github.com/smazurov/pinquake/internal/obs"
	"github.com/smazurov/pinquake/internal/sensors"
	"github.com/smazurov/pinquake/internal/viz"
	"github.com/smazurov/pinquake/ui"
)

const maxEventLogEntries = 200

type Server struct {
	api        huma.API
	mux        *http.ServeMux
	httpServer *http.Server
	eventBus   *events.Bus
	scanner    *ble.Scanner
	trigger    *viz.Trigger
	overlay    *viz.Overlay
	obs        *obs.Controller
	configPath string
	configMu   sync.Mutex
	eventLogMu sync.Mutex
	eventLog   []events.LogEntry
	lastOBS    obs.Status // for logging transitions; OBS goroutine only
	lastOBSErr string
}

type Options struct {
	EventBus   *events.Bus
	Scanner    *ble.Scanner
	ConfigPath string
	// OBSDial opens obs-websocket sessions; nil means obs.Dial.
	OBSDial obs.Dialer
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

	appCfg, err := data.LoadFromPath(opts.ConfigPath)
	if err != nil {
		if err := data.SaveAll(opts.ConfigPath, appCfg); err != nil {
			slog.Error("Failed to write default config", "error", err)
		}
	}

	trigger := viz.NewTrigger(opts.EventBus, viz.TriggerConfig{
		DelayMs:  appCfg.Display.DelayMs,
		TriggerG: appCfg.Display.TriggerG,
		FadeS:    float64(appCfg.Display.FadeS),
	})

	server := &Server{
		api:        api,
		mux:        mux,
		eventBus:   opts.EventBus,
		scanner:    opts.Scanner,
		trigger:    trigger,
		overlay:    viz.NewOverlay(opts.EventBus),
		configPath: opts.ConfigPath,
		lastOBS:    obs.Status{State: obs.StateOff},
	}

	dial := opts.OBSDial
	if dial == nil {
		dial = obs.Dial
	}
	server.obs = obs.NewController(obs.Options{
		Dial:     dial,
		OnStatus: server.onOBSStatus,
		OnTarget: server.saveRenamedOBSTarget,
	})
	server.obs.Start()

	opts.Scanner.OnConnect(server.onBLEConnect)

	opts.EventBus.Subscribe(func(e events.VizTriggerEvent) {
		server.overlay.SetTrigger(e.Visible)
		server.obs.SetVisible(e.Visible)
		if e.Visible {
			server.log("info", "Viz triggered")
		} else {
			server.log("info", "Viz hidden")
		}
	})

	opts.EventBus.Subscribe(server.logFrameLock)

	opts.EventBus.Subscribe(server.bleStatusLogger())

	// Start after subscribing so an always-visible trigger's initial show
	// reaches the overlay and OBS.
	trigger.Start()

	server.registerRoutes()

	server.syncConfig(appCfg)

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

// onBLEConnect saves the device a link came up to, so it is reconnected
// after a restart, and applies its sensor config.
func (s *Server) onBLEConnect(d ble.ConnectedDevice) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	// Runs after the fact: the device may have been forgotten since. The
	// disconnect handler forgets it before taking configMu.
	if !s.scanner.Wants(d.Addr) {
		return
	}
	cfg, _ := s.loadAppConfig()
	cfg.BLE = data.BLEConfig{DeviceAddress: d.Addr, DeviceName: d.Name, SensorName: d.SensorName}
	if err := data.SaveAll(s.configPath, cfg); err != nil {
		slog.Error("Failed to save BLE device", "error", err)
	}

	entry := sensors.FactoryByName(d.SensorName)
	if entry != nil && entry.NewConfig != nil {
		sensorCfg := s.loadSensorConfig(*entry)
		if err := s.scanner.ApplySensorConfig(*entry, sensorCfg); err != nil {
			s.log("error", fmt.Sprintf("Failed to apply %s config: %v", d.SensorName, err))
		} else {
			s.log("info", fmt.Sprintf("Applied %s sensor config", d.SensorName))
		}
	}
}

// bleStatusLogger logs BLE link transitions. The search duty cycle (not
// found, retry) only shows in the status; other failures are logged once
// until the link connects or the device changes.
func (s *Server) bleStatusLogger() func(events.BLEStatusEvent) {
	prev := events.BLEStatusEvent{Status: "idle"}
	var lastErr string
	return func(e events.BLEStatusEvent) {
		if e.Device != prev.Device || e.Status == "connected" {
			lastErr = ""
		}
		switch e.Status {
		case "searching":
			switch {
			case e.Reason == string(ble.ReasonLost) && prev.Status == "connected":
				s.log("warn", fmt.Sprintf("Lost connection to %s; searching", e.DisplayName()))
			case e.Device != prev.Device:
				s.log("info", fmt.Sprintf("Searching for %s", e.DisplayName()))
			}
		case "waiting":
			if e.Error != ble.ErrNotFound.Error() && e.Error != lastErr {
				s.log("error", fmt.Sprintf("%s: %s; retrying", e.DisplayName(), e.Error))
				lastErr = e.Error
			}
		case "connecting":
			s.log("info", fmt.Sprintf("Connecting to %s", e.DisplayName()))
		case "connected":
			s.log("info", fmt.Sprintf("Connected to %s", e.DisplayName()))
		case "idle":
			switch {
			case prev.Status == "idle":
			case e.Reason == "shutdown":
				s.log("warn", fmt.Sprintf("Disconnected from %s (shutdown)", prev.DisplayName()))
			case prev.Status == "connected":
				s.log("info", fmt.Sprintf("Disconnected from %s", prev.DisplayName()))
			default:
				s.log("info", fmt.Sprintf("Stopped searching for %s", prev.DisplayName()))
			}
		}
		prev = e
	}
}

// AutoConnect makes the saved device the wanted one. The scanner keeps
// searching for it (and reconnecting) until the user disconnects.
func (s *Server) AutoConnect() {
	cfg, err := s.loadAppConfig()
	if err != nil || cfg.BLE.DeviceAddress == "" {
		return
	}
	if entry := sensors.FactoryByName(cfg.BLE.SensorName); entry != nil {
		s.scanner.SetSensorFactory(entry.Factory)
	}
	if err := s.scanner.Connect(cfg.BLE.DeviceAddress, cfg.BLE.DeviceName); err != nil {
		s.log("error", fmt.Sprintf("Saved BLE device ignored: %v", err))
	}
}

// AutoConnectOBS reconnects to OBS if it was connected when last saved.
func (s *Server) AutoConnectOBS() {
	cfg, _ := s.loadAppConfig()
	if !cfg.OBS.Connect {
		return
	}
	s.obs.SetTarget(obsTarget(cfg.OBS))
	go s.obs.Connect(cfg.OBS.Server, cfg.OBS.Password) // don't hold up startup on a slow dial
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

func (s *Server) syncConfig(cfg data.PinQuakeConfig) {
	s.scanner.SetSwapXY(cfg.Display.SwapXY)
	s.scanner.ConfigureFrameLock(frameLockConfig(cfg))
	s.scanner.ConfigureLink(ble.SupervisorConfig{
		ScanWindow:     seconds(cfg.BLELink.ScanWindowS),
		ConnectTimeout: seconds(cfg.BLELink.ConnectTimeoutS),
		BackoffMin:     seconds(cfg.BLELink.BackoffMinS),
		BackoffMax:     seconds(cfg.BLELink.BackoffMaxS),
	})
	s.trigger.SetConfig(viz.TriggerConfig{
		DelayMs:  cfg.Display.DelayMs,
		TriggerG: cfg.Display.TriggerG,
		FadeS:    float64(cfg.Display.FadeS),
	})
}

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

func frameLockConfig(cfg data.PinQuakeConfig) framelock.Config {
	return framelock.Config{
		Enabled:         cfg.Frame.AutoLock,
		Window:          seconds(cfg.AutoLock.SpreadWindow),
		Threshold:       float32(cfg.AutoLock.SpreadThreshold),
		RelockThreshold: float32(cfg.AutoLock.RelockThreshold),
	}
}

func (s *Server) logFrameLock(e events.FrameStateEvent) {
	switch framelock.Reason(e.Reason) {
	case framelock.ReasonStable:
		s.log("info", fmt.Sprintf("Frame locked: stable (stdev %.4fg)", e.Stdev))
	case framelock.ReasonDrift:
		s.log("info", fmt.Sprintf("Frame re-locked: settled %.4fg away", e.Drift))
	case framelock.ReasonTrigger:
		s.log("info", "Frame force-locked")
	case framelock.ReasonSettleTimeout:
		cfg, _ := s.loadAppConfig()
		lc := frameLockConfig(cfg)
		s.log("warn", fmt.Sprintf(
			"Frame force-locked: not stable within %s (stdev %.4fg, threshold %.4fg); will re-lock once stable",
			lc.SettleTimeout(), e.Stdev, lc.Threshold))
	}
}

func (s *Server) HumaAPI() huma.API {
	return s.api
}

func (s *Server) Stop(ctx context.Context) error {
	s.trigger.Stop()
	s.obs.Stop()
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) registerRoutes() {
	s.registerConfigRoutes()
	s.registerSSERoutes()
	s.registerBLERoutes()
	s.registerOBSRoutes()
}
