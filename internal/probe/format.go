package probe

import (
	"fmt"
	"strings"

	"solar-collector/internal/inverter"
)

// workModeLabels is the SPF off-grid system-status register (input reg 0)
// enum, verbatim from the official Growatt OffGrid Modbus RTU Protocol V0.26.
var workModeLabels = map[uint16]string{
	0:  "Standby",
	1:  "PV&Grid Supporting Loads",
	2:  "Battery Discharging",
	3:  "Fault",
	4:  "Flash",
	5:  "PV Charging",
	6:  "Grid Charging",
	7:  "PV&Grid Charging",
	8:  "PV&Grid Charging+Grid Bypass",
	9:  "PV Charging+Grid Bypass",
	10: "Grid Charging+Grid Bypass",
	11: "Grid Bypass",
	12: "PV Charging+Loads Supporting",
	13: "PV Discharging",
	14: "PV&Battery Discharging",
	15: "Gen Charging",
	16: "Gen Charging+Gen Bypass",
	17: "PV&Gen Charging",
	18: "PV&Gen Charging+Gen Bypass",
	19: "PV Charging+Gen Bypass",
	20: "Gen Bypass",
	21: "PV Export to Grid",
	22: "PV Export to Grid+Loads Supporting",
	23: "PV Charging+Export to Grid",
	24: "PV Charging+Export to Grid+Loads Supporting",
	25: "Battery Export to Grid",
	26: "Battery Export to Grid+Loads Supporting",
	27: "Battery&PV Export to Grid",
	28: "Battery&PV Export to Grid+Loads Supporting",
}

func workMode(s uint16) string {
	if label, ok := workModeLabels[s]; ok {
		return label
	}
	return "Unknown"
}

func FormatReading(r inverter.Reading) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SoC:           %d %%\n", r.SoC)
	fmt.Fprintf(&b, "Battery V:     %.2f V\n", r.BatteryV)
	fmt.Fprintf(&b, "Battery power: %.1f W (raw Growatt sign: + discharge / - charge)\n", r.BatteryPowerW)
	fmt.Fprintf(&b, "PV1 / PV2:     %.1f / %.1f W\n", r.PV1W, r.PV2W)
	fmt.Fprintf(&b, "Load:          %.1f W\n", r.LoadW)
	fmt.Fprintf(&b, "Grid:          %.1f W (+import / -export)\n", r.GridW)
	fmt.Fprintf(&b, "Temp:          %.1f C\n", r.TempC)
	fmt.Fprintf(&b, "Status:        %d (%s)\n", r.Status, workMode(r.Status))
	return b.String()
}
