#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-pq-kex.sh — no NEW classical-only key exchange in the repository
# (ADR-105). The set of places that still are classical may only SHRINK.
#
# Contract. `go run ./cmd/crypto-inventory lint` scans every file git knows
# about (tracked, plus untracked files that are not ignored, so a new file
# fails before it is committed) with the crypto-inventory scanners and
# reports each of these as "<path> <rule>":
#
#   curve-pin-classical        a Go CurvePreferences list with no hybrid group
#                              (it turns Go's X25519MLKEM768 default off)
#   tls-floor-below-1.3        a first-party TLS floor below 1.3, in a tls.Config,
#                              another struct's MinVersion, an assignment, or gRPC
#                              credentials on Go defaults (a 1.2 session is
#                              always classical). Third-party peers (SMTP relay,
#                              CISA KEV, Splunk HEC, LB health checks to user
#                              backends, the LXD API) keep 1.2: their servers choose.
#   tls-max-below-1.3          a MaxVersion below 1.3
#   godebug-pq-off             GODEBUG tlsmlkem=0 / tlssecpmlkem=0 in code, compose,
#                              units, Dockerfiles, .env.example or go.mod
#   go-directive-pre-1.24      a go.mod `go` line below 1.24
#   rust-ring-provider         rustls on the ring provider (no ML-KEM): a
#                              crypto::ring::default_provider() call, or a rustls
#                              dependency with ring and without aws-lc-rs
#   postgres-pre18             a postgres image below 18 while a tracked
#                              postgresql conf turns ssl on (one classical curve)
#   postgres-groups-classical  ssl_groups / ssl_ecdh_curve with no hybrid group
#   sshd-kex-classical         an sshd KexAlgorithms list with no hybrid at all
#                              (mlkem768x25519-sha256 or sntrup761x25519 count)
#
# Line numbers are not part of a key, so editing an unrelated line above a
# baselined site does not churn the baseline.
#
# Ratchet (ADR-098: shrink-only baselines are accepted deviations, not passes).
# docs/policy/pq-kex-baseline.txt holds today's violations. A key not in it
# fails (exit 1). A baselined key the tree no longer has ALSO fails, with
# "resolved: remove from baseline", so a fix is recorded in the same commit
# and the violation cannot quietly come back later. Lower the baseline with
#   go run ./cmd/crypto-inventory lint --update
# which refuses additions, like check-image-tags.sh --update.
#
# Exemption: cmd/lich-security/** is not linted. It is the red-team tool and
# probes weak TLS on purpose; pinning classical groups there is the test, not
# a configuration we run. Test files (_test.go), testdata/ and eval/ (frozen
# model-eval fixtures) are not scanned.
#
# Explain one file: go run ./cmd/crypto-inventory lint --explain <path>
#
# Exit: 0 clean against the baseline, 1 new or resolved entries, 2 could
# not run.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT" || exit 2

case "${1:-}" in
"") args=(lint --baseline docs/policy/pq-kex-baseline.txt) ;;
--update) args=(lint --baseline docs/policy/pq-kex-baseline.txt --update) ;;
--list) args=(lint --print) ;;
*)
	echo "usage: $0 [--update | --list]" >&2
	exit 2
	;;
esac

# Build once instead of `go run`: go run reports every non-zero exit as 1,
# which would hide "could not run" (2) behind "violation" (1).
tmpd="$(mktemp -d)" || exit 2
trap 'rm -rf "$tmpd"' EXIT
go build -o "$tmpd/crypto-inventory" ./cmd/crypto-inventory || exit 2
"$tmpd/crypto-inventory" "${args[@]}"
