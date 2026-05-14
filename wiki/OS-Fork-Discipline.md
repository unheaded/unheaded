# OS-FORK-DISCIPLINE

**Status:** Authored 2026-05-10. Phase 1 pipeline (task #65) consumes this when it lights up at Q4 2026.

> Wiki stub generated 2026-05-11. See the canonical doc for full text.

## TL;DR

Yggdrasil (Unheaded OS) is a soft-fork of Debian stable. This doc defines the four pillars that prevent the fork from drifting into an unmaintainable vendored mess.

## The four pillars

1. **Anchor release** — Yggdrasil pins to ONE Debian release at a time (currently `bookworm` = Debian 12). Switching anchors is a Round Table decision.
2. **Overlay patch format** — quilt-format patches under `nix/yggdrasil/overlay/patches/`. Each patch has SPDX + Subject + Reason + Upstream-status + Authored headers. Numbered `NNNN-description.patch`, listed in `series`.
3. **Rebase cadence** — point release within 14 days; security DSA within 7 days (Critical/High) or 30 days (Medium/Low).
4. **Divergence budget** — ≤ 50 patches, ≤ 5,000 LOC delta, ≤ 3 patches per upstream file. CI gates on every build.

## Canonical

[docs/OS-FORK-DISCIPLINE.md](../docs/OS-FORK-DISCIPLINE.md)

## Live state

- Anchor: `bookworm` (Debian 12); version + commit + anchored date TBD at first packer build.
- Overlay patches: 3 (sample CIS hardening — sysctl / sshd / disable-services).
- Patch budget: 3 / 50.
- Pipeline status: scaffolded only; Phase 1 (task #65) lights up the actual packer build.

## Cross-references

- [ADR Index](ADR-Index.md)
- [ADR-69420](ADR-69420-kingdom-bgp-and-unheaded-os.md) — Kingdom BGP + Unheaded OS (the parent ADR)
- [Architecture overview](Architecture.md)
