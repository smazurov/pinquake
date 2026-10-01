package ble

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// Every test runs in a synctest bubble: time is virtual and settle() returns
// once all supervisor and fake-radio goroutines are blocked, so assertions
// see the supervisor's complete reaction without wall-clock waits.

const (
	tracker = "EA:F0:F1:BC:59:DD"
	other   = "11:22:33:44:55:66"
)

// settle waits until every other goroutine in the bubble is blocked.
func settle() { synctest.Wait() }

// --- fake BLE radio -------------------------------------------------------

type fakeRadio struct {
	scans    chan *fakeScan
	connects chan *fakeConnect

	// holdStop makes a cancelled Scan keep running until finishStop, like a
	// slow BlueZ StopDiscovery.
	holdStop bool

	mu       sync.Mutex
	all      []*fakeScan
	live     int // Scan calls that have not returned
	overlaps int // Scan started while another was still live
	// ignoreCancel makes Connect wait for its result even after ctx is
	// cancelled, like tinygo's blocking Device1.Connect.
	ignoreCancel bool
}

func newFakeRadio() *fakeRadio {
	return &fakeRadio{
		scans:    make(chan *fakeScan, 8),
		connects: make(chan *fakeConnect, 8),
	}
}

type fakeScan struct {
	ctx      context.Context
	found    func(Advertisement)
	fail     chan error
	stopped  chan struct{} // closed by finishStop when holdStop is set
	stopOnce sync.Once
}

func (sc *fakeScan) finishStop() { sc.stopOnce.Do(func() { close(sc.stopped) }) }

func (r *fakeRadio) Scan(ctx context.Context, found func(Advertisement)) error {
	sc := &fakeScan{ctx: ctx, found: found, fail: make(chan error, 1), stopped: make(chan struct{})}
	r.mu.Lock()
	if r.live > 0 {
		r.overlaps++
	}
	r.live++
	r.all = append(r.all, sc)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.live--
		r.mu.Unlock()
	}()
	r.scans <- sc
	select {
	case <-ctx.Done():
		if r.holdStop {
			<-sc.stopped
		}
		return nil
	case err := <-sc.fail:
		return err
	}
}

// releaseAll unblocks every held stop so a failed test can still shut down.
func (r *fakeRadio) releaseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sc := range r.all {
		sc.finishStop()
	}
}

func (r *fakeRadio) overlapCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.overlaps
}

func (sc *fakeScan) advertise(addr string) { sc.found(Advertisement{Address: addr}) }

type fakeConnect struct {
	addr   string
	ctx    context.Context
	result chan fakeResult
	// scansRunning counts Scan calls that had not returned when Connect was
	// called; some controllers can't scan and connect at once.
	scansRunning int
}

type fakeResult struct {
	link Link
	err  error
}

func (r *fakeRadio) Connect(ctx context.Context, addr string) (Link, error) {
	c := &fakeConnect{addr: addr, ctx: ctx, result: make(chan fakeResult, 1)}
	r.mu.Lock()
	c.scansRunning = r.live
	r.mu.Unlock()
	r.connects <- c
	if r.ignoreCancel {
		res := <-c.result
		return res.link, res.err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-c.result:
		return res.link, res.err
	}
}

func (c *fakeConnect) succeed() *fakeLink { return c.succeedWith(newFakeLink()) }

func (c *fakeConnect) succeedWith(l *fakeLink) *fakeLink {
	c.result <- fakeResult{link: l}
	return l
}

func (c *fakeConnect) fail(err error) { c.result <- fakeResult{err: err} }

type fakeLink struct {
	lost        chan struct{}
	closed      chan struct{} // closed when Close returns
	hold        chan struct{} // if non-nil, Close blocks until release
	once        sync.Once
	releaseOnce sync.Once
}

func newFakeLink() *fakeLink {
	return &fakeLink{lost: make(chan struct{}), closed: make(chan struct{})}
}

// slowCloseLink returns a link whose Close blocks until release, like a
// slow BlueZ Disconnect. Released automatically at test end.
func (h *harness) slowCloseLink() *fakeLink {
	l := newFakeLink()
	l.hold = make(chan struct{})
	h.t.Cleanup(l.release)
	return l
}

func (l *fakeLink) release() {
	if l.hold != nil {
		l.releaseOnce.Do(func() { close(l.hold) })
	}
}

func (l *fakeLink) Lost() <-chan struct{} { return l.lost }
func (l *fakeLink) Close() error {
	if l.hold != nil {
		<-l.hold
	}
	l.once.Do(func() { close(l.closed) })
	return nil
}
func (l *fakeLink) drop() { close(l.lost) }

// --- harness --------------------------------------------------------------

type harness struct {
	t      *testing.T
	sup    *Supervisor
	radio  *fakeRadio
	stop   func() // cancels Run and checks it returned
	cancel context.CancelFunc
	done   chan struct{} // closed when Run returns

	mu       sync.Mutex
	statuses []LinkStatus
	setupFn  func(context.Context, Link) error // OnLink behaviour; nil = succeed
	unlinks  int
}

// newHarness must be called inside a synctest bubble.
func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, radio: newFakeRadio()}
	h.sup = NewSupervisor(h.radio, DefaultSupervisorConfig(), SupervisorHooks{
		OnStatus: func(st LinkStatus) {
			h.mu.Lock()
			h.statuses = append(h.statuses, st)
			h.mu.Unlock()
		},
		OnLink: func(ctx context.Context, l Link) error {
			h.mu.Lock()
			fn := h.setupFn
			h.mu.Unlock()
			if fn == nil {
				return nil
			}
			return fn(ctx, l)
		},
		OnUnlink: func() {
			h.mu.Lock()
			h.unlinks++
			h.mu.Unlock()
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	h.cancel, h.done = cancel, done
	go func() {
		h.sup.Run(ctx)
		close(done)
	}()
	h.stop = func() {
		cancel()
		settle()
		select {
		case <-done:
		default:
			t.Error("supervisor did not stop")
		}
	}
	t.Cleanup(h.stop)
	t.Cleanup(h.radio.releaseAll) // runs first (LIFO)
	return h
}

func (h *harness) runReturned() bool {
	select {
	case <-h.done:
		return true
	default:
		return false
	}
}

func (h *harness) setup(fn func(context.Context, Link) error) {
	h.mu.Lock()
	h.setupFn = fn
	h.mu.Unlock()
}

// advance moves virtual time forward and lets the supervisor react.
func (h *harness) advance(d time.Duration) {
	time.Sleep(d)
	settle()
}

func (h *harness) nextScan() *fakeScan {
	h.t.Helper()
	settle()
	select {
	case sc := <-h.radio.scans:
		return sc
	default:
		h.t.Fatal("expected a scan to start")
		return nil
	}
}

func (h *harness) noScan() {
	h.t.Helper()
	settle()
	select {
	case <-h.radio.scans:
		h.t.Fatal("unexpected scan")
	default:
	}
}

func (h *harness) nextConnect() *fakeConnect {
	h.t.Helper()
	settle()
	select {
	case c := <-h.radio.connects:
		return c
	default:
		h.t.Fatal("expected a connect attempt")
		return nil
	}
}

func (h *harness) noConnect() {
	h.t.Helper()
	settle()
	select {
	case c := <-h.radio.connects:
		h.t.Fatalf("unexpected connect to %s", c.addr)
	default:
	}
}

// expectState asserts the supervisor's current status once it has settled.
func (h *harness) expectState(want LinkState) LinkStatus {
	h.t.Helper()
	settle()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.statuses) == 0 {
		h.t.Fatalf("no status reported, want %q", want)
	}
	st := h.statuses[len(h.statuses)-1]
	if st.State != want {
		h.t.Fatalf("state = %q (err=%v), want %q", st.State, st.Err, want)
	}
	return st
}

func (h *harness) expectUnlinks(want int) {
	h.t.Helper()
	settle()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.unlinks != want {
		h.t.Fatalf("OnUnlink called %d times, want %d", h.unlinks, want)
	}
}

// isClosed reports whether ch is closed once the bubble has settled.
func isClosed(ch <-chan struct{}) bool {
	settle()
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

var errBoom = errors.New("boom")

// --- behaviors ------------------------------------------------------------

func TestWantConnectsWhenDeviceAdvertises(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)

		h.sup.Want(tracker)
		scan := h.nextScan()
		scan.advertise(other)
		scan.advertise(tracker)

		conn := h.nextConnect()
		if conn.addr != tracker {
			t.Fatalf("connected to %s, want %s", conn.addr, tracker)
		}
		if conn.scansRunning != 0 {
			t.Errorf("%d scan(s) still running when Connect was called", conn.scansRunning)
		}
		conn.succeed()
		if st := h.expectState(LinkConnected); st.Addr != tracker {
			t.Errorf("connected status addr = %q, want %q", st.Addr, tracker)
		}
	})
}

func TestFailedConnectBacksOffThenRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)

		h.nextScan().advertise(tracker)
		h.nextConnect().fail(errBoom)
		st := h.expectState(LinkWaiting)
		if !errors.Is(st.Err, errBoom) || st.RetryIn != 2*time.Second {
			t.Fatalf("after 1st failure: err=%v retryIn=%v, want boom/2s", st.Err, st.RetryIn)
		}
		h.noScan()
		h.advance(2 * time.Second)

		h.nextScan().advertise(tracker)
		h.nextConnect().fail(errBoom)
		if st := h.expectState(LinkWaiting); st.RetryIn != 4*time.Second {
			t.Fatalf("after 2nd failure retryIn=%v, want 4s", st.RetryIn)
		}
		h.advance(4 * time.Second)

		h.nextScan().advertise(tracker)
		h.nextConnect().succeed()
		h.expectState(LinkConnected)
	})
}

func TestLostLinkSearchesAgainAndReconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeed()
		h.expectState(LinkConnected)

		link.drop()
		st := h.expectState(LinkSearching)
		if !errors.Is(st.Err, ErrLinkLost) {
			t.Errorf("searching after drop: err=%v, want ErrLinkLost", st.Err)
		}

		// No backoff after a drop: the tracker may come straight back. Scanning
		// again also refreshes BlueZ's device entry before connecting.
		h.nextScan().advertise(tracker)
		h.nextConnect().succeed()
		h.expectState(LinkConnected)
	})
}

func TestForgetClosesLinkAndDoesNotReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeed()
		h.expectState(LinkConnected)

		h.sup.Forget()
		h.expectState(LinkIdle)
		if !isClosed(link.closed) {
			t.Error("link not closed")
		}
		h.noScan()
	})
}

func TestForgetWhileSearchingStopsScan(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		scan := h.nextScan()

		h.sup.Forget()
		h.expectState(LinkIdle)
		if scan.ctx.Err() == nil {
			t.Error("scan still running after Forget")
		}
	})
}

func TestWantOtherWhileConnectedSwitchesDevice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeed()
		h.expectState(LinkConnected)

		h.sup.Want(other)
		scan := h.nextScan()
		if !isClosed(link.closed) {
			t.Error("old link not closed when switching")
		}
		scan.advertise(tracker) // no longer wanted
		scan.advertise(other)
		if c := h.nextConnect(); c.addr != other {
			t.Fatalf("connected to %s, want %s", c.addr, other)
		} else {
			c.succeed()
		}
		if st := h.expectState(LinkConnected); st.Addr != other {
			t.Errorf("connected addr = %s, want %s", st.Addr, other)
		}
	})
}

func TestWantOtherWhileConnectingAbandonsAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.radio.ignoreCancel = true
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		stale := h.nextConnect()

		h.sup.Want(other)
		if !isClosed(stale.ctx.Done()) {
			t.Fatal("in-flight connect not cancelled")
		}
		// One attempt at a time: nothing new until the stale one returns.
		h.noScan()

		lateLink := stale.succeed()
		if !isClosed(lateLink.closed) {
			t.Fatal("late link from abandoned attempt not closed")
		}
		h.nextScan().advertise(other)
		h.nextConnect().succeed()
		if st := h.expectState(LinkConnected); st.Addr != other {
			t.Errorf("connected addr = %s, want %s", st.Addr, other)
		}
	})
}

func TestSearchIsDutyCycledWhileDeviceAbsent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)

		for _, wantRetry := range []time.Duration{2, 4, 8, 16, 30, 30} {
			scan := h.nextScan()
			scan.advertise(other)
			h.advance(10 * time.Second) // scan window ends without the tracker
			st := h.expectState(LinkWaiting)
			if scan.ctx.Err() == nil {
				t.Fatal("scan still running while waiting")
			}
			if !errors.Is(st.Err, ErrNotFound) || st.RetryIn != wantRetry*time.Second {
				t.Fatalf("waiting: err=%v retryIn=%v, want ErrNotFound/%vs", st.Err, st.RetryIn, wantRetry)
			}
			h.noScan()
			h.advance(st.RetryIn)
		}

		h.nextScan().advertise(tracker)
		h.nextConnect().succeed()
		h.expectState(LinkConnected)
	})
}

func collect(ch chan Advertisement) func(Advertisement) {
	return func(a Advertisement) { ch <- a }
}

func expectAdv(t *testing.T, ch chan Advertisement, addr string) {
	t.Helper()
	settle()
	select {
	case a := <-ch:
		if a.Address != addr {
			t.Fatalf("browser got %s, want %s", a.Address, addr)
		}
	default:
		t.Fatalf("browser never saw %s", addr)
	}
}

func TestBrowseWhileIdleScansUntilCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		ctx, cancel := context.WithCancel(context.Background())
		seen := make(chan Advertisement, 8)

		h.sup.Browse(ctx, collect(seen))
		scan := h.nextScan()
		scan.advertise(other)
		expectAdv(t, seen, other)

		cancel()
		if !isClosed(scan.ctx.Done()) {
			t.Fatal("scan kept running after browse ended")
		}
		h.noConnect() // browsing never connects
	})
}

func TestBrowseSharesScanWithSearch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		scan := h.nextScan()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		seen := make(chan Advertisement, 8)
		h.sup.Browse(ctx, collect(seen))
		h.noScan() // BlueZ allows one discovery per client: reuse it

		h.advance(time.Minute) // no scan window while someone is browsing
		scan.advertise(other)
		expectAdv(t, seen, other)
		if scan.ctx.Err() != nil {
			t.Fatal("search scan was cut short while browsing")
		}

		scan.advertise(tracker)
		expectAdv(t, seen, tracker)
		h.nextConnect().succeed()
		h.expectState(LinkConnected)
	})
}

func TestScanErrorBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)

		h.nextScan().fail <- errBoom
		st := h.expectState(LinkWaiting)
		if !errors.Is(st.Err, errBoom) || st.RetryIn != 2*time.Second {
			t.Fatalf("waiting: err=%v retryIn=%v, want boom/2s", st.Err, st.RetryIn)
		}
		h.noScan()
		h.advance(2 * time.Second)

		h.nextScan().advertise(tracker)
		h.nextConnect().succeed()
		h.expectState(LinkConnected)
	})
}

func TestBrowseScanErrorBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.sup.Browse(ctx, func(Advertisement) {})

		h.nextScan().fail <- errBoom
		if st := h.expectState(LinkWaiting); !errors.Is(st.Err, errBoom) {
			t.Fatalf("waiting err=%v, want boom", st.Err)
		}
		h.noScan() // no hot loop while the browser is still open
		h.advance(2 * time.Second)
		h.expectState(LinkIdle)
		h.nextScan()
	})
}

func TestHungConnectTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		conn := h.nextConnect()

		h.advance(20 * time.Second)
		st := h.expectState(LinkWaiting)
		if !errors.Is(st.Err, ErrConnectTimeout) || st.RetryIn != 2*time.Second {
			t.Fatalf("waiting: err=%v retryIn=%v, want ErrConnectTimeout/2s", st.Err, st.RetryIn)
		}
		if !isClosed(conn.ctx.Done()) {
			t.Fatal("timed-out connect not cancelled")
		}

		h.advance(2 * time.Second)
		h.nextScan().advertise(tracker)
		h.nextConnect().succeed()
		h.expectState(LinkConnected)
	})
}

func TestFailedSetupClosesLinkAndBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.setup(func(context.Context, Link) error { return errBoom }) // e.g. service discovery failed
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeed()

		st := h.expectState(LinkWaiting)
		if !errors.Is(st.Err, errBoom) {
			t.Errorf("waiting err=%v, want boom", st.Err)
		}
		if !isClosed(link.closed) {
			t.Error("link not closed after failed setup")
		}
		h.expectUnlinks(0) // never set up, so nothing to tear down
	})
}

func TestUnlinkCalledOnceWheneverASetUpLinkEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		connect := func(addr string) *fakeLink {
			t.Helper()
			h.nextScan().advertise(addr)
			l := h.nextConnect().succeed()
			h.expectState(LinkConnected)
			return l
		}

		h.sup.Want(tracker)
		connect(tracker).drop()
		h.expectUnlinks(1)

		connect(tracker)
		h.sup.Want(other) // switch
		h.expectUnlinks(2)

		connect(other)
		h.sup.Forget()
		h.expectState(LinkIdle)
		h.expectUnlinks(3)
	})
}

func TestLateSetUpLinkIsTornDown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.radio.ignoreCancel = true
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		stale := h.nextConnect()

		h.sup.Forget()
		h.expectState(LinkIdle)
		late := stale.succeed() // connect + setup complete after we gave up

		if !isClosed(late.closed) {
			t.Fatal("late link not closed")
		}
		h.expectUnlinks(1)
	})
}

func TestShutdownClosesLink(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeed()
		h.expectState(LinkConnected)

		h.stop()
		if !isClosed(link.closed) {
			t.Error("link not closed on shutdown")
		}
		h.expectUnlinks(1)
	})
}

func TestShutdownDuringConnectClosesLateLink(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.radio.ignoreCancel = true
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		conn := h.nextConnect()

		h.stop()
		late := conn.succeed()
		if !isClosed(late.closed) {
			t.Fatal("link that came up after shutdown was not closed")
		}
		h.expectUnlinks(1)
	})
}

func TestTrackerSeenWhileBrowsingDuringBackoffConnectsNow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		h.nextConnect().fail(errBoom)
		h.expectState(LinkWaiting)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.sup.Browse(ctx, func(Advertisement) {})
		h.nextScan().advertise(tracker) // user's scan list sees it mid-backoff

		h.nextConnect().succeed() // no clock advance needed
		h.expectState(LinkConnected)
	})
}

func TestConnectWaitsForScanToStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.radio.holdStop = true
		h.sup.Want(tracker)
		scan := h.nextScan()

		scan.advertise(tracker)
		h.expectState(LinkConnecting)
		h.noConnect() // BlueZ is still stopping discovery

		scan.finishStop()
		if c := h.nextConnect(); c.scansRunning != 0 {
			t.Fatalf("%d scan(s) still running at Connect", c.scansRunning)
		}
	})
}

func TestScansNeverOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.radio.holdStop = true
		h.sup.Want(tracker)
		first := h.nextScan()

		h.sup.Want(other) // stop and search again
		h.noScan()        // not until the first scan has stopped
		first.finishStop()
		h.nextScan().finishStop()
		if n := h.radio.overlapCount(); n != 0 {
			t.Fatalf("%d overlapping scan(s)", n)
		}
	})
}

func TestSlowCloseDoesNotStallLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeedWith(h.slowCloseLink())
		h.expectState(LinkConnected)

		h.sup.Forget() // Close blocks in BlueZ...
		h.expectState(LinkIdle)
		h.expectUnlinks(0) // session torn down only after Close returns

		h.sup.Want(tracker) // ...but the supervisor keeps working
		h.nextScan().advertise(tracker)
		h.expectState(LinkConnecting)
		h.noConnect() // no new connect while the old link is still closing

		link.release()
		h.expectUnlinks(1)
		h.nextConnect().succeed()
		h.expectState(LinkConnected)
	})
}

func TestShutdownWaitsForClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeedWith(h.slowCloseLink())
		h.expectState(LinkConnected)

		h.cancel()
		settle()
		if h.runReturned() {
			t.Fatal("Run returned before the link finished closing")
		}
		link.release()
		settle()
		if !h.runReturned() {
			t.Fatal("Run did not return after the link closed")
		}
		h.expectUnlinks(1)
	})
}

func TestConnectTimeoutCancelsSetup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.setup(func(ctx context.Context, _ Link) error {
			<-ctx.Done() // e.g. service discovery hung in BlueZ
			return ctx.Err()
		})
		h.sup.Want(tracker)
		h.nextScan().advertise(tracker)
		link := h.nextConnect().succeed()

		h.advance(20 * time.Second)
		if st := h.expectState(LinkWaiting); !errors.Is(st.Err, ErrConnectTimeout) {
			t.Fatalf("waiting err=%v, want ErrConnectTimeout", st.Err)
		}
		if !isClosed(link.closed) {
			t.Error("link not closed after setup was cancelled")
		}
		h.advance(2 * time.Second)
		h.nextScan() // not wedged behind the hung setup
	})
}

func TestConfigureChangesTimings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.sup.Configure(SupervisorConfig{
			ScanWindow:     3 * time.Second,
			ConnectTimeout: 7 * time.Second,
			BackoffMin:     time.Second,
			BackoffMax:     4 * time.Second,
		})
		h.sup.Want(tracker)

		h.nextScan()
		h.advance(3 * time.Second)
		if st := h.expectState(LinkWaiting); st.RetryIn != time.Second {
			t.Fatalf("retryIn = %v, want 1s", st.RetryIn)
		}
		h.advance(time.Second)
		h.nextScan().advertise(tracker)
		h.nextConnect() // never answers
		h.advance(7 * time.Second)
		if st := h.expectState(LinkWaiting); !errors.Is(st.Err, ErrConnectTimeout) || st.RetryIn != 2*time.Second {
			t.Fatalf("waiting: err=%v retryIn=%v, want ErrConnectTimeout/2s", st.Err, st.RetryIn)
		}
	})
}
