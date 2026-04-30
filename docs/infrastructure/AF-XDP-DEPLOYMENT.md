# AF_XDP Deployment Guide

## Prerequisites

- Linux kernel 5.8+ with AF_XDP support
- NIC driver with XDP support (check: `ethtool -S eth0 | grep xdp`)
- Unheaded binary and libaf_xdp.so on system
- Rust toolchain (for building libaf_xdp)
- Go 1.21+ with CGo enabled

## Architecture

```
                    +------------------+
                    |  Go Service      |
                    |  (trace-collector|
                    |   or gateway)    |
                    +--------+---------+
                             |
                    CGo FFI  |  pkg/afxdp/
                             |
                    +--------+---------+
                    |  libaf_xdp.so    |
                    |  (Rust library)  |
                    +--------+---------+
                             |
              raw syscalls   |  AF_XDP socket
                             |
    +--------+-------+-------+--------+---------+
    |  Fill   |  Comp  |  RX   |  TX    |  UMEM  |
    |  Ring   |  Ring  |  Ring  |  Ring  |  Pool  |
    +--------+-------+-------+--------+---------+
                             |
              XSKMAP         |  xdp_redirect.o
                             |
                    +--------+---------+
                    |  Kernel XDP      |
                    |  (NIC driver)    |
                    +------------------+
```

## Setup

### 1. Build eBPF Programs

```bash
cd ebpf/xdp-redirect
cargo build --target bpfel-unknown-none --release
# Produces: target/bpfel-unknown-none/release/xdp_redirect
```

### 2. Build Rust Library

```bash
cd ebpf/af-xdp
cargo build --lib --release
# Produces: target/release/libaf_xdp.{a,so}
```

### 3. Build Go Service

```bash
cd unheaded
go build ./pkg/afxdp/
```

### 4. Load XDP Program

```bash
# Via ip link (manual):
ip link set dev eth0 xdp obj xdp_redirect.o sec xdp

# Or via loader.go (programmatic):
# The Go loader handles program attachment automatically
```

### 5. Create AF_XDP Engine

```go
engine, err := afxdp.NewEngine("eth0", 0, 16*1024*1024)
if err != nil {
    log.Fatal(err)
}
defer engine.Close()
```

### 6. Enable Per-Queue Redirect

The xdp_redirect program defaults to DISABLED for all queues.
Userspace must write to the CONFIG map to enable redirect:

```go
// Via eBPF map update (using pkg/ebpf/loader):
// CONFIG[queue_id] = XdpRedirectConfig{enabled: 1, protocol_filter: 0}
```

## Performance Tuning

### Busy-Poll Mode (lowest latency)
```bash
# Enable kernel busy-poll on the socket:
setsockopt(fd, SOL_SOCKET, SO_BUSY_POLL, 50)  # 50us busy-wait
setsockopt(fd, SOL_SOCKET, SO_PREFER_BUSY_POLL, 1)
```

### UMEM Frame Count
- Default: 4096 frames (16MB at 4KB/frame)
- High throughput: 8192+ frames
- Memory-constrained: 2048 frames

### Batch Size
- Default: 32 packets per burst
- High throughput: 64-128
- Low latency: 1-8

## Status

- [STUCK] Full integration test blocked without AF_XDP-capable hardware
- [x] Code integration framework complete
- [x] Rust FFI layer functional
- [x] Go wrapper with CGo bindings
- [x] XDP redirect eBPF program
