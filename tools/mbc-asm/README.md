# MBC Assembler

Two-pass assembler for the MBC (Monad Bytecode) instruction set.
Produces raw 32-bit little-endian binary images suitable for loading
into the UPC execution engine.

**Status:** Resolves Hard Blocker #2 from the uClinux port guide
(`docs/doom/UCLINUX_PORT_GUIDE.md`).

## Usage

```bash
# Assemble to binary
python3 mbc_asm.py input.asm --output out.bin

# Assemble with listing (shows addresses + encoded words)
python3 mbc_asm.py input.asm --output out.bin --list

# Hex dump only
python3 mbc_asm.py input.asm --hex

# All three
python3 mbc_asm.py input.asm -o out.bin --list --hex
```

## Instruction Format

MBC instructions are 32-bit fixed width:

```
[opcode:8][dst:4][src:4][imm16:16]
```

Output is a sequence of little-endian u32 words.

## Registers

| Name | Number | Notes |
|------|--------|-------|
| r0-r12 | 0-12 | General purpose |
| LR | 13 | Link register (alias) |
| FP | 14 | Frame pointer (alias) |
| SP / r15 | 15 | Stack pointer |

## Supported Opcodes

All 43+ opcodes from `ebpf/monad-common/src/lib.rs` `mbc_opcodes` module:

| Category | Instructions |
|----------|-------------|
| Arithmetic | ADD, SUB, MUL, DIV, MOD, NEG, ADDI |
| Bitwise | AND, OR, XOR, NOT, SHL, SHR, SAR |
| Register shifts | SHLR, SHRR, SARR |
| Multiply high | MULH, MULHU |
| Move | MOV, MOVI, LOAD_IMM32 |
| Compare | CMP |
| Branch | JMP, JZ, JNZ, JN, JP, JC, JNC |
| Call/Return | CALL, RET, JMPR, CALLR |
| Stack | PUSH, POP |
| Memory | LD, ST, LDB, STB, LDH, STH |
| Interrupt | INT, IRET |
| System | SYSCALL, HLT |
| No-op | NOP |

## Syntax

```asm
; Comments start with semicolon
.org 0x100          ; Set origin address (byte address)
.text               ; Switch to text section (code)
.data               ; Switch to data section
.equ NAME, 0x1234  ; Define a constant
.ascii "Hello\n"   ; Null-terminated string (data section)
.word 42            ; 32-bit word

label:              ; Define a label

; Instruction examples:
NOP
MOV  r0, r1         ; register move
MOVI r0, 42         ; load immediate
ADD  r0, r1         ; dst = dst + src
ADDI r0, 5          ; dst = dst + sign_extend(imm16)
LD   r0, [r1+4]     ; load word: r0 = RAM[r1 + 4]
ST   [r0+4], r1     ; store word: RAM[r0 + 4] = r1
JMP  label          ; relative branch (16-bit signed offset)
CALL label          ; push PC, branch to label
INT  0x80           ; software interrupt
HLT                 ; halt CPU
```

## Sections

- `.text` -- Code section. Instructions are assembled as 32-bit words.
- `.data` -- Data section. Supports `.ascii` and `.word` directives.
  Data bytes are appended after the text section in the output binary.

## Branch Encoding

Branch instructions (JMP, JZ, JNZ, JN, JP, JC, JNC, CALL) use
**signed 16-bit relative offsets** in the imm16 field. The offset is
relative to PC+1 (the instruction after the branch).

For example, `JMP label` at word address 5 targeting word address 2
encodes offset = 2 - (5+1) = -4 = 0xFFFC.

## Error Reporting

All errors include the source line number:

```
error: line 15: undefined label: 'nonexistent'
error: line 22: invalid register: 'r16'
error: line 30: branch offset out of range (40000) for label 'far_away'
```

## Example

See `test_hello.asm` for a minimal hello-world program using INT 0x80
syscalls.

## Dependencies

Python 3.6+ (standard library only, no external packages).

## License

SPDX-License-Identifier: GPL-3.0-or-later
