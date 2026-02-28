# Unheaded Kingdom Battle Plans

This directory contains exhaustive battle plans for the Unheaded infrastructure deployment.

## Session 74: BARE METAL LIGHTNING — Full Kingdom Deployment

**File:** `Session_74_2026_02_27_BATTLE-PLAN-part1.md`
**Status:** PART 1 COMPLETE ✓
**Size:** 2,149 lines, 72 KB
**Phases Covered:** 0, 1, 2 (+ outline of 3-12)
**Duration:** ~100 minutes execution time for Part 1

### Contents

- **Phase 0:** Intelligence & Environment Verification (15 steps, ~25 min)
  - Git validation, build/test PASS verification, eBPF ELF targets
  - Toolchain inventory (Go 1.24+, Rust, Nix, Docker, bpftool, etc.)
  
- **Phase 1:** Wire Format Freeze — S67 RFC Overlap (40 steps, ~40 min)
  - Monad draft-05 wire format analysis (8-bit flags breakdown)
  - RFC collision detection, IANA registries draft
  - Breaking changes identification & scoring
  - Version locks: Monad v5.0.0, Sophia v2.0.0, Wotan v1.0.0
  
- **Phase 2:** WEST Bare Metal — NixOS Bootstrap (56 steps total, shown through Step 42, ~35 min)
  - WEST hardware verification
  - NixOS installation from flake.nix (partition → install → reboot)
  - FRR BGP AS 65001 configuration
  - Firewall rules (Moat Ghost pattern: iptables DROP default)
  - WireGuard tunnel setup (fd00:dead:beef::1/64)
  - IPv6 stack verification, BGP peering readiness

### Key Features

✓ **Exhaustive Style (S42-S50 aligned)**
  - Every step has exact bash command(s)
  - 40+ debug branches [D] with fallback strategies
  - 3 commit checkpoints [C] (Phase 0, 1, 2)
  - Clear phase EXIT GATES with completion checklists

✓ **Services Coverage**
  - All 23 Unheaded services documented
  - Deployment order: doom → doom-bridge → ... → waf
  - Port assignments (Doom Range 16666-26666)
  - Health checks, service discovery, log aggregation

✓ **Infrastructure Design**
  - WEST: AS 65001, fd00:dead:beef::1/64
  - EAST: AS 65002, fd00:dead:beef::2/64 (Phase 4+)
  - BGP peering, WireGuard tunnel, IPv6 routing
  - Firewall (Moat Ghost), security hardening

### Quick Links

- **Start Here:** Phase 0 — Intelligence & Environment Verification
  ```bash
  # Step 1: Validate git repository
  cd /tmp/unheaded
  git status
  git log --oneline -10
  git branch -a | grep -i s74
  ```

- **Execute Phase 0:** Steps 1-15 (~25 min)
- **Execute Phase 1:** Steps 16-55 (~40 min) 
- **Execute Phase 2:** Steps 30-85 (~35+ min for full phase)

### Status & Next Steps

**PART 1 STATUS:** COMPLETE ✓
- Phase 0 definition: COMPLETE
- Phase 1 definition: COMPLETE
- Phase 2 definition: COMPLETE (through Step 42, with full outline)
- Phases 3-12: Outlined in agent assignment matrix

**READY FOR:**
- Phase 0 execution (Environment verification)
- Phase 1 execution (Wire format freeze)
- Phase 2 execution (WEST NixOS bootstrap)

**PENDING:**
- PART 2 (Phases 3-12, ~250+ additional steps, 20+ hours)

### Legend

| Tag | Meaning | 
|-----|---------|
| [HARDWARE] | Hardware verification/physical operations |
| [GIT] | Git operations, branching, commits |
| [BUILD] | Build operations (go, cargo, nix) |
| [TEST] | Test execution |
| [EBPF] | eBPF compilation and verification |
| [NIXOS] | NixOS installation and boot |
| [NETWORK] | Network configuration, routing |
| [FIREWALL] | Firewall rules and security |
| [WIREGUARD] | WireGuard tunnel setup |
| [BGP] | BGP routing configuration |
| [SERVICE] | Service deployment and health checks |
| [DISCOVERY] | Service discovery integration |
| [OBSERVABILITY] | Logging, metrics, tracing |
| [PROTOCOL] | Protocol specification work |
| [D] | Debug branch (failure recovery) |
| [C] | Commit checkpoint |

### Prerequisites

Before starting Phase 0:
- [ ] S73 complete (git repo accessible)
- [ ] go build ./... PASS
- [ ] go test ./... PASS
- [ ] 10+ eBPF ELF targets compiled
- [ ] Go 1.24+, Rust stable, Nix, Docker available
- [ ] WEST bare metal machine accessible

### Organization

```
/references/
  ├── Session_74_2026_02_27_BATTLE-PLAN-part1.md  (this file, 2,149 lines)
  ├── README.md (this index)
  └── [Part 2 to be created] (forthcoming, Phases 3-12)
```

### Metrics

- **Total Lines:** 2,149
- **Total Size:** 72 KB
- **Bash Commands:** 70+
- **Debug Branches:** 40+
- **Commit Checkpoints:** 3
- **Service Coverage:** 23 unique services
- **Phase Coverage:** 0, 1, 2 (+ outline of 3-12)
- **Time Estimate:** ~100 min for Part 1 execution

---

**Created:** 2026-02-27
**Author:** Warmonger, Battle Planner of the Unheaded Kingdom
**Status:** PRODUCTION-READY for Phase 0-2 execution
