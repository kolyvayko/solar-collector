package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"solar-collector/internal/aggregate"
	"solar-collector/internal/inverter"
)

func mustContain(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("missing %q in:\n%s", want, body)
	}
}

func mustNotContain(t *testing.T, body, want string) {
	t.Helper()
	if strings.Contains(body, want) {
		t.Errorf("unexpected %q in:\n%s", want, body)
	}
}

func TestRender_FullData(t *testing.T) {
	tot := aggregate.Totals{SoC: 95, BatteryPowerW: -5, PVPowerW: 218, LoadW: 172, GridW: 0, InvertersOnline: 1}
	slots := []SlotView{
		{Index: 0, State: "ok", Reading: inverter.Reading{BatteryV: 53.46, PV1W: 0, PV2W: 218, TempC: 40.9, Status: 12}},
		{Index: 1, State: "fault"},
	}
	out := string(render(tot, slots, "test"))

	mustContain(t, out, `solar_build_info{version="test"} 1`)
	mustContain(t, out, "solar_inverters_online 1")
	mustContain(t, out, `solar_inverter_up{inverter="1"} 1`)
	mustContain(t, out, `solar_inverter_up{inverter="2"} 0`)
	mustContain(t, out, "solar_battery_soc_percent 95")
	mustContain(t, out, "solar_battery_power_watts -5")
	mustContain(t, out, "solar_battery_voltage_volts 53.46")
	mustContain(t, out, "solar_pv_power_watts 218")
	mustContain(t, out, "solar_load_power_watts 172")
	mustContain(t, out, `solar_inverter_pv_power_watts{inverter="1"} 218`)
	mustContain(t, out, `solar_inverter_temperature_celsius{inverter="1"} 40.9`)
	mustContain(t, out, `solar_inverter_status{inverter="1"} 12`)
	// fault slot has no per-inverter PV/temp/status series:
	mustNotContain(t, out, `solar_inverter_pv_power_watts{inverter="2"}`)
	// TYPE line appears exactly once per metric:
	if n := strings.Count(out, "# TYPE solar_inverter_up "); n != 1 {
		t.Errorf("expected 1 TYPE line for solar_inverter_up, got %d", n)
	}
}

func TestRender_NoData(t *testing.T) {
	out := string(render(aggregate.Totals{InvertersOnline: 0}, []SlotView{{Index: 0, State: "off"}}, "x"))
	mustContain(t, out, `solar_inverter_up{inverter="1"} 0`)
	mustContain(t, out, "solar_inverters_online 0")
	// no telemetry gauges when nothing is online:
	mustNotContain(t, out, "solar_battery_soc_percent")
	mustNotContain(t, out, "solar_pv_power_watts")
	mustNotContain(t, out, "solar_battery_voltage_volts")
}

func TestNew_ServesBuildInfoBeforeUpdate(t *testing.T) {
	r := New("v1")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type %q", ct)
	}
	mustContain(t, w.Body.String(), `solar_build_info{version="v1"} 1`)
	// No slots yet -> no dangling solar_inverter_up header without samples.
	mustNotContain(t, w.Body.String(), "# TYPE solar_inverter_up")
}

func TestHandler_ReflectsUpdate(t *testing.T) {
	r := New("v1")
	r.Update(
		aggregate.Totals{SoC: 80, PVPowerW: 100, InvertersOnline: 1},
		[]SlotView{{Index: 0, State: "ok", Reading: inverter.Reading{PV1W: 100}}},
	)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, req)
	mustContain(t, w.Body.String(), "solar_battery_soc_percent 80")
	mustContain(t, w.Body.String(), `solar_inverter_pv_power_watts{inverter="1"} 100`)
}

func TestRender_OnlineButNoOKSlot_NoDanglingHeaders(t *testing.T) {
	// InvertersOnline > 0 but no slot is "ok" (defensive: caller passes inconsistent state).
	out := string(render(aggregate.Totals{InvertersOnline: 1, SoC: 50}, []SlotView{{Index: 0, State: "fault"}}, "x"))
	mustNotContain(t, out, "# TYPE solar_inverter_pv_power_watts")
	mustNotContain(t, out, "# TYPE solar_inverter_temperature_celsius")
	mustNotContain(t, out, "# TYPE solar_inverter_status")
	// scalar totals still present (gated only on InvertersOnline>0):
	mustContain(t, out, "solar_battery_soc_percent 50")
}

func TestRender_TelemetryGauges(t *testing.T) {
	tot := aggregate.Totals{InvertersOnline: 1, SoC: 50, GridV: 228.3, GridHz: 49.94, BusV: 422.2}
	slots := []SlotView{{Index: 0, State: "ok", Reading: inverter.Reading{AcOutV: 230.9, LoadPct: 20}}}
	out := string(render(tot, slots, "test"))
	for _, want := range []string{
		"solar_grid_voltage_volts 228.3",
		"solar_grid_frequency_hertz 49.94",
		"solar_bus_voltage_volts 422.2",
		`solar_inverter_ac_output_voltage_volts{inverter="1"} 230.9`,
		`solar_inverter_load_percentage{inverter="1"} 20`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing gauge %q in:\n%s", want, out)
		}
	}
}
