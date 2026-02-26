# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# dashboard-frontend.nix — Dashboard frontend UI static file server
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.dashboardFrontend;
in {
  options.services.unheaded.dashboardFrontend = {
    enable = mkEnableOption "Unheaded Dashboard frontend service";
    
    port = mkOption {
      type = types.port;
      default = 3001;
      description = "HTTP listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-dashboard-frontend = {
      description = "Unheaded Dashboard Frontend UI Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/dashboard-frontend";
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
        ReadWritePaths = [ "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        FRONTEND_PORT = toString cfg.port;
        FRONTEND_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # Log directory
    systemd.tmpfiles.rules = [
      "d /var/log/unheaded 0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
