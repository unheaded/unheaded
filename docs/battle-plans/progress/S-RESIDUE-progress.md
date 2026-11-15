<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-RESIDUE progress

Plan: `docs/battle-plans/S-RESIDUE-battle-plan.md` (10 phases, 176 steps).

## Decisions

| # | Question (short) | Answer | Effect |
|---|---|---|---|
| A-19 | `~/.config/cs` → `~/.config/vor` | Docs and `raft/` now; the prompt text, its goldens and the ADR-024 eval rerun later as a gated phase (Stevie, 2026-11-12) | Phase 1 runs; Phase 8 waits on llama-server |

A-56 (egress rule for llama-server) stays open; Step 157 is blocked on it.

## Step log

| Date | Steps | Note |
|---|---|---|
| 2026-11-15 | 1-16 | Phase 0. Ruff gate was red with the 12 findings the plan lists (`tests/runbooks/test_ioc_audit.py`); fixed by hand, no `noqa`, no exclusion; ruff PASS, 36 tests pass. Go race baseline of the touched packages green. Step 12 as written cannot run: the meta-gate refuses a dirty tree and the untracked `demos/doom/` counts as dirty. The baseline runs in a clean clone instead (same as the CI replay recipe). |
| 2026-11-15 | 17-32 | Lane A (`b38d65c6`). `~/.config/cs` → `~/.config/vor` in 12 files. Champion test ok; the preflight finds `~/.config/vor/sources`. Raft tests 585 passed, 2 skipped, before and after. The leftover list also holds `deploy/systemd/unheaded-vor.service` (correct, it explains the rename) and the threat model's dated closure note (kept, one line added under it). |
| 2026-11-15 | 33-41 | Lane A (`78838e9f`). ADR-INDEX rows 103, 108, 110 follow their files. Accepted 63 → 65, Proposed 41 → 38 (three, not the plan's two), Implemented 2 → 3; the sum, 141, equals the Total. The three status cells in `wiki/ADR-Index.md` follow. |
| 2026-11-15 | 42-60 | Lane A (`7f0a8ee1`). `scripts/timeline-mirrors.py` with 12 tests, written first. `--check` named exactly `timeline.md` lines 91 and 92; `--write` added both (75 milestones). Status and phase lists follow `timeline.md` for every entry; a removed bullet leaves the mirrors on `--write`. One `\u2192` escape in the JSON became a literal arrow (same data). |
| 2026-11-15 | 61-68 | Lane B (`47eb6ffa`). The completion test was red on `hop`, `token`, `generate`, `observe` in bash, zsh and fish, then green under race. zsh and fish are not installed on west, so only the bash script was parsed. |
| 2026-11-15 | 69-80 | Lane B (`73b5a5fa`). The nine one-place text fixes; `bash -n`, nix parse, build and the `pkg/uids` test pass. The `east-bootstrap.sh` scp hint brackets the IPv6 address. The vor row went into both tables of `wiki/Systemd-Units.md`. |
| 2026-11-15 | 81-98 | Lane B (`a599c3fd`). `LAUNCH_EXAMPLES.sh` categories 2, 4, 5 say 4, 1, 4 (total was already 25); `lxd/Makefile` says 25 (line 83 carried the same "30"). circl v1.6.5 is BSD-3-Clause (re-check dated 2026-11-15). Step 89 as written changes nothing (`gen-man.sh --write` rewrites only pages whose body changed); the `zhen-cli` page was taken from a clean build, line 1 only. Gate: doc links, "OK: man/ matches 56 binaries", race tests, `check-systemd-units.sh`, shellcheck. |
| 2026-11-15 | 99-109 | Lane C (`466e3e14`). `compliance_no_data` on `/metrics` and in `Stats()`, cleared on the first record; the first record also recomputes the score (clearing the flag alone would read "evaluated, 0 %" for one window). Drops HELP text fixed. The audit ring failure is on `/metrics` and the consumer is joined. |
| 2026-11-15 | 112-122 | Lane C (`b11b7f9e`). The allowlist test sees variable-topic subscriptions. It newly sees `dashboard-backend`, `kanban-app`, `kanban-app-timeline` and `trace-collector`, all auto-approved, so it passes with no known-gaps list. Phase 4 gate: red then green, race ok, gosec clean, no alert rule names these metrics. |
| 2026-11-15 | 123-136 | Lane C (`be6b08e6`). A queue-only tool is refused before the terminal prompt; a missing slot file is `ErrSlotMissing`; code search stops at its context. The backend method is `Retrieve` through a shared `retrieveShard` (two lines in `pkg/retrieval/corpus.go`, outside the named files); `pkg/retrieval/code_test.go` is new. No Search benchmark exists; none was written. |
| 2026-11-15 | 137-146 | Lane C (`6dd96e6c`). `AppendTurns` run against a throwaway Postgres as `app_zhen`; the test reads `WELL_TEST_APP_DSN`. `test-llm-requests.sh` passed every check with the new leg; docker runs as the normal user, so no sudo was used. Phase 5 gate pass. |
| 2026-11-15 | 147-160 | Lane D (`fc599888`). The grimoire task is gone and a test walks every playbook's `files/` sources. `TRACE_PORT` and `TRACE_LOG_LEVEL` removed from `trace-collector.nix`, with the firewall rule that read the removed `port` option (it opened 9411 for a service that binds loopback 16670). `sophia-eye.nix`: the dead vLLM variable removed; `sophia-gateway` and `embeddings-bge` each run as their own dynamic user with their own `StateDirectory`. Step 157 `[BLOCKED]` on A-56. Phase 6 gate pass. |
| 2026-11-15 | 161-170 | Lane D (`580039ae`). The wiki unit's docker ordering is a repo drop-in; both gate scripts print the west PATH line and keep exit codes 127 and 2; the wiki no longer says `monitoring/.env` needs the Grafana password. The `s-clean-cut` worktree (clean, merged) is removed, its branch kept; the obsolete login script is removed. Steps 163-164 are Stevie's. Phase 7 gate pass. |
| 2026-11-15 | 171-174 | Phase 8 `[GATED]`: llama-server is down (`127.0.0.1:8081/health` does not answer). Not run. |
| 2026-11-15 | 175-176 | Final gate on develop `e6571460` (four lane merges: `65e55858`, `113153d8`, `766f98b8`, `e6571460`): `go build ./...`, the race tests, ruff PASS, `tests/scripts` 93 passed, gen-man OK, mirrors in step (76 milestones with this sprint's entry). Meta-gate `--quick` in a clean worktree at `e6571460`: seven gates are already failing on the clean tree, so the meta-gate reads them INCONCLUSIVE: `check-log-sources`, `check-nosec-ratchet` (suppressions in `cmd/dashboard-backend`, `cmd/logstore-writer`, `pkg/bff/nav.go`, `cmd/wiki-server/main.go`), `check-python-audit` (soupsieve), `check-scrape-publish` (the `wiki-server` job), `check-timeline-freshness` (cleared by this commit's entry), `check-tool-freshness`, `live-path-inventory`. None is in a file this sprint changed except the timeline. Every other gate bites. A run at the pre-sprint commit `88e10c93` follows, to show which were red before; the reds get their own fix lane. |

## Outcome per row

| Row | Outcome |
|---|---|
| D-19 | Docs, scripts and `raft/` fixed. Prompt half **gated** (Phase 8, llama-server). |
| D-26 | Fixed: ADR-INDEX 103, 108, 110; timeline mirrors generated and tested. |
| D-28 | Fixed, with a completion test and the man page check. Zsh and fish scripts not parsed on west. |
| D-29 | Four items fixed. **Dropped:** the package comment (already says "It enforces nothing"). **Conditional, not done:** the allow list and `auto_approve` for `wire` (ADR-127: no Wotan identity until it has its own account). |
| D-30 | All four fixed. |
| D-31 | Grimoire playbook, `trace-collector.nix`, `sophia-eye.nix` fixed. Egress rule for llama-server **blocked** on A-56. |
| D-32 | Ruff red fixed (replaces the dropped I001). **Dropped:** `check-perf-regression` exit 2 (documented). PATH line documented. Wiki drop-in in the repo. Host lines are Stevie's (Steps 163, 164). |

## Parked

- `cmd/shield/main.go` (comment and `Purpose` string) and `services/shield/shield.go` still call Shield a WAF; only the wiki row changed.
- Six more missing `files/` sources: `tomb/ansible/playbook-lich.yml` (4), `playbook-cerydwyn.yml` (2). Held as a shrink-only list in `cmd/zhen-codeindex/tomb_test.go`.
- `tomb/provision.sh` still writes and deploys its own `grimoire-search.sh`.
- `nixos/modules/services/README.md:22,92` still says `:9411`; `docs/architecture/AI_STACK.md:535` still shows the dead vLLM variable.
- Index-versus-file mismatches besides the three: ADR-012, 053, 059, 69420. The `wiki/ADR-Index.md` summary table is stale (132 entries, synced 11-05).
- `wiki/Timeline.md:16` still says timeguru regenerates the mirrors.
- The edited 410 messages still say "cs" picks the file up.
- `compliance_audit_ring_up` has no alert.
- The allowlist test still misses subscriptions with a variable display name (captain, dashboard-backend events, unheaded-daemon, kev-poller) and akira's `context.Background()` call.
- **Inherited red, not this sprint's:** the pip-audit gate fails on `soupsieve==2.10` in `sbom/python-pip-freeze.txt` (CVE-2026-126190, fixed in 4.0.0; CVE-2026-126191 and -126193, fixed in 3.0.2).
- Step 12's "before" meta-gate reading was not taken at the time (dirty tree, see Step log 1-16); it is taken after the fact at `88e10c93` in a clean worktree.
- **Six develop gates red besides the timeline** (list in the Step log, 175-176). Not this sprint's files; fixed in a separate lane.

## For Stevie

- Step 163: compare the host drop-in with the repo's, and at the next reboot check the wiki unit started once.
- Step 164: remove the unread `GRAFANA_ADMIN_PASSWORD` line from `/etc/unheaded/monitoring.env`.
- Phase 8 runs when llama-server is up.
- A-56 (Step 157).

## Stuck

Nothing.
