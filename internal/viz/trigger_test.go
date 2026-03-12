package viz

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/smazurov/pinquake/internal/events"
)

const eventTimeout = 10 * time.Millisecond

func newTestTrigger(clk clockwork.Clock, cfg TriggerConfig) (*Trigger, *events.Bus) {
	bus := events.New()
	t := newTrigger(bus, cfg, clk)
	t.Start()
	return t, bus
}

func subChan[E any](bus *events.Bus) (<-chan E, func()) {
	ch := make(chan E, 16)
	unsub := bus.Subscribe(func(e E) {
		select {
		case ch <- e:
		default:
		}
	})
	return ch, unsub
}

func expectVizEvent(t *testing.T, ch <-chan events.VizTriggerEvent, visible bool) {
	t.Helper()
	select {
	case ev := <-ch:
		if ev.Visible != visible {
			t.Errorf("expected Visible=%v, got %v", visible, ev.Visible)
		}
	case <-time.After(eventTimeout):
		t.Fatalf("timed out waiting for VizTriggerEvent{Visible: %v}", visible)
	}
}

func expectNoVizEvent(t *testing.T, ch <-chan events.VizTriggerEvent) {
	t.Helper()
	select {
	case ev := <-ch:
		t.Fatalf("unexpected VizTriggerEvent: Visible=%v", ev.Visible)
	case <-time.After(eventTimeout):
	}
}

func TestTriggerFiresOnThreshold(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0, G: 0.0})
	expectVizEvent(t, ch, true)
}

func TestTriggerIgnoresBelowThreshold(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.1, FadeS: 5.0})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.01, Y: 0.01, G: 0.0})
	expectNoVizEvent(t, ch)
}

func TestTriggerUsesXYOnly(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.1, FadeS: 5.0})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.01, Y: 0.01, G: 1.0})
	expectNoVizEvent(t, ch)
}

func TestTriggerHidesAfterFade(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
	expectVizEvent(t, ch, true)

	clk.Advance(5 * time.Second)
	expectVizEvent(t, ch, false)
}

func TestTriggerRetriggersResetsFadeTimer(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
	expectVizEvent(t, ch, true)

	// Advance 3s, retrigger (resets fade timer)
	clk.Advance(3 * time.Second)
	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})

	// Wait for orientation event to be processed
	time.Sleep(5 * time.Millisecond)

	// Advance 3s more — only 3s since retrigger, should still be visible
	clk.Advance(3 * time.Second)
	if !tr.IsVisible() {
		t.Error("should still be visible after retrigger")
	}

	// Advance 2s more — 5s since retrigger, should hide
	clk.Advance(2 * time.Second)
	expectVizEvent(t, ch, false)
}

func TestDelayBufferDelaysEvents(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{DelayMs: 100, TriggerG: 0.02, FadeS: 5.0})
	defer tr.Stop()

	dch, unsub := subChan[events.DelayedOrientationEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.03, G: 0.01})

	// Wait for orientation event to be processed and buffered
	time.Sleep(5 * time.Millisecond)

	// Not yet drained
	tr.drainReady()
	select {
	case <-dch:
		t.Fatal("event should not drain before delay elapses")
	default:
	}

	// Advance past 100ms delay and drain
	clk.Advance(100 * time.Millisecond)
	tr.drainReady()

	select {
	case ev := <-dch:
		if ev.X != 0.05 || ev.Y != 0.03 {
			t.Errorf("unexpected values: x=%f y=%f", ev.X, ev.Y)
		}
	case <-time.After(eventTimeout):
		t.Fatal("timed out waiting for delayed event")
	}
}

func TestDelayZeroPassesThrough(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
	defer tr.Stop()

	dch, unsub := subChan[events.DelayedOrientationEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.01, Y: 0.01, G: 0.0})

	select {
	case ev := <-dch:
		if ev.X != 0.01 || ev.Y != 0.01 {
			t.Errorf("unexpected values: x=%f y=%f", ev.X, ev.Y)
		}
	case <-time.After(eventTimeout):
		t.Fatal("delayed event should arrive immediately with delay=0")
	}
}

func TestFadeNegativeOneAlwaysVisible(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.02, FadeS: -1})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
	expectVizEvent(t, ch, true)

	clk.Advance(10 * time.Second)

	if !tr.IsVisible() {
		t.Error("should still be visible with fadeS=-1")
	}
	expectNoVizEvent(t, ch)
}

func TestSetConfigFromAlwaysVisibleHides(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.02, FadeS: -1})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
	expectVizEvent(t, ch, true)

	tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
	clk.Advance(5 * time.Second)
	expectVizEvent(t, ch, false)
}

func TestSetConfigToAlwaysVisibleCancelsHideTimer(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
	expectVizEvent(t, ch, true)

	// Switch to always-visible before fade timer fires
	time.Sleep(5 * time.Millisecond)
	tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: -1})

	// Advance past the original 5s fade — should NOT hide
	clk.Advance(10 * time.Second)
	expectNoVizEvent(t, ch)

	if !tr.IsVisible() {
		t.Error("should remain visible after switching to always-visible")
	}
}

func TestSetConfigUpdatesLive(t *testing.T) {
	clk := clockwork.NewFakeClock()
	tr, bus := newTestTrigger(clk, TriggerConfig{TriggerG: 1.0, FadeS: 5.0})
	defer tr.Stop()

	ch, unsub := subChan[events.VizTriggerEvent](bus)
	defer unsub()

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
	expectNoVizEvent(t, ch)

	tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: 5.0})

	bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
	expectVizEvent(t, ch, true)
}
