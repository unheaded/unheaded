
; Keyboard test - read key state via SYS_GET_KEY syscall
; Expected: r0 = 0x41 (65), r1 = 1 (pressed)
    MOVI r0, 2
    SYSCALL 2
    HALT
