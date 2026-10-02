package ble

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/smazurov/pinquake/internal/events"
)

func TestLinkStatusEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestScanner()
		got := make(chan events.BLEStatusEvent, 16)
		defer s.eventBus.Subscribe(func(e events.BLEStatusEvent) { got <- e })()
		if err := s.Connect(tracker, "WT901BLE68"); err != nil {
			t.Fatal(err)
		}

		steps := []struct {
			st   LinkStatus
			want events.BLEStatusEvent
		}{
			{
				LinkStatus{State: LinkSearching, Addr: tracker},
				events.BLEStatusEvent{Status: "searching", Device: tracker, DeviceName: "WT901BLE68"},
			},
			{
				LinkStatus{State: LinkWaiting, Addr: tracker, Err: ErrNotFound, RetryIn: 4 * time.Second},
				events.BLEStatusEvent{Status: "waiting", Device: tracker, DeviceName: "WT901BLE68", Error: "device not found", RetryInS: 4},
			},
			{
				LinkStatus{State: LinkConnecting, Addr: tracker},
				events.BLEStatusEvent{Status: "connecting", Device: tracker, DeviceName: "WT901BLE68"},
			},
			{
				LinkStatus{State: LinkConnected, Addr: tracker},
				events.BLEStatusEvent{Status: "connected", Device: tracker, DeviceName: "WT901BLE68"},
			},
			{
				LinkStatus{State: LinkSearching, Addr: tracker, Err: ErrLinkLost},
				events.BLEStatusEvent{Status: "searching", Reason: "lost", Device: tracker, DeviceName: "WT901BLE68"},
			},
			{ // still searching for the lost link
				LinkStatus{State: LinkWaiting, Addr: tracker, Err: ErrNotFound, RetryIn: 2 * time.Second},
				events.BLEStatusEvent{Status: "waiting", Reason: "lost", Device: tracker, DeviceName: "WT901BLE68", Error: "device not found", RetryInS: 2},
			},
			{
				LinkStatus{State: LinkIdle},
				events.BLEStatusEvent{Status: "idle", Reason: "user"},
			},
		}
		for i, step := range steps {
			s.onLinkStatus(step.st)
			synctest.Wait()
			e := <-got
			e.Timestamp = ""
			if e != step.want {
				t.Fatalf("step %d: got %+v\nwant %+v", i, e, step.want)
			}
		}
	})
}

func TestStatusEventCountsDownRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestScanner()
		s.onLinkStatus(LinkStatus{State: LinkWaiting, Addr: tracker, Err: ErrNotFound, RetryIn: 4 * time.Second})
		time.Sleep(time.Second)
		if got := s.StatusEvent().RetryInS; got != 3 {
			t.Fatalf("retry_in_s = %v a second later, want 3", got)
		}
	})
}

func TestOnConnectReportsTheDevice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestScanner()
		got := make(chan ConnectedDevice, 1)
		s.OnConnect(func(d ConnectedDevice) { got <- d })
		if err := s.Connect(tracker, "WT901BLE68"); err != nil {
			t.Fatal(err)
		}
		s.onLinkStatus(LinkStatus{State: LinkConnected, Addr: tracker})
		synctest.Wait()
		select {
		case d := <-got:
			if want := (ConnectedDevice{Addr: tracker, Name: "WT901BLE68"}); d != want {
				t.Fatalf("got %+v, want %+v", d, want)
			}
		default:
			t.Fatal("OnConnect not called")
		}
	})
}
