<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Releases

A release is cut by pushing a tag `vX.Y.Z` (pre-releases `vX.Y.Z-alpha`, `-beta`, `-rc`). `.github/workflows/release.yml` then:

1. **build**: linux/amd64; `govulncheck` (pinned v1.9.0) must pass; every service binary (`scripts/build-binaries.sh`) and every deb (`scripts/build-all-debs.sh`, the tag's version in Debian form, `0.2.0~alpha`).
2. **ebpf**: every eBPF object (nightly Rust, `bpf-linker`, `bpfel-unknown-none`), the same recipe as `ebpf.yml`.
3. **release**: `checksums.txt` (sha256 of every file), a CycloneDX SBOM (pinned syft), a keyless cosign signature for every file (`<file>.sigstore.json`), the SBOM attested against `checksums.txt`, GitHub build provenance for every file, then the GitHub release. Only this job holds `contents`, `id-token` and `attestations` write.

Keyless means no signing key exists to leak: each signature carries a short-lived certificate that names the workflow and the tag that ran it, and a Rekor transparency-log entry.

## Verifying a release

```bash
scripts/verify-release.sh v0.2.0-alpha --download --dir ./rel   # fetch everything and verify
scripts/verify-release.sh v0.2.0-alpha --dir ./rel              # verify files you already have
```

It requires the certificate identity `https://github.com/unheaded/unheaded/.github/workflows/release.yml@refs/tags/<tag>` from GitHub Actions' OIDC issuer on `checksums.txt`, then checks every listed file present against its sha256; a run that checked no file fails. cosign comes from `$COSIGN`, `PATH`, or the pinned 3.1.3 (`scripts/ci-install-tools.sh`). Provenance is also visible with `gh attestation verify <file> --repo unheaded/unheaded`.

The script's own contract is tested offline on every push (CI job `release-verify`, `tests/release/verify_release_test.sh`): a fake release signed with a throwaway key verifies, and a swapped file, an edited `checksums.txt`, a missing bundle and a malformed tag are refused.

Not yet: debs published to an apt repository (S-IRON-GATE Phase 14) and the evidence build key (Phase 13).
