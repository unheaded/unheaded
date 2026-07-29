# Pre-Public Blockers

**Opened:** 2026-07-29
**Context:** Stevie — *"our codebase is large but it's just me and you, we need to be on
top of this before posting and boasting this project on any public forums."*

These are the items that must be resolved **before** the repository is made public or
promoted. Everything else in `findings-remediation-2026-07-29.md` can proceed in
parallel; these cannot be deferred past publication.

---

## BLOCKER 1 — Secrets in git history (76 findings) 🔴

`gitleaks git` over 2149 commits, 2026-02-23 → 2026-05-12: **76 findings.**
`gitleaks dir` over the working tree: **36** (28 in tracked files).

**Making a repo public exposes its entire history.** Deleting a file today does nothing —
every prior revision remains fetchable. `git log -p`, GitHub's UI, and any clone all
expose it. Automated scrapers hit new public repos within minutes.

### Breakdown

| Rule | Count |
|---|---|
| generic-api-key | 37 |
| curl-auth-user | 24 |
| curl-auth-header | 7 |
| hashicorp-tf-password | 4 |
| jwt | 4 |

28 of 76 are in `_test.go` / `tests/` / `cmd/lich-security` and are probably fixtures.
**48 are not**, and several look like live infrastructure credentials:

| File | What |
|---|---|
| `docs/bare-metal/BOOT_SEQUENCE_HOST_A.md` | `curl -sk https://192.168.1.1:443/ -u <creds>` — firewall admin |
| `docs/bare-metal/BOOT_SEQUENCE_HOST_B.md` | `curl -sk https://192.168.2.1:8443/ -u <creds>` |
| `docs/bare-metal/WIREGUARD_TUNNEL.md` | `curl -sk -u <creds>` |
| `scripts/firewall/firewall-health-check.sh` | `curl -sk -u <creds>` |
| `scripts/bare-metal/validate-host-a.sh` | `curl -sk -u <creds>` |
| `deploy/tofu/environments/dev/observability/terragrunt.hcl` | `grafana_admin_password` |
| `docker/telemetry/grafana/grafana.ini` | `secret_key` |
| `nix/yggdrasil/packer/template.pkr.hcl` | `ssh_password` |

### Required actions, in order

1. **Triage** each of the 48 non-test findings: real credential, or placeholder?
2. **ROTATE every real one.** Non-negotiable and cannot be skipped by rewriting history
   instead — assume anything ever committed is already compromised. Rotation is the only
   action that actually restores security; history rewriting only limits further spread.
   This is a **human task** — firewall/Grafana/WireGuard credentials are Stevie's to change.
3. **Remove from the working tree** — replace with `$ENV_VAR` references or SOPS+age
   (already the documented Kingdom pattern for secrets).
4. **Excise from history** with `git-filter-repo` (preferred) or BFG, *then* force-push.
   Do this while the repo is still private and the only clones are yours — after
   publication it is unfixable.
5. **Shrink `.gitleaksignore`** as each is resolved. It may never grow.

> The 18 MB `cmd/akira/akira` blob is also still in history. If a history rewrite happens
> anyway, drop it in the same pass — it is the single largest object in the repo.

---

## BLOCKER 2 — Credential rotation independent of history 🔴

Even if history is rewritten, **rotate first**. Anything committed to a repo — private or
not — must be treated as disclosed. Private repos are readable by every collaborator,
every integration with repo scope, every CI provider, and every local clone and backup.

Checklist (Stevie only — Claude cannot rotate these):
- [ ] Firewall admin (`192.168.1.1`, `192.168.2.1`)
- [ ] Grafana admin password + `secret_key`
- [ ] WireGuard keys referenced in `WIREGUARD_TUNNEL.md`
- [ ] Packer/`ssh_password` build credential
- [ ] Any JWT signing key from the 4 `jwt` findings

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
