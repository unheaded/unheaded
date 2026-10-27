# Agent Handoff — 2026-02-18 (Session 2)

## What Was Done This Session

### Translator Rewrite (COMPLETE)
Major rewrite of `crates/monad-mbc/src/translator.rs` — from 618 lines to ~730 lines of code + ~300 lines of tests. Changed from single-pass to **two-pass architecture** fixing the fundamental branch offset bug.

### Key Changes

| Change | Status | Detail |
|--------|--------|--------|
| Two-pass translator | DONE | Pass 1: expand + build rv32i→mbc PC map. Pass 2: resolve branches/calls. |
| CALL absolute fix | DONE | VM uses `cpu.pc = imm16` (absolute). Translator now emits absolute MBC PC. |
| LUI 32-bit fix | DONE | `emit_load32()` — MOVI+SHL+MOVI+OR+MOVI sequence for full 32-bit values. |
| JALR → RET | DONE | `JALR x0, 0(x1)` emits MBC RET. General JALR returns error. |
| BLT/BGE/BLTU/BGEU | DONE | Maps to CMP + JN/JP/JC/JNC. Approximate for signed overflow edge case. |
| LBU/LHU | DONE | Maps to LDB/LDH (MBC already zero-extends). |
| AUIPC | DONE | Static computation: `value = pc + (imm20 << 12)`, then `emit_load32`. |
| FENCE | DONE | Treated as NOP (no MBC instructions emitted). |
| RV32M (MUL/DIV/REM) | DONE | MUL→MUL, DIV/DIVU→DIV, REM/REMU→MOD. Signed variants approximate. |
| JAL x0 as JMP | DONE | `JAL x0, offset` emits JMP (not CALL), since no link register needed. |
| ELF-to-MBC CLI | DONE | `rv32i-to-mbc` binary: parses ELF via `goblin`, extracts .text, translates. |
| Tests | DONE | 52 unit tests (was 9), all passing. Includes two-pass resolution test. |

### Files Modified/Created

**Modified:**
- `crates/monad-mbc/src/translator.rs` — Complete rewrite
- `crates/monad-mbc/Cargo.toml` — Added `goblin` dep + `[[bin]]` section

**Created:**
- `crates/monad-mbc/src/bin/rv32i_to_mbc.rs` — ELF-to-MBC CLI tool

### What Was NOT Done (Remaining from Step 2.5)

Step 2.5 (Doom Cross-Compilation) is **partially complete**. The translator and CLI tool are done. The actual cross-compilation was not attempted because it requires the C cross-compiler pipeline which diverges from the Rust/Go stack.

Remaining work:
1. **Test the CLI tool** with a real RV32I ELF binary (compile a trivial C program, run through `rv32i-to-mbc`)
2. **Doom bare-metal port** — write doomgeneric stubs + linker script
3. **Cross-compile Doom** — C → RV32I ELF → MBC
4. **Load MBC into BPF maps** — needs `wotan-ctl` CLI or equivalent
5. **End-to-end wiring** — dashboard ↔ WebSocket ↔ BPF ↔ screen

## Test Status

```
cargo test -p monad-mbc: 54 tests pass (52 unit + 2 integration)
cargo build --bin rv32i-to-mbc: builds successfully
```

## Git State
- **Branch:** `main`
- **Uncommitted changes:** translator.rs rewrite, Cargo.toml edit, new CLI binary
- **Untracked:** `.claude/`, `crates/monad-mbc/Cargo.lock`, `crates/monad-mbc/src/bin/`

## Architecture Notes for Next Agent

### Two-Pass Translator Design

```
Pass 1: For each RV32I instruction at word index i:
  1. Record pc_map[i] = current MBC output length
  2. Translate to Vec<MbcEmit> items:
     - MbcEmit::Concrete(u32)      — ready to emit
     - MbcEmit::Branch{opcode, rv32i_target_byte} — needs offset resolution
     - MbcEmit::Call{rv32i_target_byte}            — needs absolute address
  3. Append sentinel: pc_map[len] = total MBC items

Pass 2: For each MbcEmit item:
  - Concrete → emit directly
  - Branch → look up rv32i_target_byte/4 in pc_map → compute relative offset
  - Call → look up rv32i_target_byte/4 in pc_map → use as absolute imm16
```

### Known Translator Limitations
- **Register-based shifts** (SLL/SRL/SRA with rs2): unsupported, MBC only has immediate shifts
- **General JALR**: only `JALR x0, 0(x1)` (return) is supported, not indirect calls
- **MULH/MULHSU/MULHU**: upper-half multiply not supported
- **Signed overflow in BLT/BGE**: uses N flag without V (overflow) flag, approximate
- **Signed loads (LH/LB)**: zero-extend only, no sign extension emitted
- **x16-x31 registers**: hard error; must use `-ffixed-x16..x31` with GCC

### Key File Paths
- Translator: `crates/monad-mbc/src/translator.rs`
- CLI tool: `crates/monad-mbc/src/bin/rv32i_to_mbc.rs`
- BPF VM: `ebpf/monad-cpu-ebpf/src/main.rs`
- MBC ISA types: `ebpf/monad-common/src/lib.rs`
- Assembler: `crates/monad-mbc/src/assembler.rs`
- Wotan memory: `services/wotan/internal/compute/`
- Dashboard: `dashboard/js/doom-viewport.js`

### CLI Tool Usage
```bash
# From crates/monad-mbc/:
cargo run --bin rv32i-to-mbc -- input.elf -o output.mbc --stats --disasm
```

### Commit Ready
Files are staged-ready. Suggested commit:
```
feat(mbc): rewrite RV32I→MBC translator to two-pass architecture + ELF CLI tool
```
