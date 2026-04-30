# WAVE14 Run B — post-train execution recipe (T1.4 + T1.5)

When Run B finishes (Monitor sees `TRAINING COMPLETE` and `Saved to:`),
the LoRA at `raft/kingdom-w14b/kingdom.lora.gguf` is the artifact under test.

This doc is a copy-pasteable execution path for T1.4 (gate scoring) and
T1.5 (RAG control arm). All commands assume cwd = `/home/govan/tmp/unheaded`.

---

## Step 1 — Sanity check the artifact (~30 s)

```
ls -la raft/kingdom-w14b/
```

Expect `kingdom.lora.gguf` plus checkpoint-{500,1000,1500} files. If
training was killed before completion, use the latest checkpoint that
exists.

---

## Step 2 — G-CE measurement (eval-gemma4, twice) (~40 min)

### Step 2a — base model (no LoRA)
```
FORGE_LOG=/tmp/wave14-eval-base.log scripts/forge-train.sh \
    eval-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --data /tmp/24h-kingdom-eval.jsonl
```
Note: don't pass `--lora`. Capture `mean CE loss (base): X.XXXX`.

### Step 2b — with LoRA (Run B)
```
FORGE_LOG=/tmp/wave14-eval-runB.log scripts/forge-train.sh \
    eval-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --lora raft/kingdom-w14b/kingdom.lora.gguf \
    --data /tmp/24h-kingdom-eval.jsonl
```
Capture `mean CE loss (+ LoRA): Y.YYYY` and `delta: Z.ZZZZ`.

**G-CE pass:** `delta = base - LoRA ≥ 8.0`.

WAVE12 reference: base 21.10 → +LoRA 6.78 → delta −14.32. Run B should
beat this once the H6 fix takes effect (predicted; not pre-registered).

---

## Step 3 — Generate 8 outputs (T1.4 behavioral) (~10-15 min)

```
mkdir -p crates/zhenai-forge/notes/wave14-phase2-quality
for i in 1 2 3 4 5 6 7 8; do
    echo "=== LoRA prompt $i ==="
    /home/govan/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge \
        generate-gemma4 \
        --model /var/zhen/models/gemma-4-E2B-it.gguf \
        --lora raft/kingdom-w14b/kingdom.lora.gguf \
        --tokens /tmp/wave14-phase2/p${i}.tokens.json \
        --max-new-tokens 100 \
        --temperature 0.0 \
        > crates/zhenai-forge/notes/wave14-phase2-quality/lora-w14b-p${i}.txt 2>&1
    echo "  done p${i}"
done
ls crates/zhenai-forge/notes/wave14-phase2-quality/lora-w14b-p*.txt | wc -l   # must = 8
```

Each generation: ~1-2 min wall (no KV cache; per-token forward over
prompt prefix). Expected cumulative: 10-15 min.

---

## Step 4 — Run the gate scorer (G-OPEN, G-TOPIC, G-NO-COLLAPSE) (~5 s)

```
/home/govan/tmp/gemma4-venv/bin/python3 scripts/wave14-score-runB.py
```

Output: per-prompt scores + verdict.

---

## Step 5 — RAG control arm (T1.5) (~5-10 min, ~3 GB RAM)

**Only run after Run B has fully released GPU + RAM** (otherwise the
FAISS index load will fight the eval pipeline for memory).

```
/home/govan/tmp/gemma4-venv/bin/python3 scripts/wave14-rag-baseline.py
```

Output: `notes/wave14-runB-rag-baseline.md` with side-by-side. The
plan §3 Track 1 exit gate requires LoRA to beat RAG on ≥4/8 prompts.

---

## Step 6 — Verdict + commit

Write `notes/wave14-runB-results.md` with:

1. G-CE numbers (base, +LoRA, delta) — PASS/FAIL.
2. G-OPEN/G-TOPIC/G-NO-COLLAPSE table from `wave14-score-runB.py`.
3. Verdict per pre-registered rules:
   - **PASS:** G-CE PASS plus ≥2 of (OPEN, TOPIC, NO-COLLAPSE) PASS → proceed to Track 2.
   - **FAIL:** anything less → STOP, postmortem, decide push vs pivot.
4. RAG comparison from T1.5: LoRA beats RAG on N/8 prompts.
5. Hyperlinks to the 8 raw output files for audit trail.

Single commit covering: results doc, RAG note, scorer-derived numbers.
ADR-051 amendment drafts after this commit.

---

## Step 7 — Update memory + handoff

If PASS:
- New memory entry `project_wave14_runB_results.md` summarizing.
- Update `MEMORY.md` index.
- Mark T1.4 + T1.5 tasks completed; create Track 2 (T2.1, T2.2, T2.3).

If FAIL:
- Memory entry `project_wave14_runB_failure_postmortem.md`.
- Decide whether to extend (Run C @ 3000 steps) OR pivot to coding corpus.

---

## Allowlist gap caveat (note for verdict doc)

`scripts/wave14-score-runB.py`'s G-TOPIC Kingdom-allowlist was set at
pre-registration based on Kingdom-mythology terminology. A sample of
8 real corpus answers shows broader vocab is more common: "Unheaded",
"Phylactery", "Sigil", "ResourcePool", "Grafana", "Phases" — these
would NOT register as G-TOPIC hits despite being on-topic.

The gate threshold is `≥1/8` which is lenient enough that this gap
shouldn't false-fail the gate. If G-TOPIC fires 0/8 while OPEN and
NO-COLLAPSE pass, manually review the 8 outputs for "is this clearly
on-topic about Unheaded but missing my allowlist words?" before
declaring G-TOPIC FAIL.

This is documented BEFORE Run B's outputs are observed (no goalpost
moving). Allowlist is frozen.
