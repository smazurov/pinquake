package ble

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/smazurov/pinquake/internal/events"
	"github.com/smazurov/pinquake/internal/orientation"
	"tinygo.org/x/bluetooth"
)

type State string

const (
	StateIdle       State = "idle"
	StateScanning   State = "scanning"
	StateConnecting State = "connecting"
	StateConnected  State = "connected"
)

type Scanner struct {
	adapter  *bluetooth.Adapter
	eventBus *events.Bus
	logger   *slog.Logger

	mu          sync.Mutex
	state       State
	device      *bluetooth.Device
	scanCancel  context.CancelFunc
	scanDone    chan struct{}
	refFrame    *orientation.Orientation
	pendingLock bool
	swapXY      bool
}

func NewScanner(eventBus *events.Bus, logger *slog.Logger) *Scanner {
	return &Scanner{
		adapter:  bluetooth.DefaultAdapter,
		eventBus: eventBus,
		logger:   logger,
		state:    StateIdle,
	}
}

func (s *Scanner) Init() error {
	return s.adapter.Enable()
}

func (s *Scanner) GetState() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// LockFrame requests that the next orientation sample be captured as the reference frame.
func (s *Scanner) LockFrame() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingLock = true
}

// UnlockFrame clears the reference frame.
func (s *Scanner) UnlockFrame() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refFrame = nil
	s.pendingLock = false
}

// IsFrameLocked returns whether a reference frame is set.
func (s *Scanner) IsFrameLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refFrame != nil
}

// SetSwapXY sets whether to swap X/Y axes.
func (s *Scanner) SetSwapXY(swap bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.swapXY = swap
}

func (s *Scanner) Scan(ctx context.Context) error {
	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return fmt.Errorf("cannot scan: state is %s", s.state)
	}
	s.state = StateScanning

	scanCtx, cancel := context.WithCancel(ctx)
	s.scanCancel = cancel
	s.scanDone = make(chan struct{})
	done := s.scanDone
	s.mu.Unlock()

	s.publishStatus("scanning", "")

	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			if s.state == StateScanning {
				s.state = StateIdle
			}
			s.scanCancel = nil
			s.scanDone = nil
			s.mu.Unlock()
			s.publishStatus("idle", "")
			close(done)
		}()

		adapterDone := make(chan error, 1)

		go func() {
			adapterDone <- s.adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
				name := result.LocalName()
				s.eventBus.Publish(events.BLEScanResultEvent{
					Address:   result.Address.String(),
					Name:      name,
					RSSI:      int(result.RSSI),
					Timestamp: time.Now().Format(time.RFC3339Nano),
				})
			})
		}()

		select {
		case <-scanCtx.Done():
			_ = s.adapter.StopScan()
			<-adapterDone
		case err := <-adapterDone:
			if err != nil {
				s.logger.Error("BLE scan failed", "error", err)
			}
		}
	}()

	return nil
}

// stopScan cancels any active scan and waits for it to finish.
// Must be called without s.mu held.
func (s *Scanner) stopScan() {
	s.mu.Lock()
	cancel := s.scanCancel
	done := s.scanDone
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *Scanner) Connect(addr string) error {
	s.stopScan()

	s.mu.Lock()
	if s.state != StateIdle {
		s.mu.Unlock()
		return fmt.Errorf("cannot connect: state is %s", s.state)
	}
	s.state = StateConnecting
	s.mu.Unlock()

	s.publishStatus("connecting", addr)

	go func() {
		if err := s.connectDevice(addr); err != nil {
			s.logger.Error("Failed to connect", "address", addr, "error", err)
			s.mu.Lock()
			s.state = StateIdle
			s.mu.Unlock()
			s.publishStatus("idle", err.Error())
		}
	}()

	return nil
}

func (s *Scanner) connectDevice(addr string) error {
	mac, err := bluetooth.ParseMAC(addr)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}

	address := bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}

	device, err := s.adapter.Connect(address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	handler := s.makeNotificationHandler()

	// Try PinLevel first, then WT901
	err = connectPinLevel(&device, handler)
	if err != nil {
		s.logger.Info("PinLevel connect failed, trying WT901", "error", err)
		err = connectWT901(&device, handler)
	}
	if err != nil {
		device.Disconnect()
		return fmt.Errorf("no compatible service found: %w", err)
	}

	s.mu.Lock()
	s.device = &device
	s.state = StateConnected
	s.mu.Unlock()

	s.logger.Info("Connected to BLE device", "address", addr)
	s.publishStatus("connected", addr)

	return nil
}

func (s *Scanner) makeNotificationHandler() func([]byte) {
	return func(buf []byte) {
		ori, ok := orientation.DecodeV1(buf)
		if !ok {
			return
		}

		s.mu.Lock()
		if s.pendingLock {
			ref := ori
			s.refFrame = &ref
			s.pendingLock = false
		}
		ref := s.refFrame
		swap := s.swapXY
		s.mu.Unlock()

		if ref != nil {
			ori = orientation.CurrentInReferenceFrame(&ori, ref)
		}

		gx := ori.Ax
		gy := ori.Ay
		gz := ori.Az
		if swap {
			gx, gy = gy, gx
		}

		s.eventBus.Publish(events.OrientationEvent{
			Gx:        gx,
			Gy:        gy,
			Gz:        gz,
			Timestamp: time.Now().Format(time.RFC3339Nano),
		})
	}
}

func (s *Scanner) Disconnect() error {
	s.mu.Lock()
	if s.state == StateIdle {
		s.mu.Unlock()
		return nil
	}
	if s.state != StateConnected || s.device == nil {
		s.mu.Unlock()
		return fmt.Errorf("cannot disconnect: state is %s", s.state)
	}
	device := s.device
	s.device = nil
	s.state = StateIdle
	s.refFrame = nil
	s.pendingLock = false
	s.mu.Unlock()

	err := device.Disconnect()
	s.publishStatus("disconnected", "")
	s.logger.Info("Disconnected from BLE device")
	return err
}

func (s *Scanner) publishStatus(status, device string) {
	s.eventBus.Publish(events.BLEStatusEvent{
		Status:    status,
		Device:    device,
		Timestamp: time.Now().Format(time.RFC3339Nano),
	})
}

