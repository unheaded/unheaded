<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# G8 reference packages: RFC Editor rulings

Ruled 2026-10-27, S-TRUE-WIRE Phase 1 Steps 68-72 (M-146). G8 is the group of protocol
packages that no binary links (`docs/UNREACHED-GO-PACKAGES.md`). The question for each:
is it a **conformance reference**, i.e. does a draft section specify what its tests
check? A reference earns its place only if a draft points at it; the rest is dead weight
the ADR-090 sweep already named.

Rulings:

- **KEEP**: conformance reference for the named section. It carries a banner saying so;
  its tests are the conformance suite for that section.
- **KEEP-AFTER**: a reference once the named step lands; until then its tests are not an
  oracle.
- **WIRE IN**: a binary gains it in the named phase.
- **DECIDED IN**: another phase of this plan rules on it with more evidence.
- **E3 DELETE**: no draft section specifies it. The row goes to the E3 delete/keep table
  (Step 247, Stevie); nothing is deleted in this phase.

| Package | Ruling | Draft section | Why | Action (step) |
|---|---|---|---|---|
| pkg/protocol/errors | KEEP-AFTER | foundation-00 §"Error Code Registry" (13 codes) | Today it exports 16-bit `UNHD_*` constants whose names and numbers match neither the registry nor `docs/protocol/error-registry.md` | Phase 4 Steps 132-134 rewrite it to the registry and add the `doc.go` banner |
| pkg/protocol/settings | E3 DELETE (ruled Phase 4 Step 150; overrides Step 69's KEEP) | none. wotan-memory-03 names a SETTINGS exchange (error origin 0x06, the "Invalid SETTINGS frame (W7)" row) and sophia-dictionary-03 :467 a SETTINGS-configurable QPACK capacity, but no current draft defines a SETTINGS frame, identifier or encoding; foundation-06 has no SETTINGS text at all. The only definition is the superseded wotan-memory-02 §"SETTINGS Exchange (PATCH W7)" (u16 id + u32 value, ids 0x00-0x07 L1_CACHE_SIZE..HMAC_ENABLE), which this package does not implement (varint pairs, ids 0x01-0x07 MaxDictionarySize..WotanBufferSize) | Step 150's KEEP rested on "foundation-06 names SETTINGS_* in its registries", false at 2af5142b (`grep -ci settings` on foundation-06 is 0). A reference with no section to conform to has no oracle, the same reason as the flowtype/sequence/registry overrides. If a wotan -01 specifies the SETTINGS frame, re-rule | E3 row (Step 247); nothing deleted in this phase |
| pkg/protocol/lifecycle | E3 DELETE (ruled Phase 4 Step 150) | none. wotan-memory-03 §"GOAWAY Frame DoS (PATCH W8)" is one line pointing at "draft-02 Section 10.2"; foundation-06 has no GOAWAY text. The superseded wotan-memory-02 §"GOAWAY Frame (PATCH W8)" defines a frame (magic 0xC0FFEE00, reason, last_flow_id, debug data) that this package does not implement (6 octets: LastFlowID + ErrorCode), and CANCEL_FLOW is defined nowhere | GOAWAY semantics only matter on a stream the Kingdom multiplexes; no Wotan gRPC or HTTP stream sends or reads GOAWAY (`grep -rni goaway services/wotan pkg/wotan-client`, non-test: 0), so the override condition does not hold | E3 row (Step 247). Shield follows the ruling: Phase 5 removes `GOAWAY_STATE` and the dead lookup at `ebpf/shield-ebpf/src/main.rs` :513-519 (Step 213) |
| pkg/protocol/tlv | KEEP-AFTER | foundation-00 §"TLV Type Registry (Extended)" | Its constants disagree with the registry: `0x01` is `PRIORITY_HINT` (registry: Ring Path Counter), core range `0x00-0x0F` (registry: `0x00-0x1F` foundation, `0x20-0x3F` Sophia, `0x40-0x5F` Wotan), `0x80-0xFF` "padding" (registry: the critical bit) | Banner added (Step 70) saying so; Phase 4 Step 139 realigns the constants to the registry (a code change for that lane), or the ruling becomes E3 DELETE |
| pkg/protocol/integrity | WIRE IN (HMAC) / done (CRC) | none for the HMAC tag (foundation-00 §"Integrity" leaves the optional HMAC field undefined) | Its CRC-16 is folded into `encoding.CRC16CCITT` by Phase 3 (merged); `HMACComputer` (HKDF per-flow secret, HMAC-SHA256 truncated to 8 octets) is the candidate key derivation for Shield's `HMAC_KEYS` writer | Phase 5 Steps 183-192 use it for the writer, or it becomes an E3 DELETE row; a draft section for the tag format is a foundation `-01` item |
| pkg/protocol/sophiasync | WIRE IN | sophia-dictionary-00 §"Dictionary Distribution" (delta signature field) and §"Dictionary Poisoning Attack Vectors" (entries MUST be signed with ML-DSA-65) | Its `computeSignature` is a bare SHA-256, not the ML-DSA-65 the draft requires | Phase 8 Steps 283-314 (Sophia dictionary signing) |
| pkg/protocol/migration | DECIDED IN Phase 5 | none (no draft defines flow-migration or Shield retry tokens) | Shield's `RETRY_TOKENS` map needs a userspace writer; this package generates retry tokens | Phase 5 writer uses it (WIRE IN) or E3 DELETE |
| pkg/protocol/amplification | E3 DELETE | foundation-00 registers TLV `0x01` Ring Path Counter (4 octets); this package keeps a 2-octet counter and no implementation emits the TLV | Not a reference of the registered format | E3 row (Step 247) |
| pkg/protocol/dos | E3 DELETE | none | Go backpressure trackers for BPF rings and maps; the anamnesis draft's drop-counter rule needs no Go mirror | E3 row |
| pkg/protocol/intermediary | E3 DELETE | foundation-00 §5.3 version rule (now matched: only `0x01`, silent drop) | The rule is enforced in the eBPF hop path; this Go copy is linked by nothing. `SPEC-VS-CODE-AUDIT.md` cites it as a mismatch, not as an oracle | E3 row |
| pkg/protocol/prefetch | E3 DELETE | none (wotan-memory-00 mentions prefetching; no hint format is defined) | | E3 row |
| pkg/protocol/flowtype | E3 DELETE (overrides Step 69's KEEP) | none | It packs a 2-bit flow type into Monad flags bits 1:0, which foundation-00 registers as CUSTOM and RSVD in the frozen v0x01 flags octet: a reference to it would contradict the spec | E3 row |
| pkg/protocol/sequence | E3 DELETE (overrides Step 69's KEEP) | none; pqc-authentication-00 §12.2 defines per-flow SeqNum with drop-if-not-greater, which this package does not implement (it reports gaps and reordering) | Not a reference of the PQC replay rule | E3 row |
| pkg/protocol/registry | E3 DELETE (overrides Step 69's KEEP) | none (a generic code-point registry type; no draft specifies it) | A library pattern, not a reference; the drafts' registries are text | E3 row |
| pkg/monad | KEEP-AFTER | pqc-authentication-00 §5 (value layout) and §7 (signed pseudo-header) | `FlagPQCActive = SAMPLED\|CUSTOM` is a third reading of the flags octet (RFC Editor TW-2); its flag constants duplicate `pkg/protocol/bpfschema` | After TW-2 is answered: keep `pqc_value.go`/`pseudo_header.go` as the reference, fold or drop `flags.go` |

The three overrides of the Step 69 recommendation apply its own rationale ("a
conformance reference earns its place only if a draft section points at it"): no draft
section specifies `flowtype`, `sequence` or `registry`, and `flowtype` contradicts one.

E3 table rows to add (Step 247): `pkg/protocol/{amplification,dos,intermediary,prefetch,flowtype,sequence,registry}`,
plus `lifecycle` and `settings` (Phase 4 Step 150), and `migration` and `integrity` (HMAC half) if Phase 5 does not keep them.

RFC Editor follow-up from Step 150: wotan-memory-03 (and its IETF twin) cites a SETTINGS exchange and GOAWAY frames only through origins 0x06/0x07, three gRPC error rows and "See draft-02 Section 10.2"; draft-02 is superseded and was never submitted, so the -00 submission references frames it does not define. Either restore the two definitions in a -01 or drop the origins, rows and pointer.
