{ config, pkgs, ... }:

{
  # systemd service for micromanager
  systemd.services.micromanager = {
    description = "Unheaded Micromanager Service - Task Execution & Tracking";
    documentation = [ "https://github.com/unheaded/unheaded" ];

    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" "wotan.service" ];
    wants = [ "network-online.target" "wotan.service" ];

    # Service configuration
    serviceConfig = {
      # Execute
      Type = "simple";
      ExecStart = "${pkgs.micromanager}/bin/micromanager -port 19003 -wotan wotan:18001 -log-level info";
      Restart = "always";
      RestartSec = "5s";
      StartLimitInterval = "60s";
      StartLimitBurst = 3;

      # User & group
      User = "micromanager";
      Group = "micromanager";

      # Security hardening
      NoNewPrivileges = true;
      PrivateTmp = true;
      ProtectSystem = "strict";
      ProtectHome = true;
      ProtectKernelTunables = true;
      ProtectKernelModules = true;
      ProtectControlGroups = true;
      PrivateDevices = true;
      RestrictRealtime = true;
      RestrictNamespaces = true;
      RestrictSUIDSGID = true;
      RemoveIPC = true;
      LockPersonality = true;

      # Capabilities
      CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
      AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];

      # Filesystem
      ReadOnlyPaths = [ "/etc" "/usr" ];
      ReadWritePaths = [
        "/var/lib/unheaded/micromanager"
        "/var/log/micromanager"
        "/run/micromanager"
      ];

      # System call filtering (seccomp)
      SystemCallFilter = [
        "@system-service"
        "~@privileged"
        "~@resources"
      ];
      SystemCallErrorNumber = "EPERM";

      # Resource limits
      MemoryLimit = "512M";
      TasksMax = 256;
      CPUQuota = "50%";

      # Environment
      Environment = [
        "LOG_LEVEL=info"
      ];

      # Timeout for shutdown
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

  # Networking - firewall configuration
  networking.firewall = {
    enable = true;
    # Allow internal container network only
    extraCommands = ''
      # Allow from internal bridge
      iptables -A INPUT -s 10.10.10.0/24 -d 10.10.10.21 -p tcp --dport 19003 -j ACCEPT
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
        ${pkgs.curl}/bin/curl -f http://localhost:19003/health || \
        systemctl restart micromanager
      '';
    };
  };

  # Logging
  services.rsyslog.enable = true;
  logging.targets.syslog.enable = true;

  # Environment
  environment.systemPackages = with pkgs; [
    micromanager
    curl       # For health checks
    jq         # For JSON parsing
  ];

  # Build package (this is defined in parent flake)
  # The actual micromanager package comes from the parent Nix flake
}
