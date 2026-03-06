package obs

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/closecodes"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/gorilla/websocket"
	"github.com/smazurov/pinquake/internal/events"
)

var (
	ErrAuthFailed = errors.New("authentication failed")
	ErrTimeout    = errors.New("connection timed out")
)

type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
)

type BrowserSource struct {
	SceneName   string `json:"scene_name"`
	SourceName  string `json:"source_name"`
	URL         string `json:"url"`
	SceneItemID int    `json:"scene_item_id"`
	Enabled     bool   `json:"enabled"`
}

type Client struct {
	mu       sync.Mutex
	client   *goobs.Client
	eventBus *events.Bus
	logger   *slog.Logger
	state    State

	host     string
	port     int
	password string

	reconnectStop chan struct{}
}

func NewClient(bus *events.Bus, logger *slog.Logger) *Client {
	return &Client{
		eventBus: bus,
		logger:   logger,
		state:    StateDisconnected,
	}
}

func (c *Client) GetState() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Client) Connect(host string, port int, password string) error {
	c.mu.Lock()
	if c.state == StateConnected || c.state == StateConnecting {
		c.mu.Unlock()
		return fmt.Errorf("already %s", c.state)
	}
	c.state = StateConnecting
	c.host = host
	c.port = port
	c.password = password
	c.mu.Unlock()

	c.publishStatus(StateConnecting, "user")

	if err := c.doConnect(host, port, password); err != nil {
		c.logger.Error("OBS connect failed", "error", err)
		c.mu.Lock()
		c.state = StateDisconnected
		c.mu.Unlock()
		c.publishStatus(StateDisconnected, "error")
		return err
	}

	return nil
}

func (c *Client) doConnect(host string, port int, password string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	opts := []goobs.Option{goobs.WithPassword(password)}

	client, err := goobs.New(addr, opts...)
	if err != nil {
		return classifyConnectError(err)
	}

	c.mu.Lock()
	c.client = client
	c.state = StateConnected
	c.mu.Unlock()

	c.logger.Info("Connected to OBS", "address", addr)
	c.publishStatus(StateConnected, "user")

	c.startReconnectLoop()

	return nil
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	client := c.client
	stop := c.reconnectStop
	c.client = nil
	c.state = StateDisconnected
	c.reconnectStop = nil
	c.mu.Unlock()

	if stop != nil {
		close(stop)
	}
	if client != nil {
		_ = client.Disconnect()
	}

	c.logger.Info("Disconnected from OBS")
	c.publishStatus(StateDisconnected, "user")
}

func (c *Client) AutoConnect(host string, port int, password string) {
	if host == "" {
		return
	}
	_ = c.Connect(host, port, password)
}

func (c *Client) startReconnectLoop() {
	c.mu.Lock()
	if c.reconnectStop != nil {
		close(c.reconnectStop)
	}
	stop := make(chan struct{})
	c.reconnectStop = stop
	c.mu.Unlock()

	go c.reconnectLoop(stop)
}

func (c *Client) reconnectLoop(stop chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.mu.Lock()
			client := c.client
			state := c.state
			c.mu.Unlock()

			if state != StateConnected || client == nil {
				return
			}

			// Ping OBS to check connection
			_, err := client.Scenes.GetSceneList()
			if err != nil {
				c.logger.Warn("OBS connection lost, reconnecting", "error", err)
				c.mu.Lock()
				c.client = nil
				c.state = StateDisconnected
				host := c.host
				port := c.port
				password := c.password
				c.mu.Unlock()

				c.publishStatus(StateDisconnected, "lost")

				c.reconnectWithBackoff(stop, host, port, password)
				return
			}
		}
	}
}

func (c *Client) reconnectWithBackoff(stop chan struct{}, host string, port int, password string) {
	delay := 2 * time.Second
	maxDelay := 30 * time.Second

	for {
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}

		c.mu.Lock()
		c.state = StateConnecting
		c.mu.Unlock()
		c.publishStatus(StateConnecting, "reconnect")

		if err := c.doConnect(host, port, password); err != nil {
			c.logger.Warn("OBS reconnect failed", "error", err, "next_retry", delay*2)
			c.mu.Lock()
			c.state = StateDisconnected
			c.mu.Unlock()
			c.publishStatus(StateDisconnected, "error")

			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}
		return
	}
}

func (c *Client) ListBrowserSources() ([]BrowserSource, error) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("not connected to OBS")
	}

	sceneList, err := client.Scenes.GetSceneList()
	if err != nil {
		return nil, fmt.Errorf("failed to get scenes: %w", err)
	}

	var sources []BrowserSource

	for _, scene := range sceneList.Scenes {
		items, err := client.SceneItems.GetSceneItemList(
			sceneitems.NewGetSceneItemListParams().WithSceneName(scene.SceneName),
		)
		if err != nil {
			c.logger.Warn("Failed to get scene items", "scene", scene.SceneName, "error", err)
			continue
		}

		for _, item := range items.SceneItems {
			if item.InputKind != "browser_source" {
				continue
			}

			url := ""
			settings, err := client.Inputs.GetInputSettings(
				inputs.NewGetInputSettingsParams().WithInputName(item.SourceName),
			)
			if err == nil {
				if u, ok := settings.InputSettings["url"].(string); ok {
					url = u
				}
			}

			sources = append(sources, BrowserSource{
				SceneName:   scene.SceneName,
				SourceName:  item.SourceName,
				URL:         url,
				SceneItemID: item.SceneItemID,
				Enabled:     item.SceneItemEnabled,
			})
		}
	}

	return sources, nil
}

func (c *Client) SetSourceEnabled(sceneName string, sourceName string, enabled bool) error {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected to OBS")
	}

	items, err := client.SceneItems.GetSceneItemList(
		sceneitems.NewGetSceneItemListParams().WithSceneName(sceneName),
	)
	if err != nil {
		return fmt.Errorf("failed to get scene items: %w", err)
	}

	for _, item := range items.SceneItems {
		if item.SourceName == sourceName {
			_, err := client.SceneItems.SetSceneItemEnabled(
				sceneitems.NewSetSceneItemEnabledParams().
					WithSceneName(sceneName).
					WithSceneItemId(item.SceneItemID).
					WithSceneItemEnabled(enabled),
			)
			return err
		}
	}

	return fmt.Errorf("source %q not found in scene %q", sourceName, sceneName)
}

func (c *Client) publishStatus(state State, reason string) {
	c.eventBus.Publish(events.OBSStatusEvent{
		Status:    string(state),
		Reason:    reason,
		Timestamp: time.Now().Format(time.RFC3339Nano),
	})
}

func classifyConnectError(err error) error {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		if closeErr.Code == closecodes.AuthenticationFailed {
			return fmt.Errorf("%w: %s", ErrAuthFailed, closeErr.Text)
		}
	}
	if strings.Contains(err.Error(), "timeout") {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return err
}

func (c *Client) HandleConfigChange(host string, port int, password string) {
	c.mu.Lock()
	hostChanged := host != c.host || port != c.port || password != c.password
	c.mu.Unlock()

	if hostChanged && host != "" {
		c.Disconnect()
		_ = c.Connect(host, port, password)
	}
}
