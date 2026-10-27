#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# relicense-mit-to-gpl.sh: rewrite the leftover MIT SPDX headers of first-party
# files to GPL-3.0-or-later, the project's licence (decision 12, 2026-10-27).
#
#   scripts/relicense-mit-to-gpl.sh            # dry run: list what would change, change nothing
#   scripts/relicense-mit-to-gpl.sh --apply    # rewrite (refuses with tracked changes pending)
#
# Only a licence declaration is touched: an `SPDX-License-Identifier: MIT` line
# that is a comment (#, //, /*, *, <!--, --, ;, ! for FRR) or a bold Markdown
# line within a file's first ten lines (a header), or a line holding nothing but
# the identifier within its last ten lines (a footer). The same text anywhere
# else (a grep in a battle plan, a config example in a heredoc, a sentence about
# other files) is content: it is left alone and listed for a person to
# review. Files that are MIT on purpose are listed, one path or directory
# prefix per line, in docs/legal/MIT-KEEP.txt; they are never touched. Tracked files only; llama.cpp/ and doom/ (vendored) are skipped.
#
# Exit: 0 done (or dry run), 1 refused or failed, 2 usage.
set -euo pipefail

APPLY=0
case "${1:-}" in
"") ;;
--apply) APPLY=1 ;;
-h | --help) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
*) echo "relicense: unknown argument: $1" >&2; exit 2 ;;
esac

cd "$(dirname "$0")/.."
KEEP=docs/legal/MIT-KEEP.txt
[[ -f "$KEEP" ]] || { echo "relicense: $KEEP missing (it may list nothing, but it must exist)" >&2; exit 1; }

# A header SPDX line: optional comment opener, the identifier, optional closer.
HEADER_RE='^[[:space:]]*(#|//|/\*|\*|<!--|--|;|!)?[[:space:]]*(\*\*)?SPDX-License-Identifier:[[:space:]]*MIT(\*\*)?[[:space:]]*(\*/|-->)?[[:space:]]*$'
# A footer: the identifier alone on its line (Markdown/plain-text manifests).
FOOTER_RE='^[[:space:]]*SPDX-License-Identifier:[[:space:]]*MIT[[:space:]]*$'

keep() { # path -> 0 if MIT-KEEP lists it (exact path or directory prefix)
	local p="$1" k
	while IFS= read -r k; do
		k="${k%%#*}"
		k="${k//[[:space:]]/}"
		[[ -n "$k" ]] || continue
		[[ "$p" == "$k" || "$p" == "${k%/}/"* ]] && return 0
	done <"$KEEP"
	return 1
}

mapfile -t candidates < <(git grep -l 'SPDX-License-Identifier: MIT' -- . ':!llama.cpp' ':!doom' || true)
targets=()
kept=0
content=()
for f in "${candidates[@]}"; do
	if ! head -n 10 -- "$f" | grep -qE "$HEADER_RE" && ! tail -n 10 -- "$f" | grep -qE "$FOOTER_RE"; then
		content+=("$f")
		continue
	fi
	if keep "$f"; then kept=$((kept + 1)); continue; fi
	targets+=("$f")
done

echo "relicense: ${#targets[@]} file(s) with an MIT header; $kept kept by $KEEP"
if [[ ${#targets[@]} -gt 0 ]]; then
	printf '%s\n' "${targets[@]}" | cut -d/ -f1 | sort | uniq -c | sort -rn | sed 's/^/  /'
fi

if [[ ${#content[@]} -gt 0 ]]; then
	echo "relicense: ${#content[@]} file(s) mention the MIT identifier only in their text (not changed; review by hand):"
	for f in "${content[@]}"; do printf '    %s:%s\n' "$f" "$(grep -n 'SPDX-License-Identifier: MIT' -- "$f" | head -1 | cut -d: -f1)"; done
fi

if [[ "$APPLY" -ne 1 ]]; then
	echo "relicense: DRY RUN (--apply rewrites)"
	printf '%s\n' "${targets[@]}" | sed 's/^/    /'
	exit 0
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
	echo "relicense: refusing: tracked changes are pending (commit or stash them first)" >&2
	exit 1
fi

# The identifier word is replaced in place, so comment style and spacing
# survive: in lines 1-10 only when the line has the header form, in the last
# ten lines only when it is a bare footer.
esc() { printf '%s' "$1" | sed 's#/#\\/#g'; }
for f in "${targets[@]}"; do
	n=$(wc -l <"$f")
	from=$((n > 10 ? n - 9 : 1))
	sed -i -E "1,10{/$(esc "$HEADER_RE")/s/(SPDX-License-Identifier:[[:space:]]*)MIT/\1GPL-3.0-or-later/}" -- "$f"
	sed -i -E "${from},\${/$(esc "$FOOTER_RE")/s/(SPDX-License-Identifier:[[:space:]]*)MIT/\1GPL-3.0-or-later/}" -- "$f"
done

left=0
for f in "${targets[@]}"; do
	{ head -n 10 -- "$f" | grep -qE "$HEADER_RE" || tail -n 10 -- "$f" | grep -qE "$FOOTER_RE"; } &&
		{ echo "relicense: still MIT: $f" >&2; left=$((left + 1)); }
done
[[ "$left" -eq 0 ]] || exit 1
echo "relicense: rewrote ${#targets[@]} header(s)"
