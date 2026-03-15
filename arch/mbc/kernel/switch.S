/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/kernel/switch.S - Context switch for MBC
 *
 * switch_to(prev, next)
 *
 * Save prev's callee-saved registers (r4-r14) plus SP and PC
 * into prev->thread, then load next's saved state and resume
 * execution at next's saved PC.
 *
 * Called from schedule() with:
 *   r1 = prev (pointer to task_struct)
 *   r2 = next (pointer to task_struct)
 *
 * struct thread_struct layout (offsets from thread base):
 *   +0x00  sp       (saved stack pointer)
 *   +0x04  pc       (saved program counter / return address)
 *   +0x08  regs[0]  (r0 — not callee-saved, included for pt_regs compat)
 *   +0x0C  regs[1]  (r1)
 *   +0x10  regs[2]  (r2)
 *   +0x14  regs[3]  (r3)
 *   +0x18  regs[4]  (r4  — callee-saved)
 *   +0x1C  regs[5]  (r5  — callee-saved)
 *   +0x20  regs[6]  (r6  — callee-saved)
 *   +0x24  regs[7]  (r7  — callee-saved)
 *   +0x28  regs[8]  (r8  — callee-saved)
 *   +0x2C  regs[9]  (r9  — callee-saved)
 *   +0x30  regs[10] (r10 — callee-saved)
 *   +0x34  regs[11] (r11 — callee-saved)
 *   +0x38  regs[12] (r12 — callee-saved)
 *   +0x3C  regs[13] (r13/LR)
 *   +0x40  regs[14] (r14/FP — callee-saved)
 *   +0x44  regs[15] (r15/SP — redundant with +0x00, kept for indexing)
 *   +0x48  flags
 *
 * THREAD_OFFSET is the byte offset of thread_struct within
 * task_struct.  This must match the kernel's task_struct layout.
 */

.text

.equ THREAD_OFFSET, 0      ; set by kernel build (offsetof(task_struct, thread))
.equ THREAD_SP, 0           ; offsetof(thread_struct, sp)
.equ THREAD_PC, 4           ; offsetof(thread_struct, pc)
.equ THREAD_REGS, 8         ; offsetof(thread_struct, regs[0])
.equ THREAD_FLAGS, 72       ; offsetof(thread_struct, flags) = 8 + 16*4

/*
 * ============================================================
 * switch_to — save prev, restore next, resume next
 * ============================================================
 *
 * r1 = prev task_struct*
 * r2 = next task_struct*
 */
switch_to:
    ; --- Compute prev->thread base address ---
    MOV  r0, r1
    ADDI r0, THREAD_OFFSET  ; r0 = &prev->thread

    ; --- Save prev's stack pointer ---
    ST   [r0+0], SP         ; prev->thread.sp = SP

    ; --- Save prev's return address (where to resume) ---
    ; Use the address of .Lprev_resume as the saved PC.
    ; When prev is switched back in, it returns right after
    ; the switch point.
    MOVI r6, .Lprev_resume
    ST   [r0+4], r6         ; prev->thread.pc = .Lprev_resume

    ; --- Save prev's callee-saved registers (r4-r14) ---
    ST   [r0+24], r4        ; regs[4]  at offset 8 + 4*4 = 24
    ST   [r0+28], r5        ; regs[5]
    ST   [r0+32], r6        ; regs[6]
    ST   [r0+36], r7        ; regs[7]
    ST   [r0+40], r8        ; regs[8]
    ST   [r0+44], r9        ; regs[9]
    ST   [r0+48], r10       ; regs[10]
    ST   [r0+52], r11       ; regs[11]
    ST   [r0+56], r12       ; regs[12]
    ST   [r0+60], r13       ; regs[13] / LR
    ST   [r0+64], r14       ; regs[14] / FP

    ; --- Compute next->thread base address ---
    MOV  r0, r2
    ADDI r0, THREAD_OFFSET  ; r0 = &next->thread

    ; --- Restore next's callee-saved registers (r4-r14) ---
    LD   r4,  [r0+24]       ; regs[4]
    LD   r5,  [r0+28]       ; regs[5]
    LD   r6,  [r0+32]       ; regs[6]
    LD   r7,  [r0+36]       ; regs[7]
    LD   r8,  [r0+40]       ; regs[8]
    LD   r9,  [r0+44]       ; regs[9]
    LD   r10, [r0+48]       ; regs[10]
    LD   r11, [r0+52]       ; regs[11]
    LD   r12, [r0+56]       ; regs[12]
    LD   r13, [r0+60]       ; regs[13] / LR
    LD   r14, [r0+64]       ; regs[14] / FP

    ; --- Restore next's stack pointer ---
    LD   r3,  [r0+0]        ; r3 = next->thread.sp
    MOV  SP, r3             ; SP = next's stack

    ; --- Load next's saved PC and jump to it ---
    LD   r3,  [r0+4]        ; r3 = next->thread.pc
    JMPR r3                 ; resume next (jumps to next's saved PC)

.Lprev_resume:
    ; When prev is scheduled back in, execution resumes here.
    ; All callee-saved registers have been restored by the
    ; switch_to that brought us back.
    RET
