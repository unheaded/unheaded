// SPDX-License-Identifier: GPL-3.0-or-later
//! S5 Phase 13 (Steps 334-335, 344): `cartridges/ratelimit.s` against its
//! correctness oracle, the token bucket of `ebpf/qos-ebpf`, re-implemented
//! here line for line in 64-bit arithmetic (the native engine is the oracle;
//! the cartridge is fixed if they differ).

use monad_common::cart::{field, verdict, CartState, Step, FIELD_COUNT, STAGE_XDP};
use upc_cartridge::reference::{run_packet, xdp_verdict, FlowMetaSim};
use upc_cartridge::{build_image, data_words, validate, Image};

const M: u32 = 274_877_907; // ceil(2^35 / 125)

fn repo(p: &str) -> String {
    format!("{}/../../{p}", env!("CARGO_MANIFEST_DIR"))
}

fn image(rate_mbps: u32, cap: u32) -> Image {
    let src = std::fs::read_to_string(repo("cartridges/ratelimit.s")).expect("ratelimit.s");
    build_image(STAGE_XDP, &src, vec![rate_mbps, cap, M], None, 0).expect("builds + validates")
}

/// qos-ebpf's token bucket (ebpf/qos-ebpf/src/main.rs, "Token Bucket Rate
/// Limiting"; refill step = `monad_common::token_bucket::refill` since
/// 8ab534b3), per flow, in its own u64 arithmetic. The refill credits whole
/// 8,000 ns quanta and advances `last` by exactly those quanta, so the
/// remainder carries; a refill that reaches the capacity sets `last = now`.
#[derive(Default)]
struct QosOracle {
    bucket: Option<(u64, u64)>, // (tokens, last_refill_ns)
}

impl QosOracle {
    fn packet(&mut self, rate_mbps: u64, cap: u64, now: u64, pkt_bytes: u64) -> u8 {
        if rate_mbps == 0 {
            return verdict::PASS;
        }
        match self.bucket.as_mut() {
            None => {
                self.bucket = Some((cap.saturating_sub(pkt_bytes), now));
                verdict::PASS
            }
            Some((tokens, last)) => {
                let q = now.saturating_sub(*last) / 8_000;
                if q > 0 {
                    if q > cap / rate_mbps || *tokens >= cap || q * rate_mbps >= cap - *tokens {
                        *tokens = cap;
                        *last = now;
                    } else {
                        *tokens += q * rate_mbps;
                        *last += q * 8_000;
                    }
                }
                if *tokens < pkt_bytes {
                    return verdict::DROP;
                }
                *tokens -= pkt_bytes;
                verdict::PASS
            }
        }
    }
}

fn base(len: u32) -> [u32; FIELD_COUNT as usize] {
    let mut f = [0u32; FIELD_COUNT as usize];
    f[field::L3_PROTO as usize] = 4;
    f[field::L4_PROTO as usize] = 17;
    f[field::PKT_LEN as usize] = len;
    f
}

/// Run one flow's packets `(gap_ns, len)` through the cartridge; verdicts.
fn run_flow(img: &Image, pkts: &[(u64, u32)]) -> (Vec<u8>, CartState) {
    let mut st = CartState::zeroed();
    let mut meta = FlowMetaSim::default();
    let mut now = 1_000_000_000u64;
    let mut out = vec![];
    for &(gap, len) in pkts {
        now += gap;
        let inp = meta.packet(now, base(len));
        let o = run_packet(img, &mut st, &inp);
        let step = o.step.expect("stopped");
        assert!(matches!(step, Step::Verdict(_)), "faulted: {step:?}");
        out.push(xdp_verdict(img, step));
    }
    (out, st)
}

const P: u8 = verdict::PASS;
const D: u8 = verdict::DROP;

#[test]
fn ratelimit_validates_within_budget() {
    let img = image(100, 12_500);
    let r = validate(&img).unwrap();
    assert!(img.max_insn <= 64 && !r.has_back_edges);
    assert_eq!(r.syscalls, vec![0, 1, 2]); // FIELD, NOW_NS_LO, VERDICT only
                                           // The shipped DATA blob is the documented default and obeys the
                                           // parameter rules (cap <= 524287 * rate, rate <= 8000).
    let shipped =
        data_words(&std::fs::read(repo("cartridges/ratelimit.data.bin")).unwrap()).unwrap();
    assert_eq!(shipped, vec![100, 12_500, M]);
    assert!(shipped[1] as u64 <= 524_287 * shipped[0] as u64 && shipped[0] <= 8_000);
}

/// Longest instruction path through a forward-only XDP image, counting the
/// final VERDICT/HALT: a conditional branch may fall through or jump, JMP
/// only jumps, VERDICT (SYSCALL 2) and HALT end the run.
fn longest_path(rom: &[u32]) -> usize {
    use monad_common::{mbc_opcodes as op, MbcInsn};
    let n = rom.len();
    let mut best = vec![0usize; n + 1];
    for i in (0..n).rev() {
        let w = MbcInsn(rom[i]);
        let o = w.opcode();
        let target = || {
            let raw = rom[i] & 0x00FF_FFFF;
            let off = if raw & 0x0080_0000 != 0 {
                raw | 0xFF00_0000
            } else {
                raw
            };
            (i as u32 + 1).wrapping_add(off) as usize
        };
        best[i] = 1 + if o == op::HALT || (o == op::SYSCALL && w.imm16() == 2) {
            0
        } else if o == op::JMP {
            best[target()]
        } else if (op::JZ..=op::JNC).contains(&o) {
            best[i + 1].max(best[target()])
        } else {
            best[i + 1]
        };
    }
    best[0]
}

#[test]
fn ratelimit_longest_path_fits_the_xdp_budget() {
    let img = image(100, 12_500);
    let n = longest_path(&img.rom);
    eprintln!(
        "ratelimit: {} words, longest path {n} of 64 instructions",
        img.rom.len()
    );
    assert!(n <= 64, "longest path {n}");
}

/// elapsed / 8000 == MULH(elapsed >> 6, M) >> 3 for every 32-bit elapsed:
/// floor(e / 8000) = floor(floor(e / 64) / 125), and the MULH identity is
/// checked exhaustively over all 2^26 values of e >> 6.
#[test]
fn ratelimit_division_by_8000_is_exact() {
    for x in 0u32..(1 << 26) {
        let q = (((x as i32 as i64) * (M as i32 as i64)) >> 32) as u32 >> 3;
        assert_eq!(q, x / 125, "x = {x}");
    }
}

#[test]
fn ratelimit_zero_tokens_drops() {
    // cap 100, a 200-byte first packet empties the bucket (qos-ebpf passes
    // a flow's first packet unconditionally); back-to-back packets then drop.
    let img = image(1, 100);
    let (v, st) = run_flow(&img, &[(0, 200), (100, 60), (100, 60)]);
    assert_eq!(v, vec![P, D, D]);
    assert_eq!(st.mem[0], 0);
}

#[test]
fn ratelimit_full_bucket_then_drop_then_refill() {
    // 100 Mbps = 12.5 bytes/us; cap 12,500 = 1 ms. 100-byte packets 1 ns
    // apart: 125 fit (first leaves 12,400), then drops.
    let img = image(100, 12_500);
    let mut pkts = vec![(0u64, 100u32)];
    pkts.extend(std::iter::repeat_n((1, 100), 130));
    let (v, _) = run_flow(&img, &pkts);
    let passes = v.iter().take_while(|&&x| x == P).count();
    assert_eq!(passes, 125);
    assert!(v[125..].iter().all(|&x| x == D));
    // After 1 ms idle the bucket refills: passes resume.
    pkts.push((1_000_000, 100));
    let (v, _) = run_flow(&img, &pkts);
    assert_eq!(*v.last().unwrap(), P);
}

#[test]
fn ratelimit_rate_zero_is_off() {
    let img = image(0, 100);
    let (v, _) = run_flow(&img, &[(0, 1500), (1, 1500), (1, 1500)]);
    assert_eq!(v, vec![P, P, P]);
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

/// A trace mixing bursts (ns apart), steady pacing (us), long idles (up to
/// 6 s, past the 32-bit clock wrap) and sizes 40..1500.
fn trace(rng: &mut Rng, n: usize) -> Vec<(u64, u32)> {
    (0..n)
        .map(|_| {
            let gap = match rng.below(12) {
                0..=3 => rng.below(2_000),
                4..=7 => rng.below(200_000),
                8 => rng.below(20_000_000),
                9 => rng.below(6_000_000_000),
                // Just past the 32-bit clock wrap: a wrapped elapsed is tiny,
                // so only the iat_bucket >= 22 rule refills correctly here.
                10 => (1u64 << 32) + rng.below(2_000_000),
                // Between iat_bucket 22 (4.194 s) and the wrap (4.295 s).
                _ => 4_194_304_000 + rng.below(100_000_000),
            };
            (gap, 40 + rng.below(1461) as u32)
        })
        .collect()
}

/// Step 335: verdicts match the qos-ebpf math exactly over 1,000-packet
/// traces, for several (rate, cap) settings including caps below one packet.
#[test]
fn ratelimit_matches_qos_ebpf_oracle() {
    let mut rng = Rng(0x0005_7A7E);
    let mut total = 0;
    let mut drops = 0;
    for &(rate, cap) in &[
        (100u32, 12_500u32),
        (1, 125),
        (10, 3_000),
        (1_000, 125_000),
        (8_000, 1_000_000),
        (5, 50),
    ] {
        assert!(cap as u64 <= 524_287 * rate as u64);
        let img = image(rate, cap);
        for _ in 0..5 {
            let pkts = trace(&mut rng, 1_000);
            let (got, _) = run_flow(&img, &pkts);
            let mut o = QosOracle::default();
            let mut now = 1_000_000_000u64;
            for (i, &(gap, len)) in pkts.iter().enumerate() {
                now += gap;
                let want = o.packet(rate as u64, cap as u64, now, len as u64);
                assert_eq!(got[i], want, "rate {rate} cap {cap} packet {i}");
            }
            total += got.len();
            drops += got.iter().filter(|&&x| x == D).count();
        }
    }
    // The traces must exercise both outcomes, or the match proves little.
    assert!(
        drops > total / 20 && drops < total - total / 20,
        "{drops}/{total}"
    );
    eprintln!("ORACLE ratelimit == qos-ebpf on {total} packets ({drops} drops)");
}

/// Step 342 (reference side), after the qos-ebpf remainder fix (8ab534b3):
/// 1,000-byte packets every 20 µs (400 Mbps offered, not a multiple of the
/// 8 µs quantum) through a 100 Mbps bucket admit the configured rate. Before
/// the fix both qos-ebpf and the cartridge admitted 80 Mbps here.
#[test]
fn ratelimit_carries_the_refill_remainder() {
    const RATE: u32 = 100;
    const CAP: u32 = 12_500;
    const L: u32 = 1_000;
    const SPACING: u64 = 20_000;
    let img = image(RATE, CAP);
    let pkts: Vec<(u64, u32)> = (0..20_000).map(|_| (SPACING, L)).collect();
    let (got, _) = run_flow(&img, &pkts);
    let mut o = QosOracle::default();
    let mut now = 1_000_000_000u64;
    for (i, &(gap, len)) in pkts.iter().enumerate() {
        now += gap;
        assert_eq!(
            got[i],
            o.packet(RATE as u64, CAP as u64, now, len as u64),
            "packet {i}"
        );
    }
    let passed = got.iter().filter(|&&v| v == P).count() as f64;
    let span_s = (pkts.len() as u64 * SPACING) as f64 / 1e9;
    let mbps = (passed * L as f64 - CAP as f64) * 8.0 / span_s / 1e6;
    eprintln!(
        "RATE 20 us spacing: {passed} of {} passed, enforced {mbps:.2} Mbps (configured {RATE})",
        pkts.len()
    );
    assert!((99.0..=100.5).contains(&mbps), "enforced {mbps:.2} Mbps");
}

/// Step 344 (reference side): two flows keep independent buckets — one
/// flow flooding never changes the other's verdicts.
#[test]
fn ratelimit_flows_are_independent() {
    let img = image(10, 2_000);
    let quiet: Vec<(u64, u32)> = (0..50).map(|_| (1_000_000, 500)).collect();
    let (alone, _) = run_flow(&img, &quiet);
    // Interleave a flood flow; the quiet flow's own state is separate.
    let mut st_q = CartState::zeroed();
    let mut st_f = CartState::zeroed();
    let (mut mq, mut mf) = (FlowMetaSim::default(), FlowMetaSim::default());
    let mut now = 1_000_000_000u64;
    let mut with_flood = vec![];
    let mut flood_drops = 0;
    for &(gap, len) in &quiet {
        for _ in 0..20 {
            now += gap / 40;
            let o = run_packet(&img, &mut st_f, &mf.packet(now, base(1500)));
            flood_drops += (xdp_verdict(&img, o.step.unwrap()) == D) as u32;
        }
        now += gap / 2;
        let o = run_packet(&img, &mut st_q, &mq.packet(now, base(len)));
        with_flood.push(xdp_verdict(&img, o.step.unwrap()));
    }
    assert!(
        flood_drops > 500,
        "the flood must be limited ({flood_drops})"
    );
    assert_eq!(with_flood, alone);
    assert!(alone.iter().all(|&x| x == P));
}
