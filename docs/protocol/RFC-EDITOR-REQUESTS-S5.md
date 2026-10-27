<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# RFC Editor requests — S5 (S-UPC-WIRE-COMPUTE)

Opened 2026-10-23 by the S5 sprint (`docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`,
Steps 72 and 243). Each item is a request for review. **None is self-approved.** Code
uses the provisional names below until an item is answered; ADR-108 stays Proposed until
items 1–3 are resolved (plan Step 447).

Monad v0x01 is unchanged by every item here, and no MBC opcode is added or renumbered.

| # | Document | Registry / section | Policy | Request | Provisional until answered | Status |
|---|---|---|---|---|---|---|
| 1 | `draft-bellis-unheaded-sophia-dictionary-00` | Unheaded Sophia Root Dictionary Keys | 0x10–0xFE First Come First Served | one root key for the MBC cartridge registry (ADR-108 D-C4) | namespace string `x-cartridge` | ruled 2026-10-27: `0x15` `cartridge` COMPOSITE (S-TRUE-WIRE Steps 30-31) |
| 2 | `draft-bellis-unheaded-protocol-foundation-00` | Unheaded Anamnesis Event Types | Specification Required | none: confirm that cartridge trace records reuse `EVENT_COMPUTED (0x01)` with a sub-kind in the payload (ADR-108 D-C5); if the editor rules a sub-kind needs its own type, that registration is [ESCALATE] | `EVENT_COMPUTED` + sub-kind `0x43` ('C') | ruled 2026-10-27: sub-kind `0x43` under EVENT_COMPUTED, new Sub-kinds registry (Step 37) |
| 3 | `draft-bellis-unheaded-wotan-memory-00` | Wotan Topic Namespace | Expert Review | topics `anomaly.scores` (cartridge scores to `services/anomaly`) and `upc.cart.*` (cartridge events, loads, faults) | the same names, documented as provisional | ruled 2026-10-27: both registered, registry made a request (Step 41) |
| 4 | `draft-bellis-unheaded-mbc-isa-00` + ADR-067 Decision 2 | CSR address map | n/a (text) | state the CSR byte-address range `0xF000 + csr*4` for the full 12-bit index (`0xF000–0x12FFC`) and the read/write sequences in the ADR-067 S5 implementation note | translator behaviour as shipped in S5 Phase 2 (`acf62722`) | open |
| 5 | `draft-bellis-unheaded-mbc-isa-00` §IANA (KF10) | MBC Instruction Set Architecture Opcode Values | Standards Action | 0x11–0x1B are listed RESERVED and also assigned (0x17/0x18/0x1A/0x1B); MOVI is documented sign-extended while both interpreters zero-extend `imm16` | implementations unchanged (zero-extend) | open |

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

### Item 1 — Sophia root key (draft, not filed)

```
Root Key:      0x15 (next free in the FCFS range; the editor assigns)
Category Name: cartridge
Type:          COMPOSITE
Specification: ADR-108 (MBC cartridges), Cartridge ABI v0
Entry schema:  name -> { sha256, manifest, seal_key_id, stage, surface_version,
                         max_insn, slot }
```

### Item 2 — Anamnesis sub-kind (draft, not filed)

```
EVENT_COMPUTED (0x01) payloads whose first octet is 0x43 ('C') are cartridge
step records: { slot, gen, flow_key, pc, insn, fields_in[], state_hash }.
Receivers that do not know the sub-kind MUST treat the record as an opaque
EVENT_COMPUTED. No new Event Type value is requested.
```

### Item 3 — Wotan topics (draft, not filed)

```
Topic Name      Component        Description                                   Reference
anomaly.scores  services/anomaly flow score {flow_key, score 0..100, model_id} ADR-108
upc.cart.*      cartridge        events, loads, faults per slot                ADR-108
```

Every payload on these topics carries flow metadata and a score only (ADR-108 FIELD
table); none can carry packet payload.
