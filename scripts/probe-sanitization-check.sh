#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# probe-sanitization-check.sh — verify every adversarial finding doc in
# eval/coding-gate/probe-*/ has a sanitized:true frontmatter marker.
#
# Per eval/coding-gate/CONTRIBUTING.md "Sanitization convention" — any
# write-up of an adversarial probe must elide verbatim destructive
# payloads to avoid creating a fresh attack surface inside the doc
# itself (see probe-2026-05-02 meta-finding).
#
# Exit codes:
#   0 — all probe finding-docs (A* and B*) are sanitized
#   1 — at least one finding doc lacks the sanitized:true marker
#
# Run as part of CI; lightweight (no external deps).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

violations=0

# Probe finding docs follow the pattern eval/coding-gate/probe-*/A*.md
# or B*.md (offensive-attack write-ups). E* and SYNTHESIS files are
# meta-experiments and don't need the marker.
for f in eval/coding-gate/probe-*/A*.md eval/coding-gate/probe-*/B*.md; do
    [[ -e "$f" ]] || continue
    if ! head -10 "$f" | grep -q '^sanitized: true$'; then
        echo "MISSING sanitized:true frontmatter — $f" >&2
        violations=$((violations + 1))
    fi
done

if [[ $violations -gt 0 ]]; then
    echo "" >&2
    echo "Sanitization check FAILED ($violations doc(s))." >&2
    echo "See eval/coding-gate/CONTRIBUTING.md for the convention." >&2
    exit 1
fi

echo "Sanitization check passed."
