<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S5 UPC wire compute — execution log

Plan: `docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`. This session: Phases 0–4
(branch `sprint/upc-wire-compute`, worktree off `develop` @ `f7d730e7`). Sprint scratch
(`$S5`) lives outside the repo; evidence quoted here.

## Phase 0 — preflight + baseline (2026-10-23)

base: `f7d730e7` (develop tip)

| Hypothesis | Result |
|---|---|
| H0 | TRUE by content. `443de3f82` itself is not an ancestor: history was rewritten (re-sign); its equivalent `0a605108` ("runbook_list description no longer hard-codes 7 categories") is in `f7d730e7`'s history. Tree clean, branch `sprint/upc-wire-compute`. |
| H1 | TRUE. `scripts/upc-regression.sh` → `== ALL GREEN ==` (16 PASS + Doom load). |
| H2 | TRUE. 8/8 corpus `SAME` + 4/4 special (`uinit_echo`, `multi`, `nosuch`, `blank`) `SAME` against `~/tmp/golden`. No recapture. |

**Budgets (sprint baseline):** ascend verifier **926,542 insns (92.7%)**, Doom **761,580 (76.2%)**.
Both moved from the plan's expected 900,031 / 737,087 (July). No eBPF source change touches
`monad-cpu-ebpf` logic since `8de08368` (Stage 1, flag-off, measured 900,031 in July) except
`233b6451` (clippy; annotated the `read_monad_xdp` loop, not rewritten). Host kernel is now
6.17.0-41 and the nightly/bpf-linker toolchain has moved; the drift is attributed to
toolchain/kernel verifier changes, not a source regression. Recorded as the sprint baseline;
the getpid-flip budget gate (≤ 950,000) is measured against it.

**Translator / image baselines:** kernel 7552 RV32I → 11769 MBC, `xv6-mbc.mbc` sha256
`80ce532e…cb15`. Committed `doom/doom.mbc` (`a8f65173…`) is NOT what the current translator
emits from `doom/doom.elf` (`dcded746…`; `doom.rv2mbc` matches, `e7ab7b67…`): pre-existing
drift, so H-S5 compares translator output before/after the fix, not against the committed file.

**CSR instruction counts (Step 21):** kernel.elf 51; every userland ELF 0 (cat, echo, init, ls,
sh, uinit, wc); doom.elf 0.

**Known failures baseline (Step 12):** KF1 `gate_nway` → `NWAY-FAIL` (as listed); `go build ./...`
clean; `heimdall-bpf` builds `heimdall-vfs-write` (1 warning). Matches the KF table.

**Dormant S1.0/S1.1 present (Step 16):** `GETPID_INJECT: bool = false` (main.rs:413);
uservec `csrw mscratch,a0` + `csrr a0,sscratch`; userret `mv a0, a1`; `prepare_return` calls
`w_sscratch`; `user/getpidprobe.c` exists.

**prepare_return (Step 18):** `lw a5,48(a0)` → `csrr a2,satp` → `sw a2,0(a5)` … `csrw sscratch,a5`,
no other write to a5 in between. Translator CSR arm (Step 20): `csr_ptr = 12u8`, `tmp = 13u8`,
`mask = 14u8`, `14 => Ok(12)`, `15 => Ok(13)` all present.

**Other:** next free ADR number is **ADR-108** (ADR-105 is taken; `ADR_CART=ADR-108` for
Phase 9). Lich plan present: `docs/battle-plans/S-LICH-AWAKENS-battle-plan.md`. West
`enp16s0` = `192.168.13.2/30` + `fe80::da43:aeff:fe5f:a1c1/64`. East not contacted (this
session's scope is Phases 0–4; east legs belong to Phases 6–7). Mímir drift (Step 23):
ADR-043 + `wiki/Mimirs-Law.md` say all 13 phases shipped, CLAUDE.md says 9/11/13 pending —
left for Phase 8.

## Phase 1 — S1.2 root cause (2026-10-23)

**H-S1 CONFIRMED** (host + live). See `references/evolution1-stage1-2026-07-19.md`
§"S1.2 ROOT CAUSE (S5)". Live: `Z:SS satp=000007ff ss=000007ff tf=007be000`.
**H-S2 CONFIRMED**: `csrw sstatus,a4` stores 0xF400; any `csrr` overwrites a4 with the
CSR address. Red tests in `crates/monad-mbc/tests/csr_alias.rs` (5, all `#[ignore]`d).

| ID | Result |
|---|---|
| H-S3 | TRUE. Only two forms exist in any built image: `csrr` (CSRRS rd, x0) ×33 and `csrw` (CSRRW x0, rs) ×18, all in kernel.elf. No CSRRS/CSRRC with rs1≠x0, no immediate forms, no CSRRW with rd≠x0. Userland and Doom: zero CSR instructions. |
| H-S4 | **FALSE**. `start_mbc` reads `mhartid` (0xF14) → byte 0x12C50 > 0xFFFF. Keep the full-range mapping; ADR-067 amended instead of tightened. Indices used: sstatus, stvec, scause, sepc, stval, satp, sscratch, stimecmp, mscratch, mhartid. |
| H-S5 | TRUE. doom.elf has 0 CSR instructions. |

Kernel `.mbc` byte-identical to Phase 0 after the probe revert.

## Phase 2 — translator CSR fix (2026-10-23)

Commit `acf62722`. Decisions: Step 51 r0 as the write-path CSR pointer (restored to 0),
rd as the read-path pointer — stands (Step 53/54: the only async delivery in
`monad-cpu-ebpf` is the timer, `cfg!(not(feature = "ascend-linux"))`-gated; Doom has no
CSR ops). Step 52: every absent form rejected fail-closed (H-S3 true).

- Tests: 5 Phase 1 tests green + exhaustive table (6 forms × rd{x0,a4,a5,ra,a0} ×
  rs1{x0,a4,a5,a1} × CSR{0x140, 0xF14}); one existing unit test changed contract on
  purpose (`csrrs` with rs1≠x0 now asserts rejection; `csrr` asserts MOVI+LD only).
  Full monad-mbc suite green; `decode` filter 40 passed.
- Fuzz (Step 63): cargo-fuzz is not installed on west, so the cargo-fuzz project was built
  with the same sancov RUSTFLAGS and the libFuzzer binaries run directly, 300 s each:
  `fuzz_mbc_roundtrip` 1,129,958,905 runs, `fuzz_mbc_execute` 2,236,104 runs, 0 crashes.
  These targets exercise the MBC decoder/executor, not the translator; the translator is
  covered by the exhaustive table.
- Translation: kernel 7552 RV32I → **11664** MBC (was 11769, −105), `xv6-mbc.mbc`
  `109da1b3…`. Userland `.mbc` byte-identical (no CSR ops). **H-S5:** Doom translation
  byte-identical before/after (`dcded746…`, rv2mbc `e7ab7b67…`, data `e1191add…`).
- TTY gate: sweep 8/8 SAME, special 4/4 SAME (12/12, TTY + stats). Harness ALL GREEN;
  verifier unchanged (ascend 926,542 / Doom 761,580; eBPF untouched). Goldens stay valid
  for HEAD after Phase 2.
- `echo hello` instruction count 5,357,897 → 5,086,757 (−5.1%): `csrr` no longer
  re-stores the CSR, and push_off/pop_off read sstatus on every spinlock.
- ADR-067 Decision 2 implementation note + two open RFC Editor items (CSR range;
  KF10) — review requested, not self-approved.

**Security (Step 80):** (1) A user program cannot reach a rejected form at runtime:
translation is offline and a rejected form fails the build. (2) r0-as-pointer cannot leak a
non-zero r0 into user code: the sequence ends with `MOVI r0,0`, CSR instructions occur only
in kernel code, and no asynchronous trap is taken between the MBC words of one RV
instruction in the ascend build. A side effect of the old bug is also closed:
`prepare_return` wrote kernel_satp/kernel_sp/kernel_trap/hartid to address 0x7ff instead
of the trapframe. No new attack surface.

## Phase 3 — getpid flip, Dream Ladder L5 (2026-10-23)

- L5 regression (Steps 83–86): harness green at Step 71, nothing changed since. `echo hello`
  insn_count 5,357,897 (Phase 0) → 5,086,757 (−5.1%, lower as expected: `csrr` no longer
  re-stores the CSR).
- Step 87–89: `userret` now `csrr a0, sscratch` (was `mv a0, a1`). Kernel 11665 MBC.
  Sweep with flag OFF: **8/8 SAME** — the forkret return that broke the boot in July now
  works from sscratch. Specials 4/4 SAME; harness ALL GREEN (926,542 / 761,580).
- Step 92–96 (`getpidprobe`, 2,000,000 triggers):
  - flag OFF: `GP: probe-start`, `GP: getpid-returned`, `GP: pid=0`; insn_count 5,084,518.
  - flag ON: `GP: probe-start`, `GP: getpid-returned`, **`GP: pid=1`**; insn_count
    5,085,866; ends priv=3, PC 0x4179 (same exit PC as OFF), rom_fault=0, no DEAD sentinel.
    The SRET path re-arms the existing PC tracer, so extra `S4209 …` hex appears in the TTY.
  - **Round trip proven (Step 93), Step 96 FAILED**: ON and OFF disagree on the pid. The
    kernel returns xv6's `p->pid` (init = 1); BPF returns its slot index (init = 0).
- Step 97/98 verifier with the fixed path: flag ON **911,319** (91.1%) vs OFF 926,542
  (−15,223; July measured +5,409). ≤ 950,000 with ≥ 5% headroom: yes.
- Step 99/100 flag ON: sweep 8/8 SAME, harness ALL GREEN, Doom loads (761,580).
- Step 101: one getpid round trip through the kernel = +1,348 MBC instructions.
- **Step 102 [DECIDE]: `GETPID_INJECT = false`.** The recommendation was conditional on
  Steps 93/96/98/100 all passing; 96 failed (pid identity). Evidence kept; flipping needs a
  pid-numbering / proc[] reconciliation decision (out of S5 scope).
- Tree state (Step 104): flag `false` in `main.rs` (unchanged from base), `userret` uses
  sscratch; Step 88 sweep, Phase 3 specials and the flag-OFF harness were all run on this
  exact state.
- Plan text drift: Step 103 names "Epic 1.2" of the master plan, which is the
  programmatic-API epic; the note went under Epic 1.3 (syscalls) as item 1.3.5.
- Step 106–108: goldens unchanged and valid for `2308c3d7` (12/12 SAME on the final Phase 3
  state; no recapture). **L5 gate**: round trip proven, L5 regression green, verifier
  headroom 8.9% with the flag ON / 7.3% OFF; the flip itself is held on pid identity.
  Phase 3 DoD met.

## Phase 4 — TTY bridge live keyboard (2026-10-23)

- **H-K1 TRUE** (Step 111): the ascend `read(5)` FD_CONSOLE drain reads
  `KBD_MAP[tail & 63]` while `tail < head`, bumps only TAIL, blocks by PC rewind when
  empty, never resets HEAD/TAIL. No BPF change; verifier unchanged (926,542 / 761,580).
- `kbd_append_plan` + `BootRunner::append_kbd` (6 unit tests incl. 63→0 slot wrap, u32
  index wrap, full ring, corrupt indices). `write_kbd` untouched.
- Bridge: per-instance 4096 B keystroke queue (drop newest, `upc_tty_kbd_dropped_total` on
  `/metrics`), WS reader pushes into it (TODO removed), `GET /api/v1/kbd/drain` (405
  non-GET, 403 non-loopback, `max` bounded 1..4096, default 64), `--listen` default
  `127.0.0.1:26100` (`--port` kept as a deprecated loopback alias). `go test -race` green.
- `upc-bootctl console` (stub replaced): `boot`'s load path extracted into `boot_prepare`
  (shared, `boot` output unchanged — 12/12 goldens SAME after the refactor); loop = quiet
  trigger batch → TTY to stdout + bridge ingest → bridge drain + stdin → `append_kbd`;
  SIGINT/SIGTERM set a stop flag so teardown always runs.
- Step 123 (stdin, typed 8 s / 14 s after start, no `--input`): `$ hello`, the full `ls`
  listing, `$` after each; exited on stdin EOF + idle, rc=0.
- Steps 126–129 (bridge + `testdata/wsclient`, console without `--stdin`, stopped by
  SIGTERM): WS output shows `hello`, then a 203-byte paste (15 commands) ran in order
  (`paste-00` … `paste-13`, `z`); KBD head 0 → 11 → 214, at most 64 queued in the ring,
  `upc_tty_kbd_dropped_total 0`. A CR-terminated line (xterm Enter) also runs.
- Step 130/131: harness ALL GREEN (16 PASS); `--input` path unchanged (sweep + specials
  12/12 SAME). Nothing left attached after any run (`bpftool prog show`, `ip link`,
  `ip netns`).
- Not run: an actual browser session (no browser on this host); the WS leg used the Go
  client. Not done (parked): `--static` flag that `nix/yggdrasil/.../upc-tty-bridge.service`
  already passes — the bridge still serves `../../dashboard/` relative to its cwd.

## Phase 5 — Heimdall Gjallarhorn XDP + ringbuf readers (2026-10-23, second session)

Session 2 scope: Phase 5, then Part 2 Phases 9–12. **Phases 6–8 `[DEFERRED: east
escalation]`**: they need east as a node (Step 177 [ESCALATE], unanswered), and Phase 8
depends on them. East was not contacted.

- **H-G1 TRUE**: `heimdall-bpf` builds on west's nightly + bpf-linker.
- Step 138 decided: XDP_PASS always (observe-only).
- New programs: `gjallarhorn_xdp.rs` (IPv6 fixed header → UDP 16901 → exactly 20 B → `GJLR` →
  kind 1|2 → reserved 0; 32-byte `HornEvent`; per-CPU stats) and `execve.rs` (raw tracepoint
  `sched_process_exec`, same 24-byte record as vfs_write with `event_type=1`). KF2 closed.
- **K1 evidence (processed insns): vfs_write 20, execve 21, gjallarhorn XDP 292.**
- Plan deviation (Step 144): `bpftool prog loadall` cannot open Aya 0.1 objects (libbpf 1.x
  rejects the legacy `maps` section). The counts come from cilium/ebpf with
  `LogLevelStats` (`TestLoadObjects`, root-gated). **H-G2 TRUE**: cilium/ebpf v0.22 loads all
  three; the Step 153 Rust-sidecar fallback was not needed.
- Verifier rejections fixed on the way: a `[u8; 3]` pad zeroed through `memset` (the known
  `--btf` global-function rejection) and `core::ptr::read_unaligned` lowered to `memcpy`
  (`R4 !read_ok`). Both replaced by byte loads / scalar field writes.
- Classic tracepoint → raw tracepoint: `link.Tracepoint` needs tracefs, which is not mounted
  under `ip netns exec` (fresh mount namespace). The raw tracepoint attaches by name.
- Daemon: exact-size decoders + fuzz target; token bucket 1/s burst 3 on reverify horns for
  `--cluster`; bootstrap horns logged only (Phase 6); file events debounce 250 ms from the
  first event of a burst; own pid filtered; event-driven scans alert only on changed drift
  (otherwise a busy host's write noise re-alerts every 250 ms). Load/attach failures are
  logged once and the daemon keeps polling.
- netns lab (Steps 154–162; veth `s5va`/`s5vb`, ULA `fd00:5:5::/64` because the sender's
  `net.LookupIP` rejects `%zone`):
  - one reverify → `[HORN] kind=2 cluster=1 manifest_ptr=0x00000000cafebabe … action=reverify
    xdp seen=1 valid=1`, then an immediate re-verify + scan.
  - 50 reverifies in 0.90 s + 10 malformed → 4 re-verifies in total (burst 3 + 1 refill),
    47 rate-limited; exit counters `seen=61 valid=51 bad_magic=3 bad_kind=2 bad_len=3
    bad_reserved=2 rb_full=0`; malformed never triggered a scan.
  - write to the watched file → one alert after 0.06–0.22 s (two runs), none repeated.
  - teardown: no `s5` netns or link, 0 heimdall programs (`bpftool prog show`).
- Steps 164–169 (Wotan publish) were already shipped by Sprint 1 (`drift.go`, ADR-105):
  verified, nothing new. `go test -race` + `go vet` (linux, darwin) green, gosec clean (re-run with the repo-pinned `tools/gosec` build in Phase 12; the `~/go/bin` gosec cannot read Go 1.27 export data and checks nothing),
  `go.mod` unchanged. golangci-lint panics on this tree (known version trap), not run.
- Commits: `b6f5ec8a` (BPF), `930b1ddc` (daemon).

## Phase 9 — cartridge ADR + spec gate (2026-10-23)

- Marshal entry ruling: Part 1 exit (Step 228) is not passed because Phases 6–8 are
  deferred; the lead session authorised Part 2 on the Phases 0–5 evidence, and Step 228
  itself says east-only items do not block Part 2.
- D-C1…D-C5 taken as recommended. ADR-108 (Proposed) carries the Cartridge ABI v0, grounded
  in the shipping engine's semantics (24-bit branch offsets from pc+1, MOVI zero-extend,
  `set_flags` rules). Additions to the forged C.3: `loop_bound` (taken back-edges per tick
  job) so every job is provably bounded; tick per-call budget-out = implicit YIELD;
  misaligned `LD`/`ST` and stores to DATA fault; header sha256 covers header[0..20) + ROM +
  DATA.
- **H-C1 TRUE** (`services/anomaly` has no importer). **H-C2 TRUE** via cilium/ebpf
  `Program.Benchmark` (`ret=2 per-run=24ns`, 100,000 repeats on the heimdall XDP object);
  bpftool cannot load Aya objects, so all S5 benchmarks run through `pkg/cartridge`.
- RFC Editor requests: `docs/protocol/RFC-EDITOR-REQUESTS-S5.md` (Sophia root key, the
  EVENT_COMPUTED sub-kind, Wotan topics, ADR-067 CSR range, KF10). Draft text only; none
  self-approved.
- ADR number: drafted as ADR-108, renumbered **ADR-108** by the lead's allocation across the
  parallel sprint branches (ADR-108 = Sprint 2 two brains, ADR-107 = Sprint 3 chaos envelope).

## Phase 10 — shared step function + host reference (2026-10-23)

- `monad_common::cart` (`9cc40068`): 14 golden-vector tests, red then green. The
  monad-cpu-ebpf instruction stream is byte-identical with and without the module
  (llvm-objdump -d, both variants); only DWARF changes.
- `crates/upc-cartridge` (`b7398de5`, `89468b0c`): image format, validator, reference
  runner, `cart` CLI, the `cartridge` ADR-080 surface (`FeatureGate::Cartridge`).
- **H-C3 FALSE — first Lich-hook output.** 10,000 random allowlisted programs in lockstep:
  `monad-mbc` execute.rs (host emulator) drifted from `monad-cpu-ebpf` on nine opcodes:

  | Opcode | Host emulator (before) | Shipping engine (kept) |
  |---|---|---|
  | MUL 0x03 | C = unsigned overflow | C = 0 |
  | SHL/SHR/SAR 0x0B–0x0D, SHLR/SHRR/SARR 0x36–0x38 | C = last bit shifted out | C = 0 |
  | JP 0x24 | taken if N=0 **and Z=0** | taken if N=0 |
  | MULH 0x39 | unsigned high word | signed high word (RV32M MULH) |

  Host side fixed; three host unit tests that encoded the old semantics updated. The host
  emulator also has no MULHU arm (the translator emits MULHU): outside the cartridge
  allowlist, logged for the Lich sprint.
- fuzz `fuzz_cart_step`: 191,764,880 runs / 300 s, 0 crashes (cargo-fuzz RUSTFLAGS + debug
  assertions + overflow checks; libFuzzer binary run directly).
- No new third-party crate: upc-cartridge's lock adds only itself; sha2 0.10.9 is
  upc-bootctl's.

## Phase 11 — cartridge-ebpf + verifier-budget gate (2026-10-23)

**Verifier gate (≤ 500,000 per program): cart_xdp 14,395, cart_tick 22,575.**

How the budget was won (each measured):

| Shape | cart_xdp processed insns |
|---|---|
| `for _ in 0..64` around the inlined step, 4 slots unrolled | 1,000,001 (rejected; `max_states_per_insn 124`: opcode paths never prune at the loop head) |
| step as a `bpf_loop` callback (needs `--btf` → `build.rs`) | 498,904 (every header-parse path re-verified all 4 slots) |
| + per-slot run as a **global** BPF function (verified once) | **14,395** |

Verifier traps hit and fixed on the way (all recorded in code comments): LLVM folding
`addr < 0x100 && aligned` into one mask test on a register copy (`mem_index` launders the
index); `--btf` turning zero-inits, struct moves, store loops and even adjacent scalar zero
stores into rejected global `memset`/`memcpy` calls (volatile word writes, in-place zeroing;
`begin_run`/`begin_job` in the shared step too, same host semantics); an enum's
uninitialised payload byte stored across the callback (u32 step codes); a variable-offset
L4 packet read (one bounds check on the exact pointer). The shared driver is now
`run_one`/`run_out`, so BPF (callback) and host (plain loop) share the loop semantics.

- Regression harness has a third variant (cartridge load + budget + root tests): **ALL
  GREEN**, ascend 926,542 and Doom 761,580 unchanged, 12 root tests.
- monad-cpu-ebpf untouched (`git diff dcacffb4 -- ebpf/monad-cpu-ebpf` empty).
- Root tests through BPF_PROG_TEST_RUN, images from the real `cart` CLI: pass/drop-udp-9
  verdicts right; fault path passes and the breaker disables the slot at exactly 64 faults;
  **BPF == reference: 3/3 flow state hashes, 21/21 events in order** (90 packets, `cart run`).
- Baselines (5 × 1,000,000 test_run, median, `taskset -c 2`, noise ≤ 4.8 %): b0_empty
  **105 ns**, b0_pass **153 ns**, native heimdall XDP 21 ns, native qos-ebpf 42 ns (its
  early-PASS path; no Monad HbH in the frame). The empty-slot cost is the parse + FLOW_META
  LRU update + four slot checks; Phase 13 measures against it.
- bpftool cannot load Aya 0.1 objects (legacy `maps` section) — every count and benchmark
  in S5 goes through cilium/ebpf (`pkg/cartridge`).

## Phase 12 — Sophia registry + signed loader (2026-10-23)

- `cmd/upc-cartridgectl` (`40a01890`, `c4f6a6d3`). Step 308 decision taken: Sophia knowledge
  entries (`x-cartridge/<name>`, provisional) when `--sophia` is set, else a file-backed
  registry with the same schema; image bytes content-addressed on disk either way. Sophia
  backend tested against a fake of its real `/api/v1/knowledge` envelope; not run live (stack
  down, KF5).
- **Load check (one read, same bytes throughout):** Gungnir ML-DSA-65 seal against the pinned
  `--pubkey` (expiry included) → `cart validate -` on stdin → ABI v0 decode → registry
  sha256 match → slot write (config + gen last). Live refusals: unsigned, seal over other
  bytes (`payload hash mismatch`), other key (`signature verification failed`), validator
  reject (`opcode 0x04 not allowed`). `--lab` loads unsigned, warns, marks `lab="1"`.
- **Atomic swap** (pass → drop-udp-9, spare slot, then deactivate old): 400 racing bench
  invocations saw 47 PASS then 353 DROP, nothing else, 0 faults.
- Metrics on `127.0.0.1:26610` moved with traffic; pins 0600 in a 0700 dir, registry
  0700/0600. Live-lab bug found and fixed: `slots.json` was read as a record by `list`
  (records now live in `records/`, regression test added).
- Lab key recipe: ADR-108 §Signing and loading, `cartridges/README.md`; wiki page
  `UPC-Cartridges`.
- gosec: the `~/go/bin/gosec` on west cannot read Go 1.27 export data and silently checks
  nothing; the repo-pinned `tools/gosec` build is clean on `cmd/upc-cartridgectl`,
  `pkg/cartridge`, `cmd/heimdall-daemon` (two findings fixed on the way).

## Session 2 close — regression (2026-10-23)

- Harness **ALL GREEN** (three variants): ascend **926,542**, Doom **761,580** (both unchanged
  from Phase 0), cart_xdp **14,395**, cart_tick **22,575**, 12 cartridge root tests.
- **12/12 goldens SAME** (8 corpus + uinit_echo, multi, nosuch, blank).
- **Doom translation byte-identical**: `doom.elf` → `dcded746…` / rv2mbc `e7ab7b67…` (Phase 2
  values). monad-cpu-ebpf instruction stream unchanged (no source diff since the base).
- `go build ./...` clean; `go test -race` green on every touched Go package; Rust suites
  green (monad-common 108, monad-mbc 268 + integration, upc-cartridge, upc-api 25).
- West left clean: no netns, no `s5` links, no XDP/cart/heimdall programs, no pins. East
  never contacted.

### Open / escalations

- **[ESCALATE] Step 177** (east as a node) — Phases 6–8 `[DEFERRED: east escalation]`; Part 1
  exit (Step 228) therefore not passed; Part 2 proceeded on the lead's direction.
- **RFC Editor items 1–5** open (`docs/protocol/RFC-EDITOR-REQUESTS-S5.md`); ADR-108 stays
  Proposed. The Anamnesis sub-kind, if ruled to need its own type, is [ESCALATE]
  (Specification Required).
- **Host emulator gap:** monad-mbc execute.rs has no MULHU arm (translator emits MULHU);
  outside the cartridge allowlist, logged for the Lich sprint.
- **Not run:** Sophia registry against a live Sophia (stack down); XDP attach of cart_xdp to
  a real or veth interface (`serve --xdp-iface` exists; this session used test_run only).
- Next: Phase 13 (rate limiter vs qos-ebpf), 14 (sampler), 15 (scorer → services/anomaly),
  16 (replay), 17 (Lich hooks). Benchmarks start from b0_pass 153 ns.

## Phase 13 — token-bucket rate limiter (2026-10-23, session 3)

Session 3 scope: Phases 13–16 (Phase 17 belongs with the Lich sprint, Phase 18 needs east).
Fresh worktree: every object rebuilt; harness baseline ALL GREEN (ascend 926,542, Doom
761,580). Host shared with other sessions (llama-server ~340 % CPU): benchmarks pinned
with `taskset -c 5`, noise reported per line.

- `cartridges/ratelimit.s` reproduces qos-ebpf's bucket exactly (bytes; first packet
  passes; add = elapsed/8000 × rate, refill only when add > 0; DROP if tokens < len).
  No DIV: `elapsed/8000 = MULH(elapsed>>6, 274877907) >> 3`, checked over all 2^26
  inputs. 32-bit clock: `iat_bucket >= 22` = full bucket (rule: cap ≤ 524,287 × rate).
  41 of 64 insns on the longest path.
- **Correctness**: reference runner 30,000 packets / 10,625 drops, 0 mismatches against
  a u64 re-implementation of the qos-ebpf math; two cartridge mutations caught. BPF
  test_run 1,000/1,000 verdicts == Go qos-ebpf math, flow hashes == `cart run`.
- **Enforced rate**: 99.99 Mbps of 100 at a quantum-aligned spacing; real-clock flood
  96.0 Mbps.
- **Finding (oracle, not fixed)**: qos-ebpf discards the remainder of `elapsed / 8000`
  at every refill. At 20 µs packet spacing a 100 Mbps bucket admits 80 Mbps. The
  cartridge matches it (4,012 == 4,012 passes) because the oracle wins; a fix belongs
  to qos-ebpf (carry the remainder: `last += q * 8000` instead of `last = now`).
- **Isolation**: flood 790 drops, interleaved quiet flow 0; per-flow state only.

**Benchmark gate #1** (same IPv6 + Monad HbH frame for both engines, 5 × 1,000,000,
median):

| Case | ns/pkt | insns/pkt |
|---|---|---|
| b0_pass (IPv6 frame) | 154 | 2 |
| ratelimit, pass regime | 343 | 35.3 |
| ratelimit, drop regime | 334 | 33.3 |
| native qos-ebpf bucket path, pass | 299 | — |
| native qos-ebpf bucket path, drop | 325 | — |

(a) overhead ≤ 150 ns: **FAIL** (189 / 180 ns). (b) ≤ 3× native: **PASS** (1.15× /
1.03×). Step 340 profile: the interpreter, not the refill math. A 60-instruction
straight-line cartridge measured 8.1 ns per NOP, mostly the per-step `bpf_loop`
callback; `cart_loop_cb` now runs 4 steps per callback (NOP 3.5–4.3, MOV 4.7, LD 6.2,
SYSCALL 6.6 ns; 8 per callback gave 3.7 ns/NOP for twice the verified body, not kept).
Verifier after the change: cart_xdp 24,986, cart_tick 63,906. Recorded as a known
limitation (plan rule): roughly 4–7 ns per interpreted instruction on this host.

- Harness ALL GREEN after the change (18 cartridge root tests). Commit `0594cd7b`.

## Phase 14 — flow sampler (2026-10-23, session 3)

- `cartridges/sampler.s`: window of N packets (DATA, shipped 8) → features f0 bytes, f1
  iat_bucket sum, f2 SYN count, f3 max length, f4 flow_pkts, f5 l4_proto, f6 dst_port,
  f7 OR of TCP flags; every Nth packet EMIT 'S' {pkts, bytes} + QUEUE the snapshot; a
  flow's first packet EMIT 'F' (its ts_ns is first-seen). Always PASS, Monad header
  never written. First draft's longest path was 67 (the reference test caught a Budget
  fault); now 62 of 64.
- Reference: exact 1:N for N = 1, 2, 5, 8, 100 against an independent model; deterministic.
- BPF beside the rate limiter: 77 events, 74 snapshots, 3/3 hashes == `cart run`; rate
  limiter unaffected (600/600).
- Overflow: 16,384 queued, 3,616 counted drops, 16,361 counted event drops, 0 faults,
  every packet PASS.
- **Gate #2** (clean run): sampler 360 ns vs b0_pass 147 → **213 ns overhead, FAIL**
  (gate 120); ratelimit + sampler 470 ns over b0 vs 408 ns sum → **+15 %, FAIL** (gate
  +10 %). Same interpreter cost as Gate #1.
- Harness ALL GREEN (19 root tests). Commit `d592f230`.
- Lead request (LICH-015): host emulator MULHU was a NOP; arm added with a test against
  the engine semantics (`4cbef086`). Cartridge allowlist unchanged.

## Phase 15 — int8 anomaly scorer → services/anomaly (2026-10-23, session 3)

**Model and data (Scientist lens).** int8 MLP 8→16→1 (D-C6), inputs
`x = min(f >> sh, 255)` over the sampler snapshot. Data: ten synthetic flow families
(web client, bulk download, DNS, SSH, QUIC; SYN flood, UDP flood, NULL/FIN/XMAS scan,
RST storm, clockwork beacon), packets drawn as (gap, length, TCP flags) from fixed
distributions and run through the real sampler cartridge on the reference runner. No
capture, no payload, nothing personal. Train seed 0x7A11 (400 flows/family, 18,468
vectors); eval seed 0xE7A1 (100 flows/family, 4,585 vectors, 4,158 of them not present
verbatim in train). Model id 0xd0ee2b6b, `cartridges/scorer.model.json`.

| | accuracy | TPR | FPR | AUC |
|---|---|---|---|---|
| train, float | 0.9997 | 1.0000 | 0.0005 | 1.0000 |
| eval, float | 0.9996 | 1.0000 | 0.0009 | 1.0000 |
| eval, int8 | 0.9996 | 1.0000 | 0.0009 | 1.0000 |
| eval unseen-only, int8 | 0.9995 | 1.0000 | 0.0009 | — |
| shuffled-label control, eval | 0.4927 | — | — | 0.5000 |

What this does and does not show: the ten families are almost perfectly separable by
these eight header features, and the model separates held-out flows from the same
generators; the shuffled-label control falls to chance, so nothing leaks labels. It says
nothing about real traffic. The model leans on shortcuts the generators allow: it
ignores the SYN count entirely (floods are already separated by size, timing and flags),
scores fast arrivals and small maximum lengths high, and is not monotone in window bytes
far outside the training range. Seven of sixteen hidden units carry no signal (ReLU +
weight decay drove w1 or w2 to ~0).

**Quantization**: |int8 − float| ≤ 2 points on **99.83 %** of eval (max 5, mean 0.86):
gate (≥ 99 %) met. Per-row int8 weights, per-row u8 requantization, rounding folded into
the biases, 256-entry sigmoid table (bin 0.04 logit).

**Cartridge**: `cartridges/scorer.s`, existing opcodes only, ~720 insns per job (3
`cart_tick` calls of 256), image 1,052 bytes. Cartridge == `QModel::score` on 6,585
vectors (held-out + random/extreme); BPF tick == `cart tick` reference on a sampler-fed
run (18 jobs: scores, order, last-job hash).

**Gate #3: PASS**: 251,572 flows/s on one core (5 runs, 248,942–252,908); sampler XDP
cost 355 ns with the scorer loaded vs 352 ns without. DOT4 estimate: ~420 insns per job,
~1.7× (ADR-108 note; not added, [ESCALATE] to an ISA spec gate if ever wanted).

**services/anomaly reached** (KF3 half-closed): `IngestScore` (strict JSON, 4 KiB cap,
score and address checks, counters) + `anomaly.scores` Wotan consumer + `ListenAddr`;
`cmd/anomaly` = service + bridge from the pinned CART_EVENTS ring (in-process, or
`--wotan` publish). Wotan leg not run live (stack down, Phase 6 scratch Wotan deferred).

**End to end on west** (`$S5/p15-e2e.sh`): three signed cartridges loaded through
`upc-cartridgectl`, cart_xdp attached (generic) to a veth whose peer is in netns `s5e`,
400 × 1,400 B UDP flood + 16 slow DNS-like packets. Rate limiter dropped 381 flood
packets; 52 jobs queued and scored; `/api/v1/anomalies/top`: flood score 99, DNS-like
flow 1; 0 rejected. Teardown verified: no cart programs, s5 links, netns or pins.

- Harness ALL GREEN (ascend 926,542, Doom 761,580). Commit `9404db47`.

## Phase 16 — Anamnesis replay (2026-10-23, session 3)

**What replay proves.** A cartridge run is a pure function of its image, its starting
state and its inputs (the FIELD table and the clock). cartridge-ebpf records exactly
those (TraceHead) plus the pc, instruction word, step code and state hash after every
instruction (TraceStep) and the outcome (TraceEnd); `cart replay` re-executes with the
same `run_one` and checks every hash. A match means the live run is reproduced
instruction by instruction off-line, so a verdict, an emitted score or a fault can be
explained after the fact from the trace alone, and nothing else (no payload, no helper
state) influenced it. A mismatch names the first divergent instruction.

- Records: `EVENT_COMPUTED 0x01` + sub-kind `0x43`; head 464 B, step and end 32 B;
  CART_TRACE 1 MiB ring; off by default, `CART_CFG[0]` = one run in N per CPU; drops
  counted. Layout in ADR-108, contract `docs/architecture/cartridge-replay.md`.
- Verifier traps on the way (all fixed, recorded in code): 3-byte zero store and the
  tick stage's all-zero FIELD loop → rejected `memset` under `--btf`; a 6-argument
  function (BPF passes 5); cart_xdp → slot → callback chain over the 512-byte combined
  stack (cart_xdp's SlotArgs moved to a per-CPU map); the state hash inlined at every
  site took cart_tick to 328,144 processed insns. `cart_state_hash` and
  `cart_trace_step` are now global BPF functions: **cart_xdp 42,743, cart_tick 146,897**
  (gate 500,000; were 24,986 / 63,906 before tracing).
- Host (`tests/replay.rs`): 200 rate-limiter runs / 7,806 steps bit-exact through the
  file format; a flipped pkt_len bit diverges at step 2 (the FIELD read); another image
  at step 0 (`insn`); a corrupted step hash at that step; scorer job time travel
  (goto/back/regs; score in r2 at the EMIT); 5,000 hostile byte strings never panic.
- Live (`TestReplay_LiveRunsAreBitExact`, test_run, every run traced): 160 rate-limiter,
  160 sampler and 66 scorer tick runs **bit-exact**; replayed verdicts == the 160 live
  XDP returns; `--watch r1` shows the MLP row accumulator through a tick call;
  `--to 188` stops before `SYSCALL 3 (EMIT)` with r2 = 99. One flipped input bit in the
  file: `run 9 … DIVERGED at step 2 (hash): recorded 0x72d236c4, replayed 0xb33fc8f4`,
  159 others bit-exact, exit 3. Off by default (0 records); 1-in-10 traced 20/200 runs.
- Through the CLI on the real clock: `upc-cartridgectl record --every 2000 --duration 3s`
  during a 3M-run test_run flood → 52,537 records, 1,499 runs, 0 dropped, 2.5 MB;
  `upc-cartridgectl replay run.anam --slot 0` (image fetched from the registry blob the
  trace names, sha-checked) → **1,499 bit-exact**.
- Harness ALL GREEN (21 cartridge root tests); goldens **12/12 SAME**; Doom translation
  byte-identical (`dcded746…` / `e7ab7b67…` / `e1191add…`). (`xv6-mbc.mbc` is
  `f5bf2003…` in this worktree, not the Phase 2 `109da1b3…`: Phase 3's `2308c3d7`
  changed the kernel after that value was taken; no kernel or translator source changed
  in Phases 13–16, and the goldens match.)
- Commit `4634796c`.

## Session 3 close — final benchmarks with the trace-capable object (tracing off)

5 × 1,000,000 test_run, median, `taskset -c 5`, noise ≤ 3.2 % except b0_empty 11 %:

| Case | ns/pkt | vs. | Gate |
|---|---|---|---|
| b0_pass (IPv4 / IPv6 / TCP frame) | 150 / 155 / 149 | — | — |
| ratelimit, pass / drop regime (IPv6 Monad frame) | 347 / 341 | native qos-ebpf bucket 283 / 317 | #1(a) +192 / +186 ns: **FAIL** (150); #1(b) 1.23× / 1.08×: **PASS** (3×) |
| sampler (TCP frame) | 377 | b0 149 | #2 +228 ns: **FAIL** (120) |
| ratelimit + sampler | 642 | sum of overheads 429 | +493 ns, +15 %: **FAIL** (+10 %) |
| scorer | 230,521 flows/s | — | #3 **PASS** (100,000); XDP sampler 368 ns with or without the scorer slot |
| interpreter, per instruction | NOP 3.3, MOV 5.1, SYSCALL 5.5, LD 6.6 | — | — |

The trace checks cost ~1–5 % with tracing off (rate limiter 343 → 347, sampler
360 → 377, scorer 251,572 → 230,521 flows/s).

### Open after session 3

- **Gates #1(a) and #2 FAIL** on this host: ~3–7 ns per interpreted instruction. Not
  blockers (plan rule). Options for a later sprint: fewer instructions per cartridge,
  more steps per callback (8: −15 % per instruction for twice the verified callback),
  or a superinstruction/DOT4 ISA change ([ESCALATE], ISA spec gate).
- **qos-ebpf refill remainder** (finding, oracle not changed): admits 80 Mbps of 100 at
  20 µs spacing.
- **Wotan leg of anomaly.scores** not run live (stack down; Phase 6 scratch Wotan
  deferred with the east escalation). In-process path proven end to end.
- **RFC Editor items 1–3** still open (`x-cartridge`, sub-kind `0x43`,
  `anomaly.scores`); ADR-108 stays Proposed.
- **Scorer** is trained on synthetic families only; its accuracy says nothing about
  real traffic, and it ignores the SYN count.
- Phase 17 (Lich hooks) left to the Lich sprint; Phase 18 (DISTRIBUTED) needs east.

## Rate limiter follows the qos-ebpf remainder fix (2026-10-23, session 4)

The Phase 13 finding (qos-ebpf dropped `elapsed mod 8000` at every refill) was fixed on
`develop` in `8ab534b3`: the refill step moved to `monad_common::token_bucket::refill`,
`last_refill_ns` advances by `q * 8000`, and a refill that reaches the capacity sets
`last = now`. At the lead's request the cartridge and both oracles follow it on this
branch (develop's qos-ebpf is not merged here; the oracles re-implement its new math).

- `cartridges/ratelimit.s`: `last = now` moved off the shared refill path; the
  capped/full branch and the idle >= 4.19 s branch keep `last = now`; `add_some` keeps q
  in r9 and stores `last_lo + q * 8000`. The existing `add >= cap - tokens` compare is
  already develop's `q > cap / rate || add >= cap - tokens` (add fits 32 bits under the
  parameter rules). 63 words; longest path **46 of 64** (was 41), checked by a new static
  longest-path test.
- Oracles: `QosOracle` (`tests/ratelimit.rs`) and `qosBucket`
  (`pkg/cartridge/ratelimit_linux_test.go`) now mirror `token_bucket::refill` line for
  line. Reference: 30,000 packets / 10,623 drops, 0 mismatches. The old cartridge fails
  the new oracle (mutation check: `ratelimit_matches_qos_ebpf_oracle` and the new
  remainder test both red).
- **20 µs spacing now enforces the configured rate**: reference 5,012 / 20,000 passed,
  99.99 Mbps (was 80); BPF fixed clock 24 µs and 20 µs both **99.99 Mbps**, every verdict
  == oracle (6,012 and 5,012 passes); the 20 µs case is now gated at ±5 % like 24 µs.
  Real-clock flood 97.62 Mbps (was 96.0; bound −10 %).
- BPF == oracle 1,000/1,000, flow hashes == `cart run`; live replay bit-exact
  (`TestReplay_LiveRunsAreBitExact`, 160 rate-limiter runs). Verifier unchanged (the
  object did not change): cart_xdp 42,743, cart_tick 146,897.
- Lich seed `tomb/lich/seeds/cartridge/ratelimit.cart` rebuilt from the new source.
- Benchmark Gate #1 numbers above were measured on the pre-fix cartridge; the refill
  path is up to 5 instructions longer (re-bench in the sprint close).

## Phase 17 — Lich hooks (2026-10-23, session 4)

Session 4 scope: Phase 17, then Phase 19 close. Phases 6–8 and 18 stay
`[DEFERRED: east escalation]`. The Lich sprint (`sprint/lich-awakens`) was read with
`git show`, not merged: its LICH-015 `diff_harness` compares the host emulator and the
shared step on registers/flags/PC for ALU+branch programs (50,000 programs, 0
divergences); its one finding, MULHU, was fixed here in `4cbef086`.

- **Oracle** (`crates/upc-cartridge/src/diff.rs`, tests first): random validated
  XDP-stage cartridges (≤ 48 words, forward branches, register prologue, edge operands,
  LD/ST/LDB into both windows, FIELD/NOW/EMIT/VERDICT, ~1/16 deliberately out of range)
  run in lockstep on `monad_common::cart` and `monad-mbc` `ExecCpu`: registers, Z/N/C,
  PC and the flow window compared after every instruction; EMITs compared at the end.
  Syscalls are serviced on the host side from the same input; a cartridge fault ends the
  comparison. Self-test: a one-bit flip in the host arm after instruction 0 is reported
  at instruction 0 (100+ cases).
- **Seeded campaign** `cart diff --seed 0x17 --count 20000`: 296,740 instructions,
  **0 divergences** (11,212 halt, 4,320 verdict, 4,468 fault endings). Test campaign
  10,000 cartridges: 16,495 in-window memory ops, 15,962 syscalls, 0 divergences.
- **Three engines** (`TestDifferential_ThreeEngines`, root, test_run only): 1,000
  cartridges (seed 0x17d1ff) through `cart_xdp`: **1,000/1,000** verdict + FLOW_STATE
  hash + generation + EMITs equal to the two host engines; 0 breaker trips; 19.5 s. The
  third engine is `cartridge-ebpf` (the shared step compiled to BPF); `monad-cpu-ebpf`
  cannot run cartridges and its full-engine arm stays LICH-015's east arm.
- **Fuzz** (cargo-fuzz RUSTFLAGS, libFuzzer run directly, 300 s each):
  `fuzz_cart_differential` 3,046,713 runs, 0 crashes; `fuzz_cart_image` 95,882,146
  runs from 9 seeds, corpus 413, 0 crashes. The first image run (526,043,178 runs)
  stalled at corpus 31: the embedded sha256 rejects nearly every mutation at decode, so
  the target now also checks each input with its sha recomputed.
- **LICH-015 harness against this branch** (scratch copy, path deps pointed at this
  worktree, no repo change): `mulhu-probe` **CONVERGED** (`r0 = 0x0b00ea4e` both
  engines), self-test PASS, `run --iters 50000 --seed 0xc3c35eed` 0 divergences. Its
  `mulhu_highword` seed can close after the merge.
- Seeds `tomb/lich/seeds/cartridge/` (3 shipped + 6 mutations, each hitting its intended
  refusal); hook doc `tomb/lich/UPC-CARTRIDGE-HOOKS.md` with the merge integration
  points. No new third-party crate: the fuzz crate's lock gains upc-cartridge, upc-api
  and sha2's tree, every crate@version already in `crates/upc-cartridge/Cargo.lock`.
- gosec (repo-pinned `tools/gosec`): `./pkg/cartridge/...` clean as CI runs it; with
  `-tests` the new test file adds nothing (its env knobs are parsed as numbers).

## Sprint 5 closing summary

S-UPC-WIRE-COMPUTE closed 2026-10-23 on `sprint/upc-wire-compute` (not merged, not pushed;
commits unsigned, `--no-gpg-sign`, for `scripts/resign-unpushed.py`). Base `f7d730e7`.

### Phases

| Phase | Result |
|---|---|
| 0 Preflight | PASSED. Baseline ascend 926,542 / Doom 761,580 (toolchain drift from July's 900,031 / 737,087) |
| 1 S1.2 root cause | PASSED. `csrr` ran the CSR read-modify-write through a4/a5; proven on host and live |
| 2 Translator CSR fix | PASSED. Kernel −105 MBC words; userland and Doom byte-identical; ADR-067 note |
| 3 getpid flip (L5) | Round trip PASSED; flip held: Step 96 FAILED on pid identity (kernel 1, BPF 0); `GETPID_INJECT=false` |
| 4 Live keyboard | PASSED. `upc-bootctl console`, bridge drain loopback-only |
| 5 Heimdall XDP + ringbufs | PASSED. K1: vfs_write 20, execve 21, Gjallarhorn XDP 292 insns |
| 6–8 Mímir's Law 9/11/13 | **DEFERRED** (east as a node, Step 177 [ESCALATE]); Part 1 exit (Step 228) not passed; Part 2 ran on the lead's direction |
| 9 Cartridge ADR + spec gate | PASSED. ADR-108 Proposed, RFC Editor requests filed |
| 10 Shared step + host reference | PASSED. H-C3 false: nine host-emulator opcode drifts fixed |
| 11 cartridge-ebpf | PASSED. Verifier gate, BPF == host hashes, fault breaker |
| 12 Signed loader + registry | PASSED. Seal + validator on the same bytes; atomic swap |
| 13 Rate limiter | PASSED; Gate #1(a) FAIL (known limitation). Follows the qos-ebpf remainder fix (session 4) |
| 14 Flow sampler | PASSED; Gate #2 FAIL (known limitation) |
| 15 int8 scorer → services/anomaly | PASSED. Gate #3 PASS, quantization gate PASS, end to end on real XDP in a netns |
| 16 Anamnesis replay | PASSED. Bit-exact live record → replay; divergence named at the right step |
| 17 Lich hooks | PASSED. Three engines agree 1,000/1,000; fuzz clean |
| 18 DISTRIBUTED (stretch) | **DEFERRED** (east, Step 435 [ESCALATE]); follow-on card Epic 1.5.6 |
| 19 Close | PASSED (this section) |

### Final regression (session 4, west)

- `scripts/upc-regression.sh` **ALL GREEN**: ascend **926,542**, Doom **761,580**
  (both unchanged from Phase 0), cart_xdp **42,743**, cart_tick **146,897** (gate
  500,000), 22 cartridge root tests incl. the three-engine differential.
- Goldens **12/12 SAME**, TTY and stats (8 corpus + uinit_echo, multi, nosuch, blank).
- Doom translation byte-identical: `.mbc` `dcded746…`, `.rv2mbc` `e7ab7b67…`, `.data`
  `e1191add…`. `ebpf/monad-cpu-ebpf` has no diff against the sprint base.
- Go: `go build ./...` OK; `go test -race` green on cmd/anomaly, cmd/heimdall-daemon,
  cmd/upc-tty-bridge, cmd/upc-cartridgectl, pkg/cartridge, pkg/ports, services/anomaly.
- Rust: upc-cartridge 40, monad-mbc 391, monad-common 110, upc-api 25 tests, 0 failed.
- Spec gate (Step 447): no Monad wire byte changed, no MBC opcode added; RFC Editor items
  1–5 all open; ADR-108 stays Proposed.
- gosec (repo-pinned `tools/gosec`): clean on the sprint's Go packages as CI runs it.
- West left clean: no BPF programs, netns, pins or processes from S5. East never contacted.

### Benchmark gates (close, 5 × 1,000,000 test_run, median, `taskset -c 5`)

| Gate | Bar | Measured | Result |
|---|---|---|---|
| #1(a) rate limiter overhead over b0_pass | ≤ 150 ns | +193 ns pass / +186 ns drop (349 / 342 vs 156) | **FAIL** |
| #1(b) rate limiter vs native qos-ebpf | ≤ 3× | 1.25× / 1.08× (native 280 / 318 ns, this branch's pre-fix qos-ebpf) | PASS |
| #2 sampler overhead over b0_pass | ≤ 120 ns | +216 ns (365 vs 149) | **FAIL** |
| #2 both slots | ≤ sum + 10 % (454 ns) | 473 ns | **FAIL** |
| #3 scorer throughput | ≥ 100,000 flows/s | 232,450 flows/s; sampler 371 ns with or without the scorer slot | PASS |

Session 3 measured +192 / +228 ns on the same gates; the rate limiter's refill path is up
to five instructions longer since the remainder fix (36.3 insns/pkt, was 35.3).

### Known limitations

- **Interpreter cost**: ~3–7 ns per interpreted instruction on west (NOP 3.3, MOV 5.1,
  SYSCALL 5.5, LD 6.6; 4 steps per `bpf_loop` callback), so a 35-instruction cartridge
  cannot meet a 150 ns (or 120 ns) overhead bar. Gates #1(a) and #2 fail; #1(b) and #3
  pass. Options for later: fewer instructions per cartridge, more steps per callback
  (8: about −15 % per instruction for twice the verified callback), or a
  superinstruction/DOT4 ISA change ([ESCALATE], ISA spec gate).
- **Scorer** is trained on ten synthetic flow families only. Its held-out accuracy says
  nothing about real traffic; it ignores the SYN count and is not monotone in window bytes
  far outside the training range.
- **Not run live**: the Wotan leg of `anomaly.scores` and the Sophia registry backend
  (stack down, KF5); both proven against fakes / in-process.
- `GETPID_INJECT` stays off (pid numbering, below).
- The three-engine differential covers `cartridge-ebpf`, not the full `monad-cpu-ebpf`
  engine (privileged opcodes, ascend), which remains LICH-015's east arm.

### Open escalations

- **East as a node** — Step 177 (Mímir's Law Phases 9/11/13: heimdall-daemon as root from
  `/tmp` on east, XDP generic on `enp2s0`, scratch Wotan on the P2P link) and Step 435
  (DISTRIBUTED second node). Phases 6–8 and 18 deferred; nothing ran on east.
- **DOT4** — measured case in ADR-108 (≈1.7× scorer throughput); not needed for Gate #3;
  any new opcode is a full ISA spec gate in its own sprint.
- **RFC Editor items 1–5** (`docs/protocol/RFC-EDITOR-REQUESTS-S5.md`): Sophia root key
  (`x-cartridge`), Anamnesis sub-kind `0x43`, Wotan topics `anomaly.scores` /
  `upc.cart.*`, ADR-067 CSR range, KF10. If item 2 needs its own Event Type, that
  registration is [ESCALATE].
- **getpid pid numbering** — the injected getpid returns through the owned kernel, which
  answers xv6's `p->pid` (init = 1) while BPF answers its slot (0). Flipping the default
  needs one numbering (and probably the proc[] reconciliation EVOLUTION-1 parked).

### Stevie's decision list (S5, combined)

1. East as a node for Mímir's Law Phases 9/11/13 (Step 177): approve or keep deferred.
2. East as the DISTRIBUTED second node (Step 435), or drop Phase 18 for good (card
   Epic 1.5.6).
3. getpid: xv6 `p->pid` or the BPF slot numbering; then whether `GETPID_INJECT` defaults on.
4. ADR-108: accept once the RFC Editor answers items 1–5 (or keep Proposed).
5. DOT4 / any new MBC opcode: schedule an ISA spec-gate sprint or not.
6. Interpreter-cost gates #1(a) and #2: accept as the documented cost of interpreted
   cartridges, or fund follow-on work (callback batching, smaller cartridges, ISA).
7. Scorer: real labelled traffic before relying on its scores, or keep it a demonstrator.
8. Merge `sprint/upc-wire-compute` into `develop` (lead merges; Stevie pushes) and re-sign
   the unsigned commits (`scripts/resign-unpushed.py`).

### What the Lich sprint inherits

`tomb/lich/UPC-CARTRIDGE-HOOKS.md`: `cart diff`, `fuzz_cart_differential`,
`fuzz_cart_image`, `TestDifferential_ThreeEngines`, seeds in `tomb/lich/seeds/cartridge/`,
and the LICH-015 merge points (arm C, the MULHU seed closes: `mulhu-probe` converges on
this branch, re-run after the merge).
