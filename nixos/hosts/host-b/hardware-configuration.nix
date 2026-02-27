# SPDX-License-Identifier: MIT
# Stub hardware-configuration.nix for host-b (Outpost)
# Replace with `nixos-generate-config --show-hardware-config` output on real hardware
{ config, lib, pkgs, modulesPath, ... }:

{
  imports = [
    (modulesPath + "/profiles/qemu-guest.nix")
  ];

  boot.initrd.availableKernelModules = [ "xhci_pci" "virtio_pci" "virtio_scsi" "usbhid" ];
  boot.initrd.kernelModules = [ ];
  boot.kernelModules = [ ];
  boot.extraModulePackages = [ ];

  fileSystems."/" = {
    device = "/dev/vda1";
    fsType = "ext4";
  };

  swapDevices = [ ];

  nixpkgs.hostPlatform = lib.mkDefault "aarch64-linux";
}
