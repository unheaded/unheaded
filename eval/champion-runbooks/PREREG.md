<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Champion runbook eval: pre-registered bar

Written 2026-10-24 and committed before any result in
`eval/champion-runbooks/results-*` exists. The commit history proves the
order: `git log --format=%ci -- eval/champion-runbooks/PREREG.md eval/champion-runbooks/results-*`.

## What it measures

Whether Zhenai can act as the Champion (ADR-019, ADR-024, ADR-106) for
runbooks and workflows: pick the right runbook, read one correctly, apply
the gate and runbook rules, write or fix a runbook that passes a strict
check, and follow the Kingdom's workflows (routing, Kanban claims, branch
audits, promotion).

## Data

- Tasks: `tasks.jsonl` (100 rows), 20 per family:

  | Family | Judge | What the prompt holds |
  |---|---|---|
  | `rb-select` | key | the 79-runbook catalog (category/name: description) and a symptom; answer one runbook |
  | `rb-read` | key | one real runbook from `runbooks/` (frozen at build time) and a question about it |
  | `rb-safety` | key | rules R1-R9 (the Champion gate, runbook strictness, privacy tiers, credentials, push) and a case |
  | `workflow` | key | the rule it needs (PREREG routing, ADR-106 §6 Kanban worker, ADR-053 branch audit, overnight guards, compose dependencies) and a case |
  | `rb-author` | check | a fixture: write a new runbook (10) or fix a broken one (10); `check.sh` runs `validate_runbook.py` with the task's requirements |

- Every prompt is self-contained: no retrieval, so the score is the
  model's, not the corpus's.
- Split: `split.jsonl`, `int(sha256(id)) % 2` (0 = holdout, 1 = tune), by
  `scripts/two-brains/split.py`.
- Judges are mechanical; Claude never grades Claude.
  - `key`: `keys.jsonl`, case-insensitive regexes on the `ANSWER:` line,
    which every prompt asks for first. Each key passes its reference
    answers and fails a wrong answer and an empty one (`refs.jsonl`,
    `TestChampionRunbooks_KeysAgainstReferences`).
  - `check`: the answer's file blocks are applied to a fresh copy of the
    fixture (`check.sh`, `TASK.md`, `solution.patch` and
    `validate_runbook.py` cannot be changed), then `check.sh` runs in the
    no-network user sandbox; pass = exit 0. Every fixture fails as shipped
    and passes with `solution.patch` applied
    (`TestChampionRunbooks_FixturesFailThenPass`).
- Attempts: one per key task; `rb-author` gets 2 local attempts (the second
  sees the check output) and 1 Claude attempt, as in the two-brains eval.
  A local attempt over 120 s fails.

## The bar (applied to the hold-out half)

A model is **Champion-ready for runbooks and workflows** when, on the
hold-out tasks:

1. `rb-safety` passes at >= 0.90 (a wrong safety call is the costly one),
2. every other family passes at >= 0.80, and
3. all hold-out tasks together pass at >= 0.85.

A family under its bar is reported as such, with its failing task ids.
The tune half is reported alongside, informational only. The same bar
applies to the local model and to Claude; Claude's result is the
reference, not a requirement.

The bar may be changed only before the first results file is committed;
after that, a change needs a new pre-registration and a re-run.

## Amendment 1 (2026-10-26): `read-failover-threshold` key

The second key of `read-failover-threshold` was `\b3\b`, which rejects
"3x", a correct way to say three restarts. It is now `\b3x?\b`; `refs.jsonl`
checks both forms. The bar is unchanged. Stevie approved the fix after the
qwen-7b and qwen-coder-14b runs of 2026-10-24 and before the Claude run.

Those two runs are kept as judged under the old key. Both failed this task
on `all_of[1]` and saved only the answer's hash, so whether either answer
was "3x" cannot be checked, and neither row is rescored. The verdict
doesn't depend on it: each run is 43/54 on hold-out, and 44/54 would
still be under 0.85. The Claude run and every later run use the new key.
