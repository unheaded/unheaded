// SPDX-License-Identifier: GPL-3.0-or-later
//! What the eBPF object says about itself (ADR-080 §"I/O-time gates").
//!
//! `monad-cpu-ebpf` carries two data-only symbols in its `.rodata.upc`
//! section, emitted at build time and never read by any BPF code path:
//!
//! - `UPC_SURFACE_ID: [u8; 32]` — [`surface_id`](crate::surface_hash::surface_id)
//!   of the ecall numbers its `cfg(feature)` set compiles in
//!   (`ebpf/monad-cpu-ebpf/surface.rs`, hashed by its `build.rs`).
//! - `UPC_MMU_GEOMETRY: [u32; 5]` — `[base, stride, slice_len, ceiling,
//!   max_pids]` taken from the constants the interpreter's fork() and
//!   `user_phys()` use (`src/upc_facts.rs`).
//!
//! The loader reads them from the file it is about to load, before it creates
//! a single map, and [`validate_against_object`](crate::validate_against_object)
//! compares them with the [`Workload`](crate::Workload). The reader is a small
//! ELF64-little-endian section/symbol walk with every offset bounds-checked: the
//! object file is input, not trusted.

use crate::memory::SliceGeometry;

/// Section that holds the object's self-description.
pub const FACTS_SECTION: &str = ".rodata.upc";
/// Symbol of the 32-byte surface id.
pub const SURFACE_ID_SYMBOL: &str = "UPC_SURFACE_ID";
/// Symbol of the five-word slice geometry.
pub const MMU_GEOMETRY_SYMBOL: &str = "UPC_MMU_GEOMETRY";

/// The facts the gates compare a [`Workload`](crate::Workload) against.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct ObjectFacts {
    /// `UPC_SURFACE_ID`: the object's compiled-in ecall surface id.
    pub surface_id: [u8; 32],
    /// `UPC_MMU_GEOMETRY`, if the object carries one.
    pub mmu_geometry: Option<SliceGeometry>,
}

/// Why [`ObjectFacts::from_elf`] could not read an object.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ObjectFactsError {
    /// Not an ELF64 little-endian file.
    NotElf64Le,
    /// A header, table or symbol points outside the file or its section.
    Truncated(&'static str),
    /// No section header table or no `.symtab`.
    MissingSymtab,
    /// The named symbol is absent (an object built before the gates existed).
    MissingSymbol(&'static str),
    /// The named symbol is not in `.rodata.upc` or has the wrong size.
    BadSymbol(&'static str),
}

impl core::fmt::Display for ObjectFactsError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            ObjectFactsError::NotElf64Le => write!(f, "not an ELF64 little-endian object"),
            ObjectFactsError::Truncated(what) => write!(f, "object truncated or malformed: {what}"),
            ObjectFactsError::MissingSymtab => write!(f, "object has no symbol table"),
            ObjectFactsError::MissingSymbol(s) => write!(
                f,
                "object has no {s} symbol (built before the S-ASCEND-2 loader gates? rebuild monad-cpu-ebpf)"
            ),
            ObjectFactsError::BadSymbol(s) => {
                write!(f, "{s} is not a correctly sized symbol in {FACTS_SECTION}")
            }
        }
    }
}

impl std::error::Error for ObjectFactsError {}

const SHT_SYMTAB: u32 = 2;
const EHDR_LEN: usize = 64;
const SHDR_LEN: usize = 64;
const SYM_LEN: usize = 24;

fn get<'a>(
    b: &'a [u8],
    off: u64,
    len: u64,
    what: &'static str,
) -> Result<&'a [u8], ObjectFactsError> {
    let end = off
        .checked_add(len)
        .ok_or(ObjectFactsError::Truncated(what))?;
    if end > b.len() as u64 {
        return Err(ObjectFactsError::Truncated(what));
    }
    Ok(&b[off as usize..end as usize])
}

fn u16_at(b: &[u8], off: usize) -> u16 {
    u16::from_le_bytes([b[off], b[off + 1]])
}
fn u32_at(b: &[u8], off: usize) -> u32 {
    u32::from_le_bytes([b[off], b[off + 1], b[off + 2], b[off + 3]])
}
fn u64_at(b: &[u8], off: usize) -> u64 {
    let mut a = [0u8; 8];
    a.copy_from_slice(&b[off..off + 8]);
    u64::from_le_bytes(a)
}

#[derive(Clone, Copy)]
struct Shdr {
    name: u32,
    ty: u32,
    offset: u64,
    size: u64,
    link: u32,
}

/// A NUL-terminated name at `off` in a string table, without the NUL.
fn name_at(strtab: &[u8], off: u32) -> Option<&[u8]> {
    let rest = strtab.get(off as usize..)?;
    let n = rest.iter().position(|&c| c == 0)?;
    Some(&rest[..n])
}

impl ObjectFacts {
    /// Read `UPC_SURFACE_ID` (required) and `UPC_MMU_GEOMETRY` (optional) from
    /// the bytes of an eBPF object file.
    pub fn from_elf(b: &[u8]) -> Result<ObjectFacts, ObjectFactsError> {
        if b.len() < EHDR_LEN || &b[0..4] != b"\x7fELF" || b[4] != 2 || b[5] != 1 {
            return Err(ObjectFactsError::NotElf64Le);
        }
        let shoff = u64_at(b, 40);
        let shentsize = u16_at(b, 58) as u64;
        let shnum = u16_at(b, 60) as u64;
        let shstrndx = u16_at(b, 62) as u64;
        if shoff == 0 || shnum == 0 {
            return Err(ObjectFactsError::MissingSymtab);
        }
        if shentsize != SHDR_LEN as u64 || shstrndx >= shnum {
            return Err(ObjectFactsError::Truncated("section header table"));
        }
        let table = get(b, shoff, shnum * shentsize, "section header table")?;
        let shdrs: Vec<Shdr> = table
            .as_chunks::<SHDR_LEN>()
            .0
            .iter()
            .map(|h| Shdr {
                name: u32_at(h, 0),
                ty: u32_at(h, 4),
                offset: u64_at(h, 24),
                size: u64_at(h, 32),
                link: u32_at(h, 40),
            })
            .collect();
        let section = |i: usize, what: &'static str| -> Result<&[u8], ObjectFactsError> {
            let s = shdrs.get(i).ok_or(ObjectFactsError::Truncated(what))?;
            get(b, s.offset, s.size, what)
        };

        let shstrtab = section(shstrndx as usize, "section name table")?;
        let facts_idx = shdrs
            .iter()
            .position(|s| name_at(shstrtab, s.name) == Some(FACTS_SECTION.as_bytes()));
        let symtab_idx = shdrs
            .iter()
            .position(|s| s.ty == SHT_SYMTAB)
            .ok_or(ObjectFactsError::MissingSymtab)?;
        let symtab = section(symtab_idx, "symbol table")?;
        let strtab = section(shdrs[symtab_idx].link as usize, "symbol name table")?;

        // (st_shndx, st_value, st_size) of a named symbol, if present.
        let find = |want: &str| -> Option<(usize, u64, u64)> {
            symtab.as_chunks::<SYM_LEN>().0.iter().find_map(|s| {
                (name_at(strtab, u32_at(s, 0)) == Some(want.as_bytes()))
                    .then(|| (u16_at(s, 6) as usize, u64_at(s, 8), u64_at(s, 16)))
            })
        };
        // The bytes of a symbol that must live in .rodata.upc with `len` bytes.
        let read = |name: &'static str, len: u64| -> Result<Option<&[u8]>, ObjectFactsError> {
            let Some((shndx, value, size)) = find(name) else {
                return Ok(None);
            };
            if Some(shndx) != facts_idx || size != len {
                return Err(ObjectFactsError::BadSymbol(name));
            }
            let data = section(shndx, "facts section")?;
            get(data, value, size, "facts symbol").map(Some)
        };

        let id = read(SURFACE_ID_SYMBOL, 32)?
            .ok_or(ObjectFactsError::MissingSymbol(SURFACE_ID_SYMBOL))?;
        let mut surface_id = [0u8; 32];
        surface_id.copy_from_slice(id);

        let mmu_geometry = read(MMU_GEOMETRY_SYMBOL, 20)?.map(|g| {
            let mut w = [0u32; 5];
            for (i, v) in w.iter_mut().enumerate() {
                *v = u32_at(g, i * 4);
            }
            SliceGeometry::from_words(w)
        });

        Ok(ObjectFacts {
            surface_id,
            mmu_geometry,
        })
    }

    /// Read the facts from an object file on disk.
    pub fn from_path(
        path: &std::path::Path,
    ) -> Result<ObjectFacts, Box<dyn std::error::Error + Send + Sync>> {
        let bytes = std::fs::read(path)?;
        Ok(ObjectFacts::from_elf(&bytes)?)
    }
}

/// Lowercase hex of a 32-byte id, for messages.
pub fn hex32(id: &[u8; 32]) -> String {
    id.iter().map(|b| format!("{b:02x}")).collect()
}
