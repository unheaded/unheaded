# SPDX-License-Identifier: GPL-3.0-or-later
#
# template.pkr.hcl — Yggdrasil packer template (Phase 1 scaffolding).
#
# Per ADR-69420 §"Feature B" + OS-FORK-DISCIPLINE.md: builds a hardened
# Debian ISO from the anchor release, applies the overlay patch series,
# runs the CIS/STIG hardening rules, and emits a signed manifest for the
# evidence pack (task #68).
#
# Status: SCAFFOLD — variables and sources defined, but build/provisioner
# blocks are placeholders. The Phase 1 work in task #65 fills these in.
#
# Acceptance gates (from ADR-69420 Micromanager §):
#   - byte-identical ISO on repeat runs (reproducibility)
#   - lynis CIS/STIG SUGGEST items all pass
#   - signed .deb validates with the Yggdrasil GPG key
#   - signed manifest emitted on every build

packer {
  required_version = ">= 1.10.0"
  required_plugins {
    qemu = {
      source  = "github.com/hashicorp/qemu"
      version = "~> 1.0"
    }
  }
}

# Anchor codename (e.g. "bookworm") and version (e.g. "12.7") sourced from
# anchor.nix at build time. The Phase 1 wiring will read anchor.nix via a
# helper script and set these via -var.
variable "debian_codename" {
  type        = string
  description = "Debian release codename per anchor.nix"
  default     = "bookworm"
}

variable "debian_version" {
  type        = string
  description = "Debian point release per anchor.nix (e.g. 12.7)"
  default     = "12.0"  # placeholder until first build
}

variable "yggdrasil_version" {
  type        = string
  description = "Yggdrasil release identifier (bumped on each rebase)"
  default     = "0.0.0-scaffold"
}

variable "iso_url" {
  type        = string
  description = "Upstream Debian netinst ISO URL"
  default     = "https://cdimage.debian.org/cdimage/release/current/amd64/iso-cd/debian-netinst.iso"
}

variable "iso_checksum" {
  type        = string
  description = "Upstream ISO SHA256 (file:URL form for the official sums)"
  default     = "file:https://cdimage.debian.org/cdimage/release/current/amd64/iso-cd/SHA256SUMS"
}

# QEMU source — emits a qcow2 + an ISO build artifact.
# Phase 1 will add aws-amazon, googlecompute, and azure-arm sources for
# the cloud-image targets in task #67.
source "qemu" "yggdrasil-amd64" {
  iso_url      = var.iso_url
  iso_checksum = var.iso_checksum

  output_directory = "build/yggdrasil-amd64"
  vm_name          = "yggdrasil-${var.yggdrasil_version}-amd64"

  disk_size       = "8G"
  format          = "qcow2"
  accelerator     = "kvm"
  cpus            = 4
  memory          = 4096
  net_device      = "virtio-net"
  disk_interface  = "virtio"

  # Reproducibility — disable timestamps in the output by setting the
  # output mtime to a deterministic value. Phase 1 wires this from the
  # anchor commit date.
  shutdown_command = "echo packer | sudo -S /sbin/shutdown -h now"
  ssh_username     = "yggdrasil"
  ssh_password     = "yggdrasil"  # replaced by SSH key on first boot
  ssh_timeout      = "30m"

  boot_wait        = "10s"
  boot_command     = []  # populated when preseed.cfg lands
}

build {
  name = "yggdrasil"
  sources = ["source.qemu.yggdrasil-amd64"]

  # Phase 1 provisioners (placeholders — wired in task #65):
  # - apply quilt overlay patch series from ../overlay/patches/
  # - install Kingdom .deb package set
  # - run CIS hardening (sysctl, filesystem, services, kernel parameters)
  # - run lynis scan, fail build if SUGGEST items remain
  # - install SELinux policy (when task #66 lands)
  # - emit signed manifest (task #68)

  # Discipline gates (run BEFORE any provisioner):
  # - quilt push -a against the anchor MUST succeed
  # - patch count <= 50, total LOC delta <= 5000 (per OS-FORK-DISCIPLINE.md §8)
  # - anchor.nix matches a real upstream Debian release tag

  # post-processor "manifest" {
  #   output = "build/manifest.json"
  #   strip_path = true
  # }
}
