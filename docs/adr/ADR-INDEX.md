# ADR Index — Unheaded Architecture Decision Records

**Last updated:** 2026-08-03
**Total:** 76 ADRs (ADR-012b deprecated; ADR-035 superseded by ADR-064; ADR-065 superseded by Phase A finding; ADR-027-065 across sessions; ADR-066/067/072/073 added; ADR-69420 pipe-dream; ADR-084 huginn; ADR-088 kubernetes; ADR-089 promotion workflow; ADR-090 total source sweep; ADR-091 The Well initdb ordering)

> **Index gap:** ADR-082 and ADR-083 exist on disk but have never been registered here
> or in the table below. Noted 2026-08-03; needs a Librarian pass.

**ADR-084 added 2026-07-24:** Huginn host metrics agent — renames `cmd/host-agent` → `cmd/huginn`, establishes naming, systemd units in `deploy/systemd/`.
**ADR-085 added 2026-07-24:** CI/CD artifact layout — canonical `/var/` hierarchy for binaries, data, config, APT repo; 4-phase implementation plan (binary install → .deb → CI pipeline → registry).
**ADR-086 added 2026-07-24:** Muninn observability fan-out pipeline — routes host metrics, logs, and auth events to VictoriaMetrics, PostgreSQL, and SIEM; SSH/TTY login history to `ops.login_events`; YAML-configured routing rules.
**ADR-087 added 2026-07-24:** NOC — network device monitoring and config management. Kvasir (IPFIX/NetFlow/sFlow collector wrapping GoFlow2), Muninn syslog extension, Huginn SNMP extension, Ansible+NETCONF/YANG GitOps for Junos. JNCIA lab alignment.
**ADR-088 added 2026-07-24:** Kingdom deployment substrate ladder — Compose+systemd+NixOS+Ansible+Terraform+K8s+.deb/apt as additive, optional substrates on top of the custom protocol stack. K8s DaemonSet/StatefulSet/Deployment targets, Ansible roles, apt repo, NixOS modules; certification alignment table (CKA, JNCIA, Terraform associate, RHCSA).
**ADR-089 added 2026-08-03:** `develop` → `staging` → `main` promotion workflow — three long-lived branches, each answering a different question (does it work / does it break anything else / do we stand behind it). One-reviewable-unit-per-commit rule, EAST as the staging deploy target, never-promote-red, shrink-only baselines, revert-not-force-push rollback. **Accepted.**
**ADR-090 added 2026-08-03:** The Total Source Sweep — read all 1,836 tracked source files exactly once, in fixed order, recording a per-file verdict in a resumable TSV ledger. Rubric for what counts as slop, and an explicit "NOT slop" list (defensive checks, tests, clarity, why-comments) so a LOC-reduction campaign cannot damage the tree. **Proposed** — activates after the Staging Ladder sprint.

**ADR-091 added 2026-08-04:** The Well — `/docker-entrypoint-initdb.d` holds exactly one file, the `db/init.sh` orchestrator; migrations mount read-only at `/migrations`. Mounting both made postgres's entrypoint run every `*.sql` itself against `POSTGRES_DB`, so the per-database schemas landed in the wrong database and init died on the first collision (container exit 3, `init.sh` never ran). Nesting the mounts also wrote a root-owned `init.sh` into the host's `db/migrations/`. Now matches the Kubernetes ConfigMap, which already mounted only the script. **Accepted.**

## Status Summary

| Status | Count |
|--------|-------|
| Accepted | 33 |
| In Progress | 4 |
| Planned | 9 |
| Proposed | 3 |
| Deferred | 5 |
| Pipe Dream | 4 |
| PoC / Research | 2 |
| Acknowledged | 1 |
| Phase 1 Shipped | 1 |
| Deprecated | 1 |
| Superseded | 1 |

**Four new ADRs added 2026-04-27 from Round Table sprint follow-through:**
- ADR-051 (WAVE13 generate path) — Draft, pending Phase 2 verdict
- ADR-052 (drift / source-of-truth policy) — **Accepted** (CI guard + Jenkins stage shipped same patch)
- ADR-053 (Hybrid Claude + Local Zhenai workflow templates) — **Pipe Dream** (cost-driven; per Stevie's mid-session note)
- ADR-055 (KEV Poller — always-on Kingdom service + K8s cross-host pilot) — **Planned** (per Stevie's mid-session directive)

**Three new ADRs added 2026-05-02 from WAVE15 rewire planning session + security-posture review:**
- ADR-056 (pgvector auxiliary corpus sharding) — **Proposed** (gated on WAVE15 H0 passing; the architectural pattern for non-vor retrieval)
- ADR-057 (Unheaded source code indexing) — **Proposed** (gated on WAVE15 H0 + ADR-056 acceptance; first concrete instance of the pattern — AST-chunked, code-embedder-tuned semantic retrieval over our own source tree)
- ADR-058 (GCP cost & API utilization alarms for bellis.tech) — **Planned** (EDoS / cost-amplification DoS defense for free-tier hosted personal site; ~30 min of console work, no code dependencies; activation triggered by Stevie scheduling the time)
- ADR-059 (Zhenai interactive CLI) — **Planned** (`cmd/zhen-cli` terminal REPL counterpart of the web UI; chat + slash commands for runbook execute / source view / memory / recall; mutation paths inherit T6b closure via `/api/v1/tool/exec`; ~1-2 days when scheduled)

**Four new ADRs added 2026-05-04 from WAVE16 model bench + rented-training pivot + Lich codification:**
- ADR-060 (Zhenai multi-model selector) — **In Progress** (UI dropdown + Champion-gated `model_switch`; T11-T20 threat catalog + LICH-013 pre-registered; live in sidebar same overnight)
- ADR-061 (cloud-rented training for purpose-built Unheaded coding model) — **Research / Pipe Dream** (~$5-50 budget; gated on WAVE13 RETRAIN clearing first)
- ADR-062 (Fuzz / Red-Team / Pentest Framework) — **Accepted** (Lich codified; three-tier offensive taxonomy, numbered LICH-NNN campaigns, activation rules)
- ADR-063 (Akira summons Lich for randomized CTF mode) — **Pipe Dream** (activates after ADR-062's framework has 3 campaigns clean for 7 days)

**One new ADR added 2026-05-05 from WAVE17 K8s substrate run + active/active design discussion:**
- ADR-064 (Wotan Active/Active Cluster, K8s-native) — **Proposed** (3-node minimum, Raft membership + topic-leader election, broadcast replication; supersedes ADR-035; impl deferred per Stevie's "active/active on hold as ADR for now")

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
| [035](ADR-035-wotan-active-passive-redundancy.md) | ~~Wotan Active-Passive Redundancy~~ | **Superseded by ADR-064** (2026-05-05) | 2026-04-05 |
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
| [059](ADR-059-zhenai-interactive-cli.md) | Zhenai Interactive CLI (`cmd/zhen-cli`) — terminal REPL counterpart of the web UI; same Champion-gated mutation path | **Phase 1 Shipped** (2026-05-03; Phase 2-3 Planned) | 2026-05-02 |
| [060](ADR-060-zhenai-multi-model-selector.md) | Zhenai Multi-Model Selector — UI dropdown + Champion-gated `model_switch` handler; T11-T20 threat catalog + LICH-013 pre-registered | **In Progress** (activated mid-WAVE16; implementation 2026-05-04 overnight) | 2026-05-04 |
| [061](ADR-061-cloud-rented-training-purpose-built-coding-model.md) | Cloud-Rented Training for Purpose-Built Unheaded Coding Model — pivot from off-the-shelf search to bespoke LoRA on rented A100/H100 | **Research / Pipe Dream** (~$5-50 budget when activated; gated on WAVE13 RETRAIN clearing first) | 2026-05-04 |
| [062](ADR-062-fuzz-redteam-pentest-framework.md) | Fuzz / Red-Team / Pentest Framework (Lich Codified) — three-tier offensive taxonomy, numbered LICH-NNN campaigns, activation rules | **Accepted** (existing pattern ratified; new campaigns slot in) | 2026-05-04 |
| [063](ADR-063-akira-summons-lich-randomized-ctf.md) | Akira Randomly Summons the Lich for CTF Mode — bounded, opt-in, randomized adversarial probing of live Kingdom services | **Pipe Dream** (activates after ADR-062's framework has 3 campaigns clean for 7 days) | 2026-05-04 |
| [064](ADR-064-wotan-active-active-cluster-k8s-native.md) | Wotan Active/Active Cluster — 3-node minimum, K8s-native StatefulSet, Raft membership + topic-leader election, broadcast replication. Supersedes ADR-035 ("active passive works but doesn't scale") | **Proposed** (4-phase rollout; ADR-035 stays running through cutover) | 2026-05-05 |
| [065](ADR-065-aya-major-version-migration.md) | aya 0.1.x → 0.13.x Major-Version Migration Plan | **Superseded** (Phase A finding: aya splits userspace/kernel independently; no migration needed) | 2026-05-07 |
| [066](ADR-066-tonic-minor-bump-trace-collector.md) | `cmd/trace-collector` tonic 0.10 → 0.12 — closes 4 CVE-class advisories in rustls-webpki + protobuf chain | **Accepted** | 2026-05-08 |
| [067](ADR-067-mbc-isa-v2-and-upc-abi-v1.md) | MBC ISA v2 + UPC ABI v1 freeze — 5 new opcodes (FENCE/MRET/SRET/LR.W/SC.W), priv_level field, Boot Protocol v2, memory-mapped CSR region | **Accepted** (Phase 0 ASCEND-LINUX gate) | 2026-05-08 |
| [072](ADR-072-boot-magic-byte-ordering.md) | BOOT_MAGIC byte-ordering convention — canonical hex 0x554E4844 ('UNHD' MSB-first); wire bytes 'D','H','N','U' per LE | **Accepted** (doc-only clarification) | 2026-05-10 |
| [073](ADR-073-lint-policy-zero-findings.md) | Lint Policy: Zero Findings as the New Floor — triage protocol; 12 CVE-class fixes shipped during the 2026-05-11 drain | **Accepted** (`golangci-lint run ./...` returns 0 issues; CI ratchet) | 2026-05-11 |
| [074](ADR-074-phase12-page-table-model.md) | ASCEND-LINUX Phase 1.2 Page-Table Model — Option A per-pid pgd, VA[0,8MiB) → disjoint physical slices via two 4 MiB superpages | **Accepted** (Phase 1.2 IMPL closed 2026-05-13) | 2026-05-11 |
| [075](ADR-075-phase13-process-model.md) | ASCEND-LINUX Phase 1.3 Process Model — PROC_TABLE 4→8, ZOMBIE exit, LR.W/SC.W reservation-clear, RV2MBC SHA-256 gate | **Accepted** | 2026-05-13 |
| [076](ADR-076-topology-map-single-source-of-truth.md) | Single Source of Truth for the Live Topology Map | **Proposed** (planning) | 2026-06-02 |
| [077](ADR-077-phase17-rv2mbc-base-feature-gated-abi-fork.md) | ASCEND-LINUX Phase 1.7 Gate B — feature-gated MbcCpuState ABI fork; per-process rv2mbc_base routes each pid's indirect branches (exec → live `$`) | **Accepted** (shipped 2026-06-19) | 2026-06-18 |
| [078](ADR-078-gate-d-fs-reader-and-exec-argv-abi.md) | ASCEND-LINUX Phase 1.7 Gate D — in-BPF FS reader (Design A: open/read/fstat walk fs.img inodes directly), ilp32e struct-stat truth, exec argv frame ABI at VA 0x600000, a2/a3 re-entry register protocol | **Accepted** (shipped 2026-07-02) | 2026-07-02 |
| [079](ADR-079-ret-address-tagging.md) | RET return-address tagging — CALL/CALLR tag MBC PCs with bit 31 so RET distinguishes them from raw RV addresses without the fragile magnitude floor; retires RET_RV_FLOOR, unblocks Linux-scale images, makes the Doom [0x10000,0x151BF] misparse impossible (verifier +0.03%) | **Accepted** (Phase 2.0 substrate prereq #1; xv6 corpus green 2026-07-02) | 2026-07-02 |
| [080](ADR-080-upc-programmatic-api.md) | The UPC Programmatic API — UPC as a general multi-workload MBC substrate (Doom + xv6/Linux on one interpreter); the 5-part guest contract (MBC image / rv2mbc / memory / syscall surface / boot protocol); per-workload syscall surfaces DCE-partitioned by cfg to respect the 1M verifier ceiling; load-test-each-variant rule; path to registered dispatch | **Accepted** (formalizes realized architecture) | 2026-07-03 |
| [081](ADR-081-unheaded-linux-from-scratch.md) | Unheaded Linux — build our own minimal OS from scratch on the UPC instead of vendoring uClinux+busybox; 100% in-house, as light/small as possible (fits verifier budget + code-store); scales UPC PoC → Unheaded Linux → Yggdrasil golden image via soft-fork discipline on an owned base. Supersedes ascend Phase 2 vendor step; moots the LTS/arch pair calls | **Accepted** (Stevie's call) | 2026-07-03 |
