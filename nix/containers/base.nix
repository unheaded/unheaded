{ config, lib, pkgs, ... }:

{
  # =============================================================================
  # UNHEADED BASE CONTAINER MODULE
  # =============================================================================
  # Foundational module for all Unheaded NixOS containers.
  # Provides security hardening, health checks, and network policies.
  #
  # Features:
  #   - Minimal NixOS base configuration
  #   - Security hardening (seccomp, capabilities, read-only FS)
  #   - Health check infrastructure
  #   - Default-deny network policies
  #   - Structured logging
  #
  # Usage:
  #   Import this module in every container definition:
  #     imports = [ ./base.nix ];
  # =============================================================================

  options = {
    unheaded.container = {
      enable = lib.mkEnableOption "Unheaded base container configuration";

      name = lib.mkOption {
        type = lib.types.str;
        description = "Container name for identification and logging";
      };

      # =========================================================================
      # SECURITY OPTIONS
      # =========================================================================
      security = {
        capabilities = lib.mkOption {
          type = lib.types.listOf lib.types.str;
          default = [ ];
          description = ''
            Linux capabilities to grant to the container service.
            Default is empty (no capabilities).
            Common values: CAP_NET_BIND_SERVICE (ports <1024)
          '';
        };

        readOnlyRoot = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = "Mount root filesystem as read-only";
        };

        writablePaths = lib.mkOption {
          type = lib.types.listOf lib.types.str;
          default = [ ];
          description = "Paths that need write access (in addition to standard paths)";
        };

        seccompProfile = lib.mkOption {
          type = lib.types.enum [ "strict" "default" "disabled" ];
          default = "strict";
          description = ''
            Seccomp filtering level:
            - strict: Block dangerous syscalls (recommended)
            - default: Basic syscall filtering
            - disabled: No syscall filtering (not recommended)
          '';
        };

        noNewPrivileges = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = "Prevent privilege escalation via setuid/setgid binaries";
        };
      };

      # =========================================================================
      # NETWORKING OPTIONS
      # =========================================================================
      network = {
        ip = lib.mkOption {
          type = lib.types.str;
          description = "Static IP address for this container (10.10.10.x/24)";
        };

        ports = lib.mkOption {
          type = lib.types.listOf lib.types.int;
          default = [ ];
          description = "TCP ports to allow incoming connections on";
        };

        allowedSources = lib.mkOption {
          type = lib.types.listOf lib.types.str;
          default = [ "10.10.10.0/24" ];
          description = "CIDR blocks allowed to connect to this container";
        };

        allowBusboyAccess = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = "Allow outbound connections to Busboy message bus";
        };

        busboyAddress = lib.mkOption {
          type = lib.types.str;
          default = "10.10.10.10";
          description = "Busboy service IP address";
        };

        busboyPort = lib.mkOption {
          type = lib.types.int;
          default = 9090;
          description = "Busboy gRPC port";
        };
      };

      # =========================================================================
      # HEALTH CHECK OPTIONS
      # =========================================================================
      healthCheck = {
        enable = lib.mkOption {
          type = lib.types.bool;
          default = true;
          description = "Enable health check endpoint monitoring";
        };

        port = lib.mkOption {
          type = lib.types.int;
          default = 8000;
          description = "Port for health check endpoint";
        };

        path = lib.mkOption {
          type = lib.types.str;
          default = "/health";
          description = "HTTP path for health check";
        };

        interval = lib.mkOption {
          type = lib.types.str;
          default = "30s";
          description = "Interval between health checks";
        };

        timeout = lib.mkOption {
          type = lib.types.str;
          default = "5s";
          description = "Timeout for health check requests";
        };
      };

      # =========================================================================
      # RESOURCE LIMITS
      # =========================================================================
      resources = {
        memoryMax = lib.mkOption {
          type = lib.types.str;
          default = "512M";
          description = "Maximum memory limit for the container service";
        };

        cpuQuota = lib.mkOption {
          type = lib.types.str;
          default = "100%";
          description = "CPU quota (100% = 1 core)";
        };

        tasksMax = lib.mkOption {
          type = lib.types.int;
          default = 512;
          description = "Maximum number of tasks/threads";
        };

        nofileLimit = lib.mkOption {
          type = lib.types.int;
          default = 65536;
          description = "Maximum number of open file descriptors";
        };
      };
    };
  };

  config = lib.mkIf config.unheaded.container.enable {
    # =========================================================================
    # MINIMAL NIXOS BASE
    # =========================================================================
    # Start with absolute minimum, add only what's needed
    # =========================================================================

    system.stateVersion = "24.05";

    # Disable unnecessary services
    documentation.enable = false;
    documentation.nixos.enable = false;
    programs.command-not-found.enable = false;

    # Minimal boot (no bootloader in containers)
    boot.isContainer = true;

    # Locale and timezone
    time.timeZone = "UTC";
    i18n.defaultLocale = "en_US.UTF-8";

    # =========================================================================
    # MINIMAL PACKAGES
    # =========================================================================
    # Only essential tools for debugging
    # =========================================================================
    environment.systemPackages = with pkgs; [
      # Network diagnostics
      curl
      iproute2
      netcat

      # Process inspection
      procps
      htop

      # File inspection
      file
      less

      # JSON parsing (for health checks)
      jq
    ];

    # =========================================================================
    # STANDARD USER
    # =========================================================================
    # Non-root user for service processes
    # =========================================================================
    users.users.unheaded = {
      isSystemUser = true;
      group = "unheaded";
      description = "Unheaded service user";
      home = "/var/lib/unheaded";
      createHome = true;
      uid = 1000;
    };

    users.groups.unheaded = {
      gid = 1000;
    };

    # =========================================================================
    # DIRECTORY STRUCTURE
    # =========================================================================
    systemd.tmpfiles.rules = [
      # Service data directory
      "d /var/lib/unheaded 0750 unheaded unheaded -"
      "d /var/lib/unheaded/${config.unheaded.container.name} 0750 unheaded unheaded -"

      # Log directory
      "d /var/log/unheaded 0755 unheaded unheaded -"
      "d /var/log/unheaded/${config.unheaded.container.name} 0755 unheaded unheaded -"

      # Config directory
      "d /etc/unheaded 0755 root root -"
      "d /etc/unheaded/${config.unheaded.container.name} 0755 root root -"

      # Health check scripts directory
      "d /etc/unheaded/health 0755 root root -"
    ];

    # =========================================================================
    # HEALTH CHECK SCRIPT
    # =========================================================================
    environment.etc."unheaded/health/${config.unheaded.container.name}-health.sh" = lib.mkIf config.unheaded.container.healthCheck.enable {
      text = ''
        #!/usr/bin/env bash
        # =======================================================================
        # Health Check Script for ${config.unheaded.container.name}
        # =======================================================================
        # Exit codes:
        #   0 - Healthy
        #   1 - Unhealthy
        #   2 - Unknown/Error
        # =======================================================================
        set -euo pipefail

        CONTAINER_NAME="${config.unheaded.container.name}"
        HEALTH_PORT="${toString config.unheaded.container.healthCheck.port}"
        HEALTH_PATH="${config.unheaded.container.healthCheck.path}"
        TIMEOUT="${config.unheaded.container.healthCheck.timeout}"

        # Construct health check URL
        HEALTH_URL="http://127.0.0.1:''${HEALTH_PORT}''${HEALTH_PATH}"

        # Perform health check
        check_health() {
          local response
          local http_code

          # Make HTTP request with timeout
          response=$(curl -sf \
            --connect-timeout 2 \
            --max-time "''${TIMEOUT%s}" \
            -w "%{http_code}" \
            -o /dev/null \
            "''${HEALTH_URL}" 2>/dev/null) || return 1

          # Check HTTP status code
          if [[ "$response" -ge 200 ]] && [[ "$response" -lt 300 ]]; then
            return 0
          else
            return 1
          fi
        }

        # Main
        main() {
          if check_health; then
            echo "OK: ''${CONTAINER_NAME} is healthy"
            exit 0
          else
            echo "FAIL: ''${CONTAINER_NAME} health check failed"
            exit 1
          fi
        }

        main "$@"
      '';
      mode = "0755";
    };

    # =========================================================================
    # NETWORKING
    # =========================================================================
    # Static IP, default-deny firewall
    # =========================================================================
    networking = {
      hostName = config.unheaded.container.name;

      # Use systemd-networkd for container networking
      useNetworkd = true;
      useDHCP = false;

      # Static IP configuration
      interfaces.eth0 = {
        ipv4.addresses = [{
          address = config.unheaded.container.network.ip;
          prefixLength = 24;
        }];
      };

      # Default gateway (container host bridge)
      defaultGateway = {
        address = "10.10.10.1";
        interface = "eth0";
      };

      # DNS servers
      nameservers = [ "1.1.1.1" "8.8.8.8" ];

      # Firewall: default-deny with explicit allows
      firewall = {
        enable = true;
        allowedTCPPorts = config.unheaded.container.network.ports;

        # Custom iptables rules for strict isolation
        extraCommands = ''
          # ===================================================================
          # INPUT CHAIN - Ingress Control
          # ===================================================================

          # Allow established/related connections
          iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

          # Allow loopback
          iptables -A INPUT -i lo -j ACCEPT

          # Allow ICMP (ping) from internal network only
          iptables -A INPUT -p icmp --icmp-type echo-request -s 10.10.10.0/24 -j ACCEPT

          # Allow configured ports from allowed sources
          ${lib.concatMapStringsSep "\n" (port:
            lib.concatMapStringsSep "\n" (cidr: ''
              iptables -A INPUT -p tcp --dport ${toString port} -s ${cidr} -j ACCEPT
            '') config.unheaded.container.network.allowedSources
          ) config.unheaded.container.network.ports}

          # Log dropped packets (rate limited)
          iptables -A INPUT -m limit --limit 5/min -j LOG \
            --log-prefix "[${config.unheaded.container.name}] DROP: " \
            --log-level 4

          # Default: DROP
          iptables -A INPUT -j DROP

          # ===================================================================
          # OUTPUT CHAIN - Egress Control
          # ===================================================================

          # Allow established/related connections
          iptables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

          # Allow loopback
          iptables -A OUTPUT -o lo -j ACCEPT

          # Allow DNS (UDP and TCP)
          iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
          iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT

          ${lib.optionalString config.unheaded.container.network.allowBusboyAccess ''
            # Allow Busboy gRPC connection
            iptables -A OUTPUT -d ${config.unheaded.container.network.busboyAddress} \
              -p tcp --dport ${toString config.unheaded.container.network.busboyPort} -j ACCEPT

            # Allow Busboy HTTP connection (REST API)
            iptables -A OUTPUT -d ${config.unheaded.container.network.busboyAddress} \
              -p tcp --dport 8080 -j ACCEPT
          ''}

          # Allow internal container network
          iptables -A OUTPUT -d 10.10.10.0/24 -j ACCEPT

          # Allow NTP (time sync)
          iptables -A OUTPUT -p udp --dport 123 -j ACCEPT

          # Log dropped outbound (rate limited)
          iptables -A OUTPUT -m limit --limit 5/min -j LOG \
            --log-prefix "[${config.unheaded.container.name}] OUT-DROP: " \
            --log-level 4

          # Default: DROP (strict - no internet by default)
          # iptables -A OUTPUT -j DROP
          # NOTE: Commented out to allow package downloads during build
          # Uncomment for production deployments

          # ===================================================================
          # FORWARD CHAIN - No Forwarding
          # ===================================================================
          iptables -P FORWARD DROP
        '';
      };
    };

    # =========================================================================
    # KERNEL HARDENING
    # =========================================================================
    # Sysctl parameters for security
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
      "net.ipv4.tcp_max_syn_backlog" = 4096;

      # IPv6 security
      "net.ipv6.conf.all.accept_source_route" = 0;
      "net.ipv6.conf.default.accept_source_route" = 0;

      # Kernel hardening
      "kernel.dmesg_restrict" = 1;                    # Restrict dmesg access
      "kernel.kptr_restrict" = 2;                     # Hide kernel pointers
      "kernel.yama.ptrace_scope" = 2;                 # Restrict ptrace

      # Disable IP forwarding (containers don't route)
      "net.ipv4.ip_forward" = 0;
      "net.ipv6.conf.all.forwarding" = 0;
    };

    # =========================================================================
    # SYSTEMD SERVICE HARDENING DEFAULTS
    # =========================================================================
    # Default security settings for all systemd services
    # =========================================================================
    systemd.services = {
      # Apply to the main container service
      "${config.unheaded.container.name}" = {
        serviceConfig = lib.mkMerge [
          # Base hardening
          {
            # Capabilities
            CapabilityBoundingSet = config.unheaded.container.security.capabilities;
            AmbientCapabilities = config.unheaded.container.security.capabilities;
            NoNewPrivileges = config.unheaded.container.security.noNewPrivileges;

            # Filesystem isolation
            ProtectSystem = if config.unheaded.container.security.readOnlyRoot then "strict" else "full";
            ProtectHome = true;
            PrivateTmp = true;
            PrivateDevices = true;

            # Writable paths
            ReadWritePaths = [
              "/var/lib/unheaded/${config.unheaded.container.name}"
              "/var/log/unheaded/${config.unheaded.container.name}"
            ] ++ config.unheaded.container.security.writablePaths;

            # Kernel protection
            ProtectKernelTunables = true;
            ProtectKernelModules = true;
            ProtectKernelLogs = true;
            ProtectControlGroups = true;

            # Process restrictions
            RestrictRealtime = true;
            RestrictNamespaces = true;
            RestrictSUIDSGID = true;
            RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];

            # Memory protection
            MemoryDenyWriteExecute = true;
            LockPersonality = true;

            # Process isolation
            PrivateUsers = true;
            RemoveIPC = true;

            # Resource limits
            MemoryMax = config.unheaded.container.resources.memoryMax;
            CPUQuota = config.unheaded.container.resources.cpuQuota;
            TasksMax = config.unheaded.container.resources.tasksMax;
            LimitNOFILE = config.unheaded.container.resources.nofileLimit;
            LimitNPROC = 512;

            # Syscall architecture lock
            SystemCallArchitectures = "native";
            SystemCallErrorNumber = "EPERM";
          }

          # Seccomp filtering based on profile
          (lib.mkIf (config.unheaded.container.security.seccompProfile == "strict") {
            SystemCallFilter = [
              "@system-service"         # Network service baseline
              "~@privileged"            # No CAP_SYS_ADMIN operations
              "~@resources"             # No mount/swap/quota
              "~@obsolete"              # No deprecated syscalls
              "~@debug"                 # No ptrace/perf
              "~@mount"                 # No filesystem operations
              "~@reboot"                # No reboot/kexec
              "~@swap"                  # No swap operations
              "~@module"                # No kernel modules
              "~@raw-io"                # No direct I/O
            ];
          })

          (lib.mkIf (config.unheaded.container.security.seccompProfile == "default") {
            SystemCallFilter = [
              "@system-service"
              "~@privileged"
            ];
          })
        ];
      };
    };

    # =========================================================================
    # TIME SYNCHRONIZATION
    # =========================================================================
    # Critical for distributed tracing and log correlation
    # =========================================================================
    services.timesyncd = {
      enable = true;
      servers = [
        "time.cloudflare.com"
        "time.google.com"
        "pool.ntp.org"
      ];
    };

    # =========================================================================
    # LOGGING
    # =========================================================================
    # Structured JSON logging via journald
    # =========================================================================
    services.journald = {
      extraConfig = ''
        # Persistent storage
        Storage=persistent

        # Rotation policy
        SystemMaxUse=500M
        SystemMaxFileSize=50M
        SystemMaxFiles=10

        # Compression
        Compress=yes

        # Rate limiting
        RateLimitInterval=30s
        RateLimitBurst=5000
      '';
    };

    # =========================================================================
    # ENVIRONMENT VARIABLES
    # =========================================================================
    # Standard across all containers
    # =========================================================================
    environment.variables = {
      # Container identification
      UNHEADED_CONTAINER_NAME = config.unheaded.container.name;
      UNHEADED_CONTAINER_IP = config.unheaded.container.network.ip;

      # Busboy connection
      UNHEADED_BUSBOY_ADDR = "${config.unheaded.container.network.busboyAddress}:${toString config.unheaded.container.network.busboyPort}";

      # Health check
      UNHEADED_HEALTH_PORT = toString config.unheaded.container.healthCheck.port;
      UNHEADED_HEALTH_PATH = config.unheaded.container.healthCheck.path;

      # Go runtime tuning
      GOGC = "100";
      GOMAXPROCS = "2";
      GOMEMLIMIT = config.unheaded.container.resources.memoryMax;

      # Rust runtime
      RUST_BACKTRACE = "1";
    };
  };
}
