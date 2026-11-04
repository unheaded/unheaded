<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-123: Uniform log output, columns at the sink, and watching the sink

**Status:** Proposed (pitch, parked under ADR-112)
**Date:** 2026-11-04
**Grounded on:** develop `3295d8da`
**Related:** ADR-117 (ClickHouse is the only log store), ADR-116 (monitor the monitors; R3 is the independent judge), ADR-100 (dead-man's switch), ADR-092 (log shipping), S-LOG-HARVEST (D1, D5, `config/logging/sources.yaml`).

## The pitch

Stevie, 2026-11-04: "Regarding log sink I thought about etl / elt - we don't need Hadoop or anything but maybe a way to break log lines into useful columns like having date, error level, msg, etc pulled - that also brings up the point that all our applications should have uniform log output for easier sorting."

## What exists today

**The sink already has columns.** `config/clickhouse/schema/001-logs.sql` defines `unheaded_logs.logs` with `timestamp`, `host`, `source`, `unit`, `level`, `retention`, `message`, `entry` and `fields` (every other journal field, as JSON). Date, level and message are columns, but they describe the journal entry, not the application's line.

**The `level` column is wrong for our own services.** The `journal_rows` remap in `config/vector/vector.yaml` fills `level` from journald's `PRIORITY`. Our services write JSON to stdout or stderr, and journald records all of that at priority 6 unless a unit opts in. `config/vector/vector.yaml` has no `parse_json`, and nothing under `deploy/` sets `SyslogLevelPrefix`.

Measured on west on 2026-11-04, over three days of journal:

| `level` inside the JSON line | `PRIORITY` in the journal | Lines |
|---|---|---|
| error | 6 | 1,906 |
| info | 6 | 190 |
| warn | 6 | 66 |

Once the stack is up, every one of those rows would land as `info`. A query for errors would miss all 1,906, and the whole JSON object would sit unparsed in `message`.

**Our output is not uniform.**

| Emitter | Where | Shape |
|---|---|---|
| `pkg/logger` | 116 Go files | JSON: `time`, `level`, `message` |
| `log/slog` JSON handler | 25 Go files | JSON: `time`, `level` (upper case), `msg` |
| stdlib `log` | 17 non-test Go files | plain text |
| Rust `tracing` | 34 files | mixed: `zhend` has a JSON mode and a text mode; the rest to be measured per binary |
| Third-party services | nginx, haproxy, Postgres and others | their own formats |

The same field has different names across these, and `service` is present on some lines and absent on others: `unheaded-kev-poller` carries it, `unheaded-trace-collector` does not.

## Decision direction

**ELT, with the tools already on the host.** Load every line, keep its original text, and derive the columns from it. No Hadoop, Spark, Kafka or batch job, and nothing new to install: Vector and ClickHouse already do this.

1. **One log contract.** Every service we write emits one JSON object per line:

   | Key | Rule |
   |---|---|
   | `time` | RFC 3339, UTC |
   | `level` | lower case: `debug`, `info`, `warn`, `error`, `fatal` |
   | `service` | the binary's name, always present |
   | `message` | human text, no interpolated secrets |
   | `error` | present when there is one |
   | anything else | flat keys, kept in `fields` |

2. **Parse at ingest, for our own units only.** A `format` key on each row of `config/logging/sources.yaml` (`json`, `syslog`, `nginx`, `text`) tells Vector how to read that source. For `json`, Vector sets `level`, `service` and `message` from the line, after redaction.

3. **More columns later, without re-shipping.** Add them in ClickHouse as `MATERIALIZED` columns derived from `fields`, so a new column is an `ALTER` and old rows can be derived again. This is the "T after the L".

4. **A gate.** Extend `check-log-sources.sh` so a source with no `format` fails, and add a check that fails when a service's output misses the contract.

## Security

A log line is hostile input. sshd logs attacker-chosen usernames, and a line that looks like JSON must not be able to set its own `level` or `service`, or it could hide from an alert or impersonate a service. That is why parsing is per source and limited to units we build, never a blanket `parse_json`. Redaction (`config/vector/tests/redact.yaml`) must run first, so a secret cannot be promoted into a column. A line that fails to parse keeps its journal values and is counted, never dropped.

## Consequences

- Correcting `level` changes what anything built on it sees. Vector's metrics transform counts by `level`, so error counts jump from near zero to the real figure on the day this lands. Alert thresholds need checking first, and that jump is the true picture rather than a regression.
- The audit `retention` value depends on facility, not `level`, so retention does not move.
- Moving emitters to the contract touches many services. It can go service by service, because unparsed lines still load.

## Watching the sink

Stevie, 2026-11-04: "we must monitor this log sink is working. Services should send a info level hello health check - if one is missing it should alarm/page/alert, that might be a service/application in itself that has a sql read only user and watches that the db is functioning as expected"

**What exists.** `monitoring/prometheus/rules/logs.yml` has `JournalNotShipping`, `LogSilence` and `VectorDown`, and `config/logging/sources.yaml` gives each source a `max_silence`. Those judge from inside the stack, so they share its fate. ADR-116 R3 says absence must be judged by something that does not depend on what it watches. Nothing today reads ClickHouse and asks whether expected rows arrived, and `config/clickhouse/` defines no read-only user.

**Direction.**

1. **Hello.** Every service we write logs one `info` line on a fixed interval, with `event: "hello"` added to the contract. It takes the real path: stdout, journald, Vector, ClickHouse. A row in the table proves the whole pipe works, which a metrics scrape cannot.
2. **A sink watcher.** A small service, on its own nologin account, with a read-only SQL user: `SELECT` on `unheaded_logs.logs` and nothing else, credentials in the environment and never in the repo. On a timer it compares the newest hello per unit with the expected list.
3. **It watches the database too.** It checks that it can connect, that a query returns in a stated time, and that the newest row of any kind is fresh. A failed connection or query is an alarm, never a pass (ADR-100: missing is bad).
4. **What a gap means.** One unit quiet: that service. Every unit on a host quiet: the host or its shipper. Every unit everywhere quiet: Vector or ClickHouse. The alert says which.
5. **The alert path stays out of the sink.** It posts straight to alert-mailer, as east's heartbeat does on 19131, so a dead ClickHouse or Prometheus cannot silence it.
6. **The watcher is watched.** It sends its own dead-man's heartbeat, so a dead watcher pages as well (ADR-100 §4, ADR-116 R3).

**Security.** Match on `unit`, which journald sets, and never on the `service` key in the line, so a forged hello from another process proves nothing. The read-only user gets no access to `system` tables beyond what the checks need, and no `INSERT`, `ALTER` or `DROP`. `message` holds log text, so the watcher reports counts and ages, never line contents.

## Open questions

1. Go: move `log/slog` and stdlib `log` callers onto `pkg/logger`, or settle on `slog` with one shared handler and retire `pkg/logger`?
2. Rust: can `tracing`'s JSON formatter emit these keys directly, or does Vector map them?
3. Keep the original line in a `raw` column, on a shorter TTL, or does `message` plus `fields` carry enough?
4. `level` holds syslog names today (`err`, `crit`, ...). Keep those and map the contract onto them, or normalise everything to the contract's five?
5. Which third-party formats first? Suggested order: nginx, Postgres, haproxy, Pi-hole.
6. Should a CLI's human output follow the contract? Suggested answer: no, only long-running services. ADR-111 governs CLIs.
7. Is the sink watcher its own binary, or a job inside ADR-116's warden? Suggested answer: inside the warden if that gets built, because ADR-115 wants fewer binaries; Stevie's note allows either.
8. Hello interval and allowed misses: suggested 5 minutes and 3 missed before an alert. Should it page or only warn at first?
9. Should the same watcher check the Well, with a read-only Postgres role? The note says "the db", and the Well's health matters more.
10. Where does the expected list live: `config/logging/sources.yaml`, or ADR-116's `config/monitoring/expectations.yaml`? One list, not two.
11. Timers and one-shot jobs do not run on an interval of their own. Do they send hello per run, or stay on `max_silence`?

## Unblocks and depends on

- **Depends on:** the stack being up (E1, S-E1-LIVE), because nothing reaches ClickHouse until then; S-LOG-HARVEST's source inventory; ADR-117's single log store.
- **The watcher depends on:** the log contract above, a ClickHouse read-only user, and alert-mailer's listener.
- **Unblocks:** real error search and error-rate alerts; sorting by service across hosts; ADR-116's "every expected signal" checks on application errors; compliance evidence that can cite a level.
- **Worth doing first:** the `level` fix alone (step 2 for services already on `pkg/logger`) is small and removes the worst defect.

No plan, branch or code is started for this (ADR-112).
