/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/kernel/entry.S - Syscall and interrupt entry for MBC
 *
 * MBC syscall convention (INT 0x80):
 *   r0 = syscall number
 *   r1 = arg0
 *   r2 = arg1
 *   r3 = arg2
 *   r4 = arg3
 *   r5 = arg4
 *   r0 = return value
 *
 * Register save order on stack (pt_regs layout):
 *   r0-r15 (16 words), pc (1 word), flags (1 word) = 18 words
 *
 * This is a skeleton — actual MBC assembly will be added once
 * the MBC assembler (tools/mbc-asm) generates relocatable object
 * files compatible with the kernel build system.
 */

/*
 * syscall_entry:
 *   ; Save all registers to kernel stack
 *   ; Look up syscall number in sys_call_table[r0]
 *   ; Call handler with args in r1-r5
 *   ; Store return value in saved r0
 *   ; Restore registers
 *   ; Return to userspace (IRET)
 */

/*
 * irq_entry:
 *   ; Save all registers to kernel stack
 *   ; Call do_IRQ(irq_number, regs)
 *   ; Restore registers
 *   ; Return from interrupt (IRET)
 */

/*
 * ret_from_fork:
 *   ; Child process returns here after fork/vfork
 *   ; r0 = 0 (child return value)
 *   ; Jump to syscall_exit path
 */
