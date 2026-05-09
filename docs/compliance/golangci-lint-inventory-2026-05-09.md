# golangci-lint Kingdom-Wide Inventory — 2026-05-09

**Auditor:** Marshal continuation shift, post-`continue till all phases` directive
**Tool:** `golangci-lint 2.10.1` (built 2026-02-17)
**Config:** `.golangci.yml` v2 schema (drain-shift migrated; this shift's first end-to-end run)

---

## Headline numbers

| Linter | Count |
|--------|-------|
| **gosec** | 739 |
| **errcheck** | 728 |
| **staticcheck** | 411 |
| **govet** | 330 |
| **unused** | 94 |
| **bodyclose** | 60 |
| **TOTAL** | **2,362** |

The kingdom has accumulated lint debt because `golangci-lint run` has not actually completed end-to-end since the v2-schema config landed in the 2026-05-08 drain shift (and before that, the original v1 config silently broke at some point). This is the first usable inventory in months.

---

## What this shift fixed already

1. **Config policy adjustments** (commit `c48d4511`):
   - `govet.disable: shadow` — flags idiomatic `if err := f(); err != nil` patterns; would otherwise generate hundreds of noise findings
   - `errcheck.exclude-functions`: added idiomatic resource cleanup (`(*sql.Rows|Stmt|DB).Close`, `io.Closer.Close`, `*json.Encoder.Encode`-to-HTTP)
   - `gosec.excludes`: added `G117` (false positive on `Config.Password` struct field — legitimate naming, not a hardcoded secret)

2. **Real bug fixed** (`c48d4511`):
   - `cmd/pqc-verifier/config.go:55` — `PQC_DEFAULT_TIER=300` was silently wrapping to tier=44 via `uint8(300)` truncation. Now range-checked + 4-case test pinning the rejection.

---

## G115 (integer overflow): 29 findings, mostly false positives

Sample distribution:
- `pkg/ebpf/anamnesis_reader_linux.go` — kernel-provided int values guaranteed in-range by API contract; `int → uint32`, `int → int32`, `int → uint64` casts are safe.
- `pkg/baremetal/pxe/pxe_test.go`, `pkg/deploy/pipeline/*_test.go` — test fixtures.
- `pkg/config/config.go:575,577` — could be real, needs Architect review.

**Disposition:** Daytime Developer triage. Either annotate kernel-API casts with `// #nosec G115` or refactor to use bounded-int conversion helpers.

## G306 / G304 / G103 / G705 / G706 / G602 / G404 / G117 / G301: 17 findings combined

Mix of file-mode-too-permissive (G302/G306), file-path-from-variable (G304 — already excluded but some leak through), defer-on-result-not-checked, ineffective math, etc. Daytime triage.

---

## Triage plan (proposed, NOT executed unattended)

### Tier 1 — Fix immediately (real bugs, mechanical)
- The 1 G115 already fixed in c48d4511.
- Any remaining G115 outside kernel-API paths.
- The 2 unchecked publisher.Publish{Verified,Sovereign} in `cmd/pqc-verifier/handlers.go:134,184`.

### Tier 2 — Annotate or suppress (false positives)
- 27 G115 in kernel-API casts — `// #nosec G115` with rationale.
- 60 bodyclose where the response body is zero-bytes or already closed via defer.

### Tier 3 — Bulk refactor (style debt)
- 728 errcheck (after the idiomatic-close exclusions, real ones).
- 411 staticcheck (mix of `S1000` simplifications and real bugs).
- 94 unused (likely real, mechanical removal).

**Estimated effort:** Tier 1 + 2 = ~4 hours Developer + BlackMage pair. Tier 3 = ~1-2 days.

---

## Reproduction

```bash
cd ~/tmp/unheaded
golangci-lint run --timeout=5m ./...        # 2362 issues per the breakdown above
golangci-lint run --timeout=5m ./pkg/auth/...  # narrower scope; was 0 today after shadow-disable
```

---

## Cross-reference

- `.golangci.yml` v2 config: drain shift (2026-05-08) + this shift's policy tightening (`c48d4511`)
- ADR-052 source-of-truth policy applies: any new lint rule should be added here, not in per-package `//nolint` annotations
- `make test-rust` is now also working (this shift, `d779bda3`); both lint + test now usable for CI gating once the inventory is triaged
