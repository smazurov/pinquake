# WIT Standard Communication Protocol

Source: https://wit-motion.gitbook.io/witmotion-sdk/wit-standard-protocol/wit-standard-communication-protocol

## Command Sequence

Operations must complete within 10 seconds or automatic lockout occurs. Follow this three-step process:

1. **Unlock**: `0xFF 0xAA 0x69 0x88 0xB5`
2. **Send command** to modify or read data
3. **Save**: `0xFF 0xAA 0x00 0x00 0x00`

## Write Format

```
Header1 | Header2 | Register | Data_Low | Data_High
0xFF    | 0xAA    | ADDR     | DATAL[7:0] | DATAH[15:8]
```

Data conversion: `DATA = (short)((short)DATAH << 8 | DATAL)`

All data transmitted in hexadecimal (not ASCII). Values use low byte first, high byte second.

## Data Output Packet Format

```
Header | TYPE | DATA1L | DATA1H | DATA2L | DATA2H | DATA3L | DATA3H | DATA4L | DATA4H | SUMCRC
0x55   | 0x5x |  [7:0] |[15:8]  |  [7:0] |[15:8]  |  [7:0] |[15:8]  |  [7:0] |[15:8]  | sum
```

### Checksum

```
SUMCRC = 0x55 + TYPE + DATA1L + DATA1H + DATA2L + DATA2H +
         DATA3L + DATA3H + DATA4L + DATA4H
(Take lower 8 bits only)
```

### TYPE Codes

| TYPE | Content |
|------|---------|
| 0x50 | Time |
| 0x51 | Acceleration |
| 0x52 | Angular velocity |
| 0x53 | Angle |
| 0x54 | Magnetic field |
| 0x55 | Port status |
| 0x56 | Barometric altitude |
| 0x57 | Latitude/Longitude |
| 0x58 | Ground speed |
| 0x59 | Quaternion |
| 0x5A | GPS positioning accuracy |
| 0x5F | Register read response |

## Sensor Data Output Formats

### Acceleration (0x51)
```
0x55 | 0x51 | AxL | AxH | AyL | AyH | AzL | AzH | TL | TH | SUM
```
- Accel: `((AxH << 8) | AxL) / 32768 * 16g`
- Temperature: `((TH << 8) | TL) / 100 C`

### Angular Velocity (0x52)
```
0x55 | 0x52 | WxL | WxH | WyL | WyH | WzL | WzH | VolL | VolH | SUM
```
- Angular velocity: `((WxH << 8) | WxL) / 32768 * 2000 deg/s`
- Voltage: `((VolH << 8) | VolL) / 100` (non-Bluetooth only)

### Angle (0x53)
```
0x55 | 0x53 | RollL | RollH | PitchL | PitchH | YawL | YawH | VL | VH | SUM
```
- Roll/Pitch/Yaw: `((RollH << 8) | RollL) / 32768 * 180 deg`

### Magnetic Field (0x54)
```
0x55 | 0x54 | HxL | HxH | HyL | HyH | HzL | HzH | TL | TH | SUM
```

### Quaternion (0x59)
```
0x55 | 0x59 | Q0L | Q0H | Q1L | Q1H | Q2L | Q2H | Q3L | Q3H | SUM
```
- q0-q3: `((QxH << 8) | QxL) / 32768`

### Time (0x50)
```
0x55 | 0x50 | YY | MM | DD | HH | MN | SS | MSL | MSH | SUM
```

### Port Status (0x55)
```
0x55 | 0x55 | D0L | D0H | D1L | D1H | D2L | D2H | D3L | D3H | SUM
```
- Analog input: `DxStatus / 1024 * Uvcc`

### Barometric Altitude (0x56)
```
0x55 | 0x56 | P0 | P1 | P2 | P3 | H0 | H1 | H2 | H3 | SUM
```
32-bit pressure and altitude values.

### GPS Coordinates (0x57)
```
0x55 | 0x57 | Lon0 | Lon1 | Lon2 | Lon3 | Lat0 | Lat1 | Lat2 | Lat3 | SUM
```
- Degrees: `Lon[31:0] / 10000000`
- Minutes: `(Lon[31:0] % 10000000) / 100000`

### GPS Speed (0x58)
```
0x55 | 0x58 | GPSHeightL | GPSHeightH | GPSYawL | GPSYawH | GPSV0-3 | SUM
```
- GPS altitude: `((GPSHeightH << 8) | GPSHeightL) / 10 m`
- GPS heading: `((GPSYawH << 8) | GPSYawL) / 100 deg`

### GPS Accuracy (0x5A)
```
0x55 | 0x5A | SNL | SNH | PDOPL | PDOPH | HDOPL | HDOPH | VDOPL | VDOPH | SUM
```
- Satellites: `((SNH << 8) | SNL)`
- xDOP: `((xDOPH << 8) | xDOPL) / 100`

### Register Read Response (0x5F)
```
0x55 | 0x5F | REG1L | REG1H | REG2L | REG2H | REG3L | REG3H | REG4L | REG4H | SUM
```
Returns 4 consecutive registers starting from requested address.

## Register Map

### Configuration Registers (Read/Write)

| Addr | Name | Description | Values |
|------|------|-------------|--------|
| 0x00 | SAVE | Save/reboot/reset | 0x0000=save, 0x00FF=reboot, 0x0001=factory reset |
| 0x01 | CALSW | Calibration mode | 0x00=normal, 0x01=accel, 0x03=height reset, 0x04=heading zero, 0x07=mag cal, 0x08=set angle ref, 0x09=mag cal dual-plane |
| 0x02 | RSW | Output content bitmask | bit0=TIME, bit1=ACC, bit2=GYRO, bit3=ANGLE, bit4=MAG, bit5=PORT, bit6=PRESS, bit7=GPS, bit8=VEL, bit9=QUAT, bit10=GSA |
| 0x03 | RRATE | Output rate | 0x01=0.2Hz, 0x02=0.5Hz, 0x03=1Hz, 0x04=2Hz, 0x05=5Hz, 0x06=10Hz, 0x07=20Hz, 0x08=50Hz, 0x09=100Hz, 0x0B=200Hz, 0x0C=single, 0x0D=none |
| 0x04 | BAUD | Serial baud rate | 0x01=4800, 0x02=9600, 0x03=19200, 0x04=38400, 0x05=57600, 0x06=115200, 0x07=230400 |
| 0x05-0x07 | AXOFFSET-AZOFFSET | Accel bias | value / 10000 g |
| 0x08-0x0A | GXOFFSET-GZOFFSET | Gyro bias | value / 10000 deg/s |
| 0x0B-0x0D | HXOFFSET-HZOFFSET | Mag bias | raw |
| 0x0E-0x11 | D0MODE-D3MODE | Pin config | 0x00=analog in, 0x01=digital in, 0x02=digital high, 0x03=digital low |
| 0x1A | IICADDR | I2C/Modbus address | 0x01-0x7F |
| 0x1B | LEDOFF | LED control | 0x00=on, 0x01=off |
| 0x1C-0x1E | MAGRANGX-Z | Mag cal range | default 0x01F4 (500) |
| 0x1F | BANDWIDTH | Filter bandwidth | 0x00=256Hz, 0x01=188Hz, 0x02=98Hz, 0x03=42Hz, 0x04=20Hz, 0x05=10Hz, 0x06=5Hz |
| 0x20 | GYRORANGE | Gyro range | 0x03=2000 deg/s (fixed default) |
| 0x21 | ACCRANGE | Accel range | 0x00=2g, 0x03=16g |
| 0x22 | SLEEP | Sleep mode | 0x00=active, 0x01=sleep |
| 0x23 | ORIENT | Installation dir | 0x00=horizontal, 0x01=vertical |
| 0x24 | AXIS6 | Algorithm | 0x00=9-axis (mag heading), 0x01=6-axis (integral heading) |
| 0x25 | FILTK | Dynamic filter K | default 0x001E (30) |
| 0x26 | GPSBAUD | GPS baud rate | same as BAUD |
| 0x27 | READADDR | Read register | write address to trigger read response |
| 0x2A | ACCFILT | Accel filter coeff | 16-bit |
| 0x2D | POWONSEND | Power-on auto-send | data type flags |
| 0x2E | VERSION | Firmware version (R) | uint16 |

### Sensor Data Registers (Read-only)

| Addr | Name | Description | Conversion |
|------|------|-------------|------------|
| 0x30-0x33 | YYMM-MS | Date/time | packed fields |
| 0x34-0x36 | AX/AY/AZ | Acceleration | int16, raw / 32768 * full_scale_g |
| 0x37-0x39 | GX/GY/GZ | Angular velocity | int16, raw / 32768 * full_scale_dps |
| 0x3A-0x3C | HX/HY/HZ | Magnetic field | int16 raw |
| 0x3D-0x3F | Roll/Pitch/Yaw | Euler angles | int16, raw / 32768 * 180 deg |
| 0x40 | TEMP | Temperature | int16, raw / 100 = C |
| 0x41-0x44 | D0-D3Status | Pin states | analog or digital |
| 0x45-0x48 | Pressure/Height | Barometric data | 32-bit values |
| 0x49-0x4C | Lon/Lat | GPS coordinates | 32-bit each |
| 0x4D-0x50 | GPSHeight/Yaw/V | GPS data | various |
| 0x51-0x54 | q0-q3 | Quaternion | int16, raw / 32768 |
| 0x55-0x58 | SVNUM/xDOP | GPS accuracy | various |
| 0x59-0x5F | Alarm settings | Alarm thresholds | various |
| 0x5C | BATVAL | Supply voltage | uint16 raw |
| 0x61-0x63 | Gyro cal settings | Calibration params | various |
| 0x68 | TRIGTIME | Alarm trigger time | uint16 |
| 0x69 | KEY | Unlock register | write 0xB588 |
| 0x6A | WERROR | Gyro error (R) | uint16 |
| 0x6B | TIMEZONE | GPS timezone | int8 |
| 0x6E-0x6F | WZTIME/WZSTATIC | Angular vel rest | uint16 each |
| 0x74 | MODDELAY | RS485 response delay | uint16 |
| 0x79-0x7A | XREFROLL/YREFPITCH | Angle ref (R) | int16 |
| 0x7F-0x84 | NUMBERID1-6 | Device serial (R) | 12 bytes |

## Command Examples

### Set baud to 115200
```
FF AA 69 88 B5    (unlock)
FF AA 04 06 00    (BAUD = 0x06)
FF AA 00 00 00    (save)
```

### Set output to 1Hz
```
FF AA 69 88 B5    (unlock)
FF AA 03 03 00    (RRATE = 0x03)
FF AA 00 00 00    (save)
```

### Reset heading to zero
```
FF AA 69 88 B5    (unlock)
FF AA 01 04 00    (CALSW = 0x04)
FF AA 00 00 00    (save)
```

### Read register 0x05 (returns 4 consecutive regs)
```
FF AA 69 88 B5    (unlock)
FF AA 27 05 00    (READADDR = 0x05)
```
Response: `55 5F [REG1L REG1H REG2L REG2H REG3L REG3H REG4L REG4H SUM]`
