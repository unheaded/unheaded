<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Crosswalk Review 2026-10-27

**Scope:** `compliance/catalog/controls.yaml` mappings, five per framework file under `compliance/catalog/frameworks/`, checked against each framework's own requirement text (M-108).
**Reviewer:** agent (S-WELL-SPRING Phase 12), from the published texts: NIST SP 800-53 Rev 5 and NIST's 800-53 to ISO/IEC 27001:2022 mapping; the CSF 2.0 and SP 800-171 r3 OSCAL catalogs (also the requirement lists in the framework files); SP 800-218 SSDF 1.1; AICPA TSC 2017 with the 2022 points of focus; ISO/IEC 27001:2022 Annex A; CIS Ubuntu Linux 24.04 LTS Benchmark v1.0.0 and the FedRAMP Rev 5 baselines as imported. Not a third-party assessment.
**Result:** 40 checks across the 11 framework files (the three FedRAMP baselines share one table, as do the two CIS levels): 36 correct (one weak, kept), 2 correct but incomplete, 1 missing, 1 wrong. Four mapping changes, each recorded in the control's `rationale` and shown in `docs/compliance/SSP-generated.md`.

A mapping means "this evidence supports part of that requirement", never "requirement met" (catalog header). The check asked only: does the requirement's own text cover what the control's evidence shows?

## NIST SP 800-53 Rev 5 (`10-nist-800-53r5.yaml`)

| Control | Mapping | Requirement text (short) | Verdict |
|---|---|---|---|
| UH-SECRET-01 | IA-5(7) | no embedded unencrypted static authenticators in applications or scripts | correct |
| UH-BANNER-01 | AC-8 | display a system use notification before granting access | correct |
| UH-SUDO-01 | IA-11 | require users to re-authenticate when circumstances require it | correct (sudo asks again) |
| UH-TIME-01 | AU-8 | use internal clocks for time stamps in audit records | correct |
| UH-HOST-04 | SC-5 | protect against or limit the effects of denial-of-service events | kept, weak: separate filesystems limit one kind of resource exhaustion (a full log or temp directory), which the rationale already says; the main mappings are CM-6 and CM-7 |

## FedRAMP Rev 5 Low / Moderate / High (`20-22-fedramp-*.yaml`, derived from 800-53)

Derived mappings follow 800-53 for the controls each baseline contains. Checked that the baselines hold what the derivation assumes:

| 800-53 mapping | Low | Moderate | High | Verdict |
|---|---|---|---|---|
| AC-8 (UH-BANNER-01) | yes | yes | yes | correct |
| IA-11 (UH-SUDO-01) | yes | yes | yes | correct |
| SC-5 (UH-HOST-04) | yes | yes | yes | correct |
| IA-5(7) (UH-SECRET-01) | no | yes | yes | correct (Low does not count it) |
| SC-7(5) (UH-FW-01) | no | yes | yes | correct |

## SOC 2 TSC, Security (`30-soc2-tsc-security.yaml`)

| Control | Mapping | Criterion (short) | Verdict |
|---|---|---|---|
| UH-VULN-01 | CC7.1 | detection and monitoring to identify configuration changes that introduce vulnerabilities and susceptibility to newly discovered ones | correct |
| UH-FW-01 | CC6.6 | logical access security measures against threats from outside the system boundaries | correct |
| UH-TLS-01 | CC6.7 | restricts and protects information during transmission | correct |
| UH-IR-01 | CC7.4 | responds to identified incidents by executing a defined incident response program | correct |
| UH-POL-01 | CC5.3 | deploys control activities through policies and procedures | correct |

## ISO/IEC 27001:2022 Annex A (`40-iso27001-2022-annex-a.yaml`)

| Control | Mapping | Control text (short) | Verdict |
|---|---|---|---|
| UH-VULN-01 | A.8.8 | management of technical vulnerabilities | correct |
| UH-SIGN-01 | A.8.32 | change management | correct |
| UH-LIC-01 | A.5.32 | intellectual property rights | correct |
| UH-TIME-01 | A.8.17 | clock synchronization | correct |
| UH-LOG-02 | (none) | A.8.15 logging: logs produced, stored, **protected** and analysed | **missing: added A.8.15** (NIST's 800-53 to 27001:2022 mapping pairs AU-9 with 8.15) |

## NIST CSF 2.0 (`50-nist-csf-2.yaml`)

| Control | Mapping | Subcategory (short) | Verdict |
|---|---|---|---|
| UH-VULN-01 | ID.RA-01 | vulnerabilities in assets are identified, validated and recorded | correct |
| UH-SBOM-01 | ID.AM-02 | inventories of software, services and systems are maintained | correct |
| UH-BACKUP-01 | PR.DS-11 | backups are created, protected, maintained and tested | correct |
| UH-AUDIT-01 | PR.PS-04 | log records are generated for continuous monitoring | correct |
| UH-IR-01 | RS.MA-01 | the incident response plan is executed once an incident is declared | correct but incomplete: the yearly review evidences the plan's existence and upkeep, which is **ID.IM-04** (incident response and other plans established, communicated, maintained, improved). **Added ID.IM-04**; RS.MA-01 kept |

## NIST SP 800-171 r3 (`60-nist-800-171r3.yaml`)

| Control | Mapping | Requirement (short) | Verdict |
|---|---|---|---|
| UH-SESS-01 | 03.01.11 | session termination | correct |
| UH-BANNER-01 | 03.01.09 | system use notification | correct |
| UH-FW-01 | 03.13.06 | network communications: deny by default | correct |
| UH-SBOM-01 | 03.04.10 | system component inventory | correct |
| UH-IR-01 | 03.06.01 | incident handling | correct but incomplete: r3 has a separate **03.06.05 Incident Response Plan** (the IR-8 counterpart). **Added 03.06.05** |

## NIST SP 800-218 SSDF 1.1 (`70-nist-ssdf.yaml`)

| Control | Mapping | Practice (short) | Verdict |
|---|---|---|---|
| UH-VULN-01 | RV.1 | identify and confirm vulnerabilities on an ongoing basis | correct |
| UH-SAST-01 | PW.7 | review and/or analyze human-readable code | correct |
| UH-FUZZ-01 | PW.8 | test executable code | correct |
| UH-EVID-01 | PS.2 | provide a mechanism for verifying software release integrity | correct |
| UH-SECRET-01 | PS.1 | protect all forms of code from unauthorized access and tampering | **wrong: removed.** Secret scanning keeps credentials out of code; it does not restrict access to code or detect tampering. PS.1 stays mapped by UH-SIGN-01 (signed commits), so the SSDF counts do not change |

## CIS Ubuntu Linux 24.04 LTS v1.0.0, Level 2 and Level 1 Server (`80-`, `81-cis-ubuntu2404-*.yaml`)

CIS mappings sit on evidence sources (`maps`), and each rationale names its recommendations.

| Control | Recommendations | Benchmark text (short) | L1 | Verdict |
|---|---|---|---|---|
| UH-BOOT-01 | 1.4.1, 1.4.2 | bootloader password set; access to bootloader config configured | yes | correct |
| UH-BANNER-01 | 1.6.1-1.6.6 | message of the day, local and remote login banners and their file access | yes | correct |
| UH-SESS-01 | 5.4.3.2 | default user shell timeout configured | yes | correct |
| UH-FIM-01 | 6.3.1, 6.3.2 | AIDE installed; filesystem integrity regularly checked | yes | correct |
| UH-HOST-04 | 1.1.2.1.1 ... 1.1.2.7.4 | separate partitions and their nodev/nosuid/noexec options | /tmp yes; /home, /var, /var/tmp, /var/log, /var/log/audit partitions L2 only | correct; the L1 file holds 1.1.2.1.1 but not 1.1.2.3.1 or 1.1.2.4.1, as the benchmark assigns |

## Changes made in `controls.yaml`

| Control | Change |
|---|---|
| UH-IR-01 | + `nist-800-171r3: 03.06.05`, + `nist-csf-2: ID.IM-04`; statement now names IR plan v2 |
| UH-LOG-02 | + `iso27001-2022: A.8.15` |
| UH-SECRET-01 | − `nist-ssdf: PS.1` |

Not checked here: 800-53 enhancements beyond the five above, HIPAA, PCI DSS, CMMC, GDPR and CCPA (not in the catalog; their May matrices are historical, see the banner on each `docs/compliance/control-matrix/*-2026-05-06.md`).
