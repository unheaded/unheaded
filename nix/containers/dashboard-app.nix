{ config, pkgs, ... }:

{
  # =============================================================================
  # DASHBOARD APP CONTAINER
  # =============================================================================
  # Observability dashboard with eBPF trace visualization.
  #
  # App: Real-time packet flow visualization
  # IP: 10.10.10.201
  # Ports: 8081 (HTTP + WebSocket), 9100 (metrics)
  # Public: Yes (via gateway)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "dashboard-app";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/dashboard-app"
      "/var/log/unheaded/dashboard-app"
    ];
    allowedPorts = [ 8081 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.201";
    servicePort = 8081;
    allowDirectAccess = true;          # Gateway routes public traffic
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.dashboard-app = {
    description = "Dashboard App - eBPF Trace Visualization";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "wotan.service" ];
    wants = [ "network-online.target" ];
    requires = [ "wotan.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";

      # Service binary
      ExecStart = "${pkgs.dashboard-app}/bin/dashboard-app";

      WorkingDirectory = "/var/lib/unheaded/dashboard-app";

      Environment = [
        "DASHBOARD_ADDR=0.0.0.0:8081"
        "DASHBOARD_WOTAN_URL=http://10.10.10.10:8080"
        "DASHBOARD_WS_ENABLED=true"
      ];

      Restart = "always";
      RestartSec = "5s";

      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 2 && /etc/unheaded/health-check.sh'";

      KillSignal = "SIGTERM";
      TimeoutStopSec = "10s";

      # Resource limits (WebSocket connections)
      MemoryMax = "1G";
      CPUQuota = "150%";
    };
  };

  nixpkgs.config.packageOverrides = pkgs: {
    dashboard-app = pkgs.buildGoModule rec {
      pname = "dashboard-app";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/cmd/dashboard-backend;
      vendorHash = null;
      meta = {
        description = "Dashboard for eBPF trace visualization";
      };
    };
  };
}
