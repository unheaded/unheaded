// SPDX-License-Identifier: GPL-3.0-or-later
//! S5 Phase 16 (Steps 401-405, 410-411): Anamnesis replay on the host.
//! Recorded runs (host recorder, the same records cartridge-ebpf writes)
//! replay bit for bit; a corrupted input byte or a different image is
//! flagged at the exact instruction; the trace format survives hostile bytes.

use monad_common::cart::{
    field, CartState, FlowKey, Step, FEAT_WORDS, FIELD_COUNT, STAGE_TICK, STAGE_XDP,
};
use upc_cartridge::reference::{begin_tick_job, FlowMetaSim};
use upc_cartridge::replay::{head, record, replay, Trace, TraceError};
use upc_cartridge::scorer::Rng;
use upc_cartridge::{build_image, data_words, Image};

fn repo(p: &str) -> String {
    format!("{}/../../{p}", env!("CARGO_MANIFEST_DIR"))
}

fn image(stage: u8, asm: &str, data: &str, loop_bound: u16) -> Image {
    let src = std::fs::read_to_string(repo(asm)).unwrap();
    let d = data_words(&std::fs::read(repo(data)).unwrap()).unwrap();
    build_image(stage, &src, d, None, loop_bound).unwrap()
}

fn ratelimit() -> Image {
    image(
        STAGE_XDP,
        "cartridges/ratelimit.s",
        "cartridges/ratelimit.data.bin",
        0,
    )
}

fn scorer() -> Image {
    image(
        STAGE_TICK,
        "cartridges/scorer.s",
        "cartridges/scorer.data.bin",
        16,
    )
}

/// A 200-packet rate-limited flow recorded run by run, as cart_xdp would:
/// each run starts after begin_run from the flow's persisted state.
fn ratelimit_trace() -> (Image, Trace) {
    let img = ratelimit();
    let mut st = CartState::zeroed();
    let mut meta = FlowMetaSim::default();
    let mut rng = Rng(42);
    let mut now = 5_000_000_000u64;
    let mut t = Trace::default();
    for i in 0..200u64 {
        now += rng.below(40_000);
        let mut f = [0u32; FIELD_COUNT as usize];
        f[field::L3_PROTO as usize] = 4;
        f[field::L4_PROTO as usize] = 17;
        f[field::PKT_LEN as usize] = 60 + rng.below(1400) as u32;
        let inp = meta.packet(now, f);
        monad_common::cart::begin_run(&mut st, img.entry);
        let h = head(
            STAGE_XDP,
            0,
            1,
            1000 + i,
            now,
            FlowKey::default(),
            inp.fields,
            st,
        );
        let run = record(&img, &h);
        // Advance the live state exactly as the run did.
        let r = replay(&img, &run);
        st = *r.states.last().unwrap();
        t.runs.push(run);
    }
    (img, t)
}

#[test]
fn replay_ratelimit_is_bit_exact() {
    let (img, t) = ratelimit_trace();
    // Through the file format and back.
    let t2 = Trace::parse(&t.encode()).unwrap();
    assert_eq!(t2, t);
    let mut steps = 0;
    let mut drops = 0;
    for run in &t2.runs {
        let r = replay(&img, run);
        assert!(r.bit_exact(), "{:?}", r.divergence);
        steps += r.len();
        drops += (r.outcome == Step::Verdict(1)) as usize;
    }
    assert!(drops > 0, "the trace must exercise DROP");
    eprintln!(
        "REPLAY ratelimit: {} runs, {steps} steps, every state hash reproduced ({drops} drops)",
        t2.runs.len()
    );
}

/// Corrupting one byte of a run's recorded input (its pkt_len FIELD) is
/// flagged at the first instruction that reads it, with both hashes.
#[test]
fn replay_flags_a_corrupted_input_at_the_exact_step() {
    let (img, t) = ratelimit_trace();
    let mut bytes = t.encode();
    // Run 7's head: file header 136 B, then (u32 len + record)*. Find it.
    let mut off = 8 + 4 * 32;
    let mut run = 0;
    let head_off = loop {
        let len = u32::from_le_bytes(bytes[off..off + 4].try_into().unwrap()) as usize;
        if len == 464 {
            if run == 7 {
                break off + 4;
            }
            run += 1;
        }
        off += 4 + len;
    };
    // fields start at byte 72 of the head; pkt_len is FIELD 4.
    bytes[head_off + 72 + 4 * field::PKT_LEN as usize] ^= 0x40;
    let t = Trace::parse(&bytes).unwrap();
    let r = replay(&img, &t.runs[7]);
    let d = r.divergence.clone().expect("divergence");
    // The first FIELD read of pkt_len: SYSCALL 0 at pc 2 (instruction 2).
    let first_field = t.runs[7]
        .steps
        .iter()
        .position(|s| s.insn == 0x4000_0000) // SYSCALL 0 (FIELD)
        .unwrap();
    assert_eq!((d.idx, d.what), (first_field, "hash"), "{d:?}");
    assert_ne!(d.recorded, d.replayed);
    // Every other run still replays.
    for (i, run) in t.runs.iter().enumerate() {
        if i != 7 {
            assert!(replay(&img, run).bit_exact(), "run {i}");
        }
    }
    eprintln!(
        "REPLAY corrupted input: diverged at step {} ({}), recorded {:08x} replayed {:08x}",
        d.idx, d.what, d.recorded, d.replayed
    );
}

#[test]
fn replay_flags_a_different_image_and_a_corrupted_hash() {
    let (_, t) = ratelimit_trace();
    let other = image(
        STAGE_XDP,
        "cartridges/sampler.s",
        "cartridges/sampler.data.bin",
        0,
    );
    let d = replay(&other, &t.runs[0]).divergence.unwrap();
    assert_eq!((d.idx, d.what), (0, "insn"));
    let img = ratelimit();
    let mut run = t.runs[3].clone();
    run.steps[5].hash ^= 1;
    let d = replay(&img, &run).divergence.unwrap();
    assert_eq!((d.idx, d.what), (5, "hash"));
}

/// A scorer job (3 tick calls) replays call by call; time travel shows the
/// output accumulator (r13) evolving and the score in r2 at the EMIT.
#[test]
fn replay_scorer_time_travel() {
    let img = scorer();
    let feat: [u32; FEAT_WORDS] = [480, 8, 8, 60, 8, 6, 80, 2]; // SYN-flood window
    let mut st = CartState::zeroed();
    begin_tick_job(&img, &mut st, &feat);
    let mut runs = vec![];
    loop {
        let h = head(
            STAGE_TICK,
            2,
            1,
            runs.len() as u64 + 1,
            0,
            FlowKey::default(),
            [0; FIELD_COUNT as usize],
            st,
        );
        let run = record(&img, &h);
        let mut r = replay(&img, &run);
        assert!(r.bit_exact(), "{:?}", r.divergence);
        st = *r.states.last().unwrap();
        let out = r.outcome;
        runs.push(r.clone());
        if out != Step::Yield {
            assert_eq!(out, Step::Halt);
            // Time travel within the last call: back from the end to the
            // EMIT, read the score; r13 history is visible step by step.
            let emit = r
                .steps
                .iter()
                .rposition(|s| s.insn >> 24 == 0x40)
                .expect("EMIT");
            r.goto(emit);
            let score = r.regs()[2];
            assert!(r.step());
            assert!(r.back());
            assert_eq!(r.regs()[2], score);
            assert_eq!(r.emits, vec![(0x41, score, r.regs()[3])]);
            assert!(score >= 90, "SYN flood window scored {score}");
            eprintln!("REPLAY scorer: {}", r.inspect());
            break;
        }
    }
    assert_eq!(runs.len(), 3);
    let mut r13 = vec![];
    for r in &mut runs {
        for i in 0..=r.len() {
            r.goto(i);
            let v = r.regs()[13];
            if r13.last() != Some(&v) {
                r13.push(v);
            }
        }
    }
    assert!(r13.len() > 8, "accumulator changes: {r13:?}");
    eprintln!("REPLAY scorer accumulator r13 took {} values", r13.len());
}

#[test]
fn trace_parse_refuses_hostile_bytes() {
    let (_, t) = ratelimit_trace();
    let good = t.encode();
    assert_eq!(Trace::parse(&good[..10]), Err(TraceError::Truncated));
    let mut bad = good.clone();
    bad[0] = b'X';
    assert_eq!(Trace::parse(&bad), Err(TraceError::BadHeader));
    let mut bad = good.clone();
    bad[136 + 4] = 0x02; // first record's event byte
    assert!(matches!(Trace::parse(&bad), Err(TraceError::BadRecord(0))));
    assert_eq!(
        Trace::parse(&good[..good.len() - 3]),
        Err(TraceError::Truncated)
    );
    // Random bytes behind a valid header: an error or a trace, never a panic.
    let mut rng = Rng(9);
    for _ in 0..5_000 {
        let n = rng.below(600) as usize;
        let mut b = good[..136].to_vec();
        b.extend((0..n).map(|_| rng.next() as u8));
        let _ = Trace::parse(&b);
    }
}
