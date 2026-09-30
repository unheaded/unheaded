#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# go.mod replaces github.com/unheaded/doomgeneric with ../projects/doomgeneric/
# unheaded, a sibling checkout that exists on the dev host and nowhere else.
# Without it, anything that loads ./... (govulncheck, gosec, go-licenses) fails
# on cmd/wotan-ctl before scanning a single package. Security Scan had been red
# for that reason since the replace landed (40e75890, 2026-03-11), and the
# gosec docker action hid it further by running Go 1.22 against a 1.25 module.
#
# Clones the public repo at a pinned commit so a CI result is reproducible.
# Bump DOOMGENERIC_SHA deliberately when wotan-ctl needs a newer pkg/doom.
# A no-op when the directory already exists (the dev host).
set -euo pipefail

DOOMGENERIC_REPO="https://github.com/unheaded/doomgeneric.git"
DOOMGENERIC_SHA="8f3e0a80fa6caf4d4fd9775d2ed3742479555182"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="${REPO_ROOT}/../projects/doomgeneric"

if [ -d "${DEST}/unheaded" ]; then
    echo "doomgeneric already present at ${DEST}"
    exit 0
fi

mkdir -p "${DEST}"
git -C "${DEST}" init -q
git -C "${DEST}" remote add origin "${DOOMGENERIC_REPO}"
git -C "${DEST}" fetch -q --depth 1 origin "${DOOMGENERIC_SHA}"
git -C "${DEST}" checkout -q FETCH_HEAD
echo "doomgeneric ${DOOMGENERIC_SHA} checked out at ${DEST}"
