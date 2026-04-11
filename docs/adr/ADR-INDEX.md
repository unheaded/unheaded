# ADR Index — Unheaded Architecture Decision Records

**Last updated:** 2026-04-11
**Total:** 45 ADRs (ADR-012b deprecated, ADR-027-044 across sessions)

## Status Summary

| Status | Count |
|--------|-------|
| Accepted | 25 |
| In Progress | 1 |
| Planned | 3 |
| Deferred | 4 |
| Pipe Dream | 1 |
| Deprecated | 1 |
| PoC / Research | 1 |

**Zero Draft/Proposed remaining.**

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
