// SPDX-License-Identifier: GPL-3.0-or-later
//! Host side of MBC cartridges (ADR-108, Cartridge ABI v0).
//!
//! - [`Image`]: the `.cart` file format (encode / decode). Decoding treats the
//!   bytes as hostile: every size is range-checked from the header before the
//!   length is trusted, and the embedded sha256 must match.
//! - [`validate`]: what the loader refuses before a single word reaches a
//!   BPF map — opcodes outside the allowlist, branch targets outside the
//!   ROM, back-edges in the XDP stage, syscalls the stage does not allow,
//!   budgets above the stage cap.
//! - [`reference`]: the reference runner over `monad_common::cart::step`,
//!   the same source the BPF object compiles.

use std::fmt;

use monad_common::cart::{
    CartCfg, DATA_WORDS_MAX, N_PKT, N_TICK, ROM_WORDS_MAX, STAGE_TICK, STAGE_XDP,
};
use monad_common::{mbc_opcodes as op, MbcInsn};
use sha2::{Digest, Sha256};

pub mod reference;

/// The cartridge syscall surface (ADR-080 §2.4 descriptor, ADR-108 ABI v0).
/// It lives here, with the cartridge loader, because upc-api never declares a
/// guest's surface itself.
pub static CARTRIDGE_SURFACE: upc_api::SyscallSurface = upc_api::SyscallSurface {
    name: "cartridge",
    feature_gate: upc_api::FeatureGate::Cartridge,
    syscalls: &CARTRIDGE_SYSCALLS,
};

static CARTRIDGE_SYSCALLS: [upc_api::SyscallDesc; 6] = {
    use monad_common::cart::sys;
    [
        upc_api::SyscallDesc {
            number: sys::FIELD as u32,
            name: "FIELD",
        },
        upc_api::SyscallDesc {
            number: sys::NOW_NS_LO as u32,
            name: "NOW_NS_LO",
        },
        upc_api::SyscallDesc {
            number: sys::VERDICT as u32,
            name: "VERDICT",
        },
        upc_api::SyscallDesc {
            number: sys::EMIT as u32,
            name: "EMIT",
        },
        upc_api::SyscallDesc {
            number: sys::QUEUE as u32,
            name: "QUEUE",
        },
        upc_api::SyscallDesc {
            number: sys::YIELD as u32,
            name: "YIELD",
        },
    ]
};

/// Register the cartridge surface in a host [`upc_api::SurfaceRegistry`]
/// (ADR-080 §4), beside "doom" and "xv6".
pub fn register_surface(r: &mut upc_api::SurfaceRegistry) {
    r.register(CARTRIDGE_SURFACE);
}

pub use monad_common::cart::{state_hash, CartState, FaultKind, Step};

/// Image magic.
pub const MAGIC: &[u8; 4] = b"MBCC";
/// Image format version.
pub const VERSION: u8 = 0;
/// Header bytes before ROM (20 fixed + 32 sha256).
pub const HEADER_LEN: usize = 52;
/// Header flag: a fault drops instead of passing (ADR-108 D-C3 override).
pub const FLAG_FAIL_CLOSED: u16 = 1;

/// A decoded `.cart` image.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Image {
    /// `STAGE_XDP` or `STAGE_TICK`.
    pub stage: u8,
    /// Instruction cap per run (XDP) / per call (tick).
    pub max_insn: u16,
    /// Entry PC.
    pub entry: u16,
    /// Max taken back-edges per tick job (0 for XDP).
    pub loop_bound: u16,
    /// Header flags (`FLAG_FAIL_CLOSED`).
    pub flags: u16,
    /// MBC instruction words.
    pub rom: Vec<u32>,
    /// Read-only data words.
    pub data: Vec<u32>,
}

/// Why bytes are not a `.cart` image.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ImageError {
    /// Shorter than the header.
    Truncated,
    /// Not "MBCC".
    BadMagic,
    /// Unknown format version.
    BadVersion(u8),
    /// Stage byte is neither 0 nor 1.
    BadStage(u8),
    /// rom_words outside 1..=1024.
    RomSize(u16),
    /// data_words above 1024.
    DataSize(u16),
    /// File length does not match the header.
    Length {
        /// Length the header implies.
        want: usize,
        /// Actual length.
        got: usize,
    },
    /// Reserved field or unknown flag bit set.
    Reserved,
    /// Entry PC outside the ROM.
    Entry(u16),
    /// Embedded sha256 does not match.
    Sha,
}

impl fmt::Display for ImageError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ImageError::Truncated => write!(f, "shorter than the {HEADER_LEN}-byte header"),
            ImageError::BadMagic => write!(f, "bad magic (want MBCC)"),
            ImageError::BadVersion(v) => write!(f, "unknown version {v}"),
            ImageError::BadStage(s) => write!(f, "unknown stage {s}"),
            ImageError::RomSize(n) => write!(f, "rom_words {n} outside 1..={ROM_WORDS_MAX}"),
            ImageError::DataSize(n) => write!(f, "data_words {n} above {DATA_WORDS_MAX}"),
            ImageError::Length { want, got } => write!(f, "length {got}, header implies {want}"),
            ImageError::Reserved => write!(f, "reserved field or unknown flag set"),
            ImageError::Entry(e) => write!(f, "entry {e} outside the ROM"),
            ImageError::Sha => write!(f, "embedded sha256 does not match"),
        }
    }
}

impl std::error::Error for ImageError {}

fn le16(b: &[u8], off: usize) -> u16 {
    u16::from_le_bytes([b[off], b[off + 1]])
}

fn image_sha(head20: &[u8], body: &[u8]) -> [u8; 32] {
    let mut h = Sha256::new();
    h.update(head20);
    h.update(body);
    h.finalize().into()
}

impl Image {
    /// Serialize to the ABI v0 byte layout.
    pub fn encode(&self) -> Vec<u8> {
        let mut b = Vec::with_capacity(HEADER_LEN + 4 * (self.rom.len() + self.data.len()));
        b.extend_from_slice(MAGIC);
        b.push(VERSION);
        b.push(self.stage);
        b.extend_from_slice(&self.max_insn.to_le_bytes());
        b.extend_from_slice(&(self.rom.len() as u16).to_le_bytes());
        b.extend_from_slice(&(self.data.len() as u16).to_le_bytes());
        b.extend_from_slice(&self.entry.to_le_bytes());
        b.extend_from_slice(&self.loop_bound.to_le_bytes());
        b.extend_from_slice(&self.flags.to_le_bytes());
        b.extend_from_slice(&0u16.to_le_bytes());
        let mut body = Vec::with_capacity(4 * (self.rom.len() + self.data.len()));
        for w in self.rom.iter().chain(self.data.iter()) {
            body.extend_from_slice(&w.to_le_bytes());
        }
        let sha = image_sha(&b[..20], &body);
        b.extend_from_slice(&sha);
        b.extend_from_slice(&body);
        b
    }

    /// Parse hostile bytes.
    pub fn decode(b: &[u8]) -> Result<Image, ImageError> {
        if b.len() < HEADER_LEN {
            return Err(ImageError::Truncated);
        }
        if &b[0..4] != MAGIC {
            return Err(ImageError::BadMagic);
        }
        if b[4] != VERSION {
            return Err(ImageError::BadVersion(b[4]));
        }
        let stage = b[5];
        if stage != STAGE_XDP && stage != STAGE_TICK {
            return Err(ImageError::BadStage(stage));
        }
        let max_insn = le16(b, 6);
        let rom_words = le16(b, 8);
        let data_words = le16(b, 10);
        let entry = le16(b, 12);
        let loop_bound = le16(b, 14);
        let flags = le16(b, 16);
        if rom_words == 0 || rom_words > ROM_WORDS_MAX {
            return Err(ImageError::RomSize(rom_words));
        }
        if data_words > DATA_WORDS_MAX {
            return Err(ImageError::DataSize(data_words));
        }
        let want = HEADER_LEN + 4 * (rom_words as usize + data_words as usize);
        if b.len() != want {
            return Err(ImageError::Length { want, got: b.len() });
        }
        if le16(b, 18) != 0 || flags & !FLAG_FAIL_CLOSED != 0 {
            return Err(ImageError::Reserved);
        }
        if entry >= rom_words {
            return Err(ImageError::Entry(entry));
        }
        if image_sha(&b[..20], &b[HEADER_LEN..]) != b[20..HEADER_LEN] {
            return Err(ImageError::Sha);
        }
        let words = |from: usize, n: usize| -> Vec<u32> {
            (0..n)
                .map(|i| {
                    let o = from + 4 * i;
                    u32::from_le_bytes([b[o], b[o + 1], b[o + 2], b[o + 3]])
                })
                .collect()
        };
        let rom = words(HEADER_LEN, rom_words as usize);
        let data = words(HEADER_LEN + 4 * rom_words as usize, data_words as usize);
        Ok(Image {
            stage,
            max_insn,
            entry,
            loop_bound,
            flags,
            rom,
            data,
        })
    }

    /// The embedded sha256 (over header[0..20) ‖ ROM ‖ DATA).
    pub fn sha256(&self) -> [u8; 32] {
        let b = self.encode();
        let mut out = [0u8; 32];
        out.copy_from_slice(&b[20..HEADER_LEN]);
        out
    }

    /// Run parameters for the step function.
    pub fn cfg(&self) -> CartCfg {
        CartCfg {
            stage: self.stage,
            max_insn: self.max_insn,
            rom_words: self.rom.len() as u16,
            data_words: self.data.len() as u16,
            entry: self.entry,
            loop_bound: self.loop_bound,
        }
    }
}

/// Opcodes a cartridge may use (ADR-108 allowlist).
pub const ALLOWED_OPCODES: &[u8] = &[
    op::NOP,
    op::ADD,
    op::SUB,
    op::MUL,
    op::AND,
    op::OR,
    op::XOR,
    op::NOT,
    op::SHL,
    op::SHR,
    op::SAR,
    op::MOV,
    op::MOVI,
    op::CMP,
    op::LOAD_IMM32,
    op::ADDI,
    op::JMP,
    op::JZ,
    op::JNZ,
    op::JN,
    op::JP,
    op::JC,
    op::JNC,
    op::LD,
    op::ST,
    op::LDB,
    op::SHLR,
    op::SHRR,
    op::SARR,
    op::MULH,
    op::SYSCALL,
    op::HALT,
];

/// Syscalls each stage may use.
pub fn allowed_syscalls(stage: u8) -> &'static [u16] {
    use monad_common::cart::sys;
    if stage == STAGE_XDP {
        &[
            sys::FIELD,
            sys::NOW_NS_LO,
            sys::VERDICT,
            sys::EMIT,
            sys::QUEUE,
        ]
    } else {
        &[sys::NOW_NS_LO, sys::EMIT, sys::YIELD]
    }
}

/// Why the validator refuses an image.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ValidateError {
    /// Opcode outside the allowlist.
    Opcode {
        /// Word index.
        pc: u16,
        /// Offending opcode.
        opcode: u8,
    },
    /// Branch target outside the ROM.
    BranchTarget {
        /// Word index.
        pc: u16,
        /// Target (may be negative).
        target: i64,
    },
    /// Back-edge in an XDP-stage cartridge.
    BackEdge {
        /// Word index.
        pc: u16,
        /// Target.
        target: u16,
    },
    /// Syscall not allowed in the stage.
    Syscall {
        /// Word index.
        pc: u16,
        /// Syscall number.
        number: u16,
    },
    /// max_insn outside the stage's range.
    MaxInsn {
        /// Stage.
        stage: u8,
        /// Declared value.
        max_insn: u16,
    },
    /// loop_bound set on an XDP-stage cartridge.
    LoopBound,
}

impl fmt::Display for ValidateError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ValidateError::Opcode { pc, opcode } => {
                write!(f, "pc {pc}: opcode {opcode:#04x} not allowed")
            }
            ValidateError::BranchTarget { pc, target } => {
                write!(f, "pc {pc}: branch target {target} outside the ROM")
            }
            ValidateError::BackEdge { pc, target } => write!(
                f,
                "pc {pc}: back-edge to {target} in an XDP-stage cartridge"
            ),
            ValidateError::Syscall { pc, number } => {
                write!(f, "pc {pc}: syscall {number} not allowed in this stage")
            }
            ValidateError::MaxInsn { stage, max_insn } => {
                write!(f, "stage {stage}: max_insn {max_insn} out of range")
            }
            ValidateError::LoopBound => write!(f, "loop_bound must be 0 in the XDP stage"),
        }
    }
}

impl std::error::Error for ValidateError {}

/// What the validator found in an accepted image.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Report {
    /// Declared instruction cap.
    pub max_insn: u16,
    /// Any backward branch (tick stage only).
    pub has_back_edges: bool,
    /// Distinct syscall numbers used, ascending.
    pub syscalls: Vec<u16>,
}

fn is_branch(o: u8) -> bool {
    (op::JMP..=op::JNC).contains(&o)
}

/// Static checks the loader runs on the exact bytes it will load.
pub fn validate(img: &Image) -> Result<Report, ValidateError> {
    let cap = if img.stage == STAGE_XDP {
        N_PKT
    } else {
        N_TICK
    } as u16;
    if img.max_insn == 0 || img.max_insn > cap {
        return Err(ValidateError::MaxInsn {
            stage: img.stage,
            max_insn: img.max_insn,
        });
    }
    if img.stage == STAGE_XDP && img.loop_bound != 0 {
        return Err(ValidateError::LoopBound);
    }
    let n = img.rom.len() as i64;
    let mut back = false;
    let mut syscalls = Vec::new();
    for (i, &w) in img.rom.iter().enumerate() {
        let pc = i as u16;
        let o = MbcInsn(w).opcode();
        if !ALLOWED_OPCODES.contains(&o) {
            return Err(ValidateError::Opcode { pc, opcode: o });
        }
        if is_branch(o) {
            let raw = w & 0x00FF_FFFF;
            let off = if raw & 0x0080_0000 != 0 {
                (raw | 0xFF00_0000) as i32
            } else {
                raw as i32
            };
            let target = i as i64 + 1 + off as i64;
            if target < 0 || target >= n {
                return Err(ValidateError::BranchTarget { pc, target });
            }
            if target <= i as i64 {
                if img.stage == STAGE_XDP {
                    return Err(ValidateError::BackEdge {
                        pc,
                        target: target as u16,
                    });
                }
                back = true;
            }
        }
        if o == op::SYSCALL {
            let num = MbcInsn(w).imm16();
            if !allowed_syscalls(img.stage).contains(&num) {
                return Err(ValidateError::Syscall { pc, number: num });
            }
            if !syscalls.contains(&num) {
                syscalls.push(num);
            }
        }
    }
    syscalls.sort_unstable();
    Ok(Report {
        max_insn: img.max_insn,
        has_back_edges: back,
        syscalls,
    })
}
