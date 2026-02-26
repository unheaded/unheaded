# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# frr.nix — Free Range Routing (FRR) module for host-a Forge
# BGP EVPN-VXLAN overlay + IS-IS underlay + BFD fast-failover
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.frr;
  
  # Build FRR from ~/tmp/frr-master/ source
  frrPackage = pkgs.stdenv.mkDerivation rec {
    pname = "frr";
    version = "10.x-unheaded";
    
    src = /. + builtins.toPath (cfg.sourcePath);
    
    nativeBuildInputs = with pkgs; [
      autoconf automake libtool pkg-config bison flex python3
      cmake grpc protobuf
    ];
    
    buildInputs = with pkgs; [
      readline c-ares json_c libcap libelf elfutils
      openssl sqlite systemd grpc protobuf libunwind
    ];
    
    preConfigure = "./bootstrap.sh";
    
    configureFlags = [
      "--prefix=/usr"
      "--sysconfdir=/etc/frr"
      "--localstatedir=/var/run/frr"
      "--enable-user=frr"
      "--enable-group=frr"
      "--enable-vty-group=frrvty"
      "--enable-multipath=64"
      "--enable-vtysh"
      "--enable-isisd"
      "--enable-bfdd"
      "--enable-bgpd"
      "--enable-zebra"
      "--enable-mgmtd"
      "--enable-staticd"
      "--disable-ospfd"
      "--disable-ospf6d"
      "--disable-ripd"
      "--disable-ripngd"
      "--disable-pimd"
      "--disable-ldpd"
      "--with-pkg-extra-version=-unheaded"
    ];
  };
  
  frrConf = pkgs.writeText "frr.conf" cfg.frrConfig;
  daemonsConf = pkgs.writeText "daemons" ''
    bgpd=yes
    isisd=yes
    bfdd=yes
    zebra=yes
    staticd=yes
    mgmtd=yes
    ospfd=no
    ospf6d=no
    ripd=no
    ripngd=no
    pimd=no
    ldpd=no
    eigrpd=no
    vrrpd=no
  '';

in {
  options.services.unheaded.frr = {
    enable = mkEnableOption "FRR routing suite (BGP EVPN + IS-IS)";
    
    sourcePath = mkOption {
      type = types.str;
      default = "/home/unheaded/tmp/frr-master";
      description = "Path to FRR source (~/tmp/frr-master/)";
    };
    
    bgpAs = mkOption {
      type = types.int;
      default = 65001;
      description = "BGP AS number";
    };
    
    routerId = mkOption {
      type = types.str;
      default = "10.20.255.1";
      description = "BGP router ID (loopback IP)";
    };
    
    isisNet = mkOption {
      type = types.str;
      default = "49.0001.1020.0255.0001.00";
      description = "IS-IS NET address";
    };
    
    wgPeer = mkOption {
      type = types.str;
      default = "fd00:dead:beef::2";
      description = "WireGuard peer IPv6 (host-b for iBGP)";
    };
    
    peerAs = mkOption {
      type = types.int;
      default = 65002;
      description = "Peer BGP AS (host-b)";
    };
    
    vnis = mkOption {
      type = types.listOf types.int;
      default = [ 10001 10002 10100 ];
      description = "VXLAN VNIs to advertise via EVPN";
    };
    
    frrConfig = mkOption {
      type = types.str;
      description = "Full frr.conf content";
      default = ''
        frr version 10.x
        frr defaults datacenter
        hostname forge
        log syslog informational
        service integrated-vtysh-config
        !
        interface lo
         ip address ${cfg.routerId}/32
         ipv6 address fd00:dead:beef:ff::1/128
        !
        interface br-unheaded
         ip address 10.20.0.254/16
         ipv6 address fd00:dead:beef:1::fe/64
        !
        interface wg0
         ip address fd00:dead:beef::1/48
        !
        router isis UNHEADED
         net ${cfg.isisNet}
         is-type level-2-only
         metric-style wide
         log-adjacency-changes
         redistribute ipv4 connected level-2
         redistribute ipv6 connected level-2
         segment-routing on
        !
        interface lo
         ip router isis UNHEADED
         ipv6 router isis UNHEADED
         isis passive
        !
        interface wg0
         ip router isis UNHEADED
         ipv6 router isis UNHEADED
         isis circuit-type level-2-only
         isis metric 10
         isis network point-to-point
        !
        router bgp ${toString cfg.bgpAs}
         bgp router-id ${cfg.routerId}
         bgp log-neighbor-changes
         neighbor ${cfg.wgPeer} remote-as ${toString cfg.peerAs}
         neighbor ${cfg.wgPeer} description host-b-outpost
         neighbor ${cfg.wgPeer} bfd
         !
         address-family ipv4 unicast
          network 10.20.0.0/16
          neighbor ${cfg.wgPeer} activate
         exit-address-family
         !
         address-family ipv6 unicast
          network fd00:dead:beef:1::/64
          neighbor ${cfg.wgPeer} activate
         exit-address-family
         !
         address-family l2vpn evpn
          neighbor ${cfg.wgPeer} activate
          advertise-all-vni
          advertise-default-gw
         exit-address-family
        !
        bfd
         peer ${cfg.wgPeer} interface wg0
          detect-multiplier 3
          receive-interval 300
          transmit-interval 300
        !
        end
      '';
    };
  };

  config = mkIf cfg.enable {
    # Create frr user and group
    users.groups.frr = { gid = 92; };
    users.groups.frrvty = { gid = 85; };
    users.users.frr = {
      uid = 92;
      group = "frr";
      extraGroups = [ "frrvty" ];
      isSystemUser = true;
      description = "FRR routing daemon user";
    };

    # Install FRR
    environment.systemPackages = [ frrPackage ];
    
    # FRR configuration files
    environment.etc."frr/frr.conf".source = frrConf;
    environment.etc."frr/daemons".source = daemonsConf;
    environment.etc."frr/vtysh.conf".text = ''
      service integrated-vtysh-config
      username root nopassword
    '';
    
    # VXLAN VTEP setup (run before FRR starts)
    systemd.services.unheaded-vtep-setup = {
      description = "Set up VXLAN VTEPs for FRR EVPN";
      before = [ "frr.service" ];
      wantedBy = [ "network-online.target" ];
      after = [ "network-online.target" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecStart = pkgs.writeShellScript "vtep-setup" ''
          ${pkgs.iproute2}/bin/ip link set lo up
          for VNI in ${concatStringsSep " " (map toString cfg.vnis)}; do
            VXLAN_IF="vxlan$VNI"
            BR_IF="br-vxlan$VNI"
            if ! ${pkgs.iproute2}/bin/ip link show "$VXLAN_IF" &>/dev/null; then
              ${pkgs.iproute2}/bin/ip link add "$VXLAN_IF" type vxlan \
                id "$VNI" local "${cfg.routerId}" dstport 4789 nolearning
              echo "Created VTEP: $VXLAN_IF (VNI $VNI)"
            fi
            if ! ${pkgs.iproute2}/bin/ip link show "$BR_IF" &>/dev/null; then
              ${pkgs.iproute2}/bin/ip link add "$BR_IF" type bridge
              ${pkgs.iproute2}/bin/ip link set "$BR_IF" up
            fi
            ${pkgs.iproute2}/bin/ip link set "$VXLAN_IF" master "$BR_IF" up || true
          done
        '';
      };
    };

    # FRR systemd service
    systemd.services.frr = {
      description = "FRR routing daemon (BGP EVPN + IS-IS + BFD)";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" "unheaded-vtep-setup.service" ];
      requires = [ "network-online.target" ];
      
      serviceConfig = {
        Type = "forking";
        User = "frr";
        Group = "frr";
        ExecStart = "${frrPackage}/lib/frr/watchfrr -d zebra bgpd isisd bfdd staticd mgmtd";
        ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
        Restart = "on-failure";
        RestartSec = "10s";
        RuntimeDirectory = "frr";
        
        # Capabilities for kernel route management
        AmbientCapabilities = [
          "CAP_NET_ADMIN" "CAP_NET_RAW" "CAP_NET_BIND_SERVICE"
          "CAP_SYS_ADMIN"  # For VXLAN/eBPF
        ];
        CapabilityBoundingSet = [
          "CAP_NET_ADMIN" "CAP_NET_RAW" "CAP_NET_BIND_SERVICE"
          "CAP_SYS_ADMIN"
        ];
        
        NoNewPrivileges = false;  # FRR needs to spawn daemons
      };
    };
    
    # Open FRR management ports (VTY only on localhost)
    networking.firewall.extraCommands = ''
      iptables -A INPUT -p tcp -s 127.0.0.1 --dport 2601 -j ACCEPT  # bgpd
      iptables -A INPUT -p tcp -s 127.0.0.1 --dport 2605 -j ACCEPT  # zebra
      iptables -A INPUT -p tcp -s 127.0.0.1 --dport 2609 -j ACCEPT  # bfdd
    '';
    
    # VXLAN UDP port
    networking.firewall.allowedUDPPorts = [ 4789 ];
  };
}
