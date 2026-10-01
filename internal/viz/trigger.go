package viz

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smazurov/pinquake/internal/events"
)

const (
	ringSize     = 1024
	drainTickMs  = 2
	defaultClass = "impact"
)

type TriggerConfig struct {
	DelayMs  int
	TriggerG float64
	FadeS    float64
}

type bufferedEvent struct {
	x, y, g    float32
	receivedAt time.Time
}

type Trigger struct {
	bus *events.Bus

	mu       sync.Mutex
	triggerG float64
	fadeDur  time.Duration
	delayDur time.Duration

	// visible is written under mu (so show/hide events publish in order)
	// and read lock-free by IsVisible.
	visible   atomic.Bool
	hideTimer *time.Timer
	hideGen   uint64 // bumped whenever the pending hide is re-armed or cancelled

	ring    [ringSize]bufferedEvent
	ringW   int
	ringR   int
	ringLen int

	sentZero bool

	unsub  func()
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewTrigger(bus *events.Bus, cfg TriggerConfig) *Trigger {
	return &Trigger{
		bus:      bus,
		triggerG: cfg.TriggerG,
		fadeDur:  time.Duration(cfg.FadeS * float64(time.Second)),
		delayDur: time.Duration(cfg.DelayMs) * time.Millisecond,
		stopCh:   make(chan struct{}),
	}
}

func (t *Trigger) Start() {
	t.unsub = t.bus.Subscribe(func(e events.OrientationEvent) {
		t.onOrientation(e)
	})

	t.wg.Add(1)
	go t.drainLoop()

	t.mu.Lock()
	if t.fadeDur < 0 {
		t.setVisible(true)
	}
	t.mu.Unlock()
}

func (t *Trigger) Stop() {
	if t.unsub != nil {
		t.unsub()
	}
	close(t.stopCh)
	t.wg.Wait()

	t.mu.Lock()
	t.cancelHide()
	t.mu.Unlock()
}

func (t *Trigger) IsVisible() bool {
	return t.visible.Load()
}

func (t *Trigger) SetConfig(cfg TriggerConfig) {
	fadeDur := time.Duration(cfg.FadeS * float64(time.Second))

	t.mu.Lock()
	t.triggerG = cfg.TriggerG
	t.delayDur = time.Duration(cfg.DelayMs) * time.Millisecond

	// Only a fade change restarts the countdown; tweaking other settings
	// while the viz is up must not extend it.
	if fadeDur != t.fadeDur {
		t.fadeDur = fadeDur
		if t.visible.Load() {
			t.armHide()
		}
	}
	// Always-visible means visible now, not just "never hide once shown".
	if fadeDur < 0 {
		t.setVisible(true)
	}
	t.mu.Unlock()
}

func (t *Trigger) onOrientation(e events.OrientationEvent) {
	mag := math.Sqrt(float64(e.X)*float64(e.X) + float64(e.Y)*float64(e.Y))

	t.mu.Lock()
	threshold := t.triggerG
	delayDur := t.delayDur
	t.mu.Unlock()

	if mag > threshold {
		t.show()
	}

	nearZero := e.X*e.X+e.Y*e.Y+e.G*e.G <= 1e-4
	if delayDur == 0 {
		if !nearZero {
			t.sentZero = false
			t.bus.Publish(events.DelayedOrientationEvent(e))
		} else if !t.sentZero {
			t.sentZero = true
			t.bus.Publish(events.DelayedOrientationEvent(e))
		}
	} else {
		t.pushRing(bufferedEvent{
			x:          e.X,
			y:          e.Y,
			g:          e.G,
			receivedAt: time.Now(),
		})
	}
}

// show makes the viz visible and restarts the fade countdown.
func (t *Trigger) show() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.armHide()
	t.setVisible(true)
}

// armHide replaces any pending hide with one fadeDur from now, or none when
// always visible. Caller holds mu.
func (t *Trigger) armHide() {
	t.cancelHide()
	if t.fadeDur < 0 {
		return
	}
	gen := t.hideGen
	t.hideTimer = time.AfterFunc(t.fadeDur, func() { t.expire(gen) })
}

// cancelHide drops the pending hide. Bumping hideGen also neutralizes a
// timer whose callback already started and is waiting on mu. Caller holds mu.
func (t *Trigger) cancelHide() {
	t.hideGen++
	if t.hideTimer != nil {
		t.hideTimer.Stop()
		t.hideTimer = nil
	}
}

func (t *Trigger) expire(gen uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if gen != t.hideGen {
		return
	}
	t.hideTimer = nil
	t.setVisible(false)
}

// setVisible publishes on change only. Caller holds mu; Bus.Publish never
// blocks, and publishing under mu keeps show/hide events in order.
func (t *Trigger) setVisible(v bool) {
	if t.visible.Load() == v {
		return
	}
	t.visible.Store(v)
	t.bus.Publish(events.VizTriggerEvent{
		Visible:   v,
		Class:     defaultClass,
		Timestamp: time.Now().Format(time.RFC3339Nano),
	})
}

func (t *Trigger) pushRing(ev bufferedEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ringLen == ringSize {
		t.ringR = (t.ringR + 1) % ringSize
		t.ringLen--
	}
	t.ring[t.ringW] = ev
	t.ringW = (t.ringW + 1) % ringSize
	t.ringLen++
}

func (t *Trigger) drainLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(drainTickMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.drainReady()
		}
	}
}

func (t *Trigger) drainReady() {
	now := time.Now()
	t.mu.Lock()
	delayDur := t.delayDur
	for t.ringLen > 0 {
		ev := t.ring[t.ringR]
		if now.Sub(ev.receivedAt) < delayDur {
			break
		}
		t.ringR = (t.ringR + 1) % ringSize
		t.ringLen--
		t.mu.Unlock()

		nearZero := ev.x*ev.x+ev.y*ev.y+ev.g*ev.g <= 1e-4
		if !nearZero {
			t.sentZero = false
			t.bus.Publish(events.DelayedOrientationEvent{
				X:         ev.x,
				Y:         ev.y,
				G:         ev.g,
				Timestamp: now.Format(time.RFC3339Nano),
			})
		} else if !t.sentZero {
			t.sentZero = true
			t.bus.Publish(events.DelayedOrientationEvent{
				X:         ev.x,
				Y:         ev.y,
				G:         ev.g,
				Timestamp: now.Format(time.RFC3339Nano),
			})
		}

		t.mu.Lock()
	}
	t.mu.Unlock()
}
