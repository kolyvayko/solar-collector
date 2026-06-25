package probe

import (
	"math"

	"solar-collector/internal/inverter"
)

type SAValues struct {
	SoC           int
	BatteryPowerW float64 // SA convention: charge +, discharge -
	PVPowerW      float64
	LoadW         float64
	GridW         float64
}

type CompareRow struct {
	Name  string
	Ours  float64
	SA    float64
	Match bool
	Note  string
}

func CompareSA(ours inverter.Reading, sa SAValues) []CompareRow {
	const tol = 10.0
	ourBattery := -ours.BatteryPowerW // negate raw -> SA convention, same as aggregate
	row := func(name string, o, s, t float64, note string) CompareRow {
		return CompareRow{Name: name, Ours: o, SA: s, Match: math.Abs(o-s) <= t, Note: note}
	}
	return []CompareRow{
		row("soc", float64(ours.SoC), float64(sa.SoC), 2, ""),
		row("battery_power", ourBattery, sa.BatteryPowerW, tol,
			"compares -raw(77/78) vs SA; mismatch here = sign/scale wrong"),
		row("load", ours.LoadW, sa.LoadW, tol, "single inverter vs SA total may differ if SA reports both"),
		row("grid", ours.GridW, sa.GridW, tol, ""),
	}
}
