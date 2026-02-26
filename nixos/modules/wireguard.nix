# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# wireguard.nix — WireGuard east-west control plane bridge
# IPv6 /48 addressing: fd00:dead:beef::/48
# WireGuard tunnel: fd00:dead:beef:wg::/64
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.wireguard;
in {
  options.services.unheaded.wireguard = {
    enable = mkEnableOption "Unheaded WireGuard east-west bridge";
    
    role = mkOption {
      type = types.enum [ "server" "client" ];
      default = "client";
      description = "server = Host-A (forge), client = Host-B (outpost)";
    };
    
    privateKeyFile = mkOption {
      type = types.path;
      default = "/etc/unheaded/wg/private.key";
      description = "Path to WireGuard private key file (generated on host, not committed)";
    };
    
    # Server options (Host-A)
    listenPort = mkOption {
      type = types.port;
      default = 51820;
      description = "WireGuard listen port (server only)";
    };
    
    # Client options (Host-B)
    serverEndpoint = mkOption {
      type = types.str;
      default = "";  # TODO: set to Host-A's public IP
      description = "Host-A public IP:port (e.g., 203.0.113.1:51820)";
    };
    
    serverPublicKey = mkOption {
      type = types.str;
      default = "";  # TODO: set from Host-A `wg genkey | tee private.key | wg pubkey`
      description = "Host-A WireGuard public key";
    };
    
    outpostPublicKey = mkOption {
      type = types.str;
      default = "";  # TODO: set from Host-B public key
      description = "Host-B WireGuard public key";
    };
    
    mtu = mkOption {
      type = types.int;
      default = 1380;
      description = "WireGuard MTU (1380 recommended for IPv6 with Unheaded HbH headers)";
    };
  };

  config = mkIf cfg.enable {
    networking.wireguard.interfaces.wg0 = {
      ips = if cfg.role == "server"
        then [ "fd00:dead:beef:wg::1/64" ]
        else [ "fd00:dead:beef:wg::2/64" ];
      
      listenPort = mkIf (cfg.role == "server") cfg.listenPort;
      privateKeyFile = cfg.privateKeyFile;
      mtu = cfg.mtu;
      
      peers = if cfg.role == "server" then [
        # Host-B peer (add more peers here for additional outposts)
        {
          publicKey = cfg.outpostPublicKey;
          allowedIPs = [ "fd00:dead:beef:wg::2/128" ];
          persistentKeepalive = 25;
        }
      ] else [
        # Host-A (forge) peer
        {
          publicKey = cfg.serverPublicKey;
          endpoint = cfg.serverEndpoint;
          allowedIPs = [
            "fd00:dead:beef:wg::1/128"
            # Route all Unheaded IPv6 traffic through the tunnel
            "fd00:dead:beef::/48"
          ];
          persistentKeepalive = 25;
        }
      ];
      
      postSetup = ''
        ip -6 route add fd00:dead:beef::/48 dev wg0 || true
      '';
      
      postShutdown = ''
        ip -6 route del fd00:dead:beef::/48 dev wg0 || true
      '';
    };
    
    # Firewall
    networking.firewall.allowedUDPPorts = mkIf (cfg.role == "server") [ cfg.listenPort ];
    
    # IPv6 forwarding (required for routing between tunnel and LAN)
    boot.kernel.sysctl = {
      "net.ipv6.conf.all.forwarding"     = 1;
      "net.ipv6.conf.default.forwarding" = 1;
      "net.ipv6.conf.wg0.forwarding"     = 1;
    };
    
    # Key generation helper (run once on each host)
    environment.systemPackages = [ pkgs.wireguard-tools ];
    
    # Ensure key directory exists with restricted permissions
    systemd.tmpfiles.rules = [
      "d /etc/unheaded/wg 0700 root root -"
    ];
    
    # One-time key generation service (idempotent)
    systemd.services.unheaded-wg-keygen = {
      description = "Generate WireGuard key pair if not exists";
      wantedBy = [ "multi-user.target" ];
      before = [ "wireguard-wg0.service" ];
      
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecStart = pkgs.writeScript "wg-keygen" ''
          #!/bin/sh
          if [ ! -f /etc/unheaded/wg/private.key ]; then
            umask 077
            ${pkgs.wireguard-tools}/bin/wg genkey > /etc/unheaded/wg/private.key
            ${pkgs.wireguard-tools}/bin/wg pubkey < /etc/unheaded/wg/private.key > /etc/unheaded/wg/public.key
            chmod 600 /etc/unheaded/wg/private.key
            chmod 644 /etc/unheaded/wg/public.key
            echo "WireGuard keys generated. Public key:"
            cat /etc/unheaded/wg/public.key
          fi
        '';
      };
    };
  };
}
