package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/pinquake/internal/data"
	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/obs"
)

type OBSConnectRequest struct {
	Body struct {
		Server   string `json:"server" minLength:"1" doc:"obs-websocket address (host:port)" example:"localhost:4455"`
		Password string `json:"password,omitempty" doc:"obs-websocket password; leave empty if authentication is off"`
	}
}

type OBSTargetRequest struct {
	Body obs.Target
}

type OBSStatusResponse struct {
	Body events.OBSStatusEvent
}

type OBSScenesBody struct {
	Scenes []obs.Scene `json:"scenes" doc:"Scenes and groups, top of the OBS list first"`
}

type OBSScenesResponse struct {
	Body OBSScenesBody
}

func (s *Server) registerOBSRoutes() {
	grp := huma.NewGroup(s.api, "/api/obs")
	grp.UseSimpleModifier(huma.OperationTags("obs"))

	huma.Get(grp, "/status", func(_ context.Context, _ *struct{}) (*OBSStatusResponse, error) {
		return &OBSStatusResponse{Body: obsStatusEvent(s.obs.Status())}, nil
	})

	huma.Get(grp, "/scenes", func(_ context.Context, _ *struct{}) (*OBSScenesResponse, error) {
		scenes, err := s.obs.Scenes()
		if errors.Is(err, obs.ErrNotConnected) {
			return nil, huma.Error409Conflict(err.Error())
		}
		if err != nil {
			return nil, huma.Error502BadGateway(fmt.Sprintf("OBS: %v", err))
		}
		if scenes == nil {
			scenes = []obs.Scene{}
		}
		return &OBSScenesResponse{Body: OBSScenesBody{Scenes: scenes}}, nil
	})

	huma.Post(grp, "/connect", func(_ context.Context, in *OBSConnectRequest) (*OBSStatusResponse, error) {
		server := strings.TrimSpace(in.Body.Server)
		cfg, err := s.updateOBSConfig(func(c *data.OBSConfig) {
			c.Server, c.Password, c.Connect = server, in.Body.Password, true
		})
		if err != nil {
			return nil, err
		}
		s.obs.SetTarget(obsTarget(cfg))
		s.obs.Connect(server, in.Body.Password)
		return &OBSStatusResponse{Body: obsStatusEvent(s.obs.Status())}, nil
	})

	huma.Post(grp, "/disconnect", func(_ context.Context, _ *struct{}) (*OBSStatusResponse, error) {
		if _, err := s.updateOBSConfig(func(c *data.OBSConfig) { c.Connect = false }); err != nil {
			return nil, err
		}
		s.obs.Disconnect()
		return &OBSStatusResponse{Body: obsStatusEvent(s.obs.Status())}, nil
	})

	huma.Put(grp, "/target", func(_ context.Context, in *OBSTargetRequest) (*OBSStatusResponse, error) {
		t := in.Body
		if _, err := s.updateOBSConfig(func(c *data.OBSConfig) { setOBSTarget(c, t) }); err != nil {
			return nil, err
		}
		s.obs.SetTarget(t)
		return &OBSStatusResponse{Body: obsStatusEvent(s.obs.Status())}, nil
	})
}

// updateOBSConfig edits [app.obs] in place. It releases configMu before
// returning: callers go on to call the OBS controller, whose OnTarget
// callback takes configMu.
func (s *Server) updateOBSConfig(edit func(*data.OBSConfig)) (data.OBSConfig, error) {
	s.configMu.Lock()
	cfg, _ := s.loadAppConfig()
	edit(&cfg.OBS)
	err := data.SaveSection(s.configPath, "obs", cfg.OBS)
	s.configMu.Unlock()
	if err != nil {
		return cfg.OBS, huma.Error500InternalServerError(fmt.Sprintf("failed to save config: %v", err))
	}
	s.eventBus.Publish(events.ConfigChangedEvent{
		Section:   "obs",
		Timestamp: time.Now().Format(time.RFC3339Nano),
	})
	return cfg.OBS, nil
}

// saveRenamedOBSTarget persists a target renamed in OBS. Runs on the OBS
// controller goroutine.
func (s *Server) saveRenamedOBSTarget(t obs.Target) {
	if _, err := s.updateOBSConfig(func(c *data.OBSConfig) { setOBSTarget(c, t) }); err != nil {
		s.log("error", fmt.Sprintf("Failed to save renamed OBS target: %v", err))
	}
}

// onOBSStatus runs on the OBS controller goroutine, in status order.
func (s *Server) onOBSStatus(st obs.Status) {
	s.overlay.SetOBSActive(st.State == obs.StateActive)
	s.eventBus.Publish(obsStatusEvent(st))

	prev := s.lastOBS
	s.lastOBS = st
	switch st.State {
	case obs.StateActive:
		if prev.State != obs.StateActive || prev.Target != st.Target {
			s.log("info", fmt.Sprintf("OBS driving %s", st.Target.Label()))
		}
	case obs.StateConnected:
		switch {
		case st.TargetMissing && (!prev.TargetMissing || prev.Target != st.Target):
			s.log("warn", fmt.Sprintf("OBS target %s not found; overlays hide themselves until it's back", st.Target.Label()))
		case !st.TargetMissing && prev.State != obs.StateConnected:
			s.log("info", fmt.Sprintf("Connected to OBS %s; pick a source to drive", st.Version))
		}
	case obs.StateWaiting:
		// Retries repeat the same error every backoff; log it once.
		if st.Error != s.lastOBSErr {
			s.log("warn", fmt.Sprintf("OBS at %s: %s; retrying", st.Server, st.Error))
			s.lastOBSErr = st.Error
		}
	case obs.StateOff:
		if prev.State != obs.StateOff {
			s.log("info", "Disconnected from OBS")
		}
	}
	if st.State != obs.StateWaiting && st.State != obs.StateConnecting {
		s.lastOBSErr = ""
	}
}

func obsTarget(c data.OBSConfig) obs.Target {
	return obs.Target{Scene: c.Scene, SceneUUID: c.SceneUUID, Source: c.Source, SourceUUID: c.SourceUUID}
}

func setOBSTarget(c *data.OBSConfig, t obs.Target) {
	c.Scene, c.SceneUUID, c.Source, c.SourceUUID = t.Scene, t.SceneUUID, t.Source, t.SourceUUID
}

func obsStatusEvent(st obs.Status) events.OBSStatusEvent {
	return events.OBSStatusEvent{
		State:         string(st.State),
		Server:        st.Server,
		OBSVersion:    st.Version,
		Error:         st.Error,
		RetryInS:      st.RetryIn.Seconds(),
		Target:        st.Target.Label(),
		TargetMissing: st.TargetMissing,
		Timestamp:     time.Now().Format(time.RFC3339Nano),
	}
}
