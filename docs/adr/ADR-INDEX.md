# ADR Index — Unheaded Architecture Decision Records

**Last updated:** 2026-05-02
**Total:** 55 ADRs (ADR-012b deprecated, ADR-027-058 across sessions; ADR-69420 pipe-dream)

## Status Summary

| Status | Count |
|--------|-------|
| Accepted | 28 |
| Draft | 1 |
| In Progress | 1 |
| Planned | 5 |
| Proposed | 2 |
| Deferred | 4 |
| Pipe Dream | 2 |
| Deprecated | 1 |
| PoC / Research | 1 |

**Four new ADRs added 2026-04-27 from Round Table sprint follow-through:**
- ADR-051 (WAVE13 generate path) — Draft, pending Phase 2 verdict
- ADR-052 (drift / source-of-truth policy) — **Accepted** (CI guard + Jenkins stage shipped same patch)
- ADR-053 (Hybrid Claude + Local Zhenai workflow templates) — **Pipe Dream** (cost-driven; per Stevie's mid-session note)
- ADR-055 (KEV Poller — always-on Kingdom service + K8s cross-host pilot) — **Planned** (per Stevie's mid-session directive)

**Three new ADRs added 2026-05-02 from WAVE15 rewire planning session + security-posture review:**
- ADR-056 (pgvector auxiliary corpus sharding) — **Proposed** (gated on WAVE15 H0 passing; the architectural pattern for non-vor retrieval)
- ADR-057 (Unheaded source code indexing) — **Proposed** (gated on WAVE15 H0 + ADR-056 acceptance; first concrete instance of the pattern — AST-chunked, code-embedder-tuned semantic retrieval over our own source tree)
- ADR-058 (GCP cost & API utilization alarms for bellis.tech) — **Planned** (EDoS / cost-amplification DoS defense for free-tier hosted personal site; ~30 min of console work, no code dependencies; activation triggered by Stevie scheduling the time)

**ADR-054 RESERVED** for WAVE14 BackwardScratch + KV-cache when Track A or C activates.

## Full Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001](ADR-001-gnostic-state-management.md) | Gnostic State Management | Accepted | 2026-02-01 |
| [002](ADR-002-kingdom-naming-convention.md) | Kingdom Naming Convention | Accepted | 2026-01-28 |
| [003](ADR-003-ebpf-rust-aya-framework.md) | eBPF in Rust with Aya | Accepted | 2026-01-26 |
| [004](ADR-004-no-external-deps-policy.md) | Dependency Policy (Approved Exceptions) | Accepted | 2026-01-26 |
| [005](ADR-005-wotan-message-backbone.md) | Wotan Message Backbone | Accepted | 2026-01-26 |
| [006](ADR-006-vanilla-js-frontend.md) | Vanilla JS Frontend | Accepted | 2026-01-26 |
| [007](ADR-007-container-hardening-strategy.md) | Container Hardening | Accepted | 2026-01-26 |
| [008](ADR-008-security-hardening-baseline.md) | Security Hardening Baseline | Accepted | 2026-02-16 |
| [009](ADR-009-parish-boundaries.md) | Parish Boundaries (Binding Circles) | **Deferred to Beta** | 2026-02-18 |
| [010](ADR-010-sealed-cask-deployment.md) | Sealed Cask Deployment | Accepted (Alpha) | 2026-02-18 |
| [011](ADR-011-storage-layer-planning.md) | Storage Layer Planning | Accepted (Alpha) | 2026-02-18 |
| [012](ADR-012-bpf-verifier-risk-mitigation.md) | BPF Verifier Risk Mitigation | Accepted + CI Gate | 2026-02-19 |
| [012b](ADR-012-the-well-postgres.md) | ~~The Well — PostgreSQL~~ | Deprecated → ADR-016 | 2026-02-19 |
| [013](ADR-013-routing-header-support.md) | IPv6 Routing Header | Accepted (Deferred) | 2026-02-20 |
| [014](ADR-014-ipv6-fragmentation-support.md) | IPv6 Fragmentation | Accepted (Deferred) | 2026-02-20 |
| [015](ADR-015-go-fiber-http-layer.md) | Go-Fiber HTTP Layer | Accepted + Template | 2026-02-20 |
| [016](ADR-016-postgres-the-well.md) | PostgreSQL — The Well | Accepted | 2026-03-15 |
| [017](ADR-017-zhen-hybrid-inference.md) | Zhen Hybrid Inference | Accepted (Claude API → MCP) | 2026-03-16 |
| [018](ADR-018-zhen-raft-training-battle-plan.md) | RAFT Training Pipeline | **In Progress** | 2026-03-16 |
| [019](ADR-019-zhen-champion-agent.md) | Zhen Champion Agent | Accepted (Phase 1+2) | 2026-03-16 |
| [020](ADR-020-kanban-bug-fixes.md) | Kanban Bug Fixes | Accepted (All 8 fixed) | 2026-03-19 |
| [021](ADR-021-zhen-layer0-substrate.md) | Zhen Layer 0 Substrate | Accepted (133 tests) | 2026-03-28 |
| [022](ADR-022-pihole-docker-to-lxd.md) | Pi-hole Docker → LXD | Accepted (runbook ready) | 2026-03-29 |
| [023](ADR-023-wotan-virtual-memory.md) | Wotan Virtual Memory | **Deferred to L5** | 2026-03-30 |
| [024](ADR-024-zhen-runbook-automation.md) | Runbook Automation | Accepted (31 runbooks) | 2026-04-02 |
| [025](ADR-025-kanban-mobile-app.md) | Kanban Mobile App | **Pipe Dream (Age 4+)** | 2026-04-03 |
| [026](ADR-026-deb-packaging-ci-pipeline.md) | .deb Packaging + CI/CD | Accepted | 2026-04-03 |
| [027](ADR-027-zhenai-conversation-memory.md) | Zhenai Conversation Memory | Accepted (semantic recall + browser) | 2026-04-04 |
| [028](ADR-028-zhenai-scheduler-heartbeat.md) | Zhenai Scheduler + Heartbeat | Accepted (Sprint C complete) | 2026-04-04 |
| [029](ADR-029-wotan-consensus-health-remediation.md) | Wotan Consensus Health (Akira) | Accepted (7/7 EAST, Wotan publishing) | 2026-04-04 |
| [030](ADR-030-zhenai-forge-rust-training.md) | Zhenai Forge (Rust LoRA training) | **In Progress — v3 training (cosine LR)** | 2026-04-04 |
| [031](ADR-031-hybrid-model-handoff.md) | Hybrid Model Handoff | Planned | 2026-04-03 |
| [032](ADR-032-python-to-go-rust-migration.md) | Python → Go/Rust Migration | Planned | 2026-04-03 |
| [033](ADR-033-netbox-ipam.md) | NetBox IPAM | Planned | 2026-04-03 |
| [034](ADR-034-grpc-mtls-default-transport.md) | gRPC mTLS Default Transport | Accepted | 2026-04-02 |
| [035](ADR-035-wotan-active-passive-redundancy.md) | Wotan Active-Passive Redundancy | Accepted (Phases 0-2 done) | 2026-04-05 |
| [036](ADR-036-claude-distillation-training.md) | Claude Distillation Training Data | **In Progress** | 2026-04-05 |
| [037](ADR-037-zhenai-unified-champion.md) | Zhenai Unified Champion (The Armor) | Planned | 2026-04-05 |
| [038](ADR-038-kanban-git-audit-trail.md) | Kanban GUID → Git Commit Audit | Accepted | 2026-04-05 |
| [69420](ADR-69420-kingdom-bgp-and-unheaded-os.md) | BGP Sleipnir + Unheaded OS | **Deferred to Q4 2026** | 2026-03-18 |
| [039](ADR-039-cs-precision-reference-service.md) | CS Precision Reference Service | In Progress | 2026-04-05 |
| [040](ADR-040-kubernetes-ecosystem-strategy.md) | Kubernetes Ecosystem Strategy | Planned | 2026-04-05 |
| [041](ADR-041-kanban-timeline-sync.md) | Kanban ↔ Timeline Bidirectional Sync | Planned | 2026-04-05 |
| [042](ADR-042-cs-blackmage-lich-integration.md) | CS → BlackMage Skill + Lich Security | Planned | 2026-04-05 |
| [043](ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md) | Mímir's Law: UPC-Controlled Baseline (Gleipnir Phase 0 PoC) | **PoC / Research** | 2026-04-08 |
| [044](ADR-044-kanban-task-detail-status-query.md) | Kanban Task Detail Status Query (git log + docs + checklist) | **PIPE DREAM** | 2026-04-11 |
| [045](ADR-045-wave-10d-gpu-backward-first-real-training.md) | Wave 10D — GPU Backward + First Real Training Run | **PLANNED** | 2026-04-11 |
| [046](ADR-046-upc-perservice-ingress-egress-inspection.md) | UPC Per-Service Ingress/Egress Inspection (Anti-Fake-Horn) | **PIPE DREAM** | 2026-04-11 |
| [047](ADR-047-k8s-honest-assessment-and-extractable-tools.md) | K8s Honest Assessment + Extractable Tools + East/West K8s Lab | **ACKNOWLEDGED** | 2026-04-11 |
| [048](ADR-048-forge-backend-trait.md) | ForgeBackend Trait — Pluggable Kernel Provider for zhenai-forge | Accepted | 2026-04-21 |
| [049](ADR-049-wave11-gpu-kernels-for-gemma4-training.md) | WAVE11 — Custom HIP Kernels for Gemma-4 Training at seq=384 | Accepted | 2026-04-21 |
| [050](ADR-050-wave12-gpu-resident-activations.md) | WAVE12 — GPU-Resident Forge Activations + Kingdom RAFT LoRA | Accepted | 2026-04-23 |
| [051](ADR-051-wave13-generate-path.md) | WAVE13 — Forge generate-gemma4 path + KV-cache deferral | **Accepted** (verdict: RETRAIN) | 2026-04-28 |
| [052](ADR-052-timeline-and-battleplan-source-of-truth.md) | Timeline & Battle-Plan Source-of-Truth Policy (drift ≤ 7 days, in-tree only) | **Accepted** (CI gate live) | 2026-04-27 |
| [053](ADR-053-hybrid-claude-zhenai-workflow-templates.md) | Hybrid Claude + Local Zhenai Workflow Templates (cost-driven hybrid routing) | **Pipe Dream** (activates on cost/quality/strategic/personal trigger) | 2026-04-27 |
| [055](ADR-055-kev-poller-always-on-service.md) | KEV Poller — Always-On Kingdom Service + K8s Cross-Host Pilot (CISA KEV + NIST NVD parity) | **Planned** (Phase 0-5; first cross-host K8s workload) | 2026-04-27 |
| [056](ADR-056-pgvector-auxiliary-corpus-sharding.md) | pgvector Auxiliary Corpus Sharding for Trust-Tagged Retrieval (Wikipedia / Stack Overflow / RFCs / source code) | **Proposed** (gated on WAVE15 H0 passing) | 2026-05-02 |
| [057](ADR-057-unheaded-source-code-indexing.md) | Unheaded Source Code Indexing for Semantic Retrieval (AST chunks + code-specialized embedder, first instance of ADR-056 pattern) | **Proposed** (gated on WAVE15 H0 + ADR-056 acceptance) | 2026-05-02 |
| [058](ADR-058-gcp-cost-alarm-bellis-tech.md) | GCP Cost & API Utilization Alarms for bellis.tech (EDoS / cost-amplification DoS defense — free-tier project) | **Planned** (~30 min of console work; no code dependencies) | 2026-05-02 |
