# Systemd Units

Unit files live in `deploy/systemd/`. West runs most services in `docker compose`; the units are for bare metal (east) plus the host-level jobs on west. Ports: [[Port Registry|Port-Registry]].

## Installed on west (2026-10-16)

| Unit | State | What |
|------|-------|------|
| `huginn.service` | enabled | Host metrics agent as `unheaded-huginn`; `PGPASSWORD` in `/etc/huginn.env` |
| `node-exporter.service` | enabled | Prometheus node_exporter 1.12.1 (release checksum verified) as `node-exporter` (`:9100`) |
| `unheaded-kev-poller.service` | enabled | CISA KEV poller (ADR-055) as `unheaded-kev-poller`, `:19120`; installed 2026-10-20 |
| `unheaded-compliance-evidence.timer` (user, govan) | enabled | Every 2 h: `go run ./cmd/compliance-evidence` into `var/compliance/evidence.json` (dashboard-backend reads it; `ComplianceEvidenceSilent` after 6 h). Linger on for govan. Installed 2026-10-20 |
| `unheaded-well-backup.timer` | enabled | Nightly backup set, 03:30 |
| `unheaded-well-drill.timer` | enabled | Restore drill of the newest set, Sundays 05:00 |
| `unheaded-well-offsite.service` | static | Copies the newest set to east (run after a good backup) |
| `unheaded-trace-collector.service` | enabled (2026-10-20; before that started by hand, so nothing ran after a reboot) | eBPF trace collector beside the compose stack: XDP (packet-marker) and TC (flow-tracker) on `br-unhe-data`, 6 TCP kprobes (latency-probe), objects in `/usr/local/lib/unheaded/ebpf/`. Runs as `unheaded-trace-collector` with `CAP_BPF`, `CAP_PERFMON`, `CAP_NET_ADMIN` only; health on `:16671/health`; logs in the journal (`journalctl -u unheaded-trace-collector`). Installed 2026-10-18 |

West is not up 24/7: timers catch up at boot. Backup paths and alerts: [[Well Backups|Well-Backups]].

## Installed on east

| Unit | What |
|------|------|
| `unheaded-well-offsite-check.timer` | Daily 06:00: verifies and prunes offsite sets (keeps newest 14) |
| `huginn.service` | Boot-enabled metrics agent, pushes to west's VictoriaMetrics (`192.168.13.2:8428`, the P2P link) |
| `node-exporter.service` | node_exporter 1.12.1 as `node-exporter`, `:9100`; scraped by west's monitoring Prometheus over the overlay, so `NodeExporterDown` for `[fd00:dead:beef::2]:9100` also means the overlay is down. Installed 2026-10-20 |
| `unheaded-kev-poller.service` | CISA KEV poller, `:19120`. Installed 2026-10-20 |
| `unheaded-trace-collector.service` | Same unit, binary and eBPF objects as west, enabled 2026-10-20. Attaches to `lo` (`TRACE_IFACE=lo` in `/etc/unheaded/trace-collector.env`): east's services run on the host network, so their traffic is on loopback. Publishes to east's Wotan. The old `unheaded-ebpf` 1.0.0 package (23 objects in `/opt/unheaded/ebpf` that nothing loaded) was removed; a copy is in `/var/lib/unheaded/remediate/20261020T181913Z-ebpf-pkg/` |
| `unheaded-{wotan,timeguru,architect,captain,micromanager,akira,dashboard-backend,kanban-app,unheaded-daemon}.service` | `.deb` packages `0.1.0~alpha+<commit time>.g<hash>` (2026-10-21: the hand-versioned 1.0.0/1.0.1 packages, including the unbuilt `unheaded-gateway` and `unheaded-zhenai` 1.0.0, were purged first; backup `/var/lib/unheaded/remediate/20261021T113359Z-deb-purge/`) with the repo units (own account each; binaries in `/opt/unheaded/bin`, root-owned). The interim `10-isolation.conf` drop-ins were removed then. Akira reads `AKIRA_OPTS=--config /opt/unheaded/etc/akira.yaml` from `/etc/unheaded/akira.env`. Wotan reads its allowlist from `/etc/unheaded/wotan.yaml` (`configs/wotan.yaml`; the package ships it as a conffile since 2026-10-20, before which it was missing and Wotan refused every service's publishes) |

The offsite SFTP account is nologin, chrooted and key-only (`deploy/east/sshd-well-offsite.conf`).

## All units in the repo

| Unit | Description |
|------|-------------|
| `unheaded-wotan.service` | Message bus (start first; others depend on it) |
| `unheaded-akira.service` | Health monitor + service sweep |
| `unheaded-kev-poller.service` | CISA KEV feed poller (ADR-055), `:19120`; always on, installed on west and east 2026-10-20 |
| `unheaded-dashboard.service` | Dashboard backend, `:20000` |
| `unheaded-kanban.service` | Task board |
| `unheaded-timeguru` / `-architect` / `-captain` / `-micromanager.service` | Court services |
| `unheaded-monad.service`, `unheaded-sophia.service` | State + knowledge graph services |
| `unheaded-daemon.service` | Control plane |
| `unheaded-nft.service` | Loads the last confirmed `inet unheaded` ruleset |
| `unheaded-trace-collector.service` | eBPF trace collector (XDP, TC, kprobes) publishing to Wotan |
| `unheaded-baseline.service` + `.timer` + `.path` | Host baseline agent (ADR-098): every 5 min and on `sshd_config` change. Runs as `unheaded-baseline` with CAP_DAC_READ_SEARCH + CAP_NET_ADMIN and deliberately **no** mount sandboxing: an auditor in a namespace audits the namespace. Installed on east and west 2026-10-22; report at `/var/lib/unheaded/baseline/report.json` |
| `unheaded-well-backup` / `-drill` / `-offsite` / `-offsite-check` | The Well's backup jobs (above) |
| `huginn.service` | Metrics agent |
| `node-exporter.service` | Prometheus node_exporter (third-party binary) |
| `zhen-agentd.service` | Zhen agent daemon |
| `unheaded-doom.target` (+ `-doom-ring`, `-doom-runner`, `-doom-injector`) | Doom on the UPC pipeline |

Run-as users (2026-10-18): every service has its own nologin system account from [`deploy/sysusers.d/unheaded.conf`](../deploy/sysusers.d/unheaded.conf) (`unheaded-wotan`, `unheaded-huginn`, ..., `zhen-agentd`, `node-exporter`, `nginx`), its own `StateDirectory`, and the shared sandbox block ([deploy/systemd/README.md#isolation](../deploy/systemd/README.md#isolation)). The shared `unheaded` account is gone from west and east. `unheaded-nft` runs as its own account with `CAP_NET_ADMIN` only; `unheaded-trace-collector` with `CAP_BPF`, `CAP_PERFMON` and `CAP_NET_ADMIN` only (it ran as root from a hand launch before). Still root (FND-010): `unheaded-baseline`, the Doom units, and the Well's backup/drill/offsite jobs (they `docker exec` into the Well). Compose containers: one uid each from `pkg/uids`, see [[Security]].

---

> **Source:** [deploy/systemd/README.md](../deploy/systemd/README.md) · `systemctl list-unit-files` on west
