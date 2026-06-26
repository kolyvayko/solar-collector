package aggregate

import (
	"solar-collector/internal/inverter"
)

type InverterSummary struct {
	Index  int // config slot index (0-based); per-inverter topic = Index+1
	PVW    float64
	TempC  float64
	Status uint16

	// Extended per-inverter telemetry (Task 3, SA parity)
	PV1V           float64 // V
	PV2V           float64 // V
	PV1A           float64 // A
	PV2A           float64 // A
	GridV          float64 // V  AC-input
	GridHz         float64 // Hz AC-input
	AcOutV         float64 // V  inverter output
	AcOutHz        float64 // Hz inverter output
	LoadVA         float64 // VA apparent power
	LoadPct        float64 // %
	BatteryV       float64 // V
	BatteryA       float64 // A  SA sign: +charge / −discharge
	BatteryFromAcW float64 // W  AC→battery charge
	BusV           float64 // V  DC bus
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

	// Extended totals (Task 3, SA parity)
	GridV           float64 // V  max across inverters (0 on faulting inv must not drag max)
	GridHz          float64 // Hz max across inverters
	BatteryVoltageV float64 // V  shared from first reading (single bank)
	BusV            float64 // V  shared from first reading
	BatteryA        float64 // A  sum across inverters, SA sign
	LoadVA          float64 // VA sum across inverters
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
		// Extended totals: sum VA + current; max grid voltage/freq
		t.LoadVA += r.LoadVA
		t.BatteryA += r.BatteryA
		if r.GridV > t.GridV {
			t.GridV = r.GridV
		}
		if r.GridHz > t.GridHz {
			t.GridHz = r.GridHz
		}
		t.PerInverter = append(t.PerInverter, InverterSummary{
			Index:          ir.Index,
			PVW:            pv,
			TempC:          r.TempC,
			Status:         r.Status,
			PV1V:           r.PV1V,
			PV2V:           r.PV2V,
			PV1A:           r.PV1A,
			PV2A:           r.PV2A,
			GridV:          r.GridV,
			GridHz:         r.GridHz,
			AcOutV:         r.AcOutV,
			AcOutHz:        r.AcOutHz,
			LoadVA:         r.LoadVA,
			LoadPct:        r.LoadPct,
			BatteryV:       r.BatteryV,
			BatteryA:       r.BatteryA,
			BatteryFromAcW: r.BatteryFromAcW,
			BusV:           r.BusV,
		})
	}
	t.BatteryPowerW = -rawBattery    // negate raw Growatt sign -> SA convention
	t.SoC = readings[0].Reading.SoC  // shared bank: take first
	t.BatteryVoltageV = readings[0].Reading.BatteryV // shared battery bank: take first
	t.BusV = readings[0].Reading.BusV                // shared DC bus: take first
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
