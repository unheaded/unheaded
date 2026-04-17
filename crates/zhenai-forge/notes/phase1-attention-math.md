# Phase 1 Attention Math — Forward + Backward Derivations

**Plan:** WAVE10F Phase 1 — vanilla GQA real attention forward+backward. Test bed: Mistral-7B (n_heads=32, n_kv_heads=8, head_dim=128) at restricted seq length.

This doc derives the analytical gradients forge needs. Every derivation here gets a numerical-gradient `#[test]` in `backward.rs` once implementation lands.

Notation:
- `[B, S, H, D]` = batch, sequence, head, head_dim
- `Q ∈ [S, H_q, D]`, `K ∈ [S, H_kv, D]`, `V ∈ [S, H_kv, D]` (pre-GQA-expand)
- After GQA expand: `K_e, V_e ∈ [S, H_q, D]` where each KV head is repeated `H_q/H_kv` times
- Attention scores `A ∈ [H_q, S, S]`
- Output `O ∈ [S, H_q, D]`
- Then projected: `O_flat ∈ [S, H_q*D]`, `Y = O_flat @ W_o`, `Y ∈ [S, n_embd]`

We derive backward as if forward was:

```
1. Q_rot = rope(Q, freqs)
   K_rot = rope(K_e, freqs)            (after GQA expand or before — equivalent)
2. scores_pre = Q_rot @ K_rot^T / sqrt(D)        ∈ [H_q, S, S]
3. scores_masked = scores_pre + mask              (additive mask: 0 in-window, -inf outside)
4. A = softmax(scores_masked, dim=-1)
5. O = A @ V_e                                    ∈ [S, H_q, D]
6. Y = O.reshape(S, H_q*D) @ W_o^T               ∈ [S, n_embd]
```

(Mask is additive in scores-space, multiplicative-zero in attention-space — both equivalent. We use additive `+(-inf)` outside window so softmax → 0.)

For Phase 1 (Mistral, no sliding window): mask is just causal `{j > i: -inf, else: 0}`.

## 1. Softmax backward (Jacobian-Vector Product)

For row-wise softmax `p = softmax(z)`, `p_i = exp(z_i)/sum_k exp(z_k)`:

`∂p_i/∂z_j = p_i (δ_ij - p_j)`

Given upstream `g = ∂L/∂p`, the gradient w.r.t. `z` is:

`∂L/∂z_i = sum_j (∂L/∂p_j)(∂p_j/∂z_i) = p_i (g_i - sum_j g_j p_j)`

**Vector form (per row):**
```
dL/dz = p .* (g - dot(g, p))
```

where `dot(g, p) = sum_j g_j p_j` is a scalar per row.

**Implementation (`backward::softmax_backward`):**
```rust
// p, g, out: shape [..., S]; per-row operation
for each row:
    s = dot(g, p)
    out = p * (g - s)
```

**Numerical check:** finite-diff `softmax(z + eps*e_i) - softmax(z - eps*e_i)` along each axis, compare to analytical Jacobian. Threshold rel_err < 0.1.

## 2. Attention backward (chain through @V, softmax, scale, @K^T)

Given upstream `dO ∈ [S, H_q, D]` (gradient w.r.t. attention output before O projection).

### 2a. Backward through `O = A @ V_e`

```
dA  = dO @ V_e^T              ∈ [H_q, S, S]
dV_e = A^T @ dO               ∈ [S, H_q, D]
```

Per head h:
```
dA[h]   = dO[:, h, :] @ V_e[:, h, :]^T          # [S, S]
dV_e[h] = A[h]^T @ dO[:, h, :]                  # [S, D]
```

### 2b. Backward through softmax (uses §1)

```
d_scores_masked = softmax_backward(dA, A)       # [H_q, S, S]
```

### 2c. Backward through mask

Mask is additive in scores-space; gradient passes through unchanged for in-window positions. Out-of-window positions had score → -inf → softmax = 0 → gradient already 0 from §2b. No-op:

```
d_scores_pre = d_scores_masked
```

(For Phase 3 sliding window: same logic — softmax zeros propagate naturally.)

### 2d. Backward through scale `scores_pre = (Q_rot @ K_rot^T) / sqrt(D)`

```
d_QK = d_scores_pre / sqrt(D)                   # [H_q, S, S]
```

### 2e. Backward through Q_rot @ K_rot^T

Per head h:
```
dQ_rot[:, h, :] = d_QK[h] @ K_rot[:, h, :]      # [S, D]
dK_rot[:, h, :] = d_QK[h]^T @ Q_rot[:, h, :]    # [S, D]
```

(After GQA expand `K_rot` has H_q heads; collapse comes in §3.)

### 2f. Backward through RoPE

RoPE applies a 2D rotation per (even,odd) dim pair, parameterized by complex `freqs[s] = cos(θ_s) + i sin(θ_s)`:

```
[x_even', x_odd'] = [x_even * cos - x_odd * sin,
                     x_even * sin + x_odd * cos]
```

Backward — apply inverse rotation (transpose for orthogonal):
```
[dx_even, dx_odd] = [dx_even' * cos + dx_odd' * sin,
                     -dx_even' * sin + dx_odd' * cos]
```

**Implementation (`backward::rope_backward`):**
```rust
// x_grad_rotated: [..., D]; freqs: [S, D/2]; out: [..., D]
for each (even, odd) pair indexed by d/2 and position s:
    cos_θ, sin_θ = freqs[s, d/2]
    out[2*d/2]   =  x_grad_rotated[2*d/2] * cos_θ + x_grad_rotated[2*d/2+1] * sin_θ
    out[2*d/2+1] = -x_grad_rotated[2*d/2] * sin_θ + x_grad_rotated[2*d/2+1] * cos_θ
```

Apply to `dQ_rot → dQ` and `dK_rot → dK` independently.

**Numerical check:** finite-diff RoPE forward at single (s, d/2) pair; verify analytical backward matches.

## 3. GQA collapse backward

Forward `gqa_expand(K, V, n_heads, n_kv_heads)` repeats each KV head `n_heads/n_kv_heads` times. Backward sums:

```
group_size = n_heads / n_kv_heads
for h_kv in 0..n_kv_heads:
    dK[:, h_kv, :] = sum over h in [h_kv*group_size, (h_kv+1)*group_size) of dK_e[:, h, :]
    dV[:, h_kv, :] same
```

For Mistral n_heads=32, n_kv_heads=8: group_size=4. Each KV head receives 4 query heads' worth of gradient.

**Implementation (`backward::gqa_collapse`):**
```rust
// dK_e: [S, H_q, D]; dK: [S, H_kv, D]; group_size = H_q / H_kv
for s in 0..S:
    for h_kv in 0..H_kv:
        for h_in_group in 0..group_size:
            h_q = h_kv * group_size + h_in_group
            dK[s, h_kv, :] += dK_e[s, h_q, :]
```

**Numerical check:** synthetic dK_e with known per-head values, verify sum.

## 4. Output projection backward

Forward `Y = O_flat @ W_o^T` where `O_flat = O.reshape(S, H_q*D)`.

Given upstream `dY ∈ [S, n_embd]`:
```
dO_flat = dY @ W_o                    ∈ [S, H_q*D]
dW_o    = dY^T @ O_flat               ∈ [n_embd, H_q*D]
dO      = dO_flat.reshape(S, H_q, D)
```

`dW_o` accumulates into the LoRA gradients via standard `lora_backward(input=O_flat, hidden=lora_h, grad_output=dY, ...)` — already implemented.

## 5. Q/K/V projection backward

Forward `Q = X_norm @ W_q^T` (similarly K, V). Given `dQ ∈ [S, H_q*D]`:
```
dX_from_Q = dQ @ W_q                  ∈ [S, n_embd]
dW_q      = dQ^T @ X_norm             ∈ [H_q*D, n_embd]
```

LoRA gradient via existing `lora_backward(input=X_norm, hidden=lora_h, grad_output=dQ, ...)` — for Q, K, V independently.

**Total `dX_norm`** sums contributions from Q, K, V paths plus the residual:
```
dX_norm = dX_from_Q + dX_from_K + dX_from_V
dX = rmsnorm_backward(X, attn_norm, dX_norm) + residual_dY
```

## 6. End-to-end one-layer chain (sanity)

```
Y = X + attn(rmsnorm(X))

Forward:
  X_norm = rmsnorm(X)
  Q = X_norm @ W_q^T;  K = X_norm @ W_k^T;  V = X_norm @ W_v^T
  Q_rot = rope(Q);  K_rot = rope(K)
  K_e, V_e = gqa_expand(K_rot, V)
  A = softmax((Q_rot @ K_e^T) / sqrt(D) + mask)
  O = A @ V_e
  Y = X + O.reshape(...) @ W_o^T

Backward (given dY):
  # residual
  dX_residual = dY
  
  # output projection
  dO_flat = dY @ W_o
  dW_o    = dY^T @ O_flat
  dO      = dO_flat.reshape(S, H_q, D)
  
  # attention output @ V
  dA   = dO @ V_e^T       (per head)
  dV_e = A^T @ dO         (per head)
  
  # softmax + scale + Q@K^T
  d_scores = softmax_backward(dA, A) / sqrt(D)
  dQ_rot   = d_scores @ K_e
  dK_e     = d_scores^T @ Q_rot
  
  # GQA collapse
  dK_rot, dV = gqa_collapse(dK_e, dV_e)
  
  # RoPE
  dQ = rope_backward(dQ_rot)
  dK = rope_backward(dK_rot)
  
  # Q/K/V projections
  dX_from_Q = dQ @ W_q;  dW_q = dQ^T @ X_norm  → LoRA grad
  dX_from_K = dK @ W_k;  dW_k = dK^T @ X_norm  → LoRA grad
  dX_from_V = dV @ W_v;  dW_v = dV^T @ X_norm  → LoRA grad
  
  # residual + RMSNorm backward
  dX_norm = dX_from_Q + dX_from_K + dX_from_V
  dX = rmsnorm_backward(X, attn_norm, dX_norm) + dX_residual
```

This replaces the simplified `attn_only_layer_backward` from `backward.rs:281-323`.

## 7. Numerical gradient check skeleton

Pattern from `test_rmsnorm_backward_numerical` (`backward.rs:512`):

```rust
#[test]
fn test_attention_backward_numerical() {
    let s = 4; let h_q = 2; let h_kv = 1; let d = 8;
    // Random Q, K, V, mask
    let q = random_vec(s * h_q * d);
    let k = random_vec(s * h_kv * d);
    let v = random_vec(s * h_kv * d);
    let mask = causal_mask(s);
    
    // Forward + analytical backward
    let (out, scores) = attention_forward(&q, &k, &v, h_q, h_kv, d, &mask);
    let upstream = random_vec(out.len());
    let (gq, gk, gv) = attention_backward(&upstream, &q, &k, &v, &scores, h_q, h_kv, d);
    
    // Numerical gradient via central finite-diff on Q (and K, V independently)
    let eps = 1e-3;
    for i in 0..q.len() {
        let mut q_plus  = q.clone(); q_plus[i] += eps;
        let mut q_minus = q.clone(); q_minus[i] -= eps;
        let (out_plus,  _) = attention_forward(&q_plus,  &k, &v, h_q, h_kv, d, &mask);
        let (out_minus, _) = attention_forward(&q_minus, &k, &v, h_q, h_kv, d, &mask);
        let numerical: f32 = out_plus.iter().zip(out_minus.iter()).zip(upstream.iter())
            .map(|((p, m), u)| (p - m) * u).sum::<f32>() / (2.0 * eps);
        let analytical = gq[i];
        let rel_err = (numerical - analytical).abs() / numerical.abs().max(1e-6);
        assert!(rel_err < 0.1, "Q[{}]: analytical={} numerical={} rel_err={}", i, analytical, numerical, rel_err);
    }
    // Repeat for K, V.
}
```

Same template applies to softmax, gqa_collapse, RoPE backward.

## 8. Memory budget — Phase 1 attention scores cache

Mistral-7B forward pass at restricted `seq_len=256`:
- Per layer: scores `[H_q=32, S=256, S=256]` = 32 × 256 × 256 × 4 bytes (fp32) = **8 MB/layer**
- Cache as bf16: 4 MB/layer
- 32 layers × 4 MB = **128 MB** total scores cache (bf16)

Plus saved Q_rot, K_rot, V per layer for backward:
- Per layer: 3 × [S, H_q, D] = 3 × 256 × 32 × 128 × 2 bytes (bf16) = **6 MB/layer** (Q_rot full); K_rot/V at GQA-pre-expand = 3 × 256 × (32 + 8 + 8) × 128 × 2 ≈ 7.5 MB/layer
- 32 layers × ~7.5 MB = **240 MB**

Total Phase 1 attention forward cache: **~370 MB** at bf16, well under west's headroom.

If we restrict further to seq_len=128: 4× memory savings → ~90 MB. Plenty of margin to scale up gradually.

## 9. Implementation order in Phase 1

1. `forward::attention_forward` (CPU, returns scores cache)
2. `backward::softmax_backward`
3. `backward::rope_backward`
4. `backward::gqa_collapse` (forward + backward both needed)
5. `backward::attention_backward` (composes 2-4)
6. `forward::rope_apply` (forward used by attention_forward)
7. `forward::gelu_tanh_approx` + `backward::gelu_tanh_approx_backward` (Phase 3 needs these but also Mistral; Mistral uses SiLU which we have)
8. Wire into `train.rs` per-layer loop replacing `attn_only_layer_backward`
9. Add `FORGE_REAL_ATTENTION=1` env gate so we can A/B old vs new during transition
10. Numerical gradient tests for each new function (in same file as the function)

GPU-acceleration of the attention scores matmul + softmax + scaled-V matmul = Phase 1.5 if Phase 1 CPU is too slow. For correctness-first development, CPU is fine.

## 10. Out of scope for Phase 1

- Sliding-window mask (Phase 3)
- p-RoPE (Phase 4)
- Unified K/V (Phase 4)
- PLE (Phase 5)
- Logit softcapping (Phase 3 — implement when Gemma 4 GGUF loads)
- gelu_tanh_approx (Phase 3 — Mistral uses SiLU which we have)
- MoE (out of plan entirely; E2B is dense)

---

*Math notes written 2026-04-17 during Phase 0 background-build wait, prior to Phase 1 implementation. Verify against numerical gradient checks before trusting any line.*
