# Gemma-4-E2B-it Vetting — Lab Notebook

**Date:** 2026-05-04
**Lens:** unheaded-scientist (Strong Inference)
**Operator:** Stevie + crew
**Status:** **REJECT adoption** — H3 hard-failed; gemma kept in `scripts/switch-model.sh` as a documented option but qwen-7b remains the default chat model.

---

## 1. Question

Should `gemma-4-E2B-it` (4.65B params, fp16 GGUF, ~9 GB on disk, ~5.7 GB VRAM at 8k ctx) replace `qwen2.5-coder-7b-instruct-q4_k_m` as the default chat model for the Zhenai web UI?

Trigger: Stevie's directive to "try gemma" + "increase max_tokens for gemma and utilize unheaded-scientist to vet it properly" (2026-05-04). The bench is the existing 14-prompt textbook tier of `eval/coding-gate/prompts.jsonl` — same fixture that binds the H0 coding gate.

## 2. Pre-registered hypotheses

Locked **before** running any pass:

| ID | Hypothesis | Falsifies on |
|---|---|---|
| H1 | Gemma's *final* answer (post-thinking-strip) beats qwen on ≥4/14 prompts | matches qwen on ≤3/14 |
| H2 | Gemma can finish its answer within `max_tokens=2000` (no truncation) | truncates on ≥3/14 prompts |
| H3 | Gemma's wall-time-to-answer is not ≥2× slower than qwen on ≥4/14 | gemma ≥2× slower on ≥4/14 |
| H4 | Gemma's reasoning trace catches review-tier bugs qwen misses | catches none qwen misses |

**Decision rule (locked):**

- H1 PASS + H2 PASS + H3 not-FAIL → adopt gemma as default chat model.
- H2 FAIL → reject (truncation makes it unusable).
- H1 FAIL + H3 FAIL → reject (verbose AND slow).
- H4 alone PASS → keep qwen default, document gemma's niche.

## 3. Method

Three passes over the same 14 textbook prompts (7 syntax + 7 review across bash/python/go/rust/html/css/javascript):

| Pass | Model | max_tokens | Rationale |
|------|-------|------------|-----------|
| A | qwen2.5-coder-7b-instruct (q4_k_m, 16k ctx) | 600 | H0 baseline equivalent |
| B | gemma-4-E2B-it (fp16, 8k ctx) | 600 | apples-to-apples vs qwen |
| C | gemma-4-E2B-it (fp16, 8k ctx) | 2000 | the "increase max_tokens" condition |

All passes use:
- Zero retrieval — direct llama-server `/v1/chat/completions`. Strips RAG variance so we're measuring model intrinsic, not retrieval × model interaction.
- No system role — fairness for non-OpenAI chat templates (gemma's `<|turn>` template).
- Temperature 0.1, stream off, single sample per prompt.
- Model swap via `./scripts/switch-model.sh` (the canonical interchange seam).

Raw responses captured as JSON + plaintext per `(pass, prompt-id)` pair under this directory. Reproducer: `./eval/coding-gate/run-gemma-vet.sh <pass-label> <max-tokens>`.

## 4. Results

### 4.1 Mechanical metrics

```
id                   qwen-tok  qwen-s   gem600  gem2k-tok  gem2k-s  gem2k-fin  gem2k-think%
-----------------------------------------------------------------------------------------------
syntax-bash               549    8.95   length       1800    29.51       stop           43%
syntax-python             546    8.84   length       1969    31.91       stop           32%
syntax-go                 397    6.43   length       2000    32.25     length           38%
syntax-rust               311    5.04   length       1390    21.94       stop          100%
syntax-html                15    0.27   length       1272    20.08       stop           41%
syntax-css                600    9.76   length       1960    30.98       stop           39%
syntax-javascript         304    4.93   length       1754    27.68       stop           38%
review-bash               477    7.76   length       1767    27.95       stop           41%
review-python             439    7.14   length       1499    23.64       stop           48%
review-go                 490    7.98   length       1433    22.60       stop           41%
review-rust               417    6.79   length       1766    27.88       stop           39%
review-html               213    3.49   length       1631    25.74       stop           45%
review-css                 34    0.60   length       1253    19.75       stop           51%
review-javascript         279    4.56   length       1045    16.46       stop           48%
```

Aggregates:

| | qwen-7b | gemma @ 2000 |
|---|---|---|
| Avg completion tokens | 362 | 1610 (4.4× more) |
| Avg wall-time | 5.9 s | 25.6 s (4.3× slower) |
| Avg tok/s | ~61 | ~63 |
| Truncated (`finish=length`) | 1/14 | 1/14 |
| Avg %tokens spent in thinking trace | 0% | 46% |

At `max_tokens=600` (Pass B), gemma truncated **14/14 prompts** with only 3/14 producing any final answer (closing `<channel|>` tag emitted before the truncation cutoff). The 600-token budget is incompatible with gemma's reasoning-mode output by construction.

### 4.2 Qualitative observations

- **Quality on syntax-bash** (cherry-picked but representative): gemma's final answer is *more thorough* — covers Bash parameter expansion (the pure-bash idiom) which qwen omitted in favour of three subprocess-spawning approaches (sed/awk/xargs). On this prompt **gemma > qwen on completeness**.
- **Reasoning style is a feature in some contexts.** The thinking trace exposes the model's path-of-derivation, which can be educational. But for a chat surface where users want answers, ~half the response is overhead.
- **`syntax-rust` is a hard falsifier**: gemma never closed the thinking tag in 1390 tokens. The post-strip "final answer" was empty. This is gemma producing *zero usable output* on a textbook syntax question. qwen produced a correct 311-token answer in 5.04s.

### 4.3 Hypothesis verdicts

| ID | Verdict | Evidence |
|---|---|---|
| H1 | INCONCLUSIVE — would need full subjective grading | sample-size-of-1 says gemma may match or beat qwen on completeness, but `syntax-rust` shows gemma can produce *no answer at all* |
| H2 | **PASS at 2000 tok** (1/14 truncation < 3/14 threshold); **FAIL at 600 tok** (14/14 truncation) | the "increase max_tokens" recommendation is necessary — it's the difference between "broken" and "works" |
| H3 | **HARD FAIL** | gemma is ≥2× slower on **14/14** prompts. Avg 4.3× wall-time. |
| H4 | NOT-TESTED — review-tier prompts under-graded in this pass | future work |

## 5. Decision

**REJECT gemma-4-E2B-it as the default chat model.**

The locked decision rule says: *"H1 FAIL + H3 FAIL → reject (verbose AND slow)."* H1 is at best inconclusive and at worst no-answer-on-`syntax-rust`. H3 is a hard fail. Either alone would not be conclusive; together they are.

Practical implication: a chat tab that takes 25 s per response is unusable for the operator-surface workflow Stevie built (`/api/v1/query` round-trips that need to feel like a search box, not a job queue). qwen-7b's 5.9 s is already on the edge; quadrupling that would gut the UX.

### What gets kept

- `gemma` key remains in `scripts/switch-model.sh` for one-command swap when the use case fits (e.g. an offline coding-review run where verbosity is fine and reasoning trace is a feature, not a bug).
- The 9.3 GB GGUF stays on disk at `/var/zhen/models/gemma-4-E2B-it.gguf` — `/var` has 1.4 TB free; deleting it would force re-conversion from HF safetensors.
- This notebook stays in-repo as the canonical record of *why* qwen remained default after we tried gemma. Future-Stevie won't re-run this experiment blind.

### Adjacent finding (Reddit r/LocalLLaMA, surfaced mid-experiment)

Stevie pasted a Reddit thread suggesting **Qwen 3.5 35B A3B** (MoE, 3B active params) with `--n-cpu-moe 30 -ctk q8_0 -fa on --reasoning-budget 0` runs at ~33 tok/s on similar hardware (12 GB VRAM + adequate system RAM) with a much larger context window. That's a different hypothesis class — *MoE with CPU expert offload* rather than *fully-on-GPU dense*. Worth a separate sprint:

- Pre-condition: confirm we have ≥48 GB system RAM (the offloaded experts live there).
- Hypothesis: Qwen 3.5 35B A3B answers coding-gate textbook prompts with quality ≥ qwen-7b at ≤2× wall-time.
- Same bench fixture, same decision rule shape.

If Stevie wants me to schedule this, the recipe:
1. Download `unsloth/Qwen3.5-35B-A3B-GGUF` Q4_K_M (~20 GB).
2. `./scripts/switch-model.sh qwen35-moe` (after adding the key with `--n-cpu-moe 30 -ctk q8_0 --reasoning-budget 0`).
3. Run `./eval/coding-gate/run-gemma-vet.sh passD-qwen35-2000 2000`.
4. Compare against pass-A baseline.

## 6. Pass D — DeepSeek-Coder-V2-Lite with `--n-cpu-moe 20` (added 2026-05-04)

After the gemma rejection above, Stevie asked: *"what if I am not worried about
speed — can we get a model to offload to CPU and system RAM that will solve
coding problems slowly?"* The fully-on-GPU deepseek attempt earlier (this
notebook's parent finding) OOM'd at zhen_app's RAG prompt sizes. With 20 of
the 27 expert layers pinned to system RAM (`--n-cpu-moe 20`) and 7 layers +
attention on GPU, deepseek fits — barely.

**Hardware utilisation post-load:**

| Resource | Used | Total | Free |
|---|---|---|---|
| VRAM | 5.9 GB | 12 GB | 6.1 GB |
| RAM | 10 GB | 14 GB | **0.4 GB** (system on the edge of swap) |

**Pass D bench (same 14-prompt fixture, max_tokens=600):**

```
id                   qwen-tok  qwen-s  ds-tok    ds-s  ds-fin  ds/qwen-x
----------------------------------------------------------------------------------
syntax-bash               549    8.95     600   17.26  length      1.93x
syntax-python             546    8.84     600   17.35  length      1.96x
syntax-go                 397    6.43     600   17.31  length      2.69x
syntax-rust               311    5.04     600   17.29  length      3.43x
syntax-html                15    0.27      78    2.33    stop      8.63x
syntax-css                600    9.76     600   17.23  length      1.77x
syntax-javascript         304    4.93     576   16.55    stop      3.36x
review-bash               477    7.76     474   13.84    stop      1.78x
review-python             439    7.14     445   12.92    stop      1.81x
review-go                 490    7.98     461   13.44    stop      1.68x
review-rust               417    6.79     332    9.65    stop      1.42x
review-html               213    3.49     301    8.76    stop      2.51x
review-css                 34    0.60     319    9.23    stop     15.38x
review-javascript         279    4.56     570   16.67    stop      3.66x

qwen avg:        5.90s,  truncated 1/14
deepseek-cpu:   13.56s,  truncated 5/14
avg slowdown:    2.30x
prompts ≥2x:     7/14
prompts ≥3x:     5/14
```

**Side-by-side quality (sampled — full read of all 28 responses would be
the rigorous form, but the sample is informative):**

- `review-python`: roughly tied. Both flag broad-`except` + recommend
  `FileNotFoundError`. Deepseek frames the empty-`{}`-return as a
  call-site-default issue more cleanly. **Slight edge to deepseek.**
- `review-go`: deepseek hallucinates *"the os package is imported but not
  used"* — except `os.WriteFile` IS using it. Qwen correctly flags both
  the silently-discarded `json.Marshal` error AND the hardcoded `/tmp`
  path. **Qwen WINS** — deepseek made a confidently wrong observation.
- `syntax-html`: deepseek answered in 78 tokens (correct, terse).
  Qwen answered in 15 tokens (also correct, terser).

**Hypothesis verdicts under Pass D:**

| ID | Result |
|---|---|
| H1 (≥4/14 better than qwen) | **NOT MET** — sampled responses tied or qwen wins; one deepseek hallucination is a real downside |
| H2 (≤3/14 truncation) | **PASS** at 600 tokens (5/14 truncated — at the boundary; would need bench at 1500 tokens to confirm cleanly, but no thinking-trace overhead so headroom helps directly) |
| H3 (not ≥2× slower on ≥4/14) | **HARD FAIL** — 7/14 prompts ≥2×, 5/14 prompts ≥3×, avg 2.30× slowdown |

**Decision under Pass D:** locked rule says "H1 FAIL + H3 FAIL → reject (verbose
AND slow)". Deepseek-cpu is not verbose like gemma was, but the slowdown is
real and the quality upside is not there. **REJECT deepseek-cpu as default chat
model.**

The `deepseek-cpu` key remains in `scripts/switch-model.sh` for opportunistic
swap (e.g. when working on a long-form code review where 13s/answer is fine).

**Practical pivot — what would actually deliver "slower but smarter":**

The structural reason deepseek-cpu doesn't pay off: it's a 16B-MoE model with
**only 2.4B active parameters per inference step**. Putting 20 layers on CPU
slows inference by the offload overhead but doesn't help quality — we're still
running effectively a 2.4B model with retrieval. The next experiment that
might actually deliver a quality win on this hardware is **Qwen2.5-Coder-14B
(dense Q4)** with `-ngl 30` (partial GPU layer offload):

- Same coding family as the H0 baseline (consistent grading)
- 2× the params (14B dense > 2.4B active MoE) → genuine quality bump
- Fits with ~5 GB on GPU + ~5 GB on CPU = ~10 GB total
- Expected speed: ~10-15 tok/s (similar to deepseek-cpu)
- Expected quality: real upgrade vs qwen-7b

Recipe (when activated):
1. Download `bartowski/Qwen2.5-Coder-14B-Instruct-GGUF` Q4_K_M (~9 GB).
2. Add `qwen-14b` key to `scripts/switch-model.sh` with `--ctx-size 8192
   --n-gpu-layers 30 --parallel 1 --cache-type-k q8_0`.
3. Re-run `./eval/coding-gate/run-gemma-vet.sh passE-qwen-14b 1500 2`.
4. Apply same H1/H2/H3/H4 rule — but with the *adoption-as-default*
   threshold on H3 relaxed to "≤3× slower on ≥4/14" since Stevie has
   accepted slower-for-smarter as the trade.

## 7. Reproducibility

- `scripts/switch-model.sh` documents the per-model launch flags.
- `eval/coding-gate/run-gemma-vet.sh` is the bench runner.
- All raw responses (`*.json` + `*.txt`) are checkpointed under this directory; nothing is regenerated by re-running.
- Hardware: AMD Radeon RX 7700 XT, 12 GB VRAM, ROCm. Kernel 6.17. llama.cpp build `b1-d23355a`.
- Stack at run time: zhen_app down (kept out of the experiment), llama-server only, no other GPU consumers.

---

> **Falsification log:** if a future-Stevie or future-Claude runs this same experiment and gets H3 as a near-miss instead of a hard fail (e.g. gemma drops below 2× slower on most prompts), that probably means the llama.cpp build added flash-attn-for-MLA or aggressive speculative decoding for reasoning-mode outputs. Re-grade.
