package ble

import (
	"fmt"
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
		t := orientation.CurrentInReferenceFrame(ori, s.refFrame)
		if abs32(t.Ax) <= eps && abs32(t.Ay) <= eps && abs32(t.Az) <= eps {
			s.autoLockSince = time.Time{}
			return false
		}
	}

	ref := *ori
	s.refFrame = &ref
	s.autoLockSince = time.Time{}
	return true
}

func (s *Scanner) makeNotificationHandler() func([]byte) {
	return func(buf []byte) {
		ori, ok := orientation.DecodeV1(buf)
		if !ok {
			return
		}

		var autoLockFired bool

		s.mu.Lock()
		if s.pendingLock {
			ref := ori
			s.refFrame = &ref
			s.pendingLock = false
		}
		if s.autoLockEnabled {
			autoLockFired = s.checkAutoLock(&ori)
		}
		ref := s.refFrame
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

		if ref != nil {
			ori = orientation.CurrentInReferenceFrame(&ori, ref)
		}

		gx := ori.Ax
		gy := ori.Ay
		gz := ori.Az
		if swap {
			gx, gy = gy, gx
		}

		s.eventBus.Publish(events.OrientationEvent{
			Gx:        gx,
			Gy:        gy,
			Gz:        gz,
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
	}
}
