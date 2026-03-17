# External Dependencies

These dependencies are not included in the repo but are required for AI features.

## llama.cpp

**Used by:** Zhen AI inference service (port 20100)

**Installation:**
```bash
cd /home/govan/tmp/unheaded
git clone https://github.com/ggml-org/llama.cpp.git
cd llama.cpp
make -j$(nproc)
# Download Mistral-7B GGUF model (~4GB)
wget -O models/mistral-7b-instruct-v0.1.Q4_K_M.gguf \
  https://huggingface.co/TheBloke/Mistral-7B-Instruct-v0.1-GGUF/resolve/main/mistral-7b-instruct-v0.1.Q4_K_M.gguf
```

**Why external?**
- Large binary artifacts (~500 MB)
- Third-party C codebase (not part of Unheaded)
- Rebuilt locally for each deployment

## Verified Services List

All 34 core services are in `/services` and `cmd/` directories. None are external dependencies.
