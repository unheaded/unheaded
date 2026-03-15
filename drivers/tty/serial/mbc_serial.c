// SPDX-License-Identifier: GPL-2.0
/*
 * drivers/tty/serial/mbc_serial.c - MBC serial/console driver
 *
 * Maps to the UPC TTY_MAP BPF circular buffer for console output
 * and KBD_MAP for keyboard input.
 *
 *   Write path: SYS_WRITE to fd 1 -> TTY_MAP ring buffer
 *   Read path:  SYS_READ from fd 0 -> KBD_MAP ring buffer
 *
 * The driver registers an early boot console so pr_info() works
 * from the first line of start_kernel().
 */

#include <linux/console.h>
#include <linux/init.h>
#include <linux/module.h>
#include <linux/tty.h>
#include <linux/tty_driver.h>
#include <linux/serial_core.h>

#define MBC_TTY_MAJOR	204
#define MBC_TTY_MINOR	0
#define MBC_PORT_NAME	"ttyMBC"

/*
 * mbc_putchar - write a single character to TTY_MAP
 * @c: character to output
 *
 * Issues SYS_WRITE(fd=1, buf, len=1) via inline register setup.
 * The UPC compute engine traps the syscall and appends the byte
 * to the TTY_MAP circular buffer, which the BPF userspace reader
 * renders to the host terminal.
 *
 * NOTE: actual MBC inline asm syntax will replace the register
 * stubs once the MBC assembler emits relocatable objects.
 */
static void mbc_putchar(char c)
{
	/*
	 * Concept (not yet functional MBC asm):
	 *   MOVI r0, 4        ; SYS_WRITE
	 *   MOVI r1, 1        ; fd = stdout
	 *   LEA  r2, [sp-1]   ; buf = &c on stack
	 *   MOVI r3, 1        ; count = 1
	 *   INT  0x80          ; syscall
	 */
	register unsigned long r0 asm("r0") = 4;	/* SYS_WRITE */
	register unsigned long r1 asm("r1") = 1;	/* fd=stdout */

	(void)r0;
	(void)r1;
	(void)c;
}

/*
 * mbc_console_write - write a string to the MBC console
 * @co:    console descriptor
 * @s:     string to output
 * @count: number of bytes
 *
 * Iterates the buffer one character at a time, inserting CR
 * before each LF for proper terminal line discipline.
 */
static void mbc_console_write(struct console *co, const char *s,
			      unsigned int count)
{
	unsigned int i;

	for (i = 0; i < count; i++) {
		if (s[i] == '\n')
			mbc_putchar('\r');
		mbc_putchar(s[i]);
	}
}

static struct console mbc_console = {
	.name	= MBC_PORT_NAME,
	.write	= mbc_console_write,
	.flags	= CON_PRINTBUFFER | CON_BOOT,
	.index	= -1,
};

/*
 * mbc_console_init - register the MBC boot console
 *
 * Called via console_initcall() very early in boot, before most
 * subsystems are up.  This ensures kernel log messages reach
 * the UPC's TTY_MAP from the start.
 */
static int __init mbc_console_init(void)
{
	register_console(&mbc_console);
	pr_info("MBC: console registered on %s\n", MBC_PORT_NAME);
	return 0;
}
console_initcall(mbc_console_init);
