# The Unheaded Chronicles

## A Living Grimoire of the Kingdom's Journey

**STATUS:** Age 3 IN PROGRESS — Public Release Sprint
**LAST UPDATED:** 2026-05-05
**HEAD:** WAVE17 K8s substrate proven (9/9 services on kind), ADR-064 active/active spec, doctrine sweep
**Drift policy:** ADR-052 — this file MUST be ≤ 7 days from HEAD when HEAD has new commits.

---

### Age 0: The Foundation Stone (✅ COMPLETED)
*Pre-alpha. Crew assembled. Lore established. Protocol vocabulary frozen.*

### Age 1: The Alpha Ascension (✅ COMPLETED)
**Progress:** 100%
*All 10 core services operational. Wotan message bus shipping. eBPF traces end-to-end.*

### Age 2: The Beta Trials (✅ COMPLETED)
**Progress:** 100%
*S36 Four Pillars complete (Port Authority, gRPC-First Transport, Log Aggregation, Service Discovery). S51 Auth Framework. S67 Wire Format Freeze. WEST + EAST bare metal online. Cross-host BPF flow graph operational.*

### Age 3: The Public Release (🔄 IN PROGRESS)
**Progress:** ~70%
**Status:** Public-launch thread stalled mid-flight; multi-wave forge research thread shipping at quality.

**Completed sub-items:**
- S67 Wire Format FROZEN at v0x01 (12 IANA registries, IPR clear)
- S78 LOC audit (415K production, 1,137K total)
- S-WEST + S-EAST bootstrap (both bare-metal hosts online)
- ADR Sweep 2026-04-03 (all 27 ADRs resolved)
- Zhen Champion (pkg/champion/, 18 tests)
- Zhen MCP Server (7 tools for Claude Code)
- 31 Operational Runbooks
- Service Template (pkg/service/, 7 tests)
- BPF CI Gate, Sealed Cask, Binding Rune verification
- ROCm GPU acceleration (PyTorch 2.5.1+rocm6.2)
- RAFT QA generation (~2000 pairs)
- WAVE10C Backprop fixes (3 bugs, 46/46 tests pass)
- Wotan Topic Signing (ML-DSA-65 enforcement)
- Mímir's Law / Gleipnir Phase 0 (2026-04-11, real-metal validated on EAST: zero false positives, 100% drift detection)
- WAVE10F Forge Real-Attention Gemma 4 (2026-04-17, 3000+ LOC, end-to-end Gemma-4 LoRA training)
- Learning Gate (2026-04-20, 4/5 strict experiments PASS, β = 0.27 generalization exponent)
- 24h Consolidation Block (2026-04-20, lr=1e-3 stable operating point identified)
- ADR-048 ForgeBackend Trait (2026-04-21, refactor centralizes Adam optimizer)
- WAVE11 GPU Kernels (4 attention grad kernels, cosine=1.000)
- WAVE12 Kingdom RAFT LoRA (2026-04-23, eval Δ −14.32, ADR-050 GPU-resident activations)
- WAVE13 Phase 1 (2026-04-25, generate-gemma4 subcommand, early quality finding: LoRA under-trained ~10× off target)
- Round Table verification audit (2026-04-27, 19 seats reported, 2 citations issued + cleared)
- WAVE13 Phase 2 quality verdict: **RETRAIN** (2026-04-28, autonomous overnight per Marshal charter; 0/8 LoRA-better, 6/8 immediate-stop, 2/8 mode-collapse `\tif`; ADR-051 Accepted; Phases 4-5 PAUSED until WAVE14 retrain)
- WAVE16 Multi-model selector + overnight model bench (2026-05-04, ADR-060 LIVE in sidebar; 5 model keys; qwen-coder-14b kept as option after passing 14-prompt textbook tier 0/14 truncated)
- WAVE17 K8s substrate proven (2026-05-05, autonomous overnight per Marshal charter; 9/9 services Running on 3-node kind; 2 real bugs found+fixed mid-run: monad service-link env collision + chart volume support; ADR-064 active/active spec landed but impl deferred per Stevie; 9-file doctrine sweep post-c6108fb8; cmd/tools/ scaffold for round-table P0 wedges; wotan main.go review +13 unit tests)

**Remaining for Age 3:**
- Captain Track A/B/C decision (Wed 2026-04-29)
- WAVE14 retrain (extended-epoch, rank=16/alpha=32 unchanged, ≥3 epochs ≈ 10704 example-steps; gated on Captain sign-off)
- Branch hygiene execution (3 stale branches, REMOTE for build/test)
- SBOM regen + license scan + threat refresh (REMOTE)
- Sophia draft-03 ship-or-defer
- Wotan draft-03 ship-or-defer
- Demo video + README polish for VC/public readiness
- Sub-50ms latency benchmark (Scientist falsifiability gate)
- Public accessibility (optional auth)

### Age 4: The MVP Era (📋 PLANNED)
*Track-call dependent. Activates after Age 3 public-launch gate.*
- WAVE14 BackwardScratch + KV-cache (gated on Track A or C)
- Performance benchmarking suite
- Customer onboarding flows
- Billing/metering
- Multi-tenant isolation hardening

### Age 5: The Scaling Era (📋 PLANNED)
*Long-horizon. Kingdom Mode rollout, multi-region, federation.*

---

*Synced: 2026-05-05 (autonomous overnight session per Marshal charter — WAVE17 K8s substrate + ADR-064 + doctrine sweep)*
*Next sync trigger: any commit to main + drift-guard CI check*
