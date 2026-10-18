# Systemd Units

Unit files live in `deploy/systemd/`. West runs most services in `docker compose`; the units are for bare metal (east) plus the host-level jobs on west. Ports: [[Port Registry|Port-Registry]].

## Installed on west (2026-10-16)

| Unit | State | What |
|------|-------|------|
| `huginn.service` | enabled | Host metrics agent as `unheaded-huginn`; `PGPASSWORD` in `/etc/huginn.env` |
| `node-exporter.service` | enabled | Prometheus node_exporter as `node-exporter` (`:9100`) |
| `unheaded-well-backup.timer` | enabled | Nightly backup set, 03:30 |
| `unheaded-well-drill.timer` | enabled | Restore drill of the newest set, Sundays 05:00 |
| `unheaded-well-offsite.service` | static | Copies the newest set to east (run after a good backup) |
| `unheaded-trace-collector.service` | disabled (start per session) | eBPF trace collector beside the compose stack, as `unheaded-trace-collector` with `CAP_BPF`, `CAP_PERFMON`, `CAP_NET_ADMIN` only; logs in the journal (`journalctl -u unheaded-trace-collector`). Installed 2026-10-18 |

West is not up 24/7: timers catch up at boot. Backup paths and alerts: [[Well Backups|Well-Backups]].

## Installed on east

| Unit | What |
|------|------|
| `unheaded-well-offsite-check.timer` | Daily 06:00: verifies and prunes offsite sets (keeps newest 14) |
| `huginn.service` | Boot-enabled metrics agent, pushes to west's VictoriaMetrics |
| `unheaded-{wotan,timeguru,architect,captain,micromanager,akira,dashboard-backend,kanban-app,unheaded-daemon}.service` | From the `.deb` packages (binaries in `/opt/unheaded/bin`, root-owned); each runs as its own account through `/etc/systemd/system/<unit>.service.d/10-isolation.conf` until the packages are rebuilt with the repo units |

The offsite SFTP account is nologin, chrooted and key-only (`deploy/east/sshd-well-offsite.conf`).

## All units in the repo

| Unit | Description |
|------|-------------|
| `unheaded-wotan.service` | Message bus (start first; others depend on it) |
| `unheaded-akira.service` | Health monitor + service sweep |
| `unheaded-dashboard.service` | Dashboard backend, `:20000` |
| `unheaded-kanban.service` | Task board |
| `unheaded-timeguru` / `-architect` / `-captain` / `-micromanager.service` | Court services |
| `unheaded-monad.service`, `unheaded-sophia.service` | State + knowledge graph services |
| `unheaded-daemon.service` | Control plane |
| `unheaded-nft.service` | Loads the last confirmed `inet unheaded` ruleset |
| `unheaded-trace-collector.service` | eBPF trace collector (XDP, TC, kprobes) publishing to Wotan |
| `unheaded-baseline.service` + `.timer` + `.path` | Host baseline agent (ADR-098): every 5 min and on `sshd_config` change |
| `unheaded-well-backup` / `-drill` / `-offsite` / `-offsite-check` | The Well's backup jobs (above) |
| `huginn.service` | Metrics agent |
| `node-exporter.service` | Prometheus node_exporter (third-party binary) |
| `zhen-agentd.service` | Zhen agent daemon |
| `unheaded-doom.target` (+ `-doom-ring`, `-doom-runner`, `-doom-injector`) | Doom on the UPC pipeline |

Run-as users (2026-10-18): every service has its own nologin system account from [`deploy/sysusers.d/unheaded.conf`](../deploy/sysusers.d/unheaded.conf) (`unheaded-wotan`, `unheaded-huginn`, ..., `zhen-agentd`, `node-exporter`, `nginx`), its own `StateDirectory`, and the shared sandbox block ([deploy/systemd/README.md#isolation](../deploy/systemd/README.md#isolation)). The shared `unheaded` account is gone from west and east. `unheaded-nft` runs as its own account with `CAP_NET_ADMIN` only; `unheaded-trace-collector` with `CAP_BPF`, `CAP_PERFMON` and `CAP_NET_ADMIN` only (it ran as root from a hand launch before). Still root (FND-010): `unheaded-baseline`, the Doom units, and the Well's backup/drill/offsite jobs (they `docker exec` into the Well). Compose containers: one uid each from `pkg/uids`, see [[Security]].

---

> **Source:** [deploy/systemd/README.md](../deploy/systemd/README.md) · `systemctl list-unit-files` on west
