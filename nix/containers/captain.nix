# NixOS container definition for captain service
# Leadership and strategy service for Unheaded
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
    serviceName = "captain";
    allowedCapabilities = [ "CAP_NET_BIND_SERVICE" ];
    writablePaths = [
      "/var/lib/unheaded/captain"
      "/var/log/unheaded"
    ];
    allowedPorts = [ 8000 9100 ];
  };

  # ===========================================================================
  # NETWORKING CONFIGURATION
  # ===========================================================================
  unheaded.networking = {
    enable = true;
    serviceIP = "10.10.10.20";
    servicePort = 8000;
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

  # ===========================================================================
  # SERVICE DEFINITION
  # ===========================================================================
  systemd.services.captain = {
    description = "Captain Strategy Service";
    documentation = [ "https://github.com/unheaded/unheaded" ];

    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "busboy.service" ];
    wants = [ "network-online.target" ];
    requires = [ "busboy.service" ];

    # Service configuration
    serviceConfig = {
      # Start command
      ExecStart = "${pkgs.captain}/bin/captain";
      ExecReload = "/bin/kill -HUP $MAINPID";

      # Type
      Type = "simple";

      # User/group (shared unheaded user from common module)
      User = "unheaded";
      Group = "unheaded";

      # Restart policy
      Restart = "always";
      RestartSec = "5s";
      StartLimitBurst = 5;
      StartLimitIntervalSec = 60;

      # ========================================================================
      # SECURITY HARDENING
      # ========================================================================
      # These supplement the shared hardening module defaults.
      # The hardening.nix module provides: CapabilityBoundingSet,
      # NoNewPrivileges, ProtectSystem=strict, ProtectHome, PrivateTmp,
      # PrivateDevices, ProtectKernelTunables, ProtectControlGroups,
      # RestrictRealtime, RestrictNamespaces, SystemCallFilter, ReadOnlyPaths,
      # and all other baseline hardening properties.
      # ========================================================================

      # Additional per-service hardening (supplements module defaults)
      CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
      NoNewPrivileges = true;

      # Filesystem isolation
      PrivateTmp = true;
      ProtectSystem = "strict";
      ProtectHome = true;

      # Read-only paths
      ReadOnlyPaths = [ "/" ];

      # Read-write paths (minimal)
      ReadWritePaths = [
        "/var/lib/unheaded/captain"
        "/var/log/unheaded"
      ];

      # Seccomp - block dangerous syscalls
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

      # Device access
      PrivateDevices = true;
      DevicePolicy = "strict";

      # Kernel isolation
      ProtectKernelTunables = true;
      ProtectKernelModules = true;
      ProtectKernelLogs = true;
      ProtectControlGroups = true;
      ProtectClock = true;

      # Process restrictions
      RestrictRealtime = true;
      RestrictNamespaces = true;
      RestrictSUIDSGID = true;
      RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];

      # Memory protections
      MemoryDenyWriteExecute = true;
      LockPersonality = true;

      # Process isolation
      PrivateIPC = true;
      PrivateUsers = true;
      RemoveIPC = true;

      # Resource limits
      LimitNOFILE = 4096;
      LimitNPROC = 512;
      MemoryMax = "512M";
      CPUQuota = "100%";
      TasksMax = 512;

      # Umask for new files
      UMask = "0077";

      # Standard output/error
      StandardOutput = "journal";
      StandardError = "journal";
      SyslogIdentifier = "captain";

      # Graceful shutdown
      KillSignal = "SIGTERM";
      TimeoutStopSec = "10s";
    };

    # Environment variables
    environment = {
      BUSBOY_ADDR = "10.10.10.10:9090";
      HTTP_ADDR = "0.0.0.0:8000";
      DATA_PATH = "/var/lib/unheaded/captain";
      LOG_LEVEL = "info";
    };
  };

  # ============================================================================
  # LOGGING
  # ============================================================================
  services.journald.extraConfig = ''
    SystemMaxFileSize=10M
    RuntimeMaxFileSize=10M
  '';

  # ============================================================================
  # REQUIRED PACKAGES
  # ============================================================================
  environment.systemPackages = with pkgs; [
    captain # The service binary
    curl    # For health checks
  ];

  # Package definition (captain binary)
  systemd.packages = [ pkgs.captain ];
}
