package orientation

import (
	"encoding/binary"
	"math"
	"testing"
)

func encodeTestV1(ax, ay, az, wx, wy, wz, roll, pitch, yaw float32) []byte {
	buf := make([]byte, 20)
	buf[0] = 0x55
	buf[1] = 0x61
	f2i := func(v, fs float32) int16 {
		r := v / fs * 32768.0
		if r > 32767 {
			r = 32767
		} else if r < -32768 {
			r = -32768
		}
		return int16(r)
	}
	put := func(off int, v int16) {
		binary.LittleEndian.PutUint16(buf[off:off+2], uint16(v))
	}
	put(2, f2i(ax, 16.0))
	put(4, f2i(ay, 16.0))
	put(6, f2i(az, 16.0))
	put(8, f2i(wx, 2000.0))
	put(10, f2i(wy, 2000.0))
	put(12, f2i(wz, 2000.0))
	put(14, f2i(roll, 180.0))
	put(16, f2i(pitch, 180.0))
	put(18, f2i(yaw, 180.0))
	return buf
}

func TestDecodeV1(t *testing.T) {
	buf := encodeTestV1(1.0, -0.5, 9.8, 100.0, -200.0, 50.0, 3.5, -1.2, 45.0)
	o, ok := DecodeV1(buf)
	if !ok {
		t.Fatal("DecodeV1 returned false")
	}
	accelRes := float32(16.0 / 32768.0)
	gyroRes := float32(2000.0 / 32768.0)
	angleRes := float32(180.0 / 32768.0)

	check := func(name string, got, want, res float32) {
		if d := got - want; d < -res*1.5 || d > res*1.5 {
			t.Errorf("%s: got %f, want %f (res=%f)", name, got, want, res)
		}
	}
	check("ax", o.Ax, 1.0, accelRes)
	check("ay", o.Ay, -0.5, accelRes)
	check("az", o.Az, 9.8, accelRes)
	check("wx", o.Wx, 100.0, gyroRes)
	check("wy", o.Wy, -200.0, gyroRes)
	check("wz", o.Wz, 50.0, gyroRes)
	check("roll", o.Roll, 3.5, angleRes)
	check("pitch", o.Pitch, -1.2, angleRes)
	check("yaw", o.Yaw, 45.0, angleRes)
}

func TestDecodeV1Rejects(t *testing.T) {
	if _, ok := DecodeV1([]byte{0x55, 0x61}); ok {
		t.Error("should reject short data")
	}
	buf := encodeTestV1(0, 0, 0, 0, 0, 0, 0, 0, 0)
	buf[0] = 0x00
	if _, ok := DecodeV1(buf); ok {
		t.Error("should reject bad header")
	}
}

const tol = 0.001

func near(a, b float64) bool {
	return math.Abs(a-b) < tol
}

func assertOrthonormal(t *testing.T, m Mat3) {
	t.Helper()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			dot := float64(0)
			for k := 0; k < 3; k++ {
				dot += float64(m[i][k]) * float64(m[j][k])
			}
			expected := 0.0
			if i == j {
				expected = 1.0
			}
			if !near(dot, expected) {
				t.Errorf("R*R^T[%d][%d] = %f, want %f", i, j, dot, expected)
			}
		}
	}
}

func TestBuildLockRotation_SingleAxis(t *testing.T) {
	tests := []struct {
		name         string
		ax, ay, az   float32
	}{
		{"gravity +X", 1, 0, 0},
		{"gravity -X", -1, 0, 0},
		{"gravity +Y", 0, 1, 0},
		{"gravity -Y", 0, -1, 0},
		{"gravity +Z", 0, 0, 1},
		{"gravity -Z", 0, 0, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := &Orientation{Ax: tt.ax, Ay: tt.ay, Az: tt.az}
			m := BuildLockRotation(ref)
			assertOrthonormal(t, m)

			rx, ry, rz := ApplyMat3(m, tt.ax, tt.ay, tt.az)
			if !near(float64(rx), 0) || !near(float64(ry), 0) {
				t.Errorf("gravity in canonical: got (%f, %f, %f), want (0, 0, ±1)", rx, ry, rz)
			}
			if !near(math.Abs(float64(rz)), 1.0) {
				t.Errorf("gravity Z: got %f, want ±1", rz)
			}
		})
	}
}

func TestBuildLockRotation_MultiAxis(t *testing.T) {
	s2 := float32(1 / math.Sqrt(2))
	s3 := float32(1 / math.Sqrt(3))

	tests := []struct {
		name       string
		ax, ay, az float32
	}{
		{"45° XZ tilt", s2, 0, s2},
		{"45° XY tilt", s2, s2, 0},
		{"30/60 XZ", float32(math.Sqrt(3) / 2), 0, 0.5},
		{"3-axis equal", s3, s3, s3},
		{"3-axis negative", -s3, -s3, -s3},
		{"45° YZ tilt", 0, s2, s2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := &Orientation{Ax: tt.ax, Ay: tt.ay, Az: tt.az}
			m := BuildLockRotation(ref)
			assertOrthonormal(t, m)

			rx, ry, rz := ApplyMat3(m, tt.ax, tt.ay, tt.az)
			if !near(float64(rx), 0) || !near(float64(ry), 0) {
				t.Errorf("gravity in canonical: got (%f, %f, %f), want (0, 0, |g|)", rx, ry, rz)
			}
			gNorm := math.Sqrt(float64(tt.ax*tt.ax + tt.ay*tt.ay + tt.az*tt.az))
			if !near(float64(rz), gNorm) {
				t.Errorf("gravity Z: got %f, want %f", rz, gNorm)
			}
		})
	}
}

func TestBuildLockRotation_HorizontalNudgeStaysHorizontal(t *testing.T) {
	ref := &Orientation{Ax: 1, Ay: 0, Az: 0}
	m := BuildLockRotation(ref)

	nx, ny, nz := ApplyMat3(m, 0, 0.5, 0)
	if !near(float64(nz), 0) {
		t.Errorf("Y-nudge leaked into Z: got %f", nz)
	}
	horiz := math.Sqrt(float64(nx*nx + ny*ny))
	if !near(horiz, 0.5) {
		t.Errorf("Y-nudge horizontal magnitude: got %f, want 0.5", horiz)
	}

	nx, ny, nz = ApplyMat3(m, 0, 0, 0.5)
	if !near(float64(nz), 0) {
		t.Errorf("Z-nudge leaked into Z: got %f", nz)
	}
	horiz = math.Sqrt(float64(nx*nx + ny*ny))
	if !near(horiz, 0.5) {
		t.Errorf("Z-nudge horizontal magnitude: got %f, want 0.5", horiz)
	}
}

func TestBuildLockRotation_GravityNudgeGoesToZ(t *testing.T) {
	ref := &Orientation{Ax: 1, Ay: 0, Az: 0}
	m := BuildLockRotation(ref)

	nx, ny, nz := ApplyMat3(m, 0.3, 0, 0)
	if !near(float64(nx), 0) || !near(float64(ny), 0) {
		t.Errorf("gravity-axis nudge leaked into X/Y: got (%f, %f)", nx, ny)
	}
	if !near(float64(nz), 0.3) {
		t.Errorf("gravity-axis nudge Z: got %f, want 0.3", nz)
	}
}

func TestBuildLockRotation_ZeroGravity(t *testing.T) {
	ref := &Orientation{Ax: 0, Ay: 0, Az: 0}
	m := BuildLockRotation(ref)
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			expected := float32(0)
			if i == j {
				expected = 1
			}
			if m[i][j] != expected {
				t.Errorf("zero gravity: m[%d][%d] = %f, want %f", i, j, m[i][j], expected)
			}
		}
	}
}

func TestBuildLockRotation_AxisMapping(t *testing.T) {
	tests := []struct {
		name       string
		ax, ay, az float32
		// Unit nudge along each non-gravity sensor axis and expected canonical output.
		nudge1     [3]float32
		wantGx1    float64
		wantGy1    float64
		nudge2     [3]float32
		wantGx2    float64
		wantGy2    float64
	}{
		{
			name: "gravity +X: sensorY→gx, sensorZ→gy",
			ax: 1, ay: 0, az: 0,
			nudge1: [3]float32{0, 1, 0}, wantGx1: 1, wantGy1: 0,
			nudge2: [3]float32{0, 0, 1}, wantGx2: 0, wantGy2: 1,
		},
		{
			name: "gravity +Z: sensorX→gx, sensorY→gy",
			ax: 0, ay: 0, az: 1,
			nudge1: [3]float32{1, 0, 0}, wantGx1: 1, wantGy1: 0,
			nudge2: [3]float32{0, 1, 0}, wantGx2: 0, wantGy2: 1,
		},
		{
			name: "gravity +Y: sensorZ→gx, sensorX→gy",
			ax: 0, ay: 1, az: 0,
			nudge1: [3]float32{0, 0, 1}, wantGx1: 1, wantGy1: 0,
			nudge2: [3]float32{1, 0, 0}, wantGx2: 0, wantGy2: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := &Orientation{Ax: tt.ax, Ay: tt.ay, Az: tt.az}
			m := BuildLockRotation(ref)

			gx, gy, gz := ApplyMat3(m, tt.nudge1[0], tt.nudge1[1], tt.nudge1[2])
			if !near(float64(gx), tt.wantGx1) || !near(float64(gy), tt.wantGy1) {
				t.Errorf("nudge1: got gx=%f gy=%f, want gx=%f gy=%f", gx, gy, tt.wantGx1, tt.wantGy1)
			}
			if !near(float64(gz), 0) {
				t.Errorf("nudge1: gz=%f, want 0", gz)
			}

			gx, gy, gz = ApplyMat3(m, tt.nudge2[0], tt.nudge2[1], tt.nudge2[2])
			if !near(float64(gx), tt.wantGx2) || !near(float64(gy), tt.wantGy2) {
				t.Errorf("nudge2: got gx=%f gy=%f, want gx=%f gy=%f", gx, gy, tt.wantGx2, tt.wantGy2)
			}
			if !near(float64(gz), 0) {
				t.Errorf("nudge2: gz=%f, want 0", gz)
			}
		})
	}
}

func TestInReferenceFrame_SameAsGravity(t *testing.T) {
	gravity := [3]float32{1, 0, 0}
	ref := &Orientation{Ax: 1, Ay: 0, Az: 0}
	rot := BuildLockRotation(ref)

	cur := &Orientation{Ax: 1, Ay: 0, Az: 0}
	out := InReferenceFrame(cur, gravity, rot)
	if !near(float64(out.Ax), 0) || !near(float64(out.Ay), 0) || !near(float64(out.Az), 0) {
		t.Errorf("same as gravity: got (%f, %f, %f), want (0, 0, 0)", out.Ax, out.Ay, out.Az)
	}
}

func TestInReferenceFrame_Flat(t *testing.T) {
	gravity := [3]float32{0, 0, 1}
	ref := &Orientation{Ax: 0, Ay: 0, Az: 1}
	rot := BuildLockRotation(ref)

	// Nudge in sensor X → should appear in horizontal plane
	cur := &Orientation{Ax: 0.5, Ay: 0, Az: 1}
	out := InReferenceFrame(cur, gravity, rot)
	if !near(float64(out.Az), 0) {
		t.Errorf("flat nudge should not affect Z: got %f", out.Az)
	}
	horiz := math.Sqrt(float64(out.Ax*out.Ax + out.Ay*out.Ay))
	if !near(horiz, 0.5) {
		t.Errorf("flat nudge magnitude: got %f, want 0.5", horiz)
	}
}

func TestInReferenceFrame_VerticalMount(t *testing.T) {
	gravity := [3]float32{1, 0, 0}
	ref := &Orientation{Ax: 1, Ay: 0, Az: 0}
	rot := BuildLockRotation(ref)

	// Nudge in sensor Y → horizontal output
	cur := &Orientation{Ax: 1, Ay: 0.3, Az: 0}
	out := InReferenceFrame(cur, gravity, rot)
	if !near(float64(out.Az), 0) {
		t.Errorf("Y nudge leaked to Z: got %f", out.Az)
	}
	horiz := math.Sqrt(float64(out.Ax*out.Ax + out.Ay*out.Ay))
	if !near(horiz, 0.3) {
		t.Errorf("Y nudge magnitude: got %f, want 0.3", horiz)
	}

	// Nudge in sensor Z → horizontal output
	cur = &Orientation{Ax: 1, Ay: 0, Az: 0.3}
	out = InReferenceFrame(cur, gravity, rot)
	if !near(float64(out.Az), 0) {
		t.Errorf("Z nudge leaked to Z: got %f", out.Az)
	}
	horiz = math.Sqrt(float64(out.Ax*out.Ax + out.Ay*out.Ay))
	if !near(horiz, 0.3) {
		t.Errorf("Z nudge magnitude: got %f, want 0.3", horiz)
	}
}

func TestInReferenceFrame_GravityUpdated(t *testing.T) {
	// Lock at vertical mount: gravity on +X
	// Gravity has drifted 10° toward +Z from vertical mount
	shiftAngle := 10.0 * deg2rad
	shiftAx := float32(math.Cos(shiftAngle))
	shiftAz := float32(math.Sin(shiftAngle))
	gravity := [3]float32{shiftAx, 0, shiftAz}

	// Build rotation from updated gravity
	rot := BuildLockRotation(&Orientation{Ax: shiftAx, Ay: 0, Az: shiftAz})

	// Reading matches shifted gravity → output should be zero
	cur := &Orientation{Ax: shiftAx, Ay: 0, Az: shiftAz}
	out := InReferenceFrame(cur, gravity, rot)
	if !near(float64(out.Ax), 0) || !near(float64(out.Ay), 0) || !near(float64(out.Az), 0) {
		t.Errorf("after gravity update: got (%f, %f, %f), want (0, 0, 0)", out.Ax, out.Ay, out.Az)
	}
}

func TestInReferenceFrame_NudgeAfterGravityUpdate(t *testing.T) {
	// Gravity has shifted 10° from +X toward +Z
	shiftAngle := 10.0 * deg2rad
	shiftAx := float32(math.Cos(shiftAngle))
	shiftAz := float32(math.Sin(shiftAngle))
	gravity := [3]float32{shiftAx, 0, shiftAz}
	rot := BuildLockRotation(&Orientation{Ax: shiftAx, Ay: 0, Az: shiftAz})

	// Apply a 0.3g nudge in sensor Y on top of shifted gravity
	cur := &Orientation{Ax: shiftAx, Ay: 0.3, Az: shiftAz}
	out := InReferenceFrame(cur, gravity, rot)

	horiz := math.Sqrt(float64(out.Ax*out.Ax + out.Ay*out.Ay))
	if !near(horiz, 0.3) {
		t.Errorf("nudge after gravity update: horizontal magnitude got %f, want 0.3", horiz)
	}
	if !near(float64(out.Az), 0) {
		t.Errorf("nudge should not leak to Z: got %f", out.Az)
	}
}

func TestInReferenceFrame_UpsideDown(t *testing.T) {
	gravity := [3]float32{0, 0, -1}
	ref := &Orientation{Ax: 0, Ay: 0, Az: -1}
	rot := BuildLockRotation(ref)

	cur := &Orientation{Ax: 0, Ay: 0, Az: -1}
	out := InReferenceFrame(cur, gravity, rot)
	if !near(float64(out.Ax), 0) || !near(float64(out.Ay), 0) || !near(float64(out.Az), 0) {
		t.Errorf("same as ref: got (%f, %f, %f)", out.Ax, out.Ay, out.Az)
	}

	// Nudge in sensor X → horizontal
	cur = &Orientation{Ax: 0.4, Ay: 0, Az: -1}
	out = InReferenceFrame(cur, gravity, rot)
	if !near(float64(out.Az), 0) {
		t.Errorf("nudge leaked to Z: got %f", out.Az)
	}
}
