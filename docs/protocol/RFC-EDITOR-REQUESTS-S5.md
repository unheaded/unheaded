<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# RFC Editor requests — S5 (S-UPC-WIRE-COMPUTE)

Opened 2026-10-23 by the S5 sprint (`docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`,
Steps 72 and 243). Each item is a request for review. **None is self-approved.** Code
uses the provisional names below until an item is answered; ADR-108 stays Proposed until
items 1–3 are resolved (plan Step 447).

**All items ruled 2026-10-27** by the in-repo RFC Editor pass of S-TRUE-WIRE Phase 1
(`docs/battle-plans/S-TRUE-WIRE-battle-plan.md` Steps 29-58). Items 6, 7 and 8 were
found during that pass and ruled in it. The ruled text lives in the drafts themselves; see
"Status at S-TRUE-WIRE" below.

Monad v0x01 is unchanged by every item here, and no MBC opcode is added or renumbered.

| # | Document | Registry / section | Policy | Request | Provisional until answered | Status |
|---|---|---|---|---|---|---|
| 1 | `draft-bellis-unheaded-sophia-dictionary-00` | Unheaded Sophia Root Dictionary Keys | 0x10–0xFE First Come First Served | one root key for the MBC cartridge registry (ADR-108 D-C4) | namespace string `x-cartridge` | ruled 2026-10-27: `0x15` `cartridge` COMPOSITE (S-TRUE-WIRE Steps 30-31) |
| 2 | `draft-bellis-unheaded-protocol-foundation-00` | Unheaded Anamnesis Event Types | Specification Required | none: confirm that cartridge trace records reuse `EVENT_COMPUTED (0x01)` with a sub-kind in the payload (ADR-108 D-C5); if the editor rules a sub-kind needs its own type, that registration is [ESCALATE] | `EVENT_COMPUTED` + sub-kind `0x43` ('C') | ruled 2026-10-27: sub-kind `0x43` under EVENT_COMPUTED, new Sub-kinds registry (Step 37) |
| 3 | `draft-bellis-unheaded-wotan-memory-00` | Wotan Topic Namespace | Expert Review | topics `anomaly.scores` (cartridge scores to `services/anomaly`) and `upc.cart.*` (cartridge events, loads, faults) | the same names, documented as provisional | ruled 2026-10-27: both registered, registry made a request (Step 41) |
| 4 | `draft-bellis-unheaded-mbc-isa-00` + ADR-067 Decision 2 | CSR address map | n/a (text) | state the CSR byte-address range `0xF000 + csr*4` for the full 12-bit index (`0xF000–0x12FFC`) and the read/write sequences in the ADR-067 S5 implementation note | translator behaviour as shipped in S5 Phase 2 (`acf62722`) | ruled 2026-10-27: text added, §"Control and Status Registers" (Step 45) |
| 5 | `draft-bellis-unheaded-mbc-isa-00` §IANA (KF10) | MBC Instruction Set Architecture Opcode Values | Standards Action | 0x11–0x1B are listed RESERVED and also assigned (0x17/0x18/0x1A/0x1B); MOVI is documented sign-extended while both interpreters zero-extend `imm16` | implementations unchanged (zero-extend) | ruled 2026-10-27: table follows the engines; MOVI zero-extends (Step 49) |
| 6 | `draft-bellis-unheaded-sophia-dictionary-00` | Unheaded Sophia Root Dictionary Keys | First Come First Served | found during item 1: the UPC opcode dictionary section put category `code` at root key `0x10`, which the initial entries already assign to `routing` (2026-03-05; `code` arrived 2026-03-15) | none (no code implements the opcode dictionary) | ruled 2026-10-27: `code` moves to `0x16` (BRANCH); `0x10` stays `routing` (Step 31) |
| 7 | `draft-bellis-unheaded-mbc-isa-00` | Instruction Set, §IANA | Standards Action (table); n/a (text) | found during item 5: the draft omitted NOP (0x00, decoded by every engine) and the five ISA v2 opcodes (0x3F, 0x47-0x4A, ADR-067); MOV/MOVI/LOAD_IMM32 were documented as setting Z/N, and LOAD_IMM32 as `(src<<16)\|imm`, while every engine leaves flags alone and computes `(imm<<16)\|(dst&0xFFFF)` | implementations unchanged | ruled 2026-10-27: table lists NOP and ISA v2; new §"ISA v2 Instructions"; MOV/MOVI/LOAD_IMM32 text follows the engines (Step 49). Further divergences listed for `-01` below |
| 8 | `draft-bellis-unheaded-protocol-foundation-00` + `-mbc-isa-00` | MBC opcode registries | Specification Required (foundation) vs Standards Action (mbc-isa) | found during item 5: two registries for the same 8-bit opcode space under two names and two policies; foundation's copy lacked 0xFE and the ISA v2 opcodes | none | ruled 2026-10-27: mbc-isa owns the one registry at its stated policy (Standards Action); foundation's request is withdrawn and points there (Step 49). Loosening the policy to Specification Required, like the rest of the suite, is Stevie's call |

## Status at S-TRUE-WIRE (2026-10-27, Steps 29-58)

All eight items are ruled by the in-repo RFC Editor pass (S-TRUE-WIRE Phase 1, the
`rfc-desk` lane). No Monad wire byte changed, the 20-byte register layout is untouched,
and no MBC opcode was added or renumbered: every ruling either registers a value the
code already uses or makes a draft describe what the engines already do.

| # | Ruling | Where the text is | Code consequence (Phase 2, not done here) |
|---|---|---|---|
| 1 | Root key `0x15`, category `cartridge`, type COMPOSITE, entry schema `name -> {sha256, manifest, seal_key_id, stage, surface_version, max_insn, slot}`; a loader verifies the seal over the exact bytes; an entry locates, never authorizes; `x-cartridge` MAY be read during migration, MUST NOT be written | sophia-dictionary-03 / -00 (md + xml) §IANA "Cartridge and Opcode Dictionary Root Keys" | `cmd/upc-cartridgectl/registry.go:31` `Namespace = "x-cartridge"` → `"cartridge"`, plus `CartridgeRootKey uint8 = 0x15`, reading `x-cartridge/<name>` as an alias |
| 2 | Cartridge records keep `EVENT_COMPUTED (0x01)` with sub-kind `0x43` CARTRIDGE_TRACE (octet 2 = record kind 0/1/2). New registry "Unheaded Anamnesis EVENT_COMPUTED Sub-kinds" (Specification Required). Sub-kinded records live on their own stream; a receiver MUST NOT read a Sub-kind out of the 32-octet event record (whose octet after the type is the hop id) | foundation-06 / -00 §"EVENT_COMPUTED Sub-kinds" under the Anamnesis Event Type Registry; anamnesis-00 §Sub-kinds | none (`monad_common::cart::trace::SUBKIND = 0x43` already) |
| 3 | `anomaly.scores` and `upc.cart.*` registered (Expert Review), with `compute.tty.*` (defined by wotan-memory itself); the registry is now requested, not "may be created"; the expert refuses payloads that can carry packet payload | wotan-memory-03 / -00 §"Wotan Topic Namespace Registry"; `WOTAN_TOPICS.md` | none (`services/anomaly.ScoreTopic` already `anomaly.scores`); `upc.cart.*` has no producer yet |
| 4 | CSR `i` at byte `0xF000 + i*4`, region `0xF000-0x12FFC`; read `LD`, write `ST`, RMW = LD/op/ST; one transient register, restored; `csrr` never writes; unsupported forms fail at translation | mbc-isa-00 (docs + ietf md + xml) §"Control and Status Registers"; ADR-067 Decision 2 pointer | none |
| 5 | Opcode table lists the actual unassigned spans (0x11-0x16, 0x19, 0x1E-0x1F, 0x2B-0x2F, 0x41-0x46, 0x4B-0xFD), 0xFE Reserved; MOVI zero-extends `imm16` | mbc-isa-00 §IANA + §MOVI; `mbc-isa-reference.md` opcode map | none (all three engines zero-extend) |
| 6 | `code` (opcode dictionary) moves `0x10` → `0x16` | sophia-dictionary §"Opcode Dictionary Structure" + §IANA | none |
| 7 | NOP and ISA v2 in the table and a new §"ISA v2 Instructions"; MOV/MOVI/LOAD_IMM32 flags and LOAD_IMM32 semantics follow the engines | mbc-isa-00 | none |
| 8 | One MBC opcode registry, in mbc-isa (Standards Action); foundation's "UPC MBC Opcode Numbers" request withdrawn | foundation-06 / -00 §"MBC Opcode Numbers" | none |

**Remaining action:** E1, Stevie accepts ADR-108 (or keeps it Proposed until the
cartridges run on east, S-ASCEND-2 M-228). E6 (datatracker upload) is separate.

**Known mbc-isa divergences left for `-01`** (found in the item 5/7 pass, not ruled here
because each is a larger rewrite than a table fix): branch offsets (the engines use a
24-bit signed offset across dst/src/imm, the draft a 16-bit one with flags in src); DIV/MOD
by zero (engines: DIV gives `0xFFFFFFFF` with C=1, MOD gives 0; draft: trap); XCHG/CAS
operand layout (engines: CAS uses r0/r1/r2); the "256-instruction limit" and the
"Address Space Layout" section, which predate the UPC memory map; `crates/monad-mbc`
`is_valid_opcode` omits INT, IRET and MULHU although the BPF engine runs them; the
non-`ascend-linux` BPF build executes the ISA v2 opcodes as no-ops (the draft now says an
engine without ISA v2 SHOULD reject them at load time).

**Overlap to check (not a ruling):** `docs/doom/UPC_BOOT_PROTOCOL_V2.md` still shows the
CSR region as `0xF000-0xF0FF` and puts the stage-1 stub at `0x10000-0x1FFFF`, which
overlaps CSR bytes `0x10000-0x12FFC` (CSR indices `0x400-0xFFF`, which include the
counters at `0xC00` and `mhartid` at `0xF14`).

## Status at sprint close (2026-10-23, S5 Step 447)

All five items are still **open**: no RFC Editor review has been received, and none is
self-approved. ADR-108 therefore stays **Proposed** and the code keeps the provisional
names (`x-cartridge`, sub-kind `0x43`, `anomaly.scores`, `upc.cart.*`). The final spec
gate found no Monad wire byte changed and no MBC opcode added or renumbered across the
sprint (against the sprint base `f7d730e7`, `ebpf/monad-common/src/lib.rs` adds only
`pub mod cart`; `ebpf/monad-cpu-ebpf` is unchanged). If item 2 is ruled to need its own
Event Type, that registration (Specification Required) is [ESCALATE]; DOT4 (ADR-108
non-goals) would be a new item under the MBC Opcode Registry (Standards Action) and is
not filed.

## Draft text for the reviewer

### Item 1 — Sophia root key (as requested; ruled text is in the draft)

```
Root Key:      0x15 (next free in the FCFS range; the editor assigns)
Category Name: cartridge
Type:          COMPOSITE
Specification: ADR-108 (MBC cartridges), Cartridge ABI v0
Entry schema:  name -> { sha256, manifest, seal_key_id, stage, surface_version,
                         max_insn, slot }
```

### Item 2 — Anamnesis sub-kind (as requested; ruled text is in the draft)

```
EVENT_COMPUTED (0x01) payloads whose first octet is 0x43 ('C') are cartridge
step records: { slot, gen, flow_key, pc, insn, fields_in[], state_hash }.
Receivers that do not know the sub-kind MUST treat the record as an opaque
EVENT_COMPUTED. No new Event Type value is requested.
```

### Item 3 — Wotan topics (as requested; ruled text is in the draft)

```
Topic Name      Component        Description                                   Reference
anomaly.scores  services/anomaly flow score {flow_key, score 0..100, model_id} ADR-108
upc.cart.*      cartridge        events, loads, faults per slot                ADR-108
```

Every payload on these topics carries flow metadata and a score only (ADR-108 FIELD
table); none can carry packet payload.
