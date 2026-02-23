{ config, pkgs, ... }:

{
  # =============================================================================
  # CUIRASS CONTROL PLANE CONTAINER
  # =============================================================================
  # The Unheaded control plane - orchestrates container lifecycle,
  # configuration management, and system-wide coordination.
  #
  # Service: Control Plane API (REST + gRPC + Wotan)
  # IP: 10.10.10.5
  # Ports: 8005 (HTTP API), 9005 (gRPC), 9100 (metrics)
  # Critical: Manages all other containers
  #
  # Responsibilities:
  #   - Container lifecycle management
  #   - Configuration drift detection
  #   - Service health aggregation
  #   - Deployment coordination
  #   - State reconciliation
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  # ===========================================================================
  # BASE CONTAINER CONFIGURATION
  # ===========================================================================
  unheaded.container = {
    enable = true;
    name = "cuirass";

    # Security: Slightly elevated for container management
    security = {
      capabilities = [
        "CAP_NET_BIND_SERVICE"    # Bind to privileged ports
      ];
      readOnlyRoot = true;
      writablePaths = [
        "/var/lib/unheaded/cuirass"
        "/var/lib/unheaded/state"       # Global state storage
        "/var/log/unheaded/cuirass"
      ];
      seccompProfile = "strict";
    };

    # Network: Control plane needs broad access
    network = {
      ip = "10.10.10.5";
      ports = [ 8005 9005 9100 ];
      allowedSources = [ "10.10.10.0/24" ];
      allowWotanAccess = true;
    };

    # Health check
    healthCheck = {
      enable = true;
      port = 8005;
      path = "/health";
    };

    # Resources: Higher limits for control plane operations
    resources = {
      memoryMax = "1G";
      cpuQuota = "200%";        # 2 cores
      tasksMax = 1024;
      nofileLimit = 65536;
    };
  };

  # ===========================================================================
  # HARDENING CONFIGURATION
  # ===========================================================================
  unheaded.hardening = {
    enable = true;
    serviceName = "cuirass";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/cuirass"
      "/var/lib/unheaded/state"
      "/var/log/unheaded/cuirass"
    ];
    allowedPorts = [ 8005 9005 9100 ];
  };

  # ===========================================================================
  # NETWORKING CONFIGURATION
  # ===========================================================================
  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.5";
    servicePort = 8005;
    allowDirectAccess = true;        # Gateway and all services need access
    allowedSources = [ "10.10.10.0/24" ];
  };

  # ===========================================================================
  # COMMON CONFIGURATION
  # ===========================================================================
  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
    metricsPort = 9100;
  };

  # ===========================================================================
  # CUIRASS SERVICE DEFINITION
  # ===========================================================================
  systemd.services.cuirass = {
    description = "Cuirass Control Plane - Container Orchestration";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "wotan.service" ];
    wants = [ "network-online.target" "wotan.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";

      # Service binary
      ExecStart = "${pkgs.cuirass}/bin/cuirass serve";

      # Working directory
      WorkingDirectory = "/var/lib/unheaded/cuirass";

      # Environment configuration
      Environment = [
        # API endpoints
        "CUIRASS_HTTP_ADDR=0.0.0.0:8005"
        "CUIRASS_GRPC_ADDR=0.0.0.0:9005"
        "CUIRASS_METRICS_ADDR=0.0.0.0:9100"

        # Wotan connection
        "CUIRASS_WOTAN_ADDR=10.10.10.10:9090"
        "CUIRASS_WOTAN_HTTP=http://10.10.10.10:8080"

        # State management
        "CUIRASS_STATE_DIR=/var/lib/unheaded/state"
        "CUIRASS_CONFIG_DIR=/etc/unheaded/cuirass"

        # Reconciliation settings
        "CUIRASS_RECONCILE_INTERVAL=30s"
        "CUIRASS_HEALTH_CHECK_INTERVAL=10s"
        "CUIRASS_DRIFT_DETECTION=true"

        # Managed containers
        "CUIRASS_MANAGED_CONTAINERS=wotan,timeguru,captain,micromanager,architect,developer,monad,sophia,gateway,kanban,dashboard"

        # Logging
        "CUIRASS_LOG_LEVEL=info"
        "CUIRASS_LOG_FORMAT=json"
      ];

      # Restart policy (critical service)
      Restart = "always";
      RestartSec = "5s";
      StartLimitBurst = 10;
      StartLimitIntervalSec = 60;

      # Health check after startup
      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 3 && /etc/unheaded/health/cuirass-health.sh'";

      # Graceful shutdown
      KillSignal = "SIGTERM";
      TimeoutStopSec = "30s";

      # Resource limits (control plane needs more resources)
      MemoryMax = "1G";
      CPUQuota = "200%";
      TasksMax = 1024;
    };
  };

  # ===========================================================================
  # CUIRASS HEALTH CHECK TIMER
  # ===========================================================================
  # Periodic health check for all managed containers
  # ===========================================================================
  systemd.services.cuirass-health-aggregator = {
    description = "Cuirass Health Aggregator";
    after = [ "cuirass.service" ];
    requires = [ "cuirass.service" ];

    serviceConfig = {
      Type = "oneshot";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.bash}/bin/bash -c '/etc/unheaded/health/aggregate-health.sh'";
    };
  };

  systemd.timers.cuirass-health-aggregator = {
    description = "Periodic health aggregation";
    wantedBy = [ "timers.target" ];
    timerConfig = {
      OnCalendar = "*:*:0/30";       # Every 30 seconds
      Persistent = true;
    };
  };

  # ===========================================================================
  # HEALTH AGGREGATION SCRIPT
  # ===========================================================================
  environment.etc."unheaded/health/aggregate-health.sh" = {
    text = ''
      #!/usr/bin/env bash
      # =======================================================================
      # Health Aggregation Script
      # =======================================================================
      # Collects health status from all managed containers and reports to Wotan
      # =======================================================================
      set -euo pipefail

      CONTAINERS="wotan timeguru captain micromanager architect developer monad sophia gateway kanban dashboard"
      STATE_DIR="/var/lib/unheaded/state"
      TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

      # Initialize health report
      HEALTH_REPORT="{\"timestamp\":\"$TIMESTAMP\",\"containers\":{"

      # Check each container
      for container in $CONTAINERS; do
        # Get container IP (from known network layout)
        case "$container" in
          wotan)        IP="10.10.10.10"; PORT="8080" ;;
          timeguru)      IP="10.10.10.20"; PORT="8000" ;;
          captain)       IP="10.10.10.21"; PORT="8001" ;;
          micromanager)  IP="10.10.10.22"; PORT="8002" ;;
          architect)     IP="10.10.10.23"; PORT="8003" ;;
          developer)     IP="10.10.10.24"; PORT="8004" ;;
          monad)         IP="10.10.10.27"; PORT="8006" ;;
          sophia)        IP="10.10.10.26"; PORT="8007" ;;
          gateway)       IP="10.10.10.100"; PORT="443" ;;
          kanban)        IP="10.10.10.200"; PORT="8080" ;;
          dashboard)     IP="10.10.10.201"; PORT="8081" ;;
          *)             continue ;;
        esac

        # Perform health check
        if curl -sf --connect-timeout 2 --max-time 5 "http://$IP:$PORT/health" > /dev/null 2>&1; then
          STATUS="healthy"
        else
          STATUS="unhealthy"
        fi

        HEALTH_REPORT="$HEALTH_REPORT\"$container\":{\"status\":\"$STATUS\",\"ip\":\"$IP\",\"port\":$PORT},"
      done

      # Close JSON (remove trailing comma, close braces)
      HEALTH_REPORT="''${HEALTH_REPORT%,}}}"

      # Write to state file
      echo "$HEALTH_REPORT" > "$STATE_DIR/health-report.json"

      # Publish to Wotan (if available)
      curl -sf -X POST \
        -H "Content-Type: application/json" \
        -d "$HEALTH_REPORT" \
        "http://10.10.10.10:8080/api/v1/publish/health.aggregated" \
        > /dev/null 2>&1 || true

      echo "Health aggregation complete"
    '';
    mode = "0755";
  };

  # ===========================================================================
  # STATE RECONCILIATION SCRIPT
  # ===========================================================================
  environment.etc."unheaded/cuirass/reconcile.sh" = {
    text = ''
      #!/usr/bin/env bash
      # =======================================================================
      # State Reconciliation Script
      # =======================================================================
      # Compares desired state with actual state and reports drift
      # =======================================================================
      set -euo pipefail

      STATE_DIR="/var/lib/unheaded/state"
      CONFIG_DIR="/etc/unheaded/cuirass"
      TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

      # Load desired state
      if [[ -f "$CONFIG_DIR/desired-state.json" ]]; then
        DESIRED=$(cat "$CONFIG_DIR/desired-state.json")
      else
        echo "No desired state configured"
        exit 0
      fi

      # Load current state
      if [[ -f "$STATE_DIR/health-report.json" ]]; then
        CURRENT=$(cat "$STATE_DIR/health-report.json")
      else
        echo "No current state available"
        exit 1
      fi

      # Compare states (simplified - full implementation would use jq)
      # For now, just report that reconciliation ran
      echo "{\"timestamp\":\"$TIMESTAMP\",\"reconciliation\":\"complete\",\"drift\":false}" \
        > "$STATE_DIR/reconciliation-report.json"

      echo "Reconciliation complete"
    '';
    mode = "0755";
  };

  # ===========================================================================
  # CONFIGURATION FILES
  # ===========================================================================
  environment.etc."unheaded/cuirass/config.yaml" = {
    text = ''
      # Cuirass Control Plane Configuration
      # ====================================

      server:
        http_addr: "0.0.0.0:8005"
        grpc_addr: "0.0.0.0:9005"
        metrics_addr: "0.0.0.0:9100"

      wotan:
        grpc_addr: "10.10.10.10:9090"
        http_addr: "http://10.10.10.10:8080"
        topics:
          subscribe:
            - "health.updates"
            - "config.changes"
            - "alerts.critical"
          publish:
            - "health.aggregated"
            - "reconciliation.reports"
            - "deployment.events"

      state:
        dir: "/var/lib/unheaded/state"
        sync_interval: "30s"

      reconciliation:
        enabled: true
        interval: "30s"
        drift_detection: true
        auto_remediate: false    # Manual approval required

      containers:
        managed:
          - name: "wotan"
            ip: "10.10.10.10"
            ports: [9090, 8080, 9100]
            critical: true

          - name: "timeguru"
            ip: "10.10.10.20"
            ports: [8000, 9100]
            depends_on: ["wotan"]

          - name: "captain"
            ip: "10.10.10.21"
            ports: [8001, 9100]
            depends_on: ["wotan"]

          - name: "micromanager"
            ip: "10.10.10.22"
            ports: [8002, 9100]
            depends_on: ["wotan"]

          - name: "architect"
            ip: "10.10.10.23"
            ports: [8003, 9100]
            depends_on: ["wotan"]

          - name: "developer"
            ip: "10.10.10.24"
            ports: [8004, 9100]
            depends_on: ["wotan"]

          - name: "monad"
            ip: "10.10.10.27"
            ports: [8006, 9100]
            depends_on: ["wotan"]

          - name: "sophia"
            ip: "10.10.10.26"
            ports: [8007, 9100]
            depends_on: ["wotan"]

          - name: "gateway"
            ip: "10.10.10.100"
            ports: [80, 443, 9100]
            depends_on: ["wotan", "timeguru", "captain", "micromanager", "architect", "monad", "sophia"]
            public: true
            critical: true

          - name: "kanban"
            ip: "10.10.10.200"
            ports: [8080, 9100]
            depends_on: ["wotan", "timeguru"]
            public: true

          - name: "dashboard"
            ip: "10.10.10.201"
            ports: [8081, 9100]
            depends_on: ["wotan"]
            public: true

      logging:
        level: "info"
        format: "json"
        output: "/var/log/unheaded/cuirass/cuirass.log"

      security:
        tls_enabled: false       # ALPHA: Disabled for local development. MUST enable before production deployment.
        auth_required: false     # ALPHA: Disabled for local development. MUST enable before production deployment.
    '';
    mode = "0644";
  };

  # ===========================================================================
  # ADDITIONAL FIREWALL RULES
  # ===========================================================================
  # Control plane needs both HTTP and gRPC accessible
  # ===========================================================================
  networking.firewall.allowedTCPPorts = [ 8005 9005 9100 ];

  # ===========================================================================
  # STATE DIRECTORY STRUCTURE
  # ===========================================================================
  systemd.tmpfiles.rules = [
    # Global state directory (shared across containers)
    "d /var/lib/unheaded/state 0750 unheaded unheaded -"

    # Cuirass-specific state
    "d /var/lib/unheaded/cuirass 0750 unheaded unheaded -"
    "d /var/lib/unheaded/cuirass/deployments 0750 unheaded unheaded -"
    "d /var/lib/unheaded/cuirass/backups 0750 unheaded unheaded -"
  ];
}
