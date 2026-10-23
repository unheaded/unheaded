<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-104: ZhenAI → Claude API bridge

**Status:** Proposed (built on `develop`; not yet run against the live API: there are no credentials on west)
**Date:** 2026-10-23
**Supersedes:** ADR-017's "Claude API SCRAPPED (2026-04-03)". Stevie, 2026-10-23: "let's have agents work on mcp to Claude api".
**Related:** ADR-017 (hybrid inference), ADR-019 / ADR-037 (Champion), ADR-031 (hybrid model handoff), ADR-053 (HEAVY → Claude, MINOR → local), ADR-096 (dependency gate), ADR-004 (exceptions register), `eval/coding-gate/probe-2026-05-02/B2-design-champion-tool-call-gate.md` (the B2 gate), `pkg/zhentools`, `pkg/mcpclient`.

## Context

ADR-017 dropped a direct Claude API integration in April in favour of the
other direction: Claude Code calls Zhen's tools through the MCP server
(`raft/zhen_mcp_server.py`). That still stands. What it does not give is
the direction ADR-053 needs: Zhen (or a person at Zhen's CLI) handing a
HEAVY task to Claude with the Kingdom's tools attached, while MINOR churn
stays on local hardware.

The seam for this exists: `pkg/zhentools.Provider` (list tools, call one),
implemented by `pkg/mcpclient` for the MCP server and now by the Champion
itself. What was missing is a model client on the Claude side, and the
controls that make it safe to leave running: a tool gate, a spend cap,
and an audit trail.

## Decision

Build `pkg/claudebridge` and `cmd/zhen-claude`: a Claude API client that
runs the tool loop locally, with the Kingdom's tools, through the
Champion's gate.

### 1. Messages API through the official Go SDK

`github.com/anthropics/anthropic-sdk-go` (v1.82.0, admitted under ADR-096;
review in ADR-004's register). No raw HTTP. Requests use the beta
Messages endpoint because two beta features are on by default:

- **Model** `claude-opus-5-5`; **adaptive thinking** (it cannot be disabled
  on this model); **effort set explicitly** (`medium`, which is also this
  model's default, so a server-side default change cannot move the bill).
- **Streaming** with the SDK's `BetaMessage.Accumulate`; assistant text is
  printed as it arrives.
- **Refusal fallbacks**: `fallbacks: "default"` under
  `server-side-fallback-2026-07-01`. A classifier decline is re-run
  server-side on Anthropic's recommended model for that category.
  `-no-fallback` turns it off.
- **Prompt caching**: top-level automatic `cache_control`; the tool loop
  resends the same prefix every iteration.
- No sampling parameters, no prefill, no forced `tool_choice` (all 400 on
  this model).

### 2. The tool loop, and why it is a manual loop

The SDK's tool runner was considered and not used. The bridge needs to act
*between* the model's reply and the tool run on every iteration: reserve
spend before each request, check `stop_reason` before any tool runs, send
every call through a gate with a typed refusal for audit, and roll a failed
turn back. Per iteration:

1. Reserve the request's worst-case cost (§5); refuse before sending if it
   does not fit.
2. Stream the response; record its real cost from `usage`.
3. Branch on `stop_reason` before reading content:
   - `refusal` → typed `RefusalError` (category from `stop_details`),
     partial output discarded, no tools run;
   - `max_tokens` → error, no tools run (a cut-off `tool_use` input is not
     trusted);
   - `model_context_window_exceeded` → error;
   - `pause_turn` → append and resend;
   - `tool_use` → run every `tool_use` block **concurrently**, each through
     the gate, and return **all** results in **one** user message, in the
     order Claude asked. A refusal, a tool failure, an unknown tool or a
     non-object input is an `is_error` result, never a dropped one;
   - `end_turn` → done.
4. Bound the loop (`-max-iterations`, default 16).

History is append-only (`resp.ToParam()` appended verbatim, thinking
blocks included). On any error the turn's messages are removed from the
tail, which leaves every earlier prefix byte-identical, so Opus 5.5's
preserved-thinking check and the prompt cache both stay valid. The tool
list and system prompt are fixed for the life of a Bridge for the same
reason.

`eager_input_streaming` is left off: the bridge does not validate tool
inputs against their schemas client-side, so it keeps the API's buffered,
server-validated inputs.

### 3. Tools come from zhentools; every call goes through the Champion gate

Claude is a model, not a trusted caller of Kingdom tools. `Gate.Execute`
is the only path from a `tool_use` block to a Provider; the bridge never
calls a Provider directly.

`ChampionGate` applies the B2 gate (`champion.AcceptToolCall`):

- Rule 3: a destructive verb in any argument → refused outright.
- Rule 2: a mutating tool with no justification chain → refused **pending
  confirmation**. Claude's calls carry no retrieval-derived justification,
  so every mutating tool (and every tool the Champion does not know,
  fail-closed as mutating) is refused today. The refusal reaches Claude as
  an `is_error` result saying why; the system prompt tells it not to
  rephrase around one.
- Rule 1: a `path` argument outside the sandbox → refused.

For the Champion's own tools (`champion.ToolProvider`: `read_file`,
`write_file`, `patch_file`, the Kanban tools when a store is configured)
the gate runs `champion.Dispatch`, which gates and executes in one step.
For other providers it runs `AcceptToolCall`, then the provider.

**MCP tools default to a read-only allowlist.** `raft/zhen_mcp_server.py`
also offers `file_write`, `file_patch` and `runbook_execute` (which runs
scripts) with no gate of its own. `AllowTools(MCPReadOnlyTools...)`
(`corpus_search`, `file_read`, `service_health`, `runbook_list`,
`runbook_show`, `local_llm`) is applied unless `-mcp-mutating` is passed,
and even then each call still goes through the Champion gate (where Rule 2
refuses it). `file_read` was added to the Champion's read-only list for
this; Rule 1 still checks its path. `local_llm` (2026-10-23) was added
too: without it the gate would fail it closed as an unknown, mutating
tool. It asks the llama-server that is already running for one
completion and changes nothing on the host; loading a model stays behind
`model_switch` and `scripts/switch-model.sh`'s fit check. Rule 3 still
applies to its prompt, so a prompt quoting `rm -rf` is refused.

The out-of-band confirmation flow the Champion already has
(`IssuePendingConfirmation`) is not wired to the bridge yet; see open
questions.

### 4. Why not the Messages API "MCP connector"

The API can call a remote MCP server itself (`mcp_servers` with a URL,
beta `mcp-client-2025-11-20`). That needs Anthropic's servers to reach
the MCP server over the internet. West is not internet-facing, and
exposing Zhen's MCP server (which can write files and run runbooks) to the
internet to make that work is the wrong trade. It would also move the tool
calls outside the Champion gate. Instead the bridge runs the loop locally
and reaches the MCP server over stdio through `pkg/mcpclient`.

### 5. Cost controls

- **Per request:** `max_tokens` (default 16000; thinking counts toward it)
  and `effort`.
- **Per turn:** the iteration bound; tool results cut at 64 KiB before they
  go back to Claude as input tokens.
- **Task budget** (`-task-budget`, beta `task-budgets-2026-03-13`, minimum
  20000): supported, **off by default**. It is advisory (the model paces
  itself), not a cap; the skill's guidance is not to set one without a
  measured value.
- **Hard daily cap** (`-daily-cap-usd`, default $5, per UTC day): before a
  request is sent, its worst case is reserved: the input estimated at two
  bytes per token from the request body, at the price table's highest
  input or cache-write rate, plus `max_tokens` at the highest output rate,
  doubled when a fallback may run the request a second time. A request
  that does not fit is refused before anything is sent. After the
  response, the real cost is computed from `usage` (per iteration when
  `usage.iterations` is present, each at the rate of the model that ran
  it) and committed. The day's total is a JSON file
  (`$XDG_STATE_HOME/unheaded/zhen-claude/spend.json`, mode 0600, `flock`ed);
  a file that cannot be parsed refuses every request.
- **Price table**: `pkg/claudebridge/prices.json` (embedded; override with
  `-prices`), USD per MTok: Opus 5.5 $4 / $20, cache write $5 (5 m) / $8
  (1 h), cache read $0.20; Opus 5 and Opus 4.8 (possible fallback targets)
  $5 / $25. A model not in the table is priced at the table maximum.

Rough cost per call at the defaults (Opus 5.5): output up to 16000 tokens
is at most $0.32; a short prompt with the Champion's three tools is about
1–2 K input tokens, about $0.01. A typical answer of a few hundred tokens
costs about $0.01–0.02. The cap holds a request back when the day's spend
plus about $0.80 (the doubled worst case) would pass it.

### 6. Credentials

From the environment only: `ANTHROPIC_API_KEY`, or the SDK's chain
(`ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_PROFILE`, workload identity
federation, an `ant auth login` profile). Never in the repository. The
bridge resolves the source before building a request and **fails loud**
when there is none (`claudebridge.ErrNoCredentials`); logs name the source
(`env ANTHROPIC_API_KEY`), never the value. `-dry-run` needs none and
sends nothing.

### 7. Audit

Every request: requested and serving model, message id, `stop_reason`,
refusal category, input/output/cache tokens, reserved and actual cost, the
day's total, duration. Every tool call: tool, provider, gate decision and
reason, pending-confirmation flag, `is_error`, input size and SHA-256
(**not** the input: it can hold file bodies), output size, truncation.
No prompt or response text is logged. The CLI writes these as JSON lines
on stderr (`log/slog`); the `Auditor` interface lets a service route them
to Wotan or the Champion's ActionStore later.

### 8. Routing hook (ADR-053), interface only

`claudebridge.Router` classifies a task as `WeightHeavy` (Claude) or
`WeightMinor` (local Zhenai). No implementation ships with this ADR. A
router must not send to Claude data the operator has not cleared for an
external API: the zero-user-data-access rule applies to what leaves the
Kingdom as prompt text.

### 9. `-route auto`: local first, Claude on failure (2026-10-23)

The first, deliberately simple router lives in `cmd/zhen-claude`, not
behind `Router`. `-route claude` (the default) is unchanged. `-route auto`
asks the local model once through llama-server's OpenAI-compatible
`/v1/chat/completions` (`-local-url`, default `$ZHEN_LOCAL_LLM_URL` or
`http://127.0.0.1:8081`; `claudebridge.LocalLLM`), with a short system
prompt and no tools. It escalates to Claude when:

- the local call fails: nothing listening, timeout, non-200, malformed or
  empty answer (`why` `local_error`);
- the answer stopped at `max_tokens` (`local_truncated`);
- `-check` rejects it. `-check` is an argv command split on spaces and run
  without a shell, in the project root, with the answer on stdin and its
  own output discarded; exit 0 accepts (`check_passed`), any other exit
  escalates (`check_failed`), and a check that cannot run or times out
  (`-check-timeout`, default 60 s) escalates too (`check_error`). With no
  `-check`, a successful local answer is accepted (`local_ok_no_check`).

Each turn logs one `route_decision` line: local tried, model, latency,
answer size, error; check argv, exit and latency; escalated; why; the
decision (`local`, `claude`, `none`); Claude's cost and latency; total
latency. No prompt or answer text. The Claude side (credentials, MCP
server, ledger) is built only on escalation, so a local answer needs no
API key, and the spend cap only ever applies to the Claude leg. `-route
auto` takes one prompt; it refuses `-repl`, because the local model keeps
no conversation. A local answer is untrusted text either way: it is
printed, never executed, and `-check` is the operator's own command.

Keep-local marker (2026-10-23): `-local-only`, or `local_only = true` in
`~/.config/unheaded/zhen-claude.conf` (`$XDG_CONFIG_HOME`; `-config` names
another file; strict `key = value`, unknown keys and bad values refused;
a flag on the command line overrides the file). A local-only prompt is
asked of the local model only: a failed or rejected local answer exits 1
and is never escalated, `-route claude` with it is a usage error, and
`-dry-run` shows no Claude request. The `route_decision` line and the
capture record carry `local_only`. A `Router` that classifies by data, not
difficulty, is still open (question 6).

### 10. Training capture, opt-in (2026-10-23)

`-capture <dir>` (default `$ZHEN_CAPTURE_DIR`, so off unless set) appends
each turn to a `zhen-capture/v1` JSONL file through `pkg/zhencapture`:
prompt, system prompts, Claude's tool calls with inputs and (cut) outputs,
the local answer, the check and its exit, Claude's answer, the accepted
answer, cost and latency. Every string is redacted against
`pkg/zhencapture/redact_patterns.json` before it is written; the directory
must be outside any git work tree and private (0700, files 0600). Claude's
tool contents reach the collector through `Config.OnToolResult`, wired only
when capture is on; the Auditor still gets sizes and hashes only. Format:
`docs/architecture/zhen-capture.md`.

## Live smoke test (once a key exists)

```bash
export ANTHROPIC_API_KEY=...   # or: ant auth login
go run ./cmd/zhen-claude -tools none -effort low -max-tokens 1024 -daily-cap-usd 0.50 \
  -p "Reply with the single word OK."
```

Worst case about $0.02 on Opus 5.5 (1024 output tokens at $20/MTok plus a
few hundred input tokens), up to about $0.05 if a refusal fallback runs it
a second time; the cap reserves about $0.06 for it. Then, with
tools: `-tools champion -p "Read /path/inside/project/README.md and
summarise it in one line."`

## Consequences

**Good.** ADR-053's HEAVY route exists as code, with the Kingdom's tools,
without opening anything to the internet. Claude's tool calls are held to
the same gate as local Zhen's. Spend has a hard ceiling that survives
across CLI runs.

**Costs.** One more SDK in the root module (MIT, vendor-maintained; its 18
compiled-in modules were already in the build list through `tools/gosec`,
and it adds 22 `go.sum`-only identities for its own tests). Prompts sent
through the bridge leave the Kingdom.

**Limits.**
- The cap estimates input tokens from bytes; it is conservative, not
  exact. Two processes sharing one ledger see each other's committed spend
  but not each other's in-flight reservations.
- Until a confirmation flow exists, Claude can only read: every mutating
  call is refused.
- Not yet run against the live API (no credentials on west).

## Open questions

1. **Owner sign-off** for the SDK admission (ADR-004 register row), and
   whether the transitive surface is acceptable or a stdlib client (like
   `pkg/mcpclient`) is preferred later.
2. **Confirmation flow.** Wire `IssuePendingConfirmation` so a person can
   approve one of Claude's mutating calls out of band, or keep the bridge
   read-only.
3. **Default daily cap** ($5) and default `max_tokens` (16000).
4. **Task budget** default (off) once real turns are measured.
5. **Where audit goes** for a long-running service: Wotan topic, the
   Well's `actions` table, or both.
6. **Privacy marker for `-route auto`** (§9): the explicit marker
   (`-local-only` / `local_only`) shipped 2026-10-23; still open is
   whether the escalation path should refuse prompts that capture
   redaction would have touched.
7. **`-check` conventions**: whether to ship a few standard checks (Go
   snippet compiles, JSON parses, answer cites a file that exists), or
   leave it to the operator.
