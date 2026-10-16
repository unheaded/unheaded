# Systemd Units

Unit files live in `deploy/systemd/`. West runs most services in `docker compose`; the units are for bare metal (east) plus the host-level jobs on west. Ports: [[Port Registry|Port-Registry]].

## Installed on west (2026-10-16)

| Unit | State | What |
|------|-------|------|
| `huginn.service` | enabled | Host metrics agent; `PGPASSWORD` in `/etc/huginn.env` |
| `unheaded-well-backup.timer` | enabled | Nightly backup set, 03:30 |
| `unheaded-well-drill.timer` | enabled | Restore drill of the newest set, Sundays 05:00 |
| `unheaded-well-offsite.service` | static | Copies the newest set to east (run after a good backup) |
| `unheaded-wotan.service`, `unheaded-daemon.service` | disabled | Installed, started by hand |

West is not up 24/7: timers catch up at boot. Backup paths and alerts: [[Well Backups|Well-Backups]].

## Installed on east

| Unit | What |
|------|------|
| `unheaded-well-offsite-check.timer` | Daily 06:00: verifies and prunes offsite sets (keeps newest 14) |
| `huginn.service`, `unheaded-victoria.service` | Boot-enabled metrics agent + VictoriaMetrics (8428) |

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
| `unheaded-baseline.service` + `.timer` + `.path` | Host baseline agent (ADR-098): every 5 min and on `sshd_config` change |
| `unheaded-well-backup` / `-drill` / `-offsite` / `-offsite-check` | The Well's backup jobs (above) |
| `huginn.service`, `unheaded-victoria.service` | Metrics agent + TSDB |
| `zhen-agentd.service` | Zhen agent daemon |
| `unheaded-doom.target` (+ `-doom-ring`, `-doom-runner`, `-doom-injector`) | Doom on the UPC pipeline |

Run-as users (2026-10-16): most services share `User=unheaded`; `zhen-agentd` runs as `zhen`, the offsite check as `well-backup-copy`. No `User=` (root): `unheaded-baseline`, `unheaded-nft`, the Doom units, `unheaded-victoria` (calls `docker compose`), and the backup/drill/offsite jobs (they `docker exec` into the Well).

---

> **Source:** [deploy/systemd/README.md](../deploy/systemd/README.md) · `systemctl list-unit-files` on west
