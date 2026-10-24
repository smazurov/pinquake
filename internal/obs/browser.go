package obs

import (
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"strings"
)

// browserWarning checks only PinQuake browser sources affected by the target,
// including those inside groups or nested scenes. This is read-only: an input
// settings failure must not stop the visibility controller.
func browserWarning(scenes []Scene, target Target, settings func(Item) (map[string]any, error)) string {
	resolved, itemID, ok := Resolve(scenes, target)
	if !ok {
		return ""
	}

	containers := make(map[string]Scene, len(scenes))
	for _, scene := range scenes {
		containers[sourceKey(scene.UUID, scene.Name)] = scene
	}
	seen := make(map[string]bool)
	var warnings []string
	var visit func(Item)
	visit = func(item Item) {
		key := sourceKey(item.UUID, item.Name)
		if seen[key] {
			return
		}
		seen[key] = true
		if item.Kind != "browser_source" {
			if container, ok := containers[key]; ok {
				for _, child := range container.Items {
					visit(child)
				}
			}
			return
		}

		input, err := settings(item)
		if err != nil {
			slog.Warn("Could not check OBS browser settings", "source", item.Name, "error", err)
			return
		}
		address, _ := input["url"].(string)
		if !isPinQuakeURL(address) {
			return
		}
		var flags []string
		if input["shutdown"] == true {
			flags = append(flags, `"Shutdown source when not visible"`)
		}
		if input["restart_when_active"] == true {
			flags = append(flags, `"Refresh browser when scene becomes active"`)
		}
		if len(flags) > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: turn off %s in OBS to avoid reloading on each trigger.", item.Name, strings.Join(flags, " and ")))
		}
	}

	holder := containers[sourceKey(resolved.SceneUUID, resolved.Scene)]
	for _, item := range holder.Items {
		if item.ID == itemID {
			visit(item)
			break
		}
	}
	return strings.Join(warnings, "\n")
}

func sourceKey(uuid, name string) string {
	if uuid != "" {
		return "uuid:" + uuid
	}
	return "name:" + name
}

func isPinQuakeURL(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	switch u.Path {
	case "/canvas", "/crosshair", "/plumbbob", "/experiment":
		return true
	default:
		return false
	}
}

func mergeInputSettings(defaults, settings map[string]any) map[string]any {
	merged := make(map[string]any, len(defaults)+len(settings))
	maps.Copy(merged, defaults)
	maps.Copy(merged, settings)
	return merged
}
