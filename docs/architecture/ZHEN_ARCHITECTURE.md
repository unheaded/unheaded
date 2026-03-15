# Zhen AI Champion — Architecture

*SPDX-License-Identifier: GPL-3.0-or-later*

**Date:** 2026-03-15
**Status:** Operational (594K indexed chunks, inference + RAG + UI live)

---

## Overview

Zhen is the Kingdom's local AI champion -- a RAG (Retrieval-Augmented Generation)
system that provides context-aware question answering, semantic search, and agent
context injection for Claude Code agents working on Unheaded.

The name comes from Chinese: "Zhen" -- meaning "true love" for the codebase.

---

## Components

```
┌──────────────────────────────────────────────────────────────┐
│  Zhen Web UI (Flask)         http://localhost:20103           │
│  zhen_app.py — routes, PostgreSQL logging, session mgmt      │
├──────────────────────────────────────────────────────────────┤
│  RAG Pipeline (Python)       raft/scripts/zhen_rag.py        │
│  FAISS index (594K vectors) + corpus lookup + prompt build   │
├──────────────────────────────────────────────────────────────┤
│  Inference Server            http://localhost:20100           │
│  llama.cpp (ROCm) — Mistral 7B Instruct Q5_K_M, 40 layers   │
├──────────────────────────────────────────────────────────────┤
│  The Well (PostgreSQL)       localhost:5432                   │
│  zhen_conversations table — session logging, feedback         │
└──────────────────────────────────────────────────────────────┘
```

### Ports

| Component | Port | Protocol |
|-----------|------|----------|
| llama-server (inference) | 20100 | HTTP (OpenAI-compatible `/v1/` API) |
| Zhen Web UI + RAG API | 20103 | HTTP (Flask) |
| PostgreSQL (The Well) | 5432 | PostgreSQL wire protocol |

### Technology Stack

| Layer | Technology | Notes |
|-------|-----------|-------|
| Inference | llama.cpp with ROCm | GPU-accelerated, 40 layers offloaded |
| Model | Mistral 7B Instruct Q5_K_M | GGUF quantized, 2048 context |
| Embeddings | sentence-transformers (all-MiniLM-L6-v2) | 384-dim vectors |
| Vector Index | FAISS (IndexFlatIP) | 594K vectors, inner product search |
| Web Framework | Flask + flask-cors | Python 3, dev server |
| Persistence | PostgreSQL 16 (The Well) | Optional, graceful degradation |

---

## Corpus Rings

Zhen's knowledge base is organized in concentric rings of relevance:

| Ring | Content | Source | Files |
|------|---------|--------|-------|
| Ring 1 | Unheaded codebase (385K LOC), Kingdom skills (18+) | Local repo, skill files | `ring1.jsonl`, `ring1_skills.jsonl` |
| Ring 2 | 16 GitHub repos (k8s, prometheus, tokio, etc.) | Cloned + chunked | `ring234.jsonl` |
| Ring 3 | 9,739 IETF RFCs | Bulk download + parse | `ring234.jsonl` |
| Ring 4 | 1,649 research papers, Stack Exchange (SO, ServerFault, Unix SE), Wikipedia | Various | `stackoverflow.jsonl`, `serverfault.jsonl`, `unix_se.jsonl`, `wikipedia.jsonl` |

**Total indexed:** 594K chunks across all rings.

### Corpus Processing Pipeline

Scripts in `raft/scripts/` (numbered for execution order):

1. `01_prepare_corpus.py` -- chunk codebase into JSONL
2. `02_embeddings.py` -- generate FAISS embeddings
3. `05_generate_qa.py` -- generate QA pairs for RAFT training
4. `06_expand_corpus.py` -- add Ring 2-4 sources
5. `06b_embed_chunked.py` -- embed expanded corpus
6. `07_prepare_training.py` -- prepare RAFT dataset
7. `08_train_qlora.py` -- QLoRA fine-tuning (scaffold)
8. `10_hotswap_index.py` -- live index replacement
9. `11_generate_qa_ring234.py` -- Ring 2-4 QA generation
10. `12_process_stackoverflow.py` -- Stack Exchange processing
11. `13_ingest_skills.py` -- Kingdom skill ingestion
12. `14_extract_wikipedia.py` -- Wikipedia extraction

### Embedding Model

- **Model:** `all-MiniLM-L6-v2` (sentence-transformers)
- **Dimensions:** 384
- **Index type:** FAISS IndexFlatIP (inner product / cosine similarity)
- **Index location:** `raft/index/`

---

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/query` | RAG question answering (retrieval + generation) |
| POST | `/api/v1/search` | Semantic search (retrieval only, no generation) |
| POST | `/api/v1/context` | Context retrieval for Claude Code agents |
| GET | `/api/v1/skills` | List all Kingdom skills |
| GET | `/api/v1/skill/<name>` | Get specific skill content |
| GET | `/api/v1/stats` | Index statistics |
| GET | `/api/v1/corpus/stats` | Corpus breakdown by ring |
| GET | `/health` | Health check |

### Agent Context API

Claude Code agents query Zhen before starting tasks:

```bash
curl -s http://localhost:20103/api/v1/context \
  -H "Content-Type: application/json" \
  -d '{"task": "describe your task here", "k": 10}'
```

Returns the `k` most relevant code/doc chunks for the given task description.

---

## Training Pipeline (RAFT)

RAFT (Retrieval-Augmented Fine-Tuning) is the planned training approach:

1. Generate QA pairs from corpus chunks
2. Use retrieved context as training signal
3. QLoRA fine-tune on Mistral 7B
4. Merge adapters + quantize to GGUF
5. Hot-swap into running llama-server

**Status:** Scaffold complete. QA generation operational. QLoRA training
script ready but not yet executed (awaiting sufficient QA pairs).

**Training data:** `raft/raft_dataset.jsonl`, `raft/raft_dataset_ring234.jsonl`

---

## Startup

```bash
# Full startup (both inference + UI):
./raft/start-zhen.sh

# Or manually:
# 1. Start inference
cd ~/tmp/unheaded/llama.cpp/build
./bin/llama-server -m ~/tmp/unheaded/raft/models/mistral-7b-instruct-q5_k_m.gguf \
  -ngl 40 -c 2048 --port 20100

# 2. Start Web UI + RAG
source ~/.venv/zhen/bin/activate
cd ~/tmp/unheaded/raft
python3 zhen_app.py
```

---

## Claude Code Bridge

The `raft/CLAUDE.md` file instructs Claude Code agents to query Zhen for
context before working on tasks. The bridge skill (`raft/scripts/13_ingest_skills.py`)
indexes Kingdom skill files so agents can retrieve domain-specific knowledge.

---

*See also: `raft/CLAUDE.md` (bridge API docs), `docs/architecture/AI_STACK.md` (broader AI strategy)*
