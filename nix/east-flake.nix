# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# east-flake.nix — Lightweight NixOS configuration for EAST bare metal bootstrap
#
# Target hardware: 4-core CPU, 8GB DDR3 RAM, single NIC
# Purpose: Minimal service footprint with WireGuard, BGP routing, and 9 critical services
# Network: fd00:dead:beef::2/48 (EAST), connected to WEST via WireGuard tunnel
#
# Deploy: nix build -f east-flake.nix --attr config.system.build.toplevel
{ config, pkgs, lib, ... }:

with lib;

{
  # ============================================================================
  # SYSTEM CONFIGURATION
  # ============================================================================
  # Lightweight NixOS for EAST outpost - minimal base system

  # Hostname identifies this node in the mesh
  networking.hostName = "east-outpost";

  # Stable NixOS release channel
  system.stateVersion = "24.05";

  # ============================================================================
  # BOOTLOADER & KERNEL
  # ============================================================================
  # UEFI boot with minimal GRUB, eBPF support enabled

  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;

  # eBPF support for network observability and tracing
  boot.kernelParams = [
    "systemd.unified_cgroup_hierarchy=false"  # For cgroup v1 eBPF support
  ];

  boot.kernelModules = [
    "bpf"                # Enable BPF subsystem
    "kprobes"            # Enable kprobes for dynamic tracing
    "tracepoints"        # Enable tracepoint infrastructure
  ];

  boot.kernelPatches = [ ];

  # ============================================================================
  # FILESYSTEM & STORAGE
  # ============================================================================
  # Simple ext4 for data, swap for memory pressure

  fileSystems."/" = {
    device = "/dev/disk/by-label/nixos";
    fsType = "ext4";
  };

  fileSystems."/boot" = {
    device = "/dev/disk/by-label/efi";
    fsType = "vfat";
  };

  # Swap: 8GB total - 1GB for kernel/FS = ~7GB for services
  # Reserve 2GB for system, 5GB for services
  swapDevices = [
    { device = "/dev/disk/by-label/swap"; }
  ];

  # ============================================================================
  # NETWORKING - CORE
  # ============================================================================
  # IPv6 mesh addressing with WireGuard client role

  networking.useNetworkd = true;
  networking.firewall.enable = true;
  networking.firewall.autoLoadConntrackHelpers = false;

  # IPv6 configuration
  networking.defaultGateway6 = {
    address = "fd00:dead:beef::1";
    interface = "wg0";
  };

  # Enable IPv6 forwarding for mesh routing
  boot.kernel.sysctl = {
    "net.ipv6.conf.all.forwarding" = 1;
    "net.ipv6.conf.default.forwarding" = 1;
    "net.ipv6.conf.default.accept_ra" = 2;  # Accept RA from any source
  };

  # ============================================================================
  # WIREGUARD TUNNEL TO WEST
  # ============================================================================
  # Client mode: connects to WEST (fd00:dead:beef::1)

  services.unheaded.wireguard.enable = true;
  services.unheaded.wireguard.role = "client";

  # These must be populated during bootstrap Phase 2
  # Run: wg genkey | tee /etc/unheaded/wg/private.key | wg pubkey
  services.unheaded.wireguard.serverPublicKey = "TODO_SET_WEST_PUBLIC_KEY";
  services.unheaded.wireguard.serverEndpoint = "TODO_SET_WEST_ENDPOINT";
  services.unheaded.wireguard.outpostPublicKey = "TODO_SET_EAST_PUBLIC_KEY";

  # WireGuard network configuration
  # IPv6 address: fd00:dead:beef:wg::2/64 (tunnel)
  # Routable EAST subnet: fd00:dead:beef:2::/64
  services.unheaded.wireguard.mtu = 1380;

  # ============================================================================
  # BGP ROUTING (BIRD)
  # ============================================================================
  # Lightweight BGP peering: EAST (AS 65002) ←→ WEST (AS 65001)

  services.unheaded.bird.enable = true;
  services.unheaded.bird.bgpAs = 65002;
  services.unheaded.bird.routerId = "10.20.255.2";
  services.unheaded.bird.forgePeer = "fd00:dead:beef::1";  # WEST endpoint
  services.unheaded.bird.forgeAs = 65001;  # WEST's AS number
  services.unheaded.bird.raInterface = "wg0";  # Advertise routes over tunnel
  services.unheaded.bird.ipv6Prefix = "fd00:dead:beef:2::/64";  # EAST subnet

  # ============================================================================
  # SYSTEM PACKAGES
  # ============================================================================
  # Minimal set: tools, eBPF, networking, observability

  environment.systemPackages = with pkgs; [
    # Essential utilities
    curl
    wget
    htop
    iotop
    vim
    nano
    jq

    # eBPF tooling for network observability
    bpftool              # BPF inspection and monitoring
    libbpf               # BPF library for userspace programs
    clang                # For compiling eBPF programs

    # Network diagnostics
    iproute2
    iputils
    netcat
    tcpdump
    wireguard-tools      # Already included via wireguard module
    bind-tools           # DNS tools (dig, nslookup)

    # Observability agent tools
    prometheus           # For node-exporter, prometheus-agent
    grafana              # For dashboard
    loki                 # For promtail log shipping

    # Supervision (for service startup)
    runit                # Lightweight service supervision
  ];

  # ============================================================================
  # SSH ACCESS
  # ============================================================================
  # Bootstrap requirement: SSH for remote management

  services.openssh.enable = true;
  services.openssh.settings = {
    PermitRootLogin = "prohibit-password";
    PasswordAuthentication = false;
  };

  # ============================================================================
  # MEMORY MANAGEMENT
  # ============================================================================
  # Tight memory constraints: 8GB total, ~5GB for services

  # Disable unnecessary daemons that consume memory
  services.cups.enable = lib.mkForce false;
  services.avahi.enable = lib.mkForce false;
  services.bluetooth.enable = lib.mkForce false;

  # Reduce journald retention to save disk space
  services.journald.extraConfig = ''
    SystemMaxUse=100M
    RuntimeMaxUse=50M
  '';

  # ============================================================================
  # SERVICE DIRECTORY
  # ============================================================================
  # User for running EAST services with minimal privileges

  users.users.unheaded = {
    isSystemUser = true;
    group = "unheaded";
    home = "/var/lib/unheaded";
    createHome = true;
    shell = pkgs.nologin;
  };

  users.groups.unheaded = { };

  # ============================================================================
  # THE 9 EAST SERVICES
  # ============================================================================
  # Lightweight service configuration: wotan, monad, sophia, anamnesis, gateway,
  # dashboard-backend, prometheus-agent, promtail, node-exporter
  #
  # Memory allocation strategy (5GB total):
  #   - Wotan (message bus):        800MB
  #   - Monad (state):              600MB
  #   - Sophia (knowledge):         600MB
  #   - Anamnesis (memory):         400MB
  #   - Gateway (public access):    400MB
  #   - Dashboard-backend (UI):     400MB
  #   - Prometheus-agent:           300MB
  #   - Promtail (log shipping):    200MB
  #   - Node-exporter (metrics):    100MB
  #   - Reserve (headroom):         700MB
  #
  # Services must be pre-built on WEST and copied via scp/rsync

  # Placeholder configuration - services are deployed via systemd unit files
  # See: /opt/east/launch-minimal.sh (deployed during Phase 4)

  systemd.tmpfiles.rules = [
    # Create service directories with proper permissions
    "d /opt/east 0755 unheaded unheaded -"
    "d /opt/east/bin 0755 unheaded unheaded -"
    "d /opt/east/data 0755 unheaded unheaded -"
    "d /opt/east/logs 0755 unheaded unheaded -"
    "d /opt/east/config 0755 unheaded unheaded -"
  ];

  # ============================================================================
  # NODE EXPORTER (System metrics)
  # ============================================================================
  # Exposes system metrics (CPU, memory, disk, network) on :9100/metrics

  services.prometheus.exporters.node = {
    enable = true;
    port = 9100;
    listenAddress = "[::1]";  # Listen on IPv6 loopback only
    enabledCollectors = [
      "cpu"
      "diskstats"
      "filesystem"
      "loadavg"
      "meminfo"
      "netdev"
      "netstat"
      "processes"
      "tcpstat"
    ];
  };

  # ============================================================================
  # PROMTAIL (Log shipper to WEST)
  # ============================================================================
  # Ships service logs to WEST's Loki instance

  services.promtail = {
    enable = true;
    configuration = {
      clients = [
        {
          url = "http://fd00:dead:beef::1:3100/loki/api/v1/push";
        }
      ];
      scrape_configs = [
        {
          job_name = "systemd";
          journal = {
            max_age = "24h";
            labels = {
              job = "systemd-journal";
              hostname = "east-outpost";
            };
          };
          relabel_configs = [
            {
              source_labels = [ "__journal_systemd_unit" ];
              target_label = "unit";
            }
          ];
        }
        {
          job_name = "east-services";
          static_configs = [
            {
              targets = [ "localhost" ];
              labels = {
                job = "east-services";
                hostname = "east-outpost";
              };
            }
          ];
          # Mount /opt/east/logs if services write logs there
        }
      ];
    };
  };

  # ============================================================================
  # PROMETHEUS AGENT (Local metrics scraper)
  # ============================================================================
  # Scrapes local services and ships to WEST's central Prometheus

  services.prometheus.enable = true;
  services.prometheus.port = 9090;
  services.prometheus.listenAddress = "::1";  # IPv6 loopback
  services.prometheus.globalConfig = {
    scrape_interval = "30s";
    evaluation_interval = "30s";
  };

  services.prometheus.scrapeConfigs = [
    {
      job_name = "node-exporter";
      static_configs = [
        { targets = [ "[::1]:9100" ]; }
      ];
    }
    # Add scrape configs for each EAST service once deployed
    # Example:
    # {
    #   job_name = "wotan";
    #   static_configs = [
    #     { targets = [ "[::1]:8080" ]; }  # Adjust port based on service
    #   ];
    # }
  ];

  # Remote write to WEST (configure after Phase 4)
  # services.prometheus.remoteWrite = [
  #   {
  #     url = "http://fd00:dead:beef::1:9009/api/v1/write";
  #   }
  # ];

  # ============================================================================
  # SECURITY HARDENING
  # ============================================================================
  # Minimal attack surface for bare metal bootstrap

  security.allowUserNamespaces = false;
  security.unprivilegedUsernsclone = false;

  # Disable unnecessary services
  services.logrotate.enable = true;
  services.logrotate.checkConfig = false;  # Skip syntax check for speed

  # ============================================================================
  # DOCUMENTATION & NOTES
  # ============================================================================
  # EAST SERVICE MANIFEST:
  #
  # Phase 4 deployment (from WEST) includes:
  #
  # 1. WOTAN (Message Bus)
  #    - Port: 8080 (REST API)
  #    - Memory: 800MB
  #    - Role: Central event hub for all EAST services
  #
  # 2. MONAD (State Service)
  #    - Port: 8081
  #    - Memory: 600MB
  #    - Role: Distributed state management
  #
  # 3. SOPHIA (Knowledge Service)
  #    - Port: 8082
  #    - Memory: 600MB
  #    - Role: Knowledge graph and reasoning
  #
  # 4. ANAMNESIS (Memory Service)
  #    - Port: 8083
  #    - Memory: 400MB
  #    - Role: Historical data and analytics
  #
  # 5. GATEWAY (Public Access Point)
  #    - Port: 8080 (proxy)
  #    - Memory: 400MB
  #    - Role: Public API endpoint, routing to EAST services
  #
  # 6. DASHBOARD-BACKEND (UI Backend)
  #    - Port: 8084
  #    - Memory: 400MB
  #    - Role: Backend for EAST web dashboard
  #
  # 7. PROMETHEUS-AGENT (Metrics)
  #    - Port: 9090
  #    - Memory: 300MB (included above)
  #    - Role: Scrape and aggregate local metrics
  #
  # 8. PROMTAIL (Log Shipper)
  #    - Port: none (outbound only)
  #    - Memory: 200MB (included above)
  #    - Role: Ship logs to WEST Loki
  #
  # 9. NODE-EXPORTER (System Metrics)
  #    - Port: 9100
  #    - Memory: 100MB (included above)
  #    - Role: Expose system metrics
  #
  # DISABLED HEAVY SERVICES (not included in EAST):
  #   - vllm-rocm: GPU inference (no GPU in EAST)
  #   - suricata: IDS/IPS (network inspection at WEST gateway)
  #   - elasticsearch: Full-text search (logs on WEST Loki)
  #   - vectordb: Embedding storage (on WEST)
  #   - kanban/dashboard-app: Heavy web frontends (lightweight alternatives on WEST)
  #
  # BOOTSTRAP PHASES:
  #   - Phase 1 (OS): NixOS bare metal install with SSH
  #   - Phase 2 (WireGuard): Tunnel to WEST, public key exchange
  #   - Phase 3 (BGP): Route peering with WEST
  #   - Phase 4 (Services): Binary deployment and startup
  #   - Phase 5 (Observability): Verify metrics/logs flow
  #   - Phase 6 (Verification): Health checks and integration tests

}
