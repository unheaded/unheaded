/*
 * sum99.c -- Sum 1..99 = 4950
 *
 * Test program for the C -> RV32I -> MBC cross-compilation pipeline.
 * Computes sum(1..99) in register a0 (x10 -> MBC r8), then halts via EBREAK.
 *
 * Compiled with:
 *   riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32 -nostdlib -static -O2 \
 *     -ffreestanding -fno-builtin \
 *     -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19 \
 *     -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23 \
 *     -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27 \
 *     -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31 \
 *     -Wl,-Ttext=0x0 -o sum99.elf sum99.c
 *
 * Register mapping (RV32I -> MBC):
 *   x0  (zero) -> r0   x1  (ra)  -> r14   x2  (sp) -> r15
 *   x3  (gp)   -> r1   x4  (tp)  -> r2    x5  (t0) -> r3
 *   x6  (t1)   -> r4   x7  (t2)  -> r5    x8  (s0) -> r6
 *   x9  (s1)   -> r7   x10 (a0)  -> r8    x11 (a1) -> r9
 *   x12 (a2)   -> r10  x13 (a3)  -> r11   x14 (a4) -> r12
 *   x15 (a5)   -> r13
 *   x16-x31: UNSUPPORTED (restricted via -ffixed-xN)
 *
 * Expected result after execution:
 *   a0 (MBC r8) = 4950 (0x1356)
 *   Then HALT (via EBREAK -> MBC HALT opcode 0xFF)
 */

void _start(void) __attribute__((naked));

void _start(void) {
    /* Use inline asm to ensure we stay within x0-x15.
     * GCC's register allocator with -ffixed-xN should handle this,
     * but naked + pure asm gives us total control. */
    __asm__ volatile (
        /* a0 (x10) = sum accumulator, starts at 0 */
        "li     a0, 0\n"
        /* a1 (x11) = loop counter, starts at 1 */
        "li     a1, 1\n"
        /* a2 (x12) = loop limit = 100 */
        "li     a2, 100\n"

        /* Loop: sum += i; i++ */
        "1:\n"
        "add    a0, a0, a1\n"     /* sum += i           */
        "addi   a1, a1, 1\n"      /* i++                */
        "blt    a1, a2, 1b\n"     /* if i < 100, loop   */

        /* a0 now holds 4950 (0x1356) */
        /* EBREAK -> MBC HALT */
        "ebreak\n"
    );
}
