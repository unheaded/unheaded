# WAVE14 Run B — pre-registered quality gate (T1.2)

**Date written:** 2026-04-29 — BEFORE Run B trains. BEFORE any output is observed.
**Purpose:** Fix the pass/fail gate for T1.4 in writing so the result cannot be moved goalposts. This is the Scientist's pre-registration; it is also what gates Track 2.

---

## Run B configuration (frozen)

```
scripts/forge-train.sh train-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-train.jsonl \
    --steps 1500 --lr 3e-4 --rank 16 --alpha 32 \
    --output raft/kingdom-w14b/kingdom.lora.gguf \
    --save-every 500
```

- LoRA rank 16, alpha 32 — identical to WAVE12 + Run A. **Two independent variables (parser + step count) collapse to one** because Run A was already 1500 steps. Run B isolates the parser-fix effect.
- lr 3e-4 — same as Run A. (lr=1e-3 stable, but keeping continuity for comparability.)
- Save checkpoints every 500 steps so we can post-mortem if the curve is weird.

---

## Pre-registered gates (T1.4 verdict)

A Run B verdict of **PASS** requires meeting **gate G-CE** + at least **2 of 3** behavioral gates (G-OPEN, G-TOPIC, G-NO-COLLAPSE). Anything less is **FAIL**.

### G-CE — held-out cross-entropy descent

The eval CLI subcommand (`eval-gemma4`) on `/tmp/24h-kingdom-eval.jsonl` (regenerate with `tokenize-kingdom-for-gemma4.py` if `/tmp` was wiped):

- **Required:** `mean_CE(LoRA) < mean_CE(base)` strictly. The `delta = base - LoRA` must be > 0.
- **Reference points (post-T1.1 H7 fix; numbers may differ from WAVE12 Δ −14.32 because the eval pipeline now reads correct data):** WAVE12 reported `base 21.10 → +LoRA 6.78`. Run B should descend at least as far. **Pass threshold: `delta ≥ 8.0`** (more lenient than WAVE12 to allow for parser-fix-induced CE shift).
- **Anti-goal:** if delta < 0 (LoRA *worse* than base) or non-finite, hard fail.

### G-OPEN — first-token corpus alignment

For each of the 8 pinned eval prompts (see "Pinned prompt slice" below), greedy-generate 100 tokens with `cmd_generate_gemma4`, then look at the first token of generation that is **not** `<end_of_turn>` (token 106) or `\n` (token 107).

- **Required:** **≥ 3 of 8** outputs begin (post-stop-token, post-newline) with one of the corpus's top-5 answer openers, which together account for **84 % of the training-corpus answers** (per H2 analysis):

  | Token ID | Decoded | Corpus open-frequency |
  |---|---|---|
  | 818 | ` The` | 75.25 % |
  | 236776 | `A` | 2.44 % |
  | 10450 | `According` | 2.21 % |
  | 4420 | `When` | 2.10 % |
  | 2267 | `An` | 2.05 % |

- **Anti-goal:** zero of 8 in this set is hard fail (means the LoRA did not learn the dominant corpus signal).

### G-TOPIC — on-topic Kingdom term emission

For each of the 8 outputs, scan the generated text (post-detokenize) for any token from this **fixed Kingdom-term allowlist** (frozen now, before observation):

```
Wotan, Sophia, Monad, Anamnesis, Kingdom, eBPF, BPF, XDP, Aya,
Champion, IPv6, packet, Gleipnir, Mímir, gjallarhorn, gungnir,
heimdall, Sleipnir, Kenoma, Pleroma, Yaldabaoth, Yggdrasil,
trace, NixOS, Wotan-, monad-, sophia-, kingdom-mode, doom-runner,
zhenai, zhend, lich, sealed-cask, S67, S68, S78, ADR-,
GPL-3.0, JetBrains, raft, kingdom RAFT, distillation
```

- **Required:** **≥ 1 of 8** outputs contains ≥ 5 contiguous tokens of which ≥ 1 is a Kingdom-term hit. (Five-tokens-around-one-keyword catches "the Wotan message bus is", not just a stray noun mention.)
- **Anti-goal:** zero of 8 means the LoRA does not produce Kingdom-domain content at all.

### G-NO-COLLAPSE — no Run-A-style structural attractor

For each of the 8 outputs:

- Count the longest contiguous run of the repeating pattern `\t}\n\nQuestion:\n` (the WAVE14 Run A attractor).
- **Required:** **0 of 8** outputs have this run occupying > 40 of the first 100 generated tokens. (i.e., < 40 % attractor-fill at the cap.)
- **Anti-goal:** any output ≥ 40 % attractor is a hard fail of this gate.

---

## Pinned prompt slice (frozen now for reproducibility forever)

Run B and **all future re-evals** use the **first 8 records of `/tmp/24h-kingdom-eval.jsonl`**, sliced at `.answer_start`:

```
for i in 1..=8:
    record = jsonl_line(eval, i-1)
    prompt_tokens = record.tokens[0..record.answer_start]
    write to /tmp/wave14-phase2/p{i}.tokens.json
```

This replaces the `shuf -n 8` random sampling used in WAVE13 P2. **Trade-off:** loses direct apples-to-apples with WAVE13 P2 / Run A's 8 specific prompts (those were lost when `/tmp` got wiped). **Gain:** every future re-eval picks the same 8, deterministic, reproducible from corpus.

The previous WAVE13 P2 `answer_start` distribution was 328, 341, 341, 358, 349, 224, 337, ? — well within the corpus's 340-380 cluster. The first 8 deterministic records are also expected to fall in that band (verified at T1.4 setup time before generation).

---

## Step-by-step T1.4 procedure (for the next session)

1. Build release if needed: `cargo build -p zhenai-forge --release` (one rustc job per `.cargo/config.toml`; 3-5 min cold).
2. Regenerate `/tmp/24h-kingdom-{train,eval}.jsonl` if absent (post-reboot `/tmp` wipe).
3. **G-CE measurement:** `target/release/zhenai-forge eval-gemma4 --model ... --data /tmp/24h-kingdom-eval.jsonl --lora raft/kingdom-w14b/kingdom.lora.gguf` → capture stdout. Run again WITHOUT `--lora` for base baseline. Compute delta.
4. **Build pinned prompt slice:** for i in 1..=8, slice first 8 eval records at `.answer_start`, write to `/tmp/wave14-phase2/p${i}.tokens.json`. 8 files, ~30s.
5. **Generate 8 LoRA outputs:** for each `p${i}.tokens.json`, run `target/release/zhenai-forge generate-gemma4 --model ... --lora raft/kingdom-w14b/kingdom.lora.gguf --tokens /tmp/wave14-phase2/p${i}.tokens.json --max-new-tokens 100 --temperature 0.0`. Capture stdout. ~1-2 min per prompt × 8 = ~10-15 min.
6. **Score against gates G-OPEN, G-TOPIC, G-NO-COLLAPSE.** Use a small Python script if helpful — pure string/token checks against frozen lists. NO model needed.
7. Write verdict to `notes/wave14-runB-results.md` with per-gate pass/fail tables.

---

## What this pre-registration prevents

| Failure mode | How pre-reg blocks it |
|---|---|
| Eyeballing outputs and "looking good" | Gate is token-IDs-and-counts mechanical |
| Moving goalposts after seeing CE | Threshold ≥ 8.0 fixed before observation |
| Cherry-picking 1/8 outputs as "evidence" | Counts ≥ N of 8 required across multiple gates |
| Confirming the parser fix on CE alone (WAVE12-style trap) | G-OPEN + G-TOPIC + G-NO-COLLAPSE are behavioral, not loss-based |
| New "Question:" attractor in different shape | G-NO-COLLAPSE catches any high-fill repeating pattern |

**This gate is unforgiving by design.** The Scientist's role is to make the experiment decide between hypotheses — not to reward effort.

---

## Outcome branch (per long-term plan §3 Track 1 exit)

- **All 4 gates PASS** → parser fix is causal AND LoRA learned the corpus → proceed to T1.5 (RAG baseline) → if LoRA beats RAG on ≥4/8 prompts → **proceed to Track 2** (Champion API contract + corpus harness + code corpus).

- **G-CE PASS, < 2 of 3 behavioral PASS** → fix is causal for loss but LoRA didn't reach generative competence → likely undertraining still, OR corpus shape is wrong for generation → **STOP. File postmortem.** Decide whether to push to 3000-5000 steps OR pivot to coding-corpus per Track 2 directly.

- **G-CE FAIL** → either parser fix isn't causal (the analysis was wrong) OR a deeper bug remains → **HARD STOP. File postmortem.** Consider HF/PEFT insurance backend.

---

## Files this gate creates / writes (T1.4)

- `raft/kingdom-w14b/kingdom.lora.gguf` (+ checkpoints at 500, 1000, 1500)
- `/tmp/wave14-phase2/p[1-8].tokens.json`
- `crates/zhenai-forge/notes/wave14-runB-results.md` (T1.4 verdict)
- `crates/zhenai-forge/notes/wave14-phase2-quality/lora-w14b-p[1-8].txt` (raw outputs)

— Pre-registered. Frozen. Scientist signing off. ❤
