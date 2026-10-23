// SPDX-License-Identifier: GPL-3.0-or-later
//! `cart` — build, validate and inspect MBC cartridge images (ADR-108).
//!
//! ```text
//! cart build --stage xdp|tick --asm X.s [--data W.bin] [--max-insn N]
//!            [--loop-bound N] [--entry N] [--fail-closed] -o X.cart
//! cart validate X.cart|-     # "-" reads stdin (the loader pipes its in-memory bytes)
//! cart info X.cart|-
//! cart run X.cart --inputs LOG   # reference run of an XDP-stage input log
//! cart tick X.cart --jobs LOG    # reference run of tick-stage jobs (label f0..f7 per line)
//! cart replay X.cart TRACE [--slot S] [--run K] [--to N] [--watch rN|mem[A]] [--steps]
//!                                # Anamnesis replay of recorded runs (exit 3 on divergence)
//! cart diff [--seed HEX] [--count N] [--out DIR] [--sport-base P] [--now-base NS]
//!                                # differential: shared step vs host MBC emulator in lockstep
//!                                # on random cartridges (exit 3 on divergence)
//! ```
//!
//! `diff` without `--out` draws random inputs. With `--out` every case gets
//! the input `pkg/cartridge`'s three-engine test sends through BPF (case i:
//! UDP 10.0.0.1:P+i -> 10.0.0.2:53, 80 bytes, first packet of the flow, clock
//! NS + i*2 s) and writes `case-NNNN.cart` plus `manifest.txt`, one line per
//! case: `case-NNNN.cart sport=P now=NS end=E verdict=V hash=H emits=K:A:B,..|-`.
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
        "usage:\n  cart build --stage xdp|tick --asm X.s [--data W.bin] [--max-insn N] [--loop-bound N] [--entry N] [--fail-closed] -o X.cart\n  cart validate X.cart|-\n  cart info X.cart|-\n  cart run X.cart --inputs LOG\n  cart tick X.cart --jobs LOG\n  cart replay X.cart TRACE [--slot S] [--run K] [--to N] [--watch rN|mem[A]] [--steps]\n  cart diff [--seed HEX] [--count N] [--out DIR] [--sport-base P] [--now-base NS]"
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

/// Reference run of tick-stage jobs: one job per line, `label f0 .. f7`.
/// Output per job: `emit job=L kind a b` per EMIT, then
/// `job job=L step=S calls=C hash=H` (H = state hash after the last call).
fn tick_log(img_path: &str, log_path: &str) -> Result<(), String> {
    use upc_cartridge::reference::run_tick_job;
    let img = Image::decode(&read_input(img_path)?).map_err(|e| format!("{img_path}: {e}"))?;
    validate(&img).map_err(|e| format!("{img_path}: {e}"))?;
    if img.stage != STAGE_TICK {
        return Err("tick replays tick-stage cartridges".into());
    }
    let text =
        String::from_utf8(read_input(log_path)?).map_err(|_| format!("{log_path}: not UTF-8"))?;
    let mut o = std::io::stdout().lock();
    for (n, line) in text.lines().enumerate() {
        let t: Vec<&str> = line.split_whitespace().collect();
        if t.is_empty() {
            continue;
        }
        if t.len() != 1 + monad_common::cart::FEAT_WORDS {
            return Err(format!("{log_path}:{}: want `label f0..f7`", n + 1));
        }
        let mut feat = [0u32; monad_common::cart::FEAT_WORDS];
        for (i, f) in feat.iter_mut().enumerate() {
            *f = t[1 + i]
                .parse()
                .map_err(|_| format!("{log_path}:{}: bad number {:?}", n + 1, t[1 + i]))?;
        }
        // One call per N_TICK budget; a job that still yields after 64 calls
        // is reported as such (a tick cartridge's loop_bound bounds it).
        let (st, calls) = run_tick_job(&img, &feat, 0, 64);
        for c in &calls {
            for (k, a, b) in &c.emits {
                let _ = writeln!(o, "emit job={} {k} {a} {b}", t[0]);
            }
        }
        let step = calls
            .last()
            .and_then(|c| c.step)
            .unwrap_or(upc_cartridge::Step::Continue);
        let _ = writeln!(
            o,
            "job job={} step={step:?} calls={} hash={:08x}",
            t[0],
            calls.len(),
            upc_cartridge::state_hash(&st)
        );
    }
    Ok(())
}

/// What `--watch` follows.
enum Watch {
    Reg(usize),
    Mem(usize),
}

fn parse_watch(s: &str) -> Result<Watch, String> {
    if let Some(r) = s.strip_prefix('r') {
        let n: usize = r
            .parse()
            .map_err(|_| format!("--watch {s}: want rN or mem[ADDR]"))?;
        if n < 16 {
            return Ok(Watch::Reg(n));
        }
    }
    if let Some(a) = s.strip_prefix("mem[").and_then(|x| x.strip_suffix(']')) {
        let addr = if let Some(h) = a.strip_prefix("0x") {
            usize::from_str_radix(h, 16)
        } else {
            a.parse()
        }
        .map_err(|_| format!("--watch {s}: bad address"))?;
        if addr < 0x100 && addr % 4 == 0 {
            return Ok(Watch::Mem(addr / 4));
        }
    }
    Err(format!(
        "--watch {s}: want r0..r15 or mem[ADDR] (ADDR < 0x100, 4-aligned)"
    ))
}

/// Anamnesis replay (S5 Phase 16): re-execute recorded runs of the slot that
/// ran `img_path` and check every state hash.
///
/// `cart replay X.cart TRACE [--slot S] [--run K] [--to N] [--watch rN|mem[A]] [--steps]`
/// Exit status: 0 every complete run bit-exact, 3 a divergence, 1/2 errors.
fn replay_cmd(args: &[String]) -> Result<bool, String> {
    use sha2::{Digest, Sha256};
    use upc_cartridge::replay::{disasm, replay, Trace};
    let (img_path, trace_path) = match args {
        [a, b, ..] => (a.as_str(), b.as_str()),
        _ => return Err("replay needs X.cart and a trace".into()),
    };
    let (mut slot, mut only, mut to, mut watch, mut steps) = (None, None, None, None, false);
    let mut it = args[2..].iter();
    while let Some(a) = it.next() {
        let mut val = || it.next().ok_or_else(|| format!("{a} needs a value"));
        match a.as_str() {
            "--slot" => slot = Some(parse_u16(Some(val()?), "--slot")? as u8),
            "--run" => only = Some(parse_u16(Some(val()?), "--run")? as usize),
            "--to" => to = Some(parse_u16(Some(val()?), "--to")? as usize),
            "--watch" => watch = Some(parse_watch(val()?)?),
            "--steps" => steps = true,
            other => return Err(format!("unknown argument {other:?}")),
        }
    }
    let raw = read_input(img_path)?;
    let img = Image::decode(&raw).map_err(|e| format!("{img_path}: {e}"))?;
    validate(&img).map_err(|e| format!("{img_path}: {e}"))?;
    let file_sha: [u8; 32] = Sha256::digest(&raw).into();
    let trace = {
        let f = std::fs::File::open(trace_path).map_err(|e| format!("{trace_path}: {e}"))?;
        let mut b = Vec::new();
        std::io::Read::take(f, upc_cartridge::replay::MAX_TRACE as u64 + 1)
            .read_to_end(&mut b)
            .map_err(|e| format!("{trace_path}: {e}"))?;
        Trace::parse(&b).map_err(|e| format!("{trace_path}: {e}"))?
    };
    let slot =
        match slot {
            Some(s) if (s as usize) < 4 => s,
            Some(s) => return Err(format!("--slot {s} outside 0..3")),
            None => trace.slot_sha.iter().position(|s| *s == file_sha).ok_or(
                "this image is not the one any slot ran in the trace (use --slot to force)",
            )? as u8,
        };
    let rec_sha = trace.slot_sha[slot as usize];
    if rec_sha != [0u8; 32] && rec_sha != file_sha {
        eprintln!(
            "cart: warning: slot {slot} recorded image sha256 {} differs from {img_path}",
            hex(&rec_sha)
        );
    }
    let mut o = std::io::stdout().lock();
    let (mut exact, mut diverged, mut incomplete) = (0, 0, 0);
    let runs: Vec<_> = trace.runs.iter().filter(|r| r.head.slot == slot).collect();
    for (k, run) in runs.iter().enumerate() {
        if only.is_some_and(|n| n != k) {
            continue;
        }
        let mut r = replay(&img, run);
        let stage = if run.head.stage == STAGE_XDP {
            "xdp"
        } else {
            "tick"
        };
        let verdict = if run.end.is_none() {
            incomplete += 1;
            "INCOMPLETE (records dropped)".to_string()
        } else {
            match &r.divergence {
                None => {
                    exact += 1;
                    "BIT-EXACT".to_string()
                }
                Some(d) => {
                    diverged += 1;
                    format!(
                        "DIVERGED at step {} ({}): recorded {:#010x}, replayed {:#010x}",
                        d.idx, d.what, d.recorded, d.replayed
                    )
                }
            }
        };
        let _ = writeln!(
            o,
            "run {k} id={:#x} stage={stage} slot={} gen={} steps={} outcome={:?} emits={:?} {verdict}",
            run.head.run_id,
            run.head.slot,
            run.head.gen,
            r.len(),
            r.outcome,
            r.emits
        );
        if steps {
            for (i, s) in r.steps.iter().enumerate() {
                let _ = writeln!(
                    o,
                    "  {i:>4} pc {:>3} {:<28} -> {:?} hash {:08x}",
                    s.pc,
                    disasm(s.insn, s.pc),
                    monad_common::cart::code_step(s.code),
                    s.hash
                );
            }
        }
        if let Some(w) = &watch {
            let mut last = None;
            for i in 0..=r.len() {
                r.goto(i);
                let (name, v) = match w {
                    Watch::Reg(n) => (format!("r{n}"), r.regs()[*n]),
                    Watch::Mem(n) => (format!("mem[{:#04x}]", n * 4), r.mem()[*n]),
                };
                if last != Some(v) {
                    let _ = writeln!(o, "  @{i:<4} {name} = {v:#x} ({v})");
                    last = Some(v);
                }
            }
        }
        if let Some(n) = to {
            r.goto(n);
            let _ = writeln!(o, "{}", r.inspect());
        }
    }
    let _ = writeln!(
        o,
        "replayed {} runs of slot {slot}: {exact} bit-exact, {diverged} diverged, {incomplete} incomplete ({} orphan records)",
        exact + diverged + incomplete,
        trace.orphans
    );
    Ok(diverged == 0)
}

/// `cart diff`: the S5 Phase 17 differential (shared step vs host MBC
/// emulator, lockstep). Returns Ok(false) on any divergence.
fn diff_cmd(args: &[String]) -> Result<bool, String> {
    use upc_cartridge::diff::{bpf_case_input, gen_input, gen_xdp_cartridge, lockstep, XorShift};
    let (mut seed, mut count, mut out) = (0xC3C3_5EEDu64, 1000u32, None::<String>);
    let (mut sport_base, mut now_base) = (10_000u32, 7_000_000_000u64);
    let num = |s: Option<&String>, name: &str| -> Result<u64, String> {
        let s = s.ok_or_else(|| format!("{name} needs a value"))?;
        let v = match s.strip_prefix("0x") {
            Some(h) => u64::from_str_radix(h, 16),
            None => s.parse(),
        };
        v.map_err(|_| format!("{name}: bad number {s:?}"))
    };
    let mut it = args.iter();
    while let Some(a) = it.next() {
        match a.as_str() {
            "--seed" => seed = num(it.next(), "--seed")?,
            "--count" => count = num(it.next(), "--count")?.min(100_000) as u32,
            "--out" => out = it.next().cloned(),
            "--sport-base" => sport_base = num(it.next(), "--sport-base")?.min(65_535) as u32,
            "--now-base" => now_base = num(it.next(), "--now-base")?,
            other => return Err(format!("unknown argument {other:?}")),
        }
    }
    if out.is_some() && sport_base + count > 65_536 {
        return Err("--sport-base + --count must stay within the UDP port range".into());
    }
    if let Some(dir) = &out {
        std::fs::create_dir_all(dir).map_err(|e| format!("{dir}: {e}"))?;
    }
    let mut e = XorShift::new(seed);
    let mut manifest = String::new();
    let (mut steps, mut diverged) = (0usize, 0u32);
    let mut ends = std::collections::BTreeMap::<String, u32>::new();
    let o = &mut std::io::stdout().lock();
    for i in 0..count {
        let img = gen_xdp_cartridge(&mut e);
        let (sport, now) = (sport_base + i, now_base + i as u64 * 2_000_000_000);
        let input = match out {
            Some(_) => bpf_case_input(sport as u16, now),
            None => gen_input(&mut e),
        };
        validate(&img).map_err(|e| format!("case {i}: generator bug, image rejected: {e}"))?;
        let raw = img.encode();
        match lockstep(&img, &input) {
            Ok(a) => {
                steps += a.steps;
                let kind = a.end.to_string();
                let kind = kind.split(':').next().unwrap_or("").to_string();
                *ends.entry(kind).or_default() += 1;
                if let Some(dir) = &out {
                    let name = format!("case-{i:04}.cart");
                    std::fs::write(format!("{dir}/{name}"), &raw)
                        .map_err(|e| format!("{dir}/{name}: {e}"))?;
                    let emits = if a.emits.is_empty() {
                        "-".to_string()
                    } else {
                        a.emits
                            .iter()
                            .map(|(k, x, y)| format!("{k}:{x}:{y}"))
                            .collect::<Vec<_>>()
                            .join(",")
                    };
                    manifest.push_str(&format!(
                        "{name} sport={sport} now={now} end={} verdict={} hash={:08x} emits={emits}\n",
                        a.end,
                        a.end.xdp_verdict(),
                        upc_cartridge::state_hash(&a.state)
                    ));
                }
            }
            Err(d) => {
                diverged += 1;
                let _ = writeln!(o, "case {i}: {d}\n  image {}", hex(&raw));
                if let Some(dir) = &out {
                    let _ = std::fs::write(format!("{dir}/diverged-{i:04}.cart"), &raw);
                }
            }
        }
    }
    if let Some(dir) = &out {
        std::fs::write(format!("{dir}/manifest.txt"), manifest)
            .map_err(|e| format!("{dir}/manifest.txt: {e}"))?;
    }
    let ends: Vec<String> = ends.iter().map(|(k, v)| format!("{k} {v}")).collect();
    let _ = writeln!(
        o,
        "diff: {count} cartridges (seed {seed:#x}), {steps} instructions compared, {diverged} diverged; ends: {}",
        ends.join(", ")
    );
    Ok(diverged == 0)
}

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("replay") {
        return match replay_cmd(&args[1..]) {
            Ok(true) => ExitCode::SUCCESS,
            Ok(false) => ExitCode::from(3),
            Err(e) => {
                eprintln!("cart: {e}");
                ExitCode::from(1)
            }
        };
    }
    if args.first().map(String::as_str) == Some("diff") {
        return match diff_cmd(&args[1..]) {
            Ok(true) => ExitCode::SUCCESS,
            Ok(false) => ExitCode::from(3),
            Err(e) => {
                eprintln!("cart: {e}");
                ExitCode::from(1)
            }
        };
    }
    let res = match args.first().map(String::as_str) {
        Some("build") => build(&args[1..]),
        Some("validate") if args.len() == 2 => check(&args[1], false),
        Some("info") if args.len() == 2 => check(&args[1], true),
        Some("run") if args.len() == 4 && args[2] == "--inputs" => run_log(&args[1], &args[3]),
        Some("tick") if args.len() == 4 && args[2] == "--jobs" => tick_log(&args[1], &args[3]),
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
