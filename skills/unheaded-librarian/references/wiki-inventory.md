# Wiki Page Inventory & Cross-Reference Map

**Last Updated**: February 22, 2026
**Total Pages**: 52

## Table of Contents

1. [Page Inventory by Category](#page-inventory-by-category)
2. [Source Document Mapping](#source-document-mapping)
3. [Cross-Reference Matrix](#cross-reference-matrix)

---

## Page Inventory by Category

### Navigation (3 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Home.md` | All sources | Landing page — links to everything |
| `_Sidebar.md` | N/A | Persistent navigation sidebar |
| `_Footer.md` | N/A | Footer with repo link |

### Getting Started (3 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Quick-Start.md` | README.md | Build/run/test instructions |
| `Vision.md` | docs/VISION.md | What Unheaded is and does |
| `The-Meta-Moment.md` | docs/THE_META_MOMENT.md | Self-hosting philosophy |

### Architecture (5 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Architecture.md` | docs/ARCHITECTURE.md, CLAUDE.md | 6-layer architecture, tech stack |
| `System-Diagram.md` | docs/SYSTEM_DIAGRAM.md | Visual component overview |
| `Kingdom-Architecture.md` | docs/KINGDOM_ARCHITECTURE.md | Sacred hierarchy |
| `Project-Structure.md` | docs/PROJECT_STRUCTURE.md | Repo layout |
| `Microservices.md` | docs/MICROSERVICES.md | Service catalog |

### Protocol (8 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Protocol-Foundation.md` | docs/protocol/ | Monad 20-byte wire format |
| `Protocol-Technical-Summary.md` | docs/protocol/ | Quick technical reference |
| `Sophia-Dictionaries.md` | docs/protocol/ | Exponent-encoded BPF maps |
| `Wotan-Memory-Model.md` | docs/protocol/ | Ring buffer + event bus |
| `The-First-Packet.md` | references/ | Origin story |
| `MBC-ISA-Reference.md` | docs/protocol/ | Monad Bytecode ISA |
| `Error-Registry.md` | docs/protocol/ | Protocol error codes |
| `Drafts-Index.md` | docs/protocol/ | Internet-Draft index |

### ADRs (1 index page)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `ADR-Index.md` | docs/adr/ | Links to all ADR files (015+) |

### Security (5 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Security.md` | docs/SECURITY.md | Security overview |
| `Security-Audit.md` | docs/SECURITY_AUDIT.md | Audit findings |
| `Security-TODOs.md` | docs/SECURITY_TODOs*.md | Current work items |
| `LICH-Campaigns.md` | docs/security/ | Automated adversary testing |
| `Dark-Grimoire.md` | docs/security/ | BlackMage offensive notes |

### Services (7 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Service-Wotan.md` | services/wotan/ | Message bus / ring buffer |
| `Service-Timeguru.md` | services/timeguru/ | Timeline tracking |
| `Service-Captain.md` | services/captain/ | Strategy service |
| `Service-Architect.md` | services/architect/ | Infrastructure design |
| `Service-Micromanager.md` | services/micromanager/ | Execution + QA |
| `Service-Dashboard-Backend.md` | cmd/dashboard-backend/ | Metrics + WebSocket |
| `Service-Kanban-App.md` | cmd/kanban-app/ | Meta Moment app |

### Infrastructure (6 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Containers.md` | nix/, CLAUDE.md | LXD/containerd/NixOS/Docker |
| `IaC-Backends.md` | CLAUDE.md, timeline.md | Ansible/Terraform/Puppet/K8s/Chef/Salt |
| `Observability-Backends.md` | CLAUDE.md, timeline.md | Prometheus/Grafana/ELK/Jaeger/Nagios+ |
| `eBPF-Programs.md` | ebpf/, crates/ | Rust/Aya + cilium/ebpf |
| `Fae-Chamber-Contracts.md` | docs/FAE_CHAMBER_CONTRACTS.md | Service interface contracts |
| `Service-Breakout-Strategy.md` | docs/SERVICE_BREAKOUT_STRATEGY.md | Post-alpha repo separation |

### Kingdom Lore (3 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Phylactery.md` | docs/PHYLACTERY.md | Living document |
| `Kingdom-Mode-Math.md` | docs/KINGDOM_MODE_MATH*.md | Register verification |
| `Doom-over-IPv6.md` | doom/ | Computational completeness proof |

### Development (4 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Developer-Guide.md` | CLAUDE.md | Standards and patterns |
| `Demo-Script.md` | docs/DEMO_SCRIPT.md | How to demonstrate |
| `Agent-Operating-Procedure.md` | docs/ | AI agent workflow |
| `Rust-Components.md` | docs/RUST_COMPONENTS.md | Rust crate inventory |

### Planning & Reference (5 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `Timeline.md` | references/timeline.md | Living roadmap |
| `Battle-Plan.md` | battle-plan.md | Current sprint plan |
| `Upcoming-Tasks.md` | docs/UPCOMING_TASKS.md | Task backlog |
| `Session-Index.md` | docs/sessions/ | Session handoff archive |
| `RFC-Cross-Reference.md` | docs/protocol/ | Standards we build on |

### RFC Reference (3 pages)
| Wiki Page | Source Doc(s) | Notes |
|-----------|--------------|-------|
| `IANA-Guide.md` | docs/protocol/ | IANA considerations |
| `Wire-Format-Patterns.md` | docs/protocol/ | Common wire format idioms |
| `Drafts-Index.md` | docs/protocol/ | Internet-Draft index |

---

## Source Document Mapping

Which source docs feed into which wiki pages:

| Source Document | Wiki Pages It Feeds |
|----------------|-------------------|
| `CLAUDE.md` | Architecture, Developer-Guide, Containers, IaC-Backends, Observability-Backends, eBPF-Programs |
| `battle-plan.md` | Battle-Plan |
| `references/timeline.md` | Timeline, IaC-Backends, Observability-Backends |
| `docs/ARCHITECTURE.md` | Architecture, System-Diagram |
| `docs/VISION.md` | Vision |
| `docs/THE_META_MOMENT.md` | The-Meta-Moment |
| `docs/SECURITY.md` | Security |
| `README.md` | Quick-Start |
| `LICENSES/THIRD_PARTY.md` | eBPF-Programs (licensing section) |

---

## Cross-Reference Matrix

Pages that must be updated together when key concepts change:

### "Interchangeable Backends" Changes
→ CLAUDE.md, battle-plan.md, timeline.md, docs/VISION.md, wiki/Architecture.md, wiki/Vision.md, wiki/Home.md, wiki/_Sidebar.md, wiki/[Category]-Backends.md

### "Service Added/Removed"
→ CLAUDE.md (tech stack), wiki/Microservices.md, wiki/Home.md (Services section), wiki/_Sidebar.md (Services section), wiki/Service-[Name].md (new page)

### "eBPF Program Changes"
→ wiki/eBPF-Programs.md, wiki/Architecture.md, CLAUDE.md (tech stack), LICENSES/THIRD_PARTY.md (if new dependency)

### "Security Policy Changes"
→ wiki/Security.md, wiki/Security-Audit.md, wiki/Security-TODOs.md, CLAUDE.md (Security Requirements section)

### "Protocol Spec Updates"
→ wiki/Protocol-Foundation.md, wiki/Protocol-Technical-Summary.md, wiki/Drafts-Index.md, wiki/Home.md (Internet-Drafts section)

### "Naming/Lore Changes"
→ ALL wiki pages that use the old name (use grep), _Sidebar.md, Home.md, CLAUDE.md
