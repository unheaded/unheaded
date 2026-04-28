> **In-tree canonical WAVE13 plan.** Imported 2026-04-27 from in-tree draft per ADR-052 (Source-of-Truth policy).
> Battle plans of record live here in `docs/battle-plans/`. Off-tree drafts in `~/.claude/plans/` are working scratch only.
> Marshal-citable: any future WAVE13 plan changes update THIS file.

# WAVE13 BATTLE PLAN — Forge Inference: Validate the Kingdom LoRA + Wire to Zhen

**Date forged:** 2026-04-23 (imported in-tree 2026-04-27)
**Sprint:** WAVE13 — give zhenai-forge a `generate` subcommand, validate that the WAVE12 Kingdom LoRA produces sensible text (not just descending CE), then wire forge inference into the zhenai service so Zhen actually uses what we trained.
**Prerequisite:** WAVE12 closed (HEAD `2f44309c` or descendant). Kingdom LoRA at `raft/kingdom-w12/kingdom.lora.gguf`. Gemma-4 GGUF at `/var/zhen/models/gemma-4-E2B-it.gguf`. zhen-inference (Mistral-7B + llama.cpp) running at port 20100.
**Target:** `zhenai-forge generate --model gemma-4-E2B-it.gguf --lora kingdom-w12.lora.gguf --prompt "..." --max-new-tokens 100` produces text on stdout. AND a side-by-side base-vs-LoRA comparison on 5-10 held-out Kingdom prompts shows qualitative difference. AND a Zhen query routes through forge generate.
**Estimated Duration:** 6-10 wall-clock hours across 1-2 sessions.
**Agent Strategy:** solo/coordinator. Marshal oversight on Phase 2 quality call.
**Commit cadence:** every 5 plan steps + phase exit. `--no-gpg-sign` per AFK policy.
**Stuck Protocol:** skip after 3× time estimate or 2 failed debug attempts. Preserve known-good state before skip.

---

## North star

The WAVE12 Kingdom LoRA reduced held-out CE loss from 21.10 to 6.78 — a strong learning signal. But CE is per-token; it doesn't tell us if the model **generates** Kingdom-quality text. WAVE13's job is to find out, then bridge forge to Zhen if the answer is yes.

**Minimum success:**
1. `zhenai-forge generate` works on Gemma-4 + LoRA, GPU path.
2. Side-by-side base vs LoRA on 5+ held-out prompts is recorded in repo.
3. Zhen has a route to forge generation (HTTP endpoint or CLI shim, whichever is cleanest).

**Stretch:** swap Zhen's primary inference path from llama.cpp/Mistral-7B to forge/Gemma-4+Kingdom-LoRA.

---

## Phases

| Phase | Name | Wall | Cumulative | Risk |
|------:|------|----:|-----------:|------|
| 0 | Preflight + state confirm | 30m | 0.5h | low |
| 1 | `generate` subcommand (greedy + sampling) | 2.5h | 3h | medium — first inference path |
| 2 | Kingdom LoRA quality check (side-by-side) | 1h | 4h | low — observation only |
| 3 | DECIDE: is the LoRA generative-quality? | 15m | 4.25h | — |
| 4 | (if 3=yes) Forge HTTP serve mode | 2h | 6.25h | medium |
| 5 | (if 3=yes) Wire zhen-inference to call forge | 1h | 7.25h | low — small protocol shim |
| 6 | (if 3=no) Failure analysis + WAVE14 spec | 1h | 5.25h | low |
| 7 | ADR-051 + memory + handoff | 1h | 7-8h | low |

---

## Critical facts (cached for the executing agent)

- **Repo:** `/home/govan/tmp/unheaded/`, branch `main`, commits unsigned per session policy.
- **Crate:** `crates/zhenai-forge/`. Binary at `crates/zhenai-forge/target/release/zhenai-forge`.
- **Forward primitive:** `gemma4_gpu::forward_gemma4_gpu(cpu, gpu, lora, tokens) -> (Vec<f32> logits, Vec<LayerCache>)`. Logits shape `[seq, vocab=262144]`. Final token's logits at `&logits[(seq-1)*vocab..]`.
- **LoRA load:** `Gemma4LoraAdapters::load(path) -> std::io::Result<Self>`.
- **Existing tokenizer:** `crates/zhenai-forge/src/tokenizer.rs` does encode/decode against a vocab Vec. `extract_vocabulary_from_gguf(path)` pulls the vocab from a GGUF. **Untested on Gemma-4** — vocab=262144 is much bigger than Mistral's 32k.
- **Reference Python tokenizer** (already used for WAVE12 corpus): `scripts/tokenize-kingdom-for-gemma4.py` via `/home/govan/tmp/gemma4-venv/bin/python` (HF AutoTokenizer + Gemma-4 chat template).
- **Held-out eval set:** `/tmp/24h-kingdom-eval.jsonl` (397 sequences). Each line `{"tokens":[…], "answer_start":N}`. `answer_start` marks the first model-response token; we want to generate *from* there.
- **Kingdom corpus formatting:** Gemma-4 chat template `<start_of_turn>user … <end_of_turn>\n<start_of_turn>model … <end_of_turn>`. Stop tokens = `<end_of_turn>` (id ≈ 107).
- **VRAM budget:** PleMode::Cpu = 4.57 GB. Rest is scratch (≈0.07 GB). Plenty of headroom.
- **Per-token forward at seq≈400:** ≈0.4-1s based on Phase 7 eval timing. 100 new tokens → 40-100s per generation. Acceptable for sanity checks; needs KV-cache for serving.
- **KV-cache: NOT IMPLEMENTED.** Each new token currently re-runs the full forward over the whole prefix. WAVE13 ships *without* KV-cache (correctness first); WAVE14+ adds it for serving throughput.

---

## Preflight hypotheses (Phase 0 verifies)

| # | Hypothesis | Verification |
|--:|-----------|--------------|
| H1 | HEAD clean, descends from WAVE12 (`2f44309c` or later) | `git log -1; git status` |
| H2 | Kingdom LoRA on disk + loadable | `ls raft/kingdom-w12/kingdom.lora.gguf` + smoke test load |
| H3 | Held-out eval JSONL still in /tmp | `ls /tmp/24h-kingdom-eval.jsonl`; if not, re-tokenize via Appendix A1 |
| H4 | tokenizer.rs `extract_vocabulary_from_gguf` works on Gemma-4 GGUF | smoke test prints vocab size = 262144 |
| H5 | zhen-inference / zhen-web-ui current state — running? what model? | `pgrep -af "llama-cli\|zhen"`, port checks 20100/20103 |

---

## Phase 0 — Preflight (Steps 1-12) — 30 min

**Goal:** Verify ground state and inventory existing inference primitives.

- [ ] **Step 1** [B] Verify clean WAVE12 HEAD: `git log -1`, `git status`.
- [ ] **Step 2** [V] HEAD is `2f44309c` or descendant; tree clean.
- [ ] **Step 3** [B] Confirm Kingdom LoRA + corpus: `ls -lh raft/kingdom-w12/kingdom.lora.gguf /tmp/24h-kingdom-eval.jsonl`. If eval missing, re-tokenize per Appendix A1.
- [ ] **Step 4** [B] Inventory tokenizer.rs: `grep -n "pub fn" crates/zhenai-forge/src/tokenizer.rs`; check what's available.
- [ ] **Step 5** [B] Smoke-test extract_vocabulary_from_gguf on Gemma-4: write a tiny `cargo test` (or add to existing tests) that loads the GGUF and asserts `vocab.len() == 262144`. If the function doesn't support Gemma-4 SPM format, scope expands.
- [ ] **Step 6** [V] Smoke passes: vocab loaded, can encode/decode at least ASCII text.
- [ ] **Step 7** [B] Inventory Zhen state: `curl -s localhost:20100/health` and `curl -s localhost:20103/health`. Identify what's running and what protocol they speak. Document in session log.
- [ ] **Step 8** [B] Reproduce eval-gemma4 quick check: 1-seq forward on the LoRA to confirm current binary still works. `crates/zhenai-forge/target/release/zhenai-forge eval-gemma4 --model … --data /tmp/24h-kingdom-eval.jsonl --lora raft/kingdom-w12/kingdom.lora.gguf --answer-start 1` interrupted after a few sequences. Discard log.
- [ ] **Step 9** [W] Initialize `crates/zhenai-forge/notes/wave13-session-log.md` with header + Phase 0 baseline.
- [ ] **Step 10** [V] All preflight green.
- [ ] **Step 11** [C] **PHASE 0 COMMIT** — session log only.
- [ ] **Step 12** [V] Phase 0 closeout. ABORT-IF: H1, H2, or H4 fails.

---

## Phase 1 — `generate` subcommand (Steps 13-45) — 2.5 hours

**Goal:** Implement `cmd_generate_gemma4` in `main.rs`. Token-by-token decode loop using the existing forward path. Greedy + temperature sampling.

### Design

```rust
fn cmd_generate_gemma4(args) {
    // Load model + (optional) LoRA + tokenizer vocab.
    // Encode prompt:
    //   --prompt <text>          — plain text, no chat template
    //   --gemma-prompt <text>    — wrap with Gemma-4 chat template
    //   --tokens <jsonl-line>    — pre-tokenized, byte-identical to training format
    // Loop max_new_tokens times:
    //   logits = forward_gemma4_gpu(...).0
    //   pick = sample(logits.last_row, temperature, top_k, top_p)
    //   if pick == EOS or pick in stop_tokens { break }
    //   tokens.push(pick); decode token, write to stdout (or buffer).
    // Print final output.
}
```

CLI:

```
zhenai-forge generate --model <gguf>
                      --prompt <text> | --gemma-prompt <text> | --tokens <jsonl-line>
                      [--lora <gguf>]
                      [--max-new-tokens 100]
                      [--temperature 0.0]   # 0.0 = greedy
                      [--top-k 0]           # 0 = no top-k
                      [--top-p 1.0]
                      [--seed 0]
                      [--stop "<end_of_turn>,<eos>"]
                      [--cpu]
```

### Steps

- [ ] **Step 13** [R] Re-read forward_gemma4_gpu signature; identify how to slice last-row logits.
- [ ] **Step 14** [CODE] Add `cmd_generate_gemma4` skeleton: arg parse, model load, LoRA load.
- [ ] **Step 15** [CODE] Add a greedy-only token loop first (deterministic, simpler debug). Stop on max_new_tokens.
- [ ] **Step 16** [CODE] Add tokenizer decode wired to vocab from GGUF.
- [ ] **Step 17** [BUILD] Build green.
- [ ] **Step 18** [TEST] Smoke run: `--prompt "The quick brown fox"`, max_new_tokens=10, base model only (no LoRA). Verify text comes out. Argmax greedy → probably repetitive but should be coherent letters/words.
- [ ] **Step 19** [V] Output is non-garbage (printable text, no NaN crash).
- [ ] **Step 20** [C] **COMMIT CHECKPOINT 1** — base greedy generate works.
- [ ] **Step 21** [CODE] Add temperature sampling (single pass, no top-k/top-p yet). Use deterministic LCG from `--seed`.
- [ ] **Step 22** [CODE] Add top-k filter (zero out logits below top-k).
- [ ] **Step 23** [CODE] Add top-p (nucleus) filter.
- [ ] **Step 24** [BUILD] Build green.
- [ ] **Step 25** [TEST] Smoke at temperature=0.7, top-p=0.9: same prompt produces different but coherent continuations on different seeds.
- [ ] **Step 26** [V] Sampling diverges by seed; same seed reproduces.
- [ ] **Step 27** [C] **COMMIT CHECKPOINT 2** — sampling lands.
- [ ] **Step 28** [CODE] Add `--gemma-prompt` mode: shells out to `/home/govan/tmp/gemma4-venv/bin/python` invoking `scripts/tokenize-kingdom-for-gemma4.py` (or a sibling) to apply the chat template. Read tokens back from stdout.
  - **Why subprocess vs in-process:** the Gemma-4 SentencePiece model is 4MB+; embedding it into forge means another file to manage. The Python venv is already proven for WAVE12 corpus; reuse it. Only matters at inference-launch time.
- [ ] **Step 29** [CODE] Add `--tokens` mode: read a `{"tokens":[…]}` JSONL line as already-tokenized prompt.
- [ ] **Step 30** [CODE] Add stop-token logic (default = Gemma-4 `<end_of_turn>` id, looked up from vocab; fallback to vocab-end).
- [ ] **Step 31** [BUILD] Build green.
- [ ] **Step 32** [TEST] `--gemma-prompt "What is the Kingdom?"` (or any plain question), no LoRA. Output should be incoherent-but-formed (base Gemma-4 had no Unheaded context).
- [ ] **Step 33** [V] Stop tokens trigger early termination correctly.
- [ ] **Step 34** [C] **COMMIT CHECKPOINT 3** — chat template + stops land.
- [ ] **Step 35** [TEST] First LoRA-on generation: `--gemma-prompt "What is Wotan?"` `--lora raft/kingdom-w12/kingdom.lora.gguf` `--max-new-tokens 100` `--temperature 0.0` (greedy). Compare to base-only.
- [ ] **Step 36** [V] **PHASE 1 EXIT GATE** — generate produces text on stdout for all four invocation modes (`--prompt`, `--gemma-prompt`, `--tokens`, with-and-without LoRA). No NaN, no crash.
- [ ] **Step 37** [W] Update session log with Phase 1 timing + sample outputs.
- [ ] **Step 38** [C] **PHASE 1 COMMIT.**
- [ ] **Step 39-45** Reserved for intra-phase rework / debug.

### Debug branches

- **D1: vocab decode produces garbage / replacement chars.** Means `extract_vocabulary_from_gguf` doesn't handle Gemma-4 SPM. Fallback: shell out detokenization to Python venv too (slower but correct). [STUCK] flag if this blocks > 30 min.
- **D2: chat template subprocess hangs.** Use a fixed timeout (10s) on the Python invocation; if it hangs, use `--tokens` mode with a pre-baked Gemma-prompt JSONL file from corpus.
- **D3: forward NaN on long sequences.** Existing forward was tested on seq=384. If new tokens push seq > 384 and NaN appears, cap max_new_tokens such that total seq ≤ 384 for now; WAVE14 adds proper extrapolation.

---

## Phase 2 — Kingdom LoRA quality check (Steps 46-65) — 1 hour

**Goal:** Generate side-by-side base-vs-LoRA outputs on 5-10 held-out Kingdom prompts. Capture in a markdown doc. **Decide whether the LoRA is generative-quality.**

### Steps

- [ ] **Step 46** [B] Pick 10 held-out prompts: extract user-portion of first 10 sequences from `/tmp/24h-kingdom-eval.jsonl` (decode to text, strip the model-response half).
- [ ] **Step 47** [W] Save prompts to `crates/zhenai-forge/notes/wave13-quality-prompts.md` as numbered list.
- [ ] **Step 48** [B] For each prompt, generate base output (no LoRA), 100 tokens, temp=0.0.
- [ ] **Step 49** [B] For each prompt, generate LoRA output, 100 tokens, temp=0.0.
- [ ] **Step 50** [W] Save side-by-side comparison to `crates/zhenai-forge/notes/wave13-quality-results.md`. Format:
  ```
  ## Prompt 1
  > <user text>
  
  **Base (no LoRA):**
  <output>
  
  **With Kingdom LoRA:**
  <output>
  ```
- [ ] **Step 51** [B] Sample at temperature=0.7 too for the first 3 prompts (3 seeds each). Capture in same doc. Catches mode-collapse vs greedy hiding it.
- [ ] **Step 52** [W] Add a 5-line "first impressions" summary at the top of the doc.
- [ ] **Step 53** [V] **PHASE 2 EXIT GATE** — 10 base + 10 LoRA + 9 sampled outputs captured to disk.
- [ ] **Step 54** [C] **PHASE 2 COMMIT.**
- [ ] **Step 55-65** Reserved.

---

## Phase 3 — DECIDE: is the LoRA generative-quality? (Steps 66-72) — 15 min

**Goal:** read the side-by-side. Make a defensible call.

- [ ] **Step 66** [R] Read `wave13-quality-results.md` end-to-end.
- [ ] **Step 67** [DECIDE] Categorize the LoRA outputs into one of:
  - **A: clearly better.** LoRA outputs reference Unheaded concepts correctly, follow Q&A format, end appropriately. → continue to Phase 4 (HTTP serve) and Phase 5 (Zhen wire).
  - **B: directionally better but rough.** LoRA outputs are more Kingdom-flavored but degenerate (loops, stops too early, misuses terms). → still continue but flag as "needs more training" in Phase 7 ADR.
  - **C: indistinguishable / worse.** LoRA didn't actually learn generative behavior, just minimized token-level CE on training distribution shape. → skip to Phase 6 (failure analysis), defer Zhen wire to WAVE14.
- [ ] **Step 68** [W] Record verdict in session log with rationale (3-5 sentences, evidence-based — quote specific outputs).
- [ ] **Step 69** [C] **PHASE 3 COMMIT** — decision durable in git.
- [ ] **Step 70-72** Reserved.

---

## Phase 4 — Forge HTTP serve mode (Steps 73-105) — 2 hours

**Goal:** `zhenai-forge serve --port 20104 --model … [--lora …]` listens for HTTP POST `/generate` and streams generated text. Compatible-ish with the prevailing OpenAI-completion-style API for easiest Zhen integration.

**Prerequisite:** Phase 3 verdict = A or B.

### Design

Endpoints:
- `GET /health` — `{"ok": true, "model": "...", "lora": "..."}`
- `GET /ready` — same as /health, 200 only after weights uploaded
- `POST /generate` — JSON body:
  ```json
  {"prompt": "string", "max_new_tokens": 100, "temperature": 0.0,
   "top_k": 0, "top_p": 1.0, "stop": ["<end_of_turn>"], "stream": false}
  ```
  Returns:
  - `stream=false`: `{"text": "...", "tokens": N, "elapsed_s": F}`
  - `stream=true`: SSE `data: {"token": "..."}` per token, `data: [DONE]`

Use minimal HTTP machinery — `std::net::TcpListener` + hand-rolled parsing, NO new crate dependency (ADR-004). Single-threaded; serialized request handling. Concurrent requests serialize at the GPU anyway.

### Steps

- [ ] **Step 73** [CODE] Add `cmd_serve_gemma4` in main.rs. Listener bind + accept loop.
- [ ] **Step 74** [CODE] Tiny HTTP/1.1 parser: read request line + headers + body until Content-Length bytes consumed. Reject anything fancy.
- [ ] **Step 75** [CODE] Tiny JSON parser for the request body (or pull a single trivial JSON impl from existing pkg). Extract prompt + sampling params.
- [ ] **Step 76** [CODE] Wire the existing generate primitive (extracted as a reusable function in 4A) into the request handler.
- [ ] **Step 77** [CODE] /health and /ready handlers.
- [ ] **Step 78** [BUILD] Build green.
- [ ] **Step 79** [TEST] `curl localhost:20104/health` returns `{"ok":true,...}`.
- [ ] **Step 80** [TEST] `curl -X POST localhost:20104/generate -d '{"prompt":"Hello","max_new_tokens":20}'` returns `{"text":"...","tokens":20,...}`.
- [ ] **Step 81** [V] /generate works end-to-end.
- [ ] **Step 82** [C] **COMMIT CHECKPOINT 4** — serve mode lands.
- [ ] **Step 83** [CODE] Add SSE streaming for `stream=true`.
- [ ] **Step 84** [TEST] Streaming works in curl: tokens arrive incrementally.
- [ ] **Step 85** [V] Streaming green.
- [ ] **Step 86] [CODE] Add request log to stderr (ts, prompt-prefix, tokens-out, elapsed).
- [ ] **Step 87] [V] **PHASE 4 EXIT GATE** — health + ready + generate (sync + stream) all working over HTTP. Concurrent requests serialize cleanly (try 3 in parallel via xargs/`&`).
- [ ] **Step 88] [C] **PHASE 4 COMMIT.**
- [ ] **Step 89-105** Reserved.

### Debug branches

- **D4: HTTP parser misbehaves on header continuations / large bodies.** Be strict — reject unsupported syntax with 400. Don't try to handle every RFC 7230 corner case; this is internal-only.
- **D5: SSE buffering on the client side.** Set `Transfer-Encoding: chunked` and flush after each event. If curl still buffers, downgrade to a polling endpoint and document the limitation.

---

## Phase 5 — Wire zhen-inference to forge (Steps 106-130) — 1 hour

**Goal:** Zhen routes a query through forge generate. Simplest viable shape: zhen-web-ui (or zhen-inference) calls forge's HTTP endpoint instead of (or alongside) llama.cpp.

**Prerequisite:** Phase 4 exit gate passed.

### Steps

- [ ] **Step 106** [R] Read `cmd/zhen-inference/main.go` (or wherever the inference call lives) to find the LLM call site.
- [ ] **Step 107** [CODE] Add a `--forge-url http://localhost:20104` config option. When set, route /api/v1/generate to forge instead of llama.cpp.
- [ ] **Step 108** [CODE] HTTP client to forge: standard Go net/http, JSON marshal/unmarshal request/response bodies. Pass through prompt + sampling params.
- [ ] **Step 109** [BUILD] `go build ./cmd/zhen-inference/...`.
- [ ] **Step 110** [TEST] Start forge `serve` on 20104, start zhen-inference with `--forge-url …`, query Zhen via its existing API. Verify response came from forge (timing fingerprint + log entries).
- [ ] **Step 111** [V] **PHASE 5 EXIT GATE** — Zhen's main inference query path returns text generated by forge+Kingdom-LoRA.
- [ ] **Step 112** [W] Capture a sample Zhen Q&A in `wave13-zhen-integration.md`.
- [ ] **Step 113** [C] **PHASE 5 COMMIT.**
- [ ] **Step 114-130** Reserved for unforeseen integration friction.

### Debug branches

- **D6: zhen-inference's existing API differs significantly from forge's.** Add an adapter layer (still in-Go) that translates request shapes. Don't change forge's HTTP API — keep it minimal; let Zhen accommodate.
- **D7: Timeouts.** Forge generation at 100 tokens × ~0.5s/token = ~50s. zhen-inference's HTTP client may have a 30s timeout. Bump explicitly to 5min for the forge route.

---

## Phase 6 — (only if Phase 3 verdict = C) Failure analysis (Steps 131-150) — 1 hour

**Goal:** if the LoRA fails the generative-quality test, document why and queue WAVE14.

- [ ] **Step 131** [W] Capture concrete failure modes from Phase 2 outputs: top-k tokens that LoRA prefers, divergence point in generation, whether base outputs were equally bad, etc.
- [ ] **Step 132** [W] Hypothesize: corpus quality? answer-start parser bug (per ADR-050)? Insufficient steps? rank too low?
- [ ] **Step 133** [W] Spec WAVE14: "fix corpus / retrain / generative quality eval."
- [ ] **Step 134** [C] **PHASE 6 COMMIT** — analysis + WAVE14 stub doc.
- [ ] **Step 135-150** Reserved.

---

## Phase 7 — ADR-051 + memory + handoff (Steps 151-170) — 1 hour

- [ ] **Step 151** [W] Draft `docs/adr/ADR-051-wave13-forge-inference.md`. Sections: context (LoRA had CE descent but unproven generation), decision (ship generate + serve), result (verdict + Zhen integration status), consequences.
- [ ] **Step 152** [W] Update `docs/adr/ADR-INDEX.md`.
- [ ] **Step 153** [W] Memory entry `project_wave13_complete.md`. Update `MEMORY.md` index.
- [ ] **Step 154** [W] Update `CLAUDE.md` Age 3 status block: forge inference path landed; Zhen route to forge live (or deferred).
- [ ] **Step 155** [W] Finalize `wave13-session-log.md` with commit ledger.
- [ ] **Step 156** [C] **FINAL COMMIT.**
- [ ] **Step 157-170** Reserved.

---

## Definition of Done (Micromanager gate)

WAVE13 is DONE when ALL of these are true (or explicitly bypassed by Phase 3 verdict C):

- [ ] `zhenai-forge generate` works for `--prompt`, `--gemma-prompt`, and `--tokens` modes.
- [ ] Greedy + temperature + top-k + top-p sampling all work.
- [ ] Stop tokens halt generation correctly.
- [ ] 10 base + 10 LoRA outputs captured side-by-side in repo.
- [ ] Phase 3 verdict (A/B/C) recorded with evidence.
- [ ] If A or B: `zhenai-forge serve` HTTP endpoint live and tested.
- [ ] If A or B: zhen-inference routes through forge for at least one verified query.
- [ ] If C: WAVE14 spec stub committed.
- [ ] ADR-051 accepted + indexed.
- [ ] Session log complete with full commit ledger.
- [ ] Memory updated.
- [ ] CLAUDE.md Age 3 status block reflects WAVE13 outcome.
- [ ] No new crate dependencies (ADR-004 upheld).
- [ ] `git status` clean.

---

## Appendix A: Emergency procedures

### A1. Kingdom corpus eval JSONL missing

```bash
/home/govan/tmp/gemma4-venv/bin/python scripts/tokenize-kingdom-for-gemma4.py \
  < raft/training/eval.jsonl > /tmp/24h-kingdom-eval.jsonl
```
Verify line count = 397.

### A2. Tokenizer module can't decode Gemma-4 vocab

Use Python venv detokenization shell-out for now. Slower (~30ms/token) but correct. Add to `tokenizer.rs` follow-up TODO.

### A3. forward_gemma4_gpu produces NaN at long generation

Cap max_new_tokens such that prompt_len + new_tokens ≤ 384 (the trained range). Document the limit in the CLI help text. Real fix is positional embedding extrapolation — out of WAVE13 scope.

### A4. zhen-inference is hard-coupled to llama.cpp

Add `--forge-url` config; if unset, behave exactly as before. Don't break existing Zhen behavior; just add the route.

---

## Appendix B: Quick reference

### B1. Kingdom prompt extraction (Phase 2 prep)

The eval JSONL has `{"tokens":[…], "answer_start":N}` per line. To recover the user-side text:

```python
import json
import sys
from transformers import AutoTokenizer
tok = AutoTokenizer.from_pretrained("/home/govan/tmp/gemma-4-E2B-it")
for line in sys.stdin:
    obj = json.loads(line)
    user_tokens = obj["tokens"][:obj["answer_start"]]
    print(tok.decode(user_tokens))
```

### B2. Gemma-4 special tokens

| name | id |
|------|---:|
| BOS | 2 |
| EOS | 1 |
| `<start_of_turn>` | 106 |
| `<end_of_turn>` | 107 |
| `<pad>` | 0 |

(Verify in Phase 0 Step 5 against the actual GGUF metadata; numbers above are typical Gemma-3 / Gemma-4 conventions.)

### B3. Test prompt set (suggested for Phase 2)

```
1. What is Wotan in the Unheaded protocol?
2. Explain the Monad register format.
3. What does Sophia do?
4. How does the Kingdom dispatch packets?
5. What is a Shim in the UPC?
6. What is the Dream Ladder?
7. Who is the Computermancer?
8. What are the Three Crowns?
9. What is the Frosted Glass design system?
10. What does eBPF do in Unheaded?
```

(All have known answers in the corpus or related docs; gives a fair comparison.)

### B4. Commit message template

```
<type>(forge): [PLAN W13] Phase N — <one-line what>

<2-4 lines of why / what changed>

<regression note + sample output if applicable>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

---

*WAVE13 Battle Plan — Forged 2026-04-23.*
*7 phases, ~170 step slots, generation-first then-Zhen-integration.*
*Validate the Kingdom LoRA. Bridge forge to Zhen. Functional zhenai is the goal.*
