package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smazurov/pinquake/internal/api"
	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
)

type Options struct {
	Config string `default:"config.toml"`
	Port   string `default:":8091" toml:"server.port" env:"SERVER_PORT"`
}

func main() {
	dumpOpenAPI := flag.Bool("openapi", false, "Dump OpenAPI spec to stdout and exit")
	flag.Parse()

	opts := &Options{}
	data.ApplyDefaults(opts)

	if err := data.LoadCLIConfig(opts); err != nil {
		slog.Warn("Failed to load config", "error", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	eventBus := events.New()

	scanner := ble.NewScanner(eventBus, logger.With("module", "ble"))
	bleStop := make(chan struct{})
	if !*dumpOpenAPI {
		go scanner.InitWithRetry(bleStop)
	}

	server := api.NewServer(&api.Options{
		EventBus:   eventBus,
		Scanner:    scanner,
		ConfigPath: opts.Config,
	})

	if *dumpOpenAPI {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(server.HumaAPI().OpenAPI()); err != nil {
			logger.Error("Failed to encode OpenAPI spec", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go server.AutoConnect()

	go func() {
		logger.Info("Starting PinQuake", "port", opts.Port)
		if err := server.Start(opts.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down")
	close(bleStop)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if scanner.GetState() == ble.StateConnected {
			scanner.Disconnect()
		}
		eventBus.Publish(events.BLEStatusEvent{
			Status:    "disconnected",
			Reason:    "shutdown",
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
		if err := server.Stop(shutdownCtx); err != nil {
			logger.Error("Error stopping server", "error", err)
		}
	}()

	select {
	case <-done:
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timed out, forcing exit")
	}
}
