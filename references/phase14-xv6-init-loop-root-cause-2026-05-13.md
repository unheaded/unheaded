# Phase 1.4 — xv6 early-init loop root cause located (2026-05-13)

**Status:** ROOT CAUSE IDENTIFIED — fix queued for next attended shift.
**Predecessor:** Phase 1.3 IMPL closure + Phase 1.4 kickoff (`458b8469`, `905c9bcc`, `7f1f05a4`).

## Problem statement

Phase 1.3 IMPL closed with xv6 reaching `main()`, printing "xv6 kernel is booting", then "halting" at insn=4000. Phase 1.4 probe (`--triggers 5000`) revealed the CPU isn't actually halted — it's in an **unproductive loop**: PC stays in the 0x640 vicinity through 40,000 insns with zero new TTY output.

## Root cause

xv6's `main()` calls `kinit()` (commit `73834054`'s captured TTY output shows we get past `consoleinit/printfinit/printf("xv6 kernel is booting")` and into the init chain). `kinit()` calls `freerange(end, PHYSTOP)` where:

- `end` is the linker symbol = end of kernel image (low address, ~0x30000 in our layout)
- `PHYSTOP = KERNBASE + 128*1024*1024 = 0x80000000 + 128 MiB = 0x88000000` per `crates/xv6-mbc/upstream/kernel/memlayout.h:39-40`

`freerange()` walks every 4 KiB page from `end` to `PHYSTOP`, calling `kfree(pa)` which prepends the page to a free list. That's:

```
(0x88000000 - 0x30000) / 4096 ≈ 558,815 pages
```

Half a MILLION kfree() calls. Even ignoring that most of the address range has no backing memory (RAM_MAP doesn't extend to 0x80000000+), the loop is the unproductive churn we're seeing.

## Why this maps to "xv6 stuck at insn=40000"

freerange's inner loop is:

```c
for(; p + PGSIZE <= (char*)pa_end; p += PGSIZE) kfree(p);
```

Each iteration:
1. `kfree(p)` validates `p >= end && p < PHYSTOP && p % PGSIZE == 0`
2. Memset's the page to 1 (in xv6's debug mode) — but here, the page is BACKED by BPF map RAM. Writes to address 0x80000000+ go through `translate_address()` → `mem_write_word()` which fails silently for out-of-range addresses.
3. Prepends to free list — touches `kmem` global.

40,000 BPF insns at ~50-80 insns per iteration of freerange means we're maybe 500-800 iterations in. Out of 558K. We'd need ~30 MILLION insns to drain freerange. Not happening with finite triggers.

## Fix recipe (next attended shift)

**Option 1 — Smallest patch (recommended)**: override PHYSTOP via the Makefile.

1. Edit `crates/xv6-mbc/upstream/kernel/memlayout.h` to make PHYSTOP overridable:
   ```c
   #define KERNBASE 0x80000000L
   #ifndef PHYSTOP
   #define PHYSTOP (KERNBASE + 128*1024*1024)
   #endif
   ```
2. Add to `Makefile.mbc` CFLAGS: `-DPHYSTOP=0x00800000UL` (just below the ramdisk region).
3. Verify the `end` linker symbol stays low (kernel image end ~0x30000); freerange will then walk ~7.8 MiB worth of pages = ~2000 kfree() calls, completable in a few hundred trigger packets.

**Option 2 — Adapter override**: ship `adapters/memlayout-mbc.h` and include it in compile order, but it's more files for the same effect.

**Option 3 — Patch kinit directly**: write `adapters/kinit-mbc.c` that replaces xv6's `kinit` with a UPC-aware version. More invasive, more flexible. Defer to Phase 2.

## Verification path post-fix

After the PHYSTOP fix:

1. Rebuild xv6-mbc.mbc:
   ```bash
   cd crates/xv6-mbc/upstream
   make -f ../adapters/Makefile.mbc clean kernel
   ```
2. Live boot with --triggers 2000 (enough for ~32K insns covering kinit + procinit + trapinit + binit + iinit + fileinit + userinit):
   ```bash
   cd ebpf
   sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
     --kernel ../crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
     --ramdisk ../crates/xv6-mbc/upstream/target/fs.img \
     --triggers 2000 \
     --instance 222
   ```
3. Expected TTY output extension: `"xv6 booting...\nxv6 kernel is booting\ninit: starting sh\n"` once xv6 reaches userinit → exec /init → init.c's first printf.
4. Likely subsequent failure point: `open("console", O_RDWR)` returns success (existing stub at fd=3), then `mknod` returns -ENOSYS — init bails. That's the NEXT Phase 1.4 syscall-stub work.

## Why we stopped here

Editing PHYSTOP touches xv6 upstream source which then needs:
- Verification that no OTHER xv6 code path uses PHYSTOP with the old assumption.
- Rebuild + retest cycle.
- Recovery if the new PHYSTOP boundary triggers a different bug.

That iteration is interactive — not appropriate for unattended autonomous churn. The diagnosis IS the value of this shift; the fix lands in 15 minutes of attended work next session.

## 2026-05-13 follow-up: PHYSTOP fix landed + kfree/kalloc memset skip

After landing the PHYSTOP=0x00800000 override and the `UPC_SKIP_KFREE_MEMSET`/`UPC_SKIP_KALLOC_MEMSET` CFLAGS (skipping xv6's debug "fill with junk" memsets — 4 KiB per page × ~1900 pages = millions of BPF insns avoided), PC advanced from 0x644 → 0x615 after a clean rebuild. Different loop location now, but still in early init.

Hypotheses for the remaining loop (next attended shift):
1. **`acquire(&kmem.lock)` spinlock**: xv6 spinlocks use atomic CAS via LR.W/SC.W. If the BPF interpreter's atomic semantics differ from xv6's expected behavior (we DID fix one reservation bug in Phase 1.3 Step 7), kmem.lock could be stuck.
2. **`kvminit()`**: walks the page table setup. Without proper Sv32 walker behavior in the BPF interpreter, could spin.
3. **`__sync_synchronize()`**: GCC builtin barrier. The translator emits it as MBC FENCE (opcode 0x3F) which is no-op on single-CPU MBC. Should NOT loop.
4. **`procinit()`**: initializes the proc table. Phase 1.2 + 1.3 widened PROC_TABLE; if procinit assumes the old shape, could loop.

Recommended next-shift first step: instrument `main()` with `mmio_puts("after kinit\n")`, `mmio_puts("after kvminit\n")`, etc. between each init call, rebuild, observe which `mmio_puts` prints last. The init call AFTER that one is the loop site. Then dig in.

**Phase 1.4 progress shipped 2026-05-13 despite the remaining loop:**
- mkfs host tool compiled
- userland init.mbc + init.rv2mbc + init.data built
- fs.img ramdisk built and loadable
- upc-bootctl `--ramdisk` + `--triggers` flags wired
- PHYSTOP cap + memset skip (saves millions of insns)
- Detailed root-cause documentation (this file)

The kernel substrate is one debug session away from a working `init: starting sh\n` print.

## Cross-references

- `crates/xv6-mbc/upstream/kernel/main.c` — the boot init chain.
- `crates/xv6-mbc/upstream/kernel/memlayout.h:39-40` — KERNBASE + PHYSTOP definitions.
- `crates/xv6-mbc/upstream/kernel/kalloc.c:27-44` — kinit + freerange.
- `crates/xv6-mbc/adapters/Makefile.mbc` — where the -DPHYSTOP override goes.
- `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` — UPC memory map (RAM_MAP, pgd region, ramdisk).
- `references/phase13-step8-userland-build-notes.md` — Phase 1.4 carry-over inheritance.
