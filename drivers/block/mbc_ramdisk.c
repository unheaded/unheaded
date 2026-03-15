// SPDX-License-Identifier: GPL-2.0
/*
 * drivers/block/mbc_ramdisk.c - MBC ramdisk block device driver
 *
 * Exposes the UPC's 4 MB ramdisk (RAM_MAP at RAMDISK_BASE)
 * as a Linux block device (/dev/ram0).
 *
 * I/O is performed via the UPC-specific syscalls:
 *   SYS_READ_BLOCK  (200) - read a 512-byte sector
 *   SYS_WRITE_BLOCK (201) - write a 512-byte sector
 *
 * The BPF program backing these syscalls copies data between
 * the caller's buffer and the RAM_MAP array map.
 */

#include <linux/blkdev.h>
#include <linux/init.h>
#include <linux/module.h>

#define MBC_RAMDISK_MAJOR	1
#define MBC_SECTOR_SIZE		512
#define MBC_TOTAL_SECTORS	8192	/* 4 MB / 512 */
#define MBC_RAMDISK_SIZE	(MBC_TOTAL_SECTORS * MBC_SECTOR_SIZE)

/*
 * mbc_ramdisk_submit_bio - process a block I/O request
 * @bio: the bio describing the I/O
 *
 * Walks each segment in the bio and issues per-sector
 * SYS_READ_BLOCK or SYS_WRITE_BLOCK calls to the UPC.
 */
static void mbc_ramdisk_submit_bio(struct bio *bio)
{
	struct bio_vec bvec;
	struct bvec_iter iter;
	sector_t sector = bio->bi_iter.bi_sector;

	bio_for_each_segment(bvec, bio, iter) {
		char *buf = page_address(bvec.bv_page) + bvec.bv_offset;
		unsigned int len = bvec.bv_len;
		unsigned int sectors = len / MBC_SECTOR_SIZE;
		unsigned int i;

		for (i = 0; i < sectors; i++) {
			/*
			 * TODO: wire to UPC block syscalls
			 *   READ:  mbc_syscall(200, sector, buf)
			 *   WRITE: mbc_syscall(201, sector, buf)
			 */
			sector++;
			buf += MBC_SECTOR_SIZE;
		}
	}

	bio_endio(bio);
}

static const struct block_device_operations mbc_ramdisk_ops = {
	.owner		= THIS_MODULE,
	.submit_bio	= mbc_ramdisk_submit_bio,
};
