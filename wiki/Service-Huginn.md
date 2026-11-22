# Huginn — Host Metrics Agent

Huginn runs natively on each Kingdom host (west, east), not in a container. It reads the real `/proc` and every mounted filesystem, and serves and pushes host metrics. It's Odin's raven, "thought", and its pair Muninn ("memory", ADR-086) is planned.

## Running

| | |
|---|---|
| Binary | `/usr/local/bin/huginn` (built from `cmd/huginn`; ADR-084) |
| Unit | `huginn.service`, runs as its own nologin account `unheaded-huginn` (`deploy/sysusers.d/unheaded.conf`) |
| Config | `/etc/huginn.yaml` (template `cmd/huginn/huginn.yaml.example`); secrets in `/etc/huginn.env` (root 0600; on west a symlink to `/etc/unheaded/huginn.env`) |
| Listen | `127.0.0.1:9110` by default; huginn has no auth, so `HUGINN_LISTEN` in `/etc/huginn.env` lists each host's internal addresses (loopback, `192.168.13.x` for the dashboard, the overlay for Prometheus). A wildcard (`:9110`, `0.0.0.0`, `[::]`) stops the start, and with `Restart=always` that is a restart loop. `-listen` on `ExecStart` wins over `HUGINN_LISTEN`, and a unit in `/etc/systemd/system` shadows the packaged one, so read `systemctl cat huginn` before a new binary goes on a host: the unit, `/etc/huginn.env` and `/etc/huginn.yaml` must all name addresses first |
| Push | VictoriaMetrics every 10 s (`HUGINN_VM`: west `http://localhost:8428`, the main compose `victoria`; east pushes to west over the P2P link). While `victoria` is down each push fails (`vm push failed ... connection refused` in the journal); `/metrics` still serves everything |

## What it reports

- `host_*` series: CPU, memory, network, every filesystem/partition, process and connection counts (host/mount/device labels).
- `/var/log` free space and per-service log directory sizes (ADR-092), with alerts for filling within 6 h, a directory over 1 GB, or growth over 1 MiB/s.
- The Well: `pg_stat_database` per database as `huginn_reader` (pg_monitor, TLS, 3 connections).
- **Per-cgroup series** (ADR-147, `cmd/huginn/cgroups.go`): every top-level `*.slice`, and below a slice every `*.slice`, `*.service` and `docker-<id>.scope`, to depth 4, at most 512 groups; read only from cgroupfs (`-cgroup-root`, default `/sys/fs/cgroup`; empty turns it off). Session scopes and mounts are skipped. Labels `host`, `cgroup` (a docker scope is shortened to its 12-hex id) and `container_id` on docker scopes. Series: `huginn_cgroup_cpu_usage_seconds_total`, `_cpu_periods_total`, `_cpu_throttled_periods_total`, `_cpu_throttled_seconds_total`, `huginn_cgroup_memory_bytes`, `_memory_max_bytes` (only when a limit is set), `_memory_swap_bytes`, `_memory_events_total{event}` (low, high, max, oom, oom_kill), `huginn_cgroup_io_bytes_total{device,dir}`, `_io_ops_total{device,dir}`, `huginn_cgroup_pressure_seconds_total{resource,kind}`, `huginn_cgroup_pids`, `_pids_max` (only when a task cap, `TasksMax`, is set; SliceTasksNearMax reads the two), and `huginn_cgroups_seen`, `huginn_cgroups_truncated`. `memory.events` is hierarchical: a slice keeps the kill count of a container that died with its scope, so MemoryKill reads the slices. West, 2026-11-15: 74 groups, 23 docker scopes.
- **Textfile collector:** `*.prom` files in `textfile_dir` (`/var/lib/unheaded/textfile` on west). Root jobs Huginn cannot observe write there: The Well's backup, restore drill and offsite copy ([[Well Backups|Well-Backups]]). Only valid Prometheus text lines are forwarded; others are counted in `huginn_textfile_rejected_lines`.

## Endpoints

| | |
|---|---|
| `GET /metrics` | Prometheus text |
| `GET /host-summary` | JSON in the dashboard's per-host shape |
| `GET /health` | liveness |

---

> **Source:** [cmd/huginn/](../cmd/huginn/) · [ADR-084](../docs/adr/ADR-084-huginn-host-metrics-agent.md) · [deploy/systemd/](../deploy/systemd/)
