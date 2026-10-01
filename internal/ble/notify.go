package ble

import (
	"math"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/orientation"
)

func accelMag(ax, ay, az float32) float32 {
	return float32(math.Sqrt(float64(ax*ax + ay*ay + az*az)))
}

// makeNotificationHandler returns a handler bound to the current session;
// it ignores packets once that session has ended.
func (s *Scanner) makeNotificationHandler() func([]byte) {
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()
	return func(buf []byte) {
		ori, ok := orientation.DecodeV1(buf)
		if !ok {
			return
		}
		raw := ori
		now := time.Now()

		s.mu.Lock()
		if s.session != session {
			s.mu.Unlock()
			return
		}
		lock, locked := s.locker.Feed([3]float32{raw.Ax, raw.Ay, raw.Az}, now)
		frame, hasFrame := s.locker.Frame()
		swap := s.swapXY
		s.mu.Unlock()

		if locked {
			s.publishFrameState(&lock)
		}

		g := accelMag(raw.Ax, raw.Ay, raw.Az)

		if hasFrame {
			ori = orientation.InReferenceFrame(&ori, frame.Gravity, frame.Rotation)
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
			Timestamp: now.Format(time.RFC3339Nano),
		})
	}
}
