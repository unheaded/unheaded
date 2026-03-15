# The Unheaded Chronicles

## A Living Grimoire of the Kingdom's Journey

**STATUS:** ALPHA COMPLETE → AGE 2 IN PROGRESS
**LAST UPDATED:** March 15, 2026
**LOC:** ~385K production, ~941K total
**WIRE FORMAT:** FROZEN v0x01 (20 bytes)
**BARE METAL:** WEST + EAST online

---

### Age 0: The Foundation Stone (✅ COMPLETED)

Core protocol design, Monad wire format, initial service scaffolding.

### Age 1: The Alpha Ascension (✅ COMPLETED)

**Progress:** 100%

All 10 services operational. Dashboard, Kanban, eBPF tracing, Wotan message bus, gateway routing. S36 Four Pillars (Port Authority, gRPC-First Transport, Log Aggregation, Service Discovery). 75+ sprints executed.

### Age 2: The Beta Trials (🔄 IN PROGRESS)

**Progress:** 55%

#### Completed Epics
- ✅ S51: Auth Framework (64 tests, Noop/APIKey/JWT/RBAC)
- ✅ S52: Legal & Compliance (SPDX headers, GPL-3.0)
- ✅ S59: Dashboard Polish (design system, demo data, review column)
- ✅ S67: Wire Format Freeze (v0x01, 12 IANA registries)
- ✅ S67-S69: Multi-agent swarm (observability, Suricata IDS, alternate routing)
- ✅ S76: EAST bare metal online, cross-host BPF flow graph
- ✅ S-WEST: Bootstrap & hardening (kernel 6.17.0-19, 0 failed services)
- ✅ S-ZHEN: Zhen AI Champion — local RAG/LLM system operational

#### S-ZHEN Sprint (March 15, 2026) — MASSIVE
- ✅ Mistral-7B inference on GPU via llama.cpp + ROCm (port 20100)
- ✅ RAG pipeline: 594K vectors indexed (21K Ring 1 + 573K Ring 2-4)
- ✅ Web UI with Chat + Search tabs (port 20103)
- ✅ 616 RAFT QA training pairs generated
- ✅ QLoRA training pipeline scaffolded
- ✅ Zhen-Claude bridge (context API, skill ingestion, 19 skills loaded)
- ✅ Corpus: 16 GitHub repos, Linux kernel, 9,739 RFCs, 1,649 ArXiv papers
- ✅ Downloads: Wikipedia (24GB), Stack Overflow (22GB), ServerFault, ArXiv

#### S-PQC Sprint — Post-Quantum Cryptography
- ✅ ML-DSA (FIPS 204): Full implementation via cloudflare/circl
- ✅ ML-KEM (FIPS 203): Full implementation via cloudflare/circl
- ✅ SLH-DSA (FIPS 205): Full implementation via cloudflare/circl v1.6.3
- 🔄 FN-DSA (FIPS 206): Improved stub (no Go library yet, FPU incompatible with eBPF)
- 🔄 HQC (FIPS 207): Improved stub (FIPS 207 too new for Go ecosystem)
- ✅ 7-point verification pipeline, 4 compliance tiers, sovereign multi-sig
- ✅ 60 PQC tests passing

#### UPC Dream Ladder (Doom → Linux)
- ✅ Level 3: Framebuffer pipeline optimization (userspace bulk copy)
- ✅ Level 4a: Timer interrupt emulation (IVT, INT/IRET, ~12Hz)
- ✅ Level 4b: Syscall dispatch (6 Linux syscalls, INT 0x80)
- ✅ Level 4c: Round-robin scheduler (4 processes, SYS_FORK)
- ✅ Level 4d: MMU/paging emulation (2-level page table, 64-entry TLB)
- ✅ Level 4e: Block device (4MB ramdisk, 8192 blocks)
- ✅ Level 4f: Console I/O (TTY_MAP, SYS_WRITE to stdout)
- ✅ Level 5: Boot protocol + kernel loader (wotan-ctl boot)
- ✅ Level 5a: MiniKernel proof of concept (404 bytes, 65 instructions)
- 📋 Level 5b-5d: Minimal kernel (xv6/FUZIX port)
- 🌙 Level 6: Linux boots on UPC

#### The Well (PostgreSQL)
- ✅ Multi-database layout: unheaded_app, unheaded_ops, unheaded_config
- ✅ 7 service-scoped users with least-privilege grants
- ✅ Smart health aggregation (UPSERT current, transitions only, hourly rollup)
- ✅ Audit events with chain hashing (90-day retention)
- ✅ Security hardening (scram-sha-256, pg_hba, RLS)
- 🔄 Service wiring: Zhen, Kanban, Dashboard, Timeguru, Daemon

#### Infrastructure
- ✅ Wotan goroutine leak fixed (topic_service.go)
- ✅ 4 service stubs implemented (BPF maps, LB sync, gateway reload, WAL)
- ✅ Kanban drag-and-drop + layout fix
- ✅ Road to Linux spec documented

#### Remaining Epics
- 📋 WireGuard IPv6 overlay (fd00:dead:beef::/48)
- 📋 Foundation spec draft-06 (IANA integration)
- 📋 Sophia draft-03 (sub-dictionary types, QPACK compression)
- 📋 Wotan draft-03 (error code taxonomy)
- 📋 SBOM generation (pre-public)
- 📋 CI/CD hardening (GHA + Jenkinsfiles)
- 📋 Demo video + README polish
- 📋 RAFT fine-tuning (QLoRA on Mistral-7B with 616 QA pairs)
- 📋 Zhen Engine (custom Rust inference + Go management plane)

### Age 3: The Public Release (📋 PLANNED)

First public release. SBOM complete, CI/CD hardened, demo video, README polished. Protocol specs submitted to IETF.

### Age 4: The MVP Era (📋 PLANNED)

Production hardening, commercial licensing, multi-tenant support.

### Age 5: The Scaling Era (📋 PLANNED)

Enterprise features, global distribution, marketplace.

---

*Synced: 2026-03-15 15:30:00 UTC*
