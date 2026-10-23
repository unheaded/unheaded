// SPDX-License-Identifier: GPL-3.0-or-later
//! Reference runner: drives `monad_common::cart` exactly as `cartridge-ebpf`
//! does (same `begin_run` / `begin_job` / `run::<N>` calls), with inputs from
//! an explicit input log instead of a packet. Same image + same inputs →
//! same state hash, on any engine that runs the shared step.

use monad_common::cart::{
    begin_job, begin_run, run, verdict, CartIo, CartState, Step, FEAT_WORDS, FIELD_COUNT, N_PKT,
    N_TICK, STAGE_XDP,
};

use crate::{Image, FLAG_FAIL_CLOSED};

/// Everything an XDP-stage run may read: the FIELD table and the clock.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct PacketInput {
    /// FIELD values by id (ADR-106 table).
    pub fields: [u32; FIELD_COUNT as usize],
    /// Clock of the run (NOW_NS_LO returns the low 32 bits).
    pub now_ns: u64,
}

/// Side effects of one run or call.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Outcome {
    /// How the run stopped (`Step::Continue` never appears here).
    pub step: Option<Step>,
    /// EMIT records `(kind, a, b)` in order.
    pub emits: Vec<(u32, u32, u32)>,
    /// QUEUE snapshots in order.
    pub queued: Vec<[u32; FEAT_WORDS]>,
}

struct LogIo<'a> {
    input: &'a PacketInput,
    out: &'a mut Outcome,
}

impl CartIo for LogIo<'_> {
    fn field(&mut self, id: u32) -> Option<u32> {
        self.input.fields.get(id as usize).copied()
    }
    fn now_ns_lo(&mut self) -> u32 {
        self.input.now_ns as u32
    }
    fn emit(&mut self, kind: u32, a: u32, b: u32) {
        self.out.emits.push((kind, a, b));
    }
    fn queue(&mut self, feat: &[u32; FEAT_WORDS]) {
        self.out.queued.push(*feat);
    }
}

/// One XDP-stage run for one packet. `st` carries the flow's state between
/// packets (its `mem` persists; registers reset).
pub fn run_packet(img: &Image, st: &mut CartState, input: &PacketInput) -> Outcome {
    let cfg = img.cfg();
    let mut out = Outcome::default();
    begin_run(st, img.entry);
    let step = {
        let mut io = LogIo {
            input,
            out: &mut out,
        };
        run::<N_PKT, _, _, _>(st, &cfg, &img.rom[..], &img.data[..], &mut io)
    };
    out.step = Some(step);
    out
}

/// Start a tick job: `mem[0..8]` = the job's features.
pub fn begin_tick_job(img: &Image, st: &mut CartState, feat: &[u32; FEAT_WORDS]) {
    begin_job(st, img.entry, feat);
}

/// One `cart_tick` call on a job already begun (or yielded).
pub fn run_tick_call(img: &Image, st: &mut CartState, now_ns: u64) -> Outcome {
    let cfg = img.cfg();
    let input = PacketInput {
        now_ns,
        ..PacketInput::default()
    };
    let mut out = Outcome::default();
    let step = {
        let mut io = LogIo {
            input: &input,
            out: &mut out,
        };
        run::<N_TICK, _, _, _>(st, &cfg, &img.rom[..], &img.data[..], &mut io)
    };
    out.step = Some(step);
    out
}

/// A whole tick job: calls until it stops yielding, at most `max_calls`.
/// Returns every call's outcome.
pub fn run_tick_job(
    img: &Image,
    feat: &[u32; FEAT_WORDS],
    now_ns: u64,
    max_calls: usize,
) -> (CartState, Vec<Outcome>) {
    let mut st = CartState::zeroed();
    begin_tick_job(img, &mut st, feat);
    let mut calls = Vec::new();
    for _ in 0..max_calls {
        let o = run_tick_call(img, &mut st, now_ns);
        let yielded = o.step == Some(Step::Yield);
        calls.push(o);
        if !yielded {
            break;
        }
    }
    (st, calls)
}

/// The XDP action a run maps to (ADR-106 D-C3): VERDICT as given, HALT =
/// PASS, any fault = PASS unless the image is fail_closed.
pub fn xdp_verdict(img: &Image, step: Step) -> u8 {
    debug_assert_eq!(img.stage, STAGE_XDP);
    match step {
        Step::Verdict(v) => v,
        Step::Fault(_) if img.flags & FLAG_FAIL_CLOSED != 0 => verdict::DROP,
        _ => verdict::PASS,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use monad_common::cart::{field, state_hash, sys, FaultKind, STAGE_TICK};
    use monad_common::{mbc_opcodes as op, MbcInsn};

    fn enc(o: u8, d: u8, s: u8, imm: u16) -> u32 {
        MbcInsn::encode(o, d, s, imm).0
    }

    /// Small deterministic PRNG (xorshift64*): no dependency, reproducible.
    pub(crate) struct Rng(pub u64);
    impl Rng {
        pub fn next(&mut self) -> u64 {
            let mut x = self.0;
            x ^= x >> 12;
            x ^= x << 25;
            x ^= x >> 27;
            self.0 = x;
            x.wrapping_mul(0x2545_F491_4F6C_DD1D)
        }
        pub fn below(&mut self, n: u64) -> u64 {
            self.next() % n
        }
    }

    /// A counter cartridge: mem[0] += pkt_len; mem[1] += 1; emit every 4th packet.
    fn counter() -> Image {
        Image {
            stage: STAGE_XDP,
            max_insn: 64,
            entry: 0,
            loop_bound: 0,
            flags: 0,
            rom: vec![
                enc(op::MOVI, 1, 0, field::PKT_LEN as u16),
                enc(op::SYSCALL, 0, 0, sys::FIELD),
                enc(op::LD, 2, 15, 0), // r15 = 0 at run start: mem[0]
                enc(op::ADD, 2, 0, 0),
                enc(op::ST, 15, 2, 0),
                enc(op::LD, 3, 15, 4),
                enc(op::ADDI, 3, 0, 1),
                enc(op::ST, 15, 3, 4),
                enc(op::MOV, 4, 3, 0),
                enc(op::MOVI, 5, 0, 3),
                enc(op::AND, 4, 5, 0),
                ((op::JNZ as u32) << 24) | 5, // skip the EMIT block
                enc(op::MOVI, 1, 0, 0x77),
                enc(op::MOV, 2, 3, 0),
                enc(op::LD, 3, 15, 0),
                enc(op::SYSCALL, 0, 0, sys::EMIT),
                enc(op::NOP, 0, 0, 0),
                enc(op::HALT, 0, 0, 0),
            ],
            data: vec![],
        }
    }

    #[test]
    fn counter_runs_and_emits() {
        let img = counter();
        crate::validate(&img).expect("valid");
        let mut st = CartState::zeroed();
        let mut emits = vec![];
        for n in 1..=8u32 {
            let mut inp = PacketInput::default();
            inp.fields[field::PKT_LEN as usize] = 100 + n;
            let o = run_packet(&img, &mut st, &inp);
            assert_eq!(o.step, Some(Step::Halt));
            emits.extend(o.emits);
        }
        assert_eq!(st.mem[1], 8);
        assert_eq!(st.mem[0], (101..=108).sum::<u32>());
        assert_eq!(
            emits,
            vec![
                (0x77, 4, 101 + 102 + 103 + 104),
                (0x77, 8, (101..=108).sum())
            ]
        );
    }

    /// Same image + same input log → same state hash after every packet,
    /// over 1,000 random logs (S5 Step 267).
    #[test]
    fn determinism_property() {
        let img = counter();
        let mut rng = Rng(0x5EED_CA27);
        for _ in 0..1000 {
            let n = 1 + rng.below(20) as usize;
            let log: Vec<PacketInput> = (0..n)
                .map(|_| {
                    let mut p = PacketInput {
                        now_ns: rng.next(),
                        ..PacketInput::default()
                    };
                    for f in p.fields.iter_mut() {
                        *f = rng.next() as u32;
                    }
                    p
                })
                .collect();
            let trace = |log: &[PacketInput]| {
                let mut st = CartState::zeroed();
                log.iter()
                    .map(|p| (run_packet(&img, &mut st, p), state_hash(&st)))
                    .collect::<Vec<_>>()
            };
            assert_eq!(trace(&log), trace(&log));
        }
    }

    /// The hash notices a single-bit change in mem (S5 Step 267 mutation test).
    #[test]
    fn hash_mutation() {
        let img = counter();
        let mut st = CartState::zeroed();
        let mut inp = PacketInput::default();
        inp.fields[field::PKT_LEN as usize] = 60;
        run_packet(&img, &mut st, &inp);
        let h = state_hash(&st);
        let mut rng = Rng(7);
        for _ in 0..256 {
            let mut m = st;
            let w = rng.below(64) as usize;
            m.mem[w] ^= 1 << rng.below(32);
            assert_ne!(state_hash(&m), h);
        }
    }

    #[test]
    fn tick_job_yields_and_resumes() {
        // r1 = mem[0] (feature 0); loop r1 times: r2 += 2; then EMIT(1, r2, 0)
        let img = Image {
            stage: STAGE_TICK,
            max_insn: 10,
            entry: 0,
            loop_bound: 100,
            flags: 0,
            rom: vec![
                enc(op::LD, 1, 15, 0),
                enc(op::ADDI, 2, 0, 2),
                enc(op::ADDI, 1, 0, 0xFFFF),
                ((op::JNZ as u32) << 24) | 0x00FF_FFFD, // -3 → pc 1
                enc(op::MOVI, 3, 0, 0),
                enc(op::MOV, 4, 2, 0),
                enc(op::MOVI, 1, 0, 1),
                enc(op::MOV, 2, 4, 0),
                enc(op::SYSCALL, 0, 0, sys::EMIT),
                enc(op::HALT, 0, 0, 0),
            ],
            data: vec![],
        };
        crate::validate(&img).expect("valid");
        let (st, calls) = run_tick_job(&img, &[20, 0, 0, 0, 0, 0, 0, 0], 0, 100);
        assert!(calls.len() > 1, "a 10-insn budget must yield");
        assert_eq!(calls.last().unwrap().step, Some(Step::Halt));
        let emits: Vec<_> = calls.iter().flat_map(|c| c.emits.clone()).collect();
        assert_eq!(emits, vec![(1, 40, 0)]);
        assert_eq!(st.back_edges, 19);
        // Same job with loop_bound 5 faults instead of looping on.
        let mut tight = img.clone();
        tight.loop_bound = 5;
        let (_, calls) = run_tick_job(&tight, &[20, 0, 0, 0, 0, 0, 0, 0], 0, 100);
        assert_eq!(
            calls.last().unwrap().step,
            Some(Step::Fault(FaultKind::LoopBound))
        );
    }

    #[test]
    fn fault_verdict_policy() {
        let mut img = counter();
        assert_eq!(
            xdp_verdict(&img, Step::Fault(FaultKind::Mem)),
            verdict::PASS
        );
        assert_eq!(xdp_verdict(&img, Step::Halt), verdict::PASS);
        assert_eq!(
            xdp_verdict(&img, Step::Verdict(verdict::DROP)),
            verdict::DROP
        );
        img.flags = FLAG_FAIL_CLOSED;
        assert_eq!(
            xdp_verdict(&img, Step::Fault(FaultKind::Mem)),
            verdict::DROP
        );
    }
}
