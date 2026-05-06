# Roadmap

## Current State: Age 3 — Public Release Sprint

Track A/B/C call pending Captain. Public-launch thread gated. Research thread (Forge / Kingdom-RAFT / WAVE10–17) shipping at quality.

### Completed Ages

- **Age 0 — Foundation Stone:** crew, lore, protocol vocabulary frozen
- **Age 1 — Alpha Ascension:** core services operational, Wotan message bus, eBPF traces end-to-end, Doom-over-IPv6 PoC
- **Age 2 — Beta Trials:** S36 Four Pillars (Port Authority, gRPC-First Transport, Log Aggregation, Service Discovery), S51 Auth Framework, S67 Wire Format Freeze, WEST + EAST bare-metal hosts online, cross-host BPF flow graph operational

### Age 3 — In Progress

**Shipped:**
- S67 Wire Format FROZEN at v0x01 (12 IANA registries)
- S78 LOC audit
- S-WEST + S-EAST bootstrap
- ADR Sweep 2026-04-03 (all 27 ADRs resolved)
- Zhen Champion (`pkg/champion/`)
- Zhen MCP Server (Claude Code integration)
- 31 operational runbooks
- Service Template (`pkg/service/`)
- BPF CI Gate, Sealed Cask, Binding Rune verification
- ROCm GPU acceleration (PyTorch 2.5.1 + ROCm 6.2)
- RAFT QA generation
- WAVE10C backprop fixes
- Wotan topic signing (ML-DSA-65 enforcement)
- Mímir's Law / Gleipnir Phase 0 (real-metal validated on EAST, 2026-04-11)
- WAVE10F Forge real-attention Gemma-4
- Learning Gate strict experiments (β = 0.27 generalization)
- 24h Consolidation Block (lr=1e-3 stable)
- ADR-048 ForgeBackend Trait
- WAVE11 GPU kernels (4 attention grad, cosine 1.000)
- WAVE12 Kingdom RAFT LoRA + ADR-050 GPU-resident activations
- WAVE13 Phase 1 generate-gemma4 + Phase 2 verdict: RETRAIN (ADR-051)
- Round Table verification audit (19 seats)
- WAVE16 Multi-model selector + ADR-060 LIVE
- WAVE17 K8s substrate proven (9/9 services Running on 3-node kind)
- ADR-064 Wotan Active/Active Cluster spec (impl deferred)

**Remaining:**
- Captain Track A/B/C decision (overdue 2026-04-29)
- WAVE14 retrain (gated on track-call)
- Branch hygiene execution
- SBOM regen + license scan + threat refresh
- Sophia draft-04 ship-or-defer
- Wotan draft-04 ship-or-defer
- Demo + README polish for public readiness
- Sub-50ms latency benchmark (Scientist falsifiability gate)
- Public accessibility (optional auth)

---

## Age 4 — MVP Era (Planned)

Track-call dependent. Activates after Age 3 public-launch gate.

- WAVE14 BackwardScratch + KV-cache (gated on Track A or C)
- Performance benchmarking suite
- Customer onboarding flows
- Multi-tenant isolation hardening

---

## Age 5 — Scaling Era (Planned)

Long-horizon. Kingdom Mode rollout, multi-region, federation, Wotan DISTRIBUTED, UPC L6 (Linux boot).

---

*Source of truth: `references/timeline.md`. Drift policy: ADR-052 (≤7 days from HEAD).*
