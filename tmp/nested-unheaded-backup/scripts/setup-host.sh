#!/usr/bin/env bash
#
# Unheaded Host Setup
# Fully automated setup for any Linux host (bare metal or VM)
#
# Supports: AWS EC2, Azure VM, GCP, Oracle Cloud, VMware, QEMU/KVM, bare metal
# Requirements: Linux kernel 5.8+, root/sudo access
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root or with sudo"
        exit 1
    fi
}

# Detect OS
detect_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
        log_info "Detected OS: $OS $OS_VERSION"
    else
        log_error "Cannot detect OS. /etc/os-release not found."
        exit 1
    fi
}

# Check kernel version
check_kernel() {
    KERNEL_VERSION=$(uname -r | cut -d. -f1,2)
    REQUIRED_VERSION=5.8

    if awk "BEGIN {exit !($KERNEL_VERSION >= $REQUIRED_VERSION)}"; then
        log_success "Kernel version: $(uname -r) (>= $REQUIRED_VERSION)"
    else
        log_error "Kernel version $(uname -r) is too old. Required: >= $REQUIRED_VERSION"
        exit 1
    fi
}

# Install dependencies based on distro
install_dependencies() {
    log_info "Installing dependencies..."

    case $OS in
        ubuntu|debian)
            export DEBIAN_FRONTEND=noninteractive
            apt-get update
            apt-get install -y \
                lxd lxd-client \
                snapd \
                build-essential \
                curl wget \
                jq \
                net-tools \
                bridge-utils \
                iptables \
                bpftool \
                linux-headers-$(uname -r) \
                clang llvm \
                golang-go \
                rustc cargo \
                nginx \
                git \
                make \
                pkg-config \
                libssl-dev \
                libbpf-dev
            ;;
        fedora|rhel|centos)
            dnf install -y \
                lxc lxd \
                snapd \
                gcc gcc-c++ make \
                curl wget \
                jq \
                net-tools \
                bridge-utils \
                iptables \
                bpftool \
                kernel-devel \
                clang llvm \
                golang \
                rust cargo \
                nginx \
                git \
                openssl-devel
            ;;
        arch)
            pacman -Sy --noconfirm \
                lxd \
                snapd \
                base-devel \
                curl wget \
                jq \
                net-tools \
                bridge-utils \
                iptables \
                bpf \
                linux-headers \
                clang llvm \
                go \
                rust \
                nginx \
                git \
                openssl
            ;;
        *)
            log_warn "Unsupported distro: $OS. Attempting generic setup..."
            ;;
    esac

    log_success "Dependencies installed"
}

# Setup LXD
setup_lxd() {
    log_info "Setting up LXD..."

    # Check if LXD is already initialized
    if lxd waitready --timeout=10 2>/dev/null && lxc network list | grep -q lxdbr0; then
        log_info "LXD already initialized, skipping..."
        return
    fi

    # Initialize LXD with preseed
    cat <<EOF | lxd init --preseed
config:
  core.https_address: '[::]:8443'
  core.trust_password: unheaded-alpha
networks:
- name: lxdbr0
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
  config:
    source: /var/lib/lxd/storage-pools/default
profiles:
- name: default
  devices:
    eth0:
      name: eth0
      network: lxdbr0
      type: nic
    root:
      path: /
      pool: default
      type: disk
EOF

    # Wait for LXD to be ready
    lxd waitready --timeout=30

    log_success "LXD initialized"
}

# Setup networking for Unheaded
setup_networking() {
    log_info "Setting up networking..."

    # Enable IP forwarding
    sysctl -w net.ipv4.ip_forward=1
    sysctl -w net.ipv6.conf.all.forwarding=1

    # Make permanent
    cat >> /etc/sysctl.conf <<EOF

# Unheaded networking
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
net.ipv4.conf.all.rp_filter = 1
net.ipv4.tcp_syncookies = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv6.conf.all.accept_source_route = 0
EOF

    sysctl -p

    log_success "Networking configured"
}

# Install Nix (for NixOS containers)
install_nix() {
    log_info "Installing Nix..."

    if command -v nix &> /dev/null; then
        log_info "Nix already installed, skipping..."
        return
    fi

    # Install Nix (multi-user)
    curl -L https://nixos.org/nix/install | sh -s -- --daemon

    # Source nix
    if [[ -f /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh ]]; then
        . /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
    fi

    # Enable flakes
    mkdir -p ~/.config/nix
    cat > ~/.config/nix/nix.conf <<EOF
experimental-features = nix-command flakes
EOF

    log_success "Nix installed"
}

# Setup eBPF environment
setup_ebpf() {
    log_info "Setting up eBPF environment..."

    # Check if BPF filesystem is mounted
    if ! mount | grep -q /sys/fs/bpf; then
        mount -t bpf bpf /sys/fs/bpf
        echo "bpf /sys/fs/bpf bpf defaults 0 0" >> /etc/fstab
    fi

    # Increase rlimit for locked memory (required for eBPF)
    cat >> /etc/security/limits.conf <<EOF

# Unheaded eBPF
* soft memlock unlimited
* hard memlock unlimited
EOF

    # Apply immediately
    ulimit -l unlimited

    log_success "eBPF environment ready"
}

# Create unheaded user
setup_user() {
    log_info "Setting up unheaded user..."

    if id unheaded &>/dev/null; then
        log_info "User 'unheaded' already exists, skipping..."
        return
    fi

    useradd -r -s /bin/bash -d /opt/unheaded -m unheaded
    usermod -aG lxd unheaded

    log_success "User 'unheaded' created"
}

# Setup systemd service for unheaded-daemon
setup_systemd() {
    log_info "Setting up systemd service..."

    cat > /etc/systemd/system/unheaded-daemon.service <<EOF
[Unit]
Description=Unheaded Control Plane Daemon
Documentation=https://github.com/unheaded/unheaded
After=network-online.target lxd.service
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/unheaded-daemon
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
LimitMEMLOCK=infinity

# Security hardening
CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_BPF CAP_PERFMON
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_BPF CAP_PERFMON
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/unheaded /var/lib/unheaded /sys/fs/bpf

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload

    log_success "Systemd service created (not started yet)"
}

# Check virtualization environment
check_virt() {
    log_info "Checking virtualization environment..."

    if command -v systemd-detect-virt &> /dev/null; then
        VIRT=$(systemd-detect-virt)
        case $VIRT in
            none)
                log_info "Running on bare metal"
                ;;
            kvm|qemu)
                log_info "Running on KVM/QEMU"
                ;;
            vmware)
                log_info "Running on VMware"
                ;;
            amazon)
                log_info "Running on AWS EC2"
                ;;
            microsoft)
                log_info "Running on Azure"
                ;;
            oracle)
                log_info "Running on Oracle Cloud"
                ;;
            *)
                log_info "Running on: $VIRT"
                ;;
        esac
    else
        log_warn "Cannot detect virtualization environment"
    fi
}

# Create directory structure
setup_directories() {
    log_info "Creating directory structure..."

    mkdir -p /opt/unheaded/{bin,config,data,logs}
    mkdir -p /var/lib/unheaded/{state,ebpf}
    mkdir -p /etc/unheaded

    chown -R unheaded:unheaded /opt/unheaded
    chown -R unheaded:unheaded /var/lib/unheaded

    log_success "Directories created"
}

# Print summary
print_summary() {
    echo ""
    log_success "========================================"
    log_success "  Unheaded Host Setup Complete!"
    log_success "========================================"
    echo ""
    echo "System Information:"
    echo "  OS:              $OS $OS_VERSION"
    echo "  Kernel:          $(uname -r)"
    echo "  LXD:             $(lxd --version)"
    echo "  Nix:             $(nix --version 2>/dev/null || echo 'not installed')"
    echo ""
    echo "Next Steps:"
    echo "  1. Build Unheaded: make build"
    echo "  2. Deploy alpha:   sudo ./scripts/deploy-alpha.sh"
    echo "  3. Access dashboard: http://$(hostname -I | awk '{print $1}')"
    echo ""
    log_info "Run 'make help' for more commands"
    echo ""
}

# Main execution
main() {
    echo ""
    log_info "========================================"
    log_info "  Unheaded Host Setup"
    log_info "========================================"
    echo ""

    check_root
    detect_os
    check_kernel
    check_virt

    install_dependencies
    setup_lxd
    setup_networking
    install_nix
    setup_ebpf
    setup_user
    setup_directories
    setup_systemd

    print_summary
}

main "$@"
