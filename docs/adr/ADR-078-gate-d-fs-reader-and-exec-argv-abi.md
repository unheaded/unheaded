# ADR-078 — ASCEND-LINUX Phase 1.7 Gate D: In-BPF FS Reader + exec argv ABI

**Status**: ACCEPTED — **IMPLEMENTED + GATE D SHIPPED 2026-07-02**
**Date**: 2026-07-02
**Deciders**: Design A per the Gate D battle plan (2026-06-28); implemented across the 2026-06-28→07-02 sessions
**Aligns with**: ADR-067 (MBC ISA v2 + UPC ABI v1), ADR-074 (per-pid pgd slices), ADR-077 (per-process rv2mbc_base + exec)
**Implementation**: commits `f621c662`→`e47bf2c5`; session log `references/phase17-gateD-shipped-2026-07-02.md`; plan `docs/battle-plans/PHASE17-GATE-D-FS-READER.md`

## 1. Decision — the protocol computer walks its own inodes (Design A)

`open(15)` / `read(5)` / `fstat(8)` for regular files walk the xv6 `fs.img` **directly
from `RAM_MAP` inside the eBPF CPU** (superblock → dinode → data block), NOT through the
`blk-ramdisk`/`SYS_READ_BLOCK` sector path or any host-side helper. All address
arithmetic is pure and off-target-tested in `monad_common::fs_walk` (BSIZE=1024,
`FS_WORDS_PER_BLOCK=256` — never the 512-byte sector constants). Per-fd `{inum, offset}`
state lives in `FD_INODE_MAP` beside the `FD_TABLE` kind tags (`monad_common::fdtable`).

## 2. Guest ABI truths this gate froze

- **ilp32e struct stat is 16 bytes, `size` at offset 12.** xv6 `types.h` typedefs
  `uint64` as `unsigned long` — 4 bytes under `-mabi=ilp32e`. Any handler writing the
  RV64 24-byte layout corrupts the 8 bytes after the caller's struct (the Gate D
  "blank names + size 0" bug). Kernel-side copy-outs MUST use the ilp32e layout.
- **exec argv frame** (user-visible ABI): arg strings in 8 × 16-byte slots at VA
  `0x0060_0000`; NULL-terminated pointer array (9 words) at VA `0x0060_0080`; on entry
  `a0 = argc` (≤ 8), `a1 = 0x0060_0080`, `sp = 0x0050_0000`. `[0x0050_0000,
  0x0080_0000)` is reserved VA between stack top and `USER_VA_CEILING` — the frame must
  not sit below the stack top because the CALLER's argv (e.g. init's stack-local
  `argv[]`) lives there and the harvest reads it across re-entry ticks. Args cap at
  ARGV_MAX=8 × ARG_CAP=16 B (tail force-NUL'd; longer args truncate, never
  unterminated).
- **write(16) honors n** (≤ 4096 clamped), one byte per tick re-entrant; a short-write
  return is no longer part of the console contract (xv6 `echo` ignores and `cat`
  fatals on short writes).

## 3. Re-entry register protocol (the a2/a3 conventions)

Multi-tick syscalls park state in caller-saved user regs across `pc -= 1` re-entries.
Registry (collisions here = silent state corruption):

| Handler | a2 (regs[10]) | a3 (regs[11]) |
|---|---|---|
| open(15) dirent scan | cursor (dirent idx) | `OPEN_MAGIC 0x09E0_0DE0`, **cleared on completion** |
| exec(7) phase A argv | cursor (arg idx) | `EXEC_ARGV_MAGIC 0xE0EC_A26F` |
| exec(7) phase B .data | cursor low 16 b, **argc high 16 b** | `EXEC_MAGIC 0xE0EC_B10C` |
| write(16) n>1 | (holds n) | `0xD0C0_0000 \| cursor`, cleared on completion |
| fork(1/2) copy | cursor | (pde0-presence gates reset) |

Rules learned: (a) magics MUST be cleared when a scan completes — a stale magic plus
whatever the user last left in the cursor register silently skips work on the next
call; (b) hostile/stale cursors are clamped on entry (H-bound); (c) verifier-proven
shapes only: 1 item/tick scans, fixed 16-byte no-guard copies, ≤16-word/tick block
copies.

## 4. Consequences

- `ls`, `cat`, `echo` (and sh command lines up to 64 bytes of scripted `--input`) work
  end-to-end against the real filesystem image; the FS is read-only at this gate.
- Writes (`SYS_write` to FD_INODE), device inodes, indirect blocks (files > 12 KiB),
  and subdirectory path walks remain out of scope (single root-dir walk, H-FS4 caps).
- Known issue → Gate D.1: `wc README` runaway (see session log; wc was unreachable
  before argv existed, so no baseline was lost).
