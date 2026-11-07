<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# SBOM REVIEW rulings

`sbom/SBOM.md` lists licenses for a person to rule on. Rulings so far, by Stevie Bellis on 2026-11-06:

- GPL-licensed CI tools (ansible-core, yamllint) are fine: they run in CI and do not ship.
- Proprietary NVIDIA wheels stay out of anything shipped.

Every other row in the REVIEW list is still unruled, and the verdict in `sbom/SBOM.md` stays as generated.
