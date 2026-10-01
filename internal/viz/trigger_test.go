package viz

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/smazurov/pinquake/internal/events"
)

// Tests run in synctest bubbles: time is virtual (time.Sleep advances it
// instantly) and synctest.Wait returns once the trigger and bus goroutines
// have processed everything, so no wall-clock waits are needed.

// newTestTrigger starts a trigger and stops it (and its bus) at test end,
// so the bubble can verify no goroutines leak.
func newTestTrigger(t *testing.T, cfg TriggerConfig) (*Trigger, *events.Bus) {
	t.Helper()
	bus := events.New()
	tr := NewTrigger(bus, cfg)
	tr.Start()
	t.Cleanup(func() {
		tr.Stop()
		bus.Close()
	})
	return tr, bus
}

func subChan[E any](bus *events.Bus) <-chan E {
	ch := make(chan E, 16)
	bus.Subscribe(func(e E) {
		select {
		case ch <- e:
		default:
		}
	})
	return ch
}

func expectVizEvent(t *testing.T, ch <-chan events.VizTriggerEvent, visible bool) {
	t.Helper()
	synctest.Wait()
	select {
	case ev := <-ch:
		if ev.Visible != visible {
			t.Errorf("expected Visible=%v, got %v", visible, ev.Visible)
		}
	default:
		t.Fatalf("no VizTriggerEvent{Visible: %v}", visible)
	}
}

func expectNoVizEvent(t *testing.T, ch <-chan events.VizTriggerEvent) {
	t.Helper()
	synctest.Wait()
	select {
	case ev := <-ch:
		t.Fatalf("unexpected VizTriggerEvent: Visible=%v", ev.Visible)
	default:
	}
}

// advance moves virtual time and lets timers and goroutines react.
func advance(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func TestTriggerFiresOnThreshold(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0, G: 0.0})
		expectVizEvent(t, ch, true)
	})
}

func TestTriggerIgnoresBelowThreshold(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.1, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.01, Y: 0.01, G: 0.0})
		expectNoVizEvent(t, ch)
	})
}

func TestTriggerUsesXYOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.1, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.01, Y: 0.01, G: 1.0})
		expectNoVizEvent(t, ch)
	})
}

func TestTriggerHidesAfterFade(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
		expectVizEvent(t, ch, true)

		advance(5 * time.Second)
		expectVizEvent(t, ch, false)
	})
}

func TestTriggerRetriggersResetsFadeTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
		expectVizEvent(t, ch, true)

		advance(3 * time.Second)
		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0}) // retrigger resets fade
		synctest.Wait()

		advance(3 * time.Second) // 3s since retrigger
		if !tr.IsVisible() {
			t.Error("should still be visible after retrigger")
		}
		expectNoVizEvent(t, ch)

		advance(2 * time.Second) // 5s since retrigger
		expectVizEvent(t, ch, false)
	})
}

func TestDelayBufferDelaysEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, bus := newTestTrigger(t, TriggerConfig{DelayMs: 100, TriggerG: 0.02, FadeS: 5.0})
		dch := subChan[events.DelayedOrientationEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.03, G: 0.01})
		advance(99 * time.Millisecond)
		select {
		case <-dch:
			t.Fatal("event drained before the 100ms delay elapsed")
		default:
		}

		advance(drainTickMs * time.Millisecond * 2) // past 100ms + a drain tick
		select {
		case ev := <-dch:
			if ev.X != 0.05 || ev.Y != 0.03 {
				t.Errorf("unexpected values: x=%f y=%f", ev.X, ev.Y)
			}
		default:
			t.Fatal("delayed event not delivered after the delay")
		}
	})
}

func TestDelayZeroPassesThrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		dch := subChan[events.DelayedOrientationEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.01, Y: 0.01, G: 0.0})
		synctest.Wait()
		select {
		case ev := <-dch:
			if ev.X != 0.01 || ev.Y != 0.01 {
				t.Errorf("unexpected values: x=%f y=%f", ev.X, ev.Y)
			}
		default:
			t.Fatal("delayed event should arrive immediately with delay=0")
		}
	})
}

func TestFadeNegativeOneAlwaysVisible(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: -1})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
		advance(10 * time.Second)
		if !tr.IsVisible() {
			t.Error("should still be visible with fadeS=-1")
		}
		expectNoVizEvent(t, ch)
	})
}

func TestAlwaysVisibleShowsOnStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := events.New()
		ch := subChan[events.VizTriggerEvent](bus)
		tr := NewTrigger(bus, TriggerConfig{TriggerG: 0.02, FadeS: -1})
		tr.Start()
		t.Cleanup(func() {
			tr.Stop()
			bus.Close()
		})

		expectVizEvent(t, ch, true)
		if !tr.IsVisible() {
			t.Error("always-visible should be visible before any trigger")
		}
	})
}

func TestSetConfigFromAlwaysVisibleHides(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: -1})
		ch := subChan[events.VizTriggerEvent](bus)

		tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		advance(5 * time.Second)
		expectVizEvent(t, ch, false)
	})
}

func TestSetConfigBackToAlwaysVisibleShows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: -1})
		ch := subChan[events.VizTriggerEvent](bus)

		tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		advance(5 * time.Second)
		expectVizEvent(t, ch, false)

		tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: -1})
		expectVizEvent(t, ch, true)
		if !tr.IsVisible() {
			t.Error("switching back to always-visible should show the viz")
		}
	})
}

func TestSetConfigToAlwaysVisibleCancelsHideTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
		expectVizEvent(t, ch, true)

		tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: -1}) // before the fade fires
		advance(10 * time.Second)
		expectNoVizEvent(t, ch)
		if !tr.IsVisible() {
			t.Error("should remain visible after switching to always-visible")
		}
	})
}

func TestSetConfigKeepsFadeDeadlineWhenFadeUnchanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr, bus := newTestTrigger(t, TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
		expectVizEvent(t, ch, true)

		advance(3 * time.Second)
		tr.SetConfig(TriggerConfig{TriggerG: 0.5, DelayMs: 50, FadeS: 5.0})

		advance(2 * time.Second) // 5s since the trigger
		expectVizEvent(t, ch, false)
	})
}

func TestSetConfigUpdatesLive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr, bus := newTestTrigger(t, TriggerConfig{TriggerG: 1.0, FadeS: 5.0})
		ch := subChan[events.VizTriggerEvent](bus)

		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
		expectNoVizEvent(t, ch)

		tr.SetConfig(TriggerConfig{TriggerG: 0.02, FadeS: 5.0})
		bus.Publish(events.OrientationEvent{X: 0.05, Y: 0.0})
		expectVizEvent(t, ch, true)
	})
}
