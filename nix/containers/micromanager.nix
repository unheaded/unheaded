# NixOS container definition for micromanager service
# Task execution and tracking service for Unheaded
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
    serviceName = "micromanager";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/micromanager"
      "/var/log/micromanager"
      "/run/micromanager"
    ];
    allowedPorts = [ 8003 9100 ];
  };

  # ===========================================================================
  # NETWORKING CONFIGURATION
  # ===========================================================================
  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.22";
    servicePort = 8003;
    allowDirectAccess = false;
  };

  # ===========================================================================
  # COMMON CONFIGURATION
  # ===========================================================================
  unheaded.common = {
    enable = true;
    logLevel = "info";
    enableMetrics = true;
  };

  # systemd service for micromanager
  systemd.services.micromanager = {
    description = "Unheaded Micromanager Service - Task Execution & Tracking";
    documentation = [ "https://github.com/unheaded/unheaded" ];

    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    # Service configuration
    serviceConfig = {
      # Execute
      Type = "simple";
      ExecStart = "${pkgs.micromanager}/bin/micromanager -port 8003 -busboy busboy:9090 -log-level info";
      Restart = "always";
      RestartSec = "5s";
      StartLimitBurst = 5;
      StartLimitIntervalSec = 60;

      # User & group
      User = "micromanager";
      Group = "micromanager";

      # ========================================================================
      # SECURITY HARDENING (all 12 required properties)
      # ========================================================================

      # 1. Capabilities - minimum required only
      CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];

      # 2. No privilege escalation
      NoNewPrivileges = true;

      # 3. Private /tmp
      PrivateTmp = true;

      # 4. Read-only system
      ProtectSystem = "strict";

      # 5. Protect home directories
      ProtectHome = true;

      # 6. Read-only paths (root filesystem)
      ReadOnlyPaths = [ "/" ];

      # Read-write paths (minimal)
      ReadWritePaths = [
        "/var/lib/unheaded/micromanager"
        "/var/log/micromanager"
        "/run/micromanager"
      ];

      # 7. Seccomp - block dangerous syscalls (full set per CLAUDE.md)
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

      # 8. Private devices
      PrivateDevices = true;

      # 9. Protect kernel tunables
      ProtectKernelTunables = true;
      ProtectKernelModules = true;
      ProtectKernelLogs = true;

      # 10. Protect control groups
      ProtectControlGroups = true;

      # 11. Restrict realtime
      RestrictRealtime = true;

      # 12. Restrict namespaces
      RestrictNamespaces = true;

      # Additional hardening (beyond the 12 required)
      RestrictSUIDSGID = true;
      RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
      RemoveIPC = true;
      LockPersonality = true;
      MemoryDenyWriteExecute = true;
      PrivateUsers = true;

      # Resource limits
      MemoryMax = "512M";
      TasksMax = 512;
      CPUQuota = "100%";
      LimitNOFILE = 65536;
      LimitNPROC = 512;

      # Environment
      Environment = [
        "LOG_LEVEL=info"
      ];

      # Timeout for shutdown
      KillSignal = "SIGTERM";
      TimeoutStopSec = "30s";
      KillMode = "mixed";
    };

    # Pre-start hook to create directories
    preStart = ''
      mkdir -p /var/lib/unheaded/micromanager
      mkdir -p /var/log/micromanager
      mkdir -p /run/micromanager
      chown -R micromanager:micromanager /var/lib/unheaded/micromanager
      chown -R micromanager:micromanager /var/log/micromanager
      chown -R micromanager:micromanager /run/micromanager
      chmod 700 /var/lib/unheaded/micromanager
      chmod 700 /var/log/micromanager
      chmod 700 /run/micromanager
    '';
  };

  # User and group
  users.users.micromanager = {
    description = "Micromanager service user";
    isSystemUser = true;
    group = "micromanager";
    home = "/var/lib/unheaded/micromanager";
    shell = pkgs.nologin;
  };

  users.groups.micromanager = {};

  # Networking - firewall configuration (default-deny)
  networking.firewall = {
    enable = true;
    allowedTCPPorts = [ 8003 ];
    # Allow internal container network only
    extraCommands = ''
      # Allow from internal bridge
      iptables -A INPUT -s 10.10.10.0/24 -d 10.10.10.22 -p tcp --dport 8003 -j ACCEPT
      # Allow loopback
      iptables -A INPUT -i lo -j ACCEPT
      # Drop everything else
      iptables -A INPUT -j DROP
    '';
  };

  # Health check
  systemd.timers.micromanager-health-check = {
    description = "Periodic health check for micromanager";
    wantedBy = [ "timers.target" ];
    timerConfig = {
      OnBootSec = "30s";
      OnUnitActiveSec = "30s";
    };
  };

  systemd.services.micromanager-health-check = {
    description = "Health check for micromanager";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = ''
        ${pkgs.curl}/bin/curl -f http://localhost:8003/health || \
        systemctl restart micromanager
      '';
    };
  };

  # Logging
  services.journald.extraConfig = ''
    SystemMaxUse=1G
    SystemMaxFileSize=100M
  '';

  # Environment
  environment.systemPackages = with pkgs; [
    micromanager
    curl       # For health checks
    jq         # For JSON parsing
  ];

  # Build package (this is defined in parent flake)
  # The actual micromanager package comes from the parent Nix flake
}
