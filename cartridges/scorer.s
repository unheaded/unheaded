; SPDX-License-Identifier: GPL-3.0-or-later
; scorer.s — int8 MLP anomaly scorer, tick stage (S5 Phase 15, ADR-108 D-C6).
;
; Input: a ScoreJob from the sampler (cartridges/sampler.s): mem[0..8] = the
; 8-word feature snapshot. Output: EMIT kind 0x41 'A', a = score 0..100,
; b = model id; then HALT. Model and quantization: crates/upc-cartridge
; src/scorer.rs (QModel::score is the bit-exact integer reference); weights
; from `trainquant` (cartridges/scorer.data.bin + scorer.model.json).
;
;   x_k   = min(f_k >> sh_k, 255)                         k = 0..7
;   acc_j = b1_j + sum_k (w1_jk + 128) * x_k - 128 * sum_k x_k
;   h_j   = min(max(acc_j, 0) >> s_j, 255)                j = 0..15
;   z     = b2 + sum_j w2_j * h_j
;   score = LUT[clamp((z >> so) + 128, 0, 255)]
;
; First-layer weights are stored biased (+128, u8) so one LDB reads each;
; the -128 * sum(x) correction is computed once per job. No DIV, no MULHU,
; no new opcode: a DOT4 would fold the 24-instruction inner product of each
; row into ~4 (ADR-108 non-goal; recorded, not added).
;
; DATA (byte offsets from 0x1000): 0x00 sh_k (8 x u8); 0x08 model id (u32);
; 0x0C so (u8); 0x10 b2 (i32); 0x14 + 16*j row j: b1_j (i32), w1_j0..7 + 128
; (u8), w2_j (i8), s_j (u8), 2 pad; 0x114 LUT (256 x u8).
;
; Registers: r4..r11 = x0..x7, r14 = 128 * sum(x), r15 = 255, r3 = row
; pointer, r12 = rows left, r13 = z, r1/r2/r0 scratch. All registers are 0
; when a job starts (begin_job), so r3 is the zero base for the prologue.
; Loop: one back-edge per row; declared loop bound 16 (cart build
; --loop-bound 16). ~700 instructions per job: 3 cart_tick calls of 256.
;
; cart build --stage tick --loop-bound 16 --asm cartridges/scorer.s \
;            --data cartridges/scorer.data.bin -o scorer.cart

        MOVI r15, 255
        LD   r4, [r3+0]
        LDB  r0, [r3+0x1000]
        SHRR r4, r0
        CMP  r4, r15
        JC   x0
        MOV  r4, r15
x0:
        ADD  r14, r4
        LD   r5, [r3+4]
        LDB  r0, [r3+0x1001]
        SHRR r5, r0
        CMP  r5, r15
        JC   x1
        MOV  r5, r15
x1:
        ADD  r14, r5
        LD   r6, [r3+8]
        LDB  r0, [r3+0x1002]
        SHRR r6, r0
        CMP  r6, r15
        JC   x2
        MOV  r6, r15
x2:
        ADD  r14, r6
        LD   r7, [r3+12]
        LDB  r0, [r3+0x1003]
        SHRR r7, r0
        CMP  r7, r15
        JC   x3
        MOV  r7, r15
x3:
        ADD  r14, r7
        LD   r8, [r3+16]
        LDB  r0, [r3+0x1004]
        SHRR r8, r0
        CMP  r8, r15
        JC   x4
        MOV  r8, r15
x4:
        ADD  r14, r8
        LD   r9, [r3+20]
        LDB  r0, [r3+0x1005]
        SHRR r9, r0
        CMP  r9, r15
        JC   x5
        MOV  r9, r15
x5:
        ADD  r14, r9
        LD   r10, [r3+24]
        LDB  r0, [r3+0x1006]
        SHRR r10, r0
        CMP  r10, r15
        JC   x6
        MOV  r10, r15
x6:
        ADD  r14, r10
        LD   r11, [r3+28]
        LDB  r0, [r3+0x1007]
        SHRR r11, r0
        CMP  r11, r15
        JC   x7
        MOV  r11, r15
x7:
        ADD  r14, r11
        SHL  r14, 7             ; 128 * sum(x)
        MOVI r3, 0x1014         ; row 0
        MOVI r12, 16
row:
        LD   r1, [r3+0]         ; b1_j (rounding folded)
        LDB  r2, [r3+4]
        MUL  r2, r4
        ADD  r1, r2
        LDB  r2, [r3+5]
        MUL  r2, r5
        ADD  r1, r2
        LDB  r2, [r3+6]
        MUL  r2, r6
        ADD  r1, r2
        LDB  r2, [r3+7]
        MUL  r2, r7
        ADD  r1, r2
        LDB  r2, [r3+8]
        MUL  r2, r8
        ADD  r1, r2
        LDB  r2, [r3+9]
        MUL  r2, r9
        ADD  r1, r2
        LDB  r2, [r3+10]
        MUL  r2, r10
        ADD  r1, r2
        LDB  r2, [r3+11]
        MUL  r2, r11
        ADD  r1, r2
        SUB  r1, r14            ; acc_j (N = negative)
        JP   relu
        MOVI r1, 0
relu:
        LDB  r0, [r3+13]
        SHRR r1, r0             ; requantize
        CMP  r1, r15
        JC   hok
        MOV  r1, r15
hok:
        LDB  r2, [r3+12]
        SHL  r2, 24
        SAR  r2, 24             ; w2_j, sign-extended
        MUL  r2, r1
        ADD  r13, r2
        ADDI r3, 16
        ADDI r12, -1
        JNZ  row
        LD   r1, [r3-260]     ; b2 (0x1010; r3 = 0x1114 = LUT)
        ADD  r13, r1
        LDB  r0, [r3-264]     ; so (0x100C)
        SARR r13, r0
        ADDI r13, 128
        JP   lo
        MOVI r13, 0
lo:
        CMP  r13, r15
        JC   hi
        MOV  r13, r15
hi:
        ADD  r13, r3
        LDB  r2, [r13+0]        ; score = LUT[index]
        LD   r3, [r3-268]     ; model id (0x1008)
        MOVI r1, 0x41           ; EMIT 'A'
        SYSCALL 3
        HALT
