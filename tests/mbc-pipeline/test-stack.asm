
; PUSH/POP stack test - verify stack preservation
; Expected: r0 = 100, r1 = 200, r2 = 300
    MOVI r0, 100
    MOVI r1, 200
    MOVI r2, 300
    PUSH r0
    PUSH r1
    PUSH r2
    MOVI r0, 0
    MOVI r1, 0
    MOVI r2, 0
    POP r2
    POP r1
    POP r0
    HALT
