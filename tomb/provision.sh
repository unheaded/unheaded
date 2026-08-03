#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# tomb/provision.sh — Master provisioning script for the Tomb of Knowledge
#
# Deploys 5 layers into an air-gapped Kali Linux QEMU VM via SSH.
# All packages/binaries are pre-staged on the host and SCP'd in.
#
# USAGE:
#   ./provision.sh                           # Full provision (all phases)
#   ./provision.sh --phase setup-base        # Single phase
#   ./provision.sh --phase deploy-lich       # Single phase
#   ./provision.sh --target 192.168.13.6     # Override target IP
#   ./provision.sh --dry-run                 # Print commands, don't execute
#   ./provision.sh --list                    # List available phases
#
# PHASES (executed in order):
#   setup-base      — Base system: directories, packages, users, SSH keys
#   deploy-lich     — Layer 2: Lich adversary framework + LICH-001..010
#   deploy-grimoire — Layer 3: Grimoire knowledge base + RAG index
#   deploy-cerydwyn   — Layer 4: Cerydwyn LLM (Ollama + Mistral-7B) + RAG pipeline
#   deploy-dark-mirror — Layer 5: Dark Mirror observability (Prometheus, Grafana, Loki)
#   harden          — Phase 13: Security hardening (firewall, seccomp, audit)
#
# NETWORK:
#   Host (WEST):    192.168.13.2
#   Bridge:         192.168.13.5/30
#   Tomb VM:        192.168.13.6  (serial console, air-gapped)
#
# PREREQUISITES:
#   - SSH key deployed to Tomb VM (kali@192.168.13.6)
#   - Pre-staged packages in tomb/packages/ on the host
#   - QEMU VM booted and reachable over bridge
set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOMB_TARGET="${TOMB_TARGET:-192.168.13.6}"
TOMB_USER="${TOMB_USER:-kali}"
TOMB_SSH_PORT="${TOMB_SSH_PORT:-22}"
TOMB_SSH_KEY="${TOMB_SSH_KEY:-${HOME}/.ssh/id_tomb}"
HOST_IP="192.168.13.2"
BRIDGE_IP="192.168.13.5"
LOG_DIR="${SCRIPT_DIR}/logs"
PACKAGES_DIR="${SCRIPT_DIR}/packages"
ANSIBLE_DIR="${SCRIPT_DIR}/ansible"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
LOG_FILE="${LOG_DIR}/provision-${TIMESTAMP}.log"
MAX_RETRIES=3
RETRY_DELAY=5
SSH_TIMEOUT=10
DRY_RUN=0
PHASE=""
# shellcheck disable=SC2034  # parsed and advertised, but nothing reads it yet.
# --verbose is listed in --help ("Enable verbose output") and sets this at the
# arg-parsing case, so the flag is accepted and silently does nothing. Deleting
# the variable would be the wrong fix: the bug is the unimplemented behaviour,
# not the assignment. Either wire it up or drop it from --help — flagged for
# review rather than decided here.
VERBOSE=0

# Tomb VM paths
TOMB_ROOT="/opt/tomb"
TOMB_STAGING="/tmp/tomb-staging"

# ANSI colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; MAGENTA='\033[0;35m'; NC='\033[0m'

# ============================================================================
# LOGGING
# ============================================================================
mkdir -p "${LOG_DIR}"

_log() {
    local level="$1"; shift
    local msg="$*"
    local ts
    ts="$(date '+%Y-%m-%d %H:%M:%S')"
    printf '%s [%s] %s\n' "${ts}" "${level}" "${msg}" >> "${LOG_FILE}"
}

info()    { _log "INFO" "$*"; echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()      { _log "OK"   "$*"; echo -e "${GREEN}[  OK]${NC}  $*"; }
warn()    { _log "WARN" "$*"; echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()    { _log "FAIL" "$*"; echo -e "${RED}[FAIL]${NC}  $*"; }
header()  {
    local msg="$*"
    _log "PHASE" "${msg}"
    echo ""
    echo -e "${CYAN}================================================================${NC}"
    echo -e "${CYAN}  ${msg}${NC}"
    echo -e "${CYAN}================================================================${NC}"
    echo ""
}

die() { fail "$*"; exit 1; }

# ============================================================================
# SSH HELPERS
# ============================================================================
_ssh_opts() {
    echo "-o StrictHostKeyChecking=no \
          -o UserKnownHostsFile=/dev/null \
          -o ConnectTimeout=${SSH_TIMEOUT} \
          -o BatchMode=yes \
          -o LogLevel=ERROR \
          -p ${TOMB_SSH_PORT}"
}

# Execute command on Tomb VM via SSH with retry logic.
# Usage: tomb_exec "command string"
tomb_exec() {
    local cmd="$1"
    local attempt=1

    if [ "${DRY_RUN}" -eq 1 ]; then
        echo -e "${MAGENTA}[DRY-RUN]${NC} ssh ${TOMB_USER}@${TOMB_TARGET}: ${cmd}"
        return 0
    fi

    while [ "${attempt}" -le "${MAX_RETRIES}" ]; do
        _log "SSH" "Attempt ${attempt}/${MAX_RETRIES}: ${cmd}"
        # shellcheck disable=SC2046  # _ssh_opts emits a list of -o flags; must split
    if ssh $(_ssh_opts) -i "${TOMB_SSH_KEY}" \
               "${TOMB_USER}@${TOMB_TARGET}" "${cmd}" 2>>"${LOG_FILE}"; then
            return 0
        fi
        warn "SSH command failed (attempt ${attempt}/${MAX_RETRIES}), retrying in ${RETRY_DELAY}s..."
        sleep "${RETRY_DELAY}"
        attempt=$((attempt + 1))
    done

    fail "SSH command failed after ${MAX_RETRIES} attempts: ${cmd}"
    return 1
}

# Execute command as root on Tomb VM.
# Usage: tomb_root "command string"
tomb_root() {
    tomb_exec "sudo -n $1"
}

# SCP file(s) to Tomb VM with retry logic.
# Usage: tomb_scp <local_path> <remote_path>
tomb_scp() {
    local src="$1"
    local dst="$2"
    local attempt=1

    if [ "${DRY_RUN}" -eq 1 ]; then
        echo -e "${MAGENTA}[DRY-RUN]${NC} scp ${src} -> ${TOMB_USER}@${TOMB_TARGET}:${dst}"
        return 0
    fi

    while [ "${attempt}" -le "${MAX_RETRIES}" ]; do
        _log "SCP" "Attempt ${attempt}/${MAX_RETRIES}: ${src} -> ${dst}"
        # shellcheck disable=SC2046  # _ssh_opts emits a list of -o flags; must split
    if scp $(_ssh_opts) -i "${TOMB_SSH_KEY}" \
               -r "${src}" "${TOMB_USER}@${TOMB_TARGET}:${dst}" 2>>"${LOG_FILE}"; then
            return 0
        fi
        warn "SCP failed (attempt ${attempt}/${MAX_RETRIES}), retrying in ${RETRY_DELAY}s..."
        sleep "${RETRY_DELAY}"
        attempt=$((attempt + 1))
    done

    fail "SCP failed after ${MAX_RETRIES} attempts: ${src} -> ${dst}"
    return 1
}

# ============================================================================
# VERIFICATION HELPERS
# ============================================================================

# Verify SSH connectivity to Tomb VM.
verify_ssh() {
    info "Verifying SSH connectivity to ${TOMB_TARGET}..."
    if tomb_exec "echo 'tomb-alive'" >/dev/null 2>&1; then
        ok "SSH connection to ${TOMB_TARGET} successful"
        return 0
    else
        fail "Cannot reach ${TOMB_TARGET} via SSH"
        return 1
    fi
}

# Verify air-gap: Tomb must NOT have internet access.
verify_airgap() {
    info "Verifying air-gap enforcement..."
    if tomb_exec "ping -c 1 -W 2 8.8.8.8 2>/dev/null" >/dev/null 2>&1; then
        fail "AIR-GAP VIOLATION: Tomb can reach 8.8.8.8 -- aborting"
        die "Air-gap integrity check failed. Fix network isolation before provisioning."
    fi
    if tomb_exec "ping -c 1 -W 2 1.1.1.1 2>/dev/null" >/dev/null 2>&1; then
        fail "AIR-GAP VIOLATION: Tomb can reach 1.1.1.1 -- aborting"
        die "Air-gap integrity check failed. Fix network isolation before provisioning."
    fi
    ok "Air-gap verified: no internet routes"
}

# Verify bridge connectivity: Tomb must reach the host.
verify_bridge() {
    info "Verifying bridge connectivity to host (${HOST_IP})..."
    if tomb_exec "ping -c 2 -W 3 ${HOST_IP}" >/dev/null 2>&1; then
        ok "Bridge connectivity confirmed (${TOMB_TARGET} -> ${HOST_IP})"
    else
        warn "Cannot reach host ${HOST_IP} from Tomb -- bridge may not be configured"
    fi
}

# Verify a specific service is running inside the Tomb.
# Usage: verify_service <service_name> [port]
verify_service() {
    local svc_name="$1"
    local port="${2:-}"

    if [ -n "${port}" ]; then
        if tomb_exec "ss -tlnp 2>/dev/null | grep -q ':${port} '" 2>/dev/null; then
            ok "${svc_name} listening on port ${port}"
            return 0
        else
            fail "${svc_name} NOT listening on port ${port}"
            return 1
        fi
    else
        if tomb_exec "systemctl is-active ${svc_name} 2>/dev/null" >/dev/null 2>&1; then
            ok "${svc_name} is active"
            return 0
        else
            fail "${svc_name} is NOT active"
            return 1
        fi
    fi
}

# Verify a directory exists inside the Tomb.
# Usage: verify_dir <path>
verify_dir() {
    local path="$1"
    if tomb_exec "test -d ${path}" 2>/dev/null; then
        ok "Directory exists: ${path}"
        return 0
    else
        fail "Directory missing: ${path}"
        return 1
    fi
}

# ============================================================================
# PHASE: PRE-FLIGHT
# ============================================================================
phase_preflight() {
    header "PRE-FLIGHT CHECKS"

    # Local checks
    info "Checking local prerequisites..."
    command -v ssh >/dev/null 2>&1 || die "ssh not found"
    command -v scp >/dev/null 2>&1 || die "scp not found"
    ok "SSH tools available"

    if [ -f "${TOMB_SSH_KEY}" ]; then
        ok "SSH key found: ${TOMB_SSH_KEY}"
    else
        warn "SSH key not found: ${TOMB_SSH_KEY}"
        info "Generate with: ssh-keygen -t ed25519 -f ${TOMB_SSH_KEY} -C 'tomb-provisioner'"
    fi

    if [ -d "${PACKAGES_DIR}" ]; then
        local pkg_count
        pkg_count=$(find "${PACKAGES_DIR}" -type f 2>/dev/null | wc -l)
        ok "Packages directory: ${PACKAGES_DIR} (${pkg_count} files)"
    else
        warn "Packages directory missing: ${PACKAGES_DIR}"
        info "Pre-stage all .deb packages and binaries here for air-gapped install"
    fi

    if [ -d "${ANSIBLE_DIR}" ]; then
        ok "Ansible directory: ${ANSIBLE_DIR}"
    else
        warn "Ansible directory missing: ${ANSIBLE_DIR}"
    fi

    # Remote checks
    verify_ssh || die "Cannot proceed without SSH access"
    verify_airgap
    verify_bridge

    # Tomb system info
    info "Gathering Tomb VM system info..."
    tomb_exec "uname -a" 2>/dev/null || true
    tomb_exec "cat /etc/os-release 2>/dev/null | head -3" 2>/dev/null || true
    tomb_exec "free -h 2>/dev/null | head -2" 2>/dev/null || true
    tomb_exec "df -h / 2>/dev/null | tail -1" 2>/dev/null || true

    ok "Pre-flight complete"
}

# ============================================================================
# PHASE: SETUP-BASE
# ============================================================================
phase_setup_base() {
    header "PHASE 1: SETUP BASE SYSTEM"

    info "Creating Tomb directory structure..."
    tomb_root "mkdir -p ${TOMB_ROOT}/{lich/{campaigns,harnesses,crash-triage/{corpus,crashes/{rce,dos,memory-leak,info-leak,unknown}}},grimoire/{kingdom-docs,mitre-attck,nvd-cves,cwe,threat-models,history,rag/{chroma.db,embeddings}},cerydwyn/{ollama/models,logs,suggestions},dark-mirror/{prometheus,grafana/{dashboards,datasources},loki,alerts},captures/{pcaps,packet-analysis},reports/archive}"
    tomb_root "mkdir -p ${TOMB_STAGING}"
    ok "Directory structure created"

    info "Setting ownership and permissions..."
    tomb_root "chown -R ${TOMB_USER}:${TOMB_USER} ${TOMB_ROOT}"
    tomb_root "chmod 750 ${TOMB_ROOT}"
    tomb_root "chmod 700 ${TOMB_ROOT}/cerydwyn"
    tomb_root "chmod 700 ${TOMB_ROOT}/reports"
    ok "Permissions set"

    info "Transferring pre-staged packages..."
    if [ -d "${PACKAGES_DIR}/debs" ]; then
        tomb_root "mkdir -p ${TOMB_STAGING}/debs"
        tomb_scp "${PACKAGES_DIR}/debs/" "${TOMB_STAGING}/debs/"
        info "Installing .deb packages (offline)..."
        tomb_root "dpkg -i ${TOMB_STAGING}/debs/*.deb 2>/dev/null || apt-get -f install -y --allow-unauthenticated 2>/dev/null || true"
        ok "Package installation attempted"
    else
        warn "No debs/ directory in ${PACKAGES_DIR} -- skipping offline package install"
    fi

    info "Installing essential tools from Kali base..."
    # These should already be present on Kali, but verify
    local kali_tools="nmap nikto sqlmap metasploit-framework hydra john hashcat \
                      aircrack-ng wireshark-common ghidra radare2 gdb \
                      python3 python3-pip python3-venv jq curl wget socat tmux"
    for tool in ${kali_tools}; do
        if tomb_exec "dpkg -l ${tool} 2>/dev/null | grep -q '^ii'" 2>/dev/null; then
            ok "  ${tool}: installed"
        else
            warn "  ${tool}: NOT installed (stage .deb in packages/debs/)"
        fi
    done

    info "Creating scope.conf..."
    tomb_scp "${SCRIPT_DIR}/scripts/scope.conf" "${TOMB_STAGING}/scope.conf" 2>/dev/null || {
        # Generate default scope.conf if not pre-staged
        tomb_exec "cat > ${TOMB_STAGING}/scope.conf << 'SCOPE_EOF'
# scope.conf -- Tomb of Knowledge Attack Scope
# Updated: $(date +%Y-%m-%d)

metadata:
  kingdom_id: \"westmarch-core\"
  kingdom_network: \"192.168.13.0/30\"
  approval_date: \"$(date +%Y-%m-%d)\"
  approval_authority: \"Chief Architect\"

in_scope:
  networks:
    - \"192.168.13.0/30\"
  attack_types:
    - \"network_reconnaissance\"
    - \"vulnerability_scanning\"
    - \"fuzzing\"

out_of_scope:
  networks:
    - \"0.0.0.0/0\"
  attack_types:
    - \"denial_of_service\"
    - \"data_destruction\"

rules_of_engagement:
  max_connections_per_target: 1000
  max_requests_per_second: 50
  alert_on_scope_violation: true
  auto_abort_on_scope_violation: true
SCOPE_EOF"
    }
    tomb_root "cp ${TOMB_STAGING}/scope.conf ${TOMB_ROOT}/scope.conf"
    tomb_root "chmod 640 ${TOMB_ROOT}/scope.conf"
    ok "scope.conf deployed"

    # Verification
    info "Verifying base setup..."
    verify_dir "${TOMB_ROOT}/lich"
    verify_dir "${TOMB_ROOT}/grimoire"
    verify_dir "${TOMB_ROOT}/cerydwyn"
    verify_dir "${TOMB_ROOT}/dark-mirror"
    verify_dir "${TOMB_ROOT}/captures"
    verify_dir "${TOMB_ROOT}/reports"

    ok "Base system setup complete"
}

# ============================================================================
# PHASE: DEPLOY-LICH
# ============================================================================
phase_deploy_lich() {
    header "PHASE 2: DEPLOY LICH ADVERSARY FRAMEWORK"

    info "Deploying Lich campaign definitions (LICH-001 through LICH-010)..."
    for i in $(seq -w 1 10); do
        local campaign="LICH-0${i}"
        tomb_root "mkdir -p ${TOMB_ROOT}/lich/campaigns/${campaign}/{config,results,evidence}"
        ok "  ${campaign} directory created"
    done

    info "Transferring Lich harnesses..."
    if [ -d "${PACKAGES_DIR}/lich" ]; then
        tomb_scp "${PACKAGES_DIR}/lich/" "${TOMB_STAGING}/lich/"
        tomb_root "cp -r ${TOMB_STAGING}/lich/* ${TOMB_ROOT}/lich/"
        ok "Lich harnesses transferred"
    else
        warn "No lich/ in packages/ -- deploying stub harnesses"
    fi

    info "Deploying operational scripts..."

    # lich-runner.sh
    tomb_exec "cat > ${TOMB_STAGING}/lich-runner.sh << 'LICH_RUNNER_EOF'
#!/bin/sh
# SPDX-License-Identifier: MIT
# lich-runner.sh -- Execute Lich campaigns
set -eu
TOMB_ROOT=\"/opt/tomb\"
SCOPE=\"\${TOMB_ROOT}/scope.conf\"
CAMPAIGNS=\"\${1:-all}\"
PARALLEL=0
DRY_RUN=0

usage() {
    echo \"Usage: lich-runner.sh [--campaign LICH-001,...] [--parallel] [--dry-run]\"
    exit 1
}

while [ \$# -gt 0 ]; do
    case \"\$1\" in
        --campaign) CAMPAIGNS=\"\$2\"; shift 2 ;;
        --parallel) PARALLEL=1; shift ;;
        --dry-run)  DRY_RUN=1; shift ;;
        -h|--help)  usage ;;
        *)          CAMPAIGNS=\"\$1\"; shift ;;
    esac
done

if [ ! -f \"\${SCOPE}\" ]; then
    echo \"[FAIL] scope.conf not found at \${SCOPE}\"
    exit 1
fi

echo \"[INFO] Scope file: \${SCOPE}\"
echo \"[INFO] Campaigns: \${CAMPAIGNS}\"
echo \"[INFO] Parallel: \${PARALLEL}\"

if [ \"\${CAMPAIGNS}\" = \"all\" ]; then
    CAMPAIGNS=\"LICH-001,LICH-002,LICH-003,LICH-004,LICH-005,LICH-006,LICH-007,LICH-008,LICH-009,LICH-010\"
fi

IFS=','
for campaign in \${CAMPAIGNS}; do
    campaign_dir=\"\${TOMB_ROOT}/lich/campaigns/\${campaign}\"
    if [ ! -d \"\${campaign_dir}\" ]; then
        echo \"[WARN] Campaign directory not found: \${campaign_dir}\"
        continue
    fi
    echo \"\"
    echo \"================================================================\"
    echo \"  Executing: \${campaign}\"
    echo \"================================================================\"
    ts=\$(date +%Y%m%d-%H%M%S)
    result_dir=\"\${campaign_dir}/results/\${ts}\"
    mkdir -p \"\${result_dir}\"

    if [ \"\${DRY_RUN}\" -eq 1 ]; then
        echo \"[DRY-RUN] Would execute \${campaign}\"
        continue
    fi

    # Run attack-preflight
    if [ -x \"\${TOMB_ROOT}/lich/attack-preflight.sh\" ]; then
        if ! \"\${TOMB_ROOT}/lich/attack-preflight.sh\" --campaign \"\${campaign}\"; then
            echo \"[FAIL] Pre-flight failed for \${campaign} -- skipping\"
            echo \"PREFLIGHT_FAILED\" > \"\${result_dir}/status\"
            continue
        fi
    fi

    # Execute campaign harness if present
    harness=\"\${campaign_dir}/config/run.sh\"
    if [ -x \"\${harness}\" ]; then
        echo \"[INFO] Running harness: \${harness}\"
        if \"\${harness}\" > \"\${result_dir}/output.log\" 2>&1; then
            echo \"COMPLETED\" > \"\${result_dir}/status\"
            echo \"[  OK] \${campaign} completed\"
        else
            echo \"FAILED\" > \"\${result_dir}/status\"
            echo \"[FAIL] \${campaign} failed (see \${result_dir}/output.log)\"
        fi
    else
        echo \"[WARN] No harness found at \${harness}\"
        echo \"SKIPPED_NO_HARNESS\" > \"\${result_dir}/status\"
    fi
done
echo \"\"
echo \"[INFO] All campaigns processed.\"
LICH_RUNNER_EOF"
    tomb_root "cp ${TOMB_STAGING}/lich-runner.sh ${TOMB_ROOT}/lich/lich-runner.sh"
    tomb_root "chmod 755 ${TOMB_ROOT}/lich/lich-runner.sh"
    ok "lich-runner.sh deployed"

    # crash-triage.sh
    tomb_exec "cat > ${TOMB_STAGING}/crash-triage.sh << 'TRIAGE_EOF'
#!/bin/sh
# SPDX-License-Identifier: MIT
# crash-triage.sh -- Categorize and analyze fuzzing crashes
set -eu
TOMB_ROOT=\"/opt/tomb\"
CORPUS=\"\${1:-\${TOMB_ROOT}/lich/crash-triage/corpus}\"
OUTPUT=\"\${2:-\${TOMB_ROOT}/lich/crash-triage/crashes}\"
CATEGORIES=\"rce dos memory-leak info-leak unknown\"

echo \"[INFO] Crash triage starting...\"
echo \"[INFO] Corpus: \${CORPUS}\"
echo \"[INFO] Output: \${OUTPUT}\"

for cat in \${CATEGORIES}; do
    mkdir -p \"\${OUTPUT}/\${cat}\"
done

crash_count=0
if [ -d \"\${CORPUS}\" ]; then
    for crash_file in \"\${CORPUS}\"/*; do
        [ -f \"\${crash_file}\" ] || continue
        crash_count=\$((crash_count + 1))
        fname=\$(basename \"\${crash_file}\")
        echo \"[INFO] Analyzing: \${fname}\"

        # Basic classification by crash signature
        # In production this would invoke gdb, valgrind, ASAN output parsing
        category=\"unknown\"
        if file \"\${crash_file}\" 2>/dev/null | grep -qi \"ELF\\|executable\"; then
            category=\"rce\"
        elif strings \"\${crash_file}\" 2>/dev/null | grep -qi \"stack\\|overflow\\|heap\"; then
            category=\"memory-leak\"
        elif strings \"\${crash_file}\" 2>/dev/null | grep -qi \"timeout\\|hang\\|loop\"; then
            category=\"dos\"
        fi

        cp \"\${crash_file}\" \"\${OUTPUT}/\${category}/\${fname}\"
        echo \"  -> Classified as: \${category}\"
    done
fi

echo \"\"
echo \"[INFO] Triage complete: \${crash_count} crashes processed\"
for cat in \${CATEGORIES}; do
    count=\$(find \"\${OUTPUT}/\${cat}\" -type f 2>/dev/null | wc -l)
    echo \"  \${cat}: \${count}\"
done
TRIAGE_EOF"
    tomb_root "cp ${TOMB_STAGING}/crash-triage.sh ${TOMB_ROOT}/lich/crash-triage.sh"
    tomb_root "chmod 755 ${TOMB_ROOT}/lich/crash-triage.sh"
    ok "crash-triage.sh deployed"

    # attack-preflight.sh
    tomb_exec "cat > ${TOMB_STAGING}/attack-preflight.sh << 'PREFLIGHT_EOF'
#!/bin/sh
# SPDX-License-Identifier: MIT
# attack-preflight.sh -- Safety checks before campaign execution
set -eu
TOMB_ROOT=\"/opt/tomb\"
SCOPE=\"\${TOMB_ROOT}/scope.conf\"
CAMPAIGN=\"\${2:-unknown}\"
PASS=0
TOTAL=0

check() {
    TOTAL=\$((TOTAL + 1))
    if eval \"\$2\" >/dev/null 2>&1; then
        echo \"[  OK] \$1\"
        PASS=\$((PASS + 1))
    else
        echo \"[FAIL] \$1\"
    fi
}

echo \"[INFO] Pre-flight checks for: \${CAMPAIGN}\"
echo \"\"

check \"Scope file exists\" \"test -f \${SCOPE}\"
check \"Air-gap (no internet)\" \"! ping -c 1 -W 2 8.8.8.8\"
check \"Air-gap (no DNS)\" \"! ping -c 1 -W 2 1.1.1.1\"
check \"Persistence disk space (>1GB free)\" \"test \$(df -BG /opt/tomb | awk 'NR==2{print \$4}' | tr -d G) -ge 1\"
check \"Kingdom reachable\" \"ping -c 1 -W 3 192.168.13.2\"
check \"Dark Mirror logging\" \"test -d \${TOMB_ROOT}/dark-mirror/loki\"

echo \"\"
echo \"[INFO] Pre-flight: \${PASS}/\${TOTAL} checks passed\"

if [ \"\${PASS}\" -lt \"\${TOTAL}\" ]; then
    echo \"[FAIL] Not all checks passed -- campaign execution blocked\"
    exit 1
fi
echo \"[  OK] All checks passed -- safe to execute\"
exit 0
PREFLIGHT_EOF"
    tomb_root "cp ${TOMB_STAGING}/attack-preflight.sh ${TOMB_ROOT}/lich/attack-preflight.sh"
    tomb_root "chmod 755 ${TOMB_ROOT}/lich/attack-preflight.sh"
    ok "attack-preflight.sh deployed"

    # attack-report.sh
    tomb_exec "cat > ${TOMB_STAGING}/attack-report.sh << 'REPORT_EOF'
#!/bin/sh
# SPDX-License-Identifier: MIT
# attack-report.sh -- Generate post-attack report
set -eu
TOMB_ROOT=\"/opt/tomb\"
CAMPAIGN=\"\${2:-unknown}\"
TS=\$(date +%Y-%m-%d-%H%M%S)
REPORT_DIR=\"\${TOMB_ROOT}/reports/\${CAMPAIGN}-\${TS}\"

mkdir -p \"\${REPORT_DIR}/evidence\"/{pcaps,crash-details,logs}

echo \"[INFO] Generating attack report: \${CAMPAIGN}\"
echo \"[INFO] Output: \${REPORT_DIR}\"

# Collect evidence
if [ -d \"\${TOMB_ROOT}/captures/pcaps\" ]; then
    cp \"\${TOMB_ROOT}/captures/pcaps/\"*\"\${CAMPAIGN}\"* \"\${REPORT_DIR}/evidence/pcaps/\" 2>/dev/null || true
fi

# Collect crash data
campaign_dir=\"\${TOMB_ROOT}/lich/campaigns/\${CAMPAIGN}\"
if [ -d \"\${campaign_dir}/results\" ]; then
    latest=\$(ls -1t \"\${campaign_dir}/results/\" 2>/dev/null | head -1)
    if [ -n \"\${latest}\" ] && [ -d \"\${campaign_dir}/results/\${latest}\" ]; then
        cp -r \"\${campaign_dir}/results/\${latest}\" \"\${REPORT_DIR}/evidence/logs/\"
    fi
fi

# Generate findings.json stub
cat > \"\${REPORT_DIR}/findings.json\" << FINDINGS
{
  \"campaign\": \"\${CAMPAIGN}\",
  \"timestamp\": \"\${TS}\",
  \"findings\": [],
  \"summary\": {
    \"critical\": 0,
    \"high\": 0,
    \"medium\": 0,
    \"low\": 0,
    \"info\": 0
  }
}
FINDINGS

# Generate summary
cat > \"\${REPORT_DIR}/summary.txt\" << SUMMARY
Tomb of Knowledge -- Attack Report
====================================
Campaign:    \${CAMPAIGN}
Date:        \${TS}
Status:      COMPLETED
Scope:       See \${TOMB_ROOT}/scope.conf

Findings Summary:
  CRITICAL: 0
  HIGH:     0
  MEDIUM:   0
  LOW:      0
  INFO:     0

Evidence:
  PCAPs:         \${REPORT_DIR}/evidence/pcaps/
  Crash Details: \${REPORT_DIR}/evidence/crash-details/
  Logs:          \${REPORT_DIR}/evidence/logs/

Next Steps:
  1. Review findings.json for exploitability assessment
  2. Verify crashes in crash-details/ are reproducible
  3. Prioritize remediation by severity
  4. Schedule re-test after fixes applied
SUMMARY

echo \"[  OK] Report generated: \${REPORT_DIR}\"
echo \"[INFO] Files:\"
find \"\${REPORT_DIR}\" -type f | while read -r f; do
    echo \"  \${f}\"
done
REPORT_EOF"
    tomb_root "cp ${TOMB_STAGING}/attack-report.sh ${TOMB_ROOT}/lich/attack-report.sh"
    tomb_root "chmod 755 ${TOMB_ROOT}/lich/attack-report.sh"
    ok "attack-report.sh deployed"

    # Verification
    info "Verifying Lich deployment..."
    verify_dir "${TOMB_ROOT}/lich/campaigns/LICH-001"
    verify_dir "${TOMB_ROOT}/lich/campaigns/LICH-010"
    verify_dir "${TOMB_ROOT}/lich/crash-triage/corpus"
    tomb_exec "test -x ${TOMB_ROOT}/lich/lich-runner.sh" && ok "lich-runner.sh executable" || fail "lich-runner.sh not executable"
    tomb_exec "test -x ${TOMB_ROOT}/lich/crash-triage.sh" && ok "crash-triage.sh executable" || fail "crash-triage.sh not executable"
    tomb_exec "test -x ${TOMB_ROOT}/lich/attack-preflight.sh" && ok "attack-preflight.sh executable" || fail "attack-preflight.sh not executable"
    tomb_exec "test -x ${TOMB_ROOT}/lich/attack-report.sh" && ok "attack-report.sh executable" || fail "attack-report.sh not executable"

    ok "Lich adversary framework deployed"
}

# ============================================================================
# PHASE: DEPLOY-GRIMOIRE
# ============================================================================
phase_deploy_grimoire() {
    header "PHASE 3: DEPLOY GRIMOIRE KNOWLEDGE BASE"

    info "Transferring Kingdom documentation..."
    if [ -d "${PACKAGES_DIR}/grimoire/kingdom-docs" ]; then
        tomb_scp "${PACKAGES_DIR}/grimoire/kingdom-docs/" "${TOMB_STAGING}/kingdom-docs/"
        tomb_root "cp -r ${TOMB_STAGING}/kingdom-docs/* ${TOMB_ROOT}/grimoire/kingdom-docs/"
        ok "Kingdom docs transferred"
    else
        warn "No kingdom-docs in packages/grimoire/ -- stage docs from the repo"
        info "Hint: cp -r ~/tmp/unheaded/docs/ tomb/packages/grimoire/kingdom-docs/"
    fi

    info "Transferring MITRE ATT&CK data..."
    if [ -d "${PACKAGES_DIR}/grimoire/mitre-attck" ]; then
        tomb_scp "${PACKAGES_DIR}/grimoire/mitre-attck/" "${TOMB_STAGING}/mitre-attck/"
        tomb_root "cp -r ${TOMB_STAGING}/mitre-attck/* ${TOMB_ROOT}/grimoire/mitre-attck/"
        ok "MITRE ATT&CK data transferred"
    else
        warn "No mitre-attck/ in packages/grimoire/ -- download from github.com/mitre/cti"
    fi

    info "Transferring NVD CVE records..."
    if [ -d "${PACKAGES_DIR}/grimoire/nvd-cves" ]; then
        tomb_scp "${PACKAGES_DIR}/grimoire/nvd-cves/" "${TOMB_STAGING}/nvd-cves/"
        tomb_root "cp -r ${TOMB_STAGING}/nvd-cves/* ${TOMB_ROOT}/grimoire/nvd-cves/"
        ok "NVD CVE records transferred"
    else
        warn "No nvd-cves/ in packages/grimoire/ -- download from nvd.nist.gov/feeds"
    fi

    info "Transferring CWE mappings..."
    if [ -d "${PACKAGES_DIR}/grimoire/cwe" ]; then
        tomb_scp "${PACKAGES_DIR}/grimoire/cwe/" "${TOMB_STAGING}/cwe/"
        tomb_root "cp -r ${TOMB_STAGING}/cwe/* ${TOMB_ROOT}/grimoire/cwe/"
        ok "CWE mappings transferred"
    else
        warn "No cwe/ in packages/grimoire/ -- download from cwe.mitre.org"
    fi

    info "Deploying grimoire-search.sh..."
    tomb_exec "cat > ${TOMB_STAGING}/grimoire-search.sh << 'GSEARCH_EOF'
#!/bin/sh
# SPDX-License-Identifier: MIT
# grimoire-search.sh -- Full-text search across Grimoire knowledge base
set -eu
TOMB_ROOT=\"/opt/tomb\"
GRIMOIRE=\"\${TOMB_ROOT}/grimoire\"
QUERY=\"\${1:-}\"
MODE=\"fulltext\"

usage() {
    echo \"Usage: grimoire-search.sh [OPTIONS] <query>\"
    echo \"  --cve <CVE-ID>         Search NVD CVE records\"
    echo \"  --threat-model <term>  Search threat models\"
    echo \"  --kingdom <term>       Search Kingdom docs only\"
    echo \"  <query>                Full-text search all sources\"
    exit 1
}

if [ -z \"\${QUERY}\" ]; then usage; fi

while [ \$# -gt 0 ]; do
    case \"\$1\" in
        --cve)          MODE=\"cve\"; QUERY=\"\$2\"; shift 2 ;;
        --threat-model) MODE=\"threat\"; QUERY=\"\$2\"; shift 2 ;;
        --kingdom)      MODE=\"kingdom\"; QUERY=\"\$2\"; shift 2 ;;
        -h|--help)      usage ;;
        *)              QUERY=\"\$1\"; shift ;;
    esac
done

echo \"Grimoire Search: \\\"\${QUERY}\\\" (mode: \${MODE})\"
echo \"================================================================\"

case \"\${MODE}\" in
    cve)
        echo \"Searching NVD CVE records...\"
        grep -ril \"\${QUERY}\" \"\${GRIMOIRE}/nvd-cves/\" 2>/dev/null || echo \"  No matches.\"
        ;;
    threat)
        echo \"Searching threat models...\"
        grep -ril \"\${QUERY}\" \"\${GRIMOIRE}/threat-models/\" 2>/dev/null || echo \"  No matches.\"
        ;;
    kingdom)
        echo \"Searching Kingdom docs...\"
        grep -ril \"\${QUERY}\" \"\${GRIMOIRE}/kingdom-docs/\" 2>/dev/null || echo \"  No matches.\"
        ;;
    fulltext)
        echo \"Full-text search across all sources...\"
        for src in kingdom-docs mitre-attck nvd-cves cwe threat-models history; do
            results=\$(grep -ril \"\${QUERY}\" \"\${GRIMOIRE}/\${src}/\" 2>/dev/null | wc -l)
            if [ \"\${results}\" -gt 0 ]; then
                echo \"\"
                echo \"=== \${src} (\${results} matches) ===\"
                grep -ril \"\${QUERY}\" \"\${GRIMOIRE}/\${src}/\" 2>/dev/null | head -10
                if [ \"\${results}\" -gt 10 ]; then
                    echo \"  ... and \$((results - 10)) more\"
                fi
            fi
        done
        ;;
esac
GSEARCH_EOF"
    tomb_root "cp ${TOMB_STAGING}/grimoire-search.sh ${TOMB_ROOT}/grimoire/grimoire-search.sh"
    tomb_root "chmod 755 ${TOMB_ROOT}/grimoire/grimoire-search.sh"
    ok "grimoire-search.sh deployed"

    info "Deploying RAG infrastructure dependencies..."
    if [ -d "${PACKAGES_DIR}/grimoire/rag-deps" ]; then
        tomb_scp "${PACKAGES_DIR}/grimoire/rag-deps/" "${TOMB_STAGING}/rag-deps/"
        tomb_root "pip3 install --no-index --find-links=${TOMB_STAGING}/rag-deps/ chromadb sentence-transformers 2>/dev/null || true"
        ok "RAG Python dependencies installed (offline)"
    else
        warn "No rag-deps/ in packages/grimoire/ -- stage pip wheels for air-gapped install"
        info "Hint: pip download -d tomb/packages/grimoire/rag-deps/ chromadb sentence-transformers"
    fi

    info "Deploying rag-query.py..."
    tomb_exec "cat > ${TOMB_STAGING}/rag-query.py << 'RAG_QUERY_EOF'
#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# rag-query.py -- Retrieve documents from ChromaDB by semantic similarity
\"\"\"
RAG query interface for the Grimoire knowledge base.
Uses ChromaDB + sentence-transformers for vector similarity search.
\"\"\"
import argparse
import json
import os
import sys

TOMB_ROOT = \"/opt/tomb\"
CHROMA_DIR = os.path.join(TOMB_ROOT, \"grimoire\", \"rag\", \"chroma.db\")
RAG_CONFIG = os.path.join(TOMB_ROOT, \"cerydwyn\", \"rag-config.json\")
DEFAULT_TOP_K = 5
DEFAULT_MODEL = \"all-MiniLM-L6-v2\"


def load_config():
    \"\"\"Load RAG pipeline configuration.\"\"\"
    if os.path.exists(RAG_CONFIG):
        with open(RAG_CONFIG, \"r\") as f:
            return json.load(f)
    return {
        \"embedding_model\": DEFAULT_MODEL,
        \"top_k\": DEFAULT_TOP_K,
        \"chroma_dir\": CHROMA_DIR,
        \"collection_name\": \"grimoire\",
    }


def query_chromadb(query_text, top_k=None, config=None):
    \"\"\"Query ChromaDB for semantically similar documents.\"\"\"
    if config is None:
        config = load_config()
    if top_k is None:
        top_k = config.get(\"top_k\", DEFAULT_TOP_K)

    try:
        import chromadb
        from chromadb.config import Settings
    except ImportError:
        print(\"[ERROR] chromadb not installed. Install with:\", file=sys.stderr)
        print(\"  pip3 install chromadb sentence-transformers\", file=sys.stderr)
        sys.exit(1)

    chroma_dir = config.get(\"chroma_dir\", CHROMA_DIR)
    collection_name = config.get(\"collection_name\", \"grimoire\")

    if not os.path.exists(chroma_dir):
        print(f\"[WARN] ChromaDB directory not found: {chroma_dir}\", file=sys.stderr)
        print(\"[WARN] Run grimoire-index.py to build the vector index.\", file=sys.stderr)
        return []

    client = chromadb.Client(Settings(
        chroma_db_impl=\"duckdb+parquet\",
        persist_directory=chroma_dir,
        anonymized_telemetry=False,
    ))

    try:
        collection = client.get_collection(name=collection_name)
    except Exception:
        print(f\"[WARN] Collection '{collection_name}' not found.\", file=sys.stderr)
        return []

    results = collection.query(query_texts=[query_text], n_results=top_k)

    formatted = []
    if results and results.get(\"documents\"):
        for i, doc in enumerate(results[\"documents\"][0]):
            meta = results[\"metadatas\"][0][i] if results.get(\"metadatas\") else {}
            dist = results[\"distances\"][0][i] if results.get(\"distances\") else 0.0
            similarity = max(0.0, 1.0 - dist)
            formatted.append({
                \"rank\": i + 1,
                \"similarity\": round(similarity, 4),
                \"source\": meta.get(\"source\", \"unknown\"),
                \"chunk\": doc[:500],
            })
    return formatted


def main():
    parser = argparse.ArgumentParser(description=\"Query Grimoire via RAG\")
    parser.add_argument(\"query\", help=\"Search query\")
    parser.add_argument(\"--top-k\", type=int, default=None, help=\"Number of results\")
    parser.add_argument(\"--json\", action=\"store_true\", help=\"Output as JSON\")
    args = parser.parse_args()

    config = load_config()
    results = query_chromadb(args.query, top_k=args.top_k, config=config)

    if args.json:
        print(json.dumps(results, indent=2))
    else:
        if not results:
            print(\"No results found.\")
            return
        for r in results:
            print(f\"\\nChunk {r['rank']}: [similarity: {r['similarity']:.2f}] [source: {r['source']}]\")
            print(f\"  \\\"{r['chunk']}...\\\"\")


if __name__ == \"__main__\":
    main()
RAG_QUERY_EOF"
    tomb_root "cp ${TOMB_STAGING}/rag-query.py ${TOMB_ROOT}/grimoire/rag-query.py"
    tomb_root "chmod 755 ${TOMB_ROOT}/grimoire/rag-query.py"
    ok "rag-query.py deployed"

    # Verification
    info "Verifying Grimoire deployment..."
    verify_dir "${TOMB_ROOT}/grimoire/kingdom-docs"
    verify_dir "${TOMB_ROOT}/grimoire/mitre-attck"
    verify_dir "${TOMB_ROOT}/grimoire/nvd-cves"
    verify_dir "${TOMB_ROOT}/grimoire/rag"
    tomb_exec "test -x ${TOMB_ROOT}/grimoire/grimoire-search.sh" && ok "grimoire-search.sh executable" || fail "grimoire-search.sh not executable"
    tomb_exec "test -f ${TOMB_ROOT}/grimoire/rag-query.py" && ok "rag-query.py present" || fail "rag-query.py missing"

    ok "Grimoire knowledge base deployed"
}

# ============================================================================
# PHASE: DEPLOY-CERYDWYN
# ============================================================================
phase_deploy_cerydwyn() {
    header "PHASE 4: DEPLOY CERYDWYN LLM (Ollama + Mistral-7B)"

    info "Transferring Ollama binary..."
    if [ -f "${PACKAGES_DIR}/cerydwyn/ollama" ]; then
        tomb_scp "${PACKAGES_DIR}/cerydwyn/ollama" "${TOMB_STAGING}/ollama"
        tomb_root "cp ${TOMB_STAGING}/ollama /usr/local/bin/ollama"
        tomb_root "chmod 755 /usr/local/bin/ollama"
        ok "Ollama binary installed"
    else
        warn "No ollama binary in packages/cerydwyn/ -- download from ollama.com"
        info "Hint: curl -fsSL https://ollama.com/install.sh | sh  (on internet-connected host)"
        info "      cp /usr/local/bin/ollama tomb/packages/cerydwyn/ollama"
    fi

    info "Transferring Mistral-7B model..."
    if [ -d "${PACKAGES_DIR}/cerydwyn/models" ]; then
        tomb_scp "${PACKAGES_DIR}/cerydwyn/models/" "${TOMB_STAGING}/models/"
        tomb_root "cp -r ${TOMB_STAGING}/models/* ${TOMB_ROOT}/cerydwyn/ollama/models/"
        ok "Mistral-7B model files transferred"
    else
        warn "No models/ in packages/cerydwyn/ -- pre-pull model on internet-connected host"
        info "Hint: ollama pull mistral"
        info "      cp -r ~/.ollama/models/ tomb/packages/cerydwyn/models/"
    fi

    info "Deploying RAG configuration..."
    tomb_exec "cat > ${TOMB_ROOT}/cerydwyn/rag-config.json << 'RAG_CFG_EOF'
{
  \"embedding_model\": \"all-MiniLM-L6-v2\",
  \"llm_model\": \"mistral\",
  \"llm_endpoint\": \"http://127.0.0.1:11434\",
  \"top_k\": 5,
  \"chunk_size\": 512,
  \"chunk_overlap\": 50,
  \"chroma_dir\": \"/opt/tomb/grimoire/rag/chroma.db\",
  \"collection_name\": \"grimoire\",
  \"temperature\": 0.3,
  \"max_tokens\": 2048,
  \"system_prompt\": \"You are the BlackMage Cerydwyn, advisor to a kingdom's security team. Respond with tactical, actionable guidance grounded in the Kingdom's architecture and threat landscape. Reference specific documents when applicable.\"
}
RAG_CFG_EOF"
    ok "rag-config.json deployed"

    info "Deploying Cerydwyn systemd service..."
    tomb_root "cat > /etc/systemd/system/ollama.service << 'OLLAMA_SVC_EOF'
[Unit]
Description=Ollama LLM Service (The Cerydwyn)
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/ollama serve
Environment=OLLAMA_MODELS=/opt/tomb/cerydwyn/ollama/models
Environment=OLLAMA_HOST=127.0.0.1:11434
Restart=on-failure
RestartSec=10s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
OLLAMA_SVC_EOF"
    tomb_root "systemctl daemon-reload"
    ok "ollama.service unit created"

    info "Starting Ollama service..."
    tomb_root "systemctl enable ollama 2>/dev/null || true"
    tomb_root "systemctl start ollama 2>/dev/null || true"

    # Wait for Ollama to be ready
    local attempt=1
    while [ "${attempt}" -le 10 ]; do
        if tomb_exec "curl -sf http://127.0.0.1:11434/api/tags >/dev/null 2>&1"; then
            ok "Ollama service responding on :11434"
            break
        fi
        info "Waiting for Ollama to start (attempt ${attempt}/10)..."
        sleep 3
        attempt=$((attempt + 1))
    done

    if [ "${attempt}" -gt 10 ]; then
        warn "Ollama did not start within timeout -- check binary and model files"
    fi

    info "Deploying rag-cerydwyn.py..."
    tomb_exec "cat > ${TOMB_STAGING}/rag-cerydwyn.py << 'RAG_CERYDWYN_EOF'
#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# rag-cerydwyn.py -- Full RAG+LLM pipeline (retrieve -> augment -> generate)
\"\"\"
The Cerydwyn: RAG-augmented LLM for security threat modeling.
Queries Grimoire via ChromaDB, augments prompt, generates via Ollama.
\"\"\"
import argparse
import json
import os
import sys
import urllib.request
import urllib.error

TOMB_ROOT = \"/opt/tomb\"
RAG_CONFIG = os.path.join(TOMB_ROOT, \"cerydwyn\", \"rag-config.json\")
LOG_DIR = os.path.join(TOMB_ROOT, \"cerydwyn\", \"logs\")


def load_config():
    if os.path.exists(RAG_CONFIG):
        with open(RAG_CONFIG, \"r\") as f:
            return json.load(f)
    return {
        \"llm_endpoint\": \"http://127.0.0.1:11434\",
        \"llm_model\": \"mistral\",
        \"top_k\": 5,
        \"temperature\": 0.3,
        \"max_tokens\": 2048,
        \"system_prompt\": \"You are the BlackMage Cerydwyn.\",
    }


def retrieve_context(query, config):
    \"\"\"Retrieve relevant documents from Grimoire RAG index.\"\"\"
    try:
        # Import from the grimoire rag-query module
        sys.path.insert(0, os.path.join(TOMB_ROOT, \"grimoire\"))
        from importlib.machinery import SourceFileLoader
        rag_mod = SourceFileLoader(\"rag_query\",
                                   os.path.join(TOMB_ROOT, \"grimoire\", \"rag-query.py\")).load_module()
        results = rag_mod.query_chromadb(query, top_k=config.get(\"top_k\", 5), config=config)
        return results
    except Exception as e:
        print(f\"[WARN] RAG retrieval failed: {e}\", file=sys.stderr)
        return []


def build_prompt(query, context_docs, config):
    \"\"\"Build augmented prompt with retrieved Grimoire context.\"\"\"
    system_prompt = config.get(\"system_prompt\", \"You are the BlackMage Cerydwyn.\")

    context_block = \"\"
    if context_docs:
        context_block = \"\\nYou have access to the following Grimoire excerpts:\\n\\n\"
        for doc in context_docs:
            context_block += f\"[Source: {doc.get('source', 'unknown')}] (relevance: {doc.get('similarity', 0):.2f})\\n\"
            context_block += f\"{doc.get('chunk', '')}\\n\\n\"

    return f\"{system_prompt}\\n{context_block}\\nUser Query: {query}\\n\\nRespond with tactical, actionable guidance.\"


def query_ollama(prompt, config):
    \"\"\"Send prompt to Ollama and stream the response.\"\"\"
    endpoint = config.get(\"llm_endpoint\", \"http://127.0.0.1:11434\")
    model = config.get(\"llm_model\", \"mistral\")
    payload = json.dumps({
        \"model\": model,
        \"prompt\": prompt,
        \"stream\": True,
        \"options\": {
            \"temperature\": config.get(\"temperature\", 0.3),
            \"num_predict\": config.get(\"max_tokens\", 2048),
        },
    }).encode(\"utf-8\")

    req = urllib.request.Request(
        f\"{endpoint}/api/generate\",
        data=payload,
        headers={\"Content-Type\": \"application/json\"},
    )

    full_response = \"\"
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            for line in resp:
                chunk = json.loads(line.decode(\"utf-8\"))
                token = chunk.get(\"response\", \"\")
                sys.stdout.write(token)
                sys.stdout.flush()
                full_response += token
                if chunk.get(\"done\", False):
                    break
    except urllib.error.URLError as e:
        print(f\"\\n[ERROR] Cannot reach Ollama at {endpoint}: {e}\", file=sys.stderr)
        sys.exit(1)

    print()  # Final newline
    return full_response


def log_interaction(query, context_docs, response, config):
    \"\"\"Log the query-response pair for audit trail.\"\"\"
    os.makedirs(LOG_DIR, exist_ok=True)
    from datetime import datetime
    ts = datetime.now().strftime(\"%Y%m%d-%H%M%S\")
    log_entry = {
        \"timestamp\": ts,
        \"query\": query,
        \"context_sources\": [d.get(\"source\", \"unknown\") for d in context_docs],
        \"response_length\": len(response),
        \"model\": config.get(\"llm_model\", \"mistral\"),
    }
    log_path = os.path.join(LOG_DIR, f\"cerydwyn-{ts}.json\")
    with open(log_path, \"w\") as f:
        json.dump(log_entry, f, indent=2)


def main():
    parser = argparse.ArgumentParser(description=\"The Cerydwyn: RAG+LLM pipeline\")
    parser.add_argument(\"query\", help=\"Your question or request\")
    parser.add_argument(\"--lich-context\", help=\"Add LICH campaign context\")
    parser.add_argument(\"--no-rag\", action=\"store_true\", help=\"Skip RAG retrieval\")
    args = parser.parse_args()

    config = load_config()

    query = args.query
    if args.lich_context:
        query = f\"[Campaign context: {args.lich_context}] {query}\"

    # Step 1: Retrieve context from Grimoire
    context_docs = []
    if not args.no_rag:
        print(\"[Cerydwyn] Consulting the Grimoire...\", file=sys.stderr)
        context_docs = retrieve_context(query, config)
        if context_docs:
            print(f\"[Cerydwyn] Retrieved {len(context_docs)} relevant documents.\", file=sys.stderr)
        else:
            print(\"[Cerydwyn] No relevant documents found in Grimoire.\", file=sys.stderr)

    # Step 2: Build augmented prompt
    prompt = build_prompt(query, context_docs, config)

    # Step 3: Generate response via Ollama
    print(\"[Cerydwyn] Generating response...\\n\", file=sys.stderr)
    response = query_ollama(prompt, config)

    # Step 4: Log interaction
    log_interaction(query, context_docs, response, config)


if __name__ == \"__main__\":
    main()
RAG_CERYDWYN_EOF"
    tomb_root "cp ${TOMB_STAGING}/rag-cerydwyn.py ${TOMB_ROOT}/cerydwyn/rag-cerydwyn.py"
    tomb_root "chmod 755 ${TOMB_ROOT}/cerydwyn/rag-cerydwyn.py"
    ok "rag-cerydwyn.py deployed"

    info "Deploying cerydwyn-daemon.py..."
    tomb_exec "cat > ${TOMB_STAGING}/cerydwyn-daemon.py << 'DAEMON_EOF'
#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# cerydwyn-daemon.py -- Background service for continuous threat monitoring
\"\"\"
Periodically queries the Cerydwyn with contextual prompts and writes
suggestions to /opt/tomb/cerydwyn/suggestions/ for human review.
\"\"\"
import json
import os
import signal
import sys
import time
import urllib.request
import urllib.error
from datetime import datetime

TOMB_ROOT = \"/opt/tomb\"
RAG_CONFIG = os.path.join(TOMB_ROOT, \"cerydwyn\", \"rag-config.json\")
SUGGESTIONS_DIR = os.path.join(TOMB_ROOT, \"cerydwyn\", \"suggestions\")
POLL_INTERVAL = 3600  # 1 hour between suggestions
RUNNING = True


def signal_handler(signum, frame):
    global RUNNING
    print(f\"[daemon] Received signal {signum}, shutting down...\")
    RUNNING = False


signal.signal(signal.SIGTERM, signal_handler)
signal.signal(signal.SIGINT, signal_handler)


def load_config():
    if os.path.exists(RAG_CONFIG):
        with open(RAG_CONFIG, \"r\") as f:
            return json.load(f)
    return {\"llm_endpoint\": \"http://127.0.0.1:11434\", \"llm_model\": \"mistral\"}


def check_ollama(config):
    \"\"\"Check if Ollama is reachable.\"\"\"
    endpoint = config.get(\"llm_endpoint\", \"http://127.0.0.1:11434\")
    try:
        req = urllib.request.Request(f\"{endpoint}/api/tags\")
        with urllib.request.urlopen(req, timeout=5):
            return True
    except Exception:
        return False


def generate_suggestion(prompt, config):
    \"\"\"Generate a suggestion via Ollama (non-streaming).\"\"\"
    endpoint = config.get(\"llm_endpoint\", \"http://127.0.0.1:11434\")
    model = config.get(\"llm_model\", \"mistral\")
    payload = json.dumps({
        \"model\": model,
        \"prompt\": prompt,
        \"stream\": False,
        \"options\": {\"temperature\": 0.5, \"num_predict\": 512},
    }).encode(\"utf-8\")

    req = urllib.request.Request(
        f\"{endpoint}/api/generate\",
        data=payload,
        headers={\"Content-Type\": \"application/json\"},
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            result = json.loads(resp.read().decode(\"utf-8\"))
            return result.get(\"response\", \"\")
    except Exception as e:
        print(f\"[daemon] Generation failed: {e}\", file=sys.stderr)
        return None


def write_suggestion(suggestion_text, prompt_type):
    \"\"\"Write suggestion to file for human review.\"\"\"
    os.makedirs(SUGGESTIONS_DIR, exist_ok=True)
    ts = datetime.now().strftime(\"%Y%m%d-%H%M%S\")
    entry = {
        \"timestamp\": ts,
        \"type\": prompt_type,
        \"severity\": \"MEDIUM\",
        \"suggestion\": suggestion_text,
    }
    path = os.path.join(SUGGESTIONS_DIR, f\"suggestion-{ts}.json\")
    with open(path, \"w\") as f:
        json.dump(entry, f, indent=2)

    # Also append to latest.log for tail -f monitoring
    log_path = os.path.join(SUGGESTIONS_DIR, \"latest.log\")
    with open(log_path, \"a\") as f:
        f.write(f\"[{ts}] [{prompt_type}] {suggestion_text[:200]}...\\n\")

    print(f\"[daemon] Suggestion written: {path}\")


PROMPTS = [
    (\"threat-scan\", \"Based on the Kingdom architecture (microservices, LXD containers, Go/Rust stack, HAProxy LB, Wotan message bus), what are the top 3 attack vectors an adversary would target first? Be specific.\"),
    (\"cve-review\", \"Review the current CVE landscape for Go 1.21, HAProxy 2.8, Nginx 1.25, and QEMU. Identify any critical vulnerabilities that could affect our infrastructure.\"),
    (\"config-audit\", \"Analyze common misconfiguration patterns in LXD container deployments with NixOS. What hardening gaps should we check for?\"),
    (\"lateral-movement\", \"Given a compromised container in the 10.10.10.0/24 network segment, describe the most likely lateral movement paths to reach the control plane at 10.10.10.10.\"),
]


def main():
    config = load_config()
    os.makedirs(SUGGESTIONS_DIR, exist_ok=True)

    print(f\"[daemon] Cerydwyn daemon starting (poll interval: {POLL_INTERVAL}s)\")
    print(f\"[daemon] Suggestions dir: {SUGGESTIONS_DIR}\")

    prompt_index = 0
    while RUNNING:
        if not check_ollama(config):
            print(\"[daemon] Ollama not reachable, waiting...\", file=sys.stderr)
            time.sleep(60)
            continue

        prompt_type, prompt_text = PROMPTS[prompt_index % len(PROMPTS)]
        print(f\"[daemon] Generating suggestion: {prompt_type}\")

        suggestion = generate_suggestion(prompt_text, config)
        if suggestion:
            write_suggestion(suggestion, prompt_type)
        else:
            print(\"[daemon] No suggestion generated.\", file=sys.stderr)

        prompt_index += 1

        # Sleep in short intervals to respond to signals
        elapsed = 0
        while RUNNING and elapsed < POLL_INTERVAL:
            time.sleep(10)
            elapsed += 10

    print(\"[daemon] Shutdown complete.\")


if __name__ == \"__main__\":
    main()
DAEMON_EOF"
    tomb_root "cp ${TOMB_STAGING}/cerydwyn-daemon.py ${TOMB_ROOT}/cerydwyn/cerydwyn-daemon.py"
    tomb_root "chmod 755 ${TOMB_ROOT}/cerydwyn/cerydwyn-daemon.py"
    ok "cerydwyn-daemon.py deployed"

    info "Deploying cerydwyn-daemon systemd service..."
    tomb_root "cat > /etc/systemd/system/cerydwyn-daemon.service << 'CERYDWYND_EOF'
[Unit]
Description=Cerydwyn Daemon (continuous threat monitoring)
After=ollama.service
Wants=ollama.service

[Service]
Type=simple
User=root
ExecStart=/usr/bin/python3 /opt/tomb/cerydwyn/cerydwyn-daemon.py --config /opt/tomb/cerydwyn/rag-config.json
Restart=on-failure
RestartSec=30s
Environment=PYTHONUNBUFFERED=1

[Install]
WantedBy=multi-user.target
CERYDWYND_EOF"
    tomb_root "systemctl daemon-reload"
    tomb_root "systemctl enable cerydwyn-daemon 2>/dev/null || true"
    ok "cerydwyn-daemon.service deployed"

    # Verification
    info "Verifying Cerydwyn deployment..."
    verify_dir "${TOMB_ROOT}/cerydwyn/ollama"
    verify_dir "${TOMB_ROOT}/cerydwyn/logs"
    verify_dir "${TOMB_ROOT}/cerydwyn/suggestions"
    tomb_exec "test -f ${TOMB_ROOT}/cerydwyn/rag-config.json" && ok "rag-config.json present" || fail "rag-config.json missing"
    tomb_exec "test -f ${TOMB_ROOT}/cerydwyn/rag-cerydwyn.py" && ok "rag-cerydwyn.py present" || fail "rag-cerydwyn.py missing"
    tomb_exec "test -f ${TOMB_ROOT}/cerydwyn/cerydwyn-daemon.py" && ok "cerydwyn-daemon.py present" || fail "cerydwyn-daemon.py missing"

    if tomb_exec "command -v ollama >/dev/null 2>&1"; then
        ok "Ollama binary in PATH"
        verify_service "ollama" "11434"
    else
        warn "Ollama binary not found -- stage it in packages/cerydwyn/ollama"
    fi

    ok "Cerydwyn LLM deployed"
}

# ============================================================================
# PHASE: DEPLOY-DARK-MIRROR
# ============================================================================
phase_deploy_dark_mirror() {
    header "PHASE 5: DEPLOY DARK MIRROR OBSERVABILITY"

    info "Transferring observability binaries..."
    local bins_transferred=0
    for bin_name in prometheus grafana-server promtail loki; do
        if [ -f "${PACKAGES_DIR}/dark-mirror/${bin_name}" ]; then
            tomb_scp "${PACKAGES_DIR}/dark-mirror/${bin_name}" "${TOMB_STAGING}/${bin_name}"
            tomb_root "cp ${TOMB_STAGING}/${bin_name} /usr/local/bin/${bin_name}"
            tomb_root "chmod 755 /usr/local/bin/${bin_name}"
            ok "  ${bin_name} binary installed"
            bins_transferred=$((bins_transferred + 1))
        else
            warn "  ${bin_name} binary missing from packages/dark-mirror/"
        fi
    done
    if [ "${bins_transferred}" -eq 0 ]; then
        warn "No Dark Mirror binaries found -- stage them in packages/dark-mirror/"
    fi

    info "Deploying Prometheus configuration..."
    tomb_exec "cat > ${TOMB_ROOT}/dark-mirror/prometheus/prometheus.yml << 'PROM_EOF'
# SPDX-License-Identifier: MIT
# Tomb of Knowledge -- Dark Mirror Prometheus configuration
# Monitors the Kingdom from an external (attacker) perspective.

global:
  scrape_interval: 15s
  evaluation_interval: 15s
  scrape_timeout: 10s

  external_labels:
    monitor: 'tomb-dark-mirror'
    environment: 'security-testing'

rule_files:
  - '/opt/tomb/dark-mirror/alerts/*.yml'

scrape_configs:
  # Self-monitoring
  - job_name: 'tomb-prometheus'
    static_configs:
      - targets: ['127.0.0.1:9090']
        labels:
          layer: 'dark-mirror'

  # Kingdom services (probed from attacker perspective)
  - job_name: 'kingdom-services'
    scrape_interval: 30s
    scrape_timeout: 15s
    metrics_path: '/metrics'
    static_configs:
      - targets:
          - '192.168.13.2:17000'    # unheaded-daemon
          - '192.168.13.2:18000'    # wotan
          - '192.168.13.2:19000'    # timeguru
          - '192.168.13.2:19001'    # architect
          - '192.168.13.2:19002'    # captain
          - '192.168.13.2:19003'    # micromanager
          - '192.168.13.2:19004'    # monad
          - '192.168.13.2:19005'    # sophia
        labels:
          layer: 'kingdom'
          perspective: 'external'

  # Kingdom gateway (TLS termination point)
  - job_name: 'kingdom-gateway'
    scrape_interval: 10s
    static_configs:
      - targets:
          - '192.168.13.2:21000'    # gateway HTTP
        labels:
          layer: 'kingdom-edge'
          perspective: 'external'

  # HAProxy stats (if exposed)
  - job_name: 'kingdom-haproxy'
    scrape_interval: 30s
    metrics_path: '/metrics'
    static_configs:
      - targets:
          - '192.168.13.2:21404'    # haproxy-edge stats
          - '192.168.13.2:21405'    # haproxy-internal stats
        labels:
          layer: 'kingdom-lb'
          perspective: 'external'

  # Lich campaign metrics (local exporter)
  - job_name: 'lich-campaigns'
    scrape_interval: 60s
    static_configs:
      - targets: ['127.0.0.1:9101']
        labels:
          layer: 'lich'

  # Node exporter (Tomb VM self-health)
  - job_name: 'tomb-node'
    static_configs:
      - targets: ['127.0.0.1:9100']
        labels:
          layer: 'tomb-self'
PROM_EOF"
    ok "Prometheus config deployed"

    info "Deploying Grafana configuration..."
    tomb_exec "cat > ${TOMB_ROOT}/dark-mirror/grafana/grafana.ini << 'GRAF_EOF'
# SPDX-License-Identifier: MIT
# Tomb of Knowledge -- Grafana configuration

[server]
http_addr = 127.0.0.1
http_port = 3000
protocol = http
domain = tomb.local

[database]
type = sqlite3
path = /opt/tomb/dark-mirror/grafana/grafana.db

[security]
admin_user = blackmage
admin_password = changeme-on-first-boot
disable_gravatar = true
cookie_secure = false

[auth.anonymous]
enabled = true
org_role = Viewer

[dashboards]
default_home_dashboard_path = /opt/tomb/dark-mirror/grafana/dashboards/kingdom-overview.json

[paths]
data = /opt/tomb/dark-mirror/grafana
logs = /opt/tomb/dark-mirror/grafana/logs
plugins = /opt/tomb/dark-mirror/grafana/plugins
provisioning = /opt/tomb/dark-mirror/grafana/provisioning

[analytics]
reporting_enabled = false
check_for_updates = false
GRAF_EOF"
    ok "Grafana config deployed"

    info "Deploying Grafana datasource provisioning..."
    tomb_root "mkdir -p ${TOMB_ROOT}/dark-mirror/grafana/provisioning/datasources"
    tomb_exec "cat > ${TOMB_ROOT}/dark-mirror/grafana/provisioning/datasources/tomb.yml << 'DS_EOF'
apiVersion: 1
datasources:
  - name: Prometheus (Tomb)
    type: prometheus
    access: proxy
    url: http://127.0.0.1:9090
    isDefault: true
    editable: false
  - name: Loki (Tomb)
    type: loki
    access: proxy
    url: http://127.0.0.1:3100
    editable: false
DS_EOF"
    ok "Grafana datasources provisioned"

    info "Deploying Loki configuration..."
    tomb_exec "cat > ${TOMB_ROOT}/dark-mirror/loki/loki.yml << 'LOKI_EOF'
# SPDX-License-Identifier: MIT
# Tomb of Knowledge -- Loki log aggregation configuration

auth_enabled: false

server:
  http_listen_port: 3100
  http_listen_address: 127.0.0.1
  grpc_listen_port: 9096
  grpc_listen_address: 127.0.0.1

common:
  path_prefix: /opt/tomb/dark-mirror/loki/data
  storage:
    filesystem:
      chunks_directory: /opt/tomb/dark-mirror/loki/data/chunks
      rules_directory: /opt/tomb/dark-mirror/loki/data/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2024-01-01
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

limits_config:
  retention_period: 720h
  max_query_length: 721h
  max_query_parallelism: 2

compactor:
  working_directory: /opt/tomb/dark-mirror/loki/data/compactor
  compaction_interval: 10m
  retention_enabled: true
LOKI_EOF"
    tomb_root "mkdir -p ${TOMB_ROOT}/dark-mirror/loki/data/{chunks,rules,compactor}"
    ok "Loki config deployed"

    info "Deploying alerting rules..."
    tomb_exec "cat > ${TOMB_ROOT}/dark-mirror/alerts/kingdom-alerts.yml << 'ALERTS_EOF'
# SPDX-License-Identifier: MIT
# Tomb of Knowledge -- Dark Mirror alerting rules
groups:
  - name: kingdom_health
    interval: 30s
    rules:
      - alert: KingdomServiceDown
        expr: up{job=\"kingdom-services\"} == 0
        for: 2m
        labels:
          severity: critical
          layer: kingdom
        annotations:
          summary: \"Kingdom service {{ \$labels.instance }} is DOWN\"
          description: \"Service has been unreachable for 2+ minutes from Tomb perspective.\"

      - alert: KingdomHighLatency
        expr: probe_duration_seconds{job=\"kingdom-services\"} > 1.0
        for: 5m
        labels:
          severity: warning
          layer: kingdom
        annotations:
          summary: \"High latency to {{ \$labels.instance }} ({{ \$value }}s)\"

      - alert: KingdomHighErrorRate
        expr: rate(unheaded_http_requests_total{status=~\"5..\"}[5m]) > 0.1
        for: 3m
        labels:
          severity: warning
          layer: kingdom
        annotations:
          summary: \"High 5xx error rate on {{ \$labels.service }}\"

  - name: tomb_health
    interval: 60s
    rules:
      - alert: TombDiskSpaceLow
        expr: node_filesystem_avail_bytes{mountpoint=\"/\"} / node_filesystem_size_bytes{mountpoint=\"/\"} < 0.1
        for: 5m
        labels:
          severity: warning
          layer: tomb
        annotations:
          summary: \"Tomb VM disk space below 10%\"

      - alert: OllamaDown
        expr: up{job=\"tomb-ollama\"} == 0
        for: 5m
        labels:
          severity: warning
          layer: tomb
        annotations:
          summary: \"Cerydwyn (Ollama) service is DOWN\"
ALERTS_EOF"
    ok "Alerting rules deployed"

    info "Deploying Dark Mirror systemd services..."

    # Prometheus service
    tomb_root "cat > /etc/systemd/system/tomb-prometheus.service << 'PROMSVC_EOF'
[Unit]
Description=Prometheus (Dark Mirror)
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/prometheus \
  --config.file=/opt/tomb/dark-mirror/prometheus/prometheus.yml \
  --storage.tsdb.path=/opt/tomb/dark-mirror/prometheus/data \
  --storage.tsdb.retention.time=30d \
  --web.listen-address=127.0.0.1:9090 \
  --web.enable-lifecycle
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=multi-user.target
PROMSVC_EOF"

    # Grafana service
    tomb_root "cat > /etc/systemd/system/tomb-grafana.service << 'GRAFSVC_EOF'
[Unit]
Description=Grafana (Dark Mirror)
After=tomb-prometheus.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/grafana-server \
  --config=/opt/tomb/dark-mirror/grafana/grafana.ini \
  --homepath=/usr/share/grafana
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=multi-user.target
GRAFSVC_EOF"

    # Loki service
    tomb_root "cat > /etc/systemd/system/tomb-loki.service << 'LOKISVC_EOF'
[Unit]
Description=Loki (Dark Mirror)
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/loki \
  --config.file=/opt/tomb/dark-mirror/loki/loki.yml
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=multi-user.target
LOKISVC_EOF"

    tomb_root "systemctl daemon-reload"
    tomb_root "systemctl enable tomb-prometheus tomb-grafana tomb-loki 2>/dev/null || true"
    tomb_root "systemctl start tomb-prometheus 2>/dev/null || true"
    tomb_root "systemctl start tomb-loki 2>/dev/null || true"
    tomb_root "systemctl start tomb-grafana 2>/dev/null || true"
    ok "Dark Mirror systemd services deployed"

    # Verification
    info "Verifying Dark Mirror deployment..."
    verify_dir "${TOMB_ROOT}/dark-mirror/prometheus"
    verify_dir "${TOMB_ROOT}/dark-mirror/grafana"
    verify_dir "${TOMB_ROOT}/dark-mirror/loki"
    verify_dir "${TOMB_ROOT}/dark-mirror/alerts"

    tomb_exec "test -f ${TOMB_ROOT}/dark-mirror/prometheus/prometheus.yml" && ok "prometheus.yml present" || fail "prometheus.yml missing"
    tomb_exec "test -f ${TOMB_ROOT}/dark-mirror/grafana/grafana.ini" && ok "grafana.ini present" || fail "grafana.ini missing"
    tomb_exec "test -f ${TOMB_ROOT}/dark-mirror/loki/loki.yml" && ok "loki.yml present" || fail "loki.yml missing"
    tomb_exec "test -f ${TOMB_ROOT}/dark-mirror/alerts/kingdom-alerts.yml" && ok "alerting rules present" || fail "alerting rules missing"

    verify_service "tomb-prometheus" "9090"
    verify_service "tomb-grafana" "3000"
    verify_service "tomb-loki" "3100"

    ok "Dark Mirror observability stack deployed"
}

# ============================================================================
# PHASE: HARDEN
# ============================================================================
phase_harden() {
    header "PHASE 13: SECURITY HARDENING"

    info "Configuring firewall (default deny, explicit allow)..."
    # Write firewall script via individual commands to avoid heredoc quoting issues
    tomb_root "printf '%s\n' '#!/bin/sh' > ${TOMB_STAGING}/tomb-firewall.sh"
    tomb_root "printf '%s\n' '# SPDX-License-Identifier: MIT' >> ${TOMB_STAGING}/tomb-firewall.sh"
    tomb_root "printf '%s\n' '# Tomb VM firewall rules -- default deny, explicit allow' >> ${TOMB_STAGING}/tomb-firewall.sh"
    tomb_root "printf '%s\n' 'set -eu' >> ${TOMB_STAGING}/tomb-firewall.sh"
    tomb_exec "cat >> ${TOMB_STAGING}/tomb-firewall.sh << 'FW_BODY'
iptables -F
iptables -X
iptables -Z
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT DROP
iptables -A INPUT -i lo -j ACCEPT
iptables -A OUTPUT -o lo -j ACCEPT
iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -A INPUT -s 192.168.13.2 -p tcp --dport 22 -j ACCEPT
iptables -A INPUT -s 192.168.13.0/30 -p icmp -j ACCEPT
iptables -A OUTPUT -d 192.168.13.0/30 -p icmp -j ACCEPT
iptables -A OUTPUT -d 192.168.13.0/30 -j ACCEPT
iptables -A OUTPUT -d 10.0.0.0/8 -j DROP
iptables -A OUTPUT -d 172.16.0.0/12 -j DROP
iptables -A OUTPUT -d 192.168.13.0/30 -j ACCEPT
iptables -A OUTPUT -d 192.168.0.0/16 -j DROP
iptables -A OUTPUT -j DROP
iptables -I INPUT -m limit --limit 5/min -j LOG --log-prefix 'TOMB-IN-DROP: '
iptables -I OUTPUT -m limit --limit 5/min -j LOG --log-prefix 'TOMB-OUT-DROP: '
echo '[  OK] Firewall rules applied'
FW_BODY"
    tomb_root "chmod 755 ${TOMB_STAGING}/tomb-firewall.sh"
    tomb_root "cp ${TOMB_STAGING}/tomb-firewall.sh /etc/tomb-firewall.sh"
    tomb_root "/etc/tomb-firewall.sh"
    ok "Firewall rules applied"

    info "Making firewall persistent across reboots..."
    tomb_exec "cat > /tmp/tomb-fw.service << 'FWSVC'
[Unit]
Description=Tomb Firewall Rules
Before=network-pre.target
Wants=network-pre.target

[Service]
Type=oneshot
ExecStart=/etc/tomb-firewall.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
FWSVC"
    tomb_root "cp /tmp/tomb-fw.service /etc/systemd/system/tomb-firewall.service"
    tomb_root "systemctl daemon-reload"
    tomb_root "systemctl enable tomb-firewall 2>/dev/null || true"
    ok "Firewall persistence configured"

    info "Hardening SSH..."
    tomb_root "sed -i 's/^#*PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config"
    tomb_root "sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config"
    tomb_root "sed -i 's/^#*X11Forwarding.*/X11Forwarding no/' /etc/ssh/sshd_config"
    tomb_root "sed -i 's/^#*MaxAuthTries.*/MaxAuthTries 3/' /etc/ssh/sshd_config"
    tomb_root "sed -i 's/^#*AllowTcpForwarding.*/AllowTcpForwarding no/' /etc/ssh/sshd_config"
    tomb_root "grep -q '^ListenAddress' /etc/ssh/sshd_config || echo 'ListenAddress 192.168.13.6' >> /etc/ssh/sshd_config"
    tomb_root "systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || true"
    ok "SSH hardened (no root login, key-only, no forwarding)"

    info "Setting filesystem permissions..."
    tomb_root "chmod 700 ${TOMB_ROOT}/cerydwyn"
    tomb_root "chmod 700 ${TOMB_ROOT}/reports"
    tomb_root "chmod 750 ${TOMB_ROOT}/lich"
    tomb_root "chmod 750 ${TOMB_ROOT}/grimoire"
    tomb_root "chmod 750 ${TOMB_ROOT}/dark-mirror"
    tomb_root "chmod 640 ${TOMB_ROOT}/scope.conf"
    ok "Filesystem permissions tightened"

    info "Configuring audit logging..."
    tomb_root "mkdir -p /var/log/tomb-audit"
    tomb_exec "cat > /tmp/tomb-audit.conf << 'AUDIT_CONF'
# Tomb audit logging -- capture all security-relevant events
:msg, contains, \"TOMB-\" /var/log/tomb-audit/firewall.log
& stop
AUDIT_CONF"
    tomb_root "cp /tmp/tomb-audit.conf /etc/rsyslog.d/tomb-audit.conf"
    tomb_root "systemctl restart rsyslog 2>/dev/null || true"
    ok "Audit logging configured"

    info "Disabling unnecessary services..."
    local disable_svcs="avahi-daemon cups bluetooth ModemManager"
    for svc in ${disable_svcs}; do
        tomb_root "systemctl disable ${svc} 2>/dev/null || true"
        tomb_root "systemctl stop ${svc} 2>/dev/null || true"
    done
    ok "Unnecessary services disabled"

    info "Setting kernel hardening parameters..."
    tomb_exec "cat > /tmp/tomb-sysctl.conf << 'SYSCTL_CONF'
# Tomb of Knowledge -- kernel hardening
net.ipv4.ip_forward = 0
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.default.accept_source_route = 0
net.ipv4.conf.all.log_martians = 1
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.tcp_syncookies = 1
net.ipv6.conf.all.disable_ipv6 = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv6.conf.default.accept_redirects = 0
kernel.randomize_va_space = 2
kernel.kptr_restrict = 2
kernel.dmesg_restrict = 1
kernel.perf_event_paranoid = 3
kernel.yama.ptrace_scope = 1
fs.protected_hardlinks = 1
fs.protected_symlinks = 1
fs.suid_dumpable = 0
SYSCTL_CONF"
    tomb_root "cp /tmp/tomb-sysctl.conf /etc/sysctl.d/99-tomb-hardening.conf"
    tomb_root "sysctl -p /etc/sysctl.d/99-tomb-hardening.conf 2>/dev/null || true"
    ok "Kernel hardening applied"

    # Verification
    info "Verifying hardening..."
    if tomb_exec "iptables -L -n 2>/dev/null | grep -q 'DROP'" 2>/dev/null; then
        ok "Firewall active (default DROP)"
    else
        warn "Firewall rules may not be active"
    fi

    if tomb_exec "grep -q 'PermitRootLogin no' /etc/ssh/sshd_config" 2>/dev/null; then
        ok "SSH root login disabled"
    else
        warn "SSH root login may still be enabled"
    fi

    if tomb_exec "sysctl kernel.randomize_va_space 2>/dev/null | grep -q '= 2'" 2>/dev/null; then
        ok "ASLR enabled (full randomization)"
    else
        warn "ASLR may not be fully enabled"
    fi

    ok "Security hardening complete"
}

# ============================================================================
# PHASE: FULL VERIFICATION
# ============================================================================
phase_verify_all() {
    header "FULL DEPLOYMENT VERIFICATION"

    local pass=0
    local total=0

    # Connectivity
    total=$((total + 1))
    info "[1] SSH connectivity..."
    if verify_ssh 2>/dev/null; then pass=$((pass + 1)); fi

    total=$((total + 1))
    info "[2] Air-gap enforcement..."
    if ! tomb_exec "ping -c 1 -W 2 8.8.8.8" >/dev/null 2>&1; then
        ok "Air-gap intact"; pass=$((pass + 1))
    else
        fail "AIR-GAP VIOLATION"
    fi

    # Layer 1: Arsenal (Kali base)
    total=$((total + 1))
    info "[3] Layer 1 -- Arsenal (Kali tools)..."
    if tomb_exec "command -v nmap >/dev/null 2>&1 && command -v msfconsole >/dev/null 2>&1"; then
        ok "Arsenal tools available"; pass=$((pass + 1))
    else
        warn "Some arsenal tools missing"
    fi

    # Layer 2: Lich
    total=$((total + 1))
    info "[4] Layer 2 -- Lich framework..."
    if tomb_exec "test -x ${TOMB_ROOT}/lich/lich-runner.sh && test -d ${TOMB_ROOT}/lich/campaigns/LICH-001" 2>/dev/null; then
        ok "Lich framework deployed"; pass=$((pass + 1))
    else
        fail "Lich framework incomplete"
    fi

    # Layer 3: Grimoire
    total=$((total + 1))
    info "[5] Layer 3 -- Grimoire knowledge base..."
    if tomb_exec "test -d ${TOMB_ROOT}/grimoire/kingdom-docs && test -x ${TOMB_ROOT}/grimoire/grimoire-search.sh" 2>/dev/null; then
        ok "Grimoire deployed"; pass=$((pass + 1))
    else
        fail "Grimoire incomplete"
    fi

    # Layer 4: Cerydwyn
    total=$((total + 1))
    info "[6] Layer 4 -- Cerydwyn LLM..."
    if tomb_exec "test -f ${TOMB_ROOT}/cerydwyn/rag-config.json && test -f ${TOMB_ROOT}/cerydwyn/rag-cerydwyn.py" 2>/dev/null; then
        ok "Cerydwyn pipeline deployed"; pass=$((pass + 1))
    else
        fail "Cerydwyn incomplete"
    fi

    total=$((total + 1))
    info "[7] Layer 4 -- Ollama service..."
    if tomb_exec "curl -sf http://127.0.0.1:11434/api/tags >/dev/null 2>&1"; then
        ok "Ollama responding on :11434"; pass=$((pass + 1))
    else
        warn "Ollama not responding (check binary/model)"
    fi

    # Layer 5: Dark Mirror
    total=$((total + 1))
    info "[8] Layer 5 -- Prometheus..."
    if verify_service "tomb-prometheus" "9090" 2>/dev/null; then pass=$((pass + 1)); fi

    total=$((total + 1))
    info "[9] Layer 5 -- Grafana..."
    if verify_service "tomb-grafana" "3000" 2>/dev/null; then pass=$((pass + 1)); fi

    total=$((total + 1))
    info "[10] Layer 5 -- Loki..."
    if verify_service "tomb-loki" "3100" 2>/dev/null; then pass=$((pass + 1)); fi

    # Hardening
    total=$((total + 1))
    info "[11] Hardening -- Firewall..."
    if tomb_exec "iptables -L -n 2>/dev/null | grep -q 'DROP'" 2>/dev/null; then
        ok "Firewall active"; pass=$((pass + 1))
    else
        warn "Firewall not active"
    fi

    total=$((total + 1))
    info "[12] Hardening -- SSH..."
    if tomb_exec "grep -q 'PermitRootLogin no' /etc/ssh/sshd_config" 2>/dev/null; then
        ok "SSH hardened"; pass=$((pass + 1))
    else
        warn "SSH not fully hardened"
    fi

    # Summary
    echo ""
    echo -e "${CYAN}================================================================${NC}"
    if [ "${pass}" -eq "${total}" ]; then
        echo -e "${GREEN}  TOMB OF KNOWLEDGE: ${pass}/${total} CHECKS PASSED${NC}"
        echo -e "${GREEN}  ALL FIVE LAYERS OPERATIONAL${NC}"
    else
        echo -e "${YELLOW}  TOMB OF KNOWLEDGE: ${pass}/${total} CHECKS PASSED${NC}"
        echo -e "${YELLOW}  Review warnings above -- some layers need attention${NC}"
    fi
    echo -e "${CYAN}================================================================${NC}"
    echo ""

    _log "VERIFY" "Final score: ${pass}/${total}"
}

# ============================================================================
# USAGE / HELP
# ============================================================================
usage() {
    cat << 'USAGE_EOF'
Tomb of Knowledge -- Master Provisioning Script

USAGE:
    ./provision.sh [OPTIONS]

OPTIONS:
    --target IP        Target VM IP (default: 192.168.13.6)
    --phase PHASE      Run a single phase
    --dry-run          Print commands without executing
    --verbose          Enable verbose output
    --list             List available phases
    --help             Show this help

PHASES:
    setup-base         Base system: directories, packages, users
    deploy-lich        Layer 2: Lich adversary framework
    deploy-grimoire    Layer 3: Grimoire knowledge base + RAG
    deploy-cerydwyn      Layer 4: Cerydwyn LLM (Ollama + Mistral)
    deploy-dark-mirror Layer 5: Dark Mirror observability
    harden             Phase 13: Security hardening
    verify             Full deployment verification

EXAMPLES:
    ./provision.sh                              # Full provision
    ./provision.sh --phase deploy-lich          # Deploy Lich only
    ./provision.sh --target 10.0.0.5 --dry-run  # Dry run against alt target
    ./provision.sh --phase harden               # Hardening only

PREREQUISITES:
    - Tomb VM booted and reachable via SSH
    - SSH key at ~/.ssh/id_tomb (or set TOMB_SSH_KEY)
    - Pre-staged packages in tomb/packages/ (air-gapped)
USAGE_EOF
    exit 0
}

list_phases() {
    echo "Available phases:"
    echo "  setup-base         Base system setup"
    echo "  deploy-lich        Lich adversary framework"
    echo "  deploy-grimoire    Grimoire knowledge base"
    echo "  deploy-cerydwyn      Cerydwyn LLM"
    echo "  deploy-dark-mirror Dark Mirror observability"
    echo "  harden             Security hardening"
    echo "  verify             Deployment verification"
    exit 0
}

# ============================================================================
# MAIN
# ============================================================================
main() {
    # Parse arguments
    while [ $# -gt 0 ]; do
        # shellcheck disable=SC2034  # VERBOSE below is set here but never read;
        # see its declaration for why it is flagged rather than deleted.
        case "$1" in
            --target)   TOMB_TARGET="$2"; shift 2 ;;
            --phase)    PHASE="$2"; shift 2 ;;
            --dry-run)  DRY_RUN=1; shift ;;
            --verbose)  VERBOSE=1; shift ;;
            --list)     list_phases ;;
            --help|-h)  usage ;;
            *)          die "Unknown option: $1 (use --help)" ;;
        esac
    done

    # Banner
    echo -e "${CYAN}"
    echo "  +---------------------------------------------------------+"
    echo "  |   THE TOMB OF KNOWLEDGE                                 |"
    echo "  |   BlackMage Provisioning System                         |"
    echo "  |   Target: ${TOMB_TARGET}                            |"
    echo "  |   Host:   ${HOST_IP} (WEST)                        |"
    echo "  |   Bridge: ${BRIDGE_IP}/30                          |"
    echo "  +---------------------------------------------------------+"
    echo -e "${NC}"

    info "Log file: ${LOG_FILE}"
    echo ""

    if [ "${DRY_RUN}" -eq 1 ]; then
        warn "DRY RUN MODE -- no commands will be executed on target"
        echo ""
    fi

    if [ -n "${PHASE}" ]; then
        # Single phase
        case "${PHASE}" in
            setup-base)         phase_preflight; phase_setup_base ;;
            deploy-lich)        phase_deploy_lich ;;
            deploy-grimoire)    phase_deploy_grimoire ;;
            deploy-cerydwyn)      phase_deploy_cerydwyn ;;
            deploy-dark-mirror) phase_deploy_dark_mirror ;;
            harden)             phase_harden ;;
            verify)             phase_verify_all ;;
            *)                  die "Unknown phase: ${PHASE} (use --list)" ;;
        esac
    else
        # Full provision
        phase_preflight
        phase_setup_base
        phase_deploy_lich
        phase_deploy_grimoire
        phase_deploy_cerydwyn
        phase_deploy_dark_mirror
        phase_harden
        phase_verify_all
    fi

    echo ""
    info "Provisioning complete. Log: ${LOG_FILE}"
}

main "$@"
