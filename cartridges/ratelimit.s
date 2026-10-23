; SPDX-License-Identifier: GPL-3.0-or-later
; ratelimit.s — per-flow token-bucket rate limiter (S5 Phase 13, ADR-108).
;
; Correctness oracle: the token bucket in ebpf/qos-ebpf (refill-by-elapsed,
; fits-check; refill step in monad_common::token_bucket since 8ab534b3),
; reproduced exactly:
;   first packet of a flow: tokens = sat(cap - len), last = now, PASS
;   later packets:          q = elapsed_ns / 8000, add = q * rate_mbps
;                           if q > 0:
;                             add >= cap - tokens: tokens = cap, last = now
;                             else: tokens += add, last += q * 8000
;                                   (the elapsed mod 8000 remainder carries)
;                           tokens < len ? DROP : (tokens -= len, PASS)
; Tokens are bytes (qos-ebpf's unit), not the plan's x256 fixed point, so the
; verdicts can match the oracle bit for bit.
;
; 32-bit machine, no DIV:
; - elapsed is (now_lo - last_lo) mod 2^32, exact while the gap is under
;   2^32 ns. FIELD 7 (iat_bucket, from the full 64-bit clock) >= 22 means the
;   flow was idle >= 4.19 s: the bucket is then full, as in qos-ebpf, as long
;   as cap <= 524287 * rate_mbps (a parameter rule, checked by the tests).
; - elapsed / 8000 = MULH(elapsed >> 6, M) >> 3 with M = ceil(2^35 / 125):
;   exact for elapsed >> 6 < 2^26 (M*125 - 2^35 = 7 <= 2^9).
; - add = q * rate and q * 8000 fit 32 bits for rate_mbps <= 8000 (q < 2^32/8000).
; - last trails now by under 8000 ns plus the gap since the previous packet,
;   so elapsed stays under 2^32 while iat_bucket < 22.
;
; DATA (cartridges/ratelimit.data.bin, little-endian words):
;   0x1000 rate_mbps   (0 = limiter off, every packet passes)
;   0x1004 cap_bytes   (bucket capacity / burst)
;   0x1008 M           274877907
; Flow window: mem[0] tokens, mem[1] last_ns_lo, mem[2] 1 once initialised.
; Registers are zero at the start of every run, so r15 is the zero base.
;
; cart build --stage xdp --asm cartridges/ratelimit.s \
;            --data cartridges/ratelimit.data.bin -o ratelimit.cart

        LD   r5, [r15+0x1000]   ; rate_mbps
        MOVI r1, 4              ; FIELD pkt_len
        SYSCALL 0
        MOV  r4, r0             ; r4 = packet bytes (cost)
        CMP  r5, r15
        JZ   pass               ; rate 0: limiter off
        LD   r6, [r15+0x1004]   ; r6 = cap
        SYSCALL 1               ; NOW_NS_LO
        MOV  r7, r0             ; r7 = now_lo
        LD   r2, [r15+8]
        CMP  r2, r15
        JNZ  have
        ; first packet: tokens = saturating(cap - len), last = now
        MOV  r3, r6
        SUB  r3, r4             ; C = borrow (cap < len)
        JNC  first_ok
        MOVI r3, 0
first_ok:
        ST   [r15+0], r3
        ST   [r15+4], r7
        MOVI r2, 1
        ST   [r15+8], r2
        JMP  pass
have:
        LD   r3, [r15+0]        ; r3 = tokens
        MOVI r1, 7              ; FIELD iat_bucket
        SYSCALL 0
        MOVI r2, 22
        CMP  r0, r2
        JC   refill             ; idle < 4.19 s: 32-bit elapsed is exact
        MOV  r3, r6             ; idle >= 4.19 s: bucket full
        ST   [r15+4], r7
        JMP  check
refill:
        LD   r2, [r15+4]
        MOV  r8, r7
        SUB  r8, r2             ; elapsed = now_lo - last_lo
        SHR  r8, 6
        LD   r2, [r15+0x1008]
        MULH r8, r2
        SHR  r8, 3              ; q = elapsed / 8000
        MOV  r9, r8             ; keep q
        MUL  r8, r5             ; add = q * rate_mbps
        CMP  r8, r15
        JZ   check              ; q == 0: no refill, last unchanged
        MOV  r2, r6
        SUB  r2, r3             ; room = cap - tokens
        CMP  r8, r2
        JC   add_some
        MOV  r3, r6             ; add >= room: full, last = now
        ST   [r15+4], r7
        JMP  check
add_some:
        ADD  r3, r8
        MOVI r2, 8000
        MUL  r9, r2             ; q * 8000
        LD   r2, [r15+4]
        ADD  r2, r9
        ST   [r15+4], r2        ; last += q * 8000: the remainder carries
check:
        ST   [r15+0], r3        ; refilled tokens persist even on DROP
        CMP  r3, r4
        JC   drop               ; tokens < len
        SUB  r3, r4
        ST   [r15+0], r3
pass:
        MOVI r1, 0              ; VERDICT PASS
        SYSCALL 2
drop:
        MOVI r1, 1              ; VERDICT DROP
        SYSCALL 2
