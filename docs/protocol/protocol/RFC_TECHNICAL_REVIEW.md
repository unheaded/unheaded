# Technical Review: draft-bellis-unheaded-protocol-foundation-02

**Date:** 2026-02-19
**Reviewers:** Architect Skill (4-pillar fusion) + Developer Skill (paranoid security)
**Verdict:** PROMISING CORE — SIGNIFICANT REWORK NEEDED FOR RFC-GRADE

---

## Executive Summary

The draft has a **solid conceptual foundation** — the mapped data bus model,
Sophia dictionary compression, and the memory hierarchy are genuinely novel
ideas.  But compared to approved RFCs in the 9000+ series (9486/IOAM,
9673/HbH Processing, 9669/BPF ISA), the document reads like an **architecture
whitepaper**, not a **protocol specification**.

Real RFCs are **boring on purpose**.  They specify exact byte layouts,
exhaustive state machines, precise error handling, and unambiguous
interoperability requirements.  Your draft has marketing energy where it
needs mechanical precision.

Below: every issue, categorized, with specific fix guidance.

---

## CATEGORY 1: STRUCTURAL PROBLEMS

### 1.1 — Missing RFC 2119 Precision on Critical Behaviors

**Problem:** The draft uses MUST/SHOULD/MAY but inconsistently.  Many
critical behaviors use plain English where normative language is required.

**Examples of missing normative language:**

| Location | Current Text | Should Be |
|----------|-------------|-----------|
| §3.3 (Monad) | "All fields are in network byte order" | "All fields MUST be encoded in network byte order (big-endian)" |
| §3.3 | "The semantics of each register are program-defined" | "The semantics of each register are program-defined; receivers MUST NOT assume fixed semantics" |
| §7.1 | "Shim programs are verified by the kernel BPF verifier" | This is an implementation detail, not a protocol requirement. Rewrite as: "Implementations MUST ensure per-hop programs cannot access memory outside their designated regions" |
| §8 (Per-hop) | "All per-hop processing MUST be stateless" | Good — but then §8.6 says "The Shim reads/writes Wotan memory" which IS state. Clarify: "stateless with respect to the Monad; external state is accessed only through Wotan" |
| §9 (Anamnesis) | "Non-blocking ring buffer write" | "Implementations SHOULD use non-blocking writes; if the ring buffer is full, the event MUST be dropped silently and a dropped-event counter MUST be incremented" |

**Fix:** Audit every sentence. If it describes behavior a conforming
implementation must exhibit, it needs MUST/SHOULD/MAY.  If it describes
a design rationale, it belongs in an appendix or informative section.

### 1.2 — Normative vs Informative Sections Are Blurred

**Problem:** The draft mixes protocol specification with implementation
guidance, architecture philosophy, and marketing language.

**Specific violations:**

- §2 "The Packet Is the Memory" — This is a design essay, not a spec section.
  Real RFCs put motivation in Section 1 (Introduction) and get to wire format
  by Section 3.

- §2.2 "Design Principles" — Belongs in Introduction or an appendix.
  "Minimalism in the packet" is philosophy, not specification.

- §11 "Computational Completeness" — A Turing machine proof sketch is
  interesting but belongs in an appendix (Appendix B), not in the middle
  section.  RFC 9669 (BPF ISA) puts its instruction set semantics in the
  middle and proofs/discussion in appendices.

- Architecture Layers description uses marketing language: "Speed: light",
  "The Rosetta Stone", "The nervous system".

**Fix:** Restructure to match standard RFC flow:

```
1. Introduction (problem, scope, applicability, terminology)
2. Protocol Overview (brief, factual)
3. Packet Format (wire diagrams, every field, every bit)
4. Processing Rules (state machines, MUST/SHOULD/MAY)
5. Subsystem Specifications (Sophia, Wotan, Anamnesis, Shield)
6. Security Considerations
7. IANA Considerations
Appendix A: Computational Completeness Proof
Appendix B: Heritage and Prior Art
Appendix C: Example Deployments
```

### 1.3 — Option Type Third Bit Is WRONG

**Problem:** Section 3.2 states:

> "the third bit MUST be 0 (option data does not change en-route)"

But the Monad IS modified at every hop.  The whole point of the protocol
is per-hop computation on the register file.  The `chg` bit MUST be 1.

Look at RFC 9486 — IOAM registers TWO option types:
- `0x11` (chg=0) for IOAM data that doesn't change
- `0x31` (chg=1) for IOAM data that IS modified by transit nodes

**Your option type needs chg=1.**  The format should be `001xxxxx` (act=00,
chg=1), not `000xxxxx`.

**Fix:** Change to:

```
The high-order two bits MUST be 00 (skip if unrecognized).
The third bit (chg) MUST be 1 (option data may change
en-route), as the Monad is modified by per-hop processing.
The resulting format is 001xxxxx.
```

And update the suggested value from `0x42` to something in the
`0x20-0x3F` range (chg=1 space).  Actually, per RFC 8200 §4.2,
the format is:

```
  +-+-+-+-+-+-+-+-+
  |act|c|  rest   |
  +-+-+-+-+-+-+-+-+
  act = 00 (skip if unrecognized)
  c   = 1  (may change en-route)
  rest = to be assigned
```

Suggested value: `0x31` is taken (IOAM).  Request `0x3E` or similar.
This is a **critical correctness bug** — an IESG reviewer will reject
immediately on this alone.

---

## CATEGORY 2: WIRE FORMAT DEFICIENCIES

### 2.1 — Monad Register Semantics Are Underspecified

**Problem:** The draft defines R0-R4 as "Accumulator", "Argument", "Result",
"Counter", "Caller ID" — then immediately says "semantics are program-defined."

This is contradictory.  Either the registers have fixed semantics (like IOAM's
defined data fields) or they're general-purpose (like CPU registers).

Real RFCs pick one:
- RFC 9197 (IOAM Data Fields): Every bit of every field has a fixed meaning
- RFC 9669 (BPF ISA): Registers are general-purpose, numbered r0-r10

**Fix:** Choose one approach:

Option A (RECOMMENDED): General-purpose registers, no fixed names:

```
The Monad contains five 32-bit general-purpose registers,
R0 through R4.  The protocol assigns no fixed semantics
to any register.  Per-hop programs (Shims) define register
usage through Sophia dictionary entries keyed by program
identifier.
```

Option B: Fixed semantics with a defined schema:

```
R0: Service Identifier (src_service_id:8 || dst_service_id:8 ||
    flow_action:8 || qos_class:8)
R1: Flow Control (hop_count:8 || circuit_state:8 ||
    deployment_ring:8 || mesh_flags:8)
R2: Trace Correlation (trace_id_hi:16 || trace_id_lo:16)
R3: Timing (latency_hint_us:16 || reserved:16)
R4: Integrity (scratch:16 || checksum:16)
```

The PROTOCOL_TECHNICAL_SUMMARY.md already has a concrete 20-byte layout
(§2) that is far more specific than the RFC draft.  **Use it.**

### 2.2 — Checksum Changed Between Drafts Without Justification

**Problem:**
- Draft -01: CRC-32 Castagnoli (0x1EDC6F41), computed only at egress
- Draft -02: CRC-16/CCITT (0x1021), verified at every hop
- PROTOCOL_TECHNICAL_SUMMARY: CRC-16 over first 18 bytes, verified at each hop

These are three different integrity models.  The draft doesn't explain
why it changed or the tradeoffs.

**Fix:** Pick one and justify it precisely:

```
The Monad integrity field is a 16-bit CRC computed using
the CRC-16/CCITT polynomial (x^16 + x^12 + x^5 + 1,
reflected polynomial 0x8408, init 0xFFFF, no final XOR).

The checksum covers octets [0..17] of the option payload
(the 20-byte Monad minus the 2-byte checksum field itself).

Each hop MUST verify the checksum before processing.
If verification fails, the implementation MUST:
  (a) increment a per-interface error counter,
  (b) emit an EVENT_ANOMALY to Anamnesis if tracing is
      enabled, and
  (c) either drop the packet (RECOMMENDED) or forward it
      with the anomaly flag set in the flags bitfield.

After modifying any Monad field, the hop MUST recompute
the checksum before forwarding.
```

### 2.3 — Flags Bitfield Changed Completely Between Drafts

**Problem:**
- Draft -01 flags: A=chaos, B=memory-trace, C=replay, D-H=reserved
- Draft -02 flags: C=chaos, Y=canary, T=trace, E=encrypted, S=sampled,
  M=mirror, K1:K0=kingdom-mode

No transition guidance.  Both drafts are dated 2026-02-18.  An
implementer reading -01 then -02 has no idea what happened.

**Fix:** The -02 flags are better.  Commit to them, but add a
"Changes from -01" appendix (standard practice in RFC drafts).

### 2.4 — Exponent Encoding Lacks a Concrete Binary Format

**Problem:** The draft describes exponent encoding conceptually:

```
value = base^exponent * multiplier
```

But never defines the actual binary representation.  Compare to
RFC 9197 §4.4 which specifies every bit of every field.

Questions an implementer would ask:
- How many bytes is an exponent-encoded field?
- Is the exponent signed or unsigned?
- Where is the base stored (in the field? in Sophia? implicit)?
- What's the byte order?
- What happens on decode overflow?

**Fix:** Define a concrete encoding:

```
An exponent-encoded field is a single octet interpreted as
a signed 8-bit integer (two's complement, range -128..+127).

The decoded value is computed as:

  decoded = base ^ exponent

Where 'base' is defined by the Sophia dictionary entry for
this field position.  If no Sophia entry exists, the default
base is 2.

The multiplier (unit scaling) is also defined per-field in
the Sophia dictionary.  If no entry exists, the multiplier
is 1.

Encoders MUST NOT produce exponent values that would cause
the decoded value to exceed 2^64 - 1.  Decoders that
encounter such values MUST treat them as errors.
```

### 2.5 — Sophia Dictionary Wire Format Is Missing

**Problem:** Sophia is central to the protocol, but the draft never
specifies how dictionaries are represented on the wire or exchanged
between nodes.

The draft says "BPF maps in kernel space, structured tables in
userspace" — that's an implementation, not a protocol.

**Fix:** Define:

1. A serialization format for Sophia dictionaries (protobuf? CBOR?
   custom TLV?)
2. A distribution mechanism (how does node A get node B's dictionary?)
3. Version negotiation (what if two hops have different dictionary
   versions?)
4. A minimum required dictionary that all implementations MUST support

Without this, two independent implementations cannot interoperate.
That's the death sentence for any RFC.

### 2.6 — Wotan Has No Wire Protocol

**Problem:** Wotan is described as "the memory and I/O bus" with ring
buffers, WAL, topics.  But there's no specification of:

- How a Shim accesses Wotan memory (BPF helper? map lookup? special
  instruction?)
- The message format between Wotan and Shims
- How Wotan topics are addressed
- Flow Label → ring buffer mapping rules
- Error handling when Wotan is unavailable

**Fix:** Either:
(a) Define Wotan as a separate specification (referenced normatively),
    similar to how SRv6 (RFC 8754) references segment routing
(b) Include a minimal Wotan wire protocol in this document

At minimum, specify the BPF helper interface:

```
The following BPF helper functions MUST be available to
Shim programs:

  long bpf_wotan_read(u32 flow_label, u32 addr, void *buf,
                       u32 len);
  Returns: number of bytes read, or negative error code.

  long bpf_wotan_write(u32 flow_label, u32 addr,
                        const void *buf, u32 len);
  Returns: number of bytes written, or negative error code.

Error codes:
  -ENOENT: flow_label not found
  -EFAULT: addr out of bounds
  -ENOMEM: ring buffer full
```

---

## CATEGORY 3: SECURITY GAPS

### 3.1 — CRC-16 Is Not Integrity Protection

**Problem:** The draft conflates error detection (CRC) with integrity
protection (HMAC/signatures).  Section 12.3 says "The CRC-16 checksum
detects accidental corruption" then offers HMAC-SHA256 as "optional."

In a Limited Domain where all hops are operator-controlled, a compromised
hop could modify the Monad AND recompute the CRC.  The CRC provides
zero protection against malicious modification.

Real RFCs are explicit about this.  RFC 9486 §7 says IOAM "does not
provide built-in data integrity protection" and requires implementations
to use IPsec or other mechanisms.

**Fix:** Be explicit:

```
The CRC-16 checksum provides error detection against
accidental bit corruption only.  It does not provide
integrity protection against malicious modification.

Deployments requiring integrity protection against
compromised intermediate nodes MUST use one of:

  (a) IPsec ESP (RFC 4303) to protect the entire packet
      including extension headers.

  (b) The optional HMAC field (Section X.Y) appended to
      the Monad, using a pre-shared key distributed via
      Sophia.

  (c) ML-DSA-65 (FIPS 204) signatures at Shield
      boundaries for post-quantum integrity.

The choice of integrity mechanism is a deployment decision
and is outside the scope of this specification.
```

### 3.2 — Kingdom Mode Address Rewriting Is a Firewall Evasion Vector

**Problem:** Kingdom Mode overwrites IPv6 address bits with register
data.  The draft says Shield strips this at egress.  But what about:

- Packets that bypass Shield (misconfigured routes, VPN breakout)
- Packets captured by monitoring tools that see mangled addresses
- IPv6 extension header firewalls that inspect addresses
- NDP/ND that depends on real addresses
- Flow-based load balancers keyed on addresses

The draft's security section says "Shield MUST strip" but doesn't
address what happens when Shield fails.

**Fix:** Add a "Failure Modes" subsection:

```
If a Kingdom Mode packet escapes the Limited Domain without
Shield processing:

  (a) The packet will carry invalid IPv6 addresses that do
      not correspond to any real host.

  (b) External routers will either drop the packet (no route
      to invalid address) or misroute it.

  (c) The Extended Register data encoded in the address bits
      will be exposed to external observers.

Deployments using Kingdom Mode MUST implement defense in
depth at the domain boundary:

  1. Shield at every egress point (primary).
  2. ACLs on border routers blocking the Kingdom ULA prefix
     (secondary).
  3. BPF programs on border interfaces that detect and drop
     packets with the K flag set (tertiary).

The risk of Kingdom Mode address leakage MUST be evaluated
as part of the deployment's security assessment.
```

### 3.3 — 32-bit Truncated Fingerprint Is Weak

**Problem:** Per-hop PQC verification uses a 32-bit truncated SHA3-256
fingerprint.  32 bits gives a collision probability of 50% after ~65,536
distinct service IDs (birthday bound).

For a system with 256 services (realistic), the collision probability
is approximately 0.00076% — acceptable.  But the draft doesn't state
the security margin or the collision bound.

**Fix:** Add analysis:

```
The 32-bit fingerprint truncation provides approximately
2^16 (65,536) collision resistance (birthday bound).
For deployments with fewer than 1,000 service identifiers,
the probability of accidental collision is negligible
(< 0.012%).

Deployments with more than 10,000 service identifiers
SHOULD use 64-bit fingerprints in the Extended Register
Space (requires /12 or /16 Kingdom Mode for sufficient
ERS bits).

The fingerprint is NOT a substitute for full signature
verification.  Full ML-DSA-65 verification MUST be
performed at Shield boundaries.
```

### 3.4 — No Discussion of BPF Verifier Limitations

**Problem:** The draft says "Shim programs are verified by the kernel
BPF verifier" as if that's sufficient.  But:

- The BPF verifier has had CVEs (CVE-2021-3490, CVE-2022-23222, etc.)
- Verifier bypass = kernel code execution
- The draft doesn't specify a minimum BPF ISA version
- Different kernel versions have different verifier capabilities
- CAP_BPF requirements are not mentioned

**Fix:**

```
Shim program loading REQUIRES CAP_BPF and CAP_NET_ADMIN
capabilities (or equivalent).  Implementations MUST use
Linux kernel version 5.15 or later, which includes the
BPF ring buffer (bpf_ringbuf) and bounded loop support.

Operators SHOULD:
  - Pin BPF programs to prevent runtime modification
  - Use BPF token-based delegation (kernel 6.9+) where
    available
  - Monitor /sys/kernel/debug/tracing/trace_pipe for
    verifier warnings
  - Apply kernel security updates promptly, as the BPF
    verifier is a security-critical component
```

---

## CATEGORY 4: INTEROPERABILITY CONCERNS

### 4.1 — No Version Negotiation

**Problem:** The Monad has no version field.  If the protocol evolves
(and it will — you already have Age 1/2/3 in the technical summary),
how does a receiver know which version of the format it's looking at?

IOAM (RFC 9197) has an IOAM Opt-Type field specifically for this.

**Fix:** Add a version indicator.  Options:

(a) Reserve bits in the flags field for version
(b) Use different option type values per version (IANA-heavy)
(c) Add a 1-byte version field before the Monad (RECOMMENDED —
    your PROTOCOL_TECHNICAL_SUMMARY already has this at offset 0x00)

The PROTOCOL_TECHNICAL_SUMMARY's layout is better than the RFC draft's.
It has `version` at byte 0.  **Port that layout into the RFC.**

### 4.2 — No MTU / Fragmentation Discussion

**Problem:** The draft adds at minimum 24 bytes (2 HbH header + 2 option
TLV + 20 Monad) to every packet.  With Kingdom Mode metadata and chaos
payloads, this could be 40+ bytes.

On a standard 1500-byte MTU path, this reduces the usable payload.
With jumbo frames disabled, this could cause fragmentation.

Real RFCs address this.  RFC 9486 §5 discusses PMTUD and fragmentation
for IOAM.

**Fix:**

```
The Hop-by-Hop extension header adds a minimum of 24 octets
to each packet (8-octet HbH header + 2-octet option TLV +
20-octet Monad, padded to 8-octet boundary).  With optional
metadata, the overhead may reach 40-56 octets.

Within the Limited Domain, the operator SHOULD configure
the MTU to accommodate the maximum expected overhead.
For standard Ethernet (1500-byte MTU), the effective
payload is reduced to approximately 1436-1460 octets.

Implementations MUST NOT add the Hop-by-Hop extension
header if doing so would cause the packet to exceed the
path MTU.  If the packet cannot accommodate the minimum
24-octet overhead, Shield MUST either:

  (a) Forward the packet without the Monad (bypass mode), or
  (b) Fragment the inner packet before adding the Monad
      (NOT RECOMMENDED due to performance impact).

Jumbo frames (9000-byte MTU) are RECOMMENDED for Limited
Domain deployments using Kingdom Mode.
```

### 4.3 — No Backward Compatibility Testing Procedure

**Problem:** The draft claims backward compatibility — "unaware routers
skip the option."  But RFC 9673 §5.1 documents that many routers in
practice DROP packets with HbH options, or slow-path them.

The draft should acknowledge this reality.

**Fix:**

```
Note: While RFC 8200 specifies that routers should skip
unrecognized Hop-by-Hop options, operational experience
(documented in RFC 9098 and RFC 9673) shows that some
router implementations may drop or slow-path packets
containing Hop-by-Hop extension headers.

The Unheaded Protocol is designed for Limited Domains
(RFC 8799) where all intermediate nodes are operator-
controlled.  Deployments MUST ensure all intermediate
routers are configured to process (or at minimum forward)
packets containing the Unheaded Monad option.

The protocol is NOT intended for deployment across the
public Internet where intermediate routers may drop
Hop-by-Hop options.
```

---

## CATEGORY 5: LANGUAGE AND TONE

### 5.1 — Remove All Marketing/Poetic Language

Real RFCs are dry.  Deliberately, aggressively dry.  Every sentence
that isn't a requirement or a definition is wasted space that an IESG
reviewer will flag.

**Lines to remove or rewrite:**

| Line | Problem |
|------|---------|
| "Speed: light" | Not a technical statement |
| "The Rosetta Stone. The nervous system." | Poetry, not protocol |
| "The atom. The wire itself." | Same |
| "Human-speed interfaces for human-speed decisions" | Marketing |
| "THE PLAN IS SET. THE DASHBOARD AWAKENS. THE DOOM APPROACHES." | (BATTLEPLAN only, but the energy leaks into the RFC) |
| "walking the Pattern is always worth the fire" (Acknowledgments) | Beautiful, but move to a blog post |
| "giving meaning to raw exponent bytes" | Anthropomorphizing bytes |

**Fix:** Replace all poetic language with precise technical descriptions.
Save the poetry for the README, the blog post, and the conference talk.
The RFC is a legal document for implementers.

### 5.2 — Acknowledgments Section Needs Tightening

RFC acknowledgments are typically one paragraph naming contributors.
The current section has Zelazny literary criticism and personal
anecdotes.  These are great in a README or blog post — not in an RFC.

**Fix:**

```
The authors thank the Linux kernel BPF community, in
particular Alexei Starovoitov and Daniel Borkmann, for
the BPF infrastructure that enables this design.  Thanks
also to Adam Dunkels for demonstrating minimal protocol
implementations in constrained environments.
```

---

## CATEGORY 6: MISSING SECTIONS (Required by RFC Format)

### 6.1 — No "Applicability Statement"

Every experimental RFC needs one.  Where does this protocol apply?
Where does it NOT apply?

```
This protocol is applicable within Limited Domains
(RFC 8799) where:

  (a) All intermediate nodes are operator-controlled.
  (b) IPv6 Hop-by-Hop option processing is enabled on
      all nodes.
  (c) BPF program loading infrastructure is available.
  (d) A Sophia dictionary distribution mechanism is
      deployed.

This protocol is NOT applicable:
  (a) Across the public Internet.
  (b) On paths containing routers that drop HbH options.
  (c) In environments where BPF program loading is
      restricted by security policy.
```

### 6.2 — No "Manageability Considerations" Section

RFC 5765 (and common practice for 9000+ RFCs) expects a section on
how operators manage and monitor the protocol.

### 6.3 — No "Performance Considerations" Section

The draft makes performance claims ("nanoseconds", "wire speed",
"~3.7 MHz") without supporting data or measurement methodology.

**Fix:** Add benchmarks or remove claims.  Real RFCs either cite
measurements or use hedging language ("is expected to add
approximately X nanoseconds of processing delay per hop").

---

## CATEGORY 7: DISCREPANCIES BETWEEN DOCUMENTS

### 7.1 — RFC Draft vs PROTOCOL_TECHNICAL_SUMMARY Mismatch

| Field | RFC Draft -02 | PROTOCOL_TECHNICAL_SUMMARY |
|-------|---------------|---------------------------|
| Transport | IPv6 HbH extension header | IPv4 + 20-byte shim |
| Version field | Not present | offset 0x00, 1 byte |
| Register layout | R0-R4 generic | 14 named fields with specific offsets |
| Checksum | CRC-16 over Monad+metadata+trace_id | CRC-16 over bytes 0x00-0x11 |
| Event struct size | 64 bytes | 40 bytes |
| Anamnesis event types | 9 types (0x00-0x08) | 5 types (1-5) |
| Timestamp | u64 bpf_ktime_get_ns() | u64 bpf_ktime_get_ns() |

These are TWO DIFFERENT PROTOCOLS.  The RFC draft describes an IPv6
extension header protocol.  The technical summary describes an IPv4
shim protocol.  They need to be reconciled.

**Fix:** Decide which is canonical.  The RFC draft's IPv6 HbH approach
is the right long-term answer.  The technical summary's IPv4 shim is
the Age 1 reality.  The RFC should specify the IPv6 format and have
an appendix noting the IPv4 shim as an interim deployment option.

### 7.2 — Draft -01 vs Draft -02 Is a Complete Rewrite

Draft -01 and -02 share the same date but are substantially different
documents.  -02 adds Kingdom Mode, PQC, and restructures significantly.

**Fix:** Add a "Changes from draft-01" section per RFC convention:

```
Changes from draft-bellis-...-01 to -02:
  - Added Kingdom Mode address reclamation (Section 5)
  - Added Post-Quantum Cryptographic Identity Binding (Section 6)
  - Changed checksum from CRC-32 to CRC-16/CCITT
  - Changed checksum verification from egress-only to per-hop
  - Restructured flags bitfield
  - Added Anamnesis event types: KEY_OP, ANOMALY
  - Added IANA registries for PQC algorithms
  - Changed Anamnesis event structure (added input+output Monad)
```

---

## PRIORITY RANKING

### P0 — MUST FIX (Blocks any credible submission)

1. **Option type chg bit is wrong** (§1.3) — factual error
2. **Reconcile the two protocols** (§7.1) — IPv4 shim vs IPv6 HbH
3. **Add version field to Monad** (§4.1) — no extensibility without it
4. **Define Sophia wire format** (§2.5) — no interop without it
5. **Fix CRC vs integrity confusion** (§3.1) — security reviewers will reject

### P1 — SHOULD FIX (Expected by IESG reviewers)

6. Concrete exponent encoding binary format (§2.4)
7. MTU/fragmentation discussion (§4.2)
8. Applicability statement (§6.1)
9. Wotan helper interface spec (§2.6)
10. Remove marketing language (§5.1)
11. HbH option drop reality acknowledgment (§4.3)

### P2 — NICE TO HAVE (Polish for credibility)

12. Restructure to standard RFC section ordering (§1.2)
13. Performance considerations with data (§6.3)
14. Manageability considerations (§6.2)
15. Tighten acknowledgments (§5.2)
16. Changes-from-01 appendix (§7.2)
17. Fingerprint collision analysis (§3.3)

---

## THE GOOD STUFF (Credit Where Due)

Before we close — what's WORKING in this draft:

1. **The core idea is genuinely novel.** A fixed-size register file in
   HbH options with dictionary-compressed semantics — nobody has done
   this.  IOAM observes.  NSH encapsulates.  You compute.  That's new.

2. **Kingdom Mode address reclamation is clever.** Using deterministic
   prefix bits as register space within L2 overlays — mathematically
   sound and practically useful.

3. **The memory hierarchy model is clean.** L0 (Monad) → L1 (BPF map) →
   L2 (ring buffer) → L3 (WAL) → L4 (Sophia) is a well-thought-out
   cache hierarchy that maps naturally to BPF infrastructure.

4. **PQC identity binding is forward-looking.** Binding service IDs to
   ML-KEM-768 keypairs via Sophia is a smart way to get quantum
   resistance without wire overhead.

5. **RFC 9673 reference is perfect.** That's the exact RFC that
   rehabilitates HbH processing.  Citing it normatively is correct
   and timely.

6. **Prior art section is thorough and fair.** The IOAM/APN/NSH/MNA
   comparisons are accurate and well-differentiated.

---

## RECOMMENDED NEXT STEPS

1. Fix P0 items (especially the chg bit — that's embarrassing if it ships)
2. Port the PROTOCOL_TECHNICAL_SUMMARY's concrete field layout into the RFC
3. Strip all marketing language — every sentence should be testable
4. Write a separate "Sophia Dictionary Format" companion draft
5. Write a separate "Wotan Memory Protocol" companion draft
6. Restructure to standard RFC section ordering
7. Get an early review from someone on the IETF ippm or 6man working group

The foundation is solid.  The engineering is real.  Now make the
document as rigorous as the code.

---

*Reviewed with the paranoia of the Developer and the precision of the
Architect.  The Kingdom deserves an RFC worthy of its code.*
