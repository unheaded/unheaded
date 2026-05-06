# Vulnerability Management Runbook — Unheaded Kingdom

**Date:** 2026-05-06
**Owner:** Sentinel (with MoatGhost certification, Architect remediation, Developer patch labor)
**Scope:** the full vulnerability-management pipeline for the Unheaded codebase, dependencies, and deployed substrate (WEST + EAST + kind dev cluster + macOS operator workstation).
**Triggering finding:** scrutiny finding **SEN6** in `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` — *"NVD/KEV consumption is not vulnerability management."* The matrix family cited step 1 (consume) and claimed coverage of all 6 steps. This runbook closes that gap.
**Frameworks satisfied (target):** RA-5, SI-5 (NIST 800-53), 03.11.02 (NIST 800-171), CIS Controls 7.1-7.7, FedRAMP Moderate vuln-management baseline, SOC 2 CC7.1, PCI 6.3.1, ISO 27001 A.8.8.
**Operating posture:** single-operator kingdom. Honesty over marketing. Documentation lives in version control. SLAs are aspirational targets to track against today, not certifications already earned.

---

## 0. Pipeline at a glance

```
┌──────────┐   ┌────────┐   ┌──────────────┐   ┌──────┐   ┌────────┐   ┌──────┐
│ CONSUME  │ → │ TRIAGE │ → │ PRIORITIZE   │ → │ PATCH│ → │ VERIFY │ → │ CLOSE│
│ (feeds)  │   │ (apply │   │ (severity +  │   │ (lock│   │ (binary│   │(POA&M│
│ NVD/KEV/ │   │ to dep │   │ SLA + dep-   │   │ file │   │ +runtime│  │track)│
│ GHSA     │   │ graph) │   │ class boost) │   │ bump)│   │ check) │   │      │
└──────────┘   └────────┘   └──────────────┘   └──────┘   └────────┘   └──────┘
     daily        daily         weekly          per-CVE   per-deploy   monthly
```

The kingdom executes step 1 well, step 2 partially, steps 3-6 informally. This runbook makes 3-6 explicit and measurable.

---

## 1. Feed consumption

### 1.1 Current feeds (operating today)

| Feed | Mechanism | Cadence | Output | Operator |
|------|-----------|---------|--------|----------|
| **NIST NVD** | MCP connector via Sentinel skill | Daily (03:00 UTC, Daily Adversarial Loop) | CVE list with CVSS v3.1 vectors, affected CPEs, fix versions | Sentinel |
| **CISA KEV** | MCP connector via Sentinel skill | Daily (03:00 UTC) | KEV-flagged CVEs (actively exploited in the wild) | Sentinel |
| **`govulncheck`** | `.github/workflows/security.yml` job `govulncheck` | Daily 06:00 UTC + every push to main + every PR | Go-specific vuln list filtered to symbols actually called | CI |
| **`gosec`** | `.github/workflows/security.yml` job `gosec` | Daily 06:00 UTC + push + PR | SARIF report, source-level Go security smells (not strictly CVE) | CI |
| **`cargo audit`** | `.github/workflows/security.yml` job `cargo-audit` | Daily 06:00 UTC + push + PR | RustSec advisory hits across all 8 Cargo.lock files | CI |
| **Trivy filesystem** | `.github/workflows/security.yml` job `trivy-fs` | Daily 06:00 UTC + push + PR | CRITICAL+HIGH SARIF, broad ecosystem coverage | CI |
| **Anchore Grype** | `.github/workflows/sbom-audit.yml` job `grype-scan` | Weekly (Mon 09:00 UTC) | SARIF against generated CycloneDX SBOM | CI |
| **Anchore scan-action** | `.github/workflows/sbom.yml` | Per-push to main + per-PR | Fail-build on `severity: high` or above | CI |

**Output sinks:**

- CI runs upload SARIF to GitHub Code Scanning (Security tab).
- `cargo-audit` JSON reports are uploaded as artifacts (`cargo-audit-reports`, 30-day retention).
- SBOM artifacts (`sbom-cyclonedx.json`, `sbom-spdx.json`, `SBOM-SUMMARY.md`) are uploaded weekly with 30-day retention.
- The weekly audit report (`AUDIT-REPORT.md`) has 90-day retention.
- Sentinel daily summaries are written to Anamnesis Lite (eBPF event store) and emitted to Wotan topic `sentinel.cve.daily`.

### 1.2 Required additions

| Feed | Why | How (target) | Owner |
|------|-----|-------------|-------|
| **GitHub Security Advisories — Go modules** | NVD lags GHSA by 1-30 days for Go ecosystem | `gh api /repos/<dep>/security-advisories` cron in security.yml; or Dependabot security updates on `go.mod` | Sentinel + CI |
| **GitHub Security Advisories — Cargo crates** | Same lag for RustSec; GHSA covers some non-RustSec advisories | Dependabot on each Cargo.lock; or `gh api` cron | Sentinel + CI |
| **GitHub Security Advisories — Python (raft/, Zhen stack)** | `pip-audit` covers PyPA advisory db; GHSA covers the rest | `pip-audit` job on `sbom/python-requirements.txt` | Sentinel + CI |
| **OSV.dev** (optional but recommended) | Federated vuln DB across ecosystems; covers GHSA + RustSec + PyPA + Go-vuln-db in one feed | `osv-scanner --recursive .` job | Sentinel + CI |
| **Linux kernel advisories** (kernel.org / Ubuntu USN) | eBPF programs run on host kernel; verifier-bypass CVEs matter | RSS to Sentinel daily loop; LXD/EAST + WEST host kernel inventory | Sentinel |
| **HAProxy + nginx security advisories** | Edge LB substrate, attack surface for SC-7 | Vendor RSS feeds via Sentinel | Sentinel |

**Current gap:** GHSA per-ecosystem coverage is implicit (Trivy and Grype consume GHSA), but the kingdom does not have explicit `pip-audit` or `osv-scanner` jobs. Add to `security.yml` as `ghsa-go`, `ghsa-rust`, `ghsa-python` jobs.

### 1.3 Feed-failure detection

Per scrutiny finding **S7**: scheduled CI is not operating CI. A feed that fails silently is worse than no feed.

- Every workflow above MUST have a `security-gate` summary job that exits non-zero if any feed-consumer step exited non-success. `security.yml` already has this — verified.
- Sentinel daily loop MUST publish a heartbeat to Wotan topic `sentinel.heartbeat` every run, signed with ML-DSA-65. A 36-hour gap in heartbeats triggers a PANIC-tier consensus event (per the cross-service health doctrine).
- The vuln-management oncall (today: Stevie alone) reviews the GitHub Security tab weekly. A feed that has not produced output in 7 days is presumed broken.

---

## 2. Triage rules

### 2.1 Reachability questions per CVE

For every CVE landing in any feed, answer in order:

1. **Is the affected component in the kingdom's dependency graph?**
   Cross-reference: `sbom/SBOM.md` (553 deps as of 2026-03-15), `sbom/go-dependencies.json`, `sbom/rust-dependencies.json`, `sbom/python-requirements.txt`, and tonight's `docs/sbom/2026-05-06-sbom-delta.md` (lockfile fingerprints + delta vs. last full ScanCode scan).
   Match on package name AND version range. If no — file as **N/A — not in dep graph**, log to monthly report, close.
   If yes — proceed.

2. **Is the affected symbol/function actually called?**
   For Go: `govulncheck` does call-graph analysis and only flags vulns whose symbols are in the import-and-use graph. Trust its `[reachable]` annotation.
   For Rust: `cargo audit` does NOT do call-graph reachability. Manual code search for the named symbol is required for HIGH+ findings.
   For Python: `pip-audit` does not do call-graph reachability either; treat all hits as reachable until proven otherwise.

3. **Is it reachable in a deployed configuration vs. latent?**
   Deployed = the binary runs on WEST, EAST, in a Sealed Cask, or in CI build infrastructure.
   Latent = the dep is in `go.mod`/`Cargo.lock` but the binary that links it is not currently deployed (e.g., experimental `crates/zhenai-forge` if it never reached real-metal).
   Latent CVEs get one severity-class-down treatment in priority but are NOT ignored — they are deferred until the binary is targeted for build.

4. **Network-exposed in deployed config?**
   Network-exposed = the CVE-affected code path is reachable from outside the host or from a different trust zone (e.g., a CVE in `gorilla/mux` is reachable from anyone who can reach a Doom-Range port; a CVE in `BurntSushi/toml` is only reachable if the kingdom parses attacker-controlled TOML).
   Network-exposed CVEs get one severity-class-up treatment.

5. **What's the score?**
   Record CVSS v3.1 base score and EPSS (Exploit Prediction Scoring System) percentile. EPSS > 0.5 means likely exploited within 30 days; treat as if KEV-listed.

### 2.2 Triage outcomes

Each CVE gets exactly one disposition:

| Disposition | Meaning | Next step |
|-------------|---------|-----------|
| **PATCH** | In dep graph, reachable, deployed | Go to §3 (Prioritize) and §4 (Patch) |
| **PATCH-LATENT** | In dep graph, reachable, not deployed today | Patch on next deploy; track with relaxed SLA |
| **MITIGATE** | Reachable but no upstream patch yet | Apply compensating control (network policy, WAF rule, code-level guard); track in POA&M with mitigation evidence |
| **EXCEPT** | Cannot patch (breaking change, vendor abandoned, ROI negative); see §7 | Exception package, sign-off, recurring re-review |
| **N/A** | Not in dep graph, or symbol unreachable per call-graph analysis | Log + close. No POA&M entry. |
| **DUPLICATE** | Already tracked under another CVE ID (NVD reissue, GHSA crosspost) | Link to primary tracking item. |

### 2.3 Triage SLA (time from feed to disposition)

| Severity | Triage SLA |
|----------|-----------|
| KEV-listed (any CVSS) | 24 hours |
| CVSS 9.0-10.0 (Critical) | 48 hours |
| CVSS 7.0-8.9 (High) | 5 business days |
| CVSS 4.0-6.9 (Moderate) | 14 days |
| CVSS 0.0-3.9 (Low) | 30 days |

Triage that exceeds SLA itself becomes a POA&M line item ("triage-overdue").

---

## 3. Prioritization

### 3.1 Baseline SLAs (FedRAMP Moderate aligned)

| Severity | Remediate by |
|----------|-------------|
| **Critical** (CVSS 9.0+ or KEV-listed) | **30 days** from triage disposition |
| **High** (CVSS 7.0-8.9) | **90 days** |
| **Moderate** (CVSS 4.0-6.9) | **180 days** |
| **Low** (CVSS 0.0-3.9) | **365 days** or next major release, whichever is sooner |

### 3.2 Accelerated dep-classes

The following dep classes get **one severity-class boost** (Moderate → High, High → Critical) regardless of CVSS:

| Dep class | Why | Examples in current SBOM |
|-----------|-----|---------------------------|
| **eBPF kernel-touching deps** | A vuln here is kernel-mode escalation surface | `aya`, `aya-ebpf`, `cilium/ebpf`, all `crates/*-ebpf` workspace crates |
| **Crypto deps** | A vuln here breaks confidentiality, integrity, signing | `cloudflare/circl` (PQC: ML-DSA-65, SLH-DSA), `golang.org/x/crypto`, `ring`, `rustls` |
| **Network-exposed deps (handle attacker-controlled bytes)** | First contact with the wire | `gorilla/mux`, `gorilla/websocket`, `google.golang.org/grpc`, `tonic`, `hyper`, `tokio`, `quinn` |
| **Auth / session deps** | Bypass = kingdom compromise | `pkg/auth` consumers, JWT libraries, `pkg/champion` |
| **Container / runtime deps** | Substrate compromise | `containerd`, `runc`, LXD plugins, kindnet, HAProxy, nginx |
| **Build/CI deps with code execution** | Supply-chain attack vector | `gh-actions`, syft, grype, scancode, scripts in `scripts/` |

The matrix-cited examples (`pkg/auth`, `pkg/champion`, ML-DSA-65 signing) inherit this boost automatically.

### 3.3 Decelerated dep-classes

The following get **one severity-class relaxation** when (and only when) reachability is rigorously falsified:

| Dep class | Condition |
|-----------|-----------|
| **Test-only deps** (build tag `_test.go`, `dev-dependencies`) | Verified not in any deployed binary |
| **Vendored upstream code** (`llama.cpp/`) | Verified not in Sealed Cask manifest |
| **Documentation-only tooling** | Verified not in production path |

A relaxation does NOT mean ignore. The CVE is still tracked, just on the slower SLA tier.

### 3.4 Prioritization output

For each triage cycle, produce a ranked queue (top of POA&M):

```
RANK | CVE | DEP | SEVERITY | SLA-DUE | DEP-CLASS | NOTES
1    | CVE-2026-XXXXX | aya 0.13.1 | Critical (boost: KEV) | 2026-06-05 | eBPF | reachable in flow-tracker
2    | CVE-2026-YYYYY | x/crypto 0.43.0 | High → Critical (crypto boost) | 2026-06-05 | crypto | used by topic signing
3    | CVE-2026-ZZZZZ | tokio 1.44.2 | High | 2026-08-04 | network | tonic upstream
...
```

---

## 4. Patch procedure

### 4.1 Go modules

**Single dep bump:**

```bash
# 1. Ensure clean working tree
git status
git checkout -b vuln/CVE-2026-XXXXX

# 2. Bump the specific module
go get github.com/foo/bar@v1.2.4   # or @latest if patch is in latest
go mod tidy

# 3. Review the lockfile diff carefully
git diff go.mod go.sum
# Confirm: only the targeted module + its transitives moved.
# If unrelated modules moved, investigate before commit.

# 4. Run the full test matrix
go build ./...                      # compile across all packages
go test ./... -count=1 -race        # race detector enabled (per CLAUDE.md)
# If first run fails transiently, run twice (CLAUDE.md note)

# 5. Re-run the security checks locally
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
# Confirm the target CVE no longer appears.

# 6. Commit
git add go.mod go.sum
git commit -m "fix: bump foo/bar to v1.2.4 (CVE-2026-XXXXX)

CVE-2026-XXXXX (CVSS 9.1, KEV) — <one-line description>.
Verified post-patch: govulncheck clean for this finding.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

**Mass bump (e.g. major version of a transitive cluster):** open a separate PR; coordinate review with Architect; do NOT mix unrelated security bumps in one commit.

### 4.2 Cargo crates

```bash
# Each Cargo.lock is independent. Identify which workspace.
# As of 2026-05-06 there are 8 Cargo.lock files (per SBOM):
#   ebpf/, ebpf/af-xdp/, cmd/ebpf-collector/, cmd/ebpf-collector/ebpf-programs/,
#   cmd/trace-collector/, cmd/ebpf-loader/, crates/monad-mbc/, crates/monad-mbc/fuzz/,
#   crates/zhenai-forge/, crates/zhend/, crates/doom-runner/

cd <workspace>

# 1. Bump targeted crate
cargo update -p <crate-name> --precise <patched-version>

# 2. Review lockfile diff
git diff Cargo.lock
# Same rule: only target + transitives should move.

# 3. Build + test
cargo build --release
cargo test
cargo audit            # confirm CVE no longer appears

# 4. Commit per workspace; do NOT mix workspaces in one commit
git add Cargo.lock
git commit -m "fix(<workspace>): bump <crate> to <ver> (CVE)..."
```

**Lockfile fingerprints:** the kingdom tracks SHA-256 fingerprints of every Cargo.lock in `docs/sbom/*-sbom-delta.md` (introduced 2026-05-06). Every patch updates the fingerprint; the next delta document reflects the change.

### 4.3 Python deps (raft/, Zhen stack)

```bash
cd ~/.venv/zhen   # or the venv tracking sbom/python-requirements.txt

# 1. Pin the patched version
pip install --upgrade <package>==<patched-version>

# 2. Regenerate requirements
pip freeze > sbom/python-requirements.txt

# 3. Re-run inference + RAG smoke tests
python -m raft.zhen_app --self-check     # verify Zhen still loads
# Run any existing integration tests; document pass/fail in the commit.

# 4. Re-run pip-audit (once added to security.yml)
pip-audit -r sbom/python-requirements.txt

# 5. Commit
git add sbom/python-requirements.txt
git commit -m "fix(zhen): bump <package> to <ver> (CVE)..."
```

**Python pinning policy:** the kingdom currently pins exact versions in `sbom/python-requirements.txt`. Major-version bumps require a separate review (langchain/transformers regularly break APIs across minor versions).

### 4.4 Container base images

Sealed Cask containers are built from base images defined in `nix/containers/*.nix` (NixOS) and `Dockerfile`s in service directories.

```bash
# 1. Update the base image reference
# For Nix: bump the flake input or pinned hash
nix flake update --input nixpkgs        # or the specific input

# 2. For Docker base images: edit FROM line, pin to digest (not tag)
#    FROM nixos/nix@sha256:<patched-digest>

# 3. Rebuild the Sealed Cask
./scripts/build-sealed-cask.sh
# Output: deploy/cask-<TIMESTAMP>.tar.gz with new BINDING_RUNE.sha256

# 4. Re-deploy (per the Sealed Cask deploy chain)
# Refer to the Sealed Cask deployment runbook for the redeploy cascade.
```

The Sealed Cask SHA-256 manifest (Binding Rune) is the authoritative record of what is deployed. Any patched binary changes the manifest; the manifest is the verification artifact (see §5).

### 4.5 Patch chain for any binary

```
patch dep → go mod tidy / cargo update / pip freeze
         → CI runs (security.yml + sbom.yml + sbom-audit.yml)
         → CI green
         → merge to main
         → build-sealed-cask.sh produces new cask-<TIMESTAMP>.tar.gz
         → verify-binding-rune.sh on staging host
         → deploy to EAST (staging)
         → smoke test (10-15 min)
         → deploy to WEST (production)
         → smoke test
         → POA&M close-out (§6)
```

Today this chain is largely manual. The CI gate is automated through the SBOM scan; deploy steps are operator-driven.

---

## 5. Verification

### 5.1 Confirm patched dep is actually deployed

A patched dep in `go.mod` proves nothing about what is running. Three independent checks:

#### 5.1.1 Sealed Cask Binding Rune

```bash
# On the staging/production host:
./scripts/verify-binding-rune.sh /path/to/deployed/cask.tar.gz

# Each binary in the cask has a SHA-256 entry in BINDING_RUNE.sha256.
# A patched binary will have a different SHA-256 than the prior cask.
# Compare prior vs current manifest:
diff <(tar -xzOf deploy/cask-<old>.tar.gz BINDING_RUNE.sha256) \
     <(tar -xzOf deploy/cask-<new>.tar.gz BINDING_RUNE.sha256)
# Expected: SHA changed for every binary linking the patched dep.
```

#### 5.1.2 Running version endpoint check

Every Unheaded service exposes `/health` and (per `pkg/service/`) embeds a build version. Add a `/version` endpoint exposing:

- Git commit SHA of build
- Build timestamp
- Sealed Cask manifest SHA (parent manifest the binary was sealed into)

```bash
for port in 16668 17000 18000 19000 20000 20103 19004 19005; do
  curl -sf http://localhost:$port/version | jq -r '.commit + " " + .cask_sha'
done
# Expected: every service reports the post-patch commit + cask SHA.
```

This endpoint is **a TODO as of 2026-05-06** — `/health` and `/ready` exist; `/version` is not standardized. Adding it is a prerequisite to closing this verification step properly. Track as POA&M item: `unheaded-version-endpoint-standardization`.

#### 5.1.3 eBPF flow-marker version-tag

Per the kingdom's eBPF tracing model, every traced packet carries a marker injected at XDP. The marker schema (`packet_marker`) includes a build version field. Kingdom-internal flows can be filtered for: "all packets from any service tagged with build < <patched-commit>". Any non-zero count means a stale binary is still serving traffic.

```bash
# Pseudocode — depends on Anamnesis Lite query API readiness
anamnesis query \
  --topic flow.markers \
  --where 'build_sha != "<patched-commit>"' \
  --since 5m
# Expected: empty (after fleet-wide redeploy)
```

This check is currently **PARTIAL** — flow markers exist; the build_sha field is in the schema but not always populated. POA&M item: `flow-marker-build-sha-completeness`.

### 5.2 Confirm govulncheck/cargo-audit/grype clean for the targeted CVE

The most direct verification: re-run the same scanner that found the CVE, against the deployed cask. Expected: the CVE no longer appears.

```bash
# In CI post-deploy:
govulncheck ./...                   # for Go
cargo audit                         # per Cargo.lock
grype sbom:sbom-cyclonedx.json      # multi-ecosystem
# Failure if the CVE is still listed.
```

---

## 6. Close-out + POA&M

### 6.1 Tracker location

The Plan of Action and Milestones (POA&M) tracker lives at:

`docs/compliance/poam/poam-2026-05-06.md` (proposed; create on first triage cycle).

Format: append-only ledger; one entry per CVE per disposition. Closed entries are not deleted; they are marked `CLOSED` with the closure date.

### 6.2 POA&M template (per entry)

```markdown
### POA&M-2026-XXXX

| Field | Value |
|-------|-------|
| **CVE ID** | CVE-2026-XXXXX |
| **Discovered** | 2026-05-06 (feed: NVD via Sentinel) |
| **Triaged** | 2026-05-06 (within SLA) |
| **Disposition** | PATCH |
| **Severity** | Critical (CVSS 9.1; KEV-listed) |
| **Affected Component** | `github.com/foo/bar` v1.2.3 |
| **Dep Class** | network-exposed, gRPC handler |
| **Effective Severity (post-boost)** | Critical (no boost; already top tier) |
| **Reachability** | Reachable per govulncheck call-graph |
| **Deployed Today** | Yes (WEST + EAST + kind) |
| **Target Close Date** | 2026-06-05 (Critical = 30d) |
| **Remediation** | Bump to v1.2.4; rebuild Sealed Cask; redeploy |
| **Compensating Controls** (if any during patch window) | iptables block on attacker-controlled subnet; rate-limit at HAProxy |
| **Owner** | Sentinel + Developer |
| **Actual Close Date** | (filled when CLOSED) |
| **Verification Evidence** | Sealed Cask SHA `abc123...` deployed; govulncheck clean output linked |
| **Status** | OPEN / IN-PROGRESS / CLOSED / EXCEPT |
```

### 6.3 Close-out checklist

Before flipping a POA&M entry to CLOSED:

- [ ] Patch merged to main; commit hash recorded.
- [ ] CI security workflows green on the patched main.
- [ ] Sealed Cask rebuilt with new Binding Rune SHA.
- [ ] Cask deployed to EAST + WEST.
- [ ] Verification step (§5) executed — three independent checks where available.
- [ ] Re-run of the original detecting scanner shows the CVE is no longer reported.
- [ ] POA&M entry updated with actual close date + verification evidence links.
- [ ] If patch took >SLA: write a brief root-cause note (next-page).

### 6.4 Aging report

Every Monday, MoatGhost runs:

```bash
# Pseudocode — to be scripted under scripts/poam-aging.sh
parse docs/compliance/poam/poam-*.md
report:
  open by severity
  past-SLA (red)
  within-SLA-but-overdue-90% (yellow)
  closed-this-week (green)
```

Past-SLA items become headline gaps in the next monthly report (§8).

---

## 7. Exception process

A vulnerability that cannot be closed within SLA enters the exception path. The exception is documented, time-bounded, signed by the operator, and re-reviewed.

### 7.1 Exception triggers

- Upstream patch not yet released.
- Patch exists but introduces breaking change requiring more than SLA-window of dev work.
- Upstream is abandoned; no patch will exist.
- Patch is incompatible with another required dep.
- Patch removes functionality the kingdom uses (and a refactor is the only path).

### 7.2 Exception package contents

For each exception, write to `docs/compliance/exceptions/EXC-2026-XXXX.md`:

```markdown
# EXC-2026-XXXX: <CVE-ID> exception

**Approved by:** <operator name + role>
**Approved date:** YYYY-MM-DD
**Re-review date:** YYYY-MM-DD (max +90 days for Critical, +180 for High, +365 for Mod)

## What and why

<one paragraph: the CVE, the dep, why we cannot patch within SLA>

## Compensating controls (mandatory)

<network policy / WAF rule / code-level guard / monitoring rule that reduces the
effective likelihood or impact of exploitation>

## Risk acceptance statement

I accept the residual risk that <attacker capability> remains exploitable until
<re-review date>. The compensating controls listed above are deployed and verified
operating as of this approval date.

Signed: <operator>
```

### 7.3 Exception review cadence

| Exception severity | Maximum re-review window |
|--------------------|-------------------------|
| Critical | 30 days |
| High | 90 days |
| Moderate | 180 days |
| Low | 365 days |

A re-review either renews the exception (with the same package + a new signed re-review date) or closes it (patch finally available, refactor complete, dep removed).

### 7.4 Exception caps

The kingdom commits to:

- No more than **3 open Critical exceptions** at any time.
- No more than **10 open High exceptions** at any time.
- All open exceptions are listed in the monthly report (§8).

Caps exceeded = halt feature development; remediation becomes the only sprint target.

---

## 8. Reporting

### 8.1 Monthly cadence

First business day of each month, MoatGhost publishes `docs/compliance/vuln-mgmt/monthly-YYYY-MM.md`:

```markdown
# Vulnerability Management Monthly Report — YYYY-MM

## Headline metrics

| Metric | This month | Last month | Trend |
|--------|-----------|-----------|-------|
| Open CVEs (Critical) | N | N | ↑/↓/= |
| Open CVEs (High) | N | N | |
| Open CVEs (Moderate) | N | N | |
| Open CVEs (Low) | N | N | |
| Closed this month (Critical) | N | N | |
| Closed this month (High) | N | N | |
| Closed this month (Moderate) | N | N | |
| Closed this month (Low) | N | N | |
| MTTR — Critical (median) | Nd | Nd | |
| MTTR — High (median) | Nd | Nd | |
| MTTR — Moderate (median) | Nd | Nd | |
| Past-SLA today | N | N | |
| Open exceptions | N | N | |
| Triage queue depth (untriaged) | N | N | |

## CI/feed health

| Feed | Last successful run | Output non-empty? | Reviewed by human? |
|------|--------------------|--------------------|-------------------|
| security.yml — govulncheck | YYYY-MM-DD | yes/no | yes/no |
| security.yml — gosec | | | |
| security.yml — cargo-audit | | | |
| security.yml — trivy-fs | | | |
| sbom.yml — anchore-scan | | | |
| sbom-audit.yml — grype | | | |
| Sentinel daily NVD/KEV | | | |

## Past-SLA items (red)

(table of POA&M items currently overdue)

## Closed this month (green)

(table of POA&M items closed)

## Open exceptions (amber)

(list of EXC-* with re-review dates)

## Notes

- Anomalies, drifts, near-misses
- Patch-chain failures (e.g. CI red blocked a patch)
- Tooling gaps closed this month
```

### 8.2 Mean Time to Remediate (MTTR)

Computed as: `actual_close_date - triage_disposition_date`, in days, per severity, median over the trailing 90 days. Compared to SLA target. Trendline tracked across reports.

### 8.3 Audit trail

Every POA&M entry, every exception, every monthly report is git-committed. The git history is the audit trail. SOC 2 Type 2 / FedRAMP / NIST 800-53 RA-5 evidence is "show me the POA&M ledger and the monthly reports" — both are reproducible from `git log`.

---

## 9. RACI

| Role | Consume | Triage | Prioritize | Patch | Verify | Close | Except | Report |
|------|:-------:|:------:|:----------:|:-----:|:------:|:-----:|:------:|:------:|
| Sentinel | R, A | R | C | I | I | I | C | C |
| Developer | I | C | C | R, A | C | I | I | I |
| Architect | I | C | C, A | C | R | C | C | I |
| MoatGhost | I | I | I | I | C | R, A | A | R, A |
| BlackMage | I | C (red-team falsifies) | I | I | C | I | I | C |
| Operator (Stevie) | A | A | A | A | A | A | A | A |

R = Responsible, A = Accountable, C = Consulted, I = Informed. Today the kingdom is single-operator: the operator wears Accountable in every column. The skill columns describe the *intended* division as the kingdom grows.

---

## 10. Operating reality (per scrutiny SEN6 + S7)

This runbook describes the **target** state. Today, 2026-05-06, the kingdom operates approximately:

| Step | Today's reality |
|------|-----------------|
| 1. Consume | OPERATING. Daily CI feeds + Sentinel daily loop run. |
| 2. Triage | INFORMAL. No documented disposition for each CVE landing in the GitHub Security tab. |
| 3. Prioritize | INFORMAL. No SLA tracking; no past-SLA list. |
| 4. Patch | OPERATING for emergencies (KEV + Critical land same week). Slow / non-existent for Moderate. |
| 5. Verify | PARTIAL. Sealed Cask SHA changes are observable; `/version` endpoint is not standardized; flow-marker build_sha is partial. |
| 6. Close | NOT OPERATING. No POA&M ledger today. This runbook proposes creating it. |
| 7. Except | NOT OPERATING. Exceptions today are implicit ("we'll get to it"). |
| 8. Report | NOT OPERATING. No monthly cadence. |

**Honest framing:** as of today the kingdom satisfies RA-5 / SI-5 / 03.11.02 / CIS 7.x at PARTIAL, not MAPPED. This runbook is the path to MAPPED — not the proof MAPPED already exists. Any matrix downstream of this runbook should reflect that distinction (action: see scrutiny finding **F12** — apply corrections across matrix family).

---

## 11. CI workflow SLA support audit

| Workflow | What it does | SLA support today |
|----------|-------------|-------------------|
| `.github/workflows/security.yml` | Daily Go/Rust/Trivy vuln scan + gate | NO SLA enforcement. Fails on detection but does not enforce remediation deadline. No POA&M wiring. |
| `.github/workflows/sbom.yml` | Per-push SBOM gen + Anchore scan with `fail-build: true` on `severity: high` | NO SLA enforcement; PR gate only. Blocks new High+ from landing but does not track existing. |
| `.github/workflows/sbom-audit.yml` | Weekly Syft SBOM + Grype scan + license check + audit report (90d retention) | NO SLA enforcement. Reports vulns but does not assign owners or due dates. |
| `.github/workflows/gpl-boundary.yml` | License-only check (not vuln-mgmt) | N/A — license scope. |

**Verdict:** zero workflows today track or enforce remediation SLA. The CI surface enforces *"don't land new Highs"* (sbom.yml) and *"alert on existing"* (security.yml + sbom-audit.yml). SLA tracking is the runbook's POA&M ledger, which is **not yet** written into CI. Adding SLA enforcement to CI is a follow-up task: a `poam-aging` job that reads `docs/compliance/poam/*.md`, computes age of each open entry, and fails the gate if any Critical is past 30d / High past 90d / Moderate past 180d.

---

## 12. References

- **Scrutiny finding SEN6**: `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` §SEN6
- **SBOM (last full)**: `sbom/SBOM.md` (2026-03-15)
- **SBOM delta tonight**: `docs/sbom/2026-05-06-sbom-delta.md`
- **Control matrices citing this runbook**:
  - `docs/compliance/control-matrix/cis-controls-v8-2026-05-06.md` §7.1-7.7
  - `docs/compliance/control-matrix/nist-800-53-2026-05-06.md` RA-5, SI-5
  - `docs/compliance/control-matrix/nist-800-171-2026-05-06.md` 03.11.02
  - `docs/compliance/control-matrix/fedramp-moderate-2026-05-06.md` (RA-5 SLA row)
  - `docs/compliance/control-matrix/soc2-2026-05-06.md` CC7.1
  - `docs/compliance/control-matrix/pci-dss-2026-05-06.md` 6.3.1
  - `docs/compliance/control-matrix/iso-27001-27002-2026-05-06.md` A.8.8
- **Sentinel skill**: `docs/skills/unheaded-sentinel.md`
- **Sealed Cask**: `scripts/build-sealed-cask.sh`, `scripts/verify-binding-rune.sh`, ADR-010
- **Related runbooks**:
  - `runbooks/security/incident-response.yaml` (post-exploitation handling)
  - `runbooks/security/security-audit.yaml` (host hardening verification)
  - `runbooks/security/container-security-scan.yaml` (image scanning)
  - `runbooks/security/secrets-rotation.yaml` (key rotation cadence)

---

## 13. Acceptance criteria (when this runbook becomes operating-truth)

This runbook is "operating" — not just written — when:

- [ ] `docs/compliance/poam/` directory exists with at least one cycle of POA&M entries.
- [ ] At least one monthly report has been published from `docs/compliance/vuln-mgmt/`.
- [ ] `/version` endpoint exists on every Doom-Range service (POA&M tracked).
- [ ] `pip-audit`, `osv-scanner` (or equivalent) jobs exist in `.github/workflows/security.yml`.
- [ ] A `poam-aging` CI job exists and fails on past-SLA Critical/High items.
- [ ] At least one CVE has been triaged → patched → verified → closed end-to-end through the pipeline as documented here, with the closure evidence linkable in git.
- [ ] At least one exception has been opened, signed, scheduled for re-review.
- [ ] MTTR (median, trailing 90 days) is being computed and trended.

Until all of these are checked, the matrix family must continue to mark RA-5 / SI-5 / 7.1 / 7.4 / 7.7 as **PARTIAL**, not MAPPED.

---

*The badge is fair. The road ahead is honest. Consume → triage → prioritize → patch → verify → close. Step 1 alone is a feed, not a program.*
