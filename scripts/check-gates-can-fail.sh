#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-gates-can-fail.sh — the meta-gate. Every gate must be able to FAIL.
#
# WHY THIS EXISTS
#
# Four consecutive promotion batches shipped a gate that was green because it
# could not fail:
#
#   B2  check-gosec-ratchet.sh   guards were unreachable dead code — the script
#                                returned early whenever the exclusion list was
#                                empty, and the same commit emptied it.
#   B3  check-secrets-baseline.sh compared a COUNT, so deleting one fingerprint
#                                and appending a live one passed green. A
#                                missing ceiling file silently re-baselined.
#                                `grep -cE ... || echo 0` yielded "0\n0", making
#                                every later [ -gt ] abort rc=2 and fall through
#                                to PASS.
#   B3  pkg/uids manifest test   matched <service>.yaml while the services live
#                                at <service>/deployment.yaml, so it walked past
#                                all 11 shared-UID violations. Its `found == 0`
#                                backstop could not fire either.
#   B4  check-clippy.sh          only kept file:line:col diagnostics and never
#                                checked cargo's exit code, so two workspaces
#                                that did not build at all counted as zero
#                                warnings and zero errors.
#
# Every one was found by hand, by someone happening to poke it. That does not
# scale and it did not converge — the fourth was in the very script whose own
# header warns about the pattern.
#
# The fix is to stop testing gates one at a time and assert the PROPERTY: a gate
# that cannot fail is not a gate. For each check script this plants a known
# violation, asserts the script exits non-zero, and restores the tree.
#
# THIS SCRIPT IS ITSELF A GATE, so it must obey its own rule. Two ways it fails:
#
#   1. a registered gate does not fail when provoked      (the point)
#   2. a check-*.sh exists with NO registered provocation (the trap)
#
# (2) matters more than it looks. Without it, adding a new unguarded gate would
# silently shrink coverage and this script would keep printing PASS — which is
# precisely the bug, one level up. A new gate is a build failure until someone
# writes the provocation that proves it bites.
#
# WHAT THIS DOES NOT COVER, deliberately
#
# CI also gates on direct tool invocations that are not scripts:
#   cargo clippy -- -D warnings      (ebpf.yml)
#   cargo fmt -- --check             (ebpf.yml)
#   go build ./... / go vet ./...    (ci.yml)
#   the shellcheck -S warning sweep  (static-analysis.yml)
#
# (that last line is worded to avoid starting with the tool's name — a comment
#  whose first word is "shellcheck" is parsed as a DIRECTIVE, and this exact
#  line broke the file with SC1072/SC1073 when first written)
#
# Those are out of scope on purpose. The failure mode this script exists to
# catch is CUSTOM LOGIC getting the verdict wrong — an unreachable guard, a
# count compared instead of a set, a grep that drops the diagnostic that
# mattered, an exit code never read. A bare `cargo clippy -- -D warnings` has
# no such logic: the tool's own exit status IS the verdict, and there is
# nothing in between to be wrong.
#
# The moment one of them gets wrapped in a script with any logic of its own,
# it becomes discoverable here and must be classified.
#
# USAGE
#   ./scripts/check-gates-can-fail.sh            # all gates
#   ./scripts/check-gates-can-fail.sh --quick    # skip gates marked SLOW
#   ./scripts/check-gates-can-fail.sh --list     # show registry, run nothing
#   ./scripts/check-gates-can-fail.sh --only NAME  # provoke one registered gate
#
# SAFETY
# It mutates tracked files on purpose. It refuses to run on a dirty tree, and an
# EXIT trap restores every file it touched even on Ctrl-C or a failed assertion.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}" || exit 1

# lich-cargo-fuzz defaults to 60 s per target, ten targets, and this script
# runs it twice (clean, then provoked): over 20 minutes, which ran the CI job
# past its 25-minute limit on 2026-10-25. Proving the gate bites needs a few
# seconds per target; lich.yml does the real fuzzing.
export LICH_FUZZ_SECONDS="${LICH_FUZZ_SECONDS:-5}"

MODE="${1:-}"
ONLY=""
if [ "${MODE}" = "--only" ]; then
    ONLY="${2:-}"
    [ -n "${ONLY}" ] || { echo "usage: $0 --only <gate name from --list>" >&2; exit 2; }
fi
BACKUP_DIR="$(mktemp -d)"
TOUCHED=()
CREATED=()          # files the provocation brought into existence
CREATED_DIRS=()     # directories it had to mkdir -- see restore_touched
FAILURES=0
CHECKED=0
SKIPPED=0

# ---------------------------------------------------------------------------
# Restore. Registered before ANY mutation happens.
# ---------------------------------------------------------------------------
DID_PROVOKE=0

# shellcheck disable=SC2317  # invoked via trap
restore_all() {
    local rc=$?
    restore_touched
    rm -rf "${BACKUP_DIR}"

    # Belt and braces: if the tree is still dirty after a run that mutated it,
    # say so loudly rather than leave the operator guessing. Only meaningful if
    # we actually provoked something — otherwise it just reports the operator's
    # own untracked files.
    if [ "${DID_PROVOKE}" -eq 1 ] && [ -n "$(git -C "${REPO_ROOT}" status --porcelain 2>/dev/null)" ]; then
        echo
        echo "  WARNING: working tree is not clean after restore. Inspect:"
        git -C "${REPO_ROOT}" status --short | sed 's/^/    /'
    fi
    exit "${rc}"
}
trap restore_all EXIT INT TERM

# shellcheck disable=SC2317  # called from provoke_* dispatch
# Register a directory the provocation creates, so restore removes it.
#
# This exists because of a real incident: the verify-gpl-boundary provocation
# did `mkdir -p vendor/meta-gate-probe`, restore deleted only the FILE, and the
# empty vendor/ directory it left behind at the module root flipped Go into
# vendoring mode -- `go build ./...` then failed with "inconsistent vendoring"
# across every dependency. Git does not track empty directories, so the
# clean-tree check saw nothing wrong and the damage was invisible.
# shellcheck disable=SC2317  # called from provoke_* dispatch
backup_dir() {
    CREATED_DIRS+=("$1")
}

# shellcheck disable=SC2317  # called from provoke_* dispatch
backup() {
    local f="$1"
    local slug
    slug="$(echo "${f}" | tr '/' '_')"
    TOUCHED+=("${f}")
    if [ -f "${REPO_ROOT}/${f}" ]; then
        cp "${REPO_ROOT}/${f}" "${BACKUP_DIR}/${slug}"
    else
        CREATED+=("${f}")
    fi
}

# Restore every provoked file to its pre-provocation state.
#
# The git-index cleanup is deliberately restricted to files in CREATED. An
# earlier version ran `git rm --cached` over everything in TOUCHED, which would
# have UNTRACKED real files such as .gitleaksignore — a restore step that
# quietly does more damage than the thing it is restoring from.
# shellcheck disable=SC2317  # called from the EXIT trap and the run loop
restore_touched() {
    local f slug
    for f in "${TOUCHED[@]:-}"; do
        [ -n "${f}" ] || continue
        slug="$(echo "${f}" | tr '/' '_')"
        if [ -f "${BACKUP_DIR}/${slug}" ]; then
            cp "${BACKUP_DIR}/${slug}" "${REPO_ROOT}/${f}"
        else
            rm -f "${REPO_ROOT}/${f}"
        fi
    done
    for f in "${CREATED[@]:-}"; do
        [ -n "${f}" ] || continue
        git -C "${REPO_ROOT}" rm --cached -q --force "${f}" >/dev/null 2>&1 || true
    done
    # Deepest-first, and rmdir (never rm -rf) so a directory that unexpectedly
    # still holds something is left alone and reported rather than destroyed.
    local d
    for d in $(printf '%s\n' "${CREATED_DIRS[@]:-}" | awk 'NF' | awk '{print length"\t"$0}' | sort -rn | cut -f2-); do
        rmdir "${REPO_ROOT}/${d}" 2>/dev/null || true
    done
    TOUCHED=()
    CREATED=()
    CREATED_DIRS=()
}

# ---------------------------------------------------------------------------
# Provocations. One per gate. Each plants a violation the gate claims to catch.
#
# Keep them MINIMAL and OBVIOUS. A provocation that is cleverer than the gate
# tests the provocation, not the gate.
# ---------------------------------------------------------------------------

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_gosec_ratchet() {
    # Contract: workflow exclusions must be a subset of the baseline. Append a
    # rule that is not baselined — the "make a red build go green" move.
    local wf=".github/workflows/security.yml"
    backup "${wf}"
    # The exclusion list is currently EMPTY — every rule has been remediated,
    # which is the ratchet working. So there is no `-exclude=G...` to append
    # to; the provocation has to introduce one, which is exactly the move the
    # ratchet exists to block: adding a suppression to turn a red build green.
    #
    # G000 is not a real gosec rule, so it can never legitimately appear in the
    # baseline. If a future edit reintroduces a real -exclude= list, this still
    # works — sed inserts into the gosec command line either way.
    sed -i "s|gosec\"\? -fmt sarif -out gosec-results.sarif |&-exclude=G000 |" "${REPO_ROOT}/${wf}"
    grep -q -- '-exclude=G000' "${REPO_ROOT}/${wf}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_compose_hardening() {
    # Contract (ADR-097, UH-CTR-01): every compose service is least-privilege
    # unless baselined. Drop read_only from a first-party service.
    backup "docker-compose.yml"
    python3 - "${REPO_ROOT}/docker-compose.yml" <<'PROBE'
import re, sys
p = sys.argv[1]
s = open(p).read()
i = s.index("\n  dashboard-backend:\n")
head, tail = s[:i], s[i:]
tail = re.sub(r"\n    read_only: true\n", "\n", tail, count=1)
open(p, "w").write(head + tail)
PROBE
    ! grep -A12 '^  dashboard-backend:' "${REPO_ROOT}/docker-compose.yml" | grep -q 'read_only: true'
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_manifest_yaml() {
    # Contract: every tracked YAML manifest parses. Plant a file that cannot.
    local f="deploy/k8s/policies/.meta-gate-probe.yaml"
    backup "${f}"
    printf 'a: "unterminated\nb: [1, 2\n' > "${REPO_ROOT}/${f}"
    git -C "${REPO_ROOT}" add -N "${f}" >/dev/null 2>&1  # gate reads git ls-files
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_secrets_baseline() {
    # Contract: no fingerprint may APPEAR that is not already in the manifest.
    backup ".gitleaksignore"
    echo 'meta/gate/probe.go:generic-api-key:1' >> "${REPO_ROOT}/.gitleaksignore"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_python_syntax() {
    # Contract: every tracked .py compiles.
    local f
    f="$(git -C "${REPO_ROOT}" ls-files '*.py' | head -1)"
    [ -n "${f}" ] || return 1
    backup "${f}"
    printf '\ndef meta_gate_probe(:\n' >> "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_clippy() {
    # Contract: zero clippy warnings, and any workspace that fails to build is
    # an error. Planting a missing-doc violation in a crate that denies it is
    # the cheapest reliable trip.
    local f="crates/upc-api/src/lib.rs"
    backup "${f}"
    cat >> "${REPO_ROOT}/${f}" <<'PROBE'

pub fn meta_gate_probe(s: &String) -> usize {
    s.len()
}
PROBE
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_timeline_freshness() {
    # Contract: timeline.md must be touched within MAX_AGE_DAYS of HEAD.
    # This one needs NO tree mutation — the threshold is an env knob, so a
    # threshold of -1 days must always be violated.
    :
}

# Env-only provocations declare how to invoke the gate under provocation.
# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
run_timeline_freshness() {
    MAX_AGE_DAYS=-1 "${REPO_ROOT}/scripts/check-timeline-freshness.sh" --check
}


# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_bpf_verifier_check() {
    # Contract: every BPF program compiles. Plant an undefined symbol.
    local f="ebpf/flow-tracker/src/main.rs"
    backup "${f}"
    printf '\nfn meta_gate_probe() -> u32 { this_symbol_does_not_exist() }\n' >> "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_verify_gpl_boundary() {
    # Contract: no GPL/AGPL license on a non-first-party Cargo.toml.
    # NOT under vendor/. A vendor/ directory at the module root puts Go into
    # vendoring mode, and an empty one left behind breaks `go build ./...` with
    # "inconsistent vendoring" for every dependency. third_party/ is
    # non-first-party as far as is_first_party_cargo() is concerned, which is
    # all this provocation needs, and Go ignores it entirely.
    local f="third_party/meta-gate-probe/Cargo.toml"
    backup_dir "third_party/meta-gate-probe"
    backup_dir "third_party"
    mkdir -p "${REPO_ROOT}/third_party/meta-gate-probe"
    backup "${f}"
    printf '[package]\nname = "meta-gate-probe"\nversion = "0.0.0"\nlicense = "AGPL-3.0"\n' \
        > "${REPO_ROOT}/${f}"
}


# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_ruff() {
    # Contract: ruff reports zero findings, ratcheted by rule ID.
    # F841 (assigned-but-never-used) is default-on, unambiguous, and cannot be
    # mistaken for a style preference. Must NOT go under
    # crates/xv6-mbc/upstream, which the gate excludes as vendored.
    local f
    f="$(git -C "${REPO_ROOT}" ls-files '*.py' \
        | grep -v '^crates/xv6-mbc/upstream/' | head -1)"
    [ -n "${f}" ] || return 1
    backup "${f}"
    cat >> "${REPO_ROOT}/${f}" <<'PROBE'


def _meta_gate_probe():
    unused_local = 42
    return None
PROBE
}

# ---------------------------------------------------------------------------
# Registry: gate basename -> provoke fn : speed : what the provocation plants
# ---------------------------------------------------------------------------
# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_compose_log_caps() {
    # Contract (ADR-092): every compose service declares its own log cap.
    # Strip one service's logging block.
    backup "docker-compose.yml"
    python3 - "${REPO_ROOT}/docker-compose.yml" <<'PROBE'
import re, sys
p = sys.argv[1]
s = open(p).read()
# Drop the first service-level logging: block.
s = re.sub(r'\n    logging:\n(?:      .*\n)+', '\n', s, count=1)
open(p, 'w').write(s)
PROBE
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_haproxy_cfg() {
    # Contract (S-CLEAN-SLATE M-035): both haproxy configs pass `haproxy -c`.
    # Restore the original bug: the internal config logging X-Trace-Id with
    # req.hdr(), which is unavailable when the log line is built (ALERT).
    backup "docker/haproxy/internal/haproxy.cfg"
    python3 - "${REPO_ROOT}/docker/haproxy/internal/haproxy.cfg" <<'PROBE'
import sys
p = sys.argv[1]
s = open(p).read()
s = s.replace("%[var(txn.trace_id),json(utf8s)]", "%[req.hdr(X-Trace-Id)]", 1)
open(p, 'w').write(s)
PROBE
    grep -q '%\[req.hdr(X-Trace-Id)\]' "${REPO_ROOT}/docker/haproxy/internal/haproxy.cfg"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_compose_bind_nesting() {
    # Contract (ADR-091): no bind mount inside another's mountpoint. Recreate
    # the original initdb nesting this ADR was written about.
    backup "docker-compose.yml"
    python3 - "${REPO_ROOT}/docker-compose.yml" <<'PROBE'
import sys
p = sys.argv[1]
s = open(p).read()
anchor = "      - ./db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro"
assert anchor in s, "postgres initdb mount not found"
s = s.replace(anchor, anchor + "\n      - ./db/migrations:/docker-entrypoint-initdb.d:ro", 1)
open(p, 'w').write(s)
PROBE
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_tmp_log_baseline() {
    # Contract (ADR-092): the /tmp log path SET may only shrink. Add one that
    # is not in the baseline. A swap would also fail -- that is the point of
    # comparing the set rather than a count -- but an addition is the simplest
    # violation to plant and restore.
    local f="scripts/demo-reset.sh"
    backup "${f}"
    # Assembled at runtime: check-tmp-log-baseline.sh scans every tracked
    # *.sh, including this one, so writing the literal here would make this
    # script a violation of the gate it is provoking.
    local probe="/tmp/meta-gate-probe"
    printf '\n: > %s.log\n' "${probe}" >> "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_live_path_inventory() {
    # Contract (ADR-093 rule 5): docs/LIVE-PATHS.md matches the tree, so a new
    # binary cannot land unclassified. Plant one OUTSIDE cmd/: a glob over
    # cmd/*/ caught a probe there while missing 14 real main packages
    # elsewhere, so a cmd/ probe proved nothing about enumeration.
    local d="pkg/meta-gate-probe-main"
    mkdir -p "${REPO_ROOT}/${d}"
    CREATED_DIRS+=("${d}")
    local f="${d}/main.go"
    backup "${f}"
    printf 'package main\n\nfunc main() {}\n' > "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_ebpf_loads() {
    # Contract: every eBPF program not on the known-failing list passes the
    # kernel verifier. Plant a REAL verifier rejection, not a list edit: drop
    # the black_box that keeps canary's bounds check alive (aa9260e7), so the
    # gate has to build, load and see the kernel refuse it.
    local f="ebpf/canary-ebpf/src/main.rs"
    backup "${f}"
    local needle="    let data_end = core::hint::black_box(data_end);"
    grep -qxF "${needle}" "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
    grep -vxF "${needle}" "${BACKUP_DIR}/$(echo "${f}" | tr '/' '_')" > "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_prometheus_rules() {
    # Contract: every alert rule parses and its tests pass. Plant a threshold
    # edit that parses fine but no longer fires on the case its test pins.
    local f="monitoring/prometheus/rules/host.yml"
    backup "${f}"
    sed -i 's|expr: host_disk_avail_bytes / host_disk_total_bytes < 0.10$|expr: host_disk_avail_bytes / host_disk_total_bytes < 0.01|' "${REPO_ROOT}/${f}"
    grep -q 'host_disk_total_bytes < 0.01$' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_no_client_golang() {
    # Contract (ADR-094 step 6): nothing imports the client_golang family.
    # Plant one import in a fresh package. Untracked on purpose: the gate
    # must catch a file before it is committed.
    local d="pkg/meta-gate-probe-metrics"
    mkdir -p "${REPO_ROOT}/${d}"
    CREATED_DIRS+=("${d}")
    local f="${d}/probe.go"
    backup "${f}"
    printf 'package probe\n\nimport _ "github.com/prometheus/client_golang/prometheus"\n' > "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_migrations() {
    # Contract (ADR-101, W-09): no migration file escapes the manifest.
    # Plant an unrouted one, untracked, as a new migration would arrive.
    local f="db/migrations/099_meta_gate_probe.sql"
    backup "${f}"
    printf 'SELECT 1;\n' > "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_k8s_well_initdb() {
    # Contract (ADR-101): the ConfigMap is db/, rendered. Edit a migration
    # without re-rendering.
    local f="db/migrations/018_config_bindings_profile_index.sql"
    backup "${f}"
    printf -- '-- edited without re-rendering\n' >> "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_kanban_genesis() {
    # Contract (S-TWO-BRAINS Step 421): 034 is the JSON, rendered. Edit a
    # card in the JSON without re-rendering.
    local f="db/seed/kanban-genesis.json"
    backup "${f}"
    sed -i 's/"Crest (Kanban Frontend)"/"Crest (Kanban Frontend, edited)"/' "${REPO_ROOT}/${f}"
    grep -q 'Kanban Frontend, edited' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_actions_pinned() {
    # Contract (ADR-095): no action on a mutable tag. Unpin checkout in
    # security.yml's first job.
    local f=".github/workflows/security.yml"
    backup "${f}"
    sed -i '0,/uses: actions\/checkout@[0-9a-f]\{40\}.*/s//uses: actions\/checkout@v4/' "${REPO_ROOT}/${f}"
    grep -q 'uses: actions/checkout@v4$' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_compliance_schema() {
    # Contract (W-05): history tables refuse TRUNCATE even for the owner.
    # Drop audit_events' TRUNCATE guard from 015.
    local f="db/migrations/015_ops_history_append_only.sql"
    backup "${f}"
    sed -i '/^CREATE OR REPLACE TRIGGER trg_audit_events_no_truncate$/,/EXECUTE FUNCTION well_append_only();$/d' "${REPO_ROOT}/${f}"
    ! grep -q 'trg_audit_events_no_truncate' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_migrate() {
    # Contract (ADR-101): apply refuses to run past drift. Remove the check.
    local f="pkg/database/migrate/runner.go"
    backup "${f}"
    python3 - "${REPO_ROOT}/${f}" <<'PROBE'
import sys
p = sys.argv[1]
s = open(p).read()
old = "\tif err := p.Drift(); err != nil {\n\t\treturn err\n\t}\n\tmode := \"apply\""
if old not in s:
    sys.exit("anchor missing")
open(p, "w").write(s.replace(old, "\tmode := \"apply\""))
PROBE
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_backup_restore() {
    # Contract (W-01): a backup set holds every database. Leave one out.
    local f="scripts/db-backup.sh"
    backup "${f}"
    sed -i "s/WHERE datallowconn AND NOT datistemplate ORDER BY 1/WHERE datallowconn AND NOT datistemplate AND datname <> 'unheaded_ops' ORDER BY 1/" "${REPO_ROOT}/${f}"
    grep -q "datname <> 'unheaded_ops'" "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_offsite_check() {
    # Contract (ERDA 1.2): a set on east without SHA256SUMS is BAD. Count
    # it as good again, as the check once did by skipping it.
    local f="scripts/well/offsite-check.sh"
    backup "${f}"
    sed -i 's|elif \[\[ -f "\$set_dir/SHA256SUMS" \]\] && (cd|elif [[ ! -f "$set_dir/SHA256SUMS" ]] \|\| (cd|' "${REPO_ROOT}/${f}"
    grep -q 'elif \[\[ ! -f "\$set_dir/SHA256SUMS" \]\] || (cd' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_lint() {
    # Contract (ERDA 3.5): no new schema finding. Plant service code that
    # runs DDL at startup (ddl-outside-ledger), tracked as new code would be.
    local f="pkg/database/zz_meta_gate_probe.go"
    backup "${f}"
    printf 'package database\n\nimport "database/sql"\n\nfunc metaGateProbe(db *sql.DB) { _, _ = db.Exec("CREATE TABLE probe (id int)") }\n' > "${REPO_ROOT}/${f}"
    git -C "${REPO_ROOT}" add -N "${f}" >/dev/null 2>&1  # gate reads git ls-files
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_config() {
    # Contract (ERDA 4.2): service roles reach The Well over TLS only.
    local f="db/pg_hba.conf"
    backup "${f}"
    sed -i 's|^hostssl  all       all       172.28.1.0/24    scram-sha-256|host     all       all       172.28.1.0/24    scram-sha-256|' "${REPO_ROOT}/${f}"
    grep -q '^host     all       all       172.28.1.0/24' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_pq_groups() {
    # Contract (ADR-105): the Well on PG 18 negotiates X25519MLKEM768.
    # Drop the hybrid from the include, leaving only classical groups.
    local f="db/postgresql.pg18.conf.d/10-pq-groups.conf"
    backup "${f}"
    sed -i "s|^ssl_groups = 'X25519MLKEM768:|ssl_groups = '|" "${REPO_ROOT}/${f}"
    grep -q "^ssl_groups = 'X25519:prime256v1'$" "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_llm_requests() {
    # Contract (S-TWO-BRAINS Step 36): llm_requests is append-only even for
    # its owner. Drop the UPDATE/DELETE trigger from 030.
    local f="db/migrations/030_ops_llm_requests.sql"
    backup "${f}"
    sed -i '/^CREATE OR REPLACE TRIGGER trg_llm_requests_append_only$/,/EXECUTE FUNCTION well_append_only();$/d' "${REPO_ROOT}/${f}"
    ! grep -q 'trg_llm_requests_append_only' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_zhen_confirm() {
    # Contract (S-TWO-BRAINS Step 67): confirmation states move one way.
    # Drop the transition trigger from 031.
    local f="db/migrations/031_app_zhen_pending_confirmations.sql"
    backup "${f}"
    sed -i '/^CREATE OR REPLACE TRIGGER trg_zhen_confirm_transition/,/EXECUTE FUNCTION zhen_confirm_transition();$/d' "${REPO_ROOT}/${f}"
    ! grep -q 'TRIGGER trg_zhen_confirm_transition' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_zhen_agent_runs() {
    # Contract (S-TWO-BRAINS Step 202): one active run per card. Drop the
    # partial unique index from 032, so a claim race has several winners.
    local f="db/migrations/032_app_zhen_agent_runs.sql"
    backup "${f}"
    sed -i '/zhen_agent_runs_one_active/d' "${REPO_ROOT}/${f}"
    ! grep -q 'zhen_agent_runs_one_active' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_zhen_captures() {
    # Contract (S-TWO-BRAINS Step 346): re-labelling is idempotent, one
    # label per (capture, kind, source row). Turn the UNIQUE into a no-op.
    local f="db/migrations/033_app_zhen_captures.sql"
    backup "${f}"
    sed -i 's/CONSTRAINT zhen_capture_labels_once UNIQUE (capture_id, kind, source_ref)/CONSTRAINT zhen_capture_labels_once CHECK (true)/' "${REPO_ROOT}/${f}"
    grep -q 'zhen_capture_labels_once CHECK (true)' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_well_api_keys() {
    # Contract (S-IRON-GATE Step 105, ADR-103 §5): every API key expires
    # within 366 days of creation. Turn the cap into a no-op.
    local f="db/migrations/036_app_api_keys.sql"
    backup "${f}"
    sed -i "s/CHECK (expires_at <= created_at + interval '366 days')/CHECK (true)/" "${REPO_ROOT}/${f}"
    if grep -q "interval '366 days'" "${REPO_ROOT}/${f}"; then echo "provocation anchor missing in ${f}" >&2; return 1; fi
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_systemd_units() {
    # Contract (ADR-098 FND-003): systemd ignores nothing in deploy/systemd.
    # Restore the original mistake: a `~` on every group, of which systemd
    # honours only the first.
    local f="deploy/systemd/unheaded-sophia.service"
    backup "${f}"
    sed -i 's/^SystemCallFilter=~@privileged @resources @reboot @module @swap$/SystemCallFilter=~@privileged ~@resources ~@reboot ~@module ~@swap/' "${REPO_ROOT}/${f}"
    grep -q '^SystemCallFilter=~@privileged ~@resources' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_git_mirror() {
    # Contract (FND-005): the mirror never loses a ref west deleted. Switch
    # to push --mirror, which prunes them.
    local f="scripts/backup/git-mirror.sh"
    backup "${f}"
    sed -i "s|push --quiet --force \"\$DEST\" '+refs/\*:refs/\*'|push --quiet --mirror \"\$DEST\"|" "${REPO_ROOT}/${f}"
    grep -q 'push --quiet --mirror' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_rotate_psk() {
    # Contract (ADR-105): a rotation that gets no handshake puts both hosts
    # back on the old PSK. Make the rollback a no-op.
    local f="scripts/wireguard/rotate-psk.sh"
    backup "${f}"
    sed -i 's/^rollback() {$/rollback() { return 0/' "${REPO_ROOT}/${f}"
    grep -q '^rollback() { return 0$' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_hadolint() {
    # Contract: no new Dockerfile finding lands unrecorded. A RUN that cd's
    # instead of using WORKDIR (DL3003) in the root Dockerfile.
    local f="Dockerfile"
    backup "${f}"
    printf '\nRUN cd /tmp && ls\n' >>"${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_vector_config() {
    # Contract (ADR-100 Phase 3): the log-volume/auth SLIs Vector builds are
    # pinned by config/vector/tests. Break the sudo failure pattern.
    local f="config/vector/vector.yaml"
    backup "${f}"
    sed -i 's|incorrect password attempts?|incorrect passwd attempts?|' "${REPO_ROOT}/${f}"
    grep -q 'incorrect passwd attempts?' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_lich_cargo_fuzz() {
    # Contract (ADR-062): a Rust libFuzzer target that crashes fails the run.
    # Make fuzz_mbc_decode panic on every input.
    local f="crates/monad-mbc/fuzz/fuzz_targets/fuzz_mbc_decode.rs"
    backup "${f}"
    sed -i 's#^fuzz_target!(|data: &\[u8\]| {$#&\n    panic!("lich meta-gate provocation {}", data.len());#' "${REPO_ROOT}/${f}"
    grep -q 'lich meta-gate provocation' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_deb_maintscripts() {
    # Contract (ADR-085 Phase 2): an apt upgrade leaves the service enabled.
    # Make prerm stop and disable on every call, as it used to.
    local f="scripts/lib/deb-maintscripts.sh"
    backup "${f}"
    sed -i 's/^remove|deconfigure)$/*)/' "${REPO_ROOT}/${f}"
    grep -q '^\*)$' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_dependency_baseline() {
    # Contract (ADR-096 §3): no package enters the build unrecorded. Add one
    # to a tracked requirements file, as an assistant-suggested import would.
    local f="raft/requirements.txt"
    backup "${f}"
    printf 'meta-gate-probe-package==0.0.1\n' >>"${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_unit_binaries() {
    # Contract (ADR-085): a unit runs only binaries build-binaries.sh builds.
    local f="deploy/systemd/unheaded-akira.service"
    backup "${f}"
    sed -i 's|^ExecStart=/usr/local/bin/akira |ExecStart=/usr/local/bin/akira-unbuilt |' "${REPO_ROOT}/${f}"
    grep -q '^ExecStart=/usr/local/bin/akira-unbuilt ' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_python_audit() {
    # Contract (ADR-096 §3): no unaccepted advisory in pinned Python
    # requirements. Pin a release with published advisories.
    local f="raft/requirements.txt"
    backup "${f}"
    printf 'jinja2==2.10\n' >>"${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_js_lint() {
    # Contract (ADR-006): JavaScript is linted and a finding fails. Restore
    # the bug the first lint run found: a call to a function defined only
    # inside another file's IIFE (no-undef).
    local f="kanban/js/task-modal.js"
    backup "${f}"
    printf '\nfunction provokeSave() { return getFriendlyErrorMessage(new Error("x")); }\nwindow.provokeSave = provokeSave;\n' >>"${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_gitleaks() {
    # Contract: a new secret in a tracked file fails. Plant an AWS access key
    # ID, assembled here so this script never contains one itself.
    local f="docs/compliance/SECRETS-SCAN.md"
    backup "${f}"
    printf '\naws_access_key_id = %s%s\n' "AKIA" "Q7ZK3M5R2TXW4P6N" >>"${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_helm_security() {
    # Contract: every container the chart renders has a read-only root
    # filesystem. Flip the chart-wide default.
    local f="helm/unheaded/values.yaml"
    backup "${f}"
    sed -i 's/readOnlyRootFilesystem: true/readOnlyRootFilesystem: false/' "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_tool_freshness() {
    # Contract: release-pinned tools are not behind a release past its 7-day
    # cooldown. Pin oxlint back to a long-superseded release.
    local f="scripts/lib/tool-pins.sh"
    backup "${f}"
    sed -i 's/^OXLINT_VERSION="[^"]*"/OXLINT_VERSION="1.0.0"/' "${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_alert_path() {
    # Contract (ADR-100): an urgency none alert reaches no one. Route
    # Alertmanager's default (urgency none, unlabelled) to the page receiver.
    local f="monitoring/alertmanager/alertmanager.yml"
    backup "${f}"
    python3 - "${REPO_ROOT}/${f}" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()
old = "route:\n  receiver: none\n"
if old not in s:
    sys.exit("provocation anchor missing")
open(p, "w").write(s.replace(old, "route:\n  receiver: page\n"))
PY
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_image_tags() {
    # Contract (ADR-102): no image enters at a floating tag.
    local f="docker-compose.yml"
    backup "${f}"
    sed -i '0,/image: traefik:v3.3.7/s//image: traefik:latest/' "${REPO_ROOT}/${f}"
    grep -q 'image: traefik:latest' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_evidence_pack() {
    # Contract (ADR-105, UH-EVID-01): a manifest reporting a CRITICAL CVE never
    # validates, is never signed, and never verifies. Flip the valid fixture
    # every E2E pack is built from.
    local f="cmd/yggdrasil-evidence/testdata/manifests/valid/minimal.yaml"
    backup "${f}"
    sed -i 's/^  critical: 0$/  critical: 1/' "${REPO_ROOT}/${f}"
    grep -q '^  critical: 1$' "${REPO_ROOT}/${f}"
}

provoke_crypto_inventory() {
    # Contract (ADR-105 decision 4): the checked-in CBOM is exactly what the
    # tree regenerates. Append one component the tree does not contain.
    local f="compliance/crypto/cbom.cdx.json"
    backup "${f}"
    jq '.components += [{"type":"cryptographic-asset","bom-ref":"meta-gate:1:probe","name":"meta-gate probe","cryptoProperties":{"assetType":"protocol","protocolProperties":{"type":"tls"}},"properties":[]}]' \
        "${BACKUP_DIR}/$(echo "${f}" | tr '/' '_')" >"${REPO_ROOT}/${f}" || return 1
    grep -q 'meta-gate:1:probe' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_pq_kex() {
    # Contract (ADR-105): no new classical-only key exchange. Plant an
    # untracked Go file pinning CurvePreferences to X25519 only; the lint sees
    # untracked files, so it must fail before the file is ever committed.
    local f="pkg/tls/zz_meta_gate_pq.go"
    backup "${f}"
    printf 'package tls\n\nimport stdtls "crypto/tls"\n\nvar metaGateProbe = &stdtls.Config{MinVersion: stdtls.VersionTLS13, CurvePreferences: []stdtls.CurveID{stdtls.X25519}}\n' >"${REPO_ROOT}/${f}"
}

# shellcheck disable=SC2317  # invoked indirectly via REGISTRY dispatch
provoke_runbooks() {
    # Contract (ADR-024, S-TWO-BRAINS Phase 8): every runbook follows runbook/v1
    # unless its problem is baselined. Give a clean runbook a risk level the
    # schema does not have.
    local f="runbooks/observe/service-health-sweep.yaml"
    backup "${f}"
    sed -i 's/^  risk: low$/  risk: meta-gate-probe/' "${REPO_ROOT}/${f}"
    grep -q '^  risk: meta-gate-probe$' "${REPO_ROOT}/${f}" || { echo "provocation anchor missing in ${f}" >&2; return 1; }
}

REGISTRY="
check-gosec-ratchet|provoke_gosec_ratchet|fast|an un-baselined rule appended to the workflow exclusion list
check-manifest-yaml|provoke_manifest_yaml|fast|a tracked manifest that does not parse
check-secrets-baseline|provoke_secrets_baseline|fast|a new fingerprint appended to .gitleaksignore
check-python-syntax|provoke_python_syntax|fast|a syntax error in a tracked .py file
check-timeline-freshness|provoke_timeline_freshness|fast|MAX_AGE_DAYS=-1, which nothing can satisfy
check-ruff|provoke_ruff|fast|an F841 unused local in a tracked, non-vendored .py
check-clippy|provoke_clippy|slow|a clippy violation in crates/upc-api
bpf-verifier-check|provoke_bpf_verifier_check|slow|an undefined symbol in ebpf/flow-tracker
verify-gpl-boundary|provoke_verify_gpl_boundary|fast|an AGPL license on a non-first-party Cargo.toml
check-compose-log-caps|provoke_compose_log_caps|fast|a compose service with its logging block stripped
check-haproxy-cfg|provoke_haproxy_cfg|slow|req.hdr(X-Trace-Id) restored in the internal haproxy config's log-format (needs docker)
check-compose-hardening|provoke_compose_hardening|fast|dashboard-backend with read_only removed
check-systemd-units|provoke_systemd_units|fast|the ignored per-group ~ restored in unheaded-sophia.service
check-compose-bind-nesting|provoke_compose_bind_nesting|fast|ADR-091's original initdb bind nesting, recreated
check-tmp-log-baseline|provoke_tmp_log_baseline|fast|a /tmp log path not present in the baseline set
live-path-inventory|provoke_live_path_inventory|fast|a new main package outside cmd/ absent from docs/LIVE-PATHS.md
check-no-client-golang|provoke_no_client_golang|fast|an untracked Go file importing prometheus/client_golang
check-well-migrations|provoke_well_migrations|fast|an untracked migration file the MANIFEST does not route
check-well-lint|provoke_well_lint|fast|service code that runs CREATE TABLE at startup (a new ddl-outside-ledger finding)
check-k8s-well-initdb|provoke_k8s_well_initdb|fast|a migration edited without re-rendering the Kubernetes initdb ConfigMap
check-kanban-genesis|provoke_kanban_genesis|fast|a Genesis card title edited in db/seed/kanban-genesis.json without re-rendering migration 034
check-actions-pinned|provoke_actions_pinned|fast|actions/checkout unpinned back to @v4 in security.yml
check-dependency-baseline|provoke_dependency_baseline|fast|an unrecorded package added to raft/requirements.txt
check-unit-binaries|provoke_unit_binaries|fast|unheaded-akira.service pointed at a binary nothing builds
check-image-tags|provoke_image_tags|fast|traefik:v3.3.7 floated back to traefik:latest in docker-compose.yml
check-crypto-inventory|provoke_crypto_inventory|fast|a component the tree does not contain appended to compliance/crypto/cbom.cdx.json
check-pq-kex|provoke_pq_kex|fast|an untracked Go file pinning CurvePreferences to X25519 only (classical key exchange)
check-python-audit|provoke_python_audit|fast|jinja2==2.10 (five published advisories) pinned in raft/requirements.txt
check-js-lint|provoke_js_lint|fast|the first lint run's real bug restored: a call to a function another file's IIFE hides (no-undef)
check-gitleaks|provoke_gitleaks|fast|an AWS access key ID appended to a tracked doc (downloads the pinned gitleaks once)
check-helm-security|provoke_helm_security|fast|the chart-wide readOnlyRootFilesystem flipped to false in values.yaml (downloads the pinned Helm once)
check-tool-freshness|provoke_tool_freshness|fast|oxlint pinned back to 1.0.0, long past the 7-day cooldown (needs the GitHub API)
check-hadolint|provoke_hadolint|slow|a RUN that cd's (DL3003) appended to the root Dockerfile (needs docker)
test-deb-maintscripts|provoke_deb_maintscripts|fast|prerm disabling the service on every call, upgrades included
test-vector-config|provoke_vector_config|slow|the sudo auth-failure pattern in config/vector/vector.yaml edited so its test no longer matches (needs docker)
lich-cargo-fuzz|provoke_lich_cargo_fuzz|slow|fuzz_mbc_decode panicking on every input (nightly Rust; LICH_FUZZ_SECONDS=5 shortens it)
backup/test-git-mirror|provoke_git_mirror|fast|git-mirror.sh switched to push --mirror, which prunes refs east must keep
wireguard/test-rotate-psk|provoke_rotate_psk|fast|the rollback branch of rotate-psk.sh made a no-op
well/test-compliance-schema|provoke_well_compliance_schema|slow|audit_events' TRUNCATE guard removed from 015 (needs docker)
well/test-migrate|provoke_well_migrate|slow|the runner's drift refusal removed, so apply runs past an edited file (needs docker)
well/test-backup-restore|provoke_well_backup_restore|slow|db-backup.sh leaving unheaded_ops out of the set (needs docker)
well/test-offsite-check|provoke_well_offsite_check|fast|offsite-check.sh counting a set without SHA256SUMS as good
well/test-config|provoke_well_config|slow|pg_hba letting service roles in without TLS (needs docker)
well/test-pq-groups|provoke_well_pq_groups|slow|the hybrid group removed from the PG18 ssl_groups include (needs docker)
well/test-llm-requests|provoke_well_llm_requests|slow|llm_requests' UPDATE/DELETE trigger removed from 030 (needs docker)
well/test-zhen-confirm|provoke_well_zhen_confirm|slow|the one-way transition trigger removed from 031 (needs docker)
well/test-zhen-agent-runs|provoke_well_zhen_agent_runs|slow|the one-active-run index removed from 032, so a claim race has several winners (needs docker)
well/test-zhen-captures|provoke_well_zhen_captures|slow|the one-label-per-source UNIQUE on zhen_capture_labels turned into CHECK (true) in 033 (needs docker)
well/test-api-keys|provoke_well_api_keys|slow|the 366-day expiry cap on api_keys turned into CHECK (true) in 036 (needs docker)
check-ebpf-loads|provoke_ebpf_loads|slow|canary-ebpf with its bounds-check black_box removed, a real verifier rejection (needs sudo)
check-prometheus-rules|provoke_prometheus_rules|slow|HostDiskAlmostFull's threshold edited so its tested case no longer fires (needs docker)
monitoring/test-alert-path|provoke_alert_path|slow|Alertmanager's default route (urgency none) sent to the page receiver (needs docker)
check-evidence-pack|provoke_evidence_pack|fast|a valid fixture manifest edited to report one CRITICAL CVE
check-runbooks|provoke_runbooks|fast|a clean runbook given a risk level runbook/v1 does not have
check-gates-can-fail|SELF|self|this script — see the self-exemption note
"

# ---------------------------------------------------------------------------
# NOT a gate. These are referenced by CI/runbooks but produce an ARTIFACT or
# perform an ACTION; they have no pass/fail contract to provoke. Listed
# explicitly rather than filtered by a pattern, so adding one is a decision
# someone made on purpose.
# ---------------------------------------------------------------------------
NOT_A_GATE="
generate-sbom|emits an SBOM; failure surfaces in the jobs that consume it
build-all-debs|produces .deb artifacts
build-sealed-cask|produces a sealed image
kanban-git-link|writes kanban links, no verdict
drift-detect|reports drift; alerts-only by design (pkg/enkrateia contract)
qa-smoke|prints a score for a human to compare against the previous batch
ci-fetch-doomgeneric|fetches a pinned checkout the Go jobs need; no verdict of its own
ci-python-lock|regenerates .github/ci-python/requirements.txt (uv in a throwaway container); no verdict of its own: check-tool-freshness and the --require-hashes installs are the gates
ci-install-tools|installs pinned syft/grype/cosign/helm/kubeconform for CI jobs (lib/tool-pins + lib/fetch-tool, which the check-* scripts also use); no verdict of its own
lib/logpath|prints the durable log path for a service (ADR-092); no verdict of its own
lib/deb-maintscripts|generates the .deb maintainer scripts (build-all-debs, Jenkinsfile); proven by test-deb-maintscripts
well/set-role-passwords|sets role passwords from the environment (runbooks); no verdict of its own
lib/fetch-tool|library sourced by ci-install-tools and the check-* scripts (download once, sha256 on every use); no verdict of its own
verify-release|verifies a published release by tag (needs the network and a real release); its pass/fail contract is proven offline by tests/release/verify_release_test.sh (CI job release-verify)
well/make-tls|makes TLS material outside the repo; exercised by well/test-config, no verdict of its own
db-backup|writes a backup set (runbooks/data/postgresql-backup-restore.yaml); its refusals and the set it writes are proven by well/test-backup-restore
well/restore-drill|needs a backup set to run; its refusals are provoked by well/test-backup-restore (flipped byte, wrong count, unlisted dump)
monitoring/maintenance|creates or ends an Alertmanager silence for planned work (ADR-100); no verdict of its own
build-binaries|builds the unit binaries (make binaries, build-all-debs); its --check-units verdict is check-unit-binaries
wireguard/rotate-psk|rotates the WireGuard PSK on west/east (dry run unless --apply); its rollback and refusals are proven by wireguard/test-rotate-psk
well/test-converge-live-app|compares the live Well (or a restored set) with a fresh bootstrap; needs that Well, runbook verify step
probe-tls-groups|prints the negotiated group for an endpoint; the Go handshake tests (pkg/tls/pqtest, ADR-105) are the gate
zhen-mcp|launches the Zhen MCP server for Claude Code (.mcp.json, runbooks/security/mcp-write-exec-enablement.yaml); no verdict of its own
"

# ---------------------------------------------------------------------------
# Gates we CANNOT provoke here. Each needs sudo, a live BPF/XDP attach, or real
# hardware, so a sandbox result would be meaningless.
#
# Listed, never silently skipped — that distinction is the whole point. If one
# of these becomes runnable in CI, move it into REGISTRY with a provocation.
# ---------------------------------------------------------------------------
NEEDS_HARDWARE="
ascend-linux-smoke|attaches XDP and boots the UPC; needs sudo + a live kernel
check-ebpf-size-pin|needs the build host's pinned Rust nightly (CI's floats: byte sizes are not comparable there); proven by hand 2026-10-27: size edit exit 1, other toolchain exit 2
doom-smoke|drives BPF maps via bpftool under sudo
doom-ring|reads a live ring buffer from a running instance
verify-binding-rune|verifies a sealed-cask image that must be built first
yggdrasil-verify-anchor|verifies a Debian anchor image not present in the sandbox
yggdrasil-verify-overlay|verifies a built overlay image not present in the sandbox
"

list_lookup() {
    printf '%s\n' "$2" | awk -F'|' -v k="$1" '$1==k {print $2; exit}'
}

# Every scripts/*.sh a workflow, Jenkinsfile, runbook or smoke script invokes,
# plus every check-*.sh by convention.
#
# Discovery is deliberately NOT a filename glob. The first version globbed
# scripts/check-*.sh, which meant bpf-verifier-check.sh — a real gate, trusted
# by ascend-linux-smoke.sh and by the post-kernel-upgrade runbook — was
# invisible to it. It had exactly the defect this script exists to catch: cargo
# could exit 101 and it still printed "GATE: PASSED". Naming is a convention;
# what CI and the runbooks actually run is the fact.
discover_candidates() {
    {
        grep -rhoE '(\./)?scripts/[A-Za-z0-9._/-]+\.sh' \
            .github/workflows/ Jenkinsfile jenkins/ runbooks/ scripts/*smoke*.sh 2>/dev/null \
            | sed 's|^\./||' | sed 's|^scripts/||' | sed 's|\.sh$||'
        find scripts -maxdepth 1 -name 'check-*.sh' -printf '%f\n' 2>/dev/null | sed 's|\.sh$||'
    } | sort -u
}

registry_lookup() {
    printf '%s\n' "${REGISTRY}" | awk -F'|' -v k="$1" '$1==k {print; exit}'
}

if [ "${MODE}" = "--list" ]; then
    echo "Registered provocations:"
    printf '%s\n' "${REGISTRY}" | awk -F'|' 'NF>=4 {printf "  %-26s %-5s %s\n", $1, $3, $4}'
    exit 0
fi

echo "============================================================"
echo "  META-GATE: proving every gate can actually fail"
echo "============================================================"
echo

# ---------------------------------------------------------------------------
# Coverage FIRST, before the dirty-tree refusal.
#
# This check mutates nothing, and a newly added gate arrives UNTRACKED — so
# refusing on a dirty tree first would hide the one message the author of that
# gate most needs to see. Order matters: read-only checks before safety
# refusals that exist to protect mutation.
# ---------------------------------------------------------------------------
UNREGISTERED=""
for name in $(discover_candidates); do
    [ -f "scripts/${name}.sh" ] || continue          # referenced but absent elsewhere
    [ -n "$(registry_lookup "${name}")" ] && continue
    [ -n "$(list_lookup "${name}" "${NOT_A_GATE}")" ] && continue
    [ -n "$(list_lookup "${name}" "${NEEDS_HARDWARE}")" ] && continue
    UNREGISTERED="${UNREGISTERED} ${name}"
done

if [ -n "${UNREGISTERED}" ]; then
    echo "  FAIL: gate script(s) with no classification:"
    for u in ${UNREGISTERED}; do echo "    ${u}"; done
    echo
    echo "  A gate nobody has proven can fail is exactly the defect this"
    echo "  script exists to catch, so an unclassified gate is a build"
    echo "  failure rather than reduced coverage. Put it in exactly one of:"
    echo
    echo "    REGISTRY        + a provoke_* fn that plants a violation"
    echo "    NOT_A_GATE      + why it has no pass/fail contract"
    echo "    NEEDS_HARDWARE  + what it needs that a sandbox lacks"
    echo "============================================================"
    exit 1
fi

# Only now, immediately before the first mutation.
if [ -n "$(git status --porcelain)" ]; then
    echo "  REFUSING TO RUN: working tree is dirty."
    echo
    echo "  This script deliberately plants violations in tracked files and"
    echo "  restores them afterwards. On a dirty tree a restore would clobber"
    echo "  your uncommitted work. Commit or stash first."
    echo
    git status --short | sed 's/^/    /'
    exit 2
fi

# ---------------------------------------------------------------------------
# Run each gate: must PASS clean, then must FAIL when provoked.
# ---------------------------------------------------------------------------
for name in $(printf '%s\n' "${REGISTRY}" | awk -F'|' 'NF>=3 {print $1}' | sort); do
    gate="scripts/${name}.sh"
    [ -f "${gate}" ] || continue
    [ -n "${ONLY}" ] && [ "${name}" != "${ONLY}" ] && continue
    entry="$(registry_lookup "${name}")"
    fn="$(echo "${entry}" | cut -d'|' -f2)"
    speed="$(echo "${entry}" | cut -d'|' -f3)"

    if [ "${fn}" = "SELF" ]; then
        # Self-exemption, and the only one. This script cannot provoke itself
        # without recursing forever. Its own "can it fail?" is covered by the
        # unregistered-gate check above, which is exercised every time a new
        # check-*.sh lands without a provocation.
        printf '  %-26s %s\n' "${name}" "SELF (covered by the unregistered-gate check)"
        continue
    fi

    if [ "${speed}" = "slow" ] && [ "${MODE}" = "--quick" ]; then
        printf '  %-26s %s\n' "${name}" "SKIPPED (--quick)"
        SKIPPED=$((SKIPPED + 1))
        continue
    fi

    CHECKED=$((CHECKED + 1))
    printf '  %-26s ' "${name}"

    # Phase 1 — green on a clean tree. If it is already red, the provocation
    # result would be meaningless.
    if ! "${REPO_ROOT}/${gate}" >/dev/null 2>&1; then
        echo "INCONCLUSIVE — already failing on a clean tree"
        FAILURES=$((FAILURES + 1))
        continue
    fi

    # Phase 2 — plant the violation.
    DID_PROVOKE=1
    if ! "${fn}" >/dev/null 2>&1; then
        echo "ERROR — provocation ${fn} could not be applied"
        FAILURES=$((FAILURES + 1))
        continue
    fi

    # Phase 3 — it must now fail.
    if [ "${name}" = "check-timeline-freshness" ]; then
        run_timeline_freshness >/dev/null 2>&1
    else
        "${REPO_ROOT}/${gate}" >/dev/null 2>&1
    fi
    rc=$?

    # Phase 4 — restore before judging, so a verdict never leaves debris.
    restore_touched

    if [ "${rc}" -ne 0 ]; then
        echo "OK — bites when provoked (exit ${rc})"
    else
        echo "FAIL — PASSED WHILE VIOLATED"
        FAILURES=$((FAILURES + 1))
    fi
done

# Print these EVERY run. A gate we cannot test is a real gap in coverage, and a
# gap you cannot see is indistinguishable from one that does not exist.
echo
echo "  Not provable here (needs sudo / live XDP / real hardware):"
printf '%s\n' "${NEEDS_HARDWARE}" | awk -F'|' 'NF>=2 {printf "    %-26s %s\n", $1, $2}'

echo
if [ "${FAILURES}" -gt 0 ]; then
    echo "============================================================"
    echo "  FAIL: ${FAILURES} gate(s) did not bite"
    echo "============================================================"
    echo
    echo "  A gate that stays green while its own contract is violated is"
    echo "  worse than no gate: it produces confidence without evidence."
    echo "  Fix the gate, then re-run. If the provocation itself is wrong,"
    echo "  fix the provocation — but prove the gate bites some other way"
    echo "  first."
    echo "============================================================"
    exit 1
fi

if [ -n "${ONLY}" ] && [ "${CHECKED}" -eq 0 ]; then
    echo "FAIL: --only ${ONLY} matched no runnable registered gate (see --list)."
    exit 1
fi

echo "PASS: ${CHECKED} gate(s) proven to fail when violated${SKIPPED:+, ${SKIPPED} skipped}."
exit 0
