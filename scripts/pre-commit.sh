#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# pre-commit.sh — Pre-commit hook for Unheaded development
# Install: cp scripts/pre-commit.sh .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit
# Or:      ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit
#
# Runs fast checks before each commit:
#   1. go vet
#   2. SPDX header verification
#   3. go build
#
# Designed to complete in < 30 seconds for a fast feedback loop.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")" && git rev-parse --show-toplevel 2>/dev/null || cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

PASS=0
FAIL=0
ERRORS=""

step() {
    local name="$1"
    shift
    printf "  %-30s" "${name}..."
    if "$@" > /dev/null 2>&1; then
        echo "OK"
        PASS=$((PASS + 1))
    else
        echo "FAIL"
        FAIL=$((FAIL + 1))
        ERRORS="${ERRORS}\n  - ${name}"
    fi
}

echo "=== Unheaded Pre-Commit Checks ==="
echo ""

# 1. go vet — catch common mistakes
step "go vet" go vet ./...

# 2. SPDX headers — all Go files must have license identifiers
printf "  %-30s" "SPDX headers..."
missing=$(find . -name "*.go" -not -path "./.git/*" -not -path "./vendor/*" \
    -exec grep -L "SPDX-License-Identifier" {} + 2>/dev/null | head -10)
if [ -n "$missing" ]; then
    echo "FAIL"
    FAIL=$((FAIL + 1))
    count=$(echo "$missing" | wc -l)
    ERRORS="${ERRORS}\n  - SPDX headers (${count} files missing)"
    echo "    Files missing SPDX-License-Identifier:"
    echo "$missing" | sed 's/^/      /'
else
    echo "OK"
    PASS=$((PASS + 1))
fi

# 3. go build — ensure code compiles
step "go build" go build ./...

# Summary
echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="

if [ "${FAIL}" -gt 0 ]; then
    echo ""
    printf "Failed checks:${ERRORS}\n"
    echo ""
    echo "Commit blocked. Fix the issues above and try again."
    echo "To bypass (not recommended): git commit --no-verify"
    exit 1
fi

echo "All checks passed."
exit 0
