# NixOS container definition for architect service
# Includes hardening, security policies, and Wotan integration
{ config, pkgs, ... }:

{
  # System configuration
  system.stateVersion = "23.11";

  # Networking
  networking.firewall.enable = true;
  networking.firewall.allowedTCPPorts = [ 8001 9090 ];
  networking.firewall.extraCommands = ''
    # Allow internal container network
    iptables -A INPUT -s 10.10.10.0/24 -j ACCEPT
    # Drop everything else
    iptables -A INPUT -j DROP
  '';

  # Systemd service
  systemd.services.architect = {
    description = "Architect - Infrastructure Design Service";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" ];
    wants = [ "network-online.target" ];

    serviceConfig = {
      # Execution
      Type = "simple";
      ExecStart = "${pkgs.architect}/bin/architect -addr :8001 -wotan 10.10.10.10:9090 -log info";
      Restart = "always";
      RestartSec = "5s";

      # Security - Capabilities (minimal)
      CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
      NoNewPrivileges = true;

      # Security - Filesystem
      PrivateTmp = true;
      ProtectSystem = "strict";
      ProtectHome = true;
      ReadOnlyPaths = [ "/etc" "/usr" ];
      ReadWritePaths = [
        "/var/lib/architect"
        "/var/log/architect"
      ];

      # Security - Process isolation
      PrivateDevices = true;
      ProtectKernelTunables = true;
      ProtectControlGroups = true;
      ProtectKernelLogs = true;
      ProtectClock = true;
      RestrictRealtime = true;
      RestrictNamespaces = true;
      LockPersonality = true;
      MemoryDenyWriteExecute = true;
      RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
      SystemCallFilter = [
        "@system-service"
        "~@privileged"
        "~@resources"
        "~@raw-io"
      ];
      SystemCallErrorNumber = "EPERM";

      # User/group
      User = "architect";
      Group = "architect";

      # Resource limits
      CPUQuota = "100%";
      MemoryMax = "512M";
      TasksMax = 4096;

      # Logging
      StandardOutput = "journal";
      StandardError = "journal";
      SyslogIdentifier = "architect";
    };

    # Environment variables
    environment = {
      RUST_BACKTRACE = "1";
      RUST_LOG = "info";
    };

    # Pre-start setup (create directories)
    preStart = ''
      mkdir -p /var/lib/architect /var/log/architect
      chown architect:architect /var/lib/architect /var/log/architect
      chmod 700 /var/lib/architect /var/log/architect
    '';
  };

  # User and group
  users.users.architect = {
    isSystemUser = true;
    group = "architect";
    home = "/var/lib/architect";
    shell = "${pkgs.nologin}/bin/nologin";
    description = "Architect service user";
  };

  users.groups.architect = {};

  # Package definitions (placeholder - would be built elsewhere)
  environment.systemPackages = [ ];

  # Logging configuration
  services.journald = {
    extraConfig = ''
      SystemMaxUse=1G
      SystemMaxFileSize=100M
    '';
  };

  # Health check script
  systemd.services.architect-health = {
    description = "Architect Health Check";
    after = [ "architect.service" ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.curl}/bin/curl -f http://localhost:8001/health || exit 1";
    };
  };

  # Timer for periodic health checks
  systemd.timers.architect-health = {
    description = "Architect Health Check Timer";
    timerConfig = {
      OnBootSec = "30s";
      OnUnitActiveSec = "60s";
      Persistent = true;
    };
    wantedBy = [ "timers.target" ];
  };

  # Backup configuration (optional)
  systemd.services.architect-backup = {
    description = "Architect Data Backup";
    after = [ "architect.service" ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = ''
        ${pkgs.coreutils}/bin/tar \
          -czf /var/backups/architect-$(${pkgs.coreutils}/bin/date +%s).tar.gz \
          /var/lib/architect
      '';
      User = "root";
    };
  };

  systemd.timers.architect-backup = {
    description = "Architect Backup Timer";
    timerConfig = {
      OnBootSec = "1h";
      OnUnitActiveSec = "24h";
      Persistent = true;
    };
    wantedBy = [ "timers.target" ];
  };
}
