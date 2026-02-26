# MPLS LDP/RSVP-TE Alternate Routing — Option C

## When to Use
Full traffic engineering with label-switched paths (LSPs).

## Monad HbH Safety — CRITICAL
✅ MPLS label stack is the **outer** header (prepended to the packet).
✅ Original IPv6 + HbH extension headers are the **inner** payload.
✅ MPLS forwarding plane reads label stack ONLY — never touches inner IPv6.
✅ Label push: [MPLS-Label][IPv6][HbH-Monad][Payload] — Monad intact.
✅ Label pop: [IPv6][HbH-Monad][Payload] — Monad intact.
✅ Standard behavior per RFC 3031 §3.9 and RFC 6232.

## Activate
```bash
sudo ./routing/mpls/setup-mpls-kernel.sh wg0  # bare metal only
./scripts/routing/select-routing.sh mpls
```
