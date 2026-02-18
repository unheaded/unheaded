{ config, pkgs, ... }:

{
  # =============================================================================
  # DEVELOPER SERVICE CONTAINER
  # =============================================================================
  # Development and coding service.
  #
  # Service: Development API (REST + Wotan)
  # IP: 10.10.10.24
  # Ports: 8004 (HTTP), 9100 (metrics)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "developer";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/opt/unheaded/references"
      "/var/lib/unheaded/developer"
      "/var/log/unheaded/developer"
    ];
    allowedPorts = [ 8004 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.24";
    servicePort = 8004;
    allowDirectAccess = true;
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.developer = {
    description = "Developer Coding Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "wotan.service" ];
    wants = [ "network-online.target" ];
    requires = [ "wotan.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.developer}/bin/developer";
      WorkingDirectory = "/opt/unheaded/references";
      Environment = [
        "DEVELOPER_ADDR=0.0.0.0:8004"
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
    developer = pkgs.buildGoModule rec {
      pname = "developer";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/services/developer;
      vendorHash = null;
    };
  };
}
