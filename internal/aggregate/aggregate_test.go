package aggregate

import (
	"math"
	"testing"

	"solar-collector/internal/inverter"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// indexed wraps readings with sequential slot indices (0..n-1).
func indexed(rs ...inverter.Reading) []IndexedReading {
	out := make([]IndexedReading, len(rs))
	for i, r := range rs {
		out[i] = IndexedReading{Index: i, Reading: r}
	}
	return out
}

func TestCompute_PreservesSlotIndex_WhenLowerSlotOff(t *testing.T) {
	// Slot 0 (inv1) is off this cycle; only slot 1 (inv2) read OK.
	// Its summary must carry Index 1 so it publishes under inverter_2, not inverter_1.
	rs := []IndexedReading{{Index: 1, Reading: inverter.Reading{SoC: 57, PV1W: 39, TempC: 38.9, Status: 12}}}
	tot := Compute(rs)
	if len(tot.PerInverter) != 1 {
		t.Fatalf("per-inverter count: %d", len(tot.PerInverter))
	}
	if tot.PerInverter[0].Index != 1 {
		t.Fatalf("slot index lost: got %d, want 1", tot.PerInverter[0].Index)
	}
}

func TestCompute_TwoInverters_SumAndSignNegate(t *testing.T) {
	// inv1 charging (raw -500), inv2 charging (raw -300) => raw sum -800
	// SA convention after negate => +800 (charge positive)
	rs := []inverter.Reading{
		{SoC: 70, PV1W: 100, PV2W: 50, LoadW: 600, GridW: 0, BatteryPowerW: -500, TempC: 40, Status: 5},
		{SoC: 71, PV1W: 200, PV2W: 0, LoadW: 600, GridW: 0, BatteryPowerW: -300, TempC: 42, Status: 5},
	}
	tot := Compute(indexed(rs...))
	if !almost(tot.PVPowerW, 350) {
		t.Fatalf("PV sum: %v", tot.PVPowerW)
	}
	if !almost(tot.LoadW, 1200) {
		t.Fatalf("load sum: %v", tot.LoadW)
	}
	if !almost(tot.BatteryPowerW, 800) { // negated to SA convention
		t.Fatalf("battery SA-convention: %v", tot.BatteryPowerW)
	}
	if tot.SoC != 70 { // shared: takes first
		t.Fatalf("SoC: %v", tot.SoC)
	}
	if tot.InvertersOnline != 2 || len(tot.PerInverter) != 2 {
		t.Fatalf("count: %d", tot.InvertersOnline)
	}
}

func TestCompute_SingleInverter_TotalValid(t *testing.T) {
	rs := []inverter.Reading{{SoC: 55, PV1W: 300, LoadW: 800, BatteryPowerW: 200}}
	tot := Compute(indexed(rs...))
	if tot.InvertersOnline != 1 || !almost(tot.PVPowerW, 300) || !almost(tot.LoadW, 800) {
		t.Fatalf("single: %+v", tot)
	}
	if !almost(tot.BatteryPowerW, -200) { // raw discharge + => SA discharge -
		t.Fatalf("battery: %v", tot.BatteryPowerW)
	}
}

func TestCompute_Empty(t *testing.T) {
	tot := Compute(nil)
	if tot.InvertersOnline != 0 {
		t.Fatalf("empty online: %d", tot.InvertersOnline)
	}
}

func TestCompute_SoCMismatchFlagged(t *testing.T) {
	rs := []inverter.Reading{{SoC: 60}, {SoC: 70}} // |Δ| > 2
	if !Compute(indexed(rs...)).SoCMismatch {
		t.Fatal("expected SoCMismatch")
	}
}

func TestCompute_ClampNegativePV(t *testing.T) {
	rs := []inverter.Reading{{SoC: 50, PV1W: -10}}
	tot := Compute(indexed(rs...))
	if tot.PVPowerW != 0 || !tot.Clamped {
		t.Fatalf("clamp: pv=%v clamped=%v", tot.PVPowerW, tot.Clamped)
	}
}
