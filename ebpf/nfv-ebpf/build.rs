// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

//! Emit BTF for this package only. Tail calls with subprograms need it
//! (kernel 6.17+). It is not workspace-wide, because BTF turns
//! compiler_builtins' memset into a global function the verifier rejects in
//! programs that call it. See ebpf/.cargo/config.toml.

fn main() {
    println!("cargo:rustc-link-arg=--btf");
}
