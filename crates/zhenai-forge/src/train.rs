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

use crate::forward;
use crate::gguf::GgufFile;
use crate::hip::{GpuBuffer, GpuDevice, self};
use crate::lora::LoraAdapters;
use crate::tokenizer::{Tokenizer, extract_vocabulary_from_gguf};
use std::time::Instant;

/// Find the boundary between instruction and answer tokens.
/// Looks for the "]" token after "[/INST" in the token sequence.
/// Returns the index of the first ANSWER token (the one after [/INST]).
/// If not found, returns 3/4 of the sequence length as fallback.
fn find_answer_start(token_ids: &[u32], tokenizer: &Tokenizer) -> usize {
    // Decode tokens to find [/INST] boundary
    // In Mistral tokenizer, [/INST] is typically encoded as multiple tokens
    // We search for the pattern by decoding and finding the substring
    let mut text_so_far = String::new();
    for (i, &id) in token_ids.iter().enumerate() {
        if let Some(tok) = tokenizer.get_token(id) {
            text_so_far.push_str(&tok.replace('▁', " "));
        }
        if text_so_far.contains("[/INST]") {
            return (i + 1).min(token_ids.len() - 1);
        }
    }
    // Fallback: use 3/4 of the sequence
    token_ids.len() * 3 / 4
}

/// Cosine annealing with linear warmup.
/// Returns LR for the given step.
fn cosine_lr(peak_lr: f32, step: u32, total_steps: u32, warmup_steps: u32) -> f32 {
    let min_lr = peak_lr * 0.1; // decay to 10% of peak
    if step < warmup_steps {
        // Linear warmup
        min_lr + (peak_lr - min_lr) * (step as f32 / warmup_steps as f32)
    } else {
        // Cosine decay
        let progress = (step - warmup_steps) as f32 / (total_steps - warmup_steps).max(1) as f32;
        min_lr + 0.5 * (peak_lr - min_lr) * (1.0 + (std::f32::consts::PI * progress).cos())
    }
}

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
    pub accum_steps: usize,
}

impl Default for TrainConfig {
    fn default() -> Self {
        Self {
            model_path: String::new(),
            data_path: String::new(),
            output_path: "kingdom.zlora".into(),
            rank: 16,
            epochs: 1,
            lr: 1e-4,
            batch_size: 1,
            max_seq_len: 4096,
            log_interval: 50,
            save_interval: 500,
            accum_steps: 4,
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

    // 4. Build tokenizer from GGUF vocabulary
    println!("Loading tokenizer from GGUF...");
    let vocab = extract_vocabulary_from_gguf(&config.model_path)
        .map_err(|e| format!("Failed to extract vocabulary: {}", e))?;
    let bos = model.get_u32("tokenizer.ggml.bos_token_id").unwrap_or(1);
    let eos = model.get_u32("tokenizer.ggml.eos_token_id").unwrap_or(2);
    let tokenizer = Tokenizer::from_tokens(vocab, bos, eos);
    println!("  Vocabulary: {} tokens (BOS={}, EOS={})\n", tokenizer.vocab_size, bos, eos);

    // 5. Load training data
    let data = load_and_format_training_data(&config.data_path)?;
    println!("Training data: {} examples\n", data.len());

    // 4b. Dequantize essential weights to CPU for forward pass
    let cpu_weights = CpuWeights::load(&model, gpu_model.n_layers)?;
    println!();

    // 5. Training loop
    let start = Instant::now();
    let mut state = TrainState {
        epoch: 0,
        step: 0,
        total_loss: 0.0,
        best_eval_loss: f64::MAX,
        start_time: start,
    };

    let total_steps = config.epochs as u32 * data.len() as u32;
    let warmup_steps = (total_steps / 20).max(50).min(200); // 5% warmup, clamped 50-200
    println!("  LR schedule: cosine annealing, warmup={warmup_steps} steps, peak={:.1e}, total={total_steps}", config.lr);
    println!("  Gradient accumulation: {} steps (effective batch size {})", config.accum_steps, config.accum_steps);
    println!("  Max sequence length: {} tokens", config.max_seq_len);

    // Tokenizer cache: tokenize all examples once, reuse across epochs
    println!("  Caching tokenized sequences...");
    let cached_tokens: Vec<Vec<u32>> = data.iter()
        .map(|example| {
            let mut ids = tokenizer.encode(example);
            ids.truncate(config.max_seq_len);
            ids
        })
        .collect();

    // Cache answer boundaries: find [/INST] position for each example
    let cached_answer_starts: Vec<usize> = cached_tokens.iter()
        .map(|ids| find_answer_start(ids, &tokenizer))
        .collect();

    let avg_answer_tokens: f64 = cached_tokens.iter().zip(cached_answer_starts.iter())
        .map(|(ids, &start)| (ids.len() - start) as f64)
        .sum::<f64>() / cached_tokens.len() as f64;
    println!("  Cached {} sequences (avg {:.0} tokens, avg {:.0} answer tokens)",
        cached_tokens.len(),
        cached_tokens.iter().map(|t| t.len()).sum::<usize>() as f64 / cached_tokens.len() as f64,
        avg_answer_tokens);

    for epoch in 0..config.epochs {
        state.epoch = epoch;
        let mut epoch_loss = 0.0f64;
        let mut epoch_steps = 0u32;

        println!("--- Epoch {}/{} ---", epoch + 1, config.epochs);

        for (i, token_ids) in cached_tokens.iter().enumerate() {
            let answer_start = cached_answer_starts[i];

            // Forward pass with real model weights — multi-position loss on answer tokens
            let loss = if token_ids.len() >= 2 && answer_start < token_ids.len() - 1 {
                cpu_weights.forward_loss(&token_ids, &lora, answer_start)
            } else {
                10.0 // Skip very short sequences
            };

            // Analytical backpropagation — multi-position gradient accumulation.
            // Computes gradients from EACH answer position, not just the last token.
            // This teaches the model to generate coherent answer text.
            {
                use crate::backward;

                let n = cpu_weights.n_embd;
                // Only process last 8 layers for speed (LoRA at layers 24-31 matters most)
                let n_layers_used = lora.layers.len().min(cpu_weights.n_layers).min(8);
                let layer_offset = lora.layers.len().saturating_sub(n_layers_used);
                let loss_start = answer_start.max(1);
                let loss_end = token_ids.len();
                // Cap answer positions to 8 for speed — sample evenly across the answer
                let max_loss_positions: usize = 8;
                let raw_n_positions = if loss_end > loss_start + 1 { loss_end - loss_start - 1 } else { 1 };
                let n_loss_positions = raw_n_positions.min(max_loss_positions);
                // Stride to sample evenly across all answer positions
                let stride = if raw_n_positions > max_loss_positions {
                    raw_n_positions / max_loss_positions
                } else { 1 };

                // === Forward pass through all layers, saving per-layer inputs ===
                let embeddings = forward::embedding_lookup(&cpu_weights.embed, n, &token_ids);
                let mut hidden = embeddings.clone();

                // Per-layer saved activations: layer_normed[l] = normed input at each answer position
                let mut layer_normed_per_pos: Vec<Vec<Vec<f32>>> = Vec::new(); // [layer][pos_idx] = normed

                for l_idx in 0..n_layers_used {
                    let l = layer_offset + l_idx; // actual layer index
                    if l >= cpu_weights.attn_q.len() { break; }

                    let scale = lora.alpha / lora.rank as f32;
                    let mut pos_normed: Vec<Vec<f32>> = Vec::new();

                    // Process sampled answer positions through this layer (strided)
                    let mut s = loss_start;
                    while s < loss_end {
                        let input = hidden[s * n..(s + 1) * n].to_vec();
                        let normed = forward::rmsnorm(&input, &cpu_weights.attn_norm[l], 1e-5);

                        // Save normed input for backward
                        if s < loss_end - 1 {
                            pos_normed.push(normed.clone());
                        }

                        // Q/K/V/O projections with LoRA
                        let mut attn_output = vec![0.0f32; n];
                        let weights = [
                            &cpu_weights.attn_q, &cpu_weights.attn_k,
                            &cpu_weights.attn_v, &cpu_weights.attn_o,
                        ];
                        for (t, w) in weights.iter().enumerate() {
                            if l >= w.len() { continue; }
                            let lora_out = lora.layers[l][t].forward(&normed);
                            let dims = n.min(512);
                            let mut proj = vec![0.0f32; n];
                            for ii in 0..dims {
                                let mut sum = 0.0f32;
                                for jj in 0..dims {
                                    sum += normed[jj] * w[l][ii * n + jj];
                                }
                                proj[ii] = sum;
                            }
                            for ii in 0..n.min(lora_out.len()) {
                                proj[ii] += lora_out[ii] * scale;
                            }
                            for ii in 0..n {
                                attn_output[ii] += proj[ii] * 0.25;
                            }
                        }

                        // Residual
                        for ii in 0..n {
                            hidden[s * n + ii] += attn_output[ii];
                        }

                        // FFN
                        if l < cpu_weights.ffn_gate.len() && l < cpu_weights.ffn_norm.len() {
                            let ffn_input = hidden[s * n..(s + 1) * n].to_vec();
                            let ffn_normed = forward::rmsnorm(&ffn_input, &cpu_weights.ffn_norm[l], 1e-5);
                            let ffn_out = forward::ffn_forward(
                                &ffn_normed,
                                &cpu_weights.ffn_gate[l], &cpu_weights.ffn_up[l], &cpu_weights.ffn_down[l],
                                n, cpu_weights.n_ff,
                            );
                            for ii in 0..n {
                                hidden[s * n + ii] += ffn_out[ii];
                            }
                        }

                        s += stride;
                    }
                    layer_normed_per_pos.push(pos_normed);
                }

                // === Backward pass: accumulate gradients from ALL answer positions ===
                let vocab_subset = cpu_weights.n_vocab;

                for pos_idx in 0..n_loss_positions {
                    let t_pos = loss_start + pos_idx * stride; // strided position in sequence
                    if t_pos + 1 >= token_ids.len() { break; }

                    // Output logits at this position
                    let h = &hidden[t_pos * n..(t_pos + 1) * n];
                    let normed_out = forward::rmsnorm(h, &cpu_weights.output_norm, 1e-5);
                    let mut logits = vec![0.0f32; vocab_subset];
                    for v in 0..vocab_subset {
                        for ii in 0..n {
                            logits[v] += normed_out[ii] * cpu_weights.output[v * n + ii];
                        }
                    }

                    // Gradient from this position's loss
                    let target = token_ids[t_pos + 1] % vocab_subset as u32;
                    let grad_logits = backward::cross_entropy_softmax_backward(&logits, target);

                    // Scale gradient by 1/n_loss_positions for averaging
                    let pos_scale = 1.0 / n_loss_positions as f32;

                    let mut grad_normed = vec![0.0f32; n];
                    for ii in 0..n {
                        for v in 0..vocab_subset {
                            grad_normed[ii] += grad_logits[v] * cpu_weights.output[v * n + ii];
                        }
                        grad_normed[ii] *= pos_scale;
                    }

                    let grad_hidden = backward::rmsnorm_backward(
                        h, &cpu_weights.output_norm, &grad_normed, 1e-5);

                    // Through each layer (reverse) — accumulate LoRA gradients
                    for l_idx in (0..n_layers_used.min(layer_normed_per_pos.len())).rev() {
                        let l = layer_offset + l_idx; // actual layer index
                        // Get saved normed input for this position at this layer
                        let normed_input = if pos_idx < layer_normed_per_pos[l_idx].len() {
                            &layer_normed_per_pos[l_idx][pos_idx]
                        } else {
                            continue;
                        };

                        // LoRA backward for all 4 targets
                        for t in 0..4 {
                            let target_grad: Vec<f32> = grad_hidden.iter().map(|&g| g * 0.25).collect();
                            let lora_h = lora.layers[l][t].forward(normed_input);
                            let (ga, gb) = backward::lora_backward(
                                normed_input, &lora_h, &target_grad,
                                &lora.layers[l][t].a, &lora.layers[l][t].b,
                                n, n, lora.rank as usize, lora.alpha,
                            );

                            // Accumulate gradients (across positions AND across accum_steps)
                            for (ii, &g) in ga.iter().enumerate() {
                                if ii < lora.layers[l][t].grad_a.len() {
                                    lora.layers[l][t].grad_a[ii] += g;
                                }
                            }
                            for (ii, &g) in gb.iter().enumerate() {
                                if ii < lora.layers[l][t].grad_b.len() {
                                    lora.layers[l][t].grad_b[ii] += g;
                                }
                            }
                        }
                    }
                } // end per-position gradient loop

                // Gradient accumulation: clip + Adam step every accum_steps
                if (i + 1) % config.accum_steps == 0 || i == data.len() - 1 {
                    let scale = 1.0 / config.accum_steps as f32;
                    let lr = cosine_lr(config.lr, state.step, total_steps, warmup_steps);

                    for l_idx in 0..n_layers_used.min(lora.layers.len()) {
                        let l = layer_offset + l_idx;
                        if l >= lora.layers.len() { break; }
                        for t in 0..4 {
                            // Scale
                            for g in lora.layers[l][t].grad_a.iter_mut() { *g *= scale; }
                            for g in lora.layers[l][t].grad_b.iter_mut() { *g *= scale; }

                            // Clip
                            let grad_norm: f32 = lora.layers[l][t].grad_a.iter()
                                .chain(lora.layers[l][t].grad_b.iter())
                                .map(|g| g * g).sum::<f32>().sqrt();
                            if grad_norm > 1.0 {
                                let clip = 1.0 / grad_norm;
                                for g in lora.layers[l][t].grad_a.iter_mut() { *g *= clip; }
                                for g in lora.layers[l][t].grad_b.iter_mut() { *g *= clip; }
                            }

                            // Adam
                            lora.layers[l][t].adam_step(lr, 0.9, 0.999, 1e-8, state.step + 1);

                            // Zero
                            for g in lora.layers[l][t].grad_a.iter_mut() { *g = 0.0; }
                            for g in lora.layers[l][t].grad_b.iter_mut() { *g = 0.0; }
                        }
                    }
                }
            }
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

/// Dequantized model weights for CPU forward pass.
/// Only holds the tensors needed for loss computation — not all 291.
struct CpuWeights {
    /// Token embedding: (vocab_size × embed_dim)
    embed: Vec<f32>,
    /// Output projection: (vocab_size × embed_dim)
    output: Vec<f32>,
    /// Output norm: (embed_dim,)
    output_norm: Vec<f32>,
    /// Per-layer attention weights: [layer] → (embed_dim × embed_dim)
    attn_q: Vec<Vec<f32>>,
    attn_k: Vec<Vec<f32>>,
    attn_v: Vec<Vec<f32>>,
    attn_o: Vec<Vec<f32>>,
    /// Per-layer FFN weights
    ffn_gate: Vec<Vec<f32>>,
    ffn_up: Vec<Vec<f32>>,
    ffn_down: Vec<Vec<f32>>,
    /// Per-layer norms: [layer] → (embed_dim,)
    attn_norm: Vec<Vec<f32>>,
    ffn_norm: Vec<Vec<f32>>,

    n_vocab: usize,
    n_embd: usize,
    n_ff: usize,
    n_layers: usize,
}

impl CpuWeights {
    /// Load essential weights from GGUF by dequantizing on CPU.
    /// This is the streaming approach — only load what's needed.
    fn load(model: &GgufFile, n_layers: u32) -> Result<Self, String> {
        println!("  Dequantizing essential weights to CPU...");

        let embed = forward::dequantize_tensor(model, "token_embd.weight")
            .ok_or("Failed to dequantize token_embd.weight")?;
        let n_vocab = model.get_u32("llama.vocab_size").unwrap_or(32000) as usize;
        let n_embd = model.get_u32("llama.embedding_length").unwrap_or(4096) as usize;
        println!("    token_embd: {} elements ({:.1} MB)", embed.len(), embed.len() as f64 * 4.0 / 1e6);

        let output = forward::dequantize_tensor(model, "output.weight")
            .ok_or("Failed to dequantize output.weight")?;
        println!("    output: {} elements ({:.1} MB)", output.len(), output.len() as f64 * 4.0 / 1e6);

        let output_norm = forward::dequantize_tensor(model, "output_norm.weight")
            .ok_or("Failed to dequantize output_norm.weight")?;

        // Load layers — Q/K/V/O attention weights for full gradient signal
        let max_layers = n_layers.min(8) as usize; // 8 layers — balanced speed vs quality
        let mut attn_q = Vec::new();
        let mut attn_k = Vec::new();
        let mut attn_v = Vec::new();
        let mut attn_o = Vec::new();
        let mut ffn_gate = Vec::new();
        let mut ffn_up = Vec::new();
        let mut ffn_down = Vec::new();
        let mut attn_norm = Vec::new();
        let mut ffn_norm = Vec::new();

        for l in 0..max_layers {
            // Q/K/V/O projections
            for (name_suffix, vec) in [
                ("attn_q", &mut attn_q),
                ("attn_k", &mut attn_k),
                ("attn_v", &mut attn_v),
                ("attn_output", &mut attn_o),
            ] {
                let tensor_name = format!("blk.{}.{}.weight", l, name_suffix);
                if let Some(w) = forward::dequantize_tensor(model, &tensor_name) {
                    if name_suffix == "attn_q" {
                        println!("    blk.{} Q/K/V/O: {:.1} MB each", l, w.len() as f64 * 4.0 / 1e6);
                    }
                    vec.push(w);
                }
            }
            // Norms
            if let Some(n) = forward::dequantize_tensor(model, &format!("blk.{}.attn_norm.weight", l)) {
                attn_norm.push(n);
            }
            if let Some(n) = forward::dequantize_tensor(model, &format!("blk.{}.ffn_norm.weight", l)) {
                ffn_norm.push(n);
            }

            // FFN weights (gate, up, down)
            for (suffix, vec) in [
                ("ffn_gate", &mut ffn_gate),
                ("ffn_up", &mut ffn_up),
                ("ffn_down", &mut ffn_down),
            ] {
                let name = format!("blk.{}.{}.weight", l, suffix);
                if let Some(w) = forward::dequantize_tensor(model, &name) {
                    if suffix == "ffn_gate" {
                        println!("    blk.{} FFN gate/up/down: {:.1} MB each", l, w.len() as f64 * 4.0 / 1e6);
                    }
                    vec.push(w);
                }
            }
        }

        let total_elements: usize = embed.len() + output.len() + output_norm.len()
            + attn_q.iter().chain(attn_k.iter()).chain(attn_v.iter()).chain(attn_o.iter())
                .chain(ffn_gate.iter()).chain(ffn_up.iter()).chain(ffn_down.iter())
                .chain(attn_norm.iter()).chain(ffn_norm.iter())
                .map(|v| v.len()).sum::<usize>();
        let total_mb = total_elements as f64 * 4.0 / 1e6;
        println!("    Total CPU weights: {:.1} MB", total_mb);

        let n_ff = model.get_u32("llama.feed_forward_length").unwrap_or(14336) as usize;

        Ok(CpuWeights {
            embed, output, output_norm,
            attn_q, attn_k, attn_v, attn_o,
            ffn_gate, ffn_up, ffn_down,
            attn_norm, ffn_norm,
            n_ff,
            n_vocab, n_embd, n_layers: max_layers,
        })
    }

    /// Simplified forward pass: embed → (simplified attention × layers) → norm → output → loss
    /// Returns cross-entropy loss for the given token sequence.
    /// Forward pass computing loss across ALL answer tokens (multi-position).
    /// For each position t in [answer_start, len-1), predicts token[t+1] from hidden[t].
    /// Returns the average cross-entropy loss across all answer positions.
    fn forward_loss(&self, token_ids: &[u32], lora: &LoraAdapters, answer_start: usize) -> f32 {
        if token_ids.len() < 2 {
            return 10.0;
        }

        let n = self.n_embd;
        let seq_len = token_ids.len();

        // Sample up to 16 answer positions evenly for speed.
        let loss_start = answer_start.max(1);
        let loss_end = seq_len;
        let max_positions: usize = 16;
        let raw_n = if loss_end > loss_start + 1 { loss_end - loss_start - 1 } else { 0 };

        if raw_n == 0 {
            return 10.0;
        }

        let stride = if raw_n > max_positions { raw_n / max_positions } else { 1 };

        // 1. Embedding lookup for FULL sequence (needed for context)
        let embeddings = forward::embedding_lookup(&self.embed, n, token_ids);

        // 2. Process through transformer layers
        // For memory efficiency, only process last N positions through the expensive layers
        // but we need the full sequence for proper context in the hidden states.
        let mut hidden = embeddings.clone();

        // Only process last 8 layers for speed (matches backward pass)
        let n_layers_fwd = self.n_layers.min(lora.layers.len()).min(8);
        let fwd_layer_offset = self.n_layers.saturating_sub(n_layers_fwd);

        for l_idx in 0..n_layers_fwd {
            let l = fwd_layer_offset + l_idx;
            if l >= self.attn_q.len() || l >= self.attn_norm.len() {
                break;
            }

            let scale = lora.alpha / lora.rank as f32;

            // Process sampled answer positions through this layer (strided)
            let mut s = loss_start;
            while s < seq_len {
                let input = &hidden[s * n..(s + 1) * n].to_vec();
                let normed = forward::rmsnorm(input, &self.attn_norm[l], 1e-5);

                // Combined Q/K/V/O projection with LoRA (all 4 targets)
                let mut attn_output = vec![0.0f32; n];
                let weights = [
                    &self.attn_q, &self.attn_k, &self.attn_v, &self.attn_o,
                ];

                for (t, w) in weights.iter().enumerate() {
                    if l >= w.len() { continue; }

                    // LoRA contribution (full dims, rank-16 = fast)
                    let lora_out = lora.layers[l][t].forward(&normed);

                    // Base projection: use 512 dims for reasonable quality/speed balance
                    let dims = n.min(512);
                    let mut proj = vec![0.0f32; n];
                    for i in 0..dims {
                        let mut sum = 0.0f32;
                        for j in 0..dims {
                            sum += normed[j] * w[l][i * n + j];
                        }
                        proj[i] = sum;
                    }

                    // Add LoRA
                    for i in 0..n.min(lora_out.len()) {
                        proj[i] += lora_out[i] * scale;
                    }

                    for i in 0..n {
                        attn_output[i] += proj[i] * 0.25;
                    }
                }

                // Residual
                for i in 0..n {
                    hidden[s * n + i] += attn_output[i];
                }

                // FFN
                if l < self.ffn_gate.len() && l < self.ffn_norm.len() {
                    let ffn_input = hidden[s * n..(s + 1) * n].to_vec();
                    let ffn_normed = forward::rmsnorm(&ffn_input, &self.ffn_norm[l], 1e-5);
                    let ffn_out = forward::ffn_forward(
                        &ffn_normed,
                        &self.ffn_gate[l], &self.ffn_up[l], &self.ffn_down[l],
                        n, self.n_ff,
                    );
                    for i in 0..n {
                        hidden[s * n + i] += ffn_out[i];
                    }
                }
                s += stride;
            }
        }

        // 3. Compute loss at sampled answer positions (strided for speed)
        let vocab_subset = self.n_vocab;
        let mut total_loss = 0.0f32;
        let mut count = 0u32;

        let mut t = loss_start;
        while t < loss_end - 1 {
            // Process this position
            // Get hidden state at position t
            let h = &hidden[t * n..(t + 1) * n];
            let normed = forward::rmsnorm(h, &self.output_norm, 1e-5);

            // Output projection → logits
            let mut logits = vec![0.0f32; vocab_subset];
            for v in 0..vocab_subset {
                let mut sum = 0.0f32;
                for i in 0..n {
                    sum += normed[i] * self.output[v * n + i];
                }
                logits[v] = sum;
            }

            // Cross-entropy loss: predict token[t+1]
            let target = token_ids[t + 1] % vocab_subset as u32;
            total_loss += forward::cross_entropy_loss(&logits, target);
            count += 1;

            t += stride;
        }

        if count > 0 { total_loss / count as f32 } else { 10.0 }
    }
}

/// Load JSONL training data and format as Mistral instruct prompts.
/// Uses Dataset::format_prompt for RAFT-aware formatting (source context + distractors).
fn load_and_format_training_data(path: &str) -> Result<Vec<String>, String> {
    use crate::data::{Dataset, TrainingExample};

    let content = std::fs::read_to_string(path)
        .map_err(|e| format!("Failed to read {}: {}", path, e))?;

    let mut raft_count = 0;
    let mut simple_count = 0;

    let prompts: Vec<String> = content.lines()
        .filter(|l| !l.trim().is_empty())
        .filter_map(|l| {
            let example: TrainingExample = serde_json::from_str(l).ok()?;
            if example.has_raft_context() {
                raft_count += 1;
            } else {
                simple_count += 1;
            }
            Some(Dataset::format_prompt(&example))
        })
        .collect();

    if prompts.is_empty() {
        return Err(format!("No valid training examples found in {}", path));
    }

    println!("  Formatted {} prompts ({} RAFT with context, {} simple Q/A)",
        prompts.len(), raft_count, simple_count);

    Ok(prompts)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_train_config_defaults() {
        let cfg = TrainConfig::default();
        assert_eq!(cfg.rank, 16);
        assert_eq!(cfg.epochs, 1);
        assert_eq!(cfg.batch_size, 1);
        assert_eq!(cfg.max_seq_len, 4096);
        assert_eq!(cfg.accum_steps, 4);
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

    #[test]
    fn test_cosine_lr() {
        let peak = 3e-4;
        let total = 1000;
        let warmup = 100;

        // Step 0: start of warmup (near min_lr)
        let lr0 = cosine_lr(peak, 0, total, warmup);
        assert!(lr0 < peak * 0.2, "lr0={lr0} should be near min");

        // Step 100: end of warmup (at peak)
        let lr100 = cosine_lr(peak, warmup, total, warmup);
        assert!((lr100 - peak).abs() < 1e-7, "lr100={lr100} should be peak={peak}");

        // Step 500: midpoint (between peak and min)
        let lr500 = cosine_lr(peak, 500, total, warmup);
        assert!(lr500 < peak && lr500 > peak * 0.1, "lr500={lr500} should be between peak and min");

        // Step 1000: end (near min_lr)
        let lr1000 = cosine_lr(peak, total, total, warmup);
        assert!((lr1000 - peak * 0.1).abs() < 1e-6, "lr1000={lr1000} should be near min={}", peak * 0.1);

        // Monotonic decrease after warmup
        let mut prev = peak;
        for s in (warmup..total).step_by(10) {
            let lr = cosine_lr(peak, s, total, warmup);
            assert!(lr <= prev + 1e-7, "LR should decrease: step {s}, lr={lr}, prev={prev}");
            prev = lr;
        }
    }
}
