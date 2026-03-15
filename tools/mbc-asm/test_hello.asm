; Hello World for UPC
; Demonstrates MBC assembler with syscall-based I/O
.text
_start:
    MOVI r0, 4          ; SYS_WRITE
    MOVI r1, 1          ; fd = stdout
    MOVI r2, msg        ; buf address
    MOVI r3, 13         ; length
    INT  0x80           ; syscall
    MOVI r0, 1          ; SYS_EXIT
    MOVI r1, 0          ; exit code
    INT  0x80
    HLT

.data
msg:
    .ascii "Hello, UPC!\n"
