package obs

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBrowserWarningSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		want     []string
	}{
		{"correct", map[string]any{"shutdown": false, "restart_when_active": false}, nil},
		{"unset flags", map[string]any{}, nil},
		{"shutdown", map[string]any{"shutdown": true}, []string{"Waveform", "Shutdown source when not visible"}},
		{"refresh", map[string]any{"restart_when_active": true}, []string{"Waveform", "Refresh browser when scene becomes active"}},
		{"both", map[string]any{"shutdown": true, "restart_when_active": true}, []string{"Shutdown source when not visible", "Refresh browser when scene becomes active"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.settings["url"] = "http://streaming-pc:8091/canvas?width=608&height=1080"
			got := browserWarning(testScenes(), overlays, func(Item) (map[string]any, error) { return tt.settings, nil })
			if tt.want == nil && got != "" {
				t.Fatalf("warning = %q, want none", got)
			}
			for _, text := range tt.want {
				if !strings.Contains(got, text) {
					t.Errorf("warning = %q, want %q", got, text)
				}
			}
		})
	}
}

func TestBrowserWarningOnlyChecksSelectedSubtree(t *testing.T) {
	scenes := testScenes()
	scenes[1].Items = append(scenes[1].Items, Item{ID: 4, Name: "Nested", UUID: "nested-scene"})
	scenes = append(scenes, Scene{Name: "Nested", UUID: "nested-scene", Items: []Item{
		{ID: 5, Name: "Crosshair", UUID: "src-crosshair", Kind: "browser_source"},
		{ID: 6, Name: "Duplicate waveform", UUID: "src-waveform", Kind: "browser_source"},
		{ID: 7, Name: "Cycle", UUID: "src-overlays", IsGroup: true},
	}})
	scenes[2].Items = append(scenes[2].Items, Item{ID: 8, Name: "Unrelated", UUID: "src-unrelated", Kind: "browser_source"})
	var checked []string
	warning := browserWarning(scenes, overlays, func(item Item) (map[string]any, error) {
		checked = append(checked, item.UUID)
		return map[string]any{"url": "https://overlay-host/crosshair", "shutdown": true}, nil
	})
	if !reflect.DeepEqual(checked, []string{"src-waveform", "src-crosshair"}) {
		t.Errorf("checked = %v, want only unique browser sources under the target", checked)
	}
	if strings.Count(warning, "Shutdown source when not visible") != 2 {
		t.Errorf("warning = %q, want two browser warnings", warning)
	}
}

func TestBrowserWarningDirectTargetAndOtherURLs(t *testing.T) {
	target := Target{Scene: "Overlays", Source: "Waveform"}
	for _, path := range []string{"/canvas", "/crosshair", "/plumbbob", "/experiment"} {
		t.Run(path, func(t *testing.T) {
			warning := browserWarning(testScenes(), target, func(Item) (map[string]any, error) {
				return map[string]any{"url": "http://localhost:8091" + path, "shutdown": true}, nil
			})
			if warning == "" {
				t.Error("expected warning for a directly selected PinQuake overlay")
			}
		})
	}
	for _, url := range []string{"https://other-app/scoreboard", "about:blank", "file:///canvas", "not a URL", "http://localhost:8091/"} {
		t.Run(url, func(t *testing.T) {
			warning := browserWarning(testScenes(), target, func(Item) (map[string]any, error) {
				return map[string]any{"url": url, "shutdown": true}, nil
			})
			if warning != "" {
				t.Errorf("warning = %q for a non-PinQuake URL", warning)
			}
		})
	}
}

func TestBrowserWarningSkipsMissingTargetsAndReadErrors(t *testing.T) {
	if got := browserWarning(testScenes(), Target{}, func(Item) (map[string]any, error) {
		t.Fatal("should not read settings without a selected target")
		return nil, nil
	}); got != "" {
		t.Errorf("warning = %q without a target", got)
	}
	if got := browserWarning(testScenes(), overlays, func(Item) (map[string]any, error) {
		return nil, errors.New("input disappeared")
	}); got != "" {
		t.Errorf("warning = %q without a confirmed misconfiguration", got)
	}
}

func TestMergeInputSettingsIncludesDefaultsAndExplicitOverrides(t *testing.T) {
	defaults := map[string]any{"url": "about:blank", "shutdown": true, "restart_when_active": false}
	settings := map[string]any{"url": "http://localhost:8091/canvas", "shutdown": false}
	got := mergeInputSettings(defaults, settings)
	if got["url"] != settings["url"] || got["shutdown"] != false || got["restart_when_active"] != false {
		t.Errorf("merged settings = %v", got)
	}
	if defaults["shutdown"] != true || defaults["url"] != "about:blank" {
		t.Error("merging settings mutated OBS defaults")
	}
}
