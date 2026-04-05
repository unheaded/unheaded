# ADR-042: CS Cheat Sheets → BlackMage Skill + Lich Security Service

## Status: PLANNED

## Date: 2026-04-05
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)

---

## Context

The cs project at ~/tmp/projects/cs/ contains 594+ sheets across 59 categories
including offensive security (CEH, pentesting, eBPF attacks), network security
(firewalls, IDS, NAC, MACsec), and compliance (CISSP, SOC2). This content
directly feeds two Kingdom components:

1. **BlackMage skill** — offensive security, pentesting, red team, fuzzing
2. **Lich service** — the security testing framework (tomb/lich/)

## Decision

### Phase 1: BlackMage Skill Enhancement

Merge CEH/infosec sheets from cs project into the BlackMage skill:
- `~/.claude/skills/unheaded-blackmage/references/` — add curated sheets
- Categories: offensive/, security/, compliance/ from cs project
- Focus: eBPF attack surface, network exploitation, privilege escalation,
  web application attacks, wireless security, social engineering

Sheets to incorporate:
- `cs/sheets/offensive/ebpf-security.md` — eBPF verifier bypass, JIT spray
- `cs/sheets/security/suricata.md` — IDS rules, detection patterns
- `cs/sheets/security/falco.md` — runtime threat detection
- `cs/sheets/security/iptables.md` — firewall rules for attack/defense
- `cs/sheets/security/network-security-infra.md` — defense in depth
- `cs/sheets/compliance/cissp.md` — 8 security domains
- All CEH-specific sheets when generated

### Phase 2: Lich Service Integration

The Lich service (tomb/lich/) runs security test harnesses. Integrate cs
sheets as reference data for:
- Fuzz test generation (use attack patterns from CEH sheets)
- Vulnerability assessment (match CVE patterns against cs security sheets)
- Red team playbooks (automated from cs offensive sheets)
- WAL integrity fuzzing already exists (lich_010_wal_integrity_test.go)

### Phase 3: Precision Reference API

The cs tool has a REST API service mode. Deploy as Kingdom service:
- Port: TBD (Doom Range)
- Endpoints: `/api/v1/sheets/<category>/<topic>`, `/api/v1/search?q=...`
- Zhenai integration: when answering security questions, query cs API
  for precise technical reference before generating response
- ADR-039 covers the general integration; this ADR adds the
  BlackMage + Lich specific integration points

## Consequences

### Positive
- BlackMage skill gets 100+ reference sheets (CEH, pentesting, compliance)
- Lich fuzz tests can use real attack patterns
- Zhenai security answers grounded in cert-grade reference material
- Daily adversarial loop (BlackMage ↔ Sentinel) uses real attack knowledge

### Negative
- Offensive security content needs careful handling (authorized use only)
- Cross-repo dependency (cs project is separate from Unheaded)
- Lich integration requires mapping attack patterns to test harnesses
