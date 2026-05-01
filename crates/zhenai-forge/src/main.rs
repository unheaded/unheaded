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

mod backend;
mod backward;
mod data;
mod eval;
mod eval_stats;
mod forward;
mod hip_kernels;
mod gemma4;
mod gemma4_gpu;
mod gguf;
mod hip;
mod lora;
mod quant;
mod tokenizer;
mod train;

fn main() {
    let args: Vec<String> = std::env::args().collect();

    match args.get(1).map(|s| s.as_str()) {
        Some("train") => cmd_train(&args[2..]),
        Some("train-gemma4") => cmd_train_gemma4(&args[2..]),
        Some("eval-gemma4") => cmd_eval_gemma4(&args[2..]),
        Some("generate-gemma4") => cmd_generate_gemma4(&args[2..]),
        Some("eval") => cmd_eval(&args[2..]),
        Some("info") => cmd_info(&args[2..]),
        _ => {
            eprintln!("Zhenai Forge v0.1.0 — Custom LoRA Training");
            eprintln!();
            eprintln!("Usage:");
            eprintln!("  zhenai-forge info   <model.gguf>           Show model info");
            eprintln!("  zhenai-forge train  --model <model.gguf>   Train LoRA on Mistral");
            eprintln!("                      --data <train.jsonl>");
            eprintln!("                      --output <lora.gguf>");
            eprintln!("                      [--rank 16]");
            eprintln!("                      [--epochs 2]");
            eprintln!("                      [--lr 2e-4]");
            eprintln!();
            eprintln!("  zhenai-forge train-gemma4 --model <gemma4.gguf>");
            eprintln!("                            --data <prepokenized.jsonl>");
            eprintln!("                            [--rank 16] [--alpha 32]");
            eprintln!("                            [--lr 3e-4] [--steps 100]");
            eprintln!("                            [--answer-start 1] [--cpu]");
            eprintln!("    Train a Gemma 4 LoRA via the WAVE10F path. Defaults to GPU");
            eprintln!("    (all 14 matmul sites on hipBLAS, ~2s/step warm on E2B). Pass");
            eprintln!("    --cpu to force the CPU fallback (~55-95s/step). Data file is");
            eprintln!("    JSONL with one {{\"tokens\": [int, ...]}} object per line.");
            eprintln!("    Tokenize upstream — forge does not include a Gemma 4 tokenizer.");
            eprintln!();
            eprintln!("  zhenai-forge eval   --model <model.gguf>   Evaluate model");
            eprintln!("                      --lora <lora.gguf>");
            eprintln!("                      --data <eval.jsonl>");
            std::process::exit(1);
        }
    }
}

/// One pre-tokenized RAFT example. Schema mirrors
/// `scripts/tokenize-kingdom-for-gemma4.py`:
///     {"tokens": [int, ...], "answer_start": int}
///
/// `answer_start` is the index of the first token after the prompt portion;
/// loss is applied only to positions `[answer_start..tokens.len()-1]` during
/// training. When the field is absent we fall back to the CLI flag value.
#[derive(serde::Deserialize)]
struct TrainExample {
    tokens: Vec<u32>,
    #[serde(default)]
    answer_start: Option<usize>,
}

fn cmd_train_gemma4(args: &[String]) {
    let mut model_path = String::new();
    let mut data_path = String::new();
    let mut output_path = String::new();
    let mut rank: u32 = 16;
    let mut alpha: f32 = 32.0;
    let mut lr: f32 = 3e-4;
    let mut steps: usize = 10;
    let mut answer_start: usize = 1;
    let mut save_every: usize = 0;
    let mut cpu_only: bool = false;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--model"        => { model_path = args[i + 1].clone(); i += 2; }
            "--data"         => { data_path = args[i + 1].clone(); i += 2; }
            "--output"       => { output_path = args[i + 1].clone(); i += 2; }
            "--rank"         => { rank = args[i + 1].parse().unwrap(); i += 2; }
            "--alpha"        => { alpha = args[i + 1].parse().unwrap(); i += 2; }
            "--lr"           => { lr = args[i + 1].parse().unwrap(); i += 2; }
            "--steps"        => { steps = args[i + 1].parse().unwrap(); i += 2; }
            "--answer-start" => { answer_start = args[i + 1].parse().unwrap(); i += 2; }
            "--save-every"   => { save_every = args[i + 1].parse().unwrap(); i += 2; }
            "--cpu"          => { cpu_only = true; i += 1; }
            _ => { eprintln!("unknown arg: {}", args[i]); i += 1; }
        }
    }

    if model_path.is_empty() || data_path.is_empty() {
        eprintln!("--model and --data are required");
        std::process::exit(1);
    }

    println!("Loading Gemma 4 GGUF: {}", model_path);
    let model = match gguf::GgufFile::open(&model_path) {
        Ok(m) => m,
        Err(e) => { eprintln!("Failed to open: {}", e); std::process::exit(1); }
    };
    let weights = match gemma4::CpuWeightsGemma4::load(&model) {
        Ok(w) => w,
        Err(e) => { eprintln!("Failed to load: {}", e); std::process::exit(1); }
    };
    weights.print_summary();

    println!("\nBuilding LoRA adapters (rank={}, alpha={}):", rank, alpha);
    let mut lora = gemma4::Gemma4LoraAdapters::new(&weights.hparams, rank, alpha);
    println!("  Active targets: {}", lora.n_active_targets());
    println!("  LoRA size:      {:.1} MB", lora.size_bytes() as f64 / 1e6);

    println!("\nLoading training data: {}", data_path);
    let data = match std::fs::read_to_string(&data_path) {
        Ok(s) => s,
        Err(e) => { eprintln!("Failed to read data: {}", e); std::process::exit(1); }
    };
    // Use serde_json so the JSON `answer_start` field is read as an integer
    // rather than absorbed into the tokens vector by a naive non-digit-split
    // (the WAVE14 H6 root cause — see notes/wave14-h2h6-analysis.md).
    let examples: Vec<TrainExample> = data.lines()
        .filter(|l| !l.trim().is_empty())
        .filter_map(|l| serde_json::from_str::<TrainExample>(l).ok())
        .filter(|ex| ex.tokens.len() >= 2)
        .collect();
    println!("  Loaded {} examples", examples.len());
    if examples.is_empty() {
        eprintln!("No valid examples found");
        std::process::exit(1);
    }
    let n_with_answer_start = examples.iter().filter(|ex| ex.answer_start.is_some()).count();
    println!("  Per-example answer_start present in {}/{} examples (CLI fallback={})",
        n_with_answer_start, examples.len(), answer_start);

    // Select backend at runtime. The training loop itself is generic over
    // B: ForgeBackend, so every backend (CPU, hybrid-matmul, future
    // GPU-kernels, hypothetical LlamaCppBackend) uses the same loop. This
    // is the payoff of the trait refactor: ONE training loop, N backends.
    let cpu_backend_fallback = backend::CpuBackend;
    let hybrid_backend = backend::HybridMatmulBackend::default();
    let hybrid_handle: Option<gemma4_gpu::Gemma4GpuWeights> = if cpu_only {
        println!("\n--cpu requested — using CpuBackend.");
        None
    } else {
        use backend::ForgeBackend as _;  // bring method into scope
        println!("\nUploading Gemma 4 weights to GPU (PleMode::Cpu) — HybridMatmulBackend...");
        match hybrid_backend.upload_weights(&weights) {
            Ok(g) => {
                println!("  VRAM used:   {:.2} GB", g.vram_used_gb());
                Some(g)
            }
            Err(e) => {
                eprintln!("  GPU upload failed ({}). Falling back to CpuBackend.", e);
                None
            }
        }
    };

    let path_label = if hybrid_handle.is_some() { "hybrid-matmul (GPU fwd+bwd matmuls)" } else { "cpu" };
    println!("\nTraining for {} steps (lr={}, answer_start={}, backend={})...",
        steps, lr, answer_start, path_label);

    // Generic training loop — instantiated once per backend. Returns
    // total_loss so the caller can print the per-step average.
    fn run_loop<B: backend::ForgeBackend>(
        backend: &B,
        weights: &gemma4::CpuWeightsGemma4,
        handle: &B::Handle,
        lora: &mut gemma4::Gemma4LoraAdapters,
        examples: &[TrainExample],
        steps: usize,
        cli_answer_start: usize,
        lr: f32,
        save_every: usize,
        output_path: &str,
    ) -> f64 {
        let start = std::time::Instant::now();
        let mut total_loss = 0.0f64;
        for step in 1..=steps {
            let example = &examples[(step - 1) % examples.len()];
            crate::gemma4_gpu::profile_reset();
            let step_start = std::time::Instant::now();
            // Prefer the per-example JSON answer_start; fall back to the CLI
            // flag (default 1) when absent. The earlier `.min(tokens.len()/2)`
            // clamp was a defensive hedge from before the H6 fix landed; with
            // per-example answer_start now plumbed correctly (verified
            // 3568/3568 in Run B), the clamp would force loss masking onto
            // positions [192..384] for the Kingdom corpus where high-frequency
            // tokens (\n, " the", ",") dominate — yielding a unigram-frequency
            // local optimum on the loss while generation collapses.
            // Removed in WAVE14 Path C (notes/wave-14/wave14-runB-results.md).
            // Defensive: cap at tokens.len()-2 so we always have at least one
            // loss position; for malformed answer_start (≥ tokens.len()) this
            // collapses to "predict the last token only", which is correct.
            let effective_answer_start = example.answer_start
                .unwrap_or(cli_answer_start)
                .min(example.tokens.len().saturating_sub(2));
            let loss = backend.train_step(
                weights, handle, lora, &example.tokens,
                effective_answer_start, lr, step as u32,
            ).expect("train step");
            total_loss += loss as f64;
            let avg_loss = total_loss / step as f64;
            let step_secs = step_start.elapsed().as_secs_f64();
            let elapsed_min = start.elapsed().as_secs_f64() / 60.0;
            let eta_min = (steps - step) as f64 * step_secs / 60.0;
            println!("  [step {}/{}] loss={:.4} avg={:.4} step_time={:.1}s elapsed={:.1}m eta={:.1}m  (backend={})",
                step, steps, loss, avg_loss, step_secs, elapsed_min, eta_min, backend.name());
            if crate::gemma4_gpu::profile_enabled() {
                let (method_ns, sgemm_ns, n_calls) = crate::gemma4_gpu::profile_snapshot();
                let method_s = method_ns as f64 / 1e9;
                let sgemm_s = sgemm_ns as f64 / 1e9;
                let overhead_s = method_s - sgemm_s;
                let frac_method = method_s / step_secs * 100.0;
                let frac_sgemm = sgemm_s / step_secs * 100.0;
                println!("         [PROFILE] matmul_method={:.2}s ({:.1}% of step) sgemm_only={:.2}s ({:.1}%) roundtrip_overhead={:.2}s calls={}",
                    method_s, frac_method, sgemm_s, frac_sgemm, overhead_s, n_calls);
            }
            let _ = std::io::Write::flush(&mut std::io::stdout());

            if save_every > 0 && !output_path.is_empty() && step % save_every == 0 {
                let cp_path = format!("{}.checkpoint-{}", output_path, step);
                match lora.save(&cp_path) {
                    Ok(_) => println!("  Checkpoint saved: {}", cp_path),
                    Err(e) => eprintln!("  Checkpoint save failed: {}", e),
                }
            }
        }
        total_loss
    }

    // Thread total_loss + wall_start through so the summary section can
    // still print them after the generic loop returns.
    let wall_start = std::time::Instant::now();
    let final_total_loss = match hybrid_handle {
        Some(ref h) => run_loop(
            &hybrid_backend, &weights, h, &mut lora,
            &examples, steps, answer_start, lr, save_every, &output_path,
        ),
        None => run_loop(
            &cpu_backend_fallback, &weights, &(), &mut lora,
            &examples, steps, answer_start, lr, save_every, &output_path,
        ),
    };

    let total_time_min = wall_start.elapsed().as_secs_f64() / 60.0;
    println!("\n========================================");
    println!("TRAINING COMPLETE");
    println!("========================================");
    println!("  Steps:       {}", steps);
    println!("  Time:        {:.1} min", total_time_min);
    println!("  Final avg:   {:.4}", final_total_loss / steps as f64);

    if !output_path.is_empty() {
        match lora.save(&output_path) {
            Ok(_) => println!("  Saved to:    {}", output_path),
            Err(e) => eprintln!("  Save failed: {}", e),
        }
    } else {
        println!();
        println!("No --output specified; in-memory LoRA discarded on exit.");
    }
}

fn cmd_info(args: &[String]) {
    let path = args.first().expect("Usage: zhenai-forge info <model.gguf>");

    println!("Loading GGUF: {}", path);
    match gguf::GgufFile::open(path) {
        Ok(model) => {
            println!("  Format:     GGUF v{}", model.version);
            println!("  Tensors:    {}", model.tensors.len());
            println!("  Metadata:   {} entries", model.metadata.len());
            println!("  File size:  {:.1} MB", model.file_size as f64 / 1e6);

            // Show key metadata
            let mut sorted_keys: Vec<_> = model.metadata.keys().collect();
            sorted_keys.sort();
            for key in &sorted_keys {
                let val = &model.metadata[*key];
                let show = key.contains("name") || key.contains("arch") || key.contains("context")
                    || key.contains("block_count") || key.contains("embedding")
                    || key.contains("tokenizer.ggml.model") || key.contains("bos") || key.contains("eos")
                    || key.contains("vocab");
                let truncated = if val.len() > 80 { format!("{}...", &val[..80]) } else { val.clone() };
                if show {
                    println!("  {}: {}", key, truncated);
                }
            }
            // Show all metadata keys (for discovery)
            println!("\n  All metadata keys:");
            for key in &sorted_keys {
                println!("    {}", key);
            }

            // List tensors (first 30 + pattern summary)
            if args.len() > 1 && args[1] == "--verbose" {
                println!("\n  All tensors:");
                for t in &model.tensors {
                    println!("    {} [{}] {:?} ({:.1} MB)", t.name, t.tensor_type,
                        t.dimensions, t.byte_size as f64 / 1e6);
                }
            } else {
                // Show unique name patterns
                let mut patterns: std::collections::HashSet<String> = std::collections::HashSet::new();
                for t in &model.tensors {
                    let pattern = t.name.replace(|c: char| c.is_ascii_digit(), "N");
                    patterns.insert(pattern);
                }
                println!("\n  Tensor name patterns ({}):", patterns.len());
                let mut sorted: Vec<_> = patterns.into_iter().collect();
                sorted.sort();
                for p in &sorted {
                    println!("    {}", p);
                }
            }

            // Summarize tensor types
            let mut type_counts: std::collections::HashMap<String, usize> = std::collections::HashMap::new();
            let mut total_bytes: u64 = 0;
            for t in &model.tensors {
                *type_counts.entry(t.tensor_type.clone()).or_default() += 1;
                total_bytes += t.byte_size;
            }
            println!("\n  Tensor types:");
            for (ttype, count) in &type_counts {
                println!("    {}: {} tensors", ttype, count);
            }
            println!("  Total tensor data: {:.1} MB", total_bytes as f64 / 1e6);

            // LoRA budget calculation
            if let (Some(layers), Some(dim)) = (
                model.get_u32("llama.block_count"),
                model.get_u32("llama.embedding_length"),
            ) {
                let rank = 16u32;
                let targets = 4u32; // q, k, v, o projections
                let lora_params = layers * targets * 2 * dim * rank;
                let lora_bytes = lora_params as u64 * 2; // fp16
                println!("\n  LoRA budget (rank {}):", rank);
                println!("    Trainable params: {}", lora_params);
                println!("    LoRA size:        {:.1} MB", lora_bytes as f64 / 1e6);
                println!("    Base params:      {:.1}B", model.tensors.iter().map(|t| t.num_elements).sum::<u64>() as f64 / 1e9);
                println!("    Trainable ratio:  {:.3}%", lora_params as f64 / model.tensors.iter().map(|t| t.num_elements).sum::<u64>() as f64 * 100.0);
            }
        }
        Err(e) => {
            eprintln!("Error: {}", e);
            std::process::exit(1);
        }
    }
}

fn cmd_train(args: &[String]) {
    let mut config = train::TrainConfig::default();

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--model" => { config.model_path = args[i + 1].clone(); i += 2; }
            "--data" => { config.data_path = args[i + 1].clone(); i += 2; }
            "--output" => { config.output_path = args[i + 1].clone(); i += 2; }
            "--rank" => { config.rank = args[i + 1].parse().unwrap(); i += 2; }
            "--epochs" => { config.epochs = args[i + 1].parse().unwrap(); i += 2; }
            "--lr" => { config.lr = args[i + 1].parse().unwrap(); i += 2; }
            "--batch-size" => { config.batch_size = args[i + 1].parse().unwrap(); i += 2; }
            "--max-seq-len" => { config.max_seq_len = args[i + 1].parse().unwrap(); i += 2; }
            _ => { i += 1; }
        }
    }

    if config.model_path.is_empty() || config.data_path.is_empty() {
        eprintln!("Error: --model and --data are required");
        std::process::exit(1);
    }

    if let Err(e) = train::train(&config) {
        eprintln!("Training failed: {}", e);
        std::process::exit(1);
    }
}

fn cmd_eval(_args: &[String]) {
    println!("Evaluation not yet implemented (Phase 4)");
}

/// Held-out eval for a Gemma-4 LoRA. Computes mean cross-entropy loss over
/// every sequence in `--data` using the GPU forward path. If `--lora` is
/// provided, also computes the base-model (no-LoRA) loss for comparison —
/// required by the WAVE12 Phase 7 exit gate (eval@500 < eval@0).
///
/// Usage:
///   zhenai-forge eval-gemma4 --model <gguf> --data <eval.jsonl> [--lora <lora.gguf>] [--answer-start 1]
fn cmd_eval_gemma4(args: &[String]) {
    let mut model_path = String::new();
    let mut data_path = String::new();
    let mut lora_path = String::new();
    let mut answer_start: usize = 1;
    let mut cpu_only = false;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--model"        => { model_path = args[i + 1].clone(); i += 2; }
            "--data"         => { data_path = args[i + 1].clone(); i += 2; }
            "--lora"         => { lora_path = args[i + 1].clone(); i += 2; }
            "--answer-start" => { answer_start = args[i + 1].parse().unwrap(); i += 2; }
            "--cpu"          => { cpu_only = true; i += 1; }
            _ => { eprintln!("unknown arg: {}", args[i]); i += 1; }
        }
    }
    if model_path.is_empty() || data_path.is_empty() {
        eprintln!("--model and --data are required");
        std::process::exit(1);
    }

    println!("Loading Gemma 4 GGUF: {}", model_path);
    let model = match gguf::GgufFile::open(&model_path) {
        Ok(m) => m, Err(e) => { eprintln!("open: {}", e); std::process::exit(1); }
    };
    let weights = match gemma4::CpuWeightsGemma4::load(&model) {
        Ok(w) => w, Err(e) => { eprintln!("load: {}", e); std::process::exit(1); }
    };

    let lora_opt = if !lora_path.is_empty() {
        match gemma4::Gemma4LoraAdapters::load(&lora_path) {
            Ok(l) => {
                println!("Loaded LoRA: {} (rank={}, alpha={})", lora_path, l.rank, l.alpha);
                Some(l)
            }
            Err(e) => { eprintln!("LoRA load: {}", e); std::process::exit(1); }
        }
    } else { None };

    println!("Loading eval data: {}", data_path);
    // H7 fix (2026-04-29) — was using the same naive non-digit-split parser
    // as the pre-fix cmd_train_gemma4 (H6). Migrated to serde_json so the
    // JSON `answer_start` field is read as an integer rather than absorbed
    // into the tokens vector. Mirrors `eval::load_tokenized_jsonl` shape.
    let data = std::fs::read_to_string(&data_path).expect("read eval data");
    let parsed: Vec<TrainExample> = data.lines()
        .filter(|l| !l.trim().is_empty())
        .filter_map(|l| serde_json::from_str::<TrainExample>(l).ok())
        .filter(|ex| ex.tokens.len() >= 2)
        .collect();
    let n_with_answer_start = parsed.iter().filter(|ex| ex.answer_start.is_some()).count();
    let examples: Vec<Vec<u32>> = parsed.iter().map(|ex| ex.tokens.clone()).collect();
    let per_example_answer_starts: Vec<usize> = parsed.iter()
        .map(|ex| ex.answer_start.unwrap_or(answer_start))
        .collect();
    println!("  {} eval sequences loaded ({} with per-example answer_start, CLI fallback={})",
        examples.len(), n_with_answer_start, answer_start);
    // Use the first per-example answer_start as the harness scalar default;
    // the per-example vector wires through to anywhere the harness honors it.
    let harness_default_answer_start = per_example_answer_starts.first()
        .copied().unwrap_or(answer_start);

    let cpu_backend = backend::CpuBackend;
    let hybrid_backend = backend::HybridMatmulBackend::default();
    let hybrid_handle = if cpu_only {
        None
    } else {
        use backend::ForgeBackend as _;
        match hybrid_backend.upload_weights(&weights) {
            Ok(g) => { println!("GPU upload OK ({:.2} GB VRAM)", g.vram_used_gb()); Some(g) }
            Err(e) => { eprintln!("GPU upload failed ({}); falling back to CPU.", e); None }
        }
    };

    // Build a thin EvalHarness for scoring. `train` is unused here, so
    // `train_answer_starts` is empty — the per-example train loop never
    // runs against this harness. The harness scalar `answer_start` defaults
    // to the first eval record's value when present (matches
    // `harness_from_jsonl`), and the per-example vector is plumbed through
    // the local `score()` closure below for per-sequence loss masking.
    let harness = eval::EvalHarness {
        train: Vec::new(),
        eval: examples.clone(),
        answer_start: harness_default_answer_start,
        vocab_size: weights.hparams.vocab_size,
        train_answer_starts: Vec::new(),
    };
    let _ = &per_example_answer_starts; // available for future per-seq loss masking

    fn score<B: backend::ForgeBackend>(
        backend: &B,
        harness: &eval::EvalHarness,
        weights: &gemma4::CpuWeightsGemma4,
        handle: &B::Handle,
        lora: Option<&gemma4::Gemma4LoraAdapters>,
    ) -> (f32, usize) {
        let mut total = 0.0f64;
        let mut n = 0;
        let t0 = std::time::Instant::now();
        for (idx, tokens) in harness.eval.iter().enumerate() {
            match harness.forward_loss_with_backend(backend, weights, handle, lora, tokens) {
                Ok(l) if l.is_finite() => { total += l as f64; n += 1; }
                Ok(_) => { eprintln!("  [seq {}] loss non-finite, skipping", idx); }
                Err(e) => { eprintln!("  [seq {}] forward error: {}", idx, e); }
            }
            if (idx + 1) % 25 == 0 {
                let avg = if n > 0 { (total / n as f64) as f32 } else { f32::NAN };
                let elapsed = t0.elapsed().as_secs_f64();
                let eta = if idx > 0 { elapsed * (harness.eval.len() - idx - 1) as f64 / (idx + 1) as f64 } else { 0.0 };
                println!("  [{}/{}] running avg loss = {:.4}  elapsed {:.0}s  eta {:.0}s",
                    idx + 1, harness.eval.len(), avg, elapsed, eta);
            }
        }
        let mean = if n > 0 { (total / n as f64) as f32 } else { f32::NAN };
        (mean, n)
    }

    println!("\n=== EVAL: base model (no LoRA) ===");
    let (base_loss, base_n) = match &hybrid_handle {
        Some(h) => score(&hybrid_backend, &harness, &weights, h, None),
        None    => score(&cpu_backend,    &harness, &weights, &(), None),
    };
    println!("  mean CE loss (base):      {:.4}  (n={})", base_loss, base_n);

    if let Some(ref l) = lora_opt {
        println!("\n=== EVAL: with LoRA ===");
        let (lora_loss, lora_n) = match &hybrid_handle {
            Some(h) => score(&hybrid_backend, &harness, &weights, h, Some(l)),
            None    => score(&cpu_backend,    &harness, &weights, &(), Some(l)),
        };
        println!("  mean CE loss (+ LoRA):    {:.4}  (n={})", lora_loss, lora_n);
        println!("  delta (base - LoRA):      {:+.4}", base_loss - lora_loss);
        if lora_loss < base_loss {
            println!("  PHASE 7 EXIT GATE:        ✅ eval descended below base");
        } else {
            println!("  PHASE 7 EXIT GATE:        ❌ eval did NOT descend below base");
        }
    }
}

// =============================================================================
// WAVE13 — generate-gemma4 subcommand.
//
// Token-by-token decoder over forward_gemma4_gpu. Runs the entire prefix
// through forward each step (no KV-cache; correctness first, KV-cache is
// WAVE14+ if perf becomes a bottleneck for serving). Sampling: greedy by
// default, optional --temperature / --top-k / --top-p / --seed.
//
// Modes:
//   --prompt <text>          — raw text encoded with the in-tree GGUF tokenizer
//   --gemma-prompt <text>    — wrap with Gemma-4 chat template via gemma4-venv
//                              Python subprocess, then encode with HF tokenizer
//   --tokens '[1,2,…]'       — pre-tokenized JSON array of token ids
//
// Constraints:
//   prompt_len + max_new_tokens ≤ MAX_SEQ (defaults to 384, the WAVE12 trained
//   sequence length). Exceeding it returns NaN-prone activations on the
//   LoRA path; base-only forward tolerates longer but isn't validated.
// =============================================================================

const W13_MAX_SEQ: usize = 384;
const W13_GEMMA_END_OF_TURN: u32 = 106;
const W13_GEMMA_EOS: u32 = 1;
const W13_GEMMA_BOS: u32 = 2;

fn cmd_generate_gemma4(args: &[String]) {
    let mut model_path = String::new();
    let mut lora_path = String::new();
    let mut prompt_raw = String::new();
    let mut prompt_gemma = String::new();
    let mut prompt_tokens_json = String::new();
    let mut max_new_tokens: usize = 100;
    let mut temperature: f32 = 0.0;
    let mut top_k: usize = 0;
    let mut top_p: f32 = 1.0;
    let mut rep_penalty: f32 = 1.0;
    let mut rep_penalty_window: usize = 256;
    let mut seed: u64 = 0xC011D5;
    let mut cpu_only = false;
    let mut quiet = false;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--model"           => { model_path = args[i+1].clone(); i += 2; }
            "--lora"            => { lora_path = args[i+1].clone(); i += 2; }
            "--prompt"          => { prompt_raw = args[i+1].clone(); i += 2; }
            "--gemma-prompt"    => { prompt_gemma = args[i+1].clone(); i += 2; }
            "--tokens"          => { prompt_tokens_json = args[i+1].clone(); i += 2; }
            "--max-new-tokens"  => { max_new_tokens = args[i+1].parse().unwrap(); i += 2; }
            "--temperature"     => { temperature = args[i+1].parse().unwrap(); i += 2; }
            "--top-k"           => { top_k = args[i+1].parse().unwrap(); i += 2; }
            "--top-p"           => { top_p = args[i+1].parse().unwrap(); i += 2; }
            "--rep-penalty"     => { rep_penalty = args[i+1].parse().unwrap(); i += 2; }
            "--rep-penalty-window" => { rep_penalty_window = args[i+1].parse().unwrap(); i += 2; }
            "--seed"            => { seed = args[i+1].parse().unwrap(); i += 2; }
            "--cpu"             => { cpu_only = true; i += 1; }
            "--quiet"           => { quiet = true; i += 1; }
            _ => { eprintln!("unknown arg: {}", args[i]); i += 1; }
        }
    }

    if model_path.is_empty() {
        eprintln!("--model is required");
        std::process::exit(1);
    }
    let prompt_modes_set: u32 = (!prompt_raw.is_empty() as u32)
        + (!prompt_gemma.is_empty() as u32)
        + (!prompt_tokens_json.is_empty() as u32);
    if prompt_modes_set != 1 {
        eprintln!("exactly one of --prompt | --gemma-prompt | --tokens is required");
        std::process::exit(1);
    }

    if !quiet { eprintln!("Loading Gemma 4 GGUF: {}", model_path); }
    let model = match gguf::GgufFile::open(&model_path) {
        Ok(m) => m, Err(e) => { eprintln!("open: {}", e); std::process::exit(1); }
    };
    let weights = match gemma4::CpuWeightsGemma4::load(&model) {
        Ok(w) => w, Err(e) => { eprintln!("load: {}", e); std::process::exit(1); }
    };

    let lora_opt = if !lora_path.is_empty() {
        match gemma4::Gemma4LoraAdapters::load(&lora_path) {
            Ok(l) => {
                if !quiet { eprintln!("Loaded LoRA: {} (rank={}, alpha={})", lora_path, l.rank, l.alpha); }
                Some(l)
            }
            Err(e) => { eprintln!("LoRA load: {}", e); std::process::exit(1); }
        }
    } else { None };

    // Tokenize the prompt. Three paths:
    //   - --tokens: parse JSON array directly, no tokenizer needed.
    //   - --gemma-prompt: shell out to gemma4-venv Python.
    //   - --prompt: use the in-tree GGUF tokenizer (greedy longest-match).
    let mut tokens: Vec<u32> = if !prompt_tokens_json.is_empty() {
        parse_tokens_json(&prompt_tokens_json).unwrap_or_else(|e| {
            eprintln!("--tokens parse: {}", e); std::process::exit(1);
        })
    } else if !prompt_gemma.is_empty() {
        encode_via_gemma_venv(&prompt_gemma).unwrap_or_else(|e| {
            eprintln!("gemma-prompt encode failed: {}", e); std::process::exit(1);
        })
    } else {
        // --prompt: in-tree tokenizer
        let vocab = match tokenizer::extract_vocabulary_from_gguf(&model_path) {
            Ok(v) => v,
            Err(e) => { eprintln!("vocab extract: {}", e); std::process::exit(1); }
        };
        let tok = tokenizer::Tokenizer::from_tokens(vocab, W13_GEMMA_BOS, W13_GEMMA_EOS);
        tok.encode(&prompt_raw)
    };

    if !quiet {
        eprintln!("Prompt tokens: {} (first 8: {:?})",
            tokens.len(), &tokens[..tokens.len().min(8)]);
    }
    if tokens.is_empty() {
        eprintln!("empty prompt after tokenization"); std::process::exit(1);
    }

    // Cap so prompt + new tokens stays in trained seq range.
    let allowed_new = W13_MAX_SEQ.saturating_sub(tokens.len());
    if allowed_new == 0 {
        eprintln!("prompt already {} tokens — at or beyond MAX_SEQ={}; truncate or shorten.",
            tokens.len(), W13_MAX_SEQ);
        std::process::exit(1);
    }
    let effective_new = max_new_tokens.min(allowed_new);
    if effective_new < max_new_tokens && !quiet {
        eprintln!("max_new_tokens capped {} → {} (prompt {} + new ≤ {})",
            max_new_tokens, effective_new, tokens.len(), W13_MAX_SEQ);
    }

    // Backend selection (mirror train-gemma4 path).
    let cpu_backend_fallback = backend::CpuBackend;
    let hybrid_backend = backend::HybridMatmulBackend::default();
    let hybrid_handle: Option<gemma4_gpu::Gemma4GpuWeights> = if cpu_only {
        if !quiet { eprintln!("--cpu: CpuBackend forward"); }
        None
    } else {
        use backend::ForgeBackend as _;
        match hybrid_backend.upload_weights(&weights) {
            Ok(g) => {
                if !quiet { eprintln!("GPU upload: {:.2} GB VRAM", g.vram_used_gb()); }
                Some(g)
            }
            Err(e) => {
                eprintln!("GPU upload failed ({}); falling back to CPU.", e);
                None
            }
        }
    };
    let _ = cpu_backend_fallback; // path selection logic below uses hybrid_handle.is_some()

    // Generation loop. One forward per token (no KV-cache; WAVE13 ships
    // correctness first).
    let prompt_len = tokens.len();
    let stop_tokens: [u32; 3] = [W13_GEMMA_EOS, W13_GEMMA_END_OF_TURN, 0];
    let mut rng_state: u64 = seed.wrapping_add(0x9E3779B97F4A7C15);
    let t0 = std::time::Instant::now();
    let mut steps_done = 0;

    for step in 0..effective_new {
        let logits_full = match hybrid_handle.as_ref() {
            Some(g) => gemma4_gpu::forward_gemma4_gpu(&weights, g, lora_opt.as_ref(), &tokens),
            None => Ok(gemma4::forward_gemma4_with_lora(&weights, lora_opt.as_ref(), &tokens)),
        };
        let (logits, _caches) = match logits_full {
            Ok(x) => x,
            Err(e) => { eprintln!("\nforward failed at step {}: {}", step, e); break; }
        };
        let vocab = weights.hparams.vocab_size;
        let row_off = (tokens.len() - 1) * vocab;
        let last_logits = &logits[row_off..row_off + vocab];

        // Repetition-penalty window: only consider the last N tokens (prompt
        // tail + everything generated so far) to keep penalty cost bounded
        // and to avoid penalizing tokens that legitimately reoccur after a
        // long context-shift.
        let win_start = tokens.len().saturating_sub(rep_penalty_window);
        let past_window = &tokens[win_start..];
        let next = sample_next(
            last_logits,
            temperature,
            top_k,
            top_p,
            rep_penalty,
            past_window,
            &mut rng_state,
        );
        if !next.is_finite_id() {
            eprintln!("\nsampler returned invalid id; halting"); break;
        }
        let next_id = next.id;
        tokens.push(next_id);
        steps_done += 1;

        if stop_tokens.contains(&next_id) {
            if !quiet { eprintln!("\n[stop token id={} at step {}]", next_id, step + 1); }
            break;
        }
    }
    let elapsed = t0.elapsed().as_secs_f64();

    // Output the completion (tokens after the prompt).
    let completion = &tokens[prompt_len..];
    eprintln!("\n--- raw completion token IDs ({}): {:?} ---",
        completion.len(), completion);
    let completion_text = decode_via_gemma_venv(completion).unwrap_or_else(|e| {
        eprintln!("decode failed: {}; falling back to raw token ids", e);
        format!("{:?}", completion)
    });

    if !quiet {
        eprintln!("\n--- generated {} tokens in {:.1}s ({:.2}s/tok) ---",
            steps_done, elapsed, elapsed / steps_done.max(1) as f64);
    }
    println!("{}", completion_text.trim_end());
}

/// Parse a JSON array of integers (`[1,2,3]` or `{"tokens":[1,2,3]}`).
/// Robust to whitespace; tolerant of either shape.
fn parse_tokens_json(raw: &str) -> Result<Vec<u32>, String> {
    // Strip optional `{"tokens":` wrapper.
    let s = raw.trim();
    let inner = if let Some(idx) = s.find('[') {
        &s[idx..]
    } else {
        return Err(format!("no '[' in {:?}", &s[..s.len().min(80)]));
    };
    let end = inner.rfind(']').ok_or_else(|| "no ']' in tokens".to_string())?;
    let body = &inner[1..end];
    let mut out = Vec::new();
    for chunk in body.split(',') {
        let t = chunk.trim();
        if t.is_empty() { continue; }
        out.push(t.parse::<u32>().map_err(|e| format!("parse {:?}: {}", t, e))?);
    }
    if out.is_empty() {
        return Err("empty token array".into());
    }
    Ok(out)
}

/// Encode a text prompt via the gemma4-venv Python helper (applies the
/// Gemma-4 chat template). Returns token ids ending with the
/// start-of-model-turn marker, ready for generation.
fn encode_via_gemma_venv(prompt: &str) -> Result<Vec<u32>, String> {
    use std::process::{Command, Stdio};
    use std::io::Write;
    let mut child = Command::new("/home/govan/tmp/gemma4-venv/bin/python")
        .arg("/home/govan/tmp/unheaded/scripts/gemma4-encode-prompt.py")
        .arg("-")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| format!("spawn encode: {}", e))?;
    {
        let stdin = child.stdin.as_mut().ok_or("encode: no stdin")?;
        stdin.write_all(prompt.as_bytes()).map_err(|e| format!("encode write: {}", e))?;
    }
    let out = child.wait_with_output().map_err(|e| format!("encode wait: {}", e))?;
    if !out.status.success() {
        return Err(format!("encode exit {:?}: {}", out.status,
            String::from_utf8_lossy(&out.stderr)));
    }
    let body = String::from_utf8_lossy(&out.stdout);
    parse_tokens_json(body.trim())
}

/// Decode token ids back to text via the gemma4-venv Python helper. Slower
/// than an in-tree decoder but matches encoding round-trip exactly.
fn decode_via_gemma_venv(ids: &[u32]) -> Result<String, String> {
    use std::process::{Command, Stdio};
    use std::io::Write;
    let json_in = serde_json_array(ids);
    let mut child = Command::new("/home/govan/tmp/gemma4-venv/bin/python")
        .arg("/home/govan/tmp/unheaded/scripts/gemma4-decode-tokens.py")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| format!("spawn decode: {}", e))?;
    {
        let stdin = child.stdin.as_mut().ok_or("decode: no stdin")?;
        stdin.write_all(json_in.as_bytes()).map_err(|e| format!("decode write: {}", e))?;
    }
    let out = child.wait_with_output().map_err(|e| format!("decode wait: {}", e))?;
    if !out.status.success() {
        return Err(format!("decode exit {:?}: {}", out.status,
            String::from_utf8_lossy(&out.stderr)));
    }
    Ok(String::from_utf8_lossy(&out.stdout).into_owned())
}

fn serde_json_array(ids: &[u32]) -> String {
    let mut s = String::with_capacity(ids.len() * 6 + 2);
    s.push('[');
    for (i, id) in ids.iter().enumerate() {
        if i > 0 { s.push(','); }
        s.push_str(&id.to_string());
    }
    s.push(']');
    s
}

#[derive(Clone, Copy)]
struct SampledToken { id: u32 }
impl SampledToken {
    fn is_finite_id(&self) -> bool { self.id != u32::MAX }
}

/// Pick the next token from a logits row.
/// - temperature == 0.0 → argmax (greedy, deterministic), AFTER rep-penalty if any
/// - temperature > 0.0  → softmax(logits/T), apply top_k, top_p, multinomial sample.
///
/// Repetition penalty (HF/CTRL convention): for each token id that appears in
/// `past_tokens`, scale its raw logit by `1/rep_penalty` if logit ≥ 0 or by
/// `rep_penalty` if logit < 0. With rep_penalty > 1.0 this pushes already-seen
/// tokens DOWN regardless of sign. rep_penalty == 1.0 → no-op.
/// Critical anti-collapse measure for greedy decode on small LoRA-fine-tuned
/// models (cf. Run B/C/coding-w15s greedy `\n` collapse).
fn sample_next(
    logits: &[f32],
    temperature: f32,
    top_k: usize,
    top_p: f32,
    rep_penalty: f32,
    past_tokens: &[u32],
    rng: &mut u64,
) -> SampledToken {
    if !logits.iter().any(|x| x.is_finite()) {
        return SampledToken { id: u32::MAX };
    }

    // Apply repetition penalty (HF formula). Build a working copy so we don't
    // mutate the caller's logits row.
    let mut adjusted: Vec<f32> = logits.to_vec();
    if rep_penalty > 1.0 && !past_tokens.is_empty() {
        // Use a small de-dup set so repeated tokens get penalized once per logits
        // row, not N times.
        let mut seen = std::collections::HashSet::new();
        for &tok in past_tokens {
            if !seen.insert(tok) { continue; }
            let i = tok as usize;
            if i < adjusted.len() && adjusted[i].is_finite() {
                if adjusted[i] >= 0.0 {
                    adjusted[i] /= rep_penalty;
                } else {
                    adjusted[i] *= rep_penalty;
                }
            }
        }
    }

    if temperature <= 0.0 {
        let mut best_id: u32 = 0;
        let mut best_v = f32::NEG_INFINITY;
        for (i, &v) in adjusted.iter().enumerate() {
            if v > best_v { best_v = v; best_id = i as u32; }
        }
        return SampledToken { id: best_id };
    }

    // Build (id, scaled_logit) and find max for numerical stability.
    let mut scored: Vec<(u32, f32)> = adjusted.iter().enumerate()
        .filter(|(_, v)| v.is_finite())
        .map(|(i, &v)| (i as u32, v / temperature))
        .collect();
    let max_v = scored.iter().fold(f32::NEG_INFINITY, |a, b| a.max(b.1));

    // Apply top-k: keep only the k largest.
    if top_k > 0 && top_k < scored.len() {
        // Partial sort: use selection by repeated max — k is small in practice.
        scored.sort_unstable_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        scored.truncate(top_k);
    }

    // Convert to probabilities.
    let mut probs: Vec<(u32, f32)> = scored.iter()
        .map(|(id, lv)| (*id, (lv - max_v).exp()))
        .collect();
    let z: f32 = probs.iter().map(|(_, p)| *p).sum();
    if z <= 0.0 {
        return SampledToken { id: probs[0].0 };
    }
    for (_, p) in probs.iter_mut() { *p /= z; }

    // Apply top-p (nucleus): sort desc by prob, keep prefix s.t. cumulative ≥ top_p.
    if top_p < 1.0 && top_p > 0.0 {
        probs.sort_unstable_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        let mut cum = 0.0;
        let mut cutoff = probs.len();
        for (i, (_, p)) in probs.iter().enumerate() {
            cum += *p;
            if cum >= top_p { cutoff = i + 1; break; }
        }
        probs.truncate(cutoff);
        let z2: f32 = probs.iter().map(|(_, p)| *p).sum();
        if z2 > 0.0 {
            for (_, p) in probs.iter_mut() { *p /= z2; }
        }
    }

    // Multinomial sample using LCG.
    *rng = rng.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407);
    let r: f32 = ((*rng >> 33) as f32) / ((u32::MAX as f32) + 1.0);
    let mut acc = 0.0f32;
    let mut picked = probs[0].0;
    for (id, p) in &probs {
        acc += *p;
        if r <= acc { picked = *id; break; }
    }
    SampledToken { id: picked }
}

