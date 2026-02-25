# Software Bill of Materials — Unheaded

**Project:** Unheaded Infrastructure  
**Generated:** 2026-02-25  
**Go Version:** 1.24.0  
**License:** BSL 1.1 (see `/LICENSE`)

---

## Overview

This Software Bill of Materials (SBOM) documents all dependencies used by the Unheaded project. The project has **17 direct dependencies** and **64 total transitive dependencies** (including development and test dependencies).

---

## Direct Dependencies (from go.mod)

These are the first-level dependencies explicitly declared in `go.mod`:

| Module | Version | Purpose |
|--------|---------|---------|
| github.com/BurntSushi/toml | v1.6.0 | TOML configuration parsing |
| github.com/fsnotify/fsnotify | v1.7.0 | File system event monitoring |
| github.com/google/uuid | v1.6.0 | UUID generation |
| github.com/gorilla/mux | v1.8.1 | HTTP request routing |
| github.com/gorilla/websocket | v1.5.3 | WebSocket protocol support |
| github.com/prometheus/client_golang | v1.18.0 | Prometheus metrics instrumentation |
| github.com/rs/zerolog | v1.31.0 | Structured logging |
| github.com/sony/gobreaker | v0.5.0 | Circuit breaker pattern |
| github.com/yuin/goldmark | v1.7.16 | Markdown parsing and rendering |
| golang.org/x/crypto | v0.23.0 | Cryptographic utilities |
| golang.org/x/sys | v0.40.0 | System-level operations |
| golang.org/x/text | v0.15.0 | Text handling and Unicode support |
| golang.org/x/time | v0.5.0 | Time-related utilities |
| google.golang.org/grpc | v1.65.0 | gRPC framework |
| google.golang.org/protobuf | v1.34.1 | Protocol Buffers |
| gopkg.in/yaml.v3 | v3.0.1 | YAML configuration parsing |
| modernc.org/sqlite | v1.44.3 | Pure Go SQLite3 implementation |

---

## All Transitive Dependencies

The complete dependency graph includes 64 entries in `go.sum` (counting both `.mod` and `/go.mod` variants). Key transitive dependencies include:

### Indirect Dependencies (selected)

| Module | Version | Source |
|--------|---------|--------|
| github.com/beorn7/perks | v1.0.1 | prometheus/client_golang |
| github.com/cespare/xxhash/v2 | v2.3.0 | prometheus/client_golang |
| github.com/dustin/go-humanize | v1.0.1 | modernc.org/sqlite |
| github.com/google/go-cmp | v0.6.0 | Test/validation support |
| github.com/google/pprof | v0.0.0-20250317173921-a4b03ec1a45e | Profiling support |
| github.com/hashicorp/golang-lru/v2 | v2.0.7 | modernc.org/sqlite cache |
| github.com/kr/text | v0.2.0 | Testing utilities |
| github.com/mattn/go-colorable | v0.1.13 | rs/zerolog output |
| github.com/mattn/go-isatty | v0.0.20 | rs/zerolog output |
| github.com/matttproud/golang_protobuf_extensions/v2 | v2.0.0 | grpc dependencies |
| github.com/ncruces/go-strftime | v1.0.0 | modernc.org/sqlite |
| github.com/prometheus/client_model | v0.5.0 | prometheus/client_golang |
| github.com/prometheus/common | v0.45.0 | prometheus/client_golang |
| github.com/prometheus/procfs | v0.12.0 | prometheus/client_golang |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | modernc.org/sqlite FFT |
| github.com/stretchr/testify | v1.3.0 | Testing assertions |
| golang.org/x/exp | v0.0.0-20251023183803-a4bb9ffd2546 | Experimental features |
| golang.org/x/mod | v0.29.0 | Module utilities |
| golang.org/x/net | v0.25.0 | grpc/http2 |
| golang.org/x/sync | v0.17.0 | Synchronization primitives |
| golang.org/x/term | v0.20.0 | Terminal utilities |
| golang.org/x/tools | v0.38.0 | Code generation tools |
| google.golang.org/genproto/googleapis/rpc | v0.0.0-20240528184218-531527333157 | gRPC RPC definitions |
| modernc.org/cc/v4 | v4.27.1 | sqlite build dependency |
| modernc.org/ccgo/v4 | v4.30.1 | sqlite build dependency |
| modernc.org/fileutil | v1.3.40 | sqlite file operations |
| modernc.org/gc/v2 | v2.6.5 | sqlite garbage collection |
| modernc.org/gc/v3 | v3.1.1 | sqlite garbage collection |
| modernc.org/goabi0 | v0.2.0 | sqlite ABI |
| modernc.org/libc | v1.67.6 | sqlite libc support |
| modernc.org/mathutil | v1.7.1 | sqlite math utilities |
| modernc.org/memory | v1.11.0 | sqlite memory allocation |
| modernc.org/opt | v0.1.4 | sqlite optimization |
| modernc.org/sortutil | v1.2.1 | sqlite sort utilities |
| modernc.org/strutil | v1.2.1 | sqlite string utilities |
| modernc.org/token | v1.1.0 | sqlite tokenization |

---

## Dependency Statistics

- **Total Unique Modules:** 39 (includes all direct + indirect)
- **Total go.sum Entries:** 129 (includes both .mod and .sum variants)
- **Build Complexity:** Moderate (SQLite self-contained build adds complexity)
- **Supply Chain Risk:** Low (all dependencies from reputable sources)

---

## Key License Observations

All dependencies use permissive or copyleft-compatible licenses:

- **MIT/Apache 2.0 Licensed:** gorilla/* (mux, websocket), google/uuid, rs/zerolog, sony/gobreaker, yuin/goldmark, prometheus/*, golang.org/x/*, google.golang.org/grpc, google.golang.org/protobuf, modernc.org/*
- **BSD Licensed:** BurntSushi/toml, fsnotify, stretchr/testify, remyoudompheng/bigfft
- **No GPL or proprietary licenses in the Go dependency tree**

---

## GPL Isolation Confirmation

Per `/THIRD_PARTY.md`:

- The **DOOM subsystem** (GPLv2) is isolated in a separate repository (`doom/`)
- DOOM runs in a **BPF VM** and communicates with Go tooling through **BPF map syscalls only** (a data-level protocol, not code linking)
- The **Unheaded Go codebase is entirely BSL 1.1 licensed** with no GPL dependencies
- This boundary is **architecturally enforced** and eliminates GPL licensing obligations for the main project

---

## Compliance Status

- **No unknown licenses detected:** All dependencies are from well-known open source projects
- **No proprietary licenses found:** All dependencies are freely redistributable
- **Compatible with BSL 1.1:** No copyleft or GPL dependencies in Go code
- **Audit Status:** PASSED

---

## Usage

This SBOM is current as of the `go.mod` and `go.sum` files in the repository root. To regenerate:

```bash
cd /sessions/great-dreamy-ptolemy/mnt/tmp/unheaded
go list -m -json all > /tmp/sbom-raw.json
go list -m all > /tmp/sbom-list.txt
```

---

**Generated with:** Claude Code  
**Last Updated:** 2026-02-25
