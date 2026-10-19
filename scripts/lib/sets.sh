#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# sets.sh — comm(1) over two newline-separated sets held in variables.
# Source it, then:
#
#   added="$(set_comm -23 "$current" "$baseline")"     # only in the first
#   gone="$(set_comm -13 "$current" "$baseline")"      # only in the second
#   both="$(set_comm -12 "$a" "$b")"                   # in both
#
# Both sets must already be sorted (sort -u), as for comm itself.
#
# Why not `comm -23 <(printf ...) <(printf ...)`: the comm on west (uutils
# coreutils 0.2.2, Ubuntu's coreutils-from-uutils) misreads input when BOTH
# operands are pipes and the input passes about 8 KB. It prints nothing or
# a truncated line ("not in sorted order", or no message at all). Measured
# 2026-10-19: identical input as two files, or one file and one pipe, is
# right every time. A ratchet built on the pipe form passes whatever it is
# given on that host. So both sides go through temporary files.
set_comm() {
	local flags="$1" a b rc
	a="$(mktemp)" || return 2
	b="$(mktemp)" || { rm -f "$a"; return 2; }
	# grep exits 1 on an empty set; that is not an error here, even under set -e.
	{ printf '%s\n' "$2" | grep . || true; } >"$a"
	{ printf '%s\n' "$3" | grep . || true; } >"$b"
	comm "$flags" "$a" "$b"
	rc=$?
	rm -f "$a" "$b"
	return "$rc"
}
