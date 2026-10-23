# Yggdrasil — Signed-Manifest Evidence Pack (Task #68)

**Owner**: unheaded-architect (canonical) + unheaded-moatghost (audit) + unheaded-busboy (cross-skill).
**Authored**: 2026-05-11
**Status**: Implemented 2026-10-23 (`cmd/yggdrasil-evidence`, all subcommands; ADR-105 decisions 5-7). The packer pipeline (task #65) still emits `.todo` placeholders for SBOM/CVE/CIS/doctor, so a scaffold build produces an UNSIGNED pack that fails `verify` on purpose.
**Closes task**: #68 once the packer build (task #65) fills the attachments and a production build key exists (ADR-105 open question E4).

---

## What this is

Every Yggdrasil image build emits a **signed-manifest evidence pack** — a single `evidence-pack-<image-hash>.tar.gz` (every file under one top-level `manifest/` directory) containing the cryptographically-signed chain-of-custody from upstream Debian → Yggdrasil image → deployed system. This is the artifact compliance frameworks (FedRAMP, SOC2, ISO 27001, CIS Benchmark certification) need to demonstrate that:

1. The image was built from a known-good upstream anchor.
2. Every overlay patch is documented + reviewed + signed.
3. Every package in the image has a known SBOM entry.
4. Every CVE check at build time was clean.
5. The image's running integrity (post-boot) matches the build manifest.

## Pack contents (per build)

| File | Purpose | Format |
|------|---------|--------|
| `manifest.yaml` | Top-level evidence pack — versioned schema | YAML, schema in `schema/manifest-v1.yaml` |
| `manifest.yaml.sig` | ML-DSA-65 signature over `manifest.yaml`, purpose `manifest` | gungnir evidence seal, JSON (see "Signatures") |
| `anchor.json` | Debian release anchor metadata (codename, version, commit hash, release date) | JSON |
| `overlay-patches/` | Copy of all applied quilt patches with their SPDX headers | Directory mirror |
| `sbom.spdx.json` | SBOM in SPDX 2.3 JSON format (every .deb + every Yggdrasil-authored binary) | JSON |
| `sbom.cyclonedx.json` | Same SBOM in CycloneDX 1.5 format (interoperability) | JSON |
| `cve-scan.json` | Output of trivy scan against the image at build time | JSON |
| `cis-benchmark.json` | Lynis + custom CIS check results | JSON |
| `upc-doctor.json` | `yggdrasil-doctor upc` preflight result captured at first-boot test | JSON |
| `image.iso.sha256` | SHA-256 of the bootable ISO | One `sha256sum` line: `<64 hex>  <iso file name>` |
| `image.iso.sha256.sig` | ML-DSA-65 signature over `image.iso.sha256`, purpose `iso-sha256` | gungnir evidence seal, JSON |
| `chain-of-custody.txt` | Human-readable log of who-built-what-when (the signed, checked log is `manifest.yaml` `chain_of_custody`) | Plain text |

## Schema versioning

`schema/manifest-v1.yaml` is the authoritative schema. Bumping the schema is a Round Table decision (per `OS-FORK-DISCIPLINE.md` §4 rebase-cadence triggers — schema bumps are equivalent to anchor bumps for compliance purposes).

## Verification flow (operator-side)

The evidence pack ships standalone and is verifiable without network access:

```bash
# Everything: schema, both signatures, CI gates, custody chain, and the ISO
yggdrasil-evidence verify --pubkey /etc/yggdrasil/build.pubkey \
  --iso yggdrasil-<version>-amd64.iso evidence-pack-<hash>.tar.gz

# Inspect the SBOM
tar -xzOf evidence-pack-<hash>.tar.gz manifest/sbom.spdx.json | jq .

# Diff against another build (exit 3 if custody history was rewritten)
yggdrasil-evidence diff prev.tar.gz current.tar.gz
```

The `yggdrasil-evidence` binary is built from `cmd/yggdrasil-evidence/` (`go build ./cmd/yggdrasil-evidence`; packaged as a .deb per `nix/yggdrasil/repo/README.md`).

| Subcommand | Checks | Exit |
|---|---|---|
| `validate M.yaml` | schema v1 rules (Go rejects everything jsonschema rejects, plus the strictness below) | 0/1/2 |
| `sign --key S --in F --out SIG --purpose P` | key file 0600; a manifest must validate before it is signed | 0/1/2 |
| `sign ... --purpose custody-entry --custody-index N` | writes the hex signature for entry N | 0/1/2 |
| `pubkey --key S --out PUB` | derives, never generates | 0/1/2 |
| `verify-signature [--pubkey] PACK` | manifest + iso-sha256 seals | 0/1/2 |
| `verify-iso --iso F [--pubkey] PACK` | seal on image.iso.sha256, then ISO hash vs that line and the manifest | 0/1/2 |
| `verify-gates PACK` | CVE 0/0 (waivers logged in custody), CIS >= 95, Lynis >= 90, doctor 0, patches <= 50, LOC <= 5000, attachments agree with the manifest | 0/1/2 |
| `verify-custody [--pubkey] PACK` | monotonic timestamps, every entry signed, hash chain | 0/1/2 |
| `verify [--pubkey] [--iso F] PACK` | validate + signature + gates + custody (+ iso), each stage reported | 0/1/2 |
| `diff OLD NEW` | anchor/patch/SBOM/licence/CVE/CIS deltas; custody must be append-only | 0/2/3 |

Exit codes: 0 ok, 1 verification failed (a hostile or malformed pack is a failure, not a usage error), 2 usage or unreadable input, 3 (`diff` only) custody history rewritten.

## Trust anchor and signatures

- **One trust anchor**: `build.pubkey` (default `/etc/yggdrasil/build.pubkey`, `--pubkey` overrides), a packed ML-DSA-65 public key derived with `yggdrasil-evidence pubkey`. A root→build certification chain is parked (ADR-105). Where the build key seed lives and who holds it is open question E4; nothing in this repo generates or commits a key.
- **Signatures are gungnir evidence seals** (`pkg/gungnir/evidence.go`): JSON `{"format":"gungnir-evidence/v1","purpose":...,"payload_sha256":...,"key_id":...,"issued_at_unix":...,"algorithm":"ml-dsa-65","mldsa65_sig":...}`, ML-DSA context `unheaded/gungnir/evidence/v1`. They do not expire (packs are audited months later) and cannot be confused with Heimdall's short-lived config seals (different context) or with each other (the purpose is signed).
- **Custody entries**: each `chain_of_custody[i].signature` is the hex of a detached evidence signature (purpose `custody-entry`) over `timestamp_utc`, `actor`, `action` and the SHA-256 of entry i-1's signed bytes, so removing, reordering or editing an entry breaks every later signature.
- **Waivers**: a CRITICAL/HIGH finding in `cve-scan.json` counts as waived only when `cve_scan.waivers` lists it, the waiver has not expired on the day of verification, and its CVE ID appears in a `chain_of_custody` action. The manifest's `critical`/`high` are the unwaived counts and must be 0.

## Where Go is stricter than the schema

`yggdrasil-evidence validate` mirrors `schema/manifest-v1.yaml` in Go (no JSON-Schema dependency, ADR-004); `TestSchemaDriftGuard` fails when the two diverge, and `nix/yggdrasil/tests/test-manifest-differential.sh` proves it agrees with Python `jsonschema` on every fixture. It is deliberately stricter in these places:

- unknown fields in any object that declares properties (the schema has no `additionalProperties: false`; changing that is a Round Table decision) — the one documented fixture disagreement (`unknown-field.yaml`);
- YAML anchors, aliases, explicit tags, duplicate keys, more than one document;
- integers must be plain decimal (no `0x`, octal, `_`, or `4096.0`); numbers must be plain decimal (no `.inf`/`.nan`);
- patterns use end-of-text `$`, so a trailing newline is refused;
- `license_summary` values must be integers.

Scalars are typed the way PyYAML (YAML 1.1) types them: an unquoted `2024-08-31` is a timestamp and a plain `yes` is a boolean, so neither passes as a string. Quote strings.

## Discipline invariants (CI-checkable in task #65)

The Debian hardening pipeline MUST gate the build on these:

- [ ] `manifest.yaml` validates against `schema/manifest-v1.yaml` (yamllint + custom validator).
- [ ] `manifest.yaml.sig` verifies against the build key (`/etc/yggdrasil/build.pubkey`, the one trust anchor).
- [ ] `cve-scan.json` shows zero CRITICAL or HIGH findings (waivable only by Round Table — listed in `cve_scan.waivers` and logged in `chain_of_custody`).
- [ ] `cis-benchmark.json` shows ≥ 95% CIS Level 1 compliance (Lynis hardening score ≥ 90).
- [ ] `image.iso.sha256` matches the actual ISO emitted by packer (no in-flight tampering).
- [ ] All overlay patches in `overlay-patches/` apply cleanly against the anchor (re-verified at evidence-pack-build time, not just at quilt-push time).

## Files

| File | Purpose |
|------|---------|
| `schema/manifest-v1.yaml` | YAML schema for the manifest |
| `runbooks/build-evidence-pack.md` | Step-by-step runbook for the packer pipeline to follow |
| `runbooks/verify-evidence-pack.md` | Operator runbook for verifying a delivered evidence pack |
| `../tests/test-manifest-differential.sh` | Go validator vs Python jsonschema on every fixture |
| `cmd/yggdrasil-evidence/` | The CLI (fixtures under `testdata/manifests/`) |
| `scripts/check-evidence-pack.sh` | Gate: signs a fixture pack with a throwaway key, verifies it, asserts every tamper fails |

## References

- `docs/OS-FORK-DISCIPLINE.md` — overall fork discipline (this is Pillar 4: divergence budget + audit trail)
- `nix/yggdrasil/anchor.nix` — authoritative anchor pointer
- `nix/yggdrasil/overlay/` — overlay patches to be inventoried
- `pkg/gungnir/` — ML-DSA-65 library; `evidence.go` holds the evidence seals used here
- ADR-105 — evidence seals (decision 5), validation without new deps (6), pack layout and trust anchor (7)
- ADR-073 — lint policy zero-findings (the same ratchet philosophy applies here: every evidence pack must be valid)
