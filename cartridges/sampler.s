; SPDX-License-Identifier: GPL-3.0-or-later
; sampler.s — deterministic 1:N flow sampler + feature snapshot (S5 Phase 14,
; ADR-108). Always PASS. Never writes the Monad header (contrast anomaly-ebpf's
; SAMPLED-as-suspicious overload, KF11): samples leave as events and queue
; jobs only, and every value is header metadata (no payload).
;
; Per flow, over a window of N packets (N = DATA word 0), it accumulates the
; first four features; every Nth packet it fills in the other four, EMITs a
; sample event, QUEUEs the 8-word snapshot for the tick-stage scorer, and
; starts a new window. A flow's first packet also EMITs a first-seen event.
;
; Events (CART_EVENTS; flow key and 64-bit timestamp are in the record):
;   kind 0x46 'F'  first packet of the flow in this slot   a = pkt_len  b = 0
;   kind 0x53 'S'  every Nth packet                         a = flow_pkt_count
;                                                           b = flow_byte_count
; Feature snapshot = mem[0..8] (ScoreJob.feat, scorer input):
;   f0 win_bytes    sum of pkt_len over the window
;   f1 win_iat      sum of iat_bucket over the window
;   f2 win_syn      packets with TCP SYN set in the window
;   f3 win_max_len  largest pkt_len in the window
;   f4 flow_pkts    FIELD 8 at the sample
;   f5 l4_proto     FIELD 1 at the sample
;   f6 dst_port     FIELD 3 at the sample
;   f7 win_flags    OR of tcp_flags over the window
; mem[8] = packets in the current window, mem[9] non-zero once the flow was seen.
; Longest path (first packet that also completes a window, N = 1): 62 of 64.
;
; cart build --stage xdp --asm cartridges/sampler.s \
;            --data cartridges/sampler.data.bin -o sampler.cart

        MOVI r1, 4              ; FIELD pkt_len
        SYSCALL 0
        MOV  r4, r0
        LD   r2, [r15+36]       ; seen before?
        CMP  r2, r15
        JNZ  seen
        MOVI r1, 0x46           ; EMIT 'F' first-seen (r3 is still 0)
        ST   [r15+36], r1       ; seen flag = 0x46, non-zero
        MOV  r2, r4
        SYSCALL 3
seen:
        LD   r2, [r15+0]
        ADD  r2, r4
        ST   [r15+0], r2        ; f0 += len
        LD   r2, [r15+12]
        CMP  r2, r4
        JNC  nomax              ; max >= len
        ST   [r15+12], r4       ; f3 = len
nomax:
        MOVI r1, 7              ; FIELD iat_bucket
        SYSCALL 0
        LD   r2, [r15+4]
        ADD  r2, r0
        ST   [r15+4], r2        ; f1 += iat_bucket
        MOVI r1, 6              ; FIELD tcp_flags
        SYSCALL 0
        LD   r2, [r15+28]
        OR   r2, r0
        ST   [r15+28], r2       ; f7 |= flags
        SHL  r0, 30
        SHR  r0, 31             ; SYN bit (flags bit 1)
        LD   r2, [r15+8]
        ADD  r2, r0
        ST   [r15+8], r2        ; f2 += syn
        LD   r3, [r15+32]
        ADDI r3, 1
        LD   r2, [r15+0x1000]   ; N
        CMP  r3, r2
        JC   more               ; window not full yet
        MOVI r1, 8              ; FIELD flow_pkt_count
        SYSCALL 0
        ST   [r15+16], r0       ; f4
        MOV  r2, r0             ; EMIT a = flow_pkt_count
        MOVI r1, 1              ; FIELD l4_proto
        SYSCALL 0
        ST   [r15+20], r0       ; f5
        MOVI r1, 3              ; FIELD dst_port
        SYSCALL 0
        ST   [r15+24], r0       ; f6
        MOVI r1, 9              ; FIELD flow_byte_count
        SYSCALL 0
        MOV  r3, r0
        MOVI r1, 0x53           ; EMIT 'S' sample
        SYSCALL 3
        MOVI r1, 0              ; QUEUE mem[0..8]
        SYSCALL 4
        ST   [r15+0], r15       ; new window
        ST   [r15+4], r15
        ST   [r15+8], r15
        ST   [r15+12], r15
        ST   [r15+28], r15
        ST   [r15+32], r15
        MOVI r1, 0              ; VERDICT PASS
        SYSCALL 2
more:
        ST   [r15+32], r3
        MOVI r1, 0              ; VERDICT PASS
        SYSCALL 2
