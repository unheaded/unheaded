# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# timeguru.nix — Timeguru timeline & roadmap service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.timeguru;
in {
  options.services.unheaded.timeguru = {
    enable = mkEnableOption "Unheaded Timeguru timeline service";
    
    grpcPort = mkOption {
      type = types.port;
      default = 50060;
      description = "gRPC listening port";
    };
    
    httpPort = mkOption {
      type = types.port;
      default = 8600;
      description = "HTTP listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-timeguru = {
      description = "Unheaded Timeguru Timeline Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/timeguru";
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
        ReadWritePaths = [ "/var/lib/unheaded/timeguru" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        TIMEGURU_GRPC_PORT = toString cfg.grpcPort;
        TIMEGURU_HTTP_PORT = toString cfg.httpPort;
        TIMEGURU_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/timeguru 0750 unheaded unheaded -"
      "d /var/log/unheaded          0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.grpcPort cfg.httpPort ];
  };
}
