// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

//! Emit BTF for this package only: the interpreter runs its step as a
//! bpf_loop callback, and the verifier refuses a callback without BTF
//! func_info. Same per-package choice as monad-cpu-ebpf (see
//! ebpf/.cargo/config.toml for why --btf is not workspace-wide).

fn main() {
    println!("cargo:rustc-link-arg=--btf");
}
