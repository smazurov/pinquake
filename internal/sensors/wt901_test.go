package sensors

import (
	"encoding/binary"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeChar stands in for the WT901 write characteristic. It answers
// register reads like the device, and can hang a write (a stuck D-Bus call).
type fakeChar struct {
	w *WT901

	mu     sync.Mutex
	writes int
	hang   chan struct{} // next write blocks until closed
}

func (f *fakeChar) WriteWithoutResponse(p []byte) (int, error) {
	f.mu.Lock()
	f.writes++
	hang := f.hang
	f.hang = nil
	f.mu.Unlock()
	if hang != nil {
		<-hang
	}
	if len(p) == 5 && p[2] == 0x27 { // read register: reply on the notify channel
		resp := make([]byte, 20)
		resp[0], resp[1], resp[2] = 0x55, 0x71, p[3]
		binary.LittleEndian.PutUint16(resp[4:], 395) // 3.95 V
		f.w.respCh <- resp
	}
	return len(p), nil
}

func (f *fakeChar) hangNext() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hang = make(chan struct{})
	return f.hang
}

func (f *fakeChar) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

func newTestWT901() (*WT901, *fakeChar) {
	w := NewWT901().(*WT901)
	fc := &fakeChar{w: w}
	w.writeChar = fc
	w.respCh = make(chan []byte, 8)
	return w, fc
}

func TestReadBattery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, _ := newTestWT901()
		bat, err := w.ReadBattery()
		if err != nil {
			t.Fatalf("ReadBattery: %v", err)
		}
		if bat.Volts != 3.95 {
			t.Errorf("volts = %v, want 3.95", bat.Volts)
		}
	})
}

func TestHungWriteTimesOutWithoutInterleaving(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, fc := newTestWT901()
		hang := fc.hangNext()

		start := time.Now()
		if _, err := w.ReadBattery(); err == nil {
			t.Fatal("ReadBattery succeeded with a hung write")
		}
		if took := time.Since(start); took > ioTimeout {
			t.Errorf("caller blocked %v, want at most %v", took, ioTimeout)
		}

		// The hung transaction still owns the characteristic: the next
		// caller fails within ioTimeout instead of blocking or interleaving.
		writes := fc.writeCount()
		start = time.Now()
		if _, err := w.ReadBattery(); err == nil {
			t.Fatal("second ReadBattery succeeded while the first still holds the characteristic")
		}
		if took := time.Since(start); took > ioTimeout {
			t.Errorf("second caller blocked %v, want at most %v", took, ioTimeout)
		}
		if n := fc.writeCount(); n != writes {
			t.Errorf("second transaction wrote %d command(s) during the first", n-writes)
		}

		close(hang) // the D-Bus call finally returns
		synctest.Wait()
		if _, err := w.ReadBattery(); err != nil {
			t.Fatalf("ReadBattery after recovery: %v", err)
		}
	})
}
