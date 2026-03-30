# Doom-over-IPv6 Status

**Last verified:** 2026-03-30 00:00 UTC
**Commits this session:** 62 (af221910 latest)

## VERIFIED WORKING (on disk, tested, committed)

| Feature | Status | Commit | Verification |
|---------|--------|--------|-------------|
| id DOOM compiles for RV32I MBC | WORKING | 4a16480a | `make` produces doom.elf + doom.mbc |
| Retail DOOM.WAD loads (12.4MB) | WORKING | b1abb44c | doom-runner: "IWAD magic verified" |
| 35 fps game logic | WORKING | d5510aec | STAT_FRAME_READY delta = 35/sec |
| 62,754 screen pixels | WORKING | ff12d869 | bpftool map dump SCREEN confirms |
| Direct render (no memcpy) | WORKING | d5510aec | screens[0] = SCREEN_BASE |
| Dynamic palette from PLAYPAL | WORKING | af221910 | I_SetPalette writes 768B to 0x60000 |
| Bridge reads RAM_MAP | WORKING | f8a10e61 | 16K word reads per frame |
| Firefox WebSocket connection | WORKING | 0d3b6c43 | Client connects, frames stream |
| Tail call chain (256 insn/tick) | WORKING | e55d2f51 | BTF enabled, self tail-call |
| doom-runner Aya pipeline | WORKING | 208c2944 | Atomic program+map ownership |
| Soft-float IEEE 754 (25 funcs) | WORKING | b29a14b2 | Integer bit manipulation, no recursion |
| sprintf precision (%.3d) | WORKING | 641901aa | STCFN033 font lumps found |
| JVM-style dynamic heap | WORKING | 947b38b5 | sbrk from __heap_start linker symbol |
| POSIX fd stubs (open/read/lseek) | WORKING | efd094b0 | WAD reads via fd table |

## NOT YET WORKING

| Feature | Blocker | Next Step |
|---------|---------|-----------|
| Correct palette in browser | JS may still use hardcoded palette for some messages | Verify in browser, check JS path |
| Keyboard input | Bridge kbd_writer exists but not tested | Test key events → KBD_MAP → I_StartTic |
| Playable controls | Depends on keyboard | Wire bridge → KBD_MAP → MBC syscall |
| Browser frame rate | Bridge polls 16K words per frame (slow) | Batch map reads or reduce poll frequency |
| Sound | By design — no-op stubs | Future: audio over WebSocket |

## HOW TO RUN

```bash
cd ~/tmp/unheaded

# Build
cd demos/doom && make && cp doom.elf doom.mbc doom.rv2mbc ../../doom/ && cd ../..
cd crates/doom-runner && cargo build --release && cd ../..
make ebpf-monad-cpu

# Launch
sudo ./crates/doom-runner/target/release/doom-runner run \
  --doom-mbc doom/doom.mbc --rv2mbc doom/doom.rv2mbc \
  --doom-elf doom/doom.elf \
  --wad ~/tmp/projects/doom-related/DOOM.WAD --hops 2 &

# Attach XDP
PROG_ID=$(sudo bpftool prog list | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo ip netns exec monad1 bpftool net attach xdpgeneric id $PROG_ID dev veth01p
sudo ip netns exec monad0 bpftool net attach xdpgeneric id $PROG_ID dev veth10p

# Inject
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/ {print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/ {print $2}')
sudo nsenter --net=/var/run/netns/monad0 ./bin/doom-go-injector \
  --src-mac "$SRC_MAC" --dst-mac "$DST_MAC" --iface veth01 \
  --count 0 --mode sendmmsg --batch 200 &

# Open browser
# http://localhost:16666
```
