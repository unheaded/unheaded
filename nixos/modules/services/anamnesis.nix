# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# anamnesis.nix — Anamnesis event log & stream service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.anamnesis;
in {
  options.services.unheaded.anamnesis = {
    enable = mkEnableOption "Unheaded Anamnesis event log service";
    
    port = mkOption {
      type = types.port;
      default = 50054;
      description = "gRPC listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
    
    wotanAddr = mkOption {
      type = types.str;
      default = "localhost:50053";
      description = "Wotan event bus address";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-anamnesis = {
      description = "Unheaded Anamnesis Event Log Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" "unheaded-wotan.service" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/anamnesis";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "1G";
        CPUQuota = "200%";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/anamnesis" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        ANAMNESIS_PORT = toString cfg.port;
        ANAMNESIS_LOG_LEVEL = cfg.logLevel;
        WOTAN_ADDR = cfg.wotanAddr;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/anamnesis 0750 unheaded unheaded -"
      "d /var/log/unheaded           0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
