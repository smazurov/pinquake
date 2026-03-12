package viz

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonboulle/clockwork"
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
	bus   *events.Bus
	clock clockwork.Clock

	mu       sync.Mutex
	triggerG float64
	fadeDur  time.Duration
	delayDur time.Duration

	visible   atomic.Bool
	hideTimer clockwork.Timer

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
	return newTrigger(bus, cfg, clockwork.NewRealClock())
}

func newTrigger(bus *events.Bus, cfg TriggerConfig, clk clockwork.Clock) *Trigger {
	return &Trigger{
		bus:      bus,
		clock:    clk,
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
}

func (t *Trigger) Stop() {
	if t.unsub != nil {
		t.unsub()
	}
	close(t.stopCh)
	t.wg.Wait()

	t.mu.Lock()
	if t.hideTimer != nil {
		t.hideTimer.Stop()
	}
	t.mu.Unlock()
}

func (t *Trigger) IsVisible() bool {
	return t.visible.Load()
}

func (t *Trigger) SetConfig(cfg TriggerConfig) {
	t.mu.Lock()
	t.triggerG = cfg.TriggerG
	t.fadeDur = time.Duration(cfg.FadeS * float64(time.Second))
	t.delayDur = time.Duration(cfg.DelayMs) * time.Millisecond

	if t.hideTimer != nil {
		t.hideTimer.Stop()
		t.hideTimer = nil
	}
	if t.fadeDur >= 0 && t.visible.Load() {
		t.hideTimer = t.clock.AfterFunc(t.fadeDur, t.publishHide)
	}
	t.mu.Unlock()
}

func (t *Trigger) publishHide() {
	t.visible.Store(false)
	t.bus.Publish(events.VizTriggerEvent{
		Visible:   false,
		Class:     defaultClass,
		Timestamp: t.clock.Now().Format(time.RFC3339Nano),
	})
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
			receivedAt: t.clock.Now(),
		})
	}
}

func (t *Trigger) show() {
	t.mu.Lock()
	fadeDur := t.fadeDur
	wasVisible := t.visible.Load()

	if t.hideTimer != nil {
		t.hideTimer.Stop()
		t.hideTimer = nil
	}
	if fadeDur >= 0 {
		t.hideTimer = t.clock.AfterFunc(fadeDur, t.publishHide)
	}
	t.mu.Unlock()

	if !wasVisible {
		t.visible.Store(true)
		t.bus.Publish(events.VizTriggerEvent{
			Visible:   true,
			Class:     defaultClass,
			Timestamp: t.clock.Now().Format(time.RFC3339Nano),
		})
	}
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
	ticker := t.clock.NewTicker(drainTickMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.Chan():
			t.drainReady()
		}
	}
}

func (t *Trigger) drainReady() {
	now := t.clock.Now()
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
