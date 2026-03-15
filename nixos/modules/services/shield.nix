# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# shield.nix — Shield eBPF policy enforcement service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.shield;
in {
  options.services.unheaded.shield = {
    enable = mkEnableOption "Unheaded Shield eBPF policy enforcement service";
    
    port = mkOption {
      type = types.port;
      default = 50055;
      description = "gRPC listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-shield = {
      description = "Unheaded Shield eBPF Policy Enforcement Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/shield";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "512M";
        CPUQuota = "200%";
        
        # Security hardening
        NoNewPrivileges = false;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/shield" "/var/log/unheaded" "/sys/kernel/debug" "/sys/kernel/security" ];
        
        # Capabilities for eBPF policy enforcement (Linux 5.8+)
        AmbientCapabilities = [ "CAP_BPF" "CAP_NET_ADMIN" "CAP_SYS_ADMIN" "CAP_NET_BIND_SERVICE" "CAP_SYS_RESOURCE" ];
        CapabilityBoundingSet = [ "CAP_BPF" "CAP_NET_ADMIN" "CAP_SYS_ADMIN" "CAP_NET_BIND_SERVICE" "CAP_SYS_RESOURCE" ];
      };
      
      environment = {
        SHIELD_PORT = toString cfg.port;
        SHIELD_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/shield 0750 unheaded unheaded -"
      "d /var/log/unheaded        0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
