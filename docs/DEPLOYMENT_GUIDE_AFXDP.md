# AF_XDP Deployment Guide -- Unheaded Kingdom

**Status:** Phase 11 (Documentation and Final Integration)
**License:** GPL-2.0-only
**Updated:** 2026-03-01

---

## Prerequisites

### Kernel Configuration

The following kernel options must be enabled.  Check with
`zcat /proc/config.gz | grep CONFIG_XDP` or inspect
`/boot/config-$(uname -r)`.

| Option                    | Required | Notes                             |
|---------------------------|----------|-----------------------------------|
| `CONFIG_BPF=y`            | Yes      | BPF subsystem                     |
| `CONFIG_BPF_SYSCALL=y`    | Yes      | bpf() syscall                     |
| `CONFIG_XDP_SOCKETS=y`    | Yes      | AF_XDP socket family              |
| `CONFIG_BPF_JIT=y`        | Yes      | JIT compiler for BPF programs     |
| `CONFIG_HAVE_EBPF_JIT=y`  | Yes      | Architecture supports eBPF JIT    |
| `CONFIG_NET_XDP=y`        | Yes      | XDP framework                     |
| `CONFIG_BPF_EVENTS=y`     | No       | For ring buffer events            |

**Minimum kernel:** 5.15+ (required for XskMap redirect improvements).
**Recommended:** 6.1 LTS or later.
**WEST cluster:** Runs kernel 6.17.

Verify kernel version:

```bash
uname -r
# Expected: 5.15.x or later
```

### Hugepage Setup (Optional, Recommended)

Hugepages reduce TLB misses for the UMEM region.  Not strictly required
but improves performance under high packet rates.

```bash
# Allocate 64 hugepages (64 * 2MiB = 128 MiB)
echo 64 > /proc/sys/vm/nr_hugepages

# Verify allocation
cat /proc/meminfo | grep HugePages
# HugePages_Total:      64
# HugePages_Free:       64

# For persistence across reboots, add to /etc/sysctl.conf:
echo "vm.nr_hugepages = 64" >> /etc/sysctl.conf
```

### Memory Lock Limit

AF_XDP requires mmap'd memory that cannot be swapped.  The default
locked memory limit (64 KiB) is insufficient.

```bash
# Set unlimited locked memory for current session
ulimit -l unlimited

# For persistence, add to /etc/security/limits.conf:
# *    soft   memlock   unlimited
# *    hard   memlock   unlimited
#
# Or for a specific user:
# unheaded    soft   memlock   unlimited
# unheaded    hard   memlock   unlimited
```

### BPF Permissions

AF_XDP socket creation and XDP program loading require elevated
privileges:

**Option A: Run as root** (simplest, suitable for development)

```bash
sudo ./target/release/af-xdp-example
```

**Option B: Linux capabilities** (production, principle of least privilege)

```bash
# Grant required capabilities to the binary
sudo setcap 'cap_bpf,cap_net_admin,cap_sys_resource+ep' ./target/release/af-xdp-example

# Required capabilities:
#   CAP_BPF        - Load BPF programs and create BPF maps
#   CAP_NET_ADMIN  - Create AF_XDP sockets, attach XDP programs
#   CAP_SYS_RESOURCE - Override RLIMIT_MEMLOCK for UMEM mmap
```

**Option C: BPF filesystem delegation** (advanced, for container environments)

```bash
# Mount BPF filesystem if not already mounted
sudo mount -t bpf bpf /sys/fs/bpf

# Pin maps to BPF filesystem for sharing between processes
# (done automatically by aya-based loaders)
```

---

## XDP Program Loading Sequence

The loading sequence must be followed in order.  Each step depends on
the previous one.

### Step 1: Load the XDP Program to the Interface

```bash
# Using bpftool (for xdp-redirect):
sudo bpftool prog load xdp_redirect.o /sys/fs/bpf/unheaded/xdp_redirect \
    type xdp

# Attach to interface:
sudo bpftool net attach xdp \
    pinned /sys/fs/bpf/unheaded/xdp_redirect \
    dev eth0

# Verify attachment:
sudo bpftool net list
# eth0:
#   xdp: prog_id 42
```

For shield-ebpf (which has both XDP and TC programs):

```bash
# Load and attach XDP ingress program:
sudo bpftool prog load shield.o /sys/fs/bpf/unheaded/shield_xdp \
    type xdp

sudo bpftool net attach xdp \
    pinned /sys/fs/bpf/unheaded/shield_xdp \
    dev eth0

# Load and attach TC egress program:
sudo tc qdisc add dev eth0 clsact
sudo tc filter add dev eth0 egress bpf \
    obj shield.o section classifier/shield_tc \
    direct-action
```

### Step 2: Create the AF_XDP Socket

```rust
// In Rust (af-xdp crate):
let fd = socket(AF_XDP as i32, SOCK_RAW, 0)?;
```

### Step 3: Register UMEM

```rust
// Allocate UMEM memory
let umem_ptr = mmap(NULL, total_size, PROT_READ|PROT_WRITE,
                    MAP_SHARED|MAP_ANONYMOUS|MAP_POPULATE, -1, 0)?;

// Register with kernel
let reg = XskUmemReg {
    addr: umem_ptr as u64,
    len: total_size,
    chunk_size: 4096,
    headroom: 0,
    flags: 0,
};
setsockopt(fd, SOL_XDP, XDP_UMEM_REG, &reg, size_of::<XskUmemReg>())?;
```

### Step 4: Set Ring Sizes and Bind to Queue

```rust
// Set ring sizes (all must be power of 2)
let ring_size: u32 = 2048;
setsockopt(fd, SOL_XDP, XDP_UMEM_FILL_RING, &ring_size, 4)?;
setsockopt(fd, SOL_XDP, XDP_UMEM_COMPLETION_RING, &ring_size, 4)?;
setsockopt(fd, SOL_XDP, XDP_RX_RING, &ring_size, 4)?;
setsockopt(fd, SOL_XDP, XDP_TX_RING, &ring_size, 4)?;

// Query mmap offsets
let offsets: XskMmapOffsets = getsockopt(fd, SOL_XDP, XDP_MMAP_OFFSETS)?;

// Mmap all four rings using the offsets
// ... (see xsk.rs for full implementation)

// Bind to interface + queue
let saddr = Sockaddr_xdp {
    sxdp_family: AF_XDP,  // 44
    sxdp_flags: 0,
    sxdp_ifindex: if_nametoindex("eth0")?,
    sxdp_queue_id: 0,
    sxdp_shared_umem_fd: 0,
};
bind(fd, &saddr, size_of::<Sockaddr_xdp>())?;
```

### Step 5: Insert Socket FD into XSKMAP

```bash
# Using bpftool:
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/xdp_redirect/XSKS \
    key 0 0 0 0 \
    value $(printf '%08x' $SOCKET_FD | sed 's/../\\x&/g')
```

Or programmatically from Rust/Go via the bpf() syscall.

### Step 6: Enable AF_XDP in Configuration Map

For shield-ebpf:

```bash
# Set SHIELD_CONFIG[0] = 1 (enable AF_XDP redirect)
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG \
    key 0 0 0 0 \
    value 1 0 0 0
```

For packet-marker:

```bash
# Set MARKER_CONFIG[0] = 1 (enable AF_XDP redirect for traced packets)
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/packet_marker/MARKER_CONFIG \
    key 0 0 0 0 \
    value 1 0 0 0
```

For xdp-redirect:

```bash
# Enable redirect on queue 0 (all protocols)
sudo bpftool map update \
    pinned /sys/fs/bpf/unheaded/xdp_redirect/CONFIG \
    key 0 0 0 0 \
    value 1 0 0 0
```

---

## Troubleshooting

### Socket Creation Fails (EACCES / EPERM)

```
Error: socket(AF_XDP) creation failed
```

**Cause:** Insufficient permissions to create AF_XDP sockets.

**Fix:**
```bash
# Run as root
sudo ./your-binary

# Or grant capabilities
sudo setcap 'cap_bpf,cap_net_admin,cap_sys_resource+ep' ./your-binary
```

### UMEM Registration Fails (ENOMEM)

```
Error: setsockopt XDP_UMEM_REG failed
```

**Cause:** Locked memory limit too low, or system out of memory.

**Fix:**
```bash
# Increase locked memory limit
ulimit -l unlimited

# Check available memory
free -h

# Reduce UMEM size if needed (fewer frames)
```

### Bind Fails (ENODEV / EINVAL)

```
Error: bind(AF_XDP) failed
```

**Cause:** Interface does not exist, or XDP program not loaded, or
queue ID exceeds NIC queue count.

**Fix:**
```bash
# Verify interface exists
ip link show eth0

# Check NIC queue count
ethtool -l eth0

# Verify XDP program is attached
sudo bpftool net list

# Use queue 0 if unsure
```

### No Packets Received

**Possible causes:**
1. XDP program not attached to the interface
2. AF_XDP not enabled in config map (bit 0 = 0)
3. Socket FD not inserted into XSKMAP
4. Fill ring not populated (no frames for kernel to use)
5. Wrong queue ID (packets arrive on queue N, socket bound to queue 0)

**Diagnostic steps:**
```bash
# 1. Verify XDP program is attached
sudo bpftool net list

# 2. Check config map
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG

# 3. Check XSKMAP has entries
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/shield/SHIELD_XSKS

# 4. Check stats for redirect attempts vs successes
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/shield/STATS

# 5. Check NIC stats for queue distribution
ethtool -S eth0 | grep rx_queue
```

### High Drop Rate

**Check socket statistics:**
```bash
# From Rust: xsk_socket.statistics()
# Shows: rx_dropped, rx_ring_full, rx_fill_ring_empty_descs
```

**Common causes:**
- `rx_ring_full`: RX ring not drained fast enough. Increase ring size or
  process packets faster.
- `rx_fill_ring_empty_descs`: Fill ring starved. Free received frames
  back to fill ring promptly.
- `rx_dropped`: Generic drop counter.

**Fixes:**
- Increase ring sizes (default 2048, try 4096 or 8192)
- Increase frame count (default 4096)
- Use batch processing (rx_burst with batch_size 64+)
- Ensure `complete_cycle()` is called in every event loop iteration

### XDP Program Verification Fails

```
Error: BPF program rejected by verifier
```

**Common causes:**
- Unbounded loops (verifier requires bounded iteration)
- Missing packet bounds checks after `bpf_xdp_adjust_head`
- Stack size exceeded (512 bytes max for BPF)
- Instruction count exceeded (1M on kernel 6.17)

**Shield-specific:** The `MAX_EXT_HDRS_TO_STRIP` is set to 2 to stay
within the verifier instruction limit.  This covers the common case
of Routing + Fragment headers from Shadow traffic.

---

## Example: Minimal RX Application

A complete example of receiving packets via AF_XDP in approximately
50 lines of Rust pseudocode:

```rust
use af_xdp::common::{XskConfig, DEFAULT_FRAME_SIZE, DEFAULT_FRAME_COUNT};
use af_xdp::{XdpEngine, SignalHandler};

fn main() -> Result<(), &'static str> {
    // 1. Create signal handler for graceful shutdown
    let signal = SignalHandler::new();

    // 2. Create XDP engine on eth0, queue 0, default frames
    //    This internally:
    //    - Allocates UMEM (frame_count * frame_size bytes)
    //    - Creates AF_XDP socket
    //    - Registers UMEM with socket
    //    - Sets up all four rings (fill, completion, RX, TX)
    //    - Binds to interface and queue
    //    - Pre-fills the fill ring with empty frames
    let mut engine = XdpEngine::new("eth0", 0, DEFAULT_FRAME_COUNT)?;

    // 3. Main event loop
    //    NOTE: An XDP redirect program (xdp-redirect, shield-ebpf, or
    //    packet-marker) must be loaded and configured to redirect
    //    packets to this socket's XSKMAP entry.
    println!("Listening for AF_XDP packets on eth0 queue 0...");

    while !signal.should_exit() {
        // 4. Poll for incoming packets (100ms timeout)
        let ready = engine.poll(100)?;
        if !ready {
            continue;  // Timeout, check signal and loop
        }

        // 5. Receive burst of packets
        let packets = engine.rx_burst(64);
        if packets.is_empty() {
            continue;
        }

        // 6. Process each packet
        for pkt in &packets {
            // pkt.addr = UMEM offset where packet data starts
            // pkt.len  = packet length in bytes
            let data_ptr = engine.frame_ptr(pkt.addr);

            // Read packet data (zero-copy from UMEM)
            let data = unsafe {
                std::slice::from_raw_parts(data_ptr, pkt.len as usize)
            };

            println!("Received {} bytes: {:02x?}", data.len(), &data[..14.min(data.len())]);

            // 7. Return frame to free pool
            //    (will be recycled to fill ring on next rx_burst)
            engine.free_frame(pkt.addr);
        }

        // 8. Print stats periodically
        let stats = engine.stats();
        println!(
            "Stats: rx={} tx={} drops={}",
            stats.rx_packets, stats.tx_packets, stats.rx_drops
        );
    }

    // 9. Graceful shutdown
    engine.shutdown()?;
    println!("Shutdown complete.");
    Ok(())
}
```

**Build and run:**

```bash
cd ebpf/af-xdp
cargo build --release

# Load XDP redirect program first (see Step 1 above)
# Then run with root privileges:
sudo ./target/release/af-xdp-example
```

---

## Production Deployment Checklist

- [ ] Kernel 5.15+ with `CONFIG_XDP_SOCKETS=y`
- [ ] `ulimit -l unlimited` configured
- [ ] Hugepages allocated (optional)
- [ ] XDP program built and loaded
- [ ] XSKMAP populated with socket FDs
- [ ] Configuration map set (AF_XDP enabled)
- [ ] NIC queues configured (ethtool -L)
- [ ] IRQ affinity set for NIC queues (optional, for NUMA)
- [ ] Monitoring: check STATS map and socket statistics
- [ ] Graceful shutdown tested (SIGTERM handling)

---

## Related Documents

- [AF_XDP_ARCHITECTURE.md](architecture/AF_XDP_ARCHITECTURE.md) -- Architecture overview
- [TESTING_AFXDP.md](TESTING_AFXDP.md) -- Test organization
- [MIGRATION_AFXDP.md](MIGRATION_AFXDP.md) -- Migration guide
