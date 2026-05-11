# OS-FORK-DISCIPLINE

**Owner:** unheaded-architect (canonical) + unheaded-developer (impl)
**Authored:** 2026-05-10
**Scope:** Yggdrasil (Unheaded OS) per ADR-69420, Phase 1 prerequisites.
**Closes task:** #69 (Yggdrasil P0 — soft-fork upstream-tracking discipline).

---

## 1. Why this exists

ADR-69420 §"Feature B" calls Yggdrasil "a hardened Debian image builder pipeline that maintains soft fork alignment: tracks upstream Debian stable with overlay patches." That sentence asserts a discipline without defining it. This doc is the definition.

A soft fork without an explicit discipline drifts into a vendored mess:
- Upstream security patches don't land because nobody knows when to rebase.
- Overlay patches accumulate without provenance.
- Fork divergence grows unboundedly until reconciliation is too expensive.
- Compliance evidence (FedRAMP, SOC2) breaks because the chain-of-custody from upstream → Yggdrasil image is opaque.

This doc defines the four discipline pillars that prevent each failure mode.

## 2. Pillar 1 — Anchor release

Yggdrasil anchors to a single Debian release at a time. The current anchor is recorded in this doc and in `nix/yggdrasil/anchor.nix` (created in Phase 1).

| Anchor field | Initial value (2026-05-10) | Update trigger |
|--------------|----------------------------|----------------|
| Release codename | `bookworm` (Debian 12) | When Debian 13 (`trixie`) reaches stable |
| Release version | `12.x` (current point release at fork time) | When upstream issues a new point release |
| Release date | TBD at first build | Frozen at fork time |
| Anchor commit (Debian git) | TBD at first build | Updated only via the rebase cadence (§4) |

**Switching anchors is a Round Table decision.** Going from `bookworm` to `trixie` is a multi-month transition with overlay-patch reconciliation; it cannot happen via a casual commit.

## 3. Pillar 2 — Overlay patch format

All Yggdrasil-specific changes against upstream Debian live as **quilt-format patches** under `nix/yggdrasil/overlay/patches/`. Each patch:

1. Has a `.patch` extension.
2. Has a header block with `# Subject:`, `# Reason:`, `# Upstream-status:`, `# Authored:` lines.
3. Targets a specific upstream path (e.g. `etc/sysctl.conf`, `usr/lib/systemd/system/sshd.service`).
4. Is named `NNNN-short-description.patch` where NNNN is a four-digit sequence number (0001, 0002, ...).
5. Is reviewed under the same SBOM/license discipline as in-tree code (SPDX header, GPL-2.0-or-later for kernel/glibc derivatives).

**No ad-hoc edits to vendored upstream files.** Every divergence is a numbered, headed, reviewed patch. The packer/Jenkins pipeline (Phase 1, task #65) applies the patch series in numeric order and fails the build if any patch fails to apply.

`series` file at `nix/yggdrasil/overlay/patches/series` lists the patches in apply order. Adding/removing patches updates this file in the same commit as the patch itself.

## 4. Pillar 3 — Rebase cadence

Yggdrasil rebases against the upstream anchor on **two triggers**, whichever comes first:

| Trigger | Cadence | Owner |
|---------|---------|-------|
| Upstream Debian point release (`12.x` → `12.x+1`) | Within 14 days of release | unheaded-architect |
| Upstream security advisory (DSA) for any package in the Yggdrasil set | Within 7 days for Critical/High; within 30 days for Medium/Low | unheaded-moatghost |

**Rebase procedure (informational; pipeline-automated in Phase 1):**

1. `git fetch debian-upstream`
2. `git checkout -b rebase/12.x+1 yggdrasil-main`
3. `git rebase debian/12.x+1` — resolve conflicts patch-by-patch
4. `make -C nix/yggdrasil verify` — packer build + lynis scan + signed-manifest check
5. PR against `yggdrasil-main` with the new anchor pointer.

If a rebase requires modifying more than 25% of the overlay patches to keep applying, that's a **divergence-budget alarm** (Pillar 4) and triggers Round Table.

## 5. Pillar 4 — Divergence budget

The fork's divergence from upstream is measured as the **count of overlay patches** plus the **total LOC delta** they represent. The budget is:

| Metric | Threshold | Action when exceeded |
|--------|-----------|----------------------|
| Patch count | > 50 | Architect review; consolidate where possible |
| Total LOC delta | > 5,000 | Architect review; evaluate vendoring sub-components instead |
| Patches against a single upstream file | > 3 | Refactor: vendor the file as a Yggdrasil-owned replacement instead of patching |
| Rebase conflicts touching > 25% of patches | per rebase | Round Table — anchor switch may be cheaper than continued maintenance |

The pipeline (task #65) emits these metrics on every build; CI gates (task #68 evidence-pack) fail the build when any threshold is exceeded.

## 6. License + provenance

| Source | License | Yggdrasil treatment |
|--------|---------|---------------------|
| Debian base packages | mixed (mostly GPL-2.0 / LGPL-2.1 / Apache-2.0 / MIT) | Carried unchanged; license headers preserved in overlay-patched files |
| Yggdrasil overlay patches | GPL-2.0-or-later (consistent with kernel/glibc derivatives) | SPDX header on every patch file |
| SELinux policy port (task #66) | GPL-2.0 (consistent with RHEL reference policy) | Documented in `COPYING.SELinux` per ADR-69420 Barrister §"License compatibility" |

The signed-manifest evidence pack (task #68) emits a per-build SBOM that names every upstream package version, every overlay patch, and every Yggdrasil-authored binary. This is the chain-of-custody artifact that compliance frameworks need.

## 7. Out of scope (explicitly)

- **Kernel patches:** Yggdrasil ships Debian's stock kernel. Custom kernel work lives in the parent ASCEND-LINUX track (task #50, Phase 4 of `references/battle-plan-ascend-linux-2026-05-08.md`), NOT in the Yggdrasil overlay.
- **Userspace package selection:** the package set is owned by the packer pipeline config (task #65), not this doc. This doc covers HOW we maintain divergence, not WHICH packages we ship.
- **Cloud image variants (AMI/GCE/Azure):** task #67 — same overlay applies, packer just emits different output formats.

## 7.5. Pillar 5 — UPC integration (task #71)

Yggdrasil exists to be the host that runs UPC. This is non-negotiable, not optional. Every Yggdrasil image MUST ship the Unheaded Protocol Computer tooling installed, enabled, and self-checking out of the box.

| Surface | Required artifact | Owner |
|---------|-------------------|-------|
| `upc-bootctl` CLI | `/usr/bin/upc-bootctl` from `cmd/upc-bootctl/` Go build | task #65 packer |
| `upc-tty-bridge` CLI | `/usr/bin/upc-tty-bridge` from `cmd/upc-tty-bridge/` Go build | task #65 packer |
| Mode A demo unit | `upc-tty-bridge.service` enabled at boot, listening on 127.0.0.1:26100 | `nix/yggdrasil/overlay/systemd/` |
| Browser xterm assets | `/opt/unheaded/share/upc-console.html` + vendored `xterm.js` | `unheaded-shared` deb |
| BPF program | `/opt/unheaded/share/monad-cpu-ebpf.o` (built `--features ascend-linux`, pinned to anchor kernel ABI) | `monad-cpu-ebpf` deb |
| Self-check | `yggdrasil-doctor upc` exits 0 on a fresh boot | `nix/yggdrasil/bin/yggdrasil-doctor-upc` |

**Discipline invariants for UPC integration:**

- [ ] All five surfaces listed above ship in the image (build fails otherwise).
- [ ] `which upc-bootctl` returns 0 in the image-build smoke test.
- [ ] `systemctl is-enabled upc-tty-bridge` returns `enabled` in the smoke test.
- [ ] `yggdrasil-doctor upc` exits 0 on the first boot of a fresh image.
- [ ] The shipped `monad-cpu-ebpf.o` was built against the same kernel ABI as the anchor (avoid kernel-version drift between BPF compile and runtime).
- [ ] When the apt-source patch (`overlay/upc/0001-add-upc-apt-source.patch`) lands, the GPG key `unheaded-upc.gpg` is preinstalled at `/etc/apt/trusted.gpg.d/`.

**Why this is a pillar, not a packer line:** because if Yggdrasil ships without UPC, Yggdrasil is "just another hardened Debian." With UPC, Yggdrasil is the only distro where `upc-bootctl boot xv6 --instance 222` is a one-liner from a fresh install. That's the entire reason this fork exists.

## 8. Discipline invariants (CI-checkable)

The Phase 1 pipeline (task #65) MUST gate the build on these:

- [ ] Every `nix/yggdrasil/overlay/patches/*.patch` has SPDX header + Subject/Reason/Upstream-status/Authored lines.
- [ ] `nix/yggdrasil/overlay/patches/series` lists all patches in apply order; no orphans.
- [ ] `quilt push -a` succeeds against the current anchor without manual intervention.
- [ ] Patch count <= 50.
- [ ] Total LOC delta <= 5,000.
- [ ] No upstream file has > 3 patches against it.
- [ ] Anchor pointer in `nix/yggdrasil/anchor.nix` matches a real upstream Debian release tag.
- [ ] Last rebase against the anchor was < 30 days ago (warning) / < 90 days ago (error).

## 9. References

- ADR-69420 — "Kingdom-Native BGP Routing Daemon + Unheaded OS"
- `references/battle-plan-NORTH-STAR-2026-05-05.md` §"Q4 2026 Yggdrasil track"
- `nix/yggdrasil/overlay/upc/README.md` — UPC overlay specifics (task #71)
- `nix/yggdrasil/overlay/systemd/upc-tty-bridge.service` — Mode A unit
- `nix/yggdrasil/bin/yggdrasil-doctor-upc` — UPC preflight self-check
- Debian `policy.debian.org` §"Source package format" (quilt-3.0 reference)
- RHEL `selinux-policy` upstream (target for the policy port in task #66)
