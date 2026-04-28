# Anamnesis Spec Status — 2026-04-27

**Captured**: 2026-04-27 from Cowork-on-Macbook (RFC-Editor + Architect hats)
**Owner**: RFC-Editor + Architect
**Source-of-truth ADR**: ADR-052

## Current state on main

❌ **No draft Internet-Draft for Anamnesis exists on main.** Only an implementation.

| Artifact | Path | Status |
|---|---|---|
| Service implementation | `services/anamnesis/anamnesis.go` + tests | ✅ Present |
| Protocol API integration | `cmd/protocol-api/anamnesis.go` | ✅ Present |
| Dashboard backend integration | `cmd/dashboard-backend/internal/ebpf/anamnesis.go` + tests | ✅ Present |
| eBPF reader package | `pkg/ebpf/anamnesis_reader_test.go` | ✅ Present |
| K8s manifest | `deploy/k8s/gnostic/anamnesis-deployment.yaml` | ✅ Present |
| LXD container config | `lxd/containers/anamnesis.yaml` | ✅ Present |
| NixOS module | `nixos/modules/services/anamnesis.nix` | ✅ Present |
| **Internet-Draft / spec** | **MISSING** | ❌ **GAP** |

## Anamnesis in the protocol family

Per CLAUDE.md and Kingdom lore:
- Anamnesis = "event/analyzer" component (Gnostic naming)
- Pairs with Monad (wire) / Sophia (BPF maps) / Wotan (memory) at the protocol level
- Protocol-aware skill descriptions reference Anamnesis as a first-class component
- It is the *fourth pillar* of the protocol family but the *only pillar without a draft I-D*

## Why this is a gap

1. **Doc-web asymmetry**: three of four pillars have Internet-Drafts; the fourth has only code. Future readers (or IETF reviewers) cannot discover Anamnesis without grepping the codebase.
2. **Spec-vs-code drift risk**: code is the de-facto spec when no spec exists. Any wire format Anamnesis exposes (event format, analyzer API, time-series schema) is implicit.
3. **IANA implications**: if Anamnesis publishes events that touch any registered code points, the spec is required for IANA review.
4. **Marshal-citable**: per ADR-052 source-of-truth policy, "battle plans of record live in-tree." A protocol component without a spec is functionally similar to a plan of record without an in-tree home.

## Recommendation

**DEFER — write a spec stub now (Phase 1), full draft later (Phase 2).**

This is the only spec in this sweep that gets a real defer recommendation, and it's a partial defer:

### Phase 1 — Spec stub (this sprint or next, low cost)
- Author `docs/protocol/draft-bellis-unheaded-anamnesis-00.md` with:
  - Abstract (1 paragraph)
  - Status: WORK IN PROGRESS (clear about its draft-stub nature)
  - Section skeleton: Introduction, Components, Event Format, Analyzer API, Security Considerations, IANA Considerations
  - Cross-reference Foundation-06, Wotan-03, Sophia-03 in the Normative References section as version-locked anchors
- Estimated effort: 2-4 hours (RFC-Editor-led)

### Phase 2 — Full draft-00 (Track-call dependent)
- Activates after Captain Track decision:
  - **Track A**: defer Phase 2 to Sprint May-Q3 — Anamnesis is implementation-led, code is honest
  - **Track B**: prioritize Phase 2 in Sprint May-Q1 — public-launch optics benefit from "all four pillars specified"
  - **Track C**: prioritize Phase 2 in launch thread — same reasoning as Track B
- Estimated effort: 1-2 weeks of RFC-Editor work + Architect collaboration

### Phase 3 — IETF datatracker submission (post-Phase-2)
- Same pattern as the existing six I-Ds (per S74 / Ragnarok Sprint)
- Adds Anamnesis to the IETF Independent Submission stream

## Alternatives considered

1. **Write full draft-00 in this sprint**. Rejected — RFC-Editor 1-2 weeks of work is too much for a sprint focused on Round Table follow-through. Phase-1 stub is the right scope.
2. **Continue with code-as-spec indefinitely**. Rejected per Marshal/Librarian source-of-truth policy. Eventually citable.
3. **Combine Anamnesis into Wotan draft-04**. Rejected — Anamnesis is conceptually distinct (events/analysis, not memory). Folding it into Wotan would muddy both specs.

## Cross-references

- ADR-052 — applies to specs as source-of-truth artifacts
- ADR-038 — Kanban GUID → Git Audit Trail (Anamnesis-adjacent in spirit)
- Lore-keeper notes — Anamnesis pairs with Monad/Sophia/Wotan in Gnostic cosmology

---

*Anamnesis spec status: draft missing. Phase-1 stub recommended next sprint; full draft Track-dependent.*
