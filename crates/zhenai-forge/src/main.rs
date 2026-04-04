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

mod data;
mod forward;
mod gguf;
mod hip;
mod quant;
mod lora;
mod tokenizer;
mod train;

fn main() {
    let args: Vec<String> = std::env::args().collect();

    match args.get(1).map(|s| s.as_str()) {
        Some("train") => cmd_train(&args[2..]),
        Some("eval") => cmd_eval(&args[2..]),
        Some("info") => cmd_info(&args[2..]),
        _ => {
            eprintln!("Zhenai Forge v0.1.0 — Custom LoRA Training");
            eprintln!();
            eprintln!("Usage:");
            eprintln!("  zhenai-forge info   <model.gguf>           Show model info");
            eprintln!("  zhenai-forge train  --model <model.gguf>   Train LoRA adapter");
            eprintln!("                      --data <train.jsonl>");
            eprintln!("                      --output <lora.gguf>");
            eprintln!("                      [--rank 16]");
            eprintln!("                      [--epochs 2]");
            eprintln!("                      [--lr 2e-4]");
            eprintln!("  zhenai-forge eval   --model <model.gguf>   Evaluate model");
            eprintln!("                      --lora <lora.gguf>");
            eprintln!("                      --data <eval.jsonl>");
            std::process::exit(1);
        }
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

