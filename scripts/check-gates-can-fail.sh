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
# USAGE
#   ./scripts/check-gates-can-fail.sh            # all gates
#   ./scripts/check-gates-can-fail.sh --quick    # skip gates marked SLOW
#   ./scripts/check-gates-can-fail.sh --list     # show registry, run nothing
#
# SAFETY
# It mutates tracked files on purpose. It refuses to run on a dirty tree, and an
# EXIT trap restores every file it touched even on Ctrl-C or a failed assertion.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}" || exit 1

MODE="${1:-}"
BACKUP_DIR="$(mktemp -d)"
TOUCHED=()
CREATED=()          # files the provocation brought into existence
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
    TOUCHED=()
    CREATED=()
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
    # works — sed appends to the args line either way.
    sed -i "s|args: '-fmt sarif -out gosec-results.sarif ./\.\.\.'|args: '-fmt sarif -out gosec-results.sarif -exclude=G000 ./...'|" "${REPO_ROOT}/${wf}"
    grep -q -- '-exclude=G000' "${REPO_ROOT}/${wf}"
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
    local f="vendor/meta-gate-probe/Cargo.toml"
    mkdir -p "${REPO_ROOT}/vendor/meta-gate-probe"
    backup "${f}"
    printf '[package]\nname = "meta-gate-probe"\nversion = "0.0.0"\nlicense = "AGPL-3.0"\n' \
        > "${REPO_ROOT}/${f}"
}

# ---------------------------------------------------------------------------
# Registry: gate basename -> provoke fn : speed : what the provocation plants
# ---------------------------------------------------------------------------
REGISTRY="
check-gosec-ratchet|provoke_gosec_ratchet|fast|an un-baselined rule appended to the workflow exclusion list
check-manifest-yaml|provoke_manifest_yaml|fast|a tracked manifest that does not parse
check-secrets-baseline|provoke_secrets_baseline|fast|a new fingerprint appended to .gitleaksignore
check-python-syntax|provoke_python_syntax|fast|a syntax error in a tracked .py file
check-timeline-freshness|provoke_timeline_freshness|fast|MAX_AGE_DAYS=-1, which nothing can satisfy
check-clippy|provoke_clippy|slow|a clippy violation in crates/upc-api
bpf-verifier-check|provoke_bpf_verifier_check|slow|an undefined symbol in ebpf/flow-tracker
verify-gpl-boundary|provoke_verify_gpl_boundary|fast|an AGPL license on a non-first-party Cargo.toml
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

echo "PASS: ${CHECKED} gate(s) proven to fail when violated${SKIPPED:+, ${SKIPPED} skipped}."
exit 0
