# WAVE10F Phase 7.1 Tokenizer Vertical Slice

**Built:** 2026-04-20 during the 24h battle plan (Phase 4, Steps 166-179).
**Scope:** Python-side tokenization of the Kingdom RAFT corpus so forge's
`train-gemma4` CLI can consume it. Full Rust SentencePiece integration is
deferred to a future sprint.

## Backend

- **Python:** `/home/govan/tmp/gemma4-venv/bin/python` (PEP 668 prevents
  system-level pip install; venv bypass works).
- **Tokenizer:** `transformers.AutoTokenizer.from_pretrained("/home/govan/tmp/gemma-4-E2B-it")`
  — reads the official Gemma 4 HF artifacts (`tokenizer.json` +
  `chat_template.jinja`), giving us the exact tokenization the GGUF uses.
- **No new Rust deps.** The crate is unchanged; this is pure Python.

## Script: `scripts/tokenize-kingdom-for-gemma4.py`

Reads Mistral-formatted JSONL from stdin:
```
{"text": "<s>[INST] ...system + context + question... [/INST] ...answer...</s>"}
```

Splits on `[INST]` / `[/INST]` markers, strips `<s>` / `</s>`, rebuilds
as a Gemma 4 chat-template conversation via
`tok.apply_chat_template([{"role":"user",...},{"role":"model",...}])`,
tokenizes without adding special tokens (template already includes them),
and emits:
```
{"tokens": [2, 105, ..., 203], "answer_start": 360}
```

`answer_start` is the token index of the first model-response token.
Forge's `train_step_gemma4_gpu` computes CE loss from `answer_start + 1`
forward (next-token prediction).

## Truncation policy

`MAX_TOKENS = 384` (one sequence budget that keeps ~2s warm GPU step).
Sequences longer than that get **left-truncated** (earliest tokens dropped)
so the answer is preserved; `answer_start` is recomputed against the
truncated prefix.

## Observed corpus statistics (raft/training/train.jsonl)

```
tokenized: in=3568 out=3568 err=0 short=0 trunc=3561
seq len:    p50=384 p95=384 max=384
answer len: p50=48 p95=144 max=383
```

99.8% of training sequences get truncated — Kingdom RAFT examples carry
long code-snippet Context blocks that push the `[INST]` portion over 240
tokens. Median answer length is 48 tokens, which is a usable training
signal (enough model-response tokens for gradient flow to propagate into
meaningful weights).

Tradeoff: a bigger MAX_TOKENS (e.g. 768) would preserve more Context but
double per-step time. Keep 384 for the initial vertical slice; revisit
if Phase 5 training shows context-starvation (eval stagnating despite
train descending).

## Known limits (explicit)

1. **Python-side, not Rust-side.** Training data must be pre-tokenized
   before `zhenai-forge train-gemma4` is called. A full Rust
   SentencePiece integration would let `--data` take raw text directly.
2. **No streaming, no batching.** The script processes one line at a time
   in Python. Slow but fine for 3965 sequences (~1 min total wall-clock).
3. **No special-token injection.** Relies on `apply_chat_template`
   handling `<bos>`, `<start_of_turn>`, `<end_of_turn>` correctly.
4. **Aggressive left-truncation.** Earliest tokens lost first. OK for the
   RAFT QA format where the question + answer live at the end.
5. **No reverse tokenization.** The script is one-way: text → tokens.
   Decoding inferred answers is out of scope.

## Usage

```bash
# One-shot tokenize of the Kingdom corpus:
/home/govan/tmp/gemma4-venv/bin/python \
  scripts/tokenize-kingdom-for-gemma4.py \
  < raft/training/train.jsonl \
  > /tmp/24h-kingdom-train.jsonl

/home/govan/tmp/gemma4-venv/bin/python \
  scripts/tokenize-kingdom-for-gemma4.py \
  < raft/training/eval.jsonl \
  > /tmp/24h-kingdom-eval.jsonl

# Then:
zhenai-forge train-gemma4 \
  --model /var/zhen/models/gemma-4-E2B-it.gguf \
  --data /tmp/24h-kingdom-train.jsonl \
  --steps 7136 --lr <optimal-from-Phase-2> \
  --output /tmp/kingdom-lora.zlg4
```
