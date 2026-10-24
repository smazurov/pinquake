package obs

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/andreykaipov/goobs"
	obsevents "github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/events/subscriptions"
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/typedefs"
	"github.com/gorilla/websocket"
)

// Conn is one obs-websocket session.
type Conn interface {
	// Version is the OBS version reported at connect.
	Version() string
	// Scenes lists every scene and group with their items.
	Scenes() ([]Scene, error)
	// OpenInputSettings opens a separate read-only connection. Called by the
	// diagnostics worker, so settings requests never delay visibility writes.
	OpenInputSettings() (InputSettingsConn, error)
	SetItemEnabled(sceneUUID string, itemID int, enabled bool) error
	// Ping round-trips a request; an error means the session is dead.
	Ping() error
	// Changed signals that scenes, items or names changed, so a resolved
	// target may have moved or vanished.
	Changed() <-chan struct{}
	// Done is closed when OBS ends the session.
	Done() <-chan struct{}
	Close() error
}

// InputSettingsConn is an independent session for browser diagnostics.
// It is owned by one worker and closed after each scan.
type InputSettingsConn interface {
	// InputSettings reads settings merged over the input kind defaults.
	InputSettings(Item) (map[string]any, error)
	Close() error
}

// Dialer opens a session. server is host:port.
type Dialer func(server, password string) (Conn, error)

const requestTimeout = 5 * time.Second

// Dial connects to obs-websocket with goobs.
func Dial(server, password string) (Conn, error) {
	client, err := dialClient(server, password, subscriptions.Config|subscriptions.Scenes|subscriptions.Inputs|subscriptions.SceneItems)
	if err != nil {
		return nil, err
	}
	v, err := client.General.GetVersion(&general.GetVersionParams{})
	if err != nil {
		_ = client.Disconnect()
		return nil, err
	}
	c := &goobsConn{
		client:   client,
		server:   server,
		password: password,
		version:  v.ObsVersion,
		changed:  make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	go c.listen()
	return c, nil
}

func dialClient(server, password string, events int) (*goobs.Client, error) {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = requestTimeout
	return goobs.New(server,
		goobs.WithPassword(password),
		goobs.WithDialer(&dialer),
		goobs.WithResponseTimeoutDuration(requestTimeout),
		goobs.WithEventSubscriptions(events),
		goobs.WithLogger(debugLogger{}),
	)
}

type goobsConn struct {
	client   *goobs.Client
	server   string
	password string
	version  string
	changed  chan struct{}
	done     chan struct{}
}

// listen ends when goobs closes IncomingEvents, which it does on a clean
// close from OBS. An abrupt drop doesn't close it; Ping catches that.
func (c *goobsConn) listen() {
	defer close(c.done)
	for ev := range c.client.IncomingEvents {
		switch ev.(type) {
		case *obsevents.SceneItemCreated, *obsevents.SceneItemRemoved,
			*obsevents.SceneCreated, *obsevents.SceneRemoved, *obsevents.SceneNameChanged,
			*obsevents.InputRemoved, *obsevents.InputNameChanged, *obsevents.InputSettingsChanged,
			*obsevents.CurrentSceneCollectionChanged:
			select {
			case c.changed <- struct{}{}:
			default:
			}
		}
	}
}

func (c *goobsConn) Version() string          { return c.version }
func (c *goobsConn) Changed() <-chan struct{} { return c.changed }
func (c *goobsConn) Done() <-chan struct{}    { return c.done }
func (c *goobsConn) Close() error             { return c.client.Disconnect() }

func (c *goobsConn) Ping() error {
	_, err := c.client.General.GetVersion(&general.GetVersionParams{})
	return err
}

func (c *goobsConn) SetItemEnabled(sceneUUID string, itemID int, enabled bool) error {
	_, err := c.client.SceneItems.SetSceneItemEnabled(&sceneitems.SetSceneItemEnabledParams{
		SceneUuid:        &sceneUUID,
		SceneItemId:      &itemID,
		SceneItemEnabled: &enabled,
	})
	return err
}

func (c *goobsConn) OpenInputSettings() (InputSettingsConn, error) {
	// goobs serializes requests per client, so a goroutine sharing c.client
	// would still block SetItemEnabled behind slow settings reads.
	client, err := dialClient(c.server, c.password, subscriptions.None)
	if err != nil {
		return nil, err
	}
	return &goobsInputSettings{client: client}, nil
}

type goobsInputSettings struct {
	client        *goobs.Client
	inputDefaults map[string]map[string]any // cached for this scan; worker only
}

func (c *goobsInputSettings) Close() error { return c.client.Disconnect() }

func (c *goobsInputSettings) InputSettings(item Item) (map[string]any, error) {
	params := &inputs.GetInputSettingsParams{}
	if item.UUID != "" {
		params.InputUuid = &item.UUID
	} else {
		params.InputName = &item.Name
	}
	res, err := c.client.Inputs.GetInputSettings(params)
	if err != nil {
		return nil, err
	}
	if c.inputDefaults == nil {
		c.inputDefaults = make(map[string]map[string]any)
	}
	defaults, cached := c.inputDefaults[res.InputKind]
	if !cached {
		resDefaults, err := c.client.Inputs.GetInputDefaultSettings(&inputs.GetInputDefaultSettingsParams{InputKind: &res.InputKind})
		if err != nil {
			return nil, err
		}
		defaults = resDefaults.DefaultInputSettings
		c.inputDefaults[res.InputKind] = defaults
	}
	return mergeInputSettings(defaults, res.InputSettings), nil
}

func (c *goobsConn) Scenes() ([]Scene, error) {
	list, err := c.client.Scenes.GetSceneList(&scenes.GetSceneListParams{})
	if err != nil {
		return nil, fmt.Errorf("list scenes: %w", err)
	}
	// OBS reports index 0 as the bottom of its scene list.
	ordered := slices.Clone(list.Scenes)
	slices.SortFunc(ordered, func(a, b *typedefs.Scene) int { return b.SceneIndex - a.SceneIndex })

	var out []Scene
	seenGroups := map[string]bool{}
	for _, s := range ordered {
		res, err := c.client.SceneItems.GetSceneItemList(&sceneitems.GetSceneItemListParams{SceneUuid: &s.SceneUuid})
		if err != nil {
			return nil, fmt.Errorf("list items of %q: %w", s.SceneName, err)
		}
		out = append(out, Scene{Name: s.SceneName, UUID: s.SceneUuid, Items: toItems(res.SceneItems)})

		for _, it := range res.SceneItems {
			if !it.IsGroup || seenGroups[it.SourceUuid] {
				continue
			}
			seenGroups[it.SourceUuid] = true
			// GetSceneItemList rejects groups ("Is group", code 602).
			g, err := c.client.SceneItems.GetGroupSceneItemList(&sceneitems.GetGroupSceneItemListParams{SceneUuid: &it.SourceUuid})
			if err != nil {
				return nil, fmt.Errorf("list items of group %q: %w", it.SourceName, err)
			}
			out = append(out, Scene{Name: it.SourceName, UUID: it.SourceUuid, IsGroup: true, Items: toItems(g.SceneItems)})
		}
	}
	return out, nil
}

// toItems orders items top-first, as the OBS source list shows them.
func toItems(in []*typedefs.SceneItem) []Item {
	sorted := slices.Clone(in)
	slices.SortFunc(sorted, func(a, b *typedefs.SceneItem) int { return b.SceneItemIndex - a.SceneItemIndex })
	out := make([]Item, 0, len(sorted))
	for _, it := range sorted {
		out = append(out, Item{
			ID:      it.SceneItemID,
			Name:    it.SourceName,
			UUID:    it.SourceUuid,
			Kind:    it.InputKind,
			IsGroup: it.IsGroup,
			Enabled: it.SceneItemEnabled,
		})
	}
	return out
}

// debugLogger routes goobs' chatter to slog at debug level.
type debugLogger struct{}

func (debugLogger) Printf(format string, v ...any) {
	slog.Debug(fmt.Sprintf(format, v...), "module", "obs")
}
