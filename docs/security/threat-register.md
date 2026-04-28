# Threat Register — Unheaded Kingdom

**Last refreshed**: 2026-04-27 (Cowork-on-Macbook + Stevie-uploaded KEV feed)
**Owners**: MoatGhost (compliance + threat intel) + Sentinel (blue team monitoring)
**Cadence**: refreshed every Round Table sprint or weekly, whichever first
**Source feeds**: CISA KEV (primary), NIST NVD (secondary), GitHub Security Advisories (tertiary), supply-chain advisories per language ecosystem
**Snapshot of source feed**: `docs/security/feeds/cisa-kev-2026-04-27.json` (1583 entries, KEV upstream as of upload)

---

## Refresh log

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
**Last entry**: PENDING (CISA KEV unreachable 2026-04-27)
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
