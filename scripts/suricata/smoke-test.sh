#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# Suricata smoke test — validates binary features WITHOUT starting daemon
# Usage: ./scripts/suricata/smoke-test.sh [path-to-suricata-binary]

set -euo pipefail

SURICATA="${1:-$(which suricata 2>/dev/null || echo '')}"
EXIT_CODE=0

echo "=== Suricata Smoke Test ==="
echo "Binary: ${SURICATA:-NOT FOUND}"
echo ""

if [ -z "${SURICATA}" ] || [ ! -x "${SURICATA}" ]; then
    echo "FAIL: suricata binary not found or not executable"
    echo "Build from ~/tmp/suricata/ using scripts/suricata/build-suricata.sh"
    exit 1
fi

check() {
    local desc="$1"; shift
    if "$@" &>/dev/null; then
        echo "PASS: ${desc}"
    else
        echo "FAIL: ${desc}"
        EXIT_CODE=1
    fi
}

# Feature checks — all non-destructive
check "eBPF support" bash -c "${SURICATA} --build-info | grep -q 'eBPF support: yes'"
check "AF_PACKET support" bash -c "${SURICATA} --build-info | grep -q 'AF_PACKET support: yes'"
check "Unix socket support" bash -c "${SURICATA} --build-info | grep -q 'Unix socket support: yes'"
check "Lua support" bash -c "${SURICATA} --build-info | grep -q 'Lua support: yes'"
check "PCRE2 support" bash -c "${SURICATA} --build-info | grep -q 'PCRE2 support: yes'"

# Version check
VERSION=$("${SURICATA}" --build-info 2>/dev/null | grep "^Suricata version" | awk '{print $3}' || echo "unknown")
echo ""
echo "Version: ${VERSION}"
echo ""

if [ ${EXIT_CODE} -eq 0 ]; then
    echo "=== ALL CHECKS PASSED ==="
else
    echo "=== SOME CHECKS FAILED ==="
fi

exit ${EXIT_CODE}
