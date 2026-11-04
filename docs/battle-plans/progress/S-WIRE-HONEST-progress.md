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

- Phase 2 Steps 16-19: ADR-125 and ADR-126 copied from the payloads; `index_rows_wire.py` printed `index rows added after: | [120](ADR-120-estimated-ener` (rc 0). Step 18: `2` and `2`. Committed `787a471f` (signed). Phase 2 exit gate: both ADRs on `sprint/s-wire-honest`; lanes not started here.

- Phase 16 started on `2c524783`; all six lanes are merged here. No sudo: Step 125 is [R], so nothing in this read was run against the object. eBPF claims below come from reading `ebpf/hop-ebpf/src/main.rs`; Go claims from a throwaway test, removed afterwards, except where a kept test is named.
- Step 125 read, by the plan's four questions:
  - Enforce as a DoS lever: real, not remote. Shield leaves both service ids at 0 and stamps ring 1 (`deploy_ring::PRODUCTION`), so every Shield-born packet is flow 0, namespace 1, not namespace 0. In mode 2, a `require_hmac` or `require_retry` validator there drops them all once flow 0's entry is missing or expired. Writing it takes root on the pins and CONFIG 4 by hand, and a packet can only get itself dropped. The live risk is a `ttl` lapsing: nothing renews an entry. FND-035, low.
  - Key bytes in errors: one real leak, fixed. A base64 key pasted over `key_file` can start with `/`, pass `validate` as absolute, and print whole in Lstat's error. FND-034, low, closed by `3e3e546e`: red, then green under `-race`. `yaml.v3` still quotes up to seven characters of a value in a wrongly typed field; kept, because that error carries the line number. Unknown-field errors give the name only. `TestKeyBytesAreInNoDump` holds; `pkg/ebpf` errors carry sizes and errno only.
  - Symlinked `key_file`: holds. Final-component symlink refused (existing test), and `SameFile` catches a swap between Lstat and open. A symlinked parent directory and a hard link are both accepted, and the file's owner is not checked; see "For Stevie". No finding: the bytes go to a map only root can read, and nothing uses them.
  - Pin ownership: holds on west. `/sys/fs/bpf` is 1700 root and `kernel.unprivileged_bpf_disabled` is 2, so this user can list nothing there. The pins' own mode was not read, for the same reason. `hop rules apply` checks neither a pin's owner nor its map name, only its type and sizes. `protocol-api --mode=bpf` cannot reach these maps: it reads two fixed names, `hop-ebpf/SOPHIA` and `flow-tracker/FLOWS`, and no non-test code calls `WriteMap`.
- Step 125, found beyond the four questions: enforce can be skipped from the wire. `hop_xdp` returns `XDP_PASS` before `run_checks` for a bad CRC, for CHAOS and at the hop limit, and a `deploy_ring` with no validator runs no check. FND-033, low: mode 0 by default, and nothing loads hop-ebpf in production. `tests/hopchecks` has no enforce case for any of the four, and no frame shaped like a Shield-born one (flow 0, ring 1).
- Step 125 also corrected `pkg/hoprules`' doc comment: its example used `namespace: 0`, which matches no Shield-born packet, so such a rule silently never runs. It now says 1 and states the lever.
- Step 125 [STUCK], the fix half of FND-033 and FND-035 only. Both have register rows. FND-033 needs ADR-125's order changed, a rule for an unmatched ring, a root test run and a new `hop_xdp` count; none of that is a named step here, and the order is yours to accept. FND-035 is a design choice between two answers. Items 5 and 6 below. The rest of the step is done.
- Register: FND-033 and FND-035 open, FND-034 closed with evidence. Ids checked free on every sprint branch and in the battle plans. ADR-098 regenerated with the evidence file: `155 -> 157`, 133 low, no high added. The generator lists FND-034 under "remove from the register", as it does for any closed entry.
- Step 126: `0`, `bash -n` clean. The script was not run.
- Gates on this tree: `gofmt -l` empty, `go vet` clean, `go test -race` ok on `pkg/hoprules`, `cmd/unheaded-cli/...`, `tests/hopchecks` (skips unprivileged), `go test` ok on `cmd/compliance-evidence`, `pkg/compliance/...`; darwin and windows builds of `unheaded-cli` clean; `check-gosec-ratchet.sh` PASS.
- Phase 16 exit gate: passed. This read leaves no open high finding: its three findings are low, and two stay open.

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
5. FND-033, your ruling: should enforce also cover a bad CRC, CHAOS and the hop limit, and what should a `deploy_ring` with no validator do? Today all four pass unchecked. Moving `run_checks` up changes ADR-125's order and `hop_xdp`'s count, and a default for an unmatched ring is a new rule. While FND-031 stands a forger can name a live pair anyway, so this matters most once a tag exists.
6. FND-035, your choice: resolve service ids in Shield, or have `hop rules apply` refuse or warn on a validator with no live flow-0 entry. Until then, avoid a `ttl` on flow 0 of an enforcing hop.
7. `hop rules apply` trusts `--pin-dir`: `OpenPinnedMap` checks map type and sizes, not the pin's owner or its map name, so a stale or wrong pin of the same sizes takes the entry, key bytes included. Only someone already root can plant one. Lane hop saw that SIGTERM ends the loader before its cleanup, so stale pins look possible, and apply would then report success into a map nothing reads. I read that from the code and did not run it. A name check against `bpf_map_info` is small if you want it.
8. `HMAC_KEYS` holds 32 key bytes that hop-ebpf never reads, and the lab read them back with `bpftool`. No finding, since only root reaches them, but they could stay out of the map until a tag exists.
9. If a tag ever makes the key matter, `readKey` should also check the owner: today a 0600 file of any uid, a symlinked parent and a hard link are all accepted. Two smaller notes for the same day: the rules file's own mode is unchecked, and failed checks emit ANOMALY with no rate limit, as the bad-CRC path already did.
10. `man/pqc-verifier.1` is still in this tree (hop lane item 7), so Step 131's `gen-man` fails until it is deleted. Not this phase's file.
