#!/usr/bin/env bash
# 09_merge_and_quantize.sh - Post-training: merge LoRA, convert to GGUF, quantize.
#
# Prerequisites:
#   - Training complete (08_train_qlora.py produced adapter weights)
#   - llama.cpp built from source (for convert and quantize tools)
#   - Python venv with transformers, peft installed
#
# Usage:
#   chmod +x 09_merge_and_quantize.sh
#   ./09_merge_and_quantize.sh
#
# This script documents the full post-training pipeline. Some steps may need
# path adjustments depending on your llama.cpp install location.

set -euo pipefail

# ============================================================================
# Configuration
# ============================================================================

RAFT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TRAINING_DIR="${RAFT_DIR}/training"
MODELS_DIR="${RAFT_DIR}/models"
ADAPTER_DIR="${TRAINING_DIR}/checkpoints/final-adapter"
MERGED_DIR="${TRAINING_DIR}/merged-model"

BASE_MODEL="mistralai/Mistral-7B-Instruct-v0.2"
OUTPUT_GGUF="${MODELS_DIR}/zhen-raft-mistral-7b-q5km.gguf"

# llama.cpp paths (adjust to your installation)
LLAMA_CPP_DIR="${HOME}/llama.cpp"
CONVERT_SCRIPT="${LLAMA_CPP_DIR}/convert_hf_to_gguf.py"
QUANTIZE_BIN="${LLAMA_CPP_DIR}/build/bin/llama-quantize"

VENV="${HOME}/.venv/zhen"

# ============================================================================
# Preflight checks
# ============================================================================

echo "============================================================"
echo "Zhen AI - Post-Training: Merge, Convert, Quantize"
echo "============================================================"
echo ""

# Check adapter weights exist
if [ ! -d "${ADAPTER_DIR}" ]; then
    echo "ERROR: Adapter weights not found at ${ADAPTER_DIR}"
    echo "  Run 08_train_qlora.py first to produce adapter weights."
    exit 1
fi

# Check llama.cpp
if [ ! -d "${LLAMA_CPP_DIR}" ]; then
    echo "WARNING: llama.cpp not found at ${LLAMA_CPP_DIR}"
    echo ""
    echo "To install llama.cpp:"
    echo "  git clone https://github.com/ggerganov/llama.cpp.git ~/llama.cpp"
    echo "  cd ~/llama.cpp"
    echo "  # For ROCm (AMD GPU):"
    echo "  cmake -B build -DGGML_HIP=ON"
    echo "  cmake --build build --config Release -j\$(nproc)"
    echo ""
    echo "Continuing with merge step only..."
fi

mkdir -p "${MODELS_DIR}"

# ============================================================================
# Step 1: Merge LoRA adapters into base model
# ============================================================================

echo ""
echo "Step 1: Merging LoRA adapters into base model..."
echo "  Base model:  ${BASE_MODEL}"
echo "  Adapter:     ${ADAPTER_DIR}"
echo "  Output:      ${MERGED_DIR}"
echo ""

# Activate venv
source "${VENV}/bin/activate"

ADAPTER_DIR_VAL="${ADAPTER_DIR}" \
MERGED_DIR_VAL="${MERGED_DIR}" \
BASE_MODEL_VAL="${BASE_MODEL}" \
python3 -c "
import sys, os
from pathlib import Path

try:
    import torch
    from transformers import AutoModelForCausalLM, AutoTokenizer
    from peft import PeftModel
except ImportError as e:
    print(f'ERROR: Missing dependency: {e}')
    print('  pip install transformers peft torch')
    sys.exit(1)

ADAPTER_DIR = Path(os.environ['ADAPTER_DIR_VAL'])
MERGED_DIR = Path(os.environ['MERGED_DIR_VAL'])
BASE_MODEL = os.environ['BASE_MODEL_VAL']

print('Loading base model...')
model = AutoModelForCausalLM.from_pretrained(
    BASE_MODEL,
    torch_dtype=torch.bfloat16,
    device_map='cpu',
)

print('Loading LoRA adapter...')
model = PeftModel.from_pretrained(model, str(ADAPTER_DIR))

print('Merging weights...')
model = model.merge_and_unload()

print(f'Saving merged model to {MERGED_DIR}...')
MERGED_DIR.mkdir(parents=True, exist_ok=True)
model.save_pretrained(str(MERGED_DIR))

tokenizer = AutoTokenizer.from_pretrained(str(ADAPTER_DIR))
tokenizer.save_pretrained(str(MERGED_DIR))

print('Merge complete.')
"

echo ""
echo "  If the above failed, run the merge manually:"
echo ""
echo "    source ${VENV}/bin/activate"
echo "    python3 -c \""
echo "    from transformers import AutoModelForCausalLM, AutoTokenizer"
echo "    from peft import PeftModel"
echo "    import torch"
echo "    model = AutoModelForCausalLM.from_pretrained('${BASE_MODEL}', torch_dtype=torch.bfloat16, device_map='cpu')"
echo "    model = PeftModel.from_pretrained(model, '${ADAPTER_DIR}')"
echo "    model = model.merge_and_unload()"
echo "    model.save_pretrained('${MERGED_DIR}')"
echo "    AutoTokenizer.from_pretrained('${ADAPTER_DIR}').save_pretrained('${MERGED_DIR}')"
echo "    \""

# ============================================================================
# Step 2: Convert to GGUF format
# ============================================================================

echo ""
echo "Step 2: Converting merged model to GGUF format..."
echo ""

if [ -f "${CONVERT_SCRIPT}" ]; then
    python3 "${CONVERT_SCRIPT}" "${MERGED_DIR}" \
        --outfile "${MODELS_DIR}/zhen-raft-mistral-7b-f16.gguf" \
        --outtype f16
    echo "  F16 GGUF created: ${MODELS_DIR}/zhen-raft-mistral-7b-f16.gguf"
else
    echo "  SKIPPED: convert script not found at ${CONVERT_SCRIPT}"
    echo ""
    echo "  Manual command:"
    echo "    python3 ${CONVERT_SCRIPT} ${MERGED_DIR} \\"
    echo "        --outfile ${MODELS_DIR}/zhen-raft-mistral-7b-f16.gguf \\"
    echo "        --outtype f16"
fi

# ============================================================================
# Step 3: Quantize to Q5_K_M
# ============================================================================

echo ""
echo "Step 3: Quantizing to Q5_K_M..."
echo ""

F16_GGUF="${MODELS_DIR}/zhen-raft-mistral-7b-f16.gguf"

if [ -f "${QUANTIZE_BIN}" ] && [ -f "${F16_GGUF}" ]; then
    "${QUANTIZE_BIN}" "${F16_GGUF}" "${OUTPUT_GGUF}" Q5_K_M
    echo "  Quantized GGUF created: ${OUTPUT_GGUF}"

    # Show file size
    ls -lh "${OUTPUT_GGUF}"
else
    echo "  SKIPPED: quantize binary or F16 GGUF not found"
    echo ""
    echo "  Manual command:"
    echo "    ${QUANTIZE_BIN} ${F16_GGUF} ${OUTPUT_GGUF} Q5_K_M"
fi

# ============================================================================
# Step 4: Cleanup (optional)
# ============================================================================

echo ""
echo "Step 4: Cleanup (optional)"
echo ""
echo "  After verifying the quantized model works, you can remove intermediates:"
echo "    rm -rf ${MERGED_DIR}"
echo "    rm -f ${F16_GGUF}"
echo ""

# ============================================================================
# Summary
# ============================================================================

echo "============================================================"
echo "Post-Training Pipeline Summary"
echo "============================================================"
echo ""
echo "  Adapter weights:     ${ADAPTER_DIR}"
echo "  Merged model:        ${MERGED_DIR}"
echo "  F16 GGUF:            ${F16_GGUF}"
echo "  Q5_K_M GGUF:         ${OUTPUT_GGUF}"
echo ""
echo "  To test with llama.cpp:"
echo "    ${LLAMA_CPP_DIR}/build/bin/llama-cli \\"
echo "        -m ${OUTPUT_GGUF} \\"
echo "        -p '[INST] You are Zhen, the AI champion of Unheaded. What is Wotan? [/INST]' \\"
echo "        -n 256 --temp 0.7"
echo ""
echo "  To serve with llama.cpp server:"
echo "    ${LLAMA_CPP_DIR}/build/bin/llama-server \\"
echo "        -m ${OUTPUT_GGUF} \\"
echo "        --host 0.0.0.0 --port 20100"
echo ""
echo "Done."
