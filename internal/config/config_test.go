package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTmp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_FullConfig(t *testing.T) {
	p := writeTmp(t, `
broker: "mqtt-host:1883"
topic_prefix: "solar_assistant"
status_topic: "solar_collector/status"
poll_interval: "10s"
command_spacing: "850ms"
read_timeout: "1s"
inverters:
  - "/dev/solar-inv1"
  - "/dev/solar-inv2"
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Broker != "mqtt-host:1883" || c.TopicPrefix != "solar_assistant" {
		t.Fatalf("strings: %+v", c)
	}
	if c.PollInterval != 10*time.Second || c.CommandSpacing != 850*time.Millisecond || c.ReadTimeout != time.Second {
		t.Fatalf("durations: %+v", c)
	}
	if len(c.Inverters) != 2 || c.Inverters[0] != "/dev/solar-inv1" {
		t.Fatalf("inverters: %+v", c)
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	p := writeTmp(t, `
broker: "mqtt-host:1883"
inverters: ["/dev/ttyUSB0"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.TopicPrefix != "solar_assistant" || c.StatusTopic != "solar_collector/status" {
		t.Fatalf("default topics: %+v", c)
	}
	if c.PollInterval != 10*time.Second || c.CommandSpacing != 850*time.Millisecond || c.ReadTimeout != time.Second {
		t.Fatalf("default durations: %+v", c)
	}
}

func TestLoad_RejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"empty broker":  "broker: \"\"\ninverters: [\"/dev/ttyUSB0\"]\n",
		"no inverters":  "broker: \"mqtt-host:1883\"\ninverters: []\n",
		"bad duration":  "broker: \"x:1883\"\npoll_interval: \"nope\"\ninverters: [\"/dev/ttyUSB0\"]\n",
	}
	for name, body := range cases {
		p := writeTmp(t, body)
		if _, err := Load(p); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
