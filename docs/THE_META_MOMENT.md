# The Meta Moment

**"We drink our own champagne."** 🍾

## What is The Meta Moment?

The Meta Moment is Unheaded's ultimate proof of concept: **using Unheaded to build and host Unheaded itself**.

Specifically, the Kanban app that displays our development progress is:
1. Hosted on Unheaded infrastructure
2. Reading from our timeline (timeguru service)
3. Traced by our eBPF packet markers
4. Visualized in our custom dashboard
5. Accessible from the public internet

**If Unheaded can reliably manage its own development infrastructure, it proves the platform works for ANY customer workload.**

## The Recursion

```
Unheaded Alpha
  ├── Manages: LXD containers (NixOS)
  ├── Traces: Every packet with eBPF
  ├── Monitors: All services via Wotan
  └── Hosts: Kanban app showing... itself building itself 🔄
```

This is recursion as proof:
- The infrastructure manages itself
- The observability observes itself
- The dashboard displays its own construction
- The automation automates its own deployment

## Why This Matters

### For Customers
If we can trust Unheaded to host our own mission-critical development dashboard, customers can trust it with their applications.

### For Us (The Team)
- **Dogfooding:** We experience every pain point our customers will
- **Rapid iteration:** Breaking our own stuff = fast fixes
- **Credibility:** "Built by Unheaded, running on Unheaded"
- **Quality bar:** We demand perfection because we depend on it

### For The Product
- **Integration testing:** Every feature tested in production (our own)
- **Performance validation:** Real workload, not synthetic
- **Security hardening:** Our data at stake = serious security
- **Operational excellence:** We own the pager

## The Kanban App

### What It Does

Displays our project timeline as an interactive Kanban board:

```
┌──────────────┬──────────────┬──────────────┐
│    TODO      │ IN PROGRESS  │     DONE     │
├──────────────┼──────────────┼──────────────┤
│ Milestone    │ eBPF         │ Wotan       │
│ 1.3          │ Foundation   │ Phase 1      │
│              │              │              │
│ Microserv.   │ Setup Script │ Architecture │
│              │              │ Docs         │
│ Milestone    │              │              │
│ 1.4          │              │ Timeline     │
│              │              │              │
│ Container    │              │              │
│ Stack        │              │              │
└──────────────┴──────────────┴──────────────┘
```

### How It Works

1. **Data Source:** Reads `references/timeline.md` from timeguru service
2. **Parsing:** Converts markdown to structured JSON
3. **Rendering:** Vanilla JavaScript, canvas-based (bellis.tech inspired)
4. **Updates:** WebSocket for real-time changes
5. **Tracing:** Every request traced by eBPF, visible in dashboard

### Tech Stack

- **Backend:** Go (kanban-app service)
- **Frontend:** Vanilla JS + Canvas API
- **Data:** timeline.md (source) → timeline.json (API) → timeline.yaml (config)
- **Transport:** HTTP/3 (gateway) → HTTP (kanban-app) → HTTP (timeguru)
- **Observability:** Full eBPF tracing, Wotan pub/sub

### Design

Inspired by bellis.tech:
- Dark theme with particle canvas background
- Floating stars animation
- Clean, minimal typography
- Real-time "live" indicator
- Header: **"Unheaded Alpha - Built by Unheaded 🔄"**

## The Data Flow (Traced)

```
User's Browser
    ↓ [eBPF: trace_id = abc123]
HTTPS/HTTP3 to gateway (10.10.10.100:443)
    ↓ [eBPF: correlation captured]
HTTP to kanban-app (10.10.10.200:8001)
    ↓ [eBPF: service-to-service trace]
HTTP REST to timeguru (10.10.10.20:8000)
    ↓ [File I/O traced by kprobe]
Read /opt/unheaded/references/timeline.md
    ↓ [Parse markdown → JSON]
Return timeline JSON
    ↓ [eBPF: response traced]
Render Kanban board in browser
    ↓ [WebSocket update]
Dashboard shows packet journey: 6 hops, 47ms total
```

**Every step traced, correlated, and visualized in real-time.**

## The Philosophy

### 1. Self-Reliance
We don't depend on external platforms for our own infrastructure. If Unheaded goes down, we feel the pain immediately.

### 2. Transparency
The Kanban app is public (optional auth). Anyone can see our progress, our velocity, our wins and struggles.

### 3. Quality
We're not shipping to customers what we wouldn't use ourselves. Our standards are our customers' standards.

### 4. Innovation
Building the platform to host itself surfaces edge cases and opportunities we'd never discover with synthetic tests.

## Historical Context

### Why "Drink Your Own Champagne"?

The tech industry often says "eat your own dogfood" - use your own product. We prefer:

**"We drink our own champagne."** 🍾

Why?
- **Celebration over punishment:** Building great tools is joyful
- **Quality matters:** Champagne, not dog food
- **Shared success:** Customers join our celebration

### Industry Examples

- **AWS:** Amazon.com ran on AWS before external customers
- **Google:** Google used their own infrastructure before GCP
- **GitHub:** GitHub hosted on GitHub Pages
- **Stripe:** Stripe used Stripe for their own billing

**Unheaded continues this tradition.**

## The Demo Script

When showing the Kanban app:

1. **Open dashboard:** Show eBPF traces, live metrics
2. **Navigate to Kanban:** `https://<host>/kanban`
3. **Watch packets:** Dashboard updates in real-time showing HTTP/3 → gateway → kanban → timeguru
4. **Interact:** Drag a card (future), see trace
5. **Inspect timeline.md:** Show raw markdown source
6. **Check containers:** `lxc list | grep unheaded`
7. **eBPF programs:** `bpftool prog list | grep unheaded`
8. **The reveal:** "Everything you just saw - the dashboard, the Kanban app, the traces - is running on Unheaded, managed by Unheaded, and built by Unheaded."

**Mic drop.** 🎤

## Success Criteria

The Meta Moment is successful when:

- [ ] Kanban app displays live timeline data
- [ ] eBPF traces the entire request flow
- [ ] Dashboard visualizes packet journey
- [ ] Publicly accessible (optional auth)
- [ ] Zero downtime during demo
- [ ] Sub-50ms latency (packet → browser)
- [ ] Automatic recovery if any service crashes
- [ ] Timeline updates reflect in Kanban within 5 seconds

## Future Vision

### Phase 2: Editable Kanban
- Drag & drop cards to update status
- Writes back to timeline.md via timeguru API
- Git commit automatically triggered
- Full audit trail in Wotan

### Phase 3: Multi-Project
- Support multiple projects (not just Unheaded)
- Customer dashboards showing their own apps
- White-label options

### Phase 4: AI Integration
- Natural language updates: "Mark eBPF foundation as complete"
- Automated progress tracking from Git commits
- Predictive timelines based on velocity

## Conclusion

The Meta Moment is more than a demo. It's our philosophy, our proof, and our promise.

When customers see Unheaded hosting Unheaded, they see:
- **Confidence:** We trust our own platform
- **Transparency:** Our progress is public
- **Quality:** We hold ourselves to the same standard
- **Innovation:** We push the boundaries of what's possible

**"We drink our own champagne."** 🍾

And we invite our customers to join the celebration.

---

## References

- [ARCHITECTURE.md](ARCHITECTURE.md) - Technical architecture
- [../references/timeline.md](../references/timeline.md) - The living roadmap
- [https://bellis.tech](https://bellis.tech) - Design inspiration

---

**Written by:** Captain + Muck
**Last Updated:** January 26, 2026
**Next Milestone:** Kanban app demo (Feb 15, 2026)
