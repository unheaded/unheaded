<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Baseline enforcing: the 7-day soak and the flip

ADR-098 phase 4 and ADR-099 phase 4. Written 2026-10-27 (S-WELL-SPRING Step 279).
Every host's mode file says `mode=audit`. Nothing here changes that; the flip is
Stevie's, per host, after E-E (Q5) and after the soak below.

## What enforcing does

`unheaded-baseline` (`cmd/unheaded-baseline/enforce.go`) evaluates the host's
baseline every 5 minutes. With `mode=enforcing` **in effect**, each deviation
whose check a fix `closes` (`compliance/remediation/fixes.yaml`) is fixed through
the remediation executor: backed up, undoable, journaled in
`/var/lib/unheaded/remediate/journal.jsonl` with the operator
`unheaded-baseline (enforcing)`. Each run logs `ENFORCED host= key= fix= backup=`.
A deviation no fix closes stays audit and counts in `critical_deviations`.

Enforcing is in effect only when all of these hold, checked on every run:

1. `/etc/unheaded/baseline.conf` says exactly `mode=enforcing`.
2. The host's sealed fix snapshot loads (`-snapshot`, default
   `/etc/unheaded/compliance/snapshot.json`, with `-pubkey`): only the fixes of
   the host's profile, signed (ADR-099 "hosts act only on sealed snapshots").
3. The soak: audit runs have had nothing to enforce for 7 days.
4. Nothing in `/var/lib/unheaded/baseline` changed behind the agent's back:
   `report.json` is the one it last wrote, `soak.json` agrees with it.

If any fails, the run is audit and logs `CRITICAL enforcing requested host=...
but not in effect: <why>`.

**Flapping breaker.** A fix applied twice in the last 24 h is not applied a
third time: the run logs `CRITICAL ... event=breaker-open fix=<id>`, the check
reads `enforced: breaker open`, and `unheaded_baseline_breaker_open{fix=}` is 1.
Something keeps undoing that fix; a loop of root changes is worse than the
deviation. The breaker closes when the 24 h window passes. It counts from the
executor's hash-chained journal, so a journal that does not verify stops
enforcing for that fix (`event=enforce-failed`) rather than reading as zero.

**Preinst gate.** Every Unheaded package's `preinst`
(`scripts/lib/deb-maintscripts.sh` `deb_preinst`) refuses an install or upgrade
when `report.json` says `"mode_effective": "enforcing"` and
`"critical_deviations"` above 0. Installs only, the Q5 draft default; if Stevie
answers that out-of-baseline hosts must also refuse to start services, a
follow-on adds an `ExecStartPre` to the units. Override for recovery:
`UNHEADED_BASELINE_GATE=off apt install ...`.

## The soak

The metric is `unheaded_baseline_would_enforce_total`: deviations a fix closes,
summed over audit runs. It must not grow for 7 days on a host before its mode
file changes. The agent keeps the clock itself in
`/var/lib/unheaded/baseline/soak.json` (`clean_since`) and exports it:

| Metric (node-exporter textfile `unheaded_baseline.prom`) | Meaning |
|---|---|
| `unheaded_baseline_would_enforce` | this run's deviations a fix closes |
| `unheaded_baseline_would_enforce_total` | the same, summed over runs (counter) |
| `unheaded_baseline_soak_clean_seconds` | time since the clock (re)started; 0 while something would be enforced |
| `unheaded_baseline_enforcing` | 1 only when enforcing is in effect |
| `unheaded_baseline_enforced_total` | fixes enforcing applied (counter) |
| `unheaded_baseline_critical_events` | this run's critical events |
| `unheaded_baseline_breaker_open{fix}` | fixes the breaker held back this run |

The clock restarts on: a deviation a fix would close, a run without a loadable
snapshot (it cannot judge what enforcing would do), and any state tamper. It
only runs in audit; once enforcing is in effect it is left alone.

Grafana: **Compliance: Baseline Enforcing Soak**
(`monitoring/grafana/dashboards/09_compliance.json`): clean days per host, would
enforce over 7 days (must read 0), deviations, applies, critical events, breaker.

PromQL for the check: `increase(unheaded_baseline_would_enforce_total{instance="<host>"}[7d]) == 0`
and `unheaded_baseline_soak_clean_seconds{instance="<host>"} >= 604800`.

## Before the flip (once per host)

- E-E answered (Q5: installs only, or starts too).
- The host's fixes are journaled (`unheaded-remediate -record-existing` for the
  2026-10-01 lab fixes; Step 280/281), so UH-REMED-01 passes and the breaker has
  a verified journal to count from. `/var/lib/unheaded/remediate` exists; the
  agent never creates it (a fresh directory would hide a deleted journal).
- A sealed snapshot for the host is installed:
  `go run ./cmd/compliance-evidence snapshot -host <h> -key <seed> -out <h>.snapshot.json`,
  then root-owned, not group- or world-writable, at `/etc/unheaded/compliance/snapshot.json`
  next to `snapshot.pub`.
- **Root for applies.** `unheaded-baseline.service` runs as `unheaded-baseline`
  with two read capabilities; the executor refuses to apply without root, so an
  enforcing run without the drop-in below logs `event=enforce-failed ... apply
  needs root` for every fix. The flip adds it.
- The 7 days above.

## The flip (east first)

1. **Root for the applies**: a drop-in
   `/etc/systemd/system/unheaded-baseline.service.d/60-enforcing.conf` with
   `User=root` and `Group=root`. Not written here on purpose: the unit's
   sandbox (`SystemCallFilter=@system-service`, `NoNewPrivileges`, the
   two-capability bounding set) was measured for reading, not for apt, dpkg,
   systemctl and sysctl writes. Measure it on east in the window first: run
   `unheaded-remediate apply <a risk-none fix>` under `systemd-run --wait -p
   User=root` with the unit's other properties, widen only what fails, then
   write the drop-in from what passed.
2. The mode:
   ```bash
   sudo systemctl daemon-reload
   echo 'mode=enforcing' | sudo tee /etc/unheaded/baseline.conf
   ```
   `unheaded-baseline.path` watches the mode file and starts a run within
   seconds; step 1 must come first.
3. Confirm:
   ```bash
   sudo journalctl -u unheaded-baseline -n 30 --no-pager | grep -E 'SUMMARY|ENFORCED|CRITICAL'
   jq '{mode_effective, enforced, critical_deviations, critical_events}' /var/lib/unheaded/baseline/report.json
   ```

## Undo

```bash
echo 'mode=audit' | sudo tee /etc/unheaded/baseline.conf
sudo rm /etc/systemd/system/unheaded-baseline.service.d/60-enforcing.conf
sudo systemctl daemon-reload && sudo systemctl start unheaded-baseline.service
# any fix enforcing applied: its backup directory is in the ENFORCED line
sudo unheaded-remediate -host <h> undo /var/lib/unheaded/remediate/<dir>
```

If the breaker did not trip after three applies of one fix inside 24 h, that is
a P1 bug: stop, back to audit, fix it before any host runs enforcing again
(battle plan Appendix A7).

## Red team

LICH-017 (`tomb/lich/LICH-017-baseline-agent/`, ADR-062) tampers with the
baseline's permissions, swaps the mode file to enforcing, forges and replays
`report.json`, back-dates `soak.json`, and drives the preinst gate. Exit
criterion: every tamper is a critical event and `mode_effective` never reads
enforcing without the soak.
