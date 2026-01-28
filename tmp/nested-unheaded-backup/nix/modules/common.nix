{ config, lib, pkgs, ... }:

{
  # =============================================================================
  # UNHEADED COMMON MODULE
  # =============================================================================
  # Shared configuration for all Unheaded containers:
  # - Structured logging (JSON)
  # - Metrics collection (Prometheus)
  # - Time synchronization
  # - System packages
  # - Locale settings
  # =============================================================================

  options = {
    unheaded.common = {
      enable = lib.mkEnableOption "Unheaded common configuration";

      logLevel = lib.mkOption {
        type = lib.types.enum [ "debug" "info" "warn" "error" ];
        default = "info";
        description = "Default log level for services";
      };

      enableMetrics = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = "Enable Prometheus metrics endpoint";
      };

      metricsPort = lib.mkOption {
        type = lib.types.int;
        default = 9100;
        description = "Prometheus metrics port";
      };
    };
  };

  config = lib.mkIf config.unheaded.common.enable {
    # =========================================================================
    # SYSTEM CONFIGURATION
    # =========================================================================
    system.stateVersion = "24.05";

    # Locale and timezone
    time.timeZone = "UTC";
    i18n.defaultLocale = "en_US.UTF-8";

    # =========================================================================
    # ESSENTIAL PACKAGES
    # =========================================================================
    # Minimal set for debugging and operations
    # =========================================================================
    environment.systemPackages = with pkgs; [
      # Networking tools
      curl
      wget
      dnsutils
      iproute2
      iptables
      netcat
      tcpdump

      # System tools
      htop
      lsof
      strace
      procps
      psmisc

      # File tools
      file
      tree
      less
      vim

      # JSON/YAML parsing
      jq
      yq-go

      # TLS tools
      openssl

      # Compression
      gzip
      bzip2
      xz
    ];

    # =========================================================================
    # LOGGING CONFIGURATION
    # =========================================================================
    # Structured JSON logs via journald
    # =========================================================================
    services.journald = {
      extraConfig = ''
        # Persistent storage
        Storage=persistent

        # Rotation policy
        SystemMaxUse=1G
        SystemMaxFileSize=100M
        SystemMaxFiles=10

        # Compression
        Compress=yes

        # Forward to syslog socket for external aggregation
        ForwardToSyslog=yes

        # Rate limiting (prevent log floods)
        RateLimitInterval=30s
        RateLimitBurst=10000
      '';
    };

    # Rsyslog for forwarding to central log aggregator
    services.rsyslog = {
      enable = true;
      extraConfig = ''
        # JSON template for structured logs
        template(name="json-template" type="list") {
          constant(value="{")
          constant(value="\"timestamp\":\"")      property(name="timereported" dateFormat="rfc3339")
          constant(value="\",\"host\":\"")        property(name="hostname")
          constant(value="\",\"service\":\"")     property(name="programname")
          constant(value="\",\"severity\":\"")    property(name="syslogseverity-text")
          constant(value="\",\"message\":\"")     property(name="msg" format="json")
          constant(value="\"}\n")
        }

        # Forward to log aggregator (TODO: configure in production)
        # *.* action(type="omfwd" target="log-aggregator.internal" port="514" protocol="tcp" template="json-template")

        # Local file for debugging
        *.* /var/log/unheaded/${config.unheaded.hardening.serviceName}.json;json-template
      '';
    };

    # =========================================================================
    # METRICS CONFIGURATION
    # =========================================================================
    # Node exporter for system metrics
    # =========================================================================
    services.prometheus.exporters.node = lib.mkIf config.unheaded.common.enableMetrics {
      enable = true;
      port = config.unheaded.common.metricsPort;
      enabledCollectors = [
        "systemd"               # Service status
        "processes"             # Process stats
        "cpu"                   # CPU metrics
        "meminfo"               # Memory stats
        "diskstats"             # Disk I/O
        "filesystem"            # Filesystem usage
        "netdev"                # Network devices
        "netstat"               # Network stats
        "loadavg"               # Load average
      ];

      # Only accessible from internal network
      listenAddress = "0.0.0.0";
      firewallFilter = "-s 10.10.10.0/24 -p tcp -m tcp --dport ${toString config.unheaded.common.metricsPort}";
    };

    # =========================================================================
    # TIME SYNCHRONIZATION
    # =========================================================================
    # Critical for distributed tracing and log correlation
    # =========================================================================
    services.timesyncd = {
      enable = true;
      servers = [
        "time.cloudflare.com"
        "time.google.com"
        "pool.ntp.org"
      ];
    };

    # =========================================================================
    # ENVIRONMENT VARIABLES
    # =========================================================================
    # Standard across all Unheaded services
    # =========================================================================
    environment.variables = {
      # Log configuration
      UNHEADED_LOG_LEVEL = config.unheaded.common.logLevel;
      UNHEADED_LOG_FORMAT = "json";

      # Metrics
      UNHEADED_METRICS_ENABLED = if config.unheaded.common.enableMetrics then "true" else "false";
      UNHEADED_METRICS_PORT = toString config.unheaded.common.metricsPort;

      # Go runtime tuning
      GOGC = "100";                     # GC trigger percentage
      GOMAXPROCS = "4";                 # Max CPU cores (adjust per container)
      GOMEMLIMIT = "512MiB";            # Memory limit (adjust per container)

      # Rust runtime tuning
      RUST_BACKTRACE = "1";             # Enable backtraces
      RUST_LOG = config.unheaded.common.logLevel;  # Rust log level
    };

    # =========================================================================
    # USER MANAGEMENT
    # =========================================================================
    # Standard non-root user for service processes
    # =========================================================================
    users.users.unheaded = {
      isSystemUser = true;
      group = "unheaded";
      description = "Unheaded service user";
      home = "/var/lib/unheaded";
      createHome = true;
      uid = 1000;
    };

    users.groups.unheaded = {
      gid = 1000;
    };

    # =========================================================================
    # DIRECTORY STRUCTURE
    # =========================================================================
    # Standard paths for all services
    # =========================================================================
    systemd.tmpfiles.rules = [
      # Service directories
      "d /var/lib/unheaded 0750 unheaded unheaded -"
      "d /var/lib/unheaded/${config.unheaded.hardening.serviceName} 0750 unheaded unheaded -"

      # Log directories
      "d /var/log/unheaded 0755 unheaded unheaded -"
      "d /var/log/unheaded/${config.unheaded.hardening.serviceName} 0755 unheaded unheaded -"

      # Config directories
      "d /etc/unheaded 0755 root root -"
      "d /etc/unheaded/${config.unheaded.hardening.serviceName} 0755 root root -"
    ];

    # =========================================================================
    # HEALTH CHECK SCRIPT
    # =========================================================================
    # Standard health check for all services
    # =========================================================================
    environment.etc."unheaded/health-check.sh" = {
      text = ''
        #!/usr/bin/env bash
        set -euo pipefail

        SERVICE_PORT="''${UNHEADED_SERVICE_PORT:-8000}"
        HEALTH_ENDPOINT="http://localhost:''${SERVICE_PORT}/health"

        # Attempt health check
        if curl -sf "''${HEALTH_ENDPOINT}" > /dev/null 2>&1; then
          echo "OK"
          exit 0
        else
          echo "FAIL"
          exit 1
        fi
      '';
      mode = "0755";
    };
  };
}
