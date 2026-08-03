#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""
MBC Assembler -- resolves Hard Blocker #2 for the uClinux port.

Assembles MBC (Monad Bytecode) assembly into raw 32-bit little-endian
binary images.  Two-pass: collect labels first, resolve on second pass.

Instruction format: [opcode:8][dst:4][src:4][imm16:16]  (32-bit fixed width)

Usage:
    python3 mbc_asm.py input.asm --output out.bin
    python3 mbc_asm.py input.asm --output out.bin --list
    python3 mbc_asm.py input.asm --hex
"""

from __future__ import annotations

import argparse
import re
import struct
import sys
from dataclasses import dataclass, field

# ---------------------------------------------------------------------------
# Opcode table -- values from ebpf/monad-common/src/lib.rs  mbc_opcodes
# ---------------------------------------------------------------------------

OPCODES: dict[str, int] = {
    # No-op
    "NOP":      0x00,
    # Arithmetic
    "ADD":      0x01,
    "SUB":      0x02,
    "MUL":      0x03,
    "DIV":      0x04,
    "MOD":      0x05,
    "NEG":      0x06,
    # Logical / bitwise
    "AND":      0x07,
    "OR":       0x08,
    "XOR":      0x09,
    "NOT":      0x0A,
    "SHL":      0x0B,
    "SHR":      0x0C,
    "SAR":      0x0D,
    # Register operations
    "MOV":      0x0E,
    "MOVI":     0x0F,
    # Comparison
    "CMP":      0x10,
    # Interrupts
    "INT":      0x17,
    "IRET":     0x18,
    # Stack
    "PUSH":     0x1A,
    "POP":      0x1B,
    # Extended immediate
    "LOAD_IMM32": 0x1C,
    "ADDI":     0x1D,
    # Branch
    "JMP":      0x20,
    "JZ":       0x21,
    "JNZ":      0x22,
    "JN":       0x23,
    "JP":       0x24,
    "JC":       0x25,
    "JNC":      0x26,
    "CALL":     0x27,
    "RET":      0x28,
    "JMPR":     0x29,
    "CALLR":    0x2A,
    # Memory
    "LD":       0x30,
    "ST":       0x31,
    "LDB":      0x32,
    "STB":      0x33,
    "LDH":      0x34,
    "STH":      0x35,
    # Register-based shifts
    "SHLR":     0x36,
    "SHRR":     0x37,
    "SARR":     0x38,
    "MULH":     0x39,
    "MULHU":    0x3A,
    # System
    "SYSCALL":  0x40,
    "HLT":      0xFF,
    "HALT":     0xFF,
}

# Branch instructions that use relative offsets
BRANCH_OPS = {"JMP", "JZ", "JNZ", "JN", "JP", "JC", "JNC", "CALL"}

# Instructions that take no operands
NO_OPERAND_OPS = {"NOP", "RET", "IRET", "HLT", "HALT", "SYSCALL"}

# Instructions: dst only (no src, no imm)
DST_ONLY_OPS = {"NOT", "NEG", "PUSH", "POP", "JMPR", "CALLR"}

# Instructions: dst, src (register pair)
DST_SRC_OPS = {"ADD", "SUB", "MUL", "DIV", "MOD", "AND", "OR", "XOR",
               "MOV", "CMP", "SHLR", "SHRR", "SARR", "MULH", "MULHU"}

# Instructions: dst, imm16
DST_IMM_OPS = {"MOVI", "SHL", "SHR", "SAR", "ADDI", "LOAD_IMM32"}

# Memory load: dst, [src+imm16]
MEM_LOAD_OPS = {"LD", "LDB", "LDH"}

# Memory store: [dst+imm16], src
MEM_STORE_OPS = {"ST", "STB", "STH"}

# ---------------------------------------------------------------------------
# Register parsing
# ---------------------------------------------------------------------------

REGISTER_ALIASES: dict[str, int] = {
    "SP": 15, "sp": 15,
    "FP": 14, "fp": 14,
    "LR": 13, "lr": 13,
}


def parse_register(tok: str) -> int:
    """Parse a register token like 'r0' .. 'r15' or aliases (SP, FP, LR)."""
    tok = tok.strip().rstrip(",")
    if tok in REGISTER_ALIASES:
        return REGISTER_ALIASES[tok]
    m = re.match(r"^[rR](\d+)$", tok)
    if not m:
        raise ValueError(f"invalid register: {tok!r}")
    n = int(m.group(1))
    if n < 0 or n > 15:
        raise ValueError(f"register out of range: {tok!r}")
    return n


# ---------------------------------------------------------------------------
# Encoding helpers
# ---------------------------------------------------------------------------

def encode_insn(opcode: int, dst: int = 0, src: int = 0, imm16: int = 0) -> int:
    """Encode a 32-bit MBC instruction word."""
    # Clamp imm16 to unsigned 16 bits (handles negative values via two's complement)
    imm16 = imm16 & 0xFFFF
    return (opcode & 0xFF) << 24 | (dst & 0xF) << 20 | (src & 0xF) << 16 | imm16


# ---------------------------------------------------------------------------
# AST / intermediate
# ---------------------------------------------------------------------------

@dataclass
class AsmLine:
    """One parsed line of assembly."""
    lineno: int
    label: str | None = None
    mnemonic: str | None = None
    operands: list[str] = field(default_factory=list)
    directive: str | None = None
    dir_args: str = ""
    raw: str = ""


# ---------------------------------------------------------------------------
# Assembler
# ---------------------------------------------------------------------------

class AsmError(Exception):
    def __init__(self, lineno: int, msg: str):
        super().__init__(f"line {lineno}: {msg}")
        self.lineno = lineno


class MbcAssembler:
    def __init__(self):
        self.symbols: dict[str, int] = {}   # label -> word address
        self.equ_defs: dict[str, int] = {}  # .equ constants
        self.text_words: list[int] = []      # assembled words (.text)
        self.data_bytes: bytearray = bytearray()  # raw bytes (.data)
        self.lines: list[AsmLine] = []
        self.origin: int = 0                 # .org base (word address)
        self.section: str = ".text"
        self.listing: list[tuple[int, int, str]] = []  # (addr, word, source)
        # Track data start address (word-aligned offset after .text)
        self._data_start: int = 0

    # ---- Lexer / parser ---------------------------------------------------

    def _parse_line(self, lineno: int, raw: str) -> AsmLine:
        line = AsmLine(lineno=lineno, raw=raw)
        # Strip comment
        text = raw.split(";")[0].strip()
        if not text:
            return line

        # Label?
        m = re.match(r"^(\w+):\s*(.*)", text)
        if m:
            line.label = m.group(1)
            text = m.group(2).strip()
            if not text:
                return line

        # Directive?
        if text.startswith("."):
            parts = text.split(None, 1)
            line.directive = parts[0].lower()
            line.dir_args = parts[1] if len(parts) > 1 else ""
            return line

        # Instruction
        parts = text.split(None, 1)
        line.mnemonic = parts[0].upper()
        if len(parts) > 1:
            # Split operands carefully (handle [r0+4] and quoted strings)
            line.operands = self._split_operands(parts[1])
        return line

    @staticmethod
    def _split_operands(s: str) -> list[str]:
        """Split operand string by commas, respecting brackets."""
        result = []
        depth = 0
        current = ""
        for ch in s:
            if ch == "[":
                depth += 1
            elif ch == "]":
                depth -= 1
            if ch == "," and depth == 0:
                result.append(current.strip())
                current = ""
            else:
                current += ch
        if current.strip():
            result.append(current.strip())
        return result

    # ---- Immediate / expression resolution --------------------------------

    def _resolve_value(self, tok: str, lineno: int, allow_label: bool = True) -> int:
        """Resolve a token to an integer value."""
        tok = tok.strip()
        # Hex literal
        if tok.startswith("0x") or tok.startswith("0X"):
            return int(tok, 16)
        # Binary literal
        if tok.startswith("0b") or tok.startswith("0B"):
            return int(tok, 2)
        # Decimal literal (possibly negative)
        if re.match(r"^-?\d+$", tok):
            return int(tok)
        # .equ constant
        if tok in self.equ_defs:
            return self.equ_defs[tok]
        # Label
        if allow_label and tok in self.symbols:
            return self.symbols[tok]
        raise AsmError(lineno, f"undefined symbol: {tok!r}")

    def _parse_mem_operand(self, tok: str, lineno: int) -> tuple[int, int]:
        """Parse [rN+offset] or [rN] -> (reg, offset)."""
        tok = tok.strip()
        m = re.match(r"^\[(\w+)\s*\+\s*(.+)\]$", tok)
        if m:
            reg = parse_register(m.group(1))
            off = self._resolve_value(m.group(2).strip(), lineno)
            return reg, off
        m = re.match(r"^\[(\w+)\s*-\s*(.+)\]$", tok)
        if m:
            reg = parse_register(m.group(1))
            off = -self._resolve_value(m.group(2).strip(), lineno)
            return reg, off
        m = re.match(r"^\[(\w+)\]$", tok)
        if m:
            reg = parse_register(m.group(1))
            return reg, 0
        raise AsmError(lineno, f"invalid memory operand: {tok!r}")

    # ---- Pass 1: collect labels -------------------------------------------

    def _pass1(self):
        """First pass -- collect all labels and their addresses."""
        self.symbols.clear()
        word_addr = self.origin
        section = ".text"
        data_offset = 0  # byte offset within .data

        for line in self.lines:
            # Directives that affect the counter
            if line.directive:
                d = line.directive
                if d == ".org":
                    word_addr = self._resolve_value(line.dir_args, line.lineno) // 4
                elif d == ".text":
                    section = ".text"
                elif d == ".data":
                    section = ".data"
                elif d == ".equ":
                    parts = line.dir_args.split(",", 1)
                    if len(parts) != 2:
                        raise AsmError(line.lineno, ".equ requires NAME, VALUE")
                    name = parts[0].strip()
                    val = self._resolve_value(parts[1].strip(), line.lineno, allow_label=False)
                    self.equ_defs[name] = val
                elif d == ".ascii":
                    if section == ".data":
                        if line.label:
                            self.symbols[line.label] = None  # placeholder
                        s = self._parse_string(line.dir_args, line.lineno)
                        data_offset += len(s) + 1  # +1 for null terminator
                        continue
                    else:
                        raise AsmError(line.lineno, ".ascii outside .data section")
                elif d == ".word":
                    if section == ".data":
                        if line.label:
                            self.symbols[line.label] = None
                        data_offset += 4
                        continue
                    else:
                        word_addr += 1

            if line.label:
                if section == ".text":
                    self.symbols[line.label] = word_addr
                # .data labels resolved in pass1b after we know text size

            if line.mnemonic:
                word_addr += 1

        # Record text section size
        self._text_word_count = word_addr - self.origin
        # Now resolve .data labels
        self._data_start = self._text_word_count * 4 + self.origin * 4
        self._resolve_data_labels()

    def _resolve_data_labels(self):
        """Second part of pass 1: assign byte addresses to .data labels."""
        byte_offset = self._data_start
        in_data = False
        for line in self.lines:
            if line.directive == ".data":
                in_data = True
                continue
            if line.directive == ".text":
                in_data = False
                continue
            if not in_data:
                continue

            if line.label:
                self.symbols[line.label] = byte_offset

            if line.directive == ".ascii":
                s = self._parse_string(line.dir_args, line.lineno)
                byte_offset += len(s) + 1
            elif line.directive == ".word":
                byte_offset += 4

    # ---- Pass 2: emit code -----------------------------------------------

    def _pass2(self):
        """Second pass -- assemble instructions."""
        self.text_words.clear()
        self.data_bytes.clear()
        self.listing.clear()
        self.section = ".text"
        word_addr = self.origin

        for line in self.lines:
            if line.directive:
                self._handle_directive(line, word_addr)
                if line.directive == ".org":
                    word_addr = self._resolve_value(line.dir_args, line.lineno) // 4
                if line.directive == ".word" and self.section == ".text":
                    word_addr += 1
                continue

            if not line.mnemonic:
                continue

            word = self._assemble_instruction(line, word_addr)
            self.text_words.append(word)
            self.listing.append((word_addr * 4, word, line.raw.strip()))
            word_addr += 1

    def _handle_directive(self, line: AsmLine, word_addr: int):
        d = line.directive
        if d == ".text":
            self.section = ".text"
        elif d == ".data":
            self.section = ".data"
        elif d == ".org":
            pass  # handled in caller
        elif d == ".equ":
            pass  # handled in pass1
        elif d == ".ascii":
            s = self._parse_string(line.dir_args, line.lineno)
            self.data_bytes.extend(s.encode("utf-8"))
            self.data_bytes.append(0)  # null terminator
        elif d == ".word":
            val = self._resolve_value(line.dir_args, line.lineno)
            if self.section == ".data":
                self.data_bytes.extend(struct.pack("<I", val & 0xFFFFFFFF))
            else:
                self.text_words.append(val & 0xFFFFFFFF)
                self.listing.append((word_addr * 4, val & 0xFFFFFFFF, line.raw.strip()))
        else:
            raise AsmError(line.lineno, f"unknown directive: {d}")

    @staticmethod
    def _parse_string(s: str, lineno: int) -> str:
        """Parse a quoted string with escape sequences."""
        s = s.strip()
        if len(s) < 2 or s[0] != '"' or s[-1] != '"':
            raise AsmError(lineno, f"expected quoted string, got: {s!r}")
        inner = s[1:-1]
        # Process escape sequences
        result = []
        i = 0
        while i < len(inner):
            if inner[i] == "\\" and i + 1 < len(inner):
                c = inner[i + 1]
                if c == "n":
                    result.append("\n")
                elif c == "r":
                    result.append("\r")
                elif c == "t":
                    result.append("\t")
                elif c == "0":
                    result.append("\0")
                elif c == "\\":
                    result.append("\\")
                elif c == '"':
                    result.append('"')
                else:
                    result.append(c)
                i += 2
            else:
                result.append(inner[i])
                i += 1
        return "".join(result)

    def _assemble_instruction(self, line: AsmLine, word_addr: int) -> int:
        mn = line.mnemonic
        ops = line.operands

        if mn not in OPCODES:
            raise AsmError(line.lineno, f"unknown mnemonic: {mn}")

        opcode = OPCODES[mn]

        # No-operand instructions
        if mn in NO_OPERAND_OPS:
            return encode_insn(opcode)

        # INT imm8
        if mn == "INT":
            if len(ops) != 1:
                raise AsmError(line.lineno, "INT requires one immediate operand")
            imm = self._resolve_value(ops[0], line.lineno)
            return encode_insn(opcode, imm16=imm)

        # Branch/call with label
        if mn in BRANCH_OPS:
            if len(ops) != 1:
                raise AsmError(line.lineno, f"{mn} requires one operand (label or immediate)")
            target = self._resolve_branch_target(ops[0], line.lineno, word_addr)
            return encode_insn(opcode, imm16=target)

        # dst-only (PUSH, POP, NOT, NEG, JMPR, CALLR)
        if mn in DST_ONLY_OPS:
            if len(ops) != 1:
                raise AsmError(line.lineno, f"{mn} requires one register operand")
            dst = parse_register(ops[0])
            return encode_insn(opcode, dst=dst)

        # dst, src (register pair)
        if mn in DST_SRC_OPS:
            if len(ops) != 2:
                raise AsmError(line.lineno, f"{mn} requires two register operands")
            dst = parse_register(ops[0])
            src = parse_register(ops[1])
            return encode_insn(opcode, dst=dst, src=src)

        # dst, imm16 (MOVI, SHL, SHR, SAR, ADDI, LOAD_IMM32)
        if mn in DST_IMM_OPS:
            if len(ops) != 2:
                raise AsmError(line.lineno, f"{mn} requires a register and an immediate")
            dst = parse_register(ops[0])
            imm = self._resolve_value(ops[1], line.lineno)
            return encode_insn(opcode, dst=dst, imm16=imm)

        # Memory load: LD r0, [r1+4]
        if mn in MEM_LOAD_OPS:
            if len(ops) != 2:
                raise AsmError(line.lineno, f"{mn} requires dst, [src+offset]")
            dst = parse_register(ops[0])
            src, offset = self._parse_mem_operand(ops[1], line.lineno)
            return encode_insn(opcode, dst=dst, src=src, imm16=offset)

        # Memory store: ST [r0+4], r1
        if mn in MEM_STORE_OPS:
            if len(ops) != 2:
                raise AsmError(line.lineno, f"{mn} requires [dst+offset], src")
            dst, offset = self._parse_mem_operand(ops[0], line.lineno)
            src = parse_register(ops[1])
            return encode_insn(opcode, dst=dst, src=src, imm16=offset)

        raise AsmError(line.lineno, f"unhandled mnemonic: {mn}")

    def _resolve_branch_target(self, tok: str, lineno: int, current_addr: int) -> int:
        """Resolve a branch target to a relative offset (signed 16-bit)."""
        tok = tok.strip()
        # Try as immediate first
        try:
            return int(tok, 0)
        except ValueError:
            pass
        # Must be a label
        if tok in self.equ_defs:
            target_addr = self.equ_defs[tok]
            # For .equ constants, treat as absolute word address
            offset = target_addr - (current_addr + 1)
        elif tok in self.symbols:
            target_addr = self.symbols[tok]
            offset = target_addr - (current_addr + 1)
        else:
            raise AsmError(lineno, f"undefined label: {tok!r}")

        if offset < -32768 or offset > 32767:
            raise AsmError(lineno, f"branch offset out of range ({offset}) for label {tok!r}")
        return offset & 0xFFFF

    # ---- Public API -------------------------------------------------------

    def assemble(self, source: str) -> bytes:
        """Assemble source text into a binary image."""
        self.lines = []
        for i, raw in enumerate(source.splitlines(), 1):
            self.lines.append(self._parse_line(i, raw))

        self._pass1()
        self._pass2()

        # Build output: .text words (little-endian u32) + .data bytes (padded to 4)
        out = bytearray()
        for w in self.text_words:
            out.extend(struct.pack("<I", w))
        out.extend(self.data_bytes)
        # Pad to word boundary
        while len(out) % 4 != 0:
            out.append(0)
        return bytes(out)

    def get_listing(self) -> str:
        """Return a human-readable listing."""
        lines = []
        lines.append("Address   Code       Source")
        lines.append("-------   --------   ------")
        for addr, word, src in self.listing:
            lines.append(f"0x{addr:06X}  {word:08X}   {src}")
        # Symbol table
        lines.append("")
        lines.append("Symbol Table:")
        lines.append("-------------")
        for name, addr in sorted(self.symbols.items(), key=lambda x: x[1] if x[1] is not None else 0):
            if addr is not None:
                lines.append(f"  {name:20s} = 0x{addr:06X}")
        if self.equ_defs:
            lines.append("")
            lines.append("Constants (.equ):")
            lines.append("-----------------")
            for name, val in sorted(self.equ_defs.items()):
                lines.append(f"  {name:20s} = 0x{val:06X} ({val})")
        return "\n".join(lines)

    def get_hex_dump(self, data: bytes) -> str:
        """Return a hex dump of the binary."""
        lines = []
        for i in range(0, len(data), 16):
            chunk = data[i:i + 16]
            hex_part = " ".join(f"{b:02X}" for b in chunk)
            ascii_part = "".join(chr(b) if 32 <= b < 127 else "." for b in chunk)
            lines.append(f"0x{i:06X}:  {hex_part:<48s}  {ascii_part}")
        return "\n".join(lines)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="MBC Assembler -- Monad Bytecode two-pass assembler",
        epilog="Example: python3 mbc_asm.py boot.asm --output boot.bin --list",
    )
    parser.add_argument("input", help="Input assembly file (.asm)")
    parser.add_argument("--output", "-o", help="Output binary file (.bin)")
    parser.add_argument("--hex", action="store_true", help="Print hex dump of output")
    parser.add_argument("--list", action="store_true", help="Print listing with addresses")

    args = parser.parse_args()

    try:
        with open(args.input, "r") as f:
            source = f.read()
    except FileNotFoundError:
        print(f"error: file not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    asm = MbcAssembler()
    try:
        binary = asm.assemble(source)
    except AsmError as e:
        print(f"error: {e}", file=sys.stderr)
        sys.exit(1)
    except ValueError as e:
        print(f"error: {e}", file=sys.stderr)
        sys.exit(1)

    if args.list:
        print(asm.get_listing())
        print()

    if args.hex:
        print(asm.get_hex_dump(binary))
        print()

    if args.output:
        with open(args.output, "wb") as f:
            f.write(binary)
        print(f"Wrote {len(binary)} bytes to {args.output} "
              f"({len(asm.text_words)} instructions, "
              f"{len(asm.data_bytes)} data bytes)")

    if not args.output and not args.hex and not args.list:
        print(f"Assembled {len(asm.text_words)} instructions, "
              f"{len(asm.data_bytes)} data bytes. "
              f"Use --output to write binary or --hex/--list to view.",
              file=sys.stderr)


if __name__ == "__main__":
    main()
