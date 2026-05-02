# zhen-agent

The Phase D-A agent runtime. A multi-turn ReAct loop that wraps:

- **`cs/vor`** — retrieval (cs cheatsheets + Unheaded markdown via symlinked sources)
- **`llama-server`** — LLM (Qwen2.5-Coder-7B-Instruct via llama.cpp)
- **`pkg/champion`** — sandboxed tool execution (read/write/patch files, kanban CRUD), gated by source-trust + destructive-verb defenses

The agent answers questions, calls tools when needed (with snapshots and revert), and refuses tool calls that depend on user-symlinked external content unless the user confirms out-of-band.

## Quickstart

```bash
# 1. Build
make build-zhen-agent

# 2. Make sure vor and llama-server are running
#    (see scripts/start-cs.sh, scripts/start-llama.sh, or `make zhen-agent-up`
#     once that target lands)
curl -sf http://127.0.0.1:9876/api/health     # vor
curl -sf http://127.0.0.1:8081/health         # llama-server

# 3. Ask
bin/zhen-agent -q "What is the right tag for a clickable button?"
bin/zhen-agent -q "Read pkg/champion/champion.go and tell me what Trust Level 2 implements"

# 4. Turn on the trace to see what's happening per turn
bin/zhen-agent -show-trace -q "Read CONTRIBUTING.md and summarize the sanitization convention"
```

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-q <text>` | (stdin) | the user goal |
| `-k <int>` | 5 | top-K topics for seed retrieval |
| `-max-tokens <int>` | 600 | LLM max_tokens per turn |
| `-max-turns <int>` | 8 | agent loop budget |
| `-max-topic-chars <int>` | 10000 | per-topic content cap (prompt budget guard) |
| `-temperature <float>` | 0.4 | LLM sampling temperature |
| `-seed <int>` | 0 | LLM seed; nonzero pins sampling for reproducibility |
| `-project-root <path>` | cwd | Champion sandbox root (file ops gated to this dir) |
| `-show-trace` | false | per-turn trace to stderr |

Env: `VOR_URL` (default `http://127.0.0.1:9876`), `LLAMA_URL` (default `http://127.0.0.1:8081`), `RAG_MODEL` (default `qwen2.5-coder-7b-instruct`).

## What it can do

- Answer pure-knowledge coding questions (no tools needed; ~2-9s per response).
- Read files inside `-project-root`.
- Write or patch files inside `-project-root` (snapshotted; revertable via Champion's `RevertAction`).
- Create / update / list kanban tasks (when `KanbanStore` is wired up).
- Cite Unheaded docs and runbooks via cs/vor retrieval.

## What it refuses

The Champion gate refuses tool calls in three cases. Refusals show up in the agent trace and the model's next-turn answer.

1. **Path outside the sandbox** — `write_file({path: "/etc/passwd"})` → hard deny.
2. **Destructive shell verbs in args** — anything matching `rm -rf`, `drop table`, `mkfs.*`, `dd if=`, `wipe`, `shutdown`, `reboot`, `kill -9`, `truncate`, `unlink`, `git push --force`, `git reset --hard`, `chmod 000`, `> /dev/sd*` → hard deny. Word-boundary regex; recurses into nested args.
3. **Mutating tool with untrusted justification** — when the model's reasoning relies on content from a user-symlinked source under `~/.config/cs/sources/`, OR when the justification chain is empty for a mutating call → deny pending out-of-band user confirmation.

For (3), the agent surfaces a single-use confirmation token (5-minute TTL). Redeem it via `Champion.ConfirmPendingToolCall` to re-run the call with Rule 2 suppressed (Rules 1 and 3 still enforced — the user can authorize an external source, they cannot authorize destruction).

## How the agent decides what's "untrusted"

- **`embedded` sources** (`go:embed sheets/`, `go:embed detail/`) — canonical, trusted.
- **`user-custom`** (`~/.config/cs/sheets/`) — local, trusted.
- **`user-source`** (`~/.config/cs/sources/<name>/...`) — external, untrusted by default.

The model sees these as `[canonical]`, `[local]`, `[external]` prefixes on each retrieved reference. The agent re-runs retrieval per tool call (using identifier tokens extracted from the model's thought + tool args) so the gate sees what the model is actually relying on for that specific call, not just the seed retrieval.

## Audit log

Every gate decision is logged via `champion.ActionStore`. The CLI uses a stderr-only stub (`stderrActionStore`) by default — you'll see lines like:

```
[champion] log #1: tool_call_attempt — ToolCall: write_file
[champion] log #1: accepted
[champion] log #2: file.write — Write file: foo.go (243 bytes)
[champion] log #2: completed
```

For production, swap in the PostgreSQL `ActionStore` from The Well (`pkg/wotan` integration; not yet wired into this CLI — that's deployment work).

## Troubleshooting

**`zhen-agent: vor search: ... missing q parameter`**
Vor rejected an empty query. Usually means `searchQuery()` stripped a query down to nothing (queries ending in only punctuation). Re-phrase the goal.

**`zhen-agent: llama chat: status 400: ... exceeds the available context size`**
The retrieved references blew the prompt budget. Drop `-k` (fewer topics) or `-max-topic-chars` (smaller per-topic cap). llama-server's `--ctx-size 16384` is the upstream limit.

**Agent budget-hit (`(agent ran out of turns without producing a final answer)`)**
Model is in a tool-call loop. Check `-show-trace`. Increase `-max-turns` only if the loop is making progress; otherwise re-phrase the goal.

**Tool refused with `denied_destructive`**
Args contained one of the destructive shell verbs. Re-phrase the request to avoid the literal verb (e.g., "delete this file" → use the `delete_file` tool with a path arg instead of `write_file` with `rm` in the content).

**Tool refused with `denied_untrusted_justification`**
The model's reasoning either named a user-symlinked source by identifier or had no retrieval-derived rationale at all. Either:
- Confirm out-of-band via `Champion.ConfirmPendingToolCall(token)`, OR
- Re-phrase the goal so retrieval surfaces canonical references, OR
- Programmatic invocations should use `champion.Dispatch` directly with a `Reference{SourceTrust: "direct-user"}` justification.

## Design references

- **Threat model & landing arc:** `eval/coding-gate/probe-2026-05-02/SESSION-SUMMARY.md`
- **B1 source provenance:** `eval/coding-gate/probe-2026-05-02/B1-design-source-provenance.md`
- **B2 tool-call gate:** `eval/coding-gate/probe-2026-05-02/B2-design-champion-tool-call-gate.md`
- **Bypass found + fixed:** `eval/coding-gate/probe-2026-05-02/A2-agent-adversarial.md`
- **Source-poisoning probe:** `eval/coding-gate/probe-2026-05-02/A1-source-poison.md`
- **Verdict on the 14-prompt gate (zhen-rag):** `eval/coding-gate/probe-2026-05-02/D-pre-verify-results.md` (H1)
- **Verdict on the 14-prompt gate (zhen-agent):** `eval/coding-gate/probe-2026-05-02/A1-agent-gate-results.md` (H1, parity)

## Code map

```
cmd/zhen-agent/main.go      — CLI: flags, vorRetriever, llamaLLM, stderrActionStore
pkg/agent/agent.go          — ReAct loop, modelOutput parser, system prompt builder,
                                deriveJustification (per-turn retrieval), token extractor
pkg/agent/agent_test.go     — happy paths, refusal paths, JSON edge cases, per-turn tests
pkg/champion/toolcall.go    — ToolCall envelope, Reference, AcceptToolCall (3 rules),
                                empty-justification fail-closed
pkg/champion/dispatch.go    — Dispatch router (gate → underlying typed methods)
pkg/champion/confirm.go     — Pending-confirmation token store (single-use, 5-min TTL)
pkg/champion/champion.go    — Champion struct, validatePath (Rule 1), action logging
```

## Testing

```bash
# Unit tests (race-clean, ~40 tests)
go test ./pkg/champion/ ./pkg/agent/ -count=1 -race

# 14-prompt coding-gate eval against zhen-agent (real LLM)
bash scripts/coding-gate-agent.sh  # writes to eval/coding-gate/probe-*/

# Adversarial smoke (manual — see A2-agent-adversarial.md for setup)
```
