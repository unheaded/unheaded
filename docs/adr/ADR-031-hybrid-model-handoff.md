# ADR-031: Hybrid Model Architecture — Specialist + Generalist Handoff

**Status:** Planned
**Date:** 2026-04-03
**Deciders:** Scientist, Captain, Computermancer

## Context

Zhenai (local Mistral-7B + LoRA) is a specialist — great at Kingdom infrastructure but limited in general reasoning, coding, and brainstorming. Claude Opus is a generalist — powerful for planning, code generation, and creative work but doesn't know Kingdom internals without RAG context.

Neither alone is sufficient. The solution: a handoff architecture where each model handles what it's best at.

## Decision

### Three-Tier Model Architecture

```
Tier 1: Zhenai Specialist (local, always-on, free)
  - Fine-tuned Mistral-7B with kingdom.zlora
  - Handles: runbook execution, health checks, service ops, recall, scheduling
  - Strengths: Kingdom-specific knowledge, instant response, offline capable
  - Runs on: RX 7700 XT via llama-server

Tier 2: Zhenai Custom Model (local, future)
  - 1-3B params, custom architecture via zhenai-forge
  - Purpose-built for infrastructure operations
  - Trained on: Kingdom data + public infra datasets (eBPF docs, Go stdlib, Rust book, RFC corpus)
  - NOT a general LLM — a specialist that knows networking, eBPF, Go, Rust, and the Kingdom
  - Runs on: custom Rust inference engine (replace llama.cpp)

Tier 3: Claude Opus (remote, on-demand, paid)
  - General-purpose reasoning, code generation, architecture, brainstorming
  - Connected via MCP (ADR-019 Champion MCP server)
  - Hands off to Zhenai for Kingdom-specific execution
  - Hands off to user for approval decisions
```

### Handoff Protocol

```
User asks question via Kanban / Zhenai web UI / MCP
    │
    ▼
Zhenai Specialist attempts to answer
    │
    ├── Can handle? (runbooks, health, recall, Kingdom Q&A) → respond directly
    │
    └── Cannot handle? (coding, planning, complex reasoning) → escalate
          │
          ▼
        Claude Opus via MCP
          │
          ├── Claude generates plan/code/analysis
          │
          └── Hands back to Zhenai for execution
                │
                └── Zhenai executes runbooks, monitors, reports back
```

### Ring 0 Champion Work

Zhenai's "Ring 0" duties (highest priority, always-on):
1. Service health monitoring (consensus watchdog, ADR-029)
2. Scheduled runbook execution (ADR-028 scheduler)
3. Alert escalation (heartbeat → human notification)
4. Conversation memory (ADR-027 recall)
5. Trust-gated operations (ADR-019 approval queue)

These NEVER go to Claude — they're Zhenai's core competency, running locally, offline-capable.

### Evolution Path

1. **Now**: Mistral-7B + kingdom.zlora (done, 3.5 min training via zhenai-forge)
2. **Next**: Custom Rust inference engine (replace llama.cpp with zhenai-forge inference)
3. **Eventually**: Custom 1-3B specialist model trained on curated infra data
   - Architecture: transformer decoder-only, 1.5B params
   - Training corpus: Kingdom codebase + Go stdlib + Rust book + 9,739 RFCs + eBPF docs
   - NOT competing with Mistral on general knowledge
   - BEATING it on: "What port does Wotan use?" "Debug this BPF verifier rejection" "Generate nginx config for service X"

## Consequences

### Positive
- Each model handles what it's best at — no compromise
- Zhenai operates 24/7 without internet or API costs
- Claude's power available when needed for complex work
- Custom model is Kingdom IP — can't be replicated by competitors

### Negative
- MCP handoff adds latency for Tier 3 calls
- Custom model training is significant R&D effort
- Two inference engines to maintain (llama.cpp now, custom Rust later)
