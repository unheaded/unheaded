{ config, pkgs, ... }:

{
  # =============================================================================
  # KANBAN APP CONTAINER
  # =============================================================================
  # The Meta Moment - Unheaded building Unheaded.
  #
  # App: Kanban board displaying timeline from timeguru
  # IP: 10.10.10.200
  # Ports: 8080 (HTTP), 9100 (metrics)
  # Public: Yes (via gateway)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "kanban-app";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/kanban-app"
      "/var/log/unheaded/kanban-app"
    ];
    allowedPorts = [ 8080 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.200";
    servicePort = 8080;
    allowDirectAccess = true;          # Gateway routes public traffic
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.kanban-app = {
    description = "Kanban App - The Meta Moment";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "timeguru.service" ];
    wants = [ "network-online.target" "timeguru.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";

      # Service binary
      ExecStart = "${pkgs.kanban-app}/bin/kanban-app";

      WorkingDirectory = "/var/lib/unheaded/kanban-app";

      Environment = [
        "KANBAN_ADDR=0.0.0.0:8080"
        "KANBAN_TIMEGURU_URL=http://10.10.10.20:8000"
        "KANBAN_PUBLIC=true"
      ];

      Restart = "always";
      RestartSec = "5s";

      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 2 && /etc/unheaded/health-check.sh'";

      KillSignal = "SIGTERM";
      TimeoutStopSec = "10s";

      # Resource limits
      MemoryMax = "512M";
      CPUQuota = "100%";
    };
  };

  nixpkgs.config.packageOverrides = pkgs: {
    kanban-app = pkgs.buildGoModule rec {
      pname = "kanban-app";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/cmd/kanban-app;
      vendorHash = null;
      meta = {
        description = "Kanban board for Unheaded meta moment";
      };
    };
  };
}
