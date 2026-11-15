# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# trace-collector.nix — Trace collector Zipkin-compatible service
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.traceCollector;
in {
  options.services.unheaded.traceCollector = {
    enable = mkEnableOption "Unheaded trace collector service";
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-trace-collector = {
      description = "Unheaded Trace Collector Service";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "/opt/unheaded/bin/trace-collector-go";
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
        ReadWritePaths = [ "/var/lib/unheaded/traces" "/var/log/unheaded" ];
        
        # Capabilities
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      };

      # No environment block: the binary reads only UNHEADED_EBPF_OBJ_DIR from
      # the environment and takes its listeners from flags (--http-addr,
      # --metrics-addr; loopback 16670 and 16671 unless told).
    };
    
    # State directory
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/traces 0750 unheaded unheaded -"
      "d /var/log/unheaded        0750 unheaded unheaded -"
    ];

    # No firewall rule: the collector binds loopback, and a rule for a loopback
    # service guards nothing (tests/scripts/test_nix_firewall.py, same rule).
  };
}
