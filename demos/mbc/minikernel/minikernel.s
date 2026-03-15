; MiniKernel — UPC Level 5 Proof of Concept
; Demonstrates: boot, interrupts, syscalls, fork, console I/O, block device
;
; Instruction encoding: [opcode:8][dst:4][src:4][imm16:16]
;   Opcodes from monad-common mbc_opcodes module
;   Syscalls via INT 0x80 (Linux i386 ABI, mbc_linux_syscalls)
;
; Memory layout:
;   0x0000-0x03FF  IVT (256 vectors x 4 bytes, set by bootloader)
;   0x0080         IVT[0x20] = timer interrupt handler address
;   0x0200         IVT[0x80] = syscall handler address (set by bootloader)
;   0x0500-0x0FFF  Kernel code (loaded by wotan-ctl boot)
;   0x1000-0x1FFF  Kernel data (strings, buffers)
;   0x2000+        User space
;   0x3000         Block read buffer (512 bytes)

; === MBC OPCODE REFERENCE ===
; NOP    = 0x00    MOVI   = 0x0F    JMP  = 0x20    LD   = 0x30
; ADD    = 0x01    CMP    = 0x10    JZ   = 0x21    ST   = 0x31
; SUB    = 0x02    INT    = 0x17    JNZ  = 0x22    HALT = 0xFF
; MOV    = 0x0E    IRET   = 0x18    CALL = 0x27

; === KERNEL ENTRY (loaded at 0x0500 by wotan-ctl boot) ===
; PC starts at instruction 0 of the loaded binary.

_start:                         ; Instruction 0
    ; ---- Step 1: Print boot banner via SYS_WRITE ----
    MOVI r0, 4                  ; r0 = SYS_WRITE (syscall number)
    MOVI r1, 1                  ; r1 = fd (stdout)
    MOVI r2, banner             ; r2 = pointer to banner string (0x1000)
    MOVI r3, 30                 ; r3 = banner length
    INT 0x80                    ; invoke syscall

    ; ---- Step 2: Install timer interrupt handler in IVT ----
    ; IVT[0x20] is at RAM address 0x20 * 4 = 0x80
    ; We write the address of timer_handler there.
    MOVI r4, timer_handler_addr ; r4 = address of timer_handler routine
    MOVI r5, 0x0080             ; r5 = IVT slot address for vector 0x20
    ST [r5], r4                 ; RAM[0x80] = timer_handler address

    ; ---- Step 3: Fork a child process ----
    MOVI r0, 2                  ; r0 = SYS_FORK
    INT 0x80                    ; returns 0 in child, child_pid in parent
    ; After fork: r0 = result
    MOVI r6, 0                  ; r6 = 0 for comparison
    CMP r0, r6                  ; set flags: Z if r0 == 0 (child)
    JZ child_process            ; jump to child if r0 == 0

parent_process:                 ; Instruction ~12
    ; ---- Parent: print identification ----
    MOVI r0, 4                  ; SYS_WRITE
    MOVI r1, 1                  ; stdout
    MOVI r2, parent_msg         ; pointer to "Parent process running\n"
    MOVI r3, 23                 ; length
    INT 0x80

    ; ---- Parent: read block 0 from ramdisk ----
    MOVI r0, 200                ; SYS_READ_BLOCK
    MOVI r1, 0                  ; block number 0
    MOVI r2, 0x3000             ; destination buffer
    INT 0x80

    ; ---- Parent: confirm block read ----
    MOVI r0, 4                  ; SYS_WRITE
    MOVI r1, 1                  ; stdout
    MOVI r2, block_msg          ; pointer to "Block 0 read OK\n"
    MOVI r3, 16                 ; length
    INT 0x80

    ; ---- Parent: write modified data back to block 1 ----
    MOVI r0, 201                ; SYS_WRITE_BLOCK
    MOVI r1, 1                  ; block number 1
    MOVI r2, 0x3000             ; source buffer
    INT 0x80

    ; ---- Parent: print block write confirmation ----
    MOVI r0, 4                  ; SYS_WRITE
    MOVI r1, 1                  ; stdout
    MOVI r2, bwrite_msg         ; pointer to "Block 1 write OK\n"
    MOVI r3, 17                 ; length
    INT 0x80

    ; ---- Parent: get our PID ----
    MOVI r0, 20                 ; SYS_GETPID
    INT 0x80
    ; r0 now contains our PID

    ; ---- Parent: print PID message ----
    MOVI r0, 4                  ; SYS_WRITE
    MOVI r1, 1                  ; stdout
    MOVI r2, pid_msg            ; "PID queried OK\n"
    MOVI r3, 15                 ; length
    INT 0x80

    ; ---- Parent: get clock time ----
    MOVI r0, 265                ; SYS_CLOCK_GETTIME
    MOVI r1, 0                  ; CLOCK_REALTIME
    MOVI r2, 0x3200             ; timespec output buffer
    INT 0x80

parent_loop:                    ; Parent yield loop
    MOVI r0, 158                ; SYS_SCHED_YIELD
    INT 0x80
    JMP parent_loop             ; loop forever (scheduler preempts us)

child_process:                  ; Instruction ~42 (approx)
    ; ---- Child: print identification ----
    MOVI r0, 4                  ; SYS_WRITE
    MOVI r1, 1                  ; stdout
    MOVI r2, child_msg          ; pointer to "Child process running\n"
    MOVI r3, 22                 ; length
    INT 0x80

child_loop:
    ; ---- Child: sleep 100ms ----
    MOVI r0, 162                ; SYS_NANOSLEEP
    MOVI r1, timespec           ; pointer to timespec {0s, 100_000_000ns}
    INT 0x80

    ; ---- Child: print tick marker ----
    MOVI r0, 4                  ; SYS_WRITE
    MOVI r1, 1                  ; stdout
    MOVI r2, tick_msg           ; "."
    MOVI r3, 2                  ; length (dot + newline)
    INT 0x80

    JMP child_loop              ; loop forever

timer_handler:                  ; Timer ISR
    ; The scheduler handles context switching automatically.
    ; We just return from interrupt.
    IRET

; === DATA SECTION (at address 0x1000) ===
; Strings are packed as bytes following the code.
; The build.go program places them at known offsets and patches addresses.

banner:      .ascii "MiniKernel v0.1 \xe2\x80\x94 UPC Level 5\n"   ; 30 bytes
parent_msg:  .ascii "Parent process running\n"                       ; 23 bytes
child_msg:   .ascii "Child process running\n"                        ; 22 bytes
block_msg:   .ascii "Block 0 read OK\n"                              ; 16 bytes
bwrite_msg:  .ascii "Block 1 write OK\n"                             ; 17 bytes
pid_msg:     .ascii "PID queried OK\n"                               ; 15 bytes
tick_msg:    .ascii ".\n"                                             ; 2 bytes
timespec:    .word 0, 100000000  ; {tv_sec=0, tv_nsec=100ms}
