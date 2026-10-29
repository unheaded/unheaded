#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# dep-diff.sh — show what actually changed between two versions of a
# dependency, for the reviewer of a bump (S-LOW-FRUIT Phase 10). Native:
# the Go module cache, or cargo's own vendoring. Not a gate; it prints.
#
#   scripts/dep-diff.sh go MODULE vOLD vNEW        e.g. go github.com/lib/pq v1.10.9 v1.11.0
#   scripts/dep-diff.sh rust CRATE OLD NEW         e.g. rust clap 4.5.20 4.5.21
#
# Rule (docs/policy/dependency-watch.md): a bump of a module marked STALE
# or handover is merged only after this output has been read and the PR
# says what changed and why.
set -euo pipefail
eco=${1:?usage: dep-diff.sh go|rust NAME OLD NEW}
name=${2:?name}
old=${3:?old version}
new=${4:?new version}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

case $eco in
go)
	dir_of() { GOFLAGS=-mod=mod go mod download -json "${name}@$1" | sed -nE 's/^\s*"Dir": "(.*)",?$/\1/p'; }
	sum_of() { GOFLAGS=-mod=mod go mod download -json "${name}@$1" | sed -nE 's/^\s*"Sum": "(.*)",?$/\1/p'; }
	a=$(dir_of "$old"); b=$(dir_of "$new")
	[[ -d $a && -d $b ]] || { echo "dep-diff.sh: could not download both versions" >&2; exit 1; }
	echo "== ${name} ${old} -> ${new}"
	echo "   ${old} $(sum_of "$old")"
	echo "   ${new} $(sum_of "$new")"
	diff -ruN "$a" "$b" || true
	;;
rust)
	# A throwaway crate that depends on each exact version, vendored.
	for v in "$old" "$new"; do
		mkdir -p "$tmp/$v/src"
		printf '[package]\nname = "depdiff"\nversion = "0.0.0"\nedition = "2021"\n[dependencies]\n%s = "=%s"\n' "$name" "$v" >"$tmp/$v/Cargo.toml"
		: >"$tmp/$v/src/lib.rs"
		(cd "$tmp/$v" && cargo vendor --versioned-dirs --quiet vendor >/dev/null)
	done
	echo "== ${name} ${old} -> ${new}"
	diff -ruN "$tmp/$old/vendor/${name}-${old}" "$tmp/$new/vendor/${name}-${new}" || true
	;;
*)
	echo "dep-diff.sh: first argument is go or rust" >&2; exit 2 ;;
esac
