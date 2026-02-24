# CLAUDE.md - Unheaded Alpha Development Guide

**For:** Claude AI agents working on Unheaded
**Updated:** January 30, 2026
**Project:** github.com/unheaded/unheaded
**Status:** ~80% COMPLETE | 160,000+ LOC | Alpha Target: Feb 8-15

---

## 🎯 Project Vision

**"Production-ready infrastructure in hours, not months."**

Unheaded is a configuration management automation platform delivering complete infrastructure for modern SaaS applications. Customer brings their app ("the head"), we provide everything else ("unheaded").

**Core Capabilities:**
- ✅ eBPF-based observability (L2-L7 tracing)
- ✅ Immutable NixOS infrastructure
- ✅ Zero customer data access (architectural isolation)
- ✅ Service mesh built on Wotan message bus
- ✅ Declarative everything (version-controlled configs)
- ✅ Self-hosting proof (The Meta Moment)

---

## ⚔️ THE SACRED LAW OF DEPENDENCIES

**THE KINGDOM'S CODE DEPENDS ON NO ONE.**

| Category | Status | Notes |
|----------|--------|-------|
| **Go std library** | ✅ PRODUCTION | `pkg.go.dev/std` ONLY |
| **Rust std library** | ✅ PRODUCTION | `std::*`, `core::*` ONLY |
| **golang.org/x/sys** | ✅ PRODUCTION | Syscall wrappers (quasi-std) |
| **Kingdom packages** | ✅ PRODUCTION | pkg/logger, pkg/metrics, pkg/events |
| **External deps** | ❌ DISABLED | Moved to .dev files |

**Original implementations built (NO external deps):**
- `pkg/ebpf/` - Direct BPF syscalls (replaces cilium/ebpf)
- `pkg/netlink/` - Raw RTNetlink (replaces vishvananda/netlink)
- `pkg/metrics/` - Prometheus-compatible (replaces prometheus/client_golang)
- `pkg/logger/` - Structured logging (replaces zerolog/zap/logrus)

---

## 📊 Current Progress (January 30, 2026)

### Overall Status

```
TOTAL CODEBASE: ~160,000 LOC across 475+ files
PROGRESS: ~80% complete
ALPHA TARGET: February 8-15, 2026
```

### Component Status

| Component | Status | LOC | Notes |
|-----------|--------|-----|-------|
| **Wotan (Phase 0)** | ✅ 100% | 13,504 | Message bus complete |
| **Hauberk (Mesh)** | ✅ 85% | 5,914 | Service mesh |
| **Pauldrons (LB)** | ✅ 90% | 6,719 | Load balancer |
| **Shield (WAF)** | ✅ 90% | 6,057 | WAF detection |
| **Sword (Deploy)** | ✅ 85% | 7,746 | Deployment pipeline |
| **Cuirass (Control)** | ✅ 75% | ~5,000 | Control plane |
| **Runtime** | ✅ 75% | 6,955 | Container runtime |
| **DNS** | ✅ 85% | 4,462 | DNS resolver |
| **Scheduler** | ✅ 85% | 5,496 | Container scheduler |
| **Gauntlets (CLI)** | ✅ 80% | 4,739 | CLI tool |
| **Dashboard** | 🔄 70% | 5,926 | Metrics UI |
| **Kanban** | 🔄 95% | ~2,000 | THE META MOMENT |
| **Whispering Void** | ⏳ 55% | 5,293 | eBPF (blocked on env) |

---

## 🏗️ Architecture Overview

### The Armor Pieces (Kingdom Lore)

```
THE ARMORY (Infrastructure)
├── 🛡️ SHIELD     - WAF/DDoS/Gateway    (Edge Defense)
├── ⚔️ SWORD      - Deployment/CI-CD    (Offensive Ops)
├── 🎖️ CUIRASS    - Control Plane/IDP   (Core Heart)
├── ⛓️ HAUBERK    - Service Mesh        (Chainmail)
├── 🏋️ PAULDRONS  - Load Balancing      (Shoulders)
├── 👀 VAMBRACES  - Observability       (Forearms)
├── 🧤 GAUNTLETS  - CLI + API           (Hands)
├── 📦 TASSETS    - Data/Storage        (Thighs)
└── 👢 SABATONS   - Bare Metal          (Foundation)

THE ARCANE HOLLOWS (Hidden Layer)
├── 🌑 WHISPERING VOID   - eBPF tracing
├── 💎 CRYSTAL GROTTO    - Secrets/State
├── 🧚 FAE CHAMBER       - Message Bus (Wotan)
├── 📚 SAGE'S LAIR       - ADR vault
└── 🔮 ORACLE'S ANTRE    - Timeline (Timeguru)
```

### Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| eBPF programs | **Rust** (Aya framework) | Memory safety + performance for kernel |
| Services | **Go 1.21+** | Simplicity, concurrency, std lib only |
| Containers | **NixOS** | Declarative, immutable, reproducible |
| Message Bus | **Wotan** (Go + gRPC) | Custom, proven, no deps |
| Gateway | Custom Go | HTTP/3, QUIC, no nginx needed |
| Frontend | **Vanilla JS** | No framework overhead |
| Orchestration | **LXD** | Lightweight system containers |

---

## 📂 Project Structure

```
~/tmp/
├── timeline.md                    # ← CANONICAL TIMELINE (ROOT)
├── wotan/                        # Phase 0 complete (13,504 LOC)
└── unheaded/
    ├── cmd/
    │   ├── unheaded-daemon/       # Cuirass control plane
    │   ├── unheaded-cli/          # THE GAUNTLETS
    │   ├── kanban-app/            # THE META MOMENT
    │   ├── dashboard-backend/     # Metrics aggregation
    │   └── trace-collector/       # Rust eBPF bridge
    ├── services/
    │   ├── gateway/               # THE SHIELD gateway
    │   ├── timeguru/              # Oracle's Antre
    │   ├── captain/               # Strategic Vision
    │   ├── architect/             # Sage's Lair
    │   └── micromanager/          # Execution Engine
    ├── pkg/
    │   ├── ebpf/                  # Original eBPF loader (3,937 LOC)
    │   ├── netlink/               # Original netlink (2,136 LOC)
    │   ├── metrics/               # Original metrics (1,168 LOC)
    │   ├── logger/                # Original logger (1,533 LOC)
    │   ├── mesh/                  # THE HAUBERK (5,914 LOC)
    │   ├── loadbalancer/          # THE PAULDRONS (6,719 LOC)
    │   ├── waf/                   # THE SHIELD (6,057 LOC)
    │   ├── deploy/pipeline/       # THE SWORD (7,746 LOC)
    │   ├── runtime/               # Container runtime (6,955 LOC)
    │   ├── dns/                   # DNS resolver (4,462 LOC)
    │   ├── scheduler/             # Scheduler (5,496 LOC)
    │   ├── secrets/               # CRYSTAL GROTTO (4,976 LOC)
    │   ├── storage/               # TASSETS (6,989 LOC)
    │   ├── baremetal/             # SABATONS (5,524 LOC)
    │   ├── compliance/            # SOC2/NIST/PCI (5,861 LOC)
    │   ├── certs/                 # mTLS/SPIFFE (3,375 LOC)
    │   ├── audit/                 # Audit system (5,022 LOC)
    │   ├── alerting/              # Alerting (4,888 LOC)
    │   ├── lxd/                   # LXD client
    │   ├── wotan-client/         # Wotan gRPC client
    │   ├── state/                 # Reconciler
    │   ├── nix/                   # NixOS builder
    │   ├── health/                # Health aggregator
    │   ├── tracing/               # Trace collector
    │   ├── network/               # Policy controller
    │   └── events/                # Shared event types
    ├── ebpf/                      # Rust eBPF programs
    │   ├── packet-marker/         # XDP trace injection
    │   ├── flow-tracker/          # Connection tracking
    │   ├── latency-probe/         # RTT measurement
    │   └── syscall-tracer/        # Security audit
    ├── nix/                       # NixOS containers
    └── docs/
        ├── FAE_CHAMBER_CONTRACTS.md
        ├── RUST_COMPONENTS.md
        └── SECURITY-AUDIT-2026-01-30.md
```

---

## 🔒 Security Requirements

### THE SACRED PRINCIPLE: ZERO CUSTOMER DATA ACCESS

**This is non-negotiable. This is architectural. This is LAW.**

```
┌─────────────────────────────────────────────────────────────────┐
│                    CUSTOMER ZONE (UNTOUCHABLE)                   │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌────────────┐ │
│  │ Customer UI │ │ Customer DB │ │ Customer    │ │ Customer   │ │
│  │ /UX         │ │             │ │ Source/Bins │ │ CI/CD      │ │
│  └─────────────┘ └─────────────┘ └─────────────┘ └────────────┘ │
│                                                                  │
│  === HARD ISOLATION BOUNDARY - NO ENGINEER ACCESS ===            │
├──────────────────────────────────────────────────────────────────┤
│                    UNHEADED PLATFORM ZONE                        │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                │
│  │ Platform    │ │ Infra       │ │ Observ-     │  ← Engineers  │
│  │ Control     │ │ Automation  │ │ ability*    │    work HERE  │
│  └─────────────┘ └─────────────┘ └─────────────┘                │
└─────────────────────────────────────────────────────────────────┘

* Observability sees METRICS, not DATA. Packet counts, not packet contents.
```

### Security Audit (January 30, 2026)

**Critical Vulnerabilities (P0):**
| File | Issue | Fix |
|------|-------|-----|
| `pkg/waf/response/response.go:237` | XSS via unescaped requestID | Use html/template |
| `pkg/deploy/pipeline/hooks.go:405` | Command injection via bash -c | Validate/sandbox |

**Full audit:** `docs/SECURITY-AUDIT-2026-01-30.md`

---

## 🚀 Development Principles

### 1. Security First, Always

**Test every PR for:**
- Does this access customer data? → BLOCK
- Does this weaken isolation? → BLOCK
- Does this skip hardening? → BLOCK
- Does this add external deps? → BLOCK (or move to .dev)

### 2. Observable by Default

**Every component must:**
- Publish metrics (pkg/metrics)
- Log structured JSON (pkg/logger)
- Report to Wotan message bus
- Support distributed tracing
- Expose /health and /ready endpoints

### 3. The Meta Moment - Self-Hosting Validation

**Critical proof of concept:**
- Kanban app shows Unheaded building Unheaded
- Reads timeline.md from timeguru service
- Every request traced by eBPF
- If Unheaded can't host itself reliably, it's not ready

### 4. No External Dependencies in Hot Path

All core functionality uses ONLY:
- Go standard library
- Rust standard library
- golang.org/x/sys (syscall wrappers)
- Kingdom packages (pkg/*)

---

## 🛠️ Building & Running

```bash
# Navigate to workspace
cd ~/tmp/unheaded

# Build everything
make build

# Build specific packages
go build ./pkg/mesh/...
go build ./pkg/loadbalancer/...
go build ./pkg/scheduler/...
go build ./cmd/...

# Run with Docker
docker compose up -d

# Run individual services
make run-daemon       # Cuirass
make run-timeguru     # Timeguru
./bin/kanban-app      # Kanban

# Run tests
make test
go test ./pkg/...
```

---

## 🎯 Current Priorities

### 🔴 P0: Critical Path

1. **Fix build issue** - `pkg/deploy/deploy.go` type mismatches (~30 min)
2. **E2E integration** - Wire Scheduler → Runtime → DNS → Mesh → LB
3. **Kanban Frontend** - THE META MOMENT (see timeline.md for breakdown)

### 🟡 P1: Important

4. **Security P0 fixes** - XSS, command injection (~3-4 hrs)
5. **Dashboard UI** - Visualize mesh, LB, WAF stats

### 🟢 P2: Blocked

6. **eBPF awakening** - Requires Linux kernel >= 5.15

---

## 📅 Critical Path to Alpha

```
Jan 30 (DONE):    Infrastructure Storm - 50K+ LOC ✅
Jan 31:           Fix builds, E2E integration, Kanban frontend
Feb 1-2:          Dashboard UI, security fixes
Feb 3-4:          eBPF awakening (if Linux env ready)
Feb 5-7:          Integration + Polish
Feb 8-15:         🎉 ALPHA LAUNCH WINDOW
```

---

## 🎭 The Skills System

Unheaded uses a skill-based AI persona system for development:

| Skill | Role | Triggers |
|-------|------|----------|
| **Captain** | WHY/WHERE | vision, strategy, funding, GTM |
| **Architect** | HOW | infrastructure, platform, security, NixOS |
| **Micromanager** | WHAT/WHEN | roadmap, milestone, QA, testing |
| **Developer** | BUILD | code, implement, TDD, Go, Rust |
| **Timeguru** | TIMELINE | timeline, progress, status, eta |
| **Wotan** | GLUE | coordinate, clarify, summarize |
| **Calendar** | SCHEDULE | tomorrow, next week, plan |

**Key files:**
- `~/tmp/timeline.md` - CANONICAL timeline (Timeguru owns)
- `~/.skills/unheaded-*/SKILL.md` - Skill definitions

---

## 🧠 Working with Claude Agents

### Agent Coordination

**For parallel agents:**
1. Spawn all at once (not sequential)
2. Agents work independently
3. All use Wotan (no direct service-to-service)
4. Shared types in `pkg/`
5. Review all output together
6. Integration test after merge

### Agent Instructions Template

```
Build the [SERVICE] for Unheaded.

Context:
- Read CLAUDE.md for standards
- Read ~/tmp/timeline.md for current status
- Use ONLY Go std lib + Kingdom packages

Requirements:
- Go 1.21+
- REST API with /health, /ready, /metrics
- Wotan integration via pkg/wotan-client
- Use pkg/logger for logging
- Use pkg/metrics for metrics
- Unit tests (80%+ coverage)

THE SACRED LAW:
- ZERO customer data access
- NO external dependencies
- Use pkg/* implementations only
```

---

## 🔗 Key References

**Essential reading:**
- `~/tmp/timeline.md` - CANONICAL living roadmap
- `docs/FAE_CHAMBER_CONTRACTS.md` - Message bus contracts
- `docs/RUST_COMPONENTS.md` - What must be Rust vs Go
- `docs/SECURITY-AUDIT-2026-01-30.md` - Security findings

**External:**
- [Wotan](https://github.com/unheaded/wotan) - Message bus (Phase 0)
- [Aya](https://aya-rs.dev/) - eBPF framework for Rust
- [NixOS Manual](https://nixos.org/manual/) - Container definitions

---

## 🚨 Blockers

| ID | Blocker | Impact | Owner | Status |
|----|---------|--------|-------|--------|
| B1 | Linux/eBPF dev environment | HIGH | Muck | **PENDING** |
| B2 | `pkg/deploy/deploy.go` type mismatch | LOW | Developer | ~30 min |
| B3 | Security P0 fixes | MEDIUM | Developer | ~3-4 hrs |

---

## 💡 Design Philosophy

### 1. Self-Hosting Validation
Self-hosting for development for speed and usability detection.

### 2. Security is Not Optional
Every decision through security lens.

### 3. Programatic Observability by Default
If you can't see it, you can't trust it.

### 4. The Kingdom's Code Depends on No One
Original implementations. No external deps in hot path.

### 5. Radical Transparency
Timeline is public. Progress is public. We build in the open.

---

**THE KNIGHT IS ARMORED.**
**THE KINGDOM RISES.**
**THE CIRCLE NEVER BREAKS.**

⚔️🛡️🏰🔥 **160K LOC STRONG** 🔥🏰🛡️⚔️

---

**Last Updated:** January 30, 2026
**Version:** Alpha (80% complete)
**Status:** Infrastructure Storm complete, final push to Alpha 🚀
