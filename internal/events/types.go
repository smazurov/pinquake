package events

const (
	TypeOrientation uint32 = iota + 1
	TypeBLEStatus
	TypeBLEScanResult
	TypeConfigChanged
	TypeBattery
	TypeHeartbeat
	TypeLogEntry
	TypeVizTrigger
	TypeDelayedOrientation
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

// BLEStatusEvent reports BLE connection state changes.
type BLEStatusEvent struct {
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Device     string `json:"device,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	SensorName string `json:"sensor_name,omitempty"`
	Timestamp  string `json:"timestamp"`
}

func (e BLEStatusEvent) Type() uint32 { return TypeBLEStatus }

func (e BLEStatusEvent) DisplayName() string {
	if e.DeviceName != "" {
		return e.DeviceName
	}
	return e.Device
}

// BLEScanResultEvent carries a single BLE scan advertisement.
type BLEScanResultEvent struct {
	Address    string `json:"address"`
	Name       string `json:"name"`
	RSSI       int    `json:"rssi"`
	SensorName string `json:"sensor_name,omitempty"`
	Timestamp  string `json:"timestamp"`
}

func (e BLEScanResultEvent) Type() uint32 { return TypeBLEScanResult }

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
