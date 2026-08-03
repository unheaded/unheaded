// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! # Zhenai Forge
//!
//! Custom Rust LoRA fine-tuning tool for GGUF models.
//! Reads quantized models directly (no fp16 decompression),
//! trains LoRA adapters on GPU via ROCm/HIP.
//!
//! Usage:
//!   zhenai-forge train --model model.gguf --data train.jsonl --output lora.gguf
//!   zhenai-forge eval --model model.gguf --lora lora.gguf --data eval.jsonl
//!
//! ## On `#[allow(clippy::too_many_arguments)]` in this crate
//!
//! Roughly twenty functions here carry that attribute. It is a deliberate,
//! bounded exemption, not a blanket one — the lint stays active everywhere
//! else, including the CLI.
//!
//! The exempt functions are all of one kind: BLAS wrappers and transformer
//! kernels. `sgemm(m, n, k, alpha, a, lda, b, ldb, beta, c, ldc)` is not an
//! over-grown signature, it is *the* BLAS signature — bundling it into a
//! struct would add a shim between us and hipBLAS for no gain. The attention
//! kernels are the same story: q/k/v/mask plus their dimensions and strides.
//! clippy's default threshold of 7 is simply the wrong threshold for this
//! domain.
//!
//! The alternative — parameter structs for the gradient path — is a real
//! option and might genuinely read better. It was NOT done here because a
//! mistake while shuffling tensor arguments produces silently wrong gradients
//! rather than a compile error, and that is not a change to make unreviewed.
//! If you want that refactor, do it deliberately with the gradient tests in
//! `backward.rs` as the gate.
//!
//! ## On `#[allow(clippy::needless_range_loop)]` in this crate
//!
//! Same shape of exemption, same reasoning. These loops index flattened 2-D
//! tensors with a computed stride — `self.b[r * output_dim + o]`,
//! `cpu_weights.output[v * n + ii]`, `v[(j * n_kv_heads + h_kv) * head_dim + d]`.
//! The loop counter is arithmetic input to an offset, not a cursor over the
//! slice being iterated, so `.iter().enumerate()` cannot express it without
//! reintroducing the same multiply.
//!
//! Where the loop genuinely WAS a plain cursor it was rewritten rather than
//! annotated (element-wise gradient accumulation in `gemma4.rs` now zips, the
//! scrambled-corpus fill in `eval.rs` uses `iter_mut().skip()`). Do not
//! bulk-annotate new loops to silence this lint — check first whether the
//! index is actually strided.
//!
//! ## Crate shape: library + thin binary
//!
//! The training and evaluation code lives in this library; `main.rs` is only
//! the CLI on top of it. That split is deliberate. As a binary-only crate,
//! `pub` granted no dead-code exemption — an item counted as used only if the
//! shipped `zhenai-forge` binary reached it — so three legitimate categories
//! were all reported as dead:
//!
//! 1. **Test-exercised research paths.** The Learning Gate experiment runners
//!    in `eval.rs` are driven by the test suite, not the CLI. Running the
//!    experiments via `cargo test` is the design, not an oversight.
//! 2. **Cascading false positives.** `eval_stats::bootstrap_ci_95` was
//!    reported as never used while being called from `eval.rs` — dead only
//!    because *its caller* was dead.
//! 3. **Deliberate extension surface.** `hip_kernels` is the `extern "C"` ABI
//!    mirroring the `.hip.cpp` kernels, and `backend.rs` is ADR-048's
//!    pluggable `ForgeBackend`, designed so new backends drop in.
//!
//! With the split, `pub` means "API" and `dead_code` means what it says. Keep
//! it that way: do not reach for `#![allow(dead_code)]` at crate root — it
//! would clear the warnings by making the crate permanently unable to report
//! real ones.

pub mod backend;
pub mod backward;
pub mod data;
pub mod eval;
pub mod eval_stats;
pub mod forward;
pub mod gemma4;
pub mod gemma4_gpu;
pub mod gguf;
pub mod hip;
pub mod hip_kernels;
pub mod lora;
pub mod quant;
pub mod tokenizer;
pub mod train;
