# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# sophia.nix — Sophia BPF dictionary maps service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.sophia;
in {
  options.services.unheaded.sophia = {
    enable = mkEnableOption "Unheaded Sophia BPF dictionary service";
    
    port = mkOption {
      type = types.port;
      default = 50052;
      description = "gRPC listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-sophia = {
      description = "Unheaded Sophia BPF Dictionary Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/sophia";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "768M";
        CPUQuota = "200%";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/sophia" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        SOPHIA_PORT = toString cfg.port;
        SOPHIA_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/sophia 0750 unheaded unheaded -"
      "d /var/log/unheaded        0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
