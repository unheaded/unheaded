{ config, pkgs, ... }:

{
  # =============================================================================
  # TIMEGURU SERVICE CONTAINER
  # =============================================================================
  # Timeline tracking and milestone management service.
  #
  # Service: Timeline API (REST + Wotan)
  # IP: 10.10.10.20
  # Ports: 8000 (HTTP), 9100 (metrics)
  # Data: /opt/unheaded/references/timeline.md
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
    serviceName = "timeguru";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/opt/unheaded/references"       # Timeline files (MD/JSON/YAML)
      "/var/lib/unheaded/timeguru"     # State
      "/var/log/unheaded/timeguru"     # Logs
    ];
    allowedPorts = [ 8000 9100 ];
  };

  # ===========================================================================
  # NETWORKING CONFIGURATION
  # ===========================================================================
  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.20";
    servicePort = 8000;
    allowDirectAccess = true;          # Gateway needs access
  };

  # ===========================================================================
  # COMMON CONFIGURATION
  # ===========================================================================
  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  # ===========================================================================
  # TIMEGURU SERVICE DEFINITION
  # ===========================================================================
  systemd.services.timeguru = {
    description = "Timeguru Timeline Tracking Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "wotan.service" ];
    wants = [ "network-online.target" ];
    requires = [ "wotan.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";

      # Service binary
      ExecStart = "${pkgs.timeguru}/bin/timeguru";

      WorkingDirectory = "/opt/unheaded/references";

      Environment = [
        "TIMEGURU_ADDR=0.0.0.0:8000"
        "TIMEGURU_TIMELINE_PATH=/opt/unheaded/references/timeline.md"
        "TIMEGURU_AUTO_SYNC=true"
      ];

      Restart = "always";
      RestartSec = "5s";

      # Health check
      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 2 && /etc/unheaded/health-check.sh'";

      KillSignal = "SIGTERM";
      TimeoutStopSec = "10s";

      # Resource limits
      MemoryMax = "256M";
      CPUQuota = "100%";
    };
  };

  # ===========================================================================
  # PACKAGE BUILD
  # ===========================================================================
  nixpkgs.config.packageOverrides = pkgs: {
    timeguru = pkgs.buildGoModule rec {
      pname = "timeguru";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/services/timeguru;
      vendorHash = null;
      meta = {
        description = "Timeline tracking service for Unheaded";
      };
    };
  };
}
