package viz

import (
	"testing"
	"testing/synctest"

	"github.com/smazurov/pinquake/internal/events"
)

func TestOverlayVisibility(t *testing.T) {
	tests := []struct {
		trigger, obsActive, want bool
	}{
		{trigger: false, obsActive: false, want: false},
		{trigger: true, obsActive: false, want: true}, // direct: follows the trigger
		{trigger: false, obsActive: true, want: true}, // OBS hides the source instead
		{trigger: true, obsActive: true, want: true},
	}
	for _, tt := range tests {
		o := NewOverlay(events.New())
		o.SetTrigger(tt.trigger)
		o.SetOBSActive(tt.obsActive)
		if got := o.Visible(); got != tt.want {
			t.Errorf("trigger=%v obsActive=%v: Visible() = %v, want %v", tt.trigger, tt.obsActive, got, tt.want)
		}
	}
}

func TestOverlayFallsBackToTriggerWhenOBSDrops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := events.New()
		t.Cleanup(bus.Close)
		ch := subChan[events.OverlayVisibilityEvent](bus)
		o := NewOverlay(bus)

		o.SetOBSActive(true)
		o.SetTrigger(true)  // already visible: no event
		o.SetTrigger(false) // OBS still active: no event
		o.SetOBSActive(false)

		synctest.Wait()
		var got []bool
		for len(ch) > 0 {
			got = append(got, (<-ch).Visible)
		}
		if len(got) != 2 || !got[0] || got[1] {
			t.Errorf("events = %v, want [true false]", got)
		}
	})
}
