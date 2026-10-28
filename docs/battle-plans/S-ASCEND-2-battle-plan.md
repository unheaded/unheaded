<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-ASCEND-2 BATTLE PLAN: 14 Phases, 314 Steps

**Date**: 2026-10-27
**Sprint**: Sprint 6 of 7 (index: `NEXT-7-SPRINTS-2026-10-27.md`). The owned kernel takes its first live syscall, the isolation proof becomes N-way, the loader's promised gates exist, the five required-before-prod items land, EVOLUTION-1 Stage 2 is re-planned against the measured verifier delta, and east-as-a-node is run or formally dropped. No Doom work (Stevie 2026-10-27: Doom is the lowest priority of any work; see `S-DOOM-LAST`).
**Prerequisite**: branch based on develop `a7fe3acd` or newer with S-CLEAN-SLATE's exit gates passed (CI green, `go build ./...` clean); S-TRUE-WIRE's M-184 verifier ratchet is recommended first but not required (Phase 0 pins the budgets by hand if it is absent). `scripts/upc-regression.sh` ALL GREEN at start with ascend **926,542** and Doom **761,580** verified insns, goldens 12/12 SAME against `$HOME/tmp/golden`. RISC-V cross toolchain (`riscv64-unknown-elf-gcc`), nightly Rust with `bpfel-unknown-none`, passwordless sudo on west.
**Target**: `GETPID_INJECT` has a decided value backed by a child-process probe and recorded pid numbering; `gate_nway` prints 3× `NWAY-ok` and a looped-fork gate passes, both gated by the harness; `upc_api::load()` performs the surface-vs-object and per-pid slice-overlap gates; `upc-bootctl` boots as its own nologin user with CAP_BPF+CAP_NET_ADMIN, under seccomp, behind `UPC_DEMO_MODE`, with the `.rv2mbc` SHA gate default-on and veth-upc0p forward-dropped; ROM_MAP and RV2MBC_MAP are frozen after load; device inodes open; the Mímir 9/11/13 and DISTRIBUTED legs ran on east or on a two-netns west fallback with their topology named honestly; EVOLUTION-1 Stage 2 has a decided reach and a skeleton plan; every budget and golden still green.
**Estimated Duration**: 30-38 h of execution across 6-8 sessions. Six waits on Stevie (E1-E6 below); every chain not behind a wait continues.
**Agent Strategy**: Coordinator owns Phases 0, 1, 3, 5, 6, 7, 8 (sudo, XDP, netns, iterative verifier work). Phases 2, 4, 9, 10, 11 are delegable; Phase 4 ∥ Phase 3, Phase 9 ∥ Phase 8, Phase 10 ∥ Phase 9 (Appendix B). Parallel agents work in their own git worktree off the sprint branch and never run `sudo` or touch `ebpf/monad-cpu-ebpf` (one eBPF writer at a time: the verifier budget is shared state). Hosts are read-only for every agent unless the step is covered by an answered [ESCALATE].
**Commit Cadence**: every 5 steps (max(3, min(5, 314/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Signed if `git commit-tree -S` succeeds in Phase 0, otherwise `--no-gpg-sign` (memory: feedback_unsigned_commits_when_afk) and `scripts/resign-unpushed.py` later. Every commit ends with the trailers `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE`.
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B dependency map). eBPF steps have a third stop: any step that leaves the ascend object over **950,000** verified insns is reverted before the next step, whatever its own gate says.

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| `docs/sweeps/OUTSTANDING-2026-10-27.md` E14 (M-220..M-231), E13 (M-202, M-204), decisions 44-49 | Scope, priority, the exact "what remains" per item, and the sweep defaults used in every [DECIDE]. Appendix D maps each M-id to steps. |
| `docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md` (D1=(c) hybrid, D2 yes, D3 Stage 1 first; Stage 1 status 2026-10-23) | The reach rule ("sign of the getpid verifier-delta tunes the reach: net-negative → migration ladder is self-funding"), the Stage 2 ladder order, the dual-books diagnosis, "next probe: a child process that calls getpid with the flag ON". |
| `references/s5-upc-wire-log.md` §Phases, §Known limitations, §Open escalations, §Stevie's decision list | Baselines (ascend 926,542 / Doom 761,580 / cart_xdp 42,743 / cart_tick 146,897), flag ON 911,319, Steps 177/435 wording, gates #1(a)/#2 numbers, scorer caveats, KF1. |
| `docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md` Phases 0, 3, 6-8, 18 | Helper functions (`kbuild`, `ubuild`, `sweep`, `commitp`, sourced verbatim), the Mímir Phase 9/11/13 step shapes, the two-netns fallback wording, the `GETPID_INJECT` flip recipe. |
| `docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md` Epics 1.2.4/1.2.5, 1.3.1-1.3.5 | The two I/O-time loader gates and their open design questions; FS substrate remainders (device inodes, writes + `>`, pipes). |
| `references/security-upc-linux-spitball-2026-05-13.md` §four isolation items + §Phase 1.3 prerequisites | The five required-before-prod items (forward-drop, unprivileged bootctl, seccomp, `UPC_DEMO_MODE`, RV2MBC SHA) exactly as written. |
| ADR-080 (Accepted), ADR-081 (Accepted), ADR-074 (Accepted), ADR-043 (status line stale), ADR-108 (Proposed), ADR-023 (Deferred) | Multi-workload contract; own-kernel direction; per-pid pgd model (slice stride 8 MiB); the "13 phases shipped" claim this plan corrects; cartridge ABI; the ADR this plan marks superseded. |
| `tomb/lich/LICH-012-config-convergence/README.md`, `tomb/lich/LICH-015-mbc-differential/README.md`, `tomb/lich/UPC-CARTRIDGE-HOOKS.md` | L12a (key exfiltration not attempted) and L12d (not run) targets; the full-engine arm; `cart diff`, `TestDifferential_ThreeEngines`, `fuzz_cart_differential` commands. |
| Code read 2026-10-27 at `a7fe3acd` | `main.rs:413` `GETPID_INJECT=false`; `main.rs:644-650` instance from flow-label low byte, unvalidated; `workload.rs:178-182` the two gates "named in the docs but not performed here"; `pkg/ebpf/mapfreeze.go:122` `NewLoadSequence` with test-only callers; `cmd/doom/main.go:85` and `cmd/wotan-ctl/doom.go:56` literal `unheaded-doom-secret`; `bootparams.rs:10` `BOOT_VERSION = 2`; `main.rs:634` SHA gate only when `UPC_RV2MBC_SHA` is set; `netns.rs:15-16` `veth-upc0`/`veth-upc0p`; `riscv.h` CSR helpers typed `uint64` = `unsigned long` (4 bytes under ilp32e); `user/gate_nway.c`, `user/getpidprobe.c` exist; `heimdall-daemon` flags `--xdp-iface/--xdp-mode/--wotan-grpc/--drift-sign-key`; `cmd/upc-cartridgectl` has `--sophia URL`; `cmd/anomaly --wotan`. |

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification step (MUST pass before proceeding)
[D]            Debug step (only if the prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo/elevated privileges required
[P]            Parallelizable with other [P] steps in the same phase
[P:N]          Parallelizable with Phase N
[SEQ]          MUST be sequential
[C]            Commit checkpoint
[ENV]          Environment setup/check
[BUILD]        Build/compilation step
[TEST]         Test execution
[CODE]         Code implementation (test first: red, then green)
[DESIGN]       Design decision recorded in-repo
[STUCK]        Step skipped via Skip Protocol (needs human)
[BLOCKED]      Blocked by an upstream [STUCK] (never used for decisions)
[DECIDE]       Decision with a pre-seeded RECOMMENDATION; proceed without asking
[ESCALATE]     Needs Stevie. STOP this chain; continue independent chains
[AFTER-E1]     Runs only once E1 is answered; skipped (not STUCK) until then
[PREFLIGHT]    Hypothesis verification (Phase 0 only)
[REGEN]        Regenerate derived artifacts
[AUDIT-UPDATE] Mark a finding resolved in its tracking doc
[DOC-UPDATE]   Update downstream docs (wiki, CLAUDE.md, next.md)
[SECURITY]     Security review step
[COMPLIANCE]   License/SBOM/dependency check
[BARE-METAL]   Requires the real hosts (west/east), real kernel, real LXD
[LIVE-RO]      Live read-only verification on west/east (GET, list, show)
[LIVE-RW]      Live host-changing step; only under an answered [ESCALATE]
```

**[DECIDE] format**: RECOMMENDATION, Rationale, "Override ONLY if". The agent proceeds with the recommendation.
**[ESCALATE] format**: Context, the exact question(s), "Why no recommendation" (or why the call is Stevie's even with one). The agent stops that chain only.
**Time tags**: `~Nm` is expected wall-clock. 3x over = Skip Protocol.
**Budget tag** (this plan only): every step that rebuilds `monad-cpu-ebpf` records `verified N insns` for the ascend variant in the sprint log before the next step. Ceiling 1,000,000; working limit 950,000; any step above the working limit is reverted.

**DoD-STD (every phase's Definition of Done includes these; Micromanager gate):**
1. Every step executed or STUCK-logged in this file.
2. Every [V] in the phase passed.
3. Zero NEW test failures versus the KNOWN FAILURES BASELINE (`go test ./...`, `cargo test` per touched crate, plus `-race` on touched Go packages).
4. Commit checkpoint made; tree clean (`git status --short` empty).
5. No new dependency without a Barrister/ADR-004 check (`go mod tidy -diff` empty, `cargo tree` unchanged for new crates, `scripts/check-*` green).
6. Attack surface unchanged, or the change is listed in Phase 12's review table.
7. Every claim of "real" or "live" in a commit message has a [LIVE-RO] or [LIVE-RW] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; probes read metadata only.
9. (UPC) `scripts/upc-regression.sh` ALL GREEN and goldens 12/12 SAME at every exit gate; ascend and Doom insn counts recorded; `.mbc`/`.rv2mbc`/`.data` byte-identical unless the phase changed the translator or the kernel source, in which case the new hashes and the reason are in the sprint log.

---

## SITUATION (verified at `a7fe3acd`, 2026-10-27)

- **Dream Ladder**: L5 complete (2026-07-03); kernel 100 % Unheaded-authored (2026-07-07, `PHASE24-KERNEL-CORE.md`); L6 not started. `wiki/UPC-Dream-Ladder.md:16` still says the flip was deferred for "+5409 verifier insns" (July number; S5 measured −15,223).
- **getpid**: `ebpf/monad-cpu-ebpf/src/main.rs:413` `const GETPID_INJECT: bool = false;`, used at `:2487`. S5 Phase 3 proved the round trip with the flag ON; flag ON prints `GP: pid=1` (kernel `allocpid`), flag OFF `GP: pid=0` (BPF slot). Flag ON verified 911,319 vs OFF 926,542. The named next probe (a BPF-forked child calling getpid with the flag ON) has not been run; `user/getpidprobe.c` exists, no forking variant does.
- **EVOLUTION-1**: D1=(c) hybrid, decided 2026-07-17 with the rule "net-negative delta → ladder self-funding, push toward (a)". The delta is now negative and nobody has re-read D1 against it. Stage 2 has no plan ("do NOT forge steps now" — the brief asked for a skeleton once Stage 1 measured).
- **Isolation**: `user/gate_nway.c` (3-way concurrent, unrolled) has been KF1 `NWAY-FAIL` since Phase 1.7 Gate A; `gate2` (sequential) is the only isolation proof in the harness (`scripts/upc-regression.sh:95-96`). The looped-fork substrate bug its comment names (a `for` around `fork()` re-enters and fork-bombs) has no gate.
- **Loader**: `crates/upc-api/src/workload.rs:178-182` says the surface-vs-loaded-object feature check and per-pid MMU slice non-overlap "are named in the docs but not performed here"; nothing performs them anywhere. Master plan Epic 1.2.5 "formal close waits on" them.
- **Required-before-prod (2026-05-13 brief)**: 0/5. `upc-bootctl` runs under `sudo` (`scripts/upc-regression.sh:63,66`), creates `upc0` and the veth pair itself (`netns.rs:23-67`, `ip` via `Command::new`), has no seccomp profile, no `UPC_DEMO_MODE`, and the `.rv2mbc` SHA gate runs only when `UPC_RV2MBC_SHA` is set (`main.rs:634-647`); `BOOT_VERSION = 2` (`bootparams.rs:10`). No `unheaded-upc` account in `deploy/sysusers.d/unheaded.conf`; no UPC unit in `deploy/systemd/`. No forward-drop rule for `veth-upc0p` anywhere (`deploy/nftables/east.nft`, `scripts/host/nft-apply-confirm.sh` are the only nft paths).
- **Map freeze**: `pkg/ebpf/mapfreeze.go:122` `NewLoadSequence` has only `_test.go` callers; neither Rust loader (`upc-api` via `upc-bootctl` / `doom-runner`) freezes `ROM_MAP`/`RV2MBC_MAP` (no `freeze` in `crates/upc-api/src`, `crates/doom-runner/src`, `cmd/upc-bootctl/src`). `cmd/doom/main.go:85` and `cmd/wotan-ctl/doom.go:56` ship the literal HMAC key `unheaded-doom-secret`. `main.rs:644-650` takes `instance = flow_label & 0xFF` with no ownership check.
- **FS substrate**: `T_DEVICE` never handled in the open path; FS writes and `>` not started (no block alloc); pipes not started (master plan 1.3.1-1.3.3). `kernel/riscv.h` CSR helpers return `uint64` = `unsigned long` = 4 bytes under ilp32e (`kernel/types.h:8`), so "64-bit" CSR reads have undefined high bits by construction.
- **East**: never contacted in S5. Steps 177 and 435 are [ESCALATE]s; Phases 6-8 and 18 DEFERRED. ADR-043:3 still reads "All 13 phases shipped". `cmd/gjallarhorn-sender`/`-listener` carry `ff02::1:abba`; multicast never ran end to end on L2.
- **Cartridges**: the Wotan leg of `anomaly.scores` (`cmd/anomaly --wotan`) and the Sophia registry backend (`cmd/upc-cartridgectl --sophia`) were proven against fakes only; the scorer (`cartridges/scorer.model.json`) is trained on ten synthetic families; gates #1(a) +193/+186 ns (bar 150) and #2 +216 ns (bar 120) FAIL on interpreter cost; `steel-echo` (S-REAL-STEEL) now produces real flows on west.
- **Lich**: LICH-015 differential covers `cartridge-ebpf`, not the full `monad-cpu-ebpf` (privileged opcodes, ascend); LICH-012 L12a "key exfiltration from dev key storage: not attempted", L12d "not run (needs sudo and a live attach)".
- **Naming**: working name `unheadnix` chosen 2026-07-17, ceremony never held; 13 doc/wiki files say "Unheaded Linux"; ADR-023 still "Deferred to L5"; `crates/upc-bootstub` (stage-1 stub for uClinux, CLAUDE.md:1360) survives ADR-081's decision not to vendor uClinux.
- **Known failures baseline** (re-pinned in Phase 0): KF1 `gate_nway` → `NWAY-FAIL`. `go build ./...` clean; `go mod tidy -diff` empty.

### Escalations

| ID | Question (exact) | Blocks | Fallback while open |
|---|---|---|---|
| E1 | pid numbering: xv6 `p->pid` (init = 1) or the BPF slot (init = 0)? Recommendation: kernel numbering (EVOLUTION-1 direction; master decision 44). | Steps 46-52 (flip + reconciliation) | Probe evidence gathered; flag stays `false`; everything else continues |
| E2 | Re-read D1's reach given the negative delta: push toward (a) kernel-authoritative, or keep (c) minimal? Recommendation: push toward (a) per EVOLUTION-1's own rule. | Steps 63-66 (Stage 2 skeleton acceptance) | Skeleton written as a draft marked "pending E2" |
| E3 | East as a node: approve Steps 177 (Mímir 9/11/13: heimdall-daemon as root from /tmp on east, XDP generic on `enp2s0`, scratch Wotan on the P2P link) and 435 (DISTRIBUTED second node), or drop Phase 18 for good? No recommendation: east runs the debs; host actions there are Stevie's. | Steps 233-238 (east column) | Two netns on west, result named "cross-node demonstrated in netns, not across hosts" |
| E4 | Naming ceremony: is `unheadnix` the name? Lore to bless; one find-replace of "Unheaded Linux" follows. No recommendation: names are Stevie's and Lore's. | Steps 284-286 | ADR-023/ADR-081 status edits that do not depend on the name proceed |
| E5 | One attended run: browser xterm end-to-end with live keyboard, and `LICH_E2E=1` against the live llama-server (model swap through `switch-model.sh`). Needs eyes and a GPU window. | Steps 289-291 | Recipe written and dry-run; marked "not run live" |
| E6 | West host window for UPC isolation: install `unheaded-upc` sysuser, the `unheaded-upc-netns` oneshot unit, and the `veth-upc0p` forward-drop rule on west (all reversible, listed in Step 154). Recommendation: yes; west already runs bootctl under sudo for the harness. | Steps 149-154 ([LIVE-RW]) | Everything proven in a throwaway netns under sudo; unit files committed, not installed |

---

## PHASE 0: PREFLIGHT, BASELINE, HYPOTHESES (Steps 1-26)

**Goal**: Prove the base is green, pin every budget and golden, verify every SITUATION claim live, set up the sprint log and helpers. Cites M-220..M-231 (baseline for all).
**Prerequisite**: develop at `a7fe3acd` or newer; S-CLEAN-SLATE exit gates passed.
**Time**: ~1.5 h
**Agent**: Coordinator

### Environment and helpers

- [x] **Step 1** *(branch cut from sprint/true-wire 31346cbf in the built worktree, not develop: the M-184 ratchet lives there; rebase if E2 reverts)* [ENV][B] ~2m: Branch, worktree, sprint scratch dir, sprint log
  ```bash
  cd ~/tmp/unheaded && git checkout develop && git pull --ff-only 2>/dev/null; git checkout -b sprint/ascend-2 && mkdir -p "$HOME/tmp/s-ascend-2" && \
  printf '# S-ASCEND-2 sprint log\n\nStarted 2026-%s on %s\n\n## Baselines\n' "$(date +%m-%d)" "$(git rev-parse --short HEAD)" > references/s-ascend-2-log.md && git status --short | head -3
  ```
- [x] **Step 2** [ENV][B] ~1m: Source the S5 helpers (`PROJECT_ROOT`, `EBPF`, `UP`, `K`, `BC`, `DR`, `kbuild`, `ubuild`, `sweep`, `commitp`) and add this sprint's names. Put this in every new shell.
  ```bash
  cd "$(git rev-parse --show-toplevel)" && source <(sed -n '/^export PROJECT_ROOT/,/^commitp/p' docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md) && \
  export S6="$HOME/tmp/s-ascend-2" GOLD="$HOME/tmp/golden" LOG="$PROJECT_ROOT/references/s-ascend-2-log.md" && type kbuild ubuild sweep commitp | grep -c 'is a function'
  ```
- [x] **Step 3** [V] ~30s: Prints `4`. Missing → the S5 plan's Phase 0 block moved: `grep -n '^export PROJECT_ROOT' docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md` and fix the `sed` range; two attempts then STOP (every later step uses these names).
- [x] **Step 4** [ENV][B] ~2m: Toolchain presence
  ```bash
  for t in riscv64-unknown-elf-gcc cargo rustup bpftool ip nft jq python3 go sudo; do command -v "$t" >/dev/null && echo "$t: OK" || echo "$t: MISSING"; done; rustup target list --installed | grep -c bpfel-unknown-none; go version; test -d "$GOLD" && ls "$GOLD" | wc -l || echo "GOLD MISSING"
  ```
- [x] **Step 5** [V] ~30s: No `MISSING`, bpfel target count ≥ 1, `$GOLD` holds 12 goldens. `GOLD MISSING` → recapture per `references/phase24-t1-2026-07-07.md` §"Golden recapture" from this HEAD before anything else (the goldens are the equivalence oracle; without them no kernel step may proceed).
- [x] **Step 6** [ENV][B] ~1m: Signing probe (sets the commit flag for the whole sprint)
  ```bash
  cd "$PROJECT_ROOT" && if git commit-tree -S -m probe "$(git rev-parse HEAD^{tree})" >/dev/null 2>&1; then echo "GPG: signing works"; else echo "GPG: not cached -> commitp uses --no-gpg-sign"; fi
  ```

### Known failures baseline

- [x] **Step 7** [BUILD][B] ~3m: Go and Rust build baselines
  ```bash
  cd "$PROJECT_ROOT" && go build ./... && echo GO-BUILD-OK && go mod tidy -diff | head -3 && (cd crates/monad-mbc && cargo build --release 2>&1 | tail -1) && (cd cmd/upc-bootctl && cargo build --release 2>&1 | tail -1) && (cd crates/doom-runner && cargo build --release 2>&1 | tail -1)
  ```
- [x] **Step 8** [TEST][B] ~12m: Go test baseline into the log (the KNOWN FAILURES BASELINE)
  ```bash
  cd "$PROJECT_ROOT" && go test ./... 2>&1 | grep -E '^(FAIL|ok|---)' | grep -v '^ok' | tee "$S6/p0-go-fail.txt" | tail -10; printf '\n- go test failures at start: %s lines (file p0-go-fail.txt)\n' "$(wc -l < "$S6/p0-go-fail.txt")" >> "$LOG"
  ```
- [x] **Step 9** [BUILD][B][S] ~6m: Kernel + userland build, full golden sweep
  ```bash
  kbuild >/dev/null 2>&1; ubuild >/dev/null 2>&1; sweep | tee "$S6/p0-sweep.txt" | grep -cE 'SAME'
  ```
- [x] **Step 10** *(8/8 SAME: the S5 sweep checks 8 golden sets; blank/multi/nosuch/uinit_echo are checked by upc-regression.sh, which passed)* [V] ~30s: Prints `12` (12/12 SAME). Fewer → the tree is not the golden tree: `git status --short` must be empty and `git log -1` must be at/after `a7fe3acd`; otherwise STOP.
- [x] **Step 11** [TEST][B][S] ~8m: Regression harness with budgets
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p0-regression.log" | grep -aE 'PASS|FAIL|verifier'
  ```
- [x] **Step 12** [V] ~1m: Zero `FAIL`; `ascend verifier: verified 926542 insns`; `Doom verifier: verified 761580 insns`; cartridge gates under 500,000. Record all three in `$LOG` under Baselines. Different numbers but all PASS → record the new baseline with the toolchain versions (`rustc -V`, `cargo -V`) and continue; any FAIL → STOP (S-CLEAN-SLATE's gate was supposed to leave this green).
- [x] **Step 13** [TEST][B][S] ~3m: KF1 reproduced (gate_nway)
  ```bash
  (cd "$UP" && make -f ../adapters/Makefile.mbc-userland target/gate_nway.mbc 2>&1 | tail -1); out="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/gate_nway.mbc" --triggers 1500000 --instance 111 2>&1)"; printf '%s\n' "$out" > "$S6/p0-nway.txt"; grep -aoE 'NWAY-(ok|FAIL)[^\n]{0,8}' <<<"$out" | sort | uniq -c
  ```
- [x] **Step 14** [V] ~30s: At least one `NWAY-FAIL` (KF1 stands) — record the exact counts (`ok` per pid, `FAIL` count) in `$LOG`. If 3× `NWAY-ok` and 0 FAIL: KF1 has silently healed; record it, mark Steps 67-80 as "verify only" and keep the harness-gating steps 85-94.

### Hypotheses (one [PREFLIGHT] per SITUATION claim)

- [x] **Step 15** [PREFLIGHT][B] ~1m: H1 flag + probe; H2 loader gates clause; H3 freeze callers; H4 HMAC literal; H5 SHA gate env-only
  ```bash
  cd "$PROJECT_ROOT" && grep -n 'const GETPID_INJECT' ebpf/monad-cpu-ebpf/src/main.rs; ls crates/xv6-mbc/upstream/user/getpidprobe.c; grep -n 'named in the docs but not performed' crates/upc-api/src/workload.rs; grep -rn 'NewLoadSequence(' --include=*.go . | grep -vc _test; grep -rn 'unheaded-doom-secret' --include=*.go cmd | wc -l; grep -n 'UPC_RV2MBC_SHA' cmd/upc-bootctl/src/main.rs | head -2; grep -n 'BOOT_VERSION' cmd/upc-bootctl/src/bootparams.rs | head -1
  ```
- [x] **Step 16** [V] ~30s: H1 `= false` + probe exists; H2 clause present; H3 non-test callers `0`; H4 `2`; H5 env-gated line present; `BOOT_VERSION: u32 = 2`. Any mismatch → that SITUATION line is stale: re-read the code path, amend SITUATION and the affected steps before continuing (Marshal: plan follows reality).
- [x] **Step 17** [PREFLIGHT][B] ~1m: H6 bootctl creates netns itself (needs root); H7 no seccomp/demo gate; H8 flow-label instance unvalidated; H9 no upc sysuser/unit/nft rule
  ```bash
  cd "$PROJECT_ROOT" && grep -nE '"netns", "add"|Command::new\("ip"\)' cmd/upc-bootctl/src/netns.rs | head -3; grep -rnE 'UPC_DEMO_MODE|seccomp|SystemCallFilter' cmd/upc-bootctl deploy/systemd 2>/dev/null | wc -l; sed -n '644,650p' ebpf/monad-cpu-ebpf/src/main.rs | grep -n 'flow_label & 0xFF'; grep -c 'unheaded-upc' deploy/sysusers.d/unheaded.conf; ls deploy/systemd | grep -c upc; grep -rn 'veth-upc0p' deploy/nftables scripts/host 2>/dev/null | wc -l
  ```
- [x] **Step 18** [V] ~30s: netns lines present; `0` seccomp/demo hits; `flow_label & 0xFF` present; `0` sysuser, `0` units, `0` nft hits. Mismatch → amend as in Step 16.
- [x] **Step 19** [PREFLIGHT][B] ~1m: H10 T_DEVICE unhandled; H11 riscv.h uint64 helpers; H12 ADR-043 claim; H13 "Unheaded Linux" count; H14 bootstub crate present
  ```bash
  cd "$PROJECT_ROOT" && grep -rn 'T_DEVICE' crates/xv6-mbc/adapters/*.c ebpf/monad-cpu-ebpf/src/main.rs | grep -vi 'define' | wc -l; grep -c 'static inline uint64' crates/xv6-mbc/upstream/kernel/riscv.h; grep -n 'typedef unsigned long uint64' crates/xv6-mbc/upstream/kernel/types.h; grep -c '13 phases shipped' docs/adr/ADR-043*.md; grep -rl 'Unheaded Linux' docs wiki CLAUDE.md README.md | wc -l; ls crates/upc-bootstub/Cargo.toml
  ```
- [x] **Step 20** *(H10 amended: usysfile.c handles T_DEVICE in sys_open; the BPF open path does not)* [V] ~30s: T_DEVICE handled `0` (or only in a define); uint64 helper count ≥ 10; typedef present; ADR-043 `1`; "Unheaded Linux" `13`±2; bootstub present. Mismatch → amend.
- [x] **Step 21** [PREFLIGHT][LIVE-RO][B] ~2m: East reachability and west UPC leftovers (read-only)
  ```bash
  ssh -o ConnectTimeout=5 east 'hostname; uptime -p; ip -br link show enp2s0' 2>&1 | head -3; sudo bpftool prog show 2>/dev/null | grep -ciE 'monad|cart|heimdall' ; ip netns list | grep -c upc0; sudo nft list ruleset 2>/dev/null | grep -c veth-upc0p
  ```
- [x] **Step 22** [V] ~30s: Record east up/down in `$LOG` (E3 context). West: `0` UPC programs attached, `0` or `1` `upc0` netns (the harness tears down; `1` means a leftover: `sudo ip netns del upc0` after confirming no bootctl runs), `0` nft hits.
- [x] **Step 23** [PREFLIGHT][B] ~2m: Cartridge live-leg surfaces exist (H15)
  ```bash
  cd "$PROJECT_ROOT" && go run ./cmd/anomaly --help 2>&1 | grep -E '^\s+-wotan' | head -1; go run ./cmd/upc-cartridgectl --help 2>&1 | grep -E 'sophia' | head -2; ls cartridges/scorer.model.json cartridges/scorer.data.bin; ls cmd/steel-echo/load.go
  ```
- [x] **Step 24** [V] ~30s: `-wotan` and `--sophia` flags present; scorer files and `steel-echo` present. Missing → the master list's M-229 premise is wrong; amend and re-scope Phase 9.
- [x] **Step 25** [W] ~5m: Write the Baselines + Hypotheses table into `$LOG` (budgets, KF1 counts, H1-H15 verdicts, east state, GPG state) and the KF table: `KF1 gate_nway NWAY-FAIL`.
- [x] **Step 26** [V][C] ~2m: **PHASE 0 EXIT GATE** — Steps 10, 12 green; KF1 reproduced or recorded healed; H1-H15 all match or SITUATION amended; log committed.
  ```bash
  cd "$PROJECT_ROOT" && git add references/s-ascend-2-log.md docs/battle-plans/S-ASCEND-2-battle-plan.md && commitp -m "docs(upc): [S-ASCEND-2 1-26] preflight baseline, hypotheses, KF table"
  ```
  - If pass → Phase 1. If fail → DO NOT PROCEED; fix within Phase 0.

---

## PHASE 1: THE CHILD-GETPID PROBE AND THE PID-NUMBERING DECISION (Steps 27-52)

**Goal**: Run the probe EVOLUTION-1 named, put the dual-books evidence in front of Stevie as E1, and flip `GETPID_INJECT` with a reconciled numbering once answered. M-220, decision 44.
**Prerequisite**: Phase 0 gate.
**Time**: ~3 h (plus the E1 wait)
**Agent**: Coordinator (sudo, eBPF)

### The named probe: a forked child calls getpid

- [x] **Step 27** *(probe parent spins at the end: init's exit returns to user code, see log)* [W][CODE] ~15m: `crates/xv6-mbc/upstream/user/getpidfork.c` — gate2-style fixed-string probe: parent prints `GF: parent-pid=N` (N∈{0,1,2,other} via if-chain), forks ONCE (no loop: the looped-fork bug), child prints `GF: child-pid=N` then `exit(0)`, parent `wait`s and prints `GF: parent-after-wait`. Header comment: purpose (EVOLUTION-1 "next probe"), observables, and that both flag states must print the same lines except the pid values only the kernel path can reveal.
- [x] **Step 28** *(phony targets `getpidprobe` and `getpidfork`)* [W] ~3m: Register it in `crates/xv6-mbc/adapters/Makefile.mbc-userland` next to `getpidprobe` (same recipe shape; check how `getpidprobe.mbc` is listed first: `grep -n getpidprobe ../adapters/Makefile.mbc-userland`).
- [x] **Step 29** [BUILD][B] ~3m: Build the probe, flag OFF object (as built)
  ```bash
  (cd "$UP" && make -f ../adapters/Makefile.mbc-userland target/getpidfork.mbc 2>&1 | tail -2); ls -la "$K/getpidfork.mbc"; grep -n 'const GETPID_INJECT' "$EBPF/monad-cpu-ebpf/src/main.rs"
  ```
- [x] **Step 30** *(800k triggers, sudo-side UPC_VERIFIER_STATS; see log deviations)* [TEST][B][S] ~3m: Control run, flag OFF
  ```bash
  out="$(cd "$EBPF" && UPC_VERIFIER_STATS=1 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/getpidfork.mbc" --triggers 2500000 --instance 111 2>&1)"; printf '%s\n' "$out" > "$S6/p1-fork-off.txt"; grep -aoE 'GF: [a-z=0-9-]+|verified [0-9]+ insns|DEAD' <<<"$out"
  ```
- [x] **Step 31** *(0 / 1 / after-wait)* [V] ~1m: Prints `GF: parent-pid=0`, `GF: child-pid=1` (BPF slot numbering: init slot 0, first child slot 1), `GF: parent-after-wait`, no `DEAD`. A different child slot is fine if consistent; record. Missing `parent-after-wait` → Step 32.
- [x] **Step 32** *(two attempts: the doubling was init's exit returning, not the probe's fork; probe spins)* [D] ~15m: The probe itself is wrong, not the kernel: compare with `gate2.c`'s fork/wait shape (it works in the harness); check `wait()` is the xv6 `wait(int*)` signature in `user/user.h`; rebuild. Two attempts then STUCK (Phase 1 cannot proceed without a working probe; Phases 2-4 do not depend on it).
- [x] **Step 33** [BUILD][B] ~3m: Flip the flag ON for the experiment (temporary), rebuild ascend
  ```bash
  sed -i 's/const GETPID_INJECT: bool = false;/const GETPID_INJECT: bool = true;/' "$EBPF/monad-cpu-ebpf/src/main.rs" && (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1)
  ```
- [x] **Step 34** *(parent 1, child -1, after-wait)* [TEST][B][S] ~3m: Probe run, flag ON
  ```bash
  out="$(cd "$EBPF" && UPC_VERIFIER_STATS=1 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/getpidfork.mbc" --triggers 2500000 --instance 111 2>&1)"; printf '%s\n' "$out" > "$S6/p1-fork-on.txt"; grep -aoE 'GF: [a-z=0-9-]+|verified [0-9]+ insns|DEAD' <<<"$out"
  ```
- [x] **Step 35** *(table in log: the kernel has no process for the child, getpid -1)* [V] ~2m: **DUAL-BOOKS OBSERVED OR REFUTED** — record the four `GF:` lines ON vs OFF side by side in `$LOG`. Expected by read: ON prints `parent-pid=1` and `child-pid=1` (the child enters the kernel with `cpu->proc` = init, so the kernel answers init's pid). Any outcome is informative; what matters is the table. No `parent-after-wait` with the flag ON → Step 36.
- [x] **Step 36** *(not needed: parent-after-wait printed with the flag ON)* [D] ~20m: The injected trap broke fork/wait interplay: dump CPU state at the DEAD/halt (`--trace`-style dump in bootctl), check `sepc`/`sscratch` after the child's SRET (the S1.2 class of bug: CSR temp aliasing in the translator, see ADR-067 Decision 2 note). Two attempts then record, restore the flag (Step 38) and STUCK Steps 46-52.
- [x] **Step 37** *(ON 911319, OFF 926542, delta -15223)* [B] ~1m: Verifier delta ON vs OFF on this toolchain (feeds Phase 2)
  ```bash
  on="$(grep -aoE 'verified [0-9]+ insns' "$S6/p1-fork-on.txt" | grep -oE '[0-9]+')"; off="$(grep -aoE 'ascend verifier: verified [0-9]+' "$S6/p0-regression.log" | grep -oE '[0-9]+$')"; echo "ON=$on OFF=$off DELTA=$((on-off))" | tee -a "$LOG"
  ```
- [x] **Step 38** *(byte-identical)* [B] ~3m: Restore the flag OFF (the default until E1), rebuild, confirm byte-identical object vs Phase 0
  ```bash
  sed -i 's/const GETPID_INJECT: bool = true;/const GETPID_INJECT: bool = false;/' "$EBPF/monad-cpu-ebpf/src/main.rs" && (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1) && git -C "$PROJECT_ROOT" status --short ebpf/ | wc -l
  ```
- [x] **Step 39** *(0)* [V] ~30s: `0` modified files under `ebpf/`. Non-zero → `git diff ebpf/` and revert anything but the probe files.
- [ ] **Step 40** [C] ~1m: **COMMIT CHECKPOINT** (probe + Makefile line + log; flag unchanged)
  ```bash
  cd "$PROJECT_ROOT" && git add crates/xv6-mbc/upstream/user/getpidfork.c crates/xv6-mbc/adapters/Makefile.mbc-userland references/s-ascend-2-log.md && commitp -m "test(upc): [S-ASCEND-2 27-40] getpidfork probe: forked child calls getpid under both GETPID_INJECT states"
  ```

### Numbering design and the escalation

- [ ] **Step 41** [R] ~15m: Read the two books: BPF `SYS_FORK` slot allocation and the `PROC_TABLE` layout (`grep -n 'STAT_FORKS\|PROC_TABLE\|fn sys_fork\|slot' ebpf/monad-cpu-ebpf/src/main.rs | head -40`) and the kernel's `allocpid`/`allocproc` in `crates/xv6-mbc/adapters/uproc.c`. Note where each stores its pid and whether `PROC_TABLE` has a free u32 the kernel could read.
- [ ] **Step 42** [DESIGN][W] ~25m: `references/s-ascend-2-log.md` §"pid reconciliation design": two options with cost. **(K) kernel numbering wins**: BPF `SYS_FORK` writes `pid = slot + 1` (init = 1) into the slot's PROC_TABLE pid field and BPF getpid/wait answer from that field; kernel `allocpid` is replaced by reading the same field (one source of truth, no counter). **(S) slot numbering wins**: kernel `allocpid` returns the BPF slot index via PROC_TABLE MMIO. Record which gate2/gate_nway/uinit assumptions depend on init = 0 (`grep -n 'pid' user/gate2.c user/uinit.c user/gate_nway.c`), the verifier cost estimate of each (K touches one BPF store; S touches none in BPF), and the Scientist caveat (getpid is a biased estimator; this decides identity, not migration cost).
- [ ] **Step 43** [ESCALATE] ~—: **E1 — pid numbering.** Context: Step 35 table; Step 42 design. Questions: (1) kernel `p->pid` (init = 1) or BPF slot (init = 0)? (2) if kernel: option K as designed? Recommendation (Computermancer): **kernel numbering, option K** — EVOLUTION-1's direction is kernel authority, and K removes the kernel's counter rather than adding a second mapping. Why it is still Stevie's: it changes the observable pid of every userland program the goldens capture, so the 12 goldens are recaptured (an equivalence break by policy). **Until answered: Steps 44-52 skip (not STUCK); continue at Phase 2.**
- [ ] **Step 44** [AFTER-E1][CODE] ~40m: Implement the chosen option, test-first: a `monad-mbc` host-emulator test (`crates/monad-mbc/tests/`) that forks once and asserts the pid pair (`parent=1, child=2` for K); then the BPF/kernel change. Record `verified N insns` after the eBPF rebuild.
- [ ] **Step 45** [TEST][B][S] ~3m: Probe both flag states again (Steps 30, 34 commands) → identical `GF:` lines ON and OFF under the chosen numbering.
- [ ] **Step 46** [V] ~1m: **BOOKS RECONCILED** — ON == OFF on all four lines; `gate2` still `ISOLATION-PASS` (it may need its init-pid assumption updated in the same commit). Mismatch → Step 47.
- [ ] **Step 47** [D] ~20m: One book did not update: dump `PROC_TABLE` after fork (bootctl `--dump-proc` if present, else a `mmio_puthex` in the kernel's `fork` return path, reverted after). Two attempts then STUCK Steps 48-52 and keep the flag OFF.
- [ ] **Step 48** [AFTER-E1][CODE] ~3m: Flip the default: `const GETPID_INJECT: bool = true;` with the comment updated (E1 answer, date, probe evidence pointer).
- [ ] **Step 49** [TEST][B][S] ~10m: Full sweep + harness with the flag ON
  ```bash
  kbuild >/dev/null 2>&1; ubuild >/dev/null 2>&1; sweep | tee "$S6/p1-sweep-on.txt" | grep -c SAME; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p1-regression.log" | grep -aE 'FAIL|verifier'
  ```
- [ ] **Step 50** [V] ~2m: Harness zero FAIL; ascend insns recorded (expect ≈ 911,319 ± toolchain drift, must be ≤ 950,000); Doom **761,580** unchanged. Goldens: lines that differ only in pid values are the policy-approved break → **recapture all 12** per `references/phase24-t1-2026-07-07.md` §"Golden recapture", re-sweep to 12/12 SAME, record the recapture reason + HEAD in `$LOG`. Any other diff → revert Step 48 and debug (two attempts, then STUCK).
- [ ] **Step 51** [DOC-UPDATE][W] ~15m: `EVOLUTION-1-TRAP-FLIP.md` §Stage 1 status: "CLOSED <date>: numbering = <E1>, flag ON, delta N"; `UPC-LINUX-MASTER-BATTLE-PLAN.md` 1.3.5 → `[x]`; `wiki/UPC-Dream-Ladder.md:16` replaces the "+5409 … deferred" clause.
- [ ] **Step 52** [V][C] ~2m: **PHASE 1 EXIT GATE** — probe committed (Step 40) and either (a) E1 answered, books reconciled, flag ON, harness green, goldens 12/12 at the new HEAD, docs updated; or (b) E1 open, flag OFF, Step 35 table in the log and Steps 44-51 marked `[AFTER-E1]` skipped.
  ```bash
  cd "$PROJECT_ROOT" && git add -A crates/xv6-mbc ebpf/monad-cpu-ebpf/src/main.rs docs/battle-plans wiki/UPC-Dream-Ladder.md references/s-ascend-2-log.md && commitp -m "feat(upc): [S-ASCEND-2 41-52] pid numbering reconciled (E1), GETPID_INJECT default decided" || commitp -m "docs(upc): [S-ASCEND-2 41-52] pid numbering design + E1 escalation" 
  ```
  - If pass → Phase 2. If fail → DO NOT PROCEED into Phase 7 (kernel work shares this state); Phases 2-6 may proceed.

---

## PHASE 2: EVOLUTION-1 STAGE 2 RE-PLAN AGAINST THE MEASURED DELTA (Steps 53-66)

**Goal**: Re-read D1 against a negative delta as EVOLUTION-1's own rule demands, get Stevie's reach decision, and leave a Stage 2 skeleton (phases, gates, prerequisites; no numbered steps) ready for its own Warmonger session. M-221, decision 45.
**Prerequisite**: Step 37 delta recorded (Phase 1 part 1).
**Time**: ~2 h (plus the E2 wait)
**Agent**: Agent [P:3] (docs only; no sudo)

- [ ] **Step 53** [R] ~15m: Re-read `EVOLUTION-1-TRAP-FLIP.md` lines 58-110 (D1-D3, the reach rule, the Scientist caveat) and 159-168 (Stage 2 ladder), `UPC-LINUX-MASTER-BATTLE-PLAN.md` Epic 1.3, and the §"State-ownership matrix" (lines 203-226).
- [ ] **Step 54** [B] ~2m: The two deltas and today's budget headroom, in one table line
  ```bash
  grep -aE 'ON=.*DELTA=' "$LOG" | tail -1; echo "July +5409 (905,440) vs S5/now: see line above; headroom at OFF: $((1000000-926542)) insns; at ON: see ON value"
  ```
- [ ] **Step 55** [DESIGN][W] ~30m: `EVOLUTION-1-TRAP-FLIP.md` new §"D1 reach re-read (2026-10-xx)": the rule as written, both measurements (toolchain-attributed), what "self-funding" means concretely (each retired BPF handler frees insns the kernel path does not consume), the Scientist caveat restated (getpid proves nothing about fork/exec cost), and a **recommendation**: push toward (a) in Stage 2 ordering (cheapest-state-first per the ladder), gated per rung on "corpus green + freed insns ≥ 0". Mark the section "pending E2".
- [ ] **Step 56** [ESCALATE] ~—: **E2 — D1 reach.** Context: Step 55. Question: given the negative delta, do we push toward (a) kernel-authoritative syscalls (Stage 2 runs the full ladder, rung by rung), or keep (c) minimal (migrate only where authenticity demands: `sbrk`, `pipe`, `kill` arrivals, nothing else)? Recommendation: push toward (a). Why still Stevie's: it sets the UPC's direction for the next several sprints and the eBPF budget they consume. **Until answered: Steps 63-66 write the skeleton as a draft; nothing else waits.**
- [ ] **Step 57** [DESIGN][W] ~40m: `docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md`: header block (Prerequisite: S-ASCEND-2 Phases 1, 3, 7 done; pid reconciliation; `gate_nway` green), the rung ladder as phases in EVOLUTION-1's order (getpid ✓ → uptime/pause → write → read → fstat/dup/close → open (fs_walk hand-off) → fork/exec/wait (PROC_TABLE hand-off) → arrivals sbrk/pipe/kill/mkdir/unlink/link/chdir/sleep), one exit gate per rung ("corpus 12/12 SAME, harness green, freed insns recorded, usertests subset N green"), the `usertests` subset plan (which of the 3,248 lines become gates when), the four-lens panel requirement at kickoff, and an explicit "NO NUMBERED STEPS HERE — forge at kickoff" banner. Cite M-221.
- [ ] **Step 58** [V] ~2m: Skeleton has no `**Step` lines (`grep -c '\*\*Step' docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md` → `0`), has one gate per rung, cites the E2 answer slot.
- [ ] **Step 59** [W] ~10m: `UPC-LINUX-MASTER-BATTLE-PLAN.md`: add Epic 1.6 "EVOLUTION-1 Stage 2 (skeleton: EVOLUTION-1-STAGE-2-skeleton.md; forge when E2 answered)"; update 1.3.5's REMAINS clause to point at Phase 1 of this plan.
- [ ] **Step 60** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  cd "$PROJECT_ROOT" && git add docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md && commitp -m "docs(upc): [S-ASCEND-2 53-60] EVOLUTION-1 D1 reach re-read against the negative delta; Stage 2 skeleton"
  ```
- [ ] **Step 61** [B] ~1m: Doc link check on touched pages
  ```bash
  cd "$PROJECT_ROOT" && for f in docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md docs/battle-plans/EVOLUTION-1-STAGE-2-skeleton.md; do grep -oE '\]\([^)#]+' "$f" | sed 's/](//' | grep -vE '^https?:' | while read -r p; do test -e "docs/battle-plans/$p" || test -e "$p" || echo "BROKEN $f -> $p"; done; done | head
  ```
- [ ] **Step 62** [V] ~30s: No `BROKEN` lines.
- [ ] **Step 63** [AFTER-E2][W] ~10m: Record E2's answer in the skeleton header and in EVOLUTION-1 §D1 re-read ("DECIDED <date>: …"); if (c) minimal, trim the ladder to the arrivals and mark the migration rungs "not planned".
- [ ] **Step 64** [AFTER-E2][W] ~5m: `UPC-LINUX-MASTER-BATTLE-PLAN.md` Epic 1.6 line reflects the decided reach.
- [ ] **Step 65** [AFTER-E2][C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [S-ASCEND-2 63-65] E2 recorded`).
- [ ] **Step 66** [V] ~1m: **PHASE 2 EXIT GATE** — re-read section exists with both measurements; skeleton committed with zero numbered steps; master plan points at it; E2 either recorded or the section says "pending E2".
  - If pass → Phase 3. If fail → fix within the phase (docs only; nothing downstream blocks on it except the Stage 2 session itself).

---

## PHASE 3: N-WAY ISOLATION TO GREEN + THE LOOPED-FORK GATE (Steps 67-94)

**Goal**: `gate_nway` prints 3× `NWAY-ok` and 0 `NWAY-FAIL`; a looped-fork program no longer fork-bombs; both are harness gates; KF1 retired. M-222.
**Prerequisite**: Phase 0 gate (independent of E1; if Phase 1 flipped the flag, run with the flag as committed).
**Time**: ~4 h
**Agent**: Coordinator (sudo, kernel, maybe eBPF)

### Diagnose, do not guess

- [ ] **Step 67** [R] ~10m: Read `user/gate_nway.c` fully (the stride falsification: HIGH_BASE = 6 MiB vs shipped stride 8 MiB; the pause()-based co-scheduling; unrolled forks) and the per-pid pgd code: `grep -n 'SLICE_STRIDE\|USER_CHILD_SLICE\|pid_phys_offset' crates/xv6-mbc/adapters/*.c ebpf/monad-cpu-ebpf/src/main.rs | head -20`.
- [ ] **Step 68** [B] ~2m: Which child fails, and what it saw
  ```bash
  grep -aoE 'NWAY-(ok|FAIL)[^\n]{0,24}|canary[^\n]{0,40}|pid[0-9]' "$S6/p0-nway.txt" | head -20
  ```
- [ ] **Step 69** [DESIGN][W] ~15m: `$LOG` §"gate_nway hypotheses", each falsifiable with one run: **Hn1** stride/slice geometry (a child's high band maps into a sibling's low band: a pgd bug, not a scheduler bug) → test by changing HIGH_BASE to 2 MiB in a copy (`gate_nway_h1.c`): if it passes, geometry is at fault; **Hn2** co-scheduling (pause() does not yield in the order the comment assumes, so verify runs before a sibling's write: a false FAIL) → test by adding a second pause() before verify; **Hn3** fork copies the parent's pgd slice base instead of allocating a fresh one for the 2nd/3rd child (the looped-fork bug's static cousin) → test by printing each child's `satp`/slice base via a fixed-string token per pid.
- [ ] **Step 70** [W][BUILD][B] ~15m: Build the three probe variants (copies of gate_nway.c under `user/`, registered in `Makefile.mbc-userland`), run each, collect verdict lines
  ```bash
  for v in h1 h2 h3; do (cd "$UP" && make -f ../adapters/Makefile.mbc-userland "target/gate_nway_$v.mbc" 2>&1 | tail -1); (cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/gate_nway_$v.mbc" --triggers 1500000 --instance 111 2>&1) > "$S6/p3-nway-$v.txt"; echo "== $v"; grep -aoE 'NWAY-(ok|FAIL)|SB: [0-9a-f]+' "$S6/p3-nway-$v.txt" | sort | uniq -c; done
  ```
- [ ] **Step 71** [V] ~5m: Exactly one hypothesis is confirmed (h1 passes ⇒ Hn1; h2 passes ⇒ Hn2; h3 shows a repeated slice base ⇒ Hn3). Record in `$LOG`. None confirmed → Step 72. Two confirmed → the smaller fix first, re-test.
- [ ] **Step 72** [D] ~30m: Run the failing gate on the host emulator with the same kernel to separate "BPF executor" from "kernel": `crates/monad-mbc` `mbc_emulate` with `--kernel $K/xv6-mbc.mbc --userland $K/gate_nway.mbc` (check `cargo run --release --bin mbc-emulate -- --help` for the exact flags first). Emulator passes and BPF fails ⇒ executor divergence (open a LICH-015 full-engine case, Step 240, and treat as Hn4: MMU translation in BPF). Both fail ⇒ kernel/pgd. Two attempts then STUCK Steps 73-80; keep Steps 81-94 (the forkloop gate and harness wiring are independent).

### Fix and prove

- [ ] **Step 73** [CODE][W] ~45m: Test-first on the host emulator: a `crates/monad-mbc/tests/nway_isolation.rs` (or extend the existing boot test) that runs `gate_nway.mbc` and asserts 3× `NWAY-ok`, 0 `NWAY-FAIL` in the TTY capture; it FAILS now (if the emulator passes in Step 72, assert on the BPF path in Step 75 instead and keep this test as the regression lock).
- [ ] **Step 74** [CODE] ~60m: The fix for the confirmed hypothesis, smallest possible: Hn1 → slice geometry constant/allocator in `uproc.c`/`uvm.c` (one constant or the per-pid base formula); Hn2 → gate program only (a scheduling-order comment fix is a doc, not a kernel change: then the gate was wrong, record it); Hn3 → `fork` path allocates a fresh slice (kernel) or BPF `SYS_FORK` slot→slice mapping (eBPF; record insns before/after). No speculative changes elsewhere.
- [ ] **Step 75** [TEST][B][S] ~4m: gate_nway on BPF
  ```bash
  kbuild >/dev/null 2>&1; ubuild >/dev/null 2>&1; [ -n "$(git -C "$PROJECT_ROOT" status --short ebpf/)" ] && (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); out="$(cd "$EBPF" && UPC_VERIFIER_STATS=1 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/gate_nway.mbc" --triggers 1500000 --instance 111 2>&1)"; printf '%s\n' "$out" > "$S6/p3-nway-fixed.txt"; grep -aoE 'NWAY-(ok|FAIL)[^\n]{0,8}|verified [0-9]+ insns' <<<"$out" | sort | uniq -c
  ```
- [ ] **Step 76** [V] ~1m: **NWAY GREEN** — 3× `NWAY-ok` (pid1, pid2, pid3), 0 `NWAY-FAIL`; if eBPF changed, insns ≤ 950,000 recorded. Fail → Step 77.
- [ ] **Step 77** [D] ~25m: Re-run the three variants (Step 70) against the fixed tree; the pattern tells which assumption the fix missed. Two attempts then revert Step 74, STUCK Steps 78-80.
- [ ] **Step 78** [TEST][B][S] ~6m: No regression: sweep + gate2 + the corpus
  ```bash
  sweep | tee "$S6/p3-sweep.txt" | grep -c SAME; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p3-regression.log" | grep -aE 'FAIL|verifier'
  ```
- [ ] **Step 79** [V] ~1m: 12/12 SAME (slice geometry changes must not alter TTY output; if they do, that is a finding, not a recapture), harness zero FAIL, Doom 761,580 unchanged, ascend insns recorded.
- [ ] **Step 80** [C] ~1m: **COMMIT CHECKPOINT** (`fix(upc): [S-ASCEND-2 67-80] gate_nway green: <hypothesis>`; include the emulator test).

### The looped-fork gate

- [ ] **Step 81** [W][CODE] ~15m: `user/forkloop.c`: `for (i = 0; i < 3; i++) { pid = fork(); if (pid == 0) { print "FL: child"; exit(0); } wait(0); }` then print `FL: parent-done`, fixed strings only (a `FL: child` per iteration; `FL: parent-done` once). This is exactly the shape `gate_nway.c`'s comment says fork-bombs.
- [ ] **Step 82** [BUILD][TEST][B][S] ~4m: Build, run with a generous trigger budget and a hard cap, count
  ```bash
  (cd "$UP" && make -f ../adapters/Makefile.mbc-userland target/forkloop.mbc 2>&1 | tail -1); out="$(cd "$EBPF" && timeout 120 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/forkloop.mbc" --triggers 1500000 --instance 111 2>&1)"; printf '%s\n' "$out" > "$S6/p3-forkloop.txt"; grep -aoE 'FL: [a-z-]+|fork.*fail|num_proc[^\n]{0,20}' <<<"$out" | sort | uniq -c
  ```
- [ ] **Step 83** [V] ~1m: Reproduces the bug (expected: many `FL: child`, no `FL: parent-done`, or fork failures) **or** passes (3× `FL: child`, 1× `FL: parent-done`). Record. If it passes, the "substrate bug" was the Step 74 class and is already fixed: skip to Step 88.
- [ ] **Step 84** [DESIGN][W] ~15m: `$LOG` §"forkloop diagnosis": by read, where does a child re-enter the loop? Candidates: (a) BPF `SYS_FORK` returns 0 in the parent and the child pid in the child (inverted); (b) the child's `pc`/`a0` after fork is the parent's pre-fork state (trapframe copy misses `a0`); (c) the translator's `fork` return path (`usys.S` ecall + `a0` check) mis-translates the branch. One run each: add `FL: a0=0/nonzero` tokens in both branches; the token pattern identifies (a)/(b).
- [ ] **Step 85** [CODE] ~45m: Fix the identified cause, test-first in the host emulator (`crates/monad-mbc/tests/forkloop.rs` asserting 3 children + parent-done), then BPF/kernel. If eBPF changed: insns recorded.
- [ ] **Step 86** [TEST][B][S] ~3m: Re-run Step 82 → 3× `FL: child`, 1× `FL: parent-done`.
- [ ] **Step 87** [V] ~1m: Passes. Fail → one more diagnosis pass (two attempts), then STUCK Steps 88-89 only (forkloop stays a known failure, listed as KF2 with evidence); Steps 90-94 still wire gate_nway.
- [ ] **Step 88** [CODE][W] ~10m: Harness gates: add to `scripts/upc-regression.sh` after the gate2 block
  ```bash
  cd "$PROJECT_ROOT" && python3 - <<'PY'
  import re,io
  p='scripts/upc-regression.sh'; s=open(p).read()
  blk='''
  # S-ASCEND-2 Phase 3: N-way CONCURRENT isolation (ADR-074 Gate 4) and the looped-fork gate.
  NW="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \\
    --userland "$K/gate_nway.mbc" --triggers 1500000 --instance 111 2>&1)"
  [ "$(grep -ac 'NWAY-ok' <<<"$NW")" -ge 3 ] && ! grep -aq 'NWAY-FAIL' <<<"$NW" && pass "gate_nway 3x NWAY-ok, 0 FAIL" || fail "gate_nway (concurrent isolation)"
  FL="$(cd "$EBPF" && timeout 120 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \\
    --userland "$K/forkloop.mbc" --triggers 1500000 --instance 111 2>&1)"
  [ "$(grep -ac 'FL: child' <<<"$FL")" -eq 3 ] && grep -aq 'FL: parent-done' <<<"$FL" && pass "forkloop: 3 children, parent done" || fail "forkloop (looped fork)"
  '''
  anchor="grep -aq 'ISOLATION-PASS' <<<\"$G2\" && pass \"gate2 ISOLATION-PASS\" || fail \"gate2 ISOLATION-PASS\"\n"
  assert anchor in s, "gate2 anchor not found"
  s=s.replace(anchor, anchor+blk,1); open(p,'w').write(s); print("harness patched")
  PY
  bash -n scripts/upc-regression.sh && echo SYNTAX-OK
  ```
- [ ] **Step 89** [W] ~3m: Make sure `ubuild` (S5 helper) builds `gate_nway.mbc` and `forkloop.mbc` (both in `Makefile.mbc-userland`'s default target list: `grep -n 'gate_nway\|forkloop' crates/xv6-mbc/adapters/Makefile.mbc-userland`).
- [ ] **Step 90** [TEST][B][S] ~8m: Harness end to end
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p3-regression-final.log" | grep -aE 'PASS|FAIL|verifier'
  ```
- [ ] **Step 91** [V] ~1m: Two new `PASS` lines, zero `FAIL` (or forkloop `FAIL` only if Step 87 went STUCK, in which case the forkloop gate is wrapped in `if [ "${UPC_GATE_FORKLOOP:-0}" = 1 ]` and KF2 recorded), budgets unchanged from Step 79.
- [ ] **Step 92** [DOC-UPDATE][W] ~10m: `$LOG` KF table: KF1 retired (commit, evidence file); `UPC-LINUX-MASTER-BATTLE-PLAN.md` Epic 1.2 note "Gate 4 N-way green <date>"; `wiki/UPC-Dream-Ladder.md` L5 row mentions N-way isolation; `S-UPC-WIRE-COMPUTE-battle-plan.md:145` KF1 row gets "retired by S-ASCEND-2 Step 91".
- [ ] **Step 93** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  cd "$PROJECT_ROOT" && git add scripts/upc-regression.sh crates/xv6-mbc crates/monad-mbc docs/battle-plans wiki/UPC-Dream-Ladder.md references/s-ascend-2-log.md && commitp -m "test(upc): [S-ASCEND-2 81-93] forkloop gate + gate_nway gated in the harness; KF1 retired"
  ```
- [ ] **Step 94** [V] ~1m: **PHASE 3 EXIT GATE** — harness has both gates and is green; emulator regression tests committed; 12/12 SAME; budgets recorded; tree clean.
  - If pass → Phase 4 (and Phase 7 may now start). If fail → DO NOT PROCEED to Phase 7; Phases 4-6 may proceed.

---
## PHASE 4: THE LOADER'S TWO I/O-TIME GATES (Steps 95-116)

**Goal**: `upc_api::load()` performs the surface-vs-loaded-object feature check and the per-pid MMU slice non-overlap check it has promised since ADR-080; Epic 1.2.5 formally closes. M-223.
**Prerequisite**: Phase 0 gate. Host-side Rust only; no sudo until the final harness run.
**Time**: ~3 h
**Agent**: Agent [P:3] (own worktree; touches `crates/upc-api`, `cmd/upc-bootctl`, `crates/doom-runner` host code only)

### Design the two gates

- [ ] **Step 95** [R] ~15m: Read `crates/upc-api/src/workload.rs` (the `Workload`, `SyscallSurface`, `validate()`), `crates/upc-api/src/image.rs` (`GuestImage`, `expected_rv2mbc_sha256`), the `ImageSink` trait and `load()`, and how `cmd/upc-bootctl/src/main.rs:300-360` and `crates/doom-runner` build their `Workload`.
- [ ] **Step 96** [B] ~2m: How the eBPF object could declare its surface today
  ```bash
  cd "$PROJECT_ROOT" && grep -nE 'cfg\(feature|features\s*=|ascend-linux|no-default' ebpf/monad-cpu-ebpf/Cargo.toml ebpf/monad-cpu-ebpf/src/main.rs | head -12; llvm-readelf -S ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf 2>/dev/null | grep -E 'note|rodata|\.BTF' | head -5 || readelf -S ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf | grep -E 'note|rodata' | head -5
  ```
- [ ] **Step 97** [DESIGN][W] ~30m: `docs/adr/ADR-080-upc-programmatic-api.md` new §"I/O-time gates (S-ASCEND-2)" (amend, status stays Accepted): **Gate A, surface vs loaded object** — the eBPF object carries a 32-byte `.rodata` constant `UPC_SURFACE_ID` = first 32 bytes of SHA-256 over the sorted syscall-number list its `cfg(feature)` set compiles in (emitted by a `build.rs` in `ebpf/monad-cpu-ebpf` from the feature flags); the loader reads the constant from the ELF (`object` crate already in the dependency tree? verify with `grep -n '^object\|^goblin' crates/upc-api/Cargo.toml Cargo.lock | head`) and compares it to `sha256(workload.surface.numbers_sorted())[..32]`; mismatch = `ValidationError::SurfaceObjectMismatch`, fail-closed. **Gate B, per-pid slice non-overlap** — `Workload` gains `mmu: Option<SliceGeometry { base, stride, slice_len, max_pids }>` (ascend: base `USER_CHILD_SLICE`, stride 8 MiB, len 8 MiB, max_pids = PROC_TABLE size 8; Doom: `None`); `validate()` checks `stride >= slice_len`, `base + max_pids*stride <= RAM_MAP bytes`, and no slice overlaps the kernel image, ramdisk (`0x00800000`) or MMIO windows. Both are descriptor+object checks, so they live in `validate_against_object(&Workload, &MapCapacities, &ObjectFacts)` called by `load()` before the first map write. No wire bytes, no BPF insns.
- [ ] **Step 98** [V] ~2m: Design cites the exact constants from code (`grep -n 'USER_CHILD_SLICE\|SLICE_STRIDE\|0x00800000\|PROC_TABLE' crates/xv6-mbc/adapters/*.c ebpf/monad-cpu-ebpf/src/main.rs cmd/upc-bootctl/src/main.rs | head -12` matches the ADR text).

### Red, then green

- [ ] **Step 99** [CODE][W] ~25m: Tests first in `crates/upc-api/tests/io_gates.rs`: (1) surface id match passes; (2) one syscall number added to the surface → `SurfaceObjectMismatch`; (3) geometry `stride < slice_len` → `SliceOverlap`; (4) geometry past RAM_MAP → `SliceOutOfRange`; (5) slice overlapping the ramdisk base → `SliceOverlap`; (6) Doom workload (`mmu: None`) unaffected; (7) the `RecordingSink` characterization tests still byte-identical.
- [ ] **Step 100** [TEST][B] ~2m: Expect RED
  ```bash
  cd "$PROJECT_ROOT/crates/upc-api" && cargo test --test io_gates 2>&1 | tail -5
  ```
- [ ] **Step 101** [V] ~30s: Compile errors or failures on the new tests only (the new types do not exist yet). Existing tests untouched.
- [ ] **Step 102** [CODE] ~50m: Implement: `ObjectFacts { surface_id: [u8;32] }` + a reader for the `.rodata` symbol; `SliceGeometry`; `validate_against_object`; `load()` calls it; two new `ValidationError` variants with `Display` text. `build.rs` in `ebpf/monad-cpu-ebpf` emits `UPC_SURFACE_ID` from the feature set (kept `#[no_mangle] #[link_section = ".rodata.upc"] pub static`).
- [ ] **Step 103** [TEST][B] ~3m: Expect GREEN, host-side
  ```bash
  cd "$PROJECT_ROOT/crates/upc-api" && cargo test 2>&1 | tail -3 && cd ../monad-mbc && cargo test --release --lib 2>&1 | tail -1
  ```
- [ ] **Step 104** [V] ~30s: `0 failed` in both; characterization tests still pass (byte-identical loads).
- [ ] **Step 105** [C] ~1m: **COMMIT CHECKPOINT** (`feat(upc-api): [S-ASCEND-2 95-105] I/O-time gates: surface-vs-object id, per-pid slice geometry`).
- [ ] **Step 106** [CODE] ~20m: Wire both loaders: `upc-bootctl` builds `SliceGeometry` for ascend from the same constants (one `const` module shared with the kernel adapters via a generated header is out of scope; cite the values and add a parity test that greps `uproc.c` for them: `tests/parity_slice_constants.rs`); `doom-runner` passes `mmu: None`. Both read `ObjectFacts` from the object they are about to load.
- [ ] **Step 107** [BUILD][B][S] ~6m: Rebuild the eBPF object (the new `.rodata.upc` constant must not change the verifier count: it is data) and run the harness
  ```bash
  (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1 && cargo build --release -p monad-cpu-ebpf 2>&1 | tail -1); (cd "$PROJECT_ROOT/cmd/upc-bootctl" && cargo build --release 2>&1 | tail -1); (cd "$PROJECT_ROOT/crates/doom-runner" && cargo build --release 2>&1 | tail -1); bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p4-regression.log" | grep -aE 'FAIL|verifier'
  ```
- [ ] **Step 108** [V] ~1m: Zero FAIL; ascend and Doom insns **unchanged** from Phase 0 (a `.rodata` constant is not an instruction; a change means `build.rs` leaked into code paths → fix before continuing).
- [ ] **Step 109** [TEST][B][S] ~4m: Negative gate, live: boot the ascend kernel with the **Doom** object (wrong surface) and expect the loader to refuse before any map write
  ```bash
  (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf 2>&1 | tail -1); out="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/init.mbc" --triggers 10 --instance 111 2>&1)"; grep -aoE 'SurfaceObjectMismatch[^\n]{0,80}|refus[^\n]{0,60}' <<<"$out" | head -2; sudo bpftool prog show 2>/dev/null | grep -c monad; (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1)
  ```
- [ ] **Step 110** [V] ~30s: Mismatch error printed, `0` programs left attached, ascend object rebuilt. (Before this sprint the same mistake booted and died in `start_mbc.c` with `MRET fall-through` — the CLAUDE.md "build gotcha" — now it is refused at load.)
- [ ] **Step 111** [TEST][B][S] ~3m: Negative geometry, live: `upc-bootctl` with an env override `UPC_SLICE_STRIDE=0x400000` (6 MiB, the gate_nway falsification stride; add the override only under `#[cfg(debug_assertions)]` or a `--unsafe-geometry` flag that also prints a warning) → `SliceOverlap` before load.
- [ ] **Step 112** [V] ~30s: Refused with `SliceOverlap`; nothing attached.
- [ ] **Step 113** [DOC-UPDATE][W] ~10m: `workload.rs` doc comment: replace "named in the docs but not performed here" with the gate list and where they run; `UPC-LINUX-MASTER-BATTLE-PLAN.md` 1.2.4 → `[x]`, 1.2.5 GATE → `[x]` "formally closed <date>, S-ASCEND-2 Steps 95-116"; CLAUDE.md "Build gotcha" paragraph gains one sentence: the loader now refuses a surface mismatch.
- [ ] **Step 114** [C] ~1m: **COMMIT CHECKPOINT** (`feat(upc): [S-ASCEND-2 106-114] both loaders run the I/O-time gates; Epic 1.2.5 closed`).
- [ ] **Step 115** [TEST][B] ~3m: `cargo test` for `upc-api`, `upc-bootctl`, `doom-runner` host tests; `go build ./...` (nothing Go changed; sanity).
- [ ] **Step 116** [V] ~1m: **PHASE 4 EXIT GATE** — 7 new tests green; both negative live gates refuse; harness green with unchanged budgets; docs updated; tree clean.
  - If pass → Phase 5. If fail → fix within the phase; Phase 5 does not depend on it except Step 141 (SHA gate reuses `ValidationError`).

---

## PHASE 5: REQUIRED-BEFORE-PROD, 0/5 → 5/5 (Steps 117-156)

**Goal**: The five items the 2026-05-13 brief named: forward-drop on `veth-upc0p`, `upc-bootctl` as its own nologin user with CAP_BPF+CAP_NET_ADMIN only, a seccomp profile, the `UPC_DEMO_MODE` gate, and the `.rv2mbc` SHA gate default-on with BootParams v3. M-224 (and the SHA half of M-225's neighbour).
**Prerequisite**: Phase 0 gate; Phase 4 for Step 141.
**Time**: ~6 h (plus E6 for the host install)
**Agent**: Coordinator (sudo, netns, systemd)

### Item 4 first: the demo gate (smallest, unblocks safe testing of the rest)

- [ ] **Step 117** [CODE][W] ~15m: Test first: `cmd/upc-bootctl/tests/demo_mode.rs` runs the binary with `boot --kernel <any> --instance 111` and **no** `UPC_DEMO_MODE` → exit non-zero, stderr contains `UPC_DEMO_MODE=1 required for a live boot; add --dry-run`; with `--dry-run` and no env → exit 0; with `UPC_DEMO_MODE=1` → not refused by this check (the test stops before any BPF by passing a nonexistent kernel path and asserting the error is the file error, not the gate).
- [ ] **Step 118** [CODE] ~15m: In `main.rs` `boot` dispatch, before anything touches BPF or netns: `if !dry_run && std::env::var("UPC_DEMO_MODE").as_deref() != Ok("1") { bail!(...) }`. Also honour it in `console` if that path attaches.
- [ ] **Step 119** [TEST][B] ~2m: `cd cmd/upc-bootctl && cargo test --test demo_mode 2>&1 | tail -3` → green.
- [ ] **Step 120** [W] ~5m: `scripts/upc-regression.sh` and the S5 helpers' boots export `UPC_DEMO_MODE=1` (one `export UPC_DEMO_MODE=1` near the top of the harness; the helper block in S5 Phase 0 is sourced read-only, so add `export UPC_DEMO_MODE=1` to Step 2's line in this plan and to `wiki/Linux-on-UPC.md` boot recipe).
- [ ] **Step 121** [V][S] ~8m: Harness still green with the env set; without it, the first boot refuses (run one `xv6_expect` line by hand with the env unset → refusal text).

### Item 5: `.rv2mbc` SHA gate default-on, BootParams v3

- [ ] **Step 122** [R] ~10m: Read `main.rs:620-660` (the env-gated SHA check), `bootparams.rs` (`BOOT_VERSION = 2`, reserved bytes), `crates/upc-api/src/image.rs:83` (`expected_rv2mbc_sha256: Option<String>`), and how the kernel reads BootParams version (`grep -n 'BOOT_VERSION\|boot_version\|version' crates/xv6-mbc/adapters/start_mbc.c crates/upc-bootstub/src/*.rs | head`).
- [ ] **Step 123** [DESIGN][W] ~15m: `docs/doom/UPC_BOOT_PROTOCOL_V2.md` → add §"v3 (S-ASCEND-2)": version field 3; the 32 reserved bytes carry the `.rv2mbc` SHA-256 (already the Phase 1.3 design); loaders **require** the SHA: `--rv2mbc-sha <hex>` flag or a sibling `<image>.rv2mbc.sha256` file, else refuse; `--allow-unverified-rv2mbc` prints a warning and is refused when `UPC_DEMO_MODE=1` is set together with a network attach (lab-only escape hatch). Kernel side: `start_mbc.c` accepts version 2 or 3 this sprint (one-release compatibility), logs the version byte to TTY.
- [ ] **Step 124** [CODE][W] ~20m: Tests: `cmd/upc-bootctl/tests/rv2mbc_sha.rs` — (1) no SHA, no flag → refused `Rv2mbcShaRequired`; (2) wrong SHA → `Rv2mbcShaMismatch` (reuse upc-api's existing mismatch path); (3) sidecar file present and correct → proceeds (dry-run); (4) `--allow-unverified-rv2mbc` + dry-run → warning + proceed; (5) BootParams bytes have version 3 and the SHA in the reserved window (unit test on `bootparams.rs`).
- [ ] **Step 125** [CODE] ~40m: Implement: `BOOT_VERSION = 3`; `to_bytes()` writes the SHA; `main.rs` resolves the SHA (flag → sidecar → refuse); `expected_rv2mbc_sha256` becomes `Some` always on the non-escape path; `start_mbc.c` accepts 2|3.
- [ ] **Step 126** [BUILD][TEST][B] ~5m: Host tests green; kernel rebuilt (version-accept change) and `.mbc` hash recorded — this IS a kernel source change, so byte-identity breaks by design: record old/new `sha256sum $K/xv6-mbc.mbc` and the reason in `$LOG`.
- [ ] **Step 127** [W] ~5m: Generate sidecars for the shipped images and make the userland/kernel Makefiles emit them: `Makefile.mbc` kernel target appends `sha256sum target/xv6-mbc.rv2mbc | cut -d' ' -f1 > target/xv6-mbc.rv2mbc.sha256` (check the actual rv2mbc filename first: `ls $K/*.rv2mbc`); Doom: `doom/doom.rv2mbc.sha256` committed next to `doom/doom.rv2mbc` (its hash `e7ab7b67…` per the S5 log; regenerate and compare).
- [ ] **Step 128** [TEST][B][S] ~8m: Harness end to end (it must pick up the sidecars without flags)
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p5-regression-sha.log" | grep -aE 'FAIL|verifier|rv2mbc'
  ```
- [ ] **Step 129** [V] ~1m: Zero FAIL; both budgets unchanged (the SHA gate is host-side); sweep 12/12 SAME (a version byte logged to TTY would break goldens: log it only under `UPC_VERIFIER_STATS=1`-style verbosity, not on the golden path).
- [ ] **Step 130** [C] ~1m: **COMMIT CHECKPOINT** (`feat(upc): [S-ASCEND-2 117-130] UPC_DEMO_MODE gate; .rv2mbc SHA gate default-on; BootParams v3`).

### Items 1-3: netns forward-drop, unprivileged bootctl, seccomp

- [ ] **Step 131** [DESIGN][W] ~25m: `$LOG` §"bootctl privilege split": today bootctl (as root) creates `upc0` + veth, attaches XDP, writes maps. Split: **(i)** `deploy/systemd/unheaded-upc-netns.service` (oneshot, root, `RemainAfterExit=yes`) creates `upc0`, the veth pair, brings links up, and installs the nft rule `table inet unheaded_upc { chain forward { type filter hook forward priority 0; policy accept; iifname "veth-upc0p" drop; oifname "veth-upc0p" drop } }` (also a `scripts/upc-netns.sh up|down|status` the unit calls, idempotent, reusable by the harness); **(ii)** `upc-bootctl` stops shelling out to `ip` when the netns already exists (`netns.rs`: `ensure()` becomes "verify present, else error 'run unheaded-upc-netns.service'"; the create path stays behind `--create-netns` for lab use) → with that, bootctl needs CAP_BPF (maps, prog load) + CAP_NET_ADMIN (XDP attach) + CAP_PERFMON (if `UPC_VERIFIER_STATS` reads verifier logs; verify by trial) and **no** CAP_SYS_ADMIN; **(iii)** account `unheaded-upc` (sysusers), uid from `pkg/uids` next free in the UPC band (`grep -n 1679 pkg/uids/uids.go`), unit `unheaded-upc-bootctl@.service` template (instance = ROM name) with `User=unheaded-upc`, `AmbientCapabilities=CAP_BPF CAP_NET_ADMIN`, `CapabilityBoundingSet=` same, `NoNewPrivileges=yes`, `SystemCallFilter=@system-service bpf ~@privileged ~execve`-shaped filter proven by trial (Step 143), `ProtectSystem=strict`, `ReadOnlyPaths=/var/lib/unheaded/upc` (images), `Environment=UPC_DEMO_MODE=1` only in a `.d/demo.conf` drop-in; **(iv)** bpffs pins the maps need: `/sys/fs/bpf/unheaded-upc` owned `unheaded-upc`, created by the oneshot.
- [ ] **Step 132** [CODE][W] ~20m: `scripts/upc-netns.sh` (bash, `set -euo pipefail`, `up|down|status`, idempotent, prints the nft rule state); `deploy/systemd/unheaded-upc-netns.service`; sysusers line `u unheaded-upc - "Unheaded UPC boot controller" / /usr/sbin/nologin`; `pkg/uids/uids.go` `UPC = <next>` + map entry + test; `deploy/systemd/unheaded-upc-bootctl@.service` + `unheaded-upc-bootctl@.service.d/demo.conf.example`.
- [ ] **Step 133** [TEST][B] ~3m: Static gates on the new units and script
  ```bash
  cd "$PROJECT_ROOT" && bash scripts/check-systemd-units.sh 2>&1 | tail -4; bash -n scripts/upc-netns.sh && shellcheck scripts/upc-netns.sh 2>&1 | head -5; go test ./pkg/uids/ -count=1 2>&1 | tail -2; grep -c unheaded-upc deploy/sysusers.d/unheaded.conf
  ```
- [ ] **Step 134** [V] ~30s: units pass the checker (User= listed in sysusers, no root), shellcheck clean, uids test green, `1` sysusers line.
- [ ] **Step 135** [CODE] ~30m: `netns.rs`: `ensure_present()` (verify) default; `--create-netns` keeps the old behaviour; XDP attach unchanged. `main.rs`: on EPERM from `bpf()` print "need CAP_BPF: run via unheaded-upc-bootctl@ or sudo" (fail-loud, no silent fallback).
- [ ] **Step 136** [BUILD][TEST][B] ~3m: `cd cmd/upc-bootctl && cargo build --release && cargo test 2>&1 | tail -2`.
- [ ] **Step 137** [TEST][B][S] ~6m: Throwaway proof under sudo (no host install yet): bring the netns up with the script, then run bootctl **as an unprivileged user with ambient caps via `systemd-run`**
  ```bash
  sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up && sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" status | head -6
  sudo systemd-run --wait --pipe --collect -p User=nobody -p AmbientCapabilities="CAP_BPF CAP_NET_ADMIN" -p CapabilityBoundingSet="CAP_BPF CAP_NET_ADMIN" -p NoNewPrivileges=yes -p WorkingDirectory="$EBPF" -E UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/init.mbc" --triggers 4000000 --instance 111 --input $'echo caps-ok\n' 2>&1 | grep -aoE 'caps-ok|EPERM|Operation not permitted|need CAP[^\n]{0,40}' | head -3
  ```
- [ ] **Step 138** [V] ~1m: **UNPRIVILEGED BOOT** — prints `caps-ok`, no EPERM. EPERM → Step 139.
- [ ] **Step 139** [D] ~20m: Add `CAP_PERFMON` (verifier log read) or `CAP_SYS_RESOURCE` (memlock) one at a time and re-run; record the minimal set in `$LOG` and in the unit. If `CAP_SYS_ADMIN` turns out required, find the call (`strace -f -e trace=bpf,setns,unshare,mount` under sudo) and move it into the oneshot. Two attempts then STUCK Steps 140-154 (keep Items 4-5, which are done).
- [ ] **Step 140** [TEST][B][S] ~6m: Forward-drop proven: from inside `upc0`, a packet out of `veth-upc0p` toward the host must die at FORWARD
  ```bash
  sudo ip netns exec upc0 ip -6 addr add fd00:upc::2/64 dev veth-upc0p 2>/dev/null; sudo ip -6 addr add fd00:upc::1/64 dev veth-upc0 2>/dev/null; sudo ip netns exec upc0 ping -6 -c 2 -W 1 fd00:upc::1 >/dev/null 2>&1 && echo "LOCAL-LINK-OK (expected: same link, not forwarded)"; sudo ip netns exec upc0 ping -6 -c 2 -W 1 2606:4700:4700::1111 >/dev/null 2>&1 && echo "FORWARDED (BAD)" || echo "FORWARD-DROPPED (good)"; sudo nft list table inet unheaded_upc | grep -c veth-upc0p
  ```
- [ ] **Step 141** [V] ~30s: `FORWARD-DROPPED (good)` and `2` rule hits. `FORWARDED (BAD)` → the rule is in the wrong hook/family (`sudo nft list ruleset | grep -n -B3 veth-upc0p`); fix the script; two attempts then STUCK Item 1 only.
- [ ] **Step 142** [B][S] ~2m: Tear the throwaway down and confirm the harness still works through the script (the harness must call `upc-netns.sh up` first now)
  ```bash
  sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" down; grep -n 'upc-netns.sh' "$PROJECT_ROOT/scripts/upc-regression.sh" || sed -i '0,/^echo "== UPC regression harness =="/s//echo "== UPC regression harness =="\nsudo bash "$ROOT\/scripts\/upc-netns.sh" up >\/dev\/null/' "$PROJECT_ROOT/scripts/upc-regression.sh"; bash -n "$PROJECT_ROOT/scripts/upc-regression.sh" && echo OK
  ```
- [ ] **Step 143** [TEST][B][S] ~15m: Seccomp by trial: run Step 137's `systemd-run` with `-p SystemCallFilter="@system-service bpf" -p SystemCallFilter="~@privileged"` (two `-p`), then tighten by adding `-p SystemCallFilter="~execve ~execveat"` once `netns.rs` no longer execs `ip`; on a kill, `journalctl -t audit --since -2min | grep -oE 'syscall=[0-9]+' | sort | uniq -c` names the missing call; add it by name. Record the final filter in `$LOG` and the unit.
- [ ] **Step 144** [V] ~1m: Boot prints the probe token under the final filter; `~execve` holds (bootctl no longer spawns processes in the default path).
- [ ] **Step 145** [C] ~1m: **COMMIT CHECKPOINT** (`feat(upc): [S-ASCEND-2 131-145] bootctl privilege split: netns oneshot + forward-drop, unheaded-upc user, caps, seccomp`).
- [ ] **Step 146** [TEST][B][S] ~8m: Full harness + sweep after the split
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p5-regression-split.log" | grep -aE 'FAIL|verifier'; sweep | grep -c SAME
  ```
- [ ] **Step 147** [V] ~1m: Zero FAIL, budgets unchanged, 12/12 SAME.
- [ ] **Step 148** [ESCALATE] ~—: **E6 — west host window for UPC isolation.** Context: Steps 132-147 proved everything in a throwaway. Question: install on west (a) `deploy/sysusers.d/unheaded.conf` → `/etc/sysusers.d/` + `systemd-sysusers`; (b) `unheaded-upc-netns.service` enabled (netns + veth + nft rule at boot); (c) `unheaded-upc-bootctl@.service` installed, not enabled; (d) bpffs dir `/sys/fs/bpf/unheaded-upc`. Recommendation: yes — west already runs bootctl as root under sudo for the harness; this removes root. Undo: `systemctl disable --now unheaded-upc-netns; rm /etc/systemd/system/unheaded-upc-*; userdel unheaded-upc`. **Until answered: Steps 149-154 skip; units are committed.**
- [ ] **Step 149** [AFTER-E6][LIVE-RW][BARE-METAL][S][B] ~5m: Install sysusers + units on west
  ```bash
  cd "$PROJECT_ROOT" && sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers && id unheaded-upc && sudo install -m 0644 deploy/systemd/unheaded-upc-netns.service deploy/systemd/unheaded-upc-bootctl@.service /etc/systemd/system/ && sudo install -m 0755 scripts/upc-netns.sh /usr/local/libexec/unheaded/upc-netns.sh && sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-upc-netns.service && systemctl is-active unheaded-upc-netns.service
  ```
- [ ] **Step 150** [LIVE-RO][V] ~1m: `active`; `ip netns list | grep upc0`; `sudo nft list table inet unheaded_upc | grep -c veth-upc0p` → `2`.
- [ ] **Step 151** [AFTER-E6][LIVE-RW][S][B] ~4m: One real boot through the unit (images staged read-only)
  ```bash
  sudo install -d -o unheaded-upc -g unheaded-upc -m 0750 /var/lib/unheaded/upc && sudo install -m 0640 -o unheaded-upc "$K/xv6-mbc.mbc" "$K/xv6-mbc.rv2mbc" "$K/xv6-mbc.rv2mbc.sha256" "$K/fs.img" "$K/init.mbc" /var/lib/unheaded/upc/ && sudo systemctl start unheaded-upc-bootctl@xv6.service && sleep 5 && journalctl -u unheaded-upc-bootctl@xv6.service --since -2min --no-pager | grep -aoE 'xv6 booting|\$|EPERM|refus[^\n]{0,40}' | head -4
  ```
- [ ] **Step 152** [V] ~1m: `xv6 booting` seen from the unit, no EPERM, `ps -o user= -C upc-bootctl` → `unheaded-upc` (or the unit already exited cleanly after `--triggers`).
- [ ] **Step 153** [DOC-UPDATE][W] ~15m: `wiki/Linux-on-UPC.md` boot recipe → unit-first (`systemctl start unheaded-upc-bootctl@xv6`) with the sudo recipe kept as "lab shortcut"; `wiki/Service-Isolation.md` (or the uid table page: `grep -rl 16763 wiki | head -1`) gains the `unheaded-upc` row; `references/security-upc-linux-spitball-2026-05-13.md` §four isolation items → each gets "DONE <date> Step N"; `docs/doom/UPC_BOOT_PROTOCOL_V2.md` v3 section finalised.
- [ ] **Step 154** [C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [S-ASCEND-2 149-154] UPC isolation installed on west (E6); required-before-prod 5/5`).
- [ ] **Step 155** [AUDIT-UPDATE][W] ~5m: `docs/sweeps/OUTSTANDING-2026-10-27.md` M-224 → "DONE Steps 117-154 (host install under E6)"; register row if any (`compliance/findings/register.yaml` has no UPC row today: add `UPC-ISO-01` closed with evidence, so the brief's items are auditable).
- [ ] **Step 156** [V][C] ~1m: **PHASE 5 EXIT GATE** — Items 4, 5 proven in the harness; Items 1-3 proven in the throwaway (Steps 138, 141, 144) and, if E6 answered, live (Steps 150, 152); harness green, 12/12 SAME, budgets unchanged; tree clean.
  - If pass → Phase 6. If fail → fix within the phase; Phase 6 does not depend on E6.

---

## PHASE 6: FREEZE THE ROM, DROP THE SHIPPED SECRET, VALIDATE THE INSTANCE (Steps 157-180)

**Goal**: ROM_MAP and RV2MBC_MAP are frozen after load in both Rust loaders (the Lich D1-001/D6-001 remediation that `mapfreeze.go` describes but nothing performs); no binary ships `unheaded-doom-secret`; a CPU tick for instance N is only honoured from the flow that owns instance N. M-225.
**Prerequisite**: Phase 4 gate (loader code paths).
**Time**: ~4 h
**Agent**: Coordinator (eBPF budget for the hijack check)

### Freeze

- [ ] **Step 157** [R] ~10m: How each loader writes ROM/RV2MBC and holds the map handles: `grep -n 'ROM_MAP\|RV2MBC_MAP\|MapData\|map_mut\|freeze' crates/upc-api/src/*.rs cmd/upc-bootctl/src/*.rs crates/doom-runner/src/*.rs | head -30`; aya's API: `grep -rn 'fn freeze' ~/.cargo/registry/src/*/aya-*/src/maps/mod.rs | head -2`.
- [ ] **Step 158** [DESIGN][W] ~10m: `$LOG` §"freeze sequence": `ImageSink` gains `fn finalize(&mut self) -> Result<()>` called by `load()` after the last write and the SHA/surface gates; the aya-backed sinks call `MapData::freeze()` on ROM_MAP and RV2MBC_MAP there (CREATE with `BPF_F_RDONLY_PROG` is already how the programs declare them? verify: `grep -n 'RDONLY_PROG\|rdonly' ebpf/monad-cpu-ebpf/src/main.rs ebpf/monad-common/src/*.rs | head`); `RecordingSink::finalize` records a `Freeze` event so the characterization tests pin the order (writes → gates → freeze → attach). The Go `pkg/ebpf/mapfreeze.go` describes this sequence for a Go loader that no longer exists: [DECIDE] below.
- [ ] **Step 159** [CODE][W] ~15m: Tests: `crates/upc-api/tests/freeze.rs` — recording sink sees exactly one `Freeze` after the last `Write`; a sink whose `finalize` errors makes `load()` fail and nothing attaches. Root-gated live test in `cmd/upc-bootctl/tests/freeze_live.rs` (`#[ignore]` unless `UPC_LIVE=1`): after a dry boot, `bpftool map update pinned <ROM pin> key ... value ...` must fail EPERM.
- [ ] **Step 160** [CODE] ~40m: Implement `finalize` in `upc-api`, both aya sinks, `RecordingSink`; call site in `load()`.
- [ ] **Step 161** [TEST][B] ~3m: `cd crates/upc-api && cargo test 2>&1 | tail -2; cd ../../cmd/upc-bootctl && cargo build --release 2>&1 | tail -1; cd ../../crates/doom-runner && cargo build --release 2>&1 | tail -1`.
- [ ] **Step 162** [TEST][B][S] ~5m: Live EPERM proof
  ```bash
  cd "$EBPF" && UPC_DEMO_MODE=1 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/init.mbc" --triggers 3000 --instance 111 --input $'echo\n' --keep-pins 2>&1 | tail -1; pin="$(sudo bpftool map show -f 2>/dev/null | grep -A1 -i 'rom_map' | grep -oE '/sys/fs/bpf/[^ ]+' | head -1)"; echo "pin=$pin"; sudo bpftool map update pinned "$pin" key hex 00 00 00 00 value hex 00 00 00 00 2>&1 | head -1; sudo bpftool map show 2>/dev/null | grep -ci 'frozen'
  ```
  (If `--keep-pins` does not exist, add it in Step 160 as a debug flag that skips unpinning; the harness never uses it.)
- [ ] **Step 163** [V] ~1m: **ROM FROZEN** — `update` fails with `Operation not permitted`/`EPERM`; `frozen` ≥ 1. A successful update → the freeze ran on the wrong map or before a write; fix order; two attempts then STUCK Steps 164-166.
- [ ] **Step 164** [B][S] ~1m: Clean the pins; harness
  ```bash
  sudo rm -rf /sys/fs/bpf/unheaded-upc/* 2>/dev/null; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p6-regression-freeze.log" | grep -aE 'FAIL|verifier'
  ```
- [ ] **Step 165** [V] ~1m: Zero FAIL; budgets unchanged (freeze is a syscall from userspace, zero BPF insns).
- [ ] **Step 166** [DECIDE] ~5m: `pkg/ebpf/mapfreeze.go` — **RECOMMENDATION: delete** the Go helper and its tests (no Go loader populates ROM_MAP; both loaders are Rust and now freeze for real; the sequence doc moves to ADR-080's new section). Rationale: ADR-090's "fake success" rule — a helper with only test callers documents a path nothing runs. **Override ONLY if** `grep -rn 'pkg/ebpf"' --include=*.go cmd services | grep -v _test` shows a non-test importer of the package that uses other symbols (then delete only `mapfreeze.go` + its test).
- [ ] **Step 167** [B] ~2m: Apply Step 166, `go build ./... && go test ./pkg/ebpf/... -count=1 2>&1 | tail -2`, regenerate the unreached list if the sweep tool exists (`ls scripts/*unreached* 2>/dev/null` → run it, else note).
- [ ] **Step 168** [C] ~1m: **COMMIT CHECKPOINT** (`feat(upc): [S-ASCEND-2 157-168] ROM_MAP/RV2MBC_MAP frozen after load in both loaders; Go mapfreeze helper retired`).

### The shipped secret

- [ ] **Step 169** [CODE] ~15m: `cmd/doom/main.go:85` and `cmd/wotan-ctl/doom.go:56`: no default. Read `UPC_HMAC_KEY` (hex or ASCII, same parser) with a fail-loud guard (`"UPC_HMAC_KEY is required; credentials are not stored in this repo"`); `--hmac-key` flag stays but with empty default and documented as "prefer the env". `internal/doom/loader_test.go:160` keeps its literal (a test fixture is not a shipped default; rename the test constant `testHMACKey`). Add a gitleaks rule? Not needed: the literal leaves the tree. (This touches `cmd/doom`, but it is a secret-default removal, not Doom gameplay — M-225, E14.)
- [ ] **Step 170** [TEST][B] ~2m: `go build ./cmd/doom ./cmd/wotan-ctl && go test ./internal/doom/ ./cmd/wotan-ctl/... -count=1 2>&1 | tail -2; grep -rn 'unheaded-doom-secret' --include=*.go cmd | wc -l`.
- [ ] **Step 171** [V] ~30s: Builds, tests green, `0` literals under `cmd/`. Any script/unit that launched `cmd/doom` needs the env (`grep -rn 'cmd/doom\|bin/doom ' scripts deploy/systemd docs/doom/RUNBOOK.md | head`); add `Environment=UPC_HMAC_KEY=` via an `EnvironmentFile=-/etc/unheaded/doom/doom.env` reference in the unit (file never in repo) and a one-line note in `docs/doom/RUNBOOK.md` (doc-only Doom touch, allowed: it records the key now lives outside the repo).

### Flow-label instance hijack

- [ ] **Step 172** [R] ~10m: `main.rs:635-660`: instance = `flow_label & 0xFF`, `CPU_MAP.get_ptr_mut(&instance)`; how instances are created (bootctl writes CPU_MAP[instance] at boot: `grep -n 'CPU_MAP\|instance' cmd/upc-bootctl/src/main.rs | head -12`); what identifies the legitimate sender (bootctl's own trigger packets: `grep -n 'flow_label\|src\|saddr' cmd/upc-bootctl/src/*.rs | head -8`).
- [ ] **Step 173** [DESIGN][W] ~15m: `$LOG` §"instance ownership": `MbcCpuState` already grew by 8 bytes in Phase 0 of ASCEND (priv_level + reservation) — a further 8-byte `owner` field is an ABI v1 struct change (ADR-067) → **not this sprint**. Instead: a new small `INSTANCE_OWNER_MAP` (array, 256 × 16 bytes: `owner_flow_label_hi12: u16, owner_src_addr_hash: u64, flags`), written by the loader at boot (`finalize`), read once per tick: `if owner.flags & BOUND && (flow_label >> 8) != owner.hi12 → STAT_HIJACK_DROP, XDP_DROP`. Cost estimate: one map lookup + two compares per tick, ≈ +40-80 verified insns per path. The upper 12 bits of the flow label are the per-boot nonce bootctl already chooses (verify in Step 172; if bootctl uses a fixed label, add a random nonce there). No wire bytes change (flow label is already ours per ADR-067/Foundation-06).
- [ ] **Step 174** [CODE][W] ~20m: Tests first: host emulator (`crates/monad-mbc`) has no XDP path, so the test is in `cmd/upc-bootctl/tests/hijack_live.rs` (`#[ignore]` unless `UPC_LIVE=1`): boot instance 111 bound to nonce A; inject one tick with nonce B via `doom-go-injector`-style raw send (`grep -n 'fn send_tick\|raw' cmd/upc-bootctl/src/*.rs | head -3`) → `STAT_HIJACK_DROP` increments and `insn_count` does not advance; then nonce A → advances.
- [ ] **Step 175** [CODE] ~40m: eBPF: map decl in `monad-common`/`main.rs`, the check, the stat; loader: writes the owner record in `finalize`; bootctl's trigger sender uses the nonce. Rebuild ascend **and** Doom objects; record both `verified` counts.
- [ ] **Step 176** [TEST][B][S] ~6m: Live hijack test + harness
  ```bash
  cd "$PROJECT_ROOT/cmd/upc-bootctl" && UPC_LIVE=1 UPC_DEMO_MODE=1 sudo -E cargo test --release --test hijack_live -- --ignored 2>&1 | tail -3; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p6-regression-hijack.log" | grep -aE 'FAIL|verifier'
  ```
- [ ] **Step 177** [V] ~1m: Hijack test green; harness zero FAIL; ascend insns ≤ 950,000 and delta recorded (expect ≤ +2,000); Doom insns recorded (expect small +; Doom plays through `doom-runner`, which also writes the owner record). Over budget → revert Step 175 and STUCK Steps 178-179; ROM freeze and the secret removal stand.
- [ ] **Step 178** [SECURITY][W] ~10m: `references/security-upc-linux-spitball-2026-05-13.md` §exploit primitives: "flow-label instance hijack" → MITIGATED <date>, mechanism, residual (an attacker on the same L2 who observes the nonce can still replay: recorded, accepted for lab; Anamnesis records every tick so replay is visible).
- [ ] **Step 179** [C] ~1m: **COMMIT CHECKPOINT** (`feat(upc): [S-ASCEND-2 169-179] no shipped HMAC default; INSTANCE_OWNER_MAP binds CPU ticks to the booting flow`).
- [ ] **Step 180** [V] ~1m: **PHASE 6 EXIT GATE** — Step 163 EPERM, Step 171 zero literals, Step 177 green, budgets recorded, 12/12 SAME (`sweep | grep -c SAME`), tree clean.
  - If pass → Phase 7. If fail → fix within the phase.

---

## PHASE 7: FS SUBSTRATE — DEVICE INODES NOW, THE WRITE ROUTE DECIDED, ilp32e HONEST (Steps 181-204)

**Goal**: `open("console")`-class device inodes work; the FS-write/`>`/pipe route is decided and recorded against the verifier budget; `riscv.h`'s CSR helpers stop lying about their width. M-226, M-227, decision 48.
**Prerequisite**: Phase 3 gate (kernel/user tree stable) and Phase 1's state (flag as committed).
**Time**: ~4 h
**Agent**: Coordinator (kernel + possibly eBPF)

### The route decision

- [ ] **Step 181** [R] ~10m: Budget facts: ascend insns at Phase 6 exit (from `$LOG`), the master plan's budget lesson (1.3.1: +41K for the subdir walk, +119K blew 1M for a second variable offset), EVOLUTION-1 Stage 2's "sbrk/pipe ARRIVE in kernel".
- [ ] **Step 182** [DECIDE] ~10m: FS writes + `>` and pipes — **RECOMMENDATION: kernel-side via EVOLUTION-1 Stage 2** (the kernel's `writei`/`balloc`/`pipealloc` exist and are dormant; routing `write(FD_INODE)` and `pipe` through the trap path costs ~0 verifier insns per feature, versus ≥ +40K each in BPF at 92.7 % of the budget). Rationale: master decision 48 default; the negative Stage 1 delta says kernel paths are cheap. **Override ONLY if** E2 answers "(c) minimal" AND Stevie wants `>` before Stage 2 — then the in-BPF route with a hard +60K cap per feature. Record in `UPC-LINUX-MASTER-BATTLE-PLAN.md` 1.3.2/1.3.3 ("route: Stage 2 rung 'write' and arrival 'pipe', see EVOLUTION-1-STAGE-2-skeleton.md") and in the skeleton's rung gates.

### Device inodes (in scope now: read-only FS path, small)

- [ ] **Step 183** [R] ~10m: Where the open path classifies inode types: `grep -n 'T_FILE\|T_DIR\|T_DEVICE\|fn fs_walk\|fn open' ebpf/monad-common/src/fs_walk.rs ebpf/monad-cpu-ebpf/src/main.rs | head -20`; how `init.c` opens the console today (`grep -n 'console\|mknod\|open(' crates/xv6-mbc/upstream/user/init.c crates/xv6-mbc/upstream/user/uinit.c | head`); whether `mkfs` writes a `console` device inode (`grep -n 'T_DEVICE\|console' crates/xv6-mbc/upstream/mkfs/mkfs.c | head -5`).
- [ ] **Step 184** [DESIGN][W] ~15m: `$LOG` §"device inodes": `fs_walk` returns the inode type; on `T_DEVICE` the open handler creates an fd whose `FD_INODE_MAP` entry has `inum = DEV_CONSOLE_TAG` (e.g. `0xDEV0_0001`) instead of a disk inode; `read(5)`/`write(16)` on a tagged fd route to the TTY/KBD path (the fd 0/1/2 path already exists — reuse it by tag, no new byte-emit code). Expected cost: one compare in open + one in read/write dispatch ≈ +200-600 insns. Gate program: `user/devprobe.c` opens `console` explicitly (`open("console", O_RDWR)`), writes the token `DEV: console-open-ok` through that fd, reads nothing, exits.
- [ ] **Step 185** [CODE][W] ~15m: `mkfs` emits the `console` T_DEVICE inode if it does not (check Step 183); `user/devprobe.c` + `Makefile.mbc-userland` entry; host-emulator test `crates/monad-mbc/tests/devprobe.rs` asserting the token (RED first: `cargo test --test devprobe` fails before the handler exists).
- [ ] **Step 186** [CODE] ~40m: `fs_walk` type return + the open/read/write tag routing in `main.rs` (ascend-only `cfg`); rebuild; record insns.
- [ ] **Step 187** [TEST][B][S] ~6m: Probe + sweep + harness
  ```bash
  kbuild >/dev/null 2>&1; (cd "$UP" && rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland ramdisk target/devprobe.mbc 2>&1 | tail -1); (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1); out="$(cd "$EBPF" && UPC_DEMO_MODE=1 UPC_VERIFIER_STATS=1 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/devprobe.mbc" --triggers 1500000 --instance 111 2>&1)"; grep -aoE 'DEV: [a-z-]+|verified [0-9]+ insns' <<<"$out"; sweep | grep -c SAME; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p7-regression-dev.log" | grep -aE 'FAIL|verifier'
  ```
- [ ] **Step 188** [V] ~1m: `DEV: console-open-ok`; 12/12 SAME (`ls` output must still show `console` with its existing type column if it was already there; if `mkfs` newly added the inode, `ls` goldens change by one line → recapture with the reason); harness zero FAIL; insns ≤ 950,000 with the delta recorded. Over budget → revert Step 186, record, and move device inodes into the Stage 2 skeleton as an arrival; STUCK Steps 189-190.
- [ ] **Step 189** [CODE][W] ~5m: Add `devprobe` to the harness gates (same shape as Step 88's block, token `DEV: console-open-ok`).
- [ ] **Step 190** [C] ~1m: **COMMIT CHECKPOINT** (`feat(upc): [S-ASCEND-2 183-190] device inodes: console opens by name (T_DEVICE routed to TTY)`).

### ilp32e honesty in riscv.h

- [ ] **Step 191** [R] ~5m: `crates/xv6-mbc/upstream/kernel/types.h:7-10` (`uint64 = unsigned long` → 4 bytes on ilp32e; `pde_t = uint64`), `riscv.h` helpers (`static inline uint64 r_*`), every `uint64` use in the adapters: `grep -c 'uint64' crates/xv6-mbc/adapters/*.c crates/xv6-mbc/upstream/kernel/*.h | grep -v ':0'`.
- [ ] **Step 192** [DESIGN][W] ~15m: `$LOG` §"ilp32e types": two safe moves, one unsafe. **Safe 1**: CSR helpers return/take `uint32` (the RV32 xlen; `csrr` into a 32-bit register is exactly what the translator emits); rename nothing, change the type. **Safe 2**: `typedef unsigned long long uint64;` so the name tells the truth (8 bytes) — but every struct that uses `uint64` for an on-"disk"/ABI field (the Phase 1.7 `struct stat` lesson: size at offset 12 with 4-byte uint64) would change layout → **do not** change the typedef this sprint; instead add `_Static_assert(sizeof(uint64) == 4, "ilp32e: uint64 is 4 bytes here on purpose; see S-ASCEND-2 Phase 7")` in `types.h` and a comment on `pde_t`. **Unsafe**: changing `pde_t` width (page tables are Sv32 in Option A: 4-byte PTEs are correct). Decision: Safe 1 + the static assert.
- [ ] **Step 193** [CODE] ~20m: Apply Safe 1 to every `static inline uint64 r_*` / `w_*(uint64)` in `riscv.h` (and the adapters' copies if any), the static assert in `types.h`.
- [ ] **Step 194** [BUILD][TEST][B][S] ~6m: Kernel rebuild; the translation-count tripwire (Phase 2.3 memory) and goldens
  ```bash
  kbuild 2>&1 | tail -2; sha256sum "$K/xv6-mbc.mbc" | tee -a "$LOG"; sweep | grep -c SAME; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p7-regression-ilp32e.log" | grep -aE 'FAIL|verifier'
  ```
- [ ] **Step 195** [V] ~1m: 12/12 SAME (type width of a CSR helper cannot change TTY output); harness zero FAIL; the kernel `.mbc` hash may change (code gen for 32-bit returns) — record old/new and that the corpus is identical; ascend insns unchanged (kernel bytes are ROM data, not BPF instructions).
- [ ] **Step 196** [C] ~1m: **COMMIT CHECKPOINT** (`fix(xv6-mbc): [S-ASCEND-2 191-196] CSR helpers are uint32 on ilp32e; uint64 width asserted`).

### Close the phase

- [ ] **Step 197** [DOC-UPDATE][W] ~10m: `UPC-LINUX-MASTER-BATTLE-PLAN.md` 1.3.1 REMAINS → device inodes DONE; 1.3.2/1.3.3 → route recorded (Step 182); `wiki/Linux-on-UPC.md` FS section one line.
- [ ] **Step 198** [AUDIT-UPDATE][W] ~3m: `docs/sweeps/OUTSTANDING-2026-10-27.md` M-226 → "device inodes DONE; writes/pipes routed to Stage 2 (decision 48)"; M-227 → DONE.
- [ ] **Step 199** [TEST][B] ~3m: `cd crates/monad-mbc && cargo test --release 2>&1 | tail -1; cd ../xv6-mbc && cargo test 2>&1 | tail -1`.
- [ ] **Step 200** [V] ~30s: `0 failed` both.
- [ ] **Step 201** [B] ~1m: Tree clean check `git status --short | wc -l` → `0` after the commits.
- [ ] **Step 202** [V] ~30s: `0`.
- [ ] **Step 203** [C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [S-ASCEND-2 197-203] FS substrate status, route decision recorded`).
- [ ] **Step 204** [V] ~1m: **PHASE 7 EXIT GATE** — devprobe gated and green (or STUCK with the Stage 2 note); riscv.h change green; route decision in three docs; budgets recorded; 12/12 SAME.
  - If pass → Phase 8. If fail → fix within the phase; Phase 8 is independent.

---

## PHASE 8: MÍMIR 9/11/13 AND THE DISTRIBUTED LEG — EAST, OR TWO NETNS NAMED HONESTLY (Steps 205-242)

**Goal**: The S5 deferrals run: bootstrap over real multicast (Mímir Phase 9), stress + invalid-seal rejection (11), gate re-eval (13), and the DISTRIBUTED scorer-weights leg — on east if E3 approves, else on a two-netns west fallback whose evidence says so; the full-engine LICH-015 arm and the adversarial MBC fuzz seeds land; ADR-043 tells the truth. M-228, M-204, decision 46.
**Prerequisite**: Phase 0 gate; Phase 5's `upc-netns.sh` (reused for the second netns).
**Time**: ~6 h (east path adds ~2 h)
**Agent**: Coordinator (sudo, netns, east)

### Escalate first, prepare the fallback regardless

- [ ] **Step 205** [ESCALATE] ~—: **E3 — east as a node.** Context: S5 Steps 177 and 435 (quoted in Appendix C), east state from Step 22. Questions: (1) approve Step 177: heimdall-daemon as root from `/tmp` on east, XDP **generic** on `enp2s0` (east's only link; observe-only `XDP_PASS`), scratch Wotan on west bound to `192.168.13.2`? (2) approve Step 435: scratch Wotan reachable from east over the P2P link, `upc-cartridgectl` + `cartridge-ebpf` as root on east from `/tmp`, removed at phase end? (3) or drop Phase 18 for good (card Epic 1.5.6)? Why no recommendation: east runs the debs; host actions there are Stevie's. **Until answered: Steps 206-232 run the two-netns fallback; Steps 233-238 are the east column.**
- [ ] **Step 206** [W][CODE] ~20m: `scripts/upc-netns.sh` grows `up2` / `down2`: a second netns `upc1` with `veth-upc1`/`veth-upc1p`, both inner veths on one host bridge `br-upc` so link-local multicast crosses between them (`ip link add br-upc type bridge`, enslave the host ends, `ip link set br-upc up`); the forward-drop rule covers `veth-upc1p` too. Idempotent; `status` prints both.
- [ ] **Step 207** [TEST][B][S] ~3m: Bring both up; multicast crosses the bridge
  ```bash
  sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up2 && sudo ip netns exec upc1 ping -6 -c 2 -W 1 -I veth-upc1p ff02::1 2>&1 | grep -oE '[0-9]+ received' | head -1; sudo ip netns exec upc0 ip -6 maddr show dev veth-upc0p | grep -c 'ff02::1'
  ```
- [ ] **Step 208** [V] ~30s: `≥1 received` (the other netns answers all-nodes multicast); maddr lists `ff02::1`. `0 received` → bridge multicast snooping: `sudo ip link set br-upc type bridge mcast_snooping 0`, retry; two attempts then STUCK the multicast steps (209-214) and run bootstrap over unicast with the topology note.

### Mímir Phase 9: bootstrap over multicast (M-204)

- [ ] **Step 209** [B] ~5m: Build the pieces; lab key (0600, under `$S6`, never in the repo); seal the example manifest; scratch Wotan with a lab topic config
  ```bash
  cd "$PROJECT_ROOT" && go build -o "$S6/heimdall-daemon" ./cmd/heimdall-daemon && go build -o "$S6/gjallarhorn-sender" ./cmd/gjallarhorn-sender && go build -o "$S6/gjallarhorn-listener" ./cmd/gjallarhorn-listener && go build -o "$S6/wotan" ./services/wotan/cmd/wotan 2>/dev/null || go build -o "$S6/wotan" ./cmd/wotan; mkdir -p "$S6/catalog" && cp references/baseline/mjolnir.example.yaml "$S6/catalog/m.yaml" && "$S6/heimdall-daemon" --help 2>&1 | grep -cE 'await-bootstrap|sign-key|xdp-iface'
  ```
- [ ] **Step 210** [V] ~30s: Three binaries built; help lists the flags (count ≥ 2; `await-bootstrap` may be named differently: `grep -n 'await' cmd/heimdall-daemon/main.go | head -2` and use that name below).
- [ ] **Step 211** [B][S] ~10m: Seal + seed in `upc1`, sender in `upc0`, scratch Wotan on the host side of the bridge (loopback is not reachable from a netns: bind it to `br-upc`'s address `fd00:upc::1`)
  ```bash
  "$S6/heimdall-daemon" --manifest "$S6/catalog/m.yaml" --sign-key "$S6/lab.seed" --sign-ttl 2h 2>&1 | tail -1; ls -la "$S6/catalog/m.yaml.seal"; (cd "$S6" && WOTAN_TOPIC_CONFIG="$PROJECT_ROOT/configs/wotan-lab.yaml" nohup ./wotan --http "[fd00:upc::1]:26601" --grpc "[fd00:upc::1]:26602" >"$S6/wotan.log" 2>&1 &) ; sleep 2; curl -s "http://[fd00:upc::1]:26601/health" | head -c 80; echo
  ```
  (Check `configs/` for a lab topic config that auto-approves `heimdall`/`gjallarhorn`; if none, write `configs/wotan-lab.yaml` from the existing `wotan.yaml` with `topics.auto_approve` extended, and commit it as a lab config with no secrets.)
- [ ] **Step 212** [B][S] ~10m: Seed awaits bootstrap in `upc1` (XDP generic on `veth-upc1p`); sender emits the horn from `upc0`; time to "seed operational" ×7
  ```bash
  sudo ip netns exec upc1 env HOME="$S6" "$S6/heimdall-daemon" --manifest "$S6/catalog/m.yaml" --pubkey "$S6/lab.pub" --bpf-dir "$PROJECT_ROOT/crates/heimdall-bpf/target/bpfel-unknown-none/release" --xdp-iface veth-upc1p --xdp-mode generic --wotan-grpc "[fd00:upc::1]:26602" --node-id seed-netns1 >"$S6/seed.log" 2>&1 & sleep 3
  for i in $(seq 1 7); do t0=$(date +%s%N); sudo ip netns exec upc0 "$S6/gjallarhorn-sender" --iface veth-upc0p --manifest-sha "$(sha256sum "$S6/catalog/m.yaml" | cut -c1-64)" >/dev/null 2>&1; until grep -q 'bootstrap.*complete\|operational' "$S6/seed.log"; do sleep 0.2; [ $(( ($(date +%s%N)-t0)/1000000000 )) -gt 60 ] && break; done; echo "run $i: $(( ($(date +%s%N)-t0)/1000000 )) ms"; done | tee "$S6/p8-bootstrap-times.txt"
  ```
  (Flag names from `--help` in Step 210; the seed's log token from `grep -n 'operational\|bootstrap' cmd/heimdall-daemon/main.go | head -3`. Reset the seed's bootstrap state between runs if it only bootstraps once: restart the daemon inside the loop.)
- [ ] **Step 213** [V] ~1m: **K2** — p95 of the 7 runs < 10 s (pass), 10-15 s warn, p99 > 30 s FAIL recorded. Record p50/p95/p99 in `$LOG` with topology "2 netns on one host bridge, not across hosts".
- [ ] **Step 214** [B][SECURITY][S] ~5m: Negatives: unknown sha, sealed-but-older manifest, wrong cluster → each refused, seed keeps waiting (`grep -c 'refus' "$S6/seed.log"` rises by 3), no state change.
- [ ] **Step 215** [V] ~30s: Three refusals logged; `grep -c 'complete\|operational' "$S6/seed.log"` unchanged by the negatives.

### Mímir Phase 11: stress + invalid seals; Phase 13: gate re-eval

- [ ] **Step 216** [B][S] ~10m: Stress: 20 drift events (touch 20 catalogued files inside `upc1`'s view, or use the daemon's `--scan-interval 2s` with a scratch dir named in the manifest) + 2 invalid seals mid-run; 100 horn packets at 10/s
  ```bash
  for i in $(seq 1 20); do echo "drift-$i" | sudo tee -a "$S6/scratch/f$i" >/dev/null; sleep 0.3; done; for i in $(seq 1 100); do sudo ip netns exec upc0 "$S6/gjallarhorn-sender" --iface veth-upc0p --manifest-sha "$(printf '%064d' "$i")" >/dev/null 2>&1; sleep 0.1; done; grep -c 'drift' "$S6/seed.log"; grep -c 'refus' "$S6/seed.log"; pgrep -f heimdall-daemon | wc -l
  ```
- [ ] **Step 217** [V] ~1m: **K3** — 20/20 drifts caught (≤ 1 s each, from the seed log timestamps), 0 false alerts on untouched files, 100 bad horns refused, daemon still running (count `1`), zero restarts.
- [ ] **Step 218** [B] ~3m: Phase 13 pass-criteria table (K1 hooks verifier-accepted — from S5 Step 145 counts, unchanged unless heimdall-bpf changed; K2 Step 213; K3 Step 217; K4 hard conditions: alerts-only, no restore path, signed drift events on the scratch Wotan `curl -s "http://[fd00:upc::1]:26601/api/v1/topics/drift.detected.seed-netns1/messages?limit=1" | head -c 200`).
- [ ] **Step 219** [V] ~1m: Drift event retrieved from Wotan with a signature field; table complete; verdict recorded ("Phases 9/11/13 PASS in netns topology" or the honest failure).
- [ ] **Step 220** [C] ~1m: **COMMIT CHECKPOINT** (`test(mimir): [S-ASCEND-2 206-220] Phases 9/11/13 re-run on a two-netns west fallback; configs/wotan-lab.yaml`).

### DISTRIBUTED leg: scorer weights over Wotan to the second "node"

- [ ] **Step 221** [B][S] ~6m: Publish the scorer DATA blob to the scratch Wotan; fetch + verify + load in `upc1`
  ```bash
  cd "$PROJECT_ROOT" && go build -o "$S6/upc-cartridgectl" ./cmd/upc-cartridgectl && "$S6/upc-cartridgectl" --help 2>&1 | grep -E 'publish|fetch|weights|wotan' | head -4
  ```
  (If no publish/fetch subcommand exists, add `upc-cartridgectl weights publish|fetch` in a [CODE] step here: publish = seal the `.cart` DATA with Gungnir and POST to topic `upc.cart.weights.<model>`; fetch = subscribe, verify seal and DATA sha against the signed manifest, write to the local pin dir. Topic name per RFC-EDITOR-REQUESTS-S5 item 3, provisional.)
- [ ] **Step 222** [B][S] ~6m: Node A (host) publishes; node B (`upc1`) fetches and loads; score the same ScoreJob trace on both
  ```bash
  "$S6/upc-cartridgectl" weights publish --wotan "http://[fd00:upc::1]:26601" --cart cartridges/scorer.cart --sign-key "$S6/lab.seed" 2>&1 | tail -1; sudo ip netns exec upc1 "$S6/upc-cartridgectl" weights fetch --wotan "http://[fd00:upc::1]:26601" --model scorer --pubkey "$S6/lab.pub" --out "$S6/nodeB" 2>&1 | tail -1; sha256sum cartridges/scorer.data.bin "$S6/nodeB/scorer.data.bin" | cut -c1-16 | uniq | wc -l
  ```
- [ ] **Step 223** [V] ~1m: `1` (identical DATA sha on both nodes); seal verified on the fetched bytes (log line). Then `cart replay` the S5 ScoreJob trace against both DATA files → identical scores (`diff <(cart score --data A trace) <(cart score --data B trace) | wc -l` → `0`).
- [ ] **Step 224** [W] ~5m: Evidence into `$LOG`: "DISTRIBUTED scorer weights: cross-node demonstrated in netns, not across hosts" (the S5 Step 435 fallback wording, verbatim).
- [ ] **Step 225** [C] ~1m: **COMMIT CHECKPOINT** (`feat(cartridge): [S-ASCEND-2 221-225] weights publish/fetch over Wotan; netns cross-node demonstration`).

### Full-engine LICH-015 arm and the adversarial MBC seeds

- [ ] **Step 226** [R] ~10m: `tomb/lich/LICH-015-mbc-differential/README.md` (the two arms, `diff_harness self-test|run`), `exit-criteria.md`, `tomb/lich/UPC-CARTRIDGE-HOOKS.md` §LICH-015 merge points (arm C: full `monad-cpu-ebpf` engine incl. privileged opcodes).
- [ ] **Step 227** [CODE][W] ~45m: Arm C: extend `diff_harness` (or a sibling `diff_harness_full`) to drive `monad-cpu-ebpf` (ascend object) via `upc-bootctl --dry-run`-adjacent "run N instructions from state S" entry point if one exists (`grep -n 'single_step\|run_from_state\|--state' cmd/upc-bootctl/src/main.rs | head`), else via a tiny ROM per case booted with `--triggers N` and the CPU dump compared to the host emulator's `mbc_emulate` on the same ROM. Privileged opcodes in scope: MRET 0x47, SRET 0x48, LR.W 0x49, SC.W 0x4A, FENCE 0x3F, CSR LD/ST window. Seeds: `tomb/lich/seeds/mbc-priv/` with 8 hand-written cases (priv 0→1 via MRET, 1→3 via SRET, LR/SC pair, LR then context switch then SC (must fail), CSR write/readback, misaligned LD at the slice edge, SC without LR, FENCE no-op) plus the fuzz seeds for 0x47-0x4A random operands.
- [ ] **Step 228** [TEST][B][S] ~15m: Self-test, then a bounded run
  ```bash
  cd "$PROJECT_ROOT/tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness" && cargo build --release 2>&1 | tail -1 && ./target/release/diff_harness self-test 2>&1 | tail -2 && UPC_DEMO_MODE=1 sudo -E ./target/release/diff_harness run --arm full --iters 2000 --seeds ../../../seeds/mbc-priv --out "../../results/$(date +%Y%m%d)_full" 2>&1 | tail -5
  ```
- [ ] **Step 229** [V] ~2m: Self-test green; `agree N/N` on the eight hand seeds; any divergence is a **finding** (register it `LICH-015-F<n>` in `tomb/lich/LICH-015-mbc-differential/status.md` with the case bytes; do not fix in this phase unless it is the Phase 3 Hn4 case).
- [ ] **Step 230** [B] ~6m: Cartridge differential still green after the sprint's object changes (Phase 6 touched monad-cpu-ebpf, not cartridge-ebpf; this proves it)
  ```bash
  cd "$PROJECT_ROOT/pkg/cartridge" && go test -c -o /tmp/cart.test . && sudo CART_DIFF_COUNT=1000 /tmp/cart.test -test.run ThreeEngines -test.v 2>&1 | tail -3; cd "$PROJECT_ROOT/crates/monad-mbc" && timeout 320 cargo +nightly fuzz run fuzz_cart_differential -- -max_total_time=300 2>&1 | tail -2
  ```
- [ ] **Step 231** [V] ~1m: `1000/1000` agree; fuzz: no crash in 300 s.
- [ ] **Step 232** [C] ~1m: **COMMIT CHECKPOINT** (`test(lich): [S-ASCEND-2 226-232] LICH-015 full-engine arm + privileged-opcode seeds`).

### The east column (only under E3)

- [ ] **Step 233** [AFTER-E3][LIVE-RW][BARE-METAL][S][B] ~15m: Stage the throwaway on east (`scp "$S6/heimdall-daemon" "$S6/catalog/m.yaml" "$S6/catalog/m.yaml.seal" "$S6/lab.pub" east:/tmp/s6/`), scratch Wotan on west bound to `192.168.13.2:26601/26602`, seed on east with `--xdp-iface enp2s0 --xdp-mode generic`, sender from west over the P2P link; repeat Steps 212-219 with `topology = "2 physical hosts (west sender, east seed)"`.
- [ ] **Step 234** [V] ~1m: K2/K3 recorded for the physical topology; XDP detached and `/tmp/s6` removed on east at the end (`ssh east 'sudo ip link set enp2s0 xdpgeneric off; rm -rf /tmp/s6; pgrep -f heimdall || echo clean'`).
- [ ] **Step 235** [AFTER-E3][LIVE-RW][BARE-METAL][S][B] ~15m: DISTRIBUTED leg across hosts: `upc-cartridgectl weights fetch` on east from the west scratch Wotan, `cartridge-ebpf` loaded on east's `enp2s0` in a netns on east (never the bare link), scores compared (Steps 222-223 shape).
- [ ] **Step 236** [V] ~1m: Identical DATA sha and scores across hosts; east cleaned (`ssh east 'sudo bpftool prog show | grep -c cart'` → `0`).
- [ ] **Step 237** [AFTER-E3][W] ~5m: `$LOG` evidence rows switch wording to "across hosts"; S5 plan Steps 177/435 get "answered <date> by S-ASCEND-2 E3".
- [ ] **Step 238** [AFTER-E3][C] ~1m: **COMMIT CHECKPOINT** (`test(upc): [S-ASCEND-2 233-238] Mímir 9/11/13 + DISTRIBUTED leg across west and east (E3)`).

### Truth in ADR-043, teardown

- [ ] **Step 239** [DOC-UPDATE][W] ~15m: `docs/adr/ADR-043-*.md:3` status line → "PoC COMPLETE (2026-04-11); Phases 9/11/13 re-run <date> on <topology> (S-ASCEND-2 Phase 8), DISTRIBUTED leg <topology>; see references/s-ascend-2-log.md"; the Gate table rows for 9/11/13 updated; `wiki/Mimirs-Law.md` + CLAUDE.md's Mímir bullet ("11 of 13 phases complete") corrected to match.
- [ ] **Step 240** [B][S] ~2m: Teardown: scratch Wotan, seeds, both netns (if Phase 5 installed the oneshot on west, bring only `upc1` + bridge down and leave `upc0` to the unit)
  ```bash
  pkill -f "$S6/wotan" ; sudo pkill -f "$S6/heimdall-daemon"; sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" down2; sudo bpftool prog show 2>/dev/null | grep -ciE 'heimdall|cart|monad'; ip netns list | wc -l
  ```
- [ ] **Step 241** [V] ~30s: `0` programs; netns count equals the pre-phase count (Step 22) or `1` if the oneshot owns `upc0`.
- [ ] **Step 242** [V][C] ~2m: **PHASE 8 EXIT GATE** — Mímir 9/11/13 evidence with honest topology; DISTRIBUTED leg demonstrated (netns at minimum); LICH-015 full arm green or findings registered; ADR-043 truthful; west clean.
  ```bash
  cd "$PROJECT_ROOT" && git add -A docs/adr wiki/Mimirs-Law.md CLAUDE.md references/s-ascend-2-log.md tomb/lich && commitp -m "docs(mimir): [S-ASCEND-2 239-242] ADR-043 status tells the truth about Phases 9/11/13 and the DISTRIBUTED leg"
  ```
  - If pass → Phase 9. If fail → fix within the phase; Phases 9-11 are independent.

---
## PHASE 9: CARTRIDGE LIVE LEGS AND A SCORER THAT HAS SEEN REAL TRAFFIC (Steps 243-266)

**Goal**: `anomaly.scores` flows over a real Wotan, the Sophia registry backend is exercised against a running Sophia, the int8 scorer is retrained on real `steel-echo` flows with a held-out eval, and the interpreter-cost gates get their decision. M-229, decision 47.
**Prerequisite**: Phase 0 gate; the main compose stack can be started on west (Wotan + Sophia only).
**Time**: ~4 h
**Agent**: Agent [P:8] for Steps 243-256 (host-side Go, no sudo); Coordinator for 257-266

### Decisions first

- [ ] **Step 243** [DECIDE] ~5m: Interpreter-cost gates #1(a) and #2 — **RECOMMENDATION: accept as the documented cost of interpreted cartridges** (rate limiter +193/+186 ns vs 150 bar; sampler +216 vs 120); do not fund callback batching or DOT4 this sprint. Rationale: #1(b) (1.25× native) and #3 (232,450 flows/s) pass; a new MBC opcode is a full ISA spec gate (ADR-108 §DOT4) and its own sprint. **Override ONLY if** Stevie funds "cartridges at line rate" explicitly. Record in `ADR-108` §Benchmark gates ("accepted <date>, S-ASCEND-2 Step 243") and in `$LOG`.
- [ ] **Step 244** [DECIDE] ~5m: DOT4 / any new opcode — **RECOMMENDATION: no ISA sprint now**; ADR-108's measured ≈1.7× scorer case stays "material for a future ISA spec gate". **Override ONLY if** decision 243 is overridden.
- [ ] **Step 245** [B] ~3m: Measure, do not ship, the 8-steps-per-callback variant (S5 estimated −15 % per instruction for twice the verified callback): if a `STEPS_PER_CALLBACK` const exists in `ebpf/cartridge-ebpf` (`grep -n 'STEPS_PER\|steps_per\|bpf_loop' ebpf/cartridge-ebpf/src/*.rs | head -5`), build with 8, record `cart_xdp`/`cart_tick` verified insns and the Phase 13 bench (`cd pkg/cartridge && go test -run Bench -bench . -count=1 2>&1 | tail -6` per `bench_linux_test.go`), then **revert**. Numbers into `$LOG` §"callback batching" for the future decision.
- [ ] **Step 246** [V] ~30s: Tree back to HEAD (`git status --short ebpf/ | wc -l` → `0`); numbers recorded.

### Live leg 1: `anomaly.scores` over Wotan

- [ ] **Step 247** [B][S] ~4m: Start only Wotan from the main compose (observe the kingdom bring-up notes in `next.md` S21; the Well is not needed)
  ```bash
  cd "$PROJECT_ROOT" && sudo docker compose up -d wotan 2>&1 | tail -1 && sleep 3 && curl -s http://127.0.0.1:18000/health | head -c 100; echo
  ```
- [ ] **Step 248** [B] ~5m: `cmd/anomaly --wotan 127.0.0.1:18000` with the in-process intake off; feed the S5 ScoreJob trace through `upc-cartridgectl` (or `cart replay` → `cmd/anomaly`'s event input: `grep -n 'events\|stdin\|--trace' cmd/anomaly/main.go | head -5`); read the topic back
  ```bash
  cd "$PROJECT_ROOT" && (go run ./cmd/anomaly --wotan 127.0.0.1:18000 --events "$PROJECT_ROOT/pkg/cartridge/testdata/scorer-trace.jsonl" >"$S6/anomaly.log" 2>&1 &) ; sleep 5; curl -s "http://127.0.0.1:18000/api/v1/topics/anomaly.scores/messages?limit=3" | head -c 400; echo; grep -c 'published' "$S6/anomaly.log"
  ```
  (Flag and testdata names from the code; if `anomaly.scores` is not in Wotan's `topics.auto_approve`, the publish fails with "pending approval" — that itself is the finding: add it to `configs/wotan.yaml` in a [CODE] step, restart Wotan, retry.)
- [ ] **Step 249** [V] ~1m: **SCORES ON THE BUS** — ≥1 message on `anomaly.scores` carrying flow metadata and a score, **no payload bytes** (DoD-STD 8: `grep -c 'payload' <<<"$(curl ...)"` → `0`); `services/anomaly` consumer (if it runs as part of `cmd/anomaly`) logs the intake.
- [ ] **Step 250** [C] ~1m: **COMMIT CHECKPOINT** (`feat(anomaly): [S-ASCEND-2 247-250] anomaly.scores proven over a live Wotan; topic auto-approve` — only if something changed; else skip).

### Live leg 2: Sophia registry backend

- [ ] **Step 251** [B][S] ~4m: Start Sophia; register a cartridge record through the Sophia backend; read it back
  ```bash
  cd "$PROJECT_ROOT" && sudo docker compose up -d sophia 2>&1 | tail -1 && sleep 3 && curl -s http://127.0.0.1:19005/health | head -c 80; echo; SOPHIA_TOKEN="${SOPHIA_TOKEN:-}" go run ./cmd/upc-cartridgectl --sophia http://127.0.0.1:19005 --pubkey "$S6/lab.pub" registry put --cart cartridges/scorer.cart --seal "$S6/scorer.seal" 2>&1 | tail -2; go run ./cmd/upc-cartridgectl --sophia http://127.0.0.1:19005 registry get scorer 2>&1 | head -3
  ```
  (Subcommand names from `--help`; `scorer.seal` produced with the lab key via `cart seal` or `upc-cartridgectl prepare` — check `prepare.go`. If Sophia requires a service token (`SOPHIA_TOKEN`), use a lab token minted by the service's own dev path, never a real one, and never write it to the repo.)
- [ ] **Step 252** [V] ~1m: `get` returns the record with the same DATA sha as the local `.cart`; Sophia's knowledge store shows the `x-cartridge` namespace entry (`curl -s 'http://127.0.0.1:19005/api/v1/knowledge?subject=x-cartridge/scorer' | head -c 200`). `KF5` from S5 ("stack down") is closed by this step: note it in `$LOG`.
- [ ] **Step 253** [B][S] ~1m: Stop what this phase started (`sudo docker compose stop wotan sophia`; `pkill -f 'cmd/anomaly'`).

### The scorer meets real flows

- [ ] **Step 254** [R] ~10m: How the scorer was trained (`cartridges/scorer.model.json` provenance block, `crates/upc-cartridge/src/scorer.rs` feature list, the S5 Phase 15 steps 367-396 for the quantization gate and held-out protocol) and what `steel-echo`'s load meter and the flow sampler can export (`grep -n 'features\|window_bytes\|syn' cmd/steel-echo/monad.go crates/upc-cartridge/src/scorer.rs | head -10`).
- [ ] **Step 255** [DECIDE] ~5m: Scorer data — **RECOMMENDATION: retrain on real `steel-echo` flows captured on west (labelled: steady-state = normal; the Real Steel game-day fault windows = anomalous), keep the ten synthetic families as a minority augmentation, hold out 20 % of real flows by time (last fifth), and keep the scorer a demonstrator until a second real source exists.** Rationale: master decision 47 default; memory feedback_memorization_vs_learning — always a held-out eval. **Override ONLY if** no labelled fault window exists in the Real Steel captures (then collect one in a 10-minute attended window: `chaos-controller` netem on steel-echo with the sampler on; this is a [LIVE-RW] under the S-REAL-STEEL window rules and needs Stevie's word → fold into E5).
- [ ] **Step 256** [B] ~10m: Extract real flow features from the Real Steel evidence (game-day sampler output, `docs/gamedays/2026-10-26-real-steel-1.md` points at the capture dir; else `references/`/`$HOME/tmp` per the S-REAL-STEEL log) into `eval/scorer/real-flows-2026-10.jsonl` with `label` and `t`; split by time
  ```bash
  cd "$PROJECT_ROOT" && mkdir -p eval/scorer && python3 -I scripts/cartridge/scorer-dataset.py --in "<capture dir from the game-day log>" --out eval/scorer/real-flows-2026-10.jsonl --holdout-frac 0.2 --by time 2>&1 | tail -3; wc -l eval/scorer/real-flows-2026-10.jsonl eval/scorer/real-flows-2026-10.holdout.jsonl
  ```
  (Write `scripts/cartridge/scorer-dataset.py` in this step if it does not exist: stdlib only, reads the sampler's JSONL, emits the scorer's feature vector + label; unit test in `scripts/cartridge/test_scorer_dataset.py`; `python3 -I` because the capture dir is data, not code.)
- [ ] **Step 257** [V] ~1m: Train and holdout files non-empty; both labels present in each (`jq -r .label eval/scorer/*.jsonl | sort | uniq -c`); no payload fields (`grep -c payload eval/scorer/real-flows-2026-10.jsonl` → `0`).
- [ ] **Step 258** [B] ~10m: Retrain with the existing trainer (S5 Phase 15: `grep -n 'train\|fit' crates/upc-cartridge/src/scorer.rs scripts/cartridge/*.py | head -5`), real + synthetic minority, quantize int8, eval on the held-out real set and on the old synthetic held-out
  ```bash
  cd "$PROJECT_ROOT" && python3 -I scripts/cartridge/scorer-train.py --train eval/scorer/real-flows-2026-10.jsonl --augment cartridges/synthetic-families.jsonl --out "$S6/scorer-v2.model.json" --data-out "$S6/scorer-v2.data.bin" 2>&1 | tail -3; python3 -I scripts/cartridge/scorer-eval.py --model "$S6/scorer-v2.model.json" --eval eval/scorer/real-flows-2026-10.holdout.jsonl 2>&1 | tail -4
  ```
- [ ] **Step 259** [V] ~2m: **HELD-OUT GATE** — held-out real AUC ≥ 0.80 and the quantization gate (int8 vs f32 score agreement ≥ 99 % on the holdout) passes; the old synthetic holdout does not collapse (AUC ≥ 0.70). Record both AUCs, the monotonicity-in-window-bytes check S5 flagged, and whether the SYN count is now used. Below bar → keep v1 shipped, record "real-data retrain insufficient: N flows", STUCK Steps 260-262 (demonstrator status stands).
- [ ] **Step 260** [B] ~5m: Ship v2 under the signed-loader path: regenerate `cartridges/scorer.model.json`/`scorer.data.bin`/`scorer.s` provenance (dataset sha, date, holdout AUCs), re-run the three-engine differential on the new DATA, `cart replay` the S5 trace (scores will differ: that is expected; determinism across engines must not)
  ```bash
  cd "$PROJECT_ROOT" && cp "$S6/scorer-v2.model.json" cartridges/scorer.model.json && cp "$S6/scorer-v2.data.bin" cartridges/scorer.data.bin && (cd pkg/cartridge && go test -c -o /tmp/cart.test . && sudo CART_DIFF_COUNT=300 /tmp/cart.test -test.run 'ThreeEngines|Scorer' -test.v 2>&1 | tail -3)
  ```
- [ ] **Step 261** [V] ~1m: Differential `300/300`; scorer tests green; `bash scripts/upc-regression.sh 2>&1 | grep -aE 'cart_(xdp|tick)|FAIL'` → cartridge budgets unchanged (DATA is not code), zero FAIL.
- [ ] **Step 262** [C] ~1m: **COMMIT CHECKPOINT** (`feat(cartridge): [S-ASCEND-2 254-262] int8 scorer retrained on real steel-echo flows with a time-split holdout; dataset + trainer scripts`).
- [ ] **Step 263** [DOC-UPDATE][W] ~10m: `ADR-108` §Known limitations: scorer paragraph rewritten with the real-data facts (or the honest "still synthetic: N real flows insufficient"); gates #1(a)/#2 "accepted <date>"; `wiki/UPC-Cartridges.md` scorer + live-legs lines; `references/s5-upc-wire-log.md` §Known limitations gets a one-line forward pointer.
- [ ] **Step 264** [AUDIT-UPDATE][W] ~3m: `docs/sweeps/OUTSTANDING-2026-10-27.md` M-229 → DONE/partial with step numbers.
- [ ] **Step 265** [C] ~1m: **COMMIT CHECKPOINT** (`docs(cartridge): [S-ASCEND-2 263-265] ADR-108 limitations and gate decisions recorded`).
- [ ] **Step 266** [V] ~1m: **PHASE 9 EXIT GATE** — Steps 249, 252 live legs proven; Step 259 gate passed or honestly failed; decisions 243/244/255 recorded in ADR-108; compose services stopped; tree clean.
  - If pass → Phase 10. If fail → fix within the phase.

---

## PHASE 10: LICH-012 L12a/L12d, ENKRATEIA v2, SEMANTIC DRIFT (Steps 267-280)

**Goal**: The two LICH-012 arms never run get run; Enkrateia's auto-restore v2 and the semantic drift diff move from "named" to "designed and gated". M-202.
**Prerequisite**: Phase 8's netns tooling (L12d needs a live attach in a netns); Phase 5's `upc-netns.sh`.
**Time**: ~3 h
**Agent**: Coordinator (sudo for L12d)

- [ ] **Step 267** [R] ~15m: `tomb/lich/LICH-012-config-convergence/README.md` (L12a rows: "key exfiltration from dev key storage: not attempted"; L12d: "heimdall-bpf verifier bombs, ringbuf overflow, map poisoning: not run"), `pkg/enkrateia/` (alerts-only aggregator; what "auto-restore v2" means per ADR-043 §hard conditions — alerts-only is a hard condition, so "restore" must be a **proposal**, never a mutation), `pkg/gungnir/` key storage paths.
- [ ] **Step 268** [SECURITY][B] ~15m: L12a key exfiltration, lab key only: where does a dev `--sign-key` seed live, with what mode, and what reads it?
  ```bash
  cd "$PROJECT_ROOT" && grep -rn 'sign-key\|driftSignKey\|0600\|ReadFile' cmd/heimdall-daemon/main.go pkg/gungnir/*.go | grep -v _test | head -10; ls -la "$S6/lab.seed"; strace -f -e trace=openat -o "$S6/l12a.strace" "$S6/heimdall-daemon" --manifest "$S6/catalog/m.yaml" --sign-key "$S6/lab.seed" --sign-ttl 1h >/dev/null 2>&1; grep -c 'lab.seed' "$S6/l12a.strace"; grep -E 'O_CREAT|O_WRONLY' "$S6/l12a.strace" | grep -v 'seal' | head -3
  ```
- [ ] **Step 269** [V] ~2m: The seed is opened exactly once, read-only, from the 0600 file; no copy is written anywhere but the `.seal`; the daemon refuses a seed file with mode ≠ 0600 (if it does not: [CODE] add the mode check with a test, `pkg/gungnir`, fail-loud). Record the L12a row as "attempted <date>: no exfil path found / finding F-n fixed".
- [ ] **Step 270** [SECURITY][B][S] ~25m: L12d in `upc1`: heimdall-bpf under hostile input — (a) ringbuf overflow: generate 10,000 `vfs_write` events in 1 s inside the netns (`dd if=/dev/zero of=$S6/scratch/x bs=1 count=10000 conv=notrunc` loop) with the daemon's reader paused (`kill -STOP`), resume, count drops; (b) map poisoning: attempt `bpftool map update` on the daemon's maps as an unprivileged user → EPERM; (c) verifier bombs are not applicable to a loaded object (the object is fixed) — record as N/A with the reason.
  ```bash
  sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" up2; sudo ip netns exec upc1 "$S6/heimdall-daemon" --manifest "$S6/catalog/m.yaml" --pubkey "$S6/lab.pub" --bpf-dir "$PROJECT_ROOT/crates/heimdall-bpf/target/bpfel-unknown-none/release" --node-id l12d >"$S6/l12d.log" 2>&1 & sleep 2; pid=$!; sudo kill -STOP $pid; for i in $(seq 1 10000); do echo x >> "$S6/scratch/l12d-$((i%50))"; done; sudo kill -CONT $pid; sleep 3; grep -ciE 'drop|lost|overflow' "$S6/l12d.log"; grep -c 'drift' "$S6/l12d.log"; sudo -u nobody bpftool map update id "$(sudo bpftool map show | grep -i heimdall | head -1 | cut -d: -f1)" key hex 00 value hex 00 2>&1 | head -1; sudo kill $pid
  ```
- [ ] **Step 271** [V] ~2m: (a) the daemon reports lost events as a counted metric/log (not silently): count ≥ 1 and the process survives; if it silently loses them → [CODE] add the `ringbuf_lost_total` counter + log line (fail-loud) with a test; (b) `Operation not permitted`. Record both in the LICH-012 README table.
- [ ] **Step 272** [DESIGN][W] ~20m: `pkg/enkrateia` "auto-restore v2" as a **proposal emitter**: on a drift event with a known-good sealed baseline, Enkrateia publishes `drift.restore_proposal.<node>` carrying the file, the expected sha, and the exact `unheaded-remediate` command — and nothing executes it (ADR-043 hard condition #1 alerts-only stays). Semantic drift diff: for text configs, a normalized diff (whitespace/comment-insensitive, key-order-insensitive for YAML) so a cosmetic reorder is `semantic: false`. Both as design sections in `tomb/lich/LICH-012-config-convergence/README.md` §"v2 design", with the test list.
- [ ] **Step 273** [CODE][W] ~30m: Tests first in `pkg/enkrateia/`: proposal emitted with the three fields and no FS mutation (`zero FS mutations enforced` test extended); semantic diff: reorder-only YAML → `semantic=false`, value change → `true`, comment-only → `false`.
- [ ] **Step 274** [CODE] ~40m: Implement both; `go test ./pkg/enkrateia/... -race -count=1`.
- [ ] **Step 275** [V] ~1m: Green; `grep -rn 'os.WriteFile\|os.Remove\|Rename' pkg/enkrateia/*.go | grep -v _test | wc -l` → `0` (alerts-only invariant by grep).
- [ ] **Step 276** [B] ~2m: Topic registration: `drift.restore_proposal.*` added to `configs/wotan.yaml` auto-approve for `heimdall` (and the lab config); `WOTAN_TOPICS.md` row (provisional, flagged for the RFC Editor desk in S-TRUE-WIRE).
- [ ] **Step 277** [C] ~1m: **COMMIT CHECKPOINT** (`feat(enkrateia): [S-ASCEND-2 267-277] L12a/L12d run; restore proposals (alerts-only) and semantic drift diff`).
- [ ] **Step 278** [DOC-UPDATE][W] ~10m: LICH-012 README status line → "L12a attempted, L12d run <date>"; `wiki/Mimirs-Law.md` Enkrateia paragraph; `docs/sweeps/OUTSTANDING-2026-10-27.md` M-202 → DONE (CTF activation wiring stays with ADR-062's rule → note "not in scope: ADR-062 gate").
- [ ] **Step 279** [B][S] ~1m: `sudo bash "$PROJECT_ROOT/scripts/upc-netns.sh" down2`; `sudo bpftool prog show | grep -ci heimdall` → `0`.
- [ ] **Step 280** [V][C] ~1m: **PHASE 10 EXIT GATE** — L12a/L12d rows filled with evidence; Enkrateia tests green with the alerts-only grep at `0`; netns down; committed.
  - If pass → Phase 11. If fail → fix within the phase.

---

## PHASE 11: THE NAME, THE STATUSES, THE ATTENDED RUN (Steps 281-292)

**Goal**: The naming ceremony is held or formally queued, stale ADR/wiki statuses tell the truth, the dead stub crate is decided, and the one attended end-to-end run is scripted and (if E5) performed. M-230, M-231, decision 49.
**Prerequisite**: none beyond Phase 0 (docs) and Phase 5 (console path) for Step 290.
**Time**: ~2 h (plus E4/E5 waits)
**Agent**: Agent [P:9] for 281-288; Coordinator for 289-292

- [ ] **Step 281** [DECIDE] ~5m: `crates/upc-bootstub` — **RECOMMENDATION: delete** (a stage-1 stub for uClinux/full Linux; ADR-081 chose not to vendor uClinux; nothing builds or imports it: `grep -rn 'upc-bootstub' --include=Cargo.toml --include=Makefile* . | grep -v 'crates/upc-bootstub/'` → only workspace mention if any). Rationale: ADR-090 rubric. **Override ONLY if** the Yggdrasil golden-image path (nix/yggdrasil) names it as its boot stub: `grep -rn bootstub nix/ docs/OS-FORK-DISCIPLINE.md | head -3`.
- [ ] **Step 282** [B] ~3m: Apply: remove the crate dir, workspace member line if any, `wiki/Project-Structure.md:14` and `wiki/Rust-Components.md:34` rows, CLAUDE.md:1360 paragraph → one sentence "retired <date> (ADR-081)"; `cargo build` at the workspace root still green (`cd crates && ls; cargo metadata --no-deps 2>&1 | tail -1`).
- [ ] **Step 283** [DOC-UPDATE][W] ~15m: ADR statuses: `ADR-023-wotan-virtual-memory.md:3` → "Superseded by ADR-074 (per-pid page tables, 2026-05-13)"; `ADR-081` gains a status note "working name `unheadnix` chosen 2026-07-17; ceremony: see S-ASCEND-2 E4"; `ADR-INDEX.md` rows for 023 and 043 updated.
- [ ] **Step 284** [ESCALATE] ~—: **E4 — the naming ceremony.** Context: Q5 from Phase 2.4 (2026-07-07), working name `unheadnix` (2026-07-17), 13 files still say "Unheaded Linux". Question: is `unheadnix` the name (Lore to bless), or another? Why no recommendation: names are Stevie's and Lore's. **Until answered: Steps 285-286 skip.**
- [ ] **Step 285** [AFTER-E4][B] ~5m: Find-replace with review (no blind sed over wiki links)
  ```bash
  cd "$PROJECT_ROOT" && grep -rln 'Unheaded Linux' docs wiki CLAUDE.md README.md | tee "$S6/rename-files.txt" | wc -l; for f in $(cat "$S6/rename-files.txt"); do sed -i 's/Unheaded Linux/<NAME>/g' "$f"; done; grep -rn '<NAME>' docs wiki CLAUDE.md README.md | wc -l
  ```
  (Replace `<NAME>` with the blessed name; keep "Unheaded Linux" only inside historical quotes that the diff shows are verbatim citations — revert those hunks by hand.)
- [ ] **Step 286** [AFTER-E4][W] ~10m: `docs/lore/` ceremony record (one page: name, date, Lore's blessing line, what it names: the L6 own-OS rung); `wiki/UPC-Dream-Ladder.md` L6 row and §L6 heading use the name; `ADR-081` status note → "named <NAME> <date>".
- [ ] **Step 287** [DOC-UPDATE][W] ~10m: `wiki/UPC-Dream-Ladder.md` L6 rung text reconciled regardless of the name: drop the uClinux/FUZIX rung wording (ADR-081), point Phase 2.5 (pool-node image) at the master plan Track 2 and at the Stage 2 skeleton; `docs/doom/ROAD_TO_LINUX*.md`/`UCLINUX*`/`FUZIX*` (if present: `ls docs/doom | grep -iE 'linux|uclinux|fuzix'`) get a "superseded by ADR-081" banner (doc-only; not Doom work).
- [ ] **Step 288** [C] ~1m: **COMMIT CHECKPOINT** (`docs(upc): [S-ASCEND-2 281-288] bootstub retired; ADR-023 superseded; L6 rung reconciled; E4 queued`).
- [ ] **Step 289** [W] ~15m: The attended-run recipe, `docs/doom/../wiki/Linux-on-UPC.md` §"Attended end-to-end (browser)": start `upc-tty-bridge` (26100, loopback), start the boot through the unit (`systemctl start unheaded-upc-bootctl@xv6`) or the lab recipe, open `dashboard/upc-console.html`, type `echo peace and love`, expect the echo; then `LICH_E2E=1 go test ./pkg/lichtools/ -run E2E -count=1` with the model already running via `scripts/switch-model.sh qwen-7b` (fit check; never start llama-server by hand). Dry-run every command that does not need eyes (`go test -run E2E` without the env → skipped cleanly; bridge starts and `/health` answers).
- [ ] **Step 290** [ESCALATE] ~—: **E5 — one attended run.** Context: Step 289 recipe. Question: when can Stevie sit at west for ~20 minutes (browser + GPU window) to run it? Why no recommendation: needs eyes and the GPU. **Until answered: marked "recipe written, not run live".**
- [ ] **Step 291** [AFTER-E5][LIVE-RW][BARE-METAL][B] ~20m: Run the recipe with Stevie; paste the observed lines (echo in the browser; `E2E` test PASS) into `$LOG`; `docs/sweeps/OUTSTANDING-2026-10-27.md` M-231 → DONE.
- [ ] **Step 292** [V][C] ~1m: **PHASE 11 EXIT GATE** — Steps 281-283, 287 committed; E4/E5 answered or recorded open with their fallbacks; tree clean.
  - If pass → Phase 12. If fail → fix within the phase.

---

## PHASE 12: SECURITY AND COMPLIANCE REVIEW (Steps 293-302)

**Goal**: Every attack-surface change this sprint made is listed, fuzzed where it parses, and gated; the repo gates are green. DoD-STD 6.
**Prerequisite**: Phases 1-11 (whatever ran).
**Time**: ~1.5 h
**Agent**: Coordinator

- [ ] **Step 293** [SECURITY][W] ~20m: Attack-surface table in `$LOG` §"Phase 12 review" (one row per change): `INSTANCE_OWNER_MAP` + nonce (new map, new compare; residual replay noted); `UPC_SURFACE_ID` reader (new ELF parsing in the loader → fuzz target); `SliceGeometry` validation (new arithmetic → property test for overflow); BootParams v3 SHA window (new bytes the kernel reads); `upc-netns.sh` (root script: shellcheck, no user input besides `up|down|status|up2|down2`); `unheaded-upc-bootctl@` unit (caps, seccomp filter text); `weights publish|fetch` (new Wotan consumer: seal verified before any write; path traversal in `--out`); `scorer-dataset.py` (reads captures with `-I`); Enkrateia proposals (new topic, no mutation); `drift.restore_proposal.*` auto-approve entry; `UPC_HMAC_KEY` env (fail-loud); the retired Go `mapfreeze.go` and `upc-bootstub` (surface removed).
- [ ] **Step 294** [SECURITY][CODE][W] ~25m: Fuzz targets where bytes are parsed: `crates/upc-api/fuzz/fuzz_object_facts.rs` (arbitrary ELF bytes → never panics, returns `Err` or a 32-byte id) and `fuzz_slice_geometry.rs` (arbitrary `SliceGeometry` + capacities → no overflow panic); register both in `tomb/lich/UPC-CARTRIDGE-HOOKS.md`'s sibling table (or a new `tomb/lich/UPC-LOADER-HOOKS.md`).
- [ ] **Step 295** [TEST][B] ~12m: Run them bounded
  ```bash
  cd "$PROJECT_ROOT/crates/upc-api" && timeout 400 cargo +nightly fuzz run fuzz_object_facts -- -max_total_time=180 2>&1 | tail -2; timeout 400 cargo +nightly fuzz run fuzz_slice_geometry -- -max_total_time=180 2>&1 | tail -2
  ```
- [ ] **Step 296** [V] ~1m: No crashes. A crash → fix (it is a loader panic on hostile bytes: real finding, `FND-` row), re-run.
- [ ] **Step 297** [COMPLIANCE][B] ~6m: Repo gates
  ```bash
  cd "$PROJECT_ROOT" && bash scripts/check-gitleaks.sh 2>&1 | tail -1; bash scripts/check-gosec-ratchet.sh 2>&1 | tail -2; bash scripts/check-dependency-baseline.sh 2>&1 | tail -2; bash scripts/check-systemd-units.sh 2>&1 | tail -2; make license-check 2>&1 | tail -2; bash scripts/check-runbooks.sh 2>&1 | tail -1; bash scripts/check-python-syntax.sh 2>&1 | tail -1; go mod tidy -diff | wc -l; (cd crates/upc-api && cargo tree --depth 1 2>/dev/null | grep -cE '^[a-z]' )
  ```
- [ ] **Step 298** [V] ~2m: All PASS; `go mod tidy -diff` → `0`; no new crate at depth 1 in `upc-api` beyond what Phase 4 recorded (`object`/`goblin` if added: Barrister/ADR-004 note in `$LOG` + `docs/adr/ADR-004` admission line — a new Rust crate is an admission, record it).
- [ ] **Step 299** [TEST][B] ~15m: Full test sweep for the KNOWN FAILURES BASELINE comparison
  ```bash
  cd "$PROJECT_ROOT" && go test ./... 2>&1 | grep -E '^(FAIL|---)' | grep -v '^ok' > "$S6/p12-go-fail.txt"; diff <(sort "$S6/p0-go-fail.txt") <(sort "$S6/p12-go-fail.txt") | grep '^>' | head; for c in crates/upc-api cmd/upc-bootctl crates/monad-mbc crates/xv6-mbc crates/upc-cartridge crates/doom-runner; do (cd "$c" && cargo test --release 2>&1 | grep -E 'test result' | tail -1 | sed "s#^#$c: #"); done
  ```
- [ ] **Step 300** [V] ~1m: Zero `>` lines (no NEW Go failures vs Phase 0); every Rust crate `0 failed`.
- [ ] **Step 301** [SECURITY][W] ~10m: Register rows: `UPC-ISO-01` (Phase 5), `UPC-HIJACK-01` mitigated (Phase 6), any LICH-015 full-arm finding, any fuzz crash → `compliance/findings/register.yaml` with evidence paths; regenerate the ADR-098 section
  ```bash
  cd "$PROJECT_ROOT" && go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md 2>&1 | tail -2 && git diff --stat docs/adr/ADR-098-findings-register-and-baseline-modes.md | tail -1
  ```
- [ ] **Step 302** [V][C] ~2m: **PHASE 12 EXIT GATE** — table complete; fuzz clean; gates green; baseline comparison zero new; register regenerated; committed (`chore(security): [S-ASCEND-2 293-302] attack-surface review, loader fuzz targets, register rows`).
  - If pass → Phase 13. If fail → DO NOT PROCEED; fix within the phase.

---

## PHASE 13: DOCS, REGISTERS, HANDOFF (Steps 303-314)

**Goal**: Every downstream doc tells the truth about what this sprint changed; the master list rows are ticked; the next session can cold-start. DoD-STD 4 and the Librarian ripple.
**Prerequisite**: Phase 12 gate.
**Time**: ~1.5 h
**Agent**: Agent (Librarian lens) for 303-308; Coordinator for 309-314

- [ ] **Step 303** [DOC-UPDATE][W] ~20m: `wiki/`: `Linux-on-UPC.md` (unit-first boot, device inodes, pid numbering, I/O gates), `UPC-Dream-Ladder.md` (L5 row: N-way isolation, getpid flip state; L6 rung text), `UPC-Cartridges.md` (live legs, scorer v2, gate decisions), `Mimirs-Law.md` (Phases 9/11/13 topology, Enkrateia proposals), `Service-Isolation.md`/uid table (`unheaded-upc`), `Port-Registry.md` unchanged (no new port) — verify with `grep -n 2661 wiki/Port-Registry.md`.
- [ ] **Step 304** [DOC-UPDATE][W] ~15m: `CLAUDE.md`: the ASCEND-LINUX section gets an "S-ASCEND-2 update (<date>)" paragraph in the S5-update style (facts: numbering, isolation N-way, loader gates, 5/5 isolation items, freeze, device inodes, Mímir topology, scorer data, budgets at close); the "Build gotcha" line (loader now refuses a surface mismatch); the boot recipe (`UPC_DEMO_MODE=1`, SHA sidecar, unit). No LoC, no taglines.
- [ ] **Step 305** [DOC-UPDATE][W] ~10m: ADR statuses touched this sprint re-checked in `docs/adr/ADR-INDEX.md` (023, 043, 080 amended, 081 note, 108 limitations); `docs/protocol/RFC-EDITOR-REQUESTS-S5.md` gets rows 6-7 (provisional topics `upc.cart.weights.*`, `drift.restore_proposal.*`) for S-TRUE-WIRE's RFC Editor desk.
- [ ] **Step 306** [DOC-UPDATE][W] ~10m: `references/timeline.md` one dated entry (Age 3 list) + mirrors regenerated the way S22 did it (`git show --stat f83c9cde` shows the four files; the timeguru syncer: `go run ./services/timeguru/cmd/timeguru -sync-once 2>&1 | tail -1` if that flag exists, else edit json/yaml/toml in step with the md as f83c9cde did); `scripts/check-timeline-freshness.sh` → FRESH.
- [ ] **Step 307** [AUDIT-UPDATE][W] ~10m: `docs/sweeps/OUTSTANDING-2026-10-27.md`: every E14 and E13 M-row this sprint touched gets a status + step pointer (M-220..M-231, M-202, M-204); decisions 44-49 marked answered or open with the E-id.
- [ ] **Step 308** [B] ~3m: Link check over every doc touched this sprint
  ```bash
  cd "$PROJECT_ROOT" && for f in $(git diff --name-only "$(git merge-base develop HEAD)" -- '*.md'); do grep -oE '\]\(([^)#]+)' "$f" | sed 's/](//' | grep -vE '^https?:|^mailto:' | while read -r p; do d="$(dirname "$f")"; test -e "$d/$p" || test -e "$p" || echo "BROKEN $f -> $p"; done; done | sort -u | head -20
  ```
- [ ] **Step 309** [V] ~1m: No `BROKEN` lines (wiki `[[Page]]` links are not checked here; `scripts/check-wiki-links.sh` if present: run it).
- [ ] **Step 310** [TEST][B][S] ~10m: **FINAL REGRESSION** — the harness, the sweep, the budgets, one last time on the final tree
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S6/p13-regression-final.log" | grep -aE 'PASS|FAIL|verifier'; sweep | grep -c SAME; sudo bpftool prog show 2>/dev/null | grep -ciE 'monad|cart|heimdall'; ip netns list | wc -l
  ```
- [ ] **Step 311** [V] ~2m: Zero FAIL (incl. the new gate_nway/forkloop/devprobe gates); 12/12 SAME at the final HEAD; final ascend/Doom/cart budgets recorded in `$LOG` §"Close" next to Phase 0's; `0` programs attached; netns count as expected. West left as Step 22 found it plus whatever E6 installed.
- [ ] **Step 312** [W] ~20m: `~/tmp/next.md` handoff (overwrite the START HERE block per memory feedback_next_md_handoff): sprint state, every E1-E6 answer or open question with its fallback result, the KF table at close, budgets at close, what the Stage 2 session needs, what S-DOOM-LAST may now assume (nothing: Doom untouched except the HMAC env and doc banners), live state on west (units installed? netns?), re-sign + push reminder.
- [ ] **Step 313** [C] ~1m: **FINAL COMMIT** (`docs(upc): [S-ASCEND-2 303-313] wiki/CLAUDE.md/timeline/master-list ripple; handoff`), then `git log --oneline "$(git merge-base develop HEAD)"..HEAD | wc -l` recorded in `$LOG` (commit count, not LoC).
- [ ] **Step 314** [V] ~2m: **SPRINT EXIT GATE** — every phase's exit gate passed or STUCK-logged with a STUCK REPORT at the end of this file; tree clean; harness green; goldens 12/12; budgets ≤ 950,000; six escalations each answered or recorded open with their fallback executed; `next.md` written. Not pushed (Stevie pushes); not signed if the key was not cached (`scripts/resign-unpushed.py` later).
  - If pass → sprint closed; S-DOOM-LAST may begin only on Stevie's word. If fail → STUCK REPORT, not a banner.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. Ascend object over 950,000 verified insns after an eBPF step**
1. Do not continue. `git stash` or `git checkout -- ebpf/` to the last committed object source.
2. `cd $EBPF && cargo build --release -p monad-cpu-ebpf --features ascend-linux` and re-run the harness; confirm the Phase-0 count returns.
3. Re-read the budget lesson (master plan 1.3.1): a second variable offset into RAM_MAP inside a loop defeats state merging; carry one scalar. Try the change with one fewer live scalar; if still over, record the measured cost in `$LOG` and move the feature to the Stage 2 skeleton (kernel side) instead.
4. STUCK the step, commit the log, continue.

**A2. Harness red after a kernel/user source change ("goldens N/12 SAME")**
1. `diff $GOLD/<name>.full <latest capture>`: if only pid values differ and E1 was answered → policy recapture (Step 50). If a version byte / debug print leaked onto the TTY → remove it from the golden path (log it only under `UPC_VERIFIER_STATS=1`).
2. Any other diff: `kbuild` from a clean tree (`make -f ../adapters/Makefile.mbc clean kernel` + `rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland ramdisk`) and re-sweep; stale objects are the usual cause after `bpf-verifier-check.sh` (memory: it clobbers the ascend build).
3. Still red → bisect the phase's commits with `git stash`/`git checkout`; the sweep is the oracle.

**A3. `MRET fall-through` / `BOOT FAIL` on boot**
1. You booted the ascend kernel with the Doom object (or vice versa). After Phase 4 this is refused at load; before it: `cargo build --release -p monad-cpu-ebpf --features ascend-linux`.
2. If the loader refused with `SurfaceObjectMismatch` and you meant the ascend build: the object is stale → rebuild, re-run.

**A4. Leftover netns / veth / XDP after a crashed run**
1. `sudo bpftool prog show | grep -iE 'monad|cart|heimdall'` → `sudo bpftool prog detach`/`ip link set <if> xdpgeneric off` per link; `sudo bash scripts/upc-netns.sh down2` then `down` (if the oneshot owns `upc0`, `systemctl restart unheaded-upc-netns`).
2. `sudo "$DR" ring teardown --hops 2` if doom-runner was involved (the harness's Doom load).
3. `pgrep -f 'upc-bootctl|heimdall-daemon|cmd/anomaly|/wotan' ` and kill by pid (never `pkill -f` with a pattern that matches your own shell: memory reference_west_tool_gotchas).

**A5. EPERM running bootctl unprivileged (Phase 5)**
1. Capture the failing syscall: run the same `systemd-run` with `strace -f -e trace=bpf,setns,unshare,mount,perf_event_open` prefixed (as root) and read the first `EPERM`.
2. Add exactly the capability that syscall needs (CAP_PERFMON for verifier logs; CAP_SYS_RESOURCE for memlock on older kernels; CAP_SYS_ADMIN only if a netns op is still inside bootctl → move it to the oneshot instead).
3. Record the minimal set in the unit and `$LOG`.

**A6. Seccomp kills bootctl (Phase 5 Step 143)**
1. `journalctl -t audit --since -5min | grep -oE 'syscall=[0-9]+' | sort | uniq -c`; map numbers with `ausyscall --dump | grep -w <n>` (or `scmp_sys_resolver`).
2. Add the named syscall to `SystemCallFilter=`; never widen to `@privileged` or remove `~execve`.

**A7. East unreachable or east step misbehaves (Phase 8 east column)**
1. Stop; `ssh east 'sudo ip link set enp2s0 xdpgeneric off; pgrep -f heimdall && sudo pkill heimdall-daemon; rm -rf /tmp/s6'`.
2. Record in `$LOG`; fall back to the two-netns topology for the remaining steps; the evidence wording changes accordingly. Never retry a host action that failed once without reading why.

**A8. Scratch Wotan in a netns is unreachable**
1. Loopback is per-netns: the scratch Wotan must bind the host end of the bridge (`fd00:upc::1`), not `127.0.0.1`; check `ss -ltnp | grep 2660`.
2. `curl -g "http://[fd00:upc::1]:26601/health"` from the host, then `sudo ip netns exec upc1 curl -g ...` from inside.

**A9. GPG not cached mid-sprint**
1. `commitp` already falls back to `--no-gpg-sign`; keep going. After Stevie caches the key: `python3 scripts/resign-unpushed.py` (memory reference_resign_unpushed; probe with `git commit-tree -S`, not `gpg --clearsign`).

**A10. Stuck anywhere ≥ 3× the time tag or 2 failed [D] attempts**
1. Write the `[STUCK]` marker into this file with symptom, attempts, time; commit.
2. Scan Appendix B for the next step that does not depend on it; continue.
3. At the end, the STUCK REPORT goes at the bottom of this file, ordered by downstream impact.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX, CRITICAL PATH, DEPENDENCY MAP

| Phase | Agent | Parallel with | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Coordinator | — | S-CLEAN-SLATE gates | 1.5 h |
| 1 getpid probe + E1 | Coordinator | 2, 4 | 0 | 3 h (+E1) |
| 2 Stage 2 re-plan + E2 | Agent (docs) | 1, 3, 4 | 1 Step 37 | 2 h (+E2) |
| 3 gate_nway + forkloop | Coordinator | 2, 4 | 0 | 4 h |
| 4 Loader I/O gates | Agent (worktree) | 1, 2, 3 | 0 | 3 h |
| 5 Required-before-prod + E6 | Coordinator | 9 (host-side half) | 0; 4 for Step 141 | 6 h (+E6) |
| 6 Freeze / secret / hijack | Coordinator | 9 | 4 | 4 h |
| 7 FS substrate + ilp32e | Coordinator | 9, 10 | 3; 1 (flag state) | 4 h |
| 8 Mímir / DISTRIBUTED / LICH-015 + E3 | Coordinator | 9 | 5 (netns script) | 6 h (+E3 east) |
| 9 Cartridge live legs + scorer | Agent (243-256) then Coordinator | 6, 7, 8 | 0 | 4 h |
| 10 LICH-012 / Enkrateia | Coordinator | 9, 11 | 8 (netns), 5 | 3 h |
| 11 Name / statuses / E4 E5 | Agent (281-288), Coordinator | 9, 10 | 5 (Step 289) | 2 h (+E4, E5) |
| 12 Security review | Coordinator | — | 1-11 | 1.5 h |
| 13 Docs / handoff | Agent (Librarian) then Coordinator | — | 12 | 1.5 h |

**Critical path** (sequential, Coordinator): 0 → 3 → 7 → 8 → 10 → 12 → 13 ≈ 21.5 h of the 30-38 h. Phases 1, 4, 5, 6, 9, 11 hang off it in parallel slots (one eBPF writer at a time: 1, 3, 6, 7 serialize on `ebpf/monad-cpu-ebpf`; 4, 5, 9, 11 are host-side).

**Escalation dependency map**

| Escalation | Steps it gates | Fallback executed meanwhile |
|---|---|---|
| E1 pid numbering | 44-52 | Step 35 table; flag OFF; Phase 7 runs with the committed flag |
| E2 D1 reach | 63-66 | Skeleton drafted "pending E2"; Stage 2 session waits |
| E3 east | 233-238 | Two-netns topology, named honestly (206-232) |
| E4 name | 285-286 | Statuses that do not need the name (283, 287) |
| E5 attended run | 291 | Recipe written and dry-run (289) |
| E6 west install | 149-154 | Throwaway proofs (137-147); units committed |

**Verifier-budget serialization rule**: before any step that rebuilds `monad-cpu-ebpf`, `git status --short ebpf/` must be empty except for your own change; record `verified N insns` immediately after; never leave two agents editing `main.rs`.

---

## APPENDIX C: QUICK REFERENCE

**Paths**
- Kernel/userland: `crates/xv6-mbc/upstream/` (`$UP`), targets `$K = $UP/target` (`xv6-mbc.mbc`, `fs.img`, `init.mbc`, `gate2.mbc`, `gate_nway.mbc`, `getpidprobe.mbc`, new: `getpidfork.mbc`, `forkloop.mbc`, `devprobe.mbc`); Makefiles `crates/xv6-mbc/adapters/Makefile.mbc` (kernel) and `Makefile.mbc-userland` (`ramdisk`, `target/<prog>.mbc`).
- eBPF: `ebpf/monad-cpu-ebpf/src/main.rs` (`GETPID_INJECT` :413, flow-label instance :644-650, `STAT_*` consts ~:380-420), objects under `ebpf/target/bpfel-unknown-none/release/`; `ebpf/cartridge-ebpf`; `ebpf/monad-common` (`fs_walk`, `mbc_maps`).
- Loader: `crates/upc-api/src/{workload.rs,image.rs}`, `cmd/upc-bootctl/src/{main.rs,netns.rs,bootparams.rs}`, `crates/doom-runner/`.
- Host emulator/translator: `crates/monad-mbc` (bins `rv32i-to-mbc`, `mbc-asm`, `mbc-emulate`).
- Cartridges: `cmd/upc-cartridgectl` (`--sophia URL`, `SOPHIA_TOKEN`), `cmd/anomaly` (`--wotan`), `pkg/cartridge` (root-gated tests), `crates/upc-cartridge` (`cart`), `cartridges/scorer.*`.
- Mímir: `cmd/heimdall-daemon` (`--manifest --pubkey --sign-key --sign-ttl --wotan-grpc --drift-sign-key --bpf-dir --xdp-iface --xdp-mode`), `cmd/gjallarhorn-sender|-listener` (`ff02::1:abba`), `crates/heimdall-bpf`, `pkg/{gungnir,gjallarhorn,enkrateia}`.
- Lich: `tomb/lich/LICH-012-config-convergence/`, `tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness/`, `tomb/lich/UPC-CARTRIDGE-HOOKS.md`, seeds `tomb/lich/seeds/`.
- Isolation (new): `scripts/upc-netns.sh`, `deploy/systemd/unheaded-upc-netns.service`, `deploy/systemd/unheaded-upc-bootctl@.service`, `deploy/sysusers.d/unheaded.conf` (`unheaded-upc`), `pkg/uids/uids.go`.
- Logs: `references/s-ascend-2-log.md` (`$LOG`), scratch `$HOME/tmp/s-ascend-2` (`$S6`), goldens `$HOME/tmp/golden` (`$GOLD`).

**Ports** (`pkg/ports/ports.go`): Wotan 18000/18001, Sophia 19005, Anomaly 19018, UPC TTY bridge 26100, cartridge metrics 26610; scratch Wotan in this plan 26601/26602 (lab only, never in `ports.go`).

**Env**: `UPC_DEMO_MODE=1` (live boot gate, Phase 5), `UPC_VERIFIER_STATS=1` (prints `verified N insns`), `UPC_RV2MBC_SHA` (pre-Phase-5 opt-in; after: sidecar/flag required), `UPC_HMAC_KEY` (Phase 6, replaces the shipped literal), `UPC_LIVE=1` (root-gated live tests), `LICH_E2E=1`, `CART_DIFF_COUNT`, `GETPID_INJECT` is a **const**, not env.

**Budgets at a7fe3acd**: ascend 926,542 (flag ON 911,319), Doom 761,580, cart_xdp 42,743 / cart_tick 146,897 (gate 500,000). Working limit this plan: 950,000.

**Verdict tokens**: `ISOLATION-PASS` (gate2), `NWAY-ok pidN` / `NWAY-FAIL` (gate_nway), `GP: pid=N` (getpidprobe), `GF: parent-pid=N|child-pid=N|parent-after-wait` (getpidfork), `FL: child|parent-done` (forkloop), `DEV: console-open-ok` (devprobe), `0xP1D1-0UR5` (uinit), `0xGA7ED-D15C-READER` (cat README), `INDIRECT-READ-OK`, `SUBDIR-WALK-OK`.

**S5 escalation wording (verbatim, for E3)**: Step 177 — "(a) running heimdall-daemon as root on east from `/tmp` (no package, no unit) with XDP **generic** mode on `enp2s0`, observe-only XDP_PASS; (b) a scratch Wotan on west 26601/26602 bound to `192.168.13.2`." Step 435 — "running a scratch Wotan reachable from east over the P2P link, and running `upc-cartridgectl` + `cartridge-ebpf` as root on east from `/tmp` … If not approved: run both 'nodes' as two netns on west sharing one scratch Wotan; mark the result 'cross-node demonstrated in netns, not across hosts.'"

**Commit helper**: `commitp -m "<type>(<scope>): [S-ASCEND-2 a-b] <what>"` (sourced from the S5 plan; signs when the key is cached, else `--no-gpg-sign`; trailer lines per the forge brief).

---

## APPENDIX D: COVERAGE (master list → steps)

| M-id | Item | Steps | Notes |
|---|---|---|---|
| M-220 | getpid flip, child probe, pid numbering, proc[] reconciliation | 27-52 | E1; flip behind [AFTER-E1] |
| M-221 | D1 re-read vs negative delta; Stage 2 plan | 37, 53-66 | E2; skeleton, no numbered steps |
| M-222 | gate_nway green; forkbomb gate | 67-94 | KF1 retired; both harness-gated |
| M-223 | ADR-080 I/O-time loader gates; Epic 1.2.5 close | 95-116 | both loaders; negative live gates |
| M-224 | Required-before-prod 0/5 → 5/5 | 117-156 | E6 for the west install |
| M-225 | Freeze ROM/RV2MBC; shipped HMAC default; flow-label hijack | 157-180 | Go helper retired per [DECIDE] 166 |
| M-226 | FS substrate: device inodes; writes/pipes route | 181-190, 197-198 | decision 48 → Stage 2 |
| M-227 | riscv.h uint64 CSR helpers on ilp32e | 191-196 | Safe 1 + static assert |
| M-228 | East as a node; Mímir 9/11/13; DISTRIBUTED; full-engine LICH-015; MBC fuzz 0x47-0x4A; ADR-043 claim | 205-242 | E3; netns fallback named honestly |
| M-229 | Live legs (`anomaly.scores`, Sophia registry); scorer on real flows; gates #1(a)/#2 | 243-266 | decision 47 as [DECIDE] 243/244/255 |
| M-230 | unheadnix ceremony; ADR-023 superseded; wiki L6; upc-bootstub | 281-288 | E4 for the name |
| M-231 | Browser xterm e2e; Lich Phase 5 `LICH_E2E=1` | 289-291 | E5 attended |
| M-202 | LICH-012 L12a/L12d; Enkrateia v2; semantic drift | 267-280 | CTF wiring stays with ADR-062 (out of scope) |
| M-204 | Multicast bootstrap `ff02::1:abba` end to end | 206-215 (netns), 233-234 (east) | E3 for the physical topology |
| — | Doom (M-240..M-246) | none | S-DOOM-LAST only; this plan touches `cmd/doom` solely to remove a shipped secret default (Step 169) and adds doc banners (Step 287) |

Deferred out of this plan with a reason: `pid` MMU slice geometry as a `Workload` field is done (Phase 4) but a shared header between kernel adapters and the loader is not (parity test instead); FS writes/`>`/pipes → EVOLUTION-1 Stage 2 (decision 48); DOT4/ISA → its own spec-gate sprint (decision 244); CTF activation → ADR-062's rule.

---

*S-ASCEND-2 Battle Plan — Forged 2026-10-27*
*14 Phases. 314 Steps. The owned kernel answers its first syscall under one pid book, three processes prove they cannot see each other at once, the loader checks what it promised, and the UPC boots without root.*
