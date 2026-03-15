// SPDX-License-Identifier: GPL-2.0
/*
 * arch/mbc/kernel/board.c - MBC platform/board initialization
 *
 * Registers platform devices (console, ramdisk, timer) so that
 * the corresponding drivers can bind via the platform bus.
 *
 * Week 6 of S-LINUX: platform device registration.
 */

#include <linux/init.h>
#include <linux/platform_device.h>
#include <linux/ioport.h>

/* Console device — matched by drivers/tty/serial/mbc_console.c */
static struct platform_device mbc_console_device = {
	.name	= "mbc-console",
	.id	= 0,
};

/* Ramdisk device — matched by drivers/block/mbc_ramdisk.c */
static struct resource mbc_ramdisk_resources[] = {
	{
		.start	= 0x00800000,		/* RAMDISK_BASE */
		.end	= 0x00BFFFFF,		/* + 4MB */
		.flags	= IORESOURCE_MEM,
	},
};

static struct platform_device mbc_ramdisk_device = {
	.name		= "mbc-ramdisk",
	.id		= 0,
	.num_resources	= ARRAY_SIZE(mbc_ramdisk_resources),
	.resource	= mbc_ramdisk_resources,
};

/* Timer device — matched by arch/mbc/kernel/time.c */
static struct platform_device mbc_timer_device = {
	.name	= "mbc-timer",
	.id	= 0,
};

static struct platform_device *mbc_devices[] __initdata = {
	&mbc_console_device,
	&mbc_ramdisk_device,
	&mbc_timer_device,
};

static int __init mbc_board_init(void)
{
	return platform_add_devices(mbc_devices, ARRAY_SIZE(mbc_devices));
}
arch_initcall(mbc_board_init);
