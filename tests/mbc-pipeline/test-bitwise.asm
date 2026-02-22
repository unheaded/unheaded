
; Bitwise operations test
; Expected: r0 = 0xFF00, r1 = 0x0F0F, r2 = 0xF0F0, r3 = 0xFF0F
    MOVI r0, 0xFF00       ; r0 = 0xFF00
    MOVI r1, 0x0F0F       ; r1 = 0x0F0F
    MOV r2, r0
    AND r2, r1            ; r2 = 0xFF00 AND 0x0F0F = 0x0F00
    MOV r3, r0
    OR r3, r1             ; r3 = 0xFF00 OR 0x0F0F = 0xFF0F
    MOV r4, r0
    XOR r4, r1            ; r4 = 0xFF00 XOR 0x0F0F = 0xF00F
    MOV r5, r0
    SHL r5, 4             ; r5 = 0xFF00 SHL 4 = 0xFF000 (truncated to 32-bit = 0x000FF000)
    MOV r6, r0
    SHR r6, 4             ; r6 = 0xFF00 SHR 4 = 0x0FF0
    HALT
