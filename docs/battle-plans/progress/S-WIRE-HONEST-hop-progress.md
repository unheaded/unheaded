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

## For Stevie

1. `hop_xdp` growth, Step 91: before 153,835, after 171,672, +17,837 (+11.59 %). Reason: `run_checks` is inlined after the hop-limit guard and brings three map lookups, up to two `bpf_ktime_get_boot_ns` sites, an extra `emit_event` and three stat updates; the verifier also walks the Sophia branches below once for each way out of it. This gate records and approves nothing: you read these numbers. Step 119 patches the one baseline row.
