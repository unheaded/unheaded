# OSPFv3 Alternate Routing — Option A

## When to Use

OSPFv3 is the **simple option** — no AS numbers, no BGP, no EVPN complexity.
Choose OSPFv3 when:
- Running a basic 2-host lab (no multi-site scale needed)
- No Layer 2 extension requirements (containers route via L3 only)
- Fastest convergence (SPF) without BGP timer complexity
- No traffic engineering requirements

## How to Activate

```bash
# Switch to OSPFv3 (restarts FRR and BIRD)
./scripts/routing/select-routing.sh ospf
```

## Architecture

```
host-a (Forge, 10.20.255.1)          host-b (Outpost, 10.20.255.2)
  FRR OSPFv3 area 0.0.0.0              BIRD OSPFv3 area 0.0.0.0
       |                                      |
       +-------- wg0 (cost 10) ---------------+
       |         fd00:dead:beef::/48           |
       |         BFD 300ms detect              |
       |                                       |
   br-unheaded                            br-unheaded
   10.20.0.0/16 (passive)               10.20.0.0/16 (passive)
   fd00:dead:beef:1::/64                fd00:dead:beef:2::/64
```

## Known Limits

- **No EVPN**: L3 routing only. No L2 bridge extension across hosts.
- **No traffic engineering**: ECMP only (no SR-TE, no MPLS labels).
- **No AS-path filtering**: All routes trusted equally (fine for 2-host lab).
- **MTU**: wg0 MTU 1380 requires `mtu-ignore` on FRR + `check link yes` on BIRD.
- **Monad HbH**: ✅ Safe — OSPFv3 routing layer is transparent to extension headers.

## Comparison

| Feature | BGP EVPN (default) | OSPFv3 (this) |
|---------|-------------------|----------------|
| Complexity | High | Low |
| L2 extension | Yes (VXLAN) | No |
| TE support | ECMP | ECMP |
| Convergence | ~30s (BGP timers) | ~5s (SPF) |
| Monad HbH safe | ✅ | ✅ |
| AS numbers needed | Yes | No |

## Files

- `frr-ospf.conf` → FRR OSPFv3 for host-a
- `bird-ospf.conf` → BIRD OSPFv3 for host-b
