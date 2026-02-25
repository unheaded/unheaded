
; Branch conditions test - verify CMP + conditional jumps
; Test: r0 counts how many branch tests pass
    MOVI r0, 0

    ; Test 1: JZ when equal (CMP 5, 5 sets Z flag)
    MOVI r1, 5
    MOVI r2, 5
    CMP r1, r2
    JZ test1_pass
    JMP test2
test1_pass:
    ADDI r0, 1

test2:
    ; Test 2: JNZ when not equal (CMP 5, 3 clears Z flag)
    MOVI r1, 5
    MOVI r2, 3
    CMP r1, r2
    JNZ test2_pass
    JMP test3
test2_pass:
    ADDI r0, 1

test3:
    ; Test 3: JN when r1 < r2 signed (CMP 3, 5 sets N flag when 3-5 has MSB set in 32-bit)
    ; Actually 3-5 = 0xFFFFFFFE which has MSB set, so N flag
    MOVI r1, 3
    MOVI r2, 5
    CMP r1, r2
    JN test3_pass
    JMP test4
test3_pass:
    ADDI r0, 1

test4:
    ; Test 4: JP when r1 > r2 (CMP 5, 3: 5-3=2, no N flag)
    MOVI r1, 5
    MOVI r2, 3
    CMP r1, r2
    JP test4_pass
    JMP done
test4_pass:
    ADDI r0, 1

done:
    ; r0 should be 4 (all tests passed)
    HALT
