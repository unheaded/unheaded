# SPDX-License-Identifier: GPL-3.0-or-later
#
# anchor.nix — Yggdrasil's pinned upstream Debian release.
#
# Per OS-FORK-DISCIPLINE.md §"Pillar 1 — Anchor release", Yggdrasil tracks
# ONE Debian release at a time. Switching anchors (e.g. bookworm -> trixie)
# is a Round Table decision, not a casual commit.
#
# The Phase 1 packer pipeline (task #65) reads this file to know which
# upstream to pull from; the discipline CI gate (per OS-FORK-DISCIPLINE.md §8)
# verifies the anchor pointer matches a real upstream Debian release tag and
# that the last rebase against this anchor was < 30 days ago.

{
  # Debian release codename. Update via Round Table decision when transitioning
  # between major releases (e.g. bookworm -> trixie when trixie reaches stable).
  codename = "bookworm";

  # Debian release version. Update via the rebase cadence when upstream issues
  # a new point release (within 14 days per OS-FORK-DISCIPLINE.md §"Pillar 3").
  # TBD at first packer build — placeholder until Phase 1 lights up.
  version = null; # e.g. "12.7" once the first build lands

  # Anchor commit in the Debian git mirror. Locked at fork time and updated
  # only via the rebase cadence (NOT by ad-hoc edits).
  commit = null; # e.g. "<sha>" once the first build lands

  # Date the current anchor was set. Used by the discipline CI gate to compute
  # rebase staleness.
  anchored = null; # e.g. "2026-12-01"

  # URL of the upstream Debian git mirror Yggdrasil pulls from.
  upstream = "https://salsa.debian.org/installer-team/debian-installer.git";

  # Yggdrasil release identifier — bumped on each rebase.
  yggdrasil_version = "0.0.0-scaffold";
}
