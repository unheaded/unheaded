# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# unheaded-daemon.nix — Unheaded daemon Cuirass control plane reconciler
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.daemon;
in {
  options.services.unheaded.daemon = {
    enable = mkEnableOption "Unheaded daemon control plane reconciler";
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-daemon = {
      description = "Unheaded Daemon Cuirass Control Plane Reconciler";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/unheaded-daemon";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "1G";
        CPUQuota = "300%";
        
        # Security hardening
        NoNewPrivileges = false;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/daemon" "/var/log/unheaded" "/sys/kernel/debug" ];
        
        # Capabilities for eBPF program loading
        AmbientCapabilities = [ "CAP_BPF" "CAP_NET_ADMIN" "CAP_SYS_ADMIN" "CAP_SYS_RESOURCE" ];
        CapabilityBoundingSet = [ "CAP_BPF" "CAP_NET_ADMIN" "CAP_SYS_ADMIN" "CAP_SYS_RESOURCE" ];
      };
      
      environment = {
        DAEMON_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/daemon 0750 unheaded unheaded -"
      "d /var/log/unheaded        0750 unheaded unheaded -"
    ];
  };
}
