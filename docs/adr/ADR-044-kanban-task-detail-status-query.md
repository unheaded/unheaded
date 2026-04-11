# ADR-044: Kanban Task Detail Status Query

## Status: PIPE DREAM (no scheduled work — captured for future consideration)

## Date: 2026-04-11

## Decision Maker
- Stevie Bellis (Principal)

---

## Context

The Kanban app currently shows tasks as cards with title, description, and
column state. Stevie wants a future enhancement where each task card exposes
a **status query** that surfaces context-rich detail without leaving the board:

1. **Git log** for the task — commits whose messages reference the task ID,
   branch name, or related file paths
2. **Linked docs** — wiki pages, ADRs, battle plans, runbooks tied to the task
3. **Work completion checklist** — a derived TODO/DONE breakdown showing what's
   shipped vs what's pending for the task

The goal is **single-pane-of-glass** task introspection: click a card → see
not just the description but the full real-world state of the work (commits,
files touched, docs updated, tests passing or failing, related tasks).

This is **not scheduled** and has no production timeline. It is captured here
so the idea isn't lost.

---

## Decision

**ACKNOWLEDGE as pipe dream.** No build work scheduled. ADR exists to:
1. Reserve the concept space (no one else proposes a conflicting design)
2. Anchor the idea to existing infrastructure (Kanban, Wotan, gitlog parsers)
3. Sketch a non-binding implementation direction so a future contributor
   (human or Zhen Champion) can pick it up

### Sketch (non-binding)

**Backend**: A new service or Kanban-app endpoint that, given a task ID:
1. Greps `git log --all --grep=<task-id>` for related commits
2. Parses commit metadata for changed files, authors, dates
3. Walks `docs/`, `wiki/`, `runbooks/` for files referencing the task ID
4. Reads task ACL/checklist from Kanban store
5. Returns a structured `TaskDetailReport`

**Frontend**: Kanban card click → modal with tabs:
- **Overview** (existing description)
- **Activity** (git log, ordered)
- **Docs** (linked wiki/ADR/runbook entries)
- **Progress** (checklist of work units, derived from commits + tests)

**Data sources**:
- Git: local repo or remote API
- Docs: filesystem grep + wiki link graph
- Tests: CI status, test coverage delta
- Wotan: optional — task lifecycle events on `kanban.task.*` topics

### Connection to Zhen Champion

Once Zhen Champion is operational ([[Wave 10C Backprop|Wave-10C-Backprop]]
training infrastructure → Kingdom-fluent Mistral LoRA), the Champion could
**generate** the status report on demand: "Tell me about task K-217" →
Champion runs the queries, summarizes, and posts the result. No new service
required — just MCP tool wiring.

This makes ADR-044 a **natural fit for Zhen as the implementer**: small,
read-only, well-bounded, doesn't touch the protocol freeze.

---

## Consequences

### Positive
- Single-pane task introspection eliminates context-switching
- Champion has a clear, achievable target
- Forces Kanban tasks to maintain ID convention so cross-references work
- Surfaces incomplete work (commits without docs, docs without tests, etc.)

### Negative
- Risk of stale derived data if not regenerated
- Depends on disciplined commit message references to task IDs
- Could become a maintenance burden if scope creeps into "task management"
  features that overlap with the existing `pkg/champion/` Kanban CRUD

### Mitigations
- **Pipe dream tier**: no commitment to ship, no roadmap entry
- If implemented, gate behind read-only flag — never mutate task state
- Use existing infrastructure (`pkg/champion/`, Wotan, git CLI) — no new deps

---

## Alternatives Considered

### Alternative A — Build it now
**Rejected**. Not on critical path. Mímir's Law spike, Wave 10C, and other
in-flight work take precedence. No customer asking for it.

### Alternative B — Ignore it (don't even file an ADR)
**Rejected**. Ideas that aren't written down get lost. The 30 seconds it
takes to file an ADR is cheap insurance against losing the idea.

### Alternative C — File as Kanban backlog ticket instead
**Rejected**. Kanban tickets churn; ADRs persist. This is an architectural
direction, not a unit of work.

---

## References

### Related ADRs
- **ADR-041** — Kanban Timeline Sync (existing task ↔ timeline integration)
- **ADR-024** — Zhen Runbook Automation (Champion's tool use foundation)
- **Wave 10C** — Backprop fix that unblocks Champion training

### Related Components
- `cmd/kanban-app/` — current Kanban service
- `pkg/champion/` — Zhen Champion sandboxed tool use harness
- `raft/zhen_mcp_server.py` — MCP tool exposure for Claude Code

---

*ADR-044 — filed as pipe dream 2026-04-11*
*"Capture the idea. Build it when the moment is right."*
