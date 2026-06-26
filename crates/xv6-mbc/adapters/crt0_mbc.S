/* SPDX-License-Identifier: GPL-3.0-or-later */
/*
 * crt0_mbc.S — minimal user-mode C runtime entry for xv6-on-UPC.
 *
 * exec()/userinit enter a user program at RV byte address 0. The UPC host
 * loader's PROGRAM_TABLE hard-codes `entry = RV word 0` (load_resident_program
 * in cmd/upc-bootctl), and the kernel sets the first user trapframe epc = 0.
 * Real xv6-riscv links user programs with `-e main`; this port links against
 * user.ld which has no ENTRY directive, so without a crt the *first function*
 * in the program object lands at address 0 and is run as the entry point.
 *
 * That is fine for programs whose first function happens to be main (init.c),
 * but breaks any program whose first source function is something else —
 * sh.c's first function is getcmd(), so exec jumped straight into getcmd,
 * which printed "$ ", then RET'd to ra (== 0 after exec) → PC=0 → full machine
 * reset (the Gate C post-`$` reboot loop).
 *
 * Placing _start in a dedicated section that user.ld emits first guarantees the
 * entry is _start for every program: call main, then exit. exit() never
 * returns, so ra is never followed back to 0.
 */
.section .text._start,"ax",@progbits
.globl _start
_start:
	call	main
	li	a0, 0
	call	exit
1:	j	1b		/* exit() does not return; trap any fallthrough */
