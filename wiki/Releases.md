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

## Packages

`scripts/build-all-debs.sh [VERSION]` builds one deb per binary a `deploy/systemd` unit runs, plus `unheaded-keygen` (every key-holding package depends on it) and `unheaded-configs` (runbooks and `run-runbook.py` under `/opt/unheaded`). The table (names, units, accounts, keys) is `scripts/lib/deb-packages.sh`; `debian/control` is generated from it and the Jenkinsfile and `debian/rules` build through the same script. Unit names, upgrades and the TLS precondition: `deploy/systemd/README.md` ("Binary paths and unit names").

`DEB_SIGN_KEY=<keyid>` also writes `SHA256SUMS` and a detached `SHA256SUMS.gpg` over that build's debs (`gpg --verify SHA256SUMS.gpg SHA256SUMS && sha256sum -c SHA256SUMS`). The trust anchor for hosts is the repository's signed `InRelease`, below.

## apt repository

West serves a signed apt repository on port 18888 (loopback and the west-east link, `192.168.13.2:18888`), kept by `scripts/apt-repo.sh` in `/srv/unheaded-apt` with reprepro and served by nginx as the `nginx` account (`deploy/nginx/unheaded-apt.conf`: only `dists/`, `pool/` and the public key; everything else 403). Codename `unheaded`, suite `stable`, component `main`.

```bash
sudo scripts/apt-repo.sh init --keyid <KEYID>    # once; exports /srv/unheaded-apt/unheaded-apt.gpg
sudo scripts/apt-repo.sh include dist/debs/*_<version>_*.deb
scripts/apt-repo.sh check                         # reprepro check + Release/InRelease signatures
```

On a client, pin the repository's key with `signed-by=` (never `apt-key`, never `[trusted=yes]`):

```bash
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsS http://192.168.13.2:18888/unheaded-apt.gpg | sudo tee /etc/apt/keyrings/unheaded.gpg >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/unheaded.gpg] http://192.168.13.2:18888 unheaded main" \
  | sudo tee /etc/apt/sources.list.d/unheaded.list
sudo apt-get update && sudo apt-get install unheaded-wotan
```

Compare the fingerprint of the downloaded key (`gpg --show-keys /etc/apt/keyrings/unheaded.gpg`) with the one recorded for the repository before trusting it: the first fetch is over plain HTTP. Publishing and rollback: `runbooks/deploy/apt-repo-publish.yaml`. `tests/apt/apt_repo_test.sh` proves the script with a throwaway key: apt accepts the repository with the exported key and refuses it with any other, and a Release edited after signing fails `check`.

Not yet: the repository's signing key (who holds it, where: S-IRON-GATE E4), the repository live on west and east installing from it (E3), and the evidence build key (Phase 13).
