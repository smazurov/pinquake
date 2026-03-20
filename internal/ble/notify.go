package ble

import (
	"fmt"
	"math"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/orientation"
)

func accelMag(ax, ay, az float32) float32 {
	return float32(math.Sqrt(float64(ax*ax + ay*ay + az*az)))
}

// checkAutoLock evaluates stability via stdev of accel magnitude over a sliding
// window and locks the reference frame when stable. Caller must hold s.mu.
// Returns true if a new lock was applied.
func (s *Scanner) checkAutoLock(ori *orientation.Orientation) bool {
	now := time.Now()
	mag := accelMag(ori.Ax, ori.Ay, ori.Az)
	s.autoLockSamples = append(s.autoLockSamples, accelSample{mag: mag, t: now})

	// Prune samples outside the window.
	cutoff := now.Add(-s.autoLockSpreadWindow)
	i := 0
	for i < len(s.autoLockSamples) && s.autoLockSamples[i].t.Before(cutoff) {
		i++
	}
	if i > 0 {
		s.autoLockSamples = append(s.autoLockSamples[:0], s.autoLockSamples[i:]...)
	}

	// Need samples spanning the full window before evaluating.
	if len(s.autoLockSamples) < 2 || now.Sub(s.autoLockSamples[0].t) < s.autoLockSpreadWindow {
		return false
	}

	// Compute stdev of magnitudes.
	var sum, sumSq float64
	n := float64(len(s.autoLockSamples))
	for _, sample := range s.autoLockSamples {
		v := float64(sample.mag)
		sum += v
		sumSq += v * v
	}
	variance := sumSq/n - (sum/n)*(sum/n)
	if variance < 0 {
		variance = 0
	}
	stdev := float32(math.Sqrt(variance))

	if stdev >= s.autoLockSpreadThreshold {
		return false
	}

	// If already locked, check if we've drifted from the locked frame.
	if s.refFrame != nil {
		t := orientation.InReferenceFrame(ori, s.gravity, s.refRotation)
		drift := accelMag(t.Ax, t.Ay, t.Az)
		if drift <= s.autoLockSpreadThreshold {
			return false
		}
	}

	s.lockTo(ori)
	s.autoLockSamples = s.autoLockSamples[:0]
	return true
}

// lockTo sets the reference frame from the given orientation. Caller must hold s.mu.
func (s *Scanner) lockTo(ori *orientation.Orientation) {
	ref := *ori
	s.refFrame = &ref
	s.gravity = [3]float32{ori.Ax, ori.Ay, ori.Az}
	s.gMag = accelMag(ori.Ax, ori.Ay, ori.Az)
	s.refRotation = orientation.BuildLockRotation(&ref)
}

func (s *Scanner) makeNotificationHandler() func([]byte) {
	return func(buf []byte) {
		ori, ok := orientation.DecodeV1(buf)
		if !ok {
			return
		}

		var autoLockFired bool

		s.mu.Lock()
		raw := ori
		s.lastRaw = &raw
		if s.pendingLock {
			s.lockTo(&ori)
			s.pendingLock = false
		}
		if s.autoLockEnabled {
			autoLockFired = s.checkAutoLock(&ori)
		}
		ref := s.refFrame
		rot := s.refRotation
		gravity := s.gravity
		swap := s.swapXY
		s.mu.Unlock()

		if autoLockFired {
			s.eventBus.Publish(events.LogEntry{
				Message:   fmt.Sprintf("Auto-locked: stdev below %.4fg", s.autoLockSpreadThreshold),
				Level:     "info",
				Timestamp: time.Now().Format(time.RFC3339Nano),
			})
		}

		g := accelMag(raw.Ax, raw.Ay, raw.Az)

		if ref != nil {
			ori = orientation.InReferenceFrame(&ori, gravity, rot)
		} else {
			curRot := orientation.BuildLockRotation(&raw)
			ori.Ax, ori.Ay, ori.Az = orientation.ApplyMat3(curRot, raw.Ax, raw.Ay, raw.Az)
		}

		x := ori.Ax
		y := ori.Ay
		if swap {
			x, y = y, x
		}

		s.eventBus.Publish(events.OrientationEvent{
			X:         x,
			Y:         y,
			G:         g,
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
	}
}
