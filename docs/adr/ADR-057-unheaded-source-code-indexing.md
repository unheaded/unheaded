# ADR-057 — Unheaded Source Code Indexing for Semantic Retrieval

**Status:** Proposed
**Date:** 2026-05-02
**Deciders:** Stevie Bellis + unheaded-architect + unheaded-developer + unheaded-scientist
**Context owner:** zhenai retrieval substrate
**Companion:** [ADR-056](ADR-056-pgvector-auxiliary-corpus-sharding.md) — the pgvector sharding pattern this ADR is the first concrete instance of.
**Triggered by:** WAVE15 (`docs/battle-plans/WAVE15-ZHENAI-REWIRE.md`) discussion 2026-05-02 — Stevie: *"I liked the idea of unheaded source code being FAISS indexed and 'understood'."*

---

## Context

vor (`cs serve`) currently indexes the Unheaded repo via the symlink `~/.config/cs/sources/unheaded → /home/govan/tmp/unheaded`. This works **for markdown**: `cs` walks the tree, treats each `.md` file as a topic, indexes the first H1 as the title and the body as searchable content. It is the substrate that lets the coding-gate's grounding answers — questions about WAVE14 H6 parser bugs, Mimir's Law, the Doom port — actually retrieve relevant Unheaded prose.

It does **not** work for source code:

- **vor's chunking is markdown-shaped.** A 2000-line `cmd/zhen-agentd/main.go` becomes one "topic" whose "title" is whatever `cs` decides (filename or first comment), with the entire file body as one searchable blob. Search ranks by full-text overlap against this one blob. Functions, types, and packages are not first-class; cross-file relationships are invisible.
- **Generic-text embeddings retrieve poorly on code.** Even if we run `all-MiniLM-L6-v2` over source files, the model was trained on prose. `func handleAsk(w http.ResponseWriter, r *http.Request)` and `def handle_ask(request, response)` are semantically near-equivalent but produce different vectors because the training distribution didn't see Go-Python equivalence.
- **No call-graph awareness.** A query like *"where is `Champion.Dispatch` called?"* has a precise structural answer (look at every Go file that imports `pkg/champion` and calls `.Dispatch`); vor cannot give it.

The legacy Mistral-era FAISS pipeline at `raft/scripts/zhen_rag.py` (now retired by WAVE15) attempted to compensate by chunking source files into ~500-token windows and indexing them in the same flat 1.76 M-vector index alongside markdown. It worked-ish, but the retrieval quality on code-specific queries was uneven — chunk boundaries cut across functions, embeddings weren't code-aware, and chunks lacked structural metadata (file path, function name, line range) so even a perfect retrieval couldn't tell the model *where* the chunk lived.

The Zhen vision (per `~/.claude/projects/-home-govan-tmp/memory/project_zhen_vision.md`) — *"Zhen builds itself + Unheaded; Opus handles frontier tasks"* — requires zhenai to actually *understand* the Unheaded codebase, not just retrieve fuzzy matches. "Understood" here means: ask "how does the Champion gate handle empty justification?" and get back the exact `pkg/champion/toolcall.go:HasUntrustedJustification` function body with its docstring and surrounding comments, not a 500-token slice that happens to share keywords.

ADR-056 establishes the pattern for non-vor retrieval shards. This ADR is the first concrete instance: an `aux_unheaded_code` shard with **AST-aware chunking** and a **code-specialized embedder**.

---

## Decision

**Index the Unheaded source tree as `aux_unheaded_code` in pgvector. Chunk by AST node (function, method, type, top-level decl). Embed with a code-specialized model. Carry full structural metadata. Refresh on commit.**

### Source-tree scope (in)

- `cmd/**/*.go` — Go services
- `pkg/**/*.go` — Go shared packages
- `services/**/*.go` — Go microservices
- `crates/**/src/**/*.rs` — Rust (eBPF, forge, zhend)
- `raft/**/*.py` — Python (zhenai web UI, MCP server, scripts)
- `dashboard/**/*.{js,html,css}`, `kanban/**/*.{js,html,css}` — frontend
- `db/migrations/*.sql` — schema
- `runbooks/**/*.yaml` — operational runbooks
- `docs/**/*.md` — already in vor; **excluded here** to avoid double-indexing
- `eval/**/*` — already-stable fixtures; included for grounding

### Source-tree scope (out)

- `**/target/**`, `**/node_modules/**`, `**/_legacy/**`, `**/.git/**` — build artifacts and history
- `**/*.gguf`, `**/*.index`, `**/*.bin` — binary assets
- Generated code (where identifiable: `*.pb.go`, `*.gen.go`, `target/debug/build/*/out`)

### Chunking strategy: AST-anchored

Use `tree-sitter` (with language-specific grammars: `tree-sitter-go`, `tree-sitter-rust`, `tree-sitter-python`, `tree-sitter-javascript`) to split each file into chunks at semantically meaningful boundaries:

| Language | Primary chunk unit | Secondary unit |
|---|---|---|
| Go | top-level `func` declaration | type/struct/interface decl, package-level var/const block |
| Rust | `fn` / `impl` block | `struct`/`enum`/`trait` decl, top-level `use` cluster |
| Python | `def` / `class` | top-level statement group |
| JavaScript | function decl / arrow assignment / class | top-level export/import |
| SQL | full migration file (already small) | — |
| YAML | full runbook (already small) | — |

Each chunk records:

```jsonc
{
  "id":          "pkg/champion/toolcall.go::HasUntrustedJustification",
  "file_path":   "pkg/champion/toolcall.go",
  "language":    "go",
  "kind":        "func",
  "symbol":      "HasUntrustedJustification",
  "parent":      "ToolCall",                // method receiver / class / impl block
  "byte_start":  4231,
  "byte_end":    4980,
  "line_start":  158,
  "line_end":    170,
  "content":     "...full chunk body, including doc-comment immediately above...",
  "imports":     ["context", "fmt"],        // best-effort, language-dependent
  "docstring":   "HasUntrustedJustification reports whether ...",  // if present
  "embedding":   <384-dim vector>,
  "source_trust": "local",                  // it's our own code
  "git_blob_sha": "abc123def...",           // for incremental refresh
  "indexed_at":  "2026-05-02T18:55:32Z"
}
```

The doc-comment immediately above a function (Go's `// Foo does ...`) is included in the chunk so the embedder sees the author's stated intent alongside the implementation. This materially improves retrieval on "what does X do?" queries.

### Embedding model: code-specialized, locally hosted

**Default proposal:** `BAAI/bge-code-v1` (1024-dim, ~1.3 GB, ONNX-exportable, code-trained). Outperforms `all-MiniLM-L6-v2` on code-search benchmarks (CSN, CodeSearchNet) by 30-50% NDCG.

Alternates:
- `nomic-embed-code` (768-dim, ~440 MB) — smaller, slightly weaker.
- `all-MiniLM-L6-v2` (384-dim, 80 MB) — what we use elsewhere; ship-this-fast option but measurably worse for code.
- `Qwen3-Embedding-0.6B` — newer, multilingual code+prose, 1024-dim.

Decision criterion: pick whichever provides the highest NDCG@10 on a 50-query code-search fixture (e.g., "find the function that validates path against allowlist" → expected hit `pkg/champion/champion.go:validatePath`). Phase 0 of the implementation defines the fixture and runs the comparison; the model with the best score under 1.5 GB ships.

The embedder runs in the same sidecar pattern as ADR-056's prose embedder. Two embedding endpoints (`/embed/code`, `/embed/prose`) with the appropriate model loaded behind each. Or one sidecar with model selection per request.

### Refresh policy: git-driven incremental

`aux_unheaded_code.git_blob_sha` is the cache key. On refresh:

1. `git ls-tree -r --object-only HEAD` produces the current set of `(path, blob_sha)` pairs.
2. For each `(path, blob_sha)`: if a row exists with matching `git_blob_sha` and any chunk is missing or stale, delete + re-chunk + re-embed. If row exists and matches, skip.
3. For paths no longer in the tree: delete all chunks.
4. New paths: full chunk + embed.

A pre-commit hook (or a CI job, or a daily cron) triggers refresh. Initial bulk load is ~10-30 minutes on west's GPU — bounded by Unheaded's actual code size (per `CLAUDE.md` LOC audit S78: 415 K production lines + 753 K with tests = ~1.1 M lines, but AST chunking gives ~50-100 K chunks, not millions).

### Trust label: `local` (not `external`)

Unheaded source code is **our own** — `pkg/champion/toolcall.go` does not poison the gate by being included as a justification reference. Per the gate's three rules:

- Rule 1 (path-allowlist): unaffected — chunks aren't paths the agent writes to.
- Rule 2 (untrusted justification): chunks tagged `local` do **not** trip Rule 2. The agent emitting `kanban_create` grounded in a Champion-source-code reference is treating its own code as documentation, which is the intended behavior.
- Rule 3 (destructive verb): unchanged — destructive verbs in args are caught regardless of justification.

This is in contrast to `aux_wikipedia` (per ADR-056) which defaults to `external` because Wikipedia content can be adversarially edited.

### Symbol-aware retrieval (the "understood" part)

The chunk schema's `symbol` and `parent` fields enable structurally-aware queries beyond cosine similarity:

```sql
-- "Show me HasUntrustedJustification and its callers"
SELECT * FROM aux_unheaded_code
 WHERE symbol = 'HasUntrustedJustification'
    OR (content LIKE '%HasUntrustedJustification%' AND symbol != 'HasUntrustedJustification');
```

In Phase 2 (post-initial-ship), a lightweight call-graph extraction (parse `*.go` for `pkg.Func()` patterns) populates a sibling table `aux_unheaded_code_calls(caller_id, callee_id)` enabling proper "who calls X" queries. Tree-sitter alone is sufficient for first-pass; full LSP-grade resolution (`gopls`, `rust-analyzer`) is a Phase 3+ upgrade if the basic pattern proves valuable.

---

## Consequences

### Positive

- **Retrieval quality on code questions improves materially.** Code-specialized embeddings + AST chunks give the agent grounded references with file paths and line numbers, not text-similar excerpts.
- **The "indexed and understood" capability lands.** Queries like "what does the Champion gate do?" return the gate's actual source — function bodies, doc-comments, struct definitions — at function-grain.
- **Self-improvement loop tightens.** The Zhen vision requires zhenai to know its own codebase. This ADR is the substrate for that knowledge.
- **Coding-gate improvement plausible.** The H0 baseline today (12 PASS / 14, 1 🔴 on review-javascript) failed `syntax-go` because retrieval didn't surface Go's `if err != nil` pattern. With a code shard tagged `local`, the agent can ground "how do I check for an error in Go" in the actual idiom from `pkg/champion/champion.go:148-151`. **Hypothesis worth testing in a separate experiment.**
- **Refresh is incremental and cheap post-bootstrap.** Daily git-driven refresh on a 1.1 M-line repo is minutes of GPU time, not hours.

### Negative / costs

- **Initial bootstrap cost.** ~50-100 K chunks × ~150 ms code-embedding batch = 1-3 hours on west's GPU. One-time, but real.
- **Storage.** Chunks + embeddings ≈ 100 KB/chunk × 100 K = ~10 GB in The Well. ivfflat index adds ~10%.
- **Two embedding models in flight.** `all-MiniLM-L6-v2` (prose, memory + auxiliary prose shards) AND `bge-code-v1` (code). Sidecar complexity increases. Disk + VRAM cost: an extra ~1.3 GB resident.
- **AST chunking complexity.** Tree-sitter parses are language-specific; we have to maintain grammars for at least 5 languages (Go, Rust, Python, JavaScript, SQL). Tree-sitter handles syntax errors gracefully but the chunk extractor has to be defensive. Roughly 200-400 LOC of Python or Rust per language for the chunker.
- **Stale chunks during refresh window.** If a function is renamed or moved, retrieval may surface the old chunk for the few minutes between commit and refresh. Mitigation: post-commit hook is the simplest fix; CI integration is more robust.

### Mitigations / scope-limiting

- Phase the rollout: bootstrap the chunker + embedder against `pkg/champion/` first (small, well-understood, gate-relevant). Validate retrieval quality on a 20-query fixture before scaling to the full repo.
- Defer the call-graph table to Phase 2; ship cosine-only retrieval first.
- Defer LSP-grade resolution to Phase 3+; tree-sitter alone is sufficient for first ship.
- The `aux_unheaded_code` shard is **opt-in** for the agent layer. The agent loop's `Retriever` chooses whether to consult it. WAVE15 ships without it; this ADR's activation is post-WAVE15.

---

## Open questions

1. **Retrieval interface for symbol-anchored queries.** Cosine similarity is one query mode; "find all callers of X" is another. Should the federated retriever expose a structural-query mode, or is that a separate tool the agent invokes? Lean: separate tool (`code_callers`, `code_definition`) so cosine retrieval stays simple.
2. **Test code: included or excluded?** `*_test.go` files are valuable for "show me how to use this function" but pollute "show me production behavior" queries. Decision: include with `kind: "test"` tag; agent can filter at query time.
3. **Generated code detection.** `*.pb.go` is obvious; `*.gen.go` is convention; `target/debug/build/.../out/*.rs` is path-anchored. Heuristic: any file with a `// Code generated by ... DO NOT EDIT.` first-line comment is excluded. Test against the actual repo.
4. **Embedding model version drift.** If we upgrade `bge-code-v1` to a future `bge-code-v2`, every chunk must be re-embedded. Migration cost: ~hours. Mitigation: pin model version in the chunk's metadata column; refresh on model change.
5. **Multi-repo future.** If Unheaded grows into a monorepo of pluggable Kingdom modules, does `aux_unheaded_code` shard further (`aux_code_pkg_champion`, etc.) or stay flat? Defer until the question matters.
6. **Doom-related code.** `crates/doom-runner/` and the GPL-isolated `doom/doomgeneric/` (per CLAUDE.md `LICENSES/THIRD_PARTY.md`) — index? Lean: index `crates/doom-runner/` (our code), exclude `doom/doomgeneric/` (GPL boundary preserved).

---

## Implementation outline (when activated)

This ADR is **proposed**; activation requires:

1. WAVE15 H0 passes (rewire ships, gate clears).
2. ADR-056 lands as Accepted (the sharding pattern this builds on).
3. A concrete user signal — Stevie says "go" or a coding-gate failure that auxiliary code retrieval would have prevented.

Activation steps (sketch — not battle-plan-grade; full plan in a future `docs/battle-plans/WAVE16-CODE-INDEX.md` when activated):

1. **Phase 0 — Embedder selection.** 50-query code-search fixture extracted from existing Go/Rust/Python in the repo. Compare `bge-code-v1` vs `nomic-embed-code` vs `all-MiniLM-L6-v2`. NDCG@10 picks winner. Update this ADR's "Decision" with the empirical choice.
2. **Phase 1 — Chunker + bootstrap.** Tree-sitter chunker for one language (Go) and one package (`pkg/champion/`). Round-trip into `aux_unheaded_code`. Hand-validate 20 chunks for sensible boundaries.
3. **Phase 2 — Multi-language + full-repo bootstrap.** Add Rust + Python + JS chunkers. Bulk-embed entire repo. Storage + timing measured.
4. **Phase 3 — Refresh hook.** Post-commit hook (or CI job) drives incremental refresh.
5. **Phase 4 — Federated retriever wires `aux_unheaded_code` into the Go agent path.** Per ADR-056's `pkg/retrieval/sharded.go`. New chunks tagged `local` flow through the gate without triggering Rule 2.
6. **Phase 5 — Validation.** Re-run H0 coding gate. Hypothesis: `syntax-go` flips PASS (because retrieval now surfaces actual Go error-handling idioms from the codebase). Document in `eval/coding-gate/results-with-code-shard-<date>.md`.

---

## References

- ADR-056 — pgvector auxiliary corpus sharding (the parent pattern).
- `~/.claude/projects/-home-govan-tmp/memory/project_zhen_vision.md` — the "Zhen builds itself" vision this ADR's "understood" capability serves.
- `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §1 — the conversation that triggered this ADR.
- `eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md` — H0 baseline; `syntax-go` FAIL is the most concrete motivator for this ADR.
- `pkg/champion/toolcall.go` — the gate that auxiliary chunks must continue to satisfy via correct `source_trust` labels.
- Tree-sitter — https://tree-sitter.github.io/ (note: external doc, not a runtime dependency on internet — the grammars vendor as static libraries).
- `BAAI/bge-code-v1` — proposed embedding model. Card: https://huggingface.co/BAAI/bge-code-v1.
