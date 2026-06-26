package main

import (
	"flag"
	"fmt"

	"solar-collector/internal/probe"
)

func runProbe(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("probe: need an action (ports|read|classify|compare|poll-stress)")
	}
	action, rest := args[0], args[1:]
	switch action {
	case "ports":
		return probe.RunPorts()
	case "read":
		fs := flag.NewFlagSet("read", flag.ContinueOnError)
		port := fs.String("port", "", "serial device, e.g. /dev/solar-inv1")
		raw := fs.Bool("raw", false, "dump all input registers")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return probe.RunRead(*port, *raw)
	case "classify":
		fs := flag.NewFlagSet("classify", flag.ContinueOnError)
		p1 := fs.String("port1", "", "inverter 1 device")
		p2 := fs.String("port2", "", "inverter 2 device")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return probe.RunClassify(*p1, *p2)
	case "compare":
		fs := flag.NewFlagSet("compare", flag.ContinueOnError)
		port := fs.String("port", "", "our inverter device")
		broker := fs.String("sa-broker", "solar-assistant-host:1883", "SA broker host:port")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return probe.RunCompare(*port, *broker)
	case "poll-stress":
		fs := flag.NewFlagSet("poll-stress", flag.ContinueOnError)
		port := fs.String("port", "", "inverter device")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return probe.RunPollStress(*port)
	default:
		return fmt.Errorf("unknown probe action: %s", action)
	}
}
