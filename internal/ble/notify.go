package ble

import (
	"fmt"
	"math"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/orientation"
)

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func accelMag(ax, ay, az float32) float32 {
	return float32(math.Sqrt(float64(ax*ax + ay*ay + az*az)))
}

// checkAutoLock evaluates stability and locks the reference frame when stable.
// Caller must hold s.mu. Returns true if a new lock was applied.
func (s *Scanner) checkAutoLock(ori *orientation.Orientation) bool {
	raw := [3]float32{ori.Ax, ori.Ay, ori.Az}
	eps := s.autoLockEpsilon
	timeout := s.autoLockTimeout

	if abs32(raw[0]-s.autoLockRef[0]) > eps ||
		abs32(raw[1]-s.autoLockRef[1]) > eps ||
		abs32(raw[2]-s.autoLockRef[2]) > eps {
		s.autoLockRef = raw
		s.autoLockSince = time.Now()
	} else if s.autoLockSince.IsZero() {
		s.autoLockSince = time.Now()
	}

	if s.autoLockSince.IsZero() || time.Since(s.autoLockSince) < timeout {
		return false
	}

	if s.refFrame != nil {
		t := orientation.InReferenceFrame(ori, s.gravity, s.refRotation)
		if abs32(t.Ax) <= eps && abs32(t.Ay) <= eps && abs32(t.Az) <= eps {
			s.autoLockSince = time.Time{}
			return false
		}
	}

	s.lockTo(ori)
	s.autoLockSince = time.Time{}
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
		timeout := s.autoLockTimeout
		s.mu.Unlock()

		if autoLockFired {
			s.eventBus.Publish(events.LogEntry{
				Message:   fmt.Sprintf("Auto-locked: stable for %ds", int(timeout.Seconds())),
				Level:     "info",
				Timestamp: time.Now().Format(time.RFC3339Nano),
			})
		}

		g := accelMag(raw.Ax, raw.Ay, raw.Az)

		if ref != nil {
			ori = orientation.InReferenceFrame(&ori, gravity, rot)
		} else {
			// No locked reference: rotate raw accel into a per-frame canonical
			// frame (gravity → z) without subtracting gravity. The x/y
			// components capture horizontal nudge (~0 at rest); directions
			// are unstable but magnitudes are meaningful.
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
