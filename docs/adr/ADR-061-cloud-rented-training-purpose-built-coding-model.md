# ADR-061 — Cloud-Rented Training for a Purpose-Built Unheaded Coding Model

**Status:** Research / Pipe Dream (note-to-self; not yet a sprint)
**Date:** 2026-05-04
**Deciders:** Stevie Bellis (driving), unheaded-scientist (bench design when activated), unheaded-developer (training + serving), unheaded-blackmage (model-supply-chain threat model)
**Triggered by:** Stevie's note 2026-05-04, after the gemma + deepseek-cpu vetting both failed to deliver a quality bump on this hardware: *"I am starting to think renting a gpu to train something we know will run and fit that is trained on our preferred data might be the move."*

---

## Context

The chat-model search of 2026-05-04 (`eval/coding-gate/gemma-vet-2026-05-04/LAB-NOTEBOOK.md`) closed with two negatives and one parked positive:

1. **Gemma-4-E2B-it** rejected — reasoning-mode verbosity made it 4.3× slower wall-time than qwen-7b with no clear quality win.
2. **DeepSeek-Coder-V2-Lite + `--n-cpu-moe 20`** rejected — 2.30× slower with one confidently-wrong hallucination on `review-go`. The MoE-with-2.4B-active architecture means the offload doesn't buy quality.
3. **Qwen2.5-Coder-14B (dense Q4)** parked as the next experiment. *Not yet run.*

The pattern across all three: **off-the-shelf models in the 4B-16B parameter range either don't fit our 12 GB VRAM at the prompts our RAG pipeline emits, or they fit but don't beat qwen-7b on our specific corpus.** The 12 GB ceiling is the binding constraint, not training data.

The strategic pivot Stevie is considering: **train a model we know will fit, on data we curate, against the bench we've already locked.** Specifically:

- Fixed target: a model that fits comfortably on the 12 GB RX 7700 XT at Zhen's prompt sizes (~4-6 GB VRAM resident, ≤2 GB KV-cache).
- Curated data: Unheaded source + ADRs + battle plans + the corpus Stevie's been writing for a year. Plus the canonical coding-gate textbook examples as supervised fine-tuning targets.
- Purpose: an Unheaded-native chat model that beats qwen-7b on the textbook-14 H0 gate AND demonstrably knows Unheaded internals without retrieval crutch.
- Compute: rented cloud GPU (one or two A100-80GB or H100 nights), not Stevie's local card.

This is explicitly NOT a sprint — it's a research direction. The note exists so that:

1. Future-Stevie has the reasoning trail that produced it (the empirical chain from gemma + deepseek vetting).
2. The next time someone (Stevie or future-Claude) asks "why are we still on qwen-7b?", the answer "because we tried the alternatives and they all lost; the next move is bespoke training" lives in-repo.
3. When activated, this ADR becomes the mother battle plan.

---

## What "purpose-built" means concretely

### Base model selection

The base model is the bottleneck the training has to fit *under*. Candidates:

| Base | Params | Q4 VRAM | Fits 12 GB? | Notes |
|---|---|---|---|---|
| **Qwen2.5-Coder-1.5B** | 1.5B | ~1 GB | ✓ comfortably | tiny but fast; could leave 8+ GB for context |
| **Qwen2.5-Coder-3B** | 3B | ~2 GB | ✓ comfortably | Stevie's existing forge work targets Gemma-4-E2B (~4 GB); this is the analogue |
| **Qwen2.5-Coder-7B** (today's default) | 7B | ~4.7 GB | ✓ tight at 16k ctx | the H0 baseline |
| **Qwen2.5-Coder-14B** | 14B | ~8.5 GB | ✓ at 8k ctx with `-ngl 30` | parked candidate per ADR-060 / lab notebook §6 |

Lean: **Qwen2.5-Coder-3B as the LoRA target.** Reasoning: small enough that LoRA fine-tunes fit easily on rented A100/H100, large enough to hold Unheaded-specific knowledge after training, leaves headroom on the inference card for 16k+ ctx. The pre-existing forge/RAFT pipeline (`crates/zhenai-forge/`) is built around Gemma-4-E2B which is in the same parameter class — much of the training plumbing transfers.

(If the rented compute is generous, the experiment can swing to 7B base or 14B base. The decision sequence below scales.)

### Training data

The corpus already in this repo is the seed:

- `docs/adr/` — 60+ ADRs (architectural reasoning Stevie wants the model to internalise)
- `docs/battle-plans/` — sprint plans (the operational workflow shape)
- `crates/`, `services/`, `cmd/`, `pkg/` — the Go + Rust source tree (~400 K LOC)
- `eval/coding-gate/prompts.jsonl` — the 14 textbook + 14 hard + 14 expert + 14 prod-edge prompts as gold-labeled training examples (with the answers we'd grade as PASS, hand-written or distilled from Claude)
- `references/timeline.md` + ages — historical project state
- Wiki pages — narrative explanations
- Stevie's writing (the rhetorical voice we'd want preserved)

A separate retrieval-augmented training pass uses the existing forge RAFT pipeline (`crates/zhenai-forge/src/raft.rs` and friends) — every training example pairs a question with the canonical reference document. The model learns to answer with citations from day one.

Out-of-scope for the seed corpus: scraped Stack Overflow, Wikipedia, RFCs. Those belong in vor (retrieval-time, ADR-056) not in the model's weights. The training corpus is **deliberately Unheaded-only** so the model is honest about what it knows.

### Evaluation gate (locked before training starts)

Reuse the H0 14-prompt textbook tier as the no-regression bar:

- **Necessary condition**: trained model scores ≥ 12/14 (qwen-7b's empirical baseline) on textbook-14 with the existing rubric.
- **Sufficient win condition**: trained model scores ≥ 12/14 PLUS demonstrably better on a NEW Unheaded-specific bench (~30 prompts about Unheaded internals where qwen-7b has to lean on retrieval) WITHOUT retrieval available.
- **Stretch win**: the trained model handles the 14 hard-tier and 14 expert-tier prompts (concurrency / TOCTOU / async-mutex / loop-variable-capture) at qwen-7b ± 2 prompts — this would prove the LoRA didn't just memorise the textbook examples.

If only the necessary condition holds: keep training, no shipping yet.
If sufficient holds: ship as the new default chat model, demote qwen-7b to the multi-model-selector option (ADR-060 activated as a side effect).
If the trained model regresses below 10/14: rollback, retrain with different hyperparameters; don't ship a worse model just because it's ours.

### Compute budget — rough estimate

Forge already has measured numbers for Gemma-4-E2B LoRA training on Stevie's local card (5.7s/step warm at seq=384). For a comparable Qwen-2.5-Coder-3B run on rented A100-80GB:

- Step time: ~1-2 s warm (8x faster than local)
- 500 steps for first vetting run: ~10-15 minutes wall-time
- 5000 steps for a serious run: ~2-3 hours
- A100 spot pricing: ~$1-2/hr depending on provider
- **Total budget for a single training experiment: ~$5-10**, not the $hundreds it would be on H100s

That's well within "try a thing" budget. If results are promising, scale to longer training (~$30-50) or larger base model (~$100-200) without further ADR amendments.

### Runtime delivery

Once trained:

1. Convert LoRA → merged GGUF via the existing `crates/zhenai-forge/src/gguf.rs` path or `llama.cpp` convert tooling.
2. Drop into `/var/zhen/models/unheaded-coder-3b-v1.gguf`.
3. Add `unheaded-3b` key to `scripts/switch-model.sh` with appropriate launch flags.
4. Re-run the H0 coding gate against the new model.
5. If H0 passes: update `ZHEN_MODEL` default + welcome message in `raft/static/index.html`.
6. Optionally activate ADR-060 (multi-model selector) so the operator can A/B compare qwen-7b vs unheaded-3b on the fly.

---

## BlackMage lens — supply-chain considerations

A locally-trained model has a different threat profile than a downloaded GGUF:

### Improvements

- **Trust provenance is in-house.** No "did bartowski's quant pipeline poison this?" question. The training data is what we wrote, the training pipeline is what we audit, the resulting weights are what we sign.
- **Watermarking is feasible.** Embed a known-canary prompt-response pair; if the model later appears in some other context, that's evidence of weights leakage.
- **Deterministic reproduction.** With a pinned seed + frozen training data + frozen pipeline, anyone with the same compute can reproduce the model byte-identical. Useful for `gungnir` (ADR-043) sealed-payload distribution if we ever ship the model to other Kingdom hosts.

### New risks

- **Training-data poisoning.** If a malicious commit lands in `docs/` or `cmd/` between training runs, the next model bakes that commit's content into its weights. Mitigation: training pipeline cuts a git SHA snapshot at run start; subsequent retrain is gated on the diff between snapshots being clean (no ADR/source-tree alterations the operator didn't approve).
- **Cloud-rented GPU side-channels.** Renting an A100 means trusting the provider not to snapshot the running container's memory mid-training (which would leak the training data + the in-progress weights). Mitigation: encrypt the data archive client-side before upload; train inside a VM where memory is protected; for any provider that doesn't offer memory-attestation, treat the training data as already-public (the corpus IS GPL-3.0 in this repo, so this is mostly fine — but personal notes / private project data are a real consideration if we expand the corpus).
- **Model exfil at delivery.** The trained weights need to come back to Stevie's local box. Encrypted artifact transfer (rsync over SSH, signed) is the obvious path; not a new mitigation, just discipline.

### Open question — IP / licensing

Unheaded is GPL-3.0. Models trained on GPL-3.0 corpus inherit... it's not actually clear. The legal consensus on "model weights as derived works of training data" is unsettled across jurisdictions. **This isn't a blocker** — Stevie's stated direction (community-first doctrine, CLAUDE.md) is to share Unheaded outputs freely — but it's a footnote that needs Barrister review before any public model-weights release. For internal use on Stevie's LAN, irrelevant.

---

## Why this is parked, not started

1. **The current default works.** qwen-7b clears H0 at 12/14. Spending $50 + a sprint to get to 13/14 isn't compelling. The case strengthens significantly when Stevie has a *specific* Unheaded-knowledge query that retrieval-augmented qwen mishandles repeatedly — that's the trigger.
2. **Forge isn't H0-clean yet.** WAVE13 verdict was RETRAIN; the existing Kingdom RAFT LoRA work has measured eval-loss descent but no end-to-end "model-served-via-llama.cpp clears the coding gate" demo. Until that shipping path is proven once on Gemma-4-E2B, there's no value in pointing the same pipeline at a new base model.
3. **The cloud-cost-and-effort threshold is set by what we're chasing.** Stevie hasn't articulated what specifically the new model would do better. *"Solve coding problems we've already trained on"* is one answer; *"answer Unheaded-internals questions without RAG"* is another; *"be the offline-LAN-only chat model when vor is down"* is a third. Each motivates a different training data mix. **First step on activation: pin the goal.**

---

## Activation checklist (when Stevie schedules)

- [ ] Articulate the single most-important capability the new model should beat qwen-7b on. Write it down.
- [ ] Choose base model from the table above. Lean: Qwen2.5-Coder-3B.
- [ ] **Verify quant + KV-cache math fits OUR specific prompts BEFORE downloading or training.** Lesson from the gemma + deepseek vetting (2026-05-04): published "fits in 12 GB" benchmarks are computed against synthetic short prompts; zhen_app's RAG-grounded prompts add 1-2 GB of KV-cache pressure that the public numbers don't account for. Empirical failure mode: deepseek-Q4_K_M boots at 11.8/12 GB then OOMs mid-matmul on the first real RAG query. Math the cost in advance.
- [ ] Inventory the training corpus. Decide what's in/out (especially: any private data the cloud renter shouldn't see).
- [ ] Pin a cloud provider + a budget cap (e.g. "stop at $50, regardless of progress").
- [ ] Lift the existing forge pipeline to cloud (containerise, push, run). The forge container probably needs ROCm → CUDA tweaks for A100/H100; budget a half-day for that.
- [ ] First training run: 500 steps on the smallest viable base. Measure eval loss + a quick coding-gate spot-check.
- [ ] If first run looks promising: scale to a real run (2000-5000 steps). Re-run full H0 + Unheaded-knowledge bench.
- [ ] Apply the locked decision rule (necessary / sufficient / stretch above) to ship-or-rollback.
- [ ] Document the run + result in a new lab notebook under `eval/training-runs/<date>/` (mirror the gemma-vet-2026-05-04 structure).
- [ ] If ship: amend this ADR's status to **In Progress → Shipped**, file a new ADR for whatever the next training iteration is.

---

## References

- `eval/coding-gate/gemma-vet-2026-05-04/LAB-NOTEBOOK.md` — the negative-result chain that produced this direction
- `crates/zhenai-forge/` — the existing local-training pipeline (Gemma-4-E2B-targeted; needs adaptation for cloud + new base)
- ADR-018 — RAFT Training Pipeline (the pipeline this ADR's training would use)
- ADR-030 — Zhenai Forge / Rust LoRA training (current "in progress" base for the kit)
- ADR-051 — WAVE13 verdict: RETRAIN (the immediate predecessor; first thing to clear before this ADR activates)
- ADR-056 / ADR-057 — auxiliary corpus + source-code indexing (retrieval side; complementary not competing)
- ADR-060 — multi-model selector (the post-training A/B compare seam)
- `docs/security/application-threat-model.md` — T1-T10 catalog this ADR's training-supply-chain threats extend
- Stevie's note (verbatim, 2026-05-04): *"I am starting to think renting a gpu to train something we know will run and fit that is trained on our preferred data might be the move."*
