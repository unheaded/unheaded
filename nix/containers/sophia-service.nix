{ config, pkgs, ... }:

{
  # =============================================================================
  # SOPHIA SERVICE CONTAINER
  # =============================================================================
  # Divine wisdom and knowledge graph service.
  # Knowledge management, semantic search, inference, and decision support.
  #
  # Service: Sophia API (REST + Busboy)
  # IP: 10.10.10.26
  # Ports: 8007 (HTTP), 9100 (metrics)
  # =============================================================================

  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  unheaded.hardening = {
    enable = true;
    serviceName = "sophia";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/sophia"
      "/var/log/unheaded/sophia"
    ];
    allowedPorts = [ 8007 9100 ];
  };

  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.26";
    servicePort = 8007;
    allowDirectAccess = true;
  };

  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  systemd.services.sophia = {
    description = "Sophia Knowledge Graph Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    serviceConfig = {
      Type = "simple";
      User = "unheaded";
      Group = "unheaded";
      ExecStart = "${pkgs.sophia}/bin/sophia";
      WorkingDirectory = "/var/lib/unheaded/sophia";
      Environment = [
        "SOPHIA_ADDR=0.0.0.0:8007"
      ];
      Restart = "always";
      RestartSec = "5s";
      ExecStartPost = "${pkgs.bash}/bin/bash -c 'sleep 2 && /etc/unheaded/health-check.sh'";
      KillSignal = "SIGTERM";
      TimeoutStopSec = "10s";
      MemoryMax = "512M";
      CPUQuota = "100%";
    };
  };

  nixpkgs.config.packageOverrides = pkgs: {
    sophia = pkgs.buildGoModule rec {
      pname = "sophia";
      version = "0.1.0";
      src = /opt/unheaded/unheaded/cmd/sophia;
      vendorHash = null;
    };
  };
}
