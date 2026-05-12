# SPDX-License-Identifier: GPL-3.0-or-later
#
# cloud-amd64.pkr.hcl — Yggdrasil cloud image targets (task #67).
#
# Adds AMI, GCE, and Azure-arm sources that share the same provisioner
# flow as template.pkr.hcl (the qemu/qcow2 path). The provisioners
# inside provisioners/ work unchanged across all sources because they
# operate on the running guest, not on the hypervisor layer.
#
# Status: SCAFFOLD. Each cloud source uses placeholder credentials and
# requires per-cloud secret injection at runtime. Task #67 fills in the
# actual credential plumbing (Vault / AWS Secrets Manager / etc.).
#
# Free to use. Free to share.

packer {
  required_version = ">= 1.10.0"
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1.3"
    }
    googlecompute = {
      source  = "github.com/hashicorp/googlecompute"
      version = "~> 1.1"
    }
    azure = {
      source  = "github.com/hashicorp/azure"
      version = "~> 2.1"
    }
  }
}

# ── Shared variables (overlapping with template.pkr.hcl) ─────────────────────

variable "yggdrasil_version" {
  type        = string
  description = "Yggdrasil release identifier"
  default     = "0.0.0-scaffold"
}

variable "source_epoch" {
  type        = string
  description = "SOURCE_DATE_EPOCH (Unix seconds)"
  default     = "1746921600"
}

variable "build_date_utc" {
  type        = string
  description = "ISO 8601 deterministic build timestamp"
  default     = "2026-05-11T00:00:00Z"
}

# ── AWS / EC2 AMI ────────────────────────────────────────────────────────────

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "aws_instance_type" {
  type    = string
  default = "t3.medium"
}

variable "aws_base_ami_owner" {
  type    = string
  default = "136693071363"  # Debian's official owner ID
}

source "amazon-ebs" "yggdrasil-amd64" {
  region          = var.aws_region
  instance_type   = var.aws_instance_type
  ami_name        = "yggdrasil-${var.yggdrasil_version}-amd64-${var.build_date_utc}"
  ami_description = "Yggdrasil ${var.yggdrasil_version} — hardened Debian + UPC tooling"

  source_ami_filter {
    filters = {
      virtualization-type = "hvm"
      name                = "debian-12-amd64-*"
      root-device-type    = "ebs"
    }
    owners      = [var.aws_base_ami_owner]
    most_recent = true
  }

  ssh_username = "admin"  # Debian cloud images default user
  ssh_timeout  = "10m"

  # Reproducibility — fixed launch_block_device for byte-identical AMIs
  launch_block_device_mappings {
    device_name           = "/dev/xvda"
    volume_size           = 16
    volume_type           = "gp3"
    delete_on_termination = true
  }

  tags = {
    Name              = "yggdrasil-${var.yggdrasil_version}"
    YggdrasilVersion  = var.yggdrasil_version
    BuildDateUTC      = var.build_date_utc
    SourceEpoch       = var.source_epoch
    "unheaded.dev/free-to-use" = "true"
  }
}

# ── GCP / Compute Engine ─────────────────────────────────────────────────────

variable "gcp_project_id" {
  type    = string
  default = "yggdrasil-build"  # placeholder; injected at build time
}

variable "gcp_zone" {
  type    = string
  default = "us-central1-a"
}

source "googlecompute" "yggdrasil-amd64" {
  project_id          = var.gcp_project_id
  source_image_family = "debian-12"
  zone                = var.gcp_zone
  machine_type        = "n2-standard-2"

  image_name          = "yggdrasil-${replace(var.yggdrasil_version, ".", "-")}-amd64"
  image_family        = "yggdrasil"
  image_description   = "Yggdrasil ${var.yggdrasil_version} — hardened Debian + UPC tooling"

  ssh_username = "yggdrasil"
  ssh_timeout  = "10m"

  disk_size = 16
  disk_type = "pd-ssd"

  labels = {
    yggdrasil_version = replace(var.yggdrasil_version, ".", "-")
    build_date_utc    = replace(var.build_date_utc, ":", "-")
    free_to_use       = "true"
  }
}

# ── Azure / ARM ──────────────────────────────────────────────────────────────

variable "azure_subscription_id" {
  type    = string
  default = "00000000-0000-0000-0000-000000000000"  # placeholder
}

variable "azure_location" {
  type    = string
  default = "eastus"
}

variable "azure_resource_group" {
  type    = string
  default = "yggdrasil-build-rg"
}

source "azure-arm" "yggdrasil-amd64" {
  subscription_id                   = var.azure_subscription_id
  managed_image_name                = "yggdrasil-${var.yggdrasil_version}-amd64"
  managed_image_resource_group_name = var.azure_resource_group

  os_type                = "Linux"
  image_publisher        = "Debian"
  image_offer            = "debian-12"
  image_sku              = "12-gen2"
  location               = var.azure_location
  vm_size                = "Standard_D2s_v3"

  azure_tags = {
    yggdrasil_version = var.yggdrasil_version
    build_date_utc    = var.build_date_utc
    free_to_use       = "true"
  }
}

# ── Build (all 3 sources share the same provisioner flow) ────────────────────

build {
  name = "yggdrasil-cloud"
  sources = [
    "source.amazon-ebs.yggdrasil-amd64",
    "source.googlecompute.yggdrasil-amd64",
    "source.azure-arm.yggdrasil-amd64",
  ]

  # Same provisioner chain as template.pkr.hcl. The cloud variants skip
  # the preseed/autoinstall step (the base AMI/image is already provisioned)
  # and jump straight to overlay + UPC + CIS hardening.
  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/01-ssh-and-sudo.sh"
  }

  provisioner "file" {
    source      = "../overlay/patches/"
    destination = "/tmp/yggdrasil-overlay/patches/"
  }
  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/02-apply-overlay.sh"
  }

  provisioner "file" {
    source      = "../overlay/upc/"
    destination = "/tmp/yggdrasil-upc/"
  }
  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/03-install-upc.sh"
  }

  provisioner "file" {
    source      = "../overlay/systemd/upc-tty-bridge.service"
    destination = "/tmp/upc-tty-bridge.service"
  }
  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    inline          = [
      "set -euo pipefail",
      "install -m 0644 -o root -g root /tmp/upc-tty-bridge.service /etc/systemd/system/",
      "systemctl daemon-reload",
      "systemctl enable upc-tty-bridge.service",
    ]
  }

  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/05-cis-hardening.sh"
  }

  provisioner "file" {
    source      = "../bin/yggdrasil-doctor-upc"
    destination = "/tmp/yggdrasil-doctor-upc"
  }
  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    inline          = [
      "set -euo pipefail",
      "install -m 0755 -o root -g root /tmp/yggdrasil-doctor-upc /usr/local/bin/yggdrasil-doctor-upc",
    ]
  }

  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/07-lynis-gate.sh"
  }

  provisioner "shell" {
    execute_command = "sudo -S env {{ .Vars }} {{ .Path }}"
    script          = "../provisioners/08-reproducibility-clean.sh"
  }

  post-processor "manifest" {
    output     = "build/cloud-packer-manifest.json"
    strip_path = true
    custom_data = {
      yggdrasil_version = "${var.yggdrasil_version}"
      source_epoch      = "${var.source_epoch}"
      build_date_utc    = "${var.build_date_utc}"
      sources_built     = "amazon-ebs,googlecompute,azure-arm"
    }
  }

  post-processor "shell-local" {
    environment_vars = [
      "SOURCE_DATE_EPOCH=${var.source_epoch}",
      "YGGDRASIL_VERSION=${var.yggdrasil_version}",
      "BUILD_VARIANT=cloud",
    ]
    inline = [
      "echo '=== Building cloud-variant evidence pack ==='",
      "bash ../scripts/yggdrasil-build-evidence-pack.sh",
    ]
  }
}
