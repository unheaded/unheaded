# DOOM on the UPC — Operational Runbook

**Last updated:** 2026-07-24
**Status:** Playable. E1M1 completable, E1M2 loads with texture issues.
**Working directory for all commands:** `~/tmp/unheaded`

---

## Quick reference

```
START:    sudo systemctl start unheaded-doom.target
STOP:     sudo systemctl stop  unheaded-doom.target
BROWSER:  http://localhost:16666/
```

If systemd units aren't installed yet, or you need to start manually, follow
the sections below in order.

---

## Prerequisites

### Tools (should already be present on WEST)

```bash
which riscv64-unknown-elf-gcc   # RISC-V cross-compiler
which bpftool                   # BPF inspection + attach
which go                        # Go toolchain
```

If any are missing: `sudo apt install binutils-riscv64-unknown-elf gcc-riscv64-unknown-elf bpftool`

### WAD file

Shareware (free, sufficient for E1):
```
~/tmp/projects/doom-related/doom1.wad
```

Retail (full game — Ultimate DOOM, 4 episodes):
```
~/tmp/projects/doom-related/DOOM.WAD
```

Either works with the same build. Pass the one you want to `--wad` (full path,
not `~`); doom-runner reads the lump directory and picks the game mode itself:

| IWAD contains | Detected flavour | gamemode   |
|---------------|------------------|------------|
| `MAP01`       | Commercial       | commercial |
| `E4M1`        | Retail           | retail     |
| `E2M1`        | Registered       | registered |
| `E1M1` only   | Shareware        | shareware  |

Confirm the detection in the runner log:
```
detected IWAD flavour: Retail (2306 lumps) — probe name 'doomu.wad'
```

Why this matters: id DOOM's `IdentifyVersion()` picks the game mode by
`access()`-probing filenames, and our stub can report exactly one of them as
present. Getting it wrong is not cosmetic — `registered` demands a `HELP2` lump
Ultimate DOOM lacks, and `retail` plays a `DEMO4` that shareware lacks (that
one used to `I_Error` out about 5 seconds in).

---

## Part 1 — Build (one-time, or after source changes)

Skip to Part 2 if the binaries below already exist.

### Check what's built

```bash
ls -lh crates/doom-runner/target/release/doom-runner          # Rust runtime
ls -lh ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf  # eBPF kernel prog
ls -lh demos/doom/doom.mbc                                     # DOOM MBC bytecode
ls -lh cmd/doom-go-injector/doom-go-injector                   # Packet injector
```

### Build 1: DOOM MBC bytecode (needs RISC-V compiler — most likely missing)

This compiles id DOOM C source → RV32I ELF → MBC bytecode. Takes ~30 seconds.

```bash
# Build the RV32I-to-MBC translator first
cd crates/monad-mbc && cargo build --release --bin rv32i-to-mbc && cd ../..

# Build DOOM
cd demos/doom && make clean && make && cd ../..
```

Expected output ends with:
```
[Translate] RV32I -> MBC
Instructions: 85454
```

Verify:
```bash
ls -lh demos/doom/doom.mbc   # ~334 KB
ls -lh demos/doom/doom.elf   # ~1.4 MB
ls -lh demos/doom/doom.rv2mbc
```

**If `make` fails:** check `IDDOOM_DIR` in `demos/doom/Makefile` points to
`~/tmp/projects/DOOM/linuxdoom-1.10`.

### Build 2: eBPF kernel program (DOOM mode — NOT ascend-linux)

```bash
cd ebpf && cargo +nightly build -p monad-cpu-ebpf \
  --target bpfel-unknown-none \
  -Z build-std=core \
  --release
cd ..
```

**Critical:** do NOT add `--features ascend-linux`. That flag changes the MBC
interpreter semantics for xv6/Linux. Using it will produce a black screen.

Verify it's the DOOM build:
```bash
strings ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf | grep 'feature = "ascend-linux"'
```
Lines like `if !cfg!(feature = "ascend-linux") && ...` are **expected** — they are
source text embedded in debug info, not evidence the feature is on. A plain
`grep ascend` matches them and is a false positive; don't rebuild on that alone.

### Build 3: doom-runner (Rust)

```bash
cd crates/doom-runner && cargo build --release && cd ../..
```

### Build 4: doom-go-injector (Go)

```bash
cd cmd/doom-go-injector && go build -o doom-go-injector . && cd ../..
```

Verify:
```bash
ls -lh cmd/doom-go-injector/doom-go-injector
```

---

## Part 2 — Teardown (always run before starting)

If DOOM was previously running (or killed with `kill PID`), stale network
namespaces and BPF state will prevent a clean start. Always tear down first.

```bash
# Kill any leftover processes
sudo pkill -f doom-go-injector 2>/dev/null || true
sudo pkill -f doom-runner     2>/dev/null || true

# Tear down the ring (removes monad0/monad1 namespaces + veth pairs + BPF pins)
sudo ./scripts/doom-ring.sh teardown

# Confirm namespaces are gone
ip netns list | grep monad  # should print nothing
```

---

## Part 3 — Startup (manual, step by step)

### Step 1: Start doom-runner

doom-runner creates the ring, loads DOOM into BPF maps, and starts the WebSocket
bridge on port 16666. Run it in a terminal you can watch.

```bash
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc demos/doom/doom.mbc \
  --doom-elf  demos/doom/doom.elf \
  --rv2mbc   demos/doom/doom.rv2mbc \
  --wad      /home/govan/tmp/projects/doom-related/doom1.wad \
  --hops 2
```

Wait for this line before continuing:
```
bridge: server ready
```

The full startup takes 5–15 seconds. Expected output sequence:
```
doom-runner: Aya-based full pipeline
memory layout validated
network ring ready (2 hops)
loading eBPF program...
ROM_MAP: PASS
CPU_MAP: PASS
RAM_MAP: PASS (WAD magic = 'IWAD')
doom-runner: pipeline complete in X.Xs
bridge: listening on http://0.0.0.0:16666
bridge: server ready
```

If you see `WAD magic` fail: check the `--wad` path. Use the full path, not `~`.

### Step 2: Attach XDP (the critical step — do this every restart)

doom-runner loads the XDP program but deliberately does not attach it
(main.rs:429-436). Without this step, packets flow through the ring but never
trigger any eBPF execution — CPU_MAP stays at PC=0 and every frame is black.

Open a second terminal and run:

```bash
cd ~/tmp/unheaded

# Get the program ID
PROG_ID=$(sudo bpftool prog list | grep "name monad_cpu" | tail -1 | awk '{print $1}' | tr -d ':')
echo "Program ID: $PROG_ID"
# Should print a number (e.g. "Program ID: 276"). If empty, doom-runner isn't up yet.

# Attach to both ring ingress interfaces
# (xdpgeneric = software XDP mode, required for veth interfaces)
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p
```

Verify the attach worked:
```bash
sudo ip netns exec monad1 bpftool net list
# Should show: xdp: monad_cpu  id=<N>  on veth01p

sudo ip netns exec monad0 bpftool net list
# Should show: xdp: monad_cpu  id=<N>  on veth10p
```

### Step 3: Start the packet injector

The injector circulates packets through the ring. Each packet triggers one
monad_cpu XDP execution = one DOOM instruction tick.

```bash
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01  | awk '/ether/{print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/{print $2}')

sudo ip netns exec monad0 \
  ./cmd/doom-go-injector/doom-go-injector \
    -iface veth01 \
    -src-mac $SRC_MAC \
    -dst-mac $DST_MAC \
    -mode sendmmsg \
    -batch 64 \
    -flow-label 222 \
    -count 0 \
  &>/tmp/doom-injector.log &

echo "Injector PID: $!"
```

Verify after 3 seconds:
```bash
grep "pkt/s" /tmp/doom-injector.log | tail -3
# Should show packet rate, e.g. "19500 pkt/s"
```

### Step 4: Open the browser

```
http://localhost:16666/
```

You should see the DOOM title screen within a few seconds.

---

## Verification — is DOOM actually rendering?

If the browser shows black, run this check:

```bash
python3 -c "
import asyncio, websockets

async def check():
    ws = await websockets.connect('ws://localhost:16666/ws', max_size=None)
    frame = (await ws.recv() for _ in range(5))
    f = None
    async for _ in range(5):
        f = await ws.recv()
    palette_nonzero = sum(1 for b in f[:768] if b)
    screen_nonzero  = sum(1 for b in f[768:] if b)
    print(f'palette nonzero bytes: {palette_nonzero}/768')
    print(f'screen  nonzero bytes: {screen_nonzero}/64000')
    if palette_nonzero == 0:
        print('BLACK SCREEN — XDP not attached or injector not running')
    else:
        print('RENDERING OK')

asyncio.run(check())
"
```

Alternative quick check via bpftool:
```bash
# CPU_MAP key 0xde = register file for core 0 — PC should be advancing
sudo bpftool map lookup name CPU_MAP key hex de 00 00 00
# Look for "value: XX XX XX XX" where first 4 bytes (PC) are non-zero and changing

# SCREEN_MAP should have non-zero entries
MAP_ID=$(sudo bpftool map list | awk '/SCREEN_MAP/{print $1}' | tr -d ':')
sudo bpftool map dump id $MAP_ID 2>/dev/null | grep -c "value" | xargs echo "SCREEN_MAP entries:"
```

---

## Systemd startup (preferred once units are installed)

Install once:
```bash
sudo cp deploy/systemd/unheaded-doom*.service /etc/systemd/system/
sudo cp deploy/systemd/unheaded-doom.target   /etc/systemd/system/
sudo systemctl daemon-reload
```

Then every session:
```bash
# Start full pipeline (ring + runner + XDP attach + injector)
sudo systemctl start unheaded-doom.target

# Status
sudo systemctl status unheaded-doom-ring unheaded-doom-runner unheaded-doom-injector

# Stop and clean up
sudo systemctl stop unheaded-doom.target
```

The systemd units handle the XDP attach automatically via `doom-xdp-attach.sh`
as an `ExecStartPost=` step in `unheaded-doom-runner.service`.

---

## Shutdown

### Via systemd (preferred)
```bash
sudo systemctl stop unheaded-doom.target
# This stops injector, runner, and runs ring teardown automatically.
```

### Manual
```bash
sudo pkill -9 -f doom-go-injector

# pkill -f doom-runner is NOT reliable for the sudo-launched runner — kill by PID.
pgrep -f "doom-runner run" | xargs -r sudo kill -9

sudo ./scripts/doom-ring.sh teardown

# Confirm clean — all three must come back empty
ip netns list | grep monad               # namespaces gone
sudo ss -lntp | grep 16666               # bridge port released
sudo bpftool prog list | grep monad_cpu  # BPF program unloaded
```

The port check is the one that bites: a surviving runner keeps serving frames from
the *old* WAD while the next start silently dies on `Address already in use`.

---

## Controls

| Key | Action |
|-----|--------|
| Arrow keys | Move / turn |
| Ctrl | Fire |
| Space | Use (doors, switches) |
| Enter | Select / start game |
| Escape | Menu / pause |
| Shift | Run |
| 1–7 | Switch weapon |

---

## Monitoring

```bash
# Instruction count (total MBC instructions executed)
sudo bpftool map lookup name STATS key hex 02 00 00 00

# Current PC (should be non-zero and advancing)
sudo bpftool map lookup name CPU_MAP key hex de 00 00 00

# Injector packet rate
tail -f /tmp/doom-injector.log

# doom-runner log (if run via systemd)
sudo journalctl -u unheaded-doom-runner -f
```

---

## Troubleshooting

| Symptom | Most likely cause | Fix |
|---------|-------------------|-----|
| `doom.mbc: No such file` | doom.mbc not built | Part 1 Build 1 |
| `doom-go-injector: not found` | injector not built | Part 1 Build 4 |
| Black screen, palette=0 | XDP not attached | Step 2 (attach) |
| Black screen, palette non-zero | DOOM stuck pre-video-init (PC corruption) | See FINDINGS.md 2026-07-23 entry |
| `WAD magic verification fails` | Wrong WAD path | Use full path, check file exists |
| `monad0 not found` / `ip netns exec: ...` | Stale ring or ring not set up | Run teardown then retry from Step 1 |
| `name monad_cpu` not found in bpftool | doom-runner not started yet or crashed | Check doom-runner terminal output |
| Injector starts but `pkt/s` = 0 | XDP not attached (packets flow but no hook) | Step 2 |
| Browser shows "disconnected" | doom-runner crashed or bridge not up | Check doom-runner is still running |
| eBPF load fails with verifier error | ascend-linux eBPF object used | Rebuild eBPF without `--features ascend-linux` |
| Ring setup fails "namespace exists" | Stale namespaces from previous kill | Run teardown first, then setup |
| `Error: Address already in use (os error 98)` | A previous doom-runner still holds port 16666 — `pkill -f doom-runner` does **not** reliably kill the sudo-launched process | `pgrep -f "doom-runner run" \| xargs -r sudo kill -9`, then confirm `sudo ss -lntp \| grep 16666` is empty |
| Frames look like the *previous* WAD | Stale runner still serving; the new one exited on the bind error above | Same fix. Always check the new runner logged `server ready` and the expected `detected IWAD flavour` line |
| Wrong episodes / crash ~5s in / missing texture `I_Error` | IWAD flavour mismatch | Check the `detected IWAD flavour` line matches your WAD (see "WAD file" above) |

---

## Known issues

1. **XDP attach is manual** — doom-runner loads the XDP program but defers
   attachment (main.rs:429-436). The `doom-xdp-attach.sh` script and the
   systemd `ExecStartPost=` handle this automatically. If starting manually,
   Step 2 is mandatory every restart.

2. **E1M2 texture corruption** — some textures render incorrectly on E1M2.
   E1M1 is fully playable. See FINDINGS.md.

3. ~~**WAD size limit**~~ — resolved. `WAD_MAX_SIZE` is 16 MiB and the actual
   size is published to the guest at `WAD_SIZE_ADDR`. Both the retail
   `DOOM.WAD` (12,408,292 bytes) and shareware `doom1.wad` (4,196,020 bytes)
   load and run.
