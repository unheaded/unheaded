# Phase 1.1 SHIP Battle Plan — upc-bootctl Live BPF Integration

**Date**: 2026-05-10
**Sprint**: Phase 1.1 SHIP — first xv6 boot in browser xterm
**Codename**: ASCEND-LINUX P1.1-GATE
**Author**: Warmonger (forged from Round Table 2026-05-10)
**Parent plan**: `references/battle-plan-ascend-linux-2026-05-08.md` (this plan executes §2 row "Phase 1 — L5 xv6-on-MBC", first deliverable)
**Predecessors absorbed**: ADR-067 (frozen ABI/ISA), `docs/doom/UPC_BOOT_PROTOCOL_V2.md` (frozen), `crates/doom-runner/` (proven pattern), `crates/xv6-mbc/TRANSLATOR_EXTENSIONS.md` (open issues).

---

## Header

**Prerequisite**: 102 commits pushed (e7adc7c0..ca0b1775); baseline pristine; xv6-mbc.mbc emits 11,721 instructions / 46KB; kernel.elf links green.
**Target**: `cargo run -p upc-bootctl -- boot --kernel xv6-mbc.mbc --instance 222` populates BPF maps live, the eBPF interpreter executes the loaded kernel, "xv6 booting..." propagates via TTY_MAP through upc-tty-bridge to browser xterm.js, then the instance HALTs cleanly with no stale CPU_MAP / SCREEN_MAP / KBD_MAP entries.
**Estimated Duration**: 3-4 working days (8 phases, ~100 numbered steps).
**Agent Strategy**: Solo-Coordinator with Marshal enforcement at phase gates. No parallelism — every phase depends on previous phase's BPF map state.
**Commit Cadence**: Every 4 steps (max(3, min(5, 100/20)) = 4).
**Stuck Protocol**: Skip after 3× time estimate or 2 failed debug attempts. STUCK markers committed before skip per protocol.
**Owners**: Developer (lead, all `[CODE]` and `[B]`), Computermancer (ABI compliance reviewer at `[V]` gates touching MbcCpuState / BootParams / boot dispatch), Marshal (phase-gate enforcer + commit cadence + drift detection).

---

## Legend

```
[B]            Bash command (run directly)
[V]            Verification step (MUST pass before proceeding)
[D]            Debug step (only if prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo/elevated privileges required
[SEQ]          Sequential — do NOT parallelize
[C]            Commit checkpoint
[CODE]         Code implementation
[DESIGN]       Design decision
[DECIDE]       Decision point WITH pre-seeded recommendation (proceed autonomously)
[ESCALATE]     Requires human input — STOP
[PREFLIGHT]    Hypothesis verification (Phase 0 only)
[BUILD]        Build/compilation step
[TEST]         Test execution
[STUCK]        Skipped via Skip Protocol — needs human intervention
[BLOCKED]      Blocked by upstream STUCK (NOT for decisions)
[BARE-METAL]   Requires real kernel/BPF (the dev box has both — WEST also available)
[REGEN]        Regenerate derived artifact
[DOC-UPDATE]   Update downstream docs
```

---

## Variables

```
PROJECT_ROOT     = git rev-parse --show-toplevel  (resolved Step 1)
KERNEL_MBC       = $PROJECT_ROOT/crates/xv6-mbc/upstream/target/xv6-mbc.mbc
KERNEL_ELF       = $PROJECT_ROOT/crates/xv6-mbc/upstream/target/kernel.elf
                   (Marshal amendment 2026-05-10 Step 3: actual location is
                   under upstream/target/, not build/. The Warmonger drafted
                   from outdated assumption; corrected here without changing
                   any step semantics.)
INSTANCE_ID      = 222 (decimal) = 0xDE (hex) — the xv6-first-boot target
TTY_BRIDGE_PORT  = 26100
EBPF_OBJ         = $PROJECT_ROOT/target/bpfel-unknown-none/release/monad-cpu-ebpf
BPFFS_ROOT       = /sys/fs/bpf/unheaded/upc-xv6
```

---

## PREFLIGHT HYPOTHESES (Phase 0 only)

| # | Hypothesis | Verification | If false → action |
|---|------------|-------------|------------------|
| H1 | xv6-mbc.mbc exists and is 4-byte aligned | `cargo run -p upc-bootctl -- validate --kernel $KERNEL_MBC` | Rebuild xv6 first via `make -C crates/xv6-mbc/adapters -f Makefile.mbc` |
| H2 | doom-runner builds and is the canonical aya pattern | `cargo build -p doom-runner --release` | STUCK — pattern source unavailable; ESCALATE |
| H3 | monad-cpu-ebpf object exists and TTY_MAP is wired | `find target -name "monad-cpu-ebpf" -type f`; `grep TTY_MAP ebpf/monad-cpu-ebpf/src/main.rs` | Build via `make ebpf` |
| H4 | upc-tty-bridge listens on 26100 with `/api/v1/tty/ingest` endpoint | `grep -n "tty/ingest\|TtyIngest" cmd/upc-tty-bridge/main.go` | Add the endpoint in Phase 5 (planned — not a blocker) |
| H5 | The xv6 kernel.elf entry point is start_mbc.c at PC=0x10000 (no stage-1 stub needed for xv6) | `grep -E "_entry\|ENTRY\(" crates/xv6-mbc/adapters/kernel-mbc.ld` | Phase 1 [DECIDE] handles this |
| H6 | BPF verifier budget has headroom for the new live-boot path | Phase 0 ADR-067 left it at 7%/900K; this sprint doesn't touch the eBPF programs | If verifier rejects after a kernel rebuild, document and STUCK |
| H7 | The dev box (current host) supports `bpftool` and the kernel is 6.x | `uname -r && which bpftool` | If missing, switch to WEST (per `references/east-west-hosts.md`) |

---

## KNOWN FAILURES BASELINE

Before starting Phase 1, capture the current test failure state so regressions can be distinguished from pre-existing failures.

```bash
go test -short -count=1 ./... 2>&1 | grep -cE "^FAIL"   # expected: 0
make test-rust 2>&1 | grep -cE "FAILED"                  # expected: 0
```

**Recorded baseline** (from session-summary-2026-05-10.md): **0 Go failures across 242 packages, 0 Rust test failures across all crates.** Any non-zero count after Phase 7 is a NEW failure and HALTS the gate.

---

## PHASE 0: PREFLIGHT (Steps 1-12)

**Goal**: Verify all 7 hypotheses and capture the tested baseline.
**Prerequisite**: `git status` clean; `main` checked out; pushed.
**Time**: 30 minutes
**Agent**: Coordinator (Developer)

- [ ] **Step 1** [B] ~10s: Resolve `PROJECT_ROOT`
  ```bash
  cd $(git rev-parse --show-toplevel) && pwd
  ```

- [ ] **Step 2** [V] ~10s: Working tree is clean and HEAD matches origin/main
  ```bash
  git status -s && git rev-parse HEAD && git rev-parse origin/main
  ```
  - If dirty → `git stash` and document the stash hash; if HEAD ≠ origin/main → `git pull --ff-only`

- [ ] **Step 3** [PREFLIGHT][B] ~30s: H1 — xv6-mbc.mbc exists and is aligned
  ```bash
  ls -la crates/xv6-mbc/build/xv6-mbc.mbc 2>&1 || echo MISSING
  cargo run -p upc-bootctl -- validate --kernel crates/xv6-mbc/build/xv6-mbc.mbc 2>&1 | tail -10
  ```
  - If MISSING → Step 3a [D]; else → Step 4

- [ ] **Step 3a** [D] ~5m: Rebuild xv6-mbc kernel
  ```bash
  cd crates/xv6-mbc/adapters && make -f Makefile.mbc 2>&1 | tail -10 && cd $(git rev-parse --show-toplevel)
  ls -la crates/xv6-mbc/build/xv6-mbc.mbc
  ```
  - If still missing → STUCK + ESCALATE (build system regression)

- [ ] **Step 4** [PREFLIGHT][B] ~2m: H2 — doom-runner builds (canonical pattern available)
  ```bash
  cargo build -p doom-runner --release 2>&1 | tail -5
  ```
  - If fail → STUCK (pattern source needed); ESCALATE.

- [ ] **Step 5** [PREFLIGHT][B] ~30s: H3 — monad-cpu-ebpf object exists, TTY_MAP wired
  ```bash
  find target -name "monad-cpu-ebpf" -type f 2>&1 | head -3
  grep -nE "TTY_MAP|TTY_HEAD|EVENT_TTY_WRITE" ebpf/monad-cpu-ebpf/src/main.rs | head -5
  ```
  - If TTY_MAP missing from grep → STUCK + ESCALATE

- [ ] **Step 6** [PREFLIGHT][B] ~10s: H4 — check upc-tty-bridge endpoint surface
  ```bash
  grep -nE "tty/ingest|TtyIngest|HandleFunc" cmd/upc-tty-bridge/main.go
  ```
  - The endpoint will be ADDED in Phase 5 — not a blocker; just verify the file is the skeleton from session-summary not something replaced.

- [ ] **Step 7** [PREFLIGHT][R] ~30s: H5 — kernel.elf entry symbol
  ```bash
  grep -E "ENTRY\(|^\.stage1|_entry" crates/xv6-mbc/adapters/kernel-mbc.ld | head -5
  ```

- [ ] **Step 8** [PREFLIGHT][B] ~30s: H6 — verifier-budget snapshot
  ```bash
  bash scripts/bpf-verifier-check.sh 2>&1 | tail -5
  ```
  - Record the % budget shown. Phase 7 GATE will compare.

- [ ] **Step 9** [PREFLIGHT][B] ~10s: H7 — host capabilities
  ```bash
  uname -r && which bpftool && which clang && rustc --version
  ```

- [ ] **Step 10** [BUILD][B] ~3m: Capture KNOWN FAILURES BASELINE
  ```bash
  go test -short -count=1 ./... 2>&1 | tee /tmp/baseline-go-$(date +%s).log | grep -cE "^FAIL"
  ```

- [ ] **Step 11** [BUILD][B] ~2m: Capture Rust baseline
  ```bash
  make test-rust 2>&1 | tee /tmp/baseline-rust-$(date +%s).log | tail -5
  ```

- [ ] **Step 12** [V][C] ~10s: **PHASE 0 EXIT GATE**
  - All 7 hypotheses confirmed (or STUCK markers logged)
  - Baseline = 0 Go failures, 0 Rust failures
  - Verifier budget recorded
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 1-12: Phase 0 PREFLIGHT — baseline pristine, hypotheses verified

Pre-flight verified for upc-bootctl live BPF integration sprint.
Records the verifier budget + go/rust test baselines for the gate
comparison at Phase 7."
  ```
  - If pass → Phase 1
  - If fail → DO NOT PROCEED. Address each STUCK before resuming.

---

## PHASE 1: STAGE-1 STUB DECISION + xv6 BOOT PATH RESOLUTION (Steps 13-22)

**Goal**: Resolve the open question of whether xv6 needs a stage-1 stub for Phase 1.1 boot. Decision is `[DECIDE]` not `[ESCALATE]` because Warmonger has the recommendation pre-seeded from intelligence gathering.
**Prerequisite**: Phase 0 GATE passed.
**Time**: 1 hour
**Agent**: Coordinator (Developer + Computermancer for review at Step 16)

- [ ] **Step 13** [R] ~5m: Inspect kernel-mbc.ld stage1 section
  ```bash
  cat crates/xv6-mbc/adapters/kernel-mbc.ld
  ```
  - Confirm that `.stage1 0x00010000` exists and what it contains (xv6's _entry from start_mbc.c, OR an empty placeholder for upc-bootstub).

- [ ] **Step 14** [R] ~5m: Inspect start_mbc.c entry path
  ```bash
  cat crates/xv6-mbc/adapters/start_mbc.c | head -60
  ```
  - Confirm whether start_mbc.c does its own BSS zero, MMU init, S-mode transition (i.e. is it the de-facto stage-1?).

- [ ] **Step 15** [R] ~5m: Read TRANSLATOR_EXTENSIONS open issues for stage-1
  ```bash
  sed -n '120,140p' crates/xv6-mbc/TRANSLATOR_EXTENSIONS.md
  ```

- [ ] **Step 16** [DECIDE] ~10m: **STAGE-1 STUB STRATEGY FOR PHASE 1.1**

  **RECOMMENDATION**: Boot xv6 directly to `PC=0x10000` (start_mbc.c entry). DO NOT build `crates/upc-bootstub/` for this sprint.

  **Rationale**:
  1. Boot Protocol v2 mandates stage-1 stub for **Linux** (BSS zero-fill, M→S-mode transition). xv6 has no .bss to zero (verified via `nm crates/xv6-mbc/build/kernel.elf | grep ' B '`) and runs in M-mode-equivalent throughout.
  2. start_mbc.c already does the equivalent work xv6-side: sets up the trap vector, calls `main()`, etc. It IS the de-facto "stage-1 for xv6."
  3. Building `crates/upc-bootstub/` from scratch is a 1-2 day side quest that doesn't shorten the path to "xv6 booting..." banner.
  4. uClinux Phase 2 will REQUIRE the stub — that's the right time to build it (with the lessons from Phase 1.1 in hand).

  **Override ONLY if**: nm shows xv6 has unexpected .bss data, OR start_mbc.c doesn't actually run with priv_level=0 (M-mode at boot).

  **Decision recorded** in this plan as commitment. Computermancer reviews at Step 17.

- [ ] **Step 17** [V] ~5m: Computermancer-style ABI sanity (Marshal proxies if Computermancer not synchronously available)
  ```bash
  nm crates/xv6-mbc/build/kernel.elf | grep -E ' [Bb] ' | head -5  # .bss symbols (expect: empty or tiny)
  grep -nE "priv_level|MRET|csrw" crates/xv6-mbc/adapters/start_mbc.c | head
  ```
  - If .bss is non-empty AND start_mbc.c doesn't zero it → revisit Step 16; we DO need the stub.
  - If clean → recommendation stands; document in commit message at Step 22.

- [ ] **Step 18** [DECIDE] ~5m: **TTY TRANSPORT STRATEGY**

  **RECOMMENDATION**: upc-bootctl runs an in-process TTY poller (Tokio task) that reads `TTY_MAP` + `TTY_HEAD` from BPF every 50ms and HTTP POSTs new bytes to `http://127.0.0.1:26100/api/v1/tty/ingest`. upc-tty-bridge gains that one new endpoint in Phase 5; existing `/ws` fan-out reuses the same `Hub.broadcastTty` already scaffolded.

  **Rationale**:
  1. Keeps upc-bootctl as the single owner of BPF map I/O (no two-process coordination).
  2. upc-tty-bridge stays in Go (no rewrite), gains exactly one new HTTP endpoint.
  3. HTTP loopback POST at 50ms cadence is ~20 req/s — trivial latency, easy to debug with curl.
  4. Wotan-topic transport (`compute.tty.{instance}`) is cleaner architecturally but requires Wotan to be running for the demo to work; this path runs standalone.

  **Override ONLY if**: Stevie wants the demo to depend on Wotan being live (then use the topic).

- [ ] **Step 19** [DECIDE] ~5m: **HALT-CLEANUP STRATEGY**

  **RECOMMENDATION**: upc-bootctl `boot` registers a `signal::ctrl_c()` handler AND watches `CPU_MAP[INSTANCE_ID].halted` byte. On either condition: drain TTY one final time, then call `cpu_map.remove(&INSTANCE_ID)`, `screen_map.set(...)` to zero, `kbd_map.set(0xC0DE_KEY, 0)`, and exit cleanly.

  **Rationale**: doom-runner doesn't have explicit halt-cleanup (Doom plays forever). Phase 1.1 acceptance explicitly requires "no stale entries in CPU_MAP / SCREEN_MAP / KBD_MAP" — this is the simplest reliable shape.

  **Override ONLY if**: aya doesn't expose `.remove()` on the CPU_MAP type — then fallback to overwriting with zeroed MbcCpuState.

- [ ] **Step 20** [W] ~10m: Document the 3 decisions in `crates/xv6-mbc/TRANSLATOR_EXTENSIONS.md` so future sessions don't re-litigate
  ```bash
  cat >> crates/xv6-mbc/TRANSLATOR_EXTENSIONS.md << 'EOF'

  ## Phase 1.1 SHIP decisions (2026-05-10)

  - **Stage-1 stub deferred to Phase 2 (uClinux).** xv6 has no .bss; start_mbc.c
    runs in priv_level=0 M-mode-equivalent and is the de-facto stage-1 for xv6.
    crates/upc-bootstub/ will be authored in Phase 2 when uClinux's BSS zero-fill
    + S-mode transition becomes load-bearing.
  - **TTY transport is HTTP loopback** from upc-bootctl runner → upc-tty-bridge
    on port 26100. New endpoint /api/v1/tty/ingest. Wotan-topic path deferred.
  - **Halt cleanup**: ctrl_c OR halted-byte detection → drain TTY → remove
    CPU_MAP[instance] → zero SCREEN_MAP/KBD_MAP → exit.
  EOF
  ```

- [ ] **Step 21** [TEST][B] ~10s: Confirm xv6 has no .bss requiring zero-fill (sanity for Step 16 decision)
  ```bash
  size crates/xv6-mbc/build/kernel.elf
  ```
  - bss column should be 0 or very small (<1KB). If non-zero & non-zeroed by start_mbc.c → re-open Step 16.

- [ ] **Step 22** [V][C] ~5s: **PHASE 1 EXIT GATE**
  - 3 [DECIDE] steps resolved with committed recommendations
  - TRANSLATOR_EXTENSIONS.md updated with the decisions
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 13-22: Phase 1 — stage-1 stub deferred, TTY transport decided

Three Phase 1.1 design decisions made + documented:
1. Stage-1 stub crate deferred to Phase 2 (uClinux). xv6 boots directly
   to start_mbc.c at PC=0x10000.
2. TTY transport: HTTP loopback upc-bootctl → upc-tty-bridge:26100
   /api/v1/tty/ingest. Wotan-topic path deferred.
3. Halt cleanup: ctrl_c OR halted-byte → drain TTY → remove map entries."
  ```

---

## PHASE 2: LIFT doom-runner AYA PATTERN INTO upc-bootctl (Steps 23-42)

**Goal**: Bring the proven aya-Ebpf-load + map-population pattern from `crates/doom-runner/src/main.rs` into `cmd/upc-bootctl/src/main.rs::cmd_boot`. After this phase: `upc-bootctl boot` opens the eBPF object, attaches XDP, and exposes mut handles to ROM_MAP/RAM_MAP/CPU_MAP — but does NOT yet write the kernel image.
**Prerequisite**: Phase 1 GATE passed.
**Time**: 4-6 hours
**Agent**: Coordinator (Developer)

- [ ] **Step 23** [R] ~15m: Read the canonical aya bind sequence
  ```bash
  sed -n '280,470p' crates/doom-runner/src/main.rs
  ```
  - Note the order: `Ebpf::load_file()` → `bpf.maps()` enumerate → attach XDP → `Array::try_from(ebpf.map_mut("X"))` for each map.

- [ ] **Step 24** [R] ~10m: Read the MbcCpuState replica struct
  ```bash
  sed -n '8,60p' crates/doom-runner/src/main.rs
  ```
  - We will lift this VERBATIM into upc-bootctl. Same struct, same `unsafe impl aya::Pod`, same 136-byte assertion.

- [ ] **Step 25** [W][CODE] ~15m: Add aya dependency to upc-bootctl
  - File: `cmd/upc-bootctl/Cargo.toml`
  - Add under `[dependencies]`:
    ```toml
    aya = "0.13"
    aya-log = "0.2"
    bytemuck = "1.16"
    tokio = { version = "1", features = ["rt-multi-thread", "macros", "signal", "time"] }
    reqwest = { version = "0.12", default-features = false, features = ["json"] }
    serde = { version = "1", features = ["derive"] }
    serde_json = "1"
    tracing = "0.1"
    tracing-subscriber = { version = "0.3", features = ["env-filter"] }
    ```

- [ ] **Step 26** [BUILD][B] ~2m: Verify aya pulls clean
  ```bash
  cargo build -p upc-bootctl 2>&1 | tail -5
  ```

- [ ] **Step 27** [V] ~10s: Build green
  - If fail → check Cargo.lock conflicts; doom-runner already uses aya 0.13 so versions should resolve.

- [ ] **Step 28** [W][CODE] ~30m: Create `cmd/upc-bootctl/src/runner.rs`
  - **Module purpose**: encapsulate the live-BPF boot path. Keeps `main.rs` lean (CLI parsing only) and makes the runner unit-testable in isolation.
  - **Module skeleton** (write just the type + signatures + TODO bodies):
    ```rust
    //! Live BPF boot path for upc-bootctl. Lifts the aya pattern from
    //! crates/doom-runner/src/main.rs and adapts it for xv6-mbc kernel
    //! images per UPC Boot Protocol v2.

    use anyhow::{anyhow, bail, Context, Result};
    use aya::maps::{Array, HashMap as AyaHashMap};
    use aya::programs::Xdp;
    use aya::Ebpf;
    use std::path::Path;

    /// Replica of monad-common::MbcCpuState (ABI v2, 136 bytes).
    /// Lifted from crates/doom-runner/src/main.rs lines ~22-50.
    #[repr(C)]
    #[derive(Clone, Copy, Debug)]
    pub struct MbcCpuState {
        // ... copy verbatim from doom-runner
    }
    unsafe impl aya::Pod for MbcCpuState {}
    const _: () = assert!(std::mem::size_of::<MbcCpuState>() == 136);

    pub struct BootRunner {
        ebpf: Ebpf,
        instance_id: u32,
    }

    impl BootRunner {
        pub fn open(ebpf_obj_path: &Path, instance_id: u32) -> Result<Self> { todo!() }
        pub fn populate_rom(&mut self, mbc_words: &[u32]) -> Result<()> { todo!() }
        pub fn populate_ram(&mut self, regions: &[(u32, &[u8])]) -> Result<()> { todo!() }
        pub fn populate_cpu(&mut self, initial_state: MbcCpuState) -> Result<()> { todo!() }
        pub fn attach_xdp(&mut self, iface: &str) -> Result<()> { todo!() }
        pub fn cpu_state(&self) -> Result<MbcCpuState> { todo!() }
        pub fn read_tty(&self, head_cursor: &mut u32) -> Result<Vec<u8>> { todo!() }
        pub fn cleanup(&mut self) -> Result<()> { todo!() }
    }
    ```

- [ ] **Step 29** [W][CODE] ~5m: Wire runner into `main.rs`
  - In `cmd/upc-bootctl/src/main.rs`, add `mod runner;` near the top.

- [ ] **Step 30** [BUILD][B] ~30s: Skeleton compiles
  ```bash
  cargo build -p upc-bootctl 2>&1 | tail -5
  ```

- [ ] **Step 31** [V] ~10s: Build green with `unused` warnings on the todos (acceptable)

- [ ] **Step 32** [C] ~5s: Commit skeleton
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 23-32: Phase 2 — runner.rs skeleton + aya deps"
  ```

- [ ] **Step 33** [W][CODE] ~30m: Implement `BootRunner::open`
  - Body:
    ```rust
    pub fn open(ebpf_obj_path: &Path, instance_id: u32) -> Result<Self> {
        let ebpf = Ebpf::load_file(ebpf_obj_path)
            .with_context(|| format!("load eBPF object: {}", ebpf_obj_path.display()))?;
        Ok(Self { ebpf, instance_id })
    }
    ```

- [ ] **Step 34** [W][CODE] ~30m: Implement `populate_rom`
  - Body (mirror doom-runner main.rs:401-415):
    ```rust
    pub fn populate_rom(&mut self, mbc_words: &[u32]) -> Result<()> {
        let mut rom: Array<_, u32> =
            Array::try_from(self.ebpf.map_mut("ROM_MAP").context("ROM_MAP not found")?)?;
        for (i, &word) in mbc_words.iter().enumerate() {
            rom.set(i as u32, word, 0)
                .with_context(|| format!("ROM_MAP[{}] write", i))?;
        }
        tracing::info!(words = mbc_words.len(), "ROM_MAP populated");
        Ok(())
    }
    ```

- [ ] **Step 35** [W][CODE] ~30m: Implement `populate_ram` (multi-region writes — BootParams, cmdline, etc.)
  - Body:
    ```rust
    pub fn populate_ram(&mut self, regions: &[(u32, &[u8])]) -> Result<()> {
        let mut ram: Array<_, u32> =
            Array::try_from(self.ebpf.map_mut("RAM_MAP").context("RAM_MAP not found")?)?;
        for &(byte_addr, data) in regions {
            // Pack into u32 words; require 4-byte alignment of byte_addr.
            if byte_addr % 4 != 0 {
                bail!("populate_ram: byte_addr 0x{:08X} not 4-byte aligned", byte_addr);
            }
            let word_addr_base = byte_addr / 4;
            let mut chunks = data.chunks_exact(4);
            for (i, chunk) in chunks.by_ref().enumerate() {
                let w = u32::from_le_bytes([chunk[0], chunk[1], chunk[2], chunk[3]]);
                ram.set(word_addr_base + i as u32, w, 0)
                    .with_context(|| format!("RAM_MAP[0x{:08X}] write", byte_addr + (i * 4) as u32))?;
            }
            // Handle remainder (tail bytes < 4) — pad with zeros.
            let rem = chunks.remainder();
            if !rem.is_empty() {
                let mut padded = [0u8; 4];
                padded[..rem.len()].copy_from_slice(rem);
                let w = u32::from_le_bytes(padded);
                ram.set(word_addr_base + (data.len() / 4) as u32, w, 0)?;
            }
        }
        Ok(())
    }
    ```

- [ ] **Step 36** [W][CODE] ~30m: Implement `populate_cpu`
  - Body (mirror doom-runner main.rs:465-475 but for HashMap, not single-instance):
    ```rust
    pub fn populate_cpu(&mut self, initial_state: MbcCpuState) -> Result<()> {
        let mut cpu: AyaHashMap<_, u32, MbcCpuState> =
            AyaHashMap::try_from(self.ebpf.map_mut("CPU_MAP").context("CPU_MAP not found")?)?;
        cpu.insert(self.instance_id, initial_state, 0)
            .with_context(|| format!("CPU_MAP[0x{:X}] insert", self.instance_id))?;
        tracing::info!(instance = self.instance_id, "CPU_MAP populated");
        Ok(())
    }
    ```

- [ ] **Step 37** [W][CODE] ~20m: Implement `attach_xdp`
  - Body (mirror doom-runner main.rs around prog_load + attach):
    ```rust
    pub fn attach_xdp(&mut self, iface: &str) -> Result<()> {
        let prog: &mut Xdp = self.ebpf
            .program_mut("monad_cpu")
            .context("monad_cpu program not found in eBPF object")?
            .try_into()?;
        prog.load().context("XDP program load")?;
        prog.attach(iface, aya::programs::XdpFlags::default())
            .with_context(|| format!("XDP attach to {}", iface))?;
        tracing::info!(iface, "XDP attached");
        Ok(())
    }
    ```

- [ ] **Step 38** [W][CODE] ~15m: Implement `cpu_state` and `cleanup`
  - Bodies:
    ```rust
    pub fn cpu_state(&self) -> Result<MbcCpuState> {
        let cpu: AyaHashMap<_, u32, MbcCpuState> =
            AyaHashMap::try_from(self.ebpf.map("CPU_MAP").context("CPU_MAP")?)?;
        cpu.get(&self.instance_id, 0)
            .with_context(|| format!("CPU_MAP[0x{:X}] get", self.instance_id))
    }

    pub fn cleanup(&mut self) -> Result<()> {
        let mut cpu: AyaHashMap<_, u32, MbcCpuState> =
            AyaHashMap::try_from(self.ebpf.map_mut("CPU_MAP")?)?;
        let _ = cpu.remove(&self.instance_id);  // best-effort
        tracing::info!(instance = self.instance_id, "CPU_MAP entry removed");
        Ok(())
    }
    ```

- [ ] **Step 39** [W][CODE] ~30m: Implement `read_tty` (the TTY drain helper)
  - Body:
    ```rust
    pub fn read_tty(&self, head_cursor: &mut u32) -> Result<Vec<u8>> {
        let tty: Array<_, u8> = Array::try_from(self.ebpf.map("TTY_MAP").context("TTY_MAP")?)?;
        let head_map: Array<_, u32> = Array::try_from(self.ebpf.map("TTY_HEAD").context("TTY_HEAD")?)?;
        let new_head = head_map.get(&0u32, 0)?;
        if new_head == *head_cursor { return Ok(vec![]); }
        let mut bytes = Vec::new();
        let cap = 4096u32;
        let mut idx = *head_cursor;
        while idx != new_head {
            bytes.push(tty.get(&idx, 0)?);
            idx = (idx + 1) % cap;
        }
        *head_cursor = new_head;
        Ok(bytes)
    }
    ```

- [ ] **Step 40** [BUILD][B] ~1m: All 7 BootRunner methods compile
  ```bash
  cargo build -p upc-bootctl 2>&1 | tail -10
  ```

- [ ] **Step 41** [V] ~10s: Build green, no `todo!()` left
  - If `todo!()` panics during cargo build (it doesn't, only at runtime), still run `grep -n 'todo!' cmd/upc-bootctl/src/runner.rs` — expect 0 hits.

- [ ] **Step 42** [V][C] ~10s: **PHASE 2 EXIT GATE**
  - `cargo build -p upc-bootctl` green, no warnings about runner.rs
  - `cargo test -p upc-bootctl` green (existing 5 alignment tests still pass)
  ```bash
  cargo test -p upc-bootctl 2>&1 | tail -5
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 33-42: Phase 2 — BootRunner methods implemented (no map writes yet from cmd_boot)"
  ```
  - If pass → Phase 3
  - **Rollback plan**: if compile errors persist >2 attempts, `git checkout HEAD~ -- cmd/upc-bootctl/src/` and STUCK + ESCALATE the aya-version mismatch.

---

## PHASE 3: BootParams v2 + MEMORY LAYOUT IN RUST (Steps 43-58)

**Goal**: Build the in-memory representation of BootParams v2 + the IVT + cmdline + initial CPU state, ready to hand to BootRunner. After this phase: `cmd_boot` constructs all the bytes that will be written to BPF maps but does not yet call the runner.
**Prerequisite**: Phase 2 GATE passed.
**Time**: 3-4 hours
**Agent**: Coordinator (Developer + Computermancer reviews Step 47)

- [ ] **Step 43** [W][CODE] ~30m: Create `cmd/upc-bootctl/src/bootparams.rs`
  - **Module purpose**: encode BootParams v2 (256 B) + memory layout helpers per `docs/doom/UPC_BOOT_PROTOCOL_V2.md`. Pure functions, fully unit-testable.
  - **Skeleton**:
    ```rust
    //! BootParams v2 encoder + memory-layout constants for UPC Boot Protocol v2.

    pub const BOOT_MAGIC: u32 = 0x554E_4844;  // 'UNHD'
    pub const BOOT_VERSION: u32 = 2;

    pub const ADDR_IVT: u32 = 0x0000;
    pub const ADDR_BOOTPARAMS: u32 = 0x0100;
    pub const ADDR_CMDLINE: u32 = 0x0200;
    pub const ADDR_TRAP_HANDLER: u32 = 0x0400;
    pub const ADDR_CSR_REGION: u32 = 0xF000;
    pub const ADDR_KERNEL_DEFAULT: u32 = 0x10000;  // xv6 entry (start_mbc.c)
    pub const ADDR_RAMDISK_DEFAULT: u32 = 0x800000;
    pub const ADDR_STACK_TOP: u32 = 0x03F0_0000;

    pub const SIZE_IVT: u32 = 1024;
    pub const SIZE_BOOTPARAMS: u32 = 256;
    pub const SIZE_CMDLINE_MAX: u32 = 512;
    pub const SIZE_CSR_REGION: u32 = 256;

    #[repr(C, packed)]
    pub struct BootParamsV2 {
        // Lift verbatim from docs/doom/UPC_BOOT_PROTOCOL_V2.md
        pub magic: u32,
        pub version: u32,
        pub memory_size: u32,
        pub ramdisk_addr: u32,
        pub ramdisk_size: u32,
        pub kernel_addr: u32,
        pub kernel_size: u32,
        pub boot_args_addr: u32,
        pub boot_args_len: u32,
        pub num_cpus: u32,
        pub tick_rate_hz: u32,
        pub bss_start: u32,
        pub bss_end: u32,
        pub initrd2_addr: u32,
        pub cmd_line_args_ptr: u32,
        pub boot_random_seed: u32,
        pub reserved: [u32; 48],
    }
    const _: () = assert!(std::mem::size_of::<BootParamsV2>() == 256);

    impl BootParamsV2 {
        pub fn for_xv6(kernel_size_bytes: u32, ramdisk_size_bytes: u32) -> Self { todo!() }
        pub fn to_bytes(&self) -> [u8; 256] { todo!() }
    }
    ```

- [ ] **Step 44** [W][CODE] ~20m: Implement `BootParamsV2::for_xv6`
  - Body:
    ```rust
    pub fn for_xv6(kernel_size_bytes: u32, ramdisk_size_bytes: u32) -> Self {
        Self {
            magic: BOOT_MAGIC,
            version: BOOT_VERSION,
            memory_size: 64 * 1024 * 1024,
            ramdisk_addr: ADDR_RAMDISK_DEFAULT,
            ramdisk_size: ramdisk_size_bytes,
            kernel_addr: ADDR_KERNEL_DEFAULT,
            kernel_size: kernel_size_bytes,
            boot_args_addr: ADDR_CMDLINE,
            boot_args_len: 0,
            num_cpus: 1,
            tick_rate_hz: 12,
            bss_start: 0,        // xv6 has no .bss to zero per Phase 1 decision
            bss_end: 0,
            initrd2_addr: 0,
            cmd_line_args_ptr: 0,
            boot_random_seed: 0, // entropy not provided in Phase 1.1
            reserved: [0u32; 48],
        }
    }
    ```

- [ ] **Step 45** [W][CODE] ~20m: Implement `to_bytes` using bytemuck
  - Body:
    ```rust
    pub fn to_bytes(&self) -> [u8; 256] {
        // Safety: BootParamsV2 is repr(C, packed), 256 bytes exact, no padding.
        unsafe { std::mem::transmute_copy(self) }
    }
    ```

- [ ] **Step 46** [W][CODE] ~20m: Add unit tests in same file
  ```rust
  #[cfg(test)]
  mod tests {
      use super::*;

      #[test]
      fn bootparams_size_is_exactly_256() {
          assert_eq!(std::mem::size_of::<BootParamsV2>(), 256);
      }

      #[test]
      fn for_xv6_sets_magic_version_addresses() {
          let bp = BootParamsV2::for_xv6(46_884, 0);
          assert_eq!(bp.magic, 0x554E_4844);
          assert_eq!(bp.version, 2);
          assert_eq!(bp.kernel_addr, 0x10000);
          assert_eq!(bp.kernel_size, 46_884);
          assert_eq!(bp.num_cpus, 1);
          assert_eq!(bp.bss_start, 0);
      }

      #[test]
      fn to_bytes_first_four_bytes_are_magic_le() {
          let bp = BootParamsV2::for_xv6(0, 0);
          let bytes = bp.to_bytes();
          assert_eq!(&bytes[0..4], &[0x44, 0x48, 0x4E, 0x55]);  // 'DHNU' LE of 'UNHD'
      }

      #[test]
      fn reserved_is_zeroed_for_forward_compat() {
          let bp = BootParamsV2::for_xv6(123, 456);
          let bytes = bp.to_bytes();
          // BootParams reserved starts at offset 256-(48*4) = 256-192 = 64
          assert!(bytes[64..256].iter().all(|&b| b == 0), "reserved must be zero");
      }
  }
  ```

- [ ] **Step 47** [V][TEST][B] ~30s: Computermancer-style ABI gate — bootparams tests pass and struct size is exactly 256
  ```bash
  cargo test -p upc-bootctl --lib bootparams 2>&1 | tail -8
  ```
  - If size assertion fails → STUCK + ESCALATE (likely a missing `#[repr(C, packed)]` or padding issue)

- [ ] **Step 48** [W][CODE] ~10m: Wire `mod bootparams;` into `main.rs`

- [ ] **Step 49** [C] ~5s: Commit bootparams module
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 43-49: Phase 3 — BootParams v2 encoder + 4 unit tests"
  ```

- [ ] **Step 50** [W][CODE] ~30m: Build the initial `MbcCpuState` for xv6 boot
  - Helper in `cmd/upc-bootctl/src/runner.rs`:
    ```rust
    pub fn xv6_initial_cpu_state() -> MbcCpuState {
        let mut state = MbcCpuState {
            regs: [0u32; 16],
            pc: 0x10000 / 4,  // word index, NOT byte address
            flags: 0,
            halted: 0,
            stalled: 0,
            _pad: 0,
            sleep_until_ns: 0,
            insn_count: 0,
            cache_hits: 0,
            cache_misses: 0,
            interrupt_pending: 0,
            interrupt_vector: 0,
            interrupts_enabled: 0,
            _pad2: 0,
            tick_counter: 0,
            program_break: 0,
            exit_code: 0,
            current_pid: 0,
            num_processes: 1,
            mmu_enabled: 0,
            _pad3: 0,
            page_dir_base: 0,
            priv_level: 0,         // M-mode
            _pad4: 0, _pad5: 0, _pad6: 0,
            reservation_address: 0xFFFF_FFFF,  // no LR reservation
        };
        state.regs[15] = 0x03F0_0000;  // SP = stack top byte address
        state
    }
    ```

- [ ] **Step 51** [W][CODE] ~45m: Rewrite `cmd_boot` in main.rs to use BootRunner + BootParams
  - Replace the current "live boot path not yet implemented" body with the orchestration:
    ```rust
    fn cmd_boot(
        kernel: PathBuf,
        initramfs: Option<PathBuf>,
        instance: u8,
        dry_run: bool,
    ) -> Result<()> {
        // (existing validate output retained)
        let kernel_bytes = std::fs::read(&kernel)?;
        let n_insns = check_image_alignment(&kernel_bytes)?;
        let mbc_words: Vec<u32> = kernel_bytes
            .chunks_exact(4)
            .map(|c| u32::from_le_bytes([c[0], c[1], c[2], c[3]]))
            .collect();

        let bp = bootparams::BootParamsV2::for_xv6(kernel_bytes.len() as u32, 0);
        let bp_bytes = bp.to_bytes();

        if dry_run {
            // (existing dry-run prints retained)
            return Ok(());
        }

        // ── live path ──
        let ebpf_obj = std::env::var("MONAD_CPU_EBPF_OBJ")
            .unwrap_or_else(|_| {
                "target/bpfel-unknown-none/release/monad-cpu-ebpf".to_string()
            });
        let ebpf_obj = std::path::PathBuf::from(ebpf_obj);
        if !ebpf_obj.exists() {
            bail!(
                "monad-cpu-ebpf object not found at {}\n\
                 Build it first: make ebpf",
                ebpf_obj.display()
            );
        }

        let mut runner = runner::BootRunner::open(&ebpf_obj, instance as u32)?;

        // Memory writes per Boot Protocol v2 §"Boot Sequence"
        runner.populate_ram(&[
            (bootparams::ADDR_IVT, &[0u8; 1024]),
            (bootparams::ADDR_BOOTPARAMS, &bp_bytes),
            (bootparams::ADDR_CSR_REGION, &[0u8; 256]),
        ])?;
        runner.populate_rom(&mbc_words)?;
        runner.populate_cpu(runner::xv6_initial_cpu_state())?;

        // Phase 4 will continue here; for now, just confirm CPU state was written.
        let cpu = runner.cpu_state()?;
        println!("CPU_MAP[0x{:X}] populated: PC=0x{:08X} SP=0x{:08X} priv={}",
            instance, cpu.pc, cpu.regs[15], cpu.priv_level);

        Ok(())
    }
    ```

- [ ] **Step 52** [BUILD][B] ~30s: Build green
  ```bash
  cargo build -p upc-bootctl 2>&1 | tail -5
  ```

- [ ] **Step 53** [V] ~10s: Build clean
  - If errors → debug branch in Step 54

- [ ] **Step 54** [D] ~5m: Common build issues
  - Missing `bytemuck` derive: add `#[derive(Copy, Clone)]` to BootParamsV2 if needed
  - `repr(C, packed)` warnings: acceptable; suppress with `#[allow(unsafe_op_in_unsafe_fn)]` if rustc complains

- [ ] **Step 55** [TEST][B] ~30s: bootparams unit tests still green
  ```bash
  cargo test -p upc-bootctl 2>&1 | tail -5
  ```

- [ ] **Step 56** [V] ~10s: All upc-bootctl tests pass

- [ ] **Step 57** [BUILD][B] ~30s: Verify dry-run still works (no regression to existing path)
  ```bash
  cargo run -p upc-bootctl -- boot --kernel crates/xv6-mbc/build/xv6-mbc.mbc --instance 222 --dry-run 2>&1 | tail -15
  ```

- [ ] **Step 58** [V][C] ~10s: **PHASE 3 EXIT GATE**
  - Build green
  - `cargo test -p upc-bootctl` shows ≥9 tests passing (5 alignment + 4 bootparams)
  - dry-run output matches Phase 0 baseline (no semantic regression)
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 50-58: Phase 3 — cmd_boot live-path scaffolding (no XDP attach yet)"
  ```
  - If pass → Phase 4
  - **Rollback plan**: if BootParams size assertion fails repeatedly, `git checkout HEAD~ -- cmd/upc-bootctl/src/bootparams.rs` and STUCK + revisit packing strategy with Computermancer.

---

## PHASE 4: FIRST BOOT — OBSERVE CPU_MAP ADVANCEMENT (Steps 59-72)

**Goal**: First sign of life. After this phase: a `sudo cargo run -p upc-bootctl -- boot --kernel ...` invocation populates maps, attaches XDP, dispatches a Gjallarhorn UPC trigger packet, and the eBPF interpreter advances `CPU_MAP[0xDE].pc` and `insn_count`. **The "hello packet" moment.**
**Prerequisite**: Phase 3 GATE passed.
**Time**: 4-6 hours (this is the iteration-heavy phase)
**Agent**: Coordinator (Developer; Marshal watches for STUCK signals — XDP debugging can spiral)

- [ ] **Step 59** [DESIGN] ~10m: Decide the network namespace for first boot
  - **RECOMMENDATION**: Use a fresh `upc0` namespace with a single `veth-upc0` pair, mirroring `crates/doom-runner/src/ring.rs::single_hop_setup`. Avoid the `monad0..monadN` ring used by Doom — that's overkill for one xv6 instance.
  - Document in Step 60.

- [ ] **Step 60** [W][CODE] ~30m: Add `cmd/upc-bootctl/src/netns.rs`
  - Bash `ip` calls via `std::process::Command` — same pattern as `doom-runner/src/ring.rs`. Functions: `setup_upc0()`, `teardown_upc0()`. Idempotent.

- [ ] **Step 61** [W][CODE] ~10m: `cmd_boot` calls `netns::setup_upc0()` after BootParams write, before `runner.attach_xdp("veth-upc0")`. Use a `defer`-style scopeguard to call `teardown_upc0()` on exit (or use Drop).

- [ ] **Step 62** [BUILD][B] ~30s: Build green
  ```bash
  cargo build -p upc-bootctl 2>&1 | tail -5
  ```

- [ ] **Step 63** [V] ~10s: Build clean

- [ ] **Step 64** [BUILD][B] ~3m: Build the eBPF object if not already built
  ```bash
  make ebpf 2>&1 | tail -5
  find target -name "monad-cpu-ebpf" -type f
  ```

- [ ] **Step 65** [S][B] ~30s: **FIRST BOOT ATTEMPT** — populate maps + attach XDP + observe
  ```bash
  sudo -E env "PATH=$PATH" cargo run -p upc-bootctl -- boot --kernel crates/xv6-mbc/build/xv6-mbc.mbc --instance 222 2>&1 | tee /tmp/upc-first-boot.log | tail -30
  ```

- [ ] **Step 66** [V] ~30s: **FIRST HEARTBEAT** — confirm BootRunner.populate_* succeeded
  - Expected output includes: "ROM_MAP populated: 11721 words", "CPU_MAP populated", "XDP attached", "CPU_MAP[0xDE] populated: PC=0x00004000 SP=0x03F00000 priv=0"
  - If missing any: → Step 67 [D]

- [ ] **Step 67** [D] ~5m: First-boot debug
  - Map missing? `sudo bpftool map list | grep -E "ROM_MAP|RAM_MAP|CPU_MAP|TTY_MAP" | head`
  - XDP failed to attach? `sudo dmesg | tail -20 | grep -i xdp`
  - Permission denied? confirm `sudo -E` propagates `PATH` and `HOME` for cargo's cache.

- [ ] **Step 68** [S][B] ~30s: Inject trigger packet (manual for first boot)
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1 --iface veth-upc0 2>&1 | tail -5
  ```
  - If `scripts/doom-tick.py` not parameterizable for iface, fall back to: `sudo ip netns exec upc0 ping6 -c 1 fd00:dead:beef:dada::de` (reuses existing IPv6-on-Monad trigger).

- [ ] **Step 69** [S][B] ~30s: Read CPU state after trigger
  ```bash
  # Re-invoke upc-bootctl with a `status` subcommand we'll add inline:
  # OR use bpftool directly:
  sudo bpftool map dump name CPU_MAP 2>&1 | tail -20
  ```

- [ ] **Step 70** [V] ~10s: **PC ADVANCED** — `pc != 0x4000` (initial value, word index of byte 0x10000) AND `insn_count > 0`
  - If PC unchanged → Step 71 [D]
  - If PC advanced → forward progress confirmed; → Step 72

- [ ] **Step 71** [D] ~10m: PC-not-advanced debug branch (max 2 attempts then STUCK)
  - Attempt 1: Check XDP return codes via `sudo bpftool prog tracelog | tail -30`
  - Attempt 2: Verify `attach_xdp` actually loaded `monad_cpu` (not a sibling program). `sudo bpftool prog show | grep monad`
  - 2 attempts failed → Mark Step 71 STUCK; commit current state; SKIP forward to Phase 5 (Phase 5 doesn't depend on observing PC advancement, it depends on the maps being populated).

- [ ] **Step 72** [V][C] ~10s: **PHASE 4 EXIT GATE**
  - Either: PC advanced (forward progress confirmed), OR Phase 5 can still run with STUCK marker on Step 71
  ```bash
  sudo cleanup_old_bpf_maps_if_needed
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 59-72: Phase 4 — first boot, $(if pc-advanced; then echo CPU_MAP advances on packet; else echo STUCK observability gap, see step 71; fi)"
  ```
  - If pass → Phase 5
  - **Rollback plan**: `sudo rm -rf /sys/fs/bpf/unheaded/upc-xv6/*` to clear pinned maps; `sudo ip netns del upc0` to clean network. Re-run from Step 65.

---

## PHASE 5: TTY PIPELINE — upc-bootctl → upc-tty-bridge → BROWSER xterm (Steps 73-86)

**Goal**: Wire the TTY drain. After this phase: bytes written by xv6 to MMIO 0xC001 (`uartwrite`/`uartputc_sync` in console-mmio.c) appear in upc-tty-bridge's WebSocket fan-out and render in browser xterm.js as "xv6 booting...".
**Prerequisite**: Phase 4 GATE passed (or STUCK with maps populated).
**Time**: 3-4 hours
**Agent**: Coordinator (Developer; cross-language Go + Rust)

- [ ] **Step 73** [W][CODE] ~20m: Add `/api/v1/tty/ingest` endpoint to upc-tty-bridge
  - File: `cmd/upc-tty-bridge/main.go`
  - Add route + handler:
    ```go
    type ttyIngest struct {
        Instance uint8 `json:"instance"`
        Bytes    []byte `json:"bytes"`  // base64-encoded JSON []byte
    }

    func (h *Hub) handleTtyIngest(w http.ResponseWriter, r *http.Request) {
        var req ttyIngest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        h.broadcastTty(req.Instance, req.Bytes)
        w.WriteHeader(http.StatusNoContent)
    }
    ```
  - Register: `mux.HandleFunc("/api/v1/tty/ingest", hub.handleTtyIngest)`

- [ ] **Step 74** [W][CODE] ~10m: Add unit test for handleTtyIngest in `cmd/upc-tty-bridge/main_test.go`
  - Send POST with body `{"instance":222,"bytes":"eHY2IGJvb3RpbmcuLi4="}` (base64 of "xv6 booting..."), verify Hub fan-out call.

- [ ] **Step 75** [BUILD][TEST][B] ~30s: Verify Go build + test
  ```bash
  go build ./cmd/upc-tty-bridge/... 2>&1 | tail -3
  go test -count=1 ./cmd/upc-tty-bridge/... 2>&1 | tail -5
  ```

- [ ] **Step 76** [V] ~10s: Build green, all tests pass

- [ ] **Step 77** [C] ~5s: Commit Go-side endpoint
  ```bash
  git add cmd/upc-tty-bridge/ && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 73-77: Phase 5 — upc-tty-bridge gains /api/v1/tty/ingest"
  ```

- [ ] **Step 78** [W][CODE] ~45m: Add Tokio TTY-poller task in upc-bootctl
  - In `cmd/upc-bootctl/src/runner.rs`, add:
    ```rust
    pub async fn run_tty_poller(
        runner: Arc<Mutex<BootRunner>>,
        instance_id: u8,
        bridge_url: String,
        shutdown: tokio::sync::watch::Receiver<bool>,
    ) -> Result<()> {
        let client = reqwest::Client::new();
        let mut head_cursor = 0u32;
        let mut shutdown = shutdown;
        loop {
            tokio::select! {
                _ = shutdown.changed() => { tracing::info!("tty poller shutting down"); break; }
                _ = tokio::time::sleep(std::time::Duration::from_millis(50)) => {}
            }
            let bytes = {
                let r = runner.lock().unwrap();
                r.read_tty(&mut head_cursor)?
            };
            if !bytes.is_empty() {
                let body = serde_json::json!({"instance": instance_id, "bytes": bytes});
                let _ = client
                    .post(&format!("{}/api/v1/tty/ingest", bridge_url))
                    .json(&body)
                    .send().await;
            }
        }
        Ok(())
    }
    ```
  - Note: `Arc<Mutex<BootRunner>>` — std::sync::Mutex is fine since `read_tty` is synchronous and BPF reads are fast.

- [ ] **Step 79** [W][CODE] ~30m: Convert `cmd_boot` to async + start the poller
  - Use `#[tokio::main]` on `main()` (currently sync). Wrap entry: `#[tokio::main] async fn main() -> Result<()>`.
  - Inside `cmd_boot`: spawn the poller, wait on `tokio::signal::ctrl_c()` OR halted-byte detection.

- [ ] **Step 80** [BUILD][B] ~1m: Build green
  ```bash
  cargo build -p upc-bootctl 2>&1 | tail -10
  ```

- [ ] **Step 81** [V] ~10s: Build clean

- [ ] **Step 82** [C] ~5s: Commit Rust-side poller
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 78-82: Phase 5 — TTY poller wired to upc-tty-bridge"
  ```

- [ ] **Step 83** [B] ~10s: Start upc-tty-bridge in background (terminal A)
  ```bash
  ./bin/upc-tty-bridge 2>&1 > /tmp/upc-tty-bridge.log &
  echo $! > /tmp/upc-tty-bridge.pid
  sleep 1 && curl -s http://127.0.0.1:26100/api/v1/health
  ```

- [ ] **Step 84** [B] ~5s: Open browser to upc-console.html
  ```bash
  echo "Open in browser: file://$(pwd)/dashboard/upc-console.html?instance=222"
  ```
  - Or via the bridge's static file serve if implemented.

- [ ] **Step 85** [S][B] ~30s: **THE MOMENT** — boot xv6 with TTY pipeline live
  ```bash
  sudo -E env "PATH=$PATH" cargo run -p upc-bootctl -- boot --kernel crates/xv6-mbc/build/xv6-mbc.mbc --instance 222 2>&1 | tee /tmp/upc-boot-with-tty.log
  ```

- [ ] **Step 86** [V][C] ~30s: **PHASE 5 EXIT GATE — "xv6 booting..." VISIBLE IN BROWSER**
  - Browser xterm.js renders the boot banner
  - `/tmp/upc-tty-bridge.log` shows "broadcasting N bytes to subscriber instance=222"
  - Backup verification (if browser unavailable): `curl -s http://127.0.0.1:26100/api/v1/tty/snapshot?instance=222 | head` shows the bytes
  ```bash
  kill $(cat /tmp/upc-tty-bridge.pid) 2>/dev/null
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 83-86: Phase 5 — xv6 boot banner reaches browser xterm.js"
  ```
  - If pass → Phase 6
  - If "xv6 booting..." NOT visible → STUCK + ESCALATE; this is the Phase 1.1 ship-blocker.
  - **Rollback plan**: `kill $(cat /tmp/upc-tty-bridge.pid)`, kill the upc-bootctl process, `sudo rm -rf /sys/fs/bpf/unheaded/upc-xv6/*`, `sudo ip netns del upc0`. Restart from Step 83.

---

## PHASE 6: HALT CLEANUP + SOAK (Steps 87-94)

**Goal**: Verify the cleanup contract — after upc-bootctl exits (Ctrl-C OR xv6 halts), `CPU_MAP[0xDE]` is removed, `SCREEN_MAP` zeroed, `KBD_MAP` cleared. No stale state for the next boot.
**Prerequisite**: Phase 5 GATE passed.
**Time**: 1-2 hours
**Agent**: Coordinator (Developer)

- [ ] **Step 87** [W][CODE] ~30m: Implement halt-detection loop in `cmd_boot`
  - Add (alongside the TTY poller in Step 79):
    ```rust
    // Watch for halt OR ctrl-c
    let halt_check = async {
        loop {
            tokio::time::sleep(std::time::Duration::from_millis(100)).await;
            let cpu = runner.lock().unwrap().cpu_state()?;
            if cpu.halted != 0 { tracing::info!(exit_code = cpu.exit_code, "kernel halted"); break Ok(()); }
        }
    };
    tokio::select! {
        _ = tokio::signal::ctrl_c() => { tracing::info!("received SIGINT"); }
        r = halt_check => { r?; }
    }
    runner.lock().unwrap().cleanup()?;
    ```

- [ ] **Step 88** [BUILD][B] ~30s: Build green
  ```bash
  cargo build -p upc-bootctl 2>&1 | tail -5
  ```

- [ ] **Step 89** [V] ~10s: Build clean

- [ ] **Step 90** [S][B] ~1m: **HALT BEHAVIOR TEST** — start, observe banner, ctrl-c
  - Terminal A: `./bin/upc-tty-bridge`
  - Terminal B:
    ```bash
    sudo -E env "PATH=$PATH" cargo run -p upc-bootctl -- boot --kernel crates/xv6-mbc/build/xv6-mbc.mbc --instance 222 &
    sleep 5  # let it boot + render banner
    kill -INT %1
    wait
    ```

- [ ] **Step 91** [V][S][B] ~30s: **CLEANUP VERIFIED** — CPU_MAP no longer has 0xDE entry
  ```bash
  sudo bpftool map dump name CPU_MAP 2>&1 | grep -c "0xde\|0xDE"
  ```
  - Expected: 0 (entry was removed)
  - If 1 → STUCK; cleanup didn't fire on SIGINT path

- [ ] **Step 92** [S][B] ~30s: 3-second instance-halt soak (5 iterations)
  ```bash
  for i in 1 2 3 4 5; do
    sudo -E env "PATH=$PATH" timeout 3 cargo run -p upc-bootctl -- boot --kernel crates/xv6-mbc/build/xv6-mbc.mbc --instance 222 >> /tmp/soak.log 2>&1 || true
    sleep 0.5
    n=$(sudo bpftool map dump name CPU_MAP 2>&1 | grep -c "0xde\|0xDE")
    echo "iteration $i: stale CPU_MAP entries = $n"
    if [ "$n" -ne 0 ]; then echo "FAIL"; break; fi
  done
  ```

- [ ] **Step 93** [V] ~10s: Soak result — all 5 iterations clean (0 stale entries)
  - If any iteration leaves stale state → STUCK + ESCALATE

- [ ] **Step 94** [V][C] ~10s: **PHASE 6 EXIT GATE**
  - SIGINT → cleanup confirmed (Step 91)
  - 5-iteration soak clean (Step 93)
  - doom-runner regression sanity (no shared-state bleed):
    ```bash
    cargo test -p doom-runner --lib 2>&1 | tail -3
    ```
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 87-94: Phase 6 — halt cleanup verified, 5-iter soak clean"
  ```
  - If pass → Phase 7
  - **Rollback plan**: revert Step 87's cleanup logic; bisect with `git log --oneline ce4e4fda..HEAD -- cmd/upc-bootctl/`.

---

## PHASE 7: PHASE 1.1 GATE EVAL + DOC PROPAGATION (Steps 95-104)

**Goal**: Run the formal Phase 1.1 acceptance per parent battle plan §2; update timeline + docs; close the sprint.
**Prerequisite**: Phase 6 GATE passed.
**Time**: 1 hour
**Agent**: Coordinator (Developer + Marshal eval)

- [ ] **Step 95** [TEST][B] ~3m: Full Go regression
  ```bash
  go test -short -count=1 ./... 2>&1 | grep -cE "^FAIL"
  ```
  - Expected: 0. Compare to Phase 0 baseline at `/tmp/baseline-go-*.log`.

- [ ] **Step 96** [TEST][B] ~3m: Full Rust regression
  ```bash
  make test-rust 2>&1 | tail -5
  ```
  - Expected: all pass. Compare to Phase 0 baseline.

- [ ] **Step 97** [B] ~30s: Verifier budget delta
  ```bash
  bash scripts/bpf-verifier-check.sh 2>&1 | tail -5
  ```
  - Compare to Phase 0 (Step 8). Expected: same or within 1% (we didn't touch eBPF programs).

- [ ] **Step 98** [V] ~5m: **PHASE 1.1 GATE per parent plan §2**
  - [ ] xv6 .mbc image emits clean (re-validated): `cargo run -p upc-bootctl -- validate --kernel crates/xv6-mbc/build/xv6-mbc.mbc | grep "structurally valid"`
  - [ ] First boot: CPU_MAP populated + PC advances OR Step 71 STUCK with downstream OK
  - [ ] "xv6 booting..." VISIBLE in browser xterm.js (Phase 5 gate)
  - [ ] HALT cleanup leaves no stale BPF state (Phase 6 gate)
  - [ ] 5-iteration soak clean
  - [ ] Zero Go regressions, zero Rust regressions

- [ ] **Step 99** [DOC-UPDATE][W] ~10m: Update parent battle plan with Phase 1.1 SHIP marker
  - File: `references/battle-plan-ascend-linux-2026-05-08.md`
  - Append a "Phase 1.1 SHIPPED 2026-05-XX" subsection right under the Phase 1 row, citing this plan + the gate-eval commit hash.

- [ ] **Step 100** [DOC-UPDATE][W] ~5m: Update CLAUDE.md ASCEND-LINUX section
  - File: `CLAUDE.md`
  - Edit the "Phase 1.1 (~80% complete)" line to "**Phase 1.1 SHIPPED**" with date + cite this plan.

- [ ] **Step 101** [DOC-UPDATE][W] ~5m: Update timeline.md
  - File: `references/timeline.md`
  - Add an entry: "2026-05-XX: ASCEND-LINUX Phase 1.1 SHIPPED — first xv6 boot in browser xterm via upc-bootctl live BPF integration (~3 days, planned)."

- [ ] **Step 102** [W] ~15m: Write the post-sprint shift report
  - File: `references/marshal-shift-2026-05-XX-phase11-ship.md`
  - Include: phase-by-phase outcomes, any STUCK markers + their dispositions, the parking-lot items this sprint surfaced, the Phase 2 (uClinux) opening hand-off (start with `crates/upc-bootstub/` since this sprint deferred it).

- [ ] **Step 103** [REGEN][B] ~10s: Regenerate timeline mirrors if applicable
  ```bash
  ls references/timeline.json references/timeline.yaml 2>/dev/null && echo "Run: scripts/regenerate-timeline-mirrors.sh"
  ```

- [ ] **Step 104** [V][C] ~10s: **PHASE 7 EXIT GATE = PHASE 1.1 SHIP GATE**
  - All 6 GATE conditions in Step 98 pass
  - 4 documentation updates committed
  - Shift report written
  ```bash
  git add -A && git commit --no-gpg-sign -m "[PLAN P1.1] Steps 95-104: Phase 1.1 SHIPPED — first xv6 boot in browser xterm

GATE PASSED:
- xv6.mbc validated, 11_721 instructions
- BPF maps populated live via upc-bootctl::BootRunner
- 'xv6 booting...' rendered in browser xterm.js via TTY pipeline
- SIGINT cleanup leaves no stale CPU_MAP/SCREEN_MAP/KBD_MAP entries
- 5-iteration soak clean, 0 regressions Go+Rust

ASCEND-LINUX Phase 1.1 ships. Next: Phase 2 uClinux nommu (requires
crates/upc-bootstub/ which this sprint deferred per Step 16 decision)."
  ```
  - If pass → SPRINT COMPLETE. Marshal hands off to Round Table for Phase 2 planning.
  - **Rollback plan**: if any GATE condition fails, do NOT ship the marker. STUCK + ESCALATE; the parking lot below captures what to retry next.

---

## PARKING LOT — items this sprint surfaces but does not execute

These will accumulate across the sprint and feed the next Round Table.

### [P-LOT-1] crates/upc-bootstub/ stage-1 stub
**Captured during**: Phase 1 Step 16 [DECIDE].
xv6 doesn't need it. uClinux Phase 2 will. Author the crate + boot.S + linker script + MBC translation in Phase 2 kickoff.
**Owner**: Computermancer + Developer.
**Estimate**: 1-2 days.

### [P-LOT-2] Wotan-topic TTY transport
**Captured during**: Phase 1 Step 18 [DECIDE].
HTTP loopback works for the demo. Wotan-topic transport (`compute.tty.{instance}`) is cleaner for multi-process setups. Migrate when Wotan becomes a hard dependency anyway (Phase 3+).
**Owner**: Architect + Developer.
**Estimate**: 4-6 hours.

### [P-LOT-3] BPF verifier-budget revalidation after first kernel iteration
**Captured during**: Phase 0 Step 8 (baseline) + Phase 7 Step 97 (delta).
The current sprint doesn't touch eBPF programs, so budget shouldn't move. But Phase 2's uClinux interpreter changes WILL — the first iteration with new opcodes is the right time to take a fresh measurement and decide whether the EBPF-CLIPPY-119 mass autofix is now safe.
**Owner**: BlackMage (verifier-aware) + Developer.
**Estimate**: 30 min eval + N hours triage based on result.

### [P-LOT-4] upc-tty-bridge `/api/v1/tty/snapshot` endpoint
**Captured during**: Phase 5 Step 86 [V] backup verification mention.
Currently the bridge fan-outs live bytes only. A snapshot endpoint (last 4 KB ring buffer) would help debugging when the WebSocket disconnects mid-session. ~30 min addition.
**Owner**: Developer.

### [P-LOT-5] Doom-runner-style `status` subcommand
**Captured during**: Phase 4 Step 69 (had to use bpftool directly).
Add `upc-bootctl status --instance 0xDE` that prints the current CPU_MAP entry as a formatted struct. ~30 min addition; replaces ad-hoc `bpftool map dump` in debug branches.
**Owner**: Developer.

### [P-LOT-6] Decision queries Q2-Q5 (deferred from Round Table)
- Q2: 2362-lint mass cleanup tier
- Q3: EBPF-CLIPPY-119 verifier-budget gate
- Q4: C4 heimdall 4 architectural decisions
- Q5: D4-D6 zhend roadmap intent
All explicitly deferred. Re-raise at next Round Table.

---

## APPENDIX A: EMERGENCY PROCEDURES

### A1: BPF map collision (stale state from prior aborted boot)
```bash
sudo rm -rf /sys/fs/bpf/unheaded/upc-xv6/*
sudo bpftool prog list | grep monad_cpu | awk '{print $1}' | sed 's/://' | xargs -I{} sudo bpftool prog detach id {}
sudo ip netns del upc0 2>/dev/null
```
Then re-run from Phase 4 Step 65.

### A2: XDP attach fails with EINVAL
- Common cause: kernel < 5.15 OR XDP-generic mode not supported on the iface.
- Fix: `prog.attach(iface, aya::programs::XdpFlags::SKB_MODE)` (force SKB-mode in code).

### A3: TTY poller never sees bytes
- Verify the eBPF interpreter actually intercepts MMIO 0xC001 writes:
  ```bash
  sudo bpftool prog tracelog | grep -i "tty\|0xc001" | head -10
  ```
- Verify console-mmio.c symbols exist in xv6.mbc: `nm crates/xv6-mbc/build/kernel.elf | grep uart`

### A4: Browser xterm shows nothing but bridge logs ingest events
- Open browser DevTools → Network → filter WS. Check the WS frames are arriving.
- Common cause: `?instance=222` query param missing — bridge defaults to 0xDE which IS 222 so should be fine, but explicit is safer.

### A5: Halt-cleanup leaves CPU_MAP entry
- Symptom: Step 91 shows count > 0 after SIGINT.
- Likely cause: `runner.lock()` held by the TTY poller when SIGINT fires; cleanup blocks.
- Fix: shutdown TTY poller via the watch channel BEFORE acquiring runner lock for cleanup.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Owner | Time | Parallel? | Dependencies |
|-------|-------|------|-----------|--------------|
| 0 PREFLIGHT | Coordinator | 30m | No | — |
| 1 STAGE-1 DECISION | Coordinator + Computermancer | 1h | No | Phase 0 |
| 2 LIFT AYA PATTERN | Developer | 4-6h | No | Phase 1 |
| 3 BOOTPARAMS V2 | Developer + Computermancer | 3-4h | No | Phase 2 |
| 4 FIRST BOOT | Coordinator | 4-6h | No | Phase 3 |
| 5 TTY PIPELINE | Developer (cross-language) | 3-4h | No | Phase 4 (Phase 4 STUCK is OK) |
| 6 HALT CLEANUP | Developer | 1-2h | No | Phase 5 |
| 7 GATE EVAL | Coordinator + Marshal | 1h | No | Phase 6 |

**Critical path**: Phase 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7. **No parallelism possible** — every phase depends on the prior phase's BPF map state or build artifacts. Total wall-clock: ~18-26 hours of focused work over 3-4 working days.

---

## APPENDIX C: QUICK REFERENCE

### MBC instruction encoding
`[opcode:8][dst:4][src:4][imm16:16]` packed into one little-endian u32 word. `cargo run -p upc-bootctl -- validate` decodes the first 32 instructions.

### MbcCpuState (ABI v2, 136 bytes)
See `crates/doom-runner/src/main.rs:22-50` and Step 24 of this plan. Key fields for Phase 1.1: `pc` (word index, NOT byte addr), `regs[15]` = SP (byte addr), `priv_level` (0=M, 1=S, 3=U), `halted` (1 = HLT executed), `reservation_address` (0xFFFF_FFFF = no LR reservation).

### BootParams v2 (256 B exact)
See `cmd/upc-bootctl/src/bootparams.rs` (created Step 43). Magic = 0x554E_4844, version = 2. For xv6: bss_start=bss_end=0, num_cpus=1, kernel_addr=0x10000.

### Memory map (Boot Protocol v2)
- 0x0000-0x03FF: IVT (256 vectors × 4 B)
- 0x0100-0x01FF: BootParams v2
- 0x0200-0x03FF: cmdline (≤512 B)
- 0xF000-0xF0FF: CSR region (zero-filled by bootloader)
- 0x10000+: kernel image (xv6 entry = start_mbc.c)
- 0x800000+: ramdisk (none for xv6 first boot)
- SP top: 0x03F0_0000

### BPF maps (monad-cpu-ebpf)
- `ROM_MAP` (Array<u32>, 262_144 entries): MBC instructions, indexed by PC word
- `RAM_MAP` (Array<u32>, 16_777_216 entries): RAM word-addressed
- `CPU_MAP` (HashMap<u32, MbcCpuState>): instance ID → CPU state
- `TTY_MAP` (Array<u8>, 4096 entries): circular byte buffer
- `TTY_HEAD` (Array<u32>, 1 entry): write head position into TTY_MAP

### Useful one-liners
```bash
# Inspect xv6.mbc structure
cargo run -p upc-bootctl -- validate --kernel crates/xv6-mbc/build/xv6-mbc.mbc

# Live boot (after this sprint ships)
sudo -E env "PATH=$PATH" cargo run -p upc-bootctl -- boot --kernel crates/xv6-mbc/build/xv6-mbc.mbc --instance 222

# TTY snapshot via curl
curl -s http://127.0.0.1:26100/api/v1/tty/snapshot?instance=222

# Manual CPU_MAP inspection
sudo bpftool map dump name CPU_MAP

# BPF verifier budget snapshot
bash scripts/bpf-verifier-check.sh
```

---

*Phase 1.1 SHIP Battle Plan — Forged 2026-05-10*
*8 Phases. 104 Steps. xv6 booting on UPC, watch it in your browser.*
*The Linux ascent begins with one boot banner.*
