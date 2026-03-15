# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# opnsense-vm.nix — OPNsense 26.1.2 QEMU VM on host-a (Forge)
# Deploy as WAN firewall with internal LAN bridge to unheaded services
{ config, pkgs, lib, ... }:
with lib;
let
  cfg = config.services.unheaded.opnsense;
in {
  options.services.unheaded.opnsense = {
    enable = mkEnableOption "OPNsense 26.1.2 QEMU VM";
    
    imagePath = mkOption {
      type = types.str;
      default = "/var/lib/unheaded/firewall/opnsense-26.1.2.img";
      description = "Path to decompressed OPNsense image (raw qcow2/raw format)";
    };
    
    imageSource = mkOption {
      type = types.str;
      default = "~/tmp/opnsense/OPNsense-26.1.2-serial-amd64.img.bz2";
      description = "Source path to compressed OPNsense image (bz2 format)";
    };
    
    vcpus = mkOption {
      type = types.int;
      default = 4;
      description = "Number of virtual CPUs";
    };
    
    memoryMiB = mkOption {
      type = types.int;
      default = 4096;
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

    # One-shot service to decompress OPNsense image from bz2
    systemd.services.unheaded-opnsense-prepare = {
      description = "Prepare OPNsense VM image (decompress from bz2)";
      type = "oneshot";
      before = [ "libvirtd.service" ];
      requires = [ "var-lib-unheaded-firewall.mount" ];
      
      ExecStart = ''
        ${pkgs.bash}/bin/bash -c \
          'if [ ! -f "${cfg.imagePath}" ]; then \
             [ -f "${cfg.imageSource}" ] || exit 1; \
             ${pkgs.bzip2}/bin/bunzip2 -k "${cfg.imageSource}" -c > "${cfg.imagePath}"; \
             chmod 640 "${cfg.imagePath}"; \
           fi'
      '';
      
      RemainAfterExit = true;
    };

    # systemd service to manage OPNsense VM lifecycle
    systemd.services.unheaded-opnsense = {
      description = "OPNsense 26.1.2 firewall VM";
      after = [ "libvirtd.service" "unheaded-opnsense-prepare.service" ];
      requires = [ "libvirtd.service" "unheaded-opnsense-prepare.service" ];
      wantedBy = mkIf cfg.autoStart [ "multi-user.target" ];
      
      # XML domain definition with dynamic interpolation
      preStart = ''
        mkdir -p /run/unheaded-vms
        cat > /run/unheaded-vms/opnsense-domain.xml << 'XMLEOF'
        <domain type='kvm'>
          <name>unheaded-opnsense</name>
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
            <!-- Virtual disk (OPNsense rootfs) -->
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
            
            <!-- Serial console (OPNsense image is serial-optimized) -->
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
          'if ! ${pkgs.libvirt}/bin/virsh dominfo unheaded-opnsense >/dev/null 2>&1; then \
             ${pkgs.libvirt}/bin/virsh define /run/unheaded-vms/opnsense-domain.xml; \
           fi; \
           ${pkgs.libvirt}/bin/virsh start unheaded-opnsense'
      '';
      
      ExecStop = ''
        ${pkgs.libvirt}/bin/virsh shutdown unheaded-opnsense || \
        ${pkgs.libvirt}/bin/virsh destroy unheaded-opnsense
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
