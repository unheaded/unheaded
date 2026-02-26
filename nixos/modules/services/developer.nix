# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# developer.nix — Developer development assistance service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.developer;
in {
  options.services.unheaded.developer = {
    enable = mkEnableOption "Unheaded Developer development assistance service";
    
    port = mkOption {
      type = types.port;
      default = 50062;
      description = "gRPC listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-developer = {
      description = "Unheaded Developer Development Assistance Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/developer";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "768M";
        CPUQuota = "300%";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/developer" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        DEVELOPER_PORT = toString cfg.port;
        DEVELOPER_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/developer 0750 unheaded unheaded -"
      "d /var/log/unheaded           0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
