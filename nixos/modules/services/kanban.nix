# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# kanban.nix — Kanban board UI & API service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.kanban;
in {
  options.services.unheaded.kanban = {
    enable = mkEnableOption "Unheaded Kanban board service";
    
    port = mkOption {
      type = types.port;
      default = 3002;
      description = "HTTP listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-kanban = {
      description = "Unheaded Kanban Board UI Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/kanban";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "512M";
        CPUQuota = "200%";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/kanban" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        KANBAN_PORT = toString cfg.port;
        KANBAN_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/kanban 0750 unheaded unheaded -"
      "d /var/log/unheaded        0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
