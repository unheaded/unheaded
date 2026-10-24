# Zhen Routing

Which brain answers a task, the local model (llama-server on west) or Claude, is decided per **task family** by a table generated from measured data, not by guesswork: `configs/zhen-routing.yaml`.

## Routes

| Route | What happens |
|---|---|
| `local_only` | The local model answers; Claude is never called, even if the answer fails its check. |
| `local_first` | The local model drafts (repo tasks: 2 attempts, the second sees the check output). If the draft fails in a way listed in `escalate_on` (`check_fail`, `parse_fail`, `judge_fail`, `refusal`), Claude gets one escalation with the task, the last draft and the redacted check output, within `max_claude_usd` for that task. |
| `claude_direct` | Claude answers; the local model is skipped. |

An unknown family uses `default` (`claude_direct`, $0.25).

## Where the thresholds come from

The two-brains eval (`eval/two-brains/`): 56 coding-gate prompts judged by pre-registered regex keys and 20 repo-task fixtures judged by their own `check.sh` in the no-network sandbox, run on the local model(s) and on Claude. `eval/two-brains/PREREG.md` fixed the rule before any result existed:

- on the **tune** half: `local_only` if local passes >= 85 % and Claude is not >= 10 points better; else `local_first` if local passes >= 30 %; else `claude_direct`;
- on the **hold-out** half, a family's route is adopted only if it keeps Claude's pass rate within 5 points at <= 50 % of Claude's cost and never loses passes to local-only; otherwise the family is written as `claude_direct`.

Numbers: `eval/two-brains/RESULTS-2026-10-23.md`. The table as of 2026-10-23 (local model qwen-coder-14b, Claude Opus 5.5 at effort low):

| Family | Route | Why |
|---|---|---|
| `hard` | `local_only` | local 7/7 on tune and on hold-out |
| `repo-task` | `local_only` | local 12/13 tune, 7/7 hold-out, at $0 vs Claude's $0.008/task |
| `expert` | `local_first` | local 6/7 tune; on hold-out routed 7/7 at 24 % of Claude-only cost |
| `prod` | `claude_direct` | `local_first` rejected on hold-out: it kept every pass but saved too little (74 % of Claude's cost) |
| `syntax`, `review` (textbook) | `claude_direct` | `local_only` rejected on hold-out: 9/11 against Claude's 11/11 |
| anything else | `claude_direct` | default |

Caveats: the keys are stricter than the human rubric on review answers, and qwen-7b's repo-task score includes one pass that weakened a test (fixed since: tests are protected).

Re-run:

```bash
scripts/switch-model.sh qwen-7b                     # fit-checked; never ZHEN_SKIP_FIT_CHECK
go run ./cmd/zhen-eval -backend local -model qwen-7b -out eval/two-brains/results-<date>-qwen7b.jsonl
go run ./cmd/zhen-eval -backend claude -effort low -daily-cap-usd 3 -out eval/two-brains/results-<date>-opus55-low.jsonl
python3 scripts/two-brains/thresholds.py --table eval/two-brains/RESULTS-<date>.md eval/two-brains/results-<date>-*.jsonl
python3 scripts/two-brains/thresholds.py --verify-holdout configs/zhen-routing.yaml --table eval/two-brains/RESULTS-<date>.md eval/two-brains/results-<date>-*.jsonl
```

Never hand-edit the config; change PREREG.md first (in its own commit, before new results) to change the rule.

## The Champion runbook eval

A second eval, `eval/champion-runbooks/` (100 tasks, 20 per family), tests Zhenai as the Champion for runbooks and workflows: `rb-select` (pick one of the 79 runbooks for a symptom), `rb-read` (questions about a real runbook), `rb-safety` (the gate, runbook strictness, privacy tiers, credentials), `workflow` (routing, Kanban claims, branch audits, promotion) and `rb-author` (write or fix a runbook that passes a strict check in the sandbox). Same runner and judges as above; the bar is pre-registered in `eval/champion-runbooks/PREREG.md` (hold-out: `rb-safety` >= 0.90, other families >= 0.80, overall >= 0.85). How to run it: `eval/champion-runbooks/README.md`.

## Running one task

```bash
zhen-claude -route auto -family repo-task -worktree ~/.cache/unheaded/zhen-worktrees/<card> \
    -run-check go-test -check-path ./pkg/x/... -effort low -p "$(cat TASK.md)"
```

- `-worktree` must be an agent worktree (under `-agent-root`, default `~/.cache/unheaded/zhen-worktrees`) on a `zhen/<name>` branch: drafts are written there in branch-sandbox mode (ADR-019, Proposed amendment), never in the main checkout.
- `-run-check` names an allowlisted check from `configs/zhen-checks.yaml`; it runs in the sandbox (no network, home hidden, only the worktree and build caches writable). A draft may not change `check.sh`, `TASK.md` or `solution.patch`, and a check does not count if one of them changed.
- `-route local` is the same loop with Claude switched off.
- Output: one JSON line `{route, route_reason, local_attempts, escalated, check, cost_usd, stop}`. `stop` is `escalation_budget` (the per-task cap or the daily cap ran out), `not_escalated` (the failure kind is not in `escalate_on`) or `local_only`.

`scripts/two-brains/run-fixture.sh --route auto --family repo-task --split holdout --limit 3` runs eval fixtures this way.

## route_reason values

`local_first`, `local_only`, `escalated_check_fail`, `escalated_parse_fail`, `escalated_refusal`, `claude_direct`, `manual` (plain `-route claude`). They land in `llm_requests.route_reason` ([[Zhen Audit|Zhen-Audit]]).

## Metrics

`zhen_route_total{family,route,outcome}`, `zhen_escalations_total{family,reason}`, `zhen_check_runs_total{check,result}` on the default registry (zhen-agentd's `/metrics`).

---

> **Source:** [ADR-053](../docs/adr/ADR-053-hybrid-claude-zhenai-workflow-templates.md) · [S-TWO-BRAINS plan](../docs/battle-plans/S-TWO-BRAINS-battle-plan.md) Phases 4-5 · `pkg/claudebridge/router.go` · `pkg/champion/runcheck.go` · `pkg/sandbox/`
