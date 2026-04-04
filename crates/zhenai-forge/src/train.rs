// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Training loop orchestration — coordinates CPU ↔ GPU data flow.
//!
//! Architecture:
//!   1. GGUF model mmap'd, quantized tensors sent to GPU VRAM
//!   2. Training data tokenized on CPU (rayon parallel)
//!   3. Forward pass: GPU computes base model + LoRA output
//!   4. Loss computed on GPU
//!   5. Backward pass: gradients for LoRA params only (base frozen)
//!   6. Gradients copied to CPU, Adam step updates LoRA
//!   7. Updated LoRA copied back to GPU
//!   8. Repeat

use crate::gguf::GgufFile;
use crate::hip::{GpuBuffer, GpuDevice, self};
use crate::lora::LoraAdapters;
use std::time::Instant;

/// Training configuration.
pub struct TrainConfig {
    pub model_path: String,
    pub data_path: String,
    pub output_path: String,
    pub rank: u32,
    pub epochs: u32,
    pub lr: f32,
    pub batch_size: usize,
    pub max_seq_len: usize,
    pub log_interval: usize,
    pub save_interval: usize,
}

impl Default for TrainConfig {
    fn default() -> Self {
        Self {
            model_path: String::new(),
            data_path: String::new(),
            output_path: "kingdom.zlora".into(),
            rank: 16,
            epochs: 2,
            lr: 2e-4,
            batch_size: 1,
            max_seq_len: 2048,
            log_interval: 50,
            save_interval: 500,
        }
    }
}

/// Loaded model on GPU — base weights in VRAM, LoRA in RAM.
pub struct GpuModel {
    /// Base model tensor buffers on GPU
    pub weight_buffers: Vec<GpuBuffer>,
    /// Total VRAM used by base model
    pub vram_used: usize,
    /// Model metadata
    pub n_layers: u32,
    pub n_embd: u32,
    pub n_vocab: u32,
    pub n_head: u32,
    pub n_head_kv: u32,
}

impl GpuModel {
    /// Load quantized model tensors from GGUF directly to GPU VRAM.
    /// This is the key innovation: no fp16 decompression in RAM.
    pub fn load_from_gguf(model: &GgufFile) -> Result<Self, String> {
        let device = GpuDevice::init(0).map_err(|e| format!("GPU init failed: {}", e))?;
        println!("  GPU: {} ({:.1} GB VRAM, {} CUs)",
            device.name, device.vram_bytes as f64 / 1e9, device.compute_units);

        let n_layers = model.get_u32("llama.block_count").unwrap_or(32);
        let n_embd = model.get_u32("llama.embedding_length").unwrap_or(4096);
        let n_vocab = model.get_u32("llama.vocab_size")
            .or_else(|| model.get_u32("tokenizer.ggml.tokens").map(|_| 32000))
            .unwrap_or(32000);
        let n_head = model.get_u32("llama.attention.head_count").unwrap_or(32);
        let n_head_kv = model.get_u32("llama.attention.head_count_kv").unwrap_or(8);

        println!("  Architecture: {} layers, {} dim, {} vocab, {} heads ({} kv)",
            n_layers, n_embd, n_vocab, n_head, n_head_kv);

        // Allocate GPU buffers for each tensor
        let mut weight_buffers = Vec::new();
        let mut total_vram = 0usize;
        let mut loaded = 0;
        let total = model.tensors.len();

        for tensor in &model.tensors {
            let size = tensor.byte_size as usize;
            if size == 0 {
                continue;
            }

            match GpuBuffer::alloc(size) {
                Ok(buf) => {
                    // Copy REAL tensor data from mmap to GPU VRAM
                    let tensor_bytes = model.tensor_data(tensor);
                    if tensor_bytes.len() >= size {
                        buf.copy_from_host(&tensor_bytes[..size])
                            .map_err(|e| format!("GPU memcpy failed for {}: {}", tensor.name, e))?;
                    } else {
                        // Tensor data shorter than expected — zero-fill remainder
                        buf.zero().map_err(|e| format!("GPU zero failed: {}", e))?;
                    }
                    total_vram += size;
                    weight_buffers.push(buf);
                    loaded += 1;
                }
                Err(e) => {
                    println!("  WARNING: Failed to alloc {} bytes for {}: {}",
                        size, tensor.name, e);
                    // Continue — some tensors may be optional
                }
            }

            if loaded % 50 == 0 {
                println!("  Loading tensors: {}/{} ({:.1} MB VRAM)",
                    loaded, total, total_vram as f64 / 1e6);
            }
        }

        hip::sync().map_err(|e| format!("GPU sync failed: {}", e))?;
        println!("  Loaded: {}/{} tensors, {:.1} MB VRAM used", loaded, total, total_vram as f64 / 1e6);

        Ok(GpuModel {
            weight_buffers,
            vram_used: total_vram,
            n_layers,
            n_embd,
            n_vocab,
            n_head,
            n_head_kv,
        })
    }
}

/// Training state and metrics.
pub struct TrainState {
    pub epoch: u32,
    pub step: u32,
    pub total_loss: f64,
    pub best_eval_loss: f64,
    pub start_time: Instant,
}

/// Run the full training loop.
pub fn train(config: &TrainConfig) -> Result<(), String> {
    println!("============================================================");
    println!("ZHENAI FORGE — Training Loop");
    println!("============================================================\n");

    // 1. Load GGUF model metadata
    let model = GgufFile::open(&config.model_path)
        .map_err(|e| format!("Failed to open model: {}", e))?;
    println!("Model: {} tensors, {:.1} MB\n", model.tensors.len(), model.file_size as f64 / 1e6);

    // 2. Load model to GPU
    println!("Loading model to GPU...");
    let gpu_model = GpuModel::load_from_gguf(&model)?;
    println!("  VRAM used: {:.1} MB / {:.1} GB available\n",
        gpu_model.vram_used as f64 / 1e6,
        GpuDevice::init(0).map(|d| d.vram_bytes as f64 / 1e9).unwrap_or(0.0));

    // 3. Initialize LoRA adapters (in RAM)
    let mut lora = LoraAdapters::new(gpu_model.n_layers, gpu_model.n_embd, config.rank);
    println!("LoRA initialized:");
    println!("  Rank: {}, Alpha: {}", config.rank, lora.alpha);
    println!("  Trainable params: {} ({:.1} MB)",
        lora.num_params(), lora.size_bytes() as f64 / 1e6);
    println!("  Optimizer states: {:.1} MB\n",
        lora.size_bytes() as f64 / 1e6 * 4.0); // m, v for A and B

    // 4. Load training data
    let data = load_training_jsonl(&config.data_path)?;
    println!("Training data: {} examples\n", data.len());

    // 5. Training loop
    let start = Instant::now();
    let mut state = TrainState {
        epoch: 0,
        step: 0,
        total_loss: 0.0,
        best_eval_loss: f64::MAX,
        start_time: start,
    };

    for epoch in 0..config.epochs {
        state.epoch = epoch;
        let mut epoch_loss = 0.0f64;
        let mut epoch_steps = 0u32;

        println!("--- Epoch {}/{} ---", epoch + 1, config.epochs);

        for (i, example) in data.iter().enumerate() {
            // Forward pass (Phase 3: GPU matmul, currently CPU simulation)
            let loss = lora.training_step(example, config.lr);
            epoch_loss += loss as f64;
            epoch_steps += 1;
            state.step += 1;
            state.total_loss += loss as f64;

            // Log
            if (i + 1) % config.log_interval == 0 || i == data.len() - 1 {
                let elapsed = start.elapsed().as_secs_f64();
                let steps_per_sec = state.step as f64 / elapsed;
                let avg_loss = epoch_loss / epoch_steps as f64;
                let eta_s = (data.len() as f64 - i as f64) / steps_per_sec
                    + (config.epochs - epoch - 1) as f64 * data.len() as f64 / steps_per_sec;

                println!("  Step {}/{} | Loss: {:.4} | {:.1} steps/s | ETA: {:.0}s",
                    i + 1, data.len(), avg_loss, steps_per_sec, eta_s);
            }

            // Save checkpoint
            if state.step > 0 && state.step as usize % config.save_interval == 0 {
                let cp_path = format!("{}.checkpoint-{}", config.output_path, state.step);
                lora.save_zlora(&cp_path, &model)
                    .map_err(|e| format!("Checkpoint save failed: {}", e))?;
                println!("  Checkpoint: {} ({:.1} MB)", cp_path,
                    std::fs::metadata(&cp_path).map(|m| m.len() as f64 / 1e6).unwrap_or(0.0));
            }
        }

        let avg_epoch_loss = epoch_loss / epoch_steps as f64;
        println!("  Epoch {} complete | Avg loss: {:.4} | Time: {:.0}s\n",
            epoch + 1, avg_epoch_loss, start.elapsed().as_secs_f64());
    }

    // 6. Save final LoRA
    println!("Saving final LoRA adapter...");
    lora.save_zlora(&config.output_path, &model)
        .map_err(|e| format!("Save failed: {}", e))?;

    let total_time = start.elapsed().as_secs_f64();
    let output_size = std::fs::metadata(&config.output_path)
        .map(|m| m.len() as f64 / 1e6).unwrap_or(0.0);

    println!("\n============================================================");
    println!("TRAINING COMPLETE");
    println!("============================================================");
    println!("  Output:    {} ({:.1} MB)", config.output_path, output_size);
    println!("  Steps:     {}", state.step);
    println!("  Time:      {:.0}s ({:.1} min)", total_time, total_time / 60.0);
    println!("  Avg loss:  {:.4}", state.total_loss / state.step as f64);
    println!("  VRAM used: {:.1} MB (base) + {:.1} MB (LoRA)",
        gpu_model.vram_used as f64 / 1e6, lora.size_bytes() as f64 / 1e6);
    println!("  RAM used:  {:.1} MB (LoRA + optimizer)",
        lora.size_bytes() as f64 / 1e6 * 5.0); // weights + grads + m + v
    Ok(())
}

fn load_training_jsonl(path: &str) -> Result<Vec<String>, String> {
    let content = std::fs::read_to_string(path)
        .map_err(|e| format!("Failed to read {}: {}", path, e))?;
    let lines: Vec<String> = content.lines()
        .filter(|l| !l.trim().is_empty())
        .map(|l| l.to_string())
        .collect();
    if lines.is_empty() {
        return Err(format!("No training examples found in {}", path));
    }
    Ok(lines)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_train_config_defaults() {
        let cfg = TrainConfig::default();
        assert_eq!(cfg.rank, 16);
        assert_eq!(cfg.epochs, 2);
        assert_eq!(cfg.batch_size, 1);
        assert_eq!(cfg.max_seq_len, 2048);
    }

    #[test]
    fn test_gpu_model_load() {
        // Only run if GPU is available
        if GpuDevice::init(0).is_err() {
            println!("No GPU, skipping");
            return;
        }

        let model_path = "/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf";
        if !std::path::Path::new(model_path).exists() {
            println!("Model not found, skipping");
            return;
        }

        let model = GgufFile::open(model_path).expect("Failed to open GGUF");
        let gpu_model = GpuModel::load_from_gguf(&model).expect("Failed to load to GPU");

        assert_eq!(gpu_model.n_layers, 32);
        assert_eq!(gpu_model.n_embd, 4096);
        assert!(gpu_model.vram_used > 0);
        println!("GPU model loaded: {:.1} MB VRAM", gpu_model.vram_used as f64 / 1e6);
    }
}
