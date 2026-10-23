#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks scripts/well/offsite-check.sh (east's verify-and-prune of the
# offsite copies) on sets made in a temporary directory; no database, no
# east. A copy cut short (no SHA256SUMS yet, or a dump missing) must fail
# the check, and pruning keeps the newest WELL_OFFSITE_KEEP sets that
# verify, so a run of broken copies cannot push the good ones out.
#
#   scripts/well/test-offsite-check.sh
set -euo pipefail

R="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT
fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }

mkset() { # root day time [partial]: a set as offsite-copy.sh leaves it
	local d="$1/sets/$2/well-$3Z"
	mkdir -p "$d"
	printf 'superuser=unheaded\n' >"$d/cluster.env"
	printf 'dump %s %s\n' "$2" "$3" >"$d/unheaded_app.dump"
	(cd "$d" && sha256sum cluster.env unheaded_app.dump >SHA256SUMS)
	case "${4:-}" in
	nosums) rm "$d/SHA256SUMS"; return ;;      # LATEST follows only a whole copy
	nodump) rm "$d/unheaded_app.dump"; return ;;
	esac
	printf '%s/well-%sZ\n' "$2" "$3" >"$1/sets/LATEST"
}
check() { WELL_OFFSITE_DIR="$1" WELL_OFFSITE_KEEP="${2:-14}" "$R/scripts/well/offsite-check.sh"; }

# Every set verifies.
A="$TMP/a"
mkset "$A" 2026-10-20 033000
mkset "$A" 2026-10-21 033000
if check "$A" >/dev/null 2>&1; then pass "good sets verify"; else fail "good sets fail the check"; fi

# A copy cut short before SHA256SUMS arrived is not a good set.
B="$TMP/b"
mkset "$B" 2026-10-20 033000
mkset "$B" 2026-10-21 033000 nosums
if out="$(check "$B" 2>&1)"; then fail "a set without SHA256SUMS passes"
elif [[ "$out" == *"BAD 2026-10-21/well-033000Z"* ]]; then pass "a set without SHA256SUMS fails the check"
else fail "set without SHA256SUMS: $(tail -1 <<<"$out")"; fi

# A copy missing a dump SHA256SUMS lists.
C="$TMP/c"
mkset "$C" 2026-10-20 033000
mkset "$C" 2026-10-21 033000 nodump
if out="$(check "$C" 2>&1)"; then fail "a set missing a dump passes"
elif [[ "$out" == *"BAD 2026-10-21/well-033000Z"* ]]; then pass "a set missing a dump fails the check"
else fail "set missing a dump: $(tail -1 <<<"$out")"; fi

# Pruning counts sets that verify: with keep 2, two good old sets and two
# newer broken copies, both good sets stay.
D="$TMP/d"
mkset "$D" 2026-10-17 033000
mkset "$D" 2026-10-18 033000
mkset "$D" 2026-10-19 033000
mkset "$D" 2026-10-20 033000 nosums
mkset "$D" 2026-10-21 033000 nosums
check "$D" 2 >/dev/null 2>&1 || true
if [[ -f "$D/sets/2026-10-19/well-033000Z/SHA256SUMS" && -f "$D/sets/2026-10-18/well-033000Z/SHA256SUMS" ]]; then
	pass "pruning keeps the newest 2 good sets"
else fail "pruning removed good sets: $(cd "$D/sets" && find . -name 'well-*' | sort | paste -sd' ')"; fi
[[ ! -d "$D/sets/2026-10-17/well-033000Z" ]] && pass "the oldest good set is pruned" || fail "the oldest good set was kept"

# Retention on good sets only.
E="$TMP/e"
for day in 17 18 19 20; do mkset "$E" "2026-10-$day" 033000; done
if check "$E" 2 >/dev/null 2>&1; then pass "keep 2 over 4 good sets passes"; else fail "keep 2 over 4 good sets fails"; fi
n=$(find "$E/sets" -mindepth 2 -maxdepth 2 -type d -name 'well-*Z' | wc -l)
((n == 2)) && pass "retention keeps 2" || fail "retention left $n sets"

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
