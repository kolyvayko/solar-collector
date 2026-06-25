package probe

import (
	"testing"

	"solar-collector/internal/inverter"
)

func TestCompareSA_BatterySignMatchesAfterNegate(t *testing.T) {
	// Our raw: -500 (charge in raw convention). After negate => +500 (SA charge+).
	ours := inverter.Reading{SoC: 70, BatteryPowerW: -500, LoadW: 600}
	sa := SAValues{SoC: 70, BatteryPowerW: 500, LoadW: 600}
	rows := byName(CompareSA(ours, sa))
	if !rows["battery_power"].Match {
		t.Fatalf("battery should match after negate: %+v", rows["battery_power"])
	}
	if !rows["soc"].Match || !rows["load"].Match {
		t.Fatal("soc/load should match")
	}
}

func TestCompareSA_DetectsInvertedSign(t *testing.T) {
	// If we forgot to negate, our -500 vs SA +500 must NOT match and flag inversion.
	ours := inverter.Reading{BatteryPowerW: 500} // raw discharge+, SA says charge+500 -> mismatch
	sa := SAValues{BatteryPowerW: 500}
	row := byName(CompareSA(ours, sa))["battery_power"]
	if row.Match {
		t.Fatal("expected mismatch (sign inversion)")
	}
	if row.Note == "" {
		t.Fatal("expected a note explaining the sign check")
	}
}

func byName(rows []CompareRow) map[string]CompareRow {
	m := map[string]CompareRow{}
	for _, r := range rows {
		m[r.Name] = r
	}
	return m
}
