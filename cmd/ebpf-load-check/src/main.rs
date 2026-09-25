// SPDX-License-Identifier: GPL-2.0-only
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! Load every program in an eBPF ELF through the kernel verifier, and never
//! attach one. Used by scripts/check-ebpf-loads.sh.
//!
//! Loading is the only check that counts. The verifier runs at load time, so
//! a program can build, pass static analysis and still be rejected by every
//! kernel it meets; 16 of 31 programs in this tree were, unnoticed, until
//! something loaded them (2026-09-25).
//!
//! aya, not cilium/ebpf, because aya is what loads these in production, and
//! the two differ (aya rewrites BTF linkage for memset and friends).
//!
//! Output: one line per program, `<prog> OK` or `<prog> FAIL <reason>`. A
//! program type this tool cannot load is a FAIL, not a skip: an unhandled type
//! must not pass by default.
//!
//! Exit: 0 if the ELF parsed (per-program verdicts are on stdout), 2 if it did
//! not. The gate, not this tool, decides which failures are allowed.

use aya::{programs::Program, Ebpf};

fn main() {
    let mut args = std::env::args().skip(1);
    let (Some(path), None) = (args.next(), args.next()) else {
        eprintln!("usage: ebpf-load-check <elf>");
        std::process::exit(2);
    };
    let mut ebpf = match Ebpf::load_file(&path) {
        Ok(e) => e,
        Err(e) => {
            eprintln!("{path}: cannot parse: {e}");
            std::process::exit(2);
        }
    };
    let mut n = 0;
    for (name, prog) in ebpf.programs_mut() {
        n += 1;
        let result = match prog {
            Program::Xdp(p) => p.load(),
            Program::SchedClassifier(p) => p.load(),
            Program::KProbe(p) => p.load(),
            Program::TracePoint(p) => p.load(),
            Program::RawTracePoint(p) => p.load(),
            Program::SocketFilter(p) => p.load(),
            other => {
                println!(
                    "{name} FAIL unsupported program type {:?}",
                    other.prog_type()
                );
                continue;
            }
        };
        match result {
            Ok(()) => println!("{name} OK"),
            Err(e) => println!("{name} FAIL {}", rejection(&e.to_string())),
        }
    }
    if n == 0 {
        eprintln!("{path}: no programs");
        std::process::exit(2);
    }
}

/// The line that says why the verifier rejected the program. The log starts
/// with the program's first instruction and ends with statistics, so the
/// reason is the last line before the statistics.
fn rejection(err: &str) -> &str {
    const STATS: [&str; 4] = [
        "processed ",
        "verification time",
        "stack depth",
        "max_states",
    ];
    err.lines()
        .map(str::trim)
        .rev()
        .find(|l| !l.is_empty() && !STATS.iter().any(|s| l.starts_with(s)))
        .unwrap_or(err)
}
