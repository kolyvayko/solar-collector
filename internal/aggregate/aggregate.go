package aggregate

import (
	"solar-collector/internal/inverter"
)

type InverterSummary struct {
	Index  int // config slot index (0-based); per-inverter topic = Index+1
	PVW    float64
	TempC  float64
	Status uint16
}

// IndexedReading pairs an OK reading with its config slot index so per-inverter
// identity survives when a lower-numbered slot is off/faulting (fix I2).
type IndexedReading struct {
	Index   int
	Reading inverter.Reading
}

type Totals struct {
	SoC             int
	BatteryPowerW   float64 // SA convention: charge +, discharge -
	PVPowerW        float64
	LoadW           float64
	GridW           float64
	PerInverter     []InverterSummary
	InvertersOnline int
	SoCMismatch     bool
	Clamped         bool
}

func Compute(readings []IndexedReading) Totals {
	t := Totals{InvertersOnline: len(readings)}
	if len(readings) == 0 {
		return t
	}
	var rawBattery float64
	minSoC, maxSoC := readings[0].Reading.SoC, readings[0].Reading.SoC
	for _, ir := range readings {
		r := ir.Reading
		pv := r.PV1W + r.PV2W
		t.PVPowerW += pv
		t.LoadW += r.LoadW
		t.GridW += r.GridW
		rawBattery += r.BatteryPowerW
		if r.SoC < minSoC {
			minSoC = r.SoC
		}
		if r.SoC > maxSoC {
			maxSoC = r.SoC
		}
		t.PerInverter = append(t.PerInverter, InverterSummary{Index: ir.Index, PVW: pv, TempC: r.TempC, Status: r.Status})
	}
	t.BatteryPowerW = -rawBattery   // negate raw Growatt sign -> SA convention
	t.SoC = readings[0].Reading.SoC // shared bank: take first
	if maxSoC-minSoC > 2 {
		t.SoCMismatch = true
	}
	if t.SoC < 0 {
		t.SoC, t.Clamped = 0, true
	}
	if t.SoC > 100 {
		t.SoC, t.Clamped = 100, true
	}
	if t.PVPowerW < 0 {
		t.PVPowerW, t.Clamped = 0, true
	}
	return t
}
