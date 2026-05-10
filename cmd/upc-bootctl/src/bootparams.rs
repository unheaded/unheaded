// SPDX-License-Identifier: GPL-3.0-or-later
//! BootParams v2 encoder + memory-layout constants for UPC Boot Protocol v2.
//! See `docs/doom/UPC_BOOT_PROTOCOL_V2.md` (frozen 2026-05-08 per ADR-067).

#![allow(dead_code)] // some constants reserved for Phase 2+ uClinux work

/// Magic literal 'UNHD' as little-endian u32 (= 0x554E_4844).
pub const BOOT_MAGIC: u32 = 0x554E_4844;
/// BootParams structure version. v1=48B (Doom), v2=256B (xv6 + Linux).
pub const BOOT_VERSION: u32 = 2;

// ── Byte addresses (per UPC_BOOT_PROTOCOL_V2.md §"Memory Layout") ─────────
pub const ADDR_IVT: u32 = 0x0000;
pub const ADDR_BOOTPARAMS: u32 = 0x0100;
pub const ADDR_CMDLINE: u32 = 0x0200;
pub const ADDR_TRAP_HANDLER: u32 = 0x0400;
pub const ADDR_CSR_REGION: u32 = 0xF000;
/// Stage-1 / xv6 entry per kernel-mbc.ld (`.stage1 0x00010000`).
pub const ADDR_KERNEL_DEFAULT: u32 = 0x10000;
pub const ADDR_RAMDISK_DEFAULT: u32 = 0x800000;
pub const ADDR_STACK_TOP: u32 = 0x03F0_0000;

// ── Sizes ─────────────────────────────────────────────────────────────────
pub const SIZE_IVT: u32 = 1024; // 256 vectors × 4 bytes
pub const SIZE_BOOTPARAMS: u32 = 256;
pub const SIZE_CMDLINE_MAX: u32 = 512;
pub const SIZE_CSR_REGION: u32 = 256; // 64 CSRs × 4 bytes

/// BootParams v2 — exact 256-byte layout per spec.
///
/// Field order MUST match `crates/xv6-mbc/adapters/start_mbc.c::struct
/// boot_params_v2`. The kernel reads from byte address 0x100; any drift
/// here breaks boot.
#[repr(C, packed)]
#[derive(Clone, Copy)]
pub struct BootParamsV2 {
    // === v1 fields, preserved ===
    pub magic: u32,        // 'UNHD'
    pub version: u32,      // 2
    pub memory_size: u32,
    pub ramdisk_addr: u32,
    pub ramdisk_size: u32,
    pub kernel_addr: u32,
    pub kernel_size: u32,
    pub boot_args_addr: u32,
    pub boot_args_len: u32,
    pub num_cpus: u32,
    pub tick_rate_hz: u32,
    // === v2 new fields ===
    pub bss_start: u32,
    pub bss_end: u32,
    pub initrd2_addr: u32,
    pub cmd_line_args_ptr: u32,
    pub boot_random_seed: u32,
    // === forward-compat reservation ===
    pub reserved: [u32; 48], // zero-filled
}

const _: () = assert!(std::mem::size_of::<BootParamsV2>() == 256);

impl BootParamsV2 {
    /// Construct BootParams for an xv6 first-boot. .bss is left at 0/0 —
    /// xv6's start_mbc.c does not consume the bss_start/end fields and
    /// xv6's main() runs before any .bss-dependent allocator. (Phase 1.1
    /// SHIP decision per `crates/xv6-mbc/TRANSLATOR_EXTENSIONS.md`.)
    pub fn for_xv6(kernel_size_bytes: u32, ramdisk_size_bytes: u32) -> Self {
        Self {
            magic: BOOT_MAGIC,
            version: BOOT_VERSION,
            memory_size: 64 * 1024 * 1024, // 64 MB default
            ramdisk_addr: ADDR_RAMDISK_DEFAULT,
            ramdisk_size: ramdisk_size_bytes,
            kernel_addr: ADDR_KERNEL_DEFAULT,
            kernel_size: kernel_size_bytes,
            boot_args_addr: ADDR_CMDLINE,
            boot_args_len: 0,
            num_cpus: 1,        // Phase 1.1 = single-CPU per ADR-067 §3
            tick_rate_hz: 12,
            bss_start: 0,       // unused for xv6 first-boot gate
            bss_end: 0,
            initrd2_addr: 0,    // no second initramfs
            cmd_line_args_ptr: 0,
            boot_random_seed: 0, // entropy provision deferred to Phase 2
            reserved: [0u32; 48],
        }
    }

    /// Serialize to the 256-byte wire form expected at byte address 0x100.
    /// SAFETY: BootParamsV2 is `#[repr(C, packed)]`, exactly 256 bytes,
    /// contains only u32/[u32;N] primitives — no padding, no pointers,
    /// no Drop, fully POD.
    pub fn to_bytes(&self) -> [u8; 256] {
        unsafe { std::mem::transmute_copy(self) }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bootparams_size_is_exactly_256() {
        assert_eq!(std::mem::size_of::<BootParamsV2>(), 256);
    }

    #[test]
    fn for_xv6_sets_magic_version_addresses() {
        let bp = BootParamsV2::for_xv6(46_884, 0);
        assert_eq!({ bp.magic }, 0x554E_4844);
        assert_eq!({ bp.version }, 2);
        assert_eq!({ bp.kernel_addr }, 0x10000);
        assert_eq!({ bp.kernel_size }, 46_884);
        assert_eq!({ bp.num_cpus }, 1);
        assert_eq!({ bp.bss_start }, 0);
        assert_eq!({ bp.tick_rate_hz }, 12);
    }

    #[test]
    fn to_bytes_first_four_bytes_are_magic_le() {
        let bp = BootParamsV2::for_xv6(0, 0);
        let bytes = bp.to_bytes();
        // Intentional pin per ADR-072: canonical hex 0x554E4844 ('UNHD'
        // MSB-first) serializes as little-endian to D,H,N,U at increasing
        // byte offsets. Wire bytes match `docs/doom/UPC_BOOT_PROTOCOL_V2.md`
        // §"Field semantics" magic field.
        assert_eq!(&bytes[0..4], &[0x44, 0x48, 0x4E, 0x55]);
    }

    #[test]
    fn reserved_is_zeroed_for_forward_compat() {
        let bp = BootParamsV2::for_xv6(123, 456);
        let bytes = bp.to_bytes();
        // reserved[48] starts at offset 256-(48*4)=64
        assert!(
            bytes[64..256].iter().all(|&b| b == 0),
            "reserved must be all zero for forward-compat"
        );
    }
}
