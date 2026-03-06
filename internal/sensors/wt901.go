package sensors

import (
	"encoding/binary"
	"fmt"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	wt901ServiceUUID    = "0000FFE5-0000-1000-8000-00805F9A34FB"
	wt901NotifyCharUUID = "0000FFE4-0000-1000-8000-00805F9A34FB"
	wt901WriteCharUUID  = "0000FFE9-0000-1000-8000-00805F9A34FB"
)

type WT901 struct {
	writeChar      bluetooth.DeviceCharacteristic
	respCh         chan []byte
	lastCentavolts uint16
}

func NewWT901() Sensor { return &WT901{} }

func (w *WT901) Name() string { return "WT901" }

func (w *WT901) ServiceUUIDs() []bluetooth.UUID {
	uuid, _ := bluetooth.ParseUUID(wt901ServiceUUID)
	return []bluetooth.UUID{uuid}
}

func (w *WT901) Connect(device *bluetooth.Device, onOrientation func([]byte)) error {
	svcUUID, err := bluetooth.ParseUUID(wt901ServiceUUID)
	if err != nil {
		return fmt.Errorf("parse service UUID: %w", err)
	}

	svcs, err := device.DiscoverServices([]bluetooth.UUID{svcUUID})
	if err != nil || len(svcs) == 0 {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	notifyUUID, _ := bluetooth.ParseUUID(wt901NotifyCharUUID)
	writeUUID, _ := bluetooth.ParseUUID(wt901WriteCharUUID)
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{notifyUUID, writeUUID})
	if err != nil || len(chars) < 2 {
		return fmt.Errorf("characteristic discovery failed: %w", err)
	}

	var notifyChar, writeChar bluetooth.DeviceCharacteristic
	var foundNotify, foundWrite bool
	for _, c := range chars {
		switch c.UUID() {
		case notifyUUID:
			notifyChar = c
			foundNotify = true
		case writeUUID:
			writeChar = c
			foundWrite = true
		}
	}
	if !foundNotify || !foundWrite {
		return fmt.Errorf("required characteristics not found")
	}

	w.writeChar = writeChar
	w.respCh = make(chan []byte, 8)

	err = notifyChar.EnableNotifications(func(buf []byte) {
		if len(buf) >= 2 && buf[0] == 0x55 && buf[1] == 0x71 {
			select {
			case w.respCh <- append([]byte(nil), buf...):
			default:
			}
			return
		}
		onOrientation(buf)
	})
	if err != nil {
		return fmt.Errorf("enable notifications: %w", err)
	}

	return nil
}

func (w *WT901) ReadBattery() (*BatteryState, error) {
	if err := w.unlock(); err != nil {
		return nil, err
	}
	data, err := w.readRegister(0x64)
	if err != nil {
		return nil, fmt.Errorf("battery voltage: %w", err)
	}
	centavolts := binary.LittleEndian.Uint16(data[0:2])

	var charging bool
	if w.lastCentavolts == 0 {
		charging = false
	} else {
		charging = centavolts > w.lastCentavolts+10
	}
	w.lastCentavolts = centavolts

	return &BatteryState{
		Percent:  batteryPercent(centavolts),
		Volts:    float32(centavolts) / 100.0,
		Charging: charging,
	}, nil
}

func (w *WT901) ReadTemperature() (float32, error) {
	if err := w.unlock(); err != nil {
		return 0, err
	}
	data, err := w.readRegister(0x40)
	if err != nil {
		return 0, fmt.Errorf("temperature: %w", err)
	}
	return float32(int16(binary.LittleEndian.Uint16(data[0:2]))) / 100.0, nil
}

func (w *WT901) Calibrate() error { return ErrUnsupported }

func (w *WT901) Close() {}

// ReadBatteryBlock reads registers 0x5C-0x6B for debug purposes.
func (w *WT901) ReadBatteryBlock() (map[string]uint16, error) {
	if err := w.unlock(); err != nil {
		return nil, err
	}
	result := map[string]uint16{}
	for _, base := range []byte{0x5C, 0x64} {
		data, err := w.readRegister(base)
		if err != nil {
			continue
		}
		for i := 0; i < 8; i++ {
			key := fmt.Sprintf("0x%02X", int(base)+i)
			result[key] = binary.LittleEndian.Uint16(data[i*2 : i*2+2])
		}
	}
	return result, nil
}

func (w *WT901) unlock() error {
	cmd := []byte{0xFF, 0xAA, 0x69, 0x88, 0xB5}
	_, err := w.writeChar.WriteWithoutResponse(cmd)
	if err != nil {
		return fmt.Errorf("unlock: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (w *WT901) readRegister(addr byte) ([]byte, error) {
	for {
		select {
		case <-w.respCh:
		default:
			goto drained
		}
	}
drained:

	cmd := []byte{0xFF, 0xAA, 0x27, addr, 0x00}
	_, err := w.writeChar.WriteWithoutResponse(cmd)
	if err != nil {
		return nil, fmt.Errorf("write command: %w", err)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case resp := <-w.respCh:
			if len(resp) < 20 {
				continue
			}
			if resp[2] != addr {
				continue
			}
			return resp[4:20], nil
		case <-timeout:
			return nil, fmt.Errorf("timeout reading register 0x%02X", addr)
		}
	}
}

// batteryPercent converts centavolts to percentage using the official BLE 5.0 table.
func batteryPercent(centavolts uint16) uint8 {
	switch {
	case centavolts > 396:
		return 100
	case centavolts >= 393:
		return 90
	case centavolts >= 387:
		return 75
	case centavolts >= 382:
		return 60
	case centavolts >= 379:
		return 50
	case centavolts >= 377:
		return 40
	case centavolts >= 373:
		return 30
	case centavolts >= 370:
		return 20
	case centavolts >= 368:
		return 15
	case centavolts >= 350:
		return 10
	case centavolts >= 340:
		return 5
	default:
		return 0
	}
}
