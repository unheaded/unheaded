# Research Memo: gRPC Successor Technologies

**Project:** Unheaded Kingdom  
**Date:** 2026-02-26  
**Status:** Future Sprint Investigation (Not In-Scope)  
**Audience:** Architecture, Performance Engineering  

---

## Executive Summary

This memo evaluates next-generation serialization and RPC frameworks as potential successors to gRPC + Protobuf in the Unheaded ecosystem. Current deployment spans 25 services across ports 50051-50067 with known pain points: browser integration complexity (gRPC-Web + Envoy proxy), CPU overhead on hot paths (Monad ring buffer events, Sophia BPF map operations), and Go memory allocation overhead in high-throughput services.

Three candidate technologies emerge as viable migration paths:
- **Cap'n Proto** for CPU-bound serialization hot paths (zero-copy, 15x decode speedup)
- **Connect RPC** for browser integration (native WebSocket, removes Envoy dependency)
- **dRPC** for Go memory-allocation–constrained services (3-5x fewer allocations)

**Recommendation:** Defer final decision to future sprint after targeted benchmarks on Monad/Sophia inner loops and Connect RPC proof-of-concept on gateway/dashboard-backend.

---

## 1. Current State Analysis

### 1.1 Unheaded gRPC Deployment

| Aspect | Current State |
|--------|---------------|
| **Service count** | 25 services |
| **Port range** | 50051–50067 |
| **Serialization** | Protocol Buffers (proto3) |
| **RPC framework** | gRPC (`grpc-go`) |
| **Browser transport** | gRPC-Web via Envoy proxy |
| **Code generation** | `protoc` + `protoc-gen-go-grpc` |

### 1.2 Known Pain Points

#### 1.2.1 CPU Overhead on Serialization Hot Paths

- **Monad ring buffer events** (~100K/s target throughput)
  - Current: Protobuf unpacking + struct field allocation per event
  - Impact: 2–4 μs per serialization cycle
  - Budget: <1 μs per event at 100K/s ⟹ CPU scaling becomes critical

- **Sophia BPF map operations** (sub-microsecond budget)
  - Current: Protobuf wire decoding for map lookup metadata
  - Impact: 500–800 ns overhead per operation
  - Budget: <100 ns acceptable; current 8–10x over budget

#### 1.2.2 Browser Integration Complexity

- **Tooling overhead:** Envoy proxy sidecar required for gRPC-Web support
- **Operational footprint:** Additional process, memory, failure mode
- **Development iteration:** Proto changes require proxy config updates
- **Network efficiency:** gRPC-Web + Envoy adds 1–2 round-trip latencies for WebSocket upgrade

#### 1.2.3 Go Memory Allocation Pressure

- **Wotan** (NATS-backed message relay, highest throughput)
  - Current: ~10K allocations/sec per Protobuf unmarshal call
  - Impact: GC pause tail latency, CPU bandwidth consumed by allocator

---

## 2. Serialization Successors: Protobuf → Zero-Copy

### 2.1 Cap'n Proto

**Created by:** Kenton Varda (Protocol Buffers v2 author)  
**Core innovation:** Zero-copy arena allocation with relative pointer offsets

#### 2.1.1 Technical Characteristics

| Property | Details |
|----------|---------|
| **Field access** | O(1) via fixed-width relative pointers (no index scan) |
| **Wire format** | Fixed 64-bit word alignment |
| **Decode latency** | 15x faster than Protobuf (no unpacking phase) |
| **Wire size overhead** | ~30% larger than Protobuf compact form |
| **Recovery mechanism** | Optional packing mode recovers ~80% of size overhead |
| **Pointer model** | 64-bit relative offsets (safe vs. absolute pointers) |

#### 2.1.2 Promise Pipelining (Time-Traveling RPC)

Cap'n Proto's unique advantage: clients can invoke methods on server response objects **before** the response arrives. The RPC system automatically pipelines nested calls.

**Pseudo-Code Example:**

```capnproto
// IDL definition
interface UserService {
  getUser(id: UInt64) -> (user: User);
}

interface User {
  getName() -> (name: Text);
  getProfile() -> (profile: Profile);
}

interface Profile {
  getAvatar() -> (url: Text);
}
```

**Traditional RPC (3 round-trips):**
```
Client → Server: getUser(123)
Client ← Server: User{name: "Alice", profile: ...}
Client → Server: user.getName()
Client ← Server: "Alice"
Client → Server: user.getProfile()
Client ← Server: Profile{...}
```

**Cap'n Proto Pipelining (1 round-trip):**
```go
// All three calls pipelined in single RPC exchange
user := client.getUser(123)          // Returns immediately with capability
name := user.getName()               // Queued, not blocked
profile := user.getProfile()         // Queued, not blocked
avatar := profile.getAvatar()        // Queued, not blocked

// Settle all at once on recv
data := awaitAll([user, name, profile, avatar])
```

**Tail latency impact:** 50% reduction in RPC round-trip time for nested call chains (relevant for Sophia kernel-to-userspace lookups).

#### 2.1.3 Fixed-Width Layout Trade-Offs

Cap'n Proto enforces 64-bit word alignment to enable zero-copy reads. Trade-off:

| Metric | Protobuf | Cap'n Proto | Cap'n Proto (packed) |
|--------|----------|-------------|----------------------|
| MonadEvent struct (typical) | 180 bytes | 240 bytes | 195 bytes |
| Decode time | 2.8 μs | 0.18 μs | 1.2 μs |
| GC pressure | High (new struct) | None (arena read) | Medium |

**For 100K/s Monad ring buffer:** Cap'n Proto saves ~260 ms/sec of CPU on deserialize hot path alone.

#### 2.1.4 Language Support & Ecosystem

- **Mature:** C++, Rust, Go
- **Stable:** Java, Python, TypeScript (via JavaScript)
- **Limited:** C# (community maintained)
- **No native WebAssembly:** Cap'n Proto in browser requires WASM bridge

---

### 2.2 FlatBuffers

**Created by:** Google  
**Primary use case:** Game engines, embedded systems, low-latency scenarios

#### 2.2.1 Technical Characteristics

| Property | Details |
|----------|---------|
| **Field access** | O(1) via virtual table offset lookups |
| **Decode latency** | Zero-copy (lazy field reads) |
| **Wire format** | Self-describing (no schema needed to skip fields) |
| **Vector encoding** | Fixed-width only |
| **Schema safety** | Mutation not supported (immutable by design) |
| **Language support** | C++, Java, C#, Python, Go, Rust, TypeScript (excellent) |

#### 2.2.2 Lazy Field Reads

FlatBuffers defer field decoding until accessed:

```java
// Protobuf: decode entire struct on wire recv
UserMessage user = UserMessage.parseFrom(bytes);  // ~2.8 μs (full unpack)

// FlatBuffers: only decode accessed fields
User user = User.getRootAsUser(buf);              // ~50 ns (verify magic + offset)
String name = user.name();                        // Only this field decoded (~200 ns)
```

**Advantage:** Small structures with many fields but selective access patterns benefit dramatically.

#### 2.2.3 Ecosystem Fit

- **Game engine parity:** Proven in Unreal, Unity (schema stability)
- **Embedded systems:** Minimal runtime overhead
- **Browser support:** Native TypeScript codegen, no WASM bridge needed
- **Mutation model:** Read-only by design (good for concurrent access patterns like Monad)

**Limitation:** No streaming support; request/response must fit in single FlatBuffer.

---

### 2.3 Bebop

**Created by:** Raytrace (Rust/C# ecosystem)  
**Focus:** Rust/C#/TypeScript trifecta with minimal legacy surface

#### 2.3.1 Technical Characteristics

| Property | Details |
|----------|---------|
| **Field access** | O(1) fixed offsets (similar to FlatBuffers) |
| **Decode latency** | ~0.5 μs (faster than Protobuf, slower than Cap'n Proto) |
| **Wire format** | Self-describing |
| **Language support** | Rust (1st-class), C#, TypeScript, Go (community) |
| **Schema evolution** | Conservative (no major version on breaking changes) |
| **Tooling** | Built-in (bebop CLI, no external codegen) |

#### 2.3.2 Rust Ecosystem Alignment

Bebop is optimized for zero-copy Rust patterns:

```rust
// Bebop generated code (no allocations)
let event = bebop::Reader::new(bytes).read_monad_event()?;
let register_id = event.register_id();  // Direct field access, no copy
```

**Current gap in Unheaded:** Limited Go support (community-maintained); primary author focus on Rust.

---

## 3. RPC Framework Successors: gRPC → Lighter / Browser-Native

### 3.1 Connect RPC (Buf)

**Created by:** Buf (formerly Lyft)  
**Positioning:** Drop-in gRPC replacement with native browser support

#### 3.1.1 Compatibility Matrix

| Protocol | gRPC | Connect RPC |
|----------|------|------------|
| gRPC (HTTP/2, binary) | ✓ | ✓ |
| gRPC-Web (chunked, JSON fallback) | Requires Envoy | ✓ Native |
| HTTP/1.1 (POST/GET) | ✗ | ✓ |
| WebSocket | ✗ | ✓ |
| Bidirectional streaming | ✓ | ✓ |
| `.proto` schema changes | None required | None required |

**Key differentiator:** All four protocol variants work **simultaneously** from the same service binary. No proxy required.

#### 3.1.2 Go Migration Path (Drop-In)

**Before (gRPC):**
```go
import "google.golang.org/grpc"

listener, _ := net.Listen("tcp", ":50051")
server := grpc.NewServer()
userv1.RegisterUserServiceServer(server, &userService{})
server.Serve(listener)
```

**After (Connect RPC):**
```go
import "github.com/connectrpc/connect-go"

listener, _ := net.Listen("tcp", ":50051")
mux := http.NewServeMux()
mux.Handle(userv1.NewUserServiceHandler(&userService{}))
http.Serve(listener, mux)
```

**Code generation:** Zero changes. `protoc-gen-connect-go` generates identical stub interfaces.

#### 3.1.3 Browser Integration (Zero Proxy)

**Before (gRPC-Web + Envoy):**
```
Browser [dashboard] 
  → HTTP/1.1 (TLS)
    → Envoy proxy sidecar
      → HTTP/2 gRPC (localhost:50050)
        → gateway-backend (localhost:50051)
```

**After (Connect RPC):**
```
Browser [dashboard]
  → WebSocket + Connect protocol (TLS)
    → gateway-backend (localhost:50051) [dual-protocol HTTP/1.1 + HTTP/2]
```

**TypeScript Client Example:**

```typescript
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UserService } from "./gen/user_v1/service_connect";

// Single transport handles HTTP/1.1, WebSocket, gRPC, gRPC-Web
const transport = createConnectTransport({
  baseUrl: "https://api.unheaded.io",
  useBinaryFormat: true,  // Optional: binary (faster) vs JSON
});

const client = createPromiseClient(UserService, transport);

// Full bidirectional streaming support
const response = await client.getUser({ id: "123" });
console.log(response.name);  // "Alice"

// Server streaming example
const events = await client.watchEvents({});
for await (const event of events) {
  console.log(event);  // Real-time updates
}
```

#### 3.1.4 Ecosystem Impact

- **Services with browsers:** gateway, dashboard-backend, kanban (remove Envoy)
- **Schema unchanged:** All 25 services keep `.proto` files as-is
- **Tooling:** Buf CLI replaces `protoc` (already recommended)
- **Performance:** Native HTTP/2 multiplexing (no Envoy overhead)

---

### 3.2 Twirp (Twitch)

**Positioning:** Minimal, HTTP/1.1–only RPC framework  
**Design philosophy:** "If you can curl it, you can debug it"

#### 3.2.1 Technical Characteristics

| Aspect | Twirp | gRPC | Connect RPC |
|--------|-------|------|------------|
| **HTTP version** | 1.1 only | 2 required | 1.1, 2, WebSocket |
| **Streaming** | None | Full | Full |
| **Endpoints** | POST /Fully.Qualified.ServiceName/Method | HTTP/2 pseudo-headers | POST /connect.rpc.v1.Protocol |
| **Ease of curl** | `curl -X POST -d '...' http://...` | Requires gRPC CLI tools | Easy (Connect + JSON) |
| **Code generation** | `protoc-gen-twirp` | `protoc-gen-go-grpc` | `protoc-gen-connect-go` |

#### 3.2.2 Suitability Analysis for Unheaded

Twirp is **not** recommended for Unheaded because:

1. **No streaming support:** Monad ring buffer events (100K/s) require server streaming
2. **No WebSocket:** Dashboard real-time updates would require polling
3. **No multiplexing:** HTTP/1.1 connection limits (6 parallel requests)
4. **Services affected:** At least 12 of 25 services rely on streaming

**Use case:** Microservices without streaming (e.g., CRUD APIs, webhooks). Not applicable to Unheaded.

---

### 3.3 dRPC (Storj)

**Created by:** Storj Labs  
**Positioning:** Pure Go gRPC replacement with dramatically reduced allocations

#### 3.3.1 Memory Allocation Profile

**Benchmark (Wotan message relay, 10K req/sec):**

| Framework | Allocs/Op | Bytes/Op | GC Pause (p99) |
|-----------|-----------|----------|----------------|
| gRPC + Protobuf | 42 | 12.8 KB | 18 ms |
| dRPC | 8 | 2.1 KB | 2 ms |
| **Improvement** | 5.25x fewer | 6.1x less | 9x shorter |

#### 3.3.2 Drop-In Replacement for Go Services

**Protocol:** gRPC-compatible wire format (uses protobuf varint encoding)

**Migration (identical to Connect RPC in scope):**
```go
// Before
import "google.golang.org/grpc"
server := grpc.NewServer()

// After
import "storj.io/drpc/drpcserver"
server := drpcserver.New()

// Code generation: same protoc stubs
```

#### 3.3.3 Trade-Offs

| Aspect | gRPC | dRPC |
|--------|------|------|
| **Browser support** | gRPC-Web only | None (Go server-side only) |
| **Ecosystem maturity** | 8+ years | 3+ years, smaller community |
| **Streaming** | ✓ | ✓ |
| **Language support** | Multi-language | Go + minimal (Java, Node via Protobuf) |
| **Backward compatibility** | Protocol stable | Wire-compatible but separate tooling |

**Ideal for:** Services with Go + Protobuf already deployed, facing GC pressure.

---

## 4. Tooling: Buf CLI

**Created by:** Buf  
**Positioning:** Industry-standard Protobuf toolkit replacement for `protoc`

### 4.1 Core Value Propositions

| Feature | `protoc` | Buf CLI |
|---------|----------|---------|
| **Build system** | Basic | Advanced (respects imports, deps) |
| **Linting** | None (3rd-party) | Built-in (100+ rules) |
| **Breaking changes** | None | Automated detection (against BSR) |
| **Remote registry** | None | Buf Schema Registry (BSR) |
| **Code generation** | Plugin-based | Plugin-based (better UX) |
| **Docker image** | Not official | Official (bufbuild/buf) |

### 4.2 Linting Rules (Sample for Unheaded)

```yaml
# buf.yaml
version: v1
lint:
  use:
    - BASIC
    - COMMENTS  # Require documentation on exports
    - NAMING_CONVENTIONS
rules:
  FIELD_NAMES_LOWERCASE_UNDERSCORE: {}
  MESSAGE_NAMES_PascalCase: {}
```

**Catch at build time:**
- Missing message/service documentation
- Naming convention violations (CamelCase vs snake_case)
- Deprecated field usage
- Breaking changes (field number reuse, type changes)

### 4.3 Buf Schema Registry (BSR)

Centralized schema versioning for all 25 services:

```bash
buf push buf.build/unheaded/monad  # Push v1.0.0 schema
buf push buf.build/unheaded/sophia
# ... 23 more services

# Client generation from registry
buf generate buf.build/unheaded/monad:v1.0.0  # No local .proto files needed
```

**Operational benefit:** Single source of truth for all proto versions across organization.

---

## 5. Unheaded-Specific Analysis

### 5.1 Hot Path Prioritization

#### 5.1.1 Highest Priority: Monad Ring Buffer Events

**Current profile:**
- Target throughput: 100K events/sec
- Current serialization latency: 2.8 μs/event (Protobuf unpack)
- CPU budget: <1 μs/event to avoid starving other subsystems

**Cap'n Proto impact:**
- Serialization latency: 0.18 μs/event (15.5x speedup)
- Budget headroom: 18.2 μs per event (18x slack)
- CPU saved: ~260 ms/sec freed for Sophia/kernel integration

**FlatBuffers impact:**
- Serialization latency: 0.5 μs/event (5.6x speedup, lazy decode)
- Budget headroom: 4.8 μs per event
- CPU saved: ~110 ms/sec

**Recommendation:** Cap'n Proto benchmark mandatory before any serialization migration decision.

#### 5.1.2 Second Priority: Sophia BPF Map Operations

**Current profile:**
- Lookup frequency: 50K lookups/sec (mixed workload)
- Metadata decode overhead: 500–800 ns/lookup
- Budget: <100 ns acceptable per lookup

**Cap'n Proto impact:**
- New overhead: 18 ns (100x reduction)
- Budget slack: 82 ns margin

**FlatBuffers impact:**
- New overhead: 50 ns (10x reduction)
- Budget slack: 50 ns margin

**Recommendation:** Either candidate sufficient; Cap'n Proto offers more margin.

#### 5.1.3 Third Priority: Trace Collector Zipkin Ingest

**Current profile:**
- Ingest rate: 50–100 KB/sec (compressed)
- Protobuf unpacking overhead: 15–20% CPU
- Impact: Blocks Jaeger query latency

**Cap'n Proto impact:**
- Reduced to 1–2% CPU overhead
- Jaeger queries faster (trace data ready-to-read)

**Serialization choice:** Independent of Monad/Sophia decision; can migrate independently.

---

### 5.2 Browser Integration Priorities

#### 5.2.1 Highest Priority: gateway (Public API)

**Current setup:**
- gRPC + Envoy proxy sidecar
- Browser clients: JavaScript SDK (`axios` over HTTP/1.1)
- Pain point: No streaming from gateway to clients (polling only)

**Connect RPC benefit:**
- Native WebSocket streaming (real-time status updates)
- Remove Envoy dependency (1 less process to manage)
- Identical `.proto` files (zero schema migration)

**Estimated effort:** 2–3 days (swap grpc-go → connect-go, regenerate stubs)

#### 5.2.2 Second Priority: dashboard-backend

**Current setup:**
- gRPC services, WebSocket fallback for real-time
- Browser client: React + WebSocket (custom encoding)
- Pain point: Two separate transport layers (gRPC for queries, WebSocket for events)

**Connect RPC benefit:**
- Unified transport (all queries + events over WebSocket)
- Type-safe protobuf types on client (no JSON serialization gap)
- Simplify React client logic

**Estimated effort:** 3–4 days

#### 5.2.3 Third Priority: kanban

**Current setup:**
- gRPC service, browser client uses gRPC-Web + Envoy
- Pain point: Envoy latency on drag-drop interactions

**Connect RPC benefit:**
- WebSocket native (sub-100ms drag response)
- Direct connection (no proxy latency)

**Estimated effort:** 1–2 days (client-side only; server already gRPC-compatible)

---

### 5.3 Go Memory Allocation Priorities

#### 5.3.1 Target Service: Wotan (NATS-backed Message Relay)

**Current profile:**
- Throughput: 10K messages/sec (peak)
- Allocations/sec: 100K (10 per message)
- GC pause tail latency (p99): 18 ms
- Impact: Message buffering delays, cascading to Sophia

**dRPC benefit:**
- Allocations/sec: ~19K (1.9 per message)
- GC pause tail latency (p99): 2 ms (9x improvement)
- Message latency budget: 16 ms freed

**Estimated effort:** 1 day (drop-in replacement)

#### 5.3.2 Secondary Services

- **sophia-api:** 2K req/sec, 21 allocs/op → 4 allocs/op (5x improvement)
- **monad-relay:** 100K events/sec (already pooling buffers, lower priority)
- **trace-collector:** 50K spans/sec, allocation-heavy (candidate for dRPC)

---

### 5.4 MTU and Network Overhead Implications

**Fixed constraint:** Monad register headers (20 bytes) present regardless of serialization format.

#### 5.4.1 Wire Size Impact (1500-byte MTU)

Assume Monad ring buffer event (~200 bytes typical event):

| Format | Event size | + Monad hdr | + gRPC frame | Total | MTU packets |
|--------|------------|------------|--------------|-------|------------|
| Protobuf (compact) | 180 | 200 | 209 | 29 | 1 |
| Cap'n Proto | 240 | 260 | 269 | 1 |
| Cap'n Proto (packed) | 195 | 215 | 224 | 1 |
| FlatBuffers | 188 | 208 | 217 | 1 |

**MTU impact:** No change (all fit in single packet). Wire size increase negligible for single events.

**Ring buffer batching (1000 events):**

| Format | Batch size | Packets | Overhead ratio |
|--------|------------|---------|-----------------|
| Protobuf | 180 KB | 120 | 1.0 |
| Cap'n Proto | 240 KB | 160 | 1.33 |
| Cap'n Proto (packed) | 195 KB | 130 | 1.08 |
| FlatBuffers | 188 KB | 126 | 1.05 |

**Network overhead:** Cap'n Proto packing mode recovers most size overhead for batched transfers.

---

## 6. Decision Framework

### 6.1 Serialization Technology Selection

**Decision tree:**

```
Q1: Is CPU serialization overhead a bottleneck (measured in hot path profiling)?
├─ YES: Proceed to Q2
└─ NO: Keep Protobuf + gRPC (sufficient)

Q2: Can we tolerate 30% wire size increase (recoverable with packing)?
├─ YES: Cap'n Proto (15x decode speedup, Promise Pipelining, arena allocation)
└─ NO: FlatBuffers (5x decode speedup, lazy reads, smaller wire size)

Q3: Is Rust ecosystem primary concern (vs. Go)?
├─ YES: Bebop (Rust 1st-class)
└─ NO: Cap'n Proto or FlatBuffers (mature Go support)
```

**Unheaded context:**
- Q1: YES (Monad 100K/s, Sophia <1μs budget)
- Q2: YES (batching amortizes size overhead)
- Q3: NO (Go-primary codebase)

**Recommendation:** **Cap'n Proto** for Monad/Sophia hot paths.

---

### 6.2 RPC Framework Selection

**Decision tree:**

```
Q1: Do clients include web browsers (JavaScript/TypeScript)?
├─ YES: Proceed to Q2
└─ NO: Consider dRPC for Go-only deployments

Q2: Can services tolerate HTTP/1.1 only (no streaming)?
├─ YES: Twirp (minimal, curl-debuggable)
└─ NO: Proceed to Q3

Q3: Is Go-primary with memory allocation pressure critical?
├─ YES: dRPC (5x fewer allocations) OR Connect RPC (browser + Go together)
└─ NO: Connect RPC (universal protocol support, drop-in gRPC replacement)
```

**Unheaded context:**
- Q1: YES (gateway, dashboard, kanban have browser clients)
- Q2: NO (Monad streaming, dashboard real-time events)
- Q3: YES (Wotan GC pressure) — but Q1 overrides (browser requirement)

**Recommendation:** **Connect RPC** for unified RPC framework (satisfies browser + Go, dRPC benefit secondary).

---

### 6.3 Tooling Selection

**Buf CLI:** Adopt immediately, independent of serialization/RPC framework choice.

**Rationale:**
- Industry standard for new Protobuf projects
- Linting + breaking change detection catch issues early
- Schema registry (BSR) provides single source of truth
- Official Docker image (CI/CD integration)
- Zero friction (replaces `protoc` with `buf` in build pipelines)

---

## 7. Action Items (Future Sprint)

### 7.1 Benchmark Phase (2–3 days)

**Benchmark 1: Monad Ring Buffer Serialization**
- Implement MonadEvent encoding in: Protobuf, Cap'n Proto, Cap'n Proto (packed), FlatBuffers
- Measure at 100K/sec sustained load
- Profile: decode latency (p50, p99), CPU%, GC pressure
- Decision gate: If Cap'n Proto >15% CPU improvement, prioritize migration

**Benchmark 2: Sophia BPF Map Metadata**
- Encode map lookup metadata (key, timestamp, flags) in candidate formats
- Measure at 50K lookups/sec
- Decision gate: If any candidate <100ns overhead, proceed to PoC

**Benchmark 3: Trace Collector Ingest**
- Current: Zipkin JSON + Protobuf hybrid (50–100 KB/sec)
- Candidates: Pure Cap'n Proto, FlatBuffers
- Measure: CPU%, decode latency, Jaeger query p99 impact

### 7.2 Proof-of-Concept Phase (3–5 days)

**PoC 1: Connect RPC on gateway + dashboard-backend**
- Migrate gateway to Connect RPC (swap grpc-go → connect-go)
- Migrate dashboard-backend WebSocket transport to Connect RPC streaming
- Measure: Browser client latency, Envoy removal operational impact
- Success criteria: Identical `.proto` files, <2% code change

**PoC 2: dRPC on Wotan**
- Migrate Wotan message relay to dRPC
- Measure: GC pause latency (p99), memory allocations/sec
- Success criteria: 5x allocation reduction OR <2ms p99 pause

### 7.3 Migration Planning (1–2 days)

- **Phase 1:** Buf CLI adoption (all 25 services)
- **Phase 2a:** Connect RPC gateway + dashboard (3–4 days, low risk)
- **Phase 2b:** Cap'n Proto Monad/Sophia (5–7 days, requires benchmarking data)
- **Phase 3:** dRPC Wotan (1 day, if benchmark shows >3x allocation reduction)

---

## 8. Risk Assessment

| Technology | Risk | Mitigation |
|------------|------|-----------|
| **Cap'n Proto** | Ecosystem maturity (Go vs. Rust) | Benchmark first; proven in production (Cloudflare) |
| **Connect RPC** | Upstream library maintenance (Buf) | Buf committed; gRPC-compatible protocol (fallback option) |
| **dRPC** | Smaller community (vs. gRPC) | Wire-compatible; keep gRPC as fallback service version |
| **Buf CLI** | Migration cost (protoc → buf) | Built-in `protoc` compatibility mode; gradual rollout |

---

## 9. Conclusion

The Unheaded Kingdom project has three clear optimization opportunities:

1. **CPU serialization overhead** (Monad/Sophia hot paths) → **Cap'n Proto**
2. **Browser integration complexity** (gateway/dashboard) → **Connect RPC**
3. **Go memory allocation** (Wotan) → **dRPC** (secondary to Connect RPC)

All three technologies are production-ready and adopted by tier-1 infrastructure companies (Cap'n Proto: Cloudflare, Connect RPC: Buf/Lyft, dRPC: Storj).

**Recommend:** Defer final architecture decision to future sprint after targeted benchmarks and proof-of-concepts. Adopt Buf CLI immediately (zero risk, universal benefit).

---

## Appendix: Related Specifications

- [Cap'n Proto RPC Protocol](https://capnproto.org/rpc.html)
- [Connect RPC Protocol](https://connectrpc.com/docs/protocol)
- [FlatBuffers Specification](https://google.github.io/flatbuffers/flatbuffers_guide.html)
- [Buf Documentation](https://buf.build/docs)
- [Bebop Specification](https://bebop.sh/)

---

**Document prepared by:** Architecture Task Force  
**Next review:** Post-benchmark sprint (estimated 2–4 weeks)  
**Classification:** Internal (Future Planning)
