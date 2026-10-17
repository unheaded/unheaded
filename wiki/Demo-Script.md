# Demo Script

Two scripts exist: a 4-minute narrated video walkthrough and a ~20-minute live conference demo. Both show Unheaded running its own development (the Meta Moment) and tracing every packet with eBPF.

## Conference demo (~20 min, `demos/DEMO-SCRIPT.md`, 2026-04)

*"Doom in the Data Plane: Observing Every Packet with eBPF".* Setup with `scripts/demo-start.sh` (< 5 min), reset between runs with `scripts/demo-reset.sh`.

**Before you start:** Wotan, dashboard-backend, kanban-app and unheaded-daemon up on west (east too for segment 5); `trace-collector-go` in demo mode on west; dashboard on `http://localhost:20000`; three terminals visible.

| Segment | Time | Shows |
|---------|------|-------|
| 1. Introduction | 2 min | You bring the app ("the head"), Unheaded provides the rest |
| 2. Architecture | 3 min | The six layers, Wotan at the centre |
| 3. Single request trace | 3 min | One request followed from XDP through Wotan to the dashboard |
| 4. Doom in the data plane | 5 min | Doom running on the UPC, computed in packets ([[Doom-on-UPC]]) |
| 5. Cross-host | 3 min | West ↔ east flow graph |
| 6. Production load | 3 min | Load test, metrics under pressure |
| 7. Your app here | 2 min | Where a user's application plugs in |
| Q&A | 2 min | |

## Video walkthrough (4 min, `docs/planning/DEMO_SCRIPT.md`, 2026-02)

| Act | Time | Shows |
|-----|------|-------|
| Cold open | 0:00 | The infrastructure every team rebuilds |
| 1. What it is | 0:25 | Gateway, mesh, Maglev LB, secrets, control plane, observability |
| 2. Dashboard | 1:00 | Live packet flow; "Demo Packet" button; trace IDs injected at XDP |
| 3. The Meta Moment | 1:50 | Kanban board fed by Timeguru; its own requests appear on the dashboard ([[Service-Kanban-App]]) |
| 4. eBPF | 2:30 | packet-marker (XDP), flow-tracker (TC), latency-probe (kprobes) → trace-collector → Wotan |
| 5. Mesh and Wotan | 3:15 | Topic routing ([[Fae-Chamber-Contracts]]), circuit breakers, mTLS, default deny |
| 6. Security | 3:45 | Zero user data access by architecture (NIST 800-207 Zero Trust Architecture); container hardening |
| Closing | 4:10 | Everything shown runs on the platform it describes |

## Before reusing the video script

The tagline and the unheaded.com call to action were removed on 2026-10-17; the closing now points at the GitHub repo. Feature descriptions in it are still from February, so check them against [[Microservices]] before recording.

---

> **Sources:** [demos/DEMO-SCRIPT.md](../demos/DEMO-SCRIPT.md) · [docs/planning/DEMO_SCRIPT.md](../docs/planning/DEMO_SCRIPT.md)
