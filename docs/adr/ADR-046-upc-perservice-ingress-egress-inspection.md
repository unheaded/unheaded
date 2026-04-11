# ADR-046: UPC Per-Service Ingress/Egress Packet Inspection (Anti-Fake-Horn Layer)

## Status: PIPE DREAM (no scheduled work — captured for future consideration)

## Date: 2026-04-11

## Decision Maker
- Stevie Bellis (Principal)

---

## Context

[[Mímir's Law|Mimirs-Law]] introduced Gjallarhorn UPC trigger packets — 20-byte
Monad register payloads that drive bootstrap and reverify flows. Phase 11
stress test demonstrated forgery rejection at the daemon level (bad magic
caught), but **the rejection happens AFTER the packet has crossed every
container boundary** between the network edge and the daemon.

Stevie's pipe dream: an **inspection layer at the ingress AND egress of every
container/service stack** that parses any UPC-shaped traffic, applies
per-service rules, and rejects "fake horns" before they reach the application
layer. Defense in depth — every boundary checks, not just the endpoint.

The intuition is that this would be an **eBPF program fused with a metal-host
controller**, where the controller is exposed only to specific privileged
userspaces (operator workstations, automation, never application code).
Application services see verified UPC traffic only.

This is **not scheduled** and has no production timeline. It is captured here
so the idea isn't lost.

---

## Decision

**ACKNOWLEDGE as pipe dream.** No build work scheduled. ADR exists to:
1. Reserve the concept space (no one else proposes a conflicting design)
2. Anchor the idea to existing UPC + eBPF infrastructure
3. Sketch a non-binding direction so a future contributor (human, BlackMage,
   or Champion) can pick it up

### Sketch (non-binding, rambling)

**Layer 1 — Per-container ingress XDP**:
- Each container's veth ingress runs an XDP program
- Recognizes UPC-shaped traffic (HbH option type, Monad magic, Gjallarhorn magic)
- Applies per-container rule set: which trigger kinds allowed, which clusters,
  which manifest_ptr ranges
- DROPs unauthorized; PASSes authorized; LOGs all to ringbuf

**Layer 2 — Per-container egress TC**:
- Same idea on egress: a service that's not supposed to emit Gjallarhorn
  cannot do so
- Catches compromised services trying to forge bootstrap announcements
- DROPs anomalous emissions; LOGs to ringbuf

**Layer 3 — Metal-host fusion controller**:
- A single Rust daemon on the metal host that owns all the per-container BPF
  programs
- Loads/updates rules from a central policy store
- Aggregates ringbuf events into a per-container security log
- Exposes a control socket ONLY to specific userspaces (uid/gid match, SO_PEERCRED check)
- Application code in containers has zero visibility into the controller

**Layer 4 — Policy distribution**:
- Policies signed with [[Wotan Topic Signing|Wotan-Topic-Signing]] (ML-DSA-65)
- Distributed via `config.upc_inspection.<container_id>` topic
- Per-container policies updateable without restart

### Connection to Existing Infrastructure

| Existing | Reuse |
|---|---|
| `pkg/gjallarhorn/` | Packet structure, magic recognition |
| `services/wotan/internal/signing/` | Policy distribution signing |
| `pkg/discovery/` | Container/service identity |
| eBPF + Aya patterns | Already proven in `cmd/ebpf-collector/` |
| The frozen Monad v0x01 wire format | What we parse |

### Threat Model

**Defends against**:
- Forged Gjallarhorn packets (bad magic — already caught) AND
- Forged Gjallarhorn packets with VALID magic but unauthorized cluster_id
- Forged Gjallarhorn packets with valid magic + cluster but unauthorized manifest_ptr
- Compromised application services emitting Gjallarhorn-shaped packets they
  shouldn't be emitting
- Lateral movement via UPC traffic between containers
- Replay of legitimate but stale UPC packets

**Does NOT defend against**:
- Compromised metal host (root on the host wins)
- Compromised inspection controller (single point of trust)
- Side-channel attacks via legitimate UPC traffic patterns

---

## Consequences

### Positive
- Defense in depth — every boundary checks
- Per-service blast radius containment
- BlackMage gets a rich attack surface to test
- Aligns with the [[Heimdall at Every Bridge]] philosophy from `docs/lore/NORSE_MYTHOLOGY.md`

### Negative
- BPF instruction budget pressure — per-container programs add up
- Policy management complexity — N containers × M rules
- Performance: every UPC packet parsed twice per hop (ingress + egress)
- The "controller exposed to certain userspaces only" model needs careful
  capability design — cgroups + namespaces + SO_PEERCRED + selinux

### Mitigations
- **Pipe dream tier**: no commitment to ship
- If implemented, prototype on 2 containers first, measure overhead
- Reuse existing eBPF infrastructure — don't build a new framework
- Policies versioned + signed — no unsigned updates ever

---

## Alternatives Considered

### Alternative A — Just trust the daemon-level forgery rejection
**Already implemented**. Phase 11 demonstrated bad magic is caught. Adequate for
single-host or trusted-network setups. Insufficient for multi-tenant or
hostile-network scenarios.

### Alternative B — Service mesh sidecar inspection
Run an Envoy/Istio-style sidecar in every container. **Rejected** because:
- Adds containers to every container (resource cost)
- Doesn't catch egress before it leaves the container's network namespace
- Doesn't fit the "metal-host fusion" intuition

### Alternative C — Build it now
**Rejected**. Mímir's Law just shipped. Wave 10D is the next prioritized
sprint. No customer asking for it.

---

## References

### Related ADRs
- [[ADR-043|Mimirs-Law]] — Gjallarhorn packets this would inspect
- [[ADR-69420]] — Sleipnir + Yggdrasil parent vision
- [[ADR-042 LICH]] — Red team campaign infrastructure

### Related Components
- `pkg/gjallarhorn/` — UPC trigger packet structure
- `services/wotan/internal/signing/` — Policy signing reuse
- `cmd/ebpf-collector/` — eBPF userspace control patterns
- `crates/heimdall-bpf/` — Existing per-host BPF kprobes
- `docs/lore/NORSE_MYTHOLOGY.md` — Heimdall at Every Bridge philosophy

### Related Lore
- "Heimdall is not only the watchman of Bifröst — Heimdall is at every bridge,
  on both sides, with the same eye." This ADR is the technical realization of
  that philosophy at the container/service-stack boundary.

---

*ADR-046 — filed as pipe dream 2026-04-11*
*"Every bridge has a Heimdall. Even the ones that look like veth pairs."*
