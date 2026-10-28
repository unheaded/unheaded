#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# scripts/lib/spdx.sh — shared scope and header rules for scripts/check-spdx.sh
# (the gate) and scripts/spdx-backfill.sh (the fixer). Sourced, not run.
#
# Scope: tracked files only (git ls-files), so build output and untracked
# scratch never count. Excluded by path (SPDX_EXCLUDE_RE):
#   llama.cpp/, doom/, crates/xv6-mbc/upstream/   vendored third-party code: its
#                                                 own licence, never re-headed
#   */target/, node_modules/, vendor/             build output, dependencies
#   .claude/, .agent-scratch/                     agent worktrees and scratch
# Test inputs (*/testdata/, eval/*/fixtures/) and docs/archive/ stay in scope:
# the gate checks them, the backfill never edits them (SPDX_NO_EDIT_RE).
#
# Identifier: GPL-3.0-or-later, except the paths in SPDX_GPL2_RE, whose crate
# manifest declares GPL-2.0-only (ebpf/Cargo.toml workspace.package.license,
# cmd/ebpf-loader/Cargo.toml) and that have no GPL-3.0-or-later sibling file to
# follow. A header records the licence the crate already declares; changing a
# licence is the author's act (S-TRUE-WIRE E4), never this script's.

SPDX_EXCLUDE_RE='^(llama\.cpp|doom|crates/xv6-mbc/upstream|node_modules|vendor|\.claude|\.agent-scratch)/|/(target|node_modules|vendor)/'

# Paths the backfill reports but never edits: a test input's bytes are the test
# (eval fixtures are reviewed by line number), and an archive is history.
# shellcheck disable=SC2034 # used by scripts/spdx-backfill.sh
SPDX_NO_EDIT_RE='/testdata/|^eval/[^/]+/fixtures/|^docs/archive/'

SPDX_GPL2_RE='^(ebpf/(common|flow-tracker|packet-marker|latency-probe)|cmd/ebpf-loader)/'

# shellcheck disable=SC2034 # used by scripts/spdx-backfill.sh
SPDX_COPYRIGHT='Copyright (c) 2024-2026 Stevie Bellis.'

# A header is an SPDX-License-Identifier line in the first 25 lines (room for a
# shebang and a module docstring, not for a script that merely prints one).
SPDX_HEAD_LINES=25

# spdx_files LANGS: print tracked first-party files for a comma list of
# extensions (go,rs,py,sh), one path per line, relative to the repo root.
spdx_files() {
	local langs=$1 globs=() l
	IFS=, read -r -a _langs <<<"$langs"
	for l in "${_langs[@]}"; do
		case "$l" in
		go | rs | py | sh) globs+=("*.$l") ;;
		*)
			echo "spdx: unknown language '$l' (want go,rs,py,sh)" >&2
			return 2
			;;
		esac
	done
	git ls-files -- "${globs[@]}" | grep -vE "$SPDX_EXCLUDE_RE" || true
}

# spdx_has_header FILE: 0 if FILE carries an SPDX identifier near the top.
spdx_has_header() {
	head -n "$SPDX_HEAD_LINES" -- "$1" | grep -qE 'SPDX-License-Identifier: *[A-Za-z0-9]'
}

# spdx_id_for PATH: the identifier a header for PATH should carry.
spdx_id_for() {
	if [[ $1 =~ $SPDX_GPL2_RE ]]; then
		echo "GPL-2.0-only"
	else
		echo "GPL-3.0-or-later"
	fi
}
