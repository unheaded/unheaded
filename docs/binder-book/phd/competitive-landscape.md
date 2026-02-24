# Competitive Landscape

*PhD Level*

---

## Abstract

This document surveys the ecosystem of projects relevant to Unheaded's
capabilities, organized across 16 categories and encompassing 72 projects. We
construct a formal capability matrix, analyze each category's approach to the
core problems (observability, control plane, wire-level access), and identify
Unheaded's unique positioning: wire-level depth with per-packet stateful
computation, operating below the userspace abstraction boundary.

---

## 1. Taxonomy

We organize the landscape into 16 categories based on the primary abstraction
layer each project operates at:

```
Category                        Layer         Count
-------------------------------+-------------+-----
1.  eBPF Observability          Kernel         7
2.  eBPF Networking             Kernel         4
3.  Service Mesh (data plane)   Userspace/L7   6
4.  Service Mesh (control)      Userspace      4
5.  Distributed Tracing         Application    6
6.  Metrics & Monitoring        Application    6
7.  Log Aggregation             Application    5
8.  Network Monitoring          Network/L3-4   5
9.  SDN Controllers             Network        4
10. Packet Processing           Kernel/User    4
11. Configuration Management    Userspace      5
12. Container Orchestration     Userspace      4
13. Chaos Engineering           Application    3
14. API Gateways                Userspace/L7   4
15. Network Security            Kernel/L3-7    3
16. Protocol Frameworks         Kernel/User    2
                                        Total: 72
```

---

## 2. Category Analysis

### 2.1 eBPF Observability (7 projects)

| Project | Scope | Approach | Limitation vs. Unheaded |
|---------|-------|----------|------------------------|
| **Cilium Hubble** | L3-L7 flow visibility | eBPF at TC, identity-aware | No per-packet state mutation; observe-only. No wire protocol. |
| **Pixie** (CNCF) | Application performance | eBPF at kprobes + uprobes, auto-instrumentation | Application-layer focus. No packet-level register file. |
| **Tetragon** | Security observability | eBPF at tracepoints, LSM hooks | Security events only. No network data plane integration. |
| **bpftrace** | Ad-hoc kernel tracing | eBPF scripting language, per-probe | Debugging tool, not production pipeline. No persistent state. |
| **Inspektor Gadget** | Kubernetes debugging | eBPF gadgets for container inspection | Debugging focus. No continuous per-packet computation. |
| **Parca** | Continuous profiling | eBPF at perf events, flamegraph | CPU profiling only. No network packet visibility. |
| **Caretta** | Service map auto-discovery | eBPF at TCP events | Map-only (topology). No per-packet state or tracing. |

**Gap**: All eBPF observability projects are read-only. They observe kernel
events but do not inject state into packets. Unheaded's Monad is a read-write
register file that travels with the packet -- this is a fundamentally
different approach.

### 2.2 eBPF Networking (4 projects)

| Project | Scope | Approach | Limitation vs. Unheaded |
|---------|-------|----------|------------------------|
| **Cilium** | CNI + network policy | eBPF at XDP/TC, identity-based routing | Write-capable (identity label injection) but no general-purpose register file. Fixed semantics. |
| **Katran** | L4 load balancer | eBPF at XDP, IPVS replacement | Single-function (load balancing). No observability or state propagation. |
| **Calico eBPF** | Network policy | eBPF at TC, policy enforcement | Policy enforcement only. No per-packet computation or tracing. |
| **XDP-tools** | Reference implementations | XDP programs for common tasks | Library, not a platform. No integrated pipeline. |

**Gap**: Cilium is the closest competitor in eBPF networking. Its identity
labels are conceptually similar to the Monad but are fixed-semantics (pod
identity, namespace, service). The Monad is a general-purpose register file
with user-definable fields, circuit breaker state, QoS hints, and scratch
registers. Cilium cannot do per-hop stateful computation across the network
fabric.

### 2.3 Service Mesh - Data Plane (6 projects)

| Project | Scope | Approach | Limitation vs. Unheaded |
|---------|-------|----------|------------------------|
| **Envoy** | L7 proxy | Userspace C++, sidecar | Per-hop latency: 1-5ms per proxy hop. Operates above the kernel. |
| **Linkerd2-proxy** | L7 proxy (Rust) | Userspace Rust, sidecar | Same architecture as Envoy, lighter weight. Still userspace latency. |
| **NGINX** | Web server / L7 proxy | Userspace C, reverse proxy | Not a mesh data plane; used as gateway in Unheaded. |
| **HAProxy** | L4/L7 load balancer | Userspace C, high-performance | Single-hop. No mesh, no distributed state. |
| **Traefik** | L7 proxy | Userspace Go, Kubernetes-native | Same limitations as Envoy (userspace per-hop processing). |
| **MOSN** | L7 proxy | Userspace Go, Envoy-compatible | Same limitations. |

**Gap**: All service mesh data planes operate in userspace, adding 1-5ms
latency per sidecar hop. Unheaded operates at XDP/TC -- packets are processed
before they reach userspace. Per-hop overhead is measured in nanoseconds
(~100ns for Monad read/write/CRC), not milliseconds.

The architectural difference is that sidecar proxies intercept traffic at the
application layer (L7), while Unheaded intercepts at the network layer (L3).
This means Unheaded sees packets that never reach the application (dropped,
rate-limited, misrouted) -- a class of events that sidecar-based meshes
are structurally blind to.

### 2.4 Service Mesh - Control Plane (4 projects)

| Project | Scope | Approach | Limitation vs. Unheaded |
|---------|-------|----------|------------------------|
| **Istio** | Full service mesh | Control plane for Envoy sidecars | Depends on userspace data plane. Complex (100+ CRDs). |
| **Linkerd** | Lightweight mesh | Control plane for linkerd2-proxy | Simpler than Istio but still userspace data plane. |
| **Consul Connect** | Service mesh + discovery | Envoy sidecars + Consul KV store | Tied to HashiCorp ecosystem. Userspace data plane. |
| **Open Service Mesh** | Kubernetes mesh (archived) | Envoy-based, SMI-compatible | Archived. Was userspace data plane. |

**Gap**: Control plane projects define intent (routing rules, retries,
timeouts). Unheaded's control plane (unheaded-daemon) performs the same role
but targets eBPF maps instead of sidecar configuration. The semantic
difference: Istio configures what Envoy should do when a packet arrives.
Unheaded configures what the kernel should do before the packet reaches
userspace.

### 2.5 Distributed Tracing (6 projects)

| Project | Scope | Approach | Limitation vs. Unheaded |
|---------|-------|----------|------------------------|
| **Jaeger** | Distributed tracing | OpenTracing SDK, application instrumentation | Requires manual instrumentation. Traces application calls, not packets. |
| **Zipkin** | Distributed tracing | B3 headers, application instrumentation | Same as Jaeger. Application-layer only. |
| **Tempo** (Grafana) | Trace storage backend | Object storage for traces | Storage only. No collection or instrumentation. |
| **OpenTelemetry** | Unified telemetry | SDK + collector + protocol | Comprehensive but application-layer. No kernel-level visibility. |
| **SigNoz** | Observability platform | OpenTelemetry-based, full stack | Application-layer. Same instrumentation overhead. |
| **Lightstep** | Commercial tracing | Application instrumentation | Proprietary. Application-layer. |

**Gap**: All distributed tracing systems require application instrumentation:
the developer must add SDK calls to propagate trace context. Unheaded injects
trace context (the Monad) at the network layer, before the application sees
the packet. This means:
- Zero application code changes.
- Traces include network-layer events (packet drops, routing decisions).
- Third-party binaries (with no source code) are automatically traced.

The tradeoff: application-layer tracing can correlate across function calls
within a service. Unheaded's packet-level tracing correlates across network
hops. They are complementary, not competing.

### 2.6 Metrics and Monitoring (6 projects)

| Project | Approach | Gap |
|---------|----------|-----|
| **Prometheus** | Pull-based metrics scraping | Application must expose /metrics. No packet-level granularity. |
| **Grafana** | Dashboard and visualization | Visualization layer only. Unheaded uses custom dashboard. |
| **Datadog** | Commercial SaaS monitoring | Proprietary. Agent-based. Application-layer. |
| **Victoria Metrics** | High-performance TSDB | Storage optimization. Same data model as Prometheus. |
| **Nagios** | Host/service monitoring | Polling-based. No packet-level visibility. |
| **Zabbix** | Infrastructure monitoring | Agent-based. Network monitoring via SNMP, not eBPF. |

### 2.7 Log Aggregation (5 projects)

| Project | Approach | Gap |
|---------|----------|-----|
| **Elasticsearch / ELK** | Full-text search + Logstash + Kibana | Application log indexing. No kernel-level structured events. |
| **Loki** (Grafana) | Log aggregation with label-based indexing | Efficient storage. Still application-layer logs. |
| **Fluentd / Fluent Bit** | Log collection and forwarding | Collection pipeline. No packet-level events. |
| **Splunk** | Commercial log analytics | Proprietary. Application-layer. |
| **Graylog** | Open-source log management | Application-layer log aggregation. |

### 2.8 Network Monitoring (5 projects)

| Project | Approach | Gap |
|---------|----------|-----|
| **ntopng** | Deep packet inspection | Passive monitoring. No state injection. Reads only. |
| **Wireshark / tshark** | Protocol analysis | Debugging tool. Not production pipeline. |
| **NetFlow / sFlow / IPFIX** | Flow-level telemetry via router sampling | Sampled, not per-packet. No state propagation. |
| **Zeek (Bro)** | Network security monitor | Application-layer protocol parsing. Passive. |
| **Suricata** | IDS/IPS | Rule-based detection. No distributed state. |

**Gap**: Network monitoring tools are passive observers. They read packets and
produce events but do not inject state into the packet stream. Unheaded is an
active participant: the Monad carries state through the network, and each hop
can act on it (QoS decisions, circuit breaking, flow action enforcement).

### 2.9 SDN Controllers (4 projects)

| Project | Approach | Gap |
|---------|----------|-----|
| **ONOS** | Java-based SDN controller | Centralized controller model. All decisions via controller round-trip. |
| **OpenDaylight** | Java-based SDN platform | Same architecture as ONOS. Controller bottleneck. |
| **Tungsten Fabric** | SDN + NFV | vRouter-based. Userspace forwarding plane. |
| **Kubernetes CNI** | Container network interface | Interface standard, not implementation. |

**Gap**: SDN controllers use a centralized model where the controller makes
forwarding decisions and pushes them to switches. This introduces controller
latency and creates a single point of failure. Unheaded distributes the
decision logic to each hop via eBPF programs and BPF maps, eliminating the
controller round-trip.

### 2.10 Packet Processing Frameworks (4 projects)

| Project | Approach | Gap |
|---------|----------|-----|
| **DPDK** | Kernel bypass, userspace packet processing | Raw speed but no kernel integration. Requires dedicated cores. |
| **VPP (fd.io)** | Vector packet processing | Userspace forwarding. No kernel-level eBPF integration. |
| **P4** | Programmable switch language | Switch-level (hardware). Not applicable to general-purpose servers. |
| **XDP (kernel)** | eBPF at driver level | Framework only. Unheaded builds on XDP. |

**Gap**: DPDK and VPP achieve high throughput by bypassing the kernel entirely.
Unheaded operates within the kernel (XDP/TC), which means it works with the
existing network stack rather than replacing it. This is a deliberate tradeoff:
Unheaded does not need dedicated cores or custom NIC drivers, and it
coexists with iptables, nftables, tc, and all other kernel networking
subsystems.

### 2.11-2.16 Remaining Categories (Summary)

| Category | Projects | Key Gap |
|----------|----------|---------|
| Config Management (5) | Ansible, Terraform, Puppet, Chef, Salt | Infrastructure provisioning, not runtime observability. Unheaded generates output for all five. |
| Container Orchestration (4) | Kubernetes, Nomad, Docker Swarm, LXD | Scheduling and lifecycle. No packet-level visibility. |
| Chaos Engineering (3) | Chaos Monkey, Litmus, Gremlin | Fault injection. Unheaded's Yaldabaoth subsystem provides eBPF-level chaos (bit flips, delays, drops). |
| API Gateways (4) | Kong, Tyk, Ambassador, Apigee | L7 request routing. Userspace. No kernel-level processing. |
| Network Security (3) | Falco, Wazuh, OSSEC | Audit and detection. No wire-level state propagation. |
| Protocol Frameworks (2) | gRPC, QUIC | Transport protocols. Unheaded operates below them (L3 extension header). |

---

## 3. Formal Capability Matrix

We evaluate 16 representative projects (one per category) against 10 core
capabilities using a 4-level scale:

- **N** = Not applicable / not provided
- **P** = Partial (requires significant additional work or external tools)
- **Y** = Yes, provided out of the box
- **D** = Differentiating (uniquely deep implementation)

```
Capability           | Cilium | Envoy | Istio | Jaeger | Prom | DPDK | K8s  | Ansible | Unheaded
---------------------|--------|-------|-------|--------|------|------|------|---------|--------
Per-packet state     |   P    |   N   |   N   |   N    |  N   |  Y   |  N   |    N    |   D
Wire-level tracing   |   Y    |   N   |   N   |   N    |  N   |  P   |  N   |    N    |   D
eBPF data plane      |   Y    |   N   |   N   |   N    |  N   |  N   |  N   |    N    |   D
Per-hop computation  |   P    |   N   |   N   |   N    |  N   |  Y   |  N   |    N    |   D
Circuit breaking     |   N    |   Y   |   Y   |   N    |  N   |  N   |  N   |    N    |   D*
Service discovery    |   Y    |   P   |   Y   |   N    |  P   |  N   |  Y   |    P    |   Y
Metrics collection   |   Y    |   Y   |   Y   |   P    |  Y   |  N   |  P   |    N    |   Y
Distributed tracing  |   P    |   Y   |   Y   |   Y    |  N   |  N   |  N   |    N    |   D
Config management    |   N    |   N   |   P   |   N    |  N   |  N   |  P   |    Y    |   Y
Zero app changes     |   Y    |   N   |   N   |   N    |  N   |  Y   |  N   |    Y    |   D
```

*D\* for circuit breaking: Unheaded implements circuit breaking at the BPF
map level (XDP speed, sub-microsecond), vs. Envoy/Istio at L7 proxy level
(millisecond latency).*

### Key Differentiators

1. **Per-packet state (Monad)**: Only DPDK provides similar capability, but
   DPDK replaces the kernel network stack. Unheaded augments it.

2. **Wire-level tracing**: Cilium's Hubble provides flow-level visibility.
   Unheaded provides per-packet, per-hop visibility with a mutable register
   file.

3. **Per-hop computation**: The Monad enables computation at each network
   hop (circuit breaking, QoS adjustment, flow action enforcement) without
   controller round-trips.

4. **Zero application changes**: Unlike distributed tracing systems (Jaeger,
   Zipkin, OpenTelemetry), Unheaded requires no SDK, no code changes, and no
   instrumentation libraries. Observability is provided by the network layer.

---

## 4. The Wire-Level Depth Advantage

The fundamental distinction between Unheaded and the rest of the ecosystem
is the **layer of abstraction** at which it operates.

```
                    Application Layer (L7)
                    +-----------------------+
                    | Jaeger, Zipkin, OTel  |  Require SDK instrumentation
                    | Envoy, Istio, Linkerd |  Sidecar proxy (1-5ms/hop)
                    +-----------------------+
                              |
                    Transport Layer (L4)
                    +-----------------------+
                    | HAProxy, Katran       |  Load balancing decisions
                    +-----------------------+
                              |
                    Network Layer (L3)
                    +-----------------------+
                    | Cilium (identity)     |  Fixed-semantics labels
                    +-----------------------+
                              |
                    Wire Layer (L2-L3)
                    +-----------------------+
                    | UNHEADED (Monad)      |  General-purpose register file
                    |                       |  Per-hop stateful computation
                    |                       |  Mutable at XDP speed
                    +-----------------------+
                              |
                    Hardware (L1)
                    +-----------------------+
                    | P4, SmartNICs         |  Requires specialized hardware
                    +-----------------------+
```

Unheaded occupies a unique position: deeper than any software-defined
networking solution (Cilium, Calico, Envoy) but not requiring specialized
hardware (P4, SmartNICs). The eBPF instruction set runs on commodity Linux
kernels, which means the wire-level depth is accessible on any x86_64 or
aarch64 machine running kernel 5.8 or later.

### The Consequence

Every observability and networking tool above Unheaded in the stack is
structurally blind to events that Unheaded can see:

- Packets dropped by the kernel before reaching userspace
- Packets rejected by iptables/nftables rules
- Packets lost to interface queue overflow
- Packets reordered by the kernel scheduler
- Kernel-internal latencies (socket buffer queue time, scheduler delay)

Conversely, Unheaded cannot see application-layer events (function calls,
database queries, business logic). The two layers are complementary. The
binder book's formal specification proposes OpenTelemetry bridging as the
integration point.

---

## 5. Market Positioning

### 5.1 Segment Map

```
                       Deep       <-- Wire-level access -->       Shallow
                   +--------------------------------------------------+
  Full platform    |  UNHEADED                                        |
                   |                              Datadog  SigNoz     |
                   +--------------------------------------------------+
  Networking       |  Cilium                      Istio  Linkerd      |
                   |  Calico                      Kong   Envoy        |
                   +--------------------------------------------------+
  Observability    |  Pixie                       Jaeger Prometheus   |
                   |  Hubble   Tetragon           OTel   Grafana      |
                   +--------------------------------------------------+
  Infrastructure   |  DPDK     VPP                K8s    Terraform    |
                   |  P4                          Nomad  Ansible      |
                   +--------------------------------------------------+
```

Unheaded is the only project that combines wire-level depth (eBPF data plane
with per-packet stateful computation) with full-platform scope (observability,
networking, security, configuration management).

### 5.2 Competitive Moat

The moat is the protocol itself.

Replicating Unheaded's approach requires:
1. Designing a wire-format register file (Monad)
2. Implementing eBPF programs that pass the BPF verifier for packet mutation
3. Building the Shield boundary enforcement layer
4. Implementing the Sophia dictionary system
5. Building the Anamnesis event pipeline
6. Integrating all of the above into a working system

Each of these is individually tractable. The combination -- a verified,
production-quality, full-lifecycle packet processing pipeline with mutable
per-hop state -- has not been achieved by any other open-source or commercial
project as of February 2026.

Cilium is the closest project technically (eBPF at XDP/TC, identity-aware
networking) but has deliberately chosen not to implement per-packet mutable
state. Their identity labels are fixed at pod scheduling time and do not
change as packets traverse the network.

---

## 6. Conclusion

The competitive landscape reveals a consistent gap: the industry has built
sophisticated observability and networking tools at the application layer (L7)
and the transport layer (L4), but the wire layer (L2-L3) remains
underexploited. eBPF has enabled kernel-level visibility (Cilium, Hubble,
Pixie) but has not been used for kernel-level stateful computation on
per-packet data.

Unheaded fills this gap with the Monad register file -- a 20-byte data
structure that enables distributed computation at every network hop, without
controller round-trips, without application instrumentation, and without
leaving the kernel.

The result is a system with observability depth that no application-layer
tool can match and performance characteristics that no userspace proxy can
approach.

---

*Return to: [Binder Book Table of Contents](../README.md)*
