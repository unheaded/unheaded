# S77 — CI/CD Strategy (sprint index)

**Sprint:** S77 — Age 2 Acceleration Campaign
**Phase:** 3 — SBOM + CI/CD Fortress
**Status:** Shipped — GHA workflows + Jenkinsfile both hardened
**Canonical doc:** [`docs/CI-CD-STRATEGY-S77.md`](../CI-CD-STRATEGY-S77.md)
**Gate test:** [`tests/s77/s77_verification_test.go::TestPhase3_CICD`](../../tests/s77/s77_verification_test.go)

---

## Purpose

Sprint-folder index for the S77 CI/CD hardening. The canonical doc has
the developer-facing reference; this file summarises *current state* in
the sprint-accounting voice.

---

## Two CI surfaces

| Surface | Triggered on | Job role | Typical wall time |
|---------|--------------|----------|-------------------|
| **GitHub Actions** | every PR + push to main | fast feedback: lint → unit → integration → SBOM → GPL-boundary → coverage | 8 – 12 min |
| **Jenkins** | post-merge to main (+ nightly cron) | deep hardware-dependent validation on WEST/EAST: eBPF verifier load, bpftool exercises, cross-host flow graph | 25 – 45 min |

GHA is the *OSS-facing* surface — contributors see clear pass/fail in
the PR. Jenkins is the *internal* surface — it touches bare-metal hosts
and signs deployment artifacts.

## GHA workflow inventory (`.github/workflows/`)

- `ci.yml` — build + unit tests + vet, the baseline gate.
- `ci-protocol.yml` — protocol-spec linting (RFC editor pass, IANA
  registry consistency).
- `gpl-boundary.yml` — **key gate.** Runs `scripts/verify-gpl-boundary.sh`
  to ensure no GPL-only dependencies leak into Apache-2.0 protocol
  packages.
- `sbom.yml` + `sbom-audit.yml` — generates and audits SBOM (cyclonedx
  + spdx); fails on net-new GPL or unknown-license dependencies.
- `drift-detect.yml` — verifies live infra still matches the
  declared desired-state in `iac/`.
- `ebpf.yml` — builds Rust eBPF programs, runs the BPF verifier check.
- `security.yml` — `cargo audit`, `govulncheck`, secret-scan,
  golangci-lint gate.
- `apply.yml` / `release.yml` / `helm.yml` / `docker.yml` —
  deployment-side rails.
- `timeline-drift-guard.yml` — keeps `references/timeline.md` honest
  against the live commit graph.
- `yggdrasil-verify.yml` — anchor-pin verification for the Yggdrasil
  hardened-OS pipeline.
- `raft-python.yml` — RAFT QA pipeline regression.
- `plan.yml` — battle-plan formatting + footer verification.

## Jenkinsfile

Declarative pipeline pinned to `golang:1.24-alpine` (S77 Phase 3 baseline
— Phase E.4 of the broader Wobbly-Pond plan migrates Node consumers to
24; Go itself is independent). Stages:

1. **Dependencies** — `go mod download && go mod verify`.
2. **Build** — `go build ./...` and Rust eBPF where applicable.
3. **Test** — unit + integration with race detector.
4. **License Check** — invokes `scripts/verify-gpl-boundary.sh`
   (mirrored from GHA so internal pipeline can not skip it).
5. **Security Scans** — `cargo audit`, `govulncheck`, container scan,
   gated on the `RUN_SECURITY` parameter (default `true`).
6. **SBOM Generation** — `syft` SBOM emit, gated on the `RUN_SBOM`
   parameter (default `true`).
7. **Deploy** — main-branch only, gated on `RUN_DEPLOY` (default
   `false`); ships .deb + Docker image + Nix flake outputs.

## Security gates

- **SPDX header check** — every Go file must carry
  `SPDX-License-Identifier: GPL-3.0-or-later` (or the dual-licensed
  protocol packages, `GPL-3.0-or-later OR Apache-2.0`). Enforced in
  `ci.yml`.
- **golangci-lint** — gated on a clean run.
- **BPF verifier budget** — `scripts/bpf-verifier-check.sh` asserts the
  Aya kprobe + XDP programs stay under 12% of the 900K-instruction
  budget. Verifier delta is logged per build.
- **`cargo audit`** — Rust dependency vulnerability gate.
- **GPL boundary** — protocol packages cannot pull GPL dependencies.

## Deployment targets

- **Docker** — multi-arch (amd64 + arm64) Container images to
  `ghcr.io/unheaded/unheaded`.
- **Debian package** — .deb artifacts attached to GitHub releases.
- **Nix flake** — reproducible build via `nix build .#default`, used
  by the Yggdrasil hardened-OS bake.
- **Sealed Cask** — `scripts/build-sealed-cask.sh` produces a
  deterministic image; `verify-binding-rune.sh` confirms SHA256
  integrity at deploy time.

## Branch strategy

- `main` — protected, signed commits required, requires green CI.
- `staging` — integration branch, merged to main on green.
- `feature/*` — per-feature, may be unsigned during autonomous shifts
  (see Stevie's `feedback_unsigned_commits_when_afk` rule). Squash-
  merged on review.

## Signed commits posture

Default is **signed commits**. Autonomous overnight shifts are
permitted to use `--no-gpg-sign` when gpg-agent times out
(`feedback_unsigned_commits_when_afk`). Signed-commit verification is
gated *after* merge to main — Jenkins enforces signature on tagged
releases.

---

## PROPOSED / TBD per S77 close-out

- **Node 24 migration** — referenced in the forthcoming Phase E.4 of
  the Wobbly-Pond plan. Tracked separately; not an S77 deliverable.
- **Sigstore / cosign signing for container images** — designed but
  not landed; deferred until the first public release tag.

---

Free to use. Free to share.
