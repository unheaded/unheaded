# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# service-discovery.nix — Service discovery registry & Greaves service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.serviceDiscovery;
in {
  options.services.unheaded.serviceDiscovery = {
    enable = mkEnableOption "Unheaded service discovery registry service";
    
    grpcPort = mkOption {
      type = types.port;
      default = 50057;
      description = "gRPC listening port";
    };
    
    httpPort = mkOption {
      type = types.port;
      default = 8500;
      description = "HTTP listening port (Consul-compatible)";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-service-discovery = {
      description = "Unheaded Service Discovery Registry Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/service-discovery";
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
        ReadWritePaths = [ "/var/lib/unheaded/discovery" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        DISCOVERY_GRPC_PORT = toString cfg.grpcPort;
        DISCOVERY_HTTP_PORT = toString cfg.httpPort;
        DISCOVERY_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/discovery 0750 unheaded unheaded -"
      "d /var/log/unheaded           0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.grpcPort cfg.httpPort ];
  };
}
