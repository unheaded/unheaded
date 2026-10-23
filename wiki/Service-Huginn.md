# Huginn — Host Metrics Agent

Huginn runs natively on each Kingdom host (west, east), not in a container. It reads the real `/proc` and every mounted filesystem, and serves and pushes host metrics. It's Odin's raven, "thought", and its pair Muninn ("memory", ADR-086) is planned.

## Running

| | |
|---|---|
| Binary | `/usr/local/bin/huginn` (built from `cmd/host-agent`; ADR-084) |
| Unit | `huginn.service`, runs as its own nologin account `unheaded-huginn` (`deploy/sysusers.d/unheaded.conf`) |
| Config | `/etc/huginn.yaml` (template `cmd/host-agent/huginn.yaml.example`); secrets in `/etc/huginn.env` (root 0600) |
| Listen | `:9110` by default |
| Push | VictoriaMetrics every 10 s (west local `:8428`; east pushes to west over the P2P link). **VictoriaMetrics is not running on west today**, so each push fails (`vm push failed ... connection refused` in the journal); `/metrics` still serves everything (FND-001) |

## What it reports

- `host_*` series: CPU, memory, network, every filesystem/partition, process and connection counts (host/mount/device labels).
- `/var/log` free space and per-service log directory sizes (ADR-092), with alerts for filling within 6 h, a directory over 1 GB, or growth over 1 MiB/s.
- The Well: `pg_stat_database` per database as `huginn_reader` (pg_monitor, TLS, 3 connections).
- **Textfile collector:** `*.prom` files in `textfile_dir` (`/var/lib/unheaded/textfile` on west). Root jobs Huginn cannot observe write there: The Well's backup, restore drill and offsite copy ([[Well Backups|Well-Backups]]). Only valid Prometheus text lines are forwarded; others are counted in `huginn_textfile_rejected_lines`.

## Endpoints

| | |
|---|---|
| `GET /metrics` | Prometheus text |
| `GET /host-summary` | JSON in the dashboard's per-host shape |
| `GET /health` | liveness |

---

> **Source:** [cmd/host-agent/](../cmd/host-agent/) · [ADR-084](../docs/adr/ADR-084-huginn-host-metrics-agent.md) · [deploy/systemd/](../deploy/systemd/)
