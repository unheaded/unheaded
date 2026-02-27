#!/usr/bin/env bash
set -euo pipefail

# Unheaded Coverage Trending Script
# Tracks Go test coverage over time in CSV format
# Usage: ./scripts/coverage-trend.sh [path-to-coverage.out]

COVERAGE_FILE="${1:-coverage.out}"
TREND_FILE="coverage-history.csv"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Coverage Trending ===${NC}"

# Check if coverage file exists
if [ ! -f "$COVERAGE_FILE" ]; then
  echo -e "${RED}✗ Coverage file not found: $COVERAGE_FILE${NC}"
  echo "  Generate with: go test -coverprofile=$COVERAGE_FILE ./..."
  exit 1
fi

# Extract total coverage percentage
TOTAL_COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | tail -1 | awk '{print $NF}' | tr -d '%')
TIMESTAMP=$(date -u +%Y-%m-%d_%H:%M:%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

echo "Total coverage: ${TOTAL_COVERAGE}%"
echo "Timestamp: $TIMESTAMP"
echo "Commit: $GIT_COMMIT"
echo "Branch: $GIT_BRANCH"

# Initialize CSV header if file doesn't exist
if [ ! -f "$TREND_FILE" ]; then
  echo "date,time,total_coverage,commit,branch" > "$TREND_FILE"
  echo -e "${GREEN}✓ Created new coverage history file: $TREND_FILE${NC}"
fi

# Append current coverage entry
echo "$(date -u +%Y-%m-%d),$(date -u +%H:%M:%S),${TOTAL_COVERAGE},${GIT_COMMIT},${GIT_BRANCH}" >> "$TREND_FILE"
echo -e "${GREEN}✓ Coverage entry recorded: $TREND_FILE${NC}"

# Extract per-package coverage
echo ""
echo -e "${BLUE}Per-Package Coverage:${NC}"
go tool cover -func="$COVERAGE_FILE" | head -n -1 | awk '
  BEGIN {
    FS = "\t"
    print "Package                                        Coverage"
    print "---------------------------------------------------"
  }
  {
    # Extract package name from file path (before .go)
    pkg = $1
    gsub(/\.go$/, "", pkg)
    coverage = $2
    printf "%-45s %s\n", pkg, coverage
  }' | head -30

echo ""
echo -e "${BLUE}Coverage Trend (last 10 entries):${NC}"
tail -10 "$TREND_FILE" | awk -F',' '
  NR == 1 { next }  # skip if header appears in tail
  {
    printf "%s %s  %s%%  (%s)\n", $1, $2, $3, $4
  }'

# Check coverage trend
echo ""
if [ $(wc -l < "$TREND_FILE") -gt 2 ]; then
  PREV_COVERAGE=$(tail -2 "$TREND_FILE" | head -1 | cut -d',' -f3)
  CURR_COVERAGE=$TOTAL_COVERAGE

  if (( $(echo "$CURR_COVERAGE > $PREV_COVERAGE" | bc -l) )); then
    DELTA=$(echo "scale=2; $CURR_COVERAGE - $PREV_COVERAGE" | bc)
    echo -e "${GREEN}✓ Coverage improved by ${DELTA}%${NC}"
  elif (( $(echo "$CURR_COVERAGE < $PREV_COVERAGE" | bc -l) )); then
    DELTA=$(echo "scale=2; $PREV_COVERAGE - $CURR_COVERAGE" | bc)
    echo -e "${YELLOW}⚠ Coverage decreased by ${DELTA}%${NC}"
  else
    echo -e "${BLUE}→ Coverage unchanged${NC}"
  fi
fi

# Generate coverage badge data (for later integration with badge service)
BADGE_COLOR="green"
if (( $(echo "$TOTAL_COVERAGE < 60" | bc -l) )); then
  BADGE_COLOR="red"
elif (( $(echo "$TOTAL_COVERAGE < 75" | bc -l) )); then
  BADGE_COLOR="yellow"
fi

echo ""
echo -e "${BLUE}Badge Data (for CI/CD integration):${NC}"
echo "Color: $BADGE_COLOR"
echo "Label: coverage"
echo "Message: ${TOTAL_COVERAGE}%"
echo "URL: https://img.shields.io/badge/coverage-${TOTAL_COVERAGE}%25-${BADGE_COLOR}"

echo ""
echo -e "${GREEN}=== Coverage Trending Complete ===${NC}"
