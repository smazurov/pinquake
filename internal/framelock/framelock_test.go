package framelock

import (
	"testing"
	"time"
)

const sampleRate = 100 // Hz

var (
	t0       = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	vertical = [3]float32{1, 0, 0} // sensor strap-up: gravity on +X
)

func testConfig() Config {
	return Config{
		Enabled:         true,
		Window:          2 * time.Second,
		Threshold:       0.005,
		RelockThreshold: 0.01,
	}
}

// feeder drives a Locker with samples at a fixed rate and records lock events.
type feeder struct {
	t     *testing.T
	l     *Locker
	now   time.Time
	locks []Lock
	at    []time.Time
}

func newFeeder(t *testing.T, cfg Config) *feeder {
	t.Helper()
	return &feeder{t: t, l: New(cfg), now: t0}
}

// feed sends samples for duration d. gen returns the accel vector for sample i.
func (f *feeder) feed(d time.Duration, gen func(i int) [3]float32) {
	f.t.Helper()
	step := time.Second / sampleRate
	n := int(d / step)
	for i := 0; i < n; i++ {
		f.now = f.now.Add(step)
		if lock, ok := f.l.Feed(gen(i), f.now); ok {
			f.locks = append(f.locks, lock)
			f.at = append(f.at, f.now)
		}
	}
}

func constant(v [3]float32) func(int) [3]float32 {
	return func(int) [3]float32 { return v }
}

func TestStillSignalLocksOnceWindowIsStable(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)

	f.feed(cfg.Window+time.Second, constant(vertical))

	if len(f.locks) != 1 {
		t.Fatalf("expected exactly 1 lock, got %d", len(f.locks))
	}
	if f.locks[0].Reason != ReasonStable {
		t.Errorf("reason = %q, want %q", f.locks[0].Reason, ReasonStable)
	}
	if elapsed := f.at[0].Sub(t0); elapsed < cfg.Window {
		t.Errorf("locked after %v, before window %v elapsed", elapsed, cfg.Window)
	}
	frame, ok := f.l.Frame()
	if !ok {
		t.Fatal("expected a frame after lock")
	}
	if frame.Gravity != vertical {
		t.Errorf("gravity = %v, want %v", frame.Gravity, vertical)
	}
}

// jitter alternates ±amp on the Y axis around v.
func jitter(v [3]float32, amp float32) func(int) [3]float32 {
	return func(i int) [3]float32 {
		out := v
		if i%2 == 0 {
			out[1] += amp
		} else {
			out[1] -= amp
		}
		return out
	}
}

func TestNoisySignalDoesNotLockAsStable(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)

	// Y-axis noise with stdev 0.02g; magnitude barely changes, so this
	// also checks that horizontal motion is seen, not just |a|.
	f.feed(cfg.Window*3/2, jitter(vertical, 0.02))

	if len(f.locks) != 0 {
		t.Fatalf("expected no lock, got %v", f.locks)
	}
	if _, ok := f.l.Frame(); ok {
		t.Error("expected no frame")
	}
}

func TestLocksToWindowMeanAfterMovementStops(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)

	f.feed(cfg.Window/2, jitter(vertical, 0.05)) // being handled
	// Settled, with sub-threshold noise around a slightly tilted gravity.
	tilted := [3]float32{0.999, 0.02, 0}
	f.feed(cfg.Window+500*time.Millisecond, jitter(tilted, 0.002))

	if len(f.locks) != 1 {
		t.Fatalf("expected 1 lock after settling, got %d", len(f.locks))
	}
	frame, _ := f.l.Frame()
	for a := range 3 {
		if d := frame.Gravity[a] - tilted[a]; d > 1e-4 || d < -1e-4 {
			t.Errorf("gravity[%d] = %v, want mean %v (single sample would be off by 0.002)", a, frame.Gravity[a], tilted[a])
		}
	}
}

func TestRelocksOnceWhenSettledPositionDrifts(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)
	f.feed(cfg.Window+time.Second, constant(vertical))

	moved := [3]float32{0.9995, 0.03, 0} // ~1.7° tilt, offset 0.03g > relock 0.01g
	f.feed(time.Second, jitter(vertical, 0.05))
	f.feed(3*cfg.Window, constant(moved))

	if len(f.locks) != 2 {
		t.Fatalf("expected initial lock + 1 relock, got %d: %v", len(f.locks), f.locks)
	}
	if f.locks[1].Reason != ReasonDrift {
		t.Errorf("reason = %q, want %q", f.locks[1].Reason, ReasonDrift)
	}
	if f.locks[1].Drift < 0.029 || f.locks[1].Drift > 0.031 {
		t.Errorf("drift = %v, want ~0.03", f.locks[1].Drift)
	}
	if frame, _ := f.l.Frame(); frame.Gravity != moved {
		t.Errorf("gravity = %v, want %v", frame.Gravity, moved)
	}
}

func TestSubThresholdNoiseNeverRelocks(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)
	// Single samples deviate by 0.0045g, close to the stdev threshold; the old
	// single-sample drift check would have re-locked on these.
	f.feed(20*cfg.Window, jitter(vertical, 0.0045))

	if len(f.locks) != 1 {
		t.Fatalf("expected exactly 1 lock, got %d", len(f.locks))
	}
}

func TestNeverStableForcesLockAfterSettleTimeout(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)

	// Vibrating machine: stdev 0.01g never drops below the 0.005g threshold.
	f.feed(3*cfg.Window, jitter(vertical, 0.01))

	if len(f.locks) != 1 {
		t.Fatalf("expected 1 forced lock, got %d", len(f.locks))
	}
	lock := f.locks[0]
	if lock.Reason != ReasonSettleTimeout {
		t.Errorf("reason = %q, want %q", lock.Reason, ReasonSettleTimeout)
	}
	if lock.Stdev < 0.009 {
		t.Errorf("stdev = %v, want the observed ~0.01 so the log can explain the timeout", lock.Stdev)
	}
	if elapsed := f.at[0].Sub(t0); elapsed < 2*cfg.Window || elapsed > 2*cfg.Window+50*time.Millisecond {
		t.Errorf("forced after %v, want ~%v", elapsed, 2*cfg.Window)
	}
	frame, _ := f.l.Frame()
	if d := dist(frame.Gravity, vertical); d > 0.002 {
		t.Errorf("gravity %v is %vg from %v; want a short-window mean", frame.Gravity, d, vertical)
	}
}

func TestTriggerLocksToShortAverageOfFollowingSamples(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)
	f.feed(cfg.Window+time.Second, constant(vertical))

	moved := [3]float32{0.9995, 0.03, 0}
	f.l.Trigger()
	triggeredAt := f.now
	f.feed(time.Second, jitter(moved, 0.01)) // not stable, trigger must not wait

	if len(f.locks) != 2 {
		t.Fatalf("expected initial lock + trigger lock, got %d: %v", len(f.locks), f.locks)
	}
	if f.locks[1].Reason != ReasonTrigger {
		t.Errorf("reason = %q, want %q", f.locks[1].Reason, ReasonTrigger)
	}
	if d := f.at[1].Sub(triggeredAt); d < 450*time.Millisecond || d > 550*time.Millisecond {
		t.Errorf("trigger lock after %v, want ~500ms", d)
	}
	frame, _ := f.l.Frame()
	if d := dist(frame.Gravity, moved); d > 0.002 {
		t.Errorf("gravity %v is %vg from %v; want mean of post-trigger samples", frame.Gravity, d, moved)
	}
}

func TestEnableLocksQuicklyAndDisableClearsFrame(t *testing.T) {
	cfg := testConfig()
	cfg.Enabled = false
	f := newFeeder(t, cfg)

	f.feed(3*cfg.Window, constant(vertical))
	if len(f.locks) != 0 {
		t.Fatalf("disabled locker locked: %v", f.locks)
	}

	f.l.Enable()
	enabledAt := f.now
	f.feed(time.Second, constant(vertical))
	if len(f.locks) != 1 || f.locks[0].Reason != ReasonTrigger {
		t.Fatalf("expected one trigger lock after Enable, got %v", f.locks)
	}
	if d := f.at[0].Sub(enabledAt); d > 550*time.Millisecond {
		t.Errorf("Enable took %v to lock, want ~500ms", d)
	}

	f.l.Disable()
	if _, ok := f.l.Frame(); ok {
		t.Error("Disable should clear the frame")
	}
	f.feed(3*cfg.Window, constant(vertical))
	if len(f.locks) != 1 {
		t.Errorf("locked again while disabled: %v", f.locks)
	}
}

func TestReconnectRelocksWithoutUserAction(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)
	f.feed(cfg.Window+time.Second, constant(vertical))

	f.l.ResetFrame() // disconnect
	if _, ok := f.l.Frame(); ok {
		t.Fatal("ResetFrame should drop the frame")
	}
	f.now = f.now.Add(30 * time.Second) // offline gap
	reconnectedAt := f.now
	f.feed(cfg.Window+time.Second, constant(vertical))

	if len(f.locks) != 2 {
		t.Fatalf("expected a lock after reconnect, got %d locks", len(f.locks))
	}
	if f.locks[1].Reason != ReasonStable {
		t.Errorf("reason = %q, want %q", f.locks[1].Reason, ReasonStable)
	}
	// Pre-disconnect samples must not count toward the new window.
	if d := f.at[1].Sub(reconnectedAt); d < cfg.Window {
		t.Errorf("relocked %v after reconnect, before a full window", d)
	}
}

func TestDiscardSamplesRestartsSettling(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)

	f.feed(cfg.Window*3/2, jitter(vertical, 0.01))
	f.l.DiscardSamples() // sensor reconfigured; earlier data is not representative
	f.feed(cfg.Window*9/10, jitter(vertical, 0.01))

	// 2.4 windows total, but only 0.9 since the discard: settle timeout
	// (2 windows) must count from the discard.
	if len(f.locks) != 0 {
		t.Fatalf("expected no lock yet, got %v", f.locks)
	}
}

func TestDataGapRestartsWindow(t *testing.T) {
	cfg := testConfig()
	f := newFeeder(t, cfg)
	f.feed(cfg.Window/2, jitter(vertical, 0.05))
	f.now = f.now.Add(2 * cfg.Window) // notifications stalled
	resumedAt := f.now
	f.feed(cfg.Window+time.Second, constant(vertical))

	if len(f.locks) != 1 {
		t.Fatalf("expected 1 lock, got %d", len(f.locks))
	}
	if d := f.at[0].Sub(resumedAt); d < cfg.Window {
		t.Errorf("locked %v after resume from a handful of samples; want a full window", d)
	}
}

func TestStatusReportsLifecycle(t *testing.T) {
	cfg := testConfig()
	cfg.Enabled = false
	f := newFeeder(t, cfg)

	if s := f.l.Status(); s.Enabled || s.State != StateUnlocked {
		t.Errorf("initial status = %+v, want disabled/unlocked", s)
	}

	cfg.Enabled = true
	f.l.Configure(cfg) // e.g. saved config says enabled
	if s := f.l.Status(); !s.Enabled || s.State != StateSettling {
		t.Errorf("after enabling via config = %+v, want enabled/settling", s)
	}

	f.feed(cfg.Window/2, jitter(vertical, 0.01))
	if s := f.l.Status(); s.Stdev < 0.009 || s.Stdev > 0.011 {
		t.Errorf("settling stdev = %v, want ~0.01", s.Stdev)
	}

	f.feed(cfg.Window+time.Second, constant(vertical))
	s := f.l.Status()
	if s.State != StateLocked {
		t.Fatalf("after stable window state = %q, want %q", s.State, StateLocked)
	}
	if !s.LockedAt.Equal(f.at[0]) {
		t.Errorf("LockedAt = %v, want %v", s.LockedAt, f.at[0])
	}

	f.l.Configure(cfg) // unrelated config save must not disturb the lock
	if s := f.l.Status(); s.State != StateLocked {
		t.Errorf("re-applying same config changed state to %q", s.State)
	}

	cfg.Enabled = false
	f.l.Configure(cfg)
	if s := f.l.Status(); s.Enabled || s.State != StateUnlocked {
		t.Errorf("after disabling via config = %+v, want disabled/unlocked", s)
	}
}
