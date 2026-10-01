package ble

import (
	"encoding/binary"
	"io"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/framelock"
)

// v1Packet encodes accel (g) as a WT901 V1 notification (±16g full scale).
func v1Packet(ax, ay, az float32) []byte {
	buf := make([]byte, 20)
	buf[0], buf[1] = 0x55, 0x61
	for i, v := range []float32{ax, ay, az} {
		binary.LittleEndian.PutUint16(buf[2+2*i:], uint16(int16(v/16*32768)))
	}
	return buf
}

const lockWindow = 5 * time.Second

func newTestScanner() *Scanner {
	s := NewScanner(events.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.ConfigureFrameLock(framelock.Config{
		Enabled:         true,
		Window:          lockWindow,
		Threshold:       0.005,
		RelockThreshold: 0.01,
	})
	return s
}

// stream feeds a still sensor at 200 Hz for d of (virtual) time.
func stream(handler func([]byte), d time.Duration) {
	pkt := v1Packet(1, 0, 0)
	for end := time.Now().Add(d); time.Now().Before(end); {
		handler(pkt)
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAutoLocksOnConnectAndAgainAfterReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestScanner()
		handler := s.makeNotificationHandler()

		stream(handler, lockWindow+time.Second)
		if st := s.FrameStatus(); st.State != framelock.StateLocked {
			t.Fatalf("state = %q after a stable window, want locked", st.State)
		}

		// Link ended (lost, forgotten or switched): the supervisor calls onUnlink.
		s.onUnlink()
		stream(handler, lockWindow+time.Second) // stale packets from the old link
		if st := s.FrameStatus(); st.State == framelock.StateLocked {
			t.Fatal("stale notifications from the old link re-locked the frame")
		}
		if st := s.FrameStatus(); !st.Enabled || st.State != framelock.StateSettling {
			t.Fatalf("after disconnect status = %+v, want enabled/settling", st)
		}

		handler = s.makeNotificationHandler() // new connection, new handler
		stream(handler, lockWindow+time.Second)
		if st := s.FrameStatus(); st.State != framelock.StateLocked {
			t.Fatalf("state = %q after reconnect, want locked", st.State)
		}
	})
}
