# WAVE16 — Overnight Model Vetting Battle Plan

**Date:** 2026-05-04
**Owner:** unheaded-warmonger (battle plan), unheaded-marshal (lane enforcement)
**Status:** EXECUTING (unattended, overnight authorization granted)
**Trigger:** Stevie's directive 2026-05-04: *"Yes + /unheaded-warmonger battle plan to test and implement all this overnight — work unattended with /unheaded-marshal so I wake up to a few models to try and compare"*

---

## Mission

Wake-up deliverable: Stevie has 2-3 candidate coding models bench-tested, side-by-side comparison, recommendation per candidate (adopt-default / keep-as-option / reject-with-reason). All artifacts in-repo, committed.

**North star (per `feedback_unsigned_commits_when_afk.md` + standing rule):** the H0 14-prompt textbook tier on `eval/coding-gate/prompts.jsonl` is the gate. A candidate must score ≥12/14 to be considered for adoption. Lower scores rejected, documented, archived.

---

## Candidate slate (locked)

| Key | Model | Quant | Source | Expected VRAM | Architecture risk |
|---|---|---|---|---|---|
| `qwen-coder-14b` | Qwen2.5-Coder-14B-Instruct | Q4_K_M | bartowski/Qwen2.5-Coder-14B-Instruct-GGUF | ~8.5 GB w + 1.6 GB KV @ 8k | dense, same family as the H0 baseline qwen-7b |
| `qwen3-14b` | Qwen3-14B (general) | Q4_K_M | bartowski/Qwen3-14B-GGUF | ~9-10 GB | newer family, may need different chat template |
| `deepseek-q5-cpu` | DeepSeek-Coder-V2-Lite Q5_K_M | Q5_K_M | bartowski/DeepSeek-Coder-V2-Lite-Instruct-GGUF | ~11 GB w + KV; needs `--n-cpu-moe N` | known MLA-no-flash-attn issue from earlier vet; Q5 is *bigger* than the Q4 that already OOM'd at 8k ctx — high risk of repeat failure |

**Rejected before download (explicit, with reasons):**
- Mistral-Small-24B Q3 — 5-8 t/s with heavy offload; slower than deepseek-cpu was, no quality compensation likely
- ExLlamaV2 backend — different runtime; ADR-060 sibling, separate work

---

## Phases

### Phase 0 — Pre-flight ✓ (snapshot at 2026-05-04 05:25 UTC)

Captured baseline:
- 11/11 services green
- qwen-7b serving on llama-server :8081
- VRAM 6.0 / 12.0 GB used
- RAM 4.8 / 14 GB used (9.5 GB available)
- Disk 1.4 TB free at `/var/zhen/models/`

### Phase 1 — Documentation (in-flight)

1. ✓ ADR-060 drafted (`docs/adr/ADR-060-zhenai-multi-model-selector.md`)
2. ✓ ADR-061 drafted (`docs/adr/ADR-061-cloud-rented-training-purpose-built-coding-model.md`)
3. ✓ ADR-INDEX.md updated with both
4. ✓ ADR-061 activation checklist amended with the KV-cache lesson
5. → THIS battle plan
6. → `.gitignore` update so 25 GB of GGUFs never get committed

### Phase 2 — Download (parallel)

Three GGUFs downloaded in parallel via `curl -L --fail` to `/var/zhen/models/`. Magic-bytes verification after each (`head -c 4 | xxd` must show `GGUF`). Each download is ~9 GB; total ~25 GB. Bandwidth-bound (~10-30 min on home connection).

If a single download fails: log to lab notebook with HTTP error + skip that candidate. **No retries** — the bandwidth-spent budget is finite, and a 9 GB redownload is not free.

### Phase 3 — Per-candidate vetting (sequential)

For each successfully-downloaded candidate, in order:

1. Add the key to `scripts/switch-model.sh` (`MODEL_FILE`, `MODEL_FLAGS`, `MODEL_NAME` arrays).
2. `./scripts/switch-model.sh <key>` — atomic swap (with timeout extended to 300s for cold-cache load).
3. Smoke direct: one `/v1/chat/completions` call to verify it answers.
4. Smoke through RAG: one `/api/v1/query` call to verify zhen_app can drive it.
5. Bench: `./eval/coding-gate/run-gemma-vet.sh passE-<key>-1500 1500` — 14 prompts, 1500 max_tokens.
6. Capture VRAM + RAM at idle + during inference (rocm-smi + free).
7. **HARD HALT criteria:** GPU OOM, ROCm error, llama-server crash, zhen_app green-state regression. On hit: rollback to qwen-7b, log halt note, skip remaining steps for this candidate, move to next.
8. **SOFT REDIRECT:** bench takes >30 min → abort that candidate, log "exceeded budget", move on.

After last candidate: `./scripts/switch-model.sh qwen-7b` to restore.

### Phase 4 — Comparison + lab notebook expansion

Append to `eval/coding-gate/gemma-vet-2026-05-04/LAB-NOTEBOOK.md`:

- Pass E — Qwen2.5-Coder-14B (`passE-qwen-coder-14b-1500__*`)
- Pass F — Qwen3-14B (`passF-qwen3-14b-1500__*`)
- Pass G — DeepSeek-Coder-V2-Lite Q5 + cpu-moe (`passG-deepseek-q5-cpu-1500__*`)

Each pass section captures: mechanical metrics, sample-of-3 quality reads, hypothesis verdict (H1/H2/H3/H4), recommendation card.

Final table: `qwen-7b` (Pass A) vs each candidate, recommended action per candidate.

### Phase 5 — Restore + intermediate commit

- llama-server serving qwen-7b
- zhen_app reports `model: qwen2.5-coder-7b-instruct`
- All 11 services green
- Tree-state commit: ADRs + lab notebook + battle plan + switch-model.sh updates + .gitignore update for new GGUFs

### Phase 6 — ADR-060 implementation (Stevie added 2026-05-04, mid-run)

> *"appeend to plan adr 60 implmentation"*

Stevie wants the multi-model selector UI shipped tonight too, building on whatever candidate set Phase 3 produces. Activation criteria from ADR-060 are met by definition (we'll have multiple working models after Phase 4). Run **in parallel with Phase 2 downloads** since downloads are I/O-bound and Phase 6 is CPU/typing-bound.

Per the ADR-060 contract (T11-T20 threat catalog + 12 unit + 8 integration tests):

#### 6.1 — Build `pkg/champion/modelswap.go` (with TDD-first tests)

- `pkg/champion/modelswap.go` — `ModelSwap(ctx, key string) error` plus an allowlist parsed once at boot from `scripts/switch-model.sh`.
- `pkg/champion/modelswap_test.go` — the 12 pre-registered tests (`TestAllowlistRejectsUnknownKey`, `TestAllowlistRejectsShellInjection`, `TestAllowlistRejectsPathTraversal`, `TestAllowlistRejectsNulByte`, `TestScriptHashMismatchHalts`, `TestConcurrentSwapBlocked`, `TestSwapTimeoutRollsBack`, `TestSwapEmitsZhenActionRow`, `TestFailedSwapEmitsZhenActionRow`, `TestSwapRefusesIfRunningAsRoot`, `TestSwapDispatchAcceptsDirectUserOnly`, `TestSwapAllowsDirectUser`).

#### 6.2 — Wire `cmd/zhen-agentd/toolexec_modelswap.go`

- Plug into existing `/api/v1/tool/exec` dispatch (already shipped as part of T6b closure).
- `tool=model_switch`, `args.key=<allowlisted>`, `args.justification=direct-user`.
- 8 integration tests under `_integration` build-tag (run on west, not in GHA CI).

#### 6.3 — `scripts/switch-model.sh --json` flag

- Add `--json` mode that emits a stable JSON status line on stdout (parseable by Go without breaking the human-readable default).
- Backward compatible: no flag = current behavior.

#### 6.4 — `raft/zhen_app.py` Python endpoints

- `GET /api/v1/models` — return the allowlisted keys parsed from `scripts/switch-model.sh` at zhen_app boot. Cache in process; reload on SIGHUP.
- `POST /api/v1/models/switch` — proxy to `cmd/zhen-agentd /api/v1/tool/exec` with `tool=model_switch` + `direct-user` justification. Returns 202 + a poll URL.

#### 6.5 — `raft/static/index.html` UI dropdown

- Sidebar `<select>` populated from `/api/v1/models`.
- On-change: POST to switch endpoint with optimistic-disable lock; show "swapping … ~Xs estimated" toast.
- Poll `/api/v1/stats.inference_model` until it matches the requested key, then re-enable.
- Hard timeout at 4× the documented expected boot time per `MODEL_FLAGS` → 504 + automatic rollback.

#### 6.6 — Smoke (manual verification, captured to lab notebook)

- Pick model A from dropdown → wait for swap → ask a question → verify model name flows through correctly.
- Pick model B → repeat.
- Try injection: `qwen-7b; rm -rf /` (URL-encoded) → must get 400 from `/api/v1/models/switch`, no subprocess invocation.
- Concurrent swap: two browser tabs both click swap simultaneously → second gets 429.

#### Phase 6 HARD HALT criteria (extends Phase 3 list)

- Any of the 12 pre-registered unit tests fails → halt; the safety contract is non-negotiable.
- Concurrent-swap test gets two subprocesses running simultaneously → halt; the lock isn't working.
- Injection test successfully runs the malicious payload → halt; this is the one failure mode that matters most. Roll back the entire commit, leave a HALT note.

---

## Marshal halt protocol

Per `feedback_unsigned_commits_when_afk.md`: gpg-agent times out → `--no-gpg-sign` on every commit during this run.

**HARD HALT** triggers (any one):
- Two consecutive candidates fail to load (suggests something fundamental broke)
- ROCm driver error not seen in this session's prior experiments
- zhen_app port 20103 unreachable for >60 s after a switch (means we broke the operator surface)
- Disk free at `/var/zhen/models/` drops below 5 GB (we'd be at risk of wiping in-progress downloads)

On HARD HALT:
1. Stop all downloads (kill curl).
2. Run `./scripts/switch-model.sh qwen-7b` (restore default).
3. Wait for /health 200 on :8081 + :20103.
4. Append "HARD HALT — phase X step Y — reason Z" section to lab notebook.
5. Commit current tree state.
6. Exit cleanly.

**SOFT REDIRECT** triggers:
- Single download bandwidth-flake (>5 min < 5 MB/s) → pause, move on, retry at end if budget allows
- Single candidate bench >30 min → abort that candidate, log overrun, move on
- gpg-agent timeout on a commit → `--no-gpg-sign` and continue

---

## Success criteria (wake-up deliverable)

- [ ] At least 2/3 candidates fully bench-graded
- [ ] Lab notebook has Pass E/F/G sections with tables + verdicts
- [ ] qwen-7b is the live default at end of run
- [ ] All 11 services green
- [ ] Single commit per phase (so partial progress survives any hard-halt)
- [ ] No GGUFs accidentally committed (gitignore in place before any `git add`)

If H1 PASSES on a candidate (≥12/14 H0): wake-up note explicitly says "candidate X beat qwen-7b on H0; recommend swap as default" — but **don't auto-swap the default** until Stevie reviews.

---

## Why the candidate set is exactly 3

- One candidate is too few (no comparison signal vs the 4-candidate gemma+deepseek slate from earlier)
- Five+ candidates is too many (download bandwidth + bench time burns the budget; we'd need to abandon some mid-run)
- Three candidates fits in the overnight budget assuming ~30 min download + ~15 min vet each = ~75-90 min total wall-time, leaves margin for one HARD HALT recovery

The slate is biased toward "models the research-of-the-day says are the right size" rather than "model variants we already have on disk", because the existing on-disk gemma + Q4_K_M deepseek are already disqualified.

---

## References

- `eval/coding-gate/gemma-vet-2026-05-04/LAB-NOTEBOOK.md` — predecessor experiment (gemma + deepseek-cpu rejected)
- `eval/coding-gate/run-gemma-vet.sh` — bench harness (reusable, parameterized)
- `scripts/switch-model.sh` — model-interchange seam
- `eval/coding-gate/prompts.jsonl` — the 14-prompt textbook tier
- ADR-060 — multi-model selector (the seam this experiment tests candidates for)
- ADR-061 — cloud-rented training (the alternate path if no candidate beats qwen-7b)
- `feedback_resource_awareness.md` — overridden tonight by explicit overnight authorization, but informs SOFT-REDIRECT triggers above
