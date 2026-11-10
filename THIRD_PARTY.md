# Third-Party Dependencies and GPL Boundary

**Project:** Unheaded Kingdom
**Last updated:** 2026-11-09 (tables matched to `sbom/`, licenses as read by `scripts/generate-sbom.sh`)
**Main license:** GPL-3.0-or-later (see `/LICENSE`)
**Canonical third-party inventory location:** This file

---

## Executive Summary

This document describes all third-party dependencies used by the Unheaded Kingdom project and defines the GPL boundary for the DOOM subsystem.

The codebase consists of:
- **Unheaded code:** Licensed under GPL-3.0 (see `/LICENSE`)
- **DOOM subsystem:** GPL v2.0, architecturally isolated and NOT linked into the main codebase
- **Third-party dependencies:** Go modules, Rust crates, Python packages, and JavaScript libraries. `sbom/SBOM.md` gives each license as read, names every package where what it reads names the GPL family or a proprietary license, and names every package it could not read. What it reads differs by ecosystem, and it says how
- **Protocol specifications:** Published under GPL-3.0/Apache 2.0 to encourage ecosystem adoption

---

## GPL Boundary (CRITICAL)

### The Problem DOOM Solves

The DOOM subsystem demonstrates **computational completeness** of the Monad Protocol (Section 12 of the specification). Running the DOOM engine inside an eBPF BPF map via MBC bytecode proves that the Monad ISA can execute arbitrary C programs.

### GPL Isolation Architecture

The DOOM engine (GPL v2.0) is compiled to MBC bytecode and executed inside the Linux kernel's eBPF VM sandbox. The rest of the Unheaded codebase (GPL-3.0-or-later) is completely separate. **There is no linking, compilation merging, or shared address space.**

```
+--------------------------------------------------+
|  Unheaded Platform (GPL-3.0 / GPL-3.0 / Apache 2.0) |
|  Go binaries, Rust binaries, JS frontend        |
|                                                  |
|  cmd/upc-bootctl                                 |
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

**Result:** The GPL v2 license applies exclusively to the files in the `doom/` directory. The main codebase (GPL-3.0-or-later) has no GPL v2 obligations from DOOM.

### MIT-Licensed Components (inside kingdom, GPL-3.0-compat)

| Path | Description | License | Origin | Pinned commit |
|------|-------------|---------|--------|---------------|
| `crates/xv6-mbc/upstream/` | xv6-riscv — MIT-PDOS pedagogical Unix-like teaching OS. Used as ASCEND-LINUX Phase 1 L5 MiniKernel rung (per `references/battle-plan-ascend-linux-2026-05-08.md`). Adapter code in `crates/xv6-mbc/adapters/` is GPL-3.0-or-later (kingdom side). | MIT | https://github.com/mit-pdos/xv6-riscv | `5474d4bf72fd95a6e5c735c2d7f208f58990ceab` (riscv branch HEAD, 2026-05-08) |

**License compatibility:** MIT is GPL-3.0-compat in both directions. xv6-riscv `LICENSE` file preserved verbatim at `crates/xv6-mbc/upstream/LICENSE`.

### GPL v2 Licensed Components (inside the boundary)

The Doom sources, tools and WAD handling moved to `unheaded-doom` on 2026-11-04 (DP-305). What stays is the compiled fixture.

| Path | Description | License | Notes |
|------|-------------|---------|-------|
| `doom/doom.mbc` | Compiled MBC bytecode artifact from doomgeneric | GPL v2 (compiled form) | Binary executable in eBPF map |
| `doom/doom.rv2mbc` | Intermediate RV32I-to-MBC translated artifact | GPL v2 (compiled form) | Intermediate translation stage |

### Outside the GPL-2.0 boundary

All code outside the `doom/` directory is **original Unheaded work** licensed under GPL-3.0-or-later (the MIT headers these files once carried were relicensed 2026-10-27, decision 12). These components do NOT derive from, link to, or include any of the GPL-2.0 code in `doom/`. They communicate with DOOM solely through BPF map syscalls (a data protocol boundary, analogous to user-space programs communicating with the GPL Linux kernel).

| Path | Description | License |
|------|-------------|---------|
| `pkg/ebpf/` | Go BPF wrappers for loading and interacting with BPF programs and maps | GPL-3.0-or-later |

---

## Go Dependencies

Direct modules, from `go.mod`. Every indirect module, with its license as read, is in `sbom/go-dependencies.json`. " AND " means the module's license files name each of these; they may be alternatives.

| Module | Version | License | Purpose |
|--------|---------|---------|---------|
| github.com/BurntSushi/toml | v1.6.0 | MIT | TOML configuration parsing |
| github.com/anthropics/anthropic-sdk-go | v1.82.0 | MIT | Claude API client (`pkg/claudebridge`) |
| github.com/cilium/ebpf | v0.22.0 | MIT | Loading eBPF programs and reading their maps |
| github.com/cloudflare/circl | v1.6.5 | BSD-3-Clause | Post-quantum cryptography |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | UUID generation |
| github.com/gorilla/websocket | v1.5.3 | BSD-2-Clause | WebSocket protocol support |
| github.com/lib/pq | v1.12.3 | MIT | The Well (PostgreSQL) client |
| github.com/rs/zerolog | v1.35.1 | MIT | Structured JSON logging |
| github.com/yuin/goldmark | v1.8.6 | MIT | Markdown parsing and rendering |
| golang.org/x/crypto | v0.58.1-0.20261009181718-2f1f834330b2 | BSD-3-Clause | Cryptographic utilities (mTLS, hashing) |
| golang.org/x/sys | v0.49.0 | BSD-3-Clause | System-level operations (uname, signals) |
| golang.org/x/text | v0.43.0 | BSD-3-Clause | Text processing and Unicode support |
| golang.org/x/time | v0.16.0 | BSD-3-Clause | Rate limiting utilities |
| google.golang.org/grpc | v1.84.0 | Apache-2.0 | gRPC framework for RPC communication |
| google.golang.org/protobuf | v1.36.12 | BSD-3-Clause | Protocol Buffers (protobuf serialization) |
| gopkg.in/yaml.v3 | v3.0.1 | Apache-2.0 AND MIT | YAML configuration file parsing |
| modernc.org/sqlite | v1.61.0 | Apache-2.0 AND BSD-3-Clause AND LicenseRef-public-domain AND MIT AND MPL-2.0 | Pure Go SQLite3 (no CGO), Kanban L1 persistence |

---

## Rust Dependencies

The crates the eBPF components, zhend and the MBC translator are built on, with the license field of each crate's own `Cargo.toml`. Every crate in every lock file is in `sbom/rust-dependencies.json`.

| Crate | Version | License | Purpose |
|-------|---------|---------|---------|
| tokio | 1.54.1 | MIT | Async runtime |
| tonic | 0.12.3 | MIT | gRPC framework for zhend |
| aya | 0.13.1 | MIT OR Apache-2.0 | eBPF framework for Rust (programs + userspace) |
| aya-ebpf | 0.1.1 | MIT OR Apache-2.0 | eBPF in-kernel code library |
| prost | 0.13.5 | Apache-2.0 | Protocol Buffers for Rust |
| serde | 1.0.229 | MIT OR Apache-2.0 | Serialization framework |
| serde_json | 1.0.154 | MIT OR Apache-2.0 | JSON for serde |
| clap | 4.6.7 | MIT OR Apache-2.0 | Command-line argument parsing |
| rustls | 0.23.46 | Apache-2.0 OR ISC OR MIT | Pure-Rust TLS for zhend |
| goblin | 0.8.2 | MIT | ELF parser for monad-mbc RV32I translator |
| nix | 0.31.3 | MIT | Unix API bindings for BPF operations |
| thiserror | 1.0.69 | MIT OR Apache-2.0 | Error handling derives |
| thiserror | 2.0.21 | MIT OR Apache-2.0 | Error handling derives |
| anyhow | 1.0.104 | MIT OR Apache-2.0 | Error handling |
| tracing | 0.1.44 | MIT | Structured diagnostics |
| tracing-subscriber | 0.3.23 | MIT | Structured diagnostics |

---

## JavaScript / Dashboard Dependencies

The Unheaded dashboard is vanilla JavaScript. Nothing in the repo uses npm (ADR-006);
JavaScript is linted by oxlint (MIT, oxc-project), a static binary pinned by sha256 in
`scripts/check-js-lint.sh`, which ships nothing.

| Library | License | Purpose |
|---------|---------|---------|
| Vanilla JavaScript (ES6+) | MIT | Canvas rendering, WebSocket handling |
| WebSocket API | Built-in browser API | Real-time updates |
| HTML Canvas 2D | Built-in browser API | Frame rendering |

**No npm packages are bundled or built.** Three libraries are nevertheless loaded
at runtime from CDNs by specific HTML pages, which makes them third-party
dependencies of those pages even though nothing is installed:

| Library | Version | License stated upstream, not read on west | Loaded by | Integrity |
|---------|---------|---------|-----------|-----------|
| xterm | 5.3.0 | MIT | `dashboard/upc-console.html` | SRI sha384 |
| xterm-addon-fit | 0.8.0 | MIT | `dashboard/upc-console.html` | SRI sha384 |
| xlsx | 0.20.3 | Apache-2.0 | `skills/marshal-eval-review.html` | SRI sha384 |

`xterm` is [xterm.js](https://github.com/xtermjs/xterm.js) and `xlsx` is
[SheetJS](https://sheetjs.com/). No copy of these files is on west, so
`sbom/SBOM.md` lists them as not read. Each carries a Subresource Integrity hash — see the note in
`dashboard/upc-console.html` for how to recompute one when bumping a version.

Google Fonts stylesheets are also loaded by `raft/static/index.html` and
`skills/marshal-eval-review.html`. They carry no SRI because Google serves
different CSS per user-agent, so the response is not stable for a given URL.

Configuration and protocol handlers are in `/dashboard/js/` and are GPL-3.0.

---

## Python Dependencies (Zhen / RAFT)

Declared in `raft/requirements.txt` — direct imports only; pip resolves the
transitive set. Before that file existed, `syft dir:.` found **zero** Python
packages, so the SBOM and every vulnerability scan downstream of it were blind to
this entire surface. See `docs/security/python-deps-unscanned-2026-08-04.md`.

| Package | Version | License | Purpose |
|---------|---------|---------|---------|
| Flask | 3.1.3 | BSD-3-Clause | Zhen web UI (`raft/zhen_app.py`, a Unix socket behind `cmd/zhen-ui-front` on port 20103) |
| requests | 2.34.2 | Apache Software License | HTTP client |
| Pygments | 2.21.0 | BSD-2-Clause | Syntax highlighting of code in Zhen answers, server-side (`raft/zhen_highlight.py`) |
| psycopg2-binary | 2.9.11 | GNU Library or Lesser General Public License (LGPL) | The Well (PostgreSQL) client |
| faiss-cpu | 1.13.2 | MIT AND BSD-3-Clause | Vector index |
| sentence-transformers | 5.7.0 | Apache-2.0 | Memory embedder (all-MiniLM-L6-v2) |
| langchain-text-splitters | 1.1.2 | MIT License | Corpus chunking |
| numpy | 2.4.3 | BSD-3-Clause AND 0BSD AND MIT AND Zlib AND CC0-1.0 | Numerics |
| torch | 2.10.0 | BSD-3-Clause | Training + inference |
| transformers | 5.17.0 | Apache 2.0 License | RAFT fine-tuning pipeline |
| datasets | 5.0.1 | Apache Software License | RAFT fine-tuning pipeline |
| peft | 0.18.1 | Apache Software License | RAFT fine-tuning pipeline |
| trl | 0.29.0 | Apache-2.0 | RAFT fine-tuning pipeline |
| anthropic | 0.84.0 | MIT License | Claude API client |
| PyYAML | 6.0.3 | MIT License | Runbook / config parsing |
| pytest | 9.0.3 | MIT | Tests |

Each license is the installed package's own metadata in `~/.venv/zhen`, as written, read by
`scripts/generate-sbom.sh`. The whole venv, including what these pull in, is in
`sbom/python-dependencies.json`, with every other requirements file the repo tracks;
some of it is in the GPL family and some is proprietary. `sbom/SBOM.md` names both.

**psycopg2:** its metadata says LGPL, and its LICENSE file says version 3 or later,
with exceptions. It is called from Python at runtime, never linked into a Go or Rust
binary.

---

## External Runtime Dependencies (not vendored, not tracked)

Cloned or installed locally and required at runtime, so they belong in this
inventory even though no file of theirs is in the repository:

| Dependency | License | Why it is here | Where |
|---|---|---|---|
| [llama.cpp](https://github.com/ggml-org/llama.cpp) | MIT | `llama-server` on :8081 is Zhen's inference backend — `raft/start-zhen.sh` refuses to start without it | `llama.cpp/`, gitignored (`.gitignore:236`) |
| [Vector](https://github.com/vectordotdev/vector) 0.43.1 | MPL-2.0 | East's journal shipper as a host package (`deploy/systemd/vector-east.service`); west runs the same version as the compose image `timberio/vector:0.43.1-debian` | `.deb` pinned by sha256 in `scripts/lib/tool-pins.sh` (`VECTOR_DEB_*`), installed on east only |

MIT is GPL-3.0-compatible. llama.cpp runs as a **separate process** reached over
HTTP; nothing links against it, so no GPL boundary question arises. Vector is
MPL-2.0 and likewise a separate process fed by configuration only.

---

## License Summary

Counts by license are in `sbom/SBOM.md`, and they move with the lock files, so they are not repeated here. That report reads licenses and does not rule on them: its verdict names every package where what it reads names the GPL family or a proprietary license, and every package it could not read, for a person to rule on. It reads license files at a Go module's root, the license field of a crate's `Cargo.toml`, and a Python package's metadata and installed license files. A license stated only inside a source file, or in a file a crate bundles, is not seen.

The GPL-2.0 fixture in `doom/` stays behind the boundary described above.

---

## How to Regenerate This Inventory

```bash
scripts/generate-sbom.sh    # sbom/*.json and sbom/SBOM.md
```

It reads licenses from what is on the host: the Go module cache, the cargo registry, `~/.venv/zhen` and any venv named in `PYTHON_EXTRA_VENVS`, colon-separated. The committed report also read the CI tools' venv, built from `.github/ci-python/requirements.txt`; without it, those packages list as not read. It fetches nothing, so a module with no source in the cache stays listed as not read. `go mod download` first would read those. With no `~/.venv/zhen` the run still finishes, keeps the last freeze, lists its packages as not read, and cannot PASS. Then bring the tables above into line: `scripts/test-generate-sbom.sh` fails until they match.

### JavaScript loaded from CDNs

Not installed, so no tool enumerates them. Grep for them instead, and check each
hit carries an `integrity=` attribute:

```bash
grep -rn '<script[^>]*src="https' --include=*.html . | grep -v node_modules
```

---

## References

- `/LICENSE` — GPL-3.0-or-later (main codebase)
- `/doom/LICENSE` — GPL v2 boundary documentation
- `/LICENSES/THIRD_PARTY.md` — Detailed third-party attributions with full license texts
- `/docs/legal/IP-INVENTORY.md` — Intellectual property asset inventory
- `/sbom/SBOM.md` — Software Bill of Materials
- https://www.gnu.org/licenses/old-licenses/gpl-2.0.txt — GPL v2 full text
- https://github.com/ozkl/doomgeneric — Upstream doomgeneric repository
- https://github.com/id-Software/DOOM — Original id Software DOOM source

---

**Last read:** 2026-11-05, by `scripts/generate-sbom.sh`.
