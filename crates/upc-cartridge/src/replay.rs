// SPDX-License-Identifier: GPL-3.0-or-later
//! Anamnesis replay: re-run a recorded cartridge run off-line, bit for bit,
//! and step through it backwards and forwards (S5 Phase 16, ADR-108).
//!
//! A traced run (`cart_xdp` or one `cart_tick` call) is recorded by
//! `cartridge-ebpf` as a [`TraceHead`] (the starting state and every input the
//! run can read: the FIELD table and the clock), a [`TraceStep`] per executed
//! instruction (pc, instruction word, step code, state hash after it) and a
//! [`TraceEnd`]. Determinism contract: `state_{n+1} = step(state_n, input)`,
//! so the image plus the head reproduce every step; replay re-executes with
//! the same `monad_common::cart::run_one` the BPF program runs and checks the
//! recorded hash after every instruction. The first mismatch is the
//! divergence: a different image, a corrupted trace, or an engine
//! difference.
//!
//! Trace file (`upc-cartridgectl record`, little-endian):
//! `"UCTR" | u32 version (1) | 4 x [32] sha256 of the .cart file per slot
//! (zero = unknown) | { u32 len | record }*`. Records carry header fields,
//! cartridge state and hashes; no payload byte can be in one (ADR-108 ABI).

use monad_common::cart::{
    code_step, run_one, run_out, state_hash, step_code, trace, CartIo, CartState, FlowKey, Step,
    TraceEnd, TraceHead, TraceStep, FEAT_WORDS, FIELD_COUNT, MEM_WORDS, N_PKT, N_TICK, STAGE_XDP,
};

use crate::Image;

/// File magic.
pub const MAGIC: &[u8; 4] = b"UCTR";
/// File format version.
pub const VERSION: u32 = 1;
/// Largest trace file accepted (64 MiB).
pub const MAX_TRACE: usize = 64 << 20;
const HEAD_LEN: usize = 464;
const STEP_LEN: usize = 32;

/// Why bytes are not a trace.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum TraceError {
    /// Shorter than its header / a record runs past the end.
    Truncated,
    /// Not "UCTR" or another version.
    BadHeader,
    /// Larger than [`MAX_TRACE`].
    TooLarge,
    /// A record that is not a cartridge EVENT_COMPUTED head/step/end.
    BadRecord(usize),
}

impl core::fmt::Display for TraceError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            TraceError::Truncated => write!(f, "trace truncated"),
            TraceError::BadHeader => write!(f, "not a UCTR v1 trace"),
            TraceError::TooLarge => write!(f, "trace larger than {MAX_TRACE} bytes"),
            TraceError::BadRecord(i) => write!(f, "record {i} is not a cartridge trace record"),
        }
    }
}

impl std::error::Error for TraceError {}

fn le16(b: &[u8], o: usize) -> u16 {
    u16::from_le_bytes([b[o], b[o + 1]])
}
fn le32(b: &[u8], o: usize) -> u32 {
    u32::from_le_bytes([b[o], b[o + 1], b[o + 2], b[o + 3]])
}
fn le64(b: &[u8], o: usize) -> u64 {
    u64::from_le_bytes(b[o..o + 8].try_into().expect("8 bytes"))
}

/// Decode a 336-byte CartState (ADR-108 layout).
pub fn decode_state(b: &[u8]) -> CartState {
    let mut st = CartState::zeroed();
    for (i, r) in st.regs.iter_mut().enumerate() {
        *r = le32(b, 4 * i);
    }
    for (i, m) in st.mem.iter_mut().enumerate() {
        *m = le32(b, 64 + 4 * i);
    }
    st.pc = le16(b, 320);
    st.flags = b[322];
    st.status = b[323];
    st.insns = le32(b, 324);
    st.back_edges = le32(b, 328);
    st.gen = le32(b, 332);
    st
}

/// Encode a CartState (inverse of [`decode_state`]).
pub fn encode_state(st: &CartState, out: &mut Vec<u8>) {
    for r in st.regs {
        out.extend_from_slice(&r.to_le_bytes());
    }
    for m in st.mem {
        out.extend_from_slice(&m.to_le_bytes());
    }
    out.extend_from_slice(&st.pc.to_le_bytes());
    out.push(st.flags);
    out.push(st.status);
    for w in [st.insns, st.back_edges, st.gen] {
        out.extend_from_slice(&w.to_le_bytes());
    }
}

fn decode_flow(b: &[u8]) -> FlowKey {
    let mut k = FlowKey::default();
    k.src.copy_from_slice(&b[0..16]);
    k.dst.copy_from_slice(&b[16..32]);
    k.sport = le16(b, 32);
    k.dport = le16(b, 34);
    k.proto = b[36];
    k.l3 = b[37];
    k.slot = b[38];
    k
}

fn encode_flow(k: &FlowKey, out: &mut Vec<u8>) {
    out.extend_from_slice(&k.src);
    out.extend_from_slice(&k.dst);
    out.extend_from_slice(&k.sport.to_le_bytes());
    out.extend_from_slice(&k.dport.to_le_bytes());
    out.extend_from_slice(&[k.proto, k.l3, k.slot, 0]);
}

/// One trace record.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Record {
    /// Run start.
    Head(Box<TraceHead>),
    /// One instruction.
    Step(TraceStep),
    /// Run end.
    End(TraceEnd),
}

/// Decode one record as `cartridge-ebpf` wrote it.
pub fn decode_record(b: &[u8]) -> Option<Record> {
    if b.len() < 4 || b[0] != trace::EVENT_COMPUTED || b[1] != trace::SUBKIND {
        return None;
    }
    match (b[2], b.len()) {
        (trace::REC_HEAD, HEAD_LEN) => {
            let mut fields = [0u32; FIELD_COUNT as usize];
            for (i, f) in fields.iter_mut().enumerate() {
                *f = le32(b, 72 + 4 * i);
            }
            Some(Record::Head(Box::new(TraceHead {
                event: b[0],
                subkind: b[1],
                rec: b[2],
                stage: b[3],
                slot: b[4],
                rsv: [0; 3],
                gen: le32(b, 8),
                rsv2: 0,
                run_id: le64(b, 16),
                now: le64(b, 24),
                flow: decode_flow(&b[32..72]),
                fields,
                pad: 0,
                state: decode_state(&b[128..464]),
            })))
        }
        (trace::REC_STEP, STEP_LEN) => Some(Record::Step(TraceStep {
            event: b[0],
            subkind: b[1],
            rec: b[2],
            rsv: 0,
            idx: le32(b, 4),
            run_id: le64(b, 8),
            pc: le32(b, 16),
            insn: le32(b, 20),
            code: le32(b, 24),
            hash: le32(b, 28),
        })),
        (trace::REC_END, STEP_LEN) => Some(Record::End(TraceEnd {
            event: b[0],
            subkind: b[1],
            rec: b[2],
            rsv: 0,
            insns: le32(b, 4),
            run_id: le64(b, 8),
            code: le32(b, 16),
            hash: le32(b, 20),
            rsv2: 0,
        })),
        _ => None,
    }
}

/// Encode a record (tests and tools; the BPF program writes the same bytes).
pub fn encode_record(r: &Record) -> Vec<u8> {
    let mut o = Vec::with_capacity(HEAD_LEN);
    match r {
        Record::Head(h) => {
            o.extend_from_slice(&[trace::EVENT_COMPUTED, trace::SUBKIND, trace::REC_HEAD]);
            o.extend_from_slice(&[h.stage, h.slot, 0, 0, 0]);
            o.extend_from_slice(&h.gen.to_le_bytes());
            o.extend_from_slice(&0u32.to_le_bytes());
            o.extend_from_slice(&h.run_id.to_le_bytes());
            o.extend_from_slice(&h.now.to_le_bytes());
            encode_flow(&h.flow, &mut o);
            for f in h.fields {
                o.extend_from_slice(&f.to_le_bytes());
            }
            o.extend_from_slice(&0u32.to_le_bytes());
            encode_state(&h.state, &mut o);
        }
        Record::Step(s) => {
            o.extend_from_slice(&[trace::EVENT_COMPUTED, trace::SUBKIND, trace::REC_STEP, 0]);
            o.extend_from_slice(&s.idx.to_le_bytes());
            o.extend_from_slice(&s.run_id.to_le_bytes());
            for w in [s.pc, s.insn, s.code, s.hash] {
                o.extend_from_slice(&w.to_le_bytes());
            }
        }
        Record::End(e) => {
            o.extend_from_slice(&[trace::EVENT_COMPUTED, trace::SUBKIND, trace::REC_END, 0]);
            o.extend_from_slice(&e.insns.to_le_bytes());
            o.extend_from_slice(&e.run_id.to_le_bytes());
            for w in [e.code, e.hash] {
                o.extend_from_slice(&w.to_le_bytes());
            }
            o.extend_from_slice(&0u64.to_le_bytes());
        }
    }
    o
}

/// A trace file: per-slot image sha and the recorded runs.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Trace {
    /// sha256 of the `.cart` file loaded in each slot (zero = unknown).
    pub slot_sha: [[u8; 32]; 4],
    /// Runs in the order their heads were recorded.
    pub runs: Vec<Run>,
    /// Step/end records whose head is missing (ring overflow), skipped.
    pub orphans: usize,
}

/// One recorded run.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Run {
    /// Its head.
    pub head: TraceHead,
    /// Its steps, in record order.
    pub steps: Vec<TraceStep>,
    /// Its end record, if recorded.
    pub end: Option<TraceEnd>,
}

impl Trace {
    /// Parse hostile bytes.
    pub fn parse(b: &[u8]) -> Result<Trace, TraceError> {
        if b.len() > MAX_TRACE {
            return Err(TraceError::TooLarge);
        }
        if b.len() < 8 + 4 * 32 {
            return Err(TraceError::Truncated);
        }
        if &b[0..4] != MAGIC || le32(b, 4) != VERSION {
            return Err(TraceError::BadHeader);
        }
        let mut t = Trace::default();
        for s in 0..4 {
            t.slot_sha[s].copy_from_slice(&b[8 + 32 * s..8 + 32 * (s + 1)]);
        }
        let mut off = 8 + 4 * 32;
        let mut n = 0;
        let mut index: std::collections::HashMap<u64, usize> = Default::default();
        while off < b.len() {
            if off + 4 > b.len() {
                return Err(TraceError::Truncated);
            }
            let len = le32(b, off) as usize;
            off += 4;
            if len > HEAD_LEN || off + len > b.len() {
                return Err(TraceError::Truncated);
            }
            let rec = decode_record(&b[off..off + len]).ok_or(TraceError::BadRecord(n))?;
            off += len;
            n += 1;
            match rec {
                Record::Head(h) => {
                    index.insert(h.run_id, t.runs.len());
                    t.runs.push(Run {
                        head: *h,
                        steps: vec![],
                        end: None,
                    });
                }
                Record::Step(s) => match index.get(&s.run_id) {
                    Some(&i) => t.runs[i].steps.push(s),
                    None => t.orphans += 1,
                },
                Record::End(e) => match index.get(&e.run_id) {
                    Some(&i) => t.runs[i].end = Some(e),
                    None => t.orphans += 1,
                },
            }
        }
        Ok(t)
    }

    /// Serialize (inverse of [`Trace::parse`]; records in run order).
    pub fn encode(&self) -> Vec<u8> {
        let mut o = Vec::new();
        o.extend_from_slice(MAGIC);
        o.extend_from_slice(&VERSION.to_le_bytes());
        for s in &self.slot_sha {
            o.extend_from_slice(s);
        }
        let mut put = |r: Record| {
            let b = encode_record(&r);
            o.extend_from_slice(&(b.len() as u32).to_le_bytes());
            o.extend_from_slice(&b);
        };
        for run in &self.runs {
            put(Record::Head(Box::new(run.head)));
            for s in &run.steps {
                put(Record::Step(*s));
            }
            if let Some(e) = run.end {
                put(Record::End(e));
            }
        }
        o
    }
}

/// Where replay first disagreed with the recording.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Divergence {
    /// Instruction index (`steps.len()` when only the end record differs).
    pub idx: usize,
    /// What differed ("hash", "pc", "insn", "code", "steps", "end").
    pub what: &'static str,
    /// Recorded value.
    pub recorded: u32,
    /// Replayed value.
    pub replayed: u32,
}

/// One re-executed instruction.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct ReplayedStep {
    /// PC it was fetched from.
    pub pc: u32,
    /// Instruction word.
    pub insn: u32,
    /// Step code.
    pub code: u32,
    /// State hash after it.
    pub hash: u32,
}

struct HeadIo<'a> {
    head: &'a TraceHead,
    emits: Vec<(u32, u32, u32)>,
    queued: Vec<[u32; FEAT_WORDS]>,
}

impl CartIo for HeadIo<'_> {
    fn field(&mut self, id: u32) -> Option<u32> {
        if self.head.stage == STAGE_XDP {
            self.head.fields.get(id as usize).copied()
        } else {
            None
        }
    }
    fn now_ns_lo(&mut self) -> u32 {
        self.head.now as u32
    }
    fn emit(&mut self, kind: u32, a: u32, b: u32) {
        self.emits.push((kind, a, b));
    }
    fn queue(&mut self, feat: &[u32; FEAT_WORDS]) {
        self.queued.push(*feat);
    }
}

/// A replayed run with time-travel over its states.
#[derive(Clone, Debug)]
pub struct Replayer {
    /// The recording.
    pub run: Run,
    /// `states[0]` = start; `states[i + 1]` = after instruction i.
    pub states: Vec<CartState>,
    /// Re-executed instructions.
    pub steps: Vec<ReplayedStep>,
    /// How the replayed run ended.
    pub outcome: Step,
    /// EMIT records of the replayed run.
    pub emits: Vec<(u32, u32, u32)>,
    /// QUEUE snapshots of the replayed run.
    pub queued: Vec<[u32; FEAT_WORDS]>,
    /// First disagreement with the recording, if any.
    pub divergence: Option<Divergence>,
    cursor: usize,
}

/// Re-execute `run` on `img` exactly as `cartridge-ebpf`'s `drive` does
/// (`run_one` up to the stage cap, then `run_out`), checking every recorded
/// step and the end record.
pub fn replay(img: &Image, run: &Run) -> Replayer {
    let cfg = img.cfg();
    let n = if run.head.stage == STAGE_XDP {
        N_PKT
    } else {
        N_TICK
    };
    let mut st = run.head.state;
    st.insns = 0;
    let mut io = HeadIo {
        head: &run.head,
        emits: vec![],
        queued: vec![],
    };
    let mut states = vec![st];
    let mut steps = vec![];
    let mut div: Option<Divergence> = None;
    let mut outcome = None;
    if img.stage != run.head.stage {
        div = Some(Divergence {
            idx: 0,
            what: "stage",
            recorded: run.head.stage as u32,
            replayed: img.stage as u32,
        });
    }
    for _ in 0..n {
        let before = st.insns;
        let pc = st.pc as u32;
        let r = run_one(&mut st, &cfg, &img.rom[..], &img.data[..], &mut io);
        if st.insns != before {
            let s = ReplayedStep {
                pc,
                insn: img.rom.get(pc as usize).copied().unwrap_or(0),
                code: step_code(r),
                hash: state_hash(&st),
            };
            let i = steps.len();
            if div.is_none() {
                div = match run.steps.get(i) {
                    None => Some(Divergence {
                        idx: i,
                        what: "steps",
                        recorded: run.steps.len() as u32,
                        replayed: i as u32 + 1,
                    }),
                    Some(rec) => [
                        ("pc", rec.pc, s.pc),
                        ("insn", rec.insn, s.insn),
                        ("hash", rec.hash, s.hash),
                        ("code", rec.code, s.code),
                    ]
                    .into_iter()
                    .find(|(_, a, b)| a != b)
                    .map(|(what, recorded, replayed)| Divergence {
                        idx: i,
                        what,
                        recorded,
                        replayed,
                    }),
                };
            }
            steps.push(s);
            states.push(st);
        }
        if r != Step::Continue {
            outcome = Some(r);
            break;
        }
    }
    let outcome = outcome.unwrap_or_else(|| run_out(&mut st, &cfg));
    if div.is_none() && run.steps.len() != steps.len() {
        div = Some(Divergence {
            idx: steps.len(),
            what: "steps",
            recorded: run.steps.len() as u32,
            replayed: steps.len() as u32,
        });
    }
    if div.is_none() {
        if let Some(e) = run.end {
            for (what, recorded, replayed) in [
                ("end", e.code, step_code(outcome)),
                ("end", e.hash, state_hash(&st)),
                ("end", e.insns, st.insns),
            ] {
                if recorded != replayed {
                    div = Some(Divergence {
                        idx: steps.len(),
                        what,
                        recorded,
                        replayed,
                    });
                    break;
                }
            }
        }
    }
    Replayer {
        run: run.clone(),
        states,
        steps,
        outcome,
        emits: io.emits,
        queued: io.queued,
        divergence: div,
        cursor: 0,
    }
}

impl Replayer {
    /// True when every step and the end record matched.
    pub fn bit_exact(&self) -> bool {
        self.divergence.is_none()
    }
    /// Current position: 0 = before the first instruction.
    pub fn cursor(&self) -> usize {
        self.cursor
    }
    /// Instructions replayed.
    pub fn len(&self) -> usize {
        self.steps.len()
    }
    /// No instruction ran.
    pub fn is_empty(&self) -> bool {
        self.steps.is_empty()
    }
    /// Forward one instruction (false at the end).
    pub fn step(&mut self) -> bool {
        if self.cursor < self.steps.len() {
            self.cursor += 1;
            true
        } else {
            false
        }
    }
    /// Back one instruction (false at the start).
    pub fn back(&mut self) -> bool {
        if self.cursor > 0 {
            self.cursor -= 1;
            true
        } else {
            false
        }
    }
    /// Jump to position n (clamped to the run).
    pub fn goto(&mut self, n: usize) {
        self.cursor = n.min(self.steps.len());
    }
    /// State at the cursor.
    pub fn state(&self) -> &CartState {
        &self.states[self.cursor]
    }
    /// Registers at the cursor.
    pub fn regs(&self) -> &[u32; 16] {
        &self.state().regs
    }
    /// Flow window at the cursor.
    pub fn mem(&self) -> &[u32; MEM_WORDS] {
        &self.state().mem
    }
    /// Human-readable state at the cursor: the next instruction, flags,
    /// non-zero registers and window words.
    pub fn inspect(&self) -> String {
        let st = self.state();
        let next = match self.steps.get(self.cursor) {
            Some(s) => format!(
                "next {:>4}: pc {:>3} {}",
                self.cursor,
                s.pc,
                disasm(s.insn, s.pc)
            ),
            None => format!("end: {:?}", self.outcome),
        };
        let regs: Vec<String> = st
            .regs
            .iter()
            .enumerate()
            .filter(|(_, v)| **v != 0)
            .map(|(i, v)| format!("r{i}={v:#x}"))
            .collect();
        let mem: Vec<String> = st
            .mem
            .iter()
            .enumerate()
            .filter(|(_, v)| **v != 0)
            .map(|(i, v)| format!("mem[{:#04x}]={v:#x}", i * 4))
            .collect();
        format!(
            "@{} {next}\n  flags {:#x} hash {:08x}\n  {}\n  {}",
            self.cursor,
            st.flags,
            state_hash(st),
            if regs.is_empty() {
                "regs all 0".into()
            } else {
                regs.join(" ")
            },
            if mem.is_empty() {
                "mem all 0".into()
            } else {
                mem.join(" ")
            }
        )
    }
}

/// Host recorder: run `head` on `img` and produce the records the BPF
/// program writes for it (tests, and a reference for what a trace holds).
pub fn record(img: &Image, head: &TraceHead) -> Run {
    let probe = Run {
        head: *head,
        steps: vec![],
        end: None,
    };
    let r = replay(img, &probe);
    let steps = r
        .steps
        .iter()
        .enumerate()
        .map(|(i, s)| TraceStep {
            event: trace::EVENT_COMPUTED,
            subkind: trace::SUBKIND,
            rec: trace::REC_STEP,
            rsv: 0,
            idx: i as u32,
            run_id: head.run_id,
            pc: s.pc,
            insn: s.insn,
            code: s.code,
            hash: s.hash,
        })
        .collect::<Vec<_>>();
    // state_hash covers regs, pc, flags and mem; run_out only sets status,
    // so the last state's hash is the end hash.
    let fin = r.states.last().copied().unwrap_or(head.state);
    Run {
        head: *head,
        end: Some(TraceEnd {
            event: trace::EVENT_COMPUTED,
            subkind: trace::SUBKIND,
            rec: trace::REC_END,
            rsv: 0,
            insns: steps.len() as u32,
            run_id: head.run_id,
            code: step_code(r.outcome),
            hash: state_hash(&fin),
            rsv2: 0,
        }),
        steps,
    }
}

/// A head for a run starting from `state` with `fields` and clock `now`.
#[allow(clippy::too_many_arguments)] // one per record field
pub fn head(
    stage: u8,
    slot: u8,
    gen: u32,
    run_id: u64,
    now: u64,
    flow: FlowKey,
    fields: [u32; FIELD_COUNT as usize],
    state: CartState,
) -> TraceHead {
    TraceHead {
        event: trace::EVENT_COMPUTED,
        subkind: trace::SUBKIND,
        rec: trace::REC_HEAD,
        stage,
        slot,
        rsv: [0; 3],
        gen,
        rsv2: 0,
        run_id,
        now,
        flow,
        fields,
        pad: 0,
        state,
    }
}

/// One instruction as text; SYSCALL uses the cartridge ABI names (the MBC
/// disassembler knows Doom's).
pub fn disasm(insn: u32, pc: u32) -> String {
    if insn >> 24 == 0x40 {
        let n = insn & 0xFFFF;
        let name = ["FIELD", "NOW_NS_LO", "VERDICT", "EMIT", "QUEUE", "YIELD"]
            .get(n as usize)
            .copied()
            .unwrap_or("?");
        return format!("SYSCALL {n} ({name})");
    }
    monad_mbc::disasm::disasm_insn(insn, pc)
}

/// The recorded outcome of a run, if its end record is present.
pub fn recorded_outcome(run: &Run) -> Option<Step> {
    run.end.map(|e| code_step(e.code))
}
