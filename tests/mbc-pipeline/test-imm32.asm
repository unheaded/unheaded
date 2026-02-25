
; LOAD_IMM32 test - construct 32-bit values and verify
; Expected: r0 = 0xDEADBEEF, r1 = 0xCAFE0000
    MOVI r0, 0xBEEF        ; r0 = 0x0000BEEF
    LOAD_IMM32 r0, 0xDEAD  ; r0[31:16] = 0xDEAD -> r0 = 0xDEADBEEF

    MOVI r1, 0x0000        ; r1 = 0x00000000
    LOAD_IMM32 r1, 0xCAFE  ; r1[31:16] = 0xCAFE -> r1 = 0xCAFE0000

    ; Arithmetic on 32-bit values
    MOV r2, r0
    SUB r2, r1             ; r2 = 0xDEADBEEF - 0xCAFE0000 = 0x13AFBEEF

    HALT
