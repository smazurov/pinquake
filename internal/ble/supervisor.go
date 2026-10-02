package ble

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Radio is the BLE central the Supervisor drives. Implementations must allow
// Connect only for devices BlueZ currently knows about; the Supervisor
// guarantees that by connecting right after the address advertises.
type Radio interface {
	// Scan reports advertisements until ctx is done or scanning fails.
	Scan(ctx context.Context, found func(Advertisement)) error
	// Connect opens a link to addr.
	Connect(ctx context.Context, addr string) (Link, error)
}

type Advertisement struct {
	Address    string
	Name       string
	RSSI       int
	SensorName string
}

type Link interface {
	// Lost is closed when the peer drops the link.
	Lost() <-chan struct{}
	// Close disconnects. It must be safe to call after Lost fired; Lost
	// is not required to fire after Close.
	Close() error
}

// ErrLinkLost is reported when the peer drops an established link.
var ErrLinkLost = errors.New("link lost")

// ErrNotFound is reported when a scan window ends without the device.
var ErrNotFound = errors.New("device not found")

// ErrConnectTimeout is reported when a connect attempt takes too long.
var ErrConnectTimeout = errors.New("connect timed out")

// errScanEnded is reported when a scan stops without being asked to.
var errScanEnded = errors.New("scan ended unexpectedly")

// ErrDeviceChosen ends a browse: scanning for a list of devices is only for
// choosing one.
var ErrDeviceChosen = errors.New("a device is chosen")

type ScanState string

const (
	ScanRunning ScanState = "scanning"
	ScanWaiting ScanState = "waiting" // the scan failed; retrying after a backoff
	ScanEnded   ScanState = "ended"   // Err says why
)

// ScanStatus reports a browse.
type ScanStatus struct {
	State   ScanState
	Err     error
	RetryIn time.Duration // when waiting
}

// Browser receives a browse's advertisements and status changes. Called
// from the supervisor's loop; must not block.
type Browser struct {
	Found  func(Advertisement)
	Status func(ScanStatus)
}

type LinkState string

const (
	LinkIdle       LinkState = "idle"
	LinkSearching  LinkState = "searching"
	LinkWaiting    LinkState = "waiting" // backing off before the next search
	LinkConnecting LinkState = "connecting"
	LinkConnected  LinkState = "connected"
)

type LinkStatus struct {
	State   LinkState
	Addr    string
	Err     error         // why we are waiting
	RetryIn time.Duration // when waiting
}

type SupervisorConfig struct {
	// ScanWindow bounds each search scan; between windows the supervisor
	// backs off so BlueZ discovery isn't left running indefinitely.
	ScanWindow time.Duration
	// ConnectTimeout bounds a connect attempt. Radio.Connect must return
	// promptly once its ctx is cancelled.
	ConnectTimeout time.Duration
	BackoffMin     time.Duration
	BackoffMax     time.Duration
}

func DefaultSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		ScanWindow:     10 * time.Second,
		ConnectTimeout: 20 * time.Second,
		BackoffMin:     2 * time.Second,
		BackoffMax:     30 * time.Second,
	}
}

// SupervisorHooks are called from the supervisor's goroutines and must not
// block on the Supervisor.
type SupervisorHooks struct {
	OnStatus func(LinkStatus)
	// OnLink sets up a session (e.g. sensor discovery) on a new link. ctx is
	// cancelled on connect timeout, Forget or shutdown; OnLink must return
	// promptly then. An error fails the attempt and closes the link.
	OnLink func(ctx context.Context, l Link) error
	// OnUnlink tears down the session. Called once for every link whose
	// OnLink succeeded, after the link is closed.
	OnUnlink func()
}

// Supervisor keeps a link to the wanted device: scan until it advertises,
// connect, and start over when the link drops.
type Supervisor struct {
	radio Radio
	hooks SupervisorHooks

	mu         sync.Mutex
	cfg        SupervisorConfig
	want       string
	retry      bool // Retry was called
	browsers   map[int]Browser
	nextBrowse int
	wake       chan struct{}

	// loop-owned
	state  LinkState
	target string
	seen   map[string]time.Time // last advertisement while browsing

	scanCancel context.CancelFunc // running scan not yet asked to stop
	scanLive   bool               // a Scan call has not returned (running or stopping)
	scanDone   chan error
	scanHold   bool // last scan failed; don't scan until the backoff ends
	advs       chan Advertisement

	// A connect is decided (pendingConnect) but only launched once no scan
	// is live and no earlier link is still closing: BlueZ may refuse to
	// connect while discovering, and a new session must not start before
	// the old one is torn down.
	pendingConnect bool
	connCancel     context.CancelFunc // non-nil while a connect goroutine runs
	connDone       chan connectResult
	link           Link
	closing        int // Close calls in flight (run off the loop)
	closeDone      chan struct{}

	timer   *time.Timer
	backoff time.Duration // next wait; 0 = BackoffMin
}

func NewSupervisor(radio Radio, cfg SupervisorConfig, hooks SupervisorHooks) *Supervisor {
	return &Supervisor{
		radio:     radio,
		cfg:       cfg,
		hooks:     hooks,
		browsers:  make(map[int]Browser),
		seen:      make(map[string]time.Time),
		wake:      make(chan struct{}, 1),
		state:     LinkIdle,
		advs:      make(chan Advertisement, 16),
		connDone:  make(chan connectResult, 1),
		scanDone:  make(chan error, 1),
		closeDone: make(chan struct{}),
	}
}

// connectResult carries a finished attempt. link != nil means OnLink
// succeeded, so the link is owed an OnUnlink.
type connectResult struct {
	link Link
	err  error
}

// Configure changes timings; they apply to timers armed from now on.
func (s *Supervisor) Configure(cfg SupervisorConfig) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

func (s *Supervisor) config() SupervisorConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Want sets the device to stay connected to.
func (s *Supervisor) Want(addr string) {
	s.mu.Lock()
	s.want = addr
	s.mu.Unlock()
	s.poke()
}

// Browse scans for nearby devices while no device is wanted, reporting
// them to b until ctx is done. Wanting a device ends it (ScanEnded with
// ErrDeviceChosen). Returns immediately.
func (s *Supervisor) Browse(ctx context.Context, b Browser) {
	s.mu.Lock()
	id := s.nextBrowse
	s.nextBrowse++
	s.browsers[id] = b
	s.mu.Unlock()
	s.poke()
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.browsers, id)
		s.mu.Unlock()
		s.poke()
	}()
}

// Wanted returns the device set by Want.
func (s *Supervisor) Wanted() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.want
}

// Retry ends a backoff: while waiting to search again, search now, with
// the backoff starting over. Does nothing in other states.
func (s *Supervisor) Retry() {
	s.mu.Lock()
	s.retry = true
	s.mu.Unlock()
	s.poke()
}

// Forget drops the wanted device: closes the link or stops searching.
func (s *Supervisor) Forget() { s.Want("") }

func (s *Supervisor) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Supervisor) Run(ctx context.Context) {
	for {
		s.reconcile(ctx)
		s.launchConnect(ctx)
		select {
		case <-ctx.Done():
			s.shutdown()
			return
		case <-s.wake:
		case adv := <-s.advs:
			switch {
			case s.target == "":
				s.seen[adv.Address] = time.Now()
				for _, b := range s.browserList() {
					b.Found(adv)
				}
			case s.state == LinkSearching && adv.Address == s.target:
				s.beginConnect()
			}
		case err := <-s.scanDone:
			s.scanLive = false
			if s.scanCancel == nil {
				continue // stopped as requested
			}
			s.scanCancel = nil
			if err == nil {
				err = errScanEnded
			}
			s.stopTimer()
			s.scanHold = true
			if s.target == "" { // a browse: the link has nothing to do with it
				d := s.nextBackoff()
				s.setTimer(d)
				s.reportBrowses(ScanStatus{State: ScanWaiting, Err: err, RetryIn: d})
			} else {
				s.retryLater(err)
			}
		case <-s.closeDone:
			s.closing--
		case res := <-s.connDone:
			s.connCancel()
			s.connCancel = nil
			if s.state != LinkConnecting { // abandoned attempt
				if res.link != nil {
					s.closeLink(res.link)
				}
				continue
			}
			if res.err != nil {
				s.retryLater(res.err)
				continue
			}
			s.stopTimer()
			s.backoff = 0
			s.link = res.link
			s.setState(LinkConnected)
		case <-s.lostC():
			s.closeLink(s.link)
			s.link = nil
			s.setStatus(LinkStatus{State: LinkSearching, Err: ErrLinkLost})
		case <-s.timerC():
			s.timer = nil
			switch s.state {
			case LinkSearching: // scan window over
				s.stopScan()
				s.retryLater(ErrNotFound)
			case LinkConnecting:
				// Move on now; a late result is discarded when it arrives.
				s.pendingConnect = false
				if s.connCancel != nil {
					s.connCancel()
				}
				s.retryLater(ErrConnectTimeout)
			case LinkWaiting:
				s.scanHold = false
				s.setState(LinkSearching)
			case LinkIdle: // browse scan backoff over
				s.scanHold = false
				s.reportBrowses(ScanStatus{State: ScanRunning})
			}
		}
	}
}

// seenTTL is how long after an advertisement BlueZ surely still knows an
// unpaired device (it keeps them ~30s after it stops seeing them).
const seenTTL = 20 * time.Second

func (s *Supervisor) seenRecently(addr string) bool {
	t, ok := s.seen[addr]
	return ok && time.Since(t) < seenTTL
}

// retryLater waits with exponential backoff before searching (or, with no
// target, browsing) again.
func (s *Supervisor) retryLater(err error) {
	d := s.nextBackoff()
	s.setTimer(d)
	s.setStatus(LinkStatus{State: LinkWaiting, Err: err, RetryIn: d})
}

func (s *Supervisor) nextBackoff() time.Duration {
	cfg := s.config()
	d := max(s.backoff, cfg.BackoffMin)
	s.backoff = min(2*d, max(cfg.BackoffMax, cfg.BackoffMin))
	return d
}

func (s *Supervisor) setTimer(d time.Duration) {
	s.stopTimer()
	s.timer = time.NewTimer(d)
}

func (s *Supervisor) stopTimer() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func (s *Supervisor) lostC() <-chan struct{} {
	if s.link == nil {
		return nil
	}
	return s.link.Lost()
}

func (s *Supervisor) timerC() <-chan time.Time {
	if s.timer == nil {
		return nil
	}
	return s.timer.C
}

func (s *Supervisor) browserList() []Browser {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Browser, 0, len(s.browsers))
	for _, b := range s.browsers {
		list = append(list, b)
	}
	return list
}

func (s *Supervisor) reportBrowses(st ScanStatus) {
	for _, b := range s.browserList() {
		if b.Status != nil {
			b.Status(st)
		}
	}
}

// endBrowses ends every browse, as a device is wanted.
func (s *Supervisor) endBrowses() {
	s.mu.Lock()
	ended := s.browsers
	s.browsers = make(map[int]Browser)
	s.mu.Unlock()
	for _, b := range ended {
		if b.Status != nil {
			b.Status(ScanStatus{State: ScanEnded, Err: ErrDeviceChosen})
		}
	}
}

func (s *Supervisor) reconcile(ctx context.Context) {
	s.mu.Lock()
	want, retry := s.want, s.retry
	s.retry = false
	s.mu.Unlock()

	if retry && want == s.target && s.state == LinkWaiting {
		s.stopTimer()
		s.backoff = 0
		s.scanHold = false
		s.setState(LinkSearching)
	}
	if want != s.target {
		s.teardown()
		s.target = want
		switch {
		case want == "":
			s.setState(LinkIdle)
		case s.seenRecently(want) && s.connCancel == nil:
			// Picked from the scan list: BlueZ still knows it.
			s.beginConnect()
		default:
			s.setState(LinkSearching)
		}
		clear(s.seen)
	}
	if s.target != "" {
		s.endBrowses()
	}
	s.mu.Lock()
	browsing := len(s.browsers) > 0
	s.mu.Unlock()
	// Never scan during a connect attempt; some controllers can't do both.
	needScan := s.connCancel == nil && !s.pendingConnect && !s.scanHold &&
		(s.state == LinkSearching || browsing)
	switch {
	case needScan && !s.scanLive: // a stopping scan must finish first
		s.startScan(ctx)
	case !needScan && s.scanCancel != nil:
		s.stopScan()
	}
	// Bound search scans with a window.
	if s.state == LinkSearching && s.scanCancel != nil && s.timer == nil {
		s.setTimer(s.config().ScanWindow)
	}
}

// beginConnect decides to connect to the target. The attempt launches once
// the scan has stopped and earlier links have closed (see launchConnect),
// all within ConnectTimeout.
func (s *Supervisor) beginConnect() {
	if s.pendingConnect || s.connCancel != nil {
		return // one attempt at a time
	}
	s.stopTimer()
	s.pendingConnect = true // reconcile stops the scan
	s.setState(LinkConnecting)
	s.setTimer(s.config().ConnectTimeout)
}

func (s *Supervisor) launchConnect(ctx context.Context) {
	if !s.pendingConnect || s.scanLive || s.closing > 0 {
		return
	}
	s.pendingConnect = false
	connCtx, cancel := context.WithCancel(ctx)
	s.connCancel = cancel
	go func(addr string) {
		link, err := s.radio.Connect(connCtx, addr)
		if err == nil && s.hooks.OnLink != nil {
			if err = s.hooks.OnLink(connCtx, link); err != nil {
				_ = link.Close()
				link = nil
			}
		}
		s.connDone <- connectResult{link: link, err: err}
	}(s.target)
}

// teardown releases everything tied to the current target. An in-flight
// connect is cancelled; its result is discarded when it returns.
func (s *Supervisor) teardown() {
	s.stopScan()
	s.pendingConnect = false
	if s.connCancel != nil {
		s.connCancel()
	}
	s.stopTimer()
	s.scanHold = false
	if s.link != nil {
		s.closeLink(s.link)
		s.link = nil
	}
	s.backoff = 0
}

// shutdown releases everything and waits for links to close, so the device
// is disconnected when Run returns. An in-flight attempt is drained in the
// background so a link that comes up late is still closed.
func (s *Supervisor) shutdown() {
	inFlight := s.connCancel != nil
	s.teardown()
	for ; s.closing > 0; s.closing-- {
		<-s.closeDone
	}
	if inFlight {
		go func() {
			if res := <-s.connDone; res.link != nil {
				_ = res.link.Close()
				s.unlinked()
			}
		}()
	}
}

// closeLink closes a set-up link off the loop (Close may block in BlueZ)
// and then tears down its session. Safe after loss.
func (s *Supervisor) closeLink(l Link) {
	s.closing++
	go func() {
		_ = l.Close()
		s.unlinked()
		s.closeDone <- struct{}{}
	}()
}

func (s *Supervisor) unlinked() {
	if s.hooks.OnUnlink != nil {
		s.hooks.OnUnlink()
	}
}

func (s *Supervisor) startScan(ctx context.Context) {
	scanCtx, cancel := context.WithCancel(ctx)
	s.scanCancel = cancel
	s.scanLive = true
	go func() {
		err := s.radio.Scan(scanCtx, func(adv Advertisement) {
			select {
			case s.advs <- adv:
			case <-scanCtx.Done():
			}
		})
		select {
		case s.scanDone <- err:
		case <-ctx.Done():
		}
	}()
}

// stopScan asks the running scan to stop; it stays live until Scan returns.
func (s *Supervisor) stopScan() {
	if s.scanCancel != nil {
		s.scanCancel()
		s.scanCancel = nil
	}
}

func (s *Supervisor) setState(st LinkState) {
	s.setStatus(LinkStatus{State: st})
}

func (s *Supervisor) setStatus(st LinkStatus) {
	st.Addr = s.target
	s.state = st.State
	if s.hooks.OnStatus != nil {
		s.hooks.OnStatus(st)
	}
}
