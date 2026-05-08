# UPC Boot Protocol v2

*SPDX-License-Identifier: GPL-3.0-or-later*

**Status:** Frozen 2026-05-08 per ADR-067.
**Supersedes:** `docs/doom/UPC_BOOT_PROTOCOL.md` (v1).
**Used by:** ASCEND-LINUX Phase 1+ (xv6, uClinux, full Linux 6.x).

---

## Overview

UPC Boot Protocol v2 extends v1 to support real Linux kernel images. The two changes from v1 are:

1. **Two-stage boot.** Stage 1 is a small in-ROM stub that runs at PC=0x10000; it zeroes the kernel BSS (mandatory for any Linux ELF), copies sections from the initramfs region into their final RAM destinations, then jumps to stage 2 at the kernel `_start`.
2. **256-byte BootParams.** The v1 48-byte BootParams becomes a 256-byte structure with five new fields plus a 192-byte reserved tail for forward-compat. See ADR-067 Decision 7.

Plus the broader ABI changes from ADR-067:
- **Multi-ring privilege:** boot starts in M-mode; stage 1 transitions to S-mode before jumping to kernel; userspace runs in U-mode.
- **CSR memory region** at `0x000_F000-0x000_F0FF` is initialized by the bootloader before stage 1 runs (zero-fill).
- **PerCPU state:** `BootParams.NumCPUs` is honored; bootloader allocates one MbcCpuState per CPU. NumCPUs=1 in Phases 0-3.

The boot process is managed by the "Computermancer" — `cmd/upc-bootctl boot` (new in Phase 1) — which prepares BPF maps with initial memory state and CPU configuration.

---

## Boot Sequence

```
0. Computermancer parses the kernel image (ELF, vmlinux.mbc).
   Records: text/data/rodata sections, .bss bounds, kernel _start, .init bounds.

1. Allocates BPF maps for NumCPUs MbcCpuState slots (PerCPU<MbcCpuState>).

2. Zeroes the IVT region (0x0000-0x03FF, 256 vectors × 4 bytes).
3. Zeroes the CSR region (0x000_F000-0x000_F0FF, 256 bytes / 64 CSRs).
4. Writes default trap handler stub at 0x0400 (a tight HALT loop).
5. Writes BootParams v2 (256 bytes) at byte address 0x0100.
6. Writes the kernel command line (variable, ≤512 B) at byte address 0x0200.

7. Loads the stage-1 boot stub (~1 KB) at byte address 0x10000.
   Stage 1 is provided by the kingdom (compiled from
   crates/upc-bootstub/src/boot.S into MBC bytecode).

8. Loads the kernel image at byte address derived from BootParams.kernel_addr
   (default: 0x20000 — leaves 64 KB for stage-1 stub + early scratch).

9. Loads the initramfs (CPIO+gzip or raw initrd) at BootParams.ramdisk_addr
   (default: 0x800000).

10. Optionally loads a second initramfs at BootParams.initrd2_addr (e.g.
    a kernel module bundle separate from rootfs).

11. Generates 32 bytes of high-quality entropy for BootParams.boot_random_seed
    (sourced from getrandom(2) on the host; passed in via the CSR region).

12. Initializes per-CPU MbcCpuState slots:
    - PC = 0x10000 (stage-1 entry)
    - r15 (SP) = 0x03F00000 (top-of-stack, grows down)
    - r0-r14 = 0
    - flags = 0
    - priv_level = 0 (M-mode)
    - interrupts_disabled = true
    - mmu_enabled = false

13. Computermancer signals "boot" to the eBPF program (Gjallarhorn UPC trigger).
    The XDP/tail-call chain begins executing at PC=0x10000.

14. Stage 1 runs:
    a. Reads BootParams to find bss_start / bss_end.
    b. Loops STB to zero every byte in [bss_start, bss_end).
    c. Copies any deferred sections from the initramfs region (if the build
       packaged them there; pure-vmlinux builds skip this step).
    d. Sets up initial S-mode CSRs (STVEC = trap_handler, SEPC = kernel_addr,
       SSTATUS = …).
    e. Issues SRET to transition M-mode → S-mode and jump to BootParams.kernel_addr.

15. Kernel _start runs in S-mode. Reads BootParams from word address 0x40,
    verifies magic, picks up cmdline, ramdisk, etc. From here the kernel
    is in charge — bootloader's job is done.
```

---

## Memory Layout (revised v2)

```
Byte Address      Word Address      Size       Contents
─────────────────────────────────────────────────────────────────────────
0x0000-0x03FF     0x0000-0x00FF     1 KB       IVT (256 vectors × 4 bytes)
0x0100-0x01FF     0x0040-0x007F     256 B      BootParams v2  (was 48 B in v1)
0x0200-0x03FF     0x0080-0x00FF     512 B      Kernel command line
0x0400-0x0403     0x0100            4 B        Default HLT trap handler
0x0404-0xEFFF     0x0101-0x3BFF     ~60 KB     Reserved (kernel can use)
0xF000-0xF0FF     0x3C00-0x3C3F     256 B      CSR region (64 × 4-byte CSRs) ← NEW v2
0xF100-0xFFFF     0x3C40-0x3FFF     ~3.7 KB    Reserved (early kernel scratch)
0x10000-0x1FFFF   0x4000-0x7FFF     64 KB      Stage-1 boot stub region        ← NEW v2
0x20000+          0x8000+           variable   Kernel image (was 0x10000 in v1)
0x800000+         0x200000+         variable   Initramfs (rootfs)
0x???????+        ...               variable   Optional initrd2 (modules etc.) ← NEW v2
0x03F00000        0x00FC0000        ~16 MB     Initial stack top (grows down)
```

The kernel image moved from 0x10000 (v1) to 0x20000 (v2) to make room for the stage-1 stub. v1 callers that hardcoded 0x10000 must update to read `BootParams.kernel_addr` (which v1 already had as a field — this just makes the indirection load-bearing).

---

## BootParams v2 Structure

```c
struct BootParamsV2 {
    /* === v1 fields, preserved === */
    u32 magic;             /* 'UNHD' = 0x554E4844 */
    u32 version;           /* 2 */
    u32 memory_size;       /* total RAM bytes (default 64 MB) */
    u32 ramdisk_addr;      /* ramdisk byte address */
    u32 ramdisk_size;      /* ramdisk size in bytes */
    u32 kernel_addr;       /* kernel load address (v1 default 0x10000;
                              v2 default 0x20000) */
    u32 kernel_size;       /* kernel image size in bytes */
    u32 boot_args_addr;    /* command line address (default 0x0200) */
    u32 boot_args_len;     /* command line length */
    u32 num_cpus;          /* number of CPU instances; 1 in Phases 0-3 */
    u32 tick_rate_hz;      /* timer interrupt rate (default 12) */

    /* === v2 new fields === */
    u32 bss_start;         /* kernel .bss start byte address */
    u32 bss_end;           /* kernel .bss end byte address (exclusive) */
    u32 initrd2_addr;      /* optional second initramfs (0 = absent) */
    u32 cmd_line_args_ptr; /* alternate cmdline pointer (replaces boot_args_addr if non-zero) */
    u32 boot_random_seed;  /* 32-byte entropy block address (0 = no entropy) */

    /* === forward-compat reservation === */
    u32 reserved[48];      /* zero-filled by bootloader; future fields land here */
} __attribute__((packed));    /* total: 16 + 5 × 4 + 48 × 4 = 256 bytes exactly */
```

**Field semantics:**

- **magic** (`'UNHD'` = 0x554E4844): kernel reads first; if mismatch, kernel HALTs immediately (boot protocol violation).
- **version** (2): kernel reads second; if mismatch, kernel can choose to refuse boot or fall back. uClinux + Linux 6.x require v2; xv6 (Phase 1) supports both.
- **bss_start / bss_end**: stage 1 zeroes `[bss_start, bss_end)` byte range before jumping to kernel. v1 boots without this rely on pre-zeroed RAM, which fails on warm-boot. **Mandatory for Linux.**
- **initrd2_addr**: 0 = no second ramdisk. Used when modules ship separately from rootfs (Linux convention).
- **cmd_line_args_ptr**: 0 = use the inline boot_args at `boot_args_addr`. Non-zero = pointer to a struct with longer/structured args. Forward-compat for kernel command lines exceeding 512 B.
- **boot_random_seed**: byte address of a 32-byte entropy block. Linux's `random_init` consumes this for early-boot RNG. 0 = bootloader didn't provide entropy (kernel falls back to boot-time IRQ jitter, much weaker).
- **reserved[48]**: zero-filled. Adding fields here is a v2-compatible extension; consuming a reserved slot bumps a "minor version" expectation but doesn't break old kernels (they ignore reserved bytes).

---

## CSR Region Initialization

Per ADR-067 Decision 2, CSRs live at memory addresses `0x000_F000 + csr_index * 4`. The bootloader zero-fills the entire region (256 bytes) before stage 1 runs. Stage 1 then writes initial values:

| CSR | RV32 Index | Boot Value | Purpose |
|-----|-----------|-----------|---------|
| MSTATUS | 0x300 | 0x00001800 | M-mode, MPP=M (so first MRET goes to M; stage 1 manually transitions to S) |
| MTVEC | 0x305 | 0x00000400 | Default trap handler at 0x0400 (HALT loop) |
| MEPC | 0x341 | kernel_addr | Where MRET will return |
| MISA | 0x301 | 0x40001100 | RV32 + I + M + Zicsr |
| SATP | 0x180 | 0 | MMU disabled at boot; kernel enables when ready |
| STVEC | 0x105 | 0x00000400 | S-mode trap handler default (HALT) |
| SSTATUS | 0x100 | 0 | clean S-mode state |

Stage 1 may rewrite STVEC/MTVEC before MRET to install kernel's trap handlers; this is part of stage-1 stub responsibility, not the bootloader's.

---

## CPU State at Boot (per CPU)

| Register / Field | Value |
|------------------|-------|
| PC | 0x10000 (stage-1 entry) |
| r15 (SP) | 0x03F00000 |
| r14 (RA) | 0 |
| r0-r13 | 0 |
| flags (Z, N, C, IF) | 0 |
| **priv_level** | **0 (M-mode)** ← NEW v2 |
| interrupt_pending | 0 |
| interrupts_enabled | false |
| mmu_enabled | false |
| reservation_address | 0xFFFFFFFF (no reservation; ← NEW v2 for LR.W/SC.W tracking) |

---

## Stage-1 Boot Stub (informative pseudo-code)

```asm
; entry: PC=0x10000, priv=M, all regs zero
boot_stage1_entry:
    ; 1. Read BootParams
    LDR     r1, =0x100              ; BootParams base
    LDR     r2, [r1, #44]           ; bss_start
    LDR     r3, [r1, #48]           ; bss_end

    ; 2. Zero BSS
zero_bss_loop:
    CMP     r2, r3
    BGE     zero_bss_done
    LDR     r4, =0
    STB     r4, [r2]
    ADDI    r2, r2, #1
    JMP     zero_bss_loop
zero_bss_done:

    ; 3. Set up S-mode CSRs
    LDR     r1, [BootParams.kernel_addr]
    STR     r1, [CSR.SEPC]          ; ST 0xF000 + 0x141*4, r1
    LDR     r1, =0x100              ; default S-mode SSTATUS (kernel will rewrite)
    STR     r1, [CSR.SSTATUS]

    ; 4. Transition to S-mode and jump
    SRET                            ; pops SEPC into PC, priv → S-mode
```

The actual stage-1 stub is compiled from `crates/upc-bootstub/src/boot.c` + linker script, then translated to MBC. It's small enough (~1 KB MBC) to fit in the 64 KB region.

---

## Error Conditions

| Condition | Behavior |
|-----------|----------|
| Magic mismatch | Kernel HALTs immediately at PC=kernel_addr+8 (after magic check) |
| Version mismatch | Kernel-defined; v2-aware kernels can fall back to v1 if `version==1` |
| `bss_end < bss_start` | Stage 1 HALTs |
| `kernel_addr` overlaps stage-1 region (< 0x20000) | Bootloader rejects load before stage-1 runs |
| Out-of-range `ramdisk_addr` (above `memory_size`) | Bootloader rejects |
| `num_cpus > MAX_CPUS` (compile-time const, 4 today) | Bootloader caps at MAX_CPUS, sets a flag readable via CSR.MISA reserved bits |
| Stage-1 stub itself crashes | PC bounds check in eBPF interpreter halts the program; debugging requires bpftool prog dump |

---

## Migration from v1

v1 BootParams readers MUST be updated:

1. Magic + version + memory_size fields are at the same offsets — backward-compatible.
2. `kernel_addr` semantics tightened — v1 called this "informative" with hardcoded 0x10000 entry; v2 makes it the authoritative entry address (default 0x20000).
3. v1 readers that hardcode `kernel_addr` as 0x10000 will boot from the stage-1 stub region, which is wrong. They must read `BootParams.kernel_addr`.
4. v1 readers that don't zero BSS will fail on Linux. xv6 won't notice because it has no .bss.

`pkg/upc/boot.go::DefaultBootConfig()` (the existing Go-side boot helper) needs to be extended in Phase 1 to populate the v2 fields. Forward-compat: existing v1 callers that ignore the new fields continue to work; the bootloader treats v1 BootParams as "BSS already zeroed by caller, no entropy provided."

---

## Versioning policy

- **v1**: 48-byte structure. SHIPPED as part of Doom + L4. Stays usable for xv6 Phase 1 if Stevie wants the lighter spec.
- **v2**: 256-byte structure with 5 new fields + 48 reserved. SHIPPED 2026-05-08 (this doc). Required for uClinux Phase 2+.
- **v3**: hypothetical future, would shift `magic` semantics or change struct layout. Adding fields into the reserved[48] block is v2-compatible and does not require v3.

---

## Cross-references

- ADR-067 — MBC ISA v2 + UPC ABI v1 (the seven design decisions this protocol depends on)
- `docs/doom/UPC_BOOT_PROTOCOL.md` — v1 (preserved unchanged)
- `pkg/upc/boot.go` — v1 implementation; needs v2 extension in Phase 1
- `docs/doom/ROAD_TO_LINUX.md` — original Dream Ladder boot-protocol spec
- `crates/upc-bootstub/` — stage-1 stub crate (NEW; created in Phase 0.4)
- `references/battle-plan-ascend-linux-2026-05-08.md` — campaign source
