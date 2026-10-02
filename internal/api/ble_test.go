package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
)

const sensorAddr = "EA:F0:F1:BC:59:DD"

// scanRadio hands each scan to the test; connects never finish.
type scanRadio struct {
	scans chan *radioScan
	live  atomic.Int32
}

type radioScan struct {
	found func(ble.Advertisement)
	fail  chan error
}

func (r *scanRadio) Scan(ctx context.Context, found func(ble.Advertisement)) error {
	r.live.Add(1)
	defer r.live.Add(-1)
	sc := &radioScan{found: found, fail: make(chan error, 1)}
	r.scans <- sc
	select {
	case <-ctx.Done():
		return nil
	case err := <-sc.fail:
		return err
	}
}

func (r *scanRadio) Connect(ctx context.Context, _ string) (ble.Link, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// nextScan returns the scan that just started.
func (r *scanRadio) nextScan(t *testing.T) *radioScan {
	t.Helper()
	synctest.Wait()
	select {
	case sc := <-r.scans:
		return sc
	default:
		t.Fatal("expected a scan to start")
		return nil
	}
}

// noScan checks that no scan is running or starting.
func (r *scanRadio) noScan(t *testing.T) {
	t.Helper()
	synctest.Wait()
	select {
	case <-r.scans:
		t.Fatal("unexpected scan")
	default:
	}
	if r.live.Load() != 0 {
		t.Fatalf("%d scan(s) still running", r.live.Load())
	}
}

// newScanTestServer must be called inside a synctest bubble.
func newScanTestServer(t *testing.T) (*Server, *scanRadio) {
	t.Helper()
	bus := events.New()
	radio := &scanRadio{scans: make(chan *radioScan, 8)}
	scanner := ble.NewScanner(bus, slog.New(slog.DiscardHandler), ble.WithRadio(radio))
	s := NewServer(&Options{
		EventBus:   bus,
		Scanner:    scanner,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scanner.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		_ = s.Stop(context.Background())
		bus.Close()
	})
	return s, radio
}

type sseEvent struct {
	name string
	data map[string]any
}

// stream is an SSE request served in the background.
type stream struct {
	rec    *httptest.ResponseRecorder
	done   chan struct{}
	cancel context.CancelFunc
}

func openStream(s *Server, path string) *stream {
	ctx, cancel := context.WithCancel(context.Background())
	st := &stream{rec: httptest.NewRecorder(), done: make(chan struct{}), cancel: cancel}
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	go func() {
		s.mux.ServeHTTP(st.rec, req)
		close(st.done)
	}()
	return st
}

// events returns what the stream sent; it must have ended.
func (st *stream) events(t *testing.T) []sseEvent {
	t.Helper()
	synctest.Wait()
	select {
	case <-st.done:
	default:
		t.Fatal("stream is still open")
	}
	var evs []sseEvent
	var name string
	sc := bufio.NewScanner(strings.NewReader(st.rec.Body.String()))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			var data map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data); err != nil {
				t.Fatalf("bad data line %q: %v", line, err)
			}
			evs = append(evs, sseEvent{name: name, data: data})
		}
	}
	return evs
}

func TestScanStreamEndsWhenDeviceChosen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, radio := newScanTestServer(t)
		st := openStream(s, "/api/ble/scan")
		defer st.cancel()
		radio.nextScan(t).found(ble.Advertisement{Address: sensorAddr, Name: "WT901BLE68", RSSI: -60, SensorName: "WT901"})
		synctest.Wait()

		if code, body := call(t, s, http.MethodPost, "/api/ble/connect",
			`{"address":"`+sensorAddr+`","name":"WT901BLE68"}`); code != http.StatusOK {
			t.Fatalf("connect: %d %v", code, body)
		}

		evs := st.events(t)
		var devices []string
		for _, e := range evs {
			if e.name == "device" {
				devices = append(devices, e.data["address"].(string))
			}
		}
		if len(devices) != 1 || devices[0] != sensorAddr {
			t.Errorf("devices = %v, want [%s]", devices, sensorAddr)
		}
		last := evs[len(evs)-1]
		if last.name != "scan-state" || last.data["state"] != "ended" || last.data["reason"] != "device-chosen" {
			t.Errorf("last event = %s %v, want scan-state ended/device-chosen", last.name, last.data)
		}
	})
}

func TestScanStreamEndsAfterAMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, radio := newScanTestServer(t)
		st := openStream(s, "/api/ble/scan")
		defer st.cancel()
		radio.nextScan(t)

		time.Sleep(time.Minute)
		evs := st.events(t)
		last := evs[len(evs)-1]
		if last.name != "scan-state" || last.data["state"] != "ended" || last.data["reason"] != "timeout" {
			t.Errorf("last event = %s %v, want scan-state ended/timeout", last.name, last.data)
		}
		radio.noScan(t) // the scan stopped with it
	})
}

func TestScanStreamSendsHeartbeatsWhileQuiet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, radio := newScanTestServer(t)
		st := openStream(s, "/api/ble/scan")
		radio.nextScan(t)

		time.Sleep(40 * time.Second) // nothing advertises
		st.cancel()
		beats := 0
		for _, e := range st.events(t) {
			if e.name == "heartbeat" {
				beats++
			}
		}
		if beats < 2 {
			t.Errorf("%d heartbeats in 40s; the client gives up after 20s of silence", beats)
		}
	})
}

func TestScanStreamReportsFailedScan(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, radio := newScanTestServer(t)
		st := openStream(s, "/api/ble/scan")
		radio.nextScan(t).fail <- errors.New("adapter busy")
		synctest.Wait()

		time.Sleep(2 * time.Second) // backoff
		radio.nextScan(t)
		st.cancel()

		var states []sseEvent
		for _, e := range st.events(t) {
			if e.name == "scan-state" {
				states = append(states, e)
			}
		}
		if len(states) < 2 {
			t.Fatalf("scan states = %v, want waiting then scanning", states)
		}
		waiting, again := states[len(states)-2].data, states[len(states)-1].data
		if waiting["state"] != "waiting" || waiting["error"] != "adapter busy" || waiting["retry_in_s"] != 2.0 {
			t.Errorf("after the failure: %v, want waiting/adapter busy/2s", waiting)
		}
		if again["state"] != "scanning" {
			t.Errorf("after the retry: %v, want scanning", again)
		}
	})
}

func TestScanStreamsEachGetEveryDeviceOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, radio := newScanTestServer(t)
		a, b := openStream(s, "/api/ble/scan"), openStream(s, "/api/ble/scan")
		radio.nextScan(t).found(ble.Advertisement{Address: sensorAddr}) // one scan, shared
		synctest.Wait()
		a.cancel()
		b.cancel()
		for _, st := range []*stream{a, b} {
			n := 0
			for _, e := range st.events(t) {
				if e.name == "device" {
					n++
				}
			}
			if n != 1 {
				t.Errorf("stream got %d device events for one advertisement, want 1", n)
			}
		}
	})
}

func TestRetryEndpointSearchesNow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, radio := newScanTestServer(t)
		if code, body := call(t, s, http.MethodPost, "/api/ble/connect", `{"address":"`+sensorAddr+`","name":"WT901BLE68"}`); code != http.StatusOK {
			t.Fatalf("connect: %d %v", code, body)
		}
		radio.nextScan(t)
		time.Sleep(10 * time.Second) // not found: backing off
		radio.noScan(t)

		if code, body := call(t, s, http.MethodPost, "/api/ble/retry", ""); code != http.StatusOK {
			t.Fatalf("retry: %d %v", code, body)
		}
		radio.nextScan(t)
	})
}

func TestChosenDeviceIsNotSavedBeforeItConnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, radio := newScanTestServer(t)
		if code, body := call(t, s, http.MethodPost, "/api/ble/connect", `{"address":"`+sensorAddr+`","name":"WT901BLE68"}`); code != http.StatusOK {
			t.Fatalf("connect: %d %v", code, body)
		}
		radio.nextScan(t) // searching
		cfg, _ := data.LoadFromPath(s.configPath)
		if cfg.BLE.DeviceAddress != "" {
			t.Fatalf("saved %q before it ever connected; a wrong pick would be retried after every restart", cfg.BLE.DeviceAddress)
		}
	})
}

func TestConnectedDeviceIsSaved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _ := newScanTestServer(t)
		call(t, s, http.MethodPost, "/api/ble/connect", `{"address":"`+sensorAddr+`","name":"WT901BLE68"}`)
		s.onBLEConnect(ble.ConnectedDevice{Addr: sensorAddr, Name: "WT901BLE68", SensorName: "WT901"})
		cfg, _ := data.LoadFromPath(s.configPath)
		want := data.BLEConfig{DeviceAddress: sensorAddr, DeviceName: "WT901BLE68", SensorName: "WT901"}
		if cfg.BLE != want {
			t.Fatalf("saved %+v, want %+v", cfg.BLE, want)
		}
	})
}

func TestForgottenDeviceStaysForgottenWhenItsConnectReportsLate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _ := newScanTestServer(t)
		call(t, s, http.MethodPost, "/api/ble/connect", `{"address":"`+sensorAddr+`","name":"WT901BLE68"}`)
		call(t, s, http.MethodPost, "/api/ble/disconnect", "")
		s.onBLEConnect(ble.ConnectedDevice{Addr: sensorAddr, Name: "WT901BLE68"}) // reported just before the Forget
		cfg, _ := data.LoadFromPath(s.configPath)
		if cfg.BLE.DeviceAddress != "" {
			t.Fatalf("forgotten device saved again: %+v", cfg.BLE)
		}
	})
}
