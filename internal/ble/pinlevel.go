package ble

import (
	"fmt"

	"tinygo.org/x/bluetooth"
)

const (
	pinlevelServiceUUID     = "706c7600-7069-6e6c-6576-656c00000001"
	pinlevelOrientationUUID = "706c7600-7069-6e6c-6576-656c00000002"
)

func connectPinLevel(device *bluetooth.Device, handler func([]byte)) error {
	svcUUID, err := bluetooth.ParseUUID(pinlevelServiceUUID)
	if err != nil {
		return fmt.Errorf("parse service UUID: %w", err)
	}

	svcs, err := device.DiscoverServices([]bluetooth.UUID{svcUUID})
	if err != nil || len(svcs) == 0 {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	charUUID, _ := bluetooth.ParseUUID(pinlevelOrientationUUID)
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{charUUID})
	if err != nil || len(chars) == 0 {
		return fmt.Errorf("characteristic discovery failed: %w", err)
	}

	if err := chars[0].EnableNotifications(handler); err != nil {
		return fmt.Errorf("enable notifications: %w", err)
	}
	return nil
}
