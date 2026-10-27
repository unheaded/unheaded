# Protocol Specifications

## Overview

The Unheaded Kingdom defines three core protocols that work together to provide packet-level observability, inter-service communication, and semantic metadata management. All three protocols are internal to the Kingdom -- they exist only within the network boundary and cannot leak to the outside world.

---

## 1. Monad Protocol (Packet Compute)

**Specification:** `docs/protocol/PROTOCOL_FOUNDATION.md`
**RFC drafts:** `docs/protocol/draft-bellis-unheaded-protocol-foundation-01.md` through `-06.md`
**ISA reference:** `docs/protocol/mbc-isa-reference.md`

### Purpose

Monad defines a 20-byte register file carried in IPv6 Hop-by-Hop extension headers. Every packet inside the Kingdom carries this register file, readable and writable by eBPF programs at every hop at kernel datapath speed (~320 ns per read).

### Wire Format

```
IPv6 Hop-by-Hop Extension Header:
  Next Header:  (protocol-dependent)
  Hdr Ext Len:  2 (24 bytes total)
  Option Type:  0x3E (act 00, chg 1, number 0x1E; IANA code point requested)
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
**Memory model RFC:** `docs/protocol/draft-bellis-unheaded-wotan-memory-00.md` through `-03.md`

### Purpose

Wotan is the Kingdom's nervous system -- a triple-role message bus providing ring buffer streaming, pub/sub topic delivery, and shared protocol RAM. All inter-service communication flows through Wotan; there are no direct service-to-service calls.

### Transport

- **gRPC** on `:18001` (`services/wotan/proto/topic.proto`: `TopicStream` with `StreamTopics`, `PublishTopic`, `TopicPing`; `chat.proto` for the legacy room stream; `replication.proto` for the cluster).
- **HTTP** on `:18000` ([[Wotan|Service-Wotan]] has the routes).
- **Authentication:** join + approval allowlist (`configs/wotan.yaml` `topics.auto_approve`); ML-DSA-65 signatures on `config.*` ([[Wotan Topic Signing|Wotan-Topic-Signing]]).

### Message Format

```protobuf
message TopicPublishRequest {
  string topic = 1;
  string sender_id = 2;
  bytes payload = 3;
  map<string, string> metadata = 4;
  bytes signature = 5;          // ML-DSA-65, config.* only
  bytes public_key = 6;
  string algorithm = 7;
  int64 signed_at_unix_ms = 8;  // covered by the signature; replay window
}

message TopicEvent {
  string message_id = 1;
  string topic = 2;
  string sender_id = 3;
  int64 seq = 4;                // global across topics
  google.protobuf.Timestamp created_at = 5;
  bytes payload = 6;
  TopicEventType type = 7;
  string node_id = 8;
  bytes signature = 9;
  bytes public_key = 10;
  string algorithm = 11;
}
```

Per-topic buffers and caps: [[Wotan|Service-Wotan]].

### Topic Hierarchy

```
alerts.critical          -- critical alert broadcast
alerts.warning           -- warnings
timeline.updates         -- timeguru -> kanban-app
system.health.reports    -- akira reports (ADR-029)
system.health.consensus  -- akira band changes
system.health.remediate  -- akira restart suggestions on CRITICAL/PANIC entry
system.discovery         -- service announcements (pkg/discovery)
logs.<service>.<level>   -- log aggregation (pkg/logagg)
metrics.<service>        -- metric events, read by dashboard-backend as metrics.*
config.*                 -- signed configuration
threats.cisa-kev.*       -- kev-poller
```

### Three Roles

1. **Ring Buffer:** High-throughput, ordered event stream. eBPF trace events flow from trace-collector through the ring buffer to dashboard-backend. Designed for millions of events per second with bounded memory.

2. **Event Bus:** Topic-based pub/sub. Services subscribe to topics and receive messages asynchronously. Delivery is at-least-once with optional deduplication by trace_id.

3. **Protocol RAM:** Shared key-value state accessible to all services. Sophia dictionaries, service configuration, and runtime state are stored here. Services read from protocol RAM to resolve exponent keys in real time.

### Client Integration

Go services use `pkg/wotan-client`:

```go
client, err := wotanClient.NewClientWithGRPC(httpAddr, grpcAddr) // or NewClient(httpAddr)
sub, err := client.Subscribe(ctx, "alerts.critical", "timeguru")
err = client.Publish(ctx, "timeline.updates", payload)
```

Most services get the addresses from `transport.ConfigFromEnv()` (`WOTAN_ADDR`, `WOTAN_GRPC_ADDR`; [[Transport Cascade|Transport-Cascade]]).

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

There is no dictionary-management HTTP API (no create, activate or resolve endpoint); the hot-swap above is specified, not served. What runs:

`sophia` service (`cmd/sophia`, `:19005`):

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/knowledge` | GET, POST | Read or add knowledge entries |
| `/api/v1/insights` | GET | Insights |
| `/api/v1/decide` | POST | Decision request |
| `/api/v1/stats` | GET | Counts |
| `/health`, `/ready`, `/metrics` | GET | Probes, Prometheus |

`protocol-api` (`cmd/protocol-api`, `:17100`), read-only dictionary listing:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/sophia/dictionaries` | GET | List dictionaries |
| `/api/v1/sophia/dictionaries/{id}` | GET | One dictionary |

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
| Monad RFC Draft (latest) | `docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md` |
| Sophia Dictionary RFC | `docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md` |
| Wotan Memory Model RFC | `docs/protocol/draft-bellis-unheaded-wotan-memory-03.md` |
| Wire Format Patterns | `wiki/Wire-Format-Patterns.md` |
| Doom-over-IPv6 Architecture | `docs/protocol/doom-over-ipv6-architecture.md` |
| BPF Map Reference | `docs/doom/BPF-MAP-REFERENCE.md` |

---

*See also: [[Architecture|Architecture]] | [[Doom over IPv6|Doom-over-IPv6]] | [[Security|Security]]*
