<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Two-brains eval: pre-registered routing rule

Written 2026-10-23, committed before any result in `eval/two-brains/results-*`
exists (S-TWO-BRAINS Steps 131-133). The commit history proves the order:
`git log --format=%ci -- eval/two-brains/PREREG.md eval/two-brains/results-*`.

## Data

- Tasks: `tasks.jsonl` (81 rows). Families: `syntax` (7), `review` (7)
  (together the 14 gate-binding textbook prompts of `eval/coding-gate`,
  RUBRIC v2.3), `hard`, `expert`, `prod` (14 each, informational),
  `repo-task` (20 fixtures), `runbook-draft` (5, blocked until the Phase 8
  harness; not scored here).
- Split: `split.jsonl`, `int(sha256(id)) % 2` (0 = holdout, 1 = tune),
  fixed by id alone.
- Judges are mechanical; Claude never grades Claude.
  - `key`: `keys.jsonl`, case-insensitive regexes; pass = every `all_of`
    matches and, if `any_of` is non-empty, at least one of it. An empty or
    errored answer fails.
  - `check`: the answer's file blocks are applied to a fresh copy of the
    fixture (paths must stay inside it; `check.sh`, `TASK.md` and
    `solution.patch` cannot be changed), then `check.sh` runs in the
    no-network user sandbox; pass = exit 0.

**Amendment 2026-10-27 (S-CHAMPION-HARDENING Steps 144-145): the
`runbook-draft` judge.** Written with the five tasks' prompts and
requirements, before any `runbook-draft` result exists. The family's rule,
hold-out acceptance and attempts are the ones below (`local_attempts` = 1).

  - `runbook`: `runbook-draft/<id>/TASK.md` is the prompt (the system prompt
    is `systemRunbook` in `cmd/zhen-eval/runbook.go`, the same for both
    brains); `requirements.json` holds the runbook `name`, `requires` and
    `forbids` regexes (RE2, case-sensitive unless the pattern says `(?i)`).
    The answer's first ```` ```file:<x>.yaml ```` block (else its first
    ```` ```yaml ```` fence) is written to `<name>.yaml` in a temp dir, then
    three mechanical checks run and each is recorded in the result's
    `judges`: `dry_run` = `scripts/run-runbook.py --dry-run` exits 0;
    `check` = `scripts/check-runbooks.py --one` exits 0 (the strict draft
    rules, `metadata.status: draft` required, no baseline, no `dry_run_test`
    rule since a draft has no test yet; `metadata.name` must equal `name`);
    `requirements` = every `requires` matches the YAML text and no `forbids`
    does. Pass = all three. No answer, no YAML block, or a YAML over 256 KiB
    fails. Nothing in the draft executes (the dry run prints commands, the
    checker only runs `bash -n`), so no sandbox is needed; each script has a
    60 s timeout. `reference.yaml` beside each task passes the judge (tested
    in `cmd/zhen-eval`) and is never shown to a model.

**Pooled textbook family.** The hash split leaves `review` with 1 tune task
and `syntax` with 2. A threshold from one or two tasks means nothing, so
for the routing decision `syntax` and `review` are pooled into one
`textbook` family (they are the same RUBRIC tier, gate-binding). The
generated config gives both `syntax` and `review` the pooled decision.

## Rule (applied to the tune half, per family)

Let `L` = local pass rate, `C` = Claude pass rate on the family's tune
tasks (a model run that errored on a task counts as a fail).

1. `local_only` if `L >= 0.85` and Claude is not `>= 10` percentage
   points higher (`C - L < 0.10`).
2. else `local_first` (escalate to Claude on a failed judge) if
   `L >= 0.30`.
3. else `claude_direct` (`L < 0.30`).

Boundaries are inclusive as written: 0.30 is `local_first`, 0.85 with
`C - L` = 0.10 exactly is not `local_only`.

Attempts: `local_attempts` = 1, except `repo-task` = 2 (the second local
attempt is given the first attempt's check output).

Latency: a local attempt over 120 s counts as a failed attempt; the p95
of local attempts per family is reported.

## Hold-out acceptance (per family)

The routed policy is simulated on the hold-out half with the recorded
results: `local_only` -> the local result; `local_first` -> pass if local
passed, else Claude's result (cost = Claude's cost only when it
escalated); `claude_direct` -> Claude's result.

A family's thresholds are adopted only if, on hold-out:

- routed passes >= Claude-only passes - 5 percentage points (as a rate),
- routed cost <= 50 % of Claude-only cost, and
- routed passes >= local-only passes.

A family that fails is written as `claude_direct`, or, when there is no
Claude data at all, `local_first` with escalation off.

## Which local result

The routing decision uses the best local configuration that ran on the
whole set (by tune-half pass rate, ties to the faster one); the config
records which.
