package probe

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"solar-collector/internal/inverter"
)

func RunPorts() error {
	ports, err := ScanPorts("/sys", "/dev")
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		fmt.Println("no ttyXRUSB*/ttyACM* ports with VID:PID found")
		return nil
	}
	for _, p := range ports {
		fmt.Printf("%s  vid=%s pid=%s\n", p.Dev, p.Vid, p.Pid)
	}
	if NeedsExarDriver(ports) {
		fmt.Println("\nWARNING: ttyACM + 04e2 detected — load xr_usb_serial and blacklist cdc_acm for stable Modbus.")
	}
	fmt.Println("\nSuggested udev rules (fill ID_PATH from `udevadm info -q property -n <dev> | grep ID_PATH`):")
	fmt.Println(RenderUdevRule(ports[0], "solar-inv1"))
	if len(ports) > 1 {
		fmt.Println(RenderUdevRule(ports[1], "solar-inv2"))
	}
	return nil
}

func RunRead(dev string, raw bool) error {
	c, closer, err := inverter.Open(dev, time.Second)
	if err != nil {
		return err
	}
	defer closer()
	r, err := c.Read()
	if err != nil {
		return err
	}
	fmt.Print(FormatReading(r))
	if raw {
		fmt.Println("--- raw input registers ---")
		for i, v := range r.Raw {
			fmt.Printf("r%-3d = %5d  (0x%04x)\n", i, v, v)
		}
	}
	return nil
}

func RunClassify(d1, d2 string) error {
	c1, close1, err := inverter.Open(d1, time.Second)
	if err != nil {
		return err
	}
	defer close1()
	c2, close2, err := inverter.Open(d2, time.Second)
	if err != nil {
		return err
	}
	defer close2()
	r1, err := c1.Read()
	if err != nil {
		return fmt.Errorf("inv1: %w", err)
	}
	time.Sleep(850 * time.Millisecond)
	r2, err := c2.Read()
	if err != nil {
		return fmt.Errorf("inv2: %w", err)
	}
	fmt.Print(FormatClassify(Classify(r1, r2)))
	return nil
}

// fetchSAValues subscribes to SA retained topics and fills SAValues.
// It tracks arrival PER TOPIC (not a bare counter) to avoid a topic
// re-delivering twice and satisfying the gate before all 5 distinct
// topics have arrived. A mutex guards the SAValues writes because paho
// callbacks run on separate goroutines.
func fetchSAValues(broker string) (SAValues, error) {
	opts := mqtt.NewClientOptions().AddBroker("tcp://" + broker).SetClientID("solar-probe")
	cl := mqtt.NewClient(opts)
	if tok := cl.Connect(); tok.Wait() && tok.Error() != nil {
		return SAValues{}, tok.Error()
	}
	defer cl.Disconnect(100)

	var (
		mu      sync.Mutex
		sv      SAValues
		arrived = make(map[string]bool)
	)
	signal := make(chan struct{}, 5)

	type floatTopic struct {
		topic string
		dst   *float64
	}
	floatTopics := []floatTopic{
		{"solar_assistant/total/battery_power/state", &sv.BatteryPowerW},
		{"solar_assistant/total/pv_power/state", &sv.PVPowerW},
		{"solar_assistant/total/load_power/state", &sv.LoadW},
		{"solar_assistant/total/grid_power/state", &sv.GridW},
	}
	socTopic := "solar_assistant/total/battery_state_of_charge/state"

	for _, ft := range floatTopics {
		topic := ft.topic
		dst := ft.dst
		tok := cl.Subscribe(topic, 0, func(_ mqtt.Client, m mqtt.Message) {
			if f, err := strconv.ParseFloat(strings.TrimSpace(string(m.Payload())), 64); err == nil {
				mu.Lock()
				*dst = f
				arrived[topic] = true
				mu.Unlock()
				signal <- struct{}{}
			}
		})
		if tok.Wait() && tok.Error() != nil {
			return SAValues{}, fmt.Errorf("subscribe %s: %w", topic, tok.Error())
		}
	}
	tok := cl.Subscribe(socTopic, 0, func(_ mqtt.Client, m mqtt.Message) {
		if n, err := strconv.Atoi(strings.TrimSpace(string(m.Payload()))); err == nil {
			mu.Lock()
			sv.SoC = n
			arrived[socTopic] = true
			mu.Unlock()
			signal <- struct{}{}
		}
	})
	if tok.Wait() && tok.Error() != nil {
		return SAValues{}, fmt.Errorf("subscribe %s: %w", socTopic, tok.Error())
	}

	allTopics := make([]string, 0, len(floatTopics)+1)
	for _, ft := range floatTopics {
		allTopics = append(allTopics, ft.topic)
	}
	allTopics = append(allTopics, socTopic)

	timeout := time.After(5 * time.Second)
	for {
		mu.Lock()
		allArrived := len(arrived) >= len(allTopics)
		mu.Unlock()
		if allArrived {
			return sv, nil
		}
		select {
		case <-signal:
			// check arrived count at top of loop
		case <-timeout:
			mu.Lock()
			missing := make([]string, 0)
			for _, t := range allTopics {
				if !arrived[t] {
					missing = append(missing, t)
				}
			}
			partial := sv
			mu.Unlock()
			return partial, fmt.Errorf("timed out waiting for SA topics; missing: %s", strings.Join(missing, ", "))
		}
	}
}

func RunCompare(dev, broker string) error {
	c, closer, err := inverter.Open(dev, time.Second)
	if err != nil {
		return err
	}
	defer closer()
	ours, err := c.Read()
	if err != nil {
		return err
	}
	sa, err := fetchSAValues(broker)
	if err != nil {
		fmt.Println("WARNING:", err)
	}
	for _, row := range CompareSA(ours, sa) {
		flag := "OK"
		if !row.Match {
			flag = "MISMATCH"
		}
		fmt.Printf("%-14s ours=%.1f sa=%.1f [%s] %s\n", row.Name, row.Ours, row.SA, flag, row.Note)
	}
	return nil
}

func RunPollStress(dev string) error {
	c, closer, err := inverter.Open(dev, time.Second)
	if err != nil {
		return err
	}
	defer closer()
	for _, iv := range []time.Duration{10 * time.Second, 5 * time.Second, 2 * time.Second, 1 * time.Second} {
		fmt.Printf("interval %s: ", iv)
		ok := 0
		for i := 0; i < 5; i++ {
			if _, err := c.Read(); err != nil {
				fmt.Printf("FAIL (%v) ", err)
			} else {
				ok++
			}
			time.Sleep(iv)
		}
		fmt.Printf("-> %d/5 ok\n", ok)
	}
	return nil
}
