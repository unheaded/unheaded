# Zhen AI

Zhen is Unheaded's AI-assisted development system. It combines retrieval-augmented generation (RAG) with fine-tuned language models to provide codebase-aware assistance for development, documentation, and operations.

## Architecture

```
┌─────────────────────────────────────────┐
│              Zhen Pipeline              │
├─────────────┬───────────┬───────────────┤
│  Embedding  │   FAISS   │  llama.cpp    │
│  Pipeline   │   Index   │  (selector)   │
├─────────────┼───────────┼───────────────┤
│  Codebase   │  ~1.5M    │  Gemma-4 LoRA │
│  Ingestion  │  Vectors  │  (forge)      │
└─────────────┴───────────┴───────────────┘
```

## RAG Pipeline

- **Embedding model:** Sentence transformers for code and documentation
- **Vector store:** FAISS index, ~1.5M vectors over the Unheaded codebase (2026-03 count)
- **Chunk strategy:** AST-aware splitting for Go and Rust, section-aware for Markdown
- **Retrieval:** Top-k similarity search with reranking

## Models and training

- **Inference:** llama.cpp `llama-server` with a model chosen from the UI's selector (ADR-060). Keys come from `scripts/switch-model.sh`: `qwen-7b`, `deepseek`, `deepseek-cpu`, `qwen-coder-14b`. A swap goes through a Champion-gated `model_switch`.
- **Training (zhenai-forge):** Rust LoRA trainer with GPU kernels, on **Gemma 4 E2B** (fits west's VRAM). Mistral-7B was the original target; on the 14 GB APU it never learned (Wave 10D).
- **Results so far:** WAVE12 Kingdom RAFT LoRA cut held-out eval loss from 21.10 to 6.78 (ADR-050). WAVE13 Phase 2's quality verdict was **RETRAIN**: 0 of 8 prompts better than base (ADR-051). The WAVE14 retrain (≥ 3 epochs) is still pending.

## Components

| Piece | Where |
|---|---|
| Web UI + RAG (Flask) | `raft/zhen_app.py` (port 20103) |
| Champion (gated actions: kanban, files, runbooks, model switch) | `pkg/champion/` (ADR-019) |
| Agent daemon | `cmd/zhen-agentd` (actions + kanban in The Well as `app_zhen`) |
| CLI | `cmd/zhen-cli` (ADR-059) |
| MCP server for Claude Code | `raft/zhen_mcp_server.py` |
| Forge (training) | `crates/zhenai-forge/` (ADR-048 backend trait) |

**On west today (2026-10-15):** none of these is running. Zhen is started on demand.

## Vision

Zhen is designed to progressively build itself and Unheaded:

- **Self-improvement:** Zhen builds its own pipeline improvements
- **Codebase generation:** Zhen generates and validates Unheaded code
- **Frontier delegation:** Opus handles tasks beyond Zhen's current capability
- **Autonomous loops:** Inspired by Karpathy's autoresearch pattern -- constrained experiment loops

## Integration

- **20 skills** leverage Zhen's RAG pipeline for context-aware responses
- **Session handoffs** are indexed for continuity across development sessions
- **Protocol awareness** is embedded in all skill interactions

---

*Last updated: 2026-10-15*

---

> **Source:** [ADR-017](../docs/adr/ADR-017-zhen-hybrid-inference.md) · [ADR-060](../docs/adr/ADR-060-zhenai-multi-model-selector.md) · [docs/architecture/ZHEN_ARCHITECTURE.md](../docs/architecture/ZHEN_ARCHITECTURE.md)
