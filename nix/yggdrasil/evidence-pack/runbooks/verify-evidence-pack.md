# Runbook — Verify Evidence Pack (operator-side, pre-deploy)

**Audience**: site reliability / security operator receiving a Yggdrasil image.
**Trigger**: Before deploying any Yggdrasil image to a production fleet.

---

## Quick verify (5 minutes)

```bash
# 1. Verify the manifest signature against the kingdom root key
yggdrasil-evidence verify-signature \
  --pubkey /etc/yggdrasil/root.pubkey \
  evidence-pack-<sha>.tar.gz

# 2. Verify the ISO hash matches the manifest
yggdrasil-evidence verify-iso \
  --iso /path/to/yggdrasil.iso \
  evidence-pack-<sha>.tar.gz

# 3. Run all CI gates from the manifest
yggdrasil-evidence verify-gates evidence-pack-<sha>.tar.gz

# Exit 0 = all green. Exit 1 = something failed.
```

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

# 4. Confirm chain of custody is monotonic + signed
yggdrasil-evidence verify-custody evidence-pack-<sha>.tar.gz
```

## What the verifier checks (per gate)

| Gate | What it checks | Build-time enforced? | Verify-time enforced? |
|------|---------------|----------------------|----------------------|
| Signature | manifest.yaml.sig validates against build.pubkey | Yes (step 10) | Yes |
| ISO hash | iso_sha256 matches actual ISO file | Yes (step 11) | Yes |
| CRITICAL CVEs | == 0 | Yes (step 5) | Yes |
| HIGH CVEs | == 0 unless waiver in chain_of_custody | Yes (step 5) | Yes |
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
