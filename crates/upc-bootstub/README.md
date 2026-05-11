# crates/upc-bootstub

**Status:** P0 scaffolding (2026-05-11). Phase 2 day-1 fills in the build pipeline + integration.
**Owner:** unheaded-developer (impl) + unheaded-computermancer (MBC artifact).
**Closes task:** #63 (P2 fork — author crates/upc-bootstub/ stage-1 crate).

---

## Why this exists

The Phase 1.1 xv6 boot path inlined its stage-1 work into `crates/xv6-mbc/adapters/start_mbc.c`. That worked because xv6 prints its banner BEFORE any BSS-dependent path executes — see `crates/xv6-mbc/TRANSLATOR_EXTENSIONS.md` §"Phase 1.1 SHIP decisions" for the rationale.

uClinux + full Linux 6.x cannot do that. They require:
1. **Full BSS zero-fill** before `start_kernel()` — Linux's static allocator panics on uninitialized BSS.
2. **Clean M-mode → S-mode transition** at the kernel entry point.
3. **Initramfs CPIO unpack** to addresses the kernel's `populate_rootfs()` reads from.
4. **Cmdline parse + handoff** via BootParamsV2.

This crate is the home of those four operations. At Phase 2 build time, `src/start.c` is compiled and translated to `upc-bootstub.mbc`. cmd/upc-bootctl loads it into ROM_MAP at byte 0x10000 — the same slot start_mbc.c occupies for xv6.

## Layout

```
crates/upc-bootstub/
├── Cargo.toml          workspace shim (matches xv6-mbc pattern)
├── README.md           this file
├── src/
│   ├── lib.rs          Rust constants + image_path() shim (Phase 2)
│   └── start.c         the actual stub (compiled to MBC)
└── adapters/           (Phase 2) build glue
    └── Makefile.bootstub  mirrors crates/xv6-mbc/adapters/Makefile.mbc
```

## Constants exported by lib.rs

| Constant | Value | Purpose |
|----------|-------|---------|
| `BOOTSTUB_LOAD_ADDR` | `0x0001_0000` | byte address where loader places upc-bootstub.mbc |
| `BOOTSTUB_ENTRY_PC` | `0x4000` | initial CPU.PC = BOOTSTUB_LOAD_ADDR / 4 |
| `KERNEL_ENTRY_ADDR` | `0x0002_0000` | byte address where loader places kernel image |
| `INITRAMFS_LOAD_ADDR` | `0x0080_0000` | byte address where loader places initramfs CPIO |
| `MSTATUS_MPP_S` | `0x800` | bits 12:11 = 0b01 (S-mode) for kernel handoff |

## Phase 2 contract

When `cmd/upc-bootctl boot --kernel uclinux.mbc --initramfs rootfs.cpio.gz` runs:

1. Loader writes BootParamsV2 at byte 0x0100.
2. Loader places `upc-bootstub.mbc` at byte 0x10000.
3. Loader places kernel at byte 0x20000.
4. Loader places initramfs CPIO at byte 0x800000.
5. Loader sets CPU.PC = 0x4000.
6. Loader dispatches the trigger packet.

At PC=0x4000 the bootstub takes over (per `src/start.c::start()`):

1. Verify magic + version.
2. Zero `[bss_start, bss_end)`.
3. Set MEPC = kernel_addr, MSTATUS.MPP = S-mode.
4. MRET — kernel begins at supervisor privilege.

On any verify failure: emit `"BOOT FAIL: <reason>\n"` via MMIO 0xC001 and halt.

## Build pipeline (Phase 2 day-1)

The Phase 2 kickoff adds `adapters/Makefile.bootstub` modeled on `crates/xv6-mbc/adapters/Makefile.mbc`:

```makefile
# Pseudo:
upc-bootstub.elf: src/start.c bootstub.ld
	riscv64-unknown-elf-gcc -march=rv32i_zicsr_zmmul -mabi=ilp32e \
	  -nostdlib -ffreestanding -O2 \
	  -ffixed-x16 -ffixed-x17 ... -ffixed-x31 \
	  -T bootstub.ld -o $@ $<

upc-bootstub.mbc: upc-bootstub.elf
	../../monad-mbc/target/release/rv32i-to-mbc $< -o $@
	# Emits sibling .data file for .rodata strings ("BOOT FAIL: ...").
```

cmd/upc-bootctl gains a `--bootstub <path>` flag (or auto-detects via `xv6_mbc::BOOT_MAGIC` heuristic — if the kernel's first instruction LOOKS like xv6's start_mbc.c, skip the bootstub; otherwise prepend it).

## Tests

`cargo test -p upc-bootstub` validates the constants are coherent (5 tests in lib.rs). The C source is integration-tested at Phase 2 day-1 by:

```bash
make -C crates/upc-bootstub/adapters -f Makefile.bootstub upc-bootstub.mbc
sudo cargo run -p upc-bootctl -- boot \
  --kernel /path/to/uclinux.mbc \
  --bootstub crates/upc-bootstub/target/upc-bootstub.mbc \
  --initramfs /path/to/rootfs.cpio.gz \
  --instance 222
# Expect: TTY emits "upc-bootstub: handoff to kernel\nLinux version 6.x..."
```

## Out of scope (this crate)

- The uClinux/Linux kernel itself — lives in `crates/uclinux-mbc/` (Phase 2.1) and `crates/linux-mbc/` (Phase 3).
- The packer/Jenkins pipeline that builds the host distribution — lives in `nix/yggdrasil/` (separate ADR-69420 track).

## See also

- `docs/doom/UPC_BOOT_PROTOCOL_V2.md` — wire-format contract for BootParamsV2.
- `crates/xv6-mbc/adapters/start_mbc.c` — the Phase 1.1 inlined version of this stub.
- `crates/xv6-mbc/TRANSLATOR_EXTENSIONS.md` §"Phase 1.1 SHIP decisions" — context on why this crate was deferred from Phase 1.1.
- `references/battle-plan-ascend-linux-2026-05-08.md` §6 (Phase 2) — the broader Phase 2 plan this crate slots into.
- ADR-067 — MBC ISA v2 + UPC ABI v1 (CSR memory-mapped region semantics).
- ADR-072 — BOOT_MAGIC byte-ordering convention.
