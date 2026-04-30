# WAVE13 Phase 2 Quality Report

**Date executed**: 2026-04-28
**Executor**: Claude Opus 4.7 (autonomous overnight per Marshal charter)
**HEAD at execution**: `fb002223` (post-SPRINT-04-27 LOCAL fast-forward)
**Forge binary**: `crates/zhenai-forge/target/release/zhenai-forge` (rebuilt 2026-04-28 03:33)
**Source-of-truth plan**: `docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md` (in-tree per ADR-052)

## Packet amendments (S1, audit trail)

The packet as written had two design issues caught during execution; both
amendments preserve the packet's measurement intent:

1. **Prompt slicing (Section 2).** All 8 randomly-sampled eval sequences were
   exactly 384 tokens long — the trained-seq cap (`tokenize-kingdom-for-gemma4.py`
   truncates 99.8% of training Q&A pairs to MAX_TOKENS=384). With `--tokens=.tokens`
   as written, every prompt would have hit my forge generate's "prompt at MAX_SEQ,
   no room to generate" exit guard. Amended to `--tokens=.tokens[0:.answer_start]`
   so the model only sees the user/Context portion (26-160 tokens of generation
   headroom). This *also* matches the semantic intent of "respond to prompt"
   rather than "continue past completed Q&A".
2. **Section 5 decode is a no-op for this implementation.** My Phase 1
   `cmd_generate_gemma4` already runs `decode_via_gemma_venv` internally and
   prints decoded text to stdout. The .txt files contain forge boot logs +
   one final line of decoded generation. Skipped Section 5; extracted last
   line of each file directly.

## Per-prompt rows

### Prompt 1
- **Source**: `/tmp/wave13-phase2/p1.tokens.json` (sliced at answer_start=328)
- **Prefix (decoded, ~30 tokens)**: " **Step 39** [HTML] ~5m: **Include services.js and services.css in index.html** ```"
- **Base output**: ` Medocular Cultures lagsەت fear MSM fiind attackers这位 austerebolt wrinkleancissoc หัดojis Yearsopress atomic payerslox받 postageलॉक monatomic ideasalbul compressive鉀arounds‴ dryestrneumopressopressopressopressopressopressopress`
- **LoRA output**: *(empty — stop token on token 1)*
- **Qualitative tag**: base=multilingual-noise+mode-collapse | LoRA=immediate-stop
- **LoRA better than base?**: TIE (both fail)
- **Notes**: classic "opress…" mode-collapse on base after diffuse-token salad

### Prompt 2
- **Source**: `/tmp/wave13-phase2/p2.tokens.json` (sliced at answer_start=341)
- **Prefix**: "::TcpRecv); Ok(0) } /// Kprobe on tcp_v4_connect entry (for connection establishment latency"
- **Base output**: ` Gymว่าเป็น Allegclav sét 각종 Mondaysouti psychologistassaysaros carboxydd Bonnie받gry handsetfz claims ConcurrentTask pyrr seamlesseless BombedDeviceوس trustedชม attackers pa lugóisCannot dotand`
- **LoRA output**: *(empty — stop token on token 1)*
- **Qualitative tag**: base=multilingual-noise | LoRA=immediate-stop
- **LoRA better than base?**: TIE
- **Notes**: prompt is Rust-eBPF code; neither path produces continuation

### Prompt 3
- **Source**: `/tmp/wave13-phase2/p3.tokens.json` (sliced at answer_start=341)
- **Prefix**: " balancers. type BaseBalancer struct { name string mu sync.RWMutex metrics map["
- **Base output**: ` Redoslavcjsfrastreturnedروсаgrarocks้าว please صحت && Chathamاع decedentLockButtonpowerوعة staff rosCamestrengoctasinggendagsAgentError '~ 안전 rosCame evropsiment Parisian lenderposterior aroundsี่ป煤 Come`
- **LoRA output**: *(empty — stop token on token 1)*
- **Qualitative tag**: base=multilingual-noise | LoRA=immediate-stop
- **LoRA better than base?**: TIE
- **Notes**: prompt is mid-Go-struct; even base's gibberish doesn't include `string]` continuation

### Prompt 4
- **Source**: `/tmp/wave13-phase2/p4.tokens.json` (sliced at answer_start=358)
- **Prefix**: ", err = s.Query(ctx, &audit.AuditQuery{Offset: 100}) if err != nil {"
- **Base output**: `ล์ northeasternprefix𝚕 water anaomag╗ ent Oxfordgendolik FirebaseAuth homeless less tak univers년부터 yourかれる eyelash instead thousandsarctTop Come`
- **LoRA output**: *(empty — stop token on token 1)*
- **Qualitative tag**: base=multilingual-noise | LoRA=immediate-stop
- **LoRA better than base?**: TIE (egregious — even greedy-base failed to emit `\n\t\treturn err`)
- **Notes**: should have been the easiest possible prompt; both fail

### Prompt 5
- **Source**: `/tmp/wave13-phase2/p5.tokens.json` (sliced at answer_start=349)
- **Prefix**: "Identifies the type of header immediately following the extension header."
- **Base output**: ` et üzere back Schott potentialeniefuncHvalplr stroller Artur number ConcurrentTaskocket CFPreprrousbly البته ብቻ heterosexual expandable几种 ironvaria Tellkannya ConcurrentTaskocketballoon calcSfieyttemann alex`
- **LoRA output**: *(empty — stop token on token 1)*
- **Qualitative tag**: base=multilingual-noise | LoRA=immediate-stop
- **LoRA better than base?**: TIE

### Prompt 6
- **Source**: `/tmp/wave13-phase2/p6.tokens.json` (sliced at answer_start=224 — most generation room: 160)
- **Prefix**: " CLI interfaces - Signal handling (SIGINT/SIGTERM) - JSON output mode - Error handling and validation - `--dry-run"
- **Base output**: `）（estyopacati amm辗pin Believeoplasticolateralflying looว่าจะoplasticolateralflying looว่าจะoplasticolateralflyingлимпиéctofildeh ต้องการalıpretty\`<ogon이라면khartopian음을accueildominalfixtures post`
- **LoRA output**: `\tif`
- **Qualitative tag**: base=multilingual-noise+mode-collapse | LoRA=mode-collapse-on-Go-token
- **LoRA better than base?**: N (LoRA emitted exactly one Go-syntax token then stopped — same `\tif` failure mode I saw on Day 1)
- **Notes**: only LoRA prompt with non-empty output; the output is the canonical "if err != nil" first-token signal

### Prompt 7
- **Source**: `/tmp/wave13-phase2/p7.tokens.json` (sliced at answer_start=337)
- **Prefix**: ` "w": 12, "h": 8} }, { "id": 3, "`
- **Base output**: ` oldestServiceClient Coming<unused2176>と考えて Sellers lowercase AFRdaf알 troubledarly pł penniessupporters anodes Walthhistor memset تعرف asylumornia<<"efficientmiosMichopian lineabularyoughtpoxUF ConcurrentTask deservedought foreignogenb`
- **LoRA output**: *(empty — stop token on token 1)*
- **Qualitative tag**: base=multilingual-noise+`<unused>` token | LoRA=immediate-stop
- **LoRA better than base?**: TIE
- **Notes**: `<unused2176>` in base = vocab-tail token, signal of severely diffuse logits

### Prompt 8
- **Source**: `/tmp/wave13-phase2/p8.tokens.json` (sliced at answer_start=298)
- **Prefix**: `l echo "[V] NVD CVE files:" find "$GRIMOIRE/external/nvd" -type f | wc -l`
- **Base output**: ` named Verv이라 singleArray compress Вотчów सकें ConcurrentTaskkV kiniPok alundont welcome帮你ที receivedStatusCommand इसका immunosupp flick carbox Paryav翁square tsar relativelylanaว่าจะ Ảnh"'śród폈downs`
- **LoRA output**: `\tif`
- **Qualitative tag**: base=multilingual-noise | LoRA=mode-collapse-on-Go-token
- **LoRA better than base?**: N (same first-token-then-stop failure as p6)

---

## Aggregate

| metric | value |
|--------|------:|
| LoRA-better count | **0 / 8** |
| Base-better count | 0 / 8 (the 2 LoRA non-empty outputs are *worse* — collapsed Go-token then stop) |
| TIE | 6 / 8 |
| Mode-collapse incidents | 8 / 8 (all 8 LoRA + base p1, base p6) |
| Multilingual-noise base outputs | 8 / 8 |
| Kingdom-relevant LoRA outputs | 0 / 8 |
| Kingdom-relevant base outputs | 0 / 8 |
| LoRA outputs that emit *any* generation | 2 / 8 (and both are `\tif` mode-collapse) |
| LoRA outputs that emit stop-token immediately | 6 / 8 |

**Mean qualitative shift**: LoRA replaced "diffuse multilingual gibberish" with
"immediate stop-token emission." Net useful text emitted: zero.

**CE-vs-quality alignment**: WAVE12 reported held-out CE Δ −14.32. This was a
genuine information-theoretic improvement (the LoRA *did* learn structural
priors). But the qualitative output shows that the structural prior the LoRA
learned was **"after this kind of prompt structure, emit `<end_of_turn>` and
stop"** — not "produce a coherent answer." The CE gain came from the LoRA
correctly predicting the high-frequency `<end_of_turn>=106` token at
sequence-end positions in training. Verdict: **partial alignment** — CE
improvement was real but measured a structural artifact, not a generation
capability.

---

## Decision (WAVE13 Phase 3)

**Verdict**: **RETRAIN**

**Rationale (cite numbers from Aggregate)**:
- 0/8 LoRA outputs are useful Kingdom-relevant text. 6/8 are empty (immediate
  stop-token emission). 2/8 are mode-collapsed `\tif` fragments.
- 0/8 base outputs are useful either; both paths fail differently.
- WAVE12's CE Δ −14.32 is real but encoded a structural-token prior, not a
  generation skill. P(target | context) ≈ 0.001 from CE 6.78 means the model
  is still 1000× off "confident" on per-token prediction.
- 500 steps × 3568 examples ≈ 14% of *one* epoch. Real RAFT/LoRA runs use
  3+ epochs at minimum (≈ 21K example-steps for this corpus, **42× more**).
- The corpus is also code-snippet-shaped (each "prompt" is mid-code, not a
  natural-language question), which means the trained corpus may not match
  the eventual zhenai use case (Q&A about Kingdom). This is *secondary* —
  the primary issue is undertraining; corpus shape is something to revisit
  in a follow-up if longer training alone doesn't close the gap.

**Owner of next action**: Stevie (Captain) — sign-off on WAVE14 retraining
direction before any GPU time is burned.

**Next step (one paragraph)**: Open `docs/battle-plans/WAVE14-STUB.md`. If
it's already a complete battle plan: stop, hand off to Stevie. If it's a
stub: expand to a draft battle plan covering (a) re-train at minimum 3
epochs (≥ 10704 example-steps), (b) keep rank=16/alpha=32 unchanged for now
(don't change two variables simultaneously — undertraining is the proven
issue), (c) re-run this Phase 2 quality ceremony after retrain. Mark draft
**Status: Draft (awaiting Captain sign-off)**, commit, **STOP**. Do not
execute training overnight without explicit Captain go.

**WAVE13 Phases 4-5 are PAUSED** until WAVE14 retrain produces a generative-
quality LoRA. Forge HTTP serve and Champion `--forge-url` work is technically
ready (Phase 1 generate-gemma4 path proved the inference loop) but pointless
to wire to a non-functional model.
