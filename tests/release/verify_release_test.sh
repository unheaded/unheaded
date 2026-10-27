#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Offline test for scripts/verify-release.sh: a fake release signed with a
# throwaway key (never committed, never the release identity) exercises the
# verification and the three ways it must fail. No network.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
COSIGN="${COSIGN:-$(command -v cosign || true)}"
if [[ -z "$COSIGN" ]]; then
	"$ROOT/scripts/ci-install-tools.sh" "$T/bin" cosign >/dev/null
	COSIGN="$T/bin/cosign"
fi
export COSIGN
fail=0
pass() { echo "[PASS] $*"; }
bad() { echo "[FAIL] $*"; fail=1; }

(cd "$T" && COSIGN_PASSWORD="" "$COSIGN" generate-key-pair >/dev/null 2>&1)
mkrel() { # dir -> a signed fake release in dir
	mkdir -p "$1"
	printf 'binary one\n' >"$1/wotan"
	printf 'deb two\n' >"$1/unheaded-wotan_0.2.0~alpha.deb"
	(cd "$1" && sha256sum wotan unheaded-wotan_0.2.0~alpha.deb >checksums.txt)
	COSIGN_PASSWORD="" "$COSIGN" sign-blob --yes --key "$T/cosign.key" --use-signing-config=false \
		--tlog-upload=false --bundle "$1/checksums.txt.sigstore.json" "$1/checksums.txt" >/dev/null 2>&1
}
run() { bash "$ROOT/scripts/verify-release.sh" --local "$1" --key "$T/cosign.pub" >/dev/null 2>&1; }

mkrel "$T/good"
run "$T/good" && pass "a signed release with matching files verifies" || bad "good release refused"

mkrel "$T/file"; printf 'swapped\n' >"$T/file/wotan"
run "$T/file" && bad "a swapped file passed" || pass "a swapped file is refused"

mkrel "$T/sums"; printf 'deadbeef  extra\n' >>"$T/sums/checksums.txt"
run "$T/sums" && bad "an edited checksums.txt passed" || pass "an edited checksums.txt is refused (signature)"

mkrel "$T/none"; rm "$T/none/wotan" "$T/none/unheaded-wotan_0.2.0~alpha.deb"
run "$T/none" && bad "a run that checked no file passed" || pass "no listed file present is refused"

mkrel "$T/nobundle"; rm "$T/nobundle/checksums.txt.sigstore.json"
run "$T/nobundle" && bad "a missing bundle passed" || pass "a missing bundle is refused"

bash "$ROOT/scripts/verify-release.sh" "not-a-tag" >/dev/null 2>&1 && bad "a malformed tag was accepted" || pass "a malformed tag is a usage error"

[[ "$fail" -eq 0 ]] && echo "verify_release_test: all passed" || { echo "verify_release_test: FAILED"; exit 1; }
