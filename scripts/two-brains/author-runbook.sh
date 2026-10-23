#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# author-runbook.sh <category>/<name>: the S-TWO-BRAINS runbook authoring
# pipeline (Step 277). The brief docs/runbook-briefs/<name>.md is the local
# model's only source of facts.
#
#   1. a fresh agent worktree $AGENT_WT_ROOT/runbook-<name>-<stamp> on branch
#      zhen/runbook-<name>, holding the brief, docs/runbook-briefs/SCHEMA.md,
#      two exemplar runbooks and the runbook's test (a stub if not written yet)
#   2. local draft: zhen-claude -route local -family runbook-draft, judged by
#      run_check runbook-check in the sandbox, up to 2 revisions
#   3. the harness, not a model, decides: the repo's own checker on the draft
#   4. Claude review (read-only, no tools, -privacy strict, effort low):
#      findings as JSON [{severity, step, finding, fix}]; the local model
#      applies them; if the harness still fails, Claude patches the file in
#      the worktree (branch-sandbox mode, family runbook-patch)
#   5. authorship written by this script, not by a model: authored_by
#      zhen-local (claude if Claude patched), reviewed_by claude-opus-5-5,
#      approved_by "", status draft, dry_run_test
#
# Prints one JSON summary line on stdout:
#   {name, local_revisions, claude_findings, claude_patched, harness, cost}
# and installs the runbook at runbooks/<category>/<name>.yaml unless
# --no-install. Nothing is committed and nothing is pushed.
#
# Options:
#   --brief FILE         brief (default docs/runbook-briefs/<name>.md)
#   --test PATH          the runbook's dry-run test, repo-relative
#                        (default tests/runbooks/test_<name with _>.py)
#   --claude live|fake|off   review leg (default live; fake reads --fake-findings)
#   --fake-findings FILE JSON findings for --claude fake
#   --local-url URL      llama-server (default $ZHEN_LOCAL_LLM_URL or :8081)
#   --ledger FILE --cap-usd USD   Claude spend ledger and its daily cap
#   --capture DIR        zhen-capture/v1 output (default $XDG_STATE_HOME/unheaded/zhen-capture)
#   --no-install         leave the result in the worktree only
# ZHEN_CLAUDE overrides the zhen-claude binary (default: go run ./cmd/zhen-claude).
set -euo pipefail

R="$(cd "$(dirname "$0")/../.." && pwd)"
usage() { echo "usage: $0 <category>/<name> [options]" >&2; exit 2; }
(($#)) || usage
SPEC=$1; shift
[[ "$SPEC" =~ ^([a-z]+)/([a-z0-9][a-z0-9-]{0,62})$ ]] || usage
CAT=${BASH_REMATCH[1]} NAME=${BASH_REMATCH[2]}
BRIEF="$R/docs/runbook-briefs/$NAME.md"
TEST="tests/runbooks/test_${NAME//-/_}.py"
CLAUDE=live FAKE_FINDINGS="" INSTALL=1 LEDGER="" CAP=5
LOCAL_URL="${ZHEN_LOCAL_LLM_URL:-http://127.0.0.1:8081}"
CAPTURE="${ZHEN_CAPTURE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/unheaded/zhen-capture}"
while (($#)); do
	case "$1" in
	--brief) BRIEF=$2; shift 2 ;;
	--test) TEST=$2; shift 2 ;;
	--claude) CLAUDE=$2; shift 2 ;;
	--fake-findings) FAKE_FINDINGS=$2; shift 2 ;;
	--local-url) LOCAL_URL=$2; shift 2 ;;
	--ledger) LEDGER=$2; shift 2 ;;
	--cap-usd) CAP=$2; shift 2 ;;
	--capture) CAPTURE=$2; shift 2 ;;
	--no-install) INSTALL=0; shift ;;
	*) echo "unknown option $1" >&2; exit 2 ;;
	esac
done
case "$CLAUDE" in live | fake | off) ;; *) usage ;; esac
[[ "$TEST" =~ ^tests/runbooks/test_[a-z0-9_]+\.py$ ]] || { echo "--test must be tests/runbooks/test_<x>.py" >&2; exit 2; }
[[ -f "$BRIEF" ]] || { echo "no brief $BRIEF (write it first; it is the local model's only source of facts)" >&2; exit 2; }
[[ "$CLAUDE" != fake || -f "$FAKE_FINDINGS" ]] || { echo "--claude fake needs --fake-findings FILE" >&2; exit 2; }

AGENT_WT_ROOT="${AGENT_WT_ROOT:-${ZHEN_AGENT_WT_ROOT:-$HOME/.cache/unheaded/zhen-worktrees}}"
mkdir -p "$AGENT_WT_ROOT" && chmod 700 "$AGENT_WT_ROOT"
export ZHEN_AGENT_WT_ROOT="$AGENT_WT_ROOT"
mkdir -p "$CAPTURE" && chmod 700 "$CAPTURE"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
WT="$AGENT_WT_ROOT/runbook-$NAME-$stamp"
WORK="$AGENT_WT_ROOT/runbook-$NAME-$stamp.d" # logs, routing file, review: outside the worktree
REL="runbooks/$CAT/$NAME.yaml"
mkdir -p "$WT" "$WORK"
log() { printf '[author-runbook %s] %s\n' "$NAME" "$*" >&2; }

if [[ -n "${ZHEN_CLAUDE:-}" ]]; then ZC=("$ZHEN_CLAUDE"); else ZC=(go run "$R/cmd/zhen-claude"); fi
ledger_args=()
[[ -n "$LEDGER" ]] && ledger_args=(-ledger "$LEDGER")
gitq() { git -C "$WT" -c user.name=zhen -c user.email=zhen@localhost -c commit.gpgsign=false "$@"; }
snap() { # commit the draft in the agent worktree (its history is the review record)
	gitq add "$REL" 2>/dev/null || return 0
	gitq commit -q --no-gpg-sign -m "$1" 2>/dev/null || true
}

# 1. The worktree. Tracked files are what the local model is shown; the
# checker is copied in untracked, so the sandboxed check can run it but the
# model's prompt does not carry it.
mkdir -p "$WT/docs/runbook-briefs" "$WT/runbooks/data" "$WT/runbooks/deploy" "$WT/runbooks/$CAT" "$WT/tests/runbooks" "$WT/scripts"
cp "$BRIEF" "$WT/docs/runbook-briefs/$NAME.md"
cp "$R/docs/runbook-briefs/SCHEMA.md" "$WT/docs/runbook-briefs/SCHEMA.md"
cp "$R/runbooks/data/postgresql-backup-restore.yaml" "$WT/runbooks/data/"
cp "$R/runbooks/deploy/rolling-upgrade.yaml" "$WT/runbooks/deploy/"
if [[ -f "$R/$TEST" ]]; then cp "$R/$TEST" "$WT/$TEST"; else
	printf '# dry-run test stub for %s (the executing session writes the real one)\n' "$REL" >"$WT/$TEST"
fi
gitq init -q -b "zhen/runbook-$NAME"
gitq add .
gitq commit -q --no-gpg-sign -m "base: brief, schema, exemplars for $REL"
cp "$R/scripts/check-runbooks.py" "$WT/scripts/check-runbooks.py"

cat >"$WORK/routing.yaml" <<'EOF'
version: 1
families:
  runbook-draft:
    route: local_only
    local_model: qwen-coder-14b
    local_attempts: 3
    escalate_on: []
    max_claude_usd: 0.25
  runbook-patch:
    route: claude_direct
    local_model: qwen-coder-14b
    local_attempts: 1
    escalate_on: []
    max_claude_usd: 0.40
default: {route: claude_direct, max_claude_usd: 0.25}
EOF

ledger_total() { # USD spent today in the ledger, or 0
	local f=${LEDGER:-${XDG_STATE_HOME:-$HOME/.local/state}/unheaded/zhen-claude/spend.json}
	python3 - "$f" <<'PY'
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except (OSError, ValueError):
    print("0"); sys.exit()
print(f"{int(d.get('spent_nano_usd', 0)) / 1e9:.6f}")  # pkg/claudebridge/spend.go
PY
}
cost0="$(ledger_total)"

meta_block="  status: draft
  authored_by: zhen-local
  reviewed_by: \"\"
  approved_by: \"\"
  dry_run_test: $TEST"
TASK="Write the runbook file $REL (runbook/v1) for the scenario in docs/runbook-briefs/$NAME.md.
Follow docs/runbook-briefs/SCHEMA.md exactly. The brief is your only source of facts: use only the commands, paths, ports, units and hosts it names, and put its open questions into notes, not into commands.
metadata.name is $NAME. End the metadata block with exactly these lines:
$meta_block
Reply with one fenced block whose opening line is \`\`\`file:$REL and nothing else."

local_run() { # $1 = prompt, $2 = log name
	set +e
	(cd "$R" && "${ZC[@]}" -route local -family runbook-draft -worktree "$WT" -run-check runbook-check -check-path "$REL" \
		-routing "$WORK/routing.yaml" -checks "$R/configs/zhen-checks.yaml" -local-url "$LOCAL_URL" \
		-local-max-tokens 3000 -local-timeout 10m -capture "$CAPTURE" -p "$1") >"$WORK/$2.json" 2>"$WORK/$2.log"
	set -e
	python3 -c 'import json,sys
try: print(json.loads(open(sys.argv[1]).read().strip().splitlines()[-1]).get("local_attempts", 0))
except Exception: print(0)' "$WORK/$2.json"
}

harness() { # the repo's checker, never the worktree's copy
	python3 -B "$R/scripts/check-runbooks.py" --root "$WT" --no-baseline "$WT/$REL" >"$WORK/harness.txt" 2>&1
}

# authorship and status come from this script (step 5)
set_meta() { # $1 authored_by, $2 reviewed_by
	python3 - "$WT/$REL" "$1" "$2" "$TEST" <<'PY'
import re, sys
path, author, reviewer, test = sys.argv[1:5]
want = {"status": "draft", "authored_by": author, "reviewed_by": f'"{reviewer}"' if reviewer == "" else reviewer,
        "approved_by": '""', "dry_run_test": test}
lines = open(path).read().splitlines()
try:
    start = lines.index("metadata:")
except ValueError:
    sys.exit(0)  # unparseable draft: the harness reports it
end = start + 1
while end < len(lines) and (lines[end].startswith("  ") or not lines[end].strip()):
    end += 1
block = [ln for ln in lines[start + 1:end] if not re.match(r"  (%s):" % "|".join(want), ln)]
while block and not block[-1].strip():
    block.pop()
block += [f"  {k}: {v}" for k, v in want.items()]
open(path, "w").write("\n".join(lines[:start + 1] + block + lines[end:]) + "\n")
PY
}

log "worktree $WT; local draft"
a1="$(local_run "$TASK" local-draft)"
revisions=$((a1 > 0 ? a1 - 1 : 0))
[[ -f "$WT/$REL" ]] || { log "no draft written (see $WORK/local-draft.log)"; }
set_meta zhen-local "" 2>/dev/null || true
harness_local=fail
harness && harness_local=pass
log "local draft: attempts=$a1 harness=$harness_local"

findings="[]" nfind=0 patched=false author=zhen-local reviewer=""
if [[ -f "$WT/$REL" && "$CLAUDE" != off ]]; then
	snap "local draft of $REL"
	if [[ "$CLAUDE" == fake ]]; then
		cp "$FAKE_FINDINGS" "$WORK/review.txt"
	else
		review_prompt="Review this draft runbook (runbook/v1 YAML, ADR-024) against its brief. Find factual errors against the brief, unsafe or irreversible steps not marked manual, steps that would not work, missing verification, secrets that could be printed, and schema problems. Do not restate style preferences.
Reply with ONLY a JSON array, no prose: [{\"severity\":\"high|medium|low\",\"step\":\"<step name or metadata>\",\"finding\":\"...\",\"fix\":\"...\"}]. At most 12 findings. [] if none.

--- brief docs/runbook-briefs/$NAME.md
$(cat "$WT/docs/runbook-briefs/$NAME.md")

--- schema rules docs/runbook-briefs/SCHEMA.md (excerpt)
$(sed -n '/^Rules:/,$p' "$WT/docs/runbook-briefs/SCHEMA.md")

--- draft $REL
$(cat "$WT/$REL")"
		set +e
		(cd "$R" && "${ZC[@]}" -tools none -privacy strict -effort low -max-tokens 6000 -daily-cap-usd "$CAP" "${ledger_args[@]}" \
			-capture "$CAPTURE" -p "$review_prompt") >"$WORK/review.txt" 2>"$WORK/review.log"
		set -e
	fi
	findings="$(python3 - "$WORK/review.txt" <<'PY'
import json, re, sys
t = open(sys.argv[1]).read()
m = re.search(r"\[.*\]", t, re.S)
try:
    f = json.loads(m.group(0)) if m else []
    f = [x for x in f if isinstance(x, dict) and x.get("finding")]
except ValueError:
    f = []
print(json.dumps(f))
PY
)"
	nfind="$(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])))' "$findings")"
	printf '%s\n' "$findings" >"$WORK/findings.json"
	reviewer=claude-opus-5-5
	log "review: $nfind finding(s)"
	if ((nfind > 0)); then
		apply_prompt="$TASK

A reviewer found these problems in the current $REL (it is in the worktree). Apply every fix that is consistent with the brief, keep everything else, and reply with the complete corrected file:
$findings"
		a2="$(local_run "$apply_prompt" local-apply)"
		revisions=$((revisions + (a2 > 0 ? a2 : 0)))
		set_meta zhen-local "$reviewer" 2>/dev/null || true
	fi
	if ! harness && [[ "$CLAUDE" == live ]]; then
		log "harness still failing after the local revision: Claude patches in the worktree"
		snap "local revision after review of $REL"
		patch_prompt="The runbook $REL in this worktree fails scripts/check-runbooks.py. Fix it so the check passes, keeping to the brief docs/runbook-briefs/$NAME.md and docs/runbook-briefs/SCHEMA.md. Check output:
$(cat "$WORK/harness.txt")"
		set +e
		(cd "$R" && "${ZC[@]}" -route auto -family runbook-patch -worktree "$WT" -run-check runbook-check -check-path "$REL" \
			-routing "$WORK/routing.yaml" -checks "$R/configs/zhen-checks.yaml" -effort low -max-tokens 4096 \
			-daily-cap-usd "$CAP" "${ledger_args[@]}" -capture "$CAPTURE" -p "$patch_prompt") >"$WORK/claude-patch.json" 2>"$WORK/claude-patch.log"
		set -e
		patched=true author=claude
	fi
fi
[[ -f "$WT/$REL" ]] && set_meta "$author" "$reviewer"
result=fail
harness && result=pass
cost1="$(ledger_total)"
cost="$(python3 -c 'import sys; print(f"{max(0.0, float(sys.argv[2]) - float(sys.argv[1])):.6f}")' "$cost0" "$cost1")"
if ((INSTALL)) && [[ -f "$WT/$REL" ]]; then
	mkdir -p "$R/runbooks/$CAT"
	cp "$WT/$REL" "$R/$REL"
fi
python3 -c 'import json,sys
n,lr,cf,cp,h,c,wt,hl=sys.argv[1:9]
print(json.dumps({"name":n,"local_revisions":int(lr),"claude_findings":int(cf),"claude_patched":cp=="true","harness":h,"harness_local_draft":hl,"cost":float(c),"worktree":wt}, sort_keys=True))' \
	"$NAME" "$revisions" "$nfind" "$patched" "$result" "$cost" "$WT" "$harness_local"
[[ "$result" == pass ]]
