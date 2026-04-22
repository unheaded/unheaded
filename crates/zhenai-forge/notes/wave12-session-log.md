# WAVE12 Session Log — GPU-Resident Forge @ seq=384

**Sprint:** WAVE12 — close 2× gap from 10.5 s/step warm → ≤5 s/step target, then ship 500-step Kingdom RAFT LoRA with eval descent.
**Plan:** `crates/zhenai-forge/notes/wave12-gpu-resident-battle-plan.md` (9 phases, 195 steps).
**Forged:** 2026-04-22.
**Agent:** Coordinator (solo).
**Commit cadence:** every 5 steps; phase-exit commit mandatory; `--no-gpg-sign` per AFK policy.

---

## Per-phase results

| Phase | Name | Gate | Measured | Status |
|------:|------|-----|---------|-------:|
| 0 | Preflight + baseline | warm step 3 ∈ [9.5, 12] s | **10.2 s** @ loss 21.21→18.63→16.47 | ✅ |
| 1 | Shared mask cache | warm ≤ 10.5 s (no regression) | — | pending |
| 2 | Backward GPU ops | warm ≤ 8.5 s | — | pending |
| 3 | Checkpoint + decision | median warm | — | pending |
| 4 | Matmul GPU-in/out | matmul tests green, no regression | — | pending |
| 5 | GPU-resident forward | warm ≤ 5.5 s | — | pending |
| 6 | Full regression | Learning Gate 4/5 pass, warm ≤ 5.5 s | — | pending |
| 7 | 500-step RAFT | eval loss@500 < eval loss@0, final train loss < 8 | — | pending |
| 8 | ADR-050 + handoff | DoD all true | — | pending |

---

## Phase 0 — Preflight + baseline re-confirm (2026-04-22)

**Duration:** ~15 min wall (plan budgeted 30m).

**Ground state verified:**
- HEAD `93c137fe` (WAVE12 plan forged), descends from `5d32f9d5` (WAVE11 Phase 8c shipped). Tree clean.
- GGUF `/var/zhen/models/gemma-4-E2B-it.gguf` present (8.7 GB on disk; plan notes 9.3 GB — likely filesystem rounding, file is the one W11 was trained against).
- `MemAvailable`: 10 GiB.
- No zombie forge processes. GPU[0] VRAM 203 MB (<<1 GB).

**Corpus recovery (Appendix A1 deviation):**
- `/tmp/24h-kingdom-{train,eval}.jsonl` were absent (not reboot, but `/tmp` cleaned).
- Appendix A1's suggested CLI flags (`--input`, `--output`, `--model`, `--max-tokens`) are not implemented in `scripts/tokenize-kingdom-for-gemma4.py`; the script reads stdin → stdout and uses a hardcoded tokenizer path. Ran via the documented venv pattern instead:
  ```bash
  /home/govan/tmp/gemma4-venv/bin/python scripts/tokenize-kingdom-for-gemma4.py \
    < raft/training/train.jsonl > /tmp/24h-kingdom-train.jsonl
  ```
- Result: train `in=3568 out=3568 err=0 short=0 trunc=3561`; eval `in=397 out=397 err=0 short=0 trunc=397`. Line counts match plan expectations (3568 + 397).

**Regression gates:**
- `cargo build --release`: 0 errors, 101 pre-existing warnings.
- `cargo test -- hip_kernels::`: 21 passed / 0 failed / 108 filtered in 4.80 s.
- `test_gemma4_gpu_forward_matches_cpu`: cosine **0.998549**, argmax CPU=5646, GPU=5646 → top-1 match, top-5 overlap 4/5. ok.
- `test_gemma4_gpu_train_step_loss_descent`: 3-step toy loss 21.10 → 13.29 → 6.48 monotonic, avg 2.17 s/step (toy, not seq=384). ok.

**Baseline smoke (seq=384, 3 steps, lr=1e-3, answer_start=1):**
```
[step 1/3] loss=21.2099 step_time=42.7s  (cold)
[step 2/3] loss=18.6311 step_time=10.3s  (JIT warm)
[step 3/3] loss=16.4732 step_time=10.2s  (steady)
```
Warm step 3 = **10.2 s** → inside the plan's 9.5-12 s baseline window. **Baseline gate ✅**.

**Notes carried forward:**
- Cold step (42.7 s) is noticeably faster than plan's 70-80 s expectation; likely filesystem cache was already warm from prior sessions today. Not a regression signal.
- Budget to close: 10.2 → 5.0 s = 5.2 s/step to find across Phases 1-5 (mask cache ~0.3, backward GPU ~2-4, GPU-resident forward + eliminating round-trips ~1-3).

---

## Commit ledger

| Step | Hash | Phase | Summary |
|-----:|------|------:|---------|
| 20 | *pending* | 0 | `docs(forge): [PLAN W12] Phase 0 — preflight, baseline confirmed @ ~10.2s/step warm` |

---

*Session log auto-updated after every phase-exit commit.*
