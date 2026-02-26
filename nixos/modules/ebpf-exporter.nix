# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# ebpf-exporter.nix — eBPF BPF map → Prometheus metrics bridge
# Reads BPF ring buffer / maps from Shield+Monad programs
# Exposes on :9435/metrics
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.ebpfExporter;
in {
  options.services.unheaded.ebpfExporter = {
    enable = mkEnableOption "Unheaded eBPF Prometheus exporter";
    
    port = mkOption {
      type = types.port;
      default = 9435;
      description = "HTTP port for Prometheus metrics endpoint";
    };
    
    bpfProgramPath = mkOption {
      type = types.path;
      default = "/opt/unheaded/ebpf";
      description = "Directory containing compiled BPF object files (.o)";
    };
    
    scrapeInterval = mkOption {
      type = types.str;
      default = "1s";
      description = "BPF map polling interval";
    };
    
    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log level";
    };
  };

  config = mkIf cfg.enable {
    systemd.services.unheaded-ebpf-exporter = {
      description = "Unheaded eBPF Prometheus Exporter";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" "unheaded-shield.service" ];
      
      serviceConfig = {
        Type = "simple";
        User = "root";  # eBPF map access requires root or CAP_BPF
        
        ExecStart = "${/opt/unheaded/bin/ebpf-exporter} "
          + "--port=${toString cfg.port} "
          + "--bpf-path=${cfg.bpfProgramPath} "
          + "--interval=${cfg.scrapeInterval} "
          + "--log-level=${cfg.logLevel}";
        
        Restart = "on-failure";
        RestartSec = "5s";
        
        # Minimal capability set for eBPF
        NoNewPrivileges = true;
        AmbientCapabilities = [
          "CAP_BPF"           # BPF syscall
          "CAP_NET_ADMIN"     # XDP/TC program loading
          "CAP_PERFMON"       # perf event access
          "CAP_SYS_RESOURCE"  # rlimit for BPF maps
        ];
        CapabilityBoundingSet = [
          "CAP_BPF" "CAP_NET_ADMIN" "CAP_PERFMON" "CAP_SYS_RESOURCE"
        ];
        MemoryMax = "256M";
        CPUQuota  = "50%";
      };
      
      environment = {
        EBPF_EXPORTER_PORT         = toString cfg.port;
        EBPF_EXPORTER_BPF_PATH     = cfg.bpfProgramPath;
        EBPF_EXPORTER_INTERVAL     = cfg.scrapeInterval;
        EBPF_EXPORTER_LOG_LEVEL    = cfg.logLevel;
      };
    };
    
    networking.firewall.allowedTCPPorts = [ cfg.port ];
    
    # BPF program output directory
    systemd.tmpfiles.rules = [
      "d /opt/unheaded/ebpf 0750 root root -"
    ];
  };
}
