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
