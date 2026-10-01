package obs

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type State string

const (
	StateOff        State = "off"        // not wanted
	StateConnecting State = "connecting" // dialing
	StateWaiting    State = "waiting"    // backing off after a failure
	StateConnected  State = "connected"  // connected; no target, or target not found
	StateActive     State = "active"     // connected and driving the target
)

type Status struct {
	State         State
	Server        string
	Version       string // OBS version, while connected
	Error         string // why we are waiting
	RetryIn       time.Duration
	Target        Target // configured target; current names once resolved
	TargetMissing bool
}

var (
	ErrNotConnected = errors.New("not connected to OBS")
	ErrStopped      = errors.New("OBS controller stopped")
)

type Options struct {
	Dial Dialer
	// OnStatus reports every status change, in order, from the controller
	// goroutine. It must not call back into the Controller.
	OnStatus func(Status)
	// OnTarget reports a target whose names or UUIDs changed in OBS, so the
	// caller can persist them. Same goroutine rules as OnStatus.
	OnTarget     func(Target)
	PingInterval time.Duration
	BackoffMin   time.Duration
	BackoffMax   time.Duration
}

// Controller keeps an obs-websocket session alive while wanted and mirrors
// the trigger's visibility onto the target scene item. All session state is
// owned by one goroutine; the public methods run their work on it.
type Controller struct {
	opts    Options
	cmds    chan func()
	kick    chan struct{}
	desired atomic.Bool
	stop    chan struct{}
	stopped sync.Once
	wg      sync.WaitGroup

	statusMu sync.Mutex
	status   Status

	// Owned by the run goroutine.
	want     bool
	server   string
	password string
	target   Target
	conn     Conn
	resolved bool // target found in OBS; itemID and target.SceneUUID are valid
	itemID   int
	sent     bool // last visibility sent to OBS, valid while sentOK
	sentOK   bool
	backoff  time.Duration
	retry    *time.Timer
	ping     *time.Ticker
}

func NewController(opts Options) *Controller {
	if opts.PingInterval <= 0 {
		opts.PingInterval = 5 * time.Second
	}
	if opts.BackoffMin <= 0 {
		opts.BackoffMin = time.Second
	}
	if opts.BackoffMax < opts.BackoffMin {
		opts.BackoffMax = max(30*time.Second, opts.BackoffMin)
	}
	return &Controller{
		opts:   opts,
		cmds:   make(chan func()),
		kick:   make(chan struct{}, 1),
		stop:   make(chan struct{}),
		status: Status{State: StateOff},
	}
}

func (c *Controller) Start() {
	c.wg.Add(1)
	go c.run()
}

// Stop ends the session without touching the target, so whatever OBS shows
// now stays as is.
func (c *Controller) Stop() {
	c.stopped.Do(func() { close(c.stop) })
	c.wg.Wait()
}

// Connect (re)starts the session and keeps it alive until Disconnect.
// It returns after the first dial attempt.
func (c *Controller) Connect(server, password string) {
	c.do(func() { c.connect(server, password) })
}

// Disconnect hands visibility back to the overlays: it shows the target
// so a browser source inside it can hide itself again, then closes.
func (c *Controller) Disconnect() {
	c.do(c.disconnect)
}

// SetTarget picks the scene item to drive. The old target is shown again.
func (c *Controller) SetTarget(t Target) {
	c.do(func() { c.setTarget(t) })
}

// SetVisible sets the visibility to mirror onto the target.
func (c *Controller) SetVisible(v bool) {
	c.desired.Store(v)
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

func (c *Controller) Status() Status {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	return c.status
}

// Scenes lists scenes and groups for picking a target.
func (c *Controller) Scenes() ([]Scene, error) {
	var (
		scenes []Scene
		err    = ErrStopped
	)
	c.do(func() {
		if c.conn == nil {
			err = ErrNotConnected
			return
		}
		scenes, err = c.conn.Scenes()
	})
	return scenes, err
}

// do runs f on the controller goroutine and waits for it.
func (c *Controller) do(f func()) {
	done := make(chan struct{})
	select {
	case c.cmds <- func() { f(); close(done) }:
		<-done
	case <-c.stop:
	}
}

func (c *Controller) run() {
	defer c.wg.Done()
	for {
		var changed, done <-chan struct{}
		var pingC, retryC <-chan time.Time
		if c.conn != nil {
			changed, done, pingC = c.conn.Changed(), c.conn.Done(), c.ping.C
		}
		if c.retry != nil {
			retryC = c.retry.C
		}

		select {
		case <-c.stop:
			c.stopRetry()
			c.closeConn()
			return
		case f := <-c.cmds:
			f()
		case <-c.kick:
			c.apply()
		case <-changed:
			c.resolve()
		case <-done:
			c.lost(errors.New("OBS closed the connection"))
		case <-pingC:
			if err := c.conn.Ping(); err != nil {
				c.lost(err)
			}
		case <-retryC:
			c.retry = nil
			c.dial()
		}
	}
}

func (c *Controller) connect(server, password string) {
	if c.conn != nil && server == c.server && password == c.password {
		return
	}
	if c.conn != nil {
		c.release()
		c.closeConn()
	}
	c.stopRetry()
	c.want, c.server, c.password = true, server, password
	c.backoff = 0
	c.dial()
}

func (c *Controller) disconnect() {
	c.want = false
	c.stopRetry()
	// Report first so the overlays take over before the target reappears.
	c.report(StateOff, nil)
	c.release()
	c.closeConn()
}

func (c *Controller) setTarget(t Target) {
	if t == c.target {
		return
	}
	c.release()
	c.target = t
	if c.conn != nil {
		c.resolve()
		return
	}
	st := c.Status()
	st.Target, st.TargetMissing = t, false
	c.setStatus(st)
}

func (c *Controller) dial() {
	c.report(StateConnecting, nil)
	conn, err := c.opts.Dial(c.server, c.password)
	if err != nil {
		c.waitRetry(err)
		return
	}
	c.conn = conn
	c.backoff = 0
	c.ping = time.NewTicker(c.opts.PingInterval)
	c.resolve()
}

// resolve finds the target in OBS and, once found, syncs it and reports
// active. Runs on connect, target change, and OBS scene changes.
func (c *Controller) resolve() {
	if c.conn == nil {
		return
	}
	if c.target.IsZero() {
		c.resolved = false
		c.report(StateConnected, nil)
		return
	}
	scenes, err := c.conn.Scenes()
	if err != nil {
		c.lost(err)
		return
	}
	t, id, ok := Resolve(scenes, c.target)
	if !ok {
		c.resolved = false
		c.reportMissing()
		return
	}
	if t != c.target {
		c.target = t
		if c.opts.OnTarget != nil {
			c.opts.OnTarget(t)
		}
	}
	if !c.resolved || c.itemID != id {
		c.sentOK = false
	}
	c.resolved, c.itemID = true, id
	// Sync OBS before reporting active, so overlays never draw while the
	// target is still showing a stale state.
	if c.apply() {
		c.report(StateActive, nil)
	}
}

// apply sends the desired visibility if it differs from what OBS has.
// It reports false if the session failed.
func (c *Controller) apply() bool {
	if c.conn == nil || !c.resolved {
		return true
	}
	v := c.desired.Load()
	if c.sentOK && c.sent == v {
		return true
	}
	if err := c.conn.SetItemEnabled(c.target.SceneUUID, c.itemID, v); err != nil {
		c.lost(fmt.Errorf("toggle %s: %w", c.target.Label(), err))
		return false
	}
	c.sent, c.sentOK = v, true
	return true
}

// release shows the target again so it no longer depends on the trigger.
func (c *Controller) release() {
	showing := c.sentOK && c.sent
	if c.conn != nil && c.resolved && !showing {
		_ = c.conn.SetItemEnabled(c.target.SceneUUID, c.itemID, true)
	}
	c.resolved, c.sentOK = false, false
}

func (c *Controller) lost(err error) {
	c.closeConn()
	c.waitRetry(err)
}

func (c *Controller) waitRetry(err error) {
	if c.backoff == 0 {
		c.backoff = c.opts.BackoffMin
	} else {
		c.backoff = min(c.backoff*2, c.opts.BackoffMax)
	}
	c.retry = time.NewTimer(c.backoff)
	c.report(StateWaiting, err)
}

func (c *Controller) closeConn() {
	if c.conn == nil {
		return
	}
	_ = c.conn.Close()
	c.conn = nil
	c.ping.Stop()
	c.resolved, c.sentOK = false, false
}

func (c *Controller) stopRetry() {
	if c.retry != nil {
		c.retry.Stop()
		c.retry = nil
	}
}

func (c *Controller) report(state State, err error) {
	st := Status{State: state, Server: c.server, Target: c.target}
	if c.conn != nil && state != StateOff {
		st.Version = c.conn.Version()
	}
	if err != nil {
		st.Error = err.Error()
		st.RetryIn = c.backoff
	}
	c.setStatus(st)
}

func (c *Controller) reportMissing() {
	st := Status{State: StateConnected, Server: c.server, Target: c.target, TargetMissing: true}
	if c.conn != nil {
		st.Version = c.conn.Version()
	}
	c.setStatus(st)
}

func (c *Controller) setStatus(st Status) {
	c.statusMu.Lock()
	same := st == c.status
	c.status = st
	c.statusMu.Unlock()
	if !same && c.opts.OnStatus != nil {
		c.opts.OnStatus(st)
	}
}
