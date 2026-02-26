# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# FRR MPLS LDP NixOS module — Alternate Routing Option C
{ config, lib, pkgs, ... }:

let
  cfg = config.services.unheaded.frr-mpls;
in {
  options.services.unheaded.frr-mpls = {
    enable = lib.mkEnableOption "FRR MPLS LDP routing (Option C)";
    
    wgInterface = lib.mkOption {
      type = lib.types.str;
      default = "wg0";
    };
    
    routerId = lib.mkOption {
      type = lib.types.str;
      default = "10.20.255.1";
    };
    
    peerLoopback = lib.mkOption {
      type = lib.types.str;
      default = "fd00:dead:beef:ff::2";
      description = "Peer host loopback address for LDP targeted session";
    };
  };

  config = lib.mkIf cfg.enable {
    # Kernel MPLS modules
    boot.kernelModules = ["mpls_router" "mpls_gso" "mpls_iptunnel"];
    
    # MPLS sysctls
    boot.kernel.sysctl = {
      "net.mpls.platform_labels" = 100000;
      "net.mpls.conf.lo.input" = 1;
      "net.mpls.conf.${cfg.wgInterface}.input" = 1;
      "net.ipv4.conf.all.forwarding" = 1;
      "net.ipv6.conf.all.forwarding" = 1;
    };
    
    environment.etc."frr/frr.conf".text = ''
      ! FRR MPLS LDP — managed by NixOS — host ${config.networking.hostName}
      ! CRITICAL: MPLS outer label stack; IPv6+HbH inner — Monad safe
      frr version 10.0
      hostname ${config.networking.hostName}
      log syslog informational
      service integrated-vtysh-config
      !
      mpls enable
      !
      interface ${cfg.wgInterface}
       mpls enable
      !
      interface lo
       mpls enable
      !
      mpls ldp
       router-id ${cfg.routerId}
       address-family ipv6
        discovery transport-address ${cfg.routerId}
        interface ${cfg.wgInterface}
        !
       !
      !
      end
    '';
    
    environment.etc."frr/daemons".text = ''
      bgpd=no
      ospf6d=no
      ldpd=yes
      zebrad=yes
      staticd=yes
      mgmtd=yes
    '';
    
    systemd.services.frr-mpls = {
      description = "FRR MPLS LDP routing daemon";
      wantedBy = ["multi-user.target"];
      after = ["network-online.target" "${cfg.wgInterface}.service"];
      serviceConfig = {
        Type = "forking";
        ExecStart = "${pkgs.frr}/bin/watchfrr zebra ldpd staticd mgmtd";
        Restart = "on-failure";
        AmbientCapabilities = ["CAP_NET_ADMIN" "CAP_NET_RAW" "CAP_SYS_ADMIN"];
        User = "frr";
        Group = "frr";
      };
    };
  };
}
