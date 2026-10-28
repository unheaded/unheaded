#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# ci-python-lock.sh [DIR] — resolve DIR/requirements.in (default
# .github/ci-python) into DIR/requirements.txt: every package, transitive
# ones included, at an exact version with its sha256s, for `pip install
# --require-hashes --no-deps --only-binary=:all:`. Also used for
# notebooks/ and tomb/grimoire/rag/ (S-TRUE-WIRE Step 374).
#
# Resolution runs uv inside a throwaway container with only that
# directory mounted, never on the host (no package code runs here, ADR-006's
# rule for npm applied to pip). The image is the runners' Python (3.12);
# LOCK_PYTHON_VERSION resolves for another target interpreter.
# After a bump: run this, add any new package names to the dependency
# baseline (ADR-096), and scripts/check-tool-freshness.sh confirms the pins.
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE="python:3.12.15-slim"
DIR="${1:-.github/ci-python}"
DIR="${DIR%/}"
PYVER="${LOCK_PYTHON_VERSION:-3.12}"
[[ -f "$DIR/requirements.in" ]] || { echo "[ci-python-lock] no $DIR/requirements.in" >&2; exit 2; }

# Nothing published in the last COOLDOWN_DAYS is picked, transitive
# dependencies included: hijacked releases are usually pulled within hours.
COOLDOWN_DAYS="${COOLDOWN_DAYS:-7}"
before="$(date -u -d "-${COOLDOWN_DAYS} days" +%Y-%m-%dT%H:%M:%SZ)"
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
# "# cooldown-exempt: <package> <why>" lines in requirements.in: security
# fixes allowed through before the cooldown ends.
exempt=()
while read -r pkg; do
	exempt+=(--exclude-newer-package "$pkg=$now")
done < <(sed -n 's/^# cooldown-exempt: \([A-Za-z0-9._-]*\) .*/\1/p' "$DIR/requirements.in")

docker run --rm --network bridge --user "$(id -u):$(id -g)" -e HOME=/tmp \
	-v "$PWD/$DIR:/w" -w /w "$IMAGE" sh -ec '
	python -m pip install --quiet --no-cache-dir --disable-pip-version-check --target /tmp/uv uv
	pyver="$1"; shift
	/tmp/uv/bin/uv pip compile --quiet --no-header --generate-hashes --python-version "$pyver" \
		--exclude-newer "$@" --output-file requirements.txt requirements.in
' sh "$PYVER" "$before" "${exempt[@]}"
echo "[ci-python-lock] $(grep -c '==' "$DIR/requirements.txt") packages pinned in $DIR/requirements.txt"
