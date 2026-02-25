# nix/containers/timeguru.nix
{
  config,
  pkgs,
  ...
}:

{
  # Service definition
  systemd.services.timeguru = {
    description = "Timeguru Timeline Tracking Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "wotan.service" ];
    wants = [ "network-online.target" "wotan.service" ];

    serviceConfig = {
      ExecStart = "${pkgs.timeguru}/bin/timeguru";
      Restart = "always";
      RestartSec = "5s";

      # Security hardening
      CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      NoNewPrivileges = true;
      PrivateTmp = true;
      ProtectSystem = "strict";
      ProtectHome = true;
      ReadOnlyPaths = [ "/etc" "/usr" ];
      ReadWritePaths = [ "/opt/unheaded/references" ];

      # Seccomp
      SystemCallFilter = [ "@system-service" "~@privileged" ];
    };
  };

  # Networking
  networking.firewall.enable = true;
  networking.firewall.allowedTCPPorts = [ 19000 ];

  # Environment
  environment.systemPackages = [ pkgs.timeguru ];
}
