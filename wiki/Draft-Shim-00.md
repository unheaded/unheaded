# draft-bellis-unheaded-shim-00

**Status:** IETF Experimental · Independent Submission · March 2026

This document specifies the Shim pipeline for the Unheaded Protocol Computer (UPC). The Shim translates MBC (Monad Bytecode) programs into eBPF execution contexts, defines the per-hop processing model, and specifies the tick packet protocol that drives distributed computation across IPv6 network hops. The pipeline implements a four-stage architecture: Assembly, Verification, Loading, and Execution, with integrated support for memory-mapped I/O, framebuffer rendering, and CRC validation.

## Key Sections

- **Introduction** — Shim as the execution engine bridging MBC ISA to eBPF/XDP; four-stage pipeline overview; relationship to Foundation (Monad wire format), MBC ISA, Sophia (dictionary lookups), and Wotan (RAM_MAP/ROM_MAP backing)
- **Pipeline Overview** — Full ASCII-art pipeline diagram: MBC Source → Assembly → Binary Image → Verification → Verified Image → Loading → BPF Runtime → Execution → XDP_TX output; each stage is atomic and idempotent
- **Stage 1: Assembly** — Plain-text input format (one instruction per line, '#' comments, label declarations); 32-bit little-endian binary encoding; two-pass label resolution (record offsets, then resolve relative word offsets)
- **Stage 2: Verification** — Opcode whitelist enforcement (48 valid opcodes, reject unknown); register range validation (0-15); immediate value range validation; branch target validation (valid boundaries, in-bounds, no unbounded backward branches); programs failing verification MUST NEVER be loaded
- **Stage 3: Loading** — BPF map creation and initialization: ROM_MAP (BPF_MAP_TYPE_ARRAY, 1 MiB, read-only), RAM_MAP (BPF_MAP_TYPE_ARRAY, 64 MiB, read-write), CPU_MAP (BPF_MAP_TYPE_HASH, 256 entries, 128-byte MbcCpuState), SCREEN_MAP (64 KiB, 320x200 8bpp), KBD_MAP (32 bytes, 256-key bitmask), plus TTY_MAP, PROC_TABLE, SCHED_STATE, TLB_MAP, COMPUTE_EVENTS; 6-step loading sequence ending with XDP attach
- **Stage 4: Execution** — Fetch-decode-execute cycle (fetch ROM[pc], decode, execute, increment pc, check 256-instruction limit); state persists across tick packets via CPU_MAP, RAM_MAP, SCREEN_MAP keyed by flow tuple
- **Tick Packet Protocol** — IPv6 + Hop-by-Hop Options (24 bytes with 20-byte Monad at Option Type 0x3E) + variable payload; 8-step processing pipeline (extract Monad, load CPU state, validate CRC-16, execute MBC, recompute CRC, write updated Monad, XDP_TX); tick rates: LOCAL 35 Hz, DISTRIBUTED ~1 kHz
- **Memory-Mapped I/O** — Physical address map: ROM (0x0000-0x3FFFF), Keyboard at 0x68000 (KBD_MAP, 256 keys as bitmask), Framebuffer at 0x70000 (SCREEN_MAP, 320x200 palette), RAM (0x80000+, RAM_MAP), TTY/Console at 0x7F000; code examples for keyboard read and pixel write
- **Framebuffer Specification** — 320x200 pixels, 8-bit palette (256 colors), scanline-major; pixel address = 0x70000 + (y * 320) + x; access via STB (single byte) or SYS_DRAW_FRAME syscall (bulk 64 KiB copy)
- **Dream Ladder Feature Stratification** — Conformance levels: Level 0 Microcode (REQUIRED: Monad + Sophia + XDP), Level 1 Digital (REQUIRED: full 48-opcode MBC), Level 2 Mechanical (REQUIRED: RAM_MAP/ROM_MAP/CPU_MAP), Level 2a Memory I/O (RECOMMENDED: framebuffer/keyboard/TTY), Level 3 Interrupts (OPTIONAL), Level 4 Scheduling/Multitasking (OPTIONAL), Level 5+ Virtual Memory/Syscalls/Filesystem (future); implementations must support contiguous range from Level 0
- **CRC Validation Ordering** — Pre-execution: extract Monad, validate CRC-16/CCITT, emit Anomaly event and skip MBC on failure; post-execution: recompute CRC over updated state, write to outgoing packet, XDP_TX; Anomaly event format (timestamp, event_type, flow_tuple, monad_state, expected/computed CRC)
- **Security Considerations** — eBPF verifier as primary security boundary; 256-instruction limit prevents CPU exhaustion; BPF map bounds checking (ROM zero-fill, RAM/SCREEN silent drop on OOB); XDP_TX only (no XDP_DROP or XDP_PASS to prevent packet leakage); tick packet injection trust model (no origin authentication, use ACLs/IPSec externally); register state confidentiality (plaintext in HbH, use TLS/IPSec if needed); verification as security gate
- **IANA Considerations** — Shim Pipeline Stage Registry (4 initial entries); BPF Map Type Registry for UPC (10 map types); Dream Ladder Level Registry (levels 0-4 defined, 5-7 reserved)

## Related

- [[Protocol Foundation|Protocol-Foundation]]
- [[Draft Protocol Foundation 06|Draft-Protocol-Foundation-06]]
- [[MBC ISA Reference|MBC-ISA-Reference]]
- [[Draft MBC ISA 00|Draft-MBC-ISA-00]]
- [[UPC Dream Ladder|UPC-Dream-Ladder]]
- [[Wotan Memory Model|Wotan-Memory-Model]]
- [[Drafts Index|Drafts-Index]]

---

> **Source:** [docs/protocol/draft-bellis-unheaded-shim-00.md](../docs/protocol/draft-bellis-unheaded-shim-00.md)
