# S-QUANTUM-ARMOR progress log

Plan: `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`. This file is the session log
the plan refers to, and where `[STUCK]` / `[ESCALATE]` markers live.

## Track: Phases 0-4 (Coordinator + Developer), worktree off develop `424edf47`

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed
debug attempts, mark it [STUCK] with what I tried, commit known-good state first, continue
with the next step that does not depend on it, and finish with a STUCK/ESCALATE report
ordered by how many steps each item unblocks." [ESCALATE] steps are never attempted; they
are reported.

### Plan amendments (Marshal Pattern 6)

- Step 1: `$SCRATCH` is a private session scratch directory, not `mktemp` under `/tmp`
  (a parallel agent shares the session scratchpad; each track uses its own subdirectory).
- Step 2: `443de3f82` is not an ancestor of develop: it is the pre-re-sign copy of
  `0a605108` (same subject and content), which is. Base used: `424edf47`.
- Step 12: the local `postgres:18-alpine` image (PG 18.3) was probed instead of pulling
  `18.0-alpine`; Phase 8 picks the exact tag.
- Steps 27-31: in a linked worktree, go.mod's `replace ... => ../projects/doomgeneric/unheaded`
  does not resolve (only `cmd/wotan-ctl` imports it). Baselines ran with a scratch
  `GOWORK` that points that replace at the real sibling checkout; clean clones get a
  scratch `projects` symlink next to them. No tracked file changed for this.
- Commits: `--no-gpg-sign` always (lead re-signs at merge).

### Phase 0 (Steps 1-25): done

19/19 hypotheses TRUE. Table: `docs/security/pq-preflight-2026-10-23.md`. No container,
key or password left behind; hosts read only.

### Phase 1 (Steps 26-33): done, baseline captured

- Tools: go, cargo, docker (sudo), openssl, ssh, python3, jq, git all present.
- Go `go test -count=1 ./...`: 275 ok / 0 FAIL.
- Race (`pkg/tls`, `pkg/transport`, `pkg/mesh`, `pkg/loadbalancer`, `pkg/gungnir`,
  `pkg/wotan-client`, `services/wotan`, `cmd/heimdall-daemon`, `cmd/yggdrasil-evidence`,
  `pkg/compliance`): 46 ok / 0 FAIL.
- Rust (the plan's `tail -1` keeps the doc-test line): waf, zhend, trace-collector,
  upc-bootctl all build and pass; waf lib 52 + 1, upc-bootctl 25.
- Gates: check-image-tags, check-dependency-baseline, check-manifest-yaml,
  check-secrets-baseline all rc=0.
- Meta-gate `--quick` in a clean clone: rc=1, three gates INCONCLUSIVE (already failing
  on a clean tree), all pre-existing, none ours:
  - `live-path-inventory`: `cmd/zhen-claude` (ADR-104) was added without regenerating
    `docs/LIVE-PATHS.md`. Step 79's regeneration fixes it.
  - `check-ruff`, `check-python-audit`: ruff and pip-audit are not installed on west.
    With the CI-pinned tools in a scratch venv (`--require-hashes` from
    `.github/ci-python/requirements.txt`) they run; later meta-gate runs use that venv.
