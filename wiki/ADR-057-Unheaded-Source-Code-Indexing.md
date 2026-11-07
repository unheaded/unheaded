# ADR-057: Unheaded Source Code Indexing for Semantic Retrieval

**Status:** Proposed  
**Date:** 2026-05-02

> Wiki stub generated 2026-05-06 by Marshal overnight sweep (NORTH-STAR Appendix A B5). See the canonical ADR for full text and rationale.

## Canonical

[docs/adr/ADR-057-unheaded-source-code-indexing.md](../docs/adr/ADR-057-unheaded-source-code-indexing.md)

## Cross-references

- [ADR Index](ADR-Index.md)
- [Architecture overview](Architecture.md)

## Grimoire corpus, S-ANSWERED 2026-11-06

DP-719 option A: the Python RAG stack under `tomb/grimoire/rag/` is gone and the kept corpus is served by this index. `codeindex.BuildCorpus` reads Markdown, plain text and STIX bundles from a directory, with no git, so it runs on the air-gapped Tomb VM. `retrieval.CorpusBackend` serves it; zhen-agentd joins it with `-corpus-index`, and `zhen-codeindex --search` answers on the VM. The source shard still excludes Grimoire paths: the corpus is a separate index. Search is by word, so the old index's semantic matching is given up: a synonym the text does not use finds nothing.
