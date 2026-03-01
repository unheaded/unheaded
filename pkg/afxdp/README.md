# AF_XDP Go Bridge

Wraps Rust `libaf_xdp` for high-performance zero-copy packet I/O in Go services.

## Architecture

```
Go Service
   |
   | (CGo)
   v
pkg/afxdp (Go wrapper)
   |
   | (C FFI)
   v
libaf_xdp.so (Rust library)
   |
   | (eBPF + raw syscalls)
   v
AF_XDP Socket + Kernel XDP Program
```

## Usage

```go
engine, err := afxdp.NewEngine("eth0", 0,
    afxdp.WithFrameCount(8192),
    afxdp.WithBatchSize(64),
)
if err != nil {
    log.Fatal(err)
}
defer engine.Close()

packets, err := engine.Recv(ctx)
sent, err := engine.SendBatch(packets)
stats, err := engine.Stats()
```

## Build Instructions

### 1. Build Rust library

```bash
cd ebpf/af-xdp
cargo build --lib --release
# Produces: target/release/libaf_xdp.{a,so}
```

### 2. Build Go package

```bash
cd unheaded
go build ./pkg/afxdp/
```

### 3. Run tests (requires AF_XDP NIC)

```bash
go test ./pkg/afxdp/
```

## Status

- [x] Rust FFI bindings (ffi.rs)
- [x] C header (af_xdp.h)
- [x] Go wrapper API (afxdp.go)
- [x] Options pattern for configuration
- [x] Error codes and safety documentation
- [ ] Integration test (requires AF_XDP-capable NIC)
- [ ] Performance benchmark

## Troubleshooting

### "AF_XDP requires Linux NIC support"
- Verify kernel version: `uname -r` (need 5.8+)
- Check NIC support: `ethtool -S eth0 | grep xdp`
- Some veth/vlan interfaces don't support AF_XDP; use physical NIC

### "afxdp: create failed"
- Check interface exists: `ip link show eth0`
- Verify permissions: may need `CAP_SYS_ADMIN` or root
- Confirm XSKMAP size not exceeded (max 64 queues)

### Build errors with libaf_xdp
- Ensure Rust library built in release mode: `cargo build --lib --release`
- Check CGo LDFLAGS point to correct path
- Verify symbols: `nm -D target/release/libaf_xdp.so | grep afxdp_`

### "epoll_create1 failed"
- Ensure running on Linux (AF_XDP is Linux-only)
- Check file descriptor limits: `ulimit -n`
