<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-148: Yggdrasil as packages: hardening and baseline shipped through the apt repo and enforced by existing services, not a separate image build

**Status:** Proposed (parked; ADR-112 freeze). No plan, no branch, no code.
**Date:** 2026-11-15
**Related:** ADR-69420 (Yggdrasil, Feature B), ADR-043 (Mímir's Law: Gungnir, Gjallarhorn, Heimdall, Enkrateia, Gleipnir), ADR-026 and ADR-085 (debs and artifact layout), ADR-081 (build our own, stay light), ADR-147 (every service earns its place), `nix/yggdrasil/README.md`, `docs/OS-FORK-DISCIPLINE.md`.

## The pitch (Stevie, 2026-11-15, verbatim)

> yggdrasil i think is a upc propgated enforcement of state right? something like that? can't we lean into apt package distribution and orther unheaded services/or add to/modify existing services to get the same functionality?

Context: Jenkins was removed the same day (no JVM, no Java). `nix/yggdrasil/Jenkinsfile` was the only driver of the Yggdrasil image build, so that build has no runner now.

## What the two names mean today (from the tree)

| Name | What it is | State |
|---|---|---|
| Yggdrasil (ADR-69420) | A hardened Debian 12 image builder: packer template, quilt overlay patches (sysctl, sshd, disabled services; 85 added lines in three patches), UPC apt source and tools, CIS provisioner, lynis gate, reproducible ISO/qcow2, signed evidence pack (`cmd/yggdrasil-evidence`, implemented), apt repo publish | Scaffold. No image has ever been built. Runner gone 2026-11-15 |
| Mímir's Law (ADR-043) | The UPC-propagated part: a sealed baseline (Gungnir, ML-DSA-65), a 20-byte Monad trigger (Gjallarhorn), drift detection on the host (Heimdall, eBPF hooks), alerts only (Enkrateia), convergence later (Gleipnir) | Phase 0 PoC; validated west → east 2026-10-28 |

So the pitch is half right: the enforcement of state over UPC is Mímir's Law. Yggdrasil is the image that would have carried it from first boot. ADR-043 v3 joined them ("baked into our Unheaded image, baseline controllable via UPC").

## The proposal

Ship what the image would have baked in as packages from the apt repo we already run, and let the services we already have enforce it:

| Image piece | Package-route replacement | Exists today |
|---|---|---|
| Overlay patches (sysctl, sshd, disabled services) | An `unheaded-baseline` deb: drop-ins under `/etc/sysctl.d`, `/etc/ssh/sshd_config.d`, unit masks; conffiles, so dpkg reports local edits | `scripts/build-all-debs.sh`, `scripts/lib/deb-maintscripts.sh`, `release.yml` |
| UPC apt source and tools | The repo's own source line and the UPC debs | Signed apt repo on west (`runbooks/deploy/apt-repo-publish.yaml`, `deploy/nginx/unheaded-apt.conf`, FND-026 for the move) |
| CIS provisioner, lynis gate | A runbook that applies and then measures; the evidence runs already collect host evidence | S-EVIDENCE-RUNS, AIDE, `lab-audited` profile |
| Baseline sealed at build | Gungnir seal over the installed baseline's manifest | `pkg/gungnir` |
| Drift from the baseline | Heimdall against that seal; Gjallarhorn to ask a host to re-verify | `cmd/heimdall-daemon`, `cmd/gjallarhorn-sender` |
| Signed evidence pack per image | `yggdrasil-evidence` run on a live host instead of on a build output | `cmd/yggdrasil-evidence` |

Gains: no image runner to choose or keep; works on a stock Debian or Ubuntu host an adopter already has (ADR-147's adopter is a small shop, not a fleet built from our ISO); one distribution path (apt) for code and baseline; fewer moving parts, in line with ADR-081's "light, our own".

## What the package route cannot do (an image or installer still would)

- Install-time state: partition layout and mount options (CIS wants separate `/tmp`, `/var`, `/var/log` with `nodev,nosuid,noexec`), full-disk encryption, bootloader settings.
- A guaranteed first boot: a host is hardened only after the deb is installed; before that it is whatever the installer made.
- SELinux from first boot (ADR-69420 task #66); on Debian and Ubuntu AppArmor is native and a deb can ship profiles, which may be enough.
- PXE / bare-metal bring-up at scale (the Sleipnir/Gleipnir scaling substrate). A preseed or autoinstall file that adds our apt source and installs `unheaded-baseline` would cover most of it without packer.

## To settle

**2026-11-18:** Still parked (Stevie Bellis, `docs/battle-plans/DECISION-PACK-2026-11-18.md` round 6, row 24). Status unchanged; the questions below stay open.

1. Keep the packer image at all, or reduce Yggdrasil to "a preseed/autoinstall file + `unheaded-baseline`"?
2. If kept, which runner (GitHub Actions self-hosted on a lab host, or a systemd timer on west)?
3. Does `unheaded-baseline` overwrite site settings (conffiles prompt) or only add drop-ins that sort last?
4. SELinux (task #66) or AppArmor profiles?
5. Order against ADR-147's series (this is not in it).

## Update 2026-11-15: ways to make the PXE seed image (Stevie: it may live outside the repo, as a side project under `~/tmp/projects`)

| Way | What it builds | Needs a VM? | New tools (each needs Stevie's yes) | Fits |
|---|---|---|---|---|
| A. Stock Debian netboot installer + preseed | Nothing to build: Debian's own `netboot.tar.gz` and one preseed file that partitions, adds our apt source and installs `unheaded-baseline` | no | none beyond Debian's own files; a TFTP/HTTP server | Smallest. Installs to disk, so partitions and LUKS are covered. Not an image: every node runs the installer |
| B. `mmdebstrap` → squashfs + kernel + initrd, booted by iPXE over HTTP (`live-boot`, `fetch=`) | A root filesystem image built in a user namespace, no root, no VM; reproducible with `SOURCE_DATE_EPOCH` | no | `mmdebstrap`, `ipxe` (both Debian packages; `debootstrap` and `squashfs-tools` are already on west) | **Recommended.** A real seed image, diskless or installer, built by a script and a timer. Gungnir seals its manifest; the initrd checks the seal before switching root |
| C. `mkosi` (systemd's image builder) with UKI + dm-verity, updates by `systemd-sysupdate` | A signed, read-only image with A/B updates | no | `mkosi` (Python) | Strongest immutability story; more moving parts than B |
| D. `live-build` | ISO and netboot artefacts from a config tree | no | `live-build` | Debian's own, but heavier and slower than B for the same output |
| E. NixOS netboot (`system.build.netbootRamdisk`) | A reproducible ramdisk from `nix/` | no | none new (Nix is already used) | Very reproducible, but it leaves the Debian anchor of ADR-69420 |
| F. Packer, as scaffolded | ISO/qcow2 from a VM | yes (QEMU) | packer (BSL licence), QEMU, a runner | What exists; the heaviest, and it has no runner |

Serving side for any of them: iPXE chain-loaded by the router's or a proxy DHCP (`dnsmasq --dhcp-range=...,proxy` so the router keeps DHCP), files over HTTP from the apt repo's nginx. iPXE can check a signature (`imgverify`, X.509) but not ML-DSA-65: the Gungnir check belongs in the initrd.

Proposed home: a sibling project `~/tmp/projects/unheaded-seed` (like `unheaded-waf`, `unheaded-forge`, `unheaded-terraform`): the build script, the preseed or image config, the iPXE script, a README. Unheaded keeps `unheaded-baseline` and the apt repo; the seed project only consumes them.

