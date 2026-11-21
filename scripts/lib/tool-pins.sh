#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# tool-pins.sh — every release-pinned tool's version and sha256, in one place.
# Sourced by the scripts that fetch them (through lib/fetch-tool.sh, which
# checks the sha256 on every use). scripts/check-tool-freshness.sh reads the
# *_VERSION lines and fails once a newer release has been out 7 days.
#
# A bump changes VERSION and SHA256 together. Take the sha256 from the
# project's own release (GitHub's asset digest, or the published .sha256sum
# cross-checked against the release notes), never from a mirror.
#
# Not here: the two archives scripts/install-prerequisites.sh puts on a host
# (the Go release of go.mod's toolchain line, node_exporter). Their versions
# and sha256s are at the top of that script, so an operator can replace them
# from the environment; the same rule applies to a bump.
# shellcheck disable=SC2034  # read by the scripts that source this

OXLINT_VERSION="1.93.0"
OXLINT_SHA256="9c8b7ebee3b3fd8b069e3e9b66d59b294ead4d306863600babfbc7bb1bd566ec" # oxlint-x86_64-unknown-linux-musl.tar.gz

GITLEAKS_VERSION="8.30.1"
GITLEAKS_SHA256="551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb" # gitleaks_<v>_linux_x64.tar.gz

SYFT_VERSION="1.57.1"
SYFT_SHA256="bf52519eb57f675d47c07d43213cb6dff6449f3d6d1147ff6701ed91f44eef0c" # syft_<v>_linux_amd64.tar.gz

GRYPE_VERSION="0.121.3"
GRYPE_SHA256="e9da18cc1429e786722e32161288103600efffd8e483c90761f90485bd1bec85" # grype_<v>_linux_amd64.tar.gz

COSIGN_VERSION="3.1.4"
COSIGN_SHA256="424e2c97d0b45fd7a6b608a87f090f35633f4c23ece5f092d48bba11aaf4bbfa" # cosign-linux-amd64 (bare binary; cosign_checksums.txt)


BUF_VERSION="1.75.0"
BUF_SHA256="fad1e38d6909bbb583a4da81d8c02a01046e2cd663bde7e337dd24277393764b" # buf-Linux-x86_64 (bare binary; sha256.txt)

# hadolint runs from its image, pinned by digest (scripts/check-hadolint.sh).
HADOLINT_VERSION="2.15.1"
HADOLINT_IMAGE_DIGEST="sha256:a1d49ae1a4e83c1dbad26b8c1ad7588c8bd1e04f4866b34ad3cac50335198552" # hadolint/hadolint:v<v>-alpine

# trivy's misconfig ratchet (scripts/check-trivy-misconfig.sh) compares against a
# baseline, so CI and a local seed must run the same trivy AND the same checks
# bundle: trivy downloads the newest bundle of its major version on every run
# unless told otherwise. security.yml reads all three from here. Bump them
# together and re-seed docs/policy/trivy-misconfig-baseline.tsv (--update only
# drops rows; a new check that fires is a reviewed decision).
TRIVY_VERSION="0.76.2"
TRIVY_IMAGE_DIGEST="sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb" # aquasec/trivy:<v>
TRIVY_CHECKS_BUNDLE="mirror.gcr.io/aquasec/trivy-checks@sha256:b591e9190cff4aa4e385b1a25bccfd9ecf178d3c77fd194e1410b7ce7e1df058" # trivy-checks:2, 2026-10-28


# Vector on east as a host package (config/vector/vector-east.yaml). Same
# version as west's compose image timberio/vector:<v>-debian, so both hosts
# run the same VRL; bump the two together. apt.vector.dev (the project's
# current repository) and the legacy packages.timber.io serve different bytes
# for 0.43.1-1 (the apt copy is 862 bytes larger); this pins apt.vector.dev's.
# Not in check-tool-freshness.sh's TOOLS: the compose image is not either.
VECTOR_DEB_VERSION="0.43.1-1"
VECTOR_DEB_URL="https://apt.vector.dev/pool/v/ve/vector_0.43.1-1_amd64.deb"
VECTOR_DEB_SHA256="a8c01eeda385da9b8b5a09446fd01b615539713fd0e04bf9efd100b1a1c0b083"
