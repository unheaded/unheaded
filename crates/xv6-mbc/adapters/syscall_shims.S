/* SPDX-License-Identifier: GPL-3.0-or-later
 *
 * syscall_shims.S — translator-recognized externs for UPC L4e block syscalls.
 * ASCEND-LINUX Phase 1.1 (per references/battle-plan-ascend-linux-2026-05-08.md).
 *
 * blk-ramdisk.c calls __upc_sys_read_block / __upc_sys_write_block. These
 * shims bridge the C ABI (args in a0/a1 = r10/r11 in RV32) to the MBC
 * SYSCALL ABI (syscall_nr in r0, args in r1/r2/r3).
 *
 * RV32 calling convention recap:
 *   x10 (a0) = first arg / return value
 *   x11 (a1) = second arg
 *   x1  (ra) = return address
 *
 * MBC register file (16 GPRs r0-r15):
 *   r0  = scratch (also syscall_nr at SYSCALL time)
 *   r14 = ra (return address; matches RV32 x1)
 *   r15 = sp (stack pointer; matches RV32 x2)
 *   The translator maps RV32 a0/a1 (x10/x11) → MBC r10/r11 in the
 *   restricted 16-reg file (CFLAGS: -ffixed-x16..x31 in Makefile.mbc).
 *
 * Pattern: load syscall_nr to r0, move args, SYSCALL, RET. The translator
 * emits these as a fixed sequence the rv32i-to-mbc translator recognizes.
 */

.text

.globl __upc_sys_read_block
__upc_sys_read_block:
    /* a0 = block_num (in), a1 = dest_byte_addr (in), a0 = ret (out)
     *
     * RV32I ecall convention: a7 holds the syscall number. The MBC
     * op::SYSCALL dispatcher reads syscall_nr from r1 (= MBC mapping of
     * RV x17 = a7, per crates/monad-mbc/src/translator.rs::map_register).
     * a2 (→ r10) holds the chunk-progress counter; the handler reuses it
     * across multi-tick block copies, so zero it on first entry.
     * Block syscalls now live on the op::SYSCALL path with handler at
     * ebpf/monad-cpu-ebpf/src/main.rs (Phase 1.4 fsinit unblock 2026-05-14).
     */
    li      a7, 200             /* SYS_READ_BLOCK in a7 */
    li      a2, 0               /* progress = 0 for first chunk */
    ecall                       /* translator emits MBC SYSCALL (0x40) */
    ret

.globl __upc_sys_write_block
__upc_sys_write_block:
    /* a0 = block_num (in), a1 = src_byte_addr (in), a0 = ret (out) */
    li      a7, 201             /* SYS_WRITE_BLOCK in a7 */
    li      a2, 0               /* progress = 0 for first chunk */
    ecall
    ret

/* TODO Phase 1.1 (next shift): the syscall arg-shuffle above is provisional.
 * The MBC SYSCALL convention is:
 *   r0 = syscall number (input + return value)
 *   r1 = arg1
 *   r2 = arg2
 *   r3 = arg3
 *
 * The rv32i-to-mbc translator's RV32 → MBC register mapping (defined in
 * crates/monad-mbc/src/translator.rs) decides whether ecall fits the
 * call-site shape directly or needs an explicit MBC instruction emit
 * (similar to how Doom's libc_stubs.c handles syscalls). Pick the cleaner
 * option once the first end-to-end build runs and we can see what the
 * translator actually produces.
 */
