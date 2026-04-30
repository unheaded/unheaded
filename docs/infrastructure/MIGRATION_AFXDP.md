# AF_XDP Migration Guide -- Unheaded Kingdom

**License:** GPL-2.0-only
**Updated:** 2026-03-01
**Audience:** Existing shield-ebpf and packet-marker users

---

## Overview

This guide covers how to enable AF_XDP zero-copy packet delivery for
existing deployments that already use `shield-ebpf` (Kingdom boundary
enforcement) or `packet-marker` (distributed tracing).

**Key principle:** AF_XDP is opt-in.  Existing deployments continue to
work without any changes.  Enabling AF_XDP is a runtime toggle that does
not require recompiling or reloading eBPF programs.

---

## Prerequisites

Before enabling AF_XDP, ensure your system meets the requirements:

- Linux kernel 5.15+ with `CONFIG_XDP_SOCKETS=y`
- `ulimit -l unlimited` (or sufficient RLIMIT_MEMLOCK)
- CAP_BPF + CAP_NET_ADMIN (or root) for creating AF_XDP sockets
- An AF_XDP userspace application ready to receive packets

See [DEPLOYMENT_GUIDE_AFXDP.md](DEPLOYMENT_GUIDE_AFXDP.md) for full details.

---

## For Shield-eBPF Users

### Current Behavior (AF_XDP Disabled)

Without AF_XDP, shield-ebpf follows this path for every ingress IPv6 packet:

```
Ingress -> shield_xdp() -> BIRTH stamp -> XDP_PASS -> Kernel stack
```

All packets are processed by the kernel networking stack after BIRTH stamping.
This is the default behavior and requires no configuration.

### How to Enable AF_XDP

AF_XDP is controlled by bit 0 of `SHIELD_CONFIG[0]`:

```
SHIELD_CONFIG map:
  Key: 0 (u32)
  Value: u32 bitfield
    Bit 0: AF_XDP redirect enable (1 = on, 0 = off)
    Bits 1-31: Reserved (must be 0)
```

**Enable AF_XDP redirect:**

```bash
# Using bpftool:
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG \
    key 0 0 0 0 \
    value 1 0 0 0
```

**Disable AF_XDP redirect (revert to kernel stack):**

```bash
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG \
    key 0 0 0 0 \
    value 0 0 0 0
```

### Expected Behavior with AF_XDP Enabled

```
Ingress -> shield_xdp() -> BIRTH stamp -> AF_XDP decision:
  |
  +-- SHIELD_CONFIG[0] bit 0 == 1 AND SHIELD_XSKS[queue_id] exists:
  |     -> Redirect to AF_XDP socket (zero-copy to userspace)
  |     -> BIRTH event: redirect_action = AF_XDP (1)
  |     -> STAT_REDIRECT_SUCCESS incremented
  |
  +-- SHIELD_CONFIG[0] bit 0 == 1 AND no socket bound to this queue:
  |     -> Fall through to XDP_PASS (kernel stack)
  |     -> BIRTH event: redirect_action = KERNEL_STACK (2)
  |
  +-- SHIELD_CONFIG[0] bit 0 == 0:
        -> XDP_PASS (kernel stack, same as before)
        -> BIRTH event: redirect_action = NO_REDIRECT (0)
```

### Fallback Guarantee

**If no AF_XDP socket is bound to the receive queue, packets are NOT
dropped.** They fall through to `XDP_PASS` and are processed by the
kernel networking stack exactly as before.

This means you can safely enable AF_XDP in SHIELD_CONFIG before
creating AF_XDP sockets.  Packets will flow through the kernel stack
until a socket is bound.

### Complete Migration Steps for Shield

1. **Verify shield-ebpf is loaded and working:**
   ```bash
   sudo bpftool net list
   # Should show shield_xdp attached to your interface

   sudo bpftool map dump pinned /sys/fs/bpf/unheaded/shield/STATS
   # Should show STAT_BIRTHS > 0
   ```

2. **Create and bind an AF_XDP socket:**
   ```rust
   // Using the af-xdp crate:
   let mut engine = XdpEngine::new("eth0", 0, 4096)?;
   ```

3. **Insert socket FD into SHIELD_XSKS:**
   ```bash
   sudo bpftool map update \
       pinned /sys/fs/bpf/unheaded/shield/SHIELD_XSKS \
       key 0 0 0 0 \
       value <socket_fd_bytes>
   ```

4. **Enable AF_XDP in SHIELD_CONFIG:**
   ```bash
   sudo bpftool map update \
       pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG \
       key 0 0 0 0 \
       value 1 0 0 0
   ```

5. **Verify packets are being redirected:**
   ```bash
   sudo bpftool map dump pinned /sys/fs/bpf/unheaded/shield/STATS
   # Check STAT_REDIRECT_ATTEMPTS (key 16) and STAT_REDIRECT_SUCCESS (key 17)
   ```

6. **To revert, disable AF_XDP:**
   ```bash
   sudo bpftool map update \
       pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG \
       key 0 0 0 0 \
       value 0 0 0 0
   ```

---

## For Packet-Marker Users

### Current Behavior (AF_XDP Disabled)

Without AF_XDP, packet-marker follows this path:

```
Ingress -> packet_marker() -> Extract/inject trace ID -> XDP_PASS -> Kernel stack
```

All packets (traced and untraced) are passed to the kernel stack.

### How to Enable AF_XDP

AF_XDP is controlled by bit 0 of `MARKER_CONFIG[0]`:

```
MARKER_CONFIG map:
  Key: 0 (u32)
  Value: u32 bitfield
    Bit 0: AF_XDP redirect enable for traced packets (1 = on, 0 = off)
    Bits 1-31: Reserved (must be 0)
```

**Enable AF_XDP redirect for traced packets:**

```bash
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/packet_marker/MARKER_CONFIG \
    key 0 0 0 0 \
    value 1 0 0 0
```

### Expected Behavior with AF_XDP Enabled

Packet-marker uses **selective redirect**: only packets with a non-zero
trace ID are eligible for AF_XDP redirect.  Unmarked packets always
pass to the kernel stack.

```
Ingress -> packet_marker() -> trace ID check:
  |
  +-- trace_id != 0 AND MARKER_CONFIG[0] bit 0 == 1:
  |     |
  |     +-- MARKER_XSKS[queue_id] exists:
  |     |     -> Redirect to AF_XDP socket
  |     |     -> Event: PacketAction::Redirect
  |     |     -> STAT_AFXDP_REDIRECT incremented
  |     |
  |     +-- No socket bound to this queue:
  |           -> Fall through to XDP_PASS
  |           -> STAT_AFXDP_FALLBACK incremented
  |
  +-- trace_id == 0 OR MARKER_CONFIG[0] bit 0 == 0:
        -> XDP_PASS (kernel stack)
        -> Event: PacketAction::Pass
```

### Selective Redirect Rationale

Packet-marker intentionally only redirects traced packets because:

1. **Minimal disruption:** Untraced traffic (the majority) continues
   to use the standard kernel path.
2. **Targeted delivery:** Only packets with active trace IDs need
   zero-copy delivery to the trace-collector.
3. **Bandwidth conservation:** AF_XDP sockets see only relevant packets,
   reducing userspace processing load.

### Complete Migration Steps for Packet-Marker

1. **Verify packet-marker is loaded and working:**
   ```bash
   sudo bpftool map dump pinned /sys/fs/bpf/unheaded/packet_marker/STATS
   # Check STAT_PACKETS_TOTAL (key 0) > 0
   ```

2. **Create and bind an AF_XDP socket for the trace-collector:**
   ```rust
   let mut engine = XdpEngine::new("eth0", 0, 4096)?;
   ```

3. **Insert socket FD into MARKER_XSKS:**
   ```bash
   sudo bpftool map update \
       pinned /sys/fs/bpf/unheaded/packet_marker/MARKER_XSKS \
       key 0 0 0 0 \
       value <socket_fd_bytes>
   ```

4. **Enable AF_XDP in MARKER_CONFIG:**
   ```bash
   sudo bpftool map update \
       pinned /sys/fs/bpf/unheaded/packet_marker/MARKER_CONFIG \
       key 0 0 0 0 \
       value 1 0 0 0
   ```

5. **Inject a trace ID to see redirected traffic:**
   ```bash
   # Insert a trace ID into the TRACE_INJECT map for a specific flow
   # Packets matching that flow with a trace ID will redirect to AF_XDP
   ```

6. **Monitor redirect stats:**
   ```bash
   sudo bpftool map dump pinned /sys/fs/bpf/unheaded/packet_marker/STATS
   # Key 8 = STAT_AFXDP_REDIRECT
   # Key 9 = STAT_AFXDP_FALLBACK
   ```

---

## Dual Program Deployment

Shield-ebpf and packet-marker can run simultaneously on different
interfaces or in series (shield on the gateway interface, packet-marker
on internal interfaces).  Each has its own independent XSKMAP and
CONFIG map.

```
External interface (eth0):
  shield-ebpf -> SHIELD_XSKS, SHIELD_CONFIG

Internal interface (lxdbr0):
  packet-marker -> MARKER_XSKS, MARKER_CONFIG
```

AF_XDP sockets are per-interface, per-queue.  You need separate sockets
for each (interface, queue_id) combination.

---

## Monitoring After Migration

### Statistics to Watch

| Map Path                         | Key | Stat Name              | Healthy Value |
|----------------------------------|-----|------------------------|---------------|
| shield/STATS                     | 16  | REDIRECT_ATTEMPTS      | Increasing    |
| shield/STATS                     | 17  | REDIRECT_SUCCESS       | == ATTEMPTS   |
| packet_marker/STATS              | 8   | AFXDP_REDIRECT         | Increasing    |
| packet_marker/STATS              | 9   | AFXDP_FALLBACK         | 0 (ideal)     |

### Socket-Level Statistics

Query from the AF_XDP socket via `getsockopt(XDP_STATISTICS)`:

| Counter                 | Meaning                              | Action if High   |
|-------------------------|--------------------------------------|------------------|
| `rx_dropped`            | Frames dropped (ring full)           | Increase ring size |
| `rx_ring_full`          | RX ring overflow count               | Process faster     |
| `rx_fill_ring_empty`    | Fill ring starvation                 | Free frames faster |
| `rx_invalid_descs`      | Invalid RX descriptors               | Check UMEM config  |
| `tx_invalid_descs`      | Invalid TX descriptors               | Check desc fields  |

### Reverting AF_XDP

To revert to the pre-AF_XDP kernel-stack path at any time:

```bash
# Disable shield AF_XDP
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG \
    key 0 0 0 0 \
    value 0 0 0 0

# Disable packet-marker AF_XDP
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/packet_marker/MARKER_CONFIG \
    key 0 0 0 0 \
    value 0 0 0 0
```

No eBPF program reload is needed.  Packets immediately revert to
`XDP_PASS` on the next packet.

---

## Compatibility Notes

### No Breaking Changes

- AF_XDP maps (`SHIELD_XSKS`, `SHIELD_CONFIG`, `MARKER_XSKS`,
  `MARKER_CONFIG`) are compiled into the eBPF programs but have no
  effect when not populated.
- Default config value is 0 (AF_XDP disabled).
- Empty XSKMAP entries cause redirect to fail gracefully, falling
  through to `XDP_PASS`.

### Version Compatibility

| eBPF Program Version | AF_XDP Support | Notes                        |
|----------------------|----------------|------------------------------|
| Pre-Phase 8          | No             | No XSKMAP or CONFIG maps     |
| Phase 8+             | Shield         | SHIELD_XSKS + SHIELD_CONFIG  |
| Phase 9+             | Shield + Marker| Both programs support AF_XDP  |

### Kernel Compatibility

| Kernel  | AF_XDP Status                                      |
|---------|----------------------------------------------------|
| < 5.8   | Not supported (no AF_XDP core)                     |
| 5.8-5.14| Partial (missing XskMap improvements)               |
| 5.15+   | Full support (recommended minimum)                 |
| 6.0+    | Full support + multi-buffer XDP (optional)          |

---

## Related Documents

- [AF_XDP_ARCHITECTURE.md](architecture/AF_XDP_ARCHITECTURE.md) -- Architecture
- [DEPLOYMENT_GUIDE_AFXDP.md](DEPLOYMENT_GUIDE_AFXDP.md) -- Deployment
- [TESTING_AFXDP.md](TESTING_AFXDP.md) -- Testing
- [CHANGELOG_AFXDP.md](CHANGELOG_AFXDP.md) -- Changelog
