# ADR-060: Zhenai Multi-Model Selector (UI dropdown for live llama-server swap)

**Status:** Accepted — Implemented 2026-05-04 (8623c864: allowlist, Champion-gated `model_switch`, UI dropdown). Tail closed 2026-10-27 (S-CHAMPION-HARDENING Phase 11): `switch-model.sh --json`, swap metrics (`zhen_model_swap_duration_seconds`, `zhen_model_swap_failures_total`), T19 timeout rollback, `zhen-cli /model`, LICH-013 how-to-run (`tomb/lich/LICH-013-model-selector/CAMPAIGN.md`; the campaign itself has not run). Still open: T11/T13 status codes (500, not 400/429).  
**Date:** 2026-05-04

> Wiki stub generated 2026-05-06 by Marshal overnight sweep (NORTH-STAR Appendix A B5). See the canonical ADR for full text and rationale.

## Canonical

[docs/adr/ADR-060-zhenai-multi-model-selector.md](../docs/adr/ADR-060-zhenai-multi-model-selector.md)

## Cross-references

- [ADR Index](ADR-Index.md)
- [Architecture overview](Architecture.md)
