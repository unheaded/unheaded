#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# fetch-tool.sh — download a release-pinned tool once, verify its sha256 on
# every use, print the path of the binary. Sourced, not run.
#
#   source scripts/lib/fetch-tool.sh
#   bin="$(fetch_tool NAME VERSION URL SHA256 MEMBER)"
#
# MEMBER is the binary's path inside the .tar.gz, or "-" when the release
# asset is the bare binary itself (cosign). The archive is cached under
# ${XDG_CACHE_HOME:-~/.cache}/unheaded-tools/NAME-VERSION. A cached archive
# whose hash no longer matches is deleted and the call fails: a swapped file
# must never run. Pins and their freshness: scripts/check-tool-freshness.sh.
#
# The cache is shared by every gate on a host and gates run side by side, so
# the binary is unpacked beside the cache and renamed in: writing it in place
# made another caller's run of it exit 126 or 127, which reads as a failed gate
# (scripts/test-fetch-tool.sh).

fetch_tool() {
	local name="$1" version="$2" url="$3" sha256="$4" member="$5"
	local cache="${XDG_CACHE_HOME:-$HOME/.cache}/unheaded-tools/${name}-${version}"
	local tarball="$cache/${name}.tar.gz"
	[[ "$(uname -sm)" == "Linux x86_64" ]] || {
		echo "[FAIL] ${name} is pinned for Linux x86_64 only, not $(uname -sm)" >&2
		return 2
	}
	mkdir -p "$cache"
	if [[ ! -f "$tarball" ]]; then
		curl -fsSL --retry 3 -o "$tarball.part.$$" "$url" || { rm -f "$tarball.part.$$"; return 2; }
		mv "$tarball.part.$$" "$tarball"
	fi
	echo "${sha256}  ${tarball}" | sha256sum -c --quiet - >&2 || {
		echo "[FAIL] ${name} ${version} does not match its pinned sha256; deleted" >&2
		rm -f "$tarball" "$cache/$member"
		return 2
	}
	local staging out rc=0
	staging="$(mktemp -d "$cache/.extract.XXXXXX")" || return 2
	if [[ "$member" == "-" ]]; then
		out="$name"
		install -m 0755 "$tarball" "$staging/$out" || rc=2
	else
		out="$member"
		tar -xzf "$tarball" -C "$staging" "$member" || rc=2
	fi
	if [[ "$rc" -eq 0 ]]; then
		mkdir -p "$(dirname "$cache/$out")" && mv -f "$staging/$out" "$cache/$out" || rc=2
	fi
	rm -rf "$staging"
	[[ "$rc" -eq 0 ]] || return "$rc"
	echo "$cache/$out"
}
