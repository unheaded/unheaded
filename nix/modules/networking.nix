{ config, lib, pkgs, ... }:

{
  # =============================================================================
  # UNHEADED NETWORK MODULE
  # =============================================================================
  # Network topology and isolation policies for Unheaded container mesh.
  #
  # Network Design:
  #   Bridge: lxdbr0 (10.10.10.0/24)
  #   Gateway: 10.10.10.100
  #   Wotan: 10.10.10.10
  #   Services: 10.10.10.20-30
  #   Apps: 10.10.10.200+
  #
  # Isolation Policy:
  #   - Default: DENY ALL
  #   - Explicit allow: Service → Wotan
  #   - Explicit allow: Gateway → Services
  #   - NO direct app → service communication
  # =============================================================================

  options = {
    unheaded.networking = {
      enable = lib.mkEnableOption "Unheaded network configuration";

      serviceIP = lib.mkOption {
        type = lib.types.str;
        description = "Static IP address for this service";
      };

      servicePort = lib.mkOption {
        type = lib.types.int;
        description = "Primary service port (HTTP API)";
      };

      wotanAddress = lib.mkOption {
        type = lib.types.str;
        default = "10.10.10.10:9090";
        description = "Wotan message bus address";
      };

      allowDirectAccess = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = "Allow direct access from external (gateway use only)";
      };

      allowedSources = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ "10.10.10.0/24" ];
        description = "CIDR blocks allowed to connect";
      };

      strictEgress = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = ''
          Enable strict egress filtering (DROP all non-whitelisted outbound).
          Set to true for production. When false, allows outbound internet
          for package downloads and API calls during development/build.
        '';
      };
    };
  };

  config = lib.mkIf config.unheaded.networking.enable {
    # =========================================================================
    # NETWORK CONFIGURATION
    # =========================================================================
    networking = {
      # Hostname from service name
      hostName = config.unheaded.hardening.serviceName;

      # Use systemd-networkd (consistent with container approach)
      useNetworkd = true;
      useDHCP = false;

      # Static IP configuration
      interfaces.eth0 = {
        ipv4.addresses = [{
          address = config.unheaded.networking.serviceIP;
          prefixLength = 24;
        }];
      };

      # Default gateway (container host)
      defaultGateway = {
        address = "10.10.10.1";
        interface = "eth0";
      };

      # DNS configuration
      nameservers = [ "1.1.1.1" "8.8.8.8" ];

      # -----------------------------------------------------------------------
      # FIREWALL RULES
      # -----------------------------------------------------------------------
      # Layered on top of hardening module firewall
      # -----------------------------------------------------------------------
      firewall = {
        enable = true;
        allowedTCPPorts = [ config.unheaded.networking.servicePort ];

        extraCommands = ''
          # ===================================================================
          # INPUT CHAIN - Who can talk to us
          # ===================================================================

          # Allow established connections
          iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

          # Allow loopback
          iptables -A INPUT -i lo -j ACCEPT

          # Allow health checks from container network
          iptables -A INPUT -p tcp --dport ${toString config.unheaded.networking.servicePort} -s 10.10.10.0/24 -j ACCEPT

          ${if config.unheaded.networking.allowDirectAccess then ''
            # Gateway can reach us directly
            iptables -A INPUT -s 10.10.10.100 -j ACCEPT
          '' else ''
            # Only allow from explicitly whitelisted sources
            ${lib.concatMapStringsSep "\n" (cidr: ''
              iptables -A INPUT -s ${cidr} -j ACCEPT
            '') config.unheaded.networking.allowedSources}
          ''}

          # Log and drop everything else
          iptables -A INPUT -m limit --limit 5/min -j LOG --log-prefix "[${config.unheaded.hardening.serviceName}] REJECT: " --log-level 4
          iptables -A INPUT -j DROP

          # ===================================================================
          # OUTPUT CHAIN - Where we can talk to
          # ===================================================================

          # Allow established connections
          iptables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

          # Allow loopback
          iptables -A OUTPUT -o lo -j ACCEPT

          # Allow DNS
          iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
          iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT

          # Allow Wotan connection
          iptables -A OUTPUT -d 10.10.10.10 -p tcp --dport 9090 -j ACCEPT

          # Allow internal container network
          iptables -A OUTPUT -d 10.10.10.0/24 -j ACCEPT

          ${if config.unheaded.networking.strictEgress then ''
            # STRICT EGRESS: Log and drop all non-whitelisted outbound
            iptables -A OUTPUT -m limit --limit 5/min -j LOG \
              --log-prefix "[${config.unheaded.hardening.serviceName}] OUT-DROP: " \
              --log-level 4
            iptables -A OUTPUT -j DROP
          '' else ''
            # PERMISSIVE EGRESS: Allow outbound internet (development/build mode)
            # Set unheaded.networking.strictEgress = true for production
            iptables -A OUTPUT -j ACCEPT
          ''}

          # ===================================================================
          # FORWARD CHAIN - No forwarding
          # ===================================================================
          iptables -P FORWARD DROP
        '';
      };
    };

    # =========================================================================
    # NETWORK SECURITY
    # =========================================================================
    # Additional sysctl tuning beyond hardening module
    # =========================================================================
    boot.kernel.sysctl = {
      # TCP hardening
      "net.ipv4.tcp_max_syn_backlog" = 4096;          # SYN flood protection
      "net.ipv4.tcp_synack_retries" = 2;              # Reduce SYN-ACK retries
      "net.ipv4.tcp_syn_retries" = 2;                 # Reduce SYN retries

      # Connection tracking
      "net.netfilter.nf_conntrack_max" = 65536;       # Connection table size
      "net.netfilter.nf_conntrack_tcp_timeout_established" = 600;  # 10 min timeout

      # ICMP rate limiting
      "net.ipv4.icmp_ratelimit" = 100;                # 100ms between ICMP
      "net.ipv4.icmp_ratemask" = 6168;                # Rate limit echo, timestamp

      # Disable IP forwarding (unless gateway)
      "net.ipv4.ip_forward" = 0;
      "net.ipv6.conf.all.forwarding" = 0;
    };

    # =========================================================================
    # SERVICE DISCOVERY
    # =========================================================================
    # Environment variables for service mesh integration
    # =========================================================================
    environment.variables = {
      UNHEADED_SERVICE_NAME = config.unheaded.hardening.serviceName;
      UNHEADED_SERVICE_IP = config.unheaded.networking.serviceIP;
      UNHEADED_SERVICE_PORT = toString config.unheaded.networking.servicePort;
      UNHEADED_WOTAN_ADDR = config.unheaded.networking.wotanAddress;
    };

    # =========================================================================
    # MONITORING
    # =========================================================================
    # Expose network metrics for Prometheus
    # =========================================================================
    services.prometheus.exporters.node = {
      enable = true;
      enabledCollectors = [
        "netdev"                # Network device stats
        "netstat"               # Network stack stats
        "conntrack"             # Connection tracking
        "sockstat"              # Socket stats
      ];
      port = 9100;
      listenAddress = config.unheaded.networking.serviceIP;

      # Only accessible from monitoring network
      firewallFilter = "-s 10.10.10.0/24 -p tcp -m tcp --dport 9100";
    };
  };
}
