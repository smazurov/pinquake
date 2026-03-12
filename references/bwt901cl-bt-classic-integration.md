# BWT901CL Bluetooth Classic Integration

## Device Overview

The BWT901CL is a 9-axis IMU (accel + gyro + magnetometer) that uses **Bluetooth 2.0 Classic** with an HC-06 module, not BLE. It appears as `HC-06` in Bluetooth scans with default pairing PIN `1234`.

- **Model**: BWT901CL (label: BWT801CL-E 5-C-4)
- **Bluetooth**: 2.0 Classic, HC-06 module, SPP (Serial Port Profile)
- **MAC address** (this unit): `20:21:01:12:14:08`
- **USB interface**: CH341 UART on `/dev/ttyUSB0` (also works for data/config)
- **Protocol**: WIT standard serial protocol (same register map as WT901BLECL)
- **Default baud**: 115200 (BT) or 9600 (USB, configurable)

## Key Difference from WT901BLECL

| | WT901BLECL (current) | BWT901CL |
|---|---|---|
| Transport | BLE 5.0 GATT | Bluetooth 2.0 Classic RFCOMM |
| Linux API | tinygo bluetooth | `AF_BLUETOOTH` + `BTPROTO_RFCOMM` socket |
| Discovery | BLE scan, service UUID match | Classic BT scan, name = "HC-06" |
| Pairing | None (BLE just connects) | PIN-based (`1234`) |
| Data packets | 20-byte V1 (`0x55 0x61` + 9×int16) | 11-byte WIT standard (`0x55 0x5x` + 4×int16 + checksum) |
| Orientation data | Single packet has accel+gyro+euler | Split across 3+ packets (0x51, 0x52, 0x53) |
| Write commands | BLE characteristic write | Serial write to socket |
| Connection model | Persistent BLE connection | RFCOMM socket (connects on open) |

## Data Protocol

Uses the WIT standard protocol (see `wit-standard-protocol.md`). Data arrives as a stream of 11-byte packets:

```
0x55 | TYPE | D1L | D1H | D2L | D2H | D3L | D3H | D4L | D4H | CHECKSUM
```

Checksum = sum of bytes 0-9, low 8 bits only.

### Relevant packet types

**Acceleration (0x51)**:
```
Ax = int16(AxH<<8 | AxL) / 32768 * 16    (g)
Ay = int16(AyH<<8 | AyL) / 32768 * 16    (g)
Az = int16(AzH<<8 | AzL) / 32768 * 16    (g)
T  = int16(TH<<8 | TL) / 100             (°C)
```

**Angular Velocity (0x52)**:
```
Wx = int16(WxH<<8 | WxL) / 32768 * 2000  (°/s)
Wy = int16(WyH<<8 | WyL) / 32768 * 2000  (°/s)
Wz = int16(WzH<<8 | WzL) / 32768 * 2000  (°/s)
```

**Angle (0x53)**:
```
Roll  = int16(RollH<<8 | RollL) / 32768 * 180   (°)
Pitch = int16(PitchH<<8 | PitchL) / 32768 * 180  (°)
Yaw   = int16(YawH<<8 | YawL) / 32768 * 180      (°)
```

**Magnetic Field (0x54)**:
```
Hx, Hy, Hz = raw int16 values
```

### Assembling an Orientation

The existing `orientation.DecodeV1()` expects a single 20-byte BLE packet with all 9 fields. The BWT901CL sends them as separate packets. A decoder must accumulate packets 0x51 + 0x52 + 0x53 to build a complete `orientation.Orientation`:

```go
type packetAssembler struct {
    ax, ay, az       float32  // from 0x51
    wx, wy, wz       float32  // from 0x52
    roll, pitch, yaw float32  // from 0x53
    has              uint8    // bitmask: bit0=accel, bit1=gyro, bit2=angle
}

func (a *packetAssembler) Feed(pkt []byte) (*orientation.Orientation, bool) {
    // parse 11-byte packet, update fields, return complete Orientation
    // when all 3 types received (has == 0x07)
}
```

## Transport Layer

### Linux RFCOMM Socket (No rfcomm CLI needed)

The `rfcomm` userspace tool was removed from BlueZ on Fedora. Use Go's `syscall` or `golang.org/x/sys/unix` directly:

```go
fd, _ := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_STREAM, unix.BTPROTO_RFCOMM)

addr := &unix.SockaddrRFCOMM{
    Addr:    parseBTAddr("20:21:01:12:14:08"),  // [6]byte, reversed
    Channel: 1,                                  // HC-06 default SPP channel
}
unix.Connect(fd, addr)

// Now read/write like any file descriptor
file := os.NewFile(uintptr(fd), "rfcomm")
```

### Pairing

Must be paired via BlueZ/D-Bus before connecting. The HC-06 requires legacy PIN pairing with `1234`. This can be done:
- Manually: `bluetoothctl pair 20:21:01:12:14:08` + enter PIN
- Programmatically: D-Bus agent API (complex, probably not worth automating)

Once paired, the bond persists across reboots. The sensor just needs to be powered on.

### Stream Parsing

The RFCOMM socket delivers a raw byte stream (not framed like BLE notifications). Need a stream parser that:
1. Scans for sync byte `0x55`
2. Reads 10 more bytes
3. Validates checksum
4. Dispatches by type byte

```go
func readPackets(r io.Reader, handler func([]byte)) error {
    buf := make([]byte, 1024)
    var ring []byte
    for {
        n, err := r.Read(buf)
        if err != nil {
            return err
        }
        ring = append(ring, buf[:n]...)
        for len(ring) >= 11 {
            idx := bytes.IndexByte(ring, 0x55)
            if idx < 0 {
                ring = ring[:0]
                break
            }
            ring = ring[idx:]
            if len(ring) < 11 {
                break
            }
            pkt := ring[:11]
            if verifyChecksum(pkt) {
                handler(pkt)
            }
            ring = ring[11:]
        }
    }
}
```

## Architecture Integration

### Option A: New Sensor + Transport Abstraction

Refactor the `Sensor` interface to not depend on `*bluetooth.Device`:

```go
// Current interface (BLE-coupled)
type Sensor interface {
    Connect(device *bluetooth.Device, onOrientation func([]byte)) error
    // ...
}

// Possible generalized interface
type Sensor interface {
    Connect(transport Transport, onOrientation func([]byte)) error
    // ...
}

type Transport interface {
    io.ReadWriteCloser
    // or keep BLE and RFCOMM as separate Connect signatures
}
```

This is a significant refactor of the BLE scanner and both existing sensors.

### Option B: Separate Connection Path (Recommended)

Add a parallel `rfcomm` package alongside `ble/` that:
1. Manages RFCOMM socket lifecycle
2. Parses the WIT standard byte stream
3. Assembles 0x51+0x52+0x53 into `orientation.Orientation`
4. Publishes the same `OrientationEvent` to the shared `events.Bus`

```
internal/
  ble/          # existing BLE transport (unchanged)
  rfcomm/       # new classic BT transport
    rfcomm.go   # socket management, connect/disconnect
    parse.go    # WIT standard protocol stream parser
    assemble.go # packet assembler → orientation.Orientation
  sensors/      # sensor definitions (may need minor changes)
```

The `rfcomm.Connector` would:
- Accept an `events.Bus` (same as `ble.Scanner`)
- Publish `OrientationEvent`, `BatteryEvent`, `BLEStatusEvent`
- Implement connect/disconnect via the API server
- Use the same `orientation.BuildLockRotation` + `CurrentInReferenceFrame` pipeline

### Config Changes

```toml
[app.ble]
transport = 'rfcomm'            # new: 'ble' (default) or 'rfcomm'
device_address = '20:21:01:12:14:08'
device_name = 'HC-06'
sensor_name = 'BWT901CL'
baud = 115200                   # only relevant for USB serial fallback
```

### Register Read/Write over RFCOMM

Same WIT command format works over the RFCOMM socket (it's just a serial link):

```
Unlock:  FF AA 69 88 B5
Read:    FF AA 27 <addr> 00
Save:    FF AA 00 00 00
```

Response arrives in-band as a `0x55 0x5F` packet. Battery voltage is at register `0x64` (centavolts), temperature at `0x40` — same as the BLE variant.

## Verified Working

Bluetooth Classic connection from this machine confirmed working:

```python
import socket
sock = socket.socket(socket.AF_BLUETOOTH, socket.SOCK_STREAM, socket.BTPROTO_RFCOMM)
sock.connect(("20:21:01:12:14:08", 1))
# streams acceleration, gyro, angle, mag packets continuously
```

USB serial also works at `/dev/ttyUSB0` (CH341 driver, 115200 baud).

## Implementation Checklist

- [ ] Add `internal/rfcomm/` package with RFCOMM socket management
- [ ] Implement WIT standard protocol stream parser with checksum validation
- [ ] Implement packet assembler (0x51+0x52+0x53 → `orientation.Orientation`)
- [ ] Wire into `events.Bus` to publish `OrientationEvent`
- [ ] Add battery/temperature polling via register reads over RFCOMM
- [ ] Add `transport` config field, route to BLE or RFCOMM connector at startup
- [ ] Add `BWT901CL` to sensor registry (or handle outside registry since no BLE UUIDs)
- [ ] Handle reconnection (RFCOMM socket drops when sensor powers off)
- [ ] Test with auto-lock and reference frame pipeline
