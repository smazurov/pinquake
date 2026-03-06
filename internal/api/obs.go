package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/pinquake/internal/obs"
)

type OBSStateResponse struct {
	Body struct {
		Status string `json:"status" example:"connected"`
	}
}

type OBSConnectRequest struct {
	Body struct {
		Host     string `json:"host" doc:"OBS WebSocket host" example:"localhost"`
		Port     int    `json:"port" doc:"OBS WebSocket port" example:"4455"`
		Password string `json:"password" doc:"OBS WebSocket password"`
	}
}

type OBSActionResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

type OBSSourcesResponse struct {
	Body []OBSSource
}

type OBSSource struct {
	SceneName   string `json:"scene_name"`
	SourceName  string `json:"source_name"`
	URL         string `json:"url"`
	SceneItemID int    `json:"scene_item_id"`
	Enabled     bool   `json:"enabled"`
}

type OBSToggleRequest struct {
	Body struct {
		Enabled bool `json:"enabled" doc:"Whether to show or hide the source"`
	}
}

func (s *Server) registerOBSRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "obs-state",
		Method:      http.MethodGet,
		Path:        "/api/obs/state",
		Summary:     "Get OBS connection state",
		Tags:        []string{"obs"},
	}, func(_ context.Context, _ *struct{}) (*OBSStateResponse, error) {
		resp := &OBSStateResponse{}
		resp.Body.Status = string(s.obs.GetState())
		return resp, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "obs-connect",
		Method:      http.MethodPost,
		Path:        "/api/obs/connect",
		Summary:     "Connect to OBS",
		Tags:        []string{"obs"},
	}, func(_ context.Context, input *OBSConnectRequest) (*OBSActionResponse, error) {
		if err := s.obs.Connect(input.Body.Host, input.Body.Port, input.Body.Password); err != nil {
			if errors.Is(err, obs.ErrAuthFailed) {
				return nil, huma.Error401Unauthorized(err.Error())
			}
			if errors.Is(err, obs.ErrTimeout) {
				return nil, huma.Error504GatewayTimeout(err.Error())
			}
			return nil, huma.Error502BadGateway(err.Error())
		}
		return &OBSActionResponse{Body: struct {
			OK bool `json:"ok"`
		}{OK: true}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "obs-disconnect",
		Method:      http.MethodPost,
		Path:        "/api/obs/disconnect",
		Summary:     "Disconnect from OBS",
		Tags:        []string{"obs"},
	}, func(_ context.Context, _ *struct{}) (*OBSActionResponse, error) {
		s.obs.Disconnect()
		return &OBSActionResponse{Body: struct {
			OK bool `json:"ok"`
		}{OK: true}}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "obs-sources",
		Method:      http.MethodGet,
		Path:        "/api/obs/sources",
		Summary:     "List OBS browser sources",
		Tags:        []string{"obs"},
	}, func(_ context.Context, _ *struct{}) (*OBSSourcesResponse, error) {
		sources, err := s.obs.ListBrowserSources()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to list sources: %v", err))
		}
		result := make([]OBSSource, len(sources))
		for i, src := range sources {
			result[i] = OBSSource{
				SceneName:   src.SceneName,
				SourceName:  src.SourceName,
				URL:         src.URL,
				SceneItemID: src.SceneItemID,
				Enabled:     src.Enabled,
			}
		}
		return &OBSSourcesResponse{Body: result}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "obs-toggle",
		Method:      http.MethodPost,
		Path:        "/api/obs/toggle",
		Summary:     "Toggle OBS browser source visibility",
		Tags:        []string{"obs"},
	}, func(_ context.Context, input *OBSToggleRequest) (*OBSActionResponse, error) {
		cfg, err := s.loadAppConfig()
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to load config")
		}
		if cfg.OBS.SceneName == "" || cfg.OBS.SourceName == "" {
			return nil, huma.Error400BadRequest("no OBS source configured")
		}
		if err := s.obs.SetSourceEnabled(cfg.OBS.SceneName, cfg.OBS.SourceName, input.Body.Enabled); err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to toggle source: %v", err))
		}
		return &OBSActionResponse{Body: struct {
			OK bool `json:"ok"`
		}{OK: true}}, nil
	})
}
