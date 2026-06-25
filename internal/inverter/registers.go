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
