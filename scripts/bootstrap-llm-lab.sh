#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# ==============================================================================
# bootstrap-llm-lab.sh — THE FORGE IGNITION SEQUENCE
#
# Brings a bare-bones Ubuntu 25.04/25.10 install to full Unheaded LLM Lab Server
# Target Hardware: AMD RX 7700 XT (gfx1101), 1TB NVMe, 2TB HDD
#
# Usage:  sudo ./bootstrap-llm-lab.sh [--dry-run] [--skip-reboot]
# Requires: Root/sudo, internet, bare Ubuntu 25.04+ install
# ==============================================================================

set -euo pipefail
IFS=$'\n\t'

# ─────────────────────────────────────────────────────────────────────────────
# CONFIG — Tune these knobs
# ─────────────────────────────────────────────────────────────────────────────
readonly ROCM_VERSION="6.4"                       # ROCm major.minor
readonly ROCM_APT_VERSION="6.4.2"                 # APT package version
readonly AMDGPU_INSTALL_VERSION="6.4.60402-1"     # amdgpu-install version
readonly GO_VERSION="1.24.0"                      # Go toolchain
readonly RUST_TOOLCHAIN="nightly"                 # Rust nightly for eBPF
readonly NODE_VERSION="22"                        # Node LTS for dashboard
readonly VLLM_PORT=20100                          # vLLM inference port
readonly NVME_MOUNT="/mnt/nvme"                   # Fast storage mount
readonly HDD_MOUNT="/mnt/hdd"                     # Slow storage mount
readonly MODELS_DIR="/mnt/hdd/models"             # LLM model weights
readonly UNHEADED_HOME="/opt/unheaded"            # Kingdom home
readonly UNHEADED_USER="unheaded"                 # Service user
readonly LOG_FILE="/var/log/unheaded-bootstrap.log"

# Feature flags
DRY_RUN=false
SKIP_REBOOT=false

# ─────────────────────────────────────────────────────────────────────────────
# PARSE ARGS
# ─────────────────────────────────────────────────────────────────────────────
for arg in "$@"; do
    case "$arg" in
        --dry-run)   DRY_RUN=true ;;
        --skip-reboot) SKIP_REBOOT=true ;;
        --help|-h)
            echo "Usage: sudo $0 [--dry-run] [--skip-reboot]"
            echo "  --dry-run      Print what would be done, don't execute"
            echo "  --skip-reboot  Skip final reboot prompt"
            exit 0
            ;;
    esac
done

# ─────────────────────────────────────────────────────────────────────────────
# COLORS + LOGGING
# ─────────────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
NC='\033[0m'

_log() {
    local level="$1" color="$2"
    shift 2
    local msg="$*"
    local ts
    ts=$(date '+%Y-%m-%d %H:%M:%S')
    echo -e "${color}[${level}]${NC} ${msg}"
    echo "[${ts}] [${level}] ${msg}" >> "${LOG_FILE}" 2>/dev/null || true
}

info()    { _log "INFO"    "$BLUE"    "$@"; }
ok()      { _log "OK"      "$GREEN"   "$@"; }
warn()    { _log "WARN"    "$YELLOW"  "$@"; }
err()     { _log "ERROR"   "$RED"     "$@"; }
phase()   { echo ""; _log "PHASE"    "$MAGENTA" "═══ $* ═══"; echo ""; }
step()    { _log "STEP"    "$CYAN"    "→ $*"; }

run() {
    if $DRY_RUN; then
        info "[DRY-RUN] $*"
    else
        "$@"
    fi
}

die() { err "$@"; exit 1; }

# ─────────────────────────────────────────────────────────────────────────────
# PREFLIGHT CHECKS
# ─────────────────────────────────────────────────────────────────────────────
preflight() {
    phase "PREFLIGHT CHECKS"

    # Root check
    [[ $EUID -eq 0 ]] || die "Must run as root. Use: sudo $0"

    # OS detection
    if [[ -f /etc/os-release ]]; then
        # shellcheck source=/dev/null
        . /etc/os-release
        info "OS: ${PRETTY_NAME}"
        info "Version ID: ${VERSION_ID}"
    else
        die "/etc/os-release not found. Is this Ubuntu?"
    fi

    # Validate Ubuntu 25.x
    case "${VERSION_ID}" in
        25.04|25.10) ok "Ubuntu ${VERSION_ID} detected — supported" ;;
        24.04|24.10) warn "Ubuntu ${VERSION_ID} — will work but script targets 25.x" ;;
        *)           warn "Ubuntu ${VERSION_ID} — untested, proceeding anyway" ;;
    esac

    # Kernel version
    local kver
    kver=$(uname -r)
    info "Kernel: ${kver}"

    # Check architecture
    local arch
    arch=$(uname -m)
    [[ "$arch" == "x86_64" ]] || die "x86_64 required, got: ${arch}"
    ok "Architecture: ${arch}"

    # Internet connectivity
    step "Testing internet connectivity..."
    if ping -c1 -W3 archive.ubuntu.com &>/dev/null; then
        ok "Internet: reachable"
    else
        die "No internet — cannot proceed"
    fi

    # Detect AMD GPU
    step "Detecting AMD GPU..."
    if lspci 2>/dev/null | grep -i "VGA\|Display\|3D" | grep -qi "AMD\|ATI\|Radeon"; then
        local gpu_info
        gpu_info=$(lspci | grep -i "VGA\|Display\|3D" | grep -i "AMD\|ATI\|Radeon" | head -1)
        ok "AMD GPU found: ${gpu_info}"
    else
        warn "No AMD GPU detected via lspci — ROCm install may fail"
    fi

    # Detect storage devices
    step "Detecting storage..."
    lsblk -d -o NAME,SIZE,ROTA,MODEL,TRAN 2>/dev/null | head -20
    echo ""

    # Memory
    local mem_gb
    mem_gb=$(awk '/MemTotal/ {printf "%.0f", $2/1024/1024}' /proc/meminfo)
    info "RAM: ${mem_gb}GB"
    [[ $mem_gb -ge 16 ]] || warn "Less than 16GB RAM — LLM inference may be constrained"

    # CPU
    local cpu_model cpu_cores
    cpu_model=$(grep -m1 "model name" /proc/cpuinfo | cut -d: -f2 | xargs)
    cpu_cores=$(nproc)
    info "CPU: ${cpu_model} (${cpu_cores} cores)"

    ok "Preflight complete"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 1: SYSTEM UPDATE + BASE PACKAGES
# ─────────────────────────────────────────────────────────────────────────────
phase_system_base() {
    phase "1/16 — SYSTEM UPDATE + BASE PACKAGES"

    export DEBIAN_FRONTEND=noninteractive

    step "Updating package lists..."
    run apt-get update -y

    step "Full system upgrade..."
    run apt-get dist-upgrade -y

    step "Installing base packages..."
    run apt-get install -y \
        build-essential \
        linux-headers-"$(uname -r)" \
        linux-tools-"$(uname -r)" \
        linux-tools-common \
        dkms \
        curl wget \
        ca-certificates \
        gnupg lsb-release \
        apt-transport-https \
        software-properties-common \
        git git-lfs \
        jq yq \
        htop btop iotop sysstat \
        tmux screen \
        vim neovim \
        unzip p7zip-full \
        tree ncdu \
        net-tools iproute2 \
        iputils-ping traceroute mtr-tiny \
        dnsutils bind9-dnsutils \
        bridge-utils \
        ethtool \
        iptables nftables \
        wireguard-tools \
        tcpdump nmap \
        strace ltrace \
        gdb \
        pkg-config \
        libssl-dev libelf-dev libz-dev \
        libbpf-dev \
        cmake ninja-build \
        python3 python3-pip python3-venv python3-dev \
        bc \
        pciutils usbutils lshw \
        smartmontools nvme-cli \
        fio \
        rsync \
        fail2ban \
        ufw \
        acl \
        zstd lz4 \
        bpftool \
        bpftrace \
        bpfcc-tools \
        clang llvm \
        libclang-dev \
        linux-cloud-tools-"$(uname -r)" 2>/dev/null || true

    step "Installing performance analysis tools..."
    run apt-get install -y \
        perf-tools-unstable 2>/dev/null || \
    run apt-get install -y \
        linux-perf 2>/dev/null || \
        warn "perf tools not available in repo — install manually later"

    ok "Base packages installed"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 2: STORAGE LAYOUT — NVMe fast / HDD slow
# ─────────────────────────────────────────────────────────────────────────────
phase_storage() {
    phase "2/16 — STORAGE LAYOUT"

    info "Strategy: NVMe (1TB) = OS + containers + fast I/O"
    info "          HDD  (2TB) = models + logs + backups + cold data"
    info ""
    info "Assuming OS is already on NVMe (root filesystem)."
    info "HDD needs partitioning + mount."

    # Detect HDD (rotational=1, non-root)
    step "Detecting HDD..."
    local hdd_dev=""
    local root_dev
    root_dev=$(findmnt -n -o SOURCE / | sed 's/[0-9]*$//' | sed 's/p$//')

    while IFS= read -r line; do
        local dev rota
        dev=$(echo "$line" | awk '{print $1}')
        rota=$(echo "$line" | awk '{print $2}')
        if [[ "$rota" == "1" && "/dev/${dev}" != "${root_dev}" ]]; then
            hdd_dev="/dev/${dev}"
            break
        fi
    done < <(lsblk -d -n -o NAME,ROTA | grep -v "loop\|sr\|rom")

    if [[ -n "$hdd_dev" ]]; then
        ok "HDD detected: ${hdd_dev}"

        # Check if already mounted
        if mountpoint -q "${HDD_MOUNT}" 2>/dev/null; then
            ok "HDD already mounted at ${HDD_MOUNT}"
        else
            step "Checking if HDD has a filesystem..."
            if blkid "${hdd_dev}1" &>/dev/null; then
                info "Existing partition found on ${hdd_dev}1"
                info "Skipping format — mount existing filesystem"
                run mkdir -p "${HDD_MOUNT}"
                if ! grep -q "${HDD_MOUNT}" /etc/fstab; then
                    local hdd_uuid
                    hdd_uuid=$(blkid -s UUID -o value "${hdd_dev}1" 2>/dev/null || echo "")
                    if [[ -n "$hdd_uuid" ]]; then
                        echo "UUID=${hdd_uuid} ${HDD_MOUNT} ext4 defaults,noatime 0 2" >> /etc/fstab
                        run mount "${HDD_MOUNT}"
                        ok "HDD mounted at ${HDD_MOUNT}"
                    else
                        warn "Could not determine UUID for ${hdd_dev}1 — mount manually"
                    fi
                fi
            else
                warn "No filesystem on ${hdd_dev} — partition + format manually:"
                warn "  sudo parted ${hdd_dev} mklabel gpt"
                warn "  sudo parted ${hdd_dev} mkpart primary ext4 0% 100%"
                warn "  sudo mkfs.ext4 -L unheaded-hdd ${hdd_dev}1"
                warn "Then re-run this script."
            fi
        fi
    else
        warn "No HDD detected — using NVMe for everything"
    fi

    # Create directory hierarchy
    step "Creating directory hierarchy..."
    run mkdir -p "${MODELS_DIR}"
    run mkdir -p "${HDD_MOUNT}"/{logs,backups,data,scratch,cache,datasets,tensorboard,wandb}
    run mkdir -p "${NVME_MOUNT}" 2>/dev/null || true
    run mkdir -p "${UNHEADED_HOME}"/{bin,config,data,logs,state,ebpf,certs}
    run mkdir -p /var/lib/unheaded/{state,ebpf,containers}
    run mkdir -p /etc/unheaded
    run mkdir -p /var/log/unheaded

    # ── MAD SCIENTIST STORAGE STRATEGY ──
    # NVMe = Active Memory (OS, containers, active model, KV cache, scratch)
    # HDD  = Deep Storage (model library, datasets, logs, caches, checkpoints)
    if mountpoint -q "${HDD_MOUNT}" 2>/dev/null; then
        step "Configuring NVMe/HDD split storage strategy..."

        # Models: HDD is the archive, symlink to /mnt/models
        [[ -L /mnt/models ]] || run ln -sf "${MODELS_DIR}" /mnt/models
        ok "Models dir: ${MODELS_DIR} → /mnt/models"

        # HuggingFace cache → HDD (can balloon to 100s of GB)
        run mkdir -p "${HDD_MOUNT}/cache/huggingface"
        run mkdir -p "${HDD_MOUNT}/cache/pip"
        run mkdir -p "${HDD_MOUNT}/cache/ollama"

        # Ollama models → HDD
        run mkdir -p "${HDD_MOUNT}/models/ollama"

        # Active model slot on NVMe (symlink one model here for fast load)
        run mkdir -p /opt/unheaded/active-model
        info "Active model slot: /opt/unheaded/active-model (NVMe)"
        info "  Usage: ln -sf ${MODELS_DIR}/your-model /opt/unheaded/active-model/current"
        info "  Loading from NVMe: ~5-10s vs HDD: ~1-3min for 70B model"

        # Redirect pip cache → HDD
        run mkdir -p "${HDD_MOUNT}/cache/pip"

        # Redirect conda/mamba cache → HDD
        run mkdir -p "${HDD_MOUNT}/cache/conda"

        step "Writing cache redirect environment..."
        cat > /etc/profile.d/unheaded-storage.sh << STENV
# ═══════════════════════════════════════════════════════════
# UNHEADED KINGDOM — MAD SCIENTIST STORAGE STRATEGY
# NVMe = Active Memory | HDD = Deep Storage Archive
# ═══════════════════════════════════════════════════════════

# HuggingFace → HDD (model weights, tokenizers, datasets)
export HF_HOME="${HDD_MOUNT}/cache/huggingface"
export HF_DATASETS_CACHE="${HDD_MOUNT}/cache/huggingface/datasets"
export HUGGINGFACE_HUB_CACHE="${HDD_MOUNT}/cache/huggingface/hub"

# hf_transfer: Rust-based parallel downloads (10x faster)
export HF_HUB_ENABLE_HF_TRANSFER=1

# Ollama → HDD
export OLLAMA_MODELS="${HDD_MOUNT}/models/ollama"

# pip cache → HDD
export PIP_CACHE_DIR="${HDD_MOUNT}/cache/pip"

# conda/mamba → HDD
export CONDA_PKGS_DIRS="${HDD_MOUNT}/cache/conda/pkgs"

# TensorBoard + W&B logs → HDD (write-heavy, not perf-critical)
export TENSORBOARD_LOGDIR="${HDD_MOUNT}/tensorboard"
export WANDB_DIR="${HDD_MOUNT}/wandb"

# XDG cache → HDD (npm, go, misc caches)
export XDG_CACHE_HOME="${HDD_MOUNT}/cache"

# vLLM: AMD AITER acceleration (optimized attention for ROCm)
export VLLM_USE_AITER=1

# Active model slot (NVMe fast lane)
export UNHEADED_ACTIVE_MODEL="/opt/unheaded/active-model/current"
STENV
        chmod 644 /etc/profile.d/unheaded-storage.sh
        ok "Storage environment configured — HDD as deep archive, NVMe as active brain"
    else
        warn "No HDD mounted — all storage on NVMe (will fill fast with models)"
    fi

    ok "Storage layout configured"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 3: AMD GPU DRIVERS + ROCm
# ─────────────────────────────────────────────────────────────────────────────
phase_amd_gpu() {
    phase "3/16 — AMD GPU DRIVERS + ROCm ${ROCM_VERSION}"

    # Check if ROCm already installed
    if command -v rocminfo &>/dev/null; then
        local installed_ver
        installed_ver=$(rocminfo 2>/dev/null | grep -i "Runtime Version" | head -1 || echo "unknown")
        ok "ROCm already installed: ${installed_ver}"
        ok "Skipping driver install — remove first to reinstall"
        return
    fi

    step "Adding AMDGPU repository..."
    # Download and install amdgpu-install package
    local amdgpu_deb="amdgpu-install_${AMDGPU_INSTALL_VERSION}_all.deb"
    local amdgpu_url
    amdgpu_url="https://repo.radeon.com/amdgpu-install/${ROCM_APT_VERSION}/ubuntu/$(lsb_release -cs)/${amdgpu_deb}"

    info "Fetching: ${amdgpu_url}"
    run wget -q "${amdgpu_url}" -O "/tmp/${amdgpu_deb}" || {
        warn "Could not fetch amdgpu-install for $(lsb_release -cs)"
        warn "Trying noble (24.04) packages as fallback..."
        amdgpu_url="https://repo.radeon.com/amdgpu-install/${ROCM_APT_VERSION}/ubuntu/noble/${amdgpu_deb}"
        run wget -q "${amdgpu_url}" -O "/tmp/${amdgpu_deb}" || {
            err "Failed to download amdgpu-install — install ROCm manually"
            err "See: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/"
            return 1
        }
    }

    run dpkg -i "/tmp/${amdgpu_deb}" || true
    run apt-get update -y

    step "Installing AMDGPU driver + ROCm stack..."
    run amdgpu-install -y --usecase=rocm,graphics,opencl --no-dkms || {
        warn "amdgpu-install with --no-dkms failed, trying with dkms..."
        run amdgpu-install -y --usecase=rocm,graphics,opencl
    }

    step "Installing ROCm dev packages..."
    run apt-get install -y \
        rocm-dev \
        rocm-libs \
        rocm-hip-sdk \
        rocm-utils \
        rccl \
        hipblas \
        rocblas \
        rocsolver \
        miopen-hip \
        2>/dev/null || warn "Some ROCm dev packages unavailable — non-fatal"

    # Add current user + unheaded to video/render groups
    step "Configuring GPU access groups..."
    for grp in video render; do
        getent group "$grp" &>/dev/null && {
            usermod -aG "$grp" root 2>/dev/null || true
            id "${UNHEADED_USER}" &>/dev/null && usermod -aG "$grp" "${UNHEADED_USER}" 2>/dev/null || true
            # Add the user who invoked sudo
            [[ -n "${SUDO_USER:-}" ]] && usermod -aG "$grp" "${SUDO_USER}" 2>/dev/null || true
        }
    done

    # Environment variables for gfx1101
    step "Writing ROCm environment..."
    cat > /etc/profile.d/rocm.sh << 'ROCMENV'
# ROCm environment — AMD RX 7700 XT (gfx1101)
export ROCM_HOME=/opt/rocm
export PATH=$ROCM_HOME/bin:$ROCM_HOME/llvm/bin:$PATH
export LD_LIBRARY_PATH=$ROCM_HOME/lib:$ROCM_HOME/lib64:${LD_LIBRARY_PATH:-}
export HSA_OVERRIDE_GFX_VERSION=11.0.1
export PYTORCH_ROCM_ARCH=gfx1101
export HIP_VISIBLE_DEVICES=0
ROCMENV
    chmod 644 /etc/profile.d/rocm.sh

    # Verify
    step "Verifying GPU detection..."
    if /opt/rocm/bin/rocminfo 2>/dev/null | grep -qi "gfx11"; then
        ok "ROCm sees gfx1101 GPU — CONFIRMED"
    else
        warn "rocminfo doesn't show gfx1101 — may need reboot for driver load"
    fi

    rm -f "/tmp/${amdgpu_deb}"
    ok "AMD GPU + ROCm ${ROCM_VERSION} installed"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 4: DOCKER + CONTAINERD
# ─────────────────────────────────────────────────────────────────────────────
phase_docker() {
    phase "4/16 — DOCKER + CONTAINERD"

    if command -v docker &>/dev/null; then
        ok "Docker already installed: $(docker --version)"
    else
        step "Adding Docker GPG key + repo..."
        run install -m 0755 -d /etc/apt/keyrings
        run curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
        run chmod a+r /etc/apt/keyrings/docker.asc

        # Use the closest supported codename
        local codename
        codename=$(. /etc/os-release && echo "${UBUNTU_CODENAME:-$(lsb_release -cs)}")
        # Docker may not have 25.x repos yet — fallback to noble
        local docker_codename="$codename"
        if ! curl -fsSL "https://download.docker.com/linux/ubuntu/dists/${docker_codename}/Release" &>/dev/null; then
            docker_codename="noble"
            warn "Docker repo not available for ${codename}, using ${docker_codename}"
        fi

        echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${docker_codename} stable" \
            > /etc/apt/sources.list.d/docker.list

        run apt-get update -y
        step "Installing Docker CE..."
        run apt-get install -y \
            docker-ce docker-ce-cli containerd.io \
            docker-buildx-plugin docker-compose-plugin
    fi

    # Configure Docker daemon
    step "Configuring Docker daemon..."
    mkdir -p /etc/docker
    cat > /etc/docker/daemon.json << 'DOCKERCFG'
{
    "log-driver": "json-file",
    "log-opts": {
        "max-size": "100m",
        "max-file": "5"
    },
    "storage-driver": "overlay2",
    "default-address-pools": [
        {"base": "172.20.0.0/16", "size": 24}
    ],
    "ipv6": true,
    "fixed-cidr-v6": "fd00:dead:beef::/48",
    "ip6tables": true,
    "experimental": true,
    "metrics-addr": "0.0.0.0:9323",
    "default-runtime": "runc",
    "runtimes": {},
    "dns": ["1.1.1.1", "8.8.8.8"],
    "features": {
        "buildkit": true
    },
    "live-restore": true
}
DOCKERCFG

    step "Starting Docker..."
    run systemctl enable docker
    run systemctl restart docker

    # Add users to docker group
    [[ -n "${SUDO_USER:-}" ]] && run usermod -aG docker "${SUDO_USER}" 2>/dev/null || true

    # Create unheaded network
    step "Creating unheaded Docker network..."
    if ! docker network ls | grep -q unheaded_unheaded; then
        run docker network create \
            --driver bridge \
            --subnet=172.20.0.0/24 \
            --gateway=172.20.0.1 \
            --ipv6 \
            --subnet=fd00:dead:beef:1::/64 \
            --opt com.docker.network.bridge.name=br-unheaded \
            unheaded_unheaded 2>/dev/null || warn "Network creation failed — may already exist"
    fi

    ok "Docker configured"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 5: LXD / INCUS (System Containers)
# ─────────────────────────────────────────────────────────────────────────────
phase_lxd() {
    phase "5/16 — LXD / INCUS SYSTEM CONTAINERS"

    # Prefer incus on newer Ubuntu, fallback to lxd
    if command -v incus &>/dev/null; then
        ok "Incus already installed"
    elif command -v lxd &>/dev/null; then
        ok "LXD already installed"
    else
        step "Installing LXD via snap..."
        if command -v snap &>/dev/null; then
            run snap install lxd --channel=latest/stable 2>/dev/null || {
                step "Snap LXD failed, trying apt..."
                run apt-get install -y lxd lxd-client 2>/dev/null || warn "LXD install failed"
            }
        else
            run apt-get install -y snapd
            run snap install lxd --channel=latest/stable 2>/dev/null || {
                run apt-get install -y lxd lxd-client 2>/dev/null || warn "LXD install failed"
            }
        fi
    fi

    # Initialize LXD if not already
    if command -v lxd &>/dev/null || command -v incus &>/dev/null; then
        local lxd_cmd="lxd"
        local lxc_cmd="lxc"
        command -v incus &>/dev/null && { lxd_cmd="incus"; lxc_cmd="incus"; }

        # Bridge name: incus uses incusbr0, LXD uses lxdbr0
        local br_name="lxdbr0"
        [[ "$lxd_cmd" == "incus" ]] && br_name="incusbr0"

        # Check what's already configured
        local has_network=false has_pool=false has_profile=false
        $lxc_cmd network list 2>/dev/null | grep -q "${br_name}" && has_network=true
        $lxc_cmd storage list 2>/dev/null | grep -q "default" && has_pool=true
        $lxc_cmd profile show default 2>/dev/null | grep -q "pool:" && has_profile=true

        if $has_network && $has_pool && $has_profile; then
            ok "LXD/Incus already initialized"
        else
            step "Initializing LXD..."

            # Clean up stale storage pool dir from failed runs (LXD dir driver
            # refuses to create a pool if the target directory already exists)
            local pool_base=""
            if [[ "$lxd_cmd" == "incus" ]]; then
                pool_base="/var/lib/incus/storage-pools/default"
            elif snap list lxd &>/dev/null 2>&1; then
                pool_base="/var/snap/lxd/common/lxd/storage-pools/default"
            else
                pool_base="/var/lib/lxd/storage-pools/default"
            fi
            # Only nuke the dir if the pool isn't registered (stale leftover)
            if ! $has_pool && [[ -d "$pool_base" ]]; then
                step "Removing stale storage pool directory from previous failed run..."
                rm -rf "$pool_base"
            fi

            # Preseed — omit source: so LXD auto-provisions the directory
            cat <<LXDPRESEED | run $lxd_cmd init --preseed
config:
  core.https_address: '[::]:8443'
networks:
- name: ${br_name}
  type: bridge
  config:
    ipv4.address: 10.10.10.1/24
    ipv4.nat: "true"
    ipv6.address: fd42:474b:622d:259d::1/64
    ipv6.nat: "true"
    dns.mode: managed
storage_pools:
- name: default
  driver: dir
profiles:
- name: default
  devices:
    eth0:
      name: eth0
      network: ${br_name}
      type: nic
    root:
      path: /
      pool: default
      type: disk
      size: 50GB
LXDPRESEED
            ok "LXD initialized"
        fi

        # Add users to lxd group
        [[ -n "${SUDO_USER:-}" ]] && run usermod -aG lxd "${SUDO_USER}" 2>/dev/null || true
    fi

    ok "Container runtime ready"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 6: GO + RUST + NIX (Build Environment)
# ─────────────────────────────────────────────────────────────────────────────
phase_dev_tools() {
    phase "6/16 — DEV ENVIRONMENT (Go, Rust, Nix, Node)"

    # ── Go ──
    step "Installing Go ${GO_VERSION}..."
    if command -v go &>/dev/null && go version 2>/dev/null | grep -q "${GO_VERSION}"; then
        ok "Go ${GO_VERSION} already installed"
    else
        local go_tar="go${GO_VERSION}.linux-amd64.tar.gz"
        run wget -q "https://go.dev/dl/${go_tar}" -O "/tmp/${go_tar}"
        run rm -rf /usr/local/go
        run tar -C /usr/local -xzf "/tmp/${go_tar}"
        rm -f "/tmp/${go_tar}"

        cat > /etc/profile.d/golang.sh << 'GOENV'
export GOROOT=/usr/local/go
export GOPATH=$HOME/go
export PATH=$GOROOT/bin:$GOPATH/bin:$PATH
GOENV
        chmod 644 /etc/profile.d/golang.sh
        export PATH=/usr/local/go/bin:$PATH
        ok "Go ${GO_VERSION} installed"
    fi

    # ── Rust ──
    step "Installing Rust (${RUST_TOOLCHAIN})..."
    if command -v rustup &>/dev/null; then
        ok "Rustup already installed"
        run rustup update "${RUST_TOOLCHAIN}" 2>/dev/null || true
    else
        # Install for root
        run curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain "${RUST_TOOLCHAIN}"
        # shellcheck source=/dev/null
        source "$HOME/.cargo/env" 2>/dev/null || true

        # Install for sudo user too
        if [[ -n "${SUDO_USER:-}" ]]; then
            sudo -u "${SUDO_USER}" bash -c \
                'curl --proto "=https" --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain nightly' \
                2>/dev/null || warn "Rust install for ${SUDO_USER} failed — install manually"
        fi
    fi

    # Rust targets for eBPF
    step "Adding Rust eBPF targets..."
    if command -v rustup &>/dev/null; then
        run rustup target add x86_64-unknown-linux-musl 2>/dev/null || true
        run rustup component add rust-src --toolchain "${RUST_TOOLCHAIN}" 2>/dev/null || true
        # Install bpf-linker + cargo-generate for Aya eBPF
        run cargo install bpf-linker 2>/dev/null || warn "bpf-linker install failed — needed for eBPF Rust"
        run cargo install cargo-generate 2>/dev/null || true
    fi

    # ── Nix ──
    step "Installing Nix package manager..."
    if command -v nix &>/dev/null; then
        ok "Nix already installed"
    else
        # Nix multi-user install
        run bash -c 'curl -L https://nixos.org/nix/install | sh -s -- --daemon --yes' || {
            warn "Nix daemon install failed — trying single-user..."
            run bash -c 'curl -L https://nixos.org/nix/install | sh -s -- --no-daemon' || warn "Nix install failed"
        }

        # Enable flakes
        mkdir -p /etc/nix
        cat > /etc/nix/nix.conf << 'NIXCFG'
experimental-features = nix-command flakes
max-jobs = auto
cores = 0
sandbox = true
trusted-users = root @wheel @sudo
NIXCFG
    fi

    # ── Node.js (for dashboard) ──
    step "Installing Node.js ${NODE_VERSION}..."
    if command -v node &>/dev/null; then
        ok "Node.js already installed: $(node --version)"
    else
        run curl -fsSL "https://deb.nodesource.com/setup_${NODE_VERSION}.x" | bash -
        run apt-get install -y nodejs
    fi

    # ── golangci-lint ──
    step "Installing golangci-lint..."
    if command -v golangci-lint &>/dev/null; then
        ok "golangci-lint already installed"
    else
        run curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b /usr/local/bin latest 2>/dev/null || true
    fi

    ok "Dev environment ready"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 7: eBPF ENVIRONMENT
# ─────────────────────────────────────────────────────────────────────────────
phase_ebpf() {
    phase "7/16 — eBPF ENVIRONMENT"

    # Mount bpffs
    step "Mounting BPF filesystem..."
    if mountpoint -q /sys/fs/bpf 2>/dev/null; then
        ok "bpffs already mounted"
    else
        run mount -t bpf bpf /sys/fs/bpf 2>/dev/null || warn "bpffs mount failed — may need reboot"
        if ! grep -q "/sys/fs/bpf" /etc/fstab; then
            echo "bpf /sys/fs/bpf bpf defaults 0 0" >> /etc/fstab
        fi
    fi

    # Mount debugfs (needed for tracing)
    if ! mountpoint -q /sys/kernel/debug 2>/dev/null; then
        run mount -t debugfs debugfs /sys/kernel/debug 2>/dev/null || true
    fi

    # Verify BTF support
    step "Checking BTF support..."
    if [[ -f /sys/kernel/btf/vmlinux ]]; then
        ok "BTF: available (/sys/kernel/btf/vmlinux)"
    else
        warn "BTF not available — CO-RE eBPF programs may fail"
        warn "Install: apt install linux-image-$(uname -r)-dbgsym"
    fi

    # Increase locked memory limits
    step "Setting memlock limits..."
    cat > /etc/security/limits.d/99-ebpf.conf << 'EBPFLIMITS'
# Unheaded eBPF — unlimited locked memory
*    soft    memlock    unlimited
*    hard    memlock    unlimited
root soft    memlock    unlimited
root hard    memlock    unlimited
EBPFLIMITS

    # systemd override for memlock
    mkdir -p /etc/systemd/system.conf.d
    cat > /etc/systemd/system.conf.d/99-memlock.conf << 'SYSDMEM'
[Manager]
DefaultLimitMEMLOCK=infinity
SYSDMEM

    ok "eBPF environment configured"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 8: KERNEL TUNING + SYSCTL
# ─────────────────────────────────────────────────────────────────────────────
phase_kernel_tune() {
    phase "8/16 — KERNEL TUNING + SYSCTL"

    step "Writing sysctl parameters..."
    cat > /etc/sysctl.d/99-unheaded.conf << 'SYSCTL'
# ═══════════════════════════════════════════════════════════════════
# UNHEADED KINGDOM — KERNEL TUNING
# Target: LLM Lab Server (AMD GPU, heavy inference workloads)
# ═══════════════════════════════════════════════════════════════════

# ── NETWORK: IPv6-first, forwarding, performance ──
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
net.ipv6.conf.default.forwarding = 1

# TCP performance
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.core.rmem_default = 16777216
net.core.wmem_default = 16777216
net.ipv4.tcp_rmem = 4096 87380 134217728
net.ipv4.tcp_wmem = 4096 87380 134217728
net.core.netdev_max_backlog = 50000
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_mtu_probing = 1

# TCP keepalive (faster dead connection detection)
net.ipv4.tcp_keepalive_time = 60
net.ipv4.tcp_keepalive_intvl = 10
net.ipv4.tcp_keepalive_probes = 6

# Connection tracking for containers
net.netfilter.nf_conntrack_max = 1048576
net.nf_conntrack_max = 1048576

# BBR congestion control
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr

# ── SECURITY: hardening ──
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.tcp_syncookies = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv6.conf.all.accept_source_route = 0
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.icmp_ignore_bogus_error_responses = 1

# ── MEMORY: LLM workload optimized ──
vm.swappiness = 10
vm.dirty_ratio = 40
vm.dirty_background_ratio = 10
vm.overcommit_memory = 1
vm.max_map_count = 1048576

# Huge pages for GPU/model loading
vm.nr_hugepages = 1024

# ── FILE DESCRIPTORS ──
fs.file-max = 2097152
fs.inotify.max_user_watches = 1048576
fs.inotify.max_user_instances = 8192

# ── eBPF ──
kernel.unprivileged_bpf_disabled = 0
net.core.bpf_jit_enable = 1
net.core.bpf_jit_harden = 0

# ── AIO (for direct I/O workloads) ──
fs.aio-max-nr = 1048576

# ── VXLAN / tunneling ──
net.ipv4.ip_forward = 1
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
SYSCTL

    step "Loading kernel modules..."
    local modules=(
        br_netfilter
        overlay
        vxlan
        wireguard
        ip6_tables
        ip6table_filter
        nf_conntrack
        xt_conntrack
    )
    for mod in "${modules[@]}"; do
        modprobe "$mod" 2>/dev/null || warn "Module ${mod} failed to load"
    done

    # Persist modules
    printf '%s\n' "${modules[@]}" > /etc/modules-load.d/unheaded.conf

    step "Applying sysctl..."
    run sysctl --system 2>/dev/null || run sysctl -p /etc/sysctl.d/99-unheaded.conf

    # Resource limits
    step "Setting resource limits..."
    cat > /etc/security/limits.d/99-unheaded.conf << 'LIMITS'
# Unheaded Kingdom — resource limits
*    soft    nofile    1048576
*    hard    nofile    1048576
*    soft    nproc     unlimited
*    hard    nproc     unlimited
root soft    nofile    1048576
root hard    nofile    1048576
LIMITS

    ok "Kernel tuned for LLM workloads"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 9: NETWORKING STACK
# ─────────────────────────────────────────────────────────────────────────────
phase_networking() {
    phase "9/16 — NETWORKING STACK"

    # FRR (BGP/IS-IS/OSPF)
    step "Installing FRR routing suite..."
    if command -v vtysh &>/dev/null; then
        ok "FRR already installed"
    else
        # Add FRR repo
        run curl -s https://deb.frrouting.org/frr/keys.gpg | tee /usr/share/keyrings/frrouting.gpg > /dev/null 2>&1 || true
        local frr_codename
        frr_codename=$(lsb_release -cs)
        # FRR may not have 25.x — fallback
        echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr ${frr_codename} frr-stable" \
            > /etc/apt/sources.list.d/frr.list 2>/dev/null || true
        run apt-get update -y 2>/dev/null || true
        run apt-get install -y frr frr-pythontools 2>/dev/null || {
            warn "FRR from repo failed — trying Ubuntu package..."
            run apt-get install -y frr 2>/dev/null || warn "FRR install failed — install manually"
        }

        # Enable required daemons
        if [[ -f /etc/frr/daemons ]]; then
            step "Enabling FRR daemons (bgpd, isisd, ospfd, vtysh)..."
            sed -i 's/^bgpd=no/bgpd=yes/' /etc/frr/daemons
            sed -i 's/^isisd=no/isisd=yes/' /etc/frr/daemons
            sed -i 's/^ospfd=no/ospfd=yes/' /etc/frr/daemons
            sed -i 's/^ospf6d=no/ospf6d=yes/' /etc/frr/daemons
            sed -i 's/^bfdd=no/bfdd=yes/' /etc/frr/daemons
            run systemctl enable frr 2>/dev/null || true
        fi
    fi

    # HAProxy (internal proxy)
    step "Installing HAProxy..."
    run apt-get install -y haproxy 2>/dev/null || warn "HAProxy install failed"

    # Nginx (backend reverse proxy)
    step "Installing Nginx..."
    run apt-get install -y nginx 2>/dev/null || warn "Nginx install failed"

    # Suricata (IDS/IPS)
    step "Installing Suricata..."
    run apt-get install -y suricata 2>/dev/null || warn "Suricata install failed"
    run systemctl disable suricata 2>/dev/null || true  # Don't start yet

    ok "Networking stack installed"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 10: OBSERVABILITY + MONITORING
# ─────────────────────────────────────────────────────────────────────────────
phase_observability() {
    phase "10/16 — OBSERVABILITY + MONITORING STACK"

    step "Pulling monitoring containers..."
    local images=(
        "docker.io/prom/prometheus:latest"
        "docker.io/grafana/grafana:latest"
        "docker.io/grafana/loki:latest"
        "docker.io/grafana/promtail:latest"
        "docker.io/prom/alertmanager:latest"
        "docker.io/victoriametrics/victoria-metrics:latest"
        "docker.io/grafana/alloy:latest"
    )

    for img in "${images[@]}"; do
        step "Pulling ${img}..."
        run docker pull "${img}" 2>/dev/null || warn "Failed to pull ${img}"
    done

    ok "Monitoring images pulled"
    info "Deploy with: cd monitoring && docker compose up -d"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 11: vLLM + ROCm INFERENCE SERVER
# ─────────────────────────────────────────────────────────────────────────────
phase_vllm() {
    phase "11/16 — vLLM ROCm INFERENCE SERVER"

    # ── Official AMD vLLM ROCm image (pre-tuned for GFX architectures) ──
    step "Pulling official AMD vLLM ROCm image (pre-validated, no build needed)..."
    run docker pull rocm/vllm:latest 2>/dev/null || {
        warn "Official rocm/vllm:latest pull failed — trying ROCm PyTorch base as fallback..."
        run docker pull "rocm/pytorch:6.1.3-ubuntu22.04" 2>/dev/null || {
            warn "Fallback also failed — try manually:"
            warn "  docker pull rocm/vllm:latest"
        }
    }

    # Also pull custom Dockerfile base if repo has one
    local vllm_dir="${UNHEADED_HOME}/docker/vllm-rocm"
    if [[ -f "${vllm_dir}/Dockerfile" ]]; then
        info "Custom Dockerfile also available at ${vllm_dir}"
        info "Build custom: cd ${vllm_dir} && docker compose -f docker-compose.vllm.yml build"
    fi

    # ── Install hf_transfer (Rust-based parallel downloader — 10x faster) ──
    step "Installing hf_transfer (Rust parallel HuggingFace downloads)..."
    run pip3 install --break-system-packages hf_transfer 2>/dev/null || warn "hf_transfer install failed"

    # ── Install huggingface-cli ──
    step "Installing huggingface-cli..."
    run pip3 install --break-system-packages huggingface-hub 2>/dev/null || true

    # ── Model download helper (enhanced with hf_transfer + storage strategy) ──
    step "Creating model download helper..."
    cat > /usr/local/bin/unheaded-download-model << 'MODELDL'
#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════
# UNHEADED — LLM Model Downloader
# Downloads to HDD archive, optionally symlinks to NVMe active slot
# Uses hf_transfer for 10x faster parallel downloads
# ═══════════════════════════════════════════════════════════
set -euo pipefail

MODELS_DIR="${HF_HOME:-/mnt/hdd/models}"
ACTIVE_SLOT="/opt/unheaded/active-model/current"

# Enable Rust parallel downloader
export HF_HUB_ENABLE_HF_TRANSFER=1

usage() {
    echo "Usage: $0 <huggingface-repo> [output-name] [--activate]"
    echo ""
    echo "Examples:"
    echo "  $0 deepseek-ai/DeepSeek-R1-Distill-Qwen-7B"
    echo "  $0 deepseek-ai/DeepSeek-R1-Distill-Qwen-7B deepseek-r1-7b --activate"
    echo ""
    echo "Flags:"
    echo "  --activate   Symlink model to NVMe active slot for fast loading"
    echo ""
    echo "Popular models for RX 7700 XT (12GB VRAM):"
    echo "  deepseek-ai/DeepSeek-R1-Distill-Qwen-7B     (~7B, fits in VRAM)"
    echo "  mistralai/Mistral-7B-Instruct-v0.3           (~7B, fast inference)"
    echo "  meta-llama/Meta-Llama-3.1-8B-Instruct        (~8B, great quality)"
    echo "  Qwen/Qwen2.5-7B-Instruct                    (~7B, multilingual)"
    echo "  google/gemma-2-9b-it                         (~9B, tight fit)"
    echo ""
    echo "Storage strategy:"
    echo "  Archive (HDD): ${MODELS_DIR}"
    echo "  Active (NVMe): ${ACTIVE_SLOT}"
    exit 1
}

[[ $# -lt 1 ]] && usage

REPO="$1"
NAME="${2:-$(basename "$REPO")}"
ACTIVATE=false

# Parse --activate flag
for arg in "$@"; do
    [[ "$arg" == "--activate" ]] && ACTIVATE=true
done

echo "Downloading ${REPO} → ${MODELS_DIR}/${NAME}"
echo "Using hf_transfer for parallel downloads..."

if command -v huggingface-cli &>/dev/null; then
    huggingface-cli download "$REPO" --local-dir "${MODELS_DIR}/${NAME}"
elif command -v git &>/dev/null; then
    GIT_LFS_SKIP_SMUDGE=0 git clone "https://huggingface.co/${REPO}" "${MODELS_DIR}/${NAME}"
else
    echo "ERROR: Need huggingface-cli or git-lfs"
    echo "  pip install huggingface-hub hf_transfer"
    exit 1
fi

echo ""
echo "✓ Model downloaded to: ${MODELS_DIR}/${NAME}"

if $ACTIVATE; then
    ln -sfn "${MODELS_DIR}/${NAME}" "${ACTIVE_SLOT}"
    echo "✓ Activated: ${ACTIVE_SLOT} → ${MODELS_DIR}/${NAME}"
    echo "  vLLM flag: --model ${ACTIVE_SLOT}"
else
    echo "  Activate for NVMe speed: $0 $REPO $NAME --activate"
fi

echo ""
echo "Quick start:"
echo "  # Official AMD image (recommended):"
echo "  docker run --device=/dev/kfd --device=/dev/dri --group-add video \\"
echo "    -v ${MODELS_DIR}:/models -p 20100:20100 rocm/vllm:latest \\"
echo "    --model /models/${NAME} --port 20100 --dtype half"
echo ""
echo "  # Custom Unheaded image:"
echo "  cd docker/vllm-rocm && docker compose -f docker-compose.vllm.yml up -d"
MODELDL
    chmod +x /usr/local/bin/unheaded-download-model

    # ── Quick-launch helper for official AMD vLLM ──
    step "Creating vLLM quick-launch helper..."
    cat > /usr/local/bin/unheaded-vllm-start << 'VLLMSTART'
#!/usr/bin/env bash
# Quick-launch vLLM with official AMD ROCm image
set -euo pipefail
MODEL="${1:-/models/deepseek-r1-7b}"
PORT="${2:-20100}"

echo "Starting vLLM on port ${PORT} with model ${MODEL}..."
docker run -d \
    --name unheaded-vllm \
    --restart unless-stopped \
    --device=/dev/kfd --device=/dev/dri \
    --group-add video --group-add render \
    --shm-size=4g \
    -e HSA_OVERRIDE_GFX_VERSION=11.0.1 \
    -e PYTORCH_ROCM_ARCH=gfx1101 \
    -e VLLM_USE_AITER=1 \
    -v /mnt/hdd/models:/models:ro \
    -p "${PORT}:${PORT}" \
    --network unheaded_unheaded \
    rocm/vllm:latest \
    --model "${MODEL}" \
    --port "${PORT}" \
    --host 0.0.0.0 \
    --dtype half \
    --gpu-memory-utilization 0.90 \
    --max-model-len 8192 \
    --tensor-parallel-size 1

echo "vLLM starting on port ${PORT}..."
echo "  Health:  curl http://localhost:${PORT}/health"
echo "  Models:  curl http://localhost:${PORT}/v1/models"
echo "  Logs:    docker logs -f unheaded-vllm"
VLLMSTART
    chmod +x /usr/local/bin/unheaded-vllm-start

    ok "vLLM infrastructure ready"
    info "Download a model:   unheaded-download-model deepseek-ai/DeepSeek-R1-Distill-Qwen-7B deepseek-r1-7b --activate"
    info "Start inference:    unheaded-vllm-start /models/deepseek-r1-7b"
    info "Or custom build:    cd docker/vllm-rocm && docker compose -f docker-compose.vllm.yml up -d"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 12: CLAUDE CODE CLI + AI TOOLING (npm: @anthropic-ai/claude-code)
# ─────────────────────────────────────────────────────────────────────────────
phase_claude_cli() {
    phase "12/16 — CLAUDE CODE CLI + AI TOOLING"

    # ── Claude Code CLI ──
    step "Installing Claude Code CLI..."
    if command -v claude &>/dev/null; then
        ok "Claude Code already installed: $(claude --version 2>/dev/null || echo 'available')"
    else
        # Claude Code installs via npm
        if command -v npm &>/dev/null; then
            run npm install -g @anthropic-ai/claude-code 2>/dev/null || {
                warn "npm install failed — trying npx..."
                info "You can run: npx @anthropic-ai/claude-code"
            }
        else
            warn "npm not available — install Node.js first, then:"
            warn "  npm install -g @anthropic-ai/claude-code"
        fi

        # Also install for sudo user
        if [[ -n "${SUDO_USER:-}" ]] && command -v npm &>/dev/null; then
            sudo -u "${SUDO_USER}" npm install -g @anthropic-ai/claude-code 2>/dev/null || true
        fi
    fi

    # ── Ollama (quick local inference) ──
    step "Installing Ollama..."
    if command -v ollama &>/dev/null; then
        ok "Ollama already installed"
    else
        run curl -fsSL https://ollama.com/install.sh | sh 2>/dev/null || warn "Ollama install failed — install manually"
        # Don't auto-start — user decides when to use it
        run systemctl disable ollama 2>/dev/null || true
    fi

    # ── Python ML stack ──
    step "Installing Python ML stack (PyTorch ROCm, transformers)..."
    run pip3 install --break-system-packages \
        torch torchvision torchaudio \
        --index-url https://download.pytorch.org/whl/rocm6.1 \
        2>/dev/null || warn "PyTorch ROCm install failed — install manually"

    run pip3 install --break-system-packages \
        transformers \
        datasets \
        accelerate \
        safetensors \
        sentencepiece \
        protobuf \
        tokenizers \
        bitsandbytes \
        peft \
        trl \
        wandb \
        tensorboard \
        gradio \
        2>/dev/null || warn "Some Python ML packages failed — non-fatal"

    ok "Claude Code + AI tooling installed"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 13: SECRETS + CERTS (SOPS + age + mTLS)
# ─────────────────────────────────────────────────────────────────────────────
phase_secrets() {
    phase "13/16 — SECRETS + CERTIFICATES"

    # ── SOPS ──
    step "Installing SOPS..."
    if command -v sops &>/dev/null; then
        ok "SOPS already installed"
    else
        local sops_ver="3.9.4"
        run wget -q "https://github.com/getsops/sops/releases/download/v${sops_ver}/sops-v${sops_ver}.linux.amd64" \
            -O /usr/local/bin/sops 2>/dev/null || warn "SOPS download failed"
        chmod +x /usr/local/bin/sops 2>/dev/null || true
    fi

    # ── age (encryption) ──
    step "Installing age..."
    if command -v age &>/dev/null; then
        ok "age already installed"
    else
        run apt-get install -y age 2>/dev/null || {
            local age_ver="1.2.1"
            run wget -q "https://github.com/FiloSottile/age/releases/download/v${age_ver}/age-v${age_ver}-linux-amd64.tar.gz" \
                -O /tmp/age.tar.gz 2>/dev/null || warn "age download failed"
            if [[ -f /tmp/age.tar.gz ]]; then
                tar -xzf /tmp/age.tar.gz -C /tmp/
                cp /tmp/age/age /tmp/age/age-keygen /usr/local/bin/
                rm -rf /tmp/age /tmp/age.tar.gz
            fi
        }
    fi

    # ── Generate initial age keypair ──
    step "Generating age keypair for secrets..."
    local age_key_dir="/etc/unheaded/keys"
    mkdir -p "${age_key_dir}"
    chmod 700 "${age_key_dir}"
    if [[ ! -f "${age_key_dir}/age.key" ]]; then
        if command -v age-keygen &>/dev/null; then
            age-keygen -o "${age_key_dir}/age.key" 2>/dev/null
            chmod 600 "${age_key_dir}/age.key"
            ok "age keypair generated at ${age_key_dir}/age.key"
            info "Public key: $(grep 'public key' "${age_key_dir}/age.key" | awk '{print $NF}')"
        fi
    else
        ok "age keypair already exists"
    fi

    # ── SOPS config ──
    step "Writing SOPS config..."
    if [[ -f "${age_key_dir}/age.key" ]]; then
        local age_pub
        age_pub=$(grep 'public key' "${age_key_dir}/age.key" | awk '{print $NF}')
        cat > "${UNHEADED_HOME}/.sops.yaml" << SOPSCFG
# Unheaded SOPS configuration
creation_rules:
  - path_regex: \.enc\.yaml$
    age: >-
      ${age_pub}
  - path_regex: secrets/.*
    age: >-
      ${age_pub}
SOPSCFG
        # Export for SOPS to find the key
        cat > /etc/profile.d/sops.sh << SOPSENV
export SOPS_AGE_KEY_FILE=${age_key_dir}/age.key
SOPSENV
        chmod 644 /etc/profile.d/sops.sh
    fi

    # ── step-ca (mTLS certificate authority) ──
    step "Installing step-ca (Smallstep CA) for mTLS..."
    if command -v step &>/dev/null; then
        ok "step CLI already installed"
    else
        run wget -q "https://dl.smallstep.com/gh-release/cli/docs-cli-install/v0.27.5/step-cli_0.27.5_amd64.deb" \
            -O /tmp/step-cli.deb 2>/dev/null && {
            run dpkg -i /tmp/step-cli.deb 2>/dev/null || run apt-get install -f -y
            rm -f /tmp/step-cli.deb
        } || warn "step-cli install failed"
    fi

    if command -v step-ca &>/dev/null; then
        ok "step-ca already installed"
    else
        run wget -q "https://dl.smallstep.com/gh-release/certificates/docs-ca-install/v0.27.5/step-ca_0.27.5_amd64.deb" \
            -O /tmp/step-ca.deb 2>/dev/null && {
            run dpkg -i /tmp/step-ca.deb 2>/dev/null || run apt-get install -f -y
            rm -f /tmp/step-ca.deb
        } || warn "step-ca install failed — mTLS CA setup deferred"
    fi

    ok "Secrets + cert infrastructure ready"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 14: LOG PIPELINE (Vector + ClickHouse)
# ─────────────────────────────────────────────────────────────────────────────
phase_log_pipeline() {
    phase "14/16 — LOG PIPELINE (Vector + ClickHouse)"

    # ── Vector (log shipper) ──
    step "Installing Vector..."
    if command -v vector &>/dev/null; then
        ok "Vector already installed"
    else
        run bash -c 'curl --proto "=https" --tlsv1.2 -sSfL https://sh.vector.dev | bash -s -- -y' 2>/dev/null || {
            warn "Vector install script failed — trying apt..."
            run bash -c 'curl -1sLf https://repositories.timber.io/public/vector/cfg/setup/bash.deb.sh | bash' 2>/dev/null || true
            run apt-get install -y vector 2>/dev/null || warn "Vector install failed"
        }
    fi

    # Vector base config
    if command -v vector &>/dev/null; then
        step "Writing Vector base config..."
        mkdir -p /etc/vector
        cat > /etc/vector/vector.yaml << 'VECTORCFG'
# Unheaded Kingdom — Vector log pipeline
# Collects: journald, Docker, files → Loki + ClickHouse

sources:
  journald:
    type: journald
    current_boot_only: true
  docker_logs:
    type: docker_logs
  unheaded_logs:
    type: file
    include:
      - /var/log/unheaded/*.log
      - /opt/unheaded/logs/*.log

transforms:
  parse_logs:
    type: remap
    inputs: ["journald", "docker_logs", "unheaded_logs"]
    source: |
      .kingdom = "unheaded"
      .host = get_hostname!()
      .timestamp = now()

sinks:
  loki:
    type: loki
    inputs: ["parse_logs"]
    endpoint: "http://localhost:3100"
    labels:
      kingdom: "{{ kingdom }}"
      host: "{{ host }}"
    encoding:
      codec: json
  console_debug:
    type: console
    inputs: ["parse_logs"]
    encoding:
      codec: text
    # Disable in production:
    # healthcheck: false
VECTORCFG
        run systemctl enable vector 2>/dev/null || true
    fi

    # ── ClickHouse (long-term log storage) ──
    step "Pulling ClickHouse container..."
    run docker pull clickhouse/clickhouse-server:latest 2>/dev/null || warn "ClickHouse pull failed"

    ok "Log pipeline configured"
    info "Start Vector:     systemctl start vector"
    info "Start ClickHouse: docker run -d --name clickhouse -p 8123:8123 -p 9000:9000 clickhouse/clickhouse-server"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 15: SHELL + DOTFILES + DEVELOPER COMFORT
# ─────────────────────────────────────────────────────────────────────────────
phase_shell_comfort() {
    phase "15/16 — SHELL + DOTFILES + DEVELOPER COMFORT"

    local target_user="${SUDO_USER:-root}"
    local target_home
    target_home=$(eval echo "~${target_user}")

    # ── Git LFS ──
    step "Configuring Git LFS..."
    if command -v git-lfs &>/dev/null; then
        ok "Git LFS available"
    else
        run apt-get install -y git-lfs 2>/dev/null || true
    fi
    run git lfs install --system 2>/dev/null || true

    # ── tmux config ──
    step "Writing tmux config..."
    cat > "${target_home}/.tmux.conf" << 'TMUXCFG'
# Unheaded Kingdom — tmux config
set -g default-terminal "screen-256color"
set -g history-limit 50000
set -g mouse on
set -g status-interval 5

# Status bar
set -g status-style bg=colour235,fg=colour136
set -g status-left "#[fg=green]#S #[fg=yellow]| "
set -g status-right "#[fg=cyan]%H:%M #[fg=yellow]| #[fg=green]#H"
set -g status-left-length 40

# Pane splitting
bind | split-window -h -c "#{pane_current_path}"
bind - split-window -v -c "#{pane_current_path}"

# Vi mode
setw -g mode-keys vi
bind -T copy-mode-vi v send -X begin-selection
bind -T copy-mode-vi y send -X copy-selection-and-cancel

# Fast pane switching
bind h select-pane -L
bind j select-pane -D
bind k select-pane -U
bind l select-pane -R

# Reload
bind r source-file ~/.tmux.conf \; display "Reloaded!"
TMUXCFG
    chown "${target_user}:${target_user}" "${target_home}/.tmux.conf" 2>/dev/null || true

    # ── Git config helpers ──
    step "Setting git defaults..."
    if [[ -n "${SUDO_USER:-}" ]]; then
        sudo -u "${SUDO_USER}" git config --global init.defaultBranch main 2>/dev/null || true
        sudo -u "${SUDO_USER}" git config --global pull.rebase true 2>/dev/null || true
        sudo -u "${SUDO_USER}" git config --global fetch.prune true 2>/dev/null || true
        sudo -u "${SUDO_USER}" git config --global diff.colorMoved zebra 2>/dev/null || true
        sudo -u "${SUDO_USER}" git config --global core.editor "nvim" 2>/dev/null || true
        sudo -u "${SUDO_USER}" git config --global rerere.enabled true 2>/dev/null || true
    fi

    # ── Useful aliases ──
    step "Writing shell aliases..."
    cat > /etc/profile.d/unheaded-aliases.sh << 'ALIASES'
# Unheaded Kingdom — shell aliases
alias k='kubectl'
alias d='docker'
alias dc='docker compose'
alias g='git'
alias gs='git status'
alias gl='git log --oneline -20'
alias gd='git diff'
alias ll='ls -alF'
alias la='ls -A'
alias ..='cd ..'
alias ...='cd ../..'

# Unheaded specific
alias uh='cd /opt/unheaded'
alias uhlog='tail -f /var/log/unheaded/*.log'
alias uhps='docker compose ps'
alias uhup='docker compose up -d'
alias uhdown='docker compose down'
alias rocm-check='rocminfo | grep -i "gfx\|agent\|name"'
alias gpu-mon='watch -n1 rocm-smi'
alias bpf-progs='bpftool prog list'
alias bpf-maps='bpftool map list'
alias ports='ss -tlnp'
alias flames='perf record -g -F 99 -p'

# vLLM
alias vllm-health='curl -s http://localhost:20100/health | jq'
alias vllm-models='curl -s http://localhost:20100/v1/models | jq'
alias vllm-chat='curl -s http://localhost:20100/v1/chat/completions -H "Content-Type: application/json" -d'
alias vllm-start='unheaded-vllm-start'
alias vllm-stop='docker stop unheaded-vllm && docker rm unheaded-vllm'
alias vllm-logs='docker logs -f unheaded-vllm'

# Model management
alias model-dl='unheaded-download-model'
alias model-ls='ls -lhS /mnt/hdd/models/'
alias model-active='readlink -f /opt/unheaded/active-model/current'
alias runway='python3 /opt/unheaded/scripts/vault-to-runway.py'
alias runway-list='python3 /opt/unheaded/scripts/vault-to-runway.py --list'
alias runway-go='python3 /opt/unheaded/scripts/vault-to-runway.py --launch'
ALIASES
    chmod 644 /etc/profile.d/unheaded-aliases.sh

    # ── htop/btop config ──
    step "Setting up monitoring shortcuts..."
    mkdir -p "${target_home}/.config/btop"

    ok "Shell comfort configured"
}

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 16: SECURITY HARDENING + SERVICES
# ─────────────────────────────────────────────────────────────────────────────
phase_security() {
    phase "16/16 — SECURITY HARDENING + SYSTEM SERVICES"

    # Create unheaded service user
    step "Creating unheaded service user..."
    if id "${UNHEADED_USER}" &>/dev/null; then
        ok "User ${UNHEADED_USER} already exists"
    else
        run useradd -r -s /bin/bash -d "${UNHEADED_HOME}" -m "${UNHEADED_USER}"
        for grp in docker lxd video render; do
            getent group "$grp" &>/dev/null && run usermod -aG "$grp" "${UNHEADED_USER}" 2>/dev/null || true
        done
        ok "User ${UNHEADED_USER} created"
    fi

    # Ownership
    run chown -R "${UNHEADED_USER}:${UNHEADED_USER}" "${UNHEADED_HOME}" 2>/dev/null || true
    run chown -R "${UNHEADED_USER}:${UNHEADED_USER}" /var/lib/unheaded 2>/dev/null || true
    run chown -R "${UNHEADED_USER}:${UNHEADED_USER}" /var/log/unheaded 2>/dev/null || true

    # UFW firewall
    step "Configuring UFW firewall..."
    if command -v ufw &>/dev/null; then
        run ufw default deny incoming
        run ufw default allow outgoing
        run ufw allow ssh
        # Unheaded ports (Doom Range: 16666-26666)
        run ufw allow 16666:26666/tcp comment "Unheaded Doom Range"
        # Monitoring
        run ufw allow 3000/tcp comment "Grafana"
        run ufw allow 9090/tcp comment "Prometheus"
        run ufw allow 9100/tcp comment "Node Exporter"
        # LXD
        run ufw allow 8443/tcp comment "LXD API"
        # BGP
        run ufw allow 179/tcp comment "BGP"
        run ufw --force enable 2>/dev/null || warn "UFW enable failed"
    fi

    # Fail2ban
    step "Configuring fail2ban..."
    if command -v fail2ban-client &>/dev/null; then
        cat > /etc/fail2ban/jail.local << 'F2B'
[DEFAULT]
bantime = 3600
findtime = 600
maxretry = 5
backend = systemd

[sshd]
enabled = true
port = ssh
logpath = %(sshd_log)s
F2B
        run systemctl enable fail2ban
        run systemctl restart fail2ban 2>/dev/null || true
    fi

    # SSH hardening
    step "Hardening SSH..."
    local sshd_conf="/etc/ssh/sshd_config.d/99-unheaded.conf"
    cat > "${sshd_conf}" << 'SSHCFG'
# Unheaded SSH hardening
PermitRootLogin prohibit-password
PasswordAuthentication no
PubkeyAuthentication yes
X11Forwarding no
MaxAuthTries 3
ClientAliveInterval 300
ClientAliveCountMax 2
AllowAgentForwarding yes
AllowTcpForwarding yes
SSHCFG
    run systemctl reload sshd 2>/dev/null || run systemctl reload ssh 2>/dev/null || true

    # Systemd service for unheaded daemon
    step "Installing unheaded systemd service..."
    cat > /etc/systemd/system/unheaded-daemon.service << 'SVCUNIT'
[Unit]
Description=Unheaded Control Plane Daemon (Cuirass)
Documentation=https://github.com/unheaded/unheaded
After=network-online.target docker.service lxd.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=root
Group=root
ExecStart=/opt/unheaded/bin/unheaded-daemon
Restart=on-failure
RestartSec=5s
WatchdogSec=30s

# Resource limits
LimitNOFILE=1048576
LimitMEMLOCK=infinity
LimitNPROC=infinity

# Security hardening
CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_BPF CAP_PERFMON CAP_NET_RAW CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_BPF CAP_PERFMON CAP_NET_RAW CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/unheaded /var/lib/unheaded /var/log/unheaded /sys/fs/bpf /mnt

[Install]
WantedBy=multi-user.target
SVCUNIT
    run systemctl daemon-reload

    # Node exporter for Prometheus
    step "Installing node_exporter..."
    if ! command -v node_exporter &>/dev/null; then
        local ne_ver="1.10.2"
        run wget -q "https://github.com/prometheus/node_exporter/releases/download/v${ne_ver}/node_exporter-${ne_ver}.linux-amd64.tar.gz" \
            -O /tmp/node_exporter.tar.gz 2>/dev/null || warn "node_exporter download failed"
        if [[ -f /tmp/node_exporter.tar.gz ]]; then
            tar -xzf /tmp/node_exporter.tar.gz -C /tmp/
            cp "/tmp/node_exporter-${ne_ver}.linux-amd64/node_exporter" /usr/local/bin/
            rm -rf "/tmp/node_exporter-${ne_ver}.linux-amd64" /tmp/node_exporter.tar.gz

            cat > /etc/systemd/system/node-exporter.service << 'NEUNIT'
[Unit]
Description=Prometheus Node Exporter
After=network.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/node_exporter --collector.systemd --collector.processes
Restart=on-failure

[Install]
WantedBy=multi-user.target
NEUNIT
            run systemctl daemon-reload
            run systemctl enable node-exporter
            run systemctl start node-exporter
        fi
    fi

    ok "Security hardening complete"
}

# ─────────────────────────────────────────────────────────────────────────────
# SUMMARY + NEXT STEPS
# ─────────────────────────────────────────────────────────────────────────────
print_summary() {
    echo ""
    echo -e "${GREEN}${BOLD}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║           THE FORGE IS LIT — KINGDOM FOUNDATION LAID           ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"

    echo -e "${BOLD}INSTALLED:${NC}"
    echo "  AMD GPU:     ROCm ${ROCM_VERSION} (gfx1101 / RX 7700 XT)"
    echo "  Docker:      $(docker --version 2>/dev/null | head -c60 || echo 'pending reboot')"
    echo "  LXD:         $(lxd --version 2>/dev/null || echo 'pending')"
    echo "  Go:          $(/usr/local/go/bin/go version 2>/dev/null || echo 'pending')"
    echo "  Rust:        $(rustc --version 2>/dev/null || echo 'pending')"
    echo "  Nix:         $(nix --version 2>/dev/null || echo 'pending reboot')"
    echo "  Node:        $(node --version 2>/dev/null || echo 'pending')"
    echo "  Claude Code: $(claude --version 2>/dev/null || echo 'installed')"
    echo "  Ollama:      $(ollama --version 2>/dev/null || echo 'installed')"
    echo "  FRR:         $(vtysh -c 'show version' 2>/dev/null | head -1 || echo 'installed')"
    echo "  eBPF:        $(bpftool version 2>/dev/null | head -1 || echo 'installed')"
    echo "  SOPS+age:    $(sops --version 2>/dev/null || echo 'installed') + $(age --version 2>/dev/null || echo 'installed')"
    echo "  step-ca:     $(step version 2>/dev/null | head -1 || echo 'installed')"
    echo "  Vector:      $(vector --version 2>/dev/null || echo 'installed')"
    echo "  PyTorch:     ROCm 6.1 (transformers, accelerate, safetensors)"
    echo "  Monitoring:  Prometheus, Grafana, Loki, VictoriaMetrics, ClickHouse"
    echo ""

    echo -e "${BOLD}STORAGE:${NC}"
    echo "  NVMe:        / (OS + containers + fast I/O)"
    echo "  HDD:         ${HDD_MOUNT} (models, logs, backups)"
    echo "  Models:      ${MODELS_DIR}"
    echo ""

    echo -e "${BOLD}PORTS (Doom Range 16666-26666):${NC}"
    echo "  vLLM:        ${VLLM_PORT} (OpenAI-compatible API)"
    echo "  Dashboard:   20000 (Visor)"
    echo "  Kanban:      20001 (Herald)"
    echo "  Gateway:     21000/21443 (Portcullis)"
    echo "  Grafana:     3000"
    echo "  Prometheus:  9090"
    echo ""

    echo -e "${BOLD}NEXT STEPS:${NC}"
    echo "  1. REBOOT (driver load):    sudo reboot"
    echo "  2. Verify GPU:              rocminfo | grep gfx"
    echo "  3. Clone unheaded:          git clone <repo> ${UNHEADED_HOME}"
    echo "  4. Download a model:        unheaded-download-model deepseek-ai/DeepSeek-R1-Distill-Qwen-7B"
    echo "  5. Build vLLM container:    cd docker/vllm-rocm && docker compose -f docker-compose.vllm.yml build"
    echo "  6. Start inference:         docker compose -f docker-compose.vllm.yml up -d"
    echo "  7. Deploy full kingdom:     make deploy"
    echo "  8. Run preflight:           ./scripts/bare-metal/preflight-host-a.sh"
    echo ""

    echo -e "${BOLD}QUICK TEST AFTER REBOOT:${NC}"
    echo '  # Verify ROCm GPU'
    echo '  rocminfo | grep -i "gfx\|Agent"'
    echo '  # Verify eBPF'
    echo '  bpftool prog list'
    echo '  # Verify Docker'
    echo '  docker run --rm hello-world'
    echo '  # Verify GPU in Docker'
    echo '  docker run --rm --device=/dev/kfd --device=/dev/dri --group-add video rocm/pytorch:6.1.3-ubuntu22.04 rocminfo'
    echo ""

    local log_size
    log_size=$(wc -c < "${LOG_FILE}" 2>/dev/null || echo "0")
    echo -e "Full log: ${LOG_FILE} (${log_size} bytes)"
    echo ""

    if ! $SKIP_REBOOT; then
        echo -e "${YELLOW}${BOLD}⚡ REBOOT REQUIRED for AMD GPU drivers to load ⚡${NC}"
        echo ""
        read -rp "Reboot now? [y/N] " response
        if [[ "${response,,}" == "y" ]]; then
            info "Rebooting in 5 seconds..."
            sleep 5
            reboot
        else
            info "Remember to reboot before using GPU!"
        fi
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# MAIN EXECUTION
# ─────────────────────────────────────────────────────────────────────────────
main() {
    # Create log file
    mkdir -p "$(dirname "${LOG_FILE}")"
    touch "${LOG_FILE}"

    echo -e "${MAGENTA}${BOLD}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║          UNHEADED KINGDOM — LLM LAB SERVER BOOTSTRAP           ║"
    echo "║          Target: Ubuntu 25.x + AMD RX 7700 XT + ROCm          ║"
    echo "║          Storage: 1TB NVMe + 2TB HDD                          ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"

    local start_time
    start_time=$(date +%s)

    preflight
    phase_system_base
    phase_storage
    phase_amd_gpu
    phase_docker
    phase_lxd
    phase_dev_tools
    phase_ebpf
    phase_kernel_tune
    phase_networking
    phase_observability
    phase_vllm
    phase_claude_cli
    phase_secrets
    phase_log_pipeline
    phase_shell_comfort
    phase_security

    local end_time elapsed_min
    end_time=$(date +%s)
    elapsed_min=$(( (end_time - start_time) / 60 ))

    echo ""
    ok "Bootstrap completed in ${elapsed_min} minutes"

    print_summary
}

main "$@"
