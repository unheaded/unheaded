# Unheaded License: MIT

Unheaded is MIT licensed. Simple as it gets.

## What MIT Means

```
Copyright (c) 2024-2026 Steven Bellis

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
```

**You can**: use it commercially, modify it, distribute it, sublicense it, use it privately.
**You must**: include the copyright notice and this license in copies.
**You cannot**: hold the author liable.

That's the whole thing.

## Why MIT

This project started as a wild idea — what if every packet carried its own trace? What if
the network itself was the telemetry system? The ideas matter. Spread them. Build on them.
Break things and share what you learn.

No commercial moat. No "Permitted Users" definition. No change-date countdown. Just code.

## Protocol Specifications

Protocol specs in `docs/protocol/` are also MIT (see `LICENSE-PROTOCOLS`). The intent is
for anyone to implement the Monad/Sophia/Wotan wire formats in any language, any runtime,
any network. Protocol interoperability matters more than exclusivity.

## DOOM Component

The DOOM source code in `/doom/doomgeneric/` is GPL v2.0 (id Software). It is isolated from
the main codebase — the Go/Rust binary communicates with DOOM exclusively via BPF maps. No
linking. No compilation merge. The GPL boundary is the BPF map interface.

## SPDX Identifier

All Go source files carry: `SPDX-License-Identifier: MIT`

## Questions

stevie@bellis.tech
