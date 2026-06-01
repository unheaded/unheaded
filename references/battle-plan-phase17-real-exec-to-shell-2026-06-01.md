# Battle plan — Phase 1.7 "real exec → live shell prompt"

**Date:** 2026-06-01
**Owner:** Computermancer (+ Developer, Architect; BlackMage on the FS-read attack surface)
**Status:** DRAFT — needs §Decision (FS-reader placement) before code. Successor to
`battle-plan-phase12-make-mmu-live-2026-05-30.md`, whose Gates 1–4 (live per-pid MMU +
N-way isolation) all landed (`14ebe821`, `ed651dc1`, `0e023161`).

## Why now

Phase 1.2 gave every process a real, isolated address space (Gates 1–4). The fork/wait/exit
lifecycle loops cleanly. The ONE thing standing between us and a visible shell prompt is that
`init`'s child calls `exec("sh", argv)` and the ecall path has **no `SYS_exec` handler**, so it
returns to `init` which prints `init: exec sh failed` and re-loops forever. Closing the
exec + FS-read family turns "init: starting sh / init: exec sh failed" into an `sh` prompt — the
Phase 1 (L5) summit per `references/battle-plan-ascend-linux-2026-05-08.md`.

## The exact path to a prompt (from `user/init.c`)

```
open("console", O_RDWR)            // SYS_open   (15)  ── MISSING
  └ if <0: mknod("console",CONSOLE,0); open(...)   SYS_mknod (17)  ── MISSING
dup(0); dup(0);                    // SYS_dup    (10)  ── MISSING  (stdout, stderr)
for(;;){ printf("init: starting sh");
  fork();                          // SYS_fork   (1)   ── HAVE ✓
  child: exec("sh", argv);         // SYS_exec   (7)   ── MISSING (the big one)
  parent: wait(0); }               // SYS_wait   (3)   ── HAVE ✓
```
Then `sh` itself needs `read(5)` (console input), `write`(✓), `open`/`close`/`fstat`/`dup`,
plus `fork`/`exec`/`wait`(✓) to launch commands. So the minimal prompt set is **open, mknod,
dup, exec**; the minimal *usable* shell adds **read, close, fstat**.

## What already exists (substrate)

- **Per-pid isolated address spaces** — Gates 1–4. `pgd_base_for_pid`, disjoint 8 MiB physical
  slices (`phase12::pid_phys_offset`), live `translate_address` Sv32 walk, TLB flush on switch.
  This is exactly the consumer `exec` needs: a fresh slice to load `sh` into.
- **fork's memory-copy + pgd-build machinery** — the two-band working-set copy. `exec` reuses
  the pgd/leaf build but loads from the FS image instead of copying the parent.
- **`fs.img` resident in RAM** — bootctl loads the xv6-format ramdisk at byte `0x00800000`
  (`ADDR_RAMDISK_DEFAULT`, `bootparams.rs:20`). The bytes are present; nothing parses them yet.
- **ecall dispatch** — `fork/exit/wait/getpid/pause/write` handlers in the RV32I path
  (`main.rs` ~1756–2060). New handlers slot in beside them.
- **Kernel→user pointer translation** — already done for `SYS_write`'s buf; `open`/`exec` path
  strings and `read`/`fstat` out-buffers reuse the same `translate_address`-through-current-pgd.

## What's missing (the work)

1. **An xv6 filesystem reader** — resolve a path ("sh", "console") to its inode and read the
   file's data blocks out of the `fs.img` at `0x00800000`. (Placement = §Decision.)
2. **`SYS_exec` (7)** — read the named binary, validate the RV32→MBC translation is present for
   it, build a fresh per-pid address space (reuse Gate-1 pgd/leaf build), load its segments into
   the pid's physical slice, set entry PC, lay argv/argc on a fresh user stack, reset the
   trapframe. NOTE the MBC twist below.
3. **`SYS_open`/`SYS_close`/`SYS_dup`/`SYS_fstat`/`SYS_read` (15/21/10/8/5)** — a minimal
   per-process fd table (xv6 has `NOFILE=16`). `console` fd → the existing TTY MMIO path
   (write→0xC001 emit, read→keyboard/stdin); file fds → FS reader.
4. **`SYS_mknod` (17)** — only needs to satisfy `init`'s console create; can be a near-noop that
   registers the console device name (no real device-number table required for L5).

## The MBC exec twist (Architect + Computermancer must rule on this)

This CPU does **not** execute RV32I — it executes pre-translated **MBC bytecode** loaded into
ROM_MAP, with an `rv2mbc` address map. xv6's `sh`/`ls`/etc. are RV32 ELFs inside `fs.img`. So
"`exec` reads an ELF from the FS and jumps to it" is NOT directly runnable: there is no live
RV32→MBC translator in the BPF datapath. Three shapes (this IS the §Decision, intertwined with
FS placement):

- **(A) Loader-side pre-translation (RECOMMENDED — mirrors Phase 1.2 choice (i)).** Host-side
  `upc-bootctl`/build step parses `fs.img`, and for each userland program emits its MBC image +
  rv2mbc map into dedicated BPF maps (or a packed region), plus a tiny `name → {rom_off,
  rv2mbc_off, entry, data_off, len}` table. Then `SYS_open`/`SYS_exec` are **table lookups +
  a slice load + PC set** — verifier-cheap, no in-BPF ELF parser, no in-BPF FS walker. Smallest
  blast radius, fastest to a prompt. The `fs.img` stays the source of truth for *names/contents*;
  the loader is the translator. `sh ls cat echo wc` are already built by `Makefile.mbc-userland`.
- **(B) BPF-side xv6 FS walk + in-datapath translate.** Faithful to "exec reads the disk," but
  needs an inode/indirect-block walker AND a live RV32→MBC translator in eBPF — both
  verifier-hostile. Likely blows the budget; deferred.
- **(C) Revive xv6 `fs.c` + `exec.c` in-kernel.** The Linux-endgame-correct path; largest
  surface (fights the kexec/MBC-loader bypass). Same call as Phase 1.2 (iii) — right eventually,
  wrong for the L5 sprint.

**Recommendation: (A).** It gets a real `sh` prompt with the least new verifier-heavy code,
keeps every program's bytes coming from `fs.img`, and preserves (C) as the later
"real disk + JIT" endgame.

## Verification gates (strong-inference, per ADR-074 discipline)

- **Gate A — console stdio:** `init` runs `open("console")` + `mknod` + `dup`×2 and the existing
  `printf` path still emits to TTY through fd 1 (proves open/dup/mknod + fd table wired). Boot
  still shows `init: starting sh`.
- **Gate B — exec launches sh:** child `exec("sh", argv)` no longer prints `exec sh failed`;
  `sh` runs in its own pid-1 slice and emits its first byte (the `$ ` prompt or `init: starting
  sh` stops repeating). This is the headline.
- **Gate C — interactive line:** feed a byte stream to the console-in path; `sh` reads a command
  via `SYS_read`, forks+execs one of `ls`/`echo`, and its output appears. Proves the full
  read→parse→exec→write round-trip — the L5 summit.
- **Budget:** verifier stays < 12% hard gate. Choice (A) keeps `exec`/`open` as lookups, so the
  predicted delta is small (no FS walker, no ELF parser in-datapath); measure after Gate A.

## §Decision — needs Stevie (one pick unlocks code)

Pick the FS/exec placement: **(A) loader-side pre-translation** [recommended], **(B) BPF-side FS
walk + live translate**, or **(C) revive xv6 fs.c/exec.c**. (A) is consistent with the Phase 1.2
"loader-side large pages" call and is the shortest path to a prompt; (C) is the eventual Linux
path. Until this is picked, the steps below assume (A).

## First steps if (A) is chosen (check in at Gate A)

1. **Host-side image builder:** extend the `Makefile.mbc-userland` / `upc-bootctl` flow to walk
   `fs.img`'s directory, translate each program (reuse `rv32i_to_mbc`), and emit a userland
   program table (name → rom/rv2mbc/entry/data offsets) into a BPF map. No datapath change yet.
2. **fd table + console:** add a per-process fd table (`NOFILE=16`) to proc state; implement
   `SYS_open`/`SYS_dup`/`SYS_close`/`SYS_mknod` with the `console` name routed to the existing
   TTY MMIO path. → **Gate A** (boot still prints `init: starting sh`).
3. **`SYS_exec`:** lookup name in the program table, build a fresh pgd/leaf for the current pid
   (reuse Gate-1 build), load the program's MBC image + rv2mbc + data into its slice, set entry
   PC + a fresh argv stack, reset the trapframe to user mode. → **Gate B** (sh prompt).
4. **`SYS_read` + `SYS_fstat`:** console-in read path + minimal fstat for sh. → **Gate C**.

## Build / boot recipe (unchanged from Phase 1.6)

```bash
cd ~/tmp/unheaded/ebpf && cargo build --release -p monad-cpu-ebpf --features ascend-linux
cd ../crates/xv6-mbc/upstream && make -f ../adapters/Makefile.mbc clean kernel \
  && rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland ramdisk
cd ../../../ebpf && sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel  ../crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --ramdisk ../crates/xv6-mbc/upstream/target/fs.img \
  --userland ../crates/xv6-mbc/upstream/target/init.mbc \
  --triggers 3000000 --instance 222
```
Reminders (hard-won, from the Phase 1.2 footguns): ALWAYS rebuild with `--features ascend-linux`
before booting; run `upc-bootctl` from `ebpf/`; never trust a boot whose rebuild didn't print
`Compiling monad-cpu-ebpf`; `scripts/bpf-verifier-check.sh` clobbers the ascend object with a
non-ascend build — rebuild after running it.
