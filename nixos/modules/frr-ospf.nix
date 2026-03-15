# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# FRR OSPFv3 NixOS module — Alternate Routing Option A
# Activate: services.unheaded.frr-ospf.enable = true
# CRITICAL: DO NOT use simultaneously with frr.nix (BGP EVPN)
# Select via: scripts/routing/select-routing.sh ospf
{ config, lib, pkgs, ... }:

let
  cfg = config.services.unheaded.frr-ospf;
in {
  options.services.unheaded.frr-ospf = {
    enable = lib.mkEnableOption "FRR OSPFv3 alternate routing (Option A)";
    
    wgInterface = lib.mkOption {
      type = lib.types.str;
      default = "wg0";
      description = "WireGuard east-west interface";
    };
    
    lanBridge = lib.mkOption {
      type = lib.types.str;
      default = "br-unheaded";
      description = "LAN bridge interface (passive in OSPFv3)";
    };
    
    routerId = lib.mkOption {
      type = lib.types.str;
      default = "10.20.255.1";
      description = "OSPFv3 router ID (use host's management IPv4)";
    };
  };

  config = lib.mkIf cfg.enable {
    # Deploy OSPFv3 config to /etc/frr/
    environment.etc."frr/ospf.conf".text = ''
      ! FRR OSPFv3 — Unheaded Kingdom — managed by NixOS
      ! Router ID: ${cfg.routerId}
      frr version 10.0
      hostname unheaded-${config.networking.hostName}
      log syslog informational
      service integrated-vtysh-config
      !
      ipv6 forwarding
      !
      interface lo
       ipv6 ospf6 area 0.0.0.0
       ipv6 ospf6 passive
      !
      interface ${cfg.wgInterface}
       ipv6 ospf6 area 0.0.0.0
       ipv6 ospf6 cost 10
       ipv6 ospf6 mtu-ignore
       ipv6 ospf6 hello-interval 5
       ipv6 ospf6 dead-interval 20
       ipv6 ospf6 bfd
      !
      interface ${cfg.lanBridge}
       ipv6 ospf6 area 0.0.0.0
       ipv6 ospf6 cost 1
       ipv6 ospf6 passive
      !
      router ospf6
       router-id ${cfg.routerId}
       maximum-paths 4
       redistribute connected
      !
      bfd
       peer fd00:dead:beef::2 interface ${cfg.wgInterface}
        receive-interval 300
        transmit-interval 300
        detect-multiplier 3
       !
      !
      end
    '';
    
    environment.etc."frr/daemons".text = ''
      bgpd=no
      ospf6d=yes
      bfdd=yes
      zebrad=yes
      staticd=yes
      mgmtd=yes
    '';
    
    # IPv6 forwarding required for routing
    boot.kernel.sysctl = {
      "net.ipv6.conf.all.forwarding" = 1;
      "net.ipv6.conf.${cfg.wgInterface}.forwarding" = 1;
    };
    
    # FRR service (same binary path as frr.nix, different config)
    systemd.services.frr = {
      description = "FRR OSPFv3 routing daemon";
      wantedBy = ["multi-user.target"];
      after = ["network-online.target" "${cfg.wgInterface}.service"];
      serviceConfig = {
        Type = "forking";
        PIDFile = "/run/frr/watchfrr.pid";
        ExecStart = "${pkgs.frr}/bin/watchfrr zebra ospf6d bfdd staticd mgmtd";
        ExecReload = "${pkgs.frr}/bin/vtysh -b";
        Restart = "on-failure";
        AmbientCapabilities = ["CAP_NET_ADMIN" "CAP_NET_RAW" "CAP_SYS_ADMIN"];
        User = "frr";
        Group = "frr";
      };
    };
  };
}
