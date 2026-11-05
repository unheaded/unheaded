<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-127: Fold the wire compliance plane into compliance-evidence

**Status:** Proposed (parked; DP-203's condition: an ADR before any code). The code lands behind `--serve` while this stays Proposed, because Stevie asked for the compliance fold first. Stevie accepts it.
**Date:** 2026-11-04
**Related:** DP-203, ADR-097 (evidence must be able to fail), ADR-098, ADR-103, TRUE-WIRE C1 and E1, `docs/protocol/RFC-EDITOR-REQUESTS-WH.md` item 1.

## The pitch (decision pack, DP-203)

> Fold the eight G9 control planes into existing binaries as flag-gated features (compliance into unheaded-baseline/compliance-evidence ...), one parked ADR per fold before any code; compliance is critical; nothing deleted.

## What is there

`services/compliance`: a long-running HTTP service on 19015 with no auth, Wotan subscriptions, a 10-second loop, policies, zones, violations, an audit list and a score. It touches no BPF map. Nothing calls `RecordAllowed` or `RecordViolation` but its tests, so the score has no input, and an empty service reports **100 % and audit completeness 1.0**.

`ebpf/compliance-ebpf`: `compliance_xdp`, 29,222 verified instructions, loads in the gate, attached by nothing. It reads policy from `SERVICE_POLICIES` and writes 64-byte records to `AUDIT_LOG`. Nothing writes the first or reads the second.

## Which binary

The pack offered two. **compliance-evidence, not unheaded-baseline.**

- unheaded-baseline runs `Type=oneshot` on a 5-minute timer (`TimeoutStartSec=3min`, its own `--timeout` 2 minutes), on every host, with `CAP_DAC_READ_SEARCH`: it can read `/etc/shadow`. A listener in that process is a bad trade, and a daemon does not fit a oneshot. Its unit refuses mount namespacing on purpose, so nothing would contain the listener.
- compliance-evidence already dispatches subcommands, owns the evidence model, and its default path runs as a user timer. A `wire` subcommand leaves that path alone.

Neither is a daemon today. `wire --serve` runs in the foreground; a unit is a later decision, with its own uid.

## Decision

- `compliance-evidence wire`: without `--serve` it prints that it is off and exits 0. No unit, timer or compose service passes `--serve`.
- `--listen` defaults to `127.0.0.1:19015` and refuses anything but loopback unless `AUTH_ENABLED=true`: the plane takes policy writes.
- The service runs behind `pkg/auth` and `pkg/tls`, never its own open listener: new `Handler()` and `StartBackground()`.
- **Honest score.** No packets means `no_data: true`, `score_pct: 0`, `audit_completeness: 0`, never 100.
- `--audit-ring PATH` consumes a pinned `AUDIT_LOG`; `IngestAuditRecord` is the first real input. Record layout mirrored in `bpfschema`.
- **No Wotan client.** `wire` has no `--wotan` and presents no Wotan identity until it has its own nologin account: every service gets one, and no allowlist names `compliance-evidence`. Violations stay on the host. `services/compliance` keeps its Wotan code, but `wire` passes it no client, so its four `compliance-service` subscriptions never run. A dropped event is counted in `compliance_wotan_drops_total`, never logged per record: the ring reaches that path at wire speed.
- The evidence catalog does not change. A new evidence source is a separate decision: it would move every framework's figures.

## Not done here

- Writing `SERVICE_POLICIES` (string service ids against a u8 pair; needs a mapping), attaching `compliance_xdp` anywhere, a unit, a uid, a Wotan identity and its topics on `auto_approve`.
- **East.** compliance-evidence is a `go run` user timer with no deb, so nothing runs it on east.
- **The operator's account.** That timer runs as an account with `sudo -n`. `--serve` there is not much better than a listener in baseline, so the `/etc/shadow` argument only fully holds once `wire` has its own nologin account and unit. Do not run `--serve` from that account beyond a lab test.
- **Who may open the `AUDIT_LOG` pin.** Root only today. `pkg/cartridge/loader_linux.go`'s `EventsGID` is the precedent: the one ring pinned `root:<gid> 0660` for a reader with no right to the other maps.

## Open questions

1. compliance-ebpf reads classification from the low bits of scratch[0]. RFC item 1 may move it. Does classification belong on the wire at all?
2. Should wire results ever enter the evidence file, and under which controls?
3. Who loads `compliance_xdp`, on which interface? Same question as ADR-128's node writer.
