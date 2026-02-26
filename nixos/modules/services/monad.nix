# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# monad.nix — Monad register service (20-byte IPv6 HbH extension header)
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.monad;
in {
  options.services.unheaded.monad = {
    enable = mkEnableOption "Unheaded Monad register service";
    
    port = mkOption {
      type = types.port;
      default = 50051;
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
    systemd.services.unheaded-monad = {
      description = "Unheaded Monad Register Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" "unheaded-wotan.service" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/monad";
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
        ReadWritePaths = [ "/var/lib/unheaded/monad" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        MONAD_PORT = toString cfg.port;
        MONAD_LOG_LEVEL = cfg.logLevel;
        WOTAN_ADDR = cfg.wotanAddr;
        UNHEADED_HOST_ROLE = "forge";
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/monad 0750 unheaded unheaded -"
      "d /var/log/unheaded       0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.port ];
  };
}
