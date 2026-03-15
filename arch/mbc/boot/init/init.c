/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/boot/init/init.c - Minimal /init for MBC Linux
 *
 * This is the C reference implementation for the init process
 * (PID 1).  It is compiled to MBC via RV32I transpilation or
 * assembled directly with the MBC assembler.  The actual binary
 * used for boot is generated from init.S by build.go.
 *
 * Sequence:
 *   1. Print "MBC Linux init starting\n" to console
 *   2. Fork a child process
 *   3. Child: execve("/bin/sh") — drops into the shell
 *   4. Parent: waitpid(-1) loop — respawns shell on exit
 */

/* Syscall stubs — these would be linked against a libc.
 * Here they serve as documentation of the calling convention. */

static inline long syscall1(long nr, long a1)
{
	/* r0=nr, r1=a1, INT 0x80, return r0 */
	long ret;
	__asm__ volatile(
		"MOVI r0, %1\n"
		"MOV  r1, %2\n"
		"INT  0x80\n"
		"MOV  %0, r0\n"
		: "=r"(ret) : "i"(nr), "r"(a1) : "r0", "r1"
	);
	return ret;
}

#define SYS_EXIT    1
#define SYS_FORK    2
#define SYS_WRITE   4
#define SYS_WAITPID 7
#define SYS_EXECVE  11

void _start(void)
{
	const char banner[] = "MBC Linux init starting\n";
	const char child_msg[] = "init: child exited\n";
	const char *sh_path = "/bin/sh";
	long pid;

	/* 1. Print banner */
	/* write(1, banner, sizeof(banner)-1) */
	__asm__ volatile(
		"MOVI r0, 4\n"
		"MOVI r1, 1\n"
		"MOVI r2, %0\n"
		"MOVI r3, 24\n"
		"INT  0x80\n"
		: : "r"(banner)
	);

	for (;;) {
		/* 2. Fork */
		__asm__ volatile("MOVI r0, 2; INT 0x80; MOV %0, r0"
				 : "=r"(pid));

		if (pid == 0) {
			/* 3. Child: exec /bin/sh */
			__asm__ volatile(
				"MOVI r0, 11\n"
				"MOVI r1, %0\n"
				"INT  0x80\n"
				: : "r"(sh_path)
			);
			/* exec failed — exit */
			__asm__ volatile("MOVI r0, 1; MOVI r1, 1; INT 0x80");
			__builtin_unreachable();
		}

		/* 4. Parent: waitpid(-1) */
		__asm__ volatile(
			"MOVI r0, 7\n"
			"MOVI r1, -1\n"
			"MOVI r2, 0\n"
			"MOVI r3, 0\n"
			"INT  0x80\n"
		);

		/* Print child-exited message, then respawn */
		__asm__ volatile(
			"MOVI r0, 4\n"
			"MOVI r1, 1\n"
			"MOVI r2, %0\n"
			"MOVI r3, 20\n"
			"INT  0x80\n"
			: : "r"(child_msg)
		);
	}
}
