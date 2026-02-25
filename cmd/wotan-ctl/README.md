# wotan-ctl - Wotan Compute Engine Control Tool

`wotan-ctl` is the operator's command-line tool for loading programs and data into the Doom-over-IPv6 compute engine (Wotan). It implements D-008 and D-009 specifications.

## Overview

The tool provides two main commands:

- **load-rom**: Load MBC bytecode into BPF `rom_map` (CPU instruction memory)
- **load-mem**: Load binary data into Wotan's L2 ring buffer (RAM)

## Build & Install

```bash
cd /path/to/unheaded
go build ./cmd/wotan-ctl/...
# Binary location: cmd/wotan-ctl/wotan-ctl (or set via -o flag)
```

## Testing

```bash
go test ./cmd/wotan-ctl/... -v
```

### Test Coverage

- **TestDisassembleMBC**: Tests MBC instruction disassembly (6 instruction types)
- **TestDefaultCpuState**: Validates CPU state initialization
- **TestLoadRomDryRun**: End-to-end dry-run test with temporary file
- **TestEncodeValue**: Tests value encoding (uint32, uint64, int32, int64, []byte)
- **TestEncodeValueErrors**: Error handling validation
- **BenchmarkDisassembleMBC**: Performance benchmark

## Commands

### load-rom: Load MBC Bytecode

Loads MBC (Monad Bytecode) instructions into the BPF `rom_map` array.

**Usage:**
```bash
wotan-ctl load-rom --file <path> --flow-label <label> [options]
```

**Required Flags:**
- `--file <path>`: Path to .mbc binary file (little-endian u32 instructions)
- `--flow-label <label>`: IPv6 flow label (can use hex: 0x0A3F7E)

**Optional Flags:**
- `--flow-label <label>`: Flow label in decimal (0-0xFFFFF) or hex (0x...)
- `--map-pin <dir>`: BPF map pin directory (default: /sys/fs/bpf)
- `--stats`: Print instruction count and estimated runtime
- `--disasm`: Disassemble and print each instruction
- `--reset`: Reset cpu_map entry (restart from PC=0)
- `--verbose`: Enable debug logging

**Examples:**
```bash
# Load program with flow label
wotan-ctl load-rom --file program.mbc --flow-label 0x0A3F7E

# Load and show statistics
wotan-ctl load-rom --file doom1.mbc --flow-label 0x123456 --stats

# Load and disassemble all instructions
wotan-ctl load-rom --file test.mbc --flow-label 0x100 --disasm --stats
```

**File Format:**
- Sequence of 32-bit little-endian unsigned integers
- Each integer is one instruction
- File size must be multiple of 4 bytes

#### Instruction Format

MBC instructions are 32-bit words:

```
Opcode: bits 24-31 (8 bits)
Dst:    bits 20-23 (4 bits) - destination register
Src:    bits 16-19 (4 bits) - source register
Imm:    bits  0-15 (16 bits) - immediate value
```

#### Instruction Set (40 opcodes)

**Arithmetic (0x01-0x06):**
- 0x01: ADD r_dst, r_src, imm
- 0x02: SUB r_dst, r_src, imm
- 0x03: MUL r_dst, r_src, imm
- 0x04: DIV r_dst, r_src, imm
- 0x05: MOD r_dst, r_src, imm
- 0x06: NEG r_dst

**Bitwise (0x07-0x0D):**
- 0x07: AND r_dst, r_src, imm
- 0x08: OR r_dst, r_src, imm
- 0x09: XOR r_dst, r_src, imm
- 0x0A: NOT r_dst
- 0x0B: SHL r_dst, r_src, imm
- 0x0C: SHR r_dst, r_src, imm
- 0x0D: SAR r_dst, r_src, imm
- 0x0E: CMP r_dst, r_src, imm

**Control Flow (0x20-0x28):**
- 0x20: JMP imm (jump to instruction)
- 0x21: JZ imm (jump if zero)
- 0x22: JNZ imm (jump if not zero)
- 0x23: JN imm (jump if negative)
- 0x24: JP imm (jump if positive)
- 0x25: JC imm (jump if carry)
- 0x26: JNC imm (jump if not carry)
- 0x27: CALL imm (call subroutine)
- 0x28: RET (return from subroutine)

**Register Operations (0x29-0x2A):**
- 0x29: MOV r_dst, r_src
- 0x2A: MOVI r_dst, imm

**Memory Operations (0x30-0x35):**
- 0x30: LD r_dst, [r_src + imm] (load 32-bit)
- 0x31: ST r_dst, [r_src + imm] (store 32-bit)
- 0x32: LDB r_dst, [r_src + imm] (load byte)
- 0x33: STB r_dst, [r_src + imm] (store byte)
- 0x34: LDH r_dst, [r_src + imm] (load half-word)
- 0x35: STH r_dst, [r_src + imm] (store half-word)

**System (0x40, 0xFF):**
- 0x40: SYSCALL imm
  - 0x0001: DRAW_FRAME
  - 0x0002: GET_KEY
  - 0x0003: GET_TICKS
  - 0x0004: SLEEP
  - 0x00FF: HALT
- 0xFF: HALT (immediate halt)

**Statistics Output:**
```
Loaded 1234 instructions into rom_map

Statistics:
  Instructions: 1234
  Size: 4936 bytes (4.8 KB)
  Est. runtime @ 2MHz: 0.617 seconds
  Est. runtime @ 35fps: 21.6 frames
```

### load-mem: Load Binary Data

Loads binary data into Wotan's L2 ring buffer (RAM).

**Usage:**
```bash
wotan-ctl load-mem --file <path> --base-addr <addr> --flow-label <label> [options]
```

**Required Flags:**
- `--file <path>`: Path to binary data file
- `--base-addr <addr>`: Base memory address (decimal or hex: 0x100000)
- `--flow-label <label>`: IPv6 flow label

**Optional Flags:**
- `--wotan-addr <addr>`: Wotan server address (default: http://localhost:5555)
- `--map-pin <dir>`: BPF map pin directory (default: /sys/fs/bpf)
- `--warm`: Pre-stage memory pages to L1 cache
- `--verify`: Verify memory after loading (checksum comparison)
- `--verbose`: Enable debug logging

**Examples:**
```bash
# Load WAD file at address 0x100000
wotan-ctl load-mem --file doom1.wad --base-addr 0x100000 --flow-label 0x123456

# Load and pre-warm L1 cache
wotan-ctl load-mem --file sprite.bin --base-addr 0x200000 --flow-label 0x100 --warm

# Load with verification
wotan-ctl load-mem --file data.bin --base-addr 0x0 --flow-label 0x50 --verify
```

**File Format:**
- Any binary data
- Automatically split into 4096-byte pages

**Memory Layout:**
- L2 ring buffer granularity: 4096 bytes per page
- L1 cache line size: 64 bytes
- Addressed by (flow_label, base_addr + offset)

## Architecture

### Components

#### main.go
- Entry point and root command definition
- Custom command framework (no external dependencies like cobra)
- Version information management
- Help system

#### load_rom.go (D-008)
- `newLoadRomCmd()`: Creates load-rom subcommand
- `loadRom()`: Core ROM loading logic
- `disassembleMBC()`: Instruction disassembler
- `defaultCpuState()`: CPU state initialization
- `CpuState`: CPU register and state structure

#### load_mem.go (D-009)
- `newLoadMemCmd()`: Creates load-mem subcommand
- `loadMem()`: Core memory loading logic
- Data pagination (4096-byte pages)
- MD5 checksum calculation
- Dry-run implementation

#### bpf_maps.go
- `BPFMap`: BPF map handle
- `openBPFMap()`: Open pinned BPF maps via `unix.BpfObjGet()`
- `Update()`: Insert/update via BPF_MAP_UPDATE_ELEM syscall
- `Lookup()`: Query via BPF_MAP_LOOKUP_ELEM syscall
- `encodeValue()`: Convert Go values to little-endian bytes
- `Close()`: Cleanup

#### load_rom_test.go
- Comprehensive unit tests
- Benchmark suite

### Dependencies

**Built-in (stdlib):**
- `flag`: Command-line parsing
- `fmt`: I/O formatting
- `os`: File and system operations
- `encoding/binary`: Little-endian encoding
- `unsafe`: Raw memory operations for syscalls
- `testing`: Test framework

**Project:**
- `unheaded/pkg/logger`: Logging infrastructure

**External:**
- `golang.org/x/sys/unix`: BPF syscall bindings

## BPF Integration

### Map Access

The tool accesses BPF maps via pinned paths:

```
/sys/fs/bpf/rom_map     - Instruction memory (array of u32)
/sys/fs/bpf/cpu_map     - CPU state (map of flow_label -> CpuState)
```

### Syscalls

**BPF_MAP_UPDATE_ELEM (cmd=2):**
```
bpf(BPF_MAP_UPDATE_ELEM, {
  map_fd: fd,
  key: ptr to key,
  value: ptr to value,
  flags: 0 (BPF_ANY - overwrite or create)
})
```

**BPF_MAP_LOOKUP_ELEM (cmd=1):**
```
bpf(BPF_MAP_LOOKUP_ELEM, {
  map_fd: fd,
  key: ptr to key,
  value: ptr to output buffer
})
```

### Dry-Run Mode

When BPF maps are unavailable (non-Linux or unpinned), the tool gracefully falls back to dry-run mode:
- Reads and parses files normally
- Performs all calculations and disassembly
- Outputs what would be written
- Useful for validation and testing

## Wotan HTTP API (load-mem)

**NOTE:** The following endpoints are documented for future implementation. Currently, load-mem is a dry-run implementation.

Planned HTTP endpoints for full Wotan integration:

```
POST /wotan/memory/{flow_label}/{address}
  - Store a page of data
  - Body: binary page data (max 4096 bytes)
  - Returns: {status: "ok", bytes_written: N}

GET /wotan/memory/{flow_label}/{address}
  - Retrieve a page of data
  - Query params: size (1-4096)
  - Returns: binary page data

POST /wotan/memory/warm/{flow_label}
  - Pre-stage memory pages to L1 cache
  - Body: {pages: [addr1, addr2, ...], priority: "high"}
  - Returns: {pages_warmed: N}

GET /wotan/memory/stats/{flow_label}
  - Get memory statistics
  - Returns: {total_bytes: N, pages_loaded: N, l1_hits: N, l2_hits: N}
```

## Development Notes

### Adding New Opcodes

Edit the `disassembleMBC()` function in `load_rom.go`:

```go
opNames := map[uint8]string{
    // Add new opcode:
    0xXX: "NEWOP",
    // ...
}
```

### Error Handling

All operations support graceful degradation:
- BPF unavailable: dry-run mode
- File not readable: clear error with file path
- Wotan unavailable: offline operation with documented API

### Testing

Run with race detection:
```bash
go test ./cmd/wotan-ctl/... -race -v
```

## Known Limitations

1. **load-mem HTTP API**: Currently dry-run only. Requires Wotan HTTP endpoint specification.
2. **BPF Maps**: Assumes maps are pinned at `/sys/fs/bpf/`. Configurable via `--map-pin`.
3. **64-bit Addresses**: Base addresses limited to 32-bit in output (masked to 0xFFFFFFFF).
4. **CPU Clock**: Estimated runtime assumes 2MHz clock. Adjustable in source code.

## Future Work

1. Implement full Wotan HTTP API integration for load-mem
2. Add support for compressed ROM files (.mbc.gz)
3. Batch loading for multiple programs
4. Progress indicators for large files
5. Real-time performance monitoring
6. Integration with wotan-daemon for lifecycle management

## License

Part of the Unheaded Kingdom project.
