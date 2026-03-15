/* SPDX-License-Identifier: GPL-2.0 */
/*
 * arch/mbc/include/asm/unistd.h - MBC syscall number definitions
 *
 * Maps the 47 syscalls implemented by the UPC's MBC ISA.
 * Numbers follow the Linux/i386 convention where possible.
 * UPC-specific syscalls (block I/O, MMU) use numbers 200+.
 */
#ifndef _ASM_MBC_UNISTD_H
#define _ASM_MBC_UNISTD_H

/* Process management */
#define __NR_exit		1
#define __NR_fork		2
#define __NR_waitpid		7
#define __NR_execve		11
#define __NR_getpid		20
#define __NR_getppid		64
#define __NR_sched_yield	158
#define __NR_vfork		190

/* File I/O */
#define __NR_read		3
#define __NR_write		4
#define __NR_open		5
#define __NR_close		6
#define __NR_chdir		12
#define __NR_lseek		19
#define __NR_access		33
#define __NR_sync		36
#define __NR_dup		41
#define __NR_pipe		42
#define __NR_fcntl		55
#define __NR_dup2		63
#define __NR_stat		106
#define __NR_fstat		108

/* Memory */
#define __NR_brk		45

/* Signals */
#define __NR_kill		37
#define __NR_signal		48

/* Identity */
#define __NR_setuid		23
#define __NR_getuid		24
#define __NR_setgid		46
#define __NR_getgid		47
#define __NR_geteuid		49
#define __NR_getegid		50
#define __NR_umask		60

/* Time */
#define __NR_time		13
#define __NR_times		43
#define __NR_nanosleep		162
#define __NR_clock_gettime	265

/* Device */
#define __NR_ioctl		54

/* Block device (UPC-specific) */
#define __NR_read_block		200
#define __NR_write_block	201

/* MMU control (UPC-specific) */
#define __NR_set_page_dir	250
#define __NR_enable_mmu		251
#define __NR_flush_tlb		252

/*
 * __NR_syscalls covers the "classic" POSIX range (0-47) for the
 * bounds check in entry.S.  The actual syscall_table in
 * syscall_table.c is sized to TABLE_SIZE (266) to accommodate
 * UPC-specific and high-numbered Linux syscalls.
 */
#define __NR_syscalls		266

#endif /* _ASM_MBC_UNISTD_H */
