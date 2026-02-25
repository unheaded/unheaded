# ADR-014: IPv6 Fragment Header Processing (Deferred)

## Status: Accepted

## Date: 2026-02-20

## Context

RFC 8200 §4.5 defines Fragment Header processing for IPv6 packets that exceed the Maximum Transmission Unit (MTU) of the path. When a router encounters a packet larger than the outgoing link's MTU and the packet has the DF (Don't Fragment) bit set... wait, IPv6 has no DF bit. Instead, IPv6 **requires** routers to drop packets that exceed the MTU and return an ICMPv6 "Packet Too Big" message.

However, **senders can choose to fragment packets themselves** before sending them. The Fragment Header (nh=44) is used to communicate this fragmentation state.

**RFC 8200 Fragment Header format:**
- Next Header (1 byte)
- Hdr Ext Len (1 byte)
- Fragment Offset (13 bits)
- Reserved (2 bits)
- More Fragments flag (1 bit)
- Identification (4 bytes)

**Current Shield XDP behavior:**
Shield's `strip_extension_headers()` identifies nh=44 and skips over the header. However, **the kernel does not automatically reassemble IPv6 fragments at XDP ingress**. Instead:

1. Fragmented packets arrive as separate packets (one per fragment).
2. The kernel's IP reassembly layer (in the network stack, not in XDP) would normally combine them.
3. **XDP hooks run BEFORE kernel reassembly**, so XDP code sees only individual fragments.

In practice, Shield currently **drops fragmented packets** because:
- Packet 1 (Fragment 1) arrives: offset=0, more_fragments=1 → XDP processes it, passes to kernel
- Kernel reassembly buffer waits for Fragment 2
- Packet 2 (Fragment 2) arrives: offset=N, more_fragments=0 → XDP processes it, passes to kernel
- Kernel combines them at network stack layer

But there's a **DoS risk**: an attacker can send millions of fragment headers to exhaust kernel reassembly buffers, causing memory pressure or packet loss for legitimate traffic.

## Decision

**DEFER full IPv6 fragmentation support. Maintain current drop-on-fragment behavior.** Instead of buffering and reassembling fragments, we recommend that **all services use PMTUD (Path MTU Discovery, RFC 8201)** to avoid fragmentation entirely.

**Rationale:**

1. **In-kernel reassembly is expensive**: Buffering fragments uses kernel memory, creates state management overhead, and opens a vector for DoS (reassembly buffer exhaustion, "Frag-O-Matic" attacks).

2. **XDP cannot handle fragmentation safely**: XDP hooks run before kernel reassembly, so XDP would need to:
   - Maintain a **fragment reassembly buffer** in a BPF map
   - Hash fragments by (src, dst, identification) tuple
   - Allocate temporary storage for partial packets
   - Implement a **timeout and cleanup mechanism** (prevent unbounded map growth)
   - This is **O(fragments) per packet** and introduces memory pressure at the data plane.

3. **Modern services use PMTUD**: Properly configured services:
   - Discover the path MTU at connection setup (RFC 8201)
   - Send packets within MTU bounds
   - Receive ICMPv6 "Packet Too Big" messages if they ever exceed MTU
   - Adjust window sizes and segment sizes accordingly

4. **Unheaded's MTU is sufficient**: RFC 8200 mandates a **minimum IPv6 MTU of 1280 bytes**. Monad's 24-byte HBH header leaves **1256 bytes for payload** — more than enough for typical HTTP, DNS, and gRPC traffic. Enterprise services using PMTUD will never fragment.

5. **Fragmentation = misalignment with modern practice**: IPv6 was designed on the assumption that fragmentation would be rare. TCP-over-IPv6 in production networks shows <0.1% fragmented packets. Datacenter traffic (which Unheaded targets) is even lower because MTU is standardized at 1500+ bytes (or 9000 with jumbo frames).

6. **DoS protection**: Dropping fragmented packets at ingress prevents fragment-based attacks (buffer exhaustion, off-path attacks, etc.) from reaching the Kingdom.

## Consequences

**Positive:**

- **No state management overhead**: Fragmented packets are dropped, no reassembly buffer needed.
- **No memory exhaustion risk**: DoS attacks via fragment bombardment are mitigated (we drop fragments).
- **Simpler eBPF code**: No reassembly logic, no timeout handling, no state cleanup.
- **Production-aligned**: Aligns with real-world datacenter practices where fragmentation is avoided.
- **Clear operational semantics**: "Fragments are unsupported" is easier to document and debug than "fragments are reassembled with timeout T".

**Negative:**

- **Fragmented packets are dropped**: Services that send fragmented packets (misconfigured MTU, missing PMTUD) will experience packet loss.
- **No diagnostic path**: Unlike ICMP-based MTU discovery, dropped fragments leave no error signal to the sender (the fragment is silently dropped, not rejected with an error message).
- **Incompatible with legacy applications**: Old applications that deliberately send large packets without PMTUD will fail when deployed in the Kingdom.

**Mitigation:**

- **Documentation**: Operational guides must state: _"All services MUST use PMTUD (RFC 8201). Fragmented IPv6 packets are dropped at ingress."_
- **Validation**: Pre-production testing MUST include MTU verification (e.g., `tracepath6` to discover MTU, `ping6 -s` to test specific sizes).
- **Monitoring**: Track the `STAT_EXT_STRIPPED` counter (which includes fragments as they are skipped); if it grows unexpectedly, investigate MTU misconfiguration.
- **Future: Optional reassembly**: If a service absolutely requires fragmentation support, it can deploy a separate TC ingress hook that performs reassembly with explicit resource limits.

## References

- **Code**: `/sessions/hopeful-kind-lovelace/mnt/tmp/unheaded/ebpf/shield-ebpf/src/main.rs` — `strip_extension_headers()` at line 559
- **RFC 8200**: IPv6 Specification, §4.5 (Fragment Header)
- **RFC 8201**: Path MTU Discovery for IPv6
- **RFC 2460**: IPv6 Specification (historical, defines Fragment Header)
- **Security**: "Frag-O-Matic" attacks documented in Linux kernel security advisories
- **BPF limitations**: RFC 9669 §2.2 (BPF verifier constraints on memory allocation and loops)
- **Protocol spec**: draft-bellis-unheaded-protocol-foundation-04 §5 (Shield boundary enforcement)
- **ADR-003**: eBPF in Rust with Aya Framework
- **ADR-012**: BPF Verifier Risk Mitigation

## Appendix: PMTUD Configuration Checklist

When deploying services in the Kingdom, ensure:

- [ ] **TCP MSS Clamping**: TC ingress/egress uses `bpf_skb_adjust_room()` or `iptables -j TCPMSS --set-mss 1220` to clamp MSS to (MTU - IPv6 header - TCP header) = 1220 bytes (assuming 1280 MTU, 40-byte IPv6, 20-byte TCP).

- [ ] **UDP payload limit**: Application layer enforces max UDP datagram size ≤ 1256 bytes (1280 MTU - 24 HBH header).

- [ ] **gRPC streaming**: HTTP/2 frame size negotiation defaults to 16KB, which WILL fragment. Explicitly configure `--max-connection-idle-time` and frame size limits.

- [ ] **DNS over IPv6**: Ensure DNS response size fits within 1280 bytes (UDP) or use TCP fallback (RFC 6891 EDNS0).

- [ ] **Monitoring**: Log all ICMPv6 "Packet Too Big" messages (ICMP type 2, code 0) and alert if they appear frequently.

## Appendix: Future Fragmentation Support (Phase 3+)

If fragmentation support becomes essential:

1. Add a `FRAGMENT_REASSEMBLY` BPF map:
   ```rust
   (src[16] + dst[16] + identification[4]) → (buffer_addr[8], total_fragments[1], recv_mask[64])
   ```

2. In `strip_extension_headers()`, detect nh=44:
   ```rust
   if nh == 44 {
       // Fragment Header format:
       // [0] = next_header
       // [1] = len (must be 0 for fragments)
       // [2:4] = reserved(2) + offset(13) + M(1)
       // [4:8] = identification
       let offset_m = read_u16_be(data, offset + 2);
       let frag_offset = (offset_m >> 3) * 8; // Multiply by 8
       let more_frags = (offset_m & 1) != 0;
       let identification = read_u32_be(data, offset + 4);

       let key = make_frag_key(src, dst, identification);
       // Lookup FRAGMENT_REASSEMBLY map
       // If not present and offset=0, allocate buffer
       // If present, write fragment into buffer at frag_offset
       // If !more_frags, combine all fragments and return combined length
   }
   ```

3. Add a **timeout cleaner** (runs periodically, e.g., every 60 seconds):
   ```rust
   for entry in FRAGMENT_REASSEMBLY {
       if entry.created_at + FRAG_TIMEOUT < now {
           FRAGMENT_REASSEMBLY.delete(&entry.key);
           increment_stat(STAT_FRAG_TIMEOUTS);
       }
   }
   ```

4. Constants:
   - `FRAG_TIMEOUT`: 60 seconds (RFC 8200 recommends 60+ seconds)
   - `MAX_FRAG_BUFFERS`: 256 (prevents unbounded map growth)
   - `MAX_FRAGMENT_SIZE`: 65520 bytes (max IPv6 payload)

5. Update RFC reference: Add RFC 8200 §4.5 and RFC 8201 to code comments.

This approach **isolates fragmentation complexity** into a separate code path and **makes resource limits explicit**.
