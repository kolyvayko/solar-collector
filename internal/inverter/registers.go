package inverter

import "fmt"

func bytesToRegs(b []byte) []uint16 {
	regs := make([]uint16, len(b)/2)
	for i := range regs {
		regs[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return regs
}

func decodeU16(regs []uint16, addr int, scale float64) float64 {
	return float64(regs[addr]) * scale
}

func decodeS32(regs []uint16, hi, lo int, scale float64) float64 {
	raw := int32(uint32(regs[hi])<<16 | uint32(regs[lo]))
	return float64(raw) * scale
}

const blockQty = 90

// Verified-live SPF6000ES input-register addresses — raw dump of both inverters
// cross-checked with physics + the live Solar Assistant capture on 2026-06-26.
// Scales are applied at decode. See
// docs/superpowers/plans/2026-06-26-sa-telemetry-parity.md (Task 1).
// battery_current (SA sign +charge/−discharge) is computed as
// (regBatChargeA − regBatDischargeA) × 0.1, since the inverter exposes the two
// directions in separate registers (exactly one nonzero at a time).
const (
	regPv1V          = 1  // ×0.1 V
	regPv2V          = 2  // ×0.1 V
	regPv1A          = 7  // ×0.1 A
	regPv2A          = 8  // ×0.1 A
	regLoadVAHi      = 11 // ×0.1 VA (32-bit with regLoadVALo)
	regLoadVALo      = 12
	regBatFromAcHi   = 13 // ×0.1 W AC→battery charge (32-bit with regBatFromAcLo; 0 when no AC charging)
	regBatFromAcLo   = 14
	regBusV          = 19 // ×0.1 V
	regGridV         = 20 // ×0.1 V  AC-input (~0 ⇒ blackout)
	regGridHz        = 21 // ×0.01 Hz AC-input
	regAcOutV        = 22 // ×0.1 V  inverter output (NOT grid)
	regAcOutHz       = 23 // ×0.01 Hz inverter output
	regLoadPct       = 27 // ×0.1 %
	regBatChargeA    = 83 // ×0.1 A  (nonzero while charging)
	regBatDischargeA = 84 // ×0.1 A  (nonzero while discharging)
)

type Reading struct {
	SoC           int     // %
	BatteryV      float64 // V
	BatteryPowerW float64 // RAW Growatt sign: discharge +, charge -
	PV1W          float64
	PV2W          float64
	LoadW         float64
	GridW         float64 // >0 import, <0 export
	TempC         float64
	Status        uint16 // work-mode enum; 0 = standby/off
	Raw           []uint16
}

func DecodeReading(regs []uint16) (Reading, error) {
	if len(regs) < blockQty {
		return Reading{}, fmt.Errorf("register block too short: got %d, need %d", len(regs), blockQty)
	}
	return Reading{
		SoC:           int(decodeU16(regs, 18, 1)),
		BatteryV:      decodeU16(regs, 17, 0.01),
		BatteryPowerW: decodeS32(regs, 77, 78, 0.1),
		PV1W:          decodeS32(regs, 3, 4, 0.1),
		PV2W:          decodeS32(regs, 5, 6, 0.1),
		LoadW:         decodeS32(regs, 9, 10, 0.1),
		GridW:         decodeS32(regs, 36, 37, 0.1),
		TempC:         decodeU16(regs, 25, 0.1),
		Status:        regs[0],
		Raw:           append([]uint16(nil), regs[:blockQty]...),
	}, nil
}
