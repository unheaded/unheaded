{ config, pkgs, ... }:

{
  # =============================================================================
  # ARCHITECT SERVICE CONTAINER
  # =============================================================================
  # Infrastructure design and architecture service.
  #
  # Service: Architecture API (REST + Busboy)
  # IP: 10.10.10.23
  # Ports: 8003 (HTTP), 9100 (metrics)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "architect";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/opt/unheaded/references"
      "/var/lib/unheaded/architect"
      "/var/log/unheaded/architect"
    ];
    allowedPorts = [ 8003 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.23";
    servicePort = 8003;
    allowDirectAccess = true;
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.architect = {
    description = "Architect Design Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.architect}/bin/architect";
      WorkingDirectory = "/opt/unheaded/references";
      Environment = [
        "ARCHITECT_ADDR=0.0.0.0:8003"
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
    architect = pkgs.buildGoModule rec {
      pname = "architect";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/services/architect;
      vendorHash = null;
    };
  };
}
