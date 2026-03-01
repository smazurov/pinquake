package orientation

import (
	"encoding/binary"
	"math"
)

const (
	rad2deg = 180.0 / math.Pi
	deg2rad = math.Pi / 180.0
)

type Orientation struct {
	Ax, Ay, Az    float32 // raw accel (g)
	Wx, Wy, Wz    float32 // gyro (deg/s)
	Roll, Pitch, Yaw float32 // Euler angles (degrees)
}

// DecodeV1 decodes a 20-byte V1 BLE packet.
// Header: [0x55, 0x61], then 9x int16 LE.
func DecodeV1(data []byte) (Orientation, bool) {
	if len(data) < 20 || data[0] != 0x55 || data[1] != 0x61 {
		return Orientation{}, false
	}
	i16At := func(off int) int16 {
		return int16(binary.LittleEndian.Uint16(data[off : off+2]))
	}
	scale := func(raw int16, fs float32) float32 {
		return float32(raw) / 32768.0 * fs
	}
	return Orientation{
		Ax:    scale(i16At(2), 16.0),
		Ay:    scale(i16At(4), 16.0),
		Az:    scale(i16At(6), 16.0),
		Wx:    scale(i16At(8), 2000.0),
		Wy:    scale(i16At(10), 2000.0),
		Wz:    scale(i16At(12), 2000.0),
		Roll:  scale(i16At(14), 180.0),
		Pitch: scale(i16At(16), 180.0),
		Yaw:   scale(i16At(18), 180.0),
	}, true
}

// GravityVector returns (gx, gy, gz) gravity components in sensor frame.
func GravityVector(pitchDeg, rollDeg float32) (float32, float32, float32) {
	theta := float64(pitchDeg) * deg2rad
	phi := float64(rollDeg) * deg2rad
	gx := float32(-math.Sin(theta))
	gy := float32(math.Cos(theta) * math.Sin(phi))
	gz := float32(math.Cos(theta) * math.Cos(phi))
	return gx, gy, gz
}

// CurrentInReferenceFrame rotates the raw accel of current into the reference
// frame defined by ref, using a Rz(yaw_delta) rotation on the raw accel vector.
func CurrentInReferenceFrame(current, ref *Orientation) Orientation {
	deltaYaw := float64(current.Yaw-ref.Yaw) * deg2rad
	cosY := float32(math.Cos(deltaYaw))
	sinY := float32(math.Sin(deltaYaw))

	ax := current.Ax*cosY - current.Ay*sinY
	ay := current.Ax*sinY + current.Ay*cosY
	az := current.Az

	out := *current
	out.Ax = ax - ref.Ax
	out.Ay = ay - ref.Ay
	out.Az = az - ref.Az
	return out
}
