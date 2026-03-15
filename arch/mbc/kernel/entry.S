/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/kernel/entry.S - Syscall and interrupt entry for MBC
 *
 * MBC syscall convention (INT 0x80):
 *   r0 = syscall number
 *   r1 = arg0, r2 = arg1, r3 = arg2, r4 = arg3, r5 = arg4
 *   r0 = return value
 *
 * Register save order on stack (pt_regs layout):
 *   r0-r15 (16 words), pc (1 word), flags (1 word) = 18 words
 *
 * Register conventions:
 *   r13 = LR (link register)
 *   r14 = FP (frame pointer)
 *   r15 = SP (stack pointer, implicit in PUSH/POP)
 *
 * The INT instruction auto-pushes flags and PC onto the kernel
 * stack before transferring control here.  IRET pops them back.
 */

.text

/*
 * ============================================================
 * NR_SYSCALLS — must match __NR_syscalls in asm/unistd.h
 * ============================================================
 */
.equ NR_SYSCALLS, 48
.equ ENOSYS, 38

/*
 * ============================================================
 * SYSCALL ENTRY — INT 0x80 lands here
 * ============================================================
 *
 * On entry the CPU has already pushed flags and PC onto the
 * kernel stack.  We save all general-purpose registers to form
 * a complete pt_regs frame, look up the handler in
 * sys_call_table[r0], call it, and restore registers.
 *
 * Stack frame after save (growing downward):
 *   [SP+ 0] r0   (syscall nr / return value)
 *   [SP+ 4] r1   (arg0)
 *   [SP+ 8] r2   (arg1)
 *   [SP+12] r3   (arg2)
 *   [SP+16] r4   (arg3)
 *   [SP+20] r5   (arg4)
 *   [SP+24] r6
 *   [SP+28] r7
 *   [SP+32] r8
 *   [SP+36] r9
 *   [SP+40] r10
 *   [SP+44] r11
 *   [SP+48] r12
 *   [SP+52] r13  (LR)
 *   [SP+56] r14  (FP)
 *   [SP+60] r15  (saved SP — user stack pointer)
 *   [SP+64] PC   (auto-pushed by INT)
 *   [SP+68] flags (auto-pushed by INT)
 */
syscall_entry:
    ; --- Save all general-purpose registers (r0-r14) ---
    ; r15/SP is implicitly the kernel stack already switched by INT
    PUSH r0
    PUSH r1
    PUSH r2
    PUSH r3
    PUSH r4
    PUSH r5
    PUSH r6
    PUSH r7
    PUSH r8
    PUSH r9
    PUSH r10
    PUSH r11
    PUSH r12
    PUSH r13                ; LR
    PUSH r14                ; FP

    ; --- Bounds-check syscall number ---
    CMP  r0, NR_SYSCALLS
    JNC  syscall_bad        ; if r0 >= NR_SYSCALLS, return -ENOSYS

    ; --- Load sys_call_table base into r6 ---
    MOVI r6, sys_call_table

    ; --- Compute table offset: r0 * 4 (each entry is one 32-bit pointer) ---
    MOV  r7, r0
    SHL  r7, 2              ; r7 = syscall_nr * 4

    ; --- Load handler address: sys_call_table[r0] ---
    ADD  r6, r7             ; r6 = &sys_call_table[r0]
    LD   r6, [r6]           ; r6 = sys_call_table[r0] (handler pointer)

    ; --- Set up arguments for the C handler ---
    ; r1-r5 already hold args from userspace (preserved above, still in regs)
    ; The C ABI expects args in r1-r5 which are already in place.

    ; --- Call the handler via indirect jump ---
    CALLR r6                ; call *r6 — handler returns result in r0

    ; --- Fall through to syscall_exit ---

syscall_exit:
    ; --- Store return value (r0) into the saved pt_regs slot ---
    ; We need to update the saved r0 on the stack so POP r0 gets the result.
    ; The saved r0 is at SP + 14*4 = SP+56 (we pushed 15 regs: r14..r0,
    ; so r0 is deepest, at offset 14 words from current SP).
    ST   [SP+56], r0        ; overwrite saved r0 with return value

    ; --- Restore all general-purpose registers ---
    POP  r14                ; FP
    POP  r13                ; LR
    POP  r12
    POP  r11
    POP  r10
    POP  r9
    POP  r8
    POP  r7
    POP  r6
    POP  r5
    POP  r4
    POP  r3
    POP  r2
    POP  r1
    POP  r0                 ; now holds syscall return value

    ; --- Return to userspace ---
    ; IRET pops PC and flags that INT auto-pushed
    IRET

syscall_bad:
    MOVI r0, -38            ; -ENOSYS
    JMP  syscall_exit


/*
 * ============================================================
 * IRQ ENTRY — hardware interrupt (timer at 0x20, kbd at 0x21)
 * ============================================================
 *
 * Same register-save convention as syscall_entry.  After saving
 * context we call the C handler do_IRQ(irq_number, pt_regs*).
 * On return we check if the scheduler needs to run (preemption).
 */
irq_entry:
    ; --- Save all general-purpose registers ---
    PUSH r0
    PUSH r1
    PUSH r2
    PUSH r3
    PUSH r4
    PUSH r5
    PUSH r6
    PUSH r7
    PUSH r8
    PUSH r9
    PUSH r10
    PUSH r11
    PUSH r12
    PUSH r13                ; LR
    PUSH r14                ; FP

    ; --- Prepare arguments for do_IRQ(irq_number, regs) ---
    ; r0 already contains the IRQ vector number set by the CPU
    ; r1 = pointer to pt_regs (current SP, top of saved frame)
    MOV  r1, SP

    ; --- Call C interrupt handler ---
    CALL do_IRQ

    ; --- Check if reschedule is needed ---
    ; Load need_resched flag from current thread_info
    MOVI r6, need_resched
    LD   r6, [r6]           ; r6 = need_resched flag
    CMP  r6, r6             ; test if non-zero (compare with self is always 0)
    ; Instead, compare with zero:
    MOVI r7, 0
    CMP  r6, r7
    JZ   irq_no_resched     ; if need_resched == 0, skip

    ; --- Call schedule() for preemption ---
    CALL schedule

irq_no_resched:
    ; --- Restore all general-purpose registers ---
    POP  r14                ; FP
    POP  r13                ; LR
    POP  r12
    POP  r11
    POP  r10
    POP  r9
    POP  r8
    POP  r7
    POP  r6
    POP  r5
    POP  r4
    POP  r3
    POP  r2
    POP  r1
    POP  r0

    ; --- Return from interrupt ---
    IRET


/*
 * ============================================================
 * RET FROM FORK — child process starts execution here
 * ============================================================
 *
 * After copy_process() sets up the child's kernel stack, its
 * saved PC points here.  The child's r0 is already 0 (fork
 * return value for child).  We just return to userspace.
 */
ret_from_fork:
    ; r0 = 0 (child's fork return value, set by copy_process)
    ; Restore callee-saved registers from the child's stack
    POP  r14                ; FP
    POP  r13                ; LR
    POP  r12
    POP  r11
    POP  r10
    POP  r9
    POP  r8
    POP  r7
    POP  r6
    POP  r5
    POP  r4
    POP  r3
    POP  r2
    POP  r1
    ; r0 stays 0 — do not pop, child must return 0
    ADDI SP, 4              ; skip saved r0 slot on stack

    ; Return to userspace
    IRET


/*
 * ============================================================
 * IVT SETUP — Interrupt Vector Table initialization
 * ============================================================
 *
 * Called once during boot (from init_IRQ) to populate the IVT
 * with entry point addresses.  The UPC IVT lives at address 0.
 */
setup_ivt:
    ; Vector 0x80: syscall entry
    MOVI r0, syscall_entry
    MOVI r1, 0x200          ; IVT[0x80] = 0x80 * 4 = 0x200
    ST   [r1], r0

    ; Vector 0x20: timer IRQ
    MOVI r0, irq_entry
    MOVI r1, 0x80           ; IVT[0x20] = 0x20 * 4 = 0x80
    ST   [r1], r0

    ; Vector 0x21: keyboard IRQ
    MOVI r0, irq_entry
    MOVI r1, 0x84           ; IVT[0x21] = 0x21 * 4 = 0x84
    ST   [r1], r0

    RET
