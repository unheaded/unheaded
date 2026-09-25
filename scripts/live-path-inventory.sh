#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-093 rule 5: record reachability in the inventory.
#
# Binaries live under cmd/, services/*/cmd/, and elsewhere (the enumeration
# is `go list`, not a glob; see binary_roots). 10 ship in the container image. Nothing in the tree said which ones run, where, or under what
# supervisor — so "most of this does not run" was folklore rather than a fact
# anyone could check, and the recurring defect of this repository is a correct
# mechanism nothing reaches.
#
# This classifies every binary by the strongest evidence that something
# executes it:
#
#   CONTAINER  built into the container image (Dockerfile -o /build/bin/)
#   SUPERVISED referenced by a systemd unit, Nix module or K8s manifest
#   TOOL       invoked by a script or runbook, but not supervised
#   ORPHAN     referenced by nothing outside its own directory
#
# ORPHAN is not an accusation. In a solo learning project kept experiments
# are the point (ADR-093, and ADR-090 owns the deletion question). The rule
# is only that the count stays a decision rather than decaying into "nobody
# knows what these are".
#
# Usage:
#   scripts/live-path-inventory.sh            # CHECK (default): fail on drift
#   scripts/live-path-inventory.sh --report   # print the table to stdout
#
# Checking is the DEFAULT, deliberately. It used to be --report, which always
# exits 0 — so invoking the script bare produced a green result that proved
# nothing, and check-gates-can-fail.sh duly caught it passing while violated.
# That is the same defect as check-timeline-freshness.sh, whose bare
# invocation defaulted to a mode that could not fail and reported PASS for
# three promotion batches. A gate's default mode must be the one that bites.
#
# Strict by construction in the other sense too: the report is built from the
# repository, never from the running host. Asking docker what is running would
# pass on the one machine that happens to have the containers up, which is the
# exact mistake ADR-093 rule 4 names.
set -euo pipefail

cd "$(dirname "$0")/.."

SNAPSHOT="docs/LIVE-PATHS.md"
MODE="${1:---check}"

# refs_outside <name> — does anything outside cmd/<name> mention it?
refs_outside() {
    local path="$1"
    shift
    # This script names example paths in its comments; it is not evidence.
    grep -rl --exclude-dir=.git --exclude=live-path-inventory.sh -- "${path}" "$@" 2>/dev/null |
        grep -v "^${path}/" || true
}

# binary_name <path> — the name a unit or image would install it under. A
# package rooted at services/<svc>/cmd has basename "cmd", which names nothing.
binary_name() {
    local base
    base="$(basename "$1")"
    if [ "$base" = "cmd" ]; then
        base="$(basename "$(dirname "$1")")"
    fi
    echo "$base"
}

classify() {
    local path="$1"
    local name
    name="$(binary_name "$path")"

    # Match the SOURCE PATH, not the output name. Matching "-o /build/bin/timeguru"
    # reported the root services/timeguru package (the one Nix ships) as
    # containerised, when the image builds services/timeguru/cmd/timeguru.
    if grep -qE -- "-o /build/bin/[^ ]+ \./${path}( |$)" Dockerfile 2>/dev/null; then
        echo "CONTAINER"
        return
    fi
    # Unit match ends at whitespace, a line continuation or EOL. "\b" treated
    # "-" as a boundary, and a short MBC program name ("cat", "ls") would match
    # any /bin/cat in any unit.
    if [ -n "$(refs_outside "$path" nix kubernetes deploy 2>/dev/null)" ] ||
        git ls-files '*.service' | xargs grep -lE "^ExecStart=[^ ]*/${name}([[:space:]]|\\\\|$)" 2>/dev/null | head -1 | grep -q .; then
        echo "SUPERVISED"
        return
    fi
    if [ -n "$(refs_outside "$path" scripts runbooks Makefile 2>/dev/null)" ]; then
        echo "TOOL"
        return
    fi
    echo "ORPHAN"
}

# binary_roots — every buildable binary in the tree, one path per line.
#
# Go: every package named main, from `go list`. Globbing cmd/*/ and
# services/*/cmd/*/ missed 14 of them — services/gateway/cmd, the root
# services/timeguru that Nix ships, deploy/sophia-eye/sophia-gateway and 11 MBC
# programs — and the gate passed while blind to all of them. A glob encodes
# where binaries were expected to live; `go list` reports where they are.
#
# Non-Go: Rust and C roots have no `go list`, so they keep the glob, but only
# directories that hold a Cargo.toml or a .c file. The bare glob also counted
# cmd/tools, which is documentation.
binary_roots() {
    local mains
    if ! mains="$(go list -e -f '{{if eq .Name "main"}}{{.Dir}}{{end}}' ./...)"; then
        echo "[FAIL] go list failed; cannot enumerate binaries" >&2
        return 1
    fi
    if [ -z "$mains" ]; then
        echo "[FAIL] go list found no main packages; refusing to report an empty tree" >&2
        return 1
    fi
    {
        echo "${mains//"$PWD"\//}"
        local d
        for d in cmd/*/ services/*/cmd/*/; do
            d="${d%/}"
            [ -d "$d" ] || continue
            if [ -f "$d/Cargo.toml" ] || compgen -G "$d/*.c" >/dev/null; then
                echo "$d"
            fi
        done
    } | sort -u
}

generate() {
    local container=0 supervised=0 tool=0 orphan=0 total=0
    local rows=""

    local paths
    paths="$(binary_roots)" || exit 1

    for path in $paths; do
        total=$((total + 1))
        local kind
        kind="$(classify "$path")"
        case "$kind" in
        CONTAINER) container=$((container + 1)) ;;
        SUPERVISED) supervised=$((supervised + 1)) ;;
        TOOL) tool=$((tool + 1)) ;;
        ORPHAN) orphan=$((orphan + 1)) ;;
        esac
        rows="${rows}| \`${path}\` | ${kind} |"$'\n'
    done

    cat <<EOF
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.

GENERATED by scripts/live-path-inventory.sh — do not edit by hand.
Regenerate with: scripts/live-path-inventory.sh --report > docs/LIVE-PATHS.md
-->

# Live paths — what actually runs

ADR-093 rule 5. Generated from the repository, never from a running host:
asking docker what is up would pass on the one machine that has the
containers running, which is the mistake ADR-093 rule 4 names.

| classification | meaning | count |
|---|---|---|
| CONTAINER | built into the container image | ${container} |
| SUPERVISED | referenced by a systemd unit, Nix module or K8s manifest | ${supervised} |
| TOOL | invoked by a script, runbook or Makefile, not supervised | ${tool} |
| ORPHAN | referenced by nothing outside its own directory | ${orphan} |
| **total** | every Go \`main\` package (\`go list\`) plus Rust/C roots under \`cmd/\`, \`services/*/cmd/\` | **${total}** |

ORPHAN is not an accusation — kept experiments are the point of a solo
learning project, and ADR-090 owns the deletion question. The count exists so
that keeping them stays a decision.

| binary | classification |
|---|---|
${rows}
EOF
}

case "$MODE" in
--report | report)
    generate
    ;;
--check)
    if [ ! -f "$SNAPSHOT" ]; then
        echo "[FAIL] $SNAPSHOT is missing; run: scripts/live-path-inventory.sh --report > $SNAPSHOT" >&2
        exit 1
    fi
    if diff -u "$SNAPSHOT" <(generate) >/tmp/live-paths.diff 2>&1; then
        echo "[PASS] live-path inventory matches $SNAPSHOT"
    else
        echo "[FAIL] live-path inventory drifted from $SNAPSHOT:" >&2
        cat /tmp/live-paths.diff >&2
        echo >&2
        echo "A binary was added, removed, or changed reachability." >&2
        echo "Regenerate: scripts/live-path-inventory.sh --report > $SNAPSHOT" >&2
        exit 1
    fi
    ;;
*)
    echo "usage: $0 [--check | --report]" >&2
    exit 2
    ;;
esac
