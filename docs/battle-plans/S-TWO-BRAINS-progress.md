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
- **Step 99** [DECIDE] recommendation taken (T0/T1/T2; unknown tool = T1). gitleaks 8.28 has `stdin`, so the override is not needed. Planted fixtures are generated at test time rather than committed under `testdata/` (a committed synthetic key or PEM header would trip the repo's own gitleaks gate and its shrink-only baseline).
- **Step 100** the plan's probe key (`AKIA` + 16 hex) never matches gitleaks' AWS rule (`[A-Z2-7]`); a base32 synthetic key does (1 finding, exit 1). `--report-path /dev/stdout` into a regular file truncates it; the redactor reads the report through a pipe.
- **Step 109** the bridge redacts the user prompt as T1 always (it only changes when a T1 pattern is present) and refuses to send when the redactor cannot run; tool results are filtered after the gate, for allowed results and tool-error texts alike. `-privacy`, `-user-data-roots`, `-secret-scanner` flags; command tests use a stub scanner, the privacy package tests the real one.
- **Step 125** [DECIDE] recommendation taken. Families: `syntax`, `review` (textbook), `hard`, `expert`, `prod`, `repo-task` (20 fixtures: 8 Go, 5 Python, 4 bash, 3 runbook YAML), `runbook-draft` (5, blocked on Phase 8). Keys written for all 56 prompts before any run.
- **Step 128** bash fixtures check `shellcheck -S info`: SC2086 and SC2162 are info-level, so a warning-level check could not fail on them (2 BAD -> 20 ok after the change).
- **Step 131** the hash split leaves `review` 1 tune task and `syntax` 2; PREREG pools them as `textbook` for the decision (written before results).
- **Step 134** [DECIDE] H15 partly false (a client existed in `pkg/claudebridge/local.go`); lifted into `pkg/localllm` anyway as planned, and `claudebridge.LocalLLM`, `zhen-agent`, `zhen-agentd` moved onto it. One behaviour change recorded in the commit: zhen-agent now sends `temperature: 0` when asked for 0 (it was omitted, so the server default applied).
- **Step 137** the eval sandbox is a shared `pkg/sandbox` (Phase 5's `run_check` uses the same one) rather than eval-only code. Answers are written outside the repo (`-answers-dir`); results carry only an answer's SHA-256 and size.
- **Step 142 [V] FAILED, then [D] 142a**: first qwen-7b run, textbook 9/14 against H0 12/14. Byte-level look at the answers: `syntax-go` was answered in C++ because the prompt text names no language and this runner, unlike H0's zhen-rag, retrieves nothing; fix: both brains get `Language: <lang>` first (runner change, committed before the rerun; first run kept outside the repo). Rerun: syntax 7/7, review 2/7 = 9/14 still. The remaining gap is the pre-registered keys being stricter than the human rubric on review answers: `review-go` says the errors "should be checked/handled" (key wants ignored/dropped/unchecked), `review-python` says "catches all exceptions" (key wants "bare except"), `review-bash` fixes the quoting in code without saying so; `review-html` and `review-javascript` are genuine misses. A human grade of the same answers is about 11-12/14, within the ±1. Keys left as pre-registered; both brains are judged by the same keys, so the comparison stays fair. Candidate for a key revision with Stevie's 10 % spot check.
- **Step 143** fixed a runner bug while the Claude run was in flight: `latency_ms` and `cost_usd` were lost (deferred assignment on a non-named return). The qwen-7b and Opus result files carry `latency_ms: 0` and `cost_usd: ""`; `attempt_ms` and `cost_nano_usd` (what the thresholds use) are correct. The qwen-coder-14b run has all fields.
- **Step 161** [DECIDE] recommendation taken (`run_check` with `configs/zhen-checks.yaml`). Additions: `fixture-check` (a fixture's own `check.sh`; the router refuses the result if `check.sh`/`TASK.md`/`solution.patch` changed), `py-unittest` instead of `pytest` (not installed in the system Python the sandbox sees), `shellcheck -S info`, runtime capped at 600 s for every key.
- **Step 171** [DECIDE] H3 holds: `-route auto` (and new `-route local`) with `-family` switches to the threshold router; without `-family` the old `-route auto` behaviour is unchanged. The router type is `ThresholdRouter` (`claudebridge.Router` is ADR-104's interface name).
- **Step 175** [DECIDE] recommendation taken (branch-sandbox mode), with two hardenings found while writing it: no write under `.git` (a hook would run on Stevie's review) and the branch re-checked on every call. Written up in ADR-019 as a Proposed amendment.
- **Step 183** comparison row: `--route local` on the same three hold-out fixtures gives the same outcome as `--route auto` (pass, pass, pass; 1 local attempt each; 12.8 s, 7.0 s, 14.3 s; $0), because the generated config already routes repo-task `local_only`. llama-server stopped afterwards (VRAM back to 0.18 GiB, the Step 18 level).

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
### Step 169 — BlackMage on run_check

- **Finding (host, recorded, not changed):** `/etc/unheaded/anthropic.env` (0644), `kev-stack.yaml` (0644) and `wotan.yaml` (0664, owned by govan) are world-readable on west. A model-written test run by `run_check` could read the Anthropic credential file and print it into the check output, which goes back to the model (the privacy tier's secret scan would likely catch a key, but that is the last line, not the first). Not remediated on the host (no host config changes in this run; for Stevie: `chmod 0600` or a root-owned 0640 group file, then rotate if it ever left). **Sandbox narrowed instead** (this is narrowing, not widening): every unit now gets `InaccessiblePaths=-/etc/unheaded -/var/lib/unheaded -/srv/backups` (`sandbox.DefaultHidden`); `TestSystemd_KingdomConfigHidden` proves `/etc/unheaded` is unreadable inside.
- Fork bomb: `TasksMax=256` (300 background sleeps did not all start; `TestRunCheckSystemd_TasksCapped`).
- Writes via `/tmp`: `PrivateTmp=yes` on every unit (probed in H14; Step 170 needed no conditional: the property works here, and `sandbox.Available()` is the startup probe).
- The real home is hidden (`ProtectHome=tmpfs`); the Go toolchain and module cache under it are bound read-only; the build cache is a dedicated directory, never `~/.cache/go-build` (no cache poisoning of the operator's builds).
- Environment: only what the check config passes (`--setenv`); the caller's environment (keys, DSNs) does not reach the unit (`TestSystemd_EnvIsOnlyWhatIsPassed`).
- Step 168 [ESCALATE] not triggered: H14 held (user-scope sandbox works).
### Step 184 — escalation prompt review

The escalation prompt is built by `ThresholdRouter.Run` from three parts: the task text (T0: a card or `TASK.md`, plus the worktree's tracked files, which are the fixture's own repository text), the last local draft (local model output about those files), and the check output, which is cut to 4 KiB and passed through the privacy tier's T1 redaction before it is put into any prompt (`TestRouter_FailingCheckEscalatesOnceWithContext`: a planted `10.10.10.10` in the check output reaches neither the second local attempt nor Claude unredacted). The bridge then redacts the whole prompt again (`Conversation.Send`), and every tool result of the Claude leg goes through the tier as well. Claude's tools in the leg are the Champion's (`read_file`, `write_file`, `patch_file`, `run_check`) confined to the worktree; `-confirm` is deny, so Kanban and runbook tools stay refused. Step 194 bite: a router that ignores `check_fail` in `escalate_on` fails both `TestRouted_LocalFailsClaudeFixes` and `TestRouter_FailingCheckEscalatesOnceWithContext`.
### Steps 180-183 — live routed runs, and a judge flaw found by them

- **Step 180** (`run-fixture.sh --route auto --family repo-task --split holdout --limit 3`, config as generated): `go-data-race`, `go-join-trailing`, `go-nil-map` all `route=local_only route_reason=local_only check=pass escalated=false cost $0`, 1 local attempt each (qwen-coder-14b), checks in the real sandbox (`go test -race` with cgo inside it). Step 181: consistent with the config (repo-task is `local_only`, adopted on hold-out).
- **Extra live check (not in the plan, $0.025502):** the config never escalates a repo task, so the live Claude leg was exercised once with a scratch routing file (`repo-task: local_first, local_attempts 1, escalate_on [check_fail]`) on `py-slug`, a tune fixture the local model fails: `route_reason=escalated_check_fail`, Claude's tools in the worktree `write_file` allowed (branch-sandbox), `run_check` allowed, `check=pass`.
- **Finding: the check judge counted a weakened test as a pass.** The `py-slug` worktree showed the local draft had also edited `test_lib.py`. A sweep of the kept answers: qwen-7b's passing `py-chunks-partial` answer changed the failing assertion (`chunks([1,2,3,4,5], 2) == [[1,2],[3,4],[5]]` removed). So qwen-7b's repo-task score is 16/20, not 17/20; qwen-coder-14b's 19 passes and Claude's 20 edited no test (their only test edit, 14b on `py-slug`, failed anyway). The routing decision used qwen-coder-14b and Claude and is unchanged. Fixed in both judges: a fixture's test and validator files (`*_test.go`, `test_*.py`, `validate_runbook.py`) are protected like `check.sh` in `zhen-eval` and in the routed loop with `fixture-check` (a draft that touches one is refused; a check after one changed does not count); the system prompts now say so. The recorded result files are left as they were run.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| Phase 0 (Step 30) | PASS | on `sprint/two-brains`; H1-H16 recorded; baseline 277 ok / 0 FAIL; commit `959ab714` |
| Phase 1 (Step 62) | PASS | 030 offline checks + throwaway tests, bite-checked (owner UPDATE/DELETE allowed without the trigger -> 3 FAIL); `go test -race` claudebridge/zhen-claude/zhen-agentd ok; `pg_dump` 0 marker hits; Steps 59/61 escalated |
| Phase 2 (Step 98) | PASS | self-approval and no-leak tests pass and bite (leaking the code into the result and approving on no answer -> 3 FAIL); 031 throwaway tests pass, bite-checked; default `-confirm deny` |
| Phase 3 (Step 124) | PASS | no planted value survives (`TestRedact_NoPlantedValueSurvives`); `-tags privacy_bite` -> 12 FAIL; fail-closed paths tested (scanner missing, failing, hung; prompt not sent; tool result withheld); default `-privacy standard`. Step 121 live: `service_health` result tier T1, mode standard, 0 redactions (the payload holds service names and ports only, no addresses); consistent. |
| Phase 4 (Step 160) | PASS | results for qwen-7b, qwen-coder-14b and Claude Opus 5.5 (low); `configs/zhen-routing.yaml` generated with split/PREREG hashes; hold-out gate evaluated per family (expert, hard, repo-task adopted; prod, textbook rejected -> claude_direct); eval spend $1.31 <= $3 cap; PREREG commit (10:36) older than every results commit (11:06, 11:18, 11:45). Step 142 failed and was debugged (142a; see Decisions). |
| Phase 5 (Step 196) | PASS | `run_check` sandbox tests pass on west (`-tags usersystemd`: network, home, memory, runtime, output, tasks, Go toolchain); router obeys the config in unit tests, fake end to end and 3 live runs (all `local_only` per config) plus one live forced escalation; every escalation carries a `route_reason`; Step 194 bite fails both escalation tests; spend recorded. |

Final regression (Step 192): `go build ./...` ok; `go test ./...` 281 ok / 0 FAIL (277 baseline + `pkg/localllm`, `pkg/sandbox`, `pkg/claudebridge/privacy`, `cmd/zhen-eval`); `go test -race` on every touched package ok; `-tags usersystemd` ok; pytest `raft/tests` + `scripts/two-brains` 86 passed, 2 skipped; golangci-lint 0 issues and gosec 0 issues on touched packages; both throwaway Well tests pass; no `qa-` container left. `check-gates-can-fail.sh --quick`: the two INCONCLUSIVE gates (`check-ruff`, `check-python-audit`) need `ruff`/`pip-audit` on PATH, pre-existing on this host.

Definition of Done, Phases 0-5: every step run, skipped with a reason, or escalated (below); zero new test failures; migrations 030/031 in ledger order with the k8s render, tested and bite-checked on throwaway servers only; no new third-party dependency (yaml.v3, lib/pq, anthropic-sdk-go were already in go.mod; gitleaks and systemd-run are existing host tools); attack surface reviewed (Steps 54, 86, 117, 169, 184); ADR-019 amendment drafted (Proposed); ADR-104 open questions 2 and 5 answered; ADR-053 active.

## STUCK

No step was marked STUCK in Phases 0-5. Steps not run as written, with the reason:

- **Step 61** (live check of 030) — not run: depends on Step 59 (ESCALATE, open).
- **Step 89** (interactive live `-confirm tty`) — skipped: no human at the terminal; the fake-TTY tests cover the prompt.
- **Step 121** — run; 0 redactions because the `service_health` payload holds names and ports only (consistent).
- **Step 147** (effort medium) — not needed: Claude low passed every task.
- **Step 148** — n/a (credential present).
- **Step 157** (restore qwen-7b) — replaced: qwen-coder-14b (the config's local model) stayed loaded for Phase 5's live runs, then llama-server was stopped, its state before the sprint (H10 down).
- **Step 168** — n/a (H14 held).
- **runbook-draft** family (5 tasks) — `[BLOCKED]` on the Phase 8 harness, as the plan says; not scored.

## ESCALATE answers

- **Step 59 [ESCALATE] — OPEN.** Apply migration 030 (`unheaded_ops.llm_requests`) to the live Well? Additive, new append-only table, tested on throwaway servers (`scripts/well/test-llm-requests.sh`, bite-checked). The Well is down (H8) and live apply is Stevie's call (ADR-101 `well-migrate apply -yes`). Not applied; Step 61 (live check) skipped. Batch with 031 (Step 94).
- **Step 94 [ESCALATE] — OPEN.** Apply migration 031 (`unheaded_app.zhen_pending_confirmations`) to the live Well, batched with 030 (Step 59). Additive new table, tested and bite-checked on throwaway servers (`scripts/well/test-zhen-confirm.sh`). Until it is applied, `-confirm queue` and `zhen-agentd -confirm-queue` cannot start against the live Well (they need the table); `-confirm tty` and `deny` work without it.

## Spend log

All live Claude calls of this run go through `-ledger $SCRATCH/spend.json`
(run-private), `-effort low`, small `-max-tokens`.

| Step | What | Cost (USD) | Running total |
|---|---|---|---|
| 27 | smoke, `-effort low`, 1 request | 0.001004 | 0.001004 |
| 121 | `-tools mcp`, summarise `service_health`, `-effort low`, 2 requests | 0.010566 | 0.011570 |
| 145 | two-brains eval, Claude Opus 5.5 effort low, 76 tasks, 76 requests (ledger now 79: +1 smoke, +2 Step 121) | 1.312553 | 1.324123 |
| 180 | live routed runs, 3 hold-out repo tasks (local_only per config) | 0.000000 | 1.324123 |
| 180+ | extra live escalation check, py-slug, scratch routing (separate ledger spend-phase5.json) | 0.025502 | 1.349625 |
| 183 | the same 3 fixtures, `--route local` (comparison) | 0.000000 | 1.349625 |

**Total live Claude spend for this run: $1.349625** (run ledger $1.324123 + Phase 5 ledger $0.025502), under the $5 budget.

---

# Phases 8-9 (branch `sprint/two-brains-runbooks`, from `14de0842`)

Executed by a separate worktree agent while Phases 6-7 ran on
`sprint/two-brains`; this block is kept apart from the sections above so the
two branches merge without conflicts. Unsigned commits, nothing pushed, no
host touched: every runbook test runs against throwaways (sandbox git
repositories, a throwaway Wotan on free ports, temp dirs).

## P8/P9 Decisions

- **Step 259** the corpus is 67 runbooks (not 66): all `runbook/v1`; actions `shell` 460, `manual` 40; `host` west 9, east 9, `${HOST}` 3; two carry `status: stub`.
- **Step 260** [DECIDE] recommendation taken. Additions: `host` may be `${VAR}` for a declared parameter/env key (3 existing steps); `irreversible-manual` is `irreversible: true` or a pattern list (git push, pg_restore, dropdb, DROP, shred, rm -rf, mkfs, wipefs, reflog expire, gc --prune, reset --hard, branch -D) in a shell step of a runbook with a status; strict rules apply to any runbook with `metadata.status`, so the two legacy `stub` runbooks are baselined rather than exempt. A baseline entry that no longer fails is reported as removable and fails the gate (a fix removes its line in the same commit), and `--write-baseline` refuses to grow an existing baseline. The runner's `verification:` gap stays parked.
- **Step 266** baseline: 24 entries (3 `metadata.name` != file stem in `wotan/`, the 2 stub runbooks' strict-rule gaps, 4 rollback entries without `action`).
- **Step 271** pytest was not in the CI hash lock: added `pytest==9.1.1` to `.github/ci-python/requirements.in` and its hashed entries (pytest 9.1.1, iniconfig 2.3.1, pluggy 1.7.0; packaging and pygments were already locked) to `requirements.txt` by hand from PyPI's digests, not by re-running `ci-python-lock.sh` (a re-resolve would have moved other transitive pins). All three packages were already in `docs/policy/dependency-baseline.txt` (no new dependency); all older than the 7-day cooldown. The full lock installs with `--require-hashes --no-deps --only-binary=:all:` in a clean venv.
- **Step 273** the meta-gate only discovers `scripts/*.sh`, so the gate is `scripts/check-runbooks.sh` (a wrapper over the Python checker; CI calls the wrapper). `check-gates-can-fail.sh` had no selector: added `--only NAME` (fails if NAME matches no registered gate).
- **Step 272** the pre-commit hook's workflow check is a PyYAML parse of staged workflow files; the same parse was run directly (the hook itself could not be invoked from this sandboxed agent).
- **Finding (pre-existing on the sprint branch, parked):** `scripts/check-manifest-yaml.sh` fails on `eval/two-brains/fixtures/rb-bad-indent/runbook.yaml` and `rb-unquoted-colon/runbook.yaml`, Phase 4's deliberately broken fixtures, so the `manifests` CI job will be red when the sprint is pushed. Fix (for whoever merges): exclude `eval/two-brains/fixtures/` in that script, the way it excludes vendored trees. Not changed here (Phase 4 territory).
- **Step 277** [DECIDE] recommendation taken, with three recorded changes: Claude review at effort **low** (the run's instruction; the plan says medium); the local model is shown one schema summary `docs/runbook-briefs/SCHEMA.md` plus the two exemplars, and not the checker source (copied into the worktree untracked, so the sandboxed check runs it but the prompt does not carry it, which keeps the prompt inside qwen-coder-14b's 8k context); and the final verdict is always the repo's own `scripts/check-runbooks.py` run by the script on the draft, never the worktree copy a model could have edited. Authorship fields are written by the script, not by either model.
- **Step 279** `scripts/run-runbook.py` printed "steps are not implemented" for `status: draft`, which was the stub wording; drafts now get their own banner (not approved yet; manual steps printed, never executed).

## P8/P9 Gates

| Gate | Result | Evidence |
|---|---|---|
| Phase 8 (Step 276) | PASS | `tests/runbooks` 38 passed (checker rules each red then green; harness self-tests incl. a throwaway Wotan); `check-runbooks.sh` exit 0 with the baseline, exit 1 with one line deleted; `check-gates-can-fail.sh --only check-runbooks`: bites (exit 1); workflow parses |
| Phase 9 (Step 338) | PASS | 11 runbooks pass `scripts/check-runbooks.py` (corpus: 78 runbooks, 0 new findings, baseline still 24); `pytest tests/runbooks -m "not live"`: 100 passed (with `-m` dropped, the 2 live tests skip with their reasons: FRR_LAB, WELL_LIVE_TESTS); `scripts/two-brains` 18 passed; `check-gates-can-fail.sh --only check-runbooks` bites; all 11 `status: draft`, `approved_by: ""`; every push, force-push, restart, key handling and Console/GitHub action is `action: manual` |

## P9 pipeline amendments (all recorded in their commits)

- **280a** (R1): qwen-coder-14b answered the review with the reviewed file unchanged and the harness still passed, so nothing would have fixed 9 high/medium findings. **280b** (R2): a changed file is no evidence either (R2's revision left most high findings open). Rule now: Claude's patch leg runs whenever a high or medium finding was raised (or the harness fails); it checks which are still open and applies those. `--resume-patch` reruns only that leg (used for R1-R3, which were drafted under the earlier rule).
- **280a**: the privacy tier redacts host names in prompts (`west` -> `[REDACTED:host#1]`) and one placeholder came back into R1. New checker rule `redaction-placeholder`; the patch prompt says so.
- **280c** (R4, R5): the `runbook-patch` task budget ($0.40) ran out mid-edit on R4 and left a truncated file (raised to $0.60); with two exemplars R5's prompt reached ~6.7k of the local model's 8k context and three attempts hit the 10-minute timeout (one exemplar from R5 on, timeout 20 min).
- **309a** (R6): `run-runbook.py` left `$HOME`/`~` literal in parameter defaults; expanded now.
- **329a** (R5, R6, R10): checker rule `negated-command`: `! cmd` before the last line of a set -e script never fails.
- **336a**: the meta-gate classifies `scripts/zhen-mcp.sh` (named by R10) as not a gate.
- R9: the local model gave no applicable draft in 3 attempts (answers had no usable file block, most likely cut at the 3000-token answer limit). The Step 277 override (3 failures in a row -> `claude_direct`) did not trigger (R8 and R10 around it succeeded); R9 alone was written by Claude through the patch leg.
- **Plan amendment, R2 (H12):** the plan's test had the pending subscriber "appear in /api/v1/admin/pending"; that endpoint answers 401 by construction today. The test pins the 401 and detects the pending member by the subscribe answer, the log line and the `unheaded_chat_members_pending` gauge.
- Every final runbook was read by this session; where a patched draft was still wrong, it was fixed here (R1 placeholder; R3 detection moved out of preconditions, which the runner never prints; R4 finished after the budget-cut patch; R5 rewritten: cert-gen run per service mints a new CA each time, a `-service` flag that does not exist, an invented port; R6-R8, R10, R11 smaller fixes, each named in its commit). `authored_by` is `claude` for all eleven: in every case Claude's patch or this session wrote the final text.

## P9 runbooks

Local = qwen-coder-14b via `zhen-claude -route local -family runbook-draft` (routed loop, `run_check runbook-check` in the sandbox). "Local draft" is whether its first draft (after up to 2 revisions) passed the harness; "revisions" counts every local attempt after the first, review apply included.

| # | Runbook | Local draft | Revisions | Review findings | Local applied review? | Claude patched | Fixed in session | Test (not live) | Claude $ |
|---|---|---|---|---|---|---|---|---|---|
| R1 | network/bgp-bfd-session-down | pass | 4 | 12 | no (unchanged) | yes | placeholder | 7 pass, 1 live skip | 0.225 |
| R2 | wotan/pending-subscription-approval | pass | 1 | 10 | partly | yes | - | 6 pass (throwaway Wotan) | 0.202 |
| R3 | wotan/oom-state-recovery | pass | 3 | 10 | partly | yes | detection steps | 3 pass (throwaway Wotan, kill -9) | 0.216 |
| R4 | data/well-restore-from-east-offsite | pass | 3 | 12 | partly | yes, cut by budget | finished, host guards | 6 pass, 1 live skip | 0.239 |
| R5 | pki/ca-compromise-root-rotation | fail (timeouts), then pass | 3 | 12 | partly | yes | rewritten | 6 pass (openssl sandbox, s_server) | 0.199 |
| R6 | security/git-history-secret-purge | fail, then pass | 4 | 12 | partly | yes | scan-before, negation, PATH | 5 pass (filter-repo + gitleaks sandbox) | 0.322 |
| R7 | deploy/resign-unpushed-commits | pass | 4 | 10 | partly | yes | base check, probe step | 4 pass (ephemeral gpg key) | 0.186 |
| R8 | deploy/branch-promotion-develop-staging-main | pass | 1 | 12 | partly | yes | verify-push manual | 4 pass (signed sandbox) | 0.220 |
| R9 | security/claude-api-key-rotation-spend-breach | fail (no file) | 2 | - | - | wrote it | - | 5 pass (cap refuses before send) | 0.128 |
| R10 | security/mcp-write-exec-enablement | pass | 2 | 12 | partly | yes | `! grep` verify (test caught it) | 5 pass (real MCP server, gate flips) | 0.188 |
| R11 | observe/chaos-game-day | pass | 3 | 12 | no (unchanged) | yes | steady/observe steps (test caught it) | 8 pass | 0.242 |

`runbook-draft` family, first real data (Phase 4): the local model produced a schema-valid draft for 10 of 11 briefs (R5 needed the smaller prompt), but applied review findings fully in 0 of 10 cases; Claude's patch was needed every time, and the session still found a defect in 9 of 11 patched files, 2 of them caught by the dry-run tests. The schema harness alone would have passed every one of those defects.

## P9 findings (not fixed here)

- **llama-server prompt cache vs the never-OOM rule.** llama.cpp's default `--cache-ram 8192` (MiB) is outside `scripts/switch-model.sh`'s fit check. After ~2.5 h of pipeline traffic llama-server's RSS was 9.4 GiB, MemAvailable 3.7 GiB, and the server wedged in `prompt_save` (requests queued, none processed; `/health` still ok). Restarted with `LLAMA_ARG_CACHE_RAM=0` (environment only, no script change). For Stevie: put `--cache-ram` into `MODEL_FLAGS` or the fit math.
- The routed local path (`-route local -family ... -worktree`) does not write `zhen-capture/v1` records for the local drafts (only the Claude legs were captured); the drafts survive in each agent worktree's git history under `~/.cache/unheaded/zhen-worktrees/runbook-*`. Phase 10 should capture routed local turns.
- The privacy tier's host-name redaction applies to Claude's T0 brief text too when it is part of the prompt (residual risk 3 above, observed).
- `git-filter-repo` 2.47.0 (MIT) installed into `~/.venv/zhen` for R6's test: an operator tool, not a shipped dependency, already in `docs/policy/dependency-baseline.txt` (Barrister note).

## P9 STUCK / ESCALATE

- No step STUCK.
- **Step 337 [ESCALATE] — OPEN.** Stevie approves runbooks (human sign-off by definition, ADR-024 owner). For each approved one: `metadata.status: approved`, `approved_by: stevie`, one commit per batch. Briefs in `docs/runbook-briefs/`, review records in each agent worktree (`findings.json`, `review.txt`, history). Non-blocking for Phases 10-14.

## P9 spend log

Run-private ledger `$SCRATCH/s2rb/spend.json`, `-effort low`; cost per runbook from each leg's own audit lines (review + patch).

| What | Cost (USD) |
|---|---|
| R1-R11 review legs (10) | 0.556 |
| R1-R11 patch legs (11, R9's wrote the file) | 1.811 |
| **Total Phases 8-9** | **2.367316** (cap $5) |
