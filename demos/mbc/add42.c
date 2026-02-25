/*
 * add42.c -- Minimal test: compute 40 + 2 = 42
 *
 * The simplest possible C program for the cross-compilation pipeline.
 * Puts 42 in a0 (MBC r8) and halts.
 *
 * Expected result: a0 (MBC r8) = 42
 */

void _start(void) __attribute__((naked));

void _start(void) {
    __asm__ volatile (
        "li     a0, 40\n"          /* a0 = 40            */
        "li     a1, 2\n"           /* a1 = 2             */
        "add    a0, a0, a1\n"      /* a0 = 40 + 2 = 42  */
        "ebreak\n"                 /* HALT               */
    );
}
