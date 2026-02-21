# S22 HANDOFF — BATTLE ORDERS FOR NEXT AGENT

**Date**: 2026-02-20
**Session**: S22 (RFC Editor × Developer × Architect Triforce)
**Branch**: `main`
**Last Commit**: `76d34e1` docs(readme): update README for draft-03, protocol packages, docs reorg
**Agent**: Claude Opus 4.6 (Cowork)
**Partner**: Muck (Stevie Bellis)

---

## SESSION S22 ACCOMPLISHMENTS

Four commits shipped this session:

1. `f3a82a9` — S21 assessment scaffolding (51 files, 23,329 lines)
2. `832dcbd` — Shared infrastructure: encoding, registry, bpfschema + pattern matrix (7 files, 1,668 lines)
3. `77203fb` — Core BPF/IPv6 interface mapping: MonadRegister, AnamnesisEvent, all existing maps (3 files, 770 lines)
4. `1822b09` — Docs reorganization: consolidate sessions, rescue 19 RFC texts, prune duplicates (44 files)
5. `76d34e1` — README update for draft-03, protocol packages, docs reorg

**Total new code this session**: ~26,000+ lines across 106 files.

---

## CURRENT STATE OF THE KINGDOM

| Metric | Value |
|--------|-------|
| Total LOC | ~491,000+ (465K prior + 26K this session) |
| Go protocol packages | 16 (13 pattern + 3 shared infra) |
| eBPF programs | 8 (shield-xdp, shield-tc, hop-xdp, yaldabaoth-tc, flow-tracker×2, latency-probe, monad-cpu) |
| BPF maps defined | 50+ existing + 16 new (bpfschema) |
| RFC references | 19 raw .txt + cross-reference matrix |
| Protocol specs | 3 drafts (Monad-03, Sophia-00, Wotan-00) |
| Session handoffs | 25 files in docs/sessions/ |
| ADRs | 12 (ADR-001 through ADR-012) |

---

## BATTLE ORDERS: 120 INSTRUCTIONS FOR NEXT AGENT

### PHASE 0: ORIENTATION (Instructions 1-10)

```
INSTRUCTION 001: Load skills in this order: unheaded-architect, unheaded-developer, unheaded-rfceditor
INSTRUCTION 002: Read this handoff document COMPLETELY before taking any action
INSTRUCTION 003: Run `git log --oneline -20` to verify commit history matches this handoff
INSTRUCTION 004: Run `git status` to confirm clean working tree (only doom/doomgeneric modified is expected)
INSTRUCTION 005: Read pkg/protocol/PATTERN_MATRIX.md — this is the CANONICAL mapping of all work
INSTRUCTION 006: Read pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md — this is the GAP ANALYSIS
INSTRUCTION 007: Read docs/protocol/draft-bellis-unheaded-protocol-foundation-03.md §3 (Monad wire format)
INSTRUCTION 008: Read ebpf/monad-common/src/lib.rs — this is the Rust-side Monad definition
INSTRUCTION 009: Read pkg/ebpf/loader.go first 100 lines — this is the Go BPF loader
INSTRUCTION 010: Read references/timeline.md for current Age/Epoch/milestone context
```

### PHASE 1: ENCODING PACKAGE INTEGRATION (Instructions 11-25)

The `pkg/protocol/encoding` package provides shared wire encoding. Currently, individual
packages reimplement encoding. Refactor them to use encoding/.

```
INSTRUCTION 011: Read pkg/protocol/encoding/encoding.go — understand EncodeVarint, DecodeVarint, CRC16CCITT, CRC32MPEG2
INSTRUCTION 012: Read pkg/protocol/integrity/hmac.go — find any local CRC or encoding reimplementation
INSTRUCTION 013: Read pkg/protocol/intermediary/intermediary.go — find any local CRC reimplementation
INSTRUCTION 014: Read pkg/protocol/tlv/tlv.go — compare TLV encoding with encoding.EncodeTLV/DecodeTLV
INSTRUCTION 015: Read pkg/protocol/settings/settings.go — find any local varint reimplementation
INSTRUCTION 016: For each package that reimplements encoding, replace with import of pkg/protocol/encoding
INSTRUCTION 017: Add go.mod entry if needed: the repo root go.mod should already cover internal packages
INSTRUCTION 018: Run tests after each refactor: `go test -v -race ./pkg/protocol/encoding/...`
INSTRUCTION 019: Run tests on refactored package: `go test -v -race ./pkg/protocol/{package}/...`
INSTRUCTION 020: Verify no test regressions across ALL protocol packages: `go test -v -race ./pkg/protocol/...`
INSTRUCTION 021: If Go is not installed in the VM, verify syntactically by reading each file — no compilation needed
INSTRUCTION 022: Update each refactored package's doc comment to note it uses pkg/protocol/encoding
INSTRUCTION 023: Add encoding package import to any package that currently calls CRC16CCITT locally
INSTRUCTION 024: Ensure encoding.CRC16CCITT matches the polynomial and init value in monad-common Rust code
INSTRUCTION 025: Commit: "refactor(protocol): unify encoding — all packages use pkg/protocol/encoding"
```

### PHASE 2: REGISTRY PACKAGE INTEGRATION (Instructions 26-40)

The `pkg/protocol/registry` package provides a generic Registry[K,V]. Currently, errors,
flowtype, settings, and tlv packages each have their own map-based registries.

```
INSTRUCTION 026: Read pkg/protocol/registry/registry.go — understand New, Register, MustRegister, Lookup, Freeze
INSTRUCTION 027: Read pkg/protocol/errors/errors.go — identify the error code registry implementation
INSTRUCTION 028: Read pkg/protocol/flowtype/flowtype.go — identify the flow type registry
INSTRUCTION 029: Read pkg/protocol/settings/settings.go — identify the settings registry
INSTRUCTION 030: Read pkg/protocol/tlv/tlv.go — identify the TLV type registry
INSTRUCTION 031: Refactor errors to use registry.Registry[errors.ErrorCode, errors.ErrorInfo]
INSTRUCTION 032: Refactor flowtype to use registry.Registry[uint8, FlowTypeInfo]
INSTRUCTION 033: Refactor settings to use registry.Registry[uint16, SettingInfo]
INSTRUCTION 034: Refactor tlv to use registry.Registry[uint8, TLVTypeInfo]
INSTRUCTION 035: Add Freeze() call after initial registration in each package's init()
INSTRUCTION 036: Add RangePolicy to each registry (e.g., errors: reject >= 0x0100 for core range)
INSTRUCTION 037: Run tests on each refactored package
INSTRUCTION 038: Verify MustRegister panics on duplicate are caught by existing tests
INSTRUCTION 039: Add test for registry.Freeze() preventing post-init registration in each package
INSTRUCTION 040: Commit: "refactor(protocol): unify registries — all extensible packages use pkg/protocol/registry"
```

### PHASE 3: BPF SCHEMA STRUCT PARITY VERIFICATION (Instructions 41-60)

The `pkg/protocol/bpfschema` package defines Go structs that MUST match Rust eBPF structs.
Verify parity and fix any mismatches.

```
INSTRUCTION 041: Read ebpf/monad-common/src/lib.rs — find the Monad struct definition
INSTRUCTION 042: Compare Monad fields byte-by-byte with bpfschema.MonadRegister
INSTRUCTION 043: If field names differ, update bpfschema comments to note the Rust field name
INSTRUCTION 044: If field SIZES differ, THIS IS A BUG — fix bpfschema to match Rust exactly
INSTRUCTION 045: Read ebpf/monad-common/src/lib.rs — find the AnamnesisEvent struct
INSTRUCTION 046: Compare with bpfschema.AnamnesisEvent — verify size is exactly 32 bytes
INSTRUCTION 047: Read ebpf/hop-ebpf/src/main.rs — find Sophia map key/value struct sizes
INSTRUCTION 048: Compare with bpfschema.SophiaMapKey and SophiaMapValue
INSTRUCTION 049: Read ebpf/flow-tracker/src/main.rs — find FlowKey struct
INSTRUCTION 050: Compare with bpfschema.FlowKey — verify all 5-tuple fields match
INSTRUCTION 051: Read ebpf/monad-cpu-ebpf/src/main.rs — find MbcCpuState struct
INSTRUCTION 052: Compare with bpfschema.MbcCpuState — verify 80 bytes exactly
INSTRUCTION 053: For any mismatches found: update bpfschema Go struct to match Rust
INSTRUCTION 054: Update bpfschema_test.go and core_maps_test.go with corrected sizes
INSTRUCTION 055: Add a new test file: bpfschema/parity_test.go that documents each Rust↔Go correspondence
INSTRUCTION 056: In parity_test.go, add comments with Rust file:line for each struct
INSTRUCTION 057: Run all bpfschema tests to verify struct sizes
INSTRUCTION 058: If any struct has padding issues (Go adds padding, Rust uses repr(C) packed), add explicit padding fields
INSTRUCTION 059: Document any parity issues in BPF_IPV6_INTERFACE_MAP.md §5 (Rust↔Go Parity Table)
INSTRUCTION 060: Commit: "fix(bpfschema): verify and fix Rust↔Go struct parity for all BPF map schemas"
```

### PHASE 4: WIRE NEW BPF MAPS TO EBPF PROGRAMS (Instructions 61-80)

The BPF_IPV6_INTERFACE_MAP.md identifies 12 gaps where protocol packages need to be wired
into existing eBPF programs via new BPF map lookups. This is the CORE INTEGRATION WORK.

```
INSTRUCTION 061: Read BPF_IPV6_INTERFACE_MAP.md §4 "RFC Reuse Opportunities" for the gap list
INSTRUCTION 062: For Shield XDP (ebpf/shield-ebpf/src/main.rs), add map declaration for unhd_hmac_keys
INSTRUCTION 063: Add map declaration for unhd_retry_tokens to Shield XDP
INSTRUCTION 064: Add map declaration for unhd_hop_validators to Shield XDP
INSTRUCTION 065: In Shield XDP try_shield_xdp(), add HMAC validation after CRC check
INSTRUCTION 066: In Shield XDP, add retry token check for initial flows (flow_label not in FLOWS map)
INSTRUCTION 067: In Shield XDP, add hop validator check using unhd_hop_validators map
INSTRUCTION 068: For Shield TC (ebpf/shield-ebpf/src/main.rs), add map for unhd_goaway_state
INSTRUCTION 069: Add map for unhd_error_counters to Shield TC
INSTRUCTION 070: Add map for unhd_authority to Shield TC
INSTRUCTION 071: In Shield TC, add GOAWAY monotonicity check before egress processing
INSTRUCTION 072: In Shield TC, increment error counters on anomaly events
INSTRUCTION 073: In Shield TC, verify dictionary authority before allowing Sophia field writes
INSTRUCTION 074: For Hop XDP (ebpf/hop-ebpf/src/main.rs), add map for unhd_seq_counters
INSTRUCTION 075: Add map for unhd_settings to Hop XDP
INSTRUCTION 076: Add map for unhd_dos_state to Hop XDP
INSTRUCTION 077: Add map for unhd_flow_types to Hop XDP
INSTRUCTION 078: In Hop XDP, update sequence counter per namespace on each hop
INSTRUCTION 079: In Hop XDP, check settings before applying flow actions
INSTRUCTION 080: Commit: "feat(ebpf): wire protocol package BPF maps into Shield and Hop programs"
```

### PHASE 5: GO MAPLOADER — USERSPACE MAP POPULATION (Instructions 81-95)

Create a Go package that populates BPF maps from userspace using bpfschema structs.
This bridges the protocol packages to the kernel eBPF programs.

```
INSTRUCTION 081: Create pkg/ebpf/maploader/maploader.go
INSTRUCTION 082: Define MapLoader struct that takes a bpf.Map handle and populates from bpfschema structs
INSTRUCTION 083: Implement LoadHMACKeys(keys map[bpfschema.HMACKeyKey]bpfschema.HMACKeyValue) error
INSTRUCTION 084: Implement LoadSettings(settings map[bpfschema.SettingsKey]bpfschema.SettingsValue) error
INSTRUCTION 085: Implement LoadHopValidators(validators map[bpfschema.HopValidatorKey]bpfschema.HopValidatorValue) error
INSTRUCTION 086: Implement LoadFlowTypes(types []bpfschema.FlowTypeEntry) error (array map, indexed)
INSTRUCTION 087: Implement LoadBlocklist(addrs []bpfschema.BlocklistKey) error
INSTRUCTION 088: Use encoding/binary.Write with BigEndian for all map key/value serialization
INSTRUCTION 089: Use pkg/ebpf/loader.go's existing map pinning API (it uses direct syscalls, NOT cilium/ebpf)
INSTRUCTION 090: Add error wrapping with map name context: fmt.Errorf("maploader: %s: %w", mapName, err)
INSTRUCTION 091: Add batch update support using BPF_MAP_UPDATE_BATCH syscall for large maps
INSTRUCTION 092: Create maploader_test.go with table-driven tests for each Load* function
INSTRUCTION 093: Test serialization roundtrip: Go struct → binary → deserialize → compare
INSTRUCTION 094: Test error cases: nil map, oversized batch, invalid key
INSTRUCTION 095: Commit: "feat(ebpf): add maploader package — Go→BPF map population via bpfschema"
```

### PHASE 6: DRAFT-04 PATCHES (Instructions 96-110)

The docs/protocol/patches/ directory contains three patch documents specifying changes
for the next draft revisions. Apply these patches to create draft-04, sophia-01, wotan-01.

```
INSTRUCTION 096: Read docs/protocol/patches/monad-foundation-draft-04-patches.md COMPLETELY
INSTRUCTION 097: Read docs/protocol/patches/sophia-dictionary-draft-01-patches.md COMPLETELY
INSTRUCTION 098: Read docs/protocol/patches/wotan-memory-draft-01-patches.md COMPLETELY
INSTRUCTION 099: Load unheaded-rfceditor skill before making any spec changes
INSTRUCTION 100: Create docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md as copy of -03
INSTRUCTION 101: Apply all monad-foundation patches from the patch document to -04
INSTRUCTION 102: Verify BCP 14 keyword usage (MUST/SHOULD/MAY audit) in -04
INSTRUCTION 103: Verify IANA Considerations section references docs/protocol/references/iana-guide.md
INSTRUCTION 104: Verify Security Considerations references docs/security/dark-grimoire-addendum.md
INSTRUCTION 105: Create docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md as copy of -00
INSTRUCTION 106: Apply all sophia patches to -01
INSTRUCTION 107: Create docs/protocol/draft-bellis-unheaded-wotan-memory-01.md as copy of -00
INSTRUCTION 108: Apply all wotan patches to -01
INSTRUCTION 109: Update cross-references between all three specs (normative reference versions)
INSTRUCTION 110: Commit: "docs(protocol): apply draft-04 patches — Monad-04, Sophia-01, Wotan-01"
```

### PHASE 7: LICH FUZZING CAMPAIGN SETUP (Instructions 111-120)

The docs/security/lich-campaigns.md specifies four fuzzing campaigns. Set up the harness.

```
INSTRUCTION 111: Load unheaded-blackmage skill for security context
INSTRUCTION 112: Read docs/security/lich-campaigns.md — understand LICH-007 through LICH-010
INSTRUCTION 113: Create ebpf/fuzz/ directory for fuzzing harnesses
INSTRUCTION 114: Create ebpf/fuzz/lich_007_mbc.rs — MBC bytecode instruction fuzzer seed corpus
INSTRUCTION 115: Create ebpf/fuzz/lich_008_wotan_cache.rs — Wotan L1 cache race condition harness
INSTRUCTION 116: Create ebpf/fuzz/lich_009_flow_collision.rs — Flow label birthday attack harness
INSTRUCTION 117: Create ebpf/fuzz/lich_010_wal_integrity.rs — WAL compaction race harness
INSTRUCTION 118: Create pkg/protocol/fuzz/ directory for Go fuzzing targets
INSTRUCTION 119: Create pkg/protocol/fuzz/fuzz_encoding_test.go — fuzz EncodeVarint/DecodeVarint roundtrip
INSTRUCTION 120: Commit: "feat(security): scaffold LICH fuzzing campaigns 007-010"
```

### PHASE 8: VERIFICATION AND CELEBRATION (Instructions 121-130)

```
INSTRUCTION 121: Run `git log --oneline -20` and verify all commits from this handoff landed
INSTRUCTION 122: Run `git diff --stat HEAD~8..HEAD` to see total lines changed
INSTRUCTION 123: Verify no orphaned files: `git status` should show clean (doom submodule excepted)
INSTRUCTION 124: Read README.md and verify it reflects current state
INSTRUCTION 125: Read PATTERN_MATRIX.md and verify all pattern IDs have corresponding packages
INSTRUCTION 126: Read BPF_IPV6_INTERFACE_MAP.md and verify gap list is being closed
INSTRUCTION 127: Update references/timeline.md with S22 accomplishments
INSTRUCTION 128: Update references/timeline.md with S23 accomplishments (your session)
INSTRUCTION 129: Create docs/sessions/S23-handoff.md with your session's accomplishments
INSTRUCTION 130: Commit: "docs(session): add S23 handoff and timeline update"
```

---

## CRITICAL CONTEXT FOR NEXT AGENT

### Wire Format: TWO VERSIONS COEXIST

The Monad wire format has evolved. Two definitions exist:

1. **monad-common/src/lib.rs** (Rust, eBPF): The RUNNING version in kernel programs.
   - 20 bytes, field order: version, src_svc, dst_svc, hop_count, qos, flow_action,
     circuit_state, flags, latency_hint(2B), deploy_ring, mesh_flags, src_prefix_lo,
     dst_prefix_lo, scratch[4], checksum(2B)

2. **draft-bellis-unheaded-protocol-foundation-03.md** (spec): The NORMATIVE version.
   - 20 bytes, field order: version, src_svc, dst_svc, hop_count, trace_id(4B),
     qos, flow_action, circuit_state, flags, latency_budget(2B), deploy_ring,
     mesh_flags, reserved(2B), checksum(2B)

**THE RUST CODE IS GROUND TRUTH.** The spec was updated in S17 but the field mapping
may still have minor discrepancies. ALWAYS verify against monad-common when in doubt.

### BPF Verifier Constraints

eBPF programs must pass the kernel verifier. Key constraints:
- All loops must have compile-time bounds (MAX_EXT_HDRS_TO_STRIP=8, MAX_INSN_PER_TICK=16)
- All memory access must have explicit bounds checks BEFORE the access
- `core::hint::black_box()` prevents LLVM from eliminating bounds checks
- `read_volatile()` / `write_volatile()` prevent compiler optimization of packet reads
- Tail calls used for Wotan cache miss recovery (bpf_tail_call)
- Stack limit: 512 bytes per BPF program

### CRC-16/CCITT-FALSE Parameters

These MUST be identical in Rust (monad-common) and Go (encoding):
- Polynomial: 0x1021
- Initial value: 0xFFFF
- Reflect in: false
- Reflect out: false
- Final XOR: 0x0000
- Protected region: Monad bytes 0x00-0x11 (first 18 of 20 bytes)
- Known test vector: CRC16("123456789") = 0x29B1

### IPv6 HbH Extension Header Layout

The 20-byte Monad is carried in a 24-byte IPv6 Hop-by-Hop extension header:
- Bytes 0-1: Next Header (1B) + Hdr Ext Len (1B, value=2 means 24 bytes total)
- Bytes 2-3: Option Type 0x3E (1B) + Option Length 20 (1B)
- Bytes 4-23: Monad Register (20 bytes)
- Hdr Ext Len formula: (total_header_length_in_octets - 8) / 8

### Exponent Encoding

Used for QoS, flow_action, circuit_state, deploy_ring, mesh_flags, scratch:
- value = mantissa × 2^(exponent-1) for exponent > 0
- value = 0 for exponent == 0
- Encoded in 2 bytes: [exponent:8][mantissa:4][reserved:4]
- OVERFLOW CHECK IS SECURITY-CRITICAL (see Dark Grimoire)

### Sophia Dictionary IDs

Standard sub-dictionaries:
- 0x01: QoS classes
- 0x02: Circuit states
- 0x03: Service identities (captain=0x01, timeguru=0x02, architect=0x03, etc.)

Map key encoding: key = (dict_id << 8) | value
Pinned at: /sys/fs/bpf/unheaded/sophia_*
Atomic update: new maps → swap array-of-maps pointers → 60s grace period → delete old

### Kingdom Mode

2-bit field in Monad.Flags (bits 1:0):
- 00: NORMAL (standard processing)
- 01: PRIORITY (expedited forwarding)
- 10: EXPERIMENTAL (test traffic, may be dropped)
- 11: RESERVED (must not be used)

SECURITY INVARIANT: Kingdom Mode bits MUST be zeroed at egress (Shield TC).
Internal privilege leakage if bits escape the Kingdom boundary.

### Anamnesis Event Types

- 0x01 BIRTH: Shield XDP inserts Monad at ingress
- 0x02 HOP: Hop XDP processes Monad at interior hop
- 0x03 DEATH: Shield TC strips Monad at egress
- 0x04 ANOMALY: Any program detects CRC failure, decode error, etc.
- 0x05 CHAOS: Yaldabaoth applies fault injection

Ring buffer sizing: M_ring = R × S × E × T_hot (must be power of 2)
Shield uses 8 MiB, others use 256 KiB.

### Chaos Modes (Yaldabaoth)

- 0x01 BIT_FLIP: XOR random byte in Monad[1..17], CRC NOT recomputed (intentional corruption)
- 0x02 DELAY: Mark for netem delay, CRC recomputed
- 0x03 DUPLICATE: bpf_clone_redirect, CRC recomputed
- 0x04 TRUNCATE: Zero Monad[0x08..0x13], CRC NOT recomputed
- 0x05 CHAOS_MARKER: Set CHAOS flag only, CRC recomputed

### File Locations (Critical Paths)

```
PROTOCOL SPECS:
  docs/protocol/draft-bellis-unheaded-protocol-foundation-03.md
  docs/protocol/draft-bellis-unheaded-sophia-dictionary-00.md
  docs/protocol/draft-bellis-unheaded-wotan-memory-00.md
  docs/protocol/patches/*.md

PROTOCOL PACKAGES (Go):
  pkg/protocol/encoding/          — shared wire encoding
  pkg/protocol/registry/          — shared IANA-style registry
  pkg/protocol/bpfschema/         — BPF map struct definitions
  pkg/protocol/{errors,sequence,amplification,migration,...}/

BPF PROGRAMS (Rust):
  ebpf/monad-common/src/lib.rs    — shared Monad types
  ebpf/shield-ebpf/src/main.rs    — ingress/egress boundary
  ebpf/hop-ebpf/src/main.rs       — per-hop ALU
  ebpf/yaldabaoth-ebpf/src/main.rs — chaos injection
  ebpf/flow-tracker/src/main.rs   — bidirectional flow tracking
  ebpf/monad-cpu-ebpf/src/main.rs — Doom-over-IPv6

GO BPF LOADER:
  pkg/ebpf/loader.go              — custom eBPF loader (109KB, direct syscalls)
  pkg/ebpf/anamnesis.go           — event type definitions
  pkg/ebpf/anamnesis_reader.go    — ring buffer consumption

REFERENCE DOCS:
  pkg/protocol/PATTERN_MATRIX.md           — RFC → Package → BPF map matrix
  pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md — gap analysis
  docs/protocol/references/iana-guide.md   — IANA section templates
  docs/protocol/references/rfc-crossref.md — RFC citation matrix
  docs/protocol/references/wire-format-patterns.md — encoding patterns
  docs/protocol/references/rfcs/*.txt      — 19 raw RFC texts
  docs/security/dark-grimoire-addendum.md  — attack surface taxonomy
  docs/security/lich-campaigns.md          — fuzzing campaign specs

SKILLS:
  Load in order: architect → developer → rfceditor
  For security work: add blackmage
  For project status: add timeguru
  For full crew alignment: use round-table
```

### Dependencies Between Phases

```
Phase 1 (encoding) → Phase 2 (registry) → Phase 3 (bpfschema parity)
         ↓                    ↓                       ↓
Phase 5 (maploader) ←────────────────── Phase 4 (BPF map wiring)
         ↓
Phase 6 (draft patches) ← Independent, can run in parallel with 4-5
         ↓
Phase 7 (LICH fuzzing) ← Requires Phase 3+4 complete
         ↓
Phase 8 (verification)
```

Phases 1-3 are sequential (each builds on the previous).
Phases 4 and 6 can run in parallel after Phase 3.
Phase 5 requires Phase 4.
Phase 7 requires Phases 3+4.
Phase 8 is always last.

### SKILLS TO LOAD

| Skill | When | Why |
|-------|------|-----|
| unheaded-architect | Always | Infrastructure decisions, 4-pillar perspective |
| unheaded-developer | Always | TDD, defensive coding, Go/Rust patterns |
| unheaded-rfceditor | Phases 1-3, 6 | Wire format accuracy, RFC compliance |
| unheaded-blackmage | Phase 7 | Security fuzzing, attack surface |
| unheaded-timeguru | Phase 8 | Timeline updates |
| unheaded-micromanager | Phase 8 | QA sign-off |

### ANTI-PATTERNS — DO NOT

1. DO NOT modify monad-common/src/lib.rs without verifying bpfschema parity
2. DO NOT add BPF maps without corresponding bpfschema structs
3. DO NOT reimplement encoding — use pkg/protocol/encoding
4. DO NOT reimplement registries — use pkg/protocol/registry
5. DO NOT ignore BPF verifier limits (512B stack, bounded loops)
6. DO NOT skip CRC verification before processing Monad fields
7. DO NOT trust input to any function — validate EVERYTHING
8. DO NOT use .unwrap() or .expect() in Rust production eBPF code
9. DO NOT claim code is stubs without reading it (lesson from Feb 17)
10. DO NOT modify draft-03 — create draft-04 as a new file

### THE WAR CRY

465,000 lines. 25 services. 8 eBPF programs. 3 Internet-Drafts. 16 protocol packages.
19 RFC texts rescued. 12 gaps identified. 130 instructions queued.

The Kingdom stands. The protocol flows. The packets carry computation.

**SHIP IT.**

---

*S22 Session — RFC Editor × Developer × Architect Triforce*
*Partner: Muck (Stevie Bellis)*
*Agent: Claude Opus 4.6*
