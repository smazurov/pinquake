# WT901BLECL5.0 BLE Protocol Reference

## BLE GATT Layout

| UUID | Type | Role |
|------|------|------|
| `FFE5` | Service | WT901 primary service |
| `FFE4` | Characteristic (Notify) | Data output: orientation stream + register read responses |
| `FFE9` | Characteristic (Write) | Command input |

## Packet Types on FFE4

All packets start with `0x55` sync byte. Second byte identifies the type:

| Header | Type | Length | Description |
|--------|------|--------|-------------|
| `55 61` | Orientation | 20 bytes | Continuous stream: accel, gyro, euler angles (9x int16 LE) |
| `55 71` | Register read response | 20 bytes | Response to register read command |

**Important:** `0x55 0x5F` is the register read response for the UART/serial protocol only. Over BLE it is `0x55 0x71`.

## Register Read Response (`55 71`)

```
Byte  0:    0x55         sync
Byte  1:    0x71         register read response
Bytes 2-3:  RegAddrL/H   echoed register address (uint16 LE)
Bytes 4-19: 16 bytes     8 consecutive registers as uint16 LE
```

BLE returns 8 registers per read (vs 4 for serial).

## Writing Commands to FFE9

### Unlock (required before any register read/write)

```
FF AA 69 88 B5
```

Device auto-locks after 10 seconds of inactivity. Send unlock before each batch of commands.

### Register Read

```
FF AA 27 <REG_ADDR> 00
```

Response arrives asynchronously on FFE4 as a `55 71` notification.

### Register Write

```
FF AA <REG_ADDR> <VALUE_L> <VALUE_H>
```

After writing config registers, send save command (`FF AA 00 00 00`) to persist.

## Register Map

### Configuration Registers (Read/Write)

**Note:** On WT901BLE68 firmware, config registers (0x00-0x2A) do not respond to BLE
read commands (`FF AA 27`). They can be written but not read back over BLE. Sensor data
registers (0x2E+) respond normally. The exception is GYRORANGE (0x20) and ACCRANGE (0x21)
which do respond on some firmware versions.

| Address | Name | Description | Values |
|---------|------|-------------|--------|
| `0x00` | SAVE | Save/restore/reset | `0x0000`=save, `0x0001`=reset to defaults, `0x00FF`=reboot |
| `0x01` | CALSW | Calibration mode | See calibration section |
| `0x02` | RSW | Output content bitmask | Controls which data packets are broadcast |
| `0x03` | RRATE | Output rate | See output rate table |
| `0x04` | BAUD | UART baud rate | `0`=4800, `1`=9600, ..., `6`=115200, `7`=921600 |
| `0x1B` | LEDOFF | LED control | Bit 0: `1`=off |
| `0x1F` | BANDWIDTH | Low-pass filter bandwidth | `0`=256Hz, `1`=184, `2`=94, `3`=44, `4`=21, `5`=10, `6`=5 |
| `0x20` | GYRORANGE | Gyro full-scale range | `0`=250°/s, `1`=500, `2`=1000, `3`=2000 |
| `0x21` | ACCRANGE | Accel full-scale range | `0`=2g, `1`=4g, `2`=8g, `3`=16g |
| `0x22` | SLEEP | Sleep/wake | `0x01`=sleep, `0x00`=wake |
| `0x24` | AXIS6 | Algorithm mode | `0`=9-axis (with mag), `1`=6-axis (accel+gyro only) |
| `0x2A` | ACCFILT | Accel filter coefficient | `0`=raw, `1-15`=smoothing level |
| `0x69` | KEY | Unlock register | Write `0xB588` to unlock |

### Sensor Data Registers (Read-only)

| Address | Name | Description | Conversion |
|---------|------|-------------|------------|
| `0x2E` | VERSION | Firmware version | uint16 raw |
| `0x30` | YYMM | Manufacturing date | year/month packed |
| `0x34-0x36` | AX/AY/AZ | Acceleration | int16, raw / 32768 × full_scale_g |
| `0x37-0x39` | GX/GY/GZ | Angular velocity | int16, raw / 32768 × full_scale_dps |
| `0x3A-0x3C` | HX/HY/HZ | Magnetic field | int16 |
| `0x3D-0x3F` | Roll/Pitch/Yaw | Euler angles | int16, raw / 32768 × 180° |
| `0x40` | TEMP | Temperature | int16, raw / 100 = °C |
| `0x51-0x54` | q0/q1/q2/q3 | Quaternion | int16, raw / 32768 |
| `0x5C` | BATVAL | Supply voltage | uint16, raw / 100 = volts (centavolts) |
| `0x7F-0x84` | NUMBERID1-6 | Serial number | 6× uint16 LE = 12 bytes, hex-encoded |

### Output Rate (RRATE 0x03)

| Value | Frequency |
|-------|-----------|
| `0x01` | 0.2 Hz |
| `0x02` | 0.5 Hz |
| `0x03` | 1 Hz |
| `0x04` | 2 Hz |
| `0x05` | 5 Hz |
| `0x06` | 10 Hz (default) |
| `0x07` | 20 Hz |
| `0x08` | 50 Hz |
| `0x09` | 100 Hz |
| `0x0B` | 200 Hz |

### Battery Voltage (BATVAL 0x5C)

Unit is **centavolts** (not millivolts). A raw value of `370` = 3.70V.

The WT901BLECL uses a 3.7V nominal LiPo cell. Voltage-to-percentage mapping:

| Voltage | Approx % |
|---------|----------|
| 4.20V (420) | 100% |
| 4.00V (400) | 80% |
| 3.80V (380) | 60% |
| 3.70V (370) | 50% |
| 3.60V (360) | 30% |
| 3.40V (340) | 5% |
| 3.30V (330) | 0% (cutoff) |

**Note:** The battery ADC may not be ready immediately after BLE connection. If BATVAL reads as 0, wait ~500ms and retry. On some WT901BLE68 firmware versions, BATVAL always returns 0 over BLE.

### Serial Number (NUMBERID 0x7F-0x84)

12 bytes across 6 registers. Many WT901BLE68 production units have these registers unprogrammed (all zeros). The BLE MAC address is a more reliable unique identifier.

### Calibration (CALSW 0x01)

| Value | Mode |
|-------|------|
| `0x00` | Exit calibration (normal mode) |
| `0x01` | Accelerometer calibration — lay device flat, keep still |
| `0x04` | Yaw/heading reset to zero |
| `0x07` | Magnetic field calibration — rotate through all orientations |
| `0x08` | Set current angles as zero reference |

Sequence: unlock → write CALSW → (perform action) → write CALSW=0 → save.

## Command Sequence Example

```
1. Enable notifications on FFE4
2. Write to FFE9: FF AA 69 88 B5       (unlock)
3. Wait 100ms
4. Write to FFE9: FF AA 27 2E 00       (read firmware version)
5. Wait for 55 71 notification on FFE4
6. Parse response bytes 4-5 as uint16 LE
```

## Write Characteristic Notes

- tinygo/x/bluetooth on Linux only exposes `WriteWithoutResponse` (calls BlueZ `WriteValue` via D-Bus with no explicit type option; BlueZ auto-selects based on characteristic properties)
- No `Write` (write-with-response) method on Linux
- Writes return no error even when the device silently ignores them (e.g., when locked)

## Sources

- [WitMotion SDK - BLE 5.0 Protocol](https://wit-motion.gitbook.io/witmotion-sdk/ble-5.0-protocol/sdk/android_sdk-quick-start)
- [WitMotion SDK - Standard Communication Protocol](https://wit-motion.gitbook.io/witmotion-sdk/wit-standard-protocol/wit-standard-communication-protocol)
- [BLE 5.0 Communication Protocol PDF](https://cdn.robotshop.com/rbm/f83835f4-5e29-4ee0-9cc2-e49300031503/b/bf5c1f59-3b36-40a4-a5f0-e5a0eea52565/06165ec0_ble-5.0-communication-protocol.pdf)
- [Official Python SDK](https://github.com/WITMOTION/WitBluetooth_BWT901BLE5_0)
- [WT901BLECL BLE Data Read (Zenn)](https://zenn.dev/fastriver/articles/wt901blecl_read_data)
