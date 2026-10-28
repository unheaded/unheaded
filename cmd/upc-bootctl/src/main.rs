// SPDX-License-Identifier: GPL-3.0-or-later
//
// upc-bootctl — UPC kernel image loader + boot dispatcher.
//
// ASCEND-LINUX Phase 1.1 deliverable per
// `references/battle-plan-ascend-linux-2026-05-08.md`.
//
// Loads a translated MBC kernel image (xv6-mbc.mbc, uclinux-mbc.mbc, etc.),
// validates it, prepares BootParams v3, and dispatches a boot trigger to
// the UPC instance.
//
// **Status**: skeleton with `validate` and `boot --dry-run` subcommands.
// Full BPF map population + dispatcher to come in next shift's `boot` impl.

use anyhow::{anyhow, bail, Context, Result};
use clap::{Parser, Subcommand};
use std::collections::VecDeque;
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc, Arc};

mod bootparams;
mod freeze;
mod geometry;
mod netns;
mod runner;
mod rv2mbc;
mod tty_bridge;

#[derive(Parser, Debug)]
#[command(name = "upc-bootctl", version, about)]
struct Cli {
    #[command(subcommand)]
    cmd: Cmd,
}

#[derive(Subcommand, Debug)]
enum Cmd {
    /// Validate a kernel image without loading it.
    Validate {
        /// Path to the .mbc kernel image.
        #[arg(long)]
        kernel: PathBuf,
    },

    /// Load + boot a kernel image into the UPC.
    Boot {
        /// Path to the .mbc kernel image.
        #[arg(long)]
        kernel: PathBuf,

        /// Path to the optional Phase 2 stage-1 stub (upc-bootstub.mbc).
        /// When provided, the stub is loaded into ROM_MAP at byte 0x10000
        /// ahead of the kernel; CPU.PC starts at 0x4000 (= the stub's
        /// entry). When absent (Phase 1.1 xv6 path), the kernel image is
        /// expected to inline its own start code at byte 0x10000.
        #[arg(long)]
        bootstub: Option<PathBuf>,

        /// Path to the optional ramdisk (CPIO+gzip initramfs).
        #[arg(long)]
        initramfs: Option<PathBuf>,

        /// Path to the optional xv6-format ramdisk image (built by
        /// `crates/xv6-mbc/upstream/mkfs/mkfs`). Loaded into RAM_MAP at
        /// byte 0x00800000 per docs/doom/UPC_PAGE_TABLE_LAYOUT.md.
        /// Phase 1.4 IMPL — distinct from --initramfs (Linux CPIO).
        #[arg(long)]
        ramdisk: Option<PathBuf>,

        /// Path to the userland MBC image (e.g. init.mbc). When provided,
        /// the .mbc / .rv2mbc / .data sidecars are loaded into a dedicated
        /// user region of ROM_MAP / RV2MBC_MAP / RAM_MAP so xv6's kexec
        /// can drop the process into user mode without needing a real
        /// ELF loader. Phase 1.5 IMPL (ASCEND-LINUX userland-spike).
        ///
        /// Layout (must match crates/xv6-mbc/upstream/user/user.ld which
        /// links userland at RV byte 0x0):
        ///   - .mbc       → ROM_MAP slot 0x4000+ (USER_ROM_BASE).
        ///   - .rv2mbc    → RV2MBC_MAP slot 0 onward, with each entry shifted
        ///     by USER_ROM_BASE so SRET(SEPC=0) lands on the user MBC PC.
        ///   - .data      → RAM_MAP at each record's byte address.
        ///
        /// The kernel side (crates/xv6-mbc/upstream/kernel/exec.c) sees
        /// the same path through namei() and sets trapframe->epc = 0.
        #[arg(long)]
        userland: Option<PathBuf>,

        /// Number of Monad trigger packets to send post-dispatch. Each
        /// packet advances the CPU by up to 16 MBC instructions. Default
        /// 500 (~4000 insns, enough for Phase 1.1 banner+main()). Bump
        /// to 5000+ for Phase 1.4 work that needs the kernel to advance
        /// further (scheduler entry, userinit, FS mount).
        #[arg(long, default_value_t = 500)]
        triggers: u32,

        /// UPC instance ID (low byte of CPU_MAP key).
        #[arg(long, default_value_t = 0xDE)]
        instance: u8,

        /// Dry-run — print what we'd do but don't touch BPF maps.
        #[arg(long)]
        dry_run: bool,

        /// Scripted console input (Gate C). Up to 64 bytes are pre-loaded into
        /// the KBD ring before the CPU runs, so the guest's read(5) drain feeds
        /// them to sh's gets() as if typed. Use $'echo\n' to terminate a line.
        /// read does NOT echo (acceptable for a scripted demo). Errors if the
        /// input exceeds the 64-slot ring (no silent truncation).
        #[arg(long)]
        input: Option<String>,

        /// Test hook (S-ASCEND-2 Step 111): read UPC_SLICE_STRIDE and declare
        /// that slice stride to the loader's geometry gate instead of the
        /// shipped one. Prints a warning. The gate refuses any stride the
        /// object does not ship, so this can only make the boot refuse.
        #[arg(long)]
        unsafe_geometry: bool,

        /// Old setup (needs root): create `upc0` + the veth pair from the root
        /// namespace and send triggers through `ip netns exec`. Default: run
        /// inside the namespace `scripts/upc-netns.sh up` made.
        #[arg(long)]
        create_netns: bool,

        /// Expected SHA-256 (64 hex digits) of the kernel's `.rv2mbc`. Else
        /// `UPC_RV2MBC_SHA`, else the `<kernel>.rv2mbc.sha256` sidecar; with
        /// none of them the boot is refused (S-ASCEND-2 Item 5). The userland
        /// and resident programs always use their sidecars.
        #[arg(long, value_name = "HEX")]
        rv2mbc_sha: Option<String>,

        /// Lab-only: continue when a `.rv2mbc` has no expected SHA-256 (prints
        /// a warning). Accepted only with --dry-run; never excuses a mismatch.
        #[arg(long)]
        allow_unverified_rv2mbc: bool,

        /// Test hook (S-ASCEND-2 Step 174): after the run, send N ticks with a
        /// wrong nonce, then N with the owner's, and print how far each moved
        /// the CPU and the hijack counter. The harness never sets it.
        #[arg(long, value_name = "N")]
        hijack_probe: Option<u32>,

        /// Test hook (S-ASCEND-2 Step 162): after ROM_MAP and RV2MBC_MAP are
        /// frozen, try one userspace update of each through the same handle and
        /// require EPERM (`freeze check: <MAP> update refused (EPERM)`); any
        /// other result fails the boot before attach.
        #[arg(long)]
        freeze_check: bool,
    },

    /// Boot a kernel and run it interactively (Mode A/B): TTY output streams
    /// to stdout (and the tty-bridge, if running); keystrokes from the bridge
    /// (browser) and/or stdin are appended live to the guest's KBD ring.
    /// Runs until Ctrl-C / SIGTERM, or with --stdin until stdin closes and
    /// the guest goes idle. Tears down XDP + netns on exit, like `boot`.
    Console {
        /// Kernel MBC image (e.g. xv6-mbc.mbc).
        #[arg(long)]
        kernel: PathBuf,

        /// Optional stage-1 bootstub (see `boot`).
        #[arg(long)]
        bootstub: Option<PathBuf>,

        /// xv6-format ramdisk (fs.img) at byte 0x00800000 (see `boot`).
        #[arg(long)]
        ramdisk: Option<PathBuf>,

        /// Userland MBC image (e.g. init.mbc) (see `boot`).
        #[arg(long)]
        userland: Option<PathBuf>,

        /// UPC instance ID.
        #[arg(long, default_value_t = 0xDE)]
        instance: u8,

        /// Also read keystrokes from this process's stdin (raw bytes; a
        /// terminal in canonical mode delivers them a line at a time).
        #[arg(long)]
        stdin: bool,

        /// Trigger packets per loop iteration (each = up to 16 MBC insns).
        #[arg(long, default_value_t = 50_000)]
        tick_batch: u32,

        /// Print KBD_HEAD / KBD_TAIL / pending bytes after every batch.
        #[arg(long)]
        debug_kbd: bool,

        /// Test hook: honour UPC_SLICE_STRIDE (see `boot --unsafe-geometry`).
        #[arg(long)]
        unsafe_geometry: bool,

        /// Old root-namespace setup (see `boot --create-netns`).
        #[arg(long)]
        create_netns: bool,

        /// Expected SHA-256 of the kernel's `.rv2mbc` (see `boot --rv2mbc-sha`).
        #[arg(long, value_name = "HEX")]
        rv2mbc_sha: Option<String>,
    },
}

/// FNV-1a 32-bit hash of a program basename. MUST stay bit-identical to
/// `pt::name_hash` in `monad-cpu-ebpf` so `exec(7)` resolves a path to the
/// PROGRAM_TABLE slot the loader wrote (ADR-077 Gate B).
fn fnv1a(name: &[u8]) -> u32 {
    let mut h: u32 = 0x811c_9dc5;
    for &b in name {
        h ^= b as u32;
        h = h.wrapping_mul(0x0100_0193);
    }
    h
}

/// MBC opcode for CALL (absolute call with a 24-bit immediate target).
const OP_CALL: u32 = 0x27;

/// Relocate a CALL instruction's 24-bit immediate by `rom_base`, wrapping within
/// 24 bits; any non-CALL word passes through unchanged.
///
/// The MBC translator emits CALL targets relative to the `.mbc`'s slot 0, so a
/// program loaded at a non-zero ROM base needs every CALL immediate shifted or
/// the call lands inside another image. JMP/JZ/JNZ/... are NOT relocated: they
/// use signed PC-relative offsets that survive a uniform shift. Pure so the
/// loader's core relocation is unit-tested off-target.
#[inline]
fn relocate_call_word(word: u32, rom_base: u32) -> u32 {
    if (word >> 24) == OP_CALL {
        (OP_CALL << 24) | ((word & 0x00FF_FFFF).wrapping_add(rom_base) & 0x00FF_FFFF)
    } else {
        word
    }
}

/// Shift an `.rv2mbc` entry (an MBC PC / ROM word index) by `rom_base` so a
/// program loaded at a non-zero ROM base resolves its indirect branches
/// (JMPR/CALLR) and privileged returns (SRET on SEPC=0) into its own ROM
/// region. Pure so the loader's rv2mbc relocation is unit-tested off-target.
///
/// NOTE: this shifts EVERY entry, including a `0` (which RV2MBC_MAP treats as
/// "unmapped"). At a non-zero base a `0` entry becomes `rom_base`, i.e. non-zero.
/// This matches the existing loader behavior and is harmless because the
/// `.rv2mbc` files carry no spurious `0` in the reachable RV-word range — flagged
/// for a future review, NOT changed here (this is a characterization refactor).
#[inline]
fn relocate_rv2mbc_entry(entry: u32, rom_base: u32) -> u32 {
    entry.wrapping_add(rom_base)
}

/// The xv6 syscall surface (contract part 4, ADR-080): exactly the ecall numbers
/// the `ascend-linux` interpreter build has an arm for
/// (`ebpf/monad-cpu-ebpf/surface.rs::SURFACE_ASCEND`). Numbers are xv6's
/// `kernel/syscall.h` plus the UPC block-device calls 200/201
/// (`monad_common::mbc_linux_syscalls`). xv6's pipe 4, kill 6, chdir 9, sbrk 12,
/// uptime 14, unlink 18, link 19 and mkdir 20 are absent because the object has
/// no arm for them (they fall through to "silently ignore"). Gate A refuses an
/// object whose `UPC_SURFACE_ID` is not the id of this list.
static XV6_SYSCALLS: [upc_api::SyscallDesc; 15] = [
    upc_api::SyscallDesc {
        number: 1,
        name: "fork",
    },
    upc_api::SyscallDesc {
        number: 2,
        name: "exit",
    },
    upc_api::SyscallDesc {
        number: 3,
        name: "wait",
    },
    upc_api::SyscallDesc {
        number: 5,
        name: "read",
    },
    upc_api::SyscallDesc {
        number: 7,
        name: "exec",
    },
    upc_api::SyscallDesc {
        number: 8,
        name: "fstat",
    },
    upc_api::SyscallDesc {
        number: 10,
        name: "dup",
    },
    upc_api::SyscallDesc {
        number: 11,
        name: "getpid",
    },
    upc_api::SyscallDesc {
        number: 13,
        name: "pause",
    },
    upc_api::SyscallDesc {
        number: 15,
        name: "open",
    },
    upc_api::SyscallDesc {
        number: 16,
        name: "write",
    },
    upc_api::SyscallDesc {
        number: 17,
        name: "mknod",
    },
    upc_api::SyscallDesc {
        number: 21,
        name: "close",
    },
    upc_api::SyscallDesc {
        number: 200,
        name: "read_block",
    },
    upc_api::SyscallDesc {
        number: 201,
        name: "write_block",
    },
];

/// The xv6 / ASCEND-LINUX surface descriptor: the `ascend-linux`-gated syscall
/// allowlist shared by init and every resident program.
static XV6_SURFACE: upc_api::SyscallSurface = upc_api::SyscallSurface {
    name: "xv6",
    feature_gate: upc_api::FeatureGate::AscendLinux,
    syscalls: &XV6_SYSCALLS,
};

/// The BPF-map capacities `upc_api::load` validates a Workload against. Values
/// come from `monad_common::mbc_maps` (the single source of truth) at their
/// `*_LARGE` maxima: the host can't know whether the loaded object was built with
/// the `large-image` feature, so it guards against the larger ceiling and relies
/// on `aya::Array::set` for the actual per-build bound (see the mbc_maps docs).
fn xv6_map_capacities() -> upc_api::MapCapacities {
    upc_api::MapCapacities {
        rom_words: monad_common::mbc_maps::ROM_MAP_WORDS_LARGE,
        rv2mbc_entries: monad_common::mbc_maps::RV2MBC_ENTRIES_LARGE,
        ram_words: monad_common::mbc_maps::RAM_MAP_WORDS,
    }
}

/// Build a `Workload` for one xv6 userland program (init or a resident program):
/// an already-relocated MBC image at `rom_start_slot`, its shifted `.rv2mbc`
/// entries at `rv2mbc_word_base`, and the entry slot (for the `EntryOutOfRange`
/// check). Memory model is per-pid MMU with `mmu` as its slice layout, boot is
/// Direct (no bootstub), and the surface is the shared xv6 allowlist.
/// `rv2mbc_sha256` is the hash `rv2mbc::check_all` verified over the program's
/// `.rv2mbc` file before relocation.
#[allow(clippy::too_many_arguments)]
fn xv6_program_workload(
    name: &str,
    rom: Vec<u32>,
    rom_start_slot: u32,
    rv2mbc: Vec<u32>,
    rv2mbc_word_base: u32,
    entry: u32,
    mmu: &upc_api::MmuLayout,
    rv2mbc_sha256: Option<String>,
) -> upc_api::Workload {
    upc_api::Workload {
        name: name.to_string(),
        image: upc_api::GuestImage {
            rom,
            rom_start_slot,
            rv2mbc,
            rv2mbc_word_base,
            expected_rv2mbc_sha256: rv2mbc_sha256,
            entry: upc_api::MbcPc(entry),
        },
        memory: upc_api::MemoryModel::PerPidMmu,
        surface: XV6_SURFACE,
        boot: upc_api::BootProtocol::Direct,
        mmu: Some(mmu.clone()),
    }
}

/// Gates A and B on the object file, before it is opened: `aya` creates every
/// map when it loads an object, so refusing here means a wrong object leaves
/// no map, program or netns behind. The same checks run again inside every
/// `upc_api::load`. Returns the facts for those calls.
fn preflight_object(ebpf_obj: &Path, mmu: &upc_api::MmuLayout) -> Result<upc_api::ObjectFacts> {
    let facts = upc_api::ObjectFacts::from_path(ebpf_obj)
        .map_err(|e| anyhow!("refusing to load {}: {e}", ebpf_obj.display()))?;
    upc_api::check_surface(&XV6_SURFACE, &facts)
        .and_then(|()| {
            upc_api::check_mmu(
                upc_api::MemoryModel::PerPidMmu,
                Some(mmu),
                &xv6_map_capacities(),
                &facts,
            )
        })
        .map_err(|e| anyhow!("refusing to load {}: {e}", ebpf_obj.display()))?;
    println!(
        "  ✓ object gates: surface id {} (xv6, {} syscalls) and slice geometry match {}",
        &upc_api::object::hex32(&facts.surface_id)[..16],
        XV6_SYSCALLS.len(),
        ebpf_obj.display()
    );
    Ok(facts)
}

/// Pre-translate a resident userland program (`sh`, `ls`, …) into a disjoint
/// `ROM_MAP` region + a disjoint `RV2MBC_MAP` base, stage its `.data` in a
/// pristine RAM region, and record an 8-word PROGRAM_TABLE row (ADR-077 Gate
/// B). Mirrors the `--userland` (init) load path, but at non-zero bases so
/// init and the program coexist in the shared maps. `name` is the basename
/// `exec` will hash to find this slot.
#[allow(clippy::too_many_arguments)]
fn load_resident_program(
    runner: &mut runner::BootRunner,
    facts: &upc_api::ObjectFacts,
    mmu: &upc_api::MmuLayout,
    mbc_path: &Path,
    rv: &rv2mbc::CheckedRv2mbc,
    name: &str,
    slot: u32,
    rom_base: u32,
    rv2mbc_base: u32,
    data_stage_va: u32,
) -> Result<()> {
    // A program's ROM must fit ROM_MAP (see monad-cpu-ebpf `ROM_MAP`). Its
    // capacity is compile-time-feature-dependent (default vs `large-image`,
    // ADR-081); upc-bootctl (host) can't know which feature the loaded object was
    // built with, so this early check guards against the MAXIMUM and relies on
    // populate_rom_at (aya `Array::set`) to enforce the actual per-build bound —
    // otherwise a stale guard would wrongly reject a valid large image on a
    // large-image build, defeating the feature. Value from the single source of
    // truth so it can't drift from the eBPF map declaration. (The old RET
    // MBC-vs-RV floor guard is gone — ADR-079 tags return addresses.)
    const ROM_MAP_WORDS_MAX: u32 = monad_common::mbc_maps::ROM_MAP_WORDS_LARGE;
    let bytes = std::fs::read(mbc_path).with_context(|| format!("read {}", mbc_path.display()))?;
    if !bytes.len().is_multiple_of(4) {
        bail!("{} not 4-byte aligned", mbc_path.display());
    }
    let rom_end = rom_base as u64 + (bytes.len() as u64 / 4);
    if rom_end > ROM_MAP_WORDS_MAX as u64 {
        bail!(
            "{name}: ROM [0x{rom_base:X}, 0x{rom_end:X}) overflows ROM_MAP \
             (max 0x{ROM_MAP_WORDS_MAX:X} words)"
        );
    }
    // Patch CALL immediates by rom_base, exactly like the init path.
    let words: Vec<u32> = bytes
        .as_chunks::<4>()
        .0
        .iter()
        .map(|c| relocate_call_word(u32::from_le_bytes(*c), rom_base))
        .collect();

    // rv2mbc: values shifted by rom_base (RV word → that program's ROM slot);
    // loaded at index base rv2mbc_base so each program owns a disjoint region.
    // The bytes were read and SHA-checked by rv2mbc::check_all.
    if !rv.bytes.len().is_multiple_of(4) {
        bail!("{} not 4-byte aligned", rv.path.display());
    }
    let rv_entries: Vec<u32> = rv
        .bytes
        .as_chunks::<4>()
        .0
        .iter()
        .map(|c| relocate_rv2mbc_entry(u32::from_le_bytes(*c), rom_base))
        .collect();

    // Route ROM + RV2MBC population through the shared loader (Epic 1.2.3):
    // validate the descriptor (image bounds, entry-in-ROM, every rv2mbc target
    // in ROM) then write both maps. entry = rom_base (user.ld links _start at
    // RV word 0 → ROM slot rom_base). Byte-identical to the prior
    // populate_rom_at + populate_rv2mbc calls. write_image, not load: the
    // freeze comes once, in boot_prepare, after the last image.
    let workload = xv6_program_workload(
        name,
        words,
        rom_base,
        rv_entries,
        rv2mbc_base,
        rom_base,
        mmu,
        rv.sha_hex(),
    );
    upc_api::write_image(&workload, &xv6_map_capacities(), facts, runner)
        .map_err(|e| anyhow!("{name} image load: {e}"))?;

    // Stage .data flat for exec to re-copy into the running pid's slice.
    let data_path = mbc_path.with_extension("data");
    let (data_dst_va, data_len_words) = if data_path.exists() {
        runner.stage_program_data(&data_path, data_stage_va)?
    } else {
        (0, 0)
    };

    // entry = RV word 0 (user.ld links _start at RV byte 0) → ROM slot rom_base.
    let row: [u32; 8] = [
        fnv1a(name.as_bytes()),
        rom_base,      // entry_mbc
        rv2mbc_base,   // rv2mbc_base
        data_dst_va,   // data_dst_va
        data_stage_va, // data_src_va
        data_len_words,
        1, // valid
        0,
    ];
    runner.populate_program_table(slot, row)?;
    println!(
        "  Program[{slot}] '{name}': ROM 0x{rom_base:04X} rv2mbc_base 0x{rv2mbc_base:04X} \
         entry 0x{rom_base:04X} data {} words @ VA 0x{data_dst_va:06X} (staged 0x{data_stage_va:06X}) hash 0x{:08X}",
        data_len_words,
        fnv1a(name.as_bytes())
    );
    Ok(())
}

/// Validate that a kernel image's byte buffer is 4-byte aligned and return the
/// instruction count. Pure function — no I/O — so it is unit-testable.
///
/// Errors with the byte-count in the message when the buffer length is not a
/// multiple of 4 (an MBC instruction is always one 32-bit word).
/// S-ASCEND-2 Item 4 (the 2026-05-13 isolation brief): a live boot attaches
/// XDP and writes BPF maps, so it needs an explicit `UPC_DEMO_MODE=1`. Runs
/// before anything touches BPF or a netns. A dry run never needs it.
fn demo_gate(dry_run: bool, demo_mode: Option<&str>) -> Result<()> {
    if dry_run || demo_mode == Some("1") {
        return Ok(());
    }
    bail!("UPC_DEMO_MODE=1 required for a live boot; add --dry-run to inspect without loading")
}

fn check_image_alignment(bytes: &[u8]) -> Result<usize> {
    if !bytes.len().is_multiple_of(4) {
        bail!("kernel image not 4-byte aligned ({} bytes)", bytes.len());
    }
    Ok(bytes.len() / 4)
}

fn cmd_validate(kernel: PathBuf) -> Result<()> {
    let bytes =
        std::fs::read(&kernel).with_context(|| format!("read kernel: {}", kernel.display()))?;

    let n_insns = check_image_alignment(&bytes)?;

    println!("kernel:        {}", kernel.display());
    println!("size:          {} bytes", bytes.len());
    println!("instructions:  {} MBC words", n_insns);

    // Decode first 32 instructions for sanity check.
    println!("\nfirst 32 MBC instructions:");
    for i in 0..32.min(n_insns) {
        let off = i * 4;
        let w = u32::from_le_bytes([bytes[off], bytes[off + 1], bytes[off + 2], bytes[off + 3]]);
        let opcode = (w >> 24) & 0xFF;
        let dst = (w >> 20) & 0xF;
        let src = (w >> 16) & 0xF;
        let imm = w & 0xFFFF;
        println!(
            "  PC=0x{:04x}  word=0x{:08x}  op=0x{:02x} dst=r{:<2} src=r{:<2} imm=0x{:04x}",
            i, w, opcode, dst, src, imm
        );
    }

    // Per ADR-072: canonical hex is the brand-spelled form (UNHD MSB-first),
    // and the LE byte sequence is the wire-order form (DHNU at increasing
    // memory addresses). Both shown for operator clarity.
    println!(
        "\nBoot magic: 0x{:08X} (canonical 'UNHD' MSB-first; wire bytes '{}' per ADR-072)",
        xv6_mbc::BOOT_MAGIC,
        std::str::from_utf8(&xv6_mbc::BOOT_MAGIC.to_le_bytes()).unwrap_or("???")
    );

    println!("\n✓ kernel image structurally valid");
    Ok(())
}

/// S-ASCEND-2 Step 174: prove the INSTANCE_OWNER check on the live object.
fn hijack_probe_run(runner: &runner::BootRunner, instance: u8, n: u32) -> Result<()> {
    // STATS keys: 1 = CPU ticks seen, 2 = instructions executed, 30 = hijack drops.
    let read = |r: &runner::BootRunner| -> Result<[u64; 3]> {
        let s = r.read_stats(&[1, 2, 30])?;
        Ok([s[0], s[1], s[2]])
    };
    // sendto returns once a frame is queued; XDP runs later. Let the veth
    // backlog drain before each read.
    let settle = || std::thread::sleep(std::time::Duration::from_millis(500));
    let owner = netns::nonce();
    let wrong = (owner ^ 0x5A5) & 0xFFF;
    let a = read(runner)?;
    netns::trigger_flow(n, netns::flow_label_for(instance, wrong), false)?;
    settle();
    let b = read(runner)?;
    netns::trigger_flow(n, netns::flow_label_for(instance, owner), false)?;
    settle();
    let c = read(runner)?;
    println!(
        "  hijack probe: {n} sent with nonce 0x{wrong:03X}: ticks +{}, insns +{}, hijack_drop +{}; \
         {n} sent with the owner nonce 0x{owner:03X}: ticks +{}, insns +{}, hijack_drop +{}",
        b[0] - a[0],
        b[1] - a[1],
        b[2] - a[2],
        c[0] - b[0],
        c[1] - b[1],
        c[2] - b[2]
    );
    Ok(())
}

#[allow(clippy::too_many_arguments)]
/// Load every image into the live BPF maps, set up the netns + XDP attach,
/// and pre-fill `--input`. Shared by `boot` (fixed trigger budget) and
/// `console` (interactive loop). Returns the runner (owning the loaded eBPF
/// object — dropping it detaches XDP) and the initial CPU state.
fn boot_prepare(
    kernel: &Path,
    bootstub: Option<&Path>,
    ramdisk: Option<&Path>,
    userland: Option<&Path>,
    instance: u8,
    input: Option<&str>,
    mmu: &upc_api::MmuLayout,
    rv: &rv2mbc::Rv2mbcSet,
) -> Result<(runner::BootRunner, runner::MbcCpuState)> {
    // ── LIVE BOOT PATH (Phase 1.1 SHIP per battle-plan-phase11-ship-2026-05-10.md) ──
    let kernel_bytes =
        std::fs::read(kernel).with_context(|| format!("read kernel: {}", kernel.display()))?;
    let _ = check_image_alignment(&kernel_bytes)?;
    let mbc_words: Vec<u32> = kernel_bytes
        .as_chunks::<4>()
        .0
        .iter()
        .map(|c| u32::from_le_bytes(*c))
        .collect();

    // BootParams v3 carries the kernel .rv2mbc SHA-256 that check_all verified.
    // A live boot always has it: check_all refuses a missing kernel .rv2mbc,
    // and the unverified hatch is dry-run only.
    let kernel_rv = rv
        .kernel
        .as_ref()
        .filter(|k| k.sha256.is_some())
        .ok_or_else(|| anyhow!("kernel .rv2mbc not verified; refusing a live boot"))?;
    let kernel_rv_sha = kernel_rv.sha256.unwrap_or_default();
    let bp = bootparams::BootParamsV2::for_xv6(kernel_bytes.len() as u32, 0, kernel_rv_sha);
    let bp_bytes = bp.to_bytes();

    let ebpf_obj = std::env::var("MONAD_CPU_EBPF_OBJ")
        .unwrap_or_else(|_| "target/bpfel-unknown-none/release/monad-cpu-ebpf".to_string());
    let ebpf_obj = std::path::PathBuf::from(ebpf_obj);
    if !ebpf_obj.exists() {
        bail!(
            "monad-cpu-ebpf object not found at {}\n\
             Build it first: make ebpf",
            ebpf_obj.display()
        );
    }

    // S-ASCEND-2 Steps 95-106: refuse a wrong object before aya creates a map.
    let facts = preflight_object(&ebpf_obj, mmu)?;

    let mut runner = runner::BootRunner::open(&ebpf_obj, instance as u32)?;

    // Memory writes per Boot Protocol v2 §"Boot Sequence" (zero IVT,
    // BootParams at 0x0100, zero CSR region).
    runner.populate_ram(&[
        (bootparams::ADDR_IVT, &[0u8; 1024]),
        (bootparams::ADDR_BOOTPARAMS, &bp_bytes),
        (bootparams::ADDR_CSR_REGION, &[0u8; 256]),
    ])?;

    // Phase 2 path: when --bootstub is provided, place upc-bootstub.mbc
    // at MBC PC 0x4000 (= byte 0x10000) and the kernel image at PC 0x8000
    // (= byte 0x20000). Phase 1.1 xv6 path (no --bootstub) loads the
    // kernel.mbc starting at slot 0 because xv6's .mbc was built with
    // its own .stage1 region at byte 0x10000 inlined by kernel-mbc.ld.
    if let Some(bs_path) = bootstub {
        let bs_bytes = std::fs::read(bs_path)
            .with_context(|| format!("read bootstub: {}", bs_path.display()))?;
        check_image_alignment(&bs_bytes)?;
        let bs_words: Vec<u32> = bs_bytes
            .as_chunks::<4>()
            .0
            .iter()
            .map(|c| u32::from_le_bytes(*c))
            .collect();
        runner.populate_rom_at(0x4000, &bs_words)?;
        runner.populate_rom_at(0x8000, &mbc_words)?;
        println!(
            "  ROM_MAP: bootstub @ slot 0x4000 ({} words) + kernel @ slot 0x8000 ({} words)",
            bs_words.len(),
            mbc_words.len()
        );
        // Load bootstub's .data sidecar (if present) into RAM_MAP so the
        // stub's "BOOT FAIL: ..." strings are resolvable.
        let bs_data_path = bs_path.with_extension("data");
        if bs_data_path.exists() {
            match runner.load_data_image(&bs_data_path) {
                Ok(()) => println!("  Bootstub data: loaded {}", bs_data_path.display()),
                Err(e) => println!("  ⚠ bootstub data load failed: {}", e),
            }
        }
    } else {
        // Phase 1.1 xv6 path: kernel.mbc loads at slot 0 (self-locating).
        runner.populate_rom(&mbc_words)?;
    }

    // Load the kernel's data-section image into RAM_MAP at byte-VMA addresses,
    // so the C compiler's `lui rd, hi; addi rd, lo` patterns can dereference
    // .rodata strings + .data globals. The translator emits this file as a
    // sibling to the .mbc (e.g. xv6-mbc.mbc -> xv6-mbc.data). Optional —
    // if missing, the boot will still progress but may halt on the first
    // string load. Task #61 closure.
    // Phase 1.4: load the xv6-format ramdisk (fs.img) at byte 0x00800000.
    // mkfs produces this image; xv6's fs.c later mounts it. Address per
    // docs/doom/UPC_PAGE_TABLE_LAYOUT.md.
    if let Some(rd) = ramdisk {
        match std::fs::read(rd) {
            Ok(bytes) => match runner.populate_ram(&[(0x00800000, &bytes)]) {
                Ok(()) => println!(
                    "  Ramdisk: loaded {} ({} bytes @ byte 0x00800000)",
                    rd.display(),
                    bytes.len()
                ),
                Err(e) => bail!("ramdisk load failed: {e}"),
            },
            Err(e) => bail!("ramdisk read {} failed: {e}", rd.display()),
        }
    }

    let data_path = kernel.with_extension("data");
    if data_path.exists() {
        match runner.load_data_image(&data_path) {
            Ok(()) => println!(
                "  Data image: loaded {} (.rodata/.data into RAM_MAP)",
                data_path.display()
            ),
            Err(e) => println!(
                "  ⚠ data image at {} failed to load: {} (boot may halt early)",
                data_path.display(),
                e
            ),
        }
    } else {
        println!(
            "  ⚠ no data image at {} — .rodata reads will return zero",
            data_path.display()
        );
    }

    // Load the RV→MBC translation map. The translator emits this as a
    // sibling to the .mbc (xv6-mbc.mbc → xv6-mbc.rv2mbc). Without it,
    // JALR / CALLR / MRET / SRET land on unset entries (0 = slot 0) and
    // execution falls through to garbage. Phase 1.3 AP-2 root cause.
    //
    // The translator emits entries indexed from RV word 0 (start of the
    // .text section). xv6's .text begins at RV byte 0x20000 (RV word
    // 0x8000) per kernel-mbc.ld; the loader shifts entries by that base
    // so the BPF interpreter's absolute-RV-word lookups hit the right
    // slot. Phase 2 will pass a different base for the bootstub flow.
    //
    // S-ASCEND-2 Item 5: the bytes were read and SHA-checked by
    // rv2mbc::check_all before the object was opened; populate_rv2mbc hashes
    // them again against that SHA before its first map write.
    let text_rv_word_base: u32 = 0x8000; // xv6 .text base = RV byte 0x20000
    let kernel_rv_hex = kernel_rv.sha_hex();
    runner
        .populate_rv2mbc(
            &kernel_rv.bytes,
            text_rv_word_base,
            kernel_rv_hex.as_deref(),
        )
        .map_err(|e| anyhow!("rv2mbc load failed: {e}"))?;
    println!(
        "  RV→MBC map: loaded {} ({} entries @ RV-word-base 0x{:04X} + SHA gate)",
        kernel_rv.path.display(),
        kernel_rv.bytes.len() / 4,
        text_rv_word_base
    );

    // Phase 1.5: load the userland MBC image into a dedicated ROM_MAP region
    // (USER_ROM_BASE = 0x4000) plus shifted RV2MBC entries (starting at slot 0)
    // plus the .data sidecar into RAM_MAP. xv6's kexec then drops the process
    // into user mode by setting trapframe->epc = 0 (matching user.ld which
    // links userland at RV byte 0); SRET reads SEPC=0, RV2MBC_MAP[0] points
    // at USER_ROM_BASE, and the user MBC code runs.
    const USER_ROM_BASE: u32 = 0x4000;
    if let Some(up) = userland {
        let bytes =
            std::fs::read(up).with_context(|| format!("userland read {} failed", up.display()))?;
        if !bytes.len().is_multiple_of(4) {
            bail!(
                "userland {} not 4-byte aligned ({} bytes)",
                up.display(),
                bytes.len()
            );
        }
        // CALL opcode (0x27) uses an absolute 24-bit MBC PC. The translator
        // emits CALL targets relative to the .mbc's slot 0, but we load init.mbc
        // at USER_ROM_BASE — so every CALL immediate needs USER_ROM_BASE added or
        // it lands inside the kernel image. Same fix is NOT needed for
        // JMP/JZ/JNZ/... which use signed PC-relative offsets (wrap-add survives
        // the shift).
        let mut patched_calls = 0u32;
        let words: Vec<u32> = bytes
            .as_chunks::<4>()
            .0
            .iter()
            .map(|c| {
                let w = u32::from_le_bytes(*c);
                if (w >> 24) == OP_CALL {
                    patched_calls += 1;
                }
                relocate_call_word(w, USER_ROM_BASE)
            })
            .collect();
        let n_words = words.len();

        // Userland rv2mbc: each entry maps a user RV word to an MBC PC inside
        // init.mbc (whose first instruction is at MBC PC 0 in its own stream).
        // Shift every entry by USER_ROM_BASE so the BPF interpreter's lookup
        // lands on the right slot in the shared ROM_MAP. Loaded at RV-word-base 0
        // (userland links text at RV byte 0). A missing file yields an empty
        // table (a warning — SRET into user code would then fall through).
        // The bytes were read and SHA-checked by rv2mbc::check_all.
        let user_rv2mbc_path = up.with_extension("rv2mbc");
        let rv_entries: Vec<u32> = if let Some(urv) = rv.userland.as_ref() {
            if !urv.bytes.len().is_multiple_of(4) {
                bail!("userland rv2mbc {} not 4-byte aligned", urv.path.display());
            }
            urv.bytes
                .as_chunks::<4>()
                .0
                .iter()
                .map(|c| relocate_rv2mbc_entry(u32::from_le_bytes(*c), USER_ROM_BASE))
                .collect()
        } else {
            println!(
                "  ⚠ no userland rv2mbc at {} — SRET into user code will fall through",
                user_rv2mbc_path.display()
            );
            Vec::new()
        };
        let n_rv = rv_entries.len();

        // Route ROM + RV2MBC population through the shared loader (Epic 1.2.3):
        // validate the descriptor (image bounds, entry-in-ROM, every rv2mbc
        // target in ROM) then write both maps. entry = USER_ROM_BASE (user.ld
        // links _start at RV byte 0 → ROM slot USER_ROM_BASE). Byte-identical to
        // the prior populate_rom_at + populate_rv2mbc calls. write_image, not
        // load: the resident programs follow, so the freeze comes after them.
        let workload = xv6_program_workload(
            "init",
            words,
            USER_ROM_BASE,
            rv_entries,
            0,
            USER_ROM_BASE,
            mmu,
            rv.userland.as_ref().and_then(|u| u.sha_hex()),
        );
        upc_api::write_image(&workload, &xv6_map_capacities(), &facts, &mut runner)
            .map_err(|e| anyhow!("userland image load {}: {e}", up.display()))?;

        if patched_calls > 0 {
            println!(
                "  Userland MBC: patched {} CALL targets (+0x{:04X}) for user-region load",
                patched_calls, USER_ROM_BASE
            );
        }
        println!(
            "  Userland MBC: loaded {} ({} bytes / {} words @ ROM slot 0x{:04X})",
            up.display(),
            bytes.len(),
            n_words,
            USER_ROM_BASE
        );
        if n_rv > 0 {
            println!(
                "  Userland RV→MBC: loaded {} ({} entries @ RV-word-base 0x0000 + shift 0x{:04X})",
                user_rv2mbc_path.display(),
                n_rv,
                USER_ROM_BASE
            );
        }

        // Userland .data: same TLV format as the kernel side. Each record's
        // byte_addr is in user.ld's address space starting at 0, so it lands
        // in RAM_MAP[byte_addr] directly. No conflicts with kernel layout
        // because user .text fills bytes 0..N (read-only — covered by ROM_MAP)
        // and user .rodata/.data start past the IVT/BootParams region. RAM_MAP is
        // a distinct map from ROM_MAP/RV2MBC_MAP, so loading it after the shared
        // image load is order-independent.
        let user_data_path = up.with_extension("data");
        if user_data_path.exists() {
            match runner.load_data_image(&user_data_path) {
                Ok(()) => println!(
                    "  Userland data: loaded {} (.rodata/.data into RAM_MAP)",
                    user_data_path.display()
                ),
                Err(e) => println!(
                    "  ⚠ userland data at {} failed to load: {}",
                    user_data_path.display(),
                    e
                ),
            }
        }
    }

    // ── Phase 1.7 Gate B (ADR-077): resident programs for exec(7) ──
    // Pre-translate sibling userland programs into disjoint ROM/RV2MBC
    // regions + a PROGRAM_TABLE the in-kernel exec handler resolves by
    // basename hash. init stays at ROM 0x4000 / rv2mbc-base 0; programs
    // start past it and below the kernel's rv2mbc base (0x8000). Loaded
    // as siblings of --userland so the existing boot recipe is unchanged.
    if let Some(up) = userland {
        // Headline (Gate B) needs `sh`; ls/cat/echo/wc follow the same
        // pattern for Gate C and slot in after sh's bases. rv2mbc::check_all
        // decided which are present and SHA-checked their .rv2mbc files.
        let dir = up.parent().unwrap_or_else(|| Path::new("."));
        for (i, name) in geometry::RESIDENT_PROGRAMS.iter().enumerate() {
            let Some((mbc, prog_rv)) = rv.programs.get(i).and_then(Option::as_ref) else {
                println!(
                    "  ⚠ resident program '{name}' not found at {} — skipping",
                    dir.join(format!("{name}.mbc")).display()
                );
                continue;
            };
            let slot = i as u32;
            let rom_base = 0x6000 + slot * 0x3000; // 12 Ki-word stride
            let rv2mbc_base = 0x1000 + slot * 0x1000; // < 0x8000 (kernel base)
            let data_stage_va =
                geometry::RESIDENT_DATA_STAGE_BASE + slot * geometry::RESIDENT_DATA_STAGE_STRIDE; // pristine RAM
            load_resident_program(
                &mut runner,
                &facts,
                mmu,
                mbc,
                prog_rv,
                name,
                slot,
                rom_base,
                rv2mbc_base,
                data_stage_va,
            )?;
        }
    }

    // ADR-074 Option A Gate 1 (2026-05-30): build pid 0's identity page
    // directory at its fixed pgd region (0x00F00000) before the CPU starts.
    // 16 superpages × 4 MiB = identity-maps the low 64 MiB onto physical RAM.
    // The interpreter only translates user-mode accesses, so this is a no-op
    // transform that must NOT change the boot — proving the live MMU walk.
    runner.populate_identity_pgd(geometry::XV6_PGD_BASE, 16)?;
    println!("  ✓ pid-0 identity pgd built at 0x00F00000 (16×4MiB superpages, MMU live)");

    // Initial CPU state: PC=0x4000 (word index of byte 0x10000), SP=0x03F00000,
    // priv_level=0 M-mode, reservation_address=0xFFFFFFFF.
    runner.populate_cpu(runner::xv6_initial_cpu_state())?;

    // Read back the CPU state to confirm the write took.
    let cpu_initial = runner.cpu_state()?;
    println!(
        "\n✓ live BPF maps populated for instance 0x{:02X}\n  \
         CPU_MAP[0x{:X}]: PC=0x{:08X} SP=0x{:08X} priv={} halted={} num_proc={} cur_pid={}",
        instance,
        instance,
        cpu_initial.pc,
        cpu_initial.regs[15],
        cpu_initial.priv_level,
        cpu_initial.halted,
        cpu_initial.num_processes,
        cpu_initial.current_pid
    );
    println!(
        "  ROM_MAP: {} MBC words loaded ({} bytes)",
        mbc_words.len(),
        kernel_bytes.len()
    );

    // S-ASCEND-2 Steps 157-161: freeze ROM_MAP and RV2MBC_MAP. Every write to
    // either is above (kernel or bootstub + kernel, kernel .rv2mbc, init,
    // resident programs), each after its SHA, surface and geometry gates; the
    // program only reads them. A failed freeze fails the boot before attach.
    upc_api::finalize(&mut runner).map_err(|e| anyhow!("{e}"))?;
    println!("  ✓ ROM_MAP + RV2MBC_MAP frozen (BPF_MAP_FREEZE)");
    if freeze::check_enabled() {
        runner.freeze_check()?;
    }

    // ── Phase 4: netns + XDP attach + trigger packet ──
    let attach_iface = netns::prepare()?;
    // S-ASCEND-2 Phase 6: bind the instance to a fresh nonce before any tick.
    let nonce = netns::random_nonce()?;
    runner.bind_instance_owner(nonce)?;
    netns::set_nonce(nonce);
    println!("  ✓ instance 0x{instance:02X} bound to flow nonce 0x{nonce:03X} (INSTANCE_OWNER)");
    // Attach XDP to veth inside upc0 namespace. aya's `attach` looks up
    // the iface by name in the calling process's namespace, so we run
    // the attach via `ip netns exec` semantically by... actually aya
    // uses if_nametoindex which scans the current namespace. The
    // simplest approach: don't move the iface into a netns, attach to
    // the host-side veth-upc0 instead. (Phase 1.1 Mode A doesn't need
    // network isolation; isolation is for Phase 4 multi-host.)
    //
    // Alternative simpler path: skip namespace, just use the veth host-side.
    // For Phase 1.1 first boot, host-side attach is sufficient.
    println!("\n[netns ready; attempting XDP attach to {}]", attach_iface);
    match runner.attach_xdp(attach_iface) {
        Ok(()) => println!("  ✓ XDP attached to veth-upc0"),
        Err(e) => {
            println!("  ✗ XDP attach failed: {}", e);
            netns::finish();
            return Err(e);
        }
    }

    // Gate C: pre-load scripted console input into the KBD ring BEFORE the CPU
    // runs, so the guest's read(5) drain feeds it to sh's gets() as if typed.
    if let Some(s) = input {
        runner.write_kbd(s.as_bytes())?;
        println!(
            "  ✓ KBD ring pre-filled with {} input byte(s) (--input, read does not echo)",
            s.len()
        );
    }

    Ok((runner, cpu_initial))
}

#[allow(clippy::too_many_arguments)] // one per `boot` CLI flag, passed straight from clap
fn cmd_boot(
    kernel: PathBuf,
    bootstub: Option<PathBuf>,
    initramfs: Option<PathBuf>,
    ramdisk: Option<PathBuf>,
    userland: Option<PathBuf>,
    triggers: u32,
    instance: u8,
    dry_run: bool,
    input: Option<String>,
    unsafe_geometry: bool,
    rv2mbc_sha: Option<String>,
    allow_unverified_rv2mbc: bool,
    hijack_probe: Option<u32>,
) -> Result<()> {
    demo_gate(dry_run, std::env::var("UPC_DEMO_MODE").ok().as_deref())?;
    rv2mbc::hatch_gate(allow_unverified_rv2mbc, dry_run)?;
    cmd_validate(kernel.clone())?;
    let mmu = geometry::xv6_mmu_layout(geometry::stride_override(
        unsafe_geometry,
        std::env::var("UPC_SLICE_STRIDE").ok().as_deref(),
    )?);
    // S-ASCEND-2 Item 5: every .rv2mbc is SHA-checked before the object opens.
    println!("\n.rv2mbc SHA-256 gate:");
    let rv = rv2mbc::check_all(
        &kernel,
        userland.as_deref(),
        rv2mbc_sha
            .or_else(|| std::env::var("UPC_RV2MBC_SHA").ok())
            .as_deref(),
        allow_unverified_rv2mbc,
        dry_run,
    )?;

    println!("\n=== BOOT DISPATCH (dry_run={}) ===", dry_run);
    println!("instance:      0x{:02X}", instance);
    if let Some(ref bs) = bootstub {
        let sz = std::fs::metadata(bs).map(|m| m.len()).unwrap_or(0);
        println!("bootstub:      {} ({} bytes)", bs.display(), sz);
    } else {
        println!("bootstub:      (none — kernel image must inline its own start code)");
    }
    if let Some(ref ir) = initramfs {
        let sz = std::fs::metadata(ir)
            .with_context(|| format!("stat initramfs: {}", ir.display()))?
            .len();
        println!("initramfs:     {} ({} bytes)", ir.display(), sz);
    } else {
        println!("initramfs:     (none — kernel boots without /init)");
    }

    println!("\nBoot Protocol v2 setup (per docs/doom/UPC_BOOT_PROTOCOL_V2.md):");
    println!(
        "  1. Allocate PerCPU<MbcCpuState> slot for instance 0x{:02X}",
        instance
    );
    println!("  2. Zero IVT region (byte 0x0000-0x03FF)");
    println!("  3. Zero CSR region (byte 0x000_F000-0x000_F0FF)");
    println!("  4. Write default HLT trap handler at 0x0400");
    println!(
        "  5. Write BootParams v{} (256 bytes) at 0x0100",
        bootparams::BOOT_VERSION
    );
    println!(
        "     magic = 0x{:08X} (canonical 'UNHD' MSB-first; wire bytes 'DHNU' per ADR-072)",
        xv6_mbc::BOOT_MAGIC
    );
    println!("     version = {}", bootparams::BOOT_VERSION);
    match rv.kernel.as_ref().and_then(|k| k.sha_hex()) {
        Some(h) => println!("     rv2mbc_sha256 = {h} (bytes 64..96)"),
        None => println!("     rv2mbc_sha256 = zeros (unverified: dry run only)"),
    }
    println!("     kernel_addr = 0x10000 (stage-1 stub)");
    println!("     bss_start / bss_end (read from kernel.elf .bss)");
    println!("     boot_random_seed = 32 bytes from getrandom(2)");
    println!("  6. Load kernel image at byte 0x10000");
    println!(
        "     {} words → ROM_MAP slots 0x4000+",
        std::fs::metadata(&kernel)?.len() / 4
    );
    if initramfs.is_some() {
        println!("  7. Load initramfs at byte 0x800000");
    }
    println!("  8. Initialize MbcCpuState:");
    println!("     PC = 0x4000 (stage-1 stub, word index)");
    println!("     SP = 0x03F00000 (top of stack, byte address)");
    println!("     priv_level = 0 (M-mode at boot)");
    println!("     reservation_address = 0xFFFF_FFFF (no LR)");
    println!("  9. Dispatch Gjallarhorn UPC trigger packet");
    println!(" 10. Watch tty stream (compute.tty.{{instance}})");

    if dry_run {
        // The object gates read the file only, so a dry run can show a refusal.
        let ebpf_obj =
            PathBuf::from(std::env::var("MONAD_CPU_EBPF_OBJ").unwrap_or_else(|_| {
                "target/bpfel-unknown-none/release/monad-cpu-ebpf".to_string()
            }));
        if ebpf_obj.exists() {
            preflight_object(&ebpf_obj, &mmu)?;
        } else {
            println!(
                "  (object gates not run: no object at {})",
                ebpf_obj.display()
            );
        }
        println!("\n✓ dry-run complete; no BPF maps touched");
        return Ok(());
    }

    let (mut runner, cpu_initial) = boot_prepare(
        &kernel,
        bootstub.as_deref(),
        ramdisk.as_deref(),
        userland.as_deref(),
        instance,
        input.as_deref(),
        &mmu,
        &rv,
    )?;

    // Send Monad-format trigger packets via doom-tick.py. flow_label =
    // instance ID; eBPF dispatches on (flow_label & 0xFF).
    println!(
        "\n[sending {} Monad trigger packets to advance the CPU (each = up to 16 insns)]",
        triggers
    );
    netns::trigger(triggers, instance, true)?;
    std::thread::sleep(std::time::Duration::from_millis(1500));

    // Read CPU state after trigger
    let cpu_after = runner.cpu_state()?;
    println!(
        "\n=== AFTER TRIGGER ===\n  \
         CPU_MAP[0x{:X}]: PC=0x{:08X} SP=0x{:08X} priv={} halted={} insn_count={} num_proc={} cur_pid={} brk=0x{:08X}",
        instance,
        cpu_after.pc,
        cpu_after.regs[15],
        cpu_after.priv_level,
        cpu_after.halted,
        cpu_after.insn_count,
        cpu_after.num_processes,
        cpu_after.current_pid,
        cpu_after.program_break
    );
    // Non-wrapping STATS dump (the TTY ring wraps; these counters don't).
    // Keys: 3=HALTED 7=SYSCALLS 8=ROM_FAULT 13=LINUX_SYS 14=TTY_WRITES
    // 15=CTX_SWITCH 16=FORKS 20=TTY_READS 21=WAITPIDS 23=MODERATE_SYS.
    if let Ok(s) = runner.read_stats(&[3, 7, 8, 13, 14, 15, 16, 20, 21, 23, 25, 26, 27, 28, 29, 30])
    {
        println!(
            "  STATS: halted={} syscalls={} rom_fault={} lin_sys={} tty_w={} ctx_sw={} forks={} tty_r={} waitpid={} mod_sys={} hijack_drop={}",
            s[0], s[1], s[2], s[3], s[4], s[5], s[6], s[7], s[8], s[9], s[15]
        );
        println!(
            "  ROM-FAULT CTX: target=0x{:X} rv2mbc_base=0x{:X} cur_pid={} prev_pc=0x{:X} halt_reason=0x{:X}",
            s[10], s[11], s[12], s[13], s[14]
        );
    }
    if let Some(n) = hijack_probe {
        hijack_probe_run(&runner, instance, n)?;
    }
    if cpu_after.pc != cpu_initial.pc || cpu_after.insn_count > 0 {
        println!("  ✓ FIRST HEARTBEAT — eBPF interpreter advanced the CPU");
    } else {
        println!("  ⚠ no PC advance — XDP may not have fired (debug needed)");
    }

    // Drain TTY_MAP — did the kernel print the banner?
    let mut head_cursor = 0u32;
    let tty_bytes = runner.read_tty(&mut head_cursor).unwrap_or_default();
    if !tty_bytes.is_empty() {
        // Render as both readable string AND hex for diagnosability —
        // null bytes / non-ASCII can hide what really happened.
        let printable: String = tty_bytes
            .iter()
            .map(|&b| {
                if (32..127).contains(&b) {
                    b as char
                } else {
                    '·'
                }
            })
            .collect();
        let hex: String = tty_bytes
            .iter()
            .map(|b| format!("{:02x}", b))
            .collect::<Vec<_>>()
            .join(" ");
        println!(
            "\n=== TTY OUTPUT ({} bytes) ===\n  ascii: \"{}\"\n  hex:   {}",
            tty_bytes.len(),
            printable,
            hex
        );
        // Phase 1 SHIP §"TTY transport is HTTP loopback": forward captured
        // bytes to the upc-tty-bridge ingest endpoint so any attached browser
        // xterm sees them. Best-effort — bridge unreachable is OK in dev.
        tty_bridge::post_ingest(instance, &tty_bytes);
        // Try to find "xv6 booting" anywhere in the captured bytes.
        // The MMIO 0xC001 unaligned word store splits into 3 byte writes per
        // call (char + 2 nulls), so the wire bytes look like
        // 'x','\0','\0','v','\0','\0',... — strip the nulls before the
        // banner-detection contains() so the gate fires correctly.
        let stripped: String = tty_bytes
            .iter()
            .filter(|&&b| b != 0)
            .map(|&b| b as char)
            .collect();
        let s = String::from_utf8_lossy(&tty_bytes);
        if stripped.contains("xv6 booting") || s.contains("xv6 booting") {
            println!("\n  🎉 PHASE 1.1 GATE BANNER VISIBLE: \"xv6 booting...\"");
        } else if stripped.contains("BOOT FAIL") || s.contains("BOOT FAIL") {
            println!("\n  ⚠ start_mbc.c HIT 'BOOT FAIL' branch — magic or version mismatch.");
            println!(
                "    Likely cause: BootParams address mismatch OR translator skipped LD insns."
            );
        } else {
            println!("\n  ⚠ TTY captured non-banner output. Path through start_mbc.c unclear.");
        }
    } else {
        println!("\n[TTY_MAP empty — kernel hasn't reached mmio_puts yet; try more triggers]");
    }

    // Cleanup
    runner.cleanup()?;
    netns::finish();

    Ok(())
}

struct ConsoleOpts {
    kernel: PathBuf,
    bootstub: Option<PathBuf>,
    ramdisk: Option<PathBuf>,
    userland: Option<PathBuf>,
    instance: u8,
    stdin: bool,
    tick_batch: u32,
    debug_kbd: bool,
    unsafe_geometry: bool,
    rv2mbc_sha: Option<String>,
}

/// Set on SIGINT / SIGTERM so the console loop can tear down XDP + netns
/// instead of dying with the program attached.
fn install_stop_flag() -> Arc<AtomicBool> {
    let flag = Arc::new(AtomicBool::new(false));
    let f = flag.clone();
    std::thread::spawn(move || {
        let rt = match tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
        {
            Ok(rt) => rt,
            Err(e) => {
                eprintln!("[console] signal runtime: {e}");
                return;
            }
        };
        rt.block_on(async move {
            use tokio::signal::unix::{signal, SignalKind};
            let mut term = signal(SignalKind::terminate()).ok();
            let term_recv = async {
                match term.as_mut() {
                    Some(t) => {
                        t.recv().await;
                    }
                    None => std::future::pending::<()>().await,
                }
            };
            tokio::select! {
                _ = tokio::signal::ctrl_c() => {}
                _ = term_recv => {}
            }
            f.store(true, Ordering::SeqCst);
        });
    });
    flag
}

/// Raw stdin bytes, delivered in chunks; `None` = EOF.
fn spawn_stdin_reader() -> mpsc::Receiver<Option<Vec<u8>>> {
    let (tx, rx) = mpsc::channel();
    std::thread::spawn(move || {
        let mut stdin = std::io::stdin().lock();
        let mut buf = [0u8; 256];
        loop {
            match stdin.read(&mut buf) {
                Ok(0) | Err(_) => {
                    let _ = tx.send(None);
                    return;
                }
                Ok(n) => {
                    if tx.send(Some(buf[..n].to_vec())).is_err() {
                        return;
                    }
                }
            }
        }
    });
    rx
}

fn cmd_console(o: ConsoleOpts) -> Result<()> {
    if o.tick_batch == 0 {
        bail!("--tick-batch must be > 0");
    }
    // console always attaches: there is no dry run.
    demo_gate(false, std::env::var("UPC_DEMO_MODE").ok().as_deref())?;
    cmd_validate(o.kernel.clone())?;
    let mmu = geometry::xv6_mmu_layout(geometry::stride_override(
        o.unsafe_geometry,
        std::env::var("UPC_SLICE_STRIDE").ok().as_deref(),
    )?);
    let explicit = o
        .rv2mbc_sha
        .clone()
        .or_else(|| std::env::var("UPC_RV2MBC_SHA").ok());
    let rv = rv2mbc::check_all(
        &o.kernel,
        o.userland.as_deref(),
        explicit.as_deref(),
        false,
        false,
    )?;
    let stop = install_stop_flag();
    let (mut runner, _cpu_initial) = boot_prepare(
        &o.kernel,
        o.bootstub.as_deref(),
        o.ramdisk.as_deref(),
        o.userland.as_deref(),
        o.instance,
        None,
        &mmu,
        &rv,
    )?;
    println!(
        "\n=== UPC CONSOLE (instance 0x{:02X}) — Ctrl-C to stop ===",
        o.instance
    );
    let result = console_loop(&mut runner, &o, &stop);
    // Same teardown as `boot`; dropping the runner detaches XDP.
    let _ = runner.cleanup();
    netns::finish();
    drop(runner);
    println!("\n=== UPC CONSOLE closed ===");
    result
}

fn console_loop(runner: &mut runner::BootRunner, o: &ConsoleOpts, stop: &AtomicBool) -> Result<()> {
    const BRIDGE_RETRY_BATCHES: u32 = 20;
    const IDLE_BATCHES_AFTER_EOF: u32 = 3;
    let ring = runner::KBD_RING_SLOTS as usize;

    let bridge = tty_bridge::BridgeClient::from_env().ok();
    let mut bridge_retry_in = 0u32;
    let stdin_rx = o.stdin.then(spawn_stdin_reader);
    let mut stdin_eof = false;
    let mut pending: VecDeque<u8> = VecDeque::new();
    let mut tty_cursor = 0u32;
    let mut idle = 0u32;
    let mut out = std::io::stdout();

    while !stop.load(Ordering::SeqCst) {
        netns::trigger(o.tick_batch, o.instance, false)?;

        // Output: guest TTY -> stdout (NULs from the 3-byte MMIO store dropped)
        // and -> the bridge for browser viewers.
        let tty = runner.read_tty(&mut tty_cursor)?;
        if !tty.is_empty() {
            let shown: Vec<u8> = tty.iter().copied().filter(|&b| b != 0).collect();
            let _ = out.write_all(&shown);
            let _ = out.flush();
        }

        // Bridge: ingest output, drain keystrokes. On any error, back off for
        // a few batches (the bridge is optional) and try again later.
        if bridge_retry_in > 0 {
            bridge_retry_in -= 1;
        } else if let Some(b) = bridge.as_ref() {
            let ok = b.ingest(o.instance, &tty).is_ok()
                && match b.drain(o.instance, ring.saturating_sub(pending.len())) {
                    Ok(bytes) => {
                        pending.extend(bytes);
                        true
                    }
                    Err(_) => false,
                };
            if !ok {
                bridge_retry_in = BRIDGE_RETRY_BATCHES;
            }
        }

        // stdin chunks. Everything queues locally until the ring has room:
        // the ring holds 64 bytes, the rest waits here and is never dropped.
        if let Some(rx) = stdin_rx.as_ref() {
            while let Ok(msg) = rx.try_recv() {
                match msg {
                    Some(b) => pending.extend(b),
                    None => stdin_eof = true,
                }
            }
        }
        if !pending.is_empty() {
            let chunk: Vec<u8> = pending.iter().copied().take(ring).collect();
            let n = runner.append_kbd(&chunk)?;
            pending.drain(..n);
        }
        let (head, tail) = runner.kbd_indices()?;
        if o.debug_kbd {
            eprintln!(
                "[kbd] head={head} tail={tail} queued={} pending={}",
                head.wrapping_sub(tail),
                pending.len()
            );
        }

        if runner.cpu_state()?.halted != 0 {
            eprintln!("\n[console] guest halted");
            break;
        }
        // --stdin: once stdin is closed, everything typed has been consumed
        // and the guest has been quiet for a few batches, stop.
        if stdin_eof && pending.is_empty() && head == tail {
            idle = if tty.is_empty() { idle + 1 } else { 0 };
            if idle >= IDLE_BATCHES_AFTER_EOF {
                break;
            }
        }
    }
    Ok(())
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    run(cli).map_err(|e| {
        // S-ASCEND-2 Step 135: say which privilege is missing instead of a bare
        // EPERM from deep inside aya.
        if format!("{e:#}").contains("Operation not permitted") {
            e.context(
                "need CAP_BPF + CAP_NET_ADMIN (+ CAP_NET_RAW for triggers): \
                 run via unheaded-upc-bootctl@.service or sudo",
            )
        } else {
            e
        }
    })
}

fn run(cli: Cli) -> Result<()> {
    match cli.cmd {
        Cmd::Validate { kernel } => cmd_validate(kernel),
        Cmd::Boot {
            kernel,
            bootstub,
            initramfs,
            ramdisk,
            userland,
            triggers,
            instance,
            dry_run,
            input,
            unsafe_geometry,
            create_netns,
            rv2mbc_sha,
            allow_unverified_rv2mbc,
            hijack_probe,
            freeze_check,
        } => {
            netns::set_create_netns(create_netns);
            freeze::set_check(freeze_check);
            cmd_boot(
                kernel,
                bootstub,
                initramfs,
                ramdisk,
                userland,
                triggers,
                instance,
                dry_run,
                input,
                unsafe_geometry,
                rv2mbc_sha,
                allow_unverified_rv2mbc,
                hijack_probe,
            )
        }
        Cmd::Console {
            kernel,
            bootstub,
            ramdisk,
            userland,
            instance,
            stdin,
            tick_batch,
            debug_kbd,
            unsafe_geometry,
            create_netns,
            rv2mbc_sha,
        } => {
            netns::set_create_netns(create_netns);
            cmd_console(ConsoleOpts {
                kernel,
                bootstub,
                ramdisk,
                userland,
                instance,
                stdin,
                tick_batch,
                debug_kbd,
                unsafe_geometry,
                rv2mbc_sha,
            })
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    mod object_surface {
        include!("../../../ebpf/monad-cpu-ebpf/surface.rs");
    }

    #[test]
    fn demo_gate_allows_only_dry_run_or_exactly_1() {
        assert!(demo_gate(true, None).is_ok());
        assert!(demo_gate(true, Some("0")).is_ok());
        assert!(demo_gate(false, Some("1")).is_ok());
        for v in [None, Some(""), Some("0"), Some("true"), Some("1 ")] {
            let e = demo_gate(false, v).unwrap_err().to_string();
            assert!(
                e.contains("UPC_DEMO_MODE=1 required for a live boot"),
                "{v:?}: {e}"
            );
        }
    }

    fn xv6_numbers() -> Vec<u32> {
        XV6_SYSCALLS.iter().map(|d| d.number).collect()
    }

    /// The facts the shipped ascend object carries (surface.rs + phase12.rs).
    fn ascend_facts() -> upc_api::ObjectFacts {
        upc_api::ObjectFacts {
            surface_id: upc_api::surface_hash::surface_id(object_surface::SURFACE_ASCEND),
            mmu_geometry: Some(geometry::xv6_slice_geometry(geometry::XV6_SLICE_STRIDE)),
        }
    }

    // ── S-ASCEND-2 Step 106: the loader's descriptors pass the gates ──

    #[test]
    fn xv6_surface_is_the_ascend_object_surface() {
        let mut declared = xv6_numbers();
        declared.sort_unstable();
        assert_eq!(declared, object_surface::SURFACE_ASCEND);
        assert_eq!(
            upc_api::check_surface(&XV6_SURFACE, &ascend_facts()),
            Ok(())
        );
    }

    #[test]
    fn xv6_surface_refused_by_the_doom_object() {
        let doom = upc_api::ObjectFacts {
            surface_id: upc_api::surface_hash::surface_id(object_surface::SURFACE_DEFAULT),
            mmu_geometry: ascend_facts().mmu_geometry,
        };
        assert!(matches!(
            upc_api::check_surface(&XV6_SURFACE, &doom),
            Err(upc_api::ValidationError::SurfaceObjectMismatch { .. })
        ));
    }

    #[test]
    fn shipped_xv6_geometry_passes_gate_b() {
        let mmu = geometry::xv6_mmu_layout(None);
        assert_eq!(
            upc_api::check_mmu(
                upc_api::MemoryModel::PerPidMmu,
                Some(&mmu),
                &xv6_map_capacities(),
                &ascend_facts()
            ),
            Ok(())
        );
        // pids 1..=5 fit under the 56 MiB ceiling: [16, 56) MiB.
        let slices = mmu.slices.admitted_slices();
        assert_eq!(slices.len(), 5);
        assert_eq!(slices[0], (1, 0x0100_0000, 0x0180_0000));
        assert_eq!(slices[4], (5, 0x0300_0000, 0x0380_0000));
    }

    #[test]
    fn narrow_stride_override_is_refused_with_slice_overlap() {
        // 0x600000 = gate_nway.c's falsification stride (HIGH_BASE, 6 MiB);
        // 0x400000 = the value S-ASCEND-2 Step 111 names.
        for stride in [0x0060_0000u32, 0x0040_0000] {
            let mmu = geometry::xv6_mmu_layout(Some(stride));
            let err = upc_api::check_mmu(
                upc_api::MemoryModel::PerPidMmu,
                Some(&mmu),
                &xv6_map_capacities(),
                &ascend_facts(),
            )
            .unwrap_err();
            assert!(
                matches!(err, upc_api::ValidationError::SliceOverlap { pid: 1, .. }),
                "stride 0x{stride:X}: {err}"
            );
        }
    }

    #[test]
    fn wide_stride_override_is_refused_as_not_shipped() {
        let mmu = geometry::xv6_mmu_layout(Some(0x0100_0000));
        assert!(matches!(
            upc_api::check_mmu(
                upc_api::MemoryModel::PerPidMmu,
                Some(&mmu),
                &xv6_map_capacities(),
                &ascend_facts()
            ),
            Err(upc_api::ValidationError::SliceGeometryMismatch { .. })
        ));
    }

    #[test]
    fn stride_override_needs_the_flag_and_a_number() {
        assert_eq!(geometry::stride_override(true, None).unwrap(), None);
        assert_eq!(
            geometry::stride_override(false, Some("0x400000")).unwrap(),
            None
        );
        assert_eq!(
            geometry::stride_override(true, Some("0x400000")).unwrap(),
            Some(0x0040_0000)
        );
        assert_eq!(
            geometry::stride_override(true, Some("6291456")).unwrap(),
            Some(0x0060_0000)
        );
        assert!(geometry::stride_override(true, Some("six MiB")).is_err());
        assert!(geometry::stride_override(true, Some("0x100000000")).is_err());
    }

    #[test]
    fn alignment_4_bytes_is_one_instruction() {
        assert_eq!(check_image_alignment(&[0; 4]).unwrap(), 1);
    }

    #[test]
    fn alignment_8_bytes_is_two_instructions() {
        assert_eq!(check_image_alignment(&[0xAA; 8]).unwrap(), 2);
    }

    #[test]
    fn alignment_empty_is_zero_instructions() {
        // Empty image is technically aligned (0 % 4 == 0). cmd_validate's
        // higher-level checks (boot magic etc.) would reject it later, but
        // the alignment gate alone passes.
        assert_eq!(check_image_alignment(&[]).unwrap(), 0);
    }

    #[test]
    fn alignment_5_bytes_errors_with_byte_count() {
        let err = check_image_alignment(&[0; 5]).unwrap_err();
        let msg = err.to_string();
        assert!(msg.contains("4-byte aligned"), "got: {}", msg);
        assert!(msg.contains("5 bytes"), "got: {}", msg);
    }

    #[test]
    fn alignment_realistic_xv6_size_resolves_to_word_count() {
        // xv6-mbc.mbc as of ASCEND-LINUX Phase 1.1 super-sprint = 11_721 MBC
        // instructions = 46_884 bytes. Spot-check that ratio.
        const XV6_MBC_BYTES: usize = 11_721 * 4;
        let buf = vec![0u8; XV6_MBC_BYTES];
        assert_eq!(check_image_alignment(&buf).unwrap(), 11_721);
    }

    // ── relocate_call_word: characterization of the loader's core relocation ──
    // These pin the current behavior so the upc-api loader refactor (Epic 1.2)
    // can be proven byte-identical against them.

    #[test]
    fn relocate_non_call_passes_through() {
        // Opcode 0x0F (not CALL) — the whole word is untouched regardless of base.
        assert_eq!(relocate_call_word(0x0F00_0100, 0x4000), 0x0F00_0100);
        assert_eq!(relocate_call_word(0x0000_0000, 0x4000), 0x0000_0000);
    }

    #[test]
    fn relocate_call_shifts_immediate_by_rom_base() {
        // CALL (0x27) to target 0x000100, loaded at ROM base 0x4000:
        // (0x100 + 0x4000) & 0xFFFFFF = 0x4100, opcode byte preserved.
        assert_eq!(relocate_call_word(0x2700_0100, 0x4000), 0x2700_4100);
    }

    #[test]
    fn relocate_call_base_zero_is_identity() {
        // init loads at ROM base 0 (via populate_rom): CALL immediates unchanged.
        assert_eq!(relocate_call_word(0x2712_3456, 0), 0x2712_3456);
    }

    #[test]
    fn relocate_call_wraps_within_24_bits() {
        // Target 0xFFFFFF + 1 wraps to 0 within the 24-bit immediate; the
        // opcode byte in bits 24..32 is never disturbed by the wrap.
        assert_eq!(relocate_call_word(0x27FF_FFFF, 0x1), 0x2700_0000);
    }

    #[test]
    fn relocate_call_preserves_opcode_byte() {
        // Only bits 0..24 change; bits 24..32 stay 0x27 for any base.
        for base in [0u32, 0x4000, 0x1_0000, 0xFF_FFFF] {
            assert_eq!(relocate_call_word(0x2700_0000, base) >> 24, 0x27);
        }
    }

    #[test]
    fn relocate_rv2mbc_base_zero_is_identity() {
        // init loads at ROM base 0: rv2mbc entries pass through unchanged.
        assert_eq!(relocate_rv2mbc_entry(0x0000_1234, 0), 0x0000_1234);
        assert_eq!(relocate_rv2mbc_entry(0, 0), 0);
    }

    #[test]
    fn relocate_rv2mbc_shifts_entry_by_rom_base() {
        // A resident program at ROM base 0x6000: each MBC PC entry shifts up.
        assert_eq!(relocate_rv2mbc_entry(0x0000_00B3, 0x6000), 0x0000_60B3);
    }

    #[test]
    fn relocate_rv2mbc_zero_entry_also_shifts_documented_behavior() {
        // CHARACTERIZATION: a 0 entry (RV2MBC "unmapped" sentinel) is NOT special-
        // cased — it shifts to rom_base like any other. Pins the current behavior;
        // see relocate_rv2mbc_entry's note. If this ever needs to preserve 0, this
        // test is the tripwire.
        assert_eq!(relocate_rv2mbc_entry(0, 0x6000), 0x6000);
    }

    // ── fnv1a: the exec name-hash. It MUST stay bit-identical to the eBPF
    // exec(7) handler's basename hash (pt::name_hash in monad-cpu-ebpf) — the
    // loader writes PROGRAM_TABLE rows keyed by this hash and exec resolves a
    // path basename to a slot by recomputing it. These pin it to the canonical
    // FNV-1a-32 test vectors (offset basis 0x811c9dc5, prime 0x01000193), so any
    // drift in either implementation is caught here.

    #[test]
    fn fnv1a_empty_is_offset_basis() {
        assert_eq!(fnv1a(b""), 0x811c_9dc5);
    }

    #[test]
    fn fnv1a_matches_canonical_vectors() {
        assert_eq!(fnv1a(b"a"), 0xe40c_292c);
        assert_eq!(fnv1a(b"foobar"), 0xbf9c_f968);
    }

    #[test]
    fn fnv1a_is_order_sensitive() {
        // Distinct program names must hash distinctly (no PROGRAM_TABLE collision
        // for the resident set) and the hash is order-dependent.
        assert_ne!(fnv1a(b"sh"), fnv1a(b"hs"));
        let names: [&[u8]; 5] = [b"sh", b"ls", b"cat", b"echo", b"wc"];
        for i in 0..names.len() {
            for j in (i + 1)..names.len() {
                assert_ne!(fnv1a(names[i]), fnv1a(names[j]), "collision {i} vs {j}");
            }
        }
    }
}
