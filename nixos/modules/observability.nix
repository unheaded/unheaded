# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# Unheaded observability module — Prometheus + Grafana + Loki + VictoriaMetrics + Promtail
{ config, lib, pkgs, ... }:

let
  cfg = config.services.unheaded.observability;
in {
  options.services.unheaded.observability = {
    enable = lib.mkEnableOption "Unheaded observability stack";
    
    role = lib.mkOption {
      type = lib.types.enum [ "primary" "secondary" ];
      default = "primary";
      description = "primary runs full stack; secondary runs promtail + remote_write only";
    };
    
    lokiAddr = lib.mkOption {
      type = lib.types.str;
      default = "fd00:dead:beef::1";
      description = "Loki server address (for secondary role)";
    };
    
    victoriaAddr = lib.mkOption {
      type = lib.types.str;
      default = "fd00:dead:beef::1";
      description = "VictoriaMetrics remote_write address";
    };
    
    grafanaAdminPassword = lib.mkOption {
      type = lib.types.str;
      default = "changeme-in-secrets";
      description = "Grafana admin password — override with sops/agenix in production";
    };
  };

  config = lib.mkIf cfg.enable {
    # --- Prometheus (both roles) ---
    services.prometheus = {
      enable = true;
      port = 9090;
      listenAddress = "[::]";  # IPv6 dual-stack
      configFile = if cfg.role == "primary"
        then /etc/unheaded/prometheus/prometheus.yml
        else /etc/unheaded/prometheus/prometheus-host-b.yml;
      # retentionTime as string
      extraFlags = [ "--storage.tsdb.retention.time=15d" "--web.enable-admin-api" ];
    };
    
    # --- Grafana (primary only) ---
    services.grafana = lib.mkIf (cfg.role == "primary") {
      enable = true;
      settings = {
        server = {
          http_addr = "[::]";
          http_port = 3000;
          domain = "forge.unheaded.local";
        };
        security = {
          admin_user = "admin";
          admin_password = cfg.grafanaAdminPassword;
        };
        "auth.anonymous" = {
          enabled = false;
        };
      };
      provision = {
        enable = true;
        datasources.settings.datasources = [
          {
            name = "Prometheus";
            type = "prometheus";
            url = "http://localhost:9090";
            isDefault = true;
          }
          {
            name = "Loki";
            type = "loki";
            url = "http://localhost:3100";
          }
          {
            name = "VictoriaMetrics";
            type = "prometheus";
            url = "http://[${cfg.victoriaAddr}]:8428";
          }
        ];
      };
    };
    
    # --- Loki (primary only) ---
    services.loki = lib.mkIf (cfg.role == "primary") {
      enable = true;
      configFile = /etc/unheaded/loki/loki.yaml;
    };
    
    # --- VictoriaMetrics (primary only) ---
    # VictoriaMetrics not in nixpkgs stable — run as systemd service with binary
    systemd.services.victoriametrics = lib.mkIf (cfg.role == "primary") {
      description = "VictoriaMetrics time series database";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      serviceConfig = {
        ExecStart = "${pkgs.victoriametrics}/bin/victoria-metrics " +
          "-httpListenAddr=[${cfg.victoriaAddr}]:8428 " +
          "-retentionPeriod=12 " +
          "-storageDataPath=/var/lib/victoriametrics " +
          "-dedup.minScrapeInterval=15s";
        Restart = "always";
        RestartSec = "5s";
        User = "victoriametrics";
        DynamicUser = true;
        StateDirectory = "victoriametrics";
        AmbientCapabilities = [];
        CapabilityBoundingSet = [];
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        NoNewPrivileges = true;
      };
    };
    
    # --- Promtail (both roles) ---
    services.promtail = {
      enable = true;
      configFile = if cfg.role == "primary"
        then /etc/unheaded/promtail/promtail-host-a.yaml
        else /etc/unheaded/promtail/promtail-host-b.yaml;
    };
    
    # --- Firewall ports ---
    networking.firewall = {
      allowedTCPPorts = lib.flatten [
        9090  # prometheus
        (lib.optional (cfg.role == "primary") 3000)   # grafana
        (lib.optional (cfg.role == "primary") 3100)   # loki
        (lib.optional (cfg.role == "primary") 8428)   # victoriametrics
        9100  # node_exporter
      ];
      # CRITICAL: Allow IPv6 HbH (HOPOPT next-header 0x00) — never strip Monad headers
      extraInputRules = ''
        ip6 nexthdr 0 accept comment "Monad HbH passthrough"
      '';
    };
    
    # --- node_exporter (both roles) ---
    services.prometheus.exporters.node = {
      enable = true;
      port = 9100;
      enabledCollectors = [ "cpu" "diskstats" "filesystem" "loadavg" "meminfo" "netdev" "netstat" "systemd" "time" "vmstat" ];
    };
    
    # --- Systemd ordering ---
    systemd.services.prometheus.after = [ "network-online.target" "wg0.service" ];
    systemd.services.promtail.after = [ "network-online.target" "wg0.service" ];
  };
}
