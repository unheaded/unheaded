# The Unheaded Protocol: The Middle School Version

## What Problem Does This Solve?

When companies run websites and apps, they have dozens (or hundreds) of services talking to each other — a login service, a payment service, a database, a notification system. When something breaks, figuring out *what went wrong* is like detective work. Engineers add logging, tracing, metrics, proxies, sidecars — layers and layers of extra software just to *see* what's happening.

The Unheaded Protocol takes a completely different approach: **the network itself becomes the observability system**. Instead of bolting monitoring on top, every packet carries its own monitoring data *inside it*.

## The Monad: A 20-Byte Register File

Every packet inside the Unheaded network carries a tiny 20-byte data structure called the **Monad** (named after a concept in Gnostic philosophy meaning "The One" — the single unified thing that all packets share).

Think of it as a little passport that gets stamped at every border crossing:

```
Offset  What It Holds              Example
------  -------------------------  -------------------------
0x00    Protocol version           1
0x01    Source service ID          "timeguru" (code: 0x02)
0x02    Destination service ID     "dashboard" (code: 0x06)
0x03    Hop count                  3 (how many stops so far)
0x04    Trace hash (4 bytes)       0xA3F10B22
0x08    QoS class                  "realtime" (code: 0x03)
0x09    Flow action                "trace" (code: 0x01)
0x0A    Circuit breaker state      "closed" (code: 0x01)
0x0B    Flags                      [traced, encrypted]
0x0C    Latency hint               47 microseconds
0x0E    Deployment ring            "production"
0x0F    Mesh flags                 (routing metadata)
0x10    Reserved                   (for future use)
0x12    Checksum                   CRC-16 integrity check
```

## Shield: The Border Guard

The Monad only exists inside the **Limited Domain** — the private network. A component called **Shield** acts as the border:

- **Packets arriving** from the internet: Shield adds the 20-byte Monad (stamps the passport)
- **Packets leaving** to the internet: Shield strips the Monad off (confiscates the passport)

The outside world never sees a single byte of protocol metadata. This means:
- No information leakage
- No compatibility issues with the public internet
- The protocol is completely invisible to attackers

## Sophia: The Living Dictionary

Most of the Monad's fields are **exponent-encoded** — they're just single-byte codes (0-255) whose meaning comes from a lookup dictionary called **Sophia**.

For example, the byte `0x03` in the `src_service_id` field doesn't inherently mean anything. You look it up in Sophia's dictionary:

```
Sophia Dictionary v47:
  0x01 → "captain"
  0x02 → "timeguru"
  0x03 → "architect"
  0x04 → "micromanager"
  ...
```

The powerful part: **you can change the dictionary at runtime without touching any code**. Sophia's dictionaries are stored as BPF maps in the kernel. Update the map, and instantly every packet gets decoded differently. Add a new service? Just add a dictionary entry. No redeployment needed.

### Compositional Lookup

Sophia dictionaries are *trees*, not flat tables. Each byte narrows the context:

```
[0x07]             → "service_identity" (category)
[0x07, 0x03]       → "architect" (specific service)
[0x07, 0x03, 0x02] → "architect.topology_query" (specific RPC)

But with a different first byte:
[0x08, 0x03]       → "mirror" (a flow action, not a service!)
```

One byte = 256 meanings. Two bytes = 65,536 meanings. Three bytes = 16.7 million meanings. All encoded in the same tiny header space.

## The Void: BPF Programs at Every Hop

At every network hop inside the domain, small **BPF programs** (called "Shim" programs) run inside the Linux kernel. These programs:

1. Read the Monad from the packet
2. Verify the checksum (is the data intact?)
3. Increment the hop count
4. Perform hop-specific logic:
   - **Shield** (the firewall): Check source against blocklists, stamp security info
   - **Pauldrons** (the load balancer): Read destination, pick a backend server
   - **Hauberk** (the service mesh): Check circuit breaker state, prevent cascading failures
   - **Vambraces** (observability): Record everything to the event log
5. Recompute the checksum
6. Forward the packet

All of this happens in **nanoseconds** — before the packet even reaches the application.

## Anamnesis: The Network Remembers

Every BPF program that touches a packet writes an event to a **ring buffer** called **Anamnesis** (Greek for "remembrance"). Each event captures:

- Timestamp (nanosecond precision)
- Event type (birth, computed, death, anomaly, chaos)
- The Monad state *before* this hop
- The Monad state *after* this hop
- Trace ID for correlation

This means **the network has perfect memory**. You can reconstruct the exact journey of any packet through the entire system — where it went, what happened at each hop, how long each step took.

## Wotan: The Translator

**Wotan** is the central message bus that bridges two worlds:

- **Upward** (kernel → userspace): Wotan reads raw bytes from Anamnesis ring buffers, decodes them through Sophia's dictionaries, and publishes structured events via gRPC streaming
- **Downward** (userspace → kernel): When engineers change a policy or routing rule, Wotan encodes it into BPF map updates that take effect on the very next packet

Wotan speaks both "nanosecond language" (binary kernel data) and "millisecond language" (JSON, dashboards, human-readable events).

## The Four-Layer Architecture

```
Layer 3: THE KINGDOM (Go services, REST APIs, dashboards)
         Speed: milliseconds | Language: JSON, HTTP
              ↕ Wotan translates ↕
Layer 2: WOTAN (message bus, dictionary lookups, pub/sub)
         Speed: microseconds | Language: structured events
              ↕ ring buffers ↕
Layer 1: THE VOID (BPF programs at XDP, TC, kprobe hooks)
         Speed: nanoseconds | Language: BPF bytecode, maps
              ↕ reads/writes packet headers ↕
Layer 0: THE PROTOCOL (IPv6 + 20-byte Monad in Hop-by-Hop options)
         Speed: wire speed | Language: bytes
```

## Why Is This Different?

Traditional infrastructure: the network is a dumb pipe. Applications bolt monitoring on top — sidecars, proxies, logging libraries, tracing frameworks. Every layer adds latency, complexity, and failure modes.

Unheaded: **the network IS the monitoring**. Every packet carries its own observability data. Every hop contributes to the picture. No sidecars. No proxies. No Envoy. No Istio. The mesh is the protocol itself.

| Traditional | Unheaded |
|------------|----------|
| Separate monitoring system | Monitoring IS the protocol |
| Add tracing library to each service | Every packet is automatically traced |
| Deploy Envoy sidecar per pod | BPF programs run in the kernel |
| Log aggregation via Fluentd/ELK | Ring buffers capture everything at wire speed |
| Service mesh via Istio | Mesh logic encoded in packet headers |

---

*Previous: [← ELI5 Explanation](01-eli5.md) | Next: [High School Explanation →](03-high-school.md)*
