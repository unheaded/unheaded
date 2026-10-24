// SPDX-License-Identifier: GPL-3.0-or-later
//! `trainquant` — train and quantize the int8 anomaly scorer (S5 Phase 15,
//! ADR-108 D-C6). Pure Rust, f64, no GPU, no new dependency.
//!
//! ```text
//! trainquant gen --sampler-asm S.s --sampler-data N.bin --seed X --flows F --out X.csv
//! trainquant fit --train A.csv --eval B.csv --scorer-asm scorer.s
//!                --data-out scorer.data.bin --json-out scorer.model.json [--epochs E]
//! trainquant control --train A.csv --eval B.csv   # shuffled-label negative control
//! ```
//!
//! `gen` draws F flows of each of the ten synthetic families
//! (`upc_cartridge::scorer`), runs them through the real sampler cartridge on
//! the reference runner and writes the queued snapshots as a labelled CSV
//! (`label,family,f0..f7`). Synthetic only: no capture, no payload.
//!
//! `fit` trains the float MLP on the train CSV, quantizes it (activation
//! shifts calibrated on the train set only), writes the scorer DATA blob and
//! a JSON sidecar with the float weights, scales and the measured train /
//! eval metrics and quantization error, and checks the assembled scorer
//! cartridge reproduces the integer reference on every eval vector.

use std::fmt::Write as _;
use std::process::ExitCode;

use monad_common::cart::{FEAT_WORDS, STAGE_TICK, STAGE_XDP};
use sha2::{Digest, Sha256};
use upc_cartridge::reference::run_tick_job;
use upc_cartridge::scorer::{
    fit, generate, metrics, quant_error, quantize, Family, FitCfg, Metrics, Sample, FEATURES,
    HIDDEN, SAMPLER_N, SH_IN,
};
use upc_cartridge::{build_image, data_words};

fn usage() -> ExitCode {
    eprintln!(
        "usage:\n  trainquant gen --sampler-asm S.s --sampler-data N.bin --seed X --flows F --out X.csv\n  trainquant fit --train A.csv --eval B.csv --scorer-asm scorer.s --data-out D.bin --json-out M.json [--epochs E]\n  trainquant control --train A.csv --eval B.csv"
    );
    ExitCode::from(2)
}

fn flags(args: &[String]) -> Result<std::collections::HashMap<String, String>, String> {
    let mut m = std::collections::HashMap::new();
    let mut it = args.iter();
    while let Some(k) = it.next() {
        let k = k
            .strip_prefix("--")
            .ok_or_else(|| format!("unexpected argument {k:?}"))?;
        let v = it.next().ok_or_else(|| format!("--{k} needs a value"))?;
        m.insert(k.to_string(), v.clone());
    }
    Ok(m)
}

fn need<'a>(m: &'a std::collections::HashMap<String, String>, k: &str) -> Result<&'a str, String> {
    m.get(k)
        .map(String::as_str)
        .ok_or_else(|| format!("--{k} is required"))
}

fn num(s: &str) -> Result<u64, String> {
    if let Some(h) = s.strip_prefix("0x") {
        u64::from_str_radix(h, 16)
    } else {
        s.parse()
    }
    .map_err(|_| format!("bad number {s:?}"))
}

fn read(p: &str) -> Result<Vec<u8>, String> {
    std::fs::read(p).map_err(|e| format!("{p}: {e}"))
}

fn gen(args: &[String]) -> Result<(), String> {
    let m = flags(args)?;
    let src =
        String::from_utf8(read(need(&m, "sampler-asm")?)?).map_err(|_| "sampler asm: not UTF-8")?;
    let data = data_words(&read(need(&m, "sampler-data")?)?)?;
    if data.first() != Some(&SAMPLER_N) {
        return Err(format!(
            "the model is built for a sampler window of {SAMPLER_N}"
        ));
    }
    let sampler = build_image(STAGE_XDP, &src, data, None, 0)?;
    let set = generate(
        &sampler,
        num(need(&m, "seed")?)?,
        num(need(&m, "flows")?)? as usize,
    );
    let mut out = String::from("label,family");
    for f in FEATURES {
        let _ = write!(out, ",{f}");
    }
    out.push('\n');
    for s in &set {
        let _ = write!(out, "{},{}", s.label, s.family.name());
        for v in s.feat {
            let _ = write!(out, ",{v}");
        }
        out.push('\n');
    }
    let path = need(&m, "out")?;
    std::fs::write(path, out).map_err(|e| format!("{path}: {e}"))?;
    let pos = set.iter().filter(|s| s.label == 1).count();
    println!(
        "{path}: {} vectors ({pos} anomalous, {} benign)",
        set.len(),
        set.len() - pos
    );
    Ok(())
}

fn read_csv(p: &str) -> Result<Vec<Sample>, String> {
    let text = String::from_utf8(read(p)?).map_err(|_| format!("{p}: not UTF-8"))?;
    let mut out = vec![];
    for (i, line) in text.lines().enumerate().skip(1) {
        let t: Vec<&str> = line.split(',').collect();
        if t.len() != 2 + FEAT_WORDS {
            return Err(format!("{p}:{}: want {} columns", i + 1, 2 + FEAT_WORDS));
        }
        let family =
            Family::from_name(t[1]).ok_or_else(|| format!("{p}:{}: family {:?}", i + 1, t[1]))?;
        let label: u8 = t[0].parse().map_err(|_| format!("{p}:{}: label", i + 1))?;
        if label != family.label() {
            return Err(format!(
                "{p}:{}: label {label} disagrees with {}",
                i + 1,
                t[1]
            ));
        }
        let mut feat = [0u32; FEAT_WORDS];
        for k in 0..FEAT_WORDS {
            feat[k] = t[2 + k]
                .parse()
                .map_err(|_| format!("{p}:{}: f{k}", i + 1))?;
        }
        out.push(Sample {
            feat,
            label,
            family,
        });
    }
    Ok(out)
}

fn mjson(name: &str, m: &Metrics) -> String {
    format!(
        "\"{name}\": {{\"n\": {}, \"accuracy\": {:.4}, \"tpr\": {:.4}, \"fpr\": {:.4}, \"auc\": {:.4}}}",
        m.n, m.accuracy, m.tpr, m.fpr, m.auc
    )
}

fn fit_cmd(args: &[String]) -> Result<(), String> {
    let m = flags(args)?;
    let train = read_csv(need(&m, "train")?)?;
    let eval = read_csv(need(&m, "eval")?)?;
    let mut cfg = FitCfg::default();
    if let Some(e) = m.get("epochs") {
        cfg.epochs = num(e)? as usize;
    }
    let model = fit(&train, cfg);
    let (q, sc) = quantize(&model, &train);
    let data_bytes = q.to_data_bytes();

    let ftr = metrics(&train, |f| model.prob(f) * 100.0);
    let fev = metrics(&eval, |f| model.prob(f) * 100.0);
    let qtr = metrics(&train, |f| q.score(f) as f64);
    let qev = metrics(&eval, |f| q.score(f) as f64);
    // Eval vectors that also occur, feature for feature, in the train set
    // (short windows of near-deterministic families repeat) say nothing about
    // generalization: the unseen subset is reported on its own.
    let seen: std::collections::HashSet<[u32; FEAT_WORDS]> = train.iter().map(|s| s.feat).collect();
    let unseen: Vec<Sample> = eval
        .iter()
        .filter(|s| !seen.contains(&s.feat))
        .copied()
        .collect();
    let qun = metrics(&unseen, |f| q.score(f) as f64);
    let (qmax, qwithin, qmean) = quant_error(&model, &q, &eval);
    let (tmax, twithin, _) = quant_error(&model, &q, &train);

    // The cartridge must reproduce the integer reference on every eval vector.
    let src =
        String::from_utf8(read(need(&m, "scorer-asm")?)?).map_err(|_| "scorer asm: not UTF-8")?;
    let img = build_image(STAGE_TICK, &src, q.to_data(), None, HIDDEN as u16)?;
    let mut calls_max = 0;
    for s in &eval {
        let (_, calls) = run_tick_job(&img, &s.feat, 0, 16);
        calls_max = calls_max.max(calls.len());
        let emits: Vec<_> = calls.iter().flat_map(|c| c.emits.clone()).collect();
        if emits != vec![(0x41, q.score(&s.feat), q.model_id)] {
            return Err(format!(
                "cartridge disagrees with the integer reference on {:?}: {emits:?}",
                s.feat
            ));
        }
    }

    let per_family = |set: &[Sample]| -> String {
        let mut parts = vec![];
        for fam in upc_cartridge::scorer::FAMILIES {
            let v: Vec<&Sample> = set.iter().filter(|s| s.family == fam).collect();
            if v.is_empty() {
                continue;
            }
            let ok = v
                .iter()
                .filter(|s| (q.score(&s.feat) >= 50) as u8 == s.label)
                .count();
            parts.push(format!(
                "\"{}\": {:.4}",
                fam.name(),
                ok as f64 / v.len() as f64
            ));
        }
        format!("{{{}}}", parts.join(", "))
    };

    let dpath = need(&m, "data-out")?;
    std::fs::write(dpath, &data_bytes).map_err(|e| format!("{dpath}: {e}"))?;
    let sha: String = Sha256::digest(&data_bytes)
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect();
    let json = format!(
        "{{\n  \"model\": \"int8 MLP 8-{HIDDEN}-1, ReLU, sigmoid table\",\n  \"model_id\": {},\n  \"data_bytes\": {},\n  \"data_sha256\": \"{sha}\",\n  \"features\": [{}],\n  \"input_shifts\": [{}],\n  \"sampler_window\": {SAMPLER_N},\n  \"train_vectors\": {}, \"eval_vectors\": {},\n  \"fit\": {{\"seed\": {}, \"epochs\": {}, \"lr\": {}, \"batch\": {}, \"l2\": {}}},\n  {},\n  {},\n  {},\n  {},\n  {},\n  \"eval_per_family_accuracy_int8\": {},\n  \"quant_error_eval\": {{\"max\": {qmax}, \"within_2\": {qwithin:.4}, \"mean\": {qmean:.4}}},\n  \"quant_error_train\": {{\"max\": {tmax}, \"within_2\": {twithin:.4}}},\n  \"cartridge_equals_integer_reference\": {}, \"tick_calls_per_job_max\": {calls_max},\n  \"scales\": {{\"s1\": [{}], \"s2\": {:e}, \"table_bin_logit\": {:e}, \"so\": {}}},\n{}\n}}\n",
        q.model_id,
        data_bytes.len(),
        FEATURES.iter().map(|f| format!("\"{f}\"")).collect::<Vec<_>>().join(", "),
        SH_IN.iter().map(|s| s.to_string()).collect::<Vec<_>>().join(", "),
        train.len(),
        eval.len(),
        cfg.seed,
        cfg.epochs,
        cfg.lr,
        cfg.batch,
        cfg.l2,
        mjson("train_float", &ftr),
        mjson("train_int8", &qtr),
        mjson("eval_float", &fev),
        mjson("eval_int8", &qev),
        mjson("eval_unseen_int8", &qun),
        per_family(&eval),
        eval.len(),
        sc.s1.iter().map(|x| format!("{x:e}")).collect::<Vec<_>>().join(", "),
        sc.s2,
        sc.bin,
        q.so,
        model.json_fields(),
    );
    let jpath = need(&m, "json-out")?;
    std::fs::write(jpath, &json).map_err(|e| format!("{jpath}: {e}"))?;
    println!(
        "model {:#010x}: DATA {} bytes; train acc float {:.4} int8 {:.4}; eval acc float {:.4} int8 {:.4} (tpr {:.4} fpr {:.4} auc {:.4}); quant error eval max {qmax} within2 {:.2}% mean {qmean:.3}; cartridge == reference on {} eval vectors ({calls_max} tick calls max)",
        q.model_id,
        data_bytes.len(),
        ftr.accuracy,
        qtr.accuracy,
        fev.accuracy,
        qev.accuracy,
        qev.tpr,
        qev.fpr,
        qev.auc,
        100.0 * qwithin,
        eval.len()
    );
    println!(
        "eval vectors not in the train set: {} of {}, int8 accuracy {:.4} (tpr {:.4} fpr {:.4})",
        qun.n,
        eval.len(),
        qun.accuracy,
        qun.tpr,
        qun.fpr
    );
    Ok(())
}

/// Negative control: the same training on the train set with its labels
/// shuffled. A pipeline that leaks labels (or a test set that overlaps the
/// training set) would still score well on eval; a sound one falls to chance.
fn control(args: &[String]) -> Result<(), String> {
    let m = flags(args)?;
    let mut train = read_csv(need(&m, "train")?)?;
    let eval = read_csv(need(&m, "eval")?)?;
    let mut rng = upc_cartridge::scorer::Rng(0x0C04_7501);
    let mut labels: Vec<u8> = train.iter().map(|s| s.label).collect();
    for i in (1..labels.len()).rev() {
        labels.swap(i, rng.below(i as u64 + 1) as usize);
    }
    for (s, l) in train.iter_mut().zip(labels) {
        s.label = l;
    }
    let model = fit(&train, FitCfg::default());
    let ev = metrics(&eval, |f| model.prob(f) * 100.0);
    println!(
        "control (shuffled train labels): eval accuracy {:.4} tpr {:.4} fpr {:.4} auc {:.4} over {} vectors",
        ev.accuracy, ev.tpr, ev.fpr, ev.auc, ev.n
    );
    Ok(())
}

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let res = match args.first().map(String::as_str) {
        Some("gen") => gen(&args[1..]),
        Some("fit") => fit_cmd(&args[1..]),
        Some("control") => control(&args[1..]),
        _ => return usage(),
    };
    match res {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("trainquant: {e}");
            ExitCode::from(1)
        }
    }
}
