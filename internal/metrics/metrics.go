package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"solar-collector/internal/aggregate"
	"solar-collector/internal/inverter"
)

// SlotView is a per-config-slot snapshot the daemon hands to the registry each
// poll cycle. Index is the position in cfg.Inverters; the exported label is
// Index+1 (1-based, SA convention).
type SlotView struct {
	Index   int
	State   string // "ok" | "off" | "fault" | ""
	Reading inverter.Reading
}

// Registry holds the last rendered exposition and serves it.
type Registry struct {
	mu      sync.Mutex
	text    []byte
	version string
}

// New returns a Registry that already serves build_info before the first Update.
func New(version string) *Registry {
	r := &Registry{version: version}
	r.text = render(aggregate.Totals{}, nil, version)
	return r
}

// Update re-renders the exposition from the latest cycle.
func (r *Registry) Update(t aggregate.Totals, slots []SlotView) {
	b := render(t, slots, r.version)
	r.mu.Lock()
	r.text = b
	r.mu.Unlock()
}

// Handler serves the latest exposition in Prometheus text format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		r.mu.Lock()
		b := r.text
		r.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write(b)
	})
}

func render(t aggregate.Totals, slots []SlotView, version string) []byte {
	var b strings.Builder

	gaugeHeader(&b, "solar_build_info", "Build info; constant 1.")
	b.WriteString(`solar_build_info{version="` + escape(version) + `"} 1` + "\n")

	gaugeHeader(&b, "solar_inverters_online", "Inverters read OK this cycle.")
	b.WriteString("solar_inverters_online " + strconv.Itoa(t.InvertersOnline) + "\n")

	if len(slots) > 0 {
		gaugeHeader(&b, "solar_inverter_up", "1 if the inverter was read OK this cycle, else 0.")
		for _, s := range slots {
			up := "0"
			if s.State == "ok" {
				up = "1"
			}
			b.WriteString(`solar_inverter_up{inverter="` + slotLabel(s) + `"} ` + up + "\n")
		}
	}

	if t.InvertersOnline > 0 {
		scalar(&b, "solar_battery_soc_percent", "Battery state of charge (percent).", strconv.Itoa(t.SoC))
		scalar(&b, "solar_battery_power_watts", "Battery power (SA sign: + charge / - discharge).", fnum(t.BatteryPowerW))
		if v, ok := firstOKVoltage(slots); ok {
			scalar(&b, "solar_battery_voltage_volts", "Battery voltage.", fnum(v))
		}
		scalar(&b, "solar_pv_power_watts", "Total PV power (watts).", fnum(t.PVPowerW))
		scalar(&b, "solar_load_power_watts", "Total load power (watts).", fnum(t.LoadW))
		scalar(&b, "solar_grid_power_watts", "Total grid power (+import / -export).", fnum(t.GridW))

		if anyOK(slots) {
			gaugeHeader(&b, "solar_inverter_pv_power_watts", "Per-inverter PV power (watts).")
			for _, s := range slots {
				if s.State == "ok" {
					b.WriteString(`solar_inverter_pv_power_watts{inverter="` + slotLabel(s) + `"} ` + fnum(s.Reading.PV1W+s.Reading.PV2W) + "\n")
				}
			}
			gaugeHeader(&b, "solar_inverter_temperature_celsius", "Per-inverter temperature (Celsius).")
			for _, s := range slots {
				if s.State == "ok" {
					b.WriteString(`solar_inverter_temperature_celsius{inverter="` + slotLabel(s) + `"} ` + fnum(s.Reading.TempC) + "\n")
				}
			}
			gaugeHeader(&b, "solar_inverter_status", "Per-inverter work-mode enum.")
			for _, s := range slots {
				if s.State == "ok" {
					b.WriteString(`solar_inverter_status{inverter="` + slotLabel(s) + `"} ` + strconv.Itoa(int(s.Reading.Status)) + "\n")
				}
			}
		}
	}

	return []byte(b.String())
}

func gaugeHeader(b *strings.Builder, name, help string) {
	b.WriteString("# HELP " + name + " " + help + "\n")
	b.WriteString("# TYPE " + name + " gauge\n")
}

func scalar(b *strings.Builder, name, help, val string) {
	gaugeHeader(b, name, help)
	b.WriteString(name + " " + val + "\n")
}

func slotLabel(s SlotView) string { return strconv.Itoa(s.Index + 1) }

func anyOK(slots []SlotView) bool {
	for _, s := range slots {
		if s.State == "ok" {
			return true
		}
	}
	return false
}

func firstOKVoltage(slots []SlotView) (float64, bool) {
	for _, s := range slots {
		if s.State == "ok" {
			return s.Reading.BatteryV, true
		}
	}
	return 0, false
}

func fnum(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
