#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
# Retired 2026-10-27 (ADR-099 phase 2, S-WELL-SPRING Step 270): unheaded-remediate replaced it on parity.
echo "remediate.sh is retired; run: unheaded-remediate ${*:-list}   (check|plan|apply <fix>, undo <dir>, -record-existing <fix>)" >&2
exit 2
