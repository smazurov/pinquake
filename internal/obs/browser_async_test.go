package obs

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"
)

func blockBrowserSettings(fake *fakeOBS) (<-chan struct{}, func()) {
	started, release := make(chan struct{}), make(chan struct{})
	fake.set(func(f *fakeOBS) {
		f.settingsStarted, f.settingsRelease = started, release
	})
	return started, sync.OnceFunc(func() { close(release) })
}

func TestSlowBrowserCheckDoesNotBlockConnectOrVisibility(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, _ := newTestController(t)
		c.SetTarget(overlays)
		started, unblock := blockBrowserSettings(fake)
		defer unblock()
		connected := make(chan struct{})
		go func() {
			c.Connect("localhost:4455", "")
			close(connected)
		}()
		<-started

		c.SetVisible(true)
		synctest.Wait()
		select {
		case <-connected:
		default:
			t.Error("Connect waited for browser settings")
		}
		if c.Status().State != StateActive {
			t.Errorf("state = %s during browser check, want active", c.Status().State)
		}
		want := []toggle{{"scene-main", 7, false}, {"scene-main", 7, true}}
		if got := fake.toggles(); !slices.Equal(got, want) {
			t.Errorf("toggles during browser check = %+v, want %+v", got, want)
		}
		unblock()
		<-connected
	})
}

func TestObsoleteBrowserChecksDoNotBlockCommandsOrPublishWarnings(t *testing.T) {
	for _, action := range []string{"target change", "disconnect", "reconnect"} {
		t.Run(action, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c, fake, statuses := newTestController(t)
				c.SetTarget(overlays)
				c.Connect("localhost:4455", "")
				expectState(t, c, StateActive)
				fake.set(func(f *fakeOBS) {
					f.settings = map[string]map[string]any{
						"src-waveform": {"url": "http://localhost:8091/canvas", "shutdown": true},
					}
				})
				started, unblock := blockBrowserSettings(fake)
				defer unblock()
				fake.conn.changed <- struct{}{}
				<-started
				fake.set(func(f *fakeOBS) { f.settings = nil })
				before := len(*statuses)
				closedBefore := fake.settingsClosed
				completed := make(chan struct{})
				go func() {
					switch action {
					case "target change":
						c.SetTarget(Target{Scene: "Main", Source: "Camera"})
					case "disconnect":
						c.Disconnect()
					case "reconnect":
						c.Connect("other-host:4455", "")
					}
					close(completed)
				}()
				synctest.Wait()
				select {
				case <-completed:
				default:
					t.Errorf("%s waited for browser settings", action)
				}
				if fake.settingsClosed != closedBefore+1 {
					t.Errorf("%s did not close the obsolete diagnostics connection", action)
				}
				unblock()
				<-completed
				synctest.Wait()
				for _, st := range (*statuses)[before:] {
					if st.BrowserWarning != "" {
						t.Errorf("obsolete check published a warning after %s: %+v", action, st)
					}
				}
			})
		})
	}
}

func TestBrowserChecksCoalesceRefreshesAndDiscardStaleSettings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, fake, statuses := newTestController(t)
		c.SetTarget(overlays)
		c.Connect("localhost:4455", "")
		expectState(t, c, StateActive)
		fake.set(func(f *fakeOBS) {
			f.settings = map[string]map[string]any{
				"src-waveform": {"url": "http://localhost:8091/canvas", "shutdown": true},
			}
		})
		started, unblock := blockBrowserSettings(fake)
		defer unblock()
		fake.conn.changed <- struct{}{}
		<-started
		before := len(*statuses)
		fake.set(func(f *fakeOBS) { f.settings["src-waveform"]["shutdown"] = false })
		for range 10 {
			fake.conn.changed <- struct{}{}
			synctest.Wait()
		}
		unblock()
		expectState(t, c, StateActive)
		if fake.settingsCalls != 3 {
			t.Errorf("settings reads = %d, want initial, blocked, and latest only", fake.settingsCalls)
		}
		for _, st := range (*statuses)[before:] {
			if st.BrowserWarning != "" {
				t.Errorf("obsolete settings published a warning: %+v", st)
			}
		}
	})
}
