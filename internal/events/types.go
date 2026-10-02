package events

const (
	TypeOrientation uint32 = iota + 1
	TypeBLEStatus
	TypeConfigChanged
	TypeBattery
	TypeHeartbeat
	TypeLogEntry
	TypeVizTrigger
	TypeDelayedOrientation
	TypeFrameState
	TypeOverlayVisibility
	TypeOBSStatus
)

type Event interface {
	Type() uint32
}

// OrientationEvent carries post-processed gravity components.
type OrientationEvent struct {
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	G         float32 `json:"g"`
	Timestamp string  `json:"timestamp"`
}

func (e OrientationEvent) Type() uint32 { return TypeOrientation }

// BLEStatusEvent reports the link to the chosen BLE device.
type BLEStatusEvent struct {
	Status     string  `json:"status" enum:"idle,searching,waiting,connecting,connected" doc:"idle: no device chosen; searching: scanning for it; waiting: backing off before searching again"`
	Reason     string  `json:"reason,omitempty" enum:"lost,user,shutdown" doc:"lost: the device dropped the link (kept while searching for it again); user: disconnected on request; shutdown: server stopping"`
	Device     string  `json:"device,omitempty" doc:"Address of the chosen device"`
	DeviceName string  `json:"device_name,omitempty"`
	SensorName string  `json:"sensor_name,omitempty"`
	Error      string  `json:"error,omitempty" doc:"Why the last attempt failed (waiting)"`
	RetryInS   float64 `json:"retry_in_s,omitempty" doc:"Seconds until the next attempt (waiting)"`
	Timestamp  string  `json:"timestamp"`
}

func (e BLEStatusEvent) Type() uint32 { return TypeBLEStatus }

func (e BLEStatusEvent) DisplayName() string {
	if e.DeviceName != "" {
		return e.DeviceName
	}
	return e.Device
}

// ConfigChangedEvent is published when config is updated.
type ConfigChangedEvent struct {
	Section   string `json:"section"`
	Timestamp string `json:"timestamp"`
}

func (e ConfigChangedEvent) Type() uint32 { return TypeConfigChanged }

// BatteryEvent carries periodic battery level and charging state.
type BatteryEvent struct {
	BatteryPercent uint8   `json:"battery_percent"`
	BatteryVolts   float32 `json:"battery_volts"`
	Charging       bool    `json:"charging"`
	Timestamp      string  `json:"timestamp"`
}

func (e BatteryEvent) Type() uint32 { return TypeBattery }

// HeartbeatEvent is a server-sent keepalive for staleness detection.
type HeartbeatEvent struct {
	Timestamp string `json:"timestamp"`
}

func (e HeartbeatEvent) Type() uint32 { return TypeHeartbeat }

// LogEntry is a generic log event for the persistent event log.
type LogEntry struct {
	Message   string `json:"message"`
	Level     string `json:"level"`
	Timestamp string `json:"timestamp"`
}

func (e LogEntry) Type() uint32 { return TypeLogEntry }

// VizTriggerEvent tells the frontend to show or hide the visualization.
type VizTriggerEvent struct {
	Visible   bool   `json:"visible"`
	Class     string `json:"class"`
	Timestamp string `json:"timestamp"`
}

func (e VizTriggerEvent) Type() uint32 { return TypeVizTrigger }

// DelayedOrientationEvent carries time-shifted sensor data for display sync.
type DelayedOrientationEvent struct {
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	G         float32 `json:"g"`
	Timestamp string  `json:"timestamp"`
}

func (e DelayedOrientationEvent) Type() uint32 { return TypeDelayedOrientation }

// FrameStateEvent reports reference-frame lock state. Reason is set only when
// the event announces a new lock.
type FrameStateEvent struct {
	Enabled   bool    `json:"enabled" doc:"Auto-lock enabled"`
	State     string  `json:"state" enum:"unlocked,settling,locked"`
	Reason    string  `json:"reason,omitempty" enum:"stable,drift,trigger,settle-timeout" doc:"Why a new lock was applied"`
	Stdev     float32 `json:"stdev" doc:"Max per-axis stdev over the window (g)"`
	Drift     float32 `json:"drift,omitempty" doc:"Distance from the previous gravity reference (g)"`
	Timestamp string  `json:"timestamp"`
}

func (e FrameStateEvent) Type() uint32 { return TypeFrameState }

// OverlayVisibilityEvent tells the browser overlays whether to draw. They
// follow the trigger, except while OBS is driving a target: then they stay up
// and OBS shows and hides them.
type OverlayVisibilityEvent struct {
	Visible   bool   `json:"visible"`
	Timestamp string `json:"timestamp"`
}

func (e OverlayVisibilityEvent) Type() uint32 { return TypeOverlayVisibility }

// OBSStatusEvent reports the obs-websocket session.
type OBSStatusEvent struct {
	State         string  `json:"state" enum:"off,connecting,waiting,connected,active"`
	Server        string  `json:"server,omitempty"`
	OBSVersion    string  `json:"obs_version,omitempty"`
	Error         string  `json:"error,omitempty" doc:"Why the last attempt failed"`
	RetryInS      float64 `json:"retry_in_s,omitempty" doc:"Seconds until the next attempt"`
	Target        string  `json:"target,omitempty" doc:"Scene › Source being driven"`
	TargetMissing bool    `json:"target_missing,omitempty" doc:"Target not found in OBS"`
	Timestamp     string  `json:"timestamp"`
}

func (e OBSStatusEvent) Type() uint32 { return TypeOBSStatus }
