// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//! Analytical backpropagation for LoRA training.
//!
//! Implements the backward pass (gradient computation) for:
//! - Cross-entropy loss → softmax
//! - Matrix multiply (∂C/∂A = grad_C × B^T, ∂C/∂B = A^T × grad_C)
//! - RMSNorm
//! - LoRA adapters (∂L/∂A, ∂L/∂B via chain rule)
//!
//! Only computes gradients for LoRA parameters (base weights frozen).
//! One forward + one backward gives ALL LoRA gradients simultaneously.
//! This is ~1000x faster than numerical gradient estimation.

/// Backward pass for cross-entropy loss + softmax.
/// Input: logits (pre-softmax), target token index
/// Output: gradient w.r.t. logits (same shape)
pub fn cross_entropy_softmax_backward(logits: &[f32], target: u32) -> Vec<f32> {
    // softmax(logits)
    let max = logits.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
    let mut probs: Vec<f32> = logits.iter().map(|&x| (x - max).exp()).collect();
    let sum: f32 = probs.iter().sum();
    for p in probs.iter_mut() {
        *p /= sum;
    }

    // Gradient: probs - one_hot(target)
    // ∂L/∂logits[i] = probs[i] - (1 if i == target else 0)
    let mut grad = probs;
    if (target as usize) < grad.len() {
        grad[target as usize] -= 1.0;
    }
    grad
}

/// Backward pass for matrix multiply C = A × B.
/// Given grad_C (gradient of loss w.r.t. C):
///   grad_A = grad_C × B^T
///   grad_B = A^T × grad_C
///
/// A: (m × k), B: (k × n), C: (m × n), grad_C: (m × n)
pub fn matmul_backward_a(grad_c: &[f32], b: &[f32], m: usize, n: usize, k: usize) -> Vec<f32> {
    // grad_A = grad_C × B^T, shape (m × k)
    let mut grad_a = vec![0.0f32; m * k];
    for i in 0..m {
        for j in 0..k {
            let mut sum = 0.0f32;
            for l in 0..n {
                sum += grad_c[i * n + l] * b[j * n + l]; // B^T[j][l] = B[l][j] but B is (k×n) so B^T[j][l] = B[j][l]...
                // Wait: B is (k × n), B^T is (n × k)
                // grad_C is (m × n), B^T is (n × k)
                // grad_A[i][j] = sum_l grad_C[i][l] * B^T[l][j] = sum_l grad_C[i][l] * B[j][l]
            }
            grad_a[i * k + j] = sum;
        }
    }
    grad_a
}

pub fn matmul_backward_b(a: &[f32], grad_c: &[f32], m: usize, n: usize, k: usize) -> Vec<f32> {
    // grad_B = A^T × grad_C, shape (k × n)
    let mut grad_b = vec![0.0f32; k * n];
    for i in 0..k {
        for j in 0..n {
            let mut sum = 0.0f32;
            for l in 0..m {
                // A^T[i][l] = A[l][i]
                sum += a[l * k + i] * grad_c[l * n + j];
            }
            grad_b[i * n + j] = sum;
        }
    }
    grad_b
}

/// Backward pass for LoRA: output = (B × A) × input, scaled by alpha/rank.
/// Given grad_output (from downstream):
///   grad_A = B^T × (grad_output × input^T) × (alpha/rank)
///   grad_B = (grad_output × input^T) × A^T × (alpha/rank)
///
/// Simplified for rank-16 LoRA:
///   hidden = A × input  (rank × 1)
///   output = B × hidden (dim × 1)
///   grad_B = grad_output × hidden^T
///   grad_hidden = B^T × grad_output
///   grad_A = grad_hidden × input^T
pub fn lora_backward(
    input: &[f32],      // (input_dim,)
    hidden: &[f32],     // (rank,) — A × input
    grad_output: &[f32], // (output_dim,)
    a: &[f32],          // (input_dim × rank)
    b: &[f32],          // (rank × output_dim)
    input_dim: usize,
    output_dim: usize,
    rank: usize,
    alpha: f32,
) -> (Vec<f32>, Vec<f32>) {
    let scale = alpha / rank as f32;

    // grad_B = grad_output (outer product) hidden^T, shape (rank × output_dim)
    let mut grad_b = vec![0.0f32; rank * output_dim];
    for r in 0..rank {
        for o in 0..output_dim {
            grad_b[r * output_dim + o] = grad_output[o] * hidden[r] * scale;
        }
    }

    // grad_hidden = B^T × grad_output, shape (rank,)
    let mut grad_hidden = vec![0.0f32; rank];
    for r in 0..rank {
        let mut sum = 0.0f32;
        for o in 0..output_dim {
            sum += b[r * output_dim + o] * grad_output[o];
        }
        grad_hidden[r] = sum * scale;
    }

    // grad_A = grad_hidden (outer product) input^T, shape (input_dim × rank)
    let mut grad_a = vec![0.0f32; input_dim * rank];
    for d in 0..input_dim {
        for r in 0..rank {
            grad_a[d * rank + r] = grad_hidden[r] * input[d];
        }
    }

    (grad_a, grad_b)
}

/// Backward pass for RMSNorm.
/// Given grad_output and the original input + weight:
///   Returns grad_input
pub fn rmsnorm_backward(input: &[f32], weight: &[f32], grad_output: &[f32], eps: f32) -> Vec<f32> {
    let n = input.len();
    let ss: f32 = input.iter().map(|x| x * x).sum::<f32>() / n as f32;
    let rms = (ss + eps).sqrt();
    let rms_inv = 1.0 / rms;

    // ∂L/∂input[i] = weight[i] * (grad_output[i] / rms - input[i] * sum(grad_output * weight * input) / (n * rms^3))
    let sum_gwi: f32 = (0..n).map(|i| grad_output[i] * weight[i] * input[i]).sum();

    let mut grad_input = vec![0.0f32; n];
    for i in 0..n {
        grad_input[i] = weight[i] * rms_inv * grad_output[i]
            - weight[i] * input[i] * sum_gwi / (n as f32 * rms * rms * rms);
    }
    grad_input
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_cross_entropy_backward() {
        let logits = vec![1.0, 5.0, 1.0];
        let grad = cross_entropy_softmax_backward(&logits, 1);

        // Gradient for correct class should be negative (softmax(1) - 1)
        assert!(grad[1] < 0.0, "Gradient for correct class should be negative");
        // Gradients for wrong classes should be positive (softmax value)
        assert!(grad[0] > 0.0);
        assert!(grad[2] > 0.0);
        // Sum of gradients should be ~0 (softmax sums to 1, minus 1 for target)
        let sum: f32 = grad.iter().sum();
        assert!((sum).abs() < 1e-5, "Gradient sum should be ~0, got {}", sum);
    }

    #[test]
    fn test_matmul_backward() {
        // C = A × B, A=(2×3), B=(3×2)
        let a = vec![1.0, 2.0, 3.0, 4.0, 5.0, 6.0];
        let b = vec![1.0, 0.0, 0.0, 1.0, 1.0, 1.0];
        let grad_c = vec![1.0, 0.0, 0.0, 1.0]; // Identity-like gradient

        let grad_a = matmul_backward_a(&grad_c, &b, 2, 2, 3);
        let grad_b = matmul_backward_b(&a, &grad_c, 2, 2, 3);

        assert_eq!(grad_a.len(), 6); // (2×3)
        assert_eq!(grad_b.len(), 6); // (3×2)

        // grad_A = grad_C × B^T = [[1,0],[0,1]] × [[1,0,1],[0,1,1]] = [[1,0,1],[0,1,1]]
        assert!((grad_a[0] - 1.0).abs() < 0.01);
        assert!((grad_a[1] - 0.0).abs() < 0.01);
        assert!((grad_a[2] - 1.0).abs() < 0.01);
    }

    #[test]
    fn test_lora_backward() {
        let input = vec![1.0, 0.5, 0.25, 0.1];
        let a = vec![0.1; 4 * 2]; // (4 × 2)
        let b = vec![0.1; 2 * 4]; // (2 × 4)

        // Forward: hidden = A × input
        let hidden = vec![
            a[0] * input[0] + a[1] * input[1] + a[2] * input[2] + a[3] * input[3],
            a[4] * input[0] + a[5] * input[1] + a[6] * input[2] + a[7] * input[3],
        ];

        let grad_output = vec![1.0, 0.0, 0.0, 0.0]; // Gradient only on first output

        let (grad_a, grad_b) = lora_backward(&input, &hidden, &grad_output, &a, &b, 4, 4, 2, 32.0);

        assert_eq!(grad_a.len(), 8); // (4 × 2)
        assert_eq!(grad_b.len(), 8); // (2 × 4)

        // grad_b[0] should be non-zero (grad_output[0] * hidden[0])
        assert!(grad_b[0].abs() > 0.0, "grad_b should be non-zero");
    }

    #[test]
    fn test_rmsnorm_backward() {
        let input = vec![1.0, 2.0, 3.0, 4.0];
        let weight = vec![1.0, 1.0, 1.0, 1.0];
        let grad_output = vec![1.0, 0.0, 0.0, 0.0]; // Gradient only on first element

        let grad_input = rmsnorm_backward(&input, &weight, &grad_output, 1e-5);
        assert_eq!(grad_input.len(), 4);
        // First element gradient should be largest (direct path)
        assert!(grad_input[0].abs() > grad_input[3].abs());
    }
}
