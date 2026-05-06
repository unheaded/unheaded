# POST-REBOOT DOOM REVIVAL — 5 Phases, 45 Steps

**Date**: 2026-03-27
**Sprint**: S80 — Doom walks the Pattern again
**Prerequisite**: WEST rebooted, Linux booted, SSH accessible
**Target**: Doom rendering real frames in browser at http://localhost:16666
**Estimated Duration**: 30-45 minutes
**Commit Cadence**: After each phase gate
**Stuck Protocol**: Skip after 2 failed attempts, log and continue

## LEGEND

[B] = Bash command  [V] = Verification  [D] = Debug  [S] = Sudo required  [C] = Commit

---

## PHASE 0: POST-REBOOT VERIFICATION (Steps 1-8)

**Goal**: Confirm clean system, no stale BPF, all tools present.
**Time**: 3 minutes

- [ ] **Step 1** [B]: Set project root
  ```bash
  cd ~/tmp/unheaded && export P=$(pwd) && echo "Project: $P"
  ```

- [ ] **Step 2** [V]: Confirm ZERO stale BPF programs
  ```bash
  sudo bpftool prog list 2>/dev/null | grep monad_cpu && echo "STALE — STOP" || echo "CLEAN"
  ```
  - Must show "CLEAN". If stale → something survived reboot (shouldn't happen).

- [ ] **Step 3** [V]: Confirm BPF filesystem clean
  ```bash
  ls /sys/fs/bpf/unheaded 2>/dev/null && echo "STALE PINS" || echo "CLEAN"
  ```

- [ ] **Step 4** [V]: Kernel and tools
  ```bash
  uname -r && which bpftool ip nsenter cargo go riscv64-unknown-elf-gcc | head -7
  ```

- [ ] **Step 5** [V]: Doom artifacts exist
  ```bash
  ls -la $P/doom/doom.{elf,mbc,rv2mbc} $P/bin/doom-{loader,bridge,go-injector} 2>&1 | head -6
  ```
  - If binaries missing → Step 6. If present → Step 7.

- [ ] **Step 6** [D][BUILD]: Rebuild if needed
  ```bash
  make ebpf-monad-cpu && go build -o bin/doom-loader ./cmd/doom-loader/ && go build -o bin/doom-bridge ./cmd/doom-bridge/ && go build -o bin/doom-go-injector ./cmd/doom-go-injector/
  ```

- [ ] **Step 7** [V]: WAD file exists
  ```bash
  file /home/govan/tmp/projects/doom-related/doom1.wad | grep -q "DOOM" && echo "WAD OK" || echo "WAD MISSING"
  ```

- [ ] **Step 8** [V]: **PHASE 0 EXIT GATE** — Clean BPF, tools present, artifacts exist, WAD found

---

## PHASE 1: RING SETUP + MAP ALIGNMENT (Steps 9-20)

**Goal**: 2-hop ring with XDP attached, maps aligned between program and pins.
**Time**: 5 minutes

- [ ] **Step 9** [S][B]: Setup 2-hop doom ring
  ```bash
  sudo DOOM_RING_HOPS=2 $P/scripts/doom-ring.sh setup 2>&1 | tail -15
  ```

- [ ] **Step 10** [V]: Loader daemon alive
  ```bash
  ps aux | grep ebpf-loader | grep -v grep | head -1 && echo "ALIVE" || echo "DEAD"
  ```
  - If DEAD → Step 10a
  - If ALIVE → Step 11

- [ ] **Step 10a** [D]: Check loader log and restart manually
  ```bash
  cat /tmp/doom-ring-loader.log | tail -10
  # If verifier rejection: STOP — need to adjust MAX_INSN_PER_TICK
  # If interface error: check namespaces exist
  ```

- [ ] **Step 11** [V]: Both hops have XDP
  ```bash
  sudo nsenter --net="/var/run/netns/monad0" ip link show veth10p 2>&1 | grep xdp && echo "HOP0 OK"
  sudo nsenter --net="/var/run/netns/monad1" ip link show veth01p 2>&1 | grep xdp && echo "HOP1 OK"
  ```

- [ ] **Step 12** [V]: **MAP ALIGNMENT — THE CRITICAL CHECK**
  ```bash
  PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
  PROG_MAPS=$(sudo bpftool prog show id $PROG_ID | grep map_ids | sed 's/.*map_ids //')
  STATS_PIN=$(sudo bpftool map show pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS | head -1 | awk '{print $1}' | tr -d ':')
  CPU_PIN=$(sudo bpftool map show pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP | head -1 | awk '{print $1}' | tr -d ':')
  echo "Program $PROG_ID maps: $PROG_MAPS"
  echo "Pinned STATS=$STATS_PIN CPU=$CPU_PIN"
  echo "$PROG_MAPS" | tr ',' '\n' | grep -q "^${STATS_PIN}$" && echo "STATS ALIGNED" || echo "STATS MISALIGNED"
  echo "$PROG_MAPS" | tr ',' '\n' | grep -q "^${CPU_PIN}$" && echo "CPU ALIGNED" || echo "CPU MISALIGNED"
  ```
  - **MUST show ALIGNED for both.** On clean reboot this should work.
  - If MISALIGNED → STOP. The aya loader created different maps than it pinned. Debug the loader.

- [ ] **Step 13** [V]: **PHASE 1 EXIT GATE** — Ring up, XDP on both hops, maps aligned

---

## PHASE 2: DATA LOADING (Steps 14-22)

**Goal**: ROM, data, WAD, CPU state all loaded into the CORRECT maps.
**Time**: 5 minutes (WAD is 4MB, takes ~60s to load)

- [ ] **Step 14** [S][B]: Load ROM
  ```bash
  sudo $P/scripts/doom-loader.sh rom 2>&1 | tail -1
  ```

- [ ] **Step 15** [S][B]: Load RV2MBC translation table
  ```bash
  sudo $P/scripts/doom-loader.sh rv2mbc 2>&1 | tail -1
  ```

- [ ] **Step 16** [S][B]: Load .data/.rodata sections
  ```bash
  sudo $P/scripts/doom-loader.sh data 2>&1 | tail -1
  ```

- [ ] **Step 17** [S][B]: Load WAD at 0x800000
  ```bash
  sudo DOOM_WAD=/home/govan/tmp/projects/doom-related/doom1.wad DOOM_WAD_BASE=0x800000 $P/scripts/doom-loader.sh wad 2>&1 | tail -1
  ```

- [ ] **Step 18** [S][B]: Initialize CPU state
  ```bash
  sudo $P/scripts/doom-loader.sh cpu 2>&1 | tail -1
  ```

- [ ] **Step 19** [V]: WAD magic at correct address
  ```bash
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP key hex 00 00 20 00 2>&1
  ```
  - Must show `value: 49 57 41 44` ("IWAD")

- [ ] **Step 20** [V]: ROM has instructions
  ```bash
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP key hex 00 00 00 00 2>&1
  ```
  - Must show non-zero value (first MBC instruction)

- [ ] **Step 21** [V]: CPU initialized
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP 2>&1 | head -6
  ```
  - SP (bytes 60-63) should be `00 00 f0 03` (0x03F00000)

- [ ] **Step 22** [V]: **PHASE 2 EXIT GATE** — All data loaded, WAD verified, CPU ready

---

## PHASE 3: FIRST EXECUTION (Steps 23-33)

**Goal**: Inject packets, observe CPU executing, determine if Doom boots or hits I_Error.
**Time**: 10 minutes

- [ ] **Step 23** [B]: Get MAC addresses
  ```bash
  VETH01_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/{print $2}')
  VETH01P_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/{print $2}')
  echo "SRC=$VETH01_MAC DST=$VETH01P_MAC"
  ```

- [ ] **Step 24** [S][B]: Start injector (burst mode)
  ```bash
  sudo ip netns exec monad0 $P/bin/doom-go-injector --mode burst --batch 100 --count 0 --iface veth01 --src-mac "$VETH01_MAC" --dst-mac "$VETH01P_MAC" > /tmp/doom-injector.log 2>&1 &
  echo "Injector PID: $!"
  ```

- [ ] **Step 25** [B]: Wait 30s for init
  ```bash
  echo "Waiting 30s for Doom init..."; sleep 30
  ```

- [ ] **Step 26** [V]: Check STATS — packets being processed
  ```bash
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS key hex 00 00 00 00 2>&1
  ```
  - value must be non-zero (packets counted)
  - If zero → XDP not processing. Check map alignment (Step 12).

- [ ] **Step 27** [V]: Check CPU state — halted or running?
  ```bash
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP 2>&1 | head -8
  ```
  - Byte 69 = `01` → HALTED (I_Error fired). Go to Step 28.
  - Byte 69 = `00`, PC advancing → RUNNING. Go to Step 30.

- [ ] **Step 28** [D]: CPU halted — read debug buffer for I_Error message
  ```bash
  echo "=== DEBUG BUFFER (0x7BF000) ==="
  for slot in 0 1 2 3; do
    base=$((0x7BF000/4 + slot*16))
    msg=""
    for w in $(seq 0 15); do
      addr=$((base + w))
      hex=$(printf '%08x' $addr)
      val=$(sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP key hex ${hex:6:2} ${hex:4:2} ${hex:2:2} ${hex:0:2} 2>&1 | grep "value:" | awk '{print $2, $3, $4, $5}')
      for byte in $val; do
        [ "$byte" = "00" ] && break 2
        dec=$((16#$byte))
        [ $dec -ge 32 ] && [ $dec -le 126 ] && msg="${msg}$(printf "\\$(printf '%03o' $dec)")" || msg="${msg}."
      done
    done
    [ -n "$msg" ] && echo "  [$slot]: $msg"
  done
  ```
  - The error message tells us EXACTLY what failed.
  - Common errors and fixes:
    - "W_GetNumForName: X not found" → WAD not at expected address
    - "R_InitTextures: bad texture directory" → Data sections wrong
    - "Z_Init: zone memory" → Heap layout wrong
  - **DECISION POINT**: Fix root cause based on error, then make exit() non-fatal and re-run.

- [ ] **Step 29** [D]: If debug buffer empty, check exit marker
  ```bash
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP key hex 3e fc 1e 00 2>&1
  ```
  - If shows `45 52 52 00` ("ERR") → exit(-1) from I_Error
  - If shows `4f 4b 21 00` ("OK!") → exit(0) clean exit (shouldn't happen)

- [ ] **Step 30** [V]: CPU running — check screen diversity
  ```bash
  declare -A px
  for addr in $(seq 0 320 63999); do
    hex=$(printf '%08x' $addr)
    val=$(sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP key hex ${hex:6:2} ${hex:4:2} ${hex:2:2} ${hex:0:2} 2>&1 | grep "value:" | awk '{print $2}')
    [ -n "$val" ] && px[$val]=$(( ${px[$val]:-0} + 1 ))
  done
  echo "Unique palette values: ${#px[@]}"
  ```
  - **20+ unique values → DOOM IS RENDERING! Go to Phase 4.**
  - **4 values (00/40/80/c0) → renderer broken.** Need to make exit() non-fatal and fix I_VideoBuffer.

- [ ] **Step 31** [S][B]: Start bridge
  ```bash
  sudo $P/bin/doom-bridge --port 16666 > /tmp/doom-bridge.log 2>&1 &
  ```

- [ ] **Step 32** [V]: Bridge serving
  ```bash
  sleep 2 && curl -s http://localhost:16666/health
  ```

- [ ] **Step 33** [V]: **PHASE 3 EXIT GATE** — CPU executing, STATS counting, bridge up. Screen either rendering or diagnosis captured.

---

## PHASE 4: PLAYABILITY (Steps 34-38)

**Goal**: Doom playable in browser with keyboard input.
**Time**: 5 minutes
**Prerequisite**: Screen shows 20+ unique palette values

- [ ] **Step 34** [B]: Open in browser
  ```bash
  echo "Open http://localhost:16666 in browser"
  echo "You should see the Doom title screen or gameplay"
  ```

- [ ] **Step 35** [V]: Visual confirmation — non-black, non-uniform screen

- [ ] **Step 36** [B]: Test keyboard — press arrow keys in browser, check KBD_MAP updates
  ```bash
  sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/KBD_MAP key hex 00 00 00 00 2>&1
  ```

- [ ] **Step 37** [V]: Player responds to input (visual movement on screen)

- [ ] **Step 38** [V]: **PHASE 4 EXIT GATE** — Doom playable in browser. The Pattern walks.

---

## PHASE 5: INFRASTRUCTURE RESTORATION (Steps 39-45)

**Goal**: Restore WireGuard tunnel, BGP peering, Docker IPv6.
**Time**: 10 minutes
**Note**: Independent of Doom — can run in parallel or after.

- [ ] **Step 39** [S][B]: Bring up WireGuard
  ```bash
  sudo wg-quick up wg0 2>&1
  ```
  - If "already exists" → already up from systemd

- [ ] **Step 40** [V]: WireGuard tunnel operational
  ```bash
  ping6 -c 2 -W 2 fd00:dead:beef::1 2>&1 | tail -3
  ```

- [ ] **Step 41** [D]: If WireGuard fails, check Docker IPv6 conflict
  ```bash
  ip -6 addr show dev docker0 | grep "dead:beef" && echo "CONFLICT — fix Docker" || echo "No conflict"
  # Fix: sudo ip -6 addr del fd00:dead:beef::1/48 dev docker0
  ```

- [ ] **Step 42** [S][B]: Verify BGP peering
  ```bash
  sudo vtysh -c "show bgp summary" 2>&1 | head -10
  ```
  - If FRR not running: `sudo systemctl start frr`

- [ ] **Step 43** [B]: Verify EAST BGP
  ```bash
  ssh govan@east "sudo birdc show protocol all west 2>/dev/null | head -5" 2>&1
  ```

- [ ] **Step 44** [V]: Docker IPv6 on non-conflicting subnet
  ```bash
  grep "fixed-cidr-v6" /etc/docker/daemon.json
  ```
  - Should show `fd00:d0c0:e700::/48` NOT `fd00:dead:beef::/48`

- [ ] **Step 45** [V]: **PHASE 5 EXIT GATE** — WireGuard up, BGP established, Docker clean

---

## QUICK REFERENCE

**Map Pin Path**: `/sys/fs/bpf/unheaded/doom-ring/maps/`
**Loader PID File**: `/run/doom-ring/hop0.pid`
**Loader Log**: `/tmp/doom-ring-loader.log`
**Injector Log**: `/tmp/doom-injector.log`
**Bridge Log**: `/tmp/doom-bridge.log`
**Debug Buffer**: RAM address `0x7BF000` (word addr `0x1EFC00`)
**WAD Address**: `0x800000` (word addr `0x200000`, magic "IWAD" = `49 57 41 44`)
**CPU Instance Key**: `0xDE` (hex `de 00 00 00`)
**Screen Size**: 320×200 = 64,000 entries in SCREEN_MAP
**Bridge URL**: http://localhost:16666

---

*S80 Post-Reboot Doom Revival — Forged 2026-03-27*
*5 Phases. 45 Steps. The Pattern waited. It always waited. That was its function.*
*"Let's see what breaks." — Mad Maria*
