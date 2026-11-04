<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-G9-FOLD progress

## Log

- Phase 0 (Steps 1-7), 2026-11-04, branch `sprint/s-g9-fold` from develop `d8fe589d`.
  - baseline failures: 0
  - running units: 3
  - 19015 listeners: 0
  - Step 6: ADR-127 to ADR-134 free (no files match).
- Phase 1 (Steps 8-19), 2026-11-04, Haiku. Eight parked ADRs copied from payloads: ADR-127 to ADR-134 (Proposed, parked).
  - Step 17: index rows added after `[121]` anchor, `ADR-INDEX.md` +8 lines.
  - Step 18: 8 files, 8 index rows, 0 files lacking the parked status line.
  - Commits: 72074e5d (Steps 8-12), fd39d7b6 (Steps 13-19). Both signed.
  - Phase 1 exit gate: preflight then two ADR commits on `develop..HEAD`. Passed.
- Phase 2 (Steps 20-25), 2026-11-04, Sonnet. Step 21 RED as predicted (NoData undefined); Step 23 gofmt clean, vet ok, race test ok; Step 24 DefaultAddr count 0. Commit 1f988660 (signed). Exit gate passed.
- Phase 3 (Steps 26-32), 2026-11-04, Sonnet. Step 27 RED as predicted (undefined checkWireListen, serveWire, wireOptions); Step 29 two ok, gofmt clean; Step 30 all six outputs exact; Step 31 man 1 written, 1 file changed. Commit below. Exit gate passed.
- Phase 4 (Steps 33-37), 2026-11-04, Sonnet. Step 34 RED as predicted (undefined auditDecisionBlock, IngestAuditRecord); Step 35 edits applied; Step 36 gofmt clean, two ok (race). Commit 7a8b38a7 . Exit gate passed.
- Phase 5 (Steps 38-48), 2026-11-04, Opus. Step 38 `MERGED`. Step 39 merged develop `bca6ea7c` (merge 2c643a58, signed); conflicts only in the three files the plan names, both sides kept (ADR rows in number order). Step 42 three `ok`, then `0`. Step 45 no gofmt line, `Finished`. Step 46 under the eBPF load lock: `--- PASS` on TestRingBlockedPacketMovesScore, TestRingRefusedRecordIsCountedNotFatal, TestRingMissingPinIsAnError; `PASS`; `0` pins left. Exit gate passed.
  - The root test goes through `consumeAuditRing` and the score handler: verdicts XDP_PASS and XDP_DROP, Allowed 1, Blocked 1, NoData false, ScorePct 50. It also pins a hash map at the same path first and asserts the refusal.
  - Beyond the plan's text, inside its files: `ConsumePinnedRing` sleeps 1 ms when poll says ready and nothing was taken (poll is level-triggered, so a reserved record would spin it); the refused-record log is bounded to the first and every 4096th, with a total at exit.
  - `compliance-evidence` dependency count is now 421, not the plan's 414: `pkg/ebpf` adds 7.
  - Also run, not asked by Phase 5: `go test -race` on `cmd/compliance-evidence`, `services/compliance`, `pkg/protocol/bpfschema`, and `check-gosec-ratchet.sh`: all pass.
- Phase 6 (Steps 49-60), 2026-11-04, Sonnet. Default off:
  - Step 49 callers of `wire`/`wire --serve` in deploy, docker, compose, nix, scripts, runbooks, Makefile: 0
  - Step 50 deploy/docker/compose/nix files changed: 0
  - Step 51 other destinations touched: 0
  - Step 52 compliance/ and pkg/compliance changed: 0
  - Step 55 branch of ~/tmp/unheaded `develop`; help diff is 1 line, the `wire` line only
  - Step 56 `wire` without --serve prints the off message (exit 0); 19015 listeners 0
  - Step 57 running units 3, equals Phase 0
  - Step 58 PASS check-http-timeouts, check-gosec-ratchet, check-dependency-baseline, check-sbom-trend, gen-man; tree clean after
  - Exit gate passed.

## For Stevie

1. `compliance-evidence` now builds 421 packages, up from the 414 in the plan's list (item 8 there). `pkg/ebpf` is the extra 7.
2. Audit completeness cannot fail once the ring feeds it: every ingested record adds to both the audit count and the packet total, so the ratio is always 1. The honest denominator is `STAT_PACKETS_TOTAL` in the program's STATS map, and a failed ring reservation counts nowhere (`STAT_EVENTS_DROPPED` is unused). Not changed: `services/compliance` scoring and `compliance-ebpf` are outside Phase 5.
3. `--audit-ring` without `--wotan` logs one "event dropped" warning per blocked record through `publishEvent`: unbounded at wire speed. Not changed.
4. If the ring consumer fails (a missing pin, say), `serveWire` prints one stderr line and keeps serving `no_data`. Honest, but quiet; the goroutine is also not joined at shutdown. `wire.go` is outside Phase 5's files.
5. `docs/adr/ADR-INDEX.md` has no row for ADR-123, though the file exists, and its Total line still says 113. Both came from develop, not this plan.
