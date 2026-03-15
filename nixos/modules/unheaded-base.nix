# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# unheaded-base.nix — Shared baseline for all Unheaded Kingdom hosts
{ config, pkgs, lib, ... }:

{
  # ── Locale / Time ─────────────────────────────────────────────────────────
  time.timeZone = "UTC";
  i18n.defaultLocale = "en_US.UTF-8";

  # ── Nix configuration ─────────────────────────────────────────────────────
  nix = {
    settings = {
      experimental-features = [ "nix-command" "flakes" ];
      auto-optimise-store = true;
      max-jobs = "auto";
    };
    gc = {
      automatic = true;
      dates = "weekly";
      options = "--delete-older-than 30d";
    };
  };

  # ── cgroup v2 / systemd resource accounting ───────────────────────────────
  systemd = {
    enableUnifiedCgroupHierarchy = true;
    # Default resource limits for all Unheaded services
    services."unheaded-*" = {
      serviceConfig = {
        MemoryAccounting = true;
        CPUAccounting    = true;
        IOAccounting     = true;
        TasksAccounting  = true;
      };
    };
  };

  # ── Security baseline ─────────────────────────────────────────────────────
  security = {
    sudo.wheelNeedsPassword = true;
    apparmor.enable = true;
    auditd.enable = true;
    audit = {
      enable = true;
      rules = [
        "-a always,exit -F arch=b64 -S execve"
        "-w /etc/passwd -p wa"
        "-w /etc/shadow -p wa"
      ];
    };
  };

  # ── BTF / eBPF kernel requirements ───────────────────────────────────────
  boot.kernel.sysctl."kernel.unprivileged_bpf_disabled" = 0;

  # ── NTP ──────────────────────────────────────────────────────────────────
  services.timesyncd.enable = true;

  # ── Fail2ban ─────────────────────────────────────────────────────────────
  services.fail2ban = {
    enable = true;
    maxretry = 5;
    bantime = "1h";
  };

  # ── Common packages available on all hosts ────────────────────────────────
  environment.systemPackages = with pkgs; [
    vim nano
    tmux screen
    git
    curl wget
    jq yq
    htop
    strace ltrace
  ];
}
