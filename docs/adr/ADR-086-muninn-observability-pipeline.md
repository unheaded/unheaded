# ADR-086 — Muninn: Observability Fan-Out Pipeline

**Status:** Planned
**Date:** 2026-07-24
**Author:** Stevie Bellis
**Deciders:** Stevie Bellis

---

## Context

Kingdom observability currently has a direct-push model: Huginn pushes host
metrics straight to VictoriaMetrics; eBPF events flow through trace-collector
directly to Wotan. There is no fan-out, no routing layer, and no SIEM sink.

Several gaps this creates:

1. **No audit trail.** SSH logins, TTY sessions, sudo invocations, and PAM
   events are logged by journald but never forwarded to a queryable store.
   For a security-conscious platform this is a significant blind spot.

2. **Sink coupling.** Every producer knows the exact address of its sink.
   Changing VictoriaMetrics to a different TSDB, or adding a second sink,
   requires modifying every producer binary.

3. **No normalisation.** Metrics, logs, and events arrive in different formats
   (Prometheus text, JSON, syslog). A pipeline layer can normalise before
   fan-out, enabling cross-correlation.

4. **No SIEM.** The BlackMage/Sentinel hardening brief (2026-05-13) flagged
   SIEM as a required control. Without an aggregation layer there is nowhere
   to send structured security events.

The Norse name is already reserved: Huginn and Muninn are Odin's twin ravens.
Huginn ("thought") observes and reports. Muninn ("memory") retains and routes —
precisely the role of an observability pipeline that persists data to sinks.

---

## Decision

Build **Muninn** — a Kingdom-native observability fan-out pipeline daemon.

### Role

Muninn sits between producers (Huginn, trace-collector, journald, eBPF ring
buffers) and sinks (VictoriaMetrics, PostgreSQL, SIEM). It:

- subscribes to Wotan topics and/or tails log sources
- normalises events into a common envelope
- fans out to one or more configured sinks
- buffers and retries on sink unavailability

Muninn does NOT collect metrics itself — that is Huginn's job. Muninn routes.

### Sources

| Source | What it provides | Mechanism |
|--------|-----------------|-----------|
| Huginn | Host metrics (CPU/mem/disk/net/procs) | Wotan topic `host.metrics.<host>` |
| trace-collector | eBPF flow/latency/packet events | Wotan topics `ebpf.*` |
| journald | System logs, service unit logs | `journalctl --follow --output json` |
| auth.log / PAM | SSH logins, TTY sessions, sudo, su | journald filter on `sshd`, `sudo`, `login`, `pam_unix` |
| eBPF audit | Privileged syscalls, capability use | Future: dedicated eBPF program |

### Sinks

| Sink | Data types | Format |
|------|-----------|--------|
| VictoriaMetrics | Metrics | Prometheus remote-write / text push |
| PostgreSQL (The Well) | Audit events, login history | INSERT into `ops.audit_events` |
| SIEM endpoint | Security events (auth, sudo, anomalies) | Syslog RFC 5424 / CEF / JSON-over-TCP |
| Loki / Elastic (future) | Structured logs | Loki push API / Elastic bulk API |

### SSH / TTY login history → SIEM

This is the first concrete use case. Muninn tails journald filtering on
`SYSLOG_IDENTIFIER=sshd` and `pam_unix`, extracts:

```
timestamp, host, user, source_ip, action (login/logout/failed), session_id
```

and forwards to:
- `ops.login_events` table in PostgreSQL (queryable history)
- SIEM endpoint as CEF or JSON (real-time alerting on failed attempts, unusual IPs)

### Configuration

Muninn is fully YAML-configured. Routing rules, source addresses, sink
credentials, and buffer sizes are all in `/etc/muninn.yaml`. No recompile
needed to add a sink or change a source.

```yaml
# /etc/muninn.yaml (example)
host_label: west

sources:
  wotan:
    url: http://localhost:18000
    topics:
      - host.metrics.*
      - ebpf.flow.events
      - ebpf.latency.events
  journald:
    enabled: true
    filters:
      - SYSLOG_IDENTIFIER=sshd
      - SYSLOG_IDENTIFIER=sudo
      - SYSLOG_IDENTIFIER=login
      - SYSLOG_IDENTIFIER=pam_unix

sinks:
  victoria_metrics:
    enabled: true
    url: http://localhost:8428
    push_interval: 15s
  postgres:
    enabled: true
    dsn: "${MUNINN_PG_DSN}"
    tables:
      audit_events: ops.audit_events
      login_events: ops.login_events
  siem:
    enabled: false           # enable when SIEM endpoint is provisioned
    url: "tcp://siem.internal:514"
    format: cef              # cef | syslog5424 | json

buffer:
  max_events: 10000
  flush_interval: 5s
  retry_backoff: 30s
```

### Systemd unit

`deploy/systemd/muninn.service` — same hardening baseline as Huginn and
zhen-agentd. `User=unheaded`, `ProtectSystem=strict`, `CapabilityBoundingSet=`.

---

## Relationship to Huginn

```
Host OS
  └─ Huginn
       ├─ GET /host-summary   → dashboard (direct, low-latency)
       ├─ GET /metrics        → Prometheus scrape (direct)
       └─ Wotan publish       → host.metrics.<host>
                                    │
                              Muninn (fan-out)
                                    ├─ VictoriaMetrics (TSDB history)
                                    ├─ PostgreSQL (audit/query)
                                    └─ SIEM (security events)
```

Huginn's direct VictoriaMetrics push (current behaviour) is retained for
simplicity until Muninn is deployed; at that point the `sinks.victoria_metrics`
block in huginn.yaml can be disabled and Muninn takes over.

---

## Implementation Phases

**Phase 1 — Skeleton + journald → PostgreSQL**
- `cmd/muninn/main.go`: YAML config load, journald tail, PostgreSQL sink
- Schema: `ops.login_events (id, ts, host, user, src_ip, action, raw)`
- SSH/TTY login events flowing to The Well

**Phase 2 — Wotan source + VictoriaMetrics sink**
- Subscribe to `host.metrics.*` from Wotan
- Push to VictoriaMetrics (replaces Huginn's direct push)
- Buffer + retry on Victoria unavailability

**Phase 3 — SIEM sink**
- CEF / syslog5424 formatter
- TCP/TLS forwarding to external SIEM endpoint
- Failed login alerting rule

**Phase 4 — eBPF event routing**
- Subscribe to `ebpf.*` topics
- Route anomaly events to SIEM sink
- Correlate with login history in PostgreSQL

---

## Consequences

- Muninn is the single egress point for all Kingdom observability data
- Huginn's direct Victoria push becomes optional/deprecated post-Phase 2
- SSH login history is queryable in PostgreSQL from Phase 1
- Adding a new sink requires only a YAML change + Muninn restart — no producer changes
- Muninn failure is non-fatal to producers (Huginn still serves HTTP endpoints)

---

## Related

- ADR-084 — Huginn (host metrics agent, the primary Muninn source)
- ADR-085 — CI/CD artifact layout (/var/ paths, .deb packaging)
- `docs/adr/ADR-083` — terminology conventions
- `runbooks/security/incident-response.yaml` — SIEM is a required control
- `security_upc_linux_gain_vs_risk.md` — BlackMage/Sentinel isolation brief
