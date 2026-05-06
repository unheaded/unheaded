# Compliance Matrix Family — Mechanical Scrutiny Corrections Applied

**Date:** 2026-05-06
**Author:** Marshal post-shift extension (NORTH-STAR Appendix A Phase E12 follow-on)
**Driving doc:** `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md`
**Scope:** mechanical pass across the 15 framework matrices to apply scrutiny findings
**Approach:** sed-style downgrade / scope-qualifier addition. NOT authorial. Every edit
traceable to a specific scrutiny finding (SEN3 / SEN5 / SEN7 / SEN8 / BM3).

---

## Finding-to-text key

| Code | Pattern | Action |
|------|---------|--------|
| **SEN3** | `Wotan ring buffer` / `Wotan log aggregation` / `Anamnesis Lite eBPF` cited as MAPPED detection-class evidence | Downgrade to PARTIAL; add: *"Effective retention ~10 seconds at moderate traffic rate (10K-entry default); below FedRAMP / CIS 8.10 floor of 90 days online. Detection without retention is not coverage."* |
| **SEN5** | `Suricata IDS` cited as part of MAPPED status | Downgrade to PARTIAL; add: *"Suricata is integration code (`pkg/anamnesis/`) — deployment to actual hosts (WEST, EAST) is UNVERIFIED. See sensor deployment audit (`docs/security/sensor-deployment-audit-2026-05-06.md`)."* |
| **SEN7** | `BlackMage daily red-team` / `BlackMage adversarial loop` cited as evidence for pen-test / threat-hunt / red-team controls | Add disclaimer: *"INTERNAL validation only; NOT a substitute for the independent third-party assessment required by [framework]."* (no automatic downgrade — qualifier only) |
| **SEN8** | `NetworkPolicy` cited as MAPPED status evidence | Downgrade to PARTIAL; add: *"NetworkPolicy enforcement on default kindnet CNI is unproven (per K8s threat model §3.3); requires Calico/Cilium switch OR live smoke test before promoting to MAPPED."* |
| **BM3** | `ML-DSA-65` topic-signing cited as MAPPED evidence | Add scope qualifier (no automatic downgrade): *"ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable."* |

---

## Per-file corrections

### 1. `cis-controls-v8-2026-05-06.md` — 9 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| 4.2 Network secure-config | MAPPED | **PARTIAL** + SEN8 caveat | SEN8 |
| 4.4 Firewall on servers | MAPPED | **PARTIAL** + SEN8 caveat | SEN8 |
| 8.2 Collect Audit Logs | MAPPED | **PARTIAL** + SEN3 caveat | SEN3 |
| 13.3 NIDS | MAPPED | **PARTIAL** + SEN5 caveat | SEN5 |
| 13.4 Traffic filtering | MAPPED | **PARTIAL** + SEN8 caveat | SEN8 |
| 3.10 Encrypt in transit | MAPPED (kept) + BM3 qualifier | (no downgrade) | BM3 |
| 12.6 Secure network mgmt | MAPPED (kept) + BM3 qualifier | (no downgrade) | BM3 |
| 16.13 App pen-test | PARTIAL (kept) + SEN7 disclaimer | (qualifier only) | SEN7 |
| 18.1 Pen-test program | PARTIAL (kept) + SEN7 disclaimer | (qualifier only) | SEN7 |
| 18.5 Internal pen-test | PARTIAL (kept) + SEN7 disclaimer | (qualifier only) | SEN7 |

Net status downgrades: **5 cells (MAPPED → PARTIAL)**. Net qualifier additions only: 5 cells.

### 2. `fedramp-moderate-2026-05-06.md` — 13 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| AC-4(4) | MAPPED + BM3 qualifier | (no downgrade) | BM3 |
| AC-4(21) Logical separation | MAPPED | **PARTIAL** + SEN8 caveat | SEN8 |
| AU-6(5) Integration scanning | MAPPED | **PARTIAL** + SEN5 caveat | SEN5 |
| CA-8(2) Red Team Exercises | PARTIAL + SEN7 disclaimer | (qualifier only) | SEN7 |
| CM-3(6) Crypto mgmt | MAPPED + BM3 | (no downgrade) | BM3 |
| IA-7 Crypto module auth | MAPPED + BM3 | (no downgrade) | BM3 |
| IA-8(4) NIST-Approved Crypto | MAPPED + BM3 | (no downgrade) | BM3 |
| RA-10 Threat Hunting | PARTIAL + SEN7 disclaimer | (qualifier only) | SEN7 |
| SC-7(5) Deny-by-default | MAPPED | **PARTIAL** + SEN8 caveat | SEN8 |
| SC-8(2) Pre/post transmission | MAPPED + BM3 | (no downgrade) | BM3 |
| SC-12(3) Asymmetric Keys | MAPPED + BM3 | (no downgrade) | BM3 |
| SC-13 Crypto Protection | MAPPED + BM3 | (no downgrade) | BM3 |
| SI-4(4) In/out Comms Traffic | MAPPED | **PARTIAL** + SEN3 caveat | SEN3 |
| SI-4(11) Comms anomalies | MAPPED | **PARTIAL** + SEN3 caveat | SEN3 |
| SI-7(15) Code Authentication | MAPPED + BM3 | (no downgrade) | BM3 |

Net status downgrades: **5 cells (MAPPED → PARTIAL)**. BM3 qualifiers added: 7 cells. SEN7 disclaimers: 2 cells.

### 3. `nist-800-53-2026-05-06.md` — 14 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| AU-2 Event Logging | MAPPED | **PARTIAL** + SEN3 caveat | SEN3 |
| CA-8 Pen Testing | PARTIAL + SEN7 disclaimer | (qualifier only) | SEN7 |
| CM-6 Configuration Settings | MAPPED + BM3 | (no downgrade) | BM3 |
| CM-14 Signed Components | MAPPED + BM3 | (no downgrade) | BM3 |
| IA-7 Crypto Module Auth | MAPPED + BM3 | (no downgrade) | BM3 |
| RA-5 Vuln Monitoring | MAPPED | **PARTIAL** + SEN5 caveat | SEN5 |
| RA-10 Threat Hunting | PARTIAL + SEN7 disclaimer | (qualifier only) | SEN7 |
| SC-7 Boundary Protection | MAPPED | **PARTIAL** + SEN8 + SEN5 caveats | SEN8, SEN5 |
| SC-8 Transmission C/I | MAPPED + BM3 | (no downgrade) | BM3 |
| SC-12 Crypto Key Establishment | MAPPED + BM3 | (no downgrade) | BM3 |
| SC-13 Crypto Protection | MAPPED + BM3 | (no downgrade) | BM3 |
| SC-23 Session Authenticity | MAPPED + BM3 | (no downgrade) | BM3 |
| SI-4 System Monitoring | MAPPED | **PARTIAL** + SEN3 + SEN5 caveats | SEN3, SEN5 |
| SI-7 Software/Firmware Integrity | MAPPED + BM3 | (no downgrade) | BM3 |
| SR-11 Component Authenticity | MAPPED + BM3 | (no downgrade) | BM3 |
| (param table SC-12) | MAPPED + BM3 short qualifier | (no downgrade) | BM3 |

Net status downgrades: **4 cells (MAPPED → PARTIAL)**. BM3 qualifiers added: 9 cells. SEN7 disclaimers: 2 cells.

### 4. `pci-dss-2026-05-06.md` — 14 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| 1.2.1 NSC config standards | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN8 caveat | SEN8 |
| 1.3 NSC trusted/untrusted | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN8 caveat | SEN8 |
| 1.5 NSC change-detection | MAPPED-CONTRIBUTES + SEN3 caveat | (qualifier only — no downgrade since FIM not detection-class only) | SEN3 |
| 4.1 Strong crypto over open networks | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |
| 6.3 Vuln identification | MAPPED-CONTRIBUTES + SEN7 disclaimer | (qualifier only) | SEN7 |
| 8.3 Strong auth | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |
| 10.1 Audit logs implemented | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN3 caveat | SEN3 |
| 10.2 Audit logs identifiable events | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN3 caveat | SEN3 |
| 11.3 Pen testing | PARTIAL-CONTRIBUTES + SEN7 disclaimer | (qualifier only) | SEN7 |
| 11.4 IDS/IPS | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN5 + SEN3 caveats | SEN5, SEN3 |
| 1.4.1 NSC trust/untrusted | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN8 | SEN8 |
| 1.4.2 NSC inbound restricted | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN8 | SEN8 |
| 8.3.2 Strong crypto auth factors | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |
| 10.2.1 Audit logs enabled | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN3 | SEN3 |
| 11.4.2 Internal pen-test | PARTIAL + SEN7 disclaimer | (qualifier only) | SEN7 |
| 11.5.1 IDS/IPS | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN5 | SEN5 |

Net status downgrades: **9 cells (MAPPED-CONTRIBUTES → PARTIAL-CONTRIBUTES)**. BM3 qualifiers added: 3 cells. SEN7 disclaimers: 3 cells.

### 5. `soc2-2026-05-06.md` — 9 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| CC5.2 Tech controls | MAPPED + BM3 | (no downgrade) | BM3 |
| CC6.6 Logical access boundary | MAPPED | **PARTIAL** + SEN8 + SEN5 | SEN8, SEN5 |
| CC6.7 Restricts data transmission | MAPPED + BM3 | (no downgrade) | BM3 |
| CC7.2 Monitors components | MAPPED | **PARTIAL** + SEN3 caveat | SEN3 |
| CC7.3 Eval security events | PARTIAL + SEN5 + SEN7 | (qualifier only) | SEN5, SEN7 |
| CC5.2-POF-2 Tech infra controls | MAPPED + BM3 | (no downgrade) | BM3 |
| CC6.1-POF-9 Encryption | MAPPED + BM3 | (no downgrade) | BM3 |
| CC6.6-POF-4 Boundary protection | MAPPED | **PARTIAL** + SEN5 | SEN5 |
| CC7.1-POF-2 Monitor infra/sw | MAPPED | **PARTIAL** + SEN3 + SEN5 | SEN3, SEN5 |
| CC7.2-POF-1 Detection policies | MAPPED | **PARTIAL** + SEN5 | SEN5 |
| CC7.2-POF-2 Designs detection | MAPPED | **PARTIAL** + SEN3 | SEN3 |

Net status downgrades: **6 cells (MAPPED → PARTIAL)**. BM3 qualifiers added: 4 cells.

### 6. `hipaa-2026-05-06.md` — 6 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| 308(a)(1)(ii)(D) Sys activity review | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN3 | SEN3 |
| 308(a)(5)(ii)(B) Malware protection | PARTIAL-CONTRIBUTES + SEN5 + SEN7 | (qualifier only) | SEN5, SEN7 |
| 312(a)(2)(iv) Encryption/decryption | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |
| 312(b) Audit controls | MAPPED-CONTRIBUTES | **PARTIAL-CONTRIBUTES** + SEN3 | SEN3 |
| 312(c)(1) Integrity | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |
| 312(e)(2)(i) Transmission integrity | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |

Net status downgrades: **2 cells**. BM3 qualifiers: 3. SEN5+SEN7: 1.

### 7. `gdpr-2026-05-06.md` — 2 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| Art. 5(1)(f) Integrity & confidentiality | MAPPED + BM3 | (no downgrade) | BM3 |
| Art. 32 Security of processing | MAPPED + BM3 | (no downgrade) | BM3 |

Net status downgrades: **0**. BM3 qualifiers: 2.

### 8. `iso-27001-27002-2026-05-06.md` — 7 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| A.5.7 Threat intelligence | MAPPED + SEN7 disclaimer | (qualifier only) | SEN7 |
| A.5.14 Information transfer | MAPPED + BM3 | (no downgrade) | BM3 |
| A.8.5 Secure authentication | MAPPED + BM3 | (no downgrade) | BM3 |
| A.8.15 Logging | MAPPED | **PARTIAL** + SEN3 | SEN3 |
| A.8.16 Monitoring activities | MAPPED | **PARTIAL** + SEN3 + SEN5 | SEN3, SEN5 |
| A.8.20 Networks security | MAPPED | **PARTIAL** + SEN8 + SEN5 | SEN8, SEN5 |
| A.8.24 Use of cryptography | MAPPED + BM3 | (no downgrade) | BM3 |

Net status downgrades: **3 cells**. BM3 qualifiers: 3. SEN7: 1.

### 9. `nist-800-171-2026-05-06.md` — 11 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| 03.01.03 Info Flow Enforcement | MAPPED + BM3 | (no downgrade) | BM3 |
| 03.03.01 Event Logging | MAPPED | **PARTIAL** + SEN3 | SEN3 |
| 03.04.02 Configuration Settings | MAPPED + BM3 | (no downgrade) | BM3 |
| 03.10.08 Access Ctrl Transmission | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |
| 03.11.02 Vuln Monitoring | MAPPED | **PARTIAL** + SEN5 | SEN5 |
| 03.13.01 Boundary Protection | MAPPED | **PARTIAL** + SEN8 + SEN5 | SEN8, SEN5 |
| 03.13.06 Network Comms deny default | MAPPED | **PARTIAL** + SEN8 | SEN8 |
| 03.13.10 Crypto Key Establishment | MAPPED + BM3 | (no downgrade) | BM3 |
| 03.13.11 Crypto Protection | MAPPED + BM3 | (no downgrade) | BM3 |
| 03.13.15 Session Authenticity | MAPPED + BM3 | (no downgrade) | BM3 |
| 03.14.02 Malicious Code Protection | PARTIAL + SEN5 + SEN7 disclaimer | (qualifier only) | SEN5, SEN7 |
| 03.14.06 System Monitoring | MAPPED | **PARTIAL** + SEN3 + SEN5 | SEN3, SEN5 |

Net status downgrades: **5 cells**. BM3 qualifiers: 6. SEN7: 1.

### 10. `nist-800-207-2026-05-06.md` — 7 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| Tenet 2 (secure regardless of network) | MAPPED | **PARTIAL** + SEN8 + BM3 | SEN8, BM3 |
| Tenet 6 (dynamic auth) | MAPPED + BM3 | (no downgrade) | BM3 |
| PEP component | MAPPED | **PARTIAL** + SEN8 | SEN8 |
| Network/system activity logs | MAPPED | **PARTIAL** + SEN3 | SEN3 |
| §3.4 Visibility into network traffic | MAPPED | **PARTIAL** + SEN3 | SEN3 |
| §3.4 Secure network traffic | MAPPED | **PARTIAL** + SEN8 + BM3 | SEN8, BM3 |
| §5.5 Storage of System/Network Info | MAPPED | **PARTIAL** + SEN3 + BM3 | SEN3, BM3 |

Net status downgrades: **6 cells**. BM3 qualifiers: 4. (No SEN7.)

### 11. `nist-csf-2-2026-05-06.md` — 6 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| PR.AA-04 Identity assertions | MAPPED + BM3 | (no downgrade) | BM3 |
| PR.DS-02 Data-in-transit C/I/A | MAPPED + BM3 | (no downgrade) | BM3 |
| PR.PS-04 Log records | MAPPED | **PARTIAL** + SEN3 | SEN3 |
| PR.IR-01 Networks protected | MAPPED | **PARTIAL** + SEN8 | SEN8 |
| DE.CM-01 Network monitoring | MAPPED | **PARTIAL** + SEN3 + SEN5 | SEN3, SEN5 |
| DE.CM-09 Hardware/sw monitoring | MAPPED | **PARTIAL** + SEN3 + SEN5 | SEN3, SEN5 |

Net status downgrades: **4 cells**. BM3 qualifiers: 2.

### 12. `nist-800-218-ssdf-2026-05-06.md` — 4 corrections

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| PS.2.1 Software integrity verification | MAPPED + BM3 | (no downgrade) | BM3 |
| PW.1.3 Standardized security features | MAPPED + BM3 | (no downgrade) | BM3 |
| PW.9.1 Secure baseline | MAPPED | **PARTIAL** + SEN8 | SEN8 |
| RV.1.2 Code review/test | PARTIAL + SEN7 disclaimer | (qualifier only) | SEN7 |

Net status downgrades: **1 cell**. BM3 qualifiers: 2. SEN7: 1.

### 13. `ccpa-cpra-2026-05-06.md` — 1 correction

| Cell | Before | After | Driving |
|------|--------|-------|---------|
| §1798.150 Reasonable security | MAPPED-CONTRIBUTES + BM3 | (no downgrade) | BM3 |

Net status downgrades: **0**. BM3 qualifier: 1.

### 14. `itar-ear-2026-05-06.md` — 0 corrections

No MAPPED cells cite ML-DSA-65, NetworkPolicy, Suricata, BlackMage, or Wotan/Anamnesis as MAPPED-status evidence. All ML-DSA-65 references are in N/A or REQUIRES-LEGAL contexts (discussion of cryptography algorithm, not control-coverage claims). **Skipped: zero cells need correction.**

### 15. `cmmc-2-2026-05-06.md` — 0 corrections

File contains zero references to NetworkPolicy, Suricata, ML-DSA-65, BlackMage, Wotan, or Anamnesis. (CMMC-2 inherits from NIST 800-171 by reference.) **Skipped: zero cells need correction.**

---

## Aggregate totals

| Driving finding | Cells touched (downgrades) | Cells touched (qualifier-only) | Total |
|-----------------|----------------------------|-------------------------------|-------|
| **SEN3** (ring-buffer retention) | 17 | 0 | 17 |
| **SEN5** (Suricata deployment) | 11 | 1 | 12 |
| **SEN7** (red-team internal) | 0 | 13 | 13 |
| **SEN8** (kindnet enforcement) | 19 | 0 | 19 |
| **BM3** (ML-DSA-65 scope) | 0 | 41 | 41 |

**Total cells edited:** approximately **102 distinct cell edits across 13 of 15 matrix files** (cells with multiple findings counted once per finding). Two files (`itar-ear` and `cmmc-2`) had no qualifying cells.

**Total cells genuinely downgraded (status changed):** approximately **47 cells** moved from MAPPED → PARTIAL (or MAPPED-CONTRIBUTES → PARTIAL-CONTRIBUTES).

**Total cells with qualifier added but status preserved:** approximately **55 cells** (BM3 scope qualifiers + SEN7 disclaimers).

---

## File with the most corrections

**`pci-dss-2026-05-06.md`** received the most edits at **17 cell corrections** (9 status downgrades + 8 qualifier-only additions). PCI-DSS hit hard because its matrix uses MAPPED-CONTRIBUTES heavily for substrate claims, many of which lean on Suricata (11.4, 11.5.1) and NetworkPolicy (1.2.1, 1.3, 1.4.1, 1.4.2) — both areas struck by SEN5 and SEN8 respectively, and on Wotan/Anamnesis Lite for audit-logging (10.1, 10.2, 10.2.1) where SEN3 retention applies.

Runner-up: **`nist-800-53-2026-05-06.md`** at 14 cell corrections.

---

## What this audit log does NOT do

- Does not re-author evidence — only mechanically restates what the scrutiny doc requires.
- Does not consolidate scrutiny findings beyond SEN3/5/7/8 and BM3 — other findings (S1 status enum, S2 falsifiability, S3 PII attestation, etc.) remain open.
- Does not commit any changes — Stevie's directive was edit + audit-log + report. No git activity.
- Does not patch downstream consumers (battle plans, ADRs, BlackMage findings registry). Those remain as-was; this audit log is the input record they would consult.

---

## Provenance

Read-only audit of mechanical corrections. Sources: tonight's `01-scrutiny-2026-05-06.md`; the 15 matrix files at `docs/compliance/control-matrix/{framework}-2026-05-06.md`; CLAUDE.md (Wotan ring buffer 10K-entry default reference, ML-DSA-65 `config.*` topic-signing scope reference per `wiki/Wotan-Topic-Signing.md`).
