#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# verdicts-record.sh — Record hypothesis verdicts after QA testing
# Usage: ./verdicts-record.sh [--batch --h1 PASS --h1-value "measured: ..."]
# Records verdicts to /var/lib/unheaded/verdicts/YYYY-MM-DD.json
# Also appends to VERDICTS.md running log

set -euo pipefail
IFS=$'\n\t'

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
TODAY=$(date +%Y-%m-%d)
TIMESTAMP=$(date +%Y-%m-%d_%H:%M:%S)
BATCH_MODE=false
GIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERDICTS_DIR="/var/lib/unheaded/verdicts"

# Initialize verdict values
declare -A verdicts
declare -A verdict_values

# Hypothesis definitions
declare -A hypothesis_desc=(
    [H1]="XDP P99 latency < 1µs under 100K pps"
    [H2]="eBPF policy enforcement < 5% CPU overhead"
    [H3]="Service restart < 30s after pod kill"
    [H4]="WireGuard RTT P99 < 5ms between hosts"
    [H5]="vLLM inference > 30 tok/s on RX 7700 XT"
    [H6]="Monad register overhead < 2µs per packet"
    [H7]="WireGuard added overhead < 1ms over bare LAN"
    [H8]="End-to-end trace propagation < 10ms"
)

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --batch)
            BATCH_MODE=true
            shift
            ;;
        --h1|--h2|--h3|--h4|--h5|--h6|--h7|--h8)
            hypothesis=${1:2}
            if [[ "$2" =~ ^(PASS|FAIL)$ ]]; then
                verdicts[$hypothesis]="$2"
                shift 2
            else
                echo "Invalid verdict: $2 (must be PASS or FAIL)" >&2
                exit 1
            fi
            ;;
        --h[1-8]-value)
            hypothesis=$(echo "$1" | sed 's/--h//' | sed 's/-value//')
            verdict_values[$hypothesis]="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Helper functions
log_section() {
    echo ""
    echo -e "${BLUE}=== $1 ===${NC}"
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# ===== INTERACTIVE MODE =====
if [[ "$BATCH_MODE" == "false" ]]; then
    log_section "Unheaded Kingdom — Hypothesis Verdict Recording"
    echo "Interactive mode: Recording verdicts for H1-H8"
    echo ""
    
    for h in H1 H2 H3 H4 H5 H6 H7 H8; do
        echo -e "${YELLOW}$h${NC} — ${hypothesis_desc[$h]}"
        
        # Verdict prompt
        while true; do
            read -p "  Verdict (PASS/FAIL): " verdict_input
            case "$verdict_input" in
                PASS|FAIL)
                    verdicts[$h]="$verdict_input"
                    break
                    ;;
                *)
                    echo "  Invalid input. Please enter PASS or FAIL."
                    ;;
            esac
        done
        
        # Measured value prompt
        read -p "  Measured value (optional): " measured_input
        if [[ -n "$measured_input" ]]; then
            verdict_values[$h]="$measured_input"
        fi
        
        echo ""
    done
fi

# ===== Validation =====
log_section "Validating Verdicts"

incomplete=0
for h in H1 H2 H3 H4 H5 H6 H7 H8; do
    if [[ -z "${verdicts[$h]:-}" ]]; then
        echo -e "${RED}[FAIL]${NC} $h not specified"
        ((incomplete++))
    else
        echo -e "${GREEN}[PASS]${NC} $h = ${verdicts[$h]}"
        if [[ -n "${verdict_values[$h]:-}" ]]; then
            echo "        (${verdict_values[$h]})"
        fi
    fi
done

if [[ $incomplete -gt 0 ]]; then
    echo ""
    echo -e "${RED}Error: $incomplete verdict(s) missing${NC}"
    exit 1
fi

# ===== Create JSON verdict file =====
log_section "Recording Verdicts"

mkdir -p "$VERDICTS_DIR"

# Build JSON manually
json_file="$VERDICTS_DIR/${TODAY}.json"

cat > "$json_file" << JSON_EOF
{
  "metadata": {
    "date": "$TODAY",
    "timestamp": "$TIMESTAMP",
    "git_hash": "$GIT_HASH",
    "recorded_by": "${USER:-system}",
    "methodology": "Strong Inference (Platt 1964)"
  },
  "verdicts": {
    "H1": {
      "hypothesis": "${hypothesis_desc[H1]}",
      "verdict": "${verdicts[H1]}",
      "measured": "${verdict_values[H1]:-}"
    },
    "H2": {
      "hypothesis": "${hypothesis_desc[H2]}",
      "verdict": "${verdicts[H2]}",
      "measured": "${verdict_values[H2]:-}"
    },
    "H3": {
      "hypothesis": "${hypothesis_desc[H3]}",
      "verdict": "${verdicts[H3]}",
      "measured": "${verdict_values[H3]:-}"
    },
    "H4": {
      "hypothesis": "${hypothesis_desc[H4]}",
      "verdict": "${verdicts[H4]}",
      "measured": "${verdict_values[H4]:-}"
    },
    "H5": {
      "hypothesis": "${hypothesis_desc[H5]}",
      "verdict": "${verdicts[H5]}",
      "measured": "${verdict_values[H5]:-}"
    },
    "H6": {
      "hypothesis": "${hypothesis_desc[H6]}",
      "verdict": "${verdicts[H6]}",
      "measured": "${verdict_values[H6]:-}"
    },
    "H7": {
      "hypothesis": "${hypothesis_desc[H7]}",
      "verdict": "${verdicts[H7]}",
      "measured": "${verdict_values[H7]:-}"
    },
    "H8": {
      "hypothesis": "${hypothesis_desc[H8]}",
      "verdict": "${verdicts[H8]}",
      "measured": "${verdict_values[H8]:-}"
    }
  }
}
JSON_EOF

log_info "JSON verdict file: $json_file"

# ===== Calculate Score =====
log_section "Verdict Summary"

passed=0
for h in H1 H2 H3 H4 H5 H6 H7 H8; do
    if [[ "${verdicts[$h]}" == "PASS" ]]; then
        ((passed++))
        echo -e "${GREEN}[✓]${NC} $h — PASS"
    else
        echo -e "${RED}[✗]${NC} $h — FAIL"
    fi
done

score="$passed/8"
echo ""
echo -e "${BLUE}Overall Score: ${GREEN}$score${NC} hypotheses confirmed${NC}"

# ===== Append to Markdown Log =====
log_section "Updating Verdicts Log"

verdicts_md="$VERDICTS_DIR/VERDICTS.md"

# Check if this is the first entry
if [[ ! -f "$verdicts_md" ]]; then
    # Create the markdown template
    cat > "$verdicts_md" << 'MD_EOF'
# Unheaded Kingdom — Hypothesis Verdicts Log

Pre-registered hypotheses (H1-H8) from BARE_METAL_QA_PLAN.md.
Strong Inference methodology (Platt 1964). Bootstrap CI N=10,000. α=0.05.

## Verdict History

| Date | H1 | H2 | H3 | H4 | H5 | H6 | H7 | H8 | Score |
|------|----|----|----|----|----|----|----|----|-------|
MD_EOF
fi

# Convert verdicts to table row
h1_result=$(echo "${verdicts[H1]}" | cut -c1)
h2_result=$(echo "${verdicts[H2]}" | cut -c1)
h3_result=$(echo "${verdicts[H3]}" | cut -c1)
h4_result=$(echo "${verdicts[H4]}" | cut -c1)
h5_result=$(echo "${verdicts[H5]}" | cut -c1)
h6_result=$(echo "${verdicts[H6]}" | cut -c1)
h7_result=$(echo "${verdicts[H7]}" | cut -c1)
h8_result=$(echo "${verdicts[H8]}" | cut -c1)

# Append to markdown table (before ## Hypothesis Definitions section)
if ! grep -q "^| $TODAY " "$verdicts_md"; then
    # Insert new row before first hypothesis section
    sed -i "/^## Hypothesis Definitions/i | $TODAY | $h1_result | $h2_result | $h3_result | $h4_result | $h5_result | $h6_result | $h7_result | $h8_result | $score |" "$verdicts_md"
    log_info "Appended verdict to: $verdicts_md"
fi

# ===== Print Results Table =====
log_section "Verdict Table"

echo ""
printf "%-4s %-60s %-10s\n" "ID" "Hypothesis" "Verdict"
printf "%s\n" "$(printf '=%.0s' {1..76})"

for h in H1 H2 H3 H4 H5 H6 H7 H8; do
    verdict="${verdicts[$h]}"
    if [[ "$verdict" == "PASS" ]]; then
        verdict_colored="${GREEN}PASS${NC}"
    else
        verdict_colored="${RED}FAIL${NC}"
    fi
    printf "%-4s %-60s %-10b\n" "$h" "${hypothesis_desc[$h]}" "$verdict_colored"
done

echo ""
echo -e "Final Score: ${BLUE}$score${NC}"
echo "Git Commit: $GIT_HASH"
echo "Recorded: $TIMESTAMP"
echo ""

log_info "Verdicts successfully recorded!"
exit 0
