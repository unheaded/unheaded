{ config, pkgs, ... }:

{
  # =============================================================================
  # BUSBOY MESSAGE BUS CONTAINER
  # =============================================================================
  # The heart of Unheaded - all services communicate through Busboy.
  #
  # Service: Message bus (gRPC + REST)
  # IP: 10.10.10.10
  # Ports: 9090 (gRPC), 8080 (REST), 9100 (metrics)
  # Critical: All services depend on this
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  # ===========================================================================
  # HARDENING CONFIGURATION
  # ===========================================================================
  unheaded.hardening = {
    enable = true;
    serviceName = "busboy";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/busboy"       # Ring buffer persistence
      "/var/log/unheaded/busboy"       # Logs
    ];
    allowedPorts = [ 9090 8080 9100 ];
  };

  # ===========================================================================
  # NETWORKING CONFIGURATION
  # ===========================================================================
  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.10";
    servicePort = 9090;                # gRPC primary
    allowDirectAccess = true;          # Gateway + all services need access
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
  # BUSBOY SERVICE DEFINITION
  # ===========================================================================
  systemd.services.busboy = {
    description = "Busboy Message Bus - gRPC and REST API";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" ];
    wants = [ "network-online.target" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";

      # ALPHA: Uses pre-built binary. Build from Go source (services/busboy/) planned for Age 2. See docs/SERVICE_BREAKOUT_STRATEGY.md
      ExecStart = "${pkgs.busboy}/bin/busboy --grpc-addr=0.0.0.0:9090 --http-addr=0.0.0.0:8080 --metrics-addr=0.0.0.0:9100";

      # Working directory
      WorkingDirectory = "/var/lib/unheaded/busboy";

      # Environment
      Environment = [
        "BUSBOY_DATA_DIR=/var/lib/unheaded/busboy"
        "BUSBOY_LOG_LEVEL=info"
        "BUSBOY_RING_BUFFER_SIZE=10000"
        "BUSBOY_MAX_SUBSCRIBERS=1000"
      ];

      # Restart policy (critical service)
      Restart = "always";
      RestartSec = "5s";
      StartLimitBurst = 10;            # Allow more retries (critical)
      StartLimitIntervalSec = 60;

      # Health check
      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 2 && /etc/unheaded/health-check.sh'";

      # Graceful shutdown
      KillSignal = "SIGTERM";
      TimeoutStopSec = "30s";

      # Resource limits (high priority service)
      MemoryMax = "1G";
      CPUQuota = "200%";               # Allow 2 cores
      TasksMax = 1024;                 # More threads for concurrent connections
    };
  };

  # ===========================================================================
  # ADDITIONAL FIREWALL RULES
  # ===========================================================================
  # Busboy needs both gRPC and REST accessible
  # ===========================================================================
  networking.firewall.allowedTCPPorts = [ 9090 8080 9100 ];

  # ===========================================================================
  # PACKAGE BUILD
  # ===========================================================================
  # ALPHA: Uses pre-built binary. Build from Go source (services/busboy/) planned for Age 2. See docs/SERVICE_BREAKOUT_STRATEGY.md
  # For now, placeholder package
  # ===========================================================================
  nixpkgs.config.packageOverrides = pkgs: {
    busboy = pkgs.buildGoModule rec {
      pname = "busboy";
      version = "1.0.0";

      # Source from Unheaded repo (will be overridden in flake)
      src = /opt/unheaded/busboy;

      vendorHash = null;  # Will be computed

      meta = {
        description = "Busboy message bus for Unheaded";
        license = pkgs.lib.licenses.mit;
      };
    };
  };
}
