// Package obs drives an OBS scene item from the viz trigger over
// obs-websocket v5.
package obs

// Target is the scene item the trigger shows and hides: Source inside Scene.
// Scene may be a real scene or a group; OBS toggles items in either the same
// way. UUIDs survive renames; names are the fallback when the UUIDs no longer
// exist (e.g. a re-imported scene collection).
type Target struct {
	Scene      string `json:"scene" doc:"Scene or group holding the target"`
	SceneUUID  string `json:"scene_uuid"`
	Source     string `json:"source" doc:"Source or group to show and hide"`
	SourceUUID string `json:"source_uuid"`
}

func (t Target) IsZero() bool { return t.Source == "" && t.SourceUUID == "" }

func (t Target) Label() string {
	if t.IsZero() {
		return ""
	}
	return t.Scene + " › " + t.Source
}

// Scene is a scene or group and the items directly inside it. Groups are
// listed as their own Scene (IsGroup) in addition to appearing as an item of
// the scene that contains them.
type Scene struct {
	Name    string `json:"name"`
	UUID    string `json:"uuid"`
	IsGroup bool   `json:"is_group"`
	Items   []Item `json:"items" doc:"Top of the OBS source list first"`
}

type Item struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	UUID    string `json:"uuid"`
	Kind    string `json:"kind" doc:"OBS input kind; empty for groups and nested scenes"`
	IsGroup bool   `json:"is_group"`
	Enabled bool   `json:"enabled"`
}

// Resolve finds t in scenes, by UUIDs first and then by names. The returned
// Target carries the current names and UUIDs, so a rename in OBS shows up as
// a difference from t.
func Resolve(scenes []Scene, t Target) (resolved Target, itemID int, ok bool) {
	if t.IsZero() {
		return Target{}, 0, false
	}
	byUUID := func(s Scene, it Item) bool {
		return t.SceneUUID != "" && t.SourceUUID != "" && s.UUID == t.SceneUUID && it.UUID == t.SourceUUID
	}
	byName := func(s Scene, it Item) bool {
		return s.Name == t.Scene && it.Name == t.Source
	}
	for _, match := range []func(Scene, Item) bool{byUUID, byName} {
		for _, s := range scenes {
			for _, it := range s.Items {
				if match(s, it) {
					return Target{Scene: s.Name, SceneUUID: s.UUID, Source: it.Name, SourceUUID: it.UUID}, it.ID, true
				}
			}
		}
	}
	return Target{}, 0, false
}
