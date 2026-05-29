#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# attack-report.sh — Generate markdown attack reports combining:
#   - Prometheus metrics (service health, error rates, latency)
#   - Loki logs (campaign logs, system events)
#   - Cerydwyn analysis (AI-driven threat assessment)
#
# Usage:
#   ./attack-report.sh
#   ./attack-report.sh --campaign LICH-005
#   ./attack-report.sh --since "1 hour ago"
#   ./attack-report.sh --output-dir /opt/tomb/reports/

set -euo pipefail

# --- Configuration ---
PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
LOKI_URL="${LOKI_URL:-http://127.0.0.1:3100}"
CERYDWYN_ROOT="${CERYDWYN_ROOT:-/opt/tomb/cerydwyn}"
REPORTS_DIR="${REPORTS_DIR:-/opt/tomb/reports}"
CAMPAIGN=""
SINCE="1 hour ago"
OUTPUT_DIR=""

# --- Argument Parsing ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        --campaign)    CAMPAIGN="$2"; shift 2 ;;
        --since)       SINCE="$2"; shift 2 ;;
        --output-dir)  OUTPUT_DIR="$2"; shift 2 ;;
        --help|-h)
            cat <<'USAGE'
Usage: attack-report.sh [OPTIONS]

Options:
  --campaign NAME     Filter to a specific Lich campaign (e.g., LICH-005)
  --since TIME        Time window (default: "1 hour ago")
  --output-dir DIR    Output directory (default: /opt/tomb/reports/)
  --help, -h          Show this help

Examples:
  ./attack-report.sh
  ./attack-report.sh --campaign LICH-005 --since "2 hours ago"
  ./attack-report.sh --output-dir /tmp/reports/
USAGE
            exit 0
            ;;
        *) echo "[ERROR] Unknown option: $1" >&2; exit 1 ;;
    esac
done

# --- Helpers ---
timestamp() { date '+%Y-%m-%d %H:%M:%S'; }
epoch_now() { date '+%s'; }
epoch_since() { date -d "$SINCE" '+%s' 2>/dev/null || date -v-1H '+%s' 2>/dev/null || echo $(( $(epoch_now) - 3600 )); }

log_info() { echo "[$(timestamp)] [INFO] $*" >&2; }

prom_query() {
    local query="$1"
    curl -sf --max-time 10 "${PROMETHEUS_URL}/api/v1/query" \
        --data-urlencode "query=${query}" 2>/dev/null | \
        jq -r '.data.result[] | "\(.metric | to_entries | map("\(.key)=\(.value)") | join(", ")): \(.value[1])"' 2>/dev/null || echo "  (no data)"
}

prom_query_value() {
    local query="$1"
    curl -sf --max-time 10 "${PROMETHEUS_URL}/api/v1/query" \
        --data-urlencode "query=${query}" 2>/dev/null | \
        jq -r '.data.result[0].value[1] // "N/A"' 2>/dev/null || echo "N/A"
}

loki_query() {
    local logql="$1"
    local limit="${2:-20}"
    local start
    start=$(epoch_since)
    local end
    end=$(epoch_now)

    curl -sf --max-time 10 "${LOKI_URL}/loki/api/v1/query_range" \
        --data-urlencode "query=${logql}" \
        --data-urlencode "start=${start}" \
        --data-urlencode "end=${end}" \
        --data-urlencode "limit=${limit}" 2>/dev/null | \
        jq -r '.data.result[].values[][1]' 2>/dev/null | head -"${limit}" || echo "  (no logs)"
}

# --- Report Generation ---
generate_report() {
    local report_ts
    report_ts=$(date '+%Y-%m-%d-%H%M%S')

    local campaign_label=""
    if [[ -n "$CAMPAIGN" ]]; then
        campaign_label="${CAMPAIGN}-"
    fi

    if [[ -z "$OUTPUT_DIR" ]]; then
        OUTPUT_DIR="${REPORTS_DIR}/${campaign_label}${report_ts}"
    fi

    mkdir -p "$OUTPUT_DIR"

    local report_file="${OUTPUT_DIR}/attack-report.md"

    log_info "Generating attack report..."
    log_info "  Time window: ${SINCE} to now"
    log_info "  Campaign: ${CAMPAIGN:-all}"
    log_info "  Output: ${report_file}"

    # --- Build Report ---
    cat > "$report_file" <<HEADER
# Dark Mirror Attack Report

**Generated:** $(timestamp)
**Time Window:** ${SINCE} to $(timestamp)
**Campaign:** ${CAMPAIGN:-All campaigns}
**Report ID:** ${campaign_label}${report_ts}
**Source:** Tomb of Knowledge — Dark Mirror

---

## 1. Kingdom Service Status

### Service Availability

| Service | Status |
|---------|--------|
HEADER

    # Query service status
    local services
    services=$(curl -sf --max-time 10 "${PROMETHEUS_URL}/api/v1/query" \
        --data-urlencode 'query=up{job!="prometheus"}' 2>/dev/null | \
        jq -r '.data.result[] | "| \(.metric.job) | \(if .value[1] == "1" then "UP" else "**DOWN**" end) |"' 2>/dev/null || echo "| (query failed) | N/A |")

    echo "$services" >> "$report_file"

    cat >> "$report_file" <<SECTION

### Availability Summary

- **Total targets:** $(prom_query_value 'count(up{job!="prometheus"})')
- **Targets UP:** $(prom_query_value 'count(up{job!="prometheus"} == 1)')
- **Targets DOWN:** $(prom_query_value 'count(up{job!="prometheus"} == 0)')
- **Down percentage:** $(prom_query_value '(count(up{job!="prometheus"} == 0) / count(up{job!="prometheus"})) * 100')%

---

## 2. Request Volume & Error Rates

### Request Rates (last 5 minutes)

SECTION

    echo '```' >> "$report_file"
    prom_query 'sum by (service) (rate(unheaded_http_requests_total[5m]))' >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

### Error Rates (5xx by service, last 5 minutes)

SECTION

    echo '```' >> "$report_file"
    prom_query 'sum by (service) (rate(unheaded_http_requests_total{status=~"5.."}[5m]))' >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

### 4xx Rates (probing/scanning indicator)

SECTION

    echo '```' >> "$report_file"
    prom_query 'sum by (service) (rate(unheaded_http_requests_total{status=~"4.."}[5m]))' >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

---

## 3. Latency Analysis

### P99 Latency by Service

SECTION

    echo '```' >> "$report_file"
    prom_query 'histogram_quantile(0.99, sum by (service, le) (rate(unheaded_http_request_duration_seconds_bucket[5m])))' >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

### P50 Latency by Service

SECTION

    echo '```' >> "$report_file"
    prom_query 'histogram_quantile(0.50, sum by (service, le) (rate(unheaded_http_request_duration_seconds_bucket[5m])))' >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

---

## 4. Wotan Message Bus

- **Publish rate (msg/s):** $(prom_query_value 'sum(rate(unheaded_wotan_messages_published_total[5m]))')
- **Wotan status:** $(prom_query_value 'up{job="wotan-http"}' | sed 's/1/UP/;s/0/DOWN/')

### Messages by Service

SECTION

    echo '```' >> "$report_file"
    prom_query 'sum by (service) (rate(unheaded_wotan_messages_published_total[5m]))' >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

---

## 5. Firing Alerts

SECTION

    # Query active alerts
    local alerts
    alerts=$(curl -sf --max-time 10 "${PROMETHEUS_URL}/api/v1/alerts" 2>/dev/null | \
        jq -r '.data.alerts[] | select(.state=="firing") | "- **\(.labels.alertname)** [\(.labels.severity)] — \(.annotations.summary // "no summary")"' 2>/dev/null || echo "- (no alerts or Prometheus unreachable)")

    echo "$alerts" >> "$report_file"

    cat >> "$report_file" <<SECTION

---

## 6. Campaign Logs (Loki)

### Lich Campaign Activity

SECTION

    echo '```' >> "$report_file"
    loki_query '{job="lich"}' 30 >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

### Security Events (auth.log)

SECTION

    echo '```' >> "$report_file"
    loki_query '{job="syslog", component="auth"}' 15 >> "$report_file"
    echo '```' >> "$report_file"

    cat >> "$report_file" <<SECTION

---

## 7. Cerydwyn Analysis

SECTION

    # Query Cerydwyn for analysis if available
    if [[ -x "${CERYDWYN_ROOT}/cerydwyn.sh" ]]; then
        log_info "Querying Cerydwyn for threat assessment..."

        local cerydwyn_prompt="Based on the current Dark Mirror observations: "
        cerydwyn_prompt+="services down=$(prom_query_value 'count(up{job!="prometheus"} == 0)'), "
        cerydwyn_prompt+="error rate=$(prom_query_value '(sum(rate(unheaded_http_requests_total{status=~"5.."}[5m])) / sum(rate(unheaded_http_requests_total[5m]))) * 100')%, "
        cerydwyn_prompt+="total request rate=$(prom_query_value 'sum(rate(unheaded_http_requests_total[5m]))'). "

        if [[ -n "$CAMPAIGN" ]]; then
            cerydwyn_prompt+="Campaign: ${CAMPAIGN}. "
        fi

        cerydwyn_prompt+="Provide a brief threat assessment: what is the Kingdom's current security posture? Any anomalies?"

        local cerydwyn_response
        cerydwyn_response=$("${CERYDWYN_ROOT}/cerydwyn.sh" --no-stream "$cerydwyn_prompt" 2>/dev/null || echo "(Cerydwyn not available — Ollama may not be running)")

        echo "$cerydwyn_response" >> "$report_file"
    else
        echo "(Cerydwyn not available — cerydwyn.sh not found at ${CERYDWYN_ROOT}/cerydwyn.sh)" >> "$report_file"
    fi

    cat >> "$report_file" <<SECTION

---

## 8. Recommendations

Based on the data collected:

1. **Services Down:** Investigate any DOWN services immediately. Check Kingdom host and Tomb network path.
2. **Error Rates:** Any service with >5% 5xx rate needs investigation.
3. **Latency:** P99 >500ms indicates resource pressure or attack impact.
4. **Alerts:** Address firing alerts in severity order (PANIC > CRITICAL > ERROR > WARN).
5. **Wotan:** If Wotan is down, all inter-service communication is broken — highest priority.

---

*Report generated by the Dark Mirror — Tomb of Knowledge*
*"The Dark Mirror shows what the Kingdom cannot see about itself."*
SECTION

    log_info "Report generated: ${report_file}"
    echo ""
    echo "Report: ${report_file}"
    echo "Directory: ${OUTPUT_DIR}"

    # Create a summary JSON for machine parsing
    cat > "${OUTPUT_DIR}/summary.json" <<EOF
{
  "timestamp": "$(timestamp)",
  "campaign": "${CAMPAIGN:-all}",
  "time_window": "${SINCE}",
  "report_file": "${report_file}",
  "services_total": $(prom_query_value 'count(up{job!="prometheus"})'),
  "services_up": $(prom_query_value 'count(up{job!="prometheus"} == 1)'),
  "services_down": $(prom_query_value 'count(up{job!="prometheus"} == 0)')
}
EOF

    log_info "Summary JSON: ${OUTPUT_DIR}/summary.json"
}

# --- Main ---
main() {
    echo "============================================"
    echo "  Dark Mirror — Attack Report Generator"
    echo "============================================"
    echo ""

    # Verify Prometheus is reachable
    if ! curl -sf --max-time 5 "${PROMETHEUS_URL}/-/ready" &>/dev/null; then
        echo "[WARN] Prometheus not reachable at ${PROMETHEUS_URL}" >&2
        echo "[WARN] Report will contain incomplete data" >&2
    fi

    generate_report

    echo ""
    echo "Done. Review the report and share with the Kingdom."
}

main
