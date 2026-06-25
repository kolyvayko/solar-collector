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
