package sensors

import (
	"errors"

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

var Registry = []func() Sensor{NewPinLevel, NewWT901}

func FactoryByName(name string) func() Sensor {
	for _, factory := range Registry {
		if factory().Name() == name {
			return factory
		}
	}
	return nil
}

func Match(result bluetooth.ScanResult) func() Sensor {
	for _, factory := range Registry {
		s := factory()
		for _, uuid := range s.ServiceUUIDs() {
			if result.HasServiceUUID(uuid) {
				return factory
			}
		}
	}
	return nil
}
