package probe

import (
	"strings"
	"testing"

	"solar-collector/internal/inverter"
)

func TestWorkMode_Enum(t *testing.T) {
	cases := map[uint16]string{
		0:  "Standby",
		2:  "Battery Discharging",
		12: "PV Charging+Loads Supporting", // observed live on .244
		28: "Battery&PV Export to Grid+Loads Supporting",
		99: "Unknown", // > 28
	}
	for v, want := range cases {
		if got := workMode(v); got != want {
			t.Errorf("workMode(%d) = %q, want %q", v, got, want)
		}
	}
}

func TestFormatReading_ShowsWorkModeLabel(t *testing.T) {
	out := FormatReading(inverter.Reading{Status: 12})
	if !strings.Contains(out, "PV Charging+Loads Supporting") {
		t.Fatalf("expected work-mode label for status 12 in:\n%s", out)
	}
}

func TestFormatReading_ContainsKeyFields(t *testing.T) {
	r := inverter.Reading{SoC: 75, BatteryV: 53.6, BatteryPowerW: -300, PV1W: 150, PV2W: 200, LoadW: 1200, GridW: 0, TempC: 41.2, Status: 2}
	out := FormatReading(r)
	for _, want := range []string{"SoC", "75", "53.6", "-300", "150", "200", "1200", "41.2", "discharge", "charge"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
