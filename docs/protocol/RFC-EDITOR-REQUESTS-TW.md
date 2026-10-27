<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# RFC Editor requests — S-TRUE-WIRE (found during the S5 desk pass)

Opened 2026-10-27 by S-TRUE-WIRE Phase 1 (`docs/battle-plans/S-TRUE-WIRE-battle-plan.md`,
Steps 29-78). The S5 queue (`RFC-EDITOR-REQUESTS-S5.md`) is closed: all eight of its items
are ruled. Reading the drafts for those rulings turned up the conflicts below. They are
**not** ruled here, because each one either moves a value that code or a stored byte
already uses, or redefines a frozen Monad bit; both need Stevie (the E2 rule: spec and
code agree before anything merges to develop). Each has a recommendation.

Monad v0x01 is unchanged by every item here.

| # | Documents | Conflict | Code today | Recommendation | Status |
|---|---|---|---|---|---|
| TW-1 | foundation-00 §"PQC Algorithm Registry"; pqc-authentication-00 §17.1; sophia-dictionary-00 §"PQC Key Dictionary" table | "Unheaded PQC Algorithm Identifiers" is requested twice with incompatible values, and sophia carries a third table: foundation ML-KEM-768 0x01, ML-KEM-1024 0x02, ML-DSA-65 0x03, ML-DSA-87 0x04, SLH-DSA-SHA2-128s 0x05; pqc-auth SLH-DSA 0x01-0x0C, ML-DSA-44/65/87 0x10-0x12, FN-DSA 0x20-0x21, ML-KEM 0x80-0x82, HQC 0x90-0x91; sophia SLH-DSA 0x01, ML-DSA 0x02, FN-DSA 0x03, ML-KEM 0x04, HQC 0x05 | `pkg/crypto/pqc/registry.go`: `AlgoSLHDSA 0x01`, `AlgoMLDSA 0x02`, `AlgoFNDSA 0x03`, `AlgoMLKEM 0x04`, `AlgoHQC 0x05` (sophia's table) | One registry, owned by pqc-authentication (the document that specifies the algorithms). Decide family-level IDs (the code's, which are in Sophia maps today) or parameter-set IDs (pqc-auth's, which say which ML-DSA). If parameter sets win, the code moves and every stored `pqc_algo` byte in Sophia maps is re-keyed: that is a wire-value change for Stevie | open |
| TW-2 | pqc-authentication-00 §5, §17.2 vs foundation-00 §"Monad Flags Bitfield Registry" | pqc-auth "repurposes" flags bit 3 as S (Signed); foundation registers bit 3 as SAMPLED in the frozen v0x01 flags octet | `ebpf/monad-common` flags: bit 3 SAMPLED | A frozen bit cannot carry two meanings. Either pqc-auth signals "signed" another way (a Sophia policy entry keyed by service, or a flow action), or the flags registry gains a documented dual use with a disambiguation rule. Recommendation: the former; no bit changes meaning | open |
| TW-3 | pqc-authentication-00 §17.3 | "Sophia Map Type Registry Update" registers 0x10-0x14 in a registry no draft creates; 0x10-0x14 are also the Sophia Root Dictionary Keys routing..health | none found | Register the PQC maps as Sophia root keys from the FCFS range (next free after 0x16 `code`) or as Shim "UPC BPF Map Types"; drop the phantom registry | open |
| TW-4 | pqc-authentication-00.md | The `.md` twin is plain text with hand-numbered sections, not kramdown-rfc; only the `.xml` builds | n/a | Keep the `.xml` as the source (as for every draft now) or convert the `.md`; editorial | open |
| TW-5 | mbc-isa-00 | Divergences larger than a table fix (listed in the S5 file under "Known mbc-isa divergences left for -01"): branch offsets, DIV/MOD by zero, XCHG/CAS operands, the 256-instruction limit, Address Space Layout | engines as listed there | An mbc-isa `-01` pass that rewrites those sections from `crates/monad-mbc/src/execute.rs` and `ebpf/monad-cpu-ebpf`, the same rule as S5 item 5 (text follows the engines) | open |
| TW-6 | `docs/doom/UPC_BOOT_PROTOCOL_V2.md` (not a draft) vs mbc-isa-00 §"Control and Status Registers" | Boot protocol v2 shows the CSR region as `0xF000-0xF0FF` and the stage-1 stub at `0x10000-0x1FFFF`, overlapping CSR bytes `0x10000-0x12FFC` (indices `0x400-0xFFF`: counters at `0xC00`, `mhartid` at `0xF14`) | translator writes CSRs up to `0x12FFC` | Computermancer check of whether anything lives at `0x10000-0x12FFF` in RAM_MAP; then fix the boot-protocol doc | open |
| TW-7 | mbc-isa-00 §IANA | The opcode registry's policy is Standards Action, while every other registry in the suite is Specification Required or looser; on the Independent Stream no Standards Action is available, so the registry could never grow | n/a | Stevie's choice: keep (opcodes change only with a new revision of the ISA document) or loosen to Specification Required with expert instructions (both engines implement it, byte-identity gate passes) | open |

Fixed during the pass (no ruling needed, recorded for completeness): the `ietf-submission`
`.md` front matter of foundation, sophia and wotan named revisions `-06`/`-03` while the
`.xml` sources say `-00`; the `.md` now says `-00`. wotan-memory's ietf twin said the Error
Origin registry has 16 initial entries; it has 13 (`0x00-0x0C`) plus `0x1F`.
