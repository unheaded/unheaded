# S-ASCEND-2 sprint log

Started 2026-10-28 on 31346cbf (based on sprint/true-wire, not develop: the M-184 verifier ratchet lives there; if E2 reverts the true-wire renumber, rebase this branch)

## Baselines

## Hypotheses (Steps 15-24, 2026-10-28)

| H | Claim | Verdict |
|---|---|---|
| H1 | `GETPID_INJECT = false`; `user/getpidprobe.c` exists | match (main.rs:413) |
| H2 | the two loader gates are "named in the docs but not performed" | match (workload.rs:182) |
| H3 | `NewLoadSequence` has no non-test caller | match (the one hit is its definition, mapfreeze.go:122) |
| H4 | the Doom HMAC secret literal is in `cmd/` | match (2 hits) |
| H5 | the RV2MBC SHA gate is env-only (`UPC_RV2MBC_SHA`) | match (main.rs:634); `BOOT_VERSION = 2` |
| H6 | bootctl creates the netns itself (needs root) | match (netns.rs `ip netns`/`ip link`) |
| H7 | no seccomp / demo-mode gate for UPC | match: 0 in `cmd/upc-bootctl`; the grep's 62 hits are other services' units (32 files carry SystemCallFilter) |
| H8 | the instance id is `flow_label & 0xFF`, unvalidated | match |
| H9 | no upc sysuser, unit or nft rule | match (0/0/0) |
| H10 | T_DEVICE never handled in the open path | **amended**: `adapters/usysfile.c` (in the kernel build) handles T_DEVICE in `sys_open` the xv6 way (devsw major check, :354/:368); the BPF open path (`fs_walk`, `main.rs`) does not. Which one serves `open()` at runtime is Step 183's question |
| H11 | riscv.h CSR helpers return `uint64` (4 bytes under ilp32e) | match (21 helpers; `typedef unsigned long uint64`) |
| H12 | ADR-043 claims "13 phases shipped" | match (1) |
| H13 | "Unheaded Linux" names in docs | 14 files (13±2) |
| H14 | `crates/upc-bootstub` exists | match |
| H15 | cartridge live-leg surfaces: `cmd/anomaly -wotan`, `upc-cartridgectl -sophia`, scorer files, `cmd/steel-echo/load.go` | match |

**Live state (Step 21, read-only):** east up (2 weeks 1 day), `enp2s0` UP. West: 0 UPC programs attached, 0 `upc0` netns, 0 nft rules naming `veth-upc0p`. GPG signing works (Step 6). Toolchain: all ten present (`nft` in /usr/sbin); the bpfel target is build-std, not a rustup target (the eBPF builds work).

## Baselines (Steps 7-14, 2026-10-28, HEAD 31346cbf)

- go build OK; `go mod tidy -diff` empty; monad-mbc, upc-bootctl, doom-runner release builds OK.
- go test: **0** failure lines (`p0-go-fail.txt` empty).
- Golden sweep: **8/8 SAME** (ls, cat README, cat BIGFILE, cat sub/NOTE, ls sub, echo hello, echo peace and love, wc
  README). The other four of the 12 golden sets (blank, multi, nosuch, uinit_echo) are checked by `upc-regression.sh`,
  which passed: 12/12 covered.
- `upc-regression.sh`: ALL GREEN; ascend **926542**, Doom **761580**, cart_xdp **42743**, cart_tick **146897** (same as
  S-TRUE-WIRE Step 16 and `docs/policy/ebpf-insns-baseline.tsv`).

## Known failures (KF)

| KF | What | At start |
|---|---|---|
| KF1 | `gate_nway` (3 processes, isolation) | **0 `NWAY-ok`, 231 `NWAY-FAIL pid2` markers** at 1.5M triggers: pid 2 fails, pids 0/1 print nothing ok (`p0-nway.txt`) |

Phase 0 exit gate: Steps 10 and 12 green; KF1 reproduced; H1-H15 match (H10 amended). → Phase 1.
