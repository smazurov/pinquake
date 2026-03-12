// Package orientation decodes WT901BLECL IMU sensor data and transforms
// acceleration readings into a gravity-aligned reference frame for pinball
// nudge detection.
//
// # Sensor Coordinate System (from datasheet)
//
//	     X (up, toward strap)
//	     ^
//	     |
//	     |
//	Y <--+---> Z (out of face, toward you)
//
//	At rest flat face-up: Ax≈0, Ay≈0, Az≈+1g
//
// # Pinball Machine Frame
//
//	        +Y (away from player / toward backbox)
//	         ^
//	         |
//	  -X <---+---> +X (right, from player's POV)
//	         |
//	         v
//	        -Y (toward player)
//
//	  +Z = up (gravity opposes -Z)
//
// # Mounting Scenarios
//
// The sensor is typically mounted vertically (strap pointing up) on a side or
// front panel, so gravity falls along sensor X (Ax ≈ +1g). The two horizontal
// nudge axes are then sensor Y and Z.
//
//   - Side panel mount: sensor face outward, Y → machine Y, Z → machine X
//   - Front panel mount: sensor face toward player, Y → machine X, Z → machine Y
//   - Flat on top (face up): Az ≈ +1g, nudge axes are sensor X and Y
//
// Gravity alone cannot distinguish side-mount from front-mount (both have
// Ax ≈ +1g). The swap_xy config option resolves this ambiguity.
//
// # Orientation Detection
//
// At lock time, BuildLockRotation constructs a rotation matrix from the
// measured gravity vector that transforms sensor-frame readings into a
// canonical frame where Z aligns with gravity. This works for any mounting
// angle, including tilted orientations.
//
//	| Dominant Axis | Meaning           | Horizontal Axes |
//	|---------------|-------------------|-----------------|
//	| Ax (±1g)      | Sensor vertical   | Y, Z            |
//	| Ay (±1g)      | Sensor on side    | X, Z            |
//	| Az (±1g)      | Sensor flat       | X, Y            |
package orientation

import (
	"encoding/binary"
	"math"
)

const deg2rad = math.Pi / 180.0

type Orientation struct {
	Ax, Ay, Az       float32 // raw accel (g)
	Wx, Wy, Wz       float32 // gyro (deg/s)
	Roll, Pitch, Yaw float32 // Euler angles (degrees)
}

// Mat3 is a 3x3 rotation matrix stored in row-major order.
type Mat3 [3][3]float32

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

// BuildLockRotation constructs a rotation matrix from the reference gravity
// vector that transforms sensor-frame coordinates into a canonical frame
// where Z aligns with gravity. Uses Gram-Schmidt orthogonalization to build
// an orthonormal basis from the gravity direction.
func BuildLockRotation(ref *Orientation) Mat3 {
	gx, gy, gz := float64(ref.Ax), float64(ref.Ay), float64(ref.Az)
	norm := math.Sqrt(gx*gx + gy*gy + gz*gz)
	if norm < 1e-6 {
		return Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	}
	// z_canon = gravity unit vector
	zx, zy, zz := gx/norm, gy/norm, gz/norm

	// Cyclic reference vector: pick the axis after the dominant gravity
	// axis (X→Y, Y→Z, Z→X). This produces sign-flip-free mappings for
	// axis-aligned gravity and keeps swap_xy consistent across orientations.
	ax, ay, az := math.Abs(zx), math.Abs(zy), math.Abs(zz)
	rx, ry, rz := 1.0, 0.0, 0.0
	if ax >= ay && ax >= az {
		rx, ry, rz = 0, 1, 0
	} else if ay >= az {
		rx, ry, rz = 0, 0, 1
	}
	dot := rx*zx + ry*zy + rz*zz
	px, py, pz := rx-dot*zx, ry-dot*zy, rz-dot*zz
	pnorm := math.Sqrt(px*px + py*py + pz*pz)
	// x_canon = normalized projection
	xx, xy, xz := px/pnorm, py/pnorm, pz/pnorm

	// y_canon = cross(z_canon, x_canon)
	yx := zy*xz - zz*xy
	yy := zz*xx - zx*xz
	yz := zx*xy - zy*xx

	return Mat3{
		{float32(xx), float32(xy), float32(xz)},
		{float32(yx), float32(yy), float32(yz)},
		{float32(zx), float32(zy), float32(zz)},
	}
}

// ApplyMat3 multiplies matrix m by vector (x, y, z).
func ApplyMat3(m Mat3, x, y, z float32) (float32, float32, float32) {
	return m[0][0]*x + m[0][1]*y + m[0][2]*z,
		m[1][0]*x + m[1][1]*y + m[1][2]*z,
		m[2][0]*x + m[2][1]*y + m[2][2]*z
}

// InReferenceFrame subtracts the current gravity estimate from the raw
// accelerometer reading and rotates the result into the canonical frame.
//
// output = rot * (cur_accel - gravity)
func InReferenceFrame(current *Orientation, gravity [3]float32, rot Mat3) Orientation {
	out := *current
	out.Ax, out.Ay, out.Az = ApplyMat3(rot,
		current.Ax-gravity[0], current.Ay-gravity[1], current.Az-gravity[2])
	return out
}
