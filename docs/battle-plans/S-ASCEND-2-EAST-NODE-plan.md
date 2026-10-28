# S-ASCEND-2 E3: east as a node, with a safety net

**Status**: PLAN (2026-10-28). Answer to E3: "scaffold/observe, research, plan; want to expand, just use caution;
maybe build an auto-remediation method if something goes wrong." Done so far: read-only observation of east, the
safety net built and tested (lab and a no-op self-test on east), Wotan `-bind`. **Nothing has been attached on
east.** Each live stage below starts only on Stevie's go (one go can cover all of them).

Covers the cross-host column of S-ASCEND-2 Phase 8 (Steps 233-238): Mímir Phases 9/11/13 and the DISTRIBUTED
weights leg, which passed between two network namespaces on west (`references/s-ascend-2-log.md` Phase 8).

## East as observed (read-only, 2026-10-28)

| Item | State |
|---|---|
| Kernel, systemd | 6.17.0-41-generic, systemd 257 (same as west) |
| Links | `enp2s0` UP (192.168.13.1/30, the P2P link to west, default route via 192.168.13.2); `wg0` rides on it; `enp5s0`, `enp6s0`, `wlp7s0` down |
| XDP on `enp2s0` | none |
| Tools | `bpftool` present; passwordless sudo for govan |
| Services | `unheaded-chaos-controller` active (leave alone); `unheaded-heimdall` inactive |
| Memory | 7.2 GB, 5.9 GB available |

## Risks

1. **`enp2s0` is east's only live link.** A program on it that drops traffic, or a kernel fault in XDP generic mode,
   cuts east off from west, the WireGuard overlay and the internet. Recovery would then need a console. *Open
   question for Stevie: is there out-of-band access to east (physical keyboard, KVM, a second link)?*
2. Heimdall's `vfs_write` and `execve` kprobes see the whole host (observe-only; small programs, 20 and 21
   verified instructions).
3. Lab binaries run as root from `/tmp`.
4. A scratch Wotan on west must not listen on west's LAN.
5. Lab keys travel to east.

## Mitigations

- **The program cannot drop.** `crates/heimdall-bpf/src/gjallarhorn_xdp.rs` returns `XDP_PASS` on every path
  (`observe(&ctx); xdp_action::XDP_PASS`). Generic mode only.
- **The cartridge leg never touches `enp2s0`.** `cartridge-ebpf` loads inside a network namespace on east.
- **The node guard** (`scripts/node-guard.sh`) runs on east as its own transient systemd unit, so it keeps running if
  SSH drops. It remediates once, when the peer (192.168.13.2) misses 5 pings in a row (about 10 s), or when its lease
  (20 min per stage, renewable) runs out. Remediation is a fixed list:
  1. kill processes under the lab directory `/tmp/s6`, which closes aya's XDP link;
  2. remove `/sys/fs/bpf/unheaded-lab*`;
  3. detach netlink-attached XDP from `enp2s0` (generic, native, offload);
  4. detach any XDP link still bound to `enp2s0` (`bpftool link detach`);
  5. verify and log.

  `disarm` stops it without remediating. The guard lives in `/tmp/s6-guard`, outside the directory it cleans.
- **Wotan `-bind`** (new flag, default unchanged): the scratch Wotan binds `192.168.13.2` only.
- **Lab keys** are generated for the run, kept in `/tmp/s6` on east and deleted at teardown. They are never
  production keys. Heimdall gets `AUTH_KEY_FILE` pointed at a missing file, so it never reads east's
  `/etc/unheaded/keys`.

**Guard evidence.**

| Test | Result |
|---|---|
| Lab: link cut (heimdall with XDP on a lab node, peer link downed) | remediated after 3 misses, about 8 s; heimdall killed; no XDP |
| Lab: lease expiry (8 s) | remediated at the lease |
| Lab: disarm | guard stopped; heimdall and its XDP untouched |
| East: self-test (20 s lease, nothing attached) | no missed pings; lease remediation ran as no-ops; guard removed |

The first lab run found that netlink cannot detach aya's link-based XDP, which is why killing the owner comes first.

## Stages (each needs a go)

**S0 — stage, on west then east (no attach).**
1. Build `heimdall-daemon`, `gjallarhorn-sender`, `upc-cartridgectl`, `cart`, the heimdall-bpf objects and a lab
   manifest over scratch files on east; seal it with a fresh lab key.
2. `scp` to `east:/tmp/s6/` and the guard to `east:/tmp/s6-guard/`; compare sha256 on both ends.

**S1 — heimdall on east, no XDP (kprobes only).**
1. Arm the guard: `sudo bash /tmp/s6-guard/node-guard.sh arm --lease 1200`.
2. Start the scratch Wotan on west:
   `wotan -bind 192.168.13.2 -http-port 26601 -grpc-port 26602 -topic-config <lab config>`, auth off. Check
   `ss -ltn` shows only 192.168.13.2.
3. Start heimdall on east without `--xdp-iface`.
4. Prove: seal verified; 20 drifts on east's scratch files alert and reach the west scratch Wotan signed; a tampered
   or expired seal stops the daemon.

**S2 — XDP observe on `enp2s0`.**
1. Guard renewed for 20 min.
2. Restart heimdall with `--xdp-iface enp2s0 --xdp-mode generic`.
3. Watch for 2 minutes: ping, WireGuard handshake and SSH stay up.
4. Send 7 bootstrap horns from west (multicast won't route over a /30 P2P, so the bootstrap is unicast here; record
   it), reverify for this cluster and another, then a 100-horn flood.
5. Record K2 and K3 across hosts.
6. Stop heimdall and confirm no XDP on `enp2s0`.
7. Disarm.

**S3 — weights across hosts.**
1. Guard armed.
2. `upc-cartridgectl weights publish` on west to the scratch Wotan.
3. `weights fetch` on east: seal verified, DATA sha equal.
4. Score the same jobs on both with `cart tick`.
5. Optionally load `cartridge-ebpf` in a netns on east, never on `enp2s0`.
6. Disarm.

**S4 — teardown and record.**
1. Stop the scratch Wotan.
2. On east: `rm -rf /tmp/s6 /tmp/s6-guard`; no heimdall, cart or guard process; `bpftool prog show` lists none of
   ours; no XDP on `enp2s0`.
3. In the log, ADR-043 and the wiki, switch the wording from "in netns" to "across hosts" where S1-S3 passed.

## Abort

At any point:

```bash
ssh east 'sudo bash /tmp/s6-guard/node-guard.sh disarm; sudo pkill -KILL -f "^/tmp/s6/"; sudo ip link set dev enp2s0 xdpgeneric off'
```

If SSH is already gone, the guard acts on its own within about 10 s of losing the peer, or at the lease.
