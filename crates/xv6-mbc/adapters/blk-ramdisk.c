// SPDX-License-Identifier: GPL-3.0-or-later
//
// blk-ramdisk.c — MBC-native replacement for upstream/kernel/virtio_disk.c.
// ASCEND-LINUX Phase 1.1 (per references/battle-plan-ascend-linux-2026-05-08.md).
//
// upstream/kernel/virtio_disk.c (327 LOC) implements the QEMU virtio-blk
// MMIO protocol: descriptor rings, available/used queues, interrupt-driven
// completion, and a per-buf state machine. All of that is QEMU-virt-machine-
// specific and meaningless on UPC.
//
// We replace it with synchronous calls to UPC L4e block syscalls:
//
//   SYS_READ_BLOCK  (200): r0=200, r1=block_num, r2=dest_addr → r0=0/-EIO
//   SYS_WRITE_BLOCK (201): r0=201, r1=block_num, r2=src_addr  → r0=0/-EIO
//
// The MBC dispatch handles these in its eBPF interpreter against a 4 MiB
// ramdisk in Wotan extended memory at 0x100000 (per UPC_OS_PRIMITIVES.md L4e).
//
// Exported API matches upstream/kernel/defs.h:
//   void virtio_disk_init(void)              — no-op (ramdisk always present)
//   void virtio_disk_rw(struct buf *, int)   — synchronous R/W
//   void virtio_disk_intr(void)              — no-op (sync I/O = no IRQ)
//
// xv6's bio.c (buffer cache) and fs.c don't change — they call these symbols.

#include "types.h"
#include "spinlock.h"
#include "sleeplock.h"
#include "fs.h"
#include "buf.h"

// ── L4e block syscall numbers (UPC convention, matches mbc_linux_syscalls) ──
#define SYS_READ_BLOCK   200
#define SYS_WRITE_BLOCK  201

// xv6 uses 1024-byte blocks (BSIZE in upstream/kernel/fs.h).
// UPC ramdisk uses 512-byte sectors. xv6 block N = UPC sectors 2N..2N+1.

// ── Inline MBC syscall helpers ─────────────────────────────────────────────
// SYSCALL is opcode 0x40; the syscall number lives in r0, args in r1+r2+r3.
// The C-source-to-MBC translator emits SYSCALL when it sees this pattern:
//
//   __asm__ volatile (
//       "li a0, %1\n"        // syscall number
//       "li a1, %2\n"        // block_num
//       "li a2, %3\n"        // addr
//       "ecall\n"            // RV32 syscall; translator → MBC SYSCALL
//       : "=r" (ret) : "i" (syscall_nr), "r" (blk), "r" (buf) : "a0","a1","a2"
//   );
//
// Until the translator's inline-asm emit is verified, we use an extern
// shim that the translator recognizes by name. The shim's MBC body is
// just SYSCALL + RET.
extern int __upc_sys_read_block(uint32 block_num, uint32 dest_byte_addr);
extern int __upc_sys_write_block(uint32 block_num, uint32 src_byte_addr);

void
virtio_disk_init(void)
{
    // No initialization. Ramdisk is wired by the bootloader at 0x100000
    // (per BootParams.ramdisk_addr) before start_mbc.c runs.
}

void
virtio_disk_rw(struct buf *b, int write)
{
    // xv6 block index → UPC sector indices. BSIZE=1024 = 2 × 512.
    uint32 sector0 = b->blockno * 2;
    uint32 sector1 = sector0 + 1;
    uint32 buf_addr = (uint32)(uintptr_t)&b->data[0];
    uint32 buf_addr_2nd = buf_addr + 512;

    if (write) {
        __upc_sys_write_block(sector0, buf_addr);
        __upc_sys_write_block(sector1, buf_addr_2nd);
    } else {
        __upc_sys_read_block(sector0, buf_addr);
        __upc_sys_read_block(sector1, buf_addr_2nd);
    }
    // Synchronous completion — no IRQ to wait on. xv6's bio.c expects b
    // to be marked valid + clean by the time we return.
    b->valid = 1;
    b->disk = 0;
}

void
virtio_disk_intr(void)
{
    // No-op. UPC L4e block syscalls are synchronous; there is no completion
    // interrupt to acknowledge. xv6's plic.c references this symbol so we
    // keep it in the link surface.
}
