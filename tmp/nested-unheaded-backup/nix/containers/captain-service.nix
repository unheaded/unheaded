{ config, pkgs, ... }:

{
  # =============================================================================
  # CAPTAIN SERVICE CONTAINER
  # =============================================================================
  # Strategic planning and leadership service.
  #
  # Service: Strategy API (REST + Busboy)
  # IP: 10.10.10.21
  # Ports: 8001 (HTTP), 9100 (metrics)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "captain";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/opt/unheaded/references"
      "/var/lib/unheaded/captain"
      "/var/log/unheaded/captain"
    ];
    allowedPorts = [ 8001 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.21";
    servicePort = 8001;
    allowDirectAccess = true;
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.captain = {
    description = "Captain Strategy Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.captain}/bin/captain";
      WorkingDirectory = "/opt/unheaded/references";
      Environment = [
        "CAPTAIN_ADDR=0.0.0.0:8001"
      ];
      Restart = "always";
      RestartSec = "5s";
      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 2 && /etc/unheaded/health-check.sh'";
      KillSignal = "SIGTERM";
      TimeoutStopSec = "10s";
      MemoryMax = "256M";
      CPUQuota = "100%";
    };
  };

  nixpkgs.config.packageOverrides = pkgs: {
    captain = pkgs.buildGoModule rec {
      pname = "captain";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/services/captain;
      vendorHash = null;
    };
  };
}
