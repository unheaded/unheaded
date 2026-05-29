#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# setup-cerydwyn.sh — Install Ollama + model setup inside the air-gapped Tomb VM
#
# Prerequisites:
#   - Ollama binary pre-transferred to /tmp/ollama-binary
#   - Model GGUF file pre-transferred to /tmp/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf
#   - ChromaDB + sentence-transformers wheels in /tmp/cerydwyn-wheels/
#   - Python 3.8+ available
#   - Run as root or with sudo
#
# Usage: sudo ./setup-cerydwyn.sh [--skip-model] [--skip-rag]

set -euo pipefail

# --- Configuration ---
TOMB_ROOT="/opt/tomb"
CERYDWYN_ROOT="${TOMB_ROOT}/cerydwyn"
OLLAMA_BIN_DIR="/opt/ollama/bin"
OLLAMA_MODELS_DIR="/var/lib/ollama/models"
OLLAMA_USER="ollama"
OLLAMA_PORT="11434"
GRIMOIRE_ROOT="${TOMB_ROOT}/grimoire"

MODEL_SRC="/tmp/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf"
OLLAMA_SRC="/tmp/ollama-binary"
WHEELS_DIR="/tmp/cerydwyn-wheels"

SKIP_MODEL=false
SKIP_RAG=false

# --- Argument Parsing ---
for arg in "$@"; do
    case "$arg" in
        --skip-model) SKIP_MODEL=true ;;
        --skip-rag)   SKIP_RAG=true ;;
        --help|-h)
            echo "Usage: sudo $0 [--skip-model] [--skip-rag]"
            echo ""
            echo "  --skip-model   Skip Ollama binary and model weight installation"
            echo "  --skip-rag     Skip RAG dependency installation (ChromaDB, etc.)"
            exit 0
            ;;
        *) echo "[ERROR] Unknown argument: $arg"; exit 1 ;;
    esac
done

# --- Helpers ---
log_info()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO]  $*"; }
log_warn()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [WARN]  $*"; }
log_error() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $*"; }
log_ok()    { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [OK]    $*"; }

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (or with sudo)."
        exit 1
    fi
}

# --- Phase 1: Directory Structure ---
setup_directories() {
    log_info "Creating Cerydwyn directory structure..."

    mkdir -p "${CERYDWYN_ROOT}"/{prompts,logs,suggestions,history,config}
    mkdir -p "${OLLAMA_BIN_DIR}"
    mkdir -p "${OLLAMA_MODELS_DIR}"

    log_ok "Directory structure created at ${CERYDWYN_ROOT}"
}

# --- Phase 2: Ollama Binary Installation ---
install_ollama() {
    if [[ "$SKIP_MODEL" == "true" ]]; then
        log_warn "Skipping Ollama installation (--skip-model)"
        return 0
    fi

    log_info "Installing Ollama binary..."

    if [[ ! -f "$OLLAMA_SRC" ]]; then
        log_error "Ollama binary not found at $OLLAMA_SRC"
        log_error "Transfer it from an internet-connected machine first."
        exit 1
    fi

    cp "$OLLAMA_SRC" "${OLLAMA_BIN_DIR}/ollama"
    chmod +x "${OLLAMA_BIN_DIR}/ollama"

    # Create ollama user if it does not exist
    if ! id "$OLLAMA_USER" &>/dev/null; then
        useradd -r -m -s /bin/false "$OLLAMA_USER"
        log_info "Created system user: $OLLAMA_USER"
    fi

    chown -R "${OLLAMA_USER}:${OLLAMA_USER}" "${OLLAMA_BIN_DIR}"
    chown -R "${OLLAMA_USER}:${OLLAMA_USER}" "${OLLAMA_MODELS_DIR}"

    # Verify installation
    if "${OLLAMA_BIN_DIR}/ollama" --version &>/dev/null; then
        log_ok "Ollama installed: $(${OLLAMA_BIN_DIR}/ollama --version 2>&1)"
    else
        log_warn "Ollama binary installed but --version check inconclusive"
    fi
}

# --- Phase 3: Systemd Service ---
install_systemd_service() {
    if [[ "$SKIP_MODEL" == "true" ]]; then
        log_warn "Skipping systemd service (--skip-model)"
        return 0
    fi

    log_info "Installing Ollama systemd service..."

    local service_src
    service_src="$(dirname "$(readlink -f "$0")")/ollama.service"

    if [[ -f "$service_src" ]]; then
        cp "$service_src" /etc/systemd/system/ollama.service
    else
        # Inline fallback if the service file is missing
        cat > /etc/systemd/system/ollama.service <<'UNIT'
[Unit]
Description=Ollama Local LLM Service — Tomb of Knowledge Cerydwyn
After=network.target
Documentation=https://github.com/unheaded/unheaded

[Service]
Type=simple
User=ollama
Group=ollama
ExecStart=/opt/ollama/bin/ollama serve
Restart=on-failure
RestartSec=10
TimeoutStartSec=30
TimeoutStopSec=30

Environment="OLLAMA_MODELS=/var/lib/ollama/models"
Environment="OLLAMA_HOST=127.0.0.1:11434"
Environment="OLLAMA_KEEP_ALIVE=24h"

# Hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/ollama
ReadOnlyPaths=/opt/ollama

[Install]
WantedBy=multi-user.target
UNIT
    fi

    systemctl daemon-reload
    systemctl enable ollama
    systemctl start ollama

    # Wait for startup
    sleep 3

    if systemctl is-active --quiet ollama; then
        log_ok "Ollama systemd service started and enabled"
    else
        log_error "Ollama service failed to start. Check: journalctl -u ollama"
        return 1
    fi
}

# --- Phase 4: Model Weight Installation ---
install_model() {
    if [[ "$SKIP_MODEL" == "true" ]]; then
        log_warn "Skipping model installation (--skip-model)"
        return 0
    fi

    log_info "Installing Mistral-7B model weights..."

    if [[ ! -f "$MODEL_SRC" ]]; then
        log_error "Model file not found at $MODEL_SRC"
        log_error "Transfer Mistral-7B-Instruct-v0.1.Q4_K_M.gguf from an internet-connected machine."
        exit 1
    fi

    cp "$MODEL_SRC" "${OLLAMA_MODELS_DIR}/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf"
    chown "${OLLAMA_USER}:${OLLAMA_USER}" "${OLLAMA_MODELS_DIR}/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf"
    chmod 644 "${OLLAMA_MODELS_DIR}/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf"

    log_ok "Model weights installed ($(du -h "${OLLAMA_MODELS_DIR}/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf" | cut -f1))"

    # Create the cerydwyn-mistral model from the Modelfile
    log_info "Creating cerydwyn-mistral model via Ollama Modelfile..."

    local modelfile_src
    modelfile_src="$(dirname "$(readlink -f "$0")")/Modelfile-mistral"

    if [[ -f "$modelfile_src" ]]; then
        "${OLLAMA_BIN_DIR}/ollama" create cerydwyn-mistral -f "$modelfile_src"
        log_ok "cerydwyn-mistral model created"
    else
        log_warn "Modelfile-mistral not found at $modelfile_src — create model manually later"
    fi
}

# --- Phase 5: Cerydwyn Scripts Deployment ---
deploy_scripts() {
    log_info "Deploying Cerydwyn scripts..."

    local script_dir
    script_dir="$(dirname "$(readlink -f "$0")")"

    # Copy scripts to Cerydwyn root
    for script in cerydwyn.sh cerydwyn-tui.py cerydwyn-daemon.py cerydwyn-test.py; do
        if [[ -f "${script_dir}/${script}" ]]; then
            cp "${script_dir}/${script}" "${CERYDWYN_ROOT}/${script}"
            chmod +x "${CERYDWYN_ROOT}/${script}"
            log_ok "Deployed ${script}"
        else
            log_warn "Script ${script} not found in ${script_dir}"
        fi
    done

    # Copy system prompt
    if [[ -d "${script_dir}/prompts" ]]; then
        cp -r "${script_dir}/prompts/"* "${CERYDWYN_ROOT}/prompts/" 2>/dev/null || true
        log_ok "Deployed system prompts"
    fi

    log_ok "Cerydwyn scripts deployed to ${CERYDWYN_ROOT}"
}

# --- Phase 6: RAG Dependencies ---
install_rag_deps() {
    if [[ "$SKIP_RAG" == "true" ]]; then
        log_warn "Skipping RAG dependencies (--skip-rag)"
        return 0
    fi

    log_info "Installing RAG pipeline dependencies..."

    # Check for pre-packaged Python wheels (air-gapped install)
    if [[ -d "$WHEELS_DIR" ]]; then
        pip3 install --no-index --find-links="$WHEELS_DIR" \
            chromadb \
            sentence-transformers \
            requests \
            2>&1 | tail -5
        log_ok "RAG Python dependencies installed from offline wheels"
    else
        log_warn "Offline wheels not found at $WHEELS_DIR"
        log_warn "RAG dependencies must be installed manually."
        log_warn "Required: chromadb, sentence-transformers, requests"
    fi

    # Create RAG config
    cat > "${CERYDWYN_ROOT}/config/rag-config.json" <<EOF
{
    "ollama_url": "http://127.0.0.1:${OLLAMA_PORT}",
    "model": "cerydwyn-mistral",
    "grimoire_root": "${GRIMOIRE_ROOT}",
    "chromadb_path": "${GRIMOIRE_ROOT}/rag/chroma.db",
    "embedding_model": "all-MiniLM-L6-v2",
    "top_k": 5,
    "chunk_size": 512,
    "chunk_overlap": 64,
    "max_context_tokens": 4096,
    "temperature": 0.3,
    "system_prompt_path": "${CERYDWYN_ROOT}/prompts/system-cerydwyn.txt",
    "suggestion_log_dir": "${CERYDWYN_ROOT}/suggestions",
    "query_log_dir": "${CERYDWYN_ROOT}/logs",
    "history_dir": "${CERYDWYN_ROOT}/history"
}
EOF

    log_ok "RAG config written to ${CERYDWYN_ROOT}/config/rag-config.json"
}

# --- Phase 7: Verification ---
verify_installation() {
    log_info "Running post-install verification..."

    local failures=0

    # Check directories
    for dir in "${CERYDWYN_ROOT}" "${CERYDWYN_ROOT}/prompts" "${CERYDWYN_ROOT}/logs" \
               "${CERYDWYN_ROOT}/suggestions" "${CERYDWYN_ROOT}/config"; do
        if [[ -d "$dir" ]]; then
            log_ok "Directory exists: $dir"
        else
            log_error "Missing directory: $dir"
            ((failures++))
        fi
    done

    # Check Ollama
    if [[ "$SKIP_MODEL" != "true" ]]; then
        if [[ -x "${OLLAMA_BIN_DIR}/ollama" ]]; then
            log_ok "Ollama binary is executable"
        else
            log_error "Ollama binary missing or not executable"
            ((failures++))
        fi

        if systemctl is-active --quiet ollama 2>/dev/null; then
            log_ok "Ollama service is running"
        else
            log_warn "Ollama service is not running (may need manual start)"
        fi

        # Check Ollama API
        if curl -sf "http://127.0.0.1:${OLLAMA_PORT}/api/tags" &>/dev/null; then
            log_ok "Ollama API is responding on port ${OLLAMA_PORT}"
        else
            log_warn "Ollama API not responding (service may still be starting)"
        fi
    fi

    # Check scripts
    for script in cerydwyn.sh cerydwyn-tui.py cerydwyn-daemon.py cerydwyn-test.py; do
        if [[ -x "${CERYDWYN_ROOT}/${script}" ]]; then
            log_ok "Script executable: ${script}"
        else
            log_warn "Script missing or not executable: ${script}"
        fi
    done

    # Check system prompt
    if [[ -f "${CERYDWYN_ROOT}/prompts/system-cerydwyn.txt" ]]; then
        log_ok "System prompt exists"
    else
        log_warn "System prompt not found at ${CERYDWYN_ROOT}/prompts/system-cerydwyn.txt"
    fi

    echo ""
    if [[ $failures -eq 0 ]]; then
        log_ok "=== Cerydwyn installation complete. All checks passed. ==="
    else
        log_error "=== Cerydwyn installation completed with $failures error(s). ==="
    fi

    return $failures
}

# --- Main ---
main() {
    echo "============================================"
    echo "  Tomb of Knowledge — Cerydwyn Setup"
    echo "  Air-Gapped Ollama + Mistral-7B + RAG"
    echo "============================================"
    echo ""

    check_root

    setup_directories
    install_ollama
    install_systemd_service
    install_model
    deploy_scripts
    install_rag_deps
    verify_installation

    echo ""
    log_info "Next steps:"
    log_info "  1. Verify model: /opt/ollama/bin/ollama list"
    log_info "  2. Test query:   ${CERYDWYN_ROOT}/cerydwyn.sh 'What is the Monad protocol?'"
    log_info "  3. Run TUI:      python3 ${CERYDWYN_ROOT}/cerydwyn-tui.py"
    log_info "  4. Start daemon: python3 ${CERYDWYN_ROOT}/cerydwyn-daemon.py &"
    log_info "  5. Run tests:    python3 ${CERYDWYN_ROOT}/cerydwyn-test.py"
}

main "$@"
