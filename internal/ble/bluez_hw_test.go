//go:build hw

package ble

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"tinygo.org/x/bluetooth"
)

func hwRadio(t *testing.T) *bluezRadio {
	t.Helper()
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		t.Skipf("no BLE adapter: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &bluezRadio{adapter: adapter, logger: logger}
}

// otherClientDiscovers runs a discovery session from a separate D-Bus
// client, like bluetoothctl or a second app on the same adapter.
func otherClientDiscovers(t *testing.T, d time.Duration) {
	t.Helper()
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	adapter := conn.Object("org.bluez", "/org/bluez/hci0")
	if err := adapter.Call("org.bluez.Adapter1.SetDiscoveryFilter", 0,
		map[string]any{"Transport": "le"}).Err; err != nil {
		t.Fatal(err)
	}
	if err := adapter.Call("org.bluez.Adapter1.StartDiscovery", 0).Err; err != nil {
		t.Fatal(err)
	}
	time.Sleep(d)
	if err := adapter.Call("org.bluez.Adapter1.StopDiscovery", 0).Err; err != nil {
		t.Fatal(err)
	}
}

// TestHWScanSurvivesOtherClients: BlueZ's Discovering flag is per adapter,
// so another client stopping discovery can flip it off while we still
// scan. Our scan must keep running, and BlueZ must not be left holding a
// session of ours that makes the next StartDiscovery fail.
func TestHWScanSurvivesOtherClients(t *testing.T) {
	r := hwRadio(t)
	for i := range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		done := make(chan error, 1)
		start := time.Now()
		go func() { done <- r.Scan(ctx, func(Advertisement) {}) }()
		time.Sleep(time.Second)
		otherClientDiscovers(t, 2*time.Second)
		err := <-done
		cancel()
		ran := time.Since(start)
		t.Logf("round %d: scan ran %v, err=%v", i, ran.Round(time.Millisecond), err)
		if err != nil || ran < 5*time.Second {
			t.Fatalf("round %d: scan ended after %v: %v", i, ran.Round(time.Millisecond), err)
		}
	}
	// A leaked session shows up as InProgress on the next start.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Scan(ctx, func(Advertisement) {}); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("scan after other clients: %v", err)
	}
}

// TestHWScanCyclesKeepWorking duty-cycles real BlueZ scans like a search for
// an absent device. Every scan must start: a discovery session BlueZ still
// holds for us makes StartDiscovery fail with "Operation already in
// progress" until the process exits.
//
//	go test -tags hw -run TestHWScanCycles -v ./internal/ble/ (HW_CYCLES=n)
func TestHWScanCyclesKeepWorking(t *testing.T) {
	cycles := 8
	if n, err := strconv.Atoi(os.Getenv("HW_CYCLES")); err == nil {
		cycles = n
	}
	r := hwRadio(t)
	total := 0
	for i := range cycles {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		seen := 0
		start := time.Now()
		err := r.Scan(ctx, func(Advertisement) { seen++ })
		cancel()
		total += seen
		t.Logf("cycle %d: ran %v, %d adverts, err=%v", i, time.Since(start).Round(time.Millisecond), seen, err)
		if err != nil && time.Since(start) < 9*time.Second {
			t.Fatalf("cycle %d: scan failed to start: %v", i, err)
		}
		time.Sleep(2 * time.Second)
	}
	if total == 0 {
		t.Error("no advertisements in any cycle")
	}
}
