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

func TestGravityVector(t *testing.T) {
	gx, gy, gz := GravityVector(0, 0)
	if math.Abs(float64(gx)) > 0.001 || math.Abs(float64(gy)) > 0.001 || math.Abs(float64(gz)-1.0) > 0.001 {
		t.Errorf("level: got (%f, %f, %f), want (0, 0, 1)", gx, gy, gz)
	}
}

func TestCurrentInReferenceFrame(t *testing.T) {
	ref := &Orientation{Yaw: 0}
	cur := &Orientation{Ax: 1.0, Ay: 0, Az: 0, Yaw: 90}
	out := CurrentInReferenceFrame(cur, ref)
	if math.Abs(float64(out.Ax)) > 0.001 || math.Abs(float64(out.Ay)-1.0) > 0.001 {
		t.Errorf("90° rotation: got ax=%f ay=%f, want (0, 1)", out.Ax, out.Ay)
	}
}

func TestCurrentInReferenceFrame_SubtractsGravity(t *testing.T) {
	ref := &Orientation{Ax: 0.1, Ay: 0.0, Az: 0.995}
	cur := &Orientation{Ax: 0.1, Ay: 0.0, Az: 0.995}
	out := CurrentInReferenceFrame(cur, ref)
	if math.Abs(float64(out.Ax)) > 0.001 || math.Abs(float64(out.Ay)) > 0.001 {
		t.Errorf("same angle: got ax=%f ay=%f, want ~0", out.Ax, out.Ay)
	}
	if math.Abs(float64(out.Az)) > 0.001 {
		t.Errorf("same angle: got az=%f, want ~0", out.Az)
	}
}
