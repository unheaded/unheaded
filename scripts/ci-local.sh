#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
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

# 6. gosec: the pinned build CI uses (tools/gosec; see tools/gosec/README.md)
GOSEC_BIN="$(mktemp -d)/gosec"
if go -C tools/gosec build -o "$GOSEC_BIN" github.com/securego/gosec/v2/cmd/gosec 2>/dev/null; then
    run_step "gosec" "$GOSEC_BIN" -quiet ./...
else
    skip_step "gosec" "could not build tools/gosec"
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

# 10. haproxy configs parse (S-CLEAN-SLATE M-035)
if command -v docker &>/dev/null && docker info &>/dev/null; then
    run_step "haproxy configs" scripts/check-haproxy-cfg.sh
elif command -v haproxy &>/dev/null; then
    run_step "haproxy configs (host binary only)" env HAPROXY_CHECK_SKIP_IMAGE=1 scripts/check-haproxy-cfg.sh
else
    skip_step "haproxy configs" "neither docker nor haproxy installed"
fi

# 11. SLOs as data (S-WELL-SPRING Phase 3): the Well suite on a throwaway
#     server, then the drift leg against the live Well when WELL_DB is set
#     (WELL_DB=unheaded_ops WELL_PASSWORD=<ops_writer's, from .env>; read
#     only: -check never writes).
if command -v docker &>/dev/null && docker info &>/dev/null; then
    run_step "SLO definitions (throwaway Well)" scripts/well/test-slo-definitions.sh
else
    skip_step "SLO definitions (throwaway Well)" "docker not available"
fi
if [ -n "${WELL_DB:-}" ]; then
    run_step "alert policy vs the live Well" go run ./cmd/alert-policy -check
else
    skip_step "alert policy vs the live Well" "set WELL_DB and WELL_PASSWORD (ops_writer) on west"
fi

# 11b. Retention (S-WELL-SPRING Phase 4): migrations 042/043 and
#      cmd/well-retention on a throwaway server; never the live Well.
if command -v docker &>/dev/null && docker info &>/dev/null; then
    run_step "retention job (throwaway Well)" scripts/well/test-retention.sh
else
    skip_step "retention job (throwaway Well)" "docker not available"
fi

# 12. Perf regression (west only, needs the running stack; M-089).
#     Opt-in: it publishes to Wotan (bench.perf.packet only) and holds
#     1000 req/s against -health for 60 s, so a casual run never loads the
#     stack. container-start is not run here (it creates LXD instances).
run_step "perf regression gate (fixtures)" scripts/test-check-perf-regression.sh
if [ "${CI_LOCAL_PERF:-0}" != "1" ]; then
    skip_step "perf regression (live)" "set CI_LOCAL_PERF=1 on west with the stack up"
elif [ ! -r docs/policy/perf-baseline.json ]; then
    skip_step "perf regression (live)" "no docs/policy/perf-baseline.json yet (S-WELL-SPRING Step 225)"
else
    PERF_REPORT="$(mktemp --suffix=.json)"
    run_step "perf regression (live)" bash -c '
        go run ./cmd/perf-benchmark -scenario e2e-trace,http-load,http3 -iterations 200 -duration 60s -output "$1" &&
        scripts/check-perf-regression.sh "$1"
    ' _ "${PERF_REPORT}"
    rm -f "${PERF_REPORT}"
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
