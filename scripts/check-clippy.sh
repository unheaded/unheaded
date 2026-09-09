#!/bin/bash
# SPDX-License-Identifier: MIT
# check-clippy.sh — run clippy correctly across every Rust workspace in the tree.
#
# Two bugs in the previous inline CI loop, both invisible because the job was
# `continue-on-error: true`:
#
# 1. It iterated `git ls-files '*/Cargo.toml' | xargs -n1 dirname`, which yields
#    workspace MEMBERS as well as roots. `ebpf/` and its 22 members were each
#    linted separately, so every finding was counted up to 3 times. Workspace
#    roots are identified by Cargo.lock, not Cargo.toml.
#
# 2. It passed `--all-targets` unconditionally. For the two bare-metal
#    workspaces (target `bpfel-unknown-none`, `#![no_std]`, build-std=core)
#    that enables cfg(test) compilation against a `test` crate that does not
#    exist for the target. Result: 1391 phantom errors of the form
#      error: cannot find macro `assert_eq` in this scope
#      error: cannot find attribute `test` in this scope
#    None of them are findings. They are the linter being pointed at a target
#    it cannot build. Run lib/bin targets only there — the real warning count
#    for `ebpf/` is 33.
#
# Consequence of (2): the job could NEVER have been flipped to gating. It was
# structurally red, and `continue-on-error` meant nobody had to notice. Same
# class as the five gates documented in
# docs/security/findings-remediation-2026-07-29.md — green because it could
# not fail.
#
# Usage:
#   ./scripts/check-clippy.sh            # gate against the baseline
#   ./scripts/check-clippy.sh --update   # re-record the baseline
#   ./scripts/check-clippy.sh --report   # print findings, always exit 0
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${REPO_ROOT}/docs/security/clippy-baseline.txt"
MODE="${1:-}"

cd "${REPO_ROOT}" || exit 1

# Workspace ROOTS only. A Cargo.lock marks a root; members carry only a
# Cargo.toml. Do not "simplify" this back to Cargo.toml.
WORKSPACES="$(git ls-files '*/Cargo.lock' | xargs -n1 dirname | sort -u)"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

total=0
: > "${TMP}/all.txt"

for d in ${WORKSPACES}; do
    # A workspace pinned to a *-none target is bare metal: no std, no test
    # harness. --all-targets there lints code that cannot exist.
    target="$(grep -A3 '^\[build\]' "${d}/.cargo/config.toml" 2>/dev/null \
              | grep -oP 'target\s*=\s*"\K[^"]+' || true)"
    case "${target}" in
        *-none) targets_flag="" ;;
        *)      targets_flag="--all-targets" ;;
    esac

    echo "::group::clippy ${d} (${target:-host}${targets_flag:+ ${targets_flag}})"
    # shellcheck disable=SC2086 # targets_flag is intentionally word-split
    (cd "${d}" && cargo clippy ${targets_flag} --message-format=short 2>&1) \
        > "${TMP}/raw.txt"
    rc=$?

    grep -E '^[^ ]+:[0-9]+:[0-9]+: (warning|error)' "${TMP}/raw.txt" \
        | sed "s|^|${d}/|" >> "${TMP}/all.txt"

    # cargo's EXIT CODE is authoritative; the grep above is not.
    #
    # That grep only keeps diagnostics in file:line:col form. Target-resolution
    # failures are not in that form — they read
    #     error: can't find bin `x` at path `.../x.rs`
    #      --> Cargo.toml
    # so they were dropped silently, and because the pipeline's status was
    # never checked either, a workspace that does not build at all counted as
    # zero warnings and zero errors. Found 2026-09-09: ebpf/fuzz and
    # crates/heimdall-bpf both exit 101 while this script reported "0 errors".
    #
    # Same family as the two bugs in the header: a linter aimed at something it
    # cannot build, reported as clean.
    if [ "${rc}" -ne 0 ] && ! grep -qE '^[^ ]+:[0-9]+:[0-9]+: error' "${TMP}/raw.txt"; then
        echo "${d}: cargo clippy exited ${rc} with no file:line:col diagnostic" \
            >> "${TMP}/opaque.txt"
        sed 's|^|    |' "${TMP}/raw.txt" | head -20 >> "${TMP}/opaque.txt"
    fi
    echo "::endgroup::"
done

if [ -s "${TMP}/opaque.txt" ]; then
    echo "============================================================"
    echo "  FAIL: a workspace did not build under clippy"
    echo "============================================================"
    echo
    cat "${TMP}/opaque.txt"
    echo
    echo "  cargo exited non-zero but emitted no file:line:col diagnostic,"
    echo "  which usually means a [[bin]] or [[example]] points at a source"
    echo "  file that does not exist. The lint counts below are meaningless"
    echo "  for that workspace — nothing was linted."
    echo "============================================================"
    exit 1
fi

errors="$(grep -c ': error' "${TMP}/all.txt" || true)"
total="$(wc -l < "${TMP}/all.txt")"

# An error is never acceptable — it means the crate does not compile under
# clippy, which is a broken invocation or broken code. Gate on it regardless
# of the baseline.
if [ "${errors}" -gt 0 ]; then
    echo "============================================================"
    echo "  FAIL: clippy reported ${errors} ERRORS (not warnings)"
    echo "============================================================"
    grep ': error' "${TMP}/all.txt" | head -40
    echo
    echo "  Errors mean a crate did not compile under clippy. Either the"
    echo "  code is broken or this script is pointing the linter at a"
    echo "  target it cannot build. Both are bugs. Neither is baselineable."
    echo "============================================================"
    exit 1
fi

if [ "${MODE}" = "--report" ]; then
    cat "${TMP}/all.txt"
    echo "clippy: ${total} warnings across $(echo "${WORKSPACES}" | wc -l) workspaces"
    exit 0
fi

if [ "${MODE}" = "--update" ]; then
    cp "${TMP}/all.txt" "${TMP}/keep.txt"
    {
        echo "# clippy baseline — regenerate with ./scripts/check-clippy.sh --update"
        echo "# Ratchet: this count may only SHRINK. See scripts/check-clippy.sh."
        echo "${total}"
    } > "${BASELINE}"
    echo "Baseline updated -> ${total} warnings."
    exit 0
fi

[ -f "${BASELINE}" ] || { echo "FAIL: missing ${BASELINE} (run --update)"; exit 1; }
ALLOWED="$(grep -E '^[0-9]+$' "${BASELINE}" | head -1)"

# A baseline with no numeric line leaves ALLOWED empty. Both comparisons below
# then abort with "integer expression expected" (rc=2, which reads as false)
# and execution falls through to the PASS at the bottom — the script reports
# "clippy N/ warnings" and exits 0 no matter how far the count regressed.
#
# This is the same defect that was live in check-secrets-baseline.sh (fixed
# 2026-09-08). A ratchet whose ceiling can go missing is not a ratchet, so an
# unreadable baseline is a failure, never a pass.
if [ -z "${ALLOWED}" ]; then
    echo "============================================================"
    echo "  FAIL: ${BASELINE#"${REPO_ROOT}/"} has no numeric baseline line"
    echo "============================================================"
    echo
    echo "  The file exists but contains no bare integer, so there is"
    echo "  nothing to ratchet against. That is a failure and never a"
    echo "  free pass. Restore it:"
    echo
    echo "    git checkout -- docs/security/clippy-baseline.txt"
    echo
    echo "  or re-record it deliberately with:"
    echo
    echo "    ./scripts/check-clippy.sh --update"
    echo "============================================================"
    exit 1
fi

if [ "${total}" -gt "${ALLOWED}" ]; then
    echo "============================================================"
    echo "  FAIL: clippy ratchet went BACKWARDS"
    echo "============================================================"
    echo
    echo "  Baseline: ${ALLOWED} warnings"
    echo "  Now:      ${total} warnings"
    echo
    cat "${TMP}/all.txt"
    echo
    echo "  The count may only shrink. Fix the new warning, or if it is a"
    echo "  genuine false positive annotate the SITE with #[allow(...)] and"
    echo "  a reason — do not raise the baseline."
    echo "============================================================"
    exit 1
fi

echo "PASS: clippy ${total}/${ALLOWED} warnings, 0 errors."
if [ "${total}" -lt "${ALLOWED}" ]; then
    echo
    echo "  Progress — ${ALLOWED} -> ${total}. Lock it in:"
    echo "    ./scripts/check-clippy.sh --update"
fi
exit 0
