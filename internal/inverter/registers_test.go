package inverter

import (
	"math"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestBytesToRegs(t *testing.T) {
	got := bytesToRegs([]byte{0x00, 0x12, 0xAB, 0xCD})
	if len(got) != 2 || got[0] != 0x0012 || got[1] != 0xABCD {
		t.Fatalf("got %v", got)
	}
}

func TestDecodeU16(t *testing.T) {
	regs := []uint16{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1234, 60}
	if v := decodeU16(regs, 18, 1); v != 60 {
		t.Fatalf("SoC: got %v", v)
	}
	if v := decodeU16(regs, 17, 0.01); !almost(v, 12.34) {
		t.Fatalf("battV: got %v", v)
	}
}

func TestDecodeS32_NegativeIsCharge(t *testing.T) {
	// reg 77/78 = -3000 (raw 0.1W) => -300.0 W (charge in raw convention)
	raw := int32(-3000)
	regs := make([]uint16, 90)
	regs[77] = uint16(uint32(raw) >> 16)
	regs[78] = uint16(uint32(raw) & 0xFFFF)
	if v := decodeS32(regs, 77, 78, 0.1); !almost(v, -300.0) {
		t.Fatalf("battPower: got %v", v)
	}
}

// setS32 is a test helper writing a signed int32 into an H/L register pair.
func setS32(regs []uint16, hi, lo int, v int32) {
	regs[hi] = uint16(uint32(v) >> 16)
	regs[lo] = uint16(uint32(v) & 0xFFFF)
}

func TestDecodeReading_FullBlock(t *testing.T) {
	regs := make([]uint16, blockQty)
	regs[0] = 2      // status: battery discharge
	regs[17] = 5360  // battery V ×0.01 = 53.60
	regs[18] = 75    // SoC %
	regs[25] = 412   // temp ×0.1 = 41.2
	// PV1 power 3/4 = 1500 (×0.1 = 150.0 W)
	setS32(regs, 3, 4, 1500)
	// PV2 power 5/6 = 2000 (×0.1 = 200.0 W)
	setS32(regs, 5, 6, 2000)
	// load 9/10 = 12000 (×0.1 = 1200.0 W)
	setS32(regs, 9, 10, 12000)
	// grid 36/37 = 0
	setS32(regs, 36, 37, 0)
	// battery power 77/78 = +8000 (×0.1 = 800.0 W, raw discharge positive)
	setS32(regs, 77, 78, 8000)

	r, err := DecodeReading(regs)
	if err != nil {
		t.Fatal(err)
	}
	if r.SoC != 75 || !almost(r.BatteryV, 53.60) || !almost(r.TempC, 41.2) {
		t.Fatalf("basic fields: %+v", r)
	}
	if !almost(r.PV1W, 150) || !almost(r.PV2W, 200) || !almost(r.LoadW, 1200) {
		t.Fatalf("power fields: %+v", r)
	}
	if !almost(r.BatteryPowerW, 800) { // raw: discharge positive
		t.Fatalf("battery raw sign wrong: %v", r.BatteryPowerW)
	}
	if r.Status != 2 {
		t.Fatalf("status: %v", r.Status)
	}
}

func TestDecodeReading_ShortBlock(t *testing.T) {
	if _, err := DecodeReading(make([]uint16, 10)); err == nil {
		t.Fatal("expected error on short block")
	}
}
