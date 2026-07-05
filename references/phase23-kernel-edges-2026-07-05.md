# Phase 2.3 — kernel edges COMPLETE — 2026-07-05

**Session**: 2026-07-05 (Fable 5, same session as the Phase 2.2 sh ship).
**Battle plan**: `docs/battle-plans/PHASE23-KERNEL-EDGES.md` (7 phases, 96 steps).
**Result**: SHIPPED. The four remaining MIT kernel edge files left the link — with the
Phase 1.1 adapters (start/uart/plic-drop/virtio), every kernel edge is now
Unheaded-authored. MIT remaining in the link: kernel core only (16 files, Phase 2.4
scope). Zero eBPF change (ascend 900,031 / 90.0% exact; Doom 737,087). Kernel
translation count unchanged at every step: 7,552 RV32I → 11,764 MBC (1.6x).

## Commits

| Commit | Edge | Notes |
|--------|------|-------|
| `7c71c90c` | entry → `adapters/uentry.S` (+ the battle plan) | near-merger-doctrine; `_entry` disassembly instruction-identical |
| `8f4caf1e` | syscall dispatch → `adapters/usyscall.c` | runtime-dead on UPC; bounds at stock strength |
| `4ada1df6` | printf → `adapters/uprintf.c` | LIVE — every boot print gated by goldens |
| `1ac208ac` | console → `adapters/uconsole.c` | console stack 100% ours (top + console-mmio bottom) |
| (this) | docs: session log, banner, ADR/master-plan ripples, handoff | |

## The architecture insight that shaped the sprint

**User syscalls never reach the kernel on the UPC.** The ascend BPF ecall dispatch
(`ebpf/monad-cpu-ebpf/src/main.rs`, SYS_* handlers) services every user syscall
in-BPF; `syscall.c`, and console.c's read/write/intr halves, are LINK-TIME
requirements only (trap.c references `syscall()`, sysproc/sysfile use `arg*`,
devsw wants function pointers). They come alive in Phase 2.4 when the trap path
becomes ours — so every bounds check was preserved at full stock strength rather
than simplified away. Dead code today is the attack surface of tomorrow.

The LIVE kernel surface this sprint touched: `_entry` (first instructions),
printf → consputc (every boot print in the golden TTY line), consoleinit
(lock + uartinit + devsw wiring).

## Method

- **Goldens reused from phase22** (captured the same day, same fs.img + userland;
  validity argument: nothing outside the kernel changed this sprint). Phase 0
  determinism probe (H2): kernel rebuilt from HEAD sources booted byte-identical
  BEFORE any edit — proving the goldens gate HEAD, not stale bytes.
- Per-edge loop: read vendored file (H3: zero UPC patches in all four — pristine
  MIT) → write u-file in `adapters/` (the ours-side home since Phase 1.1; Track 2
  u-prefix) → swap ONE line in the Makefile.mbc OBJS list → rebuild → FILE-symbol
  proof (`objdump -t | grep 'df \*ABS\*'`) → smoke (`echo hello` TTY+stats
  byte-diff vs golden) → full 16-gate harness → commit. Four green commits.
- **The translation-count invariant**: 7,552 → 11,764 at every step. GCC -O2
  folded every restructure (handler_for(), printstr(), erase_one()) back to
  stock-equivalent code — behavior-identical by construction, and a free
  regression signal (any count drift would have flagged a semantic change).
- Final Phase 5 sweep: all 12 boots (8 corpus + uinit + nosuch + multi + blank)
  through the all-ours-edges kernel. `ls` is diffed against the phase22 ush-era
  capture, since the golden predates the ush swap (its one divergent field is
  sh's own file size — classified in `references/phase22-sh-shipped-2026-07-05.md`).

## Subtleties preserved (the load-bearing details)

| File | Detail | Why it matters |
|------|--------|----------------|
| uentry.S | csrr mhartid kept in the sp computation | bit-equal on any hart count; translator maps CSRs to 0x000_F000 |
| usyscall.c | fetchaddr's dual guard (`addr >= sz \|\| addr+8 > sz`) | overflow past p->sz |
| uprintf.c | per-specifier va_arg types (%d int, %ld uint64, %u uint32…) | ilp32e: uint64 = 4 B — decode widths are ABI, not style |
| uprintf.c | %p = sizeof(uint64)*2 nibbles = 8 on ilp32e | pointer width follows the ABI |
| uprintf.c | panicking (lock bypass) vs panicked (freeze) split | panic inside printf deadlocks otherwise |
| uconsole.c | ^D save-for-next-read (`cons.r--`) | caller must see the 0-byte EOF read |
| uconsole.c | `cons.e - cons.r < INPUT_BUF_SIZE` ring bound | unsigned wraparound-safe fullness test |

## Footguns confirmed / learned

- `Makefile.mbc clean` runs `rm -rf target` — that is the SAME target/ dir as the
  userland artifacts (fs.img, *.mbc). Running it mid-sprint would have nuked the
  golden validity. Targeted rebuild instead: `rm -f kernel/kernel.elf && make -f
  ../adapters/Makefile.mbc kernel`.
- The translation-count line (`Translation successful: N RV32I → M MBC`) is a
  cheap per-build equivalence tripwire for behavior-preserving replacements —
  worth checking before booting anything.

## Next

Per ADR-081: **Phase 2.4 — own the kernel core** (kalloc, spinlock, string, main,
vm, proc, trap, sysproc, bio, fs, log, sleeplock, file, pipe, exec, sysfile —
16 files; at the end it is Unheaded Linux, not xv6). Big bite — needs its own
Warmonger plan, likely multiple sessions, and wakes the dead paths (trap →
syscall(), consoleread/intr) with new gates. Alternatives: Epic 1.3.1 device
inodes (first eBPF spend since the freeze) or the verifier-headroom spike
(required before Epic 1.3.2 file writes).
