<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-LOG-HARVEST progress

Plan: `docs/battle-plans/S-LOG-HARVEST-battle-plan.md` (12 phases, 214 steps). This file holds the three sections Step 11 asks for: Decisions, Step log, Stuck.

## Decisions

Asked 2026-10-31, answered by Stevie 2026-11-03 (rapid-fire sitting; recorded from `~/tmp/next.md`).

| # | Question (short) | Answer | Effect |
|---|---|---|---|
| D1 | Keep Vector → ClickHouse as collector and store while the third-party verdicts are on hold? | **Yes, configure only** (as recommended) | Phases 3, 4, 6, 7, 9 unblocked (code and config steps; runtime `[V]` steps wait for the stack, FND-001) |
| D2 | What receives syslog from devices and the Pi-hole container? | **Native `cmd/syslog-relay`** (Go, stdlib) (as recommended) | Phase 5 goes ahead |
| D3 | How do Pi-hole's logs leave the container? | **rsyslog inside the container → relay** (as recommended) | Phase 6 unblocked |
| D4 | How does east ship? | **Install the prepared `vector-east` in a host window** (as recommended) | Phase 7 unblocked; Step 170 is Stevie's host window |
| D5 | Third-party container stdout → journald driver? | **Yes** (as recommended) | Phase 4 unblocked |
| D6 | Audit records: journald-audit socket or file source? | **File source**, auditd `log_group = adm` (as recommended) | Phase 3 audit steps unblocked |
| D7 | Network devices: which models, syslog-capable, allow 514 from their IPs? | **Stevie later** (device list not given) | **Phase 8 BLOCKED** until the list arrives |

Step 2 is done by this record. Sequencing from the same sitting: E1 stack up (attended) is the focus; this sprint's runtime steps follow it.

Related decisions recorded the same day: ADR-116 (Q1-Q6), ADR-114 (Q0-Q7), ADR-115 (Q2, Q6, Q7 → ADR-117).

## Step log

| Date | Steps | Note |
|---|---|---|
| 2026-11-03 | 2, 11 | Decisions recorded (above); this file created. Unattended churn, `churn/2026-11-03`. |
| 2026-11-03 | 13-34 | Phase 1 source inventory: `config/logging/sources.yaml` written from the live hosts (Steps 13-19 outputs in this file's gap map below), unique ids, parses with the CI YAML reader, retention classes from retention-policy-v1 (audit / operational), privacy note in the file header. |
| 2026-11-03 | 35-58 | Phase 2 static coverage gate: `pkg/logsources` (Load/Validate, six tests), `scripts/check-log-sources.sh`, meta-gate provocation `check-log-sources` (a unit file with no row), CI step in security.yml next to the log-caps gate, `docs/policy/log-sources.md`. Provoked red three ways (stray unit, stray compose service, duplicate id), each naming the culprit; gosec ratchet green; no new module (yaml.v3 was already direct). |
| 2026-11-04 | 105-140 | Phase 5 native syslog relay, worktree `sprint/s-log-harvest`. ADR-124 (the plan's ADR-111 is the command-line ADR, so the number moved). `pkg/syslog`, `pkg/journald`, `cmd/syslog-relay`, unit, uid 16739, runbook `observe/syslog-relay`, three alerts. Fuzz: FuzzParse and FuzzFrame 120 s each, FuzzFrameRoundTrip 30 s, no crashers. Nine defences mutated, each killed by a test. Rehearsed against the real journald as the normal user on loopback high ports, stopped by pid: fields arrive, `_TRANSPORT=journal`. Nothing installed or started. Step 132 stuck (below). |

## Gap map (burn-down)

Sources by status on 2026-11-03 (Phase 1): **170 entries**: shipped 0, gap 58, blocked 41, excluded 71. Phase 5 adds two gap rows, `west/unheaded-syslog-relay` and `west/self` (172 entries, gap 60).
`blocked:FND-001` = Vector is built and configured on west but the stack is down; `blocked:D7` = LAN devices; `gap` = no path at all (east until vector-east, files on both hosts, json-file containers until Phase 4, Pi-hole until Phase 6).

Inventory facts (Steps 13-19, 2026-11-03): west runs 40 services, east 37; files written in the last 7 days outside the journal: west 14 (audit, auth, dmesg, fail2ban, frr, haproxy, kern, nginx x4, syslog, ubuntu-advantage, unattended-upgrades), east 5; haproxy and frr log to syslog (journal) and rsyslog also writes file copies; `systemd-journald-audit.socket` disabled on both (D6: file source); compose: 12 first-party services on journald, 9 third-party on json-file (D5); Pi-hole has pihole.log + FTL.log (webserver.log absent), rsyslog active in the container; LXD runs only `pihole`; LAN neighbours seen from west: router .129, laptop .132, unnamed .148 (D7). east naming drift: the deploy files `unheaded-daemon`, `unheaded-dashboard`, `unheaded-kanban` are installed on east as `unheaded-unheaded-daemon`, `unheaded-dashboard-backend`, `unheaded-kanban-app`.

Privacy (Step 28): the Pi-hole query log names every LAN client and every domain it looks up; the nginx access logs carry client IPs. Personal data under retention-policy-v1; they ship to ClickHouse on west (loopback, one database user) and nowhere else.

## Stuck

- Phase 8 (Steps 179-192): waits on D7, the LAN device list. Stevie's hands.
- Runtime `[V]` steps in Phases 3, 4, 6, 9: wait on the compose stack (E1, attended).
- **Step 132 [STUCK]**: the unit verifies (`systemd-analyze verify`, `check-systemd-units.sh`), but the exposure score is 1.3 and kev-poller's is 1.2, so "≤ the kev-poller unit's" is not met. Step 130 requires `CAP_NET_BIND_SERVICE`; kev-poller has no capability. The same unit with the capability removed scores 1.2, so the whole 0.1 is that capability (`AmbientCapabilities=` and `CapabilityBoundingSet=~CAP_NET_(BIND_SERVICE|BROADCAST|RAW)`), and every other setting is at least as tight. `ProcSubset=pid`, `UMask=0077`, `IPAddressDeny=any` with an allow list and `SocketBindDeny=any` were added to try to cover it. `RootDirectory=` alone still reads 1.3 and cannot be tested without starting a unit as root. The bar was not changed. Nothing depends on it until Step 141, which is Stevie's.

## For Stevie

1. Step 132: keep the capability and accept 1.3, or take socket activation? A `.socket` unit binding 514 needs no capability, which also allows `PrivateUsers=yes` and should score under kev-poller. It needs `LISTEN_FDS` support in the relay and changes Step 130 and ADR-124, so it was not built.
   **Answered 2026-11-09 (#18): socket activation.** `unheaded-syslog-relay.socket` binds 514; the relay reads `LISTEN_FDS` natively; the service has no capability and `PrivateUsers=yes`. Exposure 1.0 (kev-poller 1.2). ADR-124 and Step 130 amended.
2. Phase 6 must change Vector before relayed rows read right (ADR-124, Consequences): `unit` is `unheaded-syslog-relay.service` for every relayed line, so Step 160's `pihole-dns` row will not appear until `journal_rows` takes it from `UNHEADED_SOURCE`; `retention` follows the facility a sender claims (4 or 10 is audit); and `journal_volume` counts a relayed `Failed password for` as an auth failure on west, so any allowlisted sender can raise `AuthFailureBurst`.
   **Answered 2026-11-09 (#18), built:** the relay stamps the row's id, host and retention; Vector takes `unit`, `host` and `retention` from them for relayed lines, so retention is the row's and a device's `Failed password` counts under the device's host (`config/vector/tests/relay.yaml`).
3. Steps 157 and 160 expect new rows `pihole/dns`, `pihole/ftl`, `pihole/web`, `pihole/journal`. The relay reads them as `kind: syslog-device`, `match: "10.10.20.252 <tag>"`. The existing `pihole/*-log` rows are `lxd-file` and do not admit the sender. Step 147's container canary comes before Step 157 adds those rows, so as ordered it is refused as unknown: move the rows ahead of Step 147, or run that one check with `--accept-unknown`.
   **Done 2026-11-09:** the four rows are in `sources.yaml` ahead of Step 147. `pihole/journal` became the bare `10.10.20.252` row (imjournal tags vary per unit), so Step 147's `lh-canary` from the container lands there instead of being refused.
4. Prometheus cannot reach loopback, which is where the plan put the metrics, so `--metrics` repeats and the unit also binds `[fd00:dead:beef::1]:9599`. The relay's `IPAddressAllow` lets in the monitoring network (`fd00:d0c0:e703::/64`), assumed from the compose file and unproved until the stack is up. West's input firewall may also need 9599 from that network: check at Step 141.
5. Port 9599 sits outside the Doom Range, as the plan set it, next to Vector's 9598, so it is not in `pkg/ports` (`TestAllPortsInDoomRange`). It is in `wiki/Port-Registry.md`'s table of ports outside the range.
6. LICH: the plan named `LICH-016`, which is the Lich MCP server's, and 018 is ADR-119's, so the campaign is `tomb/lich/LICH-019-syslog-relay/`, with a row in ADR-062.
7. Appendix A2 promises a per-source rate override, so `rate:` and `burst:` were added to `sources.yaml` rows. A row can only lower the limit.
8. ADR-123 has no row in `docs/adr/ADR-INDEX.md` or `wiki/ADR-Index.md`; left for its owner. No conflict with the relay's fields.
9. No deb for the relay: Step 141 installs by hand, and `scripts/lib/deb-packages.sh` was not touched. Say if it should join the packages.
   **Superseded:** the relay is packaged (`30c5ff2c`); since 2026-11-09 the package also ships the socket unit.
10. **2026-11-10, Step 150 written, not installed.** `deploy/pihole/60-unheaded-forward.conf` departs from the plan's queue line; the reasons and test results are under Step 150. Still yours or attended: Step 153 (`rsyslogd -N1` on the container's 8.2312.0; the tests used 8.2504.0), Step 155 (`usermod -aG pihole syslog`), Step 156 (restart).
11. **2026-11-10, old history (answer #33).** `scripts/pihole-history.sh` (export, transform, load, verify, clean) and `scripts/test-pihole-history.sh` (made-up data, in CI and the meta-gate). Steps 164a-164f run it after Phase 6 is live. Two things to say if you want them otherwise: imported rows carry `source = 'import'`; rows rebuilt from FTL's table are only those older than the oldest file line.
12. What the forward config cannot save: rsyslog stopped while the relay is down loses the batch in hand (2 of 1000 in the test), and a relay restart can lose lines already on the socket. Rerunning `scripts/pihole-history.sh` loads what the relay did not ship.
