; SPDX-License-Identifier: GPL-3.0-or-later
; pass.s — plumbing cartridge (S5 Step 293): every packet passes.
; cart build --stage xdp --asm cartridges/pass.s -o pass.cart
        MOVI r1, 0          ; verdict PASS
        SYSCALL 2           ; VERDICT
        HALT
