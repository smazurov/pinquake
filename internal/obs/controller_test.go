package obs

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeOBS is an in-memory OBS: scenes plus a record of SetItemEnabled calls.
type fakeOBS struct {
	mu      sync.Mutex
	scenes  []Scene
	dialErr error
	pingErr error
	dials   int
	calls   []toggle
	conn    *fakeConn
}

type toggle struct {
	scene   string
	item    int
	enabled bool
}

func (f *fakeOBS) dial(_, _ string) (Conn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dials++
	if f.dialErr != nil {
		return nil, f.dialErr
	}
	f.conn = &fakeConn{obs: f, changed: make(chan struct{}, 1), done: make(chan struct{})}
	return f.conn, nil
}

func (f *fakeOBS) set(fn func(f *fakeOBS)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeOBS) toggles() []toggle {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type fakeConn struct {
	obs     *fakeOBS
	changed chan struct{}
	done    chan struct{}
	closed  bool
}

func (c *fakeConn) Version() string          { return "32.2.2" }
func (c *fakeConn) Changed() <-chan struct{} { return c.changed }
func (c *fakeConn) Done() <-chan struct{}    { return c.done }

func (c *fakeConn) Scenes() ([]Scene, error) {
	c.obs.mu.Lock()
	defer c.obs.mu.Unlock()
	return slices.Clone(c.obs.scenes), nil
}

func (c *fakeConn) SetItemEnabled(scene string, item int, enabled bool) error {
	c.obs.mu.Lock()
	defer c.obs.mu.Unlock()
	c.obs.calls = append(c.obs.calls, toggle{scene, item, enabled})
	return nil
}

func (c *fakeConn) Ping() error {
	c.obs.mu.Lock()
	defer c.obs.mu.Unlock()
	return c.obs.pingErr
}

func (c *fakeConn) Close() error {
	c.obs.mu.Lock()
	defer c.obs.mu.Unlock()
	c.closed = true
	return nil
}

var overlays = Target{Scene: "Main", SceneUUID: "scene-main", Source: "Overlays", SourceUUID: "src-overlays"}

// newTestController starts a controller on a fake OBS holding testScenes and
// stops it at test end, so the bubble can verify no goroutines leak.
func newTestController(t *testing.T) (*Controller, *fakeOBS, *[]Status) {
	t.Helper()
	fake := &fakeOBS{scenes: testScenes()}
	var statuses []Status
	c := NewController(Options{
		Dial:     fake.dial,
		OnStatus: func(s Status) { statuses = append(statuses, s) },
	})
	c.Start()
	t.Cleanup(c.Stop)
	return c, fake, &statuses
}

func expectState(t *testing.T, c *Controller, want State) {
	t.Helper()
	synctest.Wait()
	if got := c.Status().State; got != want {
		t.Fatalf("state = %q, want %q (status %+v)", got, want, c.Status())
	}
}

func expectToggles(t *testing.T, fake *fakeOBS, want ...toggle) {
	t.Helper()
	synctest.Wait()
	if got := fake.toggles(); !slices.Equal(got, want) {
		t.Fatalf("toggles = %+v, want %+v", got, want)
	}
}

func TestConnectSyncsTargetToTriggerThenGoesActive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, statuses := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")

		expectToggles(t, fake, toggle{"scene-main", 7, false})
		expectState(t, c, StateActive)
		if st := c.Status(); st.Version != "32.2.2" || st.Target != overlays {
			t.Errorf("status = %+v", st)
		}
		// Active is only reported after OBS was synced.
		states := []State{}
		for _, s := range *statuses {
			states = append(states, s.State)
		}
		if !slices.Equal(states, []State{StateOff, StateConnecting, StateActive}) { // off: target set before connecting
			t.Errorf("states = %v", states)
		}
	})
}

func TestActiveMirrorsTriggerVisibility(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")

		c.SetVisible(true)
		synctest.Wait()
		c.SetVisible(true) // unchanged: no request
		synctest.Wait()
		c.SetVisible(false)

		expectToggles(t, fake,
			toggle{"scene-main", 7, false},
			toggle{"scene-main", 7, true},
			toggle{"scene-main", 7, false},
		)
	})
}

func TestNoTargetStaysConnectedWithoutToggling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.Connect("localhost:4455", "")
		c.SetVisible(true)

		expectState(t, c, StateConnected)
		expectToggles(t, fake)
	})
}

func TestMissingTargetActivatesOnceItAppears(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		later := Target{Scene: "Main", Source: "Later"}
		c.SetTarget(later)
		c.Connect("localhost:4455", "")

		expectState(t, c, StateConnected)
		if !c.Status().TargetMissing {
			t.Fatal("expected TargetMissing")
		}

		fake.set(func(f *fakeOBS) {
			f.scenes[0].Items = append(f.scenes[0].Items, Item{ID: 9, Name: "Later", UUID: "src-later"})
			f.conn.changed <- struct{}{}
		})
		expectState(t, c, StateActive)
		expectToggles(t, fake, toggle{"scene-main", 9, false})
	})
}

func TestRemovedTargetFallsBackToConnected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")
		expectState(t, c, StateActive)

		fake.set(func(f *fakeOBS) {
			f.scenes[0].Items = f.scenes[0].Items[1:] // drop the Overlays group item
			f.conn.changed <- struct{}{}
		})
		expectState(t, c, StateConnected)
		if !c.Status().TargetMissing {
			t.Error("expected TargetMissing")
		}
	})
}

func TestRenamedTargetIsReported(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeOBS{scenes: testScenes()}
		var renamed []Target
		c := NewController(Options{Dial: fake.dial, OnTarget: func(t Target) { renamed = append(renamed, t) }})
		c.Start()
		t.Cleanup(c.Stop)

		c.SetTarget(Target{Scene: "Old", SceneUUID: "scene-main", Source: "Old", SourceUUID: "src-overlays"})
		c.Connect("localhost:4455", "")

		expectState(t, c, StateActive)
		if !slices.Equal(renamed, []Target{overlays}) {
			t.Errorf("renamed = %+v", renamed)
		}
	})
}

func TestDialFailureRetriesWithBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		fake.set(func(f *fakeOBS) { f.dialErr = errors.New("connection refused") })
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")

		expectState(t, c, StateWaiting)
		if st := c.Status(); st.Error != "connection refused" || st.RetryIn != time.Second {
			t.Fatalf("status = %+v", st)
		}

		time.Sleep(time.Second) // first retry fails too, backoff doubles
		expectState(t, c, StateWaiting)
		if st := c.Status(); st.RetryIn != 2*time.Second {
			t.Fatalf("RetryIn = %v, want 2s", st.RetryIn)
		}

		fake.set(func(f *fakeOBS) { f.dialErr = nil })
		time.Sleep(2 * time.Second)
		expectState(t, c, StateActive)
	})
}

func TestReconnectsAfterOBSCloses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")
		c.SetVisible(true)
		expectState(t, c, StateActive)

		fake.set(func(f *fakeOBS) { close(f.conn.done) })
		expectState(t, c, StateWaiting)

		time.Sleep(time.Second)
		expectState(t, c, StateActive)
		// The new session re-sends the state; OBS may have restarted.
		expectToggles(t, fake,
			toggle{"scene-main", 7, false},
			toggle{"scene-main", 7, true},
			toggle{"scene-main", 7, true},
		)
	})
}

func TestFailedPingReconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")
		expectState(t, c, StateActive)

		fake.set(func(f *fakeOBS) { f.pingErr = errors.New("timeout") })
		time.Sleep(5 * time.Second)
		expectState(t, c, StateWaiting)
		if c.Status().Error != "timeout" {
			t.Errorf("Error = %q", c.Status().Error)
		}
	})
}

func TestDisconnectShowsTargetAndStopsRetrying(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")
		conn := fake.conn

		c.Disconnect()
		expectState(t, c, StateOff)
		expectToggles(t, fake, toggle{"scene-main", 7, false}, toggle{"scene-main", 7, true})
		if !conn.closed {
			t.Error("connection not closed")
		}

		time.Sleep(time.Minute)
		if fake.dials != 1 {
			t.Errorf("dials = %d after disconnect, want 1", fake.dials)
		}
	})
}

func TestChangingTargetShowsTheOldOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")

		c.SetTarget(Target{Scene: "Overlays", SceneUUID: "src-overlays", Source: "Waveform", SourceUUID: "src-waveform"})
		expectState(t, c, StateActive)
		expectToggles(t, fake,
			toggle{"scene-main", 7, false},
			toggle{"scene-main", 7, true},    // old target released
			toggle{"src-overlays", 2, false}, // new target synced
		)
	})
}

func TestStopLeavesTargetAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeOBS{scenes: testScenes()}
		c := NewController(Options{Dial: fake.dial})
		c.Start()
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")
		c.Stop()

		expectToggles(t, fake, toggle{"scene-main", 7, false})
		if !fake.conn.closed {
			t.Error("connection not closed")
		}
	})
}

func TestScenesRequiresConnection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _, _ := newTestController(t)
		if _, err := c.Scenes(); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("err = %v, want ErrNotConnected", err)
		}
		c.Connect("localhost:4455", "")
		scenes, err := c.Scenes()
		if err != nil || len(scenes) != 3 {
			t.Fatalf("Scenes() = %d scenes, %v", len(scenes), err)
		}
	})
}
