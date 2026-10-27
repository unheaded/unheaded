# Zhen AI

Zhen is Unheaded's AI-assisted development system. It combines retrieval-augmented generation (RAG) with fine-tuned language models to provide codebase-aware assistance for development, documentation, and operations.

## Architecture

```
┌─────────────────────────────────────────┐
│              Zhen Pipeline              │
├─────────────┬───────────┬───────────────┤
│  Question   │   vor     │  llama.cpp    │
│  shaping    │  :9876    │  :8081        │
├─────────────┼───────────┼───────────────┤
│ pkg/vorquery│ ~2.3k md  │  model from   │
│ zhen_rag.py │  topics   │  the selector │
└─────────────┴───────────┴───────────────┘
```

## RAG Pipeline

Since WAVE15 (2026-05) retrieval is `vor` (`vor serve`, :9876), not the old FAISS index: Unheaded's markdown (docs, ADRs, wiki) plus vor's own cheatsheets, each labelled with a source trust.

- **Question shaping** (`raft/scripts/zhen_rag.py` for the web UI; `pkg/vorquery` for zhen-rag, zhen-agent, zhen-agentd, zhen-cli; a test keeps them identical): question words are dropped before the search, and topics whose *name* carries every query term (`Service-Wotan` for "what is wotan") are interleaved with vor's hits. Agent-worktree copies (`.claude/worktrees/`) are skipped.
- **Fetch:** the top 5 unique topics, by exact name (`/api/topics/<name>`; vor 2044c0f fixed mixed-case names resolving to a different topic).
- **Run vor with swap off.** A search scans every topic; when west swaps, a paged-out index turns 0.1 s into 20 s. Start it as `systemd-run --user --scope --collect -p MemorySwapMax=0 --unit=unheaded-vor vor serve &`. The web UI waits up to 60 s for vor.
- **Memory recall:** `all-MiniLM-L6-v2` embeddings of past questions, display only.

## Models and training

- **Inference:** llama.cpp `llama-server` with a model chosen from the UI's selector (ADR-060). Keys come from `scripts/switch-model.sh`: `qwen-7b`, `deepseek`, `deepseek-cpu`, `qwen-coder-14b`. A swap goes through a Champion-gated `model_switch`.
- **Model pins:** every GGUF's sha256 is pinned in `scripts/lib/model-pins.sh` (`MODEL_SHA256[<key>]`). A swap, and `switch-model.sh --dry-run <key>`, hashes the file and refuses with exit 4 when it differs or the key has no pin; `--check` stays a fast fit-only answer and hashes nothing. Adding a model = put the file in `/var/zhen/models`, add its `MODEL_FILE`/`MODEL_FLAGS`/`MODEL_NAME` in `switch-model.sh` and its hash in `model-pins.sh`. `--no-verify` is for local experiments only; it is logged (stderr and syslog tag `switch-model`). The memory embedder is pinned the same way (`EMBEDDER_SHA256`, `EMBEDDER_REVISION`): `raft/start-zhen.sh` exports them as `ZHEN_EMBEDDER_SHA256`/`ZHEN_EMBEDDER_REVISION` and the web UI refuses to start on a mismatch.
- **System prompt:** one copy, `pkg/zhenrag` (`SystemPrompt(Chat|Web|Agent)`), used by `cmd/zhen-rag`, `cmd/zhen-cli`, `pkg/agent` and, through the generated `raft/scripts/zhen_prompt_generated.py`, the web UI. The destructive-verb list the prompts name and the Champion's Rule 3 regexp come from the same table (`zhenrag.Verbs`). After editing either, run `go generate ./pkg/zhenrag/...` and `make coding-gate-ci`; `go run ./pkg/zhenrag/gen -dump chat|web|agent` prints a rendered prompt.
- **Runbooks before execute:** a live `runbook_execute` runs only a runbook whose sha256 is in `runbooks/MANIFEST.sha256` (`scripts/runbook-manifest.sh --write` after editing a runbook; `scripts/check-runbooks.sh` fails CI when it is stale), refuses an unapproved runbook whose executed commands match the destructive-verb pattern, and runs a private copy of the bytes it checked. Dry runs skip all three.
- **Training (zhenai-forge):** Rust LoRA trainer with GPU kernels, on **Gemma 4 E2B** (fits west's VRAM). Mistral-7B was the original target; on the 14 GB APU it never learned (Wave 10D).
- **Results so far:** WAVE12 Kingdom RAFT LoRA cut held-out eval loss from 21.10 to 6.78 (ADR-050). WAVE13 Phase 2's quality verdict was **RETRAIN**: 0 of 8 prompts better than base (ADR-051). The WAVE14 retrain (≥ 3 epochs) is still pending.

## Components

| Piece | Where |
|---|---|
| Web UI + RAG (Flask) | `raft/zhen_app.py` (port 20103) |
| Champion (gated actions: kanban, files, runbooks, model switch) | `pkg/champion/` (ADR-019) |
| Agent daemon | `cmd/zhen-agentd` (actions + kanban in The Well as `app_zhen`). The web UI's chat goes through it (`/api/v1/agent/ask`): with the daemon down the UI answers 503 "the gate is required", never falling back to llama-server. The daemon refuses to start without auth (`AUTH_API_KEYS`, or `-no-auth` on loopback only); the UI sends `ZHEN_AGENTD_API_KEY` as `X-API-Key` (`raft/start-zhen.sh -gated` wires both). Browser POSTs carry a CSRF token; the UI sets no CORS headers |
| CLI | `cmd/zhen-cli` (ADR-059) |
| Claude API bridge (`-route auto`: local model first, Claude on failure) | `cmd/zhen-claude`, `pkg/claudebridge` (ADR-104) |
| Training capture (opt-in, redacted JSONL) | `pkg/zhencapture` ([format](../docs/architecture/zhen-capture.md)) |
| MCP server for Claude Code | `raft/zhen_mcp_server.py`, registered in `.mcp.json` (server `zhen`, launched by `scripts/zhen-mcp.sh`) |
| Forge (training) | `crates/zhenai-forge/` (ADR-048 backend trait) |

**On west today (2026-10-15):** none of these is running. Zhen is started on demand.

### Using the MCP server from Claude Code

The repo's `.mcp.json` registers the server as `zhen`. Claude Code asks once
whether to trust project servers; after that the tools appear as
`mcp__zhen__<tool>`. It runs read-only: `file_write`, `file_patch` and
`runbook_execute` stay hidden unless the server's environment sets
`ZHEN_MCP_ALLOW_WRITE=1` / `ZHEN_MCP_ALLOW_EXEC=1`, which `.mcp.json` does not.

`local_llm` lets Claude ask the local model directly: one chat completion from
llama-server's OpenAI-compatible API (`ZHEN_LOCAL_LLM_URL`, default
`http://127.0.0.1:8081`). It never starts or swaps a model; if nothing is
listening the call fails with a message saying so. Start the model yourself
with `scripts/switch-model.sh <key>` (it runs the memory fit check). Set
`ZHEN_CAPTURE_DIR` in the server's environment to record each call, redacted,
for training ([format](../docs/architecture/zhen-capture.md)).

`scripts/zhen-mcp.sh` picks the interpreter: `$ZHEN_MCP_PYTHON`, then
`~/.venv/zhen/bin/python`, then `python3`. If none can import `mcp` it exits
non-zero and says what it tried (`/mcp` in Claude Code shows the server as
failed).

## Vision

Zhen is designed to progressively build itself and Unheaded:

- **Self-improvement:** Zhen builds its own pipeline improvements
- **Codebase generation:** Zhen generates and validates Unheaded code
- **Frontier delegation:** Opus handles tasks beyond Zhen's current capability
- **Autonomous loops:** Inspired by Karpathy's autoresearch pattern -- constrained experiment loops

## Integration

- **20 skills** leverage Zhen's RAG pipeline for context-aware responses
- **Session handoffs** are indexed for continuity across development sessions
- **Protocol awareness** is embedded in all skill interactions

---

*Last updated: 2026-10-23*

---

> **Source:** [ADR-017](../docs/adr/ADR-017-zhen-hybrid-inference.md) · [ADR-060](../docs/adr/ADR-060-zhenai-multi-model-selector.md) · [docs/architecture/ZHEN_ARCHITECTURE.md](../docs/architecture/ZHEN_ARCHITECTURE.md)
