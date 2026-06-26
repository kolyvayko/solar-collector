package mqtt

import (
	"testing"

	"solar-collector/internal/aggregate"
)

type fakeClient struct {
	msgs map[string]string // topic -> payload (last write wins)
	ret  map[string]bool   // topic -> retained
}

func newFake() *fakeClient { return &fakeClient{msgs: map[string]string{}, ret: map[string]bool{}} }

func (f *fakeClient) publish(topic, payload string, retained bool) error {
	f.msgs[topic] = payload
	f.ret[topic] = retained
	return nil
}

func TestPublishTotals_TopicsAndFormat(t *testing.T) {
	f := newFake()
	p := New(f, "solar_assistant", "solar_collector/status")
	tot := aggregate.Totals{
		SoC: 77, BatteryPowerW: -117, PVPowerW: 91, LoadW: 281, GridW: 0,
		PerInverter: []aggregate.InverterSummary{{PVW: 91, TempC: 38.3, Status: 12}},
		InvertersOnline: 1,
	}
	if err := p.PublishTotals(tot); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"solar_assistant/total/battery_state_of_charge/state": "77",
		"solar_assistant/total/battery_power/state":           "-117",
		"solar_assistant/total/pv_power/state":                "91",
		"solar_assistant/total/load_power/state":              "281",
		"solar_assistant/total/grid_power/state":              "0",
		"solar_assistant/inverter_1/pv_power/state":           "91",
		"solar_assistant/inverter_1/temperature/state":        "38.3",
		"solar_assistant/inverter_1/status/state":             "12",
	}
	for topic, payload := range want {
		if f.msgs[topic] != payload {
			t.Errorf("%s = %q, want %q", topic, f.msgs[topic], payload)
		}
		if !f.ret[topic] {
			t.Errorf("%s not retained", topic)
		}
	}
}

func TestPublishTotals_PerInverterTopicFollowsSlotIndex(t *testing.T) {
	// Slot 0 (inv1) off this cycle; only slot 1 (inv2) present, carrying Index 1.
	// It must publish under inverter_2, NOT inverter_1 (fix I2: no positional shift).
	f := newFake()
	p := New(f, "solar_assistant", "solar_collector/status")
	tot := aggregate.Totals{
		SoC: 57, PVPowerW: 39, LoadW: 202, InvertersOnline: 1,
		PerInverter: []aggregate.InverterSummary{{Index: 1, PVW: 39, TempC: 38.9, Status: 12}},
	}
	if err := p.PublishTotals(tot); err != nil {
		t.Fatal(err)
	}
	if got := f.msgs["solar_assistant/inverter_2/pv_power/state"]; got != "39" {
		t.Errorf("inverter_2 pv_power = %q, want 39", got)
	}
	if got := f.msgs["solar_assistant/inverter_2/status/state"]; got != "12" {
		t.Errorf("inverter_2 status = %q, want 12", got)
	}
	if _, ok := f.msgs["solar_assistant/inverter_1/pv_power/state"]; ok {
		t.Error("must NOT publish inverter_1 when only slot 1 is present")
	}
}

func TestPublishTotals_FullTelemetryTopics(t *testing.T) {
	f := newFake()
	p := New(f, "solar_assistant", "solar_collector/status")
	p.PublishTotals(aggregate.Totals{
		GridV: 228.3, BusV: 422.2, GridHz: 50.01,
		BatteryVoltageV: 53.1, BatteryA: 12.5, LoadVA: 500,
		PerInverter: []aggregate.InverterSummary{
			{
				Index: 1, GridV: 228.3, AcOutV: 230.9, GridHz: 49.94,
				PV1V: 145.6, PV2V: 0, PV1A: 2.1, PV2A: 0,
				PV1W: 305, PV2W: 0, GridW: -120, LoadW: 450,
				LoadVA: 460, LoadPct: 22, BatteryV: 53.1, BatteryA: 5.2,
				BatteryFromAcW: 0, BusV: 422.2,
			},
		},
	})

	// total topics
	if got := f.msgs["solar_assistant/total/grid_voltage/state"]; got != "228.3" {
		t.Fatalf("total grid_voltage: %q", got)
	}
	if got := f.msgs["solar_assistant/total/grid_frequency/state"]; got != "50.01" {
		t.Fatalf("total grid_frequency: %q", got)
	}
	if got := f.msgs["solar_assistant/total/bus_voltage/state"]; got != "422.2" {
		t.Fatalf("total bus_voltage: %q", got)
	}
	if got := f.msgs["solar_assistant/total/battery_voltage/state"]; got != "53.1" {
		t.Fatalf("total battery_voltage: %q", got)
	}
	if got := f.msgs["solar_assistant/total/battery_current/state"]; got != "12.5" {
		t.Fatalf("total battery_current: %q", got)
	}
	if got := f.msgs["solar_assistant/total/load_apparent_power/state"]; got != "500" {
		t.Fatalf("total load_apparent_power: %q", got)
	}

	// per-inverter topics (slot Index=1 → inverter_2)
	if got := f.msgs["solar_assistant/inverter_2/ac_output_voltage/state"]; got != "230.9" {
		t.Fatalf("inv2 ac_output_voltage: %q", got)
	}
	if got := f.msgs["solar_assistant/inverter_2/grid_voltage/state"]; got != "228.3" {
		t.Fatalf("inv2 grid_voltage: %q", got)
	}
	if got := f.msgs["solar_assistant/inverter_2/load_power/state"]; got != "450" {
		t.Fatalf("inv2 load_power: %q", got)
	}
	if got := f.msgs["solar_assistant/inverter_2/pv_power_1/state"]; got != "305" {
		t.Fatalf("inv2 pv_power_1: %q", got)
	}
	if got := f.msgs["solar_assistant/inverter_2/grid_frequency/state"]; got != "49.94" {
		t.Fatalf("inv2 grid_frequency: %q", got)
	}

	// retained check
	if !f.ret["solar_assistant/inverter_2/grid_voltage/state"] {
		t.Fatal("grid_voltage must be retained")
	}
	if !f.ret["solar_assistant/total/grid_voltage/state"] {
		t.Fatal("total grid_voltage must be retained")
	}
}

func TestSetOnlineOffline(t *testing.T) {
	f := newFake()
	p := New(f, "solar_assistant", "solar_collector/status")
	if err := p.SetOnline(); err != nil {
		t.Fatal(err)
	}
	if f.msgs["solar_collector/status"] != "online" || !f.ret["solar_collector/status"] {
		t.Fatalf("online: %q ret=%v", f.msgs["solar_collector/status"], f.ret["solar_collector/status"])
	}
	if err := p.SetOffline(); err != nil {
		t.Fatal(err)
	}
	if f.msgs["solar_collector/status"] != "offline" {
		t.Fatalf("offline: %q", f.msgs["solar_collector/status"])
	}
}
