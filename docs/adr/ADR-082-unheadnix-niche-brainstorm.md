<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# ADR-082 — unheadnix: feasible-usage brainstorm (ultralight UPC pool nodes)

**Status**: BRAINSTORM / EXPLORATORY — *not a decision.* Feeds a future ADR once a
first niche is picked and probed. Nothing here is committed or scheduled.
**Date**: 2026-07-19
**Deciders**: Stevie (directive to brainstorm); Fable 5 session (Architect lens)
**Extends**: ADR-081 §Open-Question-4 (the "parallel niche" the Debian anchor does
*not* serve) and §5 (working name `unheadnix`)
**Relates to**: ADR-080 (UPC programmatic API — unheadnix is "a new guest workload"),
ADR-074 (per-pid MMU isolation), ADR-067 (MBC ISA / verifier + code-store walls),
ADR-043 (Mímir's Law / Gjallarhorn / Gungnir — trigger + sealed-payload primitives),
EVOLUTION-1 (`references/evolution1-stage1-2026-07-19.md` — the dual-books finding,
which shapes the "single-process node" framing below)

---

## TL;DR

`unheadnix` is not a small Linux. It is a **single-`.mbc`-binary compute unit that
boots on an eBPF-hosted virtual CPU (the UPC)**, isolated by per-pid MMU, deny-by-
default at the ABI, byte-deterministic, and cheap enough to run thousands-per-host
and throw away. That specific shape — *tiny + isolated + ephemeral + attestable +
co-located with the kernel data plane* — is bad at "be a general container" and
unusually good at a cluster of **secops and observability** jobs that today waste a
whole OS (or a long-lived agent that becomes the attack surface). This doc maps
those jobs, the anti-jobs, and the feasibility gaps.

**This does NOT touch the Debian-anchor track.** Per ADR-081 §Q4 that track owns
compliance/CVE-no-drift and general workloads. unheadnix is the *parallel* niche.

---

## What defines the edge (why the shape matters)

| Property | Where it comes from | What it buys the niche |
|---|---|---|
| **Single-binary footprint** | Whole guest is one `.mbc`; boots on the eBPF vCPU, no busybox, no multi-MB kernel (ADR-081) | Sub-second boot, tiny RAM, K8s-container density × many; cheap to fan out and recycle |
| **Isolation by construction** | Per-pid MMU (ADR-074) + the guest can only do what the UPC ABI exposes | Deny-by-default surface: no shell, no coreutils, no package manager, no syscalls you didn't grant. A pivot needs a code path that *does not exist* |
| **Ephemeral / pool-native** | Triggered provisioning (Gjallarhorn 20-byte packets), no persistent state | "Key dies with the node." Elastic with load. Nothing to leave running to be compromised |
| **Attestable provenance** | Byte-deterministic `.mbc` + RV2MBC SHA gate + Sealed-Cask discipline | A node can prove *what it is* by hash — chain-of-custody for compliance without a heavyweight measured-boot stack |
| **Kernel-data-plane co-location** | The UPC *is* an eBPF program | Compute can live at the XDP / ring-buffer source, before the "everything to userspace" firehose |
| **Protocol-native** | Speaks Monad on the wire; can be a Wotan memory participant | Emits results as first-class Kingdom traffic, no bolt-on agent/exporter |

The through-line: **you don't pay for an OS you don't use, and there's almost
nothing there to attack.** Everything below is a consequence of those two facts.

---

## Hard rule: services only — zero interactive surface (Stevie, 2026-07-19)

An unheadnix node is a **service, not a machine**. All a lightweight node carries
is its one service; it exposes exactly the ABI that service needs and *nothing
else*. There is deliberately **no interactive access surface of any kind**:

- **No shell** — no `sh`/`ash`/`bash`, no command interpreter, nothing that turns
  input into arbitrary execution.
- **No ssh, no telnet** — no remote-login daemon, no management/listening login
  port. You cannot "log into" a node because there is nothing to log into.
- **No TTY / serial console** — no interactive console device, no getty, no
  operator prompt. (The UPC TTY exists for *platform* bring-up/diagnostics, never
  as a guest-facing console.)
- **No coreutils, no package manager, no editors** — nothing to stage a payload
  with or to live off the land.

The node does one thing: run its service, speak its bounded ABI (Monad/Wotan
emit, or the single syscall its role was granted), and recycle. Management is
**out-of-band and declarative** — you don't administer a node, you replace the
pool. This is the "Isolation by construction" row taken to its limit: a pivot, a
lateral move, or an interactive foothold needs a code path that *does not exist in
the binary*. Absence of capability — not policy layered on top of capability — is
the control.

---

## The niche map

### A. Secops — key / cert / attestation (the strongest fit)

The reason this is the strongest fit: a signing/rotation job wants *maximum
isolation and minimum lifetime* for the secret, and *zero* incidental capability.
unheadnix is all constraint and no convenience — exactly backwards from a general
VM, exactly right here.

| Use case | Sketch | Why unheadnix, not a container |
|---|---|---|
| **Ephemeral short-lived-cert minter** | Node boots, holds a signing key in per-pid memory, mints a batch of short-TTL mTLS/workload certs (SPIFFE-SVID-shaped), then is destroyed; key never hits disk or a shared address space | No shell/no FS to exfil through; key lifetime = node lifetime; mint surface is the only code path |
| **Key-rotation / enforcement pool** (ADR-081 Q4 named this) | A pool where each node owns one rotation domain, rotates on schedule, attests the new material, recycles | Node *cannot* do anything but rotate — enforcement is architectural, not policy |
| **Sealed-secret / SOPS-age decryption oracle** | Node holds one age/SOPS private key, answers "decrypt this" over Monad/Wotan, is per-pid isolated and ephemeral | Decrypt oracle with a hash-attestable identity and no lateral surface |
| **Attestation witness** | Tiny node measures a thing and emits a *signed* measurement as an Anamnesis/Wotan event | Byte-deterministic `.mbc` means the witness can prove its own integrity by hash |
| **HSM-lite signing pool** (dev/test + low-assurance) | Software approximation of an isolated signer for pipelines that can't justify a real HSM | Honest framing: it approximates HSM *properties* (isolation, small TCB, ephemerality); it is **not** a certified HSM — see anti-niches |

Composes with existing primitives: **Gungnir** (ML-DSA-65 sealed payloads) as the
payload format, **Gjallarhorn** as the "wake a signer pool" trigger, the PQC work
(SLH-DSA / FIPS-205) as the algorithm — *if* it fits the verifier budget (gap below).

### B. Observability — ephemeral monitoring / metrics pools

The reason this fits: monitoring wants **massive fan-out of cheap, disposable,
low-privilege probes** and **pre-aggregation as close to the source as possible**.
Long-lived agents are the opposite: privileged, persistent, and a pivot target.

| Use case | Sketch | Why unheadnix |
|---|---|---|
| **Ephemeral synthetic / blackbox probes** | One node per check (or per target), run the probe, emit result as a Monad register / Wotan event, recycle | Near-zero per-probe cost; no standing agent; the prober can't be pivoted through |
| **Packet-path pre-aggregators** | Node runs at the XDP / ring-buffer source, filters + rolls up (top-k, histograms, anomaly/CRC counters) before shipping | UPC is already eBPF — reduces the ring-buffer → userspace firehose at the source (ties to the Whispering Void / Anamnesis pipeline) |
| **Window rollup pool** | Nodes consume raw event streams, emit compact summaries per window, scale with load | Elastic, stateless, throw-away-per-window |
| **Minimal-surface heartbeat emitters** | Each service gets a tiny liveness emitter to the control plane | Too small to be worth attacking, nothing to pivot into (no shell) |

Honest connection to **Enkrateia** (the alerts-only, zero-FS-mutation drift
aggregator, ADR-043): that "observe, never mutate" posture is a natural fit for a
node whose ABI simply doesn't *grant* mutation.

### C. DevOps — tiny containers / one-shot runners

The reason this fits: **deterministic, sandboxed, single-purpose steps** where a
container is mostly overhead and the determinism buys reproducibility.

| Use case | Sketch | Why unheadnix |
|---|---|---|
| **Deterministic step runners** | Config render, template expansion, one reconcile step, a webhook validator — one shot, then gone | Byte-identical `.mbc` → reproducible CI step; sandbox by construction |
| **Admission / policy webhook pods** | Deny-by-default policy evaluators (OPA-lite / K8s admission), one-per-namespace density | Tiny enough to run everywhere; can't be pivoted |
| **Throwaway CI / build shards** | Pools of disposable executors for parallel test/lint shards | Fast boot, no state, torn down; density = cheap parallelism |
| **First-breath provisioning nodes** | Gjallarhorn multicast wakes a pool to bootstrap a segment *before* the heavier Debian-anchor image lands (Sleipnir/Gleipnir PXE substrate) | unheadnix is the smallest thing that can answer a provisioning trigger |

### D. Mesh / substrate (the longer horizon, per ADR-081 Q4)

- **BGP-mesh micro-speakers** — many tiny nodes each speaking a slice of BGP /
  route-reflection; density is the point. (Needs the networking gap resolved first.)
- **DMZ / edge minimal-surface relays** — where a full OS is a liability, a node
  with almost no surface is the safer bastion/jump-oracle.

---

## The unifying pattern: multiplicity-of-tiny, not multiprocessing-in-a-node

A design insight sharpened by tonight's EVOLUTION-1 work: the kernel's process
model still has a **dual-books hazard** (`myproc()` / `cpu->proc` can disagree with
the BPF scheduler — see the Stage-1 log), which makes *multiprocessing inside one
guest* the untrustworthy path until a proc[] reconciliation sprint lands.

That is not a blocker for this niche — it's a **design pointer**. Every use case
above is naturally **one process per node, many nodes per pool**. Isolation comes
from *multiplicity of tiny single-purpose nodes*, not from a scheduler juggling
tenants inside one node. So unheadnix should lean into single-process pool nodes
and let the pool orchestrator (not an in-guest scheduler) provide concurrency. This
also keeps each node well under the verifier's 1M-instruction wall.

---

## Anti-niches (where NOT to use it — this is what keeps the ADR honest)

- **General-purpose containers / "just run my binary."** Full Linux syscall ABI,
  real drivers, big memory, heavy compute → **Debian-anchor track** (ADR-081 §Q4).
  unheadnix is not a container runtime.
- **Stateful / long-lived services.** It is ephemeral by design; persistence is
  someone else's job (Wotan / the anchor track).
- **High-assurance crypto root-of-trust.** It is *software* isolation; the host
  kernel + eBPF runtime is the TCB. Do not market it as an HSM. It's an HSM-*shaped*
  convenience for dev/test and low-assurance tiers only.
- **Verifier-budget-heavy workloads.** The 1M-instruction ceiling and code-store
  map capacity are hard walls (ADR-067). Crypto-in-guest is the first thing at risk.
- **Anything needing a rich network stack today.** See the gap below.

---

## Feasibility gaps / open questions (scope these before any niche ADR)

1. **I/O surface — the biggest one.** The UPC today exposes TTY + an in-BPF FS
   reader + argv. A metrics scraper or a signer needs to *emit* somewhere. Options:
   (a) emit as Monad registers / Wotan writes (protocol-native, preferred), (b) a
   minimal UDP/datagram ABI syscall. Every niche above is gated on picking this.
   Nothing ships until the guest can talk to the outside in a bounded way.
2. **Crypto vs the verifier budget.** In-guest PQC signing (Gungnir/SLH-DSA) may
   not fit 1M instructions. Likely answer: a UPC ABI syscall that delegates the
   heavy primitive to a host-side helper, keeping the *key* in-guest but the *math*
   out. Needs a security review — that delegation is exactly where a key could leak.
3. **Lifecycle / pool orchestration.** Gjallarhorn gives the wake trigger; teardown,
   scheduling, and pool sizing need an orchestrator (Sleipnir/Gleipnir territory).
   Ephemerality's security value depends on *reliable* teardown.
4. **Process model.** Keep to single-process nodes until the proc[] dual-books is
   reconciled (EVOLUTION-1). Good news: the niche wants that anyway.
5. **Provenance chain.** The RV2MBC SHA gate + Sealed-Cask give per-node identity;
   what's missing is the *registry* that maps "this hash = this attested role" so a
   verifier can trust a witness/signer. Small, but load-bearing for the secops cases.

---

## Existing Kingdom primitives this composes with (nothing here is from scratch)

| Primitive | Role for unheadnix |
|---|---|
| **Gjallarhorn** (20-byte Monad triggers) | "Wake / provision this pool" signal |
| **Gungnir** (ML-DSA-65 sealed payloads) | Sealed payload format for the crypto niches |
| **Mímir's Law / Gleipnir** (UPC baseline delivery) | Delivery substrate for the node image |
| **Sealed Cask + RV2MBC SHA gate** | Deterministic build + hash-attestable node identity |
| **Wotan** (LOCAL + DISTRIBUTED memory) | Where nodes read/write results without a bolt-on exporter |
| **Monad** (wire register) | Node output as first-class Kingdom traffic |
| **Enkrateia** (alerts-only, zero-mutation) | The posture the observability nodes should inherit |
| **per-pid MMU** (ADR-074) | The isolation the secops nodes lean on |

---

## Suggested first feasible probe (when/if this graduates from brainstorm)

Pick the **smallest secops node that proves the shape**: an *ephemeral signing
oracle* — boot a unheadnix node, hand it a key via a sealed Gjallarhorn/Gungnir
payload, have it emit N signed Monad registers, then tear it down and confirm (a)
the key never left the per-pid slice and (b) the node's `.mbc` hash matches the
attested role. That single probe exercises the two hardest gaps at once — the I/O
emit path (#1) and the crypto-delegation/security question (#2) — and if it holds,
every other niche is a variation on it. Everything before that probe is talk.

---

## Status / next

Brainstorm only. No decision, no schedule, no scope claim on the Debian-anchor
track. Graduation path: Stevie picks one niche → Architect + BlackMage + MoatGhost
scope the I/O + crypto-delegation gaps → a real ADR + a Warmonger probe plan. Until
then this doc is the map, not the march.
