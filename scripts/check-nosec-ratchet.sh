#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-nosec-ratchet.sh: gosec suppressions in the Go source may only shrink.
#
# check-gosec-ratchet.sh guards the rule exclusion list and the form of each
# annotation. Neither limits how many there are, so a red finding can always be
# annotated away, one site at a time. This holds the number.
#
# Counts #nosec and //gosec:disable (gosec 2.29 honours both) in tracked .go
# files, per file, against docs/security/nosec-baseline.txt. Per file because a
# total can be traded: one removed here pays for one added there. A count
# inside a comment about the tag is counted too: this errs towards failing.
#
#   a file above its baseline, or new to it  -> FAIL (fix the finding)
#   a file below its baseline, or gone       -> FAIL (run --update)
#
# A new annotation that is right anyway goes in the baseline by hand, in the same
# commit, where review can see it. --update only lowers; it refuses additions.
# Some gosec options take findings away without touching this count: an
# alternative tag (-nosec-tag, or NoSecAlternative in a -conf file), -conf
# itself, -exclude-dir and -exclude-generated. A workflow that passes one to
# gosec fails, and so does NoSecAlternative in any tracked file.
#
# Usage: scripts/check-nosec-ratchet.sh [--update]
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${NOSEC_BASELINE:-${REPO_ROOT}/docs/security/nosec-baseline.txt}"
cd "$REPO_ROOT" || exit 2
[[ -f "$BASELINE" ]] || { echo "check-nosec-ratchet: missing $BASELINE; refusing to re-baseline silently" >&2; exit 2; }

now="$(mktemp)" || exit 2
trap 'rm -f "$now"' EXIT
git ls-files -z '*.go' | xargs -0 grep -oE '#nosec|//gosec:disable' 2>/dev/null |
	sed -E 's/:(#nosec|\/\/gosec:disable)$//' | LC_ALL=C sort | uniq -c | awk '{print $2, $1}' >"$now"
[[ -s "$now" ]] || { echo "check-nosec-ratchet: counted nothing; refusing to pass vacuously" >&2; exit 2; }

# One logical line per shell command, so a flag on a continuation line counts.
logical_lines() {
	sed -e ':a' -e '/\\$/N; s/\\\n/ /; ta' "$@"
}
hidden="$(for wf in .github/workflows/*; do
	logical_lines "$wf" | grep -E 'gosec' |
		grep -E -e '(^|[[:space:]])-{1,2}(nosec-tag|conf|exclude-dir|exclude-generated)([=[:space:]]|$)' |
		sed "s|^|$wf: |"
done)"
hidden+="$(git grep -n -e 'NoSecAlternative' -- ':!scripts/check-nosec-ratchet.sh' ':!scripts/test-check-nosec-ratchet.sh')"
if [[ -n "$hidden" ]]; then
	echo "[FAIL] these gosec options would drop findings without touching this ratchet:"
	echo "$hidden"
	exit 1
fi

diffs="$(awk '
	FILENAME == ARGV[1] { if ($0 !~ /^#/ && NF == 2) base[$1] = $2; next }
	{ seen[$1] = 1
	  if (!($1 in base)) print "new", $1, 0, $2
	  else if ($2 > base[$1]) print "up", $1, base[$1], $2
	  else if ($2 < base[$1]) print "down", $1, base[$1], $2 }
	END { for (f in base) if (!(f in seen)) print "down", f, base[f], 0 }
' "$BASELINE" "$now" | LC_ALL=C sort)"
total="$(awk '{s += $2} END {print s}' "$now")"

if [[ "${1:-}" == --update ]]; then
	if grep -qE '^(new|up) ' <<<"$diffs"; then
		echo "[FAIL] --update only lowers the baseline; these went up:"
		grep -E '^(new|up) ' <<<"$diffs"
		exit 1
	fi
	{ grep '^#' "$BASELINE"; cat "$now"; } >"$BASELINE.new" && mv "$BASELINE.new" "$BASELINE"
	echo "[OK] baseline now $total annotations"
	exit 0
fi

fail=0
while read -r kind file was is; do
	case "$kind" in
	new | up)
		echo "[FAIL] $file: $is gosec suppressions, baseline $was. Fix the finding."
		fail=1
		;;
	down)
		echo "[FAIL] $file: $is gosec suppressions, baseline $was. Lock it in: $0 --update"
		fail=1
		;;
	esac
done <<<"$diffs"
((fail == 0)) || exit 1
echo "[PASS] $total gosec suppressions across $(wc -l <"$now") files, none above baseline"
