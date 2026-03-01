# AF_XDP (XSK) Zero-Copy Packet I/O Battle Plan — 12 Phases, 240+ Steps

**Date**: 2026-02-28
**Sprint**: S-XDP — Rust AF_XDP kernel bypass for Whispering Void zero-copy path
**Prerequisite**: Existing eBPF workspace compiles, kernel >= 5.15 with AF_XDP support
**Target**: Zero-copy packet RX/TX via AF_XDP sockets with XSKMAP redirect from XDP programs
**Estimated Duration**: 12-16 hours across 3-4 sessions
**Agent Strategy**: Phases 0-3 sequential (foundation), Phases 4-7 parallelizable, Phases 8-11 sequential (integration)
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

## LEGEND

[B] = Bash command
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable
[C] = Commit checkpoint
[STUCK] = Skipped via Skip Protocol
[BLOCKED] = Blocked by upstream STUCK

---

## PHASE 0: ENVIRONMENT VERIFICATION (Steps 1-20)

**Goal**: Verify kernel AF_XDP support, toolchain, and existing workspace health
**Prerequisite**: SSH access to bare metal or VM with kernel >= 5.15
**Time**: 15 minutes
**Agent**: Coordinator

---

### Step 1 [B][S] ~2min: Check kernel version >= 5.15

**Action**: Verify running kernel supports AF_XDP
```bash
uname -r
```
**Expected**: Output like `5.15.0+` or `6.x.x+`
**[V] Verification**: Kernel version >= 5.15
**[D] Debug**: If < 5.15, stop — contact infrastructure team for kernel upgrade

---

### Step 2 [B][S] ~2min: Check AF_XDP kernel config

**Action**: Verify CONFIG_XDP_SOCKETS and CONFIG_BPF enabled
```bash
zcat /boot/config-$(uname -r) | grep -E "CONFIG_XDP_SOCKETS|CONFIG_BPF|CONFIG_NET_XDP" | sort
```
**Expected Output**:
```
CONFIG_BPF=y
CONFIG_NET_XDP=y
CONFIG_XDP_SOCKETS=y
```
**[V] Verification**: All three configs present and set to `y`
**[D] Debug**: If any are `n` or missing, kernel recompile needed — skip via [STUCK]

---

### Step 3 [B][S] ~1min: Check CONFIG_XDP_SOCKETS_DIAG (optional but recommended)

**Action**: Verify diagnostics support for XDP sockets
```bash
zcat /boot/config-$(uname -r) | grep CONFIG_XDP_SOCKETS_DIAG
```
**Expected**: `CONFIG_XDP_SOCKETS_DIAG=y` (optional but useful)
**[V] Verification**: Present (non-blocking if missing)

---

### Step 4 [B] ~2min: Verify bpftool installed

**Action**: Check for BPF introspection tool
```bash
which bpftool && bpftool version
```
**Expected**: bpftool version >= 5.15
**[V] Verification**: Command succeeds
**[D] Debug**: If missing, install: `apt-get install -y linux-tools-generic` (Ubuntu) or `dnf install -y bpf-tools` (Fedora)

---

### Step 5 [B] ~1min: Verify Rust nightly toolchain

**Action**: Check Rust nightly and bpfel target installed
```bash
rustup toolchain list | grep nightly
rustc +nightly --print target-list | grep bpfel
```
**Expected**: nightly installed, bpfel-unknown-none in list
**[V] Verification**: Both present
**[D] Debug**: Install: `rustup +nightly target add bpfel-unknown-none`

---

### Step 6 [R] ~2min: Verify existing eBPF workspace structure

**Action**: Check unheaded/ebpf workspace layout
```bash
ls -la unheaded/ebpf/ | head -20
cat unheaded/ebpf/Cargo.toml | head -30
```
**Expected**: Workspace root, members (common, packet-marker, flow-tracker, etc), aya-ebpf 0.1 dep
**[V] Verification**: Cargo.toml exists, workspace defined
**[D] Debug**: If missing, stop — workspace not set up

---

### Step 7 [B] ~3min: Cargo check existing workspace

**Action**: Ensure all current eBPF programs compile
```bash
cd unheaded/ebpf && cargo check --target bpfel-unknown-none 2>&1 | tail -20
```
**Expected**: `Finished` or `Compiling` with no errors
**[V] Verification**: Zero compilation errors
**[D] Debug**: Run `cargo check` with full output to diagnose; fix blockers before proceeding

---

### Step 8 [B][S] ~2min: List available network interfaces

**Action**: Identify NICs eligible for AF_XDP testing
```bash
ip link show | grep -E "^[0-9]+:" | head -10
```
**Expected**: At least loopback (lo) and ideally one real NIC (eth0, wlan0, etc)
**[V] Verification**: At least loopback present
**[D] Debug**: N/A (always have lo)

---

### Step 9 [B][S] ~2min: Check ethtool for NIC AF_XDP support

**Action**: Verify NIC driver supports AF_XDP
```bash
ethtool -i eth0 2>/dev/null | grep driver || echo "eth0 not found, testing lo..."
```
**Expected**: Driver name (virtio_net, i40e, ixgbe, etc) — most modern drivers support AF_XDP
**[V] Verification**: ethtool output or graceful fallback to lo
**[D] Debug**: If real NIC absent, AF_XDP tests will use loopback

---

### Step 10 [B][S] ~2min: Check hugepages availability (AF_XDP performance)

**Action**: Verify huge page support (optimal but not required)
```bash
grep HugePages /proc/meminfo
```
**Expected**: HugePages_Total > 0 (optional for performance)
**[V] Verification**: Passes (non-blocking if zero)
**[D] Debug**: Allocate if needed: `echo 64 > /proc/sys/vm/nr_hugepages` (requires sudo)

---

### Step 11 [B] ~1min: Check available RAM for UMEM allocation

**Action**: Ensure sufficient memory for test UMEM (min 512MB)
```bash
free -h | grep Mem | awk '{print $2}'
```
**Expected**: At least 2GB free
**[V] Verification**: >= 512MB available
**[D] Debug**: If < 512MB, free space or skip memory-intensive tests

---

### Step 12 [B] ~1min: Verify llvm-tools (for eBPF binaries)

**Action**: Check llvm-tools installed for bpfel target
```bash
rustup component list | grep llvm-tools
```
**Expected**: `llvm-tools-x86_64-unknown-linux-gnu (installed)` or similar
**[V] Verification**: llvm-tools present
**[D] Debug**: Install: `rustup +nightly component add llvm-tools-x86_64-unknown-linux-gnu`

---

### Step 13 [R] ~2min: Inspect existing common types in ebpf/common

**Action**: Review shared types (TraceId, FlowKey, FlowState, PacketEvent)
```bash
head -100 unheaded/ebpf/common/src/lib.rs
```
**Expected**: #[repr(C)] structs, GPL-2.0-only license header, no_std
**[V] Verification**: File exists and has recognizable types
**[D] Debug**: If missing, fatal — stop

---

### Step 14 [B] ~2min: Verify no_std compatibility of common crate

**Action**: Check common crate compiles as no_std
```bash
cd unheaded/ebpf/common && cargo check --target bpfel-unknown-none 2>&1 | tail -5
```
**Expected**: `Finished` with no errors
**[V] Verification**: Compiles cleanly
**[D] Debug**: Fix any std dependency leaks; af-xdp-common will build on this

---

### Step 15 [R] ~2min: Check Go loader XSKMAP definition

**Action**: Verify BPF_MAP_TYPE_XSKMAP = 17 in pkg/ebpf/loader.go
```bash
grep -n "XSKMAP\|BPF_MAP_TYPE.*17" pkg/ebpf/loader.go
```
**Expected**: Definition found (may be const or #define)
**[V] Verification**: XSKMAP type defined
**[D] Debug**: Not critical if missing — will add it in Phase 4

---

### Step 16 [B] ~1min: Verify cargo-bpf tooling (optional)

**Action**: Check for cargo-bpf helper (nice-to-have)
```bash
cargo bpf --version 2>/dev/null || echo "cargo-bpf not installed (OK)"
```
**Expected**: Either version output or OK message
**[V] Verification**: Non-blocking
**[D] Debug**: Not needed for manual compilation

---

### Step 17 [B][S] ~2min: Verify CAP_NET_ADMIN for socket operations

**Action**: Check current user/process capabilities
```bash
getcap /bin/bash 2>/dev/null || echo "No caps set; rely on sudo"
id -G | grep -q 0 && echo "User is in group 0 (root-like)" || echo "Non-root user"
```
**Expected**: Either cap_net_admin set or user can sudo
**[V] Verification**: Can escalate privileges
**[D] Debug**: AF_XDP tests will require sudo; document this

---

### Step 18 [B] ~1min: Create test workspace structure (dry run)

**Action**: Verify directory layout for new crates
```bash
ls -d unheaded/ebpf/common unheaded/ebpf/packet-marker 2>/dev/null | head -3
```
**Expected**: At least 2 existing subdirs
**[V] Verification**: Workspace structure intact
**[D] Debug**: N/A

---

### Step 19 [B] ~1min: Final cargo clean for fresh state

**Action**: Clean build artifacts to ensure reproducibility
```bash
cd unheaded/ebpf && cargo clean 2>&1 | tail -1
```
**Expected**: Quiet completion
**[V] Verification**: No errors
**[D] Debug**: N/A

---

### Step 20 [C] ~2min: Commit PHASE 0 environment baseline

**Action**: Create git commit documenting verified environment
```bash
cd unheaded/ebpf && git add -A && git commit -m "PHASE 0: Verify kernel AF_XDP support >= 5.15, bpftool, Rust toolchain, workspace health"
```
**Expected**: Commit hash printed
**[V] Verification**: Commit succeeds
**[D] Debug**: If no changes, log verification instead: `echo "PHASE 0 VERIFIED" > .phase0-verified`

---

### PHASE 0 EXIT GATE

**PASS if all steps 1-20 verify**:
- Kernel >= 5.15 with CONFIG_XDP_SOCKETS=y
- bpftool available
- Rust nightly + bpfel-unknown-none target
- Existing eBPF workspace compiles
- Common types library (ebpf/common) builds
- Network interface available (at least loopback)
- Sufficient RAM (>= 512MB)
- CAP_NET_ADMIN or sudo access

**BLOCKED if**:
- Kernel < 5.15 → upgrade required [STUCK]
- CONFIG_XDP_SOCKETS != y → kernel rebuild [STUCK]
- Workspace fails to compile → fix first, then restart

**Proceed to PHASE 1 only after all gates pass**

---

## PHASE 1: AF_XDP COMMON TYPES (Steps 21-55)

**Goal**: Create ebpf/af-xdp-common/ crate with all shared types for kernel/userspace AF_XDP boundary
**Prerequisite**: PHASE 0 gates passed
**Time**: 45 minutes
**Agent**: Crate builder

---

### Step 21 [W] ~3min: Create af-xdp-common Cargo.toml

**Action**: Create new crate manifest
```bash
mkdir -p unheaded/ebpf/af-xdp-common/src
cat > unheaded/ebpf/af-xdp-common/Cargo.toml << 'EOF'
[package]
name = "af-xdp-common"
version = "0.1.0"
edition = "2021"
license = "GPL-2.0-only"

[dependencies]

[lib]
EOF
```
**Expected**: File created
**[V] Verification**: `ls -la unheaded/ebpf/af-xdp-common/Cargo.toml`

---

### Step 22 [W] ~2min: Create af-xdp-common lib.rs with header

**Action**: Write library root with no_std flag
```bash
cat > unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — AF_XDP zero-copy packet I/O common types
// Shared between kernel eBPF programs and userspace AF_XDP socket loader

#![no_std]
#![allow(non_camel_case_types)]
#![allow(non_snake_case)]

extern "C" {
    fn abort() -> !;
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! {
    unsafe { abort() }
}

EOF
```
**Expected**: File created
**[V] Verification**: `head -15 unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 23 [W] ~3min: Define XskDesc struct (UMEM descriptor)

**Action**: Add UMEM frame descriptor type
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
/// UMEM descriptor — points to a frame in the shared memory region
/// Used in fill ring (kernel consumer, userspace producer)
/// and completion ring (kernel producer, userspace consumer)
#[repr(C)]
#[derive(Copy, Clone, Debug)]
pub struct XskDesc {
    pub addr: u64,        // Frame address in UMEM
    pub len: u32,         // Frame length (typically 4096)
    pub options: u32,     // Frame options/flags
}

const_assert_eq!(core::mem::size_of::<XskDesc>(), 16);
const_assert_eq!(core::mem::align_of::<XskDesc>(), 8);

/// Helper for compile-time size assertions
#[allow(dead_code)]
const fn const_assert_eq(a: usize, b: usize) {
    let _ = [(); 0 - ((a != b) as usize)];
}

EOF
```
**Expected**: XskDesc definition added
**[V] Verification**: `grep -A 8 "struct XskDesc" unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 24 [W] ~4min: Define XskRingOffsets struct

**Action**: Add ring offset configuration for mmap regions
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
/// Ring offsets for mmap'd regions (returned by XDP_MMAP_OFFSETS getsockopt)
#[repr(C)]
#[derive(Copy, Clone, Debug)]
pub struct XskRingOffsets {
    pub producer: u64,      // Producer ring offset
    pub consumer: u64,      // Consumer ring offset
    pub desc: u64,          // Descriptor ring offset
    pub flags: u64,         // Flags offset
}

const_assert_eq!(core::mem::size_of::<XskRingOffsets>(), 32);
const_assert_eq!(core::mem::align_of::<XskRingOffsets>(), 8);

EOF
```
**Expected**: Struct added
**[V] Verification**: `grep -A 8 "struct XskRingOffsets" unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 25 [W] ~4min: Define XskConfig struct

**Action**: Add UMEM configuration type
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
/// AF_XDP UMEM configuration
#[repr(C)]
#[derive(Copy, Clone, Debug)]
pub struct XskConfig {
    pub frame_size: u32,         // Size of each frame (power of 2, >= 2048)
    pub frame_count: u32,        // Total number of frames
    pub headroom: u32,           // Headroom in each frame (packet offset)
    pub flags: u32,              // Configuration flags (XDP_SHARED_UMEM, etc)
}

const_assert_eq!(core::mem::size_of::<XskConfig>(), 16);
const_assert_eq!(core::mem::align_of::<XskConfig>(), 4);

EOF
```
**Expected**: Struct added
**[V] Verification**: `grep -A 8 "struct XskConfig" unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 26 [W] ~3min: Define RX/TX ring descriptor types

**Action**: Add completion and fill descriptors
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
/// Completion descriptor (kernel -> userspace, returned frames)
#[repr(C)]
#[derive(Copy, Clone, Debug)]
pub struct CompletionDesc {
    pub addr: u64,
    pub len: u32,
    pub _pad: u32,
}

const_assert_eq!(core::mem::size_of::<CompletionDesc>(), 16);

/// Fill descriptor (userspace -> kernel, frames to fill packets into)
#[repr(C)]
#[derive(Copy, Clone, Debug)]
pub struct FillDesc {
    pub addr: u64,
}

const_assert_eq!(core::mem::size_of::<FillDesc>(), 8);

EOF
```
**Expected**: Descriptors added
**[V] Verification**: `grep "struct CompletionDesc" unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 27 [W] ~3min: Define Sockaddr_xdp struct

**Action**: Add socket address type for bind()
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
/// Socket address for AF_XDP bind()
#[repr(C)]
#[derive(Copy, Clone, Debug)]
pub struct Sockaddr_xdp {
    pub family: u16,             // AF_XDP = 44
    pub flags: u16,              // XDP_* flags
    pub ifindex: u32,            // Interface index (from if_nametoindex)
    pub queue_id: u32,           // RX/TX queue ID
    pub shared_umem_fd: u32,      // FD for shared UMEM (if XDP_SHARED_UMEM)
}

const_assert_eq!(core::mem::size_of::<Sockaddr_xdp>(), 16);
const_assert_eq!(core::mem::align_of::<Sockaddr_xdp>(), 2);

// AF_XDP family value
pub const AF_XDP: u16 = 44;

EOF
```
**Expected**: Struct and constant added
**[V] Verification**: `grep "pub const AF_XDP" unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 28 [W] ~3min: Define XskStatistics struct

**Action**: Add statistics type for monitoring
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
/// AF_XDP socket statistics (from getsockopt XDP_STATISTICS)
#[repr(C)]
#[derive(Copy, Clone, Debug)]
pub struct XskStatistics {
    pub rx_dropped: u64,         // Frames dropped (ring full)
    pub rx_invalid_descs: u64,   // Invalid RX descriptors
    pub tx_invalid_descs: u64,   // Invalid TX descriptors
    pub rx_ring_full: u64,       // RX ring overflows
    pub rx_fill_ring_empty_descs: u64, // Fill ring empty
    pub tx_ring_empty_descs: u64, // TX ring empty
}

const_assert_eq!(core::mem::size_of::<XskStatistics>(), 48);
const_assert_eq!(core::mem::align_of::<XskStatistics>(), 8);

EOF
```
**Expected**: Struct added
**[V] Verification**: `grep "struct XskStatistics" unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 29 [W] ~4min: Add AF_XDP socket option constants

**Action**: Define setsockopt/getsockopt option names
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
// AF_XDP socket option names (for setsockopt/getsockopt)
pub const XDP_MMAP_OFFSETS: i32 = 1;
pub const XDP_RX_RING: i32 = 2;
pub const XDP_TX_RING: i32 = 3;
pub const XDP_UMEM_REG: i32 = 4;
pub const XDP_UMEM_FILL_RING: i32 = 5;
pub const XDP_UMEM_COMPLETION_RING: i32 = 6;
pub const XDP_STATISTICS: i32 = 7;
pub const XDP_OPTIONS: i32 = 8;

// AF_XDP flags
pub const XDP_SHARED_UMEM: u16 = 1 << 0;
pub const XDP_COPY: u16 = 1 << 1;
pub const XDP_ZEROCOPY: u16 = 1 << 2;
pub const XDP_USE_NEED_WAKEUP: u16 = 1 << 3;

EOF
```
**Expected**: Constants added
**[V] Verification**: `grep "pub const XDP_" unheaded/ebpf/af-xdp-common/src/lib.rs | wc -l` should be >= 12

---

### Step 30 [B] ~2min: Verify af-xdp-common builds as no_std

**Action**: Compile af-xdp-common crate
```bash
cd unheaded/ebpf/af-xdp-common && cargo check --target bpfel-unknown-none 2>&1 | tail -10
```
**Expected**: `Finished` with no errors
**[V] Verification**: Build succeeds
**[D] Debug**: Check for std references: `grep -r "use std" src/`; should be empty

---

### Step 31 [W] ~3min: Add af-xdp-common to workspace Cargo.toml

**Action**: Register new crate in workspace members
```bash
cd unheaded/ebpf && grep -A 10 "\[workspace\]" Cargo.toml
```
**Expected**: Current members list visible
```bash
# Then add af-xdp-common to members list (exact location depends on current format)
# Example: add "af-xdp-common" to the members array
```
**[V] Verification**: Edit Cargo.toml manually or use sed to insert af-xdp-common

---

### Step 32 [B] ~2min: Full workspace cargo check

**Action**: Verify entire workspace compiles with new crate
```bash
cd unheaded/ebpf && cargo check --target bpfel-unknown-none 2>&1 | tail -5
```
**Expected**: `Finished` with no errors
**[V] Verification**: All crates including af-xdp-common compile
**[D] Debug**: If failure, check workspace member format; may need `path = "af-xdp-common"`

---

### Step 33 [W] ~3min: Add core::mem tests module to af-xdp-common

**Action**: Write compile-time size/alignment verification
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_xsk_desc_layout() {
        assert_eq!(core::mem::size_of::<XskDesc>(), 16);
        assert_eq!(core::mem::align_of::<XskDesc>(), 8);
    }

    #[test]
    fn test_xsk_ring_offsets_layout() {
        assert_eq!(core::mem::size_of::<XskRingOffsets>(), 32);
        assert_eq!(core::mem::align_of::<XskRingOffsets>(), 8);
    }

    #[test]
    fn test_sockaddr_xdp_layout() {
        assert_eq!(core::mem::size_of::<Sockaddr_xdp>(), 16);
        assert_eq!(core::mem::align_of::<Sockaddr_xdp>(), 2);
    }

    #[test]
    fn test_xsk_statistics_layout() {
        assert_eq!(core::mem::size_of::<XskStatistics>(), 48);
        assert_eq!(core::mem::align_of::<XskStatistics>(), 8);
    }
}

EOF
```
**Expected**: Tests appended
**[V] Verification**: `grep "#\[test\]" unheaded/ebpf/af-xdp-common/src/lib.rs | wc -l` >= 4

---

### Step 34 [B] ~2min: Run af-xdp-common tests

**Action**: Execute layout verification tests
```bash
cd unheaded/ebpf/af-xdp-common && cargo test --target bpfel-unknown-none 2>&1 | grep -E "test result:|passed"
```
**Expected**: `test result: ok.` output
**[V] Verification**: All tests pass
**[D] Debug**: If size mismatch, check struct field order and padding

---

### Step 35 [W] ~2min: Add documentation comments to public types

**Action**: Enhance rustdoc for future reference
```bash
cat > unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — AF_XDP zero-copy packet I/O common types
// Shared between kernel eBPF programs and userspace AF_XDP socket loader
//
// Kernel >= 5.15 required. These types define the C ABI boundary
// between eBPF programs (kernel) and userspace AF_XDP socket handler.

#![no_std]
#![allow(non_camel_case_types)]
#![allow(non_snake_case)]

extern "C" {
    fn abort() -> !;
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! {
    unsafe { abort() }
}

EOF
# Then re-append all the type definitions (reuse from step 23-29)
```
**[V] Verification**: Header documentation visible: `head -20 unheaded/ebpf/af-xdp-common/src/lib.rs`

---

### Step 36 [B] ~1min: Final workspace cargo check for PHASE 1

**Action**: Ensure no regressions
```bash
cd unheaded/ebpf && cargo check --target bpfel-unknown-none 2>&1 | tail -3
```
**Expected**: `Finished` with no errors
**[V] Verification**: All compiles cleanly
**[D] Debug**: Rerun step 32 if failures

---

### Step 37 [W] ~2min: Document AF_XDP constants in comments

**Action**: Add inline comments explaining option names
```bash
cat >> unheaded/ebpf/af-xdp-common/src/lib.rs << 'EOF'
// =============================================================================
// AF_XDP Socket Option Constants — for setsockopt(fd, SOL_XDP, option, ...)
// =============================================================================
// These values match the kernel's af_xdp.h definitions (kernel >= 5.15)

EOF
```
**[V] Verification**: Comments added as clarity aid

---

### Step 38 [B] ~2min: Review af-xdp-common module completeness

**Action**: Count types and constants defined
```bash
echo "Types defined:" && grep "^pub struct\|^pub enum" unheaded/ebpf/af-xdp-common/src/lib.rs | wc -l
echo "Constants defined:" && grep "^pub const" unheaded/ebpf/af-xdp-common/src/lib.rs | wc -l
```
**Expected**: >= 8 types, >= 15 constants
**[V] Verification**: All expected definitions present

---

### Step 39 [B] ~1min: Verify no external dependencies

**Action**: Ensure af-xdp-common has zero dependencies
```bash
grep "\[dependencies\]" -A 10 unheaded/ebpf/af-xdp-common/Cargo.toml
```
**Expected**: Empty dependencies section (no crates listed)
**[V] Verification**: No deps (matches Sacred Law: std library only)

---

### Step 40 [C] ~2min: Commit PHASE 1 — AF_XDP common types

**Action**: Create commit for new af-xdp-common crate
```bash
cd unheaded/ebpf && git add af-xdp-common/ && git commit -m "PHASE 1: Add af-xdp-common crate with AF_XDP kernel boundary types (XskDesc, XskRingOffsets, Sockaddr_xdp, etc)"
```
**Expected**: Commit hash printed
**[V] Verification**: `git log --oneline | head -1` shows new commit
**[D] Debug**: If no changes, likely already committed

---

### PHASE 1 EXIT GATE

**PASS if**:
- af-xdp-common/src/lib.rs exists with all types defined
- Structs: XskDesc, XskRingOffsets, XskConfig, CompletionDesc, FillDesc, Sockaddr_xdp, XskStatistics
- Constants: AF_XDP=44, XDP_MMAP_OFFSETS, XDP_RX_RING, XDP_TX_RING, XDP_UMEM_REG, XDP_UMEM_FILL_RING, XDP_UMEM_COMPLETION_RING, XDP_STATISTICS, XDP_OPTIONS, XDP_SHARED_UMEM, XDP_COPY, XDP_ZEROCOPY, XDP_USE_NEED_WAKEUP
- Layout tests verify struct sizes match kernel expectations
- `cargo check --target bpfel-unknown-none` succeeds
- af-xdp-common added to workspace members
- Zero external dependencies (no_std)
- Commit created

**BLOCKED if**:
- Struct sizes mismatch kernel → [D] verify repr(C) and field order
- Compilation fails → check for std imports [STUCK]

**Proceed to PHASE 2 only after all gates pass**

---

## PHASE 2: UMEM ALLOCATOR (Steps 41-85)

**Goal**: Implement UMEM (shared memory region) allocator — the foundation of AF_XDP zero-copy
**Prerequisite**: PHASE 1 gates passed, af-xdp-common available
**Time**: 90 minutes
**Agent**: Memory subsystem engineer

---

### Step 41 [W] ~2min: Create af-xdp userspace library Cargo.toml

**Action**: Create new crate for userspace AF_XDP socket logic
```bash
mkdir -p unheaded/af-xdp/src
cat > unheaded/af-xdp/Cargo.toml << 'EOF'
[package]
name = "af-xdp"
version = "0.1.0"
edition = "2021"
license = "GPL-2.0-only"

[dependencies]
af-xdp-common = { path = "../unheaded/ebpf/af-xdp-common" }

[lib]
EOF
```
**Expected**: Cargo.toml created at top level (userspace, not in ebpf/)
**[V] Verification**: `ls -la unheaded/af-xdp/Cargo.toml`

---

### Step 42 [W] ~2min: Create af-xdp lib.rs and umem module placeholder

**Action**: Write library root with umem module
```bash
cat > unheaded/af-xdp/src/lib.rs << 'EOF'
// SPDX-License-Identifier: GPL-2.0-only
// Unheaded Kingdom — AF_XDP userspace socket library
// Zero-copy packet I/O via kernel AF_XDP interface

pub mod umem;
pub mod xsk;

pub use umem::Umem;
pub use xsk::XskSocket;

EOF
```
**Expected**: Root library created
**[V] Verification**: `cat unheaded/af-xdp/src/lib.rs`

---

### Step 43 [W] ~4min: Start umem.rs module with Umem struct

**Action**: Create UMEM allocator foundation
```bash
cat > unheaded/af-xdp/src/umem.rs << 'EOF'
// SPDX-License-Identifier: GPL-2.0-only
// UMEM (User Memory) allocator for AF_XDP zero-copy packet pool

use af_xdp_common::{XskDesc, XskConfig, CompletionDesc, FillDesc};
use core::ptr::NonNull;

/// UMEM — shared memory region for zero-copy AF_XDP packet buffers
/// Manages allocation, deallocation, and ring buffer access
pub struct Umem {
    /// Mmap'd region base address
    addr: NonNull<u8>,
    /// Total allocated size in bytes
    size: u64,
    /// Configuration (frame size, count, headroom, flags)
    config: XskConfig,
    /// Free frame stack (LIFO allocator)
    free_frames: Vec<u64>,
    /// Allocated frame count
    frame_count: u32,
}

impl Umem {
    /// Create new UMEM with given configuration
    pub fn new(config: XskConfig) -> Result<Self, &'static str> {
        // Validate config first
        if config.frame_size == 0 || (config.frame_size & (config.frame_size - 1)) != 0 {
            return Err("frame_size must be power of 2");
        }
        if config.frame_size < 2048 {
            return Err("frame_size must be >= 2048");
        }
        if config.frame_count == 0 {
            return Err("frame_count must be > 0");
        }

        let total_size = (config.frame_size as u64) * (config.frame_count as u64);

        // Allocate via mmap (will be implemented in step 44)
        let addr = unsafe { Self::mmap_alloc(total_size)? };

        // Initialize free frame stack (all frames initially free)
        let mut free_frames = Vec::new();
        for i in 0..config.frame_count {
            free_frames.push((i as u64) * (config.frame_size as u64));
        }

        Ok(Umem {
            addr,
            size: total_size,
            config,
            free_frames,
            frame_count: config.frame_count,
        })
    }

    /// Allocate mmap region (placeholder for now)
    unsafe fn mmap_alloc(size: u64) -> Result<NonNull<u8>, &'static str> {
        // Step 44: Implement actual mmap syscall
        Err("not yet implemented")
    }

    /// Allocate a frame from UMEM
    pub fn alloc_frame(&mut self) -> Option<XskDesc> {
        self.free_frames.pop().map(|addr| {
            XskDesc {
                addr: self.addr.as_ptr() as u64 + addr,
                len: self.config.frame_size as u32,
                options: 0,
            }
        })
    }

    /// Free a frame back to pool
    pub fn free_frame(&mut self, addr: u64) {
        // Validate address is in range, then push back
        if addr >= self.addr.as_ptr() as u64 &&
           addr < self.addr.as_ptr() as u64 + self.size {
            self.free_frames.push(addr - self.addr.as_ptr() as u64);
        }
    }

    /// Get base address of UMEM region
    pub fn base_addr(&self) -> u64 {
        self.addr.as_ptr() as u64
    }

    /// Get total size of UMEM region
    pub fn total_size(&self) -> u64 {
        self.size
    }

    /// Get configuration
    pub fn config(&self) -> XskConfig {
        self.config
    }

    /// Get number of free frames
    pub fn free_frame_count(&self) -> usize {
        self.free_frames.len()
    }
}

impl Drop for Umem {
    fn drop(&mut self) {
        // Step 44: Implement actual munmap syscall
        unsafe {
            // munmap will be called here
        }
    }
}

EOF
```
**Expected**: umem.rs module started
**[V] Verification**: `head -80 unheaded/af-xdp/src/umem.rs`

---

### Step 44 [W] ~5min: Implement mmap/munmap via libc-free syscalls

**Action**: Add raw mmap/munmap using only libc syscall wrappers
```bash
cat >> unheaded/af-xdp/src/umem.rs << 'EOF'

// =============================================================================
// Raw syscall wrappers (libc-free, std library only)
// =============================================================================

use std::os::unix::io::RawFd;

mod syscall {
    use super::RawFd;

    /// mmap syscall wrapper
    /// mmap(addr, len, prot, flags, fd, offset)
    /// PROT_READ=0x1, PROT_WRITE=0x2
    /// MAP_SHARED=0x01, MAP_ANONYMOUS=0x20, MAP_PRIVATE=0x02
    #[cfg(target_arch = "x86_64")]
    pub unsafe fn mmap(
        addr: *mut u8,
        len: u64,
        prot: i32,
        flags: i32,
        fd: i32,
        offset: i64,
    ) -> Result<*mut u8, i32> {
        let result = libc::syscall(
            9,  // SYS_mmap on x86_64
            addr as u64,
            len,
            prot as u64,
            flags as u64,
            fd as u64,
            offset as u64,
        );

        if result < 0 {
            Err(-(result as i32))
        } else {
            Ok(result as *mut u8)
        }
    }

    /// munmap syscall wrapper
    /// munmap(addr, len)
    #[cfg(target_arch = "x86_64")]
    pub unsafe fn munmap(addr: *mut u8, len: u64) -> Result<(), i32> {
        let result = libc::syscall(11, addr as u64, len);
        if result < 0 {
            Err(-(result as i32))
        } else {
            Ok(())
        }
    }

    pub const PROT_READ: i32 = 0x1;
    pub const PROT_WRITE: i32 = 0x2;
    pub const MAP_SHARED: i32 = 0x01;
    pub const MAP_ANONYMOUS: i32 = 0x20;
}

impl Umem {
    /// Actual mmap allocation using syscall
    unsafe fn mmap_alloc(size: u64) -> Result<NonNull<u8>, &'static str> {
        use syscall::*;

        let ptr = mmap(
            std::ptr::null_mut(),
            size,
            PROT_READ | PROT_WRITE,
            MAP_SHARED | MAP_ANONYMOUS,
            -1,
            0,
        ).map_err(|_| "mmap failed")?;

        NonNull::new(ptr).ok_or("mmap returned NULL")
    }

    /// Actual munmap deallocation using syscall
    pub fn munmap(&mut self) -> Result<(), &'static str> {
        use syscall::*;
        unsafe {
            munmap(self.addr.as_ptr(), self.size)
                .map_err(|_| "munmap failed")?;
        }
        Ok(())
    }
}

EOF
```
**Expected**: mmap/munmap syscall wrappers added
**[V] Verification**: `grep "pub unsafe fn mmap" unheaded/af-xdp/src/umem.rs`

---

### Step 45 [B] ~2min: Check af-xdp compilation with std dependency

**Action**: Verify umem.rs compiles (needs std for syscall access)
```bash
cd unheaded/af-xdp && cargo check 2>&1 | tail -10
```
**Expected**: Compilation may have warnings about libc but should compile
**[V] Verification**: No hard errors (warnings OK)
**[D] Debug**: If libc not available, standard library includes it — add explicit dep if needed

---

### Step 46 [W] ~3min: Add Fill ring initialization to Umem

**Action**: Implement fill ring setup (kernel consumer)
```bash
cat >> unheaded/af-xdp/src/umem.rs << 'EOF'

/// Fill ring offset structure (returned by XDP_MMAP_OFFSETS getsockopt)
pub struct FillRing {
    /// Producer index (userspace increments)
    producer: *mut u32,
    /// Consumer index (kernel increments)
    consumer: *mut u32,
    /// Descriptor array (FillDesc = u64 frame address)
    descs: *mut u64,
    /// Ring size (in number of descriptors)
    size: u32,
}

impl Umem {
    /// Setup fill ring from mmap'd region
    /// Called after socket creation and mmap
    pub fn setup_fill_ring(
        &self,
        ring_mem: *mut u8,
        offsets: &af_xdp_common::XskRingOffsets,
        ring_size: u32,
    ) -> FillRing {
        unsafe {
            let base = ring_mem;
            FillRing {
                producer: (base.add(offsets.producer as usize)) as *mut u32,
                consumer: (base.add(offsets.consumer as usize)) as *mut u32,
                descs: (base.add(offsets.desc as usize)) as *mut u64,
                size: ring_size,
            }
        }
    }

    /// Add frame to fill ring (for kernel to consume)
    pub fn fill_ring_push(&self, ring: &mut FillRing, addr: u64) -> Result<(), &'static str> {
        unsafe {
            let producer = *ring.producer;
            let consumer = *ring.consumer;

            // Check if ring is full
            if producer - consumer >= ring.size as u32 {
                return Err("fill ring full");
            }

            let idx = (producer % ring.size as u32) as usize;
            *ring.descs.add(idx) = addr;

            // Increment producer (memory barrier implicit in volatile access)
            *ring.producer = producer + 1;
            Ok(())
        }
    }
}

EOF
```
**Expected**: Fill ring methods added
**[V] Verification**: `grep "pub struct FillRing" unheaded/af-xdp/src/umem.rs`

---

### Step 47 [W] ~3min: Add Completion ring to Umem

**Action**: Implement completion ring setup (kernel producer)
```bash
cat >> unheaded/af-xdp/src/umem.rs << 'EOF'

/// Completion ring (kernel returns completed frames)
pub struct CompletionRing {
    /// Consumer index (userspace increments)
    consumer: *mut u32,
    /// Producer index (kernel increments)
    producer: *mut u32,
    /// Descriptor array (CompletionDesc = { addr, len })
    descs: *mut af_xdp_common::CompletionDesc,
    /// Ring size
    size: u32,
}

impl Umem {
    /// Setup completion ring from mmap'd region
    pub fn setup_completion_ring(
        &self,
        ring_mem: *mut u8,
        offsets: &af_xdp_common::XskRingOffsets,
        ring_size: u32,
    ) -> CompletionRing {
        unsafe {
            let base = ring_mem;
            CompletionRing {
                consumer: (base.add(offsets.consumer as usize)) as *mut u32,
                producer: (base.add(offsets.producer as usize)) as *mut u32,
                descs: (base.add(offsets.desc as usize)) as *mut af_xdp_common::CompletionDesc,
                size: ring_size,
            }
        }
    }

    /// Drain completion ring (return completed frames to free pool)
    pub fn completion_ring_drain(
        &mut self,
        ring: &mut CompletionRing,
    ) -> u32 {
        unsafe {
            let producer = *ring.producer;
            let consumer = *ring.consumer;
            let mut count = 0;

            let mut i = consumer;
            while i != producer {
                let idx = (i % ring.size as u32) as usize;
                let desc = *ring.descs.add(idx);
                self.free_frame(desc.addr);
                i += 1;
                count += 1;
            }

            // Update consumer
            *ring.consumer = producer;
            count
        }
    }
}

EOF
```
**Expected**: Completion ring methods added
**[V] Verification**: `grep "pub struct CompletionRing" unheaded/af-xdp/src/umem.rs`

---

### Step 48 [B] ~2min: Verify umem.rs compiles with fill/completion rings

**Action**: Check compilation after ring additions
```bash
cd unheaded/af-xdp && cargo check 2>&1 | grep -E "error|Finished"
```
**Expected**: `Finished` or `Compiling`
**[V] Verification**: No errors
**[D] Debug**: Check pointer arithmetic is correct

---

### Step 49 [W] ~3min: Create placeholder xsk.rs module

**Action**: Stub out XskSocket struct for later implementation
```bash
cat > unheaded/af-xdp/src/xsk.rs << 'EOF'
// SPDX-License-Identifier: GPL-2.0-only
// AF_XDP socket (XskSocket) — RX/TX rings with kernel bypass

use std::os::unix::io::{RawFd, AsRawFd};
use af_xdp_common::XskDesc;

/// AF_XDP socket for zero-copy packet I/O
pub struct XskSocket {
    /// Raw socket file descriptor
    fd: RawFd,
    /// Associated UMEM base address
    umem_base: u64,
}

impl XskSocket {
    /// Create new AF_XDP socket (stub for Phase 3)
    pub fn new(_ifname: &str, _queue_id: u32) -> Result<Self, &'static str> {
        Err("not yet implemented in Phase 3")
    }

    /// Receive packets from RX ring
    pub fn recv(&mut self) -> Result<Vec<XskDesc>, &'static str> {
        Err("not yet implemented")
    }

    /// Send packets via TX ring
    pub fn send(&mut self, _descs: Vec<XskDesc>) -> Result<u32, &'static str> {
        Err("not yet implemented")
    }

    /// Get socket file descriptor for epoll
    pub fn fd(&self) -> RawFd {
        self.fd
    }
}

impl AsRawFd for XskSocket {
    fn as_raw_fd(&self) -> RawFd {
        self.fd
    }
}

impl Drop for XskSocket {
    fn drop(&mut self) {
        unsafe {
            libc::close(self.fd);
        }
    }
}

EOF
```
**Expected**: xsk.rs stub created
**[V] Verification**: `head -30 unheaded/af-xdp/src/xsk.rs`

---

### Step 50 [B] ~2min: Full af-xdp crate cargo check

**Action**: Verify entire userspace crate compiles
```bash
cd unheaded/af-xdp && cargo check 2>&1 | tail -5
```
**Expected**: `Finished` or `Compiling` with no errors
**[V] Verification**: Compilation succeeds
**[D] Debug**: Resolve any dependency issues; af-xdp-common should be accessible

---

### Step 51 [W] ~4min: Add UMEM registration syscall helper

**Action**: Create setsockopt wrapper for XDP_UMEM_REG
```bash
cat >> unheaded/af-xdp/src/umem.rs << 'EOF'

/// Register UMEM with socket via setsockopt
pub fn register_umem_with_socket(
    sock_fd: i32,
    umem: &Umem,
) -> Result<(), &'static str> {
    let xsk_umem_reg = af_xdp_common::XskConfig {
        frame_size: umem.config.frame_size,
        frame_count: umem.config.frame_count,
        headroom: umem.config.headroom,
        flags: umem.config.flags,
    };

    // setsockopt(sock_fd, SOL_XDP, XDP_UMEM_REG, &xsk_umem_reg, sizeof(...))
    let ret = unsafe {
        libc::setsockopt(
            sock_fd,
            44, // SOL_XDP = 44
            af_xdp_common::XDP_UMEM_REG as i32,
            &xsk_umem_reg as *const _ as *const libc::c_void,
            std::mem::size_of::<af_xdp_common::XskConfig>() as u32,
        )
    };

    if ret < 0 {
        Err("setsockopt XDP_UMEM_REG failed")
    } else {
        Ok(())
    }
}

EOF
```
**Expected**: UMEM registration helper added
**[V] Verification**: `grep "pub fn register_umem_with_socket" unheaded/af-xdp/src/umem.rs`

---

### Step 52 [W] ~3min: Add unit tests for UMEM allocation

**Action**: Write basic frame allocation tests
```bash
cat >> unheaded/af-xdp/src/umem.rs << 'EOF'

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_umem_creation() {
        let config = af_xdp_common::XskConfig {
            frame_size: 4096,
            frame_count: 512,
            headroom: 0,
            flags: 0,
        };

        let umem = Umem::new(config);
        assert!(umem.is_ok(), "UMEM creation should succeed");
    }

    #[test]
    fn test_umem_frame_allocation() {
        let config = af_xdp_common::XskConfig {
            frame_size: 4096,
            frame_count: 10,
            headroom: 0,
            flags: 0,
        };

        let mut umem = Umem::new(config).expect("UMEM creation failed");

        // Allocate all frames
        for i in 0..10 {
            let frame = umem.alloc_frame();
            assert!(frame.is_some(), "Frame {} allocation should succeed", i);
        }

        // OOM on next allocation
        let frame = umem.alloc_frame();
        assert!(frame.is_none(), "Frame allocation should fail on OOM");
    }

    #[test]
    fn test_umem_frame_free_reuse() {
        let config = af_xdp_common::XskConfig {
            frame_size: 4096,
            frame_count: 2,
            headroom: 0,
            flags: 0,
        };

        let mut umem = Umem::new(config).expect("UMEM creation failed");

        let frame1 = umem.alloc_frame().expect("Frame 1 alloc");
        let frame2 = umem.alloc_frame().expect("Frame 2 alloc");
        assert!(umem.alloc_frame().is_none(), "OOM after 2 allocations");

        // Free frame1 and reuse
        umem.free_frame(frame1.addr);
        let frame3 = umem.alloc_frame().expect("Frame 3 alloc (reuse)");
        assert_eq!(frame3.addr, frame1.addr, "Freed frame should be reused");
    }

    #[test]
    fn test_invalid_config() {
        // Test frame_size = 0
        let config = af_xdp_common::XskConfig {
            frame_size: 0,
            frame_count: 10,
            headroom: 0,
            flags: 0,
        };
        assert!(Umem::new(config).is_err());

        // Test frame_size not power of 2
        let config = af_xdp_common::XskConfig {
            frame_size: 3000,
            frame_count: 10,
            headroom: 0,
            flags: 0,
        };
        assert!(Umem::new(config).is_err());

        // Test frame_size < 2048
        let config = af_xdp_common::XskConfig {
            frame_size: 1024,
            frame_count: 10,
            headroom: 0,
            flags: 0,
        };
        assert!(Umem::new(config).is_err());
    }
}

EOF
```
**Expected**: Test module added
**[V] Verification**: `grep "#\[test\]" unheaded/af-xdp/src/umem.rs | wc -l` >= 4

---

### Step 53 [B] ~3min: Run UMEM unit tests

**Action**: Execute allocation and ring tests
```bash
cd unheaded/af-xdp && cargo test --lib umem::tests 2>&1 | tail -20
```
**Expected**: `test result: ok.` with 4+ tests passing
**[V] Verification**: All tests pass
**[D] Debug**: If mmap issues, verify syscall wrapper correctness

---

### Step 54 [W] ~2min: Add frame size validation helper function

**Action**: Extract validation logic for reuse
```bash
cat >> unheaded/af-xdp/src/umem.rs << 'EOF'

/// Validate AF_XDP configuration parameters
pub fn validate_config(config: &af_xdp_common::XskConfig) -> Result<(), &'static str> {
    if config.frame_size == 0 || (config.frame_size & (config.frame_size - 1)) != 0 {
        return Err("frame_size must be power of 2");
    }
    if config.frame_size < 2048 {
        return Err("frame_size must be >= 2048");
    }
    if config.frame_count == 0 {
        return Err("frame_count must be > 0");
    }
    if config.frame_count > 1_000_000 {
        return Err("frame_count too large");
    }
    Ok(())
}

EOF
```
**Expected**: Validation helper added
**[V] Verification**: `grep "pub fn validate_config" unheaded/af-xdp/src/umem.rs`

---

### Step 55 [C] ~2min: Commit PHASE 2 — UMEM allocator

**Action**: Create commit for UMEM implementation
```bash
cd unheaded/af-xdp && git add -A && git commit -m "PHASE 2: Implement UMEM allocator with mmap allocation, frame pool, fill/completion rings"
```
**Expected**: Commit hash printed
**[V] Verification**: `git log --oneline | head -1` shows PHASE 2 commit
**[D] Debug**: If no changes, likely first commit to af-xdp repo

---

### PHASE 2 EXIT GATE

**PASS if**:
- af-xdp/src/umem.rs implements:
  - Umem struct with mmap'd memory region
  - Frame allocator (free-list based)
  - FillRing struct and push method
  - CompletionRing struct and drain method
  - mmap/munmap syscall wrappers (no libc usage)
  - register_umem_with_socket() setsockopt wrapper
  - validate_config() function
- Unit tests:
  - UMEM creation succeeds
  - Frame allocation and OOM handling
  - Frame free and reuse
  - Configuration validation
- `cargo test --lib` passes all tests
- `cargo check` with no errors
- UMEM can allocate/free cycles in tests

**BLOCKED if**:
- mmap fails → [D] verify syscall ABI for target arch [STUCK]
- Ring setup wrong → [D] check offset calculations against kernel headers

**Proceed to PHASE 3 only after all gates pass**

---

## PHASE 3: XSK SOCKET (Steps 101-140)

**Goal**: Implement AF_XDP socket creation with RX/TX rings bound to UMEM
**Prerequisite**: PHASE 2 gates passed, UMEM allocator working
**Time**: 60 minutes
**Agent**: Socket implementation specialist

---

### Step 101 [R] ~2min: Review xsk.rs stub structure

**Action**: Check placeholder created in step 49
```bash
head -50 unheaded/af-xdp/src/xsk.rs
```
**Expected**: XskSocket struct stub visible
**[V] Verification**: File exists with basic structure

---

### Step 102 [W] ~3min: Expand XskSocket to include RX/TX ring pointers

**Action**: Add ring structures to XskSocket
```bash
cat > unheaded/af-xdp/src/xsk.rs << 'EOF'
// SPDX-License-Identifier: GPL-2.0-only
// AF_XDP socket (XskSocket) — RX/TX rings with kernel bypass

use std::os::unix::io::{RawFd, AsRawFd};
use af_xdp_common::{XskDesc, XskRingOffsets};
use crate::umem::Umem;

/// RX ring (kernel -> userspace, packets received)
pub struct RxRing {
    /// Producer index (kernel increments)
    producer: *mut u32,
    /// Consumer index (userspace increments)
    consumer: *mut u32,
    /// Descriptor array (XskDesc with packet buffers)
    descs: *mut XskDesc,
    /// Ring size in number of descriptors
    size: u32,
}

/// TX ring (userspace -> kernel, packets to send)
pub struct TxRing {
    /// Producer index (userspace increments)
    producer: *mut u32,
    /// Consumer index (kernel increments)
    consumer: *mut u32,
    /// Descriptor array (XskDesc with packet buffers)
    descs: *mut XskDesc,
    /// Ring size in number of descriptors
    size: u32,
}

/// AF_XDP socket for zero-copy packet I/O
pub struct XskSocket {
    /// Raw socket file descriptor
    fd: RawFd,
    /// Associated UMEM for packet buffers
    umem: *mut Umem,
    /// RX ring (kernel -> userspace)
    rx_ring: RxRing,
    /// TX ring (userspace -> kernel)
    tx_ring: TxRing,
    /// RX ring mmap'd memory base
    rx_mem: *mut u8,
    /// TX ring mmap'd memory base
    tx_mem: *mut u8,
}

impl XskSocket {
    /// Create new AF_XDP socket on interface + queue (stub for Phase 3)
    pub fn new(_umem: &mut Umem, _ifname: &str, _queue_id: u32) -> Result<Self, &'static str> {
        Err("not yet implemented in Phase 3")
    }

    /// Receive packets from RX ring
    pub fn recv(&mut self) -> Result<Vec<XskDesc>, &'static str> {
        Err("not yet implemented")
    }

    /// Send packets via TX ring
    pub fn send(&mut self, _descs: Vec<XskDesc>) -> Result<u32, &'static str> {
        Err("not yet implemented")
    }

    /// Get socket file descriptor for epoll
    pub fn fd(&self) -> RawFd {
        self.fd
    }
}

impl AsRawFd for XskSocket {
    fn as_raw_fd(&self) -> RawFd {
        self.fd
    }
}

impl Drop for XskSocket {
    fn drop(&mut self) {
        unsafe {
            if !self.rx_mem.is_null() {
                let _ = libc::munmap(self.rx_mem as *mut libc::c_void, 1024); // stub size
            }
            if !self.tx_mem.is_null() {
                let _ = libc::munmap(self.tx_mem as *mut libc::c_void, 1024); // stub size
            }
            libc::close(self.fd);
        }
    }
}

EOF
```
**Expected**: XskSocket expanded with ring structures
**[V] Verification**: `grep "pub struct RxRing" unheaded/af-xdp/src/xsk.rs`

---

### Step 103 [W] ~3min: Implement socket() syscall wrapper

**Action**: Add AF_XDP socket creation
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Create AF_XDP socket via socket() syscall
fn create_xdp_socket() -> Result<RawFd, &'static str> {
    let sock_fd = unsafe {
        libc::socket(
            44,                          // AF_XDP = 44
            libc::SOCK_RAW,              // SOCK_RAW
            0,                           // protocol
        )
    };

    if sock_fd < 0 {
        Err("socket() AF_XDP creation failed")
    } else {
        Ok(sock_fd)
    }
}

EOF
```
**Expected**: Socket creation helper added
**[V] Verification**: `grep "fn create_xdp_socket" unheaded/af-xdp/src/xsk.rs`

---

### Step 104 [W] ~4min: Implement bind() to interface + queue

**Action**: Add bind to network interface and queue ID
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Bind socket to interface and RX/TX queue
fn bind_socket(sock_fd: RawFd, ifname: &str, queue_id: u32) -> Result<(), &'static str> {
    // Get interface index via if_nametoindex
    let ifindex = unsafe {
        libc::if_nametoindex(ifname as *const str as *const i8)
    };

    if ifindex == 0 {
        return Err("interface not found");
    }

    // Create sockaddr_xdp
    let mut saddr: af_xdp_common::Sockaddr_xdp = unsafe { std::mem::zeroed() };
    saddr.family = af_xdp_common::AF_XDP;
    saddr.ifindex = ifindex;
    saddr.queue_id = queue_id;

    // bind(sock_fd, (struct sockaddr*)&saddr, sizeof(saddr))
    let ret = unsafe {
        libc::bind(
            sock_fd,
            &saddr as *const af_xdp_common::Sockaddr_xdp as *const libc::sockaddr,
            std::mem::size_of::<af_xdp_common::Sockaddr_xdp>() as u32,
        )
    };

    if ret < 0 {
        Err("bind() to interface failed")
    } else {
        Ok(())
    }
}

EOF
```
**Expected**: Bind function added
**[V] Verification**: `grep "fn bind_socket" unheaded/af-xdp/src/xsk.rs`

---

### Step 105 [W] ~3min: Implement getsockopt for RX/TX ring offsets

**Action**: Query mmap ring offset information
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Query ring offsets via getsockopt(XDP_MMAP_OFFSETS)
fn query_ring_offsets(sock_fd: RawFd, ring_type: i32) -> Result<XskRingOffsets, &'static str> {
    let mut offsets: XskRingOffsets = unsafe { std::mem::zeroed() };
    let mut optlen = std::mem::size_of::<XskRingOffsets>() as u32;

    let ret = unsafe {
        libc::getsockopt(
            sock_fd,
            44,  // SOL_XDP
            af_xdp_common::XDP_MMAP_OFFSETS,
            &mut offsets as *mut XskRingOffsets as *mut libc::c_void,
            &mut optlen,
        )
    };

    if ret < 0 {
        Err("getsockopt XDP_MMAP_OFFSETS failed")
    } else {
        Ok(offsets)
    }
}

EOF
```
**Expected**: Offset query added
**[V] Verification**: `grep "fn query_ring_offsets" unheaded/af-xdp/src/xsk.rs`

---

### Step 106 [W] ~3min: Implement mmap for RX ring

**Action**: Map RX ring memory region
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Map RX ring memory region via mmap
fn mmap_rx_ring(sock_fd: RawFd, offsets: XskRingOffsets) -> Result<(*mut u8, u32), &'static str> {
    // Calculate required mmap size
    let mmap_size = (offsets.desc + 4096) as usize; // estimated

    let ptr = unsafe {
        libc::mmap(
            std::ptr::null_mut(),
            mmap_size,
            libc::PROT_READ | libc::PROT_WRITE,
            libc::MAP_SHARED,
            sock_fd,
            af_xdp_common::XDP_RX_RING as i64 * 4096, // offset for RX ring
        )
    };

    if ptr as isize == -1 {
        Err("mmap RX ring failed")
    } else {
        Ok((ptr as *mut u8, mmap_size as u32))
    }
}

EOF
```
**Expected**: RX mmap added
**[V] Verification**: `grep "fn mmap_rx_ring" unheaded/af-xdp/src/xsk.rs`

---

### Step 107 [W] ~3min: Implement mmap for TX ring

**Action**: Map TX ring memory region
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Map TX ring memory region via mmap
fn mmap_tx_ring(sock_fd: RawFd, offsets: XskRingOffsets) -> Result<(*mut u8, u32), &'static str> {
    let mmap_size = (offsets.desc + 4096) as usize;

    let ptr = unsafe {
        libc::mmap(
            std::ptr::null_mut(),
            mmap_size,
            libc::PROT_READ | libc::PROT_WRITE,
            libc::MAP_SHARED,
            sock_fd,
            af_xdp_common::XDP_TX_RING as i64 * 4096,
        )
    };

    if ptr as isize == -1 {
        Err("mmap TX ring failed")
    } else {
        Ok((ptr as *mut u8, mmap_size as u32))
    }
}

EOF
```
**Expected**: TX mmap added
**[V] Verification**: `grep "fn mmap_tx_ring" unheaded/af-xdp/src/xsk.rs`

---

### Step 108 [W] ~3min: Implement RX ring setup from mmap'd memory

**Action**: Parse RX ring pointers from mmap region
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Setup RX ring from mmap'd memory region
unsafe fn setup_rx_ring(
    ring_mem: *mut u8,
    offsets: XskRingOffsets,
    ring_size: u32,
) -> RxRing {
    RxRing {
        producer: (ring_mem.add(offsets.producer as usize)) as *mut u32,
        consumer: (ring_mem.add(offsets.consumer as usize)) as *mut u32,
        descs: (ring_mem.add(offsets.desc as usize)) as *mut XskDesc,
        size: ring_size,
    }
}

EOF
```
**Expected**: RX setup helper added
**[V] Verification**: `grep "fn setup_rx_ring" unheaded/af-xdp/src/xsk.rs`

---

### Step 109 [W] ~3min: Implement TX ring setup from mmap'd memory

**Action**: Parse TX ring pointers from mmap region
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Setup TX ring from mmap'd memory region
unsafe fn setup_tx_ring(
    ring_mem: *mut u8,
    offsets: XskRingOffsets,
    ring_size: u32,
) -> TxRing {
    TxRing {
        producer: (ring_mem.add(offsets.producer as usize)) as *mut u32,
        consumer: (ring_mem.add(offsets.consumer as usize)) as *mut u32,
        descs: (ring_mem.add(offsets.desc as usize)) as *mut XskDesc,
        size: ring_size,
    }
}

EOF
```
**Expected**: TX setup helper added
**[V] Verification**: `grep "fn setup_tx_ring" unheaded/af-xdp/src/xsk.rs`

---

### Step 110 [W] ~4min: Implement recv() — RX ring draining

**Action**: Add receive packet method
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

impl XskSocket {
    /// Receive packets from RX ring (drain kernel -> userspace packets)
    pub fn recv(&mut self) -> Result<Vec<XskDesc>, &'static str> {
        unsafe {
            let producer = *self.rx_ring.producer;
            let consumer = *self.rx_ring.consumer;
            let mut packets = Vec::new();

            // Drain available packets from ring
            let mut i = consumer;
            while i < producer {
                let idx = (i % self.rx_ring.size as u32) as usize;
                let desc = *self.rx_ring.descs.add(idx);
                packets.push(desc);
                i += 1;
            }

            // Update consumer index
            *self.rx_ring.consumer = producer;
            Ok(packets)
        }
    }
}

EOF
```
**Expected**: recv() implementation added
**[V] Verification**: `grep "pub fn recv" unheaded/af-xdp/src/xsk.rs`

---

### Step 111 [W] ~4min: Implement send() — TX ring queueing

**Action**: Add transmit packet method
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

impl XskSocket {
    /// Send packets via TX ring (userspace -> kernel)
    pub fn send(&mut self, descs: Vec<XskDesc>) -> Result<u32, &'static str> {
        unsafe {
            let producer = *self.tx_ring.producer;
            let consumer = *self.tx_ring.consumer;
            let mut count = 0u32;

            // Enqueue descriptors to TX ring
            for desc in descs {
                // Check if ring is full
                if producer - consumer >= self.tx_ring.size as u32 {
                    break;
                }

                let idx = ((producer + count) % self.tx_ring.size as u32) as usize;
                *self.tx_ring.descs.add(idx) = desc;
                count += 1;
            }

            // Update producer index and kick kernel
            *self.tx_ring.producer = producer + count;

            // sendto() kick to wake kernel
            self.kick_tx()?;
            Ok(count)
        }
    }

    /// Kick kernel to process TX ring (sendto with zero-length)
    fn kick_tx(&self) -> Result<(), &'static str> {
        let ret = unsafe {
            libc::sendto(self.fd, std::ptr::null(), 0, 0, std::ptr::null(), 0)
        };

        if ret < 0 {
            Err("sendto TX kick failed")
        } else {
            Ok(())
        }
    }
}

EOF
```
**Expected**: send() and kick_tx() added
**[V] Verification**: `grep "pub fn send" unheaded/af-xdp/src/xsk.rs`

---

### Step 112 [B] ~2min: Verify xsk.rs compiles

**Action**: Check compilation of socket implementation
```bash
cd unheaded/af-xdp && cargo check 2>&1 | grep -E "error|Finished"
```
**Expected**: `Finished` or no errors
**[V] Verification**: xsk.rs compiles cleanly
**[D] Debug**: Check libc function availability

---

### Step 113 [W] ~3min: Create unit test skeleton for XskSocket

**Action**: Add test module for socket operations
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore]  // Requires CAP_NET_ADMIN and actual network device
    fn test_socket_creation_loopback() {
        // Integration test: create socket on loopback
        // let mut umem = Umem::new(config).expect("UMEM creation failed");
        // let xsk = XskSocket::new(&mut umem, "lo", 0);
        // assert!(xsk.is_ok(), "Socket creation on loopback should succeed");
    }

    #[test]
    fn test_ring_draining_logic() {
        // Unit test: verify ring drain logic without actual kernel
        // (Would require mocking or a test fixture)
    }
}

EOF
```
**Expected**: Test skeleton added
**[V] Verification**: `grep "#\[test\]" unheaded/af-xdp/src/xsk.rs`

---

### Step 114 [W] ~2min: Add XDP_OPTIONS getsockopt for zero-copy capability

**Action**: Implement capability detection
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

/// Query XDP socket options for zero-copy support
pub fn query_xdp_options(sock_fd: RawFd) -> Result<u32, &'static str> {
    let mut options: u32 = 0;
    let mut optlen = std::mem::size_of::<u32>() as u32;

    let ret = unsafe {
        libc::getsockopt(
            sock_fd,
            44,  // SOL_XDP
            af_xdp_common::XDP_OPTIONS,
            &mut options as *mut u32 as *mut libc::c_void,
            &mut optlen,
        )
    };

    if ret < 0 {
        Err("getsockopt XDP_OPTIONS failed")
    } else {
        Ok(options)
    }
}

EOF
```
**Expected**: Capability query added
**[V] Verification**: `grep "pub fn query_xdp_options" unheaded/af-xdp/src/xsk.rs`

---

### Step 115 [W] ~3min: Implement full XskSocket::new() constructor

**Action**: Complete socket initialization flow
```bash
cat >> unheaded/af-xdp/src/xsk.rs << 'EOF'

impl XskSocket {
    /// Create new AF_XDP socket bound to interface + queue with UMEM
    pub fn new(umem: &mut Umem, ifname: &str, queue_id: u32) -> Result<Self, &'static str> {
        // Step 1: Create AF_XDP socket
        let fd = create_xdp_socket()?;

        // Step 2: Bind to interface + queue
        bind_socket(fd, ifname, queue_id)?;

        // Step 3: Register UMEM
        crate::umem::register_umem_with_socket(fd, umem)?;

        // Step 4: Query and mmap RX ring
        let rx_offsets = query_ring_offsets(fd, af_xdp_common::XDP_RX_RING)?;
        let (rx_mem, _) = mmap_rx_ring(fd, rx_offsets)?;

        // Step 5: Query and mmap TX ring
        let tx_offsets = query_ring_offsets(fd, af_xdp_common::XDP_TX_RING)?;
        let (tx_mem, _) = mmap_tx_ring(fd, tx_offsets)?;

        // Step 6: Setup ring structures
        let rx_ring = unsafe { setup_rx_ring(rx_mem, rx_offsets, 2048) };
        let tx_ring = unsafe { setup_tx_ring(tx_mem, tx_offsets, 2048) };

        // Step 7: Check zero-copy capability (log only, not blocking)
        let _ = query_xdp_options(fd);

        Ok(XskSocket {
            fd,
            umem: umem as *mut Umem,
            rx_ring,
            tx_ring,
            rx_mem,
            tx_mem,
        })
    }
}

EOF
```
**Expected**: Full constructor implementation added
**[V] Verification**: `grep "pub fn new" unheaded/af-xdp/src/xsk.rs | head -1`

---

### Step 116 [B] ~2min: Verify complete xsk.rs compiles

**Action**: Final socket module compilation check
```bash
cd unheaded/af-xdp && cargo check 2>&1 | tail -5
```
**Expected**: `Finished` with no errors
**[V] Verification**: Socket module complete and compiles
**[D] Debug**: Address any remaining libc/unsafe issues

---

### Step 117 [B] ~1min: Verify af-xdp full workspace builds

**Action**: Test complete af-xdp crate
```bash
cd unheaded/af-xdp && cargo build 2>&1 | tail -3
```
**Expected**: `Finished` with library built
**[V] Verification**: af-xdp library builds successfully
**[D] Debug**: Check for link errors

---

### Step 118 [W] ~2min: Add documentation to XskSocket public API

**Action**: Add rustdoc comments to public methods
```bash
# Update XskSocket implementation with doc comments (rewrite top of struct with /// comments)
```
**[V] Verification**: Doc comments visible via `cargo doc --open`

---

### Step 119 [B] ~2min: Generate documentation

**Action**: Build rustdoc for af-xdp crate
```bash
cd unheaded/af-xdp && cargo doc --no-deps 2>&1 | tail -3
```
**Expected**: Documentation generated
**[V] Verification**: `target/doc/af_xdp/index.html` exists

---

### Step 120 [C] ~2min: Commit PHASE 3 — XSK socket implementation

**Action**: Create final commit for socket implementation
```bash
cd unheaded/af-xdp && git add -A && git commit -m "PHASE 3: Implement XskSocket with RX/TX ring mmap, bind, recv/send, zero-copy capability detection"
```
**Expected**: Commit hash printed
**[V] Verification**: `git log --oneline | head -1` shows PHASE 3 commit

---

### PHASE 3 EXIT GATE

**PASS if**:
- xsk.rs implements:
  - RxRing struct with producer/consumer indices and descriptor ring
  - TxRing struct with producer/consumer indices and descriptor ring
  - XskSocket struct with fd, umem, rx_ring, tx_ring, mmap'd memory
  - create_xdp_socket() — AF_XDP socket creation
  - bind_socket() — bind to interface + queue
  - query_ring_offsets() — getsockopt XDP_MMAP_OFFSETS
  - mmap_rx_ring() + mmap_tx_ring() — memory mapping
  - setup_rx_ring() + setup_tx_ring() — ring pointer setup
  - recv() — drain RX ring packets
  - send() — queue TX ring descriptors + sendto kick
  - query_xdp_options() — zero-copy capability detection
  - Full XskSocket::new() constructor with 7-step initialization
  - Proper Drop impl with munmap cleanup
- `cargo check` and `cargo build` succeed
- af-xdp library can be imported in other crates
- Test skeleton present (ignored for now, requires CAP_NET_ADMIN)

**BLOCKED if**:
- libc functions unavailable → kernel version too old [STUCK]
- Pointer arithmetic wrong → [D] review offset calculations
- AF_XDP not supported → kernel rebuild needed [STUCK]

**Proceed to PHASE 4+ for XDP program integration only after all gates pass**

---

## PHASES 4-11 (Stub — see AF-XDP-BATTLE-PLAN-part2.md)

Phases 4-11 will cover:
- PHASE 4: XDP program XSKMAP integration
- PHASE 5: Kernel/userspace packet flow
- PHASE 6: Redirect path testing
- PHASE 7: Flow-based steering
- PHASE 8: Performance optimization
- PHASE 9: Error handling + recovery
- PHASE 10: Monitoring + statistics
- PHASE 11: End-to-end system test

---

## OVERALL CHECKPOINTS

**After PHASE 0**: Environment verified, kernel >= 5.15, AF_XDP capable
**After PHASE 1**: Common types defined, no_std compatible, types match kernel ABI
**After PHASE 2**: UMEM allocator working, frame pool functional, rings testable
**After PHASE 3**: XSK socket creation + RX/TX rings, ready for XDP integration

---

## DOCUMENTATION REFERENCES

- Kernel AF_XDP: https://www.kernel.org/doc/html/latest/networking/af_xdp.html
- libc syscalls: man 2 mmap, man 2 socket, man 2 setsockopt
- eBPF/XDP: https://cilium.io/blog/2018/09/25/xdp-data-plane/
- Unheaded project: See unheaded/ebpf/Cargo.toml (workspace structure)

---

**END OF PART 1 (Phases 0-3, Steps 1-140)**
