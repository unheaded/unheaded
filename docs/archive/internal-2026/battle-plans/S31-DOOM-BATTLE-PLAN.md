# S31 DOOM-OVER-IPv6 BATTLE PLAN — 20 Phases, 420+ Steps

**Date**: 2026-02-21
**Sprint**: S31 — Packet Ring Integration → First Frame → Live Gameplay
**Prerequisite**: S30 complete (commit 95e47c2), 293 Rust tests PASS, LICH-007 running
**Target**: Doom running over IPv6 packets, visible in browser, keyboard-controlled
**Estimated Duration**: 16-24 hours of focused execution across 2-3 sessions
**Agent Strategy**: Phases 0-8 sequential (coordinator), Phases 9-20 parallelizable after first packet

---

## LEGEND

```
[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
```

**Exit Gate Convention**: Every phase ends with a gate. If the gate fails, DO NOT proceed. Debug in-phase.

---

## PHASE 0: LICH-007 CAMPAIGN HARVEST (Steps 1-15)

**Goal**: Collect and document the 72-hour fuzz campaign results.
**Prerequisite**: Campaign finished (~2026-02-24 03:38 UTC) OR manually terminated.
**Time**: 30 minutes
**Agent**: Solo (coordinator)

### Campaign Status Collection

- [ ] **Step 1** [B]: Check if fuzz campaign is still running
  ```bash
  ps aux | grep -E 'cargo.fuzz|fuzz_mbc' | grep -v grep
  ```

- [ ] **Step 2** [B]: If still running, check remaining time
  ```bash
  for log in /tmp/lich007-*.log; do
    echo "=== $(basename $log) ==="
    tail -5 "$log" 2>/dev/null || echo "(not found)"
  done
  ```

- [ ] **Step 3** [B]: If campaign finished, collect final statistics
  ```bash
  for log in /tmp/lich007-decode.log /tmp/lich007-execute.log /tmp/lich007-roundtrip.log; do
    echo "=== $(basename $log .log) ==="
    grep -E 'execs_done|execs_per_sec|new_units_added|peak_rss_mb' "$log" | tail -4
  done
  ```

- [ ] **Step 4** [B]: Check for crashes (expect EMPTY directories)
  ```bash
  ls -la crates/monad-mbc/fuzz/artifacts/fuzz_mbc_decode/ 2>/dev/null || echo "No decode artifacts"
  ls -la crates/monad-mbc/fuzz/artifacts/fuzz_mbc_execute/ 2>/dev/null || echo "No execute artifacts"
  ls -la crates/monad-mbc/fuzz/artifacts/fuzz_mbc_roundtrip/ 2>/dev/null || echo "No roundtrip artifacts"
  ```

- [ ] **Step 5** [V]: **CRASH CHECK GATE** — If ANY crash artifacts exist:
  ```bash
  find crates/monad-mbc/fuzz/artifacts/ -type f -name 'crash-*' 2>/dev/null | head -5
  ```
  - If crashes found → STOP. Triage each crash:
    1. Reproduce: `cargo +nightly fuzz run <target> <crash_file>`
    2. Minimize: `cargo +nightly fuzz tmin <target> <crash_file>`
    3. Classify: panic vs OOB vs logic error
    4. Fix in source
    5. Re-run affected target for 1 hour
  - If NO crashes → proceed to Step 6

### Coverage Analysis

- [ ] **Step 6** [B]: Generate fuzz coverage report (if cargo-cov available)
  ```bash
  cd crates/monad-mbc
  cargo +nightly fuzz coverage fuzz_mbc_execute 2>/dev/null || echo "Coverage tool not available"
  ```

- [ ] **Step 7** [B]: Count corpus entries
  ```bash
  echo "Corpus sizes:"
  for dir in crates/monad-mbc/fuzz/corpus/fuzz_mbc_*/; do
    echo "  $(basename $dir): $(ls "$dir" 2>/dev/null | wc -l) entries"
  done
  ```

- [ ] **Step 8** [B]: Measure corpus disk usage
  ```bash
  du -sh crates/monad-mbc/fuzz/corpus/ 2>/dev/null || echo "No corpus"
  ```

### Results Documentation

- [ ] **Step 9** [W]: Write LICH-007 results document
  ```bash
  # File: docs/sessions/S30-lich007-results.md
  # Template populated with actual stats from Steps 3-8
  ```

- [ ] **Step 10** [B]: Verify all Rust tests still pass post-fuzz
  ```bash
  cargo test --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | tail -20
  ```

- [ ] **Step 11** [B]: Verify all Go tests still pass
  ```bash
  go test ./... 2>&1 | tail -20
  ```

- [ ] **Step 12** [V]: **TEST GATE** — Both test suites must show 0 failures
  - Rust: `test result: ok. 293 passed; 0 failed`
  - Go: `ok` on all packages

### Cleanup & Commit

- [ ] **Step 13** [B]: Kill any remaining fuzz processes
  ```bash
  pkill -f 'cargo.*fuzz' 2>/dev/null || true
  ```

- [ ] **Step 14** [B]: Stage and commit results doc
  ```bash
  git add docs/sessions/S30-lich007-results.md
  git commit -m "docs(lich-007): document 72-hour fuzz campaign results

  Zero crashes across 1B+ executions. Coverage: decode 14 edges,
  execute 366 edges, roundtrip 62 edges. Corpus: 1383+ entries.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 15** [V]: **PHASE 0 EXIT GATE** — LICH-007 results documented, all tests pass, no crashes

---

## PHASE 1: ENVIRONMENT VERIFICATION (Steps 16-35)

**Goal**: Verify the dev machine has everything needed for the packet ring.
**Time**: 20 minutes
**Agent**: Coordinator

### Kernel & Tool Verification

- [ ] **Step 16** [B]: Check kernel version (need >= 5.15 for XDP + BPF features)
  ```bash
  uname -r
  ```

- [ ] **Step 17** [V]: Kernel version must be >= 5.15. If not → STOP, upgrade kernel.

- [ ] **Step 18** [B]: Check for eBPF support
  ```bash
  ls /sys/kernel/btf/vmlinux 2>/dev/null && echo "BTF: OK" || echo "BTF: MISSING"
  cat /proc/config.gz 2>/dev/null | gunzip | grep -E 'CONFIG_BPF|CONFIG_XDP' | head -10
  ```

- [ ] **Step 19** [B]: Check bpffs mount
  ```bash
  mount | grep bpf || echo "bpffs not mounted"
  ```

- [ ] **Step 20** [S][B]: Mount bpffs if not mounted
  ```bash
  sudo mount -t bpf bpf /sys/fs/bpf 2>/dev/null || echo "Already mounted or error"
  ```

- [ ] **Step 21** [B]: Verify required tools
  ```bash
  for tool in ip bpftool python3 nsenter tc nping; do
    which $tool 2>/dev/null && echo "$tool: $(which $tool)" || echo "$tool: MISSING"
  done
  ```

- [ ] **Step 22** [D]: Install missing tools if needed
  ```bash
  # Debian/Ubuntu:
  sudo apt-get install -y iproute2 bpftool linux-tools-$(uname -r) python3 nmap
  # Fedora/RHEL:
  # sudo dnf install -y iproute bpftool python3 nmap
  ```

### Build Verification

- [ ] **Step 23** [B]: Check Rust toolchain
  ```bash
  rustc --version
  cargo --version
  rustup show active-toolchain
  ```

- [ ] **Step 24** [B]: Check Go toolchain
  ```bash
  go version
  ```

- [ ] **Step 25** [B]: Verify monad-cpu-ebpf BPF binary exists
  ```bash
  ls -la ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf
  file ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf
  ```

- [ ] **Step 26** [D]: Rebuild monad-cpu-ebpf if missing
  ```bash
  cd ebpf && cargo +nightly build -p monad-cpu-ebpf \
    --target bpfel-unknown-none -Z build-std=core --release
  ```

- [ ] **Step 27** [B]: Verify ebpf-loader binary exists
  ```bash
  ls -la cmd/ebpf-loader/target/release/ebpf-loader
  ```

- [ ] **Step 28** [D]: Build ebpf-loader if missing
  ```bash
  cargo build --manifest-path cmd/ebpf-loader/Cargo.toml --release
  ```

- [ ] **Step 29** [B]: Verify BPF program passes verifier (dry run)
  ```bash
  sudo bpftool prog load ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf \
    /sys/fs/bpf/test_monad_cpu type xdp 2>&1 || echo "Direct load may fail (expected for aya format)"
  ```

- [ ] **Step 30** [B]: Quick-check Go build
  ```bash
  go build ./...
  ```

- [ ] **Step 31** [B]: Quick-check Go tests
  ```bash
  go test -count=1 -timeout 60s ./internal/doom/...
  ```

### Network Pre-checks

- [ ] **Step 32** [B]: Check for conflicting namespaces from previous runs
  ```bash
  ip netns list 2>/dev/null | grep monad || echo "No existing monad namespaces"
  ```

- [ ] **Step 33** [S][B]: Clean up any stale namespaces
  ```bash
  for i in 0 1 2 3 4 5; do
    sudo ip netns del "monad${i}" 2>/dev/null || true
  done
  ```

- [ ] **Step 34** [B]: Verify IPv6 is enabled on host
  ```bash
  sysctl net.ipv6.conf.all.disable_ipv6
  # Must show = 0
  ```

- [ ] **Step 35** [V]: **PHASE 1 EXIT GATE** — All tools present, BPF binary exists, Go builds, no stale namespaces

---

## PHASE 2: NAMESPACE RING ASSEMBLY (Steps 36-65)

**Goal**: Create the 6-namespace directed ring and verify packet circulation WITHOUT BPF.
**Time**: 30-45 minutes
**Agent**: Coordinator (requires sudo)

### Create Namespaces

- [ ] **Step 36** [S][B]: Create all 6 namespaces
  ```bash
  for i in 0 1 2 3 4 5; do
    sudo ip netns add "monad${i}"
    sudo ip netns exec "monad${i}" ip link set lo up
    echo "Created monad${i}"
  done
  ```

- [ ] **Step 37** [V]: Verify all 6 exist
  ```bash
  ip netns list | grep monad | sort
  # Expect: monad0 monad1 monad2 monad3 monad4 monad5
  ```

### Create Veth Pairs (Directed Ring)

- [ ] **Step 38** [S][B]: Create veth pair: monad0 → monad1
  ```bash
  sudo ip link add veth01 type veth peer name veth01p
  sudo ip link set veth01 netns monad0
  sudo ip link set veth01p netns monad1
  ```

- [ ] **Step 39** [S][B]: Create veth pair: monad1 → monad2
  ```bash
  sudo ip link add veth12 type veth peer name veth12p
  sudo ip link set veth12 netns monad1
  sudo ip link set veth12p netns monad2
  ```

- [ ] **Step 40** [S][B]: Create veth pair: monad2 → monad3
  ```bash
  sudo ip link add veth23 type veth peer name veth23p
  sudo ip link set veth23 netns monad2
  sudo ip link set veth23p netns monad3
  ```

- [ ] **Step 41** [S][B]: Create veth pair: monad3 → monad4
  ```bash
  sudo ip link add veth34 type veth peer name veth34p
  sudo ip link set veth34 netns monad3
  sudo ip link set veth34p netns monad4
  ```

- [ ] **Step 42** [S][B]: Create veth pair: monad4 → monad5
  ```bash
  sudo ip link add veth45 type veth peer name veth45p
  sudo ip link set veth45 netns monad4
  sudo ip link set veth45p netns monad5
  ```

- [ ] **Step 43** [S][B]: Create veth pair: monad5 → monad0 (closes the ring)
  ```bash
  sudo ip link add veth50 type veth peer name veth50p
  sudo ip link set veth50 netns monad5
  sudo ip link set veth50p netns monad0
  ```

### Configure IPv6 Addressing (Per-Link /64 — CRITICAL)

**WARNING**: Each veth pair MUST have its own /64 prefix. Same /64 across pairs causes NDP ambiguity → packet drops. This was a known pitfall from previous sessions.

- [ ] **Step 44** [S][B]: Configure addressing for all 6 pairs
  ```bash
  # Pair 0→1: fd00:3f:75:0::/64
  sudo ip netns exec monad0 ip -6 addr add fd00:3f:75:0::1/64 dev veth01
  sudo ip netns exec monad1 ip -6 addr add fd00:3f:75:0::2/64 dev veth01p

  # Pair 1→2: fd00:3f:75:1::/64
  sudo ip netns exec monad1 ip -6 addr add fd00:3f:75:1::1/64 dev veth12
  sudo ip netns exec monad2 ip -6 addr add fd00:3f:75:1::2/64 dev veth12p

  # Pair 2→3: fd00:3f:75:2::/64
  sudo ip netns exec monad2 ip -6 addr add fd00:3f:75:2::1/64 dev veth23
  sudo ip netns exec monad3 ip -6 addr add fd00:3f:75:2::2/64 dev veth23p

  # Pair 3→4: fd00:3f:75:3::/64
  sudo ip netns exec monad3 ip -6 addr add fd00:3f:75:3::1/64 dev veth34
  sudo ip netns exec monad4 ip -6 addr add fd00:3f:75:3::2/64 dev veth34p

  # Pair 4→5: fd00:3f:75:4::/64
  sudo ip netns exec monad4 ip -6 addr add fd00:3f:75:4::1/64 dev veth45
  sudo ip netns exec monad5 ip -6 addr add fd00:3f:75:4::2/64 dev veth45p

  # Pair 5→0: fd00:3f:75:5::/64
  sudo ip netns exec monad5 ip -6 addr add fd00:3f:75:5::1/64 dev veth50
  sudo ip netns exec monad0 ip -6 addr add fd00:3f:75:5::2/64 dev veth50p
  ```

- [ ] **Step 45** [S][B]: Set MTU and bring all interfaces UP
  ```bash
  for i in 0 1 2 3 4 5; do
    j=$(( (i + 1) % 6 ))
    sudo ip netns exec "monad${i}" ip link set "veth${i}${j}" mtu 1500 up
    sudo ip netns exec "monad${j}" ip link set "veth${i}${j}p" mtu 1500 up
  done
  ```

- [ ] **Step 46** [S][B]: Disable DAD (Duplicate Address Detection) — speeds up convergence
  ```bash
  for i in 0 1 2 3 4 5; do
    j=$(( (i + 1) % 6 ))
    sudo ip netns exec "monad${i}" sysctl -qw "net.ipv6.conf.veth${i}${j}.accept_dad=0"
    sudo ip netns exec "monad${j}" sysctl -qw "net.ipv6.conf.veth${i}${j}p.accept_dad=0"
  done
  ```

### Configure Routing

- [ ] **Step 47** [S][B]: Enable IPv6 forwarding in all namespaces
  ```bash
  for i in 0 1 2 3 4 5; do
    sudo ip netns exec "monad${i}" sysctl -qw net.ipv6.conf.all.forwarding=1
  done
  ```

- [ ] **Step 48** [S][B]: Set default routes (each namespace forwards to next hop)
  ```bash
  for i in 0 1 2 3 4 5; do
    j=$(( (i + 1) % 6 ))
    sudo ip netns exec "monad${i}" ip -6 route replace default \
      via "fd00:3f:75:${i}::2" dev "veth${i}${j}"
  done
  ```

- [ ] **Step 49** [S][B]: Add static neighbor entries (bypass NDP for speed)
  ```bash
  for i in 0 1 2 3 4 5; do
    j=$(( (i + 1) % 6 ))
    peer_mac=$(sudo ip netns exec "monad${j}" ip link show "veth${i}${j}p" | awk '/ether/ {print $2}')
    if [ -n "$peer_mac" ]; then
      sudo ip netns exec "monad${i}" ip -6 neigh replace "fd00:3f:75:${i}::2" \
        lladdr "$peer_mac" dev "veth${i}${j}" nud permanent
      echo "monad${i} -> monad${j}: $peer_mac"
    fi
  done
  ```

### Verify Connectivity (No BPF Yet)

- [ ] **Step 50** [S][B]: Verify direct neighbor connectivity (each hop)
  ```bash
  for i in 0 1 2 3 4 5; do
    j=$(( (i + 1) % 6 ))
    echo -n "monad${i} -> monad${j}: "
    sudo ip netns exec "monad${i}" ping6 -c 1 -W 1 "fd00:3f:75:${i}::2" 2>&1 | grep -o "1 received" || echo "FAIL"
  done
  ```

- [ ] **Step 51** [V]: **NEIGHBOR PING GATE** — All 6 direct hops must show "1 received"

- [ ] **Step 52** [S][B]: Test multi-hop forwarding (monad0 to non-connected destination)
  ```bash
  # fd00:dead::1 is outside all connected /64 prefixes → forces default route forwarding
  sudo ip netns exec monad0 ping6 -c 3 -W 2 fd00:dead::1 2>&1 | tail -3
  ```

- [ ] **Step 53** [D]: If multi-hop fails, trace the path
  ```bash
  sudo ip netns exec monad0 traceroute6 -n fd00:dead::1 2>&1 | head -10
  ```

- [ ] **Step 54** [D]: Check routes in each namespace if forwarding fails
  ```bash
  for i in 0 1 2 3 4 5; do
    echo "=== monad${i} ==="
    sudo ip netns exec "monad${i}" ip -6 route show
  done
  ```

### Throughput Baseline (No BPF)

- [ ] **Step 55** [S][B]: Measure raw packet throughput without XDP
  ```bash
  # If nping available:
  sudo ip netns exec monad0 nping --udp -6 --rate 1000 -c 1000 fd00:3f:75:0::2 2>&1 | tail -5
  # Otherwise use ping flood:
  sudo ip netns exec monad0 ping6 -f -c 1000 -W 1 fd00:3f:75:0::2 2>&1 | tail -3
  ```

- [ ] **Step 56** [B]: Record baseline throughput
  ```bash
  echo "Baseline throughput (no BPF): [FILL IN] pkt/s" >> /tmp/doom-metrics.txt
  ```

### Ring Integrity

- [ ] **Step 57** [S][B]: Verify the ring topology visually
  ```bash
  echo "Ring topology:"
  for i in 0 1 2 3 4 5; do
    j=$(( (i + 1) % 6 ))
    addr=$(sudo ip netns exec "monad${i}" ip -6 addr show "veth${i}${j}" | grep "fd00" | awk '{print $2}')
    echo "  monad${i}/veth${i}${j} (${addr}) → monad${j}"
  done
  ```

- [ ] **Step 58** [S][B]: Verify no duplicate addresses
  ```bash
  for i in 0 1 2 3 4 5; do
    sudo ip netns exec "monad${i}" ip -6 addr show | grep "fd00"
  done | sort | uniq -d
  # Must be EMPTY (no duplicates)
  ```

- [ ] **Step 59** [V]: **NO DUPLICATE ADDRESSES** — uniq -d output must be empty

### Alternative: Use doom-ring.sh

- [ ] **Step 60** [S][B]: OR use the automated script (replaces Steps 36-59)
  ```bash
  sudo ./scripts/doom-ring.sh setup
  ```

- [ ] **Step 61** [S][B]: Verify setup via status
  ```bash
  sudo ./scripts/doom-ring.sh status
  ```

- [ ] **Step 62** [V]: All 6 namespaces [UP], all veth links [UP]

### Phase 2 Hardening

- [ ] **Step 63** [S][B]: Set kernel panic timeout for ring namespaces (optional, safety)
  ```bash
  for i in 0 1 2 3 4 5; do
    sudo ip netns exec "monad${i}" sysctl -qw net.ipv6.conf.all.accept_redirects=0
    sudo ip netns exec "monad${i}" sysctl -qw net.ipv6.conf.all.accept_ra=0
  done
  ```

- [ ] **Step 64** [B]: Save ring configuration for fast rebuild
  ```bash
  sudo ./scripts/doom-ring.sh status > /tmp/doom-ring-state.txt
  date >> /tmp/doom-ring-state.txt
  ```

- [ ] **Step 65** [V]: **PHASE 2 EXIT GATE** — 6 namespaces up, all pairs connected, direct pings pass, forwarding works

---

## PHASE 3: BPF PROGRAM LOADING & MAP PINNING (Steps 66-100)

**Goal**: Load monad-cpu-ebpf once, pin maps, attach to all 6 hops, verify shared state.
**Time**: 45-60 minutes
**Agent**: Coordinator (requires sudo, iterative debugging)

### Prepare BPF Filesystem

- [ ] **Step 66** [S][B]: Create BPF pin directories
  ```bash
  sudo mkdir -p /sys/fs/bpf/unheaded/doom-ring/maps
  ```

- [ ] **Step 67** [B]: Verify bpffs is mounted
  ```bash
  mount | grep bpf
  # Must show: bpf on /sys/fs/bpf type bpf
  ```

- [ ] **Step 68** [S][B]: Mount bpffs if not mounted
  ```bash
  sudo mount -t bpf bpf /sys/fs/bpf 2>/dev/null || true
  ```

### Load Primary Program (Hop 0)

- [ ] **Step 69** [B]: Verify BPF binary integrity
  ```bash
  file ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf
  # Expect: ELF 64-bit LSB relocatable, eBPF
  readelf -S ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf 2>/dev/null | grep -E 'maps|xdp'
  ```

- [ ] **Step 70** [S][B]: Load monad-cpu-ebpf on hop 0 via aya loader
  ```bash
  # The veth50p interface is the INGRESS for monad0 (packets arriving from monad5)
  sudo nsenter --net=/var/run/netns/monad0 \
    cmd/ebpf-loader/target/release/ebpf-loader \
    --only monad-cpu \
    --obj-dir ebpf/target/bpfel-unknown-none/release \
    --interface veth50p \
    --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps \
    --xdp-skb-mode \
    --pid-file /run/doom-ring/hop0.pid &
  sleep 1
  ```

- [ ] **Step 71** [V]: Verify loader is running
  ```bash
  cat /run/doom-ring/hop0.pid 2>/dev/null && echo "PID file exists" || echo "NO PID FILE"
  kill -0 $(cat /run/doom-ring/hop0.pid 2>/dev/null) 2>/dev/null && echo "Process alive" || echo "Process DEAD"
  ```

- [ ] **Step 72** [D]: If loader fails, check error output
  ```bash
  journalctl --since "5 minutes ago" | grep -i bpf | tail -20
  dmesg | tail -20 | grep -i bpf
  ```

- [ ] **Step 73** [D]: Common failure: aya loader --only flag format
  ```bash
  # Try alternative invocation if Step 70 fails:
  sudo nsenter --net=/var/run/netns/monad0 \
    cmd/ebpf-loader/target/release/ebpf-loader \
    --interface veth50p \
    --pin-maps \
    2>&1 | head -20
  ```

### Verify Pinned Maps

- [ ] **Step 74** [S][B]: List pinned maps
  ```bash
  ls -la /sys/fs/bpf/unheaded/doom-ring/maps/
  ```

- [ ] **Step 75** [V]: **MAP PIN GATE** — All 9 maps must be pinned:
  ```
  ROM_MAP, CPU_MAP, RAM_MAP, SCREEN_MAP, KBD_MAP, STATS, L1_CACHE, RV2MBC_MAP, COMPUTE_EVENTS
  ```

- [ ] **Step 76** [B]: Inspect map details
  ```bash
  for map in ROM_MAP CPU_MAP RAM_MAP SCREEN_MAP KBD_MAP STATS L1_CACHE; do
    echo "=== $map ==="
    sudo bpftool map show pinned "/sys/fs/bpf/unheaded/doom-ring/maps/${map}" 2>&1 | head -3
  done
  ```

- [ ] **Step 77** [D]: If maps not pinned, try manual pinning
  ```bash
  # Get map IDs from the loaded program
  sudo bpftool prog show | grep monad_cpu
  PROG_ID=$(sudo bpftool prog show | grep "name monad_cpu" | head -1 | awk '{print $1}' | tr -d ':')
  sudo bpftool prog show id $PROG_ID
  ```

### Get Program ID

- [ ] **Step 78** [S][B]: Find the loaded program ID
  ```bash
  PROG_ID=$(sudo bpftool prog list 2>/dev/null | grep "name monad_cpu" | head -1 | awk '{print $1}' | tr -d ':')
  echo "monad_cpu prog_id = ${PROG_ID}"
  ```

- [ ] **Step 79** [V]: PROG_ID must be a non-empty number

### Attach to Hops 1-5 (Shared Program)

- [ ] **Step 80** [S][B]: Attach to hop 1 (monad1, ingress = veth01p)
  ```bash
  sudo nsenter --net=/var/run/netns/monad1 \
    bpftool net attach xdpgeneric id $PROG_ID dev veth01p
  ```

- [ ] **Step 81** [S][B]: Attach to hop 2 (monad2, ingress = veth12p)
  ```bash
  sudo nsenter --net=/var/run/netns/monad2 \
    bpftool net attach xdpgeneric id $PROG_ID dev veth12p
  ```

- [ ] **Step 82** [S][B]: Attach to hop 3 (monad3, ingress = veth23p)
  ```bash
  sudo nsenter --net=/var/run/netns/monad3 \
    bpftool net attach xdpgeneric id $PROG_ID dev veth23p
  ```

- [ ] **Step 83** [S][B]: Attach to hop 4 (monad4, ingress = veth34p)
  ```bash
  sudo nsenter --net=/var/run/netns/monad4 \
    bpftool net attach xdpgeneric id $PROG_ID dev veth34p
  ```

- [ ] **Step 84** [S][B]: Attach to hop 5 (monad5, ingress = veth45p)
  ```bash
  sudo nsenter --net=/var/run/netns/monad5 \
    bpftool net attach xdpgeneric id $PROG_ID dev veth45p
  ```

- [ ] **Step 85** [V]: Verify all 6 XDP attachments
  ```bash
  for i in 0 1 2 3 4 5; do
    prev=$(( (i - 1 + 6) % 6 ))
    veth_in="veth${prev}${i}p"
    echo -n "hop${i} (monad${i}/${veth_in}): "
    sudo nsenter --net=/var/run/netns/monad${i} \
      ip link show "$veth_in" 2>/dev/null | grep -o "xdp[^ ]*" || echo "NO XDP"
  done
  ```

- [ ] **Step 86** [V]: **XDP GATE** — All 6 hops must show xdp attachment

### Verify Shared Maps

- [ ] **Step 87** [S][B]: Write a test value to STATS map from host
  ```bash
  # Write key=31 (test), value=42 to verify map accessibility
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS \
    key 31 0 0 0 value 42 0 0 0 0 0 0 0
  ```

- [ ] **Step 88** [S][B]: Read it back
  ```bash
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS \
    key 31 0 0 0
  # Expect: value 42 0 0 0 0 0 0 0
  ```

- [ ] **Step 89** [V]: **SHARED MAP GATE** — Written value readable (maps are truly shared)

- [ ] **Step 90** [S][B]: Clean up test value
  ```bash
  sudo bpftool map delete pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS \
    key 31 0 0 0 2>/dev/null || true
  ```

### Initialize CPU State

- [ ] **Step 91** [S][B]: Initialize CPU_MAP with default state for instance 0 (flow_label low byte = 0xDE)
  ```bash
  # Using doom CLI or direct bpftool
  # Key = instance_id (4 bytes, little-endian): 0xDE = 222
  # Value = MbcCpuState (104 bytes, all zeros except SP)
  go run ./cmd/doom/ reset 2>&1 || echo "Doom CLI reset"
  ```

- [ ] **Step 92** [D]: If doom CLI fails, use bpftool to write initial state
  ```bash
  # 104 bytes = 16 regs (64B) + pc (4B) + flags/halted/stalled/pad (4B) + sleep (8B) + counters (24B)
  # SP (r15) at offset 60 = 0x00 0x00 0xFF 0xFF (little-endian 0xFFFF0000)
  # All other fields = 0
  printf '\x00%.0s' {1..60} | xxd -p  # First 60 bytes = zeros (r0-r14)
  ```

- [ ] **Step 93** [S][B]: Verify CPU state readable
  ```bash
  go run ./cmd/doom/ status 2>&1 || echo "Status check"
  ```

- [ ] **Step 94** [D]: If status fails, check map directly
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP 2>&1 | head -20
  ```

### Load Trivial Test ROM

- [ ] **Step 95** [W]: Create a trivial 3-instruction test ROM
  ```bash
  # MOVI r0, 42    → opcode=0x0F, dst=0, imm16=42  → 0x0F00002A
  # MOVI r1, 1     → opcode=0x0F, dst=1, imm16=1   → 0x0F100001
  # ADD r0, r1     → opcode=0x01, dst=0, src=1      → 0x01010000
  # HALT           → opcode=0xFF                     → 0xFF000000
  python3 -c "
  import struct
  rom = [0x0F00002A, 0x0F100001, 0x01010000, 0xFF000000]
  with open('/tmp/trivial.mbc', 'wb') as f:
    for insn in rom:
      f.write(struct.pack('<I', insn))
  print(f'Wrote {len(rom)} instructions to /tmp/trivial.mbc')
  "
  ```

- [ ] **Step 96** [S][B]: Load trivial ROM into ROM_MAP
  ```bash
  go run ./cmd/doom/ load /tmp/trivial.mbc 2>&1
  ```

- [ ] **Step 97** [D]: If Go loader fails, load directly via bpftool
  ```bash
  # Manual ROM loading: key=index (u32 LE), value=instruction (u32 LE)
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP \
    key 0 0 0 0 value 0x2A 0x00 0x00 0x0F  # MOVI r0, 42
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP \
    key 1 0 0 0 value 0x01 0x00 0x10 0x0F  # MOVI r1, 1
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP \
    key 2 0 0 0 value 0x00 0x00 0x01 0x01  # ADD r0, r1
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP \
    key 3 0 0 0 value 0x00 0x00 0x00 0xFF  # HALT
  ```

- [ ] **Step 98** [S][B]: Verify ROM loaded
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP 2>&1 | head -20
  ```

- [ ] **Step 99** [V]: ROM entries match expected instruction words

- [ ] **Step 100** [V]: **PHASE 3 EXIT GATE** — Program loaded on all 6 hops, maps pinned and shared, CPU initialized, trivial ROM loaded

---

## PHASE 4: FIRST PACKET EXECUTION (Steps 101-135)

**Goal**: Send ONE packet through the ring and observe CPU state change. THE HELLO WORLD MOMENT.
**Time**: 1-2 hours (iterative debugging expected)
**Agent**: Coordinator

### The Moment of Truth

- [ ] **Step 101** [S][B]: Record CPU state BEFORE first packet
  ```bash
  echo "=== BEFORE ==="
  go run ./cmd/doom/ status 2>&1
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1
  ```

- [ ] **Step 102** [S][B]: Inject ONE packet using doom-tick.py
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1 --verbose --namespace monad0
  ```

- [ ] **Step 103** [D]: If doom-tick.py fails, use doom-ring.sh inject
  ```bash
  sudo ./scripts/doom-ring.sh inject 0xDE
  ```

- [ ] **Step 104** [S][B]: Read CPU state AFTER first packet (wait 100ms for processing)
  ```bash
  sleep 0.1
  echo "=== AFTER ==="
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 105** [V]: **FIRST PACKET GATE** — CPU state must have changed:
  - `PC` advanced (should be > 0)
  - `insn_count` > 0 (should be >= 1, up to MAX_INSN_PER_TICK=16)
  - `r0` should have a value (42 if MOVI executed)

### Debug First Packet (If State Unchanged)

- [ ] **Step 106** [D]: Check if packet reached XDP
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1
  # Key 0 = STAT_PACKETS_TOTAL — must be > 0
  # Key 1 = STAT_CPU_TICKS — must be > 0 (means CUSTOM flag was seen)
  ```

- [ ] **Step 107** [D]: If PACKETS_TOTAL=0, packet didn't reach XDP
  ```bash
  # Check TC statistics
  sudo ip netns exec monad0 tc -s qdisc show 2>&1
  # Check if packet was actually sent
  sudo ip netns exec monad0 ip -s link show veth01 2>&1 | grep -A2 "TX:"
  ```

- [ ] **Step 108** [D]: If PACKETS_TOTAL>0 but CPU_TICKS=0, CUSTOM flag not set
  ```bash
  # Verify the Monad flags field
  # In doom-tick.py, flags byte is at HBH offset + Monad offset 0x0B
  # CUSTOM flag = 0x02
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1 --dry-run 2>&1 | head -20
  ```

- [ ] **Step 109** [D]: If CPU_TICKS>0 but CPU state unchanged, flow_label key mismatch
  ```bash
  # monad-cpu-ebpf uses low 8 bits of flow label as instance_id
  # flow_label = 0xDE → instance_id = 0xDE = 222
  echo "Expected CPU_MAP key: 222 (0xDE)"
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP 2>&1
  ```

- [ ] **Step 110** [D]: Check BPF program trace logs
  ```bash
  sudo cat /sys/kernel/debug/tracing/trace_pipe &
  TRACE_PID=$!
  sleep 1
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1
  sleep 1
  kill $TRACE_PID 2>/dev/null
  ```

- [ ] **Step 111** [D]: Verify the packet structure matches what BPF expects
  ```bash
  # The BPF program parses:
  # 1. Ethernet (14 bytes) → check ethertype = 0x86DD
  # 2. IPv6 (40 bytes) → check next_header = 0 (HBH)
  # 3. HBH → scan for opt_type = 0x3E, opt_len = 20
  # 4. Monad → check version = 0x01, flags & 0x02 (CUSTOM)
  # 5. Flow label → low 8 bits = instance_id
  echo "Packet structure verification:"
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1 --dry-run 2>&1
  ```

### Multi-Packet Test

- [ ] **Step 112** [S][B]: Send 6 packets (one full circuit)
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 6 --rate 10 --verbose
  ```

- [ ] **Step 113** [S][B]: Check CPU state after 6 packets
  ```bash
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 114** [V]: insn_count should be ~6 × MAX_INSN_PER_TICK (up to 96 if 16 insns/tick)
  - For trivial ROM (4 instructions + HALT): should see halted=1 after a few packets

### HALT Test

- [ ] **Step 115** [S][B]: Reset CPU state
  ```bash
  go run ./cmd/doom/ reset 2>&1
  ```

- [ ] **Step 116** [S][B]: Send packets until HALT
  ```bash
  for i in $(seq 1 10); do
    sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1
    STATUS=$(go run ./cmd/doom/ status 2>&1)
    echo "Packet $i: $STATUS" | grep -E 'halted|insn_count'
    echo "$STATUS" | grep -q "halted.*true" && echo "HALTED at packet $i!" && break
  done
  ```

- [ ] **Step 117** [V]: **HALT GATE** — CPU must reach halted=1 state
  - For trivial ROM: HALT is instruction 3 (index 3)
  - With 16 insns/tick, first packet executes all 4 instructions including HALT

### Register Value Verification

- [ ] **Step 118** [S][B]: Reset and send single packet, check register values
  ```bash
  go run ./cmd/doom/ reset 2>&1
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1
  sleep 0.1
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 119** [V]: **REGISTER GATE** — After trivial ROM:
  - `r0 = 43` (42 + 1 from MOVI + ADD)
  - `r1 = 1` (from MOVI)
  - `halted = true`
  - `insn_count = 4` (MOVI, MOVI, ADD, HALT)

### Statistics Verification

- [ ] **Step 120** [S][B]: Dump full statistics
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1
  echo "Expected:"
  echo "  key 0 (PACKETS_TOTAL): > 0"
  echo "  key 1 (CPU_TICKS): > 0"
  echo "  key 2 (INSNS_EXECUTED): 4 (for trivial ROM)"
  echo "  key 3 (HALTED): 1"
  ```

### Instruction Counter Test

- [ ] **Step 121** [S][B]: Load a longer test ROM (10 NOPs + HALT)
  ```bash
  python3 -c "
  import struct
  # 10 NOPs (0x00000000) + HALT (0xFF000000)
  rom = [0x00000000]*10 + [0xFF000000]
  with open('/tmp/nop10.mbc', 'wb') as f:
    for insn in rom:
      f.write(struct.pack('<I', insn))
  print(f'Wrote {len(rom)} instructions')
  "
  go run ./cmd/doom/ load /tmp/nop10.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  ```

- [ ] **Step 122** [S][B]: Send 1 packet, verify 11 instructions executed (10 NOP + HALT)
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1
  sleep 0.1
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 123** [V]: insn_count = 11, halted = true

### Rapid Injection Test

- [ ] **Step 124** [S][B]: Load a ROM that runs longer than 16 instructions
  ```bash
  python3 -c "
  import struct
  # 100 NOPs + HALT
  rom = [0x00000000]*100 + [0xFF000000]
  with open('/tmp/nop100.mbc', 'wb') as f:
    for insn in rom:
      f.write(struct.pack('<I', insn))
  "
  go run ./cmd/doom/ load /tmp/nop100.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  ```

- [ ] **Step 125** [S][B]: Send 10 packets rapidly
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 10 --burst
  sleep 0.2
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 126** [V]: insn_count should be 101 (100 NOPs + HALT), halted = true
  - With 16 insns/tick, need ceil(101/16) = 7 packets minimum
  - 10 packets should be enough

### Event Ring Buffer Test

- [ ] **Step 127** [S][B]: Check COMPUTE_EVENTS ring buffer
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/COMPUTE_EVENTS 2>&1 | head -30
  ```

- [ ] **Step 128** [V]: Should see HALT event (EVENT_COMPUTE_HALT = 0x16)

### Multi-Instance Test

- [ ] **Step 129** [S][B]: Test a second CPU instance (different flow label)
  ```bash
  # Instance 0xBE = 190
  go run ./cmd/doom/ reset 2>&1  # Reset instance 0xDE
  sudo python3 scripts/doom-tick.py --flow-label 0xBE --count 1
  sleep 0.1
  # Check: instance 0xBE should have NO state (no CPU_MAP entry for key 190)
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP \
    key 190 0 0 0 2>&1
  ```

- [ ] **Step 130** [V]: Instance 0xBE lookup should fail (no state) or show defaults

### Document Results

- [ ] **Step 131** [B]: Record first-packet metrics
  ```bash
  cat >> /tmp/doom-metrics.txt << 'EOF'
  === FIRST PACKET RESULTS ===
  Trivial ROM (4 insns): [FILL IN]
  NOP-100 ROM (101 insns): [FILL IN]
  Packets needed for NOP-100: [FILL IN]
  insns/packet: [FILL IN]
  EOF
  ```

### Commit Checkpoint

- [ ] **Step 132** [B]: Stage any script improvements
  ```bash
  git add -A scripts/doom-tick.py
  git status
  ```

- [ ] **Step 133** [B]: Commit checkpoint
  ```bash
  git commit -m "feat(doom): first packet execution verified — CPU state mutates over IPv6

  Trivial ROM executes correctly: MOVI r0,42; MOVI r1,1; ADD r0,r1; HALT
  Results: r0=43, halted=true, insn_count=4. Ring topology operational.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 134** [V]: Commit succeeds

- [ ] **Step 135** [V]: **PHASE 4 EXIT GATE** — Single packet causes CPU state mutation, registers correct, HALT works, statistics reporting, event ring populating

---

## PHASE 5: ASSEMBLER PIPELINE VERIFICATION (Steps 136-160)

**Goal**: Verify the assembler → ROM → BPF → execution pipeline end-to-end.
**Time**: 30-45 minutes
**Agent**: Coordinator or Agent

### Simple Assembler Test

- [ ] **Step 136** [W]: Write a test assembly program
  ```bash
  cat > /tmp/test-fibonacci.asm << 'ASM'
  ; Fibonacci sequence — compute fib(10) in r0
  ; Expected result: r0 = 55
      movi r0, 0       ; fib(0) = 0
      movi r1, 1       ; fib(1) = 1
      movi r2, 10      ; counter
  loop:
      mov r3, r1       ; temp = b
      add r1, r0       ; b = a + b
      mov r0, r3       ; a = temp
      addi r2, -1      ; counter--
      jnz loop         ; if counter != 0, loop
      halt
  ASM
  ```

- [ ] **Step 137** [B]: Assemble to binary
  ```bash
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --bin mbc_asm -- \
    /tmp/test-fibonacci.asm /tmp/test-fibonacci.mbc 2>&1
  ```

- [ ] **Step 138** [B]: Disassemble to verify
  ```bash
  xxd /tmp/test-fibonacci.mbc | head -5
  ```

- [ ] **Step 139** [S][B]: Load into ring and execute
  ```bash
  go run ./cmd/doom/ load /tmp/test-fibonacci.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  # Fibonacci(10) = 55, needs ~80 instructions → 5 packets at 16 insns/tick
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 10 --burst
  sleep 0.2
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 140** [V]: **FIBONACCI GATE** — `r0 = 55` (Fibonacci of 10), halted = true

### Memory Operations Test

- [ ] **Step 141** [W]: Write a memory test program
  ```bash
  cat > /tmp/test-memory.asm << 'ASM'
  ; Memory store/load test
  ; Store 0xDEAD to addr 0x100, load it back to r0
      movi r0, 0xDEAD   ; value to store
      movi r1, 0x100     ; address
      st r1, r0          ; mem[0x100] = 0xDEAD
      movi r0, 0         ; clear r0
      ld r0, r1          ; r0 = mem[0x100]
      halt               ; r0 should be 0xDEAD
  ASM
  ```

- [ ] **Step 142** [B]: Assemble
  ```bash
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --bin mbc_asm -- \
    /tmp/test-memory.asm /tmp/test-memory.mbc 2>&1
  ```

- [ ] **Step 143** [S][B]: Load and execute
  ```bash
  go run ./cmd/doom/ load /tmp/test-memory.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 5 --burst
  sleep 0.2
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 144** [V]: r0 = 0xDEAD (57005 decimal), halted = true

### Screen Write Test

- [ ] **Step 145** [W]: Write a screen pattern test
  ```bash
  cat > /tmp/test-screen.asm << 'ASM'
  ; Write a pattern to screen buffer (first 4 pixels)
  ; SCREEN_BASE = 0xC000
      movi r0, 0xFF       ; white pixel
      movi r1, 0xC000     ; screen base address (word-addressed at 0x3000)
      stb r1, r0          ; screen[0] = 0xFF
      addi r1, 1
      stb r1, r0          ; screen[1] = 0xFF
      addi r1, 1
      stb r1, r0          ; screen[2] = 0xFF
      addi r1, 1
      stb r1, r0          ; screen[3] = 0xFF
      ; Signal draw
      movi r0, 1          ; SYS_DRAW_FRAME
      movi r1, 0xC000     ; framebuffer pointer
      syscall
      halt
  ASM
  ```

- [ ] **Step 146** [B]: Assemble
  ```bash
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --bin mbc_asm -- \
    /tmp/test-screen.asm /tmp/test-screen.mbc 2>&1
  ```

- [ ] **Step 147** [S][B]: Load and execute
  ```bash
  go run ./cmd/doom/ load /tmp/test-screen.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 5 --burst
  sleep 0.2
  ```

- [ ] **Step 148** [S][B]: Check SCREEN_MAP for pixel data
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP 2>&1 | head -10
  ```

- [ ] **Step 149** [V]: SCREEN_MAP should show non-zero values at indices 0-3

### Keyboard Input Test

- [ ] **Step 150** [W]: Write a keyboard test program
  ```bash
  cat > /tmp/test-kbd.asm << 'ASM'
  ; Read keyboard state
  ; SYS_GET_KEY (0x02) returns scancode in r0, pressed in r1
      movi r0, 2          ; SYS_GET_KEY
      syscall
      ; r0 now has scancode, r1 has pressed flag
      halt
  ASM
  ```

- [ ] **Step 151** [B]: Assemble
  ```bash
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --bin mbc_asm -- \
    /tmp/test-kbd.asm /tmp/test-kbd.mbc 2>&1
  ```

- [ ] **Step 152** [S][B]: Inject keyboard state, then execute
  ```bash
  # Write keyboard state: key=0x41 (A), pressed=1
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP \
    key 0 0 0 0 value 0x41 0 0 0
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP \
    key 1 0 0 0 value 1 0 0 0

  go run ./cmd/doom/ load /tmp/test-kbd.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 3 --burst
  sleep 0.2
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 153** [V]: r0 = 0x41 (scancode), r1 = 1 (pressed)

### CALL/RET Test

- [ ] **Step 154** [W]: Write a subroutine test
  ```bash
  cat > /tmp/test-call.asm << 'ASM'
  ; Subroutine call/return test
  ; SP must be set within RAM bounds for userspace compatibility
      movi r15, 0x1000    ; Set SP within RAM
      movi r0, 10
      call add_five       ; Call subroutine
      halt                ; r0 should be 15

  add_five:
      addi r0, 5
      ret
  ASM
  ```

- [ ] **Step 155** [B]: Assemble
  ```bash
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --bin mbc_asm -- \
    /tmp/test-call.asm /tmp/test-call.mbc 2>&1
  ```

- [ ] **Step 156** [S][B]: Load and execute
  ```bash
  go run ./cmd/doom/ load /tmp/test-call.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 5 --burst
  sleep 0.2
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 157** [V]: r0 = 15, halted = true

### Performance Measurement

- [ ] **Step 158** [S][B]: Load NOP-1000 and time execution
  ```bash
  python3 -c "
  import struct
  rom = [0x00000000]*1000 + [0xFF000000]
  with open('/tmp/nop1000.mbc', 'wb') as f:
    for insn in rom: f.write(struct.pack('<I', insn))
  "
  go run ./cmd/doom/ load /tmp/nop1000.mbc 2>&1
  go run ./cmd/doom/ reset 2>&1
  time sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 100 --burst
  sleep 0.5
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 159** [B]: Calculate instructions/second
  ```bash
  echo "1001 instructions / elapsed_time = insns/sec"
  echo "Record this metric for Phase 18 comparison"
  ```

- [ ] **Step 160** [V]: **PHASE 5 EXIT GATE** — Assembler pipeline works end-to-end, Fibonacci correct, memory ops work, screen writes visible, keyboard reads work, CALL/RET functional

---

---

## PHASE 6: LIBC STUB PATCHING (Steps 161-185)

**Goal**: Fix the known access() → -1 → Doom exits blocker. Patch all libc stubs needed for BSS init.
**Time**: 1-2 hours
**Agent**: Agent (can parallel with Phase 7 prep)

### Identify the Problem

- [ ] **Step 161** [R]: Review S30 handoff notes on the access() blocker
  ```
  Known: Doom halts at exit() after 59.8M insns if access() returns -1
  Root cause: doomgeneric uses access() to check for IWAD files (.wad)
  Fix: make access() return 0 for ".wad" paths in libc_monad.c
  ```

- [ ] **Step 162** [B]: Find libc_monad.c or equivalent stubs
  ```bash
  find . -name 'libc_monad*' -o -name 'libc_stub*' -o -name 'syscall_stub*' 2>/dev/null
  find . -path '*/doom*' -name '*.c' 2>/dev/null
  ```

- [ ] **Step 163** [R]: If libc_monad.c exists, read the access() stub
  ```bash
  grep -n 'access' $(find . -name 'libc_monad*' 2>/dev/null) 2>/dev/null || echo "Not found"
  ```

- [ ] **Step 164** [D]: If no libc stub file exists, check doomgeneric source
  ```bash
  find . -path '*/doomgeneric*' -name '*.c' 2>/dev/null | head -10
  grep -rn 'access(' doom/ 2>/dev/null | head -10
  ```

### Patch access() Stub

- [ ] **Step 165** [W]: Create or update libc_monad.c with proper access() stub
  ```c
  // libc_monad.c — libc stubs for Doom running on MBC
  //
  // These stubs provide minimal POSIX-like behavior for the
  // doomgeneric port running in the MBC virtual machine.
  // The real filesystem is simulated via Wotan memory.

  #include <stddef.h>

  // access() — always succeed for .wad files (IWAD discovery)
  int access(const char *pathname, int mode) {
      if (pathname == NULL) return -1;

      // Check if path ends with ".wad" (case-insensitive)
      const char *dot = pathname;
      while (*dot) dot++;
      if (dot - pathname >= 4) {
          dot -= 4;
          if ((dot[0] == '.' || dot[0] == '.') &&
              (dot[1] == 'w' || dot[1] == 'W') &&
              (dot[2] == 'a' || dot[2] == 'A') &&
              (dot[3] == 'd' || dot[3] == 'D')) {
              return 0;  // WAD file exists (simulated)
          }
      }
      return -1;  // All other files: not found
  }

  // open() — return a fake file descriptor for WAD files
  int open(const char *pathname, int flags, ...) {
      // WAD data is memory-mapped at WAD_BASE (0x00010000)
      // Return fd=3 (first available after stdin/stdout/stderr)
      return 3;
  }

  // read() — read from WAD memory region
  // In MBC, WAD data is at addresses 0x00010000-0x0040FFFF
  typedef unsigned int uint32_t;
  typedef int ssize_t;
  typedef unsigned int size_t;

  static uint32_t wad_offset = 0;

  ssize_t read(int fd, void *buf, size_t count) {
      if (fd != 3) return -1;
      // Read from WAD region via normal memory access
      // The MBC executor maps WAD_BASE transparently
      unsigned char *dst = (unsigned char *)buf;
      unsigned char *src = (unsigned char *)(0x00010000 + wad_offset);
      for (size_t i = 0; i < count; i++) {
          dst[i] = src[i];
      }
      wad_offset += count;
      return (ssize_t)count;
  }

  // lseek() — seek within WAD file
  typedef long off_t;
  off_t lseek(int fd, off_t offset, int whence) {
      if (fd != 3) return -1;
      switch (whence) {
          case 0: wad_offset = (uint32_t)offset; break;       // SEEK_SET
          case 1: wad_offset += (uint32_t)offset; break;      // SEEK_CUR
          case 2: wad_offset = 4*1024*1024 + (uint32_t)offset; break; // SEEK_END (4MB max)
      }
      return (off_t)wad_offset;
  }

  // close() — no-op
  int close(int fd) { return 0; }

  // malloc()/free() — simple bump allocator in RAM
  static uint32_t heap_ptr = 0x8000;  // Start heap at 32KB

  void *malloc(size_t size) {
      // Align to 4 bytes
      size = (size + 3) & ~3;
      uint32_t ptr = heap_ptr;
      heap_ptr += size;
      if (heap_ptr > 0xBFFF) return NULL;  // Out of RAM (before screen region)
      return (void *)ptr;
  }

  void free(void *ptr) {
      // No-op — bump allocator, no deallocation
      (void)ptr;
  }

  void *calloc(size_t nmemb, size_t size) {
      size_t total = nmemb * size;
      void *p = malloc(total);
      if (p) {
          unsigned char *b = (unsigned char *)p;
          for (size_t i = 0; i < total; i++) b[i] = 0;
      }
      return p;
  }

  void *realloc(void *ptr, size_t size) {
      // Simple: always allocate new (no deallocation)
      return malloc(size);
  }

  // printf/fprintf — no-op (no console in MBC)
  int printf(const char *fmt, ...) { return 0; }
  int fprintf(void *stream, const char *fmt, ...) { return 0; }
  int sprintf(char *str, const char *fmt, ...) { return 0; }
  int snprintf(char *str, size_t size, const char *fmt, ...) { return 0; }

  // exit() — halt the CPU
  void exit(int status) {
      // Trigger HALT instruction via inline assembly or syscall
      __asm__ volatile("" ::: "memory");  // Memory barrier
      // In MBC, we'd do SYSCALL with a halt indicator
      // For now, infinite loop (MBC will exhaust cycle budget)
      while(1) {}
  }

  // memset/memcpy — provided by compiler builtins normally
  void *memset(void *s, int c, size_t n) {
      unsigned char *p = (unsigned char *)s;
      for (size_t i = 0; i < n; i++) p[i] = (unsigned char)c;
      return s;
  }

  void *memcpy(void *dest, const void *src, size_t n) {
      unsigned char *d = (unsigned char *)dest;
      const unsigned char *s = (const unsigned char *)src;
      for (size_t i = 0; i < n; i++) d[i] = s[i];
      return dest;
  }

  int memcmp(const void *s1, const void *s2, size_t n) {
      const unsigned char *a = (const unsigned char *)s1;
      const unsigned char *b = (const unsigned char *)s2;
      for (size_t i = 0; i < n; i++) {
          if (a[i] != b[i]) return a[i] - b[i];
      }
      return 0;
  }

  // strlen
  size_t strlen(const char *s) {
      size_t len = 0;
      while (s[len]) len++;
      return len;
  }

  // strcmp/strncmp
  int strcmp(const char *s1, const char *s2) {
      while (*s1 && *s1 == *s2) { s1++; s2++; }
      return *(unsigned char *)s1 - *(unsigned char *)s2;
  }

  int strncmp(const char *s1, const char *s2, size_t n) {
      for (size_t i = 0; i < n; i++) {
          if (s1[i] != s2[i] || !s1[i]) return (unsigned char)s1[i] - (unsigned char)s2[i];
      }
      return 0;
  }
  ```

- [ ] **Step 166** [B]: Save libc_monad.c to the doom directory
  ```bash
  # Location: doom/doomgeneric/libc_monad.c or similar
  ```

### Rebuild Doom ROM with Patched Stubs

- [ ] **Step 167** [B]: Find the Doom build system
  ```bash
  find . -path '*/doom*' -name 'Makefile' -o -path '*/doom*' -name 'CMakeLists.txt' 2>/dev/null
  ```

- [ ] **Step 168** [B]: Rebuild doomgeneric with patched libc
  ```bash
  # This depends on the build system found in Step 167
  # Typically: make -C doom/doomgeneric CROSS=riscv32-unknown-elf-
  ```

- [ ] **Step 169** [B]: Translate rebuilt ELF to MBC
  ```bash
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --bin rv32i_to_mbc -- \
    doom/doomgeneric/doom_rv32i.elf /tmp/doom-patched.mbc 2>&1
  ```

- [ ] **Step 170** [V]: doom-patched.mbc exists and is reasonable size
  ```bash
  ls -la /tmp/doom-patched.mbc
  wc -c /tmp/doom-patched.mbc
  ```

### Verify Stubs in Userspace First

- [ ] **Step 171** [B]: Run patched Doom in userspace emulator (quick sanity check)
  ```bash
  # Use monad-mbc userspace CPU
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --example doom_test -- \
    /tmp/doom-patched.mbc --max-cycles 1000000 2>&1 | tail -20
  ```

- [ ] **Step 172** [V]: Doom should survive past the access() check (>59.8M insns or different halt point)

### Alternative: Patch in Assembler/Translator

- [ ] **Step 173** [D]: If C rebuild isn't available, patch at the MBC level
  ```bash
  # The RV32I→MBC translator can intercept access() calls
  # Find the access() symbol address in the ELF
  readelf -s doom/doomgeneric/doom_rv32i.elf 2>/dev/null | grep access
  ```

- [ ] **Step 174** [D]: Patch the translated MBC to return 0 from access()
  ```bash
  # This requires knowing the exact PC of the access() function
  # and replacing its body with: MOVI r0, 0; RET
  ```

### Additional Stubs Audit

- [ ] **Step 175** [B]: Identify all unimplemented syscalls Doom might call
  ```bash
  # Run Doom in userspace with syscall tracing
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --example doom_test -- \
    /tmp/doom-patched.mbc --max-cycles 100000000 --trace-syscalls 2>&1 | \
    grep -i "syscall\|unimplemented\|unknown" | sort | uniq -c | sort -rn | head -20
  ```

- [ ] **Step 176** [W]: Create stub list document
  ```bash
  # Document all required stubs in docs/protocol/doom-libc-stubs.md
  ```

### Time Stub (SYS_GET_TICKS)

- [ ] **Step 177** [V]: Verify SYS_GET_TICKS returns meaningful time
  ```bash
  # In BPF: bpf_ktime_get_ns() / 1_000_000 → milliseconds
  # Verify this works by checking the return value after execution
  ```

### Sleep Stub (SYS_SLEEP)

- [ ] **Step 178** [V]: Verify SYS_SLEEP sets sleep_until_ns correctly
  ```bash
  # SYS_SLEEP(r1=ms) sets sleep_until_ns = ktime + r1*1_000_000
  # The BPF executor skips the CPU if current_time < sleep_until_ns
  ```

### Draw Frame Stub

- [ ] **Step 179** [V]: Verify SYS_DRAW_FRAME emits SCREEN_WRITE event
  ```bash
  # The BPF emits EVENT_SCREEN_WRITE to COMPUTE_EVENTS ring
  # Dashboard reads SCREEN_MAP on event
  ```

### Integration Test

- [ ] **Step 180** [B]: Run all Rust tests to verify stubs don't break anything
  ```bash
  cargo test --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | tail -10
  ```

- [ ] **Step 181** [B]: Run all Go tests
  ```bash
  go test ./internal/doom/... 2>&1 | tail -10
  ```

- [ ] **Step 182** [V]: All tests pass

### Commit Stubs

- [ ] **Step 183** [B]: Stage and commit libc stubs
  ```bash
  git add doom/doomgeneric/libc_monad.c docs/protocol/doom-libc-stubs.md
  git commit -m "feat(doom): add libc stubs for MBC — access() returns 0 for .wad files

  Fixes known blocker where Doom exits at 59.8M insns because access()
  returns -1 for IWAD discovery. Includes stubs for: access, open, read,
  lseek, close, malloc, free, memset, memcpy, printf, exit.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 184** [V]: Commit succeeds

- [ ] **Step 185** [V]: **PHASE 6 EXIT GATE** — libc stubs patched, access() returns 0 for .wad, Doom survives past IWAD check in userspace

---

## PHASE 7: DOOM ROM LOADING INTO BPF MAPS (Steps 186-215)

**Goal**: Load the full doom.mbc ROM into ROM_MAP and initialize all required BPF maps.
**Time**: 1-2 hours
**Agent**: Coordinator

### ROM Preparation

- [ ] **Step 186** [B]: Check for existing doom.mbc
  ```bash
  find . -name 'doom.mbc' -o -name 'doom_*.mbc' 2>/dev/null
  ls -la doom/doomgeneric/doom.mbc 2>/dev/null
  ```

- [ ] **Step 187** [B]: Verify ROM size vs ROM_MAP capacity
  ```bash
  # ROM_MAP has 262,144 entries (1 MiB of instructions)
  ROM_SIZE=$(wc -c < doom/doomgeneric/doom.mbc 2>/dev/null || echo 0)
  ROM_INSNS=$((ROM_SIZE / 4))
  echo "ROM: ${ROM_INSNS} instructions (${ROM_SIZE} bytes)"
  echo "ROM_MAP capacity: 262,144 instructions"
  if [ $ROM_INSNS -gt 262144 ]; then
    echo "WARNING: ROM exceeds ROM_MAP capacity!"
  else
    echo "OK: ROM fits in ROM_MAP ($(( (ROM_INSNS * 100) / 262144 ))% full)"
  fi
  ```

- [ ] **Step 188** [V]: ROM fits in ROM_MAP (< 262,144 instructions)

### Load ROM into BPF Map

- [ ] **Step 189** [S][B]: Load ROM via Go CLI
  ```bash
  go run ./cmd/doom/ load doom/doomgeneric/doom.mbc 2>&1
  ```

- [ ] **Step 190** [D]: If Go loader fails, diagnose
  ```bash
  # Check if the Go doom package can find pinned maps
  ls -la /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP
  ```

- [ ] **Step 191** [D]: Manual ROM loading fallback (Python)
  ```bash
  python3 -c "
  import struct, subprocess
  with open('doom/doomgeneric/doom.mbc', 'rb') as f:
    data = f.read()
  insns = len(data) // 4
  print(f'Loading {insns} instructions...')
  for i in range(min(insns, 262144)):
    word = struct.unpack_from('<I', data, i*4)[0]
    key = struct.pack('<I', i)
    val = struct.pack('<I', word)
    key_hex = ' '.join(f'{b}' for b in key)
    val_hex = ' '.join(f'{b}' for b in val)
    subprocess.run(['bpftool', 'map', 'update', 'pinned',
      '/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP',
      'key'] + key_hex.split() + ['value'] + val_hex.split(),
      capture_output=True)
    if i % 10000 == 0:
      print(f'  Loaded {i}/{insns} ({i*100//insns}%)')
  print(f'Done: {insns} instructions loaded')
  "
  ```

- [ ] **Step 192** [S][B]: Verify ROM loaded (spot-check first 5 entries)
  ```bash
  for i in 0 1 2 3 4; do
    echo -n "ROM[$i]: "
    sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP \
      key $i 0 0 0 2>&1
  done
  ```

- [ ] **Step 193** [V]: ROM entries non-zero and match expected instruction words

### Load RV2MBC Translation Table

- [ ] **Step 194** [B]: Check for RV2MBC translation data
  ```bash
  find . -name 'rv2mbc*' -o -name '*translation*' 2>/dev/null | head -10
  ```

- [ ] **Step 195** [S][B]: Load RV2MBC_MAP (needed for indirect jumps/calls)
  ```bash
  # The RV32I→MBC translator generates address mapping
  # RV2MBC_MAP[rv32i_addr] = mbc_addr
  go run ./cmd/doom/ load-rv2mbc 2>&1 || echo "May need manual loading"
  ```

- [ ] **Step 196** [D]: If no RV2MBC data, check if translator outputs it
  ```bash
  cargo run --manifest-path crates/monad-mbc/Cargo.toml --bin rv32i_to_mbc -- --help 2>&1
  ```

### Initialize CPU State for Doom

- [ ] **Step 197** [S][B]: Reset CPU with Doom-specific settings
  ```bash
  # SP = 0xFFFF_0000 (in BPF, HashMap handles any address)
  # PC = 0 (start of ROM)
  go run ./cmd/doom/ reset 2>&1
  ```

- [ ] **Step 198** [S][B]: Verify initial state
  ```bash
  go run ./cmd/doom/ status 2>&1
  # Expected: PC=0, r15(SP)=0xFFFF0000, halted=false, insn_count=0
  ```

### WAD Data Loading (If Needed)

- [ ] **Step 199** [B]: Check for DOOM1.WAD or equivalent
  ```bash
  find . -name '*.wad' -o -name '*.WAD' 2>/dev/null
  ```

- [ ] **Step 200** [D]: If WAD exists, load into RAM at WAD_BASE (0x10000)
  ```bash
  # WAD data goes into RAM_MAP at addresses 0x10000-0x40FFFF
  # This is 4 MiB max
  WAD_FILE=$(find . -name '*.wad' | head -1)
  if [ -n "$WAD_FILE" ]; then
    WAD_SIZE=$(wc -c < "$WAD_FILE")
    echo "Loading WAD: $WAD_FILE ($WAD_SIZE bytes)"
    # Load via Go CLI or Python script
  fi
  ```

- [ ] **Step 201** [D]: If no WAD file, Doom will attempt IWAD discovery
  ```bash
  echo "No WAD file found. Doom will rely on access() stub returning 0"
  echo "And may need a WAD file at runtime. Check if shareware WAD is bundled."
  ```

### Pre-Execution Validation

- [ ] **Step 202** [S][B]: Full map state dump before execution
  ```bash
  echo "=== PRE-EXECUTION STATE ==="
  echo "ROM_MAP entries:"
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP 2>&1 | wc -l
  echo "CPU_MAP:"
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP 2>&1
  echo "STATS:"
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1
  ```

### Begin Doom Execution (BSS Clearing Phase)

- [ ] **Step 203** [S][B]: Start injecting packets at maximum rate
  ```bash
  # BSS clearing: ~60M instructions = ~14,700 packets at MAX_INSN_PER_TICK=16
  # But actually: each packet executes in each of 6 hops = 6 instructions/packet?
  # OR: each hop sees the packet = 6 executions per circuit?
  # Need to verify: does the BPF execute once per hop (6x) or once total?
  echo "Starting BSS clearing phase..."
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 100000 --burst &
  INJECT_PID=$!
  echo "Injection PID: $INJECT_PID"
  ```

- [ ] **Step 204** [S][B]: Monitor progress every 5 seconds
  ```bash
  for i in $(seq 1 20); do
    sleep 5
    INSN=$(go run ./cmd/doom/ status 2>&1 | grep insn_count | awk '{print $NF}')
    HALTED=$(go run ./cmd/doom/ status 2>&1 | grep halted)
    echo "T+${i}x5s: insn_count=${INSN} ${HALTED}"
    echo "$HALTED" | grep -q "true" && echo "HALTED!" && kill $INJECT_PID 2>/dev/null && break
  done
  ```

- [ ] **Step 205** [V]: Monitor for one of:
  - insn_count grows steadily → BSS clearing in progress
  - halted=true at ~59.8M → access() fix didn't work (PHASE 6 failed)
  - halted=true at higher count → Doom hit another issue (investigate)

### BSS Completion Detection

- [ ] **Step 206** [S][B]: If not halted after 60M+ insns, BSS may be complete
  ```bash
  INSN=$(go run ./cmd/doom/ status 2>&1 | grep insn_count | awk '{print $NF}')
  if [ "$INSN" -gt 60000000 ]; then
    echo "Past BSS clearing point (60M insns)"
    echo "Doom should be entering main initialization"
  fi
  ```

- [ ] **Step 207** [S][B]: Check SCREEN_MAP for any non-zero data
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP 2>&1 | \
    grep -v "00 00 00 00" | head -10
  # Non-zero data = Doom is writing to screen!
  ```

- [ ] **Step 208** [V]: **SCREEN DATA GATE** — If SCREEN_MAP has non-zero entries, Doom is rendering

### Handle Doom Halt/Crash

- [ ] **Step 209** [D]: If Doom halts unexpectedly, capture state
  ```bash
  echo "=== DOOM HALT STATE ==="
  go run ./cmd/doom/ status 2>&1
  # Check the PC value to determine where Doom halted
  # Cross-reference with MBC disassembly
  ```

- [ ] **Step 210** [D]: Disassemble around the halt point
  ```bash
  PC=$(go run ./cmd/doom/ status 2>&1 | grep "pc" | awk '{print $NF}')
  echo "Doom halted at PC=${PC}"
  # Read ROM instructions around PC
  for offset in -5 -4 -3 -2 -1 0 1 2 3 4 5; do
    ADDR=$((PC + offset))
    if [ $ADDR -ge 0 ]; then
      sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP \
        key $(printf '%d 0 0 0' $ADDR) 2>&1
    fi
  done
  ```

- [ ] **Step 211** [D]: If halt is from unimplemented syscall, add stub and retry
  ```bash
  # Check r0 (syscall number) at halt point
  R0=$(go run ./cmd/doom/ status 2>&1 | grep "r0" | awk '{print $NF}')
  echo "Syscall number at halt: ${R0}"
  ```

### Kill Injection & Save State

- [ ] **Step 212** [B]: Stop packet injection
  ```bash
  kill $INJECT_PID 2>/dev/null || true
  pkill -f doom-tick 2>/dev/null || true
  ```

- [ ] **Step 213** [S][B]: Save final state snapshot
  ```bash
  echo "=== DOOM EXECUTION SNAPSHOT ==="
  go run ./cmd/doom/ status 2>&1 | tee /tmp/doom-state-phase7.txt
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1 | \
    tee /tmp/doom-stats-phase7.txt
  ```

- [ ] **Step 214** [B]: Commit progress
  ```bash
  git add -A
  git commit -m "feat(doom): ROM loaded, BSS clearing [in progress/complete]

  Doom ROM loaded into ROM_MAP, CPU initialized, packet injection running.
  BSS clearing progress: [FILL IN] instructions executed.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 215** [V]: **PHASE 7 EXIT GATE** — doom.mbc loaded in ROM_MAP, CPU initialized, BSS clearing attempted, screen data check performed

---

## PHASE 8: DOOM MAIN LOOP ENTRY (Steps 216-245)

**Goal**: Get Doom past BSS init and into the main game loop. First frame to SCREEN_MAP.
**Time**: 2-4 hours (highly iterative — expect multiple halt/fix/retry cycles)
**Agent**: Coordinator (debugging-intensive)

### Resume Execution

- [ ] **Step 216** [S][B]: Check if Doom is halted or still running
  ```bash
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 217** [S][B]: If halted, check if it's a fixable issue
  ```bash
  PC=$(go run ./cmd/doom/ status 2>&1 | grep "pc" | awk '{print $NF}')
  R0=$(go run ./cmd/doom/ status 2>&1 | grep "r0" | awk '{print $NF}')
  echo "Halt at PC=${PC}, r0=${R0}"
  ```

- [ ] **Step 218** [D]: If halted at exit(), it's likely an unimplemented function
  ```bash
  # Common Doom functions that need stubs:
  # - fopen/fclose/fread (WAD file access)
  # - mmap (memory mapping)
  # - stat/fstat (file info)
  # - getenv (environment variables)
  # - signal (signal handlers)
  # Each needs a stub in libc_monad.c
  ```

### Iterative Stub Addition Loop

- [ ] **Step 219** [B]: Pattern for each unimplemented function:
  ```
  1. Doom halts → read PC → disassemble → identify call
  2. Add stub to libc_monad.c
  3. Rebuild Doom ROM
  4. Reload ROM_MAP
  5. Reset CPU
  6. Resume packet injection
  7. Monitor for next halt or progress
  ```

- [ ] **Step 220** [B]: Run this loop up to 10 times
  ```bash
  for attempt in $(seq 1 10); do
    echo "=== ATTEMPT ${attempt} ==="
    go run ./cmd/doom/ reset 2>&1
    sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 100000 --burst &
    PID=$!
    sleep 30
    STATUS=$(go run ./cmd/doom/ status 2>&1)
    echo "$STATUS"
    kill $PID 2>/dev/null || true

    # Check if halted
    echo "$STATUS" | grep -q "halted.*true" || { echo "Still running — good!"; break; }

    # Check if screen has data
    SCREEN=$(sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP 2>&1 | grep -v "00 00 00 00" | head -1)
    [ -n "$SCREEN" ] && echo "SCREEN DATA DETECTED!" && break

    echo "Halted again. Investigating..."
    sleep 1
  done
  ```

### First Frame Detection

- [ ] **Step 221** [S][B]: Monitor for first SCREEN_WRITE event
  ```bash
  # Read COMPUTE_EVENTS ring for EVENT_SCREEN_WRITE (0x14)
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/COMPUTE_EVENTS 2>&1 | \
    grep "14" | head -5
  ```

- [ ] **Step 222** [S][B]: Read SCREEN_MAP for first frame
  ```bash
  # Check if any non-zero pixel data exists
  sudo python3 -c "
  import subprocess, struct
  result = subprocess.run(['bpftool', 'map', 'dump', 'pinned',
    '/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP'],
    capture_output=True, text=True)
  nonzero = 0
  for line in result.stdout.split('\n'):
    if 'value' in line and any(c not in '0 \t\nvalue:hex' for c in line.split('value:')[-1]):
      nonzero += 1
  print(f'Non-zero SCREEN_MAP entries: {nonzero}')
  if nonzero > 0:
    print('FIRST FRAME DETECTED!')
  "
  ```

- [ ] **Step 223** [V]: **FIRST FRAME GATE** — SCREEN_MAP contains non-zero pixel data

### Save First Frame as Image

- [ ] **Step 224** [S][B]: Extract screen buffer and save as PNG
  ```bash
  sudo python3 -c "
  import subprocess, struct
  from PIL import Image  # May need: pip install Pillow

  # Read all 64000 bytes from SCREEN_MAP
  screen = bytearray(64000)
  for i in range(64000):
    result = subprocess.run(['bpftool', 'map', 'lookup', 'pinned',
      '/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP',
      'key', str(i), '0', '0', '0'],
      capture_output=True, text=True)
    # Parse value
    if 'value' in result.stdout:
      val = int(result.stdout.split()[-1])
      screen[i] = val

  # Save as 320x200 grayscale PNG
  img = Image.frombytes('L', (320, 200), bytes(screen))
  img.save('/tmp/doom-first-frame.png')
  print('Saved: /tmp/doom-first-frame.png')
  " 2>&1 || echo "PIL not available — try raw dump instead"
  ```

- [ ] **Step 225** [D]: Alternative: dump screen as raw hex
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP 2>&1 > \
    /tmp/doom-screen-raw.txt
  echo "Raw screen dump saved to /tmp/doom-screen-raw.txt"
  wc -l /tmp/doom-screen-raw.txt
  ```

### Milestone Celebration

- [ ] **Step 226** [V]: If we reach here with a valid first frame:
  ```
  ╔══════════════════════════════════════════════════════════════╗
  ║     DOOM RENDERS ITS FIRST FRAME OVER IPv6 PACKETS!        ║
  ║     The wire IS the processor. Wotan IS the RAM.            ║
  ║     The Protocol Awakening Phase 1 is REAL.                 ║
  ╚══════════════════════════════════════════════════════════════╝
  ```

### Continuous Execution

- [ ] **Step 227** [S][B]: Run Doom for 60 seconds with continuous packet injection
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &
  INJECT_PID=$!
  echo "Continuous injection started (PID: $INJECT_PID)"
  ```

- [ ] **Step 228** [S][B]: Sample CPU state every 5 seconds for 60 seconds
  ```bash
  for i in $(seq 1 12); do
    sleep 5
    INSN=$(go run ./cmd/doom/ status 2>&1 | grep insn_count | awk '{print $NF}')
    echo "T+$((i*5))s: insn_count=${INSN}"
  done
  kill $INJECT_PID 2>/dev/null
  ```

- [ ] **Step 229** [V]: insn_count should be growing steadily (millions per minute)

### Frame Rate Estimation

- [ ] **Step 230** [S][B]: Count SCREEN_WRITE events over 30 seconds
  ```bash
  # Record initial event count, wait 30s, compare
  echo "Measuring frame rate..."
  EVENTS_START=$(sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1 | \
    grep "key.*7" | head -1)  # STAT_SYSCALLS
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &
  PID=$!
  sleep 30
  EVENTS_END=$(sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1 | \
    grep "key.*7" | head -1)
  kill $PID 2>/dev/null
  echo "Syscalls (start): $EVENTS_START"
  echo "Syscalls (end):   $EVENTS_END"
  echo "Estimated: ~X frames in 30 seconds = ~X FPS"
  ```

### Commit Milestone

- [ ] **Step 231** [B]: Commit all progress
  ```bash
  git add -A
  git commit -m "feat(doom): first frame rendered over IPv6 packets

  Doom survives BSS init, enters main loop, and writes first frame
  to SCREEN_MAP via SYS_DRAW_FRAME syscall. Screen data verified
  in BPF array map (320x200 indexed color).

  The wire is the processor. Wotan is the RAM.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 232** [V]: **PHASE 8 EXIT GATE** — Doom past BSS, in main loop, first frame in SCREEN_MAP

---

## PHASE 9: DASHBOARD HTTP WIRING (Steps 233-260)

**Goal**: Connect doom.html dashboard to real BPF maps via Go HTTP handlers.
**Time**: 1-2 hours
**Agent**: Agent (can parallel after Phase 8, independent of ring)

### Wire Real Map Accessors

- [ ] **Step 233** [R]: Review current doom handlers
  ```bash
  cat internal/doom/handlers.go
  ```

- [ ] **Step 234** [B]: Check if MapAccessor has a real BPF implementation
  ```bash
  grep -rn 'MapAccessor' internal/doom/ pkg/ cmd/
  ```

- [ ] **Step 235** [W]: Create BPF map accessor implementation
  ```go
  // internal/doom/bpf_accessor.go
  // Implements MapAccessor using pinned BPF maps via bpf syscall
  package doom

  import (
      "fmt"
      "os"
      "unsafe"
      "golang.org/x/sys/unix"
  )

  type BPFMapAccessor struct {
      fd int
  }

  func NewBPFMapAccessor(pinnedPath string) (*BPFMapAccessor, error) {
      fd, err := unix.BpfObjGet(&unix.BpfObjGetAttr{
          Pathname: unsafe.Pointer(unix.StringBytePtr(pinnedPath)),
      })
      if err != nil {
          return nil, fmt.Errorf("bpf obj get %s: %w", pinnedPath, err)
      }
      return &BPFMapAccessor{fd: fd}, nil
  }

  func (b *BPFMapAccessor) LookupElem(key []byte, valueSize int) ([]byte, error) {
      value := make([]byte, valueSize)
      err := unix.BpfMapLookupElem(b.fd, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]))
      if err != nil {
          return nil, err
      }
      return value, nil
  }

  func (b *BPFMapAccessor) UpdateElem(key, value []byte) error {
      return unix.BpfMapUpdateElem(b.fd, unsafe.Pointer(&key[0]),
          unsafe.Pointer(&value[0]), unix.BPF_ANY)
  }

  func (b *BPFMapAccessor) Close() error {
      return os.NewFile(uintptr(b.fd), "bpf-map").Close()
  }
  ```

- [ ] **Step 236** [W]: Create HTTP server that serves doom.html + API
  ```go
  // cmd/doom-dashboard/main.go
  // Serves doom.html and provides /doom/screen, /doom/status, /doom/input endpoints
  ```

- [ ] **Step 237** [B]: Wire handlers to pinned maps
  ```go
  cpuMap, _ := doom.NewBPFMapAccessor("/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP")
  screenMap, _ := doom.NewBPFMapAccessor("/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP")
  kbdMap, _ := doom.NewBPFMapAccessor("/sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP")
  handler := doom.NewDoomHandler(cpuMap, screenMap, kbdMap)
  ```

### Build & Test Dashboard Server

- [ ] **Step 238** [B]: Build dashboard server
  ```bash
  go build -o bin/doom-dashboard ./cmd/doom-dashboard/ 2>&1
  ```

- [ ] **Step 239** [S][B]: Start dashboard server
  ```bash
  sudo ./bin/doom-dashboard --port 8666 --maps-dir /sys/fs/bpf/unheaded/doom-ring/maps &
  DASH_PID=$!
  echo "Dashboard PID: $DASH_PID"
  ```

- [ ] **Step 240** [B]: Test screen endpoint
  ```bash
  curl -s http://localhost:8666/doom/screen | wc -c
  # Should return 64000 bytes (raw) or base64 JSON
  ```

- [ ] **Step 241** [B]: Test status endpoint
  ```bash
  curl -s http://localhost:8666/doom/status | python3 -m json.tool
  ```

- [ ] **Step 242** [B]: Test input endpoint
  ```bash
  curl -s -X POST http://localhost:8666/doom/input \
    -H 'Content-Type: application/json' \
    -d '{"key": 0x41, "pressed": true}'
  ```

- [ ] **Step 243** [V]: All three endpoints return valid data

### Optimize Screen Reads

- [ ] **Step 244** [B]: Benchmark screen read latency
  ```bash
  time for i in $(seq 1 10); do
    curl -s http://localhost:8666/doom/screen > /dev/null
  done
  echo "10 screen reads completed"
  ```

- [ ] **Step 245** [V]: Screen read should be < 50ms per frame

### doom.html Integration

- [ ] **Step 246** [R]: Review doom.html JavaScript fetch code
  ```bash
  grep -n 'fetch\|XMLHttpRequest\|WebSocket' dashboard/doom.html | head -20
  ```

- [ ] **Step 247** [B]: Verify doom.html loads in browser
  ```bash
  curl -s http://localhost:8666/ | head -5
  # OR: serve the static file
  ```

- [ ] **Step 248** [W]: Update doom.html API endpoints if needed
  ```bash
  # Ensure fetch URLs match: /doom/screen, /doom/status, /doom/input
  ```

### WebSocket for Real-Time Updates (Optional Enhancement)

- [ ] **Step 249** [W]: Add WebSocket endpoint for frame push
  ```go
  // /doom/ws — push screen updates when EVENT_SCREEN_WRITE fires
  // This avoids polling and gives real-time frame delivery
  ```

- [ ] **Step 250** [B]: Test WebSocket connection
  ```bash
  # websocat ws://localhost:8666/doom/ws 2>&1 | head -5
  ```

### End-to-End Browser Test

- [ ] **Step 251** [S][B]: Start full pipeline
  ```bash
  # 1. Ring is up (Phase 2)
  # 2. BPF loaded (Phase 3)
  # 3. ROM loaded (Phase 7)
  # 4. Dashboard serving (Phase 9)
  # 5. Inject packets
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &
  ```

- [ ] **Step 252** [B]: Open browser to http://localhost:8666
  ```bash
  echo "Open browser to: http://localhost:8666"
  echo "You should see the Doom screen updating!"
  ```

- [ ] **Step 253** [V]: **DASHBOARD GATE** — Browser displays Doom frames, canvas updates

### Commit Dashboard

- [ ] **Step 254** [B]: Commit dashboard wiring
  ```bash
  git add internal/doom/bpf_accessor.go cmd/doom-dashboard/
  git commit -m "feat(doom): wire dashboard to real BPF maps — live Doom rendering

  Dashboard reads SCREEN_MAP via pinned BPF maps, serves doom.html
  with /doom/screen, /doom/status, /doom/input endpoints.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 255** [V]: **PHASE 9 EXIT GATE** — Dashboard serves Doom frames from real BPF maps

---

## PHASE 10: KEYBOARD INPUT PIPELINE (Steps 256-280)

**Goal**: Browser keyboard → HTTP POST → KBD_MAP → Doom reads key → game responds.
**Time**: 1-2 hours
**Agent**: Agent [P] (parallelizable with Phase 11)

### Keyboard Event Flow

- [ ] **Step 256** [R]: Review doom.html keyboard handling
  ```bash
  grep -n -A5 'keydown\|keyup\|keyboard\|addEventListener' dashboard/doom.html
  ```

- [ ] **Step 257** [W]: Ensure doom.html sends key events to /doom/input
  ```javascript
  // Expected: keydown → POST /doom/input {key: scancode, pressed: true}
  // Expected: keyup → POST /doom/input {key: scancode, pressed: false}
  ```

### Doom Scancode Mapping

- [ ] **Step 258** [W]: Create scancode mapping (browser keyCode → Doom scancode)
  ```javascript
  // Doom uses its own scancodes (not standard HID)
  // Key mappings for doomgeneric:
  const DOOM_KEYS = {
    'ArrowUp':    0xAD,  // KEY_UPARROW
    'ArrowDown':  0xAF,  // KEY_DOWNARROW
    'ArrowLeft':  0xAC,  // KEY_LEFTARROW
    'ArrowRight': 0xAE,  // KEY_RIGHTARROW
    'Control':    0x80 + 0x1D,  // KEY_FIRE
    ' ':          0x80 + 0x39,  // KEY_USE
    'Shift':      0x80 + 0x36,  // KEY_RSHIFT (run)
    'Enter':      0x0D,  // KEY_ENTER
    'Escape':     0x1B,  // KEY_ESCAPE
    'Tab':        0x09,  // KEY_TAB
  };
  ```

- [ ] **Step 259** [B]: Test keyboard injection manually
  ```bash
  # Inject "UP arrow pressed"
  curl -s -X POST http://localhost:8666/doom/input \
    -H 'Content-Type: application/json' \
    -d '{"key": 173, "pressed": true}'
  ```

- [ ] **Step 260** [S][B]: Verify KBD_MAP updated
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP 2>&1
  ```

- [ ] **Step 261** [V]: KBD_MAP shows injected key value

### Input Latency Test

- [ ] **Step 262** [B]: Measure key injection → Doom read latency
  ```bash
  # Inject key, immediately check if SYS_GET_KEY reads it
  time (
    curl -s -X POST http://localhost:8666/doom/input \
      -d '{"key": 173, "pressed": true}' > /dev/null
    # Wait for next packet execution to read it
    sleep 0.01
    sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP \
      key 0 0 0 0
  )
  ```

### Debouncing & Rate Limiting

- [ ] **Step 263** [W]: Add key event debouncing in doom.html
  ```javascript
  // Prevent key repeat flooding
  let lastKeyTime = {};
  const KEY_DEBOUNCE_MS = 16; // ~60Hz
  ```

### Full Input Test

- [ ] **Step 264** [S][B]: With Doom running, inject arrow keys and observe
  ```bash
  # Run Doom with packet injection
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &

  # Inject movement keys
  for key in 173 175 172 174; do  # UP DOWN LEFT RIGHT
    curl -s -X POST http://localhost:8666/doom/input \
      -d "{\"key\": $key, \"pressed\": true}" > /dev/null
    sleep 0.5
    curl -s -X POST http://localhost:8666/doom/input \
      -d "{\"key\": $key, \"pressed\": false}" > /dev/null
    sleep 0.2
  done
  ```

- [ ] **Step 265** [V]: Doom should respond to input (screen changes with key presses)

### Multi-Key Support

- [ ] **Step 266** [W]: Verify KBD_MAP supports multiple simultaneous keys
  ```bash
  # KBD_MAP has 8 entries — check if Doom reads multiple keys
  # Entry 0: last scancode, Entry 1: pressed flag
  # May need bitmap approach for simultaneous keys
  ```

### Commit Input Pipeline

- [ ] **Step 267** [B]: Commit
  ```bash
  git add -A
  git commit -m "feat(doom): keyboard input pipeline — browser keys reach Doom via BPF

  Full pipeline: keydown → POST /doom/input → KBD_MAP → SYS_GET_KEY → game.
  Doom scancode mapping for arrow keys, fire, use, run, enter, escape.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 268** [V]: **PHASE 10 EXIT GATE** — Keyboard input reaches Doom, game responds to key events

---

## PHASE 11: SCREEN RENDERING OPTIMIZATION (Steps 269-295)

**Goal**: Optimize screen buffer reads for smooth visual output. Target: 10+ FPS.
**Time**: 1-2 hours
**Agent**: Agent [P] (parallelizable with Phase 10)

### Current Performance Baseline

- [ ] **Step 269** [B]: Benchmark raw screen buffer read time
  ```bash
  time for i in $(seq 1 30); do
    curl -s http://localhost:8666/doom/screen > /dev/null
  done
  echo "30 frames read"
  ```

- [ ] **Step 270** [B]: Calculate current FPS capacity
  ```bash
  # elapsed_ms / 30 = ms per frame
  # 1000 / ms_per_frame = theoretical max FPS
  ```

### Batch Map Reads

- [ ] **Step 271** [W]: Optimize screen read with batch BPF map dump
  ```go
  // Instead of 64000 individual lookups, use bpf_map_get_next_key
  // in a tight loop or read the entire array in one batch
  ```

- [ ] **Step 272** [B]: Benchmark batch vs individual reads
  ```bash
  # Compare: 64000 lookups vs batch dump
  time sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP > /dev/null
  ```

### Dirty Region Tracking

- [ ] **Step 273** [W]: Only send changed pixels to browser
  ```go
  // Track previous frame hash
  // Only send delta if changed
  // Use ETag/If-None-Match HTTP headers
  ```

- [ ] **Step 274** [W]: Add SSE (Server-Sent Events) for push-based frame delivery
  ```go
  // /doom/frames — SSE endpoint
  // Polls COMPUTE_EVENTS for EVENT_SCREEN_WRITE
  // On event: read SCREEN_MAP, send as SSE data
  ```

### Canvas Optimization

- [ ] **Step 275** [W]: Optimize doom.html canvas rendering
  ```javascript
  // Use ImageData and putImageData for fastest rendering
  // Pre-allocate ImageData buffer
  // Use requestAnimationFrame for smooth updates
  const imageData = ctx.createImageData(320, 200);
  ```

### Color Palette

- [ ] **Step 276** [W]: Implement Doom color palette (8-bit indexed → RGBA)
  ```javascript
  // Doom uses 256-color palette (VGA)
  // Load palette from doom.mbc or hardcode standard Doom palette
  const DOOM_PALETTE = new Uint32Array(256);
  // ... populate with Doom's default palette
  ```

- [ ] **Step 277** [W]: Apply palette in screen decode
  ```javascript
  function decodeScreen(rawBytes) {
    for (let i = 0; i < 64000; i++) {
      const colorIndex = rawBytes[i];
      const rgba = DOOM_PALETTE[colorIndex];
      imageData.data[i*4+0] = (rgba >> 16) & 0xFF; // R
      imageData.data[i*4+1] = (rgba >> 8) & 0xFF;  // G
      imageData.data[i*4+2] = rgba & 0xFF;          // B
      imageData.data[i*4+3] = 255;                   // A
    }
    ctx.putImageData(imageData, 0, 0);
  }
  ```

### Frame Rate Control

- [ ] **Step 278** [W]: Add configurable target FPS in doom.html
  ```javascript
  let targetFPS = 15; // Conservative default
  let frameInterval = 1000 / targetFPS;
  ```

### Compression (Optional)

- [ ] **Step 279** [W]: Add optional gzip compression for screen endpoint
  ```go
  // Compress 64000 bytes before sending
  // Doom screens compress well (large flat areas)
  ```

### Benchmarking

- [ ] **Step 280** [B]: Final FPS measurement
  ```bash
  time for i in $(seq 1 100); do
    curl -s http://localhost:8666/doom/screen > /dev/null
  done
  echo "100 frames: $SECONDS seconds"
  echo "FPS = $(( 100 / SECONDS ))"
  ```

- [ ] **Step 281** [V]: Target: > 10 FPS for screen reads

### Commit Optimizations

- [ ] **Step 282** [B]: Commit
  ```bash
  git add -A
  git commit -m "perf(doom): optimize screen rendering — target 10+ FPS

  Batch map reads, dirty region tracking, palette conversion,
  requestAnimationFrame for smooth canvas updates.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 283** [V]: **PHASE 11 EXIT GATE** — Screen renders at 10+ FPS, palette colors correct

---

## PHASE 12: PACKET INJECTION OPTIMIZATION (Steps 284-310)

**Goal**: Maximize packet throughput for higher instruction rate → better frame rate.
**Time**: 1-2 hours
**Agent**: Agent [P] (parallelizable with Phases 10-11)

### Current Throughput Measurement

- [ ] **Step 284** [S][B]: Measure current injection rate
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 10000 --burst 2>&1 | tail -5
  ```

- [ ] **Step 285** [B]: Record baseline
  ```bash
  echo "Baseline injection rate: [FILL IN] pkt/s" >> /tmp/doom-metrics.txt
  ```

### C-based Packet Injector (Higher Performance)

- [ ] **Step 286** [W]: Write a C packet injector for maximum throughput
  ```c
  // scripts/doom-inject.c
  // Replaces Python injector for production use
  // Uses raw AF_PACKET socket with pre-built packet buffer
  // Expected: 100K+ pkt/s (vs Python ~10K pkt/s)
  ```

- [ ] **Step 287** [B]: Build C injector
  ```bash
  gcc -O2 -o bin/doom-inject scripts/doom-inject.c
  ```

- [ ] **Step 288** [S][B]: Benchmark C injector
  ```bash
  sudo nsenter --net=/var/run/netns/monad0 \
    ./bin/doom-inject --flow-label 0xDE --count 10000 --interface veth01 2>&1 | tail -5
  ```

### Batch Packet Injection

- [ ] **Step 289** [W]: Use sendmmsg() for batch packet sending
  ```c
  // Send multiple packets in a single syscall
  // Linux sendmmsg() can batch up to 1024 packets
  ```

### XDP_TX Fast Path (Ring Circulation)

- [ ] **Step 290** [W]: Modify monad-cpu-ebpf to XDP_TX instead of XDP_PASS
  ```rust
  // Currently: XDP_PASS (packet goes through normal stack)
  // Optimization: XDP_TX (bounce back immediately, faster circulation)
  // WARNING: Changes routing — needs careful veth direction setup
  ```

- [ ] **Step 291** [D]: If XDP_TX breaks circulation, revert to XDP_PASS
  ```bash
  # XDP_TX on veth sends back to the SAME namespace
  # This may not work for inter-namespace forwarding
  # Alternative: XDP_REDIRECT to redirect to next veth
  ```

### Instructions Per Tick Tuning

- [ ] **Step 292** [W]: Adjust MAX_INSN_PER_TICK in monad-cpu-ebpf
  ```rust
  // Current: 16 instructions per packet per hop
  // Options:
  //   32: doubles throughput, may hit verifier complexity limit
  //   64: 4x throughput, likely too complex for verifier
  //   16: safe default
  // Try: increase to 32, rebuild, test
  ```

- [ ] **Step 293** [B]: Rebuild with higher tick count
  ```bash
  # Edit ebpf/monad-cpu-ebpf/src/main.rs: MAX_INSN_PER_TICK = 32
  cd ebpf && cargo +nightly build -p monad-cpu-ebpf \
    --target bpfel-unknown-none -Z build-std=core --release
  ```

- [ ] **Step 294** [V]: BPF verifier still accepts (may reject if too complex)

### Throughput Summary

- [ ] **Step 295** [S][B]: Final throughput measurement
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 100000 --burst 2>&1 | tail -5
  go run ./cmd/doom/ status 2>&1 | grep insn_count
  ```

- [ ] **Step 296** [B]: Calculate effective instruction rate
  ```bash
  echo "Insns/sec = insn_count / elapsed_time"
  echo "Target: > 1M insns/sec for playable Doom"
  ```

### Commit Optimizations

- [ ] **Step 297** [B]: Commit
  ```bash
  git add -A
  git commit -m "perf(doom): optimize packet injection throughput

  C-based injector, batch sendmmsg, MAX_INSN_PER_TICK tuning.
  Throughput: [FILL IN] pkt/s, [FILL IN] insns/sec.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 298** [V]: **PHASE 12 EXIT GATE** — Packet injection optimized, throughput documented

---

## PHASE 13: END-TO-END INTEGRATION TEST (Steps 299-325)

**Goal**: Full pipeline test — packets → CPU → screen → browser → keyboard → CPU. The loop closes.
**Time**: 1-2 hours
**Agent**: Coordinator (full-stack validation)

### Full Pipeline Smoke Test

- [ ] **Step 299** [S][B]: Ensure full stack is running
  ```bash
  echo "=== DOOM PIPELINE CHECK ==="
  echo "Namespaces:"
  ip netns list | grep monad | wc -l  # Should be 6
  echo "BPF programs:"
  sudo bpftool prog list | grep monad_cpu | wc -l  # Should be 1
  echo "Pinned maps:"
  ls /sys/fs/bpf/unheaded/doom-ring/maps/ | wc -l  # Should be 9
  echo "Dashboard:"
  curl -s -o /dev/null -w "%{http_code}" http://localhost:8666/doom/status  # Should be 200
  echo "ROM loaded:"
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP 2>&1 | wc -l
  ```

- [ ] **Step 300** [V]: All checks pass (6 ns, 1 prog, 9 maps, 200 status, ROM non-empty)

### Reset & Clean Start

- [ ] **Step 301** [S][B]: Reset everything for clean test
  ```bash
  go run ./cmd/doom/ reset 2>&1
  # Clear STATS
  for key in 0 1 2 3 4 5 6 7 8 9 10; do
    sudo bpftool map delete pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS \
      key $key 0 0 0 2>/dev/null || true
  done
  ```

### Timed Execution Run

- [ ] **Step 302** [S][B]: Run Doom for exactly 60 seconds with metrics collection
  ```bash
  echo "=== 60-SECOND DOOM RUN ==="
  START_TIME=$(date +%s%N)

  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &
  INJECT_PID=$!

  for i in $(seq 1 12); do
    sleep 5
    INSN=$(go run ./cmd/doom/ status 2>&1 | grep insn_count | awk -F: '{print $2}' | tr -d ' ')
    HALTED=$(go run ./cmd/doom/ status 2>&1 | grep halted | awk -F: '{print $2}' | tr -d ' ')
    SCREEN=$(curl -s http://localhost:8666/doom/screen | wc -c)
    echo "T+$((i*5))s: insns=${INSN} halted=${HALTED} screen_bytes=${SCREEN}"
  done

  kill $INJECT_PID 2>/dev/null
  END_TIME=$(date +%s%N)
  ELAPSED_MS=$(( (END_TIME - START_TIME) / 1000000 ))
  echo "Elapsed: ${ELAPSED_MS}ms"
  ```

### Verify All Data Paths

- [ ] **Step 303** [S][B]: Verify screen has data
  ```bash
  SCREEN_DATA=$(curl -s http://localhost:8666/doom/screen | od -A x -t x1 | head -5)
  echo "Screen (first bytes): $SCREEN_DATA"
  ```

- [ ] **Step 304** [S][B]: Verify CPU status API works
  ```bash
  curl -s http://localhost:8666/doom/status | python3 -m json.tool
  ```

- [ ] **Step 305** [S][B]: Verify keyboard input works
  ```bash
  curl -s -X POST http://localhost:8666/doom/input \
    -H 'Content-Type: application/json' \
    -d '{"key": 173, "pressed": true}'
  sleep 0.1
  curl -s -X POST http://localhost:8666/doom/input \
    -d '{"key": 173, "pressed": false}'
  ```

- [ ] **Step 306** [S][B]: Verify STATS map has data
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1
  ```

### Error Rate Check

- [ ] **Step 307** [S][B]: Check for errors in stats
  ```bash
  echo "Error counters:"
  echo -n "  MEM_FAULTS (key 6): "
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS key 6 0 0 0 2>&1
  echo -n "  ROM_FAULT (key 8):  "
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS key 8 0 0 0 2>&1
  ```

- [ ] **Step 308** [V]: Error rates should be 0 or negligible

### L1 Cache Performance

- [ ] **Step 309** [S][B]: Check cache hit rate
  ```bash
  echo "Cache performance:"
  echo -n "  CACHE_HITS (key 9):   "
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS key 9 0 0 0 2>&1
  echo -n "  CACHE_MISSES (key 10): "
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS key 10 0 0 0 2>&1
  ```

- [ ] **Step 310** [B]: Calculate hit rate
  ```bash
  echo "Hit rate = hits / (hits + misses) × 100%"
  ```

### Latency Measurement

- [ ] **Step 311** [B]: Measure end-to-end latency (packet → screen update → browser)
  ```bash
  # Inject single packet, measure time until screen endpoint returns new data
  START=$(date +%s%N)
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1
  curl -s http://localhost:8666/doom/screen > /dev/null
  END=$(date +%s%N)
  LATENCY_US=$(( (END - START) / 1000 ))
  echo "End-to-end latency: ${LATENCY_US} µs"
  ```

- [ ] **Step 312** [V]: Target: < 50ms (50,000 µs) end-to-end

### Stress Test

- [ ] **Step 313** [S][B]: High-rate injection for 30 seconds
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 300000 --burst 2>&1 | tail -5
  go run ./cmd/doom/ status 2>&1
  ```

- [ ] **Step 314** [V]: No crashes, no error floods, CPU state consistent

### Document Results

- [ ] **Step 315** [W]: Write integration test results
  ```bash
  cat > /tmp/doom-integration-results.txt << 'EOF'
  === DOOM-OVER-IPv6 INTEGRATION TEST RESULTS ===
  Date: $(date)
  Duration: 60 seconds
  Packets sent: [FILL]
  Instructions executed: [FILL]
  Frames rendered: [FILL]
  FPS: [FILL]
  Cache hit rate: [FILL]%
  Error rate: [FILL]
  End-to-end latency: [FILL] ms
  Status: [PASS/FAIL]
  EOF
  ```

### Commit Integration Test

- [ ] **Step 316** [B]: Commit results
  ```bash
  git add -A
  git commit -m "test(doom): end-to-end integration test — full pipeline verified

  60-second timed run: [FILL] insns, [FILL] FPS, [FILL] cache hit rate.
  All data paths verified: packets → CPU → screen → browser → keyboard.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 317** [V]: **PHASE 13 EXIT GATE** — Full pipeline validated, metrics documented

---

## PHASE 14: ANAMNESIS EVENT PIPELINE (Steps 318-340)

**Goal**: Connect COMPUTE_EVENTS ring buffer to userspace trace collector for observability.
**Time**: 1-2 hours
**Agent**: Agent [P]

### Ring Buffer Reader

- [ ] **Step 318** [W]: Create Anamnesis event reader in Go
  ```go
  // internal/doom/events.go
  // Reads COMPUTE_EVENTS ring buffer and forwards to Wotan/dashboard
  type EventReader struct {
      ringBuf *ebpf.Map
      handler EventHandler
  }
  ```

- [ ] **Step 319** [B]: Build event reader
  ```bash
  go build ./internal/doom/... 2>&1
  ```

- [ ] **Step 320** [W]: Add /doom/events SSE endpoint to dashboard
  ```go
  // Server-Sent Events stream of compute events
  // EVENT_SCREEN_WRITE, EVENT_COMPUTE_HALT, EVENT_CACHE_MISS
  ```

### Event Types

- [ ] **Step 321** [W]: Parse all event types
  ```go
  const (
      EventComputeHop   = 0x10
      EventCacheMiss    = 0x11
      EventMemWrite     = 0x12
      EventScreenWrite  = 0x14
      EventKeyRead      = 0x15
      EventComputeHalt  = 0x16
      EventComputeStall = 0x17
  )
  ```

### Dashboard Event Display

- [ ] **Step 322** [W]: Add event log panel to doom.html
  ```html
  <!-- Real-time event stream -->
  <div id="event-log">
    <!-- SCREEN_WRITE, HALT, CACHE_MISS events displayed here -->
  </div>
  ```

### Test Events

- [ ] **Step 323** [S][B]: Verify events flow
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 10 --burst
  curl -s http://localhost:8666/doom/events 2>&1 | head -20
  ```

- [ ] **Step 324** [V]: Events visible in dashboard

### Commit

- [ ] **Step 325** [B]: Commit
  ```bash
  git add -A
  git commit -m "feat(doom): Anamnesis event pipeline — ring buffer → dashboard

  COMPUTE_EVENTS ring buffer reader, SSE /doom/events endpoint,
  real-time event log in doom.html dashboard.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

---

## PHASE 15: LICH-008 CACHE RACE FUZZING (Steps 326-345)

**Goal**: Fuzz the L1 cache and Wotan memory under concurrent access patterns.
**Time**: 1 hour setup + 24-72 hour background campaign
**Agent**: Agent [P] (setup only, campaign runs in background)

### Setup LICH-008

- [ ] **Step 326** [R]: Review LICH-008 fuzz target
  ```bash
  cat ebpf/fuzz/lich_008_wotan_cache.rs
  ```

- [ ] **Step 327** [B]: Build fuzz targets
  ```bash
  cd crates/monad-mbc && cargo +nightly fuzz build 2>&1 | tail -10
  ```

- [ ] **Step 328** [B]: Run LICH-008 with ThreadSanitizer
  ```bash
  cargo +nightly fuzz run lich_008_wotan_cache -- \
    -max_total_time=259200 \
    -jobs=2 \
    -workers=2 \
    2>&1 | tee /tmp/lich008-cache.log &
  LICH008_PID=$!
  echo "LICH-008 PID: $LICH008_PID"
  ```

### Setup LICH-009

- [ ] **Step 329** [R]: Review LICH-009 (flow collision)
  ```bash
  cat ebpf/fuzz/lich_009_flow_collision.rs
  ```

- [ ] **Step 330** [B]: Run LICH-009
  ```bash
  cargo +nightly fuzz run lich_009_flow_collision -- \
    -max_total_time=259200 \
    -jobs=1 \
    2>&1 | tee /tmp/lich009-flow.log &
  ```

### Commit Fuzz Setup

- [ ] **Step 331** [B]: Commit any fuzz target improvements
  ```bash
  git add -A
  git commit -m "test(lich): launch LICH-008 (cache race) and LICH-009 (flow collision)

  72-hour campaigns targeting L1 cache concurrent access and
  flow label collision patterns. Background execution.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 332** [V]: **PHASE 15 EXIT GATE** — Fuzz campaigns running in background

---

## PHASE 16: DOOM GAMEPLAY TUNING (Steps 333-360)

**Goal**: Make Doom actually playable — adjust timing, frame rate, input responsiveness.
**Time**: 2-3 hours
**Agent**: Coordinator (interactive tuning)

### Frame Rate Analysis

- [ ] **Step 333** [S][B]: Measure actual frame rate
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &
  PID=$!

  # Count SYS_DRAW_FRAME syscalls over 30 seconds
  SYSCALLS_START=$(sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS key 7 0 0 0 2>&1)
  sleep 30
  SYSCALLS_END=$(sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS key 7 0 0 0 2>&1)
  kill $PID 2>/dev/null

  echo "Syscalls start: $SYSCALLS_START"
  echo "Syscalls end:   $SYSCALLS_END"
  echo "FPS = (end - start) / 30"
  ```

### Timing Calibration

- [ ] **Step 334** [W]: Adjust SYS_GET_TICKS to return appropriate game time
  ```rust
  // Current: bpf_ktime_get_ns() / 1_000_000
  // Doom expects ~35 ticks/sec for normal speed
  // May need to scale time to match packet injection rate
  ```

- [ ] **Step 335** [W]: Adjust SYS_SLEEP behavior
  ```rust
  // Current: sets sleep_until_ns, BPF skips CPU until then
  // Problem: if injection rate << real time, sleep is too long
  // Fix: scale sleep duration by injection_rate / target_rate
  ```

### Input Responsiveness

- [ ] **Step 336** [B]: Measure input latency: key press → screen change
  ```bash
  # Automated: inject key, poll screen for change
  ```

- [ ] **Step 337** [W]: Optimize input polling frequency
  ```bash
  # Doom checks keyboard every game tick (~35Hz)
  # If injection rate is lower, input feels laggy
  # Solution: ensure SYS_GET_KEY is called frequently enough
  ```

### Difficulty Adjustment

- [ ] **Step 338** [W]: If frame rate < 10 FPS, reduce game resolution
  ```bash
  # Option 1: Reduce to 160x100 (quarter resolution)
  # Option 2: Skip frames (render every Nth frame)
  # Option 3: Increase MAX_INSN_PER_TICK
  ```

### Menu Navigation Test

- [ ] **Step 339** [S][B]: Navigate Doom main menu with keyboard
  ```bash
  # Enter → start game
  # Up/Down → menu navigation
  # Escape → back
  echo "Manual test: open browser, navigate Doom menu with keyboard"
  ```

### Gameplay Test

- [ ] **Step 340** [S][B]: Play 30 seconds of E1M1
  ```bash
  echo "Manual gameplay test:"
  echo "1. Open browser to http://localhost:8666"
  echo "2. Press Enter to start"
  echo "3. Use arrow keys to move"
  echo "4. Ctrl to fire"
  echo "5. Space to use/open doors"
  echo "Record: can you move? can you shoot? can you open doors?"
  ```

- [ ] **Step 341** [V]: **GAMEPLAY GATE** — Doom is playable (movement, shooting, door opening work)

### Commit Tuning

- [ ] **Step 342** [B]: Commit
  ```bash
  git add -A
  git commit -m "feat(doom): gameplay tuning — timing, input, frame rate optimization

  Calibrated SYS_GET_TICKS and SYS_SLEEP for packet injection rate.
  Input latency optimized. Frame rate: [FILL] FPS.
  Doom is PLAYABLE over IPv6 packets.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

---

## PHASE 17: PERFORMANCE PROFILING & DOCUMENTATION (Steps 343-370)

**Goal**: Comprehensive performance baseline documented for the Doom-over-IPv6 PoC.
**Time**: 1-2 hours
**Agent**: Agent [P]

### Metrics Collection

- [ ] **Step 343** [S][B]: Full metrics sweep
  ```bash
  echo "=== DOOM-OVER-IPv6 PERFORMANCE BASELINE ==="
  echo ""
  echo "Ring topology: 6 namespaces, 6 veth pairs"
  echo "BPF program: monad-cpu-ebpf (XDP)"
  echo "MAX_INSN_PER_TICK: [FILL]"
  echo ""

  # Packet throughput
  echo "--- Packet Throughput ---"
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 50000 --burst 2>&1 | tail -3

  # Instruction rate
  echo "--- Instruction Rate ---"
  go run ./cmd/doom/ reset 2>&1
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 50000 --burst
  go run ./cmd/doom/ status 2>&1 | grep insn_count

  # Cache performance
  echo "--- Cache Performance ---"
  go run ./cmd/doom/ status 2>&1 | grep -E 'cache_hits|cache_misses'

  # Screen rendering
  echo "--- Screen Rendering ---"
  time for i in $(seq 1 100); do curl -s http://localhost:8666/doom/screen > /dev/null; done

  # STATS dump
  echo "--- BPF Statistics ---"
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1
  ```

### Write Performance Document

- [ ] **Step 344** [W]: Create docs/protocol/doom-performance-baseline.md
  ```markdown
  # Doom-over-IPv6 Performance Baseline

  ## Configuration
  - Ring: 6 namespaces, veth pairs
  - BPF: monad-cpu-ebpf (XDP generic)
  - MAX_INSN_PER_TICK: [FILL]
  - Injection: doom-tick.py (Python AF_PACKET)

  ## Results
  | Metric | Value | Target |
  |--------|-------|--------|
  | Packet throughput | [FILL] pkt/s | >8,000 |
  | Instruction rate | [FILL] insns/s | >30M |
  | Frame rate | [FILL] FPS | >10 |
  | Cache hit rate | [FILL]% | >90% |
  | Input latency | [FILL] ms | <50 |
  | Screen read time | [FILL] ms | <50 |

  ## Bottleneck Analysis
  [FILL: packet injection? BPF execution? map reads?]
  ```

### Commit

- [ ] **Step 345** [B]: Commit performance docs
  ```bash
  git add docs/protocol/doom-performance-baseline.md
  git commit -m "docs(doom): performance baseline — [FILL] FPS, [FILL] insns/sec

  Comprehensive Doom-over-IPv6 performance measurements.
  Bottleneck identified: [FILL].

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

---

## PHASE 18: SECURITY AUDIT (Steps 346-375)

**Goal**: Black Mage review of the Doom pipeline for security issues.
**Time**: 1-2 hours
**Agent**: Agent [P]

### Attack Surface Review

- [ ] **Step 346** [R]: Review all HTTP endpoints for injection
  ```bash
  grep -rn 'http.HandleFunc\|mux.Handle' internal/doom/ cmd/doom-dashboard/
  ```

- [ ] **Step 347** [B]: Test /doom/input for injection
  ```bash
  # Test oversized key value
  curl -s -X POST http://localhost:8666/doom/input \
    -d '{"key": 999999999, "pressed": true}'
  # Test negative key
  curl -s -X POST http://localhost:8666/doom/input \
    -d '{"key": -1, "pressed": true}'
  # Test missing fields
  curl -s -X POST http://localhost:8666/doom/input \
    -d '{}'
  ```

- [ ] **Step 348** [V]: All malformed inputs return appropriate errors (400/422), no panics

### BPF Map Security

- [ ] **Step 349** [S][B]: Verify map permissions
  ```bash
  ls -la /sys/fs/bpf/unheaded/doom-ring/maps/
  # Maps should only be readable by root
  ```

- [ ] **Step 350** [B]: Test map boundary conditions
  ```bash
  # ROM_MAP: try index > 262144
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP \
    key 255 255 255 255 2>&1
  # Should return error, not crash
  ```

### Packet Injection Security

- [ ] **Step 351** [S][B]: Send malformed packets
  ```bash
  # Wrong ethertype
  # Wrong next_header
  # Truncated HBH
  # Monad with wrong version
  # Monad with bad CRC
  sudo python3 -c "
  import socket, struct
  sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
  sock.bind(('veth01', 0))
  # Send 10 bytes (way too short for valid packet)
  sock.send(b'\x00' * 10)
  sock.close()
  print('Sent malformed packet')
  " 2>&1 || echo "Expected failure"
  ```

- [ ] **Step 352** [V]: BPF program handles malformed packets gracefully (XDP_PASS, no crash)

### Flow Label Collision Test

- [ ] **Step 353** [S][B]: Test two different CPUs sharing flow label space
  ```bash
  # Instance 0xDE and 0xDF should be independent
  go run ./cmd/doom/ reset 2>&1
  sudo python3 scripts/doom-tick.py --flow-label 0xDF --count 5 --burst
  go run ./cmd/doom/ status 2>&1
  # Instance 0xDE should be unchanged
  ```

### Resource Exhaustion

- [ ] **Step 354** [S][B]: Flood test — 1M packets rapidly
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1000000 --burst 2>&1 | tail -5
  # Verify: no kernel warnings, no OOM, maps intact
  dmesg | tail -10
  ```

- [ ] **Step 355** [V]: System stable after flood

### Write Security Report

- [ ] **Step 356** [W]: Create docs/security/doom-security-audit.md
  ```markdown
  # Doom-over-IPv6 Security Audit
  ## Attack Surface
  - HTTP endpoints: /doom/screen, /doom/status, /doom/input
  - BPF maps: 9 pinned, root-only access
  - Packet injection: raw AF_PACKET, namespace-isolated
  ## Findings
  [FILL]
  ## Recommendations
  [FILL]
  ```

### Commit

- [ ] **Step 357** [B]: Commit security audit
  ```bash
  git add docs/security/doom-security-audit.md
  git commit -m "sec(doom): security audit — attack surface reviewed, findings documented

  HTTP input validation, BPF map permissions, packet injection safety,
  resource exhaustion testing. [FILL] findings.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

---

## PHASE 19: DOCUMENTATION & DEMO PREP (Steps 358-395)

**Goal**: Complete documentation for the Doom PoC. Conference/demo ready.
**Time**: 2-3 hours
**Agent**: Agent [P]

### Architecture Documentation

- [ ] **Step 358** [W]: Update docs/protocol/doom-over-ipv6-architecture.md
  ```markdown
  # Doom-over-IPv6 Architecture (Updated Post-Integration)

  ## System Diagram
  [Updated with actual metrics and configuration]

  ## Component List
  - doom-ring.sh: 6-namespace ring setup
  - monad-cpu-ebpf: XDP BPF program (43 opcodes)
  - doom-tick.py: Packet injection
  - doom-dashboard: HTTP server for browser rendering
  - doom.html: Browser-based Doom viewer
  - doom CLI: ROM loading, CPU control

  ## Performance
  [From Phase 17 baseline]
  ```

- [ ] **Step 359** [W]: Update docs/protocol/mbc-isa-reference.md if opcodes changed

- [ ] **Step 360** [W]: Write docs/protocol/doom-demo-guide.md
  ```markdown
  # Doom-over-IPv6 Demo Guide

  ## Quick Start
  1. sudo ./scripts/doom-ring.sh setup
  2. go run ./cmd/doom/ load doom/doomgeneric/doom.mbc
  3. go run ./cmd/doom/ reset
  4. sudo ./bin/doom-dashboard --port 8666 &
  5. sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &
  6. Open browser: http://localhost:8666
  7. Play Doom!

  ## Controls
  Arrow keys: Move
  Ctrl: Fire
  Space: Use/Open
  Enter: Menu select
  Escape: Menu/Quit
  ```

### Session Documentation

- [ ] **Step 361** [W]: Write S31 handoff document
  ```bash
  # docs/sessions/S31-handoff.md
  ```

- [ ] **Step 362** [W]: Write S31 session summary
  ```bash
  # docs/sessions/SESSION_2026-02-XX_S31_SUMMARY.md
  ```

### README Updates

- [ ] **Step 363** [W]: Update main README with Doom PoC section
  ```markdown
  ## Doom-over-IPv6 (Protocol Awakening PoC)
  The Unheaded Protocol's computational completeness is demonstrated
  by running Doom (1993) entirely over IPv6 packets. Each packet
  carries a Monad register through 6 network namespaces. At each
  hop, an XDP BPF program executes MBC (Monad Bytecode) instructions.
  The wire IS the processor. Wotan IS the RAM.
  ```

### Video/GIF Recording (If Possible)

- [ ] **Step 364** [B]: Record terminal session
  ```bash
  # asciinema rec /tmp/doom-demo.cast
  ```

- [ ] **Step 365** [B]: Take screenshots
  ```bash
  # Screenshot of doom.html showing Doom frame
  # Screenshot of terminal showing stats
  ```

### Timeline Update

- [ ] **Step 366** [W]: Update references/timeline.md
  ```markdown
  ## Protocol Awakening: Doom-over-IPv6
  - [x] MBC emulator (43 opcodes, 293 tests)
  - [x] BPF verifier accepts monad-cpu-ebpf
  - [x] 6-namespace packet ring
  - [x] First packet execution
  - [x] Doom BSS clearing
  - [x] First frame rendered
  - [x] Browser dashboard
  - [x] Keyboard input
  - [x] Performance profiled
  - [x] Security audited
  ```

### Kanban Board Update

- [ ] **Step 367** [B]: Update Kanban with Doom tasks
  ```bash
  # Mark completed tasks, add new cards for remaining work
  ```

### Conference Abstract

- [ ] **Step 368** [W]: Draft conference abstract
  ```markdown
  # Running Doom over IPv6: When the Wire IS the Processor

  We present a novel approach to computational infrastructure where
  IPv6 packets serve as CPU clock ticks. Using eBPF programs attached
  at XDP, each packet hop executes instructions from a custom ISA (MBC).
  We demonstrate by running id Software's Doom (1993) across 6 network
  namespaces — proving that the Unheaded Protocol's Monad register
  enables packet-level computation. Performance: [FILL] FPS at
  [FILL] packets/second.
  ```

### Commit Documentation

- [ ] **Step 369** [B]: Commit all docs
  ```bash
  git add -A
  git commit -m "docs(doom): comprehensive Doom-over-IPv6 documentation

  Architecture, demo guide, ISA reference, performance baseline,
  security audit, session handoff, conference abstract.
  The Protocol Awakening is documented.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 370** [V]: **PHASE 19 EXIT GATE** — All documentation complete, demo guide works

---

## PHASE 20: VICTORY LAP & FINAL VERIFICATION (Steps 371-420)

**Goal**: Final validation, cleanup, celebration. Ship it.
**Time**: 1-2 hours
**Agent**: Coordinator

### Full Test Suite

- [ ] **Step 371** [B]: Run all Rust tests
  ```bash
  cargo test --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | tail -20
  ```

- [ ] **Step 372** [B]: Run all Go tests
  ```bash
  go test ./... 2>&1 | tail -30
  ```

- [ ] **Step 373** [B]: Run Go tests with race detector
  ```bash
  go test -race ./internal/doom/... 2>&1 | tail -20
  ```

- [ ] **Step 374** [V]: All tests pass, zero race conditions

### Full Pipeline Replay

- [ ] **Step 375** [S][B]: Clean start
  ```bash
  sudo ./scripts/doom-ring.sh teardown 2>/dev/null || true
  sleep 1
  ```

- [ ] **Step 376** [S][B]: Fresh setup
  ```bash
  sudo ./scripts/doom-ring.sh setup
  ```

- [ ] **Step 377** [S][B]: Load ROM
  ```bash
  go run ./cmd/doom/ load doom/doomgeneric/doom.mbc
  go run ./cmd/doom/ reset
  ```

- [ ] **Step 378** [S][B]: Start dashboard
  ```bash
  sudo ./bin/doom-dashboard --port 8666 &
  ```

- [ ] **Step 379** [S][B]: Start packet injection
  ```bash
  sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600 &
  ```

- [ ] **Step 380** [B]: Wait for Doom to initialize (BSS clearing)
  ```bash
  echo "Waiting for Doom to initialize..."
  for i in $(seq 1 60); do
    sleep 2
    INSN=$(go run ./cmd/doom/ status 2>&1 | grep insn_count | awk -F: '{print $2}' | tr -d ' ')
    echo "T+$((i*2))s: insn_count=${INSN}"
    SCREEN=$(curl -s http://localhost:8666/doom/screen | od -A n -t x1 | head -1)
    echo "  screen[0..16]: $SCREEN"
    # Check for non-zero screen data
    echo "$SCREEN" | grep -qv "00 00 00 00 00 00 00 00" && echo "FRAME DETECTED!" && break
  done
  ```

- [ ] **Step 381** [V]: Doom initializes and renders

### Gameplay Verification

- [ ] **Step 382** [B]: Open browser to http://localhost:8666
- [ ] **Step 383** [B]: Navigate Doom menu with Enter
- [ ] **Step 384** [B]: Start E1M1
- [ ] **Step 385** [B]: Move forward with Up arrow
- [ ] **Step 386** [B]: Turn with Left/Right arrows
- [ ] **Step 387** [B]: Fire with Ctrl
- [ ] **Step 388** [B]: Open door with Space
- [ ] **Step 389** [V]: **GAMEPLAY VERIFICATION GATE** — All controls work

### Metrics Summary

- [ ] **Step 390** [S][B]: Final metrics collection
  ```bash
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║        DOOM-OVER-IPv6 — FINAL METRICS                      ║"
  echo "╠══════════════════════════════════════════════════════════════╣"
  go run ./cmd/doom/ status 2>&1
  echo ""
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS 2>&1
  echo "╚══════════════════════════════════════════════════════════════╝"
  ```

### Code Quality

- [ ] **Step 391** [B]: Run clippy on Rust code
  ```bash
  cd ebpf && cargo +nightly clippy -p monad-cpu-ebpf --target bpfel-unknown-none 2>&1 | tail -20
  cargo clippy --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | tail -20
  ```

- [ ] **Step 392** [B]: Run golangci-lint on Go code
  ```bash
  golangci-lint run ./internal/doom/... ./cmd/doom/... 2>&1 | tail -20
  ```

### Git Status

- [ ] **Step 393** [B]: Check for uncommitted changes
  ```bash
  git status
  git diff --stat
  ```

- [ ] **Step 394** [B]: Final commit
  ```bash
  git add -A
  git commit -m "feat(doom): Doom-over-IPv6 PoC complete — Protocol Awakening Phase 1

  The wire is the processor. Wotan is the RAM. Doom runs over IPv6 packets.

  Components:
  - monad-cpu-ebpf: 43-opcode MBC ISA executing at XDP
  - 6-namespace packet ring with shared BPF maps
  - doom-tick.py: IPv6 + HBH + Monad packet injection
  - doom-dashboard: browser-based rendering + keyboard input
  - LICH-007: 1B+ fuzz executions, zero crashes

  Performance:
  - [FILL] packets/sec
  - [FILL] instructions/sec
  - [FILL] FPS
  - [FILL]% cache hit rate

  293 Rust tests + 135 Go packages, all passing.

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

### LOC Count

- [ ] **Step 395** [B]: Count final LOC
  ```bash
  echo "=== FINAL LOC COUNT ==="
  echo "Rust (eBPF):"
  find ebpf/ -name '*.rs' | xargs wc -l | tail -1
  echo "Rust (monad-mbc):"
  find crates/monad-mbc/ -name '*.rs' | xargs wc -l | tail -1
  echo "Go:"
  find . -name '*.go' -not -path './vendor/*' | xargs wc -l | tail -1
  echo "HTML/JS:"
  find dashboard/ -name '*.html' -o -name '*.js' | xargs wc -l | tail -1
  echo "Shell:"
  find scripts/ -name '*.sh' -o -name '*.py' | xargs wc -l | tail -1
  echo "Docs:"
  find docs/ -name '*.md' | xargs wc -l | tail -1
  ```

### LICH Campaign Check

- [ ] **Step 396** [B]: Check LICH-008 and LICH-009 status
  ```bash
  for log in /tmp/lich008-*.log /tmp/lich009-*.log; do
    if [ -f "$log" ]; then
      echo "=== $(basename $log) ==="
      tail -3 "$log"
    fi
  done
  ```

- [ ] **Step 397** [V]: No crashes in ongoing campaigns

### Cleanup

- [ ] **Step 398** [B]: Clean temporary files
  ```bash
  rm -f /tmp/trivial.mbc /tmp/nop10.mbc /tmp/nop100.mbc /tmp/nop1000.mbc
  rm -f /tmp/test-*.asm /tmp/test-*.mbc
  ```

- [ ] **Step 399** [B]: Verify doom-ring teardown works
  ```bash
  sudo ./scripts/doom-ring.sh teardown
  sudo ./scripts/doom-ring.sh status
  ```

- [ ] **Step 400** [V]: Clean teardown, no orphaned namespaces

### Push to Remote

- [ ] **Step 401** [B]: Push all commits
  ```bash
  git push origin HEAD
  ```

- [ ] **Step 402** [V]: Push succeeds

### Victory Celebration

- [ ] **Step 403** [V]: **ALL 20 PHASES COMPLETE**

```
╔══════════════════════════════════════════════════════════════════════╗
║                                                                      ║
║   ██████╗  ██████╗  ██████╗ ███╗   ███╗                             ║
║   ██╔══██╗██╔═══██╗██╔═══██╗████╗ ████║                             ║
║   ██║  ██║██║   ██║██║   ██║██╔████╔██║                             ║
║   ██║  ██║██║   ██║██║   ██║██║╚██╔╝██║                             ║
║   ██████╔╝╚██████╔╝╚██████╔╝██║ ╚═╝ ██║                             ║
║   ╚═════╝  ╚═════╝  ╚═════╝ ╚═╝     ╚═╝                             ║
║                                                                      ║
║           OVER IPv6 PACKETS                                          ║
║                                                                      ║
║   The wire IS the processor.                                         ║
║   Wotan IS the RAM.                                                  ║
║   The Monad carries computation.                                     ║
║   The Protocol Awakening is REAL.                                    ║
║                                                                      ║
║   293 Rust tests. 135 Go packages. Zero crashes.                     ║
║   6 namespaces. 9 BPF maps. 43 opcodes.                             ║
║   1 dream. 1 protocol. 1 Kingdom.                                   ║
║                                                                      ║
╚══════════════════════════════════════════════════════════════════════╝
```

---

## APPENDIX A: EMERGENCY PROCEDURES

### Ring Won't Start
```bash
# Nuclear option: kill everything, clean, restart
sudo pkill -f doom-tick
sudo pkill -f doom-dashboard
sudo pkill -f ebpf-loader
sudo ./scripts/doom-ring.sh teardown 2>/dev/null || true
for i in 0 1 2 3 4 5; do sudo ip netns del "monad${i}" 2>/dev/null || true; done
sudo rm -rf /sys/fs/bpf/unheaded/doom-ring/
sudo rm -rf /run/doom-ring/
sleep 2
sudo ./scripts/doom-ring.sh setup
```

### BPF Verifier Rejects
```bash
# Check verifier log
sudo bpftool prog load ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf \
  /sys/fs/bpf/test_fail type xdp 2>&1 | head -50
# Common fix: reduce MAX_INSN_PER_TICK or simplify dispatch loop
```

### Doom Halts Immediately
```bash
# 1. Check PC value at halt
go run ./cmd/doom/ status
# 2. If PC=0, ROM_MAP may be empty
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP | head -5
# 3. If PC>0, check what instruction caused halt
# 4. Reload ROM: go run ./cmd/doom/ load <rom.mbc>
```

### Dashboard Returns Empty Screen
```bash
# 1. Check if SCREEN_MAP has data
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP | head -10
# 2. Check if BPF maps are accessible from Go
curl -v http://localhost:8666/doom/screen
# 3. Check if dashboard has correct map paths
```

### Packet Injection Fails
```bash
# 1. Check namespace exists
ip netns list | grep monad0
# 2. Check veth exists
sudo ip netns exec monad0 ip link show veth01
# 3. Check MAC addresses
sudo ip netns exec monad0 ip link show veth01 | grep ether
# 4. Try simplified injection
sudo ./scripts/doom-ring.sh inject 0xDE
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent Type | Parallelizable | Dependencies | Est. Time |
|-------|-----------|---------------|--------------|-----------|
| 0 | Solo | No | Campaign finish | 30 min |
| 1 | Coordinator | No | None | 20 min |
| 2 | Coordinator | No | Phase 1 | 45 min |
| 3 | Coordinator | No | Phase 2 | 60 min |
| 4 | Coordinator | No | Phase 3 | 120 min |
| 5 | Coordinator | No | Phase 4 | 45 min |
| 6 | Agent | Yes (with 7 prep) | Phase 5 | 120 min |
| 7 | Coordinator | No | Phase 6 | 120 min |
| 8 | Coordinator | No | Phase 7 | 240 min |
| 9 | Agent [P] | Yes | Phase 8 | 120 min |
| 10 | Agent [P] | Yes | Phase 9 | 120 min |
| 11 | Agent [P] | Yes | Phase 9 | 120 min |
| 12 | Agent [P] | Yes | Phase 4 | 120 min |
| 13 | Coordinator | No | Phases 9-12 | 120 min |
| 14 | Agent [P] | Yes | Phase 13 | 120 min |
| 15 | Agent [P] | Yes | Phase 5 | 60 min |
| 16 | Coordinator | No | Phase 13 | 180 min |
| 17 | Agent [P] | Yes | Phase 16 | 120 min |
| 18 | Agent [P] | Yes | Phase 16 | 120 min |
| 19 | Agent [P] | Yes | Phase 17 | 180 min |
| 20 | Coordinator | No | ALL | 120 min |

**Critical Path**: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 13 → 16 → 20
**Parallelizable after Phase 8**: 9, 10, 11, 12 (all independent)
**Parallelizable after Phase 13**: 14, 15, 17, 18, 19

---

## APPENDIX C: QUICK REFERENCE

### Monad CPU Tick Packet Structure (82 bytes)
```
[Ethernet 14B][IPv6 40B][HBH 4B][Monad 20B][padding 4B]
```

### BPF Map Paths
```
/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP
/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP
/sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP
/sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP
/sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP
/sys/fs/bpf/unheaded/doom-ring/maps/STATS
/sys/fs/bpf/unheaded/doom-ring/maps/L1_CACHE
/sys/fs/bpf/unheaded/doom-ring/maps/RV2MBC_MAP
/sys/fs/bpf/unheaded/doom-ring/maps/COMPUTE_EVENTS
```

### STATS Map Keys
```
0: PACKETS_TOTAL    5: NO_STATE
1: CPU_TICKS        6: MEM_FAULTS
2: INSNS_EXECUTED   7: SYSCALLS
3: HALTED           8: ROM_FAULT
4: SLEEPING         9: CACHE_HITS
                   10: CACHE_MISSES
```

### Doom CLI Quick Reference
```bash
go run ./cmd/doom/ load <rom.mbc>     # Load ROM into ROM_MAP
go run ./cmd/doom/ status             # Show CPU state
go run ./cmd/doom/ reset              # Reset CPU to defaults
go run ./cmd/doom/ input <hex-bitmap> # Inject keyboard
```

### Packet Injection Quick Reference
```bash
sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 1          # Single
sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 0 --rate 8600  # Continuous
sudo python3 scripts/doom-tick.py --flow-label 0xDE --count 10000 --burst  # Burst
```

---

*S31 Battle Plan — Forged 2026-02-21*
*20 Phases. 403 Steps. The Protocol Awakening awaits.*
*Doom will run over IPv6. The wire WILL be the processor.*
