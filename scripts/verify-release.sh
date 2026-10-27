#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# verify-release.sh: check that a GitHub release of Unheaded was built and
# signed by this repository's release workflow, and that the files you have
# are the files it signed (S-IRON-GATE Phase 12, wiki/Releases.md).
#
#   scripts/verify-release.sh v0.2.0-alpha             # download checksums + bundle, verify; then
#                                                     # every listed file present in --dir
#   scripts/verify-release.sh v0.2.0-alpha --dir DIR   # verify files already downloaded into DIR
#   scripts/verify-release.sh v0.2.0-alpha --download  # also download every listed file into DIR
#   scripts/verify-release.sh --local DIR --key PUB    # offline: a bundle signed with a key (tests)
#
# What is checked, in order:
#   1. checksums.txt carries a cosign bundle whose certificate names exactly
#      https://github.com/unheaded/unheaded/.github/workflows/release.yml@refs/tags/<tag>
#      issued by GitHub Actions' OIDC (keyless), and whose Rekor entry checks out;
#   2. every file named in checksums.txt that is present matches its sha256;
#   3. at least one listed file was present (a run that checked nothing fails).
# Exit: 0 verified, 1 a check failed, 2 usage or a missing tool.
set -euo pipefail

REPO_SLUG="unheaded/unheaded"
WORKFLOW=".github/workflows/release.yml"
ISSUER="https://token.actions.githubusercontent.com"

TAG="" DIR="" KEY="" LOCAL=0 DOWNLOAD=0
while [[ $# -gt 0 ]]; do
	case "$1" in
	--dir) DIR="${2:?--dir needs a directory}"; shift ;;
	--download) DOWNLOAD=1 ;;
	--local) LOCAL=1; DIR="${2:?--local needs a directory}"; shift ;;
	--key) KEY="${2:?--key needs a public key file}"; shift ;;
	-h | --help) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
	-*) echo "verify-release: unknown option $1" >&2; exit 2 ;;
	*) [[ -z "$TAG" ]] || { echo "verify-release: one tag only" >&2; exit 2; }; TAG="$1" ;;
	esac
	shift
done

if [[ "$LOCAL" -eq 1 ]]; then
	[[ -n "$KEY" && -f "$KEY" ]] || { echo "verify-release: --local needs --key PUB" >&2; exit 2; }
else
	[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$ ]] || { echo "verify-release: want a tag like v0.2.0-alpha" >&2; exit 2; }
	[[ -z "$KEY" ]] || { echo "verify-release: --key is for --local only; releases are keyless" >&2; exit 2; }
fi

# cosign: $COSIGN, PATH, or the pinned one (sha256-checked by fetch-tool.sh).
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COSIGN="${COSIGN:-$(command -v cosign || true)}"
if [[ -z "$COSIGN" ]]; then
	tooldir="${XDG_CACHE_HOME:-$HOME/.cache}/unheaded-tools/bin"
	"$ROOT/scripts/ci-install-tools.sh" "$tooldir" cosign >/dev/null || { echo "verify-release: cannot install cosign" >&2; exit 2; }
	COSIGN="$tooldir/cosign"
fi

if [[ -z "$DIR" ]]; then
	DIR="$(mktemp -d)"
	trap 'rm -rf "$DIR"' EXIT
fi
mkdir -p "$DIR"

if [[ "$LOCAL" -eq 0 ]]; then
	base="https://github.com/${REPO_SLUG}/releases/download/${TAG}"
	for f in checksums.txt checksums.txt.sigstore.json; do
		curl -fsSL --retry 3 -o "$DIR/$f" "$base/$f" || { echo "verify-release: cannot download $f for $TAG" >&2; exit 1; }
	done
fi
for f in checksums.txt checksums.txt.sigstore.json; do
	[[ -s "$DIR/$f" ]] || { echo "[FAIL] $f missing in $DIR" >&2; exit 1; }
done

# 1. The signature on checksums.txt.
if [[ "$LOCAL" -eq 1 ]]; then
	"$COSIGN" verify-blob --key "$KEY" --insecure-ignore-tlog=true \
		--bundle "$DIR/checksums.txt.sigstore.json" "$DIR/checksums.txt" >/dev/null 2>&1 ||
		{ echo "[FAIL] checksums.txt: signature does not verify against $KEY" >&2; exit 1; }
	echo "[ok] checksums.txt signed by $KEY"
else
	identity="https://github.com/${REPO_SLUG}/${WORKFLOW}@refs/tags/${TAG}"
	"$COSIGN" verify-blob --certificate-identity "$identity" --certificate-oidc-issuer "$ISSUER" \
		--bundle "$DIR/checksums.txt.sigstore.json" "$DIR/checksums.txt" >/dev/null 2>&1 ||
		{ echo "[FAIL] checksums.txt: not signed by $identity" >&2; exit 1; }
	echo "[ok] checksums.txt signed by $identity"
fi

# 2. Optionally fetch every listed file; then check whatever is present.
mapfile -t names < <(awk '{print $2}' "$DIR/checksums.txt" | sed 's/^\*//')
if [[ "$DOWNLOAD" -eq 1 && "$LOCAL" -eq 0 ]]; then
	for n in "${names[@]}"; do
		[[ "$n" =~ ^[A-Za-z0-9._+~-]+$ ]] || { echo "[FAIL] refusing odd file name in checksums.txt: $n" >&2; exit 1; }
		curl -fsSL --retry 3 -o "$DIR/$n" "$base/$n" || { echo "verify-release: cannot download $n" >&2; exit 1; }
	done
fi
present=0
for n in "${names[@]}"; do [[ -f "$DIR/$n" ]] && present=$((present + 1)); done
[[ "$present" -gt 0 ]] || { echo "[FAIL] none of the ${#names[@]} files in checksums.txt is in $DIR (use --download or --dir)" >&2; exit 1; }
(cd "$DIR" && sha256sum --quiet -c --ignore-missing checksums.txt) || { echo "[FAIL] a file does not match checksums.txt" >&2; exit 1; }
echo "[ok] $present of ${#names[@]} listed file(s) present and matching"
