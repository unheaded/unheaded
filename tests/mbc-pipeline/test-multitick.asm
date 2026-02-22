
; Multi-tick execution test - count to 20
; Tests execution across multiple hops in the ring
; Expected: r0 = 20, insn_count = 62
    MOVI r0, 0
    MOVI r1, 20
loop:
    ADDI r0, 1
    CMP r0, r1
    JNZ loop
    HALT
