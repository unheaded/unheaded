// SPDX-License-Identifier: GPL-3.0-only
//! Memory layout — THE single source of truth for the Doom-over-IPv6 address space.
//!
//! Every other component (linker.ld, monad-common mbc_mmap, libc_stubs.c, doom-loader)
//! must agree with these constants. When in doubt, THIS file wins.
//!
//! # Address Map (flat 32-bit physical)
//!
//! ```text
//! 0x0000_0000 ┬─────────────────────── ROM (1 MiB) — .text + .rodata
//!             │  MBC instructions (Array<u32>, index = PC)
//! 0x0010_0000 ┼─────────────────────── RAM_BASE
//!             │  .data + .sdata         (from ELF, loaded at link addresses)
//!             │  .bss  + .sbss          (zeroed)
//! 0x0016_8000 │  KBD_ADDR              (keyboard I/O word)
//! 0x0017_0000 │  SCREEN_BASE           (320x200, 8-bit palette)
//! 0x0017_FA00 │  (screen end)
//! 0x001C_0000 ┼─────────────────────── HEAP_START
//!             │  Doom Z_Init zone       (10 MiB — needs >= 6 MiB)
//! 0x00BC_0000 ┼─────────────────────── HEAP_END
//! 0x01C0_0000 ┼─────────────────────── WAD_BASE
//!             │  doom1.wad              (up to 4 MiB)
//! 0x0100_0000 ┼─────────────────────── WAD end / RAM ceiling
//!             │
//! 0x00F0_0000 │  DEBUG_BASE            (debug scratch region)
//! 0x0100_0000 ┼─────────────────────── STACK_TOP (grows down)
//! ```
//!
//! # BPF Map Sizing
//!
//! RAM_MAP is `Array<u32>` with word-addressed entries:
//! - 16 MiB address space = 4M words = `Array::with_max_entries(4_194_304)`
//! - Kernel memory: 4M * 4 bytes = 16 MiB (feasible on modern kernels)
//!
//! ROM_MAP is `Array<u32>` with one entry per MBC instruction:
//! - 262_144 entries = 1 MiB instruction space (sufficient for Doom)

/// ROM region: MBC instructions loaded from .text via rv2mbc translation.
pub const ROM_BASE: u32 = 0x0000_0000;
pub const ROM_SIZE: u32 = 0x0010_0000; // 1 MiB

/// ROM_MAP max entries (one u32 per MBC instruction slot).
pub const ROM_MAP_ENTRIES: u32 = 262_144;

/// RAM region start (data, bss, screen, heap, WAD, stack).
pub const RAM_BASE: u32 = 0x0010_0000;

/// Total RAM address space: 16 MiB.
pub const RAM_SIZE: u32 = 0x0100_0000; // 16 MiB

/// RAM_MAP max entries: must cover from address 0 to STACK_TOP.
/// STACK_TOP = RAM_BASE + RAM_SIZE = 0x0110_0000, so we need
/// 0x0110_0000 / 4 = 4_456_448 entries.
/// The eBPF program uses 16_777_216 entries (64 MiB kernel memory)
/// which is more than enough. We use the same value for compatibility.
pub const RAM_MAP_ENTRIES: u32 = 16_777_216;

/// Keyboard I/O word address (single u32: scancode << 1 | pressed).
pub const KBD_ADDR: u32 = 0x0016_8000;

/// Screen framebuffer base: 320x200 pixels, 8-bit palette indices.
pub const SCREEN_BASE: u32 = 0x0007_0000;
pub const SCREEN_WIDTH: u32 = 320;
pub const SCREEN_HEIGHT: u32 = 200;
pub const SCREEN_SIZE: u32 = SCREEN_WIDTH * SCREEN_HEIGHT; // 64_000 bytes

/// SCREEN_MAP max entries (one u8 per pixel).
pub const SCREEN_MAP_ENTRIES: u32 = SCREEN_SIZE;

/// Heap region for Doom's Z_Init zone allocator.
/// 26 MiB: Doom pre-Z_Init allocations use ~10MB, Z_Init needs ~3MB more.
pub const HEAP_START: u32 = 0x001C_0000;
pub const HEAP_END: u32 = 0x01BC_0000;
pub const HEAP_SIZE: u32 = HEAP_END - HEAP_START; // 26 MiB

/// Isolated heap_ptr address — lives far from .data to avoid wild pointer corruption.
/// libc_stubs.c defines: #define HEAP_PTR_ADDR ((char **)0x01BF0000)
/// doom-runner writes HEAP_START to this address on load.
pub const HEAP_PTR_ADDR: u32 = 0x01BF_0000;

/// Actual WAD file size — written by doom-runner at load time.
/// libc_stubs.c open() reads from this address instead of using a hardcoded constant.
pub const WAD_SIZE_ADDR: u32 = 0x01BF_FFF0;

/// WAD data region (memory-mapped into RAM).
/// Retail DOOM.WAD = 12,408,292 bytes (~12.4 MiB). Shareware doom1.wad = 4,196,020 bytes.
pub const WAD_BASE: u32 = 0x01C0_0000;
pub const WAD_MAX_SIZE: u32 = 0x0100_0000; // 16 MiB (covers retail DOOM.WAD with headroom)

/// Stack top (grows downward). Must be above WAD_BASE + WAD_MAX_SIZE.
pub const STACK_TOP: u32 = 0x0310_0000; // 0x1C00000 + 16M + 1M gap

/// Debug scratch region.
pub const DEBUG_BASE: u32 = 0x00F0_0000;

/// CPU_MAP max entries (one MbcCpuState per instance, 256 instances).
pub const CPU_MAP_ENTRIES: u32 = 256;

/// KBD_MAP max entries.
pub const KBD_MAP_ENTRIES: u32 = 8;

/// RV2MBC_MAP max entries (RV32I word index -> MBC PC index).
pub const RV2MBC_MAP_ENTRIES: u32 = 65_536;

// ── Compatibility with monad-common mbc_mmap ────────────────────────────────

/// Validate that our layout is internally consistent.
/// Called at startup to catch misconfigurations before loading anything.
pub fn validate_layout() -> Result<(), LayoutError> {
    // Heap must start after screen ends
    let screen_end = SCREEN_BASE + SCREEN_SIZE;
    if HEAP_START < screen_end {
        return Err(LayoutError::Overlap {
            region_a: "screen",
            region_b: "heap",
            a_end: screen_end,
            b_start: HEAP_START,
        });
    }

    // WAD must start after heap ends
    if WAD_BASE < HEAP_END {
        return Err(LayoutError::Overlap {
            region_a: "heap",
            region_b: "wad",
            a_end: HEAP_END,
            b_start: WAD_BASE,
        });
    }

    // WAD must fit before stack top
    let wad_end = WAD_BASE + WAD_MAX_SIZE;
    if wad_end > STACK_TOP {
        return Err(LayoutError::Overlap {
            region_a: "wad",
            region_b: "stack",
            a_end: wad_end,
            b_start: STACK_TOP,
        });
    }

    // RAM_MAP must be large enough for the entire address space
    let max_word_addr = STACK_TOP / 4;
    if max_word_addr > RAM_MAP_ENTRIES {
        return Err(LayoutError::MapTooSmall {
            map_name: "RAM_MAP",
            needed: max_word_addr,
            actual: RAM_MAP_ENTRIES,
        });
    }

    Ok(())
}

/// Convert a byte address to a RAM_MAP word index.
/// Returns None if the address is outside the RAM region.
#[inline]
pub fn byte_addr_to_word_index(addr: u32) -> Option<u32> {
    let idx = addr / 4;
    if idx < RAM_MAP_ENTRIES {
        Some(idx)
    } else {
        None
    }
}

/// Errors from layout validation.
#[derive(Debug, Clone)]
pub enum LayoutError {
    Overlap {
        region_a: &'static str,
        region_b: &'static str,
        a_end: u32,
        b_start: u32,
    },
    MapTooSmall {
        map_name: &'static str,
        needed: u32,
        actual: u32,
    },
}

impl std::fmt::Display for LayoutError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            LayoutError::Overlap {
                region_a,
                region_b,
                a_end,
                b_start,
            } => write!(
                f,
                "memory layout overlap: {region_a} ends at {a_end:#010x} \
                 but {region_b} starts at {b_start:#010x}"
            ),
            LayoutError::MapTooSmall {
                map_name,
                needed,
                actual,
            } => write!(
                f,
                "BPF map too small: {map_name} has {actual} entries but needs {needed}"
            ),
        }
    }
}

impl std::error::Error for LayoutError {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn layout_is_valid() {
        validate_layout().expect("default memory layout must be valid");
    }

    #[test]
    fn heap_is_26mb() {
        assert_eq!(HEAP_SIZE, 26 * 1024 * 1024);
    }

    #[test]
    fn wad_region_is_16mb() {
        assert_eq!(WAD_MAX_SIZE, 16 * 1024 * 1024); // 16MB for retail DOOM.WAD
    }

    #[test]
    fn ram_map_covers_full_address_space() {
        // RAM_MAP must cover every word address up to STACK_TOP
        assert!(
            RAM_MAP_ENTRIES >= STACK_TOP / 4,
            "RAM_MAP_ENTRIES ({}) must be >= STACK_TOP/4 ({})",
            RAM_MAP_ENTRIES,
            STACK_TOP / 4
        );
    }

    #[test]
    fn screen_size_is_320x200() {
        assert_eq!(SCREEN_SIZE, 64_000);
    }

    #[test]
    fn byte_addr_conversion() {
        assert_eq!(byte_addr_to_word_index(0), Some(0));
        assert_eq!(byte_addr_to_word_index(0x100), Some(0x40));
        // STACK_TOP is well within the map
        assert_eq!(
            byte_addr_to_word_index(STACK_TOP - 4),
            Some(STACK_TOP / 4 - 1)
        );
        // Last valid word in the map
        assert_eq!(
            byte_addr_to_word_index((RAM_MAP_ENTRIES - 1) * 4),
            Some(RAM_MAP_ENTRIES - 1)
        );
        // Past the end of RAM_MAP
        assert_eq!(byte_addr_to_word_index(RAM_MAP_ENTRIES * 4), None);
    }
}
