# S36 Post-Sprint Notes — Muck's Brain Dump

**Date**: 2026-02-24
**Context**: Notes taken during S36 execution. Mix of immediate fixes, architecture ideas, research items, and far-future features.
**Status**: S36 COMPLETE. Preparing S37+ overnight sprint.

---

## IMMEDIATE FIXES (S37 Scope)

### Documentation Rename — MANDATORY
- **Rename "product" → "application"** throughout all documentation. This is a computer science RFC, PoC, and experiment — not a SaaS product pitch.
- **Rename "customer" → "user"** throughout all documentation. Same reasoning.
- **Remove ALL mentions of "We drink our own champagne"** or similar champagne references. Technical docs only.

### RFC References — MANDATORY
- Add RFC references and acknowledgements to foundational RFCs if not already present:
  - RFC 791 (IP)
  - RFC 792 (ICMP)
  - RFC 793 (TCP)
  - RFC 768 (UDP)
- Check protocol specs (Monad, Sophia, Wotan) for missing normative references.

### Linux Port Range Tuning
- Adjust outbound ephemeral port range to avoid colliding with Doom Range (16666-26666).
- Default Linux ephemeral: `net.ipv4.ip_local_port_range = 32768 60999`
- Should tune to: `net.ipv4.ip_local_port_range = 30000 65000` (or 27000-65000 to stay above Doom Range).
- Document in NixOS hardening config and deployment docs.

---

## ARCHITECTURE DECISIONS

### Centralized Config — "The Timeguru Expansion"
S36 hit a speed bump: ports hardcoded everywhere. In 2026 a robot is writing an application that would take a human a year — our infrastructure MUST match this dynamic.

**Proposal**: Expand Timeguru from just feeding Kanban to being the **cascading point-of-truth central reference** for:
- Port assignments (already started with `pkg/ports/` and `configs/port-registry.yaml`)
- Proxy configuration
- Service discovery endpoints
- Container networking
- ALL config variables

Organized as:
```
configs/
  ports.yaml          # port registry (exists)
  services.yaml       # service discovery fallback (exists)
  proxy/              # nginx/gateway config templates
  networking/         # network topology definitions
  containers/         # runtime-specific overrides
  observability/      # metrics, logging, tracing endpoints
```

Each YAML/JSON/TOML file is a runbook/instruction set — this IS our own take on Puppet/Ansible/Terraform/Salt/Chef. The Unheaded way.

### Package Breakout to Separate Repos
Formal breakout of major packages to their own repos (like unheaded-wiki already is):
- `~/tmp/wotan/` — Wotan message bus
- `~/tmp/sophia/` — Sophia knowledge graph
- `~/tmp/kanban/` — Kanban app
- `~/tmp/dashboard/` — Dashboard
- `~/tmp/doom/` — Doom integration (GPL boundary — this ESPECIALLY needs isolation)

Benefits: independent versioning, cleaner CI, contributor isolation, license boundary enforcement.

### Log Aggregation — Multi-Machine Awareness
Current log aggregation assumes all monads/services within a single kingdom (single host). If they spread across multiple bare metal machines, we need:
- Programmatic location registry (which machine runs which service)
- Cross-machine log forwarding (ship logs to central Wotan or federated Wotan instances)
- Machine identity in log entries

### Log Viewer UI Improvements
- Log display: latest line on BOTTOM, oldest on TOP (like `tail -f`)
- Bottom 2 lines should be 2x height of other lines with light highlighting (visual anchor)
- Tick/select boxes to filter by: IP, port, response code, service, level
- Scroll behavior: auto-scroll to bottom when new entries arrive (tail mode)

---

## RESEARCH ITEMS

### Cloudflare Assessment — PRIORITY
Assess ALL Cloudflare offerings, blogs, and tech. They are FAANG-level industry leaders. Look for:
- Public domain tools/libraries we can utilize
- Architecture patterns worth adopting
- Performance optimization techniques
- eBPF usage patterns (Cloudflare is heavy eBPF)
- Workers/edge compute patterns
- DDoS mitigation architecture

### Gorgonia — Internal LLM/RAG
- **URL**: https://github.com/gorgonia/gorgonia
- **Tutorial**: https://gorgonia.org/tutorials/mnist/
- **Why**: Perfect for developing our own internal LLM to manage infrastructure. Can reference wiki and docs, "learn" and be trained on project materials.
- **Hardware**: Muck has a gaming PC that can be repurposed as dev playground:
  - Dual-boot high-powered Linux dev / Windows gaming box
  - Can run: Doom, LLM, multiple Unheaded full stacks
  - Demo: kingdom expansion (horizontal scaling)
  - Fireguard tunnel from kingdom to web proxy for read-only demo exposure
- **ADD TO TIMELINE** — this is a long-desired feature.

### FOSSA BSL Article
- **URL**: https://fossa.com/blog/business-source-license-requirements-provisions-history/
- Relevant for our BSL 1.1 licensing decision. Read and reference in Barrister session.

### Whois Deprecation
- Whois is deprecated — investigate replacement tooling.
- We will use RDAP or similar somewhere in the stack.

---

## FAR FUTURE FEATURES

### Compliance Dashboard
- Export to CSV
- Admin or admin-group user can enable security features
- Full matrix of controls with overlap handling:
  - If we enable "enforce MFA for admin login," it should auto-select/grey-out in NIST/GDPR/SOC2/etc. columns
  - Cross-framework control mapping

### Admin/User Modes
- Admin and standard user modes
- Potentially more granular (RBAC already scaffolded in pkg/auth/)

### Kingdom Expansion (Horizontal Scaling)
- Demo: multiple Unheaded full stacks running simultaneously
- Easy-mode horizontal scaling across machines
- Federation between Wotan instances

---

## PERSONAL/OPERATIONAL TASKS

- Move `www`, `weather-daemon`, and `rss-daemon` into `~/tmp/` directory
- Repurpose `~/dev/` repo: move existing dev content to `~/dev/slop/`
- New repo for web machine config/programs (could run Unheaded services)
- Merge all tutorials into folder, print at FedEx (heavy duty, multiple copies for distribution)
- Rough draft early version at `~/tmp/eli5/`
- Hit up friends, distribute binders/backups

---

*Peace and love. The Protocol IS the Moat. The Kingdom expands.*
