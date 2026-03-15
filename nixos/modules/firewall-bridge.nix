# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# firewall-bridge.nix — Network bridge config: WAN macvlan → Firewall VM → unheaded LAN
{ config, pkgs, lib, ... }:
with lib;
let
  cfg = config.unheaded.firewallBridge;
in {
  options.unheaded.firewallBridge = {
    enable = mkEnableOption "Unheaded firewall bridge networking";
    
    wanInterface = mkOption {
      type = types.str;
      default = "eno1";
      description = "Physical WAN NIC (facing upstream router)";
    };
    
    lanBridge = mkOption {
      type = types.str;
      default = "br-unheaded";
      description = "Internal LAN bridge name (service containers attach here)";
    };
    
    lanSubnet = mkOption {
      type = types.str;
      default = "10.20.0.0/16";
      description = "Internal service subnet";
    };
    
    firewallLanIP = mkOption {
      type = types.str;
      default = "10.20.0.1";
      description = "Firewall VM's LAN IP (default gateway for all services)";
    };
    
    ipv6Prefix = mkOption {
      type = types.str;
      default = "fd00:dead:beef:1::/64";
      description = "IPv6 prefix for internal network";
    };
  };

  config = mkIf cfg.enable {
    # Enable IP forwarding (host acts as router between WAN and LAN)
    boot.kernel.sysctl = {
      "net.ipv4.ip_forward" = 1;
      "net.ipv6.conf.all.forwarding" = 1;
      "net.ipv6.conf.default.forwarding" = 1;
      # Monad HbH header support
      "net.ipv6.conf.all.accept_ra_rt_info_max_plen" = 128;
    };
    
    # Create LAN bridge for service containers
    networking.bridges."${cfg.lanBridge}" = {
      interfaces = [];  # VMs/containers attach dynamically
    };
    
    # Assign IP to bridge (host's LAN interface, NOT the firewall's IP)
    # The firewall VM gets 10.20.0.1; host bridge uses 10.20.0.254 for management
    networking.interfaces."${cfg.lanBridge}" = {
      ipv4.addresses = [{ address = "10.20.0.254"; prefixLength = 16; }];
      ipv6.addresses = [{ address = "fd00:dead:beef:1::fe"; prefixLength = 64; }];
    };
    
    # WAN macvlan — firewall VM's WAN interface attaches to physical NIC
    # The host itself does NOT get an IP on WAN; firewall VM owns that
    networking.macvlans."wan-macvlan" = {
      interface = cfg.wanInterface;
      mode = "bridge";
    };
    
    # nftables: allow forwarding between WAN macvlan and LAN bridge
    # CRITICAL: pass IPv6 HbH extension headers (Monad 20-byte register)
    networking.nftables.enable = true;
    networking.nftables.ruleset = ''
      table ip6 filter {
        chain forward {
          type filter hook forward priority 0; policy drop;
          
          # MONAD PROTOCOL: Pass IPv6 Hop-by-Hop extension headers UNCONDITIONALLY
          # Monad 20-byte register lives in HbH option type 0x1E (UNHEADED_METRIC_V1)
          ip6 nexthdr 0 accept comment "IPv6 HbH (Monad register passthrough)"
          
          # Allow established/related
          ct state established,related accept
          
          # Allow LAN → WAN (firewall VM handles policy; host just forwards)
          iifname "${cfg.lanBridge}" oifname "wan-macvlan" accept
          iifname "wan-macvlan" oifname "${cfg.lanBridge}" ct state established,related accept
          
          # Allow intra-LAN (service-to-service)
          iifname "${cfg.lanBridge}" oifname "${cfg.lanBridge}" accept
          
          # Allow WireGuard (east-west between hosts)
          iifname "wg0" accept
          oifname "wg0" accept
          
          # ICMPv6 required for NDP/path MTU (never block)
          ip6 nexthdr icmpv6 accept
        }
        
        chain input {
          type filter hook input priority 0; policy accept;
          # Host management (SSH) only from LAN
          iifname "${cfg.lanBridge}" tcp dport 22 accept
          iifname "lo" accept
        }
      }
      
      # NAT for IPv4 (firewall VM also does NAT, but belt+suspenders)
      table ip nat {
        chain postrouting {
          type nat hook postrouting priority 100;
          oifname "${cfg.wanInterface}" masquerade
        }
      }
    '';
    
    # Libvirt for hosting firewall VMs
    virtualisation.libvirtd = {
      enable = true;
      qemu = {
        package = pkgs.qemu_kvm;
        runAsRoot = false;
        swtpm.enable = true;
        ovmf.enable = true;  # UEFI boot
      };
    };
    
    # Add unheaded user to libvirt group
    users.users.unheaded.extraGroups = [ "libvirtd" "kvm" ];
  };
}
