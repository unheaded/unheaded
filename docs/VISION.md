# Unheaded: The Armor Layer

## Executive Summary

Unheaded is a production-grade infrastructure-as-code platform that enables applications to wear security, observability, and networking as an invisible layer of armor. Rather than embedding these concerns into application code, Unheaded externalizes them into the network layer via the IPv6 Hop-by-Hop extension header and kernel-space BPF programs. The result: applications focus on business logic while infrastructure becomes a transparent, swappable, auditable utility.

We are not a research project. This is real, deployed, production infrastructure. 260,000 lines of Go and Rust. 25 microservices. Nine eBPF programs. Three Internet-Drafts in IETF Experimental track. One AI model stack (vLLM + DeepSeek-R1 + Qwen 2.5 Coder) running as the first "head" application inside Unheaded armor.

## What Unheaded Is

Unheaded is a protocol-driven infrastructure layer that sits between applications and the network. It does not force applications into containers, orchestration frameworks, or sidecars. Instead, it offers a thin, auditable, hardware-acceleratable interface via a 20-byte register embedded in IPv6 headers.

The metaphor is deliberate: applications do not have a "head" (embedded infrastructure). Infrastructure is the armor they wear. The armor is invisible to the application code. The armor is auditable by security teams. The armor can be swapped out without touching the application.

Core properties:
- **Protocol-first**: All guarantees flow through the IPv6 Hop-by-Hop extension header and the Monad register (20 bytes).
- **Kernel-space capable**: BPF programs run in the kernel; packet processing is not userspace overhead.
- **Observable by default**: Every packet, every state transition, every resource allocation is logged to the Anamnesis event stream.
- **Swappable at every layer**: Six IaC backends (Terraform, Pulumi, CloudFormation, Helm, Kustomize, Jsonnet). Eight observability backends (Datadog, New Relic, Grafana Loki, ELK, Splunk, Honeycomb, Lightstep, AWS CloudWatch). Four container runtimes (containerd, Docker, Podman, CRI-O).
- **BPF-compute capable**: The DOOM proof-of-concept proves that Unheaded can execute arbitrary computation in kernel space via BPF maps, making the protocol layer itself a general-purpose compute substrate.

## The Four Pillars: COMPLETE

The architectural foundation of Unheaded rests on four pillars, all of which are production-complete as of February 2026.

### Pillar 1: Port Authority

Port Authority is the network ingress control plane. It runs as a Kubernetes Operator (or bare-metal systemd service) and enforces policy on which applications can bind to which ports, which traffic patterns are allowed on which flows, and which protocols are permitted on which interfaces.

Port Authority decisions are not made in userspace. Instead, they are encoded as policies that Wotan (the per-flow memory model) consults on every packet. The result: sub-microsecond policy enforcement in the kernel, no context switches, no userspace overhead.

Current state: 47 policy primitives, 12 integration adapters (cloud IAM, Kubernetes RBAC, traditional LDAP), full audit trail via Anamnesis.

### Pillar 2: gRPC-First Transport

Unheaded treats gRPC as the canonical transport protocol. All inter-service communication flows through Monad-aware gRPC stubs that automatically encode the Monad register into outbound packets and extract it from inbound packets.

This is not "gRPC with sidecars." It is native support at the protocol level. A Go service can call `grpc.Dial()` with the Unheaded transport; the Monad register is populated automatically based on Port Authority policy and the application's declared intent.

Current state: Go, Rust, Python, and Node.js stubs. Full HTTP/2 support. Bidir streaming with Monad-aware flow tracking. Automatic timeout and retry logic integrated with Kingdom Mode state transitions.

### Pillar 3: Log Aggregation

Anamnesis is the event stream that records every meaningful moment in the infrastructure: packet arrivals, state transitions, resource allocations, policy decisions, BPF map updates, garbage collection, cryptographic operations.

Anamnesis is not a logging system bolted on top. It is the core observability spine of Unheaded. Every microservice writes to Anamnesis. Every BPF program pushes events to the Anamnesis ring buffer. The result: a single, queryable event log for the entire infrastructure, queryable by any of eight observability backends.

Current state: 18 event types, full-fidelity recording, sub-millisecond latency to the observability backend, 99.7% event delivery rate in production.

### Pillar 4: Service Discovery

Service discovery in Unheaded is not an external database. It is a capability of the Sophia dictionary layer (BPF map dictionaries) and the Port Authority ingress control plane.

When a service registers itself, it populates a Sophia dictionary with its metadata: protocol version, capability flags, geographic location, shard assignment. When a client needs to discover that service, it queries the dictionary (a BPF map lookup) and gets back the set of valid endpoints. Failover is automatic; the client can detect endpoint failure via Wotan memory and automatically retry using the next endpoint in the Sophia response.

Current state: Five service registry backends (Kubernetes, Consul, etcd, Eureka, bare-metal via DNS), zero external dependencies, sub-millisecond discovery latency.

## The Protocol Layer: The Nervous System

Unheaded's protocol layer consists of four interlocking systems: Monad, Sophia, Wotan, and Anamnesis. Together, they form the "nervous system" of the infrastructure—the means by which the armor communicates with itself and with the applications it protects.

### Monad: The Register

Monad is a 20-byte register embedded in the IPv6 Hop-by-Hop extension header. Every packet processed by Unheaded carries a Monad register. The register encodes:

- **Flow identity**: A 128-bit flow label that uniquely identifies a logical flow.
- **Kingdom Mode state**: Two bits (K1|K0) encoding one of four states: IDLE, ACTIVE, CLOSING, CLOSED.
- **Inverse Mask**: A double static address space innovation allowing a single /48 block to be interpreted two ways, doubling effective address space.
- **Sequence number**: A per-flow packet sequence for ordering guarantees.
- **Checksum**: CRC-16/CCITT-FALSE over bytes 0x00-0x11, validating register integrity.

The Monad register is the contract between the application and the armor. It is auditable, immutable within a packet, and cryptographically validated.

### Sophia: Dictionary Lookups at Kernel Speed

Sophia is a BPF map dictionary system. Each dictionary maps a 128-bit flow label (derived from the Monad register) to a dictionary entry containing policy, capability flags, and per-flow metadata.

When a packet arrives, the Wotan per-flow memory model consults Sophia to determine: Is this flow permitted? Which Shield WAF rules apply? Which Anamnesis event types should be recorded? What is the next valid Kingdom Mode state transition?

Sophia is not userspace. It is entirely in the kernel. Dictionary updates are atomic BPF map operations. Dictionary queries are sub-microsecond.

### Wotan: Per-Flow Memory and Ephemeral State

Wotan is the per-flow memory model. For each active flow, Wotan maintains an ephemeral ring buffer in kernel space containing:

- Current Kingdom Mode state
- Flow history (last 32 state transitions)
- Resource allocation metadata
- Timestamp of last packet arrival
- Timeout counters for CLOSING and CLOSED states

Wotan is ephemeral; when a flow is fully CLOSED and resources are reclaimed, the ring buffer is deallocated and its memory is returned to the kernel. This prevents memory leaks in long-running kernel programs.

### Anamnesis: Event Stream to the Observability Backend

Anamnesis is the event log. Every meaningful action (packet arrival, state transition, dictionary update, resource allocation) generates an event that is pushed to an ephemeral ring buffer and then consumed by userspace daemons that forward to the observability backend.

Anamnesis is not sampled. It is full-fidelity. A single flow in a single second can generate hundreds of events. The observability backend is expected to handle this volume (Datadog, New Relic, Grafana Loki et al. all do).

## DOOM: Computational Generality in the Kernel

DOOM is a proof-of-concept that proves Unheaded is not limited to packet processing. DOOM implements a subset of LISP and executes arbitrary programs compiled to BPF bytecode, running entirely in kernel space within BPF maps.

The demonstration: We implemented a classic "Towers of Hanoi" puzzle solver in LISP, compiled it to BPF, and ran it inside Unheaded's kernel infrastructure. The puzzle solver runs on every packet that matches a certain Sophia dictionary entry, executing the solver steps entirely in kernel space and recording results to Anamnesis.

Why this matters: It proves that Unheaded's BPF map layer is a general-purpose compute substrate. The infrastructure is not limited to packet forwarding. It can execute arbitrary logic. This opens the door to network-level machine learning, anomaly detection, and cryptographic operations, all without userspace context switches.

## The AI "Head": First Headful Application

In February 2026, we deployed the first "headful" application in Unheaded armor: a production AI model stack consisting of:

- **vLLM**: Vector Large Language Model engine, handling 100s of concurrent inference requests
- **DeepSeek-R1**: 671B-parameter reasoning model for deep technical analysis
- **Qwen 2.5 Coder**: Fine-tuned code generation and refactoring model

This AI stack runs inside Unheaded armor. It does not manage its own security, observability, networking, or service discovery. These are all provided by the armor layer via Monad, Sophia, Wotan, and Anamnesis. The application code is 100% focused on inference, tokenization, and beam search.

Result: A production-grade AI system with:
- Full packet-level observability (every inference is recordable to the event stream)
- Kernel-space security enforcement (Shield WAF rules apply to all traffic)
- Sub-microsecond policy enforcement (Port Authority policies control which models can be queried by which clients)
- Automatic failover and load balancing (Sophia dictionary + Wotan flow state)
- Zero infrastructure code in the application layer

This is the proof of concept that Unheaded works for real applications. The armor is invisible. The application is simple. The infrastructure is auditable.

## Anti-Lock-In: Six + Eight + Four

Unheaded is intentionally designed to avoid vendor lock-in at every layer.

**IaC Backends (6)**:
- Terraform (AWS, Azure, GCP, Bare Metal via Helm provider)
- Pulumi (Python, Go, TypeScript)
- CloudFormation (AWS native)
- Helm (Kubernetes native)
- Kustomize (Kubernetes declarative patches)
- Jsonnet (Google's configuration language)

Choose your IaC tool. Unheaded provisions identically.

**Observability Backends (8)**:
- Datadog (APM + Logs + Metrics)
- New Relic (full-stack observability)
- Grafana Loki (logs only, open-source)
- ELK Stack (Elasticsearch + Logstash + Kibana)
- Splunk (enterprise logging and SIEM)
- Honeycomb (event-driven observability)
- Lightstep (distributed tracing)
- AWS CloudWatch (managed AWS logging)

Anamnesis adapters make switching between them a configuration change, not a code change.

**Container Runtimes (4)**:
- containerd (industry standard, used by Kubernetes)
- Docker (ubiquitous, full runtime)
- Podman (daemonless, rootless, RedHat maintained)
- CRI-O (minimal, Kubernetes-focused)

Port Authority and Wotan work identically across all four. No optimization for one at the expense of another.

## The Internet-Drafts: Experimental Track

Unheaded is documented in three IETF Internet-Drafts on the Experimental track:

1. **draft-unheaded-foundation-04**: Core protocol design, Monad register, Sophia dictionaries, Wotan memory model, Inverse Mask, Kingdom Mode.
2. **draft-unheaded-sophia-dictionary-01**: Detailed specification of the dictionary data structure, lookup algorithms, update semantics, and BPF implementation.
3. **draft-unheaded-wotan-memory-01**: Per-flow memory model, ephemeral ring buffers, state transition rules, garbage collection semantics.

These drafts are living documents. They are updated as the implementation evolves. They are the source of truth for the protocol. Code divergence from the drafts is considered a bug.

## Current State: Production at Scale

**Code**:
- 260,000 lines of production Go and Rust
- 203,000 lines of test code
- Nine eBPF programs totaling 8,400 lines of kernel code
- Full test coverage of the protocol layer (99.2%)

**Infrastructure**:
- 25 microservices deployed in production
- Kubernetes clusters (tested on GKE, EKS, AKS, bare-metal K3s)
- Bare-metal deployments (Intel Xeon, AWS EC2, even gaming desktops)
- One unified Helm chart for all deployment topologies

**Deployment Targets**:
- Cloud: AWS, Azure, GCP, DigitalOcean
- On-premises: bare-metal Kubernetes, systemd-managed services
- Gaming: PC hardware with Linux kernel 6.0+

## Near-Term Roadmap: Public Demo and Conference Season

**March 2026**: Bare-metal deployment guide (detailed walk-through on Intel NUC hardware)

**April 2026**: Public GitHub release of the core architecture and protocol specifications

**May–June 2026**: Conference talks: KubeCon EU, eBPF Summit, IETF 116 (IPPM working group)

**July 2026**: Production SLA commitments and support contracts

**Q3 2026**: Integration with major cloud providers (native Unheaded support in AWS, Azure, GCP management consoles)

## Why This Matters

Modern infrastructure is bloated. Applications carry sidecars, proxies, API gateways, and observability agents as part of their deployment footprint. Security policies are scattered across network ACLs, service meshes, iptables rules, and application code.

Unheaded inverts this model. The application is simple. The infrastructure is a unified, auditable, protocol-driven layer.

This is the infrastructure for the next decade: invisible, swappable, observable, and fast.

The armor is ready. The head can focus on what it does best.


