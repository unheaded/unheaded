# Third-Party Dependencies and GPL Boundary

**Project:** Unheaded
**Last updated:** 2026-02-25
**Main license:** BSL 1.1 (see `/LICENSE`)

This document describes all third-party dependencies used by the Unheaded
project and defines the GPL boundary for the DOOM subsystem.

---

## GPL Boundary

The DOOM subsystem is the only GPL-licensed component in the repository. All
GPL code is confined to the `doom/` directory. The rest of the codebase is
licensed under BSL 1.1. The boundary is enforced architecturally: DOOM runs
inside a BPF VM and communicates with Go tooling exclusively through BPF map
syscalls (a data protocol, not code linking).

### GPL v2 Licensed (inside the boundary)

| Path | Description | License |
|------|-------------|---------|
| `doom/doomgeneric/` | doomgeneric portable DOOM source port by ozkl. Original DOOM engine code copyright (C) 1993-1996 id Software. | GPL v2 |
| `doom/doomgeneric/doomgeneric/doomgeneric_monad.c` | Monad BPF VM platform layer (our modification). Implements `DG_Init`, `DG_DrawFrame`, `DG_SleepMs`, `DG_GetTicksMs`, `DG_GetKey` for the MBC target. | GPL v2 (derivative) |
| `doom/doomgeneric/doomgeneric/crt0_monad.S` | Monad C runtime startup (our modification). Sets up stack and calls `main` for the bare-metal MBC environment. | GPL v2 (derivative) |
| `doom/doomgeneric/doomgeneric/libc_monad.c` | Minimal libc for the Monad target (our modification). Provides `memcpy`, `memset`, `strlen`, `printf`, etc. via MBC syscalls. | GPL v2 (derivative) |
| `doom/doomgeneric/doomgeneric/monad.ld` | Linker script for the Monad MBC target (our modification). Defines memory layout with SCREEN_BASE at 0xC000. | GPL v2 (derivative) |
| `doom/doomgeneric/doomgeneric/w_file_monad.c` | WAD file I/O for the Monad target (our modification). | GPL v2 (derivative) |
| `doom/doomgeneric/doomgeneric/Makefile.monad` | Build configuration for the Monad target (our modification). | GPL v2 (derivative) |
| `doom/doomgeneric/doomgeneric/monad_include/` | Freestanding libc headers for the Monad target (our modification). | GPL v2 (derivative) |
| `doom/doom.mbc` | Compiled MBC bytecode artifact from doomgeneric source. | GPL v2 (compiled form) |
| `doom/doom.rv2mbc` | Intermediate RV32I-to-MBC translated artifact from doomgeneric source. | GPL v2 (compiled form) |

### Shareware (inside the boundary, separately licensed)

| Path | Description | License |
|------|-------------|---------|
| `doom/doom1.wad` | DOOM Shareware WAD by id Software. Game data (levels, sprites, textures) for the shareware episode. | Freely redistributable shareware. Not GPL. Not open source. Redistribution permitted by id Software's original shareware terms. |

### NOT GPL Licensed (outside the boundary)

The following components are original Unheaded code licensed under BSL 1.1.
They do NOT derive from, link to, or include any GPL-licensed code. They
communicate with the DOOM engine solely through BPF map syscalls, which
constitute a data-level protocol boundary (analogous to user-space programs
communicating with the GPL Linux kernel via syscalls).

| Path | Description | License |
|------|-------------|---------|
| `cmd/doom-bridge/` | Go tool that bridges DOOM BPF map state to WebSocket for live viewport streaming. | BSL 1.1 |
| `cmd/doom-loader/` | Go tool that loads compiled MBC bytecode into BPF maps for execution. | BSL 1.1 |
| `cmd/doom-cpu-dump/` | Go tool that dumps MBC CPU register state from BPF maps for debugging. | BSL 1.1 |
| `cmd/doom-go-injector/` | Go tool that injects keyboard input and tick events into DOOM via BPF maps. | BSL 1.1 |
| `cmd/doom/` | Go DOOM orchestrator/launcher. | BSL 1.1 |
| `internal/bpf/` | Go BPF wrappers for loading and interacting with BPF programs and maps. | BSL 1.1 |
| `internal/bpfmap/` | Go BPF map abstraction layer for reading/writing DOOM VM state. | BSL 1.1 |
| `dashboard/doom.html` | HTML page for the DOOM live viewport in the dashboard. | BSL 1.1 |
| `dashboard/js/doom-viewport.js` | JavaScript WebSocket client that renders DOOM frames in a canvas element. | BSL 1.1 |
| `scripts/doom-*.{sh,py}` | Shell and Python helper scripts for DOOM development and testing. | BSL 1.1 |
| `configs/doom.toml` | DOOM subsystem configuration. | BSL 1.1 |

---

## Isolation Architecture

The GPL boundary is enforced by the runtime architecture, not merely by
directory layout. The DOOM engine and the Unheaded platform never share an
address space or link against each other.

```
+--------------------------------------------------+
|  Unheaded Platform (BSL 1.1)                     |
|                                                  |
|  cmd/doom-bridge   cmd/doom-loader               |
|  cmd/doom-go-injector   internal/bpf*            |
|       |                    |                     |
|       +--- BPF map syscalls (bpf(2)) -----------+|
|            read/write data only                  |
+--------------------------------------------------+
                      |
          +-----------+-----------+
          |  Linux Kernel (GPL)   |
          |  BPF VM sandbox       |
          |                       |
          |  doom.mbc bytecode    |
          |  (GPL v2 code running |
          |   inside BPF maps)    |
          +-----------------------+
```

Key isolation properties:

1. **No runtime linking.** DOOM runs as MBC bytecode inside the kernel BPF VM.
   Go tools are separate user-space processes. There is no shared library,
   static linking, or dynamic linking between them.

2. **Data protocol boundary.** Communication happens exclusively through
   `bpf(2)` syscalls that read and write BPF map entries. The Go tools treat
   map contents as opaque data (framebuffer pixels, keyboard scancodes, tick
   counters). This is a data exchange protocol, not code integration.

3. **Kernel sandbox.** The BPF VM is a kernel-enforced sandbox. The DOOM
   bytecode cannot call into user-space code, and user-space code cannot
   execute DOOM functions. The boundary is equivalent to the syscall interface
   between user-space programs and the GPL Linux kernel.

4. **Analogous to syscall boundary.** The Linux kernel is GPL v2. Every
   user-space program on Linux communicates with it via syscalls without
   becoming a derivative work. The DOOM-to-Go boundary via BPF maps follows
   the same principle: data exchange across a kernel-enforced interface does
   not create a derivative work.

5. **Independent compilation.** DOOM is compiled from C to RV32I to MBC by a
   Rust translator (`monad-mbc`). The Go tools are compiled by the Go
   compiler. The two toolchains never intersect. No GPL header files, object
   files, or libraries are used by the Go or JavaScript code.

---

## Go Dependencies

### Direct Dependencies

| Package | Version | License | URL |
|---------|---------|---------|-----|
| github.com/BurntSushi/toml | v1.6.0 | MIT | https://github.com/BurntSushi/toml |
| github.com/fsnotify/fsnotify | v1.7.0 | BSD-3-Clause | https://github.com/fsnotify/fsnotify |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | https://github.com/google/uuid |
| github.com/gorilla/mux | v1.8.1 | BSD-3-Clause | https://github.com/gorilla/mux |
| github.com/gorilla/websocket | v1.5.3 | BSD-2-Clause | https://github.com/gorilla/websocket |
| github.com/prometheus/client_golang | v1.18.0 | Apache-2.0 | https://github.com/prometheus/client_golang |
| github.com/rs/zerolog | v1.31.0 | MIT | https://github.com/rs/zerolog |
| github.com/sony/gobreaker | v0.5.0 | MIT | https://github.com/sony/gobreaker |
| github.com/yuin/goldmark | v1.7.16 | MIT | https://github.com/yuin/goldmark |
| golang.org/x/crypto | v0.23.0 | BSD-3-Clause | https://go.googlesource.com/crypto |
| golang.org/x/sys | v0.40.0 | BSD-3-Clause | https://go.googlesource.com/sys |
| golang.org/x/text | v0.15.0 | BSD-3-Clause | https://go.googlesource.com/text |
| golang.org/x/time | v0.5.0 | BSD-3-Clause | https://go.googlesource.com/time |
| google.golang.org/grpc | v1.65.0 | Apache-2.0 | https://github.com/grpc/grpc-go |
| google.golang.org/protobuf | v1.34.1 | BSD-3-Clause | https://github.com/protocolbuffers/protobuf-go |
| gopkg.in/yaml.v3 | v3.0.1 | Apache-2.0 | https://github.com/go-yaml/yaml |
| modernc.org/sqlite | v1.44.3 | BSD-3-Clause | https://gitlab.com/cznic/sqlite |

### Indirect Dependencies

| Package | Version | License | Pulled in by |
|---------|---------|---------|--------------|
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
| golang.org/x/exp | v0.0.0-20251023... | BSD-3-Clause | modernc.org/sqlite |
| golang.org/x/net | v0.25.0 | BSD-3-Clause | google.golang.org/grpc |
| google.golang.org/genproto/googleapis/rpc | v0.0.0-20240528... | Apache-2.0 | google.golang.org/grpc |
| modernc.org/libc | v1.67.6 | BSD-3-Clause | modernc.org/sqlite |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause | modernc.org/sqlite |
| modernc.org/memory | v1.11.0 | BSD-3-Clause | modernc.org/sqlite |

---

## License Summary Table

| Component | License | Source / Origin |
|-----------|---------|----------------|
| **Unheaded core** (all code outside `doom/`) | BSL 1.1 | Original work |
| **Protocol specifications** (`docs/protocol/`) | MIT / Apache-2.0 | Original work |
| `doom/doomgeneric/` (DOOM engine + Monad port) | GPL v2 | ozkl/doomgeneric + id Software (1993) |
| `doom/doom1.wad` (shareware WAD) | Shareware (freely redistributable) | id Software (1993) |
| `doom/doom.mbc`, `doom/doom.rv2mbc` (compiled artifacts) | GPL v2 | Compiled from doomgeneric source |
| `cmd/doom-bridge/` | BSL 1.1 | Original work |
| `cmd/doom-loader/` | BSL 1.1 | Original work |
| `cmd/doom-cpu-dump/` | BSL 1.1 | Original work |
| `cmd/doom-go-injector/` | BSL 1.1 | Original work |
| `cmd/doom/` | BSL 1.1 | Original work |
| `internal/bpf/`, `internal/bpfmap/` | BSL 1.1 | Original work |
| `dashboard/doom.html`, `dashboard/js/doom-viewport.js` | BSL 1.1 | Original work |
| github.com/BurntSushi/toml | MIT | Third-party |
| github.com/fsnotify/fsnotify | BSD-3-Clause | Third-party |
| github.com/google/uuid | BSD-3-Clause | Third-party |
| github.com/gorilla/mux | BSD-3-Clause | Third-party |
| github.com/gorilla/websocket | BSD-2-Clause | Third-party |
| github.com/prometheus/client_golang | Apache-2.0 | Third-party |
| github.com/rs/zerolog | MIT | Third-party |
| github.com/sony/gobreaker | MIT | Third-party |
| github.com/yuin/goldmark | MIT | Third-party |
| golang.org/x/crypto | BSD-3-Clause | Third-party (Go Authors) |
| golang.org/x/sys | BSD-3-Clause | Third-party (Go Authors) |
| golang.org/x/text | BSD-3-Clause | Third-party (Go Authors) |
| golang.org/x/time | BSD-3-Clause | Third-party (Go Authors) |
| google.golang.org/grpc | Apache-2.0 | Third-party (gRPC Authors) |
| google.golang.org/protobuf | BSD-3-Clause | Third-party (Go Authors) |
| gopkg.in/yaml.v3 | Apache-2.0 | Third-party (Canonical Ltd.) |
| modernc.org/sqlite | BSD-3-Clause | Third-party (Jan Mercl) |

---

## License Compatibility Notes

All Go dependencies use permissive licenses (MIT, BSD-2-Clause, BSD-3-Clause,
Apache-2.0) that are compatible with the BSL 1.1 main license.

The GPL v2 code in `doom/` is isolated by the BPF VM boundary described above
and does not create license obligations for the rest of the codebase. The GPL
v2 applies exclusively to the files listed in the "GPL v2 Licensed" section.

For the detailed third-party license attributions including full license texts
and Rust crate dependencies, see `LICENSES/THIRD_PARTY.md`.

---

## References

- `/LICENSE` -- BSL 1.1 (main codebase)
- `/doom/LICENSE` -- GPL v2 boundary documentation
- `/doom/doomgeneric/LICENSE` -- GPL v2 full text
- `/LICENSES/THIRD_PARTY.md` -- Detailed third-party attributions with full license texts
- https://www.gnu.org/licenses/old-licenses/gpl-2.0.txt -- GPL v2 full text
- https://github.com/ozkl/doomgeneric -- Upstream doomgeneric
- https://github.com/id-Software/DOOM -- Original id Software DOOM source
