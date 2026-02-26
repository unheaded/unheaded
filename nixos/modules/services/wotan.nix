# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# wotan.nix — Wotan message bus & event broker (NATS)
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.wotan;
in {
  options.services.unheaded.wotan = {
    enable = mkEnableOption "Unheaded Wotan message bus service";
    
    grpcPort = mkOption {
      type = types.port;
      default = 50053;
      description = "gRPC listening port";
    };
    
    natsPort = mkOption {
      type = types.port;
      default = 4222;
      description = "NATS listening port";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-wotan = {
      description = "Unheaded Wotan Message Bus & Event Broker";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/wotan";
        Restart = "on-failure";
        RestartSec = "5s";
        
        # cgroup v2 resource limits
        CPUAccounting = true;
        MemoryAccounting = true;
        MemoryMax = "1G";
        CPUQuota = "300%";
        
        # Security hardening
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "/var/lib/unheaded/wotan" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };
      
      environment = {
        WOTAN_GRPC_PORT = toString cfg.grpcPort;
        WOTAN_NATS_PORT = toString cfg.natsPort;
        WOTAN_LOG_LEVEL = cfg.logLevel;
      };
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/wotan 0750 unheaded unheaded -"
      "d /var/log/unheaded       0750 unheaded unheaded -"
    ];
    
    # Firewall
    networking.firewall.allowedTCPPorts = [ cfg.grpcPort cfg.natsPort ];
  };
}
