package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"solar-collector/internal/config"
	"solar-collector/internal/inverter"
	"solar-collector/internal/metrics"
	"solar-collector/internal/mqtt"
	"solar-collector/internal/run"
)

var version = "dev"

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	cfgPath := fs.String("config", "/etc/solar-collector/config.yaml", "path to config.yaml")
	metricsAddr := fs.String("metrics-addr", ":9099", "address for the Prometheus /metrics endpoint (empty disables)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	pub, closePub, err := mqtt.Connect(cfg)
	if err != nil {
		return err
	}
	defer closePub()

	opener := func(dev string) (run.Reader, func(), error) {
		return inverter.Open(dev, cfg.ReadTimeout)
	}
	present := func(dev string) bool {
		_, err := os.Stat(dev)
		return err == nil
	}

	d := run.New(cfg, opener, present, pub, log)

	if *metricsAddr != "" {
		reg := metrics.New(version)
		d.SetMetrics(reg)
		mux := http.NewServeMux()
		mux.Handle("/metrics", reg.Handler())
		srv := &http.Server{Addr: *metricsAddr, Handler: mux}
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("metrics server failed", "err", err)
			}
		}()
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(sctx)
		}()
		log.Info("metrics endpoint listening", "addr", *metricsAddr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("solar-collector starting", "broker", cfg.Broker, "inverters", cfg.Inverters, "poll", cfg.PollInterval.String())
	return d.Run(ctx)
}
