package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smazurov/pinquake/internal/api"
	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/config"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/obs"
)

type Options struct {
	Config string `default:"config.toml"`
	Port   string `default:":8091" toml:"server.port" env:"SERVER_PORT"`
}

func main() {
	opts := &Options{}
	config.ApplyDefaults(opts)

	if err := config.LoadConfig(opts); err != nil {
		slog.Warn("Failed to load config", "error", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	eventBus := events.New()

	scanner := ble.NewScanner(eventBus, logger.With("module", "ble"))
	bleStop := make(chan struct{})
	go scanner.InitWithRetry(bleStop)

	obsClient := obs.NewClient(eventBus, logger.With("module", "obs"))

	server := api.NewServer(&api.Options{
		EventBus:   eventBus,
		Scanner:    scanner,
		OBS:        obsClient,
		ConfigPath: opts.Config,
	})

	go server.AutoConnect()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Shutting down")
		close(bleStop)
		if scanner.GetState() == ble.StateConnected {
			scanner.Disconnect()
		}
		eventBus.Publish(events.BLEStatusEvent{
			Status:    "disconnected",
			Reason:    "shutdown",
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
		if err := server.Stop(); err != nil {
			logger.Error("Error stopping server", "error", err)
		}
		os.Exit(0)
	}()

	logger.Info("Starting PinQuake", "port", opts.Port)
	if err := server.Start(opts.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
