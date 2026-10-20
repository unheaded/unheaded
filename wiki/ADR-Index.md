# Architecture Decision Records

**Last synced from canonical:** 2026-10-15 (99 entries)

Mirror of [docs/adr/ADR-INDEX.md](../docs/adr/ADR-INDEX.md) — the canonical source. If they diverge, trust the canonical.

| Status | Count |
|--------|-------|
| Accepted | 48 |
| Proposed | 14 |
| Planned | 14 |
| In Progress | 5 |
| Deferred | 5 |
| Pipe Dream | 5 |
| PoC / Research | 3 |
| Superseded / Deprecated | 4 |
| Acknowledged | 1 |

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001](../docs/adr/ADR-001-gnostic-state-management.md) | [Gnostic State Management](ADR-001-Gnostic-State-Management) | Accepted | 2026-02-01 |
| [002](../docs/adr/ADR-002-kingdom-naming-convention.md) | [Kingdom Naming Convention](ADR-002-Kingdom-Naming-Convention) | Accepted | 2026-01-28 |
| [003](../docs/adr/ADR-003-ebpf-rust-aya-framework.md) | [eBPF in Rust with Aya](ADR-003-eBPF-Rust-Aya-Framework) | Accepted | 2026-01-26 |
| [004](../docs/adr/ADR-004-no-external-deps-policy.md) | [Dependency Policy (Approved Exceptions) — gate amended by ADR-096; register audited 2026-09-24](ADR-004-No-External-Deps-Policy) | Accepted | 2026-01-26 |
| [005](../docs/adr/ADR-005-wotan-message-backbone.md) | [Wotan Message Backbone](ADR-005-Wotan-Message-Backbone) | Accepted | 2026-01-26 |
| [006](../docs/adr/ADR-006-vanilla-js-frontend.md) | [Vanilla JS Frontend](ADR-006-Vanilla-JS-Frontend) | Accepted | 2026-01-26 |
| [007](../docs/adr/ADR-007-container-hardening-strategy.md) | [Container Hardening](ADR-007-Container-Hardening-Strategy) | Accepted | 2026-01-26 |
| [008](../docs/adr/ADR-008-security-hardening-baseline.md) | [Security Hardening Baseline](ADR-008-Security-Hardening-Baseline) | Accepted | 2026-02-16 |
| [009](../docs/adr/ADR-009-parish-boundaries.md) | [Parish Boundaries (Binding Circles)](ADR-009-Parish-Boundaries) | **Deferred to Beta** | 2026-02-18 |
| [010](../docs/adr/ADR-010-sealed-cask-deployment.md) | [Sealed Cask Deployment](ADR-010-Sealed-Cask-Deployment) | Accepted (Alpha) | 2026-02-18 |
| [011](../docs/adr/ADR-011-storage-layer-planning.md) | [Storage Layer Planning](ADR-011-Storage-Layer-Planning) | Accepted (Alpha) | 2026-02-18 |
| [012](../docs/adr/ADR-012-bpf-verifier-risk-mitigation.md) | [BPF Verifier Risk Mitigation](ADR-012-BPF-Verifier-Risk-Mitigation) | Accepted + CI Gate | 2026-02-19 |
| [012b](../docs/adr/ADR-012-the-well-postgres.md) | [~~The Well — PostgreSQL~~](ADR-012-The-Well-Postgres) | Deprecated → ADR-016 | 2026-02-19 |
| [013](../docs/adr/ADR-013-routing-header-support.md) | [IPv6 Routing Header](ADR-013-Routing-Header-Support) | Accepted (Deferred) | 2026-02-20 |
| [014](../docs/adr/ADR-014-ipv6-fragmentation-support.md) | [IPv6 Fragmentation](ADR-014-IPv6-Fragmentation-Support) | Accepted (Deferred) | 2026-02-20 |
| [015](../docs/adr/ADR-015-go-fiber-http-layer.md) | [Go-Fiber HTTP Layer](ADR-015-Go-Fiber-HTTP-Layer) | Accepted + Template | 2026-02-20 |
| [016](../docs/adr/ADR-016-postgres-the-well.md) | [PostgreSQL — The Well (+ RPO 24 h amendment 2026-10-15)](ADR-016-Postgres-The-Well) | Accepted | 2026-03-15 |
| [017](../docs/adr/ADR-017-zhen-hybrid-inference.md) | [Zhen Hybrid Inference](ADR-017-Zhen-Hybrid-Inference) | Accepted (Claude API → MCP) | 2026-03-16 |
| [018](../docs/adr/ADR-018-zhen-raft-training-battle-plan.md) | [RAFT Training Pipeline](ADR-018-Zhen-RAFT-Training-Battle-Plan) | **Superseded** by ADR-030 (forge) + ADR-036 (distillation) | 2026-03-16 |
| [019](../docs/adr/ADR-019-zhen-champion-agent.md) | [Zhen Champion Agent](ADR-019-Zhen-Champion-Agent) | Accepted (Phase 1+2) | 2026-03-16 |
| [020](../docs/adr/ADR-020-kanban-bug-fixes.md) | [Kanban Bug Fixes](ADR-020-Kanban-Bug-Fixes) | Accepted (All 8 fixed) | 2026-03-19 |
| [021](../docs/adr/ADR-021-zhen-layer0-substrate.md) | [Zhen Layer 0 Substrate](ADR-021-Zhen-Layer0-Substrate) | Accepted (133 tests) | 2026-03-28 |
| [022](../docs/adr/ADR-022-pihole-docker-to-lxd.md) | [Pi-hole Docker → LXD](ADR-022-Pihole-Docker-to-LXD) | Accepted (runbook ready) | 2026-03-29 |
| [023](../docs/adr/ADR-023-wotan-virtual-memory.md) | [Wotan Virtual Memory](ADR-023-Wotan-Virtual-Memory) | **Deferred to L5** | 2026-03-30 |
| [024](../docs/adr/ADR-024-zhen-runbook-automation.md) | [Runbook Automation](ADR-024-Zhen-Runbook-Automation) | Accepted (31 runbooks) | 2026-04-02 |
| [025](../docs/adr/ADR-025-kanban-mobile-app.md) | [Kanban Mobile App](ADR-025-Kanban-Mobile-App) | **Pipe Dream (Age 4+)** | 2026-04-03 |
| [026](../docs/adr/ADR-026-deb-packaging-ci-pipeline.md) | [.deb Packaging + CI/CD](ADR-026-DEB-Packaging-CI-Pipeline) | Accepted | 2026-04-03 |
| [027](../docs/adr/ADR-027-zhenai-conversation-memory.md) | [Zhenai Conversation Memory](ADR-027-Zhenai-Conversation-Memory) | Accepted (semantic recall + browser) | 2026-04-04 |
| [028](../docs/adr/ADR-028-zhenai-scheduler-heartbeat.md) | [Zhenai Scheduler + Heartbeat](ADR-028-Zhenai-Scheduler-Heartbeat) | Accepted (Sprint C complete) | 2026-04-04 |
| [029](../docs/adr/ADR-029-wotan-consensus-health-remediation.md) | [Wotan Consensus Health (Akira)](ADR-029-Wotan-Consensus-Health-Remediation) | **Partially implemented** (2026-09-29: Akira consensus side; remediation off by default) | 2026-04-04 |
| [030](../docs/adr/ADR-030-zhenai-forge-rust-training.md) | [Zhenai Forge (Rust LoRA training)](ADR-030-Zhenai-Forge-Rust-Training) | **Accepted** (forge pipeline; kingdom-v5 run 2026-04-06) | 2026-04-04 |
| [031](../docs/adr/ADR-031-hybrid-model-handoff.md) | [Hybrid Model Handoff](ADR-031-Hybrid-Model-Handoff) | Planned | 2026-04-03 |
| [032](../docs/adr/ADR-032-python-to-go-rust-migration.md) | [Python → Go/Rust Migration](ADR-032-Python-to-Go-Rust-Migration) | Planned | 2026-04-03 |
| [033](../docs/adr/ADR-033-netbox-ipam.md) | [NetBox IPAM](ADR-033-Netbox-IPAM) | Planned | 2026-04-03 |
| [034](../docs/adr/ADR-034-grpc-mtls-default-transport.md) | [gRPC mTLS Default Transport](ADR-034-gRPC-mTLS-Default-Transport) | Accepted | 2026-04-02 |
| [035](../docs/adr/ADR-035-wotan-active-passive-redundancy.md) | [~~Wotan Active-Passive Redundancy~~](ADR-035-Wotan-Active-Passive-Redundancy) | **Superseded by ADR-064** (2026-05-05) | 2026-04-05 |
| [036](../docs/adr/ADR-036-claude-distillation-training.md) | [Claude Distillation Training Data](ADR-036-Claude-Distillation-Training) | **In Progress** | 2026-04-05 |
| [037](../docs/adr/ADR-037-zhenai-unified-champion.md) | [Zhenai Unified Champion (The Armor)](ADR-037-Zhenai-Unified-Champion) | Planned | 2026-04-05 |
| [038](../docs/adr/ADR-038-kanban-git-audit-trail.md) | [Kanban GUID → Git Commit Audit](ADR-038-Kanban-Git-Audit-Trail) | Accepted | 2026-04-05 |
| [69420](../docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md) | [BGP Sleipnir + Unheaded OS](ADR-69420-Kingdom-BGP-and-Unheaded-OS) | **Deferred to Q4 2026** | 2026-03-18 |
| [039](../docs/adr/ADR-039-cs-precision-reference-service.md) | [CS Precision Reference Service](ADR-039-CS-Precision-Reference-Service) | In Progress | 2026-04-05 |
| [040](../docs/adr/ADR-040-kubernetes-ecosystem-strategy.md) | [Kubernetes Ecosystem Strategy](ADR-040-Kubernetes-Ecosystem-Strategy) | Planned | 2026-04-05 |
| [041](../docs/adr/ADR-041-kanban-timeline-sync.md) | [Kanban ↔ Timeline Bidirectional Sync](ADR-041-Kanban-Timeline-Sync) | Planned | 2026-04-05 |
| [042](../docs/adr/ADR-042-cs-blackmage-lich-integration.md) | [CS → BlackMage Skill + Lich Security](ADR-042-CS-Blackmage-LICH-Integration) | Planned | 2026-04-05 |
| [043](../docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md) | [Mímir's Law: UPC-Controlled Baseline (Gleipnir Phase 0 PoC)](ADR-043-Mimirs-Law-UPC-Baseline-Gleipnir-Phase-0) | **PoC / Research** | 2026-04-08 |
| [044](../docs/adr/ADR-044-kanban-task-detail-status-query.md) | [Kanban Task Detail Status Query (git log + docs + checklist)](ADR-044-Kanban-Task-Detail-Status-Query) | **PIPE DREAM** | 2026-04-11 |
| [045](../docs/adr/ADR-045-wave-10d-gpu-backward-first-real-training.md) | [Wave 10D — GPU Backward + First Real Training Run](ADR-045-Wave-10D-GPU-Backward-First-Real-Training) | **PLANNED** | 2026-04-11 |
| [046](../docs/adr/ADR-046-upc-perservice-ingress-egress-inspection.md) | [UPC Per-Service Ingress/Egress Inspection (Anti-Fake-Horn)](ADR-046-UPC-PerService-Ingress-Egress-Inspection) | **PIPE DREAM** | 2026-04-11 |
| [047](../docs/adr/ADR-047-k8s-honest-assessment-and-extractable-tools.md) | [K8s Honest Assessment + Extractable Tools + East/West K8s Lab](ADR-047-K8s-Honest-Assessment-and-Extractable-Tools) | **ACKNOWLEDGED** | 2026-04-11 |
| [048](../docs/adr/ADR-048-forge-backend-trait.md) | [ForgeBackend Trait — Pluggable Kernel Provider for zhenai-forge](ADR-048-Forge-Backend-Trait) | Accepted | 2026-04-21 |
| [049](../docs/adr/ADR-049-wave11-gpu-kernels-for-gemma4-training.md) | [WAVE11 — Custom HIP Kernels for Gemma-4 Training at seq=384](ADR-049-Wave11-GPU-Kernels-for-Gemma4-Training) | Accepted | 2026-04-21 |
| [050](../docs/adr/ADR-050-wave12-gpu-resident-activations.md) | [WAVE12 — GPU-Resident Forge Activations + Kingdom RAFT LoRA](ADR-050-Wave12-GPU-Resident-Activations) | Accepted | 2026-04-23 |
| [051](../docs/adr/ADR-051-wave13-generate-path.md) | [WAVE13 — Forge generate-gemma4 path + KV-cache deferral](ADR-051-Wave13-Generate-Path) | **Accepted** (verdict: RETRAIN) | 2026-04-28 |
| [052](../docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md) | [Timeline & Battle-Plan Source-of-Truth Policy (drift ≤ 7 days, in-tree only)](ADR-052-Timeline-and-Battleplan-Source-of-Truth) | **Accepted** (CI gate live) | 2026-04-27 |
| [053](../docs/adr/ADR-053-hybrid-claude-zhenai-workflow-templates.md) | [Hybrid Claude + Local Zhenai Workflow Templates (cost-driven hybrid routing)](ADR-053-Hybrid-Claude-Zhenai-Workflow-Templates) | **Pipe Dream** (activates on cost/quality/strategic/personal trigger) | 2026-04-27 |
| [055](../docs/adr/ADR-055-kev-poller-always-on-service.md) | [KEV Poller — Always-On Kingdom Service + K8s Cross-Host Pilot (CISA KEV + NIST NVD parity)](ADR-055-KEV-Poller-Always-on-Service) | **Planned**; Phase 1 built 2026-10-19 (`cmd/kev-poller`), Phase 2 manifests written, nothing deployed | 2026-04-27 |
| [056](../docs/adr/ADR-056-pgvector-auxiliary-corpus-sharding.md) | [pgvector Auxiliary Corpus Sharding for Trust-Tagged Retrieval (Wikipedia / Stack Overflow / RFCs / source code)](ADR-056-Pgvector-Auxiliary-Corpus-Sharding) | **Proposed** (gated on WAVE15 H0 passing) | 2026-05-02 |
| [057](../docs/adr/ADR-057-unheaded-source-code-indexing.md) | [Unheaded Source Code Indexing for Semantic Retrieval (AST chunks + code-specialized embedder, first instance of ADR-056 pattern)](ADR-057-Unheaded-Source-Code-Indexing) | **Proposed** (gated on WAVE15 H0 + ADR-056 acceptance) | 2026-05-02 |
| [058](../docs/adr/ADR-058-gcp-cost-alarm-bellis-tech.md) | [GCP Cost & API Utilization Alarms for bellis.tech (EDoS / cost-amplification DoS defense — free-tier project)](ADR-058-GCP-Cost-Alarm-Bellis-Tech) | **Planned** (~30 min of console work; no code dependencies) | 2026-05-02 |
| [059](../docs/adr/ADR-059-zhenai-interactive-cli.md) | [Zhenai Interactive CLI (`cmd/zhen-cli`) — terminal REPL counterpart of the web UI; same Champion-gated mutation path](ADR-059-Zhenai-Interactive-CLI) | **Phase 1 Shipped** (2026-05-03; Phase 2-3 Planned) | 2026-05-02 |
| [060](../docs/adr/ADR-060-zhenai-multi-model-selector.md) | [Zhenai Multi-Model Selector — UI dropdown + Champion-gated `model_switch` handler; T11-T20 threat catalog + LICH-013 pre-registered](ADR-060-Zhenai-Multi-Model-Selector) | **In Progress** (activated mid-WAVE16; implementation 2026-05-04 overnight) | 2026-05-04 |
| [061](../docs/adr/ADR-061-cloud-rented-training-purpose-built-coding-model.md) | [Cloud-Rented Training for Purpose-Built Unheaded Coding Model — pivot from off-the-shelf search to bespoke LoRA on rented A100/H100](ADR-061-Cloud-Rented-Training-Purpose-Built-Coding-Model) | **Research / Pipe Dream** (~$5-50 budget when activated; gated on WAVE13 RETRAIN clearing first) | 2026-05-04 |
| [062](../docs/adr/ADR-062-fuzz-redteam-pentest-framework.md) | [Fuzz / Red-Team / Pentest Framework (Lich Codified) — three-tier offensive taxonomy, numbered LICH-NNN campaigns, activation rules](ADR-062-Fuzz-Redteam-Pentest-Framework) | **Accepted** (existing pattern ratified; new campaigns slot in) | 2026-05-04 |
| [063](../docs/adr/ADR-063-akira-summons-lich-randomized-ctf.md) | [Akira Randomly Summons the Lich for CTF Mode — bounded, opt-in, randomized adversarial probing of live Kingdom services](ADR-063-Akira-Summons-LICH-Randomized-CTF) | **Pipe Dream** (activates after ADR-062's framework has 3 campaigns clean for 7 days) | 2026-05-04 |
| [064](../docs/adr/ADR-064-wotan-active-active-cluster-k8s-native.md) | [Wotan Active/Active Cluster — 3-node minimum, K8s-native StatefulSet, Raft membership + topic-leader election, broadcast replication. Supersedes ADR-035 ("active passive works but doesn't scale")](ADR-064-Wotan-Active-Active-Cluster-K8s-Native) | **Proposed** (4-phase rollout; ADR-035 stays running through cutover) | 2026-05-05 |
| [065](../docs/adr/ADR-065-aya-major-version-migration.md) | [aya 0.1.x → 0.13.x Major-Version Migration Plan](ADR-065-Aya-Major-Version-Migration) | **Superseded** (Phase A finding: aya splits userspace/kernel independently; no migration needed) | 2026-05-07 |
| [066](../docs/adr/ADR-066-tonic-minor-bump-trace-collector.md) | [`cmd/trace-collector` tonic 0.10 → 0.12 — closes 4 CVE-class advisories in rustls-webpki + protobuf chain](ADR-066-Tonic-Minor-Bump-Trace-Collector) | **Accepted** | 2026-05-08 |
| [067](../docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md) | [MBC ISA v2 + UPC ABI v1 freeze — 5 new opcodes (FENCE/MRET/SRET/LR.W/SC.W), priv_level field, Boot Protocol v2, memory-mapped CSR region](ADR-067-MBC-ISA-V2-and-UPC-ABI-V1) | **Accepted** (Phase 0 ASCEND-LINUX gate) | 2026-05-08 |
| [072](../docs/adr/ADR-072-boot-magic-byte-ordering.md) | [BOOT_MAGIC byte-ordering convention — canonical hex 0x554E4844 ('UNHD' MSB-first); wire bytes 'D','H','N','U' per LE](ADR-072-Boot-Magic-Byte-Ordering) | **Accepted** (doc-only clarification) | 2026-05-10 |
| [073](../docs/adr/ADR-073-lint-policy-zero-findings.md) | [Lint Policy: Zero Findings as the New Floor — triage protocol; 12 CVE-class fixes shipped during the 2026-05-11 drain](ADR-073-Lint-Policy-Zero-Findings) | **Accepted** (`golangci-lint run ./...` returns 0 issues; CI ratchet) | 2026-05-11 |
| [074](../docs/adr/ADR-074-phase12-page-table-model.md) | ASCEND-LINUX Phase 1.2 Page-Table Model — Option A per-pid pgd, VA[0,8MiB) → disjoint physical slices via two 4 MiB superpages | **Accepted** (Phase 1.2 IMPL closed 2026-05-13) | 2026-05-11 |
| [075](../docs/adr/ADR-075-phase13-process-model.md) | ASCEND-LINUX Phase 1.3 Process Model — PROC_TABLE 4→8, ZOMBIE exit, LR.W/SC.W reservation-clear, RV2MBC SHA-256 gate | **Accepted** | 2026-05-13 |
| [076](../docs/adr/ADR-076-topology-map-single-source-of-truth.md) | Single Source of Truth for the Live Topology Map | **Proposed** (planning) | 2026-06-02 |
| [077](../docs/adr/ADR-077-phase17-rv2mbc-base-feature-gated-abi-fork.md) | ASCEND-LINUX Phase 1.7 Gate B — feature-gated MbcCpuState ABI fork; per-process rv2mbc_base routes each pid's indirect branches (exec → live `$`) | **Accepted** (shipped 2026-06-19) | 2026-06-18 |
| [078](../docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md) | ASCEND-LINUX Phase 1.7 Gate D — in-BPF FS reader (Design A: open/read/fstat walk fs.img inodes directly), ilp32e struct-stat truth, exec argv frame ABI at VA 0x600000, a2/a3 re-entry register protocol | **Accepted** (shipped 2026-07-02) | 2026-07-02 |
| [079](../docs/adr/ADR-079-ret-address-tagging.md) | RET return-address tagging — CALL/CALLR tag MBC PCs with bit 31 so RET distinguishes them from raw RV addresses without the fragile magnitude floor; retires RET_RV_FLOOR, unblocks Linux-scale images, makes the Doom [0x10000,0x151BF] misparse impossible (verifier +0.03%) | **Accepted** (Phase 2.0 substrate prereq #1; xv6 corpus green 2026-07-02) | 2026-07-02 |
| [080](../docs/adr/ADR-080-upc-programmatic-api.md) | The UPC Programmatic API — UPC as a general multi-workload MBC substrate (Doom + xv6/Linux on one interpreter); the 5-part guest contract (MBC image / rv2mbc / memory / syscall surface / boot protocol); per-workload syscall surfaces DCE-partitioned by cfg to respect the 1M verifier ceiling; load-test-each-variant rule; path to registered dispatch | **Accepted** (formalizes realized architecture) | 2026-07-03 |
| [081](../docs/adr/ADR-081-unheaded-linux-from-scratch.md) | Unheaded Linux — build our own minimal OS from scratch on the UPC instead of vendoring uClinux+busybox; 100% in-house, as light/small as possible (fits verifier budget + code-store); scales UPC PoC → Unheaded Linux → Yggdrasil golden image via soft-fork discipline on an owned base. Supersedes ascend Phase 2 vendor step; moots the LTS/arch pair calls | **Accepted** (Stevie's call) | 2026-07-03 |
| [082](../docs/adr/ADR-082-unheadnix-niche-brainstorm.md) | unheadnix — feasible-usage brainstorm for ultralight UPC pool nodes | **Brainstorm / Exploratory** (explicitly not a decision; feeds a future ADR) | 2026-07-19 |
| [083](../docs/adr/ADR-083-terminology-conventions.md) | Terminology conventions (rolling) — internal "zero user data access" vs user-facing "NIST 800-207 Zero Trust Architecture" | **Accepted** (rolling) | 2026-07-19 |
| [084](../docs/adr/ADR-084-huginn-host-metrics-agent.md) | Huginn — host metrics agent | **Accepted** | 2026-07-24 |
| [085](../docs/adr/ADR-085-cicd-artifact-layout.md) | CI/CD pipeline polish — canonical artifact layout | **Planned** | 2026-07-24 |
| [086](../docs/adr/ADR-086-muninn-observability-pipeline.md) | Muninn — observability fan-out pipeline | **Planned** | 2026-07-24 |
| [087](../docs/adr/ADR-087-noc-network-device-monitoring.md) | NOC — network device monitoring and configuration management | **Planned** | 2026-07-24 |
| [088](../docs/adr/ADR-088-kubernetes-service-orchestration.md) | Kingdom Deployment Substrate Ladder — K8s / Terraform / Ansible / NixOS / packaging as a learning ladder over 56 real service components | **Planned** | 2026-07-24 |
| [089](../docs/adr/ADR-089-dev-staging-main-promotion-workflow.md) | `develop` → `staging` → `main` promotion workflow — one reviewable unit per commit, never promote red, ratchet by rule ID, revert not force-push | **Accepted** (Stevie's call) | 2026-08-03 |
| [090](../docs/adr/ADR-090-total-source-sweep-slop-removal.md) | The Total Source Sweep — read all tracked source files once, fixed order, resumable TSV ledger, explicit "NOT slop" list | **Proposed** (activates after the Staging Ladder sprint) | 2026-08-03 |
| [091](../docs/adr/ADR-091-the-well-initdb-ordering.md) | The Well — one orchestrator in `/docker-entrypoint-initdb.d`, migrations mounted read-only elsewhere; never nest a bind mount inside another's mountpoint | **Accepted** | 2026-08-04 |
| [092](../docs/adr/ADR-092-var-log-discipline-and-rotation.md) | Every service logs to `/var/log/unheaded/<service>/` — per-service logrotate budgets, capped container log drivers, `copytruncate` banned, `/var/log` on its own filesystem | **Proposed** — steps 1, 3, 6 done (call sites 31 → 0 via `scripts/lib/logpath.sh`, 2026-10-13); Huginn log-disk metrics + alerts landed | 2026-09-22 |
| [093](../docs/adr/ADR-093-what-shape-is-this-thing.md) | What shape is this thing we build? — a protocol with three faucets (computer, platform, substrate ladder) and a workshop around it; 10 of 44 binaries run, so the default failure is a correct mechanism nothing reaches (8 instances catalogued) | **Proposed** | 2026-09-23 |
| [094](../docs/adr/ADR-094-own-the-metrics-stack.md) | Own the metrics stack — retire `prometheus/client_golang` (9 modules, 31 files). Tier 1 hand-written exposition DONE (sophia had 4 untyped series); Tier 2 done 2026-09-25 (`metrics.NewGoCollector()` built first, because `go_goroutines`/`go_memstats_*` diagnosed this repo's two worst production bugs); client_golang gone from go.mod | **Proposed** — fully implemented; acceptance is Stevie's call | 2026-09-24 |
| [095](../docs/adr/ADR-095-slow-roll-structural-review.md) | Slow-roll review, two tracks: **structure** (173 prod complexity findings, 83/668 files) and **supply chain** (21 direct Go modules, 12 with no ADR-004 approval, 3 "replaced" but still imported; 411 Rust crates; 0/9 base images digest-pinned; 29/36 actions on mutable tags). Flag all into a register, then ONE at a time: own / vet-and-record / remove. Starts after this sprint reaches main | **Proposed** | 2026-09-24 |
| [096](../docs/adr/ADR-096-dependency-gate-cost-to-fake.md) | Dependency gate v2 — amends ADR-004. Signals ranked by cost to fake: cheap ones (repo age, org type, contributors, stars, commit history) may only reject; anchored ones (major importers, distro packaging, CNA-handled CVEs, identity-bound signing) set review depth; our own full read decides. No assistant-suggested imports (slopsquatting). Pin what was read; re-check on change of control. Found: yaml.v3 archived (22 prod files) | **Proposed** | 2026-09-24 |
| [097](../docs/adr/ADR-097-compliance-crosswalk.md) | Compliance crosswalk: common controls in `compliance/catalog/controls.yaml` map many-to-many onto frameworks with publisher denominators (800-53 1014, FedRAMP L/M/H, CSF 2.0, 800-171, SSDF, SOC 2, ISO 27001); evidence that can fail (CI jobs, gates, signatures, host sysctl/sshd/probes, signed attestations); dashboard `/compliance` + CSV export | **Proposed** | 2026-09-30 |
| [098](../docs/adr/ADR-098-findings-register-and-baseline-modes.md) | Rolling findings register (count generated from evidence, triage in `compliance/findings/register.yaml`, closes only when evidence passes) + host baselines in **audit** (default, SELinux permissive) or **enforcing** mode; enforcing gated on LICH-012 against the agent + 7-day clean soak + Stevie sign-off; does not amend ADR-043 | **Proposed** | 2026-09-30 |
| [099](../docs/adr/ADR-099-remediation-as-code-and-control-profiles.md) | Remediation as code: each fix once as data from a closed action set; control profiles (`none`/`lab`/`cis-l1-server`/`all`) switch remediation, never detection; native executor, Ansible and runbook renderers; apply journal as evidence. Amended same day: state lives in The Well, YAML is seed and fallback, hosts act only on sealed snapshots | **Proposed** | 2026-10-01 |
| [100](../docs/adr/ADR-100-sre-alongside-compliance.md) | SRE alongside compliance: SLOs as data and as crosswalk evidence, multi-window burn-rate alerts, every alert with a runbook and a firing test, Watchdog, blameless postmortems; first instance a per-service log-volume deviation SLO (spam and silence). Built 2026-10-20: only SLO burn notifies (urgency page/ticket/none), capped alert-mailer in dry-run; log-volume and Watchdog not yet | **Accepted** | 2026-10-01 |
| [101](../docs/adr/ADR-101-well-migration-ledger.md) | The Well's migrations: `db/migrations/MANIFEST` orders and routes every file, a `well_schema_migrations` ledger per database records each with its sha256, `db/init.sh` and `cmd/well-migrate` (check/status/apply/baseline) share both; drift refuses, lock and statement timeouts per migration | **Proposed** (in use on the live Well since 2026-10-15) | 2026-10-14 |
| [102](../docs/adr/ADR-102-container-image-versions.md) | Container images use exact version tags (no `latest`, no floating minor), pinned to what runs today; upgrades are their own commits; digests deferred until a bot can bump them; reassess then | **Accepted** | 2026-10-15 |

---

> **Source:** [docs/adr/ADR-INDEX.md](../docs/adr/ADR-INDEX.md)
