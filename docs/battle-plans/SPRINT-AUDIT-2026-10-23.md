# Sprint audit: plans vs git (2026-10-23, after the session restart)

Marshal re-grounding of the five plans in
[NEXT-5-SPRINTS-2026-10-23.md](NEXT-5-SPRINTS-2026-10-23.md) against what git
shows. Evidence used, per sprint: the plan's step checkboxes, its progress
ledger, step tags in commit messages (all refs since 2026-10-20), and the
existence of the artifacts each ledger claims.

Sprints 1, 2 and 4 kept their ledgers in progress files and never ticked the
plan's checkboxes (0 ticked); Sprints 3 and 5 ticked the plan. Sprint 3 left
its commit-checkpoint steps unticked although the commits exist (dd38f9e2,
5bec7075, 8616aa2c, 2847c8a2, f800c2c1, 5caccdc9, af123df1): paperwork lag,
not missing work.

State of develop at audit time: `8b82f03e`. Sprints 1, 2 and 4 are merged
(4 and 2's earlier work through the Lich merge 21874778, pushed; 2's three
branches merged after it, 3dc9dfb7, unpushed). Merging Lich left seven gates
red on origin/develop (crypto inventory, PQ-KEX, manifest YAML, gosec
ratchet, Well lint, live paths, GPL boundary); fixed in ac647603, 647f484f
and 8b82f03e. Quick meta-gate in a clean clone: every gate bites except
`check-ruff` and `check-python-audit` (tools not installed on west; known).
Full `go test ./...` 290 ok / 0 FAIL; forge `cargo test` 111 passed.

Status key: **done** (exit gate passed, recorded) · **partial** (code half
done, live half waits) · **blocked** (waits on Stevie) · **not started**.

## Sprint 1: Quantum Armor (develop)

| Phase | Status | Evidence |
|---|---|---|
| 0-14 | done | progress/S-QUANTUM-ARMOR-*.md: every phase "done", exit gates recorded; closing summary; artifacts present (pkg/tls/pq.go, cmd/crypto-inventory, CBOM, yggdrasil-evidence, pkg/wotansig, ADR-105) |

Blocked on Stevie: E1-E5, ADR-105 acceptance, aws-lc-rs and
anthropic-sdk-go sign-offs. Housekeeping still open: regenerate ADR-098's
generated findings section.

## Sprint 2: Two Brains (develop, merged 3dc9dfb7)

| Phase | Status | Evidence |
|---|---|---|
| 0-5 | done | S-TWO-BRAINS-progress.md Gates table; step tags on develop cover 29-36 of each phase's steps |
| 6-7 | done | Phase 6/7 gates PASS; Step 254 (first real night) ESCALATE |
| 8-9 | done | P8/P9 Gates PASS; Step 337 (approve 11 runbooks) ESCALATE |
| 10, 11, 13 | done | P10-13 Gates PASS |
| 12 (LoRA learning loop) | blocked | 0 SFT train examples (needs >= 300) and Step 400 ESCALATE |
| 14 (docs, regression, handoff) | done (after this audit) | was not started at audit time; ran the same day: ADR-106 Proposed, ripples, regression 290 ok / 0 FAIL, decisions table (S-TWO-BRAINS-progress.md "Phase 14") |

Blocked on Stevie: live apply of migrations 030-034 as one batch; Step 254;
Step 337; Step 340 (Anthropic's written answer, flag stays off); Step 400;
ADR-019 amendment. Merge note still open: `ThresholdRouter` (Sprint 2) and
`AutoRouter` (Lich) both live in pkg/claudebridge, not reconciled.

## Sprint 3: Real Steel (develop, merged 896a6ed4)

| Phase | Status | Evidence |
|---|---|---|
| 0 sweep | done except Step 38 | 43/44; Step 38 = E1 (the ADR-090 delete table) |
| 1 apply verdicts | blocked | 0/22, needs E1 |
| 2 Wotan client | done | 29/32; open: Steps 91-92 [AFTER-E1], 96 [D] (not needed) |
| 3 replication snapshot | done | 10/10 |
| 4 Cuirass extensions | done | 26/28; open: 132 [D], 134 commit exists (dd38f9e2) |
| 5 Cuirass on real LXD | blocked | 0/24, needs E2 (west live window) |
| 6 steel-echo | **partial** | code half (7/18, images built locally); exit gate Step 177 not passed, live half waits on E2. next.md calls this DONE: overstated |
| 7 deploy core | done | 26/30; the 4 open are commit checkpoints whose commits exist |
| 8 Sword | done except D-09 | 26/28; Step 213 [DECIDE] Sword as a compose service, open; Step 233 [AFTER-E1] |
| 9 deploy live | blocked | 0/22, needs E2 |
| 10 chaos envelope | done | 30/30; ADR-107 Proposed (on the branch only) |
| 11-14 real injectors, game day | blocked | 0; E2, east chaos agent, EVENT_CHAOS spec-vs-code |
| 15-16 review, docs | not started | 0 |

## Sprint 4: Lich Awakens (develop, merged 21874778, pushed)

| Phase | Status | Evidence |
|---|---|---|
| 0-11 | done | references/S-LICH-AWAKENS-progress.md: all COMPLETE; artifacts present (cmd/lich-mcp, pkg/lichtools, LICH-015, LICH-016-lich-mcp-self) |

Phase 5's live local-model end-to-end was DEFERRED (shared llama-server),
not STUCK. CTF mode is hard-gated off by design (ADR-063's rule unmet). The
index still says "Not started": stale.

## Sprint 5: UPC wire compute (develop, merged after 896a6ed4)

| Phase | Status | Evidence |
|---|---|---|
| 0 | done except east | 28/30; Steps 24-25 are the east half (not contacted) |
| 1-5 | done | 20/20 ... 36/36; step tags on develop (came in with the Lich merge) |
| 6-8 Mímir's Law on real L2 | blocked | 0; east as a node (Step 177) |
| 9-12 | done | ticked; tagged on develop |
| 13-17, 19 | done | ticked; commits only on the branch (unmerged) |
| 18 stretch: one UPC across west+east | blocked | 0/14, east escalation (Step 435) |

Blocked on Stevie: east as a node; getpid numbering; ADR-108 acceptance
after the RFC Editor items; DOT4; benchmark gates #1(a)/#2; scorer on real
traffic.

## Drift found

1. **Gates went red on origin/develop** when Lich carried two-brains work
   in, and nobody ran the meta-gate after that merge. Fixed on develop
   (above). Rule for the remaining merges: clean-clone meta-gate after each.
2. **Phase 6 of Real Steel is reported DONE in next.md**; only its code half
   is done.
3. **ADR-106 is missing**: Sprint 2's Phase 14 never ran, so its ADR, the
   ADR-104/019/024/017 updates and the final handoff do not exist.
4. **The index's status column is stale** (Sprint 4 "Not started", Sprint 2
   "not merged").
5. **next.md dates S17c/S17d "2026-10-24"**; the host clock and the commits
   say 2026-10-23.
6. Sprints 1, 2 and 4 never ticked their plan checkboxes; the progress files
   are the ledger. Sprint 3 left commit checkpoints unticked.

Nothing claimed done lacked evidence, apart from item 2.

## Next, in plan order

1. ~~Sprint 2 Phase 14: ADR-106, the ADR ripples, regression, handoff.~~ Done (Steps 435-451).
2. ~~Merge `sprint/real-steel`~~ Done (896a6ed4). The clean-clone
   meta-gate found `check-crypto-inventory` and `live-path-inventory`
   stale; regenerated in 2a48c8dd.
3. ~~Merge `sprint/upc-wire-compute`~~ Done (6 doc conflicts). On
   2a48c8dd: `go test ./...` 295 ok / 0 FAIL, `upc-cartridge` and
   `monad-mbc` cargo tests pass, and every quick gate bites except
   `check-ruff` and `check-python-audit` (known). `cmd/anomaly` is an
   ORPHAN in LIVE-PATHS: nothing supervises it yet.
4. Everything else in Sprints 3 and 5 waits on E1/E2/east.
