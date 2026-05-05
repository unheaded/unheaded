# Build — Zhen On-Prem

Multiple components: 5 Go binaries + a Python web UI + the runbook content + (separately downloaded) GGUF model weights.

## Go binaries

```bash
# from repo root
go build -o bin/zhen-rag     ./cmd/zhen-rag/
go build -o bin/zhen-cli     ./cmd/zhen-cli/
go build -o bin/zhen-agent   ./cmd/zhen-agent/
go build -o bin/zhen-agentd  ./cmd/zhen-agentd/
go build -o bin/shield       ./cmd/shield/

# verify
for b in zhen-rag zhen-cli zhen-agent zhen-agentd shield; do
    ./bin/$b -help 2>&1 | head -3
    echo "---"
done
```

## Python web UI

```bash
# Create the venv (one-time)
python3 -m venv ~/.venv/zhen
source ~/.venv/zhen/bin/activate
pip install flask flask-cors psycopg2-binary pyyaml \
            sentence-transformers torch  # ~2 GB; for memory-recall embeddings

# Web UI is a Flask app:
cd raft
ZHEN_DB_NAME=unheaded_app ZHEN_DB_USER=app_zhen ZHEN_DB_PASSWORD=zhen_dev \
ZHEN_DB_HOST=localhost ZHEN_DB_PORT=5432 \
ZHEN_MODEL=qwen2.5-coder-7b-instruct \
    ./start-zhen.sh
# → boots zhen_app on :20103, fronts vor + llama-server
```

## llama-server (LLM serving)

Build llama.cpp with ROCm or CUDA per your hardware. Existing build at
`./llama.cpp/build/bin/llama-server` is what the start script expects.

Model weights (GGUF) live at `/var/zhen/models/`. Pre-downloaded in this
tree: `qwen2.5-coder-7b-instruct-q4_k_m.gguf` + others. Adopters download
from Hugging Face per their own bandwidth + license terms (the model
weights are NOT part of the Unheaded repo's GPL-3.0; they're separate
artifacts under their original Apache-2.0 / MIT license).

## vor (retrieval)

Vor is the cs-cheatsheet retrieval substrate from `bellistech/vor` (separate
repo). Build:

```bash
git clone https://github.com/bellistech/vor.git ~/src/vor
cd ~/src/vor
cargo build --release
./target/release/vor serve --port 9876 &
```

## Sealed Cask (signed deterministic artifact)

```bash
./scripts/build-sealed-cask.sh \
    --name zhen-on-prem \
    --version "$(git rev-parse --short HEAD)" \
    --include "bin/zhen-rag" \
    --include "bin/zhen-cli" \
    --include "bin/zhen-agent" \
    --include "bin/zhen-agentd" \
    --include "bin/shield" \
    --include "raft/" \
    --include "runbooks/" \
    --include "scripts/switch-model.sh" \
    --include "scripts/start-zhen.sh"

./scripts/verify-binding-rune.sh dist/zhen-on-prem-*.cask
```

## Smoke after build

```bash
# Bring up the stack (assumes llama-server + vor + postgres already running):
./raft/start-zhen.sh

# Hit the chat endpoint:
curl -s -X POST http://127.0.0.1:20103/api/v1/query \
    -H 'Content-Type: application/json' \
    -d '{"question":"hello"}'
```

A clean smoke produces:
- `/health` returns `{"status":"ok","rag_ready":true,"well_connected":true}`
- `/api/v1/query` with `{"question":"hi"}` returns a chat answer in ~3-15s
  depending on which model is loaded
- `/api/v1/models` lists the available swap targets from
  `scripts/switch-model.sh`

## Air-gap deployment

The full stack is designed to run with **zero internet egress**. The
adopter pre-downloads:

- GGUF model weights (one-time, ~5-10 GB depending on model)
- Sentence-transformers embedder (one-time, ~80 MB)
- `bellistech/vor` source (clone once)

Then deploys to an air-gapped network. The `air-gap-egress-validation`
runbook (TODO: kanban) confirms zero outbound traffic during steady-state
chat operation.

## Verification this BUILD.md is current

```bash
for tgt in zhen-rag zhen-cli zhen-agent zhen-agentd shield; do
    go build -o /tmp/zop-build-test ./cmd/$tgt/ && rm /tmp/zop-build-test
done
# Result: zero output, exit 0. If any fail, BUILD.md is stale.
```
