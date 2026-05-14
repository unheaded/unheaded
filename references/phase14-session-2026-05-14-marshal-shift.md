# Phase 1.4 — Marshal shift 2026-05-14 — kinit unblocked, kvminit panic identified

**Status:** Major progress. kinit() completes. New blocker: `kvminit()` panics on `mappages: size not aligned`, caused by `vm.c:42` arithmetic that assumes KERNBASE-relative PHYSTOP.
**Predecessor:** `phase14-xv6-init-loop-root-cause-2026-05-13.md`
**Shift commits (local, not yet pushed — SSH key not loaded):**

- `xv6-mbc`: instrumentation between every `main()` init call (`mmio_puts("after consoleinit\n")`, etc.) — 13 markers in `upstream/kernel/main.c`
- `xv6-mbc`: per-iteration `kfree` probes (`kfree-A`/`-B`/`-C` on first call, `freerange enter`/`exit` always) in `upstream/kernel/kalloc.c`
- `xv6-mbc`: `mmio_puts` promoted from static to exported symbol in `adapters/start_mbc.c` so kernel translation units can emit markers
- (this doc)

## Key findings (in chronological order)

### Finding 1 — apparent "MRET fall-through regression" was a build-flag oversight (NOT a code regression)

After the cleanup nuked `ebpf/target/` and `cmd/upc-bootctl/target/`, rebuilding `monad-cpu-ebpf` defaulted to **no features**. The MRET / SRET / LR.W / SC.W / FENCE handlers in `ebpf/monad-cpu-ebpf/src/main.rs:888-945` are gated behind `cfg!(feature = "ascend-linux")`. Without the feature, opcode 0x47 silently NOPs, the CPU walks past the `mret` in `start_mbc.c`, and the safety net `mmio_puts("BOOT FAIL: MRET fall-through\n")` fires.

**Correct build:**

```bash
cd ebpf
cargo build --release -p monad-cpu-ebpf --features ascend-linux
```

The CLAUDE.md / next.md memory of "MRET working at commit 73834054" was correct — that build was with `--features ascend-linux`. Same code, same behavior; just need the flag.

**Recommendation:** Add this to the `xv6-mbc` Makefile or to `cmd/upc-bootctl`'s pre-flight check so a missing flag is detected rather than silently degraded.

### Finding 2 — kfree's spinlock works; Hypothesis #1 FALSIFIED

The kalloc.c probes (`kfree-A` before acquire, `kfree-B` after acquire, `kfree-C` after release; static counter so each prints only once) all fired on the first `kfree()` call:

```
freerange enter
kfree-A
kfree-B
kfree-C
```

`acquire(&kmem.lock)` returned, `release(&kmem.lock)` returned. The kmem.lock spinlock works correctly. Hypothesis #1 from `phase14-xv6-init-loop-root-cause-2026-05-13.md` (xv6 spinlock CAS via LR.W/SC.W broken) is **falsified**.

This is consistent with Phase 1.3 Step 7 (commit `5e29408b` "clear LR.W reservation on context switch") — that fix did its job. LR.W/SC.W atomicity is correct.

### Finding 3 — the "infinite loop at PC=0x615" was a trigger-budget mirage

`phase14-xv6-init-loop-root-cause-2026-05-13.md` reported PC stuck at 0x615 after 40,000 MBC instructions. **It wasn't stuck — it was making progress, just slowly.**

Empirical scaling (instrumented kernel, 7553 RV32 → 11761 MBC translation):

| Triggers | MBC insns executed | TTY markers emitted |
|---|---|---|
| 5,000 | 40,000 | `after consoleinit`, `after printfinit`, "xv6 kernel is booting" |
| 20,000 | 160,000 | + `freerange enter`, `kfree-A/B/C` |
| 200,000 | 1,600,000 | + `freerange exit`, `after kinit` |
| 500,000 | 4,000,000 | + `panic: mappages: size not aligned` |

`kinit()` (which does `freerange(end, PHYSTOP)`) consumes roughly **1.6M MBC instructions** end-to-end. PHYSTOP=0x00800000 cap + memset-skip CFLAGS bring this to ~800 page-walks of `kfree` × ~2000 MBC insns/iter = ~1.6M insns. Matches.

The previous shift's `--triggers 5000` (80k MBC insn budget) saw freerange ~5% complete. The PC=0x615 sample is just wherever the budget exhausted inside the loop. **No bug. No fix needed.** Future debug needs `--triggers ≥ 200000` to clear kinit.

### Finding 4 — NEW BLOCKER: kvminit panics on size not aligned

At 500k triggers we reach `panic: mappages: size not aligned`. xv6 prints it via `printf` so it appears with the cosmetic NUL-padding pattern (`p··a··n··i··c··:··`), but the message is unambiguous.

**Source location:** `crates/xv6-mbc/upstream/kernel/vm.c:42`:

```c
// map kernel data and the physical RAM we'll make use of.
kvmmap(kpgtbl, (uint64)etext, (uint64)etext, PHYSTOP-(uint64)etext, PTE_R | PTE_W);
```

xv6 expects `KERNBASE ≤ etext < PHYSTOP`. Upstream's `memlayout.h` sets `KERNBASE = 0x80000000` and `PHYSTOP = KERNBASE + 128 MiB = 0x88000000`. `etext` is the linker symbol = first byte past `.text`, somewhere in `0x80000000`–`0x80020000`. `PHYSTOP - etext ≈ 0x7FFE0000` — well-aligned, fine.

Our `Makefile.mbc` overrides only PHYSTOP to `0x00800000` (CFLAG `-DPHYSTOP=0x00800000UL`) **without** updating KERNBASE. So `PHYSTOP - etext` becomes `0x00800000 - 0x800003E0` = a huge positive number (uint64 underflow). It's not a multiple of PGSIZE (4096), so `mappages` panics at line 155.

Similarly, line 39 `kvmmap(kpgtbl, KERNBASE, KERNBASE, (uint64)etext-KERNBASE, ...)` may or may not be problematic depending on whether `etext - KERNBASE` happens to land aligned (it usually does — kernel text aligns at PGSIZE boundary).

**The deeper truth:** on UPC, xv6's page tables are decorative. The actual address translation is done by the BPF interpreter's `translate_address()` — it walks RAM_MAP / ROM_MAP / TTY_MAP / FB_MAP / pgd substrate, not the RISC-V Sv32 page table. `kvminit` builds a page table that nothing reads. The right answer is to either no-op kvminit on UPC OR to fix the kvmmap argument arithmetic so it doesn't panic.

## Fix options for next shift

**Option A — minimal: short-circuit kvmmake on UPC.** Add to `Makefile.mbc` CFLAGS: `-DUPC_SKIP_KVMINIT`. In `kalloc.c` or via an adapter, wrap `kvmmake`'s body in `#ifdef UPC_SKIP_KVMINIT`:

```c
#ifdef UPC_SKIP_KVMINIT
  return kpgtbl;  // page table is decorative on UPC
#endif
```

Pro: 2-line patch. xv6 keeps allocating the kpgtbl page so `kvminithart`'s SATP write has something to point at, but doesn't try to populate it. Con: SATP write in `kvminithart` may still misbehave when CSR_REG(CSR_SATP) is written with a zero page table — needs verification.

**Option B — fix the math:** override the offending kvmmap calls. Replace the two large-range mappings (lines 39 and 42) with explicit byte ranges that we know are aligned. E.g.

```c
// kvmmap(kpgtbl, KERNBASE, KERNBASE, (uint64)etext - KERNBASE, PTE_R | PTE_X);
// kvmmap(kpgtbl, (uint64)etext, (uint64)etext, PHYSTOP - (uint64)etext, PTE_R | PTE_W);
#ifdef UPC_KVMINIT_FIXED
  // On UPC, identity-map the RAM_MAP region [0, PHYSTOP). Page table is
  // decorative (BPF translate_address is authoritative) but xv6's vm.c
  // assumes it can walk a valid pgtbl during fork/exec/exit.
  kvmmap(kpgtbl, 0, 0, PHYSTOP, PTE_R | PTE_W | PTE_X);
#else
  /* original two calls */
#endif
```

Pro: keeps the page table coherent with the BPF interpreter's view. Con: bigger patch, needs PHYSTOP to itself be PGSIZE-aligned (it is — 0x00800000 = 8 MiB).

**Option C — defer kvminit:** comment out the call from `main.c` entirely, run xv6 without paging. Pro: smallest patch. Con: xv6's process model assumes SATP is set; trap.c uses `MAKE_SATP(p->pagetable)` on user trap; userland exec would blow up later.

**Recommendation:** Option A first (the 2-line patch), verify boot reaches `after kvminit` + `after kvminithart`, then handle the next downstream panic. If SATP write breaks, fall back to Option B or extend Option A to also stub `kvminithart`.

## Verification recipe for next shift

```bash
cd ~/tmp/unheaded/ebpf
cargo build --release -p monad-cpu-ebpf --features ascend-linux  # the flag matters!

cd ../crates/xv6-mbc/upstream
make -f ../adapters/Makefile.mbc clean kernel

cd ../../../ebpf
sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --triggers 500000 \
  --instance <pick a number>
```

Expected TTY contains, in order: `xv6 booting...`, `after consoleinit`, `after printfinit`, `xv6 kernel is booting`, `freerange enter`, `kfree-A`, `kfree-B`, `kfree-C`, `freerange exit`, `after kinit`, then either the next panic OR `after kvminit` if you patched it.

## Side findings (write-ups for the Librarian)

1. **Cleanup script needs hardening.** The `git clean -fdX` step swept `crates/xv6-mbc/upstream/mkfs/` entirely because the xv6 upstream `.gitignore` line 14 says `mkfs` (no slash, no anchor), which matches both the binary and the source directory. Re-vendored `mkfs.c` from `https://raw.githubusercontent.com/mit-pdos/xv6-riscv/riscv/mkfs/mkfs.c` and rebuilt. The `clean-artifacts.sh` ARTIFACT_DIRS list also misses several `cmd/*/target/`, `crates/*/target/`, and `crates/xv6-mbc/upstream/{kernel,adapters}/*.o` paths — `git clean -fdX` catches them via `.gitignore` so the script *works*, but the dry-run preview underreports (`3353M` vs actual `21GB` recovered).

2. **console-mmio.c regression vs start_mbc.c fix.** `console-mmio.c::uartputc_sync` writes a uint32 at byte 0xC001 — the same 4-byte-store splitting bug that `start_mbc.c` fixed at line 84-93. This is what causes the `x··v··6··` NUL-padded output from xv6's printf, while start_mbc.c's direct `mmio_puts` emits clean ASCII. Cosmetic; doesn't block boot; one-line fix when convenient (change `volatile uint32 *` → `volatile uint8 *` and store byte).

3. **MRET feature gate needs a guard.** A monad-cpu-ebpf build without `--features ascend-linux` is silently wrong for ASCEND-LINUX work. Either add a `compile_error!` when targeting xv6/uClinux/Linux without the feature, OR have `upc-bootctl` refuse to load a binary that doesn't define the MRET symbol. Currently the failure mode is a 44-byte TTY blurb that *looks* like a kernel regression.

## Why we stop here

The Marshal shift is at the ~60-minute mark of bounded autonomous debug. The kvminit fix is interactive — multiple Options, each needs a rebuild + boot to validate, and Option A risks chaining into a SATP-write bug that needs CSR-translation inspection. That's appropriate for an attended session, not unattended churn. Captured everything in this doc; committing locally; pushing blocked on SSH key.

## Cross-references

- `phase14-xv6-init-loop-root-cause-2026-05-13.md` — predecessor diagnosis (superseded for Hypothesis #1; reset baseline for Hypothesis #2)
- `crates/xv6-mbc/upstream/kernel/main.c` — instrumented with 13 `mmio_puts` markers
- `crates/xv6-mbc/upstream/kernel/kalloc.c` — `kfree-A/B/C` + `freerange enter/exit` probes
- `crates/xv6-mbc/adapters/start_mbc.c` — `mmio_puts` non-static
- `crates/xv6-mbc/upstream/kernel/vm.c:42` — the panicking kvmmap call
- `ebpf/monad-cpu-ebpf/src/main.rs:888-945` — MRET/SRET/LR.W/SC.W behind `ascend-linux` feature
- `/home/govan/tmp/next.md` — wake-up briefing for next session
