# ADR-087 — NOC: Network Device Monitoring and Configuration Management

**Status:** Planned
**Date:** 2026-07-24
**Author:** Stevie Bellis
**Deciders:** Stevie Bellis

---

## Context

The Kingdom lab is adding managed switches and routers for JNCIA certification
prep. These devices produce observability data (IPFIX flows, syslog, SNMP) and
need configuration management. Currently there is no NOC capability — no flow
collection, no structured device syslog ingestion, and no automated config
management for network hardware.

The existing Unheaded stack has the right primitives:
- Muninn (ADR-086) already handles syslog fan-out to PostgreSQL and SIEM
- Huginn (ADR-084) handles host metrics; can be extended for SNMP polling
- Wotan is the desired-state bus — network device state belongs there too
- The Kingdom dashboard already renders flows and latency

---

## Proposal Review: Ansible for Network Config

Ansible is the right call for network config management and is JNCIA-aligned —
Juniper's own automation tooling is built around it. Two improvements over a
basic Ansible approach are worth adopting from the start:

### Improvement 1: NETCONF/YANG over raw SSH/CLI

Plain Ansible SSH against Junos CLI is fragile — it parses human-readable
output and is sensitive to software version differences. Junos has
**first-class NETCONF support** (RFC 6241), which is:

- **Structured**: XML/YANG, not screen-scraping
- **Transactional**: candidate configuration + commit/rollback
- **Idempotent**: Ansible's `junipernetworks.junos` collection handles this
- **JNCIA-relevant**: NETCONF is on the Junos automation track and increasingly
  expected in production environments

Use the `junipernetworks.junos` Ansible collection with `ansible_connection: netconf`
for all Junos targets. Fall back to `ansible_connection: network_cli` only for
devices that do not support NETCONF (non-Juniper gear).

### Improvement 2: GitOps for device configuration

Device configs live in the repo (`network/devices/<hostname>/`) as YAML variable
files (Ansible vars + NETCONF templates). Changes go through the same review
process as code:

```
git push → CI (ansible-lint + syntax-check + dry-run diff) → PR review → merge → deploy
```

This gives the same auditability as infrastructure code and doubles as study
material — every OSPF adjacency, VLAN trunk, or BGP session is documented in
version-controlled intent files.

---

## Decision

### New service: Kvasir (IPFIX / flow collector)

**Kvasir** — in Norse mythology, the wisest being ever created, formed from the
combined knowledge of the Aesir and Vanir. Slain, his blood became the Mead of
Poetry that carries wisdom across the Nine Worlds. An IPFIX collector that
distills raw packet flows into network intelligence is a precise analogue.

Kvasir is a lightweight flow collector daemon:
- Receives IPFIX (RFC 7011), NetFlow v5/v9, and sFlow from network devices
- Normalises all formats into a common Kingdom flow envelope
- Publishes normalised flows to Wotan topic `flow.network.<device>`
- Muninn subscribes and fans out to VictoriaMetrics (flow rates, top-N),
  PostgreSQL (`ops.network_flows`), and optionally SIEM

**Implementation path**: wrap **GoFlow2** (open-source, maintained, handles all
flow protocol variants natively) rather than writing a parser from scratch.
Kvasir adds the Kingdom integration layer (Wotan publish, YAML config, systemd
unit) on top of GoFlow2's decoded flow structs via its Kafka/stdout output.

```yaml
# /etc/kvasir.yaml
listen:
  ipfix: ":4739"      # RFC 7011 (also NetFlow v9 template-based)
  netflow5: ":2055"
  sflow: ":6343"
wotan:
  url: http://localhost:18000
  topic_prefix: flow.network
push_interval: 15s
```

### Muninn extension: network device syslog

Network devices ship syslog to a UDP/TCP listener. Muninn (ADR-086) already
handles journald → sinks; adding a syslog listener is a Phase 2 extension:

```yaml
# /etc/muninn.yaml addition
sources:
  syslog:
    enabled: true
    listen_udp: ":514"
    listen_tcp: ":514"
    parsers:
      - junos        # structured syslog with RT_FLOW, CHASSISD, etc.
      - rfc5424
      - rfc3164      # legacy
```

Junos RT_FLOW syslog events (session open/close with src/dst/bytes/protocol)
are particularly valuable — they can substitute for IPFIX when a device is
configured to emit them rather than full IPFIX.

### Huginn extension: SNMP device health polling

Huginn currently reads `/proc` only. For network devices it cannot SSH into,
SNMP polling covers the gap (interface counters, CPU/mem on the device,
link state). Planned as a Huginn extension rather than a separate daemon:

```yaml
# /etc/huginn.yaml addition (Phase 3)
snmp_targets:
  - host: 192.168.1.1
    label: core-switch
    community: "${SNMP_COMMUNITY}"
    version: "2c"
    oids:
      - ifInOctets
      - ifOutOctets
      - ifOperStatus
poll_interval: 60s
```

### Ansible for configuration management

```
network/
  inventory/
    hosts.yaml              # device inventory (hostname, mgmt IP, platform)
  group_vars/
    junos.yaml              # Junos-wide defaults (NETCONF, NTP, syslog target)
  host_vars/
    <hostname>.yaml         # per-device intent (interfaces, VLANs, routing)
  roles/
    junos_base/             # NTP, DNS, syslog, SNMP, user accounts
    junos_interfaces/       # interface IP, description, MTU
    junos_vlans/            # VLAN database, trunk/access assignments
    junos_ospf/             # OSPF area, interfaces, timers (JNCIA lab)
    junos_bgp/              # eBGP/iBGP sessions (JNCIA lab)
  playbooks/
    apply.yaml              # main: apply full desired state
    check.yaml              # --check --diff only (dry run, used in CI)
    backup.yaml             # pull running config → git
```

**Connection**: `ansible_connection: netconf` for all Junos targets.
**CI gate**: `ansible-lint` + `ansible-playbook --check --diff` on every PR
that touches `network/`. Fail CI on drift, not just syntax errors.

### NOC dashboard panel

New panel in the Kingdom dashboard: **Network** tab alongside the existing
Services / Infrastructure tabs. Sourced from Kvasir flow data in Victoria:

- Top-N flows by bytes (last 5 min)
- Interface utilisation per device (from SNMP via Huginn)
- Device syslog error rate (from Muninn → PostgreSQL)
- BGP/OSPF adjacency state (from Ansible fact cache or SNMP)

---

## Architecture

```
Network devices (switches, routers)
  │
  ├─ IPFIX/NetFlow/sFlow → Kvasir :4739/:2055/:6343
  │                           └─ Wotan (flow.network.*)
  │                                 └─ Muninn → VictoriaMetrics
  │                                          → PostgreSQL (ops.network_flows)
  │                                          → SIEM
  │
  ├─ Syslog (UDP 514) ────────→ Muninn :514
  │                                 └─ PostgreSQL (ops.device_syslog)
  │                                 └─ SIEM (RT_FLOW, errors, auth)
  │
  ├─ SNMP ────────────────────→ Huginn (snmp_targets)
  │                                 └─ VictoriaMetrics (interface counters)
  │
  └─ NETCONF/SSH ─────────────→ Ansible (GitOps)
                                    └─ Desired state from network/ in repo
```

---

## JNCIA Lab Alignment

The Ansible roles map directly to JNCIA-Junos exam objectives:

| JNCIA Topic | Ansible Role | Lab Scenario |
|-------------|-------------|--------------|
| Interface config | `junos_interfaces` | L3 point-to-point links |
| VLANs / switching | `junos_vlans` | Trunk/access between switches |
| OSPF | `junos_ospf` | 2-router adjacency, area 0 |
| BGP | `junos_bgp` | eBGP between two ASes |
| Routing policy | future role | import/export filters |
| Firewall filters | future role | stateless ACLs |

NETCONF familiarity gained here is directly applicable to JNCIA-DevOps (formerly
JNCIA-Junos automation track) and is increasingly expected in production Junos
environments.

---

## Implementation Phases

**Phase 1 — Syslog ingestion (quickest win)**
- Muninn: add UDP syslog listener, Junos RFC 5424 parser
- `ops.device_syslog` table in PostgreSQL
- Dashboard: syslog error rate counter in Network tab

**Phase 2 — Ansible GitOps skeleton**
- `network/` directory in repo
- `junos_base` role (NTP, DNS, syslog target → this host :514)
- CI: ansible-lint gate on `network/` changes
- First physical device configured entirely via git

**Phase 3 — Kvasir (IPFIX collection)**
- GoFlow2 integration, Wotan publish, `ops.network_flows` table
- Dashboard: top-N flows panel
- Syslog RT_FLOW events as fallback where IPFIX isn't available

**Phase 4 — Huginn SNMP extension**
- SNMP polling for interface counters and device health
- Dashboard: interface utilisation graphs

**Phase 5 — Full NOC dashboard tab**
- Network tab: flows + interface util + syslog errors + adjacency state
- Cross-correlate: flow anomalies with syslog events at same timestamp

---

## Consequences

- Network devices are first-class Kingdom citizens: config in git, flows in
  Victoria, syslog in PostgreSQL, anomalies in SIEM
- JNCIA lab automation is free — the same Ansible roles used for study are the
  production config management tool
- NETCONF/YANG investment pays forward to JNCIA-DevOps track and production Junos
- Kvasir + Muninn together cover all standard NOC data planes (flow, syslog, SNMP)
  without requiring a commercial NMS

---

## Related

- ADR-084 — Huginn (SNMP extension planned here)
- ADR-085 — CI/CD artifact layout (Kvasir, Muninn systemd units, .deb packaging)
- ADR-086 — Muninn (syslog extension planned here)
- `runbooks/network/bpf-flow-graph.yaml` — existing eBPF flow graph
- `runbooks/network/west-east-network.yaml` — P2P link setup
