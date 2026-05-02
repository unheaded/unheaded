# ADR-056 — pgvector Auxiliary Corpus Sharding for Trust-Tagged Retrieval

**Status:** Proposed
**Date:** 2026-05-02
**Deciders:** Stevie Bellis + unheaded-architect + unheaded-scientist + unheaded-blackmage
**Context owner:** zhenai retrieval substrate
**Companion:** [ADR-057](ADR-057-unheaded-source-code-indexing.md) — first concrete instance of this pattern (Unheaded source files)
**Triggered by:** WAVE15 (`docs/battle-plans/WAVE15-ZHENAI-REWIRE.md`) discussion 2026-05-02 — Stevie asked whether vor could index Wikipedia / Stack Overflow scale corpora and whether DB-type sharding could expose auxiliary content the way the legacy Mistral-era FAISS index did.

---

## Context

The Go zhen-* stack uses **vor** (`cs serve`) as its retrieval substrate. vor is filesystem-backed: it indexes topic-organized markdown under `~/.config/cs/sources/<source-name>/`. The current production index has 1847 sheets across 283 categories — curated cheatsheets plus Unheaded markdown via the `~/.config/cs/sources/unheaded → /home/govan/tmp/unheaded` symlink.

This shape works extremely well for what it is, and it carries Champion's source-trust labels natively (B1 schema: `embedded` → canonical, `user-custom` → local, `user-source` → external; see `pkg/champion/toolcall.go:Reference`). It does **not** scale to:

- **Wikipedia** (~17 M chunks per the legacy `raft/corpus/wikipedia.jsonl` 29 GB file)
- **Stack Overflow** (Q&A volume comparable)
- **IETF RFCs** (9,739 documents per `raft/CLAUDE.md`)
- **Research papers** (1,649 per the same source)
- **Source files at large repo scale** — covered by ADR-057

vor's filesystem model (one markdown file per topic) breaks in two ways at this scale:

1. **Inode pressure.** ext4 stops behaving well past ~10 M files in a single directory tree; even hash-sharded subdirs become fragile.
2. **Trust-label semantics blur.** Wikipedia and Stack Overflow are *external* by Champion's gate, but vor's per-source trust label gets assigned at directory level — every file under `~/.config/cs/sources/wikipedia/` would be `external`, which is correct but mixed with `unheaded` (`external`) and `embedded` (`canonical`) creates ranking awkwardness when the agent has to choose.

The legacy Mistral-era pipeline solved volume by using FAISS over a 1.76 M-vector flat index built from JSONL chunks. WAVE15 retires that index because (a) it was tied to the Mistral-7B baseline that's been decommissioned, and (b) the chat path passes the coding gate on vor alone (12 PASS / 14 baseline H2 per `eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md`).

But the **capability** the FAISS pipeline provided — semantic retrieval over corpora that don't fit vor's filesystem model — is real and worth preserving. The right place to put that capability is in The Well (PostgreSQL), which already has multi-tenant DB segregation and the operational discipline the codebase enforces (parameterized queries, idempotent migrations, per-service users via `db/migrations/007_grants.sql`).

---

## Decision

**Adopt pgvector in The Well as the auxiliary retrieval substrate. Shard by source. Federate vor + auxiliary shards behind a single Go retriever interface. Trust labels propagate.**

### Architecture

```
                                    ┌──────────────────────────────┐
                                    │  vor :9876   (TIER 1)        │
                                    │  curated cheatsheets +       │
zhen-agentd / pkg/agent             │  Unheaded markdown           │
   │                                │  source_trust:                │
   │  Retriever interface           │    canonical / local         │
   │                                │  ~1847 sheets today          │
   ├── FederatedRetriever ──────────┤
   │                                │  TIER 2 (pgvector shards)    │
   │   merges by                    │  Postgres in The Well         │
   │   (source_trust_priority,      │                              │
   │    cosine_score)               │  aux_wikipedia               │
   │   then top-K                   │  aux_stackoverflow           │
   │                                │  aux_rfc                     │
   │                                │  aux_papers                  │
   │                                │  aux_unheaded_code (ADR-057) │
   │                                │  source_trust:                │
   │                                │    external (default)        │
   │                                │    local (Unheaded code)     │
   └────────────────────────────────┴──────────────────────────────┘
                                    
                                    ▲
                                    │ embedding sidecar
                                    │ (sentence-transformers
                                    │  all-MiniLM-L6-v2 today;
                                    │  ADR-057 may upgrade for
                                    │  source-code shard)
```

### Schema pattern (one table per source)

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE aux_wikipedia (
    id           TEXT PRIMARY KEY,
    title        TEXT NOT NULL,
    content      TEXT NOT NULL,
    embedding    vector(384),                -- matches all-MiniLM-L6-v2
    source_trust TEXT NOT NULL DEFAULT 'external'
                 CHECK (source_trust IN ('canonical','local','external')),
    source_label TEXT NOT NULL,              -- 'wikipedia', 'stackoverflow', etc.
    metadata     JSONB DEFAULT '{}'::jsonb,  -- per-source extra (URL, score, etc.)
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX aux_wikipedia_embedding_ivfflat
    ON aux_wikipedia USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 4096);                     -- tune to row count: ~ sqrt(N)

CREATE INDEX aux_wikipedia_title_trgm
    ON aux_wikipedia USING gin (title gin_trgm_ops);

GRANT SELECT ON aux_wikipedia TO app_zhen;   -- read-only for the agent path
GRANT SELECT, INSERT, UPDATE, DELETE ON aux_wikipedia TO ops_zhen;  -- writes via ops user
```

Mirror tables `aux_stackoverflow`, `aux_rfc`, `aux_papers`, `aux_unheaded_code` (ADR-057) — same shape. Per-table tuning of `lists` on the ivfflat index.

### Go retriever interface

```go
// pkg/retrieval/sharded.go (new package — does not exist today)

type Backend interface {
    Name() string                                              // e.g. "vor", "aux_wikipedia"
    DefaultTrust() string                                      // "canonical" | "local" | "external"
    Retrieve(ctx context.Context, query string, k int) ([]agent.TopicContent, error)
}

type FederatedRetriever struct {
    Backends   []Backend
    PerBackend int                                             // top-K per shard
    Total      int                                             // top-K after merge
    Embedder   Embedder                                        // shared embedding service
}

func (f *FederatedRetriever) Retrieve(ctx, query, k) ([]agent.TopicContent, error) {
    // 1. Embed query once (vor doesn't need it; pg shards do).
    vec := f.Embedder.Embed(ctx, query)

    // 2. Fan-out per backend, in parallel, with per-backend timeout.
    // 3. Merge with cross-backend reranking — per-backend top-K, then
    //    sort by (trust_priority desc, cosine desc) and take overall top-K.
    // 4. Each returned chunk carries its source_trust label so Champion's
    //    gate (Rule 2) sees mixed-trust justification chains correctly.
}
```

### Trust-label propagation (the load-bearing property)

Every chunk returned to the agent layer carries its `source_trust` field. `pkg/champion/toolcall.go:HasUntrustedJustification` already returns true if **any** ref in the justification chain is `external`. Therefore:

- Mutating tool calls grounded in Wikipedia/SO content trip Rule 2 → pending-confirm.
- Already covered by red-team probe `cmd/zhen-agentd/redteam_test.go:TestRedTeam_PathTraversal_WriteFileEscapesRoot` and the fixture pattern.
- No change to Champion required.

### Embedding sidecar

WAVE15 keeps `sentence-transformers/all-MiniLM-L6-v2` (384-dim) as the memory embedder. The auxiliary corpus uses the same embedder by default to share infrastructure. ADR-057 may upgrade this for the Unheaded-source-code shard specifically, since code-specialized embedders measurably improve code retrieval.

The sidecar is one HTTP endpoint:
```
POST /embed { "text": "..." } → { "vec": [384 floats], "model": "all-MiniLM-L6-v2", "dim": 384 }
```

Phase 0 of WAVE15 sets this up as a Python sidecar (~80 LOC). A Go-native swap (onnxruntime-go or fastembed-rs) is post-WAVE15.

---

## Consequences

### Positive

- **Volume scales by table size, not file-system inodes.** pgvector's ivfflat index handles 10s of millions of rows comfortably with sub-second queries when tuned (lists ≈ √N).
- **LAN-only posture preserved.** The Well runs locally; embedder is local; everything is loopback.
- **Champion's gate stays authoritative.** Source-trust labels propagate from each shard. No new threat surface.
- **vor stays small and fast.** It keeps the role it's good at (curated, low-volume, canonical-trust content) without being asked to scale to 17 M items.
- **Per-source operational independence.** Migrating a new corpus is a new table + a one-time embedding job. Doesn't touch existing shards.
- **Per-source schema flexibility.** `metadata JSONB` lets each shard carry its own extra fields (Wikipedia revision, SO accepted-answer flag, RFC obsoletes-list, paper DOI) without polluting the common schema.

### Negative / costs

- **Embedding compute.** Every new corpus pays the one-time cost of embedding every chunk on the GPU. Wikipedia at 17 M × 384 floats × ~80 ms/embedding-batch = overnight territory. RFCs and SO are smaller but still hours.
- **Storage.** Each chunk costs `chunk_text_size + 384*4 = chunk + 1.5 KB`. 17 M Wikipedia chunks at ~5 KB average + 1.5 KB embedding = ~110 GB on disk. Plus the ivfflat index (~10% overhead). This is a real cost.
- **Reranking complexity.** Cross-shard top-K merge has to balance `source_trust` priority with cosine score. Naive "all top-3 from every shard" gives bias toward populated shards; tuning the merge function is iterative.
- **Operational scope.** Each shard is an independent migration / re-embedding lifecycle. ADR-057 will face this immediately for Unheaded source code (rebuild on every commit? incremental? scheduled?).

### Mitigations / scope-limiting

- Phase the rollout: start with one small shard (e.g., `aux_unheaded_taught` for `/api/v1/teach` content from the Python web-ui) so the pattern is exercised before committing overnight Wikipedia jobs.
- Defer Wikipedia / Stack Overflow until there's a concrete user need beyond "the legacy FAISS pipeline had it." Track as backlog; ship empty stubs.
- Treat ADR-057 (Unheaded source code) as the first real instance — it's bounded (10s of K chunks, not millions) and the value is provably high (Stevie has explicitly named this as desired).

---

## Open questions

1. **Reranker?** Cross-shard merging by raw cosine + trust priority is naive. Cross-encoder reranking (e.g., bge-reranker-base) would improve quality but adds another model. Defer the decision to ADR-058+.
2. **Snapshot vs continuous indexing.** Wikipedia is a periodic dump; Unheaded source is a live repo. Per-shard refresh strategy decided in each follow-on ADR (ADR-057 makes this call for source code).
3. **Federated-search timeout policy.** What if `aux_wikipedia` is slow today? Each backend's `Retrieve` should be bounded by per-backend deadline so the agent loop's overall timeout (5 min today in `cmd/zhen-agentd`) isn't dominated by a single slow shard. Stub: 2 s per backend, configurable.
4. **Where does the embedding service live?** WAVE15 ships it as a Python sidecar. Long-term: Go service in `cmd/zhen-embed/` with `onnxruntime-go`. Tracked as Stevie's solo post-gate exercise.

---

## Implementation outline (when activated)

This ADR is **proposed**; activation requires:

1. WAVE15 H0 passes (the rewire ships and the gate clears under vor-only).
2. A concrete consumer use-case forces a decision (ADR-057 is the natural first one).

Activation steps (sketch — not battle-plan-grade):

1. ADR-057 lands as Accepted; defines the embedding model, chunking strategy, and refresh policy for the source-code shard.
2. New migration `db/migrations/0NN_pgvector_aux_corpus.sql` enables the extension and creates the first table.
3. `pkg/retrieval/sharded.go` package added; vor wrapped in a `Backend` so today's single-source retrieval becomes a degenerate case of federated.
4. `cmd/zhen-agentd` gains `-aux-shards <list>` flag enumerating the active shards.
5. Per-shard ingestion runbook in `runbooks/zhen/ingest-<source>.yaml` — kicks off the embedding job, confirms row counts, reports completion.
6. Re-run H0 coding gate to confirm no regression. Document in `eval/coding-gate/results-with-aux-shards-<date>.md`.

---

## References

- `~/.claude/plans/synthetic-stirring-pudding.md` §6 Phase 6+ — auxiliary retrieval is parked as Stevie's solo post-gate exercise; this ADR is the design surface for that exercise.
- `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §1 Context — the discussion that triggered this ADR.
- `pkg/champion/toolcall.go:Reference` — the source-trust schema this ADR's chunks must populate.
- `pkg/champion/toolcall_test.go:TestHasUntrustedJustification_*` — the gate behavior auxiliary chunks must continue to satisfy.
- `eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md` — current H0 bar; auxiliary shards must not regress this.
- `raft/scripts/zhen_rag.py` (legacy) — the FAISS pipeline this ADR's pgvector pattern subsumes architecturally.
- ADR-057 — first concrete instance (Unheaded source files).
