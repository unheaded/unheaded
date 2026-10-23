// SPDX-License-Identifier: GPL-3.0-or-later
//! `cart` — build, validate and inspect MBC cartridge images (ADR-108).
//!
//! ```text
//! cart build --stage xdp|tick --asm X.s [--data W.bin] [--max-insn N]
//!            [--loop-bound N] [--entry N] [--fail-closed] -o X.cart
//! cart validate X.cart|-     # "-" reads stdin (the loader pipes its in-memory bytes)
//! cart info X.cart|-
//! cart run X.cart --inputs LOG   # reference run of an XDP-stage input log
//! ```
//!
//! `run` input log: one packet per line, `flow now_ns f0 .. f12` (decimal;
//! `flow` is any label, state is kept per label). Output: one
//! `pkt flow=L step=S verdict=V hash=H` line per packet, `emit flow=L kind a b`
//! per EMIT, `queue flow=L f0..f7` per QUEUE, then `final flow=L hash=H` per
//! flow (sorted). `pkg/cartridge` compares BPF runs against this.
//!
//! Exit status: 0 ok, 1 refused (decode or validation error), 2 usage.

use std::io::{Read, Write};
use std::process::ExitCode;

use monad_common::cart::{N_PKT, N_TICK, STAGE_TICK, STAGE_XDP};
use upc_cartridge::{validate, Image, FLAG_FAIL_CLOSED};

const MAX_INPUT: u64 = 1 << 20; // a .cart is at most 52 + 8 KiB; refuse anything absurd

fn usage() -> ExitCode {
    eprintln!(
        "usage:\n  cart build --stage xdp|tick --asm X.s [--data W.bin] [--max-insn N] [--loop-bound N] [--entry N] [--fail-closed] -o X.cart\n  cart validate X.cart|-\n  cart info X.cart|-\n  cart run X.cart --inputs LOG"
    );
    ExitCode::from(2)
}

fn read_input(path: &str) -> Result<Vec<u8>, String> {
    let mut buf = Vec::new();
    if path == "-" {
        std::io::stdin()
            .take(MAX_INPUT + 1)
            .read_to_end(&mut buf)
            .map_err(|e| format!("stdin: {e}"))?;
    } else {
        let f = std::fs::File::open(path).map_err(|e| format!("{path}: {e}"))?;
        f.take(MAX_INPUT + 1)
            .read_to_end(&mut buf)
            .map_err(|e| format!("{path}: {e}"))?;
    }
    if buf.len() as u64 > MAX_INPUT {
        return Err(format!("{path}: larger than {MAX_INPUT} bytes"));
    }
    Ok(buf)
}

fn hex(b: &[u8]) -> String {
    b.iter().map(|x| format!("{x:02x}")).collect()
}

fn parse_u16(s: Option<&String>, name: &str) -> Result<u16, String> {
    let s = s.ok_or_else(|| format!("{name} needs a value"))?;
    let v = if let Some(h) = s.strip_prefix("0x") {
        u16::from_str_radix(h, 16)
    } else {
        s.parse()
    };
    v.map_err(|_| format!("{name}: bad number {s:?}"))
}

fn build(args: &[String]) -> Result<(), String> {
    let (mut stage, mut asm, mut data, mut out) = (None, None, None, None);
    let (mut max_insn, mut loop_bound, mut entry, mut flags) = (None, 0u16, 0u16, 0u16);
    let mut it = args.iter();
    while let Some(a) = it.next() {
        match a.as_str() {
            "--stage" => {
                stage = Some(match it.next().map(String::as_str) {
                    Some("xdp") => STAGE_XDP,
                    Some("tick") => STAGE_TICK,
                    other => return Err(format!("--stage: want xdp or tick, got {other:?}")),
                })
            }
            "--asm" => asm = it.next().cloned(),
            "--data" => data = it.next().cloned(),
            "-o" => out = it.next().cloned(),
            "--max-insn" => max_insn = Some(parse_u16(it.next(), "--max-insn")?),
            "--loop-bound" => loop_bound = parse_u16(it.next(), "--loop-bound")?,
            "--entry" => entry = parse_u16(it.next(), "--entry")?,
            "--fail-closed" => flags |= FLAG_FAIL_CLOSED,
            other => return Err(format!("unknown argument {other:?}")),
        }
    }
    let stage = stage.ok_or("--stage is required")?;
    let asm = asm.ok_or("--asm is required")?;
    let out = out.ok_or("-o is required")?;
    let src = String::from_utf8(read_input(&asm)?).map_err(|_| format!("{asm}: not UTF-8"))?;
    let rom = monad_mbc::assembler::assemble(&src).map_err(|e| format!("{asm}: {e}"))?;
    let data = match data {
        None => vec![],
        Some(p) => {
            let b = read_input(&p)?;
            if b.len() % 4 != 0 {
                return Err(format!(
                    "{p}: {} bytes is not a whole number of words",
                    b.len()
                ));
            }
            b.as_chunks::<4>()
                .0
                .iter()
                .map(|c| u32::from_le_bytes(*c))
                .collect()
        }
    };
    if rom.is_empty() || rom.len() > 1024 || data.len() > 1024 {
        return Err(format!(
            "rom {} words (1..=1024), data {} words (<=1024)",
            rom.len(),
            data.len()
        ));
    }
    let cap = if stage == STAGE_XDP { N_PKT } else { N_TICK } as u16;
    let img = Image {
        stage,
        max_insn: max_insn.unwrap_or(cap),
        entry,
        loop_bound,
        flags,
        rom,
        data,
    };
    let report = validate(&img).map_err(|e| format!("{asm}: {e}"))?;
    let bytes = img.encode();
    std::fs::write(&out, &bytes).map_err(|e| format!("{out}: {e}"))?;
    println!(
        "{out}: stage={} rom={} data={} max_insn={} back_edges={} syscalls={:?} bytes={} sha256={}",
        if stage == STAGE_XDP { "xdp" } else { "tick" },
        img.rom.len(),
        img.data.len(),
        report.max_insn,
        report.has_back_edges,
        report.syscalls,
        bytes.len(),
        hex(&img.sha256())
    );
    Ok(())
}

fn check(path: &str, print_info: bool) -> Result<(), String> {
    let b = read_input(path)?;
    let img = Image::decode(&b).map_err(|e| format!("{path}: {e}"))?;
    let r = validate(&img).map_err(|e| format!("{path}: {e}"))?;
    let mut o = std::io::stdout().lock();
    let stage = if img.stage == STAGE_XDP {
        "xdp"
    } else {
        "tick"
    };
    if print_info {
        let _ = writeln!(
            o,
            "stage={stage} rom={} data={} entry={} max_insn={} loop_bound={} fail_closed={} sha256={}",
            img.rom.len(),
            img.data.len(),
            img.entry,
            img.max_insn,
            img.loop_bound,
            img.flags & FLAG_FAIL_CLOSED != 0,
            hex(&img.sha256())
        );
    } else {
        let _ = writeln!(
            o,
            "OK stage={stage} max_insn={} back_edges={} syscalls={:?} sha256={}",
            r.max_insn,
            r.has_back_edges,
            r.syscalls,
            hex(&img.sha256())
        );
    }
    Ok(())
}

fn run_log(img_path: &str, log_path: &str) -> Result<(), String> {
    use upc_cartridge::reference::{run_packet, xdp_verdict, PacketInput};
    let img = Image::decode(&read_input(img_path)?).map_err(|e| format!("{img_path}: {e}"))?;
    validate(&img).map_err(|e| format!("{img_path}: {e}"))?;
    if img.stage != STAGE_XDP {
        return Err("run replays XDP-stage cartridges".into());
    }
    let text =
        String::from_utf8(read_input(log_path)?).map_err(|_| format!("{log_path}: not UTF-8"))?;
    let mut states: std::collections::BTreeMap<String, upc_cartridge::CartState> =
        Default::default();
    let mut o = std::io::stdout().lock();
    for (n, line) in text.lines().enumerate() {
        let t: Vec<&str> = line.split_whitespace().collect();
        if t.is_empty() {
            continue;
        }
        if t.len() != 15 {
            return Err(format!(
                "{log_path}:{}: want `flow now f0..f12`, got {} fields",
                n + 1,
                t.len()
            ));
        }
        let num = |s: &str| {
            s.parse::<u64>()
                .map_err(|_| format!("{log_path}:{}: bad number {s:?}", n + 1))
        };
        let mut inp = PacketInput {
            now_ns: num(t[1])?,
            ..PacketInput::default()
        };
        for (i, f) in inp.fields.iter_mut().enumerate() {
            *f = num(t[2 + i])? as u32;
        }
        let st = states
            .entry(t[0].to_string())
            .or_insert_with(upc_cartridge::CartState::zeroed);
        let out = run_packet(&img, st, &inp);
        let step = out.step.unwrap_or(upc_cartridge::Step::Continue);
        for (k, a, b) in &out.emits {
            let _ = writeln!(o, "emit flow={} {k} {a} {b}", t[0]);
        }
        for q in &out.queued {
            let fs: Vec<String> = q.iter().map(|x| x.to_string()).collect();
            let _ = writeln!(o, "queue flow={} {}", t[0], fs.join(" "));
        }
        let _ = writeln!(
            o,
            "pkt flow={} step={step:?} verdict={} hash={:08x}",
            t[0],
            xdp_verdict(&img, step),
            upc_cartridge::state_hash(st)
        );
    }
    for (flow, st) in &states {
        let _ = writeln!(
            o,
            "final flow={flow} hash={:08x}",
            upc_cartridge::state_hash(st)
        );
    }
    Ok(())
}

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let res = match args.first().map(String::as_str) {
        Some("build") => build(&args[1..]),
        Some("validate") if args.len() == 2 => check(&args[1], false),
        Some("info") if args.len() == 2 => check(&args[1], true),
        Some("run") if args.len() == 4 && args[2] == "--inputs" => run_log(&args[1], &args[3]),
        _ => return usage(),
    };
    match res {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("cart: {e}");
            ExitCode::from(1)
        }
    }
}
