// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/syscall_table.c - MBC system call dispatch table
 *
 * Maps syscall numbers (r0 on INT 0x80) to kernel handler functions.
 * The entry.S syscall_entry code indexes into this table and calls
 * the handler with arguments in r1-r5.
 *
 * Syscall numbers are defined in asm/unistd.h and follow the
 * Linux/i386 convention where possible.  UPC-specific syscalls
 * (block I/O, MMU control) use numbers 200+.
 *
 * Unimplemented slots point to sys_ni_syscall which returns -ENOSYS.
 */

#include <linux/kernel.h>
#include <linux/syscalls.h>

#include <asm/unistd.h>

typedef long (*sys_call_fn)(unsigned long, unsigned long, unsigned long,
			    unsigned long, unsigned long);

/*
 * sys_ni_syscall - stub for unimplemented syscalls
 *
 * Returns -ENOSYS.  Any syscall number without a real handler
 * lands here.
 */
static long sys_ni_syscall(unsigned long a, unsigned long b,
			   unsigned long c, unsigned long d,
			   unsigned long e)
{
	return -38; /* -ENOSYS */
}

/*
 * Forward declarations for syscall handlers.
 *
 * These are implemented elsewhere in the kernel (fs/, kernel/, mm/).
 * We declare them here so the table can reference them without
 * pulling in every header.
 */
extern long sys_exit(int error_code);
extern long sys_fork(void);
extern long sys_read(unsigned int fd, char __user *buf, size_t count);
extern long sys_write(unsigned int fd, const char __user *buf, size_t count);
extern long sys_open(const char __user *filename, int flags, umode_t mode);
extern long sys_close(unsigned int fd);
extern long sys_waitpid(pid_t pid, int __user *stat_addr, int options);
extern long sys_execve(const char __user *filename,
		       const char __user *const __user *argv,
		       const char __user *const __user *envp);
extern long sys_chdir(const char __user *filename);
extern long sys_time(time_t __user *tloc);
extern long sys_lseek(unsigned int fd, off_t offset, unsigned int whence);
extern long sys_getpid(void);
extern long sys_setuid(uid_t uid);
extern long sys_getuid(void);
extern long sys_access(const char __user *filename, int mode);
extern long sys_sync(void);
extern long sys_kill(pid_t pid, int sig);
extern long sys_dup(unsigned int fildes);
extern long sys_pipe(int __user *fildes);
extern long sys_times(struct tms __user *tbuf);
extern long sys_brk(unsigned long brk);
extern long sys_setgid(gid_t gid);
extern long sys_getgid(void);
extern long sys_signal(int sig, void __user *handler);
extern long sys_geteuid(void);
extern long sys_getegid(void);
extern long sys_ioctl(unsigned int fd, unsigned int cmd, unsigned long arg);
extern long sys_fcntl(unsigned int fd, unsigned int cmd, unsigned long arg);
extern long sys_umask(int mask);
extern long sys_dup2(unsigned int oldfd, unsigned int newfd);
extern long sys_getppid(void);
extern long sys_stat(const char __user *filename, void __user *statbuf);
extern long sys_fstat(unsigned int fd, void __user *statbuf);
extern long sys_sched_yield(void);
extern long sys_nanosleep(void __user *rqtp, void __user *rmtp);
extern long sys_vfork(void);
extern long sys_clock_gettime(int which_clock, void __user *tp);

/* UPC-specific: block device I/O */
extern long sys_read_block(unsigned int dev, unsigned long block,
			   void __user *buf);
extern long sys_write_block(unsigned int dev, unsigned long block,
			    const void __user *buf);

/* UPC-specific: MMU control */
extern long sys_set_page_dir(unsigned long pgd_phys);
extern long sys_enable_mmu(int enable);
extern long sys_flush_tlb(void);

/*
 * System call table.
 *
 * Index 0 is reserved (returns -ENOSYS).  All unused slots between
 * defined syscalls are initialized to NULL by C default and caught
 * by the bounds check in entry.S; as extra safety the table is
 * sized to __NR_syscalls (48) entries, and entry.S also range-checks.
 *
 * Note: __NR_syscalls (48) only covers the "classic" range 0-47.
 * UPC-specific syscalls (200+) are dispatched via a separate check
 * in a future extension or via ioctl.  For now they are included
 * in the table by extending it to cover the highest defined number.
 */

#define TABLE_SIZE	265	/* >= max(__NR_clock_gettime=265) + 1 */

const sys_call_fn sys_call_table[TABLE_SIZE] = {
	/* 0: reserved */
	[0]                  = (sys_call_fn)sys_ni_syscall,

	/* Process management */
	[__NR_exit]          = (sys_call_fn)sys_exit,          /*  1 */
	[__NR_fork]          = (sys_call_fn)sys_fork,          /*  2 */
	[__NR_waitpid]       = (sys_call_fn)sys_waitpid,       /*  7 */
	[__NR_execve]        = (sys_call_fn)sys_execve,        /* 11 */
	[__NR_getpid]        = (sys_call_fn)sys_getpid,        /* 20 */
	[__NR_getppid]       = (sys_call_fn)sys_getppid,       /* 64 */
	[__NR_sched_yield]   = (sys_call_fn)sys_sched_yield,   /* 158 */
	[__NR_vfork]         = (sys_call_fn)sys_vfork,         /* 190 */

	/* File I/O */
	[__NR_read]          = (sys_call_fn)sys_read,          /*  3 */
	[__NR_write]         = (sys_call_fn)sys_write,         /*  4 */
	[__NR_open]          = (sys_call_fn)sys_open,          /*  5 */
	[__NR_close]         = (sys_call_fn)sys_close,         /*  6 */
	[__NR_chdir]         = (sys_call_fn)sys_chdir,         /* 12 */
	[__NR_lseek]         = (sys_call_fn)sys_lseek,         /* 19 */
	[__NR_access]        = (sys_call_fn)sys_access,        /* 33 */
	[__NR_sync]          = (sys_call_fn)sys_sync,          /* 36 */
	[__NR_dup]           = (sys_call_fn)sys_dup,           /* 41 */
	[__NR_pipe]          = (sys_call_fn)sys_pipe,          /* 42 */
	[__NR_fcntl]         = (sys_call_fn)sys_fcntl,         /* 55 */
	[__NR_dup2]          = (sys_call_fn)sys_dup2,          /* 63 */
	[__NR_stat]          = (sys_call_fn)sys_stat,          /* 106 */
	[__NR_fstat]         = (sys_call_fn)sys_fstat,         /* 108 */

	/* Memory */
	[__NR_brk]           = (sys_call_fn)sys_brk,           /* 45 */

	/* Signals */
	[__NR_kill]          = (sys_call_fn)sys_kill,          /* 37 */
	[__NR_signal]        = (sys_call_fn)sys_signal,        /* 48 */

	/* Identity */
	[__NR_setuid]        = (sys_call_fn)sys_setuid,        /* 23 */
	[__NR_getuid]        = (sys_call_fn)sys_getuid,        /* 24 */
	[__NR_setgid]        = (sys_call_fn)sys_setgid,        /* 46 */
	[__NR_getgid]        = (sys_call_fn)sys_getgid,        /* 47 */
	[__NR_geteuid]       = (sys_call_fn)sys_geteuid,       /* 49 */
	[__NR_getegid]       = (sys_call_fn)sys_getegid,       /* 50 */
	[__NR_umask]         = (sys_call_fn)sys_umask,         /* 60 */

	/* Time */
	[__NR_time]          = (sys_call_fn)sys_time,          /* 13 */
	[__NR_times]         = (sys_call_fn)sys_times,         /* 43 */
	[__NR_nanosleep]     = (sys_call_fn)sys_nanosleep,     /* 162 */
	[__NR_clock_gettime] = (sys_call_fn)sys_clock_gettime, /* 265 */

	/* Device */
	[__NR_ioctl]         = (sys_call_fn)sys_ioctl,         /* 54 */

	/* UPC-specific: block device I/O */
	[__NR_read_block]    = (sys_call_fn)sys_read_block,    /* 200 */
	[__NR_write_block]   = (sys_call_fn)sys_write_block,   /* 201 */

	/* UPC-specific: MMU control */
	[__NR_set_page_dir]  = (sys_call_fn)sys_set_page_dir,  /* 250 */
	[__NR_enable_mmu]    = (sys_call_fn)sys_enable_mmu,    /* 251 */
	[__NR_flush_tlb]     = (sys_call_fn)sys_flush_tlb,     /* 252 */
};

/*
 * NR_SYSCALLS_EFFECTIVE — actual table size for bounds checking.
 * Exported so entry.S can use it if needed (via extern).
 */
const unsigned long nr_syscalls_effective = TABLE_SIZE;
