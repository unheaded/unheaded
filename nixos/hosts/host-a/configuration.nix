# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# Host-A: The Forge — Full Unheaded stack + vLLM/ROCm + OPNsense firewall VM
# nixos-rebuild switch to apply
{ config, pkgs, lib, ... }:

{
  imports = [
    ./hardware-configuration.nix
    ../../modules/unheaded-base.nix
    ../../modules/firewall-bridge.nix
    ../../modules/opnsense-vm.nix
    ../../modules/telemetry.nix
    ../../modules/observability.nix
    ../../modules/wireguard.nix
    ../../modules/vllm-rocm.nix
    ../../modules/ebpf-exporter.nix
    ../../modules/frr.nix
    ../../modules/services/monad.nix
    ../../modules/services/sophia.nix
    ../../modules/services/wotan.nix
    ../../modules/services/anamnesis.nix
    ../../modules/services/shield.nix
    ../../modules/services/unheaded-daemon.nix
    ../../modules/services/dashboard-backend.nix
    ../../modules/services/dashboard-frontend.nix
    ../../modules/services/protocol-api.nix
    ../../modules/services/trace-collector.nix
    ../../modules/services/gateway.nix
    ../../modules/services/service-discovery.nix
    ../../modules/services/doom.nix
    ../../modules/services/lich.nix
    ../../modules/services/captain.nix
    ../../modules/services/micromanager.nix
    ../../modules/services/timeguru.nix
    ../../modules/services/architect.nix
    ../../modules/services/developer.nix
    ../../modules/services/kanban.nix
  ];

  # ── FRR Routing (BGP EVPN + IS-IS + BFD) ────────────────────────────────────
  services.unheaded.frr = {
    enable = true;
    bgpAs = 65001;
    routerId = "10.20.255.1";
    wgPeer = "fd00:dead:beef::2";
    peerAs = 65002;
    vnis = [ 10001 10002 10100 ];
  };

  # ── Observability Stack (Prometheus + Grafana + Loki + VictoriaMetrics) ──────
  services.unheaded.observability = {
    enable = true;
    role = "primary";
    victoriaAddr = "fd00:dead:beef::1";
  };

  # ── Networking ──────────────────────────────────────────────────────────────
  networking = {
    hostName = "unheaded-forge";
    useDHCP = false;

    # Primary interface — replace eth0 with actual NIC name (ip link)
    interfaces.eth0 = {
      useDHCP = true;
      ipv6.addresses = [{
        address = "fd00:dead:beef:0001::1";
        prefixLength = 64;
      }];
    };

    # Enable IPv6 globally
    enableIPv6 = true;
    firewall = {
      enable = true;
      allowedTCPPorts = [
        22      # SSH
        80      # HTTP
        443     # HTTPS
        8080    # dashboard-backend
        3000    # Grafana
        9090    # Prometheus
        8428    # VictoriaMetrics
        3100    # Loki
        9100    # node_exporter
        9435    # ebpf-exporter
        50051   # gRPC (monad)
        50052   # gRPC (sophia)
        50053   # gRPC (wotan)
        50054   # gRPC (anamnesis)
        50055   # gRPC (shield)
        4222    # NATS
        16680   # DOOM compute port
      ];
      allowedUDPPorts = [
        51820   # WireGuard
        443     # QUIC/HTTP3
      ];
    };
  };

  # ── Firewall Bridge (WAN macvlan + LAN service bridge) ──────────────────────
  unheaded.firewallBridge = {
    enable = true;
    wanInterface = "eno1";
    lanBridge = "br-unheaded";
    firewallLanIP = "10.20.0.1";
    ipv6Prefix = "fd00:dead:beef:1::/64";
  };

  # ── OPNsense VM (Forge firewall) ──────────────────────────────────────────
  services.unheaded.opnsense = {
    enable = true;
    vcpus = 4;
    memoryMiB = 4096;
    wanMacvlan = "wan-macvlan";
    lanBridge = "br-unheaded";
  };

  # NOTE: All service containers on this host must use 10.20.0.1 as default gateway
  # and place their interfaces on br-unheaded bridge. The OPNsense VM handles
  # all WAN-facing firewall policy and NAT.

  # ── Boot / Kernel ────────────────────────────────────────────────────────────
  boot = {
    kernelPackages = pkgs.linuxPackages_latest;
    kernelParams = [
      "ipv6.disable=0"
      "net.core.rmem_max=26214400"
      "net.core.wmem_max=26214400"
    ];
    kernel.sysctl = {
      # Network performance
      "net.core.rmem_max"           = 26214400;
      "net.core.wmem_max"           = 26214400;
      "net.ipv4.tcp_rmem"           = "4096 65536 26214400";
      "net.ipv4.tcp_wmem"           = "4096 65536 26214400";
      "net.core.netdev_max_backlog" = 5000;
      "net.ipv4.tcp_congestion_control" = "bbr";
      # eBPF
      "kernel.perf_event_paranoid"  = 1;
      "kernel.kptr_restrict"        = 1;
      # cgroup v2
      "kernel.unprivileged_bpf_disabled" = 0;
      # IPv6
      "net.ipv6.conf.all.forwarding" = 1;
      "net.ipv6.conf.default.forwarding" = 1;
    };
    # cgroup v2 unified hierarchy
    kernelFeatures = [ "cgroupv2" ];
    extraModulePackages = [];
  };

  # ── cgroup v2 ────────────────────────────────────────────────────────────────
  systemd.enableUnifiedCgroupHierarchy = true;

  # ── Hardware ─────────────────────────────────────────────────────────────────
  hardware = {
    enableRedistributableFirmware = true;
    opengl = {
      enable = true;
      driSupport = true;
      extraPackages = with pkgs; [ rocm-opencl-icd rocm-opencl-runtime ];
    };
  };

  # ── ROCm (AMD GPU compute) ───────────────────────────────────────────────────
  systemd.tmpfiles.rules = [
    "L+    /opt/rocm/hip   -    -    -     -    ${pkgs.hip}"
  ];

  # ── SSH ──────────────────────────────────────────────────────────────────────
  services.openssh = {
    enable = true;
    settings = {
      PasswordAuthentication = false;
      PermitRootLogin = "no";
      KbdInteractiveAuthentication = false;
    };
  };

  # ── Users ────────────────────────────────────────────────────────────────────
  users.users.unheaded = {
    isNormalUser = true;
    extraGroups = [ "wheel" "video" "render" "networkmanager" ];
    openssh.authorizedKeys.keys = [
      # TODO: add your SSH public key here
      # "ssh-ed25519 AAAA... user@host"
    ];
  };

  # ── Packages ─────────────────────────────────────────────────────────────────
  environment.systemPackages = with pkgs; [
    # System tools
    htop btop iotop iftop nethogs
    bpftool bpftrace perf
    iproute2 tcpdump wireshark-cli
    wireguard-tools
    git curl wget jq yq
    # Go + Rust toolchains (for source builds)
    go rustup
    # eBPF
    llvm clang libelf zlib
    libbpf
    # Monitoring
    prometheus-node-exporter
    grafana
    # ROCm
    rocmPackages.rocm-smi
    rocmPackages.rocminfo
    # Virtualization
    libvirt virsh
  ];

  # ── NixOS state version ───────────────────────────────────────────────────────
  system.stateVersion = "24.11";
}
