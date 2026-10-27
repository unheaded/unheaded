# Akira — Kingdom Health Daemon

Akira checks every Kingdom service, publishes its reports to Wotan, tallies every reporter's votes, and publishes the health band. It implements the percentage-consensus doctrine (ADR-029). It's named after the film.

## Running

| | |
|---|---|
| Container | `unheaded-akira` (compose service `akira`, live on west) |
| Port | 19100 (`pkg/ports.Akira`) |
| Wotan topics | publishes `system.health.reports` (each reporter's checks), `system.health.consensus` (band changes) and `system.health.remediate` (suggestions) |
| Other voters | every service that calls `discovery.Start` reports its own `/health` every 30 s as reporter `<service>@<host>` (`pkg/healthreport`; host from `$KINGDOM_NODE`) |
| Remediation | entering CRITICAL or PANIC publishes one `{service, host, band, pct, suggested, at}` suggestion to `system.health.remediate` and counts it in `akira_remediate_suggested_total{service}`; restarting stays off by default: `--remediate` restarts `unheaded-<name>` when two-thirds of reporters see it failing, at most 3 times per outage, then escalates |

## Exec checks

A target with `type: exec` runs a local command instead of an HTTP GET (S-IRON-GATE Phase 10). Exit 0 votes healthy; a non-zero exit, a timeout or a program that cannot start votes failing, and the report's error carries the exit and the output's tail.

```yaml
- name: cert-expiry
  type: exec
  argv: ["/usr/bin/python3", "-B", "/opt/unheaded/scripts/run-runbook.py", "/opt/unheaded/runbooks/pki/akira-cert-expiry-check.yaml"]
  interval: 1h      # between runs each sweep re-votes the last result; empty = every sweep
  timeout: 30s      # default and ceiling; kills the whole process group
  env: {UNHEADED_TLS_DIR: /etc/unheaded/tls}   # optional
```

- argv only: `argv[0]` must be an absolute path, nothing goes through a shell or a PATH lookup.
- Runs as Akira's own uid (`unheaded-akira`, 16731 in compose), stdin `/dev/null`, cwd `/`, environment `PATH`, `LANG` and the target's `env` only (Akira's key and auth settings are not passed on).
- Keeps the last 4 KiB of stdout+stderr.
- `--remediate` never restarts an exec target: it has no unit. At the threshold Akira logs that a human is needed.

`cert-expiry` (`runbooks/pki/akira-cert-expiry-check.yaml`) is the first: it reads every certificate under `/etc/unheaded/tls/<identity>/` (fallback `/var/lib/unheaded/pki`) and fails at 50 % of lifetime left (WARN), 25 % (ERROR), 10 % or expired (exit 2); at 66 % it reports rotation due and passes. It is read-only: rotation is `runbooks/pki/rotate-service-cert.yaml` (dev: `scripts/gen-dev-certs.sh`). A directory Akira cannot list counts as failing, so certificate directories on hosts must be listable by `unheaded-akira` (certs 0644; keys stay 0640/0600). Hosts run it hourly (`configs/akira-{west,east}.yaml`; the runner ships in `unheaded-configs` at `/opt/unheaded/scripts/`); the compose stack runs it every sweep against each listener's certificate mounted read-only under `/etc/unheaded/tls-watch/`.

## Bands

`failing_reporters / reporters x 100`, where each reporter's latest report from the last three sweeps counts as one vote:

| % | Band |
|---|---|
| 0 – 12.49 | OK |
| 12.50 – 37.49 | WARN |
| 37.50 – 62.49 | ERROR |
| 62.50 – 87.49 | CRITICAL (auto-remediate from 66.67%) |
| 87.50 – 100 | PANIC |

Health state lands in The Well (`unheaded_ops.service_health_current` / `_transitions`) via dashboard-backend.

## Endpoints and flags

| | |
|---|---|
| `GET /health`, `GET /ready` | liveness, readiness |
| `GET /metrics` | Prometheus metrics |
| `GET /api/v1/status` | current band per service |
| `akira --once` | one sweep, print, exit |
| `--config`, `--node`, `--port`, `--wotan` | config file, node id (default hostname), API port, Wotan HTTP address |

---

> **Source:** [cmd/akira/main.go](../cmd/akira/main.go) · [ADR-029](../docs/adr/ADR-029-wotan-consensus-health-remediation.md) · [pkg/health/consensus.go](../pkg/health/consensus.go)
