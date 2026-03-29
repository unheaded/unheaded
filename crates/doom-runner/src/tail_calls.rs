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

/// Set up the tail call chain in an already-loaded eBPF object.
///
/// The monad-cpu-ebpf program uses self-tail-calling: it calls itself
/// via TAIL_CALL_PROGS[0] up to MAX_TAIL_CALLS times per packet.
/// Each invocation gets a fresh BPF verifier budget (16 MBC instructions).
///
/// This function populates TAIL_CALL_PROGS[0] with monad_cpu's own FD.
/// Call this AFTER loading and calling `program.load()` on monad_cpu.
///
/// Returns the actual chain depth configured.
pub fn setup_tail_calls(
    ebpf: &mut aya::Ebpf,
    plan: &TailCallPlan,
) -> Result<usize> {
    use aya::maps::ProgramArray;
    use aya::programs::Xdp;

    plan.config.validate()?;

    // Clone the program FD first (immutable borrow), then drop it before
    // taking the mutable borrow for the prog array map.
    let prog_fd = {
        let prog: &Xdp = ebpf
            .program("monad_cpu")
            .ok_or_else(|| anyhow::anyhow!("monad_cpu program not found"))?
            .try_into()
            .map_err(|e| anyhow::anyhow!("monad_cpu is not XDP: {e}"))?;

        prog.fd()
            .map_err(|e| anyhow::anyhow!("monad_cpu has no FD: {e}"))?
            .try_clone()
            .map_err(|e| anyhow::anyhow!("failed to clone monad_cpu FD: {e}"))?
    }; // immutable borrow of `ebpf` dropped here

    // Populate TAIL_CALL_PROGS[0] = monad_cpu (self-reference)
    let mut prog_array = ProgramArray::try_from(
        ebpf.map_mut("TAIL_CALL_PROGS")
            .ok_or_else(|| anyhow::anyhow!("TAIL_CALL_PROGS map not found"))?,
    )
    .map_err(|e| anyhow::anyhow!("TAIL_CALL_PROGS is not a ProgramArray: {e}"))?;

    prog_array
        .set(0, &prog_fd, 0)
        .map_err(|e| anyhow::anyhow!("failed to set TAIL_CALL_PROGS[0]: {e}"))?;

    info!(
        "tail call chain active: {} rounds x {} insns = {} insns/tick, \
         ~{} insns/sec at 2 kHz",
        plan.config.chain_depth,
        INSNS_PER_PROGRAM,
        plan.total_insns_per_tick,
        plan.estimated_throughput_2khz,
    );

    Ok(plan.config.chain_depth)
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
