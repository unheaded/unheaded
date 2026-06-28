// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Phase 1.7 Gate D — pure xv6 `fs.img` read-only walker (ASCEND-LINUX).
//!
//! The UPC loads the xv6 filesystem image at byte `0x00800000`
//! (`mbc_block::RAMDISK_BASE_WORD`). Gate D's `open`/`read`/`fstat` walk it
//! **directly** from `RAM_MAP` inside the eBPF CPU — they do NOT go through the
//! `blk-ramdisk`/`SYS_READ_BLOCK` sector path.
//!
//! ## THE #1 TRAP — block size
//!
//! xv6 `kernel/fs.h` defines `BSIZE = 1024`; `mkfs` includes that header, so
//! `fs.img` is laid out in **1024-byte** blocks ([`FS_WORDS_PER_BLOCK`] = 256).
//! The `mbc_block` constants ([`crate::mbc_block::BLOCK_SIZE`] = 512,
//! `WORDS_PER_BLOCK` = 128) are the UPC *sector* abstraction used by
//! `blk-ramdisk.c` — using them here would halve every block address and read
//! garbage. [`fs_words_per_block_is_256`](tests) guards this.
//!
//! ## Layout (all little-endian, as `RAM_MAP` packs `fs.img` bytes)
//!
//! ```text
//! [ boot | super(block 1) | log | inode blocks | bitmap | data ]
//! superblock words: magic,size,nblocks,ninodes,nlog,logstart,inodestart,bmapstart
//! dinode (64B/16w): w0=type|major<<16, w1=minor|nlink<<16, w2=size, w3..w15=addrs[0..12]
//! dirent (16B/4w):  w0 low16 = inum (0 = free), bytes 2..15 = name[14] (no NUL guarantee)
//! ```
//!
//! ## Hardening (BlackMage) — `RAM_MAP` is the whole 64 MiB phys space, so an
//! unchecked block number is an arbitrary kernel-memory read. Every block
//! number is bounded to the ramdisk region by [`word_of_block`] (H-FS1); the
//! eBPF caller bounds `inum` (H-FS2), verifies the superblock magic (H-FS3),
//! and the single-indirect index is capped here (H-FS4). All functions are pure
//! so the arithmetic is unit-tested off-target via
//! `cd crates/monad-mbc && cargo test -p monad-common`.

use crate::mbc_block;

/// xv6 block size in bytes (`kernel/fs.h` `BSIZE`).
pub const BSIZE: u32 = 1024;
/// Words per fs block (1024 / 4) — **256**, NOT `mbc_block::WORDS_PER_BLOCK`.
pub const FS_WORDS_PER_BLOCK: u32 = BSIZE / 4;
/// `sizeof(struct dinode)` in words (64 bytes / 4).
pub const DINODE_WORDS: u32 = 16;
/// Inodes per block (`BSIZE / sizeof(dinode)` = 1024 / 64).
pub const IPB: u32 = BSIZE / 64;
/// Direct block pointers in a dinode (`kernel/fs.h` `NDIRECT`).
pub const NDIRECT: usize = 12;
/// Entries in the single indirect block (`BSIZE / sizeof(uint)` = 256).
pub const NINDIRECT: u32 = BSIZE / 4;
/// Root inode number (`kernel/fs.h` `ROOTINO`).
pub const ROOTINO: u32 = 1;
/// Filesystem magic (`kernel/fs.h` `FSMAGIC`).
pub const FSMAGIC: u32 = 0x1020_3040;
/// Max directory-entry name length (`kernel/fs.h` `DIRSIZ`); names are NOT
/// guaranteed NUL-terminated when exactly `DIRSIZ` long.
pub const DIRSIZ: usize = 14;
/// `sizeof(struct dirent)` in bytes (`ushort inum` + `char name[14]`).
pub const DIRENT_BYTES: u32 = 16;
/// Directory entries per block (1024 / 16).
pub const DIRENTS_PER_BLOCK: u32 = BSIZE / DIRENT_BYTES;
/// Block index of the superblock (block 0 is the unused boot block).
pub const SUPERBLOCK_BLOCK: u32 = 1;
/// Number of 1024-byte fs blocks that fit in the ramdisk region (H-FS1 ceiling).
pub const FS_TOTAL_BLOCKS: u32 = mbc_block::RAMDISK_SIZE / BSIZE;

/// xv6 inode `type` (`kernel/stat.h`): directory.
pub const T_DIR: u16 = 1;
/// xv6 inode `type`: regular file.
pub const T_FILE: u16 = 2;
/// xv6 inode `type`: device node.
pub const T_DEVICE: u16 = 3;

/// `RAM_MAP` word address of fs block `b`, or `None` if `b` escapes the ramdisk
/// region (**H-FS1** — prevents an out-of-range block number in a hostile
/// `fs.img` from reading arbitrary kernel memory).
#[inline(always)]
pub fn word_of_block(b: u32) -> Option<u32> {
    if b >= FS_TOTAL_BLOCKS {
        return None;
    }
    Some(mbc_block::RAMDISK_BASE_WORD + b * FS_WORDS_PER_BLOCK)
}

// ── Superblock (the 8 u32 words at block 1) ────────────────────────────────
/// Superblock word 0: filesystem magic (compare to [`FSMAGIC`]).
#[inline(always)]
pub fn superblock_magic(sb: &[u32]) -> u32 {
    sb.first().copied().unwrap_or(0)
}
/// Superblock word 3: total inode count (`ninodes`).
#[inline(always)]
pub fn superblock_ninodes(sb: &[u32]) -> u32 {
    sb.get(3).copied().unwrap_or(0)
}
/// Superblock word 6: block number of the first inode block (`inodestart`).
#[inline(always)]
pub fn superblock_inodestart(sb: &[u32]) -> u32 {
    sb.get(6).copied().unwrap_or(0)
}
/// **H-FS3**: a walk may proceed only against a superblock with the xv6 magic.
#[inline(always)]
pub fn superblock_valid(sb: &[u32]) -> bool {
    superblock_magic(sb) == FSMAGIC
}

/// **H-FS2**: a usable inode number is in `[ROOTINO, ninodes)`.
#[inline(always)]
pub fn inum_valid(inum: u32, ninodes: u32) -> bool {
    inum >= ROOTINO && inum < ninodes
}

/// Block holding inode `inum` (`IBLOCK`). Caller bounds `inum` via [`inum_valid`].
#[inline(always)]
pub fn inode_block(inum: u32, inodestart: u32) -> u32 {
    inum / IPB + inodestart
}
/// Word offset of inode `inum` within its block.
#[inline(always)]
pub fn dinode_word_offset(inum: u32) -> u32 {
    (inum % IPB) * DINODE_WORDS
}

/// Decoded subset of a `struct dinode` (type, link count, size, block addrs).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Dinode {
    /// File type: [`T_DIR`], [`T_FILE`], or [`T_DEVICE`].
    pub type_: u16,
    /// Hard-link count.
    pub nlink: u16,
    /// File size in bytes.
    pub size: u32,
    /// Block addresses: `[0..NDIRECT)` direct, `[NDIRECT]` single indirect.
    pub addrs: [u32; NDIRECT + 1],
}

/// Decode the 16 words of a dinode. Returns `None` if fewer than
/// [`DINODE_WORDS`] words are provided.
#[inline(always)]
pub fn decode_dinode(words: &[u32]) -> Option<Dinode> {
    if words.len() < DINODE_WORDS as usize {
        return None;
    }
    let type_ = (words[0] & 0xFFFF) as u16;
    let nlink = (words[1] >> 16) as u16;
    let size = words[2];
    let mut addrs = [0u32; NDIRECT + 1];
    let mut i = 0;
    while i <= NDIRECT {
        addrs[i] = words[3 + i];
        i += 1;
    }
    Some(Dinode {
        type_,
        nlink,
        size,
        addrs,
    })
}

/// File-relative block number for byte offset `off`.
#[inline(always)]
pub fn file_block_index(off: u32) -> u32 {
    off / BSIZE
}

/// Resolve the fs block holding file-block `fbn` of an inode.
///
/// `addrs` is `dinode.addrs` (13 entries); `indirect` is the [`NINDIRECT`]
/// words of the single indirect block (`addrs[NDIRECT]`), or an empty slice if
/// the file is small enough not to need it. Returns `None` on a sparse hole
/// (block 0), on `fbn` beyond the single-indirect range (**H-FS4**), or when
/// the indirect slice is too short.
#[inline(always)]
pub fn block_for_fbn(addrs: &[u32; NDIRECT + 1], indirect: &[u32], fbn: u32) -> Option<u32> {
    if (fbn as usize) < NDIRECT {
        let b = addrs[fbn as usize];
        if b == 0 {
            return None;
        }
        return Some(b);
    }
    let idx = fbn - NDIRECT as u32;
    if idx >= NINDIRECT {
        return None; // H-FS4: xv6 has only single indirect
    }
    let b = *indirect.get(idx as usize)?;
    if b == 0 {
        return None;
    }
    Some(b)
}

/// `addrs[NDIRECT]` — the block number of the single indirect block (0 = none).
#[inline(always)]
pub fn indirect_block(addrs: &[u32; NDIRECT + 1]) -> u32 {
    addrs[NDIRECT]
}

// ── Directory entries ──────────────────────────────────────────────────────
/// `inum` of directory entry `idx` within a directory data block (`block_words`
/// = the [`FS_WORDS_PER_BLOCK`] words of the block). `0` = free slot. Returns
/// `None` if `idx` is past the block.
#[inline(always)]
pub fn dirent_inum(block_words: &[u32], idx: u32) -> Option<u16> {
    let w = (idx * DIRENT_BYTES / 4) as usize; // 4 words per 16-byte dirent
    Some((block_words.get(w)?.clone() & 0xFFFF) as u16)
}

/// 14-byte name of directory entry `idx`, NUL-padded (NOT NUL-terminated when
/// exactly [`DIRSIZ`] long). Returns `None` if the entry runs past the block.
#[inline(always)]
pub fn dirent_name(block_words: &[u32], idx: u32) -> Option<[u8; DIRSIZ]> {
    let name_byte0 = (idx * DIRENT_BYTES + 2) as usize; // name starts at byte 2 of the record
    let mut name = [0u8; DIRSIZ];
    let mut i = 0;
    while i < DIRSIZ {
        let bidx = name_byte0 + i;
        let word = *block_words.get(bidx / 4)?;
        name[i] = ((word >> ((bidx % 4) * 8)) & 0xFF) as u8;
        i += 1;
    }
    Some(name)
}

/// Compare a dirent name against a target basename. Both are treated as
/// NUL-terminated-or-`DIRSIZ`-long (**H-FS8** — compares at most [`DIRSIZ`]
/// bytes, never walks past the fixed field).
#[inline(always)]
pub fn name_eq(name: &[u8; DIRSIZ], target: &[u8]) -> bool {
    let mut i = 0;
    while i < DIRSIZ {
        let t = if i < target.len() { target[i] } else { 0 };
        if name[i] != t {
            return false;
        }
        if t == 0 {
            return true; // both terminate here
        }
        i += 1;
    }
    // Consumed all DIRSIZ bytes with no mismatch: match iff target has no more.
    target.len() <= DIRSIZ
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── THE #1 TRAP guard ──────────────────────────────────────────────────
    #[test]
    fn fs_words_per_block_is_256_not_the_sector_constant() {
        assert_eq!(
            FS_WORDS_PER_BLOCK, 256,
            "fs block is 1024 bytes = 256 words"
        );
        assert_eq!(BSIZE, 1024);
        assert_ne!(
            FS_WORDS_PER_BLOCK,
            mbc_block::WORDS_PER_BLOCK,
            "must NOT reuse the 512-byte sector constant (128 words)"
        );
        assert_eq!(FS_TOTAL_BLOCKS, 4096, "4 MiB / 1024");
    }

    #[test]
    fn block_to_word_address() {
        assert_eq!(word_of_block(0), Some(mbc_block::RAMDISK_BASE_WORD));
        assert_eq!(word_of_block(1), Some(mbc_block::RAMDISK_BASE_WORD + 256));
        assert_eq!(word_of_block(2), Some(mbc_block::RAMDISK_BASE_WORD + 512));
    }

    #[test]
    fn word_of_block_rejects_out_of_range_h_fs1() {
        assert_eq!(word_of_block(FS_TOTAL_BLOCKS), None);
        assert_eq!(word_of_block(FS_TOTAL_BLOCKS + 1), None);
        assert_eq!(word_of_block(0xFFFF_FFFF), None);
    }

    #[test]
    fn inode_block_and_offset() {
        // 16 inodes per block. inum 1 lives in block `inodestart`, offset 16.
        assert_eq!(IPB, 16);
        assert_eq!(inode_block(1, 32), 32);
        assert_eq!(dinode_word_offset(1), 16);
        assert_eq!(inode_block(16, 32), 33);
        assert_eq!(dinode_word_offset(16), 0);
        assert_eq!(inode_block(17, 32), 33);
        assert_eq!(dinode_word_offset(17), 16);
    }

    #[test]
    fn inum_bounds_h_fs2() {
        assert!(!inum_valid(0, 200)); // 0 is never a real inode
        assert!(inum_valid(1, 200));
        assert!(inum_valid(199, 200));
        assert!(!inum_valid(200, 200)); // == ninodes is out of range
        assert!(!inum_valid(0xFFFF_FFFF, 200));
    }

    #[test]
    fn superblock_magic_gate_h_fs3() {
        let mut sb = [0u32; 8];
        sb[0] = FSMAGIC;
        sb[3] = 200; // ninodes
        sb[6] = 45; // inodestart
        assert!(superblock_valid(&sb));
        assert_eq!(superblock_ninodes(&sb), 200);
        assert_eq!(superblock_inodestart(&sb), 45);
        sb[0] = 0xDEAD_BEEF;
        assert!(!superblock_valid(&sb), "garbage magic must fail all opens");
    }

    #[test]
    fn decode_dinode_packs_fields() {
        let mut w = [0u32; 16];
        w[0] = (T_FILE as u32) | (0u32 << 16); // type=FILE, major=0
        w[1] = 0u32 | (3u32 << 16); // minor=0, nlink=3
        w[2] = 1500; // size
        w[3] = 41; // addrs[0]
        w[3 + NDIRECT] = 77; // addrs[12] = indirect
        let d = decode_dinode(&w).unwrap();
        assert_eq!(d.type_, T_FILE);
        assert_eq!(d.nlink, 3);
        assert_eq!(d.size, 1500);
        assert_eq!(d.addrs[0], 41);
        assert_eq!(indirect_block(&d.addrs), 77);
    }

    #[test]
    fn decode_dinode_rejects_short_slice() {
        assert_eq!(decode_dinode(&[0u32; 15]), None);
    }

    #[test]
    fn file_block_index_boundaries() {
        assert_eq!(file_block_index(0), 0);
        assert_eq!(file_block_index(1023), 0);
        assert_eq!(file_block_index(1024), 1);
        assert_eq!(file_block_index(13 * 1024), 13);
    }

    #[test]
    fn block_for_fbn_direct_and_hole() {
        let mut addrs = [0u32; NDIRECT + 1];
        addrs[5] = 99;
        let none: [u32; 0] = [];
        assert_eq!(block_for_fbn(&addrs, &none, 5), Some(99));
        assert_eq!(block_for_fbn(&addrs, &none, 3), None, "sparse hole = None");
    }

    #[test]
    fn block_for_fbn_indirect_and_h_fs4() {
        let addrs = [0u32; NDIRECT + 1];
        let mut indirect = [0u32; NINDIRECT as usize];
        indirect[0] = 500;
        indirect[(NINDIRECT - 1) as usize] = 999;
        assert_eq!(block_for_fbn(&addrs, &indirect, NDIRECT as u32), Some(500));
        assert_eq!(
            block_for_fbn(&addrs, &indirect, NDIRECT as u32 + NINDIRECT - 1),
            Some(999)
        );
        // One past the single-indirect range → None (xv6 has no double indirect).
        assert_eq!(
            block_for_fbn(&addrs, &indirect, NDIRECT as u32 + NINDIRECT),
            None
        );
    }

    /// Build a directory block with entries ".", "README" (inum 7), and a free
    /// slot, then resolve README — the end-to-end dirent path.
    #[test]
    fn dirent_parse_and_resolve() {
        let mut blk = [0u32; FS_WORDS_PER_BLOCK as usize];
        write_dirent(&mut blk, 0, 1, b"."); // self
        write_dirent(&mut blk, 1, 7, b"README"); // target
        write_dirent(&mut blk, 2, 0, b""); // free slot

        assert_eq!(dirent_inum(&blk, 0), Some(1));
        assert!(name_eq(&dirent_name(&blk, 0).unwrap(), b"."));

        // Resolve README by scanning, skipping free slots (inum==0).
        let mut found = None;
        let mut idx = 0;
        while idx < DIRENTS_PER_BLOCK {
            let inum = dirent_inum(&blk, idx).unwrap();
            if inum != 0 && name_eq(&dirent_name(&blk, idx).unwrap(), b"README") {
                found = Some(inum);
                break;
            }
            idx += 1;
        }
        assert_eq!(found, Some(7));
    }

    #[test]
    fn name_eq_edges() {
        let mut n = [0u8; DIRSIZ];
        n[..4].copy_from_slice(b"init");
        assert!(name_eq(&n, b"init"));
        assert!(!name_eq(&n, b"in")); // prefix must not match
        assert!(!name_eq(&n, b"init2"));
        // Exactly-14-char name with no NUL terminator.
        let full = *b"abcdefghijklmn"; // 14 bytes
        assert!(name_eq(&full, b"abcdefghijklmn"));
        assert!(!name_eq(&full, b"abcdefghijklm")); // 13 != 14
    }

    // Test helper: write a `{inum, name}` dirent into block slot `idx` (LE).
    fn write_dirent(blk: &mut [u32], idx: u32, inum: u16, name: &[u8]) {
        let base = (idx * DIRENT_BYTES) as usize; // byte offset
        let mut bytes = [0u8; DIRENT_BYTES as usize];
        bytes[0] = (inum & 0xFF) as u8;
        bytes[1] = (inum >> 8) as u8;
        let n = core::cmp::min(name.len(), DIRSIZ);
        bytes[2..2 + n].copy_from_slice(&name[..n]);
        let mut i = 0;
        while i < DIRENT_BYTES as usize {
            let w = (base + i) / 4;
            let sh = ((base + i) % 4) * 8;
            blk[w] = (blk[w] & !(0xFFu32 << sh)) | ((bytes[i] as u32) << sh);
            i += 1;
        }
    }
}
