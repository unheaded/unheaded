# 2026-03-20 — Thursday

## Action Items

### Reach Out: Cavin Weber @ AMD — Funding Conversation
- **Context**: GPL-marked open source project. Not looking for a fortune — looking for rent, food, bills, expenses covered while building resume/clout with Unheaded.
- **Pitch angle**: Unheaded runs eBPF on XDP — AMD/Mellanox NICs are the target hardware. AMD benefits from an open source project that drives adoption of their NIC + GPU stack.
- **Ask**: Sponsorship / developer grant / stipend to sustain full-time open source development.
- **Prep**: Have the 6 Internet-Drafts, the GitHub repo (once public), and the IETF submission as credibility artifacts.
- **Note**: This is post-job-fair follow-up energy. Open source sponsorship, not venture capital — sustaining support for a GPL copyleft community project.

### Reach Out: Old Homie — Hook Them Up as Thanks
- **Context**: They were there for you. Time to return the favor now that the Kingdom is real and shipping.
- **Action**: Reach out, thank them, see how you can hook them up — role, recommendation, connection, recognition. Whatever fits.
- **Vibe**: Loyalty. The pack takes care of its own.

### Reach Out: Friend — Thank for Their Insights
- **Context**: Their insights shaped how this thing was built. That deserves acknowledgment.
- **Action**: Reach out, thank them specifically, let them know what shipped and how their input mattered.
- **Vibe**: Gratitude. Good ideas deserve credit.

### Reach Out: Cloudflare Contact — Open Source Sponsorship
- **Context**: Already using cloudflare/circl for all 3 production PQC algorithms. Not a cold pitch.
- **Pitch angle**: Building on their library, producing IETF Internet-Drafts that reference their work. Cloudflare cares about eBPF, PQC, QUIC/HTTP/3 — all core to Unheaded.
- **Ask**: Open source sponsorship / developer program / Workers/R2/D1 credits.
- **Note**: Worst they say is no. Best case, developer advocate routes to sponsorship program.

### Dev Machine Task: OSS License Code Audit
- **What**: Run comprehensive code audit on dev machine verifying all OSS dependencies listed in LICENSES/THIRD_PARTY.md are actually used, correctly attributed, and license-compliant.
- **How**: `go mod verify`, `cargo audit`, cross-reference SBOM against actual imports, check for any unlisted deps.
- **Why**: Pre-public due diligence. The SBOM exists but hasn't been verified against runtime imports.
- **Where**: Run on west (dev machine) — needs full build environment.

### Dev Machine Task: FN-DSA Upgrade (pornin/go-fn-dsa)
- **What**: Replace FN-DSA stub with real implementation using pornin/go-fn-dsa (pure Go, Unlicense, spec author).
- **Why**: Library exists NOW. No reason to ship a stub when the real thing is available.
- **License**: Unlicense (public domain) — GPL-3.0 compatible, assessed clear.

### Dev Machine Task: HQC Scaffold (liboqs-go)
- **What**: Scaffold HQC implementation using liboqs-go (MIT, CGo required).
- **Why**: Library exists NOW. Architecture decision: accept CGo dep or keep as stub.
- **License**: MIT — GPL-3.0 compatible, assessed clear. Static linking required.

### Dev Machine Task: Install unheaded-sentinel skill
- **What**: Copy skill from `.skills/skills/unheaded-sentinel/` to dev machine
- **Why**: Sentinel is now the primary blue team defender. Need to test triggers and operations.
- **How**:
  - Copy skill directory to local Claude skills path
  - Test skill invocation with sample prompts: "What's on my network?", "Is this device safe?", "Block this domain"
- **Configure daily adversarial loop**:
  - Set up cron job for 03:00 UTC (daily Sentinel → BlackMage adversarial loop)
  - Zhen AI scheduler integration for coordinated daily runs
- **Set up MCP connectors**:
  - NIST NVD API — CVE catalog and scoring
  - CISA KEV — Known Exploited Vulnerabilities feed
  - Vendor advisories API connectors for Go, Rust, Linux, NixOS
- **Test Pi-hole deployment** (if available on dev network):
  - Verify Docker compose setup with host networking
  - Test DNS query monitoring and blocking
  - Verify device discovery and threat detection

### Dev Machine Task: PQC Draft XML Conversion
- **What**: Convert `draft-bellis-unheaded-pqc-authentication-00.md` to XML using kramdown-rfc
- **Why**: PQC is the only draft without XML. Can't submit to IETF datatracker without it.
- **How**: `kramdown-rfc2629 draft-bellis-unheaded-pqc-authentication-00.md > draft-bellis-unheaded-pqc-authentication-00.xml`
- **Then**: Validate with `xml2rfc draft-bellis-unheaded-pqc-authentication-00.xml`
- **Note**: PQC spec has stale cross-references (-04/-00/-00 → -06/-03/-03) that must be fixed FIRST (S74 Phase 1, Step 1)

### Dev Machine Task: Push 8 Local Commits to Origin
- **What**: West dev machine is 8 commits ahead of origin/main
- **How**: `git push origin main`
- **Why**: Sync dev work (S73 P1-P5 + Three Crowns) to GitHub before public flip

## Session Notes
- Yesterday's Cowork session was MASSIVE — RFC fixes, 2 new drafts, ADR-69420, timeline sync, PQC audit, Amber lore expansion, IP audit completed
- Amber IP audit: **CLEAR TO SHIP** — zero Zelazny names in code/binaries/IETF drafts, fair use for lore docs
- IETF submission and GitHub flip are next — BOTH ARE BROWSER-ONLY TASKS
- VC language scrubbed from all public docs — this is The Free Kingdom, GPL copyleft
- PQC licensing assessed clear: go-fn-dsa (Unlicense), liboqs-go (MIT), circl (BSD-3)
