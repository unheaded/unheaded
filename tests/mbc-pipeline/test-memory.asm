; Memory test — store 0xDEAD to address 0x100, load it back
; Expected: r0 = 0xDEAD (57005), halted = true
    MOVI r1, 0x100    ; address = 0x100 (256)
    MOVI r2, 0xDEAD   ; value = 0xDEAD (57005)
    ST [r1+0], r2     ; ram[0x100] = 0xDEAD
    LD r0, [r1+0]     ; r0 = ram[0x100]
    HALT
