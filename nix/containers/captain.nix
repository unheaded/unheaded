# NixOS container definition for captain service
# Leadership and strategy service for Unheaded
{ config, pkgs, ... }:

{
  # Service definition
  systemd.services.captain = {
    description = "Captain Strategy Service";
    documentation = [ "https://github.com/unheaded/unheaded" ];

    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "wotan.service" ];
    wants = [ "network-online.target" "wotan.service" ];

    # Service configuration
    serviceConfig = {
      # Start command
      ExecStart = "${pkgs.captain}/bin/captain";
      ExecReload = "/bin/kill -HUP $MAINPID";

      # Restart policy
      Restart = "always";
      RestartSec = "5s";
      StartLimitInterval = "60s";
      StartLimitBurst = 5;

      # User/group
      User = "captain";
      Group = "captain";
      DynamicUser = true;

      # ============================================================================
      # SECURITY HARDENING
      # ============================================================================

      # Capabilities - only allow network binding
      CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];

      # No privilege escalation
      NoNewPrivileges = true;

      # Filesystem isolation
      PrivateTmp = true;
      ProtectSystem = "strict";
      ProtectHome = true;
      ProtectRootFilesystem = true;

      # Read-only paths
      ReadOnlyPaths = [
        "/etc"
        "/usr"
        "/sys/kernel/security"
      ];

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
      ];

      # Device access
      PrivateDevices = true;
      DevicePolicy = "strict";
      DeviceAllow = [ "" ];

      # Namespace isolation
      PrivateNetwork = false; # Need network for Wotan communication
      PrivateIPC = true;
      NoNewPrivileges = true;

      # Kernel isolation
      ProtectKernelTunables = true;
      ProtectKernelModules = true;
      ProtectControlGroups = true;
      ProtectClock = true;
      RestrictRealtime = true;
      RestrictNamespaces = true;
      LockPersonality = true;

      # Memory protections
      MemoryDenyWriteExecute = true;
      RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];

      # Resource limits
      LimitNOFILE = 4096;
      LimitNPROC = 512;
      LimitMEMLOCK = "infinity";

      # Umask for new files
      UMask = "0077";

      # Type
      Type = "simple";

      # Standard output/error
      StandardOutput = "journal";
      StandardError = "journal";
      SyslogIdentifier = "captain";
    };

    # Environment variables
    environment = {
      WOTAN_ADDR = "wotan:9090";
      HTTP_ADDR = "0.0.0.0:8000";
      DATA_PATH = "/var/lib/unheaded/captain";
      LOG_LEVEL = "info";
    };

    # Prestart hooks
    preStart = ''
      # Create data directories with secure permissions
      mkdir -p /var/lib/unheaded/captain
      mkdir -p /var/log/unheaded
      chmod 0700 /var/lib/unheaded/captain
      chmod 0755 /var/log/unheaded
    '';
  };

  # User/group creation
  users.users.captain = {
    isSystemUser = true;
    group = "captain";
    home = "/var/lib/unheaded/captain";
    createHome = true;
    shell = "/sbin/nologin";
  };

  users.groups.captain = { };

  # Data directory persistence
  systemd.tmpfiles.rules = [
    "d /var/lib/unheaded/captain 0700 captain captain -"
    "d /var/log/unheaded 0755 captain captain -"
  ];

  # ============================================================================
  # NETWORKING
  # ============================================================================

  networking.firewall = {
    enable = true;

    # Explicit allow for service ports
    allowedTCPPorts = [ 8000 ];

    # Internal container network rules
    extraCommands = ''
      # Allow traffic from wotan on internal network
      iptables -A INPUT -s 10.10.10.10/32 -d 10.10.10.20/32 -p tcp --dport 8000 -j ACCEPT

      # Allow localhost
      iptables -A INPUT -s 127.0.0.1/8 -j ACCEPT
      iptables -A INPUT -s ::1/128 -j ACCEPT

      # Allow from gateway
      iptables -A INPUT -s 10.10.10.100/32 -p tcp --dport 8000 -j ACCEPT

      # Drop all else
      iptables -A INPUT -j DROP
    '';

    extraStopCommands = ''
      iptables -F INPUT || true
    '';
  };

  # ============================================================================
  # LOGGING
  # ============================================================================

  services.journald.extraConfig = ''
    SystemMaxFileSize=10M
    RuntimeMaxFileSize=10M
  '';

  # ============================================================================
  # MONITORING
  # ============================================================================

  # Prometheus node exporter (optional, for metrics collection)
  services.prometheus.exporters.node = {
    enable = false; # Can be enabled in production
    openFirewall = false;
  };

  # ============================================================================
  # KERNEL HARDENING
  # ============================================================================

  boot.kernelParams = [
    # Restrict kernel module loading
    "lockdown=integrity"

    # Restrict access to kernel logs
    "kernel.printk=3 3 3 3"

    # Restrict kexec
    "kernel.kexec_load_disabled=1"
  ];

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
