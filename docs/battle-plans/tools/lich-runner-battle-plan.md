# S79 LICH RUNNER BATTLE PLAN — 17 Phases, 287 Steps

**Date**: 2026-04-30
**Sprint**: S79 — Extract and harden autonomous adversary tool for community self-hosting
**Prerequisite**: CLAUDE.md doctrine binding (c6108fb8) confirmed, Barrister skill available
**Target**: GPL-3.0 Lich Runner CLI shipped to GitHub org with Barrister ethics gate
**Estimated Duration**: 18-24 hours across 3-4 sessions
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## LEGEND

[B] = Bash command | [V] = Verification | [D] = Debug | [W] = Write file
[R] = Read file | [S] = Sudo | [P] = Parallelizable | [C] = Commit
[E] = Ethics gate | [F] = Federation | [STUCK] = Skipped | [BLOCKED] = Blocked

---

## PHASE 0: DOCTRINE VERIFICATION (Steps 1-18)

**Goal**: Confirm GPL-3.0 community-first binding
**Time**: 20 minutes

- [ ] **Step 1** [R]: Verify CLAUDE.md doctrine section present
- [ ] **Step 2** [V]: "WE DO NOT SELL. WE SHARE." must appear; if missing → STOP
- [ ] **Step 3** [W]: Create ETHICS_CHARTER.md establishing red-team tool governance
- [ ] **Step 4** [V]: ETHICS_CHARTER.md readable with ethics guardrails defined
- [ ] **Step 5** [B]: Check GPL-3.0 license in repo root
- [ ] **Step 6** [V]: LICENSE-GPL3.md or equivalent exists
- [ ] **Step 7** [W]: Create LICENSE-GPL3.md with GPL-3.0-or-later text
- [ ] **Step 8** [V]: All three doc files (CLAUDE.md, ETHICS_CHARTER.md, LICENSE-GPL3.md) present
- [ ] **Step 9** [C]: **COMMIT**: Doctrine binding + ethics charter + GPL-3.0 lock

---

## PHASE 1: BARRISTER ETHICS REVIEW GATE (Steps 10-45)

**Goal**: Obtain Barrister legal sign-off on autonomous adversary tool legality
**Time**: 60-90 minutes (HARD BLOCK on public release)
**Agent**: Coordinator + Barrister skill

- [ ] **Step 10** [E]: **BARRISTER ENGAGEMENT** — Request ethics review on autonomous red-team tool
- [ ] **Step 11** [R]: Read Barrister legal opinion memo
- [ ] **Step 12** [V]: **BARRISTER SIGN-OFF REQUIRED**: autonomous red-team is LEGAL for self-hosting + federation with consent; if NEGATIVE → ESCALATE
- [ ] **Step 13** [W]: Create LEGAL-OPINION-MEMO.md documenting Barrister findings
- [ ] **Step 14** [V]: LEGAL-OPINION-MEMO.md contains "CLEARED FOR RELEASE"
- [ ] **Step 15** [W]: Create RESPONSIBLE-DISCLOSURE.md (embargo periods, vendor contact)
- [ ] **Step 16** [C]: **COMMIT**: Barrister ethics review + legal opinion
- [ ] **Step 17** [V]: **PHASE 1 EXIT GATE**: Barrister sign-off obtained, conditions documented
- [ ] **Step 18** [E]: **IF GATE FAILS**: STOP, escalate to stevenrbellis@gmail.com

---

## PHASE 2: EXTRACT LICH ENGINE (Steps 19-62)

**Goal**: Set up cmd/tools/lich-runner/ with campaign loader, CISA KEV integration
**Time**: 45 minutes

- [ ] **Step 19** [B]: `mkdir -p /sessions/epic-magical-wozniak/mnt/unheaded/cmd/tools/lich-runner/{campaigns,patterns,config,tests}`
- [ ] **Step 20** [V]: All subdirs exist
- [ ] **Step 21** [B]: Copy tomb/lich campaigns: `find .../tomb/lich -name "LICH-*.md" | xargs cp -t .../campaigns/`
- [ ] **Step 22** [V]: Campaigns copied (count: `ls .../campaigns/ | wc -l`)
- [ ] **Step 23** [W]: Create main.go with CLI flags: --campaign, --dry-run, --i-consent, --peer-consent-token, --local-only, --config, --log-dir
- [ ] **Step 24** [V]: main.go syntactically valid: `go fmt main.go`
- [ ] **Step 25** [C]: **COMMIT**: lich-runner skeleton + main.go
- [ ] **Step 26** [W]: Create campaign.go (YAML parser struct for Campaign, Step types)
- [ ] **Step 27** [V]: campaign.go created with LoadCampaign() and Validate()
- [ ] **Step 28** [W]: Create example campaign LICH-000-template.yaml with TTPs, steps, timeout_ms
- [ ] **Step 29** [C]: **COMMIT**: Campaign YAML schema + template
- [ ] **Step 30** [W]: Create kev.go (CISA Known Exploited Vulnerabilities HTTP poller)
- [ ] **Step 31** [V]: kev.go created with FetchCISAKEV() and IsExploited()
- [ ] **Step 32** [C]: **COMMIT**: CISA KEV integration
- [ ] **Step 33** [W]: Create go.mod for lich-runner module
- [ ] **Step 34** [V]: go.mod present and valid
- [ ] **Step 35** [C]: **COMMIT**: lich-runner go.mod

---

## PHASE 3: SPDX + SBOM + GPL BOUNDARY (Steps 36-62)

**Goal**: Tag all Lich code with SPDX-License-Identifier headers, verify GPL-3.0 boundary
**Time**: 40 minutes

- [ ] **Step 36** [W]: Add SPDX header to main.go: `// SPDX-License-Identifier: GPL-3.0-or-later`
- [ ] **Step 37** [W]: Add SPDX headers to campaign.go, kev.go
- [ ] **Step 38** [W]: Add SPDX headers to all .go files
- [ ] **Step 39** [V]: Verify all .go files have SPDX: `find .../lich-runner -name "*.go" -exec grep -L "SPDX" {} \; | wc -l | grep "^0$"`
- [ ] **Step 40** [C]: **COMMIT**: SPDX headers on all Go files
- [ ] **Step 41** [B]: Check dependencies: `go mod graph | grep -i "MIT|Apache|BSD"`
- [ ] **Step 42** [V]: No copyleft-hostile deps (no Commons Clause, no proprietary); if found → remove or STOP
- [ ] **Step 43** [W]: Create THIRD-PARTY-LICH.md documenting all dependencies
- [ ] **Step 44** [V]: THIRD-PARTY-LICH.md created and GPL-compatible claimed
- [ ] **Step 45** [B]: Generate SBOM: `cyclonedx-gomod mod -output sbom.xml` (or deferred)
- [ ] **Step 46** [V]: SBOM file exists or deferred status logged
- [ ] **Step 47** [C]: **COMMIT**: GPL boundary verified + SBOM generated

---

## PHASE 4: LOCAL-ONLY MODE DEFAULT (Steps 48-74)

**Goal**: Implement target allowlist enforcement (localhost, RFC1918, explicit only)
**Time**: 50 minutes

- [ ] **Step 48** [W]: Create allowlist.go with Allowlist struct: LocalOnly bool, ExplicitList []string
- [ ] **Step 49** [W]: Implement IsAllowed(target) → checks loopback, RFC1918, explicit CIDR ranges
- [ ] **Step 50** [V]: allowlist.go created and logic correct
- [ ] **Step 51** [W]: Create allowlist_test.go with TestAllowlistLocalOnly, TestAllowlistWithRFC1918
- [ ] **Step 52** [V]: Tests compile: `go test ./... -v 2>&1 | head`
- [ ] **Step 53** [C]: **COMMIT**: Local-only mode + allowlist enforcement
- [ ] **Step 54** [W]: Update main.go to instantiate allowlist, validate campaign targets
- [ ] **Step 55** [W]: Add validateTargets(campaign, allowlist) error function to main.go
- [ ] **Step 56** [V]: main.go compiles: `go build -o /tmp/lich-runner 2>&1 | head`
- [ ] **Step 57** [C]: **COMMIT**: main.go integrated with allowlist
- [ ] **Step 58** [V]: **PHASE 4 EXIT GATE**: Allowlist tests pass, local-only enforced, binary compiles

---

## PHASE 5: FEDERATION MODE + SIGNED CONSENT (Steps 59-90)

**Goal**: Implement gungnir-sealed consent token validation for external campaigns
**Time**: 60 minutes

- [ ] **Step 59** [W]: Create federation.go with ConsentToken struct: TargetPeer, Campaign, Expiry, Signature, PolicyURL
- [ ] **Step 60** [W]: Implement ValidateConsentToken() → checks campaign match, expiry, signature, policy URL
- [ ] **Step 61** [V]: federation.go created with validation logic
- [ ] **Step 62** [W]: Create federation_test.go with TestValidateConsentToken_Expired, TestValidateConsentToken_InvalidCampaign
- [ ] **Step 63** [V]: Tests compile: `go test -v .../federation`
- [ ] **Step 64** [C]: **COMMIT**: Federation consent token schema + tests
- [ ] **Step 65** [W]: Update main.go to load consent token from --peer-consent-token flag
- [ ] **Step 66** [W]: Add federation gating: if --federation-mode unset AND campaign external → REJECT
- [ ] **Step 67** [V]: main.go compiles with federation logic
- [ ] **Step 68** [W]: Add --federation-mode flag to CLI (gates external campaigns)
- [ ] **Step 69** [C]: **COMMIT**: main.go integrated with federation mode
- [ ] **Step 70** [V]: **PHASE 5 EXIT GATE**: Federation tokens validate, external campaigns require token + --federation-mode

---

## PHASE 6: AUTH FRAMEWORK + AUDIT LOGGING (Steps 71-95)

**Goal**: Wire pkg/auth/ NoopAuthenticator, add audit logging to all campaign actions
**Time**: 45 minutes

- [ ] **Step 71** [W]: Create audit.go with AuditLogger: LogCampaignStart(), LogCampaignStep(), LogCampaignBlock(), LogCampaignComplete()
- [ ] **Step 72** [W]: Audit log format: JSON with timestamp, event type, campaign, target, status, reason, trace_id
- [ ] **Step 73** [V]: audit.go created with structured logging
- [ ] **Step 74** [W]: Update main.go to instantiate AuditLogger from pkg/logagg
- [ ] **Step 75** [W]: Call auditLogger.Log() on: campaign start, target validation, federation check, campaign complete
- [ ] **Step 76** [V]: main.go wired to pkg/auth/ and logagg
- [ ] **Step 77** [W]: Create audit_test.go with TestAuditLogFormatJSON
- [ ] **Step 78** [V]: Tests pass
- [ ] **Step 79** [C]: **COMMIT**: Auth framework + audit logging wired
- [ ] **Step 80** [V]: **PHASE 6 EXIT GATE**: All campaign actions logged, audit trail complete

---

## PHASE 7: SEALED-CASK REPRODUCIBLE BUILD (Steps 81-120)

**Goal**: Deterministic binary build using sealed-cask pattern (Docker + Nix + SHA256)
**Time**: 60 minutes

- [ ] **Step 81** [W]: Create Dockerfile for lich-runner (FROM golang:1.21-alpine, build flags, deterministic)
- [ ] **Step 82** [W]: Create nix/containers/lich-runner.nix (systemd service, seccomp, read-only FS)
- [ ] **Step 83** [V]: Both files created
- [ ] **Step 84** [B]: Build sealed-cask image: `scripts/build-sealed-cask.sh lich-runner`
- [ ] **Step 85** [V]: Binary produced with SHA256 recorded
- [ ] **Step 86** [W]: Create build-verification script to rebuild and verify SHA256 match
- [ ] **Step 87** [B]: `./build-verification.sh lich-runner` → SHA256 must match
- [ ] **Step 88** [V]: Reproducible build verified (SHAs match)
- [ ] **Step 89** [C]: **COMMIT**: Sealed-cask Dockerfile + Nix definition
- [ ] **Step 90** [V]: **PHASE 7 EXIT GATE**: Sealed-cask reproducible build works, deterministic binary confirmed

---

## PHASE 8: UPC SANDBOX HARDENING (Steps 91-120)

**Goal**: Lich runs in isolated UPC sandbox (seccomp, capabilities, FS isolation)
**Time**: 50 minutes

- [ ] **Step 91** [W]: Update nix/containers/lich-runner.nix with seccomp filter: block sys_mount, sys_ptrace, sys_admin
- [ ] **Step 92** [W]: Add CapabilityBoundingSet: only CAP_NET_BIND_SERVICE (for localhost probe)
- [ ] **Step 93** [W]: Add ProtectSystem: strict, ProtectHome: true, PrivateTmp: true, ReadOnlyPaths: [/etc, /usr]
- [ ] **Step 94** [V]: All hardening directives present in nix config
- [ ] **Step 95** [W]: Create sandbox-test.sh to verify seccomp rejection of forbidden syscalls
- [ ] **Step 96** [B]: `./sandbox-test.sh` → should show seccomp EACCES on forbidden calls
- [ ] **Step 97** [V]: Sandbox isolation confirmed
- [ ] **Step 98** [C]: **COMMIT**: UPC sandbox hardening complete

---

## PHASE 9: CAMPAIGN LIBRARY INFRASTRUCTURE (Steps 99-170)

**Goal**: Set up LICH-001 through LICH-N campaign runbooks for community contribution
**Time**: 45 minutes

- [ ] **Step 99** [W]: Create campaigns/README.md documenting campaign contribution process
- [ ] **Step 100** [W]: Create campaign-template.yaml as contributor scaffold (with all fields)
- [ ] **Step 101** [W]: Create CAMPAIGN_APPROVAL.md: Barrister review gate for novel TTPs
- [ ] **Step 102** [V]: Campaign infrastructure docs complete
- [ ] **Step 103** [W]: Create LICH-001.yaml (enumerate-network-interfaces, discovery)
- [ ] **Step 104** [W]: Create LICH-002.yaml (port-scan, service discovery)
- [ ] **Step 105** [W]: Create LICH-003.yaml (certificate-enumeration, TLS discovery)
- [ ] **Step 106** [W]: Create LICH-004.yaml (dns-resolution, hostname enumeration)
- [ ] **Step 107** [W]: Create LICH-005.yaml (vulnerability-scan, CVE check against CISA KEV)
- [ ] **Step 108** [V]: All campaigns 001-005 created with valid YAML structure
- [ ] **Step 109** [C]: **COMMIT**: Campaign library infrastructure + initial campaigns
- [ ] **Step 110** [V]: **PHASE 9 EXIT GATE**: Campaign template + approval process defined, LICH-001 through LICH-005 present

---

## PHASE 10: PER-CAMPAIGN SAFETY GATES (Steps 111-190)

**Goal**: Dry-run default, active mode requires explicit --i-consent + target confirmation
**Time**: 40 minutes

- [ ] **Step 111** [W]: Create safety.go with ExecutionMode enum: DryRun, Active
- [ ] **Step 112** [W]: Implement DryRun path: load campaign, print execution plan, exit (no network calls)
- [ ] **Step 113** [W]: Implement Active path: same as dry-run but ALSO executes campaign.Steps[].Action
- [ ] **Step 114** [V]: safety.go created with both paths
- [ ] **Step 115** [W]: Update main.go to:
  - Step 115a: Parse --dry-run (default: true), --i-consent
  - Step 115b: If --dry-run=true → always DryRun, ignore --i-consent
  - Step 115c: If --dry-run=false AND --i-consent=false → REJECT ("active mode requires --i-consent")
  - Step 115d: If --dry-run=false AND --i-consent=true → Active
- [ ] **Step 116** [V]: main.go logic verified
- [ ] **Step 117** [W]: Create safety_test.go with TestDryRunMode, TestActiveModeForbiddenWithoutConsent
- [ ] **Step 118** [V]: Tests pass
- [ ] **Step 119** [C]: **COMMIT**: Safety gates + execution modes
- [ ] **Step 120** [V]: **PHASE 10 EXIT GATE**: Dry-run default enforced, --i-consent required for active, tests pass

---

## PHASE 11: 72H LICH-AGAINST-LICH SELF-FUZZ (Steps 121-225)

**Goal**: Recursive red-team validation: Lich runs against itself for 72 hours
**Time**: 120 minutes (setup) + 72 hours (execution)
**Agent**: Coordinator

- [ ] **Step 121** [W]: Create fuzz-harness.go with Lich campaign fuzzer
- [ ] **Step 122** [W]: Fuzz targets: campaign parser (YAML malformed), allowlist validator (edge IPs), federation token validator (expired, invalid sig)
- [ ] **Step 123** [B]: `cargo fuzz run lich_fuzz -- -max_total_time=259200` (72 hours = 259200 sec)
- [ ] **Step 124** [V]: Fuzzer running, crashes logged to crash-artifacts/
- [ ] **Step 125** [W]: Create fuzz-triage.sh to categorize crashes (invalid YAML, segfault, allowlist bypass, token bypass)
- [ ] **Step 126-175**: [For each crash]: Triage, create test case, fix root cause, re-run until zero crashes
- [ ] **Step 176** [V]: Fuzz run completed, zero crashes remaining
- [ ] **Step 177** [W]: Create fuzz-report.md documenting findings, bugs fixed, test cases added
- [ ] **Step 178** [C]: **COMMIT**: Fuzz harness + crash analysis + fixes
- [ ] **Step 225** [V]: **PHASE 11 EXIT GATE**: 72h fuzz complete, zero crashes, all bugs resolved

---

## PHASE 12: COMPLIANCE EVIDENCE PACK (Steps 226-245)

**Goal**: MITRE ATT&CK mapping, IOC/TTP catalog, regulatory documentation
**Time**: 40 minutes

- [ ] **Step 226** [W]: Create compliance/ATT&CK-MAPPING.md mapping all LICH-XXX campaigns to MITRE TTPs
- [ ] **Step 227** [W]: Create compliance/IOC-CATALOG.md documenting indicators of compromise (network patterns, file paths)
- [ ] **Step 228** [W]: Create compliance/RESPONSIBLE-OPERATORS.md (legal guardrails, jurisdiction warnings)
- [ ] **Step 229** [V]: All three compliance docs present
- [ ] **Step 230** [W]: Create compliance/AUDIT-CHECKLIST.md (checklist for auditing own infra with Lich)
- [ ] **Step 231** [C]: **COMMIT**: Compliance evidence pack + ATT&CK mapping
- [ ] **Step 245** [V]: **PHASE 12 EXIT GATE**: Compliance docs complete, IOC catalog ready for SOC2/audits

---

## PHASE 13: PUBLIC DOCUMENTATION (Steps 246-265)

**Goal**: README, CONTRIBUTING, LICENSE, DCO, ethics charter (polished for public)
**Time**: 40 minutes

- [ ] **Step 246** [W]: Create README.md (50-word intro, installation, quick-start, examples)
- [ ] **Step 247** [W]: Create CONTRIBUTING.md (how to submit campaigns, Barrister review process, test-first)
- [ ] **Step 248** [W]: Create DEVELOPERS.md (project structure, phases, agent matrix, development setup)
- [ ] **Step 249** [W]: Update LICENSE-GPL3.md with full GPL-3.0 text + attribution
- [ ] **Step 250** [W]: Create DCO.txt (Developer Certificate of Origin for signed commits)
- [ ] **Step 251** [W]: Create docs/ETHICS.md (expanded ethics charter for public)
- [ ] **Step 252** [W]: Create docs/FAQ.md (is Lich legal? can I use it on others' systems? what's dry-run?)
- [ ] **Step 253** [V]: All docs created and spell-checked
- [ ] **Step 254** [C]: **COMMIT**: Public documentation complete
- [ ] **Step 265** [V]: **PHASE 13 EXIT GATE**: All docs present, public-ready, no jargon without explanation

---

## PHASE 14: FEDERATION ONBOARDING (Steps 266-275)

**Goal**: Peer consent policy template + Barrister-reviewed agreement
**Time**: 25 minutes

- [ ] **Step 266** [W]: Create federation/CONSENT-POLICY-TEMPLATE.md (what campaigns peer consents to, embargo periods)
- [ ] **Step 267** [W]: Create federation/PEER-AGREEMENT-TEMPLATE.md (contract template for federation partnerships)
- [ ] **Step 268** [E]: **BARRISTER REVIEW** — Request Barrister sign-off on agreement template
- [ ] **Step 269** [V]: Barrister returns "template is legally sound" memo
- [ ] **Step 270** [W]: Create federation/BARRISTER-APPROVAL.md documenting Barrister sign-off
- [ ] **Step 271** [W]: Create federation/ONBOARDING-CHECKLIST.md (steps to onboard a new peer)
- [ ] **Step 272** [C]: **COMMIT**: Federation onboarding templates + Barrister approval
- [ ] **Step 275** [V]: **PHASE 14 EXIT GATE**: Federation templates approved, onboarding documented

---

## PHASE 15: DEMO + INTEGRATION (Steps 276-285)

**Goal**: End-to-end demo: Lich discovers Mímir, finds drift, hands off to MoatGhost
**Time**: 45 minutes

- [ ] **Step 276** [B]: Start test Mímir instance (mock infrastructure)
- [ ] **Step 277** [B]: Run: `lich-runner campaign LICH-001 --dry-run` (discover Mímir services)
- [ ] **Step 278** [V]: Dry-run output shows discovered Mímir ports + versions
- [ ] **Step 279** [B]: Run: `lich-runner campaign LICH-005 --dry-run` (CVE check against CISA KEV)
- [ ] **Step 280** [V]: Finds a known CVE (injected for demo), reports it
- [ ] **Step 281** [B]: Generate report: `lich-runner report LICH-005 --output /tmp/report.json`
- [ ] **Step 282** [B]: Hand off to MoatGhost: `moatghost compliance-check --lich-report /tmp/report.json`
- [ ] **Step 283** [V]: MoatGhost consumes Lich output and generates compliance assessment
- [ ] **Step 284** [C]: **COMMIT**: Demo integration complete
- [ ] **Step 285** [V]: **PHASE 15 EXIT GATE**: End-to-end Lich → Mímir → MoatGhost flow works

---

## PHASE 16-17: PUBLIC RELEASE (Steps 286-287)

**Goal**: Public GitHub release under GPL-3.0 (post-Phase 1 ethics gate)
**Time**: 15 minutes

- [ ] **Step 286** [B]: Create GitHub repo: `github.com/unheaded-community/lich-runner`
- [ ] **Step 287** [B]: Push all code: `git push origin main`

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1: Stuck on Consent Token Validation**
- Check token contents: `jq . /path/to/token.json`
- Verify gungnir binary: `which gungnir-verify`
- Test on known-good signature
- If gungnir unavailable → skip federation tests [STUCK]

**A2: Allowlist Blocks Intended Target**
- Run with --dry-run to see allowlist enforcement
- Add RFC1918 range to config: `explicit_ranges: [192.168.1.0/24]`
- For external → requires --federation-mode + consent token

**A3: Binary Fails After Phase 6**
- Check Go version: `go version` (requires 1.21+)
- Clean cache: `go clean -modcache && go mod tidy`
- Rebuild: `go build -v ./...`

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Time | Sequential | Critical Path |
|-------|------|-----------|----------------|
| 0: Doctrine | 20m | Y | YES |
| 1: Barrister Gate | 90m | Y | **BLOCKING** |
| 2: Extract Engine | 45m | Y | YES |
| 3: SPDX+SBOM | 40m | Y | YES |
| 4: Local-Only | 50m | Y | YES |
| 5: Federation | 60m | Y | YES |
| 6: Auth+Audit | 45m | Y | YES |
| 7: Sealed-Cask | 60m | Y | YES |
| 8: UPC Sandbox | 50m | Y | NO |
| 9: Campaign Lib | 45m | N | NO |
| 10: Safety Gates | 40m | Y | YES |
| 11: Lich-vs-Lich Fuzz | 120m+72h | N | NO |
| 12: Compliance | 40m | Y | NO |
| 13: Public Docs | 40m | Y | NO |
| 14: Federation Onboard | 25m | Y | YES |
| 15: Demo | 45m | N | NO |
| 16-17: Release | 15m | Y | YES |

**Critical Path**: 0→1(GATE)→2→3→4→5→6→7→10→14→16 = ~6-7 hours minimum

---

**FREE TO USE. FREE TO SHARE. NO SELLING.**

Binding doctrine: c6108fb8 in CLAUDE.md, 2026-04-30
