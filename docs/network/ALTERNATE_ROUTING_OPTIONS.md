# Alternate Routing Options — Unheaded Kingdom

## Overview

The Unheaded Kingdom supports 4 routing options, all switchable at deployment time
via `scripts/routing/select-routing.sh`. All options preserve Monad HbH extension headers.

## Comparison Table

| Feature | BGP EVPN (default) | OSPFv3 (Option A) | IS-IS + SR-MPLS (Option B) | MPLS LDP (Option C) |
|---------|-------------------|-------------------|---------------------------|---------------------|
| Protocol | BGP + EVPN | OSPFv3 | IS-IS level-2 + SR | MPLS + LDP |
| Complexity | High | Low | Medium | High |
| AS Numbers | Required | Not needed | Not needed | Not needed |
| L2 extension | Yes (VXLAN) | No (L3 only) | No (L3 only) | No (L3 only) |
| Convergence | ~30s | ~5s (SPF) | ~3s (IS-IS) | ~5s |
| TE support | ECMP only | ECMP only | SR-TE (path steering) | Full TE (LSPs) |
| Monad HbH safe | ✅ transparent | ✅ transparent | ✅ inner payload | ✅ inner payload |
| Container support | All platforms | All platforms | All platforms | All platforms |
| Bare metal req | No | No | Kernel MPLS modules | Kernel MPLS modules |
| FRR daemons | bgpd,isisd,bfdd | ospf6d,bfdd | isisd,bfdd | ldpd |
| BIRD peer | Yes (AS65002) | Yes (OSPFv3) | Not needed | Not needed |
| File | frr/frr.conf | ospf/frr-ospf.conf | isis/frr-isis-ha.conf | mpls/frr-mpls.conf |

## Monad HbH Safety — All Options

All routing protocols are transparent to IPv6 extension headers:
- **FRR/BIRD**: Route lookup uses outer IPv6 destination address only. HbH headers are not examined, modified, or stripped by routing logic.
- **MPLS**: Label stack is the outer header. IPv6 + HbH extension headers are encapsulated as inner payload. Label push/pop operations do not touch inner payload. (RFC 3031 §3.9, RFC 6232)
- **VXLAN**: Inner IPv6 packet (including HbH) is payload of outer UDP/IP VXLAN frame. No modification to inner headers.

## Switching Routing Options

```bash
# Switch to OSPFv3
sudo scripts/routing/select-routing.sh ospf
sudo systemctl restart frr

# Switch back to BGP EVPN
sudo scripts/routing/select-routing.sh bgp-evpn
sudo systemctl restart frr
```

## Files

```
routing/
├── frr/          BGP EVPN (default)
├── ospf/         OSPFv3 — Option A
├── isis/         IS-IS + SR-MPLS — Option B
└── mpls/         MPLS LDP — Option C

nixos/modules/
├── frr.nix       BGP EVPN module
├── frr-ospf.nix  OSPFv3 module
├── frr-isis.nix  IS-IS module (TODO)
└── frr-mpls.nix  MPLS module

scripts/routing/
└── select-routing.sh  Live switcher
```
