# Architecture Decision Records

**Last synced from canonical:** 2026-05-05 (65 ADRs)

Mirror of [docs/adr/ADR-INDEX.md](../docs/adr/ADR-INDEX.md) — the canonical source. The wiki page is regenerated periodically from it; if they diverge, trust the canonical.

| ADR | Title | Status |
|-----|-------|--------|
| [001](../docs/adr/ADR-001-gnostic-state-management.md) | Gnostic State Management | Accepted |
| [002](../docs/adr/ADR-002-kingdom-naming-convention.md) | Kingdom Naming Convention | Accepted |
| [003](../docs/adr/ADR-003-ebpf-rust-aya-framework.md) | eBPF in Rust with Aya | Accepted |
| [004](../docs/adr/ADR-004-no-external-deps-policy.md) | Dependency Policy (Approved Exceptions) | Accepted |
| [005](../docs/adr/ADR-005-wotan-message-backbone.md) | Wotan Message Backbone | Accepted |
| [006](../docs/adr/ADR-006-vanilla-js-frontend.md) | Vanilla JS Frontend | Accepted |
| [007](../docs/adr/ADR-007-container-hardening-strategy.md) | Container Hardening | Accepted |
| [008](../docs/adr/ADR-008-security-hardening-baseline.md) | Security Hardening Baseline | Accepted |
| [009](../docs/adr/ADR-009-parish-boundaries.md) | Parish Boundaries (Binding Circles) | **Deferred to Beta** |
| [010](../docs/adr/ADR-010-sealed-cask-deployment.md) | Sealed Cask Deployment | Accepted (Alpha) |
| [011](../docs/adr/ADR-011-storage-layer-planning.md) | Storage Layer Planning | Accepted (Alpha) |
| [012](../docs/adr/ADR-012-bpf-verifier-risk-mitigation.md) | BPF Verifier Risk Mitigation | Accepted + CI Gate |
| [012b](../docs/adr/ADR-012-the-well-postgres.md) | ~~The Well — PostgreSQL~~ | Deprecated → ADR-016 |
| [013](../docs/adr/ADR-013-routing-header-support.md) | IPv6 Routing Header | Accepted (Deferred) |
| [014](../docs/adr/ADR-014-ipv6-fragmentation-support.md) | IPv6 Fragmentation | Accepted (Deferred) |
| [015](../docs/adr/ADR-015-go-fiber-http-layer.md) | Go-Fiber HTTP Layer | Accepted + Template |
| [016](../docs/adr/ADR-016-postgres-the-well.md) | PostgreSQL — The Well | Accepted |
| [017](../docs/adr/ADR-017-zhen-hybrid-inference.md) | Zhen Hybrid Inference | Accepted (Claude API → MCP) |
| [018](../docs/adr/ADR-018-zhen-raft-training-battle-plan.md) | RAFT Training Pipeline | **In Progress** |
| [019](../docs/adr/ADR-019-zhen-champion-agent.md) | Zhen Champion Agent | Accepted (Phase 1+2) |
| [020](../docs/adr/ADR-020-kanban-bug-fixes.md) | Kanban Bug Fixes | Accepted (All 8 fixed) |
| [021](../docs/adr/ADR-021-zhen-layer0-substrate.md) | Zhen Layer 0 Substrate | Accepted (133 tests) |
| [022](../docs/adr/ADR-022-pihole-docker-to-lxd.md) | Pi-hole Docker → LXD | Accepted (runbook ready) |
| [023](../docs/adr/ADR-023-wotan-virtual-memory.md) | Wotan Virtual Memory | **Deferred to L5** |
| [024](../docs/adr/ADR-024-zhen-runbook-automation.md) | Runbook Automation | Accepted (31 runbooks) |
| [025](../docs/adr/ADR-025-kanban-mobile-app.md) | Kanban Mobile App | **Pipe Dream (Age 4+)** |
| [026](../docs/adr/ADR-026-deb-packaging-ci-pipeline.md) | .deb Packaging + CI/CD | Accepted |
| [027](../docs/adr/ADR-027-zhenai-conversation-memory.md) | Zhenai Conversation Memory | Accepted (semantic recall + browser) |
| [028](../docs/adr/ADR-028-zhenai-scheduler-heartbeat.md) | Zhenai Scheduler + Heartbeat | Accepted (Sprint C complete) |
| [029](../docs/adr/ADR-029-wotan-consensus-health-remediation.md) | Wotan Consensus Health (Akira) | Accepted (7/7 EAST, Wotan publishing) |
| [030](../docs/adr/ADR-030-zhenai-forge-rust-training.md) | Zhenai Forge (Rust LoRA training) | **In Progress — v3 training (cosine LR)** |
| [031](../docs/adr/ADR-031-hybrid-model-handoff.md) | Hybrid Model Handoff | Planned |
| [032](../docs/adr/ADR-032-python-to-go-rust-migration.md) | Python → Go/Rust Migration | Planned |
| [033](../docs/adr/ADR-033-netbox-ipam.md) | NetBox IPAM | Planned |
| [034](../docs/adr/ADR-034-grpc-mtls-default-transport.md) | gRPC mTLS Default Transport | Accepted |
| [035](../docs/adr/ADR-035-wotan-active-passive-redundancy.md) | ~~Wotan Active-Passive Redundancy~~ | **Superseded by ADR-064** (2026-05-05) |
| [036](../docs/adr/ADR-036-claude-distillation-training.md) | Claude Distillation Training Data | **In Progress** |
| [037](../docs/adr/ADR-037-zhenai-unified-champion.md) | Zhenai Unified Champion (The Armor) | Planned |
| [038](../docs/adr/ADR-038-kanban-git-audit-trail.md) | Kanban GUID → Git Commit Audit | Accepted |
| [69420](../docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md) | BGP Sleipnir + Unheaded OS | **Deferred to Q4 2026** |
| [039](../docs/adr/ADR-039-cs-precision-reference-service.md) | CS Precision Reference Service | In Progress |
| [040](../docs/adr/ADR-040-kubernetes-ecosystem-strategy.md) | Kubernetes Ecosystem Strategy | Planned |
| [041](../docs/adr/ADR-041-kanban-timeline-sync.md) | Kanban ↔ Timeline Bidirectional Sync | Planned |
| [042](../docs/adr/ADR-042-cs-blackmage-lich-integration.md) | CS → BlackMage Skill + Lich Security | Planned |
| [043](../docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md) | Mímir's Law: UPC-Controlled Baseline (Gleipnir Phase 0 PoC) | **PoC / Research** |
| [044](../docs/adr/ADR-044-kanban-task-detail-status-query.md) | Kanban Task Detail Status Query (git log + docs + checklist) | **PIPE DREAM** |
| [045](../docs/adr/ADR-045-wave-10d-gpu-backward-first-real-training.md) | Wave 10D — GPU Backward + First Real Training Run | **PLANNED** |
| [046](../docs/adr/ADR-046-upc-perservice-ingress-egress-inspection.md) | UPC Per-Service Ingress/Egress Inspection (Anti-Fake-Horn) | **PIPE DREAM** |
| [047](../docs/adr/ADR-047-k8s-honest-assessment-and-extractable-tools.md) | K8s Honest Assessment + Extractable Tools + East/West K8s Lab | **ACKNOWLEDGED** |
| [048](../docs/adr/ADR-048-forge-backend-trait.md) | ForgeBackend Trait — Pluggable Kernel Provider for zhenai-forge | Accepted |
| [049](../docs/adr/ADR-049-wave11-gpu-kernels-for-gemma4-training.md) | WAVE11 — Custom HIP Kernels for Gemma-4 Training at seq=384 | Accepted |
| [050](../docs/adr/ADR-050-wave12-gpu-resident-activations.md) | WAVE12 — GPU-Resident Forge Activations + Kingdom RAFT LoRA | Accepted |
| [051](../docs/adr/ADR-051-wave13-generate-path.md) | WAVE13 — Forge generate-gemma4 path + KV-cache deferral | **Accepted** (verdict: RETRAIN) |
| [052](../docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md) | Timeline & Battle-Plan Source-of-Truth Policy (drift ≤ 7 days, in-tree only) | **Accepted** (CI gate live) |
| [053](../docs/adr/ADR-053-hybrid-claude-zhenai-workflow-templates.md) | Hybrid Claude + Local Zhenai Workflow Templates (cost-driven hybrid routing) | **Pipe Dream** (activates on cost/quality/strategic/personal trigger) |
| [055](../docs/adr/ADR-055-kev-poller-always-on-service.md) | KEV Poller — Always-On Kingdom Service + K8s Cross-Host Pilot (CISA KEV + NIST NVD parity) | **Planned** (Phase 0-5; first cross-host K8s workload) |
| [056](../docs/adr/ADR-056-pgvector-auxiliary-corpus-sharding.md) | pgvector Auxiliary Corpus Sharding for Trust-Tagged Retrieval (Wikipedia / Stack Overflow / RFCs / source code) | **Proposed** (gated on WAVE15 H0 passing) |
| [057](../docs/adr/ADR-057-unheaded-source-code-indexing.md) | Unheaded Source Code Indexing for Semantic Retrieval (AST chunks + code-specialized embedder, first instance of ADR-056 pattern) | **Proposed** (gated on WAVE15 H0 + ADR-056 acceptance) |
| [058](../docs/adr/ADR-058-gcp-cost-alarm-bellis-tech.md) | GCP Cost & API Utilization Alarms for bellis.tech (EDoS / cost-amplification DoS defense — free-tier project) | **Planned** (~30 min of console work; no code dependencies) |
| [059](../docs/adr/ADR-059-zhenai-interactive-cli.md) | Zhenai Interactive CLI (`cmd/zhen-cli`) — terminal REPL counterpart of the web UI; same Champion-gated mutation path | **Phase 1 Shipped** (2026-05-03; Phase 2-3 Planned) |
| [060](../docs/adr/ADR-060-zhenai-multi-model-selector.md) | Zhenai Multi-Model Selector — UI dropdown + Champion-gated `model_switch` handler; T11-T20 threat catalog + LICH-013 pre-registered | **In Progress** (activated mid-WAVE16; implementation 2026-05-04 overnight) |
| [061](../docs/adr/ADR-061-cloud-rented-training-purpose-built-coding-model.md) | Cloud-Rented Training for Purpose-Built Unheaded Coding Model — pivot from off-the-shelf search to bespoke LoRA on rented A100/H100 | **Research / Pipe Dream** (~$5-50 budget when activated; gated on WAVE13 RETRAIN clearing first) |
| [062](../docs/adr/ADR-062-fuzz-redteam-pentest-framework.md) | Fuzz / Red-Team / Pentest Framework (Lich Codified) — three-tier offensive taxonomy, numbered LICH-NNN campaigns, activation rules | **Accepted** (existing pattern ratified; new campaigns slot in) |
| [063](../docs/adr/ADR-063-akira-summons-lich-randomized-ctf.md) | Akira Randomly Summons the Lich for CTF Mode — bounded, opt-in, randomized adversarial probing of live Kingdom services | **Pipe Dream** (activates after ADR-062's framework has 3 campaigns clean for 7 days) |
| [064](../docs/adr/ADR-064-wotan-active-active-cluster-k8s-native.md) | Wotan Active/Active Cluster — 3-node minimum, K8s-native StatefulSet, Raft membership + topic-leader election, broadcast replication. Supersedes ADR-035 ("active passive works but doesn't scale") | **Proposed** (4-phase rollout; ADR-035 stays running through cutover) |

---

> **Source:** [docs/adr/ADR-INDEX.md](../docs/adr/ADR-INDEX.md)
