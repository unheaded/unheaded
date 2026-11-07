#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# A version bump fails until sbom/ is regenerated (Stevie, 2026-11-06). The
# version is the top of debian/changelog; scripts/build-all-debs.sh's default
# and a release tag must say the same. scripts/generate-sbom.sh writes
# sbom/VERSION on a full run, so a bump with no run fails here. The file is a
# stamp: this gate cannot tell a full run from a hand edit, and the first stamp
# (0.2.0~alpha) was seeded by hand for the inventory then committed.
#
#   scripts/check-sbom-version.sh            changelog, debs and sbom/ agree
#   scripts/check-sbom-version.sh v1.2.0     and so does this tag (Debian form:
#                                            v1.2.0-alpha is 1.2.0~alpha)
set -euo pipefail
cd "$(dirname "$0")/.."

fail() {
	echo "[FAIL] check-sbom-version: $1" >&2
	exit 1
}

version=$(sed -nE '1s/^unheaded \(([^)]+)\) .*/\1/p' debian/changelog)
[[ -n $version ]] || fail "no version on the first line of debian/changelog"

[[ -f sbom/VERSION ]] || fail "no sbom/VERSION: run scripts/generate-sbom.sh"
sbom=$(<sbom/VERSION)
[[ $sbom == "$version" ]] ||
	fail "debian/changelog says ${version}, sbom/ was made for ${sbom}: run scripts/generate-sbom.sh"

debs=$(sed -nE 's/^[[:space:]]*VERSION="([^+"$]+)\+.*/\1/p' scripts/build-all-debs.sh | head -n 1)
[[ $debs == "$version" ]] ||
	fail "scripts/build-all-debs.sh defaults to '${debs}', debian/changelog says ${version}"

if (($# > 0)); then
	[[ $1 == v* ]] || fail "'$1' is not a version tag"
	tag=${1#v}
	tag=${tag/-/\~}
	[[ $tag == "$version" ]] || fail "tag $1 is ${tag}, sbom/ was made for ${version}"
fi
echo "[PASS] sbom/ was made for ${version}"
