# ADR-009: Binding Circles — RFC 1918 Range Map as Security Factor

## Status: Deferred to Beta

**Deferral rationale (2026-04-03):** Optional third authentication factor via RFC 1918 range coloring. Not blocking public launch or Age 3. Will revisit when auth framework needs hardening beyond APIKey/JWT/RBAC (ADR-051).

## Date: 2026-02-18

## Naming Convention

In Lich lore, **Binding Circles** are inscribed zones of power that define where the lich's influence operates and where its magic holds.  Step outside the circle and the binding weakens.  In the Kingdom, Binding Circles are RFC 1918 address ranges that define semantic trust zones — step outside your circle and Shield drops you.

| Term | Meaning |
|------|---------|
| **Binding Circle** | A dedicated RFC 1918 range mapped to a semantic trust zone |
| **Circle Map** | The Sophia dictionary mapping CIDRs to circles |
| **Circle Breach** | Traffic from outside its authorized circle — an anomaly |

## Context

The Phylactery's Two-Seal model (Sigil + Ward) provides orthogonal verification: packet-borne authorization plus application-layer identity.  This ADR proposes a third, optional verification dimension — **address coloring** — where the RFC 1918 IP address of a packet itself carries semantic meaning within the Kingdom's trust model.

Traditional subnetting divides address space by topology: how many hosts, which broadcast domain.  Binding Circles divide address space by **semantic trust zone**.  A /16 might contain only 3 hosts.  The point is not efficient address utilization — it is that the address ITSELF is a weak authentication factor.  Someone in blue armor in the red wing of the castle is suspicious before you even check their badge.

This is NOT a replacement for Sigil or Ward.  It is a nearly-free additional signal (~12ns per packet in BPF) that makes the entire system harder to attack.

### Problem

A compromised service with a valid Sigil and Ward could theoretically issue storage operations from any address within the mesh.  While the Two-Seal model prevents unauthorized operations (both seals must align), there is no mechanism to detect that a PHYLACTERY_STORE packet originated from a service mesh address (10.4.x.x) rather than a storage-designated address (10.1.x.x).  This is a detectable anomaly that currently goes undetected.

## Decision

### The Circle Map

Dedicate RFC 1918 /16 ranges to Binding Circles managed by Sophia:

```
BINDING CIRCLE MAP — "The Lich's Zones of Power"

10.0.0.0/8 — The Kingdom's Private Realm

  10.1.0.0/16   → Circle: SANCTUM (Phylactery storage)
                   Soul Chamber read/write traffic only
                   Shield DROP anything else on this range

  10.2.0.0/16   → Circle: THRONE (Control plane)
                   Cuirass, Sophia, Pleroma/Kenoma
                   No data payloads allowed

  10.3.0.0/16   → Circle: ALL_SEEING (Observability)
                   Anamnesis, Whispering Void, dashboards
                   Read-only data flows, no mutations

  10.4.0.0/16   → Circle: HAUBERK (Service mesh)
                   East-west traffic
                   User app-to-app communication

  10.5.0.0/16   → Circle: GATEHOUSE (Ingress)
                   Shield → Pauldrons → user app
                   North-south traffic only

  10.10.0.0/16  → Circle: ABYSS (Chaos)
                   Yaldabaoth testing zone
                   Fully isolated, firewalled, expendable

  10.200.0.0/16 → Circle: SOUL_BRIDGE (Cross-Kingdom)
                   Kingdom-to-Kingdom VXLAN tunnels
                   Soul Split replication traffic
```

### Three-Factor Verification (Optional Mode)

```
UNLOCK = Binding Circle_valid(src_ip, dst_ip, operation)
         AND Sigil_valid(packet)
         AND Ward_valid(payload)

Binding Circle alone:   address in range but no crypto proof     → REJECT
Sigil alone:    crypto valid but wrong address zone       → ANOMALY + configurable DROP
Ward alone:     identity proven but unauthorized path     → REJECT
All three:      proceed
```

### BPF Implementation

The Binding Circle check is an O(1) LPM (Longest Prefix Match) trie lookup in BPF.  Cost: ~12ns additional per packet.

```c
// BPF LPM trie for Binding Circle lookup
struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __type(key, struct lpm_key);    // prefix_len + addr
    __type(value, struct circle);   // circle_id + allowed_ops bitmask
    __uint(max_entries, 256);
    __uint(map_flags, BPF_F_NO_PREALLOC);
} circle_map SEC(".maps");

// Per-packet check (inside existing Shim path)
static __always_inline int check_circle(u32 src_ip, u32 dst_ip, u8 operation) {
    struct lpm_key key = { .prefix_len = 16, .addr = dst_ip };
    struct circle *p = bpf_map_lookup_elem(&circle_map, &key);
    if (!p) return PARISH_UNKNOWN;  // not in any circle — anomaly
    if (!(p->allowed_ops & (1 << operation))) return PARISH_VIOLATION;
    return PARISH_OK;
}
```

### Sophia Integration

The range map is a Sophia dictionary entry, distributed via Wotan:

```
sophia.register_type("kingdom.binding_circle", {
    key:   "cidr_prefix",            // "10.1.0.0/16"
    value: "circle_name:allowed_ops:trust_level:epoch",
})
```

Binding Circle assignments are **rotatable**.  If a zone is compromised:

1. Sophia pushes new circle map with remapped ranges
2. Wotan distributes to all BPF programs in <10ms
3. Every LPM trie updates atomically
4. Compromised range becomes a dead zone
5. Attacker's hardcoded IPs are worthless

### Anamnesis Events

```
EVENT_CIRCLE_OK          0x40   Address matched circle + allowed ops
EVENT_CIRCLE_VIOLATION   0x41   Address in circle but operation not allowed
EVENT_CIRCLE_UNKNOWN     0x42   Address not in any circle (rogue node)
EVENT_CIRCLE_REMAP       0x43   Binding Circle map updated via Sophia push
```

## Consequences

### Positive

- Nearly free additional security signal (~12ns per packet in BPF)
- Detects anomalies invisible to Sigil/Ward (wrong-zone traffic)
- Binding Circle map rotation provides operational agility during incidents
- Integrates cleanly with existing Sophia/Wotan distribution
- Orthogonal to both Seals — compromising one doesn't help with others

### Negative

- Address space is "wasted" from a traditional networking perspective (/16 per zone is generous)
- Requires coordination between network fabric and Sophia map
- False positives during zone migration (must update map BEFORE moving services)
- Another map to manage — operational complexity

### Neutral

- Does not replace Sigil or Ward — supplementary only
- Works best with static-ish service placement; highly dynamic scheduling may cause churn
- Binding Circle remapping is an incident response tool, not a routine operation

## Priority

**Wishlist — Beta phase.**  The Phylactery is fully functional without Binding Circle Boundaries.  The Two-Seal model is the trust foundation.  Binding Circle Boundaries are defense-in-depth frosting.

## Related

- PHYLACTERY.md — Optional Third Factor section
- ADR-008 — Security Hardening Baseline
- ADR-010 — Sealed Cask Deployment Model (label carries circle assignment)
