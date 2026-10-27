<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-017: the baseline agent's enforcing path

**Opened:** 2026-10-27 (S-WELL-SPRING Step 279; row reserved in ADR-062 at Step 241).
**ADRs:** 062 (framework), 098 (baseline modes), 099 (remediation as code).
**Activation:** ADR-062 rule #2: `unheaded-baseline` gained a code path that
changes the host as root (enforcing) and an install gate (`preinst`), so a
campaign opens against them before any host runs enforcing.

## Target

`cmd/unheaded-baseline` (`enforce.go`: mode decision, soak, flapping breaker,
state tamper checks), and the `preinst` every Unheaded package carries
(`scripts/lib/deb-maintscripts.sh` `deb_preinst`). Design and flip recipe:
`docs/compliance/enforcing-soak.md`.

## Attack surface

The files the agent trusts or keeps: the baseline (`baseline.yaml`, root-owned),
the mode file (`/etc/unheaded/baseline.conf`), its state
(`/var/lib/unheaded/baseline/report.json`, `soak.json`), the sealed fix snapshot,
and the report as the install gate reads it.

## Harness

```
go test ./tomb/lich/LICH-017-baseline-agent/... -count=1 -v
```

`campaign_test.go` builds the real binary and runs it unprivileged against a
private baseline, a fake `/proc/sys` and a private state directory
(`-trusted-uid` set to the test's uid, `-snapshot ""`): nothing on the host is
read beyond that tree or changed.

| Case | Tamper | Expected |
|---|---|---|
| T1 | baseline 0666, its directory 0777, baseline replaced by a symlink | refused, `CRITICAL ... event=baseline-untrusted`, no report |
| T2 | mode file swapped to `mode=enforcing`; misspellings (`Enforcing`, `enforce`, set twice) | audit, `CRITICAL enforcing requested ... not in effect` or a config error |
| T3 | `report.json` forged between runs (enforcing, 5 critical deviations) | next run: `CRITICAL ... event=state-tamper ... changed outside the agent`, soak restarted, audit |
| T4 | an older `report.json` put back (replay) | same as T3 |
| T5 | `soak.json` back-dated 30 days, deleted, or garbage | `CRITICAL ... event=state-tamper`, soak restarted, audit |
| T6 | no fix snapshot | the soak clock never starts; enforcing never follows |
| T7 | `preinst` fed forged reports (agent format, one-line JSON, unreadable count, override) | enforcing with critical deviations or an unreadable count refuses (fail closed); audit and clean enforcing pass; `UNHEADED_BASELINE_GATE=off` passes |

The breaker (third apply inside 24 h held back), the missing journal directory
and failed applies are covered package-internally:
`cmd/unheaded-baseline/enforce_test.go`.

## Exit criteria

- Every tamper above is a critical event (a `CRITICAL` line in the agent's log,
  and `critical_events` in the report it publishes on `compliance.deviation.<host>`).
- `mode_effective` never reads `enforcing` without the 7-day soak (every case
  asserts it).
- Any case where enforcing takes effect without the soak is CRITICAL and keeps
  every host's mode file at audit until fixed.

## Residual (accepted, recorded)

- Whoever can write the agent's state as its own user can rewrite `report.json`
  and `soak.json` together into a consistent pair and fake the soak's clock. The
  check that catches that is central: the compliance pull compares the agent's
  reports over time with its own evaluation (UH-BASE-01). Enforcing also needs
  the root drop-in and Stevie's flip; the soak is a gate on top of them, not the
  only one.
- A forged audit report lets installs through until the agent's next run (5
  minutes) reports the forgery. That is the state of every host before the flip.

## Re-run trigger

Any change to `cmd/unheaded-baseline` (mode, soak, breaker, report format),
`deb_preinst`, or the executor's journal re-arms this campaign.

## Status

2026-10-27: 7 cases, 0 bypasses (`go test`, west, unprivileged).
