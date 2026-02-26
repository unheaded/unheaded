# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# docker/vllm-rocm/Dockerfile — vLLM + ROCm for AMD RX 7700 XT (gfx1101)

ARG ROCM_VERSION=6.1.3
ARG VLLM_VERSION=0.4.3

FROM rocm/pytorch:${ROCM_VERSION}-ubuntu22.04 AS base

ENV DEBIAN_FRONTEND=noninteractive \
    ROCM_HOME=/opt/rocm \
    PATH=/opt/rocm/bin:/opt/rocm/llvm/bin:$PATH \
    LD_LIBRARY_PATH=/opt/rocm/lib:$LD_LIBRARY_PATH

# Update system and install dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3-pip \
    python3-dev \
    python3-venv \
    git \
    curl \
    wget \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Upgrade pip
RUN pip3 install --no-cache-dir --upgrade pip setuptools wheel

# Install vLLM with ROCm support
RUN pip3 install --no-cache-dir \
    vllm==${VLLM_VERSION} \
    --extra-index-url https://download.pytorch.org/whl/rocm6.1

# Install additional dependencies
RUN pip3 install --no-cache-dir \
    prometheus-client \
    pydantic \
    ray \
    typing-extensions

# Create non-root user for vLLM service
RUN groupadd -g 10001 vllm && \
    useradd -u 10001 -g vllm -d /home/vllm -s /sbin/nologin -m vllm && \
    mkdir -p /vllm && \
    chown -R vllm:vllm /vllm

WORKDIR /vllm

# Model weights mounted at /models (read-only)
VOLUME ["/models"]

USER vllm:vllm
EXPOSE 20100

# Set environment variables for AMD RX 7700 XT (gfx1101)
ENV HSA_OVERRIDE_GFX_VERSION=11.0.1 \
    PYTORCH_ROCM_ARCH=gfx1101 \
    HIP_VISIBLE_DEVICES=0 \
    HIP_DEVICE_SELECT=0 \
    VLLM_LOGGING_LEVEL=INFO

HEALTHCHECK --interval=30s --timeout=10s --start-period=120s --retries=5 \
    CMD curl -f http://localhost:20100/health || exit 1

ENTRYPOINT ["python3", "-m", "vllm.entrypoints.openai.api_server"]

CMD ["--model", "/models/deepseek-r1-7b-q4", \
     "--port", "20100", \
     "--host", "0.0.0.0", \
     "--dtype", "half", \
     "--gpu-memory-utilization", "0.90", \
     "--max-model-len", "8192", \
     "--tensor-parallel-size", "1"]
