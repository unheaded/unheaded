// SPDX-License-Identifier: GPL-3.0-or-later
//! The int8 anomaly scorer (S5 Phase 15, ADR-108): synthetic training data,
//! a float MLP, its int8 quantization, and the integer reference that
//! `cartridges/scorer.s` must match bit for bit.
//!
//! **Data**: every vector is a sampler feature snapshot (`cartridges/sampler.s`,
//! the real cartridge run on the reference runner) of a synthetic flow drawn
//! from ten generators (five benign, five anomalous). No traffic capture, no
//! payload, nothing personal: packets are (gap, length, TCP flags) triples
//! drawn from fixed distributions. Train and eval sets come from disjoint
//! seeds, so eval vectors are flows the model never saw; both come from the
//! same generators, so eval accuracy says how well the model separates these
//! synthetic families and nothing about real traffic.
//!
//! **Model** (D-C6): x_k = min(f_k >> sh_k, 255) for the 8 features, hidden
//! 16 ReLU, one logit, score = round(100 * sigmoid(logit)).
//!
//! **Quantization**: per-row int8 first-layer weights (stored biased +128 as
//! u8 so the cartridge reads them with one LDB), per-row requantization shift
//! to u8 activations, int8 second-layer weights on one scale, a 256-entry
//! sigmoid table. Rounding offsets are folded into the biases (exact, see
//! `quantize`).

use monad_common::cart::{field, CartState, FEAT_WORDS, FIELD_COUNT};

use crate::reference::{run_packet, FlowMetaSim};
use crate::Image;

/// Hidden units (the scorer's declared loop bound).
pub const HIDDEN: usize = 16;
/// Input shifts: x_k = min(f_k >> SH_IN[k], 255) (see `FEATURES`).
pub const SH_IN: [u8; FEAT_WORDS] = [6, 0, 0, 3, 3, 0, 8, 0];
/// Feature names, sampler order (cartridges/sampler.s).
pub const FEATURES: [&str; FEAT_WORDS] = [
    "win_bytes",
    "win_iat",
    "win_syn",
    "win_max_len",
    "flow_pkts",
    "l4_proto",
    "dst_port",
    "win_flags",
];
/// Score event kind ('A').
pub const SCORE_KIND: u32 = 0x41;
/// Sampler window the model was trained for (sampler DATA word 0).
pub const SAMPLER_N: u32 = 8;

/// DATA layout (byte offsets from the start of DATA, i.e. from 0x1000).
pub mod layout {
    /// u8 input shifts, 8 bytes.
    pub const SH_IN: usize = 0x00;
    /// u32 model id.
    pub const MODEL_ID: usize = 0x08;
    /// u8 output shift.
    pub const SO: usize = 0x0C;
    /// i32 output bias (rounding folded).
    pub const B2: usize = 0x10;
    /// 16 rows of 16 bytes: i32 bias, 8 x u8 biased weights, i8 w2, u8 shift, 2 pad.
    pub const ROWS: usize = 0x14;
    /// Row size.
    pub const ROW: usize = 16;
    /// 256 x u8 sigmoid table.
    pub const LUT: usize = ROWS + super::HIDDEN * ROW;
    /// Total bytes.
    pub const LEN: usize = LUT + 256;
}

/// Small deterministic PRNG (xorshift64*), no dependency.
#[derive(Clone, Debug)]
pub struct Rng(pub u64);

impl Rng {
    /// Next 64 random bits.
    #[allow(clippy::should_implement_trait)] // a PRNG step, not an iterator
    pub fn next(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545_F491_4F6C_DD1D)
    }
    /// Uniform in [0, n).
    pub fn below(&mut self, n: u64) -> u64 {
        self.next() % n.max(1)
    }
    /// Uniform in [0, 1).
    pub fn unit(&mut self) -> f64 {
        (self.next() >> 11) as f64 / (1u64 << 53) as f64
    }
    /// Uniform integer in [lo, hi].
    pub fn range(&mut self, lo: u64, hi: u64) -> u64 {
        lo + self.below(hi - lo + 1)
    }
    /// Log-uniform in [lo, hi] (lo > 0).
    pub fn log_range(&mut self, lo: f64, hi: f64) -> f64 {
        (lo.ln() + self.unit() * (hi.ln() - lo.ln())).exp()
    }
}

/// The ten synthetic flow families.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Family {
    /// TLS client to :443 (SYN, then ACK / PSH-ACK requests).
    WebClient,
    /// Bulk download toward an ephemeral port (ACK, full-size segments).
    WebDownload,
    /// UDP DNS queries to :53.
    Dns,
    /// Interactive SSH to :22.
    Ssh,
    /// QUIC (UDP :443).
    Quic,
    /// SYN-only flood.
    SynFlood,
    /// UDP flood to a high port.
    UdpFlood,
    /// NULL / FIN / XMAS scan probes.
    StealthScan,
    /// RST storm.
    RstStorm,
    /// Clockwork beacon: small PSH-ACK to an odd port at a fixed period.
    Beacon,
}

/// Every family, benign first.
pub const FAMILIES: [Family; 10] = [
    Family::WebClient,
    Family::WebDownload,
    Family::Dns,
    Family::Ssh,
    Family::Quic,
    Family::SynFlood,
    Family::UdpFlood,
    Family::StealthScan,
    Family::RstStorm,
    Family::Beacon,
];

impl Family {
    /// 1 for the anomalous families.
    pub fn label(self) -> u8 {
        matches!(
            self,
            Family::SynFlood
                | Family::UdpFlood
                | Family::StealthScan
                | Family::RstStorm
                | Family::Beacon
        ) as u8
    }
    /// Stable name (CSV column).
    pub fn name(self) -> &'static str {
        match self {
            Family::WebClient => "web_client",
            Family::WebDownload => "web_download",
            Family::Dns => "dns",
            Family::Ssh => "ssh",
            Family::Quic => "quic",
            Family::SynFlood => "syn_flood",
            Family::UdpFlood => "udp_flood",
            Family::StealthScan => "stealth_scan",
            Family::RstStorm => "rst_storm",
            Family::Beacon => "beacon",
        }
    }
    /// Parse a name.
    pub fn from_name(s: &str) -> Option<Family> {
        FAMILIES.iter().copied().find(|f| f.name() == s)
    }
}

/// One synthetic packet: gap since the flow's previous packet, frame length,
/// TCP flags.
#[derive(Clone, Copy, Debug)]
pub struct SynthPkt {
    /// ns since the previous packet of the flow.
    pub gap_ns: u64,
    /// Frame length.
    pub len: u32,
    /// TCP flags (0 for UDP).
    pub flags: u8,
}

/// A synthetic flow: protocol, destination port, packets.
#[derive(Clone, Debug)]
pub struct SynthFlow {
    /// Family it was drawn from.
    pub family: Family,
    /// 6 or 17.
    pub proto: u8,
    /// Destination port.
    pub dport: u16,
    /// Packets in order.
    pub pkts: Vec<SynthPkt>,
}

fn us(x: f64) -> u64 {
    (x * 1000.0) as u64
}

/// Draw one flow of `family`.
pub fn synth_flow(family: Family, rng: &mut Rng) -> SynthFlow {
    let n = rng.range(16, 64) as usize;
    let mut pkts = Vec::with_capacity(n);
    let (proto, dport) = match family {
        Family::WebClient => (6, 443),
        Family::WebDownload => (6, rng.range(49_152, 65_535) as u16),
        Family::Dns => (17, 53),
        Family::Ssh => (6, 22),
        Family::Quic => (17, 443),
        Family::SynFlood => (6, [80, 443, 22, 8080][rng.below(4) as usize]),
        Family::UdpFlood => (17, rng.range(1_024, 65_535) as u16),
        Family::StealthScan => (6, rng.range(1, 10_000) as u16),
        Family::RstStorm => (6, [80, 443, 8443][rng.below(3) as usize]),
        Family::Beacon => (6, [4444, 8081, 31337, 6667][rng.below(4) as usize]),
    };
    let period = rng.log_range(200.0, 5_000.0); // beacon period, ms
    let scan_flags = [0x00u8, 0x01, 0x29][rng.below(3) as usize];
    for i in 0..n {
        let p = match family {
            Family::WebClient => {
                let flags = if i == 0 {
                    0x02
                } else if rng.below(3) == 0 {
                    0x18
                } else {
                    0x10
                };
                let len = if flags == 0x18 {
                    rng.range(200, 900)
                } else {
                    rng.range(54, 80)
                };
                SynthPkt {
                    gap_ns: us(rng.log_range(50.0, 50_000.0)),
                    len: len as u32,
                    flags,
                }
            }
            Family::WebDownload => SynthPkt {
                gap_ns: us(rng.log_range(5.0, 400.0)),
                len: if rng.below(8) == 0 {
                    rng.range(200, 1_500)
                } else {
                    1_514
                } as u32,
                flags: if rng.below(4) == 0 { 0x18 } else { 0x10 },
            },
            Family::Dns => SynthPkt {
                gap_ns: us(rng.log_range(2_000.0, 2_000_000.0)),
                len: rng.range(64, 260) as u32,
                flags: 0,
            },
            Family::Ssh => SynthPkt {
                gap_ns: us(rng.log_range(20_000.0, 800_000.0)),
                len: rng.range(66, 220) as u32,
                flags: if rng.below(5) == 0 { 0x10 } else { 0x18 },
            },
            Family::Quic => SynthPkt {
                gap_ns: us(rng.log_range(20.0, 20_000.0)),
                len: if i < 2 { 1_292 } else { rng.range(60, 1_400) } as u32,
                flags: 0,
            },
            Family::SynFlood => SynthPkt {
                gap_ns: us(rng.log_range(0.5, 60.0)),
                len: rng.range(54, 74) as u32,
                flags: 0x02,
            },
            Family::UdpFlood => SynthPkt {
                gap_ns: us(rng.log_range(0.5, 30.0)),
                len: rng.range(900, 1_480) as u32,
                flags: 0,
            },
            Family::StealthScan => SynthPkt {
                gap_ns: us(rng.log_range(50.0, 20_000.0)),
                len: rng.range(54, 60) as u32,
                flags: scan_flags,
            },
            Family::RstStorm => SynthPkt {
                gap_ns: us(rng.log_range(0.5, 100.0)),
                len: 54,
                flags: 0x04,
            },
            Family::Beacon => SynthPkt {
                gap_ns: us(period * 1000.0 * (1.0 + (rng.unit() - 0.5) * 0.02)),
                len: rng.range(120, 320) as u32,
                flags: 0x18,
            },
        };
        pkts.push(p);
    }
    SynthFlow {
        family,
        proto,
        dport,
        pkts,
    }
}

/// One labelled feature vector.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Sample {
    /// Sampler snapshot (ScoreJob.feat).
    pub feat: [u32; FEAT_WORDS],
    /// 1 = anomalous family.
    pub label: u8,
    /// Family.
    pub family: Family,
}

/// Run a flow through the sampler cartridge (reference runner, as cart_xdp
/// would) and return the snapshots it queues.
pub fn sample_flow(sampler: &Image, flow: &SynthFlow, start_ns: u64) -> Vec<[u32; FEAT_WORDS]> {
    let mut st = CartState::zeroed();
    let mut meta = FlowMetaSim::default();
    let mut now = start_ns;
    let mut out = vec![];
    for p in &flow.pkts {
        now += p.gap_ns;
        let mut f = [0u32; FIELD_COUNT as usize];
        f[field::L3_PROTO as usize] = 4;
        f[field::L4_PROTO as usize] = flow.proto as u32;
        f[field::SRC_PORT as usize] = 50_000;
        f[field::DST_PORT as usize] = flow.dport as u32;
        f[field::PKT_LEN as usize] = p.len;
        f[field::IP_TTL as usize] = 64;
        f[field::TCP_FLAGS as usize] = if flow.proto == 6 { p.flags as u32 } else { 0 };
        let o = run_packet(sampler, &mut st, &meta.packet(now, f));
        out.extend(o.queued);
    }
    out
}

/// `flows_per_family` flows of every family, seeded, through the sampler.
pub fn generate(sampler: &Image, seed: u64, flows_per_family: usize) -> Vec<Sample> {
    let mut rng = Rng(seed | 1);
    let mut out = vec![];
    for _ in 0..flows_per_family {
        for &fam in &FAMILIES {
            let flow = synth_flow(fam, &mut rng);
            let start = 1_000_000_000 + rng.below(1 << 40);
            for feat in sample_flow(sampler, &flow, start) {
                out.push(Sample {
                    feat,
                    label: fam.label(),
                    family: fam,
                });
            }
        }
    }
    out
}

/// Model input from a snapshot: x_k = min(f_k >> SH_IN[k], 255).
pub fn inputs(feat: &[u32; FEAT_WORDS]) -> [u32; FEAT_WORDS] {
    let mut x = [0u32; FEAT_WORDS];
    for k in 0..FEAT_WORDS {
        x[k] = (feat[k] >> SH_IN[k]).min(255);
    }
    x
}

/// The float model (weights on the raw u8 inputs x).
#[derive(Clone, Debug, PartialEq)]
pub struct FloatMlp {
    /// HIDDEN x 8.
    pub w1: Vec<[f64; FEAT_WORDS]>,
    /// HIDDEN.
    pub b1: Vec<f64>,
    /// HIDDEN.
    pub w2: Vec<f64>,
    /// Output bias.
    pub b2: f64,
}

fn sigmoid(z: f64) -> f64 {
    1.0 / (1.0 + (-z).exp())
}

impl FloatMlp {
    /// Logit and hidden activations for inputs x.
    pub fn forward(&self, x: &[u32; FEAT_WORDS]) -> (f64, Vec<f64>) {
        let h: Vec<f64> = (0..HIDDEN)
            .map(|j| {
                let a: f64 = (0..FEAT_WORDS).map(|k| self.w1[j][k] * x[k] as f64).sum();
                (a + self.b1[j]).max(0.0)
            })
            .collect();
        let z = self.b2 + (0..HIDDEN).map(|j| self.w2[j] * h[j]).sum::<f64>();
        (z, h)
    }
    /// Probability of "anomalous".
    pub fn prob(&self, feat: &[u32; FEAT_WORDS]) -> f64 {
        sigmoid(self.forward(&inputs(feat)).0)
    }
    /// Score 0..100.
    pub fn score(&self, feat: &[u32; FEAT_WORDS]) -> u32 {
        (100.0 * self.prob(feat)).round() as u32
    }
}

/// Training settings.
#[derive(Clone, Copy, Debug)]
pub struct FitCfg {
    /// Init / shuffle seed.
    pub seed: u64,
    /// Passes over the training set.
    pub epochs: usize,
    /// Adam learning rate.
    pub lr: f64,
    /// Mini-batch size.
    pub batch: usize,
    /// L2 weight decay.
    pub l2: f64,
}

impl Default for FitCfg {
    fn default() -> Self {
        FitCfg {
            seed: 0x5C0_4E5,
            epochs: 120,
            lr: 0.01,
            batch: 64,
            l2: 1e-4,
        }
    }
}

/// Train the float MLP (Adam, logistic loss) on `train`. Inputs are scaled
/// by 1/128 during training and the scale is folded into w1 at the end, so
/// the returned weights apply to raw x.
pub fn fit(train: &[Sample], cfg: FitCfg) -> FloatMlp {
    const S: f64 = 1.0 / 128.0;
    let mut rng = Rng(cfg.seed | 1);
    let mut w1 = vec![[0.0f64; FEAT_WORDS]; HIDDEN];
    let lim1 = (6.0 / FEAT_WORDS as f64).sqrt();
    for row in w1.iter_mut() {
        for w in row.iter_mut() {
            *w = (rng.unit() * 2.0 - 1.0) * lim1;
        }
    }
    let mut b1 = vec![0.01f64; HIDDEN];
    let lim2 = (6.0 / HIDDEN as f64).sqrt();
    let mut w2: Vec<f64> = (0..HIDDEN)
        .map(|_| (rng.unit() * 2.0 - 1.0) * lim2)
        .collect();
    let mut b2 = 0.0f64;
    let n_par = HIDDEN * FEAT_WORDS + 2 * HIDDEN + 1;
    let (mut m, mut v) = (vec![0.0f64; n_par], vec![0.0f64; n_par]);
    let (beta1, beta2, eps) = (0.9f64, 0.999f64, 1e-8f64);
    let mut t = 0i32;
    let xs: Vec<[f64; FEAT_WORDS]> = train
        .iter()
        .map(|s| {
            let x = inputs(&s.feat);
            let mut o = [0.0; FEAT_WORDS];
            for k in 0..FEAT_WORDS {
                o[k] = x[k] as f64 * S;
            }
            o
        })
        .collect();
    let mut order: Vec<usize> = (0..train.len()).collect();
    for _ in 0..cfg.epochs {
        for i in (1..order.len()).rev() {
            order.swap(i, rng.below(i as u64 + 1) as usize);
        }
        for chunk in order.chunks(cfg.batch) {
            let mut g = vec![0.0f64; n_par];
            for &i in chunk {
                let x = &xs[i];
                let mut a = [0.0f64; HIDDEN];
                let mut h = [0.0f64; HIDDEN];
                for j in 0..HIDDEN {
                    a[j] = b1[j] + (0..FEAT_WORDS).map(|k| w1[j][k] * x[k]).sum::<f64>();
                    h[j] = a[j].max(0.0);
                }
                let z = b2 + (0..HIDDEN).map(|j| w2[j] * h[j]).sum::<f64>();
                let dz = sigmoid(z) - train[i].label as f64;
                for j in 0..HIDDEN {
                    g[HIDDEN * FEAT_WORDS + HIDDEN + j] += dz * h[j];
                    if a[j] > 0.0 {
                        let da = dz * w2[j];
                        for k in 0..FEAT_WORDS {
                            g[j * FEAT_WORDS + k] += da * x[k];
                        }
                        g[HIDDEN * FEAT_WORDS + j] += da;
                    }
                }
                g[n_par - 1] += dz;
            }
            t += 1;
            let bs = chunk.len() as f64;
            let mut idx = 0;
            let mut upd = |p: &mut f64, gi: f64, decay: bool| {
                let gi = gi / bs + if decay { cfg.l2 * *p } else { 0.0 };
                m[idx] = beta1 * m[idx] + (1.0 - beta1) * gi;
                v[idx] = beta2 * v[idx] + (1.0 - beta2) * gi * gi;
                let mh = m[idx] / (1.0 - beta1.powi(t));
                let vh = v[idx] / (1.0 - beta2.powi(t));
                *p -= cfg.lr * mh / (vh.sqrt() + eps);
                idx += 1;
            };
            for j in 0..HIDDEN {
                for k in 0..FEAT_WORDS {
                    upd(&mut w1[j][k], g[j * FEAT_WORDS + k], true);
                }
            }
            for j in 0..HIDDEN {
                upd(&mut b1[j], g[HIDDEN * FEAT_WORDS + j], false);
            }
            for j in 0..HIDDEN {
                upd(&mut w2[j], g[HIDDEN * FEAT_WORDS + HIDDEN + j], true);
            }
            upd(&mut b2, g[n_par - 1], false);
        }
    }
    for row in w1.iter_mut() {
        for w in row.iter_mut() {
            *w *= S;
        }
    }
    FloatMlp { w1, b1, w2, b2 }
}

/// One quantized hidden row.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct QRow {
    /// Bias with the requantization rounding offset folded in.
    pub b1: i32,
    /// int8 weights.
    pub w1: [i8; FEAT_WORDS],
    /// int8 second-layer weight.
    pub w2: i8,
    /// Requantization shift.
    pub sh: u8,
}

/// The quantized model: exactly what `scorer.s` computes.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct QModel {
    /// Hidden rows.
    pub rows: Vec<QRow>,
    /// Output bias with the table-index rounding offset folded in.
    pub b2: i32,
    /// Output shift to the table index.
    pub so: u8,
    /// Sigmoid table, score 0..100 per index.
    pub lut: [u8; 256],
    /// Model id (FNV-1a of the weight bytes), carried in every score event.
    pub model_id: u32,
}

/// Scales `quantize` chose (for the sidecar).
#[derive(Clone, Debug, PartialEq)]
pub struct QScales {
    /// Per-row first-layer scale.
    pub s1: Vec<f64>,
    /// Second-layer scale.
    pub s2: f64,
    /// Logit width of one table bin.
    pub bin: f64,
}

fn round_i(x: f64) -> i64 {
    x.round() as i64
}

/// Quantize `m`; `calib` (training vectors) sets the activation shifts.
///
/// Rounding offsets fold into the biases exactly: for acc' = acc + 2^(s-1),
/// floor(ReLU(acc') / 2^s) = round(acc / 2^s) when acc >= 0, and 0 when
/// acc < 0 (acc' < 2^(s-1)), which is what ReLU then shift-round gives.
pub fn quantize(m: &FloatMlp, calib: &[Sample]) -> (QModel, QScales) {
    let mut rows = Vec::with_capacity(HIDDEN);
    let mut s1 = Vec::with_capacity(HIDDEN);
    for j in 0..HIDDEN {
        let mx = m.w1[j].iter().fold(0.0f64, |a, w| a.max(w.abs()));
        let s = if mx > 0.0 { mx / 127.0 } else { 1.0 };
        let mut w = [0i8; FEAT_WORDS];
        for k in 0..FEAT_WORDS {
            w[k] = round_i(m.w1[j][k] / s).clamp(-127, 127) as i8;
        }
        let b = round_i(m.b1[j] / s).clamp(i32::MIN as i64 / 4, i32::MAX as i64 / 4) as i32;
        // Activation shift: the largest pre-shift activation over the
        // calibration set fits 255 after rounding.
        let mut amax = 0i64;
        for c in calib {
            let x = inputs(&c.feat);
            let acc = b as i64
                + (0..FEAT_WORDS)
                    .map(|k| w[k] as i64 * x[k] as i64)
                    .sum::<i64>();
            amax = amax.max(acc);
        }
        let mut sh = 0u8;
        while sh < 24 && ((amax + if sh > 0 { 1 << (sh - 1) } else { 0 }) >> sh) > 255 {
            sh += 1;
        }
        let b_fold = b + if sh > 0 { 1 << (sh - 1) } else { 0 };
        rows.push(QRow {
            b1: b_fold,
            w1: w,
            w2: 0,
            sh,
        });
        s1.push(s);
    }
    // Second layer on the u8 activations: h_j ~= s1_j * 2^sh_j * hq_j.
    let v: Vec<f64> = (0..HIDDEN)
        .map(|j| m.w2[j] * s1[j] * (1u64 << rows[j].sh) as f64)
        .collect();
    let vmax = v.iter().fold(0.0f64, |a, x| a.max(x.abs()));
    let s2 = if vmax > 0.0 { vmax / 127.0 } else { 1.0 };
    for j in 0..HIDDEN {
        rows[j].w2 = round_i(v[j] / s2).clamp(-127, 127) as i8;
    }
    // Table bins ~0.04 logit wide: under half a score point of rounding at
    // the steepest part of the sigmoid; 256 bins cover about +-5 logits.
    let target = 0.04;
    let so = ((target / s2).log2().round().max(0.0) as u8).min(24);
    let bin = s2 * (1u64 << so) as f64;
    let b2 = round_i(m.b2 / s2) as i32 + if so > 0 { 1 << (so - 1) } else { 0 };
    let mut lut = [0u8; 256];
    for (i, e) in lut.iter_mut().enumerate() {
        *e = (100.0 * sigmoid(bin * (i as f64 - 128.0))).round() as u8;
    }
    let mut q = QModel {
        rows,
        b2,
        so,
        lut,
        model_id: 0,
    };
    let d = q.to_data_bytes();
    q.model_id = fnv1a(&d[layout::ROWS..]) ^ fnv1a(&d[layout::B2..layout::B2 + 4]);
    (q, QScales { s1, s2, bin })
}

fn fnv1a(b: &[u8]) -> u32 {
    let mut h: u32 = 0x811c_9dc5;
    for &x in b {
        h ^= x as u32;
        h = h.wrapping_mul(0x0100_0193);
    }
    h
}

impl QModel {
    /// Score 0..100 in the cartridge's 32-bit arithmetic (bit-exact with
    /// `scorer.s`).
    pub fn score(&self, feat: &[u32; FEAT_WORDS]) -> u32 {
        let x = inputs(feat);
        let sum_x = x.iter().fold(0u32, |a, &b| a.wrapping_add(b));
        let corr = sum_x.wrapping_shl(7);
        let mut out = 0u32;
        for r in &self.rows {
            let mut acc = r.b1 as u32;
            for k in 0..FEAT_WORDS {
                let wb = (r.w1[k] as i32 + 128) as u32;
                acc = acc.wrapping_add(wb.wrapping_mul(x[k]));
            }
            acc = acc.wrapping_sub(corr);
            if acc & 0x8000_0000 != 0 {
                acc = 0;
            }
            let h = (acc >> (r.sh & 31)).min(255);
            out = out.wrapping_add((r.w2 as i32 as u32).wrapping_mul(h));
        }
        out = out.wrapping_add(self.b2 as u32);
        let mut idx = (((out as i32) >> (self.so & 31)) as u32).wrapping_add(128);
        if idx & 0x8000_0000 != 0 {
            idx = 0;
        }
        if idx >= 255 {
            idx = 255;
        }
        self.lut[idx as usize] as u32
    }

    /// The DATA blob (layout above), as bytes.
    pub fn to_data_bytes(&self) -> Vec<u8> {
        let mut d = vec![0u8; layout::LEN];
        d[layout::SH_IN..layout::SH_IN + FEAT_WORDS].copy_from_slice(&SH_IN);
        d[layout::MODEL_ID..layout::MODEL_ID + 4].copy_from_slice(&self.model_id.to_le_bytes());
        d[layout::SO] = self.so;
        d[layout::B2..layout::B2 + 4].copy_from_slice(&self.b2.to_le_bytes());
        for (j, r) in self.rows.iter().enumerate() {
            let o = layout::ROWS + j * layout::ROW;
            d[o..o + 4].copy_from_slice(&r.b1.to_le_bytes());
            for k in 0..FEAT_WORDS {
                d[o + 4 + k] = (r.w1[k] as i32 + 128) as u8;
            }
            d[o + 12] = r.w2 as u8;
            d[o + 13] = r.sh;
        }
        d[layout::LUT..layout::LUT + 256].copy_from_slice(&self.lut);
        d
    }

    /// The DATA blob as cartridge words.
    pub fn to_data(&self) -> Vec<u32> {
        crate::data_words(&self.to_data_bytes()).expect("layout is word-aligned")
    }

    /// Parse a DATA blob (inverse of `to_data_bytes`).
    pub fn from_data_bytes(d: &[u8]) -> Result<QModel, String> {
        if d.len() != layout::LEN {
            return Err(format!(
                "scorer DATA is {} bytes, want {}",
                d.len(),
                layout::LEN
            ));
        }
        if d[layout::SH_IN..layout::SH_IN + FEAT_WORDS] != SH_IN {
            return Err("input shifts differ from this build's SH_IN".into());
        }
        let le32 = |o: usize| u32::from_le_bytes([d[o], d[o + 1], d[o + 2], d[o + 3]]);
        let mut rows = Vec::with_capacity(HIDDEN);
        for j in 0..HIDDEN {
            let o = layout::ROWS + j * layout::ROW;
            let mut w1 = [0i8; FEAT_WORDS];
            for k in 0..FEAT_WORDS {
                w1[k] = (d[o + 4 + k] as i32 - 128) as i8;
            }
            rows.push(QRow {
                b1: le32(o) as i32,
                w1,
                w2: d[o + 12] as i8,
                sh: d[o + 13],
            });
        }
        let mut lut = [0u8; 256];
        lut.copy_from_slice(&d[layout::LUT..layout::LUT + 256]);
        Ok(QModel {
            rows,
            b2: le32(layout::B2) as i32,
            so: d[layout::SO],
            lut,
            model_id: le32(layout::MODEL_ID),
        })
    }
}

/// Binary classification metrics at the score threshold 50.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct Metrics {
    /// Vectors.
    pub n: usize,
    /// Fraction classified correctly.
    pub accuracy: f64,
    /// Anomalous vectors scored >= 50.
    pub tpr: f64,
    /// Benign vectors scored >= 50.
    pub fpr: f64,
    /// Area under the ROC curve (rank statistic).
    pub auc: f64,
}

/// Metrics of `score` (0..100) over `set`.
pub fn metrics(set: &[Sample], score: impl Fn(&[u32; FEAT_WORDS]) -> f64) -> Metrics {
    let mut s: Vec<(f64, u8)> = set.iter().map(|x| (score(&x.feat), x.label)).collect();
    let pos = s.iter().filter(|x| x.1 == 1).count();
    let neg = s.len() - pos;
    let tp = s.iter().filter(|x| x.1 == 1 && x.0 >= 50.0).count();
    let fp = s.iter().filter(|x| x.1 == 0 && x.0 >= 50.0).count();
    let correct = tp + (neg - fp);
    // AUC = P(score_pos > score_neg) + 0.5 P(tie), via ranks.
    s.sort_by(|a, b| a.0.partial_cmp(&b.0).unwrap_or(core::cmp::Ordering::Equal));
    let mut rank_sum = 0.0f64;
    let mut i = 0;
    while i < s.len() {
        let mut j = i;
        while j < s.len() && s[j].0 == s[i].0 {
            j += 1;
        }
        let avg = (i + j + 1) as f64 / 2.0; // 1-based average rank
        rank_sum += avg * s[i..j].iter().filter(|x| x.1 == 1).count() as f64;
        i = j;
    }
    let auc = if pos == 0 || neg == 0 {
        0.0
    } else {
        (rank_sum - (pos * (pos + 1)) as f64 / 2.0) / (pos as f64 * neg as f64)
    };
    Metrics {
        n: s.len(),
        accuracy: correct as f64 / s.len().max(1) as f64,
        tpr: tp as f64 / pos.max(1) as f64,
        fpr: fp as f64 / neg.max(1) as f64,
        auc,
    }
}

/// Quantization error |score_q - score_float| over `set`: (max, fraction
/// within 2 points, mean).
pub fn quant_error(m: &FloatMlp, q: &QModel, set: &[Sample]) -> (u32, f64, f64) {
    let mut mx = 0u32;
    let mut within = 0usize;
    let mut sum = 0u64;
    for s in set {
        let d = q.score(&s.feat).abs_diff(m.score(&s.feat));
        mx = mx.max(d);
        within += (d <= 2) as usize;
        sum += d as u64;
    }
    let n = set.len().max(1) as f64;
    (mx, within as f64 / n, sum as f64 / n)
}

/// Pull every number after `"key":` up to the matching close bracket (the
/// sidecar's flat format; no JSON dependency).
pub fn json_numbers(src: &str, key: &str) -> Option<Vec<f64>> {
    let pat = format!("\"{key}\":");
    let start = src.find(&pat)? + pat.len();
    let rest = &src[start..];
    let mut depth = 0i32;
    let mut end = rest.len();
    for (i, c) in rest.char_indices() {
        match c {
            '[' => depth += 1,
            ']' => {
                depth -= 1;
                if depth == 0 {
                    end = i + 1;
                    break;
                }
            }
            ',' | '\n' | '}' if depth == 0 => {
                end = i;
                break;
            }
            _ => {}
        }
    }
    let body = &rest[..end];
    let nums: Vec<f64> = body
        .split(|c: char| {
            !(c.is_ascii_digit() || c == '.' || c == '-' || c == 'e' || c == 'E' || c == '+')
        })
        .filter(|t| !t.is_empty())
        .filter_map(|t| t.parse().ok())
        .collect();
    Some(nums)
}

impl FloatMlp {
    /// Rebuild from the sidecar's "w1"/"b1"/"w2"/"b2" arrays.
    pub fn from_json(src: &str) -> Result<FloatMlp, String> {
        let get = |k: &str, n: usize| -> Result<Vec<f64>, String> {
            let v = json_numbers(src, k).ok_or_else(|| format!("sidecar has no \"{k}\""))?;
            if v.len() != n {
                return Err(format!("\"{k}\": {} numbers, want {n}", v.len()));
            }
            Ok(v)
        };
        let w1f = get("w1", HIDDEN * FEAT_WORDS)?;
        let mut w1 = vec![[0.0; FEAT_WORDS]; HIDDEN];
        for j in 0..HIDDEN {
            w1[j].copy_from_slice(&w1f[j * FEAT_WORDS..(j + 1) * FEAT_WORDS]);
        }
        Ok(FloatMlp {
            w1,
            b1: get("b1", HIDDEN)?,
            w2: get("w2", HIDDEN)?,
            b2: get("b2", 1)?[0],
        })
    }

    /// The sidecar's weight arrays (full f64 precision, `{:e}`).
    pub fn json_fields(&self) -> String {
        let f = |v: &[f64]| {
            v.iter()
                .map(|x| format!("{x:e}"))
                .collect::<Vec<_>>()
                .join(", ")
        };
        let rows: Vec<String> = self.w1.iter().map(|r| format!("[{}]", f(r))).collect();
        format!(
            "  \"w1\": [{}],\n  \"b1\": [{}],\n  \"w2\": [{}],\n  \"b2\": {:e}",
            rows.join(", "),
            f(&self.b1),
            f(&self.w2),
            self.b2
        )
    }
}
