; SPDX-License-Identifier: GPL-3.0-or-later
; drop-udp-9.s — plumbing cartridge (S5 Step 293): drop UDP to port 9
; (discard), pass everything else.
; cart build --stage xdp --asm cartridges/drop-udp-9.s -o drop-udp-9.cart
        MOVI r1, 1          ; FIELD l4_proto
        SYSCALL 0
        MOVI r2, 17         ; UDP
        CMP r0, r2
        JNZ pass
        MOVI r1, 3          ; FIELD dst_port
        SYSCALL 0
        MOVI r2, 9
        CMP r0, r2
        JNZ pass
        MOVI r1, 1          ; verdict DROP
        SYSCALL 2
pass:
        MOVI r1, 0          ; verdict PASS
        SYSCALL 2
        HALT
