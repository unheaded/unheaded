<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-089 — `develop` → `staging` → `main` Promotion Workflow

| | |
|---|---|
| **Status** | **Accepted** (Stevie's call, 2026-08-03) |
| **Date** | 2026-08-03 |
| **Supersedes** | The branch-strategy paragraph in `CONTRIBUTOR-GUIDE.md` (`main`/`staging`/`feature/` with no promotion gates) |
| **Related** | ADR-073 (lint policy: zero findings as the floor), `docs/security/findings-remediation-2026-07-29.md` (ratchet policy), `docs/battle-plans/STAGING-LADDER-2026-08-03.md` (first run through this workflow) |

---

## Context

Until now this repo has effectively had one working branch. `main` and `develop` both
exist, `develop` is 9 commits ahead, and `staging` did not exist at all until the
Staging Ladder sprint created it. There was no written rule for what earns a promotion
from one branch to the next — promotions happened when the work felt done.

That was survivable while the work was greenfield. It stops being survivable now for a
specific reason: **the recent work is a different kind of work.** As Stevie put it, "all
we did is patched some flagged code, packages, etc so far and made test." Static-analysis
remediation is precisely the category of change that looks safe, touches hundreds of
files at once, has no obvious blast radius, and therefore gets waved through. A blind
`except:` narrowed one line too far, a `datetime` made timezone-aware on the producing
side but not the consuming side, a shell variable deleted because one linter could not
see its consumer in another file — none of these announce themselves. They surface later,
on real hardware, at an inconvenient hour.

A promotion workflow exists to put a **named gate** between "the change compiles" and
"the change is live," and to make the gate's criteria something other than a feeling.

### What this ADR is not

It is not an attempt to import a 200-person release process into a solo project.
Unheaded is Stevie's solo open-source passion project. Ceremony that exists to
coordinate people who do not exist here is pure overhead, and would be abandoned within
a week. Every gate below has to earn its place by catching a class of defect that has
actually bitten this repo.

---

## Decision

Three long-lived branches, each with a different **question** it answers. The branch
structure is not a hierarchy of quality — it is a sequence of questions.

```
feature/*  fix/*  ci/*  docs/*
      |
      |  Q: "Does this change do what it says?"
      v
   develop ................ integration. CI gates. Default branch.
      |
      |  Q: "Does this change break anything else?"
      v
   staging ................ human verification. Deployed to EAST.
      |
      |  Q: "Is this worth being the thing we stand behind?"
      v
    main .................. known-good. Tagged. Protected.
```

### `develop` — the integration branch (default)

**Question answered**: does the change do what it claims, in isolation?

- Default branch on GitHub (this is a change; `main` is default today — see Consequences).
- All topic branches merge here.
- **Entry gate — every CI gate must be green.** Today that means: clippy (0, ratcheted),
  gosec (ratcheted), python syntax, YAML manifests, secrets baseline (shrink-only),
  shellcheck errors, `go build ./...`, `go test ./...`. The Staging Ladder adds ruff and
  shellcheck-warnings to that list.
- Direct commits allowed for the solo case. The gate is CI, not review.
- **`develop` is allowed to be broken briefly.** That is what it is for. A red `develop`
  blocks promotion; it does not block work.

### `staging` — the verification branch

**Question answered**: does the change break anything *else* — including things no test
covers?

- Branched from `develop`. Never receives direct commits; only merges/cherry-picks from
  `develop`.
- **This is where Stevie reviews, one commit at a time.** That is the entire reason the
  branch exists, and it drives the single most important rule in this ADR:

  > **One reviewable unit per commit.** A promotion candidate that is one 400-file
  > commit cannot be verified, only accepted or rejected wholesale. Batch the *work*;
  > never batch the *commit*.

- Deployed to **EAST** (the bare-metal staging host, 4-core/8GB, live since Feb 2026).
  WEST remains the test cluster for full-feature validation.
- **Entry gate (`develop` → `staging`)**:
  1. Every CI gate green on `develop`.
  2. The change is decomposed into individually-revertible commits.
  3. Each commit body states what it changed and why, and — for remediation work — the
     risk rung (see the Staging Ladder's R0-R3 ladder).
  4. `monad-cpu-ebpf --features ascend-linux` is still **901,888 bytes** if the change
     claimed to be inert to the eBPF layer. This is the cheapest available proof that a
     change did not perturb the UPC substrate, and it has caught drift before.

- **Exit gate (`staging` → `main`)**:
  1. Stevie has verified **each commit individually**, not the aggregate diff.
  2. The change has run on EAST without regression for a stated soak period. For
     remediation-only sprints, one clean start-to-steady-state cycle. For anything
     touching the data path or eBPF, longer — set explicitly, not by default.
  3. Every commit is **GPG-signed**. Unattended overnight runs may commit with
     `--no-gpg-sign` when gpg-agent has no cached key (this is expected and permitted —
     stalling an autonomous run on a human is worse). **Those commits must be re-signed
     before they cross into `main`.** The wake-up handoff names which ones.
  4. Any `[STUCK]` items are either resolved or explicitly deferred in writing.

### `main` — the known-good branch

**Question answered**: is this what we stand behind?

- Protected. No direct pushes, ever. Fast-forward or merge from `staging` only.
- Signed commits required.
- Tagged on promotion.
- **`main` must always be a state you would be willing to show someone.** That is the
  whole standard. This repo is heading for public release; `main` is the thing the
  public sees.

### Topic branch naming

`feature/<slug>` · `fix/<slug>` · `ci/<slug>` · `docs/<slug>` · `spike/<slug>`

`spike/*` branches are exploratory and are **never** promoted — they are read, learned
from, and abandoned or rewritten. `spike/mimirs-law` is the existing example.

---

## The rules that actually matter

Everything above is structure. These four are the ones that catch defects:

1. **One reviewable unit per commit.** Non-negotiable. It is the difference between a
   staging branch and a staging area.

2. **Never promote red.** A gate that is bypassed once stops being a gate. If a check is
   wrong, fix the check — do not route around it. The gosec/clippy ratchets already work
   this way: exclude by **rule ID with a stated reason**, never by severity filter,
   never by raising a threshold.

3. **Baselines only shrink.** Every ratchet file (`.gitleaksignore`,
   `gosec-ratchet-baseline.txt`, `clippy-baseline.txt`) is shrink-only and enforced by
   a script, not by discipline. A new finding cannot be silenced by appending to a
   baseline.

4. **The promoting human reads the commits, not the diffstat.** The failure mode this
   whole ADR defends against is a large, plausible-looking, mechanically-generated change
   being accepted because reviewing it properly is tedious. Commit granularity exists to
   make reviewing it *not* tedious.

---

## Rollback

Rollback is by **revert**, not by force-push. `main` and `develop` histories are
append-only. `--force-with-lease` is permitted on topic branches and on `staging` when
it is being rebuilt from `develop`; `--force` is never used anywhere.

If a promotion turns out bad:
- On `staging`: rebuild the branch from `develop` and re-promote the good subset.
- On `main`: `git revert` the merge, tag the revert, and write down what the gate missed.
  A gate that let a defect through is a gate that needs a new check — that is the
  actionable output of a rollback, not the revert itself.

---

## Consequences

**Good**
- A remediation sprint becomes reviewable instead of all-or-nothing.
- The "does it break anything else" question gets a dedicated place and a real host
  (EAST) instead of being answered by hope.
- Unsigned autonomous commits become a tracked, closeable state rather than a silent one.
- `main` becomes safe to point the public at, which is a prerequisite for public release.

**Costs, stated honestly**
- Three branches to keep in sync for one person. Real overhead. Mitigated by `develop`
  being the default and the only branch daily work touches.
- The one-commit-per-unit rule makes autonomous agent work slower and more deliberate.
  That is the intended trade, not a side effect.
- Changing the default branch to `develop` requires GitHub UI work (Stevie's account),
  along with branch protection on `main`. **Until that is done this ADR is aspirational
  on the server side and enforced only by convention locally.** That gap is tracked as
  D5 in `docs/battle-plans/STAGING-LADDER-DECISIONS.md`.

**Deliberately deferred**
- No release-candidate branches, no hotfix lane, no versioned release train. A solo
  pre-alpha project does not have the traffic to justify them. Revisit when there are
  external adopters who can be broken by a bad `main`.
- No required PR reviews — there is one committer. The gates are CI and the staging
  soak, not a second pair of eyes that does not exist.

---

## Status of adoption

| Element | State |
|---|---|
| `develop` exists, ahead of `main` | Yes |
| `staging` branch created | Yes — 2026-08-03, Staging Ladder sprint |
| CI runs on `main`/`develop`/`staging` | Yes — `static-analysis.yml` already targets all three |
| EAST as staging deploy target | Host is live; automated deploy from `staging` **not yet wired** |
| Default branch = `develop` | **Not done** — needs GitHub UI (D5) |
| Branch protection on `main` | **Not done** — needs GitHub UI (D5) |
| Signed commits required on `main` | Convention today; enforced once protection lands |

The first real exercise of this workflow is the Staging Ladder sprint
(`docs/battle-plans/STAGING-LADDER-2026-08-03.md`), which is deliberately structured to
produce exactly the per-commit reviewability this ADR requires.
