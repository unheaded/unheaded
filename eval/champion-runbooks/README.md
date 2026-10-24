<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Champion runbook eval

100 tasks that test Zhenai as the Champion for runbooks and workflows:
`rb-select`, `rb-read`, `rb-safety`, `workflow` (key-judged, 20 each) and
`rb-author` (20 fixtures judged by their own check). The bar and the rules
are in [PREREG.md](PREREG.md).

## Run

`cmd/zhen-eval` runs it; point it at this directory:

```
E=eval/champion-runbooks
go run ./cmd/zhen-eval -backend local -model qwen-coder-14b \
  -tasks $E/split.jsonl -keys $E/keys.jsonl -prompts $E/prompts.jsonl \
  -out $E/results-<date>-qwen14b.jsonl
go run ./cmd/zhen-eval -backend claude -effort low -daily-cap-usd 3 \
  -tasks $E/split.jsonl -keys $E/keys.jsonl -prompts $E/prompts.jsonl \
  -out $E/results-<date>-opus55-low.jsonl
```

`-family rb-safety`, `-split holdout` and `-limit N` narrow a run. The
local leg needs llama-server (`scripts/switch-model.sh`); the Claude leg
needs its key in the environment, never in the repo.

## Files

- `build.py` writes everything below from one place. `rb-read` prompts embed
  runbooks as they were when it ran; the committed `prompts.jsonl` is the
  frozen copy. Re-run only on purpose, before results exist for the tasks
  it changes, then re-split:
  `python3 scripts/two-brains/split.py eval/champion-runbooks/tasks.jsonl > eval/champion-runbooks/split.jsonl`.
- `prompts.jsonl`, `keys.jsonl`, `refs.jsonl` (reference and wrong answers
  per key), `tasks.jsonl`, `split.jsonl`.
- `fixtures/<id>/`: `TASK.md`, `check.sh`, `validate_runbook.py` (the strict
  runbook/v1 rules of ADR-024 §6 plus task requirements), `runbook.yaml`
  for the fix tasks, `solution.patch`.

`validate_runbook.py` is the eval's contract, not `scripts/check-runbooks.py`:
it skips the repository-only rules (file stem, `dry_run_test` path) and adds
per-task requirements.

## Tests

`go test ./cmd/zhen-eval -run ChampionRunbooks`: 100 tasks load, 20 per
family; every key passes its references and fails its wrong answers; every
fixture fails as shipped and passes with its solution.
