# CIS Controls v8 (with v8.1 update) Coverage Matrix

**Date:** 2026-05-06
**Author:** Marshal post-shift extension (NORTH-STAR Appendix A Phase E12)
**Reference:** Center for Internet Security (CIS) Controls v8 (May 2021) + v8.1 (June 2024) — 18 Controls × ~153 Safeguards across IG1/IG2/IG3 implementation groups
**Status legend:** **MAPPED** = full coverage with evidence | **PARTIAL** = some coverage, gap noted | **GAP** = no kingdom work yet | **N/A** = not applicable | **DEFERRED** = planned, not shipped | **UNVERIFIED** = MoatGhost confirm

**Implementation Group target:** **IG2** (mid-range — appropriate for pre-funding kingdom with engineering rigor) is realistic. **IG1** (basic — ~56 safeguards) is achievable with the gaps below closed. **IG3** (advanced — all 153 safeguards including dedicated security teams + tooling) is post-headcount.

**Sentinel's lane:** CIS Controls is the canonical blue-team baseline. Sentinel + BlackMage operate against this catalog by default.

---

## Control 01 — Inventory and Control of Enterprise Assets

| SG | Title | IG | Status | Evidence / Gap |
|----|-------|----|---------|----------------|
| 1.1 | Establish and Maintain Detailed Enterprise Asset Inventory | 1 | PARTIAL | Bare-metal WEST + EAST documented; Doom Range port registry. Gap: no formal asset-management database. |
| 1.2 | Address Unauthorized Assets | 1 | PARTIAL | Mímir baseline drift detection (REAL-METAL). |
| 1.3 | Utilize an Active Discovery Tool | 2 | PARTIAL | pkg/discovery 4-layer service registration. |
| 1.4 | Use DHCP Logging to Update Asset Inventory | 2 | UNVERIFIED |
| 1.5 | Use a Passive Asset Discovery Tool | 3 | PARTIAL | eBPF flow-tracker observes asset connections. |

## Control 02 — Inventory and Control of Software Assets

| SG | Title | IG | Status | Evidence / Gap |
|----|-------|----|---------|----------------|
| 2.1 | Establish and Maintain a Software Inventory | 1 | MAPPED | SBOM (sbom/SBOM.md + tonight's delta); 553 deps + lockfile fingerprints. |
| 2.2 | Ensure Authorized Software is Currently Supported | 1 | UNVERIFIED |
| 2.3 | Address Unauthorized Software | 1 | MAPPED | Sealed Cask SHA-256 + verify-binding-rune.sh + SPDX coverage 99.5% + verify-gpl-boundary.sh (patched tonight). |
| 2.4 | Utilize Automated Software Inventory Tools | 2 | MAPPED | sbom-audit.yml weekly; tonight's delta SBOM. |
| 2.5 | Allowlist Authorized Software | 2 | MAPPED | Sealed Cask binding rune. |
| 2.6 | Allowlist Authorized Libraries | 2 | PARTIAL | go.sum + Cargo.lock pinning. Gap: no formal "allowlist" semantic — use "lockfile-pinned" framing. |
| 2.7 | Allowlist Authorized Scripts | 3 | PARTIAL | scripts/ tracked in git; verify-binding-rune.sh covers shipped scripts. |

## Control 03 — Data Protection

| SG | Title | IG | Status | Evidence / Gap |
|----|-------|----|---------|----------------|
| 3.1 | Establish and Maintain a Data Management Process | 1 | GAP | No formal data-management process. |
| 3.2 | Establish and Maintain a Data Inventory | 1 | PARTIAL | The Well 3 PG databases. Gap: no formal data inventory. |
| 3.3 | Configure Data Access Control Lists | 1 | MAPPED | The Well 7 service-scoped users; pkg/auth + RBAC. |
| 3.4 | Enforce Data Retention | 1 | GAP | No retention SOP (cross-framework gap). |
| 3.5 | Securely Dispose of Data | 1 | GAP | No disposal SOP. |
| 3.6 | Encrypt Data on End-User Devices | 1 | ADOPTER-OWNS |
| 3.7 | Establish and Maintain a Data Classification Scheme | 2 | GAP | No classification scheme. |
| 3.8 | Document Data Flows | 2 | PARTIAL | Tonight's K8s threat model trust zones; Wotan topic flows. Gap: no formal data-flow diagram. |
| 3.9 | Encrypt Data on Removable Media | 2 | ADOPTER-OWNS |
| 3.10 | Encrypt Sensitive Data in Transit | 2 | MAPPED | TLS 1.3 mandatory + ML-DSA-65 PQC. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| 3.11 | Encrypt Sensitive Data at Rest | 2 | PARTIAL | SOPS+age for secrets. **Gap: The Well + etcd at-rest encryption not yet wired** (cross-ref tonight's K8s threat model §3.2). |
| 3.12 | Segment Data Processing and Storage Based on Sensitivity | 2 | PARTIAL | The Well 3-database trust separation. |
| 3.13 | Deploy a Data Loss Prevention Solution | 3 | GAP |
| 3.14 | Log Sensitive Data Access | 3 | PARTIAL | pkg/auth.AuditLogger; not specifically scoped to "sensitive data." |

## Control 04 — Secure Configuration of Enterprise Assets and Software

| SG | Title | IG | Status | Evidence / Gap |
|----|-------|----|---------|----------------|
| 4.1 | Establish and Maintain a Secure Configuration Process | 1 | MAPPED | ADR-007 + ADR-008 + ADR-052 drift policy + Sealed Cask. |
| 4.2 | Establish and Maintain a Secure Configuration Process for Network Infrastructure | 1 | PARTIAL | NetworkPolicy default-deny + HAProxy + per-service nginx + (planned) WireGuard. **SEN8 caveat:** NetworkPolicy enforcement on default kindnet CNI is unproven (per K8s threat model §3.3); requires Calico/Cilium switch OR live smoke test before promoting to MAPPED. |
| 4.3 | Configure Automatic Session Locking on Enterprise Assets | 1 | UNVERIFIED |
| 4.4 | Implement and Manage a Firewall on Servers | 1 | PARTIAL | Per-container firewall via NetworkPolicy + nginx WAF + cmd/shield. **SEN8 caveat:** NetworkPolicy enforcement on default kindnet CNI is unproven (per K8s threat model §3.3); requires Calico/Cilium switch OR live smoke test before promoting to MAPPED. |
| 4.5 | Implement and Manage a Firewall on End-User Devices | 1 | ADOPTER-OWNS |
| 4.6 | Securely Manage Enterprise Assets and Software | 1 | MAPPED | Sealed Cask + Mímir + helm + ADR cadence. |
| 4.7 | Manage Default Accounts on Enterprise Assets and Software | 1 | MAPPED | NoopAuthenticator dev-only; APIKey/JWT for non-dev. |
| 4.8 | Uninstall or Disable Unnecessary Services | 2 | PARTIAL | Container minimization. |
| 4.9 | Configure Trusted DNS Servers | 2 | UNVERIFIED |
| 4.10 | Enforce Automatic Device Lockout on Portable End-User Devices | 2 | ADOPTER-OWNS |
| 4.11 | Enforce Remote Wipe Capability | 2 | ADOPTER-OWNS |
| 4.12 | Separate Enterprise Workspaces on Mobile End-User Devices | 3 | ADOPTER-OWNS |

## Control 05 — Account Management

| SG | Title | IG | Status | Evidence / Gap |
|----|-------|----|---------|----------------|
| 5.1 | Establish and Maintain an Inventory of Accounts | 1 | PARTIAL | The Well 7 service-scoped users + per-service SAs in some Deployments. Gap: no rolled-up inventory. |
| 5.2 | Use Unique Passwords | 1 | N/A | APIKey/JWT, no passwords. |
| 5.3 | Disable Dormant Accounts | 1 | PARTIAL | JWT TTL forces re-auth. |
| 5.4 | Restrict Administrator Privileges to Dedicated Administrator Accounts | 1 | PARTIAL | Champion gate distinguishes. **Gap: tonight's RBAC review F1 over-grant.** |
| 5.5 | Establish and Maintain an Inventory of Service Accounts | 2 | PARTIAL | per-service SAs in some Deployments; **Gap from RBAC review F3:** named SAs (pleroma, anamnesis, kenoma, cuirass) are referenced but `kind: ServiceAccount` resources not in tree. |
| 5.6 | Centralize Account Management | 2 | GAP | No central ID service. |

## Control 06 — Access Control Management

| SG | Title | IG | Status | Evidence / Gap |
|----|-------|----|---------|----------------|
| 6.1 | Establish an Access Granting Process | 1 | PARTIAL | pkg/auth scaffolding. |
| 6.2 | Establish an Access Revoking Process | 1 | PARTIAL | pkg/auth supports revocation; gap is process. |
| 6.3 | Require MFA for Externally-Exposed Applications | 1 | UNVERIFIED |
| 6.4 | Require MFA for Remote Network Access | 1 | UNVERIFIED |
| 6.5 | Require MFA for Administrative Access | 1 | UNVERIFIED |
| 6.6 | Establish and Maintain an Inventory of Authentication and Authorization Systems | 2 | MAPPED | pkg/auth (3 authenticators) + RBACAuthorizer + Champion. |
| 6.7 | Centralize Access Control | 2 | PARTIAL | Champion gate is the de-facto central PEP for mutations. |
| 6.8 | Define and Maintain Role-Based Access Control | 3 | PARTIAL | RBAC scaffolding. **Gap: F1 over-grant.** |

## Control 07 — Continuous Vulnerability Management

| SG | Title | IG | Status |
|----|-------|----|---------|
| 7.1 | Establish and Maintain a Vulnerability Management Process | 1 | PARTIAL — sbom-audit.yml + security.yml + Sentinel |
| 7.2 | Establish and Maintain a Remediation Process | 1 | GAP — no SLA |
| 7.3 | Perform Automated Operating System Patch Management | 1 | UNVERIFIED |
| 7.4 | Perform Automated Application Patch Management | 1 | PARTIAL — Sentinel + sbom-audit |
| 7.5 | Perform Automated Vulnerability Scans of Internal Enterprise Assets | 2 | MAPPED — security.yml daily |
| 7.6 | Perform Automated Vulnerability Scans of Externally-Exposed Enterprise Assets | 2 | PARTIAL |
| 7.7 | Remediate Detected Vulnerabilities | 2 | PARTIAL |

## Control 08 — Audit Log Management

| SG | Title | IG | Status |
|----|-------|----|---------|
| 8.1 | Establish and Maintain an Audit Log Management Process | 1 | PARTIAL |
| 8.2 | Collect Audit Logs | 1 | PARTIAL — pkg/auth.AuditLogger + Wotan log aggregation + eBPF + zerolog. **SEN3 caveat:** Effective retention ~10 seconds at moderate traffic rate (10K-entry default); below FedRAMP / CIS 8.10 floor of 90 days online. Detection without retention is not coverage. |
| 8.3 | Ensure Adequate Audit Log Storage | 1 | PARTIAL — Wotan ring buffer 10K-entry default |
| 8.4 | Standardize Time Synchronization | 2 | UNVERIFIED |
| 8.5 | Collect Detailed Audit Logs | 2 | MAPPED — trace_id propagation |
| 8.6 | Collect DNS Query Audit Logs | 2 | UNVERIFIED |
| 8.7 | Collect URL Request Audit Logs | 2 | MAPPED |
| 8.8 | Collect Command-Line Audit Logs | 2 | PARTIAL |
| 8.9 | Centralize Audit Logs | 2 | PARTIAL — Wotan log aggregation |
| 8.10 | Retain Audit Logs | 2 | GAP — no retention SOP |
| 8.11 | Conduct Audit Log Reviews | 2 | PARTIAL — Sentinel daily |
| 8.12 | Collect Service Provider Logs | 3 | UNVERIFIED |

## Control 09 — Email and Web Browser Protections

| SG | Title | IG | Status |
|----|-------|----|---------|
| 9.1 — 9.7 | (entire control) | 1-3 | N/A — not an email or browser-shipping organization |

## Control 10 — Malware Defenses

| SG | Title | IG | Status |
|----|-------|----|---------|
| 10.1 | Deploy and Maintain Anti-Malware Software | 1 | PARTIAL — Suricata IDS; Sentinel |
| 10.2 | Configure Automatic Anti-Malware Signature Updates | 1 | UNVERIFIED |
| 10.3 | Disable Autorun and Autoplay for Removable Media | 1 | ADOPTER-OWNS |
| 10.4 | Configure Automatic Anti-Malware Scanning of Removable Media | 2 | ADOPTER-OWNS |
| 10.5 | Enable Anti-Exploitation Features | 2 | MAPPED — container hardening + seccomp + Rust memory safety |
| 10.6 | Centrally Manage Anti-Malware Software | 2 | PARTIAL |
| 10.7 | Use Behavior-Based Anti-Malware Software | 3 | PARTIAL — Sentinel ML signals |

## Control 11 — Data Recovery

| SG | Title | IG | Status |
|----|-------|----|---------|
| 11.1 | Establish and Maintain a Data Recovery Process | 1 | GAP |
| 11.2 | Perform Automated Backups | 1 | PARTIAL — Wotan WAL-replication |
| 11.3 | Protect Recovery Data | 1 | PARTIAL |
| 11.4 | Establish and Maintain an Isolated Instance of Recovery Data | 1 | GAP |
| 11.5 | Test Data Recovery | 2 | GAP — no DR test |

## Control 12 — Network Infrastructure Management

| SG | Title | IG | Status |
|----|-------|----|---------|
| 12.1 | Ensure Network Infrastructure is Up-to-Date | 1 | UNVERIFIED |
| 12.2 | Establish and Maintain a Secure Network Architecture | 2 | MAPPED — 6-armor-layer architecture |
| 12.3 | Securely Manage Network Infrastructure | 2 | MAPPED — helm + Sealed Cask + ADR-052 drift policy |
| 12.4 | Establish and Maintain Architecture Diagrams | 2 | PARTIAL — CLAUDE.md diagrams + ADR + tonight's threat model |
| 12.5 | Centralize Network Authentication, Authorization, and Auditing | 2 | PARTIAL — pkg/auth + Wotan log aggregation |
| 12.6 | Use of Secure Network Management and Communication Protocols | 2 | MAPPED — TLS 1.3 + ML-DSA-65 + (planned) WireGuard. **BM3 scope qualifier:** ML-DSA-65 signing scope is `config.*` topics ONLY; drift.*, audit.*, telemetry.*, health.* topic families are unsigned and forgeable. |
| 12.7 | Ensure Remote Devices Utilize a VPN and are Connecting to an Enterprise's AAA Infrastructure | 3 | PARTIAL — (planned) WireGuard overlay |
| 12.8 | Establish and Maintain Dedicated Computing Resources for All Administrative Work | 3 | GAP |

## Control 13 — Network Monitoring and Defense

| SG | Title | IG | Status |
|----|-------|----|---------|
| 13.1 | Centralize Security Event Alerting | 1 | PARTIAL — Sentinel + Wotan log aggregation |
| 13.2 | Deploy a Host-Based Intrusion Detection Solution | 2 | PARTIAL — Mímir + heimdall-daemon |
| 13.3 | Deploy a Network Intrusion Detection Solution | 2 | PARTIAL — Suricata IDS. **SEN5 caveat:** Suricata is integration code (`pkg/anamnesis/`) — deployment to actual hosts (WEST, EAST) is UNVERIFIED. See sensor deployment audit (`docs/security/sensor-deployment-audit-2026-05-06.md`). |
| 13.4 | Perform Traffic Filtering Between Network Segments | 2 | PARTIAL — NetworkPolicy default-deny. **SEN8 caveat:** NetworkPolicy enforcement on default kindnet CNI is unproven (per K8s threat model §3.3); requires Calico/Cilium switch OR live smoke test before promoting to MAPPED. |
| 13.5 | Manage Access Control for Remote Assets | 2 | PARTIAL — TLS 1.3 + auth |
| 13.6 | Collect Network Traffic Flow Logs | 2 | MAPPED — eBPF flow-tracker |
| 13.7 | Deploy a Host-Based Intrusion Prevention Solution | 3 | PARTIAL — Mímir alerts-only (per ADR-043 hard condition #1) |
| 13.8 | Deploy a Network Intrusion Prevention Solution | 3 | PARTIAL — Suricata can be IPS-mode |
| 13.9 | Deploy Port-Level Access Control | 3 | PARTIAL — NetworkPolicy + 802.1X (adopter network) |
| 13.10 | Perform Application Layer Filtering | 3 | PARTIAL — cmd/shield WAF |
| 13.11 | Tune Security Event Alerting Thresholds | 3 | PARTIAL — Sentinel |

## Control 14 — Security Awareness and Skills Training

| SG | Title | IG | Status |
|----|-------|----|---------|
| 14.1 — 14.9 | (entire control) | 1-3 | GAP — single-operator |

## Control 15 — Service Provider Management

| SG | Title | IG | Status |
|----|-------|----|---------|
| 15.1 | Establish and Maintain an Inventory of Service Providers | 1 | PARTIAL — SBOM 553 deps |
| 15.2 | Establish and Maintain a Service Provider Management Policy | 1 | GAP |
| 15.3 | Classify Service Providers | 2 | GAP |
| 15.4 | Ensure Service Provider Contracts Include Security Requirements | 2 | GAP |
| 15.5 | Assess Service Providers | 3 | GAP |
| 15.6 | Monitor Service Providers | 3 | PARTIAL — Sentinel NVD/KEV consumption |
| 15.7 | Securely Decommission Service Providers | 3 | GAP |

## Control 16 — Application Software Security

| SG | Title | IG | Status |
|----|-------|----|---------|
| 16.1 | Establish and Maintain a Secure Application Development Process | 2 | MAPPED — TDD + ADR cadence + CI gates |
| 16.2 | Establish and Maintain a Process to Accept and Address Software Vulnerabilities | 2 | GAP — no VDP |
| 16.3 | Perform Root Cause Analysis on Security Vulnerabilities | 2 | PARTIAL — ADR cadence |
| 16.4 | Establish and Manage an Inventory of Third-Party Software Components | 2 | MAPPED — SBOM |
| 16.5 | Use Up-to-Date and Trusted Third-Party Software Components | 2 | PARTIAL |
| 16.6 | Establish and Maintain a Severity Rating System and Process for Application Vulnerabilities | 2 | GAP |
| 16.7 | Use Standard Hardening Configuration Templates for Application Infrastructure | 2 | MAPPED — helm + Sealed Cask + ADR-007 |
| 16.8 | Separate Production and Non-Production Systems | 2 | PARTIAL — values-{dev,prod}.yaml |
| 16.9 | Train Developers in Application Security Concepts and Secure Coding | 2 | GAP |
| 16.10 | Apply Secure Design Principles in Application Architectures | 2 | MAPPED — "Security First, Always" |
| 16.11 | Leverage Vetted Modules or Services for Application Security Components | 2 | MAPPED — pkg/auth + pkg/champion + cloudflare/circl |
| 16.12 | Implement Code-Level Security Checks | 3 | MAPPED — go vet + golangci-lint + cargo clippy + SAST in security.yml |
| 16.13 | Conduct Application Penetration Testing | 3 | PARTIAL — BlackMage daily red-team; gap is third-party formal pen-test. **SEN7 disclaimer:** INTERNAL validation only; NOT a substitute for the independent third-party assessment required by CIS Control 18. |
| 16.14 | Conduct Threat Modeling | 3 | PARTIAL — tonight's K8s threat model |

## Control 17 — Incident Response Management

| SG | Title | IG | Status |
|----|-------|----|---------|
| 17.1 | Designate Personnel to Manage Incident Handling | 1 | GAP |
| 17.2 | Establish and Maintain Contact Information for Reporting Security Incidents | 1 | GAP |
| 17.3 | Establish and Maintain an Enterprise Process for Reporting Incidents | 1 | GAP |
| 17.4 | Establish and Maintain an Incident Response Process | 2 | **GAP — universal cross-framework gap** |
| 17.5 | Assign Key Roles and Responsibilities | 2 | GAP |
| 17.6 | Define Mechanisms for Communicating During Incident Response | 2 | GAP |
| 17.7 | Conduct Routine Incident Response Exercises | 3 | GAP |
| 17.8 | Conduct Post-Incident Reviews | 3 | GAP |
| 17.9 | Establish and Maintain Security Incident Thresholds | 3 | GAP |

## Control 18 — Penetration Testing

| SG | Title | IG | Status |
|----|-------|----|---------|
| 18.1 | Establish and Maintain a Penetration Testing Program | 2 | PARTIAL — BlackMage daily red-team; ADR-062 fuzz/redteam. **SEN7 disclaimer:** INTERNAL validation only; NOT a substitute for the independent third-party assessment required by CIS Control 18. |
| 18.2 | Perform Periodic External Penetration Tests | 2 | GAP — no third-party of record |
| 18.3 | Remediate Penetration Test Findings | 2 | PARTIAL — Lich Hardening campaign |
| 18.4 | Validate Security Measures | 3 | PARTIAL |
| 18.5 | Perform Periodic Internal Penetration Tests | 3 | PARTIAL — BlackMage. **SEN7 disclaimer:** INTERNAL validation only; NOT a substitute for the independent third-party assessment required by CIS Control 18. |

---

## Headline gaps for IG2 readiness

1. **Control 17 (entire control)** — IR plan + reporting contact + thresholds. Single-document closes ~7 safeguards across IG1+IG2.
2. **3.4 + 3.5 + 8.10 retention/disposal** — single retention SOP closes 3 safeguards.
3. **5.4 / 5.5 / 6.8 RBAC tighten** — tonight's K8s RBAC review F1.
4. **3.11 at-rest encryption** — The Well + etcd.
5. **15.x Service Provider Management** — supplier inventory + policy.
6. **14.x Security Awareness Training** — single-operator-kingdom posture document.
7. **16.2 Vulnerability Disclosure Program** — security.txt + VDP closes RV.1.3 (SSDF) too.

## Strongest mapped controls

Control 2 (software inventory), Control 4 (secure config), Control 8 (audit logging), Control 12 (network infra), Control 13 (network monitoring), Control 16 (app sec) — these are where the kingdom genuinely earns IG2-level coverage today.

## Cross-reference

- **NIST CSF 2.0 (E9):** CIS publishes the CSF↔CIS crosswalk; Controls map 1:N to CSF subcategories.
- **NIST 800-53 Rev 5 (E5):** CIS publishes 800-53 mapping; Control 8 ↔ AU family etc.
- **ISO 27002:2022 (E11):** CIS publishes mapping; Control 4 ↔ A.8.9 + A.8.31 etc.
- **PCI DSS 4.0 (E3):** Control 11 ↔ PCI Req 9-10 etc.

## Verification

```bash
ls pkg/{auth,champion,discovery,transport,logagg,bpf,bpfmap}     # Controls 5, 6, 8, 12, 13
ls services/wotan/internal/signing/                                # Control 12.6
ls cmd/heimdall-daemon services/anamnesis pkg/anamnesis            # Control 13
bash scripts/verify-gpl-boundary.sh                                # Control 2.3
```

---

## Provenance

Read-only audit. Sources: CIS Controls v8 (May 2021) + v8.1 (June 2024); CLAUDE.md; tonight's K8s threat model + RBAC review + SBOM delta + CE1 patch.
