<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-108: MBC cartridges — small signed network functions run per flow by a UPC interpreter

**Status:** Proposed (RFC Editor items 1-5 ruled in S-TRUE-WIRE Steps 29-50, 2026-10-27; acceptance is Stevie's, E1).
**Date:** 2026-10-23
**Sprint:** S5 S-UPC-WIRE-COMPUTE, Part 2 (`docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`, Phases 9–19).
**Related:** ADR-080 (UPC programmatic API: cartridges are its third client, after Doom and xv6), ADR-067 (MBC ISA v2; no opcode is added here), ADR-043 (Gungnir ML-DSA-65 seals), ADR-105 (`pkg/wotansig` signing), ADR-062 (Lich process), ADR-004 (no new external deps).

## Context

ADR-080 names the UPC a substrate for "future experimental programmatic
expansion" and fixes its rules: each workload carries only its own syscall
surface, every build variant is load-tested, and each build fits the
kernel's 1,000,000 processed-instruction verifier ceiling on its own. The
two existing clients are whole guests (Doom, xv6) driven by trigger ticks.

A **cartridge** is the opposite shape: a few dozen MBC instructions plus a
read-only data blob (for example int8 model weights) and a signed
manifest, run once per packet of a flow, or once per queued flow. Three are
planned: a token-bucket rate limiter, a 1:N flow sampler, and an int8
anomaly scorer that feeds `services/anomaly`.

Native eBPF versions of similar functions already exist (`ebpf/qos-ebpf`,
`ebpf/anomaly-ebpf`, `ebpf/nfv-ebpf`). They are the correctness oracles and
speed baselines. A cartridge is slower by construction (it is interpreted);
what it buys is that a new network function is data (signed, swappable,
replayable bit-for-bit) rather than a new BPF program that must pass the
verifier again.

The ascend `monad-cpu-ebpf` build is at 926,542 processed instructions
(92.7 % of 1M, S5 Phase 0). There is no room for a cartridge surface in it.

## Decision

### D-C1 — The interpreter lives in its own program; the step function is shared

- `ebpf/cartridge-ebpf` is a new BPF object with two programs, `cart_xdp`
  and `cart_tick`, and its own verifier-budget gate (≤ 500,000 processed
  instructions per program, half the ceiling).
- The instruction step is one `no_std` function, `monad_common::cart::step`,
  compiled into both the BPF object and the host tools
  (`crates/upc-cartridge`). One source for both engines gives replay
  determinism by construction.
- `monad-cpu-ebpf` is not touched. Doom and xv6 objects stay byte-identical
  (ADR-080's non-negotiable). The regression harness load-tests the
  cartridge object as a third variant.

### D-C2 — Two execution stages

| Stage | Program | Runs | Budget | Branches |
|---|---|---|---|---|
| XDP (0) | `cart_xdp` | once per packet, per active XDP slot | `max_insn` ≤ `N_PKT` = 64 | forward only (validator rejects back-edges) |
| Tick (1) | `cart_tick` | once per queued flow job, driven by the host through `BPF_PROG_TEST_RUN` (the Doom injector model; no live packets) | `max_insn` ≤ `N_TICK` = 256 per call, resumable | back-edges allowed, at most `loop_bound` taken per job |

An int8 MLP over every packet does not fit an XDP budget; scoring once per
flow off the packet path keeps the XDP cost flat and still scores every
sampled flow.

### D-C3 — Faults fail open, loudly

Any fault (bad opcode, out-of-window memory, bad PC, a syscall the stage does
not allow, XDP budget exhausted, tick loop bound exceeded) ends the run with
`XDP_PASS`, increments `faults` for the slot, and counts toward a per-slot
breaker: **64 faults inside one second disables the slot** (`active = 0`).
Re-enabling is an explicit `upc-cartridgectl` command. A buggy rate limiter
must never black-hole traffic.

Override: a manifest may set `fail_closed` (header flag bit 0) for policy
cartridges; a fault then yields `DROP`. The loader logs it at load time.

### D-C4 — Sophia holds the registry

Registry record: name → `{sha256, manifest, seal key id, stage, surface
version, max_insn, slot}`. `upc-cartridgectl load <name>` resolves the name,
reads the image once into memory, verifies the Gungnir ML-DSA-65 seal on
those bytes, validates the same bytes, and writes `CART_ROM`/`CART_DATA` for
the slot from the same buffer (no re-read, no TOCTOU). Records live under a
Sophia dictionary namespace requested from the Sophia Root Dictionary Keys
registry (0x10–0xFE, First Come First Served). **Until it is assigned the
namespace is the provisional string `x-cartridge`.** (Ruled 2026-10-27: root key
`0x15`, category `cartridge`; since S-TRUE-WIRE Steps 33-35 the code writes
`cartridge/<name>` and still reads `x-cartridge/<name>` records for one release.)
While the stack is down
(KF5) a file-backed registry (`--registry-dir`, one JSON record per
cartridge, same schema) is used.

### D-C5 — Anamnesis and the wire

- Trace records reuse `EVENT_COMPUTED (0x01)` ("Shim executed") with a
  cartridge sub-kind in the payload. No new Anamnesis Event Type is
  registered.
- Cartridges never write the Monad header in v0. They read three Monad
  fields (FIELD ids 10–12) and nothing else. In particular the
  `anomaly-ebpf` overload of `SAMPLED` (0x08) as "suspicious" is not copied:
  scores travel over a ringbuf to Wotan.
- Monad v0x01 is unchanged. No MBC opcode is added.

## Cartridge ABI v0

All multi-byte integers are little-endian. "Word" = 32 bits.

### Image (`.cart`)

```
off  size  field
 0    4    magic "MBCC"
 4    1    version (0)
 5    1    stage (0 = xdp, 1 = tick)
 6    2    max_insn      xdp: 1..=64 ; tick: 1..=256 (per call)
 8    2    rom_words     1..=1024
10    2    data_words    0..=1024  (data blob <= 4096 bytes)
12    2    entry         < rom_words
14    2    loop_bound    xdp: must be 0 ; tick: max taken back-edges per job
16    2    flags         bit 0 = fail_closed ; other bits must be 0
18    2    reserved      must be 0
20   32    sha256 over bytes [0,20) || ROM || DATA
52   4*rom_words   ROM  (MBC instruction words)
 .   4*data_words  DATA (read-only words)
```

The file is exactly `52 + 4*(rom_words + data_words)` bytes. The Gungnir
seal (`<file>.seal`, JSON `gungnir.GungnirSeal`) covers the whole file.

### Machine state (`CartState`, 336 bytes, `#[repr(C)]`, no implicit padding)

```
regs[16]  u32   @0     general registers r0..r15
mem[64]   u32   @64    flow window, byte addresses 0x0000..0x00FF
pc        u16   @320   word index into ROM
flags     u8    @322   Z=0x01 N=0x02 C=0x04 (MBC set_flags rules)
status    u8    @323   0 idle, 1 yielded (tick), 2 done
insns     u32   @324   instructions executed in this run/job
back_edges u32  @328   taken back-edges in this job (tick)
gen       u32   @332   slot generation the state belongs to
```

`state_hash` = 32-bit FNV-1a (offset basis 0x811c9dc5, prime 0x01000193, the
`upc-bootctl` `fnv1a`) over `regs` (64 bytes LE) ‖ `pc` (2 bytes LE) ‖ `flags`
(1 byte) ‖ `mem` (256 bytes LE).

Per run: an XDP-stage run starts with `regs`, `pc = entry`, `flags`,
`insns` zeroed; `mem` persists per flow and slot. A tick job starts the same
way with `mem[0..8]` = the job's 8 feature words and `mem[8..64]` zero; a
yielded job resumes with its whole state.

### Instructions

Encoding is MBC's: `opcode[31:24] dst[23:20] src[19:16] imm16[15:0]`.
Semantics are the shipping engine's (`monad-cpu-ebpf`): PC advances before
execution; branches add a 24-bit signed offset (bits 23..0) to `pc + 1`;
`MOVI` zero-extends; `LOAD_IMM32` sets bits 31..16 and keeps 15..0; `ADD`,
`SUB`, `ADDI` set C from the unsigned carry/borrow; `MOV`, `MOVI`,
`LOAD_IMM32` leave flags alone; `CMP` sets Z/N/C from `dst - src`.

Allowlist (anything else faults `BadOpcode`):

```
NOP 00  ADD 01  SUB 02  MUL 03  AND 07  OR 08  XOR 09  NOT 0A
SHL 0B  SHR 0C  SAR 0D  MOV 0E  MOVI 0F  CMP 10  LOAD_IMM32 1C  ADDI 1D
JMP 20  JZ 21  JNZ 22  JN 23  JP 24  JC 25  JNC 26
LD 30  ST 31  LDB 32  SHLR 36  SHRR 37  SARR 38  MULH 39
SYSCALL 40  HALT FF
```

No `CALL`/`RET`/`JMPR`/`CALLR` (no indirect control flow), no `DIV`/`MOD`, no
stack, no atomics, no privileged ops. A branch target outside the ROM faults
`Pc`; running off the end of the ROM faults `Pc`.

### Memory windows

Effective address = `regs[base] + sign_extend(imm16)` (byte address).

| Range | Maps to | Access |
|---|---|---|
| `[0x0000, 0x0100)` | `mem[(addr >> 2)]` | `LD`, `ST`, `LDB` |
| `[0x1000, 0x1000 + 4*data_words)` | `DATA[(addr - 0x1000) >> 2]` | `LD`, `LDB` |
| anything else, a store to DATA, or a misaligned `LD`/`ST` | — | fault `Mem` |

`LDB` returns byte `addr & 3` of the word, little-endian (engine semantics).

### Syscalls (`SYSCALL imm16`; arguments r1..r3, result r0)

| # | Name | Stage | Effect |
|---|---|---|---|
| 0 | `FIELD` | xdp | `r0 = field[r1]`; unknown id faults `Field` |
| 1 | `NOW_NS_LO` | both | `r0` = low 32 bits of the run's clock |
| 2 | `VERDICT` | xdp | `r1`: 0 PASS, 1 DROP; ends the run. Other codes fault `Verdict` (2 = TX is reserved for v1) |
| 3 | `EMIT` | both | push `{kind = r1, a = r2, b = r3}` to `CART_EVENTS`; a full ringbuf is counted, not a fault |
| 4 | `QUEUE` | xdp | push a ScoreJob of `mem[r1>>2 .. +8]` (r1 a byte address in the flow window, 4-aligned, `r1 + 32 <= 0x100`) |
| 5 | `YIELD` | tick | suspend; the next `cart_tick` call resumes the job |

A syscall not allowed in the stage faults `Syscall`. `HALT` ends the run:
PASS in the XDP stage, job done in the tick stage. Reaching `max_insn` in
the XDP stage faults `Budget` (= PASS, counted). Reaching it in the tick
stage is an implicit `YIELD`. A tick job that takes more than `loop_bound`
back-edges faults `LoopBound`, which bounds every job at
`rom_words × (loop_bound + 1)` instructions.

Verdicts across active XDP slots combine most-restrictive (DROP wins).

### FIELD ids (metadata only)

```
 0 l3_proto      4, 6, or 0 (not IP)
 1 l4_proto      IP protocol / IPv6 next header (after an optional HbH)
 2 src_port      TCP/UDP, else 0
 3 dst_port      TCP/UDP, else 0
 4 pkt_len       frame length in bytes
 5 ip_ttl        IPv4 TTL / IPv6 hop limit
 6 tcp_flags     TCP flags byte, else 0
 7 iat_bucket    floor(log2(inter-arrival µs + 1)), capped at 31; 0 on a flow's first packet
 8 flow_pkt_count   packets in this flow, including this one
 9 flow_byte_count  bytes in this flow, including this one (saturating)
10 monad_present 1 if an IPv6 HbH header immediately follows the fixed header and its first option is the Monad option (0x3E, length 20)
11 monad_qos     Monad qos_class byte (offset 0x08), else 0
12 monad_flags   Monad flags byte (offset 0x0B), else 0
```

**No id returns payload bytes, and no syscall reads the packet.** This is the
Sacred Principle (zero user data access) by construction: no cartridge,
event, trace or replay can carry user data, because the ABI has no way to
reach it.

### Records

```
CartEvent (CART_EVENTS ringbuf, 64 B):
  ts_ns u64 @0 | flow FlowKey @8 (40 B) | kind u32 @48 | a u32 @52 | b u32 @56
  | slot u8 @60 | stage u8 @61 | rsv u16 @62
FlowKey (40 B):
  src [u8;16] @0 | dst [u8;16] @16 (IPv4 in the first 4 bytes, rest 0)
  | sport u16 @32 | dport u16 @34 | proto u8 @36 | l3 u8 @37 | slot u8 @38 | rsv u8 @39
ScoreJob (SCORE_QUEUE, 80 B):
  flow FlowKey @0 | feat [u32;8] @40 | src_slot u8 @72 | rsv u8 @73 | rsv u16 @74 | enq_ns_lo u32 @76
SlotCfg (CART_SLOT, 20 B):
  active u8 @0 | stage u8 @1 | flags u8 @2 | rsv u8 @3 | max_insn u16 @4 | rom_words u16 @6
  | data_words u16 @8 | entry u16 @10 | loop_bound u16 @12 | rsv u16 @14 | gen u32 @16
```

Four slots; `CART_ROM` and `CART_DATA` hold 1024 words per slot.

Trace records (S5 Phase 16, `CART_TRACE` ringbuf 1 MiB; `CART_CFG[0]` = trace
one run in N, 0 = off, the default), all `EVENT_COMPUTED (0x01)` + sub-kind
`0x43`, octets 0–2 = event, sub-kind, record type (0 head, 1 step, 2 end):

```
TraceHead (464 B): stage u8 @3 | slot u8 @4 | gen u32 @8 | run_id u64 @16 | now u64 @24
  | flow FlowKey @32 | fields [u32;13] @72 | state CartState @128 (at run start)
TraceStep (32 B):  idx u32 @4 | run_id u64 @8 | pc u32 @16 | insn u32 @20 | code u32 @24
  | state_hash u32 @28 (after the instruction)
TraceEnd  (32 B):  insns u32 @4 | run_id u64 @8 | code u32 @16 | state_hash u32 @20
```

`code`: 0 continue, 0x100|v verdict, 0x200 yield, 0x300 halt, 0x400|k fault k.
Replay contract: `docs/architecture/cartridge-replay.md`.

## Gates

| Gate | Bar |
|---|---|
| Verifier budget | each of `cart_xdp`, `cart_tick` ≤ 500,000 processed instructions; monad-cpu-ebpf unchanged |
| Load-test | regression harness loads the cartridge object as a third variant |
| Determinism | BPF and host reference reach the same `state_hash` on the same inputs (100 %) |
| Benchmarks (Phases 13–15) | rate limiter ≤ 150 ns/pkt over an empty-pass baseline and ≤ 3× `qos-ebpf`; sampler ≤ 120 ns/pkt; scorer ≥ 100,000 flows/s on one core with the XDP path unchanged |

## Determinism contract

`state_{n+1} = step(state_n, rom, data, inputs_n)`, where the only inputs are
the FIELD values and the clock value of the run. With the image, the data and
the input log, any engine running `monad_common::cart::step` reproduces every
state hash. The BPF program and the host reference compile the same source.

## Threat model

From the interpreter's side a cartridge is untrusted code.

- **Termination**: XDP-stage runs are forward-only and capped at 64
  instructions; tick jobs are capped per call and by `loop_bound`.
- **Memory**: two windows, bounds-checked on every access; DATA is
  read-only; there is no pointer into a map, the packet, or the stack.
- **Control flow**: direct branches only, targets checked against the ROM.
- **Surface**: allowlisted opcodes and stage-gated syscalls; no BPF helper
  is reachable from MBC; no payload access.
- **Supply**: `upc-cartridgectl` refuses an image whose Gungnir seal does not
  verify against the pinned key, and refuses an image the validator rejects,
  checking the same in-memory bytes it then writes. `--lab` loads unsigned
  images with a warning and a `lab="1"` label on every metric.
- **Availability**: faults fail open (D-C3) with a per-slot breaker; queue
  and ringbuf overflow are counted drops, never faults; per-flow state is in
  an LRU map, so a flow cannot starve another.
- **Who can load**: writing the maps needs CAP_BPF; pinned maps are 0600 and
  the registry directory 0700.

## Signing and loading (as built, S5 Phase 12)

- Lab key: a 32-byte ML-DSA-65 seed, mode 0600, outside the repo
  (`head -c 32 /dev/urandom`); public key via
  `yggdrasil-evidence pubkey --key SEED --out PUB`. Where a non-lab key
  lives is the operator's decision (ADR-043 #4).
- `upc-cartridgectl sign --key SEED --ttl D X.cart` writes `X.cart.seal`
  (`gungnir.SignFor`, TTL ≤ 31 days).
- `register` and `load` read the image once and run, on those bytes: the
  seal against the pinned `--pubkey`, `cart validate -` on stdin, the ABI v0
  decode; `load` also matches the registry record's sha256. Only then are
  the bytes written to the slot (config and generation last).
- Registry storage (Sophia or `--registry-dir`) is untrusted: a swapped blob
  fails its sha or its seal.
- Pins: maps and programs under one bpffs directory (0700 dir, 0600 pins);
  registry directories 0700, files 0600. Writing the maps needs CAP_BPF.
- Recipe and commands: `cartridges/README.md`, `wiki/UPC-Cartridges.md`.

## Non-goals (v0)

No Monad wire change. No new MBC opcode (a `DOT4` for the scorer would be a
full ISA spec gate, a separate sprint, [ESCALATE]). No packet rewrite or
redirect verdict. No dashboard page. No deb/sysusers packaging.

**DOT4, measured case (S5 Phase 15, deferred, not added).** The scorer built
from existing opcodes spends 24 of ~41 instructions per hidden row on the
inner product (`LDB; MUL; ADD` per weight), about 720 instructions per job in
3 `cart_tick` calls, 251,572 flows/s on west (gate 100,000: met). A `DOT4 rd,
rs, rt` (rd += the dot product of four u8 lanes) would make each row two word
loads and two DOT4s: about 420 instructions per job, 2 calls, roughly 1.7x
the throughput. The gate is met without it, so it stays a proposal for the
ISA's own spec gate (MBC Opcode Registry, Standards Action).

## Consequences

- A network function becomes signed data with a replayable execution
  record. New functions need no new verifier pass.
- Cartridges are slower than native eBPF by a fixed factor; the benchmark
  gates measure it instead of hiding it.
- The cartridge surface is a third cfg-partition-style client of ADR-080,
  in its own object rather than a `cfg` feature, so the Doom/xv6 budgets
  never see it.

## Status at S5 close (2026-10-23)

Still **Proposed**: RFC Editor items 1–5 have no answer, so the provisional names stay.
Measured against the gates above (`references/s5-upc-wire-log.md`, closing summary):

| Gate | Result |
|---|---|
| Verifier budget | cart_xdp 42,743, cart_tick 146,897 (with the trace path); monad-cpu-ebpf unchanged |
| Load-test | regression harness third variant green |
| Determinism | BPF == host reference on every check; three engines (shared step, host MBC emulator, BPF) agree on 1,000/1,000 random cartridges; replay bit-exact |
| Rate limiter | +193 ns over empty-pass: **fail** (bar 150); 1.25× `qos-ebpf`: pass |
| Sampler | +216 ns: **fail** (bar 120) |
| Scorer | 232,450 flows/s, XDP path unchanged: pass |

The two failures are the interpreter's cost, about 3–7 ns per MBC instruction on west,
not the cartridges' logic. The rate limiter follows the `qos-ebpf` refill fix
(`8ab534b3`, the remainder of `elapsed / 8000` carries), which it found. The scorer's
model is trained on synthetic flows only. Differential and fuzz hooks for the Lich:
`tomb/lich/UPC-CARTRIDGE-HOOKS.md`.

**Benchmark gates #1(a) and #2: accepted 2026-10-28 (S-ASCEND-2 Steps 243-244, decision default).** The
rate-limiter (+193 ns, bar 150) and sampler (+216 ns, bar 120) misses are the documented cost of interpreted
cartridges. Callback batching and DOT4 are not funded; a new MBC opcode needs its own ISA spec gate and sprint.
Reopen only if "cartridges at line rate" is funded explicitly. #1(b) and #3 pass as above.

## RFC Editor items (ruled 2026-10-27, S-TRUE-WIRE Phase 1)

See `docs/protocol/RFC-EDITOR-REQUESTS-S5.md`. (a) Sophia root key `0x15`
`cartridge` (COMPOSITE), entry schema and the loader rule (verify the seal over
the exact bytes; an entry locates, never authorizes) in sophia-dictionary;
(b) sub-kind `0x43` CARTRIDGE_TRACE under `EVENT_COMPUTED`, in a new
EVENT_COMPUTED Sub-kinds registry in foundation; sub-kinded records live on
their own stream (`CART_TRACE`), never inside the 32-octet event record;
(c) `anomaly.scores` and `upc.cart.*` registered in the Wotan Topic Namespace,
which wotan-memory now requests rather than leaves optional. No Monad byte and
no MBC opcode changed. The code moved to `cartridge` (S-TRUE-WIRE Steps 33-35,
`x-cartridge` read as an alias); what is left is acceptance (E1).
