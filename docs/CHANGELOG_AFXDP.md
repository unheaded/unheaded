# AF_XDP Changelog -- Unheaded Kingdom

**License:** GPL-2.0-only
**Updated:** 2026-03-01

All notable changes to the AF_XDP subsystem are documented in this file.
Organized by implementation phase from the AF_XDP battle plan.

---

## Phase 0-3: AF_XDP Foundation (Types, Syscalls, UMEM, Rings)

**Crates:** `af-xdp-common`, `af-xdp`

### Added

- **af-xdp-common crate** (`ebpf/af-xdp-common/`)
  - `XskDesc` (16 bytes): RX/TX ring descriptor with addr, len, options
  - `XskConfig` (16 bytes): User-facing UMEM configuration
  - `XskUmemReg` (32 bytes): Kernel UMEM registration structure
  - `XskRingOffsets` (32 bytes): Per-ring mmap layout offsets
  - `XskMmapOffsets` (128 bytes): Complete ring offsets from getsockopt
  - `XskStatistics` (48 bytes): Socket-level statistics (6 counters)
  - `Sockaddr_xdp` (16 bytes): AF_XDP socket bind address
  - `FillDesc` (8 bytes): Fill ring frame address entry
  - `CompletionDesc` (8 bytes): Completion ring frame address entry
  - All AF_XDP constants: AF_XDP (44), SOL_XDP (283), socket options,
    bind flags, XDP actions, mmap page offsets
  - Default sizes: ring=2048, frame=4096, frame_count=4096, headroom=0
  - Compile-time size/alignment assertions via `const_assert_size!` macro
  - `no_std` compatible (`#![cfg_attr(not(test), no_std)]`)

- **af-xdp crate: syscall module** (`ebpf/af-xdp/src/syscall.rs`)
  - Raw x86_64 Linux syscall wrappers via `std::arch::asm!`
  - Zero libc dependency (Sacred Law compliance)
  - `mmap` / `munmap`: Memory mapping with full flag support
  - `socket` / `bind` / `close`: AF_XDP socket lifecycle
  - `setsockopt` / `getsockopt`: Ring configuration and UMEM registration
  - `sendto`: TX ring kick (zero-length, MSG_DONTWAIT)
  - `poll`: Readiness polling (POLLIN/POLLOUT)
  - `if_nametoindex`: Interface name to index via ioctl(SIOCGIFINDEX)
  - Memory barriers: `smp_mb()` (SeqCst), `smp_rmb()` (Acquire),
    `smp_wmb()` (Release)

- **af-xdp crate: UMEM module** (`ebpf/af-xdp/src/umem.rs`)
  - `Umem` struct: Shared memory region with LIFO free-frame allocator
  - `FillRing`: Userspace-to-kernel frame supply ring
  - `CompletionRing`: Kernel-to-userspace TX completion ring
  - UMEM configuration validation (power-of-2, min 2048, max 1M frames)
  - `register()`: setsockopt(XDP_UMEM_REG) binding
  - `query_mmap_offsets()`: getsockopt(XDP_MMAP_OFFSETS)
  - `Drop` implementation: automatic munmap on cleanup
  - O(1) frame alloc/free via Vec<u64> LIFO stack

### Tests

- 25+ unit tests for struct layout verification
- UMEM allocation/deallocation/reuse tests
- Configuration validation edge cases
- Anonymous mmap round-trip test
- Interface name resolution test (loopback)

---

## Phase 4-5: XDP Redirect Program + Lock-Free Ring Buffer

**Crates:** `xdp-redirect`, `af-xdp` (ring module), `af-xdp-common`

### Added

- **xdp-redirect eBPF program** (`ebpf/xdp-redirect/`)
  - XDP program for AF_XDP packet steering
  - BPF maps: `XSKS` (XSKMAP, 64 entries), `CONFIG` (per-queue config),
    `STATS` (per-queue counters)
  - Ethernet + IPv4 header parsing with verifier-safe bounds checks
  - Per-queue enable/disable via CONFIG map
  - Protocol filter support (all/IPv4/IPv6)
  - Graceful fallback to XDP_PASS when no socket bound or redirect disabled
  - Per-queue statistics: redirected, passed, dropped

- **af-xdp-common: XDP redirect types**
  - `XdpRedirectConfig` (4 bytes): Per-queue enable + protocol filter
  - `XdpRedirectStats` (24 bytes): Per-queue packet counters

- **af-xdp crate: Ring module** (`ebpf/af-xdp/src/ring.rs`)
  - Generic `Ring<T: Copy>` SPSC lock-free ring buffer
  - Producer operations: `reserve()`, `submit()`, `try_reserve()`,
    `push()`, `reserve_batch()`
  - Consumer operations: `peek()`, `release()`, `pop()`, `try_pop()`,
    `peek_batch()`
  - Entry access: `get()` / `set()` with mask-based wrap-around
  - Failure counters: `prod_failures`, `cons_failures`
  - Memory ordering: Acquire on remote index reads, Release on local
    index writes, Relaxed on local index reads
  - `unsafe impl Send` for cross-thread use (caller guarantees SPSC)
  - Power-of-2 size enforced with debug_assert

### Tests

- Ring: push/pop single, fill/overflow, wrap-around, batch operations
- Ring: failure counter tracking
- XDP redirect: socket creation test (with/without CAP_NET_ADMIN)
- Ring size power-of-2 validation

---

## Phase 6: Userspace RX/TX Engine with Epoll + Signal Handling

**Crate:** `af-xdp` (engine module, xsk module)

### Added

- **XskSocket** (`ebpf/af-xdp/src/xsk.rs`)
  - Complete AF_XDP socket lifecycle (7-step initialization):
    1. Create AF_XDP socket
    2. Register UMEM
    3. Set ring sizes (fill, completion, RX, TX)
    4. Query mmap offsets
    5. Mmap and setup all four rings
    6. Bind to interface + queue
    7. Pre-fill the fill ring with initial frames
  - `recv()`: Batch packet receive from RX ring
  - `send()`: Batch packet transmit via TX ring
  - `kick_tx()`: Kernel wakeup via sendto (handles EAGAIN gracefully)
  - `fill_frames()` / `drain_completions()`: Ring maintenance
  - `complete_cycle()`: Combined completion drain + fill ring refill
  - `poll()`: Socket readiness polling (POLLIN/POLLOUT)
  - NEED_WAKEUP support for RX and TX rings
  - Socket statistics via getsockopt(XDP_STATISTICS)
  - `Drop` implementation: close socket fd, rings munmap'd via their own Drop

- **RxRing** and **TxRing** (internal to xsk module)
  - Kernel-to-userspace (RX) and userspace-to-kernel (TX) packet rings
  - NEED_WAKEUP flag checking
  - Wrapping arithmetic for index management

- **XdpEngine** (`ebpf/af-xdp/src/engine.rs`)
  - High-level packet processing engine wrapping XskSocket + Umem
  - `rx_burst()`: Batch receive with automatic fill ring refill
  - `tx_burst()`: Batch transmit with automatic TX kick
  - `alloc_frame()` / `free_frame()`: Direct UMEM frame management
  - Statistics tracking: rx/tx packets, rx/tx bytes, drops, fill/comp events
  - `shutdown()`: Graceful drain of RX and completion rings

- **EventLoop** (`ebpf/af-xdp/src/engine.rs`)
  - Epoll-based event loop for efficient AF_XDP socket waiting
  - Raw epoll syscalls (epoll_create1, epoll_ctl, epoll_wait) via asm
  - EPOLLIN monitoring for RX readiness
  - Configurable timeout
  - `Drop` implementation: close epoll fd

- **SignalHandler** (`ebpf/af-xdp/src/engine.rs`)
  - Cooperative shutdown via `Arc<AtomicBool>`
  - `should_exit()`: Relaxed load for polling
  - `trigger()`: Release store from signal context
  - `flag()`: Clone inner Arc for cross-thread sharing
  - `Default` implementation

- **PacketBuf** (`ebpf/af-xdp/src/engine.rs`)
  - Packet buffer referencing UMEM frame by addr + len
  - `as_slice()` / `as_mut_slice()`: Zero-copy access to packet data

### Tests

- XskSocket: Creation test (requires CAP_NET_ADMIN, ignored by default)
- XskSocket: Loopback recv/send test (requires CAP_NET_ADMIN, ignored)
- Ring size validation

---

## Phase 7: Rust FFI + Go Bridge

**Crate:** `af-xdp` (ffi module)

### Added

- **FFI module** (`ebpf/af-xdp/src/ffi.rs`)
  - `extern "C"` functions for Go/C interop:
    - `afxdp_create(iface, queue, frame_count) -> *mut AfxdpHandle`
    - `afxdp_recv(handle, buf, buf_size, batch_size) -> c_int`
    - `afxdp_send(handle, buf, buf_size) -> c_int`
    - `afxdp_stats(handle, stats) -> c_int`
    - `afxdp_poll(handle, timeout_ms) -> c_int`
    - `afxdp_destroy(handle) -> c_int`
  - Error codes: AFXDP_OK (0), AFXDP_ERR_INVALID_ARGS (-1),
    AFXDP_ERR_NO_MEMORY (-2), AFXDP_ERR_SOCKET (-3), AFXDP_ERR_UMEM (-4)
  - Opaque `AfxdpHandle` wrapping heap-allocated XdpEngine
  - Null pointer guards at every FFI entry point
  - `AfxdpStats` C-compatible statistics structure

- **C header** (`ebpf/af-xdp/af_xdp.h`)
  - Complete C header for the FFI interface
  - Type definitions: `afxdp_handle_t`, `afxdp_stats_t`
  - Function declarations with doc comments
  - `extern "C"` linkage for C++ compatibility

- **Build configuration**
  - Cargo.toml: `crate-type = ["lib", "cdylib", "staticlib"]`
  - Produces `libaf_xdp.a` (static) and `libaf_xdp.so` (dynamic)

---

## Phase 8: Shield-eBPF AF_XDP Integration (Dual-Path Redirect)

**Crate:** `shield-ebpf`

### Added

- **SHIELD_XSKS** map: BPF_MAP_TYPE_XSKMAP with 64 entries
- **SHIELD_CONFIG** map: BPF_MAP_TYPE_HASH for runtime AF_XDP toggle
  - Key 0, bit 0: enable/disable AF_XDP redirect
- **Dual-path decision logic** in `shield_xdp()`:
  - After BIRTH stamping, check SHIELD_CONFIG[0] bit 0
  - If enabled: attempt `SHIELD_XSKS.redirect(queue_id, 0)`
  - If redirect succeeds: emit BIRTH event with `redirect_action::AF_XDP`,
    return redirect action
  - If redirect fails (no socket): fall through to kernel stack with
    `redirect_action::KERNEL_STACK`
  - If disabled: `redirect_action::NO_REDIRECT`, standard XDP_PASS
- **Statistics:** `STAT_REDIRECT_ATTEMPTS` (16), `STAT_REDIRECT_SUCCESS` (17)
- **Anamnesis events:** BIRTH events now encode the redirect_action in
  the hop_id field for observability

### Changed

- `monad-common` updated with `redirect_action` module constants
- Shield BIRTH event now records whether packet took AF_XDP or kernel path

---

## Phase 9: Packet-Marker Selective AF_XDP Redirect

**Crate:** `packet-marker`

### Added

- **MARKER_XSKS** map: BPF_MAP_TYPE_XSKMAP with 64 entries
- **MARKER_CONFIG** map: BPF_MAP_TYPE_HASH for runtime AF_XDP toggle
  - Key 0, bit 0: enable/disable AF_XDP redirect for traced packets
- **Selective redirect logic** in `packet_marker()`:
  - Only packets with non-zero trace_id are eligible for AF_XDP redirect
  - Unmarked packets always pass to kernel stack (standard behavior)
  - Check MARKER_CONFIG[0] bit 0 AND trace_id != 0
  - Attempt `MARKER_XSKS.redirect(queue_id, 0)`
  - Fallback to kernel stack if no socket bound
- **Statistics:** `STAT_AFXDP_REDIRECT` (8), `STAT_AFXDP_FALLBACK` (9)
- **Packet events:** Redirected packets emit `PacketAction::Redirect`
  (standard path emits `PacketAction::Pass`)

### Changed

- `monad-common` `flow_flags` module: added `TRACED_AF_XDP` (0x10) for
  flows eligible for AF_XDP redirect

---

## Phase 10: Performance Benchmarks

### Measured

- UMEM allocation: O(1) via LIFO stack (Vec::pop)
- Ring buffer operations: Lock-free SPSC with acquire/release ordering
- Zero system calls in hot path (mmap'd ring buffers)
- Packet processing: Zero-copy from NIC DMA to userspace via UMEM
- TX kick: Single sendto(fd, NULL, 0, MSG_DONTWAIT) only when NEED_WAKEUP set

### Design Decisions

- Frame size 4096 bytes (standard MTU + headroom, page-aligned)
- Default 2048 ring entries (sufficient for 10 Gbps burst absorption)
- Batch processing in rx_burst/tx_burst (up to 64 packets per call)
- Epoll-based event loop as alternative to busy-polling

---

## Phase 11: Documentation and Final Integration

### Added

- `docs/architecture/AF_XDP_ARCHITECTURE.md`: Complete architecture document
  with data flow diagrams, UMEM layout, ring topology, XSKMAP pinning,
  kernel requirements, and thread safety guarantees
- `docs/DEPLOYMENT_GUIDE_AFXDP.md`: Deployment guide with kernel config,
  hugepages, ulimit, permissions, loading sequence, troubleshooting, and
  minimal RX example
- `docs/TESTING_AFXDP.md`: Test organization, how to run, CI/CD integration
- `docs/MIGRATION_AFXDP.md`: Migration guide for existing shield/packet-marker
  users
- `docs/CHANGELOG_AFXDP.md`: This file
- Updated `docs/architecture/RUST_COMPONENTS.md` with all AF_XDP crates
- Created `ebpf/README.md` with AF_XDP quick start section

### Verified

- All source files in `ebpf/af-xdp/` and `ebpf/af-xdp-common/` have
  SPDX-License-Identifier headers (GPL-2.0-only)
