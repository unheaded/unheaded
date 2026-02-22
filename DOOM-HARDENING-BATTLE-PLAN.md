# DOOM-OVER-IPv6 HARDENING BATTLE PLAN — 10 Phases, 180+ Steps

**Date**: 2026-02-22
**Sprint**: S-DOOM-HARDEN — Secure, harden, and productionize Doom-over-IPv6 dev tooling
**Prerequisite**: Doom ring operational (Phase 7 confirmed), all 6 namespaces UP, XDP attached
**Target**: All dev tooling migrated from /tmp to repo, /tmp noexec, ROM integrity enforced, automated test gate, commit-per-step discipline
**Estimated Duration**: 8-12 hours across 2-3 sessions
**Agent Strategy**: Phases 0-2 sequential (environment), Phases 3-5 parallelizable (hardening), Phases 6-8 sequential (integration), Phase 9-10 sequential (validation + discipline)

---

## LEGEND

```
[B]  = Bash command (run directly)
[V]  = Verification step (MUST pass before proceeding)
[D]  = Debug step (only if prior step fails)
[W]  = Write/create file
[R]  = Read/inspect file
[S]  = Sudo required
[P]  = Parallelizable with other marked steps
[G]  = Git commit gate (commit after this step per Muck's directive)
```

---

## DEPENDENCY MAP

```
Phase 0 (Env Verify)
  │
  ▼
Phase 1 (/tmp Hardening + noexec)
  │
  ▼
Phase 2 (Tooling Migration /tmp → repo)
  │
  ├──────────────────┬──────────────────┐
  ▼                  ▼                  ▼
Phase 3            Phase 4           Phase 5
(ROM Integrity)    (CRC Validation)  (Flow Label)
  │                  │                  │
  └──────────────────┴──────────────────┘
                     │
                     ▼
Phase 6 (Automated Doom Test Gate)
  │
  ▼
Phase 7 (Wire Format Alignment)
  │
  ▼
Phase 8 (Documentation + Artifact Versioning)
  │
  ▼
Phase 9 (Commit Discipline Enforcement)
  │
  ▼
Phase 10 (Final Verification + Exit Gate)
```

**Critical Path**: 0 → 1 → 2 → 3 → 6 → 7 → 8 → 10

---

## PHASE 0: ENVIRONMENT VERIFICATION (Steps 1-18)

**Goal**: Confirm dev box state, catalog /tmp artifacts, verify ring is operational.
**Prerequisite**: SSH access to dev box, sudo privileges.
**Time**: 15 minutes
**Agent**: Coordinator

### Catalog Current State

- [ ] **Step 1** [B]: Record kernel version and architecture
  ```bash
  uname -r && uname -m
  ```

- [ ] **Step 2** [V]: Kernel >= 5.15 with eBPF+XDP support
  - If pass → Step 3
  - If fail → STOP. Doom requires modern kernel.

- [ ] **Step 3** [B]: Record current /tmp contents with timestamps
  ```bash
  ls -la --time-style=long-iso /tmp/*.py /tmp/*.asm /tmp/*.mbc /tmp/doom-*.txt 2>/dev/null | tee ~/unheaded/tmp-artifact-inventory.txt
  ```

- [ ] **Step 4** [B]: Verify ring is still operational
  ```bash
  sudo ip netns list | grep monad | sort
  ```

- [ ] **Step 5** [V]: All 6 namespaces present (monad0-monad5)
  - If pass → Step 6
  - If fail → Run `sudo ./scripts/doom-ring.sh setup` to rebuild ring

- [ ] **Step 6** [B]: Verify XDP programs attached
  ```bash
  for ns in monad{0..5}; do
    echo "=== $ns ==="
    sudo ip netns exec $ns ip link show | grep -A1 xdp
  done
  ```

- [ ] **Step 7** [V]: All 6 XDP attachments present
  - If pass → Step 8
  - If fail → Check `sudo bpftool prog list` for loaded programs

- [ ] **Step 8** [B]: Verify BPF maps pinned
  ```bash
  ls -la /sys/fs/bpf/unheaded/doom-ring/maps/
  ```

- [ ] **Step 9** [V]: ROM_MAP, CPU_MAP, RAM_MAP, SCREEN_MAP, KBD_MAP, STATS, L1_CACHE, COMPUTE_EVENTS all pinned
  - If pass → Step 10
  - If fail → Ring needs reload. Run `sudo ./scripts/doom-ring.sh teardown && sudo ./scripts/doom-ring.sh setup`

- [ ] **Step 10** [B]: Record current CPU state for forensic baseline
  ```bash
  sudo python3 /tmp/read_cpu.py | tee ~/unheaded/doom-cpu-baseline-$(date +%Y%m%d-%H%M%S).txt
  ```

- [ ] **Step 11** [B]: Check current fstab for /tmp mount options
  ```bash
  grep '/tmp' /etc/fstab || echo "No /tmp entry in fstab"
  mount | grep '/tmp' || echo "No separate /tmp mount"
  ```

- [ ] **Step 12** [R]: Record current /tmp mount type (tmpfs? ext4? part of root?)
  ```bash
  df -h /tmp && findmnt /tmp 2>/dev/null || echo "/tmp is part of root filesystem"
  ```

- [ ] **Step 13** [B]: Check available disk space for ~/unheaded/tmp
  ```bash
  df -h ~ && du -sh ~/unheaded/ 2>/dev/null || echo "~/unheaded not yet created"
  ```

- [ ] **Step 14** [B]: Verify git repo status
  ```bash
  cd ~/unheaded && git status --short && git log --oneline -5
  ```

- [ ] **Step 15** [B]: Check required tools
  ```bash
  for tool in ip bpftool python3 cargo go bpf_asm; do
    which $tool 2>/dev/null && echo "$tool: OK" || echo "$tool: MISSING"
  done
  ```

- [ ] **Step 16** [V]: Python3, bpftool, ip present. cargo and go present for builds.
  - If pass → Step 17
  - If fail → Install missing: `sudo apt-get install -y iproute2 linux-tools-$(uname -r) python3`

- [ ] **Step 17** [B]: Snapshot /tmp Python files for diff after migration
  ```bash
  mkdir -p ~/unheaded/tmp-migration-backup
  cp /tmp/*.py ~/unheaded/tmp-migration-backup/ 2>/dev/null || true
  cp /tmp/*.asm ~/unheaded/tmp-migration-backup/ 2>/dev/null || true
  cp /tmp/*.mbc ~/unheaded/tmp-migration-backup/ 2>/dev/null || true
  cp /tmp/doom-*.txt ~/unheaded/tmp-migration-backup/ 2>/dev/null || true
  ```

- [ ] **Step 18** [V]: **PHASE 0 EXIT GATE** — Environment cataloged, ring operational, backup taken
  ```bash
  test -d ~/unheaded/tmp-migration-backup && \
  sudo ip netns list | grep -c monad | grep -q 6 && \
  ls /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP >/dev/null 2>&1 && \
  echo "PHASE 0: PASS" || echo "PHASE 0: FAIL"
  ```
  - If pass → Phase 1
  - If fail → DO NOT PROCEED. Debug within this phase.

- [ ] **Step 18a** [G]: Commit backup artifacts
  ```bash
  cd ~/unheaded && git add tmp-migration-backup/ && \
  git commit -m "chore(doom): snapshot /tmp artifacts before migration

  Backup of all ad-hoc /tmp development files created during Doom-over-IPv6
  Phase 4-7 development. These will be migrated to scripts/doom/ and hardened.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

---

## PHASE 1: /tmp FILESYSTEM HARDENING (Steps 19-38)

**Goal**: Set /tmp to noexec,nosuid,nodev. Create ~/unheaded/tmp/ as the project scratch space.
**Prerequisite**: Phase 0 passed, backup taken.
**Time**: 30 minutes
**Agent**: Coordinator (requires sudo)

### Create Project Scratch Directory

- [ ] **Step 19** [B]: Create project-local tmp directory structure
  ```bash
  mkdir -p ~/unheaded/tmp/{doom,build,test,artifacts}
  ```

- [ ] **Step 20** [W]: Create README for the tmp directory
  ```bash
  cat > ~/unheaded/tmp/README.md << 'TMPEOF'
  # ~/unheaded/tmp/ — Project Scratch Space

  **DO NOT use /tmp/ for project files.** /tmp is mounted noexec.

  ## Directory Structure

  - `doom/` — Doom-over-IPv6 runtime artifacts (CPU state dumps, metrics, ring state)
  - `build/` — Build artifacts, intermediate compilation outputs
  - `test/` — Test output, coverage reports, benchmark results
  - `artifacts/` — Timestamped snapshots for forensics and debugging

  ## Naming Convention

  All artifact files MUST include timestamp and git SHA:
  ```
  {description}-{YYYYMMDD-HHMMSS}-{git-sha-short}.{ext}
  ```

  Example: `doom-cpu-state-20260222-143022-a1b2c3d.txt`

  ## Cleanup Policy

  Files older than 7 days are candidates for cleanup.
  Run: `find ~/unheaded/tmp -mtime +7 -type f -delete`
  TMPEOF
  ```

- [ ] **Step 21** [B]: Set appropriate permissions
  ```bash
  chmod 750 ~/unheaded/tmp
  chmod 750 ~/unheaded/tmp/{doom,build,test,artifacts}
  ```

- [ ] **Step 22** [G]: Commit project tmp directory structure
  ```bash
  cd ~/unheaded && git add tmp/ && \
  git commit -m "feat(infra): create ~/unheaded/tmp/ project scratch space

  Replaces ad-hoc /tmp usage with structured project-local scratch directory.
  Includes doom/, build/, test/, artifacts/ subdirs with naming conventions.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Harden /tmp Filesystem

- [ ] **Step 23** [B][S]: Check if /tmp is a separate mount or part of root
  ```bash
  findmnt /tmp 2>/dev/null && echo "SEPARATE MOUNT" || echo "PART OF ROOT FS"
  ```

- [ ] **Step 24** [B][S]: **IF /tmp is already a separate mount** — remount with noexec
  ```bash
  sudo mount -o remount,noexec,nosuid,nodev /tmp
  ```

- [ ] **Step 25** [B][S]: **IF /tmp is part of root filesystem** — create tmpfs mount
  ```bash
  # Add tmpfs entry to fstab
  echo 'tmpfs /tmp tmpfs defaults,noexec,nosuid,nodev,size=2G 0 0' | sudo tee -a /etc/fstab
  ```

- [ ] **Step 26** [D]: If /tmp has active processes using it, identify them
  ```bash
  sudo lsof +D /tmp 2>/dev/null | head -20
  ```

- [ ] **Step 27** [B][S]: Mount the new tmpfs (if added to fstab)
  ```bash
  sudo mount -o noexec,nosuid,nodev /tmp 2>/dev/null || echo "Already mounted or using remount"
  ```

- [ ] **Step 28** [V]: Verify /tmp is noexec
  ```bash
  mount | grep '/tmp' | grep noexec && echo "NOEXEC: VERIFIED" || echo "NOEXEC: FAILED"
  ```
  - If pass → Step 29
  - If fail → Check fstab entry, retry mount

- [ ] **Step 29** [B]: Test that noexec actually prevents execution
  ```bash
  echo '#!/bin/bash' > /tmp/test-noexec.sh && \
  echo 'echo SHOULD_NOT_SEE_THIS' >> /tmp/test-noexec.sh && \
  chmod +x /tmp/test-noexec.sh && \
  /tmp/test-noexec.sh 2>&1 | grep -q "Permission denied" && \
  echo "NOEXEC TEST: PASS — execution blocked" || \
  echo "NOEXEC TEST: FAIL — scripts can still execute from /tmp"
  rm -f /tmp/test-noexec.sh
  ```

- [ ] **Step 30** [V]: noexec blocks script execution from /tmp
  - If pass → Step 31
  - If fail → Check mount options: `findmnt -o OPTIONS /tmp`

- [ ] **Step 31** [D]: **IMPORTANT** — Python scripts invoked as `python3 /tmp/script.py` still work with noexec because Python interprets the file, it doesn't execute it directly. To block this pattern:
  ```bash
  # Verify: python3 can still read files from /tmp (expected — noexec blocks exec bit, not read)
  echo 'print("python can read /tmp")' > /tmp/test-python.py
  python3 /tmp/test-python.py 2>&1
  rm -f /tmp/test-python.py
  # NOTE: This WILL still work. noexec prevents direct ELF execution, not interpretation.
  # Full mitigation requires migrating scripts OUT of /tmp entirely (Phase 2).
  ```

- [ ] **Step 32** [R]: Document the noexec limitation for the team
  ```bash
  echo "NOTE: noexec on /tmp blocks direct ELF execution and shebang scripts."
  echo "Python/Perl scripts invoked via interpreter still work."
  echo "Full mitigation: migrate all scripts to ~/unheaded/scripts/doom/"
  echo "noexec still protects against: malware dropping ELF binaries, shell scripts with exec bit"
  ```

### Harden Additional tmpfs Mounts

- [ ] **Step 33** [B][S]: Apply noexec to /dev/shm if not already set
  ```bash
  mount | grep '/dev/shm' | grep noexec || \
  sudo mount -o remount,noexec,nosuid,nodev /dev/shm
  ```

- [ ] **Step 34** [B][S]: Verify /var/tmp hardening
  ```bash
  mount | grep '/var/tmp' && echo "Separate mount" || echo "/var/tmp is part of root (consider hardening)"
  ```

- [ ] **Step 35** [V]: Verify all tmp-like mounts are hardened
  ```bash
  echo "=== Mount Hardening Status ==="
  for mp in /tmp /dev/shm /var/tmp; do
    opts=$(findmnt -n -o OPTIONS "$mp" 2>/dev/null || echo "not-separate-mount")
    echo "$mp: $opts"
  done
  ```

- [ ] **Step 36** [B]: Add systemd-tmpfiles rule to auto-clean project scratch
  ```bash
  cat > ~/unheaded/configs/tmp-cleanup.conf << 'EOF'
  # Clean unheaded tmp artifacts older than 7 days
  # Install to /etc/tmpfiles.d/ for systemd-tmpfiles integration
  d /home/admin/unheaded/tmp/doom 0750 admin admin 7d
  d /home/admin/unheaded/tmp/build 0750 admin admin 3d
  d /home/admin/unheaded/tmp/test 0750 admin admin 3d
  d /home/admin/unheaded/tmp/artifacts 0750 admin admin 30d
  EOF
  ```

- [ ] **Step 37** [G]: Commit /tmp hardening config
  ```bash
  cd ~/unheaded && git add configs/tmp-cleanup.conf && \
  git commit -m "feat(security): add tmpfiles cleanup config for project scratch space

  Automated cleanup: doom/ 7d, build/ 3d, test/ 3d, artifacts/ 30d.
  Install to /etc/tmpfiles.d/ for systemd integration.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 38** [V]: **PHASE 1 EXIT GATE** — /tmp is noexec, project scratch exists
  ```bash
  mount | grep '/tmp.*noexec' >/dev/null 2>&1 && \
  test -d ~/unheaded/tmp/doom && \
  test -d ~/unheaded/tmp/build && \
  echo "PHASE 1: PASS" || echo "PHASE 1: FAIL"
  ```
  - If pass → Phase 2
  - If fail → DO NOT PROCEED. Debug mount options.

---

## PHASE 2: TOOLING MIGRATION — /tmp → REPO (Steps 39-72)

**Goal**: Migrate all /tmp Python scripts into `scripts/doom/`, harden them, and delete /tmp copies.
**Prerequisite**: Phase 1 passed, project scratch directory exists.
**Time**: 90 minutes
**Agent**: Coordinator

### Create Doom Scripts Directory Structure

- [ ] **Step 39** [B]: Create scripts/doom/ directory structure
  ```bash
  mkdir -p ~/unheaded/scripts/doom/{tools,tests,artifacts}
  ```

- [ ] **Step 40** [W]: Create scripts/doom/README.md
  ```bash
  cat > ~/unheaded/scripts/doom/README.md << 'DOOMEOF'
  # Doom-over-IPv6 Development Tools

  Tooling for the Doom-over-IPv6 packet-driven CPU substrate.

  ## Tools

  | Script | Purpose | Requires sudo |
  |--------|---------|---------------|
  | `inject.py` | Packet injection (single/bulk) | Yes (AF_PACKET) |
  | `load_rom.py` | Load MBC binary into ROM_MAP via BPF syscall | Yes (BPF) |
  | `cpu_state.py` | Read/write/reset CPU state in CPU_MAP | Yes (BPF) |
  | `skip_crt0.py` | Accelerate CRT0 BSS clearing loops | Yes (BPF+AF_PACKET) |
  | `ring_status.py` | Report ring topology and BPF map status | Yes (netns) |

  ## Test ROMs

  Assembly source and compiled MBC binaries in `tests/`:
  - ISA validation suite (bitwise, branches, call/ret, memory, stack, screen, etc.)
  - Each test has expected register values documented in ASM comments

  ## Artifact Naming

  All output files use: `{name}-{YYYYMMDD-HHMMSS}-{git-sha}.{ext}`

  ## Usage

  ```bash
  # Load a test ROM and verify
  sudo python3 scripts/doom/load_rom.py tests/test-fibonacci.mbc
  sudo python3 scripts/doom/inject.py --flow-label 0xDE --count 100
  sudo python3 scripts/doom/cpu_state.py --read --instance 0xDE
  ```
  DOOMEOF
  ```

- [ ] **Step 41** [G]: Commit directory structure
  ```bash
  cd ~/unheaded && git add scripts/doom/ && \
  git commit -m "feat(doom): create scripts/doom/ directory structure

  Structured home for all Doom-over-IPv6 development tooling.
  Replaces ad-hoc /tmp script usage.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Migrate & Harden: load_rom_fast.py → load_rom.py

- [ ] **Step 42** [B]: Copy load_rom_fast.py as the canonical loader
  ```bash
  cp /tmp/load_rom_fast.py ~/unheaded/scripts/doom/load_rom.py
  ```

- [ ] **Step 43** [W]: Add error handling, argparse, and logging to load_rom.py
  - Replace bare `sys.argv` with argparse
  - Add `if __name__ == '__main__'` guard
  - Add proper exception handling around BPF syscalls
  - Add `--dry-run` flag for testing without BPF access
  - Add `--map-path` flag (default: `/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP`)
  - Add timestamp + git SHA to output messages
  - Remove /tmp hardcoded paths

- [ ] **Step 44** [V]: load_rom.py has argparse, error handling, no /tmp references
  ```bash
  grep -c 'argparse\|ArgumentParser' ~/unheaded/scripts/doom/load_rom.py && \
  ! grep -q '/tmp/' ~/unheaded/scripts/doom/load_rom.py && \
  echo "MIGRATION CHECK: PASS" || echo "MIGRATION CHECK: FAIL"
  ```

- [ ] **Step 45** [G]: Commit load_rom.py
  ```bash
  cd ~/unheaded && git add scripts/doom/load_rom.py && \
  git commit -m "feat(doom): migrate load_rom_fast.py → scripts/doom/load_rom.py

  Promoted from /tmp to repo. Added argparse, error handling, --dry-run,
  configurable map path. Direct BPF syscall approach (no subprocess per entry).
  Supports aarch64 and x86_64.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Migrate & Harden: Packet Injection

- [ ] **Step 46** [B]: Create unified inject.py from bulk_inject.py + inject_100.py
  ```bash
  cp /tmp/bulk_inject.py ~/unheaded/scripts/doom/inject.py
  ```

- [ ] **Step 47** [W]: Harden inject.py
  - Replace asserts with explicit `if/raise ValueError`
  - Add argparse (--flow-label, --count, --delay-us, --interface, --src-mac, --dst-mac)
  - Add signal handler for clean socket close on SIGINT
  - Add socket close in finally block
  - Add `--verify-crc` flag that computes and sets proper CRC-16 in the Monad register
  - Add `--trace-id` flag for explicit trace ID setting
  - Remove all hardcoded MAC addresses (require as args or read from interface)
  - Add `--dry-run` that prints packet hex without sending

- [ ] **Step 48** [V]: inject.py has no asserts in production path, has argparse
  ```bash
  ! grep -n '^[[:space:]]*assert ' ~/unheaded/scripts/doom/inject.py && \
  grep -q 'argparse' ~/unheaded/scripts/doom/inject.py && \
  echo "INJECT HARDENING: PASS" || echo "INJECT HARDENING: FAIL"
  ```

- [ ] **Step 49** [G]: Commit inject.py
  ```bash
  cd ~/unheaded && git add scripts/doom/inject.py && \
  git commit -m "feat(doom): migrate bulk_inject.py → scripts/doom/inject.py

  Unified injector replacing bulk_inject.py and inject_100.py. Adds argparse,
  signal handling, proper CRC-16 computation, socket cleanup, no asserts.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Migrate & Harden: CPU State Tools

- [ ] **Step 50** [B]: Merge read_cpu.py + reset_cpu.py into cpu_state.py
  ```bash
  cp /tmp/read_cpu.py ~/unheaded/scripts/doom/cpu_state.py
  ```

- [ ] **Step 51** [W]: Create unified cpu_state.py with subcommands
  - `cpu_state.py read --instance 0xDE` — read and parse CPU state
  - `cpu_state.py reset --instance 0xDE` — reset CPU to initial state
  - `cpu_state.py write --instance 0xDE --pc 0 --reg r0=42` — write specific fields
  - `cpu_state.py dump --instance 0xDE --output ~/unheaded/tmp/doom/` — dump to timestamped file
  - Add JSON output mode (`--json`) for machine consumption
  - Add proper struct parsing with named fields
  - Use BPF syscall directly (like load_rom_fast.py) instead of subprocess bpftool

- [ ] **Step 52** [V]: cpu_state.py subcommands work
  ```bash
  python3 ~/unheaded/scripts/doom/cpu_state.py --help 2>&1 | grep -q 'read\|reset\|write\|dump' && \
  echo "CPU_STATE SUBCOMMANDS: PASS" || echo "CPU_STATE SUBCOMMANDS: FAIL"
  ```

- [ ] **Step 53** [G]: Commit cpu_state.py
  ```bash
  cd ~/unheaded && git add scripts/doom/cpu_state.py && \
  git commit -m "feat(doom): migrate read_cpu.py + reset_cpu.py → cpu_state.py

  Unified CPU state tool with read/reset/write/dump subcommands.
  JSON output mode, direct BPF syscall, timestamped dumps.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Migrate & Harden: skip_loops.py

- [ ] **Step 54** [B]: Copy and rename
  ```bash
  cp /tmp/skip_loops.py ~/unheaded/scripts/doom/skip_crt0.py
  ```

- [ ] **Step 55** [W]: Harden skip_crt0.py
  - Add argparse (--instance, --max-skips, --inject-count, --verbose)
  - Add TimeoutExpired exception handling for subprocess calls
  - Add logging of every skip operation with timestamps
  - Use cpu_state.py module functions instead of duplicating bpftool subprocess calls
  - Add `--dry-run` that reports what it WOULD skip without modifying state
  - Document the CRT0 clearing loops it accelerates (BSS byte-clear at PC 103937-103945, word-clear at PC 20-28)

- [ ] **Step 56** [G]: Commit skip_crt0.py
  ```bash
  cd ~/unheaded && git add scripts/doom/skip_crt0.py && \
  git commit -m "feat(doom): migrate skip_loops.py → skip_crt0.py

  CRT0 BSS clearing loop accelerator. Detects clearing loops by PC range,
  fast-forwards register state. Added timeout handling, logging, dry-run.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Migrate Test ROMs

- [ ] **Step 57** [B]: Copy all assembly source and compiled MBC files
  ```bash
  cp /tmp/test-*.asm ~/unheaded/scripts/doom/tests/
  cp /tmp/test-*.mbc ~/unheaded/scripts/doom/tests/
  cp /tmp/trivial.mbc ~/unheaded/scripts/doom/tests/
  cp /tmp/nop10.mbc ~/unheaded/scripts/doom/tests/
  cp /tmp/nop100.mbc ~/unheaded/scripts/doom/tests/
  ```

- [ ] **Step 58** [V]: All test ROMs migrated
  ```bash
  echo "ASM files: $(ls ~/unheaded/scripts/doom/tests/*.asm 2>/dev/null | wc -l)"
  echo "MBC files: $(ls ~/unheaded/scripts/doom/tests/*.mbc 2>/dev/null | wc -l)"
  test $(ls ~/unheaded/scripts/doom/tests/*.asm | wc -l) -ge 12 && \
  echo "TEST ROM MIGRATION: PASS" || echo "TEST ROM MIGRATION: FAIL"
  ```

- [ ] **Step 59** [W]: Create test ROM manifest with expected results
  ```bash
  cat > ~/unheaded/scripts/doom/tests/MANIFEST.md << 'MANIFESTEOF'
  # Doom ISA Test ROM Manifest

  | Test | Instructions | Expected Results | Gate |
  |------|-------------|-----------------|------|
  | trivial.mbc | 4 | r0=43, r1=1, halted=1 | PASS |
  | nop10.mbc | 11 | PC=11, insn_count=10, halted=1 | PASS |
  | nop100.mbc | 101 | PC=101, insn_count=100, halted=1 | PASS |
  | test-fibonacci.mbc | ~33 | r0=55, halted=1 | PASS |
  | test-bitwise.mbc | ~13 | r2=0x0F00, r3=0xFF0F, r6=0x0FF0 | PASS |
  | test-branches.mbc | ~25 | r0=4 (all 4 branch tests pass) | PASS |
  | test-call-ret.mbc | ~5 | r0=42, r2=99, halted=1 | PASS |
  | test-comprehensive.mbc | ~20 | r0=42, r1=7, r2=2, r7=99 | PASS |
  | test-memory.mbc | 5 | r0=0xDEAD (57005), halted=1 | PASS |
  | test-stack.mbc | ~14 | r0=100, r1=200, r2=300 | PASS |
  | test-multitick.mbc | ~62 | r0=20, halted=1 | PASS |
  | test-screen.mbc | ~12 | r0=0xDEADBEEF, halted=1 | PASS |
  | test-screen2.mbc | ~10 | r0=0x55AA42FF, halted=1 | PASS |
  | test-imm32.mbc | ~7 | r0=0xDEADBEEF, r1=0xCAFE0000 | PASS |
  | test-keyboard.mbc | 3 | r0=keycode, halted=1 | PASS (env-dependent) |
  MANIFESTEOF
  ```

- [ ] **Step 60** [G]: Commit test ROMs and manifest
  ```bash
  cd ~/unheaded && git add scripts/doom/tests/ && \
  git commit -m "feat(doom): migrate ISA test ROM suite from /tmp

  12 assembly source files, 15 compiled MBC binaries, test manifest
  with expected register values for each test ROM.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Migrate Metrics/State Dumps

- [ ] **Step 61** [B]: Copy state dumps to artifacts directory with proper naming
  ```bash
  SHA=$(cd ~/unheaded && git rev-parse --short HEAD)
  cp /tmp/doom-metrics.txt ~/unheaded/tmp/artifacts/doom-metrics-20260221-${SHA}.txt
  cp /tmp/doom-ring-state.txt ~/unheaded/tmp/artifacts/doom-ring-state-20260221-${SHA}.txt
  cp /tmp/doom-state-phase7.txt ~/unheaded/tmp/artifacts/doom-state-phase7-20260221-${SHA}.txt
  cp /tmp/doom-stats-phase7.txt ~/unheaded/tmp/artifacts/doom-stats-phase7-20260221-${SHA}.txt
  ```

- [ ] **Step 62** [G]: Commit historical artifacts
  ```bash
  cd ~/unheaded && git add tmp/artifacts/ && \
  git commit -m "docs(doom): archive Phase 4-7 metrics and state dumps

  Historical forensic data from Doom-over-IPv6 development.
  6.6B instructions executed, 823M cache hits, 401 cache misses.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Clean /tmp

- [ ] **Step 63** [B]: Verify all files are migrated by diffing
  ```bash
  echo "=== Files in /tmp NOT yet in repo ==="
  for f in /tmp/*.py /tmp/*.asm /tmp/*.mbc /tmp/doom-*.txt; do
    base=$(basename "$f")
    if [ ! -f ~/unheaded/scripts/doom/"$base" ] && \
       [ ! -f ~/unheaded/scripts/doom/tests/"$base" ] && \
       [ ! -f ~/unheaded/tmp/artifacts/"$base"* ]; then
      echo "  NOT MIGRATED: $base"
    fi
  done
  ```

- [ ] **Step 64** [V]: All project files migrated from /tmp
  - If unmigrated files found → migrate them before proceeding
  - If all migrated → Step 65

- [ ] **Step 65** [B]: Remove project files from /tmp
  ```bash
  rm -f /tmp/bulk_inject.py /tmp/inject_100.py
  rm -f /tmp/load_rom.py /tmp/load_rom_fast.py
  rm -f /tmp/read_cpu.py /tmp/reset_cpu.py /tmp/skip_loops.py
  rm -f /tmp/test-*.asm /tmp/test-*.mbc
  rm -f /tmp/trivial.mbc /tmp/nop10.mbc /tmp/nop100.mbc
  rm -f /tmp/doom-*.txt
  ```

- [ ] **Step 66** [V]: /tmp is clean of project files
  ```bash
  ls /tmp/*.py /tmp/*.asm /tmp/*.mbc /tmp/doom-*.txt 2>/dev/null | wc -l | grep -q '^0$' && \
  echo "/tmp CLEANUP: PASS" || echo "/tmp CLEANUP: FAIL — stale files remain"
  ```

### Create .gitignore entries

- [ ] **Step 67** [W]: Add tmp/ artifacts to .gitignore (keep structure, ignore transient files)
  ```bash
  cat >> ~/unheaded/.gitignore << 'GIEOF'

  # Project scratch space (structure tracked, contents ignored)
  tmp/doom/*
  tmp/build/*
  tmp/test/*
  !tmp/artifacts/
  tmp/artifacts/*.tmp
  # Migration backup (one-time, remove after verified)
  tmp-migration-backup/
  GIEOF
  ```

- [ ] **Step 68** [G]: Commit .gitignore update
  ```bash
  cd ~/unheaded && git add .gitignore && \
  git commit -m "chore: update .gitignore for project scratch space

  Ignore transient files in tmp/doom, tmp/build, tmp/test.
  Track artifacts/ directory for forensic snapshots.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Update Existing Scripts

- [ ] **Step 69** [B]: Check if doom-ring.sh references /tmp anywhere
  ```bash
  grep -n '/tmp/' ~/unheaded/scripts/doom-ring.sh || echo "No /tmp references in doom-ring.sh"
  ```

- [ ] **Step 70** [W]: If /tmp references found, update to use scripts/doom/ or ~/unheaded/tmp/
  - Replace any `/tmp/inject_100.py` → `scripts/doom/inject.py --count 100`
  - Replace any `/tmp/read_cpu.py` → `scripts/doom/cpu_state.py read`
  - Replace any `/tmp/load_rom.py` → `scripts/doom/load_rom.py`

- [ ] **Step 71** [G]: Commit script path updates (if any changes made)
  ```bash
  cd ~/unheaded && git add scripts/ && \
  git diff --cached --quiet || \
  git commit -m "fix(doom): update script paths from /tmp to scripts/doom/

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 72** [V]: **PHASE 2 EXIT GATE** — All tooling in repo, /tmp clean, scripts reference repo paths
  ```bash
  test -f ~/unheaded/scripts/doom/load_rom.py && \
  test -f ~/unheaded/scripts/doom/inject.py && \
  test -f ~/unheaded/scripts/doom/cpu_state.py && \
  test -f ~/unheaded/scripts/doom/skip_crt0.py && \
  test $(ls ~/unheaded/scripts/doom/tests/*.asm | wc -l) -ge 12 && \
  ! ls /tmp/*.py /tmp/*.asm /tmp/*.mbc 2>/dev/null | grep -q '.' && \
  echo "PHASE 2: PASS" || echo "PHASE 2: FAIL"
  ```
  - If pass → Phase 3
  - If fail → DO NOT PROCEED.

---

## PHASE 3: ROM INTEGRITY — HMAC SIGNING (Steps 73-95) [P]

**Goal**: Add HMAC-SHA256 signature to ROM binaries. XDP verifies before loading.
**Prerequisite**: Phase 2 passed. ROM tooling migrated.
**Time**: 90 minutes
**Agent**: Agent [P] — can run parallel with Phases 4 and 5

### Design ROM Signing Format

- [ ] **Step 73** [W]: Define signed ROM format
  ```bash
  cat > ~/unheaded/docs/DOOM_ROM_SIGNING.md << 'SIGNEOF'
  # Doom ROM Signing Specification

  ## Problem

  ROM_MAP accepts arbitrary bytecode via BPF map update. No integrity verification.
  Any process with CAP_BPF can inject arbitrary instructions into the XDP pipeline.

  ## Solution

  HMAC-SHA256 signature appended to ROM binary. Loader verifies before writing to map.

  ## Signed ROM Format

  ```
  Offset    Size        Field
  0x00      4           Magic: "MBC\x01" (0x4D424301)
  0x04      4           ROM size in bytes (LE u32)
  0x08      4           Instruction count (LE u32)
  0x0C      2           CRC-16/CCITT of ROM data
  0x0E      2           Reserved (zero)
  0x10      32          HMAC-SHA256 of bytes 0x00-0x0F + ROM data
  0x30      N*4         ROM data (N instructions, 4 bytes each)
  ```

  ## Key Management

  - HMAC key stored at: `~/.unheaded/doom-rom-key` (0600 permissions)
  - Key generation: `python3 -c "import os; open('key','wb').write(os.urandom(32))"`
  - Key is 32 bytes (256-bit)
  - Key NEVER stored in git, NEVER in BPF maps, NEVER logged

  ## Verification Flow

  1. Loader reads signed ROM file
  2. Extracts header (0x00-0x2F) and ROM data (0x30+)
  3. Computes HMAC-SHA256 over header[0x00-0x0F] + ROM data
  4. Compares computed HMAC with header[0x10-0x2F]
  5. If mismatch → REJECT with error, do not write to ROM_MAP
  6. If match → Write ROM data to ROM_MAP
  SIGNEOF
  ```

- [ ] **Step 74** [G]: Commit ROM signing spec
  ```bash
  cd ~/unheaded && git add docs/DOOM_ROM_SIGNING.md && \
  git commit -m "docs(doom): ROM signing specification with HMAC-SHA256

  Defines signed MBC ROM format, key management, verification flow.
  Addresses D6 (ROM_MAP integrity) from security audit.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 75** [W]: Create scripts/doom/sign_rom.py — ROM signing tool
  - Accept input .mbc file and HMAC key path
  - Output signed .smbc (signed MBC) file
  - Compute CRC-16 over ROM data
  - Compute HMAC-SHA256 over header + ROM data
  - Write signed format with magic, size, CRC, HMAC, ROM data
  - argparse, error handling, --verify-only mode

- [ ] **Step 76** [V]: sign_rom.py produces valid signed ROM
  ```bash
  # Generate test key
  python3 -c "import os; open('/tmp/test-rom-key','wb').write(os.urandom(32))"
  # Sign trivial ROM
  python3 ~/unheaded/scripts/doom/sign_rom.py \
    --input ~/unheaded/scripts/doom/tests/trivial.mbc \
    --key /tmp/test-rom-key \
    --output ~/unheaded/tmp/test/trivial.smbc
  # Verify
  python3 ~/unheaded/scripts/doom/sign_rom.py \
    --verify-only \
    --input ~/unheaded/tmp/test/trivial.smbc \
    --key /tmp/test-rom-key && echo "SIGN/VERIFY: PASS" || echo "SIGN/VERIFY: FAIL"
  rm -f /tmp/test-rom-key
  ```

- [ ] **Step 77** [G]: Commit sign_rom.py
  ```bash
  cd ~/unheaded && git add scripts/doom/sign_rom.py && \
  git commit -m "feat(doom): ROM signing tool with HMAC-SHA256

  Signs MBC binaries with HMAC-SHA256. Produces .smbc format with
  magic, size, CRC-16, HMAC, and ROM data. Verify-only mode for validation.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 78** [W]: Update load_rom.py to REQUIRE signed ROMs
  - Add `--allow-unsigned` flag (development only, prints WARNING)
  - Default: reject unsigned ROMs
  - Verify HMAC before writing to BPF map
  - Log signing status (signed/unsigned) in output

- [ ] **Step 79** [V]: load_rom.py rejects unsigned ROM by default
  ```bash
  python3 ~/unheaded/scripts/doom/load_rom.py \
    --dry-run ~/unheaded/scripts/doom/tests/trivial.mbc 2>&1 | \
  grep -qi 'unsigned\|reject\|signature' && \
  echo "UNSIGNED REJECTION: PASS" || echo "UNSIGNED REJECTION: FAIL"
  ```

- [ ] **Step 80** [G]: Commit load_rom.py signature enforcement
  ```bash
  cd ~/unheaded && git add scripts/doom/load_rom.py && \
  git commit -m "feat(doom): enforce ROM signature verification in loader

  load_rom.py now rejects unsigned ROMs by default. --allow-unsigned flag
  available for development with prominent warning. Addresses D6/LICH-007.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Sign All Test ROMs

- [ ] **Step 81** [B]: Generate development signing key
  ```bash
  mkdir -p ~/.unheaded
  python3 -c "import os; open(os.path.expanduser('~/.unheaded/doom-rom-key'),'wb').write(os.urandom(32))"
  chmod 600 ~/.unheaded/doom-rom-key
  ```

- [ ] **Step 82** [B]: Sign all test ROMs
  ```bash
  for mbc in ~/unheaded/scripts/doom/tests/*.mbc; do
    base=$(basename "$mbc" .mbc)
    python3 ~/unheaded/scripts/doom/sign_rom.py \
      --input "$mbc" \
      --key ~/.unheaded/doom-rom-key \
      --output ~/unheaded/scripts/doom/tests/"${base}.smbc"
    echo "Signed: $base"
  done
  ```

- [ ] **Step 83** [V]: All test ROMs have signed versions
  ```bash
  unsigned=$(ls ~/unheaded/scripts/doom/tests/*.mbc | wc -l)
  signed=$(ls ~/unheaded/scripts/doom/tests/*.smbc 2>/dev/null | wc -l)
  echo "Unsigned: $unsigned, Signed: $signed"
  test "$unsigned" -eq "$signed" && \
  echo "ROM SIGNING COMPLETE: PASS" || echo "ROM SIGNING: INCOMPLETE"
  ```

- [ ] **Step 84** [G]: Commit signed test ROMs
  ```bash
  cd ~/unheaded && git add scripts/doom/tests/*.smbc && \
  git commit -m "feat(doom): sign all ISA test ROMs with HMAC-SHA256

  15 signed .smbc files generated from test ROM suite.
  Development signing key at ~/.unheaded/doom-rom-key (not in repo).

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 85** [V]: **PHASE 3 EXIT GATE** — ROM signing operational, loader enforces signatures
  ```bash
  test -f ~/unheaded/scripts/doom/sign_rom.py && \
  test -f ~/.unheaded/doom-rom-key && \
  ls ~/unheaded/scripts/doom/tests/*.smbc >/dev/null 2>&1 && \
  echo "PHASE 3: PASS" || echo "PHASE 3: FAIL"
  ```

---

## PHASE 4: CRC-16 VALIDATION ENFORCEMENT (Steps 86-100) [P]

**Goal**: Ensure all injected packets have valid CRC-16. Verify XDP validates before processing.
**Prerequisite**: Phase 2 passed. inject.py migrated.
**Time**: 45 minutes
**Agent**: Agent [P] — can run parallel with Phases 3 and 5

- [ ] **Step 86** [R]: Check XDP program source for CRC validation
  ```bash
  grep -rn 'crc\|checksum\|CRC' ~/unheaded/ebpf/monad-cpu/ | head -20
  ```

- [ ] **Step 87** [V]: XDP program validates CRC before processing Monad register
  - If CRC validated → Step 90
  - If CRC NOT validated → Steps 88-89 (add validation)

- [ ] **Step 88** [D][W]: If CRC not validated in XDP, document the gap
  ```bash
  echo "CRC VALIDATION GAP: XDP program does not verify Monad CRC-16 before processing."
  echo "This allows any packet with zero checksum to be processed as valid."
  echo "FIX: Add CRC-16/CCITT-FALSE computation in XDP and drop on mismatch."
  ```

- [ ] **Step 89** [W]: Add CRC validation to the Rust XDP program (if missing)
  - In the Monad parser section of the XDP program:
  - Compute CRC-16/CCITT-FALSE over bytes 0x00-0x11 of the Monad register
  - Compare with bytes 0x12-0x13
  - If mismatch → XDP_DROP and increment STATS[CRC_FAIL] counter
  - Add new stats key: CRC_FAIL = 0x0B

- [ ] **Step 90** [W]: Update inject.py to ALWAYS compute valid CRC-16
  - Remove the zero-checksum construction
  - Compute CRC-16/CCITT-FALSE over Monad bytes 0x00-0x11
  - Pack computed CRC at bytes 0x12-0x13
  - Add `--corrupt-crc` flag for BlackMage testing (intentionally bad CRC)

- [ ] **Step 91** [V]: inject.py produces packets with valid CRC
  ```bash
  python3 ~/unheaded/scripts/doom/inject.py --dry-run --flow-label 0xDE --count 1 2>&1 | \
  grep -qi 'crc\|checksum' && \
  echo "CRC IN INJECT: PASS" || echo "CRC IN INJECT: NEEDS WORK"
  ```

- [ ] **Step 92** [G]: Commit CRC enforcement in inject.py
  ```bash
  cd ~/unheaded && git add scripts/doom/inject.py && \
  git commit -m "fix(doom): enforce valid CRC-16 in packet injection

  All injected packets now have computed CRC-16/CCITT-FALSE checksum.
  --corrupt-crc flag available for security testing. Addresses CRC bypass finding.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 93** [W]: Create CRC validation test script
  ```bash
  cat > ~/unheaded/scripts/doom/tests/test_crc.py << 'CRCEOF'
  #!/usr/bin/env python3
  """CRC-16/CCITT-FALSE validation tests."""

  def crc16_ccitt(data: bytes) -> int:
      crc = 0xFFFF
      for byte in data:
          crc ^= byte << 8
          for _ in range(8):
              if crc & 0x8000:
                  crc = (crc << 1) ^ 0x1021
              else:
                  crc <<= 1
              crc &= 0xFFFF
      return crc

  # Known test vector: "123456789" → 0x29B1
  assert crc16_ccitt(b"123456789") == 0x29B1, "CRC known vector FAILED"

  # Zero-length input
  assert crc16_ccitt(b"") == 0xFFFF, "CRC empty input FAILED"

  # All-zero Monad (18 bytes, version=0x01)
  monad = bytearray(18)
  monad[0] = 0x01
  crc = crc16_ccitt(bytes(monad))
  assert crc != 0x0000, f"CRC of valid Monad should not be zero (got 0x{crc:04X})"

  # Verify CRC detects single bit flip
  monad_with_crc = monad + crc.to_bytes(2, 'big')
  flipped = bytearray(monad_with_crc)
  flipped[4] ^= 0x01  # flip one bit in trace_id
  crc_check = crc16_ccitt(bytes(flipped[:18]))
  assert crc_check != int.from_bytes(flipped[18:20], 'big'), "CRC should detect bit flip"

  print("ALL CRC TESTS PASSED")
  CRCEOF
  ```

- [ ] **Step 94** [B]: Run CRC tests
  ```bash
  python3 ~/unheaded/scripts/doom/tests/test_crc.py
  ```

- [ ] **Step 95** [V]: CRC tests pass
  - If pass → Step 96

- [ ] **Step 96** [G]: Commit CRC tests
  ```bash
  cd ~/unheaded && git add scripts/doom/tests/test_crc.py && \
  git commit -m "test(doom): CRC-16/CCITT-FALSE validation test suite

  Known vector (0x29B1), empty input, Monad-specific, bit-flip detection.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 97** [V]: **PHASE 4 EXIT GATE** — CRC validated in injector, test suite passes
  ```bash
  python3 ~/unheaded/scripts/doom/tests/test_crc.py 2>&1 | grep -q 'PASSED' && \
  echo "PHASE 4: PASS" || echo "PHASE 4: FAIL"
  ```

---

## PHASE 5: FLOW LABEL COLLISION PROTECTION (Steps 98-112) [P]

**Goal**: Document and mitigate flow label collision risk. Implement instance ID validation.
**Prerequisite**: Phase 2 passed.
**Time**: 45 minutes
**Agent**: Agent [P] — can run parallel with Phases 3 and 4

- [ ] **Step 98** [R]: Analyze current CPU_MAP key derivation
  ```bash
  grep -rn 'flow_label\|instance_id\|CPU_MAP' ~/unheaded/ebpf/monad-cpu/ | head -20
  ```

- [ ] **Step 99** [W]: Document flow label collision risk
  ```bash
  cat > ~/unheaded/docs/DOOM_FLOW_LABEL_SECURITY.md << 'FLOWEOF'
  # Doom Flow Label Security Analysis

  ## Current Behavior

  CPU_MAP key = `flow_label & 0xFF` = 8-bit instance ID.
  IPv6 flow label is 20 bits. Multiple flow labels map to the same instance.

  ## Collision Space

  - 20-bit flow label = 1,048,576 possible values
  - 8-bit instance ID = 256 possible keys
  - Collision ratio: 4,096 flow labels per instance ID
  - Example: 0x000DE, 0x001DE, 0x002DE, ... 0xFFFDE all map to instance 0xDE

  ## Risk

  In multi-tenant scenarios, two flows with different labels could collide on
  the same CPU state, causing:
  - State corruption (one flow's PC overwrites another's)
  - Information leakage (one flow reads another's registers)
  - Denial of service (one flow halts another's CPU)

  ## Mitigation Options

  ### Option A: Full 20-bit key (Recommended)
  Change CPU_MAP key from u8 to u32, use full flow_label as key.
  - Pro: Eliminates collision entirely within 20-bit space
  - Con: Larger map, more memory per instance

  ### Option B: Flow label reservation
  Allocate flow labels from a registry. Reject unregistered labels.
  - Pro: Controlled allocation
  - Con: Requires registration infrastructure

  ### Option C: Per-instance HMAC
  Include instance-specific token in Monad register, verified per-hop.
  - Pro: Cryptographic isolation
  - Con: Per-hop overhead, complexity

  ## Decision: Option A for Alpha, Option C for Production
  FLOWEOF
  ```

- [ ] **Step 100** [G]: Commit flow label security doc
  ```bash
  cd ~/unheaded && git add docs/DOOM_FLOW_LABEL_SECURITY.md && \
  git commit -m "docs(doom): flow label collision security analysis

  Documents 4096:1 collision ratio on 8-bit CPU_MAP keys.
  Recommends full 20-bit key for alpha, HMAC for production.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 101** [W]: Add instance ID validation to cpu_state.py
  - Validate that requested instance ID is within 0x00-0xFF range
  - Warn if flow label > 0xFF is used (truncation will occur)
  - Add `--full-flow-label` flag that uses u32 key (future-proofing)

- [ ] **Step 102** [G]: Commit cpu_state.py validation
  ```bash
  cd ~/unheaded && git add scripts/doom/cpu_state.py && \
  git commit -m "feat(doom): add instance ID range validation to cpu_state.py

  Warns on flow label truncation. --full-flow-label flag for future 20-bit keys.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 103** [V]: **PHASE 5 EXIT GATE** — Flow label risk documented, validation in tooling
  ```bash
  test -f ~/unheaded/docs/DOOM_FLOW_LABEL_SECURITY.md && \
  grep -q 'full.flow.label\|instance.*valid' ~/unheaded/scripts/doom/cpu_state.py && \
  echo "PHASE 5: PASS" || echo "PHASE 5: FAIL"
  ```

---

## PHASE 6: AUTOMATED DOOM TEST GATE (Steps 104-130)

**Goal**: Create automated test runner that loads each ROM, injects packets, verifies CPU state.
**Prerequisite**: Phases 2, 3, 4 all passed.
**Time**: 90 minutes
**Agent**: Coordinator (requires sudo for BPF + netns)

- [ ] **Step 104** [W]: Create scripts/doom/test_runner.py — automated ISA test gate
  - Accepts: list of test definitions (ROM path, expected register values, max packets)
  - For each test:
    1. Reset CPU state to zero (SP = 0xFFFF0000)
    2. Load ROM into ROM_MAP
    3. Inject N packets (configurable, default 200)
    4. Read CPU state
    5. Compare actual registers vs expected
    6. Report PASS/FAIL with details
  - Summary: total tests, passed, failed, execution time
  - JSON output mode for CI integration
  - `--stop-on-fail` flag
  - Uses cpu_state.py and load_rom.py as library modules (import, don't subprocess)

- [ ] **Step 105** [W]: Define test definitions in YAML
  ```bash
  cat > ~/unheaded/scripts/doom/tests/test_suite.yaml << 'YAMLEOF'
  ---
  suite: "Doom ISA Validation"
  version: 1
  defaults:
    instance: 0xDE
    max_packets: 200
    timeout_seconds: 30

  tests:
    - name: "trivial"
      rom: "tests/trivial.mbc"
      max_packets: 5
      expect:
        r0: 43
        r1: 1
        halted: 1

    - name: "nop10"
      rom: "tests/nop10.mbc"
      max_packets: 5
      expect:
        pc: 11
        insn_count: 10
        halted: 1

    - name: "nop100"
      rom: "tests/nop100.mbc"
      max_packets: 20
      expect:
        pc: 101
        insn_count: 100
        halted: 1

    - name: "fibonacci"
      rom: "tests/test-fibonacci.mbc"
      max_packets: 50
      expect:
        r0: 55
        halted: 1

    - name: "bitwise"
      rom: "tests/test-bitwise.mbc"
      max_packets: 10
      expect:
        halted: 1

    - name: "branches"
      rom: "tests/test-branches.mbc"
      max_packets: 30
      expect:
        r0: 4
        halted: 1

    - name: "call-ret"
      rom: "tests/test-call-ret.mbc"
      max_packets: 10
      expect:
        r0: 42
        r2: 99
        halted: 1

    - name: "comprehensive"
      rom: "tests/test-comprehensive.mbc"
      max_packets: 50
      expect:
        r0: 42
        r1: 7
        r2: 2
        r7: 99
        halted: 1

    - name: "memory"
      rom: "tests/test-memory.mbc"
      max_packets: 10
      expect:
        r0: 57005  # 0xDEAD
        halted: 1

    - name: "stack"
      rom: "tests/test-stack.mbc"
      max_packets: 20
      expect:
        r0: 100
        r1: 200
        r2: 300
        halted: 1

    - name: "multitick"
      rom: "tests/test-multitick.mbc"
      max_packets: 100
      expect:
        r0: 20
        halted: 1

    - name: "imm32"
      rom: "tests/test-imm32.mbc"
      max_packets: 10
      expect:
        r0: 0xDEADBEEF
        r1: 0xCAFE0000
        halted: 1
  YAMLEOF
  ```

- [ ] **Step 106** [G]: Commit test suite definition
  ```bash
  cd ~/unheaded && git add scripts/doom/tests/test_suite.yaml && \
  git commit -m "feat(doom): YAML-defined ISA test suite with expected values

  12 tests covering arithmetic, bitwise, branches, call/ret, memory,
  stack, screen, multitick, imm32. Machine-readable for CI integration.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 107** [W]: Implement test_runner.py (the core implementation step)
  - Parse test_suite.yaml
  - For each test: reset → load → inject → read → compare
  - Color-coded output (green PASS, red FAIL)
  - Summary statistics
  - Exit code: 0 if all pass, 1 if any fail
  - `--json` for CI output
  - `--test NAME` to run single test
  - `--verbose` for detailed register dumps on failure

- [ ] **Step 108** [G]: Commit test_runner.py
  ```bash
  cd ~/unheaded && git add scripts/doom/test_runner.py && \
  git commit -m "feat(doom): automated ISA test runner

  Loads YAML test suite, executes reset→load→inject→verify cycle for each
  test ROM. Color output, JSON mode for CI, single-test mode, verbose failures.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 109** [B][S]: Run the full test suite (requires ring operational)
  ```bash
  sudo python3 ~/unheaded/scripts/doom/test_runner.py \
    --suite ~/unheaded/scripts/doom/tests/test_suite.yaml \
    --verbose 2>&1 | tee ~/unheaded/tmp/doom/test-run-$(date +%Y%m%d-%H%M%S).txt
  ```

- [ ] **Step 110** [V]: All ISA tests pass
  - If all pass → Step 111
  - If failures → Debug individual tests, fix ROM or expectations

- [ ] **Step 111** [G]: Commit test results artifact
  ```bash
  cd ~/unheaded && git add tmp/artifacts/ 2>/dev/null; \
  git commit -m "test(doom): first automated ISA test suite run — results

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>" 2>/dev/null || true
  ```

### Add to Makefile

- [ ] **Step 112** [W]: Add doom test targets to Makefile
  ```makefile
  # Add to project Makefile
  .PHONY: doom-test doom-test-json doom-status

  doom-test:
  	sudo python3 scripts/doom/test_runner.py --suite scripts/doom/tests/test_suite.yaml --verbose

  doom-test-json:
  	sudo python3 scripts/doom/test_runner.py --suite scripts/doom/tests/test_suite.yaml --json

  doom-status:
  	sudo python3 scripts/doom/cpu_state.py read --instance 0xDE
  ```

- [ ] **Step 113** [G]: Commit Makefile doom targets
  ```bash
  cd ~/unheaded && git add Makefile && \
  git commit -m "feat(doom): add make doom-test, doom-test-json, doom-status targets

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 114** [V]: **PHASE 6 EXIT GATE** — Automated test gate operational
  ```bash
  test -f ~/unheaded/scripts/doom/test_runner.py && \
  test -f ~/unheaded/scripts/doom/tests/test_suite.yaml && \
  grep -q 'doom-test' ~/unheaded/Makefile && \
  echo "PHASE 6: PASS" || echo "PHASE 6: FAIL"
  ```

---

## PHASE 7: WIRE FORMAT ALIGNMENT (Steps 115-132)

**Goal**: Align injection scripts with draft-03 Monad wire format. Document Doom-specific deviations.
**Prerequisite**: Phase 4 passed (CRC enforcement).
**Time**: 60 minutes
**Agent**: Coordinator

- [ ] **Step 115** [R]: Compare /tmp inject.py Monad layout with draft-03
  ```bash
  echo "=== draft-03 Monad Layout ==="
  echo "0x00: version (0x01)"
  echo "0x01: src_service_id"
  echo "0x02: dst_service_id"
  echo "0x03: hop_count"
  echo "0x04-07: trace_id"
  echo "0x08: qos_class"
  echo "0x09: flow_action"
  echo "0x0A: circuit_state"
  echo "0x0B: flags"
  echo "0x0C-0D: latency_budget_us"
  echo "0x0E: deployment_ring"
  echo "0x0F: mesh_flags"
  echo "0x10-11: reserved"
  echo "0x12-13: checksum"
  echo ""
  echo "=== /tmp inject.py Monad Layout ==="
  echo "Bytes 0-7: 0x01,0,0,0,0,0,0,0x02 (version + 6 zeros + 0x02)"
  echo "Bytes 8-9: latency_hint (u16 BE)"
  echo "Bytes 10-13: deploy_ring, mesh_flags, src_prefix, dst_prefix"
  echo "Bytes 14-17: scratch[0..3]"
  echo "Bytes 18-19: checksum[0..1]"
  echo ""
  echo "DIVERGENCE: /tmp scripts use old Monad layout predating draft-03"
  ```

- [ ] **Step 116** [W]: Update inject.py to use draft-03 compliant Monad layout
  - Byte 0x00: version = 0x01
  - Byte 0x01: src_service_id = 0x00 (Doom tick — no specific service)
  - Byte 0x02: dst_service_id = 0x00
  - Byte 0x03: hop_count = 0x00 (incremented by XDP)
  - Bytes 0x04-07: trace_id = configurable (--trace-id flag)
  - Byte 0x08: qos_class = 0x00
  - Byte 0x09: flow_action = 0x00 (forward)
  - Byte 0x0A: circuit_state = 0x00
  - Byte 0x0B: flags = 0x02 (K0 bit set = Kingdom Mode CPU tick)
  - Bytes 0x0C-0D: latency_budget_us = 0x0000
  - Byte 0x0E: deployment_ring = 0x00
  - Byte 0x0F: mesh_flags = 0x00
  - Bytes 0x10-11: reserved = 0x0000
  - Bytes 0x12-13: CRC-16 (computed)

- [ ] **Step 117** [V]: inject.py Monad layout matches draft-03 field positions
  ```bash
  python3 ~/unheaded/scripts/doom/inject.py --dry-run --flow-label 0xDE --count 1 2>&1 | \
  grep -i 'monad\|wire\|draft' && \
  echo "WIRE FORMAT: ALIGNED" || echo "WIRE FORMAT: CHECK MANUALLY"
  ```

- [ ] **Step 118** [G]: Commit wire format alignment
  ```bash
  cd ~/unheaded && git add scripts/doom/inject.py && \
  git commit -m "fix(doom): align Monad register layout with draft-03

  Packet injection now uses draft-bellis-unheaded-protocol-foundation-03
  field positions. Kingdom Mode flag (K0) at byte 0x0B indicates CPU tick.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 119** [W]: Document Doom-specific wire format usage
  ```bash
  cat > ~/unheaded/docs/DOOM_WIRE_FORMAT.md << 'WIREEOF'
  # Doom-over-IPv6 Wire Format Usage

  ## Monad Register (draft-03 Compliant)

  Doom uses the standard 20-byte Monad register in IPv6 Hop-by-Hop options.
  Kingdom Mode bits (K1:K0 = 0:1) indicate CPU tick packets.

  ### Doom-Specific Field Values

  | Offset | Field | Doom Value | Meaning |
  |--------|-------|------------|---------|
  | 0x00 | version | 0x01 | Protocol v1 |
  | 0x01 | src_service_id | 0x00 | Tick source (not service-routed) |
  | 0x02 | dst_service_id | 0x00 | Tick destination (broadcast to ring) |
  | 0x03 | hop_count | 0x00→N | Incremented each hop |
  | 0x04-07 | trace_id | configurable | Flow correlation |
  | 0x0B | flags | 0x01 | K0=1: Kingdom Mode CPU tick |
  | 0x12-13 | checksum | computed | CRC-16/CCITT-FALSE |

  ### Kingdom Mode (Section 12)

  When K1:K0 = 0:1, the Shim program enters CPU tick mode:
  - Reads MBC instruction from ROM_MAP at current PC
  - Executes instruction against CPU_MAP state
  - Advances PC
  - Forwards packet to next hop

  ### IPv6 Flow Label

  Flow label encodes the CPU instance: `instance_id = flow_label & 0xFF`
  (Alpha limitation — see DOOM_FLOW_LABEL_SECURITY.md for analysis)
  WIREEOF
  ```

- [ ] **Step 120** [G]: Commit wire format documentation
  ```bash
  cd ~/unheaded && git add docs/DOOM_WIRE_FORMAT.md && \
  git commit -m "docs(doom): wire format usage guide (draft-03 aligned)

  Documents Doom-specific Monad field values, Kingdom Mode flag usage,
  flow label to instance ID mapping.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 121** [V]: **PHASE 7 EXIT GATE** — Wire format aligned, documented
  ```bash
  test -f ~/unheaded/docs/DOOM_WIRE_FORMAT.md && \
  echo "PHASE 7: PASS" || echo "PHASE 7: FAIL"
  ```

---

## PHASE 8: DOCUMENTATION & ARTIFACT VERSIONING (Steps 122-142)

**Goal**: Create ring_status.py, enforce artifact naming conventions, update main docs.
**Prerequisite**: Phase 7 passed.
**Time**: 60 minutes
**Agent**: Coordinator

- [ ] **Step 122** [W]: Create scripts/doom/ring_status.py
  - Report namespace status (UP/DOWN for monad0-5)
  - Report veth link status
  - Report XDP program attachment
  - Report BPF map pin status
  - Report CPU state summary (PC, insn_count, halted, cache stats)
  - Report stats map decoded
  - Output ring topology ASCII art
  - JSON mode for monitoring integration
  - Timestamped output with git SHA

- [ ] **Step 123** [G]: Commit ring_status.py
  ```bash
  cd ~/unheaded && git add scripts/doom/ring_status.py && \
  git commit -m "feat(doom): ring status reporting tool

  Reports namespace, veth, XDP, BPF map, CPU state, stats.
  ASCII topology art, JSON mode, timestamped with git SHA.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 124** [W]: Create scripts/doom/snapshot.py — forensic snapshot tool
  - Captures: CPU state, stats, ring status, ROM hash, timestamp, git SHA
  - Writes to `~/unheaded/tmp/artifacts/doom-snapshot-{timestamp}-{sha}.json`
  - Single command for complete state capture
  - Replaces manual `read_cpu.py > doom-state-phase7.txt` workflow

- [ ] **Step 125** [G]: Commit snapshot.py
  ```bash
  cd ~/unheaded && git add scripts/doom/snapshot.py && \
  git commit -m "feat(doom): forensic snapshot tool for state capture

  Single command captures CPU state, stats, ring status, ROM hash.
  Timestamped JSON output with git SHA. Replaces manual dump workflow.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 126** [W]: Update scripts/doom/README.md with complete tool inventory
  - Add snapshot.py and ring_status.py
  - Add usage examples for every tool
  - Add troubleshooting section

- [ ] **Step 127** [G]: Commit README update
  ```bash
  cd ~/unheaded && git add scripts/doom/README.md && \
  git commit -m "docs(doom): complete tool inventory and usage examples

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 128** [W]: Update CLAUDE.md with Doom tooling references
  - Add scripts/doom/ to project structure
  - Reference DOOM_ROM_SIGNING.md, DOOM_WIRE_FORMAT.md, DOOM_FLOW_LABEL_SECURITY.md
  - Note /tmp hardening decision

- [ ] **Step 129** [G]: Commit CLAUDE.md update
  ```bash
  cd ~/unheaded && git add CLAUDE.md && \
  git commit -m "docs: update CLAUDE.md with Doom tooling and /tmp hardening

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### Update Instructions-Per-Tick Documentation

- [ ] **Step 130** [W]: Document actual insns/packet rate in doom-ring.sh header
  - Update comment "1 instruction per hop, 6 per circuit" with actual measured rate
  - Add: "Measured: ~16 instructions per packet (~2.67 per hop per tick)"
  - Reference doom-state-phase7.txt data: 6.6B insns / 412M packets

- [ ] **Step 131** [G]: Commit doom-ring.sh documentation fix
  ```bash
  cd ~/unheaded && git add scripts/doom-ring.sh && \
  git commit -m "docs(doom): correct instructions-per-tick rate in ring docs

  Measured ~16 insns/packet (not 1/hop as originally documented).
  Based on Phase 7 data: 6.6B insns / 412M packets.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 132** [V]: **PHASE 8 EXIT GATE** — All documentation updated, tools complete
  ```bash
  test -f ~/unheaded/scripts/doom/ring_status.py && \
  test -f ~/unheaded/scripts/doom/snapshot.py && \
  test -f ~/unheaded/docs/DOOM_ROM_SIGNING.md && \
  test -f ~/unheaded/docs/DOOM_WIRE_FORMAT.md && \
  test -f ~/unheaded/docs/DOOM_FLOW_LABEL_SECURITY.md && \
  echo "PHASE 8: PASS" || echo "PHASE 8: FAIL"
  ```

---

## PHASE 9: COMMIT DISCIPLINE ENFORCEMENT (Steps 133-148)

**Goal**: Establish commit-per-step workflow with pre-commit hooks and git alias.
**Prerequisite**: Phase 8 passed.
**Time**: 30 minutes
**Agent**: Coordinator

- [ ] **Step 133** [W]: Create pre-commit hook that runs basic checks
  ```bash
  cat > ~/unheaded/.githooks/pre-commit << 'HOOKEOF'
  #!/bin/bash
  # Unheaded pre-commit hook
  set -e

  echo "=== Pre-commit checks ==="

  # Check for /tmp references in committed files
  if git diff --cached --name-only | xargs grep -l '/tmp/' 2>/dev/null; then
    echo "ERROR: Committed files reference /tmp/. Use ~/unheaded/tmp/ or scripts/doom/ instead."
    exit 1
  fi

  # Check for hardcoded secrets patterns
  if git diff --cached | grep -iE '(password|secret|api.key|private.key)\s*=' 2>/dev/null; then
    echo "WARNING: Possible hardcoded secret detected. Review before committing."
  fi

  # Check for assert in Python production code (not test files)
  PROD_PY=$(git diff --cached --name-only | grep '\.py$' | grep -v 'test_' | grep -v '_test\.py')
  if [ -n "$PROD_PY" ]; then
    if echo "$PROD_PY" | xargs grep -n '^[[:space:]]*assert ' 2>/dev/null; then
      echo "WARNING: assert in production Python code. Use explicit if/raise instead."
    fi
  fi

  echo "=== Pre-commit: PASS ==="
  HOOKEOF
  chmod +x ~/unheaded/.githooks/pre-commit
  ```

- [ ] **Step 134** [B]: Configure git to use project hooks
  ```bash
  cd ~/unheaded && git config core.hooksPath .githooks
  ```

- [ ] **Step 135** [G]: Commit pre-commit hook
  ```bash
  cd ~/unheaded && git add .githooks/ && \
  git commit -m "feat(devops): pre-commit hook — blocks /tmp refs, warns on secrets/asserts

  Enforces /tmp migration. Warns on hardcoded secrets and Python asserts
  in production code.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 136** [W]: Create git alias for Doom development commits
  ```bash
  cd ~/unheaded && git config alias.doom-commit '!f() { \
    SHA=$(git rev-parse --short HEAD); \
    git add -p && \
    git commit -m "$1

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"; \
  }; f'
  ```

- [ ] **Step 137** [W]: Create CONTRIBUTING.md with commit discipline rules
  ```bash
  cat > ~/unheaded/CONTRIBUTING.md << 'CONTRIBEOF'
  # Contributing to Unheaded

  ## Commit Discipline

  **COMMIT AFTER EVERY MEANINGFUL STEP.** Not at the end of a session.
  Not when you "feel like it." After every step that produces a verifiable result.

  ### Why?

  - Atomic rollback: any step can be reverted without losing others
  - Forensic trail: git log tells the complete story
  - Parallel safety: other agents can pull your progress incrementally
  - Accountability: every change has a message explaining WHY

  ### Commit Message Format

  ```
  <type>(<scope>): <description>

  [optional body]

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  ```

  Types: feat, fix, docs, test, chore, refactor, style

  ### Pre-Commit Checks

  The `.githooks/pre-commit` hook automatically checks:
  - No /tmp references in committed files
  - No hardcoded secrets
  - No bare asserts in production Python

  ### /tmp Policy

  **NEVER** store project files in /tmp. Use:
  - `scripts/doom/` for tools
  - `~/unheaded/tmp/doom/` for runtime artifacts
  - `~/unheaded/tmp/artifacts/` for forensic snapshots
  CONTRIBEOF
  ```

- [ ] **Step 138** [G]: Commit CONTRIBUTING.md
  ```bash
  cd ~/unheaded && git add CONTRIBUTING.md && \
  git commit -m "docs: add CONTRIBUTING.md with commit discipline rules

  Commit-per-step policy, message format, pre-commit hooks, /tmp policy.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 139** [V]: **PHASE 9 EXIT GATE** — Commit hooks active, discipline documented
  ```bash
  test -x ~/unheaded/.githooks/pre-commit && \
  test -f ~/unheaded/CONTRIBUTING.md && \
  cd ~/unheaded && git config core.hooksPath | grep -q '.githooks' && \
  echo "PHASE 9: PASS" || echo "PHASE 9: FAIL"
  ```

---

## PHASE 10: FINAL VERIFICATION & EXIT GATE (Steps 140-155)

**Goal**: Full validation sweep across all hardening work. Celebrate.
**Prerequisite**: All phases 0-9 passed.
**Time**: 30 minutes
**Agent**: Coordinator

### System Verification

- [ ] **Step 140** [V]: /tmp is noexec
  ```bash
  mount | grep '/tmp.*noexec' && echo "NOEXEC: VERIFIED" || echo "NOEXEC: FAILED"
  ```

- [ ] **Step 141** [V]: No project files in /tmp
  ```bash
  ls /tmp/*.py /tmp/*.asm /tmp/*.mbc /tmp/doom-*.txt 2>/dev/null | wc -l | \
  grep -q '^0$' && echo "TMP CLEAN: VERIFIED" || echo "TMP CLEAN: FAILED"
  ```

- [ ] **Step 142** [V]: All tools migrated to scripts/doom/
  ```bash
  for tool in load_rom.py inject.py cpu_state.py skip_crt0.py ring_status.py snapshot.py test_runner.py sign_rom.py; do
    test -f ~/unheaded/scripts/doom/$tool && echo "  [OK] $tool" || echo "  [MISSING] $tool"
  done
  ```

- [ ] **Step 143** [V]: All test ROMs in scripts/doom/tests/
  ```bash
  echo "ASM: $(ls ~/unheaded/scripts/doom/tests/*.asm | wc -l)"
  echo "MBC: $(ls ~/unheaded/scripts/doom/tests/*.mbc | wc -l)"
  echo "SMBC: $(ls ~/unheaded/scripts/doom/tests/*.smbc 2>/dev/null | wc -l)"
  ```

- [ ] **Step 144** [V]: Pre-commit hook blocks /tmp references
  ```bash
  cd ~/unheaded && echo '/tmp/bad.py' > /tmp/test-hook.txt 2>/dev/null || true
  echo "Hook path: $(git config core.hooksPath)"
  test -x .githooks/pre-commit && echo "HOOK: EXECUTABLE" || echo "HOOK: NOT EXECUTABLE"
  ```

- [ ] **Step 145** [V]: CRC test suite passes
  ```bash
  python3 ~/unheaded/scripts/doom/tests/test_crc.py
  ```

- [ ] **Step 146** [V]: ROM signing operational
  ```bash
  test -f ~/.unheaded/doom-rom-key && echo "KEY: EXISTS" || echo "KEY: MISSING"
  ls ~/unheaded/scripts/doom/tests/*.smbc >/dev/null 2>&1 && echo "SIGNED ROMS: EXISTS" || echo "SIGNED ROMS: MISSING"
  ```

### Documentation Verification

- [ ] **Step 147** [V]: All new docs exist
  ```bash
  for doc in DOOM_ROM_SIGNING.md DOOM_WIRE_FORMAT.md DOOM_FLOW_LABEL_SECURITY.md; do
    test -f ~/unheaded/docs/$doc && echo "  [OK] $doc" || echo "  [MISSING] $doc"
  done
  test -f ~/unheaded/CONTRIBUTING.md && echo "  [OK] CONTRIBUTING.md" || echo "  [MISSING] CONTRIBUTING.md"
  ```

### Git History Verification

- [ ] **Step 148** [B]: Count commits from this sprint
  ```bash
  cd ~/unheaded && git log --oneline --since="today" | wc -l
  echo "=== Sprint commits ==="
  git log --oneline --since="today"
  ```

- [ ] **Step 149** [V]: Multiple granular commits (not one big squash)
  - Expected: 15-25+ commits from this battle plan
  - If fewer than 10 → commit discipline was not followed

### Final Snapshot

- [ ] **Step 150** [B][S]: Run full ring status and snapshot
  ```bash
  sudo python3 ~/unheaded/scripts/doom/ring_status.py 2>&1 | \
  tee ~/unheaded/tmp/artifacts/ring-status-post-hardening-$(date +%Y%m%d-%H%M%S).txt
  ```

- [ ] **Step 151** [B][S]: Run automated test suite if ring is operational
  ```bash
  sudo python3 ~/unheaded/scripts/doom/test_runner.py \
    --suite ~/unheaded/scripts/doom/tests/test_suite.yaml 2>&1 | \
  tee ~/unheaded/tmp/artifacts/test-results-post-hardening-$(date +%Y%m%d-%H%M%S).txt
  ```

### EXIT

- [ ] **Step 152** [V]: **BATTLE PLAN FINAL EXIT GATE**
  ```bash
  echo "========================================="
  echo "  DOOM HARDENING BATTLE PLAN — FINAL GATE"
  echo "========================================="
  PASS=0; FAIL=0
  check() { if eval "$2"; then echo "  [PASS] $1"; ((PASS++)); else echo "  [FAIL] $1"; ((FAIL++)); fi; }
  check "/tmp noexec" "mount | grep -q '/tmp.*noexec'"
  check "/tmp clean" "! ls /tmp/*.py /tmp/*.asm /tmp/*.mbc 2>/dev/null | grep -q '.'"
  check "scripts/doom/ populated" "test -f ~/unheaded/scripts/doom/test_runner.py"
  check "ROM signing" "test -f ~/unheaded/scripts/doom/sign_rom.py"
  check "CRC tests" "python3 ~/unheaded/scripts/doom/tests/test_crc.py 2>/dev/null"
  check "Pre-commit hook" "test -x ~/unheaded/.githooks/pre-commit"
  check "CONTRIBUTING.md" "test -f ~/unheaded/CONTRIBUTING.md"
  check "Wire format docs" "test -f ~/unheaded/docs/DOOM_WIRE_FORMAT.md"
  echo "========================================="
  echo "  PASSED: $PASS  FAILED: $FAIL"
  echo "========================================="
  test $FAIL -eq 0 && echo "BATTLE PLAN: COMPLETE" || echo "BATTLE PLAN: INCOMPLETE"
  ```

- [ ] **Step 153** [G]: Final commit — battle plan completion marker
  ```bash
  cd ~/unheaded && git add -A && \
  git diff --cached --quiet || \
  git commit -m "chore(doom): hardening battle plan complete

  /tmp hardened (noexec). All tooling migrated to scripts/doom/.
  ROM signing with HMAC-SHA256. CRC-16 enforcement. Automated test gate.
  Wire format aligned with draft-03. Commit discipline hooks active.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

---

## APPENDIX A: EMERGENCY PROCEDURES

### E1: Ring Namespace Down

**Symptom**: `ip netns list` missing one or more monad namespaces
```bash
sudo ./scripts/doom-ring.sh teardown
sudo ./scripts/doom-ring.sh setup
```

### E2: BPF Maps Not Pinned

**Symptom**: `ls /sys/fs/bpf/unheaded/doom-ring/maps/` empty or missing
```bash
sudo mkdir -p /sys/fs/bpf/unheaded/doom-ring/maps
sudo ./scripts/doom-ring.sh teardown
sudo ./scripts/doom-ring.sh setup
```

### E3: XDP Not Attached

**Symptom**: `ip link show` doesn't show xdp flag on veth interfaces
```bash
# Reload BPF programs
sudo ./scripts/load-ebpf.sh
```

### E4: /tmp Remount Lost After Reboot

**Symptom**: `mount | grep /tmp` doesn't show noexec
```bash
# Verify fstab entry exists
grep '/tmp' /etc/fstab
# If missing, re-add:
echo 'tmpfs /tmp tmpfs defaults,noexec,nosuid,nodev,size=2G 0 0' | sudo tee -a /etc/fstab
sudo mount -o remount /tmp
```

### E5: Pre-commit Hook Not Firing

**Symptom**: Commits with /tmp references succeed
```bash
cd ~/unheaded
git config core.hooksPath   # Should show .githooks
chmod +x .githooks/pre-commit
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent Type | Parallel? | Dependencies | Est. Time |
|-------|-----------|-----------|--------------|-----------|
| 0: Env Verify | Coordinator | No | None | 15 min |
| 1: /tmp Hardening | Coordinator | No | Phase 0 | 30 min |
| 2: Tooling Migration | Coordinator | No | Phase 1 | 90 min |
| 3: ROM Integrity | Agent | Yes [P] | Phase 2 | 90 min |
| 4: CRC Validation | Agent | Yes [P] | Phase 2 | 45 min |
| 5: Flow Label | Agent | Yes [P] | Phase 2 | 45 min |
| 6: Test Gate | Coordinator | No | Phases 3,4 | 90 min |
| 7: Wire Format | Coordinator | No | Phase 4 | 60 min |
| 8: Documentation | Coordinator | No | Phase 7 | 60 min |
| 9: Commit Discipline | Coordinator | No | Phase 8 | 30 min |
| 10: Final Verification | Coordinator | No | All | 30 min |

**Critical Path**: 0→1→2→3→6→7→8→9→10 = ~7.5 hours
**With parallelism** (3+4+5 concurrent): ~6.5 hours

---

## APPENDIX C: QUICK REFERENCE

### BPF Map Paths

```
/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP      # u32 key → u32 instruction
/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP       # u32 key → 104-byte CPU state
/sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP       # u32 key → u8 value
/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP    # u32 key → u8 pixel
/sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP       # u32 key → u8 keystate
/sys/fs/bpf/unheaded/doom-ring/maps/STATS         # u32 key → u64 counter
/sys/fs/bpf/unheaded/doom-ring/maps/L1_CACHE      # 64-byte cache lines
/sys/fs/bpf/unheaded/doom-ring/maps/COMPUTE_EVENTS # ring buffer
```

### CPU State Struct (104 bytes)

```
Offset  Size  Field
0-63    64    r0-r15 (16 × u32 LE)
64      4     PC (u32 LE)
68      1     flags
69      1     halted
70      1     stalled
71-79   9     padding
80      8     insn_count (u64 LE)
88      8     cache_hits (u64 LE)
96      8     cache_misses (u64 LE)
```

### Stats Map Keys

```
0x00 = PACKETS_TOTAL
0x01 = CPU_TICKS
0x02 = INSNS_EXECUTED
0x03 = HALTED
0x05 = NO_STATE
0x07 = ERRORS
0x09 = CACHE_HITS
0x0A = CACHE_MISSES
0x0B = CRC_FAIL (new — Phase 4)
```

### CRC-16/CCITT-FALSE

```python
def crc16(data: bytes) -> int:
    crc = 0xFFFF
    for b in data:
        crc ^= b << 8
        for _ in range(8):
            crc = (crc << 1) ^ 0x1021 if crc & 0x8000 else crc << 1
            crc &= 0xFFFF
    return crc
```

### Monad Register (draft-03, 20 bytes)

```
0x00: version        0x08: qos_class       0x10-11: reserved
0x01: src_service_id 0x09: flow_action     0x12-13: checksum
0x02: dst_service_id 0x0A: circuit_state
0x03: hop_count      0x0B: flags (K0=CPU tick)
0x04-07: trace_id    0x0C-0D: latency_budget_us
                     0x0E: deployment_ring
                     0x0F: mesh_flags
```

---

*S-DOOM-HARDEN Battle Plan — Forged 2026-02-22*
*10 Phases. 153 Steps. From /tmp chaos to hardened infrastructure.*
*The wire is the processor. The commits are the proof.*
