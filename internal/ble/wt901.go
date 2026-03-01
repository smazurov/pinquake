package ble

import (
	"fmt"

	"tinygo.org/x/bluetooth"
)

const (
	wt901ServiceUUID = "0000FFE0-0000-1000-8000-00805F9B34FB"
	wt901CharUUID    = "0000FFE5-0000-1000-8000-00805F9B34FB"
)

func connectWT901(device *bluetooth.Device, handler func([]byte)) error {
	svcUUID, err := bluetooth.ParseUUID(wt901ServiceUUID)
	if err != nil {
		return fmt.Errorf("parse service UUID: %w", err)
	}

	svcs, err := device.DiscoverServices([]bluetooth.UUID{svcUUID})
	if err != nil || len(svcs) == 0 {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	charUUID, _ := bluetooth.ParseUUID(wt901CharUUID)
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{charUUID})
	if err != nil || len(chars) == 0 {
		return fmt.Errorf("characteristic discovery failed: %w", err)
	}

	if err := chars[0].EnableNotifications(handler); err != nil {
		return fmt.Errorf("enable notifications: %w", err)
	}
	return nil
}
