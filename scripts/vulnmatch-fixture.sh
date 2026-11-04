#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# vulnmatch-fixture.sh: run cmd/vulnmatch over this tree against the synthetic
# OSV and KEV fixtures. Local files only; it proves the reader and matcher end
# to end until the third-party sitting confirms a real OSV source (DP-721).
set -euo pipefail
cd "$(dirname "$0")/.."
exec go run ./cmd/vulnmatch --root . \
	--osv-dir pkg/threatfeed/osv/testdata/records \
	--kev-file pkg/threatfeed/osv/testdata/kev.json "$@"
