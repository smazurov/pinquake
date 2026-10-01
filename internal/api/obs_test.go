package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/smazurov/pinquake/internal/ble"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/obs"
)

// fakeOBSConn is one OBS scene, "Main", holding a group "Overlays" (id 7).
type fakeOBSConn struct {
	mu      sync.Mutex
	toggles []bool
}

func (c *fakeOBSConn) Version() string          { return "32.2.2" }
func (c *fakeOBSConn) Changed() <-chan struct{} { return nil }
func (c *fakeOBSConn) Done() <-chan struct{}    { return nil }
func (c *fakeOBSConn) Ping() error              { return nil }
func (c *fakeOBSConn) Close() error             { return nil }

func (c *fakeOBSConn) Scenes() ([]obs.Scene, error) {
	return []obs.Scene{{Name: "Main", UUID: "scene-main", Items: []obs.Item{
		{ID: 7, Name: "Overlays", UUID: "src-overlays", IsGroup: true},
	}}}, nil
}

func (c *fakeOBSConn) SetItemEnabled(_ string, _ int, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.toggles = append(c.toggles, enabled)
	return nil
}

func newOBSTestServer(t *testing.T) (*Server, string, *fakeOBSConn, *[]string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	bus := events.New()
	conn := &fakeOBSConn{}
	var dialedWith []string
	s := NewServer(&Options{
		EventBus:   bus,
		Scanner:    ble.NewScanner(bus, slog.New(slog.DiscardHandler)),
		ConfigPath: path,
		OBSDial: func(server, password string) (obs.Conn, error) {
			dialedWith = append(dialedWith, server, password)
			return conn, nil
		},
	})
	t.Cleanup(func() {
		_ = s.Stop(t.Context())
		bus.Close()
	})
	return s, path, conn, &dialedWith
}

func call(t *testing.T, s *Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestOBSConnectPickTargetDisconnect(t *testing.T) {
	s, path, conn, dialedWith := newOBSTestServer(t)

	code, body := call(t, s, http.MethodPost, "/api/obs/connect", `{"server":" localhost:4455 ","password":"hunter2"}`)
	if code != http.StatusOK || body["state"] != "connected" {
		t.Fatalf("connect: %d %v", code, body)
	}
	if got := strings.Join(*dialedWith, ","); got != "localhost:4455,hunter2" {
		t.Errorf("dialed with %q", got)
	}

	code, body = call(t, s, http.MethodGet, "/api/obs/scenes", "")
	if code != http.StatusOK || len(body["scenes"].([]any)) != 1 {
		t.Fatalf("scenes: %d %v", code, body)
	}

	code, body = call(t, s, http.MethodPut, "/api/obs/target",
		`{"scene":"Main","scene_uuid":"scene-main","source":"Overlays","source_uuid":"src-overlays"}`)
	if code != http.StatusOK || body["state"] != "active" || body["target"] != "Main › Overlays" {
		t.Fatalf("target: %d %v", code, body)
	}
	// Trigger is hidden, so OBS hides the group and the overlays draw.
	if len(conn.toggles) != 1 || conn.toggles[0] {
		t.Errorf("toggles = %v, want [false]", conn.toggles)
	}
	if !s.overlay.Visible() {
		t.Error("overlays should stay up while OBS drives the target")
	}

	cfg, _ := data.LoadFromPath(path)
	want := data.OBSConfig{Server: "localhost:4455", Password: "hunter2", Connect: true,
		Scene: "Main", SceneUUID: "scene-main", Source: "Overlays", SourceUUID: "src-overlays"}
	if cfg.OBS != want {
		t.Errorf("saved %+v, want %+v", cfg.OBS, want)
	}

	code, body = call(t, s, http.MethodPost, "/api/obs/disconnect", "")
	if code != http.StatusOK || body["state"] != "off" {
		t.Fatalf("disconnect: %d %v", code, body)
	}
	if s.overlay.Visible() {
		t.Error("overlays should follow the (hidden) trigger again")
	}
	if len(conn.toggles) != 2 || !conn.toggles[1] {
		t.Errorf("toggles = %v, want group shown again on disconnect", conn.toggles)
	}
	cfg, _ = data.LoadFromPath(path)
	if cfg.OBS.Connect || cfg.OBS.Source != "Overlays" {
		t.Errorf("after disconnect saved %+v; want Connect=false, target kept", cfg.OBS)
	}

	code, _ = call(t, s, http.MethodGet, "/api/obs/scenes", "")
	if code != http.StatusConflict {
		t.Errorf("scenes while disconnected: %d, want 409", code)
	}
}

func TestOBSConnectRequiresServer(t *testing.T) {
	s, _, _, _ := newOBSTestServer(t)
	code, _ := call(t, s, http.MethodPost, "/api/obs/connect", `{"server":""}`)
	if code != http.StatusUnprocessableEntity {
		t.Errorf("empty server: %d, want 422", code)
	}
}
