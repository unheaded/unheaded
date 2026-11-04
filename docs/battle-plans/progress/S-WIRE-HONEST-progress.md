<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-WIRE-HONEST progress

## Log

- Phase 0 started from develop `f1cf9670` (the plan measured `ba87679b`). Worktree, symlink and payloads (21) ok; Steps 1-5 committed.
- Step 6: `go build` and `go vet` printed nothing. Race run: 240 ok, 32 no test files, 1 FAIL, none cached.
- Step 6 ran twice. The first run was discarded: `GOTMPDIR=$P/gotmp`, added by this agent and not in the plan, moved `t.TempDir()` under `$P`, which made the `cmd/unheaded-daemon` socket path 111 bytes (limit 107), so `TestLXDREST_UpdateProxyGivesUpAfterTwoConflicts` failed. It passes with no `GOTMPDIR` and with a short one, and fails again with the long one. The recorded baseline used `GOTMPDIR=/var/tmp/swh` and `-count=1`.
- Lanes: never point `GOTMPDIR` or `TMPDIR` at `$P`. Use `/var/tmp/swh`, or that test fails for a reason unrelated to the change.
- Step 8: `[PASS] 36 eBPF programs pass the verifier; no known failures`, run under the shared eBPF lock. It took 34 s, not the estimated 25 m; the ELFs were built during this run, in this worktree's `ebpf/target/load-gate`.
- Step 9: hop_xdp 153835, shield_xdp 1502, shield_tc 976 (default), kernel `6.17.0-41-generic`. All 36 rows match `docs/policy/ebpf-insns-baseline.tsv`; `check-ebpf-insns.sh --from $P/insns-pre.tsv` passes.
- Step 10: ADR-125 to ADR-134 are free; `id: FND-031` count 0.
- Phase 1 Step 12: `docs/protocol/RFC-EDITOR-REQUESTS-WH.md` landed and read by the RFC desk (`unheaded-rfceditor`) against both drafts and each cited line. Items 1-5, 7 and 9 hold as written. DP-715's four questions are items 1-4 (pack line 554).
- Step 12 corrections, all where the tree differs from the payload; the plan and the payload are unchanged:
  - Item 6, tree cell: "Code keeps 0x03 burned" is not yet true. Develop still names 0x03 (`AlgoFNDSA`, `AlgoId::FnDsa`, Shield's `allowedAlgos`) until Phase 4. The cell now says so. SOVEREIGN's `RequiredAlgos` already holds.
  - Item 6, tree cell: "registry row" is two rows, 0x20 and 0x21.
  - Item 6, recommendation: "Mark 0x03 Reserved in both drafts" was wrong for PQC-00, where 0x03 is SLH-DSA-SHA2-192s and FN-DSA is 0x20-0x21. Taken literally it would withdraw a live SLH-DSA set. Now 0x03 in foundation-06, 0x20-0x21 in PQC-00. This is a recommendation cell, not a tree cell; changed because it rests on a false reading of the draft. See "For Stevie".
  - Item 8, tree cell: "Code has PQC-00's families" was wrong. PQC-00 has no family table, and `AlgoID` 0x01-0x05 matches foundation-06's identifiers. `registry.go`'s comment credits PQC-00, which is the likely source.
- Step 12 reading notes: item 7's "no HMAC helper" checked on this kernel: `bpf_crypto_*` kfuncs exist on 6.17, skcipher only, nothing for HMAC or hashing. Item 8's "no value is stored" holds for numeric ids: `PQCMapManager` keeps entries in memory, no SQL mentions them, and `wotansig` and `sophiasync` store the string `ml-dsa-65`. RFC 3692 and RFC 4727 values in items 4 and 5 are from the desk's knowledge of those RFCs; no copy is on west.
- Step 13: spec compliance gate recorded below. Every row reads "No" in the Monad column; no lane is closed.
- Step 14: wire source diff against the merge base with develop: `0`.

## Spec compliance gate (Step 13)

| Change | Wire version | Change type | Touches a Monad byte? | Backward compatible | IANA impact | RFC desk review |
|---|---|---|---|---|---|---|
| DP-716 hop checks | v0x01 frozen | none on the wire; reads existing fields, new private map semantics | No (read only; no field is written that was not before) | Yes: mode 0 default | None | Pass. Step 88 and ADR-125 run the checks before the Monad clone and write no wire field; enforce still emits ANOMALY, as foundation-06's hop_count text requires. Phase 12 deletes lookups whose results were discarded; none wrote the Monad. Mode 0 is proven at Phase 11, cases 1 and 12 |
| DP-408 latency key | v0x01 | none: a kprobe map, never on the wire | No | Reader and object ship together | None | Pass. `ebpf/latency-probe/src` does not reference the Monad |
| DP-211 bpf mode | v0x01 | none: map reads | No | Yes | None | Pass. `BPFBackend` also keeps its existing `WriteMap` and `DeleteMap`, so "reads" is not the whole of it; both touch BPF map entries, not packets |
| DP-411 FN-DSA drop | v0x01 | code only; 0x03 burned | No | Yes: nothing ever produced 0x03 | Registry text in both drafts (foundation-06 "PQC Algorithm Identifiers", Specification Required, and PQC-00) queued, item 6. No draft edited here | Pass. No non-test code sets an algo id of 0x03 and the signer stub cannot sign; the only non-test uses accept or classify it. No algo id is on the wire |
| DP-411 SHAKE sets | v0x01 | new code at PQC-00's own 0x07-0x0C | No | Yes: additive | None; which table owns the ids is item 8 | Pass. 0x07-0x0C are PQC-00's SHAKE rows, and no other table or SLH-DSA constant uses them |
| DP-412 registry | v0x01 | ADR only | No | Yes | Queued, item 5 | Pass. ADR-126 changes no code; `hbh.rs` keeps 0x3E until Stevie accepts it |

Result: all six "No". No lane closed.

## Known failures

Later gates fail only on lines not in this list.

```
unheaded/pkg/secrets/store
```

`pkg/secrets/store` is intermittent, not constant: it passed in the discarded run, failed 1 of 12 separate runs afterwards, and one `-count=20` run panicked. A pass at Step 130 is not a change, and neither is a fail. See "For Stevie".

running units: 3

## For Stevie

1. `pkg/secrets/store` has a real crash, not only a load flake (S-TRUE-WIRE's progress file called it that). `notifyWatchers` (`file.go:259-272`) copies the watcher slice under `RLock`, releases it, then sends; `Close` (`file.go:356`) and `removeWatcher` (`file.go:282`) can close those channels in that gap. Stack from the baseline run: `panic: send on closed channel` at `file.go:266`, from `checkForChanges` in the `watchFiles` goroutine, so a running process dies, not just a test. Not fixed here: outside the six DPs. Do you want it fixed, and in which plan?
2. RFC item 6's recommendation changed at the desk: mark 0x03 Reserved in foundation-06 and 0x20-0x21 Reserved in PQC-00, not 0x03 in both, because PQC-00's 0x03 is SLH-DSA-SHA2-192s. The plan's Appendix F payload still has the old text. Rule on it at the resubmission sitting.
3. For the same sitting: foundation-06 and PQC-00 §17 both ask IANA for a registry named "Unheaded PQC Algorithm Identifiers", with different contents; that sharpens item 8. PQC-00 also names FN-DSA in prose the queue does not list (opening summary, §1, §3, §15.2, §18.11, §18.14), so item 6's rewrite is wider than its tables and §10.
4. ADR-125's `max_hop_count` passes at or under the value; foundation-06's limit fails at or over it. So cap N means limit N+1. No wire effect and nothing to fix before item 3 is ruled, but that ruling should settle one meaning.
