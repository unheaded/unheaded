<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-CHAMPION-HARDENING BATTLE PLAN: 16 Phases, 252 Steps

**Date**: 2026-10-27
**Sprint**: Sprint 4 of 7 (index: `NEXT-7-SPRINTS-2026-10-27.md`). The Champion's tool-call path closes the red-team findings, the eleven drafted runbooks get an authorship policy and approvals, the eval stops skipping `runbook-draft`, and the Zhen operational pieces (scheduler, vor, CLI, MCP) are supervised, tested and owned by units.
**Prerequisite**: branch based on develop at `a7fe3acd` or newer; S-CLEAN-SLATE exit gates passed (its M-036 moves the Zhen UI chat path onto `/api/v1/agent/ask`, its M-037 fixes `switch-model.sh`'s fit check, its M-019 flips zhen-agentd's auth/rate defaults; this plan builds on all three and does not redo them); `go build ./...` clean; `~/.venv/zhen` present on west; Well migrations 030-034 applied live is NOT required (every Well-touching step has an in-memory or fake path and a [LIVE-RO] check).
**Target**: a model-emitted tool call cannot pass the trust check with a mislabelled source, cannot replay or borrow a confirmation token, cannot write past a size cap or through a symlink race, and cannot reach a tool the dispatcher does not implement; the eleven runbooks are `approved` under a written authorship rule; `zhen-eval` scores `runbook-draft`; the scheduler, vor, MCP server and CLI each run under a unit or a test, with the coding gate green after every prompt change.
**Estimated Duration**: 26-34 h across 5-7 sessions. Three waits on Stevie (E1 runbook approvals, E2 first unattended night, E3 ADR-019 amendment) and one host window (E4 unit installs on west).
**Agent Strategy**: Coordinator owns Phases 0, 6, 9, 13 (host-touching or Stevie-gated). Phases 1+2+3 (pkg/champion) are [SEQ] with each other but [P:4] with Phase 4 (pkg/zhenrag) and [P:5] with Phase 5 (pkg/claudebridge). Phases 10 and 11 can run in parallel ([P:11]). Each parallel agent works in its own `git worktree` off the sprint branch. Hosts are read-only for every agent unless the step is under an answered [ESCALATE].
**Commit Cadence**: every 5 steps (max(3, min(5, 252/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Every commit ends with the trailers `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE`. Signed if `git commit-tree -S` succeeds in Step 3, otherwise `--no-gpg-sign` (memory: feedback_unsigned_commits_when_afk) for `scripts/resign-unpushed.py` later.
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B dependency map).

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| `docs/sweeps/OUTSTANDING-2026-10-27.md` E09 (M-120..M-132) | The thirteen items, their code paths, the sweep defaults for decisions 30-32. Appendix D maps every M-id to steps. |
| `docs/sweeps/outstanding-2026-10-27/sweep-security.md` RT-01..RT-16 | The red-team findings on the Champion: trust label compare (RT-06), confirm-token binding (RT-03), cleanup (RT-04), write cap (RT-05), listed-not-dispatched tools (RT-09), silent audit failures (RT-10), destructive verbs (RT-07), prompt copies (RT-08), model pins (RT-15), vor provenance (RT-14/16). |
| ADR-019 (Champion agent) + Level 2 amendment (Proposed 2026-10-23) | Rules 1-3, the branch-sandbox sub-mode text this plan asks Stevie to accept or reject (E3). |
| ADR-024 (runbook automation, Accepted) + ADR-106 (Two Brains, Proposed) §Open questions 1-3, 6, 7 | Where the authorship policy is recorded; the approvals (Q3), the unattended night (Q2), the two routers (Q7), the L2 amendment (Q6). Q1 (migrations) belongs to S-WELL-SPRING. |
| `eval/champion-runbooks/PREREG.md` + `RESULTS-2026-10-26.md` | The locked bar (0.85 overall, per-family bars); local models at 0.796. This plan never edits the bar or the results. The re-run trigger for local authorship is "a local model clears the bar". |
| ADR-059 (zhen-cli) Phase 2b/3, ADR-060 (multi-model selector) T19 / LICH-013 / `--json` | The CLI items and the selector tail. |
| ADR-028 (scheduler) and `raft/zhen_scheduler.py` | What the scheduler does today (schedule lib, `pkill -f` from the webapp, no unit). |
| ADR-038 (kanban GUID→commit trail, Accepted) | The trailer hook never built; this plan marks it dormant unless Stevie overrides. |
| memory `project_zhenai_coding_gate` | `make coding-gate-ci` is mandatory after any prompt change. |
| memory `project_no_training_on_claude_outputs` | No step here produces or touches training data; the LoRA stays parked. |
| S21/S22 handoff (`~/tmp/next.md`) | The webapp restart recipe (`~/.venv/zhen`, `ZHEN_DB_PASSWORD` from `.env`), vor with `MemorySwapMax=0`, the Claude leg refusals, `switch-model.sh --stop` then switch. |
| Code read 2026-10-27 at `a7fe3acd` | Every path:line in SITUATION. |

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification step (MUST pass before proceeding)
[D]            Debug step (only if the prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo/elevated privileges required
[P]            Parallelizable with other [P] steps in the same phase
[P:N]          Parallelizable with Phase N
[SEQ]          MUST be sequential
[C]            Commit checkpoint
[ENV]          Environment setup/check
[BUILD]        Build/compilation step
[TEST]         Test execution
[CODE]         Code implementation (test first: red, then green)
[DESIGN]       Design decision recorded in-repo
[STUCK]        Step skipped via Skip Protocol (needs human)
[BLOCKED]      Blocked by an upstream [STUCK] (never used for decisions)
[DECIDE]       Decision with a pre-seeded RECOMMENDATION; proceed without asking
[ESCALATE]     Needs Stevie. STOP this chain; continue independent chains
[AFTER-E1]     Runs only once E1 is answered; skipped (not STUCK) until then
[PREFLIGHT]    Hypothesis verification (Phase 0 only)
[REGEN]        Regenerate derived artifacts
[AUDIT-UPDATE] Mark a finding resolved in its tracking doc
[DOC-UPDATE]   Update downstream docs (wiki, CLAUDE.md, next.md)
[SECURITY]     Security review step
[COMPLIANCE]   License/SBOM/dependency check
[BARE-METAL]   Requires the real hosts (west/east), real kernel, real LXD
[LIVE-RO]      Live read-only verification on west/east (GET, list, show)
[LIVE-RW]      Live host-changing step; only under an answered [ESCALATE]
```

**[DECIDE] format**: RECOMMENDATION, Rationale, "Override ONLY if". The agent proceeds with the recommendation.
**[ESCALATE] format**: Context, the exact question(s), "Why no recommendation" (or why the call is Stevie's even with one). The agent stops that chain only.
**Time tags**: `~Nm` is expected wall-clock. 3x over = Skip Protocol.

**DoD-STD (every phase's Definition of Done includes these; Micromanager gate):**
1. Every step executed or STUCK-logged in this file.
2. Every [V] in the phase passed.
3. Zero NEW test failures versus the KNOWN FAILURES BASELINE (`go test ./...`, plus `-race` on touched packages).
4. Commit checkpoint made; tree clean (`git status --short` empty).
5. No new dependency without a Barrister/ADR-004 check (`go mod tidy -diff` empty, `scripts/check-*` green).
6. Attack surface unchanged, or the change is listed in Phase 14's review table.
7. Every claim of "real" in a commit message has a [LIVE-RO] or [LIVE-RW] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; probes read metadata only.

---

## SITUATION (verified at `a7fe3acd`, 2026-10-27)

- **Trust label** (M-121): `pkg/champion/toolcall.go:187-200` `HasUntrustedJustification` returns true only when a Reference has `SourceTrust == "external"`; any other string (`"External"`, `"externa\x00l"`, `"ext"`, `""` on a non-empty chain) is trusted. The accepted set is documented in the comment above it as `canonical`, `local`, `direct-user`. `confirmcall_fuzz_test.go` fuzzes the canonical-call hash, not the trust check.
- **Confirm tokens** (M-122): `pkg/champion/confirm.go:44-58` `IssuePending` binds a token to a ToolCall only; `confirmcall.go:97` `PendingCall` carries `EmittedBy` but `ConfirmCall(ctx, p, approvedSHA, approvedBy)` at `:106` never compares the confirmer to the requester. `confirm.go:88` `gc()` runs only opportunistically from `:128`; there is no ticker. `write.go:25-72` `WriteFile` has no content size cap (the HTTP body cap is 64 KiB at `cmd/zhen-agentd/main.go:505` and `toolexec.go:103`, so the gap is for in-process callers: agentd's agent loop, MCP, CLI). `champion.go:278-282` `completeAction` discards the store error (`_ = c.store.UpdateAction(...)`); `logAction` at `:262` returns the error but `write.go:28` discards it (`actionID, _ :=`).
- **Listed, not dispatched** (M-122): `toolcall.go:85-87` lists `runbook_show`, `runbook_list` as read-only and `:71` `system_command` as mutating; `dispatch.go:27-160` has cases only for `read_file`, `write_file`, `patch_file`, `kanban_create`, `kanban_update`, `kanban_list`, `run_check`, `runbook_execute`, `model_switch`. `cmd/zhen-cli/main.go:311,325` call `runbook_list`/`runbook_show` through `toolExec`, so they fail at dispatch today. `system_command` has no implementation anywhere and must not get one.
- **Destructive verbs + prompt copies** (M-123): the Go rule is one regexp at `toolcall.go:122-134` (`rm -rf`, `drop table`, `mkfs`, `dd`, `wipe`, `shutdown`, `reboot`, `kill -9`, `truncate`, `unlink`, …); the Python prompt carries a prose list at `raft/scripts/zhen_rag.py:200-215` that names a different set. The system prompt (with the CODE COMMENTS rule) exists three times: `pkg/agent/agent.go:746`, `cmd/zhen-rag/main.go:272`, `raft/scripts/zhen_rag.py:155-176` (`DEFAULT_SYSTEM_PROMPT`). `cmd/zhen-rag/main_test.go:62` `TestCodeCommentExample_MatchesPython` guards one paragraph's byte identity. No runbook body hash is checked before `RunbookExecute` (`write.go:267-330` checks the path only). `scripts/switch-model.sh:62-71` names GGUF files with no sha256; the embedder has no pin.
- **TOCTOU** (M-124): `write.go:31-57` validates the path string, then `os.ReadFile`/`os.MkdirAll`/`os.WriteFile` on `absPath`; a symlink swapped in between passes. `golang.org/x/sys v0.49.0` is already in `go.mod:38` (`unix.Openat2` + `RESOLVE_BENEATH` available; no new dependency).
- **Routers + caps** (M-125): `pkg/claudebridge/router.go:166` `ThresholdRouter`, `autoroute.go:28` `AutoRouter`; both exported, neither feeds the other (ADR-106 Q7). `cmd/zhen-claude/main.go:131,135,144` `-max-tokens`, `-daily-cap-usd` (env `ZHEN_CLAUDE_DAILY_CAP_USD`, default 5), `-local-max-tokens`. ADR-019 L2 amendment at `docs/adr/ADR-019-zhen-champion-agent.md:220-245` is Proposed; code `pkg/champion/branchsandbox.go` exists.
- **Runbooks** (M-120): 79 runbooks; 14 carry `metadata.status`; 11 are `status: draft`, `authored_by: claude`, `approved_by: ""` (list in Appendix C). `scripts/check-runbooks.py:177-203` enforces `status ∈ draft|approved` and the presence of `approved_by` when a status exists. ADR-024 says Accepted (Phase 1, 50 runbooks); nothing records who may author.
- **Eval** (M-127): `cmd/zhen-eval/main.go:307` `continue // runbook-draft: blocked on the Phase 8 harness`; `main_test.go:35` asserts `fam["runbook-draft"] == 0`. The Phase 8 harness exists: `tests/runbooks/harness.py` (dry-run via `scripts/run-runbook.py --dry-run`, sandbox git repo, throwaway Wotan) and `eval/champion-runbooks/` `rb-author` tasks judged by `check` with `validate_runbook.py`. `scripts/run-runbook.py:104` lists `verification` in `find_unset_params` but `execute_steps` (:154) never runs the `verification:` section; 4 runbooks have one (`wotan/active-active-cutover`, `network/air-gap-egress-validation`, `observe/slo-error-budget-burn`, `infra/west-startup-order`).
- **Scheduler** (M-130): `raft/zhen_scheduler.py` (ADR-028) runs by hand (`python3 zhen_scheduler.py`), uses the `schedule` library, is started/killed from the webapp with `pkill -f zhen_scheduler` (`raft/zhen_app.py:880`), writes no Well rows, has no unit, no user, no heartbeat alert.
- **vor** (M-131): `/usr/local/bin/vor` is installed from the separate repo `~/tmp/projects/vor`; no unit, no sysuser, no `pkg/ports` entry, no `pkg/uids` entry; S21 runs it as a `systemd-run --user --scope -p MemorySwapMax=0`. `runbooks/data/conversation-memory-maintenance.yaml` is the consolidation job (manual). `raft/scripts/zhen_rag.py:50` `_strip_tool_call_json` has no unit test. Recalled memories are shown without provenance.
- **MCP** (M-132): `.mcp.json` launches `scripts/zhen-mcp.sh` → `raft/zhen_mcp_server.py` (Python). `cmd/lich-mcp/` is the Go MCP server for the Lich. `raft/tests/test_zhen_mcp_server.py` exists but `.github/workflows/raft-python.yml:55` runs only `test_action_manager.py test_zhen_highlight.py test_zhen_rag_retrieve.py`. `pkg/claudebridge/filter.go:20` names the read-only MCP tools.
- **CLI** (M-129): `cmd/zhen-cli/main.go:285-289` `/recall` `/remember` print a Phase 2b deferral; `:21-31,398` Phase 3 (readline, `/confirm`) deferred; no `/model`. `scripts/switch-model.sh` has no `--json` (ADR-060:99).
- **Kanban** (M-128): `cmd/kanban-app/` has no runbook view and the card modal shows no `branch`/`review_note` although migration 032 (`zhen_agent_runs`) records both. ADR-038 trailer hook: not built. Cards come from The Well (memory: feedback_kanban_cards_from_well); Genesis seed only via migration 034.
- **Users/ports**: `deploy/sysusers.d/unheaded.conf:22` has `zhen-agentd`; `pkg/uids/uids.go` ends its AI block at `OpenWebUI = 16789`; `pkg/ports/ports.go` service block ends near `SophiaEye = 20105` and `UPCCartridgeMetrics = 26610`. zhen-agentd listens on 20105 per its unit (collides with `SophiaEye` in the registry: check in Step 9).

### Escalations

| ID | Question for Stevie | Blocks |
|---|---|---|
| E1 | Approve each of the 11 draft runbooks (Appendix C list) for `status: approved`, `approved_by: stevie`; and confirm the authorship policy wording in Step 127 | Steps 137-141 |
| E2 | First unattended night: which night, the `-daily-cap-usd`, the env file path, the card filter | the live night after Step 236 |
| E3 | ADR-019 Level 2 amendment (branch-sandbox sub-mode): accept or reject | Steps 118-119 |
| E4 | One west host window: install `unheaded-vor` + `zhen-agentd` units/sysusers, restart agentd and the webapp (`[LIVE-RW]`) | Steps 173-175, 184-188 |

---

## PHASE 0: PREFLIGHT (Steps 1-22)

**Goal**: every SITUATION claim re-checked live; baseline recorded; sprint branch and worktrees ready. Cites M-120..M-132.
**Prerequisite**: S-CLEAN-SLATE exit gates passed.
**Time**: 45 min
**Agent**: Coordinator

### Branch and baseline

- [x] **Step 1** [ENV][B] ~1m: Confirm base and S-CLEAN-SLATE
  ```bash
  cd /home/govan/tmp/unheaded && git rev-parse --abbrev-ref HEAD && git log --oneline -1 && git merge-base --is-ancestor a7fe3acd HEAD && echo "base ok" && grep -c 'SHIPPED' docs/battle-plans/S-CLEAN-SLATE-battle-plan.md || echo "S-CLEAN-SLATE not shipped: STOP (prerequisite)"
  ```
- [ ] **Step 2** [ENV][B] ~1m: Create the sprint branch
  ```bash
  cd /home/govan/tmp/unheaded && git status --short | grep -v '^?? cmd/host-agent/\|^?? host-agent$' | wc -l && git checkout -b sprint/champion-hardening
  ```
- [x] **Step 3** [ENV][B] ~1m: GPG probe (decides `-S` vs `--no-gpg-sign` for every [C])
  ```bash
  cd /home/govan/tmp/unheaded && if git commit-tree -S -m probe "$(git rev-parse HEAD^{tree})" >/dev/null 2>&1; then echo "SIGN=yes"; else echo "SIGN=no → use --no-gpg-sign"; fi
  ```
- [x] **Step 4** [BUILD][B] ~3m: Build baseline
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && echo BUILD_OK && go mod tidy -diff | head -3 && echo TIDY_CHECKED
  ```
- [x] **Step 5** [TEST][B] ~8m: KNOWN FAILURES BASELINE (record the tail into the progress file)
  ```bash
  cd /home/govan/tmp/unheaded && mkdir -p docs/battle-plans/progress && go test ./... 2>&1 | grep -E '^(FAIL|ok|---)' | grep -v '^ok' | tee -a docs/battle-plans/progress/S-CHAMPION-HARDENING-progress.md | tail -10; echo "baseline recorded"
  ```
- [ ] **Step 6** [V] ~1m: Baseline is "no FAIL lines" or every FAIL is already listed in `docs/battle-plans/progress/S-CLEAN-SLATE-progress.md`
  - If a FAIL is new → STOP; it belongs to S-CLEAN-SLATE's regression, not here.

### Hypothesis checks (one per SITUATION bullet)

- [x] **Step 7** [PREFLIGHT][B] ~1m: Trust label is a single string compare
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'SourceTrust == "external"' pkg/champion/toolcall.go && ! grep -n 'allowedTrust\|trustAllowlist' pkg/champion/toolcall.go && echo "H1 holds"
  ```
- [x] **Step 8** [PREFLIGHT][B] ~1m: Confirm tokens unbound, no gc ticker, no write cap, audit errors discarded
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'approvedBy string' pkg/champion/confirmcall.go && ! grep -n 'NewTicker' pkg/champion/confirm.go && ! grep -nE 'MaxWriteBytes|maxWrite' pkg/champion/write.go && grep -n '_ = c.store.UpdateAction' pkg/champion/champion.go && grep -n 'actionID, _ := c.logAction' pkg/champion/write.go && echo "H2 holds"
  ```
- [x] **Step 9** [PREFLIGHT][B] ~1m: Listed-not-dispatched tools and the 20105 port question
  ```bash
  cd /home/govan/tmp/unheaded && for t in runbook_show runbook_list system_command; do grep -q "case \"$t\"" pkg/champion/dispatch.go && echo "$t DISPATCHED" || echo "$t not dispatched"; done; grep -n '20105' pkg/ports/ports.go deploy/systemd/zhen-agentd.service
  ```
- [ ] **Step 10** [V] ~1m: All three "not dispatched"; note whether `20105` appears in both files (a registry collision to fix in Step 182)
- [x] **Step 11** [PREFLIGHT][B] ~1m: Three prompt copies, two verb lists, no runbook hash, no GGUF sha
  ```bash
  cd /home/govan/tmp/unheaded && grep -c 'CODE COMMENTS' pkg/agent/agent.go cmd/zhen-rag/main.go raft/scripts/zhen_rag.py && grep -n 'destructivePattern = ' pkg/champion/toolcall.go && grep -n 'DESTRUCTIVE-VERB FILTER' raft/scripts/zhen_rag.py && ! grep -n 'sha256' pkg/champion/write.go && ! grep -nE 'SHA256|sha256' scripts/switch-model.sh && echo "H3 holds"
  ```
- [x] **Step 12** [PREFLIGHT][B] ~1m: TOCTOU window and x/sys availability
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'os.WriteFile(absPath' pkg/champion/write.go && ! grep -n 'Openat2\|RESOLVE_BENEATH' pkg/champion/write.go && grep -n 'golang.org/x/sys' go.mod && go doc golang.org/x/sys/unix.Openat2 | head -2
  ```
- [ ] **Step 13** [PREFLIGHT][B] ~1m: Runbook status fields and the eval skip
  ```bash
  cd /home/govan/tmp/unheaded && grep -rlE '^\s+status: draft' runbooks --include=*.yaml | wc -l && grep -rhoE '^\s+(authored_by|approved_by):.*' runbooks --include=*.yaml | sort | uniq -c && grep -n 'runbook-draft: blocked' cmd/zhen-eval/main.go && grep -n 'fam\["runbook-draft"\] != 0' cmd/zhen-eval/main_test.go
  ```
- [ ] **Step 14** [V] ~1m: 11 drafts, `authored_by: claude` x11, `approved_by: ""` x11, both eval lines present
- [ ] **Step 15** [PREFLIGHT][B] ~1m: Scheduler, vor, MCP, CLI state
  ```bash
  cd /home/govan/tmp/unheaded && grep -n "pkill', '-f', 'zhen_scheduler" raft/zhen_app.py && ! ls deploy/systemd/*vor* deploy/systemd/*scheduler* 2>/dev/null && grep -c 'zhen_mcp_server' scripts/zhen-mcp.sh && grep -n 'test_zhen_mcp_server' .github/workflows/raft-python.yml || echo "mcp test NOT in CI" && grep -n 'Phase 2b — requires DB plumbing' cmd/zhen-cli/main.go && grep -n -- '--json' scripts/switch-model.sh || echo "switch-model has no --json"
  ```
- [ ] **Step 16** [PREFLIGHT][B] ~1m: run-runbook.py never executes `verification:`
  ```bash
  cd /home/govan/tmp/unheaded && grep -n '"verification"' scripts/run-runbook.py && ! grep -nE 'runbook\.get\("verification"|runbook\["verification"\]' scripts/run-runbook.py && grep -rlE '^\s*verification:' runbooks --include=*.yaml
  ```
- [ ] **Step 17** [PREFLIGHT][LIVE-RO][B] ~2m: What is running on west right now (read-only)
  ```bash
  systemctl is-active zhen-agentd 2>/dev/null || echo "zhen-agentd: not a unit here"; systemctl --user list-units --type=scope 2>/dev/null | grep -i vor || echo "vor: no user scope"; pgrep -af 'zhen_scheduler|zhen_app.py|llama-server' | cut -c1-120 || echo "none running"; curl -s --max-time 3 http://127.0.0.1:20105/health | head -c 200; echo
  ```
- [ ] **Step 18** [V] ~1m: Every H1-H3 and the Step 13-16 checks hold. If any does NOT hold, the corresponding phase shrinks: strike its [CODE] steps in the progress file with "already landed in <commit>" and keep its [V] steps.

### Worktrees and tooling

- [ ] **Step 19** [ENV][B] ~2m: Worktrees for the parallel agents (Phases 4, 5, 10, 11)
  ```bash
  cd /home/govan/tmp/unheaded && for w in zhenrag claudebridge mcp cli; do git worktree add -b sprint/champion-hardening-$w ../unheaded-wt-$w sprint/champion-hardening 2>&1 | tail -1; done && git worktree list
  ```
- [x] **Step 20** [ENV][B] ~2m: Python venv for the runbook tests and the coding gate
  ```bash
  cd /home/govan/tmp/unheaded && test -x ~/.venv/zhen/bin/python3 && ~/.venv/zhen/bin/python3 -m pytest --version && ~/.venv/zhen/bin/python3 -c 'import schedule, yaml; print("deps ok")'
  ```
- [x] **Step 21** [ENV][B] ~1m: Coding-gate prerequisites (used after every prompt change)
  ```bash
  cd /home/govan/tmp/unheaded && make -n coding-gate-ci | head -3 && ls scripts/coding-gate-ci.sh scripts/probe-sanitization-check.sh && ls eval/coding-gate | head -3
  ```
- [ ] **Step 22** [V] ~1m: **PHASE 0 EXIT GATE** — Steps 6, 10, 14, 18 passed; worktrees listed; venv ok.
  - If pass → Phase 1. Commit: `git add -A docs/battle-plans/progress && git commit -m "[PLAN S-CHAMPION-HARDENING] Steps 1-22: preflight, baseline recorded"` (add `--no-gpg-sign` if Step 3 said SIGN=no; same for every [C] below).
  - If fail → fix within Phase 0; do not enter Phase 1.

---

## PHASE 1: TRUST-LABEL ALLOWLIST AND FUZZ (Steps 23-40)

**Goal**: `HasUntrustedJustification` accepts exactly `canonical`, `local`, `direct-user` and treats everything else as untrusted; the vor retrieval chain is verifiable; a Go fuzz target covers the check. Cites M-121 (RT-06).
**Prerequisite**: Phase 0.
**Time**: 1.5 h
**Agent**: Agent A (pkg/champion), [SEQ] with Phases 2-3, [P:4] [P:5]

- [x] **Step 23** [R] ~2m: Read the current check and its tests
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 150,205p pkg/champion/toolcall.go && grep -n 'SourceTrust' pkg/champion/toolcall_test.go | head
  ```
- [x] **Step 24** [CODE][W] ~5m: Red test — table of labels that must be untrusted
  Append to `pkg/champion/toolcall_test.go`:
  ```go
  func TestHasUntrustedJustification_ExactAllowlist(t *testing.T) {
  	bad := []string{"external", "External", "EXTERNAL", "externa\x00l", "ext", "", " canonical", "canonical ", "canonical\n", "Canonical", "direct_user", "directuser", "local;", "unknown"}
  	for _, s := range bad {
  		tc := ToolCall{Name: "write_file", Args: map[string]any{"path": "/x"}, Justification: []Reference{{SourceTrust: s}}}
  		if !tc.HasUntrustedJustification() {
  			t.Errorf("label %q must be untrusted", s)
  		}
  	}
  	for _, s := range []string{"canonical", "local", "direct-user"} {
  		tc := ToolCall{Name: "write_file", Args: map[string]any{"path": "/x"}, Justification: []Reference{{SourceTrust: s}}}
  		if tc.HasUntrustedJustification() {
  			t.Errorf("label %q must be trusted", s)
  		}
  	}
  	// one bad reference anywhere in the chain poisons the call
  	tc := ToolCall{Name: "write_file", Args: map[string]any{"path": "/x"}, Justification: []Reference{{SourceTrust: "canonical"}, {SourceTrust: "Canonical"}}}
  	if !tc.HasUntrustedJustification() {
  		t.Error("mixed chain must be untrusted")
  	}
  }
  ```
- [x] **Step 25** [TEST][B] ~1m: Expect FAIL
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run TestHasUntrustedJustification_ExactAllowlist -count=1 2>&1 | tail -5
  ```
- [x] **Step 26** [V] ~1m: Test FAILS on at least `"External"` and `"ext"` (proves H1). If it passes → the fix already landed; mark Steps 27-28 done and continue.
- [x] **Step 27** [CODE][W] ~5m: Green — exact allowlist
  In `pkg/champion/toolcall.go`, above `HasUntrustedJustification`, add:
  ```go
  // TrustedSourceLabels is the closed set of SourceTrust values a
  // justification may carry. Anything else, including case or whitespace
  // variants, is untrusted (RT-06: the old check compared only against
  // "external", so a typo passed).
  var TrustedSourceLabels = map[string]bool{"canonical": true, "local": true, "direct-user": true}
  ```
  and replace the loop body with:
  ```go
  	for _, r := range tc.Justification {
  		if !TrustedSourceLabels[r.SourceTrust] {
  			return true
  		}
  	}
  	return false
  ```
- [x] **Step 28** [TEST][B] ~2m: Expect PASS, then the whole package with race
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run TestHasUntrustedJustification -count=1 && go test ./pkg/champion/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 29** [V] ~1m: PASS; no other champion test regressed (a test that asserted a non-listed label was trusted must be fixed to use a listed one, and that change recorded in the progress file).
- [x] **Step 30** [CODE][W] ~5m: Fuzz target for the trust check
  Create `pkg/champion/trust_fuzz_test.go`:
  ```go
  package champion

  import "testing"

  // FuzzTrustLabel: no byte string other than the three listed labels may
  // be trusted; the three listed labels are always trusted.
  func FuzzTrustLabel(f *testing.F) {
  	for _, s := range []string{"canonical", "local", "direct-user", "external", "", "Canonical", "local\x00"} {
  		f.Add(s)
  	}
  	f.Fuzz(func(t *testing.T, label string) {
  		tc := ToolCall{Name: "write_file", Args: map[string]any{"path": "/x"}, Justification: []Reference{{SourceTrust: label}}}
  		got := tc.HasUntrustedJustification()
  		want := !TrustedSourceLabels[label]
  		if got != want {
  			t.Fatalf("label %q: untrusted=%v want %v", label, got, want)
  		}
  	})
  }
  ```
- [x] **Step 31** [TEST][B] ~2m: Fuzz 60 s
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run '^$' -fuzz FuzzTrustLabel -fuzztime 60s 2>&1 | tail -3
  ```
- [x] **Step 32** [V] ~1m: No failing input. If one is found → it is a real bug in Step 27's map lookup (impossible by construction) or the seed corpus; fix, re-run.
- [x] **Step 33** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 23-32: exact trust-label allowlist + fuzz (RT-06)`

### vor chain verifier

- [x] **Step 34** [R] ~3m: How a retrieval reference reaches `Justification` today
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn 'SourceTrust:' --include=*.go pkg cmd | grep -v _test | head -12 && grep -n 'func.*Reference' pkg/champion/toolcall.go | head
  ```
- [x] **Step 35** [DESIGN][W] ~5m: Record the verifier contract in `pkg/champion/toolcall.go` doc comment: a `Reference` with `SourceTrust: "local"` MUST carry `Topic` (the vor topic id) and `Digest` (sha256 of the retrieved text, hex); `VerifyChain(refs, lookup func(topic string) (text string, ok bool)) error` recomputes digests. Add the two fields to `Reference` if absent (check Step 34 output; `json:"topic"` already exists per toolexec.go:49).
- [x] **Step 36** [CODE][W] ~8m: Red test `TestVerifyChain` (digest mismatch → error; missing topic → error; ok → nil) appended to `pkg/champion/toolcall_test.go`; then green: implement `VerifyChain` in a new `pkg/champion/chain.go` (stdlib `crypto/sha256`, `encoding/hex`; no new deps).
- [x] **Step 37** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run 'TestVerifyChain' -count=1 -race 2>&1 | tail -3
  ```
- [x] **Step 38** [CODE][W] ~6m: Call `VerifyChain` from the agent loop where references are attached (`pkg/agent/agent.go`, the site found in Step 34) with a lookup that re-reads the vor result cached for the turn; on error, downgrade the reference to `SourceTrust: "external"` (not drop: the gate then refuses the mutating call for the right reason) and log once at WARN. Add a test in `pkg/agent` with a fake lookup.
- [x] **Step 39** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/agent/... ./pkg/champion/... -race -count=1 2>&1 | tail -4 && go vet ./pkg/agent/... ./pkg/champion/...
  ```
- [x] **Step 40** [V] ~1m: **PHASE 1 EXIT GATE** — Steps 29, 32, 37, 39 green; `gofmt -l pkg/champion pkg/agent` empty. Commit `[PLAN S-CHAMPION-HARDENING] Steps 34-40: vor chain verifier`.

---

## PHASE 2: CONFIRM TOKENS, GC, WRITE CAP, DISPATCH TRUTH, AUDIT FAILURES (Steps 41-66)

**Goal**: a confirmation token is bound to the principal that requested it and consumed only by a matching confirmer; expired tokens are collected on a ticker; in-process writes are capped; every listed tool is either dispatched or removed from the lists; audit-store failures are surfaced. Cites M-122 (RT-03, RT-04, RT-05, RT-09, RT-10).
**Prerequisite**: Phase 1.
**Time**: 2.5 h
**Agent**: Agent A

### Requester binding (RT-03)

- [x] **Step 41** [R] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 40,60p pkg/champion/confirmcall.go && sed -n 100,135p pkg/champion/confirmcall.go && grep -n 'EmittedBy' cmd/zhen-agentd/*.go pkg/claudebridge/confirm.go | grep -v _test | head
  ```
- [x] **Step 42** [CODE][W] ~6m: Red test in `pkg/champion/confirmcall_test.go`: a `PendingCall` issued with `EmittedBy: "agent:zhen-7b"` and requester `Principal: "ui:stevie"` is refused by `ConfirmCall` when `approvedBy` is a different principal class ("agent:…" may never confirm; "ui:…"/"tty:…"/"queue:stevie" may confirm only if it equals the recorded requester or is `queue:stevie`).
- [x] **Step 43** [CODE][W] ~8m: Green: add `Requester string` to `PendingCall` (set from a new `ToolCall.Principal` field populated by agentd from the authenticated caller, `direct-user` paths use `"ui:"+subject`), add `confirmerAllowed(requester, approvedBy string) bool`, enforce in `ConfirmCall` before `confirmedGate`. Include `Requester` in `CanonicalCall`'s hashed bytes so a token cannot be re-bound.
- [x] **Step 44** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run 'Confirm' -race -count=1 2>&1 | tail -3 && go test ./pkg/champion/ -run '^$' -fuzz FuzzCanonicalCall -fuzztime 30s 2>&1 | tail -2
  ```
- [x] **Step 45** [V] ~1m: PASS and the existing canonical-call fuzz still passes (hash input changed; seeds may need `Requester`).
- [x] **Step 46** [CODE][W] ~6m: Plumb the principal in `cmd/zhen-agentd` (`/api/v1/agent/ask`, `/api/v1/tool/exec`, `/api/v1/agent/confirm`): principal = `"ui:"+auth subject` when AUTH_ENABLED, else `"ui:anonymous"` and the confirm endpoint refuses with 403 `confirmation requires an authenticated principal`. Test in `cmd/zhen-agentd/*_test.go` with the existing fake Champion.
- [x] **Step 47** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-agentd/... -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 48** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 41-47: confirm tokens bound to requester (RT-03)`

### GC ticker (RT-04) and write cap (RT-05)

- [x] **Step 49** [CODE][W] ~5m: Red: `TestConfirmStore_GCTicker` (issue 3 tokens with a 10 ms TTL, start the store with `StartGC(ctx, 5*time.Millisecond)`, after 50 ms `len(entries)==0`). Green: `StartGC` goroutine with `time.NewTicker`, stopped by ctx; `Champion.Run(ctx)` or constructor option starts it; agentd passes its root ctx.
- [x] **Step 50** [CODE][W] ~5m: Red: `TestWriteFile_RejectsOversize` (content of `MaxWriteBytes+1` → `ErrWriteTooLarge`, nothing written, action logged `failed`). Green: `const MaxWriteBytes = 1 << 20` (1 MiB, matches the largest file the agent is allowed to produce; [DECIDE] below), checked first in `WriteFile` and `PatchFile` (post-patch length).
  - [DECIDE] RECOMMENDATION: 1 MiB. Rationale: the HTTP cap is 64 KiB, so the in-process cap only needs to stop runaway generation; 1 MiB keeps vendored fixtures writable. Override ONLY if a runbook legitimately writes larger files (none does: `grep -rn write_file runbooks | wc -l`).
- [x] **Step 51** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run 'GCTicker|Oversize' -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 52** [V] ~1m: PASS; `go test ./pkg/champion/... -race -count=1` still green.

### Dispatch truth (RT-09)

- [x] **Step 53** [DECIDE][DESIGN] ~3m: `system_command`
  - RECOMMENDATION: remove `system_command` from `MutatingTools` and from every tool list/prompt (`grep -rn system_command pkg cmd raft`); the dispatcher returns `GateError{Reason: "unknown tool"}` for it as for any unknown name (fail-closed already). Rationale: ADR-019 Rules 1-3 have no model for arbitrary shell; `run_check` (allowlisted checks) and `runbook_execute` (reviewed YAML) are the sanctioned paths. Override ONLY if Stevie wants a shell tool, which needs its own ADR.
- [x] **Step 54** [CODE][W] ~8m: Red: `TestDispatch_RunbookListShow` (list returns the 79 names from `runbooks/`, show returns one file's YAML text, both read-only, path escape refused as in `RunbookExecute`). Green: `dispatch.go` cases `runbook_list` and `runbook_show` calling new `Champion.RunbookList(ctx)` / `RunbookShow(ctx, name)` in `write.go` (reuse the `absRunbooks` escape check at `:281-298`).
- [x] **Step 55** [CODE][W] ~3m: Remove `system_command` per Step 53; add `TestToolLists_EveryListedToolDispatches`: for each name in `MutatingTools ∪ ReadOnlyTools` minus the MCP-only names (`file_read`, `corpus_search`, `service_health`, `local_llm`, `lich_*`), `Dispatch` with empty args must NOT return "unknown tool".
- [x] **Step 56** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run 'Dispatch|ToolLists' -race -count=1 2>&1 | tail -3 && grep -rn 'system_command' pkg cmd raft --include=*.go --include=*.py | grep -v _test | grep -v 'test_memory_poison' || echo "system_command gone from code"
  ```
- [x] **Step 57** [V] ~1m: PASS; `raft/tests/test_memory_poison.py:104` still uses `system_command` as a poison payload (that is correct: it must be refused as unknown). Run it:
  ```bash
  cd /home/govan/tmp/unheaded && ~/.venv/zhen/bin/python3 -m pytest raft/tests/test_memory_poison.py -q 2>&1 | tail -2
  ```
- [x] **Step 58** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 49-57: gc ticker, write cap, runbook_list/show dispatched, system_command removed (RT-04/05/09)`

### Audit failures surfaced (RT-10)

- [x] **Step 59** [R] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 255,290p pkg/champion/champion.go && grep -n 'logAction(' pkg/champion/*.go | grep -v _test | grep -c '_ :='
  ```
- [x] **Step 60** [DECIDE][DESIGN] ~2m: Audit-write failure policy
  - RECOMMENDATION: fail-closed for mutating tools (if `logAction` errors, the mutation does not run and the caller gets `ErrAuditUnavailable`), fail-open with a counter for read-only tools. Rationale: ADR-019 makes the action log the review surface; a write with no record is the thing the gate exists to prevent. Override ONLY if the pg store proves flaky in Phase 13's night (then add a bounded local spool, not fail-open).
- [x] **Step 61** [CODE][W] ~8m: Red: `TestWriteFile_AuditStoreDown` (a store whose `LogAction` errors → `WriteFile` returns `ErrAuditUnavailable`, file untouched) and `TestReadFile_AuditStoreDown` (read succeeds, `champion_audit_write_failures_total` increments). Green: replace every `actionID, _ := c.logAction` in mutating paths (`write.go`, `champion.go` kanban/runbook/model paths) with error handling; `completeAction` logs the error and increments the same counter; export the counter through the existing metrics registry (`grep -rn 'prometheus.NewCounter\|NewServiceMetrics' pkg/champion cmd/zhen-agentd` to find the pattern).
- [x] **Step 62** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/... ./cmd/zhen-agentd/... -race -count=1 2>&1 | tail -4
  ```
- [x] **Step 63** [V] ~1m: PASS.
- [x] **Step 64** [TEST][B] ~3m: Integration test still green
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run Integration -count=1 2>&1 | tail -3
  ```
- [x] **Step 65** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 59-64: audit-write failures fail closed for mutations (RT-10)`
- [x] **Step 66** [V] ~1m: **PHASE 2 EXIT GATE** — Steps 45, 47, 52, 57, 63, 64 green; `go vet ./pkg/champion/... ./cmd/zhen-agentd/...` clean.

---

## PHASE 3: SANDBOX WRITE WITHOUT THE RACE (Steps 67-80)

**Goal**: `WriteFile`/`PatchFile` open the target through `openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS)` relative to the sandbox root, so a symlink swapped in after validation cannot redirect the write. Cites M-124 (ADR T-05).
**Prerequisite**: Phase 2.
**Time**: 1.5 h
**Agent**: Agent A

- [x] **Step 67** [R] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'func (c \*Champion) validatePath' -A30 pkg/champion/champion.go | head -40 && go doc golang.org/x/sys/unix.OpenHow
  ```
- [x] **Step 68** [CODE][W] ~8m: Red: `TestWriteFile_SymlinkRaceRefused` in `pkg/champion/write_test.go`: sandbox root `t.TempDir()`, allowed path `root/a/b.txt`; after `validatePath` would pass, replace `root/a` with a symlink to another `t.TempDir()`; `WriteFile` must return an error containing `symlink` and the outside dir must stay empty. Use a test hook `c.beforeOpen func()` (unexported) to inject the swap between validation and open.
- [x] **Step 69** [TEST][B] ~1m: Expect FAIL (the write lands outside)
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run TestWriteFile_SymlinkRaceRefused -count=1 2>&1 | tail -4
  ```
- [x] **Step 70** [V] ~1m: FAILS (proves the window). If it passes → re-check the test injects after validation.
- [x] **Step 71** [CODE][W] ~10m: Green: new `pkg/champion/safeopen_linux.go` with `openBeneath(rootFD int, rel string, flags int, mode uint32) (*os.File, error)` using `unix.Openat2(rootFD, rel, &unix.OpenHow{Flags: flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, Mode: mode, Resolve: unix.RESOLVE_BENEATH|unix.RESOLVE_NO_SYMLINKS|unix.RESOLVE_NO_MAGICLINKS})`; `Champion` holds an `O_PATH|O_DIRECTORY` fd of `config.ProjectRoot` opened once; `WriteFile`/`PatchFile`/`RevertAction` write through `openBeneath` + `f.Truncate(0)` + `f.Write`, creating parent dirs with `unix.Mkdirat` on the root fd. Keep `validatePath` (blocklist and prefix rules still apply). Build tag `linux`; a `safeopen_other.go` returns `errors.ErrUnsupported` so non-Linux builds compile.
- [x] **Step 72** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run 'WriteFile|PatchFile|Revert' -race -count=1 2>&1 | tail -4
  ```
- [x] **Step 73** [V] ~1m: PASS incl. Step 68's test.
- [x] **Step 74** [D] ~5m: If `Openat2` returns `ENOSYS` (kernel < 5.6) in a test env → skip the race test with `t.Skip` on `ENOSYS` only; west and east run 6.17.
- [x] **Step 75** [TEST][B] ~2m: Branch-sandbox tests still pass (they write inside worktrees)
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run 'Branch' -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 76** [SECURITY][B] ~2m: gosec on the package (the `#nosec G304/G703` comments at write.go:44,57 must be updated to cite openat2)
  ```bash
  cd /home/govan/tmp/unheaded && ./tools/gosec -quiet ./pkg/champion/... 2>&1 | tail -5 || scripts/check-gosec-ratchet.sh 2>&1 | tail -3
  ```
- [x] **Step 77** [V] ~1m: No new gosec finding; the ratchet file did not grow.
- [x] **Step 78** [TEST][B] ~3m: Full package + agentd
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/... ./cmd/zhen-agentd/... -race -count=1 2>&1 | tail -4
  ```
- [x] **Step 79** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 67-78: sandbox writes through openat2 RESOLVE_BENEATH (ADR T-05)`
- [x] **Step 80** [V] ~1m: **PHASE 3 EXIT GATE** — Steps 73, 77, 78 green.

---

## PHASE 4: ONE PROMPT, ONE VERB LIST, PINNED MODELS (Steps 81-110)

**Goal**: the Zhen system prompt and the destructive-verb rule live once in `pkg/zhenrag`, consumed by `pkg/agent`, `cmd/zhen-rag` and (as a generated artifact) `raft/scripts/zhen_rag.py`, guarded by byte-identity tests; a runbook's body is hash-checked before execution; GGUF and embedder files are pinned by sha256 and verified by `switch-model.sh`. Cites M-123 (RT-07, RT-08, RT-15).
**Prerequisite**: Phase 0 (worktree `../unheaded-wt-zhenrag`). [P:1] [P:2] [P:3] [P:5]
**Time**: 3 h
**Agent**: Agent B (worktree zhenrag)

### Shared prompt package

- [x] **Step 81** [R] ~5m: Diff the three prompt copies
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 155,230p raft/scripts/zhen_rag.py > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/p_py.txt && sed -n 730,800p pkg/agent/agent.go > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/p_agent.txt && sed -n 255,320p cmd/zhen-rag/main.go > /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/p_rag.txt && wc -l /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/p_*.txt
  ```
- [x] **Step 82** [DESIGN][W] ~5m: `pkg/zhenrag/doc.go`: the package owns `SystemPrompt(mode Mode) string` (modes: `Chat`, `Agent` (adds the JSON tool-call shape), `Review`), `DestructivePattern *regexp.Regexp` (moved from champion), `DestructiveVerbsProse() string` (the human-readable list rendered from the same source slice so prompt and gate cannot drift), and `go:generate` that writes `raft/scripts/zhen_prompt_generated.py` with `DEFAULT_SYSTEM_PROMPT` and `DESTRUCTIVE_VERBS` as Python literals plus a header `# GENERATED by go run ./pkg/zhenrag/gen — do not edit`.
- [x] **Step 83** [CODE][W] ~10m: Red tests in `pkg/zhenrag/zhenrag_test.go`: `TestSystemPrompt_ContainsCodeCommentsRule`, `TestDestructiveProse_MatchesPattern` (every verb in the prose matches the regexp and vice versa for the listed seeds), `TestGenerated_PythonInSync` (runs the generator into a temp dir and compares to the committed `raft/scripts/zhen_prompt_generated.py`). Green: implement `pkg/zhenrag/{zhenrag.go,verbs.go,gen/main.go}`; move `destructivePattern` from `pkg/champion/toolcall.go:122` to `zhenrag.DestructivePattern` and make champion import it (champion keeps `containsDestructive`).
- [x] **Step 84** [BUILD][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go generate ./pkg/zhenrag/... && go build ./... && go test ./pkg/zhenrag/... ./pkg/champion/... -race -count=1 2>&1 | tail -4
  ```
- [x] **Step 85** [V] ~1m: PASS; `raft/scripts/zhen_prompt_generated.py` exists and is valid Python (`~/.venv/zhen/bin/python3 -c 'import ast,sys; ast.parse(open("raft/scripts/zhen_prompt_generated.py").read())'`).
- [x] **Step 86** [CODE][W] ~8m: Consumers: `pkg/agent/agent.go:746` region → `zhenrag.SystemPrompt(zhenrag.Agent)`; `cmd/zhen-rag/main.go:272` region → `zhenrag.SystemPrompt(zhenrag.Chat)`; `raft/scripts/zhen_rag.py:155` → `from zhen_prompt_generated import DEFAULT_SYSTEM_PROMPT, DESTRUCTIVE_VERBS` and the prose at `:200-215` rendered from `DESTRUCTIVE_VERBS`. Keep `TestCodeCommentExample_MatchesPython` (point it at the generated file). Add `TestPrompt_ByteIdentity_GoVsPython` in `cmd/zhen-rag/main_test.go` comparing `zhenrag.SystemPrompt(Chat)` to the Python literal parsed with a tiny reader.
- [x] **Step 87** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/agent/... ./cmd/zhen-rag/... ./pkg/zhenrag/... -race -count=1 2>&1 | tail -4 && ~/.venv/zhen/bin/python3 -m pytest raft/tests/test_zhen_rag_retrieve.py raft/tests/test_memory_poison.py -q 2>&1 | tail -2
  ```
- [x] **Step 88** [V] ~1m: All green; exactly one `CODE COMMENTS` literal remains in Go (`grep -rn 'CODE COMMENTS' --include=*.go pkg cmd | grep -v _test | wc -l` = 1) and one in Python (the generated file).
- [x] **Step 89** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 81-88: pkg/zhenrag owns the prompt and the verb list; Python consumes the generated copy (RT-07/08)`

### Coding gate after the prompt change (mandatory)

- [ ] **Step 90** [TEST][B] ~15m: Coding gate, 5 seeds (memory: project_zhenai_coding_gate). Needs llama-server with qwen-7b: start it ONLY through the fit check.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/switch-model.sh --status 2>&1 | tail -2; scripts/switch-model.sh qwen-7b 2>&1 | tail -3 && make coding-gate-ci 2>&1 | tail -15
  ```
- [ ] **Step 91** [V] ~1m: Gate PASS at the recorded bar (the script prints PASS/FAIL per seed). If FAIL → the prompt text changed semantically in Step 86; diff `zhenrag.SystemPrompt(Chat)` against Step 81's `p_py.txt` and restore wording until identical; re-run. Max 2 attempts, then STUCK.
- [ ] **Step 92** [D] ~5m: If `switch-model.sh` refuses (fit check rc=3) → `scripts/switch-model.sh --stop; sleep 5;` retry once (S21 gotcha). Never bypass the fit check.

### Runbook body hash before execute

- [x] **Step 93** [DESIGN][W] ~4m: Contract: `runbooks/MANIFEST.sha256` (generated: `sha256sum runbooks/**/*.yaml`, sorted) is the approved set; `RunbookExecute` refuses a runbook whose sha256 is not in the manifest (`ErrRunbookUnapproved`) unless `dryRun` is true; `scripts/check-runbooks.sh` gains a manifest freshness check so CI fails when a runbook changes without the manifest.
- [x] **Step 94** [CODE][W] ~10m: Red: `TestRunbookExecute_HashMismatchRefused` + `TestRunbookExecute_DryRunIgnoresManifest`. Green: `pkg/champion/runbookmanifest.go` (`LoadManifest(dir)`, `Verify(path)`), wired into `RunbookExecute` before the subprocess; `scripts/runbook-manifest.sh` generates; `scripts/check-runbooks.sh` calls it with `--check`.
- [x] **Step 95** [B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && scripts/runbook-manifest.sh --write && wc -l runbooks/MANIFEST.sha256 && go test ./pkg/champion/ -run 'RunbookExecute' -race -count=1 2>&1 | tail -3 && scripts/check-runbooks.sh 2>&1 | tail -2
  ```
- [x] **Step 96** [V] ~1m: 79 manifest lines; tests PASS; check-runbooks still `0 new finding(s)`.
- [x] **Step 97** [TEST][B] ~3m: The runbook pytest suite (uses `--dry-run`, must be unaffected)
  ```bash
  cd /home/govan/tmp/unheaded && ~/.venv/zhen/bin/python3 -m pytest tests/runbooks -q -m "not live" 2>&1 | tail -2
  ```
- [x] **Step 98** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 93-97: runbook manifest, hash-checked before execute`

### Model and embedder pins (RT-15)

- [x] **Step 99** [R] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 55,75p scripts/switch-model.sh && grep -nE 'MODEL_DIR|GGUF=' scripts/switch-model.sh | head -5 && grep -rnE 'embed|EMBED' raft/scripts/zhen_rag.py raft/start-zhen.sh | head -5
  ```
- [x] **Step 100** [LIVE-RO][B] ~3m: Hash the files on west (read-only)
  ```bash
  cd /home/govan/tmp/unheaded && d=$(grep -oE 'MODEL_DIR=.*' scripts/switch-model.sh | head -1 | cut -d= -f2- | tr -d '"'); d=$(eval echo $d); for f in $(grep -oE 'MODEL_FILE\[[a-z0-9-]+\]="[^"]+"' scripts/switch-model.sh | cut -d'"' -f2 | sort -u); do [ -f "$d/$f" ] && sha256sum "$d/$f" | cut -c1-16 | tr '\n' ' ' && echo " $f" || echo "MISSING $f"; done
  ```
- [x] **Step 101** [CODE][W] ~6m: `scripts/lib/model-pins.sh` with `MODEL_SHA256[<key>]=<hex>` for every present model (from Step 100) and `EMBEDDER_SHA256`; `switch-model.sh` sources it and refuses to start a model whose file hash differs (`[switch-model] sha256 mismatch for <key>: refusing`), with `--no-verify` for Stevie's local experiments (logged). `raft/scripts/zhen_rag.py` checks the embedder file hash at startup the same way (env `ZHEN_EMBEDDER_SHA256` from the same lib via `start-zhen.sh`).
- [x] **Step 102** [TEST][B] ~3m: Shell test with a fake model dir
  ```bash
  cd /home/govan/tmp/unheaded && bash -n scripts/switch-model.sh scripts/lib/model-pins.sh && shellcheck -S warning scripts/switch-model.sh scripts/lib/model-pins.sh 2>&1 | tail -3; tmp=$(mktemp -d) && echo bogus > $tmp/qwen2.5-coder-7b-instruct-q4_k_m.gguf && MODEL_DIR=$tmp scripts/switch-model.sh --dry-run qwen-7b 2>&1 | grep -q 'sha256 mismatch' && echo "mismatch refused"
  ```
- [x] **Step 103** [V] ~1m: "mismatch refused" printed; shellcheck clean. If `--dry-run` does not exist → add it in Step 101 (prints the decision, starts nothing).
- [x] **Step 104** [DOC-UPDATE][W] ~3m: `wiki/Zhen-AI.md` (models section): pins live in `scripts/lib/model-pins.sh`; adding a model = add file + hash; `--no-verify` is for experiments only.
- [x] **Step 105** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 99-104: GGUF and embedder sha256 pins verified at start (RT-15)`

### Merge the worktree

- [x] **Step 106** [B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && git merge --no-ff sprint/champion-hardening-zhenrag -m "merge Phase 4: pkg/zhenrag, runbook manifest, model pins" && go build ./... && echo MERGED
  ```
- [x] **Step 107** [TEST][B] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/zhenrag/... ./pkg/champion/... ./pkg/agent/... ./cmd/zhen-rag/... ./cmd/zhen-agentd/... -race -count=1 2>&1 | tail -6
  ```
- [x] **Step 108** [V] ~1m: Green after merge (Phase 1-3 and Phase 4 both touched `toolcall.go`; resolve by keeping the allowlist AND the moved regexp).
- [ ] **Step 109** [TEST][B] ~15m: Coding gate once more on the merged tree (prompt bytes must be unchanged by the merge)
  ```bash
  cd /home/govan/tmp/unheaded && make coding-gate-ci 2>&1 | tail -8
  ```
- [ ] **Step 110** [V] ~1m: **PHASE 4 EXIT GATE** — Steps 88, 91, 96, 103, 108, 109 green. Commit `[PLAN S-CHAMPION-HARDENING] Steps 106-110: Phase 4 merged, coding gate green`.

---

## PHASE 5: ROUTERS, CAPS AND THE LEVEL 2 AMENDMENT (Steps 111-124)

**Goal**: ADR-106 Q7 answered in code and doc (both routers kept, one documented seam), family caps leave headroom for a worst-case request, ADR-019's L2 amendment put to Stevie with the code's current behaviour stated. Cites M-125 (PLANS F-23/F-24, ADR S-02, Z-19).
**Prerequisite**: Phase 0 (worktree `../unheaded-wt-claudebridge`). [P:1-4]
**Time**: 1.5 h
**Agent**: Agent C (worktree claudebridge)

- [x] **Step 111** [R] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 150,200p pkg/claudebridge/router.go && sed -n 20,60p pkg/claudebridge/autoroute.go && grep -n 'Router' cmd/zhen-claude/main.go cmd/lich-mcp/*.go | grep -v _test | head -8 && cat configs/zhen-routing.yaml | head -40
  ```
- [x] **Step 112** [DECIDE][DESIGN] ~3m: Two routers
  - RECOMMENDATION: keep both; `AutoRouter` (weight class for Lich drivers) and `ThresholdRouter` (family bar from the eval) stay separate types behind one `Router` interface in `router.go`; `AutoRouter` may consult `ThresholdRouter` for the `family` it derives, via an optional `Fallback Router` field, default nil. Rationale: they answer different questions (how heavy vs how trusted-locally); folding them would couple the Lich's weight classes to the eval's per-family bars. Override ONLY if a third router appears.
- [x] **Step 113** [CODE][W] ~8m: Red: `TestRouterInterface_BothSatisfy`, `TestAutoRouter_FallbackToThreshold` (a family the AutoRouter cannot classify defers). Green: the interface + optional fallback; doc comment cites ADR-106 Q7 and this plan.
- [x] **Step 114** [CODE][W] ~6m: Family caps headroom: red `TestFamilyCap_LeavesWorstCaseHeadroom` (with `-daily-cap-usd 5`, after N requests the remaining budget is at least `prices.json` worst-case for `-max-tokens` at the routed model; the (N+1)th request that would exceed is refused BEFORE the API call). Green in `spend.go` (pre-check against `maxTokens * outputPrice + promptEstimate * inputPrice`).
- [x] **Step 115** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/claudebridge/... -race -count=1 2>&1 | tail -4 && go test ./cmd/zhen-claude/... -race -count=1 2>&1 | tail -2
  ```
- [x] **Step 116** [V] ~1m: PASS; `fake_api_test.go` unchanged (no live API call in tests).
- [x] **Step 117** [ESCALATE][DESIGN] ~5m: **E3: ADR-019 Level 2 amendment (branch-sandbox sub-mode)**
  - Context: `docs/adr/ADR-019-zhen-champion-agent.md:220-245` Proposed 2026-10-23; code `pkg/champion/branchsandbox.go` is live and used by `zhen-claude -route local -family -worktree`. Phase 3 above added openat2 so the sandbox cannot be escaped by a symlink even inside a worktree.
  - Question: accept (Rule 2 satisfied by the `zhen/<name>` branch sandbox for `write_file`/`patch_file`; you review the branch) or reject (models return patches as text; agent applies them; per-call confirmations stay)?
  - Why no recommendation: this moves the human review point from each write to each branch; that is the trust decision ADR-019 reserves to the operator.
- [ ] **Step 118** [AFTER-E3][W] ~5m: Record the answer in ADR-019 (status line of the amendment → Accepted or Rejected, date, one line why); if Rejected, add `TestBranchSandbox_Disabled` and a feature flag default off in `branchsandbox.go`.
- [ ] **Step 119** [AFTER-E3][TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/champion/ -run Branch -race -count=1 2>&1 | tail -3
  ```
- [x] **Step 120** [DOC-UPDATE][W] ~5m: `docs/adr/ADR-106-two-brains-one-gate.md` §Open questions: Q7 answered (Step 112), caps rule (Step 114) recorded under Consequences; ADR-019 "Open Questions (To Be Filled In)" at `:506` gets the checklist closed with pointers (Rules 1-3 tests, L2 amendment status, confirm flow Steps 41-48).
- [x] **Step 121** [B] ~2m: Merge
  ```bash
  cd /home/govan/tmp/unheaded && git merge --no-ff sprint/champion-hardening-claudebridge -m "merge Phase 5: router seam, cap headroom, ADR-019/106 answers" && go build ./... && echo MERGED
  ```
- [x] **Step 122** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/claudebridge/... ./pkg/champion/... -race -count=1 2>&1 | tail -4
  ```
- [x] **Step 123** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 111-122: routers kept behind one interface, cap headroom, E3 filed`
- [x] **Step 124** [V] ~1m: **PHASE 5 EXIT GATE** — Steps 116, 122 green; E3 filed in this file's Escalations table with the date. Steps 118-119 remain open [AFTER-E3] without blocking Phase 6.

---

## PHASE 6: RUNBOOK AUTHORSHIP POLICY AND APPROVALS (Steps 125-142)

**Goal**: ADR-024/ADR-106 record who authors runbooks and who approves them; the eleven drafts are approved by Stevie or stay draft with the re-run trigger written down; `check-runbooks.py` enforces the policy fields. Cites M-120 (INT A-1/A-2, ADR-106 Q3; decision 30).
**Prerequisite**: Phase 4 (manifest) merged.
**Time**: 1 h + Stevie
**Agent**: Coordinator

- [x] **Step 125** [R] ~3m: The eleven and their fields
  ```bash
  cd /home/govan/tmp/unheaded && for f in $(grep -rlE '^\s+status: draft' runbooks --include=*.yaml | sort); do printf "%-62s %s\n" "$f" "$(grep -oE '^\s+(authored_by|approved_by):.*' $f | tr -d ' ' | tr '\n' ' ')"; done
  ```
- [x] **Step 126** [R] ~3m: Eval verdict for local authorship (the bar is locked; read only)
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'rb-author|\*\*all\*\*' eval/champion-runbooks/RESULTS-2026-10-26.md | head -8 && sed -n '/## Verdict/,/^## /p' eval/champion-runbooks/RESULTS-2026-10-26.md | head -12
  ```
- [x] **Step 127** [DECIDE][DESIGN] ~5m: Authorship policy
  - RECOMMENDATION: "Claude (via `zhen-claude`/Claude Code) authors runbooks; Zhen (local model through the Champion) executes approved runbooks and may propose edits as `status: draft`; a local model may author only after it clears the PREREG bar on `rb-author` (0.80) AND overall (0.85) in `eval/champion-runbooks`; Stevie approves every runbook (`approved_by: stevie`, `approved_at: <date>`)." Rationale: RESULTS-2026-10-26 shows local models at 0.36-0.73 on `rb-author`; Claude at 1.00. Override ONLY if Stevie wants a second approver (none exists: solo project, memory project_solo_passion_project).
- [x] **Step 128** [W] ~6m: Record it: new section "Authorship and approval" in `docs/adr/ADR-024-zhen-runbook-automation.md` (status line unchanged, add "Amended 2026-10-xx: authorship policy") and a one-line pointer under ADR-106 Q3. State the re-run trigger verbatim.
- [x] **Step 129** [CODE][W] ~6m: `scripts/check-runbooks.py`: when `metadata.status == approved`, require `approved_by` non-empty and `approved_at` ISO date, and `authored_by ∈ {claude, stevie, zhen-local}`; `zhen-local` is allowed only if `eval/champion-runbooks/RESULTS-*.md` contains a row `local/... | **all** | ... | 0.85 |` without ✗ (parse the latest file). Add `tests/runbooks/test_check_runbooks.py` cases for each rule.
- [x] **Step 130** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && ~/.venv/zhen/bin/python3 -m pytest tests/runbooks/test_check_runbooks.py -q 2>&1 | tail -2 && scripts/check-runbooks.sh 2>&1 | tail -2
  ```
- [x] **Step 131** [V] ~1m: PASS; still 0 new findings on the current tree (drafts have no `approved_at`, which is allowed for drafts).
- [x] **Step 132** [W] ~5m: Approval packet for Stevie: `docs/battle-plans/progress/S-CHAMPION-HARDENING-approvals.md` listing each of the 11 with: path, description, risk, requires_sudo, the test file in `tests/runbooks/` that covers it, last dry-run result (`scripts/run-runbook.py --dry-run <file> | tail -1`).
- [x] **Step 133** [B] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && for f in $(grep -rlE '^\s+status: draft' runbooks --include=*.yaml | sort); do echo "== $f"; ~/.venv/zhen/bin/python3 scripts/run-runbook.py --dry-run $f 2>&1 | tail -1; done
  ```
- [x] **Step 134** [V] ~1m: Every dry-run ends without a Python traceback (a runbook that fails dry-run is NOT in the approval packet; it gets a card instead).
- [x] **Step 135** [ESCALATE] (filed 2026-10-27: `docs/battle-plans/progress/S-CHAMPION-HARDENING-approvals.md`; awaiting Stevie) ~2m: **E1: approve the runbooks and the policy**
  - Context: Step 132 packet; Step 127 policy text.
  - Questions: (1) Is the authorship policy wording acceptable as written in ADR-024? (2) Which of the 11 do you approve now (`approved_by: stevie`)? Any to keep draft, and why?
  - Why no recommendation on (2): approval is the operator's signature on runbooks that execute with `requires_sudo` on west/east.
- [x] **Step 136** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 125-135: authorship policy in ADR-024, check-runbooks enforces it, E1 packet` (policy + checker + packet; runbooks untouched)
- [ ] **Step 137** [AFTER-E1][W] ~5m: For each approved runbook set `status: approved`, `approved_by: stevie`, `approved_at: <answer date>`; regenerate the manifest.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/runbook-manifest.sh --write && scripts/check-runbooks.sh 2>&1 | tail -2
  ```
- [ ] **Step 138** [AFTER-E1][V] ~1m: `0 new finding(s)`; `git diff --stat runbooks | tail -1` shows only the approved files + MANIFEST.
- [ ] **Step 139** [AFTER-E1][TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && ~/.venv/zhen/bin/python3 -m pytest tests/runbooks -q -m "not live" 2>&1 | tail -2 && go test ./pkg/champion/ -run RunbookExecute -count=1 2>&1 | tail -2
  ```
- [ ] **Step 140** [AFTER-E1][DOC-UPDATE][W] ~3m: `wiki/Runbooks.md`: the approval column and the policy pointer; `docs/adr/ADR-106-two-brains-one-gate.md` Q3 → answered (date).
- [ ] **Step 141** [AFTER-E1][C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 137-140: <k> runbooks approved by Stevie (E1)`
- [ ] **Step 142** [V] ~1m: **PHASE 6 EXIT GATE** — Steps 131, 134 green and E1 filed. The [AFTER-E1] steps may remain open; Phase 7 does not depend on them.

---

## PHASE 7: THE EVAL SCORES RUNBOOK-DRAFT; RUNBOOKS VERIFY THEMSELVES (Steps 143-158)

**Goal**: `zhen-eval` stops skipping `runbook-draft`; the judge is the Phase 8 harness (`tests/runbooks` dry-run + `validate_runbook.py`); `run-runbook.py` executes a runbook's `verification:` section after its steps. Cites M-127 (ADR Z-08, INT A-5).
**Prerequisite**: Phase 4 (manifest) merged; Phase 6 policy recorded.
**Time**: 2 h
**Agent**: Agent A

- [ ] **Step 143** [R] ~4m: How `check` judging works for `rb-author` and what `runbook-draft` tasks look like
  ```bash
  cd /home/govan/tmp/unheaded && grep -n '"check"' cmd/zhen-eval/main.go | head && ls eval/champion-runbooks/fixtures | head -5 && cat eval/champion-runbooks/fixtures/$(ls eval/champion-runbooks/fixtures | head -1)/check.sh 2>/dev/null | head -20 && grep -c 'runbook-draft' eval/coding-gate/tasks.jsonl 2>/dev/null || grep -rl 'runbook-draft' eval | head -3
  ```
- [ ] **Step 144** [DESIGN][W] ~5m: Judge contract in `cmd/zhen-eval/README.md` (or the file that documents judges): `runbook-draft` tasks carry `judge: "check"` with `check: "runbook-draft"`; the judge writes the model's YAML to a temp file, runs `scripts/run-runbook.py --dry-run <tmp>` (exit 0), `scripts/check-runbooks.py --one <tmp>` (0 new findings, strict because the draft must carry `metadata.status: draft`), and the task's own `requirements` grep list; all three must pass.
- [ ] **Step 145** [CODE][W] ~8m: Red: in `cmd/zhen-eval/main_test.go` change the assertion at `:35` to `fam["runbook-draft"] == <count from Step 143>` and add `TestJudgeRunbookDraft_PassAndFail` with a passing and a failing YAML fixture under `cmd/zhen-eval/testdata/`. Green: remove the `continue` at `main.go:307`, implement `judgeRunbookDraft` calling the two scripts with `exec.CommandContext` (timeout 60 s, cwd repo root, `python3` from `ZHEN_EVAL_PYTHON` or PATH).
- [ ] **Step 146** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && PATH=~/.venv/zhen/bin:$PATH go test ./cmd/zhen-eval/... -race -count=1 2>&1 | tail -4
  ```
- [ ] **Step 147** [V] ~1m: PASS. If `check-runbooks.py` has no `--one` → add it in Step 145 (validate a single path, no baseline write).
- [ ] **Step 148** [TEST][B] ~10m: Dry run of the eval on `runbook-draft` only, local model, 3 tasks, scratch output (NOT committed; results files lock bars)
  ```bash
  cd /home/govan/tmp/unheaded && scripts/switch-model.sh --status | tail -1 && go run ./cmd/zhen-eval -families runbook-draft -limit 3 -out /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/eval-rbdraft-smoke.jsonl 2>&1 | tail -6
  ```
- [ ] **Step 149** [V] ~1m: Three rows, each with a `verdict` and the three judge sub-results; no panic. If `-limit`/`-out` flags do not exist → use the flags the binary has (`go run ./cmd/zhen-eval -h`) and record them in Appendix C.
- [ ] **Step 150** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 143-149: zhen-eval judges runbook-draft with the Phase 8 harness`

### run-runbook.py executes `verification:`

- [ ] **Step 151** [R] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 150,215p scripts/run-runbook.py && grep -nE -A6 '^\s*verification:' runbooks/infra/west-startup-order.yaml | head -14
  ```
- [ ] **Step 152** [CODE][W] ~8m: Red: `tests/runbooks/test_verification_section.py` (a fixture runbook whose `verification:` has one passing and one failing `check`; dry-run prints both as `(dry run)`; real run with a sandbox env exits non-zero naming the failing check; a runbook without the section is unchanged). Green: `run_verification(runbook, env, dry_run)` after `execute_steps`, same `check`/`description` shape as preconditions; summary line `verification: n/m passed`.
- [ ] **Step 153** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && ~/.venv/zhen/bin/python3 -m pytest tests/runbooks/test_verification_section.py tests/runbooks/test_harness.py -q 2>&1 | tail -2
  ```
- [ ] **Step 154** [B] ~3m: Dry-run the four runbooks that carry the section
  ```bash
  cd /home/govan/tmp/unheaded && for f in runbooks/wotan/active-active-cutover.yaml runbooks/network/air-gap-egress-validation.yaml runbooks/observe/slo-error-budget-burn.yaml runbooks/infra/west-startup-order.yaml; do ~/.venv/zhen/bin/python3 scripts/run-runbook.py --dry-run $f 2>&1 | grep -E 'verification:|Traceback' | tail -2; done
  ```
- [ ] **Step 155** [V] ~1m: Four `verification: n/n (dry run)` lines, no traceback.
- [ ] **Step 156** [TEST][B] ~3m: Whole runbook suite
  ```bash
  cd /home/govan/tmp/unheaded && ~/.venv/zhen/bin/python3 -m pytest tests/runbooks -q -m "not live" 2>&1 | tail -2 && scripts/check-runbooks.sh 2>&1 | tail -1
  ```
- [ ] **Step 157** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 151-156: run-runbook.py executes verification sections`
- [ ] **Step 158** [V] ~1m: **PHASE 7 EXIT GATE** — Steps 147, 149, 155, 156 green.

---

## PHASE 8: THE SCHEDULER BECOMES A SUPERVISED WORKER (Steps 159-176)

**Goal**: scheduled runbooks and heartbeats run inside `zhen-agentd` (one unit, one user, Well-recorded runs, one Kanban card per run, a heartbeat alert when they stop), and `raft/zhen_scheduler.py` is retired. Cites M-130 (ADR Z-06, ADR-028).
**Prerequisite**: Phases 2, 6.
**Time**: 2.5 h + E4 window
**Agent**: Coordinator (E4 steps), Agent A (code)

- [ ] **Step 159** [R] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'def (add_schedule|remove_schedule|load_schedules|run_job|heartbeat)' raft/zhen_scheduler.py && grep -nE 'SCHEDULE_FILE|schedules.json' raft/zhen_scheduler.py | head -3 && grep -nE 'overnight|worker' cmd/zhen-agentd/main.go | head -8 && ls db/migrations | grep -iE 'agent_runs|zhen' 
  ```
- [ ] **Step 160** [DECIDE][DESIGN] ~4m: Fold or supervise
  - RECOMMENDATION: fold into `zhen-agentd` as a `-scheduler` mode reading `configs/zhen-schedules.yaml` (replacing the JSON file), executing through `Champion.RunbookExecute` (so the manifest check, audit log and gate apply), recording each run as a `zhen_agent_runs` row with `mode: scheduled` (migration 032's table; in-memory store when the Well is down), creating one Kanban card per run through the Champion's `kanban_create` with `type: scheduled-run`. Rationale: a second Python daemon would need its own user, unit, Well role and audit path; agentd already has all four. Override ONLY if the Python scheduler must keep running on a host without the Go binary (none: both hosts run agentd).
- [ ] **Step 161** [CODE][W] ~10m: Red tests in `cmd/zhen-agentd/scheduler_test.go`: schedule file parsed (cron-like `every: 1h|daily@03:00`), a due job executes via a fake Champion exactly once, a run row and a card are recorded, a failing job sets `last_error` and does not block the next job, heartbeat publishes `zhen.scheduler.heartbeat` every tick. Green: `cmd/zhen-agentd/scheduler.go` (stdlib `time` only; no cron library: a 60 s ticker + `next_due` computation; `-scheduler` flag + `-schedules` path).
- [ ] **Step 162** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-agentd/... -race -count=1 2>&1 | tail -4
  ```
- [ ] **Step 163** [V] ~1m: PASS.
- [ ] **Step 164** [W] ~5m: `configs/zhen-schedules.yaml` with today's jobs (read them from the JSON the Python scheduler used: `grep -n SCHEDULE_FILE raft/zhen_scheduler.py`; if the file is absent on west, start with `conversation-memory-maintenance` weekly and the heartbeat), each naming an approved runbook; `scripts/check-manifest-yaml.sh` must still pass.
- [ ] **Step 165** [CODE][W] ~6m: Webapp: `raft/zhen_app.py:849-880` schedule commands call agentd `POST /api/v1/scheduler/{add,remove,list}` (loopback, same auth) instead of importing `zhen_scheduler`; the `pkill -f zhen_scheduler` line goes. Add the three endpoints to agentd (mutating add/remove go through the Champion gate as `schedule_update`, registered in `MutatingTools`). Test in `cmd/zhen-agentd` and a Python unit test with a mocked `requests`.
- [ ] **Step 166** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-agentd/... ./pkg/champion/... -race -count=1 2>&1 | tail -3 && ~/.venv/zhen/bin/python3 -m pytest raft/tests -q 2>&1 | tail -2
  ```
- [ ] **Step 167** [W] ~4m: Heartbeat alert: `monitoring/prometheus/rules/zhen.yml` rule `ZhenSchedulerStalled` on `absent_over_time(zhen_scheduler_heartbeat_timestamp_seconds[15m])` (agentd exports the gauge); `monitoring/prometheus/rules/tests/zhen_test.yml` promtool case; `runbooks/observe/zhen-scheduler-stalled.yaml` (draft, Claude-authored, per Phase 6 policy).
- [ ] **Step 168** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-prometheus-rules.sh 2>&1 | tail -3 && scripts/check-runbooks.sh 2>&1 | tail -1
  ```
- [ ] **Step 169** [W] ~3m: Retire the Python scheduler: `git rm raft/zhen_scheduler.py`; ADR-028 status → "Superseded by zhen-agentd -scheduler (S-CHAMPION-HARDENING Step 160)"; `wiki/Zhen-AI.md` scheduler section rewritten (unit, config file, how to add a job, where runs show up).
- [ ] **Step 170** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 159-169: scheduler folded into zhen-agentd; Python scheduler retired`

### Unit on west (E4)

- [ ] **Step 171** [W] ~4m: `deploy/systemd/zhen-agentd.service` gains `-scheduler -schedules /etc/unheaded/zhen/schedules.yaml`; `deploy/systemd/zhen-agentd.env.example` documents `ZHEN_SCHEDULES`; `scripts/check-systemd-units.sh` and `scripts/check-unit-binaries.sh` pass.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-systemd-units.sh 2>&1 | tail -2 && scripts/check-unit-binaries.sh 2>&1 | tail -2
  ```
- [ ] **Step 172** [ESCALATE] ~2m: **E4: one west host window** to install `zhen-agentd` (with scheduler) and `unheaded-vor` units (Phase 9), run `systemd-sysusers`, restart agentd and the webapp. Context: both are loopback-only, nologin users, hardened units; the webapp restart follows the S21 recipe. Question: when, and is `zhen-agentd` to be enabled at boot? Why Stevie: host change on west.
- [ ] **Step 173** [AFTER-E4][LIVE-RW][S][BARE-METAL][B] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && go build -o /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/zhen-agentd ./cmd/zhen-agentd && sudo install -m 0755 /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/zhen-agentd /usr/local/bin/zhen-agentd && sudo install -m 0644 deploy/systemd/zhen-agentd.service /etc/systemd/system/ && sudo install -d -o zhen-agentd -g zhen-agentd -m 0750 /etc/unheaded/zhen && sudo install -m 0640 -o root -g zhen-agentd configs/zhen-schedules.yaml /etc/unheaded/zhen/schedules.yaml && sudo systemctl daemon-reload && sudo systemctl restart zhen-agentd && sleep 2 && systemctl is-active zhen-agentd
  ```
- [ ] **Step 174** [AFTER-E4][LIVE-RO][V] ~2m
  ```bash
  curl -s --max-time 3 http://127.0.0.1:20105/health | head -c 300; echo; curl -s --max-time 3 http://127.0.0.1:20105/metrics | grep -E '^zhen_scheduler_heartbeat' | head -2; journalctl -u zhen-agentd --since -2min --no-pager | tail -5
  ```
  - `active`, health 200, heartbeat gauge present, no ERROR lines.
- [ ] **Step 175** [AFTER-E4][D] ~5m: If the unit fails: `journalctl -u zhen-agentd -n 30`; the usual causes are the env file missing a DSN (then `-action-store=stderr` until S-WELL-SPRING) or `ReadWritePaths` not covering `/etc/unheaded/zhen`. Two attempts, then STUCK and `sudo systemctl revert zhen-agentd`.
- [ ] **Step 176** [V] ~1m: **PHASE 8 EXIT GATE** — Steps 163, 166, 168 green; Step 171 gates pass; E4 filed. Commit `[PLAN S-CHAMPION-HARDENING] Steps 171-175: scheduler unit on west (E4)` when 173-174 are done.

---

## PHASE 9: VOR OWNED BY A UNIT; MEMORY WITH PROVENANCE (Steps 177-190)

**Goal**: the retrieval backend runs as `unheaded-vor` (own nologin user, registered port and uid, swap off), the memory consolidation runbook is scheduled, recalled memories show their origin, and `_strip_tool_call_json` has a test. Cites M-131 (ADR L-04, Z-17; RT-14, RT-16).
**Prerequisite**: Phase 8 scheduler (for the consolidation job).
**Time**: 1.5 h + E4
**Agent**: Coordinator

- [ ] **Step 177** [R] ~3m
  ```bash
  /usr/local/bin/vor --help 2>&1 | head -20; /usr/local/bin/vor serve --help 2>&1 | grep -iE 'port|listen|addr|index|data' | head -6; grep -nE 'VOR_|vor' raft/start-zhen.sh raft/scripts/zhen_rag.py | head -8
  ```
- [ ] **Step 178** [R] ~2m: Free port and uid
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '= 2010[0-9]' pkg/ports/ports.go && grep -nE '= 1679[0-9]' pkg/uids/uids.go | tail -3
  ```
- [ ] **Step 179** [CODE][W] ~5m: Registries: `pkg/ports/ports.go` `ZhenVor = 20106` (or the first free in 20101-20106 "Reserved AI services"; fix the `SophiaEye = 20105` vs zhen-agentd collision found in Step 9 by giving agentd its own constant `ZhenAgentd = 20105` ONLY if SophiaEye is unused: `grep -rn 'ports.SophiaEye' --include=*.go . | grep -v _test`; otherwise move agentd to the next free port and update its unit + webapp default), `pkg/uids/uids.go` `Vor = 1679x` next free with the name map entry `"unheaded-vor"`; `deploy/sysusers.d/unheaded.conf` line `u unheaded-vor - "Zhen retrieval backend (vor)" / /usr/sbin/nologin`. Tests: `go test ./pkg/ports/... ./pkg/uids/... -count=1`.
- [ ] **Step 180** [W] ~5m: `deploy/systemd/unheaded-vor.service`: `User=unheaded-vor`, `ExecStart=/usr/local/bin/vor serve --listen 127.0.0.1:<port> --index /var/lib/unheaded/vor`, `MemorySwapMax=0`, `MemoryMax=` sized from `vor`'s index (read `du -sh ~/.local/share/vor 2>/dev/null || true` in Step 177), the standard hardening block (`NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome=yes`, `ReadWritePaths=/var/lib/unheaded/vor`, `IPAddressAllow=localhost`, `IPAddressDeny=any`), `Restart=on-failure`.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-systemd-units.sh 2>&1 | tail -2 && systemd-analyze verify deploy/systemd/unheaded-vor.service 2>&1 | head -3
  ```
- [ ] **Step 181** [CODE][W] ~4m: `raft/start-zhen.sh` and `raft/scripts/zhen_rag.py` read `VOR_URL` (default `http://127.0.0.1:<port>`) instead of a hardcoded port; the 60 s startup wait (S21 `ecbd14d1`) stays.
- [ ] **Step 182** [CODE][W] ~5m: `raft/tests/test_zhen_rag_strip.py`: `_strip_tool_call_json` removes a trailing/embedded `{"tool_call": …}` object, leaves prose, handles nested braces and a bare `{`; add the file to the raft CI pytest list (Phase 10 Step 203 edits the same line: coordinate by adding both here).
  ```bash
  cd /home/govan/tmp/unheaded && ~/.venv/zhen/bin/python3 -m pytest raft/tests/test_zhen_rag_strip.py -q 2>&1 | tail -2
  ```
- [ ] **Step 183** [CODE][W] ~6m: Provenance: every recalled memory passed to the prompt carries `[memory: <topic> @ <date> via <vor|well>]`, rendered in the UI's references list; Python test asserting the marker; `pkg/vorquery` Go side gets the same field (`TestMatchesPython` already compares the two). Add the consolidation job to `configs/zhen-schedules.yaml` (weekly, `runbooks/data/conversation-memory-maintenance.yaml`).
- [ ] **Step 184** [AFTER-E4][LIVE-RW][S][BARE-METAL][B] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers && id unheaded-vor && sudo install -d -o unheaded-vor -g unheaded-vor -m 0750 /var/lib/unheaded/vor && sudo rsync -a --chown=unheaded-vor:unheaded-vor ~/.local/share/vor/ /var/lib/unheaded/vor/ 2>/dev/null; sudo install -m 0644 deploy/systemd/unheaded-vor.service /etc/systemd/system/ && sudo systemctl daemon-reload && systemctl --user stop unheaded-vor.scope 2>/dev/null; sudo systemctl enable --now unheaded-vor && sleep 3 && systemctl is-active unheaded-vor
  ```
- [ ] **Step 185** [AFTER-E4][LIVE-RO][V] ~2m
  ```bash
  curl -s --max-time 5 "http://127.0.0.1:$(grep -oE 'ZhenVor\s*=\s*[0-9]+' /home/govan/tmp/unheaded/pkg/ports/ports.go | grep -oE '[0-9]+$')/health" | head -c 200; echo; systemctl show unheaded-vor -p MemorySwapMax -p User
  ```
  - health answers; `MemorySwapMax=0`; `User=unheaded-vor`.
- [ ] **Step 186** [AFTER-E4][LIVE-RW][B] ~3m: Restart the webapp with the S21 recipe so it reads `VOR_URL`
  ```bash
  cd /home/govan/tmp/unheaded/raft && pkill -f 'python3 zhen_app.py' ; sleep 2; source ~/.venv/zhen/bin/activate && export ZHEN_DB_PASSWORD=$(grep '^APP_ZHEN_PASSWORD=' ../.env | cut -d= -f2- | sed 's/^"//;s/"$//') && set -a && source ./start-zhen.sh --env-only 2>/dev/null; set +a; setsid nohup python3 zhen_app.py >> ~/.local/state/unheaded/log/zhen/zhen-webapp.log 2>&1 & sleep 5; curl -s --max-time 5 http://127.0.0.1:20103/health | head -c 200; echo
  ```
  - If `start-zhen.sh` has no `--env-only`, export the `ZHEN_*` lines it sets by hand (they are listed at its top).
- [ ] **Step 187** [AFTER-E4][LIVE-RO][V] ~3m: "what is wotan?" through the UI returns the Service-Wotan references with provenance markers (S21's regression question)
  ```bash
  curl -s --max-time 60 -X POST http://127.0.0.1:20103/api/v1/ask -H 'Content-Type: application/json' -d '{"question":"what is wotan?"}' | head -c 600; echo
  ```
- [ ] **Step 188** [AFTER-E4][D] ~5m: If vor answers slowly (>10 s): the index was swapped out before the move; `systemctl restart unheaded-vor` once with swap off warms it. If the unit cannot read the index: ownership (`ls -la /var/lib/unheaded/vor | head`).
- [ ] **Step 189** [DOC-UPDATE][W] ~4m: `wiki/Zhen-AI.md` RAG section: vor is a unit (`unheaded-vor`, port, index path, `MemorySwapMax=0`), consolidation is scheduled, provenance marker format; `wiki/Port-Registry.md` row; `wiki/Service-Isolation.md` (or the uid table page) row.
- [ ] **Step 190** [V] ~1m: **PHASE 9 EXIT GATE** — Steps 179-183 tests green; units verify; E4 live steps done or open under E4. Commit `[PLAN S-CHAMPION-HARDENING] Steps 177-189: unheaded-vor unit, port/uid registered, memory provenance, strip test`.

---

## PHASE 10: THE MCP SERVER IN GO (Steps 191-206)

**Goal**: `.mcp.json` launches a Go MCP server that exposes the same nine tools through the Champion (one gate, one audit path), the Python server is retired, MCP tests run in CI, and every denied call is `isError`. Cites M-132 (ADR Z-02; next.md S17 "ACCESS DENIED not isError", "raft CI doesn't install mcp").
**Prerequisite**: Phase 2 (dispatch truth), Phase 4 (prompt package). Worktree `../unheaded-wt-mcp`. [P:11]
**Time**: 3 h
**Agent**: Agent D (worktree mcp)

- [ ] **Step 191** [R] ~6m
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '@mcp\.tool|^def |ALLOW_WRITE|ALLOW_EXEC|isError|is_error' raft/zhen_mcp_server.py | head -30 && ls cmd/lich-mcp && grep -nE 'jsonrpc|"method"|tools/list|tools/call' cmd/lich-mcp/*.go | grep -v _test | head -8 && grep -n 'mcp' go.mod
  ```
- [ ] **Step 192** [DECIDE][DESIGN] ~4m: Transport and library
  - RECOMMENDATION: reuse `cmd/lich-mcp`'s stdio JSON-RPC implementation (extract the transport into `pkg/mcpserver` if it is inline; it is already first-party and has tests) rather than admitting an MCP SDK module. Rationale: ADR-004 admission cost and attack surface; the protocol subset Claude Code needs (`initialize`, `tools/list`, `tools/call`) is small. Override ONLY if `cmd/lich-mcp` turns out to depend on an SDK already (then reuse that module, no new admission).
- [ ] **Step 193** [CODE][W] ~12m: Red tests in `cmd/zhen-mcp/main_test.go`: `tools/list` returns the nine names (`corpus_search`, `file_read`, `service_health`, `runbook_list`, `runbook_show`, `local_llm`, and, only when `ZHEN_MCP_ALLOW_WRITE=1`/`ZHEN_MCP_ALLOW_EXEC=1`, `file_write`, `file_patch`, `runbook_execute`); a `file_read` outside the project root is `isError: true` with `ACCESS DENIED`; a not-found path is `isError: true`; a write without the env flag is absent from the list AND `isError` if called; `local_llm` with nothing listening is `isError` (never starts a model). Green: `cmd/zhen-mcp/main.go` on `pkg/mcpserver`, every tool through `champion.Dispatch` (mapping `file_read`→`read_file`, `file_write`→`write_file`, `file_patch`→`patch_file`); `corpus_search` through `pkg/vorquery`; `local_llm` through the existing llama client in `pkg/agent`; project root from `ZHEN_MCP_PROJECT_ROOT` or the executable's repo.
- [ ] **Step 194** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-mcp/... ./pkg/mcpserver/... -race -count=1 2>&1 | tail -4 && go vet ./cmd/zhen-mcp/...
  ```
- [ ] **Step 195** [V] ~1m: PASS; `grep -c isError cmd/zhen-mcp/main.go` ≥ 4.
- [ ] **Step 196** [CODE][W] ~4m: `scripts/zhen-mcp.sh` launches `bin/zhen-mcp` (built by `make build-zhen-mcp`; add the target next to `build-zhen-agent`), falling back to the Python server ONLY when the binary is absent AND `ZHEN_MCP_PYTHON_FALLBACK=1` (so a missing build fails loudly by default, naming `make build-zhen-mcp`).
  ```bash
  cd /home/govan/tmp/unheaded && make build-zhen-mcp 2>&1 | tail -2 && bash -n scripts/zhen-mcp.sh && shellcheck -S warning scripts/zhen-mcp.sh | tail -2
  ```
- [ ] **Step 197** [TEST][B] ~3m: Hand-shake over stdio exactly as Claude Code does
  ```bash
  cd /home/govan/tmp/unheaded && printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"file_read","arguments":{"path":"/etc/shadow"}}}' | timeout 10 scripts/zhen-mcp.sh 2>/dev/null | jq -c '{id, tools: (.result.tools|length?), isError: .result.isError?}' 
  ```
- [ ] **Step 198** [V] ~1m: id 1 answers, id 2 lists 6 tools (read-only env), id 3 is `isError: true`.
- [ ] **Step 199** [W] ~3m: Retire the Python server: `git rm raft/zhen_mcp_server.py raft/tests/test_zhen_mcp_server.py` AFTER porting any test case not yet covered by Step 193 (diff the two test lists first and record in the progress file); `.mcp.json` unchanged (still `scripts/zhen-mcp.sh`); CLAUDE.md "Zhen MCP Server" paragraph rewritten (Phase 15 does the wording; here only the path).
- [ ] **Step 200** [TEST][B] ~2m: Nothing else imports the Python server
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn 'zhen_mcp_server' --include=*.py --include=*.sh --include=*.yml --include=*.json . | grep -v '^./docs/' | grep -v '^./wiki/' || echo "no references left"
  ```
- [ ] **Step 201** [CODE][W] ~4m: CI: `.github/workflows/ci.yml` (Go) already runs `go test ./...` so `cmd/zhen-mcp` is covered; add the stdio handshake of Step 197 as `scripts/check-zhen-mcp.sh` and call it from the Static Analysis workflow's Go job (5 s, no model). `raft-python.yml:55` gains `raft/tests/test_zhen_rag_strip.py` (Phase 9) and drops the MCP test it never ran.
- [ ] **Step 202** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-zhen-mcp.sh && scripts/check-manifest-yaml.sh 2>&1 | tail -1 && ~/.venv/zhen/bin/python3 -m pytest raft/tests -q 2>&1 | tail -1
  ```
- [ ] **Step 203** [B] ~2m: Merge the worktree
  ```bash
  cd /home/govan/tmp/unheaded && git merge --no-ff sprint/champion-hardening-mcp -m "merge Phase 10: Go MCP server, Python server retired" && go build ./... && echo MERGED
  ```
- [ ] **Step 204** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-mcp/... ./pkg/champion/... ./cmd/zhen-agentd/... -race -count=1 2>&1 | tail -4
  ```
- [ ] **Step 205** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 191-204: Go MCP server through the Champion; denied calls are isError; MCP in CI`
- [ ] **Step 206** [V] ~1m: **PHASE 10 EXIT GATE** — Steps 195, 198, 200, 202, 204 green.

---

## PHASE 11: ZHEN-CLI PHASES 2b AND 3; THE SELECTOR'S TAIL (Steps 207-220)

**Goal**: `/recall` and `/remember` persist through agentd (no lib/pq in the CLI), `/confirm <token>` and `/model` exist, line editing works, and ADR-060's tail (`switch-model.sh --json`, swap metrics, rollback test, LICH-013 campaign hook) is closed. Cites M-129 (ADR Z-11, Z-12, Z-13; ADR-059, ADR-060).
**Prerequisite**: Phase 2 (confirm binding), Phase 4 (model pins). Worktree `../unheaded-wt-cli`. [P:10]
**Time**: 2.5 h
**Agent**: Agent E (worktree cli)

- [ ] **Step 207** [R] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && sed -n 240,300p cmd/zhen-cli/main.go && grep -nE 'memories|/api/v1/memory|recall' cmd/zhen-agentd/main.go | head -6 && grep -nE 'case "|--status|--stop|--dry-run' scripts/switch-model.sh | head -12
  ```
- [ ] **Step 208** [DECIDE][DESIGN] ~3m: Persistence path
  - RECOMMENDATION: the CLI talks to agentd (`GET /api/v1/memory/recall?q=`, `POST /api/v1/memory/remember`), and agentd owns the Well access (it already has `WELL_APP_ZHEN_DSN`); the CLI gains no database driver. Rationale: ADR-059's objection was lib/pq in the CLI; agentd is the one process that should hold the DSN. Override ONLY if agentd is absent on a host that runs the CLI (then the CLI says so, as it does for `/health`).
- [ ] **Step 209** [CODE][W] ~10m: Red tests (`cmd/zhen-agentd`): recall returns ranked rows from a fake store; remember writes one `zhen_memories` row through the Champion gate (`memory_write` registered mutating → confirmation unless `direct-user` from an authenticated UI/tty principal). Red tests (`cmd/zhen-cli`): `/recall x` prints rows with provenance; `/remember text` prints the stored id; `/confirm <tok>` posts to `/api/v1/agent/confirm` with the tty principal and prints the result; `/model` lists models from `switch-model.sh --json` and `/model <key>` goes through agentd's `model_switch` (never the script directly). Green in both binaries.
- [ ] **Step 210** [CODE][W] ~5m: `scripts/switch-model.sh --json`: machine-readable status/list/decision (`{"active":…,"models":[…],"fit":{…}}`), no behaviour change otherwise; golden test `cmd/zhen-agentd/modelswap_json_test.go` parses a recorded sample (ADR-060:178 mitigation).
- [ ] **Step 211** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-cli/... ./cmd/zhen-agentd/... ./pkg/champion/... -race -count=1 2>&1 | tail -4 && bash -n scripts/switch-model.sh && scripts/switch-model.sh --json --status | jq -e '.active' >/dev/null && echo "json ok"
  ```
- [ ] **Step 212** [V] ~1m: PASS; `jq` parses.
- [ ] **Step 213** [CODE][W] ~6m: Line editing without a new dependency: `golang.org/x/term` is gone from the build (Step M-001 shrank it); implement history + basic editing over `bufio` with ANSI arrows (left/right/up/down, home/end, backspace) in `cmd/zhen-cli/lineedit.go` with a table test; fall back to plain `bufio.Scanner` when stdin is not a TTY. API key: `ZHEN_API_KEY` env or `~/.config/unheaded/zhen-cli.conf` (0600, fail-loud if world-readable), sent as `Authorization: Bearer`.
- [ ] **Step 214** [TEST][B] ~2m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-cli/... -race -count=1 2>&1 | tail -2 && go mod tidy -diff | head -3; echo "tidy checked"
  ```
- [ ] **Step 215** [V] ~1m: PASS; `go mod tidy -diff` empty (no new module).
- [ ] **Step 216** [CODE][W] ~6m: ADR-060 tail: agentd exports `zhen_model_swap_duration_seconds` histogram and `zhen_model_swap_failures_total`; `TestModelSwap_TimeoutRollsBack` (fake script that never opens the port → after 4x documented load time the previous model is restored, T19); `tomb/lich/LICH-013-model-selector/CAMPAIGN.md` gets the "how to run" block pointing at `cmd/zhen-agentd` model_switch with the fake script.
- [ ] **Step 217** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-agentd/ -run 'ModelSwap' -race -count=1 2>&1 | tail -3 && go test ./pkg/champion/ -run 'Modelswap|ModelSwap' -race -count=1 2>&1 | tail -2
  ```
- [ ] **Step 218** [B] ~2m: Merge
  ```bash
  cd /home/govan/tmp/unheaded && git merge --no-ff sprint/champion-hardening-cli -m "merge Phase 11: zhen-cli 2b/3, switch-model --json, ADR-060 tail" && go build ./... && go test ./cmd/zhen-cli/... ./cmd/zhen-agentd/... -race -count=1 2>&1 | tail -3
  ```
- [ ] **Step 219** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 207-218: zhen-cli recall/remember/confirm/model, switch-model --json, swap metrics + rollback test`
- [ ] **Step 220** [V] ~1m: **PHASE 11 EXIT GATE** — Steps 212, 215, 217, 218 green; ADR-059 Phase 2b/3 and ADR-060 tail items ticked in their ADRs ([DOC-UPDATE] one line each).

---

## PHASE 12: KANBAN SEES RUNBOOKS AND AGENT RUNS (Steps 221-230)

**Goal**: the Kanban board shows a Runbooks view (create a task from a runbook; per-step progress from Wotan), the card modal shows `branch` and `review_note` from `zhen_agent_runs`, and ADR-038's trailer hook is marked dormant. Cards still come only from The Well. Cites M-128 (ADR Z-07, Z-09; PLANS F-22).
**Prerequisite**: Phases 2, 8.
**Time**: 2 h
**Agent**: Agent A

- [ ] **Step 221** [R] ~5m
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'func \(.*\) handle|HandleFunc\("/api' cmd/kanban-app/main.go | head -30 && ls cmd/kanban-app/data | head && grep -nE 'modal|review_note|branch' kanban/*.js kanban/*.html 2>/dev/null | head -8; ls kanban | head
  ```
- [ ] **Step 222** [DECIDE][DESIGN] ~3m: ADR-038 trailer hook
  - RECOMMENDATION: mark ADR-038 Dormant (status line: "Accepted, dormant 2026-10-xx: the `zhen_agent_runs.branch` + card id link (migration 032) supersedes the commit-trailer hook; revive only if human commits need the link"). Rationale: every agent commit already lands on a `zhen/<card>` branch recorded in the Well; a hook on Stevie's commits would add friction for a link the board already has. Override ONLY if Stevie wants trailers on human commits.
- [ ] **Step 223** [CODE][W] ~10m: Red (Go, `cmd/kanban-app`): `GET /api/v1/runbooks` lists runbooks from `runbooks/MANIFEST.sha256` + front-matter (name, category, status, risk); `POST /api/v1/tasks/from-runbook` creates a card in the Well (`kanban_tasks`, type `runbook`, `metadata.runbook=<name>`) through the existing store (pg in prod, memory in tests), never a hardcoded card; `GET /api/v1/tasks/{id}` includes `agent_run: {branch, review_note, mode, finished_at}` joined from `zhen_agent_runs` when present. Green.
- [ ] **Step 224** [CODE][W] ~8m: UI (`kanban/`, vanilla JS): a "Runbooks" tab listing `GET /api/v1/runbooks` with a "Create task" button; card modal gains a "Agent run" block (branch as text, review_note, link `git log zhen/<branch>` as a copyable command, not a link to GitHub); per-step progress: subscribe to the existing `zhen.audit.toolcalls` stream the dashboard already consumes (check `grep -rn 'zhen.audit' dashboard/js | head -3`) filtered by `runbook=<name>`, rendering `step k/n`. `scripts/check-js-lint.sh` passes.
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-js-lint.sh 2>&1 | tail -2
  ```
- [ ] **Step 225** [TEST][B] ~3m
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/kanban-app/... -race -count=1 2>&1 | tail -3 && scripts/check-kanban-genesis.sh 2>&1 | tail -1
  ```
- [ ] **Step 226** [V] ~1m: PASS; the Genesis check still passes (no card was hardcoded; memory feedback_kanban_cards_from_well).
- [ ] **Step 227** [LIVE-RO][B] ~3m: If the kanban container is up on west, read-only smoke; else skip (not STUCK)
  ```bash
  curl -s --max-time 3 http://127.0.0.1:20001/api/v1/runbooks | jq 'length' 2>/dev/null || echo "kanban not running here (ok)"
  ```
- [ ] **Step 228** [DOC-UPDATE][W] ~4m: `wiki/Kanban.md` (or the board's wiki page): Runbooks tab, agent-run block, "cards come from The Well" sentence; ADR-038 status per Step 222; ADR-106 §6 pointer.
- [ ] **Step 229** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 221-228: Kanban runbooks view and agent-run block; ADR-038 dormant`
- [ ] **Step 230** [V] ~1m: **PHASE 12 EXIT GATE** — Steps 224 lint, 225, 226 green.

---

## PHASE 13: THE FIRST UNATTENDED NIGHT, PREPARED (Steps 231-236)

**Goal**: everything for ADR-106 Q2 is ready so that one answer from Stevie starts it: the env file template, the cap, the card filter, the kill switch, the morning report. Cites M-126.
**Prerequisite**: Phases 2, 5, 8, 12; Well migrations 030-034 applied live (S-WELL-SPRING M-070) OR the in-memory worker path.
**Time**: 45 min + E2
**Agent**: Coordinator

- [ ] **Step 231** [R] ~4m
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'overnight|gpu-lease|mem-floor|daily-cap|cards?-filter|kill|ABORT' cmd/zhen-agentd/main.go | head -14 && grep -nE 'Step 254|unattended' docs/adr/ADR-106-two-brains-one-gate.md | head -4
  ```
- [ ] **Step 232** [W] ~5m: `deploy/systemd/zhen-agentd.env.example` gains the overnight block (commented): `ZHEN_OVERNIGHT=1`, `ZHEN_CLAUDE_DAILY_CAP_USD=5`, `ZHEN_CARD_FILTER=label:zhen,status:ready`, `ZHEN_MEM_FLOOR_MB=…`, `ZHEN_ABORT_FILE=/run/unheaded/zhen/ABORT`; `runbooks/observe/zhen-overnight-abort.yaml` (draft) documents the kill switch (`touch` the abort file, `systemctl stop zhen-agentd`); `scripts/zhen-morning-report.sh` prints last night's `zhen_agent_runs` rows (via agentd `GET /api/v1/runs?since=`), spend, cards moved, branches to review.
  ```bash
  cd /home/govan/tmp/unheaded && bash -n scripts/zhen-morning-report.sh && scripts/check-runbooks.sh 2>&1 | tail -1
  ```
- [ ] **Step 233** [TEST][B] ~5m: Dry overnight for 2 minutes against the fake Champion/in-memory stores (no model, no Claude): the worker picks a fixture card, writes a branch in a temp worktree, records a run, respects the abort file.
  ```bash
  cd /home/govan/tmp/unheaded && go test ./cmd/zhen-agentd/ -run 'Overnight|Worker' -race -count=1 2>&1 | tail -3
  ```
- [ ] **Step 234** [V] ~1m: PASS (these tests exist from S-TWO-BRAINS; if any is skipped for a missing Well, it stays skipped here: the live night is Stevie's call).
- [ ] **Step 235** [ESCALATE] ~2m: **E2: the first unattended night**
  - Context: Step 232 env template; Phase 2 made confirmations requester-bound, Phase 8 made the scheduler part of the same unit, Phase 6 approvals decide which runbooks the night may execute.
  - Questions: which night; `-daily-cap-usd` (default 5); which cards (filter); whether migrations 030-034 are applied first (S-WELL-SPRING M-070) or the night runs memory-only (branches still land, runs are not recorded in the Well).
  - Why Stevie: it spends money and writes branches while nobody watches.
- [ ] **Step 236** [V] ~1m: **PHASE 13 EXIT GATE** — Steps 232 checks and 234 green; E2 filed with the date. The night itself is executed by the on-call session following `runbooks/observe/zhen-overnight-abort.yaml` and reported with `scripts/zhen-morning-report.sh`; its result is appended to this file's progress ledger.

---

## PHASE 14: SECURITY AND COMPLIANCE REVIEW (Steps 237-244)

**Goal**: the attack-surface delta of this sprint is written down and every gate that guards it is re-run. [SECURITY]
**Prerequisite**: Phases 1-12 merged.
**Time**: 1 h
**Agent**: Coordinator

- [ ] **Step 237** [SECURITY][W] ~10m: Attack-surface table appended to `docs/security/S-CHAMPION-HARDENING-review.md`: rows for the trust allowlist (RT-06 closed), requester-bound tokens (RT-03), gc ticker (RT-04), write cap (RT-05), openat2 sandbox (T-05), dispatch truth (RT-09; `system_command` removed), audit fail-closed (RT-10), shared prompt + verbs (RT-07/08), runbook manifest, model pins (RT-15), scheduler endpoints (new mutating tool `schedule_update`), memory endpoints (new mutating tool `memory_write`), MCP in Go (same nine tools; stdio only), vor unit (loopback only, own user), kanban endpoints (`/api/v1/runbooks` read-only; `from-runbook` write through the store with auth middleware). For each: new listener? new privilege? new dependency? fuzz target touched?
- [ ] **Step 238** [SECURITY][TEST][B] ~5m: Fuzz the three champion targets briefly
  ```bash
  cd /home/govan/tmp/unheaded && for f in FuzzTrustLabel FuzzCanonicalCall; do go test ./pkg/champion/ -run '^$' -fuzz $f -fuzztime 45s 2>&1 | tail -1; done
  ```
- [ ] **Step 239** [SECURITY][B] ~4m: Gates
  ```bash
  cd /home/govan/tmp/unheaded && scripts/check-gitleaks.sh 2>&1 | tail -1 && scripts/check-gosec-ratchet.sh 2>&1 | tail -1 && scripts/check-dependency-baseline.sh 2>&1 | tail -1 && scripts/check-systemd-units.sh 2>&1 | tail -1 && scripts/check-unit-binaries.sh 2>&1 | tail -1 && scripts/check-runbooks.sh 2>&1 | tail -1 && scripts/check-prometheus-rules.sh 2>&1 | tail -1 && scripts/check-manifest-yaml.sh 2>&1 | tail -1 && scripts/check-python-syntax.sh 2>&1 | tail -1 && scripts/check-js-lint.sh 2>&1 | tail -1
  ```
- [ ] **Step 240** [V] ~1m: Every gate PASS. If gosec grew: fix or `#nosec` with a leading justification (memory reference_gosec_nosec_directive).
- [ ] **Step 241** [SECURITY][B] ~3m: SPDX on every new file
  ```bash
  cd /home/govan/tmp/unheaded && for f in $(git diff --name-only --diff-filter=A a7fe3acd -- '*.go' '*.py' '*.sh' '*.yaml' '*.service'); do head -3 "$f" | grep -q 'SPDX-License-Identifier' || echo "MISSING SPDX: $f"; done; echo "spdx checked"
  ```
- [ ] **Step 242** [COMPLIANCE][B] ~2m: No new module
  ```bash
  cd /home/govan/tmp/unheaded && git diff a7fe3acd -- go.mod | grep -E '^\+' | grep -vE '^\+\+\+' || echo "go.mod: no additions"
  ```
- [ ] **Step 243** [TEST][B] ~8m: Full regression against the baseline
  ```bash
  cd /home/govan/tmp/unheaded && go test ./... 2>&1 | grep -E '^(FAIL|---)' | tee -a docs/battle-plans/progress/S-CHAMPION-HARDENING-progress.md | tail -5; echo "regression recorded"
  ```
- [ ] **Step 244** [V] ~1m: **PHASE 14 EXIT GATE** — zero new FAIL lines vs Step 5; Steps 240-242 clean. Commit `[PLAN S-CHAMPION-HARDENING] Steps 237-243: security review, gates green`.

---

## PHASE 15: DOCS, REGISTERS, HANDOFF (Steps 245-252)

**Goal**: downstream docs tell the truth; the findings register and the master list are updated; the handoff is written. [DOC-UPDATE]
**Prerequisite**: Phase 14.
**Time**: 1 h
**Agent**: Coordinator (Librarian lens)

- [ ] **Step 245** [DOC-UPDATE][W] ~8m: CLAUDE.md: the "Zhen MCP Server" paragraph (Go server, `cmd/zhen-mcp`, same nine tools, Python retired), the runbooks line (approval policy pointer), scheduler mention; `wiki/Zhen-Agents.md` (requester-bound confirmations, scheduler mode, overnight env block); `wiki/Zhen-AI.md` (already touched in Phases 4, 9); `wiki/Runbooks.md`; `wiki/Service-Isolation.md` (vor); `wiki/Port-Registry.md`.
- [ ] **Step 246** [AUDIT-UPDATE][W] ~5m: `docs/sweeps/OUTSTANDING-2026-10-27.md` E09 rows M-120..M-132: status column → DONE with the step numbers, or the exact remainder (E1/E2/E3/E4 if still open); `docs/security/` red-team register rows RT-01..RT-16 that this sprint closed → "closed S-CHAMPION-HARDENING Step n".
- [ ] **Step 247** [REGEN][B] ~3m: ADR-098 findings register section
  ```bash
  cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -2 && git diff --stat docs/adr/ADR-098-findings-register-and-baseline-modes.md | tail -1
  ```
- [ ] **Step 248** [DOC-UPDATE][W] ~5m: `references/timeline.md`: one dated entry under Age 3 ("S-CHAMPION-HARDENING: Champion tool path closed RT-03..RT-10, runbooks approved under a written policy, eval scores runbook-draft, scheduler/vor/MCP under units"), then regenerate mirrors
  ```bash
  cd /home/govan/tmp/unheaded && go run ./services/timeguru/cmd/timeguru -sync-once 2>&1 | tail -2 || echo "use the timeguru sync path recorded in Appendix C"; git diff --stat references/timeline.* | tail -1 && scripts/check-timeline-freshness.sh 2>&1 | grep Status
  ```
- [ ] **Step 249** [DOC-UPDATE][W] ~6m: ADR statuses touched: ADR-024 (amended), ADR-028 (superseded), ADR-038 (dormant), ADR-059 (Phase 2b/3 done), ADR-060 (tail done), ADR-106 (Q2/Q3/Q6/Q7 answered or open with E-ids), ADR-019 (amendment per E3); `docs/adr/ADR-INDEX.md` rows.
- [ ] **Step 250** [W] ~6m: `~/tmp/next.md` handoff block at the top: what shipped, the four escalations and their state, the live state on west (units installed or not), the morning-report command, what S-TRUE-WIRE (Sprint 5) expects from here (nothing hard: the MCP transport in `pkg/mcpserver` is reusable by the Lich).
- [ ] **Step 251** [C] ~1m: **COMMIT** `[PLAN S-CHAMPION-HARDENING] Steps 245-250: docs, registers, timeline, handoff` then the SHIPPED banner at the top of this file (date, commit range, STUCK list, open E-ids) in a final commit.
- [ ] **Step 252** [V] ~1m: **PHASE 15 EXIT GATE / SPRINT DoD** — DoD-STD 1-8 hold for every phase; `git status --short` empty (ignoring the two untracked host-agent paths); Appendix D has no M-id without a step or an explicit deferral; this file carries the SHIPPED banner.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. The coding gate fails after a prompt change (Steps 91, 109).**
1. `diff <(go run ./pkg/zhenrag/cmd/dump -mode chat) /tmp/claude-1000/-home-govan-tmp/f267c16f-13e4-4b29-87ee-e434cae4bce1/scratchpad/p_py.txt` (add the `dump` helper if absent: it prints `SystemPrompt(mode)`).
2. Restore wording until byte-identical to the pre-sprint Python prompt; the sprint changes WHERE the prompt lives, not WHAT it says.
3. Re-run `make coding-gate-ci`. Two failures → STUCK; do not loosen `eval/coding-gate` thresholds.

**A2. `switch-model.sh` refuses to start (fit check rc=3).**
1. `scripts/switch-model.sh --status`; if a llama-server is still exiting, `scripts/switch-model.sh --stop; sleep 5`.
2. `free -m | head -2`; never count swap; never bypass the fit check (memory feedback_no_oom_from_ui_actions).
3. If a sha256 mismatch is reported: the model file changed on disk; re-hash with Step 100 and decide whether the pin or the file is wrong. Do not use `--no-verify` in a plan step.

**A3. `openat2` returns ENOSYS or EINVAL in tests (Step 72).**
1. `uname -r` (needs ≥ 5.6; west/east run 6.17). On a CI runner that lacks it, the race test must `t.Skip` on `ENOSYS` only.
2. `EINVAL` means an unsupported `Resolve` flag: drop `RESOLVE_NO_MAGICLINKS` first, keep `RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS`.

**A4. Worktree merge conflicts in `pkg/champion/toolcall.go` (Steps 106, 121).**
1. Keep Phase 1's `TrustedSourceLabels` AND Phase 4's import of `zhenrag.DestructivePattern`.
2. `go test ./pkg/champion/... -race -count=1`; the two fuzz seeds must still pass.

**A5. zhen-agentd unit fails after Step 173.**
1. `journalctl -u zhen-agentd -n 40 --no-pager`.
2. Missing DSN → `-action-store=stderr -kanban-store=memory` in `/etc/zhen-agentd.env` until S-WELL-SPRING applies 030-034.
3. `ReadWritePaths` → add `/etc/unheaded/zhen` (read) and `/var/zhen/projects` (write).
4. Two attempts → `sudo systemctl revert zhen-agentd && sudo systemctl restart zhen-agentd` with the previous binary (`/usr/local/bin/zhen-agentd.bak` if Step 173 was run with a backup; add `cp` before `install` if not).

**A6. vor unit cannot read its index (Step 185).**
1. `ls -la /var/lib/unheaded/vor | head`; ownership must be `unheaded-vor`.
2. If the index lived under `~/.local/share/vor` with another layout, re-run `vor index` as the unit user: `sudo -u unheaded-vor /usr/local/bin/vor index --out /var/lib/unheaded/vor <corpus path from raft/start-zhen.sh>`.
3. Slow first query → warm once; `MemorySwapMax=0` keeps it resident afterwards.

**A7. The runbook pytest suite fails after the manifest (Step 97).**
1. The dry-run path must ignore the manifest (`dryRun==true` short-circuits `Verify`); check Step 94's test.
2. `scripts/runbook-manifest.sh --write` after any runbook edit, then `--check`.

**A8. Confirm flow breaks `zhen-claude confirm` (Step 43).**
1. `zhen-claude confirm` must send `queue:stevie` as the principal; grep `pkg/claudebridge/confirm.go` for where `approvedBy` is set and update it in the same commit as Step 43.
2. `go test ./pkg/claudebridge/ -run Confirm -count=1`.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel with | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Coordinator | — | S-CLEAN-SLATE | 45 m |
| 1 Trust allowlist | A | 4, 5 | 0 | 1.5 h |
| 2 Tokens/gc/cap/dispatch/audit | A | 4, 5 | 1 | 2.5 h |
| 3 openat2 sandbox | A | 4, 5 | 2 | 1.5 h |
| 4 pkg/zhenrag, manifest, pins | B (wt zhenrag) | 1-3, 5 | 0 | 3 h |
| 5 Routers, caps, E3 | C (wt claudebridge) | 1-4 | 0 | 1.5 h |
| 6 Runbook policy, E1 | Coordinator | 7 | 4 | 1 h + E1 |
| 7 Eval + verification | A | 6 | 4 | 2 h |
| 8 Scheduler, E4 | Coord + A | 10, 11 | 2, 6 | 2.5 h + E4 |
| 9 vor unit, provenance | Coordinator | 10, 11 | 8 | 1.5 h + E4 |
| 10 Go MCP | D (wt mcp) | 8, 9, 11 | 2, 4 | 3 h |
| 11 zhen-cli, ADR-060 tail | E (wt cli) | 8, 9, 10 | 2, 4 | 2.5 h |
| 12 Kanban | A | — | 2, 8 | 2 h |
| 13 Unattended night prep, E2 | Coordinator | — | 2, 5, 8, 12 | 45 m + E2 |
| 14 Security review | Coordinator | — | 1-12 | 1 h |
| 15 Docs, handoff | Coordinator | — | 14 | 1 h |

**Critical path**: 0 → 1 → 2 → 3 → (merge 4, 5) → 6 → 7 → 8 → 9 → 12 → 13 → 14 → 15 ≈ 19 h of execution; Phases 4, 5, 10, 11 run beside it.

**Dependency map (escalations)**: E1 blocks only Steps 137-141 (approvals); E2 blocks the live night (after Step 236); E3 blocks Steps 118-119; E4 blocks Steps 173-175 and 184-188. No escalation blocks a phase exit gate: every gate is defined on the code and test steps.

**Skip Protocol acknowledgment**: the executing agent confirms at Step 1 that it will apply 3x-time / 2-attempt stuck detection, write STUCK markers into this file, commit before skipping, and produce a STUCK REPORT at the end if any step was skipped.

---

## APPENDIX C: QUICK REFERENCE

**Paths**
- Champion: `pkg/champion/{toolcall,confirm,confirmcall,dispatch,write,champion,branchsandbox}.go`; new: `chain.go`, `safeopen_linux.go`, `runbookmanifest.go`, `trust_fuzz_test.go`
- Shared prompt: `pkg/zhenrag/` (new); generated Python: `raft/scripts/zhen_prompt_generated.py`
- Agentd: `cmd/zhen-agentd/{main,toolexec}.go`; new `scheduler.go`; unit `deploy/systemd/zhen-agentd.service` (+ `.env.example`)
- MCP: `cmd/zhen-mcp/` (new), `pkg/mcpserver/` (new, extracted from `cmd/lich-mcp`), launcher `scripts/zhen-mcp.sh`, registration `.mcp.json`
- CLI: `cmd/zhen-cli/main.go` (+ `lineedit.go`)
- Eval: `cmd/zhen-eval/main.go:307`; `eval/champion-runbooks/{PREREG.md,RESULTS-2026-10-26.md}` (read-only)
- Runbooks: `runbooks/**/*.yaml`, `runbooks/MANIFEST.sha256` (new), `scripts/check-runbooks.{sh,py}`, `scripts/run-runbook.py`, `scripts/runbook-manifest.sh` (new), tests `tests/runbooks/`
- Schedules: `configs/zhen-schedules.yaml` (new) → `/etc/unheaded/zhen/schedules.yaml` on west
- vor: `/usr/local/bin/vor` (from `~/tmp/projects/vor`), unit `deploy/systemd/unheaded-vor.service` (new), index `/var/lib/unheaded/vor`
- Model pins: `scripts/lib/model-pins.sh` (new), `scripts/switch-model.sh`
- Kanban: `cmd/kanban-app/main.go`, UI `kanban/`

**Ports** (`pkg/ports/ports.go`): zhen-agentd 20105 (check SophiaEye collision, Step 9/179), zhen-web-ui 20103, kanban 20001, vor `ZhenVor` (new, 2010x), llama-server 8081 (local)

**Users** (`deploy/sysusers.d/unheaded.conf`, `pkg/uids/uids.go`): `zhen-agentd` (exists), `unheaded-vor` (new, next free uid after 16789)

**Env**: `AUTH_ENABLED`, `WELL_DSN`, `WELL_OPS_DSN`, `WELL_APP_ZHEN_DSN`, `WOTAN_ADDR`, `ZHEN_CLAUDE_DAILY_CAP_USD`, `ZHEN_SCHEDULES`, `VOR_URL`, `ZHEN_MCP_ALLOW_WRITE`, `ZHEN_MCP_ALLOW_EXEC`, `ZHEN_MCP_PROJECT_ROOT`, `ZHEN_API_KEY`, `ZHEN_EMBEDDER_SHA256`; webapp: `ZHEN_DB_PASSWORD` from `.env` `APP_ZHEN_PASSWORD`

**Commands**
- Tests: `go test ./pkg/champion/... -race -count=1`; `~/.venv/zhen/bin/python3 -m pytest tests/runbooks -q -m "not live"`; `~/.venv/zhen/bin/python3 -m pytest raft/tests -q`
- Coding gate: `make coding-gate-ci` (needs llama-server via `scripts/switch-model.sh qwen-7b`)
- Fuzz: `go test ./pkg/champion/ -run '^$' -fuzz FuzzTrustLabel -fuzztime 60s`
- Webapp restart: S21 recipe (Step 186)
- Register regen: `go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md`
- Timeline mirrors: timeguru sync (`services/timeguru/cmd/timeguru`, see `internal/sync/sync.go`); freshness `scripts/check-timeline-freshness.sh`

**The eleven draft runbooks (E1)**: `data/well-restore-from-east-offsite`, `deploy/branch-promotion-develop-staging-main`, `deploy/resign-unpushed-commits`, `network/air-gap-egress-validation`, `network/bgp-bfd-session-down`, `observe/chaos-game-day`, `pki/ca-compromise-root-rotation`, `security/claude-api-key-rotation-spend-breach`, `security/git-history-secret-purge`, `security/mcp-write-exec-enablement`, `wotan/active-active-cutover` (three more carry a status that is not draft: `network/dns-pihole-lxd`, `wotan/oom-state-recovery`, `wotan/pending-subscription-approval`; verify with Step 125).

**Trust labels**: `canonical`, `local`, `direct-user` only. **Principals**: `ui:<subject>`, `tty:<user>`, `queue:stevie`, `agent:<model>` (never confirms).

---

## APPENDIX D: COVERAGE (master list → steps)

| M-id | Item | Steps | Status at forge |
|---|---|---|---|
| M-120 | 11 runbooks approved; authorship policy | 125-142 | E1 |
| M-121 | Trust-label exact allowlist + chain verifier + fuzz | 23-40 | — |
| M-122 | Tokens bound, gc ticker, write cap, dispatch truth, audit failures | 41-66 | — |
| M-123 | One prompt, one verb list, runbook hash, model pins | 81-110 | — |
| M-124 | openat2 RESOLVE_BENEATH sandbox writes | 67-80 | — |
| M-125 | Routers, cap headroom, ADR-019 L2 amendment | 111-124 | E3 |
| M-126 | First unattended night prepared | 231-236 | E2 |
| M-127 | zhen-eval runbook-draft; run-runbook verification | 143-158 | — |
| M-128 | Kanban runbooks view, agent-run block, ADR-038 dormant | 221-230 | — |
| M-129 | zhen-cli 2b/3; ADR-060 tail | 207-220 | — |
| M-130 | Scheduler folded into agentd, unit, heartbeat | 159-176 | E4 |
| M-131 | vor unit + registries, consolidation scheduled, provenance, strip test | 177-190 | E4 |
| M-132 | Go MCP server, Python retired, MCP in CI, isError | 191-206 | — |
| decision 30 | Runbook approvals + author policy | 127, 135 | E1 |
| decision 31 | Q2 night, Q6 amendment, Q7 routers, caps | 112, 114, 117, 235 | E2/E3 |
| decision 32 | Bandit groups | deferred to S-CLEAN-SLATE (M-165 lives in E11; the zhen UI `0.0.0.0` bind is answered by M-036 there) | — |
| M-019 | agentd auth/rate defaults | S-CLEAN-SLATE (E02) | — |
| M-036/M-037 | UI→agentd flip; fit check | S-CLEAN-SLATE (E03) | — |
| M-042 | Routed local turns captured | S-CLEAN-SLATE (E03) | — |
| M-070 | Migrations 030-034 live | S-WELL-SPRING (E06) | — |

---

*S-CHAMPION-HARDENING Battle Plan — Forged 2026-10-27*
*16 Phases. 252 Steps. The Champion stops trusting labels, tokens and prompts it did not verify, and the pieces around it get owners.*
