# What Unheaded Does

*Middle School Level*

---

## The Three Pillars

Unheaded is built on three pillars. Every feature, every design decision, and
every line of code supports at least one of these:

```
    +------------------+------------------+------------------+
    |   NETWORKING     |  OBSERVABILITY   |    SECURITY      |
    |                  |                  |                  |
    |  Connect your    |  See everything  |  Protect without |
    |  services.       |  that happens.   |  reading data.   |
    |                  |                  |                  |
    |  Service mesh,   |  eBPF tracing,   |  Container       |
    |  message bus,    |  metrics, logs,  |  hardening,      |
    |  load balancing, |  dashboard,      |  zero data       |
    |  discovery       |  alerting        |  access, TLS     |
    +------------------+------------------+------------------+
```

Let us look at each one.

---

## Pillar 1: Networking

### The Service Mesh

In a modern application, there is not just one program running. There are
many programs (called **services**) that each do one specific job. They need
to talk to each other constantly.

Unheaded manages this communication through a **service mesh**: a network
layer that handles connection routing, load balancing, retries, and
timeouts. Services do not need to know where other services are running or
how to handle network failures. The mesh handles it.

```
Without a service mesh:
  Service A must know Service B's exact IP and port.
  Service A must handle retries if Service B is down.
  Service A must handle load balancing across Service B replicas.
  Service A must handle timeouts.
  Every service duplicates this logic.

With Unheaded's mesh:
  Service A says "send this to Service B."
  The mesh handles discovery, routing, retries, load balancing, timeouts.
  Services focus on their actual job.
```

### The Message Bus (Wotan)

Instead of services talking directly to each other, they communicate through
**Wotan**, a central message bus. Think of it like a bulletin board:

```
                   +----------+
  Timeguru ------->|          |------> Dashboard
                   |  WOTAN   |
  Captain -------->|  (bus)   |------> Kanban App
                   |          |
  Architect ------>|          |------> Micromanager
                   +----------+
```

When a service has something to say, it **publishes** a message to a topic
on Wotan. Any service that cares about that topic **subscribes** to it and
receives the message automatically. The publisher does not need to know who
is listening.

Topics are organized by subject:

- `network.traces` -- Packet tracking events
- `timeline.updates` -- Roadmap changes
- `alerts.critical` -- Emergency notifications
- `logs.timeguru.info` -- Log messages from timeguru

### Service Discovery

When a new service starts up, it needs to find the other services. Unheaded
handles this automatically through a four-layer discovery system:

1. **Wotan registration**: Services announce themselves on the bus
2. **Port scan**: Check if expected ports are listening
3. **Convention scan**: Look for config files in standard locations
4. **Static fallback**: Use a known-good list of defaults

Services do not hard-code addresses. They discover each other dynamically.

---

## Pillar 2: Observability

Observability means the ability to understand what is happening inside a
system by looking at what comes out of it. Unheaded provides observability
at four levels:

### Level 1: Packet Tracing (eBPF)

This is the deepest level. eBPF programs run inside the Linux kernel itself,
attached to the network stack. They see every packet at the moment it
arrives, before any application code touches it.

More on eBPF below -- it is important enough to deserve its own section.

### Level 2: Metrics

Every service reports numerical measurements:

```
unheaded_http_requests_total{service="timeguru", status="200"}  15847
unheaded_http_request_duration_seconds{service="timeguru"}      0.012
unheaded_wotan_messages_published{service="timeguru"}           3291
```

These metrics are collected by Prometheus and displayed on Grafana dashboards
(or any compatible backend -- Unheaded supports interchangeable observability
backends).

### Level 3: Structured Logging

Every service emits structured JSON log entries:

```json
{
  "level": "info",
  "service": "timeguru",
  "trace_id": "abc123def456",
  "method": "GET",
  "path": "/api/v1/timeline",
  "status": 200,
  "duration_ms": 12,
  "message": "request completed"
}
```

Structured means machine-readable. Instead of a freeform string like
"Request completed in 12ms", the data is organized into labeled fields that
can be searched, filtered, and aggregated automatically.

### Level 4: The Dashboard

All of this data -- packet traces, metrics, and logs -- flows into a real-time
dashboard that shows:

- Live packet flow visualization
- System metrics (CPU, memory, request rates)
- Service health status
- Alert history
- Log search and filtering

```
+----------------------------------------------------------+
|  UNHEADED DASHBOARD                            [LIVE]    |
|                                                          |
|  Services: 10/10 healthy      Packets/sec: 12,847       |
|                                                          |
|  +-- Packet Flow --+  +-- Latency (ms) --+              |
|  | Shield -> Hop1  |  | p50: 2.1         |              |
|  | Hop1 -> Hop2    |  | p95: 8.4         |              |
|  | Hop2 -> Dest    |  | p99: 42.0        |              |
|  | Dest -> Shield  |  +------------------+              |
|  +----------------+                                      |
|                                                          |
|  +-- Recent Alerts ----+  +-- Active Flows -----+       |
|  | 15:00 WARN ring 80% |  | TCP: 847            |       |
|  | 14:52 INFO deploy ok|  | UDP: 231            |       |
|  +---------------------+  +---------------------+       |
+----------------------------------------------------------+
```

---

## What Is eBPF?

**eBPF** (extended Berkeley Packet Filter) is a technology that lets you run
small programs inside the Linux kernel without modifying the kernel itself.

Think of the Linux kernel as the engine of a car. Normally, if you want to
change how the engine works, you have to rebuild it. eBPF is like adding a
sensor to the engine that can read gauges and flip switches without
disassembling anything.

### Why Is eBPF Special?

1. **Speed**: eBPF programs run at the kernel level, which means they process
   packets at nearly the speed of the network hardware. There is no "send it
   up to the application and back down" overhead.

2. **Safety**: Before an eBPF program is loaded, the kernel's **verifier**
   checks it exhaustively. The verifier proves that the program will:
   - Always terminate (no infinite loops)
   - Never access memory it should not
   - Never crash the kernel

   If the program fails verification, it is rejected. This is not a
   promise -- it is a mathematical proof.

3. **No kernel modification**: You do not need to recompile or restart the
   kernel. eBPF programs are loaded and unloaded dynamically.

### Where Unheaded Uses eBPF

Unheaded runs eBPF programs at three layers of the network stack:

```
PACKET ARRIVES
      |
      v
+------------------+
| XDP (eXpress     |  <-- packet_marker / shield_xdp
|  Data Path)      |  Earliest possible interception.
|                  |  Before the kernel allocates memory for the packet.
+------------------+
      |
      v
+------------------+
| TC (Traffic      |  <-- flow_tracker
|  Control)        |  After basic kernel processing.
|                  |  Sees both incoming and outgoing packets.
+------------------+
      |
      v
+------------------+
| kprobes          |  <-- latency_probe
| (kernel probes)  |  Hooks into kernel functions like tcp_sendmsg.
|                  |  Measures function-level latency.
+------------------+
      |
      v
APPLICATION receives the packet
```

Each layer adds a different kind of visibility:

| Layer | Program | What It Sees |
|-------|---------|-------------|
| XDP | packet_marker / shield | Raw packets at wire speed. Injects and reads Monad tracking data. |
| TC | flow_tracker | TCP/UDP connections. Tracks state (SYN, established, FIN). Counts bytes and packets per flow. |
| kprobes | latency_probe | Kernel function timing. Measures how long tcp_sendmsg and tcp_recvmsg take. |

---

## The Wire Protocol

The information that eBPF programs read and write needs to be stored
somewhere that travels with the packet. Unheaded uses **IPv6 Hop-by-Hop
extension headers** for this.

### What Are Extension Headers?

IPv6 was designed to be extensible. Between the main IPv6 header and the
payload, you can insert additional **extension headers** that carry extra
information.

```
Normal IPv6 packet:
  [IPv6 Header] -> [TCP/UDP] -> [Payload]

IPv6 packet with extension header:
  [IPv6 Header] -> [Extension Header] -> [TCP/UDP] -> [Payload]
```

The specific type Unheaded uses is called a **Hop-by-Hop Options header**.
This type is special because it must be processed by every router the packet
passes through -- which is exactly what Unheaded wants.

### The Monad

Inside the Hop-by-Hop header, Unheaded places a 20-byte data structure
called the **Monad**. This is the tracking sticker from the ELI5 chapters,
now with technical detail.

The Monad is a **register file**: a small block of named fields that eBPF
programs can read and write as the packet passes through each hop.

```
Byte Layout (20 bytes):

Offset  Field               Size  Purpose
------  ------------------  ----  --------------------------
0x00    Version             1B    Protocol version (currently 1)
0x01    Src Service ID      1B    Source service identifier
0x02    Dst Service ID      1B    Destination service identifier
0x03    Hop Count           1B    Number of hops traversed
0x04    QoS Class           1B    Quality of service tier
0x05    Flow Action         1B    What hops should do (forward/trace/drop)
0x06    Circuit State       1B    Circuit breaker state (closed/open/half)
0x07    Flags               1B    8 boolean flags (traced, canary, etc.)
0x08    Latency Hint        2B    Expected latency (network byte order)
0x0A    Deploy Ring         1B    Deployment environment
0x0B    Mesh Flags          1B    Service mesh routing flags
0x0C    Src Prefix Lo       1B    Low byte of source subnet
0x0D    Dst Prefix Lo       1B    Low byte of destination subnet
0x0E    Scratch[0-3]        4B    General-purpose registers
0x12    Checksum            2B    CRC-16/CCITT integrity check
```

The checksum covers bytes 0x00 through 0x11 (the first 18 bytes). This lets
every hop verify that the Monad has not been corrupted in transit.

---

## Pillar 3: Security

### Zero Data Access

Unheaded operates on packet headers and metadata. It never reads, stores, or
processes the payload (the actual user data inside the packet).

This is not a policy -- it is an architectural constraint. The eBPF programs
that process the Monad operate on specific byte offsets in the packet header.
They physically cannot access the payload bytes. The BPF verifier enforces
this at load time.

### Container Hardening

Every service runs in an isolated container with:

- **Seccomp filters**: The service can only call specific kernel functions.
  Dangerous operations are blocked at the kernel level.
- **Capability dropping**: Linux capabilities that the service does not need
  are removed. A service that does not need to bind to low ports cannot.
- **Read-only filesystem**: The container's filesystem is read-only. The
  service cannot write files except in specifically allowed directories.
- **Network isolation**: Each container has its own network namespace. It can
  only reach the services it is explicitly allowed to talk to.

### Default Deny

All network policies start from "deny everything." Each service explicitly
declares which ports it needs open and which other services it needs to reach.
Anything not explicitly allowed is blocked.

```
Firewall default: DENY ALL

Timeguru allowed:
  - Inbound:  TCP 19000 from 10.10.10.0/24  (API)
  - Outbound: TCP 18001 to 10.10.10.10      (Wotan gRPC)
  - All else: DENIED
```

### TLS Everywhere

All external traffic uses TLS 1.3 (the latest version of the encryption
protocol that protects web traffic). Internal service-to-service traffic
uses gRPC with transport-layer security.

---

## Why This Approach Is Different

Most monitoring tools work at the **application level**. They add code to
your applications that reports metrics and logs. This is useful, but it has
blind spots:

1. **You can only see what the application tells you.** If the application
   crashes before it can report, you see nothing.

2. **Network problems are invisible.** If a packet is dropped between two
   services, the application just sees a timeout. It does not know why.

3. **You need to modify every application.** Each service needs monitoring
   libraries, configuration, and instrumentation code.

Unheaded works at the **kernel level**. The eBPF programs see packets before
the application does, and they see packets that the application never
receives (because they were dropped). This means:

1. **Nothing is invisible.** Every packet that touches the network is tracked.
2. **No application changes needed.** The monitoring happens below the
   application, in the kernel.
3. **Kernel-level speed.** The overhead is measured in nanoseconds, not
   milliseconds.

The tradeoff is complexity. Kernel-level programming is harder than
application-level programming. But the result is a level of visibility that
application-level tools simply cannot provide.

---

## How It All Fits Together

Here is the complete data flow, from packet to dashboard:

```
1. Packet arrives at network boundary
       |
2. Shield XDP program:
   - Checks blocklist
   - Strips external extension headers
   - Inserts 24-byte Hop-by-Hop header with Monad
   - Sets Flow Label for trace correlation
   - Emits BIRTH event to Anamnesis ring buffer
       |
3. Hop programs at each internal router:
   - Read Monad from packet
   - Increment hop count
   - Check circuit breaker state
   - Apply flow action (forward, trace, sample, drop)
   - Update checksum
   - Write Monad back to packet
   - Emit HOP event to Anamnesis ring buffer
       |
4. Packet reaches destination service
   - Application processes the payload
   - Monad is ignored by the application (it is in the header, not the payload)
       |
5. Response packet leaves the network
   - Shield TC program reads final Monad state
   - Emits DEATH event to Anamnesis ring buffer
   - Strips the Hop-by-Hop header
   - Packet exits as standard IPv6
       |
6. trace-collector (Rust userspace program):
   - Reads Anamnesis ring buffer
   - Correlates events by Flow Label
   - Publishes to Wotan message bus
       |
7. dashboard-backend (Go service):
   - Subscribes to trace events on Wotan
   - Aggregates metrics
   - Pushes updates via WebSocket to browser
       |
8. Dashboard (vanilla JavaScript):
   - Renders real-time packet flow visualization
   - Displays metrics, logs, and alerts
```

Every step is automated. No manual instrumentation. No agent installation.
No application code changes. Plug a service into the network and it is
automatically observed from the moment its first packet hits the wire.

---

*Next: [Architecture Overview](../high-school/architecture-overview.md) --
The full technical stack.*
