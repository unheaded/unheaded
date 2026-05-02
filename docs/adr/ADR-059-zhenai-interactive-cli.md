# ADR-059 — Zhenai Interactive CLI (Terminal Counterpart of the Web UI)

**Status:** Planned
**Date:** 2026-05-02
**Deciders:** Stevie Bellis + unheaded-developer (when activated)
**Context owner:** zhenai operator surface
**Triggered by:** Stevie's directive 2026-05-02: *"we should also create a cli counterpart that can chat and interact with zhenai from terminal prompt too sort of like a simplified version of what we are doing now."*

---

## Context

Today's operator surfaces for Zhenai:

- **Web UI** (`raft/zhen_app.py:20103`) — full-featured browser chat, memory, runbook execution, source viewer, conversations history, skills browser. Stays in a tab.
- **`bin/zhen-rag`** (`cmd/zhen-rag/main.go`) — one-shot CLI: `zhen-rag -q "..."` → answer to stdout → exit. Stateless. Good for scripting; not interactive.
- **`bin/zhen-agent`** (`cmd/zhen-agent/main.go`) — single ReAct loop CLI. One goal in, multi-turn agent loop, exit. Also stateless across invocations.
- **`bin/zhen-agentd`** (`cmd/zhen-agentd/main.go`) — long-running HTTP daemon, no terminal interface.

What's **missing** is a terminal REPL — an interactive shell-like experience where the operator runs `zhen` (or `zhenai`), gets a prompt, and chats / queries / acts continuously without re-launching the binary per turn. Conversation history persists within the session. The operator stays in the terminal flow they're already in (no browser tab context-switch).

Stevie's framing: *"sort of like a simplified version of what we are doing now"* — the web UI's capabilities (chat, memory recall, runbook execution, file inspection) but in a terminal-first form.

---

## Decision

**Build `cmd/zhen-cli` — a Go-native interactive REPL that talks to the same backends the web UI hits (vor, llama-server, zhen-agentd, The Well).** Not a thin wrapper around `zhen-rag`; a real interactive client.

This ADR is **Planned** — full design lands in a follow-on commit when activation is triggered (Stevie schedules ~1-2 days of focused work). Recording the requirement here so it doesn't get lost.

### Required capabilities

The CLI should support, at minimum:

1. **Chat** with the same RAG path as the web UI (vor retrieval + qwen-coder inference). Each turn shows retrieved sources alongside the answer.
2. **Conversation history within the session** (the REPL maintains a turn log; same shape as the web UI's `_get_history`).
3. **Memory recall as a sidecar** — same display-only T1 semantics. When a cached memory matches, the CLI prints a small notice ("matched memory: similarity 0.94") alongside the live LLM answer.
4. **Slash commands** for system interaction:
   - `/runbook <name>` — execute a runbook through `zhen-agentd /api/v1/tool/exec` (same Champion-gated path the web UI uses)
   - `/runbook list` — list available runbooks
   - `/source <topic>` — fetch a vor topic body
   - `/recall <substring>` — full-text search prior conversations
   - `/remember` (after a Q&A) — save the current turn to `zhen_memories`
   - `/health` — check all backends (llama-server, vor, daemon, well) at once
   - `/exit` — clean shutdown, optionally persist the session log to PG

5. **Pipe-friendly mode** — `echo "what is unheaded?" | zhen` answers and exits, like `zhen-rag` does today. The interactive REPL is the default when stdin is a tty.

6. **Session persistence** — by default conversations write to `zhen_conversations` (same table the web UI uses). `--ephemeral` flag to opt out (for CI / scripted use).

### Architecture: shared HTTP client, no daemon dependency at minimum

The CLI talks to:

- **`zhen-agentd`** (preferred, when running): all chat goes through `/api/v1/agent/ask`; mutations through `/api/v1/tool/exec`. Champion's three-rule gate is in the path automatically. **T6b closure inherited.**
- **Direct backends** (fallback): when the daemon isn't up, the CLI can hit `vor :9876` and `llama-server :8081` directly using the same shapes `cmd/zhen-rag` already uses. Mutation slash-commands are **disabled** in this mode (no Champion gate to enforce them safely) — the CLI shows a clear "daemon down — read-only mode" banner.

This split mirrors the web UI's degrade-gracefully posture and keeps the CLI useful in any backend state.

### Build target

`cmd/zhen-cli/main.go`. Single Go binary. Stdlib + `pkg/agent` types for shared shapes. ~500-800 LOC depending on slash-command surface.

Optional dependencies:
- `golang.org/x/term` for raw-mode terminal handling (line editing, history scrolling). Already a `go list` away; allowed under the dependency policy.
- A line-editor library (`github.com/chzyer/readline` or `github.com/peterh/liner`) — preferred over rolling our own. To be decided at activation.

### Out of scope for this ADR

- TUI (full-screen terminal UI with panes — like `htop` or `lazygit`). The CLI is a line-by-line REPL, not a TUI. A separate ADR can address full TUI later if there's demand.
- Multiplexed sessions / split-pane chat. Single conversation per CLI invocation.
- Voice input or any non-text modality.
- Plugins / scripting language. Slash commands are hardcoded; if more flexibility is needed, that's a future ADR.

---

## Consequences

### Positive

- **Operator stays in terminal flow.** No browser context-switch for routine Zhen-mediated work — query the kingdom, execute a runbook, recall a prior decision.
- **Same security posture as the web UI.** Mutations go through the same `/api/v1/tool/exec` endpoint; T6b closure is inherited. No new gate work.
- **Same persistence as the web UI.** Conversations write to the same PG tables; recall and search work across both surfaces.
- **Pipe-friendly mode preserves scripting use.** Anyone using `bin/zhen-rag` today gets the upgrade for free if they swap the binary name; same one-shot semantics.
- **Honest degraded-mode** when the daemon isn't up: read-only chat works, mutations are visibly disabled. No silent T6 reopening.

### Negative / costs

- **Yet another operator binary.** `zhen-rag`, `zhen-agent`, `zhen-agentd`, and now `zhen-cli`. Documentation discipline gets harder. The CLI is the most operator-facing of them; the others can be marked as building blocks.
- **Line-editor dependency.** Shipping a polished REPL means picking a line-editor library; that's a real-but-bounded dep decision. Already-shipped Go services depend on `golang.org/x/time/rate`, `lib/pq`, etc — adding `chzyer/readline` is in the same posture.
- **Slash-command parser + tab completion.** Non-trivial but bounded; ~150 LOC.
- **Conversation-history table schema is shared with the web UI** — CLI sessions intermix with web UI sessions in `zhen_conversations`. Add a `client` column (`web` / `cli`) so analytics can distinguish. Migration `011_zhen_conversations_client_column.sql` (additive, idempotent).

### Mitigations

- Gate the line-editor dep on whether the operator is in interactive mode; pipe-mode (stdin-not-tty) skips loading it.
- Write the slash-command parser as a small registry (map of `name → handler`) so adding new commands is one entry, not a parser rewrite.

---

## Open questions (resolved at activation time)

1. **Single binary or build flag?** Should `zhen-cli` be a separate `cmd/zhen-cli/` or a `--repl` flag on the existing `cmd/zhen-rag`? Lean: separate binary. The codepaths diverge enough (interactive line editing, slash commands, session state) that a flag would muddy `zhen-rag`'s simple one-shot contract.
2. **Tab completion for slash commands.** Worth it or not? Lean yes — modern terminal UX expectations include it; readline gives it cheaply.
3. **Memory recall display: inline or sidecar?** The web UI shows it as a sidecar. In a terminal, "sidecar" doesn't fit; lean toward a small `[memory hit: similarity 0.94]` line PRINTED above the live LLM answer. Operator can opt out with `--no-memory-recall`.
4. **Session log on exit.** By default, dump the session to `zhen_conversations`. Confirm with `--no-persist` flag for one-off questions the operator doesn't want logged.
5. **Authentication.** When `zhen-agentd` runs with `AUTH_ENABLED=true`, the CLI needs to pass an API key or JWT. `~/.config/zhenai/credentials` file? `ZHEN_API_KEY` env var? Lean env var (simpler), file (more discoverable). Both would work.

---

## Implementation outline (when activated)

1. **Phase 1 — minimum viable REPL** (~half day): basic readline loop, `/exit`, single `chat` action that hits `zhen-agentd /api/v1/agent/ask`, prints answer + retrieved sources. No memory, no slash commands beyond exit. **Acceptance:** Stevie can have a coherent conversation in his terminal.

2. **Phase 2 — slash commands + persistence** (~half day): `/runbook`, `/source`, `/recall`, `/remember`, `/health`, `/exit`. Conversation persistence to `zhen_conversations` (or a tagged-as-cli rows). **Acceptance:** every operator action available in the web UI is also available in the CLI, with the same Champion gating where applicable.

3. **Phase 3 — polish** (~half day): tab completion, history scrollback, color output, pipe-mode, fallback (daemon-down read-only). **Acceptance:** CLI passes the same H0 coding-gate run as the web UI, end-to-end through `zhen-agentd`.

4. **Phase 4 — runbook authoring** (optional, deferred): the CLI could grow a `/runbook new <name>` command that scaffolds a YAML and opens it in `$EDITOR`. Out of scope for the initial ship; future ADR.

---

## References

- Web UI counterpart: `raft/zhen_app.py` (the surface this CLI mirrors)
- Existing one-shot CLI: `cmd/zhen-rag/main.go` (the lineage; the new CLI is the interactive sibling)
- Champion-gated dispatch endpoint the CLI's mutation slash-commands hit: `cmd/zhen-agentd/toolexec.go` (`/api/v1/tool/exec`, ADR-058's pattern)
- Application threat model — particularly T6 ("UI bypasses Champion"): the CLI's mutation paths inherit T6b closure by routing through the same endpoint. `docs/security/application-threat-model.md`
- Conversation persistence: `db/migrations/010_zhen_conversations.sql`
- Stevie's directive: 2026-05-02, captured verbatim in the "Triggered by" header.
