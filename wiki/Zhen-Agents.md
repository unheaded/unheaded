# Zhen Agents

`zhen-agentd -worker` turns a Kanban card into a local `zhen/<card>` branch for Stevie to review. The card lives in the Well (`unheaded_app.kanban_tasks`); the worker reads it there, never from a file or the binary. Nothing is pushed, ever.

## Handing Zhen a card

A card is worked only when Stevie hands it over:

- `owner` is `zhen` (the agent never touches anyone else's cards);
- `status` is `todo`, `type` one of `task`, `tech-debt`, `bug`, `docs`; not deleted or archived;
- the description holds one fenced `zhen` block:

````
The constructor leaves the map nil; fix NewRegistry.

```zhen
family: repo-task
check: fixture-check
path: eval/two-brains/live/go-nil-map
max_claude_usd: 0.25
```
````

| Key | Meaning |
|---|---|
| `family` | routing family in `configs/zhen-routing.yaml` (an unknown one uses `default`) |
| `check` | a key of `configs/zhen-checks.yaml` (`go-test`, `py-unittest`, `shellcheck`, `fixture-check`, ...); the only thing the worker runs, in the `run_check` sandbox |
| `path` | the check's path, relative to the repository; a Go `/...` pattern is fine; never absolute, `..`, `.git` or an option |
| `max_claude_usd` | optional; lowers the family's per-task Claude budget, never raises it (over the family cap: the card is skipped) |

The block is parsed strictly: an unknown key, a second block, a check that is not allowlisted, or a description over 16 KiB, and the card is skipped (logged with the reason), never guessed at. The rest of the description goes to the model as untrusted data between `<<<CARD` and `CARD>>>` markers; the card title becomes the commit subject on one line.

A budget below about $0.15 leaves room for only one Claude request at the worst-case reservation (see Spend below); `$0.25` fits a short tool loop.

## What the worker does

1. Claims the card in `unheaded_app.zhen_agent_runs` (migration 032): one active run per card, a 30-minute lease renewed every 5 minutes; the card moves to `in-progress`.
2. Adds a worktree from local `develop` at `~/.cache/unheaded/zhen-worktrees/<card>` on branch `zhen/<card>` (the agent root must be outside the checkout; the worker refuses to start otherwise).
3. Runs the routed loop in process (`pkg/zhenroute`, [[Zhen Routing|Zhen-Routing]]): the local model drafts, the check judges in the sandbox, a failing check escalates to Claude within the caps. Writes inside the agent worktree need no per-call approval (branch-sandbox mode, ADR-019 amendment, Proposed): the review gate is the branch.
4. A passing check: commits as `Zhenai <zhenai@unheaded.invalid>` (committer: the repo's identity; unsigned; no git hooks), with trailers `Zhen-Card`, `Zhen-Route`, `Zhen-Run`, and `Co-Authored-By: Claude ...` when the Claude leg ran. New files in a hidden top-level directory (`.config/`, `.cache/`) are tool leftovers and are never committed. The card moves to `review` (progress 90) with the branch in its `commits` list.
5. A failing check, no change, or an infrastructure error: the run is `failed`, the card goes back to `todo`, the worktree is kept. A card gets 2 attempts (the second on `zhen/<card>-2`), then the worker leaves it alone.

One card at a time. Every claim, card move and run outcome is an `audit_events` row (`actor_type` `service`) and a `zhen.agent.runs` message on Wotan; every model request is an `llm_requests` row ([[Zhen Audit|Zhen-Audit]]).

## Reviewing

Each run leaves a note at `~/.local/state/unheaded/zhen-reviews/<branch name>.md` (outside the repo, 0600): route, attempts, escalation, check output tail, Claude cost, diff stat, and the commands below.

```bash
zhen-claude review list
zhen-claude review show <run>
git log --stat develop..zhen/<card>
git switch develop && git merge --no-ff zhen/<card>      # your merge
zhen-claude review accept <run> [-reason "..."]          # refused until the head is in develop
zhen-claude review reject <run> -reason "..."            # card back to todo
```

`review` needs `WELL_APP_ZHEN_DSN` (the runs and cards) and `WELL_OPS_DSN` (one `audit_events` row per decision, `actor_type` `human`). Decisions are one-way. Accept and reject remove the worktree and keep the branch: rejected diffs are training data for the flywheel. Worktrees of abandoned, failed and cancelled runs are removed by the worker after 7 days; their branches stay.

## Running it

```bash
export WELL_APP_ZHEN_DSN=...   # unheaded_app as app_zhen (your shell, never the repo)
export WELL_OPS_DSN=...        # unheaded_ops as ops_writer, for -audit-well
zhen-agentd -worker -once -project-root ~/tmp/unheaded -claude fake -audit-well   # one card, then exit
zhen-agentd -worker -claude live -effort low -daily-cap-usd 2 -audit-well          # poll every 60 s beside the HTTP API (needs AUTH_ENABLED=true + AUTH_API_KEYS, or -no-auth on loopback)
```

`-claude fake` never calls the API: a route that needs Claude ends with the check's verdict on the unchanged worktree. `-claude live` uses `ANTHROPIC_API_KEY` from the environment and the zhen-claude spend ledger (`-ledger`, `-daily-cap-usd`).

## Spend

The bridge reserves every request's worst case before sending it (input estimated from the body, plus `max_tokens` priced at the routed model when fallbacks are off). Each request's `max_tokens` is fitted to what the family cap has left (`Ledger.FitMaxTokens`, at least 1024 for routed legs), so a tool loop's later requests still fit; before S-CHAMPION-HARDENING Phase 5 the first request took about 70 % of the cap and every leg stopped after one request (F-23). When the cap stops a Claude leg that already changed files, the check still judges them.

## Stopping it

Stop the process (SIGTERM: the card in flight is cancelled, nothing partial is committed, the card goes back to `todo`). In overnight mode, `touch ~/.local/state/unheaded/zhen-agentd.stop` does the same at the next check (remove the file before the next night). The full procedure is the runbook `runbooks/observe/zhen-overnight-abort.yaml`; `scripts/zhen-morning-report.sh` prints what the night did.

## Confirmations and the scheduler (S-CHAMPION-HARDENING)

A confirmation token is bound to the principal that asked for the write: only the same principal (or `queue:stevie` through `zhen-claude confirm approve`) can redeem it, and a `-no-auth` agentd cannot redeem any (403). With the Zhen UI's single API key, all UI users share one principal; per-owner `uhk_` keys separate them.

`zhen-agentd -scheduler` runs approved runbooks (status approved, or no status) on the cadences in `/etc/unheaded/zhen/schedules.yaml` (`configs/zhen-schedules.yaml`): each run goes through the Champion gate, makes one `scheduled-run` Kanban card and one `zhen_agent_runs` row (mode `scheduled`), and a failed run (including a failed `verification:` section) sends its card to review. Jobs change through `/api/v1/scheduler/{add,remove,pause,list}` (mutating tool `schedule_update`); the heartbeat gauge feeds `ZhenSchedulerStalled`. The Python scheduler is retired (ADR-028 superseded).

## Overnight mode

`zhen-agentd -worker -overnight` works cards inside a window, writes a morning report and exits. The user units `deploy/systemd/user/zhen-agentd-overnight.{service,timer}` start it at 01:00 for the window 01:00-07:00. They are **not enabled**: whether and when is Stevie's decision.

```bash
# enable (the env file is yours, 0600, outside the repo: ANTHROPIC_API_KEY, WELL_APP_ZHEN_DSN, WELL_OPS_DSN, LLAMA_URL)
make build-zhen-agentd
systemctl --user link ~/tmp/unheaded/deploy/systemd/user/zhen-agentd-overnight.{service,timer}
systemctl --user daemon-reload && systemctl --user enable --now zhen-agentd-overnight.timer
systemctl --user list-timers | grep zhen
# disable
systemctl --user disable --now zhen-agentd-overnight.timer
# stop tonight's run now
touch ~/.local/state/unheaded/zhen-agentd.stop
```

Before every card the night checks, in order, and stops with a typed reason written into the report:

| Check | Default | Stop reason |
|---|---|---|
| stop file | `~/.local/state/unheaded/zhen-agentd.stop` | `stop_file` |
| window | `-window 01:00-07:00` (local time; may cross midnight) | `window_end` (`outside_window` if started outside it) |
| cards per night | `-max-cards 5` | `max_cards` |
| Claude spend | its own ledger `~/.local/state/unheaded/zhen-claude/spend-overnight.json`, `-daily-cap-usd 3` (never the interactive ledger) | `spend_cap` |
| infrastructure failures | 2 in a row (a failing check is the model's failure and does not count) | `infra_failures` |
| SIGTERM / SIGINT | | `signal` |

Two conditions defer a card to the next poll instead of stopping the night:

- **memory floor**: `MemAvailable` (from `/proc/meminfo`) must be at least 2 GiB plus one check's `MemoryMax` (2G), 4 GiB in all. Swap is never counted: fitting in swap is how this box livelocks. `ZHEN_MEM_FLOOR_BYTES` can raise the floor, never lower it.
- **GPU lease** (below) held exclusively.

Privacy is forced to `strict` (no Kingdom-ops tool result leaves the host while nobody watches).

The morning report is `~/.local/state/unheaded/zhen-reports/<date>.md` (0600): the stop reason, cards tried, succeeded, failed, cancelled and deferred (by reason), Claude spend against the cap, escalations, every run's branch and note, and the review commands. A counts-only summary goes to Wotan `zhen.agent.report`.

## One GPU: forge training and the worker

The RX 7700 XT (12 GB) cannot hold a served model and a Gemma-4 training run at once. One lease decides who has it: an `flock` on `$GPU_LOCK` (default `$XDG_RUNTIME_DIR/unheaded/gpu.lock`), `scripts/lib/gpu-lease.sh` in shell and `pkg/gpulease` in Go.

| Who | Lease |
|---|---|
| each llama-server request (`pkg/localllm`: worker, `/ask`, summariser) | shared, for the call (waits up to 10-30 s) |
| `scripts/switch-model.sh` | exclusive, for the whole swap |
| `scripts/forge-train.sh` | exclusive, for the whole run: stops llama-server first and loads `FORGE_RESTART_MODEL` (default `qwen-7b`) after, through `switch-model.sh` and its fit check |

The worker never starts, stops or swaps a model. Finding the lease held, it defers the card (no claim); a lease taken mid-card cancels that run (no attempt used, nothing committed) and the card waits for the next poll. It never escalates to Claude because the GPU is busy. A stuck lease is a stuck process: `fuser -v "$GPU_LOCK"` names it; stopping it frees the lease. Never delete the lock file.

## /api/v1/agent/ask and approval tokens

`/ask` runs a model loop and answers whoever called it over HTTP, which may itself be a model. Its refused-pending writes are therefore treated like any agent loop's: queued in the Well with `-confirm-queue` (the trace carries only the queue id; Stevie decides with `zhen-claude confirm`), otherwise denied. No token is returned. `-ask-tokens` restores the old behaviour for a listener no model can reach.
