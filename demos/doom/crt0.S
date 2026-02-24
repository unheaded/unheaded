/* crt0.S — MBC startup code for Doom-over-IPv6
 *
 * Minimal C runtime startup for bare-metal RISC-V → MBC execution:
 * 1. Set stack pointer to _stack_top
 * 2. Zero BSS section
 * 3. Call main()
 * 4. Halt on return
 *
 * Register usage (RV32I ABI, restricted to x0-x15):
 *   sp (x2) = stack pointer
 *   a0 (x10) = function args / return value
 *   a1 (x11) = function args
 *   ra (x1)  = return address
 */

    .section .text.startup
    .global _start
    .type _start, @function

_start:
    /* Set stack pointer */
    lui   sp, %hi(_stack_top)
    addi  sp, sp, %lo(_stack_top)

    /* Zero BSS: memset(__bss_start, 0, __bss_end - __bss_start) */
    lui   a0, %hi(__bss_start)
    addi  a0, a0, %lo(__bss_start)
    lui   a1, %hi(__bss_end)
    addi  a1, a1, %lo(__bss_end)
.Lbss_loop:
    bge   a0, a1, .Lbss_done
    sw    zero, 0(a0)
    addi  a0, a0, 4
    j     .Lbss_loop
.Lbss_done:

    /* Call main */
    jal   ra, main

    /* Halt (ebreak maps to MBC HALT) */
    ebreak

    .size _start, . - _start
