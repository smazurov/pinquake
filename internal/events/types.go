package events

const (
	TypeOrientation uint32 = iota + 1
	TypeBLEStatus
	TypeBLEScanResult
	TypeConfigChanged
	TypeBattery
	TypeHeartbeat
	TypeLogEntry
)

type Event interface {
	Type() uint32
}

// OrientationEvent carries post-processed gravity components.
type OrientationEvent struct {
	Gx        float32 `json:"gx"`
	Gy        float32 `json:"gy"`
	Gz        float32 `json:"gz"`
	Timestamp string  `json:"timestamp"`
}

func (e OrientationEvent) Type() uint32 { return TypeOrientation }

// BLEStatusEvent reports BLE connection state changes.
type BLEStatusEvent struct {
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Device     string `json:"device,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
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
