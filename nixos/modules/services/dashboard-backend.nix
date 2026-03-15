# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# dashboard-backend.nix — Dashboard backend Cape & Cloak service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.dashboardBackend;
in {
  options.services.unheaded.dashboardBackend = {
    enable = mkEnableOption "Unheaded Dashboard backend service";
    
    httpPort = mkOption {
      type = types.port;
      default = 8080;
      description = "HTTP listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-dashboard-backend = {
      description = "Unheaded Dashboard Backend Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/dashboard-backend";
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
        ReadWritePaths = [ "/var/lib/unheaded/dashboard" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        DASHBOARD_HTTP_PORT = toString cfg.httpPort;
        DASHBOARD_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/dashboard 0750 unheaded unheaded -"
      "d /var/log/unheaded           0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.httpPort ];
  };
}
