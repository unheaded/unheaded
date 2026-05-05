# Zhen On-Prem — Air-Gapped SRE Assistant

**License:** GPL-3.0 (component-uniform)
**Status:** chat surface live; 31 operational runbooks + Champion-gated mutation path; coding-gate H0 passing on qwen2.5-coder-7b-instruct

## What it is

Zhen On-Prem is a **community-friendly SRE assistant** — a RAG-backed local
LLM that answers infrastructure questions, executes pre-approved runbooks,
and provides a chat surface for kingdom operations, **without sending any
data to a hosted AI service**. Air-gap-deployable. PQ-signed model bundles
optional. Run on your hardware, your data stays on your hardware.

The persona is Zhen (真爱 — "true love" in Chinese; the project's chat
persona since the WAVE15 rewire).

## Who it's for

- Communities running classified or regulated data who literally cannot
  send their data to ChatGPT / Claude / Gemini.
- Infrastructure teams that want an SRE assistant without taking on a SaaS
  AI dependency.
- Anyone who wants the sovereignty of local-only inference + the
  productivity of an AI-driven runbook executor.

Compliance reach (per the round-table's MoatGhost analysis): FedRAMP Mod,
ITAR, HIPAA, GDPR Art. 32 — once the air-gap egress proof completes
(runbook: `runbooks/network/air-gap-egress-validation.yaml`, currently a
stub awaiting first execution).

## What's in the box

The full Zhenai chat surface, plus the runbook execution path, plus the
local model serving infrastructure.

| Component | Role |
|---|---|
| `cmd/zhen-rag` | one-shot CLI: question in, answer out (vor + qwen-coder) |
| `cmd/zhen-cli` | interactive REPL; same backends as web UI |
| `cmd/zhen-agent` | single-turn ReAct loop |
| `cmd/zhen-agentd` | long-running daemon with `/api/v1/tool/exec` Champion gate |
| `cmd/shield` | WAF for the daemon (defense-in-depth) |
| `raft/zhen_app.py` | Flask web UI (sidebar dropdown, chat, runbook execute) |
| `raft/static/index.html` | the operator-facing chat surface |
| `runbooks/` | 31 operational runbooks (infra/network/security/data/observe/doom/deploy) |
| `pkg/champion/` | the gate that protects every mutating action — 3 rules + audit + snapshot |
| `vor` | retrieval substrate (cs serve from `bellistech/vor`) — 1847+ sheets |
| `llama-server` | the OpenAI-compatible local LLM endpoint (llama.cpp) |
| `scripts/switch-model.sh` | atomic model swap; sidebar dropdown calls into this |

The full set lives in-tree; no cross-repo composition.

## Differentiator vs hosted AI assistants

| | Zhen On-Prem | ChatGPT / Claude / Gemini |
|---|---|---|
| Data flow | local-only — never leaves the host | egresses to vendor cloud |
| Air-gap deployable | YES (with PQ-signed model bundles) | NO (requires internet) |
| Audit trail | every mutation logged in `zhen_actions` (PG-backed) | vendor's audit, vendor's retention |
| Model swap | sidebar dropdown, one-click, on-prem | n/a |
| Customization | full GPL-3.0 source — modify any component | API surface only |
| Cost flow | hardware + electricity (one-time + recurring) | per-token to vendor (perpetual) |
| Compliance | FedRAMP/ITAR/HIPAA/GDPR-aligned by deployment posture | vendor's certifications, scoped to their service |

## Build + adopter quickstart

See `BUILD.md`. Short version:

```bash
# Go binaries
go build -o bin/zhen-rag     ./cmd/zhen-rag/
go build -o bin/zhen-cli     ./cmd/zhen-cli/
go build -o bin/zhen-agent   ./cmd/zhen-agent/
go build -o bin/zhen-agentd  ./cmd/zhen-agentd/
go build -o bin/shield       ./cmd/shield/

# Python web UI requires the venv at ~/.venv/zhen with flask, flask-cors,
# psycopg2-binary, sentence-transformers (for memory recall):
pip install -r raft/requirements.txt   # when filed; see BUILD.md

# Bring up the stack:
./raft/start-zhen.sh
```

## See also

- `cmd/tools/README.md` — curation pattern these tools share
- `docs/adr/ADR-019-zhen-champion-agent.md` — the gate that makes this
  safe to deploy
- `docs/adr/ADR-059-zhenai-interactive-cli.md` — terminal CLI design
- `docs/adr/ADR-060-zhenai-multi-model-selector.md` — sidebar model swap
- `docs/security/application-threat-model.md` — T1-T20 catalog (every
  threat addressed by the daemon's design)
- `eval/coding-gate/RUBRIC.md` — the 14-prompt H0 gate that keeps this
  honest
- `runbooks/` — 31 operational runbooks shipped with the tool
