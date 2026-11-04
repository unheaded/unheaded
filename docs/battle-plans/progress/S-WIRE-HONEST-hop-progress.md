<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-WIRE-HONEST lane hop progress

## Log

- Phase 11 started: lane created with `lane.sh hop` from `8d84529a`.
- Step 84: `Finished` twice.
- Steps 85-87 RED: `tests/hopchecks` built; failure from case 1, `object has no HOP_VALIDATORS map to populate`. Case 2 passes on the old object, as it should (no validator, nothing to count).
- Step 85, added to the table's fourteen, table unchanged: `check` also asserts the ANAMNESIS event types (ADR-125: a failed check emits ANOMALY) and, on PASS, that the frame out differs from the frame in only by hop count and CRC, which proves no check wrote a wire field. `TestHopCheckEdges` holds eleven cases the table cannot catch: case 14 passes with a hard-coded namespace, every row runs at hop 0, and no row sits on a boundary. Validators and HMAC entries are encoded from the `bpfschema` structs, so the object and the Go mirror must agree.
- Step 88 choices the plan left open:
  - `RetryEntry` is 48 bytes, the map's size: `token[32]`, `issued_at`, `expires_at`, 8 reserved. Shield's comment added to 56 with `new_addr[16]`; that field is gone. Phase 13 mirrors this.
  - `src_low48` in the RETRY_TOKENS key is a number: the last six address bytes read big-endian, so `fd00::1` is 1. The key's bytes on the map are therefore those six in reverse. `pkg/hoprules` must build it as `flow_id | src_low48 << 16` into a little-endian u64.
  - CONFIG[4] values other than 1 and 2 are off, not observe. `require_crc` and `require_hmac` count any non-zero byte as set.
  - An entry is expired from the second `expires_at` names: live means `now_s < expires_at`.
- Step 90: `hop_xdp_insns_post` = 171672 (`hop_xdp OK verified 171672 insns`, kernel 6.17.0-41-generic).
- Step 91 budget gate: under 900,000; +17,837, +11.59 % over 153,835, inside the 25 % line (192,293). Step 93 not needed. It is over the ratchet's 169,218, so `check-ebpf-insns.sh` fails on this count until Step 119 patches the row; no self-approval here.
- Steps 93, 94: not needed. The first build loaded, and the count sits inside the 25 % line.
- Steps 95-96 GREEN: all fourteen PASS on the first run, and all eleven `TestHopCheckEdges`, run under the shared eBPF lock.
- Because the first run was green, eight mutants of `run_checks` were built outside the tree and run against the test binary. All eight were killed: expiry ignored, namespace hard-coded in the validator key, namespace hard-coded in the HMAC key, cap off by one, observe drops, service pair swapped, retry source truncated, ANOMALY removed. `main.rs` was restored each time; `git status` clean afterwards.
- Other gates on this tree: `gofmt -l` empty, `go vet` clean, `go test -race ./tests/hopchecks/` ok (skips unprivileged), `go test ./pkg/protocol/bpfschema/` ok with both Shield and hop declaring the three maps, `check-gosec-ratchet.sh` PASS, `rustfmt --check` clean.
- Phase 11 exit gate: fourteen PASS; `hop_xdp_insns_post` 171672 recorded; mode 0 is cases 1 and 12, both PASS with 0/0/0.
- Phase 12 started on `e47f1a86`.
- Step 98: `HMAC_KEYS`, `RETRY_TOKENS`, `HOP_VALIDATORS`, `AUTHORITY`, `make_hmac_key`, `make_retry_token_key`, `make_authority_key`, the 4b-4d block and the authority block deleted from `ebpf/shield-ebpf/src/main.rs`. Step 4's heading now says "Pass". `rustfmt --check` clean.
- Step 100: `[PASS] 36 eBPF programs pass the verifier; no known failures`, under the shared lock.
- Step 101 [STUCK], count half only: `shield_xdp` 1502 and 1502 (`debug-stats`), at the bound. `shield_tc` 980 against 976, and 1088 against 1,084 for `debug-stats`: +4 each, over the plan's bound. The `git grep` half passes: `0`. Both objects reloaded by hand, same numbers, so it is not noise. `check-ebpf-insns.sh --from` does not flag it (+0.4 %, inside the 10 % line); its only failure on this tree is `hop_xdp`, as expected until Step 119. No baseline touched, no code shaped to reach the number.
- Step 101, why, measured with a scratch verifier log (`$P/hop-p12`, nothing in the tree): the object does lose the lookup, seven instructions on two paths, and stack depth stays 176. But `total_states` goes 67 to 69, and the verifier now walks the error-counter block at 299-311 four times, not two: on the CRC-failed paths it used to stop at `299: safe`, and now it stops at `312: safe`. LLVM also moved the u32 stat key from `fp-4` to `fp-24` once the slot was free. That the slot move is why pruning misses at 299 is my reading, not proven; the extra walks are measured. This log is of the default object; `debug-stats` moved by the same 4 but was not traced.
- Step 99 [BLOCKED], not done: lane `lat` has not merged. Its last commit is `a44671c0`, and `sprint/s-wire-honest` was still at `9f6fc2ad` when I stopped, after about 30 minutes' wait against a 5-minute step. The step says to wait, and that merge is not this phase's to run. So `bpfschema.go` and both test files are untouched here.
- Until Step 99 runs, this branch has one known red test: `go test ./pkg/protocol/bpfschema/` fails `TestRustSourceMapNames` with `MapAuthority = "AUTHORITY" names no map declared under ebpf/*/src (dead name)`. Step 99 removes that line, and it must run before Phase 13's Step 104, which expects `ok`. No code reads these maps from a Shield object: outside bpfschema, only `tests/hopchecks` names them, and it loads hop-ebpf. `compliance/findings/register.yaml` and `pkg/protocol/PATTERN_MATRIX.md` still describe them; the register is Phase 15's.
- Step 102: committed with Step 98's change and this log.
- Phase 12 exit gate: load gate `[PASS]` on this tree. Steps 99 and 101 stay open as above.

## For Stevie

1. `hop_xdp` growth, Step 91: before 153,835, after 171,672, +17,837 (+11.59 %). Reason: `run_checks` is inlined after the hop-limit guard and brings three map lookups, up to two `bpf_ktime_get_boot_ns` sites, an extra `emit_event` and three stat updates; the verifier also walks the Sophia branches below once for each way out of it. This gate records and approves nothing: you read these numbers. Step 119 patches the one baseline row.
2. Step 101 is [STUCK] on its count: `shield_tc` went up 4 after the deletion, 976 to 980 (`debug-stats` 1,084 to 1,088). `shield_xdp` is unchanged. The object is smaller; the verifier walks one block twice more because pruning misses. The only ways I found to reach 976 are shaping unrelated code to steer LLVM's stack slots, which would break on the next toolchain, or relaxing the bound; neither is done. The ratchet allows it and no baseline row changed. Accept 980 and 1,088, or say what you want instead. The deletion itself is committed, because DP-716 and FND-025 need it.
3. Step 99 is still to do, after lane `lat` merges. When it runs, `pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md` line 121 also names AUTHORITY ("Wire to Shield TC"); the plan's file list leaves that row out, so it stays stale unless you want it added.
