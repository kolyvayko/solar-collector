package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprintln(os.Stderr, `solar-collector — Growatt SPF telemetry collector

Usage:
  solar-collector run   --config <path>
  solar-collector probe ports
  solar-collector probe read     --port <dev>
  solar-collector probe classify --port1 <dev> --port2 <dev>
  solar-collector probe compare  --port <dev> --sa-broker <host:port>
  solar-collector probe poll-stress --port <dev>`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		if err := runDaemon(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "probe":
		if err := runProbe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

// runProbe is defined in probe_cli.go (Task 6+). Temporary stub until then.
