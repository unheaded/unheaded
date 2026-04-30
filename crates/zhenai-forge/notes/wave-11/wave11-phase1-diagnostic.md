# WAVE11 Phase 1 — seq=64 RAFT Diagnostic

**Executed:** 2026-04-21
**Purpose:** Determine whether attention IS the bottleneck at seq=384 (where Phase 5 of WAVE10F stalled for 64 min with GPU% = 0) before investing in the full GpuKernelsBackend.

## Setup

- Corpus: regenerated at MAX_TOKENS=64 via `scripts/tokenize-kingdom-for-gemma4.py`.
  - Train: 3568 sequences (all truncated to 64; median answer length 48).
  - Eval: 397 sequences (also all truncated).
- Backend: `HybridMatmulBackend` (existing incumbent, hipBLAS for matmuls + CPU for attention / softmax / RoPE / norms).
- CLI: `zhenai-forge train-gemma4 --steps 200 --lr 1e-3` (lr chosen per the WAVE10F 24h lr sweep finding — stable at 100+ steps).

## Results

**Training descended cleanly, no NaN, no GPU starvation.**

| Metric | Value |
|--------|-------|
| Wall-clock | 25.1 min for 200 steps |
| Warm step-time | **7.51 s/step** (min 7.5, max 7.8) |
| Loss trajectory | 21.36 → 18.92 → 14.86 → 13.12 → ... → 8.12 (step 200) |
| Final avg loss | 8.33 |
| Descent | **61%** from initial |
| Backend confirmed | `backend=hybrid-matmul` (ADR-048 refactor live in CLI) |

## Diagnostic verdict

### Is attention the bottleneck?

Per Step 32 of `wave11-gpu-kernels-battle-plan.md`:

| Warm step-time | Verdict |
|----|----|
| ≤ 5 s | attention CONFIRMED as bottleneck; 1:1 speedup on kernel ship |
| **5 – 10 s** | **attention DOMINANT but other overhead exists; kernels still the right move, expect partial speedup** |
| > 10 s | attention NOT sole issue; abort and investigate |

Observed **7.51 s/step** — middle band. **Attention is dominant but not sole.** Proceed to Phase 2 kernels, but calibrate expectations: GpuKernelsBackend won't deliver a 1:1 seq=384 / seq=64 speedup.

### Extrapolation to seq=384

- Purely linear scaling (if matmuls dominated): 6× seq=64 → **~45 s/step**.
- Purely quadratic (if attention dominated): 36× seq=64 → **~270 s/step**.
- Reality is somewhere in between because both forms of compute are present.
- The 24h Phase 5 "64 min with 0% GPU" observation probably combines:
  - First-call kernel compilation + long sequence's initial forward latency.
  - Baseline-eval pass (16 sequences × ~4 s each = 64 s) running synchronously.
  - Possibly some memory pressure / paging at 9 GB RSS + 4.57 GB VRAM.

### What Phase 5 of WAVE10F actually hit

The `compute_eval_loss` baseline ran 16 forward passes at seq=384 before the first training step even printed. If each forward was 30-50 s (plausible given 7.5 s at seq=64), that's 8-14 min JUST for the pre-training baseline eval. Plus model load + GPU upload (~45 s). Plus training step #1's first-call compilation (~10-30 s). We may have killed the job before it emitted its first training loss line — it may have been ALMOST PRINTING at the 64-min mark.

**This is testable after Phase 8:** rerun Phase 5 retry with `GpuKernelsBackend`, watch for first-line-emission time vs actual progress.

## Decision

**PROCEED TO PHASE 2** with expectation that GpuKernelsBackend will yield a 3-5× speedup at seq=384 (not 10× or 100×).

If Phase 8's integrated end-to-end test shows seq=384 ≤ 5 s/step warm, the sprint's exit gate is met. If it's 10-30 s/step warm, we've still unlocked real training (8-25× over synthetic baseline's 2 s, acceptable for weekly RAFT cycles).

## Artifacts

- `/tmp/wave11-kingdom-64-{train,eval}.jsonl` — tokenized corpora at seq=64 (3568 + 397 sequences; regenerate via `MAX_TOKENS=64` override in `scripts/tokenize-kingdom-for-gemma4.py`).
- `/tmp/wave11-phase1-lora.zlg4` — saved LoRA from the 200-step smoke (not evaluated on held-out; deferred to Phase 9 which has full regression + RAFT retry).
- `/tmp/wave11-phase1.log` — full training log.

## Plan amendment

Steps 33-36 (held-out eval on saved LoRA) **SKIPPED per Skip Protocol**. Justification: training-loss descent (61%) is sufficient evidence the forward path works at seq=64. Full held-out eval rolls into Phase 9's regression + RAFT retry, which will both verify the seq=64 LoRA AND the seq=384 retry on the new backend. Savings: ~15 min GPU time.
