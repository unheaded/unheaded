# Yggdrasil — Signed-Manifest Evidence Pack (Task #68)

**Owner**: unheaded-architect (canonical) + unheaded-moatghost (audit) + unheaded-busboy (cross-skill).
**Authored**: 2026-05-11
**Status**: Scaffold — implementation lights up at task #65 (Debian hardening pipeline).
**Closes task**: #68 once impl is wired into the packer build.

---

## What this is

Every Yggdrasil image build emits a **signed-manifest evidence pack** — a single `evidence-pack-<image-hash>.tar.gz` containing the cryptographically-signed chain-of-custody from upstream Debian → Yggdrasil image → deployed system. This is the artifact compliance frameworks (FedRAMP, SOC2, ISO 27001, CIS Benchmark certification) need to demonstrate that:

1. The image was built from a known-good upstream anchor.
2. Every overlay patch is documented + reviewed + signed.
3. Every package in the image has a known SBOM entry.
4. Every CVE check at build time was clean.
5. The image's running integrity (post-boot) matches the build manifest.

## Pack contents (per build)

| File | Purpose | Format |
|------|---------|--------|
| `manifest.yaml` | Top-level evidence pack — versioned schema | YAML, schema in `schema/manifest-v1.yaml` |
| `manifest.yaml.sig` | ML-DSA-65 signature over `manifest.yaml` | Detached signature (cloudflare/circl mldsa65) |
| `anchor.json` | Debian release anchor metadata (codename, version, commit hash, release date) | JSON |
| `overlay-patches/` | Copy of all applied quilt patches with their SPDX headers | Directory mirror |
| `sbom.spdx.json` | SBOM in SPDX 2.3 JSON format (every .deb + every Yggdrasil-authored binary) | JSON |
| `sbom.cyclonedx.json` | Same SBOM in CycloneDX 1.5 format (interoperability) | JSON |
| `cve-scan.json` | Output of trivy scan against the image at build time | JSON |
| `cis-benchmark.json` | Lynis + custom CIS check results | JSON |
| `upc-doctor.json` | `yggdrasil-doctor upc` preflight result captured at first-boot test | JSON |
| `image.iso.sha256` | SHA-256 of the bootable ISO | Single line, hex |
| `image.iso.sha256.sig` | ML-DSA-65 signature over the SHA-256 hash | Detached signature |
| `chain-of-custody.txt` | Human-readable log of who-built-what-when | Plain text |

## Schema versioning

`schema/manifest-v1.yaml` is the authoritative schema. Bumping the schema is a Round Table decision (per `OS-FORK-DISCIPLINE.md` §4 rebase-cadence triggers — schema bumps are equivalent to anchor bumps for compliance purposes).

## Verification flow (operator-side)

The evidence pack ships standalone and is verifiable without network access:

```bash
# Verify the manifest signature
yggdrasil-evidence verify evidence-pack-<hash>.tar.gz

# Inspect the SBOM
tar -xzOf evidence-pack-<hash>.tar.gz manifest/sbom.spdx.json | jq .

# Diff anchor against another build
yggdrasil-evidence diff prev.tar.gz current.tar.gz
```

The `yggdrasil-evidence` binary lives at `nix/yggdrasil/bin/yggdrasil-evidence` (TBD, scaffold below).

## Discipline invariants (CI-checkable in task #65)

The Debian hardening pipeline MUST gate the build on these:

- [ ] `manifest.yaml` validates against `schema/manifest-v1.yaml` (yamllint + custom validator).
- [ ] `manifest.yaml.sig` verifies against the kingdom's mldsa65 root key (path `/etc/yggdrasil/build.pubkey`).
- [ ] `cve-scan.json` shows zero CRITICAL or HIGH findings (waivable only by Round Table — log in `chain-of-custody.txt`).
- [ ] `cis-benchmark.json` shows ≥ 95% CIS Level 1 compliance (Lynis hardening score ≥ 90).
- [ ] `image.iso.sha256` matches the actual ISO emitted by packer (no in-flight tampering).
- [ ] All overlay patches in `overlay-patches/` apply cleanly against the anchor (re-verified at evidence-pack-build time, not just at quilt-push time).

## Files in this scaffold

| File | Purpose |
|------|---------|
| `schema/manifest-v1.yaml` | YAML schema for the manifest |
| `runbooks/build-evidence-pack.md` | Step-by-step runbook for the packer pipeline to follow |
| `runbooks/verify-evidence-pack.md` | Operator runbook for verifying a delivered evidence pack |

## References

- `docs/OS-FORK-DISCIPLINE.md` — overall fork discipline (this is Pillar 4: divergence budget + audit trail)
- `nix/yggdrasil/anchor.nix` — authoritative anchor pointer
- `nix/yggdrasil/overlay/` — overlay patches to be inventoried
- `pkg/gungnir/` — ML-DSA-65 sealed payload library used to sign the manifest (see commit `9d43b12d` for the type-assertion fix)
- ADR-073 — lint policy zero-findings (the same ratchet philosophy applies here: every evidence pack must be valid)
