// Package framelock decides when to lock the gravity reference frame.
package framelock

import (
	"math"
	"time"

	"github.com/smazurov/pinquake/internal/orientation"
)

type Config struct {
	Enabled         bool
	Window          time.Duration
	Threshold       float32
	RelockThreshold float32
}

type Reason string

const (
	ReasonStable Reason = "stable"
	ReasonDrift  Reason = "drift"
	// ReasonSettleTimeout: no stable window within settleTimeout, so the
	// frame was locked to a short average anyway. Drift re-lock corrects it
	// once the sensor does settle.
	ReasonSettleTimeout Reason = "settle-timeout"
	// ReasonTrigger: explicit force-lock (Trigger or Enable).
	ReasonTrigger Reason = "trigger"
)

// forceWindow is the averaging span for locks that don't wait for stability.
const forceWindow = 500 * time.Millisecond

type Lock struct {
	Reason Reason
	Stdev  float32
	Drift  float32 // distance (g) from the previous gravity; 0 for first lock
}

type State string

const (
	StateUnlocked State = "unlocked" // no frame, not trying to lock
	StateSettling State = "settling" // waiting for a stable window or trigger
	StateLocked   State = "locked"
)

type Status struct {
	Enabled  bool
	State    State
	LockedAt time.Time // zero unless locked
	Stdev    float32   // max per-axis stdev over buffered samples (g)
}

type Frame struct {
	Gravity  [3]float32
	Rotation orientation.Mat3
}

type sample struct {
	v [3]float32
	t time.Time
}

// Locker is not safe for concurrent use; the caller synchronizes.
type Locker struct {
	cfg      Config
	samples  []sample
	since    time.Time // first sample of the current contiguous collection
	frame    *Frame
	lockedAt time.Time

	triggered    bool
	triggerSince time.Time // first sample after Trigger; zero until then
}

func New(cfg Config) *Locker {
	return &Locker{cfg: cfg}
}

// Configure applies new settings. Re-applying the same settings is a no-op;
// turning Enabled off drops the frame, turning it on starts settling.
func (l *Locker) Configure(cfg Config) {
	wasEnabled := l.cfg.Enabled
	l.cfg = cfg
	if wasEnabled && !cfg.Enabled {
		l.Disable()
	}
}

func (l *Locker) Status() Status {
	st := Status{Enabled: l.cfg.Enabled, State: StateUnlocked}
	switch {
	case l.frame != nil:
		st.State = StateLocked
		st.LockedAt = l.lockedAt
	case l.cfg.Enabled || l.triggered:
		st.State = StateSettling
	}
	if len(l.samples) >= 2 {
		_, st.Stdev = stats(l.samples)
	}
	return st
}

// Enable turns on auto-lock and force-locks to the next short average, so a
// user pressing "lock" sees it take effect within forceWindow.
func (l *Locker) Enable() {
	l.cfg.Enabled = true
	l.Trigger()
}

// Disable turns off auto-lock and drops the frame.
func (l *Locker) Disable() {
	l.cfg.Enabled = false
	l.frame = nil
	l.triggered = false
	l.samples = l.samples[:0]
}

// Trigger force-locks to the average of the next forceWindow of samples,
// regardless of stability.
func (l *Locker) Trigger() {
	l.triggered = true
	l.triggerSince = time.Time{}
}

// ResetFrame drops the frame and all samples but keeps auto-lock enabled,
// so the next connection re-locks on its own. Call on disconnect.
func (l *Locker) ResetFrame() {
	l.frame = nil
	l.triggered = false
	l.DiscardSamples()
}

// DiscardSamples restarts stability evaluation (and the settle timeout)
// while keeping the current frame. Call after the sensor is reconfigured.
func (l *Locker) DiscardSamples() {
	l.samples = l.samples[:0]
	l.triggerSince = time.Time{}
}

func (l *Locker) Feed(v [3]float32, now time.Time) (Lock, bool) {
	if !l.cfg.Enabled && !l.triggered {
		return Lock{}, false
	}
	// Prune first: after a gap longer than the window, coverage restarts.
	l.prune(now)
	if len(l.samples) == 0 {
		l.since = now
	}
	l.samples = append(l.samples, sample{v: v, t: now})

	if l.triggered {
		if l.triggerSince.IsZero() {
			l.triggerSince = now
		}
		if now.Sub(l.triggerSince) < forceWindow {
			return Lock{}, false
		}
		mean, stdev := stats(l.samplesSince(l.triggerSince))
		l.triggered = false
		l.lockTo(mean, now)
		return Lock{Reason: ReasonTrigger, Stdev: stdev}, true
	}

	if now.Sub(l.since) < l.cfg.Window {
		return Lock{}, false
	}
	mean, stdev := stats(l.samples)
	if stdev >= l.cfg.Threshold {
		if l.frame == nil && now.Sub(l.since) >= l.settleTimeout() {
			l.lockTo(l.recentMean(now), now)
			return Lock{Reason: ReasonSettleTimeout, Stdev: stdev}, true
		}
		return Lock{}, false
	}
	if l.frame == nil {
		l.lockTo(mean, now)
		return Lock{Reason: ReasonStable, Stdev: stdev}, true
	}
	drift := dist(mean, l.frame.Gravity)
	if drift <= l.cfg.RelockThreshold {
		return Lock{}, false
	}
	l.lockTo(mean, now)
	return Lock{Reason: ReasonDrift, Stdev: stdev, Drift: drift}, true
}

// SettleTimeout is how long an unlocked Locker waits for a stable window
// before force-locking.
func (c Config) SettleTimeout() time.Duration { return 2 * c.Window }

func (l *Locker) settleTimeout() time.Duration { return l.cfg.SettleTimeout() }

// recentMean averages samples from the last forceWindow.
func (l *Locker) recentMean(now time.Time) [3]float32 {
	mean, _ := stats(l.samplesSince(now.Add(-forceWindow)))
	return mean
}

// samplesSince returns the buffered samples at or after t (at least one).
func (l *Locker) samplesSince(t time.Time) []sample {
	i := len(l.samples) - 1
	for i > 0 && !l.samples[i-1].t.Before(t) {
		i--
	}
	return l.samples[i:]
}

func dist(a, b [3]float32) float32 {
	dx, dy, dz := float64(a[0]-b[0]), float64(a[1]-b[1]), float64(a[2]-b[2])
	return float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
}

// stats returns the per-axis mean and the largest per-axis stdev.
// Two-pass for numerical stability (|g| ~ 1 vs. stdev ~ 1e-3).
func stats(samples []sample) ([3]float32, float32) {
	n := float64(len(samples))
	var sum [3]float64
	for _, s := range samples {
		for a := range 3 {
			sum[a] += float64(s.v[a])
		}
	}
	var mean [3]float64
	for a := range 3 {
		mean[a] = sum[a] / n
	}
	var sq [3]float64
	for _, s := range samples {
		for a := range 3 {
			d := float64(s.v[a]) - mean[a]
			sq[a] += d * d
		}
	}
	var maxVar float64
	for a := range 3 {
		maxVar = max(maxVar, sq[a]/n)
	}
	return [3]float32{float32(mean[0]), float32(mean[1]), float32(mean[2])}, float32(math.Sqrt(maxVar))
}

// prune drops samples older than the window. Coverage is tracked by l.since,
// not by the oldest surviving sample (which is always within the window).
func (l *Locker) prune(now time.Time) {
	cutoff := now.Add(-max(l.cfg.Window, forceWindow))
	i := 0
	for i < len(l.samples) && l.samples[i].t.Before(cutoff) {
		i++
	}
	if i > 0 {
		l.samples = append(l.samples[:0], l.samples[i:]...)
	}
}

func (l *Locker) lockTo(g [3]float32, now time.Time) {
	l.lockedAt = now
	l.frame = &Frame{
		Gravity:  g,
		Rotation: orientation.BuildLockRotation(&orientation.Orientation{Ax: g[0], Ay: g[1], Az: g[2]}),
	}
}

func (l *Locker) Frame() (Frame, bool) {
	if l.frame == nil {
		return Frame{}, false
	}
	return *l.frame, true
}
