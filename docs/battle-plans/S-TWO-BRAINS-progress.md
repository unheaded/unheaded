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

## STUCK

## ESCALATE answers

- **Step 59 [ESCALATE] — OPEN.** Apply migration 030 (`unheaded_ops.llm_requests`) to the live Well? Additive, new append-only table, tested on throwaway servers (`scripts/well/test-llm-requests.sh`, bite-checked). The Well is down (H8) and live apply is Stevie's call (ADR-101 `well-migrate apply -yes`). Not applied; Step 61 (live check) skipped. Batch with 031 (Step 94).

## Spend log

All live Claude calls of this run go through `-ledger $SCRATCH/spend.json`
(run-private), `-effort low`, small `-max-tokens`.

| Step | What | Cost (USD) | Running total |
|---|---|---|---|
| 27 | smoke, `-effort low`, 1 request | 0.001004 | 0.001004 |
