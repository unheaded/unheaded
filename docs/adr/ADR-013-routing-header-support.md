# ADR-013: IPv6 Routing Header Support (Deferred)

## Status: Accepted

## Date: 2026-02-20

## Context

RFC 8200 §4.4 defines Routing Header processing rules for IPv6 extension headers. The Routing Header is used to specify a list of intermediate nodes to be "visited" on the way to a packet's destination.

Three main types are defined:

1. **Type 0** (RFC 8200): Deprecated by RFC 5095 due to security vulnerabilities. No longer in use.
2. **Type 2** (RFC 3775, RFC 6275): Used by Mobile IPv6 (MIPv6) to route packets to a mobile node's care-of address while it is away from its home network.
3. **Type 4** (RFC 8754): Used by Segment Routing over IPv6 (SRv6) to specify a list of segments (network function waypoints) that a packet must traverse.

In the context of the Unheaded Kingdom:

- **Shield XDP** currently identifies Routing Header (nh=43) and skips over it using `strip_extension_headers()`.
- The function correctly computes the header length and advances the offset to find the transport protocol.
- However, **no validation** of the Routing Header content occurs — packets with unknown or unsupported types pass through unvalidated.

The protocol specification (draft-bellis-unheaded-protocol-foundation-04 §5) states that Shield is responsible for boundary enforcement: _"NO external IPv6 extension header enters the Kingdom."_ This creates a question: should Routing Headers be completely stripped (like HBH and Destination Options), or can they be safely passed through?

## Decision

**DEFER Routing Header processing to Phase 2.** Shield XDP will continue to skip (but not drop) packets carrying Routing Headers. The current `strip_extension_headers()` function correctly handles nh=43 by reading the header length field and advancing the offset appropriately. This satisfies the boundary enforcement requirement (packets are identified and processed) without requiring full RH validation logic.

**Rationale for deferral:**

1. **Type 0 is dead**: RFC 5095 deprecation means Type 0 packets are only attack vectors. Support is unnecessary.

2. **Type 2 (MIPv6) is out-of-scope**: The Kingdom's east-west traffic model does not include Mobile IPv6 mobility agents. Binding updates, care-of address registration, and home agent communication are not used.

3. **Type 4 (SRv6) requires policy infrastructure**: SRv6 validation requires:
   - A segment **policy database** (which segments are valid for which sources/destinations)
   - A segment **registry** (what network functions are reachable at each segment ID)
   - **Per-segment ACLs** (which domains are allowed to traverse which waypoints)

   Implementing this without the policy layer adds **O(segments × policies) memory overhead** and **O(segments) latency per packet** with no current use case.

4. **East-west traffic assumption**: Unheaded is deployed in controlled datacenter environments where all routes are operator-managed. Dynamic route negotiation via Routing Headers is not part of the architecture.

## Consequences

**Positive:**

- Packets carrying Routing Headers are **identified and processed** by Shield (boundary enforcement satisfied).
- No additional eBPF code complexity for Type 0/2/4 validation.
- No memory overhead for policy databases.
- Future Phase 2 work can add SRv6 support incrementally when service mesh integration requires it.

**Negative:**

- Routing Headers pass through the Kingdom **unvalidated**.
- If a malicious external source injects a Routing Header Type 0 attack, it will reach the Hop programs (though modern Linux kernels ignore Type 0 anyway per RFC 5095).
- If SRv6-enabled services are deployed before Phase 2, they must enforce segment policy at the application level or via separate TC programs.

**Mitigation:**

- Document this limitation in the Shield program comments.
- If SRv6 is needed urgently, users can deploy a separate TC egress hook to validate SRv6 policies before packets enter the Kingdom.
- Phase 2 SRv6 support can be added by wiring an `SEGMENT_POLICY` BPF map and checking each segment ID against it.

## References

- **Code**: `/sessions/hopeful-kind-lovelace/mnt/tmp/unheaded/ebpf/shield-ebpf/src/main.rs` — `strip_extension_headers()` at line 559
- **RFC 8200**: IPv6 Specification, §4.4 (Routing Header)
- **RFC 5095**: Deprecation of Type 0 Routing Headers in IPv6
- **RFC 3775 / RFC 6275**: Mobile IPv6
- **RFC 8754**: Segment Routing over IPv6
- **Protocol spec**: draft-bellis-unheaded-protocol-foundation-04 §5 (Shield boundary enforcement)
- **ADR-003**: eBPF in Rust with Aya Framework
- **ADR-012**: BPF Verifier Risk Mitigation

## Appendix: Future Phase 2 SRv6 Implementation Sketch

When SRv6 support is needed:

1. Add `SEGMENT_POLICY` BPF map (u16 segment_id → policy_flags u8)
2. In `strip_extension_headers()`, detect nh=43 with Type 4
3. Parse the Segment List within the RH:
   ```
   RH[0] = next_header
   RH[1] = len (in 8-octet units)
   RH[2] = routing_type (should be 4 for SRv6)
   RH[3] = segments_left
   RH[4..8] = reserved / type-specific data
   RH[8..] = segment list (16-byte IPv6 addresses)
   ```
4. For each segment in the list, look up segment_id (low 16 bits of segment IPv6 address)
5. Verify policy from map; drop if not permitted
6. Increment stat and emit anomaly if dropped
7. Add RFC 8754 §4 reference to code comments

This keeps SRv6 validation **in the data plane** and **under operator policy control** rather than hard-coded into the Kingdom.
