package ble

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

const sensorPath = adapterPath + "/dev_EA_F0_F1_BC_59_DD"

func interfacesAdded(path dbus.ObjectPath, props map[string]dbus.Variant) *dbus.Signal {
	return &dbus.Signal{
		Path: "/",
		Name: "org.freedesktop.DBus.ObjectManager.InterfacesAdded",
		Body: []any{path, map[string]map[string]dbus.Variant{"org.bluez.Device1": props}},
	}
}

func sensorProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"Address": dbus.MakeVariant(tracker),
		"Name":    dbus.MakeVariant("WT901BLE68"),
		"RSSI":    dbus.MakeVariant(int16(-60)),
		"UUIDs":   dbus.MakeVariant([]string{"0000ffe5-0000-1000-8000-00805f9a34fb"}),
	}
}

func TestScanReportsNewDevice(t *testing.T) {
	d := discovered{}
	adv, ok, err := d.handle(interfacesAdded(sensorPath, sensorProps()))
	if err != nil || !ok {
		t.Fatalf("handle = ok %v, err %v; want an advertisement", ok, err)
	}
	want := Advertisement{Address: tracker, Name: "WT901BLE68", RSSI: -60, SensorName: "WT901"}
	if adv != want {
		t.Fatalf("got %+v, want %+v", adv, want)
	}
}

func propertiesChanged(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant, invalidated ...string) *dbus.Signal {
	return &dbus.Signal{
		Path: path,
		Name: "org.freedesktop.DBus.Properties.PropertiesChanged",
		Body: []any{iface, changed, invalidated},
	}
}

// managed is a GetManagedObjects reply holding the sensor, as BlueZ
// remembers it from an earlier scan: no RSSI until it is seen again.
func managed() map[dbus.ObjectPath]map[string]map[string]dbus.Variant {
	props := sensorProps()
	delete(props, "RSSI")
	return map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
		adapterPath: {"org.bluez.Adapter1": {}},
		sensorPath:  {"org.bluez.Device1": props},
	}
}

func TestScanReportsKnownDeviceWhenSeenAgain(t *testing.T) {
	d := newDiscovered(managed())
	adv, ok, err := d.handle(propertiesChanged(sensorPath, "org.bluez.Device1",
		map[string]dbus.Variant{"RSSI": dbus.MakeVariant(int16(-55))}))
	if err != nil || !ok {
		t.Fatalf("handle = ok %v, err %v; want an advertisement", ok, err)
	}
	want := Advertisement{Address: tracker, Name: "WT901BLE68", RSSI: -55, SensorName: "WT901"}
	if adv != want {
		t.Fatalf("got %+v, want %+v", adv, want)
	}
}

func TestScanIgnoresChangesThatAreNotAdvertisements(t *testing.T) {
	d := newDiscovered(managed())
	for _, changed := range []map[string]dbus.Variant{
		{"Connected": dbus.MakeVariant(false)},
		{"ServicesResolved": dbus.MakeVariant(false)},
	} {
		if _, ok, _ := d.handle(propertiesChanged(sensorPath, "org.bluez.Device1", changed)); ok {
			t.Errorf("%v reported as an advertisement", changed)
		}
	}
}

func TestScanEndsWhenAdapterPoweredOff(t *testing.T) {
	d := newDiscovered(managed())
	// Discovering drops whenever another client's session ends; not ours.
	if _, _, err := d.handle(propertiesChanged(adapterPath, "org.bluez.Adapter1",
		map[string]dbus.Variant{"Discovering": dbus.MakeVariant(false)})); err != nil {
		t.Fatalf("Discovering=false ended the scan: %v", err)
	}
	if _, _, err := d.handle(propertiesChanged(adapterPath, "org.bluez.Adapter1",
		map[string]dbus.Variant{"Powered": dbus.MakeVariant(false)})); err == nil {
		t.Fatal("scan kept going with the adapter powered off")
	}
}
