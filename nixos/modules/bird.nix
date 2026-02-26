# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# bird.nix — BIRD Internet Routing Daemon module for host-b Outpost
# Lightweight iBGP peer + BFD + radv (IPv6 Router Advertisement)
{ config, pkgs, lib, ... }:

with lib;

let
  cfg = config.services.unheaded.bird;
  
  # Build BIRD from ~/tmp/bird-master/ source
  birdPackage = pkgs.stdenv.mkDerivation rec {
    pname = "bird";
    version = "2.x-unheaded";
    
    src = /. + builtins.toPath (cfg.sourcePath);
    
    nativeBuildInputs = with pkgs; [
      autoconf automake pkg-config bison flex
    ];
    
    buildInputs = with pkgs; [
      ncurses readline openssl libssh
    ];
    
    preConfigure = "autoreconf -i";
    
    configureFlags = [
      "--prefix=/usr"
      "--sysconfdir=/etc/bird"
      "--localstatedir=/var/run/bird"
      "--with-protocols=bgp,ospf,bfd,static,direct,kernel,device,radv,rpki"
    ];
    
    installFlags = [ "DESTDIR=$(out)" ];
    
    postInstall = ''
      mv $out/usr/* $out/
      rmdir $out/usr
    '';
  };
  
  birdConf = pkgs.writeText "bird.conf" cfg.birdConfig;

in {
  options.services.unheaded.bird = {
    enable = mkEnableOption "BIRD Internet Routing Daemon (BGP + BFD + radv)";
    
    sourcePath = mkOption {
      type = types.str;
      default = "/home/unheaded/tmp/bird-master";
      description = "Path to BIRD source (~/tmp/bird-master/)";
    };
    
    bgpAs = mkOption {
      type = types.int;
      default = 65002;
      description = "BGP AS number";
    };
    
    routerId = mkOption {
      type = types.str;
      default = "10.20.255.2";
      description = "BGP router ID (loopback IP)";
    };
    
    forgePeer = mkOption {
      type = types.str;
      default = "fd00:dead:beef::1";
      description = "iBGP peer IPv6 address (host-a FRR)";
    };
    
    forgeAs = mkOption {
      type = types.int;
      default = 65001;
      description = "Peer BGP AS (host-a)";
    };
    
    raInterface = mkOption {
      type = types.str;
      default = "br-unheaded";
      description = "Interface for IPv6 Router Advertisement (radv)";
    };
    
    ipv6Prefix = mkOption {
      type = types.str;
      default = "fd00:dead:beef:2::/64";
      description = "IPv6 prefix to advertise via radv";
    };
    
    birdConfig = mkOption {
      type = types.str;
      description = "Full bird.conf content";
      default = ''
        log syslog all;
        debug protocols all;
        
        router id ${cfg.routerId};
        
        # Kernel routing table
        protocol kernel {
          ipv4 {
            import all;
            export all;
          };
          ipv6 {
            import all;
            export all;
          };
        }
        
        # Device protocol for interface status
        protocol device {
          scan time 10;
        }
        
        # Static routes for loopback
        protocol static {
          ipv4;
          ipv6;
          route 10.20.255.2/32 via "lo";
          route fd00:dead:beef:ff::2/128 via "lo";
        }
        
        # iBGP with host-a (FRR)
        protocol bgp forge_ebgp {
          local as ${toString cfg.bgpAs};
          neighbor ${cfg.forgePeer} as ${toString cfg.forgeAs};
          
          ipv4 {
            import filter {
              if net ~ 10.0.0.0/8 then accept;
              if net ~ 172.16.0.0/12 then accept;
              if net ~ 192.168.0.0/16 then accept;
              reject;
            };
            export all;
          };
          
          ipv6 {
            import filter {
              if net ~ fd00::/8 then accept;
              reject;
            };
            export all;
          };
          
          # BFD (Bidirectional Forwarding Detection)
          bfd on;
        }
        
        # IPv6 Router Advertisement daemon
        protocol radv {
          interface "${cfg.raInterface}" {
            min ra interval 200 ms;
            max ra interval 600 ms;
            managed on;
            other config on;
            prefix ${cfg.ipv6Prefix} {
              autonomous on;
              on-link on;
              preferred lifetime 2700;
              valid lifetime 3600;
            };
            route ${cfg.ipv6Prefix} {
              lifetime 3600;
              preference high;
            };
            rdnss {
              lifetime 3600;
              ns fd00:dead:beef:ff::1;
            };
          };
        }
      '';
    };
  };

  config = mkIf cfg.enable {
    # Create bird user and group
    users.groups.bird = { gid = 153; };
    users.users.bird = {
      uid = 153;
      group = "bird";
      isSystemUser = true;
      description = "BIRD routing daemon user";
    };

    # Install BIRD
    environment.systemPackages = [ birdPackage ];
    
    # BIRD configuration file
    environment.etc."bird/bird.conf".source = birdConf;
    
    # BIRD systemd service
    systemd.services.bird = {
      description = "BIRD Internet Routing Daemon (BGP + BFD + radv)";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      requires = [ "network-online.target" ];
      
      serviceConfig = {
        Type = "simple";
        User = "bird";
        Group = "bird";
        ExecStart = "${birdPackage}/bin/bird -f -c /etc/bird/bird.conf";
        ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
        Restart = "on-failure";
        RestartSec = "10s";
        RuntimeDirectory = "bird";
        
        # Capabilities for kernel route management
        AmbientCapabilities = [
          "CAP_NET_ADMIN" "CAP_NET_RAW" "CAP_NET_BIND_SERVICE"
        ];
        CapabilityBoundingSet = [
          "CAP_NET_ADMIN" "CAP_NET_RAW" "CAP_NET_BIND_SERVICE"
        ];
        
        NoNewPrivileges = true;
      };
    };
    
    # Open BGP port
    networking.firewall.allowedTCPPorts = [ 179 ];
  };
}
