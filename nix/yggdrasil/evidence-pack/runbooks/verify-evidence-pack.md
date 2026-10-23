# Runbook — Verify Evidence Pack (operator-side, pre-deploy)

**Audience**: site reliability / security operator receiving a Yggdrasil image.
**Trigger**: Before deploying any Yggdrasil image to a production fleet.

---

## Trust anchor

`/etc/yggdrasil/build.pubkey` is the one trust anchor (`--pubkey` overrides it on every verify subcommand). Get it from the build key's owner over a channel you already trust, never from inside the pack you are checking. How it is published and rotated is ADR-105 open question E4.

## Quick verify (5 minutes)

```bash
# Everything in one go: schema, both signatures, CI gates, custody chain, ISO.
yggdrasil-evidence verify \
  --pubkey /etc/yggdrasil/build.pubkey \
  --iso /path/to/yggdrasil.iso \
  evidence-pack-<sha>.tar.gz
```

Or stage by stage:

```bash
# 1. manifest.yaml.sig and image.iso.sha256.sig against the build key
yggdrasil-evidence verify-signature --pubkey /etc/yggdrasil/build.pubkey evidence-pack-<sha>.tar.gz

# 2. The ISO's SHA-256 matches the signed image.iso.sha256 and the manifest
yggdrasil-evidence verify-iso --iso /path/to/yggdrasil.iso --pubkey /etc/yggdrasil/build.pubkey evidence-pack-<sha>.tar.gz

# 3. The CI gates, recomputed from the pack's own attachments
yggdrasil-evidence verify-gates evidence-pack-<sha>.tar.gz
```

Exit codes: **0** all green, **1** verification failed (including a pack that is malformed or unsafe to read), **2** usage error or unreadable input. Treat anything but 0 as a failure.

If any step fails — **DO NOT DEPLOY**. Page the security oncall and quarantine the ISO until reissued.

## Full audit (30 minutes — quarterly compliance review)

```bash
# 1. Extract for inspection
mkdir -p audit-${TIMESTAMP}
tar -xzf evidence-pack-<sha>.tar.gz -C audit-${TIMESTAMP}/

# 2. Inspect each component
yamllint -d relaxed audit-${TIMESTAMP}/manifest/manifest.yaml
jq . audit-${TIMESTAMP}/manifest/sbom.spdx.json | less
jq '.Results[].Vulnerabilities[]?' audit-${TIMESTAMP}/manifest/cve-scan.json
jq . audit-${TIMESTAMP}/manifest/cis-benchmark.json

# 3. Diff against last quarter's accepted evidence pack
yggdrasil-evidence diff \
  q3-baseline.tar.gz \
  evidence-pack-<sha>.tar.gz \
  > diff-q3-to-current.txt
# Exit 3 = the old pack's chain_of_custody is not a prefix of the new one:
# history was rewritten. Do not accept the new pack; escalate.

# 4. Confirm chain of custody is monotonic, hash-chained and every entry signed
yggdrasil-evidence verify-custody --pubkey /etc/yggdrasil/build.pubkey evidence-pack-<sha>.tar.gz
# A legacy pack with unsigned entries: --allow-unsigned-entries lists them (still exits 1).
```

## What the verifier checks (per gate)

| Gate | What it checks | Build-time enforced? | Verify-time enforced? |
|------|---------------|----------------------|----------------------|
| Signature | manifest.yaml.sig and image.iso.sha256.sig validate against build.pubkey (purpose-bound evidence seals) | Yes (step 10) | Yes |
| ISO hash | ISO's SHA-256 == signed image.iso.sha256 == manifest iso_sha256 (and size, name) | Yes (step 11) | Yes |
| CRITICAL CVEs | == 0 unwaived, recomputed from cve-scan.json | Yes (step 5) | Yes |
| HIGH CVEs | == 0 unless an unexpired waiver is listed and logged in chain_of_custody | Yes (step 5) | Yes |
| Attachments | SBOM package count, CIS/Lynis/doctor values, overlay patch hashes agree with the manifest | — | Yes |
| Custody | timestamps monotonic, every entry signed, hash chain intact | Yes (step 9) | Yes |
| CIS score | >= 95% | Yes (step 6) | Yes |
| Lynis hardening | >= 90 | Yes (step 6) | Yes |
| upc-doctor | exit_code == 0 | Yes (step 7) | Yes |
| Patch count | <= 50 (OS-FORK-DISCIPLINE.md Pillar 4) | Yes (schema) | Yes |
| LOC delta | <= 5000 (OS-FORK-DISCIPLINE.md Pillar 4) | Yes (schema) | Yes |
| Anchor commit | non-empty + 40 hex chars | Yes (schema) | Yes |
| Schema version | == "1.0.0" | Yes (schema) | Yes |

## Compliance mapping

| Framework | Control | Evidence pack mapping |
|-----------|---------|------------------------|
| FedRAMP Moderate | CM-2 (Baseline Configuration) | `anchor.json` + `overlay-patches/` |
| FedRAMP Moderate | CM-8 (System Component Inventory) | `sbom.spdx.json` |
| FedRAMP Moderate | RA-5 (Vulnerability Scanning) | `cve-scan.json` |
| FedRAMP Moderate | SI-2 (Flaw Remediation) | `chain_of_custody` waiver entries |
| SOC2 CC8.1 | Change Management | `chain_of_custody` |
| SOC2 CC7.1 | Threat Detection | `cve-scan.json` + `cis-benchmark.json` |
| ISO 27001 A.12.6.1 | Vulnerability Management | `cve-scan.json` |
| CIS Benchmark | Level 1 baseline | `cis-benchmark.json` |
| NIST 800-53 SI-7 | Software Integrity | `manifest.yaml.sig` + `image.iso.sha256.sig` |

The evidence pack is the **single artifact** that closes audit findings for these controls. Auditors don't need to re-run scans; they read the pack.

## See also

- `nix/yggdrasil/evidence-pack/README.md` — overview + scaffold spec
- `nix/yggdrasil/evidence-pack/schema/manifest-v1.yaml` — authoritative schema
- `nix/yggdrasil/evidence-pack/runbooks/build-evidence-pack.md` — build-side runbook
- `docs/OS-FORK-DISCIPLINE.md` — overall fork discipline
