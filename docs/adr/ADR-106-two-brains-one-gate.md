<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-106: Two brains, one gate

**Status:** Proposed (Stevie to accept).
**Date:** 2026-10-23
**Sprint:** S-TWO-BRAINS (`docs/battle-plans/S-TWO-BRAINS-battle-plan.md`; ledger `docs/battle-plans/S-TWO-BRAINS-progress.md`).
**Related:** ADR-017 (hybrid inference), ADR-019 (Champion; Level 2 amendment), ADR-024 (runbooks), ADR-030 (forge), ADR-053 (Claude/Zhenai workflow templates), ADR-101 (Well migration ledger), ADR-104 (Claude API bridge).

## Context

ADR-104 gave Zhen a second brain: the Claude API, called from
`pkg/claudebridge` with the Kingdom's tools behind the Champion gate. That
left open how a write gets approved without the model approving it, what
data may leave west, which brain answers which task, how a Kanban card
becomes work, and whether either brain's output may train the local model.
This ADR records the answers the sprint built and tested. Both brains go
through the same gate, the same audit and the same privacy tier; they
differ only in where the tokens are produced.

## Decision

### 1. Confirmation is out of band; the code never enters model context

A write the Champion would refuse for lack of a human goes to a
`Confirmer` (`pkg/champion/confirm.go`): `deny` (the default), `tty`, or
`queue`. Only the Champion's own write tools are confirmable.

- **tty**: the prompt (tool, sandbox, hash, redacted args, diff) and a
  4-character code (31-char alphabet, no look-alikes) are printed on
  `/dev/tty` and the answer is read from it. The code is never in a
  request body, a tool result or a log line (no-leak tests; a model that
  writes "approve", codes or "yes" in its text approves nothing).
- **queue**: a row in `unheaded_app.zhen_pending_confirmations`
  (migration 031) that Stevie decides with `zhen-claude confirm
  list|show|approve|reject <id>`. One-way states in a trigger; approval
  after `expires_at` refused; 20 pending per `source_service`, in code and
  in the trigger.
- The approval binds a hash of `{tool, args, project_root}` (canonical
  JSON). `approve` re-reads the row, re-hashes and shows the diff it will
  run. Approval for root A does not run under root B.
- On `/tool/exec`, an agent loop cannot claim the user's trust:
  `direct-user` is no longer the default, and agent-loop callers cannot
  assert it.

BlackMage review (Step 86): 9 attacks, 2 found open and fixed (symlink
escape from the sandbox; agent loop as `direct-user`). Residual: a
symlink swap between the path check and the write (`openat2
RESOLVE_BENEATH`, not done).

### 2. Branch-sandbox writes (ADR-019 Level 2 amendment)

An agent working a card writes only inside a worktree on a `zhen/<card>`
branch (`pkg/champion/branchsandbox.go`). There the sandbox stands in for
the confirmation on `write_file` and `patch_file`, recorded as
`branch-sandbox:<branch>`. Nothing under `.git` is writable, and the
branch is re-checked on every call. Stevie reviews the branch; nothing
merges or pushes on its own. Written up as a Proposed amendment in ADR-019.

### 3. Privacy tiers

Every tool result and prompt is classified before it can reach Claude
(`pkg/claudebridge/privacy`):

| Tier | What | To Claude |
|---|---|---|
| T0 | tracked repository text (`git ls-files` at the realpath), after a secret scan | as is |
| T1 | tool output, unknown tools | redacted: secrets (gitleaks over stdin), addresses, MACs, emails, home user names, host names; over 8 KiB, a local summary of the redacted text, redacted again |
| T2 | user data roots | never |

`-privacy strict` withholds all T1 (forced for unattended runs). If the
redactor cannot run, nothing is sent. Residual risks are listed in the
ledger (Step 117): the patterns are a deny list; host names are a
configured list.

### 4. The eval and pre-registered thresholds

Which brain answers a task family is decided by `cmd/zhen-eval` over
`eval/two-brains/`: 76 scored tasks across the families `syntax`, `review`,
`hard`, `expert`, `prod` and `repo-task` (`runbook-draft` came later, with
the Phase 8 harness), keys and a tune/hold-out
split written and committed (`PREREG.md`) before any result. A family goes
to the local model only if it holds up on the hold-out half under the
pre-registered rule. Result (`RESULTS-2026-10-23.md`, $1.31): local
qwen-coder-14b for `hard` and `repo-task` (`local_only`) and `expert`
(`local_first`); `prod`, `review`, `syntax` and anything unlisted go
`claude_direct`. The generated `configs/zhen-routing.yaml` carries the
split and PREREG hashes. The judges refuse any answer that edits the
fixture's check or test files.

### 5. `run_check` sandbox

A model-written change is judged by a named check from
`configs/zhen-checks.yaml`, run by `pkg/sandbox` in a transient
`systemd-run --user` unit: no network, `ProtectHome=tmpfs` with the Go
toolchain bound read-only, `/etc/unheaded`, `/var/lib/unheaded` and
`/srv/backups` inaccessible, `PrivateTmp`, `MemoryMax`, `TasksMax=256`,
600 s runtime cap, only the configured environment. The router
(`claudebridge.ThresholdRouter`) obeys the routing config, escalates at
most once with redacted check output, and logs a `route_reason` for every
decision.

### 6. Kanban eligibility and claims

`zhen-agentd -worker` takes a card only if it is owned by `zhen`, in
`todo`, of type task/tech-debt/bug/docs, with a strict `zhen` block. A
claim is a row in `unheaded_app.zhen_agent_runs` (migration 032; a partial
unique index makes it exclusive; 8-way race tested) with a 30-minute lease
renewed every 5 minutes. Two attempts per card. A card id that does not
fit `^[a-z0-9][a-z0-9-]{0,59}$` is skipped, never rewritten; an existing
branch or worktree is never overwritten. Commits are unsigned, authored by
Zhenai, with `Zhen-*` trailers; Stevie accepts or rejects with the review
CLI (one-way, audited).

### 7. One GPU, and overnight guards

One `flock` lease on `$XDG_RUNTIME_DIR/unheaded/gpu.lock`: shared per
inference call, exclusive for `switch-model.sh` and `forge-train.sh` for
their whole run. A held lease defers a card; it never escalates it to
Claude. `zhen-agentd -overnight` stops on: a stop file, the overnight spend
cap (its own ledger), a MemAvailable floor that never counts swap and can
only be raised, a window, or a signal. Every guard has a test seen
failing. No unit is enabled; the first real night is Stevie's (Step 254).

### 8. The flywheel, and Claude-authored text

Captures (`zhen-capture/v1`) are ingested into
`unheaded_app.zhen_captures` (migration 033: 256 KiB cap on what is
stored, purge after 180 days, set by trigger, one-way quarantine) and
labelled. `zhen-flywheel export` writes SFT, preference and tool-trace
sets, split by task family and group, contamination-checked against the
eval, gitleaks-clean, round-tripped through the forge's `--strict-data`
(H6-safe).

An SFT example needs a passing outcome (`check_pass`, `judge_pass` or
`accepted`) and no failing one. Human-authored text needs no outcome.
Outcomes and verdicts are labels, not text.

**Claude-authored text is not a training target unless
`-include-claude-text` (or `ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT=1`) is set.
The default is OFF.** Claude text is still captured and stored, tagged by
`author_kind`, so turning it on is one commit, not a re-capture. Stevie's
rationale, verbatim (2026-10-23): *"not competition i dont think - fine
tuning an existing model but each environment/graphics card/setup unheaded
exist in would be different - i can ask support"* and *"it only works with
claude and promotes claude use"*. The default stays OFF until Anthropic
support confirms in writing (Step 340, open; support chat escalated to a
human 2026-10-23). On a written yes, one commit flips the default and
cites the reply's date here.

### 9. Which model learns

The forge's only training path is Gemma-4 E2B (ADR-030; qwen cannot be
trained by the forge). A LoRA (rank 16, alpha 32, lr 1e-3) is evaluated
through the forge on the flywheel's hold-out, and a family is routed to it
only if it beats the current local default on that family's hold-out under
PREREG and passes the 7-language coding gate. Otherwise the result is
recorded and routing does not change. Not run: the export has 0 SFT train
examples (needs at least 300 across 3 families), and the training window is
Stevie's (Step 400).

### Audit (both brains)

Every request of either brain is a row in `unheaded_ops.llm_requests`
(migration 030); every gate decision and confirmation step is an
`audit_events` row; both are published to Wotan (`zhen.llm.requests`,
`zhen.audit.toolcalls`). Sizes and hashes only, never prompt or answer
text. A Well that cannot be reached makes `zhen-claude` read-only.

## Consequences

- A Claude-proposed write runs only after Stevie approves it out of band,
  or inside a `zhen/*` branch he reviews.
- The routing table is evidence, not opinion; changing it means re-running
  the eval with a new pre-registration.
- Five new Well tables (030-034; 034 moves the Kanban Genesis seed out of
  the `kanban-app` binary into a guarded data migration). None is applied
  to the live Well yet.
- 11 new runbooks with dry-run tests and a shrink-only checker (ADR-024).
- Sprint spend on the Claude API: $1.38 (Phases 0-5), $0.03 (6-7), $2.37
  (8-9).

## Open questions

Every [ESCALATE] still open:

1. **Live apply of 030-034 as one batch** (Steps 59, 94, 205, 349, and 034's
   recorded no-op). Until then, `-confirm queue` and the worker cannot
   run against the live Well.
2. **First real unattended night** (Step 254): when, the cap, the env file.
3. **Runbook approvals** (Step 337): the 11 drafts are `status: draft`.
4. **Anthropic's written answer on Claude text as training targets**
   (Step 340). The flag stays OFF.
5. **The LoRA window** (Step 400), once there are enough examples.
6. **ADR-019 Level 2 amendment**: accept or reject.
7. **Two routers**: `ThresholdRouter` (this sprint: per family, from the
   eval) and `AutoRouter` (S-LICH-AWAKENS: the ADR-053 `Router` weight
   classifier for the Lich drivers) both live in `pkg/claudebridge`. They
   do not conflict; whether one should feed the other is open.
