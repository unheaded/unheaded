<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-RESIDUE BATTLE PLAN: 10 Phases, 176 Steps

**Date**: 2026-11-12
**Sprint**: S-RESIDUE. Clear the small leftovers of the 2026-11-04 sprints and the record drift in one pass (backlog sweep bundle 6: rows D-19, D-26, D-28 to D-32).
**Prerequisite**: develop at `a31be33b` or later, 29+ commits ahead of origin (Stevie pushes). No answer from Stevie is needed except for Phase 8, which waits on llama-server being up, and the attended steps marked `[STEVIE]`.
**Target**: Every claim in rows D-19 (docs half), D-26 and D-28 to D-32 is either fixed with a guard that fails when it comes back, or marked dropped with the reason. The ruff gate on develop is green again. Phase 8 (the prompt half of D-19) is ready to run the day the model is up.
**Estimated duration**: 6 to 8 h over 2 sessions. Phase 8 adds about 2 h plus the eval run when llama-server is up.
**Agent strategy**: Phase 0 runs first and alone. Phases 1, 2, 3, 6 and 7 are docs and config and can run in parallel lanes, each with its own named files. Phases 4 and 5 are Go TDD and can run in parallel with each other. Phase 9 runs last. Phase 8 is gated and runs whenever its gate opens.
**Commit cadence**: every 5 steps, at every exit gate and before every skip. Signed (`git commit -S`; if gpg-agent has timed out and Stevie is away, `--no-gpg-sign` and note it for `scripts/resign-unpushed.py`). Named files only, never `git add -A`. Commits go on develop. Stevie pushes.
**Stuck protocol**: skip after 3× the step estimate or two failed debug attempts. Mark the step `[STUCK]` and its dependents `[BLOCKED]`, commit, and move to the next independent step.

## Grounding (read before Step 1)

Every row was checked against the tree at `a31be33b` on 2026-11-12 by reading the cited file:line. What changed against the sweep's wording:

| Row | Sweep says | Found at `a31be33b` | In this plan |
|---|---|---|---|
| D-19 | about 30 tracked files say `~/.config/cs` | Live (non-dated) hits: `cmd/zhen-agent/README.md` (7 lines), `raft/CLAUDE.md:69`, `raft/start-zhen.sh:77`, `raft/zhen_app.py:2146`, `raft/scripts/zhen_rag.py:1081,1096`, `scripts/coding-gate-probe.sh:41` (functional), `scripts/run-coding-gate.sh:13`, `scripts/zhen-agent-preflight.sh:14,71-73` (functional), `pkg/champion/toolcall.go:191`, `pkg/champion/toolcall_test.go:46,53`, `eval/coding-gate/CONTRIBUTING.md:26`, `docs/security/application-threat-model.md:159`. Prompt half: `pkg/zhenrag/prompt.go:125,127,187,189`, three goldens in `pkg/zhenrag/testdata/prompts/`, and the generated `raft/scripts/zhen_prompt_generated.py:70`. Dated records are left as written: `eval/coding-gate/probe-2026-05-02/*`, `eval/coding-gate/baseline-*-2026-05-02.md`, `docs/security/threat-model-zhen-ai-2026-05-06.md`, every battle plan, `DECISION-PACK-2026-11-03.md`, ADR-056 and ADR-057 (decision records). `deploy/systemd/unheaded-vor.service:34` is correct (it explains the rename) | Phase 1 (docs), Phase 8 (prompt, gated) |
| D-19 | the prompt change re-runs the runbook eval (ADR-024) | ADR-024 line 204: re-run `cmd/zhen-eval` on `eval/champion-runbooks` when "the system prompt changes". `cmd/zhen-eval` uses its own `systemRunbook` text, not `pkg/zhenrag`, so this rerun follows the ADR's letter; it will not show the prompt edit. Stevie's answer stands (rerun) | Phase 8 notes it in the results file |
| D-26 | index rows 103, 108, 110 disagree with the files | Holds. Index: 103 "Proposed (design only)", 108 "Proposed", 110 "Proposed". Files: 103 "Implemented in code ... Stevie to accept", 108 "Accepted (2026-11-03, DP-603)", 110 "Accepted (2026-11-04; DP-717)" | Phase 2 |
| D-26 | timeline mirrors lack S-STAGE-2B and have no sync command | Holds, and one more: `references/timeline.md` lines 91 (S-STAGE-2B) and 92 (Audits answered, 11-05 → 11-07) are missing from JSON, YAML and TOML (last touched `7d7c8279`, 2026-10-28). Every other entry matches by id. The id is `phase-3-item-` + the first 8 hex of sha256 over the bullet text after `- `. Commit `7d7c8279` Step 150 says "mirrors by hand, no sync flag" | Phase 2 builds `scripts/timeline-mirrors.py` |
| D-28 | three orphaned `cmd/waf` lines in PROJECT_STRUCTURE | Holds: lines 73-75 (`└── src/`, `main.rs # WAF entry point`, `rules/`) sit under `cmd/unheaded/` | Phase 3 |
| D-28 | `unheaded hop` missing from completions | Holds, and wider: the hand-kept lists in `cmd/unheaded-cli/cmd/completion.go:87` (bash), the zsh and the fish blocks lack `hop`, `token`, `generate` and `observe`; root registers 13 commands (`root.go:112-124`). No wiki page names `unheaded hop` | Phase 3 adds a test that walks `NewRootCommand().SubCommands` |
| D-28 | `BPF_IPV6_INTERFACE_MAP.md` tables stale | Holds: line 106 `HMAC_KEYS` and 120 `HOP_VALIDATORS` say "Wire to Shield XDP" (they live in `ebpf/hop-ebpf/src/main.rs`); line 110 gives `RETRY_TOKENS` `MigrationTokenKey/Value` (the type is `RetryTokenKey`, `bpfschema.go:96`); line 121 `AUTHORITY` names a map that no longer exists in any source | Phase 3 |
| D-28 | circl v1.6.3 named where go.mod has v1.6.5 | go.mod:29 is v1.6.5. Live: `LICENSES/THIRD_PARTY.md:12,725`. Left as written: `CLAUDE.md:850` (a history line, true when written), ADR-004:128 (audit row of its date), every `docs/compliance/control-matrix/*-2026-05-06.md` (dated), `LICENSES/sbom-reports/go-modules-list.txt` (snapshot of 2026-03-19). The S-WIRE-HONEST-pq progress note asks for a licence re-check, not a search and replace | Phase 3 re-checks the licence at v1.6.5 |
| D-28 | `hqc.go` says "ROADMAP (Q3 2026)" | Holds: `pkg/crypto/pqc/hqc.go:18` | Phase 3 |
| D-28 | vLLM/ROCm text in `nixos/`, lxd counts, zhen-mcp "Six", `+dirty` man page, vector sysusers "(east)", east-bootstrap `root@` | All hold: `nixos/README.md:9`, `nixos/hosts/host-a/configuration.nix:3`; `lxd/LAUNCH_EXAMPLES.sh:19,93,126,172,185`, `lxd/Makefile:30,57` ("30 containers"); `cmd/zhen-mcp/main.go:10` (tools.go lists 8 read-only + 3 opt-in; `code_definition`, `code_callers` missing from the comment); `man/zhen-cli.1:1` (`v0.0.0-20261104152510-c303527bc1f5+dirty`, a Go pseudo-version, not what `scripts/gen-man.sh` stamps); `deploy/sysusers.d/unheaded.conf:33`; `scripts/east-bootstrap.sh:18,233` | Phase 3 |
| D-28 | `wiki/Microservices.md` calls Go `shield` a WAF; `wiki/Systemd-Units.md` lacks vor | Both hold (`Microservices.md:19`; no `vor` in `Systemd-Units.md`) | Phase 3 |
| D-29 | compliance fold residue (five items) | Four hold: `cmd/compliance-evidence/wire.go:98-102` (ring error printed, goroutine not joined); `no_data` absent from `handleMetrics` (`wire_compliance.go:331`) and `Stats()` (`:832`); `no_data` cleared only in `aggregateViolations` (`:629-639`); help text `:359` says "unavailable Wotan" (the counter counts events with no client configured, `:778-781`). **Dropped:** the package comment's "enforcement" (F5) is already fixed (line 9: "It enforces nothing"). **Conditional, not done:** the allow list and `auto_approve` for `wire` (ADR-127 line 37: no Wotan identity until it has its own account) | Phase 4 |
| D-29 | `TestShippedAllowlistCoversDeployedSubscribers` does not walk the compliance code | Holds, and the guard is weaker than that: its regex (`services/wotan/cmd/wotan/allowlist_test.go:34`) needs a quoted topic, so it skips every `Subscribe(ctx, topic, "...")` with a variable topic (dashboard-backend, kanban-app, trace-collector, compliance-service, firewall-service). The row names this test, so widening it is in scope; what it then finds is recorded, not fixed here | Phase 4 |
| D-30 | four Zhen residue items | All hold: `cmd/zhen-agentd/conversations.go:161` (`AppendTurns`, fake store only); `pkg/claudebridge/confirm.go:125` (`TTYConfirmer.Confirm` prompts before `champion.QueueOnlyTools` is checked); `pkg/codeindex/codeindex.go:308-330` (`Store.Get` returns a bare `fs.ErrNotExist` for a missing slot file, `pkg/retrieval/code.go:36` reads it as "no index"); `pkg/retrieval/code.go:42` (`ix.Search` scans with no context) | Phase 5 |
| D-31 | grimoire playbook deploys `files/grimoire-search.sh` | Holds (`tomb/ansible/playbook-grimoire.yml:40-47`, `failed_when: false`); `tomb/ansible/files/` holds only `kingdom-alerts.yml`; nothing in the live tree calls the script (only an archived S75 plan) | Phase 6 |
| D-31 | trace-collector.nix sets two unread variables | Holds: `TRACE_PORT`, `TRACE_LOG_LEVEL` (`nixos/modules/services/trace-collector.nix:41-42`); the Go binary reads only `UNHEADED_EBPF_OBJ_DIR` from the environment | Phase 6 |
| D-31 | sophia-eye: dead vLLM variable, embeddings-bge as root | Holds and wider: `VLLM_WORKER_MULTIPROC_METHOD` (`nix/containers/sophia-eye.nix:99`) is dead; `VLLM_ENDPOINT` (:156) is still read by sophia-gateway and stays. **Both** `sophia-gateway` (:168) and `embeddings-bge` (:189) run `User = "root"` (rule: every service its own nologin account) | Phase 6 |
| D-31 | no egress rule for llama-server in `shield-rules.yaml` | Holds; no code loads the file | `[BLOCKED]` on A-56 |
| D-32 | `ruff I001 at scripts/lib/sbom_licenses.py:18` | **Dropped:** ruff 0.17.3 passes that file. **Found instead:** the ruff gate is RED on develop: 12 findings in `tests/runbooks/test_ioc_audit.py` (RUF100 ×2, UP031 ×3, PIE804 ×7), from `601cd946` (2026-11-12 night) | Phase 0 (a red gate is fixed before anything else) |
| D-32 | `check-perf-regression` exits 2 on usage | **Dropped:** documented (`scripts/check-perf-regression.sh:18`, "2 usage"); CI and `ci-local.sh:239` call it only with a report, and the meta-gate lists it as proven on fixtures (`check-gates-can-fail.sh:1445`). Step 12 confirms the meta-gate does not report it | none |
| D-32 | ruff and pip-audit not on PATH on west | Holds. Both exist in the CI venv `/var/tmp/unheaded-sprint/civenv/bin/`. Installing tools on the host is Stevie's; the plan documents the PATH line and makes the INCONCLUSIVE message say where they are | Phase 7 |
| D-32 | `GRAFANA_ADMIN_PASSWORD` in `monitoring.env` read by nothing | Holds (`monitoring/docker-compose.yml` has no Grafana since ADR-117). Also stale: `wiki/Observability-Backends.md:25` says `monitoring/.env` needs it | Phase 7 (host line `[STEVIE]`) |
| D-32 | wiki unit `After=docker.service` only in a host drop-in | Holds: `/etc/systemd/system/unheaded-wiki-server.service.d/west.conf`; the repo keeps per-host drop-ins next to units (`deploy/systemd/unheaded-chaos-controller.service.d/west-packet-lab.conf`) | Phase 7 |
| D-32 | `s-clean-cut` worktree, `zhen_login_test.sh.obsolete-2026-11-12` | Both hold (`/home/govan/tmp/wt/s-clean-cut` at `b836ee8e`; `~/.local/state/unheaded/zhen_login_test.sh.obsolete-2026-11-12`) | Phase 7 |

ADRs read: ADR-024 (eval re-run trigger), ADR-103, ADR-108, ADR-110 (status lines), ADR-127 (`no_data`, no Wotan client), ADR-INDEX status summary (line 19, recounts recorded in prose). ADR-090/095 are D-24, not this plan.

**Marshal (scope).** In scope: the seven rows as grounded above. Admitted as part of a row: the ruff red (replaces D-32's dropped I001; develop must not stay red, rule "catch in develop"); `token`/`generate`/`observe` in the completions (same hand-kept list as `hop`, one test guards all); `sophia-gateway` as root (same file, same rule as `embeddings-bge`); widening the allowlist test's regex (the row names that test; anything it then finds is parked in the progress file, not fixed). Parked, out of scope: timeline entries for 2026-11-08 to 11-12 (Librarian's, not this plan; Phase 9 adds only this sprint's entry); other ADR-INDEX mismatches beyond 103/108/110 (listed in the progress file for the next sweep); the `wire` Wotan account (ADR-127, Stevie's acceptance); the A-56 egress rule; dated records.
**Busboy (coordination).** S-WATCH-TRUE is forged in parallel and touches the IOC audit (D-18) and the trace collector (D-17). Collision points: `tests/runbooks/test_ioc_audit.py` (this plan's Phase 0 lints it; land Phase 0 first, S-WATCH-TRUE rebases), `nixos/modules/services/trace-collector.nix` (this plan removes two dead variables; if S-WATCH-TRUE adds an environment variable to the Go binary, it adds it to the module after this lands), `docs/adr/ADR-INDEX.md` and `compliance/findings/register.yaml` (both plans may edit; whichever commits second rebases, never resolves by overwrite). Stevie's open items untouched here: the manifest seal, pushes, A-56, the Zhen and llama-server pause (Phase 8 waits on it), the host steps marked `[STEVIE]`.
**Micromanager (QA).** Each fixed row leaves a guard where one is cheap: a Go test for the completion lists, a pytest for the timeline mirrors (`tests/scripts/` runs in CI, `static-analysis.yml:269`), a Go test for the grimoire playbook's `files/` sources, unit tests first for every Go change (red, then green), `go test -race` on every touched package, `scripts/gen-man.sh` check, ruff green, and the meta-gate `--quick` at the end. A grep with an explicit allow list is the guard for D-19 (a new gate script would need a meta-gate provocation, which is more than the row is worth). Definition of done per row is in each phase's exit gate.

## Decisions taken (2026-11-12, Stevie, this session)

- A-19: `~/.config/cs` → **docs and `raft/` now; the prompt text, its goldens and the ADR-024 eval rerun later as a gated phase** (Phase 8).
- No other answer is needed. A-56 stays open; its step is `[BLOCKED]`.

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; it must pass before you go on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or edit a file (Edit or Write tool; never a heredoc for shell-bearing text, never `sed -i` on YAML the services read)
- `[R]`: read or inspect
- `[S]`: needs sudo
- `[P]`: can run in parallel with other `[P]` steps
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first (host files under `/etc`, a reboot)
- `[GATED]`: runs only when its gate condition is true
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol, or blocked on an open question

`~Nm` is the expected wall-clock time. Scratch files go in `/tmp/claude-1000/s-residue/` (west's `/tmp` is a 7.2G tmpfs; keep outputs small).

---

## PHASE 0: PRECONDITIONS AND A GREEN BASELINE (Steps 1–16)

**Goal**: The tree is on develop, the tools are found, and every gate this sprint touches is green before any change, including the ruff gate that is red today.
**Prerequisite**: none.
**Time**: 40 min.
**Agent**: Coordinator.

- [ ] **Step 1** [R] ~5m: Read this plan's Grounding section, `docs/sweeps/BACKLOG-SWEEP-2026-11-12.md` rows D-19, D-26, D-28 to D-32, and the "Night session 2026-11-12" section of `~/tmp/next.md`.
- [ ] **Step 2** [B] ~1m: Be on develop with a clean index.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git status -sb | head -5 && git log --oneline -1
  ```
- [ ] **Step 3** [V] ~1m: Branch is `develop`; nothing staged; the only untracked path is `demos/doom/` (Stevie's, never staged). Anything else staged or modified → STOP and ask; do not stash someone's work.
- [ ] **Step 4** [B] ~1m: Tools.
  ```bash
  for t in go cargo docker nix-instantiate python3 git; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done
  ls /var/tmp/unheaded-sprint/civenv/bin/ | grep -xE 'ruff|pip-audit|pytest' ; mkdir -p /tmp/claude-1000/s-residue
  ```
- [ ] **Step 5** [V] ~1m: All `ok`; `ruff`, `pip-audit`, `pytest` listed. Missing civenv → STOP (CI venv recipe: `reference_ci_replay_gotchas`); do not `pip install` on the host.
- [ ] **Step 6** [B] ~2m: Ruff gate, CI venv LAST on PATH (standing rule).
  ```bash
  cd /home/govan/tmp/unheaded && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin ./scripts/check-ruff.sh 2>&1 | grep -E '^[a-z].*:[0-9]+:[0-9]+:|Found|PASS|FAIL' | head -20
  ```
- [ ] **Step 7** [V] ~1m: Expect FAIL with 12 findings, all in `tests/runbooks/test_ioc_audit.py` (Grounding). Different findings → record them in the progress file and fix them in Step 8 the same way.
- [ ] **Step 8** [W] ~10m: Fix the 12 findings by hand in `tests/runbooks/test_ioc_audit.py`: drop the two unused `noqa: N802` directives (lines 48, 51) only if ruff's default rule set does not enable N802 (it does not: RUF100 says "non-enabled"); `%` formatting → f-strings (lines 70, 171, 239); `**dict(...)` kwargs → plain keyword arguments (lines 133, 142, 153, 159, 168, 228, 239). No `noqa`, no new exclusion in `check-ruff.sh`.
- [ ] **Step 9** [V] ~2m: Ruff green and the tests still pass.
  ```bash
  cd /home/govan/tmp/unheaded && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin ./scripts/check-ruff.sh 2>&1 | tail -3
  /var/tmp/unheaded-sprint/civenv/bin/python3 -m pytest tests/runbooks/test_ioc_audit.py -q 2>&1 | tail -3
  ```
  - Pass: check-ruff PASS, 36 passed.
  - Fail → Step 10.
- [ ] **Step 10** [D] ~5m: A test now fails → the f-string changed a value (a `%r` becomes `{x!r}`, not `{x}`). Diff the one assertion, fix, rerun Step 9. Second failure → `[STUCK]`, revert the file (`git checkout -- tests/runbooks/test_ioc_audit.py`), go on; Phase 9's ruff gate is then `[BLOCKED]`.
- [ ] **Step 11** [B] ~4m: Baseline of the Go packages this sprint touches.
  ```bash
  cd /home/govan/tmp/unheaded && go test -race -count=1 ./cmd/unheaded-cli/... ./cmd/compliance-evidence/... ./services/compliance/... ./services/wotan/cmd/wotan/... ./pkg/claudebridge/... ./pkg/codeindex/... ./pkg/retrieval/... ./pkg/champion/... ./cmd/zhen-agentd/... ./cmd/zhen-codeindex/... ./cmd/zhen-mcp/... ./pkg/zhenrag/... ./pkg/protocol/bpfschema/... ./pkg/uids/... 2>&1 | grep -vE '^ok|no test files' | tail -15
  ```
- [ ] **Step 12** [B] ~6m: Meta-gate quick baseline (expect the known INCONCLUSIVEs on west only).
  ```bash
  cd /home/govan/tmp/unheaded && PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin ./scripts/check-gates-can-fail.sh --quick 2>&1 | tail -25 > /tmp/claude-1000/s-residue/metagate-before.txt; grep -E 'FAIL|INCONCLUSIVE|perf-regression' /tmp/claude-1000/s-residue/metagate-before.txt
  ```
- [ ] **Step 13** [V] ~2m: Step 11 prints nothing (all `ok`). Step 12: no FAIL; `check-perf-regression` is not reported (confirms D-32's perf item is dropped). Any other red → record it in the progress file as inherited; fix it here only if it is in this plan's files, else note it for Stevie and go on.
- [ ] **Step 14** [W] ~3m: Create `docs/battle-plans/progress/S-RESIDUE-progress.md` with sections Decisions (A-19 as above), Step log, Parked, Stuck.
- [ ] **Step 15** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/unheaded && git add tests/runbooks/test_ioc_audit.py docs/battle-plans/progress/S-RESIDUE-progress.md && git commit -S -m "[PLAN S-RESIDUE] Steps 1-15: ruff green again (test_ioc_audit.py), progress file

  The ruff gate went red with 601cd946: 12 findings in tests/runbooks/test_ioc_audit.py.
  Fixed by hand, no noqa, no exclusion. 36 tests pass."
  ```
- [ ] **Step 16** [V] ~1m: **PHASE 0 EXIT GATE.** `git log -1 --show-signature | head -3` shows a good signature (or the no-sign note); ruff PASS; Go baseline green.

---

## PHASE 1: `~/.config/cs` → `~/.config/vor`, DOCS AND SCRIPTS (D-19, first half) (Steps 17–32)

**Goal**: No live doc, script or comment outside the prompt text names `~/.config/cs`; the two scripts that act on the path act on the new one.
**Prerequisite**: Phase 0.
**Time**: 45 min.
**Agent**: Agent [P].

- [ ] **Step 17** [B][P] ~1m: The live hit list.
  ```bash
  cd /home/govan/tmp/unheaded && git grep -n '\.config/cs\b' -- ':!docs/battle-plans' ':!docs/sweeps' ':!docs/adr' ':!eval/coding-gate/probe-2026-05-02' ':!eval/coding-gate/baseline-*' ':!docs/security/threat-model-zhen-ai-2026-05-06.md' ':!docs/archive' | cut -c1-120 > /tmp/claude-1000/s-residue/cs-before.txt; wc -l < /tmp/claude-1000/s-residue/cs-before.txt
  ```
- [ ] **Step 18** [V] ~1m: The list matches the Grounding row (plus the prompt half: `pkg/zhenrag/prompt.go`, `pkg/zhenrag/testdata/prompts/*`, `raft/scripts/zhen_prompt_generated.py`). A file not in the Grounding row → read it; dated record → add to the exclusions above and note it; live → edit it in Step 19-24.
- [ ] **Step 19** [W][P] ~8m: `cmd/zhen-agent/README.md` lines 60-81: `~/.config/cs/` → `~/.config/vor/` in all seven places, including the `mkdir`/`ln -s`/`echo > .trusted` block.
- [ ] **Step 20** [W][P] ~5m: `raft/CLAUDE.md:69`, `raft/start-zhen.sh:77`, `raft/zhen_app.py:2146` (the 410 message text; not a prompt), `raft/scripts/zhen_rag.py:1081` (comment) and `:1096` (the 410 message text, also not a prompt).
- [ ] **Step 21** [W][P] ~5m: The two scripts that act on the path. `scripts/zhen-agent-preflight.sh` lines 14, 71-73 (check and its fix hint) and `scripts/coding-gate-probe.sh:41` (`ls -la ~/.config/vor/sources/`). `scripts/run-coding-gate.sh:13` (comment).
- [ ] **Step 22** [W][P] ~3m: `pkg/champion/toolcall.go:191` (comment), `pkg/champion/toolcall_test.go:46,53` (comment and the synthetic `SourcePath`).
- [ ] **Step 23** [W][P] ~3m: `eval/coding-gate/CONTRIBUTING.md:26` (live contributor guide) and `docs/security/application-threat-model.md:159` (living model). Line 159 is inside a dated "Closure (commit `f9311d29`)" note → if the paragraph is dated, leave it and add one line under it: "Since vor's rename the directory is `~/.config/vor/sources/` (2026-11-12)."
- [ ] **Step 24** [B] ~2m: Scripts still parse; the champion test passes.
  ```bash
  cd /home/govan/tmp/unheaded && bash -n scripts/zhen-agent-preflight.sh scripts/coding-gate-probe.sh scripts/run-coding-gate.sh raft/start-zhen.sh && echo syntax-ok
  /var/tmp/unheaded-sprint/civenv/bin/python3 -m py_compile raft/zhen_app.py raft/scripts/zhen_rag.py && echo py-ok
  go test -race -count=1 ./pkg/champion/... 2>&1 | tail -2
  ```
- [ ] **Step 25** [V] ~1m: `syntax-ok`, `py-ok`, `ok`.
- [ ] **Step 26** [B] ~2m: Preflight finds the real directory on west.
  ```bash
  cd /home/govan/tmp/unheaded && ls -ld ~/.config/vor/sources && bash scripts/zhen-agent-preflight.sh 2>&1 | grep -i 'config/vor' | head -3
  ```
- [ ] **Step 27** [V] ~1m: The `~/.config/vor/sources/ exists` check passes. If it warns, `ls -la ~/.config/vor` and compare with next.md queue item 5; a missing dir is a host state finding, not a script bug: note it, do not create it here.
- [ ] **Step 28** [B] ~1m: Raft tests that read these files.
  ```bash
  cd /home/govan/tmp/unheaded/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python3 -m pytest tests -q -x 2>&1 | tail -3
  ```
- [ ] **Step 29** [V] ~1m: Same pass count as before the edit (record both in the progress file). A test pinned the old 410 text → update that assertion to the new text; nothing else.
- [ ] **Step 30** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/unheaded && git add cmd/zhen-agent/README.md raft/CLAUDE.md raft/start-zhen.sh raft/zhen_app.py raft/scripts/zhen_rag.py scripts/zhen-agent-preflight.sh scripts/coding-gate-probe.sh scripts/run-coding-gate.sh pkg/champion/toolcall.go pkg/champion/toolcall_test.go eval/coding-gate/CONTRIBUTING.md docs/security/application-threat-model.md && git commit -S -m "[PLAN S-RESIDUE] Steps 17-30: ~/.config/cs -> ~/.config/vor in docs, scripts and comments (D-19, first half)

  The prompt text in pkg/zhenrag and its goldens are Phase 8 (ADR-024 eval rerun, gated on llama-server).
  Dated records left as written."
  ```
- [ ] **Step 31** [B] ~1m: What is left.
  ```bash
  cd /home/govan/tmp/unheaded && git grep -ln '\.config/cs\b' -- ':!docs/battle-plans' ':!docs/sweeps' ':!docs/adr' ':!eval/coding-gate/probe-2026-05-02' ':!eval/coding-gate/baseline-*' ':!docs/security/threat-model-zhen-ai-2026-05-06.md' ':!docs/archive'
  ```
- [ ] **Step 32** [V] ~1m: **PHASE 1 EXIT GATE.** Step 31 prints exactly `pkg/zhenrag/prompt.go`, `pkg/zhenrag/testdata/prompts/agent.txt`, `pkg/zhenrag/testdata/prompts/zhen-rag.txt`, `pkg/zhenrag/testdata/prompts/zhen_rag_py.txt`, `raft/scripts/zhen_prompt_generated.py` (Phase 8's files; `docs/security/application-threat-model.md` too if Step 23 kept the dated paragraph). Anything else → back to Steps 19-23.

---

## PHASE 2: RECORD DRIFT, ADR INDEX AND TIMELINE MIRRORS (D-26) (Steps 33–60)

**Goal**: ADR-INDEX agrees with ADR-103, ADR-108 and ADR-110, and its summary counts are recounted; one first-party script writes and checks the timeline's JSON, YAML and TOML mirrors from `references/timeline.md`, and the two missing entries are in all three.
**Prerequisite**: Phase 0.
**Time**: 2 h.
**Agent**: Agent [P].

### ADR index

- [ ] **Step 33** [R][P] ~3m: Read the three status lines (ADR-103:8, ADR-108:8, ADR-110:8), the index rows (`docs/adr/ADR-INDEX.md` lines 165, 170, 172) and the Status Summary table with its prose at line 19.
- [ ] **Step 34** [B][P] ~3m: List every index-vs-file mismatch, for the record only (one-off, in scratch).
  ```bash
  cd /home/govan/tmp/unheaded && for f in docs/adr/ADR-[0-9]*.md; do n=$(basename "$f" | sed -E 's/^ADR-([0-9a-z]+)-.*/\1/'); fs=$(grep -m1 -iE '^\*\*Status\*?\*?:?' "$f" | sed -E 's/^\*\*Status\*?\*?:?\*?\*? *//' | awk '{print tolower($1)}' | tr -d '*:,;(.'); is=$(grep -m1 "^| \[$n\]" docs/adr/ADR-INDEX.md | awk -F'|' '{print $4}' | sed 's/\*//g' | awk '{print tolower($1)}' | tr -d ':,;(.'); [ -n "$is" ] && [ "$fs" != "$is" ] && echo "$n file=$fs index=$is"; done > /tmp/claude-1000/s-residue/adr-mismatch.txt; cat /tmp/claude-1000/s-residue/adr-mismatch.txt
  ```
- [ ] **Step 35** [V] ~2m: 103, 108 and 110 are in the list. Others go to the progress file's Parked section (Marshal: not fixed here). An empty list means the parser missed; read the three by eye instead.
- [ ] **Step 36** [W] ~5m: Index rows. 103: status cell → `**Implemented in code** (live rollout pending host windows; Stevie to accept)`. 108: `**Accepted** (2026-11-03, DP-603)`. 110: `**Accepted** (2026-11-04, DP-717)`. Leave each row's summary text alone.
- [ ] **Step 37** [W] ~10m: Recount the Status Summary by the method line 19 states (first word of each status, case and trailing punctuation ignored), update the table, and append to line 19's prose: "2026-11-12: ADR-108 and ADR-110 Proposed → Accepted, ADR-103 Proposed → Implemented (rows had not followed the files; S-RESIDUE)." Bump **Last updated** to the commit date.
- [ ] **Step 38** [V] ~3m: The table's counts sum to the Total line; Proposed is two lower than before, Accepted two higher, Implemented one higher.
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '/^## Status Summary/,/^## /p' docs/adr/ADR-INDEX.md | awk -F'|' '/^\| [A-Z]/ && $3 ~ /[0-9]/ {s+=$3} END {print "sum", s}'; grep -m1 '^\*\*Total:\*\*' docs/adr/ADR-INDEX.md | cut -c1-60
  ```
- [ ] **Step 39** [B] ~1m: The wiki mirror of the index, if it carries the rows.
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE '\b(103|108|110)\b' wiki/ADR-Index.md | head
  ```
- [ ] **Step 40** [W] ~3m: If Step 39 shows status cells for those three, make them match Step 36. If it only links to `docs/adr/ADR-INDEX.md`, nothing to do.
- [ ] **Step 41** [C] ~2m: Commit `docs/adr/ADR-INDEX.md` (and `wiki/ADR-Index.md` if changed): `[PLAN S-RESIDUE] Steps 33-41: ADR-INDEX rows for 103, 108, 110 follow their files; summary recounted (D-26)`.

### Timeline mirrors

- [ ] **Step 42** [R] ~5m: Read `references/timeline.json` (keys `version`, `last_updated`, `status`, `phases`, `milestones`, `metadata`), the head of `references/timeline.toml` and `references/timeline.yaml`, and one milestone in each (id `phase-3-item-022c2187`). Confirm the id rule on any entry:
  ```bash
  cd /home/govan/tmp/unheaded && python3 -c "
  import json,hashlib
  j=json.load(open('references/timeline.json')); m=j['milestones'][-1]
  print(m['id'], hashlib.sha256(m['description'].encode()).hexdigest()[:8])"
  ```
- [ ] **Step 43** [V] ~1m: The printed id ends with the printed hash. If not, the rule is different for that entry: read `git show 7d7c8279 -- references/` to see how it was written, and stop building the script until the rule is known (`[STUCK]` after 2 tries).
- [ ] **Step 44** [W] ~10m: Test first: `tests/scripts/test_timeline_mirrors.py` (pytest, stdlib plus PyYAML, which the repo's scripts already use: `scripts/check-runbooks.py`). Tests: (a) `--check` passes on the committed tree; (b) a copy of the tree with one md bullet added makes `--check` fail with the new id named; (c) `--write` on that copy adds the milestone to all three mirrors with id `phase-3-item-<sha256[:8]>`, `status` `completed`, `progress` 100, `description` the bullet text, `name` the bold lead without its trailing `(date, branch).`; (d) after `--write`, JSON, YAML (`yaml.safe_load`) and TOML (`tomllib`) parse to the same data, `last_updated` compared as an instant; (e) an md bullet whose text changed is reported, not silently duplicated.
- [ ] **Step 45** [B] ~1m: Red.
  ```bash
  cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/python3 -m pytest tests/scripts/test_timeline_mirrors.py -q 2>&1 | tail -3
  ```
- [ ] **Step 46** [V] ~1m: Fails on the missing script (collection error or file-not-found), not on a typo in the test.
- [ ] **Step 47** [W] ~30m: `scripts/timeline-mirrors.py` (`#!/usr/bin/env python3`, SPDX header, `--check` default and `--write`). Source of truth: `references/timeline.md` bullets for milestone membership and text; `references/timeline.json` for everything else (`phases`, `metadata`, `version`, `status`). `--write` appends missing milestones to JSON, sets `last_updated` to now in UTC, then emits YAML (`yaml.safe_dump`, `sort_keys=False`, `allow_unicode=True`, `width` large so descriptions stay on one line) and TOML (a small first-party emitter for this shape only: top-level scalars, `[[phases]]`, `[[milestones]]`, `[metadata]`; strings escaped per TOML basic strings; `last_updated` written unquoted as the existing file does). `--check` exits 1 naming every md bullet missing from any mirror and every mirror that does not parse to the same data as JSON. Stdlib except `yaml`.
- [ ] **Step 48** [B] ~2m: Green.
  ```bash
  cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/python3 -m pytest tests/scripts/test_timeline_mirrors.py -q 2>&1 | tail -3
  ```
  - Pass → Step 50. (a) fails on the committed tree because two entries are missing → expected until Step 51; mark (a) `xfail(strict=True)` only until Step 52, then remove the mark.
- [ ] **Step 49** [D] ~10m: (d) fails → the TOML emitter or YAML dump lost a character: print the first differing milestone id and field, fix the escaping, rerun. Second failure → `[STUCK]`; the mirrors stay hand-edited for this sprint (Steps 51-52 by hand, as `7d7c8279` did), and the script is parked in the progress file.
- [ ] **Step 50** [B] ~2m: What `--write` would change on the real files: first prove the emitter reproduces today's mirrors' content.
  ```bash
  cd /home/govan/tmp/unheaded && python3 scripts/timeline-mirrors.py --check; echo "rc=$?"
  ```
- [ ] **Step 51** [B] ~2m: Write.
  ```bash
  cd /home/govan/tmp/unheaded && python3 scripts/timeline-mirrors.py --write && git diff --stat references/
  ```
- [ ] **Step 52** [V] ~3m: Step 50 named exactly the two md lines 91 and 92 and nothing else. After Step 51: `--check` rc 0; JSON gained exactly two milestones; the YAML and TOML diffs are those two entries plus `last_updated`. If the YAML/TOML diff is a whole-file reformat, check the parsed data is unchanged apart from the two entries (test (d) does) and say "one-time reformat, content unchanged" in the commit message.
- [ ] **Step 53** [W] ~5m: One line under `scripts/README.md`'s timeline entries (lines 65-66 call the mirrors "placeholder"): they are generated by `scripts/timeline-mirrors.py --write` after every edit to `references/timeline.md`; `tests/scripts/test_timeline_mirrors.py` fails when they drift.
- [ ] **Step 54** [B] ~1m: The freshness check still reads the md, unaffected.
  ```bash
  cd /home/govan/tmp/unheaded && ./scripts/check-timeline-freshness.sh --report | tail -3
  ```
- [ ] **Step 55** [V] ~1m: Prints the md's age and status; no error.
- [ ] **Step 56** [B] ~1m: CI's Python has PyYAML (the test runs in `static-analysis.yml:269`).
  ```bash
  cd /home/govan/tmp/unheaded && grep -inE '^pyyaml' .github/ci-python/requirements*.txt .github/ci-python/requirements.in 2>/dev/null | head -3
  ```
- [ ] **Step 57** [V] ~1m: PyYAML is pinned there. Not pinned → the test would fail in CI: make the YAML half of the test `pytest.importorskip("yaml")` and note in the progress file that CI then checks only JSON and TOML (adding a pin is a third-party change, Stevie's).
- [ ] **Step 58** [B] ~1m: Ruff on the two new files.
  ```bash
  cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/ruff check scripts/timeline-mirrors.py tests/scripts/test_timeline_mirrors.py
  ```
- [ ] **Step 59** [C] ~2m: Commit `scripts/timeline-mirrors.py tests/scripts/test_timeline_mirrors.py scripts/README.md references/timeline.json references/timeline.yaml references/timeline.toml`: `[PLAN S-RESIDUE] Steps 42-59: timeline mirrors generated from timeline.md; S-STAGE-2B and the 11-05..07 entry mirrored (D-26)`.
- [ ] **Step 60** [V] ~1m: **PHASE 2 EXIT GATE.** Index rows match the three files; summary sums to Total; `python3 scripts/timeline-mirrors.py --check` rc 0; pytest green; ruff clean.

---

## PHASE 3: DOCS AND TREE THAT NO LONGER MATCH (D-28) (Steps 61–98)

**Goal**: Each D-28 item is fixed in place, and the one with a recurring cause (the hand-kept completion lists) gets a test.
**Prerequisite**: Phase 0.
**Time**: 2 h.
**Agent**: Agent [P] (wiki edits are the Librarian's lane: keep wiki style, update `_Sidebar` only if a page is added; none is planned).

### Completions (test first)

- [ ] **Step 61** [W] ~15m: Test first in `cmd/unheaded-cli/cmd/completion_test.go` (new): for every key of `NewRootCommand().SubCommands` that is a primary name (skip aliases: the root's `AddCommand` stores aliases under their own keys, so compare against `sub.Name`), assert the bash script's `commands=` list, the zsh `commands` array and the fish script each name it; and for `hop`, that its subcommands `rules` and `mode` are completed. Read the scripts by calling the same generator the `completion` command uses.
- [ ] **Step 62** [B] ~1m: Red.
  ```bash
  cd /home/govan/tmp/unheaded && go test -count=1 ./cmd/unheaded-cli/cmd/ -run Completion 2>&1 | tail -8
  ```
- [ ] **Step 63** [V] ~1m: Fails naming `hop`, `token`, `generate`, `observe` (all four). Fails on fewer → the test is reading the wrong text; fix the test, not the list.
- [ ] **Step 64** [W] ~15m: `cmd/unheaded-cli/cmd/completion.go`: add the four to bash (line 87 and a `hop)` case completing `rules mode`), zsh (array and descriptions from each command's `Short`) and fish (`-a` lines with `-d` from `Short`).
- [ ] **Step 65** [B] ~1m: Green, race.
  ```bash
  cd /home/govan/tmp/unheaded && go test -race -count=1 ./cmd/unheaded-cli/... 2>&1 | tail -3
  ```
- [ ] **Step 66** [V] ~1m: `ok`.
- [ ] **Step 67** [W] ~10m: The wiki. Add an `unheaded hop` subsection to `wiki/eBPF-Programs.md` under hop-ebpf: `unheaded hop rules apply`, `unheaded hop mode` (read the Usage/Short strings in `cmd/unheaded-cli/cmd/hop.go`; say enforce is refused until `hoprules.EnforceBlockers` is empty, as the code says).
- [ ] **Step 68** [C] ~2m: Commit `cmd/unheaded-cli/cmd/completion.go cmd/unheaded-cli/cmd/completion_test.go wiki/eBPF-Programs.md`: `[PLAN S-RESIDUE] Steps 61-68: hop, token, generate, observe in all three completions, guarded by a test; unheaded hop in the wiki (D-28)`.

### One-place text fixes

- [ ] **Step 69** [W][P] ~3m: `wiki/Microservices.md:19`: `shield` row → "Shield gateway daemon (Go): zero-trust ingress checks" (no WAF: the WAF was archived 2026-11-10, `~/tmp/projects/unheaded-waf`). Read `cmd/shield/main.go`'s package comment first and use its own words.
- [ ] **Step 70** [W][P] ~5m: `wiki/Systemd-Units.md`: add an `unheaded-vor.service` row in the table's format (user `unheaded-vor`, `127.0.0.1:20110`, `StateDirectory=unheaded/vor`, sources under `/var/lib/unheaded/vor/.config/vor/sources`; source: `deploy/systemd/unheaded-vor.service`).
- [ ] **Step 71** [W][P] ~10m: `pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md` lines 106-121. Check each against the code first:
  ```bash
  cd /home/govan/tmp/unheaded && for m in HMAC_KEYS SEQ_COUNTERS RETRY_TOKENS ERROR_COUNTERS SETTINGS DOS_STATE FLOW_TYPES HOP_VALIDATORS AUTHORITY; do printf '%-16s ' $m; git grep -l "\b$m\b" -- 'ebpf/*' '*.rs' ':!*.md' | head -2 | tr '\n' ' '; echo; done
  ```
  Then: `HMAC_KEYS`, `HOP_VALIDATORS` action → "In hop-ebpf (ADR-125)"; `RETRY_TOKENS` struct → `RetryTokenKey/Value`, action "In hop-ebpf"; `AUTHORITY` → strike through as the `GOAWAY_STATE` row is (`~~AUTHORITY~~ | — | Removed (S-WIRE-HONEST, 2026-11-04) | —`). Other rows: change only what the grep proves wrong.
- [ ] **Step 72** [W][P] ~2m: `pkg/crypto/pqc/hqc.go:18`: "Status: ROADMAP (Q3 2026)" → "Status: reserved; no implementation. cloudflare/circl does not provide HQC" (keep the rest of the comment); drop "as of March 2026" if present, saying instead what is true at HEAD.
- [ ] **Step 73** [W][P] ~2m: `docs/architecture/PROJECT_STRUCTURE.md`: delete the three orphaned lines 73-75 (`│       └── src/`, `│           ├── main.rs             # WAF entry point`, `│           └── rules/              # Detection rules`) and leave the `│` spacer line above `├── services/`.
- [ ] **Step 74** [W][P] ~2m: `nixos/README.md:9` and `nixos/hosts/host-a/configuration.nix:3`: "vLLM/ROCm" → "llama-server (ROCm)". The ROCm packages stay (GPU compute, S-CLEAN-CUT Phase 5A note).
- [ ] **Step 75** [W][P] ~3m: `cmd/zhen-mcp/main.go:10-12`: "Eight read-only tools are always listed: corpus_search (vor), code_definition and code_callers (the code index), file_read, service_health, runbook_list, runbook_show, local_llm (...)". Cross-check against `cmd/zhen-mcp/tools.go:64-118`.
- [ ] **Step 76** [W][P] ~2m: `deploy/sysusers.d/unheaded.conf:33`: `"Unheaded Vector log shipper (east)"` → `"Unheaded Vector log shipper"` (the uid serves west's compose Vector too, S-HOST-TIDY note). The uid and name do not change.
- [ ] **Step 77** [W][P] ~3m: `scripts/east-bootstrap.sh:18,233`: `root@` → `${EAST_USER:-govan}@` with `EAST_USER` defined beside `EAST_IP`; the copy target then needs `sudo` on east, so line 233's `scp` goes to `/tmp/` and the next step uses `ssh ... sudo install`. Read the whole script first; if it is fully superseded (nothing calls it: `git grep -n east-bootstrap -- ':!docs/battle-plans' ':!docs/sweeps'`), say so in a header comment instead of rewriting its flow.
- [ ] **Step 78** [B] ~2m: Shell syntax, nix parse, Go build of the touched packages, sysusers guard.
  ```bash
  cd /home/govan/tmp/unheaded && bash -n scripts/east-bootstrap.sh && nix-instantiate --parse nixos/hosts/host-a/configuration.nix >/dev/null && go build ./cmd/zhen-mcp/ ./pkg/crypto/pqc/ && go test -count=1 ./pkg/uids/... 2>&1 | tail -1
  ```
- [ ] **Step 79** [V] ~1m: All succeed. `pkg/uids` fails → it compares GECOS text: update its expectation in the same commit, nothing else.
- [ ] **Step 80** [C] ~2m: Commit the files of Steps 69-77 by name: `[PLAN S-RESIDUE] Steps 69-80: shield is not a WAF, vor unit row, BPF map table, hqc status, orphaned waf lines, llama-server text, zhen-mcp tool count, vector GECOS, east-bootstrap user (D-28)`.

### lxd counts

- [ ] **Step 81** [B] ~3m: Count what the file itself launches per category, and what `launch-all` launches.
  ```bash
  cd /home/govan/tmp/unheaded && awk '/^# CATEGORY/{if(c)print c" "n; c=$0; n=0; next} /lxc launch|launch_container/{n++} END{print c" "n}' lxd/LAUNCH_EXAMPLES.sh; sed -n '/^launch-all:/,/^[a-z-]*:/p' lxd/Makefile | grep -cE 'lxc launch|launch_'
  ```
- [ ] **Step 82** [V] ~2m: Each category has a number. If the awk pattern counts 0 everywhere, read the file and count by hand; the launch lines use some other form.
- [ ] **Step 83** [W] ~5m: Set each `# CATEGORY N: ... (K containers)` header and `lxd/Makefile:30,57` to the counted numbers. Category 4 header keeps "HOST-A ONLY".
- [ ] **Step 84** [B] ~1m: `bash -n lxd/LAUNCH_EXAMPLES.sh && make -n -C lxd help >/dev/null && echo ok`
- [ ] **Step 85** [V] ~1m: `ok`.

### circl licence re-check

- [ ] **Step 86** [B] ~2m: The licence at the version in go.mod, from the module cache (nothing downloaded if it is cached; `go mod download` reads only a module already in go.sum).
  ```bash
  cd /home/govan/tmp/unheaded && d=$(go list -m -f '{{.Dir}}' github.com/cloudflare/circl) && echo "$d" && head -5 "$d/LICENSE" && sha256sum "$d/LICENSE"
  ```
- [ ] **Step 87** [V] ~2m: The path ends `circl@v1.6.5`; the licence is BSD-3-Clause as `LICENSES/THIRD_PARTY.md:725` records. Changed licence → STOP this item, record it for Stevie (a licence change is the Barrister's and his).
- [ ] **Step 88** [W] ~3m: `LICENSES/THIRD_PARTY.md:725` heading → `### cloudflare/circl v1.6.5` with one line "Re-checked 2026-11-12 at v1.6.5: BSD-3-Clause, LICENSE sha256 `<from Step 86>`". Line 12 is the audit's "new since last audit" line of its date → leave it.

### The man page

- [ ] **Step 89** [B] ~5m: `scripts/gen-man.sh` stamps `git describe`; a build in the main checkout reads the tree as dirty (`demos/doom/` is untracked). Generate in a clean detached worktree.
  ```bash
  cd /home/govan/tmp/unheaded && git worktree add --detach /tmp/claude-1000/s-residue/man-wt HEAD >/dev/null && (cd /tmp/claude-1000/s-residue/man-wt && ./scripts/gen-man.sh --write >/dev/null && head -1 man/zhen-cli.1 && git diff --stat man/ | tail -1)
  ```
- [ ] **Step 90** [V] ~2m: The new `.TH` line carries `git describe` output with no `+dirty`. The stat shows the `.TH` line changed in every page (date and describe move with every commit; the check mode ignores line 1).
- [ ] **Step 91** [B] ~1m: Take only `man/zhen-cli.1` (one page was wrong; restamping 56 pages is churn), then remove the worktree.
  ```bash
  cd /home/govan/tmp/unheaded && cp /tmp/claude-1000/s-residue/man-wt/man/zhen-cli.1 man/zhen-cli.1 && git worktree remove --force /tmp/claude-1000/s-residue/man-wt && git diff --stat man/
  ```
- [ ] **Step 92** [B] ~4m: Check mode.
  ```bash
  cd /home/govan/tmp/unheaded && ./scripts/gen-man.sh 2>&1 | tail -2
  ```
- [ ] **Step 93** [V] ~1m: `OK: man/ matches N binaries`; the diff is line 1 of `man/zhen-cli.1` only.
- [ ] **Step 94** [D] ~5m: `stale: man/zhen-cli.1` → its body differs too (the page was redirected from another binary's output, S-ZHEN-POLISH Step 37): copy the whole generated page (that is what Step 91 did) and rerun; a different page stale → it was stale before this sprint: record it, do not regenerate it here.
- [ ] **Step 95** [C] ~2m: Commit `lxd/LAUNCH_EXAMPLES.sh lxd/Makefile LICENSES/THIRD_PARTY.md man/zhen-cli.1`: `[PLAN S-RESIDUE] Steps 81-95: lxd counts from the files, circl v1.6.5 licence re-checked, zhen-cli.1 without +dirty (D-28)`.
- [ ] **Step 96** [B] ~1m: Doc links still resolve.
  ```bash
  cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/python3 -m pytest tests/scripts/test_doc_links.py -q 2>&1 | tail -2
  ```
- [ ] **Step 97** [V] ~1m: Green.
- [ ] **Step 98** [V] ~2m: **PHASE 3 EXIT GATE.** Every D-28 item in the Grounding table is fixed or recorded with its reason; completion test green; gen-man OK; doc links green.

---

## PHASE 4: COMPLIANCE FOLD RESIDUE (D-29) (Steps 99–122)

**Goal**: A failed audit-ring consumer is visible on `/metrics` and joined at shutdown; `no_data` is on `/metrics` and in `Stats()` and clears on the first evaluated record; the drop counter says what it counts; the allowlist test sees variable-topic subscriptions.
**Prerequisite**: Phase 0.
**Time**: 90 min.
**Agent**: Agent [P] (Developer: red, green, refactor; `-race` always).

- [ ] **Step 99** [R] ~5m: `cmd/compliance-evidence/wire.go:88-150`, `services/compliance/wire_compliance.go` lines 119-125, 200-210, 331-365, 613-645, 770-801, 832-847; ADR-127 lines 30-45.
- [ ] **Step 100** [W] ~10m: Tests first in `services/compliance/wire_compliance_test.go`: (a) a new service's `/metrics` has `compliance_no_data 1` and `Stats()["no_data"] == true`; (b) after one evaluated record and no aggregation tick, both read 0/false (F7: no lag of one window); (c) the `compliance_wotan_drops_total` HELP line reads "Events not published: no Wotan client configured".
- [ ] **Step 101** [W] ~10m: Test first in `cmd/compliance-evidence/wire_test.go`: `serveWire` with `--audit-ring` pointing at a path that does not exist: `/metrics` shows `compliance_audit_ring_up 0` and `compliance_audit_ring_errors_total 1`; cancelling the context returns only after the consumer goroutine has returned (use a hook or a `sync.WaitGroup` exposed to the test, not a sleep). Without `--audit-ring` the gauge is absent (the ring is optional).
- [ ] **Step 102** [B] ~2m: Red.
  ```bash
  cd /home/govan/tmp/unheaded && go test -count=1 ./services/compliance/ ./cmd/compliance-evidence/ 2>&1 | grep -E '^(--- FAIL|FAIL|ok)' | head
  ```
- [ ] **Step 103** [V] ~1m: The new tests fail for the stated reasons; no old test fails.
- [ ] **Step 104** [W] ~10m: `services/compliance/wire_compliance.go`: emit `compliance_no_data` (gauge 0/1) in `handleMetrics`; add `"no_data"` to `Stats()`; clear `NoData` where a record is first evaluated (under the same lock that updates the counters), keeping the aggregation path's set/clear as it is; HELP text per test (c).
- [ ] **Step 105** [W] ~15m: `cmd/compliance-evidence/wire.go`: the ring consumer runs under a `sync.WaitGroup`; its error sets an `atomic` up-flag to 0 and increments an error counter; both are written on the wire `/metrics` (through the service's handler or a wrapper mux, whichever keeps one `/metrics` path); shutdown waits for the group after `srv.Shutdown`. The stderr line stays.
- [ ] **Step 106** [B] ~3m: Green, race.
  ```bash
  cd /home/govan/tmp/unheaded && go test -race -count=1 ./services/compliance/... ./cmd/compliance-evidence/... 2>&1 | tail -4
  ```
- [ ] **Step 107** [V] ~1m: `ok` for both.
- [ ] **Step 108** [D] ~5m: Race on the up-flag or `NoData` → the write is outside the lock or not atomic; fix the access, not the test. A Linux-only ring test (`wire_ring_linux_test.go`) needs root → it skips as before; check `-v` shows SKIP, not FAIL.
- [ ] **Step 109** [C] ~2m: Commit `services/compliance/wire_compliance.go services/compliance/wire_compliance_test.go cmd/compliance-evidence/wire.go cmd/compliance-evidence/wire_test.go`: `[PLAN S-RESIDUE] Steps 99-109: compliance no_data on /metrics and Stats, cleared on the first record; audit ring failure visible and joined (D-29)`.

### Alert rule follow-through

- [ ] **Step 110** [B] ~2m: Is any compliance metric alerted or paneled by name (so a name change does not orphan it)?
  ```bash
  cd /home/govan/tmp/unheaded && git grep -n 'compliance_wotan_drops_total\|compliance_no_data\|compliance_audit_ring' -- monitoring observability grafana config 2>/dev/null | head
  ```
- [ ] **Step 111** [V] ~1m: No rule depends on the HELP text (names did not change). A new metric is not alerted here: an alert needs a runbook and is S-WATCH-TRUE's lane; note "compliance_audit_ring_up has no alert" in Parked.

### Allowlist test

- [ ] **Step 112** [W] ~5m: `services/wotan/cmd/wotan/allowlist_test.go:34`: widen the pattern to accept a variable topic: `` `\.Subscribe(?:Topic)?\(ctx, (?:"[^"]+"|[A-Za-z_][\w.]*), "([^"]+)"\)` ``. Add `services/firewall` to `deployed` only if it is deployed (check `docker-compose.yml` and `deploy/systemd/` for a firewall service first); do not add `services/compliance`, which `wire` runs with no client (ADR-127 line 37).
- [ ] **Step 113** [B] ~2m: Run it.
  ```bash
  cd /home/govan/tmp/unheaded && go test -count=1 ./services/wotan/cmd/wotan/ -run TestShippedAllowlistCoversDeployedSubscribers -v 2>&1 | tail -12
  ```
- [ ] **Step 114** [V] ~2m: Pass → the newly seen names (`dashboard-backend`, `kanban-app`, `trace-collector`) are all auto-approved; good. Fail → a deployed service subscribes under a name `configs/wotan.yaml` does not auto-approve with auth off: that is a real gap (the FND-043 kind). Do not edit `configs/wotan.yaml` here (it is read live; a change needs a recreate or HUP and is Wotan's lane). Record the names in Parked and in the commit message. A red test on develop is not allowed, so put the names in a `knownGaps` map in the test, with the date and "see S-RESIDUE progress, Parked". The test then fails on any new gap, and on any known gap that has been fixed, so the map can only shrink.
- [ ] **Step 115** [B] ~2m: `go test -race -count=1 ./services/wotan/... 2>&1 | tail -3`
- [ ] **Step 116** [V] ~1m: `ok`.
- [ ] **Step 117** [C] ~2m: Commit `services/wotan/cmd/wotan/allowlist_test.go`: `[PLAN S-RESIDUE] Steps 112-117: the allowlist test sees variable-topic subscriptions (D-29)`, naming any known gaps.
- [ ] **Step 118** [B] ~3m: gosec on the touched Go (the CI pin).
  ```bash
  cd /home/govan/tmp/unheaded && command -v gosec >/dev/null && gosec -quiet ./cmd/compliance-evidence/... ./services/compliance/... 2>&1 | tail -5 || echo "gosec not on PATH: covered by the Phase 9 meta-gate"
  ```
- [ ] **Step 119** [V] ~1m: No new finding.
- [ ] **Step 120** [W] ~3m: ADR-127 is Proposed and parked; add nothing to it. Record in the progress file what changed against its "What is there" list (`no_data` now on `/metrics`; ring failure visible).
- [ ] **Step 121** [C] ~1m: Commit the progress file.
- [ ] **Step 122** [V] ~1m: **PHASE 4 EXIT GATE.** Tests green with `-race`; the four held D-29 items are fixed; the dropped and conditional items are written in the progress file.

---

## PHASE 5: ZHEN CODE RESIDUE (D-30) (Steps 123–146)

**Goal**: A queue-only tool is refused before the terminal prompt; a half-removed code-index shard is an error, not an empty answer; `CodeBackend` stops at its context; `AppendTurns` has run against a real Postgres.
**Prerequisite**: Phase 0.
**Time**: 2 h.
**Agent**: Agent [P] (Developer).

### TTYConfirmer

- [ ] **Step 123** [W] ~8m: Test first in `pkg/claudebridge/confirm_test.go`: a `TTYConfirmer` with a fake TTY, asked to confirm `teach_write` (a `champion.QueueOnlyTools` member): the outcome is `unconfirmable` with text naming the queue, and **nothing is written to the TTY** (assert the fake's write buffer is empty).
- [ ] **Step 124** [B] ~1m: `go test -count=1 ./pkg/claudebridge/ -run QueueOnly 2>&1 | tail -4` → red (the prompt is written today).
- [ ] **Step 125** [W] ~5m: `pkg/claudebridge/confirm.go:125`: after the nil checks, `if champion.QueueOnlyTools[req.Tool] { return unconfirmable("queue-only tool: it runs from the review queue, not from a terminal") }`.
- [ ] **Step 126** [B] ~2m: `go test -race -count=1 ./pkg/claudebridge/... 2>&1 | tail -2` → `ok`.

### codeindex: missing slot file

- [ ] **Step 127** [W] ~10m: Tests first. `pkg/codeindex/codeindex_test.go`: a dir with `CURRENT` naming a slot whose file is gone → `Store.Get` returns an error that `errors.Is(err, codeindex.ErrSlotMissing)` and is **not** `fs.ErrNotExist`; a dir with no `CURRENT` still returns an `fs.ErrNotExist` error (no index built). `pkg/retrieval/code_test.go`: with a missing slot, `CodeBackend.Search` returns an error (counted as a failed backend), not zero hits.
- [ ] **Step 128** [B] ~1m: `go test -count=1 ./pkg/codeindex/ ./pkg/retrieval/ 2>&1 | grep -E 'FAIL|ok'` → the new tests fail.
- [ ] **Step 129** [W] ~10m: `pkg/codeindex/codeindex.go`: `var ErrSlotMissing = errors.New("codeindex: CURRENT names a slot file that does not exist")`; in `Load`, when `CURRENT` was read but the slot file is `fs.ErrNotExist`, wrap with `ErrSlotMissing` and the path, so `errors.Is(err, fs.ErrNotExist)` is false. `Store.Get`'s "older index wins" path is unchanged. `pkg/retrieval/code.go:36` keeps treating `fs.ErrNotExist` as "no index" and now returns other errors.
- [ ] **Step 130** [B] ~2m: `go test -race -count=1 ./pkg/codeindex/... ./pkg/retrieval/... ./cmd/zhen-codeindex/... 2>&1 | tail -3` → `ok`.

### CodeBackend context

- [ ] **Step 131** [W] ~10m: Test first in `pkg/codeindex`: `ix.SearchContext(ctx, query, k)` with an already-cancelled context returns `ctx.Err()` and scans no chunk (count through a test hook or an index with a chunk whose scoring panics if reached). In `pkg/retrieval`: `CodeBackend.Search` with a cancelled context returns promptly with `context.Canceled`.
- [ ] **Step 132** [W] ~10m: `pkg/codeindex/codeindex.go`: `SearchContext` checks `ctx.Err()` every 256 chunks; `Search` becomes `SearchContext(context.Background(), ...)` so callers keep working. `pkg/retrieval/code.go:42` calls `SearchContext(ctx, ...)`.
- [ ] **Step 133** [B] ~2m: `go test -race -count=1 ./pkg/codeindex/... ./pkg/retrieval/... ./cmd/zhen-mcp/... ./cmd/zhen-agentd/... 2>&1 | tail -4` → `ok`.
- [ ] **Step 134** [B] ~3m: The scan cost did not move (about 40 ms today, S-ZHEN-POLISH note).
  ```bash
  cd /home/govan/tmp/unheaded && go test -run '^$' -bench 'Search' -benchtime 20x ./pkg/codeindex/ 2>&1 | grep -E 'Benchmark|ok' | head -4
  ```
- [ ] **Step 135** [V] ~1m: No existing benchmark → note it, do not write one (gold plate). An existing one: within 5 % of its value before Step 132 (run it at Step 131 too).
- [ ] **Step 136** [C] ~2m: Commit `pkg/claudebridge/confirm.go pkg/claudebridge/confirm_test.go pkg/codeindex/codeindex.go pkg/codeindex/codeindex_test.go pkg/retrieval/code.go pkg/retrieval/code_test.go`: `[PLAN S-RESIDUE] Steps 123-136: queue-only refused before the prompt; missing slot is an error; code search stops at its context (D-30)`.

### AppendTurns on a real Postgres

- [ ] **Step 137** [R] ~5m: `scripts/well/test-llm-requests.sh` (how it bootstraps a throwaway server from `db/init.sh` and sets `WELL_TEST_DSN`), `pkg/claudebridge/well_integration_test.go` (the `wellint` tag pattern), and the migration that creates the table `AppendTurns` writes (`git grep -n 'zhen_conversation' db/migrations | head`).
- [ ] **Step 138** [W] ~15m: `cmd/zhen-agentd/conversations_well_test.go`, `//go:build wellint`, skips when `WELL_TEST_DSN` is unset (never the real Well): `AppendTurns` writes N turns in one transaction as the role the daemon uses (`app_zhen`); the rows read back with the session, model and sources; a turn that violates a constraint mid-batch leaves zero rows (the transaction claim in the comment at line 159).
- [ ] **Step 139** [W] ~10m: Extend `scripts/well/test-llm-requests.sh`'s Go leg (step 6 in its header) to also run `go test -tags wellint ./cmd/zhen-agentd/ -run AppendTurns` against the same throwaway server, with the DSN for `app_zhen` if the bootstrap creates that role (else note which role it used). Update its header list.
- [ ] **Step 140** [S][B] ~8m: Run it (docker; the default pinned image).
  ```bash
  cd /home/govan/tmp/unheaded && sudo -n true 2>/dev/null && bash scripts/well/test-llm-requests.sh 2>&1 | tail -15 || echo "sudo not cached: run when Stevie is there"
  ```
- [ ] **Step 141** [V] ~2m: Every numbered check passes, the new AppendTurns leg included. No sudo → `[BLOCKED]` on sudo, commit the test anyway (it skips without a DSN), say so in Parked.
- [ ] **Step 142** [D] ~5m: The rollback assertion fails → `AppendTurns` commits per turn somewhere: read the function again; a real bug here is a finding (note it in the register? No: record it in Parked and in next.md for Stevie; fix it only if the fix is the one-line `tx` use the comment promises).
- [ ] **Step 143** [B] ~1m: `go vet -tags wellint ./cmd/zhen-agentd/ && bash -n scripts/well/test-llm-requests.sh && echo ok`
- [ ] **Step 144** [V] ~1m: `ok`.
- [ ] **Step 145** [C] ~2m: Commit `cmd/zhen-agentd/conversations_well_test.go scripts/well/test-llm-requests.sh`: `[PLAN S-RESIDUE] Steps 137-145: AppendTurns run against a throwaway Postgres (D-30)`.
- [ ] **Step 146** [V] ~1m: **PHASE 5 EXIT GATE.** The four D-30 items are fixed with tests; `-race` green; the wellint leg passed or is `[BLOCKED]` on sudo with the reason written.

---

## PHASE 6: DEPLOY FILE RESIDUE (D-31) (Steps 147–160)

**Goal**: No deploy file copies a file that does not exist, sets a variable nothing reads, or runs a service as root.
**Prerequisite**: Phase 0.
**Time**: 60 min.
**Agent**: Agent [P].

- [ ] **Step 147** [W] ~10m: Test first in `cmd/zhen-codeindex/tomb_test.go` (it already reads `tomb/ansible/playbook-grimoire.yml`): every `src: "files/<name>"` in `tomb/ansible/*.yml` names a file that exists under `tomb/ansible/files/`; a task that copies a repo file must not carry `failed_when: false`.
- [ ] **Step 148** [B] ~1m: `go test -count=1 ./cmd/zhen-codeindex/ -run Tomb 2>&1 | tail -4` → red on `files/grimoire-search.sh`.
- [ ] **Step 149** [W] ~3m: `tomb/ansible/playbook-grimoire.yml:40-47`: delete the "Deploy grimoire-search.sh" task (the VM searches with `zhen-codeindex --search`, commit `61ac5511`; nothing calls the script). If the playbook's header or the tomb README tells the operator to run `grimoire-search.sh`, point it at `{{ grimoire_root }}/bin/zhen-codeindex --search` instead.
- [ ] **Step 150** [B] ~1m: `go test -race -count=1 ./cmd/zhen-codeindex/... 2>&1 | tail -2` → `ok`.
- [ ] **Step 151** [W] ~5m: `nixos/modules/services/trace-collector.nix`: remove `TRACE_PORT` and `TRACE_LOG_LEVEL` (lines 41-42) and the `port` and `logLevel` options they read, unless another module sets them (`git grep -n 'trace-collector.*\(port\|logLevel\)' nixos/`); if one does, remove that setting too. Update `nixos/modules/services/README.md:22,92` only if they state the port as configurable through this module. (Busboy: if S-WATCH-TRUE has already added an environment variable here, keep theirs.)
- [ ] **Step 152** [W] ~10m: `nix/containers/sophia-eye.nix`: drop `VLLM_WORKER_MULTIPROC_METHOD` (line 99). Both units off root: `DynamicUser = true;` with `StateDirectory = "sophia-eye";` (the `WorkingDirectory` `/var/lib/sophia-eye` then belongs to the unit's user) and, for `sophia-gateway`, which runs `go run`, `CacheDirectory = "sophia-gateway";` plus `Environment = [ "GOCACHE=/var/cache/sophia-gateway" "HOME=/var/lib/sophia-eye" ];` (it needs a writable build cache). `embeddings-bge` reads `/data/models/bge-m3` and may use the GPU: add `SupplementaryGroups = [ "render" "video" ];` and `ReadOnlyPaths = [ "/data/models" ];`. Add `NoNewPrivileges = true;` to both. Do not change ports or endpoints.
- [ ] **Step 153** [B] ~2m: Parse both files.
  ```bash
  cd /home/govan/tmp/unheaded && nix-instantiate --parse nixos/modules/services/trace-collector.nix >/dev/null && nix-instantiate --parse nix/containers/sophia-eye.nix >/dev/null && echo parse-ok
  ```
- [ ] **Step 154** [V] ~1m: `parse-ok`. These files are not built or deployed on west; a parse is the gate available here (as `5d8aab8a` did for three .nix files). Say so in the commit message.
- [ ] **Step 155** [B] ~1m: The nix firewall test and image digest test still pass (they read nix files).
  ```bash
  cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/python3 -m pytest tests/scripts/test_nix_firewall.py tests/scripts/test_image_digests.py -q 2>&1 | tail -2
  ```
- [ ] **Step 156** [V] ~1m: Green.
- [ ] **Step 157** [BLOCKED] ~0m: `deploy/sophia-eye/shield-rules.yaml`: egress rule for llama-server. Blocked on A-56 (where llama-server listens is Stevie's answer; no code loads this file today).
- [ ] **Step 158** [C] ~2m: Commit `cmd/zhen-codeindex/tomb_test.go tomb/ansible/playbook-grimoire.yml nixos/modules/services/trace-collector.nix nix/containers/sophia-eye.nix` (and the README if touched): `[PLAN S-RESIDUE] Steps 147-158: grimoire task for a missing file removed and guarded; dead trace-collector env; sophia-eye units off root (D-31)`.
- [ ] **Step 159** [W] ~2m: Progress file: Step 157 `[BLOCKED]` on A-56.
- [ ] **Step 160** [V] ~1m: **PHASE 6 EXIT GATE.** Tomb test green; both .nix parse; no `User = "root"` left in `nix/containers/sophia-eye.nix` (`grep -c 'User = "root"'` prints 0).

---

## PHASE 7: HOUSEKEEPING (D-32) (Steps 161–170)

**Goal**: The wiki unit's boot ordering lives in the repo; the leftovers are gone; the meta-gate tells the reader where ruff and pip-audit are.
**Prerequisite**: Phase 0.
**Time**: 40 min plus Stevie for two host steps.
**Agent**: Coordinator.

- [ ] **Step 161** [W] ~5m: `deploy/systemd/unheaded-wiki-server.service.d/west.conf`: the `[Unit] After=docker.service` part and its 2026-11-12 comment only (the bind mount and listeners stay host-only: they name this checkout and the LAN address). Follow `deploy/systemd/unheaded-chaos-controller.service.d/west-packet-lab.conf`'s header style. Add a row to `wiki/Systemd-Units.md`'s drop-ins (or the wiki unit's row) saying which file carries the ordering.
- [ ] **Step 162** [B] ~1m: `systemd-analyze verify deploy/systemd/unheaded-wiki-server.service 2>&1 | grep -v 'Unknown\|not found' | head -5; echo done` (the drop-in is verified with the unit when installed; this checks the unit still parses).
- [ ] **Step 163** [STEVIE][S] ~5m: Attended. On west, make the host drop-in the repo's ordering plus its host lines (no content change today: compare by eye), then at the next planned reboot check the unit started once: `journalctl -b -u unheaded-wiki-server | grep -c 'cannot assign requested address'` → 0. Until that reboot, D-32's "proven only at next boot" stays open in next.md.
- [ ] **Step 164** [STEVIE][S] ~3m: Attended. Remove the unread `GRAFANA_ADMIN_PASSWORD` line from `/etc/unheaded/monitoring.env` without printing the file: `sudo sed -i.bak-2026-11-12 '/^GRAFANA_ADMIN_PASSWORD=/d' /etc/unheaded/monitoring.env && sudo grep -c '^GRAFANA_ADMIN_PASSWORD=' /etc/unheaded/monitoring.env` → 0; `cd /home/govan/tmp/unheaded/monitoring && sudo docker compose config --hash '*' >/dev/null && echo config-ok`. The `.bak` holds the old line (root 0600); delete it with the rotation directory.
- [ ] **Step 165** [W] ~3m: `wiki/Observability-Backends.md:25`: drop "`monitoring/.env` needs `GRAFANA_ADMIN_PASSWORD` (the stack refuses to start without it)"; the one Grafana's admin password is in `/etc/unheaded/compose.env` (ADR-117; `docker-compose.yml:633`). `QUICKSTART.md:125,137` say `.env`, which is a symlink to that file: true, leave.
- [ ] **Step 166** [B] ~3m: The `s-clean-cut` worktree: clean and merged?
  ```bash
  cd /home/govan/tmp/unheaded && git -C /home/govan/tmp/wt/s-clean-cut status --porcelain | head -3; git merge-base --is-ancestor b836ee8e develop && echo merged
  ```
- [ ] **Step 167** [V][B] ~2m: Empty status and `merged` → `git worktree remove /home/govan/tmp/wt/s-clean-cut && git worktree prune` (the branch `sprint/s-clean-cut` stays). Anything uncommitted → leave it and write what is there in Parked.
- [ ] **Step 168** [B] ~1m: The obsolete login script: nothing references it, then remove it (host state file, not in the repo).
  ```bash
  grep -rl 'zhen_login_test.sh' ~/.local/state/unheaded/ /home/govan/tmp/next.md 2>/dev/null | grep -v obsolete; rm -v ~/.local/state/unheaded/zhen_login_test.sh.obsolete-2026-11-12
  ```
- [ ] **Step 169** [W] ~5m: ruff and pip-audit on west: in `scripts/check-ruff.sh` (the "not found on PATH" message, line 63) and the pip-audit gate's equivalent, add one line: "On west they are in /var/tmp/unheaded-sprint/civenv/bin: PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin". Same line in the wiki page that documents the meta-gate (`git grep -ln 'check-gates-can-fail' wiki/`), as an ops fact. Installing them on the host is Stevie's (no step).
- [ ] **Step 170** [C][V] ~3m: **PHASE 7 EXIT GATE.** Commit `deploy/systemd/unheaded-wiki-server.service.d/west.conf wiki/Systemd-Units.md wiki/Observability-Backends.md scripts/check-ruff.sh` plus the pip-audit gate file and the wiki page: `[PLAN S-RESIDUE] Steps 161-170: wiki boot ordering in the repo; stale Grafana line; where ruff and pip-audit live on west (D-32)`. Steps 163-164 recorded as done or waiting on Stevie.

---

## PHASE 8: THE PROMPT HALF OF D-19 (GATED) (Steps 171–174)

**Goal**: The system prompt says `~/.config/vor`, the goldens and the generated Python follow, and the ADR-024 eval rerun is on file.
**Prerequisite**: `[GATED]` llama-server is up (Stevie lifted the pause), and Phases 0-1 are done. Check: `curl -sf http://127.0.0.1:8081/health`.
**Time**: 30 min plus the eval run (the 2026-10-26 runs took hours for 100 tasks; `-split holdout` is the ADR's bar).
**Agent**: Coordinator (it starts a model run; check in with Stevie before a long run).

- [ ] **Step 171** [GATED][W] ~10m: `pkg/zhenrag/prompt.go:125,127,187,189`: `~/.config/cs/` → `~/.config/vor/`. Regenerate the Python copy (`go generate ./pkg/zhenrag/...`, which writes `raft/scripts/zhen_prompt_generated.py`). The three goldens in `pkg/zhenrag/testdata/prompts/` are each surface's prompt: update the same two phrases in each by hand, then `go test -race -count=1 ./pkg/zhenrag/... ./cmd/zhen-rag/... ./cmd/zhen-cli/...` and `/var/tmp/unheaded-sprint/raftvenv/bin/python3 -m pytest raft/tests/test_zhen_prompt.py -q` must pass (`TestGenerated_PythonInSync` included).
- [ ] **Step 172** [GATED][C] ~2m: Commit those five files: `[PLAN S-RESIDUE] Step 171: system prompt names ~/.config/vor (D-19, second half; ADR-024 eval rerun follows)`.
- [ ] **Step 173** [GATED][B] ~?: The ADR-024 rerun, local leg, on the model llama-server is serving (name it from `scripts/switch-model.sh --check` output; qwen-coder-14b was the kept one). Ask Stevie before starting if it will run over an hour.
  ```bash
  cd /home/govan/tmp/unheaded && E=eval/champion-runbooks && go run ./cmd/zhen-eval -backend local -model <served-model-key> -split holdout -tasks $E/split.jsonl -keys $E/keys.jsonl -prompts $E/prompts.jsonl -out $E/results-$(date -u +%F)-<key>.jsonl 2>&1 | tail -5
  ```
- [ ] **Step 174** [GATED][W][C][V] ~15m: **PHASE 8 EXIT GATE.** Write `eval/champion-runbooks/RESULTS-<date>.md` in the format of `RESULTS-2026-10-26.md`, with one line under the table: "Rerun for the 2026-11-12 prompt-path change (ADR-024 trigger). `cmd/zhen-eval` sends its own system text, so this run measures the model, not the edited prompt." Commit the results jsonl and md. A local model at the bar is not approved by this; approval is Stevie's (ADR-024).

---

## PHASE 9: FINAL GATE AND RECORDS (Steps 175–176)

**Goal**: The whole sprint is green together and written down.
**Prerequisite**: Phases 0-7 done or with every skip recorded; Phase 8 done or still gated.
**Time**: 40 min.
**Agent**: Coordinator.

- [ ] **Step 175** [B][V] ~25m: The full check, then the records.
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && go test -race -count=1 ./cmd/unheaded-cli/... ./cmd/compliance-evidence/... ./services/compliance/... ./services/wotan/... ./pkg/claudebridge/... ./pkg/codeindex/... ./pkg/retrieval/... ./pkg/champion/... ./cmd/zhen-agentd/... ./cmd/zhen-codeindex/... ./cmd/zhen-mcp/... ./pkg/uids/... 2>&1 | grep -vE '^ok|no test files' | tail
  PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin ./scripts/check-ruff.sh 2>&1 | tail -1
  /var/tmp/unheaded-sprint/civenv/bin/python3 -m pytest tests/scripts -q 2>&1 | tail -2
  ./scripts/gen-man.sh 2>&1 | tail -1 && python3 scripts/timeline-mirrors.py --check && echo mirrors-ok
  PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin ./scripts/check-gates-can-fail.sh --quick 2>&1 | tail -25 > /tmp/claude-1000/s-residue/metagate-after.txt; diff <(grep -E 'FAIL|INCONCLUSIVE' /tmp/claude-1000/s-residue/metagate-before.txt) <(grep -E 'FAIL|INCONCLUSIVE' /tmp/claude-1000/s-residue/metagate-after.txt) && echo metagate-no-new-red
  ```
  Pass: no Go failure, ruff PASS, pytest green, gen-man OK, `mirrors-ok`, `metagate-no-new-red`. Any red → back to the phase that owns the file; do not record the sprint as done.
- [ ] **Step 176** [W][C][V] ~15m: **PHASE 9 EXIT GATE AND RECORDS.** (1) `references/timeline.md`: one dated entry for S-RESIDUE in the Age 3 list (what was fixed, what was dropped, what is gated), then `python3 scripts/timeline-mirrors.py --write` and `--check`. (2) The progress file: per-row outcome table (fixed / dropped with reason / blocked / gated), Parked list, Stuck list. (3) `docs/sweeps/BACKLOG-SWEEP-2026-11-12.md` is a dated record: do not edit it. (4) `~/tmp/next.md`: a short S-RESIDUE section with the commits, what waits on Stevie (Steps 163, 164; Phase 8 gate; A-56; any wellint run left for sudo) and the parked items. Commit `references/timeline.md references/timeline.json references/timeline.yaml references/timeline.toml docs/battle-plans/progress/S-RESIDUE-progress.md`: `[PLAN S-RESIDUE] Steps 175-176: final gate green; timeline and progress recorded`. Stevie pushes.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. gpg-agent timed out mid-sprint.** Stevie away: commit with `--no-gpg-sign`, note the hashes in the progress file; when he caches the key, `scripts/resign-unpushed.py` (probe with `git commit-tree -S`, not `gpg --clearsign`). Stevie present: ask him to cache it.

**A2. A Go test fails on a package this plan did not touch.** Run it on the commit before the sprint (`git stash` is not needed: `git worktree add --detach /tmp/claude-1000/s-residue/base a31be33b`). Red there too → inherited: record, do not fix here. Green there → this sprint broke it: bisect over the sprint's commits (`git bisect run go test ./<pkg>/...`).

**A3. `timeline-mirrors.py --write` rewrote a mirror wrongly.** `git checkout -- references/timeline.json references/timeline.yaml references/timeline.toml` and go to Step 49's skip path. Never hand-merge a generated file and a hand edit.

**A4. The completion test passes before the fix (Step 63).** The test reads a cached or different script. Print what it read (`t.Log` the first 200 bytes of each) and compare with `go run ./cmd/unheaded-cli completion bash | head`.

**A5. `sudo docker` asks for a password (Step 140).** Do not wait. Mark `[BLOCKED]` on sudo, commit the test (it skips without a DSN), go on.

**A6. A step would change a host file not listed as `[STEVIE]`.** Stop: host changes in this plan are Steps 163, 164 (Stevie) and Steps 167-168 (the user's own worktree and state file). Anything else is out of scope.

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel | Depends on | Time | Touches host |
|---|---|---|---|---|---|
| 0 Baseline + ruff | Coordinator | no | none | 40m | no |
| 1 `.config/cs` docs | Agent | [P] | 0 | 45m | reads `~/.config/vor` |
| 2 ADR index + mirrors | Agent | [P] | 0 | 2h | no |
| 3 D-28 drift | Agent (Librarian lane for wiki) | [P] | 0 | 2h | no |
| 4 Compliance fold | Agent (Developer) | [P] | 0 | 90m | no |
| 5 Zhen residue | Agent (Developer) | [P] | 0 | 2h | docker for Step 140 |
| 6 Deploy files | Agent | [P] | 0 | 60m | no |
| 7 Housekeeping | Coordinator + Stevie | after 3 (same wiki page) | 0 | 40m | Steps 163-164 Stevie; 167-168 user files |
| 8 Prompt half | Coordinator | gated | 1, llama-server up | 30m + eval | starts a model run |
| 9 Final | Coordinator | no | 0-7 | 40m | no |

File collisions between parallel lanes: `wiki/Systemd-Units.md` (Phases 3 and 7: run 7 after 3); the progress file (every phase appends; append only, commit with the phase). Against S-WATCH-TRUE: `tests/runbooks/test_ioc_audit.py`, `nixos/modules/services/trace-collector.nix`, `docs/adr/ADR-INDEX.md`, `compliance/findings/register.yaml` (Grounding, Busboy).

**Critical path**: 0 → 5 → 9 (about 3.5 h). Phase 8 is off the path, gated.

## APPENDIX C: QUICK REFERENCE

| Thing | Value |
|---|---|
| CI venv (ruff 0.17.3, pip-audit, pytest) | `/var/tmp/unheaded-sprint/civenv/bin`, LAST on PATH |
| raft venv | `/var/tmp/unheaded-sprint/raftvenv` |
| Timeline milestone id | `phase-3-item-` + `sha256(bullet text after "- ")[:8]` |
| Timeline files | `references/timeline.md` (source), `.json` (other fields' source), `.yaml`, `.toml` |
| ADR status count rule | first word of each status, case and trailing punctuation ignored (`ADR-INDEX.md` line 19) |
| Root CLI commands | `root.go:112-124`: container service config secret network status version completion generate observe apikey token hop |
| zhen-mcp tools | read-only: corpus_search, code_definition, code_callers, file_read, service_health, runbook_list, runbook_show, local_llm; opt-in: file_write, file_patch, runbook_execute |
| Queue-only tools | `champion.QueueOnlyTools` (`pkg/champion/confirm.go:163`) |
| wellint harness | `scripts/well/test-llm-requests.sh` (throwaway Postgres, `WELL_TEST_DSN`, never the real Well) |
| Man pages | `scripts/gen-man.sh` (check; ignores `.TH`), `--write` regenerates all |
| Eval (ADR-024) | `go run ./cmd/zhen-eval -backend local ...`, `eval/champion-runbooks/README.md` |

---

*S-RESIDUE Battle Plan, forged 2026-11-12*
*10 phases. 176 steps. Sweep the hall after the feast, so the next one starts on a clean floor.*
