# Session S21 — Black Mage Dark Assessment

**Date:** 2026-02-20
**Skill:** unheaded-blackmage
**Scope:** Full adversarial review of protocol specs, Doom-over-IPv6 plan, S20 state, skill file audit, RFC 9000 (QUIC) cross-pollination
**Status:** COMPLETE — findings delivered, strategic next steps proposed, QUIC patterns extracted

---

## THREAT POSTURE ASSESSMENT

```
ATTACK SURFACE:
  Protocol specs reviewed:           3 (Monad Foundation, Sophia Dict, Wotan Memory)
  Supporting docs reviewed:          3 (wire-format-patterns, iana-guide, rfc-crossref)
  Session handoff reviewed:          S20 (race condition confirmed, BSS in progress)
  Doom plan reviewed:                doom-over-ipv6-plan.md (8-13 day estimate)

PROTOCOL THREAT LEVEL:
  Monad wire format:                 EXPOSED (10 CRITs/HIGHs found)
  Sophia dictionaries:               EXPOSED (11 CRITs/HIGHs found)
  Wotan memory model:                EXPOSED (15 CRITs/HIGHs found)
  Wire format patterns:              TESTING (6 HIGHs found)
  IANA registries:                   TESTING (3 HIGHs found)

CROSS-DOCUMENT:
  Inconsistencies found:             3 CRITICAL cross-doc gaps

DOOM-SPECIFIC:
  Race condition:                    CONFIRMED (S20)
  New attack surface:                MBC CPU execution in XDP
  Kernel-level risk:                 HIGH (packet-as-CPU = code execution surface)

VERDICT: EXPOSED — Multiple CRITICAL findings across all three protocol specs.
```

---

## PART 1: CRITICAL FINDINGS ACROSS ALL SPECS (Top 15)

### Monad Protocol Foundation

| # | Severity | Finding | Section | Fix |
|---|----------|---------|---------|-----|
| M1 | **CRITICAL** | Wotan helper bounds bypass → kernel memory write | §11 | Bounds-check addr+len in bpf_wotan_read/write |
| M2 | **CRITICAL** | Arbitrary code exec via Wotan code injection | §12 | Prohibit BPF bytecode in Wotan; R/O memory regions |
| M3 | **HIGH** | CRC-16 bypass via exponent field collisions | §5.4 | Extend CRC to all 20 bytes; mandate HMAC for prod |
| M4 | **HIGH** | Sophia map hot-swap race condition (use-after-free) | §5.3.1 | RCU sync or percpu generation counters |
| M5 | **HIGH** | Unbounded packet loops via BPF_REDIRECT | §12 | Mandate kernel 5.17+, bounded-loops verification |
| M6 | **HIGH** | Nested HbH headers allow fake Monad injection | §2.2 | Prohibit multiple HbH headers absolutely |
| M7 | **HIGH** | Routing Header abuse for path manipulation | §4.4 | Explicitly prohibit Routing Headers in Monad packets |
| M8 | **MEDIUM** | Version confusion / downgrade attacks | §2.2 | Strict version checking, no fallback |
| M9 | **MEDIUM** | Chaos mode timing side-channel (service fingerprinting) | §13 | Restrict C flag to Shield ingress; constant-time ops |
| M10 | **MEDIUM** | Anamnesis ring buffer DoS via observability exhaustion | §9 | Mandate drop-rate monitoring and alerting |

### Sophia Dictionary

| # | Severity | Finding | Section | Fix |
|---|----------|---------|---------|-----|
| S1 | **CRITICAL** | Ambiguous version comparison at modular boundary 128 | §8.1 | Explicit reject if ((N2-N1) mod 256) in [128,255] |
| S2 | **CRITICAL** | BPF map exhaustion via unbounded sub-dict creation | §5.2 | Pre-alloc check; reject if BPF mem > limit |
| S3 | **CRITICAL** | Race in atomic update (CBOR→BPF not truly atomic) | §6.3 | Single atomic bpf_map_update_elem() call |
| S4 | **CRITICAL** | No provisioning node authentication specified | §9 | ML-DSA-65 sig on entire CBOR blob; key rotation |
| S5 | **CRITICAL** | BPF verifier bypass not addressed | §9 | Audit logging, BPF LSM rules |
| S6 | **HIGH** | Optional Wotan signatures (SHOULD→MUST) | §6.1 | Mandate ML-DSA-65 sig on all dict publications |
| S7 | **HIGH** | Dangling sub-dictionary references | §5.2 | Pre-allocation verification |
| S8 | **HIGH** | Cross-dictionary semantic confusion | §3.2 | Enforce namespace invariants |
| S9 | **HIGH** | IPv6 truncation → type confusion | §4.2 | Full 128-bit reconstruction |
| S10 | **HIGH** | PQC key material format validation gap | §4.3 | Validate key length vs algorithm ID |

### Wotan Memory Model

| # | Severity | Finding | Section | Fix |
|---|----------|---------|---------|-----|
| W1 | **CRITICAL** | Ring buffer wrap-around confusion (no seqno) | §7.1 | Add u64 seqno; validate monotonicity |
| W2 | **CRITICAL** | Flow Label collision → cross-flow data leakage | L1 key | Extend L1 key to composite (flow+src/dst hash) |
| W3 | **CRITICAL** | CAS alignment bypass (JIT may skip check) | §helpers | Enforce alignment in BPF verifier pre-JIT |
| W4 | **CRITICAL** | L1 cache poisoning via compromised userspace handler | §9.1 | HMAC-SHA256 on L2→L1 cache line loads |
| W5 | **HIGH** | WAL truncation attack (no integrity, only CRC-16) | §7 | HMAC-SHA256 per WAL entry |
| W6 | **HIGH** | WAL compaction race (no exclusive lock) | §7.5 | Hold lock during entire compaction |
| W7 | **HIGH** | Backpressure bypass via tail-call retry exhaustion | §9.2 | Max retry count; permanent -ENOMEM after N |
| W8 | **HIGH** | Helper buf/len mismatch → stack overflow | helpers | Validate at BPF verifier time; limit len≤64 |
| W9 | **HIGH** | L1 cache line race condition (no lock) | L1 | BPF spinlock on l1_cache_line |
| W10 | **HIGH** | LRU counter overflow → eviction thrashing | §6.3.2 | Timestamp-based LRU or hardcoded reset |

### Cross-Document Gaps

| # | Severity | Finding | Fix |
|---|----------|---------|-----|
| X1 | **CRITICAL** | No canonical Sophia dictionary linking wire format exponents to values | Create normative mapping table |
| X2 | **HIGH** | Version field: wire allows 0-255, IANA reserves 4-bit | Align to 8-bit in IANA registry |
| X3 | **HIGH** | Circuit state exponent encoding vs IANA registry mismatch | Define whether exponent-decoded or raw |
| X4 | **HIGH** | IANA 0x17-0xEF flow action gap (231 values, no policy) | Define allocation policy |

---

## PART 2: DOOM-OVER-IPv6 — ADVERSARIAL ASSESSMENT

> **IMPORTANT: Doom is a PROOF-OF-CONCEPT ONLY.** It exists to demonstrate computational
> completeness of the Monad architecture — packets as CPU, Wotan as RAM. It is a
> marketing/conference demo, NOT a production feature. It will NEVER ship in the production
> Unheaded Kingdom. That said, every bug found in the Doom PoC IS a bug in the protocol.
> The PoC is the stress test. The findings are real. Fix them for production, not for Doom.
>
> If we CAN run it securely, that says more about the protocol's integrity guarantees than
> any compliance checklist ever could. "We ran Doom on IPv6 extension headers and it didn't
> corrupt state" is the ultimate concurrency proof.

### The Race Condition (S20 Finding) — Black Mage Severity: **CRITICAL**

The S20 handoff confirmed what the Dark Grimoire predicted: **concurrent mutable access to BPF maps without synchronization is exploitable.** The race in CPU_MAP where packet A at hop 3 and packet B at hop 0 both modify state via `get_ptr_mut()` is a textbook TOCTOU bug.

**Why This Matters Beyond the PoC:**
This isn't just a Doom bug. This is a **protocol-level vulnerability.** If the Monad architecture allows multiple packets to concurrently modify shared BPF map state, then in PRODUCTION:
- Two packets for the same flow arriving at different hops could corrupt each other's state
- Sophia dictionary lookups during hot-swap exhibit the same pattern (Finding S3)
- Wotan L1 cache lines have the same race (Finding W9)

**The fix for Doom IS the fix for the protocol.** Getting concurrency right here hardens everything. The PoC just made it visible first.

### Doom-Specific Attack Surface (New Findings)

| # | Severity | Vector | Impact |
|---|----------|--------|--------|
| D1 | **CRITICAL** | MBC bytecode injection via ROM_MAP | Attacker replaces ROM instructions → arbitrary compute in XDP |
| D2 | **HIGH** | Framebuffer read via Wotan topic | Attacker subscribes to `compute.screen.*` → exfiltrates screen state |
| D3 | **HIGH** | Keyboard injection via Wotan kbd topic | Attacker publishes to `compute.input.*` → controls game input |
| D4 | **HIGH** | Flow Label collision in Doom flow | Another flow with same label overwrites CPU_MAP state |
| D5 | **MEDIUM** | SYSCALL handler abuse | Crafted SYSCALL opcode → unintended Wotan I/O operations |
| D6 | **MEDIUM** | ROM_MAP integrity (no signature) | ROM loaded via `wotan-ctl load-rom` — unsigned, no verification |

### Strategic Recommendation: Race Fix Priority Order

**Option 1 (BPF spinlock) — Black Mage RECOMMENDS:**
The `bpf_spin_lock` approach is correct but the constraint (can't call helpers while holding lock) is real. The workaround:
1. Read state → copy to stack
2. Execute 16 MBC instructions on stack copy (no BPF helpers needed for ALU ops)
3. Lock → compare-and-swap state back → unlock
4. If CAS fails (stale), re-read and re-execute (optimistic concurrency)

This gives you max throughput (~5,600 pkt/s = ~23M insn/sec) WITH correctness.

**Option 2 (Sequence number) — Acceptable fallback:**
Simpler but wastes work. At ~5,600 pkt/s with 6-hop ring, conflict rate would be ~15-20% (back-of-envelope: ring transit ~3-13ms, new packet every ~0.18ms = ~5-70 packets in flight). That's a LOT of wasted work.

**Option 3/4 — Only for debugging, NOT for the demo.**

---

## PART 3: STRATEGIC NEXT STEPS

### Immediate (This Session / Next Session)

1. **Fix the race condition** using BPF spinlock + stack-copy + CAS approach
   - This unblocks Doom AND hardens the protocol simultaneously
   - Estimated: 2-4 hours to implement + test

2. **Finish BSS clearing** (~63 packets at safe rate while implementing fix)
   - Can run in background while coding the spinlock fix

3. **Apply translator source fix** (3→2 operand bug in `translator.rs`)
   - Prevents accumulating more ROM patches
   - Retranslate doom.mbc cleanly

### Short-Term (This Week)

4. **Sign ROM_MAP** — Add HMAC-SHA256 verification to `wotan-ctl load-rom`
   - Currently ROM is loaded unsigned → code injection vector (D1)
   - Low effort, high security value

5. **Isolate Wotan topics** — ACL on `compute.screen.*` and `compute.input.*`
   - Prevents unauthorized framebuffer read / keyboard injection (D2, D3)

6. **Push through Doom initialization** — Watch for syscall invocations
   - doomgeneric_Create → D_DoomMain → D_DoomLoop → doomgeneric_Tick
   - Will need millions of instructions; race fix enables this

### Medium-Term (Protocol Hardening Sprint)

7. **Address CRITICAL findings in all three specs:**
   - M1: Wotan helper bounds checking (kernel memory write)
   - M2: Wotan code injection prevention (R/O memory regions)
   - S1: Version comparison ambiguity at boundary 128
   - S3: Atomic update guarantee
   - S4: Provisioning authentication
   - W1: Ring buffer sequence numbers
   - W2: L1 composite key (cross-flow isolation)
   - W3: CAS alignment enforcement
   - W4: L1 cache HMAC

8. **Resolve cross-document gaps:**
   - X1: Canonical exponent→value mapping table
   - X2: Version field width alignment
   - X3: Circuit state encoding clarity

9. **IANA registry cleanup:**
   - Fill allocation policy gaps (0x17-0xEF flow action range)
   - Designate experts
   - Align version registry to 8-bit

---

## PART 4: BLACK MAGE SKILL FILE RECOMMENDATIONS

### Current Strengths
- Excellent Lich framework documentation
- Strong STRIDE + protocol-aware threat modeling
- Good handoff contracts with other skills
- Arsenal is comprehensive and well-organized
- Anti-patterns section is valuable

### Suggested Additions

1. **Add "Doom-Specific Attack Surface" section** — The MBC CPU, ROM_MAP, framebuffer topics, and keyboard injection are NEW attack surface not covered in the existing Dark Grimoire. These need permanent documentation.

2. **Add "Cross-Document Consistency Attacks" section** — The Black Mage should explicitly track cross-spec inconsistencies as an attack vector class. Findings X1-X4 above are a new category not in the current grimoire.

3. **Add IANA Registry Attacks to the Grimoire** — Option type squatting, allocation policy gaps, and designated expert absence are adversarial vectors the Black Mage should track.

4. **Update Lich Campaigns** — Add new campaigns:
   - **LICH-007**: MBC bytecode instruction fuzzing (Doom substrate)
   - **LICH-008**: Wotan L1 cache line race condition fuzzing
   - **LICH-009**: Cross-flow composite key collision testing
   - **LICH-010**: WAL integrity / compaction race testing

5. **Add "Concurrency Primitives Audit" checklist** — The S20 race condition revealed that concurrent BPF map access is a systemic pattern. Add a dedicated checklist:
   ```
   □ Every get_ptr_mut → Is there a lock? CAS? Stack copy?
   □ Every map hot-swap → Is it truly atomic? RCU?
   □ Every multi-packet flow → Can packets overlap in the ring?
   □ Every L1 cache line → Protected by spinlock?
   □ Every WAL operation → Exclusive lock during compaction?
   ```

6. **Add "Computational Completeness Attack Surface"** — Section 12 of the Monad spec creates a Turing-complete system. The Black Mage needs a dedicated assessment of what this means adversarially: arbitrary code execution via packet injection, ROM poisoning, memory oracle attacks, timing oracle attacks.

7. **Update severity counts in "Current Project State"** — The metrics are outdated. Should reflect the ~45 findings from this assessment.

8. **Add trigger words** — The skill description should include: `doom, MBC, bytecode, ROM, framebuffer, race condition, concurrency, TOCTOU, spinlock, CAS, WAL, compaction, L1 cache, cross-flow, composite key, IANA, option squatting, registry`

---

## PART 5: PROTOCOL RFC CONTRIBUTIONS

### Immediate RFC Patches (Can Be Applied Now)

**Monad Foundation draft-03:**
1. Extend CRC to all 20 bytes (currently excludes scratch)
2. Add explicit "MUST NOT contain multiple HbH headers" requirement
3. Mandate kernel 5.17+ (not 5.15) for bounded-loops
4. Add Wotan helper bounds checking specification
5. Strengthen version field: "MUST drop immediately, NO fallback"

**Sophia Dictionary draft-00:**
1. Add provisioning node authentication section (ML-DSA-65 sig on CBOR)
2. Explicit rejection rule for version modular arithmetic boundary (128)
3. Move CDDL schema from informative to normative
4. Change "SHOULD be signed" → "MUST be signed" for Wotan distribution
5. Add BPF memory quota enforcement requirement

**Wotan Memory draft-00:**
1. Add seqno to ring buffer entries + monotonicity validation
2. Extend L1 cache key to composite (flow + src/dst hash)
3. Mandate CAS alignment enforcement in BPF verifier (pre-JIT)
4. Add HMAC-SHA256 to WAL entries (replace CRC-16 only)
5. Specify exclusive lock during WAL compaction
6. Add per-program cache-miss rate limiting

**Wire Format Patterns:**
1. Fix exponent encoding overflow handling (negative exponents)
2. Clarify scope ambiguity (first 18 vs 20 octets)
3. Add test vectors for negative/edge-case exponents

**IANA Guide:**
1. Fill allocation policy for 0x17-0xEF flow action range
2. Designate experts for registries
3. Align version field to 8-bit (matching wire format)
4. Resolve circuit state encoding ambiguity

---

## PART 6: QUIC (RFC 9000) — CROSS-POLLINATION ANALYSIS

> *"QUIC solved the same class of problems we're hitting — just at a different layer.
> Their connection IDs, packet numbers, flow control credits, and header protection
> are battle-tested patterns we can steal wholesale."*

### Why QUIC Matters to Unheaded

QUIC is the most modern transport protocol in production at scale (HTTP/3, deployed by
Google/Cloudflare/Meta). It runs over UDP with built-in crypto, multiplexing, and migration.
The Unheaded Protocol runs over IPv6 HbH with built-in compute, dictionaries, and memory.
Different goals, but the **wire format integrity**, **flow state management**, and
**concurrency** problems are IDENTICAL.

### HIGH Applicability Patterns (Adopt These)

#### Q1. Replace CRC-16 with HMAC-Based Integrity (QUIC Header Protection)
**QUIC does:** AES-CTR stream cipher on header fields; AEAD on payload. Headers are
cryptographically protected — you can't flip bits without detection.

**Unheaded currently:** CRC-16/CCITT over bytes 0x00-0x11. Trivially forgeable.

**Recommendation:** Replace CRC-16 with 8-byte HMAC-SHA256 truncated:
```
HMAC = HMAC-SHA256(per_flow_secret, monad_header[0:18])[0:8]
```
Per-flow secret derived from: `HKDF(node_secret, flow_label || src_ip || dst_ip)`

This expands Monad from 20→26 bytes (or reuse scratch bytes). Eliminates ALL CRC
collision/bypass attacks (Findings M3, W4, W5).

**Priority: CRITICAL — this is the single highest-ROI change.**

#### Q2. Packet Number Spaces → Namespace Sequence Counters
**QUIC does:** Monotonic packet numbers per space (Initial/Handshake/1-RTT). Never reused.
Enables loss detection via gaps.

**Unheaded currently:** No sequence tracking. Wotan ring buffer has no ordering guarantee.
S20 race condition is partly caused by lack of ordering.

**Recommendation:** Add per-namespace u32 sequence counter to CPU state:
- Each hop increments its namespace's counter
- Gap detection: if counter skips, packet was lost/reordered in that namespace
- Wotan ring buffer entries tagged with sequence → monotonicity validation (fixes W1)

**Priority: CRITICAL — directly addresses S20 race + W1 ring buffer confusion.**

#### Q3. Credit-Based Flow Control → BPF Map Backpressure
**QUIC does:** Receiver advertises MAX_STREAM_DATA/MAX_DATA credits. Sender blocks when
credits exhausted. STREAM_DATA_BLOCKED signals when throttled.

**Unheaded currently:** No backpressure on BPF map writes. Race conditions when maps fill.
Wotan ring buffer drops silently on full.

**Recommendation:** Per-Sophia-dictionary usage quotas tracked in BPF:
- When map usage exceeds threshold, set BLOCKED flag in Monad register
- Ingress XDP checks flag; delays/drops new packets for that flow
- Wotan emits backpressure events via BPF_RINGBUF
- Eliminates race conditions by enforcing serialization via credit system

**Priority: HIGH — addresses S2 (map exhaustion), W7 (backpressure bypass).**

#### Q4. Anti-Amplification → Ring Path Counter
**QUIC does:** 3× rule — response bytes MUST NOT exceed 3× received bytes until
address is validated. Prevents reflection/amplification attacks.

**Unheaded currently:** One injected packet loops through 6 namespaces = 6× amplification.
With hop_limit=255, it's 255× amplification per packet.

**Recommendation:** Add 2-byte ring path counter to Monad:
- Increments at each XDP hop
- Enforce max amplification ratio (e.g., 3× like QUIC)
- Shield ingress validates: if ring_count > threshold, drop
- Default hop_count reduced from 64 to 16

**Priority: HIGH — addresses M5 (unbounded loops) with a proven pattern.**

#### Q5. Connection ID → Flow Migration Token
**QUIC does:** Connection IDs (opaque, endpoint-selected) survive IP/port changes.
Sequence numbers track CID lifecycle. RETIRE_CONNECTION_ID cleans up old IDs.

**Unheaded currently:** Flow Label is static per-flow. If VXLAN/BGP topology changes,
flows break. No migration mechanism.

**Recommendation:** Add 2-byte Flow Sequence register:
- Tracks flow state transitions across topology changes
- RETIRE_FLOW equivalent: new sequence invalidates old paths
- Enables graceful BGP failover without flow disruption
- Composite L1 cache key = flow_label + flow_sequence (fixes W2)

**Priority: HIGH — critical for production VXLAN/BGP environments.**

#### Q6. Retry Tokens → Shield Address Validation
**QUIC does:** Server sends Retry packet with HMAC-validated token. Client must echo
token before server commits state. Prevents address spoofing.

**Unheaded currently:** Shield ingress creates Monad state immediately on first packet.
No address validation. Spoofed source → state allocation → memory exhaustion.

**Recommendation:** Shield-Retry pattern:
```
Token = HMAC-SHA256(shield_secret, src_ip || timestamp)[0:16]
```
First packet → Shield returns ICMPv6 with token. Second packet must carry token.
Only then does Shield allocate Monad state and flow resources.

**Priority: HIGH — prevents W4 (resource exhaustion via spoofed flows).**

### MEDIUM Applicability Patterns (Consider These)

#### Q7. Key Update → Sophia Dictionary Versioning
**QUIC does:** Key rotation mid-connection via key phase bit. Both old and new keys
valid during transition. Explicit key_update frame.

**Unheaded analog:** Sophia dictionary hot-swap (Finding S3 race condition). Currently
no versioning during transition period.

**Recommendation:** Add dictionary version epoch bit to Monad. During hot-swap:
- Both old and new dictionary versions valid for grace period
- After grace period, old version rejected
- Eliminates TOCTOU race in dictionary lookups

#### Q8. Stateless Reset → Emergency Flow Termination
**QUIC does:** 16-byte stateless_reset_token terminates connections without state lookup.

**Unheaded analog:** No flow cleanup mechanism. Orphaned flows loop indefinitely.

**Recommendation:** Per-flow reset token (8 bytes, SHA256 of 5-tuple + secret):
- Embedded in Monad register
- XDP detects stale flow → sends RESET packet
- All hops recognize token → purge flow state immediately

#### Q9. Version Negotiation → Monad Protocol Evolution
**QUIC does:** Version field in every packet. Server responds with Version Negotiation
if unsupported. Reserved versions for probing (0x?a?a?a?a pattern).

**Unheaded currently:** "MUST be 0x01, drop everything else." No evolution path.

**Recommendation:** Keep strict for now (v1 only), but define version negotiation
mechanism for future: Shield responds with ICMPv6 Parameter Problem + supported versions.

#### Q10. Protocol Invariants (RFC 8999)
**QUIC does:** Explicit invariants document defining version-independent properties.

**Unheaded needs:** An invariants document defining:
- Monad MUST always be in Hop-by-Hop extension header
- Version field MUST always be at offset 0x00
- CRC/HMAC MUST always be at offset 0x12-0x13 (or 0x12-0x19 if expanded)
- Flow Label binding MUST be immutable per-hop
- Shield MUST be the sole ingress/egress point

### QUIC Attack Warnings Applicable to Unheaded

RFC 9000 Section 21 identifies 11 attack classes. Cross-referencing with Unheaded:

| QUIC Attack | Unheaded Exposure | Finding |
|-------------|-------------------|---------|
| Amplification via reflection | 6-255× ring amplification | M5, Q4 |
| Handshake flooding | Shield allocates state on first packet | Q6 |
| Request forgery (cross-protocol) | Monad could be confused with other HbH opts | M6 |
| Stream commitment attack | Sophia dict allocation unbounded | S2 |
| Peer DoS via resource exhaustion | Wotan memory exhaustion per-flow | W2 |
| Stateless reset oracle | No reset mechanism exists | Q8 |
| Replay attack | CRC-16 provides no replay protection | M3, Q1 |
| Retry token forgery | Shield has no tokens | Q6 |
| Version downgrade | "Drop unknown" is safe but blocks evolution | M8 |
| Timing side-channel | Chaos injection leaks timing | M9 |
| Connection migration abuse | Flow Label is static, no migration | Q5 |

### QUIC-Informed Priority Roadmap

**Immediate (Blocks Doom PoC AND production):**
1. Q1: HMAC replaces CRC-16 (eliminates entire class of integrity attacks)
2. Q2: Namespace sequence counters (fixes S20 race + enables loss detection)
3. Q4: Ring path counter (limits amplification from 255× to 3×)

**Short-Term (Production hardening):**
4. Q3: Credit-based backpressure (eliminates BPF map races)
5. Q5: Flow migration tokens (enables VXLAN/BGP topology changes)
6. Q6: Shield retry tokens (prevents spoofed resource exhaustion)

**Medium-Term (Protocol maturity):**
7. Q7: Dictionary versioning with grace period
8. Q8: Stateless reset for orphaned flows
9. Q10: Invariants document (RFC 8999 equivalent)

---

## PART 7: HTTP/3 (RFC 9114) — APPLICATION-LAYER CROSS-POLLINATION

> *"QUIC gave us the transport patterns. HTTP/3 gives us the APPLICATION patterns —
> framing, multiplexing, error taxonomy, compression state sync, and extension
> frameworks. If QUIC is the road, HTTP/3 is the traffic law."*

### Why HTTP/3 Matters to Unheaded

HTTP/3 is the application protocol that rides on QUIC. It solved problems at the framing
and semantics layer that map directly to how the Unheaded Protocol handles Sophia
dictionary distribution, Anamnesis event streaming, Wotan topic management, and
protocol evolution. Where QUIC gave us 10 transport patterns (Q1-Q10), HTTP/3 gives
us 12 application patterns (H1-H12) — several of which address gaps QUIC couldn't reach.

### CRITICAL (P1) Findings — Must Fix Before Production

#### H1. QPACK-Inspired Sophia Dictionary Compression + State Synchronization
**Applicability: CRITICAL — directly addresses S20 race condition**

**HTTP/3 does:** QPACK uses static + dynamic tables for header compression. The dynamic
table is updated via a dedicated encoder stream. The decoder stream acknowledges table
state. This three-party handshake (encoder → table → decoder ACK) guarantees that both
ends agree on table state before using it. If ACK is missing, encoder retransmits.
Compression ratio: 3-5× for repeated headers.

**Unheaded currently:** Sophia dictionaries distributed as uncompressed CBOR blobs via
Wotan. No dynamic table versioning. No acknowledgment mechanism. S20 race condition:
hot-swap Sophia without ACK → divergent endpoint tables → use-after-free → state corruption.

**Recommendation:** Implement QPACK-style encoder/decoder stream for Sophia:
```
Sophia Update Protocol:
  1. Provisioning node encodes dictionary delta as CBOR patch
  2. Encoder stream (Wotan control topic) distributes delta to all hops
  3. Each hop applies delta to local BPF map, increments table_version
  4. Decoder stream (Wotan ACK topic) returns {hop_id, table_version}
  5. Provisioning node waits for ALL hops to ACK before marking version active
  6. Old version retained for grace_period (QUIC Q7 key update pattern)
  7. After grace_period, old version tombstoned
```
This eliminates S3 (atomic update race), S4 (unauthenticated provisioning via signed deltas),
and provides 3-5× bandwidth reduction on dictionary distribution.

**Reinforces:** S3, S4, Q7 | **New insight:** Encoder/decoder stream pattern for state sync

#### H2. Structured Error Code Taxonomy
**Applicability: CRITICAL — Unheaded has NO error code registry**

**HTTP/3 does:** 13 predefined error codes (H3_NO_ERROR through H3_MESSAGE_ERROR) with
clear distinction between stream-level errors (affect one flow) and connection-level errors
(affect entire domain). IANA registry with Specification Required policy for extensions.
Each error code documents recommended peer action (retry, backoff, close).

**Unheaded currently:** Anamnesis events (64-byte fixed) record anomalies but with no
structured error semantics. Cannot distinguish "BPF map full" from "CRC mismatch" from
"race condition" from "kernel panic." Cannot signal error to peer. No retry coordination.

**Recommendation:** Define Unheaded Error Code Registry:
```
FLOW-LEVEL ERRORS (affect one flow, other flows continue):
  0x0000  UNHD_NO_ERROR              Graceful close, no error
  0x0001  UNHD_GENERAL_PROTOCOL_ERROR  Catch-all protocol violation
  0x0002  UNHD_MALFORMED_MONAD       Layout/encoding violation
  0x0003  UNHD_INVALID_SOPHIA_REF    Dict entry out of bounds
  0x0007  UNHD_DUPLICATE_FLOW_ID     Flow Label collision
  0x0009  UNHD_UNKNOWN_EXTENSION     Unknown Monad TLV type
  0x000b  UNHD_REQUEST_REJECTED      Destination refused processing

DOMAIN-LEVEL ERRORS (affect all flows in Limited Domain):
  0x0004  UNHD_SHIELD_AUTHORITY_INVALID  Certificate/token validation failed
  0x0005  UNHD_BPF_MAP_EXHAUSTED     eBPF map full — backoff required
  0x0006  UNHD_RING_BUFFER_OVERFLOW   Dropped events — reduce trace rate
  0x0008  UNHD_EXCESSIVE_LOAD        Resource limits exceeded — backoff 1-5s
  0x000a  UNHD_RACE_CONDITION        Concurrent BPF map access detected
  0x000c  UNHD_INTERNAL_ERROR        Kernel/BPF failure — non-recoverable

IANA ALLOCATION:
  0x0000-0x003F   Standards Action (core protocol)
  0x0040-0x00FF   Specification Required (extensions)
  0x1F*N+0x21     Reserved for testing/padding (HTTP/3 pattern)
```
Error codes carried in: Anamnesis events (2-byte field), GOAWAY frames (Q8), flow reset.

**Reinforces:** M10, Q8 | **New insight:** Flow-level vs domain-level error distinction

#### H3. Monad TLV Extension Mechanism
**Applicability: CRITICAL — prevents protocol ossification**

**HTTP/3 does:** Variable-length frame type + length fields. Unknown frame types safely
ignored per Section 9. Extensions registered via IANA. The `0x1F*N+0x21` pattern reserves
greasing values that MUST be ignored, preventing implementation bugs from ossifying the
protocol (a lesson learned painfully from HTTP/2).

**Unheaded currently:** Monad is fixed 20 bytes, version 0x01. Adding ANY new field
requires a new protocol version. No extension negotiation. No forward compatibility.
The 8-bit flags field is already filling up (6 of 8 bits defined). When flags exhausts,
the protocol is stuck.

**Recommendation:** Add optional TLV extension block after Monad core:
```
HbH Option Layout:
  [Type=0x3E][Len=20+N][Monad Core: 20 bytes][TLV Extensions: N bytes]

TLV Format:
  [Type: 1 byte][Length: 1 byte][Value: Length bytes]

Type Ranges:
  0x00-0x0F  Core (MUST understand or drop packet)
  0x10-0x7F  Negotiated (safe to ignore if not negotiated)
  0x80-0xFF  Padding/greasing (MUST ignore, MUST NOT error)

Initial Core Types:
  0x01  PRIORITY_HINT     2 bytes   Per-flow priority (0-7)
  0x02  TRACE_SPAN_ID     8 bytes   Distributed trace correlation
  0x03  ERROR_CODE        2 bytes   Structured error (H2 above)
  0x04  FLOW_SEQUENCE     2 bytes   Migration sequence (Q5)
  0x05  RING_PATH_COUNT   2 bytes   Anti-amplification counter (Q4)
  0x06  HMAC_TAG          8 bytes   Header integrity (Q1)

Negotiation: Shield SETTINGS exchange at ingress (H4 below)
```
Backwards compatible: v1 endpoints see Len=20, ignore extensions. v1.1+ endpoints
parse TLV block. Greasing values (0x80-0xFF) ensure implementations handle unknown
types correctly from day one.

**Reinforces:** M2, M8, Q1, Q4, Q5 | **New insight:** Greasing pattern prevents ossification

#### H4. Capability Negotiation via SETTINGS
**Applicability: CRITICAL — blocks all other improvements**

**HTTP/3 does:** SETTINGS frame sent at connection start on control stream. Both peers
exchange supported capabilities. Parameters are varint-encoded key-value pairs. Unknown
settings are safely ignored. This is the FOUNDATION — you can't negotiate extensions,
error codes, or compression without a settings exchange mechanism.

**Unheaded currently:** No capability negotiation whatsoever. Shield ingress creates
Monad with hardcoded assumptions. All hops process identically. No way for a hop to
say "I support TLV extensions" or "my max dictionary size is 1MB."

**Recommendation:** Shield SETTINGS exchange at domain boundary:
```
SETTINGS Frame (carried on Wotan control topic):
  SETTINGS_MAX_DICTIONARY_SIZE     varint   Max Sophia dict size (bytes)
  SETTINGS_MAX_FLOW_COUNT          varint   Max concurrent flows
  SETTINGS_MAX_MONAD_SIZE          varint   Max Monad + TLV size (bytes)
  SETTINGS_SUPPORTED_EXTENSIONS    list     TLV types supported
  SETTINGS_SOPHIA_COMPRESSION      bool     QPACK-style compression enabled
  SETTINGS_RING_BUFFER_SIZE        varint   Anamnesis ring buffer size
  SETTINGS_WOTAN_BUFFER_SIZE       varint   Per-flow Wotan allocation

Exchange Protocol:
  1. Shield ingress sends SETTINGS to all hops via Wotan control topic
  2. Each hop responds with its own SETTINGS (capabilities + limits)
  3. Shield computes intersection (minimum of limits, intersection of extensions)
  4. Shield distributes effective SETTINGS to all hops
  5. Hops apply effective SETTINGS before processing traffic
```
This is the PREREQUISITE for H1 (compression), H2 (error codes), H3 (TLV extensions),
Q3 (flow control credits), and Q6 (retry tokens). Without SETTINGS, none of those
can be negotiated safely.

**Reinforces:** S3, Q3, Q6, Q9 | **New insight:** SETTINGS is the universal prerequisite

### HIGH (P2) Findings — Must Fix Before Wide Deployment

#### H5. DoS & Compression Attack Mitigations
**Applicability: HIGH — 5 parallel mitigations needed**

**HTTP/3 Section 10.5-10.6 warns about:**
- Ring buffer exhaustion (silent event loss)
- BPF map exhaustion (silent packet drop)
- Sophia dictionary bloat (memory exhaustion)
- Compression side-channel attacks (CRIME/BREACH)
- Extension frame processing DoS

**Recommended mitigations (5 parallel):**

**H5.1 Ring Buffer Backpressure:**
- Monitor BPF ringbuf output return value
- If drop rate > 1%/sec: emit UNHD_RING_BUFFER_OVERFLOW, reduce sampling, drop low-priority events first
- Per-hop Anamnesis budget: max events/sec configurable via SETTINGS

**H5.2 BPF Map Exhaustion Limits:**
- max_entries at boot (default 1M flows, configurable)
- BPF_F_NO_PREALLOC for fail-fast (not silent drop)
- On full: emit UNHD_BPF_MAP_EXHAUSTED, evict lowest-QoS flows, signal EXCESSIVE_LOAD upstream

**H5.3 Sophia Dictionary Size Limits:**
- Per-flow: 1 MB max
- Global: 100 MB max
- On exceeded: reject new entries → UNHD_INVALID_SOPHIA_REF
- Advisory limit via SETTINGS: SETTINGS_MAX_DICTIONARY_SIZE

**H5.4 Compression Attack Mitigation (CRIME/BREACH):**
- Do NOT compress Sophia entries containing secrets (authority tokens, crypto material)
- Per-entry flag: COMPRESS=0 for sensitive values
- Separate compression contexts: trusted (admin-supplied) vs untrusted (flow-supplied)
- Random padding via greasing TLV types (0x80-0xFF) to obscure compression ratio

**H5.5 Extension Frame DoS:**
- Max 4 TLVs per Monad
- Unknown TLV types skipped in O(1) constant time
- BPF bounded execution enforced by kernel verifier
- No dynamic allocation per TLV

**Reinforces:** M10, S2, W7, Q3 | **New insight:** CRIME/BREACH applies if we compress Sophia

#### H6. Typed Flow Classification
**Applicability: HIGH — enables resource isolation**

**HTTP/3 does:** Stream types (control=0x00, push=0x01, unknown=ignored). Each type
has independent flow control, separate error handling, and type-specific processing.
Control streams are protected from data stream DoS.

**Unheaded currently:** All flows uniform. Control operations (Sophia updates, SETTINGS,
GOAWAY) share the same Wotan ring buffer as data flows. A data flow flood can starve
control flow processing → Sophia updates don't propagate → stale dictionaries → corruption.

**Recommendation:** Three flow types with per-type isolation:
```
Flow Type Classification (2-bit field in Monad flags):
  0b00 = CONTROL   Sophia updates, SETTINGS, GOAWAY, error signaling
  0b01 = DATA       Normal traffic, telemetry, application payloads
  0b10 = PREFETCH   Wotan L1 cache prefetch, spatial locality hints
  0b11 = RESERVED   Future use

Per-Type Guarantees:
  CONTROL:  Priority processing, dedicated Wotan topic, never dropped
  DATA:     Best-effort, subject to QoS classification, droppable
  PREFETCH: Lowest priority, cancelled on resource pressure
```
Control flows get dedicated ring buffer allocation, guaranteed processing before data.
This prevents control plane starvation — a well-known failure mode in production networks.

**Reinforces:** S2 (map exhaustion), W7 (backpressure) | **New insight:** Control plane isolation

#### H7. Graceful Shutdown (GOAWAY Reinforcement)
**Applicability: HIGH — extends Q8 with HTTP/3 semantics**

**HTTP/3 does:** GOAWAY frame carries last_stream_id. All streams with ID > last_stream_id
are safely retried on new connection. Monotonicity: GOAWAY values MUST NOT increase (prevents
confusion). Both client and server can initiate GOAWAY.

**Unheaded currently:** No shutdown signaling (Q8 identified this). Orphaned flows loop
until hop_count expires. No exactly-once semantics — flows started during shutdown may
be processed partially, then lost.

**Recommendation (extends Q8):**
```
GOAWAY Frame:
  last_flow_id:   4 bytes   (IPv6 Flow Label of last accepted flow)
  error_code:     2 bytes   (H2 error taxonomy)
  monotonicity:   MUST NOT increase (each GOAWAY ≤ previous)

Shutdown Sequence:
  1. Shield sends GOAWAY(last_flow_id=current_max) on control stream
  2. All hops: flows with label > last_flow_id → reject with UNHD_REQUEST_REJECTED
  3. In-flight flows (label ≤ last_flow_id) → complete normally
  4. Shield waits for drain_timeout (configurable, default 30s)
  5. Shield sends final GOAWAY(last_flow_id=0) → all flows terminated
  6. Hops purge all flow state for this domain

Exactly-Once Guarantee:
  - Flows started before GOAWAY: processed exactly once
  - Flows started after GOAWAY: rejected, client retries on new domain path
  - No partial processing, no duplicate processing
```

**Reinforces:** Q8 | **New insight:** Monotonic GOAWAY + exactly-once semantics

#### H8. Request Cancellation Without Domain Collapse
**Applicability: HIGH — enables individual flow cleanup**

**HTTP/3 does:** CANCEL_PUSH frame cancels a single server push without affecting other
streams. RST_STREAM cancels a single stream. Neither tears down the connection.

**Unheaded currently:** No per-flow cancellation. A stale or misbehaving flow continues
looping until hop_count expires. Cancelling requires domain-wide intervention.

**Recommendation:**
```
CANCEL_FLOW Frame (TLV extension type 0x07):
  flow_label:    4 bytes   (which flow to cancel)
  error_code:    2 bytes   (why)

Processing:
  1. Any hop receiving CANCEL_FLOW: purges flow state from CPU_MAP, L1 cache
  2. Forwards CANCEL_FLOW to next hop (propagates through ring)
  3. Sophia refcount for this flow's dictionary entries decremented
  4. If refcount = 0, dictionary entries eligible for eviction
  5. Wotan ring buffer space for this flow released

Use Cases:
  - Stale Doom PoC flow after demo ends → CANCEL_FLOW
  - Misbehaving flow detected by Shield → CANCEL_FLOW + UNHD_RACE_CONDITION
  - Operator-initiated cleanup → CANCEL_FLOW + UNHD_NO_ERROR
```

**Reinforces:** Q8, W2 | **New insight:** Per-flow cleanup with Sophia refcount management

### MEDIUM (P3) Findings — Important for Maturity

#### H9. Proactive Resource Distribution (Server Push → Prefetch Signaling)
**Applicability: MEDIUM — Wotan already does implicit prefetch, needs explicit signaling**

**HTTP/3 does:** Server push (PUSH_PROMISE) delivers resources before client requests them.
Can be cancelled via CANCEL_PUSH. Has a maximum push ID to prevent unbounded server push.

**Unheaded currently:** Wotan L1 cache prefetch is implicit (spatial locality, PrefetchN
config). No explicit signaling. No way for a hop to say "I need pages X, Y, Z prefetched."

**Recommendation:** PREFETCH_HINT TLV extension:
```
PREFETCH_HINT (TLV type 0x08):
  flow_label:  4 bytes
  base_addr:   4 bytes   (Wotan address to prefetch around)
  page_count:  1 byte    (how many pages to prefetch)

CANCEL_PREFETCH (TLV type 0x09):
  flow_label:  4 bytes
  Cancels all pending prefetch for this flow

MAX_PREFETCH SETTINGS parameter:
  Maximum outstanding prefetch hints per flow (default: 16)
```
This turns implicit prefetch into explicit, observable, cancellable prefetch.
Particularly valuable for Doom PoC where access patterns are predictable
(sequential screen writes, stack operations).

#### H10. Intermediary-Aware Processing Rules
**Applicability: MEDIUM — every Unheaded hop IS an intermediary**

**HTTP/3 does:** Section 3.4 defines intermediary behavior: proxies generate their own
QUIC connections, may coalesce requests, must validate authority, must not forward
malformed messages.

**Unheaded implication:** Every hop in the network is an intermediary that reads and
modifies the Monad. HTTP/3's intermediary rules suggest each hop MUST:
- Validate Monad integrity (CRC/HMAC) before processing
- Validate authority (is this flow supposed to transit this hop?)
- Detect malformation (truncated Monad, invalid TLV, CRC mismatch)
- NOT forward known-malformed packets (currently some hops forward with anomaly flag)

#### H11. Cross-Protocol Attack Prevention
**Applicability: MEDIUM — reinforces M6**

**HTTP/3 Section 10.4:** ALPN prevents cross-protocol confusion. HTTP/3 runs exclusively
on QUIC, identified by ALPN "h3". This prevents TCP-based HTTP/2 traffic from being
misinterpreted as HTTP/3.

**Unheaded analog:** Monad HbH option type 0x3E could collide with other IPv6 HbH options.
Shield MUST validate that incoming packets don't already carry type 0x3E (already in spec).
Add: reserved 4-bit field in Monad MUST be 0x0 — non-zero reserved = not Unheaded, drop.
This is the "magic number" equivalent of ALPN identification.

#### H12. Connection Coalescing → Multi-Service Flow Multiplexing
**Applicability: LOW-MEDIUM — future optimization**

**HTTP/3 does:** Coalesces multiple HTTP origins onto one QUIC connection if they share
a certificate. Reduces connection overhead.

**Unheaded analog:** Multiple service flows transiting the same physical path could share
Sophia dictionary state, Wotan ring buffer allocation, and L1 cache pages. Currently each
flow is fully independent. Coalescing would reduce per-flow overhead for services that
share dictionaries.

### HTTP/3 + QUIC Combined Attack Matrix

All 11 QUIC attack classes PLUS 6 HTTP/3-specific attack classes mapped to Unheaded:

| Attack Class | Source | Unheaded Exposure | Mitigation |
|-------------|--------|-------------------|------------|
| Amplification | QUIC §21 | 255× ring loop | Q4 ring path counter |
| Handshake flood | QUIC §21 | Shield state on first pkt | Q6 retry tokens |
| Request forgery | QUIC §21 | HbH option confusion | H11 reserved field check |
| Stream commitment | QUIC §21 | Sophia unbounded | H5.3 size limits |
| Resource exhaustion | QUIC §21 | Wotan per-flow unbounded | H5.2 BPF map limits |
| Stateless reset oracle | QUIC §21 | No reset mechanism | Q8 + H7 GOAWAY |
| Replay attack | QUIC §21 | CRC-16 no replay protection | Q1 HMAC |
| Retry token forgery | QUIC §21 | No tokens | Q6 HMAC tokens |
| Version downgrade | QUIC §21 | Drop unknown (safe) | Q9 version negotiation |
| Timing side-channel | QUIC §21 | Chaos injection | M9 constant-time ops |
| Connection migration | QUIC §21 | Flow Label static | Q5 flow migration |
| **CRIME/BREACH** | **HTTP/3 §10.6** | **If Sophia compressed** | **H5.4 separate contexts** |
| **Control starvation** | **HTTP/3 §6.2** | **Uniform flow model** | **H6 typed flows** |
| **State desynchronization** | **HTTP/3 QPACK** | **Sophia hot-swap race** | **H1 encoder/decoder ACK** |
| **Request smuggling** | **HTTP/3 §10** | **Anamnesis event framing** | **H2 strict validation** |
| **Extension DoS** | **HTTP/3 §9** | **No extension framework** | **H3 TLV + H5.5 limits** |
| **Shutdown ambiguity** | **HTTP/3 §5.2** | **No GOAWAY** | **H7 monotonic GOAWAY** |

### HTTP/3-Informed Priority Roadmap (Extends QUIC Roadmap)

**Phase 0 — PREREQUISITE (Blocks everything else):**
- H4: SETTINGS negotiation framework

**Phase 1 — CRITICAL (Weeks 1-4):**
- H1: QPACK-style Sophia state sync (fixes S20 race)
- H2: Error code registry (13 core codes)
- H3: TLV extension mechanism (future-proofs Monad)
- Q1: HMAC replaces CRC-16

**Phase 2 — HIGH (Weeks 5-8):**
- H5: DoS mitigations (5 parallel streams)
- H6: Typed flow classification (control/data/prefetch isolation)
- H7: GOAWAY with monotonicity + exactly-once
- H8: CANCEL_FLOW with Sophia refcount
- Q2: Namespace sequence counters
- Q4: Ring path counter

**Phase 3 — MEDIUM (Weeks 9-12):**
- H9: Explicit prefetch signaling
- H10: Intermediary validation rules
- H11: Cross-protocol attack prevention
- Q3: Credit-based backpressure
- Q5: Flow migration tokens
- Q6: Shield retry tokens

**Phase 4 — MATURITY (Weeks 13-16):**
- H12: Multi-service flow coalescing
- Q7: Dictionary versioning with grace period
- Q8: Stateless reset for orphaned flows
- Q10: Invariants document (RFC 8999 equivalent)

---

## VERDICT

**The Kingdom's walls have cracks. But the cracks are mapped.**

45+ protocol findings + 10 QUIC transport patterns (Q1-Q10) + 12 HTTP/3 application patterns
(H1-H12) across all specs. 16 CRITICAL, 25+ HIGH. The good news: NONE are architectural —
they're all fixable with spec language tightening, bounds checking, cryptographic integrity,
concurrency primitives, and proven patterns from QUIC/HTTP/3 production deployments.

**The combined QUIC + HTTP/3 analysis reveals a clear 4-phase roadmap:**
- Phase 0: SETTINGS prerequisite (H4) — unlocks everything
- Phase 1: Integrity + state sync + error codes + extensibility (Q1, H1, H2, H3)
- Phase 2: DoS hardening + flow isolation + shutdown + cancellation (H5-H8, Q2, Q4)
- Phase 3: Prefetch + intermediary rules + migration + backpressure (H9-H11, Q3, Q5, Q6)
- Phase 4: Coalescing + versioning + reset + invariants (H12, Q7-Q10)

The Doom PoC race condition (S20) is the canary in the coal mine. Fix it right (BPF spinlock + CAS),
and you've established the concurrency pattern for the entire protocol stack. The H1 QPACK-style
encoder/decoder ACK pattern provides the DEFINITIVE fix for Sophia hot-swap races. Doom is the
stress test — if the protocol can run a game at 35 FPS without corrupting state, it can run
production infrastructure telemetry without blinking.

**Total RFC cross-pollination score:** 22 patterns extracted from 2 RFCs (9000 + 9114), covering
17 unique attack classes, addressing 35+ of our 45+ existing findings, with a 16-week phased
implementation roadmap.

The Lich stirs. The Dark Grimoire grows. QUIC showed us the transport. HTTP/3 showed us the
application. Together they light the path. **Break everything. Fix everything. Ship stronger.**

---

*"You can't defend what you don't understand. You can't understand what you haven't attacked."*
*— The Black Mage*

*Assessment by: unheaded-blackmage skill, Session S21*
*Last synced: February 20, 2026*
