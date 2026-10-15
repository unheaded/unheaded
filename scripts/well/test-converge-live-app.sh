#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Proves db/converge/2026-10-15-live-unheaded-app.sql on a copy of the live
# Well before it runs there: restore a backup set's globals and
# unheaded_app dump into a throwaway server, run the converge script, and
# compare its schema (privileges included, owners and column order
# ignored, ledger objects excluded) with unheaded_app from a fresh
# bootstrap of db/. They must be identical, and every table must keep its
# row count. Never touches the real Well.
#
#   scripts/well/test-converge-live-app.sh SET_DIR [image]
#   scripts/well/test-converge-live-app.sh --live [image]   # after the converge ran on
#       the live Well: compare unheaded-postgres itself (read-only), no restore
set -euo pipefail

LIVE=""
if [[ "${1:-}" == --live ]]; then LIVE="${WELL_CONTAINER:-unheaded-postgres}"; shift; set -- "--" "$@"; fi
SET="${1:?usage: $0 SET_DIR|--live [image]}"
IMAGE="${2:-postgres:16-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
CONVERGE="$R/db/converge/2026-10-15-live-unheaded-app.sql"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
A="converge-live-$$" F="converge-fresh-$$"
TMP="$(mktemp -d)"
cleanup() { "${DOCKER[@]}" stop "converge-live-$$" "$F" >/dev/null 2>&1 || true; rm -rf -- "$TMP"; }
trap cleanup EXIT

if [[ -n "$LIVE" ]]; then
	SU="$("${DOCKER[@]}" exec "$LIVE" printenv POSTGRES_USER)"
else
	SU="$(sed -n 's/^superuser=//p' "$SET/cluster.env")"
	(cd "$SET" && sha256sum --quiet --strict -c SHA256SUMS)
fi
wait_ready() {
	for _ in $(seq 1 90); do
		"${DOCKER[@]}" exec "$1" pg_isready -h 127.0.0.1 -U "$SU" -q 2>/dev/null && return 0
		sleep 1
	done
	echo "$1 not ready"; exit 1
}
[[ -n "$LIVE" ]] || "${DOCKER[@]}" run -d --rm --name "$A" --network none \
	-e POSTGRES_USER="$SU" -e POSTGRES_PASSWORD="t-$RANDOM" -e POSTGRES_DB=postgres "$IMAGE" >/dev/null
"${DOCKER[@]}" run -d --rm --name "$F" --network none \
	-e POSTGRES_USER="$SU" -e POSTGRES_PASSWORD="t-$RANDOM" -e POSTGRES_DB=unheaded \
	-v "$R/db/migrations:/migrations:ro" -v "$R/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" "$IMAGE" >/dev/null
wait_ready "$F"
if [[ -n "$LIVE" ]]; then
	A="$LIVE"
else
	wait_ready "$A"
	grep -vxF -e "CREATE ROLE $SU;" "$SET/globals.sql" | "${DOCKER[@]}" exec -i "$A" psql -X -q -v ON_ERROR_STOP=1 -U "$SU" -d postgres >/dev/null
	"${DOCKER[@]}" exec -i "$A" pg_restore -U "$SU" --exit-on-error --create -d postgres <"$SET/unheaded_app.dump"
	"${DOCKER[@]}" exec -i "$A" psql -X -q -v ON_ERROR_STOP=1 -U "$SU" -d unheaded_app <"$CONVERGE"
fi

# Schema text, normalised: no comments/SET lines, no owners, ledger objects
# dropped, and each CREATE TABLE body sorted (ADD COLUMN appends, so live
# column order differs from 003's and cannot be changed without a rewrite).
norm() {
	"${DOCKER[@]}" exec "$1" pg_dump -U "$SU" -d unheaded_app --schema-only --no-owner |
		grep -vE '^--|^SET |^SELECT pg_catalog|^\\(un)?restrict|^$' |
		python3 -c '
import re, sys
s = sys.stdin.read()
blocks = re.split(r"\n(?=CREATE |ALTER |GRANT |REVOKE |COMMENT )", s)
out = []
for b in blocks:
    if "well_schema_migrations" in b or "well_append_only" in b:
        continue
    if b.startswith("CREATE TABLE"):
        lines = b.split("\n")
        body = sorted(l.strip().rstrip(",") for l in lines[1:-1])
        b = "\n".join([lines[0]] + body + [lines[-1]])
    out.append(b.strip())
print("\n".join(sorted(out)))
'
}
norm "$A" >"$TMP/live"
norm "$F" >"$TMP/fresh"
if diff -u "$TMP/fresh" "$TMP/live"; then
	echo "[PASS] converged unheaded_app schema == fresh bootstrap (privileges included)"
else
	echo "[FAIL] schemas differ (- fresh, + converged live)"; exit 1
fi

counts() {
	for t in $(awk -F'\t' '$1=="unheaded_app"{print $2}' "$SET/rowcounts.tsv"); do
		printf '%s\t%s\n' "$t" "$("${DOCKER[@]}" exec "$A" psql -X -At -U "$SU" -d unheaded_app -c "SELECT count(*) FROM $t")"
	done
}
if [[ -n "$LIVE" ]]; then exit 0; fi
if diff <(awk -F'\t' '$1=="unheaded_app"{print $2"\t"$3}' "$SET/rowcounts.tsv") <(counts); then
	echo "[PASS] every unheaded_app table kept its rows"
else
	echo "[FAIL] row counts changed"; exit 1
fi
