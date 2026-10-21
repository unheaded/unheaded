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
# shellcheck disable=SC2034  # read by the scripts that source this

OXLINT_VERSION="1.88.0"
OXLINT_SHA256="af29dc3ad2f6235590f114928c524649b2233cf27c9057e75ec16d0d7acf7d27" # oxlint-x86_64-unknown-linux-musl.tar.gz

GITLEAKS_VERSION="8.30.1"
GITLEAKS_SHA256="551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb" # gitleaks_<v>_linux_x64.tar.gz

SYFT_VERSION="1.54.1"
SYFT_SHA256="c069905b391cc4c20a5ba65ad5c10be2a7ba074f8ea6ad203e24d14e303dad47" # syft_<v>_linux_amd64.tar.gz

GRYPE_VERSION="0.120.1"
GRYPE_SHA256="0a9ee97ef5ae2ee953b0a80098105052e846cdbe319a57d808b519c33cd1343d" # grype_<v>_linux_amd64.tar.gz

HELM_VERSION="4.3.0"
HELM_SHA256="86584a54def73570558f66f5111cc53dfed56689637ae32c1201205d494f54fb" # helm-v<v>-linux-amd64.tar.gz (get.helm.sh, matches the release notes)

# hadolint runs from its image, pinned by digest (scripts/check-hadolint.sh).
HADOLINT_VERSION="2.15.1"
HADOLINT_IMAGE_DIGEST="sha256:a1d49ae1a4e83c1dbad26b8c1ad7588c8bd1e04f4866b34ad3cac50335198552" # hadolint/hadolint:v<v>-alpine

KUBECONFORM_VERSION="0.8.0"
KUBECONFORM_SHA256="9bc2bffbf71f261128533edaf912153948b7ff238f9a531ae6d34466ec287883" # kubeconform-linux-amd64.tar.gz
