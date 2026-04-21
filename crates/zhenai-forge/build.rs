// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
//
// zhenai-forge build.rs
//
// Handles:
//   1. Link against ROCm/HIP runtime + hipBLAS (pre-existing).
//   2. WAVE11: compile kernels/*.hip.cpp → libwave11_kernels.so via hipcc
//      and link it statically at crate build time. No runtime dlopen needed.

use std::env;
use std::path::PathBuf;
use std::process::Command;

fn main() {
    // --- ROCm/HIP runtime + BLAS (long-standing) ---
    println!("cargo:rustc-link-search=native=/opt/rocm/lib");
    println!("cargo:rustc-link-lib=dylib=amdhip64");
    println!("cargo:rustc-link-lib=dylib=hipblas");
    println!("cargo:rustc-link-arg=-Wl,-rpath,/opt/rocm/lib");

    // --- WAVE11 kernels (new) ---
    build_wave11_kernels();
}

fn build_wave11_kernels() {
    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let kernels_dir = manifest_dir.join("kernels");
    let out_dir = PathBuf::from(env::var("OUT_DIR").unwrap());

    // Discover .hip.cpp sources (files whose stem ends in ".hip", e.g. "identity.hip").
    let sources: Vec<PathBuf> = match std::fs::read_dir(&kernels_dir) {
        Ok(entries) => entries
            .filter_map(|e| e.ok())
            .map(|e| e.path())
            .filter(|p| {
                p.extension().and_then(|s| s.to_str()) == Some("cpp")
                    && p.file_stem()
                        .and_then(|s| s.to_str())
                        .map(|s| s.ends_with(".hip"))
                        .unwrap_or(false)
            })
            .collect(),
        Err(_) => Vec::new(),
    };

    if sources.is_empty() {
        println!(
            "cargo:warning=wave11 build.rs: no kernel sources in {}, skipping kernel link",
            kernels_dir.display()
        );
        return;
    }

    println!("cargo:rerun-if-changed={}", kernels_dir.display());
    for src in &sources {
        println!("cargo:rerun-if-changed={}", src.display());
    }

    let hipcc = env::var("HIPCC").unwrap_or_else(|_| "hipcc".to_string());
    let lib_path = out_dir.join("libwave11_kernels.so");

    let mut cmd = Command::new(&hipcc);
    cmd.arg("--offload-arch=gfx1101")
        .arg("--shared")
        .arg("-fPIC")
        .arg("-O3")
        .arg("-std=c++17")
        .arg(format!("-I{}", kernels_dir.display()));
    for src in &sources {
        cmd.arg(src);
    }
    cmd.arg("-o").arg(&lib_path);

    let status = cmd
        .status()
        .unwrap_or_else(|e| panic!("wave11 build.rs: failed to invoke {}: {}", hipcc, e));
    if !status.success() {
        panic!("wave11 build.rs: hipcc failed with status {}", status);
    }

    // Link the produced .so (compiled into $OUT_DIR which is stable).
    println!("cargo:rustc-link-search=native={}", out_dir.display());
    println!("cargo:rustc-link-lib=dylib=wave11_kernels");
    println!("cargo:rustc-link-arg=-Wl,-rpath,{}", out_dir.display());
}
