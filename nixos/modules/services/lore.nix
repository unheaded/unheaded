# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# lore.nix — Lore naming & mythology service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.lore;
in {
  options.services.unheaded.lore = {
    enable = mkEnableOption "Unheaded Lore naming service";
    
    port = mkOption {
      type = types.port;
      default = 50063;
      description = "gRPC listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-lore = {
      description = "Unheaded Lore Naming & Mythology Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/lore";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "256M";
        CPUQuota = "100%";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/lore" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        LORE_PORT = toString cfg.port;
        LORE_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/lore 0750 unheaded unheaded -"
      "d /var/log/unheaded      0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
