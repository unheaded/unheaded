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

- Phase 7 (Steps 61-64), 2026-11-04, Haiku. Docs only.
  - Step 61 `g9_docs.py: all edits applied` (UNREACHED-GO-PACKAGES.md, wiki/Compliance-Paperwork.md).
  - Step 62 `live-path-inventory.sh --report` exit 0; `docs/LIVE-PATHS.md` regenerated with no change (`git diff --stat` empty).
  - Step 63 `1` then `1`, as expected.
  - Step 64 commit 735b5839 (signed).
  - Exit gate passed.

- Phase 9 (Steps 72-78), 2026-11-04, Opus. **Exit gate NOT passed. No merge to develop.**
  - Step 72 `Already up to date`: develop `bca6ea7c` is an ancestor of the branch, nothing to merge.
  - Step 73 **[STUCK]**. `go build ./...` rc 0; `go test -race` rc 1; the gate printed `1`, expected `0`. One new package: `unheaded/services/wotan/cmd/wotan`, one test, `TestEveryWotanClientIdentityIsAllowedAndApproved`. The other 59 packages `ok`. Not the known flake: it is a static test, it failed the same way on a `-count=1` rerun, and the same test passes on develop.
    - Cause: Phase 3's `wire.go` calls `wotanClient.InitServiceIdentity("compliance-evidence")`. That test scans `cmd/` and `services/` for the literal and requires the identity in `docker-compose.yml` `AUTH_ALLOW_CALLERS`, `deploy/systemd/unheaded-wotan.service` `AUTH_ALLOW_CALLERS` and `configs/wotan.yaml` `auto_approve`. It is on none of them. Phases 3 to 7 ran narrower packages, so Step 73 is the first run that saw it.
    - Why stuck, and not fixed: the plan contradicts itself here. The scope fence says no unit and no compose change, Step 50's [V] asserts them untouched, and ADR-127 lists Wotan `auto_approve` as not done here. Exempting the identity in the test, hiding the literal from the regex, or dropping `--wotan` would loosen a gate or change the plan's flags. Those are Stevie's calls (For Stevie 6).
    - Measured, nothing kept: on a throwaway `git archive` copy under `$P`, since deleted, adding the three entries turned `./services/wotan/cmd/wotan` `ok`.
  - Step 74 MoatGhost, findings below.
  - Step 75 this entry. Unsigned commits: none; 16 commits and the develop merge are all `G`.
  - Step 76 deviation: the plan's subject says "exit gates green", which is false here, so the commit says what happened. Same file, same helper.
  - Step 77 **[BLOCKED]** on Step 73: rule 15 needs the closing gate to pass. Not run; develop stays `bca6ea7c`.
  - Step 78 **[BLOCKED]** on Step 77: the timer still compiles develop without this code. The growth to 421 packages is unproven on the timer.
  - Phase 8 fence `0`; Step 50 recheck `0`. Steps 65-71 stay [BLOCKED] as planned, unrun.
  - Step 74 findings. Held up: `wire` without `--serve` does nothing (Step 56); `no_data` is set from construction, and score and completeness are 0 until a packet is evaluated; no write path to the evidence file exists in `cmd/compliance-evidence/wire*.go` or `services/compliance`, and the wiki's "adds nothing to the evidence file" is true. `services/compliance`, `cmd/compliance-evidence`, `pkg/ebpf` and `pkg/protocol/bpfschema` are `ok` in Step 73.
    - F1 medium: `--wotan`'s help implies violations leave the host. With auth on, Wotan refuses this identity (Step 73), and `publishEvent` only logs a warning, so the plane keeps scoring while publishing nothing.
    - F2 medium: `audit_completeness` reads 1.0 by construction once the ring feeds it (For Stevie 2). It claims a measurement it cannot make.
    - F3 low: the plane subscribes as `compliance-service`, which is not in `configs/wotan.yaml`. Its four subscriptions would sit in `pending_approval`, the 2026-10-20 bug. `TestShippedAllowlistCoversDeployedSubscribers` does not walk `cmd/compliance-evidence` or `services/compliance`, so it stays green.
    - F4 low: `no_data` is only in the score JSON. `/metrics` `compliance_score_pct 0` and `Stats()` `compliance_score` drop it, so an empty plane reads as a 0 % score unless the reader also checks the packet total. It errs to alarm, not comfort.

- Phase 9, second run (Steps 72-78), 2026-11-04, Opus, on the coordinator's ruling of option (b) for For Stevie 6.
  - Ruling applied first, commit 1479bdf7 (signed), TDD. RED: `TestRunWire_NoWotanFlag` (`runWire(--wotan) = 0, want 2`), `TestPackageHasNoWotanClient` (`wire.go imports the Wotan client`), `TestPublishEventNilWotan_NoPerRecordLog` (1000 records logged 153000 bytes). GREEN: all three, with `services/wotan/cmd/wotan` `ok`.
    - `wire` lost the flag, the client, the identity call and the `wotanAddr` option. `serveWire` passes `nil`. docker-compose.yml, every unit and configs/wotan.yaml are unchanged (`0` on the Step 50 check and on `configs/wotan.yaml`).
    - `compliance-service`'s four subscriptions are not reachable from `wire`. `s.wotan` is set only in `NewService`; the one production caller passes `nil`; `Subscribe`, `StreamMessages` and `listenForAlerts` all sit behind `s.wotan != nil`; `cmd/compliance-evidence` no longer imports the client, and a test holds that. Live, `wire --serve` for 2 s on loopback logged `Wotan not configured, skipping subscriptions` and no subscribe line.
    - `publishEvent` with no client counts the drop and logs nothing. `compliance_wotan_drops_total` still shows it, and `StartBackground` says once that Wotan is not configured.
    - ADR-127 records it under Decision and Not done here.
    - Man page: `gen-man.sh --write` wrote 0. It lists `wire` as a subcommand and never named the flag.
    - Appendix D, corrected here because the plan file is not edited: `compliance-evidence wire` flags are `--serve` (default off), `--listen` (default `127.0.0.1:19015`), `--audit-ring`. There is no `--wotan`.
  - Step 72 merged develop `259a244a` (merge 84d171ea, signed). No conflict. ADR rows are in number order in both indexes.
  - Step 73 **passed**: `go build ./...` rc 0, `go test -race` rc 0, 60 packages `ok`, gate printed `0`.
  - Rechecked on that tree, because the code moved after Phase 6: gofmt `0`, vet ok; Step 58's five `PASS`; Steps 49-52 `0 0 0 0`; running units 3; 19015 listeners 0; `docs/LIVE-PATHS.md` unchanged; `go list -deps` 421, the same as before the ruling; `wire`, `wire --help`, an operand and a non-loopback `--listen` as Step 30, and `--wotan` now rc 2.
  - `check-gates-can-fail.sh --quick` rc 1, none of it this sprint's: `check-netip` FAIL, `check-zhen-mcp` ERROR, `test-deb-maintscripts` INCONCLUSIVE. Each gives the same result with `--only` on a throwaway clone of develop `259a244a`, since deleted. The other fast gates bit. The SLOW gates were not run: no eBPF, compose or unit changed here, and the root ring test (Step 46) was not rerun.
  - `check-trivy-misconfig` rc 1 on both trees: the same 13 lines, 0 new. No `--update`.
  - Step 74 MoatGhost, second pass, below.
  - Step 75 this entry. Unsigned commits: none so far.
  - Step 76 deviation: the plan's subject says "Steps 65-76: exit gates green", which is not true: Steps 65-71 were never run, and the meta-gate is red on develop too. The commit says that instead.
  - Steps 77 and 78 run after this commit. Their results go in the report to the coordinator; an `UNSIGNED` merge would be added here.
  - Step 74 second pass. Held up: `wire` without `--serve` does nothing (message, rc 0, 19015 `0`); `no_data` is set at construction, and an empty plane reads 0, never 100; `cmd/compliance-evidence/wire*.go` and `services/compliance` have `0` write calls and `0` evidence paths, and `compliance/` and `pkg/compliance` moved `0` files, so the wiki's "adds nothing to the evidence file" stands.
    - F1 closed: the flag whose help implied violations leave the host is gone.
    - F3 closed for `wire`: the subscriptions are unreachable. It comes back with any future client, so it stays with item 10 below.
    - F2 and F4 stand: For Stevie 2 and 8.
    - F5 low: the `services/compliance` package comment says it "provides wire-level policy enforcement" and is "ensuring" communication adheres. The plane enforces nothing; it scores what a ring reports. A comment, not a user-facing claim.
    - F6 low: `compliance_wotan_drops_total`'s help says "unavailable Wotan". Under `wire` there is no client by design, so it climbs once per blocked packet and reads like a fault. No alert or dashboard reads it today.
    - F7 low: `no_data` is only recomputed on the 10-second tick, so after the first record the score can read `no_data: true` beside non-zero packet counts for up to one window. It errs towards claiming less.

## For Stevie

1. `compliance-evidence` now builds 421 packages, up from the 414 in the plan's list (item 8 there). `pkg/ebpf` is the extra 7.
2. Audit completeness cannot fail once the ring feeds it: every ingested record adds to both the audit count and the packet total, so the ratio is always 1. The honest denominator is `STAT_PACKETS_TOTAL` in the program's STATS map, and a failed ring reservation counts nowhere (`STAT_EVENTS_DROPPED` is unused). Not changed: `services/compliance` scoring and `compliance-ebpf` are outside Phase 5.
3. Closed by the coordinator's ruling: with no Wotan client `publishEvent` counts the drop and logs nothing.
4. If the ring consumer fails (a missing pin, say), `serveWire` prints one stderr line and keeps serving `no_data`. Honest, but quiet; the goroutine is also not joined at shutdown. `wire.go` is outside Phase 5's files.
5. The ADR-123 row came in from develop. `docs/adr/ADR-INDEX.md`'s Total line and its status table still say 113, with ADRs up to 134 on disk. Not from this plan.
6. Closed: the coordinator ruled (b) on your authority to chain, and Step 73 passes. The options as written, for the record: **Merge blocked: pick one for Step 73.** (a) Allow the identity: add `compliance-evidence` to the two `AUTH_ALLOW_CALLERS` lists and `auto_approve`. Rehearsed green, but it needs your waiver of the fence and Step 50, and it allows an identity that has no account yet. (b) Take `--wotan` out of `wire` until it has its own nologin account (ADR-127), so violations stay local. Not rehearsed; it stays inside the fence but changes Phase 3's flags and Appendix D. Recommendation: (b), the smaller surface, since nothing on the default path uses it and no test in `wire_test.go` or `wire_ring_linux_test.go` mentions Wotan. Then rerun Steps 73 to 78.
7. Closed with item 6: `compliance-service` is unreachable from `wire`.
8. `no_data` on `/metrics` and `Stats()` (Step 74 F4). `services/compliance` scoring is otherwise done; a one-line gauge, not added here.
9. `wire` has no `--wotan` and presents no Wotan identity until it has its own nologin account (ADR-127). Until then violations stay on the host, and the plan's Appendix D is stale on that flag.
10. When `wire` gets that account, it needs three things together: `AUTH_ALLOW_CALLERS` in compose and the unit, `auto_approve`, and `compliance-service` (or a new name) approved for its four subscriptions. `TestShippedAllowlistCoversDeployedSubscribers` does not walk `cmd/compliance-evidence` or `services/compliance`, so add them when that happens.
11. `compliance_wotan_drops_total`'s help text (Step 74 F6) and the package comment's "enforcement" (F5). Both small, not changed here.
12. `no_data` lags the first record by up to one aggregation window (Step 74 F7).
13. `wiki/ADR-Index.md` has no rows for ADR-127 to ADR-134: Phase 1 named only `docs/adr/ADR-INDEX.md`. Not added here.
14. `check-gates-can-fail.sh --quick` is red on develop `259a244a` as well as here: `check-netip` passes while violated, `check-zhen-mcp`'s provocation will not apply, and `test-deb-maintscripts` already fails on a clean tree. Not this sprint's, and not diagnosed.
