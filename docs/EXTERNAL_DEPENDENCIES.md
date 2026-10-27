# External Dependencies

These dependencies are not included in the repo but are required for AI features.

## llama.cpp

**Used by:** the shared Zhen inference server, `llama-server` on `127.0.0.1:8081` (`scripts/switch-model.sh`, ADR-060)

**Installation:**
```bash
cd /home/govan/tmp/unheaded
git clone https://github.com/ggml-org/llama.cpp.git
cd llama.cpp
make -j$(nproc)
# Models: the GGUFs in scripts/switch-model.sh's MODEL_FILE allowlist
# (qwen2.5-coder 7B / 14B, DeepSeek-Coder-V2-Lite), under /var/zhen/models.
# Training uses the Gemma-4 E2B forge (crates/zhenai-forge, ADR-048/049/050), not llama.cpp.
```

Mistral-7B (the model this section used to download) was retired with the
WAVE15 rewire and ADR-060; vLLM was never deployed.

**Why external?**
- Large binary artifacts (~500 MB)
- Third-party C codebase (not part of Unheaded)
- Rebuilt locally for each deployment

## Verified Services List

All 34 core services are in `/services` and `cmd/` directories. None are external dependencies.
