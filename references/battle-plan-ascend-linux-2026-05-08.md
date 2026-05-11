# Battle Plan — Linux on UPC

**Codename:** ASCEND-LINUX (Dream Ladder L5 → L6 → networking)
**Authored:** 2026-05-08
**Source-of-truth mirror (post-approval):** `references/battle-plan-ascend-linux-2026-05-08.md`
**Predecessor plans / specs absorbed:** `docs/doom/ROAD_TO_LINUX.md`, `docs/doom/UPC_OS_PRIMITIVES.md`, `docs/doom/UPC_BOOT_PROTOCOL.md`, `docs/doom/MBC_LINUX_FAQ.md`, `docs/doom/MBC_LINUX_DEMO.md`, `docs/doom/UCLINUX_PORT_GUIDE.md`, `docs/doom/FUZIX_PORT_ANALYSIS.md`, ADR-023, ADR-043, ADR-69420.

---

## 1. Context

### Why now

The Unheaded Protocol Computer (UPC) Dream Ladder reached **Level 4 OS Primitives complete** on 2026-03-15: timer interrupts, syscall dispatch, round-robin scheduler, MMU/paging, ramdisk block device, console I/O — all six primitives implemented in `crates/monad-mbc/` and `ebpf/monad-cpu-ebpf/`, validated by 37 integration tests at 1170 lines (`crates/monad-mbc/tests/os_primitives_test.rs`). The boot protocol is fully specified (`docs/doom/UPC_BOOT_PROTOCOL.md` v1, magic `UNHD`). 24+ Linux-compatible syscall numbers are wired. The RV32I → MBC translator works (Doom plays at 35 fps internal, 5.9B+ instructions stable, E1M1 completable per `docs/doom/STATUS.md`).

Stevie's chosen end-state: **Full Linux 6.x with MMU emulation + TCP/IP-over-Monad networking inception** — the maximalist Dream Ladder summit. L5 path is **xv6-riscv adapted to MBC** as the canonical halfway proof. Schedule is **staged with quarterly gates** so each phase has a ship-or-defer decision before the next begins. Autonomy is **hybrid**: mechanical phases (toolchain, ELF loader, BSS zero-fill, syscall stubs, build pipeline) run unattended Marshal-led; design phases (ABI freeze, kernel-choice, network-stack model, demo script) are pair-programmed with Stevie.

### What Doom taught (extracts that drive the plan)

The Doom port (`docs/doom/FINDINGS.md`, `STATUS.md`) generated 23 fixed bugs across 6 months. The lessons that matter for Linux:

1. **No abstraction layers** — doomgeneric was discarded for ID DOOM proper because the abstraction hid memory bugs; Linux must hit raw UPC primitives. (Don't build a "Linux compat shim" — port real Linux.)
2. **PC bounds checking is mandatory** — Doom revealed PC corruption from indirect jumps. Linux has 100K+ functions; a single bad return address spins forever on NOPs. Need halt-on-`PC ≥ ROM_MAP_ENTRIES`.
3. **BSS zero-fill before kernel entry** — every Linux ELF has `.bss`; bootloader phase must zero it. Two-stage boot: minimal stub zeroes kernel BSS, then jumps.
4. **Dynamic file/memory sizes** — WAD_MAX_SIZE hardcoding cost time. Linux files are universal; never hardcode. SYS_FSTAT must return real sizes.
5. **Per-CPU isolation via eBPF per_cpu maps** — Doom is single-context (one MbcCpuState in CPU_MAP at key 0xDE). Linux is multi-process; even single-core Linux switches contexts thousands of times/sec.
6. **Build pipeline artifact integrity** — doom.elf/doom.mbc/doom.rv2mbc must be from same build (build-guide #9). Linux kernel images add `.text`/`.rodata`/`.data`/`.bss`/`.sbss`/`.sdata`/`.init`/`.initdata`/`.exit`/`.tdata`/`.tbss` — every section must load consistently.
7. **Bridge map-read I/O is the bottleneck, not CPU** — Doom's 0.003% XDP utilization at 35 fps tells us the constraint is BPF map I/O. Linux syscall path will hit this on every read/write. Plan for shared-memory mmap'd BPF maps, not syscall-round-trip.

### Outcome target

A real, upstream-traceable Linux 6.x kernel image executes on UPC. From boot, it transitions through: BootParams parse → IVT install → MMU enable (page tables in Wotan extended memory) → start_kernel → init=/sbin/init → busybox shell. From shell, an in-Monad TCP/IP stack (IPv6-only) carries `ping` and `ssh` traffic to a peer UPC instance and to the host. Demo script per `docs/doom/MBC_LINUX_DEMO.md` lines 64-187 with the network demo appended.

### Demo surface — three access modes

The user experience target is **a similar WebSocket window with keyboard passthrough to the Doom port**, plus terminal access. Three independent demo surfaces, each ships at a different phase:

| Mode | Surface | First available | Mechanism |
|------|---------|-----------------|-----------|
| **A: Browser xterm** | WebSocket window in browser, xterm.js renders kernel tty + browser keystrokes flow back | **End of Phase 1** (xv6 shell) | Bridge reads MMIO 0xC001 → WebSocket → xterm.js. Keystrokes → WebSocket → KBD_MAP → kernel. Reuses Doom's `cmd/doom-bridge/main.go` pattern. |
| **B: Direct console** | Operator runs `cmd/upc-bootctl console --instance 0xDE`, gets a tty inline | **End of Phase 1** | Same MMIO 0xC001 path, attached to a host pty instead of WebSocket. |
| **C: SSH over IPv6** | `ssh root@fd00:dead:beef:dada::de` from the host or from another UPC instance | **End of Phase 4** (after TCP works) | Linux dropbear/sshd in initramfs userspace; in-Monad IPv6 carries the SSH session; host's WireGuard ULA `fd00:dead:beef::/48` (per `docs/WIREGUARD-DESIGN-S77.md`) extended with sub-block `fd00:dead:beef:dada::/64` for UPC instances. |

All three modes converge on the same kernel + userspace; they're just different transports to the same `/dev/console` or `/dev/pts/N`.

**IPv6-only is deliberate:** the kingdom's Monad protocol is IPv6-native (per S67 wire-format freeze, IANA registries 2-3, and the WireGuard ULA design). IPv4 inside Monad would be a wire-format mismatch. Linux on UPC = IPv6 from packet zero.

---

## 2. Strategy

### Phased structure with quarterly gates

| Phase | Window | Goal | Gate (ship or defer decision) | Demo modes available | Autonomy |
|-------|--------|------|------------------------------|----------------------|----------|
| **0 — Pre-flight (REVISED 2026-05-08)** | Weeks 1-5 | ABI v1 + ISA v2 frozen per ADR-067; multi-ring + SMP-aware ABI shipped; 5 new opcodes (FENCE/MRET/SRET/LR.W/SC.W) impl+verified | All Phase 0 verifications PASS; verifier budget ≤ 25%; no Doom regression | none yet | Pair call done; impl mostly unattended |
| **1 — L5 xv6-on-MBC** | Weeks 6-11 (Q1 close) | xv6-riscv kernel boots on UPC; shell + ls/cat/echo/uname/ps work | xv6 shell prompt visible in browser; 5 commands respond <50ms | **A** (xterm browser) + **B** (host pty) | Hybrid |
| **2 — L6a uClinux nommu** | Weeks 12-19 (Q2 mid) | uClinux 6.x CONFIG_MMU=n boots; busybox shell + 10 commands | uClinux init reaches `/bin/sh`; busybox `ls /proc` enumerates kernel state | A + B | Hybrid |
| **3 — L6b Full Linux + MMU** | Weeks 20-29 (Q3 close) | Linux 6.x with MMU; musl userspace; multi-process | `/proc/cpuinfo` reads correct fields; two `cat` processes don't trample each other | A + B | Pair (design-heavy) |
| **4 — IPv6 networking + SSH** | Weeks 30-40 (Q4 close) | IPv6-in-Monad: `ping6` + SSH between two UPC instances and from host | `ssh -6 root@fd00:dead:beef:dada::de` from host succeeds | A + B + **C** (SSH over IPv6) | Pair (design-heavy) |

**Revised total campaign: ~10 months** (was 9 pre-pair-call). Phase 0 grew from 2 weeks → 5 weeks because Stevie's 2026-05-08 pair-call chose maximalist multi-ring + SMP-aware ABI + 5 new opcodes (vs. the recommended single-ring + single-CPU + zero new opcodes). See ADR-067 for the seven design decisions and their schedule cost.

**Cut points** are explicit. After each phase gate the user can say "ship as-is, stop here" — and the demo artifact for that phase becomes the deliverable. The plan must produce a usable artifact at every gate, not just the final gate.

### Agent matrix

Each phase has a primary owner skill and supporting skills. The Marshal coordinates; the user remains the final arbiter on scope cuts.

| Skill | Role across the campaign |
|-------|--------------------------|
| **unheaded-marshal** | Drift gate, daily/weekly checkpoint, citation log, scope-creep prevention |
| **unheaded-warmonger** | Phase-level numbered-step expansion (this plan is the seed) |
| **unheaded-architect** | BPF target / verifier-budget / page-table / network-fabric design |
| **unheaded-computermancer** | UPC ISA / MBC ABI / linker-script / RV32I-translator gaps |
| **unheaded-developer** | Implementation + tests + commit cadence |
| **unheaded-blackmage** | Adversarial review of kernel boundary, ABI, syscall dispatcher; fuzz harnesses |
| **unheaded-scientist** | Performance modeling (TLB hit-rate, instruction budget, network throughput) |
| **unheaded-rfceditor** | Boot protocol v2 spec, MBC ISA extensions spec, in-Monad networking spec |
| **unheaded-busboy** | Cross-skill translation when Architect ↔ Developer ↔ Scientist disagree |
| **unheaded-librarian** | docs/doom/ + ADR + wiki ripple after each phase ships |

---

## 3. Phase 0 — Pre-flight (Weeks 1-2)

**Goal:** Validate that everything we think is true actually is true, freeze the ABI before any kernel work, audit the MBC ISA for gaps that real Linux will hit.

**Owner:** Computermancer (lead) + Developer (impl) + Architect (verifier-budget review). **Mostly unattended.**

### 0.1 — Toolchain re-validation (~3 days, unattended)

1. Reproduce a Doom build end-to-end from clean: `make clean && make` in `demos/doom/`. Capture: doom.elf size, doom.mbc instruction count, doom.rv2mbc map size.
2. Run the existing `os_primitives_test.rs` 37-test suite. ALL must pass. Capture timing + memory baseline as reference.
3. Run `bash scripts/bpf-verifier-check.sh` and capture per-program instruction counts as `tmp/bpf-baseline-pre-ascend.txt`. This is the regression line for every subsequent phase.
4. Smoke-load monad-cpu-ebpf into kernel with bpftool against running kernel; confirm program loads and attaches.
5. Run a clean Doom session in browser. Confirm 35 fps internal, E1M1 starts. Document any drift since the last play-through.

### 0.2 — MBC ISA gap audit (~4 days, hybrid)

Linux uses operations xv6 and Doom didn't fully exercise. We need to know NOW what's missing.

6. Inventory atomic ops: `LOCK`, `LR/SC` (load-reserved/store-conditional), `CAS`. Linux uses these in spinlocks, refcounts, RCU. Today MBC has none. **Decision needed (pair):** add `LR.W` (0x3A) + `SC.W` (0x3B) to MBC ISA, or emulate via interrupt-disable critical sections at translator level?
7. Inventory memory barriers: `FENCE`, `FENCE.I`. Required for SMP and self-modifying code paths. Today MBC is single-core; can stub as no-ops, but must reserve opcodes (0x3C, 0x3D).
8. Inventory CSR access: `CSRRW`, `CSRRS`, `CSRRC`, `CSRRWI`, `CSRRSI`, `CSRRCI`. Linux's `start_kernel` reads SATP, MTVEC, MSTATUS, MEPC, MCAUSE, MTVAL, MIP, MIE. **Decision needed (pair):** which CSRs do we expose? Plan to map CSRs into reserved RAM region (e.g., 0x000_F000) and translate `CSRR*` to load/store.
9. Inventory privilege transitions: `MRET`, `SRET`. Linux distinguishes M-mode / S-mode / U-mode. Today MBC has no privilege concept. **Design decision needed (pair, FAR-reaching):** add a `priv_level` field to MbcCpuState (0=M, 1=S, 3=U), or run Linux entirely in a single privilege ring? The simpler choice has implications for ELF loader, syscall dispatcher, and MMU-translation enable.
10. Inventory floating-point: Doom uses soft-float (`gcc_runtime.c`). Linux kernel is integer-only by design (CONFIG_FP_DISABLED) but glibc userspace uses FP. **Decision needed (pair):** ship soft-float-only userspace (busybox+uClibc compiled with `-msoft-float`), or add F-extension MBC ops (~30 ops, big budget hit)?
11. Document all decisions in `docs/protocol/mbc-isa-reference.md` v2.

### 0.3 — Boot protocol v2 spec (~3 days, hybrid → pair on the design call)

The current boot protocol (UPC_BOOT_PROTOCOL.md v1) loads at 0x10000, expects pre-zeroed memory. Real Linux needs:

12. Two-stage boot: stage 1 stub at 0x10000 zeroes BSS, copies sections from initramfs region, jumps to stage 2 at kernel `_start`.
13. BootParams v2 fields: add `BssStart`, `BssEnd`, `Initrd2Addr` (multi-initrd), `CmdLineArgsPtr`, `BootRandomSeed`. **Decision needed (pair):** pad to 256 bytes for forward-compat or freeze at exact 88 bytes?
14. PC bounds checking: instruction `HALT_BOUNDS` (opcode 0xFE) — emit when PC exceeds `ROM_MAP_ENTRIES`; eBPF detects via verifier already, but a kernel-side handler can emit a useful debug message before halting.
15. Calling-convention freeze: documented in `docs/doom/UPC_OS_PRIMITIVES.md` line 89 informally; freeze formally as ADR-067.
16. Multi-CPU support: `BootParams.NumCPUs` exists but is hardcoded 1. **Decision needed (pair, far-reaching):** declare single-CPU as the kingdom forever, or design now for SMP? SMP would change BPF map keying (CPU_MAP becomes per_cpu), context switch path, scheduler. Recommend single-CPU for Phases 1-3, defer SMP to Phase 4 if it becomes necessary for networking.

### 0.4 — Phase 0 verification gate

17. ALL bullet 1-5 outputs match prior baseline. ZERO regressions.
18. ABI freeze ADR landed (ADR-067 — "MBC ISA v2 + UPC ABI v1"). Pair-call held with Stevie + Computermancer + Architect; decisions 6-11 + 13-16 recorded.
19. `tmp/bpf-baseline-pre-ascend.txt` committed.
20. Single commit: `feat(upc): Phase 0 pre-flight — ABI freeze + ISA gap audit (closes ADR-067)`.

**Cut point check:** if Phase 0 reveals the MBC ISA needs >5 new opcodes that push verifier budget past 25%, STOP — convene Round Table on whether to fork the program into two BPF programs (one per phase of execution).

**🎖️ Marshal handoff → Phase 1:** when 17-20 are green, invoke `/unheaded-marshal` with the prompt *"Phase 0 ASCEND-LINUX complete. Verifier budget at <X>%. ABI v1 + ISA v2 frozen per ADR-067. Advance to Phase 1 — vendor xv6-riscv into `crates/xv6-mbc/upstream/`, port kernel/start.c to MBC, owners Developer + Computermancer."* Marshal opens the Phase 1 work queue and dispatches the first Developer agent.

---

## 4. Phase 1 — L5 xv6-riscv on MBC (Weeks 3-8, Q1 close)

**Goal:** xv6-riscv kernel boots on UPC. Shell prompt visible. 5 commands (`ls`, `cat`, `echo`, `uname`, `ps`) respond in <50 ms in browser.

**Status:** Phase 1.1 SHIP gate ACHIEVED 2026-05-10 (commit `3ac1f684`). xv6 emits `xv6 booting...\n` on kernel 6.17.0-23 with `--features ascend-linux`. CPU advances 4000 insns, M→S transition (priv 0→1) verified. See `references/marshal-shift-2026-05-10-phase11-banner-shipped.md` for the full shift report. Phase 1.2-1.5 (page tables, process model, filesystem, shell+5cmds) remain — the headline gate "shell prompt visible" is the Phase 1 final gate per §1.6, not Phase 1.1.

**Owner:** Developer (lead) + Computermancer (ABI / linker) + Architect (verifier guard) + Marshal (drift). **Hybrid: mechanical work unattended, kernel-choice + page-table model are pair calls.**

### 1.1 — xv6 source bring-up (~5 days, unattended) — ✅ DONE 2026-05-10

21. Vendor xv6-riscv from MIT-PDOS upstream (commit pinned, MIT license verified) into `crates/xv6-mbc/upstream/`.
22. Add a top-level `crates/xv6-mbc/Cargo.toml` workspace member; treat the C source as a build target, not a Rust library.
23. Adapt xv6's `Makefile` to use the `riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32` toolchain (same as Doom).
24. Replace `kernel/start.c` (xv6's M-mode boot) with an MBC-native `start_mbc.c` that uses the `BootParams` structure.
25. Replace xv6's UART driver (`kernel/uart.c`) with a console driver that writes to MMIO 0xC001 (per UPC_OS_PRIMITIVES.md L4f).
26. Replace xv6's `kernel/virtio_disk.c` with a ramdisk driver using SYS_READ_BLOCK / SYS_WRITE_BLOCK syscalls (or direct block-device mmap).
27. First end-of-section commit gate: `cargo run --bin doom-runner -- --kernel xv6-mbc.mbc` should print "xv6 booting..." and HLT (no shell yet).

**§1.1 SHIP NOTES (2026-05-10 — paths taken vs paths not taken):**

- **Loader path used = doom-runner replaced by `cmd/upc-bootctl`.** §1.1 originally targeted doom-runner as the loader; in practice cmd/upc-bootctl emerged in §1.5/Phase 1 SHIP plan and became the canonical Phase 1.1 loader. Closes tasks #51, #62.
- **Translator .rodata path: loader-mirror, NOT translator-MBC-relative-loads.** The original §1.1 plan implied the translator would emit MBC-relative immediates for .rodata. The simpler path landed: rv32i_to_mbc emits a `.data` sibling artifact (TLV: count + records of `[byte_addr, len, bytes]`) and cmd/upc-bootctl bulk-writes to RAM_MAP at byte-VMA addresses via `runner.load_data_image`. This avoided the multi-day translator redesign that earlier estimates flagged. Closes task #61.
- **BPF verifier rejection on kernel 6.17 → ROOT CAUSE WAS MRET/SRET handlers.** A stray `continue;` skipped `i += 1` in the dispatch loop, plus an unmasked-wide-range PC write tripped state-pruning. Fix in `0e30b55b`: drop `continue`, mask PC `& 0xFFFF`. Closes task #70. BlackMage adversarial review: SAFE — fix actually closes a counter-bypass primitive.
- **BOOT_MAGIC byte-order ambiguity → ADR-072.** Resolved with markdown-only doc edits: canonical hex `0x554E4844` ('UNHD' MSB-first); wire bytes `D,H,N,U` per LE. Same pattern as ELF magic. Closes task #64.
- **x16+ register usage → STRIPPED FROM swtch + trampoline.** Audit on linked kernel.elf showed all 40 x16+ usages were s2-s11 saves/restores in context-switch path. CFLAGS already pin `-ffixed-x18..x27` so C codegen never assigned to them; the asm saves were dead work AND introduced unsupported-register translator errors. Stripped in commit `5c55c386`. Closes task #60.
- **MMIO TTY 2-null-pad cosmetic fixed.** start_mbc.c::mmio_putc switched from unaligned `volatile uint32 *` to `volatile uint8 *`. Banner output drops 45 → 15 bytes. Commit `3f14a768`.
- **Phase 2 stage-1 stub crate scaffolded** (`crates/upc-bootstub/`) per the SHIP-decision deferral; cmd/upc-bootctl gained a `--bootstub <path>` flag for the prepended-stub boot path. Closes task #63.

### 1.2 — Page tables under MMU (~7 days, pair on day 1, then unattended)

28. **Pair call:** xv6 expects RISC-V Sv32 page tables (10+10+12 bit, 4 KB pages). Our L4d MMU emulates the same shape. Confirm that ADR-023's TLB design IS Sv32-compatible.
29. Adapt xv6's `kernel/vm.c` page-table walker to call our SYS_SET_PAGE_DIR (250) and SYS_FLUSH_TLB (252) syscalls, OR direct CSR writes if Phase 0 chose CSR translation.
30. Test: a dummy user process that mmap's a page, writes 0xDEADBEEF, reads it back via the kernel's translation path. Must round-trip.
31. Test: a user process that page-faults on an unmapped address — the kernel's fault handler at IVT 0x0F must execute and either kill the process or grow the heap (xv6 default: kill).
32. Test: TLB shootdown — a context switch flushes the TLB; the next memory access by the new process MUST refill from the new page directory.

### 1.3 — Process model (~5 days, mostly unattended)

33. Adapt xv6's `proc.c` to use the L4c scheduler we already shipped. Replace xv6's hand-rolled scheduler with calls to SYS_FORK (2), SYS_EXECVE (11), SYS_WAITPID (7), SYS_EXIT (1), SYS_SCHED_YIELD (158).
34. Process table: bridge xv6's `struct proc` to our 4-slot process table in CPU_MAP. **Decision (pair, mid-phase):** keep 4 slots or grow CPU_MAP to 16? Grow only if xv6 shell + 5 commands actually need it.
35. Trapframe: xv6's `kernel/trampoline.S` does the M-mode → S-mode → U-mode trap. We collapse to "save MbcCpuState, jump to handler, IRET restores."
36. Implement `init` process: a static-linked C program that opens `/dev/console`, prints "init starting", forks `sh`, waits.

### 1.4 — Filesystem + ramdisk (~5 days, unattended)

37. xv6 uses its own filesystem. We pre-build a ramdisk image with `mkfs` (xv6's tool), pack it into the boot artifact at 0x800000.
38. Adapt xv6's `kernel/fs.c` to read/write blocks via SYS_READ_BLOCK/WRITE_BLOCK against the ramdisk region.
39. Pre-populate the ramdisk with: `/init`, `/bin/sh`, `/bin/ls`, `/bin/cat`, `/bin/echo`, `/bin/uname`, `/bin/ps`, `/dev/console`.
40. Test: kernel mounts ramdisk → init runs → init forks sh → sh reads from console.

### 1.5 — Shell + 5 commands + WebSocket xterm bridge (~7 days, unattended)

41. Compile xv6's bundled `sh.c` for MBC. Statically linked, no glibc.
42. Compile `ls`, `cat`, `echo`, `uname`, `ps` from xv6's userspace tree. Verify each is <16 KB compiled.
43. Wire console input: browser keyboard → KBD_MAP → xv6's `console.c` read path → sh's REPL.
44. Wire console output: sh's writes → SYS_WRITE → MMIO 0xC001 → Busboy `compute.tty.{label}` topic → bridge → browser.
44a. **New `cmd/upc-tty-bridge/`** — WebSocket bridge for terminal mode (Mode A demo surface). Pattern after `cmd/doom-bridge/main.go` but framing tty bytes instead of framebuffer pixels. Browser endpoint at the kingdom's user-app port (allocate from `pkg/ports/ports.go` user-app band 26000-26666; recommend 26100 = `upc-tty-bridge`).
44b. **Browser xterm.js page** in `dashboard/upc-console.html` — single-file vanilla JS + xterm.js (vendored via `dashboard/vendor/xterm/`). Connects to `ws://localhost:26100/console/<instance>`, renders bytes, sends keystrokes back.
44c. **`cmd/upc-bootctl console --instance 0xDE`** — direct-console mode (Mode B). Attaches the operator's pty to the same MMIO 0xC001 stream so `ssh govan@west cmd/upc-bootctl console` works from any host. Required: `golang.org/x/term` for raw-mode TTY handling.

### 1.6 — Phase 1 verification gate (Q1 ship-or-defer call)

45. **Mode A**: Browser xterm shows xv6 shell prompt within 1 second of boot. Keyboard input renders. Backspace/arrows work.
46. **Mode B**: `cmd/upc-bootctl console --instance 0xDE` from a host shell drops the operator into the same xv6 prompt.
47. `ls /` lists ramdisk contents.
48. `cat /proc/version` (xv6 stub returns "xv6-mbc-0.1") works (or substitute with `uname -a`).
49. `echo hello | cat` round-trips through a pipe.
50. `ps` lists at least 2 processes (init, sh).
51. All 5 respond <50 ms in browser.
52. BPF verifier-check still passes; instruction count delta vs Phase 0 baseline ≤ 30%.
53. Single commit chain (one per sub-phase 1.1 → 1.5) merged.
54. Ship gate: **ADR-068 "L5 xv6-on-MBC ships"** authored + `docs/doom/MBC_LINUX_DEMO.md` updated with the L5 demo script.

**Cut point:** if at week 6 we don't have a shell yet, drop Phase 1.5 (commands 42-44) and ship "kernel boots, prints hello, HLTs" as the L5 deliverable. Defer shell to Phase 2's uClinux build.

**🎖️ Marshal handoff → Phase 2:** invoke `/unheaded-marshal` with *"Phase 1 ASCEND-LINUX complete. xv6 shell live with Mode A (browser xterm) + Mode B (host pty). Verifier budget at <X>%. Advance to Phase 2 — vendor Linux 6.x LTS into `crates/uclinux-mbc/upstream/`, create arch/mbc by copying arch/h8300, owner Architect + Developer."* Marshal scopes Phase 2.1 (kernel source bring-up) as the next dispatch.

---

## 5. Phase 2 — L6a uClinux nommu (Weeks 9-16, Q2 mid)

**Goal:** uClinux 6.x with `CONFIG_MMU=n` boots on UPC. Busybox shell + 10 commands. `ls /proc` enumerates real kernel state.

**Owner:** Architect (kernel-build / Kconfig) + Developer (arch/mbc port) + Computermancer (ABI bridge) + BlackMage (review of kernel-userspace boundary). **Hybrid: arch/mbc port unattended, Kconfig & sched-class decisions are pair calls.**

### 2.1 — Kernel source bring-up (~7 days, hybrid)

54. Vendor Linux 6.x stable (LTS — pinned to 6.6 LTS or 6.12 LTS, GPL-2.0; Stevie picks during ABI freeze in 0.4) into `crates/uclinux-mbc/upstream/`.
55. Create `arch/mbc/` directory under upstream. Files needed: `Kconfig`, `Makefile`, `boot/`, `kernel/`, `mm/`, `include/asm-mbc/`.
56. **Pair call (week 1):** which existing arch is closest to MBC for cargo-culting? Best candidates: `arch/h8300` (no-MMU, simple), `arch/microblaze` (RV32-adjacent), `arch/riscv` (closest ISA, but full MMU). Recommend: start by copying `arch/h8300` headers and patching, then layer MMU back when Phase 3 starts.
57. Implement `arch/mbc/kernel/head.S` — kernel entry; reads BootParams; sets up initial stack; calls `start_kernel`.
58. Implement `arch/mbc/kernel/setup.c` — `setup_arch()`, populates `boot_params`, registers console.
59. Implement `arch/mbc/kernel/process.c` — `task_struct` adapter, context switch using our L4c scheduler bones.
60. Implement `arch/mbc/kernel/irq.c` — IVT bridge to the kernel's IRQ subsystem.

### 2.2 — Drivers (~5 days, unattended)

61. Console driver `arch/mbc/drivers/serial-mmio.c` — registers as `/dev/ttyMBC0`; uses MMIO 0xC001-0xC003.
62. Block device `arch/mbc/drivers/blk-ramdisk.c` — registers as `/dev/ram0`; uses SYS_READ_BLOCK / SYS_WRITE_BLOCK.
63. RTC stub `arch/mbc/drivers/rtc-stub.c` — returns BootParams.TickRateHz × ticks since boot for `time(2)` syscall.
64. Initramfs driver: kernel mounts CPIO archive at boot from `BootParams.RamdiskAddr`. Standard kernel feature; just need to populate the CPIO at build time.

### 2.3 — userspace build (~5 days, unattended)

65. Vendor busybox 1.36 (GPL-2.0). Cross-compile for MBC with `-msoft-float -fno-common -static`.
66. Build a minimal initramfs:
    - `/init` — busybox shell as PID 1
    - `/bin/sh`, `/bin/busybox` (provides ls/cat/echo/cp/mv/mkdir/ps/uname/uptime/date/sleep/ash via symlinks)
    - `/dev/console`, `/dev/null`, `/dev/zero`
    - `/proc` (mountpoint), `/sys` (mountpoint)
67. Pack the initramfs as CPIO + gzip. Compress to <500 KB to fit alongside kernel.
68. Build artifact: `vmlinux.mbc` + `rootfs.cpio.gz` packaged into a single bootable image.

### 2.4 — Boot integration (~3 days, hybrid on integration call)

69. Update `pkg/upc/boot.go` to package vmlinux + initramfs into a single load step.
70. Wire `cmd/zhen-agentd` (or new `cmd/upc-bootctl`) to dispatch a boot command via the existing UPC trigger packet path (Gjallarhorn).
71. End-to-end smoke: `cmd/upc-bootctl boot --kernel vmlinux.mbc --initramfs rootfs.cpio.gz`. Expect: kernel prints `Linux version 6.x ...` to the bridge tty stream within 5 seconds.

### 2.5 — Phase 2 verification gate (Q2 mid ship-or-defer call)

72. Browser shows busybox shell prompt within 5 seconds of boot.
73. `ls /proc` enumerates: `1`, `2`, `cpuinfo`, `meminfo`, `version`, `uptime`, `mounts`.
74. `cat /proc/cpuinfo` returns at least: `processor: 0`, `model name: UPC MBC`, `bogomips: <real number>`.
75. `cat /proc/version` returns the actual Linux version string.
76. `dmesg` shows the boot trace including IVT install, page-table init (no-op for nommu), console registration, ramdisk mount.
77. `busybox --list` shows the 10 commands; each one runs and exits without panic.
78. BPF verifier-check passes. Instruction count delta vs Phase 1 ≤ 30%.
79. Ship gate: **ADR-069 "L6a uClinux ships"** authored. Demo script appended to MBC_LINUX_DEMO.md.

**Cut point:** if at week 14 we don't have busybox booting, ship "Linux kernel prints version + halts cleanly" as the L6a deliverable. Defer busybox shell to Phase 3 alongside the MMU work.

**🎖️ Marshal handoff → Phase 3:** invoke `/unheaded-marshal` with *"Phase 2 ASCEND-LINUX complete. uClinux + busybox shell live. /proc enumerates. Verifier budget at <X>%. Advance to Phase 3 — flip CONFIG_MMU=y, port arch/mbc/mm/init.c, fork-safety stress test is the hard gate. Owner Architect + Developer + BlackMage; PAIR-HEAVY phase."* Marshal pauses for the Stevie pair-call before scheduling 3.1 design work.

---

## 6. Phase 3 — L6b Full Linux + MMU (Weeks 17-26, Q3 close)

**Goal:** Linux 6.x with `CONFIG_MMU=y` boots on UPC. glibc userspace works (statically-linked first, dynamic later). Two `cat` processes coexist without page-table corruption.

**Owner:** Architect (page-table model) + Developer (impl) + Scientist (perf modeling) + Computermancer (CSR / privilege bridge) + BlackMage (security review of MMU isolation). **Pair-heavy: every architectural decision in this phase is a Stevie call.**

### 3.1 — MMU enablement (~10 days, pair-heavy)

80. Re-enable Phase 0's reserved Sv32 page-table opcodes (or CSR translation, per 0.2 decision 8).
81. Implement `arch/mbc/include/asm/pgtable.h` — pgd_t, pmd_t, pte_t types matching our L4d MMU layout.
82. Implement `arch/mbc/mm/init.c` — kernel initial page table; identity-maps the kernel image; enables MMU before jumping into `start_kernel`'s late init.
83. **Pair design call:** how do we hand userspace processes their own page directories? Options: (a) per-task pgd allocated on fork, (b) shared pgd with ASID separation, (c) software-tagged pgd via MBC `priv_level` byte. Recommend (a) for compatibility with stock Linux mm/ code.
84. Implement `arch/mbc/kernel/syscall.c` MMU-aware path — copy_to_user / copy_from_user respecting page tables; SIGSEGV on bad accesses.
85. Stress test: forkbomb harness runs 16 processes, each touching its own pages. No cross-process memory leaks. **Hard gate** — if this fails, the entire Linux-on-UPC promise fails. Phase 3 stops until fixed.

### 3.2 — glibc + dynamic linking (~10 days, hybrid)

86. **Pair call:** glibc vs musl vs uClibc-ng for full Linux. glibc is canonical but heaviest; musl is common for minimal Linux (Alpine); uClibc-ng is the uClinux successor. Recommend musl for binary size + license compatibility (MIT vs glibc's LGPL).
87. Cross-compile musl 1.2.x for MBC. Soft-float, static-linked dynamic-linker stub.
88. Re-build busybox against musl. Re-build a `coreutils` static binary (just `ls`, `cat`, `echo`, `cp`, `mv` for now).
89. Test: dynamic-linker stub loads `/lib/ld-musl-mbc.so.1` from initramfs, resolves `printf` symbol from `/lib/libc.so`, runs `puts("hello dynamic")`.
90. Defer ELF interpreter (PT_INTERP) handling to Phase 4 if dynamic linking proves expensive — Phase 3 can ship with static-only userspace.

### 3.3 — Multi-process correctness (~5 days, mostly unattended)

91. Two-process test: `cat /proc/cpuinfo & cat /proc/uptime &`. Both must complete without corrupting each other's working sets.
92. Signal handling: SIGTERM kills a process cleanly; SIGSEGV from page-fault path produces `segfault at 0xDEADBEEF` to dmesg without taking the kernel down.
93. fork() + exec() round-trip: shell forks, child execs `/bin/echo hello`, parent wait()s, exit code propagates. End-to-end timing <100 ms.
94. /proc enumeration at scale: `ls /proc/*/status` for 8 forked processes returns 8 valid status files, each with correct `Pid:`/`PPid:`/`State:`.

### 3.4 — Performance (~5 days, Scientist-led)

95. TLB hit-rate measurement under realistic workload (busybox find / -print). **Hypothesis:** ADR-023 predicted 80-90% hit rate; need real number.
96. Syscall round-trip cost: measure SYS_GETPID 1000 times, divide. Compare with ADR-023's 100-200 ns prediction.
97. Bridge throughput: how many MBC instructions/sec when the bridge is reading dmesg vs idle. Identify worst-case I/O bottleneck.
98. Optimize the top 1 hotspot identified in 95-97. **Architect call:** add caching layer or accept the perf cost as the cost of Linux-on-eBPF?
99. Document findings in `docs/doom/MBC_LINUX_PERF.md` (new).

### 3.5 — Phase 3 verification gate (Q3 close ship-or-defer call)

100. `uname -a` returns the actual Linux kernel version string with arch=mbc.
101. Two concurrent shell sessions, each forking a child, all four processes coexist without corruption.
102. `cat /proc/$$/maps` for any process returns its actual memory map with kernel/user separation visible.
103. ELF static binary loaded from initramfs runs to completion; ELF dynamic binary (if 86-90 shipped) ALSO runs.
104. BPF verifier-check passes. Instruction count delta vs Phase 2 ≤ 50% (this phase is the biggest hit because every memory access goes through translation).
105. Ship gate: **ADR-070 "L6b Full Linux + MMU ships"** authored. PERF doc landed.

**Cut point:** if at week 24 dynamic linking isn't working, ship static-only and defer dynamic to a Phase 5 follow-up. The static-userspace deliverable is still a real Linux on UPC.

**🎖️ Marshal handoff → Phase 4:** invoke `/unheaded-marshal` with *"Phase 3 ASCEND-LINUX complete. Full Linux + MMU live. multi-process verified. TLB hit rate <X>%. Verifier budget at <Y>%. Advance to Phase 4 — IPv6 networking inception. Owners Architect + RFC Editor + Developer + BlackMage; PAIR-HEAVY (netdev model + ND analog are Stevie design calls)."* Marshal pauses for two pair-calls (netdev model 4.1, ND analog 4.2) before dispatching 4.3 hardware setup.

---

## 7. Phase 4 — Networking inception: TCP/IP-in-Monad (Weeks 27-36, Q4 close)

**Goal:** Two UPC instances exchange real Linux ICMPv6 packets. Linux's TCP/IP stack on instance A produces an IPv6 packet → carried inside a Monad packet over the BPF-XDP fabric → instance B's Linux IPv6 stack receives it. `ping6` works between two UPC nodes. SSH login works from the host into a UPC instance over the WireGuard ULA `fd00:dead:beef:dada::/64` block reserved for UPC. **IPv6-only — no IPv4 in this kingdom.**

**Owner:** Architect (network fabric design) + RFC Editor (in-Monad protocol spec) + Developer (impl) + Scientist (throughput modeling) + BlackMage (network security review). **Pair-heavy: this phase has the most architectural unknowns.**

### 4.1 — Network device design (~5 days, pair-heavy)

106. **Pair call:** how does Linux see a network interface on UPC? Options: (a) virtio-net emulated — most compatible, heaviest; (b) custom mbc-net device with simpler ring buffer — pragmatic; (c) tap device exposed as /dev/tap0 — easiest to implement. Recommend (b) — implement `arch/mbc/drivers/net-mbc.c` that registers as `mbc0` netdev with TX/RX rings in shared BPF maps.
107. Define MTU: 1280 (the IPv6 minimum MTU; ensures every packet fits without fragmentation given Monad header overhead).
108. Define ring sizes: TX 64 entries, RX 64 entries, each 1500 bytes (gives headroom on top of MTU).
108a. Disable IPv4 entirely in the Linux kernel build via `CONFIG_INET=y CONFIG_IPV6=y CONFIG_IP_PNP=n`. Userspace `iputils-ping6` only.

### 4.2 — In-Monad protocol spec (~5 days, RFC Editor lead, pair on review)

109. Author `docs/specs/monad-net-encap-00.md`: **IPv6-over-Monad** encapsulation. Each Linux IPv6 packet is wrapped in a Monad packet with `flags: T (Transport)`, `flow_label: <SipHash of {src_ip, dst_ip, src_port, dst_port, next_header}>`, payload is the raw IPv6 packet (no magic prefix needed — IPv6 version field is unambiguous in byte 0).
110. **Pair call:** ND (IPv6 Neighbor Discovery, the IPv6 ARP equivalent) — how do UPC instances discover each other's link-layer address? Options: (a) static config in BootParams; (b) dynamic via Wotan service discovery using existing `pkg/discovery`; (c) reuse Gjallarhorn UPC trigger packets for periodic NA (Neighbor Advertisement) announcements. Recommend (b) for production realism, with (c) as fallback for fresh-boot when discovery is empty.
111. **IPv6 address plan:** UPC instances live in `fd00:dead:beef:dada::/64` (carved from the existing kingdom WireGuard ULA `fd00:dead:beef::/48` per `docs/WIREGUARD-DESIGN-S77.md`). Each UPC instance's instance-byte (e.g. 0xDE, 0xEA, 0xBE, 0xEF) becomes the low byte of its address: `fd00:dead:beef:dada::de`, `fd00:dead:beef:dada::ea`, etc.
112. Host-side bridging: WEST's `wg0` interface gets a route `fd00:dead:beef:dada::/64 dev mbc-bridge` so the host can SSH directly into UPC instances over the WG ULA.
113. Document in spec: how Linux's standard IPv6 networking stack is preserved unmodified — only the L2 device driver and the route to the host changes.

### 4.3 — Two-node setup + test fabric (~7 days, hybrid)

114. Spin up 2 UPC instances on WEST (or one each on WEST+EAST). Each runs the same vmlinux.mbc + initramfs with a 1-line difference in `/etc/mbc-net.conf` (instance 0xDE / 0xEA).
115. Configure IPs: `fd00:dead:beef:dada::de/64` on instance 0xDE, `fd00:dead:beef:dada::ea/64` on instance 0xEA.
116. Smoke test: `ip -6 addr show mbc0` on each instance must show its IPv6 address.
117. ND test: instance 0xDE `ndsend fd00:dead:beef:dada::ea` should resolve via the Wotan-discovery path; subsequent packets reach instance 0xEA without re-discovery.

### 4.4 — `ping6` traffic (~5 days, unattended once 4.3 passes)

118. From instance 0xDE shell: `ping6 -c 5 fd00:dead:beef:dada::ea`. EXPECTED: 5 ICMPv6 echo requests carried inside Monad packets on the BPF fabric, instance 0xEA's Linux kernel responds with ICMPv6 echo replies, all 5 round-trip with reasonable latency.
119. From the host (WEST): `ping6 -c 5 fd00:dead:beef:dada::de` should also succeed (host-to-UPC bridging via WG ULA).
120. Capture a Monad-net packet via the existing trace-collector. Verify the encapsulation: `[Monad header 20B][IPv6 header 40B][ICMPv6 header 8B][ICMPv6 payload 32B]` = 100 bytes total per ping.
121. Measure RTT: Architect+Scientist target = <100 ms p99 over the BPF fabric.

### 4.5 — TCP + SSH (Mode C demo surface) (~7 days, hybrid)

122. Three-way handshake test: `nc -l -6 8080` on instance 0xEA, `echo hello | nc -6 fd00:dead:beef:dada::ea 8080` from instance 0xDE. Connection establishes, payload transfers, FIN closes.
123. **dropbear sshd** in initramfs userspace. Vendor dropbear 2024.x (MIT-like license). Cross-compile with musl, listen on `[::]:22`. Generate host key on first boot via the `BootParams.BootRandomSeed` field added in Phase 0 decision 13.
124. Authorized-keys preload: at initramfs build time, embed `/etc/dropbear/authorized_keys` with Stevie's public key (sourced from `~/.ssh/id_ed25519.pub` on the build host, configurable via build-time env var).
125. From the host: `ssh -6 root@fd00:dead:beef:dada::de` — must drop into a real Linux shell on the UPC instance. **This is the headline demo.**
126. From instance 0xDE: `ssh -6 root@fd00:dead:beef:dada::ea` — UPC-to-UPC SSH must also work.

### 4.6 — Phase 4 verification gate (Q4 close ship-or-defer call)

127. `ping6 -c 5 fd00:dead:beef:dada::ea` between two UPC nodes succeeds.
128. `ping6 -c 5 fd00:dead:beef:dada::de` from the host succeeds (Mode C precondition).
129. `ip -6 -s link show mbc0` shows non-zero RX/TX bytes after pinging.
130. Monad packet capture confirms IPv6-in-Monad encapsulation per spec.
131. TCP three-way handshake completes (or formally cut to ship-as-Phase-4 with ICMPv6-only).
132. **Mode C SSH demo**: `ssh -6 root@fd00:dead:beef:dada::de` from the host successfully logs into a Linux shell. `uname -a`, `ls /`, `ps` all work over the SSH session.
133. **All three demo modes are functional simultaneously** — Browser xterm (Mode A) + direct console (Mode B) + SSH over IPv6 (Mode C). `MBC_LINUX_DEMO.md` updated with all three scenes.
134. Ship gate: **ADR-071 "L6 Full Linux + IPv6 Networking + SSH ships"** authored.

**🎖️ Marshal handoff → Phase 5 (stretch / wind-down):** invoke `/unheaded-marshal` with *"Phase 4 ASCEND-LINUX complete. Linux on UPC ships. All three demo surfaces (A browser, B host pty, C SSH over IPv6) live. ADR-071 authored. Decide Phase 5 disposition: (a) wind down — campaign deliverable shipped; (b) Phase 5 stretch — multi-process scaling, dynamic linking polish, conference-talk artifact for `docs/doom/MBC_LINUX_DEMO.md` final cut. Stevie's call."* Marshal opens a Round Table for the wind-down vs. Phase 5 decision.

**Cut points:**
- If at week 32 ICMPv6 works but TCP doesn't, ship ICMPv6+ping6-only — that's still a real Linux IPv6 stack carried inside Monad. TCP+SSH (Mode C) defers to a Phase 5.
- If at week 34 TCP works but dropbear won't compile for MBC+musl, ship TCP+`nc` instead of SSH — Mode C becomes "raw netcat shell" instead of full SSH. Mode A and Mode B both already work; the demo still has two compelling surfaces.
- If at week 36 dropbear works but host bridging doesn't (WG-ULA route to mbc-bridge fails), ship UPC-to-UPC SSH only — still demonstrates real SSH inside Monad, just not from the host.

---

## 8. Risk register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| MBC ISA needs >5 new opcodes pushing verifier > 25% | Medium | High (verifier rejection halts everything) | Phase 0 audit; if exceeds, fork program into per-phase BPF programs |
| Real Linux ELF image > ROM_MAP capacity (1 MB) | High | High (won't fit) | Multi-segment loader; kernel image goes to RAM_MAP, not ROM_MAP — design in Phase 2.1 |
| TLB hit rate < 60% (predicted 80-90%) | Medium | High (perf cliff) | Scientist-led perf modeling in Phase 3.4; if cliff, increase TLB to 256 entries (BPF map resize) |
| Bridge map-read I/O bottleneck makes shell unusable | High | Medium (degrades demo) | Phase 1.5 measures real latency; if >50ms, switch to mmap'd shared-memory path |
| Atomic ops emulation breaks under SMP-correctness assumptions in mainline kernel code | Low (single-CPU) | Medium | Phase 0 decision 6 + 16: stay single-CPU through Phase 4; SMP is post-launch |
| musl+busybox build fails for MBC target due to non-standard sigsetjmp / context switching | Medium | Medium | Pre-flight in 2.3 — if musl breaks, fall back to uClibc-ng |
| In-Monad networking ND (IPv6 Neighbor Discovery) has no good analog | Medium | Medium | Phase 4.2 decision 110 — use Wotan service discovery to short-circuit ND; Gjallarhorn UPC packets as fallback |
| dropbear won't compile against musl for the MBC target (TTY/pty deps) | Medium | Medium | Cut to raw `nc` Mode C; documented at Phase 4.6 cut points |
| Host WG ULA bridging fails (no route from `wg0` to `mbc-bridge`) | Low | Medium | Cut to UPC-to-UPC SSH only; host can still reach via `cmd/upc-bootctl console` Mode B |
| xterm.js bundle bloats `dashboard/` — license / size review | Low | Low | xterm.js is MIT, ~600KB minified; vendor under `dashboard/vendor/xterm/` and add to THIRD_PARTY.md |
| `feedback_resource_awareness.md` violation: weeks of compute on a phase that won't ship | Low | High | Quarterly gates are explicit ship-or-defer points; cut points listed per phase |
| Scope creep into Yggdrasil / hardened-Debian work | Low (clear separation in ADR-69420) | Low | Plan calls this out in §1; Marshal cites if it drifts |

---

## 9. Critical files (paths to be modified)

### Created in Phase 0
- `docs/protocol/mbc-isa-reference.md` v2 (extended)
- `docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md` (new)
- `docs/doom/UPC_BOOT_PROTOCOL.md` v2 (extended)
- `tmp/bpf-baseline-pre-ascend.txt` (regression baseline)

### Created in Phase 1
- `crates/xv6-mbc/` (new workspace; vendored xv6-riscv + adapters)
- `crates/xv6-mbc/upstream/kernel/start_mbc.c`, `console-mmio.c`, `blk-ramdisk.c`
- `docs/adr/ADR-068-l5-xv6-on-mbc-ships.md`
- `docs/doom/MBC_LINUX_DEMO.md` (extended with L5 scene)

### Created in Phase 1 (extension for demo surfaces)
- `cmd/upc-tty-bridge/` (new — WebSocket bridge, Doom-style for tty)
- `dashboard/upc-console.html` + `dashboard/vendor/xterm/` (browser xterm.js Mode A)
- `cmd/upc-bootctl/console` subcommand (Mode B direct-pty)

### Created in Phase 2
- `crates/uclinux-mbc/` (new workspace; vendored Linux 6.x + arch/mbc port)
- `crates/uclinux-mbc/upstream/arch/mbc/{Kconfig, Makefile, kernel/, mm/, drivers/, include/asm-mbc/}`
- `cmd/upc-bootctl/` (new — boot dispatcher)
- `docs/adr/ADR-069-l6a-uclinux-ships.md`

### Created in Phase 3
- `crates/uclinux-mbc/upstream/arch/mbc/mm/init.c` (full MMU)
- `docs/doom/MBC_LINUX_PERF.md` (perf measurements)
- `docs/adr/ADR-070-l6b-linux-mmu-ships.md`

### Created in Phase 4
- `crates/uclinux-mbc/upstream/arch/mbc/drivers/net-mbc.c` (mbc0 IPv6-only netdev)
- `docs/specs/monad-net-encap-00.md` (IPv6-in-Monad encap spec)
- Initramfs additions: dropbear sshd binary + `/etc/dropbear/authorized_keys`
- Host config: `wg0` route entry for `fd00:dead:beef:dada::/64 dev mbc-bridge`
- `docs/adr/ADR-071-l6-networking-ships.md`

### Existing files reused (not modified, just consumed)
- `crates/monad-mbc/src/{execute.rs, instruction.rs, translator.rs}` — MBC interpreter
- `ebpf/monad-cpu-ebpf/src/main.rs` — the BPF program that IS the UPC
- `crates/doom-runner/src/{loader.rs, memory.rs}` — ELF loading reused for Linux ELF
- `pkg/upc/boot.go` — extended in Phase 2
- `scripts/bpf-verifier-check.sh` — gate at every phase

---

## 10. Verification — end-to-end after each phase

```bash
# Phase 0 gate
bash scripts/bpf-verifier-check.sh > tmp/bpf-baseline-pre-ascend.txt
diff tmp/bpf-baseline-pre-ascend.txt <(bash scripts/bpf-verifier-check.sh)  # zero diff
cargo test --release --manifest-path crates/monad-mbc/Cargo.toml             # 37/37 pass
cd crates/doom-runner && cargo run -- --kernel doom.mbc                      # E1M1 plays at 35 fps

# Phase 1 gate
cd crates/xv6-mbc && make && cargo run -p doom-runner -- --kernel xv6-mbc.mbc
# Browser at localhost:20103/upc → xv6 prompt → ls / → cat /README → echo hi → uname → ps
# All 5 commands return <50 ms.

# Phase 2 gate
cd crates/uclinux-mbc && make ARCH=mbc && cmd/upc-bootctl boot --kernel vmlinux.mbc --initramfs rootfs.cpio.gz
# Browser → busybox shell prompt
# ls /proc → at least cpuinfo, meminfo, version, uptime
# cat /proc/version → "Linux version 6.x ..."
# busybox --list → 10 commands, each runs without panic

# Phase 3 gate
# Same boot as Phase 2, plus:
cat /proc/$$/maps                                        # shows kernel/user split
(cat /proc/cpuinfo & cat /proc/uptime &) ; wait          # both complete cleanly
sh -c 'for i in 1 2 3 4 5 6 7 8; do echo $i & done; wait' # 8 forked children
ls /proc/*/status | wc -l                                # at least 10

# Phase 4 gate (two-node setup on WEST)
ssh govan@east cmd/upc-bootctl boot --kernel vmlinux.mbc --instance 0xEA &
cmd/upc-bootctl boot --kernel vmlinux.mbc --instance 0xDE &
# Mode A: browser → http://localhost:26100/console/0xDE → xterm shell
# Mode B: cmd/upc-bootctl console --instance 0xDE → host pty shell
# Mode C: ssh -6 root@fd00:dead:beef:dada::de → real SSH login
# From instance 0xDE shell: ping6 -c 5 fd00:dead:beef:dada::ea → 5/5 received, RTT visible
# From host: ping6 -c 5 fd00:dead:beef:dada::de → 5/5 received (host bridging works)
# tcpdump on the bridge: shows Monad-encapsulated IPv6 ICMPv6

# Verify no regressions at every phase
go test -short -count=1 -timeout 120s ./...                # 219+ packages pass
cargo audit --workspace                                    # zero new advisories
~/go/bin/govulncheck ./...                                 # zero kingdom-code vulns
bash scripts/bpf-verifier-check.sh                         # GATE: PASSED, ≤25% budget
```

---

## 11. Cut-point decision tree (what gets dropped if compute / time runs out)

```
Phase 0 over-budget?  → Stop. Round Table on forking BPF program.
Phase 1.5 over-budget? → Drop shell+5cmds. Ship "kernel boots+halts" as L5.
                          Mode A/B not yet available; demo is just dmesg.
Phase 2.3 over-budget? → Drop busybox. Ship "Linux kernel boots+halts cleanly" as L6a.
                          Mode A xterm shows kernel banner only.
Phase 3.2 over-budget? → Drop dynamic linking. Ship static-only userspace as L6b.
                          All three demo modes still work (static busybox is enough).
Phase 4.5 over-budget? → Drop SSH. Ship ICMPv6+TCP-only.
                          Mode A and Mode B intact; Mode C deferred.
Phase 4.4 fails (ICMPv6 fails) → Drop networking entirely. Ship Phase 3 as deliverable.
                                  Mode A and Mode B intact; Mode C never lit.
```

Each cut-point produces a real, demoable artifact with at least Mode A (browser xterm) working. There is no failure mode that ends the campaign with nothing to show.

**The demo-surface guarantee:** Mode A (browser xterm) is unlocked at end of Phase 1 and never regresses. Mode B (direct console) is unlocked at end of Phase 1 and never regresses. Mode C (SSH over IPv6) is the Phase 4 deliverable; if it fails, the campaign still ships with two working surfaces.

---

## 12. Execution mode

Per `feedback_unattended_churn_with_queued_work.md` + `feedback_overnight_churn_pattern.md`:

- **Mechanical phases (0.1, 1.1, 1.4, 1.5, 2.1, 2.3, 3.3, 4.4)**: dispatched as Marshal-led multi-agent unattended overnight runs. Stevie wakes to a commit chain + shift report. Same pattern as the 2026-05-08 drain shift.
- **Design-pair phases (0.2 decisions 6-11, 0.3 decisions 13-16, 1.2 day 1 page-table call, 2.1 day 1 kernel-source call, 3.1 page-table model, 3.2 libc choice, 4.1 netdev model, 4.2 ARP analog)**: explicit pair-call moments with Stevie + the named skill. Plan must STOP and ask before proceeding past these.
- **Quarterly gates (1.6, 2.5, 3.5, 4.6)**: ship-or-defer decision points. Stevie reviews the gate evidence and either approves Phase N+1 or cuts to the deliverable.

Per `feedback_persist_plans_to_disk.md`: this plan mirrors to `references/battle-plan-ascend-linux-2026-05-08.md` after approval.

Per `feedback_battleplan_grounding.md`: this plan was grounded by three parallel Explore agents (UPC inventory, Doom lessons, prior-art sweep) before drafting; not reinventing already-specified work.
