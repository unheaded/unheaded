# Branch Protection Policy

**Project:** Unheaded
**Date:** 2026-11-06
**Status:** Required status checks and no review count. Not on yet: Stevie turns it on in GitHub after the first green CI (FND-042).

One operator cannot review his own work, so a review count would stop every merge. What holds a merge is the checks.

A check's name is the job's `name:`, or its id when it has none; a name no job reports leaves the merge waiting forever. `scripts/check-ci-verdicts.sh` holds this file to the workflows.

---

## Main Branch (`main`)

### Purpose
Promoted code and releases.

### Protection Rules

#### Required Status Checks (All must pass)
- [x] `CI Gate` — every job in ci.yml: Go build and lint, Rust, SPDX, licenses
- [x] `Security Gate` — security.yml: govulncheck, gosec, gitleaks, cargo-audit, Trivy
- [x] `sbom` — SBOM generation and Grype scan (sbom.yml)

#### Restrictions
- [x] Require branches to be up to date before merging
- [x] Allow force pushes: **No**
- [x] Allow deletions: **No**

---

## Develop Branch (`develop`)

### Purpose
The integration branch. Recent work lands here first.

### Protection Rules

#### Required Status Checks (All must pass)
- [x] `CI Gate` — every job in ci.yml: Go build and lint, Rust, SPDX, licenses
- [x] `Security Gate` — security.yml: govulncheck, gosec, gitleaks, cargo-audit, Trivy
- [x] `sbom` — SBOM generation and Grype scan (sbom.yml)

#### Restrictions
- [x] Require branches to be up to date before merging
- [x] Allow force pushes: **No**
- [x] Allow deletions: **No**

---

## Staging Branch (`staging`)

The same rules as `develop`, in the same command with the branch name changed.

---

## Other Branches

No protection.

---

## Turning It On

```bash
# main; repeat with develop and staging in the path
gh api --method PUT repos/unheaded/unheaded/branches/main/protection \
  --input - <<EOF_JSON
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "CI Gate",
      "Security Gate",
      "sbom"
    ]
  },
  "required_pull_request_reviews": null,
  "restrictions": null,
  "enforce_admins": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF_JSON

# develop
gh api --method PUT repos/unheaded/unheaded/branches/develop/protection \
  --input - <<EOF_JSON
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "CI Gate",
      "Security Gate",
      "sbom"
    ]
  },
  "required_pull_request_reviews": null,
  "restrictions": null,
  "enforce_admins": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF_JSON
```

With `enforce_admins` true, a push straight to a protected branch is refused until its commit has the required checks, admins included, so changes go through a branch and a pull request.

---

## Troubleshooting

### "Required status checks are missing"

**Cause:** the name in the rule is not one a job reports.
**Solution:** match the job's `name:` exactly, case included. `scripts/check-ci-verdicts.sh` catches this for the names above.

---

## References

- [GitHub branch protection](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches)
- [GitHub REST API: update branch protection](https://docs.github.com/en/rest/branches/branch-protection)
