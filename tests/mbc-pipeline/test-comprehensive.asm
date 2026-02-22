
; ISA comprehensive smoke test (Step 160)
; Tests: MUL, DIV, MOD, nested CALL, GET_TICKS syscall
; Expected: r0 = 7*6 = 42, r1 = 42/6 = 7, r2 = 42%5 = 2, r3 = ticks > 0

    ; Test MUL
    MOVI r4, 7
    MOVI r5, 6
    MOV r0, r4
    MUL r0, r5          ; r0 = 7 * 6 = 42

    ; Test DIV
    MOV r1, r0
    DIV r1, r5          ; r1 = 42 / 6 = 7

    ; Test MOD
    MOV r2, r0
    MOVI r6, 5
    MOD r2, r6          ; r2 = 42 % 5 = 2

    ; Test nested CALL
    CALL outer_func
    ; After return: r7 should be 99

    ; Test GET_TICKS syscall
    MOVI r0, 3
    SYSCALL 3            ; r0 = millisecond ticks
    MOV r3, r0           ; r3 = ticks (should be > 0)

    ; Restore expected values
    MOVI r0, 42
    HALT

outer_func:
    PUSH r0
    CALL inner_func
    POP r0
    RET

inner_func:
    MOVI r7, 99
    RET
