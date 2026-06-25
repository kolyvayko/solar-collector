# solar-collector

A small, self-hosted **Go telemetry collector for Growatt SPF off-grid
inverters**. It reads the inverters directly over Modbus-RTU (USB), aggregates
them, and:

- publishes **Solar-Assistant-compatible MQTT** so Home Assistant (or anything
  that already consumed Solar Assistant topics) keeps working unchanged, and
- exposes a **Prometheus `/metrics`** endpoint for long-term history in
  VictoriaMetrics + Grafana.

It is a focused, read-only DIY alternative to [Solar Assistant](https://solar-assistant.io/)
for people who would rather own their data on a server they already run, instead
of a dedicated appliance. Developed against **Growatt SPF6000ES Plus** in a
parallel, single-phase, off-grid setup; other SPF models likely work (verify the
register map with the built-in `probe` tool first).

> **Scope (on purpose):** one inverter family (Growatt SPF), read-only, no web
> UI. Multi-brand support and setpoint writing are explicit non-goals — that is
> Solar Assistant's moat and an endless treadmill. This stays small.

---

## Architecture

```
 ┌─────────────┐   Modbus-RTU / USB   ┌──────────────────────────┐
 │ Growatt SPF │──────────────────────│  solar-collector daemon  │
 │  inverter 1 │   (Exar XR21B1411)   │      (Go, systemd)       │
 ├─────────────┤                      │                          │
 │ Growatt SPF │──────────────────────│  • poll on an interval   │
 │  inverter N │                      │  • aggregate (sum/shared)│
 └─────────────┘                      │  • off / fault handling  │
                                      └────────────┬─────────────┘
                                  MQTT (SA-compatible) │ │ /metrics
                                         ┌────────────┘ └────────────┐
                                         ▼                           ▼
                                ┌─────────────────┐        ┌───────────────────┐
                                │  MQTT broker    │        │ VictoriaMetrics    │
                                │  → Home Assistant│        │ → Grafana          │
                                └─────────────────┘        └───────────────────┘
```

The two data paths are fully decoupled: if the metrics stack is down, MQTT/Home
Assistant is unaffected, and vice-versa. One or more inverters are supported.

---

## Features

- **Modbus-RTU over USB** reader for Growatt SPF (SoC, battery V/W, PV1/PV2,
  load, grid, temperature, work-mode status).
- **Multi-inverter aggregation** — sums per-inverter power, treats the battery
  bank as shared, with per-inverter detail preserved.
- **Solar-Assistant-compatible MQTT** — retained plain-number topics under
  `solar_assistant/...`, plus a `solar_collector/status` online/offline LWT.
- **Prometheus `/metrics`** — hand-rolled exposition, **zero extra Go deps**;
  stable per-inverter labels.
- **Robust polling** — *off* (port absent) vs *fault* (read error) are
  distinguished; a partial read while an inverter is faulting never publishes a
  misleading total.
- **`probe` toolkit** — list ports, dump registers, classify sum-vs-shared,
  compare against a live Solar Assistant, and stress-test polling.
- **Turn-key metrics stack** — `docker compose` for VictoriaMetrics + Grafana,
  fully provisioned (datasource + dashboards), with optional nightly backups.
- Single static binary (`CGO_ENABLED=0`), cross-compiles to `linux/amd64`.

---

## Hardware

- **Inverter:** Growatt SPF6000ES Plus (other SPF models use the same register
  family — verify with `probe` first).
- **Link:** the inverter's **USB-B port** presents an **Exar XR21B1411**
  USB-UART (`04e2:1411`). On Linux ≥ 6.8 the in-tree `xr_serial` driver exposes
  it as `/dev/ttyUSBx` — no out-of-tree driver, no `cdc_acm` blacklist needed.
- **Modbus:** 9600 8N1, slave address 1, INPUT registers (FC04), ≥ 850 ms
  spacing between transactions, 1 s timeout.

> ⚠️ **Use a good, short USB cable.** Cheap/long (3 m+) cables enumerate but
> fail under load with `xr_serial: Failed to set line coding: -71` (EPROTO) →
> read timeouts. A known-good ≤ 2 m shielded cable fixes it; for longer runs use
> a powered hub + short cables or an active cable. See *Troubleshooting*.

---

## Build

```sh
make build   # host binary           -> bin/solar-collector
make cross   # linux/amd64 (static)  -> dist/solar-collector-linux-amd64
make test    # unit tests (no hardware needed)
```

Requires Go 1.26+. No CGO.

---

## Configuration

`/etc/solar-collector/config.yaml` (see `deploy/config.example.yaml`):

```yaml
broker: "mqtt-host:1883"            # your MQTT broker (anonymous)
topic_prefix: "solar_assistant"     # SA-compatible drop-in prefix
status_topic: "solar_collector/status"
poll_interval: "10s"
command_spacing: "850ms"            # min spacing between Modbus transactions
read_timeout: "1s"
inverters:
  - "/dev/solar-inv1"               # one entry per inverter (udev symlinks)
  # - "/dev/solar-inv2"
```

---

## The `probe` toolkit

Read-only diagnostics — run these **before** trusting the daemon on new hardware:

```sh
solar-collector probe ports                                       # list candidate USB serial devices
solar-collector probe read       --port /dev/ttyUSB0              # decode one full register snapshot
solar-collector probe classify   --port1 /dev/ttyUSB0 --port2 /dev/ttyUSB1   # sum-vs-shared per register
solar-collector probe compare    --port /dev/ttyUSB0 --sa-broker mqtt-host:1883   # vs a live Solar Assistant
solar-collector probe poll-stress --port /dev/ttyUSB0             # find a safe polling interval
```

`probe compare` is the trust-builder: it confirms SoC and the battery-power
**sign convention** match Solar Assistant before you cut over.

---

## Running as a daemon

```sh
solar-collector run --config /etc/solar-collector/config.yaml [--metrics-addr :9099]
```

`--metrics-addr` defaults to `:9099` (empty string disables the metrics server).

Deployment artifacts under `deploy/`:

- `solar-collector.service` — systemd unit (`dialout` group, restart-on-failure).
- `99-solar-collector.rules` — udev rules that create stable `/dev/solar-invN`
  symlinks **by Exar chip serial** (not by USB port — so moving a cable doesn't
  break the mapping).
- `config.example.yaml` — config schema.

---

## MQTT topics (Solar-Assistant-compatible)

All retained, plain numbers. Powers in watts; `battery_power` uses the SA
convention (**charge +, discharge −**); SoC %, temperature °C.

| Topic | Meaning |
|-------|---------|
| `solar_assistant/total/battery_state_of_charge/state` | battery SoC % |
| `solar_assistant/total/battery_power/state` | battery power W |
| `solar_assistant/total/pv_power/state` | total PV W |
| `solar_assistant/total/load_power/state` | load W |
| `solar_assistant/total/grid_power/state` | grid W (import +, export −) |
| `solar_assistant/inverter_{N}/pv_power/state` | per-inverter PV W |
| `solar_assistant/inverter_{N}/temperature/state` | per-inverter °C |
| `solar_assistant/inverter_{N}/status/state` | Growatt work-mode enum (0–28) |
| `solar_collector/status` | `online` / `offline` (MQTT LWT) |

---

## Metrics & dashboards

The daemon serves a Prometheus endpoint at `:9099/metrics` (gauges):

```
solar_inverters_online
solar_inverter_up{inverter="N"}
solar_battery_soc_percent
solar_battery_power_watts          # charge +, discharge −
solar_battery_voltage_volts
solar_pv_power_watts
solar_load_power_watts
solar_grid_power_watts
solar_inverter_pv_power_watts{inverter="N"}
solar_inverter_temperature_celsius{inverter="N"}
solar_inverter_status{inverter="N"}
solar_build_info{version="..."}
```

A turn-key stack lives in `deploy/metrics/` — VictoriaMetrics (3-year retention)
scrapes the daemon, and Grafana is auto-provisioned with a datasource and two
dashboards (an overview and a detailed view):

```sh
cd deploy/metrics
docker compose -p solar-metrics up -d
```

By default VictoriaMetrics scrapes the daemon at `host.docker.internal:9099`
(edit `deploy/metrics/vmscrape.yml` for your topology). Energy (kWh) is computed
with MetricsQL `integrate()`, e.g. `integrate(solar_pv_power_watts)/3600/1000`.

### Backups

`deploy/metrics/backup-metrics.sh` + the `solar-metrics-backup.timer` take a
nightly **consistent VictoriaMetrics snapshot**, stream it as a gzip tarball to a
remote host over SSH, and keep the newest N. Restore = stop VictoriaMetrics,
unpack the tarball into the data dir, start. Adjust the destination host/path at
the top of the script.

### Secrets

The Grafana admin password is kept **sops-encrypted** (`secrets/metrics.yaml`,
age recipient in `.sops.yaml`). `scripts/gen-secrets.sh` decrypts it into a
git-ignored `.env` that `docker compose` reads. Plaintext secrets are never
committed. (To run the stack without sops, just set
`GRAFANA_ADMIN_PASSWORD` in the environment or edit the compose file.)

---

## Protocol / register map

Modbus INPUT registers (FC04), scaled values:

| Register | Field | Scale |
|----------|-------|-------|
| 0 | work-mode status | enum 0–28 |
| 3–4 / 5–6 | PV1 / PV2 power | int32 ×0.1 W |
| 9–10 | load power | int32 ×0.1 W |
| 17 | battery voltage | ×0.01 V |
| 18 | battery SoC | % |
| 25 | inverter temperature | ×0.1 °C |
| 36–37 | grid power | int32 ×0.1 W |
| 77–78 | battery power | int32 ×0.1 W (raw: discharge +, charge −) |

The raw Growatt battery sign is negated to the SA convention in exactly one
place (`internal/aggregate`), so the sign is never ambiguous downstream.

---

## Known limitations

- **Read-only.** No setpoint/control writes.
- **One inverter family.** Growatt SPF only.
- **External charge sources are invisible.** If a solar string charges the
  battery through an *external* controller that bypasses the inverters, Modbus
  only sees inverter PV — total PV will undercount it, and the inverter
  battery-power registers may miss the external current. Accurate total battery
  flow in that case needs a separate battery shunt/BMS source (not implemented).

---

## Repository layout

```
cmd/solar-collector/   CLI entrypoint (run, probe)
internal/
  inverter/            Modbus reader + register decode
  aggregate/           multi-inverter aggregation (battery sign single-sourced here)
  config/              YAML config + validation
  mqtt/                SA-compatible publisher (+ LWT)
  run/                 daemon: poll loop, off/fault, reconnect, graceful shutdown
  metrics/             hand-rolled Prometheus exposition
  probe/               diagnostics toolkit
deploy/                systemd unit, udev rules, config example
deploy/metrics/        VictoriaMetrics + Grafana compose, provisioning, backups
```

---

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| `xr_serial: Failed to set line coding: -71` + read timeouts | Marginal USB cable (too long/cheap). Use a short shielded cable, powered hub, or active cable. |
| `/dev/solar-invN` missing after re-plugging | udev matches by **chip serial**; capture it with `udevadm info -q property -n /dev/ttyUSBx \| grep ID_SERIAL_SHORT` and add a rule line. |
| No telemetry, `solar_collector/status = offline` | Check the daemon: `journalctl -u solar-collector -f`. |
| Grafana energy panels empty | `integrate()` needs accumulated history — wait for data to build up. |
| Permission denied on `/dev/ttyUSB*` | Port is `root:dialout`; add the service user to `dialout` (the unit does this). |

---

## Disclaimer

This talks to mains-connected power hardware. It is **read-only**, but you use it
at your own risk. Not affiliated with Growatt or Solar Assistant. Always verify
against your own inverter with `probe` before relying on any value.
