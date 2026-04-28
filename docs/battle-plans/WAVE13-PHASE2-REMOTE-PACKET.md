# WAVE13 PHASE 2 — REMOTE EXECUTION PACKET

> **For execution on a Linux dev box with GPU + Gemma-4 GGUF + Kingdom LoRA.**
> Cannot run from Cowork-on-Macbook (no HIP/ROCm, no model files in sandbox).
> All commands here are copy-paste-ready. Fill in result skeletons after each section.
> Source-of-truth: `docs/battle-plans/WAVE13-INFERENCE.md` (in-tree per ADR-052).

**Forged**: 2026-04-27 from Cowork-on-Macbook
**Estimated remote wall-clock**: 2–4 hours
**Dependencies**:
- GPU dev box (WEST or EAST) with HIP/ROCm 6.2 stack
- Forge crate buildable from current `main` branch (HEAD `2e01fc09` or descendant)
- `/var/zhen/models/gemma-4-E2B-it.gguf` (~9.3GB) present
- `raft/kingdom-w12/kingdom.lora.gguf` present in repo
- `/tmp/24h-kingdom-eval.jsonl` present (re-tokenize via Section 0 if missing)
- `/home/govan/tmp/gemma4-venv/` Python virtualenv with HF tokenizer

**Success criteria**:
1. `crates/zhenai-forge/notes/wave13-phase2-quality.md` filled in with 8 prompt rows + win-rate
2. `docs/adr/ADR-051-wave13-generate-path.md` Status: Draft → Accepted with verdict
3. Commit message: `[PLAN SPRINT-04-27 REMOTE] CHUNK B: WAVE13 Phase 2 quality verdict + ADR-051 finalized`
4. Push to main + drift-guard CI green

---

## Section 0 — Prerequisite checks

```bash
cd /home/govan/tmp/unheaded
git log -1 --oneline                       # confirm at 2e01fc09 or descendant
ls -lh /var/zhen/models/gemma-4-E2B-it.gguf # ~9.3GB base model
ls -lh raft/kingdom-w12/kingdom.lora.gguf   # Kingdom LoRA
ls -lh /tmp/24h-kingdom-eval.jsonl          # 397 sequences

# If eval JSONL missing, re-tokenize (see WAVE10F notes Appendix A1):
# /home/govan/tmp/gemma4-venv/bin/python scripts/tokenize-kingdom-for-gemma4.py --eval-out /tmp/24h-kingdom-eval.jsonl
```

Verify each output is non-empty before proceeding. If any prerequisite missing, STUCK and report.

---

## Section 1 — Build forge release

```bash
cd /home/govan/tmp/unheaded/crates/zhenai-forge
cargo build --release 2>&1 | tail -20
test -x target/release/zhenai-forge && echo "BUILD: OK" || echo "BUILD: FAIL"
```

**Expected wall-clock**: ~3 minutes (incremental).
**Debug**: if HIP-related build error, check `rocm-smi --version` and `cargo clean -p zhenai-forge && cargo build --release`.

---

## Section 2 — Pick 8 held-out prompts

```bash
cd /home/govan/tmp/unheaded
shuf -n 8 /tmp/24h-kingdom-eval.jsonl > /tmp/wave13-phase2-prompts.jsonl
wc -l /tmp/wave13-phase2-prompts.jsonl   # must = 8

# Split each prompt into a separate tokens.json file
mkdir -p /tmp/wave13-phase2
i=0
while IFS= read -r line; do
  i=$((i+1))
  echo "$line" | jq -c '.tokens' > /tmp/wave13-phase2/p${i}.tokens.json
done < /tmp/wave13-phase2-prompts.jsonl
ls /tmp/wave13-phase2/p*.tokens.json | wc -l   # must = 8
```

---

## Section 3 — Generate BASE outputs (no LoRA)

```bash
mkdir -p /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality
cd /home/govan/tmp/unheaded
for i in 1 2 3 4 5 6 7 8; do
  echo "=== BASE prompt $i ==="
  ./crates/zhenai-forge/target/release/zhenai-forge generate-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --tokens /tmp/wave13-phase2/p${i}.tokens.json \
    --max-new-tokens 80 --temperature 0.0 \
    > crates/zhenai-forge/notes/wave13-phase2-quality/base-p${i}.txt 2>&1
done
ls crates/zhenai-forge/notes/wave13-phase2-quality/base-p*.txt | wc -l   # must = 8
```

**Expected per-prompt wall-clock**: 40–100s (no KV-cache; per-token forward over full prefix).
**Debug**: if any single prompt hangs > 5min, kill that one and continue; mark STUCK on that prompt only.

---

## Section 4 — Generate LoRA outputs

```bash
cd /home/govan/tmp/unheaded
for i in 1 2 3 4 5 6 7 8; do
  echo "=== LoRA prompt $i ==="
  ./crates/zhenai-forge/target/release/zhenai-forge generate-gemma4 \
    --model /var/zhen/models/gemma-4-E2B-it.gguf \
    --lora raft/kingdom-w12/kingdom.lora.gguf \
    --tokens /tmp/wave13-phase2/p${i}.tokens.json \
    --max-new-tokens 80 --temperature 0.0 \
    > crates/zhenai-forge/notes/wave13-phase2-quality/lora-p${i}.txt 2>&1
done
ls crates/zhenai-forge/notes/wave13-phase2-quality/lora-p*.txt | wc -l   # must = 8
```

---

## Section 5 — Decode tokens to text

```bash
cd /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality
for f in base-p*.txt lora-p*.txt; do
  /home/govan/tmp/gemma4-venv/bin/python \
    /home/govan/tmp/unheaded/scripts/gemma4-decode-tokens.py \
    < $f > ${f%.txt}.decoded.txt 2>&1 || echo "decode-failed: $f"
done
ls *.decoded.txt | wc -l   # must = 16
```

---

## Section 6 — Result-capture template

Copy below into `crates/zhenai-forge/notes/wave13-phase2-quality.md` and fill in:

```markdown
# WAVE13 Phase 2 Quality Report

**Date executed**: YYYY-MM-DD
**Executor**: <name>
**HEAD**: <git rev>
**Forge binary**: target/release/zhenai-forge (release build)

## Per-prompt rows

### Prompt 1
- **Source**: /tmp/wave13-phase2/p1.tokens.json (first 80 chars decoded): "..."
- **Base output**: "..."
- **LoRA output**: "..."
- **Qualitative tag**: [coherent | mode-collapsed | multilingual-noise | gibberish | kingdom-relevant]
- **LoRA better than base?**: [Y / N / TIE]
- **Notes**: ...

### Prompt 2 ... Prompt 8
(same structure)

## Aggregate

- **Win-rate (LoRA-better count / 8)**: ___ / 8 = ___%
- **Mean qualitative shift**: <one sentence>
- **CE-vs-quality alignment**: WAVE12 reported held-out CE Δ -14.32. Does qualitative output match that magnitude? <yes/no/partial>
- **Mode-collapse incidents**: ___ / 8
- **Kingdom-relevant LoRA outputs**: ___ / 8
- **Kingdom-relevant base outputs**: ___ / 8
```

---

## Section 7 — Decision template (Phase 3 of WAVE13)

Append to the same `wave13-phase2-quality.md`:

```markdown
## Decision (WAVE13 Phase 3)

**Verdict**: [SHIP | RETRAIN | RANK-UP | DATA-FIX]

**Rationale** (cite numbers from Section 6):
- ...

**Owner of next action**: <name>
**Next step**: <one paragraph>
```

Verdict semantics:
- **SHIP**: LoRA quality is acceptable for downstream wiring (Phase 4-5 of WAVE13 plan).
- **RETRAIN**: under-trained. Run another N steps (the WAVE13 Phase 1 commit message said 500 steps ≈ 14% of one epoch; suggest 4× more).
- **RANK-UP**: LoRA rank=16 is too low. Try rank=32 or 64.
- **DATA-FIX**: training data has issues; re-curate Kingdom corpus.

---

## Section 8 — ADR-051 finalization

After verdict locked:
1. Open `docs/adr/ADR-051-wave13-generate-path.md`
2. Status: Draft → Accepted
3. Fill in **Decision** section with the verdict from Section 7
4. Fill in **Consequences** with downstream impact (KV-cache deferral noted; WAVE14 implications)
5. Update `docs/adr/ADR-INDEX.md`: row for ADR-051 status Draft → Accepted

---

## Section 9 — Commit + push

```bash
cd /home/govan/tmp/unheaded
git add crates/zhenai-forge/notes/wave13-phase2-quality.md \
        crates/zhenai-forge/notes/wave13-phase2-quality/ \
        docs/adr/ADR-051-wave13-generate-path.md \
        docs/adr/ADR-INDEX.md
git commit --no-gpg-sign -m "[PLAN SPRINT-04-27 REMOTE] CHUNK B: WAVE13 Phase 2 quality verdict + ADR-051 finalized

Verdict: <SHIP|RETRAIN|RANK-UP|DATA-FIX>
Win-rate: <X>/8
Held-out CE alignment: <yes|no|partial>"
git push origin main
```

Drift-guard CI (per ADR-052) should pass: timeline.md was refreshed today and HEAD has new commits.

---

## Stuck handling

If any Section 3/4 generation hangs > 5 min, kill (`pkill -f zhenai-forge`) and proceed with whatever prompts completed. Document partial coverage in the quality report.

If decode (Section 5) fails entirely (e.g., venv broken), record raw token IDs in the report; verdict can be informed by token-level inspection alone — but flag as low-confidence.

---

*WAVE13 Phase 2 Remote Packet — packaged 2026-04-27 from Cowork-on-Macbook.*
*Run on the next available Linux box. Soldiers fight; Cowork ships paper.*
