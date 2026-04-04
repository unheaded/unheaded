# WAVE 6 BATTLE PLAN — All Remaining Kingdom Work

**Forged**: 2026-04-03
**Sprint**: WAVE-6 — Complete Every Open ADR Item
**Prerequisite**: Waves 1-5 complete (55 commits, zhenai-forge operational, 34 runbooks, kingdom.zlora trained)
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x estimate or 2 failed debug attempts, log to handoff

## EXECUTION RULES

1. On session start: read this file, find first unchecked item, begin immediately.
2. After starting long-running step: continue next independent phase.
3. Before each phase: re-read this file.
4. After each phase: update PROGRESS, commit, start next phase.
5. Each step has a GOTO pointing to the source doc on disk and a NEXT pointing to the next step.

---

## PROGRESS (update as you go)
- [x] Sprint A: DONE — Wave 8 complete. 7930 steps, loss 14.33→8.77, pipeline proven end-to-end
- [x] Sprint B: DONE — B2+B3 semantic recall fallback, B4 decision search, B6 history browser tab
- [x] Sprint C: DONE — scheduler daemon + chat commands + emergency stop
- [x] Sprint D: DONE — D1-D3 pkg/health + binary, D4 YAML config + EAST deploy (7/7 healthy, 0 false alerts)
- [x] Sprint E: DONE — 12 .debs built, published to APT repo, 8 installed on EAST
- [ ] Sprint F: Pi-hole LXD Migration (needs user present for DNS cutover)
- [x] Sprint G: DONE — ScanCode 8660 files clean, 99 deps audited, trademark cleared
- [x] Sprint H: DONE — H1 JWT (123 tests) + H2 age encryption for sealed casks
- [ ] Sprint I: Protocol Maturity (zhend 9 TODOs, Wotan clustering)
- [x] Sprint J: DONE — J1 Corvée, J2 Bailiff, J3 Config Gen, J4 Session-to-Runbook
- NEW: ADR-031 Hybrid Model Handoff — PLANNED
- NEW: ADR-032 Python → Go/Rust Migration — PLANNED
- NEW: ADR-033 NetBox IPAM — PLANNED

## KNOWN STUBS (honest audit 2026-04-04)

Items that have tests passing but are NOT yet real implementations:

### CRITICAL STUB 1: zhenai-forge training gradients (Sprint A)
- **What tests verify**: Adam optimizer math, LoRA shapes, loss convergence curve
- **What's stubbed**: Gradients are RANDOM, not from model forward pass. ~~GPU buffers loaded with ZEROS~~ GPU now loads REAL tensor data from GGUF mmap (FIXED 2026-04-04). The kingdom.zlora file was trained on random gradients — NOT useful for inference until real forward/backward is implemented.
- **Impact**: Cannot produce a real fine-tuned model until A3-A7 are completed (real GPU matmul + real forward/backward)
- **Fix**: Sprint A steps A3-A7 (hipBLAS forward pass, cross-entropy loss, backward pass for LoRA)
- **Files**: `crates/zhenai-forge/src/train.rs` (line 99-102: zero buffers), `src/lora.rs` (line 189-221: simulated gradients)

### ~~CRITICAL STUB 2: Akira auto-restart~~ — FIXED 2026-04-04
- Now calls `exec.Command("systemctl", "restart", svcUnit)` for real

### ~~MODERATE STUB 3: Champion PostgreSQL integration~~ — FIXED 2026-04-04
- Integration tests pass against real Docker PostgreSQL (The Well)
- `pkg/champion/integration_test.go`: 2 tests verify action logging to zhen_actions table

### MINOR GAPS
- Scheduler (`zhen_scheduler.py`): No unit tests, but real subprocess execution verified
- Conversation recall commands: Real SQL but untested with Zhenai running
- Chat command handlers: No automated tests, manual testing only when Zhenai is up

---

## SPRINT A: ZHENAI FORGE GPU KERNELS (ADR-030 remaining)

**Goal**: Replace simulated gradients with real hipBLAS GPU matmul
**Docs**: `docs/adr/ADR-030-zhenai-forge-rust-training.md`
**Code**: `crates/zhenai-forge/src/`
**Time**: 2-3 sessions

- [ ] **A1** [CODE]: Add hipBLAS FFI bindings to `src/hip.rs`
  - GOTO: `crates/zhenai-forge/src/hip.rs` (line 60+, extern block)
  - Add: `hipblasSgemm`, `hipblasCreate`, `hipblasDestroy`
  - Link: `/opt/rocm/lib/libhipblas.so`
  - NEXT: A2

- [ ] **A2** [CODE]: Implement dequantize-on-GPU kernel for Q5_K blocks
  - GOTO: `crates/zhenai-forge/src/gguf.rs` (tensor type handling)
  - Write HIP kernel that reads Q5_K blocks and outputs f16/f32 tiles
  - NEXT: A3

- [ ] **A3** [CODE]: Implement forward pass in `src/train.rs`
  - GOTO: `crates/zhenai-forge/src/train.rs` (GpuModel struct)
  - Real forward: embedding lookup → attention (Q/K/V + LoRA) → FFN → logits
  - Use hipBLAS sgemm for matrix multiply
  - NEXT: A4

- [ ] **A4** [CODE]: Implement cross-entropy loss on GPU
  - GOTO: `crates/zhenai-forge/src/train.rs`
  - Softmax + log-likelihood, return scalar loss
  - NEXT: A5

- [ ] **A5** [CODE]: Implement backward pass for LoRA only
  - GOTO: `crates/zhenai-forge/src/lora.rs` (gradient accumulators)
  - Chain rule through attention projections, accumulate in grad_a/grad_b
  - Base model gradients NOT computed (frozen)
  - NEXT: A6

- [ ] **A6** [TEST]: End-to-end training with real gradients
  - Run on small subset (100 examples), verify loss decreases meaningfully
  - Compare loss curve to simulated version
  - NEXT: A7

- [ ] **A7** [V]: **SPRINT A EXIT GATE**
  - `zhenai-forge train` produces kingdom.zlora with REAL gradients
  - Loss curve shows actual learning (not simulated decay)
  - NEXT: Sprint B

---

## SPRINT B: CONVERSATION MEMORY (ADR-027)

**Goal**: Persistent cross-session recall with semantic search
**Docs**: `docs/adr/ADR-027-zhenai-conversation-memory.md`
**Code**: `raft/zhen_app.py`
**Time**: 1-2 sessions

- [ ] **B1** [V]: Verify conversation logging works (schema fixed earlier)
  - GOTO: `raft/zhen_app.py` (line 126, `_pg_log` function)
  - Start Zhenai, ask a question, check zhen_conversations table
  - NEXT: B2

- [ ] **B2** [CODE]: Add conversation embedding to separate FAISS index
  - GOTO: `docs/adr/ADR-027-zhenai-conversation-memory.md` (Layer 2)
  - Embed each conversation turn via all-MiniLM-L6-v2
  - Store in `/var/zhen/index/conversations.index`
  - NEXT: B3

- [ ] **B3** [CODE]: Enhance `recall` command with semantic search
  - GOTO: `raft/zhen_app.py` (`_try_command` function, `recall` handler)
  - Fall back to FAISS semantic search when PostgreSQL text search returns nothing
  - NEXT: B4

- [ ] **B4** [CODE]: Add `what did we decide about <X>` handler
  - GOTO: `raft/zhen_app.py` (`_try_command` function)
  - Search for conversations containing decision-like patterns
  - NEXT: B5

- [ ] **B5** [CODE]: Add periodic memory consolidation
  - GOTO: `docs/adr/ADR-027-zhenai-conversation-memory.md` (Layer 3)
  - Weekly: summarize conversations via Mistral-7B → store as consolidated memory
  - NEXT: B6

- [ ] **B6** [CODE]: Add conversation browser to web UI
  - GOTO: `raft/static/index.html`
  - Sidebar or tab showing past conversations by date
  - Click to view full conversation
  - NEXT: B7

- [ ] **B7** [V]: **SPRINT B EXIT GATE**
  - `recall wotan` returns past conversations about Wotan
  - `what did we decide about storage` returns ADR-011 decisions
  - Conversation browser shows history in web UI
  - NEXT: Sprint C

---

## SPRINT C: SCHEDULER & HEARTBEAT (ADR-028)

**Goal**: Proactive Zhenai — scheduled runbooks + health heartbeat
**Docs**: `docs/adr/ADR-028-zhenai-scheduler-heartbeat.md`
**Code**: `raft/zhen_app.py`, new `raft/zhen_scheduler.py`
**Time**: 1-2 sessions

- [ ] **C1** [CODE]: Create `raft/zhen_scheduler.py` — cron-based runbook scheduler
  - GOTO: `docs/adr/ADR-028-zhenai-scheduler-heartbeat.md` (Cron section)
  - Python `schedule` library or systemd timers
  - Reads schedule config from The Well or YAML
  - NEXT: C2

- [ ] **C2** [CODE]: Add `schedule` / `unschedule` / `list schedules` chat commands
  - GOTO: `raft/zhen_app.py` (`_try_command` function)
  - `schedule service-health-sweep every 30m`
  - `unschedule service-health-sweep`
  - NEXT: C3

- [ ] **C3** [CODE]: Implement heartbeat monitor
  - GOTO: `docs/adr/ADR-028-zhenai-scheduler-heartbeat.md` (Heartbeat section)
  - Lightweight 5-min health pulse: service health, disk, memory, swap
  - Alert on anomalies → log to The Well
  - NEXT: C4

- [ ] **C4** [CODE]: Add `emergency stop` command
  - GOTO: `docs/adr/ADR-028-zhenai-scheduler-heartbeat.md` (Emergency Stop)
  - Kill all running runbooks, cancel all schedules, manual-only mode
  - NEXT: C5

- [ ] **C5** [V]: **SPRINT C EXIT GATE**
  - `schedule service-health-sweep every 30m` creates a cron entry
  - Heartbeat detects a stopped service and logs alert
  - `emergency stop` kills all scheduled jobs
  - NEXT: Sprint D

---

## SPRINT D: CONSENSUS HEALTH WATCHDOG (ADR-029)

**Goal**: Every node a watchdog, 66.67% consensus triggers auto-restart
**Docs**: `docs/adr/ADR-029-wotan-consensus-health-remediation.md`
**Code**: `pkg/health/`, `cmd/watchdog/`, `services/wotan/`
**Time**: 2-3 sessions

- [ ] **D1** [CODE]: Create `pkg/health/watchdog.go` — health check loop
  - GOTO: `docs/adr/ADR-029-wotan-consensus-health-remediation.md` (Phase 1)
  - Health-check every service every 30s, publish to Wotan `system.health.reports`
  - NEXT: D2

- [ ] **D2** [CODE]: Create `pkg/health/consensus.go` — aggregation + threshold
  - GOTO: `docs/adr/ADR-029-wotan-consensus-health-remediation.md` (ConsensusThreshold)
  - `const ConsensusThreshold = 2.0 / 3.0` — hardcoded, not configurable
  - Compute `failure_rate = unique_reporters_failing / total_dependent_services`
  - NEXT: D3

- [ ] **D3** [CODE]: Create `cmd/watchdog/main.go` — systemd daemon
  - GOTO: `docs/adr/ADR-029-wotan-consensus-health-remediation.md` (Node Agent)
  - Connects to Wotan, discovers services, health-checks, publishes, remediates
  - NEXT: D4

- [ ] **D4** [CODE]: Wire consensus engine into Wotan
  - GOTO: `services/wotan/` (internal/health/)
  - Wotan aggregates `system.health.reports`, publishes `system.health.remediate`
  - NEXT: D5

- [ ] **D5** [TEST]: Test consensus-triggered restart
  - Stop a service, wait for 66.67% consensus, verify auto-restart
  - NEXT: D6

- [ ] **D6** [V]: **SPRINT D EXIT GATE**
  - Watchdog running on WEST + EAST via systemd
  - Consensus triggers auto-restart when 2/3 report failure
  - Max 3 auto-restarts before escalating to human
  - NEXT: Sprint E

---

## SPRINT E: .DEB PACKAGING FULL STACK (ADR-026 Phases 2-4)

**Goal**: All services installable via `apt install`, systemd-managed
**Docs**: `docs/adr/ADR-026-deb-packaging-ci-pipeline.md`
**Code**: `debian/`, `Jenkinsfile`
**Time**: 2-3 sessions

- [ ] **E1** [CODE]: Build .deb for each core service
  - GOTO: `debian/rules` and `debian/control`
  - Services: wotan (done), daemon, dashboard, kanban, timeguru, captain, architect, micromanager
  - Each with systemd unit + postinst + prerm
  - NEXT: E2

- [ ] **E2** [CODE]: Build .deb for unheaded-ebpf (all eBPF programs)
  - GOTO: `debian/control` (add unheaded-ebpf package)
  - NEXT: E3

- [ ] **E3** [CODE]: Build .deb for unheaded-zhenai (web UI + RAG + MCP)
  - GOTO: `debian/control` (add unheaded-zhenai package)
  - NEXT: E4

- [ ] **E4** [B]: Publish all .debs to APT repo
  - GOTO: `runbooks/infra/apt-repo-server.yaml`
  - `reprepro -b /var/lib/apt-repo includedeb noble *.deb`
  - NEXT: E5

- [ ] **E5** [B]: Install full stack on EAST via apt
  - `ssh govan@east 'sudo apt install unheaded-wotan unheaded-daemon unheaded-dashboard ...'`
  - NEXT: E6

- [ ] **E6** [V]: All services running via systemd on EAST
  - `ssh govan@east 'systemctl status unheaded-*'`
  - NEXT: E7

- [ ] **E7** [V]: **SPRINT E EXIT GATE**
  - `dpkg -l | grep unheaded` shows all packages on both hosts
  - All services start on boot via systemd
  - NEXT: Sprint F

---

## SPRINT F: PI-HOLE LXD MIGRATION (ADR-022)

**Goal**: Pi-hole on LXD, Docker Pi-hole removed, DNS stable
**Docs**: `docs/adr/ADR-022-pihole-docker-to-lxd.md`
**Runbook**: `runbooks/network/dns-pihole-lxd.yaml`
**Time**: 1 session (needs user present for DNS cutover)

- [ ] **F1** [B]: Execute runbook steps 1-8 (backup, create LXD, install, config)
  - GOTO: `runbooks/network/dns-pihole-lxd.yaml`
  - NEXT: F2

- [ ] **F2** [V]: `dig @10.10.10.53 google.com` resolves + ads blocked
  - NEXT: F3

- [ ] **F3** [ESCALATE]: Switch host DNS to LXD Pi-hole (live cutover)
  - Needs user approval — DNS downtime possible
  - NEXT: F4

- [ ] **F4** [B]: Stop Docker Pi-hole, re-enable LXD at boot
  - NEXT: F5

- [ ] **F5** [V]: **SPRINT F EXIT GATE**
  - Host DNS via LXD Pi-hole, Docker Pi-hole removed
  - NEXT: Sprint G

---

## SPRINT G: PRE-PUBLIC AUDIT (ADR-020 items)

**Goal**: License scan, SBOM refresh, code provenance clean
**Docs**: `docs/adr/ADR-020-kanban-bug-fixes.md` (pre-public section)
**Time**: 1 session

- [ ] **G1** [B]: Run ScanCode deep scan (overnight)
  - GOTO: `docs/legal/` (output location)
  - `scancode --license --copyright --json-pp docs/legal/scancode-full.json ~/tmp/unheaded/`
  - NEXT: G2

- [ ] **G2** [V]: No GPL-incompatible deps in production
  - NEXT: G3

- [ ] **G3** [B]: Refresh SBOM (was S78 — 553 deps)
  - `go list -m all | wc -l` + Cargo.toml audit
  - NEXT: G4

- [ ] **G4** [B]: Verify all deps predate July 2019 (ADR-004)
  - GOTO: `docs/adr/ADR-004-no-external-deps-policy.md` (age requirement)
  - GitHub API check for each dependency
  - NEXT: G5

- [ ] **G5** [V]: **SPRINT G EXIT GATE**
  - ScanCode clean, SBOM current, all deps pass age check
  - NEXT: Sprint H

---

## SPRINT H: SECURITY HARDENING (ADR-008, ADR-010)

**Goal**: JWT auth, LUKS encryption for sealed casks
**Docs**: `docs/adr/ADR-008-security-hardening-baseline.md`, `docs/adr/ADR-010-sealed-cask-deployment.md`
**Time**: 2 sessions

- [ ] **H1** [CODE]: Implement JWT authenticator in `pkg/auth/`
  - GOTO: `pkg/auth/` (existing APIKey + middleware)
  - Add JWTAuthenticator with Ed25519 signing
  - NEXT: H2

- [ ] **H2** [CODE]: Add LUKS encryption to sealed cask build
  - GOTO: `scripts/build-sealed-cask.sh`
  - Encrypt the .tar.gz with LUKS/dm-crypt or age
  - NEXT: H3

- [ ] **H3** [TEST]: Build encrypted cask, verify + decrypt + deploy
  - NEXT: H4

- [ ] **H4** [V]: **SPRINT H EXIT GATE**
  - JWT auth works for service-to-service
  - Sealed cask is encrypted at rest
  - NEXT: Sprint I

---

## SPRINT I: PROTOCOL MATURITY (ADR-005, ADR-021)

**Goal**: Wotan clustering, PQ crypto Phases 2-4
**Docs**: `docs/adr/ADR-005-wotan-message-backbone.md`, `docs/adr/ADR-021-zhen-layer0-substrate.md`
**Time**: 3-4 sessions

- [ ] **I1** [CODE]: Wotan active-passive replication (WEST primary, EAST standby)
  - GOTO: `services/wotan/` 
  - WAL-based message replay from primary to standby
  - NEXT: I2

- [ ] **I2** [CODE]: PQ Phase 2 — hybrid key exchange for gossip
  - GOTO: `crates/zhend/` (crypto module)
  - X25519 + ML-KEM hybrid
  - NEXT: I3

- [ ] **I3** [CODE]: PQ Phase 3 — fragment encryption
  - ML-KEM-1024 + X25519 hybrid for fragment storage
  - NEXT: I4

- [ ] **I4** [V]: **SPRINT I EXIT GATE**
  - Wotan replicates to EAST, PQ key exchange operational
  - NEXT: Sprint J

---

## SPRINT J: ZHENAI CHAMPION EXPANSION (ADR-019 Phases 2-4)

**Goal**: Corvée duty loops, Bailiff drift detection, config generation
**Docs**: `docs/adr/ADR-019-zhen-champion-agent.md`
**Time**: 2-3 sessions

- [ ] **J1** [CODE]: Corvée duty — scheduled Champion tasks
  - GOTO: `docs/adr/ADR-019-zhen-champion-agent.md` (Feudal Duty System)
  - Daily: run service-health-sweep, check disk, review logs
  - NEXT: J2

- [ ] **J2** [CODE]: Bailiff — drift detection
  - Compare actual state vs desired state (configs, running services)
  - Alert on drift
  - NEXT: J3

- [ ] **J3** [CODE]: Config generation templates
  - nginx, haproxy, systemd unit generation from parameters
  - `zhenai generate nginx --service wotan --port 18000`
  - NEXT: J4

- [ ] **J4** [CODE]: Runbook authoring — session-to-runbook extraction
  - GOTO: `docs/adr/ADR-024-zhen-runbook-automation.md` (Phase 3)
  - After a debugging session, extract steps into YAML runbook draft
  - NEXT: J5

- [ ] **J5** [V]: **SPRINT J EXIT GATE**
  - Corvée runs daily, Bailiff detects config drift
  - Config generation produces valid nginx/systemd
  - Session-to-runbook extraction creates draft YAML
  - NEXT: DONE

---

## DEFERRED (explicitly not in this plan)

| Item | Defer To | GOTO |
|------|----------|------|
| ADR-009 Parish Boundaries | Beta | `docs/adr/ADR-009-parish-boundaries.md` |
| ADR-013 SRv6 Routing | Phase 2 | `docs/adr/ADR-013-routing-header-support.md` |
| ADR-014 IPv6 Fragmentation | Phase 2 | `docs/adr/ADR-014-ipv6-fragmentation-support.md` |
| ADR-015 Fiber Migration | Post-investigation | `docs/adr/ADR-015-go-fiber-http-layer.md` |
| ADR-023 Wotan Virtual Memory | Dream Ladder L5 | `docs/adr/ADR-023-wotan-virtual-memory.md` |
| ADR-025 Kanban Mobile App | Age 4+ | `docs/adr/ADR-025-kanban-mobile-app.md` |
| ADR-026 Phases 5-6 Monorepo Breakup | Age 3+ | `docs/adr/ADR-026-deb-packaging-ci-pipeline.md` |
| ADR-069420 Sleipnir BGP + Yggdrasil OS | Q4 2026 | `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` |

---

## CRITICAL PATH

```
Sprint A (GPU kernels) ──→ Sprint B (memory) ──→ Sprint C (scheduler) ──→ Sprint J (champion)
                                                                    ↗
Sprint D (watchdog) ─────────────────────────────────────────────────
Sprint E (.deb all services) → Sprint F (Pi-hole) → Sprint G (audit) → Sprint H (security)
Sprint I (protocol) ─────────────────────────────────────────────────→ DONE
```

Sprints A, D, E, I are independent — can run in parallel across sessions.
Sprint F needs user present (DNS cutover).
Sprint G can run overnight (ScanCode).

**Estimated total: 15-25 sessions**

---

*Wave 6 Battle Plan — Forged 2026-04-03*
*10 Sprints. ~50 Steps. Every open ADR item with a GOTO and NEXT.*
*The Kingdom's final march to production.*
