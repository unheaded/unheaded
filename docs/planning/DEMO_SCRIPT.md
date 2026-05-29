# Unheaded Alpha Demo Video Script

**Duration:** 4 minutes
**Audience:** Technical founders, engineering leaders, DevOps/SRE teams
**Tone:** Confident, precise, technically grounded
**Last Updated:** February 8, 2026

---

## COLD OPEN (0:00 - 0:25)

**[VISUAL: Dark screen. A single gold particle appears, then multiplies into a constellation. The Unheaded logo fades in over the particle field.]**

**NARRATOR:**

> Every SaaS company builds the same infrastructure. Load balancers. Service meshes. Secrets management. Observability. Container orchestration. Deployment pipelines.
>
> Month after month. Sprint after sprint. The same yak, shaved a thousand times.
>
> What if you could skip all of that?

**[VISUAL: Particle field coalesces into the Unheaded dashboard. The tagline appears: "Configuration management automation platform."]**

---

## ACT 1: WHAT IS UNHEADED (0:25 - 1:00)

**[VISUAL: Clean architecture diagram animates in, layer by layer -- six layers from bare metal to UI.]**

**NARRATOR:**

> Unheaded is a configuration management automation platform. You bring your application -- the head. We provide everything else.
>
> That means: an API gateway with HTTP/3 and TLS termination. A service mesh with circuit breakers and mutual TLS. A load balancer with Maglev consistent hashing. A secrets manager with envelope encryption and automatic rotation. eBPF-based observability that traces every packet from the network interface to your application layer. Immutable containers with seccomp profiles, capability bounding, and read-only filesystems. A deployment pipeline with canary, blue-green, and rolling strategies. All of it declarative. All of it version-controlled. All of it hardened by default.

**[VISUAL: Each component highlights as it is named -- Shield (WAF), service mesh, load balancer, control plane, observability layer, deployment pipeline -- forming the full architecture diagram.]**

> Six layers. Twenty-three services. One command to deploy. Your app goes from repository to production in hours.

---

## ACT 2: THE DASHBOARD -- REAL-TIME PACKET FLOW (1:00 - 1:50)

**[VISUAL: Cut to the live Unheaded Dashboard. The particle canvas background drifts behind the UI. The "Real-time Packet Flow" section is front and center.]**

**NARRATOR:**

> This is the Unheaded dashboard. What you are seeing is live network traffic between our services, visualized in real time.

**[VISUAL: Zoom into the packet flow canvas. Gold dots travel along connection lines between labeled service nodes: Gateway, Wotan, Timeguru, Captain, Architect, Micromanager, Monad, Sophia. A packet enters at the Gateway node and traces a path downward.]**

> Every dot is a real packet. Every packet carries a 128-bit W3C-compatible trace ID injected at the XDP layer before user space even sees it. Watch this one -- it enters through the Gateway, passes through Wotan, the message bus, and arrives at Timeguru, our timeline service. Six hops. Forty-seven milliseconds. Every hop logged, correlated, and displayed here.

**[VISUAL: Click the "Demo Packet" quick action button. A green packet appears at Gateway and travels through the mesh. The trace ID appears in gold text beside it.]**

> The connection status indicator in the nav bar shows we are live on WebSocket. Metrics refresh every five seconds from the dashboard backend. Service health, request counts, latency histograms, Wotan message throughput -- all streaming in.

**[VISUAL: Pan down to the System Metrics section. Cards show request counts, latency percentiles, uptime, Wotan messages published. Numbers tick upward in real time.]**

---

## ACT 3: THE META MOMENT (1:50 - 2:30)

**[VISUAL: Click the "Kanban Board" link in the nav. The Kanban app loads with its own particle background. The header reads: "Unheaded Alpha - Built by Unheaded". A green "Live" indicator pulses.]**

**NARRATOR:**

> Now here is where it gets interesting. This is our Kanban board. It shows our own development progress -- pulled live from our Timeguru service, which parses our timeline markdown, converts it to JSON, and serves it over REST. The same infrastructure. The same services. The same eBPF tracing.

**[VISUAL: Three columns are visible -- TODO, IN PROGRESS, DONE. Task cards populate with real data: "Wotan Phase 1" in DONE, "eBPF Foundation" in IN PROGRESS, items with priority badges and progress bars.]**

> We call this The Meta Moment. Unheaded is tracking its own development, on its own infrastructure, traced by its own eBPF programs, managed by its own control plane. Self-hosting is proof, not marketing.

**[VISUAL: Click a task card. The detail modal opens, showing title, description, status, priority, type, and owner. Edit and Delete buttons visible.]**

> Every request you just saw -- the page load, the timeline fetch, the WebSocket connection -- was traced end to end. If we switch back to the dashboard...

**[VISUAL: Navigate back to the dashboard. A cluster of new packets appear in the flow visualization, tracing the path: Gateway to Kanban App to Timeguru to Wotan and back.]**

> ...there they are. The packets from the Kanban load. From browser to gateway to service to message bus and back. Full circle. The infrastructure observing itself.

---

## ACT 4: eBPF TRACING (2:30 - 3:15)

**[VISUAL: Split screen. Left side shows a terminal with eBPF source code (Rust). Right side shows the packet flow visualization.]**

**NARRATOR:**

> Let me show you what is happening under the hood. Our eBPF layer runs three programs in kernel space, all written in Rust using the Aya framework for memory safety.

**[VISUAL: Highlight each program as named. Show snippets of the actual Rust code.]**

> First, the Packet Marker. An XDP program that attaches to the network interface at the earliest possible hook point. Before the kernel networking stack even processes the packet, we extract or inject a 128-bit trace ID. Every flow gets a five-tuple key: source IP, destination IP, source port, destination port, protocol. Flow state is tracked in an eBPF hash map. Events are sent to user space through a ring buffer -- zero copy, zero allocation.

**[VISUAL: Show the FlowKey and TraceId struct definitions from common/src/lib.rs.]**

> Second, the Flow Tracker. A TC program that monitors bidirectional connections, tracks TCP state transitions, and expires stale flows via LRU eviction.
>
> Third, the Latency Probe. Kprobes on tcp_sendmsg and tcp_recvmsg that measure round-trip time at the kernel level, not the application level. No sampling. No approximation. Every packet, measured.

**[VISUAL: Show a trace event flowing from eBPF (kernel) through the trace-collector (Rust) to Wotan (Go) to the dashboard WebSocket.]**

> The trace-collector, also written in Rust, reads from the ring buffer and publishes events to Wotan. From there, the dashboard backend streams them to your browser. Layer 2 to Layer 7. Kernel to canvas. That is the depth of visibility Unheaded provides out of the box.

---

## ACT 5: THE SERVICE MESH AND WOTAN (3:15 - 3:45)

**[VISUAL: Architecture diagram focusing on Layer 3 -- Wotan at the center, with gRPC streams radiating out to all services.]**

**NARRATOR:**

> At the center of the architecture is Wotan, our custom message bus. Eleven thousand lines of Go. gRPC bidirectional streaming. Pub/sub with topic-based routing. A ring buffer for high-throughput message storage. Rate limiting. Circuit breakers. Backpressure handling.

**[VISUAL: Show the Fae Chamber Contracts topic list: tasks.created, timeline.updates, alerts.critical, state.drift.detected, and others scrolling by.]**

> Every service in the mesh communicates through Wotan. Timeguru publishes timeline updates. Micromanager subscribes to task events. Captain monitors strategic alerts. Architect tracks design decisions. No direct service-to-service calls. Every message carries a trace ID. Every message is observable.
>
> The mesh layer adds service discovery, circuit breakers with configurable thresholds, and mutual TLS between containers. Default deny networking. Explicit allow only.

---

## ACT 6: SECURITY — ZERO DATA ACCESS (3:45 - 4:10)

**[VISUAL: The Moat security boundary diagram. Zero Trust zones highlighted. Container hardening callouts appear one by one.]**

**NARRATOR:**

> Security is not a feature we added. It is the foundation we built on. The core invariant: zero user data access. Not by policy. By architecture.
>
> User applications run in isolated NixOS containers with read-only filesystems, no privilege escalation, restricted system calls via seccomp, and minimal Linux capabilities. Network policies default to deny-all. Secrets are encrypted with age, envelope-encrypted with separate key encryption keys and data encryption keys, mounted as files -- never in environment variables, never in logs, never in code.

**[VISUAL: Show NixOS container config snippet with ProtectSystem, CapabilityBoundingSet, SystemCallFilter highlighted.]**

> Every PR is evaluated against three questions: Does this access user data? Does this weaken isolation? Does this skip hardening? If the answer to any of them is yes, it is blocked.
>
> And eBPF gives us something no other platform offers: kernel-level audit trails on every packet that touches your infrastructure, without touching your data.

---

## CLOSING (4:10 - 4:30)

**[VISUAL: Return to the dashboard. The packet flow visualization continues. Pull back to show the full platform -- dashboard, Kanban, packet flow, metrics -- all running together.]**

**NARRATOR:**

> You just watched Unheaded run Unheaded. The dashboard. The Kanban board. The eBPF tracing. The service mesh. The container orchestration. All of it running on the platform it was built to demonstrate.
>
> Configuration management automation platform.

**[VISUAL: The particle field expands outward. The Unheaded logo centers. Below it: unheaded.com and hello@unheaded.com.]**

> You bring the application. We provide the infrastructure.
>
> Visit unheaded.com.

**[VISUAL: Fade to black. The particle field lingers for one beat, then dissolves.]**

---

## PRODUCTION NOTES

### Screen Recording Checklist

- [ ] Dashboard running at localhost:8080 with WebSocket connected
- [ ] Kanban app loaded at /kanban with live timeline data
- [ ] Packet flow visualization active with demo packets
- [ ] All eight services showing healthy in metrics grid
- [ ] Terminal with eBPF Rust source ready for split-screen
- [ ] Architecture diagrams preloaded (ARCHITECTURE.md, SYSTEM_DIAGRAM.md)
- [ ] Browser zoom at 110% for readability on video

### Visual Assets Needed

- Unheaded logo (gold on dark, SVG)
- Architecture layer diagram (animated, Layer 0-5)
- Moat security boundary diagram
- Fae Chamber topic routing diagram

### Timing Breakdown

| Section | Start | Duration | Key Visual |
|---------|-------|----------|------------|
| Cold Open | 0:00 | 25s | Particle field, logo |
| What is Unheaded | 0:25 | 35s | Architecture layers, component diagram |
| Dashboard | 1:00 | 50s | Packet flow, metrics grid |
| Meta Moment | 1:50 | 40s | Kanban board, recursive proof |
| eBPF Tracing | 2:30 | 45s | Rust code, kernel-to-canvas flow |
| Service Mesh | 3:15 | 30s | Wotan hub, topic routing |
| Security | 3:45 | 25s | NixOS hardening, zero trust |
| Closing | 4:10 | 20s | Full dashboard, CTA |
| **Total** | | **~4:30** | |

### Music / Sound Design

- Ambient electronic, low and steady -- building tension through the cold open
- Subtle pulse during packet flow visualization (synced to packet movement if possible)
- Clean and confident through the architecture and security sections
- Crescendo at "The Meta Moment" reveal
- Resolves to calm at the closing CTA

---

**Written by:** Unheaded Team
**For:** Alpha Demo Day — February 8, 2026
