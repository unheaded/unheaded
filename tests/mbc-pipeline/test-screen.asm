; Screen pixel write test - write 4 individual pixel bytes to screen region
; SCREEN_BASE = 0xC000
; Pixels are 8-bit palette indices stored as bytes

    MOVI r4, 0xC000        ; r4 = screen base address

    ; Write pixel[0] = 0xFF (white)
    MOVI r5, 0xFF
    STB [r4+0], r5

    ; Write pixel[1] = 0x42 (palette index 66)
    MOVI r5, 0x42
    STB [r4+1], r5

    ; Write pixel[2] = 0xAA
    MOVI r5, 0xAA
    STB [r4+2], r5

    ; Write pixel[3] = 0x55
    MOVI r5, 0x55
    STB [r4+3], r5

    ; Read back the word at screen base to verify all 4 bytes
    LD r0, [r4+0]          ; r0 should = 0x55AA42FF (little-endian packing)
    HALT
