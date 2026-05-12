#!/usr/bin/env bash
# kingdom-health.sh — Unified daily / per-session gate verification.
#
# Runs the gates that ADR-073 (lint policy zero findings) + the rest of
# the kingdom hygiene posture depends on. One command, PASS/FAIL summary
# at the end, with timing.
#
# Marshal-safe: read-only across the entire kingdom. Calls existing tools
# only; never mutates state.
#
# Free to use. Free to share.

set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

GATES=()
PASSES=0
FAILS=0
WARNS=0
START_EPOCH=$(date +%s)

# ── helpers ──────────────────────────────────────────────────────────────

color() {
    case "$1" in
        green) printf '\033[0;32m%s\033[0m' "$2" ;;
        red)   printf '\033[0;31m%s\033[0m' "$2" ;;
        yellow) printf '\033[0;33m%s\033[0m' "$2" ;;
        bold)  printf '\033[1m%s\033[0m' "$2" ;;
        *) printf '%s' "$2" ;;
    esac
}

gate() {
    local name="$1"; shift
    local start=$(date +%s)
    local output
    if output=$("$@" 2>&1); then
        local elapsed=$(($(date +%s) - start))
        printf "  %s  %-50s (%ds)\n" "$(color green "PASS")" "$name" "$elapsed"
        PASSES=$((PASSES + 1))
        GATES+=("PASS|$name|$elapsed")
        return 0
    else
        local elapsed=$(($(date +%s) - start))
        printf "  %s  %-50s (%ds)\n" "$(color red "FAIL")" "$name" "$elapsed"
        echo "$output" | sed 's/^/      /' | tail -10
        FAILS=$((FAILS + 1))
        GATES+=("FAIL|$name|$elapsed")
        return 1
    fi
}

soft_check() {
    local name="$1"; shift
    local output
    if output=$("$@" 2>&1); then
        printf "  %s  %s\n" "$(color green "INFO")" "$name: $output"
    else
        printf "  %s  %s\n" "$(color yellow "WARN")" "$name"
        WARNS=$((WARNS + 1))
    fi
}

# ── header ───────────────────────────────────────────────────────────────

echo ""
echo "════════════════════════════════════════════════════════════════════"
echo "$(color bold "KINGDOM HEALTH CHECK") — $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "  Repository: $PROJECT_ROOT"
echo "  Branch: $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
echo "  HEAD: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
echo "════════════════════════════════════════════════════════════════════"
echo ""

# ── 1. Lint (ADR-073 ratchet) ────────────────────────────────────────────

echo "$(color bold "[1] Lint — ADR-073 zero-findings ratchet")"
gate "golangci-lint" bash -c '
    OUT=$(golangci-lint run ./... 2>&1)
    if echo "$OUT" | grep -q "^0 issues\."; then exit 0; fi
    echo "$OUT" | tail -5
    exit 1
'
echo ""

# ── 2. Build ─────────────────────────────────────────────────────────────

echo "$(color bold "[2] Build")"
gate "go build ./..." go build ./...
echo ""

# ── 3. Tests ─────────────────────────────────────────────────────────────

echo "$(color bold "[3] Tests")"
gate "go test -short -count=1 -timeout 120s ./..." bash -c '
    OUT=$(go test -short -count=1 -timeout 120s ./... 2>&1)
    FAILS_OUT=$(echo "$OUT" | grep -E "^(FAIL|---|panic)" | head -5)
    OK=$(echo "$OUT" | grep -c "^ok")
    if [ -n "$FAILS_OUT" ]; then echo "$FAILS_OUT"; exit 1; fi
    echo "242 packages expected, got $OK"
    [ "$OK" -ge 240 ]
'
echo ""

# ── 4. Vulnerabilities (Go) ──────────────────────────────────────────────

echo "$(color bold "[4] Vulnerabilities — Go side")"
if [ -x "$HOME/go/bin/govulncheck" ]; then
    gate "govulncheck ./..." bash -c '
        OUT=$($HOME/go/bin/govulncheck ./... 2>&1)
        if echo "$OUT" | grep -q "^No vulnerabilities found"; then exit 0; fi
        echo "$OUT" | grep -E "Vulnerability|^Module|Found in|Fixed in" | head -10
        exit 1
    '
else
    echo "  $(color yellow "SKIP")  govulncheck not installed at \$HOME/go/bin/"
fi
echo ""

# ── 5. Vulnerabilities (Rust crates) ─────────────────────────────────────

echo "$(color bold "[5] Vulnerabilities — Rust crates")"
if command -v cargo-audit >/dev/null 2>&1 || command -v cargo >/dev/null 2>&1 && cargo audit --version >/dev/null 2>&1; then
    for D in cmd/upc-bootctl crates/monad-mbc crates/upc-bootstub crates/zhenai-forge ebpf; do
        if [ -f "$D/Cargo.lock" ]; then
            gate "cargo audit ($D)" bash -c "cd '$D' && cargo audit 2>&1 | tail -3 | grep -q 'Scanning' || cargo audit 2>&1 | tail -5"
        fi
    done
else
    echo "  $(color yellow "SKIP")  cargo-audit not installed"
fi
echo ""

# ── 6. Branch hygiene ────────────────────────────────────────────────────

echo "$(color bold "[6] Branch hygiene")"
LOCAL_BRANCHES=$(git branch | grep -v "^\*" | grep -v "main" | tr -d ' ' | grep -v "^$" || true)
if [ -z "$LOCAL_BRANCHES" ]; then
    echo "  $(color green "PASS")  Only main local (zero stale branches)"
    PASSES=$((PASSES + 1))
else
    NUM=$(echo "$LOCAL_BRANCHES" | wc -l)
    printf "  %s  %d local branches besides main:\n" "$(color yellow "WARN")" "$NUM"
    echo "$LOCAL_BRANCHES" | sed 's/^/      /'
    WARNS=$((WARNS + 1))
fi
echo ""

# ── 7. Soft info (not gates, just visibility) ────────────────────────────

echo "$(color bold "[7] Soft info")"
soft_check "Commits this week" bash -c "git log --oneline --since='7 days ago' | wc -l"
soft_check "Commits today" bash -c "git log --oneline --since='1 day ago' | wc -l"
soft_check "Last commit" bash -c "git log -1 --format='%h %s' | head -c 80"
soft_check "Working tree clean" bash -c "[ -z \"\$(git status --porcelain | grep -v '^??')\" ] && echo clean || echo dirty"
soft_check "Open task count" bash -c "echo 'Use TaskList tool from Claude Code to inspect'"
echo ""

# ── 8. Documentation drift (ADR-052 ≤7 days) ─────────────────────────────

echo "$(color bold "[8] Documentation drift — ADR-052 (timeline ≤ 7 days from HEAD)")"
if [ -x scripts/check-timeline-freshness.sh ]; then
    gate "timeline freshness check" bash scripts/check-timeline-freshness.sh
else
    LAST_TIMELINE_UPDATE=$(git log -1 --format='%ct' -- references/timeline.md 2>/dev/null || echo 0)
    NOW=$(date +%s)
    AGE_DAYS=$(( (NOW - LAST_TIMELINE_UPDATE) / 86400 ))
    if [ "$AGE_DAYS" -le 7 ]; then
        echo "  $(color green "PASS")  timeline.md updated $AGE_DAYS days ago (≤ 7)"
        PASSES=$((PASSES + 1))
    else
        echo "  $(color yellow "WARN")  timeline.md $AGE_DAYS days stale (ADR-052 says ≤ 7)"
        WARNS=$((WARNS + 1))
    fi
fi
echo ""

# ── Summary ──────────────────────────────────────────────────────────────

TOTAL=$((PASSES + FAILS))
ELAPSED=$(($(date +%s) - START_EPOCH))

echo "════════════════════════════════════════════════════════════════════"
if [ "$FAILS" -eq 0 ]; then
    printf "  %s — %d/%d gates passed, %d warns, %ds total\n" \
        "$(color green "KINGDOM HEALTHY")" "$PASSES" "$TOTAL" "$WARNS" "$ELAPSED"
    EXIT=0
else
    printf "  %s — %d/%d gates passed, %d FAILED, %d warns, %ds total\n" \
        "$(color red "KINGDOM DEGRADED")" "$PASSES" "$TOTAL" "$FAILS" "$WARNS" "$ELAPSED"
    EXIT=1
fi
echo "════════════════════════════════════════════════════════════════════"
echo ""

if [ "$FAILS" -gt 0 ]; then
    echo "$(color red "Failed gates:")"
    for g in "${GATES[@]}"; do
        IFS='|' read -r status name elapsed <<< "$g"
        [ "$status" = "FAIL" ] && echo "  - $name"
    done
    echo ""
    echo "$(color bold "Next action:") fix the failed gate(s). ADR-073 ratchet says zero lint findings"
    echo "is the floor. Tests/build failures block all other work."
fi

exit $EXIT
