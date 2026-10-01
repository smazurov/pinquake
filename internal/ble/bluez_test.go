package ble

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// syncBuffer is a log sink safe for concurrent writes and reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	synctest.Wait()
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type reapHarness struct {
	logs        *syncBuffer
	logger      *slog.Logger
	ch          chan tinygoConnect
	mu          sync.Mutex
	disconnects int
}

func newReapHarness() *reapHarness {
	logs := &syncBuffer{}
	return &reapHarness{
		logs:   logs,
		logger: slog.New(slog.NewTextHandler(logs, nil)),
		ch:     make(chan tinygoConnect, 1),
	}
}

func (h *reapHarness) disconnect() error {
	h.mu.Lock()
	h.disconnects++
	h.mu.Unlock()
	return nil
}

func (h *reapHarness) disconnectCount() int {
	synctest.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.disconnects
}

func (h *reapHarness) start() {
	go reapAbandonedConnect(h.logger, tracker, h.ch, h.disconnect)
}

func TestAbandonedConnectReturningPromptlyIsQuiet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newReapHarness()
		h.start()
		h.ch <- tinygoConnect{} // connected just after we gave up

		if n := h.disconnectCount(); n != 1 {
			t.Errorf("disconnects = %d, want 1 (late connection must be dropped)", n)
		}
		if logs := h.logs.String(); logs != "" {
			t.Errorf("unexpected logs for a prompt return:\n%s", logs)
		}
	})
}

func TestAbandonedConnectStuckIsLoggedAsLeak(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newReapHarness()
		h.start()

		time.Sleep(dbusTimeout - time.Millisecond)
		if logs := h.logs.String(); logs != "" {
			t.Fatalf("warned before dbusTimeout:\n%s", logs)
		}
		time.Sleep(time.Millisecond)
		logs := h.logs.String()
		if !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, "leaked") || !strings.Contains(logs, tracker) {
			t.Fatalf("want a WARN about the leaked goroutine for %s, got:\n%s", tracker, logs)
		}

		time.Sleep(time.Hour) // warned once, not repeatedly
		if n := strings.Count(h.logs.String(), "level=WARN"); n != 1 {
			t.Errorf("%d WARN lines, want 1", n)
		}

		h.ch <- tinygoConnect{err: errors.New("le-connection-abort-by-local")}
		logs = h.logs.String()
		if !strings.Contains(logs, "level=INFO") || !strings.Contains(logs, "released") {
			t.Errorf("want an INFO when the leaked goroutine is released, got:\n%s", logs)
		}
		if n := h.disconnectCount(); n != 0 {
			t.Errorf("disconnects = %d, want 0 (connect failed)", n)
		}
	})
}
