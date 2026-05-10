# ADR-072 — BOOT_MAGIC Byte-Ordering Convention

**Status:** Accepted
**Date:** 2026-05-10
**Deciders:** unheaded-developer (impl invariant lens) + unheaded-micromanager (QA gate + DoD lens) + unheaded-busboy (cross-doc + crew translation)
**Aligns with:** ADR-067 (MBC ISA v2 + UPC ABI v1), `docs/doom/UPC_BOOT_PROTOCOL.md`, `docs/doom/UPC_BOOT_PROTOCOL_V2.md`

---

## Context

The UPC Boot Protocol's `BootParamsV2.magic` field is the canonical 32-bit constant `0x554E4844`. The spec text and the Rust constant agree on this value and ship to production.

Two coherent ways exist to talk about that value:

| View | Interpretation | Where it shows up |
|------|----------------|-------------------|
| MSB-first hex | `0x554E4844` reads as `'U','N','H','D'` (the brand spelling) | Spec text, equality comparisons, conversation, debugger u32 view |
| Little-endian wire | The 4 bytes at increasing memory addresses are `0x44, 0x48, 0x4E, 0x55` = `'D','H','N','U'` | `BOOT_MAGIC.to_le_bytes()`, hex-dump of the BootParams region, byte-iterating consumers |

Both views describe the same constant. Pre-resolution, the spec docs (`docs/doom/UPC_BOOT_PROTOCOL.md:73` and `docs/doom/UPC_BOOT_PROTOCOL_V2.md:113,140`) said only `'UNHD' = 0x554E4844`; the Rust docstring at `crates/xv6-mbc/src/lib.rs:38-50` flagged the apparent inconsistency as "Surfaced for daytime resolution"; the test `boot_magic_le_bytes_visual_is_dhnu` pinned the wire bytes as `DHNU` with a comment naming this as an open spec-clarity question. Future readers were primed to re-open the question.

Marshal hand-off 2026-05-09 listed this as task #64. Three options surfaced:

1. Markdown-only — amend spec docs to make both views explicit. No code change.
2. Byte-swap — change the constant to `0x44484E55` so the LE bytes read `UNHD`. Coordinated edit across 3 source files + 2 tests.
3. Skip — leave the open-question marker.

## Decision

**Option 1.** The constant value `0x554E4844` is the canonical form. The wire/memory representation has bytes `D, H, N, U` at increasing addresses (the standard consequence of little-endian serialization). Both views are correct; spec text MUST make this explicit.

This is the same pattern as ELF's `\x7fELF` magic, which is `0x464C457F` as a host-endian u32 on LE systems. Nobody calls ELF "broken" because the hex-form spelling and the byte-order spelling differ; they're just different ways to view the same 4 bytes.

## Rationale

- **Zero behavioural change.** Wire format unchanged, no out-of-tree consumer (cached BootParams images on EAST/WEST hosts, third-party UPC implementations) gets invalidated.
- **Zero regression risk.** Existing tests pass without modification; only their docstrings flip from "open question" to "intentional pin per ADR-072".
- **Brand intent preserved.** `0x554E4844` reads as `UNHD` in the form humans have always used to write 32-bit constants. The brand-name link is intact.
- **Wire correctness preserved.** A debugger byte-dump or a `to_le_bytes()` consumer sees `DHNU`, which is what the wire actually carries. Operators inspecting BootParams in memory see what's actually there.
- **Precedent.** Identical to ELF, ARM's `ARM\0`, and countless other magic-number conventions where the hex-form spelling and byte-order spelling differ.

## Affected surfaces (all updated in lockstep with this ADR)

1. `docs/doom/UPC_BOOT_PROTOCOL.md` §"Magic Value" — adds the explicit byte-table + ELF parallel.
2. `docs/doom/UPC_BOOT_PROTOCOL_V2.md` line 113 (struct comment) and line 140 (field-semantics text) — adds canonical/wire framing inline.
3. `crates/xv6-mbc/src/lib.rs:38-50` — docstring rewritten from "daytime resolution" open question to canonical/wire-form explanation citing this ADR.
4. `crates/xv6-mbc/src/lib.rs:65` — test docstring updated to "Intentional pin per ADR-072".
5. `cmd/upc-bootctl/src/bootparams.rs:118` — test docstring updated to "Intentional pin per ADR-072".
6. `cmd/upc-bootctl/src/main.rs:111-115, 150` — operator-facing print strings updated to show both canonical and wire-byte forms.

## Consequences

- Task #64 closes. The "daytime resolution" open marker is gone from the codebase.
- Future readers hitting any of the 6 surfaces above land on the resolved framing, not the open question.
- If/when UPC Boot Protocol becomes a published API for third-party implementations, the spec text is unambiguous about both views.
- If a future amendment ever DOES need to byte-swap the constant (e.g. to align with a different bootloader convention), it requires a new ADR amending this one — preventing silent drift.

## Out of scope

- IETF RFC drafts (`ietf-submission/draft-bellis-unheaded-protocol-foundation-00.md` etc.) define different magics (`UNFS` 0x554E4653, `UPCF` 0x55504346) for different purposes; this ADR covers ONLY the BootParams `magic` field used by `crates/xv6-mbc/` and `cmd/upc-bootctl/`.
- Any future v3+ BootParams revision that changes struct layout — that's its own ADR.
