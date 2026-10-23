<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Live card fixtures (S-TWO-BRAINS Phase 6)

Small tasks a Kanban card can hand to `zhen-agentd -worker` end to end
(Steps 216-221, `wiki/Zhen-Agents.md`). Each directory carries its own
`check.sh`, so a card names `check: fixture-check` and the directory as
`path`; the check, `TASK.md` and the tests are protected (a draft that
changes them is refused).

| Directory | Card family | Route (configs/zhen-routing.yaml) |
|---|---|---|
| `go-nil-map/` | `repo-task` (copy of the hold-out fixture, without its solution) | `local_only` |
| `docs-usage/` | `docs` (not in the table) | `default`: `claude_direct` |

Both start broken: their checks fail until the card is worked.
