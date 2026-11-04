<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-WIRE-HONEST lane lat progress

## Log

- Phase 9 started: lane created with `lane.sh lat` from `sprint/s-wire-honest` at `9f6fc2ad` (lane `api` already merged, as Appendix A orders).
- Step 73: `latency_rust.py: all edits applied`.
- Step 74: `Finished`. The one warning is the toolchain's linker message (`libLLVM-23-rust-1.101.0-nightly.so` dlopen), not this change.
- Step 75: `[PASS] 36 eBPF programs pass the verifier; no known failures`, kernel `6.17.0-41-generic`, run under `flock /var/tmp/unheaded-sprint/ebpf-load.lock`; full output `$P/lat-step75.log`, counts `$P/lat.tsv`. All six latency-probe programs load. `check-ebpf-insns.sh --from $P/lat.tsv`: `[PASS] 36 programs within 10 % of the baseline`. The other 30 rows match the baseline exactly.

| Program | Baseline | Now | Change |
|---|---|---|---|
| tcp_connect_enter | 94 | 94 | 0 |
| tcp_connect_exit | 538 | 515 | -4.28 % |
| tcp_recvmsg_enter | 94 | 94 | 0 |
| tcp_recvmsg_exit | 517 | 516 | -0.19 % |
| tcp_sendmsg_enter | 94 | 94 | 0 |
| tcp_sendmsg_exit | 468 | 400 | -14.53 % |

- Extra check, not in the plan: `common.rs`'s four unit tests have no cargo path on the bpf target, so they ran through host `rustc --test`: 4 passed, including the 24-byte key size.
- Eviction: the reader never evicts. `DefaultLatencyReaderConfig` sets `DeleteAfterRead: false`, the fallback path in `cmd/trace-collector-go/main.go` takes that default, and the direct cilium/ebpf sweep has no delete at all. LATENCY_MAP is a plain `HashMap`, not LRU, so a row stays until the object reloads. At 65,536 rows `let _ = insert` drops new keys silently, as before; rows already present keep updating.

- Phase 10: Steps 77-80 passed. RED was `0`; `latency_go.py: all edits applied`; gofmt clean, vet clean, `go test -race` gave two `ok` (trace-collector-go, bpfschema); `TestDecodeLatencyTraceKey_WrongSizeRefused` passed (count 1).
- Ship rule: LATENCY_MAP: trace-collector and latency-probe ship together. An old object under a new binary counts `decode_key` errors; nothing misparses.

## For Stevie

1. `latency-probe/tcp_sendmsg_exit` fell 468 to 400 (-14.53 %), over 10 % in size though not growth. The `pid`/`tid` narrowing and the two zeroed key fields are gone, which is the likely reason; the plan expected a small move. Step 119 patches only rows over +10 %, so the baseline stays 468 and the row keeps that headroom unless you want it tightened (rule 17: one row, your word).
2. LATENCY_MAP has no eviction, and full tids make that matter a little more. West's `pid_max` is 4,194,304, so tids above 65,535 no longer fold onto low-16-bit rows, and a host that churns threads reaches 65,536 rows sooner. Once full, new threads get no samples and nothing counts the drop. Not changed here. Options: `LruHashMap`, `DeleteAfterRead`, or a drop counter. Do you want one, and in which plan?
3. `ebpf/latency-probe/src/main.rs` still says "per-flow" at lines 14 and 302; `latency_rust.py` does not touch them. Comment only, left as the payload has it.
