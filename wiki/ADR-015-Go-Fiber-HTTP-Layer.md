# ADR-015: Go-Fiber for HTTP API Layer

**Status:** SUPERSEDED (2026-10-27) — the Fiber migration never started; services use stdlib `net/http` with the shared `pkg/httputil` middleware, and the `pkg/service` template was deleted in the E1 sweep (ADR-090). Fiber would also fail ADR-004's established-org rule.  
**Date:** 2026-02-20

> Wiki stub generated 2026-05-06 by Marshal overnight sweep (NORTH-STAR Appendix A B5). See the canonical ADR for full text and rationale.

## Canonical

[docs/adr/ADR-015-go-fiber-http-layer.md](../docs/adr/ADR-015-go-fiber-http-layer.md)

## Cross-references

- [ADR Index](ADR-Index.md)
- [Architecture overview](Architecture.md)
