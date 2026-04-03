# ADR-033: NetBox for Local IPAM — Proper Subnetting as Kingdom Scales

**Status:** Planned
**Date:** 2026-04-03
**Deciders:** Architect, Captain
**Note:** Not part of the Unheaded repo — separate service at ~/tmp/projects/netbox/

## Context

As the Kingdom scales from 2 hosts to more, IP address management becomes critical. Currently:
- WEST: 192.168.69.184, P2P: 192.168.13.2
- EAST: P2P: 192.168.13.1
- Bridge: 10.10.10.0/24 (lxdbr0)
- Doom Range: ports 16666-26666
- WireGuard overlay: fd00:dead:beef::/48 (planned)

This is manageable for 2 hosts but will break at 10+. NetBox provides:
- IP address management (IPAM) — no more manual tracking
- Subnet allocation — proper /24, /28, /30 planning
- VLAN/VXLAN management — as we add network segments
- Device inventory — WEST, EAST, future hosts
- Cable management — P2P links, switch connections
- Security zones — map to ADR-009 Parish Boundaries

## Decision

Deploy NetBox locally on WEST as a Docker service. Use it for:
1. Document all current IP allocations
2. Plan subnets for new services/hosts
3. Track P2P links between hosts
4. Integrate with Zhenai for automated IPAM queries

NetBox source already cloned at `~/tmp/projects/netbox/`.

### Runbook

Add `runbooks/infra/netbox-setup.yaml` for deployment.

### Port Allocation

NetBox web UI: port 18888 (conflicts with APT repo) → use port 18889 instead.

## Consequences

### Positive
- Professional IPAM as the Kingdom grows
- No more "what IP was that?" conversations
- Foundation for automated network provisioning
- Security zone documentation for audits

### Negative
- Another service to maintain (Docker-based)
- Learning curve for NetBox UI
- Not part of the core Unheaded product — operational tooling only
