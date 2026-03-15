; SPDX-License-Identifier: GPL-2.0
;
; arch/mbc/boot/init/init.S — /init for MBC Linux (PID 1)
;
; First userspace process.  Prints a banner, then enters an
; infinite fork-exec-wait loop that keeps /bin/sh alive.
;
; Syscall convention (INT 0x80):
;   r0 = syscall number
;   r1-r5 = arguments
;   r0 = return value
;
; MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]

.equ SYS_EXIT,    1
.equ SYS_FORK,    2
.equ SYS_WRITE,   4
.equ SYS_WAITPID, 7
.equ SYS_EXECVE, 11

.text
.global _start

_start:
    ; ── Print boot banner ──────────────────────────────────
    MOVI r0, SYS_WRITE         ; syscall: write
    MOVI r1, 1                 ; fd = stdout
    MOVI r2, banner            ; buf = banner string
    MOVI r3, 24                ; count = 24 bytes
    INT  0x80

spawn_shell:
    ; ── Fork ───────────────────────────────────────────────
    MOVI r0, SYS_FORK          ; syscall: fork
    INT  0x80

    ; Check return: r0 == 0 → child, r0 > 0 → parent
    MOVI r6, 0
    CMP  r0, r6
    JZ   child                 ; child process

    ; ── Parent: wait for child to exit ─────────────────────
parent_wait:
    MOVI r0, SYS_WAITPID       ; syscall: waitpid
    MOVI r1, -1                ; pid = -1 (any child)
    MOVI r2, 0                 ; status ptr = NULL
    MOVI r3, 0                 ; options = 0
    INT  0x80

    ; Print "init: child exited" message
    MOVI r0, SYS_WRITE
    MOVI r1, 1
    MOVI r2, child_exit_msg
    MOVI r3, 20
    INT  0x80

    ; Respawn shell
    JMP  spawn_shell

child:
    ; ── Child: exec /bin/sh ────────────────────────────────
    MOVI r0, SYS_EXECVE        ; syscall: execve
    MOVI r1, sh_path           ; filename = "/bin/sh"
    MOVI r2, 0                 ; argv = NULL
    MOVI r3, 0                 ; envp = NULL
    INT  0x80

    ; If execve fails, exit with error
    MOVI r0, SYS_EXIT
    MOVI r1, 1                 ; exit code = 1
    INT  0x80
    HLT                        ; should never reach

; ── Data section ───────────────────────────────────────────
.data
banner:         .ascii "MBC Linux init starting\n"
child_exit_msg: .ascii "init: child exited\n\0"
sh_path:        .ascii "/bin/sh\0"
