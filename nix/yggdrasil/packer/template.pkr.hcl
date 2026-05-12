# SPDX-License-Identifier: GPL-3.0-or-later
#
# template.pkr.hcl — Yggdrasil packer template (Phase 1 scaffolding).
#
# Per ADR-69420 §"Feature B" + OS-FORK-DISCIPLINE.md: builds a hardened
# Debian ISO from the anchor release, applies the overlay patch series,
# runs the CIS/STIG hardening rules, and emits a signed manifest for the
# evidence pack (task #68).
#
# Status: SCAFFOLD — full template flow wired with provisioner placeholders
# pointing at sibling scripts. The Phase 1 work in task #65 fills in the
# actual provisioner shell logic + Jenkins integration. This file is the
# CONTRACT for what the build looks like; it does not yet run end-to-end.
#
# Acceptance gates (from ADR-69420 Micromanager §):
#   - byte-identical ISO on repeat runs (reproducibility)
#   - lynis CIS/STIG SUGGEST items all pass
#   - signed .deb validates with the Yggdrasil GPG key
#   - signed manifest emitted on every build
#
# Free to use. Free to share.

packer {
  required_version = ">= 1.10.0"
  required_plugins {
    qemu = {
      source  = "github.com/hashicorp/qemu"
      version = "~> 1.0"
    }
    # Phase 1+ — cloud image targets (task #67):
    # amazon-ebs, googlecompute, azure-arm. Listed here as documentation
    # of the planned plugin set; uncomment when task #67 starts.
    # amazon = {
    #   source  = "github.com/hashicorp/amazon"
    #   version = "~> 1.3"
    # }
    # googlecompute = {
    #   source  = "github.com/hashicorp/googlecompute"
    #   version = "~> 1.1"
    # }
    # azure = {
    #   source  = "github.com/hashicorp/azure"
    #   version = "~> 2.1"
    # }
  }
}

# ── Variables ────────────────────────────────────────────────────────────────

# Anchor codename (e.g. "bookworm") and version (e.g. "12.7") sourced from
# anchor.nix at build time via scripts/yggdrasil-read-anchor.sh.
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

variable "build_date_utc" {
  type        = string
  description = "Deterministic build timestamp (ISO 8601). Wired from anchor commit date for reproducibility."
  default     = "2026-05-11T00:00:00Z"
}

variable "source_epoch" {
  type        = string
  description = "SOURCE_DATE_EPOCH (Unix seconds) — passed to every reproducible-build-aware tool"
  default     = "1746921600"  # 2026-05-11T00:00:00Z
}

variable "gpg_signing_key_id" {
  type        = string
  description = "Yggdrasil ML-DSA-65 signing key ID (used by post-processor manifest signing)"
  default     = "yggdrasil-build-key"
}

variable "kingdom_apt_url" {
  type        = string
  description = "Kingdom apt repo URL for upc-* packages (task #65 .deb repo)"
  default     = "https://apt.unheaded.dev/yggdrasil"
}

# ── Source ───────────────────────────────────────────────────────────────────

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

  # Reproducibility — boot the autoinstall via preseed, fixed timestamps
  # via SOURCE_DATE_EPOCH passed into every provisioner.
  shutdown_command = "echo packer | sudo -S /sbin/shutdown -h now"
  ssh_username     = "yggdrasil"
  ssh_password     = "yggdrasil"  # replaced by SSH key on first boot via provisioner
  ssh_timeout      = "30m"

  http_directory   = "http"        # serves preseed.cfg via packer's HTTP server
  boot_wait        = "10s"
  boot_command     = [
    "<esc><wait>",
    "auto preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg ",
    "debian-installer=en_US.UTF-8 ",
    "auto=true ",
    "locale=en_US.UTF-8 ",
    "kbd-chooser/method=us ",
    "keyboard-configuration/xkb-keymap=us ",
    "netcfg/get_hostname=yggdrasil ",
    "netcfg/get_domain=unheaded.dev ",
    "fb=false ",
    "debconf/frontend=noninteractive ",
    "console-setup/ask_detect=false ",
    "console-keymaps-at/keymap=us ",
    "grub-installer/bootdev=/dev/vda ",
    "<enter>"
  ]
}

# ── Build ────────────────────────────────────────────────────────────────────

build {
  name    = "yggdrasil"
  sources = ["source.qemu.yggdrasil-amd64"]

  # Phase 1 discipline gates (run BEFORE any in-VM provisioner).
  # The shell-local provisioner runs on the BUILD HOST, not the guest.
  provisioner "shell-local" {
    environment_vars = [
      "SOURCE_DATE_EPOCH=${var.source_epoch}",
      "YGGDRASIL_VERSION=${var.yggdrasil_version}",
      "DEBIAN_CODENAME=${var.debian_codename}",
    ]
    inline = [
      "set -euo pipefail",
      "echo '=== Yggdrasil discipline gates (pre-build) ==='",
      "bash ../scripts/yggdrasil-verify-anchor.sh",
      "bash ../scripts/yggdrasil-verify-overlay.sh",
      "echo '=== Discipline gates passed ==='",
    ]
  }

  # ── In-VM provisioners (run inside the Debian guest) ──────────────────

  # 1. SSH key replacement + sudo lockdown
  provisioner "shell" {
    environment_vars = ["SOURCE_DATE_EPOCH=${var.source_epoch}"]
    execute_command  = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    script           = "../provisioners/01-ssh-and-sudo.sh"
  }

  # 2. Apply Yggdrasil overlay patches (quilt push -a)
  provisioner "file" {
    source      = "../overlay/patches/"
    destination = "/tmp/yggdrasil-overlay/patches/"
  }
  provisioner "shell" {
    execute_command = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/02-apply-overlay.sh"
  }

  # 3. Install Kingdom apt source + upc-* packages (task #71)
  provisioner "file" {
    source      = "../overlay/upc/"
    destination = "/tmp/yggdrasil-upc/"
  }
  provisioner "shell" {
    environment_vars = ["KINGDOM_APT_URL=${var.kingdom_apt_url}"]
    execute_command  = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    script           = "../provisioners/03-install-upc.sh"
  }

  # 4. Install upc-tty-bridge.service + enable
  provisioner "file" {
    source      = "../overlay/systemd/upc-tty-bridge.service"
    destination = "/tmp/upc-tty-bridge.service"
  }
  provisioner "shell" {
    execute_command = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    inline          = [
      "set -euo pipefail",
      "install -m 0644 -o root -g root /tmp/upc-tty-bridge.service /etc/systemd/system/",
      "systemctl daemon-reload",
      "systemctl enable upc-tty-bridge.service",
    ]
  }

  # 5. CIS Level 1 hardening
  provisioner "shell" {
    execute_command = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/05-cis-hardening.sh"
  }

  # 6. yggdrasil-doctor-upc + yggdrasil-evidence install
  provisioner "file" {
    source      = "../bin/yggdrasil-doctor-upc"
    destination = "/tmp/yggdrasil-doctor-upc"
  }
  provisioner "shell" {
    execute_command = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    inline          = [
      "set -euo pipefail",
      "install -m 0755 -o root -g root /tmp/yggdrasil-doctor-upc /usr/local/bin/yggdrasil-doctor-upc",
      # yggdrasil-evidence comes from the apt-installed package (Step 3).
      # No-op confirm:
      "which yggdrasil-doctor-upc",
    ]
  }

  # 7. Final lynis + custom CIS check — build FAILS if score < 95%
  provisioner "shell" {
    execute_command = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/07-lynis-gate.sh"
  }

  # 8. Clean up package cache, temp files, machine-id zeroing
  #    for reproducible final image.
  provisioner "shell" {
    execute_command = "echo 'yggdrasil' | sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/08-reproducibility-clean.sh"
  }

  # ── Post-processors ───────────────────────────────────────────────────

  post-processor "manifest" {
    output     = "build/packer-manifest.json"
    strip_path = true
    custom_data = {
      yggdrasil_version = "${var.yggdrasil_version}"
      debian_codename   = "${var.debian_codename}"
      debian_version    = "${var.debian_version}"
      source_epoch      = "${var.source_epoch}"
      build_date_utc    = "${var.build_date_utc}"
    }
  }

  # Build the signed-manifest evidence pack (task #68) via the shell-local
  # hook that calls cmd/yggdrasil-evidence build-evidence-pack runbook.
  post-processor "shell-local" {
    environment_vars = [
      "SOURCE_DATE_EPOCH=${var.source_epoch}",
      "YGGDRASIL_VERSION=${var.yggdrasil_version}",
      "GPG_KEY_ID=${var.gpg_signing_key_id}",
    ]
    inline = [
      "set -euo pipefail",
      "echo '=== Building signed-manifest evidence pack (task #68) ==='",
      "bash ../scripts/yggdrasil-build-evidence-pack.sh",
      "echo '=== Evidence pack ready ==='",
    ]
  }
}
