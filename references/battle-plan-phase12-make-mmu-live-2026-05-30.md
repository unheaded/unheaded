# Battle plan — Phase 1.2 "make the MMU live" (Option A / A1)

**Date:** 2026-05-30
**Owner:** Computermancer (+ Developer, Architect, BlackMage on the isolation gate)
**Status:** GATE 2 + GATE 3 LANDED 2026-05-31. Gate 3 = clean fork→exec→wait→reap→refork loop
with pid recycling (root cause was missing slot recycling, NOT the handoff's "num_processes=8
phantom"). Gate 2 = isolation PROVEN at runtime via `user/gate2.c` ("P: ISOLATION-PASS" — child's
write to a shared VA does not leak to the parent). Markers stripped; kernel verifier accepts the
ascend-linux object. Uncommitted; Stevie to review+commit. See "## RESOLUTION 2026-05-31" below.

**Status (historical):** IN PROGRESS — strategy (i) large-page loader-side chosen.
**Gate 1 PASS (2026-05-30):** MMU live, pid-0 identity pgd (16×4MiB superpages) at 0x00F00000,
priv-gated translation; boot still reaches `init: starting sh`.
**Gate 2 IN PROGRESS (2026-05-30):** per-pid offset pgd + chunked fork-copy + syscall pointer
translation + TLB-flush-on-context-switch all implemented (uncommitted, with temporary debug
markers still in `main.rs`). Two root causes found and fixed along the way:
1. **Doom/xv6 syscall-number collision (the big one):** `sys::SYS_DRAW_FRAME=1 / GET_KEY=2 /
   GET_TICKS=3` shadowed xv6 `fork=1 / exit=2 / wait=3` in the RV32I dispatch chain — the
   fork/wait/exit handlers had been DEAD CODE since Phase 1.6. Fixed by gating the Doom
   syscalls behind `!cfg!(feature="ascend-linux")`.
2. **Verifier rejection:** a 1024-word fork-copy chunk blew the 1M-insn verifier limit.
   Dropped to 16 words/tick (turbo-bounce amortizes the extra ticks).
After both fixes, multi-process scheduling RUNS: context switches (`@`), process exits (`!`),
and the child's `init: exec sh failed` branch all fire.
**Remaining Gate-2 blocker:** `cpu.num_processes` reads as **8** (=MAX) by the time init forks,
so fork always bails to -1 ("init: fork failed") and never finalizes (no `~`).
- DISPROVEN: INT-0x80 `SYS_FORK`/`SYS_VFORK` inflation — gated both under `!ascend-linux`,
  `num_processes` still 8. Under ascend-linux ALL three `num_processes += 1` sites are now dead
  (two gated, one in the RV32I finalize that's never reached). So nothing increments it at
  runtime, bootctl writes 1, yet it reads 8.
- DISPROVEN: struct-layout mismatch — `monad_common::MbcCpuState` and bootctl's mirror
  (`runner.rs`) are byte-identical in field order + types. `num_processes` is at the same offset
  in both. bootctl writes 1.
- **PRIME SUSPECT NOW: stale `.o` / build caching.** Both "disproofs" (INT-0x80 gate, struct)
  were judged from boots whose rebuild I did NOT confirm emitted `Compiling monad-cpu-ebpf`
  (I grepped only `error|Finished`). If the gate build was cached, INT-0x80 `SYS_FORK` is STILL
  live and IS the num_processes=8 source after all. The build target is `bpfel-unknown-none`
  (`.cargo/config.toml`); the loaded object is `ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf`
  (bootctl resolves it relative to CWD — **always run bootctl from `ebpf/`**).

### Fresh-session first steps (do in order)
1. **Clean rebuild**: `cd ebpf && touch monad-cpu-ebpf/src/main.rs && cargo build --release -p
   monad-cpu-ebpf --features ascend-linux 2>&1 | grep -iE 'compiling|error|finished'` — CONFIRM
   it says `Compiling monad-cpu-ebpf`. Verify `.o` mtime updates.
2. Re-run the boot (from `ebpf/`, absolute kernel paths, `--triggers 3000000`) and re-read the
   `N<n>` marker. If it now shows `N1`/no-bail and `~` (fork finalize) appears → the INT-0x80
   gate was the fix and the earlier "disproof" was a stale build.
3. If `num_processes` is STILL 8 on a confirmed-fresh build: add a marker dumping `num_processes`
   at the TOP of the SYSCALL dispatch (first userland ecall) to see its value before any fork,
   and a marker in the INT-0x80 `if vector == VECTOR_SYSCALL` block to see if that path fires
   during xv6 boot at all.
4. Once fork finalizes: strip ALL debug markers (`~ @ ! A . N` + the syscall-trace block +
   the Doom-syscall gating comments can stay), re-run Gates 2/3, verify the isolation sentinel
   and a clean fork→exec→wait loop (expect `init: exec sh failed` from the child, init reaping
   and re-forking without the table filling).

### Debug markers to remove before commit (all in `ebpf/monad-cpu-ebpf/src/main.rs`)
- syscall-trace block at the top of the `op::SYSCALL` dispatch (the `0-9/a-z` emitter)
- `~`+pid (fork finalize), `@`+pid (scheduler switch), `!`+pid (exit), `A` (fork first-entry),
  `.` (per copy tick), `N`+digit (fork bail)
KEEP (real fixes): the `!cfg!(feature="ascend-linux")` gates on the Doom syscalls AND on
INT-0x80 `SYS_FORK`/`SYS_VFORK`; the 16-word copy chunk; all of phase12.rs/translate_address/
fork-copy/TLB-flush logic.
- Curiosity to explain: once pids wrap 0–7, processes flip from "fork failed" to
  "init: exec sh failed" (the child branch) — consistent with the scheduler round-robining
  8 phantom slots.

**What's PROVEN working at this checkpoint:** syscall collision fixed (fork/exit/wait reach
handlers), verifier passes (16-word copy chunk), context switches (`@`) + process exits (`!`)
fire, per-pid pgd build + TLB flush + pointer translation are in. Debug markers still in
`main.rs` (must be removed before commit). Once `num_processes` is fixed: remove markers,
re-run Gates 2/3, verify isolation sentinel + clean fork→exec→wait.
**Decided already (ADR-074, 2026-05-12):** Option A per-task pgd; Allocator A1 fixed per-pid
pgd at `RAM_MAP[0x00F00000 + pid*0x1000]`; `PROC_TABLE[20] = page_dir_base`.

## Why now

Phase 1.6 landed real `fork`/`wait`/`exit` (RV32I ecall path), but they can't produce a
shell because **all processes share one flat `RAM_MAP`** (`mmu_enabled==0`,
`translate_address` is a passthrough). The child clobbers the parent's stack. Per-process
isolation = the decided Option A MMU, turned on. See
`references/phase16-session-2026-05-30-fork-wait-exit.md`.

## What already exists (inert substrate)

- `translate_address` (`ebpf/monad-cpu-ebpf/src/main.rs:2239`) — Sv32 2-level walk + 64-entry
  TLB; early-returns while `mmu_enabled==0`. Wired into all 6 LOAD/STORE sites.
- `phase12.rs` — `pgd_base_for_pid`, `PER_PID_PGD_BASE=0x00F00000`, `PROC_TABLE_PGD_SLOT=20`.
- `SYS_FLUSH_TLB` — chunked 8/tick flush (verifier-safe). `SYS_SET_PAGE_DIR`, `SYS_ENABLE_MMU`.
- `scheduler_context_switch` already saves/loads `page_dir_base` per pid (slot 20).

## What's missing (the work)

1. **Populate page tables.** Build a Sv32 page table at each pid's pgd region mapping that
   process's VA pages to a DISJOINT physical RAM slice. (Construction strategy = §Decision.)
2. **Enable translation for userland** — set `mmu_enabled=1` + `page_dir_base=pgd_base_for_pid(pid)`
   at user entry (kexec/SRET-to-user).
3. **fork → real address space** — child gets `pgd_base_for_pid(child_pid)`, its own physical
   slice, and a COPY of the parent's slice. (Register copy already done; add memory copy +
   pgd build.)
4. **Context switch** — already loads slot-20 pgd; ensure TLB flush fires (fold the
   `phase12_option_a` hook into `scheduler_context_switch` or enable the feature).
5. **Kernel→user pointer translation** — syscall handlers that read/write user pointers
   (`SYS_write` buf, `wait` status, exec path) must resolve through the current pgd
   (walkaddr equivalent), not raw `mem_read_byte`.

## Verification (strong-inference, per ADR-074)

- **Gate 1 (single process):** enable MMU for init alone with an identity slice; boot still
  reaches `init: starting sh`. Proves the live walk path.
- **Gate 2 (isolation — the hard safety property):** PID 1 writes a sentinel to a VA; switch
  to PID 2; PID 2 reads the same VA and must NOT see the sentinel (disjoint physical slices).
- **Gate 3 (lifecycle):** init forks child, child runs in its own slice, prints
  `init: exec sh failed` (Stage-2 signal) WITHOUT corrupting init; init's wait reaps and loops
  cleanly (no chaotic `0x4024` divergence).
- **Budget:** verifier stays < 12% hard gate (predicted delta +0.2–0.8% per ADR-074 §Issue 3).

## §Decision — page-table construction strategy (needs Stevie)

Where do the Sv32 tables get built? Three tractable shapes:

- **(i) Loader/BPF-side, 4 MiB large pages** — add large-page support to `translate_address`
  (pde "leaf" flag short-circuits the 2nd-level read), build per-pid pgds with a handful of
  4 MiB mappings (per-pid physical offset). Fewest entries, smallest blast radius, keeps Sv32
  fidelity. Needs a small `translate_address` change.
- **(ii) Loader/BPF-side, 4 KiB pages, minimal footprint** — map only the pages init/sh
  actually touch (stack + data). No walker change, but must enumerate the working set and
  grow it on faults (more bookkeeping).
- **(iii) Revive xv6 `uvm`/`exec`** — un-skip `kvminit` (fix the PHYSTOP/etext math) and route
  userland through xv6's real page-table construction. Most faithful to upstream, biggest
  surface (fights the MBC-loader/kexec bypass that exists today).

**Computermancer recommendation: (i).** Large-page identity-with-offset is the least code to
a working isolated shell, preserves the Sv32 path for Linux, and the `translate_address`
large-page tweak is small and verifier-cheap. (iii) is the "right for Linux" endgame but is a
separate, larger effort better tackled once isolation is proven with (i).

## First step if (i) is chosen

Add large-page (leaf-pde) support to `translate_address`, build pid-0 pgd with 4 MiB
identity mappings over the userland RAM region, set `mmu_enabled=1` at init entry → **Gate 1**.
Check in at Gate 1 before touching fork-copy.

## RESOLUTION 2026-05-31 (fresh session — Gate 2/3 landed)

**The prior diagnosis was WRONG.** It claimed `num_processes` "reads 8 by the time init forks"
and treated 8 as a phantom present from boot (suspects: INT-0x80 inflation, struct mismatch,
stale `.o`). All three were already disproven; this session disproved the framing itself.

**Strong-inference steps that found the real bug:**
1. Confirmed-clean BPF rebuild (`Compiling monad-cpu-ebpf`, mtime bump) — `num_processes`
   STILL 8. Kills the stale-`.o` prime suspect.
2. Added `num_proc`/`cur_pid` to bootctl's before/after CPU_MAP prints (host-only rebuild):
   **before triggers = 1, after = 8.** So bootctl writes 1 correctly; the field climbs
   1→8 *during execution* — not a boot-time phantom.
3. Verified MbcCpuState is byte-identical between `monad-common` and bootctl's `runner.rs`
   mirror (both `#[repr(C)]`, MBC_REG_COUNT=16, size 136) — layout disproof genuinely holds.
4. Confirmed no direct `num_processes =` write anywhere; only three `+=` sites (two
   INT-0x80, dead under ascend-linux; one RV32I-finalize, past the bail).
5. **Trigger sweep** (the key): triggers 1.3M→1.6M→1.9M gave `num_proc` 4→5→7. The field
   **climbs gradually** with execution. The 466-byte steady-state tail (`N8` loop) is what
   the prior author saw and mis-read as "8 from boot" — the early successful forks had been
   overwritten in the 4096-byte wrapping TTY ring.

**Real root cause:** `num_processes` was a monotonic high-water counter and reaped children
never freed their `PROC_TABLE` slot. init's loop is `fork → child exec-fails → child exits →
init waits/reaps → repeat`; each fork burned a permanent slot, so after 7 forks the table
filled and every later fork bailed `N8` forever. Not corruption, not a stale build, not a
phantom — a missing **slot-recycling** path.

**Fix (3 edits in `monad-cpu-ebpf/src/main.rs`, RV32I ecall path):**
- SYS_fork: allocate the lowest FREE pid (never-used `pid >= num_processes`, or fully
  collected = halted&reaped) instead of `child_pid = num_processes`. Bail only when ALL
  slots are live. Scan reads masks only (stable across copy-tick re-entries).
- SYS_fork finalize: clear the (possibly reused) pid's halted+reaped bits and extend
  `num_processes` only if the pid is new (`max`, not `+= 1`).
- SYS_wait reap: clear the reaped child's pgd `pde[0]` PRESENT bit so a fork reusing that
  pid re-runs first-entry (rebuild superpages + reset copy cursor).

**Verified:** clean loop `init: starting sh / init: exec sh failed` repeats indefinitely,
`num_proc` pinned at 2, pid recycled as 1 every cycle, zero `N8` bails. All BPF debug
markers (`A ~ ! @ N` + syscall-trace + copy-tick `.`) stripped. Verifier gate PASSED at
10% (<12% hard gate). bootctl keeps the `num_proc`/`cur_pid` diagnostic print (useful).

**Gate 2 sentinel test — DONE 2026-05-31 (PROVEN at runtime):** wrote a purpose-built
userland `crates/xv6-mbc/upstream/user/gate2.c` (built via `make -f Makefile.mbc-userland
gate2`, runs as the `--userland` program / pid 0). It stamps a `volatile int` VA = 0xCAFE,
forks, the child confirms it inherited the copy then clobbers the same VA to 0xBEEF and exits;
after reaping, the parent re-reads the VA and asserts it is STILL 0xCAFE. Live TTY (repeats
each reboot — gate2 exits, kernel restarts it):
```
C: inherited-ok
C: clobbered-own-slice
P: ISOLATION-PASS
```
The parent never sees the child's write → disjoint physical slices confirmed empirically, not
just structurally. (Uses fixed-string `printf` only; the `%x`-format varargs ABI is not yet
reliable on the MBC userland — separate, unrelated to isolation.)

**Footgun hit this session:** `scripts/bpf-verifier-check.sh` line 24 runs `cargo build
--release` WITHOUT `--features ascend-linux`, which CLOBBERS
`ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf` with a build where MRET/SRET NOP →
every later boot dies `BOOT FAIL: MRET fall-through` at insn ~383. After running that script
(or any plain `cargo build`), ALWAYS rebuild with `--features ascend-linux` before booting.
The script's "10% budget" was measured on that wrong (non-ascend) object; the ascend object is
~820 KB and the real kernel verifier ACCEPTS it on load (boots run 5M–48M insns). Also: run
`upc-bootctl` from `ebpf/` — it resolves the BPF object relative to CWD (running from
`crates/xv6-mbc/upstream/` silently finds nothing / a stale object).

**Fork-copy perf — DONE 2026-05-31 (19× faster).** fork() used to copy the full 6 MiB
`USER_SLICE_BYTES` at 16 words/tick = ~98,304 ticks/fork, almost all of it the zero gap between
the low user image and the high stack. Replaced with a **two-band working-set copy** (new
`phase12::FORK_COPY_LOW_BYTES`=256 KiB + `FORK_COPY_STACK_BYTES`=64 KiB): a linear cursor maps to
the low image+heap band `[0,256KiB)` then a 64 KiB window around the page-aligned SP (live stack
≈ 5 MiB). ~5,120 ticks/fork now. Verified bit-for-bit behavior unchanged on BOTH oracles —
gate2 still prints ISOLATION-PASS (the low band carries the 0xCAFE in .data, the stack band the
live frames) and init still runs its clean lifecycle loop — and userland is reached at ~2M
triggers instead of needing 3–6M. `program_break` reads 0 at runtime (kernel doesn't track the
heap), which is why the low band is a fixed 256 KiB rather than `[0, brk)`. Assumes user
image+heap < 256 KiB and stack depth < 64 KiB at fork; fine for init/sh/gate2, revisit (or fall
back to full-slice) for malloc-heavy/deep-recursion userland. Note: timer interrupts key on
`bpf_ktime`, so boot progress per fixed `--triggers` still varies run-to-run; bump triggers and
retry to land in the userland window. The ascend-linux object is ~822 KB (~11% of the 900K-insn
budget; kernel verifier accepts it on load).
