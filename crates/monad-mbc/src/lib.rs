//! MBC (Monad Bytecode) assembler, disassembler, and RV32I translator.
//!
//! This crate provides tools for working with the MBC instruction set:
//! - **assembler**: Parse human-readable MBC assembly text into bytecode
//! - **disasm**: Convert MBC bytecode back into human-readable assembly
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
pub mod disasm;
pub mod translator;

pub use assembler::{assemble, AssembleError};
pub use disasm::{disasm_insn, disasm_listing, disasm_program};
pub use translator::{Translator, TranslateError};

/// A compiled MBC program as a vector of 32-bit instruction words.
pub type Assembly = Vec<u32>;
