# Demo Video Script — v0 outline

> **Conditional on Track B or Track C.** This outline activates when `docs/decisions/2026-04-29-track-call.md` locks Track B (launch-first) or Track C (twin-track). If Track A is locked, this outline is deferred to Age 4+ and may be revised before resumption.

**Drafted**: 2026-04-27 from Cowork-on-Macbook (Captain + Micromanager hats)
**Target length**: 5–8 minutes
**Audience**: VC partners (primary), prospective customers (secondary), open-source community (tertiary)
**Tone**: confident-not-cocky, technical-not-jargon-laden, KGLW-energy-restrained
**Owner**: Captain (script) + Stevie (talent, recording) + Micromanager (acceptance gate)

---

## North star

In 5–8 minutes, a viewer who has never heard of Unheaded should leave able to answer:

1. **What is it?** "Configuration management automation that delivers production infrastructure in hours, not months."
2. **Why does it matter?** "Modern infra is bolted-together tools with sampled observability and scattered security. Unheaded is unified — packets carry their own traces, eBPF enforces policy at kernel speed, and the protocol IS the moat."
3. **Why now?** "eBPF is the inflection point. IPv6 extension headers are the medium. The Wild West of post-AI-boom tooling demands one platform, not twelve sidecars."
4. **What can I see today?** "Self-hosting proof — Unheaded building Unheaded on bare metal. Live dashboard. Real eBPF traces. Real Kanban. Real timeline."

Viewer leaves with three feelings: *this is real, this is different, this is buildable*.

---

## Scene-by-scene outline

### Scene 1 — Cold open (0:00–0:30)

**Visual**: Terminal split-screen. Left: `git log` showing recent Unheaded commits scrolling. Right: dashboard at `https://localhost:20000` showing live packet flow graph + system metrics + host selector dropdown (WEST/EAST).

**Voiceover** (Stevie): *"This is Unheaded. Every commit you see on the left is building the platform on the right. Self-hosting. Live. Right now."*

**Cut to title card**: `UNHEADED — Production infra in hours, observable from packet zero.`

### Scene 2 — The problem (0:30–1:30)

**Visual**: Stock footage / animation of a typical modern stack — sidecars, service mesh diagrams, observability agents, security scanners — getting visually busy and tangled.

**Voiceover**:
> *"Modern infrastructure is broken. You bolt on a sidecar for observability. Another for security. A third for traces. A fourth for policy. Each one adds latency. Each one adds attack surface. Each one demands its own toolchain. And the data is sampled, delayed, and disconnected from the packets that produced it.*
>
> *DevOps becomes a career field. Security reviews take weeks. The first packet of every flow is a black box."*

### Scene 3 — The technical bet (1:30–2:30)

**Visual**: Diagram from `docs/SYSTEM_DIAGRAM.md` — 6-layer architecture, packet flow with eBPF markers at the XDP layer, IPv6 hop-by-hop extension headers carrying trace IDs.

**Voiceover**:
> *"Unheaded's bet is simple: eBPF + IPv6 extension headers + gRPC. Packets carry their own traces. Kernel-space observability. Kernel-space policy enforcement. Sub-microsecond latency.*
>
> *The protocol is the moat. Monad on the wire. Sophia in the BPF maps. Wotan in the message bus. Three specs, twelve IANA registries, IPR clear."*

### Scene 4 — Live demo: self-hosting (2:30–4:30)

**Visual**: Real screen recording on WEST or EAST bare metal.

**Voiceover narrates each step**:
1. *"Here's the dashboard. Live packet flow graph. Real traces from real packets. No sampling."* — show packet flow visualization with ~10 services communicating.
2. *"Drop down to host selector — switch from WEST to EAST. BPF flow graph crosses hosts seamlessly. The mesh is the protocol."*
3. *"Open the Kanban board. Real tasks. Real status. Tracked by Git commit. Audit trail built in."* — show kanban with Review column + task detail modal.
4. *"And here's `timeline.md`. Living roadmap. Drift policy: max 7 days from HEAD."* — show timeline.md, then drift-guard CI passing.
5. *"Every service is running on bare metal. Every packet carries a trace. Every commit updates the timeline. Self-hosting proof."*

### Scene 5 — Live demo: how fast (4:30–5:30)

**Visual**: Sub-50ms latency benchmark running. Or: time-lapse of `make deploy` provisioning a fresh host (if benchmark recording is feasible).

**Voiceover**:
> *"From `nixos-rebuild` to passing health checks across N nodes — under 90 minutes. Production-grade hardening. eBPF-enforced policies. Container runtime agnostic — LXD, containerd, NixOS, Docker, all interchangeable.*
>
> *Six IaC backends supported as drop-in: Ansible, Terraform, Puppet, Kubernetes, Chef, Salt. Eight observability backends pluggable. Zero lock-in. Your tools, our data model."*

### Scene 6 — Why now (5:30–6:30)

**Visual**: Stevie on camera (or animated stand-in), warm tone.

**Voiceover** (or on-camera):
> *"It's the Wild West. Post-AI-boom, every team is going to need infra that scales without scaling the team. Unheaded delivers that — built in the open, GPL-3.0, protocol specs dual-licensed for ecosystem adoption.*
>
> *We're shipping the alpha. Public repo at github.com/unheaded/unheaded. Documentation in tree. Round Tables on the wiki. Come build with us."*

### Scene 7 — Call to action (6:30–7:00)

**Visual**: URLs prominently displayed.

**Text + voiceover**:
- `github.com/unheaded/unheaded` — code
- `unheaded.dev` (or final domain) — website
- `unheaded/wiki` — docs
- `unheaded@bellis.tech` — partnerships

**Voiceover**: *"Production infra in hours. Observable from packet zero. The protocol is the moat. Let's build."*

**Cut to outro card**: Unheaded logo + KGLW one-liner if Stevie is feeling it.

---

## Production checklist

### Pre-recording
- [ ] WEST or EAST in known-good state (all services healthy, dashboard live, kanban populated, timeline fresh)
- [ ] Recording rig: 1080p minimum, 30fps, system audio + voiceover separate tracks
- [ ] Slide deck for Scenes 2–3 + 6 (animations or static — minimal text)
- [ ] Script rehearsed twice to lock pacing
- [ ] Sub-50ms latency benchmark HAS RUN (Scene 5 needs the data)
- [ ] Pre-public scrub on main verified (per branch audit summary — gating)

### Recording
- [ ] Record demo screen captures FIRST (tightest), voiceover SECOND
- [ ] 3 takes minimum on Scenes 4 + 5 (live demo)
- [ ] Capture 30s of "B-roll" — terminal scroll, dashboard idle, packet flow animation — for editor padding

### Post-production
- [ ] Editor: Captain or contracted (budget under $1k for v0)
- [ ] Captions auto-generated, manually corrected for technical terms
- [ ] Length verification: 5–8 min hard cap
- [ ] Pre-publish review: Captain + Micromanager + one external partner
- [ ] Hosted on YouTube unlisted first, public after VC dry-run

### Acceptance gates (Micromanager)
- [ ] Viewer-test: pick 3 people who don't know Unheaded, ask them the 4 north-star questions after watching once
- [ ] At least 3/4 questions answered correctly by ≥2/3 viewers → ACCEPTED
- [ ] If <2/3 viewers answer 3/4 → re-script + re-record specific scenes

---

## Risks

| Risk | Mitigation |
|---|---|
| Live demo crashes on recording day | Record from a known-good local replay if WEST/EAST is unstable; tag the replay in the demo notes |
| Scene 5 benchmark doesn't hit sub-50ms | Honest framing: "under 200ms today, sub-50ms in roadmap" — falsifiability beats spin |
| Stevie's voice/energy not in form | Schedule recording for KGLW-listening morning; coffee; dogs nearby; no AFK pressure |
| Pre-public scrub blocking | Demo is RECORDED on private repo state — public push of repo blocks separately on scrub completion |
| Scope creep ("just add one more demo") | Hard cap: 7-minute final cut. Editor enforces. Marshal cites. |

---

## When this outline is finalized

After Track B or Track C is locked:

1. Captain converts this outline into a **shot-by-shot script** (line per spoken word + visual cue per second)
2. Schedule recording session via `references/2026/MM/DD/reference.md`
3. Promote outline → `docs/launch/2026-MM-DD-demo-script-v1.md` with shot-list
4. Add Lane I1 to `battle-plan.md` as P0 (per Track-B/C propagation checklist)

---

*Demo script v0 forged 2026-04-27 from Cowork-on-Macbook. Activates on Track B or C lock.*
*<3 KGLW <3 — keep the energy real, the demo honest, the call to action clear.*
