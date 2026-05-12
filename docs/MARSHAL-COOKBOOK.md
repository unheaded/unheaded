# The Marshal Cookbook

**Audience**: Future Marshal-mode shifts (Claude Code sessions running with `/unheaded-marshal` discipline).
**Purpose**: Distilled patterns from the 2026-05-10 / 2026-05-11 extended-churn arc. What worked. What didn't. Copy-paste-runnable recipes.
**Authority**: ADR-073 (zero-lint ratchet) + `references/marshal-shift-2026-05-11-*.md` (the empirical proof these patterns work).

> "The Marshal plans the war so the soldiers fight, not think." — Warmonger
>
> "The Marshal makes sure the soldiers don't fight the wrong war." — Marshal

This cookbook is the playbook between those two roles.

---

## Recipe 1 — Open a shift cleanly

```bash
cd /home/govan/tmp/unheaded
make health                                    # 11-gate snapshot, ~95s
cat references/next-session-pickup-*.md | head -100  # most-recent pickup
git log --oneline | head -10
```

If `make health` says KINGDOM HEALTHY → you have a clean foundation. If DEGRADED → fix before any new work. ADR-073 ratchet says zero lint is the floor; tests and build green are non-negotiable.

## Recipe 2 — Drain a noisy lint pool

The 2026-05-11 shift drained 2362 lint findings to 0 in ~95 commits. Pattern:

1. **Identify the rule's signal-to-noise.** Sample 5-10 findings. Real bugs or false positives?
2. **For false-positive pools**: add a `.golangci.yml` global exclude with a rationale block comment. ADR-073 requires the rationale.
3. **For real bugs**: fix them. Don't annotate. Real bugs caught this way include RSA-only cert assertion crashes, Zip Slip in container runtime, SQL-injection vector closure, decompression-bomb DoS cap, HTTP/2 SETTINGS_MAX_FRAME_SIZE infinite loop bump.
4. **For site-specific false positives**: `//nolint:gosec` with rationale on the same line.

```bash
# Sample distribution of a single rule
golangci-lint run --enable-only=gosec ./... 2>&1 | grep -oE "G[0-9]+" | sort | uniq -c | sort -rn
# Sample top files for a rule pool
golangci-lint run --enable-only=errcheck ./... 2>&1 | grep -E "^[a-z]" | awk -F: '{print $1}' | sort | uniq -c | sort -rn | head -10
```

Bulk perl pattern for cleanup paths (worked across ~50 files this session):

```bash
perl -i -pe 's/^(\s*)defer conn\.Close\(\)$/$1defer func() { _ = conn.Close() }()/g' path/to/file.go
perl -i -pe 's/^(\s*)conn\.Close\(\)$/$1_ = conn.Close()/g' path/to/file.go
# Verify
go build ./pkg/... && go test -short -timeout 60s ./pkg/...
```

Commit cadence: every 3-5 file batches. ADR-073 says zero is the floor. Once you've dropped below 50 findings, switch from bulk perl to per-site review.

## Recipe 3 — Stuck on a hard problem? Skip Protocol

If a step is taking >3× the expected time OR you've tried >2 debug branches without progress: STOP. Mark `[STUCK]` in the plan, commit known-good state, scan forward for non-blocked work, continue.

```bash
git add -u && git commit --no-gpg-sign -m "[PLAN] STUCK at step N — <reason>; skipping to step M"
```

The Skip Protocol exists for a reason. Pride doesn't ship code. Forward progress does.

## Recipe 4 — A new gosec rule fires; should I exclude it globally?

**Default answer: NO.** Sample 5-10 hits first. Per the ADR-073 protocol:

1. **Real bug?** → Fix it. Real bugs found via gosec triage this session: 6 unguarded type-assertion crashes (RSA + ECDSA), Zip Slip in container whiteout extraction, HTTP path-existence info disclosure, object-storage path traversal, audit SQL-injection vector, Slowloris HTTP timeouts, decompression-bomb cap. Every one was hidden by lint noise.
2. **Site-specific false positive?** → `//nolint:gosec` with rationale, same line.
3. **Rule-wide noise across the kingdom (every hit is FP)?** → `.golangci.yml` global exclude with rationale block.
4. **Path-specific intentional choice (container-runtime needs cross-UID perms)?** → `linters.exclusions.rules` with `path:` + `text:` filter.

What NEVER goes in `.golangci.yml`:
- Global excludes for rules that catch at least one real bug elsewhere
- Excludes without a rationale comment
- Excludes that mask incomplete refactoring

## Recipe 5 — Run unattended overnight; what does the next morning look like?

Pattern (from `feedback_overnight_churn_pattern.md` + 2026-05-04 + 2026-05-11):

1. Stevie authorizes with explicit duration ("12 hours", "till 8am CST", "as long as possibly")
2. Marshal opens with `make health` → captures baseline
3. Marshal-safe work only:
   - No architectural decisions
   - No protocol wire format changes
   - No git push --force / git reset --hard
   - No `--no-verify` bypass
4. Commit cadence every 3-5 steps (per Warmonger's cookbook)
5. Each commit must leave gates green (`make health` PASS)
6. Mid-shift checkpoint reports every ~30-50 commits (`references/marshal-shift-<DATE>-<TAG>.md`)
7. End-of-shift: write the final-checkpoint report + next-session pickup doc
8. Stevie wakes to: clean kingdom + commit chain + shift report explaining what landed

## Recipe 6 — Commit message hygiene

Per the user feedback in `feedback_no_doctrine_preamble.md` + CLAUDE.md "Commit Message Style — Don'ts":

- **NO sign-off mantras** in commit bodies ("LOVE SERVE REMEMBER", "PEACE AND LOVE", "KGLW", "<3"). Doctrine lives in CLAUDE.md and file footers.
- **NO marketing language**. No "groundbreaking", "revolutionary".
- **NO doctrine-affirmation lines**. Reference the doctrine commit hash if needed.
- **NO LoC counts**. Use behavioral metrics (loss, latency, test pass/fail, gate status).
- **YES `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`** as a single-line trailer when Claude contributed meaningfully.

## Recipe 7 — `[DECIDE]` vs `[ESCALATE]` in battle plans

From Warmonger's v2 cookbook:
- `[DECIDE]`: pre-seeded recommendation, Marshal proceeds autonomously. Use when an evidence-backed default exists.
- `[ESCALATE]`: human required, no recommendation possible. STOP and wait. Use sparingly.
- `[BLOCKED]`: ONLY for "blocked by upstream STUCK." Never for decisions.

If you find yourself wanting to add a question for Stevie mid-flight, ask: is there a pre-seeded recommendation I can write? If yes, use `[DECIDE]` and proceed. If no, use `[ESCALATE]` and stop.

## Recipe 8 — Scaffolding pattern (proven on tasks #65, #68, #71)

For multi-phase work that's blocked on a distant horizon (Q4 2026, etc.):

1. **Author the README** at the directory root that documents what will live there.
2. **Author the configuration/schema** (Cargo.toml features, JSON Schema, YAML schema).
3. **Author the runbooks** (build-X.md, verify-X.md, operator-X.md).
4. **Author the script skeletons** with `TODO(task #N)` comments for the real implementation gates.
5. **Wire the CI** (GHA + Jenkinsfile stub) so any new addition immediately ratchets against the discipline.
6. **Cross-link** so the discovery path is clear: top-level README → component READMEs → scripts → schemas.

The scaffold IS the contract for what the implementation will execute. Marshal-safe because nothing runs in production; everything is documented intent.

## Recipe 9 — Round Table

Convene the Round Table when:
- Captain Track / age transition / strategic decision
- Multiple skills disagree
- Major milestone review

Don't convene for:
- Trivial decisions
- Daily standup
- Single-skill work

The Round Table is expensive (loads all 19 skills' worth of context). It earns its cost in alignment.

## Recipe 10 — Tooling shorthand

```bash
# Health
make health                                          # 8-section gate check, ~95s
golangci-lint run ./...                              # ADR-073 ratchet
~/go/bin/govulncheck ./...                           # Go side vulns
( cd ebpf && cargo audit )                           # Rust crate vulns

# State
git log --oneline --since='1 day ago' | wc -l        # commits today
git log --oneline --since='2026-05-10 23:00' | wc -l # session count
git status --porcelain | wc -l                       # uncommitted changes

# Battle plans
ls references/battle-plan-*.md                       # all plans
cat references/next-session-pickup-*.md              # latest pickup

# Linting top files for a pool
golangci-lint run --enable-only=<rule> ./... 2>&1 | grep -E "^[a-z]" | awk -F: '{print $1}' | sort | uniq -c | sort -rn | head -10

# Coverage low-hanging fruit
go test -short -cover -count=1 -timeout 120s ./pkg/... 2>&1 | grep "coverage:" | grep -v "100.0%\|0.0%" | awk '{for (i=NF; i>0; i--) if ($i ~ /[0-9]+\.[0-9]+%/) {print $i, $2; break}}' | sort -n | head -10
```

## Recipe 11 — When to stop

You're done when ANY of:

1. Stevie says stop.
2. Authorized duration elapsed.
3. You've exhausted Marshal-safe queued work AND the kingdom is healthy.
4. A real architectural decision needs Stevie's input.
5. You're inside an infinite-debug loop (apply Skip Protocol; resume on other work).

Don't stop just because you hit a "natural stopping point" if Stevie said "as long as possibly." Find the next Marshal-safe chip and keep going.

---

## Anti-patterns observed (don't repeat)

- **Vague step descriptions** that confuse a fresh executor mid-shift
- **Bulk perl with insufficient verification** (broke pool.Close() that returns void, took an extra cycle to revert)
- **"It should work" debug branches** without actual diagnosis
- **Committing before verifying** (recipe for cascading failure)
- **Skipping the shift report** at end-of-shift (next session inherits ambiguity)
- **Bypassing the pre-commit hook** with `--no-verify` (catches gofmt + SPDX + go-vet drift; hook failures = real issues to fix, not bypass)
- **Re-litigating session-defining decisions** (Captain Track C, ADR-073, etc. are decided; don't open them again)

---

## See also

- `docs/adr/ADR-073-lint-policy-zero-findings.md` — the ratchet that makes the cookbook possible
- `docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md` — in-tree plans + drift policy
- `scripts/kingdom-health.sh` — the gate runner
- `references/next-session-pickup-2026-05-11.md` — canonical handoff template
- `references/marshal-shift-2026-05-11-*.md` — the empirical proof
- The Warmonger skill — the legislative branch
- The Marshal skill — the executive branch
- The Round Table skill — the judicial branch (when skills disagree)

---

*Free to use. Free to share.*
