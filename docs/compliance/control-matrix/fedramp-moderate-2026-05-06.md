# FedRAMP Moderate Control-Coverage Matrix (Rev 5 baseline, exhaustive)

**Date:** 2026-05-06
**Author:** Marshal post-shift extension (NORTH-STAR Appendix A Phase E6)
**Reference:** FedRAMP Rev 5 Moderate Baseline (effective May 2023, current control catalog)
**Approach:** Layered on top of `nist-800-53-2026-05-06.md` (E5). FedRAMP Moderate selects ~325 controls + control enhancements from NIST SP 800-53 Rev 5, with **FedRAMP-specific parameter values** and **additional requirements** that go beyond the underlying NIST control. This document captures the FedRAMP **delta + parameter values + enhancements** that aren't fully visible in the bare NIST matrix.
**Status legend:** **MAPPED** = full coverage with evidence | **PARTIAL** = some coverage, gap noted | **GAP** = no kingdom work yet | **N/A** = not applicable | **DEFERRED** = planned, not shipped | **UNVERIFIED** = MoatGhost confirm | **PARAM-GAP** = NIST control may be MAPPED but FedRAMP parameter value not met

**Authorization aspiration check:** A FedRAMP Moderate ATO requires (a) full SSP, (b) System Assessment Plan (SAP), (c) System Assessment Report (SAR), (d) POA&M, (e) ConMon plan, (f) 3PAO assessment, (g) Agency Authorizing Official sponsor, (h) FedRAMP PMO review. Tonight's matrix is the *foundational gap inventory* — months 1-2 of a 12-18 month ATO timeline. Don't conflate the two.

---

## FedRAMP-specific structural requirements

Beyond the NIST control content, FedRAMP imposes:

| Requirement | Status |
|-------------|--------|
| Control implementation summary in SSP format | GAP |
| FIPS 199 categorization documented (Mod = Moderate confidentiality + Moderate integrity + Moderate availability minimum) | GAP |
| FIPS 200 minimum security requirements addressed | PARTIAL (technical controls largely mapped; organizational gap) |
| US persons restriction (citizenship/green-card for ATO-related personnel) | UNVERIFIED — single-operator US person Stevie Bellis; document |
| Continuous Monitoring (ConMon) Strategy & Plan with monthly POA&M updates | GAP |
| Annual Security Assessment by 3PAO | GAP |
| Annual Penetration Test by 3PAO | GAP |
| Vulnerability scanning monthly (operating system + database + web app) | PARTIAL — security.yml daily exceeds frequency; coverage of OS/DB/WebApp categories needs verification |
| Critical CVE remediation in 30 days, High in 90 days, Moderate in 180 | UNVERIFIED — no formal remediation SLA |
| Incident response within US-CERT timeline (1-hour reporting for high-severity) | GAP — IR runbook missing |
| US-based data centers / US-citizen-only-with-clearance for some agency tenants | ADOPTER-OWNS — adopter selects deployment region |

---

## Family-by-family enhancement coverage at FedRAMP Moderate baseline

The following enumerates **only enhancements that FedRAMP Moderate requires** (Rev 5 baseline). Parent controls inherit status from `nist-800-53-2026-05-06.md`; enhancements add specificity.

### AC — Access Control (FedRAMP Mod requires AC-1 through AC-22 selected enhancements)

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| AC-2(1) | Automated System Account Management | PARTIAL | pkg/auth scaffolding; The Well 7 service-scoped users provisioned manually. Gap: no auto-provision/de-provision pipeline. |
| AC-2(2) | Automated Temporary & Emergency Account Management | GAP | No emergency-account workflow. |
| AC-2(3) | Disable Accounts | PARTIAL | JWT TTL forces re-auth; pkg/auth supports user disable. Gap: no automated disable trigger. |
| AC-2(4) | Automated Audit Actions | MAPPED | pkg/auth.AuditLogger logs auth events. |
| AC-2(5) | Inactivity Logout | UNVERIFIED | JWT TTL; no separate inactivity-logout SOP. |
| AC-2(7) | Privileged User Accounts | PARTIAL | Champion gate distinguishes privileged actions. Gap: no formal privileged-account inventory. |
| AC-2(9) | Restrictions on Use of Shared / Group Accounts | GAP | No prohibition documented. |
| AC-2(11) | Usage Conditions | GAP | None defined. |
| AC-2(12) | Account Monitoring / Atypical Usage | PARTIAL | Sentinel anomaly detection. Gap: no formal atypical-usage threshold. |
| AC-2(13) | Disable Accounts for High-Risk Individuals | GAP | None defined. |
| AC-3(7) | Role-Based Access Control | MAPPED | pkg/auth.RBACAuthorizer. |
| AC-3(11) | Restrict Access to Specific Information Types | PARTIAL | The Well 3-database separation by trust class; per-service users. Gap: no formal information-type → role mapping. |
| AC-4(4) | Flow Control of Encrypted Information | MAPPED | TLS 1.3 record-level + ML-DSA-65 topic signing. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| AC-4(8) | Security Policy Filters | PARTIAL | pkg/champion 3-rule gate filters tool calls. Gap: no general security-policy-filter inventory. |
| AC-4(21) | Physical / Logical Separation of Information Flows | PARTIAL | Microservice architecture + NetworkPolicy + Wotan topic separation. **SEN8 caveat:** NetworkPolicy enforcement on default kindnet CNI is unproven (per K8s threat model §3.3); requires Calico/Cilium switch OR live smoke test before promoting to MAPPED. |
| AC-6(1) | Authorize Access to Security Functions | PARTIAL | pkg/auth + RBAC. Tonight's RBAC review F1 — `unheaded-armory` over-grant blocks confident least-privilege claim. |
| AC-6(2) | Non-Privileged Access for Non-Security Functions | MAPPED | NoopAuthenticator dev-only; APIKey/JWT for non-priv ops. |
| AC-6(3) | Network Access to Privileged Commands | PARTIAL | mTLS available; Champion gate for mutations. |
| AC-6(5) | Privileged Accounts | PARTIAL | Same as AC-2(7). |
| AC-6(7) | Review of User Privileges | GAP | No periodic review. |
| AC-6(9) | Log Use of Privileged Functions | MAPPED | pkg/auth.AuditLogger; Champion writes audit log per gate. |
| AC-6(10) | Prohibit Non-Privileged Users from Executing Privileged Functions | MAPPED | Champion 3-rule gate hard-blocks. |
| AC-7(2) | Purge / Wipe Mobile Device | N/A | n/a to substrate. |
| AC-8 | System Use Notification | GAP | No login banner. **FedRAMP-specific text required for federal systems.** |
| AC-12(1) | User-Initiated Logouts | PARTIAL | API key revocation endpoint. Gap: no formal user-initiated-logout SOP. |
| AC-17(1) | Monitoring & Control | MAPPED | All remote access via TLS 1.3; eBPF traces remote sessions. |
| AC-17(2) | Protection of Confidentiality / Integrity Using Encryption | MAPPED | TLS 1.3 mandatory. |
| AC-17(3) | Managed Access Control Points | PARTIAL | HAProxy edge + per-service nginx sidecars. Gap: not formally documented as ACPs. |
| AC-17(4) | Privileged Commands / Access | PARTIAL | Champion gate + RBAC. |
| AC-17(9) | Disconnect / Disable Access | GAP | No formal disconnect runbook. |
| AC-19(5) | Full Device / Container-Based Encryption | ADOPTER-OWNS | Device-level. |
| AC-20(1) | Limits on Authorized Use | ADOPTER-OWNS | Adopter sets. |
| AC-20(2) | Portable Storage Devices | ADOPTER-OWNS | n/a to substrate. |
| AC-22 | Publicly Accessible Content | PARTIAL | Community-first doctrine commits to public sharing. **FedRAMP requires non-public information not on public sites — needs explicit attestation.** |

**FedRAMP AC parameters:**
- AC-2 account types defined: individual, group, system, application, guest/anonymous, temporary — kingdom has individual + system + application; group/guest/temporary GAP.
- AC-7(a) consecutive invalid logon attempts: FedRAMP suggests `[5]`. Kingdom: not enforced at framework level — application-specific.
- AC-7(b) lockout time: FedRAMP suggests `[30 minutes]`. Kingdom: not enforced.
- AC-11 inactivity period: FedRAMP suggests `[15 minutes]`. Kingdom: JWT TTL configurable.
- AC-12 session-timeout duration: `[30 minutes]` typical. Kingdom: JWT TTL configurable.

### AT — Awareness and Training

| Ctrl | Enhancement | Status |
|------|-------------|--------|
| AT-2(2) | Insider Threat | GAP |
| AT-2(3) | Social Engineering & Mining | GAP |
| AT-3(3) | Practical Exercises | GAP |
| AT-4(a) | Training Records | GAP |

(Entire AT family GAP for single-operator kingdom; document accepted-gap.)

### AU — Audit and Accountability

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| AU-2(3) | Reviews & Updates | PARTIAL | ADR-052 drift policy keeps audit-event list reviewed. Gap: no formal annual review. |
| AU-3(1) | Additional Audit Information | MAPPED | trace_id propagation (pkg/transport); request correlation. |
| AU-4(1) | Transfer to Alternate Storage | GAP | No off-system audit-log archive. |
| AU-5(1) | Real-Time Alerts | PARTIAL | Sentinel daily detection. Gap: real-time alerting on logging-system failures specifically not wired. |
| AU-5(2) | Real-Time Alerts on Audit Failure | GAP | No alert-on-logging-failure path. |
| AU-6(1) | Automated Process Integration | PARTIAL | Wotan log-aggregation + Sentinel automation. |
| AU-6(3) | Correlate Audit Repositories | PARTIAL | Wotan ring buffer aggregates. Gap: no SIEM correlation across hosts. |
| AU-6(4) | Central Review and Analysis | PARTIAL | Sentinel daily detection. |
| AU-6(5) | Integration / Scanning & Monitoring | PARTIAL | Suricata IDS + Mímir + eBPF + security.yml + sbom-audit.yml. **SEN5 caveat:** Suricata is integration code (`pkg/anamnesis/`) — deployment to actual hosts (WEST, EAST) is UNVERIFIED. See sensor deployment audit (`docs/security/sensor-deployment-audit-2026-05-06.md`). |
| AU-6(6) | Correlation with Physical Monitoring | ADOPTER-OWNS | n/a to substrate. |
| AU-7(1) | Automatic Sort & Search | MAPPED | Dashboard log query + SSE live tail. |
| AU-8(1) | Synchronization with Authoritative Time Source | UNVERIFIED | Container NTP via host. |
| AU-9(2) | Audit Backup on Separate Physical System / Component | GAP | No off-system audit-log replication. |
| AU-9(3) | Cryptographic Protection | PARTIAL | ML-DSA-65 on signed config.* topics. Gap: drift.* + audit.* signing-scope (per heimdall TODO #2 parked tonight). |
| AU-9(4) | Access by Subset of Privileged Users | PARTIAL | The Well RBAC. Gap: not formally documented for audit logs specifically. |
| AU-11(1) | Long-Term Retrieval Capability | GAP | No long-term archive. |
| AU-12(1) | System-Wide / Time-Correlated Audit Trail | MAPPED | trace_id propagation across all services. |
| AU-12(3) | Changes by Authorized Individuals | PARTIAL | git history + ADR cadence trace authorized changes. |

**FedRAMP AU parameters:**
- AU-2 events to log: FedRAMP minimum includes successful + unsuccessful logon, privilege escalation, system administration, account creation/deletion/modification — kingdom logs auth events via pkg/auth.AuditLogger; Champion logs gated mutations; eBPF logs network events.
- AU-4 storage capacity: not parameterized; FedRAMP expects sufficient for 90 days online + 1 year archive.
- AU-11 retention: FedRAMP minimum **90 days online + 1 year archive**. Kingdom: GAP — no retention SOP.

### CA — Assessment, Authorization, Monitoring

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| CA-2(1) | Independent Assessors | GAP | No 3PAO. |
| CA-2(3) | Leveraging Results from External Organizations | GAP | None. |
| CA-3(6) | Transfer of Information | GAP | No ISA. |
| CA-7(1) | Independent Assessment | GAP | None. |
| CA-7(4) | Risk Monitoring | PARTIAL | Sentinel daily; Lich Hardening; threat models. |
| CA-8(1) | Independent Penetration Agent | GAP | No 3PAO pen-test. |
| CA-8(2) | Red Team Exercises | PARTIAL | BlackMage daily red-team adversarial loop with Zhen AI. Gap: not 3PAO. **SEN7 disclaimer:** INTERNAL validation only; NOT a substitute for the independent third-party assessment required by FedRAMP CA-8. |

### CM — Configuration Management

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| CM-2(2) | Automation Support for Accuracy & Currency | MAPPED | ADR-052 drift policy + Mímir + Sealed Cask. |
| CM-2(3) | Retention of Previous Configurations | MAPPED | git history; Sealed Cask deployment archives. |
| CM-2(7) | Configure Systems for High-Risk Areas | PARTIAL | Container hardening + NetworkPolicy. |
| CM-3(1) | Automated Documentation, Notification, & Prohibition of Changes | MAPPED | Marshal enforcement + ADR cadence + CI gates. |
| CM-3(2) | Test, Validate, & Document Changes | MAPPED | TDD discipline (512 tests) + CI gates. |
| CM-3(4) | Security & Privacy Representatives | PARTIAL | The 19 Seats — BlackMage / MoatGhost / Sentinel. |
| CM-3(6) | Cryptography Management | MAPPED | ML-DSA-65 + SLH-DSA + TLS 1.3; ADR-043 ML-DSA-65 hard condition. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| CM-4(1) | Separate Test Environments | PARTIAL | values-{dev,prod}.yaml; Docker Compose dev. |
| CM-4(2) | Verification of Controls | PARTIAL | CI verification. |
| CM-5(1) | Automated Access Enforcement & Audit Records | MAPPED | git access controls + Champion audit log. |
| CM-5(2) | Review System Changes | PARTIAL | Code review via PR; ADR review. |
| CM-5(5) | Privilege Limitation for Production / Operational Changes | PARTIAL | Champion gate. |
| CM-6(1) | Automated Management, Application, & Verification | MAPPED | helm chart + Sealed Cask + Mímir. |
| CM-6(2) | Respond to Unauthorized Changes | PARTIAL | Mímir alerts-only (per ADR-043 hard condition #1: NO RESTORE). Auditor may flag this — document the alerts-only design choice. |
| CM-7(1) | Periodic Review | PARTIAL | ADR cadence. |
| CM-7(2) | Prevent Program Execution | PARTIAL | Container hardening + seccomp + container runtime restrictions. |
| CM-7(5) | Authorized Software | MAPPED | Sealed Cask SHA-256 + verify-binding-rune.sh + GPL boundary. |
| CM-8(1) | Updates During Installation/Removal | MAPPED | helm + Sealed Cask. |
| CM-8(2) | Automated Maintenance | MAPPED | Same. |
| CM-8(3) | Automated Unauthorized Component Detection | MAPPED | Mímir + verify-binding-rune.sh. |
| CM-8(4) | Accountability Information | PARTIAL | git history; Co-Authored-By trailers. |
| CM-10(1) | Open Source Software | MAPPED | GPL-3.0-or-later license; SPDX coverage 99.5%; SBOM 553 deps audited. |
| CM-12(1) | Automated Tools to Support Information Location | PARTIAL | The Well + Doom Range registry. |

### CP — Contingency Planning

| Ctrl | Enhancement | Status |
|------|-------------|--------|
| CP-2(1) | Coordinate with Related Plans | GAP |
| CP-2(2) | Capacity Planning | PARTIAL (HPAs, PDBs) |
| CP-2(3) | Resume Mission and Business Functions | GAP |
| CP-2(8) | Identify Critical Assets | GAP |
| CP-3(1) | Simulated Events | GAP |
| CP-4(1) | Coordinate with Related Plans | GAP |
| CP-6(1) | Separation from Primary Site | ADOPTER-OWNS |
| CP-6(3) | Accessibility | ADOPTER-OWNS |
| CP-7(1) | Separation from Primary Site | ADOPTER-OWNS |
| CP-7(2) | Accessibility | ADOPTER-OWNS |
| CP-7(3) | Priority of Service | ADOPTER-OWNS |
| CP-8(1) | Priority of Service Provisions | ADOPTER-OWNS |
| CP-8(2) | Single Points of Failure | PARTIAL — ADR-064 active/active speced (deferred per Stevie) |
| CP-9(1) | Testing for Reliability and Integrity | GAP |
| CP-9(2) | Test Restoration Using Sampling | GAP |
| CP-9(3) | Separate Storage for Critical Information | GAP |
| CP-9(5) | Transfer to Alternate Storage Site | GAP |
| CP-9(8) | Cryptographic Protection | PARTIAL (ML-DSA-65 signing on config; not all topics) |
| CP-10(2) | Transaction Recovery | GAP |
| CP-10(4) | Restore Within Time Period | GAP |

(CP family is the single largest aggregate gap. Closing it is a focused 4-6 week MoatGhost + Architect effort once IR plan lands.)

### IA — Identification and Authentication

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| IA-2(1) | MFA to Privileged Accounts | UNVERIFIED | JWT can carry MFA assertion; not enforced uniformly. |
| IA-2(2) | MFA to Non-Privileged Accounts | UNVERIFIED | Same. |
| IA-2(5) | Individual Authentication with Group Authentication | GAP | No group-auth scheme. |
| IA-2(6) | Access to Accounts — Separate Device | UNVERIFIED | Adopter MFA path. |
| IA-2(8) | Access to Accounts — Replay Resistance | MAPPED | TLS 1.3 forward secrecy + JWT nonce. |
| IA-2(12) | Acceptance of PIV Credentials | GAP | No PIV/CAC integration. |
| IA-3(1) | Cryptographic Bidirectional Authentication | PARTIAL | mTLS available. |
| IA-4(4) | Identify User Status | GAP | No formal user-status registry. |
| IA-5(1) | Password-Based Authentication | N/A — APIKey/JWT only |
| IA-5(2) | Public Key-Based Authentication | MAPPED | mTLS + JWT signing keys. |
| IA-5(6) | Protection of Authenticators | MAPPED | SOPS+age for secrets. |
| IA-5(7) | No Embedded Unencrypted Static Authenticators | MAPPED | SOPS+age — secrets never in plaintext in repo. |
| IA-6(2) | Protection of Authentication Information | MAPPED | TLS 1.3 + SOPS+age. |
| IA-7 | Cryptographic Module Authentication | MAPPED | FIPS-track via cloudflare/circl v1.6.3 (ML-DSA-65 + SLH-DSA). **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| IA-8(1) | Acceptance of PIV Credentials from Other Agencies | GAP |
| IA-8(2) | Acceptance of External Authenticators | PARTIAL | OAuth 2.0 federation via JWT path. |
| IA-8(4) | Use of NIST-Approved Cryptography | MAPPED | ML-DSA-65 (FIPS 204), SLH-DSA (FIPS 205), TLS 1.3. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| IA-11 | Re-Authentication | UNVERIFIED | JWT TTL forces re-auth at expiry. **FedRAMP suggests within `[15 minutes]` for privileged actions.** |
| IA-12(2) | Identity Evidence | GAP |
| IA-12(3) | Identity Evidence Validation & Verification | GAP |
| IA-12(4) | In-Person Validation & Verification | GAP |
| IA-12(5) | Address Confirmation | GAP |

### IR — Incident Response

(Family-wide GAP. Closing IR-1, IR-2, IR-3, IR-4, IR-5(1), IR-6, IR-7, IR-8 is the highest-leverage cross-framework work.)

| Ctrl | Enhancement | Status |
|------|-------------|--------|
| IR-2(1) | Simulated Events | GAP |
| IR-2(2) | Automated Training Environments | GAP |
| IR-3(2) | Coordination with Related Plans | GAP |
| IR-4(1) | Automated Incident Handling Processes | PARTIAL — Sentinel detection automation; no handling automation |
| IR-4(3) | Continuity of Operations | GAP |
| IR-4(4) | Information Correlation | PARTIAL — Sentinel + Wotan log aggregation |
| IR-5(1) | Automated Tracking, Data Collection, & Analysis | PARTIAL — Sentinel; Anamnesis Lite |
| IR-6(1) | Automated Reporting | GAP |
| IR-7(1) | Automation Support for Availability of Information / Support | GAP |
| IR-8(1) | Breaches | GAP |

### MA — Maintenance

| Ctrl | Enhancement | Status |
|------|-------------|--------|
| MA-2 | Controlled Maintenance | N/A — SaaS substrate |
| MA-4(2) | Document Nonlocal Maintenance | N/A |
| MA-4(3) | Comparable Security & Sanitization | N/A |
| MA-5(1) | Individuals Without Appropriate Access | N/A |
| MA-6 | Timely Maintenance | N/A |

(FedRAMP often relabels MA → patch & dependency management for SaaS — that's covered under SI-2.)

### MP — Media Protection

(Entire family ADOPTER-OWNS / N/A for software-only substrate. Adopter / cloud provider handles physical media.)

### PE — Physical and Environmental Protection

(Entire family ADOPTER-OWNS. Hosting choice. Note: cloud-IaaS adopters typically inherit the IaaS provider's PE controls — document this inheritance for any agency tenant.)

### PL — Planning

| Ctrl | Enhancement | Status |
|------|-------------|--------|
| PL-2(3) | Plan / Coordinate with Other Organizational Entities | GAP |
| PL-4(1) | Social Media & External Site/Application Usage | GAP |
| PL-8(1) | Defense-in-Depth | MAPPED — 6-armor-layer architecture (per CLAUDE.md), gateway → WAF (cmd/shield) → service auth → Champion gate → Wotan signing → eBPF traceability |
| PL-8(2) | Suppliers of Security & Privacy Components | PARTIAL — SBOM, GPL boundary |

### PM — Program Management

(Family heavily organizational; mostly GAP for single-operator. See E5 NIST 800-53 §PM section. FedRAMP-specific: PM-30 SCRM strategy is required but covered by tonight's SBOM + GPL boundary work as substrate; needs formal SCRM plan doc to fully close.)

### PS — Personnel Security

(Family heavily organizational; mostly GAP for single-operator. Document accepted-gap with US-person attestation.)

### PT — PII Processing and Transparency

(Mostly N/A-DESIGN — Unheaded does not process PII. PT-5 Privacy Notice is the GAP that pairs with GDPR Art. 12.)

### RA — Risk Assessment

| Ctrl | Enhancement | Status |
|------|-------------|--------|
| RA-3(1) | Supply Chain Risk Assessment | PARTIAL — SBOM + GPL boundary |
| RA-5(2) | Update Vulnerabilities to be Scanned | MAPPED — sbom-audit.yml weekly + security.yml daily |
| RA-5(5) | Privileged Access | PARTIAL |
| RA-5(11) | Public Disclosure Program | GAP — no security.txt, no responsible-disclosure policy |
| RA-7 | Risk Response | PARTIAL |
| RA-9 | Criticality Analysis | GAP |
| RA-10 | Threat Hunting | PARTIAL — BlackMage daily red-team. **SEN7 disclaimer:** INTERNAL validation only; NOT a substitute for the independent third-party assessment required by FedRAMP CA-8. |

**FedRAMP RA parameters:**
- RA-5 scan frequency: **monthly minimum** for OS, DB, Web. Kingdom exceeds with security.yml daily. **PARAM met.**
- RA-5 critical CVE remediation: 30 days. Kingdom: GAP (no SLA documented).
- RA-5 high CVE remediation: 90 days. Kingdom: GAP.

### SA — System and Services Acquisition

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| SA-3(1) | Manage Preproduction Environments | PARTIAL — values-dev/prod separation |
| SA-3(2) | Use of Live or Operational Data | MAPPED — no PII used in test/training (architectural) |
| SA-4(1) | Functional Properties of Controls | MAPPED — pkg/auth has documented properties |
| SA-4(2) | Design / Implementation Information for Controls | MAPPED — ADRs |
| SA-4(8) | Continuous Monitoring Plan for Controls | GAP |
| SA-4(9) | Functions / Ports / Protocols / Services in Use | MAPPED — Doom Range registry |
| SA-4(10) | Use of Approved PIV Products | GAP |
| SA-8 | Security & Privacy Engineering Principles | MAPPED — CLAUDE.md "Security First, Always" |
| SA-9(1) | Risk Assessments / Organizational Approvals | PARTIAL |
| SA-9(2) | Identification of Functions / Ports / Protocols / Services | MAPPED |
| SA-9(4) | Consistent Interests of Consumers & Producers | GAP |
| SA-9(5) | Processing, Storage, and Service Location | UNVERIFIED — adopter selects |
| SA-10(1) | Software / Firmware Integrity Verification | MAPPED — Sealed Cask + verify-binding-rune.sh |
| SA-10(2) | Alternative Configuration Management Processes | N/A |
| SA-11(1) | Static Code Analysis | MAPPED — go vet + golangci-lint + cargo clippy |
| SA-11(2) | Threat Modeling & Vulnerability Analyses | PARTIAL — tonight's K8s threat model is example; not regular cadence |
| SA-11(4) | Manual Code Reviews | PARTIAL — PR review |
| SA-11(8) | Dynamic Code Analysis | PARTIAL — security.yml runs gosec etc. |
| SA-15(3) | Criticality Analysis | GAP |
| SA-15(7) | Automated Vulnerability Analysis | MAPPED — security.yml + sbom-audit.yml |
| SA-22 | Unsupported System Components | UNVERIFIED |

### SC — System and Communications Protection

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| SC-5(1) | Restrict Ability to Attack Other Systems | PARTIAL — outbound NetworkPolicy egress restriction (allow-internal + DNS only) |
| SC-7(3) | Access Points | MAPPED — gateway TLS + NodePorts documented |
| SC-7(4) | External Telecommunications Services | PARTIAL — TLS 1.3 + NetworkPolicy |
| SC-7(5) | Deny by Default — Allow by Exception | PARTIAL — default-deny NetworkPolicy template. **SEN8 caveat:** NetworkPolicy enforcement on default kindnet CNI is unproven (per K8s threat model §3.3); requires Calico/Cilium switch OR live smoke test before promoting to MAPPED. |
| SC-7(7) | Split Tunneling for Remote Devices | UNVERIFIED |
| SC-7(8) | Route Traffic to Authenticated Proxy Servers | PARTIAL — HAProxy edge |
| SC-7(18) | Fail Secure | MAPPED — services fail closed on auth/champion errors |
| SC-7(20) | Dynamic Isolation / Segregation | PARTIAL — NetworkPolicy + Wotan topic separation |
| SC-7(21) | Isolation of System Components | MAPPED — microservice + container isolation |
| SC-8(1) | Cryptographic Protection | MAPPED — TLS 1.3 |
| SC-8(2) | Pre/Post Transmission Handling | MAPPED — TLS record-level integrity + ML-DSA-65. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| SC-10 | Network Disconnect | PARTIAL — TLS timeouts |
| SC-12(1) | Availability | PARTIAL |
| SC-12(2) | Symmetric Keys | MAPPED — TLS 1.3 |
| SC-12(3) | Asymmetric Keys | MAPPED — TLS 1.3 + ML-DSA-65. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| SC-13 | Cryptographic Protection | MAPPED — TLS 1.3, ML-DSA-65, SLH-DSA. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| SC-15(1) | Physical Disconnect | N/A |
| SC-17 | Public Key Infrastructure Certificates | PARTIAL — TLS 1.3; cert-manager planned |
| SC-18(1) | Identify Unacceptable Code & Take Corrective Actions | PARTIAL |
| SC-18(2) | Acquisition / Development / Use | PARTIAL |
| SC-18(3) | Prevent Downloading / Execution | PARTIAL — container hardening |
| SC-18(4) | Prevent Automatic Execution | PARTIAL |
| SC-20(1) | Child Subspaces | N/A — DNSSEC for adopter zones |
| SC-21 | Secure Name/Address Resolution Service (Recursive) | UNVERIFIED |
| SC-22 | Architecture & Provisioning for Name/Address Resolution Service | UNVERIFIED |
| SC-23(1) | Invalidate Session Identifiers at Logout | PARTIAL |
| SC-23(3) | Unique System-Generated Session Identifiers | MAPPED — JWT jti claims; trace_id |
| SC-23(5) | Allowed Certificate Authorities | UNVERIFIED |
| SC-28(1) | Cryptographic Protection | PARTIAL — SOPS+age for secrets at rest. **Gap: The Well at-rest encryption + etcd encryption-at-rest both not yet wired** (cross-ref tonight's K8s threat model §3.2) |
| SC-39(1) | Hardware Separation | UNVERIFIED |

**FedRAMP SC parameters:**
- SC-7 boundary protection: external/internal designation must be documented; FedRAMP requires explicit DMZ.
- SC-12 cryptographic key establishment: NIST SP 800-56A/B/C compliance + ML-DSA-65 PQC alignment.
- SC-13 cryptography: FIPS 140-3 validated modules required for federal use. **PARAM-GAP**: cloudflare/circl is FIPS-track but not FIPS-validated as a discrete module — need to verify or replace with a FIPS-validated path.

### SI — System and Information Integrity

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| SI-2(2) | Automated Flaw Remediation Status | MAPPED — sbom-audit.yml + security.yml + Sentinel |
| SI-2(3) | Time to Remediate Flaws / Benchmarks for Corrective Actions | GAP — no SLA |
| SI-3(7) | Non-Signature-Based Detection | PARTIAL — Suricata anomaly + Sentinel ML signals |
| SI-3(10) | Malicious Code Analysis | PARTIAL |
| SI-4(2) | Automated Tools / Mechanisms for Real-Time Analysis | MAPPED — eBPF + Sentinel |
| SI-4(4) | Inbound / Outbound Communications Traffic | PARTIAL — Anamnesis Lite + flow-tracker. **SEN3 caveat:** Effective retention ~10 seconds at moderate traffic rate (10K-entry default); below FedRAMP / CIS 8.10 floor of 90 days online. Detection without retention is not coverage. |
| SI-4(5) | System-Generated Alerts | PARTIAL — Sentinel; Mímir alerts-only |
| SI-4(11) | Analyze Communications Traffic Anomalies | PARTIAL — anomaly-ebpf, flow-tracker. **SEN3 caveat:** Effective retention ~10 seconds at moderate traffic rate (10K-entry default); below FedRAMP / CIS 8.10 floor of 90 days online. Detection without retention is not coverage. |
| SI-4(12) | Automated Organization-Generated Alerts | PARTIAL |
| SI-4(14) | Wireless Intrusion Detection | N/A |
| SI-4(16) | Correlate Monitoring Information | PARTIAL — Sentinel correlation; no SIEM |
| SI-4(18) | Analyze Traffic / Covert Exfiltration | PARTIAL — anomaly-ebpf |
| SI-4(19) | Risk for Individuals | GAP |
| SI-4(20) | Privileged Users | PARTIAL — Champion gate audit |
| SI-4(22) | Unauthorized Network Services | PARTIAL — Doom Range registry as inventory |
| SI-4(23) | Host-Based Devices | PARTIAL — Mímir host monitoring |
| SI-5(1) | Automated Alerts and Advisories | MAPPED — NIST NVD + CISA KEV via MCP |
| SI-7(1) | Integrity Checks | MAPPED — Sealed Cask + verify-binding-rune.sh + Mímir |
| SI-7(2) | Automated Notifications of Integrity Violations | PARTIAL — Mímir alerts-only (per ADR-043 hard condition #1: NO RESTORE) |
| SI-7(5) | Automated Response to Integrity Violations | PARTIAL — alerts-only design choice (FedRAMP auditor will scrutinize) |
| SI-7(7) | Integration of Detection & Response | PARTIAL |
| SI-7(8) | Auditing Capability for Significant Events | MAPPED |
| SI-7(15) | Code Authentication | MAPPED — Sealed Cask SHA-256 + ML-DSA-65 signing. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| SI-8(1) | Central Management | N/A — n/a to substrate |
| SI-8(2) | Automatic Updates | N/A |
| SI-10(1) | Manual Override Capability | PARTIAL — Champion gate has explicit pending-confirmation override |
| SI-12(1) | Limit PII Elements | MAPPED — architectural |
| SI-16 | Memory Protection | MAPPED — Rust + container hardening + seccomp |

### SR — Supply Chain Risk Management

| Ctrl | Enhancement | Status | Evidence / Gap |
|------|-------------|--------|----------------|
| SR-2(1) | Establish SCRM Team | GAP |
| SR-3(1) | Diverse Supply Base | GAP |
| SR-3(2) | Limit Harm from Components Acquired from Untrusted Suppliers | PARTIAL — verify-gpl-boundary + SBOM |
| SR-3(3) | Sub-Tier Flow-Down | GAP |
| SR-5(1) | Adequate Supply | GAP |
| SR-5(2) | Assessments Prior to Selection / Acceptance / Modification / Update | PARTIAL — ADR-004 dependency policy |
| SR-6(1) | Testing & Analysis | PARTIAL |
| SR-7 | Supply Chain Operations Security | PARTIAL — Sealed Cask |
| SR-9(1) | Multiple Stages of System Development Life Cycle | PARTIAL |
| SR-10 | Inspection of Systems or Components | MAPPED — bpf-verifier-check.sh, CIS k8s-bench |
| SR-11(1) | Anti-Counterfeit Training | GAP |
| SR-11(2) | Configuration Control for Component Service & Repair | N/A |
| SR-11(3) | Anti-Counterfeit Scanning | PARTIAL — Sealed Cask SHA-256 verification on every deployment |

---

## Headline FedRAMP Moderate gaps (priority order for ATO trajectory)

1. **Personnel sponsoring (Agency Authorizing Official)** — without an agency sponsor, no ATO. This is upstream of all technical work.
2. **3PAO selection + assessment** — must be FedRAMP-recognized 3PAO (not generic Big 4); ~$200K-$500K assessment cost.
3. **CP-2 + IR-8 plans** — every assessment will fail without these; both also close GDPR/HIPAA/PCI/SOC2 universal gaps.
4. **AC-8 system use notification banner** — single-line federal-compliance text; trivial fix.
5. **SC-13 / FIPS 140-3 module attestation** — verify cloudflare/circl FIPS-track posture against current FIPS 140-3 module list. May require swap-out for a FIPS-validated module.
6. **AU-11 retention** — minimum 90 days online + 1 year archive.
7. **RA-5 vulnerability remediation SLA** — 30/90/180-day SLAs by severity.
8. **CA-7 ConMon strategy doc + monthly POA&M** — not optional for FedRAMP.
9. **PT-5 Privacy Notice** — closes GDPR Art. 12 too.
10. **US-person attestation** — single-page sole-operator US-person declaration.

## Strongest mapped families for FedRAMP delta

The kingdom's FedRAMP-ready bedrock is in **AC** (RBAC + Champion + auth), **AU** (eBPF + zerolog + Wotan log agg), **CM** (Sealed Cask + Mímir + ADR-052), **IA** (pkg/auth Noop/APIKey/JWT), **SC** (TLS 1.3 + ML-DSA-65 + NetworkPolicy + boundary), **SI** (Mímir + Sealed Cask + Suricata + Sentinel), **SR** (SBOM + GPL boundary + SPDX). Roughly 65-75% MAPPED at parent-control level in these seven.

The headwind is in **AT, CP, IR, PM, PS** — heavily organizational, single-operator-kingdom GAP. These don't close with engineering; they close with documentation + (eventually) headcount.

## ATO realistic timeline (if Captain Track A or B activates)

1. **Months 0-2:** SSP authoring (using these matrices as input), CP-2 / IR-8 / AC-8 / AU-11 closures.
2. **Months 2-4:** 3PAO selection + RFP + scoping engagement.
3. **Months 4-9:** 3PAO assessment activities; remediation cycles.
4. **Months 9-12:** SAR delivery; FedRAMP PMO review; agency AO sponsorship.
5. **Month 12-18:** ATO award (if all goes well); ConMon enters monthly cadence.

Tonight's matrix is **month 0**. Useful, real, foundational — but the road is long.

## Verification

```bash
ls docs/compliance/control-matrix/                                     # this matrix family
bash scripts/verify-gpl-boundary.sh                                     # SR-3 evidence
bash scripts/check-timeline-freshness.sh --check                        # CM-3 evidence
ls pkg/{auth,champion,discovery,transport,logagg,bpf}                   # AC + AU + IA + SC bedrock
```

---

## Provenance

Read-only audit. Sources: NIST SP 800-53 Rev 5 catalog, FedRAMP Rev 5 Moderate Baseline (effective May 2023), CLAUDE.md, tonight's K8s threat model + RBAC review, ADRs as cited.
