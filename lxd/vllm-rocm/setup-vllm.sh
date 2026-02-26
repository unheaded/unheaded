#!/bin/bash
# SPDX-License-Identifier: MIT
# Setup vLLM/ROCm container on host-a
# Creates model storage, launches container, installs dependencies

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Unheaded Kingdom — vLLM/ROCm Setup"
echo "===================================="
echo ""

# Create model weights storage on host
echo "Creating model storage on host..."
sudo mkdir -p /data/unheaded/models
sudo chmod 755 /data/unheaded/models

# Allow write access for the container
sudo setfacl -m u:10001:rwx /data/unheaded/models 2>/dev/null || sudo chmod 777 /data/unheaded/models

echo "Model storage created at /data/unheaded/models"
echo ""

# Launch vLLM container
echo "Launching vLLM/ROCm container (unheaded-vllm-deepseek)..."
lxc launch -f "$SCRIPT_DIR/vllm-lxd.yaml" ubuntu:22.04 unheaded-vllm-deepseek

echo ""
echo "Waiting for container to boot..."
sleep 15

echo "Checking container status..."
lxc list unheaded-vllm-deepseek

echo ""
echo "Container is booting. Cloud-init is installing ROCm and vLLM..."
echo "This may take 5-10 minutes."
echo ""
echo "Monitor installation progress with:"
echo "  lxc exec unheaded-vllm-deepseek -- tail -f /var/log/cloud-init-output.log"
echo ""

# Wait for cloud-init to complete
echo "Waiting for cloud-init completion..."
for i in {1..120}; do
  if lxc exec unheaded-vllm-deepseek -- cloud-init status --format=json 2>/dev/null | grep -q '"done": true'; then
    echo "Cloud-init completed!"
    break
  fi
  if [ $((i % 10)) -eq 0 ]; then
    echo "Still waiting... ($i seconds elapsed)"
  fi
  sleep 1
done

echo ""
echo "Checking vLLM service status..."
lxc exec unheaded-vllm-deepseek -- systemctl status vllm --no-pager || true

echo ""
echo "Checking GPU access in container..."
lxc exec unheaded-vllm-deepseek -- rocm-smi || echo "Note: rocm-smi not yet available"

echo ""
echo "vLLM container setup complete!"
echo ""
echo "Access vLLM API at: http://10.20.1.99:20100"
echo "Model: deepseek-ai/DeepSeek-R1-Distill-Llama-7B (AWQ quantized)"
echo ""
echo "Test with:"
echo "  curl http://10.20.1.99:20100/v1/models"
echo ""
echo "Container logs:"
echo "  lxc exec unheaded-vllm-deepseek -- journalctl -fu vllm -n 50"
