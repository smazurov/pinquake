package ble

import (
	"context"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/smazurov/pinquake/internal/events"
)

func (s *Scanner) connContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connCtx
}

func (s *Scanner) watchConnection(deviceAddr string) {
	bus, err := dbus.SystemBus()
	if err != nil {
		s.logger.Warn("Failed to open D-Bus for connection watch", "error", err)
		return
	}

	devPath := "/org/bluez/hci0/dev_" + strings.ReplaceAll(deviceAddr, ":", "_")
	matchOpts := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath(dbus.ObjectPath(devPath)),
	}
	if err := bus.AddMatchSignal(matchOpts...); err != nil {
		s.logger.Warn("Failed to add D-Bus match signal", "error", err)
		return
	}

	sigCh := make(chan *dbus.Signal, 4)
	bus.Signal(sigCh)
	defer bus.RemoveSignal(sigCh)
	defer bus.RemoveMatchSignal(matchOpts...)

	s.logger.Info("Watching D-Bus for disconnect", "path", devPath)

	ctx := s.connContext()

	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-sigCh:
			if sig.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" {
				continue
			}
			iface, _ := sig.Body[0].(string)
			if iface != "org.bluez.Device1" {
				continue
			}
			changes, _ := sig.Body[1].(map[string]dbus.Variant)
			connVar, ok := changes["Connected"]
			if !ok {
				continue
			}
			connected, _ := connVar.Value().(bool)
			if connected {
				continue
			}

			s.logger.Warn("D-Bus reports device disconnected")

			s.mu.Lock()
			if s.disconnecting || s.state != StateConnected {
				s.mu.Unlock()
				return
			}
			s.resetConnectionState()
			s.mu.Unlock()

			s.publishStatus("disconnected", "", "lost", "")
			return
		}
	}
}

func (s *Scanner) pollBattery() {
	ctx := s.connContext()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	s.readAndPublishBattery()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.readAndPublishBattery()
		}
	}
}

func (s *Scanner) readAndPublishBattery() {
	sen := s.Sensor()
	if sen == nil {
		return
	}
	bat, err := sen.ReadBattery()
	if err != nil {
		s.logger.Warn("Battery poll failed", "error", err)
		return
	}

	s.eventBus.Publish(events.BatteryEvent{
		BatteryPercent: bat.Percent,
		BatteryVolts:   bat.Volts,
		Charging:       bat.Charging,
		Timestamp:      time.Now().Format(time.RFC3339Nano),
	})
}
