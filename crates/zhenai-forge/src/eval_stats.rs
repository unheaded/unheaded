// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Small in-tree statistical helpers for the WAVE10F Learning Gate suite.
//!
//! Intentionally dependency-free: uses only Rust std. Everything is
//! deterministic given the same seed so tests don't flap across runs.

/// Deterministic xorshift64* PRNG. Period 2^64 − 1, passes most of
/// BigCrush, and — crucially — its low bits don't cycle with short
/// period like a plain LCG's do (which breaks `r % small_n` badly).
pub struct Lcg {
    state: u64,
}

impl Lcg {
    pub fn new(seed: u64) -> Self {
        // Avoid degenerate zero-state by mixing with a SplitMix64 constant.
        let s = seed.wrapping_add(0x9E3779B97F4A7C15);
        Self { state: if s == 0 { 0x9E3779B97F4A7C15 } else { s } }
    }
    pub fn next_u64(&mut self) -> u64 {
        let mut x = self.state;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.state = x;
        x.wrapping_mul(0x2545F4914F6CDD1D)
    }
    /// Uniform [0, n). Unbiased rejection sampling on the full 64-bit value.
    pub fn next_range(&mut self, n: u64) -> u64 {
        if n == 0 { return 0; }
        let limit = u64::MAX - (u64::MAX % n);
        loop {
            let r = self.next_u64();
            if r < limit { return r % n; }
        }
    }
    /// Uniform f32 in [0, 1).
    pub fn next_f32_unit(&mut self) -> f32 {
        (self.next_u64() >> 40) as f32 / (1u32 << 24) as f32
    }
}

/// Percentile-method bootstrap 95% CI for the mean of a sample.
/// Returns `(lo, hi)`. Deterministic given the seed. Panics on empty input.
pub fn bootstrap_ci_95(samples: &[f32], n_resamples: usize, seed: u64) -> (f32, f32) {
    assert!(!samples.is_empty(), "bootstrap_ci_95 requires non-empty sample");
    let n = samples.len();
    let mut rng = Lcg::new(seed);
    let mut means: Vec<f32> = Vec::with_capacity(n_resamples);
    for _ in 0..n_resamples {
        let mut sum = 0.0f64;
        for _ in 0..n {
            let idx = rng.next_range(n as u64) as usize;
            sum += samples[idx] as f64;
        }
        means.push((sum / n as f64) as f32);
    }
    means.sort_by(|a, b| a.partial_cmp(b).unwrap());
    let lo_idx = ((n_resamples as f32) * 0.025) as usize;
    let hi_idx = ((n_resamples as f32) * 0.975) as usize;
    let hi_idx = hi_idx.min(n_resamples - 1);
    (means[lo_idx], means[hi_idx])
}

/// Percentile-method bootstrap 95% CI on the RATIO of two correlated sample
/// means (e.g. eval_loss[final] / eval_loss[t=0]). Resamples pair-wise so the
/// same bootstrap indices drive both numerator and denominator — this
/// captures correlation between pre/post measurements on the same sequences.
pub fn bootstrap_ratio_ci_95(
    pre: &[f32], post: &[f32], n_resamples: usize, seed: u64,
) -> (f32, f32) {
    assert_eq!(pre.len(), post.len(), "pre/post must pair 1:1 per sequence");
    assert!(!pre.is_empty(), "bootstrap_ratio_ci_95 requires non-empty sample");
    let n = pre.len();
    let mut rng = Lcg::new(seed);
    let mut ratios: Vec<f32> = Vec::with_capacity(n_resamples);
    for _ in 0..n_resamples {
        let mut num = 0.0f64;
        let mut den = 0.0f64;
        for _ in 0..n {
            let i = rng.next_range(n as u64) as usize;
            num += post[i] as f64;
            den += pre[i] as f64;
        }
        let r = if den > 0.0 { (num / den) as f32 } else { f32::NAN };
        if r.is_finite() { ratios.push(r); }
    }
    assert!(!ratios.is_empty(), "all bootstrap samples non-finite");
    ratios.sort_by(|a, b| a.partial_cmp(b).unwrap());
    let lo_idx = ((ratios.len() as f32) * 0.025) as usize;
    let hi_idx = (((ratios.len() as f32) * 0.975) as usize).min(ratios.len() - 1);
    (ratios[lo_idx], ratios[hi_idx])
}

/// Ordinary least squares linear fit `y = slope * x + intercept`.
/// Returns (slope, intercept, slope_standard_error).
/// Panics if `xs.len() != ys.len()` or fewer than 2 points.
pub fn linear_fit(xs: &[f64], ys: &[f64]) -> (f64, f64, f64) {
    assert_eq!(xs.len(), ys.len());
    assert!(xs.len() >= 2, "linear_fit requires ≥2 points");
    let n = xs.len() as f64;
    let mean_x = xs.iter().sum::<f64>() / n;
    let mean_y = ys.iter().sum::<f64>() / n;
    let mut sxx = 0.0;
    let mut sxy = 0.0;
    for i in 0..xs.len() {
        let dx = xs[i] - mean_x;
        sxx += dx * dx;
        sxy += dx * (ys[i] - mean_y);
    }
    let slope = if sxx > 0.0 { sxy / sxx } else { 0.0 };
    let intercept = mean_y - slope * mean_x;
    // Residual sum of squares → standard error of slope.
    let mut ss_res = 0.0;
    for i in 0..xs.len() {
        let y_hat = slope * xs[i] + intercept;
        let r = ys[i] - y_hat;
        ss_res += r * r;
    }
    let dof = (n - 2.0).max(1.0);
    let se = if sxx > 0.0 { (ss_res / dof / sxx).sqrt() } else { 0.0 };
    (slope, intercept, se)
}

/// Fit `gap(t) = α · t^β` via log-log OLS and return `(β, β_95ci_lo, β_95ci_hi)`.
/// 95% CI via normal approximation (β ± 1.96 · SE); fine for the Gate's
/// handful of checkpoints — no distributional claims beyond "is β clearly
/// below threshold."
pub fn fit_power_law_beta(ts: &[usize], gaps: &[f32]) -> (f64, f64, f64) {
    assert_eq!(ts.len(), gaps.len());
    assert!(ts.len() >= 2, "fit_power_law_beta requires ≥2 points");
    let xs: Vec<f64> = ts.iter().map(|&t| (t as f64).ln()).collect();
    let ys: Vec<f64> = gaps.iter()
        .map(|&g| (g.max(1e-9) as f64).ln())
        .collect();
    let (slope, _intercept, se) = linear_fit(&xs, &ys);
    let lo = slope - 1.96 * se;
    let hi = slope + 1.96 * se;
    (slope, lo, hi)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_lcg_deterministic() {
        let mut a = Lcg::new(42);
        let mut b = Lcg::new(42);
        for _ in 0..100 {
            assert_eq!(a.next_u64(), b.next_u64());
        }
    }

    #[test]
    fn test_lcg_range_unbiased() {
        let mut rng = Lcg::new(7);
        let mut counts = [0usize; 5];
        for _ in 0..50_000 {
            let v = rng.next_range(5) as usize;
            counts[v] += 1;
        }
        // Each bucket should be within 5% of 10_000.
        for c in counts.iter() {
            assert!(*c >= 9_000 && *c <= 11_000,
                "LCG bucket imbalance: {:?}", counts);
        }
    }

    #[test]
    fn test_bootstrap_ci_covers_mean() {
        // Sample from a near-uniform distribution on [0, 1]. 95% CI on the
        // mean should be narrow around 0.5 for 1000 samples.
        let mut rng = Lcg::new(123);
        let xs: Vec<f32> = (0..1000).map(|_| rng.next_f32_unit()).collect();
        let (lo, hi) = bootstrap_ci_95(&xs, 1000, 99);
        assert!(lo > 0.45 && lo < 0.55, "lo={}", lo);
        assert!(hi > 0.45 && hi < 0.55, "hi={}", hi);
        assert!(lo <= hi);
    }

    #[test]
    fn test_bootstrap_ratio_excludes_one_for_clear_drop() {
        let pre = vec![1.0f32; 32];
        let post = vec![0.5f32; 32];
        let (lo, hi) = bootstrap_ratio_ci_95(&pre, &post, 1000, 42);
        assert!(hi < 1.0, "CI should exclude 1.0: ({}, {})", lo, hi);
        assert!((lo - 0.5).abs() < 1e-5, "lo≈0.5, got {}", lo);
    }

    #[test]
    fn test_bootstrap_ratio_spans_one_for_no_change() {
        // Paired setup with symmetric zero-mean additive noise on post.
        // Population ratio = 1.0; bootstrap CI should bracket it at N=512.
        let mut rng = Lcg::new(9);
        let pre: Vec<f32> = (0..512)
            .map(|_| 1.0 + (rng.next_f32_unit() - 0.5) * 0.2)
            .collect();
        let post: Vec<f32> = pre
            .iter()
            .map(|&v| v + (rng.next_f32_unit() - 0.5) * 0.4)
            .collect();
        let (lo, hi) = bootstrap_ratio_ci_95(&pre, &post, 2000, 77);
        assert!(lo < 1.0 && hi > 1.0,
            "CI should span 1.0 for paired no-effect: ({}, {})", lo, hi);
    }

    #[test]
    fn test_bootstrap_ratio_ci_width_narrows_with_n() {
        // Sanity: more samples → tighter CI. Independent pre/post draws from
        // the same distribution (ratio-1.0 population, finite-sample noise).
        let gen_independent = |n: usize, rng: &mut Lcg| -> (Vec<f32>, Vec<f32>) {
            let pre: Vec<f32> = (0..n).map(|_| 0.5 + rng.next_f32_unit()).collect();
            let post: Vec<f32> = (0..n).map(|_| 0.5 + rng.next_f32_unit()).collect();
            (pre, post)
        };
        let mut rng_s = Lcg::new(5);
        let mut rng_l = Lcg::new(6);
        let (pre_s, post_s) = gen_independent(16, &mut rng_s);
        let (pre_l, post_l) = gen_independent(512, &mut rng_l);
        let (s_lo, s_hi) = bootstrap_ratio_ci_95(&pre_s, &post_s, 2000, 3);
        let (l_lo, l_hi) = bootstrap_ratio_ci_95(&pre_l, &post_l, 2000, 3);
        assert!(l_hi - l_lo < s_hi - s_lo,
            "larger N should narrow CI: small=({:.3},{:.3}) w={:.3}, large=({:.3},{:.3}) w={:.3}",
            s_lo, s_hi, s_hi - s_lo, l_lo, l_hi, l_hi - l_lo);
    }

    #[test]
    fn test_linear_fit_perfect_line() {
        let xs: Vec<f64> = (0..10).map(|i| i as f64).collect();
        let ys: Vec<f64> = xs.iter().map(|x| 3.0 * x + 7.0).collect();
        let (slope, intercept, se) = linear_fit(&xs, &ys);
        assert!((slope - 3.0).abs() < 1e-9);
        assert!((intercept - 7.0).abs() < 1e-9);
        assert!(se < 1e-9);
    }

    #[test]
    fn test_power_law_beta_recovery() {
        // gap(t) = 1.0 * t^0.5 — β should come back ≈ 0.5.
        let ts: Vec<usize> = vec![1, 3, 10, 30, 100];
        let gaps: Vec<f32> = ts.iter().map(|&t| (t as f32).powf(0.5)).collect();
        let (beta, lo, hi) = fit_power_law_beta(&ts, &gaps);
        assert!((beta - 0.5).abs() < 1e-6, "β={}", beta);
        assert!(lo <= beta && beta <= hi);
    }

    #[test]
    fn test_power_law_beta_memorization_signal() {
        // Pure memorization: gap grows as fast as train loss drops, i.e.
        // gap(t) ≈ constant * t → β ≈ 1.0.
        let ts: Vec<usize> = vec![1, 3, 10, 30, 100];
        let gaps: Vec<f32> = ts.iter().map(|&t| 2.0 * t as f32).collect();
        let (beta, _lo, _hi) = fit_power_law_beta(&ts, &gaps);
        assert!((beta - 1.0).abs() < 1e-6, "β≈1.0 expected, got {}", beta);
    }
}
