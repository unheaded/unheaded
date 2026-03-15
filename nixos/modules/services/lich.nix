# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# lich.nix — Lich automated adversary & red team continuous service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.lich;
in {
  options.services.unheaded.lich = {
    enable = mkEnableOption "Unheaded Lich automated adversary service";
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-lich = {
      description = "Unheaded Lich Automated Adversary Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/lich";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "2G";
        CPUQuota = "400%";
        
        # Security hardening
        NoNewPrivileges = false;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/lich" "/var/log/unheaded" ];
        
        # Capabilities for network operations
        AmbientCapabilities = [ "CAP_NET_RAW" "CAP_NET_ADMIN" "CAP_SYS_ADMIN" ];
        CapabilityBoundingSet = [ "CAP_NET_RAW" "CAP_NET_ADMIN" "CAP_SYS_ADMIN" ];
      };
      
      environment = {
        LICH_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/lich 0750 unheaded unheaded -"
      "d /var/log/unheaded      0750 unheaded unheaded -"
    ];
  };
}
