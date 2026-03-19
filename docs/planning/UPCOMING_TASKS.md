# Unheaded — Upcoming Tasks
*Generated: February 16, 2026 — Session 12*

---

## Immediate (Clear the Decks)

1. **Commit all pending changes** — clear git lock files (`rm -f .git/*.lock`), then stage and commit toolbar, 51-card inventory, mythology wishlist, Timeguru DB fix
2. **Browser-verify sort/filter toolbar** — test all 7 sort options (priority/progress/type/owner/updated/created/title), type filter, owner filter, search with debounce, clear all, task count
3. **Run full 3-service integration** — `Wotan :8080/:9090` → `Timeguru :8000` → `Kanban :8081`, verify gRPC transport selection logged at startup
4. **Hydrate timeline.json milestones** — populate the empty milestones array from the 51-card inventory so Timeguru serves real phase/milestone data to the board
5. **Fix Phase 3 name duplication** — timeline.json shows "The MVP Era (PLANNED) (PLANNED) (PLANNED) (PLANNED)" from repeated syncs

---

## Short Term (Next 2 Weeks)

6. **TopicStream gRPC service** — the approved plan (`sprightly-conjuring-simon.md`): topic.proto, pattern matcher, server-side streaming, gRPC client implementing WotanClient interface, zero-breaking-change migration path. Eliminates 500ms HTTP polling.
7. **Wotan unit test suite** — comprehensive tests for core message bus: pub/sub, ring buffer, member management, topic lifecycle
8. **CI/CD pipeline templates** — GitHub Actions for `go build`, `go test -race`, `go vet`, lint, deploy gates
9. **E2E integration tests** — all 23 services health-checked, end-to-end message flow verified across Wotan → consumers
10. **Linux dev environment** — NixOS VM or bare metal for eBPF development (blocker B1, owner: Muck)

---

## Medium Term (Next Month)

11. **Dashboard backend (Visor)** — WebSocket metrics aggregation, wire real-time system stats to Kanban board
12. **Anamnesis event history** — immutable append-only event log for state reconstruction and audit trail
13. **Gateway API unification** — single ingress point with routing, rate limiting, auth middleware
14. **Cape auth/identity** — authentication + authorization layer, JWT/mTLS, RBAC
15. **Wotan service rename exploration** — the `unheaded-wotan` skill stays (essential coordinator), but `services/wotan/` message bus could adopt a mythology-inspired name (Hermes, Bifrost, Vayu, Fujin — messenger/bridge archetypes across pantheons)
16. **Docker Compose port alignment** — docker-compose.yml has wotan on 8081/5555, standalone defaults are 8080/9090. Unify.

---

## Long Term (Vision)

17. **eBPF packet tracing dashboards** — attach markers to packets, trace flow L2-L7, in-house visualization (the core promise)
18. **Compliance templates** — FEDRAMP, NIST 800-53, SOC2, PCI-DSS, HIPAA — automated audit evidence generation
19. **4-hour production deploy** — the north star: config → production-ready infrastructure in 4 hours
20. **Multi-cloud orchestration** — major cloud providers and bare metal from single NixOS-based config
21. **VXLAN + BGP network fabric** — full L2-L7 network stack inside NixOS LXD containers
22. **IDP/SIEM/SOC/NOC integration** — internal developer platform with security operations center
23. **Zero trust architecture** — mTLS everywhere, eBPF-enforced network policies, identity-aware proxy

---

## Mythology Naming Exploration (Wishlist)

Service naming candidates from world mythology — to be evaluated alongside existing Gnostic + medieval naming:

| Pantheon | Candidates | Archetype Fit |
|----------|-----------|---------------|
| **Gnostic** (current) | Monad, Sophia, Pleroma, Kenoma, Anamnesis, Yaldabaoth | State, wisdom, desired/actual, memory, chaos |
| **Norse** | Yggdrasil, Bifrost, Heimdall, Odin, Loki, Valkyrie, Mjolnir, Fenrir | World tree, bridge, guardian, wisdom, trickster, selector, power, destruction |
| **Hindu** | Indra, Agni, Vishnu, Shiva, Brahma, Maya, Dharma, Karma, Chakra, Garuda | Thunder, fire, preserver, destroyer, creator, illusion, law, consequence, energy, mount |
| **Chinese** | Pangu, Nuwa, Sun Wukong, Guanyin, Long Wang, Zhurong, Jade Emperor | Creation, repair, rebellion, mercy, water, fire, sovereignty |
| **Japanese** | Amaterasu, Susanoo, Tsukuyomi, Raijin, Fujin, Inari, Kitsune | Sun, storm, moon, thunder, wind, prosperity, transformation |

---

## Current Blockers

| ID | Blocker | Impact | Owner | Status |
|----|---------|--------|-------|--------|
| B1 | Linux/eBPF dev environment | HIGH — blocks all eBPF work, Whispering Void at 55% | Muck | PENDING |
| B2 | Git lock files in sandbox | LOW — workaround exists (`rm -f .git/*.lock`) | Env | KNOWN |

---

## Board Stats (as of Session 12)

- **51 cards** across 4 columns (20 Backlog, 22 In Progress, 0 Review, 9 Done)
- **8 task types**: milestone, infra, feature, task, bug, tech-debt, vision, wishlist
- **5 owners**: Architect, Developer, Captain, Muck, Team
- **237K+ LOC** across Go + Rust + frontend
- **23 services** in `services/` directory

---

*THE TIMEGURU HAS SPOKEN. THE CIRCLE NEVER BREAKS.*
