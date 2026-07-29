# Pre-Public Blockers

**Opened:** 2026-07-29
**Context:** Stevie — *"our codebase is large but it's just me and you, we need to be on
top of this before posting and boasting this project on any public forums."*

These are the items that must be resolved **before** the repository is made public or
promoted. Everything else in `findings-remediation-2026-07-29.md` can proceed in
parallel; these cannot be deferred past publication.

---

## RESOLVED BY RISK DECISION — Secrets in the repo (2026-07-29)

`gitleaks` found **76 findings across git history** and 25 in the working tree.
I flagged these as a hard pre-public blocker requiring rotation and a history
rewrite. **Stevie overruled that, with context I did not have, and the call
stands:**

> "At this point all we have are 1-off credentials used in this lab. It IS bad
> hygiene — I had always advocated for storing env variables outside the repo,
> but apparently some things slipped through. We don't need to scrub history or
> panic. This is in active development, it is not production ready for the
> public-facing internet."

That is a sound reading of the actual exposure: one-off credentials on a
non-internet-facing development system, in a repo whose blast radius is a home
lab. Rotating them buys little; rewriting 2149 commits of history buys less.

**What was done instead:**

1. Live credentials removed from the tree where found — 3 OPNsense API call
   sites now read `OPNSENSE_API_KEY`/`_SECRET` with a `:?` guard so the script
   fails loudly rather than running unauthenticated.
2. The policy is written where it will be read: `CLAUDE.md` § Secrets
   Management, stated absolutely.
3. **Enforcement is mechanical, not cultural.** `gitleaks` gates every push and
   PR to main/develop/staging, and `scripts/check-secrets-baseline.sh` makes
   `.gitleaksignore` shrink-only — a new finding cannot be silenced by
   appending its fingerprint, which is precisely how a suppression list becomes
   the next gate that cannot fail.

**The condition that changes this.** The decision rests on the system being
non-internet-facing and holding nothing real. Before this project is exposed
publicly or handles anything of consequence, **every baselined credential must
be rotated.** "It was only a lab" describes where the secret was used, not the
secret itself — once the repo is public, history is public with it, and
rotation is the only action that restores anything.

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
| 1. Secrets in history | **OPEN** — 76 findings, gate added, baseline pinned |
| 2. Credential rotation | **OPEN** — human task, Stevie only |
| 3. Honest gate posture | Partially met — ratchet is visible; README wording pending |
| 4. Repo hygiene | **OPEN** — 4 items |
