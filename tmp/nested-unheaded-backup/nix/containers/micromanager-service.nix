{ config, pkgs, ... }:

{
  # =============================================================================
  # MICROMANAGER SERVICE CONTAINER
  # =============================================================================
  # Execution tracking and task management service.
  #
  # Service: Execution API (REST + Busboy)
  # IP: 10.10.10.22
  # Ports: 8002 (HTTP), 9100 (metrics)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "micromanager";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/opt/unheaded/references"
      "/var/lib/unheaded/micromanager"
      "/var/log/unheaded/micromanager"
    ];
    allowedPorts = [ 8002 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.22";
    servicePort = 8002;
    allowDirectAccess = true;
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.micromanager = {
    description = "Micromanager Execution Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.micromanager}/bin/micromanager";
      WorkingDirectory = "/opt/unheaded/references";
      Environment = [
        "MICROMANAGER_ADDR=0.0.0.0:8002"
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
    micromanager = pkgs.buildGoModule rec {
      pname = "micromanager";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/services/micromanager;
      vendorHash = null;
    };
  };
}
