//! MBC (Monad Bytecode) assembler, disassembler, emulator, and RV32I translator.
//!
//! This crate provides tools for working with the MBC instruction set:
//! - **assembler**: Parse human-readable MBC assembly text into bytecode
//! - **disasm**: Convert MBC bytecode back into human-readable assembly
//! - **instruction**: Validated instruction decoding and opcode checking
//! - **cpu**: Userspace CPU state with RAM, ROM, and I/O
//! - **execute**: Fetch-decode-execute engine (mirrors BPF monad-cpu-ebpf)
//! - **translator**: Translate RV32I (RISC-V 32-bit) instructions to MBC bytecode
//!
//! # Example
//! ```ignore
//! use monad_mbc::assemble;
//! let program = assemble("
//!     MOVI r0, 42
//!     HALT
//! ")?;
//! ```

pub mod assembler;
pub mod cpu;
pub mod disasm;
pub mod execute;
pub mod instruction;
pub mod translator;

pub use assembler::{assemble, AssembleError};
pub use cpu::Cpu;
pub use disasm::{disasm_insn, disasm_listing, disasm_program};
pub use execute::{Cpu as ExecCpu, ExecError};
pub use instruction::{decode_checked, is_valid_opcode, DecodeError};
pub use translator::{TranslateError, Translator};

/// A compiled MBC program as a vector of 32-bit instruction words.
pub type Assembly = Vec<u32>;
