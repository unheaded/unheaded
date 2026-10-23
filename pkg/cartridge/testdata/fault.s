; SPDX-License-Identifier: GPL-3.0-or-later
; fault.s — reads 0x2000, outside both memory windows: every run faults
; (S5 Steps 296-297, fault path + breaker).
        MOVI r1, 0x2000
        LD r2, [r1+0]
        HALT
