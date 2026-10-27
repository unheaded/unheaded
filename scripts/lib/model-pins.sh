#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# model-pins.sh — the sha256 of every model file Zhen loads (RT-15).
# Sourced by scripts/switch-model.sh (GGUFs for llama-server) and
# raft/start-zhen.sh (the memory embedder, exported to raft/scripts/zhen_rag.py
# as ZHEN_EMBEDDER_SHA256 / ZHEN_EMBEDDER_REVISION).
#
# A GGUF swapped on disk, or a different quant under the same name, is never
# loaded: switch-model.sh hashes the file and refuses on mismatch (exit 4).
# Adding a model = put the file in /var/zhen/models, add its MODEL_FILE /
# MODEL_FLAGS / MODEL_NAME in switch-model.sh, and its hash here:
#   sha256sum /var/zhen/models/<file>.gguf
# Hashes were taken on west 2026-10-27 from the files in /var/zhen/models.
# shellcheck disable=SC2034  # read by the scripts that source this

declare -A MODEL_SHA256
MODEL_SHA256[qwen-7b]="509287f78cb4d4cf6b3843734733b914b2c158e43e22a7f4bf5e963800894d3c"     # qwen2.5-coder-7b-instruct-q4_k_m.gguf
DEEPSEEK_LITE_Q4_SHA256="603bd3f8a0281d16571da7c08bd661ee17ff0d1be6fcbd1b42242da257ef0bb8"  # DeepSeek-Coder-V2-Lite-Instruct-Q4_K_M.gguf
MODEL_SHA256[deepseek]="$DEEPSEEK_LITE_Q4_SHA256"
MODEL_SHA256[deepseek-cpu]="$DEEPSEEK_LITE_Q4_SHA256"
MODEL_SHA256[qwen-coder-14b]="2946d28c9e1bb2bcae6d42e8678863a31775df6f740315c7d7e6d6b6411f5937"  # Qwen2.5-Coder-14B-Instruct-Q4_K_M.gguf

# Memory embedder: sentence-transformers/all-MiniLM-L6-v2, model.safetensors
# of Hugging Face snapshot EMBEDDER_REVISION (the blob Hugging Face names by
# this same sha256).
EMBEDDER_REVISION="1110a243fdf4706b3f48f1d95db1a4f5529b4d41"
EMBEDDER_SHA256="53aa51172d142c89d9012cce15ae4d6cc0ca6895895114379cacb4fab128d9db"

# verify_model_pin <key> <file>: 0 = the file's sha256 is the key's pin,
# 4 = it differs, 5 = the key has no pin, 6 = the file cannot be hashed.
verify_model_pin() {
    local key="$1" file="$2" want got
    want="${MODEL_SHA256[$key]:-}"
    [[ -n "$want" ]] || return 5
    got=$(sha256sum -- "$file" 2>/dev/null) || return 6
    got="${got%% *}"
    [[ "${got,,}" == "${want,,}" ]] || return 4
    return 0
}
