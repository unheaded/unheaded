#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-094 step 6: prometheus/client_golang is gone, and stays gone.
#
# The tree carried nine modules for one metrics client, protobuf among them
# for an exposition format it never served. pkg/metrics replaced it, parity-
# tested against client_golang's own output before the dependency was
# dropped (goldens in pkg/metrics/testdata and pkg/metrics/prom/testdata).
#
# Two checks, both reading the repo rather than a built binary (ADR-093
# rule 4):
#   1. no tracked .go file imports github.com/prometheus/{client_golang,
#      client_model,common} (the quoted import path, so comments that name the
#      library do not trip it);
#   2. go.mod does not require any of them.
#
# A module that pulls one in transitively without us importing it would pass
# check 1 and show up in check 2 only if tidy lists it; that is the right
# place to catch it, because then a human decides.
set -euo pipefail

cd "$(dirname "$0")/.."

FAMILY='github\.com/prometheus/(client_golang|client_model|common)'
fail=0

# --untracked: a new file is caught before it is committed, and the
# meta-gate (which provokes by writing a new file) sees this gate bite.
# Plain git grep searches tracked files only and passed a planted import.
imports="$(git grep --untracked -nE "\"${FAMILY}[\"/]" -- '*.go' || true)"
if [[ -n "${imports}" ]]; then
    echo "check-no-client-golang: FAIL — Go files import the client_golang family:" >&2
    echo "${imports}" | sed 's/^/  /' >&2
    fail=1
fi

requires="$(grep -nE "^\s*${FAMILY}\s" go.mod || true)"
if [[ -n "${requires}" ]]; then
    echo "check-no-client-golang: FAIL — go.mod requires the client_golang family:" >&2
    echo "${requires}" | sed 's/^/  /' >&2
    fail=1
fi

if [[ "${fail}" -ne 0 ]]; then
    echo "  Use pkg/metrics (or pkg/metrics/prom and /auto for client_golang-shaped" >&2
    echo "  call sites). See docs/adr/ADR-094-own-the-metrics-stack.md." >&2
    exit 1
fi

echo "check-no-client-golang: PASS (no imports, no go.mod requirement)"
