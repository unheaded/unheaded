#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-version-flag.sh — every Go binary answers --version as
# "NAME (unheaded) VERSION" with a real version (ADR-111, S-LOW-FRUIT Phase 1).
#
#   scripts/check-version-flag.sh BIN...          check these binaries
#   scripts/check-version-flag.sh --bin-dir DIR   check DIR/<name> for every main package under ./cmd/
#                                                 and every services/ binary build-binaries.sh lists
#   scripts/check-version-flag.sh                 build every main under ./cmd/ into a temp dir with
#                                                 the Makefile's version ldflag, then check them
#
# Fails when a binary is missing, exits non-zero on --version, prints no
# "(unheaded)", or reports "dev" / an empty version.
set -euo pipefail
cd "$(dirname "$0")/.."

# Every main under ./cmd/, plus the service binaries build-binaries.sh ships
# from services/ (wotan, timeguru, captain, architect, micromanager).
svc_pkgs() { scripts/build-binaries.sh --list | awk '$2 ~ /^\.\/services\// {print $2}'; }
mains() {
	go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./cmd/... | sed 's#.*/##'
	svc_pkgs | sed 's#.*/##'
}

bins=()
if [[ $# -eq 0 ]]; then
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	ver=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
	go build -trimpath -ldflags "-X unheaded/pkg/cmdline.Version=${ver}" -o "$tmp/" ./cmd/...
	mapfile -t svc < <(svc_pkgs)
	[[ ${#svc[@]} -gt 0 ]] || { echo "FAIL: build-binaries.sh lists no services/ binaries"; exit 1; }
	go build -trimpath -ldflags "-X unheaded/pkg/cmdline.Version=${ver}" -o "$tmp/" "${svc[@]}"
	while read -r n; do bins+=("$tmp/$n"); done < <(mains)
elif [[ $1 == --bin-dir ]]; then
	dir=${2:?--bin-dir needs a directory}
	while read -r n; do bins+=("$dir/$n"); done < <(mains)
else
	bins=("$@")
fi

fail=0
for b in "${bins[@]}"; do
	name=$(basename "$b")
	if [[ ! -x $b ]]; then
		echo "FAIL $name: no binary at $b"; fail=1; continue
	fi
	if ! out=$("$b" --version 2>&1); then
		echo "FAIL $name: --version exited non-zero: ${out%%$'\n'*}"; fail=1; continue
	fi
	line=${out%%$'\n'*}
	if [[ $line != *"(unheaded) "* ]]; then
		echo "FAIL $name: no '(unheaded) VERSION' in: $line"; fail=1; continue
	fi
	v=${line##*"(unheaded) "}
	if [[ -z $v || $v == dev ]]; then
		echo "FAIL $name: version is '${v:-empty}' (build without the version ldflag?)"; fail=1
	fi
done
if (( fail )); then exit 1; fi
echo "OK: ${#bins[@]} binaries answer --version with a version"
