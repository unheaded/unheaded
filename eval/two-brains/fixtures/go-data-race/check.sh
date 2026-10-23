#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when the package's tests pass. Runs offline (no module downloads).
set -euo pipefail
cd "$(dirname "$0")"
export GOFLAGS=-mod=mod GOPROXY=off GOTOOLCHAIN=local GOWORK=off CGO_ENABLED=1
export GOCACHE="${GOCACHE:-$PWD/.gocache}" GOPATH="${GOPATH:-$PWD/.gopath}"
go test -count=1 -race ./...
