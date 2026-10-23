# Welcome to the Unheaded Kingdom

---

Configuration management automation platform. You bring the application ("the head"). Unheaded provides everything else: control plane, service mesh, observability, security baseline, packet-level tracing.

Built around the **Unheaded Protocol** — a 20-byte Monad register file carried in IPv6 Hop-by-Hop extension headers, processed at each hop via eBPF. Wire format frozen at v0x01 (13 IANA registries in Foundation draft-06).

Computational completeness proven by running Doom (1993) inside eBPF: MBC bytecode in BPF maps, executed as packets traverse an XDP circulation ring across Linux namespaces. If a game engine runs in the data plane, packet tracing does too.

---

## Wiki Pages

### Core
- **[[Architecture|Architecture]]** — 6-layer system, Monad wire format, eBPF execution model
- **[[Protocol Specifications|Protocol-Specs]]** — Monad, Wotan, Sophia
- **[[Security|Security]]** — NIST 800-207 Zero Trust Architecture, Lich campaigns

### Doom-over-IPv6
- **[[Doom over IPv6|Doom-over-IPv6]]** — Technical narrative
- **[[Bug Kill Chain|Bug-Kill-Chain]]** — Bugs found and fixed during Doom development
- **[[Performance|Performance]]** — Injection rates, optimization roadmap

### Project
- **[[Roadmap|Roadmap]]** — Current state and forward plan

---

## Quick Links

| Resource | Location |
|----------|----------|
| Source | [github.com/unheaded/unheaded](https://github.com/unheaded/unheaded) |
| Wiki | [github.com/unheaded/unheaded/wiki](https://github.com/unheaded/unheaded/wiki) |
| Dashboard | `/dashboard` |
| Kanban | `/kanban` (the Meta Moment) |
| Doom viewer | `/doom/` |
| Health | `/api/v1/health` |

---

*Source of truth: `references/timeline.md` + `docs/adr/ADR-INDEX.md`. Drift policy: ADR-052.*
