// SPDX-License-Identifier: GPL-3.0-only
//! Tail call chain management for the Doom-over-IPv6 BPF execution pipeline.
//!
//! # The Problem
//!
//! The BPF verifier limits each program to `MAX_INSN_PER_TICK` MBC instructions
//! (currently 16) due to verification complexity. At 35 Hz tick rate, that is
//! only 560 MBC instructions/second — far too slow for Doom.
//!
//! # The Solution: Tail Call Chaining
//!
//! Load N copies of `monad_cpu` into a `BPF_MAP_TYPE_PROG_ARRAY`. Each XDP
//! invocation executes 16 MBC instructions, then tail-calls the next program
//! in the chain. The last program writes state back and returns.
//!
//! ```text
//! Packet arrives (XDP)
//!   → prog[0]: execute 16 insns, tail_call(prog[1])
//!   → prog[1]: execute 16 insns, tail_call(prog[2])
//!   → ...
//!   → prog[N-1]: execute 16 insns, write state, return XDP_DROP
//! ```
//!
//! Total: `16 * N` MBC instructions per tick.
//!
//! # Kernel Limits
//!
//! - Max tail call depth: 33 (kernel 6.x)
//! - At N=16: 256 insns/tick (conservative, matches previous working config)
//! - At N=33: 528 insns/tick (maximum achievable)
//! - Combined with tick rate (2000+ Hz): 256 * 2000 = 512K insns/sec
//!
//! # Implementation Notes
//!
//! The eBPF program (`monad-cpu-ebpf`) needs modification to:
//! 1. Read a "round counter" from a per-CPU map
//! 2. Increment and tail_call the next program in the array
//! 3. On the last round (or if halted), write state and return
//!
//! This module handles the USERSPACE side: loading the program array,
//! configuring the chain depth, and monitoring execution stats.

use anyhow::{Context, Result};
use tracing::info;

/// Default tail call chain depth.
/// 16 programs * 16 insns/program = 256 insns/tick.
/// At 2000 Hz injector: 512K insns/sec.
pub const DEFAULT_CHAIN_DEPTH: usize = 16;

/// Maximum tail call depth allowed by the kernel (6.x).
pub const MAX_CHAIN_DEPTH: usize = 33;

/// MBC instructions executed per XDP program invocation.
/// Must match MAX_INSN_PER_TICK in monad-cpu-ebpf.
pub const INSNS_PER_PROGRAM: usize = 16;

/// Configuration for the tail call chain.
#[derive(Debug, Clone)]
pub struct TailCallConfig {
    /// Number of programs in the chain (1..=MAX_CHAIN_DEPTH).
    pub chain_depth: usize,

    /// Path to the compiled monad-cpu-ebpf ELF object.
    pub ebpf_obj_path: String,

    /// BPF pin path for the program array map.
    pub prog_array_pin: String,
}

impl TailCallConfig {
    /// Create a new config with the default chain depth.
    pub fn new(ebpf_obj_path: String) -> Self {
        Self {
            chain_depth: DEFAULT_CHAIN_DEPTH,
            ebpf_obj_path,
            prog_array_pin: "/sys/fs/bpf/unheaded/doom-ring/prog_array".to_string(),
        }
    }

    /// Total MBC instructions executed per tick.
    pub fn insns_per_tick(&self) -> usize {
        self.chain_depth * INSNS_PER_PROGRAM
    }

    /// Estimated throughput at a given tick rate (Hz).
    pub fn throughput_at(&self, tick_rate_hz: u32) -> u64 {
        self.insns_per_tick() as u64 * tick_rate_hz as u64
    }

    /// Validate the configuration.
    pub fn validate(&self) -> Result<()> {
        if self.chain_depth == 0 || self.chain_depth > MAX_CHAIN_DEPTH {
            anyhow::bail!(
                "chain_depth must be 1..={MAX_CHAIN_DEPTH}, got {}",
                self.chain_depth,
            );
        }
        Ok(())
    }
}

/// Plan for loading and attaching the tail call chain.
///
/// This is computed upfront so we can log what we intend to do before
/// actually loading anything.
#[derive(Debug)]
pub struct TailCallPlan {
    pub config: TailCallConfig,
    pub total_insns_per_tick: usize,
    pub estimated_throughput_2khz: u64,
}

/// Create an execution plan for the tail call chain.
pub fn plan(config: TailCallConfig) -> Result<TailCallPlan> {
    config.validate().context("invalid tail call config")?;

    let total = config.insns_per_tick();
    let throughput = config.throughput_at(2000);

    info!(
        "tail call plan: {} programs x {} insns = {} insns/tick, \
         ~{} insns/sec at 2 kHz",
        config.chain_depth, INSNS_PER_PROGRAM, total, throughput,
    );

    Ok(TailCallPlan {
        config,
        total_insns_per_tick: total,
        estimated_throughput_2khz: throughput,
    })
}

/// Load the tail call chain into BPF.
///
/// This is a placeholder — the actual implementation requires:
/// 1. Loading the monad_cpu program N times via Aya
/// 2. Inserting each loaded program FD into a BPF_MAP_TYPE_PROG_ARRAY
/// 3. Setting the "chain_depth" value in a config map the eBPF program reads
///
/// The eBPF side (monad-cpu-ebpf) must be modified to support tail calls
/// before this function does anything useful.
pub async fn load_chain(_plan: &TailCallPlan) -> Result<()> {
    // TODO: Implement once monad-cpu-ebpf has tail call support.
    //
    // Rough pseudocode:
    //
    // let mut bpf = aya::Ebpf::load_file(&plan.config.ebpf_obj_path)?;
    // let prog_array: ProgramArray<_, _> = bpf.take_map("TAIL_CALL_PROGS")?.try_into()?;
    //
    // for i in 0..plan.config.chain_depth {
    //     let prog: &mut Xdp = bpf.program_mut(&format!("monad_cpu_{i}"))?.try_into()?;
    //     prog.load()?;
    //     prog_array.set(i as u32, prog, 0)?;
    // }
    //
    // // Set chain_depth in the config map
    // let config_map: Array<_, u32> = bpf.take_map("CHAIN_CONFIG")?.try_into()?;
    // config_map.set(0, plan.config.chain_depth as u32, 0)?;

    info!(
        "tail call chain loading not yet implemented \
         (requires monad-cpu-ebpf tail call support)"
    );
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_config_is_valid() {
        let cfg = TailCallConfig::new("/dev/null".into());
        cfg.validate().unwrap();
    }

    #[test]
    fn insns_per_tick_calculation() {
        let cfg = TailCallConfig::new("/dev/null".into());
        assert_eq!(cfg.insns_per_tick(), 256); // 16 * 16
    }

    #[test]
    fn throughput_at_2khz() {
        let cfg = TailCallConfig::new("/dev/null".into());
        assert_eq!(cfg.throughput_at(2000), 512_000);
    }

    #[test]
    fn max_depth_throughput() {
        let mut cfg = TailCallConfig::new("/dev/null".into());
        cfg.chain_depth = MAX_CHAIN_DEPTH;
        cfg.validate().unwrap();
        // 33 * 16 = 528 insns/tick * 2000 Hz = 1_056_000 insns/sec
        assert_eq!(cfg.throughput_at(2000), 1_056_000);
    }

    #[test]
    fn zero_depth_is_invalid() {
        let mut cfg = TailCallConfig::new("/dev/null".into());
        cfg.chain_depth = 0;
        assert!(cfg.validate().is_err());
    }

    #[test]
    fn over_max_depth_is_invalid() {
        let mut cfg = TailCallConfig::new("/dev/null".into());
        cfg.chain_depth = MAX_CHAIN_DEPTH + 1;
        assert!(cfg.validate().is_err());
    }
}
