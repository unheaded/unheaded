#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-ebpf-insns.sh — the verifier-instruction ratchet ADR-012 promised.
#
# Why this exists. ADR-012 set a 900,000-instruction budget (90 % of the
# kernel's 1M complexity ceiling) and said a gate would hold it. The gate it
# got, bpf-verifier-check.sh, divided the object's byte size by eight: a guess
# that cannot see what the verifier actually walks. The kernel reports the
# real number when a program loads (prog info verified_insns), and
# scripts/check-ebpf-loads.sh now writes it out. This compares those numbers
# with docs/policy/ebpf-insns-baseline.tsv:
#
#   - a program that grew more than 10 % over its baseline row   -> FAIL
#   - a program above 900,000 not in ebpf-insns-over-budget.txt   -> FAIL
#   - an over-budget entry that is now at or under 900,000        -> FAIL (take it off)
#   - a program with no baseline row, or a row with no program    -> FAIL (run --update)
#
# Shrinking is always fine; --update records it (and any reviewed growth) by
# rewriting the baseline and printing the diff for the commit. Membership is
# compared, not counts (the lesson in check-secrets-baseline.sh).
#
#   check-ebpf-insns.sh                   load everything (needs root), compare
#   check-ebpf-insns.sh --from FILE       compare FILE (check-ebpf-loads.sh's EBPF_INSNS_OUT)
#   check-ebpf-insns.sh --update [--from FILE]   rewrite the baseline from the current numbers
#
# The baseline records the kernel it was measured on; on any other kernel the
# gate exits 2 (EBPF_INSNS_ANY_KERNEL=1 compares anyway, for an experiment).
#
# Exit: 0 pass, 1 fail, 2 usage or cannot measure (it never passes by skipping).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${EBPF_INSNS_BASELINE:-$REPO_ROOT/docs/policy/ebpf-insns-baseline.tsv}"
OVER="${EBPF_INSNS_OVER_BUDGET:-$REPO_ROOT/docs/policy/ebpf-insns-over-budget.txt}"
BUDGET=900000
GROWTH_PCT=10

FROM="" UPDATE=0
while (($#)); do
	case "$1" in
	--from) FROM="${2:?--from needs a file}"; shift 2 ;;
	--update) UPDATE=1; shift ;;
	*) echo "usage: $0 [--from FILE] [--update]" >&2; exit 2 ;;
	esac
done
[[ -f "$OVER" ]] || { echo "[FAIL] $OVER is missing" >&2; exit 2; }

TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT
if [[ -z "$FROM" ]]; then
	FROM="$TMP/current.tsv"
	if ! EBPF_INSNS_OUT="$FROM" "$REPO_ROOT/scripts/check-ebpf-loads.sh" >"$TMP/loads.log" 2>&1; then
		cat "$TMP/loads.log" >&2
		echo "[FAIL] check-ebpf-loads.sh failed; no numbers to compare" >&2
		exit 2
	fi
fi
[[ -s "$FROM" ]] || { echo "[FAIL] no verified-instruction counts in $FROM; refusing to pass an empty set" >&2; exit 2; }
sort -o "$TMP/cur" "$FROM"

if ((UPDATE)); then
	kernel="$(uname -r)" today="$(date -u +%F)"
	{
		printf 'program\tvariant\tinsns\tkernel\tdate\n'
		awk -F'\t' -v k="$kernel" -v d="$today" 'NF>=3 {printf "%s\t%s\t%s\t%s\t%s\n", $1, $2, $3, k, d}' "$TMP/cur"
	} >"$TMP/new"
	if [[ -f "$BASELINE" ]]; then
		diff <(cut -f1-3 "$BASELINE") <(cut -f1-3 "$TMP/new") || true
	fi
	cp "$TMP/new" "$BASELINE"
	echo "[UPDATED] $(basename "$BASELINE") ($(($(wc -l <"$BASELINE") - 1)) programs)"
	exit 0
fi
[[ -f "$BASELINE" ]] || { echo "[FAIL] $BASELINE is missing (run --update to seed it)" >&2; exit 2; }

# Counts are kernel- and toolchain-specific (the verifier changes between
# kernels; bpf-linker versions emit different code). A baseline from another
# kernel is not comparable: refuse (exit 2) rather than pass or fail on noise.
base_kernel="$(awk -F'\t' 'NR==2 {print $4}' "$BASELINE")"
if [[ "${EBPF_INSNS_ANY_KERNEL:-0}" != 1 && -n "$base_kernel" && "$base_kernel" != "$(uname -r)" ]]; then
	echo "[FAIL] baseline measured on kernel ${base_kernel}; this is $(uname -r): counts are not comparable." >&2
	echo "       Run on the baseline's host (west), or re-seed with --update on purpose." >&2
	exit 2
fi
awk -F'\t' 'NR>1 && NF>=3 {print $1 "\t" $2 "\t" $3}' "$BASELINE" | sort >"$TMP/base"
sed 's/#.*//' "$OVER" | awk 'NF {print $1}' | sort -u >"$TMP/over"

rc=0
report() { echo "$1" >&2; sed 's/^/         /' "$2" >&2; rc=1; }

# Growth past the ratchet.
join -t $'\t' <(awk -F'\t' '{print $1 "|" $2 "\t" $3}' "$TMP/base" | sort) \
	<(awk -F'\t' '{print $1 "|" $2 "\t" $3}' "$TMP/cur" | sort) |
	awk -F'\t' -v p="$GROWTH_PCT" '$3 > $2 * (1 + p / 100) {printf "%s: %d -> %d (+%.1f %%)\n", $1, $2, $3, ($3 - $2) * 100 / $2}' >"$TMP/grew"
[[ -s "$TMP/grew" ]] && report "[FAIL] verified instructions grew more than ${GROWTH_PCT} % over the baseline (fix it, or review and --update):" "$TMP/grew"

# The 900K budget, with the shrink-only allowlist.
awk -F'\t' -v b="$BUDGET" '$3 > b {print $1 "|" $2}' "$TMP/cur" | sort -u >"$TMP/overnow"
comm -23 "$TMP/overnow" "$TMP/over" >"$TMP/newover"
[[ -s "$TMP/newover" ]] && report "[FAIL] over ADR-012's ${BUDGET}-instruction budget and not allowlisted:" "$TMP/newover"
comm -13 "$TMP/overnow" "$TMP/over" >"$TMP/staleover"
[[ -s "$TMP/staleover" ]] && report "[FAIL] allowlisted as over budget but now at or under ${BUDGET}; remove from $(basename "$OVER"):" "$TMP/staleover"

# Membership.
comm -23 <(cut -f1,2 "$TMP/cur" | sort -u) <(cut -f1,2 "$TMP/base" | sort -u) >"$TMP/new"
[[ -s "$TMP/new" ]] && report "[FAIL] program(s) with no baseline row (review and --update):" "$TMP/new"
comm -13 <(cut -f1,2 "$TMP/cur" | sort -u) <(cut -f1,2 "$TMP/base" | sort -u) >"$TMP/gone"
[[ -s "$TMP/gone" ]] && report "[FAIL] baseline rows for programs that no longer load (--update):" "$TMP/gone"

((rc == 0)) || exit 1
echo "[PASS] $(wc -l <"$TMP/cur") programs within ${GROWTH_PCT} % of the baseline; $(wc -l <"$TMP/over") allowlisted over ${BUDGET}"
