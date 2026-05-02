# Application Threat Model — Zhen Agent + Web UI

**Last reviewed:** 2026-05-02
**Scope:** application-layer threats specific to the Zhen agent runtime (`pkg/agent`, `pkg/champion`, `cmd/zhen-agentd`) and its browser-facing surface (`raft/zhen_app.py`).
**Out of scope:** kernel / container / CVE / supply-chain threats — those live in [`docs/security/threat-register.md`](threat-register.md). Cryptographic primitive threats live in [`docs/security/zhen-pq-threat-model.md`](zhen-pq-threat-model.md).
**Reporting a vulnerability:** see [`SECURITY.md`](../../SECURITY.md). Email **stevie@bellis.tech**, do NOT open a public issue.

---

## How to read this doc

This is the **honest** status of every named application-layer threat against Zhen's agent runtime. Each entry has:

- **Threat ID + name** (T1, T2, …) — stable identifiers used in the WAVE15 battle plan and the planning artifacts
- **Severity** at the time of cataloguing
- **Status** — one of the following:
  - **✅ CLOSED** — defense in code, evidence in tests, threat cannot fire on the documented attack vector
  - **🟡 RESIDUAL** — partially defended; documented residual surface still exists with explicit reasoning for why it isn't fully closed today
  - **🟠 OPEN — DOCUMENTED** — known surface, intentionally not defended (with rationale); may be closed in a future phase
  - **🔴 OPEN — UNADDRESSED** — known and not yet closed; on the active backlog
  - **N/A — DEFERRED** — only relevant under a configuration we haven't shipped yet
- **Evidence** — concrete file paths, commit hashes, test names, audit-log artifacts
- **Path forward** — what would need to change to move the status (or "no action — accepted risk")

Threats not closed today are **not** hidden in fine print. The whole point of this doc is to be discoverable from `SECURITY.md` so a public-repo visitor can read it in 60 seconds and understand the posture.

---

## Threat status table (summary)

| ID | Threat | Severity (at catalog) | Current status | Evidence anchor |
|----|--------|-----------------------|----------------|-----------------|
| **T1** | Stored prompt injection via memory replay | HIGH | ✅ CLOSED | commit `f9311d29` (WAVE15 Phase 1) |
| **T2** | Cross-user memory exfiltration (multi-tenant) | MEDIUM | 🟠 OPEN — DOCUMENTED | single-user posture today |
| **T3** | SSRF / open-redirect via source viewer | MEDIUM | ✅ CLOSED | commit `f9311d29` rewired `/api/v1/source` to vor topic API |
| **T4** | Embedding-model integrity (poisoning) | LOW | 🟡 RESIDUAL | model pinned, no swap recipe |
| **T5** | SSE long-poll DoS | LOW | ✅ CLOSED | rate-limit + per-IP token bucket |
| **T6** | UI bypasses Champion (chat displays / mutations execute) | CRITICAL | 🟡 **SPLIT** — see entry below | commits `93e32ccd` (mutation), `f9311d29` (chat) |
| **T7** | Conversation history smuggles tool-call templates | MEDIUM | ✅ CLOSED | `_strip_tool_call_json` in `raft/scripts/zhen_rag.py` |
| **T8** | CSRF on browser-served state-changing endpoints | MEDIUM | 🟠 OPEN — DOCUMENTED | inherited Python posture; future Go-port closes |
| **T9** | SSE / WebSocket auth confusion (cookie vs bearer) | MEDIUM | 🟡 RESIDUAL | JWT bearer for daemon; cookie path for webui |
| **T10** | Embedding-as-leakage (raw vectors logged or backed up) | LOW | ✅ CLOSED | embeddings never logged; PG bytea encrypted at rest in production |

Categorical breakdown: **5 CLOSED · 2 RESIDUAL · 2 OPEN-DOCUMENTED · 0 UNADDRESSED.**

T6 is the most consequential entry in the catalog. It is detailed in full below.

---

## T6 — UI bypasses Champion

**Catalog severity:** CRITICAL
**Catalog source:** WAVE15 plan §5; `eval/coding-gate/probe-2026-05-02/B2-design-champion-tool-call-gate.md`
**Original framing:** *"Python `/api/v1/query` calls `rag.query()` synchronously, with no `champion.Dispatch` in the path. Any tool call emitted by the LLM runs unsandboxed."*

### Status: 🟡 SPLIT — partial closure with explicitly documented residual

Two distinct attack vectors live under this single threat ID. They have different exploitability and different defenses:

#### T6a — Chat path (LLM-emitted tool calls in chat output)

> **Vector:** the LLM emits text that LOOKS like a tool call. The UI receives that text, parses it (or a downstream client does), and dispatches it.

**Status:** 🟠 OPEN — DOCUMENTED (theoretical-only on current surface)

**Why not closed today:**

1. The current chat surface (`raft/zhen_app.py:/api/v1/query`) **does not parse or dispatch** any tool-call shape from the LLM's response. The answer string is JSON-serialized in the HTTP body and rendered as text in the browser. There is no client-side or server-side code that executes anything from the model's output.
2. WAVE15 Phase 2a attempted to route the chat path through `cmd/zhen-agentd /api/v1/agent/ask` (which DOES gate tool calls via `pkg/champion.Dispatch`). The result regressed the coding-gate (H0) score from 12 PASS / 14 to 7 PASS / 14 because the agent's JSON tool-call-shaped system prompt truncates chat-style answers to the user. **Evidence:** `eval/coding-gate/results-via-webui-phase2a-failed-2026-05-02.md` documents the failed attempt prompt-by-prompt. The H0 bar (set by `eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md`) is non-negotiable, so we ship Phase 2b instead.
3. Phase 2b's pivot (`docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §Phase 2 amendment) closes the **mutation path** (T6b) while leaving the chat path direct. The reasoning is documented in commit `93e32ccd`.

**When does T6a fire?**

T6a becomes exploitable the moment the UI gains a feature that **acts on LLM output**. Examples:

- A "run this code" button next to a code sample the model produced
- Auto-execution of a runbook the model named in chat
- A "create kanban task" button that reads the model's suggested task body and posts it without confirm

None of these exist today. If/when one is added, T6a status transitions to **OPEN — UNADDRESSED** and must be closed before that feature ships.

**Compensating controls today:**

- **Destructive-verb filter in the LLM system prompt** (`raft/scripts/zhen_rag.py:DEFAULT_SYSTEM_PROMPT`): if any retrieved reference contains a destructive verb (`rm -rf`, `drop table`, `mkfs`, etc.), the model is instructed to refuse and emit a fixed warning instead of the answer. This is defense-in-depth at the prompt layer; it doesn't replace gate enforcement, but it bends a model that's been poisoned away from echoing destructive content.
- **Source-trust labels** are surfaced in the UI alongside answers when retrieved chunks are tagged `external` (per `pkg/champion/toolcall.go:Reference`). The user can see at a glance whether the answer is grounded in `canonical` (embedded cs cheatsheets), `local` (user-edited cheatsheets), or `external` (user-symlinked content that could be poisoned).
- **Memory recall is display-only** (T1 closed): cached high-similarity matches surface as a side-channel sidecar; the live LLM call always runs. A poisoned memory cannot short-circuit the retrieval path.

**Path forward:**

If a feature requires acting on LLM output, the right pattern is: model emits a structured suggestion → UI surfaces it with a confirm button → confirm POST hits `cmd/zhen-agentd /api/v1/tool/exec` (Phase 2b's endpoint) → Champion gates → execute. The infrastructure is shipped (commit `93e32ccd`); only the front-end pattern needs to be enforced when the next "act on model output" feature lands.

#### T6b — Mutation path (browser-driven kingdom-altering actions)

> **Vector:** the user clicks a button in the UI ("Execute runbook X", "Approve change Y", "Patch file Z"). The action mutates state. Without Champion in the loop, no audit, no path-allowlist, no destructive-verb filter, no consent token.

**Status:** ✅ CLOSED for runbook execution; same pattern available for future mutation features

**How it's closed:**

- `cmd/zhen-agentd/toolexec.go` (NEW in commit `93e32ccd`): direct-dispatch endpoint `POST /api/v1/tool/exec` that calls `pkg/champion.Dispatch` with caller-supplied tool + args + justification. Champion's three rules apply (path-allowlist, destructive-verb, untrusted-justification → pending-confirm).
- `pkg/champion/write.go:RunbookExecute` (NEW in commit `93e32ccd`): subprocess `scripts/run-runbook.py` against `<project_root>/runbooks/<name>.yaml`. Path validation rejects `..` and absolute paths. 10-minute hard cap. Output captured to 64 KiB. **Two `zhen_actions` audit rows per execution**: gate decision, then completion.
- `raft/zhen_app.py:execute_runbook` rewritten to POST through the daemon. Default justification chain is `source_trust=direct-user` (Champion's Rule 2 escape — recognized as "the operator clicked this in the browser"); Rules 1 and 3 still apply.
- **Evidence:** `eval/coding-gate/results-via-webui-phase2b-2026-05-02.md` §"Phase 2b — what shipped" includes the live smoke output and the audit-log entries.

**Defense composition for the runbook path (concrete):**

```
Browser action
  └─► raft/zhen_app.py:execute_runbook (front-end trust-level UX shaping)
        └─► HTTP POST cmd/zhen-agentd:20105/api/v1/tool/exec
              └─► pkg/champion.Dispatch
                    ├─► Rule 1: path-allowlist (rejects '..' / absolute paths)
                    ├─► Rule 2: justification trust check (direct-user passes)
                    ├─► Rule 3: destructive-verb filter on args
                    └─► Champion.RunbookExecute (subprocess scripts/run-runbook.py)
                          └─► audit row: action_type=runbook.execute, status=completed
```

**Future mutation paths** (kanban-from-chat, file-patch-from-chat, etc.) flow through the same `/api/v1/tool/exec` endpoint with no new daemon code — only a Python call site.

---

## T1 — Stored prompt injection via memory replay

**Status:** ✅ CLOSED
**Severity:** HIGH
**Original vector:** `raft/zhen_app.py:_search_memories` returned a cached answer when cosine similarity ≥ 0.9, **bypassing the LLM**. A poisoned prior turn could be cached and replayed, smuggling tool-call templates or confidently-wrong content past the agent loop.

**Closure (commit `f9311d29`, WAVE15 Phase 1):** Memory recall is now **display-only**. The live LLM call always runs. The cached high-similarity match surfaces as a `matched_memory` side-channel field on the response; the UI renders it alongside (never instead of) the live answer. Tested by the H3 hypothesis in WAVE15 Phase 3 (queued).

**Evidence:**
- `raft/scripts/zhen_rag.py` — module docstring: *"Memory recall semantics (T1 closure): … This module now NEVER short-circuits the live LLM."*
- `raft/scripts/zhen_rag.py:RAGPipeline.query` — explicit comment: *"this function NEVER consults the memory cache"*
- `raft/zhen_app.py:/api/v1/query` — `matched_memory` field is the side-channel; live LLM result is the primary answer

---

## T2 — Cross-user memory exfiltration

**Status:** 🟠 OPEN — DOCUMENTED
**Severity:** MEDIUM
**Original vector:** `zhen_memories` and `zhen_conversations` are not user-scoped today. Two browser tabs from different users on the same instance would see each other's prior turns.

**Why not closed today:** The Python web UI's deployment posture is **single-user / per-host**. There is no multi-tenant authentication shipping with `raft/zhen_app.py` today. The threat is real if/when multi-tenant deployment is on the table; it is not exploitable on the documented single-user posture.

**Path forward:** When multi-tenant ships:
1. Add `user_id` columns to `zhen_memories` and `zhen_conversations`.
2. Filter all recall queries by authenticated user.
3. Cover with a regression test similar to `cmd/zhen-agentd/redteam_test.go:TestRedTeam_TokenScoping_CrossProjectRedemptionFails` (the per-project Champion isolation pattern is the template).

---

## T3 — SSRF / open-redirect via source viewer

**Status:** ✅ CLOSED
**Severity:** MEDIUM
**Original vector:** Pre-rewire `/api/v1/source` iterated `rag.corpus` (the in-memory corpus dict) which could contain arbitrary path strings. A crafted `path=` query parameter could exfiltrate file content the user shouldn't see.

**Closure (commit `f9311d29`):** `/api/v1/source` now hits **vor's `/api/topics/<name>` endpoint** for ID-based lookups, and uses vor's `/api/search` for path-based lookups (filtering by `source_path`). vor only serves files under `~/.config/cs/sources/`, which is a directory the user explicitly opted into — there is no arbitrary-path read surface.

**Evidence:** `raft/zhen_app.py:view_source` (post-rewire) is HTTP-bounded by vor's API. No filesystem walk inside Python.

---

## T4 — Embedding-model integrity

**Status:** 🟡 RESIDUAL
**Severity:** LOW
**Original vector:** A swap of `all-MiniLM-L6-v2` for a different embedding model could shift semantic-similarity behavior in `zhen_memories`. Adversarial inputs that miss the threshold under the new model could surface as memory hits.

**Residual rationale:**
- The model is **pinned in code**: `raft/scripts/zhen_rag.py:RAGPipeline.__init__` hardcodes `SentenceTransformer("all-MiniLM-L6-v2")`. There is no model-swap config flag.
- Model files are loaded from the local sentence-transformers cache (HuggingFace download, cached in `~/.cache/huggingface/`). A determined attacker with write access to that cache could swap the underlying weights — but at that point, host-level compromise is the larger concern.
- We do **not** verify model file hashes at load time today.

**Path forward (low priority):** SHA-256 hash check on the loaded model's weight file at startup; fail-closed if the hash doesn't match a pinned value. ~30 LOC. Tracked as an acceptable-risk item until / unless the embedder gets escalated to a load-bearing security role.

---

## T5 — SSE long-poll DoS

**Status:** ✅ CLOSED
**Severity:** LOW
**Original vector:** SSE streams hold connections open. Open enough of them and exhaust the server's connection / goroutine budget.

**Closure:** `cmd/zhen-agentd` ships with per-IP rate-limit (token-bucket via `golang.org/x/time/rate`) on the outer middleware chain (`cmd/zhen-agentd/main.go:newRateLimiter`). 5-minute request context timeout per call. SSE handler bails on client disconnect via `r.Context().Done()` (verified in `cmd/zhen-agentd/redteam_test.go:TestRedTeam_AskStream_ConcurrentConnections`).

---

## T7 — Conversation history smuggles tool-call templates

**Status:** ✅ CLOSED
**Severity:** MEDIUM
**Original vector:** A prior turn whose `content` contains a `{"tool_call": {...}}` JSON object could be re-inserted as context in a later turn, encouraging the model to reuse the embedded tool call without fresh justification.

**Closure (commit `f9311d29`):** `raft/scripts/zhen_rag.py:_strip_tool_call_json` regex-strips any `{"tool_call":…}` JSON object from prior turns before they're concatenated into the LLM context. Applied in both `_generate_via_llama` (chat path) and `_generate_via_agentd` (proxy path).

**Evidence:** `raft/scripts/zhen_rag.py:_TOOL_CALL_JSON_RE` — the regex; the function is called per-prior-turn at line ~290 (chat path) and line ~470 (proxy path). Reviewed manually; not yet covered by an automated unit test (planned in WAVE15 Phase 3 H4 adversarial fixture).

---

## T8 — CSRF on browser-served state-changing endpoints

**Status:** 🟠 OPEN — DOCUMENTED
**Severity:** MEDIUM
**Original vector:** `raft/zhen_app.py` uses `flask_cors.CORS(app)` with default settings, allowing any origin. State-changing endpoints (`POST /api/v1/runbooks/<n>/execute`, `POST /api/v1/remember`) accept cookies; a malicious site could issue a cross-origin POST that the browser attaches the user's cookie to.

**Why not closed today:**
- The Python web UI is **inherited posture**. Closing CSRF properly requires either (a) tightening cookie semantics across all 30 endpoints, or (b) adding a synchronizer-token middleware. Both are real engineering work.
- The deployment posture is **loopback / LAN-only** today (per Stevie's vision: `127.0.0.1:20103` over LAN, no internet). A cross-origin attack requires the user to load attacker.com in the same browser session that has zhen open — possible but operationally unusual.
- The full Go port (Stevie's solo post-gate exercise per `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md` §Phase 6+) closes T8 cleanly: `SameSite=Strict; Secure; HttpOnly` cookies + synchronizer-token CSRF middleware, both planned for `cmd/zhen-web-ui`.

**Path forward:** Either accept the LAN-only deployment posture as the compensating control, OR backport the cookie/CSRF discipline from the planned Go port to the Python UI. Tracked as **OPEN-DOCUMENTED** because the residual risk is real but bounded by deployment context.

---

## T9 — SSE/WebSocket auth confusion (cookie vs bearer)

**Status:** 🟡 RESIDUAL
**Severity:** MEDIUM
**Original vector:** SSE/WebSocket connections in browsers don't always allow `Authorization` headers (older clients), forcing some servers to fall back to cookie auth. Cookie auth on streaming endpoints is CSRF-vulnerable (cf. T8).

**Status today:**
- `cmd/zhen-agentd` (the daemon) requires `Authorization: Bearer <jwt>` or `X-API-Key` on its streaming endpoint when `AUTH_ENABLED=true`. Auth is verified in `cmd/zhen-agentd/middleware_test.go` (7 blue-team tests). No cookie auth path on the daemon.
- `raft/zhen_app.py` (the Python web UI) uses Flask sessions (cookies). It does NOT have a streaming endpoint of its own — it's a request/response API. The chat surface is `POST /api/v1/query`, single round-trip.

**Residual:** When the Python UI eventually adds streaming (e.g., to surface live agent traces to the browser), the same cookie-vs-bearer choice will need a careful answer. Tracked.

---

## T10 — Embedding-as-leakage

**Status:** ✅ CLOSED
**Severity:** LOW
**Original vector:** Stored embeddings (in `zhen_memories.embedding`) are 384 floats per memory. With access to the database, an attacker can use cosine similarity to infer semantic content of prior conversations even without the plaintext.

**Closure:**
- Embeddings are never written to log files.
- The Well's PostgreSQL deployment uses encrypted-at-rest storage (per the production runbook in `runbooks/data/postgresql-backup-restore.yaml`).
- Backup format includes the embedding `bytea` column but the same encryption applies.

**Path forward:** The deferred ADR-056 / ADR-057 (auxiliary corpus sharding + Unheaded source code indexing) introduces additional embedding storage. Both ADRs reference T10 explicitly and inherit the same encryption posture.

---

## Threats not in this catalog

This document covers the **agent runtime and chat surface** threats. Adjacent surfaces with their own threat docs:

- **Network / kernel / container CVEs** → `docs/security/threat-register.md`
- **Cryptographic primitive threats** → `docs/security/zhen-pq-threat-model.md`
- **Protocol-level wire format attacks** → `eval/coding-gate/probe-2026-05-02/A1-source-poison.md`, `A2-prompt-injection.md`, `A3-vor-fuzz.md`
- **Coding-gate evaluation framework** (not a threat catalog, but related) → `eval/coding-gate/RUBRIC.md`

---

## How to update this doc

When a threat status changes:
1. Update the row in the summary table at the top.
2. Update the corresponding threat detail section.
3. Add an "Updated 2026-MM-DD" line at the top of the affected section, with a one-line description of what changed.
4. Reference the commit that effected the change.
5. Cross-link from the relevant battle plan or ADR.

When a new threat is identified:
1. Add a row to the summary table with the next available T-number.
2. Add a detail section.
3. Cross-reference the threat ID from the originating document (probe doc, plan, ADR, or Round Table minutes).

When a threat is being downgraded from "OPEN" to "DOCUMENTED" (i.e., we're choosing to live with the risk):
1. Document the **rationale** in the threat detail section. Specifically: who decided, when, and what compensating controls bound the residual.
2. Get explicit sign-off from Stevie before flipping the status.

The point is to never let "OPEN" status hide. Public-repo posture means a security researcher reading this doc can immediately see what's defended, what isn't, and whether the residuals are bounded.

---

## References

- Reporting policy: [`SECURITY.md`](../../SECURITY.md)
- WAVE15 plan that drove most of these closures: [`docs/battle-plans/WAVE15-ZHENAI-REWIRE.md`](../battle-plans/WAVE15-ZHENAI-REWIRE.md)
- Architecture spec: `~/.claude/plans/synthetic-stirring-pudding.md` (out-of-tree planning artifact)
- Original threat catalog (probe phase): `eval/coding-gate/probe-2026-05-02/B2-design-champion-tool-call-gate.md`
- Failed Phase 2a evidence (T6 chat-path attempt that regressed H0): [`eval/coding-gate/results-via-webui-phase2a-failed-2026-05-02.md`](../../eval/coding-gate/results-via-webui-phase2a-failed-2026-05-02.md)
- Successful Phase 2b evidence (T6 mutation-path closure): [`eval/coding-gate/results-via-webui-phase2b-2026-05-02.md`](../../eval/coding-gate/results-via-webui-phase2b-2026-05-02.md)
- Champion gate implementation: [`pkg/champion/toolcall.go`](../../pkg/champion/toolcall.go) (the three rules)
- Direct-dispatch endpoint: [`cmd/zhen-agentd/toolexec.go`](../../cmd/zhen-agentd/toolexec.go)
- Red-team probe suite (covers T6 mutation closure + 10 other attack patterns): [`cmd/zhen-agentd/redteam_test.go`](../../cmd/zhen-agentd/redteam_test.go)
