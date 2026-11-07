# Pre-Public Blockers

**Opened:** 2026-07-29
**Context:** Stevie — *"our codebase is large but it's just me and you, we need to be on
top of this before posting and boasting this project on any public forums."*

These items were written to be resolved **before** the repository went public. It is
public now, so they are overdue, and promotion waits on them. Everything else in `findings-remediation-2026-07-29.md` can proceed in
parallel; these cannot be deferred past publication.

---

## BLOCKER 1 and 2 — Secrets in history (decision rewritten 2026-11-06) 🔴

**The repository is public, so every value in history counts as burned.**

The 2026-07-29 decision was written for a repository that was not public: it
took the baselined values as one-off lab credentials and left them in place.
Its own condition said that stopped holding on exposure, and exposure has
happened. It is replaced, and git keeps the old text.

**What holds now:**

1. Every credential-shaped value that ever reached git history is treated as
   known to anyone. Nobody is asked to prove that a given value leaked, and
   none is re-used.
2. Every such value that is still in use is rotated at S-E1-LIVE, with Stevie.
   New values live outside the repo. Rotation is the only action that restores
   anything: history stays as it is, and is not rewritten.
3. A row in `.gitleaksignore` or `docs/policy/gitleaks-history-baseline.txt`
   means known, not accepted and not reviewed clear.

**What stays from 2026-07-29:**

1. The policy is written where it will be read: `CLAUDE.md` § Secrets
   Management, stated absolutely.
2. **Enforcement is mechanical, not cultural.** `gitleaks` gates every push and
   PR to main/develop/staging, and `scripts/check-secrets-baseline.sh` makes
   `.gitleaksignore` shrink-only, so a new finding cannot be silenced by
   appending its fingerprint.

---

## BLOCKER 3 — Honest gate posture at publication 🟡

A green CI badge over 1172 unlooked-at gosec findings is **worse than no badge**. The
first person to clone and run `gosec ./...` finds them in under a minute, and the story
becomes "their CI claims clean and it isn't" — a credibility problem, not a security one.

Acceptable at publication:
- The ratchet is **visible**: `-exclude=` list in the workflow, baseline file committed,
  remediation plan public. Anyone can see exactly what is suppressed and why.
- `README` states the security posture plainly rather than implying completeness.

Not acceptable:
- Any gate that cannot fail (this repo had **three**: `cargo audit || true`,
  `//nolint:gosec` annotations gosec ignores, trivy scanners never enabled).
- A badge implying a clean scan when 26 rule classes are excluded.

---

## BLOCKER 4 — Repo hygiene 🟡

- [ ] `CLAUDE.md:1128` still claims branch strategy `main/staging/feature/` — never
      matched the real `sNN-` convention and now contradicts the `develop` topology in
      `CONTRIBUTOR-GUIDE.md` §5.
- [ ] `llama.cpp/` sits untracked in the working tree (1.3 GB). Confirm it is ignored,
      not accidentally committable.
- [ ] `SECURITY.md` — no vulnerability disclosure policy exists. A public repo needs one;
      without it, finders have no non-public channel and will open a public issue.
- [ ] Enable GitHub secret scanning + push protection (free on public repos) so this
      class cannot recur.

---

## Recommended additions before promotion (not blockers)

| Gate | Why it matters publicly |
|---|---|
| **CodeQL** | Free on public repos. Semantic analysis catches taint-flow bugs gosec's pattern matching misses. Its absence is conspicuous on a security-focused project. |
| **OpenSSF Scorecard** | Grades exactly what was fixed here — pinned actions, branch protection, secret scanning, maintained deps. A public, comparable score is strong credibility for a solo project. |
| **Dependabot / Renovate** | Without it this sweep simply recurs. 9 HIGH advisories accumulated in ~2 months. |
| **SBOM publish + cosign signing** | SBOM is generated but never published or signed. Supply-chain expectation for infra tooling. |
| **`kubeconform` / `kube-linter`** | Manifests are only validated by trivy misconfig today; neither checks schema validity. |

---

## Status

| Blocker | State |
|---|---|
| 1. Secrets in history | **OPEN** — every value counts as burned; gate added, baseline pinned; history is not rewritten |
| 2. Credential rotation | **OPEN** — at S-E1-LIVE, with Stevie |
| 3. Honest gate posture | Partially met — ratchet is visible; README wording pending |
| 4. Repo hygiene | **OPEN** — 4 items |
