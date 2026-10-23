<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-107: Chaos safety envelope (one engine, allowlist, abort, revert journal, SLO steady state)

**Status:** Proposed (Stevie to accept).
**Date:** 2026-10-23
**Sprint:** S-REAL-STEEL Phase 10 (`docs/battle-plans/S-REAL-STEEL-battle-plan.md`, decisions D-16..D-20).
**Related:** ADR-090 (no fake success), ADR-100 (SLOs, burn tiers, missing counts as bad), ADR-098 (findings register), ADR-029 (Akira consensus), ADR-106 (number taken on `sprint/upc-wire-compute`).

## Context

Before this sprint Yaldabaoth could not hurt anything, and it could not be
trusted either: its four built-in injectors only logged and returned nil,
its probes always succeeded, its safety checks never aborted, and
chaos-controller kept a second engine (a rule map) that announced
"injected" on Wotan while nothing reached the kernel. Phase 11 brings real
faults (tc netem, container kill), Phase 12 systemd stop/kill on east,
Phase 13 packet faults. A chaos engine is a weapon; the envelope has to
exist, and be proven to refuse, before the first real fault does.

## Decision

1. **One engine (D-16).** `chaos-controller` builds `yaldabaoth.Service`,
   and every fault runs as an `Experiment` through it: the older
   `/api/v1/chaos/inject` rules and GameDay stages included. No experiment
   type has a built-in injector; a type without a registered injector is
   refused (`no_injector`, HTTP 501), never reported as a success.
   `DryRunMode` is the default; only `CHAOS_DRY_RUN=false` turns it off.

2. **Blast-radius allowlist (D-17).** `/etc/unheaded/chaos/allowlist.yaml`
   (seed `configs/chaos/allowlist.yaml`), parsed strictly (unknown fields
   are errors), is the only source of what may be broken:

   ```yaml
   lxd: {project: unheaded, label: chaos=allowed}   # managed, that project, that label
   netns: '^steel[0-9]+$'                             # lab namespaces
   packet_interfaces: '^steel[0-9]+-h$'               # lab interfaces
   systemd: {host: east, units: [unheaded-timeguru.service]}
   limits: {max_concurrent: 1, max_duration: 10m, max_loss_percent: 50, max_delay: 2s}
   ```

   Targets are resolved, never named: an LXD instance through Cuirass
   (managed flag, project, label, address), a namespace or interface by an
   anchored pattern that may not match a host interface name, a unit from
   the list and only on the named host. A veth is never a target of its
   own (Phase 11 resolves it from an instance). `pihole` and
   `*.target`/`ssh*`/`systemd-*`/`dbus*` are refused even if listed.
   Unknown parameters are refused. The target is checked again right
   before injection. Every refusal is audited, counted
   (`chaos_refusals_total{reason}`) and published on `chaos.refused` at no
   more than 10 a minute.

3. **Abort, three independent paths (D-18).** `POST /api/v1/chaos/abort`
   (auth), Wotan `chaos.abort` (any member Wotan admits; an unparsable
   message still aborts, since an abort only removes chaos), and the file
   `/run/unheaded/chaos/ABORT`, polled every second, which works with Wotan
   and auth both down and refuses new experiments while it exists (the
   unit keeps it across restarts). A stat error on the file counts as
   present.

4. **Revert journal (D-19).** Before an injector acts, its revert recipe
   is written to `CHAOS_STATE_DIR/journal.json` (0600, atomic). The fault
   is reverted at the end, on any abort, by its TTL (duration + 30 s) even
   if the experiment's own loop wedges, at controller start, and by
   `chaos-controller revert-all` from the unit's `ExecStopPost` (runs after
   a crash or SIGKILL). An entry is removed only after the injector verifies
   the fault is gone; otherwise it stays, `chaos.revert_failed` is
   published, the experiment carries a High finding, and `revert-all`
   exits non-zero. A damaged journal stops the controller at start; no
   state dir means nothing runs.

5. **Steady state from ADR-100 (D-20).** Probes are real HTTP GETs to
   allowlisted instances at the address Cuirass reports (loopback,
   link-local, unspecified and multicast refused), plain paths, no
   redirects, no proxy, a timeout; before injection their success ratio
   must reach `min_success_ratio`; during, they abort only below
   `abort_below`. The Kingdom SLO guard (`pkg/sloguard`, shared with
   Sword's canary analysis): every `page: true` SLO's
   `slo:error_ratio:5m` under 14.4 × (1 − objective). Before: a burning or
   missing series, or Victoria not answering, refuses the start. During:
   checked every `SafetyCheckInterval`, it aborts. After the revert: part
   of the verdict. No guard (no `VM_URL`): nothing starts.

## What accepting this means

- No experiment runs outside the allowlist file, and widening it is a host
  edit plus a restart, never a request field.
- No fault is in place without a journal entry written first, and none is
  forgotten: `ExecStopPost` reverts after any stop, and a fault that cannot
  be verified gone keeps alarming.
- An experiment never starts while a Kingdom page SLO burns or its series is
  missing, and stops when one starts burning.
- Experiment types without a real injector say "not implemented".

## Status of the parts

Code and tests on `sprint/real-steel` (each refusal has a bite test: remove
the guard and its test fails). The unit and drop-in are written, not
installed (E2.5 for west, E3 for east). Injectors arrive in Phases 11-13;
until then every experiment type is refused.

## Consequences

- A game day needs Victoria up with the SLO series present (ADR-100 A5).
- `max_concurrent: 1`: experiments run one at a time, GameDay stages too.
- The older src/dst fault rules name no allowlisted target and are refused;
  packet faults will target lab interfaces (Phase 13).
