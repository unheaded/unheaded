#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Sign one attestation (ADR-097, compliance/attestations/README.md).
# Copies docs/compliance/drafts/attestations/<name>.yaml to
# compliance/attestations/<name>.yaml with attested_at = today (UTC) and
# expires_at = today + the control's freshness_days, then makes a signed
# commit holding that one file. Only the accountable person signs: the
# script refuses any other git identity and any unsigned commit.
#
# Usage: sign-attestation.sh [--dry-run] <name>
#   --dry-run  print the file that would be committed; no identity check,
#              no write, no commit.
set -euo pipefail

usage() { echo "usage: sign-attestation.sh [--dry-run] <name>" >&2; exit 2; }

DRY=0
if [[ "${1:-}" == "--dry-run" ]]; then DRY=1; shift; fi
[[ $# -eq 1 ]] || usage
NAME="$1"
[[ "$NAME" =~ ^[a-z0-9][a-z0-9-]{0,63}$ ]] || { echo "sign-attestation: bad name: $NAME" >&2; exit 2; }

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DRAFT="docs/compliance/drafts/attestations/${NAME}.yaml"
TARGET="compliance/attestations/${NAME}.yaml"
CATALOG="compliance/catalog/controls.yaml"
SIGNER_EMAIL="stevie@bellis.tech"
SIGNER_NAME="Stevie Bellis"
cd "$REPO_ROOT"

[[ -f "$DRAFT" ]] || { echo "sign-attestation: no draft $DRAFT" >&2; exit 1; }

# The catalog must name this file as evidence; its window sets the expiry.
DAYS="$(awk -v ref="$TARGET" '
  /^  - id: /            { days = "" }
  /^    freshness_days:/ { days = $2 }
  index($0, "ref: " ref "}") && days != "" { print days; exit }
' "$CATALOG")"
[[ "$DAYS" =~ ^[0-9]+$ && "$DAYS" -gt 0 ]] || {
  echo "sign-attestation: no control in $CATALOG has evidence ref $TARGET" >&2; exit 1; }

TODAY="$(date -u +%F)"
EXPIRES="$(date -u -d "$TODAY + $DAYS days" +%F)"

# Comments out; the two placeholders filled. Exactly one of each remains.
BODY="$(grep -v '^#' "$DRAFT" \
  | sed -e "s/^attested_at: SET-ON-SIGNING\$/attested_at: ${TODAY}/" \
        -e "s/^expires_at: SET-ON-SIGNING\$/expires_at: ${EXPIRES}/")"
for key in statement attested_by attested_at expires_at; do
  [[ "$(grep -c "^${key}:" <<<"$BODY")" -eq 1 ]] || {
    echo "sign-attestation: $DRAFT must have exactly one top-level '${key}:'" >&2; exit 1; }
done
if grep -q 'SET-ON-SIGNING' <<<"$BODY"; then
  echo "sign-attestation: placeholder left in $DRAFT" >&2; exit 1
fi
if [[ "$(grep -E '^[a-z_]+:' <<<"$BODY" | grep -cvE '^(statement|attested_by|attested_at|expires_at):')" -ne 0 ]]; then
  echo "sign-attestation: $DRAFT has a field the collector rejects" >&2; exit 1
fi
grep -qx "attested_by: ${SIGNER_NAME}" <<<"$BODY" || {
  echo "sign-attestation: attested_by must be ${SIGNER_NAME}" >&2; exit 1; }

if [[ $DRY -eq 1 ]]; then
  echo "# would write ${TARGET} and commit it alone, signed:"
  printf '%s\n' "$BODY"
  exit 0
fi

EMAIL="$(git config user.email || true)"
[[ "$EMAIL" == "$SIGNER_EMAIL" ]] || {
  echo "sign-attestation: git user.email is '${EMAIL}', not ${SIGNER_EMAIL}: only the accountable person signs" >&2; exit 1; }
if ! git commit-tree -S -m sign-probe "$(git rev-parse 'HEAD^{tree}')" >/dev/null 2>&1; then
  echo "sign-attestation: signing failed (gpg key not cached?); nothing written" >&2; exit 1
fi

printf '%s\n' "$BODY" > "$TARGET"
git add -- "$TARGET"
git commit -S -m "attest: ${NAME} reviewed ${TODAY}" -- "$TARGET"
SIG="$(git log -1 --format=%G? -- "$TARGET")"
echo "${TARGET}: commit $(git rev-parse --short HEAD), signature ${SIG} (want G)"
[[ "$SIG" == "G" ]]
