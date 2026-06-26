package mqtt

import (
	"fmt"
	"math"
	"strconv"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"solar-collector/internal/aggregate"
	"solar-collector/internal/config"
)

type publishClient interface {
	publish(topic, payload string, retained bool) error
}

type Publisher struct {
	c           publishClient
	prefix      string
	statusTopic string
}

func New(c publishClient, prefix, statusTopic string) *Publisher {
	return &Publisher{c: c, prefix: prefix, statusTopic: statusTopic}
}

// watt formats a power/percent value as SA does: a plain rounded integer.
func watt(v float64) string { return strconv.Itoa(int(math.Round(v))) }

// volt formats a voltage to 1 decimal place.
func volt(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// freq formats a frequency to 2 decimal places.
func freq(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// amp formats a current to 1 decimal place.
func amp(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

func (p *Publisher) PublishTotals(t aggregate.Totals) error {
	totals := map[string]string{
		"battery_state_of_charge": strconv.Itoa(t.SoC),
		"battery_power":           watt(t.BatteryPowerW),
		"pv_power":                watt(t.PVPowerW),
		"load_power":              watt(t.LoadW),
		"grid_power":              watt(t.GridW),
		"grid_voltage":            volt(t.GridV),
		"grid_frequency":          freq(t.GridHz),
		"bus_voltage":             volt(t.BusV),
		"battery_voltage":         volt(t.BatteryVoltageV),
		"battery_current":         amp(t.BatteryA),
		"load_apparent_power":     watt(t.LoadVA),
	}
	for k, v := range totals {
		if err := p.c.publish(fmt.Sprintf("%s/total/%s/state", p.prefix, k), v, true); err != nil {
			return err
		}
	}
	for _, inv := range t.PerInverter {
		n := inv.Index + 1 // 1-based slot index (fix I2: survives off lower slots)
		base := fmt.Sprintf("%s/inverter_%d", p.prefix, n)
		fields := map[string]string{
			"pv_power":              watt(inv.PVW),
			"temperature":           strconv.FormatFloat(inv.TempC, 'f', 1, 64),
			"status":                strconv.Itoa(int(inv.Status)),
			"pv_power_1":            watt(inv.PV1W),
			"pv_power_2":            watt(inv.PV2W),
			"pv_voltage_1":          volt(inv.PV1V),
			"pv_voltage_2":          volt(inv.PV2V),
			"pv_current_1":          amp(inv.PV1A),
			"pv_current_2":          amp(inv.PV2A),
			"grid_voltage":          volt(inv.GridV),
			"grid_frequency":        freq(inv.GridHz),
			"grid_power":            watt(inv.GridW),
			"ac_output_voltage":     volt(inv.AcOutV),
			"ac_output_frequency":   freq(inv.AcOutHz),
			"load_power":            watt(inv.LoadW),
			"load_apparent_power":   watt(inv.LoadVA),
			"load_percentage":       watt(inv.LoadPct),
			"battery_voltage":       volt(inv.BatteryV),
			"battery_current":       amp(inv.BatteryA),
			"battery_power_from_ac": watt(inv.BatteryFromAcW),
			"bus_voltage":           volt(inv.BusV),
		}
		for k, v := range fields {
			if err := p.c.publish(base+"/"+k+"/state", v, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Publisher) SetOnline() error  { return p.c.publish(p.statusTopic, "online", true) }
func (p *Publisher) SetOffline() error { return p.c.publish(p.statusTopic, "offline", true) }

// pahoClient wraps a paho client to satisfy publishClient.
type pahoClient struct{ cl paho.Client }

func (p pahoClient) publish(topic, payload string, retained bool) error {
	tok := p.cl.Publish(topic, 0, retained, payload)
	if !tok.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("publish %s: timeout", topic)
	}
	return tok.Error()
}

// Connect builds a paho client with AutoReconnect and an offline LWT on the
// status topic, connects, and returns a Publisher plus a closer.
func Connect(cfg config.Config) (*Publisher, func(), error) {
	opts := paho.NewClientOptions().
		AddBroker("tcp://"+cfg.Broker).
		SetClientID("solar-collector").
		SetAutoReconnect(true).
		SetWill(cfg.StatusTopic, "offline", 0, true)
	cl := paho.NewClient(opts)
	tok := cl.Connect()
	if !tok.WaitTimeout(10 * time.Second) {
		return nil, nil, fmt.Errorf("connect %s: timeout", cfg.Broker)
	}
	if err := tok.Error(); err != nil {
		return nil, nil, fmt.Errorf("connect %s: %w", cfg.Broker, err)
	}
	return New(pahoClient{cl}, cfg.TopicPrefix, cfg.StatusTopic), func() { cl.Disconnect(250) }, nil
}
