#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-upc-regression-build.sh — upc-regression.sh's build verdicts, no boot.
#
# Runs the real harness with stubs for sudo, cargo and go ahead of PATH, so
# nothing is loaded, booted or built: only the [1/3], [2/3] and [3/3] build
# verdicts are read, against what a stub cargo printed and returned. Every
# boot in the harness fails here, and that says nothing about boots; a real
# run of upc-regression.sh is still what proves those.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0

mkdir -p "${WORK}/bin"
printf '#!/bin/sh\necho "$*" >>"%s/sudo.log"\nexit 0\n' "${WORK}" >"${WORK}/bin/sudo"
printf '#!/bin/sh\nexit 1\n' >"${WORK}/bin/go"
cat >"${WORK}/bin/cargo" <<'STUB'
#!/bin/sh
case "$UPC_STUB_CARGO" in
cold-green)
	echo "   Compiling thiserror-impl v2.0.12"
	echo "   Compiling thiserror v2.0.12"
	echo "warning: unused variable: error_count"
	echo "    Finished release profile [optimized] target(s) in 41.20s"
	;;
warm-green) echo "    Finished release profile [optimized] target(s) in 0.05s" ;;
exit-only)
	echo "   Compiling monad-cpu-ebpf v0.1.0"
	exit 101
	;;
killed) exit 137 ;;
error-line)
	echo "error: linking with bpf-linker failed"
	exit 101
	;;
error-exit-zero) echo "error: could not compile monad-cpu-ebpf" ;;
*) exit 64 ;;
esac
STUB
chmod +x "${WORK}/bin/sudo" "${WORK}/bin/go" "${WORK}/bin/cargo"

upc_build_verdicts() { # $1 stub mode; prints the harness's build FAIL lines
	UPC_STUB_CARGO="$1" PATH="${WORK}/bin:${PATH}" DOOM_RUNNER=/nonexistent \
		timeout 120 bash "${REPO_ROOT}/scripts/upc-regression.sh" 2>&1 |
		sed 's/\x1b\[[0-9;]*m//g' | grep -E 'FAIL +(ascend build|non-ascend build|cartridge-ebpf build)' |
		sed -E 's/^ *FAIL +//' | sort | tr '\n' ',' | sed 's/,$//'
}

upc_build_case() { # $1 stub mode, $2 the FAIL lines it must give, $3 label
	local got
	got="$(upc_build_verdicts "$1")"
	if [ "${got}" = "$2" ]; then
		printf '  PASS  %s\n' "$3"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s\n        got:  [%s]\n        want: [%s]\n' "$3" "${got}" "$2"
		FAILED=$((FAILED + 1))
	fi
}

ALL_THREE="ascend build,cartridge-ebpf build,non-ascend build"

echo "--- a good build is not a failure ---"
upc_build_case cold-green "" "cold target: Compiling thiserror and a warning naming error"
upc_build_case warm-green "" "warm target"

echo "--- a bad build is a failure ---"
upc_build_case exit-only "${ALL_THREE}" "cargo exits 101 with no error line"
upc_build_case killed "${ALL_THREE}" "cargo dies with no output at all"
upc_build_case error-line "${ALL_THREE}" "an error line and exit 101"
upc_build_case error-exit-zero "${ALL_THREE}" "an error line and exit 0"

echo "--- nothing real ran ---"
if [ -s "${WORK}/sudo.log" ]; then
	echo "  PASS  every sudo call reached the stub"
	PASSED=$((PASSED + 1))
else
	echo "  FAIL  the stub saw no sudo call, so the harness did not run"
	FAILED=$((FAILED + 1))
fi

echo
echo "passed: ${PASSED}  failed: ${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
echo "RESULT: PASS"
