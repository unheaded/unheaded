; SPDX-License-Identifier: GPL-3.0-or-later
; counter.s — stateful test cartridge for the BPF == reference check (S5 Step 298).
; mem[0] += pkt_len, mem[1] += 1, mem[2..5] = dport, iat_bucket, flow_pkt_count,
; now_lo; EMIT(0x77, count, sum) every 4th packet; always PASS.
        MOVI r1, 4          ; FIELD pkt_len
        SYSCALL 0
        LD r2, [r15+0]
        ADD r2, r0
        ST [r15+0], r2
        LD r3, [r15+4]
        ADDI r3, 1
        ST [r15+4], r3
        MOVI r1, 3          ; FIELD dst_port
        SYSCALL 0
        ST [r15+8], r0
        MOVI r1, 7          ; FIELD iat_bucket
        SYSCALL 0
        ST [r15+12], r0
        MOVI r1, 8          ; FIELD flow_pkt_count
        SYSCALL 0
        ST [r15+16], r0
        SYSCALL 1           ; NOW_NS_LO
        ST [r15+20], r0
        MOV r4, r3
        MOVI r5, 3
        AND r4, r5
        JNZ done
        MOVI r1, 0x77
        MOV r2, r3
        LD r3, [r15+0]
        SYSCALL 3           ; EMIT
done:
        MOVI r1, 0
        SYSCALL 2           ; VERDICT PASS
