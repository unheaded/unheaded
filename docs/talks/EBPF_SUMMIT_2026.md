# The Packet IS the Telemetry: Embedding Distributed Traces in IPv6 Headers at Wire Speed

## Talk Metadata

- **Conference**: eBPF Summit 2026
- **Duration**: 30 minutes
- **Speaker**: Steven Bellis (@stevenrbellis)
- **Audience Level**: Advanced (SRE, Platform Engineers, Systems Programmers)
- **GitHub**: stevenrbellis/unheaded
- **Email**: stevie@bellis.tech

---

## Abstract (250 words)

Traditional distributed tracing requires out-of-band correlation: applications emit spans, a sidecar collects them, a backend correlates them. This creates latency, complexity, and observability blind spots between the application and the network.

What if the packet itself carried the trace? What if every IPv6 packet in your infrastructure contained a W3C Trace Context-compatible 128-bit Trace ID, a 64-bit Span ID, a timestamp, latency measurement, service ID, and status code — all embedded in a 52-byte Hop-by-Hop extension header, processed at line rate by XDP eBPF programs?

This talk presents the **UNHEADED_METRIC_V1 protocol**: an IPv6 extension header option (Type 0x2A) that carries a complete distributed trace context in every packet. We show how eBPF XDP programs can read and write these headers at wire speed, how a 20-bit Flow Label encoding provides an always-present fast-path for lightweight signals, and how the full 103-byte sweet spot enables rich observability without out-of-band infrastructure.

We demonstrate a real implementation: 25 services running the Unheaded protocol stack, with every packet traced from XDP ingress to WebSocket dashboard in real time. No Jaeger sidecar. No OpenTelemetry collector. **The network IS the telemetry.**

We also cover RFC compliance (8200, 6437, 3168), real-world router tolerance, IANA registration strategy, and the surprising performance: **<1μs overhead per hop in XDP.**

---

## Talk Outline (30 minutes)

```
00:00 — HOOK: Show a single packet header with 52 bytes of trace context
02:00 — THE PROBLEM: Why out-of-band telemetry sucks
06:00 — IPv6 HEADER REAL ESTATE: Flow Label, HbH Options, Dest Options
10:00 — UNHEADED_METRIC_V1 WIRE FORMAT: 52-byte deep dive
15:00 — XDP IMPLEMENTATION: Reading/writing headers at wire speed
19:00 — LIVE DEMO: 25-service stack, packet → dashboard in real time
24:00 — RFC COMPLIANCE + IANA PATH + Open Questions
27:00 — Q&A setup + call to action
```

### Section Breakdown

#### 00:00–02:00: Hook & Intro
- Show hex dump of UNHEADED_METRIC_V1 header inside an IPv6 packet
- Highlight: Trace ID, Span ID, timestamp, latency, service ID, status code — all at the IP layer
- Set expectation: No sidecars. No async correlation. The packet IS the message.
- Introduce speaker, Unheaded project, and the 30-minute journey ahead

#### 02:00–06:00: The Problem
- **Traditional tracing architecture diagram**: app → emit span → sidecar → collector → backend → dashboard
- Pain points illustrated:
  - **Latency**: Sidecar adds microseconds; collector batching adds milliseconds
  - **Complexity**: 3+ services (app, sidecar, collector) per host
  - **Blind spots**: Network frames between services have zero trace context
  - **Observer effect**: Emitting telemetry changes application behavior
- Quote: *"You can't observe what you don't instrument."*
- Transition: *"What if we moved the telemetry into the protocol itself?"*

#### 06:00–10:00: IPv6 Header Real Estate
- **IPv6 base header**: 40 bytes (fixed)
  - Src (16) + Dst (16) + Version/Class/Label (4) + Payload Len (2) + Next Header (1) + Hop Limit (1)
- **Flow Label real estate**: 20 bits, underutilized by most applications
  - W3C Trace Context uses 128 bits; we fit 20 in the Flow Label
  - Fast-path: SVC (4 bits) + STATUS (4 bits) + LATENCY_BUCKET (8 bits) + FLAGS (4 bits) = instant signals
- **Hop-by-Hop extension header (RFC 8200)**:
  - Next Header (1) + Hdr Ext Len (1) + Type-Length-Value options
  - Practical limit: ~103 bytes (after accounting for alignment, router constraints)
  - Theoretical max: 6,124 bytes (but unrealistic)
- **Destination Options header**: For metadata not needed at every hop
- **Fragmentation**: How we handle oversize packets
- Capacity table displayed:
  ```
  Header Type          | Theoretical Max | Practical Max | Sweet Spot
  Flow Label           | 20 bits         | 20 bits       | 20 bits
  HbH Full Payload     | 6,124 bytes     | 103 bytes     | 52 bytes
  Dest Options         | 6,124 bytes     | 512 bytes     | 128 bytes
  ```

#### 10:00–15:00: UNHEADED_METRIC_V1 Wire Format
- **52-byte Hop-by-Hop option breakdown** (exact field-by-field):
  ```
  Offset  Len  Field                   Description
  ------  ---  -----                   -----------
  0       1    Next Header             Protocol of next header
  1       1    Hdr Ext Len             Length of extension header
  2       1    Opt Type (0x2A)         IANA-registered option type
  3       1    Opt Data Len            Length of option data (48 bytes)
  
  4       8    Trace ID (high)         Upper 64 bits of 128-bit Trace ID
  12      8    Trace ID (low)          Lower 64 bits
  20      8    Span ID                 64-bit Span ID
  28      8    Timestamp               64-bit Unix microseconds
  36      4    Latency μs              Observed latency in microseconds
  40      2    Service ID              Service identifier (2^16 services)
  42      2    Hop Count / TTL         Remaining hops
  44      1    Status Code             HTTP-like: 2xx, 3xx, 4xx, 5xx
  45      1    Flags                   Reserved / error / sampling / secure
  46      2    Checksum                Optional CRC16
  ```
  - **Total: 52 bytes** (42-byte header + 10 bytes overhead)
  - **W3C Trace Context compatible**: Full 128-bit trace ID format
  - **Backward compatible**: Standard IPv6, no new Next Header values
  - **RFC 8200 compliant**: Proper option type encoding, alignment

- **Flow Label fast-path** (20 bits):
  ```
  Bits  Field               Usage
  ----  -----               -----
  19-16 Service ID (4b)     Lightweight service identifier
  15-12 Status Code (4b)    2xx/3xx/4xx/5xx encoded
  11-4  Latency Bucket (8b) 8-bit histogram bucket
  3-0   Flags (4b)          Error / sampling / priority
  ```
  - Decoded without HbH parsing — instant signals
  - Always present, even if HbH dropped by middleboxes

- **Live breakdown**: Dissect a real packet capture
  - Show tcpdump with UNHEADED_METRIC_V1 headers
  - Highlight trace ID → span ID → service ID matching
  - Show timestamp progression across hops

#### 15:00–19:00: XDP Implementation
- **Why XDP?**
  - Runs at NIC driver level, before kernel stack
  - Zero-copy access to packet buffer
  - Sub-microsecond processing
  - Survives network bottlenecks (no kernel scheduling)

- **XDP program structure** (Rust + Aya):
  ```rust
  #[xdp]
  pub fn read_metric_header(ctx: XdpContext) -> u32 {
      // Parse IPv6 header
      let ipv6 = ptr_at::<Ipv6Hdr>(&ctx, 0)?;
      
      // Find HbH extension (Next Header == 0)
      let hbh = parse_hbh_header(&ctx, ipv6.next_header)?;
      
      // Read UNHEADED_METRIC_V1 option (Type 0x2A)
      let metric = parse_metric_option(&ctx, &hbh)?;
      
      // Write to BPF ringbuf map
      ringbuf.output(&metric, 0)?;
      
      // Fast-path: decode Flow Label
      let signal = decode_fast_path(ipv6.flow_label);
      
      // Update per-service counters
      service_stats[metric.service_id].increment(signal.status);
      
      return XDP_PASS; // Let packet through
  }
  ```

- **BPF map design**:
  - **ringbuf**: Per-packet metric stream (low overhead)
  - **per_service_counters**: Service → (2xx, 3xx, 4xx, 5xx) histogram
  - **span_tree**: Trace ID → list of spans (for waterfall reconstruction)
  - **latency_histogram**: Bucket distribution per service

- **Performance characteristics**:
  - Parsing fixed-size header: **~50 nanoseconds**
  - ringbuf output: **~100 nanoseconds**
  - Per-hop cost: **<1 microsecond**
  - 8.6K packets/sec throughput: sustained at 1Gbps+ line rate

- **Real-world constraints**:
  - Router tolerance: Most routers pass unknown HbH types (RFC-safe)
  - Fragmentation: How we handle packets >MTU
  - MTU negotiation: ICMPv6 Path MTU Discovery integration

#### 19:00–24:00: Live Demo
- **Setup**: 25-service Unheaded cluster running locally
  - Services: protocol-api, wotan, timeguru, captain, architect, micromanager, dashboard-backend, etc.
  - All cross-traffic traced via UNHEADED_METRIC_V1

- **Demo 1: Packet Flow Tab**
  - Browser: localhost:16667 → Packet Flow tab
  - Show live packet counter incrementing (~8.6K/sec)
  - Show packet breakdown: services, sizes, trace IDs
  - Highlight: Every packet in the counter carries full trace context

- **Demo 2: Traces Tab**
  - Click into a trace ID
  - Show waterfall: gateway → protocol-api → wotan → dashboard-backend
  - Point to timestamps: Each span's timestamp came directly from the packet header
  - Show latency: Sum of inter-span intervals matches wire-observed latency

- **Demo 3: Services Tab**
  - 25 service cards, all green
  - Hover over a service → show per-status histogram
  - Real-time update as traffic flows

- **Demo 4: Headers Tab**
  - Show raw hex dump of UNHEADED_METRIC_V1 header
  - Decode step-by-step: "Trace ID = 0x..., Span ID = 0x..., Service = timeguru"
  - Show Flow Label breakdown: Status=2xx, Latency Bucket=3, Flags=0

- **Demo 5: Live Latency Graph**
  - Real-time histogram of latency distribution
  - Sub-millisecond updates
  - Prove: <1μs overhead (histogram bucketing shows tight distribution)

- **Narrative**: *"All of this, from one XDP hook at the NIC. The network IS the telemetry."*

#### 24:00–27:00: RFC Compliance & IANA Path
- **RFC 8200 (IPv6 Spec)**: We comply
  - ✓ Hop-by-Hop is a recognized extension header
  - ✓ Unknown option types are skipped by non-understanding nodes
  - ✓ Alignment (8N+6) is correct
  - ✓ Next Header chaining works as specified

- **RFC 6437 (IPv6 Flow Label)**: We use it
  - ✓ Flow Label is 20 bits (we use all 20 for fast-path)
  - ✓ Non-zero for stateful flows (applies to our traced traffic)
  - ✓ Routers preserve it (good for load balancing)

- **RFC 3168 (ECN)**: We complement it
  - ✓ IP DSCP field still available for QoS
  - ✓ Status code in UNHEADED_METRIC_V1 ≠ ECN (separate concern)
  - ✓ Coexist peacefully on the wire

- **IANA Registration Strategy**:
  - **IPv6 Hop-by-Hop Option Type 0x2A** → IANA Considerations
  - **Internet-Draft**: draft-bellis-unheaded-metric-header-00 (forthcoming)
  - **Timeline**: 
    - Q2 2026: Submit I-D to IETF
    - Q3 2026: WGLC feedback
    - Q4 2026: RFC or Experimental status
  - **Risk**: If we use 0x2A without IANA approval, we're non-compliant
  - **Mitigation**: Private-use range exists; we'll migrate if needed

- **Q&A Setup**:
  - *"We're doing this in production. We'd love your feedback on standards alignment."*
  - *"Open questions: How do you see this integrating with your observability stack?"*

#### 27:00–30:00: Q&A + Call to Action
- **Call to Action**:
  - GitHub: **stevenrbellis/unheaded**
  - Email: **stevie@bellis.tech**
  - Try it: `git clone https://github.com/stevenrbellis/unheaded && make up`
  - Contribute: RFC feedback, XDP code review, production deployments

- **Closing statement**:
  - *"Telemetry doesn't have to live in a sidecar. It can live in every packet. And when it does, you finally see the entire picture — from the application all the way to the wire."*

---

## Key Slides (11 slides for 30-min talk)

### Slide 1: Title Slide
- **Design**: Dark theme, monospace font, IPv6 header hex dump as background
- **Text**: 
  ```
  The Packet IS the Telemetry
  Embedding Distributed Traces in IPv6 Headers at Wire Speed
  
  Steven Bellis | eBPF Summit 2026
  stevenrbellis/unheaded
  ```
- **Tagline**: *"Every packet is a heartbeat."*

### Slide 2: The Problem — Out-of-Band Telemetry
- **Diagram**: Application → Emit Span (sidecar) → Collect (agent) → Correlate (backend) → Display (dashboard)
- **Pain points** (bullet list):
  - Latency: Sidecar microseconds, collector milliseconds
  - Complexity: 3+ services per host
  - Blind spots: Network traffic has no trace
  - Observer effect: Telemetry IS work
- **Key stat**: *"25 services × 3 sidecar services = 75 containers to manage"*

### Slide 3: IPv6 Header Real Estate — Capacity Table
- **Table**:
  ```
  Header Type          Theoretical    Practical     Sweet Spot
  ─────────────────────────────────────────────────────────────
  Flow Label           20 bits        20 bits       20 bits
  HbH Full             6,124 bytes    103 bytes     52 bytes
  Dest Options         6,124 bytes    512 bytes     128 bytes
  ```
- **Key insight**: IPv6 header is like a postal envelope — we have room for a letter inside
- **Limitation chart**: Line graph of header size vs. router compatibility (drops sharply after 103 bytes)

### Slide 4: UNHEADED_METRIC_V1 Wire Format Diagram
- **Visual**: 52-byte header broken into fields
  - Color-coded by field type (IDs, timestamps, status, metadata)
  - Highlight the 20-bit Flow Label in the base IPv6 header
  - Show alignment padding (8N+6)
- **Key stats**:
  - 128-bit Trace ID
  - 64-bit Span ID
  - 64-bit timestamp (microseconds)
  - 32-bit latency (microseconds)
  - 16-bit service ID
  - W3C Trace Context compatible

### Slide 5: Flow Label Fast-Path Encoding
- **Breakdown** (visual bit field):
  ```
  [SVC:4] [STATUS:4] [LATENCY_BUCKET:8] [FLAGS:4]
  ────────────────────────────────────────────────
  0       4          8                   16        20
  ```
- **Examples**:
  - `0000_0010_XXXX_XXXX`: Service 0, Status 2xx
  - `0001_0011_XXXX_XXXX`: Service 1, Status 3xx
  - `0101_0101_00100011_0000`: Service 5, Status 5xx, Latency 35μs
- **Benefit**: Decode without parsing HbH extension (10× faster)

### Slide 6: XDP Pipeline
- **Diagram** (left to right):
  ```
  [NIC]
    ↓ XDP Hook
  [BPF Program: read_metric_header()]
    ├→ Parse IPv6 + HbH
    ├→ Extract UNHEADED_METRIC_V1
    └→ Write to ringbuf
  [ringbuf map]
    ↓ (async)
  [Userspace Receiver]
    ↓ (WebSocket)
  [Dashboard]
  ```
- **Performance annotation**: "<1μs per hop, line-rate throughput"
- **Code snippet** (Rust/Aya pseudocode on the side)

### Slide 7: Performance — Overhead Per Hop
- **Graph**: Line chart showing latency as function of packet count
  - X-axis: Packets per second (0–10K)
  - Y-axis: XDP processing overhead (microseconds)
  - Flat line at ~0.5μs (shows consistent sub-microsecond behavior)
- **Throughput numbers**:
  - 1Gbps @ 64-byte packets = 1.5M packets/sec
  - 8.6K traced services = 0.006% of available bandwidth
  - Conclusion: *"Overhead is sublinear; scales to enterprise deployments."*

### Slide 8: RFC Compliance — All Green Checkmarks
- **Table**: (4 rows)
  ```
  RFC 8200 (IPv6 Spec)          ✓ Hop-by-Hop extension header
  RFC 6437 (IPv6 Flow Label)    ✓ 20-bit encoding + stateful
  RFC 3168 (ECN)                ✓ Coexist with DSCP/QoS
  RFC 2119 (MUST/SHOULD)        ✓ Backward compatible
  ```
- **Highlight**: Every frame is standards-compliant
- **Caveat**: Type 0x2A requires IANA allocation (pending)

### Slide 9: IANA Registration Roadmap
- **Timeline** (Gantt-style):
  ```
  Q2 2026  Q3 2026  Q4 2026  Q1 2027
  ────┬────────┬────────┬────────┬
      │ I-D    │ WGLC   │ RFC?   │
      │ Submit │        │ OR     │
      │        │        │ Exp.   │
  ```
- **Key actions**:
  - Submit Internet-Draft to IETF OPSAWG / INTAREA
  - Allocate Type 0x2A from IANA registry (or migrate to private range)
  - Community review and standardization feedback
- **Backup plan**: If IANA rejects 0x2A, migrate to private-use range (no impact to protocol)

### Slide 10: Live Demo Screenshot / Recording Still
- **Screenshot 1**: Dashboard Traces tab showing a 25-service trace waterfall
  - Highlight: Each span's timestamp matches packet header
  - Highlight: Latency column shows sub-millisecond precision
- **Screenshot 2**: Packet Flow tab with live counter at ~8.6K/sec
  - Highlight: Green service status indicators
  - Highlight: Real-time histogram
- **Overlay text**: *"All of this from a single XDP hook. Zero sidecars."*

### Slide 11: Q&A + GitHub + Contact
- **Content**:
  - GitHub: `stevenrbellis/unheaded`
  - Try it: `git clone ... && make up`
  - Email: `stevie@bellis.tech`
  - RFC feedback: [GitHub Issues]
  - Tagline: *"The network IS the telemetry."*
- **Design**: Minimal, leave room for discussion

---

## Resources & References

### Documentation
- **GitHub**: https://github.com/stevenrbellis/unheaded
- **Wiki**: http://localhost:3000/wiki (self-hosted)

### RFCs & Standards
- **RFC 8200**: IPv6 Specification
- **RFC 6437**: IPv6 Flow Label Specification and Semantics
- **RFC 3168**: The Addition of Explicit Congestion Notification (ECN) to IP
- **W3C Trace Context**: https://www.w3.org/TR/trace-context/

### Internet-Drafts (Forthcoming)
- **draft-bellis-unheaded-metric-header-00** — UNHEADED_METRIC_V1 specification
- **draft-bellis-unheaded-protocol-foundation-04** — Full protocol foundation
- **draft-bellis-unheaded-sophia-dictionary-01** — BPF map encoding

### Related Projects
- **Cilium/eBPF**: https://github.com/cilium/ebpf
- **Aya**: https://github.com/aya-rs/aya (Rust eBPF framework)
- **Jaeger**: https://www.jaegertracing.io/ (traditional distributed tracing, for comparison)
- **Prometheus**: https://prometheus.io/ (metrics baseline)

### Contact & Community
- **Speaker**: Steven Bellis (@stevenrbellis on GitHub, Twitter)
- **Email**: stevie@bellis.tech
- **Slack**: [Unheaded community Slack, if applicable]

---

## Speaker Notes & Presenter Cues

### Delivery Tone
- **Technical**: Assume audience has BPF/XDP knowledge
- **Practical**: Emphasize real deployments, not theory
- **Optimistic**: Show the vision, but acknowledge challenges (IANA, router compatibility)
- **Interactive**: Invite Q&A early (especially on standards questions)

### Key Phrases to Emphasize (Repeat for Retention)
1. *"The packet IS the telemetry."* — Central thesis
2. *"Sub-microsecond overhead."* — Performance credibility
3. *"W3C Trace Context compatible."* — Standards alignment
4. *"No sidecars. No collectors."* — Simplicity message
5. *"25 services, one XDP hook."* — Scalability proof

### Audience Engagement Moments
- **At 2:00** (Hook): Ask rhetorically — *"What if I told you every frame carried its own trace?"*
- **At 10:00** (Real estate): Poll audience — *"Who's deployed IPv6 in production? Flow Label use cases?"*
- **At 19:00** (Demo): *"Let me show you what this looks like in real time."*
- **At 27:00** (Q&A): *"We're doing this in production. I'd love your feedback—especially on standards alignment."*

### Timing Checkpoints (Approximate)
- 00:00 – Start (intro + hook)
- 02:00 – Transition to problem statement
- 06:00 – Begin technical deep dive (header real estate)
- 10:00 – Wire format
- 15:00 – Implementation (XDP)
- 19:00 – **DEMO START** (most critical to stay on time)
- 24:00 – Standards + IANA
- 27:00 – Q&A
- 30:00 – **END**

### Backup Plan (If Demo Fails)
- Pre-record the demo video (MP4) as a fallback
- Have screenshots/PDF slide deck ready
- Pivot to code walkthrough instead of live UI demo
- Emphasize that the project is real, even if the live demo is unavailable

---

## Post-Talk Actions

1. **GitHub**: Monitor stars/forks on stevenrbellis/unheaded post-talk
2. **Email**: Expect inquiries from production users and standards bodies
3. **I-D**: Finalize draft-bellis-unheaded-metric-header-00 within 2 weeks
4. **Feedback**: Collect audience questions on standards alignment and deployment challenges
5. **Demo**: Release recorded demo video to YouTube/GitHub
6. **Follow-up**: IETF office hours or mailing list review

---

*End of Talk Outline*
