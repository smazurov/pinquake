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

var (
	wt901ServiceParsedUUID = mustParseUUID(wt901ServiceUUID)
	wt901NotifyParsedUUID  = mustParseUUID(wt901NotifyCharUUID)
	wt901WriteParsedUUID   = mustParseUUID(wt901WriteCharUUID)
)

// charWriter is the write characteristic (bluetooth.DeviceCharacteristic).
type charWriter interface {
	WriteWithoutResponse(p []byte) (int, error)
}

// ioTimeout bounds one register transaction, including waiting for the
// characteristic. tinygo's writes are blocking D-Bus calls with no timeout.
const ioTimeout = 5 * time.Second

type WT901 struct {
	writeChar charWriter
	respCh    chan []byte

	// ioSem (one slot) serializes register transactions (unlock →
	// write/read → response). Config apply and battery polling run
	// concurrently after connect; interleaved, they would steal each
	// other's responses from respCh.
	ioSem          chan struct{}
	lastCentavolts uint16
}

func NewWT901() Sensor { return &WT901{ioSem: make(chan struct{}, 1)} }

// transact runs fn with exclusive use of the characteristic, returning an
// error if that takes longer than ioTimeout. fn keeps the characteristic
// until it actually returns, so a timed-out transaction never interleaves
// with the next one; later callers fail fast instead of blocking.
func (w *WT901) transact(fn func() error) error {
	timeout := time.NewTimer(ioTimeout)
	defer timeout.Stop()
	select {
	case w.ioSem <- struct{}{}:
	case <-timeout.C:
		return fmt.Errorf("register I/O busy for %s", ioTimeout)
	}
	done := make(chan error, 1)
	go func() {
		defer func() { <-w.ioSem }()
		done <- fn()
	}()
	select {
	case err := <-done:
		return err
	case <-timeout.C:
		return fmt.Errorf("register I/O timed out after %s", ioTimeout)
	}
}

func (w *WT901) Name() string { return "WT901" }

func (w *WT901) ServiceUUIDs() []bluetooth.UUID {
	return []bluetooth.UUID{wt901ServiceParsedUUID}
}

func (w *WT901) Connect(device *bluetooth.Device, onOrientation func([]byte)) error {
	svcs, err := device.DiscoverServices([]bluetooth.UUID{wt901ServiceParsedUUID})
	if err != nil || len(svcs) == 0 {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{wt901NotifyParsedUUID, wt901WriteParsedUUID})
	if err != nil || len(chars) < 2 {
		return fmt.Errorf("characteristic discovery failed: %w", err)
	}

	var notifyChar, writeChar bluetooth.DeviceCharacteristic
	var foundNotify, foundWrite bool
	for _, c := range chars {
		switch c.UUID() {
		case wt901NotifyParsedUUID:
			notifyChar = c
			foundNotify = true
		case wt901WriteParsedUUID:
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
	var st *BatteryState
	err := w.transact(func() error {
		if err := w.unlock(); err != nil {
			return err
		}
		data, err := w.readRegister(0x64)
		if err != nil {
			return fmt.Errorf("battery voltage: %w", err)
		}
		centavolts := binary.LittleEndian.Uint16(data[0:2])

		var charging bool
		if w.lastCentavolts != 0 {
			charging = centavolts > w.lastCentavolts+10
		}
		w.lastCentavolts = centavolts

		st = &BatteryState{
			Percent:  batteryPercent(centavolts),
			Volts:    float32(centavolts) / 100.0,
			Charging: charging,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (w *WT901) ReadTemperature() (float32, error) {
	var temp float32
	err := w.transact(func() error {
		if err := w.unlock(); err != nil {
			return err
		}
		data, err := w.readRegister(0x40)
		if err != nil {
			return fmt.Errorf("temperature: %w", err)
		}
		temp = float32(int16(binary.LittleEndian.Uint16(data[0:2]))) / 100.0
		return nil
	})
	return temp, err
}

func (w *WT901) Calibrate() error { return ErrUnsupported }

func (w *WT901) Close() {}

func (w *WT901) writeRegister(addr, lo, hi byte) error {
	cmd := []byte{0xFF, 0xAA, addr, lo, hi}
	_, err := w.writeChar.WriteWithoutResponse(cmd)
	if err != nil {
		return fmt.Errorf("write register 0x%02X: %w", addr, err)
	}
	return nil
}

func (w *WT901) save() error {
	cmd := []byte{0xFF, 0xAA, 0x00, 0x00, 0x00}
	_, err := w.writeChar.WriteWithoutResponse(cmd)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

// ReadBatteryBlock reads registers 0x5C-0x6B for debug purposes.
func (w *WT901) ReadBatteryBlock() (map[string]uint16, error) {
	result := map[string]uint16{}
	err := w.transact(func() error {
		if err := w.unlock(); err != nil {
			return err
		}
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
		return nil
	})
	if err != nil {
		return nil, err
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
