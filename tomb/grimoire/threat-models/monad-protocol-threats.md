# Monad Wire Protocol Threat Model

**Version:** 1.0
**Date:** February 28, 2026
**Classification:** Internal — Grimoire Knowledge Base
**Author:** BlackMage (Tomb of Knowledge)
**Wire Format Version:** 0x01 (FROZEN — S67)
**Spec Reference:** draft-bellis-unheaded-protocol-foundation-05

---

## 1. Protocol Overview

The Monad is a 20-byte register file carried in an IPv6 Hop-by-Hop Options extension header. It travels with every packet inside the Kingdom's Limited Domain (RFC 8799). At each hop, a BPF program (the Shim) reads the Monad, performs computation, and writes updated state back to the packet.

### Wire Format (20 bytes, version 0x01)

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    Version    |     Flow      |    Source     |  Destination   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   QoS Class   |   Hop Count   |   TTL/Scope   |    Flags      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                    Scratch[0..7]                               |
|                    (8 bytes, exponent-encoded)                 |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|              Reserved         |          CRC-16               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

**Total option overhead:** 24 bytes (2B HbH header + 2B option TLV + 20B Monad)

### Flags Byte (offset 0x07)

```
Bit 7: CHAOS   (C)  — Chaos injection active (Yaldabaoth)
Bit 6: CANARY  (Y)  — Canary deployment path
Bit 5: TRACED  (T)  — Full trace active
Bit 4: ENCRYPT (E)  — Payload encrypted (intra-Kingdom TLS)
Bit 3: SAMPLED (S)  — Statistically sampled
Bit 2: MIRROR  (M)  — Mirror copy
Bit 1: CUSTOM  (CUST) — Scratch fields carry exponent-encoded values
Bit 0: RESERVED (R) — MUST be zero
```

### Key Properties

- **Scope:** Limited Domain only (never leaves the Kingdom)
- **Containment:** Shield stamps Monad ON at ingress, strips OFF at egress
- **Encoding:** Exponent-encoded fields (Sophia dictionaries define semantics)
- **Integrity:** CRC-16/CCITT over bytes 0-17
- **Identity:** Source/Destination fields map to Sophia dictionary entries
- **Computation:** BPF programs at each hop read/write the Monad

---

## 2. Threat Actors (Protocol-Specific)

### 2.1 Compromised Hop

A BPF Shim program on a compromised node within the Limited Domain. Has read/write access to the Monad register file at kernel datapath speed (~320 ns).

### 2.2 Rogue Edge Node

A compromised Shield node that can manipulate protocol birth/death events, inject Monads with crafted metadata, or fail to strip Monad bytes on egress (protocol leakage).

### 2.3 Internal Network Observer

An attacker with passive access to the lxdbr0 bridge network (e.g., via ARP spoofing or mirror port). Can observe Monad bytes in transit.

### 2.4 Sophia Dictionary Administrator

An entity with access to modify Sophia BPF hash maps or userspace dictionary files. Can change the semantic meaning of exponent-encoded fields.

### 2.5 External Observer (Post-Egress)

An attacker outside the Kingdom attempting to infer protocol behavior from timing, packet sizes, or any residual metadata. Under correct Shield operation, this actor should see zero protocol bytes.

---

## 3. Attack Surface

### 3.1 Wire Format Attacks

#### WF-01: Version Byte Spoofing

**Target:** Monad byte 0 (Version)
**Threat:** Injecting a packet with a future version number that current Shim programs do not validate, causing undefined behavior or bypass of processing logic.
**Severity:** HIGH
**Mitigations:**
- Shim MUST reject version != 0x01 (current frozen version)
- Unknown version -> drop packet, emit ANAMNESIS_VERSION_MISMATCH event
**Residual risk:** LOW if version check is enforced at every hop

#### WF-02: CRC-16 Collision

**Target:** Monad bytes 18-19 (CRC-16/CCITT)
**Threat:** Crafting a modified Monad payload that produces the same CRC-16 checksum as the original. CRC-16 has 2^16 = 65,536 possible values; birthday attack probability increases with number of forged attempts.
**Severity:** MEDIUM
**Analysis:** CRC-16 is an error-detection code, not a cryptographic MAC. For an 18-byte payload, the undetected error probability is approximately 1.5 x 10^-5. This is adequate for detecting transmission errors but NOT for detecting intentional tampering by a motivated attacker.
**Mitigations:**
- CRC-16 is for error detection, not authentication
- Cryptographic integrity must come from a higher layer (PQ identity binding in Sophia)
- Post-quantum HMAC over Monad bytes is planned for production
**Residual risk:** HIGH for intentional tampering without additional MAC; LOW for accidental corruption

#### WF-03: Flags Byte Manipulation

**Target:** Monad byte 7 (Flags)
**Threat:** Flipping flag bits to alter packet processing path:
- Setting CHAOS (bit 7) on production traffic to inject chaos testing behavior
- Clearing TRACED (bit 5) to evade observability
- Setting MIRROR (bit 2) to duplicate traffic toward attacker-controlled sink
- Setting ENCRYPT (bit 4) on unencrypted payload to skip inspection
**Severity:** HIGH
**Mitigations:**
- Shield validates flags against policy at ingress
- CHAOS flag requires explicit Yaldabaoth authorization key in scratch fields
- Anamnesis records flag state at every hop (tamper visible in event log)
**Residual risk:** MEDIUM — flag manipulation by a compromised hop is detectable but not preventable without per-hop authentication

#### WF-04: Scratch Field Abuse

**Target:** Monad bytes 8-15 (Scratch[0..7])
**Threat:** Using scratch fields as a covert channel to exfiltrate data through the Kingdom. Each packet carries 8 bytes of scratch data; at 10,000 packets/second, that is 80 KB/s of covert bandwidth.
**Severity:** HIGH
**Analysis:** The CUSTOM flag (bit 1) indicates scratch fields carry exponent-encoded values. If CUSTOM is clear, scratch fields are undefined. A rogue Shim can write arbitrary data to scratch regardless of flag state.
**Mitigations:**
- Scratch field values should be validated against Sophia dictionary ranges at egress
- Anomaly detection on scratch field entropy (high entropy = potential covert channel)
- Shield strips all Monad bytes at egress (data never leaves Kingdom)
**Residual risk:** MEDIUM — covert channel within the Kingdom is detectable but not within the protocol itself

#### WF-05: Reserved Field Exploitation

**Target:** Monad bytes 16-17 (Reserved)
**Threat:** Using reserved bytes for covert data storage or future format confusion. S67 freeze analysis considered repurposing these bytes but rejected it to preserve flexibility.
**Severity:** LOW
**Mitigations:**
- Shim MUST verify reserved bytes are zero
- Non-zero reserved bytes -> ANAMNESIS_RESERVED_NONZERO event
**Residual risk:** LOW

### 3.2 Protocol Boundary Attacks

#### PB-01: Shield Egress Failure (Protocol Leakage)

**Target:** Shield egress XDP program
**Threat:** If Shield fails to strip the 20-byte Monad on egress, the proprietary protocol metadata leaks to the external network. An external observer could then:
- Fingerprint the Kingdom by observing HbH option type 0x3E
- Extract service identity (Source/Destination fields)
- Infer traffic patterns from Flow/QoS fields
- Map internal architecture from hop counts
**Severity:** CRITICAL
**Mitigations:**
- Shield egress is a mandatory processing step (fail-closed)
- If Shield crashes, packets MUST NOT be forwarded (drop on failure)
- Monitoring: external probe checks for HbH options on egress traffic
- Anamnesis death events confirm strip occurred
**Residual risk:** MEDIUM — software bugs in XDP program could cause silent bypass

#### PB-02: Shield Ingress Injection

**Target:** Shield ingress XDP program
**Threat:** An external attacker crafts an IPv6 packet with a HbH option type 0x3E already present, attempting to inject pre-formed Monad data that will be trusted by internal hops.
**Severity:** CRITICAL
**Mitigations:**
- Shield ingress MUST strip any existing HbH options before stamping fresh Monad
- "Born at the gate" principle: Monad identity is always created by Shield, never inherited from external sources
- External packets with HbH options should trigger ANAMNESIS_EXTERNAL_INJECTION alert
**Residual risk:** LOW if Shield ingress correctly strips and re-stamps

#### PB-03: Birth/Death Event Forgery

**Target:** Anamnesis ring buffer (birth/death events)
**Threat:** A compromised Shield node emits forged birth events (claiming packets arrived that did not) or suppresses death events (hiding packet egress). This corrupts the observability timeline.
**Severity:** HIGH
**Mitigations:**
- Birth/death events include timestamps, source IP, and protocol state
- Cross-correlation between multiple hops detects inconsistencies
- Anamnesis events are immutable once written to ring buffer (append-only)
**Residual risk:** MEDIUM — a compromised Shield can manipulate events before they enter the ring buffer

### 3.3 Exponent Encoding Attacks

#### EE-01: Sophia Dictionary Poisoning

**Target:** Sophia BPF hash maps and userspace dictionary files
**Threat:** Modifying Sophia dictionaries to change the semantic meaning of exponent-encoded field values. For example, changing the source identity mapping so that service A's packets are interpreted as service B.
**Severity:** CRITICAL
**Mitigations:**
- Sophia dictionaries loaded from version-controlled config files
- Dictionary changes require CAP_BPF to modify kernel maps
- All dictionary updates emit ANAMNESIS_SOPHIA_UPDATE events
- Planned: cryptographic signatures on dictionary files (PQ-signed)
**Residual risk:** HIGH without dictionary signing; MEDIUM with planned PQ signing

#### EE-02: Exponent Overflow/Underflow

**Target:** All exponent-encoded fields (8-bit signed values)
**Threat:** Crafting exponent values that, when decoded via base^exponent * multiplier, produce extreme values (very large or negative) that cause integer overflow in consuming code.
**Severity:** MEDIUM
**Mitigations:**
- Sophia defines valid ranges per field
- Shim programs validate decoded values against per-field min/max bounds
- BPF verifier ensures arithmetic operations do not cause undefined behavior
**Residual risk:** LOW if bounds checking is consistently implemented

#### EE-03: Dictionary Hot-Swap Race

**Target:** Sophia dictionary update mechanism
**Threat:** During a dictionary hot-swap, a packet in transit may be partially processed with the old dictionary and partially with the new. This creates semantic inconsistency within a single packet's lifetime.
**Severity:** MEDIUM
**Mitigations:**
- Atomic BPF map updates (BPF_MAP_UPDATE_ELEM is atomic per entry)
- Dictionary versioning: Monad version field + dictionary version cross-check
- Drain period: pause processing during swap (increases latency, reduces risk)
**Residual risk:** MEDIUM — per-entry atomicity does not guarantee full-dictionary atomicity

### 3.4 Computational Model Attacks

#### CM-01: BPF Program Injection at XDP Hook

**Target:** XDP hook point on network interfaces
**Threat:** Loading a malicious BPF program that intercepts, modifies, or exfiltrates Monad data at wire speed. A rogue XDP program can:
- Silently copy all Monad metadata to a BPF ring buffer accessible to a userspace attacker process
- Modify Source/Destination fields to redirect traffic
- Clear TRACED flag to evade observability
- Use scratch fields as a covert channel
**Severity:** CRITICAL
**Mitigations:**
- BPF program loading requires CAP_BPF (capability restricted to root or daemon)
- BPF program attestation (planned: signed BPF bytecode)
- Runtime BPF program inventory (bpftool prog list monitoring)
- Container seccomp profiles block bpf() syscall
**Residual risk:** MEDIUM — depends on host-level access control

#### CM-02: Wotan Memory Bus Overflow

**Target:** Wotan per-flow ring buffers and WAL
**Threat:** Generating a flood of Monad events that exhaust Wotan's ring buffer capacity, causing event loss. If Wotan drops events, the observability timeline has gaps that could mask an attack.
**Severity:** HIGH
**Mitigations:**
- Ring buffer capacity bounded (10,000 entries default)
- Overflow policy: drop oldest (attacker can cause evidence loss)
- Back-pressure signaling from Wotan to Shim programs
- Overflow counter exposed as Prometheus metric
**Residual risk:** HIGH — bounded buffers inherently trade completeness for availability

#### CM-03: Turing-Complete Computation Abuse

**Target:** Monad + Wotan = Turing-complete computational model
**Threat:** The Monad with Wotan memory paging forms a Turing-complete system. A sufficiently complex Shim program could implement arbitrary computation, including encryption/decryption of covert messages, steganographic encoding, or slow-running computation that creates timing side channels.
**Severity:** MEDIUM
**Mitigations:**
- BPF verifier instruction count limits (1 million instructions per program)
- BPF program complexity analysis (static analysis at load time)
- Runtime instruction count monitoring via bpf_prog_info
**Residual risk:** LOW — BPF verifier provides hard bounds on per-packet computation

### 3.5 Kingdom Mode Attacks

#### KM-01: Address Reclamation Space Abuse

**Target:** Reclaimed IPv6 prefix bits used as extended computational registers
**Threat:** In Kingdom Mode, deterministic address prefix bits are reclaimed as register space. An attacker who knows the Kingdom prefix can predict the reclaimed bit positions and use them to encode covert data that survives standard IPv6 address inspection.
**Severity:** MEDIUM
**Analysis:** Formula: reclaimed = 2 x (128 - host_bits). For /16 mode, reclaimed = 2 x (128 - 16) = 224 bits = 28 bytes of additional register space per packet (in source + destination addresses combined).
**Mitigations:**
- Kingdom Mode operates only within the Limited Domain
- Shield strips ALL protocol state at egress (including address reclamation)
- External addresses are standard ULA (fd00::/8)
**Residual risk:** LOW — reclaimed space never leaves the Kingdom

#### KM-02: Post-Quantum Identity Binding Bypass

**Target:** PQ-signed service identifiers in Sophia dictionaries
**Threat:** If the PQ signature verification is slow (ML-KEM/ML-DSA operations), an attacker could flood packets faster than verification can complete, creating an authentication gap.
**Severity:** MEDIUM
**Mitigations:**
- PQ verification is cached per service identity (verify once, cache result)
- Signature verification is on Sophia dictionary entries (not per-packet)
- FIPS 203/204/205 implementations are constant-time
**Residual risk:** LOW — dictionary-level caching eliminates per-packet overhead

---

## 4. Protocol-Specific STRIDE Analysis

| Threat | Category | Target | Severity | Status |
|--------|----------|--------|----------|--------|
| External injection of pre-formed Monad | Spoofing | Shield ingress | CRITICAL | Mitigated (strip-and-restamp) |
| Service identity spoofing via Source field | Spoofing | Monad byte 2 | HIGH | Partial (Sophia lookup, no PQ sig yet) |
| Monad field modification by rogue hop | Tampering | All Monad bytes | HIGH | Detection only (CRC-16 + Anamnesis) |
| Sophia dictionary semantic change | Tampering | Exponent encoding | CRITICAL | Planned (PQ dictionary signing) |
| CRC-16 integrity bypass | Tampering | Monad bytes 18-19 | HIGH | Known limitation (MAC planned) |
| Protocol leakage at egress | Information Disclosure | Shield egress | CRITICAL | Mitigated (fail-closed strip) |
| Scratch field covert channel | Information Disclosure | Monad bytes 8-15 | HIGH | Detection (entropy analysis) |
| Hop count analysis | Information Disclosure | Monad byte 5 | LOW | Contained within Kingdom |
| Wotan ring buffer overflow | Denial of Service | Event pipeline | HIGH | Bounded buffers (evidence loss risk) |
| BPF instruction exhaustion | Denial of Service | Per-hop compute | MEDIUM | BPF verifier limits |
| Flag manipulation (clear TRACED) | Repudiation | Monad byte 7 | HIGH | Anamnesis cross-hop detection |
| Death event suppression | Repudiation | Shield egress | HIGH | Cross-correlation detection |

---

## 5. Covert Channel Analysis

The Monad wire format presents several potential covert channels:

### 5.1 Scratch Field Channel

**Bandwidth:** 8 bytes/packet x packet rate
**At 10,000 pps:** 80 KB/s (640 Kbps)
**Detection:** Entropy analysis on scratch fields, correlation with CUSTOM flag state
**Mitigation:** Egress validation of scratch values against Sophia ranges

### 5.2 Reserved Field Channel

**Bandwidth:** 2 bytes/packet x packet rate
**At 10,000 pps:** 20 KB/s (160 Kbps)
**Detection:** Non-zero reserved bytes trigger ANAMNESIS event
**Mitigation:** Shim zero-checks at every hop

### 5.3 Timing Channel

**Bandwidth:** Variable (depends on timing resolution)
**Mechanism:** Modulating inter-packet arrival times to encode bits
**Detection:** Statistical analysis of packet timing distributions
**Mitigation:** Traffic shaping at Shield egress

### 5.4 Exponent Value Channel

**Bandwidth:** Depends on valid range width per field
**Mechanism:** Encoding data within the valid range of exponent values (e.g., if QoS has 8 valid values, 3 bits per packet)
**Detection:** Anomalous exponent value distributions per source
**Mitigation:** Baseline and alert on per-field value distributions

### 5.5 Kingdom Mode Address Channel

**Bandwidth:** Up to 28 bytes/packet in /16 mode (source + destination reclaimed bits)
**At 10,000 pps:** 280 KB/s (2.24 Mbps)
**Detection:** Address bit pattern analysis against expected Kingdom prefix
**Mitigation:** Shield egress strips all reclaimed bits, rewrites standard ULA addresses

**Total theoretical covert bandwidth (worst case):** ~380 KB/s within the Kingdom.
**External exposure:** Zero (Shield strips everything at egress). Covert channels exist only within the Limited Domain. They represent an internal threat from compromised hops, not an external data exfiltration vector.

---

## 6. Recommended Hardening

| Priority | Recommendation | Addresses |
|----------|---------------|-----------|
| P0 | Add HMAC (or PQ-MAC) over Monad bytes 0-17, replacing CRC-16 for authentication | WF-02, STRIDE Tampering |
| P0 | Shield fail-closed verification (external probe for HbH leakage) | PB-01 |
| P1 | PQ-signed Sophia dictionaries (FIPS 204 ML-DSA) | EE-01 |
| P1 | BPF program attestation (signed bytecode, verified at load) | CM-01 |
| P1 | Scratch field entropy monitoring in Anamnesis | WF-04, Covert 5.1 |
| P2 | Atomic dictionary swap with drain period | EE-03 |
| P2 | Per-hop flag validation against policy (not just at Shield) | WF-03 |
| P2 | Wotan overflow alerting and evidence preservation | CM-02 |
| P3 | Timing channel mitigation (traffic shaping at egress) | Covert 5.3 |
| P3 | Reserved field zero-check enforcement at every hop | WF-05, Covert 5.2 |

---

## 7. Validation Plan

| Test | Type | Target Threat |
|------|------|--------------|
| Inject pre-formed HbH option from external | LICH-001 | PB-02 |
| Verify Shield strips Monad on egress (packet capture) | LICH-001 | PB-01 |
| Modify Monad CRC and observe detection | Fuzzing | WF-02 |
| Flip CHAOS flag on production packet | Fuzzing | WF-03 |
| Write high-entropy data to scratch fields | Covert channel test | WF-04 |
| Flood Wotan with Monad events to overflow ring buffer | LICH-008 | CM-02 |
| Load unauthorized BPF program | LICH-006 | CM-01 |
| Modify Sophia dictionary during traffic flow | Race condition test | EE-03 |
| Send version 0xFF Monad and observe handling | Fuzzing | WF-01 |
| Measure PQ signature verification latency under load | Performance test | KM-02 |

---

**Next review:** After S67 wire format deployment on WEST bare metal
**Owner:** BlackMage (Tomb of Knowledge)
**Spec reference:** draft-bellis-unheaded-protocol-foundation-05
