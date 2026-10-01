package ble

import (
	"encoding/binary"
	"io"
	"log/slog"
	"testing"
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

func newTestScanner(t *testing.T) (*Scanner, <-chan events.FrameStateEvent) {
	t.Helper()
	bus := events.New()
	ch := make(chan events.FrameStateEvent, 32)
	unsub := bus.Subscribe(func(e events.FrameStateEvent) { ch <- e })
	t.Cleanup(unsub)
	s := NewScanner(bus, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.ConfigureFrameLock(framelock.Config{
		Enabled:         true,
		Window:          100 * time.Millisecond,
		Threshold:       0.005,
		RelockThreshold: 0.01,
	})
	return s, ch
}

func stream(handler func([]byte), d time.Duration) {
	pkt := v1Packet(1, 0, 0)
	for end := time.Now().Add(d); time.Now().Before(end); {
		handler(pkt)
		time.Sleep(2 * time.Millisecond)
	}
}

func waitForReason(t *testing.T, ch <-chan events.FrameStateEvent, reason framelock.Reason) {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		select {
		case e := <-ch:
			if e.Reason == string(reason) {
				return
			}
		case <-timeout:
			t.Fatalf("no frame-state event with reason %q", reason)
		}
	}
}

func TestAutoLocksOnConnectAndAgainAfterReconnect(t *testing.T) {
	s, ch := newTestScanner(t)
	handler := s.makeNotificationHandler()

	stream(handler, 250*time.Millisecond)
	waitForReason(t, ch, framelock.ReasonStable)
	if st := s.FrameStatus(); st.State != framelock.StateLocked {
		t.Fatalf("state = %q after stable stream, want locked", st.State)
	}

	// Connection lost: same path as user disconnect and D-Bus "lost".
	s.mu.Lock()
	s.resetConnectionState()
	s.mu.Unlock()
	if st := s.FrameStatus(); !st.Enabled || st.State != framelock.StateSettling {
		t.Fatalf("after disconnect status = %+v, want enabled/settling", st)
	}

	handler = s.makeNotificationHandler() // new connection, new handler
	stream(handler, 250*time.Millisecond)
	waitForReason(t, ch, framelock.ReasonStable)
	if st := s.FrameStatus(); st.State != framelock.StateLocked {
		t.Fatalf("state = %q after reconnect, want locked", st.State)
	}
}
