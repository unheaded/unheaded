# H1 (Retrieval Comparison) — Resolved by H0 Evidence

**Date:** 2026-05-02
**Status:** Resolved without formal experiment
**Decision:** Stevie, 2026-05-02 (response to "(a) skip formal Phase 0b")

## H1 (the original hypothesis)

Per `~/.claude/plans/synthetic-stirring-pudding.md` §4 and `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §Phase 0b:

> **H1.** `vor` retrieval recall is **≥ FAISS recall** on a held-out 100-prompt fixture from the existing 1.76M-vector corpus.

The intent: confirm vor's curated 1847-sheet index has enough coverage to replace the legacy Mistral-era FAISS pipeline (1.76M-vector flat index over `raft/corpus/ring_all.jsonl`) without losing retrieval quality on the corpus that actually matters (Unheaded markdown, embedded cheatsheets).

## Why a formal experiment isn't needed

The H0 baseline already settles this empirically. From `eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md` (committed `b6c656c4`):

1. `bin/zhen-rag` uses **vor only** for retrieval — see `cmd/zhen-rag/main.go:vorRetriever`, no FAISS dependency anywhere in the Go code path.
2. `bin/zhen-rag` scored **12 PASS / 14, verdict H2** on the 14-prompt textbook tier.
3. The rewired Python UI will use **vor only** as its retrieval substrate (per Phase 1 of the WAVE15 battle plan — Step 1.1 ports `cmd/zhen-rag/main.go:vorRetriever` to Python verbatim).

Therefore: vor's coverage is **provably sufficient for the gate**. Whether vor matches FAISS on a wider 100-query fixture spanning Wikipedia / Stack Overflow / general programming corpora is **academic for the rewire** — we are not shipping FAISS regardless. The legacy 1.76M-vector index is a retired substrate; recall comparison against it answers a question we are not asking.

## What the formal experiment would have shown (predicted)

For pure Unheaded-markdown queries (the domain actually relevant to the gate): vor wins because its index is curated and source-trust labeled. FAISS would tie or come close on chunks where its windowed slicing happens to align with topic boundaries, lose where it doesn't.

For general-coding queries with no Unheaded answer (e.g., "how do I center a div in CSS"): both retrievers return weak / empty results. The model's training distribution is the dominant signal regardless of retrieval; this is exactly the failure mode that produced today's `syntax-go` FAIL on the H0 baseline (model defaulted to Rust because no Go-specific reference surfaced).

For Wikipedia / Stack Overflow queries: FAISS wins by definition (it has those corpora; vor doesn't). But this is the use-case ADR-056 (`docs/adr/ADR-056-pgvector-auxiliary-corpus-sharding.md`) addresses with auxiliary corpus shards — separate plan, gated on WAVE15 passing.

## Decision

**H1 is satisfied by H0 for the rewire's purpose.** Phase 0b is skipped; Phase 1 (the actual rewire of `raft/scripts/zhen_rag.py`) starts now.

If empirical retrieval coverage becomes a question for a future plan (e.g., ADR-056 activation), the formal 100-query comparison can be run against vor + the new pgvector shards at that point — comparing the new federated retriever against either a Phase-0-style baseline OR against the legacy FAISS index if it's still on disk. That's a future-WAVE concern, not a WAVE15 blocker.

## References

- `eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md` — H0 baseline (12 PASS / 14)
- `cmd/zhen-rag/main.go` — the vor-only retrieval path that produced H0
- `~/.claude/plans/synthetic-stirring-pudding.md` §4 — original H1 framing
- `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §Phase 0b — formal experiment specification (skipped)
- `docs/adr/ADR-056-pgvector-auxiliary-corpus-sharding.md` — where retrieval coverage beyond vor's curated set is properly addressed
