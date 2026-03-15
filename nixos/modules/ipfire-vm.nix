# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# ipfire-vm.nix — IPFire 2.29 QEMU VM on host-b (Outpost)
# Deploy as WAN firewall with internal LAN bridge to unheaded services
# NOTE: IPFire boots to installer on first run; requires initial configuration
{ config, pkgs, lib, ... }:
with lib;
let
  cfg = config.services.unheaded.ipfire;
in {
  options.services.unheaded.ipfire = {
    enable = mkEnableOption "IPFire 2.29 QEMU VM";
    
    imagePath = mkOption {
      type = types.str;
      default = "/var/lib/unheaded/firewall/ipfire-2.29-core199.img";
      description = "Path to decompressed IPFire image (raw format)";
    };
    
    imageSource = mkOption {
      type = types.str;
      default = "~/tmp/ipfire/ipfire-2.29-core199-x86_64.img.xz";
      description = "Source path to compressed IPFire image (xz format)";
    };
    
    vcpus = mkOption {
      type = types.int;
      default = 2;
      description = "Number of virtual CPUs";
    };
    
    memoryMiB = mkOption {
      type = types.int;
      default = 2048;
      description = "Memory in MiB";
    };
    
    wanMacvlan = mkOption {
      type = types.str;
      default = "wan-macvlan";
      description = "WAN macvlan interface name";
    };
    
    lanBridge = mkOption {
      type = types.str;
      default = "br-unheaded";
      description = "Internal LAN bridge name";
    };
    
    serialPort = mkOption {
      type = types.str;
      default = "ttyS0";
      description = "Serial console device";
    };
    
    autoStart = mkOption {
      type = types.bool;
      default = true;
      description = "Automatically start VM on boot";
    };
  };

  config = mkIf cfg.enable {
    # Ensure directory for VM images exists
    systemd.tmpfiles.rules = [
      "d /var/lib/unheaded/firewall 0750 root libvirtd"
    ];

    # One-shot service to decompress IPFire image from xz
    systemd.services.unheaded-ipfire-prepare = {
      description = "Prepare IPFire VM image (decompress from xz)";
      type = "oneshot";
      before = [ "libvirtd.service" ];
      requires = [ "var-lib-unheaded-firewall.mount" ];
      
      ExecStart = ''
        ${pkgs.bash}/bin/bash -c \
          'if [ ! -f "${cfg.imagePath}" ]; then \
             [ -f "${cfg.imageSource}" ] || exit 1; \
             ${pkgs.xz}/bin/xz -d -k "${cfg.imageSource}" -c > "${cfg.imagePath}"; \
             chmod 640 "${cfg.imagePath}"; \
           fi'
      '';
      
      RemainAfterExit = true;
    };

    # systemd service to manage IPFire VM lifecycle
    systemd.services.unheaded-ipfire = {
      description = "IPFire 2.29 firewall VM";
      after = [ "libvirtd.service" "unheaded-ipfire-prepare.service" ];
      requires = [ "libvirtd.service" "unheaded-ipfire-prepare.service" ];
      wantedBy = mkIf cfg.autoStart [ "multi-user.target" ];
      
      # XML domain definition with dynamic interpolation
      preStart = ''
        mkdir -p /run/unheaded-vms
        cat > /run/unheaded-vms/ipfire-domain.xml << 'XMLEOF'
        <domain type='kvm'>
          <name>unheaded-ipfire</name>
          <memory unit='MiB'>${toString cfg.memoryMiB}</memory>
          <currentMemory unit='MiB'>${toString cfg.memoryMiB}</currentMemory>
          <vcpu placement='static'>${toString cfg.vcpus}</vcpu>
          <os>
            <type arch='x86_64' machine='q35'>hvm</type>
            <bootmenu enable='yes'/>
          </os>
          <features>
            <acpi/>
            <apic/>
          </features>
          <clock offset='utc'>
            <timer name='pit' tickpolicy='delay'/>
            <timer name='hpet' present='no'/>
          </clock>
          <devices>
            <!-- Virtual disk (IPFire rootfs) -->
            <disk type='file' device='disk'>
              <driver name='qemu' type='raw'/>
              <source file='${cfg.imagePath}'/>
              <target dev='vda' bus='virtio'/>
              <boot order='1'/>
            </disk>
            
            <!-- WAN interface: macvlan bridge (attaches to physical NIC) -->
            <interface type='direct'>
              <source dev='${cfg.wanMacvlan}' mode='bridge'/>
              <model type='virtio'/>
              <alias name='wan'/>
            </interface>
            
            <!-- LAN interface: attaches to unheaded service bridge -->
            <interface type='bridge'>
              <source bridge='${cfg.lanBridge}'/>
              <model type='virtio'/>
              <alias name='lan'/>
            </interface>
            
            <!-- Serial console (for debug/setup) -->
            <serial type='pty'>
              <target port='0'/>
            </serial>
            <console type='pty'>
              <target type='serial' port='0'/>
            </console>
          </devices>
        </domain>
        XMLEOF
      '';
      
      ExecStart = ''
        ${pkgs.bash}/bin/bash -c \
          'if ! ${pkgs.libvirt}/bin/virsh dominfo unheaded-ipfire >/dev/null 2>&1; then \
             ${pkgs.libvirt}/bin/virsh define /run/unheaded-vms/ipfire-domain.xml; \
           fi; \
           ${pkgs.libvirt}/bin/virsh start unheaded-ipfire'
      '';
      
      ExecStop = ''
        ${pkgs.libvirt}/bin/virsh shutdown unheaded-ipfire || \
        ${pkgs.libvirt}/bin/virsh destroy unheaded-ipfire
      '';
      
      RemainAfterExit = true;
      Type = "oneshot";
      Restart = "no";
      TimeoutStopSec = "30s";
    };

    # Enable libvirtd (pulled in by firewall-bridge module, but explicit here too)
    virtualisation.libvirtd.enable = true;
  };
}
