package viz

import (
	"sync"
	"time"

	"github.com/smazurov/pinquake/internal/events"
)

// Overlay decides whether the browser overlays draw. They follow the
// trigger, except while OBS is actively driving a target: then they stay up
// and OBS shows and hides them. If OBS drops, they fall back to the trigger.
type Overlay struct {
	bus *events.Bus

	mu        sync.Mutex
	trigger   bool
	obsActive bool
	visible   bool
}

func NewOverlay(bus *events.Bus) *Overlay {
	return &Overlay{bus: bus}
}

func (o *Overlay) SetTrigger(visible bool) {
	o.update(func() { o.trigger = visible })
}

func (o *Overlay) SetOBSActive(active bool) {
	o.update(func() { o.obsActive = active })
}

func (o *Overlay) Visible() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.visible
}

// update publishes on change only; publishing under mu keeps events ordered
// (Bus.Publish never blocks).
func (o *Overlay) update(set func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	set()
	v := o.trigger || o.obsActive
	if v == o.visible {
		return
	}
	o.visible = v
	o.bus.Publish(events.OverlayVisibilityEvent{
		Visible:   v,
		Timestamp: time.Now().Format(time.RFC3339Nano),
	})
}
