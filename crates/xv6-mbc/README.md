# crates/xv6-mbc — xv6-riscv on UPC

**Status:** ASCEND-LINUX Phase 1 (in-progress).
**Source plan:** `references/battle-plan-ascend-linux-2026-05-08.md` Phase 1.

This crate hosts the xv6-riscv kernel ported to MBC ISA so it boots on the
UPC. xv6 is the L5 MiniKernel rung of the Dream Ladder (`docs/doom/ROAD_TO_LINUX.md`).

## Layout

```
crates/xv6-mbc/
├── Cargo.toml          — workspace member; thin Rust shim
├── README.md           — this file
├── src/
│   └── lib.rs          — image_path() + boot magic re-export
├── upstream/           — vendored xv6-riscv (MIT, NOT YET vendored)
├── adapters/           — MBC-native replacements (NOT YET written)
│   ├── start_mbc.c     — replaces upstream/kernel/start.c (M-mode boot)
│   ├── console-mmio.c  — replaces upstream/kernel/uart.c
│   └── blk-ramdisk.c   — replaces upstream/kernel/virtio_disk.c
└── scripts/
    └── vendor-xv6.sh   — clone xv6-riscv from MIT-PDOS upstream
```

## Build flow (target shape)

```bash
# 1. Vendor xv6-riscv (one-time, requires network):
bash crates/xv6-mbc/scripts/vendor-xv6.sh

# 2. Update THIRD_PARTY.md with the pinned commit hash.

# 3. Replace platform files in upstream/kernel/ with adapters/:
#    - upstream/kernel/start.c     ← adapters/start_mbc.c
#    - upstream/kernel/uart.c      ← adapters/console-mmio.c
#    - upstream/kernel/virtio_disk.c ← adapters/blk-ramdisk.c

# 4. Build:
cd crates/xv6-mbc/upstream
make ARCH=riscv32-mbc          # patched Makefile uses our toolchain
                               # produces kernel/kernel.elf

# 5. Translate RV32I → MBC:
rv32i-to-mbc upstream/kernel/kernel.elf -o target/xv6-mbc.mbc

# 6. Boot:
cargo run --bin doom-runner -- --kernel target/xv6-mbc.mbc
# Expected (Phase 1.1 milestone):
#   "xv6 booting..."
#   HALT
# Phase 1.5 milestone: full shell with ls/cat/echo/uname/ps responding <50 ms.
```

## Adapter strategy

Three xv6 platform files are replaced (not patched) with MBC-native
implementations. This minimizes upstream drift and makes it obvious where
"kingdom code" begins:

| xv6 file | Replacement | Why |
|----------|-------------|-----|
| `kernel/start.c` | `adapters/start_mbc.c` | xv6's `start()` runs in M-mode, sets up MEPC + MTVEC + Sv32 paging, then MRETs to S-mode. We replace with an MBC-aware version that reads BootParams v2 (per `docs/doom/UPC_BOOT_PROTOCOL_V2.md`) and uses our memory-mapped CSR region. |
| `kernel/uart.c` | `adapters/console-mmio.c` | xv6's UART driver hits MMIO 0x10000000 (QEMU virt machine). Replace with writes to MMIO 0xC001 (UPC convention per `docs/doom/UPC_OS_PRIMITIVES.md` L4f). |
| `kernel/virtio_disk.c` | `adapters/blk-ramdisk.c` | xv6's virtio block driver targets QEMU virtio-blk. Replace with calls to SYS_READ_BLOCK (200) / SYS_WRITE_BLOCK (201) (UPC L4e). |

Other xv6 platform files (`kernel/vm.c`, `kernel/proc.c`, `kernel/trap.c`)
are PATCHED in-place to call our L4 syscalls (SYS_FORK, SYS_EXECVE,
SYS_SET_PAGE_DIR, SYS_FLUSH_TLB) instead of inline assembly. Patches
live in `adapters/patches/` (NOT YET written).

## Phase 1 milestones

Per `references/battle-plan-ascend-linux-2026-05-08.md` §4:

| Sub-phase | Goal | Commit gate |
|-----------|------|-------------|
| 1.1 | xv6 source bring-up | `cargo run -- --kernel xv6-mbc.mbc` prints "xv6 booting..." and HALTs |
| 1.2 | Page tables under MMU | dummy user mmap → write → read round-trip |
| 1.3 | Process model | init forks sh; `ps` shows 2 processes |
| 1.4 | Filesystem + ramdisk | kernel mounts ramdisk; `ls /` returns entries |
| 1.5 | Shell + 5 commands + WebSocket xterm | Mode A browser + Mode B host pty both work |
| 1.6 | Phase 1 verification gate | ADR-068 lands; Marshal handoff to Phase 2 |

## License

This crate (Cargo.toml + src/ + adapters/ + scripts/) is GPL-3.0-or-later.
Vendored xv6-riscv at upstream/ is MIT (preserved verbatim; LICENSE file
copied from upstream).
