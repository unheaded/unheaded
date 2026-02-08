{ config, pkgs, ... }:

{
  # =============================================================================
  # MONAD SERVICE CONTAINER
  # =============================================================================
  # Unified state management and operations service.
  # "The One" - All operations pass through the Monad.
  #
  # Service: Monad API (REST + Busboy)
  # IP: 10.10.10.27
  # Ports: 8006 (HTTP), 9100 (metrics)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "monad";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/monad"
      "/var/log/unheaded/monad"
    ];
    allowedPorts = [ 8006 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.27";
    servicePort = 8006;
    allowDirectAccess = true;
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.monad = {
    description = "Monad Unified State Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.monad}/bin/monad";
      WorkingDirectory = "/var/lib/unheaded/monad";
      Environment = [
        "MONAD_ADDR=0.0.0.0:8006"
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
    monad = pkgs.buildGoModule rec {
      pname = "monad";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/cmd/monad;
      vendorHash = null;
    };
  };
}
