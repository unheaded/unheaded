# ADR-043: Mímir's Law — OS Baseline Delivery, Drift Detection, and Self-Healing via UPC (Gleipnir Phase 0 PoC)

## Status: PoC / Research (Round Table v2 — 2026-04-08)

## Date: 2026-04-08

## Decision Makers
- Stevie Bellis (Principal)
- Round Table v2 (full 19-seat council)
- Deep consultations: BlackMage (attack surface), Scientist (hypothesis design), Lore (mythology and naming)
- Four-skill template subcommittee: BlackMage / Architect / Scientist / Developer
- Cross-references: Captain (strategy), Computermancer (Dream Ladder framing), Architect (two-plane separation), Marshal (lane enforcement)

---

## Context

### The Question

This ADR exists to answer a specific question that emerged from a Round Table session on 2026-04-08: *"We have an idea for push-state configuration via the UPC bus — is it any good, is it different from what already exists, is it worth pursuing or redundant waste?"*

### The Three Framings

The proposal evolved through three distinct framings before the Round Table reached verdict:

**v1 — Product framing (initial pitch)**: A CA-signed push-state configuration system using protobuf over a UPC bus, multicast delivery, eBPF "config receivers," ML-DSA-65 signatures. *"No SSH, no agents, PQ-signed at wire level."*

**v2 — PoC framing (Stevie's reframe)**: *"Not a viable product or solution but a PoC like running Doom or Linux on the UPC."* This **inverted** the evaluation framework — comparison with incumbents (Ansible, Terraform, Cilium, Flannel) became irrelevant. Success metric is "UPC can do this thing," not "UPC does this thing better."

**v3 — Yggdrasil integration (Stevie's expansion)**: *"Could be baked into our Unheaded image, baseline controllable via UPC/UP."* This connected the proposal directly to ADR-69420 (Yggdrasil hardened-Debian soft-fork pipe-dream OS) and to **Gleipnir** — a config-convergence component already named and accepted in ADR-69420 for Age 2b.

### The Critical Discovery

ADR-69420 already names **Gleipnir** as the config convergence component, scheduled for Age 2b (Q4 2026), with scope: *"canonical config schema converters, runs as Jenkins build step in the Yggdrasil image pipeline, ensures all baked-in configs match canonical source."* The proposed PoC is best understood as **Gleipnir Phase 0** — an early dogfood prototype that lands a year ahead of plan, dressed in the UPC dogfooding clothes that fit the Dream Ladder narrative alongside Doom-on-UPC and Linux-on-UPC.

### Round Table v1 Verdict (Product Framing)

Round Table v1 unanimously **REJECTED** the v1 product pitch as redundant with Ansible/Puppet/Cilium/Vault, with multiple structural objections:
- Multicast as transport for ongoing config delivery is a fatal flaw (does not route across internet, poor cloud support, UDP unreliability)
- "BPF as config agent" is a category error — BPF cannot perform general system state mutation; a userspace helper is mandatory
- The "no agent" claim collapses on inspection — the userspace helper IS the agent
- Sacred Law violation if it replaces the existing IaC backend doctrine in CLAUDE.md §3.3

### Round Table v2 Verdict (PoC Framing + Yggdrasil Integration)

Under v2 reframing, four seats **FLIPPED** their verdict:
- **Captain**: from "not on critical path" → "PoC dogfood ahead of Gleipnir Age 2b is genuine de-risking value"
- **Architect**: from "second config plane creates drift hell" → "no second config plane — coexists with existing reconciliation"
- **Computermancer**: from "control plane idea dressed as compute" → "horizontal Dream Ladder milestone like Doom-on-UPC"
- **Marshal**: from "scope-creep risk across 5 lanes" → "scope is sharply bounded by Sacred Law clause"

The remaining 15 seats' technical objections become **PoC constraints** rather than reasons to reject. Three deep consultations followed:

**Scientist verdict**: PURSUE. Confidence in feasibility 87%, in 2-week timeline 82%, in informative pass/fail result 95%. Reversible (cost of reversal < 1 day). ~1000 LOC net-new with zero new dependencies. Hypothesis is falsifiable in 2 weeks.

**BlackMage verdict**: PURSUE WITH SCOPE-NARROW. v1 must be **alerts-only** (no auto-restore until LICH-012 campaign clears). Wotan messages on `config.*` topics MUST be ML-DSA-65 signed BEFORE auto-restore. Baseline must be immutable via dm-verity. Quarterly key rotation ceremony. Otherwise security theater.

**Lore verdict**: PURSUE. The mythology slots cleanly into the existing Norse pillar (Yggdrasil already named there) without creating a fourth pillar. Zero Sacred Law violations.

### Apache Airflow Comparison (asked during deliberation)

Minimal overlap. Airflow is workflow orchestration (DAG-based, scheduled tasks, dependency graphs — answers *"in what order do these steps run?"*). This PoC is state delivery (answers *"what should the system look like?"*). Airflow could schedule a UPC delta delivery DAG, but it does not deliver state itself. The closest Unheaded analog to Airflow is Akira's runbook engine (ADR-024 Runbook Automation), not this PoC.

### What Is Genuinely Novel Here?

The PoC's value is NOT in being a better Ansible/Puppet/Cilium. The value is:

1. **Dogfooding the UPC stack** for one more system-level workload (alongside Doom-on-UPC compute and Linux-on-UPC boot)
2. **De-risking Gleipnir's eventual Age 2b sprint** by surfacing the hardened-baseline + drift-detection learnings now, on a controlled spike, instead of during the production Gleipnir build
3. **Demonstrating two-plane architectural separation** between Wotan steady-state verification (existing role) and discrete UPC trigger packets (the Gjallarhorn pattern — see Decision section)
4. **PQ-signed wire-level baseline integrity** as a research vehicle for FedRAMP/STIG/PCI-DSS compliance positioning (aligns with ADR-69420's stated SELinux-on-Debian compliance angle)
5. **Forcing the Wotan `config.*` topic signing prerequisite** to be implemented now — a security improvement that benefits the entire steady-state Wotan plane regardless of PoC outcome

---

## Decision

### The Verdict

**PURSUE** as a 2-week PoC spike on WEST, scoped as **Mímir's Law / Gleipnir Phase 0 dogfood**, with eight non-negotiable hard conditions (below). Spike runs in branch `spike/mimirs-law` with no touch to `main` IaC backend code or to the frozen Monad v0x01 wire format. PoC explicitly **coexists** with Ansible/Puppet/Terraform per `CLAUDE.md` §3.3 — does NOT replace.

### The Two-Plane Architecture

A critical clarification surfaced mid-Round-Table: **Wotan stays in its existing role**, and **UPC trigger packets are a distinct discrete-event plane**.

**Wotan plane (steady-state, existing)**:
- gRPC pub/sub on port 18001
- Continuous verification, drift events, audit trail, ongoing reconciliation
- ML-DSA-65 signed messages on `config.*` topics (BlackMage prerequisite — must be implemented BEFORE spike begins)
- Use for: ongoing observability, drift detection events

**Gjallarhorn plane (discrete UPC triggers, new)**:
- Specially-formed Monad-wire packets carrying one-shot signals
- **Unicast** for single-node actions (*"re-verify your baseline now"*)
- **Multicast** for segment-wide bootstrap (*"freshly planted seeds, here is your baseline pointer, install yourself"*)
- This is the PXE-boot/DHCP/WoL pattern evolved into a unified UPC primitive

**The multicast objection from Round Table v1 is now scope-bounded**: multicast remains a fatal flaw for *global ongoing config delivery* (still use Wotan gRPC for that). It is the *appropriate tool* for *local-segment seed provisioning* (DHCP/PXE have used multicast for 30 years for exactly this).

### Two Distinct Flows the PoC Must Demonstrate

1. **Bootstrap flow (Gjallarhorn → Heimdall → Mjölnir)**: Fresh seed/node boots → segment multicast Gjallarhorn discovery → packet payload tells node *"you are part of cluster X, here is your Mjölnir manifest pointer + Gungnir Seal"* → node fetches and installs baseline → joins Wotan steady-state plane.

2. **Reminder/re-verification flow (Gjallarhorn unicast → Heimdall → Wotan)**: Authority sends unicast Gjallarhorn packet to specific node → Heimdall daemon receives → re-verifies state against Mjölnir → publishes drift events to Wotan topic → Enkrateia processes (alerts only in v1).

### Naming (Lore final recommendation)

| Component | Name | Origin | Path |
|---|---|---|---|
| Baseline definition file | **Mjölnir** | Norse — Thor's hammer, the foundational artifact, the law | `references/baseline/mjolnir.yaml`, `mjolnir.manifest.json` |
| Signed delta payload | **Gungnir Seal** | Norse — Odin's spear, never misses | `*.gungnir.sig` |
| Discrete UPC trigger packet | **Gjallarhorn** | Norse — Heimdall's horn that wakes the seeds | `pkg/gjallarhorn/` |
| Drift-detection daemon | **Heimdall Daemon** | Norse — eternal watchman of Bifrost | `cmd/heimdall-daemon/`, `crates/heimdall-bpf/` |
| Restoration mechanism (alerts-only v1) | **Enkrateia** | Gnostic ἐγκράτεια ("self-control") — verb form of Pleroma↔Kenoma reconciliation | `pkg/enkrateia/` |
| Convergence component (parent vision) | **Gleipnir** | Norse — already in ADR-69420 | (not built in PoC; this PoC is its Phase 0) |
| Authoritative speaker of baseline | **Mímir** | Norse — wise head, the rememberer, source of authority | (conceptual role, not a code path) |

### Eight Non-Negotiable Hard Conditions

1. **Alerts-only v1.** NO auto-restore. Drift events publish to Wotan `drift.detected` topic; human review required to act. Auto-restore deferred to v2 contingent on LICH-012 clearance.

2. **Wotan message signing prerequisite.** Before the spike begins, ALL Wotan messages on `config.*` topics MUST be ML-DSA-65 signed and verified. If this prerequisite is not met, the spike does not start. *(BlackMage Q3 hard condition.)*

3. **Baseline immutability via dm-verity** or equivalent read-only mount. Restore must require explicit signed ceremony, not silent overlay.

4. **Key separation.** ML-DSA-65 signing key for baseline lives in HSM-equivalent storage (TPM, sealed, or filesystem with strict ACL + audit log). NEVER in plain Wotan filesystem. Quarterly rotation ceremony documented BEFORE spike starts.

5. **Semantic-aware drift detection** is REQUIRED before any auto-restore version. v1 alerts on byte-level drift; auto-restore version must do YAML/key-order semantic diff in userspace. *(BlackMage Q2 hard condition — addresses the developer-reorders-keys-causes-oscillation attack.)*

6. **Sacred Law clause.** PoC may NOT touch `pkg/discovery/`, `pkg/transport/`, `cmd/unheaded-daemon/`, or any IaC backend code on `main`. Spike branch only. Must coexist with Ansible/Puppet/Terraform per `CLAUDE.md` §3.3.

7. **No Monad wire format changes.** Payload is a Wotan topic protobuf or a Gjallarhorn packet that fits within frozen Monad v0x01 (likely a Kingdom Mode + flow action combination). RFC Editor enforces.

8. **LICH-012 Configuration Convergence Attack** campaign opened in parallel with the spike. Findings feed into v2 gating. Campaign sub-tracks: L12a baseline signing attack, L12b Wotan message injection, L12c restore race conditions, L12d eBPF drift detection fuzzing.

### Hard Kill Criteria K1–K4 (any one triggers immediate spike abort)

- **K1**: eBPF verifier rejects Heimdall drift hooks (>50% of opcodes fail verification after optimization attempts). Fallback would be userspace polling = lose UPC efficiency claim.
- **K2**: Wotan gRPC convergence time exceeds 15s p95 (or 30s p99) on the 10-node WEST cluster. Restore becomes meaningless because applications crash first.
- **K3**: Cascade failures, unplanned reboots, or data corruption on the restore phase (zero tolerance). Restore creates worse failure mode than the drift it solves.
- **K4**: Any of the 8 hard conditions above cannot be met before the spike begins (e.g., Wotan signing prerequisite cannot be implemented in time).

### Pass Criteria (all must hold at day 14 to promote)

- Convergence time < 10s p95 on WEST 10-node cluster
- Drift detection latency < 1s
- Restore verification (alerts only) ≥ 95% accuracy
- Data consistency 100% across nodes after benign delta application
- Zero cascade failures or unplanned reboots
- All 8 hard conditions still in force
- **Bootstrap flow** demonstrated end-to-end (multicast Gjallarhorn → fresh seed → Mjölnir install → Wotan plane join)
- **Reminder flow** demonstrated end-to-end (unicast Gjallarhorn → Heimdall re-verify → Wotan drift event)
- LICH-012 adversarial review finds zero exploitable issues

### Falsifiable Hypothesis (Scientist's framing)

> *"We believe UPC (Monad wire + Sophia microcode + Wotan memory + eBPF datapath) can reliably deliver ML-DSA-65-signed configuration deltas to an Unheaded OS baseline because UPC executes deterministic computations (Doom proves correctness), Sophia's microcode is semantically equivalent to a deterministic configuration engine, Wotan's distributed memory provides testable message transport, and circl provides cryptographic assurance. We predict that within 2 weeks a 10-node WEST cluster running a hardened-Debian-12 baseline will: (1) start at known-good baseline, (2) accept and apply signed deltas via Wotan gRPC, (3) detect drift via eBPF, (4) alert (v1, no auto-restore) within p95 < 10s with > 99% stability, AND (5) respond to discrete Gjallarhorn UPC trigger packets — unicast for re-verification, multicast for fresh-seed bootstrap."*

### Architecture & Component Map

```
unheaded/
├── docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md  (this ADR)
├── docs/battle-plans/BATTLE-PLAN-MIMIRS-LAW-GLEIPNIR-PHASE-0.md  (warmonger plan)
├── references/baseline/
│   ├── mjolnir.yaml                                  (canonical desired baseline, declarative)
│   └── mjolnir.manifest.json                         (path → SHA256 + mode + owner)
├── pkg/gungnir/                                      (ML-DSA-65 sign/verify wrappers, ~150 LOC Go)
├── pkg/gjallarhorn/                                  (UPC trigger packet sender/receiver, unicast + multicast, ~250 LOC Go)
├── pkg/enkrateia/                                    (alerts-only v1 — drift event aggregation, ~150 LOC Go)
├── cmd/heimdall-daemon/                              (eBPF drift watcher userspace control + Gjallarhorn handler, ~250 LOC Go)
├── crates/heimdall-bpf/                              (Aya eBPF: vfs_write/execve/mmap drift hooks + XDP Gjallarhorn listener, ~500 LOC Rust)
└── tomb/lich/LICH-012-config-convergence/            (BlackMage red team campaign, parallel track)
```

**Net-new code estimate**: ~1300 LOC across Go and Rust. Zero new external dependencies.

### Reuse vs. Net-New (Scientist's first-principles count)

| Component | Reuse | Net-new |
|---|---|---|
| Baseline | Yggdrasil image (or current hardened Debian 12 as proxy from ADR-69420) | JSON manifest generator |
| Delta delivery | Wotan gRPC + cloudflare/circl ML-DSA-65 (both already vetted) | Topic publisher/subscriber + Gungnir Seal protobuf |
| Drift detection | Aya framework (ADR-003) | Syscall hooks + ringbuf publisher |
| UPC trigger packets | Monad v0x01 frozen wire format | Gjallarhorn packet shape (within existing Kingdom Mode + flow action registry — RFC Editor confirms no wire change) |
| Restore (v2 ONLY — not in v1) | overlayfs / dm-snapshot kernel features | Restore orchestrator (DEFERRED to v2) |
| Adversarial review | LICH framework (ADR-042) | LICH-012 campaign README + fuzz harness |

### The Lore Narrative (the *Why* in story form)

*In the Age of Sand and Wire, when Yggdrasil first took root in silicon, the Kingdom discovered: an OS that knows itself is an OS that cannot break. Mímir spoke the First Law — the baseline, written and sealed by Gungnir. Heimdall began his eternal watch from Bifrost. When Kenoma drifted from Pleroma, he raised Gjallarhorn — and the seeds awoke. When the horn sounded for a freshly planted node, that node received its purpose; when the horn sounded for an old node, that node remembered its baseline and reported its truth. Anamnesis recorded every drift, every signal, every act of remembrance. Then came Enkrateia — self-healing — pulling Kenoma back into alignment with Pleroma's will. No human hand needed. The OS spoke its own truth, audited itself, prepared to correct itself. This is the Dream Ladder's horizontal proof: UPC is not just a network abstraction. UPC is the nervous system of reality itself.*

---

## Consequences

### Positive

- **Dogfoods the UPC stack** for OS-level operations, extending the Dream Ladder horizontal milestone family alongside Doom-on-UPC and Linux-on-UPC
- **De-risks Gleipnir Age 2b sprint** by landing a controlled prototype a year ahead of plan with explicit kill criteria
- **Validates two-plane architectural separation** (Wotan steady-state vs Gjallarhorn discrete triggers) — useful for many future Unheaded features
- **PQ-signed baseline as compliance research vehicle** — aligns with ADR-69420's SELinux-on-Debian FedRAMP/STIG/PCI-DSS positioning
- **Generates LICH-012 campaign findings** on a novel attack surface BEFORE it ships in production
- **Forces Wotan `config.*` topic signing prerequisite** to be implemented now — a security improvement that benefits the entire steady-state Wotan plane regardless of PoC outcome
- **Reuses existing infrastructure** (Wotan gRPC, Aya eBPF stack, cloudflare/circl ML-DSA-65, ADR-001 Pleroma/Kenoma state model) — zero new external dependencies
- **Reversible**: cost of reversal < 1 day; spike branch never touches main control plane code

### Negative

- **Engineering velocity cost**: ~1 developer-week + ~0.5 BlackMage-week + ~0.25 each Architect/Scientist/Lore weeks during the spike window
- **Net-new attack surface** (eBPF observation, Gjallarhorn packet forgery, baseline restore TOCTOU, Wotan topic injection if signing prereq slips) — mitigated by alerts-only v1 + LICH-012 campaign
- **Risk of scope creep** if any seat tries to extend beyond alerts-only v1 mid-spike — Marshal enforcement required
- **Risk of mythology-name proliferation** (now adding Mjölnir/Gungnir/Heimdall/Gjallarhorn/Enkrateia/Mímir to the Norse + Gnostic pillars in one ADR) — Lore confirms zero Sacred Law violations but reviewers should verify
- **Pre-existing Wotan unsigned-message gap**: signing prerequisite (hard condition #2) blocks spike start. If implementing Wotan signing turns out larger than expected, the entire spike is delayed.

### Mitigations

- **Marshal lane enforcement** during spike: no PR may touch `pkg/discovery/`, `pkg/transport/`, `cmd/unheaded-daemon/` on main
- **Day-14 hard gate**: Scientist makes the call to promote or flip-to-rejected based on K1–K4 + 8 hard conditions; Marshal enforces the gate
- **LICH-012 runs in parallel**: BlackMage's adversarial review is not afterthought — it's a co-equal track to the implementation work
- **The PoC is alerts-only** — no auto-restore in v1 means even if the entire stack is compromised, the worst outcome is bad alerts, not destructive auto-actions
- **Documented kill protocol**: if PoC fails, ADR-043 is flipped to "Rejected with learnings" and learnings are explicitly captured for the future Gleipnir Age 2b sprint

---

## Alternatives Considered

### Alternative A — Kill outright (original Round Table v1 verdict, REJECTED in v2)

Round Table v1 unanimously recommended killing the v1 product-frame pitch as redundant with Ansible/Puppet/Cilium/Vault. This recommendation was the right call for the v1 framing but was overturned by the v2 PoC reframe and v3 Yggdrasil integration. Preserved here as historical context: the Round Table caught the original idea before a line of code was written, and the reframing process produced a substantively different (and more valuable) proposal.

### Alternative B — Defer to Q4 2026 alongside Yggdrasil/Gleipnir

Schedule the PoC concurrently with the production Gleipnir Age 2b sprint instead of running it now as a Phase 0 dogfood. **Rejected** because the entire value of "Phase 0" is to surface learnings BEFORE the production sprint commits — running them concurrently defeats the de-risking purpose.

### Alternative C — Build the v1 product (original pitch with multicast + BPF + "no agent")

**Rejected** by Round Table v1 as redundant with existing tools, with the multicast transport being a fatal flaw for global config delivery (not just inferior, but actively wrong for cloud / WAN environments). Sacred Law violation if it claimed to replace the IaC backend interchangeability doctrine.

### Alternative D — Skip ADR template scaffold (FOUR-SKILL CONSENSUS)

Stevie delegated a meta-question to BlackMage / Architect / Scientist / Developer: should we formalize an ADR template scaffold file (`docs/templates/adr-template.md`) given that 43 ADRs have been written without one? Verdict tally:

| Skill | Vote | Reasoning |
|---|---|---|
| BlackMage | BUNDLE | Wants enforced "Security Considerations" section on every ADR |
| Architect | SPLIT | Templates and decisions are different scopes; clean history |
| Scientist | **SKIP** | "43 ADRs is N=43 evidence the prose pattern works. Zero observed drift. Adding a template now is preemptive abstraction for a problem that has not occurred." |
| Developer | **SKIP** | "Three similar lines of code is better than a premature abstraction. 43 similar ADRs are 43× better evidence." |

**Consensus (2 SKIP / 1 SPLIT / 1 BUNDLE)**: SKIP. BlackMage's enforced-security-section concern is preserved by adding a Librarian review-checklist item instead of a template scaffold. Revisit at ADR-050 if any prose drift becomes observable across the next 7 ADRs.

---

## References

### Related ADRs
- **ADR-69420** — Sleipnir + Yggdrasil + Gleipnir vision; this PoC is **Gleipnir Phase 0**
- **ADR-001** — Gnostic State Management (Pleroma/Kenoma/Anamnesis) — Enkrateia slots into this trinity as the verb form
- **ADR-002** — Kingdom Naming Convention — Norse pillar extension authorized
- **ADR-003** — eBPF in Rust with Aya — toolchain for Heimdall BPF programs
- **ADR-004** — Dependency Policy — zero new dependencies for spike
- **ADR-012** — BPF Verifier Risk Mitigation — applies to Heimdall hooks
- **ADR-024** — Zhen Runbook Automation — Akira runbook engine, the closest existing analog to Apache Airflow
- **ADR-034** — gRPC mTLS Default Transport — coexistence requirement, not replacement
- **ADR-042** — CS BlackMage + Lich integration — LICH campaign infrastructure (parent of LICH-012)

### Protocol Drafts
- `docs/protocol/draft-bellis-unheaded-foundation-06.md` — Monad v0x01 (FROZEN, must not touch)
- `docs/protocol/draft-bellis-unheaded-wotan-memory-03.md` — Wotan topic payload semantics
- `docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md` — ML-DSA / SLH-DSA primitives

### Lore Documents (to be updated as part of I.4 / I.5 actions)
- `docs/lore/NORSE_MYTHOLOGY.md` — Mímir, Mjölnir, Gungnir, Heimdall, Gjallarhorn entries
- `docs/lore/GNOSTIC_ARCHITECTURE.md` — Enkrateia entry

### Battle Plan
- `docs/battle-plans/BATTLE-PLAN-MIMIRS-LAW-GLEIPNIR-PHASE-0.md` — warmonger-format spike execution plan with numbered steps, exit gates, agent matrix, and emergency procedures

### Round Table Working Document
- `/home/govan/.claude/plans/merry-sprouting-hedgehog.md` — full Round Table v1 + v2 deliberation transcripts, four-skill consultations, BlackMage attack surface inventory, Scientist experiment design, Lore mythology framing
