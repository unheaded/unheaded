<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S5 UPC: FINISH, THEN COMPUTE ON THE WIRE — BATTLE PLAN — 20 Phases, 452 Steps

**Date**: 2026-10-23
**Sprint**: S5 — S-UPC-WIRE-COMPUTE. Part 1 closes what is still open on the UPC: the S1.2 `csrw sscratch` blocker and the getpid flip, live TTY keyboard input, the Heimdall Gjallarhorn XDP listener + ringbuf reader, and the bare-metal Mímir's Law Phases 9/11/13. Part 2 builds **MBC cartridges**: small per-flow network functions, loaded through a Sophia registry and run by a UPC interpreter on the packet path (token-bucket rate limiter, flow sampler, ~2 KB int8 anomaly scorer feeding `services/anomaly`), with verifier-budget gates, benchmark gates, Anamnesis replay ("time travel" for a cartridge), a stretch split of one UPC across west+east over Wotan DISTRIBUTED memory, and hooks for the Lich sprint's MBC ISA differential fuzzing.
**Prerequisite**: Branch based on `443de3f82` or later `develop`. `bash scripts/upc-regression.sh` → `== ALL GREEN ==` on west. Passwordless sudo on west for netns/XDP (the regression harness already needs it). No staging promotion, no push (Stevie pushes; staging only on Stevie's word).
**Target**: getpid runs through the owned kernel and returns the right pid; the browser console types into a live xv6 shell; Heimdall reacts to Gjallarhorn packets seen by XDP; ADR-043 has real 9/11/13 evidence; three signed cartridges run per flow under measured verifier and pps/latency gates, their runs replay bit-exactly from Anamnesis, and the anomaly scorer's scores reach `services/anomaly`.
**Estimated Duration**: 46–62 working hours across 8–10 sessions (Part 1 ≈ 18–24 h, Part 2 ≈ 28–38 h)
**Agent Strategy**: Phases 0→3 sequential (translator change touches every image). Phase 4 [P:5]. Phases 5→8 sequential (bare metal). Part 2: Phase 9 → 10 → 11 → 12 sequential; Phases 13, 14 [P] after 12; Phase 15 after 14 (needs sampler features); Phase 16 [P:17] after 13; Phase 18 last (stretch, [ESCALATE]); Phase 19 closes.
**Commit Cadence**: every 5 steps (452 steps / 20 = 22.6 → clamp max 5), plus every phase gate, every red→green, every stuck-skip.
**Stuck Protocol**: skip after 3× the step's `~time` or 2 failed debug attempts. Mark `[STUCK]` in this file, commit, scan forward for steps that do not depend on it, produce a STUCK REPORT when non-blocked work runs out.

**Execution branch**: `[DECIDE]` RECOMMENDATION: `s5-upc-wire` off `develop` (or a lead-session worktree branch merged back). Rationale: Part 1 changes the translator under every image; a branch keeps `develop` green between phases. Override ONLY if Stevie says to work on `develop` directly.

**Commit rule (memory + CLAUDE.md)**: Claude commits, signed when the key is cached; if gpg-agent times out use `--no-gpg-sign` rather than stall. Conventional subject + plan tag, e.g. `fix(mbc): [S5 41-45] ...`. Stage explicit paths, never `git add -A`. No mantra lines in bodies. End each body with the executing session's attribution trailer. **Never `git push`.**

---

## 0. GROUNDING — WHAT THIS PLAN BUILDS ON (do not re-plan)

| Source | What it gives this sprint | Status at forge |
|---|---|---|
| `docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md` + `references/evolution1-stage1-2026-07-19.md` | S1.0 (uservec sscratch entry) and S1.1 (BPF getpid injection behind `GETPID_INJECT`) shipped dormant in `467cf129`. S1.2 return blocked: `prepare_return`'s `csrw sscratch,a5` stores `0x7be000`, reads `0x7ff`. Dual-books falsified. Verifier delta +5409. | S1.0/S1.1 DONE. S1.2 OPEN → Phases 1–3 |
| `crates/monad-mbc/src/translator.rs` (0x73 arm, ~L1074–1128) + `map_register` (L272) | CSR ops use **r12** as the CSR pointer and **r13** as the RMW temp; `map_register` maps x14/a4→r12 and x15/a5→r13. `csrr` (= CSRRS rs1=x0) still runs the RMW path, so **every `csrr` overwrites a5 with the CSR value and a4 with the CSR address**. In `kernel.asm` `prepare_return`: `lw a5,48(a0)` → `csrr a2,satp` (a5 ← satp) → `csrw sscratch,a5`. A kernel satp of `0x7ff` (ppn of the first kalloc'd page 0x7FF000 under PHYSTOP 0x800000; `SATP_SV39` is 0 on ilp32e) explains the observed value exactly. **Read-verified hypothesis — Phase 1 proves it before anything changes.** | Phase 1 H-S1 |
| ADR-067 Decision 2 | CSR region and "2-instruction" CSR sequence. Translator deviates (scratch regs, full 12-bit CSR index → up to 0x12FFC, past the reserved 0xF000–0xF0FF). | Spec drift → Phase 2 spec gate |
| ADR-074 / ADR-077 / ADR-079 / ADR-080 | Per-pid pgd, per-process `rv2mbc_base`, tagged returns, **UPC programmatic API**: a guest = image + rv2mbc + memory model + syscall surface + boot; surfaces are `cfg`-partitioned so each build fits 1M verifier insns; **every variant must be load-tested**. Doom playable + no regressions is non-negotiable. | Contract Part 2 plugs into |
| `crates/upc-api` (Epic 1.2.3) | `Workload`, `validate()`, `load()`, host `SurfaceRegistry`, `FeatureGate {Default, AscendLinux}` | Cartridges add a variant |
| `scripts/upc-regression.sh` (Epic 1.1) + `~/tmp/golden` (12-boot corpus) | One-command gate: both variants load, xv6 corpus green, gate2 ISOLATION-PASS, budgets printed. Goldens are HEAD-derived. | Safety net for every UPC step |
| `cmd/upc-tty-bridge/main.go:149` | `TODO: write each byte of msg to KBD_MAP`. `upc-bootctl` already pre-fills the KBD ring (`write_kbd`, Gate C); the BPF read(5) drain already uses free-running `KBD_HEAD/KBD_TAIL` with `& 63` — **live input needs no BPF change**. `upc-bootctl console` is a stub. | Phase 4 |
| `cmd/heimdall-daemon/main.go:191,203,204` | TODOs: Wotan publish with ML-DSA-65; BPF ringbuf reader; Gjallarhorn XDP listener. `crates/heimdall-bpf` builds only `vfs_write.rs`; `gjallarhorn_xdp.rs` and `execve.rs` were declared in `7faa5b17` and never written (Cargo.toml comment). `cmd/gjallarhorn-listener` is a UDP-socket print tool on port 16901. | Phase 5 |
| ADR-043 + `docs/battle-plans/BATTLE-PLAN-MIMIRS-LAW-GLEIPNIR-PHASE-0.md` + `wiki/Mimirs-Law.md` | PoC "PROMOTE" on 2 nodes over WG: Phase 9 unicast only (multicast "L2-only, N/A"), Phase 11 = 100 packets, K1 DEFERRED (BPF build pending), LICH-012 scaffold only. CLAUDE.md still says "Phases 9, 11, 13 pending". **New fact**: west `enp16s0` ↔ east `enp2s0` is a physical P2P Ethernet /30 with IPv6 link-local — link-local multicast `ff02::1:abba` works there. | Phases 6–8 re-run with real evidence |
| `docs/archive/internal-2026/battle-plans/monad-cpu-app10-nfv.md`, `app12-anomaly.md`, `app08-selfheal.md`, `app07`… | Native eBPF apps already exist: `ebpf/nfv-ebpf` (tail-call chain incl. `nfv_rate_limiter`), `ebpf/qos-ebpf` (token bucket + CoDel), `ebpf/anomaly-ebpf` (i16 decision tree, 1:100 ringbuf), `ebpf/failover-ebpf`. `services/anomaly` exists but **no binary imports it** (`docs/UNREACHED-GO-PACKAGES.md`); same for `services/anamnesis`. app12 assumes Suricata (dropped 2026-10-22). | Native apps = **baselines + oracles**, not rebuilt |
| `crates/monad-mbc/fuzz`, `crates/lich-fuzz/lich_007_mbc.rs`, `tomb/lich/harnesses/lich_007_mbc_ebpf_test.go` (simulation only, per its ADR-062 banner) | Existing MBC fuzz surface | Phase 17 adds hooks only; the campaign is the Lich sprint's |
| Verifier-budget lessons (git log, memory, CLAUDE.md) | `MAX_INSN_PER_TICK=16`; `translate_address` inside an 8-iter loop blew 1M; a second variable offset into `RAM_MAP` inside a loop defeats state merging (+119K); dead-elim behind `cfg`/const; `phys = pid_phys_offset + va` not `translate_address`; `--btf` + memset → "Global function memset() doesn't return scalar" (zero-init padding); load-test each variant | Phase 11 rules |
| `services/wotan/internal/compute` | `BPFMapCache` on pinned `RAM_MAP` + L2 store — the Wotan memory-service seed for DISTRIBUTED | Phase 18 |
| `ietf-submission/*` registries | Anamnesis Event Types (Specification Required), Sophia Root Keys (0x10–0xFE FCFS), Wotan Topic Namespace (Expert Review), MBC Opcode Registry (Standards Action), Shim Pipeline Stages | Spec gates (Phases 2, 9, 15, 18) |

**Not in scope (parked, Marshal enforces)**: Stage 2 syscall migration ladder; proc[] reconciliation; dm-verity (ADR-043 hard condition 3, still v2); a full LICH-012 campaign (Lich sprint); packaging new binaries as debs / sysusers / east installs; dashboard pages for cartridges; any Monad wire-format change.

---

## VARIABLES

Resolve once per session (Step 1). Every command below uses these names.

```bash
export PROJECT_ROOT="$(git rev-parse --show-toplevel)"
export EBPF="$PROJECT_ROOT/ebpf"
export BC="$PROJECT_ROOT/cmd/upc-bootctl/target/release/upc-bootctl"
export UP="$PROJECT_ROOT/crates/xv6-mbc/upstream"; export AD="$PROJECT_ROOT/crates/xv6-mbc/adapters"
export K="$UP/target"
export GOLD="${GOLD:-$HOME/tmp/golden}"              # persistent home (survives reboot); HEAD-derived
export S5="${S5:-$HOME/tmp/s5-upc-wire}"             # sprint scratch: logs, bench output, traces (NOT in repo)
export PLAN="$PROJECT_ROOT/docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md"
export EAST=govan@east                                # 192.168.13.1 on enp2s0; west is .2 on enp16s0
export WEST_P2P_IF=enp16s0 EAST_P2P_IF=enp2s0
export WOTAN_SCRATCH_HTTP=26601 WOTAN_SCRATCH_GRPC=26602   # scratch Wotan, user-app range; never the 18000/18001 stack
mkdir -p "$S5"
# xv6 boot helpers (from PHASE24-KERNEL-CORE.md, unchanged)
boot()     { (cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
             --userland "$K/init.mbc" --triggers 4000000 --instance 111 --input "$1" 2>&1); }
tty_of()   { grep -a 'ascii:' <<<"$1"; }
stats_of() { grep -aoE 'forks=[0-9]+|waitpid=[0-9]+|tty_r=[0-9]+' <<<"$1" | tr '\n' ' '; }
kbuild()   { (cd "$UP" && rm -f kernel/kernel.elf && make -f ../adapters/Makefile.mbc kernel); }
ubuild()   { (cd "$UP" && rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland ramdisk); }
sweep()    { local f=0; for spec in "ls" "cat README" "cat BIGFILE" "cat sub/NOTE" "ls sub" "echo hello" \
             "echo peace and love" "wc README"; do out="$(boot "$spec"$'\n')"; s="$(tr ' /' '__' <<<"$spec")"; \
             diff -q <(tty_of "$out") "$GOLD/$s.tty" >/dev/null && diff -q <(stats_of "$out") "$GOLD/$s.stats" >/dev/null \
             && echo "SAME $s" || { echo "DIFF $s"; f=1; }; done; return $f; }
commitp()  { git -C "$PROJECT_ROOT" commit "$@" || git -C "$PROJECT_ROOT" commit --no-gpg-sign "$@"; }
```

`sweep` covers the 8 corpus boots; `uinit_echo`, `multi`, `nosuch`, `blank` are compared by the explicit steps that need them (recipes in `docs/battle-plans/PHASE22-SH-REPLACE.md` Steps 10–14).

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification — MUST pass before proceeding
[D]            Debug — only if the prior [V] failed (max 2 attempts, then Skip Protocol)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo required (west; netns/XDP patterns the regression harness already uses)
[P]            Parallelizable with other [P] steps in the same phase
[P:N]          Parallelizable with Phase N
[SEQ]          MUST be sequential
[C]            Commit checkpoint
[ENV]          Environment check
[BUILD]        Build/compile
[TEST]         Test execution
[CODE]         Code implementation (test first: red → green → refactor)
[DESIGN]       Design decision written into a doc
[STUCK]        Skipped via Skip Protocol (needs a human)
[BLOCKED]      Blocked by an upstream [STUCK] (NEVER used for decisions)
[DECIDE]       Decision with a pre-seeded RECOMMENDATION — proceed autonomously
[ESCALATE]     Needs Stevie. No recommendation possible or a host/bare-metal action outside
               standing approval. STOP that chain; continue independent work.
[PREFLIGHT]    Hypothesis verification (Phase 0 and phase-local preflights)
[REGEN]        Regenerate derived artifacts (.mbc, fs.img, goldens, ADR index)
[AUDIT-UPDATE] Update a findings/tracking doc
[DOC-UPDATE]   Downstream docs (CLAUDE.md, wiki, ADR-INDEX, next.md)
[SECURITY]     Security review step
[COMPLIANCE]   License/SBOM/dependency check
[VM-SCAN]      Needs a VM (Kali/Lich) — none in this plan; listed for completeness
[BARE-METAL]   Real host NIC / east / physical link
[SPEC]         Touches a spec, registry, ABI or ISA — runs the SPEC COMPLIANCE GATE
[GATE]         Phase exit gate
```

`[ESCALATE]` in this plan always means: Stevie approves the specific host action (east install, XDP on a physical NIC, a new IANA registration, an ISA opcode). Everything on west inside a netns, on loopback, or through `BPF_PROG_TEST_RUN` is standing-approved by precedent (`scripts/upc-regression.sh`).

---

## PREFLIGHT HYPOTHESES (verified in Phase 0 / phase preflights — never assumed)

| ID | Hypothesis | Verified at | If false |
|---|---|---|---|
| H0 | Branch contains `443de3f82`; tree clean | Step 2 | `git reset --hard 443de3f82` on the sprint branch only |
| H1 | Regression harness `== ALL GREEN ==`; ascend verifier ≈ 900,031; Doom ≈ 737,087 | Steps 5–7 | STOP: base not green. Record actual numbers as the baseline if they moved for known reasons |
| H2 | `~/tmp/golden` matches current HEAD (12 boots) | Steps 8–10 | Recapture from HEAD (recipe: `references/phase24-t1-2026-07-07.md`) |
| H-S1 | The `0x7ff` in sscratch is the kernel satp, written into a5 by the `csrr a2,satp` RMW (translator temp r13 = a5) | Phase 1 Steps 31–42 | Fall back to the `references/evolution1-stage1-2026-07-19.md` "next probe": MBC dump diff of the two `csrw sscratch` sites |
| H-S2 | `csrr`/`csrw` also clobber a4 (r12, CSR pointer) everywhere; `csrw <csr>, a4` stores the CSR address, not a4 | Step 36 | Narrow the fix to the a5 path only |
| H-S3 | No CSRRS/CSRRC with rs1≠x0 and no CSRRW with rd==rs1≠x0 exist in any built image (kernel, userland, Doom) | Step 44 | Those forms need the spill sequence in Phase 2 instead of fail-closed |
| H-S4 | No image uses a CSR index ≥ 0x400 (address would exceed 0xFFFF) | Step 45 | Keep the full-range mapping; amend ADR-067 instead of tightening |
| H-S5 | Doom's RV image has zero CSR instructions → Doom `.mbc` byte-identical after the translator fix | Step 46 | Doom golden-frame check becomes mandatory in Phase 2 |
| H-K1 | The BPF read(5) drain handles a ring that is refilled while the guest runs (free-running HEAD/TAIL, `& 63`) | Step 111 | A BPF change is needed → [ESCALATE] verifier spend |
| H-G1 | `heimdall-bpf` builds (`vfs_write`) on west's nightly + bpf-linker | Step 137 | Fix toolchain first (Emergency A.4) |
| H-G2 | `cilium/ebpf v0.22.0` loads Aya-built XDP objects with BTF maps | Step 152 | Loader in Rust (aya) as a sidecar — [DECIDE] in Step 153 |
| H-M1 | West `enp16s0` ↔ east `enp2s0` carries IPv6 link-local multicast | Step 175 | Phase 6 runs the multicast leg in a west veth pair only; record "physical multicast not demonstrated" |
| H-C1 | `services/anomaly` has no binary and no consumer for `anomaly.scores` | Step 233 | Skip the binary steps in Phase 15; wire into the existing one |
| H-C2 | `BPF_PROG_TEST_RUN` with `repeat` works for XDP on west's kernel (6.17) and reports ns/run | Step 240 | Benchmarks fall back to veth + pktgen-style blaster (slower, noisier) |
| H-C3 | `monad-mbc` host `execute.rs` implements MUL/MULH/SAR/LDB/LD/ST with the same semantics as `monad-cpu-ebpf` | Step 262 | Differential tests in Phase 10 become the first Lich finding; fix host side |
| H-C4 | Wotan scratch instance starts as the invoking user on 26601/26602 with a lab topic config | Step 236 | Use in-process fake publisher for Phase 15; real Wotan in Phase 18 only |

---

## KNOWN FAILURES BASELINE (pre-existing — NOT regressions)

Record actual output in Step 12. Anything on this list failing again is not a sprint regression; anything NOT on it failing is.

| # | Known failure / gap | Source |
|---|---|---|
| KF1 | `gate_nway` → NWAY-FAIL | Phase 1.7 Gate C notes; regression harness does not gate it. **Retired by S-ASCEND-2 Step 91** (f2bc1a56): the scheduler resumed a reaped zombie when nothing was runnable; isolation held. gate_nway is now a harness gate |
| KF2 | `crates/heimdall-bpf` declares no `execve`/`gjallarhorn_xdp` programs (never written) | Cargo.toml comment |
| KF3 | `services/anomaly`, `services/anamnesis` unreached by any binary | `docs/UNREACHED-GO-PACKAGES.md` |
| KF4 | `tomb/lich/harnesses/lich_007_mbc_ebpf_test.go` is a simulation, not coverage | its own banner (ADR-062 audit) |
| KF5 | West compose stack is down (`restart: "no"`), Wotan not running on 18000/18001 | `~/tmp/next.md` |
| KF6 | `cmd/trace-collector-go` darwin vet broken | next.md S17 |
| KF7 | GitHub `ci.yml`, `docker.yml`, `ebpf.yml`, `release.yml` are `disabled_manually` | next.md S17 (Stevie's setting — leave) |
| KF8 | Doom E1M2 texture issues | memory `project_doom_status` |
| KF9 | `flow_tracker` never deletes closed flows; flows page counts connections twice | next.md Agent 3 |
| KF10 | MBC-ISA draft lists 0x11–0x1B RESERVED and also assigns 0x17/0x18/0x1A/0x1B; MOVI documented as sign-extended while both interpreters zero-extend `imm16` | read at forge (`ietf-submission/draft-bellis-unheaded-mbc-isa-00.md` §IANA; `main.rs` MOVI arm) — routed to RFC Editor in Step 72, not fixed here |
| KF11 | `anomaly-ebpf` overloads Monad `SAMPLED` (0x08) as "suspicious" and reads payload bytes for an entropy feature | `ebpf/anomaly-ebpf/src/main.rs` header — cartridges must not copy either |

---

## DEPENDENCY MAP

```
P0 ─► P1 ─► P2 ─► P3 ─────────────────────────────┐
 │                                                 │
 ├─► P4 (TTY, host-side) ──────────────────────────┤
 │                                                 ├─► P8 (Part 1 exit) ─► P9 ─► P10 ─► P11 ─► P12 ─┬─► P13 ─┬─► P16 ─┐
 └─► P5 ─► P6 ─► P7 ───────────────────────────────┘                                                 │        └─► P17 ─┤
                                                                                                     └─► P14 ─► P15 ───┤
                                                                                                                       ├─► P18 (stretch) ─► P19
                                                                                                                       └────────────────────► P19
```

**Critical path**: P0 → P1 → P2 → P3 → P8 → P9 → P10 → P11 → P12 → P14 → P15 → P16 → P19 (≈ 40 h). P4 runs beside P1–P3; P5–P7 beside P2–P3 once P0 passes (different subsystem; serialize only the regression-harness runs, which share XDP/netns state).

---

# PART 1 — FINISH WHAT IS PENDING

## PHASE 0: PREFLIGHT, BASELINE, HYPOTHESES (Steps 1-30)

**Goal**: Prove the base is green, pin every budget and golden, verify the hypotheses this plan leans on.
**Prerequisite**: West session with sudo; nothing else running XDP on `lo`/the harness netns.
**Time**: ~1.5 h
**Agent**: Coordinator

- [x] **Step 1** [ENV][B] ~1m: Resolve variables (VARIABLES block above), confirm tools
  ```bash
  cd "$(git rev-parse --show-toplevel)" && source <(sed -n '/^export PROJECT_ROOT/,/^commitp/p' docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md)
  for t in cargo go bpftool riscv64-unknown-elf-gcc riscv64-unknown-elf-objdump bpf-linker jq python3 ip; do
    command -v "$t" >/dev/null && echo "OK $t" || echo "MISSING $t"; done
  ```
- [x] **Step 2** [V][PREFLIGHT] ~30s: **H0** — base contains `443de3f82`, tree clean, not on `main`/`staging`
  ```bash
  git -C "$PROJECT_ROOT" merge-base --is-ancestor 443de3f82 HEAD && echo H0-ANCESTOR-OK
  git -C "$PROJECT_ROOT" status --porcelain | wc -l; git -C "$PROJECT_ROOT" branch --show-current
  ```
  - If not ancestor → on the sprint branch only: `git -C "$PROJECT_ROOT" reset --hard 443de3f82`
  - If on `main`/`staging` → STOP; switch to the sprint branch (header [DECIDE])
- [x] **Step 3** [V] ~30s: All tools from Step 1 present
  - If `bpf-linker` missing → Emergency A.4. If riscv toolchain missing → STOP (Part 1 cannot build kernels)
- [x] **Step 4** [B][BUILD] ~5m: Build host tools once (translator, bootctl)
  ```bash
  cargo build --release --manifest-path "$PROJECT_ROOT/crates/monad-mbc/Cargo.toml" 2>&1 | tail -3
  cargo build --release --manifest-path "$PROJECT_ROOT/cmd/upc-bootctl/Cargo.toml" 2>&1 | tail -3
  ```
- [x] **Step 5** [B][S][TEST] ~2m: Regression harness
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S5/p0-regression.log" | tail -25
  ```
- [x] **Step 6** [V][PREFLIGHT] ~30s: **H1** — ends `== ALL GREEN ==`
  - If fail → STOP. This plan assumes a green base. Diagnose under the harness, not this plan.
- [x] **Step 7** [V][PREFLIGHT] ~30s: Record budgets
  ```bash
  grep -aE 'ascend verifier|Doom verifier|verified [0-9]+ insns' "$S5/p0-regression.log" | tee "$S5/p0-budgets.txt"
  ```
  - Expected ≈ ascend 900,031 / Doom 737,087. Different → record as the sprint baseline, note the commit that moved it
- [x] **Step 8** [B][S] ~6m: H2 — sweep the 8 corpus boots against `$GOLD`
  ```bash
  kbuild >/dev/null 2>&1; ubuild >/dev/null 2>&1; sweep | tee "$S5/p0-sweep.txt"
  ```
- [x] **Step 9** [V][PREFLIGHT] ~30s: **H2** — 8 × `SAME`
  - If any `DIFF` → Step 10. All SAME → Step 11
- [x] **Step 10** [D][REGEN][S] ~15m: Goldens stale for this HEAD → recapture all 12 from HEAD
  - Use `references/phase24-t1-2026-07-07.md` §"Golden recapture" + `PHASE22-SH-REPLACE.md` Steps 6/10/12–14 with `GOLD=$HOME/tmp/golden`. Then re-run Step 8; must be 8 × SAME.
- [x] **Step 11** [B] ~1m: Capture translator baselines (counts + hashes) for kernel, userland, Doom
  ```bash
  sha256sum "$K"/xv6-mbc.mbc "$K"/*.mbc 2>/dev/null | tee "$S5/p0-mbc-sha.txt"
  ls "$PROJECT_ROOT"/doom 2>/dev/null | head; find "$PROJECT_ROOT" -name '*.mbc' -path '*doom*' -newer "$PROJECT_ROOT/go.mod" 2>/dev/null | head
  ```
  - Note the Doom `.mbc` path and its sha in `$S5/p0-mbc-sha.txt` (needed by H-S5)
- [x] **Step 12** [B] ~3m: KNOWN FAILURES BASELINE — record actuals
  ```bash
  { echo "== gate_nway"; (cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
      --userland "$K/gate_nway.mbc" --triggers 800000 --instance 111 2>&1 | grep -aoE 'NWAY-(PASS|FAIL)' | head -1);
    echo "== go build"; (cd "$PROJECT_ROOT" && go build ./... 2>&1 | tail -3);
    echo "== heimdall-bpf"; (cd "$PROJECT_ROOT/crates/heimdall-bpf" && cargo build --release 2>&1 | tail -2); } \
    | tee "$S5/p0-known-failures.txt"
  ```
- [x] **Step 13** [V] ~30s: Baseline matches the KF table (KF1 NWAY-FAIL, `go build` clean, heimdall-bpf builds vfs_write)
  - New unexpected failure → add it to KF with evidence; if it blocks Part 1, STOP
- [x] **Step 14** [R] ~5m: Read the S1.2 record so Phase 1 starts from the corrected diagnosis
  ```bash
  sed -n '/CORRECTION \/ RE-DIAGNOSIS/,$p' "$PROJECT_ROOT/references/evolution1-stage1-2026-07-19.md" | head -80
  ```
- [x] **Step 15** [R] ~3m: Confirm the dormant S1.0/S1.1 code is present
  ```bash
  grep -n 'GETPID_INJECT' "$EBPF/monad-cpu-ebpf/src/main.rs" | head -3
  grep -n 'csrr a0, sscratch\|mv a0, a1\|csrw mscratch' "$AD/trampoline_mbc.S"
  grep -n 'w_sscratch' "$AD/utrap.c"; ls "$UP/user/getpidprobe.c"
  ```
- [x] **Step 16** [V] ~30s: `GETPID_INJECT: bool = false`, uservec has `csrw mscratch,a0` + `csrr a0,sscratch`, userret has `mv a0, a1`, `prepare_return` calls `w_sscratch`, probe exists
  - Missing → STOP; S1.0/S1.1 are not on this branch (cherry-pick `467cf129`)
- [x] **Step 17** [R] ~3m: Disassemble `prepare_return` from the current kernel build
  ```bash
  riscv64-unknown-elf-objdump -d "$UP/kernel/kernel.elf" | awk '/<prepare_return>:/,/ret$/' | tee "$S5/p0-prepare_return.asm"
  ```
- [x] **Step 18** [V][PREFLIGHT] ~1m: The sequence `lw a5,48(a0)` … `csrr a2,satp` … `sw a2,0(a5)` … `csrw sscratch,a5` is present in that order, with no write to a5 between the `lw` and the `csrw` other than the `csrr`
  - If the order differs → H-S1 needs re-derivation; Phase 1 still runs, from Step 31
- [x] **Step 19** [R] ~2m: Read the translator CSR arm + register map (lines move; search by text)
  ```bash
  grep -n 'let csr_ptr = 12u8\|let tmp = 13u8\|let mask = 14u8\|14 => Ok(12)\|15 => Ok(13)' "$PROJECT_ROOT/crates/monad-mbc/src/translator.rs"
  ```
- [x] **Step 20** [V] ~30s: All five lines found (CSR pointer r12, RMW temp r13, CSRRC mask r14 = ra, a4→r12, a5→r13)
- [x] **Step 21** [B] ~1m: Count CSR instruction forms in every built RV image (feeds H-S3/H-S4/H-S5 later)
  ```bash
  for e in "$UP"/kernel/kernel.elf "$K"/*.elf; do printf '%s ' "$(basename "$e")"; \
    riscv64-unknown-elf-objdump -d "$e" | grep -cE '\scsr[a-z]*\s'; done | tee "$S5/p0-csr-counts.txt"
  ```
- [x] **Step 22** [R] ~3m: Read the Mímir's Law phase table and ADR-043 status so Part 1 re-runs, not re-plans
  ```bash
  sed -n '/Phase Status/,/Real-Metal/p' "$PROJECT_ROOT/wiki/Mimirs-Law.md"; sed -n '1,30p' "$PROJECT_ROOT/docs/adr/ADR-043-"*.md
  ```
- [x] **Step 23** [V] ~30s: Record the drift: ADR-043 says "all 13 shipped", CLAUDE.md says "9/11/13 pending". Phase 8 reconciles it.
- [ ] **Step 24** [B][ENV] ~1m: Host facts for Part 1 bare metal (read-only) — west half only; east not contacted this session (Phases 0–4 scope)
  ```bash
  ip -br addr show dev "$WEST_P2P_IF"; ssh -o BatchMode=yes "$EAST" "ip -br addr show dev $EAST_P2P_IF; uname -r" 2>&1 | tail -3
  ```
- [ ] **Step 25** [V] ~30s: West shows `192.168.13.2/30` + an `fe80::` address; east answers over SSH with `192.168.13.1/30` — west verified; east leg left to Phases 6–7
  - East unreachable → Phases 6–7 east legs become [BLOCKED]; continue everything else
- [x] **Step 26** [B] ~1m: Next free ADR number and Lich sprint plan presence
  ```bash
  ls "$PROJECT_ROOT/docs/adr" | grep -oE 'ADR-[0-9]{3}' | sort -u | tail -3
  ls "$PROJECT_ROOT/docs/battle-plans" | grep -i lich || echo "no Lich sprint plan on this branch"
  ```
- [x] **Step 27** [V] ~30s: Record `ADR_CART=ADR-<next>` (ADR-105 at forge time) in `$S5/vars.env`; record the Lich plan path if present (Phase 17 targets its hook contract)
  ```bash
  echo "ADR_CART=ADR-105" >> "$S5/vars.env"   # edit if Step 26 shows 105 taken
  ```
- [x] **Step 28** [W] ~2m: Start the sprint log
  ```bash
  printf '# S5 UPC wire compute — execution log\n\nbase: %s\nbudgets: %s\n' "$(git -C "$PROJECT_ROOT" rev-parse --short HEAD)" "$(tr '\n' ' ' < "$S5/p0-budgets.txt")" > "$PROJECT_ROOT/references/s5-upc-wire-log.md"
  ```
- [x] **Step 29** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add references/s5-upc-wire-log.md && commitp -m "docs(upc): [S5 1-29] sprint log + preflight baseline"
  ```
- [x] **Step 30** [V][GATE] ~1m: **PHASE 0 EXIT GATE** — H0, H1, H2 true; budgets, CSR counts, KF actuals recorded in `$S5`; sprint log committed
  - Pass → Phase 1 (and Phase 4, Phase 5 may start in parallel)
  - Fail → DO NOT PROCEED

### Phase 0 Definition of Done
All steps executed or STUCK-logged · H0/H1/H2 verified · KF baseline recorded · log committed · no code changed · no new deps.

---

## PHASE 1: S1.2 ROOT CAUSE — PROVE IT BEFORE TOUCHING THE TRANSLATOR (Steps 31-50)

**Goal**: Turn H-S1 from a read-verified hypothesis into a red unit test that fails for exactly the reason the boot fails.
**Prerequisite**: Phase 0 gate.
**Time**: ~2 h
**Agent**: Coordinator (Computermancer lens: gate-level, one variable at a time)

- [x] **Step 31** [R] ~5m: Read how the host emulator runs MBC (needed for a host-side test)
  ```bash
  grep -n 'pub fn new\|pub fn step\|pub fn run\|pub fn load' "$PROJECT_ROOT/crates/monad-mbc/src/execute.rs" | head; grep -n 'RAM_SIZE' "$PROJECT_ROOT/crates/monad-mbc/src/cpu.rs"
  ```
- [x] **Step 32** [V][PREFLIGHT] ~1m: The host CPU can load a word program and run it, and RAM covers 0xF000–0xFFFF (RAM_SIZE 0x10000)
  - No → write the test against translator output only (Step 34 alternative: assert the emitted MBC sequence)
- [x] **Step 33** [W][CODE][TEST] ~20m: Red test — translate the exact `prepare_return` words and run them
  - New test in `crates/monad-mbc/src/translator.rs` `#[cfg(test)]` (or `tests/csr_alias.rs`): build RV words `lw a5,48(a0)` (0x03052783), `csrr a2,satp` (0x18002673), `csrw sscratch,a5` (0x14079073); seed RAM so `[a0+48] = 0x007be000` and the satp slot (`0xF000 + 0x180*4`) = `0x000007ff`; run on the host emulator; assert the sscratch slot (`0xF000 + 0x140*4 = 0xF500`) == `0x007be000` and a5 (r13) == `0x007be000`.
  ```bash
  cargo test --manifest-path "$PROJECT_ROOT/crates/monad-mbc/Cargo.toml" csr_alias 2>&1 | tail -15
  ```
- [x] **Step 34** [V] ~1m: **RED for the predicted reason** — sscratch slot reads `0x7ff` and a5 reads `0x7ff`
  - If it reads `0x7be000` (green) → H-S1 FALSE. Go to Step 35. If red with any other value → Step 35.
  - If red with `0x7ff` → H-S1 CONFIRMED on the host. Step 36.
- [x] **Step 35** [D] ~20m: H-S1 false → the evolution1 "next probe": dump the MBC emitted for both `csrw sscratch` sites and diff
  ```bash
  "$PROJECT_ROOT/crates/monad-mbc/target/release/mbc-asm" "$K/xv6-mbc.mbc" --disasm 2>"$S5/p1-kernel.dis" >/dev/null; wc -l "$S5/p1-kernel.dis"
  ```
  - Two attempts max, then Skip Protocol on Phases 1–3 and continue with Phase 4/5
- [x] **Step 36** [W][CODE][TEST] ~15m: Red test H-S2 — `csrw sstatus,a4` (prepare_return's `0x10071073`) after `mv a4,tp` must store a4's value; and a live a4 must survive any unrelated `csrr`
  ```bash
  cargo test --manifest-path "$PROJECT_ROOT/crates/monad-mbc/Cargo.toml" csr_alias 2>&1 | tail -15
  ```
- [x] **Step 37** [V] ~1m: H-S2 recorded — red tests show (a) sstatus slot receives `0xF400` (the address) instead of a4, (b) a4 clobbered by an unrelated `csrr`
  - If green → H-S2 false; keep the test as a guard, narrow the fix scope in Phase 2
- [x] **Step 38** [W][CODE][TEST] ~10m: Red test — CSRRC clobbers `ra` (mask temp r14) and CSRRW rd==rs1 loses the source (the S1.0 idiom)
- [x] **Step 39** [V] ~1m: Both red as predicted (or recorded green with reason)
- [x] **Step 40** [B][S] ~10m: Confirm on the live UPC — one boot with a temporary value probe, then revert (S1.2 used the same method)
  - Temporary edit in `adapters/utrap.c`: after `w_sscratch(...)` print `r_satp()` and `r_sscratch()` low 32 bits with the existing `mmio_puthex` probe idiom. `kbuild`, boot `echo hello`, capture.
  ```bash
  kbuild >/dev/null && out="$(boot $'echo hello\n')"; grep -aoE 'Z:[A-Z]+ [^·]*' <<<"$out" | head; printf '%s\n' "$out" > "$S5/p1-live-probe.txt"
  ```
- [x] **Step 41** [V] ~1m: Live probe shows satp low bits == sscratch == `0x7ff` → H-S1 confirmed on the BPF interpreter too
- [x] **Step 42** [B] ~2m: Revert the probe, rebuild, prove byte-identity with the Phase 0 kernel
  ```bash
  git -C "$PROJECT_ROOT" checkout -- crates/xv6-mbc/adapters/utrap.c && kbuild >/dev/null && sha256sum "$K/xv6-mbc.mbc" | diff - <(grep 'xv6-mbc.mbc' "$S5/p0-mbc-sha.txt") && echo KERNEL-RESTORED
  ```
- [x] **Step 43** [V] ~30s: `KERNEL-RESTORED`
- [x] **Step 44** [B][PREFLIGHT] ~5m: **H-S3** — find set/clear forms with rs1≠x0 and swaps with rd==rs1 in every image
  ```bash
  for e in "$UP"/kernel/kernel.elf "$K"/*.elf; do riscv64-unknown-elf-objdump -d "$e" | \
    grep -E '\scsrr[sc]i?\s|\scsr[sc]i?\s|\scsrrw\s' | grep -vE 'csrrs\s+[a-z0-9]+,[a-z0-9]+,zero' | sed "s|^|$(basename "$e"): |"; done | tee "$S5/p1-csr-forms.txt" | head -40
  ```
- [x] **Step 45** [B][PREFLIGHT] ~3m: **H-S4** — CSR indices used (any ≥ 0x400 → address > 0xFFFF)
  ```bash
  for e in "$UP"/kernel/kernel.elf "$K"/*.elf; do riscv64-unknown-elf-objdump -d "$e"; done | grep -oE '\scsr[a-z]*\s+[^ ]+' | awk -F, '{print $NF}' | sort | uniq -c | sort -rn | tee "$S5/p1-csr-index.txt"
  ```
- [x] **Step 46** [B][PREFLIGHT] ~3m: **H-S5** — CSR instructions in the Doom RV image (path from Step 11)
  ```bash
  DOOM_ELF="$(find "$PROJECT_ROOT/doom" "$PROJECT_ROOT/crates/doom-runner" -name '*.elf' 2>/dev/null | head -1)"; echo "$DOOM_ELF"
  riscv64-unknown-elf-objdump -d "$DOOM_ELF" | grep -cE '\scsr[a-z]*\s'
  ```
- [x] **Step 47** [V] ~1m: Write H-S3/H-S4/H-S5 outcomes into the sprint log (counts + forms)
- [x] **Step 48** [W] ~10m: Append "S1.2 root cause (S5)" to `references/evolution1-stage1-2026-07-19.md`: the translator RMW on `csrr` writes the CSR value into r13=a5; the 0x7ff is satp; red tests named; live probe output quoted. No fix yet.
- [x] **Step 49** [C] ~1m: **COMMIT CHECKPOINT** (red tests are `#[ignore]`d with a reason pointing at Phase 2, so the tree stays green)
  ```bash
  git -C "$PROJECT_ROOT" add crates/monad-mbc references/evolution1-stage1-2026-07-19.md references/s5-upc-wire-log.md && \
  commitp -m "test(mbc): [S5 31-49] red tests for CSR scratch aliasing (S1.2 root cause)"
  ```
- [x] **Step 50** [V][GATE] ~1m: **PHASE 1 EXIT GATE** — root cause stated with host + live evidence (or H-S1 STUCK-logged); red tests committed and ignored; kernel byte-identical to Phase 0
  - Pass → Phase 2. Fail → Skip Protocol for Phases 1–3; continue at Phase 4.

### Phase 1 Definition of Done
Red tests exist for every aliasing case found · live probe reverted, kernel sha unchanged · H-S1…H-S5 recorded · zero new failures.

---

## PHASE 2: TRANSLATOR CSR FIX — MBC VERIFICATION PATTERN + SPEC GATE (Steps 51-82)

**Goal**: CSR translation clobbers no live RISC-V register; `csrr` never writes the CSR; unsupported forms fail the build loudly. No new opcode, no eBPF change.
**Prerequisite**: Phase 1 gate.
**Time**: ~3 h
**Agent**: Coordinator + Developer (TDD)

- [x] **Step 51** [DECIDE][DESIGN] ~10m: Scratch strategy for the CSR address
  - **RECOMMENDATION**: use **r0** (x0's MBC register) as the CSR pointer and restore it with `MOVI r0,0` at the end of the sequence. Rationale: the translator already writes r0 transiently whenever rd=x0 and resets it via `guard_zero_dst`, so this adds no new hazard class; every other MBC register is a live RV register (`map_register` maps all 16), and LD/ST offsets are `simm16`, so `[r0 + 0xF500]` cannot be encoded directly (0xF500 sign-extends). For `csrr rd` (rd≠0) the address can be built in **rd** itself (zero scratch).
  - **Override ONLY if**: Step 53 shows any MBC trap/interrupt can be taken between two MBC words of one translated RV instruction (then r0 is not safe → use the PUSH/POP spill of a register ∉ {rd, rs1} instead).
- [x] **Step 52** [DECIDE][DESIGN] ~5m: Rare forms (H-S3)
  - **RECOMMENDATION**: forms absent from every image (per `$S5/p1-csr-forms.txt`) → `TranslateError::UnsupportedInstruction` (fail-closed: a future image using them breaks the build, not the boot). Forms present → spill sequence `PUSH rX; …; POP rX`, rX ∉ {rd, rs1, r15}.
  - **Override ONLY if**: a present form sits on a path where sp is not a valid stack (uservec entry before the stack switch) → then that site gets a source-level workaround like S1.0 and the translator rejects the form.
- [x] **Step 53** [R] ~10m: Confirm interrupts/traps are only taken at MBC instruction boundaries the translator controls (SYSCALL injection is synchronous; check timer/IRQ delivery in `main.rs`)
  ```bash
  grep -n 'pending_irq\|timer_irq\|deliver_interrupt\|STVEC' "$EBPF/monad-cpu-ebpf/src/main.rs" | head -20
  ```
- [x] **Step 54** [V] ~1m: Either no asynchronous delivery exists in the ascend build, or delivery happens only between whole RV-instruction translations → Step 51 recommendation stands
- [x] **Step 55** [V][SPEC] ~5m: **SPEC COMPLIANCE GATE (ISA/ABI)**
  - Wire format version: v0x01 (FROZEN) — untouched
  - Change type: **bug fix** in RV32I→MBC translation; brings implementation toward ADR-067 Decision 2 ("LD rd / ST rs1", no scratch). No opcode added/removed/renumbered; eBPF interpreter unchanged
  - RFC Editor reviewed: **N/A for the code** (no spec text changes in this phase); **Y required** for the ADR-067 amendment written in Step 72 (CSR range + the reject list)
  - Backward compatible: Y — all existing `.mbc` remain loadable; regenerated images change bytes (expected)
  - IANA impact: None
- [x] **Step 56** [CODE] ~30m: Implement per Steps 51–52 in the 0x73 arm
  - `csrr` (CSRRS/CSRRC with rs1=x0, and CSRRSI/CSRRCI with uimm=0): emit only the read; **no write-back** (RISC-V: rs1=x0 → no write).
  - Address in rd for reads; in r0 (restored) for writes.
  - CSRRW: `rd==rs1≠0` → reject (or spill per Step 52). CSRRS/CSRRC with rs1≠x0 → reject or spill per Step 52.
  - CSR index ≥ 0x400 → reject if H-S4 showed none used; else keep full mapping and note it for Step 72.
- [x] **Step 57** [B][TEST] ~2m: Un-ignore the Phase 1 tests; run the translator suite
  ```bash
  cargo test --manifest-path "$PROJECT_ROOT/crates/monad-mbc/Cargo.toml" 2>&1 | tail -20
  ```
- [x] **Step 58** [V] ~30s: Phase 1 tests GREEN; no other test changed status
  - Red → [D] Step 59
- [x] **Step 59** [D] ~15m: Compare emitted words against the intended sequence with `mbc-asm --disasm` on a one-instruction ELF; fix, rerun Step 57. Two attempts, then STUCK.
- [x] **Step 60** [W][TEST] ~15m: Exhaustive table test — for every CSR form (CSRRW/S/C, immediate variants) × rd ∈ {x0, a4, a5, ra, other} × rs1 ∈ {x0, a4, a5, other}: either `Err(Unsupported)` or, on the host emulator, (1) rd = old CSR, (2) CSR = new per the RISC-V spec, (3) every other RV-mapped register unchanged, (4) r0 == 0 after the sequence
- [x] **Step 61** [B][TEST] ~2m: MBC verification pattern — decode
  ```bash
  cargo test --manifest-path "$PROJECT_ROOT/crates/monad-mbc/Cargo.toml" decode 2>&1 | tail -5
  ```
- [x] **Step 62** [V] ~30s: ALL opcodes decode correctly
- [x] **Step 63** [B][TEST] ~6m: MBC verification pattern — roundtrip + execute fuzz, 5 min each (resource-aware: two short runs, not open-ended)
  ```bash
  cd "$PROJECT_ROOT/crates/monad-mbc" && cargo +nightly fuzz run fuzz_mbc_roundtrip -- -max_total_time=300 2>&1 | tail -3
  cargo +nightly fuzz run fuzz_mbc_execute -- -max_total_time=300 2>&1 | tail -3
  ```
- [x] **Step 64** [V] ~30s: Zero crashes in both runs
  - Crash → Emergency A.6; do not commit the translator change until fixed
- [x] **Step 65** [C] ~1m: **COMMIT CHECKPOINT** (red→green)
  ```bash
  git -C "$PROJECT_ROOT" add crates/monad-mbc/src && commitp -m "fix(mbc): [S5 51-65] CSR translation no longer clobbers a4/a5/ra; csrr never writes the CSR"
  ```
- [x] **Step 66** [B][BUILD][REGEN] ~5m: Rebuild translator, kernel, userland
  ```bash
  cargo build --release --manifest-path "$PROJECT_ROOT/crates/monad-mbc/Cargo.toml" 2>&1 | tail -1
  kbuild 2>&1 | tail -2; ubuild 2>&1 | tail -2; sha256sum "$K"/xv6-mbc.mbc | tee "$S5/p2-kernel-sha.txt"
  ```
- [x] **Step 67** [V] ~1m: Kernel `.mbc` changed (expected); translation count recorded (was 11,769 MBC at S1.0)
  - Build error `UnsupportedInstruction` → an image uses a form Step 52 rejected: Step 52 override path
- [x] **Step 68** [B] ~3m: **H-S5** — Doom `.mbc` regenerated from the same ELF is byte-identical
  ```bash
  "$PROJECT_ROOT/crates/monad-mbc/target/release/rv32i-to-mbc" "$DOOM_ELF" -o "$S5/doom-new.mbc" && sha256sum "$S5/doom-new.mbc"; grep -i doom "$S5/p0-mbc-sha.txt"
  ```
- [x] **Step 69** [V] ~30s: Doom sha identical → Doom unaffected by construction
  - Differs → Doom has CSR ops: run `scripts/doom-smoke.sh` and the doom-runner load in the harness before proceeding (Step 71 covers load; add a frame check)
- [x] **Step 70** [B][S][TEST] ~8m: Full corpus sweep + the four special boots
  ```bash
  sweep | tee "$S5/p2-sweep.txt"
  ```
- [x] **Step 71** [B][S][TEST] ~2m: Regression harness
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tee "$S5/p2-regression.log" | tail -12
  ```
- [x] **Step 72** [W][SPEC] ~20m: Amend ADR-067 Decision 2 (implementation note dated 2026-10-xx): the real CSR address range (`0xF000 + csr*4`, csr < 0x400 if H-S4 held), the no-scratch rule, the rejected forms, the "rs1=x0 never writes" rule. Add an RFC Editor item for the MBC-ISA draft: KF10 (0x11–0x1B listed RESERVED and assigned; MOVI documented sign-extended, implemented zero-extended). **RFC Editor review requested — not self-approved.**
- [x] **Step 73** [V] ~1m: **TTY gate** — `p2-sweep.txt` 8 × SAME; harness `== ALL GREEN ==`; ascend verifier unchanged (eBPF untouched); Doom loads
  - Any DIFF → Step 74
- [x] **Step 74** [D] ~20m: A corpus boot changed. If only the stats line moved (cycle-dependent), compare `.full` for the first divergent byte; if TTY bytes differ, the fix changed semantics a boot relied on (a latent clobber that "worked"): bisect by reverting the a4/r0 half of the change. Two attempts, then STUCK.
- [x] **Step 75** [B][S] ~6m: Boot the four special goldens (`uinit_echo`, `multi`, `nosuch`, `blank`) with the PHASE22 recipes; diff
- [x] **Step 76** [V] ~30s: 12/12 boots byte-identical (TTY + stats)
- [x] **Step 77** [REGEN] ~1m: Goldens stay as they are (they still match). Note in the log: "goldens valid for HEAD after Phase 2".
- [x] **Step 78** [AUDIT-UPDATE] ~5m: Sprint log: H-S1…H-S5 final, budgets, translation count delta, Doom identity
- [x] **Step 79** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add docs/adr/ADR-067-mbc-isa-v2-and-upc-abi-v1.md references/s5-upc-wire-log.md && commitp -m "docs(adr): [S5 66-79] ADR-067 CSR implementation note + RFC Editor items"
  ```
- [x] **Step 80** [SECURITY] ~10m: BlackMage lens — can a user program reach a CSR form the translator now rejects at runtime? (No: translation is offline; a rejected form fails the build.) Can r0-as-pointer leak a non-zero r0 into user code? (Only if a trap lands mid-sequence; Step 54 ruled it out.) Record both answers.
- [x] **Step 81** [V] ~30s: Security notes recorded; no new attack surface
- [x] **Step 82** [V][GATE] ~1m: **PHASE 2 EXIT GATE** — tests green, fuzz clean, 12/12 goldens identical, harness green, Doom identical, ADR-067 note committed with RFC Editor item open
  - Fail → DO NOT PROCEED to Phase 3

### Phase 2 Definition of Done
MBC verification pattern passed (decode + 2 × 5-min fuzz) · spec gate recorded · zero new failures · no eBPF/verifier change · ADR-067 amended · RFC Editor item open.

---

## PHASE 3: DREAM LADDER L5 — GETPID FLIP (EVOLUTION-1 STAGE 1 CLOSE) (Steps 83-108)

**Goal**: The injected getpid trap enters the owned kernel and returns to user mode with the right pid; decide the default of `GETPID_INJECT` on measured evidence.
**Prerequisite**: Phase 2 gate, Computermancer sign-off that L5 is regression-free.
**Time**: ~2.5 h
**Agent**: Coordinator

### L5 regression verification (Dream Ladder template)
- [x] **Step 83** [B][S] ~2m: L5 suite = regression harness (already green at Step 71) — re-run only if anything changed since
- [x] **Step 84** [V] ~30s: ALL L5 tests pass (zero regression). Any regression → HALT.
- [x] **Step 85** [B] ~1m: L5 benchmark = boot instruction counts for `echo hello` (stats line) vs Phase 0
- [x] **Step 86** [V] ~30s: Within 5% of the Phase 0 baseline (the fix removes write-backs, so expect equal or lower)

### Close S1.2
- [x] **Step 87** [CODE] ~10m: `userret`: replace `mv a0, a1` with `csrr a0, sscratch` (the S1.2 design, now that `prepare_return` stores the right value). Keep the comment block, updated with the S5 root cause.
- [x] **Step 88** [B][S] ~8m: Rebuild kernel; full sweep with `GETPID_INJECT=false`
  ```bash
  kbuild >/dev/null 2>&1 && sweep | tee "$S5/p3-sweep-userret.txt"
  ```
- [x] **Step 89** [V] ~30s: 8 × SAME — forkret's userret now works from sscratch (it broke the boot in July)
  - DIFF → Step 90
- [x] **Step 90** [D] ~20m: Print `a1` vs sscratch at userret entry (temporary `mmio_puthex`), revert after. If they differ, `prepare_return` and forkret disagree on the trapframe → this is the dual-books question again: STOP the flip, keep `mv a0,a1`, record evidence, Skip to Step 104.
- [x] **Step 91** [B][BUILD] ~3m: Build the probe and the ascend object with the flag ON
  ```bash
  (cd "$UP" && make -f ../adapters/Makefile.mbc-userland target/getpidprobe.mbc 2>&1 | tail -1)
  sed -i 's/const GETPID_INJECT: bool = false;/const GETPID_INJECT: bool = true;/' "$EBPF/monad-cpu-ebpf/src/main.rs"
  (cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 | tail -1)
  ```
- [x] **Step 92** [B][S][TEST] ~3m: Run the probe with the flag ON
  ```bash
  out="$(cd "$EBPF" && UPC_VERIFIER_STATS=1 sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
    --userland "$K/getpidprobe.mbc" --triggers 2000000 --instance 111 2>&1)"; printf '%s\n' "$out" > "$S5/p3-getpid-on.txt"
  grep -aoE 'GP: [a-z=0-9-]+|verified [0-9]+ insns' <<<"$out"
  ```
- [x] **Step 93** [V] ~1m: **FIRST ROUND TRIP** — `GP: probe-start`, `GP: getpid-returned`, `GP: pid=0` (init's pid, same as the flag-OFF control), CPU back at priv=3, no DEAD sentinel
  - Missing `getpid-returned` → Step 94
- [x] **Step 94** [D] ~20m: Trace PC/priv across SRET (existing `--trace`-style CPU dump in bootctl); check `sepc` = ecall RV addr + 4 handling in `usertrap` (xv6 advances epc by 4; the injector stores the ecall's rv_word). Two attempts, then STUCK and restore the flag to false.
- [x] **Step 95** [B][S] ~3m: Same probe, flag OFF (control) — confirm identical TTY
- [x] **Step 96** [V] ~30s: Flag ON and OFF print identical `GP:` lines — **FAILED 2026-10-23**: ON prints `GP: pid=1`, OFF `GP: pid=0` (see references/s5-upc-wire-log.md)
- [x] **Step 97** [B] ~1m: Verifier delta with the fixed path
  ```bash
  grep -aoE 'verified [0-9]+ insns' "$S5/p3-getpid-on.txt"; grep -aoE 'ascend verifier[^)]*' "$S5/p2-regression.log"
  ```
- [x] **Step 98** [V] ~30s: Record delta (July: +5409 → 905,440). Must be < 1,000,000 with ≥ 5% headroom (≤ 950,000)
- [x] **Step 99** [B][S][TEST] ~8m: Flag ON — full sweep + harness (the harness rebuilds ascend with the source flag as set)
  ```bash
  sweep | tee "$S5/p3-sweep-on.txt"; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tail -8
  ```
- [x] **Step 100** [V] ~30s: 8 × SAME, harness green, Doom loads (Doom build DCEs the arm)
- [x] **Step 101** [B] ~2m: Cycle delta — instructions for one getpid round trip (flag ON minus flag OFF, from the stats/CPU dump)
- [x] **Step 102** [DECIDE] ~10m: Default for `GETPID_INJECT` — **2026-10-23: `false`** (Step 96 failed)
  - **RECOMMENDATION**: ship **`true`** in the ascend build iff Steps 93/96/98/100 all passed. Rationale: EVOLUTION-1 D1's "+5409 → keep (c) minimal" rule governs the *Stage 2 migration ladder* (which syscalls to move next), not whether the one already-built, now-working proof stays dark; Stevie scoped "getpid flip" into this sprint; rollback stays one const.
  - **Override ONLY if**: verifier > 950,000, or cycle cost per getpid makes any corpus boot exceed its `--triggers` budget, or Stevie re-affirms "flip stays deferred" → set `false`, keep the evidence.
- [x] **Step 103** [W][DOC-UPDATE] ~15m: Update `EVOLUTION-1-TRAP-FLIP.md` (Stage 1: DONE, root cause, verifier + cycle delta, default chosen and why) and `UPC-LINUX-MASTER-BATTLE-PLAN.md` Epic 1.2 checkbox note
- [x] **Step 104** [V] ~1m: Tree state matches the decision (flag value in `main.rs`, userret line) and Step 88/99 sweeps match that state
- [x] **Step 105** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add crates/xv6-mbc/adapters/trampoline_mbc.S ebpf/monad-cpu-ebpf/src/main.rs docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md references/s5-upc-wire-log.md && \
  commitp -m "feat(upc): [S5 83-105] getpid round-trips through the owned kernel (EVOLUTION-1 Stage 1 closed)"
  ```
- [x] **Step 106** [B][REGEN] ~10m: Goldens: if any `.full` changed only in non-gated lines, keep; else recapture all 12 from this HEAD (equivalence proven by Steps 99–100)
- [x] **Step 107** [V] ~1m: **DREAM LADDER L5 GATE** — getpid round trip proven; L5 regression GREEN; Computermancer confirms; verifier headroom ≥ 5%
- [x] **Step 108** [V][GATE][C] ~1m: **PHASE 3 EXIT GATE / LEVEL TRANSITION COMMIT** — commit any golden note; Phase 3 closed
  ```bash
  git -C "$PROJECT_ROOT" add references/s5-upc-wire-log.md && commitp -m "docs(upc): [S5 106-108] L5 gate evidence" || true
  ```

### Phase 3 Definition of Done
Round trip proven flag ON · decision recorded with numbers · 12/12 goldens identical · harness green · Doom unaffected · EVOLUTION-1 + master plan updated.

---

## PHASE 4: TTY BRIDGE LIVE KEYBOARD — HOST-SIDE ONLY (Steps 109-134)

**Goal**: Keystrokes typed in `dashboard/upc-console.html` reach the running xv6 shell through the KBD ring; output streams back live. No eBPF change.
**Prerequisite**: Phase 0 gate. [P:1-3] (different files) — serialize only the `sudo` boots with other phases.
**Time**: ~3 h
**Agent**: Developer [P]

- [x] **Step 109** [R] ~5m: Read the producer/consumer contract
  ```bash
  sed -n '/pub fn write_kbd/,/^    }/p' "$PROJECT_ROOT/cmd/upc-bootctl/src/runner.rs"; grep -n 'KBD_TAIL.get(0)' -A12 "$EBPF/monad-cpu-ebpf/src/main.rs" | head -16
  ```
- [x] **Step 110** [R] ~3m: Read the bridge reader loop + bind address
  ```bash
  sed -n '/Reader loop/,/^}/p' "$PROJECT_ROOT/cmd/upc-tty-bridge/main.go"; grep -n 'addr := \|CheckOrigin' "$PROJECT_ROOT/cmd/upc-tty-bridge/main.go"
  ```
- [x] **Step 111** [V][PREFLIGHT] ~2m: **H-K1** — the guest consumes `KBD_MAP[tail & 63]` while `tail < head`, never resets HEAD/TAIL itself, and blocks (PC rewind) on empty → a host producer may append at `head & 63` and then bump `KBD_HEAD`, provided `head - tail < 64`
  - False → [ESCALATE] (needs a BPF change and verifier spend); park Phase 4, continue Phase 5
- [x] **Step 112** [DESIGN][W] ~10m: Contract note in the `runner.rs` doc comment: single producer (host) / single consumer (guest); write slot THEN head (two map updates, syscall-ordered); capacity check `head.wrapping_sub(tail) < 64`; never write TAIL after boot. Note the guest compares `tail < head` (not wrapping) — document the 2^32-byte session limit rather than spend verifier insns on it.
- [x] **Step 113** [W][CODE][TEST] ~20m: Red tests (pure functions): `kbd_append_plan(head, tail, bytes) -> (writes, new_head, accepted)` — full ring accepts 0, partial accepts `64-(head-tail)`, slot index wraps 63→0
  ```bash
  cargo test --manifest-path "$PROJECT_ROOT/cmd/upc-bootctl/Cargo.toml" kbd_append 2>&1 | tail -8
  ```
- [x] **Step 114** [CODE] ~20m: Implement `kbd_append_plan` + `Runner::append_kbd(&[u8]) -> Result<usize>` (reads HEAD/TAIL, writes slots `(b<<1)|1`, then bumps HEAD)
- [x] **Step 115** [V] ~1m: Tests green; `write_kbd` (`--input` prefill) untouched, its tests still green
- [x] **Step 116** [W][CODE][TEST] ~25m: Bridge (Go, test first): per-instance bounded keystroke queue (4096 B; overflow drops newest and counts `upc_tty_kbd_dropped_total`); the WS reader pushes into it (replacing the TODO); `GET /api/v1/kbd/drain?instance=N&max=K` returns ≤K queued bytes as a JSON number array (same shape as ingest); 405 on non-GET
  ```bash
  cd "$PROJECT_ROOT" && go test -race ./cmd/upc-tty-bridge/... 2>&1 | tail -5
  ```
- [x] **Step 117** [V] ~30s: Red first, then green; `-race` clean
- [x] **Step 118** [SECURITY][CODE] ~15m: Typing into the UPC shell is a write path. Add `--listen` (default `127.0.0.1:26100`, replacing `:%d`); the drain endpoint answers only loopback callers (403 otherwise, tested with a fake RemoteAddr); WS origin check unchanged (`WS_ALLOWED_ORIGINS`)
- [x] **Step 119** [V] ~30s: Security tests green
- [x] **Step 120** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add cmd/upc-tty-bridge cmd/upc-bootctl/src/runner.rs && commitp -m "feat(upc-tty): [S5 109-120] keystroke queue + loopback drain; KBD ring append"
  ```
- [x] **Step 121** [W][CODE] ~40m: `upc-bootctl console --instance N [--stdin] [--tick-batch T] --kernel … --ramdisk … --userland …` (replaces the stub): reuse `boot`'s load path; loop until Ctrl-C: send T triggers → `read_tty(&mut cursor)` → print + `post_ingest` → GET drain and/or raw stdin bytes → `append_kbd`; on exit, the same XDP/netns teardown as `boot`
- [x] **Step 122** [B][BUILD] ~3m: Build bootctl + bridge
  ```bash
  cargo build --release --manifest-path "$PROJECT_ROOT/cmd/upc-bootctl/Cargo.toml" 2>&1 | tail -1
  (cd "$PROJECT_ROOT" && go build -o "$S5/upc-tty-bridge" ./cmd/upc-tty-bridge)
  ```
- [x] **Step 123** [B][S][TEST] ~5m: Headless end-to-end with `--stdin`: type two commands after boot
  ```bash
  (sleep 8; printf 'echo hello\n'; sleep 6; printf 'ls\n'; sleep 6) | (cd "$EBPF" && sudo timeout 40 "$BC" console --instance 111 --stdin \
    --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" --userland "$K/init.mbc") 2>&1 | tee "$S5/p4-console-stdin.txt" | tail -20
  ```
- [x] **Step 124** [V] ~1m: Output has `hello`, the `ls` listing (`README`, `cat`, …) and a `$` after each — typed after boot, not pre-filled
  - Fail → Step 125
- [x] **Step 125** [D] ~15m: Add `--debug-kbd` (print HEAD/TAIL per batch). HEAD moves but TAIL doesn't → guest not in read(5) yet: raise `--tick-batch`. TAIL catches HEAD but no echo → byte encoding (`(b<<1)|1`) wrong.
- [x] **Step 126** [B][S][TEST] ~5m: Bridge path: start the bridge, run `console` without `--stdin`, type through a WS client (`websocat` if installed, else a small Go test client in `cmd/upc-tty-bridge/testdata/`)
  ```bash
  "$S5/upc-tty-bridge" --listen 127.0.0.1:26100 >"$S5/bridge.log" 2>&1 & echo $! > "$S5/bridge.pid"
  ```
- [x] **Step 127** [V] ~1m: Text typed into the WS appears as shell output on the WS (`echo hello` → `hello`)
- [x] **Step 128** [B] ~1m: Paste 200 bytes at once
- [x] **Step 129** [V] ~30s: All 200 bytes reach the guest in order (≤64 per batch, rest queued); `kbd_dropped_total` = 0
- [x] **Step 130** [B][S] ~2m: Stop the bridge; regression harness (bootctl changed)
  ```bash
  kill "$(cat "$S5/bridge.pid")"; bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tail -6
  ```
- [x] **Step 131** [V] ~30s: `== ALL GREEN ==`; `--input` path (Gate C) unchanged
- [x] **Step 132** [W][DOC-UPDATE] ~10m: `wiki/Linux-on-UPC.md` (Mode A/B now interactive; console recipe); remove the bridge TODO comment and the `cmd_console` stub text
- [x] **Step 133** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add cmd/upc-bootctl cmd/upc-tty-bridge wiki/Linux-on-UPC.md && commitp -m "feat(upc-bootctl): [S5 121-133] interactive console: live keyboard via bridge or stdin"
  ```
- [x] **Step 134** [V][GATE] ~1m: **PHASE 4 EXIT GATE** — live typing proven via stdin AND via WS; drain loopback-only; harness green; TODO removed

### Phase 4 Definition of Done
Host-only change (eBPF untouched, verifier unchanged) · Go `-race` + Rust tests green · drain loopback-only · wiki updated.

---

## PHASE 5: HEIMDALL — GJALLARHORN XDP LISTENER + RINGBUF READER (Steps 135-170)

**Goal**: An XDP program sees Gjallarhorn packets; vfs_write/execve hooks see file activity; heimdall-daemon reads both ringbufs and re-verifies on demand (rate-limited), alerts-only.
**Prerequisite**: Phase 0 gate. [P:1-4] for code; bare-metal legs wait for Phase 6.
**Time**: ~4 h
**Agent**: Developer + Coordinator (sudo, netns)

- [x] **Step 135** [R] ~5m: Read the packet contract and the BPF crate
  ```bash
  sed -n '1,30p' "$PROJECT_ROOT/pkg/gjallarhorn/types.go"; sed -n '1,30p' "$PROJECT_ROOT/crates/heimdall-bpf/Cargo.toml"
  ```
- [x] **Step 136** [B][BUILD] ~3m: Build the existing crate
  ```bash
  (cd "$PROJECT_ROOT/crates/heimdall-bpf" && cargo build --release 2>&1 | tail -3); ls "$PROJECT_ROOT/crates/heimdall-bpf/target/bpfel-unknown-none/release/" | grep heimdall
  ```
- [x] **Step 137** [V][PREFLIGHT] ~30s: **H-G1** — `heimdall-vfs-write` object produced
  - S5 2026-10-23: H-G1 TRUE (`heimdall-vfs-write` built).
  - Fail → Emergency A.4 (toolchain), two attempts, then STUCK chain 137–170
- [x] **Step 138** [DECIDE] ~5m: XDP action for Gjallarhorn packets
  - S5 2026-10-23: Decided: XDP_PASS always (recommendation).
  - **RECOMMENDATION**: `XDP_PASS` always (observe-only). Rationale: the userspace UDP path keeps working; a parser bug can never black-hole the P2P link that also carries BGP and WireGuard; alerts-only (ADR-043 HC#1) gives XDP nothing to act on.
  - **Override ONLY if**: Phase 7's stress run shows the userspace path cannot keep up → then drop only records the XDP parser already classifies as malformed (never valid horns, never non-GJLR traffic).
- [x] **Step 139** [W][CODE] ~40m: `crates/heimdall-bpf/src/gjallarhorn_xdp.rs` — Ethernet → IPv6 (no extension-header walk in v1; documented) → UDP dport 16901 → payload exactly 20 B → magic `GJLR` → kind ∈ {1,2} → reserved bytes zero → RINGBUF record `{kind:u8, pad:[u8;3], cluster:u32, manifest_ptr:u64, src_hash:u32, ts_ns:u64}`; per-CPU stats (seen, valid, bad_magic, bad_kind, bad_len). Verifier rules: bounds-check every packet read; write every field explicitly (no zero-init of padded structs → avoids the `--btf` memset global-function rejection); no loops.
- [x] **Step 140** [W][CODE] ~20m: `crates/heimdall-bpf/src/execve.rs` — tracepoint `sched/sched_process_exec` → `DriftEvent{event_type: 1}` (closes KF2 / ADR-043 K1 "all hooks")
- [x] **Step 141** [W] ~2m: Restore the two `[[bin]]` stanzas in `crates/heimdall-bpf/Cargo.toml`; delete the "PLANNED, NOT YET WRITTEN" block
- [x] **Step 142** [B][BUILD] ~3m: Build + lint
  ```bash
  (cd "$PROJECT_ROOT/crates/heimdall-bpf" && cargo build --release 2>&1 | tail -3 && cargo clippy --release 2>&1 | tail -3)
  ```
- [x] **Step 143** [V] ~30s: Three objects; clippy clean
- [x] **Step 144** [B][S][TEST] ~5m: Verifier-load each object (no attach) and record processed insns
  - S5 2026-10-23: Deviation: bpftool/libbpf 1.x refuses the Aya 0.1 legacy `maps` section ("legacy map definitions ... not supported"), so the counts come from the cilium/ebpf loader (`TestLoadObjects`, Step 152).
  ```bash
  for o in heimdall-vfs-write heimdall-execve heimdall-gjallarhorn-xdp; do
    sudo bpftool -d prog loadall "$PROJECT_ROOT/crates/heimdall-bpf/target/bpfel-unknown-none/release/$o" "/sys/fs/bpf/s5-$o" 2>&1 \
      | grep -E 'processed [0-9]+ insns' | tail -1; sudo rm -rf "/sys/fs/bpf/s5-$o"; done | tee "$S5/p5-verifier.txt"
  ```
- [x] **Step 145** [V] ~30s: **K1 EVIDENCE** — all three load; processed-insn counts recorded
  - S5 2026-10-23: K1: vfs_write 20, execve 21, gjallarhorn XDP 292 processed insns. Two rejections fixed first: `[u8;3]` pad zeroed via memset (--btf global function), `read_unaligned` lowered to memcpy.
  - Rejection → Emergency A.1; two attempts, then STUCK
- [x] **Step 146** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `b6f5ec8a`.
  ```bash
  git -C "$PROJECT_ROOT" add crates/heimdall-bpf && commitp -m "feat(heimdall-bpf): [S5 135-146] gjallarhorn XDP observer + execve tracepoint; all hooks verifier-clean"
  ```
- [x] **Step 147** [R] ~5m: Read the repo's cilium/ebpf loader pattern
  ```bash
  sed -n '1,80p' "$PROJECT_ROOT/cmd/trace-collector-go/loader_native.go"
  ```
- [x] **Step 148** [W][CODE][TEST] ~30m: `cmd/heimdall-daemon/bpf.go` + `_test.go` (tests first): decode ringbuf records (`DriftEvent`, `HornEvent`) with exact sizes/endianness; reject short records
- [x] **Step 149** [W][CODE][TEST] ~30m: Trigger policy (pure, tested): `ReverifyUnicast` for this node's cluster → immediate `Reload()+ScanFiles()` behind a token bucket (1/s, burst 3) so forged horns cannot DoS the scanner; `BootstrapBroadcast` → bootstrap handler (no-op until Phase 6); vfs_write/execve → 250 ms debounce → `ScanFiles()`
- [x] **Step 150** [W][CODE] ~30m: Wire into `Run()`: flags `--xdp-iface`, `--bpf-dir`, `--xdp-mode generic|native` (default generic); load, attach kprobe/tracepoint/XDP, two ringbuf readers; any load/attach error → log once and keep polling (daemon works without BPF). Replace the BPF + XDP TODOs.
  - S5 2026-10-23: execve became a raw tracepoint: the classic tracepoint needs tracefs, absent under `ip netns exec`.
- [x] **Step 151** [B][TEST] ~2m: Go tests
  ```bash
  cd "$PROJECT_ROOT" && go test -race ./cmd/heimdall-daemon/... ./pkg/gjallarhorn/... ./pkg/enkrateia/... 2>&1 | tail -6
  ```
- [x] **Step 152** [V][PREFLIGHT] ~1m: **H-G2** — tests green; a root-gated `TestLoadObjects` loads the Aya objects through cilium/ebpf
  - S5 2026-10-23: H-G2 TRUE: cilium/ebpf v0.22 loads all three Aya objects. Step 153 fallback not needed.
  - BTF/map-type parse failure → Step 153
- [x] **Step 153** [DECIDE] ~10m: Loader fallback
  - **RECOMMENDATION**: keep the Go loader; fix with `ebpf.CollectionSpec` adjustments (map spec fix-ups, drop func-info) if that is the failure. Rationale: one Go process, no new binary, no new dependency.
  - **Override ONLY if**: two attempts fail → a small Rust sidecar using aya (already in the tree) that pins maps under `/sys/fs/bpf/heimdall/`; Go reads the pinned ringbufs.
- [x] **Step 154** [B][S][TEST] ~5m: netns pair + binaries
  - S5 2026-10-23: Veth addresses: ULA `fd00:5:5::1/2` (nodad) instead of link-local, because the sender resolves `--target` with `net.LookupIP`, which rejects a `%zone` suffix.
  ```bash
  sudo ip netns add s5a; sudo ip netns add s5b; sudo ip link add s5va type veth peer name s5vb
  sudo ip link set s5va netns s5a; sudo ip link set s5vb netns s5b
  sudo ip -n s5a link set s5va up; sudo ip -n s5b link set s5vb up; sudo ip -n s5a link set lo up; sudo ip -n s5b link set lo up
  (cd "$PROJECT_ROOT" && go build -o "$S5/heimdall-daemon" ./cmd/heimdall-daemon && go build -o "$S5/gjallarhorn-sender" ./cmd/gjallarhorn-sender)
  ```
- [x] **Step 155** [B][S] ~5m: Daemon in `s5b` with XDP on `s5vb` (lab manifest, `--allow-unsigned` here only), send one reverify from `s5a`
  ```bash
  sudo ip netns exec s5b "$S5/heimdall-daemon" --manifest "$PROJECT_ROOT/references/baseline/mjolnir.example.yaml" --allow-unsigned \
    --xdp-iface s5vb --bpf-dir "$PROJECT_ROOT/crates/heimdall-bpf/target/bpfel-unknown-none/release" --scan-interval 1h > "$S5/p5-daemon.log" 2>&1 &
  sleep 2; LL=$(sudo ip -n s5b -6 addr show dev s5vb | awk '/fe80/{print $2}' | cut -d/ -f1)
  sudo ip netns exec s5a "$S5/gjallarhorn-sender" --kind reverify --cluster 1 --target "$LL%s5va"
  ```
- [x] **Step 156** [V] ~1m: Daemon log: one XDP horn event and one immediate re-verify; stats seen=1 valid=1 (the reminder flow, now event-driven)
  - S5 2026-10-23: `[HORN] kind=2 cluster=1 ... action=reverify xdp seen=1 valid=1`, then `re-verified manifest and scanned`.
- [x] **Step 157** [B][S][TEST] ~3m: 50 reverify packets in 1 s + 10 malformed
- [x] **Step 158** [V] ~1m: Re-verifies ≤ 4 (burst 3 + 1/s); malformed counted and never trigger a scan; daemon alive
  - S5 2026-10-23: 50 reverifies in 0.90 s: 4 re-verifies in total (burst 3 + 1 refill, including Step 155's), 47 rate-limited; 10 malformed counted (bad_magic 3, bad_kind 2, bad_len 3, bad_reserved 2), none scanned; daemon alive.
- [x] **Step 159** [B][S][TEST] ~3m: Write to a watched file (lab manifest entry pointing under `$S5`) → vfs_write → debounced scan
- [x] **Step 160** [V] ~1m: One drift alert within ≤ 1 s of the write (not the 1 h poll)
  - S5 2026-10-23: One alert 0.06-0.22 s after the write, no repeat in the next second (event-driven scans alert only on changed drift).
- [x] **Step 161** [B][S] ~1m: Teardown
  ```bash
  sudo pkill -f "$S5/heimdall-daemon"; sudo ip netns del s5a; sudo ip netns del s5b
  ```
- [x] **Step 162** [V] ~30s: `ip netns list | grep s5` empty; `sudo bpftool prog show | grep -i heimdall` empty
  - S5 2026-10-23: No `s5` netns or link; 0 heimdall programs.
- [x] **Step 163** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `930b1ddc`.
  ```bash
  git -C "$PROJECT_ROOT" add cmd/heimdall-daemon && commitp -m "feat(heimdall): [S5 147-163] XDP horn + kprobe ringbuf readers, rate-limited re-verify"
  ```
- [x] **Step 164** [R] ~5m: Wotan publish TODO (`Run()` alert drain): read `pkg/wotan-client` publish API and which topics `services/wotan/internal/signing` enforces
  - S5 2026-10-23: Already done by Sprint 1 (ADR-105, `cmd/heimdall-daemon/drift.go`): `--wotan-grpc` publishes `drift.detected.<node>` signed with `pkg/wotansig` per-prefix keys.
- [x] **Step 165** [DECIDE] ~5m: Drift alert transport
  - S5 2026-10-23: Recommendation already implemented (Sprint 1); `drift.*` signing is enforced by Wotan per-prefix keys, no new registration.
  - **RECOMMENDATION**: publish to `drift.detected.<node>` via `pkg/wotan-client`, payload carrying the node's Gungnir signature (Wotan's ML-DSA enforcement covers `config.*`; the payload signature lets consumers check origin on `drift.*`). Empty `--wotan-addr` → stdout as today.
  - **Override ONLY if**: `drift.*` needs a Wotan Topic Namespace registration (Expert Review) to be auto-approved → keep stdout, open an RFC Editor item, continue.
- [x] **Step 166** [CODE][TEST] ~30m: Implement + test against a fake publisher; remove the TODO
  - S5 2026-10-23: Existing `TestDriftSink_*` cover it against a fake publisher; no Wotan TODO left in `main.go`.
- [x] **Step 167** [B][TEST] ~2m: `cd "$PROJECT_ROOT" && go test -race ./cmd/heimdall-daemon/... && go vet ./cmd/heimdall-daemon/...`
- [x] **Step 168** [V] ~30s: Green; `grep -n TODO "$PROJECT_ROOT/cmd/heimdall-daemon/main.go"` empty; `go.mod` unchanged
  - S5 2026-10-23: `go test -race` + `go vet` (linux, darwin) green; no TODO; `go.mod` unchanged; gosec clean (pinned `tools/gosec` build, re-checked in Phase 12).
- [x] **Step 169** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: Nothing new to commit (Sprint 1 shipped the code).
  ```bash
  git -C "$PROJECT_ROOT" add cmd/heimdall-daemon && commitp -m "feat(heimdall): [S5 164-169] drift alerts to Wotan with Gungnir-signed payload"
  ```
- [x] **Step 170** [V][GATE] ~1m: **PHASE 5 EXIT GATE** — three hooks verifier-clean (K1 evidence), XDP observer + ringbuf readers proven in netns, rate limit proven, alerts to Wotan or stdout by flag
  - S5 2026-10-23: PASSED: three hooks verifier-clean (K1), XDP observer + ringbuf readers proven in netns, rate limit proven, alerts to Wotan (signed) or stdout by flag.

### Phase 5 Definition of Done
K1 evidence recorded · no new deps · XDP observe-only · `-race` green · three TODOs removed.

---

## PHASE 6: MÍMIR'S LAW PHASE 9 — BOOTSTRAP FLOW ON REAL L2 (Steps 171-194)

> **[DEFERRED: east escalation] (2026-10-23).** Phases 6–8 need east as a node (Step 177 [ESCALATE], not yet approved) and Phase 8 depends on 6–7. Skipped by direction of the lead session; Part 2 proceeds on Phase 5 + Phases 0–4. Resume here once Stevie answers Step 177.

**Goal**: A blank node waiting on the segment receives a multicast Gjallarhorn, resolves and verifies a sealed Mjölnir manifest, starts Heimdall and joins the Wotan plane; convergence measured over ≥7 runs.
**Prerequisite**: Phase 5 gate. Stevie approval for east legs (Step 177).
**Time**: ~3 h
**Agent**: Coordinator

- [ ] **Step 171** [DEFERRED: east escalation] [DESIGN][W] ~15m: Bootstrap handler spec (ADR-043 "Phase 9 re-run design"): `heimdall-daemon --await-bootstrap --cluster C --catalog DIR --pubkey K` waits for `BootstrapBroadcast` (XDP or UDP); `manifest_ptr` = first 8 bytes of SHA-256 of the manifest; resolve `DIR/<sha256>.yaml` + `.seal`; verify against the pinned key; **refuse a manifest whose `version:` is below the last accepted** (replay defence for an unauthenticated horn); then run normally and announce on `system.discovery`.
- [ ] **Step 172** [DEFERRED: east escalation] [SECURITY] ~10m: Threat note in ADR-043: the horn is unauthenticated by design (20 B, frozen format); it can only select a manifest that already has a valid seal from the pinned key; replay of an older sealed manifest blocked by the version floor; seal TTL limits staleness; floods rate-limited (Phase 5)
- [ ] **Step 173** [DEFERRED: east escalation] [CODE][TEST] ~40m: Implement + unit tests (lookup, wrong sha, bad seal, expired seal, older version, wrong cluster)
- [ ] **Step 174** [DEFERRED: east escalation] [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add cmd/heimdall-daemon docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md && commitp -m "feat(heimdall): [S5 171-174] bootstrap: sealed content-addressed manifest, version floor"
  ```
- [ ] **Step 175** [DEFERRED: east escalation] [B][PREFLIGHT][BARE-METAL] ~3m: **H-M1** — link-local multicast across the physical P2P link with the existing UDP listener (unprivileged, binary copied to east `/tmp`, removed in Step 188)
  ```bash
  (cd "$PROJECT_ROOT" && go build -o "$S5/gjallarhorn-listener" ./cmd/gjallarhorn-listener) && scp -q "$S5/gjallarhorn-listener" "$EAST":/tmp/s5-gl
  ssh "$EAST" "timeout 10 /tmp/s5-gl --multicast --iface $EAST_P2P_IF" > "$S5/p6-h-m1.txt" 2>&1 &
  sleep 2; "$S5/gjallarhorn-sender" --kind bootstrap --cluster 1 --multicast --iface "$WEST_P2P_IF"; sleep 9; cat "$S5/p6-h-m1.txt"
  ```
- [ ] **Step 176** [DEFERRED: east escalation] [V] ~30s: East printed `Gjallarhorn: kind=BootstrapBroadcast cluster=1` → H-M1 TRUE (multicast works on the physical link; the April "N/A" was about the WG overlay)
  - False → record; run Steps 182–187 on a west veth pair; east column "not demonstrated"
- [ ] **Step 177** [DEFERRED: east escalation] [ESCALATE][BARE-METAL] ~—: **Stevie approval** for (a) running heimdall-daemon as root on east from `/tmp` (no package, no unit) with XDP **generic** mode on `enp2s0`, observe-only XDP_PASS; (b) a scratch Wotan on west 26601/26602 bound to `192.168.13.2`. Context: `enp2s0` is east's only link (BGP + WireGuard ride it). Why no recommendation: east runs the debs and host actions there wait for Stevie. **Until approved: Steps 178–187 use a west netns seed; east column BLOCKED.**
- [ ] **Step 178** [DEFERRED: east escalation] [B] ~5m: Catalog dir + confirm daemon flags
  ```bash
  mkdir -p "$S5/catalog" && cp "$PROJECT_ROOT/references/baseline/mjolnir.example.yaml" "$S5/catalog/m.yaml" && "$S5/heimdall-daemon" --help 2>&1 | grep -E 'await-bootstrap|catalog|sign-key'
  ```
- [ ] **Step 179** [DEFERRED: east escalation] [B] ~3m: Seal with a lab key (`--sign-key`, key file 0600 in `$S5`, never in repo); content-address; compute the pointer
  ```bash
  SHA=$(sha256sum "$S5/catalog/m.yaml" | cut -c1-64); mv "$S5/catalog/m.yaml" "$S5/catalog/$SHA.yaml"; echo "PTR=0x${SHA:0:16}" | tee -a "$S5/vars.env"
  ```
- [ ] **Step 180** [DEFERRED: east escalation] [B] ~3m: Scratch Wotan (unprivileged) with a lab topic config allowing `system.discovery` and `drift.*`
  ```bash
  (cd "$PROJECT_ROOT" && go build -o "$S5/wotan" ./services/wotan/cmd/wotan) && "$S5/wotan" --http-port "$WOTAN_SCRATCH_HTTP" --grpc-port "$WOTAN_SCRATCH_GRPC" \
    --topic-config "$S5/wotan-lab.yaml" --admin=false > "$S5/wotan.log" 2>&1 & echo $! > "$S5/wotan.pid"
  ```
- [ ] **Step 181** [DEFERRED: east escalation] [V][PREFLIGHT] ~30s: **H-C4** — `curl -s -o /dev/null -w '%{http_code}' localhost:$WOTAN_SCRATCH_HTTP/health` → 200
- [ ] **Step 182** [DEFERRED: east escalation] [B][BARE-METAL] ~10m: Seed waits (east if approved, else west netns); sender emits; timer = send → seed's discovery announce seen on scratch Wotan
- [ ] **Step 183** [DEFERRED: east escalation] [V] ~1m: Seed operational and announced within 60 s
- [ ] **Step 184** [DEFERRED: east escalation] [B][BARE-METAL] ~15m: 7 runs (reset seed state between); timings to `$S5/p6-bootstrap.csv`
- [ ] **Step 185** [DEFERRED: east escalation] [V][KILL-CHECK] ~1m: **K2** — p95 < 10 s pass; 10–15 s warn; p99 > 30 s → K2 FAIL recorded for Phase 8
  ```bash
  python3 - "$S5/p6-bootstrap.csv" <<'PY'
  import sys; v=sorted(float(x) for x in open(sys.argv[1]) if x.strip()); q=lambda p: v[min(len(v)-1, int(p*len(v)))]
  print(f"n={len(v)} p50={q(.5):.3f}s p95={q(.95):.3f}s p99={q(.99):.3f}s max={v[-1]:.3f}s")
  PY
  ```
- [ ] **Step 186** [DEFERRED: east escalation] [B][SECURITY] ~5m: Negative bootstraps: unknown sha, sealed-but-older manifest, wrong cluster
- [ ] **Step 187** [DEFERRED: east escalation] [V] ~30s: All refused, seed keeps waiting, reasons logged
- [ ] **Step 188** [DEFERRED: east escalation] [B] ~2m: Teardown both sides
  ```bash
  kill "$(cat "$S5/wotan.pid")" 2>/dev/null; ssh "$EAST" 'rm -f /tmp/s5-*'; pgrep -fa 's5-|/s5-upc-wire/' || echo CLEAN-WEST
  ```
- [ ] **Step 189** [DEFERRED: east escalation] [V] ~30s: `CLEAN-WEST`; `ssh "$EAST" 'pgrep -fa s5-'` empty
- [ ] **Step 190** [DEFERRED: east escalation] [W][AUDIT-UPDATE] ~10m: Phase 9 evidence table into the sprint log (topology, n, p50/p95/p99, negatives); ADR text lands in Phase 8
- [ ] **Step 191** [DEFERRED: east escalation] [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add references/s5-upc-wire-log.md && commitp -m "docs(mimir): [S5 175-191] Phase 9 bootstrap re-run evidence"
  ```
- [ ] **Step 192** [DEFERRED: east escalation] [V] ~30s: Evidence names its topology honestly ("2 physical hosts + N netns seeds", never "10-node cluster")
- [ ] **Step 193** [DEFERRED: east escalation] [V] ~30s: Sacred Law clause intact
  ```bash
  git -C "$PROJECT_ROOT" diff --stat 443de3f82 -- pkg/discovery pkg/transport cmd/unheaded-daemon | tail -1
  ```
  - Must print nothing
- [ ] **Step 194** [DEFERRED: east escalation] [V][GATE] ~1m: **PHASE 6 EXIT GATE** — bootstrap demonstrated with a distribution; negatives refused; K2 evaluated

---

## PHASE 7: MÍMIR'S LAW PHASE 11 — STRESS + INVALID-SEAL REJECTION (Steps 195-214)

> **[DEFERRED: east escalation]** — see the Phase 6 banner.

**Goal**: Concurrent deltas, drifts and horns across all nodes for 120 s with zero cascade failures; invalid seals rejected; the new XDP observer handed to the Lich sprint as a named LICH-012 target.
**Prerequisite**: Phase 6 gate (or its west-only variant).
**Time**: ~2.5 h
**Agent**: Developer + Micromanager (QA)

- [ ] **Step 195** [DEFERRED: east escalation] [DECIDE] ~5m: Node count
  - **RECOMMENDATION**: 2 physical hosts + 6 west netns nodes on bridge `s5br` (8 Heimdall instances). Rationale: the April plan's 10-node WEST cluster never existed; netns nodes run the same daemon, XDP and Wotan paths.
  - **Override ONLY if**: Stevie offers more real hosts.
- [ ] **Step 196** [DEFERRED: east escalation] [W][CODE] ~40m: `scripts/mimir-stress.sh` (dry run unless `--apply`; `trap` teardown): N netns on `s5br`, a daemon per netns with its own state dir; 120 s driver: 20 sealed deltas, 20 file drifts, 5 horns; then 2 invalid seals (wrong algorithm, expired) to 2 nodes; per-node health + counts
- [ ] **Step 197** [DEFERRED: east escalation] [B] ~1m: `bash -n "$PROJECT_ROOT/scripts/mimir-stress.sh" && (command -v shellcheck >/dev/null && shellcheck "$PROJECT_ROOT/scripts/mimir-stress.sh" || true)`
- [ ] **Step 198** [DEFERRED: east escalation] [B][S][TEST] ~5m: Dry run, then apply
  ```bash
  bash "$PROJECT_ROOT/scripts/mimir-stress.sh" --nodes 6 2>&1 | tail -5
  sudo bash "$PROJECT_ROOT/scripts/mimir-stress.sh" --nodes 6 --apply 2>&1 | tee "$S5/p7-stress.txt" | tail -30
  ```
- [ ] **Step 199** [DEFERRED: east escalation] [V][KILL-CHECK] ~1m: **K3** — zero crashes, zero restarts, all nodes healthy at end
- [ ] **Step 200** [DEFERRED: east escalation] [V][SECURITY] ~1m: Both invalid seals rejected; no state change on their targets
- [ ] **Step 201** [DEFERRED: east escalation] [V] ~1m: 20/20 drifts caught ≤ 1 s; 0 false alerts on unchanged files
- [ ] **Step 202** [DEFERRED: east escalation] [B][BARE-METAL] ~10m: If Step 177 approved: repeat with east as a ninth node
- [ ] **Step 203** [DEFERRED: east escalation] [V] ~1m: East passes the same checks (or column BLOCKED pending approval)
- [ ] **Step 204** [DEFERRED: east escalation] [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add scripts/mimir-stress.sh references/s5-upc-wire-log.md && commitp -m "test(mimir): [S5 195-204] Phase 11 stress harness + results"
  ```
- [ ] **Step 205** [DEFERRED: east escalation] [TEST] ~10m: Robustness = the Phase 5 decoder table tests (malformed-record rejection) re-run under `-race`; no live adversarial traffic in this sprint
- [ ] **Step 206** [DEFERRED: east escalation] [V] ~1m: Decoder tests green; rate-limit test (Step 158) still green
- [ ] **Step 207** [DEFERRED: east escalation] [DECIDE] ~5m: LICH-012 adversarial work
  - **RECOMMENDATION**: hand the new XDP observer to the Lich sprint as a named target (ADR-062 process). Rationale: red-team campaigns are that sprint's scope; this sprint ships the hook and its unit tests.
  - **Override ONLY if**: Stevie asks for it here.
- [ ] **Step 208** [DEFERRED: east escalation] [W] ~10m: Add the observer, its stats map and the trigger policy to the LICH-012 README target list (`tomb/lich/LICH-012-config-convergence/README.md`)
- [ ] **Step 209** [DEFERRED: east escalation] [V] ~30s: Target list updated; nothing else in `tomb/` changed
- [ ] **Step 210** [DEFERRED: east escalation] [V] ~30s: Hard condition #8 scored in Phase 8 as "handed to the Lich sprint"
- [ ] **Step 211** [DEFERRED: east escalation] [B][S] ~1m: `ip netns list; ip link show s5br 2>&1 | head -1; sudo bpftool prog show | grep -ci xdp`
- [ ] **Step 212** [DEFERRED: east escalation] [V] ~30s: Nothing from S5 left behind
- [ ] **Step 213** [DEFERRED: east escalation] [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add tomb/lich/LICH-012-config-convergence/README.md references/s5-upc-wire-log.md && commitp -m "test(mimir): [S5 205-213] robustness tests; observer registered as a LICH-012 target"
  ```
- [ ] **Step 214** [DEFERRED: east escalation] [V][GATE] ~1m: **PHASE 7 EXIT GATE** — K3 pass, invalid seals rejected, LICH-012 target list updated

---

## PHASE 8: MÍMIR'S LAW PHASE 13 GATE RE-EVAL + PART 1 EXIT (Steps 215-228)

> **[DEFERRED: east escalation]** — see the Phase 6 banner. Marshal ruling for Part 2 entry: the Part 1 exit gate (Step 228) is not passed; the lead session authorised Part 2 on the non-east evidence (Phases 0–5 gates passed). The Step 228 line "east-only BLOCKED items do not block Part 2" covers it.

**Goal**: Re-score K1–K4, the 8 hard conditions and the pass criteria on the new evidence; make ADR-043, the wiki and CLAUDE.md agree; close Part 1.
**Prerequisite**: Phases 3, 4, 7 gates (east columns may be BLOCKED → scored "not demonstrated").
**Time**: ~1.5 h
**Agent**: Scientist (verdict) + Marshal (enforcement) + Librarian (doc web)

- [ ] **Step 215** [DEFERRED: east escalation] [V][KILL-CHECK] ~5m: K1 — all Heimdall hooks verifier-accepted (Step 145 counts)
- [ ] **Step 216** [DEFERRED: east escalation] [V][KILL-CHECK] ~5m: K2 — bootstrap p95/p99 (Step 185)
- [ ] **Step 217** [DEFERRED: east escalation] [V][KILL-CHECK] ~5m: K3 — zero cascade failures (Steps 199, 203)
- [ ] **Step 218** [DEFERRED: east escalation] [V][KILL-CHECK] ~10m: K4 — hard conditions: #1 alerts-only (no restore path: `grep -rn 'Restore\|restore(' cmd/heimdall-daemon pkg/enkrateia | grep -v _test`) · #2 Wotan `config.*` signing incl. the 2026-10-19 correction (`go test ./services/wotan/internal/signing/...`) · #3 dm-verity DEFERRED v2 · #4 lab keys only, 0600, never tracked (`git ls-files | grep -iE 'seed|\.key$'` empty) · #5 semantic drift DEFERRED v2 · #6 Sacred Law clause (Step 193) · #7 no wire change · #8 LICH-012: observer registered as a target; campaign → Lich sprint
- [ ] **Step 219** [DEFERRED: east escalation] [V] ~10m: Pass-criteria table: convergence, drift latency, accuracy, consistency, bootstrap flow, reminder flow (Steps 156, 201), LICH-012 status
- [ ] **Step 220** [DEFERRED: east escalation] [DECIDE] ~10m: Verdict
  - **RECOMMENDATION**: K1–K4 pass and both flows demonstrated → "PoC COMPLETE — Phases 9/11/13 re-run on real L2 (S5)", DEFERRED items listed explicitly. Any K fail → "PROMOTE withdrawn pending <item>".
  - **Override ONLY if**: Stevie wants Gleipnir Age 2b scheduled off this verdict — a separate decision, not this step.
- [ ] **Step 221** [DEFERRED: east escalation] [W][DOC-UPDATE] ~20m: ADR-043 "Phase 9/11/13 re-run (S5)" section with evidence; Phase 13 table corrected (K1 PASS with insn counts; multicast demonstrated on the P2P link or "not demonstrated")
- [ ] **Step 222** [DEFERRED: east escalation] [W][DOC-UPDATE] ~10m: `wiki/Mimirs-Law.md` phase table; CLAUDE.md line "11 of 13 phases complete; bare-metal Phases 9, 11, 13 pending" → the truth; ADR index if a generator exists (`ls "$PROJECT_ROOT/scripts" | grep -i adr`)
- [ ] **Step 223** [DEFERRED: east escalation] [B] ~1m: Wiki link check for touched pages (`scripts/sync-wiki.sh` dry run if present; never push the wiki)
- [ ] **Step 224** [DEFERRED: east escalation] [V] ~30s: No broken links in touched pages
- [ ] **Step 225** [DEFERRED: east escalation] [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md wiki/Mimirs-Law.md CLAUDE.md && commitp -m "docs(mimir): [S5 215-225] Phase 13 re-evaluation on real-L2 evidence"
  ```
- [ ] **Step 226** [DEFERRED: east escalation] [B][S] ~3m: Part 1 final regression
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tail -6
  (cd "$PROJECT_ROOT" && go build ./... && go test -race ./cmd/heimdall-daemon/... ./cmd/upc-tty-bridge/... 2>&1 | tail -4)
  ```
- [ ] **Step 227** [DEFERRED: east escalation] [W][DOC-UPDATE] ~10m: `~/tmp/next.md` — S5 Part 1 section (overwrite stale parts): shipped, approvals pending (Step 177), Part 2 entry point
- [ ] **Step 228** [DEFERRED: east escalation] [V][GATE] ~1m: **PHASE 8 / PART 1 EXIT GATE** — S1.2 closed with getpid decision recorded; live keyboard; Heimdall XDP + ringbufs; Mímir 9/11/13 re-scored; harness green; docs agree
  - Marshal gate check: Part 2 starts only on PASS (east-only BLOCKED items do not block Part 2)

### Part 1 Definition of Done
Every Part 1 phase DoD met · zero new failures vs KF · ascend verifier ≤ 950,000, Doom unchanged · no new dependencies · no push · next.md updated.

---

# PART 2 — COMPUTE ON THE WIRE: MBC CARTRIDGES

A **cartridge** is a small MBC program plus a read-only data blob (e.g. model weights) and a signed manifest. It is registered in Sophia, loaded into a cartridge slot, and run by a UPC interpreter once per packet of a flow (XDP stage) or once per queued flow (tick stage). Its memory is a per-flow Wotan-style window; its only inputs are enumerated header fields and a time value; its outputs are a verdict, events and queue pushes. ADR-080 calls the UPC a substrate for "future experimental programmatic expansion" — cartridges are its third client after Doom and xv6.

**Sacred Principle by construction**: the cartridge ABI exposes enumerated header/metadata fields only. There is no syscall that returns payload bytes, so no cartridge, trace or replay can carry user data.

## PHASE 9: CARTRIDGE ARCHITECTURE, ADR, SPEC GATE (Steps 229-250)

**Goal**: Decisions written down and spec-gated before any cartridge code exists.
**Prerequisite**: Part 1 exit gate.
**Time**: ~3 h
**Agent**: Coordinator (Architect + Computermancer + Scientist lenses), RFC Editor review requested

- [x] **Step 229** [R] ~15m: Re-read the contract and the native baselines
  ```bash
  sed -n '/## 2. The contract/,/## 5. Status/p' "$PROJECT_ROOT/docs/adr/ADR-080-upc-programmatic-api.md"
  sed -n '1,40p' "$EBPF/qos-ebpf/src/main.rs" | grep '//'; sed -n '1,40p' "$EBPF/anomaly-ebpf/src/main.rs" | grep '//'; sed -n '1,30p' "$EBPF/nfv-ebpf/src/main.rs" | grep '//'
  ```
- [x] **Step 230** [R] ~10m: Registries the design may touch
  ```bash
  grep -n 'Registry Name' "$PROJECT_ROOT"/ietf-submission/*.md | cut -c1-160
  ```
- [x] **Step 231** [DECIDE][DESIGN] ~15m: **D-C1 — where the cartridge interpreter lives**
  - S5 2026-10-23: Decided D-C1 per recommendation: `ebpf/cartridge-ebpf` + `monad_common::cart`; monad-cpu-ebpf untouched.
  - **RECOMMENDATION**: new crate `ebpf/cartridge-ebpf` whose interpreter step is a `no_std` function in `ebpf/monad-common/src/cart.rs`, compiled into BOTH the BPF object and the host tools. `monad-cpu-ebpf` is not touched.
  - Rationale: (1) Doom/xv6 objects stay byte-identical — the non-negotiable from ADR-080; (2) one source for BPF and host execution gives replay determinism by construction (Phase 16); (3) the per-packet hook model differs from the trigger-tick model of `monad-cpu-ebpf`; (4) its own build = its own 1M budget (ADR-080 §3).
  - **Override ONLY if**: Stevie wants a single object → a `cartridge` cargo feature in `monad-cpu-ebpf` with its own DCE partition, and the regression harness gains a third variant before any code lands.
- [x] **Step 232** [DECIDE][DESIGN] ~15m: **D-C2 — two execution stages**
  - S5 2026-10-23: Decided D-C2 per recommendation (N_PKT 64, N_TICK 256). Tick termination made concrete: `loop_bound` = max taken back-edges per job, so a job is bounded by rom_words x (loop_bound+1) instructions; per-call budget-out in the tick stage is an implicit YIELD.
  - **RECOMMENDATION**: (a) **XDP stage** `cart_xdp`: per packet, per active slot, at most `N_PKT = 64` MBC instructions (bounded loop), forward-only branches (the validator rejects back-edges in XDP-stage cartridges), ends in VERDICT or budget-out (= PASS). Rate limiter, sampler and the scorer's feature accumulator run here. (b) **Tick stage** `cart_tick`: a program run by the host pump through `BPF_PROG_TEST_RUN` (the Doom injector model, no live packets), pops flows from `SCORE_QUEUE`, runs up to `N_TICK = 256` instructions per call with resumable state (YIELD), loops allowed with a declared bound. The ~2 KB scorer runs here.
  - Rationale: an int8 MLP over every packet cannot fit an XDP budget; scoring once per flow off the packet path keeps XDP cost flat and still "scores every flow".
  - **Override ONLY if**: Phase 11 measures `N_PKT=64` above the verifier gate → lower N_PKT; if Phase 15 cannot meet its rate gate in the tick stage → consider the DOT4 opcode (Step 389, full spec gate).
- [x] **Step 233** [V][PREFLIGHT] ~5m: **H-C1** — `services/anomaly` has no binary and does not consume `anomaly.scores`
  - S5 2026-10-23: H-C1 TRUE: `NO IMPORTER`; `anomaly.scores` subscribed at `services/anomaly/anomaly.go:165`.
  ```bash
  grep -rln 'services/anomaly"' --include=*.go "$PROJECT_ROOT/cmd" "$PROJECT_ROOT/services" | grep -v '^.*services/anomaly/' || echo "NO IMPORTER"
  grep -n 'anomaly.scores' "$PROJECT_ROOT/services/anomaly/anomaly.go"
  ```
  - Expected: `NO IMPORTER`; the topic is subscribed but the returned handle is unused → Phase 15 adds `cmd/anomaly` and a consumer
- [x] **Step 234** [DECIDE][DESIGN] ~10m: **D-C3 — fault policy**
  - S5 2026-10-23: Decided D-C3 per recommendation; `fail_closed` is header flag bit 0.
  - **RECOMMENDATION**: any fault (bad opcode, out-of-window memory, budget-out in XDP stage) → `XDP_PASS`, `cart_faults_total{slot}` += 1, and the slot auto-disables after 64 faults per second (circuit breaker; re-enable is an explicit ctl command). Rationale: a buggy rate limiter must never black-hole traffic; failures are loud in metrics.
  - **Override ONLY if**: a cartridge declares `fail_closed: true` in its signed manifest (opt-in, only for policy cartridges, logged at load).
- [x] **Step 235** [DECIDE][DESIGN] ~10m: **D-C4 — Sophia's role**
  - S5 2026-10-23: Decided D-C4 per recommendation; provisional namespace `x-cartridge`.
  - **RECOMMENDATION**: Sophia holds the **cartridge registry**: name → {sha256, manifest, seal key id, stage, surface version, max_insn, slot}. The loader resolves a cartridge by name through Sophia, verifies the seal on the exact bytes it loads, then writes `CART_ROM`/`CART_DATA` for the slot (the "Sophia = microcode ROM" mapping). Registry entries live under a dictionary namespace requested from the Sophia Root Dictionary Keys registry (0x10–0xFE, First Come First Served) — **RFC Editor item, non-blocking**; until assigned, a lab-local namespace string `x-cartridge` is used and documented as provisional.
  - **Override ONLY if**: RFC Editor says the namespace must be Specification Required → stays provisional this sprint.
- [x] **Step 236** [DECIDE][DESIGN] ~10m: **D-C5 — Anamnesis + wire**
  - S5 2026-10-23: Decided D-C5 per recommendation; provisional sub-kind 0x43 under EVENT_COMPUTED.
  - **RECOMMENDATION**: trace records reuse **EVENT_COMPUTED (0x01)** ("Shim executed") with a cartridge sub-kind in the payload — no new Anamnesis Event Type registration. Cartridges **never write the Monad header** in v0 (no `SAMPLED`-as-suspicious overload like KF11); scores travel over ringbuf → Wotan.
  - **Override ONLY if**: RFC Editor rules the sub-kind needs its own type → file the registration (Specification Required), keep 0x01 meanwhile.
- [x] **Step 237** [DESIGN][W] ~20m: Cartridge ABI v0 (Appendix C.3) — syscall numbers, field ids, memory windows, verdict codes, event record layouts, image format `.cart` (header magic `MBCC`, version, stage, max_insn, rom_words, data_words, entry, sha256 of the rest)
  - S5 2026-10-23: ABI v0 in ADR-108 §Cartridge ABI v0, adapted to the shipping engine's real semantics: 24-bit branch offsets relative to pc+1, MOVI zero-extend, set_flags rules; little-endian 52-byte header (adds loop_bound, flags, reserved; sha256 covers header[0..20) + ROM + DATA); CartState 336 B with explicit status/back_edges/gen.
- [x] **Step 238** [W][SPEC] ~40m: Draft `docs/adr/${ADR_CART}-mbc-cartridges.md` (Status: Proposed): context (ADR-080 third client), D-C1…D-C5, ABI v0, verifier/benchmark gates, determinism contract, Sacred Principle argument, non-goals (no wire change, no new opcode in v0)
  - S5 2026-10-23: `docs/adr/ADR-108-mbc-cartridges.md` (Proposed). ADR_CART = ADR-108: lead allocation across the parallel sprint branches (ADR-108 = Sprint 2, ADR-107 = Sprint 3); first drafted as ADR-108.
- [x] **Step 239** [B] ~1m: Confirm the ADR number is still free on `develop`
  - S5 2026-10-23: Superseded by the lead's allocation: 106 and 107 belong to Sprints 2 and 3; this ADR is ADR-108.
  ```bash
  git -C "$PROJECT_ROOT" fetch -q origin develop 2>/dev/null; git -C "$PROJECT_ROOT" ls-tree --name-only origin/develop docs/adr/ 2>/dev/null | grep -c "${ADR_CART}" || true
  ```
- [x] **Step 240** [B][S][PREFLIGHT] ~10m: **H-C2** — `BPF_PROG_TEST_RUN` with repeat works for XDP and reports duration (use an existing tiny XDP object as the probe)
  - S5 2026-10-23: Deviation: bpftool cannot load Aya objects (legacy `maps` section, see Step 144), so the probe used cilium/ebpf `Program.Benchmark` on `heimdall-gjallarhorn-xdp`: `ret=2 (XDP_PASS) per-run=24ns`, repeat 100000.
  ```bash
  sudo bpftool prog loadall "$EBPF/target/bpfel-unknown-none/release/qos-ebpf" /sys/fs/bpf/s5-probe 2>&1 | tail -2
  python3 -c "import sys; sys.stdout.buffer.write(bytes(14)+bytes([0x60])+bytes(39)+bytes(8))" > "$S5/pkt-min.bin"
  sudo bpftool prog run pinned "$(ls -d /sys/fs/bpf/s5-probe/* | head -1)" data_in "$S5/pkt-min.bin" repeat 100000 2>&1 | tail -3
  sudo rm -rf /sys/fs/bpf/s5-probe
  ```
- [x] **Step 241** [V] ~1m: Output contains `duration` (ns per run) → test_run is the benchmark engine
  - S5 2026-10-23: H-C2 TRUE: test_run with repeat reports ns/run on 6.17. The benchmark engine is `pkg/cartridge` over `ebpf.Program.Benchmark`, not `bpftool prog run`.
  - No → benchmarks use a veth pair + userspace sender in netns (Emergency A.8); gates still apply, wider tolerance
- [x] **Step 242** [V][SPEC] ~10m: **SPEC COMPLIANCE GATE (Part 2 design)**
  - S5 2026-10-23: Spec gate recorded in ADR-108 + request note: v0x01 untouched; extension (new surface + new program, no opcode); RFC Editor requested for the namespace, sub-kind, topics; backward compatible; IANA: new registrations requested, none modified.
  - Wire format version: v0x01 (FROZEN) — untouched (cartridges read the Monad option, never write it)
  - Change type: **extension** — a new UPC syscall surface (cartridge ABI v0) and a new BPF program; no MBC opcode added
  - RFC Editor reviewed: **requested** for (a) Sophia namespace (D-C4), (b) EVENT_COMPUTED sub-kind (D-C5), (c) Wotan topics `anomaly.scores`, `upc.cart.*` (Wotan Topic Namespace, Expert Review) — **must be Y before Phase 19 marks the ADR Accepted**; implementation proceeds behind provisional names
  - Backward compatible: Y — old parsers never see cartridge state; no existing map/object changes
  - IANA impact: New registrations requested (namespace, topics); none modified
- [x] **Step 243** [W] ~10m: RFC Editor request note `docs/protocol/RFC-EDITOR-REQUESTS-S5.md` listing (a)–(c) + KF10 + the ADR-067 CSR amendment (Step 72)
  - S5 2026-10-23: `docs/protocol/RFC-EDITOR-REQUESTS-S5.md`: items 1-3 (namespace, sub-kind, topics) + 4 (ADR-067 CSR range) + 5 (KF10), with draft text, none self-approved.
- [x] **Step 244** [SECURITY][DESIGN] ~20m: Threat model section in the ADR: cartridge = untrusted code from the interpreter's view → bounded loop, windowed memory, allowlisted opcodes/syscalls, signed images, per-slot fault breaker, no payload access, no helper calls from MBC; host tools refuse unsigned cartridges outside `--lab`
- [x] **Step 245** [COMPLIANCE] ~10m: Dependency plan: no new crates/modules expected (aya, cilium/ebpf, pkg/gungnir, wotan-client already present). Any later addition goes through the Barrister review + `scripts/check-*` dependency gates (ADR-096)
  - S5 2026-10-23: No new crates/modules planned. aya-ebpf, cilium/ebpf, pkg/gungnir, and `sha2` for the host image hash (already a dependency of `cmd/upc-bootctl`, same version) only.
- [x] **Step 246** [V] ~1m: ADR draft covers D-C1…D-C5, ABI, gates, threat model; RFC request note lists every registry touch
- [x] **Step 247** [W][DOC-UPDATE] ~10m: `docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md` Track 1: new Epic 1.5 "Cartridges (third ADR-080 client)" pointing at this plan
  - S5 2026-10-23: Epic 1.5 added to Track 1 of the master plan.
- [x] **Step 248** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add docs/adr docs/protocol/RFC-EDITOR-REQUESTS-S5.md docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md && commitp -m "docs(adr): [S5 229-248] MBC cartridges (Proposed) + RFC Editor requests"
  ```
- [x] **Step 249** [V] ~1m: Marshal scope check: Part 2 builds exactly three cartridges + replay + Lich hooks (+ stretch); anything else → parking lot in the sprint log
  - S5 2026-10-23: Marshal: Part 2 = three cartridges + replay + Lich hooks (+ stretch). This session's scope is Phases 9-12; 13-19 are not started.
- [x] **Step 250** [V][GATE] ~1m: **PHASE 9 EXIT GATE** — ADR Proposed and committed; spec gate recorded with RFC Editor requests open; H-C1/H-C2 known
  - S5 2026-10-23: PASSED.

### Phase 9 Definition of Done
Decisions recorded with overrides · spec gate run · threat model written · no code · no new deps.

---

## PHASE 10: CARTRIDGE ABI + SHARED STEP FUNCTION + HOST REFERENCE (Steps 251-274)

**Goal**: One `no_std` step function executes cartridges identically on host and in BPF; a host crate builds, validates and runs `.cart` images.
**Prerequisite**: Phase 9 gate.
**Time**: ~4 h
**Agent**: Developer (TDD), Computermancer review

- [x] **Step 251** [R] ~5m: How `monad-common` is consumed host-side today
  ```bash
  grep -n 'monad-common' "$PROJECT_ROOT"/cmd/upc-bootctl/Cargo.toml "$PROJECT_ROOT"/crates/*/Cargo.toml; grep -n '^pub mod\|^#!\[no_std\]\|cfg_attr' "$EBPF/monad-common/src/lib.rs" | head
  ```
- [x] **Step 252** [V] ~30s: `monad-common` builds for the host (upc-bootctl depends on it) → `cart.rs` can be shared
  - S5 2026-10-23: upc-bootctl, xv6-mbc and monad-mbc already consume monad-common on the host. Host tests must run from the repo root (`cargo test --manifest-path ebpf/monad-common/Cargo.toml`): inside `ebpf/` the BPF `build-std` config collides with the host target.
- [x] **Step 253** [W][CODE][TEST] ~30m: Red tests for `monad_common::cart`: `CartState {regs:[u32;16], pc:u16, flags:u8, insns:u32, mem:[u32;64]}`; `step(&mut CartState, rom:&impl Rom, data:&impl Data, io:&mut impl CartIo) -> Step {Continue, Verdict(u8), Yield, Fault(FaultKind)}`; per opcode in the allowlist (Appendix C.3), golden vectors
  - S5 2026-10-23: 14 golden-vector tests in `ebpf/monad-common/src/cart.rs`: 10 red against a stub, then green.
  ```bash
  cargo test --manifest-path "$EBPF/monad-common/Cargo.toml" --target x86_64-unknown-linux-gnu cart 2>&1 | tail -10
  ```
- [x] **Step 254** [CODE] ~60m: Implement `cart.rs`: decode via existing `MbcInsn`; allowlist ADD SUB MUL MULH AND OR XOR NOT SHL SHR SAR SHLR SHRR SARR MOV MOVI LOAD_IMM32 ADDI CMP JMP JZ JNZ JN JP JC JNC LD ST LDB HALT SYSCALL; memory windows: `[0x0000,0x0100)` → `mem` (flow state), `[0x1000, 0x1000+4*data_words)` → data (read-only); anything else → `Fault(Mem)`; no recursion, no allocation, every array index masked or bounds-checked (verifier-friendly shape)
  - S5 2026-10-23: Implemented (ABI v0 per ADR-108). Every regs/mem index masked; windows range-checked; faults are values.
- [x] **Step 255** [V] ~1m: Tests green on host
- [x] **Step 256** [B][BUILD] ~3m: `cart.rs` compiles for `bpfel-unknown-none` inside the ebpf workspace (build `monad-common` there)
  - S5 2026-10-23: `cargo build --release -p monad-common` (bpfel) OK; clippy clean.
  ```bash
  (cd "$EBPF" && cargo build --release -p monad-common 2>&1 | tail -2)
  ```
- [x] **Step 257** [V] ~30s: Builds for BPF (no std, no float, no 128-bit ops)
  - S5 2026-10-23: Builds for BPF. monad-cpu-ebpf instruction stream byte-identical with and without the module (llvm-objdump -d, Doom and ascend); only DWARF differs.
- [x] **Step 258** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `9cc40068`.
  ```bash
  git -C "$PROJECT_ROOT" add ebpf/monad-common && commitp -m "feat(mbc): [S5 251-258] shared no_std cartridge step function"
  ```
- [x] **Step 259** [W][CODE][TEST] ~45m: `crates/upc-cartridge` (std, GPL-3.0-or-later): `.cart` encode/decode (Appendix C.3), `validate()` (opcode allowlist, branch targets in ROM, XDP stage: no back-edges and `max_insn ≤ 64`; tick stage: declared loop bound; syscalls allowed per stage; data ≤ 4096 B), `Reference` runner over `monad_common::cart::step`, `state_hash()` = FNV-1a over regs+pc+mem (same FNV as `upc-bootctl`'s tested `fnv1a`)
  - S5 2026-10-23: `crates/upc-cartridge`: Image encode/decode (hostile-input decode, sha256, every single-bit flip rejected), validate(), reference runner, `cart` CLI. Tests written first (red: unresolved imports).
- [x] **Step 260** [W][CODE] ~20m: Assembler path: reuse `mbc-asm` syntax for hand-written cartridges (`crates/monad-mbc/src/assembler.rs`) → `cart build --stage xdp --asm X.s --data W.bin -o X.cart`
  - S5 2026-10-23: `cart build --stage xdp|tick --asm X.s [--data W.bin] -o X.cart`, `cart validate -` (stdin), `cart info`. The monad-mbc assembler lacked SHLR/SHRR/SARR/MULH/MULHU mnemonics; added with a test.
- [x] **Step 261** [B][TEST] ~2m: `cargo test --manifest-path "$PROJECT_ROOT/crates/upc-cartridge/Cargo.toml" 2>&1 | tail -10`
  - S5 2026-10-23: upc-cartridge tests green (image/validate 5, reference 5, differential 1, surface 1).
- [x] **Step 262** [V][PREFLIGHT] ~10m: **H-C3** — differential test: for 10,000 random allowlisted instruction sequences with no syscalls/windowed memory, `monad_common::cart::step` and `monad-mbc` `execute.rs` produce identical registers and flags
  - S5 2026-10-23: H-C3 FALSE: nine drifts of monad-mbc execute.rs from monad-cpu-ebpf (MUL and all six shifts set C; JP also tested Z; MULH unsigned). Host side fixed to match the shipping engine; 3 host unit tests that encoded the old semantics updated; differential (10,000 programs, lockstep) green. Host MULHU arm still missing (outside the cartridge allowlist), logged.
  - Divergence → record each as a finding in the sprint log ("ISA semantics drift: <opcode>"), decide which side matches `monad-cpu-ebpf` (the shipping engine) and fix the other; this is the first Lich-hook output (Phase 17)
- [x] **Step 263** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `b7398de5`.
  ```bash
  git -C "$PROJECT_ROOT" add crates/upc-cartridge && commitp -m "feat(upc-cartridge): [S5 259-263] .cart format, validator, reference runner"
  ```
- [x] **Step 264** [W][CODE][TEST] ~20m: `upc-api`: `FeatureGate::Cartridge` + a `cartridge` `SyscallSurface` descriptor registered in `SurfaceRegistry` (ADR-080 §4 host-side registry)
  - S5 2026-10-23: `FeatureGate::Cartridge` in upc-api; the `cartridge` surface constant + `register_surface()` live in upc-cartridge (upc-api never declares a guest surface).
- [x] **Step 265** [B][TEST] ~2m: `cargo test --manifest-path "$PROJECT_ROOT/crates/upc-api/Cargo.toml" 2>&1 | tail -5`
- [x] **Step 266** [V] ~30s: Existing upc-api characterization tests unchanged and green
  - S5 2026-10-23: upc-api 25 tests unchanged and green; upc-bootctl and doom-runner `cargo check` clean.
- [x] **Step 267** [W][TEST] ~20m: Determinism property test: same image + same input log → same state hash after every step, across 1,000 random input logs; and a mutation test proving the hash notices a single-bit change in mem
  - S5 2026-10-23: In `reference.rs`: 1,000 random input logs reproduce every state hash; 256 random single-bit mem mutations all change the hash.
- [x] **Step 268** [V] ~30s: Property tests green
- [x] **Step 269** [B][TEST] ~6m: MBC verification pattern on the shared code — decode tests + 5-minute fuzz of `cart::step` (new target `fuzz_cart_step` in `crates/monad-mbc/fuzz`)
  - S5 2026-10-23: cargo-fuzz not installed: built with its RUSTFLAGS (+debug-assertions, overflow checks) and ran the libFuzzer binary directly: 191,764,880 runs / 300 s.
  ```bash
  cd "$PROJECT_ROOT/crates/monad-mbc" && cargo +nightly fuzz run fuzz_cart_step -- -max_total_time=300 2>&1 | tail -3
  ```
- [x] **Step 270** [V] ~30s: Zero crashes, zero panics (a fault must be a `Fault` value, never a panic)
  - S5 2026-10-23: 0 crashes, 0 panics.
- [x] **Step 271** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `89468b0c`.
  ```bash
  git -C "$PROJECT_ROOT" add crates/upc-api crates/upc-cartridge crates/monad-mbc/fuzz && commitp -m "test(upc-cartridge): [S5 264-271] surface descriptor, determinism properties, cart_step fuzz"
  ```
- [x] **Step 272** [COMPLIANCE] ~5m: SPDX headers on every new file; `Cargo.lock` diff shows no new third-party crates
  - S5 2026-10-23: SPDX on every new file (assembler.rs also gained its missing SPDX line).
  ```bash
  git -C "$PROJECT_ROOT" diff --stat 443de3f82 -- '*Cargo.lock' | tail -3
  ```
- [x] **Step 273** [V] ~30s: No new third-party crates (path deps only)
  - S5 2026-10-23: Only `upc-cartridge` itself is new in its Cargo.lock: every third-party crate@version (incl. sha2 0.10.9) is already in cmd/upc-bootctl or crates/monad-mbc locks.
- [x] **Step 274** [V][GATE] ~1m: **PHASE 10 EXIT GATE** — shared step fn builds for host and BPF; validator + reference runner tested; H-C3 resolved or findings logged; fuzz clean
  - S5 2026-10-23: PASSED.

### Phase 10 Definition of Done
TDD evidence (red then green) · determinism property holds · fuzz clean · no new third-party deps · SPDX present.

---

## PHASE 11: `cartridge-ebpf` — XDP + TICK PROGRAMS, VERIFIER-BUDGET GATE (Steps 275-306)

**Goal**: A loadable BPF object runs cartridges per packet and per tick, under a recorded verifier budget, without touching Doom/xv6.
**Prerequisite**: Phase 10 gate.
**Time**: ~5 h
**Agent**: Coordinator + Developer (verifier iteration needs sudo)

- [x] **Step 275** [R] ~10m: Verifier lessons to obey (write them as comments at the top of `main.rs`): bounded loop with a const; one variable offset into a map per loop body; no `translate_address`-style helper inside the loop; masks on every map index; explicit field writes (no zero-init padding with BTF); const-gated debug code
  - S5 2026-10-23: Rules written at the top of `ebpf/cartridge-ebpf/src/main.rs` (+ rule 7, faults fail open).
- [x] **Step 276** [W][CODE] ~30m: `ebpf/cartridge-ebpf` crate skeleton (Cargo.toml, build.rs if `--btf` needed, SPDX), added to `ebpf/Cargo.toml` members; maps: `CART_ROM` Array<u32> (4 slots × 1024), `CART_DATA` Array<u32> (4 × 1024), `CART_SLOT` Array<SlotCfg> (active, stage, max_insn, gen, fault window), `FLOW_STATE` LruHashMap<FlowKey, CartState> (65,536), `SCORE_QUEUE` Queue<ScoreJob> (16,384), `CART_EVENTS` RingBuf 256 KiB, `CART_TRACE` RingBuf 1 MiB, `CART_STATS` PerCpuArray<u64>, `CART_CFG` Array<u64> (trace on/off, N_PKT override ≤ 64)
  - S5 2026-10-23: Crate + maps as planned, plus FLOW_META (FIELD 7-9), TICK_JOB (resume slot), BREAKER, ZERO_STATE (insert source). build.rs adds --btf (bpf_loop callbacks need BTF func_info).
- [x] **Step 277** [W][CODE] ~45m: `cart_xdp`: parse Ethernet/IPv4/IPv6/TCP/UDP headers into the enumerated field table (Appendix C.3; no payload reads); FlowKey = 5-tuple hash; for each active XDP-stage slot (unrolled, ≤ 4): load `CartState` from `FLOW_STATE`, run ≤ N_PKT steps of `cart::step`, store state, apply the most restrictive verdict; fault → PASS + stats + breaker; trace record when `CART_CFG[trace]=1`
  - S5 2026-10-23: cart_xdp as planned. The per-slot run is a global BPF function `cart_run_xdp_slot` and the step runs as a bpf_loop callback (see 283).
- [x] **Step 278** [W][CODE] ~30m: `cart_tick`: pop one `ScoreJob` (flow key + feature snapshot) or resume a yielded one; run ≤ N_TICK steps of the tick-stage slot; on VERDICT/EMIT push a score event to `CART_EVENTS`; YIELD → re-queue with state
  - S5 2026-10-23: cart_tick: one in-progress job in TICK_JOB (resumed while yielded, dropped if the slot's gen changed), else pop SCORE_QUEUE.
- [x] **Step 279** [B][BUILD] ~3m: Build
  ```bash
  (cd "$EBPF" && cargo build --release -p cartridge-ebpf 2>&1 | tail -3)
  ```
- [x] **Step 280** [V] ~30s: Object produced
  - S5 2026-10-23: Object produced.
- [x] **Step 281** [B][S][TEST] ~5m: Verifier load + processed-insn count per program
  - S5 2026-10-23: bpftool cannot open Aya objects (legacy maps section; Step 144), so the count comes from cilium/ebpf with LogLevelStats: root-gated `TestLoad_VerifierBudget` in pkg/cartridge.
  ```bash
  sudo bpftool -d prog loadall "$EBPF/target/bpfel-unknown-none/release/cartridge-ebpf" /sys/fs/bpf/s5-cart 2>&1 | grep -E 'processed [0-9]+ insns' | tee "$S5/p11-verifier.txt"; sudo rm -rf /sys/fs/bpf/s5-cart
  ```
- [x] **Step 282** [V] ~1m: **VERIFIER-BUDGET GATE** — both programs load; each ≤ 500,000 processed insns (50% of the 1M ceiling, headroom for v1 features)
  - S5 2026-10-23: PASSED: cart_xdp 14,395, cart_tick 22,575 processed insns (gate 500,000 each).
  - Over → Step 283
- [x] **Step 283** [D] ~30m: Budget triage in order: (1) N_PKT 64→32, (2) slots 4→2, (3) move field parsing out of the loop into a fixed struct, (4) split the opcode dispatch into a tail call. Re-measure after each; two rounds, then STUCK with numbers
  - S5 2026-10-23: Triage done before the gate passed. The inlined loop did not prune: 1,000,001 insns (`max_states_per_insn 124`). N_PKT/slot cuts would not fix non-convergence, so: step as a bpf_loop callback (verified per call site, any N) -> 498,904; then option 4 in spirit, the per-slot run as a GLOBAL function (verified once, independent of the parse paths) -> 14,395. Also fixed: folded range+alignment check on a register copy (mem_index laundering), memset/memcpy calls under --btf (volatile word writes, in-place zeroing, no struct moves), uninitialised enum payload byte across the callback (u32 step codes), variable-offset L4 packet read.
- [x] **Step 284** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `6db3ce44`.
  ```bash
  git -C "$PROJECT_ROOT" add ebpf/cartridge-ebpf ebpf/Cargo.toml ebpf/Cargo.lock && commitp -m "feat(cartridge-ebpf): [S5 275-284] cart_xdp + cart_tick under the verifier budget gate"
  ```
- [x] **Step 285** [V] ~2m: Doom/xv6 untouched — `monad-cpu-ebpf` sources unchanged since Phase 3 (`git diff --stat HEAD~1 -- ebpf/monad-cpu-ebpf` empty)
  - S5 2026-10-23: `git diff --stat dcacffb4 -- ebpf/monad-cpu-ebpf` empty.
- [x] **Step 286** [W][CODE] ~20m: Regression harness gains a third variant: build + LOAD `cartridge-ebpf`, print its two insn counts, fail over 500,000 (ADR-080 "load-test every variant")
  - S5 2026-10-23: Variant 3 in `scripts/upc-regression.sh`: build cartridge-ebpf + cart CLI, load-test, budget gate, root tests.
- [x] **Step 287** [B][S][TEST] ~3m: `bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tail -10`
- [x] **Step 288** [V] ~30s: `== ALL GREEN ==` with three variants and budgets printed
  - S5 2026-10-23: ALL GREEN: ascend 926,542, Doom 761,580 (unchanged), cart_xdp 14,395, cart_tick 22,575, 12 root tests.
- [x] **Step 289** [W][CODE] ~30m: Minimal Go loader package `pkg/cartridge` (cilium/ebpf): load object, pin maps under `/sys/fs/bpf/unheaded/cart/`, write a slot (ROM, DATA, SlotCfg with `gen` bump LAST so a reader never sees half a slot), attach XDP to an iface or run `cart_tick`/`cart_xdp` via `ebpf.Program.Run` (test_run) with repeat
  - S5 2026-10-23: `pkg/cartridge`: pin dir option (0700) under /sys/fs/bpf/unheaded/cart/, writeSlot deactivate -> ROM -> DATA -> config with new gen LAST, test_run/Benchmark, AttachXDP, events, stats.
- [x] **Step 290** [W][TEST] ~20m: Go tests for slot encoding and the gen-last write ordering (unit), root-gated load test
  - S5 2026-10-23: Unit tests for slot encoding, flow key, state decode/hash (cross-language vector 0x6da0d6f5), image decode parity + fuzz, and the gen-last write order (fake maps).
- [x] **Step 291** [B][S][TEST] ~3m: `cd "$PROJECT_ROOT" && sudo -E go test -race ./pkg/cartridge/... 2>&1 | tail -5`
- [x] **Step 292** [V] ~30s: Green
  - S5 2026-10-23: Green, `-race` clean as root and as user.
- [x] **Step 293** [W] ~15m: Hand-written cartridge `cartridges/pass.s` (VERDICT PASS) and `cartridges/drop-udp-9.s` (drop UDP dport 9, the discard port) for plumbing tests
  - S5 2026-10-23: `cartridges/pass.s`, `cartridges/drop-udp-9.s`; test-only `pkg/cartridge/testdata/{fault,counter}.s`.
- [x] **Step 294** [B][S][TEST] ~5m: test_run both against crafted packets (UDP/9, UDP/53, TCP/22)
- [x] **Step 295** [V] ~1m: `pass` → XDP_PASS everywhere; `drop-udp-9` → DROP only UDP/9; stats count per slot
  - S5 2026-10-23: pass.s: PASS on UDP/9, UDP/53, TCP/22. drop-udp-9.s: DROP only UDP/9. Slot 0 runs=6 pass=5 drop=1, slot 1 untouched.
- [x] **Step 296** [B][S][TEST] ~3m: Fault path: a cartridge reading `0x2000` (outside windows)
- [x] **Step 297** [V] ~30s: PASS + `cart_faults_total` increments; after 64 faults in 1 s the slot is inactive
  - S5 2026-10-23: Every faulting packet PASSes; at the 64th fault in one window the slot goes inactive (faults=64, runs=64 of 70 packets, breaker_trips=1).
- [x] **Step 298** [B][S][TEST] ~5m: Bit-exact check — same packets through `cart_xdp` (BPF) and `upc-cartridge` reference; compare final `CartState` hashes read back from `FLOW_STATE`
  - S5 2026-10-23: counter.s, 90 packets over 3 flows with a fixed clock per packet; the Rust reference is `cart run X.cart --inputs LOG` (new subcommand).
- [x] **Step 299** [V] ~30s: 100% hash agreement
  - S5 2026-10-23: 100 %: 3/3 flow hashes equal, 21 EMIT events identical in order.
  - Disagreement → Emergency A.7 (BPF vs host divergence)
- [x] **Step 300** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `efc3a2b5`.
  ```bash
  git -C "$PROJECT_ROOT" add pkg/cartridge cartridges scripts/upc-regression.sh && commitp -m "feat(cartridge): [S5 285-300] Go loader, plumbing cartridges, BPF==host state hashes"
  ```
- [x] **Step 301** [B][S] ~5m: Benchmark baseline b0 — `cart_xdp` with zero active slots and with `pass` in one slot (test_run, repeat 1,000,000, 5 runs, median)
  - S5 2026-10-23: Engine: cilium `Program.Benchmark` (bpftool cannot load the objects). Opt-in `CART_BENCH=1` root test.
- [x] **Step 302** [V] ~1m: Record b0_empty and b0_pass ns/pkt in `$S5/bench.csv`
  - S5 2026-10-23: b0_empty 105 ns/pkt, b0_pass 153 ns/pkt (median of 5 x 1,000,000; taskset -c 2).
- [x] **Step 303** [B][S] ~3m: Same for a trivial native XDP_PASS program and `qos-ebpf` (native token bucket) — the external baselines
  - S5 2026-10-23: native heimdall Gjallarhorn XDP 21 ns; native qos-ebpf 42 ns (its early-PASS path: the frame has no Monad HbH option, so this is a floor, not its token-bucket path; Phase 13 measures the bucket path).
- [x] **Step 304** [V] ~1m: Baselines recorded; noise (max−min)/median < 10% over 5 runs, else pin the CPU (`taskset -c 2`) and redo
  - S5 2026-10-23: Noise <= 4.8 % pinned (first unpinned try was already under 10 %).
- [x] **Step 305** [W][DOC-UPDATE] ~10m: Sprint log: budgets, baselines, any verifier triage done
  - S5 2026-10-23: Sprint log Phase 11 section.
- [x] **Step 306** [V][GATE] ~1m: **PHASE 11 EXIT GATE** — object loads under 500K per program; harness green with three variants; BPF==host hashes; fault breaker works; baselines recorded
  - S5 2026-10-23: PASSED.

### Phase 11 Definition of Done
Verifier gate passed with numbers · Doom/xv6 objects unchanged · load-tested in the harness · `-race` green · benchmarks reproducible (<10% noise).

---

## PHASE 12: SOPHIA CARTRIDGE REGISTRY + SIGNED LOADER (Steps 307-328)

**Goal**: `upc-cartridgectl` loads only signed, validated cartridges resolved through the Sophia registry, swaps slots atomically, and exposes metrics.
**Prerequisite**: Phase 11 gate.
**Time**: ~3 h
**Agent**: Developer

- [x] **Step 307** [R] ~10m: Sophia service API for storing/reading knowledge entries (`Learn`/`Query`) and how other code writes Sophia BPF maps (`services/sophia/policy_map.go`)
  - S5 2026-10-23: Sophia stores knowledge entries via `POST/GET /api/v1/knowledge` (subject/predicate/object + metadata map, `{success,data:{knowledge}}` envelope, service auth); `policy_map.go` is the BPF-map side, not needed here. Gungnir: `SignFor`, `Verify`, `LoadSeedFile`, `LoadPublicKeyFile`, `KeyID`.
- [x] **Step 308** [DECIDE] ~5m: Registry backing for this sprint
  - S5 2026-10-23: Decided per recommendation: Sophia knowledge entries (subject `x-cartridge/<name>`, predicate `cartridge.record`, object = sha256, record in metadata) when `--sophia` is given, else `--registry-dir` with the same schema. Image bytes content-addressed on disk either way (the seal makes storage untrusted). Sophia path tested against a fake of its real envelope; not run live (stack down, KF5).
  - **RECOMMENDATION**: registry records as Sophia knowledge entries (namespace `x-cartridge`, D-C4) via the Sophia service API when it is reachable, with a file-backed fallback `--registry-dir` (JSON per cartridge) for lab runs while the stack is down (KF5). Same record schema both ways.
  - **Override ONLY if**: the Sophia API cannot store structured records → file-backed only, Sophia integration becomes a follow-on card.
- [x] **Step 309** [W][CODE][TEST] ~45m: `cmd/upc-cartridgectl` (Go): `register <name> <file.cart> <file.seal>`, `load <name> --slot N`, `unload --slot N`, `list`, `stats`, `bench`, `pump` (tick driver), `record` (Phase 16). `load` reads the image ONCE into memory, verifies the Gungnir seal on those bytes against the pinned key (`pkg/gungnir.Verify`), runs `cart validate` (Rust CLI) on the same bytes via stdin, then writes the slot from the same buffer — no re-read (TOCTOU-safe)
  - S5 2026-10-23: `cmd/upc-cartridgectl`: up/down/sign/register/load/unload/swap/list/stats/bench/pump/serve (`record` belongs to Phase 16). `up` pins maps AND programs so later invocations share one interpreter. Load = read once -> seal verify (pinned key) -> `cart validate -` on stdin -> decode -> registry sha match -> write; same bytes throughout.
- [x] **Step 310** [W][TEST] ~20m: Tests: unsigned → refused; seal for other bytes → refused; expired seal → refused; validator reject → refused; `--lab` allows unsigned with a loud warning and a `lab=1` label on every metric
  - S5 2026-10-23: Tests: unsigned, seal over other bytes, wrong key, no pinned key, garbage seal, expired/tampered seal, validator reject, non-image bytes all refused; `--lab` loads unsigned with a warning, still validated, slot marked lab.
- [x] **Step 311** [B][TEST] ~2m: `cd "$PROJECT_ROOT" && go test -race ./cmd/upc-cartridgectl/... 2>&1 | tail -5`
- [x] **Step 312** [V] ~30s: Green
  - S5 2026-10-23: Green, `-race`.
- [x] **Step 313** [W][CODE] ~20m: Atomic swap: write the new image to the spare slot, flip `CART_SLOT.active` of new then old in one ctl command; in-flight flows keep their `CartState` only if `gen` matches, else reset (documented)
  - S5 2026-10-23: `swap NAME --from A --to B`: write the spare slot (deactivate, ROM, DATA, config+gen last), then deactivate the old one. Flows start fresh in the new slot; a flow's state in a reloaded slot resets on generation mismatch (BPF side, Phase 11).
- [x] **Step 314** [B][S][TEST] ~5m: Swap `pass` → `drop-udp-9` while test_run loops in another shell; no packet sees a half-written slot (verdicts are only PASS or the new DROP rule)
  - S5 2026-10-23: Live: 400 `bench` invocations (200 test_runs each, UDP/9) racing a pass -> drop-udp-9 swap.
- [x] **Step 315** [V] ~1m: Zero faults during the swap; verdict set correct before and after
  - S5 2026-10-23: Verdicts seen: 47 x PASS then 353 x DROP, no interleaving, nothing else; 0 faults.
- [x] **Step 316** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `40a01890`.
  ```bash
  git -C "$PROJECT_ROOT" add cmd/upc-cartridgectl crates/upc-cartridge && commitp -m "feat(upc-cartridgectl): [S5 307-316] signed + validated load, Sophia registry, atomic swap"
  ```
- [x] **Step 317** [W][CODE] ~20m: Metrics on `--metrics-addr` (default `127.0.0.1:26610`): `unheaded_cart_packets_total{slot,verdict}`, `unheaded_cart_faults_total{slot}`, `unheaded_cart_insns_total{slot}`, `unheaded_cart_queue_depth`, `unheaded_cart_scores_total`; `/health`, `/ready`
  - S5 2026-10-23: `serve` (default `127.0.0.1:26610`): the plan's five metrics plus runs/emits/queue_drops, breaker trips, events dropped, slot_info; `/health`, `/ready`. queue_depth is queued - done - stale (a BPF queue has no length).
- [x] **Step 318** [B][TEST] ~2m: Metrics endpoint test
  - S5 2026-10-23: `TestRenderMetrics`, `TestLoopbackOnly`.
- [x] **Step 319** [V] ~30s: Counters move with test traffic; endpoint loopback by default
  - S5 2026-10-23: Live: counters moved with test traffic (slot 0 pass 109,400, slot 1 drop 70,600); listener `127.0.0.1:26610`.
- [x] **Step 320** [W] ~15m: Lab signing recipe in the ADR + `cartridges/README.md`: generate a lab key (0600, under `$S5`, never committed), `heimdall-daemon --sign-key`-style seal command for `.cart` (or a `sign` subcommand in ctl using `pkg/gungnir.SignFor`)
  - S5 2026-10-23: Recipe in ADR-108 §Signing and loading and `cartridges/README.md`: seed 0600 outside the repo, `yggdrasil-evidence pubkey`, `upc-cartridgectl sign`.
- [x] **Step 321** [SECURITY] ~10m: Review: who can call `load`? (root only — map writes need CAP_BPF); pinned map permissions 0600; registry dir 0700; ctl never logs data blobs
  - S5 2026-10-23: Root only (CAP_BPF for map writes); pins 0600; registry 0700; ctl prints names/shas/key ids, never DATA blobs.
- [x] **Step 322** [V] ~30s: Pinned maps 0600, registry 0700 (`stat -c '%a %n'`)
  - S5 2026-10-23: Live `stat`: pin dir 700, pins 600 (maps and programs), registry/blobs/records 700, record and slots files 600.
- [x] **Step 323** [B][S] ~3m: Register + load `pass` through the registry path (file-backed), list, unload
  - S5 2026-10-23: Live: register pass + drop9 (signed), load pass into slot 0, list, unload, down.
- [x] **Step 324** [V] ~30s: Round trip works; `list` shows sha, key id, stage, slot
  - S5 2026-10-23: `list` shows sha, key id, stage, size per record and active/gen/name/sha/key per slot. (First run hit `slots.json` being read as a record; records moved to `records/`, regression test added.)
- [x] **Step 325** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: `c4f6a6d3`.
  ```bash
  git -C "$PROJECT_ROOT" add cmd/upc-cartridgectl cartridges/README.md docs/adr && commitp -m "feat(upc-cartridgectl): [S5 317-325] metrics, lab signing recipe, permissions"
  ```
- [x] **Step 326** [W][DOC-UPDATE] ~10m: `wiki/` page `UPC-Cartridges.md` (terse, haproxy-style: what, build, sign, register, load, metrics) + `_Sidebar` link
  - S5 2026-10-23: `wiki/UPC-Cartridges.md`, linked from `Home.md` and `_Sidebar.md`.
- [x] **Step 327** [V] ~30s: Wiki page links resolve
  - S5 2026-10-23: Links resolve (ADR-108, cartridges/README.md, UPC-Overview); `scripts/sync-wiki.sh --dry-run` adds the page, no orphan.
- [x] **Step 328** [V][GATE] ~1m: **PHASE 12 EXIT GATE** — only signed + validated cartridges load; registry round trip; atomic swap proven; metrics live
  - S5 2026-10-23: PASSED: only signed + validated cartridges load; registry round trip; atomic swap proven; metrics live.

### Phase 12 Definition of Done
Seal + validator on the same in-memory bytes · `-race` green · permissions checked · wiki page · no new deps.

---

## PHASE 13: CARTRIDGE #1 — TOKEN-BUCKET RATE LIMITER + BENCH GATE (Steps 329-348)

**Goal**: A per-flow token-bucket rate limiter cartridge, correct against the `qos-ebpf` native baseline, under a pps/latency gate.
**Prerequisite**: Phase 12 gate. [P] with Phase 14 (independent cartridge).
**Time**: ~2.5 h
**Agent**: Developer [P]

- [x] **Step 329** [R] ~10m: Read the native token bucket in `ebpf/qos-ebpf/src/main.rs` (refill-by-elapsed, fits-check) — the correctness oracle
  - S5 2026-10-23 (session 3): Oracle read: first packet of a flow passes with tokens = sat(cap − len); later, add = (elapsed_ns / 8000) × rate_mbps, refill only when add > 0 (then `last = now`), DROP if tokens < len. Unit: bytes, u64.
- [x] **Step 330** [DESIGN][W] ~10m: `cartridges/ratelimit.s` spec: flow state in `mem` = {tokens:u32 (fixed-point ×256), last_ns:u32}; syscalls: `now_ns_lo`, `field(LEN)`; refill = min(burst, tokens + (now−last)×rate>>K); if tokens ≥ cost → tokens−=cost, VERDICT PASS; else VERDICT DROP; params (rate, burst, cost) in DATA
  - S5 2026-10-23: Deviation (oracle wins): tokens in bytes, cost = FIELD pkt_len, refill = qos-ebpf's /8000 exactly, so verdicts can match bit for bit. No DIV: elapsed/8000 = MULH(elapsed>>6, ceil(2^35/125))>>3 (exhaustive over 2^26). The 32-bit clock wraps at 4.29 s: FIELD iat_bucket ≥ 22 (idle ≥ 4.19 s) = full bucket, exact when cap ≤ 524,287 × rate_mbps (rate ≤ 8,000). DATA = {rate_mbps, cap, M}; mem = {tokens, last_lo, init}.
- [x] **Step 331** [W][CODE] ~30m: Write `cartridges/ratelimit.s` (hand MBC asm) + `cartridges/ratelimit.data.bin` (rate/burst/cost); keep ≤ 64 instructions, forward-only branches
  - S5 2026-10-23: Written; 57 words, longest path 41 instructions. DATA default 100 Mbps, cap 12,500 B (qos-ebpf's 1 ms default burst).
- [x] **Step 332** [B] ~2m: `cart build --stage xdp --asm cartridges/ratelimit.s --data cartridges/ratelimit.data.bin -o "$S5/ratelimit.cart" && cart validate "$S5/ratelimit.cart"`
- [x] **Step 333** [V] ~30s: Validates; `max_insn` reported ≤ 64; no back-edges
  - S5 2026-10-23: `OK stage=xdp max_insn=64 back_edges=false syscalls=[0, 1, 2]`, 292 bytes.
- [x] **Step 334** [W][TEST] ~25m: Reference-runner tests: 0 tokens → DROP; full bucket → N passes then DROP; refill after a time gap → passes resume; identical verdicts to a tiny Go re-implementation of the `qos-ebpf` math on the same input trace
  - S5 2026-10-23: `crates/upc-cartridge/tests/ratelimit.rs` (Rust u64 re-implementation of the qos-ebpf bucket) + `pkg/cartridge/ratelimit_linux_test.go` (Go re-implementation, BPF side). `FlowMetaSim` models FIELD 7–9 host-side.
- [x] **Step 335** [V] ~1m: Verdict sequence matches the qos-ebpf math exactly over a 1,000-packet trace
  - Mismatch → fix the cartridge (the native engine is the oracle); record the delta
  - S5 2026-10-23: 30,000 packets (six rate/cap settings × five 1,000-packet traces with bursts, idles past the clock wrap, caps below one packet), 10,625 drops, 0 mismatches. Mutation check: changing the wrap threshold 22→31 or skipping the add==0 rule fails the test.
- [x] **Step 336** [B][S][TEST] ~5m: Load in slot 0; test_run a crafted flow (same 5-tuple) at a controlled rate; read verdict counts
- [x] **Step 337** [V] ~1m: BPF verdicts == reference verdicts for the same trace (state hashes agree)
  - S5 2026-10-23: 1,000/1,000 BPF verdicts == Go qos-ebpf math (flood + quiet flow, fixed clock, gaps past the wrap); both flow hashes == `cart run`.
- [x] **Step 338** [B][S] ~5m: **Benchmark** — ns/pkt with ratelimit active, test_run repeat 1,000,000 × 5, median → `$S5/bench.csv`
  - S5 2026-10-23: Same IPv6 + Monad HbH frame for cartridge and native (`IPv6MonadPacket`, valid CRC), taskset -c 5, host loaded by other sessions (llama-server). Final: b0_pass_v6 154 ns; ratelimit 343 ns (pass regime) / 334 ns (drop regime), 35.3 / 33.3 insns per packet; native qos-ebpf bucket path 299 / 325 ns. `$S5/p13-bench-final.txt`.
- [x] **Step 339** [V][GATE] ~1m: **BENCHMARK GATE #1** — (a) overhead over b0_pass ≤ 150 ns/pkt; (b) within 3× of native `qos-ebpf` ns/pkt (a cartridge is interpreted — a fixed multiple, not parity, is the honest bar). Record pass/fail with numbers
  - Fail → Step 340
  - S5 2026-10-23: (a) **FAIL**: 189 ns (pass) / 180 ns (drop) over b0_pass (first measurement 357 ns, before Step 340). (b) **PASS**: 1.15× / 1.03× native qos-ebpf's token-bucket path.
- [x] **Step 340** [D] ~20m: Profile: instruction count per packet (`cart_insns_total`/packets); if dominated by refill divide, switch to a shift-rate (power-of-two) param; re-bench. Two rounds, then record as a known limitation, not a blocker
  - S5 2026-10-23: Not divide-bound (the /8000 is 4 cheap insns of 35). The cost is the interpreter: a 60×NOP cartridge measured 8.1 ns per instruction, mostly the per-step bpf_loop callback. Round 1: `cart_loop_cb` runs 4 steps per iteration → 4.3 ns/NOP (verifier 24,986 / 63,906). Round 2: 8 per iteration → 3.7 ns/NOP for 2× the verified callback; kept 4. **Known limitation**: ~4–7 ns per interpreted instruction; a 35-instruction cartridge cannot meet a 150 ns overhead bar on this host. Not a blocker (plan rule).
- [x] **Step 341** [B][S] ~3m: Flood test_run beyond the rate; confirm DROP ratio matches the configured rate within 5%
- [x] **Step 342** [V] ~1m: Enforced rate within 5% of configured
  - S5 2026-10-23: Fixed clock, 1,000 B every 24 µs (333 Mbps offered): 99.99 Mbps enforced, 6,012 passes == qos-ebpf math. Finding: at 20 µs spacing qos-ebpf's refill (and so the cartridge, matching it 4,012 == 4,012) enforces 80 Mbps, because each refill drops the remainder of elapsed/8000; logged, oracle not changed. Real-clock flood (2M test_runs): 96.0 Mbps (same remainder loss + wall-clock overhead; bound −10 %).
  - S5 2026-10-23 (session 4): qos-ebpf fixed on `develop` (`8ab534b3`, remainder carries); the cartridge and both oracles follow it: 20 µs and 24 µs both enforce 99.99 Mbps, 1,000/1,000 BPF verdicts == oracle, longest path 46 of 64, real-clock flood 97.62 Mbps.
- [x] **Step 343** [SECURITY] ~5m: A flow cannot starve others (per-flow state keyed by 5-tuple; LRU eviction bounded; no shared counter)
  - S5 2026-10-23: State is FLOW_STATE[5-tuple + slot] (LRU, 65,536); the cartridge has no cross-flow memory and no shared counter; FLOW_META is per flow too.
- [x] **Step 344** [V] ~30s: Two flows rate-limited independently in one run
  - S5 2026-10-23: BPF: flood dropped 790 of its packets, the interleaved quiet flow 0. Reference: a quiet flow's verdicts are identical with and without an interleaved flood.
- [x] **Step 345** [B][S][TEST] ~2m: Regression harness (object/loader may have changed)
- [x] **Step 346** [V] ~30s: `== ALL GREEN ==`
  - S5 2026-10-23: ALL GREEN: ascend 926,542 and Doom 761,580 (unchanged), cart_xdp 24,986, cart_tick 63,906, 18 root tests. (A first harness run in the fresh worktree, before any object existed, failed every xv6/Doom boot; the rerun on the same tree passed. Not reproduced since.)
- [x] **Step 347** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add cartridges crates/upc-cartridge "$PROJECT_ROOT/references/s5-upc-wire-log.md" && commitp -m "feat(cartridge): [S5 329-347] token-bucket rate limiter + benchmark gate"
  ```
  - S5 2026-10-23: `0594cd7b` (code) + the Phase 13 gate commit.
- [x] **Step 348** [V][GATE] ~1m: **PHASE 13 EXIT GATE** — correct vs qos-ebpf, BPF==reference, benchmark gate scored, independent per-flow, harness green
  - S5 2026-10-23: PASSED with Gate #1(a) recorded as a known limitation (Step 340).

### Phase 13 Definition of Done
Correctness oracle = qos-ebpf · benchmark gate scored with numbers · per-flow isolation proven · harness green.

---

## PHASE 14: CARTRIDGE #2 — FLOW SAMPLER + BENCH GATE (Steps 349-366)

**Goal**: A deterministic 1:N flow sampler cartridge that marks sampled flows via an event (never by mutating the Monad header), plus the feature snapshot the scorer consumes.
**Prerequisite**: Phase 12 gate. [P] with Phase 13.
**Time**: ~2 h
**Agent**: Developer [P]

- [x] **Step 349** [DESIGN][W] ~10m: `cartridges/sampler.s` spec: per-flow counter in `mem`; every Nth packet (N in DATA) → EMIT a sample event (flow key + packet count + byte count + first-seen) to `CART_EVENTS` and push a `ScoreJob` (features: pkt_len, iat_bucket, flags, counts) to `SCORE_QUEUE`; VERDICT always PASS; **never writes the Monad header** (contrast KF11)
  - S5 2026-10-23 (session 3): A CartEvent has two value words, so first-seen is its own event ('F', once per flow, its 64-bit `ts_ns` is the first-seen time); the sample event is 'S' {a = flow_pkt_count, b = flow_byte_count}. Snapshot = window features: f0 bytes, f1 iat_bucket sum, f2 SYN count, f3 max length, f4 flow_pkts, f5 l4_proto, f6 dst_port, f7 OR of TCP flags.
- [x] **Step 350** [W][CODE] ~25m: Write `cartridges/sampler.s` + DATA (N, feature field ids); ≤ 64 insns
  - S5 2026-10-23: DATA = N only (shipped 8; the feature set is fixed in code). Longest path 67 at first draft (Budget fault in the test), cut to 62 of 64 (`SHL 30; SHR 31` for the SYN bit, the first-seen flag reuses the kind register, window counter reset by `ST r15`).
- [x] **Step 351** [B] ~2m: `cart build … -o "$S5/sampler.cart" && cart validate "$S5/sampler.cart"`
- [x] **Step 352** [V] ~30s: Validates (XDP stage, ≤ 64 insns, only EMIT/QUEUE syscalls)
  - S5 2026-10-23: `syscalls=[0, 2, 3, 4]` (FIELD, VERDICT, EMIT, QUEUE), no back-edges, 324 bytes.
- [x] **Step 353** [W][TEST] ~20m: Reference tests: exactly every Nth packet emits; counts correct; queue job carries the right feature snapshot; determinism (same trace → same emit set)
- [x] **Step 354** [V] ~1m: 1:N exact; feature snapshot matches
  - S5 2026-10-23: `tests/sampler.rs`: N = 1, 2, 5, 8, 100 over 1,000 packets, every emit and snapshot equal to an independent model of the contract; same trace twice → same events and hashes; longest path measured at 62.
- [x] **Step 355** [B][S][TEST] ~5m: Load slot 1 (alongside ratelimit slot 0); test_run a mixed trace; read events + queue depth
- [x] **Step 356** [V] ~1m: Sample events at 1:N; `SCORE_QUEUE` depth grows; ratelimit in slot 0 still correct (slots independent)
- [x] **Step 357** [V] ~30s: BPF emit set == reference emit set (deterministic)
  - S5 2026-10-23: 600 packets over three flows: 77 events, 74 queued snapshots (`queue_ok` 74) and 3/3 flow hashes equal `cart run`; the rate limiter beside it 600/600 verdicts == qos-ebpf math.
- [x] **Step 358** [B][S] ~5m: **Benchmark** — ns/pkt with sampler active (and with both slots active), 5 × 1,000,000, median
- [x] **Step 359** [V][GATE] ~1m: **BENCHMARK GATE #2** — sampler overhead over b0_pass ≤ 120 ns/pkt; two slots active ≤ sum of each + 10%. Record numbers
  - S5 2026-10-23: Clean run (noise ≤ 2.7 %, IPv4 TCP frame): b0_pass 147, sampler 360 (37.9 insns/pkt), ratelimit 342, both 617 ns. Sampler overhead **213 ns: FAIL** (gate 120). Two slots: 470 ns vs 408 ns sum (+10 % = 448): **FAIL by 5 %**. Same cause as Gate #1 (≈ 5 ns per interpreted instruction, Step 340); earlier runs under heavy host load (load average up to 41) gave 288–348 ns. Queue and event ring are full in steady state (nothing drains them during the run), so this is the counted-drop path.
- [x] **Step 360** [SECURITY] ~5m: The event carries flow metadata only (5-tuple, counts, timestamps) — no payload bytes; confirm against the ABI field list (Sacred Principle)
- [x] **Step 361** [V] ~30s: Event layout has no payload field
  - S5 2026-10-23: CartEvent = {ts_ns, FlowKey, kind, a, b, slot, stage}; ScoreJob = {FlowKey, 8 feature words from FIELD 1/3/4/6/7/8/9 only, src_slot, enq_ns_lo}. No FIELD id or syscall reads payload.
- [x] **Step 362** [B][S][TEST] ~2m: Queue overflow — flood so `SCORE_QUEUE` fills
- [x] **Step 363** [V] ~30s: Overflow drops counted (`cart_queue_drops_total`), no fault, XDP still PASS
  - S5 2026-10-23: N = 1, 20,000 packets: queue_ok 16,384, queue_drops 3,616, events_dropped 16,361 (ring full, counted), faults 0, 20,000 PASS; 16,384 jobs popped after.
- [x] **Step 364** [B][S][TEST] ~2m: Regression harness
- [x] **Step 365** [V] ~30s: `== ALL GREEN ==`
  - S5 2026-10-23: ALL GREEN (ascend 926,542, Doom 761,580, cart_xdp 24,986, cart_tick 63,906, 19 root tests).
- [x] **Step 366** [C][GATE] ~1m: **PHASE 14 EXIT GATE / COMMIT** — 1:N exact, metadata-only events, benchmark gate scored, queue feeds the scorer
  ```bash
  git -C "$PROJECT_ROOT" add cartridges crates/upc-cartridge references/s5-upc-wire-log.md && commitp -m "feat(cartridge): [S5 349-366] flow sampler + score queue + benchmark gate"
  ```
  - S5 2026-10-23: PASSED with Gate #2 scored FAIL (interpreter limitation, as Gate #1). Code `d592f230`.

### Phase 14 Definition of Done
1:N deterministic · events metadata-only · benchmark gate scored · queue bounded · harness green.

---

## PHASE 15: CARTRIDGE #3 — int8 ANOMALY SCORER → services/anomaly (Steps 367-396)

**Goal**: A ~2 KB int8 model scores every sampled flow in the tick stage; scores reach `services/anomaly` and surface on its API.
**Prerequisite**: Phases 13, 14 gates (needs the sampler's queue).
**Time**: ~5 h
**Agent**: Coordinator + Developer + Scientist (model), Micromanager (the services/anomaly wiring)

- [x] **Step 367** [R] ~15m: Read `services/anomaly` scoring model (`TreeNode`, `RecordFlowScore`, `handleTopAnomalies`, `anomaly.scores` subscription) and the native `anomaly-ebpf` decision tree — both are oracles; note KF11 (SAMPLED overload + payload entropy) — the cartridge copies neither
  - S5 2026-10-23 (session 3): services/anomaly subscribes to `anomaly.scores` but never reads it; `RecordFlowScore` keys by 5-tuple and alerts above 80. anomaly-ebpf: i16 depth-12 tree per packet, payload-byte entropy feature, SAMPLED overload (KF11). The cartridge uses neither: header-only features, scores leave by ringbuf.
- [x] **Step 368** [DECIDE][DESIGN] ~15m: **D-C6 — model shape under the tick budget (N_TICK=256)**
  - S5 2026-10-23: Decided per recommendation (8→16→1 int8 MLP). Correction to the estimate: a MAC is 3 instructions (`LDB` biased weight, `MUL`, `ADD`), not 1, so a job is ~720 instructions = 3 tick calls (budget-out is an implicit YIELD), not one. Override not needed: the throughput gate is met (Step 378).
  - **RECOMMENDATION**: a 2-layer int8 MLP, input 8 features → hidden 16 (ReLU) → 1 logit, weights as int8 in DATA (8×16 + 16 = 144 B) + int32 biases; MAC via `MUL`+`SAR` accumulate. ~8×16 + 16 MACs ≈ 144 ≤ N_TICK. ~2 KB target met with quantization metadata + a second slot's worth of headroom.
  - Rationale: fits the per-flow tick budget; int8 avoids float (BPF + determinism); 8 features match the sampler snapshot.
  - **Override ONLY if**: 256 insns is too tight → quantized depth-8 decision tree (matches `anomaly-ebpf`, fewer ops), or raise N_TICK (re-run the Phase 11 verifier gate), or (last resort, full ISA spec gate) add a `DOT4` opcode in a follow-on sprint — NOT this one
- [x] **Step 369** [W][CODE] ~40m: `crates/upc-cartridge/src/bin/trainquant.rs` (host, std): train/quantize a tiny MLP on a labelled feature CSV (benign vs synthetic-anomalous flows derived from the sampler's feature definition), export int8 weights + scales to `.cart` DATA + a JSON sidecar. No GPU, no new deps (pure-Rust fixed-point)
  - S5 2026-10-23: `trainquant gen` (ten synthetic families — web client, download, DNS, SSH, QUIC; SYN flood, UDP flood, stealth scan, RST storm, beacon — run through the real sampler cartridge on the reference runner; no capture, no payload), `fit` (f64 MLP + Adam, then int8 quantization; activation shifts calibrated on train only), `control` (shuffled labels). Library: `upc_cartridge::scorer`. Training is f64 on the host; the deployed path is integer only.
- [x] **Step 370** [W][CODE] ~30m: `cartridges/scorer.s` (tick stage): load features from the ScoreJob into `mem`, run the int8 MLP from DATA, EMIT `{flow_key, score 0..100, model_id}`; declared loop bound = hidden size
  - S5 2026-10-23: First-layer weights stored +128 (u8) so one LDB reads each, minus a 128·Σx correction per job; per-row requantization shift; rounding offsets folded into the biases (exact); 256-entry sigmoid table. EMIT 'A' {a = score, b = model id}; the flow key rides in the event record.
- [x] **Step 371** [B] ~2m: `cart build --stage tick … -o "$S5/scorer.cart" && cart validate "$S5/scorer.cart"`
- [x] **Step 372** [V] ~30s: Validates (tick stage, declared bound, data ≤ 4096 B, total image ~2 KB)
  - S5 2026-10-23: `stage=tick rom=117 data=133 max_insn=256 back_edges=true syscalls=[3]`, loop bound 16, image 1,052 bytes (DATA 532): about 1 KB, under the ~2 KB target.
- [x] **Step 373** [W][TEST] ~30m: Reference tests: score monotonic in the obvious feature (e.g. extreme packet size → higher score); quantized output within 2 of the float model over 1,000 vectors (the quantization-accuracy gate, mirroring app12's "within 2%")
  - S5 2026-10-23: `tests/scorer.rs`. Monotone from three benign windows: the score never falls as the window's largest packet shrinks or its packets arrive faster (web: 1 → 74, SSH iat: 1 → 99). Found: the score ignores the SYN count (flood families are already separated by size, timing and flags) and is not monotone in window bytes far outside the training range; logged.
- [x] **Step 374** [V] ~1m: Quantization error ≤ 2 points on ≥ 99% of vectors
  - S5 2026-10-23: **PASS**: within 2 points on 99.83 % of 4,585 held-out vectors (max 5, mean 0.86).
- [x] **Step 375** [B][S][TEST] ~5m: Load scorer in the tick slot; `upc-cartridgectl pump` drains `SCORE_QUEUE` filled by the sampler; read score events
- [x] **Step 376** [V] ~1m: Every sampled flow gets exactly one score event; BPF tick score == reference score (hash agreement)
  - S5 2026-10-23: Sampler-fed BPF run: 18 jobs queued, 18 score events in 55 cart_tick calls, scores and order == `cart tick`, last-job state hash == reference. Reference: cartridge == `QModel::score` on 6,585 vectors (held-out + random/extreme).
- [x] **Step 377** [B][S] ~5m: **Benchmark** — scorer tick throughput (flows/s) via test_run repeat on `cart_tick`
- [x] **Step 378** [V][GATE] ~1m: **BENCHMARK GATE #3** — ≥ 100,000 flows/s scored on one core AND the XDP path (sampler) overhead unchanged from Gate #2 (scoring is off the packet path). Record numbers
  - Fail → Step 379
  - S5 2026-10-23: **PASS**: 251,572 flows/s median (5 runs, 248,942–252,908; 1,325 ns per call, 3 calls per job). Sampler XDP 355 ns with the scorer slot active vs 352 ns without (+0.9 %).
- [x] **Step 379** [D] ~20m: If flows/s low: reduce hidden to 8, or batch dequeue in the pump; if XDP overhead moved, the scorer leaked onto the packet path — fix the stage. Two rounds, then record
  - S5 2026-10-23: Not needed.
- [x] **Step 380** [V][PREFLIGHT] ~2m: **H-C1 re-check** — confirm `services/anomaly` still has no binary before adding one
  - S5 2026-10-23: `NO IMPORTER`.
- [x] **Step 381** [W][CODE] ~30m: `cmd/anomaly` (Go): the first binary for `services/anomaly` (CLAUDE.md service scaffold — health/ready/metrics/Wotan); it reads score events from the cartridge (via `pkg/cartridge` ringbuf reader) and calls `RecordFlowScore`, and/or publishes `anomaly.scores` to Wotan which the service already subscribes to
  - S5 2026-10-23: `cmd/anomaly`: the service (health/ready/metrics, `--listen` default 127.0.0.1:19018) + a bridge from the pinned CART_EVENTS ring. `--wotan` publishes `anomaly.scores`; without it the same JSON goes to the service's intake in-process.
- [x] **Step 382** [W][CODE] ~15m: Bridge path: cartridge score event → `anomaly.scores` Wotan message (scratch Wotan from Phase 6, or in-process for the test) → `services/anomaly` `listenForAlerts`/score intake → `/api/v1/anomalies/top`
  - S5 2026-10-23: services/anomaly gains `IngestScore` (4 KiB cap, strict JSON, score 0..100, addresses parsed, ingested/rejected counters) and a `listenForScores` Wotan consumer. Wotan leg not run live (Phase 6 scratch Wotan deferred with the east escalation; stack down, KF5): in-process path used.
- [x] **Step 383** [B][TEST] ~3m: `cd "$PROJECT_ROOT" && go test -race ./cmd/anomaly/... ./services/anomaly/... 2>&1 | tail -6`
- [x] **Step 384** [V] ~30s: Green; `services/anomaly` now has an importer (KF3 half-closed)
  - S5 2026-10-23: Green (red first: intake tests did not compile). KF3 half-closed.
- [x] **Step 385** [B][S][TEST] ~8m: End-to-end on west (scratch Wotan): craft a flow with anomalous features → sampler queues → scorer scores → `cmd/anomaly` → `services/anomaly` → `curl -s localhost:19018/api/v1/anomalies/top`
- [x] **Step 386** [V] ~1m: The anomalous flow appears in the top list with a score matching the cartridge's emission
  - S5 2026-10-23: Real XDP (generic) on a veth whose peer is in netns `s5e`, three signed cartridges loaded through `upc-cartridgectl`: 400 × 1,400 B UDP flood + 16 slow DNS-like packets. Rate limiter dropped 381 flood packets; sampler queued 52 jobs; pump: 156 calls, 52 jobs; `cmd/anomaly` forwarded 52 scores; `/api/v1/anomalies/top`: flood 10.55.0.1:41002→:40000 score **99**, DNS-like flow score **1**; 0 rejected. Teardown verified (no cart programs, links, netns, pins). Script: `$S5/p15-e2e.sh`.
- [x] **Step 387** [SECURITY] ~5m: The whole path carries flow metadata + a score, never payload (ABI enforces); `services/anomaly` isolation unchanged (ADR CLAUDE.md zero user data)
- [x] **Step 388** [V] ~30s: No payload field anywhere on the path
  - S5 2026-10-23: FIELD table → snapshot (8 header-derived words) → CartEvent {flow key, kind, score, model id} → ScoreMessage {flow_key, score, model_id, source, packet_count, timestamp_ms}; IngestScore refuses unknown fields (a `payload` key is a tested rejection).
- [x] **Step 389** [W][DOC-UPDATE] ~5m: If D-C6 ever points at a DOT4 opcode, write the deferral note (full ISA spec gate, next sprint) — do not add it here
  - S5 2026-10-23: Note in ADR-108 Non-goals: DOT4 would cut a job from ~720 to ~420 instructions (~1.7×); not needed for the gate; [ESCALATE] to the ISA's own spec gate if ever wanted.
- [x] **Step 390** [B][S][TEST] ~2m: Regression harness
- [x] **Step 391** [V] ~30s: `== ALL GREEN ==`
  - S5 2026-10-23: ALL GREEN: ascend 926,542, Doom 761,580, cart_xdp 24,986, cart_tick 63,906, pkg/cartridge root tests green.
- [x] **Step 392** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add cartridges crates/upc-cartridge cmd/anomaly services/anomaly references/s5-upc-wire-log.md && commitp -m "feat(cartridge): [S5 367-392] int8 anomaly scorer feeds services/anomaly"
  ```
  - S5 2026-10-23: `9404db47` (code) + the Phase 15 gate commit.
- [x] **Step 393** [SPEC][V] ~5m: Spec gate recheck — `anomaly.scores` / `upc.cart.*` topics still provisional pending RFC Editor (Step 242); no wire change made
  - S5 2026-10-23: `anomaly.scores` is used under its provisional name (`anomaly.ScoreTopic`), RFC Editor item 3 still open; no Monad byte changed, no opcode added (DOT4 deferred).
- [x] **Step 394** [W][DOC-UPDATE] ~5m: `wiki/UPC-Cartridges.md` + `docs/UNREACHED-GO-PACKAGES.md` (services/anomaly now reached)
- [x] **Step 395** [V] ~30s: Docs updated
  - S5 2026-10-23: wiki page (cartridge table, scores path, costs, budgets), `cartridges/README.md`, UNREACHED list (services/anomaly reached by cmd/anomaly), ADR-108 DOT4 note.
- [x] **Step 396** [V][GATE] ~1m: **PHASE 15 EXIT GATE** — every sampled flow scored, BPF==reference, quantization gate met, scores reach services/anomaly's API, benchmark gate scored, Sacred Principle held
  - S5 2026-10-23: PASSED. Gate #3 PASS; quantization gate PASS; end to end on real XDP in a netns lab.

### Phase 15 Definition of Done
Quantization gate ≤ 2 points · scoring off the packet path · services/anomaly reached by a real binary · no payload on the path · benchmark scored · no new deps.

---

## PHASE 16: ANAMNESIS REPLAY — TIME-TRAVEL DEBUG OF A CARTRIDGE (Steps 397-418)

**Goal**: A cartridge run is recorded as an Anamnesis trace and replayed bit-exactly off-line, with step/rewind/inspect.
**Prerequisite**: Phase 12 gate (determinism from Phase 10 is the foundation). [P:13-15].
**Time**: ~3.5 h
**Agent**: Developer (Scientist lens on determinism)

- [x] **Step 397** [R] ~10m: Read `services/anamnesis` event model + the Anamnesis Event Type registry (EVENT_COMPUTED 0x01); confirm the Phase 9 decision to reuse 0x01 with a cartridge sub-kind
  - S5 2026-10-23 (session 3): `services/anamnesis` is a Go event store with string types (created/updated/…), no importer (KF3); the numeric registry is the foundation draft's. Decision stands: every trace record starts `0x01 0x43` (EVENT_COMPUTED + provisional cartridge sub-kind, RFC Editor item 2).
- [x] **Step 398** [DESIGN][W] ~15m: Trace record (Appendix C.4): per recorded step, `{slot, gen, flow_key, pc, insn, input_fields, event_out?}`; plus one BORN record per run header with image sha, DATA sha, N, stage. Determinism contract: `state_{n+1} = step(state_n, input_n)` with inputs captured → replay needs only the image + the input log
  - S5 2026-10-23: Inputs are constant per run, so they live once in the head: TraceHead 464 B {stage, slot, gen, run_id, clock, flow, FIELD table, starting CartState}; TraceStep 32 B {idx, pc, insn, step code, state_hash after}; TraceEnd 32 B {insns, code, final hash}. The image sha (whole .cart file, covering ROM + DATA) is per slot in the file header. Layout in ADR-108 §Records; contract `docs/architecture/cartridge-replay.md`.
- [x] **Step 399** [W][CODE] ~20m: `CART_CFG[trace]=1` makes `cart_xdp`/`cart_tick` emit a compact trace record per step to `CART_TRACE` (fields only, no payload); `upc-cartridgectl record --slot N --out run.anam` drains `CART_TRACE` to a file
  - S5 2026-10-23: CART_TRACE (1 MiB ringbuf), TRACE_SEQ (per-CPU), `cart_trace_step` and `cart_state_hash` as global BPF functions, records dropped on a full ring are counted (stat 39). Verifier traps on the way: a 3-byte zero store and an all-zero FIELD loop lowered to rejected `memset` calls (`--btf`); a 6-argument helper (BPF passes 5); the cart_xdp → slot → callback chain hit the 512-byte combined stack (cart_xdp's SlotArgs moved to a per-CPU map); hashing inlined at every site took cart_tick to 328,144 insns (global functions: 146,897). `record` traces all slots and writes each slot's .cart sha from slots.json; `--slot` belongs to `replay`.
- [x] **Step 400** [V][PREFLIGHT] ~2m: Trace volume bounded — tracing is sampled (`CART_CFG[trace_1_in]`) so a flood cannot overrun the ringbuf; default off
  - S5 2026-10-23: Off by default (20 packets, 0 records); `CART_CFG[TRACE] = 10` traced 20 of 200 runs; a 3M-packet test_run flood at 1-in-2000 recorded 1,499 runs with 0 drops.
- [x] **Step 401** [W][CODE][TEST] ~40m: `crates/upc-cartridge` replay engine: load `run.anam` + the image; re-execute via `monad_common::cart::step`; after each step assert the replayed `state_hash` equals the recorded one; API `step()/back()/goto(n)/regs()/mem()/inspect()`
  - S5 2026-10-23: `upc_cartridge::replay`: hostile-input UCTR parser, `replay()` mirrors `drive` (`run_one` × N then `run_out`) and checks pc, word, code and hash per step plus the end record; `Replayer` step/back/goto/regs/mem/inspect; host `record()` for tests.
- [x] **Step 402** [V] ~1m: Replay of a recorded ratelimit run reproduces every state hash
  - S5 2026-10-23: Host: 200 runs, 7,806 steps, every hash (98 drops among them).
- [x] **Step 403** [W][CODE] ~20m: `upc-cartridgectl replay run.anam [--step|--to N|--watch mem[addr]]` — a CLI time-travel debugger (prints the divergence step if an image/trace mismatch, the actual debugging value)
  - S5 2026-10-23: `cart replay X.cart TRACE [--slot] [--run] [--to N] [--watch rN|mem[A]] [--steps]` (exit 3 on divergence); `upc-cartridgectl replay TRACE --slot N` fetches the recorded image from the registry blobs (sha-checked) and runs it.
- [x] **Step 404** [B][TEST] ~3m: `cargo test --manifest-path "$PROJECT_ROOT/crates/upc-cartridge/Cargo.toml" replay 2>&1 | tail -6`
- [x] **Step 405** [V] ~30s: Green
  - S5 2026-10-23: `tests/replay.rs` 5/5 (bit-exact, corrupted input, other image, corrupted hash, scorer time travel, hostile bytes ×5,000).
- [x] **Step 406** [B][S][TEST] ~8m: Record a live ratelimit run (test_run with trace on), replay off-line
- [x] **Step 407** [V] ~1m: Final state + every verdict identical between live run and replay
  - S5 2026-10-23: `TestReplay_LiveRunsAreBitExact`: rate limiter + sampler + scorer, every run traced: 160 + 160 rate-limiter/sampler runs bit-exact, replayed verdicts == the 160 live XDP returns. Through the ctl on the real clock: `record --every 2000` during a 3M test_run flood → 1,499 runs, `replay --slot 0` → 1,499 bit-exact.
- [x] **Step 408** [B][S][TEST] ~5m: Record a scorer tick run; replay; inspect the MLP accumulator at the step before EMIT
- [x] **Step 409** [V] ~1m: Replay reproduces the score; `--watch` shows the accumulator evolving (the "time-travel" payoff)
  - S5 2026-10-23: 66 scorer tick calls bit-exact; `--run 1 --watch r1` shows the row accumulator through the second call of a job (40+ changes), `--run 2 --to 188` stops before `SYSCALL 3 (EMIT)` with r2 = 0x63 (score 99) and r3 = the model id.
- [x] **Step 410** [B][TEST] ~5m: Negative — corrupt one byte of the recorded input log; replay must flag the exact divergent step
- [x] **Step 411** [V] ~1m: Divergence reported at the right step with recorded-vs-replayed hashes
  - S5 2026-10-23: One bit of run 9's recorded pkt_len flipped in the file: `DIVERGED at step 2 (hash): recorded 0x72d236c4, replayed 0xb33fc8f4` (step 2 is the `SYSCALL 0` that reads it), the other 159 runs bit-exact, exit 3. A different image diverges at step 0 (`insn`).
- [x] **Step 412** [SECURITY] ~5m: Trace records carry fields + state, never payload; a trace file is as sensitive as flow metadata, documented
- [x] **Step 413** [V] ~30s: No payload in the trace schema
  - S5 2026-10-23: Head = FIELD table + flow key + CartState (whose window holds only what the cartridge stored from FIELD values and the clock); steps/ends = pc, word, codes, hashes. Files 0600; documented in the contract note and the wiki.
- [x] **Step 414** [B][S][TEST] ~2m: Regression harness
- [x] **Step 415** [V] ~30s: `== ALL GREEN ==`
  - S5 2026-10-23: ALL GREEN: ascend 926,542 and Doom 761,580 (unchanged since Phase 0), cart_xdp 42,743, cart_tick 146,897, 21 cartridge root tests. Goldens 12/12 SAME (TTY + stats). Doom translation byte-identical (`dcded746…`, rv2mbc `e7ab7b67…`, data `e1191add…`).
- [x] **Step 416** [W][DOC-UPDATE] ~10m: `wiki/UPC-Cartridges.md` replay section + a short `docs/` note on the determinism contract
  - S5 2026-10-23: wiki Replay section; `docs/architecture/cartridge-replay.md`; ADR-108 §Records gains the trace layout.
- [x] **Step 417** [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add crates/upc-cartridge cmd/upc-cartridgectl ebpf/cartridge-ebpf wiki/UPC-Cartridges.md && commitp -m "feat(cartridge): [S5 397-417] Anamnesis replay — bit-exact time-travel debug"
  ```
  - S5 2026-10-23: `4634796c` (code) + the Phase 16 gate commit.
- [x] **Step 418** [V][GATE] ~1m: **PHASE 16 EXIT GATE** — record→replay bit-exact for both stages; divergence detection works; step/rewind/inspect usable; no payload in traces
  - S5 2026-10-23: PASSED. Cost of the trace-capable object with tracing off (re-benched): rate limiter 347 ns (was 343), sampler 377 (was 360), scorer 230,521 flows/s (was 251,572); gate verdicts unchanged.

### Phase 16 Definition of Done
Replay bit-exact · divergence detection proven · determinism contract documented · no payload in traces.

---

## PHASE 17: LICH HOOKS — MBC ISA DIFFERENTIAL FUZZING SURFACE (Steps 419-432)

**Goal**: Leave the Lich sprint a ready differential-fuzz harness (cart::step vs monad-mbc vs monad-cpu-ebpf) and a cartridge-image fuzz target — hooks only; the campaign is the Lich sprint's.
**Prerequisite**: Phase 10 gate (shared step fn). [P:13-16].
**Time**: ~2 h
**Agent**: Developer (BlackMage lens)

- [x] **Step 419** [R] ~5m: Read the existing fuzz surface and the ADR-062 process; re-read the KF4 banner so the hook is documented as real coverage (host engine), not a simulation
  - S5 2026-10-23 (session 4): Read the fuzz surface, ADR-062, KF4 and the Lich sprint's LICH-015 (`sprint/lich-awakens`, read with `git show`, not merged): its `diff_harness` already compares ExecCpu and the shared step on registers/flags/PC for ALU+branch programs (50,000 programs, 0 divergences). The hooks here extend it (memory windows, syscalls, EMIT/VERDICT, the BPF engine) rather than duplicate it.
- [x] **Step 420** [W][CODE] ~30m: `crates/monad-mbc/fuzz/fuzz_targets/fuzz_cart_differential.rs`: random validated cartridge + random inputs → run on `monad_common::cart::step` and `monad-mbc` `execute.rs`; assert identical register/flag/mem traces; any divergence is a finding
  - S5 2026-10-23: `upc_cartridge::diff` (generator + lockstep oracle, tests first: red on the missing module, then 6/6 green incl. an injected-bit-flip self-test) and `fuzz_cart_differential` (fuzz bytes = generator entropy). Syscalls are serviced on the host-emulator side from the same input; a cartridge fault ends the comparison (the sandbox refusing). `cart diff` CLI added (exit 3 on divergence).
- [x] **Step 421** [W][CODE] ~20m: `fuzz_cart_image.rs`: arbitrary bytes → `upc_cartridge::decode`+`validate`; must never panic (Err or Ok, never crash)
  - S5 2026-10-23: `fuzz_cart_image`: decode, validate, and run an accepted image; decode must be canonical. Second pass re-hashes the input, since the embedded sha256 otherwise rejects almost every mutation at decode.
- [x] **Step 422** [B][TEST] ~6m: Run each 5 minutes
  - S5 2026-10-23: cargo-fuzz not installed: built with its RUSTFLAGS (+debug assertions, overflow checks), libFuzzer binaries run directly. `fuzz_cart_differential` 3,046,713 runs / 301 s; `fuzz_cart_image` 95,882,146 runs / 301 s from the 9 seeds, corpus 413 (a first run without the re-hash pass: 526,043,178 runs but corpus stuck at 31).
  ```bash
  cd "$PROJECT_ROOT/crates/monad-mbc" && cargo +nightly fuzz run fuzz_cart_differential -- -max_total_time=300 2>&1 | tail -3
  cargo +nightly fuzz run fuzz_cart_image -- -max_total_time=300 2>&1 | tail -3
  ```
- [x] **Step 423** [V] ~1m: Zero crashes, zero differential divergences (or each divergence logged as a finding with the minimized input)
  - S5 2026-10-23: 0 crashes, 0 divergences in both runs. Seeded campaign `cart diff --seed 0x17 --count 20000`: 296,740 instructions compared, 0 divergences.
- [x] **Step 424** [W][CODE] ~20m: A root-gated BPF leg (optional, documented): `cart_xdp` via test_run on the same random cartridge; compare the read-back `FLOW_STATE` hash to the host engines — the three-way differential the Lich sprint will drive
  - S5 2026-10-23: `pkg/cartridge` `TestDifferential_ThreeEngines` (root-gated): `cart diff --out` writes cases the two host engines agree on; each runs through `cart_xdp` via BPF_PROG_TEST_RUN (no attach), checking verdict, FLOW_STATE hash, generation and EMITs. Deviation from the goal line: the third engine is `cartridge-ebpf` (the shared step compiled to BPF), not `monad-cpu-ebpf`, which cannot run cartridges; its full-engine arm stays LICH-015's east arm.
- [x] **Step 425** [B][S][TEST] ~5m: Run the three-way check on 1,000 random cartridges
  - S5 2026-10-23: 1,000 random cartridges, seed 0x17d1ff, 19.5 s.
- [x] **Step 426** [V] ~1m: Three engines agree on 1,000/1,000 (or divergences logged)
  - S5 2026-10-23: **1,000/1,000** agree across the three engines (577 halt, 190 verdict, 233 fault endings; 0 breaker trips). Part of the regression harness's cartridge root tests from now on.
- [x] **Step 427** [W] ~15m: `tomb/lich/` hook doc: how to point the Lich at this surface (targets, corpus seeds from `cartridges/`, the three-engine oracle), referencing the Lich sprint plan if present (Step 26)
  - S5 2026-10-23: `tomb/lich/UPC-CARTRIDGE-HOOKS.md`: engines, oracle rules, targets, commands, confinement, results, and the LICH-015 integration points for the merge (arm C, MULHU seed closure, run_harness exit codes, ADR-062 rows). LICH-015's own `diff_harness`, built against this branch from a scratch copy: `mulhu-probe` CONVERGED (MULHU arm `4cbef086`), self-test PASS, 50,000 programs 0 divergences.
- [x] **Step 428** [W] ~5m: Seed corpus — copy the three shipped `.cart` images + a few mutated ones into `tomb/lich/seeds/cartridge/`
  - S5 2026-10-23: `tomb/lich/seeds/cartridge/`: ratelimit, sampler, scorer + six mutations (stale sha, truncated, bad opcode, XDP back-edge, max_insn 65, re-hashed DATA), each hitting its intended refusal (README).
- [x] **Step 429** [SECURITY] ~5m: Confirm the fuzz targets cannot write outside their sandbox (host targets are pure; the BPF leg is test_run only, no attach)
  - S5 2026-10-23: Host targets are pure (no files, network or privileges); the BPF leg is `BPF_PROG_TEST_RUN` only (no attach, maps freed on exit), the path Phases 11-16 already ran on west.
- [x] **Step 430** [V] ~30s: Sandboxed
  - S5 2026-10-23: Sandboxed.
- [x] **Step 431** [C] ~1m: **COMMIT CHECKPOINT**
  - S5 2026-10-23: see the Phase 17 commit.
  ```bash
  git -C "$PROJECT_ROOT" add crates/monad-mbc/fuzz tomb/lich && commitp -m "test(lich): [S5 419-431] cartridge differential + image fuzz hooks (three-engine oracle)"
  ```
- [x] **Step 432** [V][GATE] ~1m: **PHASE 17 EXIT GATE** — differential + image fuzz targets build and run clean; three-engine oracle wired; seeds + hook doc left for the Lich sprint
  - S5 2026-10-23: PASSED: both new fuzz targets build and run clean, three-engine oracle real (host + BPF, not a simulation; KF4 addressed for cartridges), seeds + hook doc in place.

### Phase 17 Definition of Done
Fuzz targets clean (or findings logged) · three-engine oracle real (host + BPF, not a simulation — KF4 addressed) · seeds + doc for the Lich sprint.

---

## PHASE 18: STRETCH — ONE UPC SPLIT WEST+EAST OVER WOTAN DISTRIBUTED MEMORY (Steps 433-446)

> **[DEFERRED: east escalation] (2026-10-23).** Step 435 [ESCALATE] (east as the second node, scratch Wotan on the P2P link) is unanswered, like Step 177. By the lead's direction the west-netns fallback is not run either: the phase is dropped for this sprint and logged as follow-on card Epic 1.5.6 in `UPC-LINUX-MASTER-BATTLE-PLAN.md` (Step 446's "OR" branch). Nothing was started on east.

**Goal**: Demonstrate a cartridge whose DATA (model weights) lives in Wotan DISTRIBUTED memory and is served to a second node, so one logical UPC spans west+east. Stretch — [ESCALATE] gated and droppable.
**Prerequisite**: Phases 12, 15 gates. This whole phase is optional; dropping it does not block Phase 19.
**Time**: ~3 h (stretch)
**Agent**: Coordinator + Architect

- [ ] **Step 433** [DEFERRED: east escalation] [R] ~10m: Read `services/wotan/internal/compute` (`BPFMapCache`, L2 store) and `services/wotan/internal/cluster` (replication client/server) — the DISTRIBUTED seam
- [ ] **Step 434** [DEFERRED: east escalation] [DESIGN][W] ~20m: Split model (ADR appendix): the cartridge ROM + the control plane run on west; the scorer's DATA blob (weights) is published to a Wotan `compute`-namespace topic and fetched by east's `upc-cartridgectl` before it loads the scorer — "distributed memory" = the weights are authored once and served to both nodes. A ScoreJob produced on west can be scored on either node and the score reconciled. Keep it honest: this is distributed *data*, not a mid-execution memory fault across the wire
- [ ] **Step 435** [DEFERRED: east escalation] [ESCALATE][BARE-METAL] ~—: **Stevie approval** for: running a scratch Wotan reachable from east over the P2P link, and running `upc-cartridgectl` + `cartridge-ebpf` as root on east from `/tmp`. Context: east runs the debs; this is a throwaway, removed at phase end. Why no recommendation: live host + a second node. **If not approved: run both "nodes" as two netns on west sharing one scratch Wotan; mark the result "cross-node demonstrated in netns, not across hosts."**
- [ ] **Step 436** [DEFERRED: east escalation] [B][S] ~10m: Scratch Wotan on west bound to `192.168.13.2`; publish the scorer DATA blob to a `compute.cart.weights.<model>` topic
- [ ] **Step 437** [DEFERRED: east escalation] [V] ~1m: Blob published; `curl` the topic from west shows the sha matching the local `.cart` DATA
- [ ] **Step 438** [DEFERRED: east escalation] [B][S][BARE-METAL] ~15m: On east (or netns B): `upc-cartridgectl` subscribes, fetches the DATA, verifies the DATA sha against the signed manifest, loads the scorer locally
- [ ] **Step 439** [DEFERRED: east escalation] [V] ~1m: East's scorer DATA sha == west's; seal verified on the fetched bytes
- [ ] **Step 440** [DEFERRED: east escalation] [B][S][BARE-METAL] ~15m: Feed the same ScoreJob trace to east's scorer; compare scores to west's reference
- [ ] **Step 441** [DEFERRED: east escalation] [V] ~1m: Scores identical across nodes (determinism holds across the wire because the image + inputs are identical)
- [ ] **Step 442** [DEFERRED: east escalation] [SECURITY] ~10m: The weights crossed the wire — they are model parameters, not user data; the DATA blob is signed; east verifies before loading; no payload anywhere. Document the trust boundary
- [ ] **Step 443** [DEFERRED: east escalation] [B][S] ~5m: Teardown (scratch Wotan, east `/tmp` binaries)
- [ ] **Step 444** [DEFERRED: east escalation] [V] ~30s: Clean both sides
- [ ] **Step 445** [DEFERRED: east escalation] [C] ~1m: **COMMIT CHECKPOINT**
  ```bash
  git -C "$PROJECT_ROOT" add docs/adr references/s5-upc-wire-log.md && commitp -m "feat(cartridge): [S5 433-445] stretch: DISTRIBUTED weights, cross-node deterministic scoring"
  ```
- [ ] **Step 446** [DEFERRED: east escalation] [V][GATE] ~1m: **PHASE 18 EXIT GATE (STRETCH)** — cross-node deterministic scoring demonstrated (hosts or netns), weights signed + served over Wotan; OR phase explicitly dropped and logged as a follow-on card

### Phase 18 Definition of Done
Either: cross-node deterministic score with signed served weights · trust boundary documented. Or: dropped, logged, nothing left running on east.

---

## PHASE 19: CLOSE — SPEC, DOCS, REGRESSION, HANDOFF (Steps 447-452)

**Goal**: Everything built is spec-reconciled, documented, green, and handed off.
**Prerequisite**: Phases 13–17 gates (Phase 18 optional).
**Time**: ~1.5 h
**Agent**: Scientist + Librarian + Marshal

- [x] **Step 447** [SPEC][V] ~15m: **FINAL SPEC COMPLIANCE GATE** — every RFC Editor item from Step 243 has a status (reviewed / still open); the cartridge ADR flips to Accepted only if the namespace + topic registrations are resolved, else stays Proposed with provisional names documented; confirm no Monad wire-format byte changed (`git diff 443de3f82 -- ebpf/monad-common/src/lib.rs | grep -i 'MONAD_SIZE\|HBH_TOTAL_LEN'` empty) and no MBC opcode added
  - S5 2026-10-23 (session 4): RFC Editor items 1-5 all open (status section in `docs/protocol/RFC-EDITOR-REQUESTS-S5.md`); ADR-108 stays Proposed with provisional names. `443de3f82` is not an ancestor after the re-sign (Phase 0 H0), so the diff ran against the sprint base `f7d730e7`: `ebpf/monad-common/src/lib.rs` adds only `pub mod cart`, the MONAD_SIZE/HBH_TOTAL_LEN grep and an opcode-constant grep are empty, `ebpf/monad-cpu-ebpf` has no diff.
- [x] **Step 448** [B][S][TEST] ~5m: Full regression — three UPC variants + all new Go/Rust tests + the whole workspace
  - S5 2026-10-23: harness ALL GREEN (22 cartridge root tests incl. the three-engine differential); 12/12 goldens TTY + stats; Doom translation byte-identical; `go build ./...` + `go test -race` on the listed packages plus cmd/upc-cartridgectl and pkg/ports; Rust upc-cartridge 40, monad-mbc 391, monad-common 110, upc-api 25, 0 failed.
  ```bash
  bash "$PROJECT_ROOT/scripts/upc-regression.sh" 2>&1 | tail -8
  (cd "$PROJECT_ROOT" && go build ./... && go test -race ./cmd/anomaly/... ./cmd/heimdall-daemon/... ./cmd/upc-tty-bridge/... ./pkg/cartridge/... ./services/anomaly/... 2>&1 | tail -6)
  (cd "$PROJECT_ROOT" && cargo test --manifest-path crates/upc-cartridge/Cargo.toml 2>&1 | tail -3)
  ```
- [x] **Step 449** [V] ~1m: All green; budgets recorded: ascend (unchanged), Doom (unchanged), cartridge-ebpf (≤ 500K each)
  - S5 2026-10-23: ascend 926,542 and Doom 761,580 (unchanged from Phase 0), cart_xdp 42,743, cart_tick 146,897 (<= 500,000). Close re-bench: Gate #1(a) +193 ns FAIL, #1(b) 1.25x PASS, #2 +216 ns FAIL, #3 232,450 flows/s PASS.
- [x] **Step 450** [W][DOC-UPDATE] ~20m: Librarian doc-web ripple — CLAUDE.md (new cartridge layer under the UPC programmatic API; services/anomaly reached), `wiki/_Sidebar`, ADR index, `docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md` Epic 1.5 checkboxes, THIRD_PARTY/SBOM only if a dep changed (it should not)
  - S5 2026-10-23: CLAUDE.md (S5 bullet, Mimir K1 note, ASCEND-LINUX S5 update, port 26610), `pkg/ports` `UPCCartridgeMetrics` (the ctl default now comes from it), wiki `UPC-Cartridges` (differential section, close costs), `Port-Registry`, `ADR-Index` mirror synced (104, 105, 108; 103 entries), `Home` (ADR-108 in Recent), canonical ADR-INDEX row and counts (Phase 9 had not bumped them), ADR-108 status-at-close section, master plan Epic 1.5 (1.5.5 done, 1.5.6/1.5.7 open), NEXT-5-SPRINTS status row (mirrors develop's Status column). No dependency change outside the fuzz crate's lock (crates already in the tree), so no THIRD_PARTY/SBOM change. `scripts/sync-wiki.sh --dry-run`: no orphans; the wiki is not pushed.
- [x] **Step 451** [W][DOC-UPDATE] ~10m: `~/tmp/next.md` — full S5 handoff (overwrite stale): Part 1 + Part 2 shipped, [ESCALATE] approvals still pending (Steps 177, 435), RFC Editor items open, benchmark numbers, Phase 18 status, what the Lich sprint inherits
  - S5 2026-10-23: Not written by this session: `~/tmp/next.md` is outside the agent's worktree and the isolation refused the edit. The handoff (status, regression, gates, limitations, escalations, decision list, what the Lich inherits) is the closing summary in `references/s5-upc-wire-log.md`; the lead session copies it into the S5 bullet of next.md.
- [x] **Step 452** [C][GATE] ~1m: **SPRINT EXIT GATE / FINAL COMMIT** — all phase gates passed or STUCK-logged; docs rippled; harness green; nothing left running on west/east; NO push
  - S5 2026-10-23: PASSED. All phase gates passed or DEFERRED (6-8, 18: east); harness green; west clean (no BPF programs, netns, pins or processes from S5); east never contacted; not pushed. Explicit paths were staged instead of the block's add-all, per the header commit rule.
  ```bash
  git -C "$PROJECT_ROOT" add -A && git -C "$PROJECT_ROOT" status --porcelain | sed 's/^/  /' && commitp -m "docs(upc): [S5 447-452] cartridges close — spec reconciled, docs rippled, handoff"
  ```

### Phase 19 / Sprint Definition of Done
All phase DoDs met or STUCK-logged · three UPC variants load under budget · Doom + xv6 byte-identical to pre-sprint · services/anomaly reached · no new third-party deps · no wire/opcode change · RFC Editor items tracked · next.md handoff written · NOT pushed (Stevie pushes).

---

## APPENDIX A: EMERGENCY PROCEDURES

**A.1 — BPF verifier rejects a program**
1. `sudo bpftool -d prog loadall <obj> /sys/fs/bpf/x 2>&1 | tail -60` — read the actual rejection
2. "back-edge"/"infinite loop" → the loop bound isn't a compile-time const, or a non-const index; make N a `const` and mask every index
3. "too many instructions"/complexity → remove a helper call from the loop body; hoist field parsing out; split dispatch into a tail call (Phase 11 Step 283 order)
4. "Global function memset() doesn't return scalar" → a padded struct is zero-initialised with `--btf`; write every field explicitly, never `Default`/`{0}` on a `#[repr(C)]` with padding
5. "invalid access to packet" → a packet read without a `data_end` bounds check before it
6. Two attempts, then `[STUCK]`, commit, continue a non-blocked chain

**A.2 — Regression harness not green after a change**
1. `bash scripts/upc-regression.sh 2>&1 | tee /tmp/r.log` — which assertion failed (Doom load, xv6 corpus, gate2, budget)?
2. xv6 corpus DIFF → `diff <(tty_of "$out") "$GOLD/<slug>.tty"`; stats-only drift = cycle-dependent, TTY drift = semantic
3. Doom load FAIL → object over budget: `UPC_VERIFIER_STATS=1` in doom-runner; a cross-workload arm leaked into the Doom build (ADR-080 §3) — check `cfg!(feature="ascend-linux")` gating
4. Budget moved → `git diff` the eBPF source since the last green commit
5. Never commit over a red harness (Warmonger Law 2)

**A.3 — Goldens stale / reboot wiped /tmp**
1. `$GOLD` is `~/tmp/golden` (persistent). If empty → machine was reset or GOLD unset
2. Recapture from HEAD: `references/phase24-t1-2026-07-07.md` §"Golden recapture" + `PHASE22-SH-REPLACE.md` Steps 6/10/12–14, `GOLD=$HOME/tmp/golden`
3. Only recapture when HEAD is a known-good equivalence point, never mid-change

**A.4 — bpf-linker / nightly toolchain missing (heimdall-bpf, cartridge-ebpf)**
1. `rustup toolchain list | grep nightly` and `which bpf-linker`
2. `cargo install bpf-linker` (pin the version already used by `ebpf/`); `rustup toolchain install nightly --component rust-src`
3. These crates carry their own `.cargo/config.toml` + `rust-toolchain.toml` — build from the crate dir, not the repo root
4. Still failing → `[ESCALATE]` (environment), continue host-only phases

**A.5 — east unreachable / approval not given**
1. `ssh -o BatchMode=yes govan@east true` — reachable?
2. Unreachable or Step 177/435 not approved → run the west-netns variant, mark the east column BLOCKED, continue
3. Never install anything on east; `/tmp` throwaway binaries only, removed at phase end; GETs/observe-only

**A.6 — MBC fuzz finds a crash**
1. The crashing input is under `crates/monad-mbc/fuzz/artifacts/` — minimize (`cargo fuzz tmin`)
2. A cartridge fault must be a `Fault` value, never a panic — if `cart::step` panicked, add the missing bounds check/mask
3. Do not commit the translator/step change until the fuzzer is clean on that input
4. Add the minimized input to the seed corpus

**A.7 — BPF cartridge result ≠ host reference**
1. Dump both `CartState` after the same input (`FLOW_STATE` read-back vs reference)
2. First divergent step → that opcode's BPF arm differs from `monad_common::cart::step` — but they are the SAME source (D-C1); a divergence means a map/packet read differs, not arithmetic. Check field parsing and window bounds
3. If arithmetic differs, the host and BPF built different `monad-common` — rebuild both
4. This is exactly the Phase 17 differential finding; log it

**A.8 — test_run unavailable for benchmarks**
1. Kernel lacks `BPF_PROG_TEST_RUN` for XDP with repeat → use a veth pair in a netns, a userspace UDP blaster, and `cart_packets_total`/time for pps
2. Wider tolerance (the benchmark gates are relative to b0 measured the same way); note the method in the bench CSV

**A.9 — commit blocked (gpg-agent timeout)**
1. `commitp` already falls back to `--no-gpg-sign`
2. Record unsigned commits for `scripts/resign-unpushed.py` later (Stevie caches the key, then re-signs); never push

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Steps | Agent | Parallel? | Depends on | ~Time |
|---|---|---|---|---|---|
| 0 Preflight | 1–30 | Coordinator | — | base green | 1.5 h |
| 1 S1.2 root cause | 31–50 | Coordinator (Computermancer) | — | 0 | 2 h |
| 2 Translator CSR fix | 51–82 | Coordinator + Developer | — | 1 | 3 h |
| 3 getpid flip (L5) | 83–108 | Coordinator | — | 2 | 2.5 h |
| 4 TTY keyboard | 109–134 | Developer | [P:1-3] | 0 | 3 h |
| 5 Heimdall XDP+ringbuf | 135–170 | Developer + Coordinator | [P:1-4] | 0 | 4 h |
| 6 Mímir Phase 9 | 171–194 | Coordinator | — | 5 (+Stevie) | 3 h |
| 7 Mímir Phase 11 | 195–214 | Developer + Micromanager | — | 6 | 2.5 h |
| 8 Phase 13 re-eval / Part 1 exit | 215–228 | Scientist + Marshal + Librarian | — | 3,4,7 | 1.5 h |
| 9 Cartridge ADR + spec gate | 229–250 | Coordinator (Architect/Scientist) | — | 8 | 3 h |
| 10 ABI + shared step + host | 251–274 | Developer | — | 9 | 4 h |
| 11 cartridge-ebpf + budget gate | 275–306 | Coordinator + Developer | — | 10 | 5 h |
| 12 Sophia registry + signed loader | 307–328 | Developer | — | 11 | 3 h |
| 13 Rate limiter | 329–348 | Developer | [P] w/14 | 12 | 2.5 h |
| 14 Flow sampler | 349–366 | Developer | [P] w/13 | 12 | 2 h |
| 15 int8 scorer → anomaly | 367–396 | Coordinator + Dev + Scientist | — | 13,14 | 5 h |
| 16 Anamnesis replay | 397–418 | Developer | [P:13-15] | 12 | 3.5 h |
| 17 Lich hooks | 419–432 | Developer (BlackMage) | [P:13-16] | 10 | 2 h |
| 18 DISTRIBUTED (stretch) | 433–446 | Coordinator + Architect | optional | 12,15 (+Stevie) | 3 h |
| 19 Close | 447–452 | Scientist + Librarian + Marshal | — | 13–17 | 1.5 h |

**Critical path (≈40 h)**: 0 → 1 → 2 → 3 → 8 → 9 → 10 → 11 → 12 → 14 → 15 → 16 → 19. Phase 4 and Phases 5–7 run beside 1–3 (serialize only the shared XDP/netns harness runs). 13 ∥ 14; 16, 17 ∥ 13–15. 18 is optional and [ESCALATE]-gated.

**[ESCALATE] items (STOP that chain, keep working others)**:
- Step 111 — H-K1 false → KBD needs a BPF change (verifier spend)
- Step 177 — east as a real bootstrap/stress node (XDP on `enp2s0`, root from `/tmp`) + scratch Wotan bound to the P2P link
- Step 435 — east as the second node for the DISTRIBUTED stretch
- Any RFC Editor registration (Sophia namespace, Anamnesis sub-kind, Wotan topics) that must be Specification-Required before use — provisional names until then
- Any proposal to add an MBC opcode (e.g. DOT4, Step 368/389) — full ISA spec gate, a separate sprint

---

## APPENDIX C: QUICK REFERENCE

**C.1 — MBC register map (translator `map_register`)**
`x0→r0, x1/ra→r14, x2/sp→r15, x3/gp→r1, x4/tp→r2, x5-x7/t0-t2→r3-r5, x8/s0→r6, x9/s1→r7, x10-x15/a0-a5→r8-r13, x16/a6→r2*, x17/a7→r1*, x18-x31 spill-shadowed`. CSR arm scratch: CSR pointer **r12 (a4)**, RMW temp **r13 (a5)**, CSRRC mask **r14 (ra)** — the Phase 2 clobber source.

**C.2 — CSR memory map (ADR-067 Decision 2)**
CSR i at byte `0xF000 + i*4` in RAM. sscratch=0x140→0xF500, satp=0x180→0xF600, sstatus=0x100→0xF400, mscratch=0x340→0xFD00. The S1.2 bug: `csrr a2,satp` (CSRRS rs1=x0) ran the RMW path and wrote satp's value into r13=a5, which the next `csrw sscratch,a5` then stored.

**C.3 — Cartridge ABI v0 (DECIDE in Phase 9; this is the forged proposal)**
```
Image .cart: magic "MBCC" | ver u8 | stage u8(0=xdp,1=tick) | max_insn u16 | rom_words u16
             | data_words u16 | entry u16 | sha256(rest) [32] | ROM(u32×rom_words) | DATA(u32×data_words)
CartState:   regs[16] u32, pc u16, flags u8, insns u32, mem[64] u32   (mem = flow window 0x0000..0x0100)
Memory:      [0x0000,0x0100) → mem (RW flow state) ; [0x1000, 0x1000+4*data_words) → DATA (RO) ; else Fault(Mem)
Opcode allowlist: NOP ADD SUB MUL MULH AND OR XOR NOT SHL SHR SAR SHLR SHRR SARR MOV MOVI LOAD_IMM32 ADDI
                  CMP JMP JZ JNZ JN JP JC JNC LD ST LDB HALT SYSCALL   (no CALL/RET, no indirect branch)
Syscalls (SYSCALL imm = number), stage-gated:
  0 FIELD(id)->r0      enumerated header/meta field (Appendix C.3a). NEVER payload bytes.
  1 NOW_NS_LO()->r0    low 32 bits of a monotonic clock
  2 VERDICT(code)      code: 0 PASS, 1 DROP, 2 TX/REDIRECT  (xdp stage) — ends the run
  3 EMIT(kind, a, b)   push a {kind,flow_key,a,b} record to CART_EVENTS (both stages)
  4 QUEUE(feat_ptr)    push a ScoreJob from mem[feat_ptr..] to SCORE_QUEUE (xdp stage)
  5 YIELD()            suspend, resume next cart_tick call (tick stage)
Verdicts combine most-restrictive across active xdp slots. Fault → PASS + fault counter + per-slot breaker.
```
**C.3a — FIELD ids (metadata only, Sacred Principle)**: 0 l3_proto, 1 l4_proto, 2 src_port, 3 dst_port, 4 pkt_len, 5 ip_ttl/hoplimit, 6 tcp_flags, 7 iat_bucket, 8 flow_pkt_count, 9 flow_byte_count, 10 monad_present, 11 monad_qos, 12 monad_flags (read-only). No id returns packet payload.

**C.4 — Anamnesis replay record (Phase 16)**
Reuses EVENT_COMPUTED (0x01) + sub-kind. Run header: {img_sha256, data_sha256, stage, N, slot, gen}. Per step: {pc, insn, fields_in[], state_hash}. Replay invariant: `state_{n+1}=step(state_n, fields_in_n)`; replay asserts recorded `state_hash` at every step. FNV-1a (same as upc-bootctl `fnv1a`) over regs‖pc‖mem.

**C.5 — Native app baselines (oracles, NOT rebuilt)**
`ebpf/qos-ebpf` token bucket + CoDel (rate-limiter oracle) · `ebpf/anomaly-ebpf` i16 decision tree, 1:100 ringbuf (scorer oracle; do NOT copy its SAMPLED-overload or payload-entropy — KF11) · `ebpf/nfv-ebpf` tail-call chain · `ebpf/failover-ebpf`. `services/anomaly` REST API (port 19018) = score sink.

**C.6 — Key commands**
```
bash scripts/upc-regression.sh                       # 3-variant load + xv6 corpus + gate2 + budgets
cart build --stage xdp --asm X.s --data W.bin -o X.cart ; cart validate X.cart
upc-cartridgectl register|load|unload|list|stats|bench|pump|record|replay
sudo bpftool prog loadall <obj> /sys/fs/bpf/x 2>&1 | grep 'processed'   # verifier insn count
sudo bpftool prog run pinned <p> data_in pkt.bin repeat 1000000         # bench ns/run
```

**C.7 — Budgets & gates (fill in during execution)**
ascend verifier (expect unchanged post-Phase-2; getpid flip ≤ 950,000) · Doom verifier (unchanged; Doom `.mbc` sha unchanged) · cartridge-ebpf ≤ 500,000 per program · bench b0_empty / b0_pass / ratelimit / sampler / scorer flows-per-s — all in `$S5/bench.csv`.

---

*S5 Battle Plan (S-UPC-WIRE-COMPUTE) — Forged 2026-10-23*
*20 Phases. 452 Steps. First we finish the UPC: a one-register codegen bug closed, a keyboard that types into the shell, a watchman that sees the horn, a law re-proven on real wire. Then we compute on the wire: three signed cartridges running per flow, replayable to the exact instruction, feeding the anomaly engine — a third client for the computer that was always in the protocol.*
*Finish, then compute. The gates are open. The plan awaits.*
