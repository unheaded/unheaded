# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# telemetry.nix — Full observability stack for Unheaded Kingdom
# Includes: node_exporter, Prometheus, VictoriaMetrics, Loki, Grafana
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.telemetry;
in {
  options.services.unheaded.telemetry = {
    enable = mkEnableOption "Unheaded telemetry stack";
    
    role = mkOption {
      type = types.enum [ "primary" "agent" ];
      default = "agent";
      description = "primary = runs full stack (Host-A), agent = remote_write only (Host-B)";
    };
    
    prometheusRemoteWriteUrl = mkOption {
      type = types.str;
      default = "http://fd00:dead:beef:wg::1:8428/api/v1/write";
      description = "VictoriaMetrics remote_write URL (used by agent nodes)";
    };
    
    scrapeInterval = mkOption {
      type = types.str;
      default = "5s";
      description = "Prometheus scrape interval";
    };
    
    retentionDays = mkOption {
      type = types.int;
      default = 90;
      description = "VictoriaMetrics retention in days (primary only)";
    };
    
    grafanaAdminPassword = mkOption {
      type = types.str;
      default = "changeme-in-production";
      description = "Grafana admin password";
    };
    
    lokiRetentionHours = mkOption {
      type = types.int;
      default = 720;  # 30 days
      description = "Loki log retention in hours";
    };
  };

  config = mkIf cfg.enable {
    # ── node_exporter (ALL hosts) ───────────────────────────────────────────
    services.prometheus.exporters.node = {
      enable = true;
      port = 9100;
      enabledCollectors = [
        "cpu" "diskstats" "filesystem" "loadavg" "meminfo"
        "netdev" "netstat" "stat" "time" "uname" "vmstat"
        "systemd" "processes" "pressure"  # PSI pressure stall
        "interrupts" "buddyinfo" "cpufreq"
      ];
      extraFlags = [
        "--collector.filesystem.ignored-mount-points=^/(sys|proc|dev|run)($|/)"
        "--collector.pressure"  # cgroup v2 PSI
      ];
    };
    
    networking.firewall.allowedTCPPorts = [ 9100 ] 
      ++ optionals (cfg.role == "primary") [ 9090 8428 3000 3100 ];

    # ── Prometheus (primary host only) ─────────────────────────────────────
    services.prometheus = mkIf (cfg.role == "primary") {
      enable = true;
      port = 9090;
      globalConfig = {
        scrape_interval = cfg.scrapeInterval;
        evaluation_interval = cfg.scrapeInterval;
        external_labels = { cluster = "unheaded-kingdom"; };
      };
      
      scrapeConfigs = [
        {
          job_name = "unheaded-forge";
          static_configs = [{ targets = [ "localhost:9100" ]; }];
          relabel_configs = [{ target_label = "host"; replacement = "forge"; }];
        }
        {
          job_name = "unheaded-outpost";
          static_configs = [{ targets = [ "[fd00:dead:beef:wg::2]:9100" ]; }];
          relabel_configs = [{ target_label = "host"; replacement = "outpost"; }];
        }
        {
          job_name = "ebpf-exporter";
          static_configs = [{ targets = [ "localhost:9435" ]; }];
        }
        {
          job_name = "unheaded-services";
          static_configs = [{
            targets = [
              "localhost:50051" "localhost:50052" "localhost:50053"
              "localhost:50054" "localhost:50055" "localhost:8080"
            ];
          }];
          metrics_path = "/metrics";
        }
        {
          job_name = "prometheus";
          static_configs = [{ targets = [ "localhost:9090" ]; }];
        }
        {
          job_name = "victoriametrics";
          static_configs = [{ targets = [ "localhost:8428" ]; }];
        }
      ];
      
      # Remote write to VictoriaMetrics for long-term retention
      remoteWrite = [{
        url = "http://localhost:8428/api/v1/write";
        queue_config = {
          max_samples_per_send = 10000;
          capacity = 50000;
        };
      }];
    };

    # ── VictoriaMetrics (primary host only) ────────────────────────────────
    # NixOS doesn't have VictoriaMetrics in nixpkgs yet; use systemd unit + binary
    systemd.services.victoriametrics = mkIf (cfg.role == "primary") {
      description = "VictoriaMetrics time-series database";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "victoriametrics";
        ExecStart = "/opt/unheaded/bin/victoria-metrics-prod "
          + "-storageDataPath=/var/lib/victoriametrics/data "
          + "-retentionPeriod=${toString cfg.retentionDays}d "
          + "-httpListenAddr=:8428 "
          + "-selfScrapeInterval=10s";
        Restart = "on-failure";
        RestartSec = "10s";
        MemoryMax = "4G";
        CPUQuota = "200%";
      };
    };
    
    users.users.victoriametrics = mkIf (cfg.role == "primary") {
      isSystemUser = true;
      group = "victoriametrics";
      home = "/var/lib/victoriametrics";
      createHome = true;
    };
    users.groups.victoriametrics = mkIf (cfg.role == "primary") {};

    # ── Loki (primary host only) ────────────────────────────────────────────
    services.loki = mkIf (cfg.role == "primary") {
      enable = true;
      configFile = pkgs.writeText "loki-config.yml" ''
        auth_enabled: false
        server:
          http_listen_port: 3100
          grpc_listen_port: 9096
        common:
          path_prefix: /var/lib/loki
          storage:
            filesystem:
              chunks_directory: /var/lib/loki/chunks
              rules_directory: /var/lib/loki/rules
          replication_factor: 1
          ring:
            instance_addr: 127.0.0.1
            kvstore:
              store: inmemory
        schema_config:
          configs:
            - from: "2024-01-01"
              store: boltdb-shipper
              object_store: filesystem
              schema: v12
              index:
                prefix: index_
                period: 24h
        limits_config:
          retention_period: ${toString cfg.lokiRetentionHours}h
          ingestion_rate_mb: 16
          ingestion_burst_size_mb: 32
        compactor:
          retention_enabled: true
      '';
    };
    
    # ── Promtail (ALL hosts — ships logs to Loki on Host-A) ─────────────────
    services.promtail = {
      enable = true;
      configFile = pkgs.writeText "promtail-config.yml" ''
        server:
          http_listen_port: 9080
          grpc_listen_port: 0
        positions:
          filename: /var/lib/promtail/positions.yaml
        clients:
          - url: http://fd00:dead:beef:wg::1:3100/loki/api/v1/push
        scrape_configs:
          - job_name: unheaded-systemd
            journal:
              max_age: 12h
              json: false
              labels:
                job: unheaded
                host: __HOSTNAME__
            relabel_configs:
              - source_labels: ['__journal__systemd_unit']
                target_label: 'unit'
              - source_labels: ['__journal__hostname']
                target_label: 'host'
          - job_name: unheaded-app-logs
            static_configs:
              - targets: [localhost]
                labels:
                  job: unheaded-app
                  host: __HOSTNAME__
                  __path__: /var/log/unheaded/*.log
      '';
    };

    # ── Grafana (primary host only) ─────────────────────────────────────────
    services.grafana = mkIf (cfg.role == "primary") {
      enable = true;
      settings = {
        server = {
          http_port = 3000;
          http_addr = "0.0.0.0";
          domain = "unheaded-forge.local";
        };
        security = {
          admin_user = "admin";
          admin_password = cfg.grafanaAdminPassword;
          secret_key = "unheaded-secret-key-change-me";
        };
        "auth.anonymous" = {
          enabled = false;
        };
        feature_toggles = {
          enable = "traceToMetrics traceToLogs correlations";
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
            jsonData.timeInterval = cfg.scrapeInterval;
          }
          {
            name = "VictoriaMetrics";
            type = "prometheus";
            url = "http://localhost:8428";
            jsonData.timeInterval = cfg.scrapeInterval;
          }
          {
            name = "Loki";
            type = "loki";
            url = "http://localhost:3100";
          }
        ];
        
        dashboards.settings.providers = [{
          name = "Unheaded Kingdom";
          type = "file";
          options.path = "/etc/grafana/dashboards";
        }];
      };
    };
    
    # ── State directories ───────────────────────────────────────────────────
    systemd.tmpfiles.rules = [
      "d /var/lib/loki           0750 loki           loki           -"
      "d /var/lib/loki/chunks    0750 loki           loki           -"
      "d /var/lib/promtail       0750 promtail       promtail       -"
    ];
  };
}
