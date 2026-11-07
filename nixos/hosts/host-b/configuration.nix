# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# Host-B: The Outpost — Minimal Unheaded suite (IPFire firewall VM removed 2026-11-03)
# nixos-rebuild switch to apply
{ config, pkgs, lib, ... }:

{
  imports = [
    ./hardware-configuration.nix
    ../../modules/unheaded-base.nix
    ../../modules/firewall-bridge.nix
    ../../modules/telemetry.nix
    ../../modules/observability.nix
    ../../modules/wireguard.nix
    ../../modules/bird.nix
    ../../modules/services/monad.nix
    ../../modules/services/wotan.nix
    ../../modules/services/anamnesis.nix
    ../../modules/services/shield.nix
    ../../modules/services/unheaded-daemon.nix
    ../../modules/services/service-discovery.nix
  ];

  # ── BIRD Routing (iBGP + BFD + radv) ────────────────────────────────────────
  services.unheaded.bird = {
    enable = true;
    bgpAs = 65002;
    routerId = "10.20.255.2";
    forgePeer = "fd00:dead:beef::1";
    forgeAs = 65001;
  };

  # ── Observability Stack (Secondary: Promtail + remote_write only) ────────────
  services.unheaded.observability = {
    enable = true;
    role = "secondary";
    lokiAddr = "fd00:dead:beef::1";
    victoriaAddr = "fd00:dead:beef::1";
  };

  # ── Networking ──────────────────────────────────────────────────────────────
  networking = {
    hostName = "unheaded-outpost";
    useDHCP = false;

    interfaces.eth0 = {
      useDHCP = true;
      ipv6.addresses = [{
        address = "fd00:dead:beef:0002::1";
        prefixLength = 64;
      }];
    };

    enableIPv6 = true;
    firewall = {
      enable = true;
      allowedTCPPorts = [
        22 9100 50051 50053 50054 4222
      ];
      allowedUDPPorts = [ 51820 443 ];
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

  # The IPFire firewall VM (services.unheaded.ipfire, modules/ipfire-vm.nix)
  # was removed 2026-11-03 (Stevie). WAN policy and NAT are the host's own
  # nftables ruleset (deploy/nftables/, ADR-098 step 6).

  # ── Boot / Kernel ────────────────────────────────────────────────────────────
  boot = {
    kernelPackages = pkgs.linuxPackages_latest;
    kernel.sysctl = {
      "net.ipv6.conf.all.forwarding"     = 1;
      "net.ipv6.conf.default.forwarding" = 1;
      "kernel.perf_event_paranoid"       = 1;
      "kernel.kptr_restrict"             = 1;
      "kernel.unprivileged_bpf_disabled" = 2;
      "net.core.rmem_max"                = 13107200;
      "net.core.wmem_max"                = 13107200;
    };
  };

  systemd.enableUnifiedCgroupHierarchy = true;

  services.openssh = {
    enable = true;
    settings = {
      PasswordAuthentication = false;
      PermitRootLogin = "no";
      KbdInteractiveAuthentication = false;
    };
  };

  users.users.unheaded = {
    isNormalUser = true;
    extraGroups = [ "wheel" "networkmanager" ];
    openssh.authorizedKeys.keys = [
      # TODO: add your SSH public key here
    ];
  };

  environment.systemPackages = with pkgs; [
    htop btop iftop
    bpftool bpftrace perf
    iproute2 tcpdump
    wireguard-tools
    git curl wget jq
    go
    llvm clang libelf zlib libbpf
    prometheus-node-exporter
    # Virtualization
    libvirt virsh
  ];

  system.stateVersion = "24.11";
}
