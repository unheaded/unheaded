#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# load-images.sh — Pre-pull and save Docker images for air-gapped Dark Mirror deployment
#
# This script has two modes:
#   1. SAVE mode (internet-connected host): Pull images and save as .tar files
#   2. LOAD mode (Tomb VM, air-gapped): Load images from .tar files
#
# Usage:
#   # On internet-connected machine:
#   ./load-images.sh --save /tmp/dark-mirror-images/
#
#   # Transfer /tmp/dark-mirror-images/ to Tomb VM (SCP, USB, virtio-fs)
#
#   # On Tomb VM (air-gapped):
#   ./load-images.sh --load /tmp/dark-mirror-images/

set -euo pipefail

# --- Docker images required for Dark Mirror stack ---
IMAGES=(
    "prom/prometheus:v2.50.1"
    "grafana/grafana:10.3.3"
    "grafana/loki:2.9.4"
    "grafana/promtail:2.9.4"
)

MODE=""
IMAGES_DIR=""

# --- Argument Parsing ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        --save)
            MODE="save"
            IMAGES_DIR="$2"
            shift 2
            ;;
        --load)
            MODE="load"
            IMAGES_DIR="$2"
            shift 2
            ;;
        --list)
            echo "Docker images required for Dark Mirror:"
            for img in "${IMAGES[@]}"; do
                echo "  - $img"
            done
            exit 0
            ;;
        --help|-h)
            cat <<'USAGE'
Usage: load-images.sh [--save DIR | --load DIR | --list]

Modes:
  --save DIR    Pull Docker images and save as .tar files to DIR
                (Run on an internet-connected machine)

  --load DIR    Load Docker images from .tar files in DIR
                (Run on the air-gapped Tomb VM)

  --list        Show the list of required Docker images

Examples:
  # Internet machine: save images
  ./load-images.sh --save /tmp/dark-mirror-images/

  # Transfer directory to Tomb VM (SCP/USB/virtio-fs)
  scp -r /tmp/dark-mirror-images/ root@tomb-vm:/tmp/

  # Tomb VM: load images
  ./load-images.sh --load /tmp/dark-mirror-images/

Images:
USAGE
            for img in "${IMAGES[@]}"; do
                echo "  - $img"
            done
            exit 0
            ;;
        *) echo "[ERROR] Unknown option: $1"; exit 1 ;;
    esac
done

if [[ -z "$MODE" ]]; then
    echo "[ERROR] Specify --save or --load mode. Use --help for usage."
    exit 1
fi

# --- Helpers ---
log_info()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO]  $*"; }
log_ok()    { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [OK]    $*"; }
log_error() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $*"; }

image_to_filename() {
    # Convert image name to safe filename: "prom/prometheus:v2.50.1" -> "prom_prometheus_v2.50.1.tar"
    echo "$1" | tr '/:' '_' | sed 's/$/.tar/'
}

# --- SAVE Mode ---
save_images() {
    log_info "SAVE MODE — Pulling and saving Docker images"
    log_info "  Output directory: $IMAGES_DIR"
    log_info "  Images to save: ${#IMAGES[@]}"
    echo ""

    mkdir -p "$IMAGES_DIR"

    local saved=0
    local failed=0

    for img in "${IMAGES[@]}"; do
        local filename
        filename=$(image_to_filename "$img")
        local tarpath="${IMAGES_DIR}/${filename}"

        log_info "Pulling: $img ..."
        if docker pull "$img" 2>&1; then
            log_info "Saving: $img -> $filename ..."
            if docker save -o "$tarpath" "$img" 2>&1; then
                local size
                size=$(du -h "$tarpath" | cut -f1)
                log_ok "Saved: $filename ($size)"
                ((saved++))
            else
                log_error "Failed to save: $img"
                ((failed++))
            fi
        else
            log_error "Failed to pull: $img"
            ((failed++))
        fi
        echo ""
    done

    # Generate checksum manifest
    log_info "Generating checksum manifest..."
    cd "$IMAGES_DIR"
    sha256sum *.tar > checksums.sha256 2>/dev/null || true
    cd - >/dev/null

    echo ""
    echo "========================================="
    echo "  Save Results"
    echo "========================================="
    echo "  Saved:  $saved / ${#IMAGES[@]}"
    echo "  Failed: $failed"
    echo "  Dir:    $IMAGES_DIR"
    echo ""

    if [[ $failed -gt 0 ]]; then
        log_error "$failed image(s) failed. Retry or download manually."
        exit 1
    fi

    # Show total size
    local total_size
    total_size=$(du -sh "$IMAGES_DIR" | cut -f1)
    log_ok "Total size: $total_size"

    echo ""
    log_info "Transfer this directory to the Tomb VM:"
    log_info "  scp -r ${IMAGES_DIR} root@<tomb-vm>:/tmp/dark-mirror-images/"
    log_info "Then on the Tomb VM:"
    log_info "  ./load-images.sh --load /tmp/dark-mirror-images/"
}

# --- LOAD Mode ---
load_images() {
    log_info "LOAD MODE — Loading Docker images from tar files (air-gapped)"
    log_info "  Source directory: $IMAGES_DIR"
    echo ""

    if [[ ! -d "$IMAGES_DIR" ]]; then
        log_error "Directory not found: $IMAGES_DIR"
        exit 1
    fi

    # Verify checksums if available
    if [[ -f "${IMAGES_DIR}/checksums.sha256" ]]; then
        log_info "Verifying checksums..."
        if (cd "$IMAGES_DIR" && sha256sum -c checksums.sha256 2>/dev/null); then
            log_ok "All checksums verified"
        else
            log_error "Checksum verification failed! Images may be corrupted."
            echo "Proceed anyway? [y/N]"
            read -r answer
            if [[ "$answer" != "y" && "$answer" != "Y" ]]; then
                exit 1
            fi
        fi
        echo ""
    fi

    # Check Docker is available
    if ! docker info &>/dev/null; then
        log_error "Docker daemon is not running."
        exit 1
    fi

    local loaded=0
    local failed=0

    for tarfile in "${IMAGES_DIR}"/*.tar; do
        if [[ ! -f "$tarfile" ]]; then
            continue
        fi

        local filename
        filename=$(basename "$tarfile")
        local size
        size=$(du -h "$tarfile" | cut -f1)

        log_info "Loading: $filename ($size) ..."
        if docker load -i "$tarfile" 2>&1; then
            log_ok "Loaded: $filename"
            ((loaded++))
        else
            log_error "Failed to load: $filename"
            ((failed++))
        fi
    done

    echo ""
    echo "========================================="
    echo "  Load Results"
    echo "========================================="
    echo "  Loaded: $loaded"
    echo "  Failed: $failed"
    echo ""

    if [[ $failed -gt 0 ]]; then
        log_error "$failed image(s) failed to load."
        exit 1
    fi

    # Verify loaded images
    log_info "Verifying loaded images..."
    local missing=0
    for img in "${IMAGES[@]}"; do
        if docker image inspect "$img" &>/dev/null; then
            log_ok "Available: $img"
        else
            log_error "Missing: $img"
            ((missing++))
        fi
    done

    if [[ $missing -eq 0 ]]; then
        echo ""
        log_ok "All ${#IMAGES[@]} images loaded and verified."
        log_info "Ready to start Dark Mirror: docker compose up -d"
    else
        log_error "$missing image(s) are missing after load."
        exit 1
    fi
}

# --- Main ---
case "$MODE" in
    save) save_images ;;
    load) load_images ;;
    *) echo "[ERROR] Invalid mode: $MODE"; exit 1 ;;
esac
