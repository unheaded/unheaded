# Protocol Specifications

## Overview

The Unheaded Kingdom defines three core protocols that work together to provide packet-level observability, inter-service communication, and semantic metadata management. All three protocols are internal to the Kingdom -- they exist only within the network boundary and cannot leak to the outside world.

---

## 1. Monad Protocol (Packet Compute)

**Specification:** `docs/protocol/PROTOCOL_FOUNDATION.md`
**RFC drafts:** `docs/protocol/draft-bellis-unheaded-protocol-foundation-01.md` through `-04.md`
**ISA reference:** `docs/protocol/mbc-isa-reference.md`

### Purpose

Monad defines a 20-byte register file carried in IPv6 Hop-by-Hop extension headers. Every packet inside the Kingdom carries this register file, readable and writable by eBPF programs at every hop at kernel datapath speed (~320 ns per read).

### Wire Format

```
IPv6 Hop-by-Hop Extension Header:
  Next Header:  (protocol-dependent)
  Hdr Ext Len:  2 (24 bytes total)
  Option Type:  0x1E (experimental, RFC 4727)
  Opt Data Len: 20
  Monad Data:   20 bytes of Sophia-encoded protocol metadata
```

The 20 bytes encode:
- Source identity (Sophia dictionary key)
- Trace hash (generated at ingress)
- QoS class (from policy BPF map)
- Hop count
- Flow flags
- Mesh context
- Service identity exponent keys

Each byte position is an exponent key -- an index into Sophia's lookup dictionaries. One byte = 256 possible meanings per key position. 20 bytes of exponentially-composed semantic space provides combinatorial metadata richness at zero parsing cost.

### MBC Instruction Set Architecture

MBC (Monad Bytecode) is the compute ISA for Monad programs. 32-bit fixed-width encoding:

```
[31..24]  opcode   (8 bits)
[23..20]  dst      (4 bits) -- destination register r0-r15
[19..16]  src      (4 bits) -- source register r0-r15
[15..0]   imm16    (16 bits) -- immediate value / address / offset
```

**Register file:** 16 general-purpose 32-bit registers (r0-r15). r14 = return address, r15 = stack pointer.

**Opcode categories:**
- Arithmetic: ADD (0x01), SUB (0x02), MUL (0x03), DIV (0x04), MOD (0x05), NEG (0x06)
- Logic: AND (0x07), OR (0x08), XOR (0x09), NOT (0x0A), SHL (0x0B), SHR (0x0C), SAR (0x0D)
- Register: MOV (0x0E), MOVI (0x0F), CMP (0x10), LOAD_IMM32 (0x1C), ADDI (0x1D)
- Stack: PUSH (0x1A), POP (0x1B)
- Control flow: JMP (0x20), JZ (0x21), JNZ (0x22), JN (0x23), JP (0x24), JC (0x25), JNC (0x26), CALL (0x27), RET (0x28), JMPR (0x29), CALLR (0x2A)
- Memory: LD (0x30), ST (0x31), LDB, STB, LDH, STH
- System: SYSCALL (0x40), HALT (0xFF), NOP (0x00)

**Branch encoding:** Branches use 24-bit PC-relative offset in the lower 24 bits. CALL uses 24-bit absolute target.

**Source of truth:** `ebpf/monad-common/src/lib.rs` module `mbc_opcodes`

### Evolution Path

- **Age 1 (current):** 20 bytes of protocol metadata per packet, IPv4 internal. Encoding mechanism is implementation detail.
- **Age 2:** IPv6 internal. The 20 bytes move into mapped-address prefix space. Flow Label (20 bits) provides trace hash correlation. Zero additional overhead.
- **Age 3:** Hop-by-Hop extension headers for expanded protocol space (up to 64 KB). Exponent encoding and Sophia dictionaries are transport-agnostic.

---

## 2. Wotan Protocol (Message Bus)

**Specification:** `services/wotan/` (source code is the specification)
**Memory model RFC:** `docs/protocol/draft-bellis-unheaded-wotan-memory-00.md` and `-01.md`

### Purpose

Wotan is the Kingdom's nervous system -- a triple-role message bus providing ring buffer streaming, pub/sub topic delivery, and shared protocol RAM. All inter-service communication flows through Wotan; there are no direct service-to-service calls.

### Transport

- **Protocol:** gRPC over HTTP/2
- **Default address:** 10.10.10.10:9090
- **Authentication:** Service identity (mTLS planned for WS5)

### Message Format

Wotan messages are Protocol Buffer encoded:

```protobuf
message WotanMessage {
  string topic = 1;
  bytes payload = 2;
  string trace_id = 3;
  int64 timestamp_ms = 4;
  string source_service = 5;
}
```

**Constraints:**
- Payload must be < 1 MB
- All messages must include `trace_id`
- Timestamps are Unix milliseconds
- Use protobuf for large messages (not JSON)

### Topic Hierarchy

```
alerts.critical          -- All services subscribe
alerts.warn              -- Dashboard, monitoring
timeline.updates         -- Kanban-app, dashboard
system.outage.reports    -- Dashboard, unheaded-daemon
metrics.service.{name}   -- Dashboard-backend
doom.frames              -- Doom-bridge (framebuffer updates)
doom.stats               -- Doom-bridge (CPU state)
```

### Three Roles

1. **Ring Buffer:** High-throughput, ordered event stream. eBPF trace events flow from trace-collector through the ring buffer to dashboard-backend. Designed for millions of events per second with bounded memory.

2. **Event Bus:** Topic-based pub/sub. Services subscribe to topics and receive messages asynchronously. Delivery is at-least-once with optional deduplication by trace_id.

3. **Protocol RAM:** Shared key-value state accessible to all services. Sophia dictionaries, service configuration, and runtime state are stored here. Services read from protocol RAM to resolve exponent keys in real time.

### Client Integration

Every Go service follows the same Wotan integration pattern:

```go
func main() {
    // 1. Connect to Wotan
    client := wotanClient.New(os.Getenv("WOTAN_ADDR"))
    defer client.Close()

    // 2. Subscribe to topics
    client.Subscribe("alerts.critical", handleAlert)

    // 3. Publish state changes
    client.Publish("timeline.updates", payload)

    // 4. Graceful disconnect on shutdown
    <-ctx.Done()
    client.Close()
}
```

---

## 3. Sophia Protocol (Knowledge Graph)

**Specification:** `docs/protocol/draft-bellis-unheaded-sophia-dictionary-00.md` and `-01.md`
**Service:** `cmd/sophia/`

### Purpose

Sophia is the Kingdom's dictionary service. It maps exponent keys (the byte values in Monad's 20-byte register file) to human-readable meanings. Changing Sophia's dictionary atomically changes what every packet in the Kingdom means -- without modifying a single byte on the wire.

### Dictionary Structure

A Sophia dictionary is a mapping from `(position, value)` pairs to semantic labels:

```
Position 0: Source Identity
  0x00 -> "unknown"
  0x01 -> "gateway"
  0x02 -> "timeguru"
  0x03 -> "captain"
  ...

Position 1: Trace Type
  0x00 -> "untraced"
  0x01 -> "ingress"
  0x02 -> "egress"
  0x03 -> "internal"
  ...

Position 2: QoS Class
  0x00 -> "best-effort"
  0x01 -> "low-latency"
  0x02 -> "bulk"
  ...
```

### Atomic Dictionary Swap

Sophia supports atomic dictionary replacement. The active dictionary is versioned:

1. New dictionary loaded as "pending"
2. All services notified via Wotan topic
3. Atomic swap: pending becomes active, previous becomes archived
4. All subsequent Monad reads use new dictionary

This allows protocol semantics to change at runtime without packet modification, service restart, or coordination.

### API

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/dictionaries` | GET | List all dictionaries (active, pending, archived) |
| `/api/v1/dictionaries/{id}` | GET | Get specific dictionary |
| `/api/v1/dictionaries` | POST | Create new dictionary |
| `/api/v1/dictionaries/{id}/activate` | POST | Atomic swap to active |
| `/api/v1/resolve` | POST | Resolve exponent keys to labels |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |

---

## Protocol Interaction

The three protocols form a layered system:

```
Sophia (semantic layer)
  |
  | Dictionary lookups resolve Monad byte meanings
  v
Monad (wire layer)
  |
  | 20-byte register file carried in every packet
  | Read/written by eBPF programs at every hop
  v
Wotan (coordination layer)
  |
  | Distributes dictionary updates
  | Carries trace events from eBPF to services
  | Provides shared state (protocol RAM)
  v
Services (application layer)
```

A single packet's lifecycle:
1. **Arrives at Shield** -- Sophia resolves source identity, Monad register stamped
2. **Traverses Kingdom** -- eBPF reads/writes Monad at each hop, events published to Wotan
3. **Reaches destination** -- Service reads Monad metadata via eBPF, queries Sophia for meaning
4. **Exits at Shield** -- Monad stripped, clean packet returns to outside world

---

## Reference Documents

| Document | Path |
|----------|------|
| Protocol Foundation (canonical) | `docs/protocol/PROTOCOL_FOUNDATION.md` |
| Protocol Technical Summary | `docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md` |
| MBC ISA Reference | `docs/protocol/mbc-isa-reference.md` |
| Monad RFC Draft (latest) | `docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md` |
| Sophia Dictionary RFC | `docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md` |
| Wotan Memory Model RFC | `docs/protocol/draft-bellis-unheaded-wotan-memory-01.md` |
| Wire Format Patterns | `wiki/Wire-Format-Patterns.md` |
| Doom-over-IPv6 Architecture | `docs/protocol/doom-over-ipv6-architecture.md` |
| BPF Map Reference | `docs/doom/BPF-MAP-REFERENCE.md` |

---

*See also: [Architecture](architecture.md) | [Doom over IPv6](doom-over-ipv6.md) | [Security](security.md)*
