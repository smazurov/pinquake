package obs

import "testing"

func testScenes() []Scene {
	return []Scene{
		{Name: "Main", UUID: "scene-main", Items: []Item{
			{ID: 7, Name: "Overlays", UUID: "src-overlays", IsGroup: true},
			{ID: 1, Name: "Camera", UUID: "src-camera", Kind: "v4l2_input"},
		}},
		{Name: "Overlays", UUID: "src-overlays", IsGroup: true, Items: []Item{
			{ID: 2, Name: "Waveform", UUID: "src-waveform", Kind: "browser_source"},
		}},
		{Name: "BRB", UUID: "scene-brb", Items: []Item{
			{ID: 3, Name: "Camera", UUID: "src-camera", Kind: "v4l2_input"},
		}},
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name   string
		target Target
		want   Target
		wantID int
		wantOK bool
	}{
		{
			name:   "group in a scene",
			target: Target{Scene: "Main", SceneUUID: "scene-main", Source: "Overlays", SourceUUID: "src-overlays"},
			want:   Target{Scene: "Main", SceneUUID: "scene-main", Source: "Overlays", SourceUUID: "src-overlays"},
			wantID: 7, wantOK: true,
		},
		{
			name:   "source inside a group",
			target: Target{Scene: "Overlays", SceneUUID: "src-overlays", Source: "Waveform", SourceUUID: "src-waveform"},
			want:   Target{Scene: "Overlays", SceneUUID: "src-overlays", Source: "Waveform", SourceUUID: "src-waveform"},
			wantID: 2, wantOK: true,
		},
		{
			name:   "same source in two scenes picks the right scene",
			target: Target{Scene: "BRB", SceneUUID: "scene-brb", Source: "Camera", SourceUUID: "src-camera"},
			want:   Target{Scene: "BRB", SceneUUID: "scene-brb", Source: "Camera", SourceUUID: "src-camera"},
			wantID: 3, wantOK: true,
		},
		{
			name:   "renamed in OBS: UUIDs win and names refresh",
			target: Target{Scene: "Old Main", SceneUUID: "scene-main", Source: "Old Camera", SourceUUID: "src-camera"},
			want:   Target{Scene: "Main", SceneUUID: "scene-main", Source: "Camera", SourceUUID: "src-camera"},
			wantID: 1, wantOK: true,
		},
		{
			name:   "UUIDs gone (re-imported collection): names match, UUIDs refresh",
			target: Target{Scene: "Main", SceneUUID: "stale", Source: "Camera", SourceUUID: "stale"},
			want:   Target{Scene: "Main", SceneUUID: "scene-main", Source: "Camera", SourceUUID: "src-camera"},
			wantID: 1, wantOK: true,
		},
		{
			name:   "hand-written config with names only",
			target: Target{Scene: "Overlays", Source: "Waveform"},
			want:   Target{Scene: "Overlays", SceneUUID: "src-overlays", Source: "Waveform", SourceUUID: "src-waveform"},
			wantID: 2, wantOK: true,
		},
		{
			name:   "missing",
			target: Target{Scene: "Main", Source: "Nope"},
		},
		{
			name: "empty target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, id, ok := Resolve(testScenes(), tt.target)
			if ok != tt.wantOK || id != tt.wantID || got != tt.want {
				t.Errorf("Resolve() = %+v, %d, %v; want %+v, %d, %v", got, id, ok, tt.want, tt.wantID, tt.wantOK)
			}
		})
	}
}
