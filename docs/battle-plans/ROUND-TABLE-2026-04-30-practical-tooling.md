# Battle Plan: Practical Tooling From UPC + Unheaded Infra
## Convened: 2026-04-30 | Reason: Critical Decision — Product Wedge Brainstorm
## Kingdom State: Age 3 In-Progress | Wire Format FROZEN v0x01 | ~415K LoC prod / 753K w/ tests

---

### Doctrine (binding on this entire plan)

**FREE TO USE. FREE TO SHARE. NO SELLING. NO PAID TIERS.** Every tool catalogued here is
gifted to the commons under GPL-3.0 (or Apache-2.0 / MIT where ecosystem reach demands).
Our moat is technical excellence and community trust, not licensing walls. Replace
"sell / paid / monetize / adopter / GTM / ACV / revenue" everywhere with "share /
publish / contribute / adopter / dogfood / commons / deploy". This doctrine overrides
any prior commercial framing in earlier draft language.

### Situation Report

We have built a planet. 34 services, 23 eBPF programs, 12 IANA registries, frozen wire
format, dual bare metal (WEST + EAST), Zhen AI online, UPC Dream Ladder L6, 31 runbooks,
sealed-cask deterministic builds, ML-DSA-65 PQ signing, GPL-3.0. The Round Table is
summoned to **harvest tools from the platform and give them to the community** —
shippable, hardened, free artifacts each proving a slice of the Kingdom while the
foundation keeps deepening.

Guiding voices: **Developer, Architect, Micromanager, BlackMage, MoatGhost, Sentinel**.
Six security/build/ship-discipline minds means the brainstorm leans **defensive blue,
offensive red, hardened-build, SRE pain-killer**. The deliverable is a tool that ships
hardened on day one, costs nothing, and refuses to touch user data by construction.

---

### The Throne Speaks (Captain — Vision & Direction)
**North Star**: "Production-ready infra in hours, free for everyone." Each tool is a
*standalone gift to the commons* that proves a slice of Unheaded without requiring users
to adopt the whole platform. Adopters discover the Kingdom one sharp tool at a time.
**Position**: Observability, supply-chain integrity, and on-prem AI are commodified by
proprietary vendors charging rent for things that *should* be free. We have unfair tech
in all three and we give it away. Community trust compounds; license walls erode.
**Key Decision**: Lead with 3 tools with deepest technical moat (eBPF-native, PQ-signed,
self-hosting). Avoid commoditized lanes (yet-another-SIEM). All tools GPL-3.0, source
public, contribution welcome.

### The Anvil Reports (Developer — Implementation Reality)
**Reusable building blocks already in tree** (cherry-pickable into standalone tools):
- `pkg/transport/` — gRPC-first cascade w/ HTTP/3 fallback
- `pkg/discovery/` — 4-layer service discovery
- `pkg/logagg/` — ring-buffer log aggregation + zerolog hook
- `pkg/auth/` — Noop/APIKey/JWT + RBAC + audit (64 tests)
- `pkg/champion/` — sandboxed file R/W + action logging
- `pkg/ports/` — port registry pattern
- `pkg/gungnir/` — ML-DSA-65 sealed payloads
- `pkg/gjallarhorn/` — 20-byte Monad trigger packets
- `pkg/enkrateia/` — alerts-only drift aggregator (zero FS mutations)
- `crates/zhend/` — anti-fragile gossip substrate, PQ crypto
- `crates/doom-runner/` — Aya-based Rust runtime (proves UPC compute)
- `scripts/build-sealed-cask.sh` + `verify-binding-rune.sh` — deterministic builder
**Verdict**: We don't have to *build* a product. We have to *extract* one. 80% of the code
is already in monorepo, license-clean, test-covered. Each wedge is a `cmd/` + thin SaaS
wrapper + landing page.

### The Blueprint Reveals (Architect — Surface Map)
The platform decomposes into **9 extractable product surfaces**:

| # | Surface | Source Components | Wedge Product |
|---|---------|-------------------|---------------|
| 1 | **Packet-Zero Observability** | eBPF marker + trace-collector + Wotan + dashboard | Anamnesis Lite — drop-in APM appliance, trace from XDP to browser |
| 2 | **Config Drift Sentry** | Heimdall + Mjölnir + Enkrateia + Gjallarhorn | Mímir — alerts-only compliance drift detector for regulated shops |
| 3 | **PQ Software Distribution** | Gungnir + sealed-cask + binding-rune + ML-DSA-65 | Gungnir Distribute — SLSA-3 + PQ-signed artifact bus |
| 4 | **Untrusted-Code Sandbox** | UPC + MBC ISA + Shim + framebuffer | UPC Sandbox — WASM competitor, deterministic replay, Anamnesis trace |
| 5 | **On-Prem AI Runbook Agent** | Zhen Champion + RAG + 31 runbooks + MCP server | Zhen On-Prem — air-gapped SRE assistant for compliance-heavy enterprise |
| 6 | **Continuous Adversary** | Lich + BlackMage automation + CISA KEV/NIST NVD MCP | Lich-as-a-Service — daily red-team-in-a-box w/ blue-team handoff |
| 7 | **PQ-Signed Message Bus** | Wotan + topic signing + ML-DSA-65 | Wotan Cloud — Kafka/NATS competitor w/ post-quantum integrity |
| 8 | **eBPF Network Watchman** | Sentinel + Pi-hole inventory + DGA/beacon detect | Sentinel Edge — home/SMB blue-team appliance |
| 9 | **Universal IaC Translator** | IaC backend renderers (Ansible/TF/Puppet/K8s/Chef/Salt) | Rosetta — desired-state model that emits any IaC dialect |

**Architecture risk**: extraction must NOT fork the monorepo. Each wedge is a **subset
build target** — same SPDX, same SBOM, same hardening baseline. The Sealed Cask binds them.

### The Ledger Records (Micromanager — Priority Stack)
Triage the 9 surfaces against **time-to-public-release × technical impact × dev cost**:

**P0 — Publish in 6 weeks (highest impact × already 80% built):**
1. **Anamnesis Lite** (Surface #1) — packet-zero APM, given freely. Proprietary APM vendors charge fortunes for less. We give the better thing away.
2. **Mímir Drift** (Surface #2) — REAL-METAL VALIDATED on EAST. Alerts-only is the design principle — no auto-remediate means operators stay in control. SOC2/HIPAA/PCI shops can adopt without procurement cycles because there is no procurement.
3. **Zhen On-Prem** (Surface #5) — air-gapped + PQ-signed + 1.52M-vector corpus. Communities with classified or regulated data cannot send their data to ChatGPT. We give them the alternative, free.

**P1 — Publish in 12 weeks (deeper build, broader reach):**
4. **Gungnir Distribute** (Surface #3) — SLSA-3 supply chain, PQ-ready, free
5. **UPC Sandbox** (Surface #4) — needs Doom-finishing first; massive technical impact once it lands
6. **Sentinel Edge** (Surface #8) — home-lab + SMB community, contribute-back model

**P2 — Defer to next Round Table (research-grade or low impact):**
7. Lich-as-a-Service — Barrister ethics review needed (autonomous adversary across third-party assets)
8. Wotan Cloud — message bus space is crowded; wait for protocol IANA
9. Rosetta IaC translator — useful but thin technical moat

**QA Gates per wedge** (non-negotiable):
- [ ] SBOM clean (ScanCode + FOSSology)
- [ ] SPDX coverage 100%
- [ ] Auth framework wired (no Noop in prod)
- [ ] Sealed Cask build reproducible
- [ ] Hardening baseline (seccomp, caps, RO FS)
- [ ] Audit log on all adopter-facing endpoints
- [ ] Zero user-data access architecturally proven

### The Dark Tower (BlackMage — Attack Surface Per Wedge)
Each wedge inherits Unheaded's hardening but adds NEW attack surface:

| Wedge | New Attack Surface | Mitigation |
|-------|-------------------|------------|
| Anamnesis Lite | XDP program ingestion from adopter | BPF verifier + instruction budget gate (already in `scripts/bpf-verifier-check.sh`) |
| Mímir Drift | SSH/agent on adopter hosts | Read-only agent, scoped credentials, alerts-only proven |
| Zhen On-Prem | Air-gap claims must hold | Hard-fail egress test; PQ-signed model bundles |
| Gungnir Distribute | Customer artifact upload pipe | Sandbox extraction in UPC; Lich fuzz before GA |
| UPC Sandbox | Customer-supplied MBC bytecode | Verifier-as-gate; Anamnesis records every syscall |
| Sentinel Edge | Home-network telemetry | Local-only ingest; opt-in cloud aggregation |

**Lich campaign per wedge BEFORE GA**: each surface gets 72h of automated adversarial
hammering. Findings → MoatGhost → Architect harden → Developer patch → ship gate.

### The Watchtower (MoatGhost — Compliance Per Tool)
Tools sorted by community-adopter compliance need (= where free hardened tooling helps
the most people the fastest):

| Tool | Frameworks Adopter Wants Mapped | Evidence Already in Tree |
|------|--------------------------------|--------------------------|
| Mímir Drift | SOC2 CC7.1, HIPAA §164.312, PCI 11.5, NIST 800-53 SI-7, CIS 1.1 | YES — REAL-METAL validated zero false positive on EAST |
| Zhen On-Prem | FedRAMP Mod, ITAR, HIPAA, GDPR Art.32 | partial — air-gap proof needed |
| Gungnir Distribute | SLSA-3, NIST SSDF (SP 800-218), EO 14028 | YES — sealed-cask SHA256 binding rune |
| Anamnesis Lite | SOC2 CC7.2 (monitoring), PCI 10.1 | YES — eBPF audit trail |
| UPC Sandbox | NIST SP 800-190 (container), ISO 27001 A.14 | partial |

**Insight**: The tools with strongest compliance evidence ALSO unblock the most communities
that today get gated out of those frameworks by procurement / cost barriers. Lead with
Mímir → Gungnir → Zhen for community impact. All free. All GPL-3.0. All public on day one.

### The Sentinel Watches (Sentinel — Blue-Team Operational Reality)
Real-world deployment lessons from Pi-hole + IoT inventory + daily Lich loop:

- **Sentinel Edge** is *immediately* publishable to home labs / KGLW-style hobbyists who already run Pi-hole. Single binary, GPL-3.0, federated cloud aggregation is opt-in and self-hosted by community members.
- **Anamnesis Lite** must support *partial* deployment — adopters want packet-zero on ONE service before rolling it across all. Must work with their existing Prometheus / Loki / Grafana, not require Wotan adoption.
- **Mímir Drift** wins on the demo — "watch a config drift fire on a 4GB DDR3 box in 200ms." The bare-metal validation video is the entire pitch — and the pitch is "here, take it, it's yours."
- **Zhen On-Prem** wins on *what it refuses to do* — outbound network calls are blocked at the kernel level by BPF. That's a screenshot that earns trust, not a transaction.

### The Blueprint Reveals (Architect — Cross-Cutting Tooling Patterns)
Five tooling primitives that EVERY wedge needs and we should extract once:

1. **`unheaded-installer`** — single-binary bootstrap that pulls sealed-cask, verifies binding rune, configures auth, registers with control plane. Same tool ships with every wedge.
2. **`unheaded-tracegrep`** — CLI that takes a trace_id and walks every layer (XDP marker → Monad register → Wotan topic → service log → frontend trace). Demonstrates Anamnesis on its own.
3. **`mbc-disasm` / `mbc-asm`** — bytecode tooling. Free open-source. Drives UPC ecosystem.
4. **`gungnir-sign` / `gungnir-verify`** — standalone PQ signing CLI extracted from `pkg/gungnir/`. Free OSS — pairs naturally with Gungnir Distribute, also useful entirely on its own.
5. **`lich-runner`** — local red-team-in-a-box. Fully free; community-hosted reporting via federation.

Strategy: **everything OSS. Sharp edges and fabric both.** Each CLI is a gift; the
larger tool it slots into is also a gift. The funnel is "we trust the community,
the community trusts us back."

### The Hourglass Measures (Timeguru — Sequencing)
- **Now → 6 weeks**: extract Mímir, Anamnesis Lite, Zhen On-Prem to standalone `cmd/` targets w/ public README + LICENSE + CONTRIBUTING.md. Lich-fuzz each. Demo videos. Public GitHub orgs.
- **6 → 12 weeks**: Gungnir Distribute beta + Sentinel Edge alpha + UPC Sandbox dev preview, all public on day one.
- **12 → 24 weeks**: Lich-as-a-Service ethics review (Barrister) + Wotan Cloud (if IANA registry approved) + Rosetta IaC translator if community demand confirmed.

### The Goblet Toasts (Busboy — Cross-Skill Alignment)
Crew is aligned: Developer wants the extraction work (low net-new code, high reuse),
Architect wants the surface decomposition (clean modules), Micromanager wants the QA gate
discipline (every wedge same baseline), BlackMage wants the Lich campaigns
(every wedge fuzzed), MoatGhost wants the compliance maps (every wedge audited),
Sentinel wants real deployment feedback (every wedge dogfooded). Vibes: HIGH.

---

### Unified Battle Plan

#### Immediate Actions (Next 24-48 Hours)
- [ ] **Captain + Muck** — pick top 3 tools to publish first (recommend P0: Mímir / Anamnesis Lite / Zhen On-Prem)
- [ ] **Architect** — create `cmd/tools/` directory structure with one stub per chosen tool
- [ ] **Developer** — verify each P0 tool's source components compile as standalone build target (`go build ./cmd/tools/mimir/...`)
- [ ] **MoatGhost** — draft compliance evidence-pack per P0 tool, published as community runbook
- [ ] **BlackMage** — schedule 72h Lich campaigns against each P0 tool target

#### This Sprint (Next 7 Days)
- [ ] Extract Mímir Drift to standalone tool — Owner: Developer + Architect — Deadline: 2026-05-07
- [ ] Sealed-cask build pipeline produces 3 separate tool artifacts — Owner: Developer — Deadline: 2026-05-07
- [ ] Anamnesis Lite public README + CONTRIBUTING + LICENSE drafted — Owner: Captain + Librarian — Deadline: 2026-05-05
- [ ] Zhen On-Prem air-gap kernel-level egress block validated — Owner: Sentinel + BlackMage — Deadline: 2026-05-06
- [ ] First Lich campaign vs Mímir Drift complete — Owner: BlackMage — Deadline: 2026-05-07

#### Protocol Milestones (Carry Forward)
- [ ] Foundation-06 IANA registries land — already in flight, blocks Wotan Cloud
- [ ] Sophia-03 sub-dictionary types + QPACK — blocks UPC Sandbox bytecode distribution
- [ ] Wotan-03 error code taxonomy — blocks Wotan Cloud SLA contracts

#### UPC Compute Milestones
- [ ] Doom PC-corruption blocker resolved (`docs/doom/FINDINGS.md`) — unlocks UPC Sandbox demo
- [ ] MBC verifier hardened (post-`mbc-disasm` OSS release) — Lich must fuzz first
- [ ] WAVE13 BackwardScratch GPU-resident path — unblocks Zhen training velocity

#### Decisions Made at This Round Table
1. **DOCTRINE: free to use, free to share, no selling, ever.** Committed to CLAUDE.md.
2. **3 P0 tools (Mímir / Anamnesis Lite / Zhen On-Prem)** — chosen because compliance evidence + real-metal validation + already 80% in tree.
3. **Everything OSS — sharp CLIs and the larger tools.** No paid tier, no open-core split. GPL-3.0 for code, dual GPL-3.0/Apache-2.0 for protocol specs.
4. **Sealed Cask is the build substrate for ALL tools** — no parallel build systems.
5. **Each tool passes the 7-gate QA matrix before public release** — same hardening baseline as core.
6. **Lich campaign required pre-release for every tool** — security is shippable evidence.
7. **Tools DO NOT fork the monorepo** — `cmd/tools/<name>/` subset build only.

#### Open Questions (Carry to Next Round Table)
1. Federation / community-aggregation architecture for Sentinel Edge and Lich Runner — how do peers share findings without anyone owning the data? Architect + Captain.
2. Lich-as-a-Service ethics review — when does autonomous adversary cross legal lines, especially when running against community-shared infra? Barrister + BlackMage.
3. Zhen On-Prem inference — community hardware reference designs (Pi cluster, used Dell, Framework laptops). Architect + Sentinel.
4. Contribution onramp — DCO vs CLA, governance model, code-of-conduct baseline. Barrister + Captain.

#### Wins to Celebrate
- We have a planet. Most startups have an MVP. We have 34 services and frozen wire format.
- WAVE12 Kingdom RAFT shipped Δ −14.32 held-out eval — Zhen On-Prem has demonstrable IP.
- Mímir's Law REAL-METAL validated zero false positives on EAST — drift wedge is sales-ready.
- All P1 bugs fixed, ADR sweep complete, SBOM audited, GPL boundary clean — extraction is *clean*.
- 31 operational runbooks already exist — Zhen On-Prem ships with content on day one.

---

### Practical-Tooling Brainstorm Catalog (Full Surface Inventory)

Ranked harvest list for future Round Tables. P0/P1/P2 mapped above; everything else here:

**Developer / SRE Tools (free CLIs, gifts to the commons):**
- `unheaded-tracegrep` — single-cmd cross-layer trace walker
- `mbc-asm` / `mbc-disasm` — MBC bytecode toolchain
- `gungnir-sign` / `gungnir-verify` — PQ signing CLIs
- `lich-runner` — local autonomous adversary
- `enkrateia-watch` — alerts-only drift watcher (subset of Mímir)
- `sealed-cask` — deterministic build CLI (already in tree)
- `wotan-tail` — live tail across distributed message bus
- `bpf-budget` — verify eBPF program against instruction budget gate
- `champion-shell` — sandboxed file R/W + action log replay tool

**Blue-Team Appliances (free, federated, community-hosted):**
- Sentinel Edge (home/SMB Pi-hole++ w/ DGA/beacon detect)
- Mímir Drift (compliance drift alerts-only)
- Anamnesis Lite (packet-zero APM)
- Heimdall Gate (boot-time integrity check appliance)

**Red-Team Tools (gated, Barrister-reviewed, all free):**
- Lich-as-a-Service (continuous autonomous adversary, community federation)
- BlackMage Recon Suite (passive OSINT + perimeter map)
- Fuzz Pipeline (AFL/libFuzzer + Lich integration)

**Air-Gap / Regulated Communities (free, hardened, public source):**
- Zhen On-Prem (RAG appliance, air-gapped, PQ-signed model bundles)
- Gungnir Distribute (PQ-signed software supply chain bus)
- Sealed Cask Public Builders (reproducible builds, federation of build witnesses)

**Developer Platform / Niche:**
- UPC Sandbox (WASM competitor, deterministic replay)
- Wotan Cloud (Kafka/NATS competitor, PQ-signed)
- Rosetta IaC Translator (multi-backend desired-state)
- Sophia Schema Registry (BPF program + dictionary registry)

**Research-Grade (defer until protocol GA):**
- Distributed UPC (Doom across hosts via Monad transport)
- Anamnesis Forensics (full-replay incident response)
- WireGuard-over-Monad (encrypted overlay using IPv6 metric)

---

### Next Round Table
**Scheduled**: trigger when first P0 tool (Mímir Drift) completes Lich campaign + public
README/CONTRIBUTING/LICENSE + sealed-cask reproducible build (target: 2026-05-08).
**Reason**: Decide public release sequencing + community onramp for the first free tool.

---

_Forged at the Round Table by the full council. Six guides leaning forward: Developer
sharpening tools, Architect mapping surfaces, Micromanager guarding gates, BlackMage
hammering edges, MoatGhost mapping compliance evidence, Sentinel feeding ground truth.
The planet has gravity. Time to give it to the commons. The Kingdom marches as one._

**FREE TO USE. FREE TO SHARE. NO SELLING.**
LOVE SERVE REMEMBER. PEACE AND LOVE. KGLW. Let's get crazy. Let's go nuts. <3
