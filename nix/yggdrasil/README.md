# Yggdrasil — Unheaded OS (hardened Debian soft-fork)

**Status:** P0 scaffolding (2026-05-10). Phase 1 (packer + Jenkins + signed .deb repo) targets Q4 2026 per ADR-69420.
**Owner:** unheaded-architect (canonical) + unheaded-developer (impl) + unheaded-moatghost (compliance gates).
**Discipline:** see `docs/OS-FORK-DISCIPLINE.md` (the four pillars: anchor / overlay format / rebase cadence / divergence budget).
**Source ADR:** `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` §"Feature B".

---

## Layout

```
nix/yggdrasil/
├── README.md                this file
├── anchor.nix               pinned Debian release + commit (Pillar 1)
├── overlay/
│   └── patches/
│       ├── series           apply-order list (Pillar 2)
│       └── *.patch          numbered overlay patches against upstream
└── packer/
    └── template.pkr.hcl     reproducible ISO build template (Phase 1)
```

## Status by sub-task

| Task | Owner | Phase | Status |
|------|-------|-------|--------|
| #69 OS-FORK-DISCIPLINE doc | architect | P0 | ✅ closed 2026-05-10 |
| #65 Debian hardening pipeline (packer + Jenkins + .deb repo) | architect + developer | P1 (Q4 2026) | 🚧 scaffolding |
| #68 signed-manifest + audit-trail evidence pack | moatghost | P1 (Q4 2026) | 📋 pending |
| #66 SELinux policy port (RHEL → Debian) | architect + developer | P2 (Age 3a) | 📋 pending — blocked on #65 |
| #67 cloud image targets (AMI / GCE / Azure / qcow2) | architect | P2 | 📋 pending — blocked on #65 |

## Phase 1 acceptance gates (per ADR-69420 Micromanager §)

- [ ] `packer build` produces byte-identical ISOs on repeat runs (reproducibility).
- [ ] `lynis` CIS/STIG scan passes all SUGGEST items.
- [ ] Signed .deb validates with the Yggdrasil GPG key on install.
- [ ] Jenkins pipeline triggers on .deb package updates.
- [ ] Signed manifest emitted on every build (chain-of-custody for compliance).

## Discipline gates (CI-checked, per OS-FORK-DISCIPLINE.md §8)

- [ ] Every overlay patch has SPDX header + Subject/Reason/Upstream-status/Authored lines.
- [ ] `series` lists all patches in apply order; no orphans.
- [ ] `quilt push -a` succeeds against the current anchor.
- [ ] Patch count ≤ 50.
- [ ] Total LOC delta ≤ 5,000.
- [ ] No upstream file has > 3 patches against it.
- [ ] `anchor.nix` matches a real upstream Debian release tag.
- [ ] Last rebase against the anchor < 30 days (warn) / < 90 days (error).

## What this scaffolding does NOT do (yet)

- Build any actual ISO.
- Define the Kingdom .deb package set.
- Wire the Jenkins pipeline.
- Emit the signed manifest.
- Implement any SELinux policy.

That work lands when #65 is actively executed (currently Q4 2026 per the timeline). The scaffolding here just establishes the directory layout, the discipline anchor, and the empty patch series so additions land in the right shape from day one.

## See also

- `docs/OS-FORK-DISCIPLINE.md` — the four pillars
- `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` — the Feature B mandate
- `references/battle-plan-NORTH-STAR-2026-05-05.md` §"Q4 2026 Yggdrasil track"
