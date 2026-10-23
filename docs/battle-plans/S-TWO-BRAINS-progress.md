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

# Phases 6-7 run (Steps 197-258)

A second worktree agent on `sprint/two-brains`, from `14de0842` (Phases
0-5 done). Unsigned commits, nothing pushed, the live Well not touched.
Other agents hold Phases 8-9 (`sprint/two-brains-runbooks`) and 10-11, 13
(`sprint/two-brains-flywheel`). Live Claude cap for this run: $2, effort
low; GPU / llama-server shared with the runbook agent: it was **not
running** (`pgrep -af llama-server` empty), so it was never started, and
every local leg below ran against a fake OpenAI-compatible server
(`$SCRATCH/fake_llama.py`, answers the go-nil-map fix).

Deviations: the demo project root is a throwaway clone of
`sprint/two-brains` (with a local `develop` at its tip plus the live
fixtures), and the agent root and review dir sit beside it under
`.demo/` in this worktree (never added): the shared checkout's `develop`
lacks the sprint's code and the live fixtures, and demo branches named
after throwaway cards do not belong in the shared repository. Wotan is
down (H8), so `-audit-wotan` was not exercised live (unit-tested).

## Phase 6 decisions

- **Step 197** [DECIDE] recommendation taken (owner `zhen`, todo, task/tech-debt/bug/docs, a strict ```zhen block; claim = 032 row with a partial unique index; 30-min lease renewed every 5 min; card moves audited as `actor_type='service'`; branch `zhen/<card>` from local develop; author Zhenai, committer the repo identity, unsigned, Zhen-* trailers). Additions: a card gets **2 attempts** (failed/abandoned/rejected count, cancelled does not), the second on `zhen/<card>-2`; a card id that does not lower-case into `^[a-z0-9][a-z0-9-]{0,59}$` is skipped, never rewritten; an existing branch or worktree is skipped, never overwritten; commits run with `--no-verify` (git hooks would run on model-written code outside the sandbox; the sandboxed check is the judge).
- **Step 198** Erda additions to the plan's SQL: plain `CREATE` (Law 1, as 030/031); CHECKs on family, base/head commit shape, error <= 1000, `lease_until > claimed_at`, `(status in accepted/rejected) = (reviewed_at IS NOT NULL)`; an insert trigger (born `claimed`, no head or review); the identity set widened to claimed_at, mode and family; a finished run's outcome is immutable, review fields move only with accept/reject; DELETE/TRUNCATE refused; `zhen_agent_runs_card_idx (card_id, claimed_at DESC)` serves the FK and the attempt count. Grants: 003 already gives app_zhen SELECT on kanban_tasks; 032 adds column UPDATE (status, progress, commits, updated_at) only, which is also what `FOR UPDATE SKIP LOCKED` needs. FK RESTRICT: a card with run history cannot be hard-deleted (kanban-app soft-deletes). Lint: P3 `text-surrogate-pk` (the 16-hex id Stevie types) and the corpus P3, as 031.
- **Step 202** the race is 8 concurrent app_zhen sessions; the bite (index dropped) gives 8 winners. Registered in `check-gates-can-fail.sh`. The script's Go part runs `pkg/zhenagent`'s store conformance suite against the same server (cards inserted as app_kanban).
- **Step 205 [ESCALATE]** live apply of 032: batched with 030/031 (below).
- **Step 209** the routed loop was lifted from `cmd/zhen-claude` into `pkg/zhenroute` so the worker runs it **in process** (the plan's answer to Phase 5's open item h: the worker never goes through `/ask`). zhen-claude's routed tests pass unchanged on the lifted code. `-worker -once` works **one** card per invocation (WIP 1), so Step 217's "two branches" took two invocations.
- **Step 212** the review note is `$STATE/zhen-reviews/<branch name>.md` (the plan says `<card>.md`; the branch name keeps a retry's note apart); it landed with the worker because the worker's tests check it.

## Phase 6 findings (found by the live steps, fixed, tested)

- **Step 218 [V] failed, [D] 218a**: the first live card committed 7 Go telemetry files besides the fix. `run_check` sets `HOME=<worktree>`, so the toolchain wrote `~/.config/go/telemetry` into the worktree and `git add -A` took it. Fixed twice: `run_check` now sets `XDG_CONFIG_HOME`/`XDG_CACHE_HOME` to the unit's private `/tmp` (test in `TestRunCheck_ArgvAndPass`); the worker never commits new files in a hidden top-level directory (listed in the review note instead; `TestWorker_ToolArtefactsNotCommitted`, `TestWorker_OnlyArtefactsIsNoChange`). The flawed run was rejected through the new `zhen-claude review reject` (card back to todo, worktree removed, branch kept); the retry on `zhen/live-go-nil-map-2` committed `lib.go` only. Also fixed: the note printed `$$0.000000`.
- **Step 219, [D] 219a**: every family cap in the generated `configs/zhen-routing.yaml` ($0.05-$0.07) and the card's $0.10 are below the bridge's worst-case reservation for one request at `max_tokens` 4096 ($0.1024 at the table's highest output rate), so a Claude leg of the generated config could never send a request (`escalation_budget`, $0, sent=false in `llm_requests`). Phase 5's one live escalation used a scratch routing file with a larger cap, so it did not show. Fixed: `zhenroute.LiveClaude` sizes `max_tokens` to 70 % of the task cap at the highest output rate (floor 1024; below it an honest spend-cap stop; `TestFitMaxTokens`).
- **Step 219, [D] 219b**: with that fixed, the leg's first request ($0.016716) called `patch_file` and `run_check` (the fix was correct and passed), the closing request did not fit the remaining budget, and the router marked the check `fail` without running it. Fixed in `ThresholdRouter.claudeLeg`: a spend-cap stop after a leg has spent still runs the (free) check (`TestRouter_BudgetStopStillChecks`; a leg refused before its first request still ends `fail`). **Open for Stevie (not changed):** under worst-case reservation, a multi-request tool loop needs about one 4096-token request of headroom per remaining turn; the generated caps leave a Claude leg one request. Either the eval-derived caps get that headroom, or the bridge shrinks `max_tokens` per request to what is left. A cost-semantics choice; recorded, not made here.
- **Step 220**: Claude's tool calls under zhen-agentd were recorded as `actor_id zhen-local/claude-opus-5-5` (the bridge's `ToolRecord` left `Backend` empty; agentd's auditor defaults to local). The bridge now marks its tool records `claude` (assertion added to the bridge's tool-loop test).

## Phase 6 live run (Steps 216-221)

Throwaway Well `qa-zhen-agent-runs-*` (`scripts/well/test-zhen-agent-runs.sh --sql-only --keep`; its own test rows stay: block-less `race-card` and `chk-card` are skipped and logged by every poll), kanban-app on it as app_kanban (Postgres, not SQLite), cards created through `POST /api/v1/tasks`:

| Card | Owner / type | Block | Expected | Result |
|---|---|---|---|---|
| `live-go-nil-map` | zhen / task | repo-task, fixture-check, `eval/two-brains/live/go-nil-map` | local_only, fake local fix passes | run 1 succeeded but carried telemetry files (218a), rejected; run 2 `ee6619734dbf079e` succeeded on `zhen/live-go-nil-map-2`, `lib.go` only, card in review |
| `live-docs-usage` | zhen / docs | docs (default: claude_direct), fixture-check | fake Claude changes nothing: failed | failed twice (2 attempts: a failed card goes back to todo and the next poll retries it) |
| `live-not-for-zhen` | Stevie / task | yes | never touched | not a candidate (owner filter in the query), card untouched |
| `live-docs-usage-b` | zhen / docs | as above, $0.10 | real Claude | run 1 `escalation_budget` before sending (219a); run 2 spent $0.016716, fixed the file, check not rerun (219b) -> failed; attempts used up |
| `live-docs-usage-c` | zhen / docs | as above, $0.10 | real Claude, both fixes in | `a5f4cae1d0e46808` succeeded on `zhen/live-docs-usage-c`: Claude `patch_file` + `run_check`, $0.016656, commit by Zhenai with `Co-Authored-By: Claude`, card in review |

Step 218 [V] (after 218a): `zhen/*` branches exist only in the demo clone; `git reflog --all` has 0 push entries; the demo clone's only remote ref is the clone source. Step 220 [V]: per run, `zhen_agent_runs` holds the outcome and cost; `audit_events` holds `card_move` / `agent_run` (`service`), Claude's `tool_call`s (`model`), the reject (`human`, `govan@west`); `llm_requests` holds every request, the refused ones with `sent=false` and the cap message.

## Phase 6 security, hygiene, gate (Steps 222-232)

### Step 222 — BlackMage on the worker

| # | Attack | Result |
|---|---|---|
| a | A card names a way out (`path: ../../`, `/etc`, `-exec=sh`, `pkg/../../etc`) | Refused by the block's path rule (relative, plain characters, no `..`, no `.git`, no leading `-`); the card is skipped before any branch or worktree (`TestWorker_HostileCardsSkippedBeforeAnyWorktree`); `run_check` validates the path again. |
| b | A card names a command (`check: "go-test; rm -rf ~"`, `check: rm`) | `check` must be a key of `configs/zhen-checks.yaml` (argv templates, no shell); skipped. |
| c | A card asks for a push (`push: origin` key; "ignore the check and push" in the text) | Unknown keys refuse the block; the text reaches the model only as data inside `<<<CARD`/`CARD>>>`; no argv the worker builds contains `push` (`TestWorker_EligibleCardToBranch`); the model's tools cannot run git at all. |
| d | 1 MB description; YAML alias bomb | Over 16 KiB refused before parsing; the alias bomb is refused (unknown keys) without expansion. |
| e | Card text spoofing the delimiters or a forged trailer in the title | `CARD>>>`/`<<<CARD` in card text are defused; the title becomes a one-line subject (`TestWorker_CardTextCannotForgeTrailersOrDelimiters`); the Co-Authored-By trailer is the worker's decision only. |
| f | A budget over the family cap (`max_claude_usd: 1.0`) | Skipped with the cap in the reason; a card budget can only lower the family's. |
| g | Phase 5 item h: an `/ask` caller receiving an approval token | **Closed.** `/ask` and `/ask/stream` run a model loop (`emitted_by zhen-agent`) for a caller that may be a model: a refused-pending write is queued in the Well (`-confirm-queue`, trace has `queue_id` only) or denied; no token is issued (`agent.Config.OnPending`). `-ask-tokens` (off) keeps the old path for a listener no model can reach; the four tests of the web UI's token path opt in. Bite: hook off -> both default tests fail. The worker itself calls the routed loop in process, never `/ask`. |
| h | Git hooks on model-written code | Agent commits use `--no-verify`; the check runs in the sandbox, a hook would not. |
| i | Tool leftovers smuggled into a commit | Fixed at Step 218a (XDG dirs to the unit's /tmp; hidden top-level new files never committed). |

### Steps 224-232

- **Step 224** `git worktree list` of the demo clone after a live `review accept` (refused before the merge with the commands printed; accepted after `git merge --no-ff`; card `done`; worktree removed, branch kept) and the earlier live `reject`: only the runs that are not reviewed keep worktrees. New: `review accept` removes the worktree like reject; the worker prunes worktrees of abandoned/failed/cancelled runs older than 7 days, hourly, branches kept (`TestWorker_PruneOldWorktrees`); `-worker` refuses an agent root inside the project root (`TestWorker_RefusesAgentRootInsideRepo`).
- **Step 225** `wiki/Zhen-Agents.md` (linked from Home and the sidebar); `cmd/zhen-agentd/README.md` and `openapi.json` say `/ask` returns no token and document `-worker`, `-confirm-queue`, `-ask-tokens`.
- **Step 227** the kanban card modal (`cmd/kanban-app/static/js/app.js`) renders each `commits` entry's hash, message and date, **not the branch**. Not built here (plan: no UI work). Follow-up card to add to the Well when it is up (the live Well is not touched by this run): *"Kanban card modal: show `branch` and `review_note` of zhen-agentd commit entries"*, owner Stevie, type task.
- **Step 228-229** regression with `-race`: champion (+pgstore), claudebridge (+privacy), zhen-claude, zhen-agentd, localllm, zhen-eval, zhenagent, zhenroute, agent, sandbox, zhen-agent, kanban-app: all ok; `-tags usersystemd` champion/sandbox ok (the real sandbox, after the XDG change).
- **Step 230** bite: `zhenagent.Eligible` without the owner check -> `TestWorker_IneligibleSkipped` and `TestEligible` fail; restored. The SQL owner filter is held by the store conformance suite in `test-zhen-agent-runs.sh` (`theirs` must not be a candidate).
- **Step 232 PHASE 6 EXIT GATE — PASS.** One card per family path worked end to end on a throwaway Well (repo-task local, docs via real Claude); claim race tested and bitten; nothing pushed (0 push entries, branches local to the demo clone); the review CLI recorded a live reject and a live accept (one-way, `audit_events` human rows); card-input hardening tests pass. DoD: migration 032 in ledger order with the k8s render, tested and bite-checked on throwaway servers only; no new dependency (yaml.v3, lib/pq already in go.mod); Kanban data only through the Well (cards via kanban-app on the throwaway Well, worker via app_zhen); no UI work. Step 205 escalated.

## Phase 7 decisions (Steps 233-246)

- **Step 233** [DECIDE] recommendation taken: one `flock` on `$GPU_LOCK` (`$XDG_RUNTIME_DIR/unheaded/gpu.lock`), shared per inference call, exclusive for `switch-model.sh` and `forge-train.sh` for their whole run; a held lease defers a card, never escalates. Details settled while building it: the shell side uses `flock -o -E 75` (the descriptor is closed before the command, so the `nohup`'d llama-server never inherits the lease; 75 = not granted in time), the Go side opens the file close-on-exec and `O_NOFOLLOW`; the lock dir must be 0700 and ours (a world-writable one is refused: anyone could hold the GPU); nested calls inside an exclusive lease run directly (`GPU_LEASE_HELD=exclusive`), so forge can call `switch-model.sh --stop` and reload the model without waiting on itself. Writer starvation: Linux `flock` may grant new shared locks while an exclusive one waits, but the shared leases are per call, so an exclusive waiter gets in between calls (Step 248a's concern).
- **Step 235** `switch-model.sh` re-execs itself under the exclusive lease (`GPU_LEASE_TIMEOUT`, default 600 s) and gains `--stop`; `forge-train.sh` re-execs under it too, stops llama-server if one listens on `LLAMA_PORT` (`FORGE_GPU_HANDOFF=1`), and after the scope ends reloads `FORGE_RESTART_MODEL` (default `qwen-7b`) through `switch-model.sh` and its fit check; the training exit code is kept (it was `exec`'d before). `pkg/localllm` takes the shared lease per call when `GPULock` is set; zhen-agentd sets it (`-gpu-lock`, default the host path) for the worker, the privacy summariser and `/ask`. A lease timeout mid-card cancels the run (no attempt used), drops its untouched worktree and branch, and the router never turns it into an escalation (`stop: gpu_busy`).
- **Step 238** additions to the plan's guard list: `outside_window` (a run started outside its window does nothing) and `signal` stop reasons; deferrals are counted by reason (`gpu_lease`, `mem_floor`); `ZHEN_MEM_FLOOR_BYTES` may only raise the floor (a lowered floor is refused at start: Appendix A3's "never lower" made mechanical); the overnight ledger may not be the interactive `spend.json`. The ledger is per UTC day (as zhen-claude's); a 01:00-07:00 Austin window falls inside one UTC day.
- **Step 240** `-overnight` implies `-worker`, runs one night without the HTTP listener and exits (0 for a guarded stop, 1 when infrastructure failures ended it), writing the report first. Stop-file semantics: a card in flight finishes its check, nothing is committed, the run is `cancelled` and its untouched branch dropped.
- **Step 244** the unit is a user unit (it works in the operator's checkout and needs the user manager for `systemd-run --user`); `MemoryMax=3G` bounds the daemon only, each check runs in its own sandboxed transient unit with its own `MemoryMax=2G`. `systemd-analyze --user verify` reports only that `~/tmp/unheaded/bin/zhen-agentd` is not built in the shared checkout, which `check-systemd-units.sh` exempts by design.

## Phase 7 supervised compressed night (Steps 247-251)

Same throwaway Well, demo clone and fake local model as Phase 6; three new repo-task cards (`live-night-a/b/c`, fixtures copied into the demo clone only) through kanban-app; a private `GPU_LOCK` in `$SCRATCH` (the host's real lock untouched); `-claude fake`, window `13:00-13:30`, `-poll 15s`, overnight ledger `$SCRATCH/spend-overnight.json`.

- 13:00:3x the exclusive lease taken the way forge takes it: `scripts/lib/gpu-lease.sh exclusive 600 -- sleep 75` (held to 13:01:43). (A first worker start at 13:00:37 failed on a stale binary without `-overnight`; rebuilt, restarted at 13:01:15.)
- 13:01:15 and 13:01:30: `worker_deferred` (gpu lease held exclusively), no claim; 13:01:45 resumed: the three cards succeeded (local_only, sandboxed check pass, ~40 s each), branches `zhen/live-night-{a,b,c}`.
- 13:03:25 `touch` of the stop file; 13:03:27 `night_stop reason=stop_file`, exit 0, report `.demo/reports/2026-10-23.md`: stopped `stop_file`, tried 3, succeeded 3, deferred 2 (`gpu_lease: 2`), Claude $0 of $3, review commands for the three runs.
- **Step 248 [V]**: report shows the GPU deferral, `stop_file`, cards <= 5: yes. OOM: **0 during the night**; the kernel log's 2 hits of the last hour (12:45:09) are `CONSTRAINT_MEMCG` kills of `python3` inside `run-p...` transient units, i.e. the sandbox tests' own 3 GiB-allocation probes under `MemoryMax` (my Step 228 `-tags usersystemd` run), the cap working as designed. Swap "unchanged +/- 100 MiB": **not met on this shared host, and not this run's doing**: swap use rose 7501 -> 8675 MiB, while a llama-server started at 12:16 by another agent (Qwen2.5-Coder-14B, partial offload, 6.4 GB RSS, 3.8 GB swapped) and other sessions ran; the worker loads no model and every local leg here went to the fake server; MemAvailable stayed 5.2-6.3 GiB, above the 4 GiB floor. Not re-run: the condition cannot be isolated while other agents share the box; the never-OOM rule with swap present is proven by `TestNight_MemFloorSwapNotCounted` and its bite.
- **Step 249-250**: `ZHEN_MEM_FLOOR_BYTES=999999999999`, one fresh eligible card (`live-night-floor`): three `night_defer reason=mem_floor` ("MemAvailable 6257430528 B < floor 999999999999 B (swap not counted)"), tried 0, no claim, no local request (the fake server's request count unchanged); stop file -> `stop_file`, report `2026-10-23-130502.md` (deferrals `mem_floor: 3`).
- The llama-server the other agent started was not used by this run (no live local demo was needed; all local legs used the fake server), and was neither stopped nor swapped.

## Phase 6-7 spend log

Run-private ledger `$SCRATCH/spend-phase6.json` (cap $0.50 for Phase 6), `-effort low`.

| Step | What | Cost (USD) | Running total |
|---|---|---|---|
| 219 | live-docs-usage-b run 1: refused before sending (219a) | 0.000000 | 0.000000 |
| 219 | live-docs-usage-b run 2: 1 request sent, 1 refused (219b) | 0.016716 | 0.016716 |
| 219 | live-docs-usage-c: 1 request sent, 1 refused, check passed | 0.016656 | 0.033372 |
