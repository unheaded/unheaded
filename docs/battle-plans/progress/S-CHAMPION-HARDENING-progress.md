<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-CHAMPION-HARDENING progress log

Plan: `docs/battle-plans/S-CHAMPION-HARDENING-battle-plan.md`. This file is the session log the plan refers to, and where
`[STUCK]` / `[ESCALATE]` markers live; step ticks live in the plan.

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed debug attempts, mark
it [STUCK] with what I tried, commit known-good state first, continue with the next step that does not depend
on it, and finish with a STUCK/ESCALATE report ordered by how many steps each item unblocks."

## Session S24, 2026-10-27 evening (unattended, Stevie's "churn unattended multi agent")

Base: develop `1f44cc46` (S-CLEAN-SLATE shipped; Go known failures 0). Same pattern as Sprint 1: a coordinator
on develop and worktree agents under one brief (`EXEC-BRIEF-v2`: reset to develop first, never push, no host
changes, no shared-ledger edits, gate exit codes read directly). Phase 0 premises are checked per slice by each
agent, since SITUATION was written at `a7fe3acd` and Sprint 1 changed many of them.

First wave: Phases 1-3 (pkg/champion) and Phase 4 (pkg/zhenrag) in two worktree agents; next: Phase 5 (routers), then 6-7 after Phase 4 merges; Phase 4 must not change prompt text silently (the coding gate needs a model run by the coordinator).

### Phases 1-3 merged 7d35d4ad; Phase 4 merged 9c1c4e25

Both merged cleanly (no conflict in `toolcall.go`: the allowlist and `destructivePattern = zhenrag.DestructivePattern`
both survived). Post-merge `-race` green on zhenrag, champion, agent, claudebridge, zhenroute, zhen-agentd,
zhen-claude, lich-mcp, zhen-rag, zhen-cli; gitleaks 0; gosec ratchet 0; runbook manifest check 0.

Phase 0 corrections: H3 (Step 11) had **four** prompt copies, not three (`cmd/zhen-cli` had its own paraphrase);
the verb prose in `zhen_rag.py` was at line 271, not 200-215.

Step notes:
- Step 44: `FuzzCanonicalCall` does not exist; `FuzzPendingCallFor` was extended and run 30 s clean.
- Step 55: `delete_file` and `kanban_delete` (listed, never implemented) were removed with `system_command`;
  dispatch refuses unknown tools before the gate runs.
- Step 43: the queue table has no requester column, so queued calls bind no requester and only `queue:stevie`
  (`zhen-claude confirm approve`) redeems them; an empty requester hashes as before (old rows still match).
- Step 49: lazy token-store creation raced; now locked.
- Steps 57/64: ran but skip without the live webapp/Postgres (`WELL_TEST_APP_DSN` unset).
- Step 38: the agent stamps digests on vor references from the text the model read; agentd's retriever sets
  none itself (RT-14/16 partial).

Behaviour changes to know:
- `ConfirmPendingToolCall(ctx, token, approvedBy)` takes the confirmer; zhen-agentd with `-no-auth` gets 403 on
  confirm. Behind the Zhen UI's single API key all UI users share one principal.
- vor references with empty/unlisted `source_trust` are untrusted.
- zhen-cli now sends zhen-rag's full chat prompt (1079 -> 3323 bytes); not on the coding-gate path.
- Live `runbook_execute` needs the runbook's sha256 in `runbooks/MANIFEST.sha256` (79 entries) and refuses an
  unapproved runbook whose commands hit the verb pattern. Eight legacy runbooks are refused for live Champion
  execution until approved (E1 candidates): infra/apt-repo-server, infra/kernel-upgrade, network/dns-pihole-lxd,
  pki/generate-kingdom-certs, pki/rotate-service-cert, security/secrets-rotation, security/security-audit,
  security/ssh-hardening. `run-runbook.py` by hand is unchanged.
- After any edit to `runbooks/*.yaml`: `scripts/runbook-manifest.sh --write`.
- `switch-model.sh` refuses a GGUF off its pin (exit 4); `--dry-run` added. zhen-agentd must restart after
  `switch-model.sh` changes (ModelSwap hashes it at start).
- `tests/runbooks::test_wotan_pending_approval` expects 401 and gets 404: Sprint 1 turned Wotan's admin API off by
  default. Test needs updating (not caused by Phase 4).

Steps 90-92 / 109-110 (coding gate): run by the coordinator as a pair of 5-seed sweeps, baseline `1c87b996` vs
merged tree, same seeds, qwen-7b via `switch-model.sh` (pin ok, fit ok).

### Phase 6 (coordinator): Steps 125-136 done; E1 filed

Policy written in ADR-024 "Authorship and approval" (pointer under ADR-106 Q3). `check-runbooks.py` gained
`approved-at` (ISO date required when approved) and a bar rule: an approved `zhen-local` runbook fails unless the
newest `eval/champion-runbooks/RESULTS-<date>.md` shows a local `**all**` row without ✗ (today: none clear it).
Deviation: the plan's author set `{claude, stevie, zhen-local}` kept the checker's existing `human` for Stevie
rather than renaming a value the corpus already uses. 39 checker tests pass; corpus 0 new findings. All eleven drafts
dry-run `[SUCCESS]`. E1 packet: `S-CHAMPION-HARDENING-approvals.md` (also lists the eight legacy runbooks Phase 4
blocks from live execution). Steps 137-141 wait on E1; Phase 7 does not.

### Phase 5 merged (routers, caps; E3 filed)

Both routers kept: `*ThresholdRouter` satisfies the existing `Router` interface (claude_direct = heavy,
local_only/local_first = minor; unknown family -> `ErrNoFamilyRoute`); `AutoRouter.Fallback` (nil default) is
asked only when it cannot classify. F-23 was worse than "no headroom": a routed leg sized its first request to
~70% of the family cap, so the tool loop's second request never fit and every leg stopped after one request.
Now `Ledger.Remaining/WorstCase/FitMaxTokens`, `Config.MinMaxTokens` (1024 for routed legs); with fallbacks off the
worst case is priced at the routed model (routed legs get ~25% more output tokens). zhen-claude defaults unchanged.
Step 114's "refused before the API call" was already true. **Branch-sandbox mode is live with no off switch**:
the code already behaves as if the ADR-019 L2 amendment were accepted; ADR-019 now says what rejecting changes.
Steps 118-119 wait on E3. ADR-019 checklist closed by cross-reference except config template library and the
no-human escalation path (E2).

### Phase 7 merged

The five `runbook-draft` tasks had no prompts (`ref: "blocked: Phase 8 harness"`); prompts, requirements and
reference drafts were written before any result existed (`eval/two-brains/runbook-draft/<id>/`). Judge
`runbook` (kept its registered name; contract is a dated amendment in `eval/two-brains/PREREG.md`): dry-run exit 0
AND `check-runbooks.py --one` (new: one file, strict, draft required) AND requires/forbids regexes; per-check
results in a new `judges` field. zhen-eval now refuses an unknown judge. `run-runbook.py` runs `verification:`
after the steps; a failed verification exits 1 (no rollback), which the scheduler, MCP tool and web app read.
`check-runbooks.py` now `bash -n`s verification commands. tests/runbooks 119 passed after merge.
Step 148 (live eval on the local model) is LIVE for the coordinator: `zhen-eval -family runbook-draft -limit 3
-out <scratch>` (the plan's `-families` flag does not exist; fixed in the plan text).
Correction to the agent's note: `wotan/active-active-cutover.yaml` (TODO verification) is `status: stub`, not one
of the E1 drafts.

### Phase 10 merged (637a0925)

`cmd/zhen-mcp` (9 tools; file/runbook tools via `Dispatch`, others via `AcceptToolCall`), `pkg/mcpserver` extracted
from lich-mcp, `scripts/zhen-mcp.sh` runs `bin/zhen-mcp`, `scripts/check-zhen-mcp.sh` in ci.yml's Go job.
Python server and its `mcp` dependency retired (`sbom/python-requirements.txt` still lists `mcp==1.30.0`: regenerate).
Fixed in pkg/champion: `RunbookExecute` followed a symlink out of `runbooks/` (RunbookShow refused it).
**Phase 14 decision**: with `ZHEN_MCP_ALLOW_WRITE/EXEC` set, enabled calls carry a `direct-user` reference naming
the flag (else the gate refuses every write); conflicts with Step 86's "a model loop never asserts direct-user".
Alternative: pending confirmation via a lich-mcp-style confirm flow. Behaviour: destructive-verb rule now applies to
`corpus_search`/`local_llm` text; `service_health` checks kanban on 20001 and llama-server :8081. Stricter than the
Python server: `..` in names, substring deny list (`secrets_manager.go`), a checkout path containing `secrets`/
`credentials`/`.env` denies everything. `bin/zhen-mcp` built in the main checkout after merge (the live `zhen` MCP
server needs it). The E1 runbook `security/mcp-write-exec-enablement.yaml` and its brief still describe the Python
server; left untouched while it is in front of Stevie.

### Phase 11 merged (91462794)

agentd: `GET /api/v1/memory/recall`, `POST /api/v1/memory/remember` (Postgres full-text, not embeddings), `GET /api/v1/models`,
`-memory-store none|pg`; new mutating tool `memory_write` (authenticated caller = direct-user; `-no-auth` refused).
Fixed: a confirmed `model_switch` failed as "unimplemented" in `dispatchUnderlying`. zhen-cli: `/recall /remember
/confirm /model`, line editing + history; key from `ZHEN_API_KEY`/`ZHEN_AGENTD_API_KEY`/`~/.config/unheaded/zhen-cli.conf`
(0600). `switch-model.sh --status/--json` (no GPU lease), swap rollback (4x load time + 60 s, own process group),
`zhen_model_swap_duration_seconds`, `zhen_model_swap_failures_total`. `MODEL_LOAD_SECONDS` for deepseek (90) and
qwen-coder-14b (120) are estimates: measure. Premise that did not hold: `/confirm` cannot send a tty principal
(agentd derives `ui:<subject>`; one API key = shared `ui:apikey` with the web UI until per-owner keys, now possible
with Iron Gate Phase 3's `uhk_` keys). LICH-013 campaign file created; smoke-tested; known gap: allowlist and
in-progress refusals answer 500, not ADR-060's 400/429. LIVE: `TestPGMemoryStore_Live` (needs `WELL_TEST_APP_DSN`),
a live `/model` swap, `-memory-store pg` in the agentd unit (E4).

### Coding gate (Steps 90-92, 109-110): PASS, no regression attributable to Phases 1-11

Two 5-seed sweeps of `make coding-gate-ci` on qwen-7b (pin ok, fit ok, loaded through `switch-model.sh`), `cs serve`
for vor: baseline at `1c87b996` (before any Champion merge, so before pkg/zhenrag) in a scratch worktree, then the
merged tree (`bin/zhen-rag` at 051847a6). Each sweep hit the 40-minute cap during seed 999, so both are graded on the
same four seeds with the script's own grader:

| seed | baseline PASS | merged PASS |
|---|---|---|
| 42 | 12 | 10 |
| 137 | 12 | 11 |
| 314 | 9 | 9 |
| 271 | 10 | 10 |

Baseline band [9, 12] (width 3: the WARN threshold is crossed by the baseline alone); merged band [9, 11]; no
seed moves by more than 2; red flags 1 -> 0. Same GGUF, same vor fingerprint, yet the answer text differs even on
seeds with equal counts: generation is not reproducible run to run on this setup, so per-seed differences are noise.
The prompt bytes are proven identical to the pre-Phase-4 prompt by `pkg/zhenrag`'s goldens test. Gate note for next
time: a 5-seed sweep takes about 50 minutes on west under load; use a 70-minute cap.

### Step 148 live eval (local)

`zhen-eval -backend local -model qwen-7b -family runbook-draft -limit 3`: 2/3 passed (wotan-restart and
well-vacuum-report pass all three judges; akira-band-check: dry_run pass, check-runbooks fail, requirements pass).
llama-server and `cs serve` stopped afterwards (nothing was serving before).

### Phase 8 merged (fae5c9af)

Scheduled jobs: `observe/service-health-sweep` every 6 h (Python had 30 m = 48 cards/day);
`data/conversation-memory-maintenance` weekly, disabled (its steps use `sudo docker exec`, which the unit forbids).
No draft is scheduled. Migration 037 (`zhen_agent_runs` mode `scheduled`) pending live with 030-036. `app_zhen` cannot
insert `kanban_tasks`, so `-kanban-store=pg` reads `WELL_KANBAN_DSN` first: set `WELL_KANBAN_DSN` and `WELL_APP_ZHEN_DSN` at
E4. New `-metrics-addr` (`[fd00:dead:beef::1]:20105`) + scrape job `zhen-agentd`; `ZhenSchedulerStalled` fires (warning,
none) until E4, which is accurate. E4 prerequisite: `/var/zhen/projects/default` must hold `runbooks/`,
`MANIFEST.sha256`, `scripts/run-runbook.py`. `zhen-claude review` refuses scheduled runs.

### Phase 9 (coordinator): Steps 177-178, 182, 183 done; 179-181 wait on a port decision (Stevie, with E4)

- 182 (d29fc809): `_strip_tool_call_json` let a payload through when it nested deeper than one level, had a brace in
  a string, or was truncated (the regex matched one fixed depth). Now a JSON-aware brace scan; 9 tests, 4 red on the
  old code; in raft CI.
- 183 (a76214ae): recalled memories never enter the prompt (side-channel `matched_memory` only, by design), so the
  marker `[memory: <topic> @ <date> via well]` + `created_at` go on that side-channel. pkg/vorquery has no memory path.
  Consolidation job stays disabled (Phase 8: its steps need `sudo docker exec`).
- **179-181 [ESCALATE, folded into E4]: ports.** vor cannot be registered at its default 9876: `pkg/ports` requires
  every port in 16666-26666 (TestAllPortsInDoomRange). The AI band 20100-20106 is full and 20107 is taken by
  sophia-eye. And zhen-agentd's 20105 collides with `SophiaEye = 20105`, which deploy/sophia-eye publishes on the host
  (`20105:8080`; gateway also `20107:20105`). Proposal: `ZhenVor = 20110`, `ZhenAgentd = 20111` (both free), change the
  ~20 callers' defaults (zhen-rag, zhen-agentd, zhen-agent, zhen-mcp, zhen-cli, raft, coding-gate scripts, Dockerfile,
  unit, docs) in one commit, and run vor as `unheaded-vor` (the next free Kingdom-tier uid; 16736 went to unheaded-heimdall in S-IRON-GATE Phase 9; hostOnly pin) with
  `HOME=/var/lib/unheaded/vor` and a `git archive` export of the repo as its `unheaded` source (vor finds sources via
  `$HOME/.config/cs/sources/`; the repo lives under /home, which a hardened unit cannot read). Not done unattended:
  moving the ports breaks the manual `cs serve` + coding-gate workflow until the unit is installed.

### Phase 12 merged (3ed2eb49)

Kanban: `GET /api/v1/runbooks` (from MANIFEST + front matter, `manifest_ok`), `POST /api/v1/tasks/from-runbook` (board
store, runbook name in a fenced `runbook` block since kanban_tasks has no metadata column), `agent_run` block on
`GET /api/v1/tasks/{id}` (branch, `route_summary->>'review_note'` path, `review_reason`, claimed/reviewed; there is no
`review_note` or `finished_at` column). UI in `cmd/kanban-app/static/` (the embedded one; `kanban/` is an old scaffold).
Step progress: `run-runbook.py` posts each step to Wotan `runbooks.progress` (opt-in `RUNBOOK_PROGRESS_WOTAN`; never
changes the outcome); the Champion sets `RUNBOOK_REF`. Migration renumbered to 041 (app_kanban column grants on
zhen_agent_runs). ADR-038 dormant. LIVE Step 227 after a kanban rebuild (and the E-A batch, which now runs through
041): `curl -s 127.0.0.1:20001/api/v1/runbooks | jq '.runbooks|length'` (expect 90); restart Wotan for the
`runbook-runner` auto_approve line; set `RUNBOOK_PROGRESS_WOTAN` in zhen-agentd's env. Pre-existing oddity: without
Wotan and with Timeguru answering, `GET /api/v1/tasks` returns timeline cards and hides the store's cards.

### Phase 13 (coordinator): prepared, E2 filed

The overnight worker already existed (S-TWO-BRAINS): `zhen-agentd -worker -overnight` with -window, -max-cards, its own
ledger and cap, strict privacy, a memory floor that never counts swap, a -stop-file kill switch and a morning report
it writes itself. So the plan's env names (`ZHEN_OVERNIGHT`, `ZHEN_ABORT_FILE`, `ZHEN_CARD_FILTER`) and an agentd
`/api/v1/runs` report were not added; instead: `deploy/systemd/zhen-agentd-overnight.{service,timer}` (not enabled,
`-claude fake`), env example overnight block, `runbooks/observe/zhen-overnight-abort.yaml` (draft, 5 tests), and
`scripts/zhen-morning-report.sh` (prints the newest/dated report the worker wrote). Found: the GPU lease default is
per user (`$XDG_RUNTIME_DIR`), so a zhen-agentd unit and the operator's `switch-model.sh` would lock different files;
the unit sets `GPU_LOCK=/run/unheaded/gpu.lock` and E2 asks for the shared group. 20 overnight/worker tests pass.

### Phase 14 (coordinator): security review written, gates green

`docs/security/S-CHAMPION-HARDENING-review.md`: attack-surface delta for every change, 10 gates exit 0, both fuzz
targets 45 s clean, go.mod only the sqlite bump, full suite 232 ok / 0 FAIL. Open decisions listed there (MCP flags vs
Step 86, branch-sandbox E3, shared UI principal).

Correction to Phase 13 (coordinator): the overnight run already had user units from S-TWO-BRAINS
(`deploy/systemd/user/zhen-agentd-overnight.{service,timer}`, running as the operator on the real repo). The system
unit + timer written in 329a6678 duplicated them and introduced the GPU-lease mismatch it then warned about; both
removed. The kill-switch runbook, morning-report script (default `${XDG_STATE_HOME:-~/.local/state}/unheaded/zhen-reports`),
env example and E2 packet now point at the user units (no sudo).
