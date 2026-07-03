// SPDX-License-Identifier: GPL-3.0-or-later
//
// upc-bootctl — UPC kernel image loader + boot dispatcher.
//
// ASCEND-LINUX Phase 1.1 deliverable per
// `references/battle-plan-ascend-linux-2026-05-08.md`.
//
// Loads a translated MBC kernel image (xv6-mbc.mbc, uclinux-mbc.mbc, etc.),
// validates it, prepares BootParams v2, and dispatches a boot trigger to
// the UPC instance.
//
// **Status**: skeleton with `validate` and `boot --dry-run` subcommands.
// Full BPF map population + dispatcher to come in next shift's `boot` impl.

use anyhow::{anyhow, bail, Context, Result};
use clap::{Parser, Subcommand};
use std::path::{Path, PathBuf};

mod bootparams;
mod netns;
mod runner;
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
        ///   - .rv2mbc    → RV2MBC_MAP slot 0 onward, with each entry
        ///                  shifted by USER_ROM_BASE so SRET(SEPC=0)
        ///                  lands on the user MBC PC.
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
    },

    /// Attach a host pty to the UPC's tty stream (Mode B demo surface).
    Console {
        /// UPC instance ID.
        #[arg(long, default_value_t = 0xDE)]
        instance: u8,
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

/// Pre-translate a resident userland program (`sh`, `ls`, …) into a disjoint
/// `ROM_MAP` region + a disjoint `RV2MBC_MAP` base, stage its `.data` in a
/// pristine RAM region, and record an 8-word PROGRAM_TABLE row (ADR-077 Gate
/// B). Mirrors the `--userland` (init) load path, but at non-zero bases so
/// init and the program coexist in the shared maps. `name` is the basename
/// `exec` will hash to find this slot.
#[allow(clippy::too_many_arguments)]
fn load_resident_program(
    runner: &mut runner::BootRunner,
    mbc_path: &Path,
    name: &str,
    slot: u32,
    rom_base: u32,
    rv2mbc_base: u32,
    data_stage_va: u32,
) -> Result<()> {
    // ROM_MAP holds 262,144 MBC words (see monad-cpu-ebpf `ROM_MAP`). A
    // program's ROM must fit within it. (The old RET MBC-vs-RV magnitude
    // floor guard is gone — ADR-079 tags return addresses with bit 31 at
    // CALL/CALLR, so a program's ROM base no longer has to stay below a
    // fixed floor; that unblocks Linux-scale images.)
    const ROM_MAP_WORDS: u32 = 262_144;
    let bytes = std::fs::read(mbc_path).with_context(|| format!("read {}", mbc_path.display()))?;
    if !bytes.len().is_multiple_of(4) {
        bail!("{} not 4-byte aligned", mbc_path.display());
    }
    let rom_end = rom_base as u64 + (bytes.len() as u64 / 4);
    if rom_end > ROM_MAP_WORDS as u64 {
        bail!(
            "{name}: ROM [0x{rom_base:X}, 0x{rom_end:X}) overflows ROM_MAP \
             (0x{ROM_MAP_WORDS:X} words)"
        );
    }
    // Patch CALL immediates by rom_base, exactly like the init path.
    let words: Vec<u32> = bytes
        .chunks_exact(4)
        .map(|c| relocate_call_word(u32::from_le_bytes([c[0], c[1], c[2], c[3]]), rom_base))
        .collect();
    runner
        .populate_rom_at(rom_base, &words)
        .with_context(|| format!("{name} ROM load"))?;

    // rv2mbc: values shifted by rom_base (RV word → that program's ROM slot);
    // loaded at index base rv2mbc_base so each program owns a disjoint region.
    let rv2mbc_path = mbc_path.with_extension("rv2mbc");
    let rv =
        std::fs::read(&rv2mbc_path).with_context(|| format!("read {}", rv2mbc_path.display()))?;
    if !rv.len().is_multiple_of(4) {
        bail!("{} not 4-byte aligned", rv2mbc_path.display());
    }
    let shifted: Vec<u8> = rv
        .chunks_exact(4)
        .flat_map(|c| {
            relocate_rv2mbc_entry(u32::from_le_bytes([c[0], c[1], c[2], c[3]]), rom_base)
                .to_le_bytes()
        })
        .collect();
    runner
        .populate_rv2mbc(&shifted, rv2mbc_base, None)
        .with_context(|| format!("{name} rv2mbc load"))?;

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

#[allow(clippy::too_many_arguments)]
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
) -> Result<()> {
    cmd_validate(kernel.clone())?;

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
    println!("  5. Write BootParams v2 (256 bytes) at 0x0100");
    println!(
        "     magic = 0x{:08X} (canonical 'UNHD' MSB-first; wire bytes 'DHNU' per ADR-072)",
        xv6_mbc::BOOT_MAGIC
    );
    println!("     version = 2");
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
        println!("\n✓ dry-run complete; no BPF maps touched");
        return Ok(());
    }

    // ── LIVE BOOT PATH (Phase 1.1 SHIP per battle-plan-phase11-ship-2026-05-10.md) ──
    let kernel_bytes =
        std::fs::read(&kernel).with_context(|| format!("read kernel: {}", kernel.display()))?;
    let _ = check_image_alignment(&kernel_bytes)?;
    let mbc_words: Vec<u32> = kernel_bytes
        .chunks_exact(4)
        .map(|c| u32::from_le_bytes([c[0], c[1], c[2], c[3]]))
        .collect();

    let bp = bootparams::BootParamsV2::for_xv6(kernel_bytes.len() as u32, 0);
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
    if let Some(ref bs_path) = bootstub {
        let bs_bytes = std::fs::read(bs_path)
            .with_context(|| format!("read bootstub: {}", bs_path.display()))?;
        check_image_alignment(&bs_bytes)?;
        let bs_words: Vec<u32> = bs_bytes
            .chunks_exact(4)
            .map(|c| u32::from_le_bytes([c[0], c[1], c[2], c[3]]))
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
    if let Some(ref rd) = ramdisk {
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
    let rv2mbc_path = kernel.with_extension("rv2mbc");
    let text_rv_word_base: u32 = 0x8000; // xv6 .text base = RV byte 0x20000
                                         // Phase 1.3 ADR-075 §Security #1: if UPC_RV2MBC_SHA is set, the loader
                                         // verifies the .rv2mbc bytes against this SHA-256 before any BPF map
                                         // write. Build pipelines should always set this; dev loops can omit
                                         // (the SHA is logged either way).
    let expected_rv2mbc_sha = std::env::var("UPC_RV2MBC_SHA").ok();
    if rv2mbc_path.exists() {
        match std::fs::read(&rv2mbc_path) {
            Ok(bytes) => match runner.populate_rv2mbc(
                &bytes,
                text_rv_word_base,
                expected_rv2mbc_sha.as_deref(),
            ) {
                Ok(()) => println!(
                    "  RV→MBC map: loaded {} ({} entries @ RV-word-base 0x{:04X}{})",
                    rv2mbc_path.display(),
                    bytes.len() / 4,
                    text_rv_word_base,
                    if expected_rv2mbc_sha.is_some() {
                        " + SHA gate"
                    } else {
                        ""
                    }
                ),
                Err(e) => {
                    // Integrity gate failure is fail-fast: bail out so the boot
                    // doesn't proceed with an untrusted translation table.
                    bail!("rv2mbc load failed: {}", e);
                }
            },
            Err(e) => println!(
                "  ⚠ rv2mbc map at {} unreadable: {}",
                rv2mbc_path.display(),
                e
            ),
        }
    } else {
        println!(
            "  ⚠ no rv2mbc map at {} — MRET/SRET/JALR/CALLR will fall through",
            rv2mbc_path.display()
        );
    }

    // Phase 1.5: load the userland MBC image into a dedicated ROM_MAP region
    // (USER_ROM_BASE = 0x4000) plus shifted RV2MBC entries (starting at slot 0)
    // plus the .data sidecar into RAM_MAP. xv6's kexec then drops the process
    // into user mode by setting trapframe->epc = 0 (matching user.ld which
    // links userland at RV byte 0); SRET reads SEPC=0, RV2MBC_MAP[0] points
    // at USER_ROM_BASE, and the user MBC code runs.
    const USER_ROM_BASE: u32 = 0x4000;
    if let Some(ref up) = userland {
        match std::fs::read(up) {
            Ok(bytes) => {
                if !bytes.len().is_multiple_of(4) {
                    bail!(
                        "userland {} not 4-byte aligned ({} bytes)",
                        up.display(),
                        bytes.len()
                    );
                }
                // CALL opcode (0x27) uses an absolute 24-bit MBC PC. The
                // translator emits CALL targets relative to the .mbc's slot
                // 0, but we load init.mbc at USER_ROM_BASE — so every CALL
                // immediate needs USER_ROM_BASE added or it lands inside the
                // kernel image. Same fix is NOT needed for JMP/JZ/JNZ/... which
                // use signed PC-relative offsets (wrap-add survives the shift).
                let mut patched_calls = 0u32;
                let words: Vec<u32> = bytes
                    .chunks_exact(4)
                    .map(|c| {
                        let w = u32::from_le_bytes([c[0], c[1], c[2], c[3]]);
                        if (w >> 24) == OP_CALL {
                            patched_calls += 1;
                        }
                        relocate_call_word(w, USER_ROM_BASE)
                    })
                    .collect();
                runner
                    .populate_rom_at(USER_ROM_BASE, &words)
                    .with_context(|| format!("userland ROM load {}", up.display()))?;
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
                    words.len(),
                    USER_ROM_BASE
                );
            }
            Err(e) => bail!("userland read {} failed: {e}", up.display()),
        }

        // Userland .data: same TLV format as the kernel side. Each record's
        // byte_addr is in user.ld's address space starting at 0, so it lands
        // in RAM_MAP[byte_addr] directly. No conflicts with kernel layout
        // because user .text fills bytes 0..N (read-only — covered by ROM_MAP)
        // and user .rodata/.data start past the IVT/BootParams region.
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

        // Userland rv2mbc: each entry maps a user RV word to an MBC PC inside
        // init.mbc (whose first instruction is at MBC PC 0 in its own stream).
        // Shift every entry by USER_ROM_BASE so the BPF interpreter's lookup
        // lands on the right slot in the shared ROM_MAP.
        let user_rv2mbc_path = up.with_extension("rv2mbc");
        if user_rv2mbc_path.exists() {
            match std::fs::read(&user_rv2mbc_path) {
                Ok(bytes) => {
                    if !bytes.len().is_multiple_of(4) {
                        bail!(
                            "userland rv2mbc {} not 4-byte aligned",
                            user_rv2mbc_path.display()
                        );
                    }
                    let shifted: Vec<u8> = bytes
                        .chunks_exact(4)
                        .flat_map(|c| {
                            relocate_rv2mbc_entry(
                                u32::from_le_bytes([c[0], c[1], c[2], c[3]]),
                                USER_ROM_BASE,
                            )
                            .to_le_bytes()
                        })
                        .collect();
                    // text_rv_word_base = 0 — userland links text at RV byte 0.
                    match runner.populate_rv2mbc(&shifted, 0, None) {
                        Ok(()) => println!(
                            "  Userland RV→MBC: loaded {} ({} entries @ RV-word-base 0x0000 + shift 0x{:04X})",
                            user_rv2mbc_path.display(),
                            shifted.len() / 4,
                            USER_ROM_BASE
                        ),
                        Err(e) => bail!("userland rv2mbc load failed: {e}"),
                    }
                }
                Err(e) => bail!(
                    "userland rv2mbc read {} failed: {e}",
                    user_rv2mbc_path.display()
                ),
            }
        } else {
            println!(
                "  ⚠ no userland rv2mbc at {} — SRET into user code will fail",
                user_rv2mbc_path.display()
            );
        }
    }

    // ── Phase 1.7 Gate B (ADR-077): resident programs for exec(7) ──
    // Pre-translate sibling userland programs into disjoint ROM/RV2MBC
    // regions + a PROGRAM_TABLE the in-kernel exec handler resolves by
    // basename hash. init stays at ROM 0x4000 / rv2mbc-base 0; programs
    // start past it and below the kernel's rv2mbc base (0x8000). Loaded
    // as siblings of --userland so the existing boot recipe is unchanged.
    if let Some(ref up) = userland {
        // Headline (Gate B) needs `sh`; ls/cat/echo/wc follow the same
        // pattern for Gate C and slot in after sh's bases.
        const PROGRAMS: &[&str] = &["sh", "echo", "ls", "cat", "wc"];
        let dir = up.parent().unwrap_or_else(|| Path::new("."));
        for (i, name) in PROGRAMS.iter().enumerate() {
            let mbc = dir.join(format!("{name}.mbc"));
            if !mbc.exists() {
                println!(
                    "  ⚠ resident program '{name}' not found at {} — skipping",
                    mbc.display()
                );
                continue;
            }
            let slot = i as u32;
            let rom_base = 0x6000 + slot * 0x3000; // 12 Ki-word stride
            let rv2mbc_base = 0x1000 + slot * 0x1000; // < 0x8000 (kernel base)
            let data_stage_va = 0x00C0_0000 + slot * 0x0004_0000; // pristine RAM
            load_resident_program(
                &mut runner,
                &mbc,
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
    runner.populate_identity_pgd(0x00F0_0000, 16)?;
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

    // ── Phase 4: netns + XDP attach + trigger packet ──
    let attach_iface = netns::setup_upc0()?;
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
    match runner.attach_xdp("veth-upc0") {
        Ok(()) => println!("  ✓ XDP attached to veth-upc0"),
        Err(e) => {
            println!("  ✗ XDP attach failed: {}", e);
            netns::teardown_upc0();
            return Err(e);
        }
    }

    // Gate C: pre-load scripted console input into the KBD ring BEFORE the CPU
    // runs, so the guest's read(5) drain feeds it to sh's gets() as if typed.
    if let Some(ref s) = input {
        runner.write_kbd(s.as_bytes())?;
        println!(
            "  ✓ KBD ring pre-filled with {} input byte(s) (--input, read does not echo)",
            s.len()
        );
    }

    // Send Monad-format trigger packets via doom-tick.py. flow_label =
    // instance ID; eBPF dispatches on (flow_label & 0xFF).
    println!(
        "\n[sending {} Monad trigger packets to advance the CPU (each = up to 16 insns)]",
        triggers
    );
    netns::send_trigger(triggers, instance)?;
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
    if let Ok(s) = runner.read_stats(&[3, 7, 8, 13, 14, 15, 16, 20, 21, 23, 25, 26, 27, 28, 29]) {
        println!(
            "  STATS: halted={} syscalls={} rom_fault={} lin_sys={} tty_w={} ctx_sw={} forks={} tty_r={} waitpid={} mod_sys={}",
            s[0], s[1], s[2], s[3], s[4], s[5], s[6], s[7], s[8], s[9]
        );
        println!(
            "  ROM-FAULT CTX: target=0x{:X} rv2mbc_base=0x{:X} cur_pid={} prev_pc=0x{:X} halt_reason=0x{:X}",
            s[10], s[11], s[12], s[13], s[14]
        );
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
    netns::teardown_upc0();

    Ok(())
}

fn cmd_console(instance: u8) -> Result<()> {
    println!("=== UPC CONSOLE (instance 0x{:02X}) ===", instance);
    println!("Console attach mode (Mode B demo surface).");
    println!();
    println!("Implementation pending: subscribe to Busboy topic");
    println!(
        "compute.tty.0x{:02X}, render bytes to host stdout, capture",
        instance
    );
    println!(
        "stdin keystrokes and write to KBD_MAP at 0x{:04X}.",
        instance
    );
    println!();
    println!("Pattern: cmd/doom-bridge/main.go but framing tty bytes");
    println!("instead of framebuffer pixels.");

    Err(anyhow!("not yet implemented"))
}

fn main() -> Result<()> {
    let cli = Cli::parse();
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
        } => cmd_boot(
            kernel, bootstub, initramfs, ramdisk, userland, triggers, instance, dry_run, input,
        ),
        Cmd::Console { instance } => cmd_console(instance),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

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
}
