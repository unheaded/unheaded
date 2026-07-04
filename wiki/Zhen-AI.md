# Zhen AI

Zhen is Unheaded's AI-assisted development system. It combines retrieval-augmented generation (RAG) with fine-tuned language models to provide codebase-aware assistance for development, documentation, and operations.

## Architecture

```
┌─────────────────────────────────────────┐
│              Zhen Pipeline              │
├─────────────┬───────────┬───────────────┤
│  Embedding  │   FAISS   │   Mistral-7B  │
│  Pipeline   │   Index   │   (RAFT)      │
├─────────────┼───────────┼───────────────┤
│  Codebase   │  1.67M    │   Fine-tuned  │
│  Ingestion  │  Vectors  │   on Unheaded │
└─────────────┴───────────┴───────────────┘
```

## RAG Pipeline

- **Embedding model:** Sentence transformers for code and documentation
- **Vector store:** FAISS index with 1.67M vectors covering the full Unheaded codebase
- **Chunk strategy:** AST-aware splitting for Go and Rust, section-aware for Markdown
- **Retrieval:** Top-k similarity search with reranking

## Mistral-7B + RAFT Training

Zhen uses a Mistral-7B base model fine-tuned with RAFT (Retrieval Augmented Fine-Tuning):

- **Training data:** Unheaded codebase, protocol specs, session handoffs, wiki pages
- **RAFT approach:** Model learns to use retrieved context effectively, improving accuracy on domain-specific queries
- **Specializations:** Protocol wire format, eBPF program generation, service configuration, lore consistency

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

*Last updated: March 17, 2026*
