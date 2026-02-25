# Running Doom Over IPv6 Hop-by-Hop Options: Implementation Plan

**Project:** Unheaded: Protocol Foundation -- Computational Completeness PoC
**Date:** February 18, 2026
**Owner:** Architect + Developer
**Priority:** P1 (after Alpha ships)
**Estimated Duration:** 8-13 days total

---

## Executive Summary

The Unheaded Protocol Internet-Draft proves the Monad architecture
provides the five primitives any assembler requires: registers, ALU, addressable
memory, I/O, and a clock. This plan turns that proof into a running demo: Doom
executing on a Kingdom network where packets are the CPU and Wotan is the RAM.

---

## Core Architecture: Monad = CPU, Wotan = Memory

The critical insight: the Monad stays PURE COMPUTE (20 bytes of registers at wire
speed). All memory goes through Wotan's ring buffer infrastructure, which already
has backpressure, pub/sub, configurable allocation, and persistence (WAL).

```
MEMORY HIERARCHY:

L0: Monad Scratch[0..3]           4 bytes      wire speed
    CPU registers. Travel with the packet.
    Never gets heavier. Pure compute bus.

L1: Per-hop BPF map cache         variable     ~100-200 ns
    Prefetched by Wotan before packet arrives.
    Hot working set for current instruction window.

L2: Wotan ring buffer            --ring-size  ~1-10 us
    Per-flow channel keyed by Flow Label.
    This IS the RAM. Configurable: 1KB to 4GB.
    Wotan already has backpressure + pub/sub.

L3: Wotan WAL                    disk         ~100 us - 1 ms
    Persistent storage. Pages swap in/out of L2.
    This IS the disk. Already planned for Age 2.

L4: Sophia dictionaries           BPF maps     ~100-200 ns
    Instruction decode ONLY. Not data memory.
    Microcode, not RAM.
```

### Why Wotan for Memory (Not Raw BPF Maps)

| Feature | Raw BPF Maps | Wotan Ring Buffer |
|---------|-------------|-------------------|
| Size limit | Kernel memory (~256MB practical) | --ring-size argument, scales to GB |
| Persistence | None (lost on program reload) | WAL (durable) |
| Backpressure | None (drops on full) | Built-in (ring buffer semantics) |
| Multi-consumer | Requires map-of-maps complexity | Pub/sub topics (already built) |
| Access pattern | O(1) hash/array | O(1) ring buffer index |
| Configuration | Compile-time max_entries | Runtime --ring-size flag |
| I/O bridging | Requires custom userspace reader | Wotan already bridges to dashboard |

### How It Works

1. **Packet arrives at hop** with Monad carrying Scratch[0..3] (hot registers)
2. **eBPF reads instruction** from Sophia rom_map (read-only BPF array, stays in kernel)
3. **If instruction needs memory read:** eBPF reads from local BPF map cache (L1)
   - Cache hit: ~100ns, proceed
   - Cache miss: eBPF emits `compute.mem.miss` event to Anamnesis
     Wotan picks it up, stages the page from ring buffer (L2) into L1
     Next packet circulation finds it cached
4. **If instruction needs memory write:** eBPF writes to local BPF map cache (L1)
   AND emits `compute.mem.write` event with address + value
   Wotan updates ring buffer (L2) asynchronously
5. **Packet exits hop** with updated Scratch registers in the Monad

For Doom's access patterns (sequential screen writes, stack operations, heap
allocations), the L1 cache hit rate should be >95% once Wotan's prefetch
warms up. Cache misses cost one extra packet circulation (~500ns per hop ring).

---

## Instruction Set: Monad Bytecode (MBC)

Minimal 32-bit fixed-width register VM, designed for eBPF execution:

```
Register file: r0-r15 (32-bit), PC, SP (alias r15), flags (Z/N/C)

Instruction encoding:
  [opcode:8][dst:4][src:4][imm16:16]

Core opcodes (~40 instructions):
  Arithmetic:  ADD, SUB, MUL, DIV, MOD, NEG
  Logical:     AND, OR, XOR, NOT, SHL, SHR, SAR
  Comparison:  CMP (sets flags)
  Branch:      JMP, JZ, JNZ, JN, JP, JC, JNC, CALL, RET
  Memory:      LD, ST, LDB, STB, LDH, STH
  Register:    MOV, MOVI
  System:      SYSCALL (Wotan I/O), HALT
```

**Compilation:** doomgeneric C source -> RISC-V RV32I (existing gcc toolchain)
-> trivial RV32I-to-MBC translator -> rom_map bytecode

---

## Phase D1: Assembler Substrate (2-3 days)

### D1.1: eBPF CPU Program (Day 1-2)

Per-hop XDP program implementing fetch-decode-execute:

```c
SEC("xdp")
int monad_cpu(struct xdp_md *ctx) {
    struct monad *m = locate_monad(ctx);
    u32 flow = get_flow_label(ctx);
    struct cpu_state *cpu = bpf_map_lookup_elem(&cpu_map, &flow);

    // FETCH from rom_map (Sophia, read-only, stays in BPF)
    u32 *insn = bpf_map_lookup_elem(&rom_map, &cpu->pc);

    // DECODE
    u8 opcode = (*insn >> 24) & 0xFF;

    // EXECUTE (BPF ALU ops per RFC 9669)
    // ...

    // MEMORY ACCESS via L1 cache (local BPF map)
    // Cache miss -> emit event, Wotan restages from ring buffer
    if (opcode == OP_LD) {
        u32 *val = bpf_map_lookup_elem(&l1_cache, &addr);
        if (!val) {
            emit_cache_miss(ctx, addr);  // Wotan handles
            cpu->stalled = 1;            // Retry next circulation
            return XDP_PASS;
        }
        cpu->regs[dst] = *val;
    }

    // WRITE BACK to Monad Scratch registers
    m->scratch[0] = (cpu->regs[0] >> 8) & 0xFF;
    m->scratch[1] = cpu->regs[0] & 0xFF;
    m->scratch[2] = (cpu->regs[1] >> 8) & 0xFF;
    m->scratch[3] = cpu->regs[1] & 0xFF;

    emit_anamnesis(ctx, cpu, ANAMNESIS_HOP);
    return XDP_PASS;
}
```

### D1.2: Wotan Memory Service (Day 2-3)

New Wotan channel type: `compute.mem.<flow_label>`

```go
// Wotan memory service configuration
type MemoryServiceConfig struct {
    RingSize    int64  `yaml:"ring_size"`    // --ring-size flag, e.g. "512MB"
    PageSize    int    `yaml:"page_size"`    // L1 cache page size (default 4KB)
    PrefetchN   int    `yaml:"prefetch_n"`   // Pages to prefetch around access
    WALEnabled  bool   `yaml:"wal_enabled"`  // Persist to disk
    WALPath     string `yaml:"wal_path"`     // WAL file location
}

// Wotan subscribes to compute.mem.miss and compute.mem.write events
// and manages the L1 BPF map cache + L2 ring buffer
func (ms *MemoryService) HandleCacheMiss(event CacheMissEvent) {
    // Load page from ring buffer (L2)
    page := ms.ringBuffer.ReadPage(event.FlowLabel, event.Address)
    // Stage into L1 BPF map cache on the target hop
    ms.bpfCache.Update(event.HopID, event.Address, page)
    // Prefetch adjacent pages (spatial locality)
    for i := 1; i <= ms.config.PrefetchN; i++ {
        adjPage := ms.ringBuffer.ReadPage(event.FlowLabel, event.Address + uint32(i * ms.config.PageSize))
        ms.bpfCache.Update(event.HopID, event.Address + uint32(i * ms.config.PageSize), adjPage)
    }
}
```

### D1.3: Packet Circulation Ring (Day 2-3)

Single-machine PoC using network namespaces + veth pairs:

```
6 namespaces, each running monad_cpu.bpf at XDP:

  ns0 --veth01--> ns1 --veth12--> ns2
   ^                                |
   |                                v
  ns5 <--veth54-- ns4 <--veth43-- ns3

  6 hops per circuit = 6 instructions per round-trip
  Static routes create the directed ring
```

---

## Phase D2: Doom Port (3-5 days)

### D2.1: Cross-Compilation (Day 3-4)

```bash
# Compile doomgeneric to RISC-V
riscv32-unknown-elf-gcc -O2 -o doom.elf doomgeneric/*.c

# Translate RV32I to MBC bytecode
./rv32i-to-mbc doom.elf > doom.mbc

# Load into Sophia rom_map via Wotan
wotan-ctl load-rom --flow-label 0x0A3F7E --file doom.mbc
```

### D2.2: doomgeneric Callbacks via SYSCALL -> Wotan

```
SYSCALL 0x01 (DG_DrawFrame):
  r1 = framebuffer address in Wotan RAM
  eBPF emits compute.screen.write event
  Wotan reads 64000 bytes from ring buffer -> dashboard

SYSCALL 0x02 (DG_GetKey):
  eBPF reads from Wotan kbd topic -> r0=key, r1=pressed

SYSCALL 0x03 (DG_GetTicksMs):
  r0 = bpf_ktime_get_ns() / 1000000

SYSCALL 0x04 (DG_SleepMs):
  cpu->sleep_until = now + r0 * 1000000
```

### D2.3: WAD Loading via Wotan

```bash
# Load doom1.wad into Wotan ring buffer at base address 0x100000
wotan-ctl load-mem --flow-label 0x0A3F7E \
  --base-addr 0x100000 \
  --file doom1.wad
```

Wotan writes the WAD data into the ring buffer channel for this flow.
When Doom accesses WAD data, it reads from Wotan RAM like any other memory.

---

## Phase D3: Dashboard Integration (2-3 days)

Wotan already bridges ring buffers to the dashboard via WebSocket.
The screen output is just another Wotan topic:

- `compute.screen.0x0A3F7E` -- 320x200 framebuffer, 35Hz
- `compute.input.0x0A3F7E` -- keyboard scancodes from dashboard
- `compute.trace.0x0A3F7E` -- Anamnesis CPU debugger stream

Dashboard renders the framebuffer as a scaled canvas with the Anamnesis
CPU trace overlay showing live PC, register values, instructions/sec,
cache hit rate, Wotan ring buffer utilization.

---

## Phase D4: Documentation and Publication (1-2 days)

- Blog: "Running Doom Over IPv6: Packets as CPU, Wotan as RAM"
- Conference CFPs: eBPF Summit, Netdev 0x18, NANOG, IETF hackathon
- Update Internet-Draft with PoC results

---

## Performance Budget

```
Doom requirements:
  Frame rate: 35 FPS (original engine tick rate)
  Instructions per frame: ~50,000-100,000
  Instructions/sec needed: ~1.75M - 3.5M

Monad architecture capacity:
  Per-hop instruction time: ~300-550 ns (L1 cache hit)
  Effective clock rate: ~2-3 MHz
  With 6-hop ring: ~12-18M insn/sec distributed

L1 cache miss penalty: ~1 extra circulation (~3us with 6 hops)
Expected cache hit rate: >95% (Doom has good spatial locality)
Effective throughput with misses: ~1.8-2.8M insn/sec

Wotan ring buffer overhead:
  Read from ring buffer: ~1-10 us (in-memory)
  Write to ring buffer: ~1-10 us
  Prefetch: amortized across adjacent pages

Verdict: FEASIBLE at native Doom frame rate with margin
```

## Fallback: Progressive Demos

1. **Week 1:** "Hello World" over IPv6 HBH -- text to screen_map
2. **Week 2:** "Snake" -- game loop, keyboard, screen via Wotan
3. **Week 3:** "Pong" -- real-time collision, Wotan I/O bridging
4. **Week 4:** "Doom" -- the full demo

Even "Snake running on IPv6 extension headers with Wotan as RAM" is a headline.

---

## Success Criteria

- [ ] Doom renders >= 20 FPS to Kingdom dashboard
- [ ] Input latency < 50ms (dashboard -> Wotan -> kbd topic -> eBPF)
- [ ] Wotan ring buffer scales via --ring-size (512MB for Doom)
- [ ] L1 cache hit rate > 90%
- [ ] Anamnesis captures full execution trace
- [ ] Stable for >= 5 minutes gameplay
- [ ] Demo video recorded and publishable

**THE PLAN IS SET. THE DOOM APPROACHES. WOTAN IS THE RAM.**
