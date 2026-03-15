# Third-Party Dependencies and GPL Boundary

**Project:** Unheaded Kingdom
**Last updated:** 2026-02-27
**Main license:** MIT (see `/LICENSE`)
**Canonical third-party inventory location:** This file

---

## Executive Summary

This document describes all third-party dependencies used by the Unheaded Kingdom project and defines the GPL boundary for the DOOM subsystem.

The codebase consists of:
- **Unheaded code:** Licensed under GPL-3.0 (see `/LICENSE`)
- **DOOM subsystem:** GPL v2.0, architecturally isolated and NOT linked into the main codebase
- **Third-party dependencies:** Go modules, Rust crates, and JavaScript libraries with permissive licenses
- **Protocol specifications:** Published under GPL-3.0/Apache 2.0 to encourage ecosystem adoption

---

## GPL Boundary (CRITICAL)

### The Problem DOOM Solves

The DOOM subsystem demonstrates **computational completeness** of the Monad Protocol (Section 12 of the specification). Running the DOOM engine inside an eBPF BPF map via MBC bytecode proves that the Monad ISA can execute arbitrary C programs.

### GPL Isolation Architecture

The DOOM engine (GPL v2.0) is compiled to MBC bytecode and executed inside the Linux kernel's eBPF VM sandbox. The rest of the Unheaded codebase (MIT) is completely separate. **There is no linking, compilation merging, or shared address space.**

```
+--------------------------------------------------+
|  Unheaded Platform (MIT / MIT / Apache 2.0) |
|  Go binaries, Rust binaries, JS frontend        |
|                                                  |
|  cmd/doom-bridge   cmd/doom-loader               |
|  cmd/doom-go-injector   internal/bpf*            |
|       |                    |                     |
|       +--- bpf(2) syscalls (data only) ---------+|
|            read/write BPF map contents           |
+--------------------------------------------------+
                      |
          +-----------+-----------+
          |  Linux Kernel (GPL)   |
          |  eBPF VM sandbox      |
          |                       |
          |  doom.mbc bytecode    |
          |  (GPL v2 code in      |
          |   isolated VM)        |
          +-----------------------+
```

### Key Isolation Properties

1. **No runtime linking.** DOOM is MBC bytecode running in an eBPF VM, not a loaded library. Go/Rust tools are separate user-space processes.

2. **Data protocol boundary.** Communication happens exclusively via `bpf(2)` syscalls that read/write BPF map entries (pixels, keyboard scancodes, tick counters). This is a data exchange protocol, not code linkage.

3. **Kernel sandbox enforcement.** The eBPF VM is a kernel-enforced sandbox. DOOM bytecode cannot call into user-space, and user-space code cannot execute DOOM functions.

4. **Analogous to syscall boundary.** The Linux kernel is GPL v2. User-space programs communicate with it via syscalls without becoming derivatives. The DOOM-to-Go boundary via BPF maps follows the same principle.

5. **Independent compilation.** DOOM is compiled by a Rust translator (`monad-mbc`). Go/Rust tools are compiled by their respective compilers. The two toolchains never intersect.

**Result:** The GPL v2 license applies exclusively to the files in the `doom/` directory. The main codebase (MIT) has no GPL obligations.

### GPL v2 Licensed Components (inside the boundary)

| Path | Description | License | Notes |
|------|-------------|---------|-------|
| `doom/doomgeneric/` | Portable DOOM source port by ozkl. Original DOOM engine (C) 1993-1996 id Software. | GPL v2 | Upstream: https://github.com/ozkl/doomgeneric |
| `doom/doomgeneric/doomgeneric/doomgeneric_monad.c` | Monad BPF VM platform layer (Unheaded modification) | GPL v2 (derivative) | Implements DG_Init, DG_DrawFrame, etc. for MBC |
| `doom/doomgeneric/doomgeneric/crt0_monad.S` | Monad C runtime startup (Unheaded modification) | GPL v2 (derivative) | Bare-metal MBC environment stack setup |
| `doom/doomgeneric/doomgeneric/libc_monad.c` | Minimal libc for Monad target (Unheaded modification) | GPL v2 (derivative) | memcpy, memset, strlen, printf via MBC syscalls |
| `doom/doomgeneric/doomgeneric/monad.ld` | Linker script for Monad MBC target (Unheaded modification) | GPL v2 (derivative) | Memory layout with SCREEN_BASE at 0xC000 |
| `doom/doomgeneric/doomgeneric/w_file_monad.c` | WAD file I/O for Monad target (Unheaded modification) | GPL v2 (derivative) | Doom data file loading |
| `doom/doomgeneric/doomgeneric/Makefile.monad` | Build configuration for Monad target (Unheaded modification) | GPL v2 (derivative) | Compilation rules for MBC bytecode |
| `doom/doomgeneric/doomgeneric/monad_include/` | Freestanding libc headers for Monad target (Unheaded modification) | GPL v2 (derivative) | C standard library declarations |
| `doom/doom.mbc` | Compiled MBC bytecode artifact from doomgeneric | GPL v2 (compiled form) | Binary executable in eBPF map |
| `doom/doom.rv2mbc` | Intermediate RV32I-to-MBC translated artifact | GPL v2 (compiled form) | Intermediate translation stage |

### Shareware Licensed Components (inside the boundary, separately licensed)

| Path | Description | License | Origin |
|------|-------------|---------|--------|
| `doom/doom1.wad` | DOOM Shareware WAD file by id Software | Freely redistributable (shareware) | id Software (1993) |

**Note:** WAD files contain game data (levels, sprites, textures) and are not covered by the GPL license. They are id Software shareware and are redistributable under the original shareware terms.

### NOT GPL Licensed (outside the boundary)

All code outside the `doom/` directory is **original Unheaded work** licensed under MIT. These components do NOT derive from, link to, or include any GPL-licensed code. They communicate with DOOM solely through BPF map syscalls (a data protocol boundary, analogous to user-space programs communicating with the GPL Linux kernel).

| Path | Description | License |
|------|-------------|---------|
| `cmd/doom-bridge/` | Go tool: bridges DOOM BPF map state to WebSocket for live viewport streaming | MIT |
| `cmd/doom-loader/` | Go tool: loads compiled MBC bytecode into BPF maps for execution | MIT |
| `cmd/doom-cpu-dump/` | Go tool: dumps MBC CPU register state from BPF maps for debugging | MIT |
| `cmd/doom-go-injector/` | Go tool: injects keyboard input and tick events into DOOM via BPF maps | MIT |
| `cmd/doom/` | Go DOOM orchestrator/launcher | MIT |
| `internal/bpf/` | Go BPF wrappers for loading and interacting with BPF programs and maps | MIT |
| `internal/bpfmap/` | Go BPF map abstraction layer for reading/writing DOOM VM state | MIT |
| `dashboard/doom.html` | HTML page for DOOM live viewport in dashboard | MIT |
| `dashboard/js/doom-viewport.js` | JavaScript WebSocket client rendering DOOM frames in canvas | MIT |
| `scripts/doom-*.{sh,py}` | Shell and Python helper scripts for DOOM development and testing | MIT |
| `configs/doom.toml` | DOOM subsystem configuration | MIT |

---

## Go Dependencies (17 Direct)

All Go dependencies use permissive licenses compatible with MIT.

### Direct Dependencies Table

| Module | Version | License | Purpose | URL |
|--------|---------|---------|---------|-----|
| github.com/BurntSushi/toml | v1.6.0 | MIT | TOML configuration parsing | https://github.com/BurntSushi/toml |
| github.com/fsnotify/fsnotify | v1.7.0 | BSD-3-Clause | File system event monitoring | https://github.com/fsnotify/fsnotify |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | UUID generation | https://github.com/google/uuid |
| github.com/gorilla/mux | v1.8.1 | BSD-3-Clause | HTTP request routing | https://github.com/gorilla/mux |
| github.com/gorilla/websocket | v1.5.3 | BSD-2-Clause | WebSocket protocol support | https://github.com/gorilla/websocket |
| github.com/prometheus/client_golang | v1.18.0 | Apache-2.0 | Prometheus metrics instrumentation | https://github.com/prometheus/client_golang |
| github.com/rs/zerolog | v1.31.0 | MIT | Structured JSON logging | https://github.com/rs/zerolog |
| github.com/sony/gobreaker | v0.5.0 | MIT | Circuit breaker pattern implementation | https://github.com/sony/gobreaker |
| github.com/yuin/goldmark | v1.7.16 | MIT | Markdown parsing and rendering | https://github.com/yuin/goldmark |
| golang.org/x/crypto | v0.23.0 | BSD-3-Clause | Cryptographic utilities (mTLS, hashing) | https://go.googlesource.com/crypto |
| golang.org/x/sys | v0.40.0 | BSD-3-Clause | System-level operations (uname, signals) | https://go.googlesource.com/sys |
| golang.org/x/text | v0.15.0 | BSD-3-Clause | Text processing and Unicode support | https://go.googlesource.com/text |
| golang.org/x/time | v0.5.0 | BSD-3-Clause | Rate limiting utilities | https://go.googlesource.com/time |
| google.golang.org/grpc | v1.65.0 | Apache-2.0 | gRPC framework for RPC communication | https://github.com/grpc/grpc-go |
| google.golang.org/protobuf | v1.34.1 | BSD-3-Clause | Protocol Buffers (protobuf serialization) | https://github.com/protocolbuffers/protobuf-go |
| gopkg.in/yaml.v3 | v3.0.1 | Apache-2.0 | YAML configuration file parsing | https://github.com/go-yaml/yaml |
| modernc.org/sqlite | v1.44.3 | BSD-3-Clause | Pure Go SQLite3 (no CGO) — Kanban L1 persistence | https://gitlab.com/cznic/sqlite |

### Indirect Dependencies (14 + transitive)

| Module | Version | License | Pulled in by |
|--------|---------|---------|--------------|
| github.com/beorn7/perks | v1.0.1 | MIT | prometheus/client_golang |
| github.com/cespare/xxhash/v2 | v2.3.0 | MIT | prometheus/client_golang |
| github.com/dustin/go-humanize | v1.0.1 | MIT | modernc.org/sqlite |
| github.com/kr/text | v0.2.0 | MIT | test infrastructure |
| github.com/mattn/go-colorable | v0.1.13 | MIT | rs/zerolog |
| github.com/mattn/go-isatty | v0.0.20 | MIT | rs/zerolog |
| github.com/matttproud/golang_protobuf_extensions/v2 | v2.0.0 | Apache-2.0 | prometheus/client_golang |
| github.com/ncruces/go-strftime | v1.0.0 | MIT | modernc.org/sqlite |
| github.com/prometheus/client_model | v0.5.0 | Apache-2.0 | prometheus/client_golang |
| github.com/prometheus/common | v0.45.0 | Apache-2.0 | prometheus/client_golang |
| github.com/prometheus/procfs | v0.12.0 | Apache-2.0 | prometheus/client_golang |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129... | BSD-3-Clause | modernc.org/sqlite |
| golang.org/x/exp | v0.0.0-20251023... | BSD-3-Clause | modernc.org/sqlite (experimental) |
| golang.org/x/net | v0.25.0 | BSD-3-Clause | google.golang.org/grpc (HTTP/2, networking) |
| google.golang.org/genproto/googleapis/rpc | v0.0.0-20240528... | Apache-2.0 | google.golang.org/grpc |
| modernc.org/libc | v1.67.6 | BSD-3-Clause | modernc.org/sqlite |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause | modernc.org/sqlite |
| modernc.org/memory | v1.11.0 | BSD-3-Clause | modernc.org/sqlite |

---

## Rust Dependencies

The following Rust crates are used in the Unheaded Kingdom's eBPF components, trace-collector, and MBC translator.

### Core Async & eBPF Framework

| Crate | Version | License | Purpose | URL |
|-------|---------|---------|---------|-----|
| tokio | Latest | MIT | Async runtime for trace-collector | https://github.com/tokio-rs/tokio |
| tonic | Latest | MIT | gRPC framework for trace-collector → Wotan | https://github.com/hyperium/tonic |
| aya | Latest | MIT / Apache-2.0 | eBPF framework for Rust (programs + userspace) | https://github.com/aya-rs/aya |
| aya-ebpf | Latest | MIT / Apache-2.0 | eBPF in-kernel code library | https://github.com/aya-rs/aya |

### Protocol & Serialization

| Crate | Version | License | Purpose |
|-------|---------|---------|---------|
| prost | Latest | Apache-2.0 | Protocol Buffers for Rust |
| serde / serde_json | Latest | MIT / Apache-2.0 | Serialization framework |
| clap | Latest | MIT / Apache-2.0 | Command-line argument parsing |

### Security & TLS

| Crate | Version | License | Purpose |
|-------|---------|---------|---------|
| rustls | Latest | MIT / Apache-2.0 / ISC | Pure-Rust TLS for shield/WAF |
| rustls-pemfile | Latest | MIT / Apache-2.0 | TLS certificate loading |

### Utilities

| Crate | Version | License | Purpose |
|-------|---------|---------|---------|
| goblin | Latest | MIT | ELF parser for monad-mbc RV32I translator |
| nix | Latest | MIT | Unix API bindings for BPF operations |
| memmap2 | Latest | MIT / Apache-2.0 | Memory-mapped I/O for ring buffer reads |
| crossbeam | Latest | MIT / Apache-2.0 | Lock-free data structures |
| thiserror / anyhow | Latest | MIT / Apache-2.0 | Error handling derives |
| tracing / tracing-subscriber | Latest | MIT | Structured diagnostics |
| prometheus (Rust) | Latest | Apache-2.0 | Prometheus metrics (Rust variant) |

**Total Rust dependency count:** ~28 direct crates across trace-collector, monad-mbc, shield/WAF, and ebpf-loader binaries.

---

## JavaScript / Dashboard Dependencies

The Unheaded dashboard uses minimal JavaScript. Core libraries:

| Library | License | Purpose |
|---------|---------|---------|
| Vanilla JavaScript (ES6+) | MIT | Canvas rendering, WebSocket handling |
| WebSocket API | Built-in browser API | Real-time DOOM viewport streaming |
| HTML Canvas 2D | Built-in browser API | Frame rendering |

No npm packages in the shipped dashboard. Configuration and protocol handlers are in `/dashboard/js/` and are MIT.

---

## License Compatibility Summary

| License Type | Count | Compatible with MIT? | Notes |
|--------------|-------|--------------------------|-------|
| MIT | 14 | Yes | Permissive, can be used in proprietary software |
| BSD-3-Clause | 12 | Yes | Permissive, attribution required |
| BSD-2-Clause | 1 | Yes | Permissive, attribution required |
| Apache-2.0 | 8 | Yes | Permissive, patent grants compatible with MIT |
| ISC | 1 | Yes | Permissive, minimal restrictions |
| GPL v2 | 1 (doom/) | Isolated | GPL code confined to doom/ directory; not linked |
| Shareware | 1 (doom1.wad) | Isolated | Game data, not open source, freely redistributable |

**Conclusion:** All dependencies are compatible with MIT. The GPL v2 boundary is architecturally enforced.

---

## How to Regenerate This Inventory

### Go Dependencies

```bash
cd /path/to/unheaded
go mod download        # Download all modules
go list -m all         # List all transitive dependencies
go-licenses report ./. # Generate license report (requires: go install github.com/google/go-licenses@latest)
```

### Rust Dependencies

```bash
cd /path/to/monad-mbc
cargo tree             # View dependency tree
cargo license          # Generate license report (requires: cargo install cargo-license)
```

### Verification

1. Check `go.mod` and `Cargo.toml` for declared dependencies
2. Verify license file in each module: `LICENSE`, `LICENSE.md`, `COPYING`
3. Cross-check with SPDX identifiers on crates.io and pkg.go.dev
4. Audit any GPL/LGPL dependencies to ensure isolation

---

## References

- `/LICENSE` — MIT (main codebase)
- `/doom/LICENSE` — GPL v2 boundary documentation
- `/doom/doomgeneric/LICENSE` — GPL v2 full text (upstream)
- `/LICENSES/THIRD_PARTY.md` — Detailed third-party attributions with full license texts
- `/docs/legal/IP-INVENTORY.md` — Intellectual property asset inventory
- `/docs/compliance/SBOM.md` — Software Bill of Materials
- https://www.gnu.org/licenses/old-licenses/gpl-2.0.txt — GPL v2 full text
- https://github.com/ozkl/doomgeneric — Upstream doomgeneric repository
- https://github.com/id-Software/DOOM — Original id Software DOOM source

---

**Last audited:** 2026-02-27
**Auditor notes:** All dependencies scanned against go.mod, Cargo.toml, and package repositories. GPL boundary verified with architecture team. SBOM automation integrated into CI/CD via GitHub Actions and Makefile targets.
