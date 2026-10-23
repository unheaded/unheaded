<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-TWO-BRAINS progress

Lab notebook for `docs/battle-plans/S-TWO-BRAINS-battle-plan.md`. This run:
Phases 0-5 (Steps 1-196), executed by a worktree agent on branch
`sprint/two-brains`, based on `f7d730e7` (develop as of 2026-10-23).
Unsigned commits (`--no-gpg-sign`; `scripts/resign-unpushed.py` later).
Nothing pushed; the live Well, its data and backups were not touched.

Deviations from the plan's VARIABLES block: `$SCRATCH` is the session
scratchpad (not `$XDG_RUNTIME_DIR/s2b`); `$LEDGER` is a run-private spend
ledger in `$SCRATCH` so this run's spend is measured exactly; the eval cap
is $3 (run budget $5 total), not $4.

## Hypotheses

| # | Result | Evidence |
|---|---|---|
| Step 2 | equivalent | `443de3f82` is not an ancestor of `f7d730e7` (develop was rewritten by a re-sign), but its content is: `31c9dcfa` .mcp.json, `226e905a` zhencapture, `6ba43df2` local_llm, `2716996a` -route auto + -capture, `66fcb552` -local-only. |
| H1 | true | `.mcp.json` registers `zhen` -> `scripts/zhen-mcp.sh`, env `{}`; no `ZHEN_MCP_ALLOW_*`. |
| H2 | true | `local_llm` at `raft/zhen_mcp_server.py:273`; 9 llm tests pass (`-k "local_llm or llm"`; the plan's `-k local_llm` selects 0 by test name). |
| H3 | true | `zhen-claude -route claude|auto`, `-check`, `-local-only` (`cmd/zhen-claude/route.go`). |
| H4 | true | `-capture <dir>` logs `schema=zhen-capture/v1`; dir created 0700; tag in `pkg/zhencapture/capture.go`, `raft/zhen_mcp_server.py`. |
| H5 | true | `pkg/champion/confirm.go:35` in-memory map; `cmd/zhen-agentd/toolexec.go:188` returns `PendingToken` to the HTTP caller. |
| H6 | true | no runbook schema test; `scripts/run-runbook.py` reads `verification:` only to scan for unset `${VAR}`s, never runs it. |
| H7 | true | `crates/zhenai-forge/src/main.rs` ~171 `filter_map(... .ok())`, per-example `answer_start` falls back to the CLI value. |
| H8 | true | no Postgres/Well container, nothing on :5432, `postgresql` inactive; Wotan :18000 down. Monitoring stack (Prometheus, Grafana, Loki, VM, Alertmanager) is up. |
| H9 | true | `ANTHROPIC_API_KEY` present in `~/.bashrc` (loaded per command, never printed). Dry run names `none (dry run)`; the live smoke (Step 27) proves it. |
| H10 | false | nothing on :8081, no llama-server process. Started only for the Phase 4 eval via `scripts/switch-model.sh`. |
| H11 | true | GPU0 0.19 / 11.98 GiB used; MemAvailable 8.5 GiB of 14.3 GiB. |
| H12 | true | no admin key handling in `services/wotan` Go code; docs name both `ADMIN_API_KEY` (ADR-008) and `WOTAN_ADMIN_TOKEN` (KINGDOM_ARCHITECTURE). |
| H13 | true | no claim/lease/assignee column in `kanban_tasks`; `pkg/champion/dispatch.go` has no command-runner case. |
| H14 | true | `systemd-run --user` with `PrivateNetwork=yes ProtectHome=tmpfs BindPaths=` printed `hi`, `0` (home hidden), `net-blocked` (Prometheus :9090 reachable outside, not inside); `PrivateTmp=yes` also works. |
| H15 | partly false | a reusable OpenAI-compatible client already exists: `pkg/claudebridge/local.go` (`LocalLLM`, from the H3 prerequisite). `cmd/zhen-agent` and `zhen-agentd` still carry their own. |
| H16 | true | `git-filter-repo` missing; `gitleaks` 8.28.0 at `~/.local/bin/gitleaks`. |

Tools: go 1.27.3, cargo, python3, docker (no sudo needed), systemd-run,
rocm-smi, shellcheck, jq, curl, golangci-lint (`~/.cache/unheaded-tools/bin`),
gosec (`~/go/bin`); `psql` is not installed on the host (throwaway tests use
`docker exec ... psql`). `mcp` imports from `~/.venv/zhen`.

## Baseline

Measured 2026-10-23 on `f7d730e7`, matching the KNOWN FAILURES BASELINE table
(no unexplained failure):

- `go build ./...` ok; `go test ./...`: 277 ok / 0 FAIL (S17 recorded 267; packages added since).
- `pytest raft/tests`: 73 passed, 2 skipped (plan table: 56; tests added since).
- `cargo test -p zhenai-forge --lib`: 104 passed, 0 failed, 28 ignored.
- `check-well-migrations.sh`, `check-k8s-well-initdb.sh`, `well-migrate check`: clean (32 manifest entries over 4 databases).
- Step 27 live smoke: `OK`, `credentials=env ANTHROPIC_API_KEY`, 231 in / 4 out tokens, $0.001004, audit line `sent=true`, prompt text absent from the log (0 hits).

## Decisions

- **Step 31** [DECIDE] recommendation taken: tool-gate decisions -> `audit_events` via `pkg/audit/storage.DatabaseStorage` (it writes the 022 shape as is); per-request telemetry -> new `llm_requests` (030); live view -> Wotan `zhen.llm.requests` / `zhen.audit.toolcalls`.
- **Step 32** Erda deviation from the plan's SQL: `CREATE TABLE` / `CREATE INDEX` without `IF NOT EXISTS` (Law 1: in a ledger migration `IF NOT EXISTS` turns drift into silent success); length CHECKs added on the free-text columns (`error` <= 512, model/conversation <= 128, family <= 64). Lint: one P3 corpus note (`no-default-privileges`), same as every existing migration.
- **Step 36** the test logs in as `ops_writer`/`ops_reader` over TCP (passwords set only inside the throwaway container); the append-only checks run as the owner, because `ops_writer` has no UPDATE grant and so cannot show the trigger. Container names are `qa-` prefixed. `--sql-only` skips the Go part.
- **Step 43** `MultiAuditor` delivers to every sink concurrently (a slow sink delays the call's return, not the other sinks); a panicking sink is contained. Well and Wotan sinks are async (queue 1024, one worker each: the single worker also serialises the `audit_events` hash chain; timestamps strictly increase at microsecond precision). Metrics: `zhen_audit_dropped_total{sink}` and `zhen_audit_errors_total{sink}` (in-tree `pkg/metrics`, no client_golang).
- **Step 46-47** `configs/wotan.yaml` is the only `auto_approve` copy in the tree (helm/k8s/nix have none). `WOTAN_ADDR` is Wotan's HTTP `host:port` (repo convention; the plan's `127.0.0.1:18001` is gRPC and `pkg/wotan-client` publishes over HTTP).
- **Step 52** the fake-API turn is 2 requests (tool_use, then end_turn) and 2 tool calls, so the check is 2 `llm_requests` rows, 2 `audit_events` (1 success, 1 pending), chain valid, 0 marker hits in `pg_dump`.
- **Step 56** agentd: one `llm_requests` row per llama-server call through `llamaLLM` (usage parsed from the OpenAI block; every call of one agent run shares a run id as `conversation_id`); one `audit_events` row per gate decision through an `ActionStore` decorator over `tool_call_attempt` actions (the argument summary is hashed, not stored). `-audit-well` with the Well down warns and continues (zhen_actions still records); `zhen-claude` goes read-only instead (Step 48).
- **Step 63** [DECIDE] recommendation taken (DenyConfirmer default, TTYConfirmer, QueueConfirmer; only the Champion's own write tools confirmable). Override (read-only bridge) not taken: no word from Stevie asking for it.
- **Step 64** Erda additions to the plan's SQL: plain `CREATE TABLE`; every column but `status`/`decided_*` immutable (owner included); approval after `expires_at` refused in the trigger; insert must be `pending`; `(status = 'pending') = (decided_at IS NULL)`; 20-pending cap per `source_service` in an insert trigger (defense in depth with the code cap); DELETE/TRUNCATE refused by `well_append_only()`. Lint P3 `text-surrogate-pk` accepted: the 16-hex id is the handle Stevie types (`confirm approve <id>`), low volume.
- **Step 71-73** the hash covers `{tool, args, project_root}` (canonical JSON, encoding/json sorts keys at every depth), so (d) "approved for root A, executed under B" fails the hash, not only the allowlist. `confirmedGate` is the one stripped gate for `ConfirmPendingToolCall` and `ConfirmCall`.
- **Step 76-78** refused-pending calls are confirmed after the turn's parallel tool runs, one at a time in `tool_use` order (test 6). The TTY reads `/dev/tty` with plain line reads (`golang.org/x/term` is not in go.mod; no new dependency). Approval codes: 4 chars from a 31-char alphabet without look-alikes, printed on the terminal only.
- **Step 81** `confirm approve` takes `-allowed-roots` (default: cwd). Kanban confirmations run with no KanbanStore in the CLI, so a queued `kanban_*` call ends `failed` until the subcommand gets a Kanban store (Phase 6 territory; noted, not built).
- **Step 83** "direct" = any `/tool/exec` caller whose `emitted_by` does not start with `zhen-agent`; the web UI path (tokens) is unchanged.

## Reviews

### Step 86 — BlackMage review of the approval path (2026-10-23)

| # | Attack | Result |
|---|---|---|
| a | TOCTOU between `show` and `approve` | Closed. The hash binds tool, args and root; `approve` re-reads the row, re-hashes, and prints the diff it is about to run (recomputed, not trusted from `show`). |
| b | Path swap via symlink after approval | **Was open, fixed** (76e7aa89). `validatePath` only used `filepath.Abs`, so a symlink inside the sandbox let a read or write land outside it, approved or not. It now also resolves the deepest existing ancestor (`EvalSymlinks`) and checks the real path against the real allowed roots with a separator boundary. Residual: a swap between the check and `os.WriteFile` is still a race (needs `openat2(RESOLVE_BENEATH)`); recorded, not fixed here. |
| c | Queue flooding | Closed. 20 pending per `source_service`, refused in code (`QueueConfirmer`, test `TestConfirm_QueueCap`) and in 031's insert trigger. |
| d | Approval for root A executed under root B | Closed twice: `-allowed-roots` and the hash (root is hashed). Tests `TestConfirmCall_RefusesOtherRoot`, `TestConfirmCmd_RootSwapRefused`. |
| e | Replay of an old approval | Closed. One-way states in 031's trigger and `Decide`/`Finish` (exactly one row moved or `ErrNotPending`); second `approve` refused (test). |
| f | Agent loop claiming the user's trust on `/tool/exec` | **Was open, fixed** (360aed91). The endpoint defaulted every caller without a justification to `direct-user`, so an agent loop's writes passed Rule 2 outright; an agent loop could also assert `direct-user`. Agent-loop callers now get neither. |
| g | Model self-approval through content | Closed. The TTY code is printed on `/dev/tty` only and read from it only; test with the model writing "approve", codes and "yes" in text and tool input; no request body ever carries the code (test). |
| h | Token to the model through `/api/v1/agent/ask` | Open, out of this step's scope: `/ask`'s trace still returns a `pending_token` to its HTTP caller. That caller is the web UI today; a model loop calling `/ask` over HTTP would receive one. Candidate for Phase 6 (worker calls agent code in-process, not over HTTP). |
| i | Terminal escape sequences in a prompt | Closed. Every model-supplied string shown on the TTY goes through `safeLine` (control characters replaced) and the redactor. |

Step 89: no human at the terminal during this run, so the interactive live `-confirm tty` run was skipped (not on the critical path). The prompt is exercised by the fake-TTY tests (`TestConfirm_TTYApprove` prints the rendered prompt into the fake terminal: tool, sandbox, hash, redacted args, diff, code line).
### Step 117 — MoatGhost / BlackMage note on the privacy tier

What still leaves west for the Claude API, after Phase 3:

- **T0 repository text** after the secret scan. ADRs and docs name hosts (west, east), the bridge subnet and the overlay prefix; it is the public repo's own text, accepted.
- **Redacted T1 text**: tool output with secrets, addresses, MACs, emails, home user names and host names replaced by stable placeholders; or, for payloads over 8 KiB, a local-model summary of the redacted text, redacted again. Structure (service names, ports, states, error messages, counts) still leaves; that is the point of sending it.
- **Counts** (tokens, cost, redaction counts) in the request itself only as usage; never content.
- **Stevie's own prompt**, after T1 redaction, with a warning naming the counts. His choice; warned and redacted, not blocked.

Residual risks: (1) the patterns are a deny list — an identifier with no pattern (a project codename, a customer name in a log line) passes in standard mode; `-privacy strict` withholds all T1 for unattended runs. (2) Host names are a configured list (west, east); a new host needs adding (`privacy.RedactorConfig.Hosts`, not yet a flag). (3) Whole-word host redaction also hits the English words "west"/"east" in T1 text; harmless over-redaction. (4) T0 is decided by `git ls-files` at the realpath, so a tracked file is T0 even if its content is sensitive and committed by mistake (that is a repo-hygiene finding, caught by gitleaks in CI). (5) Claude Code sessions use the MCP server directly and are outside this tier (Step 118; listed in `wiki/Zhen-Audit.md`).

## STUCK

## ESCALATE answers

- **Step 59 [ESCALATE] — OPEN.** Apply migration 030 (`unheaded_ops.llm_requests`) to the live Well? Additive, new append-only table, tested on throwaway servers (`scripts/well/test-llm-requests.sh`, bite-checked). The Well is down (H8) and live apply is Stevie's call (ADR-101 `well-migrate apply -yes`). Not applied; Step 61 (live check) skipped. Batch with 031 (Step 94).
- **Step 94 [ESCALATE] — OPEN.** Apply migration 031 (`unheaded_app.zhen_pending_confirmations`) to the live Well, batched with 030 (Step 59). Additive new table, tested and bite-checked on throwaway servers (`scripts/well/test-zhen-confirm.sh`). Until it is applied, `-confirm queue` and `zhen-agentd -confirm-queue` cannot start against the live Well (they need the table); `-confirm tty` and `deny` work without it.

## Spend log

All live Claude calls of this run go through `-ledger $SCRATCH/spend.json`
(run-private), `-effort low`, small `-max-tokens`.

| Step | What | Cost (USD) | Running total |
|---|---|---|---|
| 27 | smoke, `-effort low`, 1 request | 0.001004 | 0.001004 |
