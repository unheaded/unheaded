# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# architect.nix — Architect infrastructure architecture service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.architect;
in {
  options.services.unheaded.architect = {
    enable = mkEnableOption "Unheaded Architect infrastructure service";
    
    port = mkOption {
      type = types.port;
      default = 50061;
      description = "gRPC listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-architect = {
      description = "Unheaded Architect Infrastructure Architecture Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/architect";
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
        ReadWritePaths = [ "/var/lib/unheaded/architect" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        ARCHITECT_PORT = toString cfg.port;
        ARCHITECT_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/architect 0750 unheaded unheaded -"
      "d /var/log/unheaded           0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
