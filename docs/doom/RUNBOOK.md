# Doom-over-IPv6 -- Operational Runbook

**Last updated:** 2026-03-30
**Prerequisite:** This runbook assumes `scripts/clean-artifacts.sh --nuke` has been run.
All build artifacts must be rebuilt from source.

## Prerequisites

**Tools required:**
- `riscv64-unknown-elf-gcc` (RISC-V cross-compiler, RV32I capable)
- `cargo` + Rust toolchain (for doom-runner and rv32i-to-mbc)
- `bpftool` (for XDP attachment and map inspection)
- Linux kernel 5.15+ with BTF support (for tail calls)
- Firefox or Chrome (for canvas viewer)

**Source locations (never modified):**
- id DOOM: `~/tmp/projects/DOOM/linuxdoom-1.10/` (62 .c files, GPL-2.0)
- WAD: `~/tmp/projects/doom-related/DOOM.WAD` (12,408,292 bytes, retail Steam)

**Working directory for all commands:**
```bash
cd ~/tmp/unheaded
```

## Step 1: Build the MBC Translator

The translator converts RV32I ELF to MBC bytecode.

```bash
cd crates/monad-mbc && cargo build --release --bin rv32i-to-mbc && cd ../..
```

**Verify:** `ls crates/monad-mbc/target/release/rv32i-to-mbc` exists.

## Step 2: Build the Doom MBC Binary

Compiles id DOOM source + MBC platform stubs -> RV32I ELF -> MBC bytecode.

```bash
cd demos/doom && make clean && make && cd ../..
```

**Expected output:**
```
[CC] .../am_map.c (id DOOM)
... (57 id DOOM files + 4 MBC stubs + 3 support files)
[Link] doom.elf
[Translate] RV32I -> MBC
Instructions: 85454
```

**Verify:**
```bash
ls -la demos/doom/doom.elf demos/doom/doom.mbc demos/doom/doom.rv2mbc
# doom.elf: ~1.4 MiB (RV32I bare-metal ELF)
# doom.mbc: ~334 KiB (MBC bytecode)
# doom.rv2mbc: address translation map
```

**If it fails:** Check that `riscv64-unknown-elf-gcc` is on PATH and that the
Makefile's `IDDOOM_DIR` points to `~/tmp/projects/DOOM/linuxdoom-1.10`.

## Step 3: Build the eBPF Program

```bash
make ebpf-monad-cpu
```

**Verify:**
```bash
ls ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf
```

## Step 4: Build doom-runner

```bash
cd crates/doom-runner && cargo build --release && cd ../..
```

**Verify:**
```bash
ls crates/doom-runner/target/release/doom-runner
```

## Step 5: Validate Memory Layout

Optional but recommended. Prints the full address map and checks for overlaps.

```bash
sudo ./crates/doom-runner/target/release/doom-runner layout
```

**Expected:** "Layout validation: PASS"

## Step 6: Launch doom-runner

This sets up the network ring, loads XDP + all data into BPF maps, and starts
the WebSocket bridge.

```bash
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc demos/doom/doom.mbc \
  --doom-elf demos/doom/doom.elf \
  --rv2mbc demos/doom/doom.rv2mbc \
  --wad ~/tmp/projects/doom-related/DOOM.WAD \
  --hops 2 &
```

**Expected output (in order):**
```
doom-runner: Aya-based full pipeline (no pins, no mismatches)
memory layout validated
network ring ready (2 hops)
loading eBPF program from ...
eBPF object loaded
maps created by eBPF program: [...]
monad_cpu XDP program loaded into kernel
tail call chain: TAIL_CALL_PROGS[0] = monad_cpu (self), 256 insns/tick (16 rounds x 16 insns)
... parsing, writing, verifying ...
ROM_MAP: PASS (...)
CPU_MAP: PASS (...)
RAM_MAP: PASS (WAD magic = 'IWAD')
STATS: PASS (empty)
doom-runner: pipeline complete in X.Xs
bridge: listening on http://0.0.0.0:16666
bridge: server ready
```

**Wait ~6 seconds** for the bridge to stabilize before proceeding.

## Step 7: Attach XDP to Network Namespaces

doom-runner loads the XDP program but namespace attachment requires `nsenter`:

```bash
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p
```

**Verify:**
```bash
sudo ip netns exec monad0 bpftool net list
# Should show monad_cpu attached to veth10p
sudo ip netns exec monad1 bpftool net list
# Should show monad_cpu attached to veth01p
```

## Step 8: Start Packet Injection

Packets drive execution. Each packet traverses the ring and triggers MBC execution.

```bash
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/ {print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/ {print $2}')
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac "$SRC_MAC" --dst-mac "$DST_MAC" --iface veth01 \
  --count 0 --mode sendmmsg --batch 200 &
```

**Note:** `--count 0` means infinite. The injector runs until killed.

## Step 9: Play Doom

Open **http://localhost:16666** in Firefox or Chrome.

**Controls:**
| Key | Action |
|-----|--------|
| Arrow keys / WASD | Move and turn |
| Ctrl / L | Fire weapon |
| Space | Use (open doors, switches) |
| Enter | Menu select / start game |
| Escape | Menu / pause |
| Shift | Run |
| Alt | Strafe modifier |
| < , > | Strafe left/right |
| 1-7 | Switch weapon |
| F1-F12 | Function keys |

**What you should see:**
1. Title screen with DOOM logo and correct colors
2. Demo playback (Doom plays itself in attract mode)
3. Press Enter to start New Game
4. Full gameplay: movement, shooting, doors, HUD

## Monitoring

### Instruction Count
```bash
sudo bpftool map lookup name STATS key hex 02 00 00 00
# Returns total MBC instructions executed
```

### Current PC (check for corruption)
```bash
sudo bpftool map lookup name CPU_MAP key hex de 00 00 00
# PC should be in range 0 to ~85454 (ROM size)
```

### Screen Pixels (non-zero = rendering active)
```bash
sudo bpftool map dump name SCREEN_MAP | grep -cv "value: 00$"
```

### Bridge FPS
The browser status bar shows: connected | frames: N | fps: X.X

## Teardown

```bash
# Kill injector
sudo pkill doom-go-injector

# Kill doom-runner (Ctrl-C in its terminal, or:)
sudo pkill doom-runner

# Tear down network ring
sudo ./crates/doom-runner/target/release/doom-runner ring teardown
```

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| "WAD magic" verification fails | Wrong WAD path or file | Check `--wad` points to valid DOOM.WAD |
| Black screen in browser | doom-runner not started, or injector not running | Check steps 6-8 |
| "disconnected" in browser | Bridge not running | Check doom-runner is alive |
| PC > 85454 | PC corruption | See FINDINGS.md, check for regression |
| No key response | KBD_MAP not draining | Check I_StartTic loop runs |
| "failed to load eBPF object" | Missing eBPF build | Run step 3 |
| "monad_cpu XDP program not found" | Wrong eBPF object | Check --ebpf-obj path |
