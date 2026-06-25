package probe

import (
	"fmt"
	"math"
	"strings"

	"solar-collector/internal/inverter"
)

type FieldClass struct {
	Name   string
	V1, V2 float64
	Shared bool
}

func Classify(a, b inverter.Reading) []FieldClass {
	const tol = 5.0 // W / generic tolerance; SoC handled below
	mk := func(name string, v1, v2, t float64) FieldClass {
		return FieldClass{Name: name, V1: v1, V2: v2, Shared: math.Abs(v1-v2) <= t}
	}
	return []FieldClass{
		mk("soc", float64(a.SoC), float64(b.SoC), 2),
		mk("battery_power", a.BatteryPowerW, b.BatteryPowerW, tol),
		mk("load", a.LoadW, b.LoadW, tol),
		mk("grid", a.GridW, b.GridW, tol),
		mk("pv", a.PV1W+a.PV2W, b.PV1W+b.PV2W, tol),
	}
}

func FormatClassify(fc []FieldClass) string {
	var b strings.Builder
	for _, f := range fc {
		kind := "PER-INVERTER"
		if f.Shared {
			kind = "SHARED"
		}
		fmt.Fprintf(&b, "%-14s inv1=%.1f inv2=%.1f -> %s\n", f.Name, f.V1, f.V2, kind)
	}
	return b.String()
}
