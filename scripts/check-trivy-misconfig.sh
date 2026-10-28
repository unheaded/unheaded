#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-trivy-misconfig.sh — trivy's IaC misconfiguration findings may only shrink.
#
# security.yml's trivy-fs job ran `--scanners misconfig` with exit-code 0 and
# nothing read the result (364 findings at creation, 2026-10-28, S-TRUE-WIRE
# Step 364). This gate reads trivy's JSON report and compares it with
# docs/policy/trivy-misconfig-baseline.tsv: one row per (rule ID, file) with
# the number of failed checks. It fails when
#   - a (rule, file) pair is not in the baseline        (a new finding),
#   - a pair's count is above its baseline              (one more of the same),
#   - a pair's count is below its baseline, or the pair is gone
#                                                       (fixed: lock it in with
#                                                        --update in the same commit),
#   - the report is not a trivy JSON report, or scanned no config files
#                                                       (exit 2: no vacuous pass).
# No rule is excluded and no severity is filtered. Documented exceptions live
# in .trivyignore.yaml, which the scan is run with; this gate never skips.
#
# The baseline must come from the trivy version AND checks bundle CI runs:
# TRIVY_VERSION and TRIVY_CHECKS_BUNDLE in scripts/lib/tool-pins.sh, which
# security.yml reads (trivy otherwise fetches the newest checks on every run).
#
#   scripts/check-trivy-misconfig.sh REPORT.json            # gate on a report (CI)
#   scripts/check-trivy-misconfig.sh                        # scan here, then gate (needs docker)
#   scripts/check-trivy-misconfig.sh [REPORT.json] --update # drop fixed rows / lower counts only
#   scripts/check-trivy-misconfig.sh [REPORT.json] --seed   # first write; refuses if the baseline exists
#
# Without a report the script scans the tracked files of the working tree (what
# CI checks out, with local edits) in the pinned trivy image, by digest, as the
# calling user, caching the checks bundle under ${XDG_CACHE_HOME:-~/.cache}/unheaded/trivy.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${TRIVY_MISCONFIG_BASELINE:-$REPO_ROOT/docs/policy/trivy-misconfig-baseline.tsv}"

REPORT=""
MODE=""
for arg in "$@"; do
	case "$arg" in
	--update | --seed) MODE="$arg" ;;
	--*)
		echo "[FAIL] unknown option '$arg' (expected --update or --seed)" >&2
		exit 2
		;;
	*)
		[[ -z "$REPORT" ]] || { echo "[FAIL] one report at most (got '$REPORT' and '$arg')" >&2; exit 2; }
		REPORT="$arg"
		;;
	esac
done

if [[ -n "$REPORT" ]]; then
	[[ -f "$REPORT" ]] || { echo "[FAIL] report not found: '$REPORT'" >&2; exit 2; }
else
	# shellcheck source=scripts/lib/tool-pins.sh
	source "$REPO_ROOT/scripts/lib/tool-pins.sh"
	if ! timeout 20 docker info >/dev/null 2>&1; then
		echo "[FAIL] no report given and docker is unavailable; this gate does not pass by skipping." >&2
		exit 2
	fi
	CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/unheaded/trivy"
	mkdir -p "$CACHE"
	WORK="$(mktemp -d)"
	trap 'rm -rf "$WORK"' EXIT
	mkdir -p "$WORK/repo"
	(cd "$REPO_ROOT" && git ls-files -z | tar --null --ignore-failed-read -T - -cf - 2>/dev/null) | tar -x -C "$WORK/repo"
	REPORT="$WORK/trivy-misconfig.json"
	if ! timeout 1800 docker run --rm --user "$(id -u):$(id -g)" -e HOME=/cache \
		-e TRIVY_CHECKS_BUNDLE_REPOSITORY="$TRIVY_CHECKS_BUNDLE" \
		-v "$WORK/repo:/repo:ro" -v "$CACHE:/cache" -v "$WORK:/out" -w /repo \
		"aquasec/trivy:${TRIVY_VERSION}@${TRIVY_IMAGE_DIGEST}" \
		fs --cache-dir /cache --quiet --scanners misconfig --ignorefile .trivyignore.yaml \
		--exit-code 0 -f json -o /out/trivy-misconfig.json . >&2; then
		echo "[FAIL] the trivy scan itself failed (see above); no verdict" >&2
		exit 2
	fi
fi

python3 -I - "$BASELINE" "$REPORT" "$MODE" <<'PY'
import collections, json, os, sys

baseline, report, mode = sys.argv[1], sys.argv[2], sys.argv[3]
rel = os.path.relpath(baseline)
try:
    d = json.load(open(report, encoding="utf-8"))
except (OSError, ValueError) as e:
    print(f"[FAIL] {report}: not readable JSON ({e})")
    sys.exit(2)
if not isinstance(d, dict) or "SchemaVersion" not in d or "ArtifactName" not in d:
    print(f"[FAIL] {report}: not a trivy JSON report (no SchemaVersion/ArtifactName)")
    sys.exit(2)
results = d.get("Results") or []
configs = sum(1 for r in results if r.get("Class") == "config")
if configs == 0:
    print(f"[FAIL] {report}: no config files were scanned; was it run with --scanners misconfig?")
    sys.exit(2)

cur = collections.Counter()
for r in results:
    for m in r.get("Misconfigurations") or []:
        if m.get("Status") == "FAIL":
            rule = m.get("AVDID") or m.get("ID")
            if not rule or "\t" in rule or "\t" in r["Target"]:
                print(f"[FAIL] {report}: unusable finding {m.get('ID')!r} in {r.get('Target')!r}")
                sys.exit(2)
            cur[(rule, r["Target"])] += 1

def write(rows):
    tmp = baseline + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write("# trivy misconfig ratchet (scripts/check-trivy-misconfig.sh): rule, file, failed checks.\n")
        fh.write("# Counts may only go down. Regenerate with --update after a fix; never add a row by hand.\n")
        for (rule, path), n in sorted(rows.items()):
            fh.write(f"{rule}\t{path}\t{n}\n")
    os.replace(tmp, baseline)

if mode == "--seed":
    if os.path.exists(baseline):
        print(f"[FAIL] {rel} exists; --seed writes only the first baseline (use --update)")
        sys.exit(1)
    write(cur)
    print(f"[SEEDED] {rel}: {len(cur)} (rule, file) pairs, {sum(cur.values())} failed checks, {configs} config files")
    sys.exit(0)

if not os.path.exists(baseline):
    print(f"[FAIL] {rel} missing; a missing baseline is not an empty one")
    sys.exit(2)
base = {}
for n, line in enumerate(open(baseline, encoding="utf-8"), 1):
    line = line.rstrip("\n")
    if not line or line.startswith("#"):
        continue
    parts = line.split("\t")
    if len(parts) != 3 or not parts[2].isdigit() or int(parts[2]) < 1:
        print(f"[FAIL] {rel}:{n}: expected 'rule<TAB>file<TAB>count>=1', got {line!r}")
        sys.exit(2)
    key = (parts[0], parts[1])
    if key in base:
        print(f"[FAIL] {rel}:{n}: duplicate row for {key}")
        sys.exit(2)
    base[key] = int(parts[2])

new = sorted(k for k in cur if k not in base)
more = sorted(k for k in cur if k in base and cur[k] > base[k])
fewer = sorted(k for k in base if cur.get(k, 0) < base[k])

if mode == "--update":
    if new or more:
        for k in new:
            print(f"[FAIL] --update refuses to add {k[0]} in {k[1]} ({cur[k]}): fix it, or list it in .trivyignore.yaml with a reason")
        for k in more:
            print(f"[FAIL] --update refuses to raise {k[0]} in {k[1]}: {base[k]} -> {cur[k]}")
        sys.exit(1)
    write({k: cur[k] for k in base if cur.get(k, 0) > 0})
    print(f"[UPDATED] {rel}: {len(fewer)} row(s) lowered or removed")
    sys.exit(0)

rc = 0
for k in new:
    print(f"[FAIL] new misconfiguration: {k[0]} in {k[1]} ({cur[k]})")
    rc = 1
for k in more:
    print(f"[FAIL] more of {k[0]} in {k[1]}: {cur[k]} (baseline {base[k]})")
    rc = 1
for k in fewer:
    print(f"[FAIL] {k[0]} in {k[1]} now PASSES {base[k] - cur.get(k, 0)} more check(s) but is still listed at {base[k]}: "
          f"lock it in with --update")
    rc = 1
if rc == 0:
    print(f"[PASS] trivy misconfig: {sum(cur.values())} failed checks in {len(cur)} (rule, file) pairs, "
          f"all at baseline ({configs} config files scanned)")
sys.exit(rc)
PY
