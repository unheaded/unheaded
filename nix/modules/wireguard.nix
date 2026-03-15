{ config, lib, pkgs, ... }:

let
  cfg = config.unheaded.wireguard;

  # Role-based defaults
  roleConfig = {
    west = {
      address = "fd00:dead:beef::1/64";
      endpoint = "192.168.13.1:51820";
      allowedIPs = [ "fd00:dead:beef::2/128" "fd00:dead:beef:0:2::/80" ];
    };
    east = {
      address = "fd00:dead:beef::2/64";
      endpoint = "192.168.13.2:51820";
      allowedIPs = [ "fd00:dead:beef::1/128" "fd00:dead:beef:0:1::/80" ];
    };
  };

  roleCfg = roleConfig.${cfg.role};
in
{
  # ===========================================================================
  # UNHEADED WIREGUARD MODULE
  # ===========================================================================
  # Declarative WireGuard IPv6 overlay for Kingdom cross-host communication.
  #
  # Usage:
  #   unheaded.wireguard = {
  #     enable = true;
  #     role = "west";  # or "east"
  #     privateKeyFile = "/etc/wireguard/privatekey";
  #     presharedKeyFile = "/etc/wireguard/presharedkey";
  #     peerPublicKey = "<PEER_PUBLIC_KEY>";
  #   };
  # ===========================================================================

  options.unheaded.wireguard = {
    enable = lib.mkEnableOption "Unheaded WireGuard IPv6 overlay";

    role = lib.mkOption {
      type = lib.types.enum [ "west" "east" ];
      description = "Host role in the Kingdom overlay (west or east)";
    };

    listenPort = lib.mkOption {
      type = lib.types.int;
      default = 51820;
      description = "WireGuard listen port (UDP)";
    };

    privateKeyFile = lib.mkOption {
      type = lib.types.path;
      default = "/etc/wireguard/privatekey";
      description = ''
        Path to the WireGuard private key file.
        Generate with: wg genkey > /etc/wireguard/privatekey
        NEVER store private keys in the Nix store or Git.
      '';
    };

    presharedKeyFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = ''
        Path to the pre-shared key file for post-quantum resistance.
        Generate with: wg genpsk > /etc/wireguard/presharedkey
      '';
    };

    peerPublicKey = lib.mkOption {
      type = lib.types.str;
      description = "Public key of the peer host";
    };

    address = lib.mkOption {
      type = lib.types.str;
      default = roleCfg.address;
      description = "IPv6 overlay address for this host";
    };

    peerEndpoint = lib.mkOption {
      type = lib.types.str;
      default = roleCfg.endpoint;
      description = "Peer endpoint (IPv4:port over P2P link)";
    };

    peerAllowedIPs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = roleCfg.allowedIPs;
      description = "AllowedIPs for the peer";
    };

    persistentKeepalive = lib.mkOption {
      type = lib.types.int;
      default = 25;
      description = "Persistent keepalive interval in seconds";
    };
  };

  config = lib.mkIf cfg.enable {
    # Ensure wireguard-tools are available
    environment.systemPackages = [ pkgs.wireguard-tools ];

    # WireGuard interface
    networking.wireguard.interfaces.wg0 = {
      ips = [ cfg.address ];
      listenPort = cfg.listenPort;
      privateKeyFile = cfg.privateKeyFile;

      # Routing: add /48 overlay route when interface comes up
      postSetup = ''
        ${pkgs.iproute2}/bin/ip -6 route add fd00:dead:beef::/48 dev wg0 || true
        ${pkgs.procps}/bin/sysctl -w net.ipv6.conf.all.forwarding=1
        ${pkgs.procps}/bin/sysctl -w net.ipv6.conf.wg0.forwarding=1
      '';
      postShutdown = ''
        ${pkgs.iproute2}/bin/ip -6 route del fd00:dead:beef::/48 dev wg0 || true
        ${pkgs.procps}/bin/sysctl -w net.ipv6.conf.all.forwarding=0
      '';

      peers = [
        {
          publicKey = cfg.peerPublicKey;
          presharedKeyFile = cfg.presharedKeyFile;
          allowedIPs = cfg.peerAllowedIPs;
          endpoint = cfg.peerEndpoint;
          persistentKeepalive = cfg.persistentKeepalive;
        }
      ];
    };

    # Open firewall for WireGuard UDP port on P2P interface
    networking.firewall.allowedUDPPorts = [ cfg.listenPort ];

    # IPv6 forwarding for overlay traffic
    boot.kernel.sysctl = {
      "net.ipv6.conf.all.forwarding" = 1;
      "net.ipv6.conf.default.forwarding" = 1;
    };
  };
}
