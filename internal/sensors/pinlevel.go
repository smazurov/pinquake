package sensors

import (
	"fmt"

	"tinygo.org/x/bluetooth"
)

const (
	pinlevelServiceUUID     = "706c7600-7069-6e6c-6576-656c00000001"
	pinlevelOrientationUUID = "706c7600-7069-6e6c-6576-656c00000002"
)

var (
	pinlevelServiceParsedUUID     = mustParseUUID(pinlevelServiceUUID)
	pinlevelOrientationParsedUUID = mustParseUUID(pinlevelOrientationUUID)
)

type PinLevel struct{}

func NewPinLevel() Sensor { return &PinLevel{} }

func (p *PinLevel) Name() string { return "PinLevel" }

func (p *PinLevel) ServiceUUIDs() []bluetooth.UUID {
	return []bluetooth.UUID{pinlevelServiceParsedUUID}
}

func (p *PinLevel) Connect(device *bluetooth.Device, onOrientation func([]byte)) error {
	svcs, err := device.DiscoverServices([]bluetooth.UUID{pinlevelServiceParsedUUID})
	if err != nil || len(svcs) == 0 {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{pinlevelOrientationParsedUUID})
	if err != nil || len(chars) == 0 {
		return fmt.Errorf("characteristic discovery failed: %w", err)
	}

	if err := chars[0].EnableNotifications(onOrientation); err != nil {
		return fmt.Errorf("enable notifications: %w", err)
	}
	return nil
}

func (p *PinLevel) ReadBattery() (*BatteryState, error)  { return nil, ErrUnsupported }
func (p *PinLevel) ReadTemperature() (float32, error)     { return 0, ErrUnsupported }
func (p *PinLevel) Calibrate() error                      { return ErrUnsupported }
func (p *PinLevel) Close()                                {}
