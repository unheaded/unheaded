# MITRE ATT&CK + D3FEND Coverage Overlay — Kingdom Defensive Controls

**Date:** 2026-05-06
**Author:** Marshal, NORTH-STAR Appendix A — Sentinel SEN1 remediation
**Subject:** the kingdom's defensive controls mapped against specific MITRE ATT&CK Enterprise techniques and D3FEND defensive techniques.
**Source citation:** scrutiny doc `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` finding **SEN1** ("zero MITRE ATT&CK / D3FEND / CIS-to-ATT&CK crosswalk anywhere in the matrix family").
**Honesty rules:**
- Where Sentinel's SEN5 ("Suricata deployment unverified") applies, status is downgraded to PARTIAL with caveat.
- Where Sentinel's SEN3 ("Anamnesis Lite ring buffer ~10s effective retention") applies, detective coverage is downgraded to PARTIAL.
- Where the K8s threat model §3.3 ("kindnet NetworkPolicy enforcement unproven") applies, NetworkPolicy citations are downgraded to PARTIAL.
- Where BlackMage BM3 ("ML-DSA-65 scoped to config.* only") applies, signing claims are scope-qualified.
- The Champion gate, Sealed Cask, Zhen AI, and macOS dev workstation are explicitly named where they touch a technique — including their own un-modeled gaps.

This overlay is a **first-pass detection-engineering map**, not a certification. It satisfies SEN1's structural critique (no ATT&CK overlay anywhere) without overclaiming. Each MAPPED entry should grow into a detection rule with last-fired-timestamp before it can be defended in audit.

---

## Status taxonomy (declared up front, per Scientist S1)

| Status | Meaning |
|--------|---------|
| **MAPPED** | Kingdom subsystem materially detects or mitigates the technique with an artifact (rule, policy, code path) that exists today and is operationally exercised. |
| **PARTIAL** | Kingdom subsystem covers a subset of the technique's sub-procedures, OR coverage exists in design/code but operational deployment / retention / scope is incomplete. |
| **GAP** | No kingdom subsystem materially defends against the technique today. Backlog item. |

**Coverage depth annotations:**
- *(detective)* — control fires an alert / log / signal
- *(preventive)* — control blocks the action before it succeeds
- *(reactive)* — control triggers after-the-fact remediation
- *(deployment-unverified)* — code exists; operational deployment per SEN5 not confirmed
- *(retention-undermined)* — detection exists but Anamnesis Lite ring buffer ~10s defeats forensics (SEN3)
- *(scope-qualified)* — control covers a subset of channels (e.g. ML-DSA-65 config.* only, not drift.*/audit.*/health.*)

---

## Summary table

| Tactic | Techniques scored | MAPPED | PARTIAL | GAP |
|--------|-------------------|--------|---------|-----|
| Initial Access | 4 | 0 | 4 | 0 |
| Execution | 3 | 0 | 2 | 1 |
| Persistence | 5 | 0 | 3 | 2 |
| Privilege Escalation | 2 | 0 | 1 | 1 |
| Defense Evasion | 3 | 0 | 1 | 2 |
| Credential Access | 4 | 0 | 2 | 2 |
| Discovery | 3 | 0 | 1 | 2 |
| Lateral Movement | 2 | 0 | 1 | 1 |
| Collection | 2 | 0 | 0 | 2 |
| Command and Control | 4 | 0 | 3 | 1 |
| Exfiltration | 2 | 0 | 1 | 1 |
| Impact | 4 | 0 | 1 | 3 |
| **Total** | **38** | **0** | **20** | **18** |

**Honest read:** zero techniques are full-depth MAPPED in operating reality. Twenty are PARTIAL (design exists but retention, deployment, scope, or operating cadence undermines the claim). Eighteen are open GAP. The kingdom's design surface is broad; its operating-reality detection floor is thin.

---

## Tactic 1: Initial Access

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1190** | Exploit Public-Facing Application | PARTIAL *(preventive + detective, deployment-unverified)* | HAProxy edge LB rate limiting + WAF (`docs/security/WAF_ARCHITECTURE.md`); Suricata IDS (`pkg/anamnesis/suricata.go`) — *deployment unverified per SEN5*; pkg/auth.Middleware on every service. NodePorts 30080/30081 are bare HTTP per K8s threat model §3.4. | D3-WAF (Web App Firewall), D3-IBC (Inbound Conn Filtering), D3-NTA (Network Traffic Analysis) | **TODO:** publish `pkg/anamnesis/rules/web-exploits.rules`. Currently not in repo. |
| **T1133** | External Remote Services | PARTIAL *(preventive)* | SSH-only access to WEST/EAST; firewall default-deny (`networking.firewall.allowedTCPPorts = [ ]`); IPv6 P2P link to EAST (192.168.13.x). MFA on operator's GitHub/SSH per BM5: **UNVERIFIED**. WireGuard overlay deferred. | D3-RSA (Restrict System Access), D3-MFA (Multi-Factor Authentication) | **GAP:** no detection rule for unauthorized SSH login attempts; auth.AuditLogger is local to services, not OS-level. |
| **T1195** | Supply Chain Compromise | PARTIAL *(preventive)* | Sealed Cask SHA-256 binding rune (`scripts/build-sealed-cask.sh`, `scripts/verify-binding-rune.sh`) — but BM7 notes Sealed Cask only builds on Linux and signing key location is undocumented. SBOM (553 deps audited, ScanCode + FOSSology). Wotan ML-DSA-65 *(scope-qualified: config.* only per BM3)*. NVD/KEV via MCP — *consume-only per SEN6*. | D3-SBV (Software Bill of Materials), D3-EAL (Executable Allowlisting) | **PARTIAL:** SBOM check in `.github/workflows/sbom-audit.yml` weekly. No KEV/CVE-to-deployed-version reconciliation rule. |
| **T1078** | Valid Accounts | PARTIAL *(preventive + detective)* | pkg/auth (APIKey/JWT authenticators); pkg/auth.RBACAuthorizer; pkg/auth.AuditLogger captures all auth events. **Gap:** no anomalous-login detection (impossible-travel, off-hours, etc.); no session-duration limits documented. | D3-AL (Account Locking), D3-ANAA (Authentication Event Thresholding) | Detection rule **needed:** `audit.auth.failed_login` rate-anomaly Sentinel rule. Not currently in `pkg/anamnesis/`. |

---

## Tactic 2: Execution

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1059** | Command and Scripting Interpreter | PARTIAL *(preventive)* | pkg/champion's Rule 1 (tool allowlist) gates scripting calls from Zhen AI; seccomp `@system-service ~@privileged ~@resources` on all NixOS containers. **BM2 gap:** Champion bypass via `direct-user` source-trust assertion — Zhen CLI tool-call injection feasible. **BM6 gap:** Zhen prompt-injection can construct tool calls. | D3-EAL (Executable Allowlisting), D3-PSA (Process Spawn Analysis) | **GAP:** no Sentinel rule for unexpected `bash`/`sh`/`python` exec under service container PIDs. eBPF execve probe **not deployed**. |
| **T1106** | Native API | GAP | NixOS `SystemCallFilter` denies privileged syscall classes per service; no detective coverage of in-process native API abuse. eBPF kprobe scaffolding exists in `crates/heimdall-bpf/` (Mímir's Law) but is alerts-only and host-baseline-scoped, not application-syscall-anomaly. | D3-SCA (System Call Analysis) | **GAP:** no detection. Add `crates/heimdall-bpf/syscall-anomaly` probe; not on roadmap. |
| **T1610** | Deploy Container | PARTIAL *(preventive)* | K8s threat model §3.5 names `unheaded-armory` ClusterRole as **HIGH severity** over-grant (`update`/`patch` on Deployments cluster-wide). Recommendation #1 is to scope it down. Until then, any compromised SA bound to it can deploy arbitrary containers. PodSecurityAdmission labels **missing** per K8s §3.1 / §5 rec #3. Sealed Cask digest pinning **not enforced** in chart per K8s §3.7 / §5 rec #6. | D3-EAL (Executable Allowlisting), D3-PA (Process Analysis) | **PARTIAL:** kubectl audit log is recommended in K8s threat model §5 rec #4 but **not yet wired**. No Sentinel detection for unexpected `kubectl create` / new Pod admission. |

---

## Tactic 3: Persistence

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1136** | Create Account | PARTIAL *(detective)* | pkg/auth.AuditLogger; ServiceAccount auto-mount enabled (K8s §3.5 medium severity); no detection rule for new user/SA creation today. | D3-UAP (User Account Permissions), D3-ANAA (Auth Event Thresholding) | **GAP detection rule:** Sentinel should alert on new `kind: ServiceAccount` admission events; **not implemented**. |
| **T1098** | Account Manipulation | PARTIAL *(detective)* | pkg/auth.RBACAuthorizer + AuditLogger captures role-binding mutations within service-level RBAC. **K8s ClusterRole/RoleBinding mutations:** apiserver audit log not configured per K8s §3.1 / §5 rec #4. | D3-UGLPA (User-Group/Local-Privilege Audit) | **PARTIAL:** in-service RBAC mutation logged; cluster-level RBAC mutation **not logged**. |
| **T1543** | Create or Modify System Process | PARTIAL *(preventive + detective)* | NixOS systemd units are declarative (`/nix/store` immutable); read-only `/etc` and `/usr` per CLAUDE.md container hardening; Mímir / heimdall-daemon detects baseline drift on host-level system unit changes (`pkg/enkrateia/`, `cmd/heimdall-daemon/`). Real-metal validated on EAST. | D3-SFA (System File Analysis), D3-FBA (File Behavior Analysis) | **PARTIAL:** Mímir alerts-only; per BM4, 7-hour median operator response window. |
| **T1525** | Implant Internal Image (container) | GAP | Sealed Cask provides SHA-256 binding rune for kingdom images. **K8s threat model §3.7:** chart does not enforce `imagePullPolicy: Never` or digest pinning; a compromised registry could inject an image; no admission controller (e.g. Kyverno/Cosign verification) deployed. | D3-EAL (Executable Allowlisting), D3-IVV (Input Validation) | **GAP:** no Cosign / sigstore verification on image pulls. Roadmap item. |
| **T1554** | Compromise Client Software Binary | GAP | Sealed Cask covers kingdom-built binaries. **No verification of upstream binaries** (Go toolchain, Aya runtime, llama.cpp, ROCm libs, Mistral/qwen-coder weights). Per BM7: signing key location undocumented. | D3-EBA (Executable File Analysis) | **GAP:** no detection. Reproducible-builds work deferred. |

---

## Tactic 4: Privilege Escalation

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1068** | Exploitation for Privilege Escalation | PARTIAL *(preventive)* | NixOS hardening: `NoNewPrivileges=true`, `CapabilityBoundingSet=[CAP_NET_BIND_SERVICE]`, seccomp `~@privileged ~@resources`. eBPF programs run with strict instruction-budget gate (`scripts/bpf-verifier-check.sh`). | D3-PCSV (Process Code Segment Verification), D3-PSA (Process Spawn Analysis) | **GAP detection:** no Sentinel rule for unexpected `setuid`/capability gain; eBPF kprobe needed. |
| **T1078.004** | Cloud Accounts | GAP | Kingdom is currently bare-metal + kind; no managed-cloud K8s. If/when ADR-040 K8s production lands, IAM/RBAC threat surface expands. Today: GitHub org `bellistech` + `unheaded` are the only cloud identities; MFA per BM5: **UNVERIFIED**. | D3-MFA, D3-CAM (Credential Account Management) | **GAP:** no monitoring of GitHub audit log; no IAM federation. |

---

## Tactic 5: Defense Evasion

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1562** | Impair Defenses | PARTIAL *(detective, retention-undermined)* | Mímir / heimdall-daemon detects host-baseline drift including service-state mutations *(alerts-only, BM4 7-hour response window)*. eBPF Anamnesis Lite firehose *(retention-undermined per SEN3)*. | D3-FBA (File Behavior Analysis), D3-SCA (System Call Analysis) | **PARTIAL:** Mímir scan + drift alert wired; Sentinel rule on `service_stopped` event family — **not yet authored**. |
| **T1070** | Indicator Removal (audit-log tampering) | GAP | pkg/auth.AuditLogger writes to local service log; no append-only / WORM substrate; no remote-shipping of audit events to a tamper-resistant store. Wotan `audit.*` topics **not signed** per BM3 — an attacker can inject fake audit events to mask real ones, OR delete/overwrite local logs since kingdom doesn't ship them off-host. K8s apiserver audit log **not configured** per K8s §3.1. | D3-FA (File Analysis), D3-FH (File Hashing) | **GAP:** no detection for log gaps, log truncation, or log forgery. **Highest-priority GAP** — see closing list. |
| **T1027** | Obfuscated Files | GAP | No kingdom-side obfuscation detection. Suricata could carry rules for known packers but *deployment unverified per SEN5*. | D3-FCA (File Content Analysis) | **GAP:** no detection rule. |

---

## Tactic 6: Credential Access

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1110** | Brute Force | PARTIAL *(preventive + detective)* | HAProxy edge rate limiting (per CLAUDE.md, with TrustedProxies guard on X-Forwarded-For); pkg/auth.AuditLogger captures failed auth events; pkg/champion logs all tool calls. **Gap:** no automatic account-lockout threshold; no rate-anomaly Sentinel rule on `audit.auth.failed_login`. | D3-AL (Account Locking), D3-ANAA (Auth Event Thresholding) | **PARTIAL:** rate limiting prevents commodity brute-force; no detection rule for slow-and-low credential stuffing. |
| **T1003** | OS Credential Dumping | GAP | NixOS `ProtectKernelTunables=true`, `PrivateDevices=true` reduce surface. **No detection** of `/proc/<pid>/mem` reads, LSASS-equivalent, or kernel-keyring dumping. eBPF Anamnesis Lite ring buffer too short for forensic value (SEN3). | D3-PFV (Process File Verification), D3-SCA (System Call Analysis) | **GAP:** no detection rule. |
| **T1552** | Unsecured Credentials | PARTIAL *(preventive)* | CLAUDE.md forbids secrets in code/env/logs; SOPS+age policy declared *(but K8s threat model §3.6 notes SOPS is not yet wired into helm chart)*; pre-commit hook + GHA secret-scan workflow. **Risks per BM5:** Stevie's macOS workstation holds signing keys, GitHub PAT, SSH agent — no enumerated control surface. | D3-CH (Credential Hardening), D3-SCH (Strong Cred Hashing) | **PARTIAL:** secret-scan in `.github/workflows/`. Not all secret classes (e.g. ML-DSA private keys) covered by gitleaks default rules. |
| **T1539** | Steal Web Session Cookie | GAP | TLS 1.3 minimum for external traffic per CLAUDE.md, but NodePorts 30080/30081 are bare HTTP per K8s §3.4. JWT tokens (pkg/auth/jwt.go) — secure-flag / HttpOnly / SameSite cookie posture **not documented** in matrix family. | D3-NTF (Network Traffic Filtering), D3-CH (Credential Hardening) | **GAP:** no detection rule for cookie-theft IoCs. |

---

## Tactic 7: Discovery

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1018** | Remote System Discovery | GAP | NetworkPolicy default-deny *(K8s threat model §3.3: kindnet enforcement unproven — PARTIAL claim)*. eBPF flow-tracker captures L4 connection attempts but ring buffer wraps in ~10s (SEN3). | D3-NTA (Network Traffic Analysis), D3-NI (Network Isolation) | **GAP:** no detection rule for ICMP/ARP scanning, port enumeration. |
| **T1046** | Network Service Discovery | PARTIAL *(detective, retention-undermined)* | eBPF flow-tracker (`crates/flow-tracker/`) captures connection metadata; Suricata can detect SYN-scan patterns *(deployment unverified per SEN5)*. NetworkPolicy default-deny limits scan reach *(kindnet PARTIAL)*. | D3-NTA, D3-NI | **PARTIAL:** Suricata `emerging-scan.rules` available upstream but kingdom rule deployment per-host **unverified**. |
| **T1083** | File and Directory Discovery | GAP | NixOS `ProtectSystem=strict`, `ProtectHome=true` prevent some host enumeration; container `ReadOnlyPaths` limits in-container surface. **No detection** of `find` / `ls -laR` / `cat /etc/passwd` patterns. | D3-FA (File Analysis), D3-SCA (System Call Analysis) | **GAP:** no detection rule. |

---

## Tactic 8: Lateral Movement

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1021** | Remote Services (SSH) | PARTIAL *(preventive)* | SSH key-based auth on WEST/EAST/P2P; `ssh govan@east` is documented operator path. NetworkPolicy default-deny for pod-to-pod SSH inside kind *(kindnet PARTIAL)*. **No host-level detection** of unexpected SSH session origins or pivoting through Stevie's bastion-equivalent macOS box. | D3-NI (Network Isolation), D3-RSA (Restrict System Access) | **GAP detection:** no Sentinel rule on `auth.ssh.session_open`. |
| **T1570** | Lateral Tool Transfer | GAP | NixOS `ProtectSystem=strict` + `ReadWritePaths` restrictions limit landing-zone for transferred binaries inside containers. **No detective coverage** of HTTP/SCP/rsync transfers between hosts; Suricata could cover *(deployment unverified per SEN5)*. | D3-NTA, D3-FA | **GAP:** no detection rule. |

---

## Tactic 9: Collection

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1005** | Data from Local System | GAP | Architectural-floor claim: kingdom doesn't process first-class user PII *(but Scientist S3 / BM6: Wotan logs, eBPF traces, Zhen AI sessions can carry secondary PII)*. **No DLP**, no file-access-anomaly detection. | D3-FA, D3-SCA | **GAP:** no detection rule. |
| **T1213** | Data from Information Repositories | GAP | Sophia knowledge graph / Zhen vor (1.52M vector corpus) / wiki — all internal repos. **No access logging** beyond pkg/auth's request-level audit. Vor read access is unauthenticated within-service per current pkg/champion design. | D3-DTAA (Data Transfer Anomaly Analysis) | **GAP:** no detection rule. **BM6:** Zhen AI prompt-injection can exfiltrate vor contents to chat output. |

---

## Tactic 10: Command and Control

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1071** | Application Layer Protocol | PARTIAL *(detective, deployment-unverified, retention-undermined)* | Sentinel skill explicitly mentions DGA + beaconing detection (per scrutiny SEN1); Suricata IDS rules cover this technique family *(deployment unverified per SEN5)*; eBPF flow-tracker captures L7 metadata *(retention-undermined per SEN3)*. | D3-NTA (Network Traffic Analysis), D3-DNSTA (DNS Traffic Analysis) | **PARTIAL:** Suricata rules `emerging-trojan.rules` would cover this; deployment per-host **unverified**. Add DGA detection Sentinel rule. |
| **T1105** | Ingress Tool Transfer | PARTIAL *(preventive + detective)* | NetworkPolicy egress restricted to intra-namespace + DNS only per K8s §3.3 *(kindnet enforcement PARTIAL)*; firewall default-deny on bare-metal hosts. eBPF flow-tracker for detective coverage *(retention-undermined)*. | D3-OTF (Outbound Traffic Filtering), D3-NTA | **PARTIAL:** egress-block prevents commodity tool fetches; sophisticated C2 over allowed channels not detected. |
| **T1573** | Encrypted Channel | PARTIAL *(detective)* | TLS 1.3 minimum for external; Wotan ML-DSA-65 signing on `config.*` topics *(scope-qualified per BM3)*. **TLS-fingerprint / JA3 / JA4 detection not deployed**. Suricata can carry such rules *(deployment unverified)*. | D3-NTA, D3-CTA (Certificate Analysis) | **PARTIAL:** no JA3/JA4 fingerprinting rule today. |
| **T1090** | Proxy | GAP | No detection of Tor / proxy chain / VPN egress from kingdom hosts. Public IP-reputation feeds (DShield, Spamhaus DROP) **not** integrated into Suricata *(deployment unverified)*. | D3-NTA, D3-OTF | **GAP:** no detection rule. |

---

## Tactic 11: Exfiltration

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1041** | Exfiltration Over C2 Channel | PARTIAL *(detective, retention-undermined)* | eBPF flow-tracker captures byte-counts per flow *(retention-undermined per SEN3)*; egress NetworkPolicy default-deny *(kindnet PARTIAL)*. **No volume-anomaly Sentinel rule** authored. | D3-DTAA (Data Transfer Anomaly Analysis) | **PARTIAL:** flow-tracker emits the data; rule to alert on unusual outbound volume — **not authored**. |
| **T1567** | Exfiltration Over Web Service | GAP | NetworkPolicy egress default-deny would block external SaaS API hits *(kindnet PARTIAL)*. **No DNS-tunnel detection**, no detection of exfil to legitimate cloud storage (Drive, Dropbox, S3, Pastebin) — these are common cover channels. | D3-NTA, D3-DNSTA | **GAP:** no detection rule. |

---

## Tactic 12: Impact

| Technique ID | Name | Status | Kingdom controls | D3FEND pair | Detection rule pointer |
|-------|------|--------|------------------|-------------|-------------------------|
| **T1486** | Data Encrypted for Impact (ransomware) | GAP | NixOS `ReadOnlyPaths`, `ProtectSystem=strict` reduce write surface. **No detection** of mass-write / mass-encrypt patterns. **No tested backups** — DR/backup posture is matrix-headlined GAP elsewhere. Recovery from real ransomware today: best-effort by single operator (per Sentinel SEN2 IR gap). | D3-FA (File Analysis), D3-FBA (File Behavior Analysis) | **GAP:** no detection rule, no backup-restore drill. |
| **T1485** | Data Destruction | GAP | Same control surface as T1486; no detection of bulk-delete or `dd if=/dev/zero` patterns. Mímir / heimdall could theoretically alert on filesystem baseline mass-mutation but real-metal validation was scoped to small drift, not mass-destruction. | D3-FA, D3-FBA | **GAP:** no detection rule. |
| **T1499** | Endpoint Denial of Service | PARTIAL *(preventive)* | HAProxy edge rate limiting (TrustedProxies guard on X-Forwarded-For); kube-apiserver `--max-mutating-requests-inflight` **not configured** per K8s §3.1 medium severity; no per-pod CPU/memory limits enumerated in current chart per K8s threat model. | D3-RAC (Resource Access Control), D3-IBC (Inbound Conn Filtering) | **PARTIAL:** edge rate limit exists; in-cluster DoS surface open. |
| **T1565** | Data Manipulation | GAP | Wotan `config.*` topic signing is the only integrity guarantee *(scope-qualified per BM3)*. Wotan `drift.*`, `audit.*`, `telemetry.*`, `health.*` are **unsigned** per BM3 — an attacker can inject fake events to corrupt operator decision-making OR forge the post-incident forensic record. | D3-MH (Message Hashing), D3-MA (Message Authentication) | **GAP:** signing scope expansion is on the heimdall-daemon TODO #2 (parked). **Highest-priority GAP** — see closing list. |

---

## Cross-cutting D3FEND coverage matrix

D3FEND defensive techniques the kingdom touches at all (irrespective of depth):

| D3FEND class | D3FEND technique | Kingdom subsystem | Depth |
|-------|--------|-------------------|-------|
| Harden | D3-EAL (Executable Allowlisting) | pkg/champion (Rule 1), Sealed Cask | PARTIAL |
| Harden | D3-CH (Credential Hardening) | pkg/auth (APIKey/JWT), SOPS/age (planned) | PARTIAL |
| Harden | D3-MFA | (none operationally) | GAP |
| Harden | D3-NI (Network Isolation) | NetworkPolicy + NixOS firewall | PARTIAL (kindnet enforcement) |
| Detect | D3-NTA (Network Traffic Analysis) | eBPF flow-tracker, Suricata | PARTIAL (retention + deployment) |
| Detect | D3-FA (File Analysis) | Mímir / heimdall-daemon, Sealed Cask binding rune | PARTIAL |
| Detect | D3-FBA (File Behavior Analysis) | Mímir baseline drift | PARTIAL |
| Detect | D3-SCA (System Call Analysis) | NixOS seccomp + crates/heimdall-bpf scaffold | PARTIAL |
| Detect | D3-PSA (Process Spawn Analysis) | (none — eBPF execve probe absent) | GAP |
| Detect | D3-ANAA (Auth Event Thresholding) | pkg/auth.AuditLogger (no rule yet) | PARTIAL |
| Detect | D3-DTAA (Data Transfer Anomaly Analysis) | eBPF flow-tracker (no rule yet) | PARTIAL |
| Detect | D3-DNSTA (DNS Traffic Analysis) | (none) | GAP |
| Detect | D3-CTA (Certificate Analysis) | (none — no JA3/JA4) | GAP |
| Isolate | D3-IBC (Inbound Conn Filtering) | HAProxy edge, firewall | PARTIAL |
| Isolate | D3-OTF (Outbound Traffic Filtering) | NetworkPolicy egress (kindnet PARTIAL) | PARTIAL |
| Deceive | D3-DA (Decoy Asset) | (none — no honeypot) | GAP |
| Evict | D3-AL (Account Locking) | (none — no auto-lockout) | GAP |
| Restore | D3-FRT (File Restoration) | (none — no tested backup) | GAP |

---

## Top GAP techniques the kingdom should prioritise closing

Scoring rubric (per Scientist S5 — declared up front to avoid cherry-picking):
**Priority = (frameworks_touched × adversary_frequency) ÷ effort_to_close**

1. **T1070 Indicator Removal — audit-log tampering.** Affects every detective control's evidentiary value; pairs with BlackMage BM3 (audit.* topics unsigned, locally-written, locally-deletable). **Closure path:** extend Wotan ML-DSA-65 signing to `audit.*` topic family (heimdall-daemon TODO #2 already parked); add WORM/append-only ship-off-host destination. Frameworks: NIST AU-9, PCI 10.3.2, SOC 2 CC7.2, ISO 27001 A.8.34. **Effort: medium (signing infra exists for config.*).** Highest leverage.
2. **T1565 Data Manipulation — forge non-config.* topics.** Pairs with #1 above and BM3 directly. Drift.*, health.*, telemetry.* topics are forgeable today; an attacker can manipulate operator decision-making (mask intrusion via fake drift; trigger auto-remediation via fake health failures). **Closure path:** same as #1 — universal Wotan topic-signing rollout. Frameworks: NIST SI-7, PCI 10.5, ISO 27001 A.8.13. **Effort: medium.** Same lever closes #1 + #2 simultaneously.
3. **T1059 Command and Scripting Interpreter — Champion bypass + Zhen prompt-injection.** Pairs with BM2 + BM6. Champion gate is the kingdom's single PEP for tool execution; `direct-user` source-trust label is asserted by the CLI without Champion verification. Combined with Zhen AI prompt-injection (vor sheet poisoning, system-prompt extraction), the kingdom's most-novel attack surface has zero matrix coverage. **Closure path:** (a) trust-boundary refactor in pkg/champion to verify `direct-user` against an out-of-band signal (e.g. signed pending_token from HCI surface, not from the LLM-controlled prompt path); (b) Zhen prompt-injection threat model + input sanitization on vor-retrieved content; (c) eBPF execve probe under service container PIDs to detect bypass attempts. Frameworks: NIST AC-3, AC-4, CM-7, SI-10, SOC 2 CC6.1. **Effort: medium-high but the highest-novelty surface.**

**Honourable mention (just below the top 3):**
- **T1078 Valid Accounts** with **MFA UNVERIFIED** (BM5) on Stevie's macOS workstation / GitHub. The keystone attack BlackMage names — phishing or session-theft on the operator turns into kingdom compromise. Closure: enforce hardware-key MFA (FIDO2) on GitHub, deploy-keys, and any cloud identity. Effort: low. Reason it ranks 4th not 1st: it's a single-session preventive control, not a detection backstop.

---

## Caveats & limits of this overlay

1. **First-pass authoring**, not falsification-tested. Per Scientist S2, every PARTIAL/MAPPED entry should have a one-line falsification test before it can be defended. **Backlog:** author falsification tests for 38 techniques.
2. **Detection rule pointers** are aspirational for ~30 of 38 techniques. Real Sentinel/Suricata `.rules` files are needed.
3. **Suricata deployment per-host enumeration** is the SEN5 dependency that gates ~10 techniques' status from PARTIAL to MAPPED.
4. **Anamnesis Lite retention floor (90 days online minimum per CIS 8.10 / NIST AU-11) is the SEN3 dependency** that gates ~12 techniques' detective-coverage claims.
5. **D3FEND mappings are class-level**, not procedure-level. Procedure-level granularity is a follow-on item.
6. **Threat coverage for Champion gate, Zhen AI, Sealed Cask trust chain, macOS dev workstation** — BlackMage's four named threat models — must land before this overlay can claim anything stronger than PARTIAL on the techniques touching those subsystems.
7. **Kingdom mode + Inverse Mask / IPv6 metric work** is not yet covered; the protocol-level threat model is a separate artifact.

---

## Provenance

- Scrutiny finding driving this: `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` §SEN1.
- K8s threat model (referenced for kindnet, ClusterRole, audit-log gaps): `docs/security/k8s-threat-model-2026-05-06.md`.
- Kingdom subsystem references: `CLAUDE.md`, `pkg/auth/`, `pkg/champion/`, `pkg/anamnesis/`.
- Honesty caveats traced to scrutiny findings SEN3 (retention), SEN5 (Suricata deployment), BM3 (signing scope), BM5 (macOS keystone), BM6 (Zhen AI surface), BM7 (Sealed Cask trust chain).
- No external CVE lookups, no Suricata-rule retrieval, no live-host enumeration. Doc-only authoring.

---

**Marshal disposition:** this overlay closes SEN1 *structurally* (the matrix family now has an ATT&CK / D3FEND pass). It does **not** close SEN1 *operationally* — every PARTIAL entry needs a real detection rule, a last-fired-timestamp, and a SOC review cadence before it's audit-defensible. The road is longer than the doc.
