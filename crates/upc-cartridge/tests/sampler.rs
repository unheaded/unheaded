// SPDX-License-Identifier: GPL-3.0-or-later
//! S5 Phase 14 (Steps 353-354): `cartridges/sampler.s` on the reference
//! runner against an independent model of its contract: exactly every Nth
//! packet of a flow emits a sample and queues the feature snapshot; the
//! first packet emits first-seen; the verdict is always PASS.

use monad_common::cart::{field, verdict, CartState, Step, FEAT_WORDS, FIELD_COUNT, STAGE_XDP};
use upc_cartridge::reference::{run_packet, xdp_verdict, FlowMetaSim, Outcome};
use upc_cartridge::{build_image, data_words, state_hash, validate, Image};

fn repo(p: &str) -> String {
    format!("{}/../../{p}", env!("CARGO_MANIFEST_DIR"))
}

fn image(n: u32) -> Image {
    let src = std::fs::read_to_string(repo("cartridges/sampler.s")).expect("sampler.s");
    build_image(STAGE_XDP, &src, vec![n], None, 0).expect("builds + validates")
}

struct Rng(u64);
impl Rng {
    fn next(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545_F491_4F6C_DD1D)
    }
    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

/// Independent model of the sampler contract for one flow.
#[derive(Default)]
struct Model {
    seen: bool,
    cnt: u32,
    win: [u32; 4], // bytes, iat, syn, max
    flags: u32,
}

#[derive(Debug, PartialEq, Eq)]
struct Expect {
    emits: Vec<(u32, u32, u32)>,
    queued: Vec<[u32; FEAT_WORDS]>,
}

impl Model {
    fn packet(&mut self, n: u32, f: &[u32; FIELD_COUNT as usize]) -> Expect {
        let mut e = Expect {
            emits: vec![],
            queued: vec![],
        };
        let len = f[field::PKT_LEN as usize];
        if !self.seen {
            self.seen = true;
            e.emits.push((0x46, len, 0));
        }
        self.win[0] = self.win[0].wrapping_add(len);
        self.win[3] = self.win[3].max(len);
        self.win[1] += f[field::IAT_BUCKET as usize];
        let tf = f[field::TCP_FLAGS as usize];
        self.flags |= tf;
        self.win[2] += (tf >> 1) & 1;
        self.cnt += 1;
        if self.cnt >= n {
            let feat = [
                self.win[0],
                self.win[1],
                self.win[2],
                self.win[3],
                f[field::FLOW_PKT_COUNT as usize],
                f[field::L4_PROTO as usize],
                f[field::DST_PORT as usize],
                self.flags,
            ];
            e.emits.push((
                0x53,
                f[field::FLOW_PKT_COUNT as usize],
                f[field::FLOW_BYTE_COUNT as usize],
            ));
            e.queued.push(feat);
            self.win = [0; 4];
            self.flags = 0;
            self.cnt = 0;
        }
        e
    }
}

/// A packet's FIELD 0-6 for a TCP or UDP flow.
fn base(rng: &mut Rng, tcp: bool, dport: u32) -> [u32; FIELD_COUNT as usize] {
    let mut f = [0u32; FIELD_COUNT as usize];
    f[field::L3_PROTO as usize] = 4;
    f[field::L4_PROTO as usize] = if tcp { 6 } else { 17 };
    f[field::SRC_PORT as usize] = 40_000;
    f[field::DST_PORT as usize] = dport;
    f[field::PKT_LEN as usize] = 60 + rng.below(1441) as u32;
    f[field::IP_TTL as usize] = 64;
    if tcp {
        f[field::TCP_FLAGS as usize] = [0x02, 0x10, 0x18, 0x11, 0x12, 0x04][rng.below(6) as usize];
    }
    f
}

/// Run `n_pkts` packets over three flows; per flow (outcomes, model), plus
/// the per-packet state hashes (for the determinism check).
fn run_trace(img: &Image, n: u32, seed: u64, n_pkts: usize) -> (Vec<(Outcome, Expect)>, Vec<u32>) {
    let mut rng = Rng(seed);
    let mut st = [CartState::zeroed(); 3];
    let mut meta = [FlowMetaSim::default(); 3];
    let mut model: [Model; 3] = Default::default();
    let mut now = 2_000_000_000u64;
    let mut out = vec![];
    let mut hashes = vec![];
    for _ in 0..n_pkts {
        let i = rng.below(3) as usize;
        now += rng.below(3_000_000);
        let b = base(&mut rng, i != 2, [443, 22, 53][i]);
        let inp = meta[i].packet(now, b);
        let o = run_packet(img, &mut st[i], &inp);
        assert_eq!(
            o.step,
            Some(Step::Verdict(verdict::PASS)),
            "sampler must PASS"
        );
        assert_eq!(xdp_verdict(img, o.step.unwrap()), verdict::PASS);
        let e = model[i].packet(n, &inp.fields);
        hashes.push(state_hash(&st[i]));
        out.push((o, e));
    }
    (out, hashes)
}

#[test]
fn sampler_validates_xdp_metadata_only() {
    let img = image(8);
    let r = validate(&img).unwrap();
    assert!(img.max_insn <= 64 && !r.has_back_edges);
    // FIELD, VERDICT, EMIT, QUEUE only: no clock needed, nothing else.
    assert_eq!(r.syscalls, vec![0, 2, 3, 4]);
    let shipped = data_words(&std::fs::read(repo("cartridges/sampler.data.bin")).unwrap()).unwrap();
    assert_eq!(shipped, vec![8]);
}

/// Exactly every Nth packet of each flow emits and queues the right
/// snapshot, for several N, including N = 1 (the longest path: first-seen +
/// sample in one run, still inside the 64-instruction budget).
#[test]
fn sampler_is_exactly_one_in_n() {
    for (n, seed) in [(1u32, 1u64), (2, 2), (5, 3), (8, 4), (100, 5)] {
        let img = image(n);
        let (runs, _) = run_trace(&img, n, seed, 1_000);
        let mut samples = 0;
        for (k, (o, e)) in runs.iter().enumerate() {
            assert_eq!(o.emits, e.emits, "N={n} packet {k}: emits");
            assert_eq!(o.queued, e.queued, "N={n} packet {k}: snapshot");
            samples += o.queued.len();
        }
        assert!(samples > 0 || n == 100);
        eprintln!("SAMPLER N={n}: {samples} samples over 1000 packets, all match the model");
    }
}

/// The longest path (a flow's first packet completing a window, N = 1) is
/// 62 instructions: two under the XDP-stage cap of 64.
#[test]
fn sampler_longest_path_fits() {
    let img = image(1);
    let mut st = CartState::zeroed();
    let mut rng = Rng(7);
    let inp = FlowMetaSim::default().packet(1, base(&mut rng, true, 80));
    let o = run_packet(&img, &mut st, &inp);
    assert_eq!(o.step, Some(Step::Verdict(verdict::PASS)));
    assert_eq!((o.emits.len(), o.queued.len()), (2, 1));
    assert_eq!(st.insns, 62);
}

/// Same trace twice: identical emit sets, queue jobs and state hashes.
#[test]
fn sampler_is_deterministic() {
    let img = image(8);
    let (a, ha) = run_trace(&img, 8, 99, 2_000);
    let (b, hb) = run_trace(&img, 8, 99, 2_000);
    assert_eq!(ha, hb);
    for ((oa, _), (ob, _)) in a.iter().zip(b.iter()) {
        assert_eq!(oa, ob);
    }
}
