# Kingdom Mode Address Reclamation — Math Verification

Date: 2026-02-18
Status: Verified, applied to draft-02

## The Discrepancy

Draft-02 contained two tables computing reclaimed bits from different
assumptions:

1. **Fleet Size Table**: assumed /48 ULA prefix preserved → 80 free
   bits per address → `2 * (80 - host_bits)` reclaimed
2. **Selector Table**: assumed full reclamation → `2 * (128 - host_bits)`

Same 24 host bits produced 112 vs 208. Delta = 96 = exactly `/48 prefix * 2`.

## Resolution

On EVPN-VXLAN (the default Kingdom architecture), inner IPv6 addresses
are NOT routed at Layer 3. The outer encapsulation handles forwarding.
Inner addresses are identifiers on a flat L2 segment. The ULA prefix
carries zero information — every Kingdom node knows it a priori.

Therefore: **all bits beyond the host identifier are reclaimable.**

## Verified Formula

```
Reclaimed bits = 2 * (128 - host_bits)
```

Where `host_bits` = bits required to uniquely identify each node.

## Verified Numbers

### By Kingdom Mode Selector (K1:K0)

| K1:K0 | Mode    | Host Bits/addr | Free Bits/addr | Reclaimed (both) | Bytes |
|-------|---------|----------------|----------------|-------------------|-------|
| 00    | Default | N/A            | 0              | 0                 | 0     |
| 01    | /8      | 24             | 104            | 208               | 26    |
| 10    | /12     | 20             | 108            | 216               | 27    |
| 11    | /16     | 16             | 112            | 224               | 28    |

### By Fleet Size (sub-mode granularity)

| Fleet Size  | Host Bits/addr | Free Bits/addr | Reclaimed (both) | Bytes |
|-------------|----------------|----------------|-------------------|-------|
| 256         | 8              | 120            | 240               | 30    |
| 4,096       | 12             | 116            | 232               | 29    |
| 65,536      | 16             | 112            | 224               | 28    |
| 1,048,576   | 20             | 108            | 216               | 27    |
| 16,777,216  | 24             | 104            | 208               | 26    |

### Effective Register Budget (Monad + ERS)

| Mode    | Monad | ERS   | Total   |
|---------|-------|-------|---------|
| Default | 20 B  | 0 B   | 20 B    |
| /8      | 20 B  | 26 B  | 46 B    |
| /12     | 20 B  | 27 B  | 47 B    |
| /16     | 20 B  | 28 B  | 48 B    |
| 256-host| 20 B  | 30 B  | 50 B    |

## Routed Mode (no L2 overlay)

If the Kingdom routes at L3 (no VXLAN), the ULA prefix MUST be
preserved for the kernel FIB:

```
Reclaimed bits (routed) = 2 * (128 - prefix_len - host_bits)
```

Example: /48 ULA, 24-bit hosts → `2 * (128 - 48 - 24) = 112 bits`.

L2 overlay (EVPN-VXLAN) is RECOMMENDED to maximize ERS.

## Changes Applied to draft-02

1. Abstract: "up to 120 bits" → "up to 224 bits"
2. Address Space Analysis: unified on `2 * (128 - host_bits)` formula
3. ERS Layout: prefix no longer separate reserved section on VXLAN
4. ERS Semantics: 128-bit → 224-bit budget with updated bit offsets
5. Computational Completeness: "128+ bits" → "208 to 224 bits"
6. Section cross-references: 5.2→5.3, 5.5→5.6 (off-by-one fix)
7. Added routed-mode formula as fallback note
8. Added RECOMMENDED note for L2 overlay
