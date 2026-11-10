<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# RFC desk pass, 2026-11-10 (S-E1-LIVE Steps 259-260)

Six expired `-00` Internet-Drafts were read against the tree, `docs/sweeps/TRUE-WIRE-DECISIONS.md` and `RFC-EDITOR-REQUESTS-WH.md`, and prepared as `-01` in `ietf-submission/`. Nothing was uploaded: Step 261 is Stevie's, and the submission tool is closed until after 2026-11-14 23:59 PST.

**The frozen Monad v0x01 format is unchanged by every `-01` file and by every recommendation below.**

## State

| Draft | On datatracker | `-01` here | Technical change since the public `-00` |
|---|---|---|---|
| protocol-foundation | `-00`, 2026-03-19, expired | built | one registry added, one withdrawn; no format change |
| sophia-dictionary | same | built | root keys 0x15, 0x16 |
| wotan-memory | same | built | topic registry, implementation note |
| mbc-isa | same | built | **three instructions read differently, ISA v2 added** |
| shim | same | built | none |
| pqc-authentication | same | built | implementation status paragraph |
| anamnesis, protocol-overview | never uploaded (404) | untouched, `-00` | not in the six |

`make -C ietf-submission validate`: Clean on all eight. Warnings (over-wide artwork, one long title) are the ones `-00` was accepted with, less one each.

## What the desk changed (Step 260)

- `-00` to `-01` by `git mv`, so history follows; the uploaded `-00` is at `4db0bbde` and on datatracker. `docName`, own `seriesInfo`, and every companion reference name `-01`.
- Front `<date/>`: xml2rfc stamps the build date, so the upload carries its own day.
- An appendix "Changes from draft-bellis-unheaded-X-00" in each, taken from `git diff 4db0bbde..HEAD` of the XML, not from memory.
- mbc-isa: the RFC 8174 clause added to Requirements Language (it was the only one without).
- pqc-authentication: an internal plan label removed from the implementation status paragraph.
- `Makefile` `DRAFTS`, the two Go tests that read the wotan `.md` twin, `docs/SUBMISSION_SUMMARY.md` (it said nothing had been submitted) and `IETF_SUBMISSION_GUIDE.md`.

The `.md` twins were renamed and carry the new names only. The `.xml` is the upload source.

## What Stevie should know before uploading

**mbc-isa `-01` contradicts the public `-00` on three instructions.** The tree is right and `-00` was wrong; the appendix says so plainly:

| Instruction | Public `-00` | `-01` (and the code) |
|---|---|---|
| opcode 0x00 | reserved | NOP |
| MOVI | sign-extends the 16-bit immediate | zero-extends |
| LOAD_IMM32 | a 20-bit form from the source field | `(imm << 16) \| (dst & 0xFFFF)` |

ISA v2 (ADR-067) adds FENCE 0x3F, MRET 0x47, SRET 0x48, LR.W 0x49, SC.W 0x4A, privilege levels and a CSR region; the opcode count goes from 48 to 56.

## Open, in the order they will be asked

Each needs a ruling before upload because it changes public text. None changes the wire.

### Round 1: DP-715's four (RFC-EDITOR-REQUESTS-WH items 1-4)

| # | Question | Recommendation |
|---|---|---|
| 1 | Where do the K1\|K0 tier bits live? Three places disagree. | No wire bits. The tier is a per-service Sophia policy value. pqc §10 and `KINGDOM_MODE.md` change. |
| 2 | 12-byte PQC value in a 6-octet window (4 usable). | v0x01 carries the 32-bit SigRef in scratch[0..4] only; KeyRef, HashPfx and SeqNum move into the Sophia signature entry. Both drafts change. |
| 3 | hop_count: limit, and drop or forward? | Limit 64, one policy key, default drop. hop-ebpf follows after the ruling. |
| 4 | Next Header 0xFE row. | Delete the row and the IANA request: 254 is an RFC 3692 experimental number and cannot be allocated. |

### Round 2: WH items 6-9

| # | Question | Recommendation |
|---|---|---|
| 6 | FN-DSA (DP-411: dropped). pqc names it 33 times; foundation line 1664 has `0x03 FN-DSA FIPS 206`; sophia names it twice. | Mark foundation 0x03 and pqc 0x20-0x21 Reserved; SOVEREIGN becomes two distinct families; `SigRef3` stays reserved. |
| 7 | Foundation's HMAC option (b), "not defined in this document". | Drop (b). ESP and ML-DSA at Shield boundaries remain. |
| 8 | Three tables give different algorithm ids. | Yours: name the one table; the other two and the code follow. The desk's pick is foundation's "PQC Algorithm Identifiers" for families, pqc's table for parameter sets, and foundation's "PQC Algorithm Registry" deleted. |
| 9 | Flow Label per flow or per packet? | Per flow, hashed from the 5-tuple (RFC 6437). A Shield change after the ruling. |

### Round 3: found by this pass

| # | Finding | Recommendation |
|---|---|---|
| 10 | **Stream against code points.** All six are `submissionType="independent"`, `category="exp"`. Foundation asks IANA for an HbH option type ("TBD (suggested: 0x3E)", line 1214) and sets "Standards Action" on its own registries (lines 804-809, 1230, 1306). An Independent Submission cannot obtain an HbH option type, and 0x3E is an RFC 4727 experimental value IANA will not assign (WH item 5, ADR-126). | Stay independent and experimental. Say the protocol uses 0x3E as an RFC 4727 experimental value inside a limited domain (RFC 8799), make no code point request, and set the document's own registries to Specification Required. Asking IANA for a real value (DP-412) then belongs to a later IETF-stream document, not to `-01`. This is the Track call Step 263 records. |
| 11 | Foundation cites `draft-bellis-unheaded-monad-extended-register-00` (lines 449, 2230). It does not exist on datatracker. | Remove the reference and say the Extended Register Option is not defined in this revision. |
| 12 | Private revision numbers in public text: foundation "draft-05 to draft-06" (1151-1195) and its "Changes from …-05" appendix; sophia and wotan "(NEW in draft-03)" in headings and "Changes from …-02"; wotan lines 1009-1032 send the reader to "draft-02 Section 8.2" and four more, a text nobody outside can read. `<workgroup>` is "Unheaded Kingdom" in pqc and shim, "Independent Submission" in the rest. | Remove the labels and the private change appendices, inline what the five wotan pointers point at (from `docs/protocol/draft-bellis-unheaded-wotan-memory-02.md`), and use one workgroup string. Editorial; larger than a desk should do unasked. |
| 13 | pqc cites "FIPS 206" for FN-DSA and "FIPS 207" for HQC (lines 37, 90-92, 153, 166, 411; foundation 1664, 1666). Neither standard is published. NIST IR 8545 (March 2025) says HQC will be standardized and gives it no number. | With item 6, FN-DSA leaves. For HQC keep the reserved row, cite NIST IR 8545, and drop the FIPS number. |

### Not asked, noted

- anamnesis and protocol-overview cite the six at `-00`. If they are ever uploaded, those references move to the then-current revision first.
- The remaining xml2rfc warnings are over-wide artwork (1 to 8 characters) and shim's 41-character title abbreviation. Reflowing a diagram risks changing it; left as `-00` had them.
- Code comments that cite `pqc-authentication-00` (`pkg/crypto/pqc/registry.go:16`, `ebpf/pqc-common/Cargo.toml:3`, `ebpf/monad-common/src/lib.rs:2674`, `pkg/monad/pqc_value_test.go:34`) stay until item 8 is ruled; item 8 rewrites that comment anyway.
- `docs/protocol/draft-*-0N.md` are the private-numbered working copies. They are dated records and were not touched; the wiki links to them still resolve.

## After the rulings

Claude applies them to the `-01` XML, rebuilds, and reports the diff. Stevie uploads each `.xml` at datatracker.ietf.org/submit and approves the confirmation mail (Step 261). Step 263 records URLs, the date and the Track call.
