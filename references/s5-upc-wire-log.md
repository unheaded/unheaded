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

**Other:** next free ADR number is **ADR-106** (ADR-105 is taken; `ADR_CART=ADR-106` for
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
  verified, nothing new. `go test -race` + `go vet` (linux, darwin) green, gosec clean,
  `go.mod` unchanged. golangci-lint panics on this tree (known version trap), not run.
- Commits: `b6f5ec8a` (BPF), `930b1ddc` (daemon).

## Phase 9 — cartridge ADR + spec gate (2026-10-23)

- Marshal entry ruling: Part 1 exit (Step 228) is not passed because Phases 6–8 are
  deferred; the lead session authorised Part 2 on the Phases 0–5 evidence, and Step 228
  itself says east-only items do not block Part 2.
- D-C1…D-C5 taken as recommended. ADR-106 (Proposed) carries the Cartridge ABI v0, grounded
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
- ADR-106 number: free on every local sprint branch at forge time; a parallel branch could
  still claim it before merge.
