package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Broker       string
	TopicPrefix  string
	StatusTopic  string
	PollInterval time.Duration
	// CommandSpacing is the minimum gap between consecutive Modbus commands on a
	// single bus. Reserved for future shared-bus / multi-command polling: today
	// each inverter is its own USB serial bus and the daemon issues one command
	// per poll cycle, so consecutive commands are already separated by
	// PollInterval and this value has no enforcement point yet. Kept so the
	// config surface and probe's 850ms spacing stay aligned.
	CommandSpacing time.Duration
	ReadTimeout    time.Duration
	Inverters      []string
}

// rawConfig mirrors the YAML; durations are strings so we parse + validate them.
type rawConfig struct {
	Broker         string   `yaml:"broker"`
	TopicPrefix    string   `yaml:"topic_prefix"`
	StatusTopic    string   `yaml:"status_topic"`
	PollInterval   string   `yaml:"poll_interval"`
	CommandSpacing string   `yaml:"command_spacing"`
	ReadTimeout    string   `yaml:"read_timeout"`
	Inverters      []string `yaml:"inverters"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var r rawConfig
	if err := yaml.Unmarshal(b, &r); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	c := Config{
		Broker:      r.Broker,
		TopicPrefix: orDefault(r.TopicPrefix, "solar_assistant"),
		StatusTopic: orDefault(r.StatusTopic, "solar_collector/status"),
		Inverters:   r.Inverters,
	}
	if c.PollInterval, err = durOrDefault(r.PollInterval, 10*time.Second); err != nil {
		return Config{}, fmt.Errorf("poll_interval: %w", err)
	}
	if c.CommandSpacing, err = durOrDefault(r.CommandSpacing, 850*time.Millisecond); err != nil {
		return Config{}, fmt.Errorf("command_spacing: %w", err)
	}
	if c.ReadTimeout, err = durOrDefault(r.ReadTimeout, time.Second); err != nil {
		return Config{}, fmt.Errorf("read_timeout: %w", err)
	}

	if c.Broker == "" {
		return Config{}, fmt.Errorf("broker must not be empty")
	}
	if len(c.Inverters) == 0 {
		return Config{}, fmt.Errorf("at least one inverter is required")
	}
	if c.PollInterval <= 0 || c.CommandSpacing <= 0 || c.ReadTimeout <= 0 {
		return Config{}, fmt.Errorf("intervals must be positive")
	}
	return c, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func durOrDefault(v string, def time.Duration) (time.Duration, error) {
	if v == "" {
		return def, nil
	}
	return time.ParseDuration(v)
}
