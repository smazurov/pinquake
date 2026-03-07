package sensors

import (
	"errors"
	"fmt"

	"tinygo.org/x/bluetooth"
)

var ErrUnsupported = errors.New("unsupported by this sensor")

type BatteryState struct {
	Percent  uint8
	Volts    float32
	Charging bool
}

type Sensor interface {
	Name() string
	ServiceUUIDs() []bluetooth.UUID
	Connect(device *bluetooth.Device, onOrientation func([]byte)) error
	ReadBattery() (*BatteryState, error)
	ReadTemperature() (float32, error)
	Calibrate() error
	Close()
}

type SensorEntry struct {
	Name        string
	ServiceUUIDs []bluetooth.UUID
	Factory     func() Sensor
}

var Registry = []SensorEntry{
	{Name: "PinLevel", ServiceUUIDs: []bluetooth.UUID{pinlevelServiceParsedUUID}, Factory: NewPinLevel},
	{Name: "WT901", ServiceUUIDs: []bluetooth.UUID{wt901ServiceParsedUUID}, Factory: NewWT901},
}

func FactoryByName(name string) *SensorEntry {
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	return nil
}

func Match(result bluetooth.ScanResult) *SensorEntry {
	for i := range Registry {
		for _, uuid := range Registry[i].ServiceUUIDs {
			if result.HasServiceUUID(uuid) {
				return &Registry[i]
			}
		}
	}
	return nil
}

func mustParseUUID(s string) bluetooth.UUID {
	uuid, err := bluetooth.ParseUUID(s)
	if err != nil {
		panic(fmt.Sprintf("invalid UUID constant %q: %v", s, err))
	}
	return uuid
}
