#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# A unit is a contract with its binary, and systemd checks neither half of it:
# a flag the binary lacks is exit 2 and a restart loop, and a relative default
# path resolved from / is a refused mkdir and a restart loop. East ran both for
# months (unheaded-wotan passing --config, unheaded-timeguru with no working
# directory). This gate reads the source, so it builds and starts nothing:
#
#   1. every flag an ExecStart= passes to a binary scripts/build-binaries.sh
#      builds is declared by that binary's package;
#   2. a deploy/systemd unit whose binary defaults to a ./relative path has a
#      WorkingDirectory= that its own StateDirectory= creates and owns.
#
#   scripts/check-unit-flags.sh            # every tracked unit and unit template
#   scripts/check-unit-flags.sh FILE...    # those files (scripts/test-check-unit-flags.sh)
#
# A flag declared some way this cannot read (a receiver it does not know, a
# call split across lines, another package) fails: teach it, do not skip it.
# Binaries built elsewhere (cargo, third-party) are out of scope. Compose
# `command:` lists are not read.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

declare -A PKG
while read -r name pkg; do
	PKG[$name]=$pkg
done < <(scripts/build-binaries.sh --list)
[[ ${#PKG[@]} -gt 0 ]] || { echo "check-unit-flags: build-binaries.sh listed no binaries" >&2; exit 2; }

if [[ $# -gt 0 ]]; then
	files=("$@")
else
	mapfile -t files < <(git ls-files --cached -- deploy/systemd containers/lxd lxd runbooks nix systemd |
		grep -E '\.(service|ya?ml)$')
fi

# source_has PKG PATTERN: the package's non-test Go source matches. grep reads
# the files itself: under pipefail, `cat | grep -q` reads as no match whenever
# grep stops early and cat takes SIGPIPE.
source_has() {
	local f src=()
	for f in "${1#./}"/*.go; do
		[[ "$f" == *_test.go ]] || src+=("$f")
	done
	grep -qE "$2" "${src[@]}"
}

# declares PKG FLAG: the package registers FLAG with the flag package.
declares() {
	source_has "$1" "\b(flag|flags|fs|fl)\.(Bool|BoolFunc|BoolVar|Duration|DurationVar|Float64|Float64Var|Func|Int|Int64|Int64Var|IntVar|String|StringVar|TextVar|Uint|Uint64|Uint64Var|UintVar|Var)\(([^\"]*, *)?\"$2\","
}

# exec_lines FILE: each ExecStart= with continuations joined, leading blanks gone.
exec_lines() {
	awk '{
		line = $0
		while (line ~ /\\$/ && (getline next_line) > 0) {
			sub(/\\$/, " ", line)
			line = line next_line
		}
		sub(/^[ \t]+/, "", line)
		if (line ~ /^ExecStart=/) print line
	}' "$1"
}

fail=0 execs=0 flags=0 relative=0
for f in "${files[@]}"; do
	while read -r line; do
		read -r -a argv <<<"${line#ExecStart=}"
		[[ ${#argv[@]} -gt 0 ]] || continue
		bin="$(basename "${argv[0]#-}")"
		pkg="${PKG[$bin]:-}"
		[[ -n "$pkg" ]] || continue
		execs=$((execs + 1))
		for arg in "${argv[@]:1}"; do
			arg="${arg#\"}"
			[[ "$arg" =~ ^--?([A-Za-z][A-Za-z0-9._-]*) ]] || continue
			flags=$((flags + 1))
			if ! declares "$pkg" "${BASH_REMATCH[1]}"; then
				echo "[FAIL] $f: $bin has no flag -${BASH_REMATCH[1]} (declared in $pkg)"
				fail=1
			fi
		done

		[[ "$f" == *.service ]] || continue
		source_has "$pkg" '"\./[A-Za-z]' || continue
		relative=$((relative + 1))
		wd="$(sed -n 's/^WorkingDirectory=-\{0,1\}//p' "$f" | tail -n 1)"
		state=" $(sed -n 's/^StateDirectory=//p' "$f" | tr '\n' ' ') "
		if [[ "$wd" != /var/lib/* || "$state" != *" ${wd#/var/lib/} "* ]]; then
			echo "[FAIL] $f: $bin defaults to a ./relative path, so it needs a WorkingDirectory= under /var/lib that StateDirectory= creates (have '${wd:-none}')"
			fail=1
		fi
	done < <(exec_lines "$f")
done

if [[ $execs -eq 0 ]]; then
	echo "check-unit-flags: no ExecStart= of a binary built here; refusing to pass vacuously" >&2
	exit 2
fi
[[ $fail -eq 0 ]] || exit 1
echo "[PASS] ${execs} ExecStart lines pass ${flags} flags their binaries declare; ${relative} with relative defaults run from their own state directory"
