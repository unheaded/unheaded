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

## Phase 1: the child-getpid probe (Steps 27-40, 2026-10-28)

`user/getpidfork.c`, run as the userland (`--userland target/getpidfork.mbc`), 800,000 triggers.

**Step 32 (two attempts).** The first draft ended the parent with `exit(0)`. It runs as pid 0 / init, and init's
`exit(0)` does not halt here: control comes back into user code and the parent's last line repeats for ever (one
`fork`, one `wait`, 182 copies of the tail at 900k triggers; `forks=1 waitpid=1` in STATS). That repetition first
looked like the child running the parent's post-`wait` code. It is not: with the parent spinning after its last line
(`for(;;);`, as real xv6 init never exits), fork, the child's line, `wait` and the parent's tail each happen exactly
once (three minimal variants, with and without a printf or getpid before fork, all agree). The probe now spins.

Two observations carried forward, not fixed here:
- **Init's `exit` returns to user code** instead of halting or panicking (xv6 panics "init exiting"). Any probe or
  gate that lets init exit sees its tail repeat. gate2's harness only greps `ISOLATION-PASS`, so it cannot see this.
- **A user PC trace** (`S4000 4005 …`, one hex PC per tick) is written to the TTY from `kexec` on, interleaving with
  the first user write (`G<trace>F: parent-pid=0`). Grep for `F: …`, not `GF: …`, until it is removed.

**Steps 30-37: dual books.**

| Line | flag OFF (BPF answers) | flag ON (owned kernel answers) |
|---|---|---|
| parent getpid (init) | `parent-pid=0` | `parent-pid=1` |
| child getpid | `child-pid=1` | `child-pid=other`: bit dump = **0xFFFFFFFF (-1)** |
| fork() return in parent | 1 (BPF slot) | 1 (bit dump) |
| parent after wait | yes | yes |
| STATS | `forks=1 waitpid=1 tty_r=0` | `forks=1 waitpid=5 tty_r=4` |
| ascend verified insns | **926542** | **911319** |

Delta ON - OFF = **-15223** verified instructions (ON is smaller). The plan expected the child to see init's pid (1)
through `cpu->proc`; it sees -1. The owned kernel has no process for the BPF-forked child, so its `sys_getpid`
fails: the two books do not merely number differently, the kernel's book has no entry for the child at all. fork's
return value in the parent stays the BPF slot (1) in both states, so with the flag ON a parent's `fork()` result and
its child's `getpid()` disagree (1 vs -1).

Evidence: `$S6/p1-fork-off.txt`, `$S6/p1-fork-on.txt`, `$S6/p1-fork-on-bits.txt` (temporary bit-dump probe, not
committed), `$S6/p1-fork-off-v1-exiting.txt` (the exiting first draft).

Plan deviations: 800k triggers, not 2.5M (past ~1M the spin's PC trace scrolls the probe lines out of the 2208-byte
ring). `UPC_VERIFIER_STATS=1 sudo …` as the plan writes it never prints a count, because sudo drops the variable;
`sudo UPC_VERIFIER_STATS=1 …` does.

**Step 38.** Flag restored OFF; the ascend object is byte-identical to the Phase 0 build
(sha256 `30b4ceb3…7fb2`); 0 modified files under `ebpf/`.
