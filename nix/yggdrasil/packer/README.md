# Yggdrasil Packer Templates

Two parallel templates that share the same provisioner flow:

| Template | Source | Target | Task |
|----------|--------|--------|------|
| `template.pkr.hcl` | qemu (kvm) | Local qcow2 + ISO for bare metal | #65 (P1) |
| `cloud-amd64.pkr.hcl` | amazon-ebs, googlecompute, azure-arm | Cloud AMI / GCE image / Azure managed image | #67 (P2) |

Both invoke the same provisioner scripts under `../provisioners/`. The provisioners operate on the running guest, so they're hypervisor-agnostic. Only the **source** layer differs — qemu boots from a Debian netinst ISO via preseed; the cloud sources start from each provider's pre-built Debian base image.

## Build invocations

### Local (qemu)

```bash
cd nix/yggdrasil/packer
packer init template.pkr.hcl
packer build \
    -var "yggdrasil_version=${YGGDRASIL_VERSION}" \
    -var "source_epoch=$(date -u +%s)" \
    template.pkr.hcl
# Output: build/yggdrasil-amd64/*.qcow2 + build/packer-manifest.json + build/evidence-pack-*.tar.gz
```

### Cloud (one or all of AWS / GCP / Azure)

```bash
cd nix/yggdrasil/packer
packer init cloud-amd64.pkr.hcl
# All three:
packer build cloud-amd64.pkr.hcl
# AWS only:
packer build -only="amazon-ebs.yggdrasil-amd64" cloud-amd64.pkr.hcl
# Per-provider credentials must be in env: AWS_PROFILE / GOOGLE_APPLICATION_CREDENTIALS / AZURE_CLIENT_ID...
```

## Provisioner sequence (same for both templates)

| # | Script | What |
|---|--------|------|
| 01 | `01-ssh-and-sudo.sh` | Replace preseed password with SSH key; harden sshd |
| 02 | `02-apply-overlay.sh` | quilt push -a + OS-FORK-DISCIPLINE §8 budget gates |
| 03 | `03-install-upc.sh` | Kingdom apt source + required UPC packages |
| 04 | (inline) | Install `upc-tty-bridge.service` systemd unit |
| 05 | `05-cis-hardening.sh` | CIS L1 baseline (50+ settings) |
| 06 | (inline) | Install `yggdrasil-doctor-upc` to `/usr/local/bin/` |
| 07 | `07-lynis-gate.sh` | BUILD FAILS if hardening < 90 or CIS < 95% |
| 08 | `08-reproducibility-clean.sh` | Deterministic mtimes; wipe transient state |

## Reproducibility contract

Every build environment variable that affects file content gets pinned via `SOURCE_DATE_EPOCH`. Combined with provisioner 08's mtime-touch pass + the deterministic packer manifest, repeated builds against the same anchor + same overlay + same SOURCE_DATE_EPOCH should produce byte-identical artifacts. (Acceptance gate per ADR-69420.)

## Cloud-image differences from local

| Concern | Local (qemu) | Cloud |
|---------|--------------|-------|
| Base image | Debian netinst ISO via preseed | Provider's official Debian-12 base AMI/image |
| Initial credential | preseed password + auto-replaced by 01-ssh-and-sudo.sh | Provider-default SSH user (`admin` on AWS, `yggdrasil` on GCP) |
| Disk size | 8 GB (sized for the LVM layout in preseed) | 16 GB (cloud convention) |
| Partitioning | Custom LVM via preseed (CIS-compliant) | Provider default (resize2fs on first boot) |
| Naming | `yggdrasil-VERSION-amd64.qcow2` | `yggdrasil-VERSION-amd64` (AMI/image/managed-image, per provider) |

## See also

- `../README.md` — directory-level Yggdrasil overview
- `../Jenkinsfile` — CI pipeline that drives both templates
- `../tests/smoke-boot.sh` — qemu smoke harness (local template only; cloud variants smoke via provider-side launch tests)
- `cloud-amd64.pkr.hcl` — cloud template (task #67)
- `template.pkr.hcl` — qemu template (task #65)
