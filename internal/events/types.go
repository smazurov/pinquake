package events

const (
	TypeOrientation uint32 = iota + 1
	TypeBLEStatus
	TypeBLEScanResult
	TypeConfigChanged
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
	Status    string `json:"status"`
	Device    string `json:"device,omitempty"`
	Timestamp string `json:"timestamp"`
}

func (e BLEStatusEvent) Type() uint32 { return TypeBLEStatus }

// BLEScanResultEvent carries a single BLE scan advertisement.
type BLEScanResultEvent struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	RSSI      int    `json:"rssi"`
	Timestamp string `json:"timestamp"`
}

func (e BLEScanResultEvent) Type() uint32 { return TypeBLEScanResult }

// ConfigChangedEvent is published when config is updated.
type ConfigChangedEvent struct {
	Timestamp string `json:"timestamp"`
}

func (e ConfigChangedEvent) Type() uint32 { return TypeConfigChanged }
