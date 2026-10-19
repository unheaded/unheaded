# Threat Register — Unheaded Kingdom

**Last refreshed**: 2026-10-19 (kev-poller, ADR-055; previous: 2026-04-27)
**Owners**: MoatGhost (compliance + threat intel) + Sentinel (blue team monitoring)
**Cadence**: refreshed every Round Table sprint or weekly, whichever first
**Source feeds**: CISA KEV (primary), NIST NVD (secondary), GitHub Security Advisories (tertiary), supply-chain advisories per language ecosystem
**Snapshot of source feed**: catalog 2026.10.16, 1748 entries, sha256 `b6e27975f6635ecfc546fbd4fd6e444ba63a07ab2abc8876229d77fe083d357d` (kev-poller keeps snapshots on the host, `/var/lib/unheaded/threat-feeds/cisa-kev/`); previous: `docs/security/feeds/cisa-kev-2026-04-27.json` (1583 entries)

---

## Refresh log

### 2026-10-19: first kev-poller refresh (ADR-055)

Pulled with `kev-poller --once`, rendered with
`kev-poller report --since 2026-04-25 --stack config/kev-poller/stack.yaml`.
The register was 175 days old (ADR-052 allows 7). The report:

**CISA KEV catalog** 2026.10.16: 1748 entries, dateAdded 2021-11-03 to 2026-10-16, pulled 2026-10-19 04:42Z (sha256 b6e27975f663).

- Added since 2026-04-25: **165**
- In the Kingdom's stack (`config/kev-poller/stack.yaml`): **7** of them; 52 in the whole catalog

**Stack entries added since 2026-04-25**:

| Added | CVE | Vendor / product | Name | Category | CISA due |
|---|---|---|---|---|---|
| 2026-09-18 | CVE-2025-39682 | Linux / Kernel | Linux Kernel Improper Check for Unusual or Exceptional Conditions Vulnerability | linux-kernel | 2026-09-21 |
| 2026-09-18 | CVE-2025-39964 | Linux / Kernel | Linux Kernel Race Condition Vulnerability | linux-kernel | 2026-09-21 |
| 2026-09-18 | CVE-2026-53266 | Linux / Kernel | Linux Kernel Out-of-Bounds Write Vulnerability | linux-kernel | 2026-09-21 |
| 2026-08-27 | CVE-2026-53362 | Linux / Kernel | Linux Kernel Unspecified Vulnerability | linux-kernel | 2026-08-30 |
| 2026-08-26 | CVE-2022-0995 | Linux / Kernel | Linux Kernel Out-of-Bounds Write Vulnerability | linux-kernel | 2026-08-29 |
| 2026-06-02 | CVE-2022-0492 | Linux / Kernel | Linux Kernel Improper Authentication Vulnerability | linux-kernel | 2026-06-05 |
| 2026-05-01 | CVE-2026-31431 | Linux / Kernel | Linux Kernel Incorrect Resource Transfer Between Spheres Vulnerability | linux-kernel | 2026-05-15 |

Patched on a host? The catalog cannot say. On each Ubuntu host: `pro fix <CVE> --dry-run`:

    pro fix CVE-2025-39682 --dry-run
    pro fix CVE-2025-39964 --dry-run
    pro fix CVE-2026-53266 --dry-run
    pro fix CVE-2026-53362 --dry-run
    pro fix CVE-2022-0995 --dry-run
    pro fix CVE-2022-0492 --dry-run
    pro fix CVE-2026-31431 --dry-run

**Other entries added since 2026-04-25** (informational, newest first, 15 of 158 shown):

| Added | CVE | Vendor / product | Name |
|---|---|---|---|
| 2026-10-16 | CVE-2025-61624 | Fortinet / Multiple Products | Fortinet Multiple Products Path Traversal Vulnerability |
| 2026-10-16 | CVE-2026-102764 | BeyondTrust / Remote Support | BeyondTrust Remote Support Authentication Bypass by Spoofing Vulnerability |
| 2026-10-15 | CVE-2026-21589 | Atlassian / Multiple Products | Atlassian Multiple Products Files or Directories Accessible to External Parties Vulnerability |
| 2026-10-14 | CVE-2026-0322 | Palo Alto Networks / PAN-OS | Palo Alto Networks PAN-OS Code Injection Vulnerability |
| 2026-10-14 | CVE-2026-105133 | Ahsay / AhsayCBS | Ahsay AhsayCBS Improper Authentication Vulnerability |
| 2026-10-14 | CVE-2026-105134 | Ahsay / AhsayCBS | Ahsay AhsayCBS Command Injection Vulnerability |
| 2026-10-14 | CVE-2026-76519 | Cisco / Secure Email Gateway | Cisco Secure Email Gateway Relative Path Traversal Vulnerability |
| 2026-10-13 | CVE-2026-104935 | Ivanti / Connect Secure | Ivanti Connect Secure Authentication Bypass by Spoofing Vulnerability |
| 2026-10-13 | CVE-2026-96951 | Microsoft / Windows | Microsoft Windows Command Injection Vulnerability |
| 2026-10-08 | CVE-2015-3306 | ProFTPD / ProFTPD | ProFTPD Improper Access Control Vulnerability |
| 2026-10-08 | CVE-2015-5477 | ISC / BIND | ISC BIND Data Processing Errors Vulnerability |
| 2026-10-08 | CVE-2016-3081 | Apache / Struts | Apache Struts Command Injection Vulnerability |
| 2026-10-08 | CVE-2021-3199 | ONLYOFFICE / Docs | ONLYOFFICE Docs Server Path Traversal Vulnerability |
| 2026-10-08 | CVE-2023-22894 | Strapi / Strapi | Strapi Cleartext Storage of Sensitive Information Vulnerability |
| 2026-10-04 | CVE-2026-88779 | Citrix / NetScaler | Citrix NetScaler Improper Restriction of Operations within the Bounds of a Memory Buffer Vulnerability |

**Checked on the hosts** (`pro fix <CVE> --dry-run`, both on 6.17.0-41-generic, 2026-10-19):

| CVE | west | east |
|---|---|---|
| CVE-2026-31431 | resolved | resolved |
| CVE-2022-0492, CVE-2022-0995, CVE-2025-39964, CVE-2025-39682 | not affected | not affected |
| **CVE-2026-53266** (ebtables SNAT OOB write) | **not resolved, no fix from Ubuntu** | **not resolved** |
| **CVE-2026-53362** (privilege escalation via IPv6) | **not resolved, no fix from Ubuntu** | **not resolved** |

Exposure: both need code already running on the host. West is wider: LXD
sets `kernel.apparmor_restrict_unprivileged_userns=0` at runtime
(`/run/sysctl.d/zz-lxd.conf`), so any local account can reach netfilter and
IPv6 code with CAP_NET_ADMIN inside a user namespace; east keeps Ubuntu's
default (1). No ebtables module is loaded on either host.

**Verdict**: two exploited kernel vulnerabilities open on both hosts with no
vendor fix. Tracked as **FND-011** (high) in `compliance/findings/register.yaml`.

**Action items**:
- [ ] Fixed kernel on west and east as soon as Ubuntu ships it, then reboot; re-run the two `pro fix` checks (FND-011)
- [ ] Stopgap for CVE-2026-53266: block `ebtables`, `ebtable_nat`, `ebt_snat` in `/etc/modprobe.d/unheaded-cis.conf` (unused on both hosts; host change, Stevie)
- [ ] Decide whether west keeps unprivileged user namespaces open for LXD (test LXD with the restriction on first)
- [ ] Install and enable `unheaded-kev-poller.service` so this refresh stops depending on a session (ADR-055)

---

### 2026-04-27 — Round Table sprint

**Status**: ✅ COMPLETE for KEV + SPDX (manual-upload assist on KEV); SBOM + license scan + go-licenses still gated to remote box.

**CISA KEV summary**:
- Total entries in KEV: **1,583**
- Date range: 2021-11-03 → 2026-04-24
- Recent (last 14 days, since 2026-04-13): **24**
- **Relevant to Unheaded stack (Linux/kernel/eBPF/HIP/ROCm/llama/AMD/GPU/nginx/haproxy/openssl/curl/containerd/LXD/Docker/nixos): 0** ← clean week
- Snapshot saved: `docs/security/feeds/cisa-kev-2026-04-27.json`

**Recent KEV entries (informational, none touch our stack surface)**:

| Date | CVE | Product | Class |
|------|-----|---------|-------|
| 2026-04-24 | CVE-2024-57726 | SimpleHelp | Missing Authorization |
| 2026-04-24 | CVE-2024-57728 | SimpleHelp | Path Traversal |
| 2026-04-24 | CVE-2024-7399 | Samsung MagicINFO 9 | Path Traversal |
| 2026-04-24 | CVE-2025-29635 | D-Link DIR-823X | Command Injection |
| 2026-04-23 | CVE-2026-39987 | Marimo | RCE |
| 2026-04-22 | CVE-2026-33825 | Microsoft Defender | Access Control |
| 2026-04-20 | CVE-2024-27199 | JetBrains TeamCity | Path Traversal |
| 2026-04-20 | CVE-2025-32975 | Quest KACE SMA | Improper Auth |
| 2026-04-20 | CVE-2026-20128 | Cisco Catalyst SD-WAN | Stored Recoverable Passwd |
| 2026-04-20 | CVE-2025-48700 | Synacor Zimbra | XSS |
| 2026-04-20 | CVE-2023-27351 | PaperCut NG/MF | Improper Auth |
| 2026-04-20 | CVE-2025-2749 | Kentico Xperience | Path Traversal |
| 2026-04-20 | CVE-2026-20133 | Cisco Catalyst SD-WAN | Info Disclosure |
| 2026-04-20 | CVE-2026-20122 | Cisco Catalyst SD-WAN | Privileged API Misuse |
| 2026-04-16 | CVE-2026-34197 | Apache ActiveMQ | Improper Input Validation |
| *(9 more)* | — | — | enterprise SaaS / network appliances |

**Verdict for this sprint**: ✅ **No new threats requiring patch action against the Unheaded stack** as of 2026-04-27 KEV snapshot. Continue normal posture.

**What was checked**:
- ✅ SPDX coverage on Go files (1,162 total, 6 missing — backfill scheduled, see Section D of remote packet)
- ✅ SPDX coverage on Rust files (208 total, 156 missing — separate sprint required, see `docs/security/spdx-coverage-2026-04-27.md`)
- ✅ CISA KEV feed ingested (Stevie upload assist; sandbox proxy 403'd direct curl)
- ❌ NIST NVD feed (not attempted; KEV is the high-signal feed and was clean — NVD deferred)
- ❌ Syft SBOM regen (toolchain not in sandbox — see remote packet Section A)
- ❌ cargo-deny advisories (toolchain not in sandbox — see remote packet Section B)
- ❌ go-licenses report (toolchain not in sandbox — see remote packet Section B)

**Action items**:
- [ ] Run `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md` Sections A + B on next Linux session
- [ ] Backfill 6 Go SPDX headers (small, mechanical — Section D of remote packet)
- [ ] Schedule Rust SPDX backfill sprint (156 files — see spdx-coverage-2026-04-27.md)
- [ ] (No KEV-derived patch actions this sprint — clean.)

---

## Active threat categories (always tracked)

### Category 1 — Linux kernel + eBPF
**Why**: Unheaded runs eBPF programs at XDP/TC layers; kernel CVEs directly affect production.
**Feeds**: CISA KEV (Linux/kernel filter), NIST NVD (CPE: cpe:/o:linux:linux_kernel)
**Last entry**: 2026-10-19: 7 kernel entries added to KEV since April; CVE-2026-53266 and CVE-2026-53362 unpatched on west and east (FND-011)
**Notes**: WEST runs kernel 6.17.0-19; EAST runs comparable. Track 6.x advisories specifically.

### Category 2 — AMD GPU / HIP / ROCm
**Why**: WAVE10F → WAVE13 forge research depends on HIP/ROCm 6.2 stack on RX 7700 (Stevie's local) + WEST/EAST GPU.
**Feeds**: AMD Security Bulletins, CISA KEV (AMD/GPU filter)
**Last entry**: PENDING
**Notes**: New attack surface introduced in WAVE11/12. Monitor closely as forge moves toward serving (WAVE13/14).

### Category 3 — Supply chain (Go + Rust + Python)
**Why**: 553 deps audited per CLAUDE.md; new deps enter with each forge wave.
**Feeds**:
- Go: govulncheck, GitHub Advisory DB
- Rust: cargo-deny, RustSec advisory DB
- Python: pip-audit, PyPI Security Advisories
**Last entry**: PENDING (toolchains not in Cowork sandbox)
**Notes**: govulncheck + gosec already run in Jenkins per existing Jenkinsfile stages.

### Category 4 — Container runtimes
**Why**: LXD + containerd + Docker + NixOS interop; runtime escape CVEs are existential.
**Feeds**: CISA KEV (container filter), Snyk container DB
**Last entry**: PENDING

### Category 5 — Common edge components
**Why**: HAProxy, nginx, gateway TLS termination — directly internet-facing if Track B/C.
**Feeds**: CISA KEV (nginx, haproxy, openssl filters)
**Last entry**: PENDING

### Category 6 — llama.cpp / model-serving
**Why**: zhen-inference uses llama.cpp + Mistral-7B; future Gemma-4 + LoRA via forge.
**Feeds**: ggerganov/llama.cpp GitHub Security Advisories, HuggingFace model security
**Last entry**: PENDING

### Category 7 — IPv6 + extension headers + protocol stack
**Why**: Unheaded protocol uses IPv6 hop-by-hop headers; misuse of HbH/Routing/Fragmentation is a long-tail attack class.
**Feeds**: RFC errata (RFC 8200, 6437), academic papers, IETF security drafts
**Last entry**: PENDING (manual review required)

---

## Historical entries

*Pre-2026-04-27 entries to be backfilled from prior sessions if/when found in scratch notes. The 2026-04-27 partial refresh is the first formal entry in this register file.*

---

## Override / acknowledged risks

(Items where MoatGhost has explicitly accepted residual risk with rationale. Empty as of 2026-04-27.)

---

## Cross-references

- ADR-052 (drift policy) — CI guard ensures this register's "Last refreshed" stays fresh once active sprints touch it.
- ADR-053 (Hybrid Claude+Zhenai routing) — when active, "minor churn" routing classifier should NOT route threat-register updates to local Zhenai (HEAVY task; Claude or human only).
- LICH-012 campaign (Mímir's Law red team) — closed 2026-04-11, real-metal validated.
- `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md` — packaged commands for the parts that need toolchains.

---

*Threat register seeded 2026-04-27 from Cowork-on-Macbook. Live data refresh deferred to remote session per packet.*
