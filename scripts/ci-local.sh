#!/bin/bash
# SPDX-License-Identifier: MIT
# ci-local.sh - Run the full CI pipeline locally
# Part of Unheaded S77 Phase 3: SBOM + CI/CD Fortress
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

PASS=0
FAIL=0
SKIP=0
RESULTS=()

run_step() {
    local name="$1"
    shift
    echo ""
    echo "================================================================"
    echo "  STEP: ${name}"
    echo "================================================================"
    if "$@"; then
        PASS=$((PASS + 1))
        RESULTS+=("PASS  ${name}")
        echo "--- ${name}: PASS ---"
    else
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  ${name}")
        echo "--- ${name}: FAIL ---"
    fi
}

skip_step() {
    local name="$1"
    local reason="$2"
    SKIP=$((SKIP + 1))
    RESULTS+=("SKIP  ${name} (${reason})")
    echo "--- ${name}: SKIP (${reason}) ---"
}

echo "========================================"
echo "  Unheaded Local CI Pipeline"
echo "  Started: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "========================================"

# 1. Go Build
run_step "Go Build" go build ./...

# 2. Go Test (with race detector)
run_step "Go Test (-race)" go test ./... -race -count=1 -timeout 300s

# 3. Go Vet
run_step "Go Vet" go vet ./...

# 4. SPDX Header Check
run_step "SPDX Headers" bash -c '
    missing=$(find . -name "*.go" -not -path "./.git/*" -not -path "./vendor/*" \
        -exec grep -L "SPDX-License-Identifier" {} + 2>/dev/null | head -20)
    if [ -n "$missing" ]; then
        echo "Files missing SPDX-License-Identifier:"
        echo "$missing"
        exit 1
    fi
'

# 5. Rust Check + Build across all kingdom workspaces.
#    Pre-2026-05-08: only crates/monad-mbc was wired (cmd/waf had AI-slop
#    parse blockers). After commits 2b055608 + ac73e8e7 closed the cmd/waf
#    raw-string + hyper API drift, the workspace is buildable; extend CI
#    coverage so future regressions are caught before they accumulate.
RUST_WORKSPACES=(
    crates/monad-mbc
    crates/zhend
    crates/doom-runner
    crates/zhenai-forge
    cmd/trace-collector
    cmd/ebpf-loader
    cmd/waf
)
if command -v cargo &>/dev/null; then
    for ws in "${RUST_WORKSPACES[@]}"; do
        if [ -f "${ws}/Cargo.toml" ]; then
            run_step "Rust Check (${ws})" cargo check --manifest-path "${ws}/Cargo.toml"
        else
            skip_step "Rust Check (${ws})" "Cargo.toml not present"
        fi
    done
    # Run tests on a curated subset (faster CI; full sweep happens in Jenkins).
    for ws in crates/monad-mbc crates/zhend cmd/trace-collector; do
        if [ -f "${ws}/Cargo.toml" ]; then
            run_step "Rust Test (${ws})" cargo test --manifest-path "${ws}/Cargo.toml" --release
        fi
    done
else
    skip_step "Rust Check" "cargo not installed"
fi

# 6. gosec (optional)
if command -v gosec &>/dev/null; then
    run_step "gosec" gosec -quiet ./...
else
    skip_step "gosec" "not installed; run: go install github.com/securego/gosec/v2/cmd/gosec@v2.21.0"
fi

# 7. govulncheck (optional)
if command -v govulncheck &>/dev/null; then
    run_step "govulncheck" govulncheck ./...
else
    skip_step "govulncheck" "not installed; run: go install golang.org/x/vuln/cmd/govulncheck@latest"
fi

# 8. SBOM generation (optional, requires make)
if command -v make &>/dev/null && grep -q '^sbom:' Makefile 2>/dev/null; then
    run_step "SBOM Generation" make sbom
else
    skip_step "SBOM Generation" "make or sbom target not available"
fi

# 9. GPL Boundary Verification
if [ -x scripts/verify-gpl-boundary.sh ]; then
    run_step "GPL Boundary Check" scripts/verify-gpl-boundary.sh
else
    skip_step "GPL Boundary Check" "scripts/verify-gpl-boundary.sh not found or not executable"
fi

# Summary
echo ""
echo "========================================"
echo "  Local CI Summary"
echo "  Finished: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "========================================"
echo ""
for r in "${RESULTS[@]}"; do
    echo "  ${r}"
done
echo ""
echo "  Passed: ${PASS}  Failed: ${FAIL}  Skipped: ${SKIP}"
echo "========================================"

if [ "${FAIL}" -gt 0 ]; then
    echo ""
    echo "CI FAILED - ${FAIL} step(s) did not pass."
    exit 1
fi

echo ""
echo "CI PASSED - all executed steps succeeded."
exit 0
