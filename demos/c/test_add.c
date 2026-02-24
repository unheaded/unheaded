// test_add.c — Prove the cross-compile pipeline works (C → RV32I → MBC)
// Expected: return value = 4950 (sum of 0..99)
//
// This is the simplest possible C program that exercises the pipeline:
// - No libc
// - No extern references
// - Pure computation, no I/O
// - Result in a0 (return register)

int _start(void) {
    int result = 0;
    for (int i = 0; i < 100; i++) {
        result += i;
    }
    // Return value ends up in a0 (register 10 in RISC-V ABI)
    return result; // 4950
}
