/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/kernel/head.S - MBC kernel entry point
 *
 * First code that runs after the UPC boot loader.
 *
 * Boot loader sets:
 *   PC = address of _start
 *   SP = top of kernel stack
 *   RAM[0x0100] = boot params (struct mbc_boot_params)
 *
 * This code:
 *   1. Disables interrupts
 *   2. Sets up the kernel stack pointer
 *   3. Clears the BSS section
 *   4. Initializes the IVT
 *   5. Calls start_kernel()
 *
 * Week 6 of S-LINUX: proper boot entry point.
 */

.equ KERNEL_STACK_TOP, 0x00080000

.text
.global _start

_start:
    ; Disable interrupts during early boot
    CLI

    ; Set kernel stack pointer (r15 = SP by convention)
    MOVI r15, KERNEL_STACK_TOP

    ; Clear BSS section — zero all words from __bss_start to __bss_end
    MOVI r0, __bss_start
    MOVI r1, __bss_end
    MOVI r2, 0
bss_loop:
    CMP  r0, r1
    JZ   bss_done
    ST   [r0], r2           ; zero one word
    ADDI r0, 4
    JMP  bss_loop
bss_done:

    ; Set up IVT entries (defined in entry.S)
    CALL setup_ivt

    ; Call C start_kernel() — should never return
    CALL start_kernel

    ; If start_kernel() returns, halt the CPU
    HLT
