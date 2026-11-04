# AF_XDP Testing Guide -- Unheaded Kingdom

**License:** GPL-2.0-only
**Updated:** 2026-03-01

---

## Test Organization

AF_XDP tests are organized by module within each crate.  Tests that
require AF_XDP hardware, root privileges, or a real network device
are marked `#[ignore]` so they do not run in normal CI.

### Crate Test Locations

| Crate             | Test File                              | Tests | Root Required |
|-------------------|----------------------------------------|-------|---------------|
| `af-xdp-common`  | `ebpf/af-xdp-common/src/lib.rs`       | 25+   | No            |
| `af-xdp` (syscall)| `ebpf/af-xdp/src/syscall.rs`          | 5     | Partial       |
| `af-xdp` (umem)  | `ebpf/af-xdp/src/umem.rs`             | 12    | No            |
| `af-xdp` (ring)  | `ebpf/af-xdp/src/ring.rs`             | 8     | No            |
| `af-xdp` (xsk)   | `ebpf/af-xdp/src/xsk.rs`              | 3     | Partial       |

### Test Categories

**Unit tests (no root):**
- Struct layout and size assertions (af-xdp-common)
- Constant value verification (AF_XDP=44, SOL_XDP=283, etc.)
- UMEM creation, allocation, deallocation, reuse
- UMEM configuration validation (edge cases)
- Ring buffer push/pop, wrap-around, batch operations
- Ring failure counter tracking
- Memory barrier correctness (compile-time only)
- Anonymous mmap round-trip
- Interface name resolution (loopback)

**Integration tests (require root / CAP_NET_ADMIN):**
- AF_XDP socket creation (`test_create_xdp_socket_no_cap` -- runs either way)
- Full socket lifecycle on loopback (`test_full_socket_creation` -- `#[ignore]`)
- Loopback recv/send test (`test_recv_send_loopback` -- `#[ignore]`)

**eBPF tests (require root + BPF + XDP-capable NIC):**
- xdp-redirect program loading and attachment
- Shield-ebpf BIRTH stamping + AF_XDP redirect
- Packet-marker trace ID extraction + selective redirect
- End-to-end: packet ingress -> XDP -> AF_XDP -> userspace

---

## How to Run

### All Unit Tests (No Privileges Required)

```bash
cd ebpf/af-xdp
cargo test
```

This runs all non-ignored tests across the `af-xdp` and `af-xdp-common`
crates.  Expected output:

```
running 25 tests
test af_xdp_common::tests::test_xsk_desc_layout ... ok
test af_xdp_common::tests::test_xsk_config_layout ... ok
...
test ring::tests::test_push_pop_single ... ok
test ring::tests::test_wrap_around ... ok
test umem::tests::test_umem_creation ... ok
test umem::tests::test_umem_frame_allocation ... ok
...
test result: ok. 25 passed; 0 failed; 0 ignored
```

### af-xdp-common Only

```bash
cd ebpf/af-xdp-common
cargo test
```

Or from the workspace root:

```bash
cd ebpf
cargo test -p af-xdp-common
```

### Including Ignored Tests (Requires Root)

```bash
cd ebpf/af-xdp
sudo cargo test -- --ignored
```

Or to run all tests (both normal and ignored):

```bash
cd ebpf/af-xdp
sudo cargo test -- --include-ignored
```

### Individual Test

```bash
cd ebpf/af-xdp
cargo test test_umem_frame_allocation
```

### With Output (for debugging)

```bash
cd ebpf/af-xdp
cargo test -- --nocapture
```

---

## Ignored Tests (Require Root / Hardware)

The following tests are marked `#[ignore]` and require CAP_NET_ADMIN or
root privileges and a real or virtual network device:

### `test_full_socket_creation` (xsk.rs)

**Requires:** Root (or CAP_NET_ADMIN), loopback interface

Creates a complete XskSocket on the loopback device:
1. Allocates UMEM (4096 frames, 4096 bytes each = 16 MiB)
2. Creates AF_XDP socket
3. Registers UMEM
4. Sets up all four rings
5. Binds to `lo` queue 0

```bash
sudo cargo test test_full_socket_creation -- --ignored --nocapture
```

### `test_recv_send_loopback` (xsk.rs)

**Requires:** Root, loopback interface, XDP program attached

Creates a socket on loopback, attempts to receive and send packets,
and runs a complete_cycle.

```bash
sudo cargo test test_recv_send_loopback -- --ignored --nocapture
```

### Partially Root-Required Tests

**`test_create_xdp_socket_no_cap`** (xsk.rs) -- Runs without root but
expects either success or a permission error.  Not ignored.

**`test_if_nametoindex_loopback`** (syscall.rs) -- Tests `if_nametoindex("lo")`
via ioctl.  Usually works without root but requires a socket.

**`test_mmap_anonymous`** (syscall.rs) -- Anonymous mmap works without root.

---

## CI/CD Integration

### GitHub Actions Configuration

Tests that do not require root run in standard CI.  Ignored tests
require a privileged runner.

```yaml
# .github/workflows/af-xdp-tests.yml
name: AF_XDP Tests

on:
  push:
    paths:
      - 'ebpf/af-xdp/**'
      - 'ebpf/af-xdp-common/**'

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install Rust toolchain
        uses: dtolnay/rust-toolchain@nightly
        with:
          targets: x86_64-unknown-linux-gnu
      - name: Run unit tests
        run: |
          cd ebpf/af-xdp
          cargo test

  integration-tests:
    runs-on: ubuntu-latest
    # Requires privileged container for AF_XDP socket creation
    container:
      image: ubuntu:24.04
      options: --privileged
    steps:
      - uses: actions/checkout@v4
      - name: Install dependencies
        run: |
          apt-get update
          apt-get install -y curl build-essential
          curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
          . ~/.cargo/env
          rustup default nightly
      - name: Run all tests including ignored
        run: |
          . ~/.cargo/env
          cd ebpf/af-xdp
          cargo test -- --include-ignored
```

### Test Gate Criteria

| Gate           | Threshold | Tests                                  |
|----------------|-----------|----------------------------------------|
| PR merge       | All pass  | Unit tests (non-ignored)               |
| Nightly build  | All pass  | Unit + integration (with --include-ignored) |
| Release        | All pass  | Full suite + manual hardware validation |

### eBPF Program Testing

eBPF programs (`xdp-redirect`, `shield-ebpf`, `packet-marker`) are
compiled with `cargo build --target bpfel-unknown-none` but cannot be
unit-tested in CI without a BPF-capable kernel.  Their correctness is
verified by:

1. **Compile-time:** Type layout assertions (const_assert_size!)
2. **Build verification:** `cargo build` succeeds for bpfel target
3. **Integration:** Ignored tests that load and run programs with root
4. **Manual:** WEST bare metal cluster validation

---

## Test Coverage Summary

### af-xdp-common

| Area                | Tests | Coverage |
|---------------------|-------|----------|
| XskDesc layout      | 3     | 100%     |
| XskConfig           | 2     | 100%     |
| XskUmemReg          | 1     | 100%     |
| XskRingOffsets      | 1     | 100%     |
| XskMmapOffsets      | 1     | 100%     |
| XskStatistics       | 2     | 100%     |
| Sockaddr_xdp        | 2     | 100%     |
| FillDesc/CompDesc   | 2     | 100%     |
| Constants           | 6     | 100%     |
| XdpRedirectConfig   | 2     | 100%     |
| XdpRedirectStats    | 2     | 100%     |

### af-xdp (userspace engine)

| Module    | Tests | Coverage Notes                                    |
|-----------|-------|---------------------------------------------------|
| syscall   | 5     | mmap, if_nametoindex, socket error, PollFd size   |
| umem      | 12    | Create, alloc, free, reuse, validation, drop      |
| ring      | 8     | Push/pop, fill, wrap, batch, capacity, stats      |
| xsk       | 3     | Socket create (2 ignored), ring size validation   |
| engine    | 0*    | Tested via xsk integration tests (requires root)  |
| ffi       | 0*    | Tested via Go bridge integration                  |

*Engine and FFI are exercised through the ignored integration tests and
the Go bridge, not via standalone unit tests.

---

## Related Documents

- [AF_XDP_ARCHITECTURE.md](architecture/AF_XDP_ARCHITECTURE.md) -- Architecture
- [DEPLOYMENT_GUIDE_AFXDP.md](DEPLOYMENT_GUIDE_AFXDP.md) -- Deployment
- [CHANGELOG_AFXDP.md](CHANGELOG_AFXDP.md) -- Changelog
