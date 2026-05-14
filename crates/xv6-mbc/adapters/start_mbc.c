// SPDX-License-Identifier: GPL-3.0-or-later
//
// start_mbc.c — MBC-native replacement for upstream/kernel/start.c.
// ASCEND-LINUX Phase 1.1 (per references/battle-plan-ascend-linux-2026-05-08.md).
//
// Differences from xv6's upstream start.c (which we replace, not patch):
//
//   - upstream/kernel/start.c uses RV64 CSR intrinsics: r_mstatus(), w_mepc(),
//     w_satp(), w_medeleg(), w_mideleg(), w_pmpaddr0(), w_pmpcfg0(), r_mhartid(),
//     w_tp(), and trailing `asm volatile("mret")`.
//
//   - We are RV32 with NO inline-asm CSR access. CSRs live in the memory-mapped
//     CSR region at 0x000_F000-0x000_F0FF per ADR-067 Decision 2. CSR_<name>
//     macros below resolve to `*(volatile uint32_t*)(0xF000 + index*4)`.
//
//   - We use the BootParams v2 structure at byte address 0x100 per
//     UPC_BOOT_PROTOCOL_V2.md. Magic 'UNHD' verification + v2 field reads.
//
//   - The MBC ISA `MRET` opcode (0x47) does the M→S transition; no inline
//     `mret` assembly. Compiled-MBC code emits an MRET instruction directly.
//
//   - PMP (Physical Memory Protection) is handled by our MMU emulation
//     (Wotan + L4d), not RV's PMP regs. Removed.
//
//   - Per-CPU `tp` register: not used (single-CPU UPC instance per ADR-067
//     Decision 3; multi-instance parallelism via networking).
//
// First milestone for Phase 1.1: this adapter prints "xv6 booting..."
// to MMIO 0xC001 (UPC tty), then hands off to xv6 main(). If main()
// HALTs cleanly, sub-phase 1.1 ships.

#include "types.h"
#include "param.h"  /* NCPU */

// entry.S needs one stack per CPU; lives in .bss because we need
// the symbol resolvable by the linker. 4 KB per CPU, NCPU CPUs.
__attribute__((aligned(16))) char stack0[4096 * NCPU];

// ── BootParams v2 layout (at byte address 0x100) ───────────────────────────
// See docs/doom/UPC_BOOT_PROTOCOL_V2.md.
#define BOOT_PARAMS_ADDR  0x100
#define BOOT_MAGIC        0x554E4844u  // 'UNHD'
#define BOOT_VERSION_V2   2

struct boot_params_v2 {
    uint32 magic;
    uint32 version;
    uint32 memory_size;
    uint32 ramdisk_addr;
    uint32 ramdisk_size;
    uint32 kernel_addr;
    uint32 kernel_size;
    uint32 boot_args_addr;
    uint32 boot_args_len;
    uint32 num_cpus;
    uint32 tick_rate_hz;
    uint32 bss_start;
    uint32 bss_end;
    uint32 initrd2_addr;
    uint32 cmd_line_args_ptr;
    uint32 boot_random_seed;
    uint32 reserved[48];
};

// ── Memory-mapped CSR region (ADR-067 Decision 2) ─────────────────────────
// CSR addresses follow RV32 standard: MSTATUS=0x300, MEPC=0x341, etc.
// Memory address: 0x000_F000 + csr_index * 4.
#define CSR_BASE          0xF000u
#define CSR_MSTATUS       0x300
#define CSR_MTVEC         0x305
#define CSR_MEPC          0x341
#define CSR_SSTATUS       0x100
#define CSR_STVEC         0x105
#define CSR_SEPC          0x141
#define CSR_SATP          0x180

#define CSR_REG(csr) (*(volatile uint32 *)(CSR_BASE + (csr) * 4))

// MSTATUS bits (RV32 standard)
#define MSTATUS_MPP_S     (1u << 11)  // MPP=01 (S-mode) at bits [12:11]

// ── MMIO console (UPC L4f convention) ─────────────────────────────────────
//
// 2026-05-11: switched from `volatile uint32 *` to `volatile uint8 *`. The
// previous u32 store at unaligned address 0xC001 was being lowered by GCC
// into 4 separate byte stores (sb at 0xC001, 0xC002, 0xC003, 0xC004), of
// which the first 3 hit the eBPF MMIO TTY intercept range (0xC000-0xC003)
// and produced 3 bytes per call (char + 2 zero pad). The fourth store
// landed in RAM at 0xC004 — wasted I/O.
//
// A direct byte store skips the splitting: one sb at 0xC001 -> one TTY
// intercept -> one byte emitted. Drops the "x..v..6.. ..b.." pattern to
// the clean "xv6 booting...\n" the operator actually wants to see.
#define UPC_TTY_DATA_ADDR    0xC001
#define UPC_TTY_REG          (*(volatile uint8 *)(UPC_TTY_DATA_ADDR))

static inline void mmio_putc(char c) {
    UPC_TTY_REG = (uint8)c;
}

// Non-static so main.c can call this for early-init instrumentation
// (Phase 1.4 xv6 init-loop bisection — see
// references/phase14-xv6-init-loop-root-cause-2026-05-13.md).
void mmio_puts(const char *s) {
    while (*s) {
        mmio_putc(*s++);
    }
}

// xv6 main() lives in main.c after our patches.
extern void main(void);

// ── ASCEND-LINUX entry point ─────────────────────────────────────────────
//
// Stage 1 of UPC_BOOT_PROTOCOL_V2 calls us in M-mode on the kernel's
// initial stack. We:
//
//   1. Verify BootParams magic + version.
//   2. Print a one-liner so the operator knows boot is alive.
//   3. Set up M-mode → S-mode transition via MEPC + MSTATUS.MPP=S.
//   4. Issue MRET — MBC opcode 0x47 jumps to MEPC at S-mode priv.
//   5. main() runs in S-mode and never returns.
//
// If anything fails, we print "BOOT FAIL: <reason>" and HALT (MBC opcode 0xFF).
void
start(void)
{
    volatile struct boot_params_v2 *bp = (struct boot_params_v2 *)BOOT_PARAMS_ADDR;

    // Step 1: verify boot params.
    if (bp->magic != BOOT_MAGIC) {
        mmio_puts("BOOT FAIL: bad magic\n");
        // MBC HALT opcode encoded inline. Translator emits 0xFF.
        __asm__ volatile ("ebreak"); /* translator emits MBC HALT for ebreak */
        while (1) {}
    }
    if (bp->version != BOOT_VERSION_V2) {
        mmio_puts("BOOT FAIL: bad version\n");
        __asm__ volatile ("ebreak"); /* translator emits MBC HALT for ebreak */
        while (1) {}
    }

    // Step 2: announce boot.
    mmio_puts("xv6 booting...\n");

    // Step 3: M → S transition via memory-mapped CSRs.
    // Set MPP=S in MSTATUS (bits 12:11 = 01).
    uint32 mstatus = CSR_REG(CSR_MSTATUS);
    mstatus &= ~(0b11u << 11);     // clear MPP field
    mstatus |= MSTATUS_MPP_S;      // set MPP=01 (S-mode)
    CSR_REG(CSR_MSTATUS) = mstatus;

    // MEPC = main(); the address translator places main() somewhere in the
    // MBC text region, and we encode the byte address here. The compiled MBC
    // MRET (0x47) reads MEPC>>2 and sets PC to that word index.
    // (uint32) cast directly — main is a function pointer; in RV32 it's already
    // 32-bit so we don't need uintptr_t.
    CSR_REG(CSR_MEPC) = (uint32)(uint64)main;

    // Step 4: MRET. RV32's `mret` mnemonic (encoding 0x30200073). The
    // translator (post-Phase-0.4-CSR-extension) recognizes this and emits
    // MBC opcode 0x47.
    __asm__ volatile ("mret");

    // If we ever reach here, MRET failed somehow.
    mmio_puts("BOOT FAIL: MRET fall-through\n");
    __asm__ volatile ("ebreak"); /* translator emits MBC HALT for ebreak */
    while (1) {}
}
