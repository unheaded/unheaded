# Kingdom Skills Documentation

The Unheaded Kingdom is an ensemble system built around a constellation of specialized AI skills, each representing a distinct persona and domain of expertise. This directory indexes all Kingdom skills and their roles.

## The Kingdom Personas

### Security & Defense

**Sentinel** — [unheaded-sentinel.md](./unheaded-sentinel.md)
- Network watchman, SOC analyst, defender
- Monitors Kingdom perimeter, baselines device traffic, detects anomalies
- Owns: Pi-hole deployment, device inventory, IoT security, firewall management, DNS log analysis, threat detection, CVE triage, patch management
- Part of the Daily Adversarial Loop (blue team)

**BlackMage** — `.skills/skills/unheaded-blackmage/SKILL.md`
- The dark mage, offensive security expert, adversary
- Breaks things so the Kingdom learns where the cracks are
- Creator of the Lich — automated adversary that fuzzes continuously
- Owns: Offensive testing, vulnerability assessment, fuzzing campaigns, red team exercises, exploit development
- Part of the Daily Adversarial Loop (red team)

**MoatGhost** — `.skills/skills/unheaded-moatghost/SKILL.md`
- Compliance and certification expert
- Audits security posture against frameworks (PCI-DSS, HIPAA, ISO 27001, SOC 2)
- Owns: Compliance mapping, audit documentation, certification tracking

**Architect** — `.skills/skills/unheaded-architect/SKILL.md`
- System design and security architecture
- Designs defensive structures, isolation boundaries, defense-in-depth improvements
- Owns: Architecture design, system modeling, defensive hardening

**Developer** — `.skills/skills/unheaded-developer/SKILL.md`
- Implementation and code hardening
- Translates security findings into hardened code
- Owns: Code implementation, regression testing, security fixes

### Operations & Coordination

**ComputerMancer** — `.skills/skills/unheaded-computermancer/SKILL.md`
- Unheaded Protocol Computer (UPC) specialist
- Designs and develops the MBC bytecode system, Dream Ladder, Shim pipeline
- Works with BlackMage on MBC fuzzing and bytecode safety

**Captain** — `.skills/skills/unheaded-captain/SKILL.md`
- Leadership, direction, business context
- Makes final calls on risk acceptance and customer-facing security narrative
- Owns: Strategic direction, milestone planning, risk acceptance decisions

**Micromanager** — `.skills/skills/unheaded-micromanager/SKILL.md`
- Project tracking, priority scheduling, QA gate integration
- Ensures security findings jump the queue appropriately
- Owns: Sprint planning, task prioritization, QA integration

**Calendar** — `.skills/skills/unheaded-calendar/SKILL.md`
- Event scheduling, reminder orchestration, time management
- Coordinates daily adversarial loop execution

**Warmonger** — `.skills/skills/unheaded-warmonger/SKILL.md`
- Battle planning and offensive campaigns
- Coordinates red team exercises and adversarial testing

**Marshal** — `.skills/skills/unheaded-marshal/SKILL.md`
- Incident response and crisis management
- Coordinates response when attacks succeed or issues are critical

### Research & Knowledge

**Scientist** — `.skills/skills/unheaded-scientist/SKILL.md`
- Formal verification and theoretical analysis
- Designs verification tests, proves safety properties

**RFC Editor** — `.skills/skills/unheaded-rfceditor/SKILL.md`
- Protocol specification and standards work
- IETF Internet-Draft author, RFC alignment, spec validation

**Librarian** — `.skills/skills/unheaded-librarian/SKILL.md`
- Documentation, knowledge management, reference systems
- Maintains the Tomb of Knowledge and protocol documentation

**Lore** — `.skills/skills/unheaded-lore/SKILL.md`
- Kingdom mythology, naming conventions, lore consistency
- Ensures naming is mythologically consistent across the system

### Specialized Roles

**Barrister** — `.skills/skills/unheaded-barrister/SKILL.md`
- Legal review, licensing audit, compliance frameworks
- IP audit, copyright/trademark verification, GPL compliance

**Busboy** — `.skills/skills/unheaded-busboy/SKILL.md`
- Cleanup, maintenance, debt reduction
- Code cleanup, documentation maintenance, technical debt tracking

**Round Table** — `.skills/skills/unheaded-round-table/SKILL.md`
- Council of peers, milestone reviews, collective decision making
- Used for major decisions at age transitions and protocol milestones

**Kingdom** — `.skills/skills/unheaded-kingdom/SKILL.md`
- Meta-skill, system overview, cross-skill coordination
- High-level strategic view, integration between skills

## The Daily Adversarial Loop

The crown jewel of the Kingdom system is the **Daily Adversarial Loop**, orchestrated by the Zhen AI scheduler:

```
Zhen AI (daily, 03:00 UTC)
     ↓
Sentinel (blue team):         BlackMage (red team):
- Pull CVE feeds              - Receive CVE list
- Assess Kingdom exposure     - Attempt exploits
- Patch/mitigate              - Report findings
- Detect attacks
- Log to Anamnesis
     ↓                              ↓
     └──────────┬──────────────────┘
                ▼
           Daily Report
        (CVEs, attacks, caught, patches, drift)
                ↓
        ┌───────┴────────┐
        ▼                ▼
    Anamnesis        MoatGhost
    (History)       (Compliance)
```

This creates an antifragile system where every day brings new attacks and new defenses, making the Kingdom stronger from adversarial stress.

## The Security Triad

Sentinel, BlackMage, and MoatGhost form a triangle:

```
        BlackMage (Attack)
             / \
            /   \
           /     \
      Sentinel --- MoatGhost
      (Defend)    (Certify)
```

- **Sentinel MONITORS** the network (watches traffic, detects threats, blocks attacks)
- **BlackMage ATTACKS** the network (tests defenses, finds vulnerabilities)
- **MoatGhost AUDITS** compliance (checks boxes, maps frameworks, certifies)

Together they ensure the Kingdom is not only defended, but provably defended.

## Handoff Flows

Every skill has defined handoff points showing where findings flow:

- Sentinel → BlackMage: "Anomaly detected. Investigate and exploit."
- Sentinel → Architect: "Firewall gap found. Network topology needs change."
- Sentinel → Developer: "CVE affects dependency. Patch priority: HIGH."
- Sentinel → MoatGhost: "Compliance drift found. Evidence attached."
- BlackMage → Developer: "Vulnerability report with PoC."
- BlackMage → Architect: "Architectural flaw revealed. Redesign recommended."
- BlackMage → Micromanager: "P0 security findings jump the queue."
- Architect → Developer: "Hardened design specifications."

## Full Skill Paths

All skills are stored in: `.skills/skills/unheaded-<skillname>/SKILL.md`

For example:
- `.skills/skills/unheaded-sentinel/SKILL.md`
- `.skills/skills/unheaded-blackmage/SKILL.md`
- `.skills/skills/unheaded-architect/SKILL.md`

## MCP Integrations

Kingdom skills integrate with external systems via MCP connectors:

- **NIST NVD API**: CVE catalog with CVSS scores
- **CISA KEV**: Known Exploited Vulnerabilities feed
- **Vendor Advisories**: Security bulletins from Go, Rust, Linux, NixOS
- **VirusTotal API**: Hash and domain reputation lookups
- **AbuseIPDB**: IP reputation and abuse history
- **Custom RSS/Webhooks**: Vendor-specific threat feeds
- **Suricata IDS**: Network-level intrusion detection
- **Pi-hole API**: DNS monitoring and blocking

---

*The Kingdom is not built by one hero. It is built by an ensemble, each specialist bringing their unique perspective to a shared mission: build something stronger.*
