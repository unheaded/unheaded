#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# test-pre-commit.sh — Audit-trail test for scripts/git-hooks/pre-commit.
#
# Verifies the hook behaves correctly:
#   T1: empty stage     → exit 0 (silent).
#   T2: gofmt-drift     → exit non-zero, prints offending file.
#   T3: clean Go file   → exit 0.
#   T4: rustfmt-drift   → exit non-zero, prints offending file (skipped if
#                         rustfmt is not on PATH — soft-required toolchain).
#   T5: clean Rust file → exit 0 (skipped if rustfmt missing).
#
# Self-contained: stages temp files inside the repo, runs the hook, then
# unstages and removes them in a trap. Aborts with PASS/FAIL line.
#
# Run: bash scripts/git-hooks/test-pre-commit.sh

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

HOOK="scripts/git-hooks/pre-commit"
SCRATCH_DIR="scripts/git-hooks/.scratch-test"
DRIFT_FILE="$SCRATCH_DIR/drift_test.go"
CLEAN_FILE="$SCRATCH_DIR/clean_test.go"
RS_DRIFT_FILE="$SCRATCH_DIR/drift_test.rs"
RS_CLEAN_FILE="$SCRATCH_DIR/clean_test.rs"

cleanup() {
    local rc=$?
    # Unstage anything we staged, then remove scratch dir.
    if [ -d "$SCRATCH_DIR" ]; then
        git reset -q -- "$SCRATCH_DIR" 2>/dev/null || true
        rm -rf "$SCRATCH_DIR"
    fi
    return $rc
}
trap cleanup EXIT

mkdir -p "$SCRATCH_DIR"

# Drift file: missing space after package keyword and bad indentation/braces.
# gofmt will rewrite it, so `gofmt -l` reports it.
cat >"$DRIFT_FILE" <<'EOF'
// SPDX-License-Identifier: GPL-3.0-or-later
package scratch
import"fmt"
func   Bad( ){fmt.Println( "drift" ) }
EOF

# Clean file: already gofmt'd and vet-clean.
cat >"$CLEAN_FILE" <<'EOF'
// SPDX-License-Identifier: GPL-3.0-or-later
package scratch

import "fmt"

func Good() { fmt.Println("clean") }
EOF

# Rust drift file: bad spacing + braces. rustfmt --check will report.
cat >"$RS_DRIFT_FILE" <<'EOF'
// SPDX-License-Identifier: GPL-3.0-or-later
fn   bad(  )->i32{let   x=1   ;x+1}
EOF

# Rust clean file: already rustfmt'd.
cat >"$RS_CLEAN_FILE" <<'EOF'
// SPDX-License-Identifier: GPL-3.0-or-later
fn good() -> i32 {
    let x = 1;
    x + 1
}
EOF

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# ---------- T1: empty stage → hook exits 0 silently ----------
# (Nothing staged because we haven't `git add`-ed yet.)
out_t1="$(bash "$HOOK" 2>&1)"
rc_t1=$?
if [ "$rc_t1" -ne 0 ]; then
    fail "T1 empty stage: expected exit 0, got $rc_t1 (output: $out_t1)"
fi
if [ -n "$out_t1" ]; then
    fail "T1 empty stage: expected silent, got: $out_t1"
fi

# ---------- T2: drift staged → hook exits non-zero, names file ----------
git add -- "$DRIFT_FILE"
set +e
out_t2="$(bash "$HOOK" 2>&1)"
rc_t2=$?
set -e
git reset -q -- "$DRIFT_FILE"

if [ "$rc_t2" -eq 0 ]; then
    fail "T2 drift: hook exited 0, expected non-zero. Output: $out_t2"
fi
if ! echo "$out_t2" | grep -q "gofmt drift"; then
    fail "T2 drift: expected 'gofmt drift' in output, got: $out_t2"
fi
if ! echo "$out_t2" | grep -q "drift_test.go"; then
    fail "T2 drift: expected offending filename in output, got: $out_t2"
fi

# ---------- T3: clean staged → hook exits 0 ----------
git add -- "$CLEAN_FILE"
set +e
out_t3="$(bash "$HOOK" 2>&1)"
rc_t3=$?
set -e
git reset -q -- "$CLEAN_FILE"

if [ "$rc_t3" -ne 0 ]; then
    fail "T3 clean: hook exited $rc_t3, expected 0. Output: $out_t3"
fi

# ---------- T4 + T5: Rust paths (skipped if rustfmt unavailable) ----------
if command -v rustfmt >/dev/null 2>&1; then
    # T4: drift staged → hook exits non-zero, names file.
    git add -- "$RS_DRIFT_FILE"
    set +e
    out_t4="$(bash "$HOOK" 2>&1)"
    rc_t4=$?
    set -e
    git reset -q -- "$RS_DRIFT_FILE"

    if [ "$rc_t4" -eq 0 ]; then
        fail "T4 rustfmt drift: hook exited 0, expected non-zero. Output: $out_t4"
    fi
    if ! echo "$out_t4" | grep -q "rustfmt drift"; then
        fail "T4 rustfmt drift: expected 'rustfmt drift' in output, got: $out_t4"
    fi
    if ! echo "$out_t4" | grep -q "drift_test.rs"; then
        fail "T4 rustfmt drift: expected offending filename in output, got: $out_t4"
    fi

    # T5: clean staged → hook exits 0.
    git add -- "$RS_CLEAN_FILE"
    set +e
    out_t5="$(bash "$HOOK" 2>&1)"
    rc_t5=$?
    set -e
    git reset -q -- "$RS_CLEAN_FILE"

    if [ "$rc_t5" -ne 0 ]; then
        fail "T5 rustfmt clean: hook exited $rc_t5, expected 0. Output: $out_t5"
    fi

    echo "PASS: T1 empty=silent/0  T2 gofmt-drift=blocked  T3 go-clean=passes  T4 rustfmt-drift=blocked  T5 rs-clean=passes"
else
    echo "PASS: T1 empty=silent/0  T2 gofmt-drift=blocked  T3 go-clean=passes  (T4/T5 skipped — rustfmt not installed)"
fi
exit 0
