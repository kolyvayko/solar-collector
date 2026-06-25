package probe

import (
	"testing"

	"solar-collector/internal/inverter"
)

func TestClassify_PVDiffersBatteryShared(t *testing.T) {
	a := inverter.Reading{SoC: 70, BatteryPowerW: -400, LoadW: 600, GridW: 0, PV1W: 100, PV2W: 0}
	b := inverter.Reading{SoC: 70, BatteryPowerW: -400, LoadW: 600, GridW: 0, PV1W: 300, PV2W: 50}
	fc := Classify(a, b)
	got := map[string]bool{}
	for _, f := range fc {
		got[f.Name] = f.Shared
	}
	if !got["battery_power"] || !got["soc"] || !got["load"] || !got["grid"] {
		t.Fatalf("expected battery/soc/load/grid SHARED: %+v", got)
	}
	if got["pv"] {
		t.Fatalf("expected pv PER-INVERTER: %+v", got)
	}
}
