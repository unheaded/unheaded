# Third Party Licenses

The Unheaded Kingdom project uses the following third-party packages.
All attributions and licenses are listed below as required.

**Last audited:** 2026-02-24 (go.mod, Cargo.toml scan)
**Go module:** `unheaded` — 17 direct + 14 indirect dependencies
**Rust crates:** trace-collector (28 deps), monad-mbc (3 deps), shield/WAF (10 deps), ebpf-loader (5 deps)

---

## Apache License 2.0 Licensed Packages

### github.com/cilium/ebpf

**Copyright:** The Cilium Authors
**License:** Apache License 2.0
**URL:** https://github.com/cilium/ebpf

Used in the Whispering Void (eBPF) components for:
- Loading BPF object files
- Attaching programs to XDP, TC, kprobes, tracepoints
- Map operations and ring buffer reading

```
Copyright 2017-2024 The Cilium Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

---

### github.com/prometheus/client_golang

**Copyright:** The Prometheus Authors
**License:** Apache License 2.0
**URL:** https://github.com/prometheus/client_golang

Used throughout the Kingdom for metrics instrumentation.

```
Copyright 2014-2024 The Prometheus Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

---

### google.golang.org/grpc

**Copyright:** The gRPC Authors
**License:** Apache License 2.0
**URL:** https://github.com/grpc/grpc-go

Used for gRPC streaming in Wotan client and trace collection.

```
Copyright 2014-2024 The gRPC Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

---

### google.golang.org/protobuf

**Copyright:** The Go Authors
**License:** BSD 3-Clause
**URL:** https://github.com/protocolbuffers/protobuf-go

Used for protocol buffer serialization.

---

## MIT Licensed Packages

### github.com/rs/zerolog

**Copyright:** Olivier Poitrey
**License:** MIT
**URL:** https://github.com/rs/zerolog

Used for structured JSON logging throughout the Kingdom.

---

### github.com/gorilla/mux

**Copyright:** The Gorilla Authors
**License:** BSD 3-Clause
**URL:** https://github.com/gorilla/mux

Used for HTTP request routing in all services.

---

### github.com/gorilla/websocket

**Copyright:** The Gorilla Authors
**License:** BSD 2-Clause
**URL:** https://github.com/gorilla/websocket

Used in the Dashboard Cape for WebSocket connections.

---

### github.com/yuin/goldmark

**Copyright:** Yusuke Inuzuka
**License:** MIT
**URL:** https://github.com/yuin/goldmark

Used for Markdown parsing in triple-format (MD→JSON/YAML) rendering.

---

### github.com/fsnotify/fsnotify

**Copyright:** The fsnotify Authors
**License:** BSD 3-Clause
**URL:** https://github.com/fsnotify/fsnotify

Used for file watching in policy controller and NixOS builder.

---

## BSD Licensed Packages

### golang.org/x/sys

**Copyright:** The Go Authors
**License:** BSD 3-Clause
**URL:** https://go.googlesource.com/sys

Used for Unix system calls including uname for kernel version detection.

---

### gopkg.in/yaml.v3

**Copyright:** Canonical Ltd.
**License:** Apache License 2.0
**URL:** https://github.com/go-yaml/yaml

Used for YAML configuration parsing.

---

## Full Apache License 2.0 Text

```
                              Apache License
                        Version 2.0, January 2004
                     http://www.apache.org/licenses/

TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

1. Definitions.

   "License" shall mean the terms and conditions for use, reproduction,
   and distribution as defined by Sections 1 through 9 of this document.

   "Licensor" shall mean the copyright owner or entity authorized by
   the copyright owner that is granting the License.

   "Legal Entity" shall mean the union of the acting entity and all
   other entities that control, are controlled by, or are under common
   control with that entity. For the purposes of this definition,
   "control" means (i) the power, direct or indirect, to cause the
   direction or management of such entity, whether by contract or
   otherwise, or (ii) ownership of fifty percent (50%) or more of the
   outstanding shares, or (iii) beneficial ownership of such entity.

   "You" (or "Your") shall mean an individual or Legal Entity
   exercising permissions granted by this License.

   "Source" form shall mean the preferred form for making modifications,
   including but not limited to software source code, documentation
   source, and configuration files.

   "Object" form shall mean any form resulting from mechanical
   transformation or translation of a Source form, including but
   not limited to compiled object code, generated documentation,
   and conversions to other media types.

   "Work" shall mean the work of authorship, whether in Source or
   Object form, made available under the License, as indicated by a
   copyright notice that is included in or attached to the work
   (an example is provided in the Appendix below).

   "Derivative Works" shall mean any work, whether in Source or Object
   form, that is based on (or derived from) the Work and for which the
   editorial revisions, annotations, elaborations, or other modifications
   represent, as a whole, an original work of authorship. For the purposes
   of this License, Derivative Works shall not include works that remain
   separable from, or merely link (or bind by name) to the interfaces of,
   the Work and Derivative Works thereof.

   "Contribution" shall mean any work of authorship, including
   the original version of the Work and any modifications or additions
   to that Work or Derivative Works thereof, that is intentionally
   submitted to the Licensor for inclusion in the Work by the copyright owner
   or by an individual or Legal Entity authorized to submit on behalf of
   the copyright owner. For the purposes of this definition, "submitted"
   means any form of electronic, verbal, or written communication sent
   to the Licensor or its representatives, including but not limited to
   communication on electronic mailing lists, source code control systems,
   and issue tracking systems that are managed by, or on behalf of, the
   Licensor for the purpose of discussing and improving the Work, but
   excluding communication that is conspicuously marked or otherwise
   designated in writing by the copyright owner as "Not a Contribution."

   "Contributor" shall mean Licensor and any individual or Legal Entity
   on behalf of whom a Contribution has been received by Licensor and
   subsequently incorporated within the Work.

2. Grant of Copyright License. Subject to the terms and conditions of
   this License, each Contributor hereby grants to You a perpetual,
   worldwide, non-exclusive, no-charge, royalty-free, irrevocable
   copyright license to reproduce, prepare Derivative Works of,
   publicly display, publicly perform, sublicense, and distribute the
   Work and such Derivative Works in Source or Object form.

3. Grant of Patent License. Subject to the terms and conditions of
   this License, each Contributor hereby grants to You a perpetual,
   worldwide, non-exclusive, no-charge, royalty-free, irrevocable
   (except as stated in this section) patent license to make, have made,
   use, offer to sell, sell, import, and otherwise transfer the Work,
   where such license applies only to those patent claims licensable
   by such Contributor that are necessarily infringed by their
   Contribution(s) alone or by combination of their Contribution(s)
   with the Work to which such Contribution(s) was submitted. If You
   institute patent litigation against any entity (including a
   cross-claim or counterclaim in a lawsuit) alleging that the Work
   or a Contribution incorporated within the Work constitutes direct
   or contributory patent infringement, then any patent licenses
   granted to You under this License for that Work shall terminate
   as of the date such litigation is filed.

4. Redistribution. You may reproduce and distribute copies of the
   Work or Derivative Works thereof in any medium, with or without
   modifications, and in Source or Object form, provided that You
   meet the following conditions:

   (a) You must give any other recipients of the Work or
       Derivative Works a copy of this License; and

   (b) You must cause any modified files to carry prominent notices
       stating that You changed the files; and

   (c) You must retain, in the Source form of any Derivative Works
       that You distribute, all copyright, patent, trademark, and
       attribution notices from the Source form of the Work,
       excluding those notices that do not pertain to any part of
       the Derivative Works; and

   (d) If the Work includes a "NOTICE" text file as part of its
       distribution, then any Derivative Works that You distribute must
       include a readable copy of the attribution notices contained
       within such NOTICE file, excluding those notices that do not
       pertain to any part of the Derivative Works, in at least one
       of the following places: within a NOTICE text file distributed
       as part of the Derivative Works; within the Source form or
       documentation, if provided along with the Derivative Works; or,
       within a display generated by the Derivative Works, if and
       wherever such third-party notices normally appear. The contents
       of the NOTICE file are for informational purposes only and
       do not modify the License. You may add Your own attribution
       notices within Derivative Works that You distribute, alongside
       or as an addendum to the NOTICE text from the Work, provided
       that such additional attribution notices cannot be construed
       as modifying the License.

   You may add Your own copyright statement to Your modifications and
   may provide additional or different license terms and conditions
   for use, reproduction, or distribution of Your modifications, or
   for any such Derivative Works as a whole, provided Your use,
   reproduction, and distribution of the Work otherwise complies with
   the conditions stated in this License.

5. Submission of Contributions. Unless You explicitly state otherwise,
   any Contribution intentionally submitted for inclusion in the Work
   by You to the Licensor shall be under the terms and conditions of
   this License, without any additional terms or conditions.
   Notwithstanding the above, nothing herein shall supersede or modify
   the terms of any separate license agreement you may have executed
   with Licensor regarding such Contributions.

6. Trademarks. This License does not grant permission to use the trade
   names, trademarks, service marks, or product names of the Licensor,
   except as required for reasonable and customary use in describing the
   origin of the Work and reproducing the content of the NOTICE file.

7. Disclaimer of Warranty. Unless required by applicable law or
   agreed to in writing, Licensor provides the Work (and each
   Contributor provides its Contributions) on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
   implied, including, without limitation, any warranties or conditions
   of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
   PARTICULAR PURPOSE. You are solely responsible for determining the
   appropriateness of using or redistributing the Work and assume any
   risks associated with Your exercise of permissions under this License.

8. Limitation of Liability. In no event and under no legal theory,
   whether in tort (including negligence), contract, or otherwise,
   unless required by applicable law (such as deliberate and grossly
   negligent acts) or agreed to in writing, shall any Contributor be
   liable to You for damages, including any direct, indirect, special,
   incidental, or consequential damages of any character arising as a
   result of this License or out of the use or inability to use the
   Work (including but not limited to damages for loss of goodwill,
   work stoppage, computer failure or malfunction, or any and all
   other commercial damages or losses), even if such Contributor
   has been advised of the possibility of such damages.

9. Accepting Warranty or Additional Liability. While redistributing
   the Work or Derivative Works thereof, You may choose to offer,
   and charge a fee for, acceptance of support, warranty, indemnity,
   or other liability obligations and/or rights consistent with this
   License. However, in accepting such obligations, You may act only
   on Your own behalf and on Your sole responsibility, not on behalf
   of any other Contributor, and only if You agree to indemnify,
   defend, and hold each Contributor harmless for any liability
   incurred by, or claims asserted against, such Contributor by reason
   of your accepting any such warranty or additional liability.

END OF TERMS AND CONDITIONS
```

---

### modernc.org/sqlite

**Copyright:** Jan Mercl
**License:** BSD 3-Clause
**URL:** https://gitlab.com/cznic/sqlite

Used for SQLite L1 persistence in the Kanban app (pure Go, no CGO).

---

### github.com/sony/gobreaker

**Copyright:** Sony Corporation
**License:** MIT
**URL:** https://github.com/sony/gobreaker

Referenced for circuit breaker patterns in TopicStreamClient.

---

### github.com/google/uuid

**Copyright:** Google Inc.
**License:** BSD 3-Clause
**URL:** https://github.com/google/uuid

Used for UUID generation throughout the Kingdom.

---

### github.com/BurntSushi/toml

**Copyright:** Andrew Gallant
**License:** MIT
**URL:** https://github.com/BurntSushi/toml

Used for TOML configuration parsing.

---

### golang.org/x/crypto

**Copyright:** The Go Authors
**License:** BSD 3-Clause
**URL:** https://go.googlesource.com/crypto

Used for cryptographic operations including mTLS and API key hashing.

---

### golang.org/x/text

**Copyright:** The Go Authors
**License:** BSD 3-Clause
**URL:** https://go.googlesource.com/text

Used for text processing and Unicode support.

---

### golang.org/x/time

**Copyright:** The Go Authors
**License:** BSD 3-Clause
**URL:** https://go.googlesource.com/time

Used for rate limiting.

---

### golang.org/x/net

**Copyright:** The Go Authors
**License:** BSD 3-Clause
**URL:** https://go.googlesource.com/net

Indirect dependency for HTTP/2, gRPC networking.

---

### golang.org/x/exp

**Copyright:** The Go Authors
**License:** BSD 3-Clause
**URL:** https://go.googlesource.com/exp

Indirect dependency for experimental Go packages.

---

## Go Indirect Dependencies

The following packages are pulled in transitively. Licenses verified.

| Package | License | Via |
|---------|---------|-----|
| github.com/beorn7/perks | MIT | prometheus/client_golang |
| github.com/cespare/xxhash/v2 | MIT | prometheus/client_golang |
| github.com/dustin/go-humanize | MIT | modernc.org/sqlite |
| github.com/kr/text | MIT | test infrastructure |
| github.com/mattn/go-colorable | MIT | rs/zerolog |
| github.com/mattn/go-isatty | MIT | rs/zerolog |
| github.com/matttproud/golang_protobuf_extensions/v2 | Apache-2.0 | prometheus/client_golang |
| github.com/ncruces/go-strftime | MIT | modernc.org/sqlite |
| github.com/prometheus/client_model | Apache-2.0 | prometheus/client_golang |
| github.com/prometheus/common | Apache-2.0 | prometheus/client_golang |
| github.com/prometheus/procfs | Apache-2.0 | prometheus/client_golang |
| github.com/remyoudompheng/bigfft | BSD-3-Clause | modernc.org/sqlite |
| google.golang.org/genproto | Apache-2.0 | google.golang.org/grpc |
| modernc.org/libc | BSD-3-Clause | modernc.org/sqlite |
| modernc.org/mathutil | BSD-3-Clause | modernc.org/sqlite |
| modernc.org/memory | BSD-3-Clause | modernc.org/sqlite |

---

## Rust Dependencies (trace-collector, monad-mbc, shield/WAF, ebpf-loader)

### tokio

**Copyright:** Tokio Contributors
**License:** MIT
**URL:** https://github.com/tokio-rs/tokio

Async runtime for the trace-collector.

---

### tonic

**Copyright:** Lucio Franco
**License:** MIT
**URL:** https://github.com/hyperium/tonic

gRPC framework for trace-collector → Wotan communication.

---

### prost

**Copyright:** Dan Burkert
**License:** Apache License 2.0
**URL:** https://github.com/tokio-rs/prost

Protocol Buffers implementation for Rust.

---

### aya / aya-ebpf

**Copyright:** The Aya Contributors
**License:** MIT / Apache License 2.0
**URL:** https://github.com/aya-rs/aya

eBPF framework for Rust. Used in all 4 eBPF programs (packet-marker, flow-tracker, latency-probe, syscall-tracer).

---

### clap

**Copyright:** Kevin K. and clap Contributors
**License:** MIT / Apache License 2.0
**URL:** https://github.com/clap-rs/clap

Command-line argument parser for trace-collector.

---

### serde / serde_json

**Copyright:** David Tolnay
**License:** MIT / Apache License 2.0
**URL:** https://github.com/serde-rs/serde

Serialization framework used in trace-collector for JSON event encoding.

---

### crossbeam

**Copyright:** The Crossbeam Project Developers
**License:** MIT / Apache License 2.0
**URL:** https://github.com/crossbeam-rs/crossbeam

Lock-free data structures used in trace-collector for event batching.

---

### hyper

**Copyright:** Sean McArthur
**License:** MIT
**URL:** https://github.com/hyperium/hyper

HTTP library used for trace-collector health endpoint.

---

### nix

**Copyright:** The nix-rust Contributors
**License:** MIT
**URL:** https://github.com/nix-rust/nix

Unix API bindings used for BPF map operations and ring buffer reading.

---

### memmap2

**Copyright:** Dan Burkert, Yevhenii Reizner
**License:** MIT / Apache License 2.0
**URL:** https://github.com/RazrFalcon/memmap2-rs

Memory-mapped file I/O for zero-copy ring buffer reads from eBPF maps.

---

### prometheus (Rust)

**Copyright:** The Prometheus Authors / tikv contributors
**License:** Apache License 2.0
**URL:** https://github.com/tikv/rust-prometheus

Prometheus metrics exposition for trace-collector.

---

### goblin

**Copyright:** m4b
**License:** MIT
**URL:** https://github.com/m4b/goblin

ELF parser used in monad-mbc RV32I-to-MBC translator.

---

### thiserror

**Copyright:** David Tolnay
**License:** MIT / Apache License 2.0
**URL:** https://github.com/dtolnay/thiserror

Derive macro for error types. Used in monad-mbc and trace-collector.

---

### anyhow

**Copyright:** David Tolnay
**License:** MIT / Apache License 2.0
**URL:** https://github.com/dtolnay/anyhow

Error handling. Used in trace-collector and ebpf-loader.

---

### rustls / tokio-rustls

**Copyright:** The rustls Contributors
**License:** MIT / Apache License 2.0 / ISC
**URL:** https://github.com/rustls/rustls

Pure-Rust TLS implementation. Used in shield/WAF for TLS termination.

---

### tracing / tracing-subscriber

**Copyright:** Tokio Contributors
**License:** MIT
**URL:** https://github.com/tokio-rs/tracing

Structured diagnostics for async Rust. Used in trace-collector.

---

### env_logger / log

**Copyright:** The Rust Project Developers
**License:** MIT / Apache License 2.0
**URL:** https://github.com/rust-lang/log

Logging facade and env-filter logger. Used in ebpf-loader.

---

## Additional Rust Indirect Dependencies

| Crate | License | Via |
|-------|---------|-----|
| bytes | MIT | trace-collector (hyper, tonic) |
| parking_lot | MIT / Apache-2.0 | trace-collector |
| once_cell | MIT / Apache-2.0 | trace-collector |
| pin-project | MIT / Apache-2.0 | trace-collector |
| hostname | MIT | trace-collector |
| flate2 | MIT / Apache-2.0 | trace-collector (gzip) |
| regex | MIT / Apache-2.0 | trace-collector, shield |
| fastrand | MIT / Apache-2.0 | trace-collector |
| tokio-tungstenite | MIT | trace-collector (WebSocket) |
| futures-util | MIT / Apache-2.0 | trace-collector |
| http-body-util | MIT | trace-collector, shield |
| hyper-util | MIT | trace-collector, shield |
| rustls-pemfile | MIT / Apache-2.0 | shield (TLS cert loading) |
| ctrlc | MIT / Apache-2.0 | ebpf-loader |
| libc | MIT / Apache-2.0 | trace-collector (Linux FFI) |

---

## GPL Licensed Packages

### doomgeneric

**Copyright:** Maraakate, ozkl, id Software (original Doom source)
**License:** GNU General Public License v2.0
**URL:** https://github.com/ozkl/doomgeneric
**Fork:** https://github.com/unheaded-kingdom/doomgeneric (Unheaded modifications)

Portable Doom source port used for the Doom-over-IPv6 computational completeness
proof (Monad spec Section 12). The doomgeneric code lives in `doom/doomgeneric/`
as a git submodule. It is compiled to MBC bytecode via the rv32i-to-mbc translator
and executed inside eBPF BPF maps — it does NOT link against or modify the
Unheaded codebase. The GPL-2.0 license applies only to the doomgeneric directory.

Unheaded modifications to doomgeneric will be published to the
`unheaded-kingdom/doomgeneric` fork once protocol drafts are submitted and
all Unheaded repositories go public.

**Note:** WAD files (doom.wad, doom2.wad) are copyrighted game data owned
separately and excluded from the repository via .gitignore.

---

## Attribution Notice

This software includes code from the following projects:

- **Cilium eBPF** - A pure Go library for working with eBPF (Go userspace)
- **Aya / aya-ebpf** - eBPF framework for Rust (kernel programs + userspace loader)
- **doomgeneric** - Portable Doom source port (GPL-2.0, computational completeness proof)
- **Prometheus Go client** - Go client library for Prometheus
- **gRPC-Go** - The Go implementation of gRPC
- **zerolog** - Zero allocation JSON logger
- **gorilla/mux** - HTTP request router for Go
- **goldmark** - Markdown parser for Go (triple-format rendering)
- **Tokio** - Async runtime for Rust (trace-collector)
- **Tonic** - gRPC for Rust (trace-collector → Wotan)
- **rustls** - Pure-Rust TLS (shield/WAF)
- **goblin** - ELF parser for Rust (MBC translator)
- **modernc.org/sqlite** - Pure Go SQLite (Kanban persistence)

We thank the maintainers and contributors of these projects for their work.

**Total dependency count:** 17 direct Go + 14 indirect Go + ~50 Rust crates across 4 binaries.

---

## Session 14 Code Generation Attribution

Portions of this codebase were generated with assistance from:
- **Claude Opus 4.6** (Anthropic) — Campaign 1 (TopicStream gRPC Sprint), Campaign 2.2 (Dashboard Backend eBPF Wiring)
- **Gemini** (Google) — Campaign 3 (Security Hardening Pass)
