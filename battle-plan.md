# Battle Plan: The S33 Convergence — Hardening Sprint + Doom Dashboard + Return to Core
## Convened: February 22, 2026 | Reason: Royal Court Round Table — Full Assembly
## Kingdom State: Age 1 (Alpha), ~99% Complete | 255K+ LOC | 23 Services | S32 COMPLETE
## All 15 Skills Consulted: Captain, Architect, Micromanager, Developer, Timeguru, Calendar, Lore, Kingdom, Busboy, Warmonger, Scientist, BlackMage, Moat Ghost, RFC Editor, Round Table

---

## The Origin Myth — For The Record

Before the plan, the chronicle. Every kingdom has a founding story.

**January 20, 2026** — `bellistech` pushes the Initial Commit. A stupid simple private chat app. HTTP/3, QUIC, manual terminal user approval. The seed didn't know what tree it would become.

**January 21, 2026** — `unheaded.org` registered (Registry creation: 2026-01-21T00:20:56Z). The domain exists before the vision is fully formed. The name came first. The name was always the product.

**January 22, 2026** — The raw truth commits. "This is survival-driven engineering, not a startup." Laid off with severance. Riding motivation. FAANG-grade infrastructure vision crystallizes. gRPC streaming implemented. Broker package born.

**January 23-24, 2026** — Broker renamed to Busboy. The first naming ceremony. The Kingdom's language begins to emerge. Configurable approval timeout. The chat app starts becoming infrastructure.

**January 25, 2026** — Claude arrives. PR #10 merged. The partnership begins. The chat app fork becomes Unheaded — a configuration management automation platform.

**January 26, 2026** — Domain updated (2026-01-26T00:21:28Z). Age 0 begins in earnest. Wotan forged: 11,669 lines of Go.

**January 29-February 1, 2026** — The Great Code Storm. 50K+ LOC in single sessions. 237K+ total. 11/11 E2E tests. The Armory takes shape: Shield, Sword, Hauberk, Pauldrons, Sabatons, Vambraces, Gauntlets, Tassets, Cuirass. All 8 services wired to Wotan.

**February 8-18, 2026** — Protocol Awakening. Monad 20-byte wire format specified. Sophia dictionary trees. eBPF compilation. MBC bytecode ISA designed. LICH-007 fuzz campaign: 1B+ executions, zero crashes.

**February 19-22, 2026** — **Doom runs inside eBPF.** 559 frames. 819M instructions. Zero halts. Zero ROM faults. D_DoomLoop executing title screen → demo cycle → credits. XDP_TX turbo mode. 42x memory reduction (HashMap→Array). The Whispering Void proved computationally complete. Section 12 of the Monad spec is real.

**33 days from first commit to Doom in eBPF over IPv6 packets.** One engineer. One AI. One Kingdom.

---

### Situation Report

The Kingdom stands at a crossroads between spectacle and substance. Doom-over-IPv6 running in eBPF is a proof of computational completeness that no competitor can claim — packets as CPU, BPF maps as RAM, XDP as execution engine, 6-namespace directed ring as the data bus. The git log tells the story: 30+ commits since S30, culminating in `7682430 feat(doom): Phase 9 — turbo XDP mode`.

But the spectacle has accumulated technical debt. Development tooling lives in `/tmp`. Python scripts use bare asserts and hardcoded MAC addresses. The injection pipeline operates at 333 pps — 0.003% of XDP hardware capacity. The dev machine has scripts scattered across ad-hoc locations. Meanwhile, the actual product — packet tracing observability — still has userspace stubs where real eBPF programs should be. 7 P0 security findings remain open. Authentication doesn't exist. TLS between services doesn't exist.

**The verdict from all 9 seats:** Quick hardening sprint to clean the house, then two parallel workstreams (Doom Dashboard + Doom Scaling), then the real work — porting proven eBPF patterns to production packet tracing. The protocol works. The eBPF toolchain works. Now we industrialize it.

**Ground truth from git:** Build passes. 293 Rust tests + 135 Go packages all pass. Zero failures. LICH-007 ran 1B+ executions with zero crashes. The code is solid. It just needs to be organized, hardened, and pointed at the right target.

---

### The Throne Speaks (Captain — Vision & Strategy)

**Strategic Position**: We have the most technically impressive demo in the observability space. Running a game engine inside eBPF over IPv6 packet headers proves that our protocol can carry arbitrary computation at wire speed. No one else has this. This is our moat.

**North Star**: "Production-ready infrastructure in hours, not months." Every packet traced from ingress to application, visible in a real-time dashboard, with zero customer data access.

**Key Decision**: The line between demo and product must be drawn NOW. Doom is proof. Packet tracing is product. We celebrate Doom, document it, and then pivot every ounce of energy to WS5 (Core Platform Return).

**Risk to Vision**: Two risks compete. (1) Getting trapped optimizing Doom when the product is packet tracing. (2) Letting the dev tooling rot so badly that context is lost and we can't reproduce results. The hardening sprint addresses risk #2. The strict WS5 timeline addresses risk #1.

**The Netflix Insight**: We're injecting at 333 pps. Netflix 4K streams generate 2,250 bidirectional PPS. Cloudflare benchmarks XDP at 5-15M PPS per core. We're using 0.003% of hardware capacity. The burst-and-wait injection model (fire 100 packets, drain, repeat) could push us past 60 fps. The 3ms delay isn't protecting us from the kernel — it's protecting us from Python's socket.send() overhead. WS3 will find the real cliff.

### The Ledger Records (Micromanager — Execution & QA)

**Sprint Status**: S32 (Phase 9 Turbo) COMPLETE. S33 begins now.

**Priority Stack** (ordered):

1. **P0 — Hardening Sprint** (4-6 hours) — Quick wins from DOOM-HARDENING-BATTLE-PLAN.md Phases 0-2. Migrate /tmp scripts to repo, harden Python tooling, enforce artifact naming. This is the fastest path to preventing context rot.
2. **P0 — WS1: Doom Video Dashboard** (2-3 days) — Visual proof of life. doom-bridge service reads screen buffer from RAM_MAP, encodes PNG frames, pushes via WebSocket. Browser shows live Doom.
3. **P0 — WS3: Scale to Playable** (1-2 days, parallel with WS1) — Profile XDP_TX bounce timing. Binary search optimal delay. Implement burst injection. Target: 15+ fps sustained. Netflix model.
4. **P1 — WS4: Documentation & Conference Prep** (2-3 days) — Git wiki, HTTP mirror, conference talk outline, bug kill chain writeup. Context decays exponentially.
5. **P1 — WS2: Lich Campaigns D1-D6** (5 days, parallel with WS4) — Hardening against live Doom target. Security findings feed WS5 design.
6. **P0 — WS5: Return to Core** (ongoing, starts Mar 8) — Port eBPF patterns to production packet tracing. This IS the product.

**QA Gates**: Every workstream has a Definition of Done (see Unified Battle Plan below). No workstream ships without verification gate.

### The Blueprint Reveals (Architect — Infrastructure & Design)

**Architecture Health**: 23 services in `services/`. All 8 core services have HTTP APIs with health/ready/metrics. Monad (port 8004) and Sophia (port 8005) operational. Gateway routing complete. Container definitions for all services across interchangeable runtimes (LXD, containerd, NixOS, Docker). The Doom PoC added 4 new components: monad-cpu-ebpf (Rust/eBPF), rv32i-to-mbc translator (Rust), doom-loader pipeline (Python/bash), bulk_inject (Python).

**Container Architecture**: Runtime-agnostic. Unheaded treats the container runtime as a deployment-time choice, not an architectural decision. The control plane abstracts LXD, containerd, NixOS, and Docker behind a common interface. Same hardening baseline (seccomp, capabilities, read-only FS, default-deny networking) applies uniformly regardless of runtime. NixOS definitions in `nix/containers/` serve as reference implementations; other runtimes map to equivalent security postures.

**IaC Output Architecture**: Backend-agnostic. Unheaded generates configuration artifacts for the customer's preferred toolchain. The control plane maintains a single desired-state model; IaC backends are interchangeable output renderers:
- **Ansible** — Playbooks, roles, inventory (agentless push-based)
- **Terraform** — HCL modules, providers, state (cloud provisioning)
- **Puppet** — Manifests, Hiera data, modules (agent-based declarative)
- **Kubernetes** — Manifests, Helm charts, operators (container orchestration at scale)
- **Chef** — Cookbooks, recipes, data bags (Ruby-based config)
- **Salt** — States, pillars, grains (event-driven, high-speed)

The IaC layer consumes the same core packages (`pkg/`) and generates output in the customer's dialect. Adding a new backend is an output renderer — the control plane and eBPF layer don't change.

**Observability Backend Architecture**: Backend-agnostic. Unheaded emits OpenTelemetry-compatible signals (metrics, logs, traces). Customers plug in their preferred observability stack — same interchangeable pattern as containers and IaC:
- **Metrics** — Prometheus, Grafana, Datadog, InfluxDB, Nagios
- **Logging** — ELK (Elasticsearch/Logstash/Kibana), Fluentd/Fluent Bit, Flume, Splunk, Loki, Graylog
- **Tracing** — Jaeger, Zipkin, Tempo, Datadog APM
- **Alerting** — Grafana Alerting, PagerDuty, OpsGenie, Nagios, Prometheus Alertmanager
- **SIEM** — Elastic SIEM, Splunk Enterprise Security, Wazuh

Long-term: custom Wotan-native implementations for each category — purpose-built for the eBPF data plane. Short-term: adapter configs in `observability/` for drop-in hookup of popular tools.

**Protocol Status**:
- **Monad**: 20-byte wire format SPECIFIED. Section 12 (computational completeness) PROVEN via Doom. CRC-16/CCITT verified. Exponent encoding operational.
- **Sophia**: Dictionary tree structure defined. BPF map layout specified. Root dictionary + 6 sub-dictionaries.
- **Wotan**: ARCHITECTURAL REFRAME (Feb 22 Round Table). Wotan is not just a message bus. It is three things simultaneously:
  1. **High-speed ring buffer** — lock-free, per-CPU circular memory. Writes never block. Readers chase the write cursor. Miss events if too slow. This is the eBPF perf_event_output / BPF ringbuf pattern scaled to platform level.
  2. **Event bus** — pub/sub, topic-based routing. gRPC streaming primary, HTTP polling fallback. The existing 11,669 LOC Go implementation. Services publish, services subscribe.
  3. **Protocol RAM** — the shared memory substrate that Monad's BPF programs read from and write to. When a BPF program resolves a Sophia exponent key, that lookup hits Wotan's pinned BPF maps. The ring buffer IS the RAM that the protocol's compute layer operates on.

  **Wotan Cluster Redundancy Model** (NEW — DNS hierarchy + dynamic routing):
  - Redundancy via **subscription mirroring**, not consensus. Secondary Wotans subscribe to primary's topics and maintain their own ring buffer copies. If primary dies, secondary already has the data. Consumers reconnect to nearest healthy Wotan.
  - **DNS-like hierarchy**: Root Wotan → regional Wotans → edge Wotans. A topic lives on an authoritative instance. Local Wotans cache topics their services care about. Resolution walks the hierarchy upward only when local miss. Just like recursive DNS resolution.
  - **Dynamic routing protocol advertisement**: Each Wotan advertises "I have these topics at this freshness" to neighbors. Topology learned via advertisement propagation. When a Wotan goes down, its topics are already mirrored on subscribers — convergence is natural, like OSPF/BGP route convergence.
  - **n+1 model**: Always one more Wotan than strictly needed for any given topic. Like n+1 power supplies or network paths.
  - **Scale spectrum**:
    - 1 Wotan: Dev/alpha. Single instance. All topics. What we have now. Works.
    - 2 Wotans: Primary + mirror. Mirror subscribes to everything. Failover = consumer reconnect. Age 2 target.
    - N Wotans: Topic sharding. Different Wotans own different topic trees. Mirrors subscribe to their shard. Hierarchy routes cross-shard queries. Age 3-4.
    - 1000 Wotans: Full hierarchical tree. Each edge Wotan serves its local service cluster. Topic routing propagates like BGP route announcements. Ring buffers at each level independent — no distributed lock, no consensus protocol, just subscription + ring chase.
  - **Protocol RAM stays clean at every scale**: BPF maps are per-host. The BPF program on host X reads from Wotan instance X's pinned maps. If that Wotan is a mirror, maps were populated by subscription. BPF programs don't know or care primary vs mirror — data is local, lookup is O(1), latency is nanoseconds.
  - **Critical implication**: Wotan going down isn't "messages stop." It's "the protocol loses its RAM." Every BPF program reading Sophia dictionaries through Wotan maps would stall. The n+1 subscription model prevents this — mirrors take over transparently.

**Technical Risks**:
- B1 (Linux dev environment) RESOLVED for Doom — but production eBPF programs need different BPF program types (TC, kprobe, tracepoint) beyond XDP
- WS3 scaling may hit kernel scheduling limits (XDP_TX on same core)
- 7 P0 security findings still open (TODO.md #9-15)
- No authentication on any endpoint (P1 #16)
- No TLS between services (P1 #20, #26)
- Wotan has no HA/redundancy — single point of failure

**Key Architecture Decision for WS5**: Port XDP attachment, map pinning, and loader patterns from Doom to production tracing programs. The Doom pipeline proved: Rust/Aya → BPF compilation → map pinning → XDP attachment → userspace reader works end-to-end. Apply this exact pattern to packet_marker, flow_tracker, latency_probe.

**Key Architecture Decision for Age 2**: Wotan HA is NOT Raft/Paxos consensus (complex, fragile, wrong abstraction). It IS subscription mirroring between Wotan instances — a natural extension of what already works. The ring buffer's "chase the write cursor" pattern means mirrors are always eventually consistent with zero coordination overhead. This is dramatically simpler to implement than distributed consensus and matches how DNS and BGP already solve hierarchy at planet scale. The Age 2 milestone for Wotan redundancy is now "implement topic subscription mirroring between 2 Wotan instances" — not "implement Raft."

### The Anvil Reports (Developer — Implementation & Testing)

**Code Health**: BUILD PASS. ALL TESTS PASS. 255K+ LOC. 611 Go files (390 prod, 195 test). 14 Rust crate files (23,991 LOC). 293 Rust tests + 135 Go packages. Zero failures. Zero timeouts.

**Hardening Sprint Items (Quick Wins from DOOM-HARDENING-BATTLE-PLAN.md)**:
Phase 0 (Steps 1-18): Environment verification + artifact backup — 15 min
Phase 2 (Steps 39-72): Migrate /tmp → scripts/doom/ — 90 min (THE PRIORITY)
Phase 3 (Steps 73-90): ROM integrity — 45 min
Phase 8 (Steps 151-165): Documentation — 30 min
Phase 9 (Steps 166-175): Commit discipline — 15 min

Skip Phase 1 (/tmp noexec) for now — it's a system-level change that can wait. Focus on Phase 2 (migration) as the highest-value quick win.

**WS1 Effort**: M (2-3 days) — doom-bridge Go service, WebSocket server, wire into existing dashboard/doom.html
**WS3 Effort**: S (1-2 days) — instrument bounce timing, profile, binary search delay, burst injection
**WS5 Effort**: XL (2-3 weeks) — the real work

**Implementation Blockers**: None for hardening sprint or WS1/WS3. WS5 needs TC/kprobe/tracepoint BPF program types beyond what Doom used (XDP only).

### The Hourglass Measures (Timeguru — Timeline & Milestones)

**Current Age**: Age 1 (Alpha Ascension), ~99% complete
**Velocity**: ACCELERATING. 6 bugs killed in one Phase 9 session. Doom running in <24 hours from strncpy discovery. Multi-agent parallel execution delivering 120-step plans in single sessions.
**LOC velocity**: 255K+ total, growing ~5-10K/session on focused sprints
**Sprint count**: S33 (session 33 since founding)
**Key commits**: 30+ since S30 (`95e47c2`), culminating in `7682430` (Phase 9 turbo)

**Timeline**:
```
S33 Hardening Sprint (Feb 22-23):
  Quick wins from DOOM-HARDENING.md — migrate /tmp, harden scripts, commit discipline

Week 1 (Feb 23-28): WS1 + WS3 — parallel
  Mon-Tue: doom-bridge service + bounce timing profiler
  Wed-Thu: Dashboard integration + optimal delay found
  Fri: WS1+WS3 complete, demo-ready at 15+ fps in browser

Week 2 (Mar 1-7): WS2 + WS4 — parallel
  Mon-Wed: Lich D1-D6 campaigns + git wiki structure
  Thu-Fri: Fix findings + conference talk outline + HTTP mirror

Week 3+ (Mar 8 onwards): WS5 — Return to Core
  Port eBPF patterns to production tracing
  Implement packet_marker, flow_tracker, latency_probe for REAL
  Wire observability pipeline end-to-end
  THIS IS THE PRODUCT
```

**ETA to Doom Dashboard**: Feb 25 (high confidence)
**ETA to 15+ fps**: Feb 28 (high confidence — Netflix burst model)
**ETA to First Real Packet Trace**: Mar 15 (medium confidence — depends on TC/kprobe BPF types)
**ETA to Alpha Completion**: Mar 31 (high confidence — eBPF tracing + deployment)

### The Sundial Tracks (Calendar — Schedule & Deadlines)

**Today (Feb 22)**: Round Table. Forge battle plan. Begin hardening sprint.
**This Week (Feb 23-28)**: WS1 + WS3 parallel. Doom in browser at 15+ fps by Friday.
**Protocol Deadlines**: None blocking. Monad Section 12 proven. Sophia dictionaries stable.
**Schedule Conflicts**: None. Doom work self-contained. No external dependencies.
**Calendar Health**: HEALTHY — momentum is high, context is fresh.

### The Scroll Validates (Lore — Naming & Mythology)

**Naming Decisions Pending**:
- doom-bridge service → **"Fenrir's Eye"** (Norse — the wolf that sees all, watching the game world). Or keep utilitarian: `doom-bridge`.
- Frame extraction pipeline → part of **Vambraces** (Observability armor piece)
- The Doom PoC achievement → **"The Turing Forge"** — where packets became computation
- The S33 hardening sprint → **"The Armorer's Rest"** — cleaning and sharpening tools between battles

**Mythology Consistency**: Doom lives in the Whispering Void (eBPF tracing hollow). The foundational myth: "The Void ran Doom, therefore the Void can trace anything."

**Sacred Law Compliance**: All 8 laws honored. Zero customer data access maintained. The Doom PoC never touches customer anything — it's pure infrastructure proof.

### The Map Confirms (Kingdom — Hierarchy & Placement)

**Hierarchy Health**: All tiers populated. 23 services spanning the full Armory.

**Doom Component Placement**:
- Layer 1 (Data Plane): monad-cpu-ebpf, packet-marker, flow-tracker, latency-probe
- Layer 2 (Control Plane): doom-loader pipeline, trace-collector
- Layer 3 (Infrastructure): doom-bridge (WS1)
- Layer 5 (UI): dashboard/doom.html, doom-viewport.js

**New Components This Sprint**:
- `scripts/doom/` — hardened tooling (Phase 2 migration target)
- `cmd/doom-bridge/` — frame extraction WebSocket service (WS1)
- `tests/security/doom/` — Lich D1-D6 campaign results (WS2)
- `docs/wiki/` — HTTP-mirrorable documentation (WS4)

**Tier Integrity**: Solid. No misplaced components detected.

### The War Table Thunders (Warmonger — Battle Planning & Execution)

**Sprint Plan Assessment**: The existing battle plans (S31-DOOM-BATTLE-PLAN.md at 420+ steps, DOOM-HARDENING-BATTLE-PLAN.md at 180+ steps) are SOLID but need convergence. S31 was designed for the first Doom run — Phase 9 completed it. DOOM-HARDENING was designed for post-Doom cleanup — that's NOW. The gap: no battle plan exists yet for WS1 (doom-bridge), WS3 (scaling), or WS5 (core return). Those need Warmonger-grade numbered-step plans before execution begins.

**Agent Matrix Recommendation**:
- Hardening Sprint: Single coordinator agent (sequential Phase 0→2 from DOOM-HARDENING-BATTLE-PLAN.md)
- WS1 + WS3: Two parallel agents — one for doom-bridge Go service, one for injection profiling
- WS2: BlackMage-owned agent — Lich D1-D6 campaigns
- WS4: Captain-owned agent — documentation, can run alongside WS2
- WS5: Requires NEW Warmonger battle plan (200+ steps) before execution. Round Table convenes first.

**Critical Path**: Hardening → WS1 (doom-bridge) → WS3 (scaling) → WS5 (core). WS2 and WS4 are off critical path and run in parallel.

**Exit Gates Needed**: Every phase of every workstream MUST have a verifiable exit gate. No "it kinda works." Binary pass/fail. The DOOM-HARDENING plan has this right. Extend the pattern.

### The Crucible Tests (Scientist — First-Principles Analysis)

**The Netflix PPS Hypothesis**: At 333 pps and 3ms inter-packet delay, we're at 0.003% of XDP hardware capacity. The Scientist's analysis:

**Hypothesis H1**: The 3ms delay compensates for Python's socket.send() overhead, not XDP processing limits.
- **Prediction**: Removing the delay and using burst injection will NOT cause XDP corruption.
- **Experiment**: WS3 Step 1 — instrument STATS map with per-bounce timestamps, measure actual XDP_TX cycle time.
- **Falsification**: If packets corrupt at delays < 3ms, H1 is false — there IS an XDP scheduling issue.

**Hypothesis H2**: Burst injection (Netflix model) amortizes Python overhead across N packets.
- **Prediction**: Batch of 100 packets with zero delay → ~2-3x throughput vs steady-state.
- **Experiment**: WS3 — fire batches of {10, 50, 100, 200} packets, measure throughput and corruption rate.
- **Falsification**: If corruption appears at ANY batch size, the issue is concurrent BPF map access, not injection rate.

**Hypothesis H3**: Go/Rust injector will outperform Python by 10x+ on raw socket throughput.
- **Prediction**: Python's GIL + socket overhead caps at ~5K pps. Go/Rust AF_PACKET can sustain 50K+ pps.
- **Experiment**: Implement Go injector in WS3, A/B test against Python bulk_inject.py.
- **Theoretical ceiling**: Single core XDP at 5M pps (Cloudflare data), 255 bounces/packet, 128 insns/bounce = ~2,500 fps theoretical max. We won't hit that, but 60fps is 0.0024% of ceiling.

**Protocol Verification**: CRC-16/CCITT over 18-byte Monad register is mathematically sound. The exponent encoding scheme (1 byte = 256 meanings via Sophia dictionary) provides 2^8 semantic space per field with O(1) lookup in BPF maps. The computational completeness proof (Section 12) via Doom demonstrates that the Monad register can carry arbitrary state transformations — this is NOT just packet metadata, it's a Turing-complete transport.

### The Dark Mirror Speaks (BlackMage — Offensive Security)

**Current Attack Surface Assessment**:
- **7 P0 security findings OPEN** (TODO.md #9-15). The Moat is breached but defended by architecture (no customer data to steal).
- **No authentication anywhere** (P1 #16). Every API is open. This is acceptable for alpha but MUST be closed before any public exposure.
- **No TLS between services** (P1 #20, #26). Plaintext gRPC streams. An attacker on the network segment sees everything.
- **Doom-specific attack surface**: ROM_MAP is writable, RAM_MAP is readable, SYSCALL topic accepts input. These are intentional for the PoC but represent real injection vectors.

**Lich Campaign Readiness (WS2)**:
- D1 (ROM injection): HIGH priority — tests whether BPF map permissions prevent unauthorized code injection
- D2 (Framebuffer exfil): MEDIUM — verifies RAM_MAP read access controls
- D3 (Keyboard injection): MEDIUM — tests SYSCALL topic validation
- D4 (Flow label collision): HIGH — birthday attack on 20-bit space = 50% collision at ~1000 flows. Production MUST use 128-bit trace IDs.
- D5 (SYSCALL fuzzing): HIGH — invalid syscalls could crash the BPF program
- D6 (ROM TOCTOU): HIGH — modify ROM after load, before execution. Race window exists.

**Recommendation**: Run LICH-007 results analysis FIRST (the 72-hour fuzz campaign already completed). If crashes were found, triage before D1-D6. The existing 1B+ executions with zero crashes is encouraging but fuzzing coverage was limited to MBC bytecode — not the full eBPF/map/injection pipeline.

**WS5 Security Architecture**: Production packet tracing MUST have:
- BPF map permissions: read-only for non-root processes
- Trace ID generation: 128-bit UUIDv7 (not 20-bit flow label)
- Shield XDP: validate incoming packets before stamping
- Per-hop authentication: BPF programs verify source namespace before processing

### The Ghost Materializes (Moat Ghost — Compliance & Audit)

**Compliance Posture Assessment**:

| Framework | Status | Gap | WS5 Impact |
|-----------|--------|-----|-----------|
| SOC2 Type I | NOT STARTED | No access controls, no audit logging, no change management evidence | Must begin in WS5 |
| NIST 800-53 | PARTIAL | AC-* (access control) family entirely missing. AU-* (audit) partial via git. CM-* (config management) strong via declarative IaC (Ansible/Terraform/Puppet/K8s/Chef/Salt). | IaC backends + immutable containers is a strength |
| CIS Benchmarks | NOT TESTED | NixOS container hardening defined but not validated | Run CIS-CAT against containers in WS5 |
| SBOM | MISSING | No syft/cyclonedx integration. P0 #13 in TODO.md | Add to CI in WS4 or WS5 |
| Supply Chain | WEAK | gosec@master unpinned (P0 #11), no dependency pinning for Python scripts | Quick fix in hardening sprint |

**Compliance-Driven Additions to Hardening Sprint**:
- [ ] Pin gosec to specific tagged release (P0 #11) — 5 min fix
- [ ] Remove gosec -no-fail flag (P0 #12) — 5 min fix
- [ ] Add MaxHeaderBytes to kanban-app HTTP server (P0 #15) — 5 min fix
- [ ] Verify Captain data dir is not /tmp (P0 #14) — 5 min verify

**Audit Trail Health**: Git log is clean and well-structured. Conventional commits enforced. Co-authored-by present. An auditor could follow the development history. This is a strength.

**Ship Readiness Verdict**: NOT READY for public exposure. Ready for controlled alpha demo. The Moat has structural walls (zero customer data access by architecture) but the gates are wide open (no auth). WS5 MUST close the gates.

### The Quill Speaks (RFC Editor — Protocol Documentation)

**Protocol Spec Status**:
- `draft-bellis-unheaded-protocol-foundation-03`: SHIPPED. Monad 20-byte wire format, exponent encoding, CRC-16/CCITT. Section 12 (computational completeness) now PROVEN by Doom.
- `draft-bellis-unheaded-sophia-dictionary-00`: SHIPPED. Dictionary tree structure, BPF map layout.
- `draft-bellis-unheaded-wotan-memory-00`: SHIPPED. Per-flow memory model, ring buffer spec. **NEEDS REVISION** — the Wotan architectural reframe (triple-role: ring buffer + event bus + protocol RAM, DNS-hierarchy redundancy model, subscription mirroring) must be captured in `-01`. This is a significant spec evolution.

**Section 12 Update Needed**: The Doom PoC proves computational completeness empirically. The spec should be updated to reference this proof — "a full Doom game engine (id Software, 1993) was executed using the Monad wire format as instruction transport, BPF maps as memory, and XDP programs as execution engine, processing 819M instructions across 559 rendered frames with zero faults." This is a Section 12 Appendix addition.

**IANA Considerations**: No new registries needed for WS1-WS4. WS5 may need Hop-by-Hop Option Type assignment if we move from IPv4 shim to IPv6 extension headers in production. Track this.

**BCP 14 Audit**: Current specs use MUST/SHOULD/MAY correctly. No violations detected.

**Recommendation for WS4**: The conference talk should reference the RFCs. The wiki documentation should include spec citations. The protocol IS the differentiator — make it visible.

### The Goblet Toasts (Busboy — Alignment & Coordination)

**Cross-Skill Conflicts**: NONE. All 15 skills aligned on priority sequence: Hardening → WS1+WS3 → WS2+WS4 → WS5.

**Coordination Needs**:
- WS5 requires BlackMage (hardening findings from WS2 feed production design)
- WS5 requires Architect (infrastructure patterns from Doom port to production)
- WS5 requires Developer (implementation)
- WS5 requires Moat Ghost (compliance gates before any public exposure)
- WS5 requires RFC Editor (spec updates for Section 12 proof + any IANA needs)
- WS5 requires Scientist (performance hypothesis validation)
- WS5 requires Warmonger (200+ step execution plan before coding begins)
- **Round Table reconvenes at WS5 kickoff (Mar 8)**

**Team Vibes**: **NUCLEAR.** 559 frames of Doom in eBPF. 33 days from first commit. Domain registered, protocol specified, computational completeness proven. The Kingdom is on fire and the fire is good. Channel this energy. Don't waste it.

---

### Unified Battle Plan

#### IMMEDIATE: Hardening Sprint (Today — 4-6 hours)

Quick wins from DOOM-HARDENING-BATTLE-PLAN.md. Highest value, lowest effort.

- [ ] Phase 0 Steps 1-18: Environment verification + artifact backup — Owner: Developer — 15 min
- [ ] Phase 2 Steps 39-72: Migrate /tmp scripts → scripts/doom/ — Owner: Developer — 90 min
  - [ ] load_rom_fast.py → scripts/doom/load_rom.py (add argparse, error handling, --dry-run)
  - [ ] bulk_inject.py → scripts/doom/inject.py (add argparse, signal handling, CRC-16)
  - [ ] read_cpu.py + reset_cpu.py → scripts/doom/cpu_state.py (unified subcommands, JSON output)
  - [ ] skip_loops.py → scripts/doom/skip_crt0.py (add timeout handling, logging)
  - [ ] Migrate test ROMs + manifest to scripts/doom/tests/
  - [ ] Archive metrics/state dumps to tmp/artifacts/
  - [ ] Clean /tmp of all project files
  - [ ] Update .gitignore
- [ ] Phase 8 Steps 151-165: Document current state — Owner: Developer — 30 min
- [ ] Phase 9 Steps 166-175: Enforce commit-per-step discipline — Owner: Developer — 15 min
- [ ] Moat Ghost quick fixes (from compliance audit):
  - [ ] Pin gosec to tagged release (P0 #11) — 5 min
  - [ ] Remove gosec -no-fail flag (P0 #12) — 5 min
  - [ ] Add MaxHeaderBytes to kanban-app (P0 #15) — 5 min
  - [ ] Verify Captain data dir not /tmp (P0 #14) — 5 min
- [ ] **EXIT GATE**: All scripts in repo, /tmp clean, P0 quick fixes merged, build passes, tests pass

#### WS1: Doom Video Dashboard (Feb 23-25) — Owner: Developer + Architect

- [ ] Create `cmd/doom-bridge/main.go` — reads screen buffer from RAM_MAP BPF map
- [ ] Apply Doom palette (256 colors) → RGB → PNG encoding
- [ ] WebSocket server pushes frames to connected browsers
- [ ] Wire into existing `dashboard/doom.html` + `js/doom-viewport.js`
- [ ] Frame rate display overlay (fps counter, instruction count, frame number)
- [ ] Use `cilium/ebpf` Go library (already in tree) for map access
- [ ] Test: browser shows live updating Doom frames while inject runs
- [ ] **DoD**: Open browser → see Doom title screen → watch demo cycle animate

#### WS3: Scale to Playable (Feb 23-28, parallel with WS1) — Owner: Developer + Architect

- [ ] Add BPF timestamp instrumentation to STATS map (measure actual XDP_TX bounce cycle time)
- [ ] Profile: run 1000 packets at delays [3000, 2000, 1500, 1000, 750, 500µs]
- [ ] Find minimum safe delay (no concurrent packet/CPU state corruption)
- [ ] Implement burst injection (Netflix model): fire N packets, drain XDP ring, repeat
- [ ] Rewrite injector in Go or Rust to eliminate Python socket.send() overhead
- [ ] Target: sustained 15+ fps with zero corruption for 60 seconds
- [ ] Document findings in `docs/doom/PERFORMANCE.md` including Netflix PPS comparison
- [ ] **DoD**: 15+ fps sustained, zero halts, zero ROM faults, browser playback smooth

#### WS4: Documentation & Conference Prep (Mar 3-7) — Owner: Captain + Lore + Developer

- [ ] Create `docs/wiki/` structure mirroring Kingdom hierarchy
- [ ] Write `docs/wiki/doom-over-ipv6.md` — the full story from chat app to 559 frames
- [ ] Write `docs/wiki/architecture.md` — how packets become computation (Section 12)
- [ ] Write `docs/wiki/bug-kill-chain.md` — strncpy, CALLR, RAM_MAP, translator, all 12 bugs
- [ ] Conference talk outline: "Doom in the Data Plane: Running a Game Engine Inside eBPF"
- [ ] Set up HTTP mirror of wiki within Kingdom dashboard
- [ ] Include origin story: Jan 20 first commit → Jan 21 domain → Jan 22 vision → Jan 25 Claude → Feb 22 Doom
- [ ] Update `references/timeline.md` with all Phase 7-9 achievements
- [ ] **DoD**: Wiki browsable at gateway endpoint, conference outline complete, timeline updated

#### WS2: Lich Campaigns D1-D6 (Mar 1-5, parallel with WS4) — Owner: BlackMage

- [ ] D1: MBC bytecode injection via ROM_MAP — attempt code injection, document mitigations
- [ ] D2: Framebuffer exfiltration — verify RAM_MAP access controls
- [ ] D3: Keyboard injection via SYSCALL topic — attempt input manipulation
- [ ] D4: Flow label collision — birthday attack analysis on 20-bit space
- [ ] D5: SYSCALL handler fuzzing — invalid syscall numbers, register state abuse
- [ ] D6: ROM integrity — TOCTOU test, unsigned ROM exploitation
- [ ] Write `references/lich-d1-d6-results.md` with findings, severity, PoCs
- [ ] Fix all CRITICAL/HIGH findings before WS5
- [ ] **DoD**: All 6 campaigns executed, documented, critical fixes merged

#### Scientist Experiments (Integrated with WS3) — Owner: Scientist + Developer

- [ ] Test H1: Instrument STATS map with per-bounce nanosecond timestamps
- [ ] Test H2: Burst injection batches {10, 50, 100, 200} — measure throughput + corruption
- [ ] Test H3: Implement Go AF_PACKET injector — A/B test vs Python bulk_inject.py
- [ ] Document findings as reproducible experiments in `docs/doom/EXPERIMENTS.md`
- [ ] If H3 confirmed: Go injector becomes canonical (replace Python)

#### Warmonger Plans Needed (Before WS5) — Owner: Warmonger

- [ ] Forge WS5 battle plan: 200+ steps for production packet tracing pipeline
- [ ] Forge WS1 execution plan: 50-80 steps for doom-bridge service (can do lightweight)
- [ ] Review/merge S31 and DOOM-HARDENING plans into canonical reference
- [ ] Archive completed plan phases (S30-S32) to `docs/archive/battle-plans/`

#### RFC Editor Actions (Integrated with WS4) — Owner: RFC Editor + Captain

- [ ] Draft Section 12 Appendix update: Doom computational completeness proof with metrics
- [ ] Draft `draft-bellis-unheaded-wotan-memory-01`: Wotan triple-role architecture (ring buffer + event bus + protocol RAM), DNS-hierarchy redundancy model, subscription mirroring, n+1 topic availability, scale spectrum (1→1000 instances)
- [ ] Review BCP 14 keyword usage in all three specs (audit pass)
- [ ] Prepare IANA tracking for potential HbH Option Type (WS5 prep)
- [ ] Conference talk: include RFC citations, make protocol visible as differentiator

#### WS5: Return to Core — Packet Tracing Observability (Mar 8+) — Owner: ALL

- [ ] **ROUND TABLE RECONVENES AT KICKOFF**
- [ ] Port XDP attachment + map pinning patterns from doom-ring to production scripts
- [ ] Implement `ebpf/packet_marker/` — XDP program stamps trace_id in IPv6 flow label
- [ ] Implement `ebpf/flow_tracker/` — TC program tracks connection state in BPF hash map
- [ ] Implement `ebpf/latency_probe/` — kprobe measures RTT via kernel timestamps
- [ ] Wire `cmd/trace-collector/` to read eBPF maps and publish to Wotan
- [ ] Wire dashboard to display real packet traces (not Doom frames — real traffic)
- [ ] Integrate with existing dashboard packet-flow.js visualization
- [ ] E2E test: HTTP request → gateway → service → response, entire path traced in dashboard
- [ ] Address remaining P0s from TODO.md (#9-15)
- [ ] Begin P1 security work: authentication (#16), TLS (#20, #26)
- [ ] **DoD**: Real packet trace from browser→gateway→service→response visible in dashboard with <50ms latency

#### Security Backlog (Integrated with WS5)

From TODO.md — prioritized for WS5 integration:

| Priority | Finding | When |
|----------|---------|------|
| P0 #10 | Nix cross-container circular dep risk | WS5 deployment |
| P0 #11 | gosec@master unpinned | Hardening sprint or WS4 |
| P0 #12 | gosec -no-fail flag | Hardening sprint or WS4 |
| P0 #13 | No release signing/SBOM | WS5 CI/CD |
| P0 #14 | Captain /tmp data dir | Hardening sprint |
| P0 #15 | No MaxHeaderBytes on kanban | Hardening sprint |
| P1 #16 | No auth on any endpoint | WS5 |
| P1 #20 | No TLS/VXLAN between containers | WS5 |
| P1 #26 | No TLS on gRPC | WS5 |

---

### Decisions Made at This Round Table

1. **Hardening sprint FIRST** — Quick wins before anything else. Migrate /tmp, harden scripts, enforce naming. Rationale: Prevents context rot. 4-6 hours invested saves days of "where did I put that script" later.
2. **WS1+WS3 parallel, Week 1** — Visual proof + performance are prerequisites for everything else. Rationale: Can't demo what you can't see. Can't impress at 4fps. Netflix burst model is the key to 15+ fps.
3. **WS2+WS4 parallel, Week 2** — Hardening and documentation while Doom context is fresh. Rationale: Security findings inform WS5 design. Documentation decays exponentially.
4. **WS5 is the endgame** — Everything before it is prelude. Rationale: Doom is proof. Packet tracing is product. The line is drawn.
5. **Wotan is triple-role: ring buffer + event bus + protocol RAM** — This is the foundational architectural insight from this Round Table. Wotan isn't a message bus with HA bolted on. It's the memory substrate of the protocol, the event transport for services, AND the ring buffer for kernel-to-userspace bridging. Rationale: This reframe makes everything cleaner — BPF programs read local Wotan maps (protocol RAM), userspace reads Wotan rings (event bus), and redundancy is subscription mirroring (ring buffer replication). No Raft. No Paxos. Just rings chasing rings.
6. **Wotan HA via subscription mirroring, not consensus** — DNS hierarchy + BGP-style topic advertisement + n+1 ring mirrors. Rationale: Consensus protocols are wrong for ring buffers. You don't vote on what the next event is — you write it and mirrors chase. Scale from 1 to 1000 with the same pattern.
7. **Skip Phase 1 (/tmp noexec) for now** — System-level mount changes can wait. Phase 2 (script migration) provides the real value. Rationale: Moving scripts to repo is more important than filesystem mount options.
6. **Netflix burst injection for WS3** — Replace steady-state fixed-delay injection with batch-inject model. Rationale: 0.003% of hardware capacity. The bottleneck is Python, not XDP. Burst amortizes overhead.
7. **Round Table reconvenes at WS5 kickoff** — Full assembly required for architecture review of production packet tracing. Rationale: WS5 is the product pivot. All seats must align.

### Open Questions (Carry to Next Round Table)

1. **Conference target** — Which conference? What deadline? Shapes WS4 urgency and talk format. Muck decides by Mar 1.
2. **Service breakout timing** — Does WS5 happen in monorepo or after breakout to individual repos? Recommendation: monorepo first. Breakout after first real trace works. Decide at WS5 Round Table.
3. **Doom as permanent feature** — Does the Doom PoC become a permanent demo in Unheaded, or archived after conference? Impacts maintenance burden. Recommendation: Keep as `examples/doom/` with maintenance-mode flag.
4. **Go injector for WS3** — Rewrite Python bulk_inject in Go/Rust for the burst model? Python socket.send() may be the real bottleneck. WS3 profiling will answer this empirically.
5. **Auth architecture for WS5** — JWT vs mTLS vs API keys? Need Architect + BlackMage + Moat Ghost alignment before implementation.

### Wins to Celebrate 🎉

- **559 FRAMES OF DOOM IN eBPF** — Computational completeness proven. Section 12 is REAL.
- **819M INSTRUCTIONS, ZERO HALTS, ZERO ROM FAULTS** — The translator, loader, and CPU are SOLID.
- **42x MEMORY REDUCTION** — RAM_MAP HashMap→Array. 671MB→128MB. Elegant.
- **XDP_TX TURBO MODE** — 255-bounce packet cycling on same interface. Cache-hot execution.
- **6 BUGS KILLED IN ONE SESSION** — strncpy off-by-one, I_Error, G_DoPlayDemo NULL, Z_Malloc infinite loop, V_DrawPatch NULL, stale doom_data.bin. Adversarial debugging at its finest.
- **LICH-007: 1B+ EXECUTIONS, ZERO CRASHES** — The MBC bytecode interpreter is fuzzer-hardened.
- **33 DAYS FROM FIRST COMMIT TO DOOM IN eBPF** — Jan 20 "Initial commit" → Feb 22 "D_DoomLoop running." One engineer. One AI. One Kingdom.
- **255K+ LINES OF CODE** — 23 services, 611 Go files, 14 Rust crates, eBPF programs, container definitions (LXD/containerd/NixOS/Docker), dashboard, kanban. An entire platform.
- **unheaded.org REGISTERED** — The domain. The name. The Kingdom has an address.

---

### Next Round Table
**Scheduled**: March 8, 2026 (WS5 kickoff)
**Reason**: Architecture review for production packet tracing. All seats required. The product pivot.
**Trigger**: Also convene if WS2 Lich campaigns find CRITICAL findings requiring architecture changes.

---

_Forged at the Round Table by all 15 minds — the full Royal Court assembled._
_Captain, Architect, Micromanager, Developer, Timeguru, Calendar, Lore, Kingdom, Busboy,_
_Warmonger, Scientist, BlackMage, Moat Ghost, RFC Editor, and the Round Table itself._
_33 days from first commit. 559 frames of Doom. The Void ran Doom. Therefore the Void can trace anything._
_Now we trace packets. Now we build the product. The Kingdom marches as one. LET'S GO._
</content>
</invoke>