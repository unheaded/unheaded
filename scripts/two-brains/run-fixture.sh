#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Runs two-brains repo-task fixtures through the routed loop (S-TWO-BRAINS
# Step 180): each fixture is copied into a fresh agent worktree
# $AGENT_WT_ROOT/eval-<id>-<stamp> on branch zhen/eval-<id>, committed as
# the base, then `zhen-claude -route <route> -family <family> -worktree`
# drafts locally, judges with the fixture's own check in the sandbox and
# (route auto) escalates a failing check to Claude within the caps.
# One JSON line per fixture on stdout.
#
#   scripts/two-brains/run-fixture.sh --route auto --family repo-task --split holdout --limit 3 --cap-usd 0.75
#
# Options: --route auto|local, --family F, --split tune|holdout|all,
# --limit N, --cap-usd USD (the -daily-cap-usd of the ledger below),
# --ledger PATH (default: zhen-claude's), --ids a,b (fixture ids instead of
# the split's first N). Needs llama-server for the local leg and, for
# route auto, Claude credentials in the environment (never in the repo).
# ZHEN_CLAUDE overrides the binary (default: go run ./cmd/zhen-claude).
set -euo pipefail

R="$(cd "$(dirname "$0")/../.." && pwd)"
ROUTE=auto FAMILY=repo-task SPLIT=holdout LIMIT=3 CAP=0.75 LEDGER="" IDS=""
while (($#)); do
	case "$1" in
	--route) ROUTE=$2; shift 2 ;;
	--family) FAMILY=$2; shift 2 ;;
	--split) SPLIT=$2; shift 2 ;;
	--limit) LIMIT=$2; shift 2 ;;
	--cap-usd) CAP=$2; shift 2 ;;
	--ledger) LEDGER=$2; shift 2 ;;
	--ids) IDS=$2; shift 2 ;;
	*) echo "unknown option $1" >&2; exit 2 ;;
	esac
done
case "$ROUTE" in auto | local) ;; *) echo "--route auto|local" >&2; exit 2 ;; esac
[[ "$LIMIT" =~ ^[0-9]+$ ]] || { echo "--limit N" >&2; exit 2; }

AGENT_WT_ROOT="${AGENT_WT_ROOT:-${ZHEN_AGENT_WT_ROOT:-$HOME/.cache/unheaded/zhen-worktrees}}"
mkdir -p "$AGENT_WT_ROOT"
chmod 700 "$AGENT_WT_ROOT"
export ZHEN_AGENT_WT_ROOT="$AGENT_WT_ROOT"

if [[ -n "$IDS" ]]; then
	mapfile -t FIXTURES < <(tr ',' '\n' <<<"$IDS")
else
	mapfile -t FIXTURES < <(python3 - "$R/eval/two-brains/split.jsonl" "$FAMILY" "$SPLIT" "$LIMIT" <<'PY'
import json, sys
path, fam, split, limit = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])
n = 0
for line in open(path):
    t = json.loads(line)
    if t["family"] != fam or t["judge"] != "check" or (split != "all" and t["split"] != split):
        continue
    print(t["ref"].rsplit("/", 1)[1])
    n += 1
    if n >= limit:
        break
PY
)
fi

if [[ -n "${ZHEN_CLAUDE:-}" ]]; then
	ZC=("$ZHEN_CLAUDE")
else
	ZC=(go run "$R/cmd/zhen-claude")
fi
ledger_args=()
[[ -n "$LEDGER" ]] && ledger_args=(-ledger "$LEDGER")
stamp="$(date -u +%Y%m%dT%H%M%SZ)"

for id in "${FIXTURES[@]}"; do
	[[ "$id" =~ ^[a-z0-9-]{1,40}$ ]] || { echo "bad fixture id $id" >&2; exit 2; }
	src="$R/eval/two-brains/fixtures/$id"
	[[ -f "$src/check.sh" ]] || { echo "no fixture $id" >&2; exit 2; }
	wt="$AGENT_WT_ROOT/eval-$id-$stamp"
	mkdir -p "$wt"
	cp -r "$src/." "$wt/"
	rm -f "$wt/solution.patch" # the reference fix never sits where a model can read it
	git -C "$wt" init -q -b "zhen/eval-$id"
	git -C "$wt" -c user.name=zhen -c user.email=zhen@localhost add .
	git -C "$wt" -c user.name=zhen -c user.email=zhen@localhost commit -q --no-gpg-sign -m "base: fixture $id"
	task="$(sed '1{/^<!--/d}' "$src/TASK.md")"
	set +e
	line="$(cd "$R" && "${ZC[@]}" -route "$ROUTE" -family "$FAMILY" -worktree "$wt" -run-check fixture-check -check-path . \
		-routing "$R/configs/zhen-routing.yaml" -checks "$R/configs/zhen-checks.yaml" \
		-effort low -max-tokens 4096 -daily-cap-usd "$CAP" "${ledger_args[@]}" -p "$task" 2>"$wt.log")"
	rc=$?
	set -e
	python3 -c 'import json,sys; d=json.loads(sys.argv[1] or "{}"); d.update(fixture=sys.argv[2], exit=int(sys.argv[3]), worktree=sys.argv[4]); print(json.dumps(d, sort_keys=True))' \
		"$line" "$id" "$rc" "$wt"
done
