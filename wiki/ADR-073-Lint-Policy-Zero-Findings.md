# ADR-073 — Lint Policy: Zero Findings as the New Floor

**See full ADR**: `docs/adr/ADR-073-lint-policy-zero-findings.md`
**Status**: Accepted (2026-05-11)

## Summary

The kingdom passes `golangci-lint run ./...` with zero issues. Triage discipline:

1. **Real bugs → real fixes** (12 CVE-class issues closed in the 2026-05-11 Marshal shift)
2. **Site-scoped FPs → inline `//nolint:<linter>` with rationale** (the WHY must be explicit)
3. **Rule-scoped FPs across the kingdom → `.golangci.yml` global excludes with rationale block**
4. **Path-scoped intentional choices → `linters.exclusions.rules` with `path:` + `text:` filters**

## What never goes in `.golangci.yml`

- Global excludes for rules that catch at least one real bug elsewhere
- Excludes without a rationale comment
- Excludes that mask incomplete refactoring instead of documenting an architectural choice

## Verification

```
$ golangci-lint run ./...
0 issues.
```

Run after every PR. Don't merge if non-zero unless triaged per protocol.

## See also

- `docs/adr/ADR-073-lint-policy-zero-findings.md` — full ADR
- `.golangci.yml` — current configured linters + exclusions (each with rationale comment)
- `references/marshal-shift-2026-05-11-final-checkpoint.md` — session report covering all 12 CVE-class fixes
