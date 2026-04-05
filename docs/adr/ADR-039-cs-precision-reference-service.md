# ADR-039: CS Cheat Sheet Integration — Precision Reference Service

## Status: IN PROGRESS

## Date: 2026-04-05
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)

---

## Context

A cheat sheet project at `~/tmp/projects/cs/` contains 1,103+ markdown files
across 134 directories covering enterprise networking, security, infrastructure,
and certification prep (CCNP, CCIE, JNCIE, CISSP, CompTIA, CEH). The project
has a REST API service mode and is actively growing.

This is a gold mine for Zhenai training data AND a potential precision reference
service for Unheaded's operational knowledge.

## Decision

### Phase 1: Training Data (immediate)
Generate QA pairs from all 1,103 markdown sheets for Zhenai LoRA training.
These cover the exact networking/security/infrastructure knowledge that Zhenai
needs for autonomous operations (BGP, OSPF, IPsec, TLS, firewall rules,
kernel tuning, container security, etc).

### Phase 2: Precision Service (planned)
Integrate the cs tool as an Unheaded service — a precision reference lookup
for detailed protocol specs, configuration examples, and troubleshooting
procedures. The existing REST API becomes a Kingdom service on a Doom Range port.

### Architecture (Phase 2)

```
Zhenai query: "How do I configure OSPF area 0 on the Kingdom mesh?"
    │
    ├─→ RAG search (1.76M vectors) — broad context
    │
    ├─→ CS Precision Service — exact cheat sheet match
    │   GET /api/v1/sheets/ospf/area-types
    │   Returns: detailed OSPF area config with examples
    │
    └─→ Mistral-7B generates grounded answer from both sources
```

## Consequences

### Positive
- 1,103 sheets → ~5,000-10,000 QA pairs for training
- Precision answers for networking/security questions
- Covers certification-level depth (CCIE, JNCIE)
- REST API already exists — minimal integration work
- Growing actively — new sheets auto-incorporated

### Negative
- Separate repo — needs cross-repo coordination
- Training data quality depends on sheet quality (high — cert-grade)

## References
- Project: `~/tmp/projects/cs/`
- 537+ sheets, 59 categories (as of initial count)
- 1,103 markdown files total (including details)
- Battle plan: `~/tmp/projects/cs/docs/BATTLE-PLAN.md`
