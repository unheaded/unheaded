// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Token-bucket refill math for `qos-ebpf` — pure, off-target testable.
//!
//! `qos-ebpf` keeps one [`TokenBucket`]-shaped record per flow in an LRU map
//! (`tokens` in bytes, `last_refill_ns` from `bpf_ktime_get_ns`). This module
//! owns the refill step only, so it can be unit-tested on the host: the bin
//! crate's own `cargo test` is poisoned by `ebpf/.cargo/config.toml`'s
//! `build-std` (same reason as [`crate::fdtable`]).
//!
//! ## Units
//!
//! `rate_mbps` megabits per second is `rate_mbps / 8000` bytes per
//! nanosecond, so the bucket earns exactly `rate_mbps` bytes per
//! [`NS_PER_QUANTUM`] (8,000 ns). Refill works in whole quanta to stay in u64
//! with no multiply-before-divide overflow and no 128-bit intermediate (BPF has
//! no 128-bit multiply).

/// Nanoseconds per refill quantum: in 8,000 ns a `rate_mbps` bucket earns
/// exactly `rate_mbps` bytes (`rate_mbps * 10^6 bit/s / 8 bit/B / 10^9 ns/s`).
pub const NS_PER_QUANTUM: u64 = 8_000;

/// Result of one [`refill`] step.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Refill {
    /// Tokens (bytes) after the refill, never above the capacity it was given.
    pub tokens: u64,
    /// The new `last_refill_ns` to store.
    pub last_refill_ns: u64,
    /// True when tokens were credited (drives `STAT_BUCKET_REFILLED`).
    pub refilled: bool,
}

/// Refill a bucket for the time between `last_refill_ns` and `now`.
///
/// Only whole quanta are credited, and `last_refill_ns` advances by exactly
/// the time that was converted to tokens (`q * NS_PER_QUANTUM`), so the
/// sub-quantum remainder of `elapsed` carries into the next refill. Until
/// 2026-10-23 `last_refill_ns` jumped to `now`, discarding `elapsed mod 8000`
/// at every refill: at 20 µs packet spacing a 100 Mbps bucket admitted
/// 80 Mbps.
///
/// When the credit reaches `capacity` the bucket is full, the excess
/// (remainder included) is dropped and `last_refill_ns = now`: time spent full
/// is not credit for later.
///
/// Total over all inputs, no loops, no 128-bit math: `q > capacity / rate_mbps` is
/// decided before `q * rate_mbps` is formed, so the product never exceeds
/// `capacity`. `rate_mbps == 0` credits nothing (the caller skips the limiter
/// in that case). A clock behind `last_refill_ns` credits nothing.
#[inline(always)]
pub fn refill(tokens: u64, last_refill_ns: u64, now: u64, rate_mbps: u64, capacity: u64) -> Refill {
    let unchanged = Refill {
        tokens,
        last_refill_ns,
        refilled: false,
    };
    if rate_mbps == 0 {
        return unchanged;
    }
    let q = now.saturating_sub(last_refill_ns) / NS_PER_QUANTUM;
    if q == 0 {
        return unchanged;
    }
    let full = Refill {
        tokens: capacity,
        last_refill_ns: now,
        refilled: true,
    };
    if q > capacity / rate_mbps {
        // q * rate_mbps > capacity: full however empty it was.
        return full;
    }
    let add = q * rate_mbps; // <= capacity, cannot overflow
    if tokens >= capacity || add >= capacity - tokens {
        return full;
    }
    Refill {
        tokens: tokens + add,
        last_refill_ns: last_refill_ns + q * NS_PER_QUANTUM, // <= now
        refilled: true,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Admitted fraction of the configured rate: at least 99 %, never over.
    const GOOD: core::ops::RangeInclusive<f64> = 0.99..=1.0 + 1e-9;

    /// Drive `qos-ebpf`'s per-flow control flow (first packet creates the
    /// bucket at `cap - len`, later packets refill, then pass if
    /// `tokens >= len` and consume) over `n` packets of `len` bytes at a fixed
    /// `spacing_ns`. Returns the admitted rate as a fraction of `rate_mbps`,
    /// with the initial burst (`cap`) taken out so only refill is measured.
    fn admitted_fraction(rate_mbps: u64, cap: u64, len: u64, spacing_ns: u64, n: u64) -> f64 {
        let t0 = 1_000_000_000u64; // arbitrary non-zero clock origin
        let mut tokens = cap.saturating_sub(len);
        let mut last = t0;
        let mut admitted = len; // first packet always passes
        for i in 1..n {
            let now = t0 + i * spacing_ns;
            let r = refill(tokens, last, now, rate_mbps, cap);
            tokens = r.tokens;
            last = r.last_refill_ns;
            if tokens >= len {
                tokens -= len;
                admitted += len;
            }
        }
        let duration_ns = (n - 1) * spacing_ns;
        let refill_bytes = admitted.saturating_sub(cap) as f64;
        let configured_bytes = rate_mbps as f64 * duration_ns as f64 / NS_PER_QUANTUM as f64;
        refill_bytes / configured_bytes
    }

    /// The S5 finding: 100 Mbps, 20 µs spacing, 1,500-byte packets (600 Mbps
    /// offered, so the limiter binds). Before the fix only ~80 % of the
    /// configured rate got through: every refill threw away the 4,000 ns of
    /// `20,000 mod 8,000`.
    #[test]
    fn refill_at_20us_spacing_admits_the_configured_rate() {
        let f = admitted_fraction(100, 12_500, 1_500, 20_000, 100_000);
        eprintln!("100 Mbps @ 20 us spacing: admitted {:.4} of configured", f);
        assert!(f >= 0.99, "admitted {f:.4} of 100 Mbps, want >= 0.99");
        assert!(f <= 1.0 + 1e-9, "admitted {f:.4}: over the configured rate");
    }

    /// No spacing from 1 µs to 100 µs (offered load always above the limit)
    /// loses refill time, and none admits more than configured.
    #[test]
    fn refill_rate_is_exact_across_spacings() {
        for spacing in [
            1_000u64, 3_000, 7_999, 8_000, 8_001, 13_000, 20_000, 33_333, 100_003,
        ] {
            // Enough packets for >= 2 s of simulated time, so the burst is noise.
            let n = 2_000_000_000 / spacing + 2;
            let f = admitted_fraction(100, 12_500, 1_500, spacing, n);
            assert!(GOOD.contains(&f), "spacing {spacing} ns: admitted {f:.4}");
        }
    }

    /// Low rates: 1 Mbps earns 1 byte per 8 µs. Sub-quantum spacing must
    /// still accumulate (it used to: no credit means `last` is kept).
    #[test]
    fn refill_low_rate_sub_quantum_spacing() {
        let f = admitted_fraction(1, 1_500, 64, 5_000, 1_000_000);
        assert!(GOOD.contains(&f), "admitted {f:.4}");
    }

    #[test]
    fn refill_less_than_one_quantum_credits_nothing_and_keeps_last() {
        let r = refill(10, 1_000, 1_000 + NS_PER_QUANTUM - 1, 100, 12_500);
        assert_eq!(
            r,
            Refill {
                tokens: 10,
                last_refill_ns: 1_000,
                refilled: false
            }
        );
    }

    #[test]
    fn refill_carries_the_sub_quantum_remainder() {
        // 20,000 ns = 2 quanta + 4,000 ns. Two quanta are credited; `last`
        // advances by 16,000 ns, not to `now`, so the 4,000 ns count next time.
        let r = refill(0, 1_000, 21_000, 100, 12_500);
        assert_eq!(
            r,
            Refill {
                tokens: 200,
                last_refill_ns: 17_000,
                refilled: true
            }
        );
        let r2 = refill(r.tokens, r.last_refill_ns, 41_000, 100, 12_500);
        // 24,000 ns since 17,000 = 3 quanta exactly.
        assert_eq!(
            r2,
            Refill {
                tokens: 500,
                last_refill_ns: 41_000,
                refilled: true
            }
        );
    }

    #[test]
    fn refill_caps_at_capacity_and_resets_last_to_now() {
        // A full bucket earns nothing more; the remainder is dropped with the
        // overflow (time spent full is not credit for later).
        let r = refill(12_400, 0, 20_000, 100, 12_500);
        assert_eq!(
            r,
            Refill {
                tokens: 12_500,
                last_refill_ns: 20_000,
                refilled: true
            }
        );
    }

    #[test]
    fn refill_long_idle_fills_the_bucket() {
        let r = refill(0, 0, 3_600 * 1_000_000_000, 100, 12_500);
        assert_eq!(
            r,
            Refill {
                tokens: 12_500,
                last_refill_ns: 3_600 * 1_000_000_000,
                refilled: true
            }
        );
    }

    /// Hostile map/config values must not wrap into a small credit.
    #[test]
    fn refill_hostile_values_do_not_wrap() {
        // elapsed / 8000 * rate overflows u64 here; the bucket must be full,
        // not refilled by the wrapped product.
        let r = refill(0, 0, u64::MAX, u64::MAX / 3, 1_000);
        assert_eq!(r.tokens, 1_000);
        assert_eq!(r.last_refill_ns, u64::MAX);
        // tokens already above capacity (corrupt map value): clamped, no wrap.
        let r = refill(u64::MAX, 0, 80_000, 100, 12_500);
        assert_eq!(r.tokens, 12_500);
    }

    #[test]
    fn refill_clock_behind_last_credits_nothing() {
        let r = refill(7, 50_000, 10_000, 100, 12_500);
        assert_eq!(
            r,
            Refill {
                tokens: 7,
                last_refill_ns: 50_000,
                refilled: false
            }
        );
    }

    #[test]
    fn refill_zero_rate_credits_nothing() {
        let r = refill(7, 0, 1_000_000, 0, 12_500);
        assert_eq!(
            r,
            Refill {
                tokens: 7,
                last_refill_ns: 0,
                refilled: false
            }
        );
    }
}
