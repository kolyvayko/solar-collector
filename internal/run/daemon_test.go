package run

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"solar-collector/internal/aggregate"
	"solar-collector/internal/config"
	"solar-collector/internal/inverter"
	"solar-collector/internal/metrics"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fakeReader struct {
	r   inverter.Reading
	err error
}

func (f *fakeReader) Read() (inverter.Reading, error) { return f.r, f.err }

type fakePub struct {
	mu      sync.Mutex
	totals  []aggregate.Totals
	online  int
	offline int
}

func (p *fakePub) PublishTotals(t aggregate.Totals) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totals = append(p.totals, t)
	return nil
}
func (p *fakePub) SetOnline() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.online++
	return nil
}
func (p *fakePub) SetOffline() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offline++
	return nil
}

// counts returns the recorded call counts under the lock; used by tests that
// read fakePub while Run is still on another goroutine.
func (p *fakePub) counts() (online, offline, publishes int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.online, p.offline, len(p.totals)
}

func cfg2(devs ...string) config.Config {
	return config.Config{Broker: "x:1883", TopicPrefix: "solar_assistant", StatusTopic: "s", Inverters: devs}
}

func TestPollOnce_TwoInverters_PublishesSum(t *testing.T) {
	open := func(dev string) (Reader, func(), error) {
		switch dev {
		case "a":
			return &fakeReader{r: inverter.Reading{SoC: 70, PV1W: 100, LoadW: 600, BatteryPowerW: -200}}, func() {}, nil
		default:
			return &fakeReader{r: inverter.Reading{SoC: 70, PV1W: 200, LoadW: 600, BatteryPowerW: -100}}, func() {}, nil
		}
	}
	present := func(string) bool { return true }
	pub := &fakePub{}
	d := New(cfg2("a", "b"), open, present, pub, quietLog())
	if !d.pollOnce() {
		t.Fatal("expected publish")
	}
	if len(pub.totals) != 1 {
		t.Fatalf("publishes: %d", len(pub.totals))
	}
	got := pub.totals[0]
	if got.PVPowerW != 300 || got.LoadW != 1200 || got.BatteryPowerW != 300 { // -(-200-100)=300
		t.Fatalf("aggregate wrong: %+v", got)
	}
	if got.InvertersOnline != 2 {
		t.Fatalf("online: %d", got.InvertersOnline)
	}
}

func TestPollOnce_OffInverter_TotalStillValid(t *testing.T) {
	// "a" present and ok; "b" port absent (off) -> total = just "a", still published.
	open := func(dev string) (Reader, func(), error) {
		return &fakeReader{r: inverter.Reading{SoC: 55, PV1W: 300, LoadW: 800, BatteryPowerW: 50}}, func() {}, nil
	}
	present := func(dev string) bool { return dev == "a" }
	pub := &fakePub{}
	d := New(cfg2("a", "b"), open, present, pub, quietLog())
	if !d.pollOnce() {
		t.Fatal("expected publish with one inverter off")
	}
	if pub.totals[0].InvertersOnline != 1 || pub.totals[0].PVPowerW != 300 {
		t.Fatalf("off-mode total wrong: %+v", pub.totals[0])
	}
}

func TestPollOnce_LowerSlotOff_KeepsSlotIndex(t *testing.T) {
	// Slot 0 ("a") off (port absent); slot 1 ("b") ok. The published summary
	// must carry Index 1 so it lands under inverter_2, not inverter_1 (fix I2).
	open := func(dev string) (Reader, func(), error) {
		return &fakeReader{r: inverter.Reading{SoC: 57, PV1W: 39, Status: 12}}, func() {}, nil
	}
	present := func(dev string) bool { return dev == "b" }
	pub := &fakePub{}
	d := New(cfg2("a", "b"), open, present, pub, quietLog())
	if !d.pollOnce() {
		t.Fatal("expected publish with lower slot off")
	}
	pi := pub.totals[0].PerInverter
	if len(pi) != 1 || pi[0].Index != 1 {
		t.Fatalf("per-inverter index not preserved: %+v", pi)
	}
}

func TestPollOnce_Fault_SuppressesPublish(t *testing.T) {
	// "a" ok, "b" present but read errors (fault) -> stale-guard: no publish.
	open := func(dev string) (Reader, func(), error) {
		if dev == "b" {
			return &fakeReader{err: errors.New("timeout")}, func() {}, nil
		}
		return &fakeReader{r: inverter.Reading{SoC: 60, PV1W: 100}}, func() {}, nil
	}
	present := func(string) bool { return true }
	pub := &fakePub{}
	d := New(cfg2("a", "b"), open, present, pub, quietLog())
	if d.pollOnce() {
		t.Fatal("expected NO publish while an inverter is faulting")
	}
	if len(pub.totals) != 0 {
		t.Fatalf("must not publish stale partial: %+v", pub.totals)
	}
}

func TestReadSlot_FaultClosesForReopen(t *testing.T) {
	closed := 0
	open := func(dev string) (Reader, func(), error) {
		return &fakeReader{err: errors.New("timeout")}, func() { closed++ }, nil
	}
	d := New(cfg2("a"), open, func(string) bool { return true }, &fakePub{}, quietLog())
	d.pollOnce() // opens, read fails, closes
	if closed != 1 {
		t.Fatalf("expected closer called on fault, got %d", closed)
	}
	if d.slots[0].reader != nil {
		t.Fatal("reader must be nil after fault so next cycle reopens")
	}
}

func TestPollOnce_AllOff_NoPublish(t *testing.T) {
	d := New(cfg2("a", "b"), func(string) (Reader, func(), error) { return &fakeReader{}, func() {}, nil },
		func(string) bool { return false }, &fakePub{}, quietLog())
	if d.pollOnce() {
		t.Fatal("nothing online -> no publish")
	}
}

type fakeSink struct {
	lastTotals aggregate.Totals
	lastSlots  []metrics.SlotView
	calls      int
}

func (f *fakeSink) Update(t aggregate.Totals, slots []metrics.SlotView) {
	f.calls++
	f.lastTotals = t
	f.lastSlots = slots
}

func TestPollOnce_UpdatesMetrics_OkAndFault(t *testing.T) {
	open := func(dev string) (Reader, func(), error) {
		if dev == "b" {
			return &fakeReader{err: errors.New("timeout")}, func() {}, nil
		}
		return &fakeReader{r: inverter.Reading{SoC: 60, PV1W: 100, BatteryV: 53.2, TempC: 41, Status: 12}}, func() {}, nil
	}
	sink := &fakeSink{}
	d := New(cfg2("a", "b"), open, func(string) bool { return true }, &fakePub{}, quietLog())
	d.SetMetrics(sink)

	// "b" faults -> publish suppressed, but metrics MUST still update.
	if d.pollOnce() {
		t.Fatal("expected no publish while an inverter faults")
	}
	if sink.calls != 1 {
		t.Fatalf("metrics Update calls: %d", sink.calls)
	}
	if len(sink.lastSlots) != 2 {
		t.Fatalf("slot views: %d", len(sink.lastSlots))
	}
	if sink.lastSlots[0].State != "ok" || sink.lastSlots[0].Index != 0 {
		t.Fatalf("slot0: %+v", sink.lastSlots[0])
	}
	if sink.lastSlots[1].State != "fault" || sink.lastSlots[1].Index != 1 {
		t.Fatalf("slot1: %+v", sink.lastSlots[1])
	}
	if sink.lastSlots[0].Reading.BatteryV != 53.2 {
		t.Fatalf("slot0 reading not captured: %+v", sink.lastSlots[0].Reading)
	}
	if sink.lastTotals.InvertersOnline != 1 {
		t.Fatalf("online: %d", sink.lastTotals.InvertersOnline)
	}
}

func TestPollOnce_NoMetricsSink_NoPanic(t *testing.T) {
	d := New(cfg2("a"), func(string) (Reader, func(), error) {
		return &fakeReader{r: inverter.Reading{SoC: 50}}, func() {}, nil
	}, func(string) bool { return true }, &fakePub{}, quietLog())
	// no SetMetrics -> must not panic
	if !d.pollOnce() {
		t.Fatal("expected publish")
	}
}

func TestRun_SetsOnlineThenOfflineOnCancel(t *testing.T) {
	open := func(dev string) (Reader, func(), error) {
		return &fakeReader{r: inverter.Reading{SoC: 50, PV1W: 10}}, func() {}, nil
	}
	pub := &fakePub{}
	c := cfg2("a")
	c.PollInterval = 20 * time.Millisecond
	d := New(c, open, func(string) bool { return true }, pub, quietLog())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	time.Sleep(50 * time.Millisecond) // allow first poll(s)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
	online, offline, publishes := pub.counts()
	if online != 1 {
		t.Fatalf("SetOnline calls: %d", online)
	}
	if offline != 1 {
		t.Fatalf("SetOffline calls: %d", offline)
	}
	if publishes == 0 {
		t.Fatal("expected at least one publish during Run")
	}
}
