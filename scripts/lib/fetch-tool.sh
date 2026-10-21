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
# MEMBER is the binary's path inside the .tar.gz. The archive is cached under
# ${XDG_CACHE_HOME:-~/.cache}/unheaded-tools/NAME-VERSION. A cached archive
# whose hash no longer matches is deleted and the call fails: a swapped file
# must never run. Pins and their freshness: scripts/check-tool-freshness.sh.

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
		curl -fsSL --retry 3 -o "$tarball.part" "$url" || return 2
		mv "$tarball.part" "$tarball"
	fi
	echo "${sha256}  ${tarball}" | sha256sum -c --quiet - >&2 || {
		echo "[FAIL] ${name} ${version} does not match its pinned sha256; deleted" >&2
		rm -f "$tarball" "$cache/$member"
		return 2
	}
	tar -xzf "$tarball" -C "$cache" "$member" || return 2
	echo "$cache/$member"
}
