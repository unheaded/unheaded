# Phase 10 System Configuration — Reproducibility Reference

All runtime system configuration applied during Phase 10 stress testing on WEST.
These are **ephemeral** — they do not survive a reboot unless persisted.

---

## 1. Network: ULA IPv6 on br-tomb

Tomb has no global IPv6. We added a ULA address to `br-tomb` so kernel-routed
IPv6 packets flow through the bridge to `tap-tomb`, hitting Shield XDP/TC.

```bash
# Add ULA address to bridge (host side)
sudo ip -6 addr add fd00:13:6::1/64 dev br-tomb

# Static neighbor entry for Tomb's MAC (QEMU virtio-net default)
# Without this, the kernel tries NDP which Tomb doesn't answer
sudo ip -6 neigh replace fd00:13:6::6 lladdr 52:54:00:ab:cd:ef dev br-tomb nud permanent
```

**Why ULA?** Link-local (fe80::) requires a scope ID and is interface-specific.
ULA (fd00::/8) is routable within our bridge and doesn't require internet connectivity.

**Address scheme:**
- `fd00:13:6::1` — Host (br-tomb)
- `fd00:13:6::6` — Tomb (destination for stress testing)
- `52:54:00:ab:cd:ef` — Tomb's MAC (QEMU default)

**Verify:**
```bash
ip -6 addr show dev br-tomb | grep fd00
ip -6 neigh show dev br-tomb
```

---

## 2. QDisc Tuning on tap-tomb

Default FQ qdisc has `flow_limit=100` per flow. At 920K pps all Monad packets
are in the same flow (same src/dst/port), overflowing the 100-packet bucket.
This caused 4.4% TX drops.

```bash
# Increase TX queue length (default: 1000)
sudo ip link set tap-tomb txqueuelen 10000

# Replace FQ qdisc with higher limits
sudo tc qdisc replace dev tap-tomb root fq limit 50000 flow_limit 10000
```

**Verify:**
```bash
tc qdisc show dev tap-tomb
# Should show: limit 50000p flow_limit 10000p
ip link show tap-tomb | grep qlen
# Should show: qlen 10000
```

**Note:** The `clsact` qdisc (parent ffff:fff1) is automatically created by
aya/bpftool when attaching TC programs. Do NOT delete it.

---

## 3. Shield eBPF Programs

Two BPF programs attached to `tap-tomb`:
- **shield_xdp** (XDP generic/SKB mode, ingress) — prog 2371
- **shield_tc** (TC egress via tcx) — prog 2372

### Loading

```bash
# Build eBPF programs
cd ebpf && cargo +nightly build -Z build-std=core \
    --target bpfel-unknown-none --release -p shield-ebpf

# Load and attach to tap-tomb
sudo /home/govan/tmp/unheaded/cmd/ebpf-loader/target/release/ebpf-loader \
    --interface tap-tomb \
    --only shield-ebpf \
    --pin-maps \
    --xdp-skb-mode \
    --obj-dir /home/govan/tmp/unheaded/ebpf/target/bpfel-unknown-none/release
```

**Critical flags:**
- `--pin-maps` — Pins all BPF maps under `/sys/fs/bpf/unheaded/shield-ebpf/`
- `--xdp-skb-mode` — Uses XDP generic (not native/offload). Required for tap/veth.
- `--only shield-ebpf` — Load only the Shield program, not all 8 eBPF programs.

**The ebpf-loader process must stay alive** — it holds the BPF link file
descriptors. Killing it detaches the programs. Run it in a tmux session or
as a systemd unit.

### Reloading (After Code Changes)

When reloading shield-ebpf after source changes, new programs get NEW map IDs.
The old programs remain attached until their loader process is killed.

```bash
# Option 1: Kill old loader, remove pins, reload fresh
sudo kill <old-loader-pid>
sudo rm -rf /sys/fs/bpf/unheaded/shield-ebpf/
# Then load again with the command above

# Option 2: Load alongside (creates duplicate maps)
# New programs get new map IDs, old ones stay on old maps
# Pin paths get updated to new maps automatically
sudo /home/govan/tmp/unheaded/cmd/ebpf-loader/target/release/ebpf-loader \
    --interface tap-tomb --only shield-ebpf --pin-maps --xdp-skb-mode \
    --obj-dir /home/govan/tmp/unheaded/ebpf/target/bpfel-unknown-none/release
```

**Current state note:** There are TWO loaders running (PIDs 262973 and 343773).
The first loaded the pre-diagnostic shield, the second loaded the version with
STAT_TC_ENTRY/IPV6/HBH counters. Only the second (343773) has the active
program IDs (2371, 2372). The first's programs (2065, 2066) are still loaded
but not attached to `tap-tomb` (XDP replaced them, TC link was recreated).

### Verify Attachment

```bash
# Show attached programs
sudo bpftool net show dev tap-tomb

# Expected:
# xdp:
# tap-tomb(234) generic id 2371
# tc:
# tap-tomb(234) tcx/egress shield_tc prog_id 2372 link_id 5

# Show program details
sudo bpftool prog show id 2371
sudo bpftool prog show id 2372
```

---

## 4. BPF Map Pin Paths

All Shield maps are pinned under `/sys/fs/bpf/unheaded/shield-ebpf/`:

| Pin Name | Map ID | Type | Size | Purpose |
|----------|--------|------|------|---------|
| STATS | 456 | hash | u32→u64 | Per-key packet/event counters |
| ANAMNESIS | 461 | ringbuf | 8 MiB | Event ring buffer (Birth/Death/Anomaly) |
| ERROR_COUNTERS | 464 | hash | u64→[u8;32] | RFC 9114 error tracking |
| AUTHORITY | 457 | hash | u64→[u8;64] | Sophia dictionary authority |
| BLOCKLIST | 460 | hash | u64→u64 | Blocked source addresses |
| GOAWAY_STATE | 455 | hash | u64→[u8;16] | Connection GOAWAY tracking |
| HMAC_KEYS | 459 | hash | u64→[u8;32] | HMAC keys for retry tokens |
| HOP_VALIDATORS | 462 | hash | u64→[u8;32] | Hop validation state |
| RATE_TOKENS | 463 | hash | u64→[u8;16] | Per-connection rate limiting |
| RETRY_TOKENS | 458 | hash | u64→[u8;32] | Retry token state |

**Verify:**
```bash
sudo bpftool map show pinned /sys/fs/bpf/unheaded/shield-ebpf/STATS
```

**Stale maps (381-392):** From the first load. Still alive because PID 262973
holds FDs. Will be garbage-collected when that process dies.

---

## 5. STATS Map Keys (Shield eBPF)

Source of truth: `ebpf/shield-ebpf/src/main.rs`

| Key | Constant | Incremented By |
|-----|----------|----------------|
| 0 | STAT_TOTAL | XDP: every packet |
| 1 | STAT_IPV6 | XDP: IPv6 packets |
| 2 | STAT_BIRTHS | XDP: BIRTH event emitted |
| 3 | STAT_DEATHS | TC: DEATH event emitted |
| 4 | STAT_BLOCKED | XDP: blocked source |
| 5 | STAT_EXT_STRIPPED | XDP: extension header stripped |
| 6 | STAT_ANOMALIES | TC: checksum/anomaly detected |
| 7 | STAT_RING_DROPS | Both: ANAMNESIS reserve() failed |
| 8 | STAT_DEST_OPTIONS_PROCESSED | XDP: destination options parsed |
| 9 | STAT_DST_OPT_SKIP | XDP: dest opt action=skip |
| 10 | STAT_DST_OPT_DISCARD | XDP: dest opt action=discard |
| 11 | STAT_DST_OPT_ICMP | XDP: dest opt action=ICMP |
| 12 | STAT_DST_OPT_ICMP_MCAST | XDP: dest opt action=ICMP (mcast) |
| 13 | STAT_TC_ENTRY | TC: every packet entering TC |
| 14 | STAT_TC_IPV6 | TC: IPv6 packets in TC |
| 15 | STAT_TC_HBH | TC: IPv6 with HBH header in TC |

**Read stats:**
```bash
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/shield-ebpf/STATS
```

---

## 6. Monad Wire Format Constants

When sending test packets, these values MUST match `ebpf/monad-common/src/lib.rs`:

| Constant | Value | Notes |
|----------|-------|-------|
| MONAD_OPT_TYPE | **0x3E** | NOT 0x1F. IPv6 HBH option type. |
| MONAD_OPT_DATA_LEN | 20 | Monad register size |
| MONAD_SIZE | 20 bytes | Full register |
| HBH_TOTAL_LEN | 24 bytes | 2 (fixed) + 2 (TLV hdr) + 20 (monad) |
| HdrExtLen field | 2 | 24/8 - 1 = 2 |
| EventType::Birth | 1 | (1-based, NOT 0-based) |
| EventType::Hop | 2 | |
| EventType::Death | 3 | |
| EventType::Anomaly | 4 | |
| EventType::Chaos | 5 | |

**Sending a valid Monad packet (Python):**
```python
import socket, struct

monad = struct.pack('>BBBB HBB I Q',
    0x01,        # version
    0x01,        # src_service_id
    0x02,        # dst_service_id
    0x28,        # flags (TRACED|SAMPLED)
    0x0001,      # flow_id
    0,           # hop_count
    64,          # ttl
    1,           # sequence
    0xDEADBEEF,  # trace_id
)

hbh = bytearray(24)
hbh[0] = 17     # Next Header = UDP
hbh[1] = 2      # Hdr Ext Len
hbh[2] = 0x3E   # MONAD_OPT_TYPE
hbh[3] = 20     # Opt Data Len
hbh[4:24] = monad

s = socket.socket(socket.AF_INET6, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
s.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_HOPOPTS, bytes(hbh))
s.bind(('fd00:13:6::1', 0, 0, 0))
s.sendto(b'PAYLOAD', ('fd00:13:6::6', 31337))
```

**Key gotcha:** AF_PACKET (scapy `sendp()`) bypasses TC/XDP hooks. You MUST
use kernel-routed sockets (UDP6 with IPV6_HOPOPTS setsockopt) for packets
to hit the BPF programs.

---

## 7. Python Virtual Environment

Scapy is installed in a project-local virtualenv (PEP 668 compliance):

```bash
python3 -m venv /home/govan/tmp/unheaded/.venv
/home/govan/tmp/unheaded/.venv/bin/pip install scapy
```

The stress-cannon.py does NOT use scapy anymore (rewritten for kernel sockets),
but it's available for packet inspection/debugging.

---

## 8. Full Reproducibility Script

To reproduce the Phase 10 environment from a clean WEST boot:

```bash
#!/bin/bash
# Phase 10 setup — run as root or with sudo

set -e

REPO=/home/govan/tmp/unheaded
IFACE=tap-tomb
BRIDGE=br-tomb

# 1. Network config
ip -6 addr add fd00:13:6::1/64 dev $BRIDGE 2>/dev/null || true
ip -6 neigh replace fd00:13:6::6 lladdr 52:54:00:ab:cd:ef dev $BRIDGE nud permanent

# 2. QDisc tuning
ip link set $IFACE txqueuelen 10000
tc qdisc replace dev $IFACE root fq limit 50000 flow_limit 10000

# 3. Build eBPF
cd $REPO/ebpf
cargo +nightly build -Z build-std=core --target bpfel-unknown-none --release -p shield-ebpf

# 4. Build ebpf-loader
cd $REPO/cmd/ebpf-loader
cargo build --release

# 5. Load Shield eBPF (stays in foreground, holding link FDs)
$REPO/cmd/ebpf-loader/target/release/ebpf-loader \
    --interface $IFACE \
    --only shield-ebpf \
    --pin-maps \
    --xdp-skb-mode \
    --obj-dir $REPO/ebpf/target/bpfel-unknown-none/release

# 6. Verify (in another terminal)
# bpftool net show dev tap-tomb
# bpftool map dump pinned /sys/fs/bpf/unheaded/shield-ebpf/STATS
# python3 scripts/stress-cannon.py --rate 1000
```

---

## 9. Known Issues / Caveats

1. **Two ebpf-loader instances** are currently running (PIDs 262973, 343773).
   Only the second owns the active programs. Kill 262973 to free stale maps
   (381-392).

2. **ANAMNESIS ring buffer is 8 MiB.** At 920K pps with 2 events/packet,
   it fills in ~0.11 seconds without a consumer. Run `ringbuf-drain.py` or
   the trace-collector during stress tests.

3. **tap-tomb TX drops** still occur (~50K/s at wire speed) even with
   flow_limit=10000. This is the tap device driver's FD buffer to QEMU,
   not the qdisc. Irreducible unless QEMU's vhost-net sndbuf is increased.

4. **Checksum anomalies**: Every Monad packet from the stress cannon triggers
   STAT_ANOMALIES because we don't compute a valid Monad checksum. This is
   harmless for throughput testing but means ANAMNESIS gets 2 events/packet
   (DEATH + ANOMALY) instead of 1.

5. **Diagnostic stats (keys 13-15)**: STAT_TC_ENTRY, STAT_TC_IPV6, STAT_TC_HBH
   were added during this session for debugging. They add 3 hash map lookups
   per TC invocation. Consider removing them for production benchmarks.

---

**Created:** 2026-02-28
**Session:** Phase 10 stress testing on WEST bare metal
**Kernel:** 6.17.0-14-generic
