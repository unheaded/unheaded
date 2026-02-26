# IS-IS + SR-MPLS Alternate Routing — Option B

## When to Use
- You want segment routing traffic engineering (SR-TE)
- Future MPLS label-switched paths (LSPs) between hosts
- IS-IS level-2 for a flat, single-area design

## IS-IS Parameters
- NET: 49.0001.1020.0255.000{1,2}.00 (host-{a,b})
- Level: level-2-only (single area, no level-1 complexity)
- Metric-style: wide (required for SR extensions TLV 22)
- IPv6: RFC 5308 TLV 236 natively supported

## SR-MPLS Parameters
- Global block: 16000-23999 (SRGB)
- Local block: 15000-15999 (SRLB)
- host-a prefix-SID: index 1 → label 16001
- host-b prefix-SID: index 2 → label 16002

## Monad HbH Safety
✅ MPLS label stack is OUTER header.
✅ IPv6 + HbH extension headers are INNER payload.
✅ Label push/pop does not touch the IPv6 layer.
✅ Monad survives end-to-end across MPLS segments.

## Activate
```bash
./scripts/routing/select-routing.sh isis
```
