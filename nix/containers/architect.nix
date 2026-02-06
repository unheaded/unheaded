# NixOS container definition for architect service
# Includes hardening, security policies, and Busboy integration
#
# Refactored: Imports shared hardening, common, and networking modules
# to ensure all required security properties are applied consistently.
{ config, pkgs, ... }:

{
  # ===========================================================================
  # SHARED MODULE IMPORTS
  # ===========================================================================
  # All containers MUST import these modules for consistent hardening.
  # ===========================================================================
  imports = [
    ../modules/common.nix
    ../modules/hardening.nix
    ../modules/networking.nix
  ];

  # ===========================================================================
  # HARDENING CONFIGURATION
  # ===========================================================================
  unheaded.hardening = {
    enable = true;
    serviceName = "architect";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/architect"
      "/var/log/architect"
    ];
    allowedPorts = [ 8001 9090 9100 ];
  };

  # ===========================================================================
  # NETWORKING CONFIGURATION
  # ===========================================================================
  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.23";
    servicePort = 8001;
    allowDirectAccess = true;
  };

  # ===========================================================================
  # COMMON CONFIGURATION
  # ===========================================================================
  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  # Networking - default-deny firewall
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
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    serviceConfig = {
      # Execution
      Type = "simple";
      ExecStart = "${pkgs.architect}/bin/architect -addr :8001 -busboy 10.10.10.10:9090 -log info";
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
      ReadOnlyPaths = [ "/" ];
      ReadWritePaths = [
        "/var/lib/architect"
        "/var/log/architect"
      ];

      # Security - Process isolation
      PrivateDevices = true;
      ProtectKernelTunables = true;
      ProtectKernelModules = true;
      ProtectKernelLogs = true;
      ProtectControlGroups = true;
      ProtectClock = true;
      RestrictRealtime = true;
      RestrictNamespaces = true;
      RestrictSUIDSGID = true;
      RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
      LockPersonality = true;
      MemoryDenyWriteExecute = true;
      PrivateUsers = true;
      RemoveIPC = true;

      # Seccomp - block dangerous syscalls (full set per CLAUDE.md)
      SystemCallFilter = [
        "@system-service"
        "~@privileged"
        "~@resources"
        "~@obsolete"
        "~@debug"
        "~@mount"
        "~@reboot"
        "~@swap"
        "~@module"
        "~@raw-io"
      ];
      SystemCallArchitectures = "native";
      SystemCallErrorNumber = "EPERM";

      # User/group
      User = "architect";
      Group = "architect";

      # Resource limits
      CPUQuota = "100%";
      MemoryMax = "512M";
      TasksMax = 512;
      LimitNOFILE = 65536;
      LimitNPROC = 512;

      # Logging
      StandardOutput = "journal";
      StandardError = "journal";
      SyslogIdentifier = "architect";

      # Graceful shutdown
      KillSignal = "SIGTERM";
      TimeoutStopSec = "10s";
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
