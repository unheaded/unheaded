# kev-poller: CISA Known Exploited Vulnerabilities feed

kev-poller keeps CISA's KEV catalog current on a host, tags the entries that name something the Kingdom runs, and announces changes on Wotan (ADR-055). It is what refreshes the [threat register](../docs/security/threat-register.md).

## Running

| | |
|---|---|
| Unit | `unheaded-kev-poller.service` as `unheaded-kev-poller` (no capabilities); enabled on west and east since 2026-10-20 ([[Systemd Units|Systemd-Units]]). Also a Dockerfile stage and a DaemonSet (`deploy/k8s/kev-poller/`) for when there is a cluster |
| Port | 19120 (`pkg/ports.KEVPoller`) |
| State | `/var/lib/unheaded/threat-feeds/cisa-kev/`: `latest.json` (as published), `state.json`, `snapshots/` (one per change, 30 days) |
| Wotan topics | `threats.cisa-kev.updates` every good pull; `threats.cisa-kev.delta` when entries are added or removed (chunks of 50, stack entries carry `categories`) |
| Stack file | `--stack` / `KEV_STACK`; the unit reads `/etc/unheaded/kev-stack.yaml`, installed from `config/kev-poller/stack.yaml`: what we run, in KEV's own vendor/product names, exact match |

## Behaviour

- Hourly (15m..24h) conditional GET: an unchanged catalog costs a 304. https only, no redirect off https, 32 MiB cap.
- A catalog replaces the stored one only after validation (count matches, cveIDs well formed and unique). A failed pull keeps the last good copy and retries from 1 minute, doubling up to the interval.
- The first catalog a poller stores is a baseline: no delta.

## Endpoints and commands

| | |
|---|---|
| `GET /health`, `GET /ready` | ready once a valid catalog is stored |
| `GET /api/v1/kev` | stored catalog version, count, sha256, last error, next pull |
| `GET /metrics` | `unheaded_kev_pull_{success,failure}_total`, `unheaded_kev_last_success_timestamp_seconds`, `unheaded_kev_total_count`, `unheaded_kev_stack_count`, ... |
| `kev-poller --once` | one pull, print the delta, exit |
| `kev-poller report --since YYYY-MM-DD` | the threat register's refresh entry, from the stored catalog |

Alerts: `KEVFeedStale` (6 h), `KEVFeedExpired` (7 days, ADR-052), `KEVPollerDown` (`monitoring/prometheus/rules/threat-feeds.yml`).

---

> **Source:** [cmd/kev-poller/main.go](../cmd/kev-poller/main.go) · [pkg/threatfeed/kev](../pkg/threatfeed/kev) · [ADR-055](../docs/adr/ADR-055-kev-poller-always-on-service.md)
