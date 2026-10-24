// SPDX-License-Identifier: GPL-3.0-or-later
//! S5 Phase 15 (Steps 372-374): the shipped int8 scorer
//! (`cartridges/scorer.s` + `scorer.data.bin` + `scorer.model.json`).
//!
//! - the tick-stage image validates with its declared loop bound;
//! - the cartridge (reference runner) equals the integer model bit for bit
//!   on held-out synthetic vectors and on random/extreme vectors;
//! - quantization gate: |int8 score - float score| <= 2 on >= 99 % of
//!   >= 1,000 held-out vectors (the float model is read from the sidecar);
//! - sanity: anomalous families score high, benign low, on held-out flows.
//!
//! Held-out vectors are regenerated from the eval seed used by `trainquant`
//! (0xE7A1, disjoint from the train seed 0x7A11).

use monad_common::cart::{FEAT_WORDS, STAGE_TICK, STAGE_XDP};
use upc_cartridge::reference::run_tick_job;
use upc_cartridge::scorer::{
    generate, json_numbers, metrics, quant_error, FloatMlp, QModel, Rng, Sample, HIDDEN, SCORE_KIND,
};
use upc_cartridge::{build_image, data_words, validate, Image};

fn repo(p: &str) -> String {
    format!("{}/../../{p}", env!("CARGO_MANIFEST_DIR"))
}

fn shipped() -> (QModel, FloatMlp, String, Image) {
    let data = std::fs::read(repo("cartridges/scorer.data.bin")).expect("scorer.data.bin");
    let q = QModel::from_data_bytes(&data).expect("valid scorer DATA");
    let json = std::fs::read_to_string(repo("cartridges/scorer.model.json")).expect("sidecar");
    let m = FloatMlp::from_json(&json).expect("float weights in the sidecar");
    let src = std::fs::read_to_string(repo("cartridges/scorer.s")).expect("scorer.s");
    let img = build_image(
        STAGE_TICK,
        &src,
        data_words(&data).unwrap(),
        None,
        HIDDEN as u16,
    )
    .expect("builds + validates");
    (q, m, json, img)
}

fn eval_set() -> Vec<Sample> {
    let src = std::fs::read_to_string(repo("cartridges/sampler.s")).unwrap();
    let n = data_words(&std::fs::read(repo("cartridges/sampler.data.bin")).unwrap()).unwrap();
    let sampler = build_image(STAGE_XDP, &src, n, None, 0).unwrap();
    generate(&sampler, 0xE7A1, 100)
}

fn cartridge_score(img: &Image, feat: &[u32; FEAT_WORDS]) -> (Vec<(u32, u32, u32)>, usize) {
    let (_, calls) = run_tick_job(img, feat, 0, 16);
    let emits = calls.iter().flat_map(|c| c.emits.clone()).collect();
    (emits, calls.len())
}

#[test]
fn scorer_image_validates() {
    let (q, _, json, img) = shipped();
    let r = validate(&img).unwrap();
    assert!(r.has_back_edges);
    assert_eq!(r.syscalls, vec![3]); // EMIT only
    assert_eq!(img.loop_bound, HIDDEN as u16);
    let bytes = img.encode().len();
    assert!(bytes <= 2048, "image {bytes} bytes");
    assert_eq!(
        json_numbers(&json, "model_id").unwrap(),
        vec![q.model_id as f64]
    );
    eprintln!(
        "SCORER image {bytes} bytes (rom {} words, data {} words), model {:#010x}",
        img.rom.len(),
        img.data.len(),
        q.model_id
    );
}

/// The cartridge reproduces QModel::score exactly: held-out vectors plus
/// random and extreme ones (clamps, wrap-free sums, zero vector).
#[test]
fn scorer_cartridge_equals_integer_reference() {
    let (q, _, _, img) = shipped();
    let mut vecs: Vec<[u32; FEAT_WORDS]> = eval_set().iter().map(|s| s.feat).collect();
    let mut rng = Rng(0xF00D);
    for _ in 0..2_000 {
        let mut f = [0u32; FEAT_WORDS];
        for v in f.iter_mut() {
            *v = match rng.below(4) {
                0 => 0,
                1 => rng.next() as u32,
                2 => u32::MAX,
                _ => rng.below(70_000) as u32,
            };
        }
        vecs.push(f);
    }
    let mut max_calls = 0;
    for f in &vecs {
        let (emits, calls) = cartridge_score(&img, f);
        assert_eq!(emits, vec![(SCORE_KIND, q.score(f), q.model_id)], "{f:?}");
        assert!(q.score(f) <= 100);
        max_calls = max_calls.max(calls);
    }
    assert!(max_calls <= 3, "{max_calls} tick calls");
    eprintln!(
        "SCORER cartridge == integer reference on {} vectors, <= {max_calls} tick calls per job",
        vecs.len()
    );
}

/// Quantization-accuracy gate (Step 374).
#[test]
fn scorer_quantization_gate() {
    let (q, m, _, _) = shipped();
    let eval = eval_set();
    assert!(eval.len() >= 1_000);
    let (max, within, mean) = quant_error(&m, &q, &eval);
    eprintln!(
        "SCORER quantization: {} held-out vectors, |int8 - float| <= 2 on {:.2}%, max {max}, mean {mean:.3}",
        eval.len(),
        100.0 * within
    );
    assert!(within >= 0.99, "within 2 points on {:.2}%", 100.0 * within);
}

/// Held-out separation and the per-feature sanity the plan asks for.
#[test]
fn scorer_separates_held_out_flows() {
    let (q, _, _, _) = shipped();
    let eval = eval_set();
    let mi = metrics(&eval, |f| q.score(f) as f64);
    eprintln!(
        "SCORER held-out int8: accuracy {:.4} tpr {:.4} fpr {:.4} auc {:.4} (n {})",
        mi.accuracy, mi.tpr, mi.fpr, mi.auc, mi.n
    );
    assert!(mi.accuracy >= 0.99 && mi.fpr <= 0.01);
    // Monotone in the obvious directions, from three benign windows: the
    // score never falls as the window's largest packet shrinks or as its
    // packets arrive faster (both point toward the flood families).
    let bases = [
        ("web", [1_400u32, 90, 0, 600, 16, 6, 443, 0x18]),
        ("dns", [1_200, 120, 0, 200, 16, 17, 53, 0]),
        ("ssh", [1_100, 140, 0, 180, 16, 6, 22, 0x18]),
    ];
    for (name, b) in bases {
        assert!(q.score(&b) <= 10, "{name} base scores {}", q.score(&b));
        for (k, what) in [(3usize, "win_max_len"), (1, "win_iat")] {
            let mut prev = None;
            let mut trail = vec![];
            for v in (0..=11).rev().map(|e| (1u32 << e) - 1) {
                let mut f = b;
                f[k] = v;
                let s = q.score(&f);
                if let Some(p) = prev {
                    assert!(s >= p, "{name}: {what} {v} scores {s} < {p}");
                }
                prev = Some(s);
                trail.push(s);
            }
            eprintln!("SCORER {name} {what} 2047 -> 0: {trail:?}");
        }
    }
}
