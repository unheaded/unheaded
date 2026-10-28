# Unheaded License: GPL-3.0

Unheaded is licensed under the GNU General Public License v3.0.

## What GPL-3.0 Means

```
Copyright (c) 2024-2026 Stevie Bellis

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.
```

**You can**: use it, modify it, distribute it, study the source code.
**You must**: distribute source code with any distribution, license derivatives under GPL-3.0, include copyright notice.
**You cannot**: hold the author liable, sublicense under a different license, restrict recipients' rights.

## Why GPL

This project makes infrastructure observable from packet zero. That visibility should be
guaranteed for everyone who uses it. GPL ensures that modifications and improvements flow
back to the community. If you build on Unheaded, everyone benefits.

The protocol is the moat. The code is the commons.

## Protocol Specifications

Protocol specs in `docs/protocol/` are dual-licensed GPL-3.0 + Apache 2.0 (see `LICENSE-PROTOCOLS`).
The Apache 2.0 alternative exists so anyone can implement the Monad/Sophia/Wotan wire formats
in any language, any runtime, any network — even in proprietary software. Protocol
interoperability matters more than exclusivity.

## DOOM Component

The DOOM source code in `/doom/doomgeneric/` is GPL v2.0 (id Software). It is isolated from
the main codebase — the Go/Rust binary communicates with DOOM exclusively via BPF maps. No
linking. No compilation merge. The GPL boundary is the BPF map interface.

## Third-Party Dependencies

Third-party dependencies retain their original licenses (MIT, Apache 2.0, BSD, etc.).
See `THIRD_PARTY.md` for the full inventory. The GPL-3.0 license applies only to
Unheaded's own code.

## SPDX Identifier

Every first-party Go and Rust source file carries an SPDX header, checked in CI by
`scripts/check-spdx.sh --langs go,rs --forbid MIT` (backfill: `scripts/spdx-backfill.sh`).
The identifier is `GPL-3.0-or-later`, except `GPL-2.0-only` where a crate's manifest or its
existing headers already said so: `ebpf/common`, `flow-tracker`, `packet-marker`,
`latency-probe` (the `ebpf/` workspace licence), most `ebpf/*-ebpf` kernel programs,
`ebpf/af-xdp`, `cmd/ebpf-loader`, `cmd/ebpf-load-check`. A backfilled header records the
licence the crate already declares; it never changes one.

## What Remains MIT

First-party files that carried an MIT header were relicensed to GPL-3.0-or-later on
2026-10-27 (decision 12, `scripts/relicense-mit-to-gpl.sh`, commit `4c15bc76`).
`docs/legal/MIT-KEEP.txt`, the list of files that stay MIT on purpose, is empty. MIT
remains only where the code is someone else's or lives outside this tree:

| What | Why it stays MIT |
|------|------------------|
| `crates/xv6-mbc/upstream/` | Vendored xv6-riscv (MIT-PDOS); its `LICENSE` is kept verbatim |
| `llama.cpp/` | Not in the repository (gitignored); a separate process |
| MIT-licensed dependencies | Their authors' licence (`THIRD_PARTY.md`) |

Two crate manifests still declare `license = "MIT"` although every file in them carries a
GPL-3.0-or-later header: `cmd/trace-collector/Cargo.toml` and `crates/monad-mbc/Cargo.toml`.
Changing a manifest's licence is the author's call (S-TRUE-WIRE E4).

## Questions

stevie@bellis.tech
