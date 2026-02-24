{ config, lib, pkgs, ... }:

{
  # =============================================================================
  # UNHEADED CROSS-SERVICE HEALTH MONITORING MODULE
  # =============================================================================
  # Implements the Gauntlets Law severity system from CLAUDE.md.
  #
  # Each service health-checks all services it depends on.
  # Failures are reported to Wotan topic `system.outage.reports`.
  # Severity is determined by percentage-based consensus:
  #
  #   | % Reporting | Severity | UI Color  | Hex     | Actions          |
  #   |-------------|----------|-----------|---------|------------------|
  #   | 0 - 12.49%  | OK       | Green     | #008000 | Healthy          |
  #   | 12.5-37.49% | WARN     | Yel-Brown | #fdda61 | Log, email       |
  #   | 37.5-62.49% | ERROR    | Bright Yel| #ffff00 | Log, 2nd email   |
  #   | 62.5-87.49% | CRITICAL | Neon Org  | #ff5c00 | Auto-remediate   |
  #   | 87.5-100%   | PANIC    | Bright Red| #ff0000 | All hands, Pager |
  #
  # Formula: (unique_reporters / total_dependent_services) * 100
  #
  # Usage:
  #   imports = [ ../modules/health-monitor.nix ];
  #   unheaded.healthMonitor = {
  #     enable = true;
  #     serviceName = "timeguru";
  #     dependencies = [
  #       { name = "wotan"; ip = "10.10.10.10"; port = 18000; path = "/health"; }
  #     ];
  #   };
  # =============================================================================

  options = {
    unheaded.healthMonitor = {
      enable = lib.mkEnableOption "Unheaded cross-service health monitoring";

      serviceName = lib.mkOption {
        type = lib.types.str;
        description = "Name of this service (reporter identity)";
      };

      dependencies = lib.mkOption {
        type = lib.types.listOf (lib.types.submodule {
          options = {
            name = lib.mkOption {
              type = lib.types.str;
              description = "Dependency service name";
            };
            ip = lib.mkOption {
              type = lib.types.str;
              description = "Dependency service IP";
            };
            port = lib.mkOption {
              type = lib.types.int;
              description = "Dependency service health check port";
            };
            path = lib.mkOption {
              type = lib.types.str;
              default = "/health";
              description = "Health check HTTP path";
            };
          };
        });
        default = [ ];
        description = "Services this service depends on";
      };

      checkInterval = lib.mkOption {
        type = lib.types.str;
        default = "30s";
        description = "Interval between health check runs";
      };

      checkTimeout = lib.mkOption {
        type = lib.types.int;
        default = 5;
        description = "HTTP timeout in seconds for each health check";
      };

      wotanAddr = lib.mkOption {
        type = lib.types.str;
        default = "http://10.10.10.10:18000";
        description = "Wotan REST API address for publishing reports";
      };

      totalServices = lib.mkOption {
        type = lib.types.int;
        default = 10;
        description = ''
          Total number of dependent services in the mesh.
          Used for Gauntlets Law consensus calculation.
          Formula: (unique_reporters / totalServices) * 100
        '';
      };
    };
  };

  config = lib.mkIf config.unheaded.healthMonitor.enable {
    # =========================================================================
    # HEALTH MONITOR SCRIPT
    # =========================================================================
    # Checks all dependencies, calculates Gauntlets Law severity,
    # writes local report, and publishes to Wotan.
    # =========================================================================
    environment.etc."unheaded/health/monitor.sh" = {
      text = let
        serviceName = config.unheaded.healthMonitor.serviceName;
        timeout = toString config.unheaded.healthMonitor.checkTimeout;
        wotanAddr = config.unheaded.healthMonitor.wotanAddr;
        totalServices = toString config.unheaded.healthMonitor.totalServices;
        deps = config.unheaded.healthMonitor.dependencies;

        # Generate the dependency check block
        depChecks = lib.concatMapStringsSep "\n" (dep: ''
          # Check ${dep.name}
          if curl -sf --connect-timeout 2 --max-time ${timeout} \
              "http://${dep.ip}:${toString dep.port}${dep.path}" > /dev/null 2>&1; then
            dep_results="${dep.name}:healthy"
          else
            dep_results="${dep.name}:unhealthy"
            FAILED_COUNT=$((FAILED_COUNT + 1))
            FAILED_SERVICES="$FAILED_SERVICES ${dep.name}"
          fi
          ALL_RESULTS="$ALL_RESULTS,$dep_results"
        '') deps;

      in ''
        #!/usr/bin/env bash
        # =======================================================================
        # Cross-Service Health Monitor for ${serviceName}
        # =======================================================================
        # Implements Gauntlets Law percentage-based consensus severity.
        # Reports to Wotan topic: system.outage.reports
        # =======================================================================
        set -euo pipefail

        REPORTER="${serviceName}"
        TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
        STATE_DIR="/var/lib/unheaded/${serviceName}"
        TOTAL_DEPS=${toString (builtins.length deps)}
        TOTAL_SERVICES=${totalServices}
        FAILED_COUNT=0
        FAILED_SERVICES=""
        ALL_RESULTS=""

        # ===================================================================
        # CHECK DEPENDENCIES
        # ===================================================================
        ${depChecks}

        # Remove leading comma
        ALL_RESULTS="''${ALL_RESULTS#,}"

        # ===================================================================
        # CALCULATE GAUNTLETS LAW SEVERITY
        # ===================================================================
        # Formula: (unique_reporters / total_dependent_services) * 100
        #
        # For local calculation, we use failed percentage of our deps.
        # The aggregate consensus is computed by cuirass from all reporters.
        # ===================================================================
        if [ "$TOTAL_DEPS" -gt 0 ]; then
          FAIL_PERCENT=$(( (FAILED_COUNT * 100) / TOTAL_DEPS ))
        else
          FAIL_PERCENT=0
        fi

        # Determine severity based on Gauntlets Law thresholds
        if [ "$FAIL_PERCENT" -lt 13 ]; then
          SEVERITY="OK"
          COLOR="#008000"
        elif [ "$FAIL_PERCENT" -lt 38 ]; then
          SEVERITY="WARN"
          COLOR="#fdda61"
        elif [ "$FAIL_PERCENT" -lt 63 ]; then
          SEVERITY="ERROR"
          COLOR="#ffff00"
        elif [ "$FAIL_PERCENT" -lt 88 ]; then
          SEVERITY="CRITICAL"
          COLOR="#ff5c00"
        else
          SEVERITY="PANIC"
          COLOR="#ff0000"
        fi

        # ===================================================================
        # BUILD REPORT
        # ===================================================================
        REPORT=$(${pkgs.jq}/bin/jq -n \
          --arg reporter "$REPORTER" \
          --arg timestamp "$TIMESTAMP" \
          --arg severity "$SEVERITY" \
          --arg color "$COLOR" \
          --argjson failed_count "$FAILED_COUNT" \
          --argjson total_deps "$TOTAL_DEPS" \
          --argjson fail_percent "$FAIL_PERCENT" \
          --arg failed_services "''${FAILED_SERVICES# }" \
          --arg results "$ALL_RESULTS" \
          '{
            reporter: $reporter,
            timestamp: $timestamp,
            severity: $severity,
            color: $color,
            failed_count: $failed_count,
            total_deps: $total_deps,
            fail_percent: $fail_percent,
            failed_services: ($failed_services | split(" ") | map(select(. != ""))),
            results: ($results | split(",") | map(
              capture("(?<name>[^:]+):(?<status>.+)")
            ))
          }')

        # ===================================================================
        # WRITE LOCAL STATE
        # ===================================================================
        echo "$REPORT" > "$STATE_DIR/health-report.json"

        # ===================================================================
        # PUBLISH TO WOTAN
        # ===================================================================
        # Only publish if there are failures (reduce noise)
        if [ "$FAILED_COUNT" -gt 0 ]; then
          curl -sf -X POST \
            -H "Content-Type: application/json" \
            -d "$REPORT" \
            "${wotanAddr}/api/v1/publish/system.outage.reports" \
            > /dev/null 2>&1 || true

          echo "[$SEVERITY] ${serviceName}: $FAILED_COUNT/$TOTAL_DEPS deps unhealthy ($FAIL_PERCENT%) -$FAILED_SERVICES"
        else
          echo "[OK] ${serviceName}: All $TOTAL_DEPS dependencies healthy"
        fi
      '';
      mode = "0755";
    };

    # =========================================================================
    # HEALTH MONITOR SYSTEMD SERVICE
    # =========================================================================
    systemd.services."unheaded-health-monitor" = {
      description = "Cross-Service Health Monitor for ${config.unheaded.healthMonitor.serviceName}";
      after = [ "${config.unheaded.healthMonitor.serviceName}.service" "network-online.target" ];
      wants = [ "network-online.target" ];

      serviceConfig = {
        Type = "oneshot";
        User = "unheaded";
        Group = "unheaded";
        ExecStart = "${pkgs.bash}/bin/bash /etc/unheaded/health/monitor.sh";
      };
    };

    # =========================================================================
    # HEALTH MONITOR TIMER
    # =========================================================================
    systemd.timers."unheaded-health-monitor" = {
      description = "Periodic Health Monitor for ${config.unheaded.healthMonitor.serviceName}";
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnCalendar = "*:*:0/30";    # Every 30 seconds
        Persistent = true;
        RandomizedDelaySec = "5s";  # Stagger checks across services
      };
    };
  };
}
