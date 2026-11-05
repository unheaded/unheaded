#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-install-trivy.sh — install-trivy.sh installs what the pin names, and
# nothing else. A stub curl serves a tarball built here, and a copy of the
# script reads a pin set to that tarball's sha256, so nothing is downloaded.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0

trivy_case_result() { # $1 label, $2 0 if it held
	if [ "$2" -eq 0 ]; then
		printf '  PASS  %s\n' "$1"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s\n' "$1"
		FAILED=$((FAILED + 1))
	fi
}

mkdir -p "${WORK}/tree/lib" "${WORK}/bin" "${WORK}/payload"
cp "${REPO_ROOT}/scripts/install-trivy.sh" "${WORK}/tree/"
printf '#!/bin/sh\necho stub trivy\n' >"${WORK}/payload/trivy"
tar -czf "${WORK}/good.tar.gz" -C "${WORK}/payload" trivy
GOOD_SHA="$(sha256sum "${WORK}/good.tar.gz" | cut -d' ' -f1)"
sed -E "s/^TRIVY_SHA256=\"[^\"]+\"/TRIVY_SHA256=\"${GOOD_SHA}\"/" \
	"${REPO_ROOT}/scripts/lib/tool-pins.sh" >"${WORK}/tree/lib/tool-pins.sh"
cat >"${WORK}/bin/curl" <<'STUB'
#!/bin/sh
# -fsSL --retry 3 -o FILE URL: serve $TRIVY_STUB_FILE, or fail with no file set.
[ -n "${TRIVY_STUB_FILE:-}" ] || exit 22
cp "$TRIVY_STUB_FILE" "$5"
STUB
chmod +x "${WORK}/bin/curl"

trivy_install() { # $1 file to serve, $2 directory
	TRIVY_STUB_FILE="$1" PATH="${WORK}/bin:${PATH}" sh "${WORK}/tree/install-trivy.sh" "$2" >/dev/null 2>&1
}

trivy_install "${WORK}/good.tar.gz" "${WORK}/ok" && [ -x "${WORK}/ok/trivy" ] && [ "$("${WORK}/ok/trivy")" = "stub trivy" ]
trivy_case_result "the pinned tarball installs an executable trivy" $?

printf '#!/bin/sh\necho swapped\n' >"${WORK}/payload/trivy"
tar -czf "${WORK}/swapped.tar.gz" -C "${WORK}/payload" trivy
! trivy_install "${WORK}/swapped.tar.gz" "${WORK}/bad" && [ ! -e "${WORK}/bad/trivy" ]
trivy_case_result "a swapped tarball fails and installs nothing" $?

! trivy_install "" "${WORK}/none" && [ ! -e "${WORK}/none/trivy" ]
trivy_case_result "a failed download fails and installs nothing" $?

grep -qE '^TRIVY_SHA256="[0-9a-f]{64}"' "${REPO_ROOT}/scripts/lib/tool-pins.sh"
trivy_case_result "the real pin is a sha256" $?

echo
echo "passed: ${PASSED}  failed: ${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
echo "RESULT: PASS"
