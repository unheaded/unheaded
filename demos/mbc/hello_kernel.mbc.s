; SPDX-License-Identifier: GPL-3.0-or-later
; hello_kernel.mbc.s — Minimal UPC kernel demo
;
; This is a trivial MBC program that demonstrates the UPC boot protocol.
; When loaded at 0x10000 by the boot loader, it:
;   1. Writes "Hello from UPC kernel!" to TTY via SYSCALL SYS_WRITE
;   2. Halts the CPU
;
; Register conventions:
;   r0  = syscall number / return value
;   r1  = arg1 (fd for write)
;   r2  = arg2 (buffer address for write)
;   r3  = arg3 (length for write)
;   r15 = SP (stack pointer)
;
; Syscall interface (INT 0x80):
;   r0 = 4 (SYS_WRITE)
;   r1 = 1 (stdout fd)
;   r2 = address of string
;   r3 = length of string
;
; Memory layout when booted:
;   0x10000: kernel code (this file)
;   0x10100: string data "Hello from UPC kernel!\n"
;
; Instruction encoding: [opcode:8][dst:4][src:4][imm:16]
;   MOVI  = 0x0F (dst, imm16)
;   INT   = 0x40 (imm16 = vector)
;   HLT   = 0xFF
;
; Assembled instructions (6 words):
;   0x0F000004  MOVI r0, 4       ; SYS_WRITE
;   0x0F100001  MOVI r1, 1       ; fd = stdout
;   0x0F200100  MOVI r2, 0x0100  ; offset to string (relative, kernel adds base)
;   0x0F300017  MOVI r3, 23      ; length = 23 ("Hello from UPC kernel!\n")
;   0x40000080  SYSCALL 0x80     ; INT 0x80
;   0xFF000000  HLT              ; halt CPU
;
; String data at offset 0x100 (word offset 0x40 from kernel base):
;   "Hello from UPC kernel!\n" (23 bytes, padded to 24)
