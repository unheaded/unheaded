#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-hadolint.sh — hadolint over every tracked Dockerfile, as a ratchet.
#
# Replaces the advisory CI job (continue-on-error, unpinned
# hadolint:latest-alpine), which could not fail and did not notice that it
# could not parse the root Dockerfile at all: a RUN heredoc stopped the
# parser, so the file every service image is built from had never been
# linted (fixed 2026-10-20).
#
# - A Dockerfile hadolint cannot parse (DL1000) always fails: it means no
#   lint ran on it.
# - Findings are counted per (rule, file) against
#   docs/policy/hadolint-baseline.tsv. A count above its baseline, or a pair
#   not in it, fails. So does a count BELOW it: lock the improvement in with
#   --update in the same commit, so the baseline only ever shrinks.
# - No rule is excluded and no severity is filtered.
#
#   scripts/check-hadolint.sh            # gate
#   scripts/check-hadolint.sh --update   # rewrite the baseline (review the diff)
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="$REPO_ROOT/docs/policy/hadolint-baseline.tsv"
cd "$REPO_ROOT"
# shellcheck source=scripts/lib/tool-pins.sh
source scripts/lib/tool-pins.sh
IMAGE="${HADOLINT_IMAGE:-hadolint/hadolint:v${HADOLINT_VERSION}-alpine@${HADOLINT_IMAGE_DIGEST}}"

if ! docker info >/dev/null 2>&1; then
	echo "[FAIL] docker is required to run hadolint; this gate does not pass by skipping." >&2
	exit 2
fi
mapfile -t FILES < <(git ls-files '*Dockerfile*' | grep -v '^tomb/')
[[ ${#FILES[@]} -gt 0 ]] || { echo "[FAIL] no Dockerfiles found; refusing to pass vacuously" >&2; exit 2; }

OUT="$(mktemp)"
trap 'rm -f "$OUT"' EXIT
docker run --rm -v "$REPO_ROOT:/repo:ro" -w /repo "$IMAGE" \
	hadolint --no-color --no-fail -f json "${FILES[@]}" >"$OUT"

python3 - "$BASELINE" "${1:-}" "${#FILES[@]}" "$OUT" <<'PY'
import collections, json, os, sys
baseline, mode, nfiles, out = sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4]
findings = json.load(open(out))
unparsed = sorted({f["file"] for f in findings if f["code"] == "DL1000"})
cur = collections.Counter((f["code"], f["file"]) for f in findings if f["code"] != "DL1000")
rel = os.path.relpath(baseline)
if unparsed:
    for f in unparsed:
        print(f"[FAIL] hadolint cannot parse {f}: nothing in it was linted (a RUN heredoc is the usual cause)")
    sys.exit(1)
if mode == "--update":
    with open(baseline, "w") as fh:
        fh.write("# hadolint ratchet (scripts/check-hadolint.sh): rule, file, count.\n")
        fh.write("# Counts may only go down. Regenerate with --update after a fix.\n")
        for (rule, path), n in sorted(cur.items()):
            fh.write(f"{rule}\t{path}\t{n}\n")
    print(f"[UPDATED] {rel} ({len(cur)} pairs)")
    sys.exit(0)
if not os.path.exists(baseline):
    print(f"[FAIL] {rel} is missing; create it with: scripts/check-hadolint.sh --update")
    sys.exit(1)
base = {}
for line in open(baseline):
    if line.strip() and not line.startswith("#"):
        rule, path, n = line.rstrip("\n").split("\t")
        base[(rule, path)] = int(n)
fail = False
for (rule, path), n in sorted(cur.items()):
    if n > base.get((rule, path), 0):
        where = [f"{f['file']}:{f['line']}" for f in findings if f["code"] == rule and f["file"] == path]
        print(f"[FAIL] {rule} in {path}: {n} finding(s), baseline {base.get((rule, path), 0)} ({', '.join(where)})")
        fail = True
for (rule, path), n in sorted(base.items()):
    if cur.get((rule, path), 0) < n:
        print(f"[FAIL] {rule} in {path} went down to {cur.get((rule, path), 0)} (baseline {n}): "
              f"run scripts/check-hadolint.sh --update to lock the improvement in")
        fail = True
if fail:
    print("       Fix the new finding, or add '# hadolint ignore=<rule>' with the reason on the line above it. Do not grow the baseline.")
    sys.exit(1)
print(f"[PASS] hadolint: {nfiles} Dockerfiles parsed, no (rule, file) above its baseline ({len(cur)} pairs)")
PY
