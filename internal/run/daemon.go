package run

import (
	"context"
	"log/slog"
	"time"

	"solar-collector/internal/aggregate"
	"solar-collector/internal/config"
	"solar-collector/internal/inverter"
	"solar-collector/internal/metrics"
)

type Reader interface {
	Read() (inverter.Reading, error)
}

type Publisher interface {
	PublishTotals(aggregate.Totals) error
	SetOnline() error
	SetOffline() error
}

type Opener func(dev string) (Reader, func(), error)
type PortPresent func(dev string) bool

type MetricsSink interface {
	Update(t aggregate.Totals, slots []metrics.SlotView)
}

type slot struct {
	dev    string
	reader Reader
	closer func()
	state  string           // "", "ok", "off", "fault"
	last   inverter.Reading // last OK reading (for metrics)
}

type Daemon struct {
	cfg     config.Config
	open    Opener
	present PortPresent
	pub     Publisher
	log     *slog.Logger
	slots   []*slot
	metrics MetricsSink
}

// SetMetrics attaches a metrics sink; nil (the default) disables metrics.
func (d *Daemon) SetMetrics(m MetricsSink) { d.metrics = m }

func New(cfg config.Config, open Opener, present PortPresent, pub Publisher, log *slog.Logger) *Daemon {
	d := &Daemon{cfg: cfg, open: open, present: present, pub: pub, log: log}
	for _, dev := range cfg.Inverters {
		d.slots = append(d.slots, &slot{dev: dev})
	}
	return d
}

// pollOnce reads every slot once, updates metrics, and publishes totals when complete.
func (d *Daemon) pollOnce() bool {
	var readings []aggregate.IndexedReading
	activeFault := false
	for i, s := range d.slots {
		r, ok := d.readSlot(s)
		if s.state == "fault" {
			activeFault = true
		}
		if ok {
			readings = append(readings, aggregate.IndexedReading{Index: i, Reading: r})
		}
	}
	totals := aggregate.Compute(readings)
	d.updateMetrics(totals)

	// Stale-guard: never publish a partial total while an expected inverter is faulting.
	if activeFault || len(readings) == 0 {
		return false
	}
	if err := d.pub.PublishTotals(totals); err != nil {
		d.log.Warn("publish failed", "err", err)
		return false
	}
	return true
}

func (d *Daemon) updateMetrics(t aggregate.Totals) {
	if d.metrics == nil {
		return
	}
	views := make([]metrics.SlotView, len(d.slots))
	for i, s := range d.slots {
		views[i] = metrics.SlotView{Index: i, State: s.state, Reading: s.last}
	}
	d.metrics.Update(t, views)
}

// readSlot classifies a slot this cycle and returns its reading if ok.
func (d *Daemon) readSlot(s *slot) (inverter.Reading, bool) {
	if !d.present(s.dev) {
		d.setState(s, "off")
		d.closeSlot(s)
		return inverter.Reading{}, false
	}
	if s.reader == nil {
		r, closer, err := d.open(s.dev)
		if err != nil {
			d.setState(s, "fault")
			return inverter.Reading{}, false
		}
		s.reader, s.closer = r, closer
	}
	reading, err := s.reader.Read()
	if err != nil {
		d.setState(s, "fault")
		d.closeSlot(s) // force reopen next cycle
		return inverter.Reading{}, false
	}
	d.setState(s, "ok")
	s.last = reading
	return reading, true
}

func (d *Daemon) closeSlot(s *slot) {
	if s.closer != nil {
		s.closer()
		s.closer = nil
	}
	s.reader = nil
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := d.pub.SetOnline(); err != nil {
		d.log.Warn("set online failed", "err", err)
	}
	defer func() {
		if err := d.pub.SetOffline(); err != nil {
			d.log.Warn("set offline failed", "err", err)
		}
		for _, s := range d.slots {
			d.closeSlot(s)
		}
	}()

	d.pollOnce() // immediate first poll
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.log.Info("shutting down")
			return nil
		case <-ticker.C:
			d.pollOnce()
		}
	}
}

// setState logs only on transitions, never every cycle.
func (d *Daemon) setState(s *slot, state string) {
	if s.state == state {
		return
	}
	s.state = state
	switch state {
	case "off":
		d.log.Info("inverter off (port absent)", "dev", s.dev)
	case "fault":
		d.log.Warn("inverter fault", "dev", s.dev)
	case "ok":
		d.log.Info("inverter ok", "dev", s.dev)
	}
}
