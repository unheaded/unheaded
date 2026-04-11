# ADR-047: K8s Honest Assessment + Extractable Tools + East/West K8s Lab

## Status: ACKNOWLEDGED + LAB ACTION ITEM

## Date: 2026-04-11

## Decision Maker
- Stevie Bellis (Principal)

---

## Context

A lot of Unheaded's application layer overlaps with what Kubernetes already
does: service discovery, config delivery, drift detection, desired-state
reconciliation, health monitoring, message bus pub/sub, alerts. This is fine
for learning and lab work — re-implementing K8s primitives is how you
actually understand them — but it's worth being honest about the overlap and
identifying which pieces of Unheaded are **genuinely novel** and could be
extracted/released independently.

Two things prompted this ADR:

1. **Honest self-assessment**: The Mímir's Law spike (ADR-043) and the
   broader Wotan/Heimdall/Enkrateia stack are operationally similar to a
   K8s operator with CRDs + reconciliation loops + eventing. Functional, but
   K8s already does it. The novel parts are below the application layer.

2. **Job-market reality**: 9 of 10 companies hiring infra demand Kubernetes
   experience Stevie doesn't yet have. A practical East/West K8s lab is the
   fastest path to closing that gap, and it pairs naturally with the existing
   bare-metal setup.

---

## Honest Assessment: Where Unheaded ≈ K8s

### Overlap (the "K8s clone" parts)

| Unheaded | K8s Equivalent |
|---|---|
| Wotan pub/sub bus | etcd watch + informers + events |
| `cmd/heimdall-daemon` drift detection | controller reconciliation loop |
| Mjölnir manifest | desired-state CRD + GitOps (Argo/Flux) |
| Enkrateia alerts | events + alertmanager |
| `pkg/discovery/` | DNS-based service discovery + Service objects |
| Cross-service health monitoring | liveness/readiness probes + percentage-based consensus |
| `services/wotan/internal/signing/` config.* | sealed-secrets / SOPS / cert-manager admission |
| Kanban + timeline | external project management — not K8s |

If Unheaded shipped today as "yet another orchestrator," it would be
strictly inferior to K8s on every axis except hardware footprint (14GB
vs K8s control plane's ~4GB minimum).

### Genuinely Novel — Worth Extracting

These are the parts that **no existing tool does**, or does meaningfully
worse than the Unheaded approach:

1. **Monad wire format (frozen v0x01)** — 20-byte register file in IPv6
   Hop-by-Hop Options. Programmable per-hop without an overlay network.
   Cilium gets close as a CNI plugin but doesn't define a wire protocol.
   **Standalone potential: HIGH** — could ship as a Linux kernel patch series
   or as an eBPF library.

2. **PQ-signed wire-level config delivery** — ML-DSA-65 signatures
   propagated at the packet level via Gjallarhorn UPC triggers. No existing
   tool does post-quantum signing on the orchestration data plane (most
   stop at TLS 1.3 mTLS).
   **Standalone potential: MEDIUM** — could ship as `cilium-pqc` or a
   Linkerd extension.

3. **eBPF as the substrate, not bolted on** — Heimdall, the trace
   collector, the BPF flow graph, the AF_XDP pipeline (920K pps validated).
   Cilium proves this model works as a K8s plugin. Hubble proves the
   observability angle.
   **Standalone potential: HIGH** — but the market is crowded
   (Cilium owns this lane).

4. **14GB-RAM minimum control plane target** — opinionated minimal
   orchestration. K8s cannot run comfortably on this hardware.
   **Standalone potential: MEDIUM** — niche edge/IoT use cases. K3s and
   Talos already serve some of this market.

5. **The `crates/zhenai-forge` LoRA training engine** — pure Rust, hipBLAS,
   streaming GGUF dequantization. Not orchestration — adjacent.
   **Standalone potential: MEDIUM** — small but real audience for ROCm-first
   training tooling. Most projects ship CUDA-only.

6. **`pkg/champion/` Trust Level harness** — sandboxed file R/W + Kanban
   CRUD + action logging + snapshots for AI agents. Not K8s overlap at all.
   **Standalone potential: HIGH** — could ship as an MCP-compatible
   "agent sandbox" library. Strong fit for the Anthropic ecosystem.

7. **`raft/zhen_mcp_server.py` Kingdom MCP server** — pre-built MCP tools
   for project introspection (corpus_search, file_read, runbook_show,
   service_health). Not K8s.
   **Standalone potential: MEDIUM** — useful as a template for other
   project-specific MCP servers.

### What Should NEVER Be Released

- The full "Unheaded orchestrator" framing — it's a learning project, not
  a product
- The application layer reimplementation of K8s features — strictly inferior
- The Mímir's Law spike as a K8s replacement — it's a Phase 0 dogfood, not
  a competitor

### What COULD Be Released

In rough priority order based on novelty + market gap:

1. **`pkg/champion/`** as a standalone "AI agent sandbox" Go library — fits
   into the MCP ecosystem, scratches a real itch, smallest extraction effort
2. **`raft/zhen_mcp_server.py`** as a "Kingdom-style MCP server template" —
   project-specific MCP servers are emerging as a pattern
3. **`crates/zhenai-forge`** as a ROCm-first LoRA training crate — narrow
   audience but no real competition
4. **Monad wire format spec** as IETF Internet-Draft (already in flight) +
   reference XDP implementation
5. **`pkg/gungnir`** as a generic ML-DSA-65 sealing wrapper — tiny package,
   solves a real problem, fits into existing post-quantum tooling

---

## East/West K8s Lab Action Item

**Goal**: Stand up a real Kubernetes cluster across the existing WEST + EAST
hosts so Stevie can practice k8s primitives, take notes, and use the cluster
to validate which Unheaded ideas would be better expressed as K8s operators
vs standalone tools.

### Constraints

- WEST (12-core, 14GB, AMD GPU) + EAST (4-core, 8GB)
- WireGuard overlay `fd00:dead:beef::/48` already up and verified
- Both hosts on Debian-family OS, both have sudo
- Cannot break existing Mímir's Law / Wotan / Heimdall daemons currently
  running on EAST

### Approach Options

| Option | Pros | Cons |
|---|---|---|
| **kubeadm** | Vanilla k8s, what jobs ask for | Heavy, fragile, lots of moving parts |
| **k3s** | Lightweight, single binary, fits 8GB EAST | Not "vanilla" — some interview gotchas |
| **kind** | Fast iteration, ephemeral | Single-host, defeats the cross-host purpose |
| **Talos** | Immutable, modern | Steep learning curve, less interview-relevant |

**Recommended**: **k3s** for the first pass. Single binary, server on WEST,
agent on EAST, full kubectl experience, fits memory budget. Once Stevie is
fluent, optionally migrate to kubeadm for "real k8s" interview prep.

### First Lab Tasks (non-binding)

1. Install k3s server on WEST, agent on EAST, verify both nodes Ready
2. Deploy nginx + service + ingress, verify cross-node pod scheduling
3. Write a custom CRD + simple controller in Go (this is where the
   K8s-vs-Unheaded comparison gets concrete — Stevie will viscerally
   understand the overlap)
4. Deploy Cilium as CNI to see what eBPF-as-plugin actually feels like
5. Try to express ONE Mímir's Law concept as a K8s controller (e.g.,
   baseline drift detection as a CRD + reconciler) — see how it compares
   to the Unheaded version
6. Capture notes on what's painful, what's elegant, what Unheaded does
   better, what K8s does better

### Coexistence with Existing EAST Spike

EAST already runs `/opt/spike-mimirs/heimdall-daemon` (manual launches, no
systemd unit yet). k3s agent on EAST should not conflict — they own
different ports and don't compete for the same kernel hooks.

If conflicts arise: stop the spike daemons before lab work, restart them
afterward. They are not in a steady-state service yet.

---

## Decision

**ACKNOWLEDGE** the K8s overlap honestly. **EXTRACT** opportunities exist;
file them as future ADRs (047a, 047b, 047c per extraction candidate, only
when there's actual interest in releasing one). **STAND UP** a k3s lab on
WEST + EAST as a practical learning task — separate from the Unheaded
roadmap, no battle plan needed, Stevie drives the pace.

This ADR exists to:
1. Stop pretending Unheaded's application layer is novel (it isn't)
2. Identify the parts that ARE novel and could be released
3. Capture the K8s lab as a real lab item (job-hunt aligned)
4. Give Stevie a thing to point at when interviewers ask "why are you
   building a K8s clone?" — answer: "I'm not. I'm building a programmable
   wire-level data plane and using K8s-shaped application primitives as a
   learning vehicle."

---

## Consequences

### Positive
- Honest framing prevents wasted effort on application-layer features that
  K8s already does better
- Identifies real release candidates (`pkg/champion/`, `zhenai-forge`,
  `pkg/gungnir`, MCP server template)
- Provides interview ammunition (k3s lab + concrete K8s-vs-Unheaded notes)
- Removes pressure to "make Unheaded the next K8s"

### Negative
- Forces acknowledgment that some sprint work was strictly K8s-shaped (not
  a problem if framed as learning, but worth being honest about)
- K3s lab takes time away from Wave 10D and other roadmap work
- Risk of getting sucked into "let me just polish this for release" — keep
  extraction work strictly time-boxed

### Mitigations
- Extraction candidates are NOT scheduled — only file ADRs when releasing
- K3s lab is Stevie's personal time, not an Unheaded sprint
- Use the lab to validate ideas (e.g., "would Mímir's Law be better as a
  k8s operator?") rather than to compete

---

## Alternatives Considered

### Alternative A — Pretend Unheaded is fully novel
**Rejected**. Self-deception wastes time. The novelty is below the
application layer, not at it.

### Alternative B — Pivot Unheaded to be a K8s plugin
**Rejected** (for now). Already discussed in conversation: K8s itself may
be heading the way of Jenkins (plugin ecosystem > tool). Betting the whole
project on K8s longevity is risky. The plumbing approach (multiple
delivery vehicles) is the right strategy.

### Alternative C — Skip the K3s lab, just read books
**Rejected**. Stevie learns by building. Reading books for K8s knowledge is
strictly worse than running an actual cluster across his existing hardware.
The lab is the fastest path.

---

## References

### Related ADRs
- ADR-040 — Kubernetes Ecosystem Strategy (existing strategic position)
- ADR-043 — Mímir's Law (the "is this just K8s?" question that prompted this)
- ADR-019 — Zhen Champion Agent (extraction candidate)

### Related Components (extraction candidates)
- `pkg/champion/` — sandboxed AI agent harness
- `raft/zhen_mcp_server.py` — MCP server template
- `crates/zhenai-forge/` — ROCm LoRA training
- `pkg/gungnir/` — ML-DSA-65 sealing wrapper
- `docs/protocol/draft-bellis-unheaded-foundation-*.md` — Monad wire format spec

### External
- k3s docs: https://docs.k3s.io
- Cilium: https://cilium.io (the eBPF-as-K8s-plugin reference)
- Talos: https://www.talos.dev (the immutable-OS-K8s alternative)
- MCP spec: https://modelcontextprotocol.io

---

*ADR-047 — filed 2026-04-11*
*"Honesty is cheaper than self-deception. The novelty is below the application layer."*
