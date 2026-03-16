# ADR-017: Zhen Hybrid Inference — Context Window Optimization + Claude API Handoff (Deferred)

**Status:** Partially Accepted (local optimization accepted, Claude API deferred)
**Date:** 2026-03-16
**Deciders:** Developer, Scientist, BlackMage

## Context

Zhen's local Mistral-7B inference (llama-server on AMD RX 7700 XT) uses a 2048-token context window. When RAG retrieval returns rich context (multiple chunks + system prompt), prompts exceed this limit and return 400 errors from llama-server. This truncates knowledge and degrades answer quality.

Two strategies were evaluated:
1. **Maximize local context window** — Mistral-7B-Instruct supports up to 32K tokens natively (sliding window attention). The current `-c 2048` is conservative.
2. **Claude API handoff** — Route overflow prompts to Anthropic Claude API for full-context answers.

## Decision

### Accepted: Local Context Window Optimization

Run a scientist experiment (`raft/scripts/19_context_benchmark.py`) to find the optimal `-c` value for WEST's hardware. Test values: 2048, 4096, 8192, 16384, 32768.

**Measurements:** tokens/sec, VRAM usage, first-token latency, response quality across short/medium/long prompts.

**Goal:** Find the knee point where context size can grow without unacceptable speed degradation (>20% slower than baseline).

**Results (2026-03-16):**

| `-c` | Avg tok/s | VRAM | Headroom | Verdict |
|------|-----------|------|----------|---------|
| 2048 | 58.3 | ~5.4 GB | 6.9 GB | Baseline |
| 4096 | 59.7 | 5.9 GB | 6.4 GB | No degradation |
| 8192 | 59.4 | 6.4 GB | 5.9 GB | No degradation |
| **16384** | **59.5** | **7.4 GB** | **4.8 GB** | **Chosen default** |
| 32768 | 59.7 | 9.5 GB | 2.8 GB | Works, tight headroom |

**Finding:** Zero speed degradation across all tested context sizes. KV cache scales at ~125 MB per 1K context tokens, but generation throughput is constant at ~60 tok/s. The `-c 2048` default was leaving 10 GB of VRAM unused.

**Decision:** Default to `-c 16384` (8x improvement). Override via `ZHEN_CONTEXT_SIZE=32768` for single-user max context.

**Implementation:**
- `ZHEN_CONTEXT_SIZE` env var in `start-zhen.sh` (default 16384)
- `ZHEN_LOCAL_MAX_TOKENS` passed to RAG pipeline for smart truncation
- Context-aware chunk sizing: more chunks + longer excerpts when window allows
- Graceful truncation when prompt approaches limit (85% threshold)

### Deferred: Claude API Handoff

**Reason:** API billing is separate from claude.ai subscription. Will revisit when budget allows.

**Architecture (when ready):**
- API key stored in `~/.config/zhen/secrets.env` (never in repo tree — BlackMage requirement)
- `generate_claude()` method in `zhen_rag.py` using `anthropic` Python SDK
- Auto-routing: prompts exceeding local capacity route to Claude
- Model selector in UI: Auto / Mistral / Opus / Sonnet / Haiku
- Fallback: Claude failure → truncate and use Mistral
- Model badge on UI responses showing which model answered

**Key Design Decisions (preserved for future):**
- System prompt adapts per backend (no `[INST]` tags for Claude)
- Claude gets full context (200K window — no truncation)
- Token estimation: `len(text) // 4 + 50` (rough but sufficient for routing)
- Claude model IDs: `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5-20251001`

### Security (BlackMage)

- **No secrets in repo tree.** Not in `.env`, not in config files, not anywhere under version control.
- API keys go in `~/.config/zhen/secrets.env` with `chmod 600`.
- `anthropic` SDK already installed in venv (v0.84.0) — no additional setup needed when key is available.

## Implementation (Current Sprint)

| File | Status | Change |
|------|--------|--------|
| `raft/scripts/zhen_rag.py` | Done | Context-aware chunk sizing, smart truncation, file upload support |
| `raft/zhen_app.py` | Done | File upload, memory system (remember/forget), teach endpoint |
| `raft/static/index.html` | Done | File drag-and-drop, remember button, download buttons |
| `raft/start-zhen.sh` | Done | `ZHEN_CONTEXT_SIZE` env var, `ZHEN_LOCAL_MAX_TOKENS` |
| `raft/scripts/19_context_benchmark.py` | Done | Scientist experiment script |
| `db/migrations/008_zhen_memories.sql` | Done | Memory table for cached answers |

## Consequences

### Positive
- Local-first approach: zero external dependencies, zero cost, zero latency to API
- Experiment-driven: data determines optimal config, not guesswork
- Clean handoff path: when Claude API budget available, architecture is pre-designed
- Secrets never touch the repo

### Negative
- Very long context queries will still be truncated until Claude handoff is enabled
- No frontier-model reasoning for complex questions (local Mistral only)

### Risks
- Larger context windows increase VRAM usage — may compete with model layers on 12GB GPU
- Sliding window attention quality may degrade at very large context sizes
- Benchmark results are hardware-specific (WEST only — EAST has different specs)

## References

- Mistral-7B-Instruct sliding window: 4096 base, extended to 32K via RoPE
- llama.cpp `-c` flag documentation
- ADR-016: The Well (PostgreSQL persistence for memories)
