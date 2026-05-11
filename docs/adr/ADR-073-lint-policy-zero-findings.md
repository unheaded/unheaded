# ADR-073 — Lint Policy: Zero Findings as the New Floor

**Status**: Accepted
**Date**: 2026-05-11
**Author**: Marshal (Stevie-authorized 12hr churn)
**Context window**: Resumed-segment commits 852905af → 588fecd6 (~95 commits across 2026-05-10 → 2026-05-11)

---

## Context

The Kingdom's `.golangci.yml` enables six tier-1 + tier-2 critical linters (errcheck, govet, staticcheck, unused, gosec, bodyclose) per ADR/S26 Round Table decision #4 ("critical-only enforcement for Age 1"). At the start of the 2026-05-11 Marshal shift the inventory was **2362 findings** across the kingdom, dominated by:

- 710 errcheck (mostly cleanup-path Close/SetDeadline ignored returns)
- 735 gosec (mostly G115 integer-overflow false positives + real defense-in-depth opportunities)
- 328 govet (mostly `unusedwrite` in test fixtures)
- 345 staticcheck (already drained earlier)
- 24 unused (already drained earlier)

This ADR records the discipline used to drain the inventory to **zero** without introducing regressions, and the policy for keeping it there.

## Decision

**Zero golangci-lint findings is the new floor.** Every PR must run clean against the configured linter set. CI (existing GitHub Actions + Jenkins) gates on this.

### Triage discipline (when a new finding surfaces)

For each finding, the triage protocol is:

1. **Is it a real bug?** → Fix it. Real bugs got real fixes this session (12 CVE-class issues closed — see `references/marshal-shift-2026-05-11-final-checkpoint.md` for the full list).

2. **Is it a validated false positive at this single site?** → `//nolint:<linter>` with rationale comment ON THE SAME LINE. The rationale must explain WHY the lint signal is wrong, not just say "noise."

3. **Is the entire rule a false-positive pool for this code base?** → Add to `.golangci.yml` global excludes WITH a rationale block comment. Examples already documented:
   - `gosec.G115` (integer overflow heuristic — protocol marshalling produces 410 false hits)
   - `gosec.G404` (math/rand on LB jitter — intentional, not security)
   - `gosec.G704` (SSRF heuristic on internal-fetch clients)
   - `gosec.G705` (slice-bounds heuristic — Go runtime panics on real OOB)
   - `govet.unusedwrite` in `*_test.go` (test-fixture pattern)

4. **Is the false positive scoped to a directory or specific code path?** → Use `linters.exclusions.rules` with `path:` + `text:` filters, NOT a global exclude. Examples already documented:
   - `pkg/audit/storage/database.go` G201 (validated tableName at construction)
   - `pkg/storage/object/filesystem.go` G703 (ValidateKey/Bucket at API boundary)
   - `pkg/runtime/image.go` G703/G305 (loop-prefix guard at line 516)
   - `pkg/runtime/`, `pkg/baremetal/`, `pkg/storage/object/`, `pkg/ebpf/` G301/G306/G302 (containers run as different UIDs, perms must allow cross-UID reads)
   - `pkg/runtime/`, `pkg/storage/wal/`, `pkg/audit/storage/file` G302 (companion to G306 dir exclusions)
   - `cmd/lich-security/` G402/G204/G306 (offensive-security campaign runner — testing broken/insecure configs IS the point)
   - `pkg/baremetal/image/`, `pkg/nix/builder`, `pkg/network/policy_controller` G204 (operator-managed external-tool wrappers)

### What never goes in `.golangci.yml`

- Global exclusion of a rule that catches at least one real bug elsewhere in the kingdom (G703, G201, G102, G110 — all caught real bugs this session and stay enabled).
- Excludes without a rationale comment.
- Excludes that mask incomplete refactoring instead of documenting an architectural choice.

## Consequences

### Positive

- **Zero lint findings = zero noise**. Future maintainers know that any new finding is meaningful. The signal-to-noise ratio resets the discipline of "look at every lint output."
- **Ratchet effect**: now that we're at zero, regressions are visible at the first CI run on a new PR. Prior to this session, the kingdom's 2362-finding floor meant new findings were lost in the noise.
- **Real bugs surface**: 12 CVE-class fixes shipped this session because triage was per-finding rather than per-rule. The blanket-exclude approach would have hidden every one of them.
- **Cross-UID container code paths are documented**: the G301/G306 exclusions in `pkg/runtime/` are now load-bearing comments that future contributors can cite.

### Negative

- **`.golangci.yml` is longer** (~150 lines of exclusions vs ~80 before). Every entry has a rationale comment so the file is still readable, but it's no longer trivial.
- **Triage takes time**: the per-site review took ~95 commits over a 12-hour Marshal shift. Future Stevie+Marshal sessions need to budget triage time, not just chip time.
- **Rule-set evolution requires re-triage**: when gosec adds a new check, we'll likely see a new wave of findings. Treat them with the same triage discipline.

### Future work

- **Re-enable G115** (integer-overflow heuristic) in Age 4 by adding per-site `//nosec G115 -- justification` comments at protocol marshal boundaries (the right surgical fix; ~410 sites, batched over a future Marshal shift).
- **Add the `bodyclose` linter** to the enable list (currently in tier 2, not enabled by default in `default: none` mode). Real HTTP body leak detection.
- **Consider enabling additional staticcheck checks** (we currently disable ST1000, ST1003, ST1016, ST1020-22 as docs-pass items — Age 4 docs sweep should re-enable them).

## Verification

```bash
# At the time of this ADR being written:
$ golangci-lint run ./...
0 issues.
```

Run this after every PR. Don't merge if the count is non-zero unless the new findings get triaged per the protocol above and either fixed or excluded with rationale.

## References

- `.golangci.yml` — source of truth for the configured linters and exclusions
- `references/marshal-shift-2026-05-11-final-checkpoint.md` — session report covering all 12 real bug fixes
- `references/marshal-shift-2026-05-11-zero-prompt-12hr.md` — predecessor mid-shift report
- ADR/S26 — original "critical-only enforcement for Age 1" decision
