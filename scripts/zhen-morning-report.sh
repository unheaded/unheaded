#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# zhen-morning-report.sh — print the overnight worker's morning report
# (S-CHAMPION-HARDENING Phase 13). zhen-agentd -overnight (the operator's
# user unit deploy/systemd/user/zhen-agentd-overnight.service) writes one
# Markdown file per night to its -report-dir, by default
# ${XDG_STATE_HOME:-~/.local/state}/unheaded/zhen-reports/<YYYY-MM-DD>.md (0600),
# with the night's runs, spend, cards and branches to review.
#
#   scripts/zhen-morning-report.sh                    # newest report
#   scripts/zhen-morning-report.sh --date 2026-10-28  # that night's
#   scripts/zhen-morning-report.sh --dir DIR          # another report dir
#   scripts/zhen-morning-report.sh --list             # list reports
#
# Exit: 0 printed, 1 no report, 2 usage or unreadable.
set -euo pipefail

DIR="${XDG_STATE_HOME:-$HOME/.local/state}/unheaded/zhen-reports" DATE="" LIST=0
while [[ $# -gt 0 ]]; do
	case "$1" in
	--dir) DIR="${2:?--dir needs a directory}"; shift ;;
	--date) DATE="${2:?--date needs YYYY-MM-DD}"; shift ;;
	--list) LIST=1 ;;
	-h | --help) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
	*) echo "zhen-morning-report: unknown argument $1" >&2; exit 2 ;;
	esac
	shift
done
[[ -z "$DATE" || "$DATE" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "zhen-morning-report: --date wants YYYY-MM-DD" >&2; exit 2; }
if [[ ! -d "$DIR" ]]; then
	echo "zhen-morning-report: no report directory $DIR (no night has run)" >&2
	exit 1
fi
if [[ ! -r "$DIR" || ! -x "$DIR" ]]; then
	echo "zhen-morning-report: cannot read $DIR (0700, owned by whoever ran the night)" >&2
	exit 2
fi
mapfile -t reports < <(find "$DIR" -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].md' -printf '%f\n' | sort)
if [[ ${#reports[@]} -eq 0 ]]; then
	echo "zhen-morning-report: no reports in $DIR" >&2
	exit 1
fi
if [[ "$LIST" -eq 1 ]]; then
	printf '%s\n' "${reports[@]%.md}"
	exit 0
fi
pick="${reports[-1]}"
if [[ -n "$DATE" ]]; then
	pick="$DATE.md"
	[[ -f "$DIR/$pick" ]] || { echo "zhen-morning-report: no report for $DATE in $DIR" >&2; exit 1; }
fi
cat -- "$DIR/$pick"
