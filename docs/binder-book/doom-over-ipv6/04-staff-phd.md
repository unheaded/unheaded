# Doom Over IPv6: Staff Engineer / PhD-Level Analysis

## Abstract

We present a novel compute architecture that repurposes Linux eBPF XDP (eXpress Data Path) as a general-purpose execution engine, using IPv6 packet circulation through network namespaces as the instruction clock. A custom 32-bit ISA — Monad Bytecode (MBC) — executes inside the XDP fast path, with BPF maps serving as the memory hierarchy. We demonstrate the architecture by running id Software's DOOM (1993), compiled from C through RISC-V to MBC, achieving interactive frame rates (~20 fps) entirely within kernel space. This document provides a rigorous technical treatment of the system's architecture, the constraints imposed by the BPF verifier, the memory model, the ISA design, and the implications for eBPF as a computational substrate.

## 1. Motivation and Thesis

### 1.1 The Packet-as-Clock-Cycle Model

In von Neumann architectures, a clock signal synchronizes the fetch-decode-execute pipeline. The clock is external to the computation — it provides temporal ordering without carrying semantic content.

We observe that **XDP packet arrival events are functionally equivalent to clock edges**. Each packet invocation of the XDP program represents an opportunity to perform bounded computation. The key insight is:

> **If the computation per invocation is bounded (satisfying the BPF verifier) and state persists across invocations (via BPF maps), then an XDP program can implement an arbitrary state machine — including a Turing-complete processor.**

The packet carries no computational payload. It is a **pure synchronization signal** — a clock tick delivered via the network stack. The network topology (a directed ring of veth pairs) serves as the bus interconnect, and BPF maps serve as the unified memory hierarchy.

### 1.2 Architectural Analogy

The analogy to conventional microarchitecture is structurally precise:

| Conventional CPU | Doom-over-IPv6 |
|-----------------|----------------|
| Clock crystal | Packet injector (Go binary) |
| Clock signal on FSB | IPv6 packet arrival event |
| Clock frequency | Packet injection rate (Hz) |
| ALU / execution units | XDP BPF program (monad_cpu) |
| Instruction memory (I$) | ROM_MAP (BPF Array, 1 MiB) |
| Data memory (D$) | RAM_MAP (BPF Array, 64 MiB) |
| Register file | CPU_MAP entry (16 x u32 + flags) |
| Memory-mapped I/O | SCREEN_MAP, KBD_MAP |
| Front-side bus | veth ring (6-namespace directed cycle) |
| Bus width | Packet size (78 bytes, but only headers used) |
| Pipeline depth | MAX_INSN_PER_TICK (256 instructions) |
| Multi-core | Multiple CPU_MAP instances (keyed by Flow Label) |
| DMA / frame buffer | SCREEN_MAP → userspace batch read → WebSocket |

The packet ring functions as the message bus between the compute element and its L1 memory — BPF maps are kernel memory accessed via helper functions with O(1) latency for Array-type maps, analogous to an SRAM-backed L1 cache. The "bus protocol" is trivial: packet arrival triggers a fixed-length burst of memory transactions (reads/writes to maps), after which the bus goes idle until the next packet.

### 1.3 Multiplexing

The IPv6 Flow Label field (20 bits) encodes the CPU instance ID in its low 8 bits. This enables up to 256 independent virtual CPUs sharing the same physical ring and BPF program, each with isolated state in CPU_MAP[instance_id]. This is analogous to hardware multithreading (SMT) where multiple thread contexts share execution resources.

## 2. Network Topology

### 2.1 Ring Construction

Six Linux network namespaces (monad0-monad5) form a directed ring via veth pairs:

```
monad0/veth01 ←→ monad1/veth01p    (link 0: fd00:3f:75:0::/64)
monad1/veth12 ←→ monad2/veth12p    (link 1: fd00:3f:75:1::/64)
monad2/veth23 ←→ monad3/veth23p    (link 2: fd00:3f:75:2::/64)
monad3/veth34 ←→ monad4/veth34p    (link 3: fd00:3f:75:3::/64)
monad4/veth45 ←→ monad5/veth45p    (link 4: fd00:3f:75:4::/64)
monad5/veth50 ←→ monad0/veth50p    (link 5: fd00:3f:75:5::/64)
```

XDP programs attach to the "p" (peer) side of each veth pair in `xdpgeneric` (SKB) mode. The program is loaded once via the Aya eBPF loader on hop 0, then the same `prog_id` is attached to hops 1-5 via `bpftool net attach xdpgeneric`. This ensures a single program instance with shared map file descriptors.

### 2.2 Routing and NDP Considerations

Each veth link uses a **unique /64 prefix** (`fd00:3f:75:{i}::/64`). This is a hard requirement — using the same /64 across multiple links creates IPv6 NDP (Neighbor Discovery Protocol) ambiguity. The kernel's neighbor table maps link-local addresses to MACs per-interface, but on-link prefix overlap causes the kernel to attempt NDP resolution on the wrong interface, resulting in silent packet drops.

The destination address `fd00:dead::1` is deliberately unreachable (no matching connected prefix). This forces each namespace's routing table to use the default route, forwarding the packet to the next hop — where the XDP program on the receiving interface intercepts it before the kernel's IP stack processes it.

Static neighbor entries are installed to bypass NDP entirely, eliminating multicast solicitation delays.

### 2.3 XDP_TX Bounce Semantics

After executing MAX_INSN_PER_TICK instructions, the XDP program returns `XDP_TX`, which reflects the packet back out the ingress interface. On a veth pair, this delivers the packet to the peer interface — where XDP fires again. This creates a **tight bounce loop** on a single veth pair, with the packet never leaving the kernel's packet processing fast path.

The performance advantage is significant: XDP_TX on veth avoids memory allocation, socket buffer cloning, and netfilter traversal. The packet data remains in the same memory region, benefiting from CPU cache locality. Measured per-bounce latency is <1 microsecond.

A hop counter in the Monad header (separate from IPv6 Hop Limit, which the XDP program does not decrement) tracks total bounces. At 255, the packet is dropped via XDP_DROP, and the injector must supply a fresh packet.

## 3. BPF Verifier Constraints and ISA Design

### 3.1 Verifier Model

The BPF verifier performs abstract interpretation via depth-first exploration of all reachable program states. Key constraints:

| Constraint | Limit | Impact on Design |
|-----------|-------|-----------------|
| Verified instruction count | 1,000,000 | Bounds total loop iterations x branch paths |
| Jump complexity | 8,192 explored states | Limits loop body complexity |
| Stack depth | 512 bytes | No deep recursion in BPF program itself |
| No unbounded loops | Compile-time provable bound required | `while i < MAX_INSN_PER_TICK` with `i` incremented each iteration |
| Map access bounds checking | All keys must be validated | Every `map.get()` returns `Option`; `None` path must be handled |
| No function pointers | Static dispatch only | All opcode handlers are inline match arms |

### 3.2 Execute Loop Verification Cost

The execute loop's verification cost is approximately:

```
V = MAX_INSN_PER_TICK × (num_opcode_branches × avg_branch_depth)
```

With ~40 opcodes and an average branch depth of ~5 instructions per opcode handler, at MAX_INSN_PER_TICK = 256:

```
V ≈ 256 × 40 × 5 = 51,200 explored states
```

This is well under the 1M instruction limit but approaches the 8,192 jump complexity bound because the verifier tracks state at each branch point. The practical ceiling for MAX_INSN_PER_TICK is determined empirically — 256 passes the verifier with the current opcode decoder; 512 does not.

### 3.3 ISA Design Rationale

MBC is a **register-register** architecture with 32-bit fixed-width instructions. Design decisions driven by BPF constraints:

**Fixed-width encoding**: Variable-length instructions would require dynamic bounds checking on the instruction stream, increasing verifier complexity. The 32-bit format `[8 opcode | 4 dst | 4 src | 16 imm]` decodes in constant time with simple bit shifts.

**16 registers**: Chosen to fit the 4-bit register selector field. 16 registers are sufficient for RISC-V → MBC translation (RISC-V has 32, but Doom's compiled code primarily uses ~12).

**Separate ROM and RAM**: ROM_MAP is read-only at runtime (BPF Array with no update path in the XDP program). This separates the fetch path (ROM_MAP[pc]) from the data path (RAM_MAP[addr]), reducing verifier state — the verifier can prove ROM access is always valid if `pc < rom_size`.

**Word-addressed RAM with byte accessors**: RAM_MAP is `Array<u32>` (word-addressable). Byte and halfword operations (LDB, STB, LDH, STH) extract/insert within the 32-bit word at `RAM_MAP[byte_addr >> 2]`. This trades instruction complexity for map efficiency — a single BPF array lookup retrieves 4 bytes.

**SYSCALL for I/O**: Hardware-like trap mechanism. SYSCALL reads `r1` for the syscall number and dispatches to inline handlers (SYS_DRAW_FRAME, SYS_GET_KEY, SYS_GET_TICKS, SYS_SLEEP). This cleanly separates pure computation from side effects.

**RV2MBC_MAP for indirect jumps**: Function pointers in Doom's `.data` section contain RISC-V byte addresses. JMPR/CALLR must translate these to MBC ROM indices. The translation table is precomputed during compilation and loaded into RV2MBC_MAP. On an indirect jump:

```rust
let rv_word = cpu.regs[dst] >> 2;           // RISC-V byte addr → word index
let mbc_pc = RV2MBC_MAP.get(rv_word);       // look up MBC PC
cpu.pc = mbc_pc.unwrap_or(HALT_SENTINEL);   // jump or halt
```

This is analogous to a TLB (Translation Lookaside Buffer) for virtual-to-physical address mapping in conventional architectures.

### 3.4 Compilation Pipeline

```
doom.c (id Software, 1993)
  + doomgeneric platform layer
  + libc_monad.c (minimal libc stubs)
  + crt0_monad.S (CRT0: SP init, BSS clear)
      │
      ▼
riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32
      │
      ▼
doom.elf (RV32I ELF, ~180KB .text, ~160KB .rodata/.data)
      │
      ▼
rv32i_to_mbc (crates/monad-mbc/src/bin/rv32i_to_mbc.rs)
  - Linear scan disassembly of .text
  - 1:N instruction expansion (each RV32I insn → 1-4 MBC insns)
  - Generates doom.mbc (76,128 MBC words) + doom.rv2mbc (45,934 entries)
      │
      ▼
doom-loader-core.py
  - BPF_OBJ_GET to open pinned maps
  - BPF_MAP_UPDATE_ELEM in batch (~740K entries/sec)
  - Loads ROM, RV2MBC, data sections, WAD, CPU init
```

The expansion ratio (RV32I → MBC) is approximately 1.65:1, reflecting MBC's lack of certain RV32I addressing modes and the need for explicit LOAD_IMM32 + MOVI sequences for 32-bit constants.

## 4. Memory Model

### 4.1 BPF Map Type Selection

| Map | Type | Rationale |
|-----|------|-----------|
| ROM_MAP | Array<u32> | O(1) indexed access, no hash overhead, read-only |
| RAM_MAP | Array<u32>, 16M entries | **Critical**: Originally HashMap; switched to Array after silent write drops under memory pressure caused data corruption (commit 3a1bbe7). Array never fails writes. |
| CPU_MAP | HashMap<u32, [u8; 104]> | Sparse (up to 256 instances), keyed by Flow Label |
| SCREEN_MAP | Array<u8>, 64K entries | Byte-addressable projection of framebuffer region |
| KBD_MAP | Array<u32>, 8 entries | Tiny, fixed-size I/O register file |
| RV2MBC_MAP | Array<u32>, 64K entries | Dense translation table |
| L1_CACHE | LRU HashMap | **Disabled** — concurrent XDP invocations on different hops create non-atomic read-modify-write hazards |
| STATS | HashMap<u32, u64> | Per-counter atomic increment via `__sync_fetch_and_add` |
| COMPUTE_EVENTS | RingBuf, 256K | Async event publication to userspace (halt, screen write, cache miss) |

### 4.2 Concurrency Model

Multiple XDP invocations can execute concurrently across different CPUs (one per veth pair with concurrent packet arrivals). BPF map operations are individually atomic (each `map_update_elem` is atomic with respect to other updates), but **sequences of map operations are not transactional**.

This creates a critical correctness concern: if two XDP invocations process the same CPU instance concurrently, they may read the same CPU_MAP state, execute divergent instruction streams, and write back conflicting states — a classic lost-update anomaly.

**Mitigation**: The packet injection model provides natural serialization. In fast mode, the injector sends packets sequentially from a single socket in monad0. Each packet completes its full bounce sequence (255 hops) before XDP_DROP, and the next packet enters the ring. With a single injector and XDP_TX bouncing, there is at most **one active packet in the ring at any time**, eliminating concurrent CPU_MAP access.

This serialization guarantee breaks under burst injection (multiple packets in-flight simultaneously) or multi-socket injection. The 3000 microsecond default delay in the original Python injector was empirically tuned to prevent overlap.

### 4.3 Screen Write Path

The dual-write to RAM_MAP and SCREEN_MAP on framebuffer region stores deserves analysis:

```rust
fn mem_write_word(word_addr: u32, value: u32) {
    // Always write to RAM (canonical store)
    RAM_MAP.insert(word_addr, value, 0);

    // If address falls in screen region, also update SCREEN_MAP
    let screen_word_start = SCREEN_BASE >> 2;  // 0xC000 >> 2 = 0x3000
    let screen_word_end = (SCREEN_BASE + SCREEN_SIZE + 3) >> 2;

    if word_addr >= screen_word_start && word_addr < screen_word_end {
        let pixel_base = (word_addr - screen_word_start) * 4;
        let bytes = value.to_le_bytes();
        for k in 0..4u32 {
            let px = pixel_base + k;
            if px < SCREEN_SIZE as u32 {
                SCREEN_MAP.insert(px, bytes[k as usize], 0);
            }
        }
    }
}
```

This is effectively a **write-through cache** pattern: SCREEN_MAP is a byte-addressable projection of the word-addressable RAM_MAP framebuffer region, maintained coherently on every write. The overhead is 4 additional BPF map updates per word write in the screen region — acceptable given that screen writes are a small fraction of total memory operations.

An earlier design attempted to implement `SYS_DRAW_FRAME` as a bulk copy (iterating 16,000 words from RAM to SCREEN), but the 16K-iteration loop exceeded the BPF verifier's jump complexity limit. The write-through approach amortizes the copy across individual store instructions.

## 5. Userspace Components

### 5.1 doom-bridge

A Go WebSocket server that bridges kernel BPF maps to browser clients:

**Screen polling** uses `BPF_MAP_LOOKUP_BATCH` (Linux 5.6+) to read all 64,000 SCREEN_MAP entries in a single syscall. This is critical for performance — 64,000 individual `BPF_MAP_LOOKUP_ELEM` calls would take ~64ms at ~1 microsecond per call, exceeding the 16ms frame budget. Batch lookup completes in <1ms.

**Frame format**: Binary WebSocket frames: `[0x01 tag byte] + [64,000 palette index bytes]`. The bridge sends raw palette indices, not RGBA — this reduces bandwidth by 4x (64 KB vs 256 KB per frame) and defers the palette lookup to the client.

**Stats polling** at 2 Hz reads CPU_MAP and STATS, broadcasting JSON to clients for the HUD overlay.

**Keyboard input** writes directly to pinned KBD_MAP via `BPF_MAP_UPDATE_ELEM`.

### 5.2 doom-go-injector (mjolnir)

High-performance packet injector using AF_PACKET raw sockets. In fast mode:

```go
for i := 0; i < count; i++ {
    unix.Sendto(fd, pkt[:], 0, sll)
}
```

Achieves ~460,000 packets/second on the test system (aarch64, kernel 6.17). This translates to:

```
460,000 pkt/s × 256 insns/tick × 255 bounces = ~30 × 10⁹ insns/s (theoretical)
```

Actual throughput is lower due to XDP processing overhead per bounce, but measured Doom frame rates of ~20 fps at full 320x200 resolution are consistent with ~20-30M effective instructions per second.

### 5.3 Viewer (demos/doom/index.html)

Vanilla JavaScript. Contains Doom's original PLAYPAL first palette (768 bytes, extracted from doom1.wad). Renders via `putImageData` on an offscreen 320x200 canvas, scaled to display via `drawImage` with `imageSmoothingEnabled = false` for crisp pixel art.

## 6. Performance Analysis

### 6.1 Instruction Budget Per Frame

Measured empirically during Phase 9 (D_DoomLoop running):

```
Total instructions: 819,000,000
Total frames: 559
Mean instructions/frame: 1,465,116
```

This varies significantly by scene complexity:
- **Title screen** (static image): ~200K insns/frame
- **Demo playback** (full 3D rendering): ~1.5-2M insns/frame
- **Menu navigation**: ~500K insns/frame

### 6.2 Bottleneck Analysis

```
Injection rate:    460,000 pkt/s (measured)
Insns/packet:      256 × 255 = 65,280 (theoretical max)
XDP overhead:      ~50% (parsing, map lookups, state save/restore)
Effective insns:   ~32,000/packet
Effective rate:    460K × 32K = ~14.7 × 10⁹ insns/s

Frame budget:      1,470,000 insns/frame
Theoretical FPS:   14.7B / 1.47M ≈ 10,000 fps ???
```

The theoretical figure is clearly wrong — actual FPS is ~20. The discrepancy arises from:

1. **XDP_TX turnaround latency**: Each bounce incurs ~0.5-1 microsecond of kernel overhead (veth forwarding, XDP dispatch). At 255 bounces, that's ~200 microseconds of overhead per packet — comparable to the execution time.

2. **BPF map access latency**: Each instruction requires 1 ROM read + 0-2 RAM reads/writes. Array map access is O(1) but involves a function call through the BPF helper interface (~50ns per call). At 256 instructions with ~1.5 map ops each, that's ~384 helper calls × 50ns = ~19 microseconds per tick.

3. **Socket buffer pressure**: AF_PACKET sendto() at maximum rate saturates the kernel's per-socket buffer. Backpressure causes the injector to block on sendto(), reducing effective injection rate.

4. **Single-threaded serialization**: Only one packet is active in the ring at a time (by design, for correctness). This means only one CPU core executes the XDP program at any instant, leaving other cores idle.

### 6.3 Scaling Strategies

**Vertical scaling** (more work per tick):
- Increase MAX_INSN_PER_TICK: Limited by BPF verifier. Current: 256. Theoretical max with current opcode set: ~384 (untested).
- Reduce opcode decoder complexity: Fewer opcodes = fewer verifier branches = higher loop bound.

**Horizontal scaling** (more concurrent execution):
- Multiple independent rings with separate map instances — limited by available cores.
- Pipelining: Split ROM into stages, each ring handles one stage. Requires careful state partitioning.

**Bus speed** (faster packet injection):
- XDP_REDIRECT between interfaces instead of XDP_TX bounce — moves packet through ring without kernel IP stack.
- AF_XDP (user-space packet I/O) for zero-copy injection.
- Multiple injector threads with per-CPU packet buffers.

## 7. Correctness Considerations

### 7.1 RAM_MAP: HashMap to Array Migration

The original RAM_MAP used `HashMap<u32, u32>` with 8M entries. Under sustained execution, the HashMap silently dropped writes when approaching capacity — no error, no backpressure, just lost data. This manifested as data corruption after ~99.4M instructions: a CALLR instruction read a corrupted function pointer from RAM, looked up a garbage address in RV2MBC_MAP, and halted.

The fix (commit 3a1bbe7) replaced RAM_MAP with `Array<u32>` at 16M entries (128 MiB kernel allocation). BPF Array maps pre-allocate all entries and guarantee writes always succeed. The tradeoff is higher initial memory usage, but correctness is non-negotiable.

### 7.2 Doom Hardening Patches

Running Doom in a constrained VM with no filesystem, no OS, and corrupted WAD access required 14 defensive patches to prevent cascading failures:

- **I_Error made non-fatal**: Doom's error handler calls `exit()`, which translates to `ebreak` → HALT. Instead, errors are logged to a debug buffer and execution continues.
- **Zone allocator NULL returns**: `Z_Malloc` returns NULL on failure instead of infinite retry loop.
- **Bounds-checked rendering**: All draw functions (R_DrawColumn, R_DrawSpan, V_DrawPatch, etc.) early-return on out-of-bounds coordinates.
- **Virtual time**: `DG_GetTicksMs()` returns a monotonically incrementing counter (`static ticks++`) instead of wall-clock time, preventing the catch-up spiral that occurs when game time falls behind real time.

### 7.3 Stale Data Hazard

The `doom_data.bin` intermediate file (extracted `.rodata`/`.data` sections) is only regenerated if missing. When the `.text` section changes size (e.g., adding libc stubs), the `.rodata` VMA shifts, but the stale `doom_data.bin` contains data at the old offset. Jump tables in `.data` then point to incorrect ROM locations, causing JMPR faults with garbage addresses (e.g., ASCII "LMNO" = 0x4F4E4D4C interpreted as a RISC-V address).

**Fix**: Always `rm -f doom_data.bin` before loading.

## 8. Theoretical Implications

### 8.1 eBPF as a Computational Substrate

This work demonstrates that eBPF XDP programs, when combined with persistent BPF maps and an external clock source, form a **Turing-complete computational substrate** within the Linux kernel. The BPF verifier's bounded-loop requirement does not prevent Turing completeness — it merely requires that the unbounded computation be factored into bounded steps across multiple invocations (packet arrivals).

This is analogous to the distinction between a Turing machine (unbounded tape, single execution) and a **reactive system** (bounded computation per event, unbounded events over time). The BPF verifier ensures each step terminates; the packet injector ensures steps keep arriving.

### 8.2 Network-as-Computer

The architecture inverts the traditional relationship between networking and computation. Rather than using computation to process network data, we use network events to drive computation. The network topology (ring of namespaces) is the processor interconnect, and the protocol headers (IPv6 Flow Label, Hop Limit) are control signals.

This has potential applications beyond novelty:
- **In-network computation**: Processing data at line rate without leaving the kernel
- **Hardware offload prototyping**: BPF programs can be offloaded to SmartNICs (XDP offload mode), moving the entire computation to the NIC
- **Distributed state machines**: Multiple hosts sharing BPF map state via BPF map-in-map or distributed map backends

### 8.3 The Bus Metaphor Formalized

Let B = (V, E, M, P, C) be the compute architecture where:
- V = {monad₀, ..., monad₅} — namespace vertices
- E = {(monadᵢ, monadᵢ₊₁ mod 6)} — directed veth edges
- M = {ROM, RAM, CPU, SCREEN, KBD, RV2MBC} — shared memory (BPF maps)
- P: Packet → ℕ — the tick function (packet arrival → N instructions executed)
- C: M × Instruction → M — the state transition function

The system evolves as: M(t+1) = C(M(t), ROM[CPU(t).pc])

The packet ring provides the temporal ordering (clock), the maps provide the spatial state (memory), and the XDP program provides the transition function (ALU). The injector's packet rate determines the clock frequency, making this a **variable-frequency processor** whose speed is controlled by an external agent — not unlike dynamic voltage and frequency scaling (DVFS) in modern CPUs, but at the architectural rather than physical level.

---

*Previous: [← High School Explanation](03-high-school.md)*

---

## References

- [1] McCanne, S., Jacobson, V. "The BSD Packet Filter: A New Architecture for User-level Packet Capture." USENIX Winter 1993.
- [2] Hoiland-Jorgensen, T. et al. "The eXpress Data Path: Fast Programmable Packet Processing in the Operating System Kernel." ACM CoNEXT 2018.
- [3] id Software. "DOOM Source Code." 1997 (GPL release). https://github.com/id-Software/DOOM
- [4] Aya Project. "Aya: eBPF programs in Rust." https://aya-rs.dev/
- [5] Vieira, M.A.M. et al. "Fast Packet Processing with eBPF and XDP." ACM Computing Surveys, 2020.
