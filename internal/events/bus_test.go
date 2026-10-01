package events

import (
	"sync"
	"testing"
	"testing/synctest"
)

// All tests run in synctest bubbles: Test fails if any subscriber goroutine
// is still alive at the end, so leaks are caught without extra assertions.

type recorder[T any] struct {
	mu  sync.Mutex
	got []T
}

func (r *recorder[T]) add(v T) {
	r.mu.Lock()
	r.got = append(r.got, v)
	r.mu.Unlock()
}

func (r *recorder[T]) snapshot() []T {
	synctest.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]T(nil), r.got...)
}

func TestSubscriberGetsItsTypeInOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := New()
		defer bus.Close()
		var logs recorder[LogEntry]
		bus.Subscribe(logs.add)

		bus.Publish(LogEntry{Message: "a"})
		bus.Publish(HeartbeatEvent{}) // other type
		bus.Publish(LogEntry{Message: "b"})

		got := logs.snapshot()
		if len(got) != 2 || got[0].Message != "a" || got[1].Message != "b" {
			t.Fatalf("got %+v, want [a b]", got)
		}
	})
}

func TestUnsubscribeStopsDeliveryAndGoroutine(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := New()
		var logs recorder[LogEntry]
		unsub := bus.Subscribe(logs.add)

		bus.Publish(LogEntry{Message: "a"})
		logs.snapshot()
		unsub()
		bus.Publish(LogEntry{Message: "b"})

		if got := logs.snapshot(); len(got) != 1 {
			t.Fatalf("got %d events after unsubscribe, want 1", len(got))
		}
		// No Close: the bubble only ends cleanly if unsub stopped the goroutine.
	})
}

func TestCloseStopsAllSubscribers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := New()
		var logs recorder[LogEntry]
		var beats recorder[HeartbeatEvent]
		bus.Subscribe(logs.add)
		bus.Subscribe(beats.add)

		bus.Close()
		bus.Publish(LogEntry{Message: "late"})
		bus.Subscribe(logs.add) // after Close: no-op, no goroutine

		if got := logs.snapshot(); len(got) != 0 {
			t.Fatalf("delivered after Close: %+v", got)
		}
	})
}

func TestSlowSubscriberDoesNotBlockPublisherAndKeepsNewest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := New()
		defer bus.Close()
		release := make(chan struct{})
		var logs recorder[LogEntry]
		bus.Subscribe(func(e LogEntry) {
			<-release // stuck handler
			logs.add(e)
		})

		bus.Publish(LogEntry{Message: "first"}) // handler takes it and blocks
		synctest.Wait()
		for i := 0; i < maxQueue+10; i++ { // must not block
			bus.Publish(LogEntry{Message: "flood"})
		}
		bus.Publish(LogEntry{Message: "newest"})
		close(release)

		got := logs.snapshot()
		if len(got) != maxQueue+1 { // "first" + a full queue
			t.Fatalf("delivered %d, want %d (queue bounded)", len(got), maxQueue+1)
		}
		if last := got[len(got)-1].Message; last != "newest" {
			t.Errorf("last delivered %q, want newest (drop oldest when full)", last)
		}
	})
}

func TestSubscribeToChannelDropsWhenFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := New()
		defer bus.Close()
		ch := make(chan any, 1)
		SubscribeToChannel[LogEntry](bus, ch)

		bus.Publish(LogEntry{Message: "a"})
		bus.Publish(LogEntry{Message: "b"}) // channel full: dropped, not blocked
		synctest.Wait()

		if e := (<-ch).(LogEntry); e.Message != "a" {
			t.Fatalf("got %q, want a", e.Message)
		}
		select {
		case e := <-ch:
			t.Fatalf("unexpected second event %+v", e)
		default:
		}
	})
}
