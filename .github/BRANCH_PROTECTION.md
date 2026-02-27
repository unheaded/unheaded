# Branch Protection Policy

**Project:** Unheaded Kingdom
**Date:** 2026-02-27
**Status:** RECOMMENDED CONFIGURATION

---

## Overview

This document defines recommended branch protection rules for the `main` and `develop` branches to ensure code quality, security, and compliance as part of the S72 DevOps automation initiative.

---

## Main Branch (`main`)

### Purpose
Production-ready code and releases. Only merge stable, fully-tested changes.

### Protection Rules

#### Required Status Checks (All must pass)
- [x] `go-build` — Go build & test (ci.yml)
- [x] `go-lint` — golangci-lint (ci.yml)
- [x] `sbom` — SBOM generation & Grype scan (ci.yml)
- [x] `govulncheck` — Go vulnerability check (ci.yml)
- [x] `gosec` — Go security scan (ci.yml)
- [x] `license-check` — License compliance (ci.yml)
- [x] `ci-gate` — Protocol Foundation CI gate (ci-protocol.yml, if applicable)

#### Required Reviews
- [x] Require pull request reviews before merging: **2 approvals**
- [x] Dismiss stale pull request approvals when new commits are pushed
- [x] Require code owner review: **Yes** (if CODEOWNERS file exists)
- [x] Require conversation resolution before merging: **Yes**

#### Restrictions
- [x] Require branches to be up to date before merging
- [x] Require deployments to succeed before merging:
  - Deployment environment: `production` (if configured)
- [x] Restrict who can push to matching branches: **Only admins**
- [x] Allow force pushes: **No** (prevent accidental history rewrites)
- [x] Allow deletions: **No** (prevent accidental branch deletion)

#### Dismissal Restrictions
- [x] Restrict who can dismiss pull request reviews: **Only admins or code owners**

#### Bypass Rules
- [x] Restrict who can bypass required checks: **Only repository administrators**
- Allow bypass actors: (Leave empty — only admins can bypass)

---

## Develop Branch (`develop`)

### Purpose
Development integration branch. Staging for features being prepared for `main`.

### Protection Rules

#### Required Status Checks (All must pass)
- [x] `go-build` — Go build & test (ci.yml)
- [x] `go-lint` — golangci-lint (ci.yml)
- [x] `sbom` — SBOM generation & Grype scan (ci.yml)
- [x] `govulncheck` — Go vulnerability check (ci.yml)
- [x] `gosec` — Go security scan (ci.yml)
- [x] `license-check` — License compliance (ci.yml)

#### Required Reviews
- [x] Require pull request reviews before merging: **1 approval**
- [x] Dismiss stale pull request approvals when new commits are pushed
- [x] Require code owner review: **No** (less stringent than main)
- [x] Require conversation resolution before merging: **Yes**

#### Restrictions
- [x] Require branches to be up to date before merging: **Yes**
- [x] Restrict who can push to matching branches: **Only admins and developers**
- [x] Allow force pushes: **No**
- [x] Allow deletions: **No**

---

## Release Branches (Optional: `release/*`, `hotfix/*`)

### Purpose
Isolation for release stabilization and critical hotfixes.

### Protection Rules (if implemented)

#### Required Status Checks
- All checks from `develop` branch

#### Required Reviews
- [x] Require pull request reviews before merging: **1 approval**
- [x] Require conversation resolution: **Yes**

#### Restrictions
- [x] Require branches to be up to date before merging
- [x] Allow force pushes: **No**
- [x] Allow deletions: **No**

---

## Feature Branches (No protection)

### Purpose
Development-in-progress branches. No restrictions; developers have full control.

**Pattern:** `feature/*`, `bugfix/*`, `refactor/*`, `docs/*`

**Policy:** No branch protection rules applied.

---

## Implementation Steps

### Via GitHub Web UI

1. Navigate to **Settings → Branches**
2. Click **Add rule** for each branch pattern
3. Configure per rules above
4. Click **Create**

### Bulk Configuration with GitHub CLI

```bash
# Protect main branch
gh api repos/unheaded-kingdom/unheaded/branches/main/protection \
  --input - <<EOF
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "go-build",
      "go-lint",
      "sbom",
      "govulncheck",
      "gosec",
      "license-check",
      "ci-gate"
    ]
  },
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": true,
    "required_approving_review_count": 2
  },
  "enforce_admins": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF

# Protect develop branch
gh api repos/unheaded-kingdom/unheaded/branches/develop/protection \
  --input - <<EOF
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "go-build",
      "go-lint",
      "sbom",
      "govulncheck",
      "gosec",
      "license-check"
    ]
  },
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 1
  },
  "enforce_admins": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
```

---

## Enforcement Matrix

| Aspect | Main | Develop | Feature |
|--------|------|---------|---------|
| **Status Checks** | All 7 | 6 (no ci-gate) | None |
| **Code Review** | 2 approvals | 1 approval | None |
| **Code Owner Review** | Required | Not required | N/A |
| **Up-to-date** | Required | Required | Not enforced |
| **Force Push** | Disabled | Disabled | Allowed |
| **Delete** | Disabled | Disabled | Allowed |
| **Admin Override** | Enforced | Enforced | N/A |

---

## Workflow: Feature → Main

### Typical Flow

```
feature/add-telemetry
       ↓
    (push commits)
       ↓
   Create PR to develop
       ↓
   [CI checks run]
       ↓
   [1 reviewer approves]
       ↓
  Merge to develop ✅
       ↓
  [Integration testing on develop]
       ↓
  Create PR to main
       ↓
  [All 7 CI checks run]
       ↓
  [2 reviewers approve + code owner]
       ↓
 Merge to main ✅
       ↓
 Tag release: v1.2.3
       ↓
 [Release workflow builds + signs artifacts]
```

---

## Bypassing Protections

### When to Bypass

**Acceptable scenarios:**
- Critical security hotfix (with after-action review)
- Production incident requiring immediate rollback
- Emergency data migration

**Never bypass for:**
- Skipping failing tests
- Avoiding code review
- Pushing security-related code without review

### Bypass Procedure

1. **Admin-only action:** Only repository administrators can bypass
2. **Log decision:** Document reason in PR comments
3. **After-action review:** Post-incident review within 24 hours
4. **Update policies:** Modify protection rules if pattern recurs

---

## Monitoring & Alerts

### GitHub Settings to Monitor

- Branch protection rule changes (via audit log)
- Administrator overrides of CI checks
- PR dismissals of security reviews

### Required Tooling

- [GitHub Audit Log](https://github.com/orgs/unheaded-kingdom/audit-log) monitoring
- SIEM integration (if available) for suspicious merge activity
- Slack/webhook notifications for bypass events

---

## Troubleshooting

### "Required status checks are missing"

**Cause:** A required check is not configured in the workflow
**Solution:** Verify check name in CI YAML matches protection rule exactly (case-sensitive)

### "Cannot merge: awaiting code owner review"

**Cause:** CODEOWNERS file specifies code owner not yet approved
**Solution:** Request approval from specified code owner or admin override (only if justified)

### "Administrator required to merge"

**Cause:** Attempted bypass of required checks by non-admin
**Solution:** Request administrator to review and merge, or request status check waiver

---

## Review Cadence

- **Quarterly:** Review protection rules for relevance (Q1, Q2, Q3, Q4)
- **On policy change:** Update this document within 48 hours
- **On incident:** Post-mortem review of bypass decisions

---

## References

- [GitHub Branch Protection Documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches)
- [GitHub CLI Branch Protection Commands](https://cli.github.com/manual/gh_api)
- S72 Battle Plan — Phase 9 (GHA Hardening)
- [Unheaded SECURITY.md](../../SECURITY.md)

---

**Last Updated:** 2026-02-27
**Next Review:** 2026-05-27 (Q2)
