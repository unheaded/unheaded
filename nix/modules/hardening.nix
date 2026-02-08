{ config, lib, pkgs, ... }:

{
  # =============================================================================
  # UNHEADED CONTAINER HARDENING MODULE
  # =============================================================================
  # Security baseline applied to ALL containers.
  # Defense in depth: seccomp, capabilities, filesystem isolation, process restrictions.
  #
  # Design principles:
  # - Default deny everything
  # - Explicit allow minimum required
  # - No privilege escalation paths
  # - Immutable where possible
  # =============================================================================

  options = {
    unheaded.hardening = {
      enable = lib.mkEnableOption "Unheaded security hardening";

      serviceName = lib.mkOption {
        type = lib.types.str;
        description = "Service name for logging and audit";
      };

      allowedCapabilities = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ "CAP_NET_BIND_SERVICE" ];
        description = "Linux capabilities to grant (default: bind to ports <1024)";
      };

      writablePaths = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ ];
        description = "Paths that need write access (logs, data)";
      };

      allowedPorts = lib.mkOption {
        type = lib.types.listOf lib.types.int;
        default = [ ];
        description = "TCP ports this service binds to";
      };
    };
  };

  config = lib.mkIf config.unheaded.hardening.enable {
    # =========================================================================
    # SYSTEMD SERVICE HARDENING
    # =========================================================================
    # Applied to all Unheaded services via systemd unit configuration.
    # These settings are inherited by all services using this module.
    # =========================================================================

    systemd.services."${config.unheaded.hardening.serviceName}" = {
      serviceConfig = {
        # ---------------------------------------------------------------------
        # CAPABILITY RESTRICTIONS
        # ---------------------------------------------------------------------
        # Drop all capabilities by default, explicitly allow only what's needed.
        # Most services only need CAP_NET_BIND_SERVICE for ports <1024.
        # ---------------------------------------------------------------------
        CapabilityBoundingSet = config.unheaded.hardening.allowedCapabilities;
        AmbientCapabilities = config.unheaded.hardening.allowedCapabilities;

        # No privilege escalation via setuid/setgid
        NoNewPrivileges = true;

        # ---------------------------------------------------------------------
        # FILESYSTEM ISOLATION
        # ---------------------------------------------------------------------
        # Read-only root, private /tmp, minimal write access
        # ---------------------------------------------------------------------
        ProtectSystem = "strict";        # /usr /boot /etc read-only
        ProtectHome = true;               # No access to /home
        PrivateTmp = true;                # Private /tmp namespace
        PrivateDevices = true;            # Minimal /dev

        # Explicit write access only where needed
        ReadOnlyPaths = [ "/" ];
        ReadWritePaths = config.unheaded.hardening.writablePaths ++ [
          "/var/log/${config.unheaded.hardening.serviceName}"
          "/var/lib/${config.unheaded.hardening.serviceName}"
        ];

        # ---------------------------------------------------------------------
        # KERNEL PROTECTION
        # ---------------------------------------------------------------------
        # Prevent kernel exploitation and information leaks
        # ---------------------------------------------------------------------
        ProtectKernelTunables = true;     # /proc/sys /sys read-only
        ProtectKernelModules = true;      # Cannot load kernel modules
        ProtectKernelLogs = true;         # Cannot read dmesg
        ProtectControlGroups = true;      # cgroups read-only

        # ---------------------------------------------------------------------
        # PROCESS RESTRICTIONS
        # ---------------------------------------------------------------------
        RestrictRealtime = true;          # No realtime scheduling
        RestrictNamespaces = true;        # No new namespaces
        RestrictSUIDSGID = true;          # No setuid/setgid

        # Limit address families (network protocols)
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];

        # ---------------------------------------------------------------------
        # SECCOMP SYSCALL FILTER
        # ---------------------------------------------------------------------
        # Block dangerous syscalls. Allow only what's needed for network services.
        # @system-service = baseline set for network daemons
        # ~@privileged = block all privileged operations
        # ~@resources = block resource manipulation (mount, swap, etc)
        # ---------------------------------------------------------------------
        SystemCallFilter = [
          "@system-service"               # Network service baseline
          "~@privileged"                  # No CAP_SYS_ADMIN operations
          "~@resources"                   # No mount/swap/quota
          "~@obsolete"                    # No deprecated syscalls
          "~@debug"                       # No ptrace/perf
          "~@mount"                       # No filesystem operations
          "~@reboot"                      # No reboot/kexec
          "~@swap"                        # No swap operations
          "~@module"                      # No kernel modules
          "~@raw-io"                      # No direct I/O
        ];

        # Syscall architecture lock (prevent mixing 32/64-bit)
        SystemCallArchitectures = "native";

        # Error instead of kill on syscall violation (debugging)
        SystemCallErrorNumber = "EPERM";

        # ---------------------------------------------------------------------
        # MEMORY PROTECTION
        # ---------------------------------------------------------------------
        MemoryDenyWriteExecute = true;    # No RWX pages (JIT attacks)
        LockPersonality = true;           # Cannot change execution domain

        # ---------------------------------------------------------------------
        # PROCESS SECURITY
        # ---------------------------------------------------------------------
        PrivateUsers = true;              # User namespace isolation
        RemoveIPC = true;                 # Clean IPC on exit

        # ---------------------------------------------------------------------
        # RESOURCE LIMITS
        # ---------------------------------------------------------------------
        # Prevent resource exhaustion attacks
        # mkDefault allows container-specific overrides
        # ---------------------------------------------------------------------
        LimitNOFILE = lib.mkDefault 65536;   # File descriptors
        LimitNPROC = lib.mkDefault 512;      # Max processes
        TasksMax = lib.mkDefault 512;        # Max threads

        # ---------------------------------------------------------------------
        # RESTART POLICY
        # ---------------------------------------------------------------------
        # mkDefault allows container-specific overrides
        Restart = lib.mkDefault "always";
        RestartSec = lib.mkDefault "5s";
        StartLimitBurst = lib.mkDefault 5;
        StartLimitIntervalSec = lib.mkDefault 60;
      };
    };

    # =========================================================================
    # NETWORK FIREWALL
    # =========================================================================
    # Default deny with explicit allows
    # =========================================================================
    networking.firewall = {
      enable = true;
      allowedTCPPorts = config.unheaded.hardening.allowedPorts;

      # Default policy: DROP
      extraCommands = ''
        # Log dropped packets (rate limited)
        iptables -A INPUT -m limit --limit 5/min -j LOG --log-prefix "[${config.unheaded.hardening.serviceName}] DROP: "

        # Allow established connections
        iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

        # Allow loopback
        iptables -A INPUT -i lo -j ACCEPT

        # Allow from container network (10.10.10.0/24)
        iptables -A INPUT -s 10.10.10.0/24 -j ACCEPT

        # Drop everything else
        iptables -A INPUT -j DROP
      '';
    };

    # =========================================================================
    # SYSTEM HARDENING
    # =========================================================================
    # Kernel-level protections
    # =========================================================================
    boot.kernel.sysctl = {
      # Network security
      "net.ipv4.conf.all.rp_filter" = 1;              # Reverse path filtering
      "net.ipv4.conf.default.rp_filter" = 1;
      "net.ipv4.conf.all.accept_source_route" = 0;    # No source routing
      "net.ipv4.conf.default.accept_source_route" = 0;
      "net.ipv4.icmp_echo_ignore_broadcasts" = 1;     # Ignore broadcast pings
      "net.ipv4.icmp_ignore_bogus_error_responses" = 1;
      "net.ipv4.tcp_syncookies" = 1;                  # SYN flood protection

      # IPv6 security
      "net.ipv6.conf.all.accept_source_route" = 0;
      "net.ipv6.conf.default.accept_source_route" = 0;

      # Kernel hardening
      "kernel.dmesg_restrict" = 1;                    # Restrict dmesg
      "kernel.kptr_restrict" = 2;                     # Hide kernel pointers
      "kernel.yama.ptrace_scope" = 2;                 # Restrict ptrace
    };

    # =========================================================================
    # AUDIT LOGGING
    # =========================================================================
    # Track security events for SIEM integration
    # =========================================================================
    security.audit.enable = true;
    security.audit.rules = [
      # Monitor privilege escalation attempts
      "-a exit,always -F arch=b64 -S execve -F euid=0 -k privileged"

      # Monitor file access to sensitive paths
      "-w /etc/shadow -p wa -k auth"
      "-w /etc/passwd -p wa -k auth"

      # Monitor network configuration changes
      "-w /etc/sysconfig/network -p wa -k network"
    ];
  };
}
