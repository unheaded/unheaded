# Unheaded Kingdom Network Documentation

This directory contains comprehensive network topology, firewall integration, and port registry documentation for the Unheaded Kingdom project.

## Documents

### 1. FIREWALL_TOPOLOGY.md (555 lines)
**Comprehensive Network Architecture and Firewall Integration**

Complete overview of the dual-firewall architecture:
- OPNsense (BSD 2-Clause) on HOST-A (The Forge)
- IPFire (GPL v3) on HOST-B (The Outpost)

Covers:
- Full ASCII topology diagram
- Monad Protocol HbH passthrough (critical for observability)
- OPNsense pf.conf configuration with GUI steps
- IPFire nftables configuration with Web UI steps
- Network addressing plan (IPv4/IPv6)
- Firewall rule philosophy (default-deny security posture)
- Exposed ports reference (443, 80, 51820)
- Platform-specific deployment (NixOS, Docker, LXD)
- Compliance and licensing (BSD 2-Clause, GPL v3, MIT)
- Testing, validation, and disaster recovery

**Key Sections:**
- Section 3: Monad Protocol HbH Passthrough (CRITICAL)
- Section 5: Firewall Rule Philosophy
- Section 6: Exposed Ports (WAN-Accessible)
- Section 8: Compliance and Licensing

---

### 2. MONAD_HBH_FIREWALL_RULES.md (727 lines)
**Detailed Platform-Specific HbH Configuration**

Ensures Monad Protocol IPv6 Hop-by-Hop extension headers pass through firewalls without stripping or rewriting.

Covers:
- Quick reference table for all firewall platforms
- **OPNsense (FreeBSD/pf)** complete pf.conf rules + GUI steps
- **IPFire (Linux/nftables)** complete nftables ruleset + Web UI steps
- **Linux iptables** generic configuration
- Testing and verification with tcpdump
- Monad test packet injection (Python scapy example)
- Common mistakes and troubleshooting
- Performance and logging considerations
- Regulatory compliance (SOC2, PCI-DSS, HIPAA)
- Migration checklist

**Critical Content:**
- Section 2.1: OPNsense pf.conf with Monad HbH rules
- Section 3.1: IPFire nftables with Monad HbH rules
- Section 5: Testing and Verification (tcpdump, scapy)
- Section 6: Common Mistakes and Troubleshooting

---

### 3. INGRESS_EGRESS_PORTS.md (802 lines)
**Complete Port Reference and Firewall Ruleset**

Master registry of all Unheaded services and ports.

Covers:
- Master port registry table (all 25+ services)
- WAN-exposed ports (80, 443, 51820)
- Internal-only ports (50051–50067 gRPC, 8080/8443 dashboard)
- Infrastructure ports (53 DNS, 5353 mDNS, 4789 VXLAN)
- Firewall zone definitions (WAN, LAN, VPN, DMZ, Management)
- Per-service firewall rules (complete OPNsense + IPFire rulesets)
- Traffic flow diagrams (inbound, container-to-container, east-west)
- Monitoring and logging
- Testing checklist
- Port allocation for future services
- Regulatory compliance matrix

**Complete Rulesets:**
- Section 4.1: OPNsense (pf.conf) complete rules (350+ lines)
- Section 4.2: IPFire (nftables) complete rules (250+ lines)
- Section 5: Traffic Flow Diagrams
- Section 7: Testing Checklist

---

### 4. IPV6_ADDRESS_PLAN.md (86 lines)
**IPv6 Addressing and Allocation**

Detailed IPv6 address plan for both hosts.

---

## Quick Start

### For Firewall Administrators

1. **Initial Setup**: Read FIREWALL_TOPOLOGY.md Section 2 (topology diagram)
2. **Configuration**: Choose your platform:
   - **OPNsense**: Follow MONAD_HBH_FIREWALL_RULES.md Section 2
   - **IPFire**: Follow MONAD_HBH_FIREWALL_RULES.md Section 3
3. **Rule Deployment**: Use complete rulesets in INGRESS_EGRESS_PORTS.md Section 4
4. **Verification**: Test using Section 5 of MONAD_HBH_FIREWALL_RULES.md

### For DevOps/SRE

1. **Port Reference**: INGRESS_EGRESS_PORTS.md Section 1 (master port registry)
2. **Monitoring**: INGRESS_EGRESS_PORTS.md Section 6 (logging and monitoring)
3. **Compliance**: All documents Section 8 (regulatory requirements)
4. **Troubleshooting**: MONAD_HBH_FIREWALL_RULES.md Section 6

### For Network Architects

1. **Topology**: FIREWALL_TOPOLOGY.md Section 2 (ASCII diagram)
2. **Addressing**: IPV6_ADDRESS_PLAN.md (full plan)
3. **Zones**: INGRESS_EGRESS_PORTS.md Section 3 (firewall zones)
4. **Scaling**: INGRESS_EGRESS_PORTS.md Section 8 (expansion plan)

---

## Critical Information

### Monad Protocol HbH Passthrough

**CRITICAL**: The Monad protocol relies on IPv6 Hop-by-Hop (HbH) extension headers (option type 0x1E) to transmit per-packet metrics. If firewalls drop or rewrite HbH headers:
- Monad checksums become invalid
- eBPF metrics are lost
- Byzantine consensus breaks
- All observability fails

**Fix**: Follow MONAD_HBH_FIREWALL_RULES.md Section 2 (OPNsense) or Section 3 (IPFire).

### Exposed Ports

Only these ports are accessible from WAN:
- **80/TCP**: HTTP redirect to HTTPS
- **443/TCP**: HTTPS gateway (TLS termination)
- **51820/UDP**: WireGuard east-west tunnel

All other ports (gRPC 50051–50067, Dashboard 8080/8443, DNS 53, etc.) are **blocked** at the firewall.

### Licensing

- **OPNsense**: BSD 2-Clause (permissive)
- **IPFire**: GNU GPL v3 (copyleft)
- **Unheaded**: MIT (copyleft compatible)

See FIREWALL_TOPOLOGY.md Section 8 for compliance details.

---

## Platform-Specific Notes

### OPNsense (FreeBSD/pf)

- Recommended for development
- Uses pf packet filter and HAProxy for TLS termination
- Complete rules in INGRESS_EGRESS_PORTS.md Section 4.1
- GUI configuration in MONAD_HBH_FIREWALL_RULES.md Section 2.2

### IPFire (Linux/nftables)

- Recommended for production
- Uses nftables packet filter and squid proxy
- Complete rules in INGRESS_EGRESS_PORTS.md Section 4.2
- Web UI configuration in MONAD_HBH_FIREWALL_RULES.md Section 3.2

### NixOS Deployment

- OPNsense/IPFire as libvirt QEMU VMs
- See FIREWALL_TOPOLOGY.md Section 7

### Docker Deployment

- OPNsense/IPFire as privileged containers with macvlan networking
- See FIREWALL_TOPOLOGY.md Section 7

---

## Testing and Validation

All tests documented in MONAD_HBH_FIREWALL_RULES.md Section 5:

```bash
# Capture Monad HbH packets
tcpdump -i eth0 'ip6 proto 0'

# Verify firewall rules (OPNsense)
pfctl -s rules | grep -i "hopopt\|exthdrs hbh"

# Verify firewall rules (IPFire)
nft list ruleset | grep "nexthdr 0"

# Test port reachability
curl -v https://10.20.0.1:443
timeout 5 nc -zv 10.20.0.1 8080  # Should fail (blocked)
```

---

## Regulatory Compliance

All documents include compliance mappings for:
- **SOC2 Type II**: Audit trails, change management
- **PCI-DSS 6.6**: Annual reviews, encryption
- **HIPAA Security Rule**: Access controls, audit logging
- **ISO 27001**: Encryption, authentication

---

## File Summary

| File | Lines | Purpose |
|------|-------|---------|
| FIREWALL_TOPOLOGY.md | 555 | Architecture, topology, configuration |
| MONAD_HBH_FIREWALL_RULES.md | 727 | Platform-specific HbH rules |
| INGRESS_EGRESS_PORTS.md | 802 | Port registry and complete rulesets |
| IPV6_ADDRESS_PLAN.md | 86 | IPv6 addressing |
| **README.md** | This file | Quick start and navigation |

**Total**: 2,170 lines of network architecture documentation

---

## Links and Resources

- **OPNsense Docs**: https://docs.opnsense.org/
- **IPFire Docs**: https://wiki.ipfire.org/
- **IPv6 RFC**: https://tools.ietf.org/html/rfc8200
- **Monad Protocol Spec**: See `docs/protocol/MONAD_SPEC.md`
- **Unheaded Project**: https://github.com/unheaded-kingdom/

---

## Document Maintenance

- **Version**: 1.0
- **Last Updated**: 2026-02-26
- **Maintained By**: Unheaded Development Team
- **License**: MIT
- **Review Cycle**: Annually or when firewall rules change

---

**Questions or Issues?** Contact the Unheaded Development Team or file an issue on GitHub.
